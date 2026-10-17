package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Pools written inside an item, and the courses a sub-line adds to a pool.

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
	if fixed == nil || !coursesOnly(fixed) || !joinPool(pool, extra) {
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

// coursesOnly reports whether n is a course, or an and or or of such nodes.
func coursesOnly(n *types.ReqNode) bool {
	switch n.Type {
	case types.NodeCourse:
		return true
	case types.NodeAnd, types.NodeOr:
		return !slices.ContainsFunc(n.Items, func(it *types.ReqNode) bool { return !coursesOnly(it) })
	}
	return false
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
	complLabFloorRe = regexp.MustCompile(`(?i)^at\s+least\s+(\d+\.\d+)\s+credits?\s+must\s+be\s+an?\s+laboratory\s+course$`)
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
	if courses == nil || courses.Type != types.NodeOr || !coursesOnly(courses) || pool == nil || pool.Type != types.NodeCredits || len(pool.Courses) == 0 ||
		!slices.Equal(sortedCopy(nodeCodes(courses)), sortedCopy(append([]string{m[5]}, replaced...))) ||
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

func sortedCopy(xs []string) []string {
	out := slices.Clone(xs)
	slices.Sort(out)
	return out
}

var (
	// complPlusRe matches the "plus" between an item's parts.
	complPlusRe = regexp.MustCompile(`,?\s+plus\s+`)
	// complPlusPoolRe matches a pool part: "1.0 credit of ( PHL325H1/
	// PHL331H1)".
	complPlusPoolRe = regexp.MustCompile(`(?i)^(\d+\.\d+\s+credits?)\s+of\s+\(([^()]+)\)$`)
	// complPlusRestRe matches a designator's credits with a floor on a list:
	// "2.0 additional PHL credits, at least 0.5 credit of which must be from
	// Course Group 2 - Value Theory".
	complPlusRestRe = regexp.MustCompile(`^(\d+\.\d+)\s+additional\s+([A-Z]{3})\s+credits,\s+at\s+least\s+(\d+\.\d+)\s+credits?\s+of\s+which\s+must\s+be\s+from\s+(?:Course\s+)?(Group\s+\w+)(?:\s+-\s+[A-Z][\w ]*)?$`)
)

// plusParts splits an item of three parts joined by "plus" into a group of two
// allocations, since the parts share courses: the courses and pool, and the
// designator's credits with the list floor under it. Runs before namedLists
// marks the lists used.
func (a *compAssembler) plusParts(ls compListRefs) {
	for _, row := range a.allRows() {
		n := row.r.Req
		if row.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || n.Kind != types.TextOther || len(row.children) > 0 {
			continue
		}
		parts := complPlusRe.Split(strings.TrimSuffix(strings.TrimSpace(n.Text), "."), -1)
		if len(parts) != 3 {
			continue
		}
		fixed, _ := parseCompletionText(parts[0])
		pm := complPlusPoolRe.FindStringSubmatch(strings.TrimSpace(parts[1]))
		rm := complPlusRestRe.FindStringSubmatch(strings.TrimSpace(parts[2]))
		if fixed == nil || !coursesOnly(fixed) || pm == nil || rm == nil {
			continue
		}
		pool, _ := parseCompletionText(pm[1] + " from: " + pm[2])
		names := ls.resolve(listRefNames(rm[4]))
		if pool == nil || pool.Type != types.NodeCredits || len(pool.Courses) == 0 || len(names) != 1 || sharesCourses(fixed, pool) {
			continue
		}
		amount, _ := strconv.ParseFloat(rm[1], 64)
		floor, _ := strconv.ParseFloat(rm[3], 64)
		rest := &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Raw: row.r.Raw,
			Req: &types.ReqNode{Type: types.NodeCredits, Amount: amount, Designators: []string{rm[2]}}}}
		rest.children = []*compRow{{r: &types.Requirement{Kind: types.RequirementConstraint, Raw: strings.TrimSpace(parts[2]),
			Req: &types.ReqNode{Type: types.NodeCredits, Amount: floor, Lists: names}}}}
		row.r.Req = nil
		row.children = []*compRow{
			{r: &types.Requirement{Kind: types.RequirementAllocation, Raw: row.r.Raw, Req: andNode(fixed, pool)}},
			rest,
		}
	}
}
