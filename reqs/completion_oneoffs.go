package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// More one-off fixes (Task 10F4, the item-20 one-offs): wordings one or two
// programs use, read where no wider rule covers them. The hooks are in
// completion_fixes.go (oneOffRow, oneOffs, rewriteOneOffs) and listRefRow.

// complExclusionAltsRe: an item whose alternatives are its course's
// exclusions: "POL222H1 (or one of its exclusions: ECO220Y1/ PSY201H1/
// SOC202H1/ STA220H1; only 0.5 credit of ECO220Y1 can count toward this
// Focus)", "POL232H1 (or its exclusion: GGR270H1)". Groups: the course, the
// exclusions, the full-year course only half of which counts.
var complExclusionAltsRe = regexp.MustCompile(`(?i)^(` + codePat + `)\s*\(\s*or\s+(?:one\s+of\s+)?its\s+exclusions?:\s*((?:` + codePat + `\s*[/,]?\s*)+?)(?:;\s*only\s+(?:a\s+)?0\.5\s+credit\s+of\s+(` + codePat + `)\s+(?:can|may|will)\s+count\b[^()]*)?\)\.?$`)

// exclusionAlts reads a complExclusionAltsRe item: the or of the course and
// its exclusions; the full-year course only half of which counts is
// credits{0.5, [it], weight 0.5} (design 2, weight), so its other half may
// count elsewhere and a plain course node doesn't claim all of it.
func exclusionAlts(text string) *types.ReqNode {
	m := complExclusionAltsRe.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil || courseWeight(m[1]) != 0.5 {
		return nil
	}
	alts := []*types.ReqNode{{Type: types.NodeCourse, Code: m[1]}}
	for _, c := range dedupe(codeRe.FindAllString(m[2], -1)) {
		if c == m[3] && courseWeight(c) == 1 {
			alts = append(alts, &types.ReqNode{Type: types.NodeCredits, Amount: 0.5, Courses: []string{c}, Weight: 0.5})
			continue
		}
		alts = append(alts, &types.ReqNode{Type: types.NodeCourse, Code: c})
	}
	if m[3] != "" && !slices.Contains(codeRe.FindAllString(m[2], -1), m[3]) {
		return nil
	}
	return orNode(alts...)
}

var (
	// complFromLabelLeadRe: "From Group A: 1.0 credit" and "From Group A:
	// 1.0 credit or DRM220Y1": the grammar's order, "1.0 credit from Group A
	// [or DRM220Y1]".
	complFromLabelLeadRe = regexp.MustCompile(`(?i)^from\s+(group\s+[A-Z]):\s*(\d+(?:\.\d+)?\s+credits?)(\s+or\s+` + codePat + `)?$`)
	// complWhenEligibleRe: "or other DRM (when eligible)": eligibility to
	// enrol (Group B's studio courses give Drama students priority), which a
	// completed course has met.
	complWhenEligibleRe = regexp.MustCompile(`(?i)(\bother\s+[A-Z]{3})\s+\(when\s+eligible\)`)
)

// rewriteListOneOffs rewrites wordings of items that draw on lists into the
// list grammar's (listRefRow, before listRefAlloc).
func rewriteListOneOffs(text string) string {
	text = complFromLabelLeadRe.ReplaceAllString(strings.TrimSpace(text), "$2 from $1$3")
	return complWhenEligibleRe.ReplaceAllString(text, "$1")
}

// moreOneOffs runs the one-off readings of this file that need the routes
// placed and the lists read (oneOffs). It returns the lists with any it
// adds.
func (a *compAssembler) moreOneOffs(lists []*types.CourseList) []*types.CourseList {
	a.splitAlternatives()
	a.sharedEssay()
	a.moreOneOffPasses(lists)
	a.remainderTotalPool(lists)
	lists = a.coreGroupsMix(lists)
	lists = a.levelOrAnyLevel(lists)
	return a.totalCourseNote(lists)
}

// coreGroupsMix reads the item coreOrGroups (completion_residue.go) made
// or(credits{N, core courses}, credits{N, lists}) from "N credits from the
// Core Group, or N credits from Groups A, B, C, D" by the ambiguity policy:
// the strict reading (all N from one side) is Met, and the lenient one (N
// from both, mixed) is Review beside it, and(credits{N, union},
// text{ambiguity}), with the union a list of the row's own.
func (a *compAssembler) coreGroupsMix(lists []*types.CourseList) []*types.CourseList {
	walkRows(a.items, func(r *compRow) {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr || len(n.Items) != 2 || r.r.Label == "" ||
			!complCoreOrGroupsRe.MatchString(strings.TrimSuffix(complClean(complItemLabelRe.ReplaceAllString(rawHead(r.r), "")), ".")) {
			return
		}
		core, groups := n.Items[0], n.Items[1]
		if !isPool(core) || groups.Type != types.NodeCredits || core.Amount != groups.Amount || core.Courses == nil || core.Lists != nil ||
			groups.Lists == nil || groups.Courses != nil || core.Designators != nil || groups.Designators != nil {
			return
		}
		name := unionName(r, lists)
		lists = append(lists, &types.CourseList{Name: name, Items: []*types.ReqNode{
			{Type: types.NodeCredits, Courses: slices.Clone(core.Courses)},
			{Type: types.NodeCredits, Lists: slices.Clone(groups.Lists)}}})
		lenient := andNode(&types.ReqNode{Type: types.NodeCredits, Amount: core.Amount, Lists: []string{name}},
			textNode(types.TextAmbiguity, "the credits may be drawn from the Core Group and the Groups together"))
		r.r.Req = orNode(core, groups, lenient)
	})
	return lists
}

