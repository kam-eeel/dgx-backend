package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Program routes: the approval routes and widened pools stated for the
// program rather than an item ("Students can request substitutions from the
// IRP Director", "Up to 1.0 credit may be from other departments"). Once
// every row is read, finish calls placeProgramRoutes, which puts each route
// on the rows it names, replaces or describes (routeTargets); a route that
// concerns no row is a note.

// placeProgramRoutes adds each approval route and widened pool stated for the
// program to the requirements it concerns (routeTargets). A route that names
// or describes no requirement is a note: an advisor records the substitution
// it allows, which the checker can't anticipate (Review for everyone would be
// wrong). Sentences that only look like routes (complNotRouteRe) are notes
// too.
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
			// "MAT221H1/ MAT223H1/ MAT240H1 is recommended as preparation for
			// ECO375H1. Students taking one of these MAT courses can have it
			// count in lieu of ...": the route names its courses before.
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
			// "(e.g., as courses “at the 400-level”)": the route also
			// counts toward the floor on that level.
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

// complHalfCountRe: the section a share of a course's credit counts for: "half
// of the credit will count for Compulsory Foundation Sciences", "the other
// half will count for Additional Foundation Sciences".
var complHalfCountRe = regexp.MustCompile(`(?i)\bhalf\s+(?:of\s+the\s+credit\s+)?will\s+count\s+(?:for|towards?)\s+(?:the\s+)?([^,.;]+)`)

// halfCounts places a sentence that splits a course's credit between
// sections ("Advanced courses, like CHM151Y1, PHY151H1, ..., can be used as
// credits for both foundation sciences sections, meaning, half of the
// credit will count for Compulsory Foundation Sciences, and the other half
// will count for Additional Foundation Sciences") and reports whether it
// did. The rows of the sections it names take it as an alternative,
// or(strict, text): Review, not NotMet, for a student who took an advanced
// course instead of a required one. The halves aren't encoded as weights:
// each needs a structured row to count in, and the sentence names the
// courses only by example ("like").
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

// complNotRouteRe: sentences that read like routes but aren't: weight
// splitting ("In the instance that a Y course is used ..., the other half
// may be used toward another"), list membership ("If ARH312Y1 is completed,
// it will be considered a Group A course"), refusals ("No course
// substitutions will be accepted for CSC110Y1").
var complNotRouteRe = regexp.MustCompile(`(?i)\bthe\s+other\s+half\b|\bwill\s+be\s+considered\s+an?\s+group\b|\bconsidered\s+an?\s+(?:group|list)\s+\w+\s+course\b|^no\s+(?:course\s+)?substitutions?\s+(?:will|can|may)\b`)

// complDirectCourseRe: "PSL432H1, HPS390H1, HPS391H1 may count toward the
// "Advanced Topics" requirement": courses the named row also counts.
var complDirectCourseRe = regexp.MustCompile(`(?i)^((?:` + codePat + `\s*(?:,|/|or|and)?\s*)+)\s+(?:may|can|will)\s+(?:also\s+)?(?:count|be\s+used|be\s+counted)\s+(?:towards?|for)\s+(?:the\s+)?["“']?(.+?)["”']?\s+requirement\.?$`)

// directCourses adds the courses of a complDirectCourseRe sentence to the row
// whose title it names, and reports whether it did: to its pool when they
// pass the pool's levels, else as alternatives.
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
		var alts []*types.ReqNode
		for _, c := range codes {
			alts = append(alts, &types.ReqNode{Type: types.NodeCourse, Code: c})
		}
		r.r.Req = orNode(append([]*types.ReqNode{n}, alts...)...)
	}
	addRaw(r, s)
	return true
}

// complAboveConditionsRe: "JHE353H1, ... or PHL357H1 may also be used in
// meeting the above two conditions".
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
	// The conditions are the floors read after the lead: the courses count
	// toward each floor whose levels they pass.
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

// titledRows returns the allocations whose title is title: the label, the
// text before the colon in their raw ("4. Advanced Topics: 1.0 credit ..."),
// or their section, case aside.
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

// wrapRoute adds route text alt to row r: on the node of r's body the route
// replaces (the course or alternatives naming the replaced codes, "in lieu
// of PSY201H1"), else on the whole body.
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

// wrapNode replaces, in n, the smallest node naming every code of codes
// with or(node, alt).
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

// rowsHolding returns the rows among rows whose body names one of codes: as
// a course, or among a pool's listed courses.
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

// complAsCoursesAtLevelRe: a route that counts toward a level floor: "as
// courses “at the 400-level”".
var complAsCoursesAtLevelRe = regexp.MustCompile(`(?i)\bas\s+courses\s+["“'‘]?at\s+the\s+([1-4])00[-\s]?level`)

