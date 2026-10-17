package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Unit, stream and language pools: pools with pairs among their members, pools
// joining a unit's courses, stream leads, and language counts.

var (
	// complNonDesigRe matches floors in and outside a designator: "Students
	// must take a minimum of 8.0 credits in RSM and 8.0 credits in non-RSM
	// courses".
	complNonDesigRe = regexp.MustCompile(`(?i)^students\s+must\s+take\s+a\s+minimum\s+of\s+` + amountPat + `\s+credits\s+in\s+(` + desigPat + `)\s+and\s+` + amountPat + `\s+credits\s+in\s+non-(` + desigPat + `)\s+courses(?:\s+to\s+complete\s+their\s+[a-z ]+\s+requirements)?$`)
	// complCountsAsDesigRe matches a course counted as a designator's:
	// "MGT100H1 is considered equivalent to RSM100H1 and counts as 0.5 credit
	// in RSM courses towards this requirement".
	complCountsAsDesigRe = regexp.MustCompile(`(?i)^(` + codePat + `)\s+is\s+considered\s+equivalent\s+to\s+` + codePat + `\s+and\s+counts\s+as\s+` + numPat + `\s+credits?\s+in\s+(` + desigPat + `)\s+courses\s+towards\s+this\s+requirement$`)
	// complCrNcrTranscriptRe matches a CR/NCR rule's last clause: ", and the
	// 8.0 RSM credit requirement".
	complCrNcrTranscriptRe = regexp.MustCompile(`(?i),?\s+and\s+the\s+` + amountPat + `\s+(` + desigPat + `)\s+credit\s+requirement$`)
	// complOtherUnitsRe matches "Courses offered by other academic units may
	// not be counted towards this program".
	complOtherUnitsRe = regexp.MustCompile(`(?i)^courses\s+offered\s+by\s+other\s+academic\s+units\s+may\s+not\s+be\s+counted\s+towards?\s+this\s+program$`)
	// complLeadFilterPoolRe matches a pool whose lead is a filter: "2.0 APM/MAT
	// credits at the 300+ level including 0.5 credit at the 400-level:
	// ACT230H1/ ACT240H1".
	complLeadFilterPoolRe = regexp.MustCompile(`^(?:[^:]+:\s*)?` + amountPat + `\s+((?:[A-Z]{3}/)*[A-Z]{3})\s+credits\s+at\s+the\s+([1-4])00\+\s+level\s+including\s+` + amountPat + `\s+credit\s+at\s+the\s+([1-4])00-level:\s*(.+)$`)
	// complAnyCombinationRe matches a note restating such a pool: "students can
	// take any combination of 300+ and 400+ level APM or MAT courses that adds
	// up to 2.0 credits, with the condition that at least a 0.5 credit is at
	// the 400+ level".
	complAnyCombinationRe = regexp.MustCompile(`(?i)^students\s+can\s+take\s+any\s+combination\s+of\s+300\+\s+and\s+400\+\s+level\s+(` + desigPat + `)\s+or\s+(` + desigPat + `)\s+courses\s+that\s+adds?\s+up\s+to\s+` + amountPat + `\s+credits,\s+with\s+the\s+condition\s+that\s+` + atLeastPat + `\s+an?\s+` + numPat + `\s+credit\s+is\s+at\s+the\s+400\+\s+level$`)
	// complUnitPoolRe matches a pool with any courses before it: "FRE272H1, 1.0
	// credit selected from FRE210H1/ ( FSL320H1, FSL322H1)".
	complUnitPoolRe = regexp.MustCompile(`(?i)^((?:` + codePat + `\s*,\s*)*)` + amountPat + `\s+credits?\s+(?:selected\s+|chosen\s+)?from(?:\s+the\s+following)?\s*:?\s+(.+?)\.?$`)
	// complPairUnitRe matches a pair: "( PHL205H1, PHL206H1)".
	complPairUnitRe = regexp.MustCompile(`^\(\s*(` + codePat + `)\s*,\s*(` + codePat + `)\s*\)$`)
	// complAltUnitRe matches alternatives in brackets: "( MAT235Y1/ MAT237Y1)".
	complAltUnitRe = regexp.MustCompile(`^\(\s*(` + codePat + `(?:\s*/\s*` + codePat + `)*)\s*\)$`)
	// complRecommendedUnitRe matches "( CHM249H1 strongly recommended)".
	complRecommendedUnitRe = regexp.MustCompile(`^\(\s*(` + codePat + `)\s+((?:strongly\s+)?recommended)\s*\)$`)
	// complPairInLieuRe matches an in-lieu sentence: "( MAT235H1, MAT236H1)/
	// ECO210H1 is strongly recommended for this program. Students taking one of
	// these courses can have it count in lieu of 0.5 300-level course required
	// for this program".
	complPairInLieuRe = regexp.MustCompile(`(?i)^(.+?)\s+is\s+strongly\s+recommended\s+for\s+this\s+program\.?\s+students\s+taking\s+one\s+of\s+these\s+courses\s+can\s+have\s+it\s+count\s+in\s+lieu\s+of\s+(?:an?\s+)?` + amountPat + `\s+([1-4])00-level\s+course\s+required\s+for\s+this\s+program$`)
)

