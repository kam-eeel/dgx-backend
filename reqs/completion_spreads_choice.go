package reqs

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Choices among lists: selections, pathways, areas and the lists below a pool,
// and a list that may stand in for an item's half courses.

// complSelectionRe matches one selection of a choice among lists named by
// their subject: "1.0 credit from the Politics and Economics
// disciplinary/thematic cluster".
var complSelectionRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+from\s+the\s+(.+?)\s+(?:(?:disciplinary/thematic|disciplinary|thematic)\s+)?(?:cluster|group|area|list|concentration)$`)

// listSubject returns the subject a list's title names, as a subjectKey:
// "Politics and Economics" in "Cluster 1: Politics and Economics".
func listSubject(l *compList) string {
	t := l.list.Title
	if t == "" {
		t = l.list.Name
	}
	if i := strings.LastIndex(t, ": "); i >= 0 {
		t = t[i+2:]
	} else if i := strings.Index(t, " - "); i >= 0 {
		t = t[i+3:]
	}
	return subjectKey(t)
}

// subjectKey is a subject in a form to compare: lower case, "&" as "and".
func subjectKey(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.ReplaceAll(s, "&", " and "))), " ")
}

// selections reads a choice whose options are text naming a list by its
// subject ("1.0 credit from the Politics and Economics cluster"): each option
// is credits from that list.
func (a *compAssembler) selections(row *compRow, ls compListRefs) bool {
	n := row.r.Req
	if n.Type != types.NodeChoose && n.Type != types.NodeOr && n.Type != types.NodeAnd || len(n.Items) < 2 {
		return false
	}
	var parts []*types.ReqNode
	for _, it := range n.Items {
		if it.Type != types.NodeText || it.Kind != types.TextOther {
			return false
		}
		m := complSelectionRe.FindStringSubmatch(complClean(it.Text))
		if m == nil {
			return false
		}
		var found []*compList
		for _, l := range a.sortedLists(ls) {
			if !l.bad && listSubject(l) == subjectKey(m[2]) {
				found = append(found, l)
			}
		}
		if len(found) != 1 {
			return false
		}
		v, _ := strconv.ParseFloat(m[1], 64)
		parts = append(parts, &types.ReqNode{Type: types.NodeCredits, Amount: v, Lists: []string{found[0].list.Name}})
	}
	k := 1
	switch n.Type {
	case types.NodeChoose:
		k = n.Count
	case types.NodeAnd:
		k = len(parts)
	}
	body, ok := partsNode(k, parts, ls)
	if !ok {
		return false
	}
	if k == 1 && body.Type == types.NodeChoose {
		body = orNode(body.Items...)
	}
	row.r.Req = body
	return true
}

var (
	// complPathwaysLeadRe matches "1.0 credit from one of the following pathways
	// below:".
	complPathwaysLeadRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+from\s+one\s+of\s+the\s+following\s+pathways(?:\s+below)?\s*:?$`)
	// complPathwayRe matches one pathway: "Thesis Option: HIS475H1 and HIS476H1".
	complPathwayRe = regexp.MustCompile(`(?i)^(?:[a-z]\)\s*)?(\w+)\s+option:\s*(.+)$`)
	// complInOrderRe matches "taken in that order", which the courses' requisites
	// see to, and a pointer to a note: "(see Note 2)".
	complInOrderRe = regexp.MustCompile(`(?i)\s+taken\s+in\s+that\s+order\b\.?|\s*\(\s*see\s+note\s+\d+\s*\)`)
	// complSeniorListRe matches "1.0 credit of Senior (300 or 400-level)
	// Methodology course(s), listed below".
	complSeniorListRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+of\s+senior\s+\(\s*(` + levelPat + `)\s+or\s+(` + levelPat + `)\s*-?\s*level\s*\)\s+(.+?)\s+course(?:\(s\)|s)?,?\s+listed\s+below$`)
)

// pathways reads a choice of pathways whose options are notes or text rules
// under it: their or, each option courses or credits from a list named by its
// title.
func (a *compAssembler) pathways(row *compRow, ls compListRefs) bool {
	t, put, ok := rowText(row)
	if !ok {
		return false
	}
	m := complPathwaysLeadRe.FindStringSubmatch(strings.TrimSpace(complItemLabelRe.ReplaceAllString(t, "")))
	if m == nil {
		return false
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	var opts []*types.ReqNode
	var used []string
	var kids []*compRow
	read := func(s string) bool {
		p := complPathwayRe.FindStringSubmatch(strings.TrimSpace(s))
		if p == nil {
			return false
		}
		body := strings.TrimSpace(strings.TrimSuffix(complInOrderRe.ReplaceAllString(p[2], ""), "."))
		if l := complSeniorListRe.FindStringSubmatch(body); l != nil {
			list := ls[complTitleKey(l[4]+" courses")]
			v, _ := strconv.ParseFloat(l[1], 64)
			if list == nil || list.bad || v != amount {
				return false
			}
			opts = append(opts, &types.ReqNode{Type: types.NodeCredits, Amount: v, Lists: []string{list.list.Name}, Levels: parseLevels(l[2] + "/" + l[3] + "-level")})
			return true
		}
		n, _ := parseCompletionText(body)
		if v, ok := minCredits(n); n == nil || hasText(n) || !ok || v != amount {
			return false
		}
		opts = append(opts, n)
		return true
	}
	for _, n := range row.r.Notes {
		if read(n) {
			used = append(used, n)
		}
	}
	for _, c := range row.children {
		if c.r.Kind == types.RequirementConstraint && c.r.Req.Type == types.NodeText && read(complClean(c.r.Req.Text)) {
			continue
		}
		kids = append(kids, c)
	}
	if len(opts) < 2 || len(opts) != len(used)+len(row.children)-len(kids) {
		return false
	}
	for _, s := range used {
		if i := slices.IndexFunc(a.lines, func(l string) bool { return strings.Contains(rawWords(l), rawWords(s)) }); i >= 0 {
			addRaw(row, strings.TrimSpace(a.lines[i]))
		}
	}
	for _, c := range row.children {
		if !slices.Contains(kids, c) {
			addRaw(row, c.r.Raw)
		}
	}
	put(orNode(opts...))
	row.children = kids
	return true
}

var (
	// complEligibleBelowRe matches a pool's alternative of the lists below it:
	// "from the eligible courses listed below".
	complEligibleBelowRe = regexp.MustCompile(`(?i)^(?:from\s+)?(?:the\s+)?(?:eligible|pre-approved)\s+courses\s+(?:from\s+cognate\s+units\s+)?listed\s+below(?:\s*\(\s*(` + complListRefPat + `)\s*\))?$`)
	// complMustIncludeFloorRe matches "These credits must include 1.0 credit at
	// the 300+ level from AMS courses".
	complMustIncludeFloorRe = regexp.MustCompile(`(?i)^these\s+credits\s+must\s+include\s+` + amountPat + `\s+credits?\s+at\s+the\s+(` + levelPat + `)\+\s+level\s+from\s+(` + desigPat + `)\s+courses$`)
	// complBreadthSplitRe matches a split over breadth categories: "Of the 2.5
	// credits, a minimum of 0.5 credit must be from the Social Sciences/BR=3;
	// another 0.5 credit must be from the Humanities/BR=1 or 2".
	complBreadthSplitRe = regexp.MustCompile(`(?i)^of\s+the\s+` + amountPat + `\s+credits,\s+a\s+minimum\s+of\s+` + amountPat + `\s+credits?\s+must\s+be\s+from\s+the\s+[\w\s]+?/BR=(\d)((?:\s+or\s+\d)?);\s+another\s+` + amountPat + `\s+credits?\s+must\s+be\s+from\s+the\s+[\w\s]+?/BR=(\d)((?:\s+or\s+\d)?)\.?\s*(?:\(\s*(.+?)\s+can\s+be\s+used\s+to\s+cover\s+the\s+[\w\s]+?\s+requirement\s+and\s+(.+?)\s+for\s+the\s+[\w\s]+?\s+requirement\s*\)\.?)?$`)
)

// cognatePool reads a pool whose alternative is the lists below it ("or from
// the eligible courses listed below"): one list of both, with the text rules
// under it read: a floor, and a breadth split as a group of parts.
func (a *compAssembler) cognatePool(row *compRow, ls compListRefs) bool {
	n := row.r.Req
	if n.Type != types.NodeOr || len(n.Items) != 2 || !isPool(n.Items[0]) || n.Items[0].Lists != nil || n.Items[1].Type != types.NodeText || row.r.Label == "" {
		return false
	}
	m := complEligibleBelowRe.FindStringSubmatch(strings.TrimSuffix(complClean(n.Items[1].Text), "."))
	if m == nil {
		return false
	}
	names := a.leadLists(row, ls)
	if m[1] != "" {
		names = ls.resolve(listRefNames(m[1]))
	}
	if names == nil {
		return false
	}
	pool := n.Items[0]
	var split *compRow
	var kids []*compRow
	for _, c := range row.children {
		if c.r.Kind == types.RequirementConstraint && c.r.Req.Type == types.NodeText {
			t := strings.TrimSuffix(complClean(c.r.Req.Text), ".")
			if f := complMustIncludeFloorRe.FindStringSubmatch(t); f != nil {
				v, _ := strconv.ParseFloat(f[1], 64)
				c.r.Req = &types.ReqNode{Type: types.NodeCredits, Amount: v, Designators: []string{f[3]}, Levels: parseLevels(f[2] + "+ level")}
			} else if complBreadthSplitRe.MatchString(t) {
				split = c
				continue
			}
		}
		kids = append(kids, c)
	}
	member := cloneNode(pool)
	member.Amount = 0
	name, _, ok := a.addUnion(row, []*types.ReqNode{member, {Type: types.NodeCredits, Lists: names}}, ls)
	if !ok {
		return false
	}
	body := &types.ReqNode{Type: types.NodeCredits, Amount: pool.Amount, Lists: []string{name}}
	row.r.Req = body
	if split == nil {
		return true
	}
	s := complBreadthSplitRe.FindStringSubmatch(strings.TrimSuffix(complClean(split.r.Req.Text), "."))
	total, _ := strconv.ParseFloat(s[1], 64)
	first, _ := strconv.ParseFloat(s[2], 64)
	second, _ := strconv.ParseFloat(s[5], 64)
	if math.Abs(total-pool.Amount) > 1e-9 || first+second > total {
		row.children = append(kids, split)
		return true
	}
	part := func(v float64, br, more, codes string) *compRow {
		p := &types.ReqNode{Type: types.NodeCredits, Amount: v, Lists: []string{name}, Breadth: parseBreadths(br + more)}
		if cs := codeRe.FindAllString(codes, -1); cs != nil {
			// courses the split names as able to cover this part
			p = orNode(p, &types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: cs})
		}
		return &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Req: p, Raw: split.r.Raw}}
	}
	rest := &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Req: &types.ReqNode{Type: types.NodeCredits, Amount: total - first - second, Lists: []string{name}}, Raw: rawHead(row.r)}}
	row.r.Req = nil
	row.children = append([]*compRow{part(first, s[3], s[4], s[8]), part(second, s[6], s[7], s[9]), rest}, kids...)
	return true
}

// breadthDigitRe matches a breadth category: "3".
var breadthDigitRe = regexp.MustCompile(`\d`)

// parseBreadths reads "3", "1 or 2" as breadth categories.
func parseBreadths(s string) []int {
	var out []int
	for _, f := range breadthDigitRe.FindAllString(s, -1) {
		v, _ := strconv.Atoi(f)
		out = append(out, v)
	}
	return out
}

var (
	// complHeadingLevelRe matches a heading's level filter: "(200+ level)".
	complHeadingLevelRe = regexp.MustCompile(`(?i)\(\s*(` + levelPat + `)\+\s*level\s*\)`)
	// complAreaRowRe matches an area's share: "2.0 credits in Africa/Asia/Middle
	// East with at least one 0.5 credit in 2 of the 3 areas".
	complAreaRowRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+(?:in\s+)?((?-i:[A-Z])[\w/ ]*?)(?:\s+with\s+` + atLeastPat + `\s+one\s+` + amountPat + `\s+credits?\s+in\s+(\w+)\s+of\s+the\s+(\w+)\s+areas)?$`)
	// complAnyAreaCapRe matches "This requirement cannot be met by more than 1.0
	// credit from any area".
	complAnyAreaCapRe = regexp.MustCompile(`(?i)^this\s+requirement\s+cannot\s+be\s+met\s+by\s+more\s+than\s+` + amountPat + `\s+credits?\s+from\s+any\s+(?:one\s+)?(?:area|group|cluster|list)$`)
	// complAreaHeadingRe matches a heading over an area's sub-lists: "Geographic
	// Area a): Africa, Asia, Middle East".
	complAreaHeadingRe = regexp.MustCompile(`^(Geographic\s+Area\s+([a-z])\))\s*:\s*(.+)$`)
	// complSubGroupTitleRe matches a sub-list's title: "(Sub-group: Africa)".
	complSubGroupTitleRe = regexp.MustCompile(`^\(\s*Sub-group:\s*(.+?)\s*\)$`)
)