// complHalfFromAboveRe: an alternative that is half of a full-year course
// an item above lists: "Astronomy: AST430H1, or AST425Y1 (0.5 credit from
// point 1 above)", beside AST430H1 in the or. Groups: the course, the point.
var complHalfFromAboveRe = regexp.MustCompile(`(?i)^(?:[\w\s&]+:\s*)?(?:` + codePat + `,?\s+or\s+)?(` + codePat + `)\s+\(0\.5\s+credit\s+from\s+point\s+(\d{1,2})\s+above\)\.?$`)

// splitAlternatives reads a complHalfFromAboveRe alternative as the half of
// the course the point above leaves (design 2, weight): credits{0.5, [it],
// weight 0.5}, when the point's pool lists the course and the or holds the
// other courses the line names. The point's pool takes the other half the
// same way: unweighted, it would claim all of the course (only a weight
// match splits one) and leave this row NotMet. credits{0.5, [AST424H1,
// AST425Y1]} becomes or(credits{0.5, [AST424H1]}, credits{0.5, [AST425Y1],
// weight 0.5}).
func (a *compAssembler) splitAlternatives() {
	walkRows(a.items, func(r *compRow) {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr {
			return
		}
		for i, it := range n.Items {
			if it.Type != types.NodeText {
				continue
			}
			m := complHalfFromAboveRe.FindStringSubmatch(strings.TrimSpace(it.Text))
			if m == nil || courseWeight(m[1]) != 1 {
				continue
			}
			q := a.pointRow(r, m[2], m[1])
			if q == nil {
				continue
			}
			for _, c := range codeRe.FindAllString(it.Text, -1) {
				if c != m[1] && !slices.ContainsFunc(n.Items, func(x *types.ReqNode) bool { return x.Type == types.NodeCourse && x.Code == c }) {
					return
				}
			}
			n.Items[i] = &types.ReqNode{Type: types.NodeCredits, Amount: 0.5, Courses: []string{m[1]}, Weight: 0.5}
			half := &types.ReqNode{Type: types.NodeCredits, Amount: 0.5, Courses: []string{m[1]}, Weight: 0.5}
			if rest := slices.DeleteFunc(slices.Clone(q.r.Req.Courses), func(c string) bool { return c == m[1] }); len(rest) > 0 {
				p := cloneNode(q.r.Req)
				p.Courses = rest
				q.r.Req = orNode(p, half)
			} else {
				q.r.Req = half
			}
		}
	})
}

// pointRow returns the sibling of r labelled like r with the number point
// ("Third or fourth year 1" for r's "Third or fourth year 3") when it is a
// pool of 0.5 credit from courses that lists code, else nil.
func (a *compAssembler) pointRow(r *compRow, point, code string) *compRow {
	label := r.r.Label
	if i := strings.LastIndexByte(label, ' '); i >= 0 {
		label = label[:i+1] + point
	} else {
		label = point
	}
	var found *compRow
	walkRows(a.items, func(q *compRow) {
		n := q.r.Req
		if q.r.Label == label && q.r.Section == r.r.Section && n != nil && n.Type == types.NodeCredits && n.Amount == 0.5 && n.Weight == 0 &&
			slices.Contains(n.Courses, code) && n.Designators == nil && n.Levels == nil && n.Lists == nil && n.Requirements == nil {
			found = q
		}
	})
	return found
}

// complSharedEssayRe: one full-year course that stands in for two
// half-credit requirements: "Students may also satisfy requirements (4)
// and (5) through completion of a senior essay ( FRE490Y1)". Groups: the
// two labels, the course.
var complSharedEssayRe = regexp.MustCompile(`(?i)^students\s+may\s+also\s+satisfy\s+requirements\s+\((\d{1,2})\)\s+and\s+\((\d{1,2})\)\s+through\s+(?:the\s+)?completion\s+of\s+[^()]*\(\s*(` + codePat + `)\s*\)\.?$`)

// sharedEssay reads a complSharedEssayRe alternative, on both rows it names
// (placeProgramRoutes), as half of the course in each (design 2, weight,
// as splitYAlternatives): credits{0.5, [it], weight 0.5}. Both rows must
// need 0.5 credit and hold the sentence as their alternative.
func (a *compAssembler) sharedEssay() {
	var rows []*compRow
	var m []string
	walkRows(a.items, a.fromListsIn)
	walkRows(a.items, func(r *compRow) {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr || len(n.Items) != 2 || n.Items[1].Type != types.NodeText {
			return
		}
		if g := complSharedEssayRe.FindStringSubmatch(n.Items[1].Text); g != nil && (r.r.Label == g[1] || r.r.Label == g[2]) {
			if v, ok := nodeCredits(n.Items[0]); ok && v == 0.5 && courseWeight(g[3]) == 1 {
				rows, m = append(rows, r), g
			}
		}
	})
	if len(rows) != 2 || rows[0].r.Label == rows[1].r.Label {
		return
	}
	for _, r := range rows {
		r.r.Req.Items[1] = &types.ReqNode{Type: types.NodeCredits, Amount: 0.5, Courses: []string{m[3]}, Weight: 0.5}
	}
}