// unitRows reads a row of text that is a pool of pairs or a pool joining its
// lead's filter and its members, and reports whether it did.
func (a *compAssembler) unitRows(row *compRow, text *types.ReqNode, ls compListRefs) bool {
	body, rules, notes, ok := a.leadFilterPool(row, text.Text, ls)
	if !ok {
		body, notes, ok = a.unitPool(row, text.Text, ls)
	}
	if !ok {
		return false
	}
	if text == row.r.Req {
		row.r.Req = body
	} else {
		row.r.Req.Items[0] = body
	}
	for _, r := range rules {
		a.rowUnder(row, r)
	}
	row.r.Notes = dedupe(append(row.r.Notes, notes...))
	return true
}

// leadFilterPool reads a complLeadFilterPoolRe pool as written: the union of
// the lead's filter and the members, which may be below its level, with the
// floor a constraint under the row.
func (a *compAssembler) leadFilterPool(row *compRow, text string, ls compListRefs) (*types.ReqNode, []*types.Requirement, []string, bool) {
	m := complLeadFilterPoolRe.FindStringSubmatch(complClean(text))
	if m == nil {
		return nil, nil, nil, false
	}
	var lead []string
	for _, d := range strings.Split(m[2], "/") {
		lead = append(lead, d+" "+m[3]+"00+ level courses")
	}
	s := m[1] + " credits, including at least " + m[4] + " credit at the " + m[5] + "00-level, from " + strings.Join(lead, "/ ") + "/ " + m[6]
	n, rows, notes, ok := a.filterMembers(row, s, ls)
	if !ok || n.Lists == nil || !a.keepsCodes(m[6], n, rows, notes) {
		return nil, nil, nil, false
	}
	return n, rows, notes, true
}

// unitPool reads a pool with pairs among its members as a union list, a pair
// counting as one unit. "," and "/" both separate members: a "/" cap could
// leave the pool short.
func (a *compAssembler) unitPool(row *compRow, text string, ls compListRefs) (*types.ReqNode, []string, bool) {
	s := complClean(text)
	m := complUnitPoolRe.FindStringSubmatch(s)
	if m == nil {
		return nil, nil, false
	}
	members, ok := splitUnits(m[3])
	if !ok {
		return nil, nil, false
	}
	var singles, notes []string
	var pairs []*types.ReqNode
	seen := map[string]bool{}
	total := 0.0
	add := func(code string) bool {
		if seen[code] {
			return false
		}
		seen[code] = true
		total += courseWeight(code)
		return true
	}
	for _, u := range members {
		switch {
		case codeRe.FindString(u) == u:
			if !add(u) {
				return nil, nil, false
			}
			singles = append(singles, u)
		case complRecommendedUnitRe.MatchString(u):
			x := complRecommendedUnitRe.FindStringSubmatch(u)
			if !add(x[1]) {
				return nil, nil, false
			}
			singles = append(singles, x[1])
			notes = append(notes, x[1]+" "+x[2])
		case complPairUnitRe.MatchString(u):
			x := complPairUnitRe.FindStringSubmatch(u)
			if !add(x[1]) || !add(x[2]) {
				return nil, nil, false
			}
			pairs = append(pairs, andNode(&types.ReqNode{Type: types.NodeCourse, Code: x[1]}, &types.ReqNode{Type: types.NodeCourse, Code: x[2]}))
		default:
			return nil, nil, false
		}
	}
	amount, _ := strconv.ParseFloat(m[2], 64)
	before := codeRe.FindAllString(m[1], -1)
	if len(pairs) == 0 || amount <= 0 || total < amount-1e-9 || slices.ContainsFunc(before, func(c string) bool { return seen[c] }) {
		return nil, nil, false
	}
	var items []*types.ReqNode
	if singles != nil {
		items = append(items, &types.ReqNode{Type: types.NodeCredits, Courses: singles})
	}
	items = append(items, pairs...)
	name, _, ok := a.addUnion(row, items, ls)
	if !ok {
		return nil, nil, false
	}
	pool := &types.ReqNode{Type: types.NodeCredits, Amount: amount, Lists: []string{name}}
	if before == nil {
		return pool, notes, true
	}
	return andNode(append(courseNodes(dedupe(before)), pool)...), notes, true
}

