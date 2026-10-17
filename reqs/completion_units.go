package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Node-language additions of Task 10 (stream E1): designators a pool
// excludes ("8.0 credits in non-RSM courses", exclude_designators), a
// course-weight filter ("any FRE or FCS half-course", max_course_amount,
// read by filterMembers), pairs counted as one unit in a list ("2.0 credits
// from the following: PHL200Y1/( PHL205H1, PHL206H1)/ PHL210Y1", a pair item
// and(course, course)) and a pool that is the union of its lead's filter
// and the courses it lists ("2.0 APM/MAT credits at the 300+ level
// including 0.5 credit at the 400-level: ACT230H1/ ...").

var (
	// complNonDesigRe: "Students must take a minimum of 8.0 credits in RSM
	// and 8.0 credits in non-RSM courses (to complete their Specialist
	// requirements)". The groups: the two amounts and designators.
	complNonDesigRe = regexp.MustCompile(`(?i)^students\s+must\s+take\s+a\s+minimum\s+of\s+(\d+(?:\.\d+)?)\s+credits\s+in\s+((?-i:[A-Z]{3}))\s+and\s+(\d+(?:\.\d+)?)\s+credits\s+in\s+non-((?-i:[A-Z]{3}))\s+courses(?:\s+to\s+complete\s+their\s+[a-z ]+\s+requirements)?$`)
	// complCountsAsDesigRe: the program note "MGT100H1 is considered
	// equivalent to RSM100H1 and counts as 0.5 credit in RSM courses towards
	// this requirement". The groups: the course, the designator.
	complCountsAsDesigRe = regexp.MustCompile(`(?i)^(` + codePat + `)\s+is\s+considered\s+equivalent\s+to\s+` + codePat + `\s+and\s+counts\s+as\s+\d+(?:\.\d+)?\s+credits?\s+in\s+((?-i:[A-Z]{3}))\s+courses\s+towards\s+this\s+requirement$`)
	// complCrNcrTranscriptRe: a CR/NCR rule's last clause, "..., and the 8.0
	// RSM credit requirement". The groups: the amount and the designator.
	complCrNcrTranscriptRe = regexp.MustCompile(`(?i),?\s+and\s+the\s+(\d+(?:\.\d+)?)\s+((?-i:[A-Z]{3}))\s+credit\s+requirement$`)
	// complOtherUnitsRe: "Courses offered by other academic units may not be
	// counted towards this program".
	complOtherUnitsRe = regexp.MustCompile(`(?i)^courses\s+offered\s+by\s+other\s+academic\s+units\s+may\s+not\s+be\s+counted\s+towards?\s+this\s+program$`)
	// complLeadFilterPoolRe: a pool whose lead is a filter and whose members
	// follow a colon: "Further Topics: 2.0 APM/MAT credits at the 300+ level
	// including 0.5 credit at the 400-level: ACT230H1/ ...". The groups: the
	// amount, the designators, the level, the floor's amount and level, the
	// members.
	complLeadFilterPoolRe = regexp.MustCompile(`^(?:[^:]+:\s*)?(\d+(?:\.\d+)?)\s+((?:[A-Z]{3}/)*[A-Z]{3})\s+credits\s+at\s+the\s+([1-4])00\+\s+level\s+including\s+(\d+(?:\.\d+)?)\s+credit\s+at\s+the\s+([1-4])00-level:\s*(.+)$`)
	// complAnyCombinationRe: a note restating such a pool, "students can
	// take any combination of 300+ and 400+ level APM or MAT courses that
	// adds up to 2.0 credits, with the condition that at least a 0.5 credit
	// is at the 400+ level". The groups: the designators, the amount.
	complAnyCombinationRe = regexp.MustCompile(`(?i)^students\s+can\s+take\s+any\s+combination\s+of\s+300\+\s+and\s+400\+\s+level\s+((?-i:[A-Z]{3}))\s+or\s+((?-i:[A-Z]{3}))\s+courses\s+that\s+adds?\s+up\s+to\s+(\d+(?:\.\d+)?)\s+credits,\s+with\s+the\s+condition\s+that\s+at\s+least\s+an?\s+\d+(?:\.\d+)?\s+credit\s+is\s+at\s+the\s+400\+\s+level$`)
	// complUnitPoolRe: a pool whose members include pairs, with courses
	// before it: "FRE272H1, 1.0 credit selected from FRE210H1/ ..., (
	// FSL320H1, FSL322H1)/ FSL321Y1". The groups: the courses before, the
	// amount, the members.
	complUnitPoolRe = regexp.MustCompile(`(?i)^((?:` + codePat + `\s*,\s*)*)(\d+(?:\.\d+)?)\s+credits?\s+(?:selected\s+|chosen\s+)?from(?:\s+the\s+following)?\s*:?\s+(.+?)\.?$`)
	// complPairUnitRe: a pair, "( PHL205H1, PHL206H1)".
	complPairUnitRe = regexp.MustCompile(`^\(\s*(` + codePat + `)\s*,\s*(` + codePat + `)\s*\)$`)
	// complAltUnitRe: courses in brackets that are alternatives, "(
	// MAT235Y1/ MAT237Y1)".
	complAltUnitRe = regexp.MustCompile(`^\(\s*(` + codePat + `(?:\s*/\s*` + codePat + `)*)\s*\)$`)
	// complRecommendedUnitRe: "( CHM249H1 strongly recommended)".
	complRecommendedUnitRe = regexp.MustCompile(`^\(\s*(` + codePat + `)\s+((?:strongly\s+)?recommended)\s*\)$`)
	// complPairInLieuRe: "( MAT235H1, MAT236H1)/( MAT235Y1/ MAT237Y1)/
	// ECO210H1 is strongly recommended for this program. Students taking one
	// of these courses can have it count in lieu of 0.5 300-level course
	// required for this program". The groups: the members, the amount, the
	// level.
	complPairInLieuRe = regexp.MustCompile(`(?i)^(.+?)\s+is\s+strongly\s+recommended\s+for\s+this\s+program\.?\s+students\s+taking\s+one\s+of\s+these\s+courses\s+can\s+have\s+it\s+count\s+in\s+lieu\s+of\s+(?:an?\s+)?(\d+(?:\.\d+)?)\s+([1-4])00-level\s+course\s+required\s+for\s+this\s+program$`)
)