// complFromListsInRe: "0.5 additional credit from the list of courses in
// (3) or (4)". Groups: the amount, the two labels.
var complFromListsInRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+additional\s+credits?\s+from\s+the\s+list\s+of\s+courses\s+in\s+\((\d{1,2})\)\s+or\s+\((\d{1,2})\)\.?$`)

// fromListsIn reads an or whose first item is complFromListsInRe text as
// the pools of the two items it names: credits{amount, requirements: [their
// rows]} (numberReqRefs). "Additional" is the claims rule.
func (a *compAssembler) fromListsIn(r *compRow) {
	n := r.r.Req
	if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr || n.Items[0].Type != types.NodeText {
		return
	}
	m := complFromListsInRe.FindStringSubmatch(strings.TrimSpace(n.Items[0].Text))
	if m == nil || m[2] == m[3] {
		return
	}
	var rows []*compRow
	for _, l := range m[2:4] {
		q := a.labelled(l)
		if q == nil || q == r || q.r.Kind != types.RequirementAllocation {
			return
		}
		rows = append(rows, q)
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	ref := &types.ReqNode{Type: types.NodeCredits, Amount: amount}
	a.reqRefs = append(a.reqRefs, compReqRef{node: ref, rows: rows})
	n.Items[0] = ref
}

// complCanBeUsedNoteRe: "Note: CHM395Y1 can be used toward this
// requirement", the total's alternative when the course is outside its
// pool's levels (the "Additional ... to a total of" fold).
var complCanBeUsedNoteRe = regexp.MustCompile(`(?i)^(?:note:\s*)?(` + codePat + `)\s+can\s+be\s+used\s+towards?\s+this\s+requirement\.?$`)

// totalCourseNote reads row 1's or(pool, text{complCanBeUsedNoteRe}) as one
// pool: a list of row 1's own (design 3.2), "Requirement 1 courses", the
// pool's filter and the course. It returns the lists with it.
func (a *compAssembler) totalCourseNote(lists []*types.CourseList) []*types.CourseList {
	if a.total == nil {
		return lists
	}
	n := a.total.r.Req
	if n == nil || n.Type != types.NodeOr || len(n.Items) != 2 || n.Items[1].Type != types.NodeText {
		return lists
	}
	m := complCanBeUsedNoteRe.FindStringSubmatch(strings.TrimSpace(n.Items[1].Text))
	pool := n.Items[0]
	if m == nil || pool.Type != types.NodeCredits || pool.Lists != nil || pool.Designators == nil && pool.Levels == nil {
		return lists
	}
	const name = "Requirement 1 courses"
	if slices.ContainsFunc(lists, func(l *types.CourseList) bool { return l.Name == name }) {
		return lists
	}
	filter := &types.ReqNode{Type: types.NodeCredits, Designators: pool.Designators, Levels: pool.Levels, Courses: pool.Courses, Exclude: pool.Exclude, MaxCourseAmount: pool.MaxCourseAmount}
	lists = append(lists, &types.CourseList{Name: name, Items: []*types.ReqNode{filter, {Type: types.NodeCredits, Courses: []string{m[1]}}}})
	a.total.r.Req = &types.ReqNode{Type: types.NodeCredits, Amount: pool.Amount, Lists: []string{name}}
	return lists
}

// walkRows calls f on every row of rs and their children.
func walkRows(rs []*compRow, f func(r *compRow)) {
	for _, r := range rs {
		f(r)
		walkRows(r.children, f)
	}
}

// moreOneOffRow reads, in oneOffRow with the lists known, an allocation of
// text (text: its text; alts: the routes beside it) by the readers below.
// ok: it read the row.
func (a *compAssembler) moreOneOffRow(row *compRow, text *types.ReqNode, alts []*types.ReqNode, ls compListRefs) bool {
	s := strings.TrimSuffix(strings.TrimSpace(text.Text), ".")
	set := func(n *types.ReqNode) {
		row.r.Req = orNode(append([]*types.ReqNode{n}, alts...)...)
	}
	if n := exclusionAlts(s); n != nil && alts == nil {
		set(n)
		return true
	}
	if n := worldMusicAlt(s); n != nil {
		set(n)
		return true
	}
	if n := groupSubgroups(s, ls); n != nil {
		set(n)
		return true
	}
	if n, floor := furtherFloor(s); n != nil {
		set(n)
		k := &types.Requirement{Kind: types.RequirementConstraint, Req: orNode(append([]*types.ReqNode{floor}, alts...)...), Raw: row.r.Raw}
		a.rowUnder(row, k)
		return true
	}
	if n := a.listAbove(row, s); n != nil {
		set(n)
		return true
	}
	if n := researchAlts(s); n != nil {
		set(n)
		row.r.Notes = slices.DeleteFunc(row.r.Notes, func(x string) bool { return strings.Contains(s, strings.TrimSuffix(x, ".")) })
		return true
	}
	if n := averageAlts(s); n != nil {
		set(n)
		return true
	}
	return false
}

// complWorldMusicRe: "MUS200H1 or an alternative 200-level 0.5 credit in
// world music ( MUS209H1, MUS211H1, MUS212H1, or MUS215H1)": the course,
// or 0.5 credit from the bracket's courses. Groups: the course, the level,
// the amount, the bracket.
var complWorldMusicRe = regexp.MustCompile(`(?i)^(` + codePat + `)\s+or\s+an\s+alternative\s+([1-4])00-level\s+(\d+(?:\.\d+)?)\s+credits?\s+in\s+[\w\s]+?\(\s*((?:` + codePat + `,?\s*(?:or\s+)?)+)\)$`)

// worldMusicAlt reads a complWorldMusicRe item; nil when a bracketed course
// isn't at the level.
func worldMusicAlt(s string) *types.ReqNode {
	m := complWorldMusicRe.FindStringSubmatch(s)
	if m == nil {
		return nil
	}
	codes := codeRe.FindAllString(m[4], -1)
	level := int(m[2][0]-'0') * 100
	if slices.ContainsFunc(codes, func(c string) bool { return courseLevel(c) != level }) {
		return nil
	}
	v, _ := strconv.ParseFloat(m[3], 64)
	return orNode(&types.ReqNode{Type: types.NodeCourse, Code: m[1]}, &types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: codes})
}

// complGroupSubgroupsRe: "ANT200Y1/ ARH100Y1/ ( ANT204H1 and ANT207H1/0.5
// credit 300-level Social Anthropology course from Anthropology Group C, or
// Subgroup C(i), C(ii))". Groups: the full-year courses, the first half
// course, the second, the level, the group's letter, the sub-groups'
// numerals.
var complGroupSubgroupsRe = regexp.MustCompile(`^((?:` + codePat + `\s*/\s*)+)\(\s*(` + codePat + `)\s+and\s+(` + codePat + `)\s*/\s*0\.5\s+credit\s+([1-4])00-level\s+[\w\s]+?\s+course\s+from\s+[\w\s]*?Group\s+([A-Z]),\s+or\s+Subgroup\s+[A-Z]\s*\((i{1,3})\),\s*[A-Z]\s*\((i{1,3})\)\s*\)$`)

// groupSubgroups reads a complGroupSubgroupsRe item: or(the full-year
// courses, and(the first half course, or(the second, 0.5 credit at the
// level from the group and its sub-groups))); nil when a list isn't there.
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
	var alts []*types.ReqNode
	for _, c := range codeRe.FindAllString(m[1], -1) {
		alts = append(alts, &types.ReqNode{Type: types.NodeCourse, Code: c})
	}
	level := int(m[4][0]-'0') * 100
	half := orNode(&types.ReqNode{Type: types.NodeCourse, Code: m[3]}, &types.ReqNode{Type: types.NodeCredits, Amount: 0.5, Levels: []int{level}, Lists: names})
	return orNode(append(alts, andNode(&types.ReqNode{Type: types.NodeCourse, Code: m[2]}, half))...)
}

// complFurtherFloorRe: "Advanced Topics: 4.0 credits of further APM/MAT
// 300+ level courses including at least 2.5 credits of APM/MAT courses at
// the 400-level". Groups: the amount, the designators, the level, the
// floor, the floor's designators, its level.
var complFurtherFloorRe = regexp.MustCompile(`^(?:[A-Z][\w\s]*:\s*)?(\d+(?:\.\d+)?)\s+credits\s+of\s+further\s+([A-Z]{3}(?:/[A-Z]{3})*)\s+([1-4])00\+\s+level\s+courses\s+including\s+at\s+least\s+(\d+(?:\.\d+)?)\s+credits\s+of\s+([A-Z]{3}(?:/[A-Z]{3})*)\s+courses\s+at\s+the\s+([1-4])00-level$`)

// furtherFloor reads a complFurtherFloorRe item: the pool, and the floor
// under it (a constraint).
func furtherFloor(s string) (pool, floor *types.ReqNode) {
	m := complFurtherFloorRe.FindStringSubmatch(s)
	if m == nil || m[2] != m[5] || m[6] < m[3] {
		return nil, nil
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	f, _ := strconv.ParseFloat(m[4], 64)
	designators := strings.Split(m[2], "/")
	var levels []int
	for l := int(m[3][0]-'0') * 100; l <= 400; l += 100 {
		levels = append(levels, l)
	}
	pool = &types.ReqNode{Type: types.NodeCredits, Amount: v, Designators: designators, Levels: levels}
	floor = &types.ReqNode{Type: types.NodeCredits, Amount: f, Designators: slices.Clone(designators), Levels: []int{int(m[6][0]-'0') * 100}}
	return pool, floor
}

// complListAboveRe: "6.0 credits from the list above and/or SMC165H1/
// MST226H1/ ...": the list above and the codes. Groups: the amount, the
// codes.
var complListAboveRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+from\s+the\s+list\s+above\s+and/or\s+((?:` + codePat + `\s*/?\s*)+)$`)

