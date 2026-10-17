package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Cognate caps (10F2 item 2): a program's courses from other units, capped,
// read as a union pool (design 3.2) with a limit on the other units' share.

// cognates reads the cognate caps once the routes are placed and the lists
// read (finish), and returns lists with the lists they add:
//   - "Students complete 4.0 credits, of which up to 1.0 cognate credit may
//     be taken in PRT. No other cognates are permitted for a Spanish Minor":
//     the total's pool is the subject's designator and the cognate's, and
//     the refusal joins the total (closedCognates);
//   - "Up to 3.0 credits from approved courses ... taught by other
//     departments, notably Art History and Philosophy, may be substituted
//     for the 5.5 credits in CLA courses from Requirement 4" over lists of
//     those departments' courses, and "Up to 2.0 credits in GRK or LAT may
//     be substituted for ...": a union of the pool, the lists' courses and
//     the designators, a limit on each share, and the approved courses the
//     lists only exemplify kept as the item's route (substitutedShares);
//   - "Additional 2.5 credits, of which a maximum 1.0 credit may be
//     environmental courses offered by academic units other than the School
//     of the Environment (from Group A below). The remainder must be
//     selected from the following: ..." (remainderShares);
//   - "3.0 credits from courses offered by the Canadian Studies Program,
//     including additional courses from Requirements 1 and 2. Up to 1.0
//     credit of this requirement can also be completed from HST211H1/
//     PHS211H1, ..." (programShares);
//   - "Up to 1.0 credit from Cognate Courses:" over the courses, an item
//     that caps what the total draws on (cognateItem);
//   - "1.0 credit may be courses on East Asia offered by other departments"
//     under an item drawing on "Society-Culture Courses on East Asia
//     Offered by Other Departments": a limit on that list (listCaps).
func (a *compAssembler) cognates(lists []*types.CourseList) []*types.CourseList {
	a.closedCognates()
	for _, row := range a.allRows() {
		lists = a.substitutedShares(row, lists)
		lists = a.remainderShares(row, lists)
		lists = a.programShares(row, lists)
	}
	lists = a.cognateItem(lists)
	a.listCaps(lists)
	return lists
}

// cognateSubjects maps subject names a cognate rule is written with to the
// designator of the subject's own courses (beside subjectDesignators).
var cognateSubjects = map[string]string{"spanish": "SPA"}

// complNoOtherCognatesRe: "No other cognates are permitted for a Spanish
// Minor". Group: the subject.
var complNoOtherCognatesRe = regexp.MustCompile(`(?i)^no\s+other\s+cognates\s+are\s+permitted\s+for\s+an?\s+(\w+(?:\s+\w+)?)\s+(?:minor|major|specialist)(?:\s+program)?$`)

// closedCognates reads a refusal of other cognates under the total
// (complNoOtherCognatesRe) beside a cap on the one cognate allowed
// (credits{designators, max_amount}): the total, a plain amount, draws on
// the subject's designator and the cognate's, and the refusal's row goes.
// The rule names every course that counts, so the total has no text.
func (a *compAssembler) closedCognates() {
	t := a.total
	if t == nil || t.r.Req == nil || t.r.Req.Type != types.NodeCredits || t.r.Req.Designators != nil || t.r.Req.Levels != nil || t.r.Req.Courses != nil || t.r.Req.Lists != nil {
		return
	}
	var refusal, limit *compRow
	var own string
	for _, r := range a.allRows() {
		n := r.r.Req
		if r == t || n == nil || len(r.children) > 0 {
			continue
		}
		if m := complNoOtherCognatesRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(complClean(n.Text)), ".")); r.r.Kind == types.RequirementConstraint && n.Type == types.NodeText && m != nil {
			subject := strings.ToLower(m[1])
			if own = cognateSubjects[subject]; own == "" {
				own = subjectDesignators[subject]
			}
			refusal = r
		}
		if r.r.Kind == types.RequirementLimit && n.Type == types.NodeCredits && n.MaxAmount != nil && len(n.Designators) > 0 &&
			n.Courses == nil && n.Levels == nil && complCognateRe.MatchString(r.r.Raw) {
			if limit != nil {
				return // two caps: which one the refusal leaves is unclear
			}
			limit = r
		}
	}
	if refusal == nil || limit == nil || own == "" || slices.Contains(limit.r.Req.Designators, own) {
		return
	}
	t.r.Req.Designators = append([]string{own}, limit.r.Req.Designators...)
	addRaw(t, refusal.r.Raw)
	if !a.detach(refusal) {
		t.children = slices.DeleteFunc(t.children, func(r *compRow) bool { return r == refusal })
	}
}

