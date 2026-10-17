package completion

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/parser/grammar"
	"degree-explorer/parser/programs"
	"degree-explorer/types"
)

// Cognate and share pools: cognate caps and refusals, substituted shares, list
// caps, and a group's or a program's share of a pool.

// cognates reads the cognate caps and returns lists with the lists they add.
func (a *Assembler) cognates(lists []*types.CourseList) []*types.CourseList {
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

// cognateSubjects maps subject names a cognate rule uses to the subject's
// designator, where SubjectDesignators has none.
var cognateSubjects = map[string]string{"spanish": "SPA"}

// noOtherCognatesRe matches "No other cognates are permitted for a Spanish
// Minor".
var noOtherCognatesRe = regexp.MustCompile(`(?i)^no\s+other\s+cognates\s+are\s+permitted\s+for\s+an?\s+(\w+(?:\s+\w+)?)\s+(?:minor|major|specialist)(?:\s+program)?$`)

// closedCognates reads a refusal of other cognates beside a cap on the one
// allowed: the plain total draws on the subject's and the cognate's
// designators, and needs no text since the rule names every course.
func (a *Assembler) closedCognates() {
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
		if m := noOtherCognatesRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(Clean(n.Text)), ".")); r.r.Kind == types.RequirementConstraint && n.Type == types.NodeText && m != nil {
			subject := strings.ToLower(m[1])
			if own = cognateSubjects[subject]; own == "" {
				own = grammar.SubjectDesignators[subject]
			}
			refusal = r
		}
		if r.r.Kind == types.RequirementLimit && n.Type == types.NodeCredits && n.MaxAmount != nil && len(n.Designators) > 0 &&
			n.Courses == nil && n.Levels == nil && cognateRe.MatchString(r.r.Raw) {
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
	// substApprovedRe matches approved courses substituted for a pool: "Up
	// to 3.0 credits from approved courses ... taught by other departments,
	// notably Art History, may be substituted for the 5.5 credits in CLA
	// courses from Requirement 4".
	substApprovedRe = regexp.MustCompile(`(?i)^up\s+to\s+` + grammar.AmountPat + `\s+credits?\s+from\s+approved\s+courses\s+.+?\btaught\s+by\s+other\s+departments\s*,\s*notably\s+(.+?)\s*,\s*may\s+be\s+substituted\s+for\s+the\s+` + grammar.AmountPat + `\s+credits\s+in\s+([A-Z]{3})\s+courses\s+from\s+requirement\s+(\d{1,2})$`)
	// substDesigsRe matches designators substituted for a pool: "Up to 2.0
	// credits in GRK or LAT may be substituted for the 5.5 credits in CLA
	// courses from Requirement 4".
	substDesigsRe = regexp.MustCompile(`(?i)^up\s+to\s+` + grammar.AmountPat + `\s+credits?\s+in\s+([A-Z]{3}(?:\s*(?:,|or|and)\s*[A-Z]{3})*)\s+may\s+be\s+substituted\s+for\s+the\s+` + grammar.AmountPat + `\s+credits\s+in\s+([A-Z]{3})\s+courses\s+from\s+requirement\s+(\d{1,2})$`)
	// titledCodesRe matches a note of a list by its title: "Art History:
	// FAH206H1, FAH207H1".
	titledCodesRe = regexp.MustCompile(`^([A-Z][\w&' -]*?):\s*(.+)$`)
)

