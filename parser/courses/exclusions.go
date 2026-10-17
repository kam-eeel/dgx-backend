package courses

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"degree-explorer/parser/grammar"
	"degree-explorer/types"
)

var (

	// condClauseRe matches a condition after a course, which limits when it
	// excludes: "LINC61H3 if the same language was analyzed".
	condClauseRe = regexp.MustCompile(`(?i)^(?:if|when)\b`)

	// titleRe matches an unbracketed topic title after a course, two or more
	// capitalized words: "Financial Trading Strategies" after RSM412H1.
	titleRe = regexp.MustCompile(`^[:\-–]?\s*[A-Z][\w'’]*(?:\s+(?:[A-Z][\w'’]*|of|in|the|and|for|to|a|an|on|at|around|with|from|&))+$`)

	// Sentence rewrites (exclusions only).
	alsoExcludedRe = regexp.MustCompile(`(?i)^also\s+excluded\s+(?:are|is)\b\s*:?\s*`)
	// prevConcurrentRe matches "<course> (if) taken previously or concurrently";
	// taken covers in progress. Only right after a course or "course", so "A or
	// equivalent, taken previously ..." keeps its note.
	prevConcurrentRe = regexp.MustCompile(`(` + grammar.CodePat + `|\b(?i:courses?))(?:\s*\(\s*` + grammar.PrevConcurrentPat + `\s*\)|\s*,?\s*` + grammar.PrevConcurrentPat + `)`)
	// gluedLevelRe matches a designator glued to its level: "FSL300+ level".
	gluedLevelRe = regexp.MustCompile(`\b([A-Z]{3})(` + grammar.LevelPat + `(?:\s*\+|-level|\s+level))`)
	// takenUnderRe matches "<Title> taken under URB432H1", rewritten "URB432H1 (<Title>)".
	takenUnderRe = regexp.MustCompile(`(?i)^([^,;()]+?)\s+taken\s+under\s+(` + grammar.CodePat + `)`)
	// exceptTailRe matches an except list with a non-course tail, bracketed so it
	// stays with its filter: "except A, B, AP, IB or GCE Transfer Credits".
	exceptTailRe = regexp.MustCompile(`((?i:except|excluding)\s+` + grammar.CodePat + `(?:\s*(?:,|/|(?i:and|or)\b)\s*` + grammar.CodePat + `)*)\s*,\s*([^;()]*?(?i:transfer\s+credits?))`)

	// placementPhraseRe matches one placement condition, not a list of courses:
	// "placement in A, B or C-level courses based on the ... Placement Test".
	placementPhraseRe = regexp.MustCompile(`(?i)\bplacement\s+(?:in|at)\b[^;]*?\bplacement\s+test\b`)

	// exclAdvisoryRe matches list entries that are remarks, not exclusions: "not recommended".
	exclAdvisoryRe = regexp.MustCompile(`(?i)\bnot\s+recommended\b|\bdoes\s+not\s+count\b|^none$`)

	// speakersRe and schoolRe mark prior language or school knowledge: "Grade 8
	// Hebrew".
	speakersRe = regexp.MustCompile(`(?i)\bspeakers?\b`)
	schoolRe   = regexp.MustCompile(`(?i)\bgrade\s+\d{1,2}\b|\bOAC\b`)

	filterHeadRe = regexp.MustCompile(`^(?i:any|an?|one)\s+`)
	exceptSepRe  = regexp.MustCompile(`(?i)[\s,/]+|\b(?:and|or)\b`)
)

// separators: in an exclusion list every top-level separator means "any of these".
var separators = []grammar.TokKind{grammar.TComma, grammar.TCommaAnd, grammar.TCommaOr, grammar.TSemi, grammar.TSlash, grammar.TAnd, grammar.TOr}