var (
	// complSubstApprovedRe: "Up to 3.0 credits from approved courses
	// involving the ancient Greek and Roman civilizations taught by other
	// departments, notably Art History and Philosophy, may be substituted
	// for the 5.5 credits in CLA courses from Requirement 4". Groups: the
	// share, the departments named, the pool's amount, its designator, the
	// requirement.
	complSubstApprovedRe = regexp.MustCompile(`(?i)^up\s+to\s+(\d+(?:\.\d+)?)\s+credits?\s+from\s+approved\s+courses\s+.+?\btaught\s+by\s+other\s+departments\s*,\s*notably\s+(.+?)\s*,\s*may\s+be\s+substituted\s+for\s+the\s+(\d+(?:\.\d+)?)\s+credits\s+in\s+([A-Z]{3})\s+courses\s+from\s+requirement\s+(\d{1,2})$`)
	// complSubstDesigsRe: "Up to 2.0 credits in GRK or LAT may be
	// substituted for the 5.5 credits in CLA courses from Requirement 4".
	// Groups: the share, the designators, then as above.
	complSubstDesigsRe = regexp.MustCompile(`(?i)^up\s+to\s+(\d+(?:\.\d+)?)\s+credits?\s+in\s+([A-Z]{3}(?:\s*(?:,|or|and)\s*[A-Z]{3})*)\s+may\s+be\s+substituted\s+for\s+the\s+(\d+(?:\.\d+)?)\s+credits\s+in\s+([A-Z]{3})\s+courses\s+from\s+requirement\s+(\d{1,2})$`)
	// complTitledCodesRe: a note of a list by its title: "Art History:
	// FAH206H1, FAH207H1, ...". Groups: the title, the members.
	complTitledCodesRe = regexp.MustCompile(`^([A-Z][\w&' -]*?):\s*(.+)$`)
)

