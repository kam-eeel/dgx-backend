package reqs

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Readings of wordings that one or two programs use, where no wider rule
// covers them.

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

// areaCorrespondence ties a language-area item to the culture item for the
// chosen area: each area becomes a group of its two parts, under a choice of
// one. Areas pair by name or correspondence; if any doesn't, nothing changes.
func (a *compAssembler) areaCorrespondence() {
	find := func(re func(string) []string) *compRow {
		for _, r := range a.items {
			t := strings.TrimSpace(complItemLabelRe.ReplaceAllString(stripEmphasis(rawHead(r.r)), ""))
			if r.r.Kind == types.RequirementAllocation && len(r.children) == 0 && re(t) != nil {
				return r
			}
		}
		return nil
	}
	lang, cult := find(complOneAreaRe.FindStringSubmatch), find(complChosenAreaRe.FindStringSubmatch)
	if lang == nil || cult == nil {
		return
	}
	type area struct {
		name, other string
		pools       []*types.ReqNode
		line        string
	}
	read := func(row *compRow, re func(string) []string) (areas []area, alts []*types.ReqNode, ok bool) {
		m := re(strings.TrimSpace(complItemLabelRe.ReplaceAllString(stripEmphasis(rawHead(row.r)), "")))
		for _, d := range a.listDefs {
			if d.row != row || d.block == nil {
				continue
			}
			pools, ok := areaPools(m[1], d.members)
			if !ok {
				return nil, nil, false
			}
			title := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(strings.ReplaceAll(stripEmphasis(d.title), "_", "")), ":"))
			ar := area{name: strings.TrimSpace(complAreaParenRe.ReplaceAllString(title, "")), pools: pools, line: a.line(d.block)}
			if cm := complCorrespondsRe.FindStringSubmatch(title); cm != nil {
				ar.name, ar.other = strings.TrimSpace(cm[1]), strings.TrimSpace(cm[2])
			}
			areas = append(areas, ar)
		}
		// the body as areaChoice and the routes left it: or(the areas' pools,
		// [the correspondence's and], text alternatives)
		n := row.r.Req
		if n == nil || n.Type != types.NodeOr {
			return nil, nil, false
		}
		for _, it := range n.Items {
			if it.Type == types.NodeText {
				alts = append(alts, it)
			}
		}
		return areas, alts, len(areas) >= 2
	}
	las, lalts, ok := read(lang, complOneAreaRe.FindStringSubmatch)
	if !ok {
		return
	}
	cas, calts, ok := read(cult, complChosenAreaRe.FindStringSubmatch)
	if !ok || len(cas) != len(las) {
		return
	}
	var groups []*compRow
	used := map[int]bool{}
	for _, x := range las {
		k := slices.IndexFunc(cas, func(y area) bool {
			return strings.EqualFold(y.name, x.name) || x.other != "" && strings.EqualFold(y.name, x.other) || y.other != "" && strings.EqualFold(y.other, x.name)
		})
		if k < 0 || used[k] {
			return
		}
		used[k] = true
		g := &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Label: x.name, Section: lang.r.Section, Raw: x.line + "\n" + cas[k].line}}
		for _, part := range []struct {
			pools, alts []*types.ReqNode
			line        string
			section     string
		}{{x.pools, lalts, x.line, lang.r.Section}, {cas[k].pools, calts, cas[k].line, cult.r.Section}} {
			body := slices.Clone(part.pools)
			for _, t := range part.alts {
				body = append(body, textNode(t.Kind, t.Text)) // a node of each row's own
			}
			row := &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Section: part.section, Req: orNode(body...), Raw: part.line}}
			for _, t := range part.alts {
				addRaw(row, t.Text)
			}
			g.children = append(g.children, row)
		}
		groups = append(groups, g)
	}
	var nodes []*types.ReqNode
	for _, g := range groups {
		nodes = append(nodes, a.reqNode(g))
	}
	choice := &compRow{r: &types.Requirement{Kind: types.RequirementChoice, Section: lang.r.Section, Req: choiceNode(1, nodes), Raw: rawHead(lang.r) + "\n" + rawHead(cult.r), Notes: dedupe(append(slices.Clone(lang.r.Notes), cult.r.Notes...))}}
	var items []*compRow
	for _, r := range a.items {
		switch r {
		case lang:
			items = append(append(items, groups...), choice)
		case cult:
		default:
			items = append(items, r)
		}
	}
	a.items = items
}