// unitRows reads, in listRefRow (oneOffRow) with the lists known, a row of
// text that is a pool of pairs (unitPool) or a pool joining its lead's
// filter and its members (leadFilterPool). It reports whether it read the
// row.
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

// leadFilterPool reads "2.0 APM/MAT credits at the 300+ level including 0.5
// credit at the 400-level: ACT230H1/ ACT240H1/ ... any APM400-level course/
// ..." as written (D4): the union list of the lead's filter (APM and MAT at
// the 300+ level) and the members, which include 200-level courses, with
// the floor a constraint under the row (filterMembers).
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

// unitPool reads a pool whose members are courses and pairs ("2.0 credits
// from the following: PHL200Y1/( PHL205H1, PHL206H1)/ PHL210Y1") as a union
// list of the row's own (addUnion) whose items are the single courses and a
// pair item and(course, course) per pair: the pair counts as one unit,
// claimed together, worth its summed credits. "," and "/" between members
// both separate them: a "/" cap of the heaviest member would leave the
// pool short of its amount ("2.0 credits from" three 1.0 members), and the
// courses of a "/" group exclude each other in the catalogue anyway, so
// none is added. Courses before the pool ("FRE272H1, 1.0 credit selected
// from ...") are required beside it; "( CHM249H1 strongly recommended)" is
// the course and a note. Nothing changes unless a member is a pair.
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

// splitUnits splits a pool's members at the "," and "/" outside brackets
// ("PHL200Y1/( PHL205H1, PHL206H1)/ PHL210Y1": three members). ok is false
// when the brackets don't balance or a member is empty.
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