var (
	// complReplacedRe: the courses a route stands in for: "in lieu of PSY201H1
	// and PSY202H1", "in place of PSY100H1", "towards the requirement to
	// complete PCL472Y1/ PCL474Y1/ JPM400Y1", "to meet the MAT246H1
	// requirement".
	complReplacedRe = regexp.MustCompile(`(?i)\b(?:in\s+lieu\s+of|in\s+place\s+of|instead\s+of|substitut\w*\s+(?:\w+\s+){0,4}for|requirement\s+to\s+complete|to\s+meet\s+the|replace)\s+((?:` + codePat + `[\s,/]*(?:and|or)?\s*)+)`)
	// complRefPhraseRe: a requirement named in a route: "list 2", "Group C",
	// "requirement 3", a quoted title.
	complRefPhraseRe = regexp.MustCompile(`(?i)\b(?:list|group|requirement|cluster|area)\s+(?:[A-Z]|\d{1,2})\b|["“]([^"”]{3,60})["”]`)
	// complRouteTopicStop: words too common to tell rows apart.
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

// routeTargets returns the rows a program-level route concerns, and the
// codes it stands in for, if it names them:
//   - the requirement it names: by a quoted title or "list 2", "Group C",
//     "requirement 3" (titled or labelled, or whose lead says so);
//   - the rows holding the courses it stands in for ("in lieu of PSY201H1
//     and PSY202H1": each row with one of them; its node is the one
//     wrapped);
//   - the item naming one of its courses ("If the FOR301H1 is not on offer,
//     ...");
//   - the item its footnote marker points at ("*A research project ..." for
//     "HMB496Y1*");
//   - the program's only allocation;
//   - the allocations under a heading it names ("... for both foundation
//     sciences sections");
//   - the allocations whose lead shares a telling word with it ("the
//     sequence of language courses" and the language items);
//   - the allocations whose courses match what it describes: designators or
//     subjects and levels ("physics courses", "a 0.5 credit 300-level ECO
//     elective"), preferring pools when it speaks of credits or electives
//     (describedRows);
//   - for a widened pool ("Up to 1.0 credit may be from other
//     departments"), the items that are pools of designators or levels.
//
// Otherwise nil: the route is a note (placeProgramRoutes).
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
		// "An elective course ... not captured in the above list", "not
		// listed in the above clusters": the last item above the sentence
		// that lists courses.
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
		// "... can be used as credits for both foundation sciences sections";
		// "up to 1.0 credit in the Higher Years Requirement with two of the
		// following ARC courses": credits replace a share of the heading's
		// pools (and items of text), not its required courses.
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
		// "Up to 1.0 credit may be from other departments": it widens the
		// pools (electives), not the named courses.
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

// referencedRows returns the rows a route names (complRefPhraseRe): a quoted
// title ("the “Advanced Topics” requirement"), or "list 2", "Group C",
// "requirement 3", found in a row's label or the lead of its raw. Nil when
// it names none, or a name no row has.
func (a *compAssembler) referencedRows(s string) []*compRow {
	// "..., provided no other Group A or B courses are being counted": a
	// condition on the route, not what it is for.
	s = complProvisoRe.ReplaceAllString(s, "")
	var out []*compRow
	for _, loc := range complRefPhraseRe.FindAllStringSubmatchIndex(s, -1) {
		m := []string{s[loc[0]:loc[1]], ""}
		if loc[2] >= 0 {
			m[1] = s[loc[2]:loc[3]]
		}
		if complSubstituteWithRe.MatchString(s[:loc[0]]) {
			// "may substitute language courses with the equivalent number of
			// credits in requirement 3": the courses come from it; the route
			// is on what they replace.
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
		// "list 2", "requirement 3": the item numbered 2 or 3 ("2. 3.0
		// credits from the following:").
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

// complProvisoRe: a condition closing a route: ", provided no other Group A or
// B courses are being counted".
var complProvisoRe = regexp.MustCompile(`(?i),?\s+provided\s+(?:that\s+)?[^:.;]*`)

// poolOrText reports whether n counts credits from a pool or has text: a
// share of it can be replaced by other courses.
func poolOrText(n *types.ReqNode) bool {
	found := false
	walkReq(n, func(x *types.ReqNode) {
		found = found || x.Type == types.NodeText || x.Type == types.NodeCredits && x.Amount > 0
	})
	return found
}

// complGroupLettersRe: several groups named at once: "groups a and c", "groups
// a, b or c".
var complGroupLettersRe = regexp.MustCompile(`\bgroups\s+[a-z](?:\s*(?:,|and/or|and|or)\s*[a-z]\b)+`)

// complGroupLetterRe: one group's letter in a complGroupLettersRe phrase.
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

// complAboveListRe: a route about the list just above it.
var complAboveListRe = regexp.MustCompile(`(?i)\bin\s+the\s+above\s+(?:list|clusters?|groups?)\b`)

// complSubstituteWithRe: the source of a substitution, before the requirement
// it is drawn from: "substitute language courses with the equivalent number
// of credits in".
var complSubstituteWithRe = regexp.MustCompile(`(?i)\bsubstitut\w*\b[^.]*\bwith\b[^.]*$`)

// dedupeRows removes repeated rows, keeping order.
func dedupeRows(rs []*compRow) []*compRow {
	var out []*compRow
	for _, r := range rs {
		if !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	return out
}

// topicRows returns the allocations (at most three) whose lead shares a
// telling word with route s ("exceptions may be made ... regarding the
// sequence of language courses" and "A two-year progressive sequence of
// courses in a single language"), or nil.
func topicRows(s string, rows []*compRow) []*compRow {
	// A word and its plural are one topic: "the sequence of language
	// courses" concerns "1.0 credit from one of the following languages".
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

// sectionRows returns the rows whose section (the heading they are under,
// "Compulsory Foundation Sciences") a route names, or nil.
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
		// "the Third/Fourth Year Requirement" names "Third/Fourth Year",
		// not "Fourth Year" inside it.
		if longest != "" && strings.ToLower(r.r.Section) == longest {
			out = append(out, r)
		}
	}
	return out
}

// complRouteTargetRe: the words that name what a route replaces or counts
// toward: "in lieu of ...", "count towards ...".
var complRouteTargetRe = regexp.MustCompile(`(?i)\b(?:in\s+lieu\s+of|instead\s+of|substitut\w*\s+for|counts?\s+towards?|used\s+towards?)\b(.*)$`)

// routeRows returns the allocations a route may widen: each item with a
// body, the parts of a group, and Req 1 when it has a pool of its own.
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

// complFootnoteMarkRe: a footnote's marker: "*A research project ...".
var complFootnoteMarkRe = regexp.MustCompile(`^\s*(\*+)`)

// footnoteItems returns the items whose raw puts the footnote's marker after
// a code ("HMB496Y1*" for "*A research project ..."), or nil.
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
	// complRouteLevelRe: levels a route names: "300-level", "300+", "300/400
	// level", "400-series".
	complRouteLevelRe = regexp.MustCompile(`(?i)\b[1-4]00(?:\s*(?:-|/|or|and|,)\s*[1-4]00)*\s*(?:\+|-?\s*levels?\b|-series\b)(?:\s+or\s+(?:higher|above))?`)
	// complRouteYearLevelRe: "at the third or fourth year levels".
	complRouteYearLevelRe = regexp.MustCompile(`(?i)\b((?:first|second|third|fourth)(?:\s*(?:,|or|and|/)\s*(?:first|second|third|fourth))*)[\s-]+years?[\s-]+levels?\b`)
	complRouteOrdinalRe   = regexp.MustCompile(`(?i)first|second|third|fourth`)
	// complRoutePoolRe: a route that speaks of credits or electives replaces a
	// share of a pool, not a named course.
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

// complApproverRe: the person or office a route names.
var complApproverRe = regexp.MustCompile(`(?i)\b(?:approval|permission|consent)\s+(?:of|from)\s+the\s+[^.;,]*?\b(?:director|coordinator|chair|office|advisor|department)\b`)

// complAddressRe: an e-mail address or a web address.
var complAddressRe = regexp.MustCompile(`\S+@\S+|https?://\S+|www\.\S+`)

// complProgramDesigRe: a designator naming a program: "a RLG program".
var complProgramDesigRe = regexp.MustCompile(`\b[A-Z]{3}\s+programs?\b`)

// complSubjectWordCIRe matches the subject names of subjectDesignators in any
// case: "Additional Philosophy courses".
var complSubjectWordCIRe = regexp.MustCompile(`(?i)` + complSubjectWordRe.String())

// routeFilters reads the designators (named, or a subject: "physics" is
// PHY) and levels a route describes.
func routeFilters(s string) (desigs []string, levels []int) {
	t := codeRe.ReplaceAllString(s, " ")
	// "a RLG program" names a program, not RLG courses; "UTM" is a campus;
	// "religion.undergrad@utoronto.ca" is an address, not religion courses.
	t = complProgramDesigRe.ReplaceAllString(complAddressRe.ReplaceAllString(t, " "), " ")
	// "with approval of the Canadian Studies Program Director": who decides,
	// not which courses.
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

// describedRows returns the rows whose courses match what route s
// describes (routeFilters), or nil when it describes nothing or no row
// matches. Among the matches it prefers pools when s speaks of credits or
// electives, then rows all of whose courses fit the description.
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
				// A pool from named lists: their courses, not any course. A
				// route describes them no better than before the lists were
				// read, when the row was text.
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
		// A route that speaks of credits or electives replaces a share of a
		// pool: with no pool among the rows it fits, it names none.
		if !slices.ContainsFunc(fits, func(f fit) bool { return f.pool }) {
			return nil
		}
		keep(func(f fit) bool { return f.pool })
	}
	// "will count toward the Department for the Study of Religion courses
	// required": the pools of those designators, before rows of listed
	// courses that happen to be all RLG.
	keep(func(f fit) bool { return f.filter })
	keep(func(f fit) bool { return f.pure })
	var out []*compRow
	for _, f := range fits {
		out = append(out, f.r)
	}
	return out
}

// itemNaming returns the only item whose body names a course of s ("If the
// FOR301H1 is not on offer, a student may use a substitute ..."), or nil.
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

// complBoundPoolRe: an approval route that names the wider pool it may
// let count (design 3.3): "Some offerings of other POL Special Topics
// courses ...", "any 400-level finance-themed RSM special topics course as
// determined by the Director", "any POL course with European content
// approved by the Program Coordinator", "any marketing-themed RSM special
// topics and RSM Summer Abroad courses", "any strategy, innovation and/or
// entrepreneurship-themed RSM special topics ...". The theme is tried last,
// so it never spans a designator's own pool ("any other POL courses and any
// finance-themed RSM ...", constructed, reads POL; routePool then sees the
// second pool and doesn't bound).
var complBoundPoolRe = regexp.MustCompile(`(?i)^(?:some\s+offerings\s+of\s+)?(?:other\s+|any\s+(?:other\s+)?)(?:([1-4]00)-level\s+)?(?:[\w,/\s-]+?[-\s]themed\s+)??((?-i:[A-Z]{3}))\s+(?:special\s+topics\s+(?:and\s+(?-i:[A-Z]{3})\s+summer\s+abroad\s+)?)?courses?\b`)

// boundRoutes bounds the approval routes that name their wider pool
// (design 3.3): or(strict, text permission) on a pool becomes or(strict,
// and(the same amount from the wider pool, text permission)), so a course
// outside both pools is NotMet rather than Review. The wider pool keeps the
// strict pool's courses: the student may count both. A route that names no
// designator stays bare. A route over the designator's special-topics
// courses is narrowed at Link to those the catalogue titles "Topics in …"
// (linkOtherTopics): the parser can't see titles.
func (a *compAssembler) boundRoutes() {
	var walk func(rs []*compRow)
	walk = func(rs []*compRow) {
		for _, r := range rs {
			if n := r.r.Req; r.r.Kind == types.RequirementAllocation && n != nil && n.Type == types.NodeOr && len(n.Items) >= 2 {
				strict := n.Items[0]
				if strict.Type == types.NodeCredits && strict.Amount > 0 && strict.MaxAmount == nil && strict.Items == nil && strict.Lists == nil {
					for i, it := range n.Items[1:] {
						if wider := routePool(strict, it); wider != nil {
							n.Items[i+1] = andNode(wider, it)
						}
					}
				}
			}
			walk(r.children)
		}
	}
	walk(a.items)
}

// complAnotherPoolRe: a second pool after the one complBoundPoolRe read: "...
// courses and any finance-themed RSM special topics courses" (constructed).
var complAnotherPoolRe = regexp.MustCompile(`(?i)\b(?:and|or)\s+(?:any|other)\b`)

// routePool returns the wider pool a permission route names (complBoundPoolRe)
// for strict, or nil.
func routePool(strict, route *types.ReqNode) *types.ReqNode {
	if route.Type != types.NodeText || route.Kind != types.TextPermission {
		return nil
	}
	t := strings.TrimSpace(route.Text)
	m := complBoundPoolRe.FindStringSubmatch(t)
	if m == nil || complAnotherPoolRe.MatchString(t[len(m[0]):]) {
		return nil // a second pool after the first: bounded by one, the other would be NotMet
	}
	w := &types.ReqNode{Type: types.NodeCredits, Amount: strict.Amount, Designators: []string{m[2]}, Courses: slices.Clone(strict.Courses)}
	if m[1] != "" {
		w.Levels = parseLevels(m[1] + "-level")
	}
	if slices.ContainsFunc(w.Courses, func(c string) bool { return !levelMatches(w.Levels, c) || slices.Contains(strict.Exclude, c) }) ||
		strict.Levels != nil && w.Levels == nil || strict.Breadth != nil || strict.Exclude != nil {
		return nil
	}
	if strict.Designators != nil {
		if !slices.Contains(strict.Designators, m[2]) {
			return nil
		}
		w.Designators = slices.Clone(strict.Designators)
	}
	return w
}

// complTopicContentRe: a unit's special-topics courses on a topic that count
// toward the program: "Special Topics courses with Islam content (like
// NMC277H1 or RLG412H1) will also count towards the program",
// "Health-related Anthropology Special Topics courses at the 300 or 400
// level, and health-related Independent Research courses at the 400-level,
// will count towards program requirements".
var complTopicContentRe = regexp.MustCompile(`(?i)^(?:[\w-]+\s+)*?special\s+topics\s+courses\b.*\bwill\s+(?:also\s+)?count\s+towards?\s+(?:the\s+)?program(?:\s+requirements)?$`)

// topicContentRoutes places each complTopicContentRe note on the one row
// whose course pools hold courses of every designator it names (a subject,
// or its examples' designators) at the levels it names: each such pool,
// credits{N, courses}, becomes or(pool, and(credits{N, courses, designators,
// levels}, text{content})). The course's topic decides (D1), so a course of
// the designators is Review and any other is NotMet. A note that matches no
// row, or several, stays a note (S2-6, ASMAJ1359; ASMIN1778).
func (a *compAssembler) topicContentRoutes() {
	var keep []string
	for _, s := range a.notes {
		if !complTopicContentRe.MatchString(s) || !a.topicContentRoute(s) {
			keep = append(keep, s)
		}
	}
	a.notes = keep
}

// topicContentRoute places one complTopicContentRe note (topicContentRoutes)
// and reports whether it did.
func (a *compAssembler) topicContentRoute(s string) bool {
	desigs, levels := routeFilters(s)
	for _, c := range codeRe.FindAllString(s, -1) {
		desigs = dedupe(append(desigs, c[:3]))
	}
	if desigs == nil {
		return false
	}
	// pools reports whether n is a pool of listed courses the route widens.
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
	// A rule before may have left the sentence as a bare alternative of the
	// row (upToByCourse, ASMIN1778): bounded now, it goes.
	walkReq(target.r.Req, func(n *types.ReqNode) {
		if n.Type == types.NodeOr && len(n.Items) > 2 {
			n.Items = slices.DeleteFunc(n.Items, func(x *types.ReqNode) bool { return x.Type == types.NodeText && x.Text == s })
		}
	})
	addRaw(target, s)
	return true
}

// dsrLanguagePools reads the Department for the Study of Religion's
// language paragraph, "Up to two language courses totaling 1.0 credit in
// Pali ( RLG264H1, RLG265H1), ... or Modern Hebrew ( MHB155H1, ...) will
// count toward the Department for the Study of Religion courses required to
// complete any Major Program in Religion. If a student wishes a different
// language ... they should consult with the Associate Chair, Undergraduate"
// (R13). The cap is a limit already (courseCountCap); the courses count as
// the department's courses, so each pool of its designators (credits{RLG,
// ...}) takes the codes of other designators at its levels, and an item of
// listed courses whose floor is such a pool takes all of them. The
// paragraph is then no route on any row, and the different language, which
// names no pool, is a note.
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
	// widen adds the codes n's levels allow that its designators and
	// courses don't already hold, and reports whether n is such a pool.
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

// complUntargetedRouteRe: an approval route that names no course, pool or
// requirement: "Alternative courses may be substituted with the prior
// approval of the Director, Rotman Commerce", "Students can request
// substitutions from the IRP Director", "Other courses from
// UTM/UTSC/other Departments may be considered with permission", and the
// program-wide "Students may be permitted to take an independent reading
// course. A 0.5 credit of these courses may count towards program
// requirements, where relevant (...)".
var complUntargetedRouteRe = regexp.MustCompile(`(?i)^alternative\s+courses\s+may\s+be\s+substituted\s+with\s+the\s+(?:prior\s+)?approval\s+of\b` +
	`|^students\s+can\s+request\s+substitutions\s+from\s+the\b` +
	`|^other\s+courses\s+from\s+[\w/\s]+?\s+may\s+be\s+considered\s+with\s+permission\.?$` +
	`|\bmay\s+count\s+towards?\s+(?:\w+\s+)?program\s+requirements,\s+where\s+relevant\b`)

// untargetedRoutes moves the complUntargetedRouteRe alternatives of the
// allocations to the notes (R11): a route that names no target is a note,
// as an advisor records the substitution it allows; on a row it made the
// row Review for a student short of it. A bounded form, and(pool,
// text), and a route on a constraint ("as courses at the 400-level") stay.
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

// placedElsewhere drops a route placed on two rows from the one it fits
// less (R11): a bare text alternative at the top of an allocation, when
// another allocation reads the same sentence as structure (its raw holds
// the sentence but no text node does: "Students taking one of these MAT
// courses can have it count in lieu of a 0.5 300-level ECO course", a
// substitute on the 300-level ECO pool), or holds it on the node of the
// courses it replaces ("... towards the requirement to complete PCL472Y1/
// PCL474Y1/ JPM400Y1", on or(PCL472Y1, PCL474Y1, JPM400Y1, text)) rather
// than on a pool that happens to hold one of them.
func (a *compAssembler) placedElsewhere() {
	var rows []*compRow
	walkRows(a.items, func(r *compRow) {
		if r.r.Kind == types.RequirementAllocation && r.r.Req != nil {
			rows = append(rows, r)
		}
	})
	// fits reports whether row r2 holds text t better than r does at its
	// top: nested on a node of r2, or read into r2's alternatives (r2 an or
	// with no text left, whose raw holds t).
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

// complRouteCodesPat: codes in a route, "GGR493Y1", "GGR491Y1, GGR492H1, or
// GGR499H1", "CSC494H1/ CSC495H1", "POL336H1; POL357H1; ... POL497H1".
const complRouteCodesPat = `((?:` + codePat + `)(?:\s*(?:[/,;]|,?\s*(?:or|and))\s*` + codePat + `)*)`

var (
	// complSuitableCodesRe: "0.5 credit from GGR493Y1 can be used to meet
	// this requirement, based on suitability of the placement ...", "Up to
	// 1.0 credit can be used from GGR493Y1, based on suitability ...", "Up
	// to 0.5 credit from GGR491Y1, ..., and GGR499H1 based on suitability
	// ...". The groups: "up to" (a cap), the amount, the codes.
	complSuitableCodesRe = regexp.MustCompile(`(?i)^(up\s+to\s+)?(?:an?\s+|one\s+)?(\d+(?:\.\d+)?)\s+credits?\s+(?:can\s+be\s+used\s+)?from\s+` + complRouteCodesPat + `,?\s+(?:can\s+be\s+used\s+to\s+meet\s+this\s+requirement,?\s+)?based\s+on\s+(?:the\s+)?suitability\b`)
	// complRouteOfferingsRe (beside someOfferings' complSomeOfferingsRe,
	// which reads commas only): "Some offerings of POL336H1; ...; POL497H1 may
	// also count towards requirement 2 of this focus at the department's
	// discretion".
	complRouteOfferingsRe = regexp.MustCompile(`(?i)^(?:note:\s*)?some\s+offerings\s+of\s+` + complRouteCodesPat + `\s+may\s+also\s+count\s+towards?\b`)
	// complDiscretionRe: someone decides, in a route that names its courses.
	complDiscretionRe = regexp.MustCompile(`(?i)\b(?:discretion|approv\w*|permission)\b`)
	// complSubstituteForRe: "Note that students may substitute ARH495H1
	// (Research Practicum) for 0.5 credit of this requirement ...". The
	// groups: the code, the cap.
	complSubstituteForRe = regexp.MustCompile(`(?i)\bstudents\s+may\s+substitute\s+(` + codePat + `)(?:\s*\([^)]*\))?\s+for\s+(\d+(?:\.\d+)?)\s+credits?\s+of\s+this\s+requirement\b`)
	// complSubstituteOneOfRe: "Students who complete an independent study
	// project ( CSC494H1/ CSC495H1) ... may request to substitute one of
	// CSC494H1/ CSC495H1 for one of the courses in list 2 above".
	complSubstituteOneOfRe = regexp.MustCompile(`(?i)\bmay\s+request\s+to\s+substitute\s+one\s+of\s+` + complRouteCodesPat + `\s+for\s+one\s+of\s+the\s+courses\s+in\b`)
	// complUnitTopicsRe: a unit's special-topics courses, named by subject:
	// "Special Topics courses, Research Opportunity courses, and Research
	// Excursion courses from Geography & Planning can be used to meet this
	// requirement, based on suitability ... and approval by the department"
	// (group 1: the subject), "Health related Anthropology Special Topics
	// courses may be chosen when offered at the third or fourth year levels"
	// (group 2: the subject).
	complUnitTopicsRe = regexp.MustCompile(`(?i)^(?:special\s+topics\s+courses,\s+research\s+opportunity\s+courses,\s+and\s+research\s+excursion\s+courses\s+from\s+([\w&\s]+?)\s+can\s+be\s+used\s+to\s+meet\s+this\s+requirement\b|([\w\s-]+?)\s+special\s+topics\s+courses\s+may\s+be\s+chosen\s+when\s+offered\s+at\s+the\s+[\w\s]+?\s+levels?$)`)
	// complOtherLevelRe: any course at a level, "Other 400-level courses with
	// a significant Digital Humanities practicum element can also be
	// considered for fulfillment of the capstone requirement, if students
	// receive written permission ...".
	complOtherLevelRe = regexp.MustCompile(`(?i)^other\s+([1-4]00)-level\s+courses\b.*\bcan\s+(?:also\s+)?be\s+considered\s+for\s+fulfil?lment\s+of\b`)
	// complOneOutsideRe: one course off the lists, "Note: One 0.5 credit
	// course at the 300- or 400-level not listed in the above clusters may
	// count towards the elective requirement through a nomination process".
	// Groups: the amount, the levels.
	complOneOutsideRe = regexp.MustCompile(`(?i)^(?:note:\s*)?one\s+(\d+(?:\.\d+)?)\s+credit\s+course\s+at\s+the\s+([\w\s-]+?levels?)\s+not\s+listed\s+in\s+the\s+above\s+(?:clusters|lists|groups)\s+may\s+count\s+towards?\b`)
	// complOtherProgramsRe: "Approved courses in other programs may be
	// substituted for up to 1.0 FAH credit". The groups: the cap, the
	// designator.
	complOtherProgramsRe = regexp.MustCompile(`(?i)^approved\s+courses\s+in\s+other\s+programs\s+may\s+be\s+substituted\s+for\s+up\s+to\s+(\d+(?:\.\d+)?)\s+((?-i:[A-Z]{3}))\s+credits?$`)
	// complSubjectShareRe: courses of subjects up to a cap, "In consultation
	// with the Undergraduate Coordinator, students may take 0.5 credit in
	// history or political science courses corresponding to their chosen
	// language area and taught outside the department", "... up to 1.0
	// credit in ... their two chosen language areas (0.5 credit per language
	// area) ...". The groups: the cap, the subjects, the cap per area.
	complSubjectShareRe = regexp.MustCompile(`(?i)^in\s+consultation\s+with\s+the\s+undergraduate\s+coordinator,\s+students\s+may\s+take\s+(?:up\s+to\s+)?(\d+(?:\.\d+)?)\s+credits?\s+in\s+([\w\s]+?)\s+courses\s+corresponding\s+to\s+their\s+(?:two\s+)?chosen\s+language\s+areas?(?:\s+\((\d+(?:\.\d+)?)\s+credits?\s+per\s+language\s+area\))?\s+and\s+taught\s+outside\s+the\s+department$`)
	// complSiblingSubstituteRe: the courses of the other part of an area,
	// "In consultation with the Undergraduate Coordinator, and based on
	// results in language placement tests, some students may substitute
	// language courses with the equivalent number of credits in requirement
	// 3 (below)".
	complSiblingSubstituteRe = regexp.MustCompile(`(?i)\bsome\s+students\s+may\s+substitute\s+language\s+courses\s+with\s+the\s+equivalent\s+number\s+of\s+credits\s+in\s+requirement\s+\d+\b`)
)

// compStrictPool is the strict reading of a row a named route widens: an
// amount of courses (with designators and levels, or resolved from lists)
// or one of some courses of the same weight; floors are the in-node floors
// and(pool, floor) carries (complCountFloorRe, a "including at least" part).
type compStrictPool struct {
	pool   *types.ReqNode // credits: amount, courses, designators, levels
	lists  []string       // the pool's lists, when it was read from them
	floors []*types.ReqNode
	byName map[string]*types.CourseList
}

// strictPool reads the non-text alternatives of or node n (namedRoutes):
// pools of the same amount, or courses of the same weight. ok is false for
// any other strict reading.
func (a *compAssembler) strictPool(n *types.ReqNode, byName map[string]*types.CourseList) (sp compStrictPool, ok bool) {
	plain := func(x *types.ReqNode) bool {
		return x.Type == types.NodeCredits && x.Amount > 0 && x.MaxAmount == nil && x.Items == nil && x.Breadth == nil && x.Exclude == nil &&
			x.ExcludeDesignators == nil && x.Requirements == nil && x.MinGrade == 0 && x.Weight == 0 && !x.Transfer && x.MaxCourseAmount == 0
	}
	var p *types.ReqNode
	for _, it := range n.Items {
		if hasText(it) {
			continue
		}
		switch {
		case it.Type == types.NodeAnd && len(it.Items) >= 2 && plain(it.Items[0]) && p == nil:
			for _, f := range it.Items[1:] {
				if !plain(f) || f.Amount >= it.Items[0].Amount {
					return sp, false
				}
			}
			sp.floors = it.Items[1:]
			it = it.Items[0]
			fallthrough
		case plain(it):
			if p == nil {
				p = cloneNode(it)
				if it.Lists != nil {
					if it.Designators != nil || it.Courses != nil {
						return sp, false
					}
					sp.lists = it.Lists
					// Lists with filters stay a pool of lists, which only a
					// route naming courses already on them widens (routeWider).
					if codes, ok := a.listCodes(it.Lists, byName); ok {
						codes = slices.DeleteFunc(codes, func(c string) bool { return !levelMatches(it.Levels, c) })
						p.Courses, p.Levels, p.Lists = codes, nil, nil
					}
				}
				continue
			}
			if it.Amount != p.Amount || it.Designators != nil || it.Levels != nil || it.Lists != nil || p.Designators != nil || p.Levels != nil || p.Lists != nil || sp.floors != nil {
				return sp, false
			}
			p.Courses = dedupe(append(p.Courses, it.Courses...))
		case it.Type == types.NodeCourse:
			w := courseWeight(it.Code)
			if p == nil {
				p = &types.ReqNode{Type: types.NodeCredits, Amount: w}
			}
			if p.Amount != w || p.Designators != nil || p.Levels != nil || p.Lists != nil || sp.floors != nil {
				return sp, false
			}
			p.Courses = dedupe(append(p.Courses, it.Code))
		default:
			return sp, false
		}
	}
	sp.pool = p
	sp.byName = byName
	return sp, p != nil
}

// namedRoutes bounds the approval routes that name their wider pool by
// codes, a unit, subjects or levels, which boundRoutes' designator form
// (complBoundPoolRe) doesn't read (R1, design 3.3): each such alternative
// text{route} on or(strict, ...) becomes and(wider, text), wider being the
// strict pool and the route's courses, so a course outside both is NotMet
// rather than Review. The route caps its courses with a limit under the row
// where it says so ("Up to 1.0 credit can be used from GGR493Y1, based on
// suitability of placement ..."). The readers:
//   - named codes (complSuitableCodesRe, complRouteOfferingsRe,
//     complSubstituteForRe, complSubstituteOneOfRe): the strict pool's
//     courses and the codes;
//   - a unit's special-topics courses (complUnitTopicsRe): the pool and the
//     subject's designator, at the levels named when every strict course
//     is at them, else a list of the row's own (the pool's courses and the
//     designator at the levels); kind content where no one approves ("may
//     be chosen");
//   - other courses at a level (complOtherLevelRe): any course at it, when
//     every strict course is at it;
//   - one course off the lists (complOneOutsideRe): the amount of any
//     courses, the rest from the strict pool; at the levels it names, the
//     amount from a list of the row's own (the pool, and any course at
//     them);
//   - courses of other programs for part of a designator's credits
//     (complOtherProgramsRe): any course, with a limit on the other
//     designators;
//   - courses of subjects for part of a pool (complSubjectShareRe): the pool
//     and the subjects' designators, with a limit on their courses off the
//     pool;
//   - the area's other part (complSiblingSubstituteRe): the pool and the
//     courses of the one other allocation under the same parent.
//
// A pool of lists is read as the courses the lists hold (listCodes); an
// in-node floor and(pool, floor) becomes a constraint under the row, so the
// floor holds for the route too. A route the readers don't read, or whose
// pool can't be widened this way, stays bare (D3).
func (a *compAssembler) namedRoutes(lists []*types.CourseList) {
	byName := map[string]*types.CourseList{}
	for _, l := range lists {
		byName[l.Name] = l
	}
	var walk func(rs []*compRow, parent *compRow)
	walk = func(rs []*compRow, parent *compRow) {
		for _, r := range rs {
			a.namedRoute(r, parent, byName)
			walk(r.children, r)
		}
	}
	if a.total != nil {
		walk([]*compRow{a.total}, nil)
	}
	walk(a.floors, nil)
	walk(a.items, nil)
	walk(a.program, nil)
}

// namedRoute bounds the named routes of one row (namedRoutes).
func (a *compAssembler) namedRoute(r, parent *compRow, byName map[string]*types.CourseList) {
	n := r.r.Req
	if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr || len(n.Items) < 2 {
		return
	}
	// The constraints already under the row (its floors), before the limits
	// a route adds.
	floors := slices.DeleteFunc(slices.Clone(r.children), func(c *compRow) bool { return c.r.Kind != types.RequirementConstraint || c.r.Req == nil })
	sp, ok := a.strictPool(n, byName)
	if !ok {
		return
	}
	strict := sp.pool
	bounded := false
	for i, it := range n.Items {
		// Kind other too: textKinds, which makes such a route permission,
		// runs later; the readers are specific.
		if it.Type != types.NodeText || it.Kind != types.TextPermission && it.Kind != types.TextOther {
			continue
		}
		wider, kind, limits := a.routeWider(sp, a.clean(it.Text), r, parent)
		if wider == nil {
			continue
		}
		text := cloneNode(it)
		text.Kind = kind
		n.Items[i] = andNode(wider, text)
		// The floors already under the row: a floor on the row's courses
		// holds the named courses too, as Review ("Note that students may
		// substitute ARH495H1 ... for 0.5 credit of this requirement
		// (assignment to Group A or B to be determined by Undergraduate
		// Coordinator)", over floors on Groups A and B): or(floor, and(floor
		// with the courses, text)). A floor on levels the named courses are
		// all at stays; any other floor keeps the route as its Review
		// alternative, or(floor, text), as markIncomplete marked it beside
		// the bare route: the route's courses may count toward it ("the
		// relevant cluster" of a nominated course).
		var added []string
		if wider.Type == types.NodeCredits && wider.Designators == nil && wider.Lists == nil && wider.Levels == nil {
			added = slices.DeleteFunc(slices.Clone(wider.Courses), func(c string) bool { return slices.Contains(strict.Courses, c) })
		}
		for _, f := range floors {
			switch {
			case added != nil && a.floorWith(f.r.Req, strict.Courses, added, byName) != nil:
				f.r.Req = orNode(f.r.Req, andNode(a.floorWith(f.r.Req, strict.Courses, added, byName), cloneNode(text)))
			case added != nil && levelFloor(f.r.Req) && !slices.ContainsFunc(added, func(c string) bool { return !levelMatches(f.r.Req.Levels, c) }):
			case wider.Type == types.NodeCredits && levelFloor(f.r.Req) && slices.Equal(wider.Levels, f.r.Req.Levels):
			case wider.Type == types.NodeAnd && spreadFloor(f.r.Req) != nil:
				// One course off the lists, in "the relevant cluster": the
				// spread over one list fewer.
				c := cloneNode(spreadFloor(f.r.Req))
				c.Count--
				if c.Count == 1 {
					// One part: a course on two lists can't count twice, so
					// the strict spread's exclusions go.
					for _, it := range c.Items {
						it.Exclude = nil
					}
				}
				f.r.Req = orNode(f.r.Req, andNode(c, cloneNode(text)))
			default:
				f.r.Req = orNode(f.r.Req, cloneNode(text))
			}
		}
		for _, l := range limits {
			r.children = append(r.children, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Req: l, Raw: it.Text}})
		}
		bounded = true
	}
	if !bounded || sp.floors == nil {
		return
	}
	// The in-node floor, a constraint under the row now that no route is
	// bare beside it.
	for k, it := range n.Items {
		if it.Type == types.NodeAnd && !hasText(it) {
			n.Items[k] = it.Items[0]
			for _, f := range it.Items[1:] {
				r.children = append(r.children, &compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Req: f, Raw: rawHead(r.r)}})
			}
		}
	}
}

