package reqs

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// One-off readings of items that draw on lists: lists above or below, titled
// lists, a remainder from lists, and lists of a program's own courses.

// complGroupSubgroupsRe matches "ANT200Y1/ ARH100Y1/ ( ANT204H1 and
// ANT207H1/0.5 credit 300-level Social Anthropology course from Anthropology
// Group C, or Subgroup C(i), C(ii))".
var complGroupSubgroupsRe = regexp.MustCompile(`^((?:` + codePat + `\s*/\s*)+)\(\s*(` + codePat + `)\s+and\s+(` + codePat + `)\s*/\s*0\.5\s+credit\s+([1-4])00-level\s+[\w\s]+?\s+course\s+from\s+[\w\s]*?Group\s+([A-Z]),\s+or\s+Subgroup\s+[A-Z]\s*\((i{1,3})\),\s*[A-Z]\s*\((i{1,3})\)\s*\)$`)

// groupSubgroups reads complGroupSubgroupsRe as or(the full-year courses,
// and(the first half course, or(the second, 0.5 credit at the level from the
// group and sub-groups))); nil when a list isn't there.
func groupSubgroups(s string, ls compListRefs) *types.ReqNode {
	m := complGroupSubgroupsRe.FindStringSubmatch(s)
	if m == nil {
		return nil
	}
	g := m[5]
	names := ls.resolve([]string{complListKey("group", g), complListKey("subgroup", g+"("+m[6]+")"), complListKey("subgroup", g+"("+m[7]+")")})
	if names == nil {
		return nil
	}
	alts := courseNodes(codeRe.FindAllString(m[1], -1))
	level := int(m[4][0]-'0') * 100
	half := orNode(&types.ReqNode{Type: types.NodeCourse, Code: m[3]}, &types.ReqNode{Type: types.NodeCredits, Amount: 0.5, Levels: []int{level}, Lists: names})
	return orNode(append(alts, andNode(&types.ReqNode{Type: types.NodeCourse, Code: m[2]}, half))...)
}

// complListAboveRe matches "6.0 credits from the list above and/or SMC165H1/
// MST226H1/ ...".
var complListAboveRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+from\s+the\s+list\s+above\s+and/or\s+((?:` + codePat + `\s*/?\s*)+)$`)

// listAbove reads complListAboveRe as one pool of its codes and the list
// above: the one earlier item whose pool is courses from "the following".
// Nil when there is none or more.
func (a *compAssembler) listAbove(row *compRow, s string) *types.ReqNode {
	m := complListAboveRe.FindStringSubmatch(s)
	if m == nil {
		return nil
	}
	sibs := a.items
	i := slices.Index(sibs, row)
	if i < 0 {
		return nil
	}
	var above []*compRow
	for _, q := range sibs[:i] {
		if q.r.Req != nil && plainPool(q.r.Req) && strings.Contains(strings.ToLower(q.r.Raw), "the following") {
			above = append(above, q)
		}
	}
	if len(above) != 1 {
		return nil
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	return &types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: dedupe(append(slices.Clone(above[0].r.Req.Courses), codeRe.FindAllString(m[2], -1)...))}
}

var (
	// complComplementaryRe matches "5.0 credits of complementary courses, i.e.,
	// courses relevant to PCJ".
	complComplementaryRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits\s+of\s+complementary\s+courses\b`)
	// complComplementaryNoteRe matches "Complementary courses include but are not
	// limited to the following", with or without codes after it.
	complComplementaryNoteRe = regexp.MustCompile(`(?i)^complementary\s+courses\s+include\s+but\s+are\s+not\s+limited\s+to\s+the\s+following\b`)
	// complOtherEligibleRe matches "Other courses are also eligible to meet this
	// requirement; please contact the program director for ... approval".
	complOtherEligibleRe = regexp.MustCompile(`(?i)^other\s+courses\s+are\s+also\s+eligible\s+to\s+meet\s+this\s+requirement;.*\bapproval\b`)
	// complCodesOnlyRe matches a line of codes alone.
	complCodesOnlyRe = regexp.MustCompile(`^(?:` + codePat + `\s*[,/]?\s*)+\.?$`)
)

