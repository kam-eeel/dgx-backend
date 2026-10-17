package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Pools written with excluded designators, pairs counted as one unit, and a
// lead's filter joined to the courses it lists; and the rules built on them.

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

// designatorRules encodes the program rules written with excluded designators:
// the in-and-outside floors (complNonDesigRe) as a transcript constraint, and
// the other-units rule as a limit of 0.
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
			// the two amounts exceed the program's total, so the transcript
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

// transcriptCrNcr adds, after a CR/NCR limit of 0 that also names the
// designator's transcript constraint, the same limit over the transcript on
// pool's courses.
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

// ownUnitsLimit returns the limit of 0 on courses outside the designators the
// allocations draw on, except courses the text names. Nil when no allocation
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
	lim.ExcludeDesignators = own
	lim.Exclude = dedupe(named)
	return lim
}

// restatedPools moves a note restating a lead-filter pool from its own row to
// the allocation of that pool.
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

// pairSubstitutes reads, as substitutes does for single courses, an in-lieu
// alternative whose courses include a pair: or(pool, and(pool less v,
// credits{v, lists: [the courses]})). It returns lists with the lists it made.
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

// inLieuItems reads the members of an in-lieu sentence as list items each worth
// at most v. ok is false unless one member is a pair and every single course
// weighs at least v.
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