// designatorRules reads, after the routes are placed (oneOffs), the
// program rules written with designators a pool excludes:
//   - "Students must take a minimum of 8.0 credits in RSM and 8.0 credits
//     in non-RSM courses", a row of text, is a constraint
//     and(credits{8, designators: [RSM]}, credits{8, exclude_designators:
//     [RSM]}); the note "MGT100H1 is considered equivalent to RSM100H1 and
//     counts as 0.5 credit in RSM courses towards this requirement" puts
//     MGT100H1 in the first and excludes it from the second, and joins the
//     row's raw. The constraint counts over the transcript (scope
//     transcript): its amounts sum to more than the program's total. A CR/NCR
//     limit that adds "and the 8.0 RSM credit requirement" gets a second
//     limit over the transcript, credits{cr_ncr, designators: [RSM],
//     courses: [MGT100H1], max_amount: 0}, with the same raw;
//   - "Courses offered by other academic units may not be counted towards
//     this program" is a limit of 0 on courses without the designators the
//     program's allocations draw on, excluding the courses its text names
//     from other units (the Religion Minor's Modern Hebrew courses, which
//     "will count toward the Department for the Study of Religion courses").
func (a *compAssembler) designatorRules() {
	for _, r := range a.allRows() {
		n := r.r.Req
		if r.r.Kind != types.RequirementConstraint || n == nil || n.Type != types.NodeText || n.Kind != types.TextOther || len(r.children) > 0 {
			continue
		}
		t := strings.TrimSuffix(strings.TrimSpace(n.Text), ".")
		if m := complNonDesigRe.FindStringSubmatch(t); m != nil && m[2] == m[4] {
			in, _ := strconv.ParseFloat(m[1], 64)
			out, _ := strconv.ParseFloat(m[3], 64)
			yes := &types.ReqNode{Type: types.NodeCredits, Amount: in, Designators: []string{m[2]}}
			no := &types.ReqNode{Type: types.NodeCredits, Amount: out, ExcludeDesignators: []string{m[2]}}
			a.notes = slices.DeleteFunc(a.notes, func(note string) bool {
				x := complCountsAsDesigRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(note), "."))
				if x == nil || x[2] != m[2] || x[1][:3] == m[2] {
					return false
				}
				yes.Courses = append(yes.Courses, x[1])
				no.Exclude = append(no.Exclude, x[1])
				addRaw(r, note)
				return true
			})
			r.r.Req = andNode(yes, no)
			// Counted over the transcript: the two amounts sum to more than
			// any of the three programs' totals (16.0 against 12.5 to 15.0).
			r.r.Scope = types.ScopeTranscript
			a.transcriptCrNcr(m[1], yes)
			continue
		}
		if complOtherUnitsRe.MatchString(t) {
			if lim := a.ownUnitsLimit(); lim != nil {
				r.r.Kind, r.r.Req = types.RequirementLimit, lim
			}
		}
	}
}

// transcriptCrNcr adds, after a program-level CR/NCR limit of 0 that also
// names the transcript constraint on the designator's credits ("The CR/NCR
// option cannot be used for any program completion requirements, and the 8.0
// RSM credit requirement"), the same limit over the transcript on the
// courses that count toward it (pool: the RSM courses and MGT100H1).
func (a *compAssembler) transcriptCrNcr(amount string, pool *types.ReqNode) {
	for i, r := range a.program {
		n := r.r.Req
		if r.r.Kind != types.RequirementLimit || n == nil || n.Type != types.NodeCredits || !n.CrNcr || n.MaxAmount == nil || *n.MaxAmount != 0 || len(n.Designators) > 0 || len(n.Courses) > 0 {
			continue
		}
		m := complCrNcrTranscriptRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(r.r.Raw), "."))
		if m == nil || m[1] != amount || m[2] != pool.Designators[0] {
			continue
		}
		k := &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Section: r.r.Section, Scope: types.ScopeTranscript, Raw: r.r.Raw,
			Req: &types.ReqNode{Type: types.NodeCredits, Designators: slices.Clone(pool.Designators), Courses: slices.Clone(pool.Courses), CrNcr: true, MaxAmount: maxAmount(0)}}}
		a.program = slices.Insert(a.program, i+1, k)
		return
	}
}

// ownUnitsLimit returns the limit of 0 on courses from other units: the
// designators of the program's allocations are its own, and the courses its
// text names with other designators still count. Nil when no allocation
// draws on a designator.
func (a *compAssembler) ownUnitsLimit() *types.ReqNode {
	var own []string
	for _, r := range a.allRows() {
		if r.r.Kind != types.RequirementAllocation {
			continue
		}
		walkReq(r.r.Req, func(n *types.ReqNode) {
			if n.Type == types.NodeCredits {
				own = append(own, n.Designators...)
			}
		})
	}
	if own = dedupe(own); own == nil {
		return nil
	}
	var named []string
	for _, code := range codeRe.FindAllString(a.raw, -1) {
		if !slices.Contains(own, code[:3]) {
			named = append(named, code)
		}
	}
	lim := &types.ReqNode{Type: types.NodeCredits, MaxAmount: maxAmount(0)}
	// Every course whose designator isn't the program's: no designator
	// source, the program's excluded.
	lim.ExcludeDesignators = own
	lim.Exclude = dedupe(named)
	return lim
}

// restatedPools joins a note restating a lead-filter pool
// (complAnyCombinationRe, a row of text at the program level) to the row of
// that pool, the allocation of the same amount whose raw names the
// designators ("2.0 APM/MAT credits at the 300+ level"), in place of a row
// of text (oneOffs).
func (a *compAssembler) restatedPools() {
	for _, r := range a.allRows() {
		n := r.r.Req
		if r.r.Kind != types.RequirementConstraint || n == nil || n.Type != types.NodeText || len(r.children) > 0 {
			continue
		}
		m := complAnyCombinationRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(n.Text), "."))
		if m == nil {
			continue
		}
		for _, o := range a.allRows() {
			p := o.r.Req
			if p != nil && p.Type == types.NodeOr {
				p = p.Items[0]
			}
			if o.r.Kind != types.RequirementAllocation || p == nil || p.Type != types.NodeCredits || p.Lists == nil ||
				fmtAmount(p.Amount) != m[3] || !strings.Contains(o.r.Raw, m[1]+"/"+m[2]+" credits at the 300+ level") {
				continue
			}
			if a.detach(r) {
				addRaw(o, r.r.Raw)
				o.r.Notes = append(o.r.Notes, r.r.Notes...)
			}
			break
		}
	}
}