// complementaryCourses reads a non-exhaustive list of complementary courses
// as the item's pool beside its approval route (Met by the listed courses,
// Review by others): from a line of codes and two program notes, or from a
// pool's permission alternative that holds the list.
func (a *compAssembler) complementaryCourses() {
	all := append(append(append([]*compRow{}, a.items...), a.program...), a.floors...)
	walkRows(all, func(r *compRow) {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr || n.Items[0].Type != types.NodeCredits || n.Items[0].Designators == nil {
			return
		}
		var codes []string
		for _, it := range n.Items[1:] {
			if it.Type == types.NodeText && complComplementaryNoteRe.MatchString(it.Text) {
				codes = codeRe.FindAllString(it.Text, -1)
			}
		}
		if codes == nil {
			return
		}
		pool := n.Items[0]
		pool.Courses = dedupe(append(pool.Courses, codes...))
		// a route for "this 100-level course requirement" is another item's
		head := rawHead(r.r)
		n.Items = slices.DeleteFunc(n.Items, func(it *types.ReqNode) bool {
			m := complThisLevelReqRe.FindStringSubmatch(it.Text)
			if it.Type != types.NodeText || m == nil || strings.Contains(head, m[1]+"-level") || !a.othersRaw(r, it.Text) {
				return false
			}
			lines := strings.Split(r.r.Raw, "\n")
			r.r.Raw = strings.Join(slices.DeleteFunc(lines, func(l string) bool { return rawWords(l) == rawWords(it.Text) }), "\n")
			return true
		})
	})
	var row, list *compRow
	walkRows(all, func(r *compRow) {
		if t := r.r.Req; t != nil && t.Type == types.NodeText {
			switch {
			case r.r.Kind == types.RequirementAllocation && complComplementaryRe.MatchString(t.Text):
				row = r
			case r.r.Kind == types.RequirementConstraint && complCodesOnlyRe.MatchString(strings.TrimSpace(t.Text)):
				list = r
			}
		}
	})
	lead := slices.IndexFunc(a.notes, func(s string) bool { return complComplementaryNoteRe.MatchString(s) && codeRe.FindString(s) == "" })
	route := slices.IndexFunc(a.notes, func(s string) bool { return complOtherEligibleRe.MatchString(s) })
	if row == nil || list == nil || lead < 0 || route < 0 {
		return
	}
	v, _ := strconv.ParseFloat(complComplementaryRe.FindStringSubmatch(row.r.Req.Text)[1], 64)
	perm, note := a.notes[route], a.notes[lead]
	row.r.Req = orNode(&types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: dedupe(codeRe.FindAllString(list.r.Req.Text, -1))}, textNode(types.TextPermission, perm))
	addRaw(row, note)
	addRaw(row, perm)
	addRaw(row, list.r.Raw)
	a.notes = slices.DeleteFunc(a.notes, func(s string) bool { return s == perm || s == note })
	a.dropRow(list)
}

// complThisLevelReqRe matches a route for one level's item: "eligible to
// meet this 100-level course requirement".
var complThisLevelReqRe = regexp.MustCompile(`(?i)\bthis\s+(` + levelPat + `)-level\s+course\s+requirement\b`)

// othersRaw reports whether s is in the raw of a row other than r.
func (a *compAssembler) othersRaw(r *compRow, s string) bool {
	found := false
	walkRows(a.items, func(q *compRow) {
		if q != r && strings.Contains(rawWords(q.r.Raw), rawWords(s)) {
			found = true
		}
	})
	return found
}

