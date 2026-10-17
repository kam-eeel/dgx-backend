package reqs

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Approval routes and alternatives: adding them to rows, and placing the routes
// stated for the program rather than an item on the rows they concern.

// alternative adds s as another way to meet row: or(strict, text), its kind
// saying what decides it. A permission naming its wider pool is bounded by it
// once every row is read.
func (a *compAssembler) alternative(row *compRow, s string) {
	a.addRoute(row, s, textNode(altKind(s), s))
}

// altKind is the kind of an alternative's text: a permission when a person
// decides, else other. Never membership: "count toward the program" is about
// courses, not programs.
func altKind(s string) types.TextKind {
	if classify(cleanNote(s)) == types.TextPermission {
		return types.TextPermission
	}
	return types.TextOther
}

// approval adds a route only a person can grant to target as or(strict,
// permission text); a pool it names bounds it later. A program-level route
// waits until every row is read, then goes to the rows it names, else a note.
func (a *compAssembler) approval(target *compRow, s string) {
	a.addRoute(target, s, textNode(types.TextPermission, cleanNote(s)))
}

// addRoute adds route s to row as or(strict, alt). A nil row is the
// program's, held until every row is read.
func (a *compAssembler) addRoute(row *compRow, s string, alt *types.ReqNode) {
	if row == nil {
		a.programRoutes = append(a.programRoutes, s)
		return
	}
	if row.r.Req == nil {
		// a group: the route is a part's alternative, not the group's
		a.textRow(row, s)
		return
	}
	row.r.Req = orNode(row.r.Req, alt)
	addRaw(row, s)
}