// substitutedShares reads row's or(credits{amount, designators: [D]},
// text{permission}, ...) whose texts substitute courses for the pool's
// credits (complSubstApprovedRe, complSubstDesigsRe, naming the row's own
// label, amount and designator): the row draws on a list of its own of
// the pool, the courses of the departments' lists (notes whose titles the
// sentence names) and the designators, at the pool's levels, with a limit
// on each share. The approved courses the lists only exemplify ("notably")
// stay the row's route.
func (a *compAssembler) substitutedShares(row *compRow, lists []*types.CourseList) []*types.CourseList {
	n := row.r.Req
	if row.r.Kind != types.RequirementAllocation || row.r.Label == "" || n == nil || n.Type != types.NodeOr || len(n.Items) != 3 {
		return lists
	}
	pool := n.Items[0]
	if pool.Type != types.NodeCredits || len(pool.Designators) != 1 || pool.Courses != nil || pool.Lists != nil || pool.Exclude != nil {
		return lists
	}
	var approved *types.ReqNode
	var approvedShare, desigShare float64
	var codes, desigs, used []string
	var limits []*types.Requirement
	for _, alt := range n.Items[1:] {
		if alt.Type != types.NodeText || alt.Kind != types.TextPermission {
			return lists
		}
		t := strings.TrimSuffix(strings.TrimSpace(complClean(alt.Text)), ".")
		if m := complSubstApprovedRe.FindStringSubmatch(t); m != nil && approved == nil && a.substFits(row, pool, m[3:]) {
			approvedShare, _ = strconv.ParseFloat(m[1], 64)
			for _, note := range a.notes {
				g := complTitledCodesRe.FindStringSubmatch(note)
				if g == nil || !strings.Contains(strings.ToLower(m[2]), strings.ToLower(g[1])) {
					continue
				}
				items, except, perm, ok := listMembersPerm(g[2])
				if !ok || except != nil || perm != "" || len(items) != 1 || items[0].Courses == nil || items[0].Designators != nil {
					return lists
				}
				codes = append(codes, items[0].Courses...)
				used = append(used, note)
			}
			approved = alt
			continue
		}
		if m := complSubstDesigsRe.FindStringSubmatch(t); m != nil && desigs == nil && a.substFits(row, pool, m[3:]) {
			desigShare, _ = strconv.ParseFloat(m[1], 64)
			desigs = desigRe.FindAllString(m[2], -1)
			limits = append(limits, &types.Requirement{Kind: types.RequirementLimit, Raw: t,
				Req: &types.ReqNode{Type: types.NodeCredits, Designators: slices.Clone(desigs), MaxAmount: maxAmount(desigShare)}})
			continue
		}
		return lists
	}
	if approved == nil || codes == nil || desigs == nil || slices.Contains(desigs, pool.Designators[0]) {
		return lists
	}
	codes = dedupe(codes)
	limits = append([]*types.Requirement{{Kind: types.RequirementLimit, Raw: strings.TrimSuffix(strings.TrimSpace(complClean(approved.Text)), "."),
		Req: &types.ReqNode{Type: types.NodeCredits, Courses: slices.Clone(codes), MaxAmount: maxAmount(approvedShare)}}}, limits...)
	own := &types.ReqNode{Type: types.NodeCredits, Designators: slices.Clone(pool.Designators), Levels: slices.Clone(pool.Levels)}
	// The designators stand in for the pool's credits, so at its levels.
	others := &types.ReqNode{Type: types.NodeCredits, Designators: desigs, Levels: slices.Clone(pool.Levels)}
	name := unionName(row, lists)
	lists = append(lists, &types.CourseList{Name: name, Items: []*types.ReqNode{own, {Type: types.NodeCredits, Courses: codes}, others}})
	row.r.Req = orNode(&types.ReqNode{Type: types.NodeCredits, Amount: pool.Amount, Lists: []string{name}}, approved)
	// "including 2.0 credits at the 300+-level": of the pool's courses, the
	// strict reading; the route the row keeps marks the floor (markIncomplete),
	// so a substitute at the level is Review.
	for _, c := range row.children {
		if c.r.Kind != types.RequirementConstraint {
			continue
		}
		walkReq(c.r.Req, func(x *types.ReqNode) {
			if x.Type == types.NodeCredits && x.Amount > 0 && x.Designators == nil && x.Courses == nil && x.Lists == nil {
				x.Designators = slices.Clone(pool.Designators)
			}
		})
	}
	for _, l := range limits {
		a.rowUnder(row, l)
	}
	for _, u := range used {
		addRaw(row, u)
	}
	a.notes = slices.DeleteFunc(a.notes, func(s string) bool { return slices.Contains(used, s) })
	return lists
}

// substFits reports whether a substitution's pool (m: amount, designator,
// requirement label) is row's pool.
func (a *compAssembler) substFits(row *compRow, pool *types.ReqNode, m []string) bool {
	v, _ := strconv.ParseFloat(m[0], 64)
	return v == pool.Amount && m[1] == pool.Designators[0] && strings.HasSuffix(row.r.Label, m[2]) && a.labelled(row.r.Label) == row
}