// listAbove reads a complListAboveRe item as one pool of its codes and the
// list above it: the one item before it under the same parent whose pool is
// courses from "the following" list. Nil when there is none or more.
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

// complResearchAltsRe: "( ENV421Y1/ ENV421H1)/ ENV491Y1*/ ENV492H1*/
// ENV493H1* or a minimum of 0.5 credit from any approved departmental or
// college independent research project (*the research topic for these
// courses must be related to environment and health)". Groups: the
// courses, the approved route, the starred courses' condition.
var complResearchAltsRe = regexp.MustCompile(`(?i)^((?:\(?\s*` + codePat + `\*?\s*\)?\s*/\s*)*\(?\s*` + codePat + `\*?\s*\)?)\s+or\s+(a\s+minimum\s+of\s+[^()]*?\bapproved\b[^()]*?)\s*\(\s*\*\s*([^()]+?)\s*\)$`)

// researchAlts reads a complResearchAltsRe item: the plain courses, the
// starred ones with their condition (a content text: the topic decides)
// and the approved route (permission).
func researchAlts(s string) *types.ReqNode {
	m := complResearchAltsRe.FindStringSubmatch(s)
	if m == nil {
		return nil
	}
	var alts, starred []*types.ReqNode
	for _, loc := range codeRe.FindAllStringIndex(m[1], -1) {
		n := &types.ReqNode{Type: types.NodeCourse, Code: m[1][loc[0]:loc[1]]}
		if loc[1] < len(m[1]) && m[1][loc[1]] == '*' {
			starred = append(starred, n)
		} else {
			alts = append(alts, n)
		}
	}
	if alts == nil || starred == nil {
		return nil
	}
	alts = append(alts, andNode(orNode(starred...), textNode(types.TextContent, m[3])))
	return orNode(append(alts, textNode(types.TextPermission, m[2]))...)
}

