package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Task 10 stream F1: pools whose groups of alternatives aren't pairs ("2.5
// credits from ( MAT130H1/ MAT135H1, MAT136H1, MAT246H1)/ [...], (
// MAT235H1, MAT236H1)/ ...", "1.0 credit from any combination of ...",
// ";" groups), a remainder pool to the program's total, and one-off pools
// over lists (ASMAJ0552, ASSPE1707, ASSPE1944, ASMAJ2660, ASMIN0235). They
// are read by groupPools, after the rows of text are read with the lists
// (namedLists).

var (
	// complGroupedPoolRe: an amount, or a range ("2.0 – 2.5", "2.0 to
	// 2.5"), from groups of alternatives written with brackets. The groups:
	// the amounts, the members.
	complGroupedPoolRe = regexp.MustCompile(`(?i)^(?:[^:\d][^:]*:\s*)?(\d+(?:\.\d+)?)(?:\s*(?:–|-|to)\s*(\d+(?:\.\d+)?))?\s+credits?\s+from\s+(.+[\])].*)$`)
	// complGroupedRecommendRe: a pool's trailing advice in brackets: ". [(
	// MAT148H1, MAT149H1)/ MAT137Y1/ ... recommended]".
	complGroupedRecommendRe = regexp.MustCompile(`(?i)\.?\s*\[([^\[\]]*\brecommended)\]$`)
	// complAnyCombinationOfRe: "BIO120H1, BIO130H1, ( CHM135H1, CHM136H1)/
	// CHM151Y1, and 1.0 credit from any combination of ( MAT130H1/ ...)/
	// ..., PHY131H1/ PHY151H1, PHY132H1/ PHY152H1 (see NOTE 1)". The
	// groups: the courses before, the amount, the members, the reference.
	complAnyCombinationOfRe = regexp.MustCompile(`(?i)^(?:[^:\d][^:]*:\s*)?(.+?),?\s+and\s+(\d+(?:\.\d+)?)\s+credits?\s+from\s+any\s+combination\s+of\s+(.+?)(?:\s*\((see\s+note\s+\d+)\))?$`)
	// complSemicolonGroupsRe: "2.0 credits chosen from BIO120H1; (
	// CHM135H1, CHM136H1); ...; PHY131H1, PHY132H1". The groups: the
	// amount, the members.
	complSemicolonGroupsRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+chosen\s+from\s+([^;]+(?:;[^;]+)+)$`)
	// complOpenCalcRe: a group whose bracket isn't closed, "MAT130H1/
	// MAT135H1, MAT136H1, ( MAT148H1, MAT149H1/ MAT137Y1": the calculus
	// options the same department writes "( MAT130H1/ MAT135H1, MAT136H1)/ (
	// MAT148H1, MAT149H1/ MAT137Y1)" (ASSPE1762, ASSPE2082). The groups:
	// the first option, the second.
	complOpenCalcRe = regexp.MustCompile(`^\s*(` + codePat + `\s*/\s*` + codePat + `\s*,\s*` + codePat + `)\s*,\s*(\(\s*` + codePat + `\s*,\s*` + codePat + `\s*/\s*` + codePat + `)\s*$`)
	// complRemainderListRe: "To bring the program total to 14.0 credits,
	// students may select from the following list: BIO220H1/ ...". The
	// groups: the total, the members.
	complRemainderListRe = regexp.MustCompile(`(?i)^to\s+bring\s+the\s+program\s+total\s+to\s+(\d+(?:\.\d+)?)\s+credits?,\s+students\s+may\s+select\s+from\s+the\s+following\s+list:\s*(.+)$`)
	// complOneOfMemberRe: a member "one of CSC108H1 or CSC148H1".
	complOneOfMemberRe = regexp.MustCompile(`(?i)^one\s+of\s+(` + codePat + `)\s+or\s+(` + codePat + `)$`)
	// complApprovedFilterMemberRe: "any suitable 300-level lecture-based
	// course from CHM/ CSB/ ... (departmental approval required prior to
	// enrolling)". The groups: the level, the designators, the approval.
	complApprovedFilterMemberRe = regexp.MustCompile(`(?i)^any\s+suitable\s+([1-4])00-level\s+[\w-]*\s*course\s+from\s+((?-i:[A-Z]{3})(?:\s*/\s*(?-i:[A-Z]{3}))*)\s*\(([^()]*approval[^()]*)\)$`)
	// complListsCapRe: "2.5 LAS credits or pre-approved courses from Lists
	// A and B, of which a maximum of 0.5 credit may be from List B". The
	// groups: the amount, the designator, the lists, the cap, the capped
	// list.
	complListsCapRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+((?-i:[A-Z]{3}))\s+credits\s+or\s+pre-approved\s+courses\s+from\s+lists\s+([A-Z])\s+and\s+([A-Z]),\s+of\s+which\s+a\s+maximum\s+of\s+(\d+(?:\.\d+)?)\s+credits?\s+may\s+be\s+from\s+list\s+([A-Z])$`)
	// complProgressionRe: "2.0 credits in language courses from Group C (a
	// progression in one language) or 2.0 credits in any major African
	// language approved by the Program Committee". The groups: the amount,
	// the list, the route.
	complProgressionRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits\s+in\s+language\s+courses\s+from\s+(group\s+[A-Z])\s+\(\s*a\s+progression\s+in\s+one\s+language\s*\)\s+or\s+(\d+(?:\.\d+)?\s+credits\s+in\s+any\s+[\w\s]+?\s+language\s+approved\s+by\s+the\s+[\w\s]+)$`)
	// complProgressionLineRe: a line of a list of languages, the last part
	// of its note: "Swahili: AFR280Y1, AFR380Y1".
	complProgressionLineRe = regexp.MustCompile(`([A-Z][A-Za-z ]*):\s*([^:]+)$`)
	// complFloorFromRe: "1.0 credit, including at least a 0.5 PHY credit at
	// the 400-level, from PHY 300-level courses/ PHY 400-level courses/
	// JPE395H1/ JPE493H1, excluding JPH311H1, JPH441H1". The groups: the
	// amount, the floor's amount, designator and level, the members, the
	// courses excluded.
	complFloorFromRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?,\s+including\s+at\s+least\s+a\s+(\d+(?:\.\d+)?)\s+((?-i:[A-Z]{3}))\s+credit\s+at\s+the\s+([1-4])00-level,\s+from\s+(.+?),\s+excluding\s+(` + codePat + `(?:\s*,\s*` + codePat + `)*)$`)
	// complFloorFromMemberRe: a member of complFloorFromRe, "PHY 300-level
	// courses".
	complFloorFromMemberRe = regexp.MustCompile(`^([A-Z]{3})\s+([1-4])00-level\s+courses$`)
	// complCountsAsPartRe: "PHY479Y1 (Undergraduate Research Project)
	// satisfies Requirement 3 in Third or Fourth Year and counts as a 0.5
	// credit at the PHY 400-level for Requirement 4 in Third or Fourth
	// Year". The groups: the course, the weight, the designator, the level.
	complCountsAsPartRe = regexp.MustCompile(`(?i)^(` + codePat + `)\s*(?:\([^()]*\))?\s+satisfies\s+requirement\s+.+?\s+and\s+counts\s+as\s+a\s+(\d+(?:\.\d+)?)\s+credit\s+at\s+the\s+((?-i:[A-Z]{3}))\s+([1-4])00-level\s+for\s+requirement\s+.+$`)
	// complFurtherCodedRe: "0.5 credit from further MUN coded100 level
	// courses or POL101H1/ ... or another First Year Foundation/One course
	// relevant to the study of Public Policy". The groups: the amount, the
	// designator, the level, the courses, the course the content decides.
	complFurtherCodedRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+from\s+further\s+((?-i:[A-Z]{3}))\s+coded\s*([1-4])00\s+level\s+courses\s+or\s+(` + codePat + `(?:\s*/\s*` + codePat + `)*)\s+or\s+(another\s+.+?\s+relevant\s+to\s+.+)$`)
	// complTwoGroupsExceptRe: "0.5 credit from Group A and 0.5 credit from
	// Group B (excluding 100-level courses)". The groups: the amounts and
	// lists, the level excluded.
	complTwoGroupsExceptRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+from\s+(group\s+[A-Z])\s+and\s+(\d+(?:\.\d+)?)\s+credits?\s+from\s+(group\s+[A-Z])\s+\(\s*excluding\s+([1-4])00-level\s+courses\s*\)$`)
	// complGroupsOrTitledRe: "1.0 credit from Group A or B (excluding
	// 100-level courses), or at any level in an Asian language (see
	// Accepted Language Courses below)". The groups: the amount, the lists,
	// the level excluded, the titled list.
	complGroupsOrTitledRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+from\s+(group\s+[A-Z]\s+or\s+[A-Z])\s+\(\s*excluding\s+([1-4])00-level\s+courses\s*\),\s+or\s+at\s+any\s+level\s+in\s+an?\s+[\w\s]+?\s+\(\s*see\s+(.+?)\s+below\s*\)$`)
)

// groupPools reads, in namedLists with the lists known, the rows of text
// the readers below encode. A remainder pool joins Req 1 and leaves the
// rows.
func (a *compAssembler) groupPools(ls compListRefs) {
	var walk func(rows []*compRow) []*compRow
	walk = func(rows []*compRow) []*compRow {
		var out []*compRow
		for _, r := range rows {
			r.children = walk(r.children)
			if a.groupRow(r, ls) {
				continue // Req 1's own pool
			}
			out = append(out, r)
		}
		return out
	}
	a.items = walk(a.items)
	a.program = walk(a.program)
}

// groupRow reads one allocation of text (its body, or the first item of
// or(text, routes)). It reports whether the row joined Req 1.
func (a *compAssembler) groupRow(r *compRow, ls compListRefs) bool {
	n := r.r.Req
	if r.r.Kind != types.RequirementAllocation || n == nil {
		return false
	}
	if n.Type == types.NodeOr && len(n.Items) == 2 && n.Items[0].Type == types.NodeText && n.Items[1].Type == types.NodeText {
		if body, ok := a.countsAsPart(r, n.Items[0].Text, n.Items[1].Text, ls); ok {
			r.r.Req = body
		}
		return false
	}
	text := n
	if n.Type == types.NodeOr && len(n.Items) > 1 && !slices.ContainsFunc(n.Items[1:], func(it *types.ReqNode) bool {
		return it.Type != types.NodeText || it.Kind != types.TextPermission
	}) {
		text = n.Items[0]
	}
	if text.Type != types.NodeText || text.Kind != types.TextOther {
		return false
	}
	s := strings.TrimSuffix(complClean(text.Text), ".")
	if a.remainderPool(r, s, ls) {
		return true
	}
	var body *types.ReqNode
	var notes []string
	ok := false
	for _, read := range []func() (*types.ReqNode, []string, bool){
		func() (*types.ReqNode, []string, bool) { return groupedPool(s) },
		func() (*types.ReqNode, []string, bool) { return anyCombination(s) },
		func() (*types.ReqNode, []string, bool) { return semicolonGroups(s) },
		func() (*types.ReqNode, []string, bool) { return a.listsCap(r, s, ls) },
		func() (*types.ReqNode, []string, bool) { return progression(s, ls) },
		func() (*types.ReqNode, []string, bool) { return furtherCoded(s) },
		func() (*types.ReqNode, []string, bool) { return twoGroupsExcept(s, ls) },
		func() (*types.ReqNode, []string, bool) { return a.groupsOrTitled(r, s, ls) },
	} {
		if body, notes, ok = read(); ok {
			break
		}
	}
	if !ok {
		return false
	}
	if text == n {
		r.r.Req = body
	} else {
		n.Items[0] = body
	}
	r.r.Notes = dedupe(append(r.r.Notes, notes...))
	return false
}

// groupedPool reads complGroupedPoolRe: the members are the and of their
// groups, each the or of its alternatives, as the grammar reads them
// without the amount ("( MAT130H1/ MAT135H1, MAT136H1, MAT246H1)": the and
// of an or and two courses). The groups are what the text requires, so the
// stated amount is only checked: the fewest credits that meet the groups
// must be within it.
func groupedPool(s string) (*types.ReqNode, []string, bool) {
	var notes []string
	if m := complGroupedRecommendRe.FindStringSubmatch(s); m != nil {
		notes = append(notes, strings.TrimSpace(m[1]))
		s = strings.TrimSpace(s[:len(s)-len(m[0])])
	}
	m := complGroupedPoolRe.FindStringSubmatch(s)
	if m == nil || strings.Contains(strings.ToLower(m[3]), "any combination") || !strings.ContainsAny(m[3], "([") {
		return nil, nil, false
	}
	lo, _ := strconv.ParseFloat(m[1], 64)
	hi := lo
	if m[2] != "" {
		hi, _ = strconv.ParseFloat(m[2], 64)
	}
	n, more := parseCompletionText(m[3])
	if !closedGroups(n) || n.Type != types.NodeAnd || len(n.Items) < 2 {
		return nil, nil, false
	}
	if w := minWeight(n); w < lo-1e-9 || w > hi+1e-9 {
		return nil, nil, false
	}
	return n, append(notes, more...), true
}

// closedGroups reports whether n holds only courses, pools and their
// and/or: no text.
func closedGroups(n *types.ReqNode) bool {
	if n == nil {
		return false
	}
	ok := true
	walkReq(n, func(x *types.ReqNode) {
		switch x.Type {
		case types.NodeAnd, types.NodeOr, types.NodeCourse:
		case types.NodeCredits:
			ok = ok && x.Items == nil && x.Amount > 0
		default:
			ok = false
		}
	})
	return ok
}

// unitChoice is the strict reading of an amount from groups that each
// count whole: the or of the smallest sets of groups whose fewest credits
// make the amount. ok is false when no set does.
func unitChoice(groups []*types.ReqNode, amount float64) (*types.ReqNode, bool) {
	if len(groups) > 8 {
		return nil, false
	}
	ws := make([]float64, len(groups))
	for i, g := range groups {
		ws[i] = minWeight(g)
	}
	var alts []*types.ReqNode
	for set := 1; set < 1<<len(groups); set++ {
		sum := 0.0
		for i := range groups {
			if set&(1<<i) != 0 {
				sum += ws[i]
			}
		}
		if sum < amount-1e-9 {
			continue
		}
		minimal := true
		for i := range groups {
			if set&(1<<i) != 0 && sum-ws[i] >= amount-1e-9 {
				minimal = false
			}
		}
		if !minimal {
			continue
		}
		var parts []*types.ReqNode
		for i, g := range groups {
			if set&(1<<i) != 0 {
				parts = append(parts, cloneNode(g))
			}
		}
		alts = append(alts, andNode(parts...))
	}
	if alts == nil {
		return nil, false
	}
	return orNode(alts...), true
}

// lenientUnits is the lenient reading of an amount from groups of courses
// (anyCombination, semicolonGroups): any of their courses, whole groups or
// not, beside the sentence (ambiguity).
func lenientUnits(amount float64, groups []*types.ReqNode, s string) *types.ReqNode {
	return andNode(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: dedupe(nodeCodes(andNode(groups...)))}, textNode(types.TextAmbiguity, s))
}

// anyCombination reads complAnyCombinationOfRe: the courses before are
// required; the amount comes from the groups (the members' "," parts),
// each counted whole (unitChoice: the strict reading), or from any of
// their courses (the lenient reading, Review), which "any combination" may
// mean.
func anyCombination(s string) (*types.ReqNode, []string, bool) {
	m := complAnyCombinationOfRe.FindStringSubmatch(s)
	if m == nil {
		return nil, nil, false
	}
	lead, more := parseCompletionText(m[1])
	pool, more2 := parseCompletionText(m[3])
	if !closedGroups(lead) || !closedGroups(pool) || hasPool(lead) || hasPool(pool) {
		return nil, nil, false
	}
	groups := []*types.ReqNode{pool}
	if pool.Type == types.NodeAnd {
		groups = pool.Items
	}
	amount, _ := strconv.ParseFloat(m[2], 64)
	strict, ok := unitChoice(groups, amount)
	if !ok || slices.ContainsFunc(nodeCodes(lead), func(c string) bool { return slices.Contains(nodeCodes(pool), c) }) {
		return nil, nil, false
	}
	notes := append(more, more2...)
	if m[4] != "" {
		notes = append(notes, m[4])
	}
	body := andNode(lead, orNode(strict, lenientUnits(amount, groups, cleanNote(m[2]+" credit from any combination of "+m[3]))))
	return body, notes, true
}

// hasPool reports whether n holds a credits node.
func hasPool(n *types.ReqNode) bool {
	found := false
	walkReq(n, func(x *types.ReqNode) { found = found || x.Type == types.NodeCredits })
	return found
}

// semicolonGroups reads complSemicolonGroupsRe: groups split by ";", each
// counted whole (unitChoice) or, the lenient reading, any of their
// courses. A group whose bracket isn't closed (complOpenCalcRe) is read as
// the department writes it closed.
func semicolonGroups(s string) (*types.ReqNode, []string, bool) {
	m := complSemicolonGroupsRe.FindStringSubmatch(s)
	if m == nil {
		return nil, nil, false
	}
	var groups []*types.ReqNode
	var notes []string
	for _, g := range strings.Split(m[2], ";") {
		if c := complOpenCalcRe.FindStringSubmatch(g); c != nil {
			g = "( " + c[1] + ")/ " + c[2] + ")"
		}
		if strings.Count(g, "(") != strings.Count(g, ")") {
			return nil, nil, false
		}
		n, more := parseCompletionText(strings.TrimSpace(g))
		if !closedGroups(n) || hasPool(n) {
			return nil, nil, false
		}
		groups = append(groups, n)
		notes = append(notes, more...)
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	strict, ok := unitChoice(groups, amount)
	if !ok || len(groups) < 2 {
		return nil, nil, false
	}
	return orNode(strict, lenientUnits(amount, groups, s)), notes, true
}

// remainderPool reads complRemainderListRe, the courses that bring the
// program to its total, as Req 1's own pool (as additionalUnion does): a
// list of the row's own (addUnion) of its members, "( MAT235H1, MAT236H1)"
// a pair item, "one of CSC108H1 or CSC148H1" both courses and a limit of one
// under Req 1, and a filter needing approval a maybe item. The row leaves
// the rows; its raw joins Req 1's.
func (a *compAssembler) remainderPool(r *compRow, s string, ls compListRefs) bool {
	m := complRemainderListRe.FindStringSubmatch(s)
	if m == nil || a.total == nil || len(r.children) > 0 || !slices.Contains(a.items, r) {
		return false
	}
	pool := a.total.r.Req
	v, _ := strconv.ParseFloat(m[1], 64)
	if pool == nil || pool.Type != types.NodeCredits || pool.Amount != v || pool.Designators != nil || pool.Levels != nil || pool.Courses != nil || pool.Lists != nil {
		return false
	}
	var singles []string
	var items, caps []*types.ReqNode
	members := splitTop(m[2], '/')
	if i := strings.Index(strings.ToLower(m[2]), "/ any suitable "); i >= 0 {
		// The filter's designators are written with "/" too.
		members = append(splitTop(m[2][:i], '/'), m[2][i+2:])
	}
	for _, u := range members {
		u = strings.TrimSpace(u)
		switch {
		case codeRe.FindString(u) == u:
			singles = append(singles, u)
		case complPairUnitRe.MatchString(u):
			x := complPairUnitRe.FindStringSubmatch(u)
			items = append(items, andNode(&types.ReqNode{Type: types.NodeCourse, Code: x[1]}, &types.ReqNode{Type: types.NodeCourse, Code: x[2]}))
		case complOneOfMemberRe.MatchString(u):
			x := complOneOfMemberRe.FindStringSubmatch(u)
			if courseWeight(x[1]) != 0.5 || courseWeight(x[2]) != 0.5 {
				return false
			}
			singles = append(singles, x[1], x[2])
			caps = append(caps, &types.ReqNode{Type: types.NodeCredits, Courses: []string{x[1], x[2]}, MaxAmount: maxAmount(0.5)})
		case complApprovedFilterMemberRe.MatchString(u):
			x := complApprovedFilterMemberRe.FindStringSubmatch(u)
			var ds []string
			for _, d := range strings.Split(x[2], "/") {
				ds = append(ds, strings.TrimSpace(d))
			}
			f := &types.ReqNode{Type: types.NodeCredits, Designators: ds, Levels: []int{int(x[1][0]-'0') * 100}}
			items = append(items, andNode(f, textNode(types.TextPermission, u)))
		default:
			return false
		}
	}
	if singles != nil {
		items = append([]*types.ReqNode{{Type: types.NodeCredits, Courses: dedupe(singles)}}, items...)
	}
	name, _, ok := a.groupUnion(r, items, ls)
	if !ok {
		return false
	}
	pool.Lists = []string{name}
	for _, c := range caps {
		a.total.children = append(a.total.children, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Req: c, Raw: r.r.Raw}})
	}
	a.total.r.Raw += "\n" + r.r.Raw
	a.total.r.Notes = append(a.total.r.Notes, r.r.Notes...)
	return true
}

// groupUnion is addUnion, after dropping the list of the row's base name a
// reader before this pass made and left unused (its text stayed), so the
// row's list takes that name, not "... (2)".
func (a *compAssembler) groupUnion(r *compRow, items []*types.ReqNode, ls compListRefs) (string, *compList, bool) {
	if r.r.Label != "" {
		base := "Requirement " + r.r.Label
		if !strings.HasSuffix(strings.ToLower(base), " courses") {
			base += " courses"
		}
		used := false
		var walk func(rows []*compRow)
		walk = func(rows []*compRow) {
			for _, x := range rows {
				walkReq(x.r.Req, func(n *types.ReqNode) { used = used || slices.Contains(n.Lists, base) })
				walk(x.children)
			}
		}
		walk(a.items)
		walk(a.program)
		for _, l := range ls {
			for _, it := range l.list.Items {
				used = used || slices.Contains(it.Lists, base)
			}
		}
		if l := ls[complUnionKey(base)]; l != nil && !used && ls[complUnionKey(base+" (2)")] == nil {
			delete(ls, l.key)
			a.unions = slices.DeleteFunc(a.unions, func(x *compList) bool { return x == l })
		}
	}
	return a.addUnion(r, items, ls)
}

// listsCap reads complListsCapRe: a list of the row's own (addUnion), the
// designator and the two lists, with the cap a limit under the row.
func (a *compAssembler) listsCap(r *compRow, s string, ls compListRefs) (*types.ReqNode, []string, bool) {
	m := complListsCapRe.FindStringSubmatch(s)
	if m == nil {
		return nil, nil, false
	}
	names := ls.resolve(append(listRefNames("List "+m[3]), listRefNames("List "+m[4])...))
	capped := ls.resolve(listRefNames("List " + m[6]))
	if len(names) != 2 || len(capped) != 1 || !slices.Contains(names, capped[0]) {
		return nil, nil, false
	}
	name, _, ok := a.groupUnion(r, []*types.ReqNode{
		{Type: types.NodeCredits, Designators: []string{m[2]}},
		{Type: types.NodeCredits, Lists: names},
	}, ls)
	if !ok {
		return nil, nil, false
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	c, _ := strconv.ParseFloat(m[5], 64)
	limit := &types.ReqNode{Type: types.NodeCredits, Lists: capped, MaxAmount: maxAmount(c)}
	a.rowUnder(r, &types.Requirement{Kind: types.RequirementLimit, Req: limit, Raw: r.r.Raw})
	return &types.ReqNode{Type: types.NodeCredits, Amount: v, Lists: []string{name}}, nil, true
}

// progression reads complProgressionRe: the amount from one language's
// courses of the list (its lines "Swahili: AFR280Y1, AFR380Y1" and the
// sub-lists under it, from their notes), or the route the committee
// approves.
func progression(s string, ls compListRefs) (*types.ReqNode, []string, bool) {
	m := complProgressionRe.FindStringSubmatch(s)
	if m == nil {
		return nil, nil, false
	}
	keys := listRefNames(m[2])
	if len(keys) != 1 || ls[keys[0]] == nil {
		return nil, nil, false
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	var alts []*types.ReqNode
	seen := map[string]bool{}
	for _, l := range ls {
		if l.key != keys[0] && l.parent != keys[0] {
			continue
		}
		for _, note := range l.notes {
			x := complProgressionLineRe.FindStringSubmatch(note)
			if x == nil {
				return nil, nil, false
			}
			codes := codeRe.FindAllString(x[2], -1)
			total := 0.0
			for _, c := range codes {
				if seen[c] {
					return nil, nil, false
				}
				seen[c] = true
				total += courseWeight(c)
			}
			if total < v-1e-9 {
				return nil, nil, false
			}
			alts = append(alts, &types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: codes})
		}
	}
	if len(alts) < 2 {
		return nil, nil, false
	}
	slices.SortStableFunc(alts, func(x, y *types.ReqNode) int { return strings.Compare(x.Courses[0], y.Courses[0]) })
	return orNode(append(alts, textNode(types.TextPermission, m[3]))...), nil, true
}

// countsAsPart reads, on row r, complFloorFromRe with its alternative
// complCountsAsPartRe: a list of the row's own (addUnion) of the members,
// the course that counts in part as a weighted item (and excluded from
// the filter that would count it whole), the exclusions on the pool, and
// the floor a constraint under the row.
func (a *compAssembler) countsAsPart(r *compRow, s, alt string, ls compListRefs) (*types.ReqNode, bool) {
	m := complFloorFromRe.FindStringSubmatch(strings.TrimSuffix(complClean(s), "."))
	w := complCountsAsPartRe.FindStringSubmatch(strings.TrimSuffix(complClean(alt), "."))
	if m == nil || w == nil || w[3] != m[3] || w[4] != m[4] {
		return nil, false
	}
	part := w[1]
	var codes []string
	var items []*types.ReqNode
	for _, u := range strings.Split(m[5], "/") {
		u = strings.TrimSpace(u)
		if codeRe.FindString(u) == u {
			codes = append(codes, u)
			continue
		}
		f := complFloorFromMemberRe.FindStringSubmatch(u)
		if f == nil {
			return nil, false
		}
		lv := int(f[2][0]-'0') * 100
		if i := slices.IndexFunc(items, func(it *types.ReqNode) bool { return it.Designators[0] == f[1] }); i >= 0 {
			items[i].Levels = append(items[i].Levels, lv)
			continue
		}
		items = append(items, &types.ReqNode{Type: types.NodeCredits, Designators: []string{f[1]}, Levels: []int{lv}})
	}
	if items == nil || slices.Contains(codes, part) || part[:3] != m[3] || courseLevel(part) != int(m[4][0]-'0')*100 {
		return nil, false
	}
	for _, it := range items {
		if it.Designators[0] == part[:3] && slices.Contains(it.Levels, courseLevel(part)) {
			it.Exclude = []string{part}
		}
	}
	weight, _ := strconv.ParseFloat(w[2], 64)
	if codes != nil {
		items = append(items, &types.ReqNode{Type: types.NodeCredits, Courses: codes})
	}
	items = append(items, &types.ReqNode{Type: types.NodeCredits, Courses: []string{part}, Weight: weight})
	name, _, ok := a.groupUnion(r, items, ls)
	if !ok {
		return nil, false
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	f, _ := strconv.ParseFloat(m[2], 64)
	a.rowUnder(r, &types.Requirement{Kind: types.RequirementConstraint, Req: &types.ReqNode{Type: types.NodeCredits, Amount: f, Designators: []string{m[3]}, Levels: []int{int(m[4][0]-'0') * 100}}, Raw: r.r.Raw})
	return &types.ReqNode{Type: types.NodeCredits, Amount: v, Lists: []string{name}, Exclude: codeRe.FindAllString(m[6], -1)}, true
}

// furtherCoded reads complFurtherCodedRe: one pool of the designator and
// the courses, at the level all of them are at, or a course whose content
// decides.
func furtherCoded(s string) (*types.ReqNode, []string, bool) {
	m := complFurtherCodedRe.FindStringSubmatch(s)
	if m == nil {
		return nil, nil, false
	}
	lv := int(m[3][0]-'0') * 100
	codes := codeRe.FindAllString(m[4], -1)
	if slices.ContainsFunc(codes, func(c string) bool { return courseLevel(c) != lv }) {
		return nil, nil, false
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	pool := &types.ReqNode{Type: types.NodeCredits, Amount: v, Designators: []string{m[2]}, Courses: codes, Levels: []int{lv}}
	return orNode(pool, textNode(types.TextContent, m[5])), nil, true
}

// levelsFrom are the levels from l up to 400.
func levelsFrom(l int) []int {
	var out []int
	for v := l; v <= 400; v += 100 {
		out = append(out, v)
	}
	return out
}

// listCourses returns the courses listed by code on the list key names.
func (ls compListRefs) listCourses(key string) []string {
	l := ls[key]
	if l == nil {
		return nil
	}
	var cs []string
	for _, it := range l.list.Items {
		cs = append(cs, it.Courses...)
	}
	return cs
}

// twoGroupsExcept reads complTwoGroupsExceptRe. Whether the bracket
// excludes the level from both lists or only the second is the text's
// ambiguity: both is the strict reading; only the second, Review beside
// it. The lists share courses, and a row's nodes may count one course
// twice, so the strict reading also needs the two amounts from the two
// lists together; it is read only when no full-year course is on both.
func twoGroupsExcept(s string, ls compListRefs) (*types.ReqNode, []string, bool) {
	m := complTwoGroupsExceptRe.FindStringSubmatch(s)
	if m == nil {
		return nil, nil, false
	}
	ka, kb := listRefNames(m[2]), listRefNames(m[4])
	na, nb := ls.resolve(ka), ls.resolve(kb)
	if len(na) != 1 || len(nb) != 1 || na[0] == nb[0] {
		return nil, nil, false
	}
	ca, cb := ls.listCourses(ka[0]), ls.listCourses(kb[0])
	if len(ca) != len(ls[ka[0]].list.Items[0].Courses) || len(ls[ka[0]].list.Items) != 1 || len(ls[kb[0]].list.Items) != 1 {
		return nil, nil, false // lists of courses only
	}
	if slices.ContainsFunc(ca, func(c string) bool { return courseWeight(c) > 0.5 && slices.Contains(cb, c) }) {
		return nil, nil, false
	}
	va, _ := strconv.ParseFloat(m[1], 64)
	vb, _ := strconv.ParseFloat(m[3], 64)
	lv := levelsFrom(int(m[5][0]-'0')*100 + 100)
	strict := andNode(
		&types.ReqNode{Type: types.NodeCredits, Amount: va + vb, Lists: []string{na[0], nb[0]}, Levels: lv},
		&types.ReqNode{Type: types.NodeCredits, Amount: va, Lists: na, Levels: lv},
		&types.ReqNode{Type: types.NodeCredits, Amount: vb, Lists: nb, Levels: slices.Clone(lv)},
	)
	strict.Items[0].Levels = slices.Clone(lv)
	lenient := andNode(
		&types.ReqNode{Type: types.NodeCredits, Amount: va, Lists: slices.Clone(na)},
		&types.ReqNode{Type: types.NodeCredits, Amount: vb, Lists: slices.Clone(nb), Levels: slices.Clone(lv)},
		textNode(types.TextAmbiguity, s),
	)
	return orNode(strict, lenient), nil, true
}

// groupsOrTitled reads complGroupsOrTitledRe: the amount from the lists
// (above the level), or from the titled list at any level (the strict
// reading: one or the other); from both together (a list of the row's own,
// addUnion) is the lenient reading, Review.
func (a *compAssembler) groupsOrTitled(r *compRow, s string, ls compListRefs) (*types.ReqNode, []string, bool) {
	m := complGroupsOrTitledRe.FindStringSubmatch(s)
	if m == nil {
		return nil, nil, false
	}
	refs := strings.SplitN(m[2], " or ", 2)
	names := ls.resolve(append(listRefNames(strings.TrimSpace(refs[0])), listRefNames("Group "+strings.TrimSpace(refs[1]))...))
	titled := ls[complTitleKey(strings.TrimSpace(m[4]))]
	if len(names) != 2 || titled == nil || titled.bad {
		return nil, nil, false
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	lv := levelsFrom(int(m[3][0]-'0')*100 + 100)
	groups := &types.ReqNode{Type: types.NodeCredits, Lists: names, Levels: lv}
	lang := &types.ReqNode{Type: types.NodeCredits, Lists: []string{titled.list.Name}}
	name, _, ok := a.groupUnion(r, []*types.ReqNode{cloneNode(groups), cloneNode(lang)}, ls)
	if !ok {
		return nil, nil, false
	}
	g, l := cloneNode(groups), cloneNode(lang)
	g.Amount, l.Amount = v, v
	return orNode(g, l, andNode(&types.ReqNode{Type: types.NodeCredits, Amount: v, Lists: []string{name}}, textNode(types.TextAmbiguity, s))), nil, true
}