// areaLists names the History programs' sub-lists by subject ("(Sub-group:
// Africa)" is "Africa") and makes the area headings over them lists of them.
// It runs on the lists buildLists read, before they are looked up by key.
func areaLists(built []*compList) []*compList {
	byKey := map[string]*compList{}
	for _, l := range built {
		byKey[l.key] = l
	}
	var out []*compList
	for _, l := range built {
		m := complSubGroupTitleRe.FindStringSubmatch(l.list.Name)
		if m == nil || !l.titled || len(l.defs) != 1 || l.defs[0].block == nil || byKey[complTitleKey(m[1])] != nil {
			out = append(out, l)
			continue
		}
		l.list.Title, l.list.Name = l.list.Name, m[1]
		l.key = complTitleKey(m[1])
		byKey[l.key] = l
		out = append(out, l)
		var h *compBlock
		for h = l.defs[0].block.heading; h != nil && !complAreaHeadingRe.MatchString(h.section); h = h.heading {
		}
		if h == nil || l.parent != "" {
			continue
		}
		a := complAreaHeadingRe.FindStringSubmatch(h.section)
		key := complTitleKey(a[1])
		p := byKey[key]
		if p == nil {
			p = &compList{key: key, list: &types.CourseList{Name: a[1], Title: h.section}, line: -1, titled: true}
			byKey[key] = p
			out = append(out, p)
		}
		p.at(h.line)
		l.parent = key
		p.list.Items = append(p.list.Items, &types.ReqNode{Type: types.NodeCredits, Lists: []string{l.list.Name}})
	}
	return out
}