var (
	// complListBelowFloorRe matches "2.5 credits in MUS courses from the list
	// below, including one credit at the 300+ level".
	complListBelowFloorRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits\s+in\s+(` + desigPat + `)\s+courses\s+from\s+the\s+list\s+below,\s+including\s+(one|` + numPat + `)\s+credits?\s+at\s+the\s+([1-4])00\+\s+level$`)
	// complEitherCountedRe matches "Either MUS120Y1 or MUS220Y1 can be counted
	// towards the 2.5 credits in MUS courses".
	complEitherCountedRe = regexp.MustCompile(`(?i)^either\s+(` + codePat + `)\s+or\s+(` + codePat + `)\s+can\s+be\s+counted\s+towards?\s+the\b`)
)

// listBelowMissing reads or(text{complListBelowFloorRe},
// text{complEitherCountedRe}) where the calendar leaves the list out:
// and(credits{N, D}, text), with the floor and a one-course limit under it.
func (a *compAssembler) listBelowMissing(r *compRow) {
	n := r.r.Req
	if len(n.Items) != 2 || n.Items[0].Type != types.NodeText || n.Items[1].Type != types.NodeText || len(a.listDefs) > 0 {
		return
	}
	m := complListBelowFloorRe.FindStringSubmatch(strings.TrimSpace(n.Items[0].Text))
	e := complEitherCountedRe.FindStringSubmatch(strings.TrimSpace(n.Items[1].Text))
	if m == nil || e == nil || courseWeight(e[1]) != courseWeight(e[2]) {
		return
	}
	if lines := strings.Split(strings.TrimSpace(a.raw), "\n"); !strings.Contains(r.r.Raw, strings.TrimSpace(lines[len(lines)-1])) {
		return // a list may follow
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	f := 1.0
	if !strings.EqualFold(m[3], "one") {
		f, _ = strconv.ParseFloat(m[3], 64)
	}
	var levels []int
	for l := int(m[4][0]-'0') * 100; l <= 400; l += 100 {
		levels = append(levels, l)
	}
	r.r.Req = andNode(&types.ReqNode{Type: types.NodeCredits, Amount: v, Designators: []string{m[2]}}, n.Items[0])
	a.rowUnder(r, &types.Requirement{Kind: types.RequirementConstraint, Req: &types.ReqNode{Type: types.NodeCredits, Amount: f, Levels: levels}, Raw: r.r.Raw})
	a.rowUnder(r, &types.Requirement{Kind: types.RequirementLimit, Req: &types.ReqNode{Type: types.NodeCredits, Courses: []string{e[1], e[2]}, MaxAmount: maxAmount(courseWeight(e[1]))}, Raw: n.Items[1].Text})
	addRaw(r, n.Items[1].Text)
}

var (
	// complTitledFloorsRe matches a total with floors on a titled list: "7.0 ENG
	// credits including 2.0 credits from 300+ level Creative Writing courses, at
	// least 0.5 credit of which must be at the 400-level".
	complTitledFloorsRe = regexp.MustCompile(`(?i)^` + numPat + `\s+` + desigPat + `\s+credits\s+(including\s+` + amountPat + `\s+credits\s+from\s+([1-4])00\+\s+level\s+(.+?)\s+courses),\s+(` + atLeastPat + `\s+` + amountPat + `\s+credits?\s+of\s+which\s+must\s+be\s+at\s+the\s+([1-4])00-level)\.?$`)
	// complToIncludeTitledRe matches an item with floors on a titled list: "3.0
	// credits from ENG courses, to include 2.0 credits from Creative Writing
	// courses at the 300+ level, at least 0.5 credits of which must be ...".
	complToIncludeTitledRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits\s+from\s+(` + desigPat + `)\s+courses,\s+to\s+include\s+` + amountPat + `\s+credits\s+from\s+(.+?)\s+courses\s+at\s+the\s+([1-4])00\+\s+level,\s+` + atLeastPat + `\s+` + amountPat + `\s+credits?\s+of\s+which\s+must\s+be\s+from\s+(.+?)\s+courses\s+at\s+the\s+([1-4])00\s+level\.?$`)
)

// titledFloors reads floors on a titled list that the grammar leaves as text:
// the total's (complTitledFloorsRe, with titledTotal) and an item's
// (complToIncludeTitledRe), whose lenient reading counts the list's other
// courses among the designator's.
func (a *compAssembler) titledFloors(ls compListRefs) {
	list := func(title string) string {
		for _, l := range ls {
			if l.titled && !l.bad && strings.EqualFold(l.list.Name, strings.TrimSpace(title)) {
				return l.list.Name
			}
		}
		return ""
	}
	levels := func(d string, plus bool) []int {
		l := int(d[0]-'0') * 100
		if !plus {
			return []int{l}
		}
		var out []int
		for ; l <= 400; l += 100 {
			out = append(out, l)
		}
		return out
	}
	floor := func(v string, name string, lv []int) *types.ReqNode {
		f, _ := strconv.ParseFloat(v, 64)
		return &types.ReqNode{Type: types.NodeCredits, Amount: f, Lists: []string{name}, Levels: lv}
	}
	if a.total != nil {
		if m := complTitledFloorsRe.FindStringSubmatch(strings.TrimSpace(a.clean(rawHead(a.total.r)))); m != nil {
			if name := list(m[4]); name != "" && a.titledTotal(name, ls) {
				for _, r := range a.floors {
					var texts []*types.ReqNode
					walkReq(r.r.Req, func(n *types.ReqNode) {
						if n.Type == types.NodeText {
							texts = append(texts, n)
						}
					})
					if r.r.Kind != types.RequirementConstraint || len(texts) > 1 {
						continue
					}
					text := ""
					if len(texts) == 1 {
						text = strings.TrimSuffix(texts[0].Text, ".")
					} else if q := r.r.Req; q.Type == types.NodeCredits && q.Lists == nil && strings.Contains(r.r.Raw, m[5]) && fmtAmount(q.Amount) == m[6] && slices.Equal(q.Levels, levels(m[7], false)) {
						text = m[5] // the grammar's level floor, of the whole program
					}
					switch text {
					case m[1]:
						r.r.Req = floor(m[2], name, levels(m[3], true))
					case m[5]:
						r.r.Req = floor(m[6], name, levels(m[7], false))
					}
				}
			}
		}
	}
	walkRows(a.items, func(r *compRow) {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText {
			return
		}
		m := complToIncludeTitledRe.FindStringSubmatch(strings.TrimSpace(n.Text))
		if m == nil || !strings.EqualFold(m[4], m[7]) {
			return
		}
		name := list(m[4])
		if name == "" {
			return
		}
		union, _, ok := a.addUnion(r, []*types.ReqNode{{Type: types.NodeCredits, Designators: []string{m[2]}}, {Type: types.NodeCredits, Lists: []string{name}}}, ls)
		if !ok {
			return
		}
		v, _ := strconv.ParseFloat(m[1], 64)
		r.r.Req = orNode(
			&types.ReqNode{Type: types.NodeCredits, Amount: v, Designators: []string{m[2]}},
			andNode(&types.ReqNode{Type: types.NodeCredits, Amount: v, Lists: []string{union}}, textNode(types.TextAmbiguity, n.Text)),
		)
		for _, f := range []*types.ReqNode{floor(m[3], name, levels(m[5], true)), floor(m[6], name, levels(m[8], false))} {
			a.rowUnder(r, &types.Requirement{Kind: types.RequirementConstraint, Req: f, Raw: r.r.Raw})
		}
	})
}

