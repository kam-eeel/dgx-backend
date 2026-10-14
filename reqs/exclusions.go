package reqs

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"degree-explorer/types"
)

const prevConcurrentPat = `(?i:(?:if\s+)?taken\s+previously\s+or\s+concurrently)\b`

var (
	// termRe recognizes offering qualifiers: "offered in Winter 2010", "Fall
	// 2024", "taken in 2019", and ACORN section/session codes ("LEC0201 20261").
	termRe = regexp.MustCompile(`(?i)\b(?:offered|taken)\b|\b(?:fall|winter|summer)(?:\s+session)?\s+\d{4}\b|^\d{4}(?:-\d{2,4})?$|\b(?:LEC|TUT|PRA)\d{4}\b|\b20\d{2}[159]\b`)

	// condClauseRe: a condition right after a course ("LINC61H3 if the same
	// language was analyzed") limits when that course excludes.
	condClauseRe = regexp.MustCompile(`(?i)^(?:if|when)\b`)

	// titleRe: an unbracketed topic title after a course ("RSM412H1 Financial
	// Trading Strategies", "WGS335H1: The Politics of Girlhood"). Words are
	// capitalized apart from short connectors, and there are at least two.
	titleRe = regexp.MustCompile(`^[:\-–]?\s*[A-Z][\w'’]*(?:\s+(?:[A-Z][\w'’]*|of|in|the|and|for|to|a|an|on|at|around|with|from|&))+$`)

	// gradCodeRe: a graduate course code ("PHM1130H", "JFK1120H").
	gradCodeRe = regexp.MustCompile(`^[A-Z]{3}\d{4}[HY]\d?$`)

	// Sentence rewrites (exclusions only).
	alsoExcludedRe = regexp.MustCompile(`(?i)^also\s+excluded\s+(?:are|is)\b\s*:?\s*`)
	// "<course> (if) taken previously or concurrently": taken already covers
	// in progress. Only stripped right after a course or "course", so "A or
	// equivalent, taken previously ..." keeps its qualifier note.
	prevConcurrentRe = regexp.MustCompile(`(` + codePat + `|\b(?i:courses?))(?:\s*\(\s*` + prevConcurrentPat + `\s*\)|\s*,?\s*` + prevConcurrentPat + `)`)
	// "FSL300+ level" -> "FSL 300+ level", so the designator reads as a word.
	gluedLevelRe = regexp.MustCompile(`\b([A-Z]{3})([1-4]00(?:\s*\+|-level|\s+level))`)
	// "<Title> taken under URB432H1 ..." -> "URB432H1 (<Title>) ...".
	takenUnderRe = regexp.MustCompile(`(?i)^([^,;()]+?)\s+taken\s+under\s+(` + codePat + `)`)
	// "except A, B, AP, IB or GCE Transfer Credits": the non-course tail of an
	// except list is bracketed, so it stays with the filter it qualifies.
	exceptTailRe = regexp.MustCompile(`((?i:except|excluding)\s+` + codePat + `(?:\s*(?:,|/|(?i:and|or)\b)\s*` + codePat + `)*)\s*,\s*([^;()]*?(?i:transfer\s+credits?))`)

	// placementPhraseRe: "placement in A, B or C-level courses based on the
	// results of the French Placement Test" is one placement condition, not a
	// list of courses.
	placementPhraseRe = regexp.MustCompile(`(?i)\bplacement\s+(?:in|at)\b[^;]*?\bplacement\s+test\b`)

	// exclAdvisoryRe: list entries that are remarks, not exclusions.
	exclAdvisoryRe = regexp.MustCompile(`(?i)\bnot\s+recommended\b|\bdoes\s+not\s+count\b|^none$`)

	// Kinds for prior-knowledge exclusions; see exclKind.
	speakersRe = regexp.MustCompile(`(?i)\bspeakers?\b`)
	schoolRe   = regexp.MustCompile(`(?i)\bgrade\s+\d{1,2}\b|\bOAC\b`)

	filterHeadRe = regexp.MustCompile(`^(?i:any|an?|one)\s+`)
	exceptSepRe  = regexp.MustCompile(`(?i)[\s,/]+|\b(?:and|or)\b`)
)

