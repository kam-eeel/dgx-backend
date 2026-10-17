package reqs

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Lead lines over too few codes for listLead's sub-lists, exemption
// replacements, and pools that draw on a union or a list by name.

var (
	// complCodeTitleLineRe matches a course and its title: "SDS255H1:
	// Histories of Sexuality".
	complCodeTitleLineRe = regexp.MustCompile(`^(` + codePat + `)\s*:\s*[A-Za-z][^\d;/]*$`)
	// complAreaLineRe matches an area and its courses: "Statistics: STA302H1,
	// STA313H1;". The groups are the area and the codes.
	complAreaLineRe = regexp.MustCompile(`^([A-Z][A-Za-z&' ]{1,60}):\s*(` + codePat + `(?:\s*,\s*` + codePat + `)*)\s*[;.]?$`)
	// complAreaOrLineRe matches an area line with a rule of its own: "Astronomy:
	// AST430H1, or AST425Y1 (0.5 credit from point 1 above).".
	complAreaOrLineRe = regexp.MustCompile(`^[A-Z][A-Za-z&' ]{1,60}:\s*(` + codePat + `),?\s+or\s+` + codePat + `\b.*$`)
	// complNamedCoursesLeadRe matches a lead naming the courses under it: "1.0
	// credit from the SDS Foundational Courses:".
	complNamedCoursesLeadRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+from\s+the\s+(?-i:[A-Z])[\w&' -]*?\s+courses\s*:$`)
	// complCoursesSumLeadRe matches a lead naming courses and their total: "SDS
	// Foundational Courses (1.5 credits):".
	complCoursesSumLeadRe = regexp.MustCompile(`(?i)^(?-i:[A-Z])[\w&' -]*?\s+courses\s+\((\d+(?:\.\d+)?)\s+credits?\)\s*:$`)
	// complFollowingListLeadRe matches a lead over one line of courses: "(a
	// minimum of one 0.5 credit from the following list):".
	complFollowingListLeadRe = regexp.MustCompile(`(?i)\((?:a\s+minimum\s+of\s+)?(?:one\s+)?(\d+(?:\.\d+)?)\s+credits?\s+from\s+the\s+following\s+list\)\s*:$`)
	// complAllCoursesLineRe matches that line: "All 400-level SDS and JNS
	// courses.".
	complAllCoursesLineRe = regexp.MustCompile(`^All\s+(.+?\s+courses)\.?$`)
)

// leadLines reads an item whose lead names the courses on the lines under it,
// when they have too few codes for listLead: a pool of them, all of them, an
// or of a pool per area, or one line's pool. row is the item's allocation, not
// yet added. It reports whether it read the item.
func (a *compAssembler) leadLines(b *compBlock, row *compRow, ctx compCtx) bool {
	lead := strings.TrimSpace(b.text)
	kids := b.children
	if len(kids) == 0 || !strings.HasSuffix(lead, ":") {
		return false
	}
	plain := func(c *compBlock) bool {
		return c.kind != blockHeading && len(c.children) == 0 && !complAmountRe.MatchString(c.text)
	}
	// text is a line as written, with its label: "Statistics: STA302H1, ..."
	text := func(c *compBlock) string {
		if c.kind == blockLabelled && c.label != "" {
			return c.label + ": " + strings.TrimSpace(c.text)
		}
		return strings.TrimSpace(c.text)
	}
	run := func(re *regexp.Regexp) int {
		n := 0
		for n < len(kids) && plain(kids[n]) && re.MatchString(text(kids[n])) {
			n++
		}
		return n
	}
	var body *types.ReqNode
	n := 0
	switch {
	case complNamedCoursesLeadRe.MatchString(lead):
		amount, _ := strconv.ParseFloat(complNamedCoursesLeadRe.FindStringSubmatch(lead)[1], 64)
		if n = run(complCodeTitleLineRe); n < 2 || n < len(kids) && complCodeTitleLineRe.MatchString(text(kids[n])) {
			return false
		}
		var codes []string
		for _, c := range kids[:n] {
			codes = append(codes, complCodeTitleLineRe.FindStringSubmatch(text(c))[1])
		}
		if weightOf(codes) < amount-1e-9 {
			return false
		}
		body = &types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: dedupe(codes)}
	case complCoursesSumLeadRe.MatchString(lead):
		amount, _ := strconv.ParseFloat(complCoursesSumLeadRe.FindStringSubmatch(lead)[1], 64)
		if n = run(complCodeTitleLineRe); n < 2 || n < len(kids) && complCodeTitleLineRe.MatchString(text(kids[n])) {
			return false
		}
		var codes []string
		for _, c := range kids[:n] {
			codes = append(codes, complCodeTitleLineRe.FindStringSubmatch(text(c))[1])
		}
		if math.Abs(weightOf(codes)-amount) > 1e-9 {
			return false // all of them, so they must add up to the amount
		}
		body = andNode(courseNodes(dedupe(codes))...)
	case complOneOfLeadRe.MatchString(lead) && complAmountRe.MatchString(lead):
		amount, err := strconv.ParseFloat(strings.Fields(complAmountRe.FindString(lead))[0], 64)
		if n = run(complAreaLineRe); err != nil || n < 2 {
			return false
		}
		var opts []*types.ReqNode
		for _, c := range kids[:n] {
			codes := codeRe.FindAllString(complAreaLineRe.FindStringSubmatch(text(c))[2], -1)
			switch w := weightOf(codes); {
			case len(codes) == 1 && math.Abs(w-amount) < 1e-9:
				opts = append(opts, &types.ReqNode{Type: types.NodeCourse, Code: codes[0]})
			case w >= amount-1e-9:
				opts = append(opts, &types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: dedupe(codes)})
			default:
				return false // an area that can't make the amount
			}
		}
		if c := kids[min(n, len(kids)-1)]; n < len(kids) && c.kind != blockHeading && len(c.children) == 0 {
			// an area with a rule of its own: its first course, and the line
			if m := complAreaOrLineRe.FindStringSubmatch(text(c)); m != nil {
				if math.Abs(courseWeight(m[1])-amount) < 1e-9 {
					opts = append(opts, &types.ReqNode{Type: types.NodeCourse, Code: m[1]})
				}
				opts = append(opts, textNode(types.TextOther, cleanNote(text(c))))
				n++
			}
		}
		body = orNode(opts...)
	case complFollowingListLeadRe.MatchString(lead) && len(kids) >= 1 && plain(kids[0]) && complAllCoursesLineRe.MatchString(text(kids[0])):
		m := complFollowingListLeadRe.FindStringSubmatch(lead)
		p, _ := parseCompletionText(m[1] + " credit from " + complAllCoursesLineRe.FindStringSubmatch(text(kids[0]))[1])
		if !isPool(p) || p.Courses != nil || p.Designators == nil || hasText(p) {
			return false
		}
		body, n = p, 1
	default:
		return false
	}
	row.r.Req = body
	for _, c := range kids[:n] {
		row.r.Raw += "\n" + a.line(c)
	}
	a.add(row, ctx.owner)
	a.leadRows[b] = row
	a.leadRows[kids[n-1]] = row
	a.children(kids[n:], b.text, ctx, row)
	return true
}

