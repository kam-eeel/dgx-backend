package reqs

import (
	"regexp"
	"slices"
	"strings"

	"degree-explorer/types"
)

// termRe recognizes offering qualifiers: "offered in Winter 2010", "Fall 2024", "taken in 2019".
var termRe = regexp.MustCompile(`(?i)\b(?:offered|taken)\b|\b(?:fall|winter|summer)(?:\s+session)?\s+\d{4}\b|^\d{4}(?:-\d{2,4})?$`)

// separators: in an exclusion list every top-level separator means "any of these".
var separators = []tokKind{tComma, tCommaAnd, tCommaOr, tSemi, tSlash, tAnd, tOr}

// ParseExclusions turns a raw exclusions field into its stored JSON form
func ParseExclusions(raw string) *types.Exclusions {
	e := &types.Exclusions{Raw: raw, Items: []types.ExclusionItem{}}
	text, urls := normalize(raw)

	var parts []string
	for _, s := range splitSentences(text) {
		if isAdvisory(s) {
			e.Notes = append(e.Notes, s)
		} else {
			parts = append(parts, s)
		}
	}
	e.Notes = append(e.Notes, urls...)

	src := strings.Join(parts, "; ")
	toks, notes := absorbQualifiers(src, lex(src))
	seen := map[string]bool{}
	for _, seg := range splitSegments(toks) {
		for _, it := range segmentItems(src, seg, notes) {
			key := strings.Join(slices.Sorted(slices.Values(it.Codes)), ",") + "|" + it.Text
			if !seen[key] {
				seen[key] = true
				e.Items = append(e.Items, it)
			}
		}
	}
	return e
}

// absorbQualifiers removes the topic title and offering terms that follow a
// course, e.g. `RSM410H1 (Special Topics…), offered in Winter 2010/Winter 2011`,
// and returns them as notes keyed by that course's node.
//
// The candidate run is every token after the course up to the next course or
// an unmatched ")" (minus separators right before it). If the run mentions an offering term,
// all of it is the note; otherwise only a bracketed title directly after the
// course is.
func absorbQualifiers(src string, toks []token) ([]token, map[*types.ReqNode]string) {
	notes := map[*types.ReqNode]string{}
	var out []token
	for i := 0; i < len(toks); i++ {
		out = append(out, toks[i])
		if toks[i].kind != tCourse {
			continue
		}
		next, depth := i+1, 0
	scan:
		for ; next < len(toks); next++ {
			switch toks[next].kind {
			case tCourse:
				break scan
			case tLParen:
				depth++
			case tRParen:
				if depth == 0 {
					break scan // closes a bracket opened before the course
				}
				depth--
			}
		}
		last := next - 1
		for last > i && slices.Contains(separators, toks[last].kind) {
			last--
		}

		hasTerm := slices.ContainsFunc(toks[i+1:last+1], func(t token) bool {
			return t.kind == tText && termRe.MatchString(src[t.start:t.end])
		})
		if !hasTerm {
			last = i
			if i+1 < len(toks) && toks[i+1].kind == tLParen {
				if k := closingParen(toks, i+1); k > 0 && k < next {
					last = k
				}
			}
		}

		if last > i {
			notes[toks[i].node] = strings.Trim(src[toks[i].end:toks[last].end], " ,")
			i = last
		}
	}
	return out, notes
}

// closingParen returns the index of the ")" matching the "(" at open, or -1.
func closingParen(toks []token, open int) int {
	depth := 0
	for k := open; k < len(toks); k++ {
		switch toks[k].kind {
		case tLParen:
			depth++
		case tRParen:
			depth--
			if depth == 0 {
				return k
			}
		}
	}
	return -1
}

// splitSegments splits at every separator outside brackets.
func splitSegments(toks []token) [][]token {
	var segs [][]token
	var cur []token
	depth := 0
	for _, t := range toks {
		switch {
		case t.kind == tLParen:
			depth++
		case t.kind == tRParen:
			depth = max(0, depth-1)
		case depth == 0 && slices.Contains(separators, t.kind):
			if len(cur) > 0 {
				segs = append(segs, cur)
			}
			cur = nil
			continue
		}
		cur = append(cur, t)
	}
	if len(cur) > 0 {
		segs = append(segs, cur)
	}
	return segs
}

// segmentItems parses one segment. Course-only trees become one item per
// combination of codes; anything else becomes a single text item.
func segmentItems(src string, seg []token, notes map[*types.ReqNode]string) []types.ExclusionItem {
	p := newParser(src, seg)
	n := p.post(p.parseAll())
	if n == nil {
		return nil
	}
	if combos := toDNF(n); combos != nil {
		var items []types.ExclusionItem
		for _, codes := range combos {
			items = append(items, types.ExclusionItem{Codes: codes, Note: noteFor(n, codes, notes)})
		}
		return items
	}
	if n.Type == types.NodeText {
		return []types.ExclusionItem{{Kind: n.Kind, Text: n.Text}}
	}
	s := p.spans[n]
	text := strings.Trim(src[s.start:s.end], " ,;:.")
	return []types.ExclusionItem{{Kind: classify(text), Text: text}}
}

// noteFor joins the qualifier notes of the courses in codes.
func noteFor(n *types.ReqNode, codes []string, notes map[*types.ReqNode]string) string {
	var out []string
	var walk func(*types.ReqNode)
	walk = func(n *types.ReqNode) {
		if n.Type == types.NodeCourse && slices.Contains(codes, n.Code) && notes[n] != "" {
			out = append(out, notes[n])
		}
		for _, it := range n.Items {
			walk(it)
		}
	}
	walk(n)
	return strings.Join(out, "; ")
}

// toDNF turns a tree of course/and/or nodes into OR-of-ANDs, or returns nil
// if the tree contains anything other than courses.
func toDNF(n *types.ReqNode) [][]string {
	switch n.Type {
	case types.NodeCourse:
		return [][]string{{n.Code}}
	case types.NodeOr:
		var out [][]string
		for _, c := range n.Items {
			sub := toDNF(c)
			if sub == nil {
				return nil
			}
			out = append(out, sub...)
		}
		return out
	case types.NodeAnd:
		out := [][]string{{}}
		for _, c := range n.Items {
			sub := toDNF(c)
			if sub == nil {
				return nil
			}
			var next [][]string
			for _, a := range out {
				for _, b := range sub {
					next = append(next, slices.Concat(a, b))
				}
			}
			out = next
		}
		return out
	}
	return nil
}