// splitUnits splits a pool's members at the "," and "/" outside brackets. ok is
// false when the brackets don't balance or a member is empty.
func splitUnits(s string) (units []string, ok bool) {
	depth, start := 0, 0
	for i, ch := range s {
		switch ch {
		case '(':
			depth++
		case ')':
			if depth--; depth < 0 {
				return nil, false
			}
		case ',', '/':
			if depth == 0 {
				units = append(units, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	units = append(units, strings.TrimSpace(s[start:]))
	if depth != 0 || slices.Contains(units, "") {
		return nil, false
	}
	return units, true
}

var (
	// complPrereqMembersRe matches courses open to students with their
	// prerequisites, which join the pool beside them: "For those with the
	// appropriate prerequisites: CSC207H1/ CSC320H1".
	complPrereqMembersRe = regexp.MustCompile(`(?i)^for\s+those\s+with\s+the\s+appropriate\s+prerequisites\s*:\s*((?:` + codePat + `[\s/,]*)+)$`)
	// complStreamLeadRe matches a lead whose sub-lines name a stream's courses:
	// "3.0 credits from Perception and Attention stream courses".
	complStreamLeadRe = regexp.MustCompile(`(?i)^(\d+\.\d+)\s+credits\s+from\s+[A-Z][\w\s&]*?\s+stream\s+courses\s*:?$`)
	// complStreamPartsRe matches courses and a pool of the stream's other
	// courses: "PHL232H1/ PSY280H1; and 2.0 credits of any of the following:
	// COG260H1".
	complStreamPartsRe = regexp.MustCompile(`(?i)^(.+?);\s*and\s+(\d+\.\d+\s+credits)\s+of\s+any\s+of\s+the\s+following\s*:\s*(.+)$`)
)

// streamPools reads pools written inside an item: a stream lead over its parts,
// a pool with a full-year course's extra credit, and a prerequisite line whose
// courses join the pool above it.
func (a *compAssembler) streamPools() {
	for _, row := range a.allRows() {
		if a.inLieuCredit(row) {
			continue
		}
		note, extra, rest := prereqMembers(row.r.Notes)
		if extra == nil {
			continue
		}
		if a.streamLead(row, extra) || joinPool(row.r.Req, extra) {
			addRaw(row, note)
			row.r.Notes = rest
		}
	}
}

// prereqMembers returns the first complPrereqMembersRe note, its codes, and the
// other notes.
func prereqMembers(notes []string) (note string, codes []string, rest []string) {
	for _, n := range notes {
		if m := complPrereqMembersRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(n), ".")); m != nil && codes == nil {
			note, codes = n, codeRe.FindAllString(m[1], -1)
			continue
		}
		rest = append(rest, n)
	}
	return note, codes, rest
}

// joinPool adds codes to a pool of listed courses with no filter.
func joinPool(n *types.ReqNode, codes []string) bool {
	if n == nil || n.Type != types.NodeCredits || len(n.Courses) == 0 || n.Designators != nil || n.Levels != nil || n.Lists != nil || n.Requirements != nil {
		return false
	}
	for _, c := range codes {
		if !slices.Contains(n.Courses, c) {
			n.Courses = append(n.Courses, c)
		}
	}
	return true
}

// streamLead makes a stream lead over one text constraint of its parts the pool
// of every course they name, plus extra, and the parts its constraint.
func (a *compAssembler) streamLead(row *compRow, extra []string) bool {
	if row.r.Req == nil || row.r.Req.Type != types.NodeText || len(row.children) != 1 {
		return false
	}
	lead := complStreamLeadRe.FindStringSubmatch(strings.TrimSpace(row.r.Req.Text))
	part := row.children[0]
	if lead == nil || part.r.Kind != types.RequirementConstraint || part.r.Req == nil || part.r.Req.Type != types.NodeText {
		return false
	}
	m := complStreamPartsRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(part.r.Req.Text), "."))
	if m == nil {
		return false
	}
	fixed, _ := parseCompletionText(m[1])
	pool, _ := parseCompletionText(m[2] + " from: " + m[3])
	if fixed == nil || !onlyCourses(fixed) || !joinPool(pool, extra) {
		return false
	}
	all := append(nodeCodes(fixed), pool.Courses...)
	total, _ := parseCompletionText(lead[1] + " credits from: " + strings.Join(all, ", "))
	if total == nil || total.Type != types.NodeCredits {
		return false
	}
	row.r.Req = total
	part.r.Req = andNode(fixed, pool)
	return true
}