// weightOf returns the credits of codes together.
func weightOf(codes []string) float64 {
	sum := 0.0
	for _, c := range dedupe(slices.Clone(codes)) {
		sum += courseWeight(c)
	}
	return sum
}

var (
	// complExemptAnyRe matches an exemption replaced by any course of some
	// designators: "Students exempted from FSL220H1/ FSL222H1 may replace it with
	// any FRE 200+ or FSL 300+ level course".
	complExemptAnyRe = regexp.MustCompile(`(?i)^students\s+exempted\s+from\s+` + complRelaxCodesPat + `\s+may\s+replace\s+(?:it|them)\s+with\s+any\s+((?-i:[A-Z]{3})(?:\s+[1-4]00\+)?(?:\s+or\s+(?-i:[A-Z]{3})(?:\s+[1-4]00\+)?)*)(?:\s+level)?\s+course$`)
	// complExemptCombinationRe matches an exemption replaced by a requirement's
	// pool or a filter: "Students exempted from any of FSL220H1/ ... must replace
	// these credits with any combination of the following two options: ...".
	complExemptCombinationRe = regexp.MustCompile(`(?i)^students\s+exempted\s+from\s+any\s+of\s+` + complRelaxCodesPat + `\s+must\s+replace\s+these\s+credits\s+with\s+any\s+combination\s+of\s+the\s+following\s+two\s+options:\s*(?:\(a\)\s*)?any\s+additional\s+credit\(s\)\s+from\s+category\s+\((\d{1,2})\)\s+above,?\s+or\s+(?:\(b\)\s*)?any\s+((?-i:[A-Z]{3}))\s+credit\(s\)\s+at\s+the\s+([1-4]00(?:/[1-4]00)*)\s+level$`)
	// complExemptFilterRe matches one filter of complExemptAnyRe: "FRE 200+".
	complExemptFilterRe = regexp.MustCompile(`([A-Z]{3})(?:\s+([1-4])00\+)?`)
	// complPairAdditionalRe matches a course and more credits, or a full course:
	// "( FSL420H1, 0.5 additional credit of FRE at the 200+ level)/ FSL421Y1".
	// The groups are the course, the amount, the filters, the full course.
	complPairAdditionalRe = regexp.MustCompile(`(?i)^\(\s*(` + codePat + `),\s*(\d+(?:\.\d+)?)\s+additional\s+credits?\s+of\s+((?-i:[A-Z]{3})\s+at\s+the\s+[1-4]00\+\s+level(?:\s+or\s+(?-i:[A-Z]{3})\s+at\s+the\s+[1-4]00\+\s+level)*)\s*\)\s*/\s*(` + codePat + `)$`)
	// complLevelFilterRe matches one filter of complPairAdditionalRe: "FSL at
	// the 300+ level".
	complLevelFilterRe = regexp.MustCompile(`([A-Z]{3})\s+at\s+the\s+([1-4])00\+\s+level`)
	// complCountFloorRe matches a count of courses with a floor from a list: "Any
	// 2 (1.0 credit) 400-level GGR courses, at least one of which must be from
	// Group E". The groups are the amount, the level, the designator, the list.
	complCountFloorRe = regexp.MustCompile(`(?i)^any\s+(?:\d+|two|three|four)\s+\(\s*(\d+(?:\.\d+)?)\s+credits?\s*\)\s+([1-4])00-level\s+((?-i:[A-Z]{3}))\s+courses,\s+at\s+least\s+one\s+of\s+which\s+must\s+be\s+from\s+(group\s+[A-Z])$`)
)