// titledTotal makes a total of one designator's credits the strict reading
// beside the lenient one that counts a titled list's courses too,
// and(credits{N, union}, text{ambiguity}).
func (a *compAssembler) titledTotal(name string, ls compListRefs) bool {
	n := a.total.r.Req
	if n.Type == types.NodeOr && len(n.Items) == 2 && n.Items[1].Type == types.NodeText {
		n = n.Items[0] // or(pool, the header): its marker
	}
	if n.Type != types.NodeCredits || len(n.Designators) != 1 || n.Levels != nil || n.Courses != nil || n.Lists != nil {
		return false
	}
	union, _, ok := a.addUnion(&compRow{r: &types.Requirement{Label: "1"}}, []*types.ReqNode{{Type: types.NodeCredits, Designators: slices.Clone(n.Designators)}, {Type: types.NodeCredits, Lists: []string{name}}}, ls)
	if !ok {
		return false
	}
	head := strings.TrimSuffix(strings.TrimSpace(a.clean(rawHead(a.total.r))), ".")
	a.total.r.Req = orNode(n, andNode(&types.ReqNode{Type: types.NodeCredits, Amount: n.Amount, Lists: []string{union}}, textNode(types.TextAmbiguity, head)))
	return true
}

// complLevelOrAnyRe matches "300-level CAS courses and/or 400-level
// courses". The groups are the two levels.
var complLevelOrAnyRe = regexp.MustCompile(`(?i)\b([1-4])00-level\s+` + desigPat + `\s+courses\s+and/or\s+([1-4])00-level\s+courses\b`)

// levelOrAnyLevel replaces the ambiguity text of or(credits{both levels},
// text{complLevelOrAnyRe}) with its lenient reading: credits from a union of
// the first level's designators and any course at the second.
func (a *compAssembler) levelOrAnyLevel(lists []*types.CourseList) []*types.CourseList {
	walkRows(a.items, func(r *compRow) {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr || len(n.Items) != 2 || r.r.Label == "" {
			return
		}
		pool, text := n.Items[0], n.Items[1]
		if !isPool(pool) || pool.Designators == nil || pool.Courses != nil || pool.Lists != nil || text.Type != types.NodeText || text.Kind != types.TextAmbiguity {
			return
		}
		m := complLevelOrAnyRe.FindStringSubmatch(text.Text)
		if m == nil {
			return
		}
		first, second := int(m[1][0]-'0')*100, int(m[2][0]-'0')*100
		if !slices.Equal(pool.Levels, []int{first, second}) {
			return
		}
		name := unionName(r, lists)
		lists = append(lists, &types.CourseList{Name: name, Items: []*types.ReqNode{
			{Type: types.NodeCredits, Designators: slices.Clone(pool.Designators), Levels: []int{first}},
			{Type: types.NodeCredits, Levels: []int{second}}}})
		n.Items[1] = andNode(&types.ReqNode{Type: types.NodeCredits, Amount: pool.Amount, Lists: []string{name}}, text)
	})
	return lists
}

var (
	// complLanguageCoursesRe matches the title of a program's list of language
	// courses: "_Language Courses_".
	complLanguageCoursesRe = regexp.MustCompile(`(?i)^language\s+courses$`)
	// complHigherLanguageRe matches "a higher level language course".
	complHigherLanguageRe = regexp.MustCompile(`(?i)^an?\s+higher[\s-]+level\s+language\s+course$`)
)