var (
	// complInLieuCreditRe matches courses and a pool with its floors and a
	// course's extra credit: "BCH311H1/ MGY311Y1; 2.5 credits from MGY314H1
	// (..., 0.5 credit additional are given if taking MGY311Y1 in lieu of
	// BCH311H1)".
	complInLieuCreditRe = regexp.MustCompile(`(?i)^(.+?);\s*(\d+\.\d+\s+credits\s+from\s+[^()]+?)\s*\((.+?),\s*(\d+\.\d+)\s+credits?\s+additional\s+(?:are|is)\s+given\s+if\s+taking\s+(` + codePat + `)\s+in\s+lieu\s+of\s+((?:` + codePat + `)(?:\s*/\s*` + codePat + `)*)\)$`)
	// complLabFloorRe matches a floor on laboratory courses, which no filter
	// marks, so it is kept as content: "at least 0.5 credit must be a
	// laboratory course".
	complLabFloorRe = regexp.MustCompile(`(?i)^` + atLeastPat + `\s+(\d+\.\d+)\s+credits?\s+must\s+be\s+an?\s+laboratory\s+course$`)
)

// inLieuCredit encodes a complInLieuCreditRe row as or(and(courses replaced,
// pool), and(the course, pool less the extra credit)), each floor a constraint
// under it.
func (a *compAssembler) inLieuCredit(row *compRow) bool {
	n := row.r.Req
	if row.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || len(row.children) > 0 {
		return false
	}
	m := complInLieuCreditRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(n.Text), "."))
	if m == nil {
		return false
	}
	courses, _ := parseCompletionText(m[1])
	full, _ := parseCompletionText(m[2])
	pool, caps := poolCaps(full)
	extra, _ := strconv.ParseFloat(m[4], 64)
	replaced := codeRe.FindAllString(m[6], -1)
	// the courses must be one of the course and those it replaces, no other
	if courses == nil || courses.Type != types.NodeOr || !onlyCourses(courses) || pool == nil || pool.Type != types.NodeCredits || len(pool.Courses) == 0 ||
		!slices.Equal(slices.Sorted(slices.Values(nodeCodes(courses))), slices.Sorted(slices.Values(append([]string{m[5]}, replaced...)))) ||
		courseWeight(m[5])-courseWeight(replaced[0]) != extra || pool.Amount <= extra {
		return false
	}
	var floors []*types.Requirement
	for _, f := range strings.Split(m[3], " and ") {
		f = strings.TrimSpace(f)
		if l := complLabFloorRe.FindStringSubmatch(f); l != nil {
			v, _ := strconv.ParseFloat(l[1], 64)
			floors = append(floors, &types.Requirement{Kind: types.RequirementConstraint, Raw: f,
				Req: andNode(&types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: slices.Clone(pool.Courses)}, textNode(types.TextContent, f))})
			continue
		}
		rows, ok := readFloor(f)
		if !ok {
			return false
		}
		floors = append(floors, rows...)
	}
	less := cloneNode(pool)
	less.Amount -= extra
	lessCaps := make([]*types.ReqNode, len(caps))
	for i, c := range caps {
		lessCaps[i] = cloneNode(c)
	}
	without := orNode(courseNodes(replaced)...)
	row.r.Req = orNode(
		andNode(append([]*types.ReqNode{without, pool}, caps...)...),
		andNode(append([]*types.ReqNode{{Type: types.NodeCourse, Code: m[5]}, less}, lessCaps...)...))
	for _, f := range floors {
		a.rowUnder(row, f)
	}
	return true
}

// complLanguageCountRe matches an amount in a few languages of one designator:
// "4.0 credits in one or two primary source languages (with NML designator)".
var complLanguageCountRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+in\s+one\s+or\s+two\s+(?:[\w-]+\s+){0,3}languages\s+\(with\s+(` + desigPat + `)\s+designator\)`)

// complStreamPoolRe matches a pool of two streams or approved courses: "1.0
// credit in Medieval or Modern stream courses or those from a list of
// pre-approved courses offered by other departments".
var complStreamPoolRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+in\s+(\w+)\s+or\s+(\w+)\s+stream\s+courses\s+or\s+(those\s+from\s+a\s+list\s+of\s+pre-approved\s+courses\s+offered\s+by\s+other\s+departments)$`)