// separators: in an exclusion list every top-level separator means "any of these".
var separators = []tokKind{tComma, tCommaAnd, tCommaOr, tSemi, tSlash, tAnd, tOr}

// ParseExclusions turns a raw exclusions field into its stored JSON form
func ParseExclusions(raw string) *types.Exclusions {
	e := &types.Exclusions{Raw: raw, Items: []types.ExclusionItem{}}
	text, urls := normalize(raw)
	text = noteBreakRe.ReplaceAllString(text, ". $1:") // "CSCD27H3 NOTE: ..."

	var parts []string
	for _, s := range splitSentences(text) {
		if isAdvisory(s) {
			e.Notes = append(e.Notes, s)
		} else if s = rewriteExclusion(s); s != "" {
			parts = append(parts, s)
		}
	}
	e.Notes = append(e.Notes, urls...)

	src := strings.Join(parts, "; ")
	toks := mergePhrases(src, gradCodes(src, lex(src)), placementPhraseRe)
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
			t := balanceParens(src, first.start, last.end)
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
			if len(run) > 0 && strings.Contains(src[run[len(run)-1].span.end:r.span.start], ";") {
				flush()
			}
			run = append(run, r)
		}
	}
	flush()
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
		if codeRe.MatchString(g[2]) {
			return m
		}
		return g[1] + " (except " + g[2] + ")"
	})
	return strings.TrimSpace(s)
}

// itemKey identifies an item for deduping; the note doesn't count.
func itemKey(it types.ExclusionItem) string {
	return fmt.Sprint(slices.Sorted(slices.Values(it.Codes)), it.Designators, it.Levels,
		slices.Sorted(slices.Values(it.Exclude)), it.Text)
}

// gradCodes turns text tokens that are a whole graduate course code into
// course tokens.
func gradCodes(src string, toks []token) []token {
	for i, t := range toks {
		if t.kind == tText && gradCodeRe.MatchString(src[t.start:t.end]) {
			toks[i] = token{kind: tCourse, start: t.start, end: t.end,
				node: &types.ReqNode{Type: types.NodeCourse, Code: src[t.start:t.end]}}
		}
	}
	return toks
}

// mergePhrases replaces the tokens inside each match of re with one text token.
func mergePhrases(src string, toks []token, re *regexp.Regexp) []token {
	for _, m := range re.FindAllStringIndex(src, -1) {
		var out []token
		merged := false
		for _, t := range toks {
			if t.start < m[0] || t.start >= m[1] {
				out = append(out, t)
				continue
			}
			if !merged {
				out = append(out, token{kind: tText, start: m[0], end: m[1]})
				merged = true
			}
			out[len(out)-1].end = max(out[len(out)-1].end, t.end)
		}
		toks = out
	}
	return toks
}

// absorbQualifiers removes the topic title and offering terms that follow a
// course, e.g. `RSM410H1 (Special Topics...), offered in Winter 2010/Winter 2011`,
// and returns them as notes keyed by that course's node.
//
// The candidate run is every token after the course up to the next course or
// an unmatched ")" (minus separators right before it). If the run mentions an offering term,
// all of it is the note. Otherwise the note is a bracketed title directly
// after the course, or a run of words up to the next separator that is a
// title ("RSM412H1 Financial Trading Strategies") or an if/when clause.
//
// When a qualifier runs straight into the next course with no separator
// ("...offered in Winter 2026 PHL235H1"), a separator is inserted.
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
			} else if k := wordRun(toks, i+1); k > i+1 {
				words := src[toks[i+1].start:toks[k-1].end]
				if condClauseRe.MatchString(words) || titleRe.MatchString(words) {
					last = k - 1
				}
			}
		}

		if last > i {
			notes[toks[i].node] = strings.Trim(src[toks[i].end:toks[last].end], " ,:-–")
			i = last
			if i+1 < len(toks) && toks[i+1].kind == tCourse && endsCleanly(src, toks[i]) {
				out = append(out, token{kind: tComma, start: toks[i+1].start, end: toks[i+1].start})
			}
		}
	}
	return out, notes
}