// languageCourses reads "language course" by the program's list of them: "a
// 400-level language course" is the list's 400-level courses, and "a higher
// level language course" beside listed ones is the list's courses above them.
func (a *compAssembler) languageCourses() {
	var lang []string
	for _, d := range a.listDefs {
		title := strings.TrimSuffix(strings.TrimSpace(strings.ReplaceAll(stripEmphasis(d.title), "_", "")), ":")
		if complLanguageCoursesRe.MatchString(strings.TrimSpace(title)) {
			lang = append(lang, codeRe.FindAllString(d.members, -1)...)
		}
	}
	if lang = dedupe(lang); lang == nil {
		return
	}
	pool := func(amount float64, levels []int) *types.ReqNode {
		var codes []string
		for _, c := range lang {
			if levelMatches(levels, c) {
				codes = append(codes, c)
			}
		}
		if codes == nil {
			return nil
		}
		return &types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: codes, Levels: slices.Clone(levels)}
	}
	var fix func(n *types.ReqNode)
	fix = func(n *types.ReqNode) {
		if n == nil {
			return
		}
		for i, it := range n.Items {
			switch {
			case it.Type == types.NodeAnd && len(it.Items) == 2 && it.Items[1].Type == types.NodeText && strings.EqualFold(strings.TrimSpace(it.Items[1].Text), "language course"):
				c := it.Items[0]
				if c.Type == types.NodeCredits && c.Levels != nil && c.Courses == nil && c.Designators == nil && c.Lists == nil && c.Breadth == nil {
					if p := pool(c.Amount, c.Levels); p != nil {
						n.Items[i] = p
					}
				}
			case it.Type == types.NodeText && n.Type == types.NodeOr && complHigherLanguageRe.MatchString(strings.TrimSpace(it.Text)):
				top, ok := 0, true
				for _, x := range n.Items {
					if x.Type == types.NodeCourse {
						ok = ok && slices.Contains(lang, x.Code)
						top = max(top, courseLevel(x.Code))
					}
				}
				var levels []int
				for l := top + 100; l <= 400; l += 100 {
					levels = append(levels, l)
				}
				if ok && top > 0 && levels != nil {
					if p := pool(0.5, levels); p != nil {
						n.Items[i] = p
					}
				}
			default:
				fix(it)
			}
		}
	}
	for _, r := range a.items {
		fix(r.r.Req)
	}
}

// complAsNecessaryRe matches an item holding the rest of the total: "(as
// necessary to total 4.0 credits in minor)" (ASMIN1333).
var complAsNecessaryRe = regexp.MustCompile(`(?i)\(as\s+necessary\s+to\s+total\s+` + amountPat + `\s+credits\b`)

// remainderTotalPool makes a complAsNecessaryRe item's list the total's pool
// when every other item's courses are in it. The total counts the rest from
// the same pool, so the range needs no marker.
func (a *compAssembler) remainderTotalPool(lists []*types.CourseList) {
	if a.total == nil || len(a.items) < 2 {
		return
	}
	total, last := a.total.r.Req, a.items[len(a.items)-1]
	m := complAsNecessaryRe.FindStringSubmatch(last.r.Raw)
	if total == nil || total.Type != types.NodeCredits || m == nil || total.Designators != nil || total.Courses != nil ||
		total.Levels != nil || total.Lists != nil || total.Breadth != nil || total.Exclude != nil || total.Requirements != nil ||
		last.r.Req == nil || !onlyLists(last.r.Req) || len(last.r.Req.Lists) != 1 {
		return
	}
	if v, _ := strconv.ParseFloat(m[1], 64); v != total.Amount {
		return
	}
	byName := listsByName(lists)
	union := byName[last.r.Req.Lists[0]]
	if union == nil {
		return
	}
	in := func(code string) bool {
		for _, it := range union.Items {
			switch {
			case it.Type == types.NodeCredits && it.Designators != nil && it.Courses == nil && it.Lists == nil && it.Exclude == nil && it.Breadth == nil:
				if filterMatches(it, code) {
					return true
				}
			case onlyListsItem(it):
				if codes, ok := a.listCodes(it.Lists, byName); ok && slices.Contains(codes, code) {
					return true
				}
			}
		}
		return false
	}
	desigIn := func(d string) bool {
		return slices.ContainsFunc(union.Items, func(it *types.ReqNode) bool {
			return it.Type == types.NodeCredits && slices.Contains(it.Designators, d) && it.Levels == nil && it.Courses == nil &&
				it.Lists == nil && it.Exclude == nil && it.Breadth == nil
		})
	}
	var covered func(n *types.ReqNode) bool
	covered = func(n *types.ReqNode) bool {
		switch n.Type {
		case types.NodeCourse:
			return in(n.Code)
		case types.NodeAnd, types.NodeOr:
			return !slices.ContainsFunc(n.Items, func(x *types.ReqNode) bool { return !covered(x) })
		case types.NodeCredits:
			if n.Lists != nil || n.Requirements != nil || n.Breadth != nil || (n.Courses == nil && n.Designators == nil) {
				return false
			}
			return !slices.ContainsFunc(n.Courses, func(c string) bool { return !in(c) }) &&
				!slices.ContainsFunc(n.Designators, func(d string) bool { return !desigIn(d) })
		}
		return false
	}
	for _, r := range a.items[:len(a.items)-1] {
		if r.r.Kind == types.RequirementAllocation && (r.r.Req == nil || !covered(r.r.Req)) {
			return
		}
	}
	total.Lists = slices.Clone(last.r.Req.Lists)
}