// averageAlts reads an item of alternatives each with its own average
// ("( MAT130H1/ MAT135H1, MAT136H1) with an average grade of 73%/ (
// MAT148H1, MAT149H1)/ MAT137Y1 with an average grade of 63%/ ..."), as
// the enrolment grammar does (itemAvgRe): each segment up to an average is
// a group of alternatives averaged on its own, "/" between them. Nil when a
// segment isn't courses alone.
func averageAlts(s string) *types.ReqNode {
	locs := itemAvgRe.FindAllStringSubmatchIndex(s, -1)
	if len(locs) < 2 {
		return nil
	}
	var alts []*types.ReqNode
	last := 0
	for _, m := range locs {
		grade, _ := strconv.Atoi(subOr(s, m, 2) + subOr(s, m, 4))
		seg := strings.TrimSpace(strings.TrimLeft(s[last:m[0]], " /"))
		n, _ := parseCompletionText(seg)
		if n == nil || !fixedOrCourses(n) {
			return nil
		}
		if n.Type == types.NodeOr {
			// "( MAT148H1, MAT149H1)/ MAT137Y1 with an average grade of
			// 63%": each alternative averaged.
			var each []*types.ReqNode
			for _, part := range n.Items {
				each = append(each, withAverage(part, grade))
			}
			alts = append(alts, each...)
		} else {
			alts = append(alts, withAverage(n, grade))
		}
		last = m[1]
		if rest := strings.TrimSpace(s[last:]); rest != "" && !strings.HasPrefix(rest, "/") {
			return nil
		}
	}
	if strings.TrimSpace(s[last:]) != "" {
		return nil
	}
	for _, alt := range alts {
		walkReq(alt, func(n *types.ReqNode) {
			if n.Type == types.NodeAverage && n.Amount > 0 {
				n.Best = true // the student chooses which course of "/" applies
			}
		})
	}
	return orNode(alts...)
}

// fixedOrCourses reports whether n is courses alone: a course, or an and/or
// of courses.
func fixedOrCourses(n *types.ReqNode) bool {
	switch n.Type {
	case types.NodeCourse:
		return true
	case types.NodeAnd, types.NodeOr:
		return !slices.ContainsFunc(n.Items, func(x *types.ReqNode) bool { return !fixedOrCourses(x) })
	}
	return false
}

// moreOneOffPasses runs the readings of rows that aren't one allocation of
// text (moreOneOffs).
func (a *compAssembler) moreOneOffPasses(lists []*types.CourseList) {
	a.complementaryCourses()
	a.cognateTotal()
	a.departmentCap()
	a.restatedConstraints()
	a.distributionSummary()
	a.languageBanNote()
	walkRows(a.items, throughDaniels)
	walkRows(a.items, possibilities)
	walkRows(a.items, approvedSequences)
	walkRows(a.items, approvalOverAll)
	walkRows(a.items, a.trailingCode)
	byName := map[string]*types.CourseList{}
	for _, l := range lists {
		byName[l.Name] = l
	}
	walkRows(a.items, func(r *compRow) { a.capstoneSubstitute(r, byName) })
	walkRows(a.items, oneDiscipline)
	a.noteFloorAfterLine()
	a.floorIfCompleted()
	walkRows(a.items, languagesNamed)
	a.progressiveSequence()
	walkRows(a.items, levelCodedRoute)
	walkRows(a.items, func(r *compRow) {
		if r.r.Kind != types.RequirementAllocation || r.r.Req == nil || r.r.Req.Type != types.NodeOr {
			return
		}
		if n := languageAtAnyLevel(r.r.Req); n != nil {
			r.r.Req = n
		} else if n := globalExperience(r); n != nil {
			r.r.Req = n
		} else if n := transferEquivalents(r.r.Req); n != nil {
			r.r.Req = n
		} else {
			a.listBelowMissing(r)
		}
	})
}