var (
	// complGatewayRe matches courses grouped under the disciplines that offer
	// them: "1.0 credit from the gateway courses in American Studies ( AMS100H1),
	// English ( ENG250H1 and ENG270H1), ...".
	complGatewayRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+from\s+the\s+[\w-]+\s+courses\s+in\s+(.+)$`)
	// complGatewayGroupRe matches one discipline and its courses, and the
	// separator after it: "English ( ENG250H1 and ENG270H1), ".
	complGatewayGroupRe = regexp.MustCompile(`^\s*[A-Z][A-Za-z]*(?:\s+[A-Z][A-Za-z]*)*\s*\(\s*(` + codePat + `(?:\s+and\s+` + codePat + `)*)\s*\)\s*(?:,\s*(?:or\s+)?|,?\s+or\s+|$)`)
	// complAdditionalUnionRe matches Req 1's pool as designators or lists:
	// "Additional AMS/USA courses or courses from the American Studies Program
	// disciplinary/thematic clusters to a total of 7.0 credits".
	complAdditionalUnionRe = regexp.MustCompile(`(?i)^additional\s+((?-i:[A-Z]{3})(?:/(?-i:[A-Z]{3}))*)\s+courses\s+or\s+courses\s+from\s+the\s+[\w\s/-]*?\b(clusters|groups)\s+to\s+a\s+total\s+of\s+(\d+(?:\.\d+)?)\s+credits?(?:,\s*including\s+requirement\s+#?\d+\s+above)?$`)
)

// oneOffRow reads a row of text (text: the body, or the first item of
// or(text, routes)) with the one-off readers, once the lists are known. ok
// reports whether it read the row; rows is what stands for it.
func (a *compAssembler) oneOffRow(row *compRow, text *types.ReqNode, ls compListRefs) (rows []*compRow, ok bool) {
	var alts []*types.ReqNode
	if n := row.r.Req; n != text {
		alts = n.Items[1:]
	}
	if a.additionalUnion(row, text.Text, alts, ls) {
		return nil, true // the row joins Req 1
	}
	if a.residueAltRow(row, text, ls) {
		return []*compRow{row}, true
	}
	if text == row.r.Req && a.gatewayGroups(row) {
		return []*compRow{row}, true
	}
	if a.moreOneOffRow(row, text, alts, ls) {
		return []*compRow{row}, true
	}
	if a.unitRows(row, text, ls) {
		return []*compRow{row}, true
	}
	return a.residueRow(row, text, ls)
}

// gatewayGroups reads complGatewayRe as the or of the groups that make the
// amount, each the and of its courses. Whether courses of several groups may
// make it is the text's ambiguity: the pool of all of them is the Review
// alternative. It reports whether it read the row.
func (a *compAssembler) gatewayGroups(row *compRow) bool {
	n := row.r.Req
	if row.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || n.Kind != types.TextOther || len(row.children) > 0 {
		return false
	}
	m := complGatewayRe.FindStringSubmatch(strings.TrimSuffix(complClean(n.Text), "."))
	if m == nil {
		return false
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	var strict []*types.ReqNode
	var all []string
	for rest := m[2]; rest != ""; {
		g := complGatewayGroupRe.FindStringSubmatch(rest)
		if g == nil {
			return false
		}
		rest = rest[len(g[0]):]
		codes := codeRe.FindAllString(g[1], -1)
		all = append(all, codes...)
		sum := 0.0
		var group []*types.ReqNode
		for _, c := range codes {
			sum += courseWeight(c)
			group = append(group, &types.ReqNode{Type: types.NodeCourse, Code: c})
		}
		if math.Abs(sum-amount) < 1e-9 {
			strict = append(strict, andNode(group...))
		}
	}
	if len(strict) == 0 || len(all) < 2 {
		return false
	}
	lenient := andNode(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: dedupe(all)}, textNode(types.TextAmbiguity, n.Text))
	row.r.Req = orNode(append(strict, lenient)...)
	return true
}

// additionalUnion reads complAdditionalUnionRe as Req 1's pool: a union of
// the designators and every list named by the word ("Cluster 1", ...). The
// row joins Req 1's raw, routes and notes. It reads nothing unless Req 1 is a
// bare pool of the total's amount and two lists are named.
func (a *compAssembler) additionalUnion(row *compRow, text string, alts []*types.ReqNode, ls compListRefs) bool {
	m := complAdditionalUnionRe.FindStringSubmatch(strings.TrimSuffix(complClean(text), "."))
	if m == nil || a.total == nil || len(row.children) > 0 || !slices.Contains(a.items, row) {
		return false
	}
	pool := a.total.r.Req
	v, _ := strconv.ParseFloat(m[3], 64)
	if pool == nil || pool.Type != types.NodeCredits || pool.Amount != v || pool.Designators != nil || pool.Levels != nil || pool.Courses != nil || pool.Lists != nil {
		return false
	}
	word := strings.TrimSuffix(strings.ToLower(m[2]), "s")
	var names []string
	for _, l := range ls {
		if l.list != nil && !l.bad && strings.HasPrefix(strings.ToLower(l.list.Name), word+" ") {
			names = append(names, l.list.Name)
		}
	}
	if names = dedupe(names); len(names) < 2 {
		return false
	}
	slices.Sort(names)
	name, _, ok := a.addUnion(row, []*types.ReqNode{
		{Type: types.NodeCredits, Designators: strings.Split(m[1], "/")},
		{Type: types.NodeCredits, Lists: names},
	}, ls)
	if !ok {
		return false
	}
	pool.Lists = []string{name}
	if alts != nil {
		a.total.r.Req = orNode(append([]*types.ReqNode{pool}, alts...)...)
	}
	a.total.r.Raw += "\n" + row.r.Raw
	a.total.r.Notes = append(a.total.r.Notes, row.r.Notes...)
	return true
}

// oneOffs runs the one-off readings that need the routes placed and the lists
// read.
func (a *compAssembler) oneOffs() {
	a.areaCorrespondence()
	a.languageCourses()
	a.exclusionExceptions()
	a.designatorRules()
	a.restatedPools()
	a.breadthConditions()
	a.subjectRows()
	a.inlineDesignatorFloors()
	a.remainingFrom()
	a.consecutiveLanguages()
}

// complExclusionExceptRe matches a note lifting an item's exclusions: "the
// exclusion of GER400H1 and GER401H1 does not apply if the student's entry
// level is post- GER100Y1".
var complExclusionExceptRe = regexp.MustCompile(`(?i)\bthe\s+exclusion\s+of\s+(.+?)\s+does\s+not\s+apply\s+if\b`)

// exclusionExceptions reads complExclusionExceptRe as the pool's lenient
// alternative, or(pool, and(pool without the exclusions, text)): Met without
// the excluded courses, Review with them.
func (a *compAssembler) exclusionExceptions() {
	a.notes = slices.DeleteFunc(a.notes, func(note string) bool {
		m := complExclusionExceptRe.FindStringSubmatch(note)
		if m == nil {
			return false
		}
		codes := codeRe.FindAllString(m[1], -1)
		for _, r := range a.items {
			n := r.r.Req
			if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeCredits || len(codes) == 0 || slices.ContainsFunc(codes, func(c string) bool { return !slices.Contains(n.Exclude, c) }) {
				continue
			}
			wide := *n
			wide.Exclude = slices.DeleteFunc(slices.Clone(n.Exclude), func(c string) bool { return slices.Contains(codes, c) })
			if len(wide.Exclude) == 0 {
				wide.Exclude = nil // omitempty
			}
			// the student's entry level is the placement test's
			r.r.Req = orNode(n, andNode(&wide, textNode(types.TextPlacement, note)))
			addRaw(r, note)
			return true
		}
		return false
	})
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

// complOtherCoursesRe matches a route for courses other than those listed
// above it: "Other courses, such as Special Topics courses ..., may be
// approved by the Director".
var complOtherCoursesRe = regexp.MustCompile(`(?i)^other\s+courses\b.*\bmay\s+be\s+approved\b`)

// listRoutes places a complOtherCoursesRe route on the pool of courses whose
// raw ends on the line before it: or(pool, text{permission}).
// before placeProgramRoutes, which would keep it as a note
func (a *compAssembler) listRoutes() {
	a.programRoutes = slices.DeleteFunc(a.programRoutes, func(s string) bool {
		if !complOtherCoursesRe.MatchString(complClean(s)) {
			return false
		}
		at := slices.IndexFunc(a.lines, func(l string) bool { return strings.HasPrefix(strings.TrimSpace(l), strings.TrimSpace(s)) })
		if at < 1 {
			return false
		}
		prev := strings.TrimSpace(a.lines[at-1])
		for _, r := range a.items {
			lines := strings.Split(r.r.Raw, "\n")
			n := r.r.Req
			if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeCredits || n.Courses == nil || strings.TrimSpace(lines[len(lines)-1]) != prev {
				continue
			}
			r.r.Req = orNode(n, textNode(types.TextPermission, s))
			addRaw(r, s)
			return true
		}
		return false
	})
}

var (
	// complFromOneOfRe matches "credit from one of PHY224H1", which means
	// "credit from PHY224H1".
	complFromOneOfRe = regexp.MustCompile(`(?i)\b(credits?\s+from)\s+one\s+of\s+(` + codePat + `)`)
	// complAllRequiredRe matches a lead over courses all required: "2.0 credits
	// comprised of the following core PCJ courses, all of which are required: ".
	complAllRequiredRe = regexp.MustCompile(`(?i)^\d+(?:\.\d+)?\s+credits?\s+(?:comprised\s+)?of\s+the\s+following\s+[\w\s]*?courses,\s+all\s+of\s+which\s+are\s+required:\s*`)
	// complListFloorRe matches a floor before its list: "At least 3.5 credits
	// from the following list including at least 2.5 with the BMS/JSM/JWB
	// designator: BMS300H1/ ...".
	complListFloorRe = regexp.MustCompile(`(?i)^(?:at\s+least\s+)?(\d+(?:\.\d+)?\s+credits?)\s+from\s+the\s+following\s+list\s+including\s+at\s+least\s+(\d+(?:\.\d+)?)\s+(?:credits?\s+)?with\s+the\s+((?-i:[A-Z]{3})(?:/(?-i:[A-Z]{3}))*)\s+designators?:\s*(.+)$`)
	// complGroupFloorRe matches a group set out after its floors: "2.0 credits
	// from the Core Group at the 300+ level, at least 0.5 of which must be at the
	// 400-level: CAR315H1, ...". The groups: amount, level, floors, codes.
	complGroupFloorRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?\s+credits?)\s+from\s+the\s+[\w\s]+?\s+group(?:\s+at\s+the\s+([1-4])00\+\s+level)?(?:,\s*|\s*\(\s*)((?:including\s+)?at\s+least\s+[^():]+?)\)?:\s*((?:` + codePat + `,?\s*)+)$`)
	// complWhichFloorRe matches "at least 0.5 of which must be at the 400-level".
	complWhichFloorRe = regexp.MustCompile(`(?i)^at\s+least\s+(\d+(?:\.\d+)?)\s+of\s+which\s+must\s+be\s+(at\s+the\s+[1-4]00-level)$`)
	// complDesigButNotRe matches "in courses with a GER designator, but cannot be
	// GER400H1 or GER401H1".
	complDesigButNotRe = regexp.MustCompile(`(?i)\bin\s+courses\s+with\s+an?\s+((?-i:[A-Z]{3}))\s+designator,\s+but\s+cannot\s+be\s+(` + codePat + `)\s+or\s+(` + codePat + `)`)
	// complFurtherDesigRe matches "in further ITA courses, including language
	// courses": the language courses are ITA courses too.
	complFurtherDesigRe = regexp.MustCompile(`(?i)\bin\s+further\s+((?-i:[A-Z]{3}))\s+courses(?:,\s+including\s+language\s+courses$)?`)
	// complOnlyOneFromRe matches a cap at the end of a "/" list: "/ (only one
	// from ESS261H1/ GGR203H1)".
	complOnlyOneFromRe = regexp.MustCompile(`(?i)/?\s*\(\s*only\s+one\s+from\s+((?:` + codePat + `\s*/\s*)*` + codePat + `)\s*\)$`)
	// complSelectedThenPairRe matches a pair after a "/" pool, which ends the
	// pool (ASMAJ0525): "credit selected from FRE210H1/ JFG388H1, ( FSL320H1,
	// FSL322H1)/ FSL321Y1". complMeetsAsWellRe matches "The Physics course
	// JPH441H1 meets this requirement as well as any of the following courses:".
	complSelectedThenPairRe = regexp.MustCompile(`(?i)(\bcredits?\s+selected\s+from\s+(?:` + codePat + `\s*/\s*)+` + codePat + `),\s*(\(\s*` + codePat + `\s*,\s*` + codePat + `\s*\)\s*/\s*` + codePat + `\.?)$`)
	complMeetsAsWellRe      = regexp.MustCompile(`(?i)^(?:[\w\s&,]+\s+requirement:\s*)?the\s+\w+\s+course\s+(` + codePat + `)\s+meets\s+this\s+requirement\s+as\s+well\s+as\s+any\s+of\s+the\s+following\s+courses:\s*`)
)

// rewriteOneOffs rewrites wordings one or two items use into the grammar's.
func rewriteOneOffs(text string) string {
	text = complFromOneOfRe.ReplaceAllString(text, "$1 $2")
	text = complSelectedThenPairRe.ReplaceAllString(text, "$1; $2")
	if m := complGroupFloorRe.FindStringSubmatch(text); m != nil {
		floor := strings.TrimSpace(m[3])
		if f := complWhichFloorRe.FindStringSubmatch(floor); f != nil {
			floor = "including at least " + f[1] + " credit " + f[2]
		}
		codes := codeRe.FindAllString(m[4], -1)
		// the group's level is the codes' own, which the grammar's pool says
		low := 0
		if m[2] != "" {
			low = int(m[2][0]-'0') * 100
		}
		if strings.HasPrefix(strings.ToLower(floor), "including") && !slices.ContainsFunc(codes, func(c string) bool { return courseLevel(c) < low }) {
			text = m[1] + " from " + strings.Join(codes, ", ") + ", " + floor
		}
	}
	text = complMeetsAsWellRe.ReplaceAllString(text, "$1/ ")
	text = complDesigButNotRe.ReplaceAllString(text, "in $1 courses, excluding $2, $3")
	text = complFurtherDesigRe.ReplaceAllString(text, "in $1 courses")
	if m := complOnlyOneFromRe.FindStringSubmatchIndex(text); m != nil {
		// one course of the bracket, when each is a half course: at most 0.5 credit
		codes := text[m[2]:m[3]]
		if !slices.ContainsFunc(codeRe.FindAllString(codes, -1), func(c string) bool { return courseWeight(c) != 0.5 }) {
			text = strings.TrimSpace(text[:m[0]]) + "/ " + codes + "; up to 0.5 credit from " + codes
		}
	}
	text = complListFloorRe.ReplaceAllString(text, "$1 from $4, including at least $2 credits from $3 courses")
	return complAllRequiredRe.ReplaceAllString(text, "")
}

// restatedConstraints drops a program constraint credits{w, courses} that an
// item already requires as or(those courses), each of weight w, joining its
// sentence to the item's raw.
func (a *compAssembler) restatedConstraints() {
	a.program = slices.DeleteFunc(a.program, func(r *compRow) bool {
		n := r.r.Req
		if r.r.Kind != types.RequirementConstraint || n == nil || len(r.children) > 0 {
			return false
		}
		if n.Type == types.NodeOr && len(n.Items) == 2 && n.Items[1].Type == types.NodeText {
			n = n.Items[0]
		}
		if !isPool(n) || n.Courses == nil || n.Designators != nil || n.Levels != nil || n.Lists != nil || n.Amount <= 0 {
			return false
		}
		want := slices.Sorted(slices.Values(n.Courses))
		requires := func(x *types.ReqNode) bool {
			if x.Type != types.NodeOr {
				return false
			}
			var codes []string
			for _, it := range x.Items {
				if it.Type != types.NodeCourse || courseWeight(it.Code) != n.Amount {
					return false
				}
				codes = append(codes, it.Code)
			}
			slices.Sort(codes)
			return slices.Equal(codes, want)
		}
		for _, q := range a.items {
			body := q.r.Req
			if q.r.Kind != types.RequirementAllocation || body == nil {
				continue
			}
			if requires(body) || body.Type == types.NodeAnd && slices.ContainsFunc(body.Items, requires) {
				addRaw(q, r.r.Raw)
				return true
			}
		}
		return false
	})
}

// complLanguageBanRe matches "Language courses cannot be used to fulfil the
// requirements of the Minor program".
var complLanguageBanRe = regexp.MustCompile(`(?i)^language\s+courses\s+cannot\s+be\s+used\s+to\s+(?:fulfil|fulfill|satisfy)\s+the\s+requirements\s+of\s+the\s+(?:minor|major|specialist)\s+program$`)

// languageBanNote joins a complLanguageBanRe row to the total's raw, or makes
// it a note, when namedPoolsFill: the courses counted are the ones the calendar
// lists (ASMIN1058, whose lists hold no language course).
func (a *compAssembler) languageBanNote() {
	if !a.namedPoolsFill() {
		return
	}
	a.program = slices.DeleteFunc(a.program, func(r *compRow) bool {
		if r.r.Req == nil || r.r.Req.Type != types.NodeText || len(r.children) > 0 || !complLanguageBanRe.MatchString(a.clean(r.r.Req.Text)) {
			return false
		}
		if a.total != nil {
			addRaw(a.total, r.r.Req.Text) // restated: the total's items name every course
		} else {
			a.notes = append(a.notes, cleanNote(r.r.Req.Text))
		}
		return true
	})
}

// namedPoolsFill reports whether every item is courses and pools of named
// courses or lists and the items fill the total: no course the program
// doesn't name counts.
func (a *compAssembler) namedPoolsFill() bool {
	named := a.total == nil || a.total.r.Req != nil && a.total.r.Req.Type == types.NodeCredits
	sum := 0.0
	for _, q := range a.items {
		if q.r.Kind != types.RequirementAllocation || q.r.Req == nil {
			continue
		}
		strict := q.r.Req // an ambiguity's strict reading
		if strict.Type == types.NodeOr && strict.Items[0].Type != types.NodeText {
			strict = strict.Items[0]
		}
		c, ok := nodeCredits(strict)
		sum += c
		walkReq(q.r.Req, func(x *types.ReqNode) {
			// an ambiguity's text sits beside its lenient pool, which is named
			named = named && ok && (x.Type != types.NodeText || x.Kind == types.TextAmbiguity) && (x.Type != types.NodeCredits || x.Designators == nil && x.Levels == nil && (x.Courses != nil || x.Lists != nil))
		})
	}
	return named && (a.total == nil || sum >= a.total.r.Req.Amount-1e-9)
}

// complThroughDanielsRe matches "taken through the Daniels Faculty".
var complThroughDanielsRe = regexp.MustCompile(`(?i)^taken\s+through\s+the\s+Daniels\s+Faculty$`)

// throughDaniels drops complThroughDanielsRe from a body beside a VIS pool:
// the Daniels Faculty offers VIS courses, so it restates the designator
// (ASMIN0660). The row's raw keeps it.
func throughDaniels(r *compRow) {
	n := r.r.Req
	if n == nil || n.Type != types.NodeAnd {
		return
	}
	vis := slices.ContainsFunc(n.Items, func(x *types.ReqNode) bool {
		return x.Type == types.NodeCredits && slices.Equal(x.Designators, []string{"VIS"})
	})
	for i, it := range n.Items {
		if vis && it.Type == types.NodeText && complThroughDanielsRe.MatchString(complClean(it.Text)) {
			r.r.Req = andNode(slices.Delete(slices.Clone(n.Items), i, i+1)...)
			return
		}
	}
}

// complPossibilitiesRe matches a note listing examples of an open pool:
// "Possibilities include but are not limited to: ( MUN100H1 and MUN110H1) or
// ... or ECO105Y1".
var complPossibilitiesRe = regexp.MustCompile(`(?i)^possibilities\s+include\s+but\s+are\s+not\s+limited\s+to:\s*(.+)$`)

// possibilities reads a row of text with a complPossibilitiesRe note as
// or(the examples, text{content}, routes): the examples don't exhaust it.
func possibilities(r *compRow) {
	n := r.r.Req
	if r.r.Kind != types.RequirementAllocation || n == nil {
		return
	}
	items := []*types.ReqNode{n}
	if n.Type == types.NodeOr {
		items = n.Items
	}
	if items[0].Type != types.NodeText || items[0].Kind != types.TextOther || slices.ContainsFunc(items, func(x *types.ReqNode) bool { return x.Type != types.NodeText }) {
		return
	}
	for i, note := range r.r.Notes {
		m := complPossibilitiesRe.FindStringSubmatch(note)
		if m == nil {
			continue
		}
		ex, _ := parseCompletionText(strings.TrimSuffix(m[1], "."))
		if ex == nil || ex.Type != types.NodeOr || hasText(ex) {
			return
		}
		items[0].Kind = types.TextContent
		r.r.Req = orNode(append(slices.Clone(ex.Items), items...)...)
		if r.r.Notes = slices.Delete(r.r.Notes, i, i+1); len(r.r.Notes) == 0 {
			r.r.Notes = nil
		}
		return
	}
}

var (
	// complDistributionRe matches a lead summing up the items below it: "The
	// distribution of the 7.0 credits is as follows:".
	complDistributionRe = regexp.MustCompile(`(?i)^the\s+distribution\s+of\s+the\s+\d+(?:\.\d+)?\s+credits\s+is\s+as\s+follows:?$`)
	// complSingleLanguageRe matches the rule such a summary adds: "A two-year
	// progressive sequence of courses in a single language".
	complSingleLanguageRe = regexp.MustCompile(`(?i)\bsequence\s+of\s+courses\s+in\s+a\s+single\s+language\b`)
)

// distributionSummary makes a complDistributionRe item and its bullets notes,
// since the items below say each again (ASMAJ1626). The one-language rule
// they add stays a program text constraint.
func (a *compAssembler) distributionSummary() {
	a.items = slices.DeleteFunc(a.items, func(r *compRow) bool {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || !complDistributionRe.MatchString(a.clean(n.Text)) ||
			slices.ContainsFunc(r.children, func(c *compRow) bool { return c.r.Req == nil || c.r.Req.Type != types.NodeText || len(c.children) > 0 }) {
			return false
		}
		sents := slices.Clone(r.r.Notes)
		for _, c := range r.children {
			sents = append(sents, c.r.Req.Text)
		}
		for _, s := range sents {
			if complSingleLanguageRe.MatchString(s) {
				a.program = append(a.program, &compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Req: textNode(types.TextOther, s), Raw: s}})
			} else {
				a.notes = append(a.notes, cleanNote(s))
			}
		}
		return true
	})
}

// complApprovedSequencesRe matches "Approved sequences of courses include
// CLT141Y1, CLT251H1, and CLT252H1; EAS100Y1 and EAS200Y1".
var complApprovedSequencesRe = regexp.MustCompile(`(?i)^approved\s+sequences\s+of\s+courses\s+include\s+(.+)$`)

// approvedSequences reads and(text, text{complApprovedSequencesRe}) in an or:
// each sequence is a strict alternative, and the item's own sentence, which
// "include" doesn't exhaust, is content beside them.
func approvedSequences(r *compRow) {
	n := r.r.Req
	if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr {
		return
	}
	for i, it := range n.Items {
		if it.Type != types.NodeAnd || len(it.Items) != 2 || it.Items[0].Type != types.NodeText || it.Items[1].Type != types.NodeText {
			continue
		}
		m := complApprovedSequencesRe.FindStringSubmatch(strings.TrimSuffix(complClean(it.Items[1].Text), "."))
		if m == nil {
			continue
		}
		var seqs []*types.ReqNode
		for _, part := range strings.Split(m[1], ";") {
			s, _ := parseCompletionText(strings.TrimSpace(part))
			if s == nil || hasText(s) || s.Type != types.NodeAnd {
				return
			}
			seqs = append(seqs, s)
		}
		own := it.Items[0]
		own.Kind = types.TextContent
		items := append(append(append([]*types.ReqNode{}, n.Items[:i]...), seqs...), own)
		r.r.Req = orNode(append(items, n.Items[i+1:]...)...)
		return
	}
}

// approvalOverAll reads "CHM395Y1/ ... and/or CHM397H0/ CHM398H0 ... with
// prior approval" by the ambiguity policy: the approval over every course is
// the strict reading, over the last group only the lenient one (Review).
func approvalOverAll(r *compRow) {
	n := r.r.Req
	if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr || len(n.Items) < 2 ||
		!strings.Contains(rawHead(r.r), " and/or ") {
		return
	}
	first, last := n.Items[:len(n.Items)-1], n.Items[len(n.Items)-1]
	if slices.ContainsFunc(first, func(x *types.ReqNode) bool { return x.Type != types.NodeCourse }) ||
		last.Type != types.NodeAnd || len(last.Items) != 2 || last.Items[0].Type != types.NodeOr || last.Items[1].Type != types.NodeText ||
		last.Items[1].Kind != types.TextPermission || slices.ContainsFunc(last.Items[0].Items, func(x *types.ReqNode) bool { return x.Type != types.NodeCourse }) {
		return
	}
	all := orNode(append(slices.Clone(first), last.Items[0].Items...)...)
	r.r.Req = orNode(andNode(all, last.Items[1]),
		andNode(orNode(slices.Clone(first)...), textNode(types.TextAmbiguity, "the approval may be needed for the courses before \"and/or\" too")))
}

// complTrailingCodeRe matches a pool ending at a ";" before one more
// course: "1.0 credit from FOR300H1, ..., FOR310H1; EEB321H1".
var complTrailingCodeRe = regexp.MustCompile(`\d+(?:\.\d+)?\s+credits?\s+from\s+[^;]+;\s*(` + codePat + `)\.?$`)

// complSemicolonPoolRe matches a pool whose ";" separates groups of its
// courses: "1.0 credit from MSE101H1; ECO101H1, ECO102H1".
var complSemicolonPoolRe = regexp.MustCompile(`\d+(?:\.\d+)?\s+credits?\s+from\s+[^;]+;`)

// trailingCode reads complTrailingCodeRe by the ambiguity policy: the last
// course as its own item is strict, the pool taking it is Review. Where another
// item's pool has ";" between groups, the pool reading stands alone
// (coordinator ruling on ASMAJ1098, ASSPE1102, ASMAJ1100, ASSPE1100).
func (a *compAssembler) trailingCode(r *compRow) {
	n := r.r.Req
	m := complTrailingCodeRe.FindStringSubmatch(stripEmphasis(rawHead(r.r)))
	if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeAnd || m == nil {
		return
	}
	if slices.ContainsFunc(a.items, func(o *compRow) bool {
		return o != r && complSemicolonPoolRe.MatchString(stripEmphasis(rawHead(o.r)))
	}) {
		return
	}
	for i, it := range n.Items {
		if !isPool(it) || it.Designators != nil || it.Levels != nil || len(it.Courses) < 3 || it.Courses[len(it.Courses)-1] != m[1] {
			continue
		}
		pool := cloneNode(it)
		pool.Courses = pool.Courses[:len(pool.Courses)-1]
		strict := slices.Clone(n.Items)
		strict[i] = pool
		strict = append(strict, &types.ReqNode{Type: types.NodeCourse, Code: m[1]})
		r.r.Req = orNode(andNode(strict...), andNode(n, textNode(types.TextAmbiguity, m[1]+" may be one of the pool's courses rather than a course of its own")))
		return
	}
}

// complCapstoneSubstituteRe matches "May substitute a 0.5 credit in this
// requirement from the list of Capstone courses (all 400-level SDS and JNS
// courses)". The groups are the amount, the level, the designators.
var complCapstoneSubstituteRe = regexp.MustCompile(`(?i)^may\s+substitute\s+an?\s+(\d+(?:\.\d+)?)\s+credits?\s+in\s+this\s+requirement\s+from\s+the\s+list\s+of\s+[\w\s]+\s+courses\s+\(all\s+([1-4])00-level\s+((?-i:[A-Z]{3})(?:\s*(?:,|and)\s*(?-i:[A-Z]{3}))*)\s+courses\)$`)

// capstoneSubstitute reads or(and(parts), text{complCapstoneSubstituteRe}),
// each part an amount from one list, as one part less and a capstone. A
// capstone can be on a part's list (ASMIN1240), so each case is its own
// alternative. The lists must hold only courses, share none, and have
// half-course capstones.
func (a *compAssembler) capstoneSubstitute(r *compRow, byName map[string]*types.CourseList) {
	n := r.r.Req
	if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr || len(n.Items) != 2 ||
		n.Items[0].Type != types.NodeAnd || n.Items[1].Type != types.NodeText {
		return
	}
	m := complCapstoneSubstituteRe.FindStringSubmatch(strings.TrimSuffix(complClean(n.Items[1].Text), "."))
	if m == nil {
		return
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	parts := n.Items[0].Items
	if len(parts) < 2 || slices.ContainsFunc(parts, func(x *types.ReqNode) bool { return !onlyLists(x) || x.Amount != amount }) {
		return
	}
	desigs := desigRe.FindAllString(m[3], -1)
	level := int(m[2][0]-'0') * 100
	seen := map[string]bool{}
	var capstones [][]string
	var all []string
	for _, p := range parts {
		codes, ok := a.listCodes(p.Lists, byName)
		if !ok {
			return
		}
		var cs []string
		for _, c := range codes {
			if seen[c] {
				return
			}
			seen[c] = true
			if slices.Contains(desigs, c[:3]) && levelMatches([]int{level}, c) {
				if c[6] != 'H' {
					return
				}
				cs = append(cs, c)
			}
		}
		capstones = append(capstones, cs)
		all = append(all, cs...)
	}
	// choose returns k of the parts other than skip (-1: none)
	choose := func(k, skip int) *types.ReqNode {
		var xs []*types.ReqNode
		for i, p := range parts {
			if i != skip {
				xs = append(xs, cloneNode(p))
			}
		}
		switch k {
		case len(xs):
			return andNode(xs...)
		case 1:
			return orNode(xs...)
		}
		return &types.ReqNode{Type: types.NodeChoose, Count: k, Items: xs}
	}
	alts := []*types.ReqNode{n.Items[0], andNode(choose(len(parts)-1, -1), &types.ReqNode{Type: types.NodeCredits, Amount: amount, Designators: desigs, Levels: []int{level}, Exclude: all})}
	for i, p := range parts {
		if capstones[i] == nil {
			continue
		}
		two := cloneNode(p)
		two.Amount = 2 * amount
		in := []*types.ReqNode{two, {Type: types.NodeCredits, Amount: amount, Courses: capstones[i]}}
		if len(parts) > 2 {
			in = append(in, choose(len(parts)-2, i))
		}
		alts = append(alts, andNode(in...))
	}
	r.r.Req = orNode(alts...)
}

// noteFloorAfterLine reads a floor noted after the last item of a program
// with no total by the ambiguity policy: on that item is strict (Met), on the
// whole program lenient (Review).
func (a *compAssembler) noteFloorAfterLine() {
	if a.total != nil || len(a.program) != 1 {
		return
	}
	last := a.program[0]
	var above *compRow
	for _, r := range a.items {
		if r.r.Kind == types.RequirementAllocation {
			above = r
		}
	}
	n := last.r.Req
	if above == nil || last.r.Kind != types.RequirementConstraint || last.r.Parent != 0 || len(last.children) > 0 || n == nil ||
		n.Type != types.NodeCredits || n.Amount == 0 || n.Levels == nil || n.Designators != nil || n.Courses != nil || n.Lists != nil ||
		n.Requirements != nil || !strings.Contains(a.raw, above.r.Raw+"\nNote: "+last.r.Raw) {
		return
	}
	ref := cloneNode(n)
	ref.Requirements = []int{}
	a.reqRefs = append(a.reqRefs, compReqRef{node: ref, rows: []*compRow{above}, scope: true})
	last.r.Req = orNode(ref, andNode(n, textNode(types.TextAmbiguity, last.r.Raw)))
}

// complFloorIfCompletedRe matches "at least 1.0 must be 300+ series if 1.0
// credit is completed in #7 above" (ASSPE1006). The groups are the amount
// the condition names and the item.
var complFloorIfCompletedRe = regexp.MustCompile(`(?i)^at\s+least\s+\d+(?:\.\d+)?\s+must\s+be\s+[1-4]00\+\s+series\s+if\s+(\d+(?:\.\d+)?)\s+credits?\s+is\s+completed\s+in\s+#(\d+)\s+above$`)

// complRangeLeadRe matches an item's range: "1.0 to 1.5 credits". The group
// is the upper amount.
var complRangeLeadRe = regexp.MustCompile(`(?i)^(?:\d+\.\s+)?\d+(?:\.\d+)?\s+to\s+(\d+(?:\.\d+)?)\s+credits?\b`)

// floorIfCompleted reads a complFloorIfCompletedRe text constraint as
// or(credits{upper, requirements: [the item]}, text): the floor is on credits
// no row counts, but an item claiming its upper amount makes the condition false.
func (a *compAssembler) floorIfCompleted() {
	all := append(append(append([]*compRow{}, a.floors...), a.items...), a.program...)
	walkRows(all, func(r *compRow) {
		n := r.r.Req
		if r.r.Kind != types.RequirementConstraint || n == nil || n.Type != types.NodeText {
			return
		}
		m := complFloorIfCompletedRe.FindStringSubmatch(strings.TrimSpace(complClean(n.Text)))
		if m == nil {
			return
		}
		var item *compRow
		walkRows(a.items, func(x *compRow) {
			if x.r.Kind == types.RequirementAllocation && x.r.Label == m[2] {
				item = x
			}
		})
		if item == nil {
			return
		}
		rm := complRangeLeadRe.FindStringSubmatch(item.r.Raw)
		cond, _ := strconv.ParseFloat(m[1], 64)
		if rm == nil {
			return
		}
		upper, _ := strconv.ParseFloat(rm[1], 64)
		if upper <= cond {
			return
		}
		ref := &types.ReqNode{Type: types.NodeCredits, Amount: upper, Requirements: []int{}}
		a.reqRefs = append(a.reqRefs, compReqRef{node: ref, rows: []*compRow{item}, scope: true})
		r.r.Req = orNode(ref, n)
	})
}

// complNamedLanguagesRe matches a pool's languages: "in Modern Stream
// languages: Arabic, Hebrew, Persian, Turkish" (ASMAJ2669).
var complNamedLanguagesRe = regexp.MustCompile(`\blanguages:\s*([A-Z][a-z]+(?:,\s*(?:and\s+)?[A-Z][a-z]+)*)\.?$`)

// languagesNamed gives and(credits{amount}, text{complNamedLanguagesRe}) in
// r's body the languages' designators; the text stays, since which of their
// courses count is the course's language.
func languagesNamed(r *compRow) {
	walkReq(r.r.Req, func(n *types.ReqNode) {
		if n.Type != types.NodeAnd || len(n.Items) != 2 || n.Items[0].Type != types.NodeCredits || n.Items[1].Type != types.NodeText ||
			n.Items[1].Kind != types.TextContent {
			return
		}
		pool := n.Items[0]
		m := complNamedLanguagesRe.FindStringSubmatch(n.Items[1].Text)
		if m == nil || pool.Designators != nil || pool.Courses != nil || pool.Lists != nil || pool.Levels != nil || pool.Requirements != nil {
			return
		}
		var desigs []string
		for _, l := range regexp.MustCompile(`[A-Z][a-z]+`).FindAllString(m[1], -1) {
			d := languageDesignators[strings.ToLower(l)]
			if d == nil {
				return
			}
			desigs = append(desigs, d...)
		}
		pool.Designators = dedupe(desigs)
	})
}

// complLevelCodedRe matches a route by level: "any 100 or 200 level coded
// course from Munk ... with prior permission of the Director" (ASMAJ2660).
var complLevelCodedRe = regexp.MustCompile(`(?i)^any\s+([1-4]00(?:\s+or\s+[1-4]00)*)\s+level\s+coded\s+course\s+from\b.*\bwith\s+prior\s+permission\b`)

// levelCodedRoute bounds r's complLevelCodedRe route by its levels: the
// unit's courses carry designators the text doesn't list, so the levels are
// the bound.
func levelCodedRoute(r *compRow) {
	n := r.r.Req
	if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr || len(n.Items) != 2 ||
		n.Items[0].Type != types.NodeCredits || n.Items[0].Amount == 0 || n.Items[1].Type != types.NodeText || n.Items[1].Kind != types.TextPermission {
		return
	}
	m := complLevelCodedRe.FindStringSubmatch(complClean(n.Items[1].Text))
	if m == nil {
		return
	}
	var levels []int
	for _, l := range regexp.MustCompile(`[1-4]00`).FindAllString(m[1], -1) {
		levels = append(levels, int(l[0]-'0')*100)
	}
	n.Items[1] = andNode(&types.ReqNode{Type: types.NodeCredits, Amount: n.Items[0].Amount, Levels: levels}, n.Items[1])
}

// approvedOnLists reads or(pool, and(pool, text{complSuitableCodesRe})) on
// lists that already hold the route's courses (ASSPE1252's Group C): they
// count only with the approval, so the strict pool excludes them.
func approvedOnLists(r *compRow) {
	n := r.r.Req
	if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr || len(n.Items) != 2 {
		return
	}
	strict, alt := n.Items[0], n.Items[1]
	if strict.Type != types.NodeCredits || strict.Lists == nil || alt.Type != types.NodeAnd || len(alt.Items) != 2 ||
		!sameNode(alt.Items[0], strict) || alt.Items[1].Type != types.NodeText {
		return
	}
	m := complSuitableCodesRe.FindStringSubmatch(complClean(alt.Items[1].Text))
	if m == nil {
		return
	}
	strict.Exclude = dedupe(append(slices.Clone(strict.Exclude), codeRe.FindAllString(m[3], -1)...))
}

// complGraduateShareRe matches a share of graduate courses at a level:
// "Students may be permitted to take Math graduate courses. A 0.5 credit of
// these courses may count toward ... at the 400-level" (ASSPE1165, ASSPE1580).
var complGraduateShareRe = regexp.MustCompile(`(?i)^students\s+may\s+be\s+permitted\s+to\s+take\s+(\w+)\s+graduate\s+courses\.\s+an?\s+(\d+(?:\.\d+)?)\s+credits?\s+of\s+these\s+courses\s+may\s+count\s+toward\b.*\bat\s+the\s+([1-4])00-level\b`)

// graduateAtLevel gives the program's graduate-course route to every other
// pool or floor at its level, as or(row, and(row less the share, text)):
// Review, since only the approval tells.
func (a *compAssembler) graduateAtLevel() {
	all := append(append(append([]*compRow{}, a.floors...), a.items...), a.program...)
	var route *types.ReqNode
	var share float64
	var level int
	walkRows(all, func(r *compRow) {
		walkReq(r.r.Req, func(n *types.ReqNode) {
			if route != nil || n.Type != types.NodeText {
				return
			}
			if m := complGraduateShareRe.FindStringSubmatch(complClean(n.Text)); m != nil {
				route = n
				share, _ = strconv.ParseFloat(m[2], 64)
				level = int(m[3][0]-'0') * 100
			}
		})
	})
	if route == nil {
		return
	}
	walkRows(all, func(r *compRow) {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation && r.r.Kind != types.RequirementConstraint || n == nil || n.Type != types.NodeCredits ||
			n.Amount <= share || !slices.Contains(n.Levels, level) || n.Courses != nil || n.Lists != nil || n.Requirements != nil || n.MaxAmount != nil {
			return
		}
		less := cloneNode(n)
		less.Amount = n.Amount - share
		text := cloneNode(route)
		text.Kind = types.TextPermission
		r.r.Req = orNode(n, andNode(less, text))
	})
}

// complAsNecessaryRe matches an item holding the rest of the total: "(as
// necessary to total 4.0 credits in minor)" (ASMIN1333).
var complAsNecessaryRe = regexp.MustCompile(`(?i)\(as\s+necessary\s+to\s+total\s+(\d+(?:\.\d+)?)\s+credits\b`)

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
	byName := map[string]*types.CourseList{}
	for _, l := range lists {
		byName[l.Name] = l
	}
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

// complOneDisciplineRe matches "1.0 credit in ONE other discipline offered
// within the Faculty of Arts & Science – African Studies, ..., and other
// departments as approved by the program coordinator".
var complOneDisciplineRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+(credits?)\s+in\s+one\s+other\s+discipline\s+offered\s+within\s+the\s+Faculty\s+of\s+Arts\s+(?:&|and)\s+Science\s*[–—-]\s*(.+?),?\s+and\s+other\s+departments\s+as\s+approved\s+by\s+the\s+program\s+coordinator$`)

// oneDiscipline reads complOneDisciplineRe (ASMAJ1023, ASSPE1023) as an or of
// the amount from each discipline's designator, content text for those with
// none, and a permission route. An unknown name leaves the row.
func oneDiscipline(r *compRow) {
	n := r.r.Req
	if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || n.Kind != types.TextOther || len(r.children) > 0 {
		return
	}
	m := complOneDisciplineRe.FindStringSubmatch(strings.TrimSuffix(complClean(n.Text), "."))
	if m == nil {
		return
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	var alts []*types.ReqNode
	var content []string
	for _, name := range strings.Split(m[3], ",") {
		name = strings.TrimSpace(name)
		if d, ok := disciplineDesignators[strings.ToLower(name)]; ok {
			alts = append(alts, &types.ReqNode{Type: types.NodeCredits, Amount: amount, Designators: []string{d}})
		} else if contentDisciplines[strings.ToLower(name)] {
			content = append(content, name)
		} else {
			return
		}
	}
	any := func() *types.ReqNode { return &types.ReqNode{Type: types.NodeCredits, Amount: amount} }
	if content != nil {
		alts = append(alts, andNode(any(), textNode(types.TextContent, m[1]+" "+m[2]+" in ONE of "+strings.Join(content, ", "))))
	}
	alts = append(alts, andNode(any(), textNode(types.TextPermission, "other departments as approved by the program coordinator")))
	r.r.Req = orNode(alts...)
}

// complSeparatorCapRe matches a "/" run that pairCaps capped but may be the
// list's separator: "at least 1.0 from PSY320H1/ PSY321H1/ PSY322H1,
// PSY324H1/" (ASSPE2270), "1.0 credit from HMB430H1/ ...; JPM400Y1;" (ASMAJ0482).
var complSeparatorCapRe = regexp.MustCompile(`(?:\bat\s+least\s+1\.0\s+from|\b1\.0\s+credit\s+from)\s+((?:` + codePat + `\s*/\s*){2}` + codePat + `)\s*(?:,\s*PSY324H1\s*/|;\s*JPM400Y1\s*;)`)

// separatorCaps reads complSeparatorCapRe by the ambiguity policy: the cap is
// strict (Met), the group as plain courses lenient (Review). The limit goes,
// and its pool P becomes or(and(P, P less the cap from its other courses),
// and(P, text{ambiguity})).
func (a *compAssembler) separatorCaps(roots []*compRow, lists []*types.CourseList) {
	byName := map[string]*types.CourseList{}
	for _, l := range lists {
		byName[l.Name] = l
	}
	for _, u := range a.rowUnions {
		byName[u.list.Name] = u.list // named after numbering (nameRowUnions)
	}
	var walk func(rs []*compRow)
	walk = func(rs []*compRow) {
		for _, r := range rs {
			walk(r.children)
			for _, k := range slices.Clone(r.children) {
				l := k.r.Req
				if k.r.Kind != types.RequirementLimit || l == nil || !isCourseCap(l) || len(l.Courses) < 2 {
					continue
				}
				m := complSeparatorCapRe.FindStringSubmatch(k.r.Raw)
				if m == nil || !slices.Equal(codeRe.FindAllString(m[1], -1), l.Courses) {
					continue
				}
				n, ok := a.separatorPool(r.r.Req, l.Courses, *l.MaxAmount, byName)
				if !ok {
					continue
				}
				r.r.Req = n
				r.children = slices.DeleteFunc(r.children, func(x *compRow) bool { return x == k })
			}
		}
	}
	walk(roots)
}

// separatorPool returns n with the pool that names every course of group,
// above the cap, read both ways; ok is false when n has no such pool.
func (a *compAssembler) separatorPool(n *types.ReqNode, group []string, capAmount float64, byName map[string]*types.CourseList) (*types.ReqNode, bool) {
	if n == nil {
		return nil, false
	}
	if n.Type == types.NodeAnd || n.Type == types.NodeOr {
		for i, it := range n.Items {
			if x, ok := a.separatorPool(it, group, capAmount, byName); ok {
				n.Items[i] = x
				return n, true
			}
		}
		return n, false
	}
	if n.Type != types.NodeCredits || n.MaxAmount != nil || n.Amount <= capAmount+1e-9 || n.Exclude != nil || n.Items != nil {
		return n, false
	}
	// its lists may hold filters too: "PSL400-series courses"
	codes := slices.Clone(n.Courses)
	for _, name := range n.Lists {
		l := byName[name]
		if l == nil {
			return n, false
		}
		for _, it := range l.Items {
			if it.Type == types.NodeCredits && it.Exclude == nil {
				codes = append(codes, it.Courses...)
			}
		}
	}
	if slices.ContainsFunc(group, func(c string) bool { return !slices.Contains(codes, c) }) {
		return n, false
	}
	rest := cloneNode(n)
	rest.Amount = n.Amount - capAmount
	if n.Lists == nil {
		rest.Courses = slices.DeleteFunc(rest.Courses, func(c string) bool { return slices.Contains(group, c) })
	} else {
		rest.Exclude = slices.Clone(group)
	}
	text := fmt.Sprintf("the \"/\" in %s may separate the pool's courses rather than join alternatives, so more than one of them may count", strings.Join(group, "/ "))
	return orNode(andNode(n, rest), andNode(cloneNode(n), textNode(types.TextAmbiguity, text))), true
}