// onlyListsItem reports whether a list item is lists and nothing else.
func onlyListsItem(n *types.ReqNode) bool {
	return n.Type == types.NodeCredits && n.Lists != nil && n.Courses == nil && n.Designators == nil && n.Levels == nil &&
		n.Breadth == nil && n.Requirements == nil && n.Exclude == nil && n.Items == nil && n.Weight == 0
}

// namedAndBelow reads complNamedAndBelowRe as credits from a union of the
// courses another item lists "from the <word> courses:" and the lists below.
func (a *compAssembler) namedAndBelow(row *compRow, ls compListRefs) bool {
	m := complNamedAndBelowRe.FindStringSubmatch(strings.TrimSuffix(complClean(row.r.Req.Text), "."))
	if m == nil {
		return false
	}
	lead := "from the " + strings.ToLower(m[2]) + " courses:"
	var codes []string
	for _, r := range a.allRows() {
		if n := r.r.Req; r != row && n != nil && n.Type == types.NodeCredits && n.Courses != nil && strings.Contains(strings.ToLower(r.r.Raw), lead) {
			if codes != nil {
				return false // two such items
			}
			codes = n.Courses
		}
	}
	names := a.listsBelow(row, ls)
	if codes == nil || names == nil {
		return false
	}
	name, _, ok := a.addUnion(row, []*types.ReqNode{
		{Type: types.NodeCredits, Courses: slices.Clone(codes)},
		{Type: types.NodeCredits, Lists: names},
	}, ls)
	if !ok {
		return false
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	row.r.Req = &types.ReqNode{Type: types.NodeCredits, Amount: amount, Lists: []string{name}}
	return true
}

// remainingBelow reads complRemainingBelowRe as the pool of the one list
// below, of the amount complRemainingAmountRe gives.
func (a *compAssembler) remainingBelow(row *compRow, text *types.ReqNode, alts []*types.ReqNode, ls compListRefs) bool {
	if !complRemainingBelowRe.MatchString(strings.TrimSuffix(complClean(text.Text), ".")) {
		return false
	}
	var amount float64
	notes := slices.Clone(a.notes)
	for _, r := range a.allRows() {
		notes = append(notes, r.r.Notes...)
	}
	for _, n := range notes {
		if m := complRemainingAmountRe.FindStringSubmatch(complClean(n)); m != nil {
			if amount != 0 {
				return false
			}
			amount, _ = strconv.ParseFloat(m[1], 64)
		}
	}
	names := a.listsBelow(row, ls)
	if amount == 0 || len(names) != 1 {
		return false
	}
	row.r.Req = orNode(append([]*types.ReqNode{{Type: types.NodeCredits, Amount: amount, Lists: names}}, alts...)...)
	return true
}

// coreParts reads complCoreLeadRe over parts whose amounts make the lead's:
// the lead is the pool of every part's courses, each part an allocation under
// it. A part over categories gets a constraint of each category's amount.
func (a *compAssembler) coreParts(row *compRow, ls compListRefs) bool {
	m := complCoreLeadRe.FindStringSubmatch(complClean(row.r.Req.Text))
	if m == nil || len(row.children) == 0 {
		return false
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	var kids []*compRow
	var all []string
	sum := 0.0
	var used []int // the notes the categories were
	for _, k := range row.children {
		n := k.r.Req
		if k.r.Kind != types.RequirementConstraint || n == nil || n.Type != types.NodeText || len(k.children) > 0 {
			return false
		}
		t := complClean(n.Text)
		alloc := &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Section: k.r.Section, Raw: k.r.Raw}}
		if p := complCorePartRe.FindStringSubmatch(t); p != nil {
			v, _ := strconv.ParseFloat(p[1], 64)
			list := complCodeTitledRe.ReplaceAllString(p[2], "$1")
			if !complCodeRunRe.MatchString(list) {
				return false
			}
			codes := codeRe.FindAllString(list, -1)
			if weightOf(codes) < v-1e-9 {
				return false
			}
			alloc.r.Req = &types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: dedupe(codes)}
			for _, g := range strings.Split(list, ",") {
				alts := codeRe.FindAllString(g, -1)
				if most := heaviestCourse(alts); len(alts) > 1 && most < v-1e-9 {
					alloc.children = append(alloc.children, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Section: k.r.Section, Raw: k.r.Raw,
						Req: &types.ReqNode{Type: types.NodeCredits, Courses: alts, MaxAmount: maxAmount(most)}}})
				}
			}
			kids, all, sum = append(kids, alloc), append(all, codes...), sum+v
			continue
		}
		p := complCategoriesPartRe.FindStringSubmatch(t)
		if p == nil {
			return false
		}
		v, _ := strconv.ParseFloat(p[1], 64)
		each, _ := strconv.ParseFloat(p[2], 64)
		var parts []*types.ReqNode
		var codes []string
		for i, note := range row.r.Notes {
			if c := complCategoryLineRe.FindStringSubmatch(complClean(note)); c != nil && !slices.Contains(used, i) {
				cs := codeRe.FindAllString(c[1], -1)
				parts = append(parts, &types.ReqNode{Type: types.NodeCredits, Amount: each, Courses: cs})
				codes = append(codes, cs...)
				used = append(used, i)
				addRaw(alloc, note)
			}
		}
		if len(parts) < 2 || math.Abs(each*float64(len(parts))-v) > 1e-9 {
			return false
		}
		floor, ok := partsNode(len(parts), parts, ls)
		if !ok {
			return false
		}
		alloc.r.Req = &types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: dedupe(codes)}
		alloc.children = []*compRow{{r: &types.Requirement{Kind: types.RequirementConstraint, Section: k.r.Section, Raw: alloc.r.Raw, Req: floor}}}
		kids, all, sum = append(kids, alloc), append(all, codes...), sum+v
	}
	if math.Abs(sum-amount) > 1e-9 {
		return false
	}
	row.r.Req = &types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: dedupe(all)}
	row.children = kids
	var notes []string
	for i, n := range row.r.Notes {
		if !slices.Contains(used, i) {
			notes = append(notes, n)
		}
	}
	row.r.Notes = notes
	return true
}