var (
	// complComplementaryRe: "5.0 credits of complementary courses, i.e.,
	// courses relevant to PCJ and offered by other programs".
	complComplementaryRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits\s+of\s+complementary\s+courses\b`)
	// complComplementaryNoteRe: "Complementary courses include but are not
	// limited to the following", with the codes after it or not.
	complComplementaryNoteRe = regexp.MustCompile(`(?i)^complementary\s+courses\s+include\s+but\s+are\s+not\s+limited\s+to\s+the\s+following\b`)
	// complOtherEligibleRe: "Other courses are also eligible to meet this
	// requirement; please contact the program director for discussion and
	// approval".
	complOtherEligibleRe = regexp.MustCompile(`(?i)^other\s+courses\s+are\s+also\s+eligible\s+to\s+meet\s+this\s+requirement;.*\bapproval\b`)
	// complCodesOnlyRe: a line of codes alone.
	complCodesOnlyRe = regexp.MustCompile(`^(?:` + codePat + `\s*[,/]?\s*)+\.?$`)
)

// complementaryCourses reads a list of complementary courses the calendar
// says is not exhaustive ("include but are not limited to") as the item's
// pool, with its approval route kept beside it (design 3.3: Met by the
// listed courses, Review by others):
//   - "5.0 credits of complementary courses, i.e., ..." whose codes are a
//     line of their own (a row of text) after the program notes
//     "Complementary courses include but are not limited to the following"
//     and "Other courses are also eligible to meet this requirement; ...
//     approval": or(credits{5.0, codes}, text{permission});
//   - a pool of designators whose permission alternative holds the list
//     ("Complementary courses include but are not limited to the following;
//     other relevant courses are eligible on the approval of the program
//     director: AFR298H1, ..."): the codes join the pool. A route for
//     "this 100-level course requirement" that another row holds too
//     (placeProgramRoutes) leaves it when its lead names no such level.
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
		// A route for "this 100-level course requirement" (another item's)
		// isn't this item's.
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

// complThisLevelReqRe: a route for one level's item: "Other introductory
// courses in relevant disciplines are also eligible to meet this 100-level
// course requirement; ...".
var complThisLevelReqRe = regexp.MustCompile(`(?i)\bthis\s+([1-4]00)-level\s+course\s+requirement\b`)

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

// dropRow takes r out of the rows.
func (a *compAssembler) dropRow(r *compRow) {
	var drop func(rs []*compRow) []*compRow
	drop = func(rs []*compRow) []*compRow {
		rs = slices.DeleteFunc(rs, func(q *compRow) bool { return q == r })
		for _, q := range rs {
			q.children = drop(q.children)
		}
		return rs
	}
	a.items, a.program, a.floors = drop(a.items), drop(a.program), drop(a.floors)
	if a.total != nil {
		a.total.children = drop(a.total.children)
	}
}

var (
	// complRequiresOfWhichRe: a total stated as an item: "The Minor Program
	// requires 4.0 credits, of which 3.0 credits must be GER courses".
	// Groups: the total, the share, its designator.
	complRequiresOfWhichRe = regexp.MustCompile(`(?i)^the\s+(?:minor|major|specialist)\s+program\s+requires\s+(\d+(?:\.\d+)?)\s+credits,\s+of\s+which\s+(\d+(?:\.\d+)?)\s+credits\s+must\s+be\s+((?-i:[A-Z]{3}))\s+courses$`)
	// complCognateCapRe: "A maximum of 1.0 credit in cognate courses is
	// allowed (i.e. courses offered by other departments ...); please
	// inquire at the Department and get approved in advance".
	complCognateCapRe = regexp.MustCompile(`(?i)^a\s+maximum\s+of\s+(\d+(?:\.\d+)?)\s+credits?\s+in\s+cognate\s+courses\s+is\s+allowed\b.*\bapprov`)
)

// cognateTotal reads a complRequiresOfWhichRe item, in a program with no
// total, as the total (Req 1), when a program note allows the rest as
// approved cognate courses (complCognateCapRe): the total's pool is the
// designator's courses, or, with the cognates, any courses with the
// approval (design 3.3, an unbounded route: other departments' courses):
// or(credits{N, D}, and(credits{N}, text{permission})), and the share of D
// is a constraint under it. The item and the note join its raw.
func (a *compAssembler) cognateTotal() {
	if a.total != nil {
		return
	}
	i := slices.IndexFunc(a.notes, func(s string) bool { return complCognateCapRe.MatchString(s) })
	if i < 0 {
		return
	}
	for _, r := range a.items {
		t := r.r.Req
		if t == nil || t.Type != types.NodeText || len(r.children) > 0 {
			continue
		}
		m := complRequiresOfWhichRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(t.Text), "."))
		if m == nil {
			continue
		}
		total, _ := strconv.ParseFloat(m[1], 64)
		share, _ := strconv.ParseFloat(m[2], 64)
		rest, _ := strconv.ParseFloat(complCognateCapRe.FindStringSubmatch(a.notes[i])[1], 64)
		if total-share != rest {
			return
		}
		note := a.notes[i]
		a.total = &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Raw: r.r.Raw + "\n" + note, Req: orNode(
			&types.ReqNode{Type: types.NodeCredits, Amount: total, Designators: []string{m[3]}},
			andNode(&types.ReqNode{Type: types.NodeCredits, Amount: total}, textNode(types.TextPermission, note)),
		)}}
		a.floors = append(a.floors, &compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Req: &types.ReqNode{Type: types.NodeCredits, Amount: share, Designators: []string{m[3]}}, Raw: r.r.Raw}})
		a.notes = slices.Delete(a.notes, i, i+1)
		a.dropRow(r)
		return
	}
}

// complDSRCountRe: what a course-count cap (complCourseCountCapRe) says the
// courses count toward: "... will count toward the Department for the
// Study of Religion courses required to complete any Major Program in
// Religion".
var complDSRCountRe = regexp.MustCompile(`(?i)\bwill\s+count\s+towards?\s+the\s+department\s+for\s+the\s+study\s+of\s+religion\s+courses\s+required\b`)

// departmentCap reads the rest of a course-count cap sentence whose limit
// is a row (courseCountCap) and that says the courses count toward the
// "Department for the Study of Religion courses required": in a program
// where no other row speaks of such courses (its rows are lists of codes),
// the sentence names no requirement of this program, so the limit, whose
// raw holds it, says all of it: no row of text that makes everyone Review,
// and no note that restates the limit.
func (a *compAssembler) departmentCap() {
	all := append(append(append([]*compRow{}, a.items...), a.program...), a.floors...)
	var text, limit *compRow
	other := false
	walkRows(all, func(r *compRow) {
		t := r.r.Req
		switch {
		case t != nil && t.Type == types.NodeText && r.r.Kind == types.RequirementConstraint && complDSRCountRe.MatchString(t.Text) && courseCountCap(t.Text) != nil:
			text = r
		case r.r.Kind == types.RequirementLimit && t != nil:
			if limit == nil || r.r.Raw != limit.r.Raw {
				limit = r
			}
		case strings.Contains(strings.ToLower(r.r.Raw), "study of religion"):
			other = true
		}
	})
	if text == nil || limit == nil || limit.r.Raw != text.r.Raw || other || a.total != nil && strings.Contains(strings.ToLower(a.total.r.Raw), "study of religion") {
		return
	}
	a.dropRow(text) // the limit's raw holds the sentence

}

var (
	// complLanguageAnyLevelRe: "1.0 credit in German language at any level,
	// including GER100Y1, GER101H1, ...". Groups: the amount, the codes.
	complLanguageAnyLevelRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+in\s+\w+\s+language\s+at\s+any\s+level,\s+including\s+((?:` + codePat + `,?\s*)+)\.?$`)
	// complAdvancedTopicsRe: "Students with very advanced previous knowledge
	// of German may in consultation with the Department take a combination
	// of other 300 or 400-level GER topics courses (taught in German) to
	// meet the requirement". Groups: the levels, the designator.
	complAdvancedTopicsRe = regexp.MustCompile(`(?i)^students\s+with\s+very\s+advanced\s+previous\s+knowledge\s+of\s+\w+\s+may\s+in\s+consultation\s+with\s+the\s+department\s+take\s+a\s+combination\s+of\s+other\s+([1-4])00\s+or\s+([1-4])00-level\s+((?-i:[A-Z]{3}))\s+topics\s+courses\b`)
)