// leftoverPools reads, once the lists are known, the text pools that need a
// list of their own or a list by name: pairAdditional, optionsPool,
// countFloor, jointDesignators and listAlternative.
func (a *compAssembler) leftoverPools(ls compListRefs) {
	var walk func(rows []*compRow)
	walk = func(rows []*compRow) {
		for _, r := range rows {
			walk(r.children)
			if r.r.Kind != types.RequirementAllocation || r.r.Req == nil {
				continue
			}
			r.r.Req = listAlternatives(r.r.Req, ls)
			n := r.r.Req
			text := n
			if n.Type == types.NodeOr && len(n.Items) > 1 && n.Items[0].Type == types.NodeText &&
				!slices.ContainsFunc(n.Items[1:], func(it *types.ReqNode) bool { return it.Type != types.NodeText || it.Kind != types.TextPermission }) {
				text = n.Items[0] // or(text, routes)
			}
			if text.Type != types.NodeText || text.Kind != types.TextOther {
				continue
			}
			body := a.pairAdditional(r, text.Text, ls)
			if body == nil {
				body = a.optionsPool(r, text.Text, ls)
			}
			if body == nil {
				body = countFloor(text.Text, ls)
			}
			if body == nil {
				body = jointDesignators(text.Text)
			}
			if body == nil {
				body = listAlternative(text.Text, ls)
			}
			if body == nil {
				continue
			}
			if text == n {
				r.r.Req = body
			} else {
				n.Items[0] = body
			}
		}
	}
	walk(a.items)
	walk(a.program)
}