// ParseExclusions parses a raw exclusions field into its stored form.
func ParseExclusions(raw string) *types.Exclusions {
	e := &types.Exclusions{Raw: raw, Items: []types.ExclusionItem{}}
	text, urls := grammar.Normalize(raw)
	text = grammar.NoteBreakRe.ReplaceAllString(text, ". $1:") // "CSCD27H3 NOTE: ..."

	var parts []string
	var timing []types.ExclusionItem
	for _, s := range grammar.SplitSentences(text) {
		if items := timingItems(s); items != nil {
			timing = append(timing, items...) // "May not be taken concurrently with …"
		} else if grammar.IsAdvisory(s) {
			e.Notes = append(e.Notes, s)
		} else if s = rewriteExclusion(s); s != "" {
			parts = append(parts, s)
		}
	}
	e.Notes = append(e.Notes, urls...)

	src := strings.Join(parts, "; ")
	toks := mergePhrases(src, grammar.GradCodes(src, grammar.Lex(src)), placementPhraseRe)
	toks, notes := absorbQualifiers(src, toks)

	seen := map[string]bool{}
	add := func(it types.ExclusionItem) {
		if key := itemKey(it); !seen[key] {
			seen[key] = true
			e.Items = append(e.Items, it)
		}
	}

	// Consecutive text segments of one sentence are one condition ("Senior
	// high school German or equivalent"), so they become a single text item.
	var run []segResult
	flush := func() {
		switch len(run) {
		case 0:
			return
		case 1:
			add(run[0].items[0])
		default:
			first, last := run[0].span, run[len(run)-1].span
			t := balanceParens(src, first.Start, last.End)
			add(types.ExclusionItem{Kind: exclKind(t, ""), Text: t})
		}
		run = nil
	}
	for _, seg := range splitSegments(toks) {
		r := segmentItems(src, seg, notes)
		switch {
		case !r.text:
			flush()
			for _, it := range r.items {
				add(it)
			}
		case exclAdvisoryRe.MatchString(r.items[0].Text):
			flush()
			e.Notes = append(e.Notes, r.items[0].Text)
		default:
			if len(run) > 0 && strings.Contains(src[run[len(run)-1].span.End:r.span.Start], ";") {
				flush()
			}
			run = append(run, r)
		}
	}
	flush()
	for _, it := range timing {
		add(it)
	}
	return e
}

// rewriteExclusion applies the exclusion-only rewrites to one sentence.
func rewriteExclusion(s string) string {
	s = alsoExcludedRe.ReplaceAllString(s, "")
	s = prevConcurrentRe.ReplaceAllString(s, "$1")
	s = gluedLevelRe.ReplaceAllString(s, "$1 $2")
	s = takenUnderRe.ReplaceAllString(s, "$2 ($1)")
	s = exceptTailRe.ReplaceAllStringFunc(s, func(m string) string {
		g := exceptTailRe.FindStringSubmatch(m)
		if grammar.CodeRe.MatchString(g[2]) {
			return m
		}
		return g[1] + " (except " + g[2] + ")"
	})
	return strings.TrimSpace(s)
}

// itemKey identifies an item for deduping; the note doesn't count.
func itemKey(it types.ExclusionItem) string {
	return fmt.Sprint(slices.Sorted(slices.Values(it.Courses)), it.Designators, it.Levels,
		slices.Sorted(slices.Values(it.Exclude)), it.Concurrent, it.Text)
}

// mergePhrases replaces the tokens inside each match of re with one text token.
func mergePhrases(src string, toks []grammar.Token, re *regexp.Regexp) []grammar.Token {
	for _, m := range re.FindAllStringIndex(src, -1) {
		var out []grammar.Token
		merged := false
		for _, t := range toks {
			if t.Start < m[0] || t.Start >= m[1] {
				out = append(out, t)
				continue
			}
			if !merged {
				out = append(out, grammar.Token{Kind: grammar.TText, Start: m[0], End: m[1]})
				merged = true
			}
			out[len(out)-1].End = max(out[len(out)-1].End, t.End)
		}
		toks = out
	}
	return toks
}