// unionName names a list of row's own pool as addUnion does: "Requirement
// <label> courses", then "(2)", "(3)" when lists has it.
func unionName(row *compRow, lists []*types.CourseList) string {
	base := "Requirement " + row.r.Label
	if !strings.HasSuffix(strings.ToLower(base), " courses") {
		base += " courses"
	}
	name := base
	for k := 2; slices.ContainsFunc(lists, func(l *types.CourseList) bool { return l.Name == name }); k++ {
		name = base + " (" + strconv.Itoa(k) + ")"
	}
	return name
}

// listCaps reads an allocation of text "1.0 credit may be courses on East
// Asia offered by other departments" (complWidenOnRe) as a limit under the
// total on the list titled as those courses ("Society-Culture Courses on
// East Asia Offered by Other Departments") that another row draws on: the
// courses count only through that row, so the sentence caps them.
func (a *compAssembler) listCaps(lists []*types.CourseList) {
	if a.total == nil {
		return
	}
	for _, r := range a.items {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || len(r.children) > 0 {
			continue
		}
		m := complWidenOnRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(complClean(n.Text)), "."))
		if m == nil {
			continue
		}
		suffix := strings.ToLower(" courses on " + m[2] + " offered by other departments")
		var name string
		for _, l := range lists {
			if strings.HasSuffix(strings.ToLower(l.Name), suffix) && a.drawsOn(r, l.Name) {
				name = l.Name
			}
		}
		if name == "" {
			continue
		}
		v, _ := strconv.ParseFloat(m[1], 64)
		r.r.Kind, r.r.Label = types.RequirementLimit, ""
		r.r.Req = &types.ReqNode{Type: types.NodeCredits, Lists: []string{name}, MaxAmount: maxAmount(v)}
	}
}

// drawsOn reports whether a row other than r draws on list name.
func (a *compAssembler) drawsOn(r *compRow, name string) bool {
	found := false
	for _, o := range a.allRows() {
		if o == r || o.r.Kind != types.RequirementAllocation {
			continue
		}
		walkReq(o.r.Req, func(x *types.ReqNode) {
			if slices.Contains(x.Lists, name) {
				found = true
			}
		})
	}
	return found
}

var (
	// complMaxShareGroupRe: "Additional 2.5 credits, of which a maximum 1.0
	// credit may be environmental courses offered by academic units other
	// than the School of the Environment (from Group A below)". Groups: the
	// amount, the share, the group.
	complMaxShareGroupRe = regexp.MustCompile(`(?i)^additional\s+(\d+(?:\.\d+)?)\s+credits\s*,\s*of\s+which\s+a\s+maximum\s+(?:of\s+)?(\d+(?:\.\d+)?)\s+credits?\s+may\s+be\s+.+?\s*\(\s*from\s+(group\s+[A-Z])\s+below\s*\)$`)
	// complRemainderFromRe: "The remainder must be selected from the
	// following: ENV261H1/ ENV360H1, ..., ENV411H1*, ...". Group: the
	// members.
	complRemainderFromRe = regexp.MustCompile(`(?i)^the\s+remainder\s+must\s+be\s+selected\s+from\s+the\s+following:\s*(.+)$`)
	// complOtherApprovedRe: a list's last member a unit decides: ", other
	// courses approved by School of the Environment".
	complOtherApprovedRe = regexp.MustCompile(`(?i)\s*,\s*(other\s+courses\s+approved\s+by\s+.+)$`)
)

