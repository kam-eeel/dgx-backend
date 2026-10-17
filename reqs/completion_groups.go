package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Groups of alternatives: pools of groups that aren't pairs, items split into
// groups, unions of groups and a remainder pool to the program's total.

var (
	// complGroupedPoolRe matches an amount or range from bracketed groups: "2.5
	// credits from ( MAT130H1/ MAT135H1, MAT136H1)/ ...".
	complGroupedPoolRe = regexp.MustCompile(`(?i)^(?:[^:\d][^:]*:\s*)?` + amountPat + `(?:\s*(?:–|-|to)\s*` + amountPat + `)?\s+credits?\s+from\s+(.+[\])].*)$`)
	// complGroupedRecommendRe matches a pool's closing advice: ". [( MAT148H1,
	// MAT149H1)/ MAT137Y1 recommended]".
	complGroupedRecommendRe = regexp.MustCompile(`(?i)\.?\s*\[([^\[\]]*\brecommended)\]$`)
	// complAnyCombinationOfRe matches courses and a pool from groups: "BIO120H1,
	// and 1.0 credit from any combination of ( MAT130H1/ ...)/ ...".
	complAnyCombinationOfRe = regexp.MustCompile(`(?i)^(?:[^:\d][^:]*:\s*)?(.+?),?\s+and\s+` + amountPat + `\s+credits?\s+from\s+any\s+combination\s+of\s+(.+?)(?:\s*\((see\s+note\s+\d+)\))?$`)
	// complSemicolonGroupsRe matches groups split by ";": "2.0 credits chosen from
	// BIO120H1; ( CHM135H1, CHM136H1); PHY131H1, PHY132H1".
	complSemicolonGroupsRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+chosen\s+from\s+([^;]+(?:;[^;]+)+)$`)
	// complOpenCalcRe matches a group whose bracket isn't closed: "MAT130H1/
	// MAT135H1, MAT136H1, ( MAT148H1, MAT149H1/ MAT137Y1".
	complOpenCalcRe = regexp.MustCompile(`^\s*(` + codePat + `\s*/\s*` + codePat + `\s*,\s*` + codePat + `)\s*,\s*(\(\s*` + codePat + `\s*,\s*` + codePat + `\s*/\s*` + codePat + `)\s*$`)
	// complRemainderListRe matches a remainder pool: "To bring the program total
	// to 14.0 credits, students may select from the following list: BIO220H1".
	complRemainderListRe = regexp.MustCompile(`(?i)^to\s+bring\s+the\s+program\s+total\s+to\s+` + amountPat + `\s+credits?,\s+students\s+may\s+select\s+from\s+the\s+following\s+list:\s*(.+)$`)
	// complOneOfMemberRe matches a member "one of CSC108H1 or CSC148H1".
	complOneOfMemberRe = regexp.MustCompile(`(?i)^one\s+of\s+(` + codePat + `)\s+or\s+(` + codePat + `)$`)
	// complApprovedFilterMemberRe matches a filter needing approval: "any
	// suitable 300-level lecture-based course from CHM/ CSB (departmental approval
	// required)".
	complApprovedFilterMemberRe = regexp.MustCompile(`(?i)^any\s+suitable\s+([1-4])00-level\s+[\w-]*\s*course\s+from\s+(` + desigPat + `(?:\s*/\s*` + desigPat + `)*)\s*\(([^()]*approval[^()]*)\)$`)
	// complListsCapRe matches "2.5 LAS credits or pre-approved courses from Lists
	// A and B, of which a maximum of 0.5 credit may be from List B".
	complListsCapRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+(` + desigPat + `)\s+credits\s+or\s+pre-approved\s+courses\s+from\s+lists\s+([A-Z])\s+and\s+([A-Z]),\s+of\s+which\s+a\s+maximum\s+of\s+` + amountPat + `\s+credits?\s+may\s+be\s+from\s+list\s+([A-Z])$`)
	// complProgressionRe matches "2.0 credits in language courses from Group C (a
	// progression in one language) or 2.0 credits in any major African language
	// approved by the Program Committee".
	complProgressionRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits\s+in\s+language\s+courses\s+from\s+(group\s+[A-Z])\s+\(\s*a\s+progression\s+in\s+one\s+language\s*\)\s+or\s+(` + numPat + `\s+credits\s+in\s+any\s+[\w\s]+?\s+language\s+approved\s+by\s+the\s+[\w\s]+)$`)
	// complProgressionLineRe matches a language's line in a list's notes:
	// "Swahili: AFR280Y1, AFR380Y1".
	complProgressionLineRe = regexp.MustCompile(`([A-Z][A-Za-z ]*):\s*([^:]+)$`)
	// complFloorFromRe matches a pool with a floor and exclusions: "1.0 credit,
	// including at least a 0.5 PHY credit at the 400-level, from PHY 300-level
	// courses/ JPE395H1, excluding JPH311H1".
	complFloorFromRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?,\s+including\s+` + atLeastPat + `\s+a\s+` + amountPat + `\s+(` + desigPat + `)\s+credit\s+at\s+the\s+([1-4])00-level,\s+from\s+(.+?),\s+excluding\s+(` + codePat + `(?:\s*,\s*` + codePat + `)*)$`)
	// complFloorFromMemberRe matches a filter member of complFloorFromRe: "PHY
	// 300-level courses".
	complFloorFromMemberRe = regexp.MustCompile(`^([A-Z]{3})\s+([1-4])00-level\s+courses$`)
	// complCountsAsPartRe matches "PHY479Y1 satisfies Requirement 3 ... and counts
	// as a 0.5 credit at the PHY 400-level for Requirement 4 ...".
	complCountsAsPartRe = regexp.MustCompile(`(?i)^(` + codePat + `)\s*(?:\([^()]*\))?\s+satisfies\s+requirement\s+.+?\s+and\s+counts\s+as\s+a\s+` + amountPat + `\s+credit\s+at\s+the\s+(` + desigPat + `)\s+([1-4])00-level\s+for\s+requirement\s+.+$`)
	// complFurtherCodedRe matches "0.5 credit from further MUN coded100 level
	// courses or POL101H1 or another ... course relevant to the study of ...".
	complFurtherCodedRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+from\s+further\s+(` + desigPat + `)\s+coded\s*([1-4])00\s+level\s+courses\s+or\s+(` + codePat + `(?:\s*/\s*` + codePat + `)*)\s+or\s+(another\s+.+?\s+relevant\s+to\s+.+)$`)
	// complTwoGroupsExceptRe matches "0.5 credit from Group A and 0.5 credit from
	// Group B (excluding 100-level courses)".
	complTwoGroupsExceptRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+from\s+(group\s+[A-Z])\s+and\s+` + amountPat + `\s+credits?\s+from\s+(group\s+[A-Z])\s+\(\s*excluding\s+([1-4])00-level\s+courses\s*\)$`)
	// complGroupsOrTitledRe matches "1.0 credit from Group A or B (excluding
	// 100-level courses), or at any level in an Asian language (see Accepted
	// Language Courses below)".
	complGroupsOrTitledRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+from\s+(group\s+[A-Z]\s+or\s+[A-Z])\s+\(\s*excluding\s+([1-4])00-level\s+courses\s*\),\s+or\s+at\s+any\s+level\s+in\s+an?\s+[\w\s]+?\s+\(\s*see\s+(.+?)\s+below\s*\)$`)
)

// groupPools reads the rows of text the readers below encode; it runs in
// namedLists, with the lists known. A remainder pool joins Req 1 and leaves
// the rows.
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

// groupRow reads one allocation of text: its body, or the first item of
// or(text, permissions). It reports whether the row joined Req 1.
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

// groupedPool reads complGroupedPoolRe as the and of the groups, each the or
// of its alternatives. The groups are what the text requires, so the amount
// is only checked: the groups' fewest credits must be within it.
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

// closedGroups reports whether n holds only courses, pools and their and/or.
func closedGroups(n *types.ReqNode) bool {
	if n == nil {
		return false
	}
	return structureOnly(n, func(x *types.ReqNode) bool {
		return x.Type == types.NodeCredits && x.Items == nil && x.Amount > 0
	})
}

// unitChoice is the strict reading of an amount from groups that each count
// whole: the or of the minimal sets of groups that make it.
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

// lenientUnits is the lenient reading of an amount from groups: any of their
// courses, whole groups or not, beside the sentence as an ambiguity.
func lenientUnits(amount float64, groups []*types.ReqNode, s string) *types.ReqNode {
	return andNode(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: dedupe(nodeCodes(andNode(groups...)))}, textNode(types.TextAmbiguity, s))
}

// anyCombination reads complAnyCombinationOfRe: the courses before are
// required; the amount comes from whole groups (strict), or any of their
// courses (lenient, Review), as "any combination" may mean.
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

func hasPool(n *types.ReqNode) bool {
	found := false
	walkReq(n, func(x *types.ReqNode) { found = found || x.Type == types.NodeCredits })
	return found
}

// semicolonGroups reads complSemicolonGroupsRe: whole groups (strict) or any
// of their courses (lenient). An unclosed group is read as the department
// writes it closed: "( MAT130H1/ MAT135H1, MAT136H1)/ ( MAT148H1, ...)".
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

// remainderPool reads complRemainderListRe as Req 1's own pool: a union list
// of its members, with each "one of" member capped at one course under Req 1.
// The row leaves the rows; its raw joins Req 1's.
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
		// the filter's designators are written with "/" too
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

// groupUnion is addUnion, after dropping an unused list of the row's base name
// that an earlier reader made, so the row's list takes that name, not "(2)".
func (a *compAssembler) groupUnion(r *compRow, items []*types.ReqNode, ls compListRefs) (string, *compList, bool) {
	if r.r.Label != "" {
		base := "Requirement " + r.r.Label
		if !strings.HasSuffix(strings.ToLower(base), " courses") {
			base += " courses"
		}
		used := false
		visit := func(x *compRow) {
			walkReq(x.r.Req, func(n *types.ReqNode) { used = used || slices.Contains(n.Lists, base) })
		}
		walkRows(a.items, visit)
		walkRows(a.program, visit)
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

// listsCap reads complListsCapRe as a union list of the designator and the two
// lists, with the cap a limit under the row.
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

// progression reads complProgressionRe: the amount from one language's courses
// (the list's note lines and its sub-lists'), or the committee's route.
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

// countsAsPart reads complFloorFromRe with complCountsAsPartRe as a union list:
// the part course is a weighted item, excluded from the filter that would
// count it whole; the floor is a constraint under the row.
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

// furtherCoded reads complFurtherCodedRe: one pool of the designator and the
// courses at their shared level, or a course whose content decides.
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

// levelsFrom returns the levels from l up to 400.
func levelsFrom(l int) []int {
	var out []int
	for v := l; v <= 400; v += 100 {
		out = append(out, v)
	}
	return out
}

// listCourses returns the courses listed by code on list key.
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

// twoGroupsExcept reads complTwoGroupsExceptRe. The level is excluded from
// both lists (strict) or only the second (Review). The strict reading needs a
// joint amount too, so it is read only when no full-year course is on both.
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

// groupsOrTitled reads complGroupsOrTitledRe: the lists above the level, or the
// titled list at any level (strict); both together is the lenient reading.
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

var (
	// complPlusRe matches the "plus" between an item's parts.
	complPlusRe = regexp.MustCompile(`,?\s+plus\s+`)
	// complPlusPoolRe matches a pool part: "1.0 credit of ( PHL325H1/
	// PHL331H1)".
	complPlusPoolRe = regexp.MustCompile(`(?i)^(\d+\.\d+\s+credits?)\s+of\s+\(([^()]+)\)$`)
	// complPlusRestRe matches a designator's credits with a floor on a list:
	// "2.0 additional PHL credits, at least 0.5 credit of which must be from
	// Course Group 2 - Value Theory".
	complPlusRestRe = regexp.MustCompile(`^(\d+\.\d+)\s+additional\s+([A-Z]{3})\s+credits,\s+` + atLeastPat + `\s+(\d+\.\d+)\s+credits?\s+of\s+which\s+must\s+be\s+from\s+(?:Course\s+)?(Group\s+\w+)(?:\s+-\s+[A-Z][\w ]*)?$`)
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
		if fixed == nil || !onlyCourses(fixed) || pm == nil || rm == nil {
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