// areaGroup reads the History programs' "Geographic Distribution" group: each
// part draws on the area list its letter names, at the heading's levels, with
// its spread and caps per sub-list. It reports whether it read every part.
func (a *compAssembler) areaGroup(row *compRow, ls compListRefs) bool {
	lv := complHeadingLevelRe.FindStringSubmatch(rawHead(row.r))
	if lv == nil || len(row.children) == 0 {
		return false
	}
	levels := parseLevels(lv[1] + "+ level")
	type read struct {
		body  *types.ReqNode
		rules []*types.Requirement
	}
	var reads []read
	for _, c := range row.children {
		t, _, ok := rowText(c)
		m := complAreaRowRe.FindStringSubmatch(strings.TrimSuffix(t, "."))
		if !ok || c.r.Kind != types.RequirementAllocation || m == nil || c.r.Label == "" {
			return false
		}
		area := ls[complTitleKey("Geographic Area "+c.r.Label[len(c.r.Label)-1:]+")")]
		if area == nil || area.bad || !strings.Contains(strings.ToLower(area.list.Title), strings.ToLower(strings.Split(m[2], "/")[0])) {
			return false
		}
		var subs []string
		for _, it := range area.list.Items {
			subs = append(subs, it.Lists...)
		}
		if ls.resolve(keysOf(ls, subs)) == nil {
			return false
		}
		amount, _ := strconv.ParseFloat(m[1], 64)
		r := read{body: &types.ReqNode{Type: types.NodeCredits, Amount: amount, Lists: []string{area.list.Name}, Levels: slices.Clone(levels)}}
		if m[3] != "" {
			each, _ := strconv.ParseFloat(m[3], 64)
			if countOf(m[5]) != len(subs) {
				return false
			}
			n, ok := spreadNode(countOf(m[4]), each, subs, levels, ls)
			if !ok {
				return false
			}
			r.rules = append(r.rules, &types.Requirement{Kind: types.RequirementConstraint, Req: n, Raw: rawHead(c.r)})
		}
		for _, k := range c.children {
			if k.r.Kind != types.RequirementConstraint || k.r.Req.Type != types.NodeText {
				return false
			}
			cm := complAnyAreaCapRe.FindStringSubmatch(strings.TrimSuffix(complClean(k.r.Req.Text), "."))
			if cm == nil {
				return false
			}
			v, _ := strconv.ParseFloat(cm[1], 64)
			for _, s := range subs {
				r.rules = append(r.rules, &types.Requirement{Kind: types.RequirementLimit, Req: &types.ReqNode{Type: types.NodeCredits, Lists: []string{s}, MaxAmount: maxAmount(v)}, Raw: k.r.Raw})
			}
		}
		reads = append(reads, r)
	}
	for i, c := range row.children {
		c.r.Req = reads[i].body
		c.children = nil
		for _, r := range reads[i].rules {
			a.rowUnder(c, r)
		}
	}
	return true
}

