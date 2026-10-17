package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Caps and floors on lists: an item over its own lists with a cap or floor on
// each, caps on pairs of lists, and caps no pool can reach.

var (
	// complSameCategoryCapRe matches a pool over the item's own lists with a cap
	// on each: "Any 3.0 credits from the following, with no more than 1.0 credit
	// from the same category".
	complSameCategoryCapRe = regexp.MustCompile(`(?i)^(?:any\s+)?` + amountPat + `\s+credits?\s+from\s+the\s+following,?\s+with\s+` + noMoreThanPat + `\s+` + amountPat + `\s+credits?\s+from\s+the\s+same\s+(?:category|group|list|area|sub-?group)$`)
	// complFloorCapEachRe matches a floor and a cap on each of the item's own
	// lists, with the heading's amount: "At least 0.5 credit, but no more than
	// 1.0 credit from each subgroup".
	complFloorCapEachRe = regexp.MustCompile(`(?i)^` + atLeastPat + `\s+` + amountPat + `\s+credits?,?\s+but\s+` + noMoreThanPat + `\s+` + amountPat + `\s+credits?\s+from\s+each\s+(?:category|group|list|area|sub-?group)$`)
	// complListLevelRulesRe matches a pool over the item's own lists with a floor
	// and a cap by level: "... At least 1.0 credit needs to be at the 400 level,
	// and no more than 1.5 credits from the 200 level".
	complListLevelRulesRe = regexp.MustCompile(`(?i)^a\s+minimum\s+of\s+` + amountPat + `\s+credits?\s+from\s+the\s+following\s+list\s+that\s+have\s+not\s+been\s+used\s+to\s+fulfil+\s+the\s+above\s+requirements\.\s+` + atLeastPat + `\s+` + amountPat + `\s+credits?\s+needs?\s+to\s+be\s+at\s+the\s+(` + levelPat + `)\s*-?\s*level,?\s+and\s+` + noMoreThanPat + `\s+` + amountPat + `\s+credits?\s+from\s+the\s+(` + levelPat + `)\s*-?\s*level$`)
	// complCountTowardsRe matches a note adding an earlier item's extra courses
	// to this one: "In that case, the additional courses will count towards the
	// Other Geosciences"; complChooseMoreRe the sentence before it: "You may
	// choose more than 2.5 credits from the Additional Foundation list".
	complCountTowardsRe = regexp.MustCompile(`(?i)^in\s+that\s+case,\s+the\s+additional\s+courses\s+will\s+count\s+towards?\s+the\s+(.+?)$`)
	complChooseMoreRe   = regexp.MustCompile(`(?i)^you\s+may\s+choose\s+more\s+than\s+` + numPat + `\s+credits?\s+from\s+the\s+(.+?)\s+list$`)
	// complLevelFloorFromListRe matches a pool at a level over the item's own
	// lists with a floor on one: "2.0 credits at the 300+ level (at least 1.0
	// credit must be from Group 1) from:".
	complLevelFloorFromListRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+at\s+the\s+([1-4])00\+\s+level\s+\(\s*` + atLeastPat + `\s+` + amountPat + `\s+credits?\s+must\s+be\s+from\s+(` + complListRefPat + `)\s*\)\s+from\s*:?$`)
	// complHeadingAmountRe matches a heading's amount: "Additional Foundation
	// Geosciences (2.5 credits):".
	complHeadingAmountRe = regexp.MustCompile(`(?i)\(\s*` + amountPat + `\s+credits?\s*\)\s*:?\s*(?:\*\*)?\s*$`)
)

// listPairCaps returns the caps on the "/" pairs of the lists names (each
// defined once), or false when their members don't read as a pool. The amount
// is 0.5 so the grammar doesn't read the members as all required.
func listPairCaps(names []string, ls compListRefs) ([]*types.ReqNode, bool) {
	var members []string
	for _, name := range names {
		l := ls.byName(name)
		if l == nil || len(l.defs) != 1 {
			return nil, false
		}
		members = append(members, strings.TrimSuffix(strings.TrimSpace(l.defs[0].members), "."))
	}
	all := strings.Join(members, ", ")
	n, _ := parseCompletionText("0.5 credit from: " + all)
	if pool, caps := poolCaps(n); isPool(pool) {
		return caps, true
	}
	return nil, !strings.Contains(all, "/") // no pairs to cap
}

// headingAmount returns the amount of the heading row's lead sits under.
func (a *compAssembler) headingAmount(row *compRow) (float64, bool) {
	found, amount := false, 0.0
	for b, r := range a.leadRows {
		if r != row || b.heading == nil {
			continue
		}
		m := complHeadingAmountRe.FindStringSubmatch(b.heading.text)
		if m == nil {
			return 0, false
		}
		v, _ := strconv.ParseFloat(m[1], 64)
		if found && v != amount {
			return 0, false
		}
		found, amount = true, v
	}
	return amount, found
}

// categoryCaps reads an item over its own lists with a cap or floor on each, or
// level rules. A note letting an earlier item's extra courses count here adds
// its lists to the pool: exclusive allocation leaves only the extra ones.
func (a *compAssembler) categoryCaps(row *compRow, ls compListRefs) bool {
	t, put, ok := rowText(row)
	if !ok {
		return false
	}
	t = strings.TrimSpace(strings.TrimSuffix(t, "."))
	names := a.ownLists(row, ls)
	if len(names) < 2 {
		return false
	}
	limits := func(v float64, caps []*types.ReqNode) {
		for _, c := range caps {
			a.rowUnder(row, &types.Requirement{Kind: types.RequirementLimit, Req: c, Raw: rawHead(row.r)})
		}
		for _, name := range names {
			a.rowUnder(row, &types.Requirement{Kind: types.RequirementLimit, Req: &types.ReqNode{Type: types.NodeCredits, Lists: []string{name}, MaxAmount: maxAmount(v)}, Raw: rawHead(row.r)})
		}
	}
	if m := complSameCategoryCapRe.FindStringSubmatch(t); m != nil {
		amount, _ := strconv.ParseFloat(m[1], 64)
		v, _ := strconv.ParseFloat(m[2], 64)
		caps, ok := listPairCaps(names, ls)
		if !ok || v*float64(len(names)) < amount-1e-9 {
			return false
		}
		put(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Lists: names})
		limits(v, caps)
		return true
	}
	if m := complFloorCapEachRe.FindStringSubmatch(t); m != nil {
		amount, ok := a.headingAmount(row)
		each, _ := strconv.ParseFloat(m[1], 64)
		v, _ := strconv.ParseFloat(m[2], 64)
		if !ok || each*float64(len(names)) > amount+1e-9 || v*float64(len(names)) < amount-1e-9 {
			return false
		}
		caps, ok := listPairCaps(names, ls)
		if !ok || !a.spread(row, put, amount, len(names), each, names, nil, nil, ls) {
			return false
		}
		limits(v, caps)
		return true
	}
	if m := complLevelFloorFromListRe.FindStringSubmatch(t); m != nil {
		// the floor's list is one of the item's; the pool is at the level already
		amount, _ := strconv.ParseFloat(m[1], 64)
		lo, _ := strconv.Atoi(m[2])
		floor, _ := strconv.ParseFloat(m[3], 64)
		on := ls.resolve(listRefNames(m[4]))
		caps, ok := listPairCaps(names, ls)
		if !ok || len(on) != 1 || !slices.Contains(names, on[0]) || floor > amount {
			return false
		}
		var levels []int
		for l := lo * 100; l <= 400; l += 100 {
			levels = append(levels, l)
		}
		put(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Levels: levels, Lists: names})
		for _, c := range caps {
			a.rowUnder(row, &types.Requirement{Kind: types.RequirementLimit, Req: c, Raw: rawHead(row.r)})
		}
		a.rowUnder(row, &types.Requirement{Kind: types.RequirementConstraint, Req: &types.ReqNode{Type: types.NodeCredits, Amount: floor, Lists: on}, Raw: rawHead(row.r)})
		return true
	}
	if m := complListLevelRulesRe.FindStringSubmatch(t); m != nil {
		amount, _ := strconv.ParseFloat(m[1], 64)
		floor, _ := strconv.ParseFloat(m[2], 64)
		hi, _ := strconv.Atoi(m[3])
		v, _ := strconv.ParseFloat(m[4], 64)
		lo, _ := strconv.Atoi(m[5])
		caps, ok := listPairCaps(names, ls)
		if !ok {
			return false
		}
		pool := slices.Clone(names)
		var route, note string
		for _, s := range a.programRoutes {
			if m := complCountTowardsRe.FindStringSubmatch(strings.TrimSuffix(complClean(s), ".")); m != nil && strings.EqualFold(m[1], row.r.Section) {
				route = s
			}
		}
		for _, x := range a.notes {
			if c := complChooseMoreRe.FindStringSubmatch(strings.TrimSuffix(x, ".")); c != nil && route != "" {
				for _, r := range a.items {
					if r != row && strings.EqualFold(r.r.Section, c[1]) {
						if more := a.ownLists(r, ls); more != nil && note == "" {
							note, pool = x, append(pool, more...)
						}
					}
				}
			}
		}
		put(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Lists: pool})
		for _, c := range caps {
			a.rowUnder(row, &types.Requirement{Kind: types.RequirementLimit, Req: c, Raw: rawHead(row.r)})
		}
		a.rowUnder(row, &types.Requirement{Kind: types.RequirementConstraint, Req: &types.ReqNode{Type: types.NodeCredits, Amount: floor, Levels: []int{hi}}, Raw: rawHead(row.r)})
		a.rowUnder(row, &types.Requirement{Kind: types.RequirementLimit, Req: &types.ReqNode{Type: types.NodeCredits, Levels: []int{lo}, MaxAmount: maxAmount(v)}, Raw: rawHead(row.r)})
		if note != "" {
			addRaw(row, note+". "+route)
			a.notes = slices.DeleteFunc(a.notes, func(x string) bool { return x == note })
			a.programRoutes = slices.DeleteFunc(a.programRoutes, func(x string) bool { return x == route })
		}
		return true
	}
	return false
}

// complCombinationCapRe matches a pool over the lists below with a cap on one:
// "An additional 3.0 credits from any combination of courses listed in the
// three groups below, with no more than 1.0 credit from Group C".
var complCombinationCapRe = regexp.MustCompile(`(?i)^(?:an\s+)?additional\s+` + amountPat + `\s+credits?\s+from\s+any\s+combination\s+of\s+courses\s+listed\s+in\s+the\s+(two|three|four|five|\d)\s+groups\s+below\*?,?\s+with\s+` + noMoreThanPat + `\s+` + amountPat + `\s+credits?\s+from\s+(` + complListRefPat + `)(?::\s+[^,;]+)?$`)

// combinationCap reads a complCombinationCapRe item: the pool of the lists
// below and a limit on the one it caps. A note that lets some students take
// less (complOnlyNeedRe) adds the smaller pool with the note, Review for them.
func (a *compAssembler) combinationCap(row *compRow, ls compListRefs) bool {
	n := row.r.Req
	var alts []*types.ReqNode
	if n != nil && n.Type == types.NodeOr && len(n.Items) > 1 {
		n, alts = n.Items[0], n.Items[1:]
	}
	var only *types.ReqNode
	if n != nil && n.Type == types.NodeAnd && len(n.Items) == 2 && n.Items[1].Type == types.NodeText && complOnlyNeedRe.MatchString(n.Items[1].Text) {
		n, only = n.Items[0], n.Items[1]
	}
	if n == nil || n.Type != types.NodeText || n.Kind != types.TextOther {
		return false
	}
	m := complCombinationCapRe.FindStringSubmatch(strings.TrimSuffix(complClean(n.Text), "."))
	if m == nil {
		return false
	}
	names := a.listsBelow(row, ls)
	capped := ls.resolve(listRefNames(m[4]))
	if len(names) != countOf(m[2]) || len(capped) != 1 || !slices.Contains(names, capped[0]) {
		return false
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	v, _ := strconv.ParseFloat(m[3], 64)
	caps, ok := listPairCaps(names, ls)
	if !ok {
		return false
	}
	body := &types.ReqNode{Type: types.NodeCredits, Amount: amount, Lists: names}
	if only != nil {
		less, _ := strconv.ParseFloat(complOnlyNeedRe.FindStringSubmatch(only.Text)[1], 64)
		if less <= 0 || less >= amount {
			return false
		}
		smaller := cloneNode(body)
		smaller.Amount = less
		body = orNode(body, andNode(smaller, only))
	}
	row.r.Req = orNode(append([]*types.ReqNode{body}, alts...)...)
	for _, c := range caps {
		a.rowUnder(row, &types.Requirement{Kind: types.RequirementLimit, Req: c, Raw: rawHead(row.r)})
	}
	a.rowUnder(row, &types.Requirement{Kind: types.RequirementLimit, Req: &types.ReqNode{Type: types.NodeCredits, Lists: capped, MaxAmount: maxAmount(v)}, Raw: rawHead(row.r)})
	return true
}

var (
	// complSameTraditionRe matches a department pool with a floor in the
	// student's tradition: "... at least 0.5 credit of which must be in the same
	// tradition as was chosen from #3 above".
	complSameTraditionRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+from\s+([1-4])00\+\s+level\s+department\s+for\s+the\s+study\s+of\s+religion\s+courses,\s+(` + atLeastPat + `\s+` + amountPat + `\s+credits?\s+of\s+which\s+must\s+be\s+in\s+the\s+same\s+tradition\s+as\s+was\s+chosen\s+from\s+#\d+\s+above)$`)
	// complFocusListsRe matches the text the grammar leaves beside an amount in
	// the student's area of focus: "in your area of focus selected from
	// Department for the Study of Religion courses".
	complFocusListsRe = regexp.MustCompile(`(?i)^(in\s+your\s+area\s+of\s+focus)\s+selected\s+from\s+department\s+for\s+the\s+study\s+of\s+religion\s+courses$`)
	// complDSRDesignatorsRe matches another item's list of the department's
	// designators: "Department for the Study of Religion courses carrying any of
	// the following designators: RLG, JAR, JCR".
	complDSRDesignatorsRe = regexp.MustCompile(`(?i)\bdepartment\s+for\s+the\s+study\s+of\s+religion\s+courses\s+carrying\s+any\s+of\s+the\s+following\s+designators:\s*` + complAliasDesigListPat + `\s*\.?\s*$`)
)