// endsCleanly reports whether an absorbed qualifier ends with a bracket or
// an offering term, so a course right after it starts a new item. Prose
// ("ECO250Y0 (Summer 2023) has an exclusion to ECO225H1") doesn't.
func endsCleanly(src string, last token) bool {
	return last.kind == tRParen || last.kind == tText && termRe.MatchString(src[last.start:last.end])
}

// wordRun returns the index just past the run of text and "and" tokens
// starting at i, or i if the run is not followed by a separator, a ")" or the
// end.
func wordRun(toks []token, i int) int {
	k := i
	for k < len(toks) && (toks[k].kind == tText || toks[k].kind == tAnd) {
		k++
	}
	if k < len(toks) && toks[k].kind != tRParen && !slices.Contains(separators, toks[k].kind) {
		return i
	}
	return k
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

// segResult is what one segment parsed into. A text result has exactly one
// item.
type segResult struct {
	items []types.ExclusionItem
	text  bool
	span  span
}

// segmentItems parses one segment. Course-only trees become one item per
// combination of codes, a "courses matching" phrase becomes a filter item,
// and anything else becomes a single text item.
func segmentItems(src string, seg []token, notes map[*types.ReqNode]string) segResult {
	sp := span{seg[0].start, seg[len(seg)-1].end}
	p := newParser(src, seg)
	n := p.post(p.parseAll())
	if n == nil {
		return segResult{span: sp}
	}
	if combos := toDNF(n); combos != nil {
		var items []types.ExclusionItem
		for _, codes := range combos {
			items = append(items, types.ExclusionItem{Codes: codes, Note: noteFor(n, codes, notes)})
		}
		return segResult{items: items, span: sp}
	}
	if it, ok := filterItem(strings.Trim(src[sp.start:sp.end], " ,;:.")); ok {
		return segResult{items: []types.ExclusionItem{it}, span: sp}
	}
	s := p.spans[n]
	text, preset := balanceParens(src, s.start, s.end), types.TextKind("")
	if n.Type == types.NodeText {
		preset = n.Kind
	}
	return segResult{items: []types.ExclusionItem{{Kind: exclKind(text, preset), Text: text}}, text: true, span: sp}
}

// filterItem recognizes a whole phrase naming a set of courses by designator
// and level: "FSL 200+ level courses", "any FSL 400-level course", "Any CSC
// course except A, B", "Any PHY course (except A/ B)". Only explicit
// designators count; subject names ("any physics course") stay text.
// Non-course words in an except list ("except AP, IB ... Transfer Credits")
// become the note.
func filterItem(s string) (types.ExclusionItem, bool) {
	rest := filterHeadRe.ReplaceAllString(s, "")
	sp := qualSpan(rest)
	if !courseWordRe.MatchString(sp) || subjectRe.MatchString(sp) {
		return types.ExclusionItem{}, false
	}
	it := types.ExclusionItem{Designators: dedupe(desigRe.FindAllString(sp, -1)), Levels: parseLevels(sp)}
	if len(it.Designators) == 0 {
		return types.ExclusionItem{}, false
	}
	rest = rest[len(sp):]
	var notes []string
	for {
		m := excludingRe.FindStringSubmatch(rest)
		if m == nil {
			break
		}
		list := m[1] + m[2]
		codes := codeRe.FindAllString(list, -1)
		it.Exclude = append(it.Exclude, codes...)
		if strings.TrimSpace(exceptSepRe.ReplaceAllString(codeRe.ReplaceAllString(list, ""), "")) != "" {
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
	k := classify(text)
	switch {
	case k == types.KindPermission:
		return k
	case schoolRe.MatchString(text):
		return types.KindPreparation
	case speakersRe.MatchString(text):
		return types.KindPlacement
	case k == types.KindOther && preset != "":
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