// complShareRouteRe matches a share route: "1.0 credit may be taken from
// courses offered by other departments, pending approval".
var complShareRouteRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+may\s+be\s+taken\s+from\s+courses\s+offered\s+by\s+other\s+departments,?\s+pending\s+approval\b`)

// shareRoutes makes a plain credits total either the floor's designators plus
// the courses listed outside them, or the total with the route's approval.
func (a *compAssembler) shareRoutes() {
	if a.total == nil || a.total.r.Req == nil {
		return
	}
	t := a.total.r.Req
	if t.Type != types.NodeCredits || t.Designators != nil || t.Courses != nil || t.Lists != nil || t.Levels != nil {
		return
	}
	for i, s := range a.notes {
		m := complShareRouteRe.FindStringSubmatch(a.clean(s))
		if m == nil {
			continue
		}
		v, _ := strconv.ParseFloat(m[1], 64)
		floor := a.shareFloor(t.Amount - v)
		if floor == nil {
			continue
		}
		strict := &types.ReqNode{Type: types.NodeCredits, Amount: t.Amount, Designators: slices.Clone(floor.Designators), Courses: a.listedOutside(floor.Designators)}
		a.total.r.Req = orNode(strict, andNode(&types.ReqNode{Type: types.NodeCredits, Amount: t.Amount}, textNode(types.TextPermission, cleanNote(s))))
		addRaw(a.total, s)
		a.notes = slices.Delete(a.notes, i, i+1)
		return
	}
}

// shareFloor returns the program-level floor of designators alone at amount:
// "At least 5.0 credits must be NMC and/or NML courses".
func (a *compAssembler) shareFloor(amount float64) *types.ReqNode {
	for _, r := range slices.Concat(a.floors, a.program) {
		n := r.r.Req
		if r.r.Kind == types.RequirementConstraint && n != nil && n.Type == types.NodeCredits && n.Designators != nil &&
			n.Courses == nil && n.Lists == nil && n.Levels == nil && n.Exclude == nil && math.Abs(n.Amount-amount) < 1e-9 {
			return n
		}
	}
	return nil
}

// listedOutside returns the courses the program lists, in rows or lines of
// codes, outside desigs.
func (a *compAssembler) listedOutside(desigs []string) []string {
	var out []string
	add := func(c string) {
		if !slices.Contains(desigs, c[:3]) {
			out = append(out, c)
		}
	}
	for _, r := range a.allRows() {
		walkReq(r.r.Req, func(n *types.ReqNode) {
			if n.Type == types.NodeCourse {
				add(n.Code)
			}
			for _, c := range n.Courses {
				add(c)
			}
		})
	}
	for _, l := range a.lines {
		if mostlyCodes(l) {
			for _, c := range codeRe.FindAllString(l, -1) {
				add(strings.TrimSpace(c))
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// placeProgramRoutes adds each program route to the rows it concerns. One that
// concerns no row is a note: an advisor records the substitution, and Review
// for everyone would be wrong.
func (a *compAssembler) placeProgramRoutes() {
	for _, s := range a.programRoutes {
		if a.halfCounts(s) {
			continue
		}
		if complNotRouteRe.MatchString(a.clean(s)) {
			a.notes = append(a.notes, cleanNote(s))
			continue
		}
		text := s
		if c := a.routeContext[s]; c != "" {
			// The route names its courses in the sentence before: "MAT221H1/
			// ... is recommended as preparation for ECO375H1. Students taking
			// one ...".
			text = strings.TrimSuffix(strings.TrimSpace(c), ".") + ". " + s
		}
		if a.directCourses(s) || a.conditionCourses(s) {
			continue
		}
		targets, replaced := a.routeTargets(s)
		if targets == nil {
			a.notes = append(a.notes, cleanNote(text))
			continue
		}
		for _, t := range targets {
			kind := altKind(s)
			if a.isApproval(s) {
				kind = types.TextPermission
			}
			a.wrapRoute(t, replaced, textNode(kind, text))
			addRaw(t, s)
		}
		if m := complAsCoursesAtLevelRe.FindStringSubmatch(s); m != nil {
			// "as courses “at the 400-level”": the route also counts toward that
			// level's floor.
			d, _ := strconv.Atoi(m[1])
			lvl := d * 100
			for _, f := range a.floors {
				if n := f.r.Req; f.r.Kind == types.RequirementConstraint && n != nil && n.Type == types.NodeCredits && slices.Contains(n.Levels, lvl) {
					a.alternative(f, text)
				}
			}
		}
	}
}

// complHalfCountRe matches the section a half of a course's credit counts for:
// "half of the credit will count for Compulsory Foundation Sciences".
var complHalfCountRe = regexp.MustCompile(`(?i)\bhalf\s+(?:of\s+the\s+credit\s+)?will\s+count\s+(?:for|towards?)\s+(?:the\s+)?([^,.;]+)`)

// halfCounts places a sentence splitting a course's credit between sections as
// or(strict, text) on those sections' rows, and reports whether it did. The
// halves aren't weights: it names the courses only by example ("like").
func (a *compAssembler) halfCounts(s string) bool {
	var secs []string
	for _, m := range complHalfCountRe.FindAllStringSubmatch(a.clean(s), -1) {
		secs = append(secs, strings.ToLower(strings.TrimSpace(m[1])))
	}
	if len(secs) < 2 {
		return false
	}
	var rows []*compRow
	for _, r := range a.routeRows() {
		if r != a.total && slices.Contains(secs, strings.ToLower(r.r.Section)) {
			rows = append(rows, r)
		}
	}
	if rows == nil {
		return false
	}
	for _, r := range rows {
		if r.r.Req.Type != types.NodeText {
			r.r.Req = orNode(r.r.Req, textNode(altKind(s), s))
		}
		addRaw(r, s)
	}
	return true
}

// complNotRouteRe matches sentences that read like routes but aren't: "No
// course substitutions will be accepted for CSC110Y1".
var complNotRouteRe = regexp.MustCompile(`(?i)\bthe\s+other\s+half\b|\bwill\s+be\s+considered\s+an?\s+group\b|\bconsidered\s+an?\s+(?:group|list)\s+\w+\s+course\b|^no\s+(?:course\s+)?substitutions?\s+(?:will|can|may)\b`)

// complDirectCourseRe matches courses a named row also counts: "PSL432H1,
// HPS390H1, HPS391H1 may count toward the "Advanced Topics" requirement".
var complDirectCourseRe = regexp.MustCompile(`(?i)^((?:` + codePat + `\s*(?:,|/|or|and)?\s*)+)\s+(?:may|can|will)\s+(?:also\s+)?(?:count|be\s+used|be\s+counted)\s+(?:towards?|for)\s+(?:the\s+)?["“']?(.+?)["”']?\s+requirement\.?$`)

// directCourses adds the courses of a complDirectCourseRe sentence to the row
// it names, and reports whether it did: to its pool when they pass its
// levels, else as alternatives.
func (a *compAssembler) directCourses(s string) bool {
	m := complDirectCourseRe.FindStringSubmatch(strings.TrimSpace(a.clean(s)))
	if m == nil {
		return false
	}
	rows := a.titledRows(m[2])
	if len(rows) != 1 || rows[0].r.Req == nil {
		return false
	}
	r := rows[0]
	codes := codeRe.FindAllString(m[1], -1)
	n := r.r.Req
	if n.Type == types.NodeCredits && n.Items == nil && !slices.ContainsFunc(codes, func(c string) bool { return !levelMatches(n.Levels, c) }) {
		n.Courses = dedupe(append(slices.Clone(n.Courses), codes...))
	} else {
		alts := courseNodes(codes)
		r.r.Req = orNode(append([]*types.ReqNode{n}, alts...)...)
	}
	addRaw(r, s)
	return true
}

// complAboveConditionsRe matches "JHE353H1, ... or PHL357H1 may also be used
// in meeting the above two conditions".
var complAboveConditionsRe = regexp.MustCompile(`(?i)\bmay\s+also\s+be\s+used\s+(?:in|for|toward)\s+(?:meeting|fulfilling|satisfying)\s+the\s+above\b`)

// conditionCourses adds the courses a complAboveConditionsRe sentence names to
// the total's pool, after a lead into the pool's conditions, and reports
// whether it did.
func (a *compAssembler) conditionCourses(s string) bool {
	if !a.poolConditions || a.total == nil || !complAboveConditionsRe.MatchString(s) {
		return false
	}
	n := a.total.r.Req
	codes := codeRe.FindAllString(s, -1)
	if n.Type != types.NodeCredits || codes == nil || slices.ContainsFunc(codes, func(c string) bool { return !levelMatches(n.Levels, c) }) {
		return false
	}
	n.Courses = dedupe(append(slices.Clone(n.Courses), codes...))
	addRaw(a.total, s)
	// The conditions are the floors read after the lead.
	for _, f := range a.floors {
		fn := f.r.Req
		if f.r.Kind != types.RequirementConstraint || fn == nil || fn.Type != types.NodeCredits || !sameSet(fn.Designators, n.Designators) {
			continue
		}
		var fit []string
		for _, c := range codes {
			if levelMatches(fn.Levels, c) {
				fit = append(fit, c)
			}
		}
		if fit != nil {
			fn.Courses = dedupe(append(slices.Clone(fn.Courses), fit...))
			addRaw(f, s)
		}
	}
	return true
}

// titledRows returns the allocations titled title, case aside: by label,
// section, or the text before the colon ("4. Advanced Topics: ...").
func (a *compAssembler) titledRows(title string) []*compRow {
	t := strings.ToLower(strings.TrimSpace(title))
	if t == "" {
		return nil
	}
	var out []*compRow
	for _, r := range a.routeRows() {
		first := rawHead(r.r)
		lead := ""
		if i := strings.Index(first, ":"); i > 0 {
			lead = strings.ToLower(strings.TrimSpace(blkItemRe.ReplaceAllString(strings.Trim(first[:i], "*_ "), "")))
		}
		if strings.ToLower(r.r.Label) == t || lead == t || strings.ToLower(r.r.Section) == t {
			out = append(out, r)
		}
	}
	return out
}

// wrapRoute adds route alt to row r: on the node naming the replaced codes,
// else on the whole body.
func (a *compAssembler) wrapRoute(r *compRow, replaced []string, alt *types.ReqNode) {
	if r.r.Req == nil {
		a.textRow(r, alt.Text)
		return
	}
	if replaced != nil {
		var done bool
		r.r.Req = wrapNode(r.r.Req, replaced, alt, &done)
		if done {
			return
		}
	}
	r.r.Req = orNode(r.r.Req, alt)
}

// wrapNode replaces, in n, the smallest node naming every code of codes with
// or(node, alt).
func wrapNode(n *types.ReqNode, codes []string, alt *types.ReqNode, done *bool) *types.ReqNode {
	if n == nil || *done {
		return n
	}
	for i, it := range n.Items {
		n.Items[i] = wrapNode(it, codes, alt, done)
		if *done {
			if n.Type == types.NodeOr {
				return orNode(n.Items...) // or(or(A, alt), B) is or(A, alt, B)
			}
			return n
		}
	}
	have := map[string]bool{}
	walkReq(n, func(x *types.ReqNode) {
		if x.Type == types.NodeCourse {
			have[x.Code] = true
		}
		for _, c := range x.Courses {
			have[c] = true
		}
	})
	if slices.ContainsFunc(codes, func(c string) bool { return !have[c] }) {
		return n
	}
	*done = true
	return orNode(n, alt)
}

// rowsHolding returns the rows whose body names one of codes, as a course or
// in a pool's courses.
func rowsHolding(rows []*compRow, codes []string) []*compRow {
	var out []*compRow
	for _, r := range rows {
		has := false
		walkReq(r.r.Req, func(x *types.ReqNode) {
			has = has || x.Type == types.NodeCourse && slices.Contains(codes, x.Code) ||
				slices.ContainsFunc(x.Courses, func(c string) bool { return slices.Contains(codes, c) })
		})
		if has {
			out = append(out, r)
		}
	}
	return out
}

// complAsCoursesAtLevelRe matches a route that counts toward a level floor:
// "as courses “at the 400-level”".
var complAsCoursesAtLevelRe = regexp.MustCompile(`(?i)\bas\s+courses\s+["“'‘]?at\s+the\s+([1-4])00[-\s]?level`)

var (
	// complReplacedRe matches the courses a route stands in for: "in lieu of
	// PSY201H1 and PSY202H1".
	complReplacedRe = regexp.MustCompile(`(?i)\b(?:in\s+lieu\s+of|in\s+place\s+of|instead\s+of|substitut\w*\s+(?:\w+\s+){0,4}for|requirement\s+to\s+complete|to\s+meet\s+the|replace)\s+((?:` + codePat + `[\s,/]*(?:and|or)?\s*)+)`)
	// complRefPhraseRe matches a requirement a route names: "Group C", or a
	// quoted title.
	complRefPhraseRe = regexp.MustCompile(`(?i)\b(?:list|group|requirement|cluster|area)\s+(?:[A-Z]|\d{1,2})\b|["“]([^"”]{3,60})["”]`)
	// complRouteTopicStop holds words too common to tell rows apart.
	complRouteTopicStop = map[string]bool{"courses": true, "course": true, "credits": true, "credit": true, "students": true, "student": true,
		"program": true, "programs": true, "requirement": true, "requirements": true, "department": true, "approval": true,
		"permission": true, "consultation": true, "coordinator": true, "undergraduate": true, "director": true, "offered": true,
		"other": true, "level": true, "levels": true, "years": true, "first": true, "second": true, "third": true, "fourth": true,
		"higher": true, "minor": true, "major": true, "specialist": true, "focus": true, "certificate": true, "chair": true,
		"office": true, "please": true, "contact": true, "approved": true, "additional": true, "these": true, "those": true,
		"which": true, "their": true, "there": true, "where": true, "relevant": true, "taken": true, "students’": true,
		"faculty": true, "science": true, "sciences": true, "toward": true, "towards": true, "count": true, "counted": true,
		"substitute": true, "substitutions": true, "substitution": true, "equivalent": true, "equivalents": true, "basis": true,
		"case-by-case": true, "written": true, "prior": true, "request": true, "requests": true, "considered": true, "following": true}
)

// routeTargets returns the rows program-level route s concerns, and the codes
// it stands in for. The narrowest match wins, from the rows it names to the
// rows its words describe; nil makes it a note.
func (a *compAssembler) routeTargets(s string) ([]*compRow, []string) {
	rows := a.routeRows()
	desc := s
	if c := a.routeContext[s]; c != "" {
		desc = c + " " + s
	}
	if rs := a.referencedRows(desc); rs != nil {
		return rs, nil
	}
	if complAboveListRe.MatchString(s) {
		// "not captured in the above list": the last item above that lists
		// courses.
		at := a.lineOf(s)
		for i := len(rows) - 1; i >= 0; i-- {
			if l := a.lineOf(rawHead(rows[i].r)); l >= 0 && l < at &&
				(len(codeRe.FindAllString(rows[i].r.Raw, -1)) >= 3 || len(rows[i].r.Notes) >= 2) {
				return rows[i : i+1], nil
			}
		}
	}
	if m := complReplacedRe.FindStringSubmatch(desc); m != nil {
		replaced := codeRe.FindAllString(m[1], -1)
		if out := rowsHolding(rows, replaced); out != nil {
			// Each row wraps the replaced codes it holds.
			return out, replaced
		}
	}
	if r := a.itemNaming(s); r != nil {
		return []*compRow{r}, nil
	}
	if rs := a.footnoteItems(s); rs != nil {
		return rs, nil
	}
	if len(rows) == 1 {
		return rows, nil
	}
	if rs := sectionRows(desc, rows); rs != nil {
		// Credits replace a share of the heading's pools and text items, not its
		// required courses.
		if complRoutePoolRe.MatchString(s) {
			if ps := slices.DeleteFunc(slices.Clone(rs), func(r *compRow) bool { return !poolOrText(r.r.Req) }); ps != nil {
				return ps, nil
			}
		}
		return rs, nil
	}
	if rs := topicRows(desc, rows); rs != nil {
		return rs, nil
	}
	if rs := describedRows(desc, rows); rs != nil {
		return rs, nil
	}
	if m := complRouteTargetRe.FindStringSubmatchIndex(desc); m != nil {
		if rs := describedRows(desc[m[2]:], rows); rs != nil {
			return rs, nil // "... in lieu of a 0.5 credit 300-level ECO elective"
		}
	}
	if complWidenRe.MatchString(a.clean(s)) {
		// A widening widens the pools, not the named courses.
		var pools []*compRow
		for _, r := range rows {
			filter := false
			walkReq(r.r.Req, func(x *types.ReqNode) {
				filter = filter || x.Type == types.NodeCredits && x.Amount > 0 && (x.Designators != nil || x.Levels != nil)
			})
			if filter {
				pools = append(pools, r)
			}
		}
		if pools != nil {
			return pools, nil
		}
	}
	return nil, nil
}

// referencedRows returns the rows route s names by complRefPhraseRe, found by
// title, label or the lead of their raw. Nil when it names none, or a name no
// row has.
func (a *compAssembler) referencedRows(s string) []*compRow {
	// A proviso is a condition on the route, not what it is for: "provided no
	// other Group A or B courses are being counted".
	s = complProvisoRe.ReplaceAllString(s, "")
	var out []*compRow
	for _, loc := range complRefPhraseRe.FindAllStringSubmatchIndex(s, -1) {
		m := []string{s[loc[0]:loc[1]], ""}
		if loc[2] >= 0 {
			m[1] = s[loc[2]:loc[3]]
		}
		if complSubstituteWithRe.MatchString(s[:loc[0]]) {
			// "substitute language courses with ... credits in requirement 3":
			// the courses come from it, and the route is on what they replace.
			continue
		}
		name := m[0]
		if m[1] != "" {
			name = m[1]
		}
		if rs := a.titledRows(name); rs != nil {
			out = append(out, rs...)
			continue
		}
		lower := strings.ToLower(name)
		// "list 2", "requirement 3": the item numbered 2 or 3.
		num := ""
		if f := strings.Fields(lower); len(f) == 2 && (f[0] == "list" || f[0] == "requirement") && f[1][0] >= '0' && f[1][0] <= '9' {
			num = f[1]
		}
		nameRe := regexp.MustCompile(`\b` + regexp.QuoteMeta(lower) + `\b`)
		for _, r := range a.routeRows() {
			first := complGroupLettersRe.ReplaceAllStringFunc(strings.ToLower(rawHead(r.r)), expandGroups)
			listed := slices.ContainsFunc(r.r.Notes, func(n string) bool { return strings.HasPrefix(strings.ToLower(n), lower+":") })
			if strings.ToLower(r.r.Label) == lower || num != "" && (r.r.Label == num || strings.HasSuffix(r.r.Label, " "+num)) ||
				nameRe.MatchString(first) || listed {
				out = append(out, r)
			}
		}
	}
	return dedupeRows(out)
}

// lineOf returns the index of the first raw line holding text s, ignoring
// markers, emphasis and spacing, or -1.
func (a *compAssembler) lineOf(s string) int {
	w := rawWords(cleanNote(s))
	if len(w) > 60 {
		w = w[:60]
	}
	for i, l := range a.lines {
		if w != "" && strings.Contains(rawWords(l), w) {
			return i
		}
	}
	return -1
}

// complProvisoRe matches a condition closing a route: ", provided no other
// Group A or B courses are being counted".
var complProvisoRe = regexp.MustCompile(`(?i),?\s+provided\s+(?:that\s+)?[^:.;]*`)

// poolOrText reports whether n counts credits from a pool or has text: a share
// of it can be replaced by other courses.
func poolOrText(n *types.ReqNode) bool {
	found := false
	walkReq(n, func(x *types.ReqNode) {
		found = found || x.Type == types.NodeText || x.Type == types.NodeCredits && x.Amount > 0
	})
	return found
}

// complGroupLettersRe matches several groups named at once: "groups a, b or
// c".
var complGroupLettersRe = regexp.MustCompile(`\bgroups\s+[a-z](?:\s*(?:,|and/or|and|or)\s*[a-z]\b)+`)

// complGroupLetterRe matches one group's letter in a complGroupLettersRe
// phrase.
var complGroupLetterRe = regexp.MustCompile(`\b[a-z]\b`)

// expandGroups writes "groups a and c" as "group a group c", so that "Group
// C" finds the row that draws on it.
func expandGroups(s string) string {
	var out []string
	for _, l := range complGroupLetterRe.FindAllString(strings.TrimPrefix(s, "groups"), -1) {
		out = append(out, "group "+l)
	}
	return strings.Join(out, " ")
}

// complAboveListRe matches a route about the list just above it: "in the
// above list".
var complAboveListRe = regexp.MustCompile(`(?i)\bin\s+the\s+above\s+(?:list|clusters?|groups?)\b`)

// complSubstituteWithRe matches a substitution's source before its row:
// "substitute language courses with the equivalent number of credits in".
var complSubstituteWithRe = regexp.MustCompile(`(?i)\bsubstitut\w*\b[^.]*\bwith\b[^.]*$`)

func dedupeRows(rs []*compRow) []*compRow {
	var out []*compRow
	for _, r := range rs {
		if !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	return out
}

// topicRows returns the allocations, at most three, whose lead shares a
// telling word with route s, or nil.
func topicRows(s string, rows []*compRow) []*compRow {
	// A word and its plural are one topic: "the sequence of language courses"
	// concerns "... one of the following languages".
	singular := func(w string) string { return strings.TrimSuffix(strings.Trim(w, ".,;:()\"“”'’"), "s") }
	words := map[string]bool{}
	for _, w := range strings.Fields(strings.ToLower(codeRe.ReplaceAllString(s, " "))) {
		w = strings.Trim(w, ".,;:()\"“”'’")
		if len(w) >= 5 && !complRouteTopicStop[w] {
			words[singular(w)] = true
		}
	}
	var out []*compRow
	for _, r := range rows {
		lead := strings.ToLower(rawHead(r.r))
		for _, w := range strings.Fields(codeRe.ReplaceAllString(lead, " ")) {
			if words[singular(w)] {
				out = append(out, r)
				break
			}
		}
	}
	if len(out) == 0 || len(out) > 3 {
		return nil
	}
	return out
}

// sectionRows returns the rows whose section a route names, the longest named,
// or nil.
func sectionRows(s string, rows []*compRow) []*compRow {
	lower := strings.ToLower(strings.Join(strings.Fields(s), " "))
	var out []*compRow
	longest := ""
	for _, r := range rows {
		if sec := strings.ToLower(r.r.Section); len(strings.Fields(sec)) >= 2 && strings.Contains(lower, sec) && len(sec) > len(longest) {
			longest = sec
		}
	}
	for _, r := range rows {
		// "the Third/Fourth Year Requirement" names "Third/Fourth Year", not
		// "Fourth Year" inside it.
		if longest != "" && strings.ToLower(r.r.Section) == longest {
			out = append(out, r)
		}
	}
	return out
}

// complRouteTargetRe matches the words naming what a route replaces or counts
// toward: "in lieu of ...". Group 1 is what follows.
var complRouteTargetRe = regexp.MustCompile(`(?i)\b(?:in\s+lieu\s+of|instead\s+of|substitut\w*\s+for|counts?\s+towards?|used\s+towards?)\b(.*)$`)

// routeRows returns the allocations a route may widen: each item with a body,
// the parts of a group, and Req 1 when it has a pool of its own.
func (a *compAssembler) routeRows() []*compRow {
	var out []*compRow
	if t := a.total; t != nil && t.r.Req != nil && t.r.Req.Type == types.NodeCredits &&
		(t.r.Req.Designators != nil || t.r.Req.Levels != nil || t.r.Req.Courses != nil) {
		out = append(out, t)
	}
	var walk func(rs []*compRow)
	walk = func(rs []*compRow) {
		for _, r := range rs {
			switch {
			case r.r.Kind != types.RequirementAllocation:
			case r.r.Req != nil:
				out = append(out, r)
			default:
				walk(r.children)
			}
		}
	}
	walk(a.items)
	return out
}

// complFootnoteMarkRe matches a footnote's marker: "*A research project
// ...".
var complFootnoteMarkRe = regexp.MustCompile(`^\s*(\*+)`)

// footnoteItems returns the items whose raw puts the footnote's marker after a
// code ("HMB496Y1*"), or nil.
func (a *compAssembler) footnoteItems(s string) []*compRow {
	m := complFootnoteMarkRe.FindStringSubmatch(s)
	if m == nil {
		return nil
	}
	starred := regexp.MustCompile(codePat + `\*{` + strconv.Itoa(len(m[1])) + `}(?:[^*]|$)`)
	var out []*compRow
	for _, r := range a.routeRows() {
		if starred.MatchString(r.r.Raw) {
			out = append(out, r)
		}
	}
	return out
}

var (
	// complRouteLevelRe matches levels a route names: "300/400 level".
	complRouteLevelRe = regexp.MustCompile(`(?i)\b` + levelPat + `(?:\s*(?:-|/|or|and|,)\s*` + levelPat + `)*\s*(?:\+|-?\s*levels?\b|-series\b)(?:\s+or\s+(?:higher|above))?`)
	// complRouteYearLevelRe matches "third or fourth year levels".
	complRouteYearLevelRe = regexp.MustCompile(`(?i)\b((?:first|second|third|fourth)(?:\s*(?:,|or|and|/)\s*(?:first|second|third|fourth))*)[\s-]+years?[\s-]+levels?\b`)
	complRouteOrdinalRe   = regexp.MustCompile(`(?i)first|second|third|fourth`)
	// complRoutePoolRe matches a route speaking of credits or electives, which
	// replaces a share of a pool, not a named course: "0.5 credit".
	complRoutePoolRe = regexp.MustCompile(`(?i)\bcredits?\b|\belectives?\b|\b\d\.\d\b`)
)

// complSubjectWordRe matches the subject names of subjectDesignators.
var complSubjectWordRe = func() *regexp.Regexp {
	var names []string
	for name := range subjectDesignators {
		names = append(names, regexp.QuoteMeta(name))
	}
	slices.SortFunc(names, func(x, y string) int { return len(y) - len(x) }) // "study of religion" before "religion"
	return regexp.MustCompile(`\b(?:` + strings.Join(names, "|") + `)\b`)
}()

// complApproverRe matches the person or office a route names: "approval of
// the Program Director".
var complApproverRe = regexp.MustCompile(`(?i)\b(?:approval|permission|consent)\s+(?:of|from)\s+the\s+[^.;,]*?\b(?:director|coordinator|chair|office|advisor|department)\b`)

// complAddressRe matches an e-mail address or a web address.
var complAddressRe = regexp.MustCompile(`\S+@\S+|https?://\S+|www\.\S+`)

// complProgramDesigRe matches a designator naming a program: "RLG program".
var complProgramDesigRe = regexp.MustCompile(`\b[A-Z]{3}\s+programs?\b`)

// routeFilters reads the designators, named or by subject ("physics" is PHY),
// and levels a route describes.
func routeFilters(s string) (desigs []string, levels []int) {
	t := codeRe.ReplaceAllString(s, " ")
	// "a RLG program" names a program, "UTM" is a campus, and an address such
	// as "religion.undergrad@utoronto.ca" names no courses.
	t = complProgramDesigRe.ReplaceAllString(complAddressRe.ReplaceAllString(t, " "), " ")
	// The approver says who decides, not which courses.
	t = complApproverRe.ReplaceAllString(t, " ")
	desigs = slices.DeleteFunc(desigRe.FindAllString(t, -1), func(d string) bool { return d == "UTM" })
	for _, m := range complSubjectWordRe.FindAllString(strings.ToLower(t), -1) {
		desigs = append(desigs, subjectDesignators[m])
	}
	for _, m := range complRouteLevelRe.FindAllString(t, -1) {
		levels = append(levels, parseLevels(m)...)
	}
	for _, m := range complRouteYearLevelRe.FindAllStringSubmatch(t, -1) {
		for _, w := range complRouteOrdinalRe.FindAllString(m[1], -1) {
			levels = append(levels, ordinalYear(w)*100)
		}
	}
	return dedupe(desigs), dedupe(levels)
}

// describedRows returns the rows whose courses match what route s describes,
// or nil. It prefers pools when s speaks of credits or electives, then rows
// with a filter on the designators, then rows whose courses all fit.
func describedRows(s string, rows []*compRow) []*compRow {
	desigs, levels := routeFilters(s)
	if desigs == nil && levels == nil {
		return nil
	}
	type fit struct {
		r                  *compRow
		pool, pure, filter bool
	}
	var fits []fit
	for _, r := range rows {
		codes, filters := poolParts(r.r.Req)
		var ds []string
		var ls []int
		anyD, anyL := false, false
		for _, c := range codes {
			ds = append(ds, c[:3])
			if sgCodeRe.MatchString(c) {
				ls = append(ls, int(c[3]-'0')*100)
			}
		}
		for _, f := range filters {
			if f.Lists != nil && f.Designators == nil && f.Levels == nil && f.Breadth == nil {
				// A pool from named lists describes their courses, not any
				// course.
				continue
			}
			ds = append(ds, f.Designators...)
			ls = append(ls, f.Levels...)
			anyD = anyD || f.Designators == nil
			anyL = anyL || f.Levels == nil
		}
		dOK := desigs == nil || anyD || slices.ContainsFunc(ds, func(d string) bool { return slices.Contains(desigs, d) })
		lOK := levels == nil || anyL || slices.ContainsFunc(ls, func(l int) bool { return slices.Contains(levels, l) })
		if !dOK || !lOK || ds == nil && !anyD {
			continue
		}
		pure := (desigs == nil || !anyD && !slices.ContainsFunc(ds, func(d string) bool { return !slices.Contains(desigs, d) })) &&
			(levels == nil || !anyL && !slices.ContainsFunc(ls, func(l int) bool { return !slices.Contains(levels, l) }))
		pool, filter := false, false
		walkReq(r.r.Req, func(x *types.ReqNode) {
			pool = pool || x.Type == types.NodeCredits && x.Amount > 0
			filter = filter || desigs != nil && x.Type == types.NodeCredits && x.Amount > 0 && isSubset(desigs, x.Designators)
		})
		fits = append(fits, fit{r, pool, pure, filter})
	}
	keep := func(ok func(fit) bool) {
		var k []fit
		for _, f := range fits {
			if ok(f) {
				k = append(k, f)
			}
		}
		if k != nil {
			fits = k
		}
	}
	if complRoutePoolRe.MatchString(s) {
		// With no pool among the rows it fits, a pool route names none.
		if !slices.ContainsFunc(fits, func(f fit) bool { return f.pool }) {
			return nil
		}
		keep(func(f fit) bool { return f.pool })
	}
	// "count toward the Department for the Study of Religion courses": the
	// pools of those designators, before rows of listed courses all RLG.
	keep(func(f fit) bool { return f.filter })
	keep(func(f fit) bool { return f.pure })
	var out []*compRow
	for _, f := range fits {
		out = append(out, f.r)
	}
	return out
}

// itemNaming returns the only item whose body names a course of s, or nil.
func (a *compAssembler) itemNaming(s string) *compRow {
	var found *compRow
	for _, code := range codeRe.FindAllString(s, -1) {
		for _, r := range a.items {
			have := map[string]bool{}
			walkReq(r.r.Req, func(n *types.ReqNode) {
				have[n.Code] = true
				for _, c := range n.Courses {
					have[c] = true
				}
			})
			if !have[code] {
				continue
			}
			if found != nil && found != r {
				return nil
			}
			found = r
		}
	}
	return found
}

// complTopicContentRe matches special-topics courses on a topic: "Special
// Topics courses with Islam content ... will also count towards the program".
var complTopicContentRe = regexp.MustCompile(`(?i)^(?:[\w-]+\s+)*?special\s+topics\s+courses\b.*\bwill\s+(?:also\s+)?count\s+towards?\s+(?:the\s+)?program(?:\s+requirements)?$`)

// topicContentRoutes places each complTopicContentRe note on the one row whose
// pools hold courses of every designator it names, at its levels: or(pool,
// and(pool widened to them, text{content})). Otherwise it stays a note.
func (a *compAssembler) topicContentRoutes() {
	var keep []string
	for _, s := range a.notes {
		if !complTopicContentRe.MatchString(s) || !a.topicContentRoute(s) {
			keep = append(keep, s)
		}
	}
	a.notes = keep
}

// topicContentRoute places complTopicContentRe note s and reports whether it
// did.
func (a *compAssembler) topicContentRoute(s string) bool {
	desigs, levels := routeFilters(s)
	for _, c := range codeRe.FindAllString(s, -1) {
		desigs = dedupe(append(desigs, c[:3]))
	}
	if desigs == nil {
		return false
	}
	// pool reports whether n is a pool of listed courses the route widens.
	pool := func(n *types.ReqNode) bool {
		return n.Type == types.NodeCredits && n.Amount > 0 && n.Courses != nil && n.Designators == nil && n.Levels == nil &&
			n.Lists == nil && n.MaxAmount == nil && n.Breadth == nil && n.Exclude == nil && n.Items == nil
	}
	// fits reports whether pool n holds a course of each designator at the
	// levels.
	fits := func(n *types.ReqNode) bool {
		for _, d := range desigs {
			if !slices.ContainsFunc(n.Courses, func(c string) bool { return c[:3] == d && levelMatches(levels, c) }) {
				return false
			}
		}
		return true
	}
	var target *compRow
	for _, r := range a.routeRows() {
		hit := false
		walkReq(r.r.Req, func(n *types.ReqNode) { hit = hit || pool(n) && fits(n) })
		if hit {
			if target != nil {
				return false
			}
			target = r
		}
	}
	if target == nil {
		return false
	}
	var wrap func(n *types.ReqNode) *types.ReqNode
	wrap = func(n *types.ReqNode) *types.ReqNode {
		if pool(n) && fits(n) {
			w := cloneNode(n)
			w.Designators, w.Levels = slices.Clone(desigs), slices.Clone(levels)
			w.Courses = dedupe(append(w.Courses, codeRe.FindAllString(s, -1)...)) // its examples
			return orNode(n, andNode(w, textNode(types.TextContent, s)))
		}
		for i, it := range n.Items {
			n.Items[i] = wrap(it)
		}
		return n
	}
	target.r.Req = wrap(target.r.Req)
	// An earlier rule may have left the sentence a bare alternative of the row:
	// bounded now, it goes.
	walkReq(target.r.Req, func(n *types.ReqNode) {
		if n.Type == types.NodeOr && len(n.Items) > 2 {
			n.Items = slices.DeleteFunc(n.Items, func(x *types.ReqNode) bool { return x.Type == types.NodeText && x.Text == s })
		}
	})
	addRaw(target, s)
	return true
}

// dsrLanguagePools reads the Study of Religion language paragraph, a limit
// already: its courses count as the department's, so each RLG pool takes them
// at its levels. Its other sentences are notes.
func (a *compAssembler) dsrLanguagePools() {
	all := append(append(append([]*compRow{}, a.floors...), a.items...), a.program...)
	if a.total != nil {
		all = append([]*compRow{a.total}, all...)
	}
	var codes []string
	var para string
	walkRows(all, func(r *compRow) {
		if t := a.clean(r.r.Raw); codes == nil && r.r.Kind == types.RequirementLimit && complDSRCountRe.MatchString(t) && courseCountCap(t) != nil {
			codes, para = r.r.Req.Courses, r.r.Raw
		}
	})
	if codes == nil {
		return
	}
	first := strings.ToLower(rawWords(splitSentences(a.clean(para))[0]))
	// strip drops the paragraph's text alternatives from or nodes.
	var strip func(n *types.ReqNode) *types.ReqNode
	strip = func(n *types.ReqNode) *types.ReqNode {
		if n == nil {
			return nil
		}
		for i, it := range n.Items {
			n.Items[i] = strip(it)
		}
		if n.Type == types.NodeOr {
			n.Items = slices.DeleteFunc(n.Items, func(it *types.ReqNode) bool {
				return it.Type == types.NodeText && strings.Contains(strings.ToLower(rawWords(a.clean(it.Text))), first)
			})
			if len(n.Items) == 1 {
				return n.Items[0]
			}
		}
		return n
	}
	// widen adds the codes n's levels allow that n doesn't hold already, and
	// reports whether n is such a pool.
	widen := func(n *types.ReqNode, all bool) bool {
		if n == nil || n.Type != types.NodeCredits || n.Amount == 0 || n.Lists != nil || n.Breadth != nil {
			return false
		}
		if !all && !slices.Contains(n.Designators, "RLG") {
			return false
		}
		for _, c := range codes {
			if levelMatches(n.Levels, c) && !slices.Contains(n.Exclude, c) && !slices.Contains(n.Courses, c) && (all || !slices.Contains(n.Designators, c[:3])) {
				n.Courses = append(n.Courses, c)
			}
		}
		return true
	}
	walkRows(all, func(row *compRow) {
		row.r.Req = strip(row.r.Req)
		if row.r.Kind != types.RequirementAllocation {
			return
		}
		floor := false
		for _, k := range row.children {
			if k.r.Kind == types.RequirementConstraint {
				k.r.Req = strip(k.r.Req)
				walkReq(k.r.Req, func(n *types.ReqNode) { floor = widen(n, false) || floor })
			}
		}
		walkReq(row.r.Req, func(n *types.ReqNode) {
			if !widen(n, false) && floor && n.Type == types.NodeCredits && n.Designators == nil && n.Levels == nil && n.Courses != nil {
				widen(n, true)
			}
		})
	})
	for _, p := range splitSentences(a.clean(para))[1:] {
		a.notes = append(a.notes, cleanNote(p))
	}
}

// complUntargetedRouteRe matches a route naming no course, pool or row:
// "Students can request substitutions from the IRP Director".
var complUntargetedRouteRe = regexp.MustCompile(`(?i)^alternative\s+courses\s+may\s+be\s+substituted\s+with\s+the\s+(?:prior\s+)?approval\s+of\b` +
	`|^students\s+can\s+request\s+substitutions\s+from\s+the\b` +
	`|^other\s+courses\s+from\s+[\w/\s]+?\s+may\s+be\s+considered\s+with\s+permission\.?$` +
	`|\bmay\s+count\s+towards?\s+(?:\w+\s+)?program\s+requirements,\s+where\s+relevant\b`)

// untargetedRoutes moves the allocations' complUntargetedRouteRe alternatives
// to the notes: on a row they made it Review for a student short of it. A
// bounded and(pool, text), and a route on a constraint, stay.
func (a *compAssembler) untargetedRoutes() {
	all := append(append(append([]*compRow{}, a.floors...), a.items...), a.program...)
	if a.total != nil {
		all = append([]*compRow{a.total}, all...)
	}
	walkRows(all, func(r *compRow) {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr {
			return
		}
		n.Items = slices.DeleteFunc(n.Items, func(it *types.ReqNode) bool {
			if it.Type == types.NodeText && it.Kind == types.TextPermission && complUntargetedRouteRe.MatchString(strings.TrimSpace(a.clean(it.Text))) {
				a.notes = append(a.notes, cleanNote(it.Text))
				return true
			}
			return false
		})
		if len(n.Items) == 1 {
			r.r.Req = n.Items[0]
		}
	})
}

// placedElsewhere drops a bare top-level text alternative from an allocation
// when another allocation holds the same sentence better: nested on the node
// it replaces, or read into structure.
func (a *compAssembler) placedElsewhere() {
	var rows []*compRow
	walkRows(a.items, func(r *compRow) {
		if r.r.Kind == types.RequirementAllocation && r.r.Req != nil {
			rows = append(rows, r)
		}
	})
	// fits reports whether row r2 holds text t better than a top-level text:
	// nested on a node of r2, or read into r2's alternatives (an or with no text
	// left, whose raw holds t).
	fits := func(r2 *compRow, t string) bool {
		w := rawWords(a.clean(t))
		text, nested := false, false
		var walk func(n *types.ReqNode, depth int)
		walk = func(n *types.ReqNode, depth int) {
			if n.Type == types.NodeText {
				text = true
				nested = nested || depth > 1 && rawWords(a.clean(n.Text)) == w
			}
			for _, it := range n.Items {
				walk(it, depth+1)
			}
		}
		walk(r2.r.Req, 0)
		return nested || !text && r2.r.Req.Type == types.NodeOr && len(r2.r.Req.Items) >= 2 && strings.Contains(rawWords(a.clean(r2.r.Raw)), w)
	}
	for _, r := range rows {
		n := r.r.Req
		if n.Type != types.NodeOr {
			continue
		}
		n.Items = slices.DeleteFunc(n.Items, func(it *types.ReqNode) bool {
			if it.Type != types.NodeText || it.Kind != types.TextPermission && it.Kind != types.TextOther {
				return false
			}
			for _, r2 := range rows {
				if r2 != r && fits(r2, it.Text) {
					return true
				}
			}
			return false
		})
		if len(n.Items) == 1 {
			r.r.Req = n.Items[0]
		}
	}
}

// complDSRCountRe matches what a course-count cap says its courses count
// toward: "will count toward the Department for the Study of Religion courses
// required".
var complDSRCountRe = regexp.MustCompile(`(?i)\bwill\s+count\s+towards?\s+the\s+department\s+for\s+the\s+study\s+of\s+religion\s+courses\s+required\b`)