// absorbQualifiers moves each course's qualifiers into notes keyed by its node:
// the run to the next course if it names a term ("offered in Winter 2010"), else
// a title or if/when clause. One running into the next course gets a separator.
func absorbQualifiers(src string, toks []grammar.Token) ([]grammar.Token, map[*types.ReqNode]string) {
	notes := map[*types.ReqNode]string{}
	var out []grammar.Token
	for i := 0; i < len(toks); i++ {
		out = append(out, toks[i])
		if toks[i].Kind != grammar.TCourse {
			continue
		}
		next, depth := i+1, 0
	scan:
		for ; next < len(toks); next++ {
			switch toks[next].Kind {
			case grammar.TCourse:
				break scan
			case grammar.TLParen:
				depth++
			case grammar.TRParen:
				if depth == 0 {
					break scan // closes a bracket opened before the course
				}
				depth--
			}
		}
		last := next - 1
		for last > i && slices.Contains(separators, toks[last].Kind) {
			last--
		}

		hasTerm := slices.ContainsFunc(toks[i+1:last+1], func(t grammar.Token) bool {
			return t.Kind == grammar.TText && grammar.TermRe.MatchString(src[t.Start:t.End])
		})
		if !hasTerm {
			last = i
			if i+1 < len(toks) && toks[i+1].Kind == grammar.TLParen {
				if k := closingParen(toks, i+1); k > 0 && k < next {
					last = k
				}
			} else if k := wordRun(toks, i+1); k > i+1 {
				words := src[toks[i+1].Start:toks[k-1].End]
				if condClauseRe.MatchString(words) || titleRe.MatchString(words) {
					last = k - 1
				}
			}
		}

		if last > i {
			notes[toks[i].Node] = strings.Trim(src[toks[i].End:toks[last].End], " ,:-–")
			i = last
			if i+1 < len(toks) && toks[i+1].Kind == grammar.TCourse && endsCleanly(src, toks[i]) {
				out = append(out, grammar.Token{Kind: grammar.TComma, Start: toks[i+1].Start, End: toks[i+1].Start})
			}
		}
	}
	return out, notes
}

// endsCleanly reports whether an absorbed qualifier ends with a bracket or
// an offering term, so a course right after it starts a new item. Prose
// ("ECO250Y0 (Summer 2023) has an exclusion to ECO225H1") doesn't.
func endsCleanly(src string, last grammar.Token) bool {
	return last.Kind == grammar.TRParen || last.Kind == grammar.TText && grammar.TermRe.MatchString(src[last.Start:last.End])
}

// wordRun returns the index just past the run of text and "and" tokens
// starting at i, or i if the run is not followed by a separator, a ")" or the
// end.
func wordRun(toks []grammar.Token, i int) int {
	k := i
	for k < len(toks) && (toks[k].Kind == grammar.TText || toks[k].Kind == grammar.TAnd) {
		k++
	}
	if k < len(toks) && toks[k].Kind != grammar.TRParen && !slices.Contains(separators, toks[k].Kind) {
		return i
	}
	return k
}

// closingParen returns the index of the ")" matching the "(" at open, or -1.
func closingParen(toks []grammar.Token, open int) int {
	depth := 0
	for k := open; k < len(toks); k++ {
		switch toks[k].Kind {
		case grammar.TLParen:
			depth++
		case grammar.TRParen:
			depth--
			if depth == 0 {
				return k
			}
		}
	}
	return -1
}