// languagePools encodes a language count as its designator plus the row's text,
// since the languages aren't in the catalogue, and a stream pool as the
// streams' lists or the approval. A stream without a list leaves the row as it
// is.
func (a *compAssembler) languagePools() {
	for _, r := range a.allRows() {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText {
			continue
		}
		t := a.clean(n.Text)
		if m := complLanguageCountRe.FindStringSubmatch(t); m != nil {
			v, _ := strconv.ParseFloat(m[1], 64)
			r.r.Req = andNode(&types.ReqNode{Type: types.NodeCredits, Amount: v, Designators: []string{m[2]}}, textNode(types.TextContent, n.Text))
			continue
		}
		m := complStreamPoolRe.FindStringSubmatch(t)
		if m == nil {
			continue
		}
		var codes, heads, used []string
		ok := true
		for _, name := range m[2:4] {
			head := strings.ToLower(name) + " stream"
			c, lines := a.streamList(head)
			if c == nil {
				ok = false
				break
			}
			codes = append(codes, c...)
			heads = append(heads, head)
			used = append(used, lines...)
		}
		if !ok {
			continue
		}
		v, _ := strconv.ParseFloat(m[1], 64)
		r.r.Req = orNode(&types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: dedupe(codes)}, textNode(types.TextPermission, m[4]))
		for _, l := range used {
			addRaw(r, l)
		}
		a.notes = slices.DeleteFunc(a.notes, func(s string) bool {
			return slices.ContainsFunc(heads, func(h string) bool { return strings.HasPrefix(strings.ToLower(s), h+":") })
		})
	}
}

// streamList returns the codes under the heading name ("_Medieval Stream:_",
// lowercased) and the lines read; nil when there is none.
func (a *compAssembler) streamList(name string) (codes, lines []string) {
	for i, l := range a.lines {
		if strings.ToLower(strings.Trim(stripEmphasis(l), " :*_")) != name {
			continue
		}
		lines = append(lines, l)
		for k := i + 1; k < len(a.lines) && mostlyCodes(a.lines[k]); k++ {
			codes = append(codes, codeRe.FindAllString(a.lines[k], -1)...)
			lines = append(lines, a.lines[k])
		}
		return codes, lines
	}
	return nil, nil
}

// pairPool reads a pool of singles and pairs the item grammar leaves as text,
// each pair making the amount alone: or(the singles' pool, and(pair)...), a
// pair counting as a unit. ok is false when s isn't such a pool.
func (a *compAssembler) pairPool(s string) (body *types.ReqNode, rows []*types.Requirement, notes []string, ok bool) {
	t := strings.TrimSuffix(complClean(s), ".")
	m := complPairPoolRe.FindStringSubmatch(t)
	if a.inPairPool || m == nil || !complPairRe.MatchString(m[2]) {
		return nil, nil, nil, false
	}
	a.inPairPool = true
	body, rows, notes = a.itemBody(s)
	defer func() { a.inPairPool = false }()
	if !hasText(body) {
		// The item grammar reads it: "( PHL205H1, PHL206H1)/ PHL210Y1".
		return body, rows, notes, true
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	var pairs []*types.ReqNode
	for _, p := range complPairRe.FindAllStringSubmatch(m[2], -1) {
		if courseWeight(p[1])+courseWeight(p[2]) != amount {
			return body, rows, notes, true
		}
		pairs = append(pairs, andNode(&types.ReqNode{Type: types.NodeCourse, Code: p[1]}, &types.ReqNode{Type: types.NodeCourse, Code: p[2]}))
	}
	rest := strings.Trim(complPairOutRe.ReplaceAllString(complPairAfterSlashRe.ReplaceAllString(m[2], ""), ""), ", /")
	if strings.ContainsAny(rest, "()[]") || !codeRe.MatchString(rest) {
		return body, rows, notes, true
	}
	singles, srows, snotes := a.itemBody(m[1] + " credit from " + rest)
	// The singles' "/" groups are caps beside the pool, capping singles only.
	if pool, _ := poolCaps(singles); hasText(singles) || !isPool(pool) || pool.Designators != nil {
		return body, rows, notes, true
	}
	return orNode(append([]*types.ReqNode{singles}, pairs...)...), srows, snotes, true
}

// languageDesignators maps a language to the designators its courses carry.
// A language's courses are among them, not all of them: NML and MHB carry
// other languages and courses in translation too.
var languageDesignators = map[string][]string{
	"arabic": {"NML"}, "persian": {"NML"}, "turkish": {"NML"}, "french": {"FRE", "FSL"}, "german": {"GER"}, "hebrew": {"MHB", "NML"}, "italian": {"ITA"},
	"latin": {"LAT"}, "portuguese": {"PRT"}, "spanish": {"SPA"}, "yiddish": {"GER"},
}