// exemptReplacements reads the exemptions replaced by any course of some
// designators, once the routes are placed beside the courses, and returns
// lists with the unions they make.
func (a *compAssembler) exemptReplacements(lists []*types.CourseList) []*types.CourseList {
	union := func(r *compRow, items []*types.ReqNode) (string, bool) {
		if r.r.Label == "" {
			return "", false
		}
		base := "Requirement " + r.r.Label + " courses"
		name := base
		for k := 2; slices.ContainsFunc(lists, func(l *types.CourseList) bool { return l.Name == name }); k++ {
			name = base + " (" + strconv.Itoa(k) + ")"
		}
		lists = append(lists, &types.CourseList{Name: name, Items: items})
		return name, true
	}
	var walk func(rows []*compRow)
	walk = func(rows []*compRow) {
		for _, r := range rows {
			walk(r.children)
			if r.r.Kind == types.RequirementAllocation && r.r.Req != nil {
				r.r.Req = exemptAny(r, r.r.Req, union)
			}
		}
	}
	walk(a.items)
	walk(a.program)
	// one union per sentence: the first row's
	made := map[string]string{}
	var combine func(rows []*compRow)
	combine = func(rows []*compRow) {
		for _, r := range rows {
			combine(r.children)
			if r.r.Kind == types.RequirementAllocation && r.r.Req != nil {
				r.r.Req = a.exemptCombination(r, r.r.Req, union, made)
			}
		}
	}
	combine(a.items)
	return lists
}

// exemptCombination replaces a complExemptCombinationRe alternative of an or
// whose courses it names with and(credits{the exempted credits, a union of the
// requirement's list pool and the filter}, text{placement}).
func (a *compAssembler) exemptCombination(r *compRow, n *types.ReqNode, union func(*compRow, []*types.ReqNode) (string, bool), made map[string]string) *types.ReqNode {
	for i, it := range n.Items {
		n.Items[i] = a.exemptCombination(r, it, union, made)
	}
	if n.Type != types.NodeOr {
		return n
	}
	for i, it := range n.Items {
		if it.Type != types.NodeText {
			continue
		}
		m := complExemptCombinationRe.FindStringSubmatch(complClean(it.Text))
		if m == nil {
			continue
		}
		v := exemptedCredits(n, codeRe.FindAllString(m[1], -1))
		if v == 0 {
			continue
		}
		name, ok := made[it.Text]
		if !ok {
			var pool *types.ReqNode
			for _, x := range a.items {
				if x.r.Label == m[2] && x.r.Kind == types.RequirementAllocation && x.r.Req != nil && x.r.Req.Type == types.NodeCredits && x.r.Req.Courses == nil && x.r.Req.Designators == nil && x.r.Req.Lists != nil {
					pool = &types.ReqNode{Type: types.NodeCredits, Lists: slices.Clone(x.r.Req.Lists), Levels: slices.Clone(x.r.Req.Levels)}
				}
			}
			if pool == nil {
				continue
			}
			f := &types.ReqNode{Type: types.NodeCredits, Designators: []string{m[3]}}
			for _, l := range strings.Split(m[4], "/") {
				lv, _ := strconv.Atoi(l)
				f.Levels = append(f.Levels, lv)
			}
			if name, ok = union(r, []*types.ReqNode{pool, f}); !ok {
				continue
			}
			made[it.Text] = name
		}
		n.Items[i] = andNode(&types.ReqNode{Type: types.NodeCredits, Amount: v, Lists: []string{name}}, textNode(types.TextPlacement, it.Text))
	}
	return n
}