// routeWider returns the wider pool route s names for strict (namedRoutes),
// the kind of its text and the limits it states, or nil.
func (a *compAssembler) routeWider(sp compStrictPool, s string, r, parent *compRow) (*types.ReqNode, types.TextKind, []*types.ReqNode) {
	strict := sp.pool
	wider := cloneNode(strict)
	// withCodes widens wider by codes, which must be at the pool's levels. A
	// pool of lists with filters (listCodes can't list it) takes only codes
	// already on its lists: the route then adds no course, only its cap.
	withCodes := func(codes []string) bool {
		if slices.ContainsFunc(codes, func(c string) bool { return !levelMatches(strict.Levels, c) }) {
			return false
		}
		if strict.Lists != nil {
			return !slices.ContainsFunc(codes, func(c string) bool { return !listHas(strict.Lists, c, sp.byName, 0) })
		}
		wider.Courses = dedupe(append(wider.Courses, codes...))
		return true
	}
	capOf := func(codes []string, v float64) []*types.ReqNode {
		if v <= 0 || v >= strict.Amount && v >= weightOf(codes) {
			return nil
		}
		return []*types.ReqNode{{Type: types.NodeCredits, Courses: dedupe(slices.Clone(codes)), MaxAmount: maxAmount(v)}}
	}
	if m := complSuitableCodesRe.FindStringSubmatch(s); m != nil {
		codes := codeRe.FindAllString(m[3], -1)
		if !withCodes(codes) {
			return nil, "", nil
		}
		var limits []*types.ReqNode
		if m[1] != "" {
			v, _ := strconv.ParseFloat(m[2], 64)
			limits = capOf(codes, v)
		}
		return wider, types.TextPermission, limits
	}
	if m := complRouteOfferingsRe.FindStringSubmatch(s); m != nil {
		if !withCodes(codeRe.FindAllString(m[1], -1)) {
			return nil, "", nil
		}
		// The offering's topic decides (D1, as someOfferings), unless
		// someone does ("at the department's discretion").
		if complDiscretionRe.MatchString(s) {
			return wider, types.TextPermission, nil
		}
		return wider, types.TextContent, nil
	}
	if m := complSubstituteForRe.FindStringSubmatch(s); m != nil {
		v, _ := strconv.ParseFloat(m[2], 64)
		if !withCodes([]string{m[1]}) {
			return nil, "", nil
		}
		return wider, types.TextPermission, capOf([]string{m[1]}, v)
	}
	if m := complSubstituteOneOfRe.FindStringSubmatch(s); m != nil {
		codes := codeRe.FindAllString(m[1], -1)
		one := 0.0
		for _, c := range codes {
			one = max(one, courseWeight(c))
		}
		if !withCodes(codes) {
			return nil, "", nil
		}
		return wider, types.TextPermission, capOf(codes, one)
	}
	if strict.Lists != nil && !complOneOutsideRe.MatchString(s) {
		return nil, "", nil // the readers below widen pools of courses or filters
	}
	if m := complUnitTopicsRe.FindStringSubmatch(s); m != nil {
		desigs, _ := routeFilters(m[1] + m[2])
		_, levels := routeFilters(s)
		if len(desigs) != 1 || strict.Levels != nil {
			return nil, "", nil
		}
		switch {
		case levels != nil && strict.Designators == nil && strict.Lists == nil && strict.Courses != nil &&
			slices.ContainsFunc(strict.Courses, func(c string) bool { return !levelMatches(levels, c) }):
			// Some pool course below the levels ("ANT208H1" beside "at the
			// third or fourth year levels"): a list of the row's own, the
			// pool's courses and the designator's at the levels.
			l := a.rowUnion(r, []*types.ReqNode{
				{Type: types.NodeCredits, Courses: slices.Clone(strict.Courses)},
				{Type: types.NodeCredits, Designators: desigs, Levels: levels},
			})
			wider = &types.ReqNode{Type: types.NodeCredits, Amount: strict.Amount, Lists: []string{l.Name}}
		default:
			wider.Designators = dedupe(append(wider.Designators, desigs...))
			if strict.Designators == nil && !slices.ContainsFunc(strict.Courses, func(c string) bool { return !levelMatches(levels, c) }) {
				wider.Levels = levels
			}
		}
		kind := types.TextPermission
		if m[2] != "" {
			kind = types.TextContent // the course's topic decides (D1)
		}
		return wider, kind, nil
	}
	if m := complOtherLevelRe.FindStringSubmatch(s); m != nil {
		levels := parseLevels(m[1] + "-level")
		if d, _ := routeFilters(s); d != nil || strict.Designators != nil || strict.Levels != nil ||
			slices.ContainsFunc(strict.Courses, func(c string) bool { return !levelMatches(levels, c) }) {
			return nil, "", nil
		}
		return &types.ReqNode{Type: types.NodeCredits, Amount: strict.Amount, Levels: levels}, types.TextPermission, nil
	}
	if m := complOneOutsideRe.FindStringSubmatch(s); m != nil {
		v, _ := strconv.ParseFloat(m[1], 64)
		if v >= strict.Amount {
			return nil, "", nil
		}
		// The amount of any courses, the rest from the strict pool as stated
		// (its lists): and(credits{amount}, credits{amount − v, pool}).
		w := cloneNode(r.r.Req.Items[0])
		if w.Type != types.NodeCredits {
			return nil, "", nil
		}
		w.Amount = strict.Amount - v
		total := &types.ReqNode{Type: types.NodeCredits, Amount: strict.Amount}
		if levels := parseLevels(m[2]); levels != nil && w.MaxAmount == nil {
			// "at the 300- or 400-level": the total from a list of the row's
			// own, the pool and any course at those levels.
			item := cloneNode(w)
			item.Amount = 0
			l := a.rowUnion(r, []*types.ReqNode{item, {Type: types.NodeCredits, Levels: levels}})
			total.Lists = []string{l.Name}
		}
		return andNode(total, w), types.TextPermission, nil
	}
	if m := complOtherProgramsRe.FindStringSubmatch(s); m != nil {
		v, _ := strconv.ParseFloat(m[1], 64)
		if !slices.Equal(strict.Designators, []string{m[2]}) || strict.Courses != nil || strict.Levels != nil || v >= strict.Amount {
			return nil, "", nil
		}
		return &types.ReqNode{Type: types.NodeCredits, Amount: strict.Amount}, types.TextPermission,
			[]*types.ReqNode{{Type: types.NodeCredits, ExcludeDesignators: []string{m[2]}, MaxAmount: maxAmount(v)}}
	}
	if m := complSubjectShareRe.FindStringSubmatch(s); m != nil {
		desigs, levels := routeFilters(m[2])
		if len(desigs) < 1 || levels != nil || strict.Designators != nil || strict.Levels != nil {
			return nil, "", nil
		}
		v, _ := strconv.ParseFloat(m[1], 64)
		if m[3] != "" {
			v, _ = strconv.ParseFloat(m[3], 64) // per area: per row
		}
		wider.Designators = desigs
		var listed []string
		for _, c := range strict.Courses {
			if slices.Contains(desigs, c[:3]) {
				listed = append(listed, c)
			}
		}
		return wider, types.TextPermission, []*types.ReqNode{{Type: types.NodeCredits, Designators: slices.Clone(desigs), Exclude: listed, MaxAmount: maxAmount(v)}}
	}
	if complSiblingSubstituteRe.MatchString(s) && parent != nil {
		var other *compRow
		for _, c := range parent.children {
			if c != r && c.r.Kind == types.RequirementAllocation {
				if other != nil {
					return nil, "", nil
				}
				other = c
			}
		}
		if other == nil || other.r.Req == nil {
			return nil, "", nil
		}
		on := other.r.Req
		if on.Type != types.NodeOr {
			on = orNode(on)
		}
		osp, ok := a.strictPool(on, sp.byName)
		if !ok || osp.pool.Designators != nil || osp.pool.Levels != nil || !withCodes(osp.pool.Courses) {
			return nil, "", nil
		}
		return wider, types.TextPermission, nil
	}
	return nil, "", nil
}