// complRemainingFromRe matches an item making up the total from a pool: "The
// remaining courses should be taken from: GER361H1, ..., or any course offered
// by and cross-listed by the Centre for Jewish Studies".
var complRemainingFromRe = regexp.MustCompile(`(?i)^the\s+remaining\s+courses\s+should\s+be\s+taken\s+from:?\s*((?:` + codePat + `)(?:\s*,\s*(?:` + codePat + `))*),?\s+or\s+(any\s+course\s+offered\s+by\s+and\s+cross-listed\s+by\s+the\s+[\w\s]+)$`)

// remainingFrom reads a complRemainingFromRe item as or(credits{rest, the
// codes}, and(credits{rest}, text{content})), rest being the total less the
// most the other items take. The catalogue doesn't record a unit's courses.
func (a *compAssembler) remainingFrom() {
	row, m, amount := a.remainingFromItem()
	if row == nil {
		return
	}
	codes := dedupe(codeRe.FindAllString(m[1], -1))
	row.r.Req = orNode(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: codes},
		andNode(&types.ReqNode{Type: types.NodeCredits, Amount: amount}, textNode(types.TextContent, m[2])))
}

// remainingFromItem returns remainingFrom's item, its complRemainingFromRe
// match and its amount, or a nil row.
func (a *compAssembler) remainingFromItem() (*compRow, []string, float64) {
	if a.total == nil {
		return nil, nil, 0
	}
	tot := a.total.r.Req
	if tot == nil || tot.Type != types.NodeCredits || tot.Amount <= 0 || len(tot.Courses) > 0 || len(tot.Designators) > 0 || len(tot.Lists) > 0 {
		return nil, nil, 0
	}
	var row *compRow
	var m []string
	taken := 0.0
	for _, r := range a.items {
		n := r.r.Req
		if n != nil && n.Type == types.NodeText {
			if mm := complRemainingFromRe.FindStringSubmatch(strings.TrimSuffix(complClean(n.Text), ".")); mm != nil && row == nil && r.r.Kind == types.RequirementAllocation {
				row, m = r, mm
				continue
			}
		}
		if n == nil || r.r.Kind != types.RequirementAllocation || maxCredits(n) <= 0 {
			return nil, nil, 0
		}
		if _, ok := minCredits(n); !ok {
			return nil, nil, 0
		}
		taken += maxCredits(n)
	}
	amount := math.Round((tot.Amount-taken)*100) / 100
	if row == nil || amount <= 0 {
		return nil, nil, 0
	}
	return row, m, amount
}