// exemptedCredits returns the credits of the or n's heaviest alternative made
// only of named courses: 1.0 for "( FSL220H1, FSL222H1)". It is 0 when none is.
func exemptedCredits(n *types.ReqNode, named []string) float64 {
	best := 0.0
	for _, it := range n.Items {
		if it.Type == types.NodeText {
			continue
		}
		sum, ok := 0.0, true
		walkReq(it, func(x *types.ReqNode) {
			switch x.Type {
			case types.NodeCourse:
				ok = ok && slices.Contains(named, x.Code)
				sum += courseWeight(x.Code)
			case types.NodeAnd:
			default:
				ok = false
			}
		})
		if ok && sum > best {
			best = sum
		}
	}
	return best
}

// exemptAny replaces a complExemptAnyRe alternative of an or, whose courses
// are among the or's other alternatives, with and(credits{the exempted
// credits or 0.5, the filters or their union}, text{placement}).
func exemptAny(r *compRow, n *types.ReqNode, union func(*compRow, []*types.ReqNode) (string, bool)) *types.ReqNode {
	for i, it := range n.Items {
		n.Items[i] = exemptAny(r, it, union)
	}
	if n.Type != types.NodeOr {
		return n
	}
	have := map[string]bool{}
	for _, it := range n.Items {
		if it.Type != types.NodeText {
			walkReq(it, func(x *types.ReqNode) { have[x.Code] = have[x.Code] || x.Code != "" })
		}
	}
	for i, it := range n.Items {
		if it.Type != types.NodeText {
			continue
		}
		m := complExemptAnyRe.FindStringSubmatch(complClean(it.Text))
		if m == nil || slices.ContainsFunc(codeRe.FindAllString(m[1], -1), func(c string) bool { return !have[c] }) {
			continue
		}
		var filters []*types.ReqNode
		for _, f := range complExemptFilterRe.FindAllStringSubmatch(m[2], -1) {
			filters = append(filters, levelFilter(f[1], f[2]))
		}
		pool := &types.ReqNode{Type: types.NodeCredits, Amount: 0.5}
		if v := exemptedCredits(n, codeRe.FindAllString(m[1], -1)); v > 0 {
			pool.Amount = v // the credits the exemption takes
		}
		if len(filters) == 1 {
			pool.Designators, pool.Levels = filters[0].Designators, filters[0].Levels
		} else if name, ok := union(r, filters); ok {
			pool.Lists = []string{name}
		} else {
			continue
		}
		n.Items[i] = andNode(pool, textNode(types.TextPlacement, it.Text))
	}
	return n
}

// levelFilter returns a pool's filter: designator d at level l00 and up (""
// for any level).
func levelFilter(d, l string) *types.ReqNode {
	f := &types.ReqNode{Type: types.NodeCredits, Designators: []string{d}}
	if l != "" {
		for v := int(l[0]-'0') * 100; v <= 400; v += 100 {
			f.Levels = append(f.Levels, v)
		}
	}
	return f
}

// pairAdditional reads complPairAdditionalRe as or(and(course, credits{the
// filters, excluding the course}), full course): the credits are additional.
func (a *compAssembler) pairAdditional(r *compRow, t string, ls compListRefs) *types.ReqNode {
	m := complPairAdditionalRe.FindStringSubmatch(complClean(t))
	if m == nil {
		return nil
	}
	amount, _ := strconv.ParseFloat(m[2], 64)
	var filters []*types.ReqNode
	for _, f := range complLevelFilterRe.FindAllStringSubmatch(m[3], -1) {
		filters = append(filters, levelFilter(f[1], f[2]))
	}
	pool := &types.ReqNode{Type: types.NodeCredits, Amount: amount, Exclude: []string{m[1]}}
	if len(filters) == 1 {
		pool.Designators, pool.Levels = filters[0].Designators, filters[0].Levels
	} else if name, _, ok := a.addUnion(r, filters, ls); ok {
		pool.Lists = []string{name}
	} else {
		return nil
	}
	return orNode(andNode(&types.ReqNode{Type: types.NodeCourse, Code: m[1]}, pool), &types.ReqNode{Type: types.NodeCourse, Code: m[4]})
}