// substitutedShares makes a pool whose alternatives substitute courses for it
// draw on a union of the pool, the named departments' lists and the
// designators, with a limit on each share. The approved courses the lists only
// exemplify ("notably") stay the row's route.
func (a *Assembler) substitutedShares(row *compRow, lists []*types.CourseList) []*types.CourseList {
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
		t := strings.TrimSuffix(strings.TrimSpace(Clean(alt.Text)), ".")
		if m := substApprovedRe.FindStringSubmatch(t); m != nil && approved == nil && a.substFits(row, pool, m[3:]) {
			approvedShare, _ = strconv.ParseFloat(m[1], 64)
			for _, note := range a.notes {
				g := titledCodesRe.FindStringSubmatch(note)
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
		if m := substDesigsRe.FindStringSubmatch(t); m != nil && desigs == nil && a.substFits(row, pool, m[3:]) {
			desigShare, _ = strconv.ParseFloat(m[1], 64)
			desigs = grammar.DesigRe.FindAllString(m[2], -1)
			limits = append(limits, &types.Requirement{Kind: types.RequirementLimit, Raw: t,
				Req: &types.ReqNode{Type: types.NodeCredits, Designators: slices.Clone(desigs), MaxAmount: grammar.MaxAmount(desigShare)}})
			continue
		}
		return lists
	}
	if approved == nil || codes == nil || desigs == nil || slices.Contains(desigs, pool.Designators[0]) {
		return lists
	}
	codes = grammar.Dedupe(codes)
	limits = append([]*types.Requirement{{Kind: types.RequirementLimit, Raw: strings.TrimSuffix(strings.TrimSpace(Clean(approved.Text)), "."),
		Req: &types.ReqNode{Type: types.NodeCredits, Courses: slices.Clone(codes), MaxAmount: grammar.MaxAmount(approvedShare)}}}, limits...)
	own := &types.ReqNode{Type: types.NodeCredits, Designators: slices.Clone(pool.Designators), Levels: slices.Clone(pool.Levels)}
	// the designators stand in for the pool's credits, so at its levels
	others := &types.ReqNode{Type: types.NodeCredits, Designators: desigs, Levels: slices.Clone(pool.Levels)}
	name := unionName(row, lists)
	lists = append(lists, &types.CourseList{Name: name, Items: []*types.ReqNode{own, {Type: types.NodeCredits, Courses: codes}, others}})
	row.r.Req = programs.OrNode(&types.ReqNode{Type: types.NodeCredits, Amount: pool.Amount, Lists: []string{name}}, approved)
	// a level floor is on the pool's own courses, the strict reading; the route
	// kept marks the floor, so a substitute at the level is Review
	for _, c := range row.children {
		if c.r.Kind != types.RequirementConstraint {
			continue
		}
		grammar.WalkReq(c.r.Req, func(x *types.ReqNode) {
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
func (a *Assembler) substFits(row *compRow, pool *types.ReqNode, m []string) bool {
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

// listCaps makes an item "1.0 credit may be courses on East Asia offered by
// other departments" a limit on the list of those courses that another row
// draws on: the courses count only through that row.
func (a *Assembler) listCaps(lists []*types.CourseList) {
	if a.total == nil {
		return
	}
	for _, r := range a.items {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || len(r.children) > 0 {
			continue
		}
		m := widenOnRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(Clean(n.Text)), "."))
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
		r.r.Req = &types.ReqNode{Type: types.NodeCredits, Lists: []string{name}, MaxAmount: grammar.MaxAmount(v)}
	}
}

// drawsOn reports whether a row other than r draws on list name.
func (a *Assembler) drawsOn(r *compRow, name string) bool {
	found := false
	for _, o := range a.allRows() {
		if o == r || o.r.Kind != types.RequirementAllocation {
			continue
		}
		grammar.WalkReq(o.r.Req, func(x *types.ReqNode) {
			if slices.Contains(x.Lists, name) {
				found = true
			}
		})
	}
	return found
}

var (
	// maxShareGroupRe matches an amount capping one group's share:
	// "Additional 2.5 credits, of which a maximum 1.0 credit may be
	// environmental courses (from Group A below)".
	maxShareGroupRe = regexp.MustCompile(`(?i)^additional\s+` + grammar.AmountPat + `\s+credits\s*,\s*of\s+which\s+a\s+maximum\s+(?:of\s+)?` + grammar.AmountPat + `\s+credits?\s+may\s+be\s+.+?\s*\(\s*from\s+(group\s+[A-Z])\s+below\s*\)$`)
	// remainderFromRe matches "The remainder must be selected from the
	// following: ENV261H1/ ENV360H1".
	remainderFromRe = regexp.MustCompile(`(?i)^the\s+remainder\s+must\s+be\s+selected\s+from\s+the\s+following:\s*(.+)$`)
	// otherApprovedRe matches a list's last member that a unit decides: ",
	// other courses approved by School of the Environment".
	otherApprovedRe = regexp.MustCompile(`(?i)\s*,\s*(other\s+courses\s+approved\s+by\s+.+)$`)
)

// remainderShares reads an item capping a group's share over its remainder: it
// draws on a union of the remainder's courses, starred ones with the footnote's
// permission, and the group's list, capping the group.
func (a *Assembler) remainderShares(row *compRow, lists []*types.CourseList) []*types.CourseList {
	n := row.r.Req
	if row.r.Kind != types.RequirementAllocation || row.r.Label == "" || n == nil || n.Type != types.NodeOr || len(row.children) != 1 || n.Items[0].Type != types.NodeText {
		return lists
	}
	m := maxShareGroupRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(Clean(n.Items[0].Text)), "."))
	rest := row.children[0]
	if m == nil || rest.r.Kind != types.RequirementConstraint || rest.r.Req == nil || rest.r.Req.Type != types.NodeText || len(rest.children) > 0 {
		return lists
	}
	r := remainderFromRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(rest.r.Req.Text), "."))
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
		codes := anyCodeRe.FindAllString(part, -1)
		if codes == nil || !listRestRe.MatchString(anyCodeRe.ReplaceAllString(part, " ")) {
			return lists
		}
		if strings.HasSuffix(strings.TrimSpace(part), "*") {
			starred = append(starred, codes...) // "ENV421Y1/ ENV421H1*": the star covers the pair
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
	items := []*types.ReqNode{{Type: types.NodeCredits, Courses: grammar.Dedupe(plain)}}
	if starred != nil {
		items = append(items, programs.AndNode(&types.ReqNode{Type: types.NodeCredits, Courses: grammar.Dedupe(starred)}, TextNode(types.TextPermission, strings.TrimSpace(strings.TrimPrefix(footnote.Text, "*")))))
	}
	items = append(items, &types.ReqNode{Type: types.NodeCredits, Lists: []string{group.Name}})
	name := unionName(row, lists)
	lists = append(lists, group, &types.CourseList{Name: name, Items: items})
	amount, _ := strconv.ParseFloat(m[1], 64)
	share, _ := strconv.ParseFloat(m[2], 64)
	alts := []*types.ReqNode{{Type: types.NodeCredits, Amount: amount, Lists: []string{name}}}
	if approval != "" {
		alts = append(alts, TextNode(types.TextPermission, approval))
	}
	row.r.Req = programs.OrNode(append(alts, routes...)...)
	row.children = []*compRow{{r: &types.Requirement{Kind: types.RequirementLimit, Raw: strings.TrimSuffix(strings.TrimSpace(Clean(n.Items[0].Text)), "."),
		Req: &types.ReqNode{Type: types.NodeCredits, Lists: []string{group.Name}, MaxAmount: grammar.MaxAmount(share)}}}}
	addRaw(row, rest.r.Raw)
	addRaw(row, note)
	a.notes = slices.DeleteFunc(a.notes, func(s string) bool { return s == note })
	return lists
}

// groupNote returns the list the note titled title holds, the note, and the
// approval of other courses its last member gives, if any.
func (a *Assembler) groupNote(title string) (*types.CourseList, string, string, bool) {
	for _, note := range a.notes {
		g := titledCodesRe.FindStringSubmatch(note)
		if g == nil || !strings.EqualFold(g[1], title) {
			continue
		}
		members, approval := strings.TrimSuffix(strings.TrimSpace(g[2]), "."), ""
		if loc := otherApprovedRe.FindStringSubmatchIndex(members); loc != nil {
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
	// programCoursesRe matches "3.0 credits from courses offered by the
	// Canadian Studies Program, including additional courses from Requirements
	// 1 and 2".
	programCoursesRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits\s+from\s+courses\s+(offered\s+by\s+the\s+.+?\s+program)\s*,\s*including\s+additional\s+courses\s+from\s+requirements\s+(\d{1,2}(?:\s*(?:,|and|or)\s*\d{1,2})*)$`)
	// alsoCompletedFromRe matches "Up to 1.0 credit of this requirement
	// can also be completed from HST211H1/ PHS211H1".
	alsoCompletedFromRe = regexp.MustCompile(`(?i)^up\s+to\s+` + grammar.AmountPat + `\s+credits?\s+of\s+this\s+requirement\s+can\s+also\s+be\s+completed\s+from\s*:?\s*(.+)$`)
)

// programShares reads an item of a program's courses, part of which may come
// from listed courses: it draws on a union of the program's courses (a content
// condition), the rows it names and the listed ones, capping those.
func (a *Assembler) programShares(row *compRow, lists []*types.CourseList) []*types.CourseList {
	n := row.r.Req
	if row.r.Kind != types.RequirementAllocation || row.r.Label == "" || n == nil || n.Type != types.NodeOr || len(n.Items) < 2 || len(row.children) > 0 ||
		n.Items[0].Type != types.NodeText || n.Items[1].Type != types.NodeText {
		return lists
	}
	m := programCoursesRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(Clean(n.Items[0].Text)), "."))
	c := alsoCompletedFromRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(Clean(n.Items[1].Text)), "."))
	if m == nil || c == nil {
		return lists
	}
	codes := anyCodeRe.FindAllString(c[2], -1)
	if codes == nil || !listRestRe.MatchString(anyCodeRe.ReplaceAllString(c[2], " ")) {
		return lists
	}
	var rows []*compRow
	for _, l := range reqNumRe.FindAllString(m[3], -1) {
		r := a.labelled(l)
		if r == nil || r == row {
			return lists
		}
		rows = append(rows, r)
	}
	ref := &types.ReqNode{Type: types.NodeCredits}
	codes = grammar.Dedupe(codes)
	name := unionName(row, lists)
	l := &types.CourseList{Name: name, Items: []*types.ReqNode{
		programs.AndNode(&types.ReqNode{Type: types.NodeCredits}, TextNode(types.TextContent, m[2])),
		ref,
		{Type: types.NodeCredits, Courses: codes},
	}}
	lists = append(lists, l)
	a.reqRefs = append(a.reqRefs, compReqRef{node: ref, rows: rows, list: l})
	amount, _ := strconv.ParseFloat(m[1], 64)
	share, _ := strconv.ParseFloat(c[1], 64)
	row.r.Req = programs.OrNode(append([]*types.ReqNode{{Type: types.NodeCredits, Amount: amount, Lists: []string{name}}}, n.Items[2:]...)...)
	a.rowUnder(row, &types.Requirement{Kind: types.RequirementLimit, Raw: strings.TrimSuffix(strings.TrimSpace(Clean(n.Items[1].Text)), "."),
		Req: &types.ReqNode{Type: types.NodeCredits, Courses: slices.Clone(codes), MaxAmount: grammar.MaxAmount(share)}})
	return lists
}

// cognateItemRe matches an item capping the cognate courses its note
// lists: "Up to 1.0 credit from Cognate Courses:".
var cognateItemRe = regexp.MustCompile(`(?i)^(?:\d{1,2}\.\s+)?up\s+to\s+` + grammar.AmountPat + `\s+credits?\s+from\s+cognate\s+courses\s*:?$`)

// cognateItem reads a cognateItemRe item as a limit, when the other items
// are closed: the plain total draws on their courses and the cognates, and the
// route's text keeps it Review short of the list.
func (a *Assembler) cognateItem(lists []*types.CourseList) []*types.CourseList {
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
		if n != nil && n.Type == types.NodeOr && len(n.Items) > 0 && n.Items[0].Type == types.NodeText && cognateItemRe.MatchString(strings.TrimSpace(Clean(n.Items[0].Text))) && item == nil {
			item = r
			continue
		}
		if n == nil || OpenText(n) || len(r.children) > 0 {
			return lists
		}
		grammar.WalkReq(n, func(x *types.ReqNode) {
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
		if alt.Type != types.NodeText || !cognateRe.MatchString(alt.Text) || !strings.Contains(t.r.Raw, strings.TrimSuffix(alt.Text, ".")) {
			return lists // only the header's restatement may join the item
		}
	}
	items, except, perm, ok := listMembersPerm(item.r.Notes[0])
	if !ok || except != nil || len(items) != 1 || items[0].Courses == nil || items[0].Designators != nil {
		return lists
	}
	m := cognateItemRe.FindStringSubmatch(strings.TrimSpace(Clean(n.Items[0].Text)))
	share, _ := strconv.ParseFloat(m[1], 64)
	cognates := items[0].Courses
	name := unionName(item, lists)
	lists = append(lists, &types.CourseList{Name: name, Items: []*types.ReqNode{{Type: types.NodeCredits, Courses: grammar.Dedupe(own)}, {Type: types.NodeCredits, Courses: slices.Clone(cognates)}}})
	pool := &types.ReqNode{Type: types.NodeCredits, Amount: t.r.Req.Amount, Lists: []string{name}}
	t.r.Req = pool
	if perm != "" {
		t.r.Req = programs.OrNode(pool, TextNode(types.TextPermission, perm))
	}
	item.r.Kind, item.r.Label = types.RequirementLimit, ""
	item.r.Req = &types.ReqNode{Type: types.NodeCredits, Courses: slices.Clone(cognates), MaxAmount: grammar.MaxAmount(share)}
	addRaw(item, item.r.Notes[0])
	item.r.Notes = nil
	return lists
}