// keysOf returns the keys of the lists named names, nil if one is missing.
func keysOf(ls compListRefs, names []string) []string {
	var out []string
	for _, n := range names {
		l := ls.byName(n)
		if l == nil {
			return nil
		}
		out = append(out, l.key)
	}
	return out
}

var (
	// complOneAreaRe matches a lead over a sub-list per area: "2.0 credits in one
	// cultural area chosen from the list below:".
	complOneAreaRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+(?:of\s+[\w\s]+?\s+)?in\s+one\s+(?:language\s+|cultural\s+)?area\s+chosen\s+from\s+the\s+list\s+below\s*:?$`)
	// complChosenAreaRe matches "2.0 credits from culture courses corresponding
	// to the student’s chosen language area:".
	complChosenAreaRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+from\s+[\w\s]+?\s+courses\s+corresponding\s+to\s+the\s+student[’']s\s+chosen\s+language\s+area\s*:?$`)
	// complAltSetsRe matches a sub-list of two sets, one or the other:
	// "[ SLA107Y1, SLA118H1]; /[ SLA109Y1, SLA209H1]".
	complAltSetsRe = regexp.MustCompile(`^\s*\[([^\[\]]+)\]\s*;?\s*/\s*\[([^\[\]]+)\]\s*$`)
)