// countFloor reads complCountFloorRe as and(credits{amount, designator,
// level}, credits{0.5 of those from the list}): the floor's course is one of
// those counted.
func countFloor(t string, ls compListRefs) *types.ReqNode {
	m := complCountFloorRe.FindStringSubmatch(complClean(t))
	if m == nil {
		return nil
	}
	l := ls.byName(strings.Replace(m[4], "group", "Group", 1))
	if l == nil || l.bad {
		return nil
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	level := int(m[2][0]-'0') * 100
	pool := &types.ReqNode{Type: types.NodeCredits, Amount: amount, Designators: []string{m[3]}, Levels: []int{level}}
	floor := &types.ReqNode{Type: types.NodeCredits, Amount: 0.5, Designators: []string{m[3]}, Levels: []int{level}, Lists: []string{l.list.Name}}
	return andNode(pool, floor)
}

var (
	// complOptionsPoolRe matches an amount from options separated by ";": "1.0
	// credit from the following options: MCS329H1/ MCS329Y1; ...".
	complOptionsPoolRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+from\s+the\s+following\s+options:\s*(.+)$`)
	// complNotTakenRefRe matches another requirement's courses not used there:
	// "courses listed in requirement 3 not already taken".
	complNotTakenRefRe = regexp.MustCompile(`(?i)^courses\s+listed\s+in\s+requirement\s+(\d{1,2})\s+not\s+already\s+taken$`)
	// complRelatedOptionsRe matches options ending in a course the topic decides:
	// "1.0 credit from: EDS356H1, ..., or a course related to Education and
	// Society at the 300+ level ...". Group 4 is that course's level.
	complRelatedOptionsRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+from:\s*(.+?),\s+or\s+(an?\s+course\s+related\s+to\s+.+?\s+at\s+the\s+([1-4])00\+\s+level\b.*)$`)
	// complListBelowRe matches a list by name: "cognate courses listed below".
	complListBelowRe = regexp.MustCompile(`(?i)^(\w+\s+courses)\s+listed\s+below$`)
)

// optionsPool reads complOptionsPoolRe as credits from a union of its
// options, every one of which must read. With complRelatedOptionsRe the
// topic's course is the lenient alternative: and(credits{amount, level},
// text{content}).
func (a *compAssembler) optionsPool(r *compRow, t string, ls compListRefs) *types.ReqNode {
	m := complOptionsPoolRe.FindStringSubmatch(complClean(t))
	var opts []string
	var related *types.ReqNode
	switch rm := complRelatedOptionsRe.FindStringSubmatch(strings.TrimSuffix(complClean(t), ".")); {
	case m != nil:
		opts = splitTop(m[2], ';')
	case rm != nil:
		m, opts = rm, strings.Split(rm[2], ", ")
		v, _ := strconv.ParseFloat(rm[1], 64)
		related = andNode(&types.ReqNode{Type: types.NodeCredits, Amount: v, Levels: parseLevels(rm[4] + "00+-level")}, textNode(types.TextContent, rm[3]))
	default:
		return nil
	}
	var items []*types.ReqNode
	var codes []string
	var refs []compReqRef
	for _, o := range opts {
		o = strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(o), "or ")), ".")
		if rm := complNotTakenRefRe.FindStringSubmatch(o); rm != nil {
			row := a.labelled(rm[1])
			if row == nil || row == r {
				return nil
			}
			ref := &types.ReqNode{Type: types.NodeCredits}
			items = append(items, ref)
			refs = append(refs, compReqRef{node: ref, rows: []*compRow{row}})
			continue
		}
		if lm := complListBelowRe.FindStringSubmatch(o); lm != nil {
			var found *compList
			for _, l := range ls {
				if strings.EqualFold(l.list.Name, lm[1]) && !l.bad {
					found = l
				}
			}
			if found == nil {
				return nil
			}
			items = append(items, &types.ReqNode{Type: types.NodeCredits, Lists: []string{found.list.Name}})
			continue
		}
		if !isCodeList(o) {
			return nil
		}
		codes = append(codes, complAnyCodeRe.FindAllString(o, -1)...)
	}
	if codes != nil {
		items = append(items, &types.ReqNode{Type: types.NodeCredits, Courses: dedupe(codes)})
	}
	if len(items) < 2 {
		return nil
	}
	name, l, ok := a.addUnion(r, items, ls)
	if !ok {
		return nil
	}
	for _, ref := range refs {
		ref.list = l.list
		a.reqRefs = append(a.reqRefs, ref)
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	return orNode(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Lists: []string{name}}, related)
}