// remainderShares reads row's or(text{complMaxShareGroupRe}, text{the
// footnote of starred courses, "*with permission of ..."}, routes...) over
// its constraint of text "The remainder must be selected from the
// following: ..." and the group's note ("Group A: ANT364H1, ..., other
// courses approved by School of the Environment"): the row draws on a list
// of its own, the remainder's courses (the starred ones a maybe item with
// the footnote's permission) and the group's list, with a limit on the
// group's share. A unit's approval of other group courses is the row's
// route; the other routes stay.
func (a *compAssembler) remainderShares(row *compRow, lists []*types.CourseList) []*types.CourseList {
	n := row.r.Req
	if row.r.Kind != types.RequirementAllocation || row.r.Label == "" || n == nil || n.Type != types.NodeOr || len(row.children) != 1 || n.Items[0].Type != types.NodeText {
		return lists
	}
	m := complMaxShareGroupRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(complClean(n.Items[0].Text)), "."))
	rest := row.children[0]
	if m == nil || rest.r.Kind != types.RequirementConstraint || rest.r.Req == nil || rest.r.Req.Type != types.NodeText || len(rest.children) > 0 {
		return lists
	}
	r := complRemainderFromRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(rest.r.Req.Text), "."))
	if r == nil {
		return lists
	}
	var footnote *types.ReqNode
	var routes []*types.ReqNode
	for _, alt := range n.Items[1:] {
		if alt.Type == types.NodeText && alt.Kind == types.TextPermission && strings.HasPrefix(alt.Text, "*") && footnote == nil {
			footnote = alt
			continue
		}
		routes = append(routes, alt)
	}
	var plain, starred []string
	for _, part := range strings.Split(r[1], ",") {
		codes := complAnyCodeRe.FindAllString(part, -1)
		if codes == nil || !complListRestRe.MatchString(complAnyCodeRe.ReplaceAllString(part, " ")) {
			return lists
		}
		if strings.HasSuffix(strings.TrimSpace(part), "*") {
			starred = append(starred, codes...) // "ENV421Y1/ ENV421H1*": the pair
		} else {
			plain = append(plain, codes...)
		}
	}
	if (starred == nil) != (footnote == nil) {
		return lists
	}
	group, note, approval, ok := a.groupNote(m[3])
	if !ok || slices.ContainsFunc(lists, func(l *types.CourseList) bool { return l.Name == group.Name }) {
		return lists
	}
	items := []*types.ReqNode{{Type: types.NodeCredits, Courses: dedupe(plain)}}
	if starred != nil {
		items = append(items, andNode(&types.ReqNode{Type: types.NodeCredits, Courses: dedupe(starred)}, textNode(types.TextPermission, strings.TrimSpace(strings.TrimPrefix(footnote.Text, "*")))))
	}
	items = append(items, &types.ReqNode{Type: types.NodeCredits, Lists: []string{group.Name}})
	name := unionName(row, lists)
	lists = append(lists, group, &types.CourseList{Name: name, Items: items})
	amount, _ := strconv.ParseFloat(m[1], 64)
	share, _ := strconv.ParseFloat(m[2], 64)
	alts := []*types.ReqNode{{Type: types.NodeCredits, Amount: amount, Lists: []string{name}}}
	if approval != "" {
		alts = append(alts, textNode(types.TextPermission, approval))
	}
	row.r.Req = orNode(append(alts, routes...)...)
	row.children = []*compRow{{r: &types.Requirement{Kind: types.RequirementLimit, Raw: strings.TrimSuffix(strings.TrimSpace(complClean(n.Items[0].Text)), "."),
		Req: &types.ReqNode{Type: types.NodeCredits, Lists: []string{group.Name}, MaxAmount: maxAmount(share)}}}}
	addRaw(row, rest.r.Raw)
	addRaw(row, note)
	a.notes = slices.DeleteFunc(a.notes, func(s string) bool { return s == note })
	return lists
}

// groupNote returns the list a note of the group's courses ("Group A:
// ANT364H1, ..., other courses approved by School of the Environment")
// holds, the note, and the approval of other courses its last member
// gives, if any.
func (a *compAssembler) groupNote(title string) (*types.CourseList, string, string, bool) {
	for _, note := range a.notes {
		g := complTitledCodesRe.FindStringSubmatch(note)
		if g == nil || !strings.EqualFold(g[1], title) {
			continue
		}
		members, approval := strings.TrimSuffix(strings.TrimSpace(g[2]), "."), ""
		if loc := complOtherApprovedRe.FindStringSubmatchIndex(members); loc != nil {
			approval = members[loc[2]:loc[3]]
			members = members[:loc[0]]
		}
		items, except, perm, ok := listMembersPerm(members)
		if !ok || except != nil || perm != "" || len(items) != 1 || items[0].Courses == nil || items[0].Designators != nil {
			return nil, "", "", false
		}
		return &types.CourseList{Name: g[1], Items: items}, note, approval, true
	}
	return nil, "", "", false
}