// languageAtAnyLevel reads or(text{complLanguageAnyLevelRe},
// text{complAdvancedTopicsRe}): the listed courses ("including", examples
// of the language courses), or another language course of their designator
// (content: what the course teaches decides), or, after consultation, the
// designator's topics courses at those levels (permission).
func languageAtAnyLevel(n *types.ReqNode) *types.ReqNode {
	if len(n.Items) != 2 || n.Items[0].Type != types.NodeText || n.Items[1].Type != types.NodeText {
		return nil
	}
	a, b := n.Items[0].Text, n.Items[1].Text
	m := complLanguageAnyLevelRe.FindStringSubmatch(strings.TrimSpace(a))
	t := complAdvancedTopicsRe.FindStringSubmatch(strings.TrimSpace(b))
	if m == nil || t == nil {
		return nil
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	var levels []int
	for _, l := range []string{t[1], t[2]} {
		levels = append(levels, int(l[0]-'0')*100)
	}
	codes := codeRe.FindAllString(m[2], -1)
	// The language's other courses: its designator's (audit FIX-ONE
	// ASCER1401), the content text deciding which are language courses.
	content := textNode(types.TextContent, a)
	if !slices.ContainsFunc(codes, func(c string) bool { return c[:3] != codes[0][:3] }) {
		content = andNode(&types.ReqNode{Type: types.NodeCredits, Amount: v, Designators: []string{codes[0][:3]}}, content)
	}
	return orNode(
		&types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: codes},
		content,
		andNode(&types.ReqNode{Type: types.NodeCredits, Amount: v, Designators: []string{t[3]}, Levels: levels}, textNode(types.TextPermission, b)),
	)
}

var (
	// complInternshipFormRe: a note naming a course that meets the item:
	// "May take the form of an internship course, GER391H1 ‘iPRAKTIKUM
	// ...’".
	complInternshipFormRe = regexp.MustCompile(`(?i)^may\s+take\s+the\s+form\s+of\s+an\s+internship\s+course,\s+(` + codePat + `)\b`)
	// complStudyAbroadSuchAsRe: "Students may also complete a study abroad
	// course in a German-speaking country, such as GER354Y0".
	complStudyAbroadSuchAsRe = regexp.MustCompile(`(?i)^students\s+may\s+also\s+complete\s+a\s+study\s+abroad\s+course\b[^.]*?,\s+such\s+as\s+(` + codePat + `)\.?$`)
)

// globalExperience reads an item of global experience whose note names a
// course that meets it (complInternshipFormRe) and whose alternative names
// an example study abroad course (complStudyAbroadSuchAsRe): both courses
// are alternatives; the study abroad sentence stays for the courses it
// doesn't name, a content text (the course's kind decides). The note joins
// the raw.
func globalExperience(r *compRow) *types.ReqNode {
	n := r.r.Req
	i := slices.IndexFunc(r.r.Notes, func(s string) bool { return complInternshipFormRe.MatchString(s) })
	j := slices.IndexFunc(n.Items, func(x *types.ReqNode) bool {
		return x.Type == types.NodeText && complStudyAbroadSuchAsRe.MatchString(strings.TrimSpace(x.Text))
	})
	if i < 0 || j < 0 {
		return nil
	}
	internship := complInternshipFormRe.FindStringSubmatch(r.r.Notes[i])[1]
	abroad := complStudyAbroadSuchAsRe.FindStringSubmatch(strings.TrimSpace(n.Items[j].Text))[1]
	items := []*types.ReqNode{{Type: types.NodeCourse, Code: internship}, {Type: types.NodeCourse, Code: abroad}}
	for k, x := range n.Items {
		if k == j {
			x = textNode(types.TextContent, x.Text)
		}
		items = append(items, x)
	}
	addRaw(r, r.r.Notes[i])
	if r.r.Notes = slices.Delete(r.r.Notes, i, i+1); len(r.r.Notes) == 0 {
		r.r.Notes = nil
	}
	return orNode(items...)
}

// complTransferEquivRe: "Transfer credit equivalents of the following
// courses earned from participation in an international exchange or actual
// courses completed via the Summer Abroad program will qualify for this
// requirement: ECO324H1/ ...". Group: the codes.
var complTransferEquivRe = regexp.MustCompile(`(?i)^transfer\s+credit\s+equivalents\s+of\s+the\s+following\s+courses\b[^:]*\bwill\s+qualify\s+for\s+this\s+requirement:\s*((?:` + codePat + `\s*[/,]?\s*)+)\.?$`)