var (
	// complLanguageListAltRe matches a list of language courses: "1.0 credit at
	// any level of an Asian language (see Accepted Language Courses below)".
	complLanguageListAltRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+at\s+any\s+level\s+(?:of|in)\s+an?\s+\w+\s+language\s+\(see\s+(.+?)\s+below\)$`)
	// complGroupCourseAltRe matches a course from a group: "0.5 credit in a
	// quantitative or qualitative methods course from Group C".
	complGroupCourseAltRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+in\s+an?\s+[\w ]+?\s+course\s+from\s+(group\s+[A-Z])(?:\s+to\s+fulfil+\s+the\s+[\w ]+)?$`)
	// complJointDesignatorsRe matches a level's courses of a designator and its
	// joint ones: "1.0 credit in 300-level CAS courses (including JHA, JPA, and
	// SAS)". Group 4 is an optional second level.
	complJointDesignatorsRe = regexp.MustCompile(`^(\d+(?:\.\d+)?)\s+credits?\s+(?:in|from)\s+([1-4])00-level\s+([A-Z]{3})\s+courses(?:\s+and/or\s+([1-4])00-level\s+courses)?\s+\(including\s+([A-Z]{3}(?:,\s*[A-Z]{3})*,?\s+and\s+[A-Z]{3})\)$`)
)

// listAlternatives reads the text alternatives of n's or nodes that draw on a
// list by name.
func listAlternatives(n *types.ReqNode, ls compListRefs) *types.ReqNode {
	for i, it := range n.Items {
		n.Items[i] = listAlternatives(it, ls)
	}
	if n.Type != types.NodeOr {
		return n
	}
	for i, it := range n.Items {
		if it.Type == types.NodeText && it.Kind == types.TextOther {
			if body := listAlternative(it.Text, ls); body != nil {
				n.Items[i] = body
			}
		}
	}
	return n
}