// pairSubstitutes reads, after substitutes, an in-lieu alternative whose
// courses include a pair ("( MAT235H1, MAT236H1)/( MAT235Y1/ MAT237Y1)/
// ECO210H1 is strongly recommended for this program. Students taking one of
// these courses can have it count in lieu of 0.5 300-level course required
// for this program") on the pool at that level, as substitutes does for
// single courses: or(pool, and(the pool less 0.5, credits{0.5, lists:
// [the courses]})). The list holds each member at the credit it replaces:
// the pair is a pair item with that weight, a heavier course a credits
// item with it. It returns lists with the lists it made.
func (a *compAssembler) pairSubstitutes(lists []*types.CourseList) []*types.CourseList {
	for _, r := range a.allRows() {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr || r.r.Label == "" {
			continue
		}
		for i, it := range n.Items {
			if it.Type != types.NodeText {
				continue
			}
			m := complPairInLieuRe.FindStringSubmatch(complClean(it.Text))
			if m == nil {
				continue
			}
			pool := n.Items[0]
			level, _ := strconv.Atoi(m[3] + "00")
			if pool.Type != types.NodeCredits || pool.Designators == nil || pool.Lists != nil || !slices.Contains(pool.Levels, level) {
				continue
			}
			v, _ := strconv.ParseFloat(m[2], 64)
			items, ok := inLieuItems(m[1], v)
			if !ok || pool.Amount <= v {
				continue
			}
			name := "Requirement " + r.r.Label + " substitutes"
			if slices.ContainsFunc(lists, func(l *types.CourseList) bool { return l.Name == name }) {
				continue
			}
			lists = append(lists, &types.CourseList{Name: name, Items: items})
			less := cloneNode(pool)
			less.Amount -= v
			n.Items[i] = andNode(less, &types.ReqNode{Type: types.NodeCredits, Amount: v, Lists: []string{name}})
			addRaw(r, it.Text)
		}
	}
	return lists
}

// inLieuItems reads the members of an in-lieu sentence ("( MAT235H1,
// MAT236H1)/( MAT235Y1/ MAT237Y1)/ ECO210H1") as list items worth v each:
// a pair item, or credits items, with weight v where they weigh more. ok is
// false unless one member is a pair and every member weighs at least v.
func inLieuItems(s string, v float64) ([]*types.ReqNode, bool) {
	units, ok := splitUnitsSlash(s)
	if !ok {
		return nil, false
	}
	var items []*types.ReqNode
	pair := false
	single := func(codes []string) *types.ReqNode {
		x := &types.ReqNode{Type: types.NodeCredits, Courses: codes}
		if courseWeight(codes[0]) > v {
			x.Weight = v
		}
		return x
	}
	for _, u := range units {
		switch {
		case codeRe.FindString(u) == u:
			if courseWeight(u) < v {
				return nil, false
			}
			items = append(items, single([]string{u}))
		case complPairUnitRe.MatchString(u):
			x := complPairUnitRe.FindStringSubmatch(u)
			p := andNode(&types.ReqNode{Type: types.NodeCourse, Code: x[1]}, &types.ReqNode{Type: types.NodeCourse, Code: x[2]})
			if courseWeight(x[1])+courseWeight(x[2]) > v {
				p.Weight = v
			}
			items = append(items, p)
			pair = true
		case complAltUnitRe.MatchString(u):
			codes := codeRe.FindAllString(u, -1)
			if slices.ContainsFunc(codes, func(c string) bool { return courseWeight(c) != courseWeight(codes[0]) || courseWeight(c) < v }) {
				return nil, false
			}
			items = append(items, single(codes))
		default:
			return nil, false
		}
	}
	return items, pair
}

// splitUnitsSlash splits members at the "/" outside brackets.
func splitUnitsSlash(s string) ([]string, bool) {
	units, ok := splitUnits(strings.ReplaceAll(s, ",", "\x00"))
	for i := range units {
		units[i] = strings.ReplaceAll(units[i], "\x00", ",")
	}
	return units, ok
}