// areaChoice reads a lead over a sub-list per area: "one area" is the or of
// the areas' pools; "the chosen area" adds the correspondence as text, which
// no node can check: NotMet when no area has the amount, else Review.
func (a *compAssembler) areaChoice(row *compRow) bool {
	t, put, ok := rowText(row)
	if !ok {
		return false
	}
	t = strings.TrimSpace(complItemLabelRe.ReplaceAllString(t, ""))
	m := complOneAreaRe.FindStringSubmatch(t)
	chosen := false
	if m == nil {
		if m = complChosenAreaRe.FindStringSubmatch(t); m == nil {
			return false
		}
		chosen = true
	}
	var opts, caps []*types.ReqNode
	var notes []string
	for _, d := range a.listDefs {
		if d.row != row {
			continue
		}
		alts, ok := areaPools(m[1], d.members)
		if !ok {
			return false
		}
		opts = append(opts, alts...)
		notes = append(notes, d.note)
		if n, _ := parseCompletionText("0.5 credit from: " + d.members); !strings.Contains(d.members, "[") {
			_, c := poolCaps(n)
			caps = append(caps, c...)
		}
	}
	if len(opts) < 2 {
		return false
	}
	body := orNode(opts...)
	if chosen {
		body = andNode(body, textNode(types.TextOther, "corresponding to the student’s chosen language area"))
	}
	put(body)
	// the areas' "/" pair caps, unless a chosen-area item makes this a choice
	if !slices.ContainsFunc(a.items, func(r *compRow) bool {
		return r != row && complChosenAreaRe.MatchString(strings.TrimSpace(complItemLabelRe.ReplaceAllString(complClean(rawHead(r.r)), "")))
	}) && !chosen {
		for _, c := range dedupeNodes(caps) {
			a.rowUnder(row, &types.Requirement{Kind: types.RequirementLimit, Req: c, Raw: rawHead(row.r)})
		}
	}
	row.r.Notes = slices.DeleteFunc(row.r.Notes, func(n string) bool { return slices.Contains(notes, n) })
	return true
}