// listAlternative reads text that draws on a list by name as credits from
// it, or nil.
func listAlternative(t string, ls compListRefs) *types.ReqNode {
	t = complClean(t)
	m := complLanguageListAltRe.FindStringSubmatch(t)
	if m == nil {
		m = complGroupCourseAltRe.FindStringSubmatch(t)
	}
	if m == nil {
		return nil
	}
	var found *compList
	for _, l := range ls {
		if strings.EqualFold(l.list.Name, strings.Trim(m[2], "_")) && !l.bad {
			found = l
		}
	}
	if found == nil {
		return nil
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	return &types.ReqNode{Type: types.NodeCredits, Amount: amount, Lists: []string{found.list.Name}}
}

// jointDesignators reads complJointDesignatorsRe. With "and/or 400-level
// courses", both levels of the designator is the strict reading and the
// sentence the lenient one (Review).
func jointDesignators(t string) *types.ReqNode {
	m := complJointDesignatorsRe.FindStringSubmatch(complClean(t))
	if m == nil {
		return nil
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	pool := &types.ReqNode{Type: types.NodeCredits, Amount: amount, Designators: append([]string{m[3]}, desigRe.FindAllString(m[5], -1)...), Levels: []int{int(m[2][0]-'0') * 100}}
	if m[4] == "" {
		return pool
	}
	pool.Levels = dedupeInts(append(pool.Levels, int(m[4][0]-'0')*100))
	return orNode(pool, textNode(types.TextAmbiguity, t))
}

var (
	// complExclusionNoteRe matches a note that earlier courses exclude an item's:
	// "If you completed CSC148H1 before being admitted to the Minor, please note
	// that this course is an exclusion to EEB125H1".
	complExclusionNoteRe = regexp.MustCompile(`(?i)^if\s+you\s+completed\s+.+?\s+before\s+being\s+admitted\s+to\s+the\s+\w+,\s+please\s+note\s+that\s+(?:these\s+courses\s+are\s+exclusions|this\s+course\s+is\s+an\s+exclusion)\s+to\b`)
	// complInsteadExtraRe matches the alternative such a note gives: "to meet
	// requirement 1 or can instead complete an additional 0.5 credit from the list
	// of courses in requirement 7, for a total of 1.5 credits for requirement 7".
	complInsteadExtraRe = regexp.MustCompile(`(?i)\bto\s+meet\s+requirement\s+(\d{1,2})\s+or\s+can\s+instead\s+complete\s+an\s+additional\s+(\d+(?:\.\d+)?)\s+credits?\s+from\s+the\s+list\s+of\s+courses\s+in\s+requirement\s+(\d{1,2}),\s+for\s+a\s+total\s+of\s+\d+(?:\.\d+)?\s+credits?\s+for\s+requirement\s+(\d{1,2})$`)
	// complCoursesThenRouteRe matches listed courses ending in an approval
	// route: "At least 1.0 credit from the following courses: BCB410H1/ ... with
	// prior approval from the Associate Chair".
	complCoursesThenRouteRe = regexp.MustCompile(`(?i)^at\s+least\s+(\d+(?:\.\d+)?)\s+credits?\s+from\s+the\s+following\s+courses:\s*((?:` + codePat + `\s*/\s*)+)(\d+(?:\.\d+)?\s+or\s+\d+(?:\.\d+)?\s+credits?\s+from\s+.+\bwith\s+prior\s+approval\s+from\b.+)$`)
)

// exclusionAlternatives turns complExclusionNoteRe rows into notes and gives
// the item named by complInsteadExtraRe or(its body, and(credits from the
// other item's courses, text)): Review, since admission dates aren't on the
// transcript. ASMIN0160.
func (a *compAssembler) exclusionAlternatives() {
	var walk func(rs []*compRow)
	walk = func(rs []*compRow) {
		for _, r := range slices.Clone(rs) {
			walk(r.children)
			if n := r.r.Req; n != nil && n.Type == types.NodeText && r.r.Kind == types.RequirementAllocation &&
				len(r.children) == 0 && complExclusionNoteRe.MatchString(complClean(n.Text)) && a.detach(r) {
				a.notes = append(append(a.notes, cleanNote(n.Text)), r.r.Notes...)
			}
		}
	}
	walk(a.items)
	for _, s := range slices.Clone(a.notes) {
		m := complInsteadExtraRe.FindStringSubmatch(complClean(s))
		if m == nil || m[3] != m[4] {
			continue
		}
		item, other := a.labelled(m[1]), a.labelled(m[3])
		if item == nil || other == nil || item == other || item.r.Req == nil || other.r.Req == nil {
			continue
		}
		if t := other.r.Req; t.Type == types.NodeText {
			p := complCoursesThenRouteRe.FindStringSubmatch(complClean(t.Text))
			if p == nil {
				continue
			}
			v, _ := strconv.ParseFloat(p[1], 64)
			other.r.Req = orNode(&types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: dedupe(codeRe.FindAllString(p[2], -1))},
				textNode(types.TextPermission, strings.TrimSuffix(p[3], ".")))
		}
		pool := other.r.Req
		if pool.Type == types.NodeOr {
			pool = pool.Items[0]
		}
		if !isPool(pool) || pool.Courses == nil || pool.Lists != nil {
			continue
		}
		v, _ := strconv.ParseFloat(m[2], 64)
		alt := andNode(&types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: slices.Clone(pool.Courses)}, textNode(types.TextOther, cleanNote(s)))
		if !slices.ContainsFunc(item.r.Req.Items, func(x *types.ReqNode) bool { return x.Type == types.NodeAnd && sameNode(x.Items[0], alt.Items[0]) }) {
			item.r.Req = orNode(item.r.Req, alt) // once for notes that give the same alternative
		}
		addRaw(item, cleanNote(s))
		a.notes = slices.DeleteFunc(a.notes, func(n string) bool { return n == s })
	}
}