// traditionLists reads an item over its lists of each tradition's (or area of
// focus's) courses, where the student chooses which counts: the lists are the
// pool, and the choice is content text beside them.
func (a *compAssembler) traditionLists(row *compRow, ls compListRefs) bool {
	names := a.ownLists(row, ls)
	if len(names) < 2 {
		return false
	}
	n := row.r.Req
	if n != nil && n.Type == types.NodeOr && len(n.Items) > 1 && !slices.ContainsFunc(n.Items[1:], func(it *types.ReqNode) bool { return it.Type != types.NodeText || it.Kind != types.TextPermission }) {
		n = n.Items[0] // or(body, a route)
	}
	if n != nil && n.Type == types.NodeAnd && len(n.Items) == 2 && n.Items[1].Type == types.NodeText && n.Items[1].Kind == types.TextOther {
		c := n.Items[0]
		m := complFocusListsRe.FindStringSubmatch(strings.TrimSuffix(complClean(n.Items[1].Text), "."))
		if m == nil || c.Type != types.NodeCredits || c.Amount <= 0 || c.Levels == nil || c.Courses != nil || c.Designators != nil || c.Lists != nil || c.Items != nil {
			return false
		}
		c.Lists = names
		n.Items[1] = textNode(types.TextContent, m[1])
		return true
	}
	t, put, ok := rowText(row)
	if !ok {
		return false
	}
	t = strings.TrimSpace(strings.TrimSuffix(t, "."))
	levelsFrom := func(d string) []int {
		lo, _ := strconv.Atoi(d)
		var levels []int
		for l := lo * 100; l <= 400; l += 100 {
			levels = append(levels, l)
		}
		return levels
	}
	if m := complSameTraditionRe.FindStringSubmatch(t); m != nil {
		var desigs []string
		for _, l := range a.lines {
			if d := complDSRDesignatorsRe.FindStringSubmatch(complClean(l)); d != nil {
				if desigs != nil {
					return false
				}
				desigs = desigRe.FindAllString(d[1], -1)
			}
		}
		if desigs == nil {
			return false
		}
		amount, _ := strconv.ParseFloat(m[1], 64)
		floor, _ := strconv.ParseFloat(m[4], 64)
		put(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Designators: desigs, Levels: levelsFrom(m[2])})
		a.rowUnder(row, &types.Requirement{Kind: types.RequirementConstraint, Req: andNode(&types.ReqNode{Type: types.NodeCredits, Amount: floor, Lists: names}, textNode(types.TextContent, m[3])), Raw: rawHead(row.r)})
		return true
	}
	return false
}