// complHalfFromListRe matches a list that may stand in for one of an item's
// half courses: "up to one half course from Group H".
var complHalfFromListRe = regexp.MustCompile(`(?i)^up\s+to\s+one\s+half\s+course\s+from\s+(` + complListRefPat + `)$`)

// halfCourseFromList reads or(the item's half courses, a complHalfFromListRe
// clause): the clause is all but a half course from the courses and one from
// the list, with caps on the courses' "/" pairs.
func (a *compAssembler) halfCourseFromList(row *compRow, ls compListRefs) bool {
	n := row.r.Req
	if n == nil || n.Type != types.NodeOr || len(n.Items) != 2 || n.Items[1].Type != types.NodeText || n.Items[1].Kind != types.TextOther {
		return false
	}
	m := complHalfFromListRe.FindStringSubmatch(strings.TrimSuffix(complClean(n.Items[1].Text), "."))
	if m == nil {
		return false
	}
	names := ls.resolve(listRefNames(m[1]))
	if len(names) != 1 {
		return false
	}
	strict := n.Items[0]
	if strict.Type != types.NodeAnd || len(strict.Items) < 2 {
		return false
	}
	var codes []string
	var caps []*types.ReqNode
	for _, part := range strict.Items {
		var alt []string
		switch {
		case part.Type == types.NodeCourse:
			alt = []string{part.Code}
		case part.Type == types.NodeOr && !slices.ContainsFunc(part.Items, func(x *types.ReqNode) bool { return x.Type != types.NodeCourse }):
			for _, x := range part.Items {
				alt = append(alt, x.Code)
			}
			caps = append(caps, &types.ReqNode{Type: types.NodeCredits, Courses: alt, MaxAmount: maxAmount(0.5)})
		default:
			return false
		}
		for _, c := range alt {
			if courseWeight(c) != 0.5 {
				return false
			}
		}
		codes = append(codes, alt...)
	}
	amount := 0.5 * float64(len(strict.Items))
	row.r.Req = orNode(strict, andNode(&types.ReqNode{Type: types.NodeCredits, Amount: amount - 0.5, Courses: dedupe(codes)}, &types.ReqNode{Type: types.NodeCredits, Amount: 0.5, Lists: names}))
	for _, c := range caps {
		a.rowUnder(row, &types.Requirement{Kind: types.RequirementLimit, Req: c, Raw: rawHead(row.r)})
	}
	return true
}

// areaPools reads one area's line under areaChoice as the pools of amount it
// allows: one, or one per set when the line is two sets, one or the other.
// False when a set isn't a pool of courses that can supply the amount.
func areaPools(amount string, members string) ([]*types.ReqNode, bool) {
	want, _ := strconv.ParseFloat(amount, 64)
	sets := []string{members}
	if s := complAltSetsRe.FindStringSubmatch(members); s != nil {
		sets = s[1:] // one set or the other, not a mix
	}
	var alts []*types.ReqNode
	for _, set := range sets {
		n, _ := parseCompletionText(amount + " credits from: " + strings.NewReplacer("[", " ", "]", " ", "(", " ", ")", " ").Replace(set))
		n, _ = poolCaps(n)
		if !isPool(n) || n.Courses == nil || n.Designators != nil || n.Levels != nil || n.Amount != want {
			return nil, false
		}
		sum := 0.0
		for _, c := range n.Courses {
			sum += courseWeight(c)
		}
		if sum < want-1e-9 {
			return nil, false // the area can't supply the amount: not a pool as read
		}
		alts = append(alts, n)
	}
	return alts, true
}