// splitSegments splits at every separator outside brackets.
func splitSegments(toks []grammar.Token) [][]grammar.Token {
	var segs [][]grammar.Token
	var cur []grammar.Token
	depth := 0
	for _, t := range toks {
		switch {
		case t.Kind == grammar.TLParen:
			depth++
		case t.Kind == grammar.TRParen:
			depth = max(0, depth-1)
		case depth == 0 && slices.Contains(separators, t.Kind):
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

// segResult is what one segment parsed into. A text result has exactly one
// item.
type segResult struct {
	items []types.ExclusionItem
	text  bool
	span  grammar.Span
}

// segmentItems parses one segment. Course-only trees become one item per
// combination of codes, a "courses matching" phrase becomes a filter item,
// and anything else becomes a single text item.
func segmentItems(src string, seg []grammar.Token, notes map[*types.ReqNode]string) segResult {
	sp := grammar.Span{Start: seg[0].Start, End: seg[len(seg)-1].End}
	p := grammar.NewParser(src, seg)
	n := p.Post(p.ParseAll())
	if n == nil {
		return segResult{span: sp}
	}
	if combos := toDNF(n); combos != nil {
		var items []types.ExclusionItem
		for _, codes := range combos {
			items = append(items, types.ExclusionItem{Courses: codes, Note: noteFor(n, codes, notes)})
		}
		return segResult{items: items, span: sp}
	}
	if it, ok := filterItem(strings.Trim(src[sp.Start:sp.End], " ,;:.")); ok {
		return segResult{items: []types.ExclusionItem{it}, span: sp}
	}
	s := p.Spans[n]
	text, preset := balanceParens(src, s.Start, s.End), types.TextKind("")
	if n.Type == types.NodeText {
		preset = n.Kind
	}
	return segResult{items: []types.ExclusionItem{{Kind: exclKind(text, preset), Text: text}}, text: true, span: sp}
}

// filterItem reads a phrase naming courses by explicit designator and level
// ("any FSL 400-level course", "Any CSC course except A, B"); "any physics
// course" stays text. Non-course words in an except list become the note.
func filterItem(s string) (types.ExclusionItem, bool) {
	rest := filterHeadRe.ReplaceAllString(s, "")
	sp := grammar.QualSpan(rest)
	if !grammar.CourseWordRe.MatchString(sp) || grammar.SubjectRe.MatchString(sp) {
		return types.ExclusionItem{}, false
	}
	it := types.ExclusionItem{Designators: grammar.Dedupe(grammar.DesigRe.FindAllString(sp, -1)), Levels: grammar.ParseLevels(sp)}
	if len(it.Designators) == 0 {
		return types.ExclusionItem{}, false
	}
	rest = rest[len(sp):]
	var notes []string
	for {
		m := grammar.ExcludingRe.FindStringSubmatch(rest)
		if m == nil {
			break
		}
		list := m[1] + m[2]
		codes := grammar.CodeRe.FindAllString(list, -1)
		it.Exclude = append(it.Exclude, codes...)
		if strings.TrimSpace(exceptSepRe.ReplaceAllString(grammar.CodeRe.ReplaceAllString(list, ""), "")) != "" {
			notes = append(notes, "except "+strings.TrimSpace(list))
		}
		rest = rest[len(m[0]):]
	}
	if strings.Trim(rest, " ,;:.") != "" {
		return types.ExclusionItem{}, false
	}
	it.Note = strings.Join(notes, "; ")
	return it, true
}

// exclKind classifies an exclusion text item. Prior language or school
// knowledge ("Advanced speakers of Turkish", "Grade 8 Hebrew", "OAC Hebrew")
// is checked before the generic kinds, except permission.
func exclKind(text string, preset types.TextKind) types.TextKind {
	k := grammar.Classify(text)
	switch {
	case k == types.TextPermission:
		return k
	case schoolRe.MatchString(text):
		return types.TextPreparation
	case speakersRe.MatchString(text):
		return types.TextPlacement
	case k == types.TextOther && preset != "":
		return preset
	}
	return k
}

// balanceParens returns the trimmed text of src[start:end], extended over
// closing brackets that the span cut off.
func balanceParens(src string, start, end int) string {
	for strings.Count(src[start:end], "(") > strings.Count(src[start:end], ")") {
		k := end
		for k < len(src) && src[k] == ' ' {
			k++
		}
		if k >= len(src) || src[k] != ')' {
			break
		}
		end = k + 1
	}
	return strings.Trim(src[start:end], " ,;:.")
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