var (
	// complProgramCoursesRe: "3.0 credits from courses offered by the
	// Canadian Studies Program, including additional courses from
	// Requirements 1 and 2". Groups: the amount, what offers the courses,
	// the requirements.
	complProgramCoursesRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits\s+from\s+courses\s+(offered\s+by\s+the\s+.+?\s+program)\s*,\s*including\s+additional\s+courses\s+from\s+requirements\s+(\d{1,2}(?:\s*(?:,|and|or)\s*\d{1,2})*)$`)
	// complAlsoCompletedFromRe: "Up to 1.0 credit of this requirement can
	// also be completed from HST211H1/ PHS211H1, INS200H1, ...". Groups: the
	// share, the courses.
	complAlsoCompletedFromRe = regexp.MustCompile(`(?i)^up\s+to\s+(\d+(?:\.\d+)?)\s+credits?\s+of\s+this\s+requirement\s+can\s+also\s+be\s+completed\s+from\s*:?\s*(.+)$`)
)

// programShares reads row's or(text{complProgramCoursesRe},
// text{complAlsoCompletedFromRe}, routes...): the row draws on a list of its
// own, the program's courses (a content condition, D5: a maybe item
// and(credits{}, text{content})), the rows it names (credits{requirements},
// numbered later) and the courses, with a limit on the courses' share; the
// routes stay.
func (a *compAssembler) programShares(row *compRow, lists []*types.CourseList) []*types.CourseList {
	n := row.r.Req
	if row.r.Kind != types.RequirementAllocation || row.r.Label == "" || n == nil || n.Type != types.NodeOr || len(n.Items) < 2 || len(row.children) > 0 ||
		n.Items[0].Type != types.NodeText || n.Items[1].Type != types.NodeText {
		return lists
	}
	m := complProgramCoursesRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(complClean(n.Items[0].Text)), "."))
	c := complAlsoCompletedFromRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(complClean(n.Items[1].Text)), "."))
	if m == nil || c == nil {
		return lists
	}
	codes := complAnyCodeRe.FindAllString(c[2], -1)
	if codes == nil || !complListRestRe.MatchString(complAnyCodeRe.ReplaceAllString(c[2], " ")) {
		return lists
	}
	var rows []*compRow
	for _, l := range regexp.MustCompile(`\d{1,2}`).FindAllString(m[3], -1) {
		r := a.labelled(l)
		if r == nil || r == row {
			return lists
		}
		rows = append(rows, r)
	}
	ref := &types.ReqNode{Type: types.NodeCredits}
	codes = dedupe(codes)
	name := unionName(row, lists)
	l := &types.CourseList{Name: name, Items: []*types.ReqNode{
		andNode(&types.ReqNode{Type: types.NodeCredits}, textNode(types.TextContent, m[2])),
		ref,
		{Type: types.NodeCredits, Courses: codes},
	}}
	lists = append(lists, l)
	a.reqRefs = append(a.reqRefs, compReqRef{node: ref, rows: rows, list: l})
	amount, _ := strconv.ParseFloat(m[1], 64)
	share, _ := strconv.ParseFloat(c[1], 64)
	row.r.Req = orNode(append([]*types.ReqNode{{Type: types.NodeCredits, Amount: amount, Lists: []string{name}}}, n.Items[2:]...)...)
	a.rowUnder(row, &types.Requirement{Kind: types.RequirementLimit, Raw: strings.TrimSuffix(strings.TrimSpace(complClean(n.Items[1].Text)), "."),
		Req: &types.ReqNode{Type: types.NodeCredits, Courses: slices.Clone(codes), MaxAmount: maxAmount(share)}})
	return lists
}

// complCognateItemRe: an item capping the cognate courses its note lists:
// "Up to 1.0 credit from Cognate Courses:". Group: the share.
var complCognateItemRe = regexp.MustCompile(`(?i)^(?:\d{1,2}\.\s+)?up\s+to\s+(\d+(?:\.\d+)?)\s+credits?\s+from\s+cognate\s+courses\s*:?$`)

// cognateItem reads an item of text complCognateItemRe whose note lists the
// cognate courses ("SMC219Y1, ..., or another creativity-themed course
// approved by the program coordinator"), under a total of a plain amount
// whose other items are closed: the item is a cap, not an allocation (the
// header: "Up to 1.0 credit may be chosen from approved courses offered by
// other departments"). The total draws on a list of its own, the courses
// the other items name and the cognates, with the item a limit on the
// cognates, and the approval of another course the total's route. The
// program's own courses the items don't name may count too: the route's
// text makes the total Review short of the list.
func (a *compAssembler) cognateItem(lists []*types.CourseList) []*types.CourseList {
	t := a.total
	if t == nil || t.r.Req == nil || t.r.Req.Type != types.NodeCredits || t.r.Req.Designators != nil || t.r.Req.Levels != nil || t.r.Req.Courses != nil || t.r.Req.Lists != nil {
		return lists
	}
	var item *compRow
	var own []string
	for _, r := range a.items {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation {
			continue
		}
		if n != nil && n.Type == types.NodeOr && len(n.Items) > 0 && n.Items[0].Type == types.NodeText && complCognateItemRe.MatchString(strings.TrimSpace(complClean(n.Items[0].Text))) && item == nil {
			item = r
			continue
		}
		if n == nil || openText(n) || len(r.children) > 0 {
			return lists
		}
		walkReq(n, func(x *types.ReqNode) {
			if x.Type == types.NodeCourse {
				own = append(own, x.Code)
			}
			if x.Type == types.NodeCredits {
				own = append(own, x.Courses...)
			}
		})
	}
	if item == nil || own == nil || len(item.r.Notes) != 1 || len(item.children) > 0 {
		return lists
	}
	n := item.r.Req
	for _, alt := range n.Items[1:] {
		if alt.Type != types.NodeText || !complCognateRe.MatchString(alt.Text) || !strings.Contains(t.r.Raw, strings.TrimSuffix(alt.Text, ".")) {
			return lists // only the header's restatement may join the item
		}
	}
	items, except, perm, ok := listMembersPerm(item.r.Notes[0])
	if !ok || except != nil || len(items) != 1 || items[0].Courses == nil || items[0].Designators != nil {
		return lists
	}
	m := complCognateItemRe.FindStringSubmatch(strings.TrimSpace(complClean(n.Items[0].Text)))
	share, _ := strconv.ParseFloat(m[1], 64)
	cognates := items[0].Courses
	name := unionName(item, lists)
	lists = append(lists, &types.CourseList{Name: name, Items: []*types.ReqNode{{Type: types.NodeCredits, Courses: dedupe(own)}, {Type: types.NodeCredits, Courses: slices.Clone(cognates)}}})
	pool := &types.ReqNode{Type: types.NodeCredits, Amount: t.r.Req.Amount, Lists: []string{name}}
	t.r.Req = pool
	if perm != "" {
		t.r.Req = orNode(pool, textNode(types.TextPermission, perm))
	}
	item.r.Kind, item.r.Label = types.RequirementLimit, ""
	item.r.Req = &types.ReqNode{Type: types.NodeCredits, Courses: slices.Clone(cognates), MaxAmount: maxAmount(share)}
	addRaw(item, item.r.Notes[0])
	item.r.Notes = nil
	return lists
}