// listHas reports whether code is on one of the lists names (byName): listed,
// or matched by an item's designators and levels and not excluded. An item
// with other filters (breadth, requirements) is read as not holding it.
func listHas(names []string, code string, byName map[string]*types.CourseList, depth int) bool {
	if depth > 4 {
		return false
	}
	for _, name := range names {
		l := byName[name]
		if l == nil {
			continue
		}
		for _, it := range l.Items {
			if it.Type != types.NodeCredits || it.Breadth != nil || it.Requirements != nil || slices.Contains(it.Exclude, code) || !levelMatches(it.Levels, code) {
				continue
			}
			switch {
			case slices.Contains(it.Courses, code):
				return true
			case it.Lists != nil && it.Courses == nil && it.Designators == nil:
				if listHas(it.Lists, code, byName, depth+1) {
					return true
				}
			case it.Designators != nil && it.Courses == nil && it.Lists == nil && len(code) >= 3 && slices.Contains(it.Designators, code[:3]):
				return true
			}
		}
	}
	return false
}

// floorWith returns floor n (a constraint under a row whose strict pool
// lists courses) with the courses added to each pool of n that lists only
// some of courses, or nil when n has no such pool or any other node but
// and, or and text. A pool of lists only counts as the lists' codes less its
// exclusions (a floor on Groups A and B of a row whose pool is the two
// groups).
func (a *compAssembler) floorWith(n *types.ReqNode, courses, added []string, byName map[string]*types.CourseList) *types.ReqNode {
	if n.Type == types.NodeOr {
		for _, it := range n.Items {
			if !hasText(it) {
				return a.floorWith(it, courses, added, byName)
			}
		}
		return nil
	}
	w := cloneNode(n)
	found, ok := false, true
	walkReq(w, func(x *types.ReqNode) {
		switch x.Type {
		case types.NodeAnd, types.NodeOr, types.NodeText:
		case types.NodeCredits:
			if x.Lists != nil && x.Courses == nil && x.Designators == nil && x.Levels == nil {
				codes, lok := a.listCodes(x.Lists, byName)
				if !lok {
					ok = false
					return
				}
				x.Courses = slices.DeleteFunc(codes, func(c string) bool { return slices.Contains(x.Exclude, c) })
				x.Lists, x.Exclude = nil, nil
			}
			if x.Courses == nil || x.Designators != nil || x.Lists != nil || slices.ContainsFunc(x.Courses, func(c string) bool { return !slices.Contains(courses, c) }) {
				ok = false
				return
			}
			x.Courses = dedupe(append(x.Courses, added...))
			found = true
		default:
			ok = false
		}
	})
	if !found || !ok {
		return nil
	}
	return w
}