// listAnyLevel reads complListAnyLevelRe as credits from a list of the row's
// own, of the courses and the designator's at the level, with the floor and
// the "/" caps under it. It returns the list, or nil.
func (a *compAssembler) listAnyLevel(r *compRow, lists []*types.CourseList) *types.CourseList {
	n := r.r.Req
	if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || r.r.Label == "" {
		return nil
	}
	m := complListAnyLevelRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(complClean(n.Text)), "."))
	if m == nil {
		return nil
	}
	rest := complWhicheverRe.ReplaceAllString(m[4], "")
	anyLoc := complAnyLevelCourseRe.FindStringSubmatchIndex(rest)
	if anyLoc == nil {
		return nil
	}
	level, desig := rest[anyLoc[2]:anyLoc[3]], rest[anyLoc[4]:anyLoc[5]]
	rest = strings.TrimSpace(rest[:anyLoc[0]] + ", " + rest[anyLoc[1]:])
	rest = strings.Trim(rest, ",; ")
	v, _ := strconv.ParseFloat(m[1], 64)
	floor, _ := strconv.ParseFloat(m[2], 64)
	pool, caps := codesCredits(m[1], rest)
	name := "Requirement " + r.r.Label + " courses"
	if pool == nil || floor >= v || slices.ContainsFunc(lists, func(l *types.CourseList) bool { return l.Name == name }) {
		return nil
	}
	l := &types.CourseList{Name: name, Items: []*types.ReqNode{
		{Type: types.NodeCredits, Courses: pool.Courses},
		{Type: types.NodeCredits, Designators: []string{desig}, Levels: []int{int(level[0]-'0') * 100}},
	}}
	r.r.Req = &types.ReqNode{Type: types.NodeCredits, Amount: v, Lists: []string{name}}
	kids := []*compRow{{r: &types.Requirement{Kind: types.RequirementConstraint, Section: r.r.Section, Raw: rawHead(r.r),
		Req: &types.ReqNode{Type: types.NodeCredits, Amount: floor, Levels: parseLevels(m[3] + "00-level")}}}}
	for _, c := range caps {
		c.Section = r.r.Section
		kids = append(kids, &compRow{r: c})
	}
	r.children = append(kids, r.children...)
	return l
}

// complSeriesFromRe matches a list lead: "2.0 credits from: ANT100Y1, ...".
var complSeriesFromRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+from:\s*(.+)$`)

// seriesList reads "N credits from: <courses and series>" as credits from a
// list of its own, with the "/" choices capped. A code with no campus makes
// the sentence a bounded alternative. It returns the list, or nil.
func (a *compAssembler) seriesList(r *compRow, lists []*types.CourseList) *types.CourseList {
	n := r.r.Req
	if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || r.r.Label == "" || len(r.children) > 0 {
		return nil
	}
	m := complSeriesFromRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(complClean(n.Text)), "."))
	name := "Requirement " + r.r.Label + " courses"
	if m == nil || !complSeriesRe.MatchString(m[2]) || slices.ContainsFunc(lists, func(l *types.CourseList) bool { return l.Name == name }) {
		return nil
	}
	var items []*types.ReqNode
	for _, g := range complSeriesRe.FindAllStringSubmatch(m[2], -1) {
		items = append(items, &types.ReqNode{Type: types.NodeCredits, Designators: []string{g[1]}, Levels: []int{int(g[2][0]-'0') * 100}, Exclude: codeRe.FindAllString(g[3], -1)})
	}
	rest := complSeriesRe.ReplaceAllString(m[2], "")
	partial := false
	var kept []string
	for _, part := range complListSepRe.Split(rest, -1) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !complCodeChoiceRe.MatchString(part) {
			return nil
		}
		codes := codeRe.FindAllString(part, -1)
		partial = partial || len(codes) < len(strings.Split(part, "/"))
		if codes != nil {
			kept = append(kept, strings.Join(codes, "/ "))
		}
	}
	pool, caps := codesCredits(m[1], strings.Join(kept, ", "))
	if pool == nil {
		return nil
	}
	l := &types.CourseList{Name: name, Items: append([]*types.ReqNode{{Type: types.NodeCredits, Courses: pool.Courses}}, items...)}
	body := &types.ReqNode{Type: types.NodeCredits, Amount: pool.Amount, Lists: []string{name}}
	r.r.Req = body
	if partial {
		r.r.Req = orNode(body, andNode(&types.ReqNode{Type: types.NodeCredits, Amount: pool.Amount}, textNode(types.TextOther, n.Text)))
	}
	for _, c := range caps {
		c.Section = r.r.Section
		r.children = append(r.children, &compRow{r: c})
	}
	return l
}