// transferEquivalents reads an alternative complTransferEquivRe says
// qualifies: the transfer credits of the listed courses, the amount the
// item's first text states (credits{transfer}); the sentence stays beside
// it for the Summer Abroad courses (content: how the course was taken
// decides).
func transferEquivalents(n *types.ReqNode) *types.ReqNode {
	if n.Items[0].Type != types.NodeText {
		return nil
	}
	am := regexp.MustCompile(`^(\d+(?:\.\d+)?)\s+credits?\b`).FindStringSubmatch(strings.TrimSpace(n.Items[0].Text))
	if am == nil {
		return nil
	}
	v, _ := strconv.ParseFloat(am[1], 64)
	var items []*types.ReqNode
	found := false
	for _, x := range n.Items {
		if m := complTransferEquivRe.FindStringSubmatch(strings.TrimSpace(x.Text)); x.Type == types.NodeText && m != nil {
			items = append(items, &types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: dedupe(codeRe.FindAllString(m[1], -1)), Transfer: true}, textNode(types.TextContent, x.Text))
			found = true
			continue
		}
		items = append(items, x)
	}
	if !found {
		return nil
	}
	return orNode(items...)
}

var (
	// complListBelowFloorRe: "2.5 credits in MUS courses from the list
	// below, including one credit at the 300+ level". Groups: the amount,
	// the designator, the floor, its level.
	complListBelowFloorRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits\s+in\s+((?-i:[A-Z]{3}))\s+courses\s+from\s+the\s+list\s+below,\s+including\s+(one|\d+(?:\.\d+)?)\s+credits?\s+at\s+the\s+([1-4])00\+\s+level$`)
	// complEitherCountedRe: "Either MUS120Y1 or MUS220Y1 can be counted
	// towards the 2.5 credits in MUS courses". Groups: the two courses.
	complEitherCountedRe = regexp.MustCompile(`(?i)^either\s+(` + codePat + `)\s+or\s+(` + codePat + `)\s+can\s+be\s+counted\s+towards?\s+the\b`)
)

// listBelowMissing reads or(text{complListBelowFloorRe},
// text{complEitherCountedRe}) in a program whose text has no list below
// (the calendar leaves it out): the designator's credits are needed, and
// the list is unknown, so and(credits{N, D}, the sentence): NotMet without
// them, Review with them. The floor is a constraint under it; "either X or
// Y" is a limit of one course's weight on them. The second sentence joins
// the raw.
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
	// complTitledFloorsRe: a total whose floors name a titled list: "7.0
	// ENG credits including 2.0 credits from 300+ level Creative Writing
	// courses, at least 0.5 credit of which must be at the 400-level".
	// Groups: the floor, its level, the title, the floor under it, its
	// level.
	complTitledFloorsRe = regexp.MustCompile(`(?i)^\d+(?:\.\d+)?\s+(?-i:[A-Z]{3})\s+credits\s+(including\s+(\d+(?:\.\d+)?)\s+credits\s+from\s+([1-4])00\+\s+level\s+(.+?)\s+courses),\s+(at\s+least\s+(\d+(?:\.\d+)?)\s+credits?\s+of\s+which\s+must\s+be\s+at\s+the\s+([1-4])00-level)\.?$`)
	// complToIncludeTitledRe: "3.0 credits from ENG courses, to include 2.0
	// credits from Creative Writing courses at the 300+ level, at least 0.5
	// credits of which must be from Creative Writing courses at the 400
	// level". Groups: the amount, the designator, the floor, the title, its
	// level, the floor under it, its title, its level.
	complToIncludeTitledRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits\s+from\s+((?-i:[A-Z]{3}))\s+courses,\s+to\s+include\s+(\d+(?:\.\d+)?)\s+credits\s+from\s+(.+?)\s+courses\s+at\s+the\s+([1-4])00\+\s+level,\s+at\s+least\s+(\d+(?:\.\d+)?)\s+credits?\s+of\s+which\s+must\s+be\s+from\s+(.+?)\s+courses\s+at\s+the\s+([1-4])00\s+level\.?$`)
)

// titledFloors reads floors on a titled list ("Creative Writing", a list
// of the program's own, in notes) that the grammar leaves as text, in
// namedLists with the lists known (Creative Writing courses are ENG, DRM,
// CRE and WRR courses):
//   - the total's (complTitledFloorsRe): its floor rows, of text or the
//     grammar's level floor, are the list's floors, and the total is the
//     item's form below (titledTotal);
//   - an item's (complToIncludeTitledRe): the item's own pool is the
//     designator's courses, and, as the lenient reading (the list's other
//     courses "included" in the designator's credits), and(the pool and
//     the list, text{ambiguity}); its floors are constraints under it.
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

// titledTotal makes a total of the designator's credits whose floors name
// a titled list ("7.0 ENG credits including 2.0 credits from 300+ level
// Creative Writing courses") the strict reading, beside the lenient one
// that counts the list's other courses as the designator's:
// or(credits{N, D}, and(credits{N, the pool and the list}, text{ambiguity})),
// the union a list of row 1's own ("Requirement 1 courses").
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

// complLevelOrAnyRe: "1.0 credit from 300-level CAS courses and/or
// 400-level courses (including JHA, JPA, and SAS)". Groups: the two levels.
var complLevelOrAnyRe = regexp.MustCompile(`(?i)\b([1-4])00-level\s+(?-i:[A-Z]{3})\s+courses\s+and/or\s+([1-4])00-level\s+courses\b`)

// levelOrAnyLevel gives a row or(credits{amount, designators, both levels},
// text{ambiguity: complLevelOrAnyRe}) its lenient reading by the ambiguity
// policy, in place of the bare text: the second level's courses of any
// designator, and(credits{amount, union}, text{ambiguity}), with the union
// (the first level's designators, any course at the second) a list of the
// row's own.
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