// complListsOrWebsiteRe matches a pool of two named lists and a list kept on a
// website: "2.0 credits from Group A and/or Group B and/or from the additional
// list of approved courses as listed on the program website".
var complListsOrWebsiteRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+(?:additional\s+)?credits?\s+from\s+(.+?)\s+and/or\s+(.+?)\s+and/or\s+(from\s+the\s+additional\s+list\s+of\s+approved\s+courses\b.*?\s+as\s+listed\s+on\s+the\s+.+?\s+website\b.*)$`)

// websiteLists reads a complListsOrWebsiteRe item: the pool of the two lists,
// or the amount with the website's list as content text (the catalogue doesn't
// hold it), and the caps on the lists' "/" pairs.
func (a *compAssembler) websiteLists(row *compRow, ls compListRefs) bool {
	t, put, ok := rowText(row)
	if !ok {
		return false
	}
	m := complListsOrWebsiteRe.FindStringSubmatch(strings.TrimSpace(strings.TrimSuffix(t, ".")))
	if m == nil {
		return false
	}
	var names []string
	for _, title := range []string{m[2], m[3]} {
		l := ls.byName(strings.TrimSpace(title))
		if l == nil || l.bad || !l.titled {
			return false
		}
		names = append(names, l.list.Name)
	}
	caps, ok := listPairCaps(names, ls)
	if !ok {
		return false
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	put(orNode(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Lists: names},
		andNode(&types.ReqNode{Type: types.NodeCredits, Amount: amount}, textNode(types.TextContent, m[4]))))
	for _, c := range caps {
		a.rowUnder(row, &types.Requirement{Kind: types.RequirementLimit, Req: c, Raw: rawHead(row.r)})
	}
	return true
}

// unreachableCaps drops the limits under a pool that leave it short of its
// amount, so it can never be Met: the "/" or cap can't mean what they say. The
// pool becomes and(pool, ambiguity text): Review, never NotMet.
func (a *compAssembler) unreachableCaps(roots []*compRow, lists []*types.CourseList) {
	byName := listsByName(lists)
	walkRowsPost(roots, func(r *compRow) {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeCredits || n.Amount == 0 ||
			n.Designators != nil || n.Levels != nil || n.Breadth != nil || n.Requirements != nil || n.Exclude != nil {
			return
		}
		codes := n.Courses
		if n.Lists != nil {
			cs, ok := a.listCodes(n.Lists, byName)
			if !ok || n.Courses != nil {
				return
			}
			codes = cs
		}
		codes = dedupe(slices.Clone(codes))
		if codes == nil {
			return
		}
		var caps []*compRow
		used := map[string]bool{}
		reach := 0.0
		for _, k := range r.children {
			l := k.r.Req
			if k.r.Kind != types.RequirementLimit || l == nil || l.Type != types.NodeCredits || l.Courses == nil ||
				l.MaxAmount == nil || l.Designators != nil || l.Levels != nil || l.Lists != nil || l.Transfer ||
				slices.ContainsFunc(l.Courses, func(c string) bool { return !slices.Contains(codes, c) || used[c] }) {
				continue
			}
			caps = append(caps, k)
			for _, c := range l.Courses {
				used[c] = true
			}
			reach += min(weightOf(l.Courses), *l.MaxAmount)
		}
		if caps == nil {
			return
		}
		for _, c := range codes {
			if !used[c] {
				reach += courseWeight(c)
			}
		}
		// only when the caps are why: short without them, the list is partial
		if reach >= n.Amount-1e-9 || weightOf(codes) < n.Amount-1e-9 {
			return
		}
		r.children = slices.DeleteFunc(r.children, func(k *compRow) bool { return slices.Contains(caps, k) })
		r.r.Req = andNode(n, textNode(types.TextAmbiguity, "the caps on the pool's courses leave less than its amount, so they may not apply as written"))
	})
}