// complExemptAtLevelRe: an exemption replaced by a designator's courses at a
// level, worded after the course (exemptAny's complExemptAnyRe reads "any
// FRE 200+ or FSL 300+ level course"): "Students exempted from FSL320H1/
// FSL322H1 may replace it with any FRE course at the 300+ level". The
// groups: the codes, the designator, the level.
var complExemptAtLevelRe = regexp.MustCompile(`(?i)^students\s+exempted\s+from\s+` + complRelaxCodesPat + `\s+may\s+replace\s+(?:it|them)\s+with\s+any\s+((?-i:[A-Z]{3}))\s+course\s+at\s+the\s+([1-4]00)\+\s+level$`)

// exemptAtLevel bounds the complExemptAtLevelRe alternatives of the rows'
// "or" nodes (R1): and(credits{the credits the exemption takes, DES, the
// level and up}, text{placement}), as exemptAny does for its wording.
func (a *compAssembler) exemptAtLevel() {
	var walk func(n *types.ReqNode)
	walk = func(n *types.ReqNode) {
		if n == nil {
			return
		}
		for _, it := range n.Items {
			walk(it)
		}
		if n.Type != types.NodeOr {
			return
		}
		for i, it := range n.Items {
			if it.Type != types.NodeText {
				continue
			}
			m := complExemptAtLevelRe.FindStringSubmatch(a.clean(it.Text))
			if m == nil {
				continue
			}
			v := exemptedCredits(n, codeRe.FindAllString(m[1], -1))
			if v == 0 {
				continue
			}
			from, _ := strconv.Atoi(m[3])
			var levels []int
			for l := from; l <= 400; l += 100 {
				levels = append(levels, l)
			}
			n.Items[i] = andNode(&types.ReqNode{Type: types.NodeCredits, Amount: v, Designators: []string{m[2]}, Levels: levels}, textNode(types.TextPlacement, it.Text))
		}
	}
	for _, rs := range [][]*compRow{a.floors, a.items, a.program} {
		var rows func(rs []*compRow)
		rows = func(rs []*compRow) {
			for _, r := range rs {
				walk(r.r.Req)
				rows(r.children)
			}
		}
		rows(rs)
	}
}

// levelFloor reports a floor on levels only: credits{amount, levels}.
func levelFloor(n *types.ReqNode) bool {
	return n.Type == types.NodeCredits && n.Amount > 0 && n.Levels != nil && n.Courses == nil && n.Designators == nil && n.Lists == nil &&
		n.Breadth == nil && n.Requirements == nil && n.Exclude == nil && n.Items == nil && n.MaxAmount == nil
}

// spreadFloor returns the choose node of a spread floor (choose{k ≥ 2, ...},
// alone or as the strict alternative of an or), or nil.
func spreadFloor(n *types.ReqNode) *types.ReqNode {
	if n.Type == types.NodeOr && len(n.Items) > 0 {
		n = n.Items[0]
	}
	if n.Type == types.NodeChoose && n.Count >= 2 {
		return n
	}
	return nil
}

// otherProgramsFloors places a complOtherProgramsRe note ("Approved courses
// in other programs may be substituted for up to 2.0 FAH credits") that no
// row's pool took (namedRoutes reads it on a total of the designator's
// credits) on the floor of the designator's credits, a constraint
// credits{M, DES} (ASSPE0908, "At least 9.0 FAH credits"): or(floor,
// and(credits{M − N, DES}, text{permission})). The substitutes count toward
// the floor only, not the total, so no limit goes on the program: its other
// credits (the languages) are not substitutes.
func (a *compAssembler) otherProgramsFloors() {
	var keep []string
	for _, s := range a.notes {
		m := complOtherProgramsRe.FindStringSubmatch(strings.TrimSuffix(a.clean(s), "."))
		if m == nil || !a.otherProgramsFloor(s, m) {
			keep = append(keep, s)
		}
	}
	a.notes = keep
}

// otherProgramsFloor places one note (otherProgramsFloors) and reports
// whether it did: on the one floor that fits.
func (a *compAssembler) otherProgramsFloor(s string, m []string) bool {
	v, _ := strconv.ParseFloat(m[1], 64)
	var fit []*compRow
	groups := [][]*compRow{a.floors, a.items, a.program}
	if a.total != nil {
		groups = append(groups, a.total.children)
	}
	for _, rs := range groups {
		for _, r := range rs {
			n := r.r.Req
			if r.r.Kind == types.RequirementConstraint && n != nil && n.Type == types.NodeCredits && slices.Equal(n.Designators, []string{m[2]}) &&
				n.Courses == nil && n.Levels == nil && n.Lists == nil && n.Exclude == nil && n.Amount > v && len(r.children) == 0 {
				fit = append(fit, r)
			}
		}
	}
	if len(fit) != 1 {
		return false
	}
	r := fit[0]
	less := cloneNode(r.r.Req)
	less.Amount -= v
	r.r.Req = orNode(r.r.Req, andNode(less, textNode(types.TextPermission, strings.TrimSuffix(s, "."))))
	addRaw(r, s)
	return true
}
