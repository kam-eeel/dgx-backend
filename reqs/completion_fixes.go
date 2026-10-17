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

// One-off fixes (Task 10D): wordings one or two programs use, read where no
// wider rule covers them.

// areaPools reads one area's line under a lead over a sub-list per area
// (areaChoice) as the pools of amount it allows: one pool, or one per set
// when the line is two sets, one or the other ("[( SLA107Y1/ SLA117H1,
// SLA118H1); SLA214H1; ...]; /[ SLA109Y1, SLA209H1, SLA213H1]"). False when a
// set isn't a pool of courses that can supply the amount.
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

// areaCorrespondence ties the two items areaChoice reads one by one ("2. 2.0
// credits of language coursework in one language area chosen from the list
// below:" over the areas' language lines, "3. 2.0 credits from culture
// courses corresponding to the student’s chosen language area:" over their
// culture lines): each area is an option, a group of its two parts, and a
// choice row picks one of them, as areaOptions does for two areas. The
// areas pair by name, or by what one says it corresponds to ("Yiddish
// (corresponds to the Jewish Cultures in Eastern Europe area)"). The items'
// text alternatives (an approval, a substitution) stay on each of their
// parts. Both items must be read as pools already, and every area must
// pair; else nothing changes. After the routes are placed
// (placeProgramRoutes), whose alternatives it carries.
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
		// The body as areaChoice and the routes left it: or(the areas' pools,
		// [the correspondence's and], text alternatives).
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
	// complGatewayRe: an amount from courses grouped under the disciplines
	// that offer them: "1.0 credit from the gateway courses in American
	// Studies ( AMS100H1), English ( ENG250H1 and ENG270H1), ..., or Cinema
	// Studies ( CIN270Y1)". The groups: the amount, the disciplines.
	complGatewayRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+from\s+the\s+[\w-]+\s+courses\s+in\s+(.+)$`)
	// complGatewayGroupRe: one discipline and its courses: "English (
	// ENG250H1 and ENG270H1)", then what separates it from the next.
	complGatewayGroupRe = regexp.MustCompile(`^\s*[A-Z][A-Za-z]*(?:\s+[A-Z][A-Za-z]*)*\s*\(\s*(` + codePat + `(?:\s+and\s+` + codePat + `)*)\s*\)\s*(?:,\s*(?:or\s+)?|,?\s+or\s+|$)`)
	// complAdditionalUnionRe: Req 1's own pool as designators or a program's
	// lists: "Additional AMS/USA courses or courses from the American Studies
	// Program disciplinary/thematic clusters to a total of 7.0 credits,
	// including requirement #4 above". The groups: the designators, the word
	// the lists are named by, the total.
	complAdditionalUnionRe = regexp.MustCompile(`(?i)^additional\s+((?-i:[A-Z]{3})(?:/(?-i:[A-Z]{3}))*)\s+courses\s+or\s+courses\s+from\s+the\s+[\w\s/-]*?\b(clusters|groups)\s+to\s+a\s+total\s+of\s+(\d+(?:\.\d+)?)\s+credits?(?:,\s*including\s+requirement\s+#?\d+\s+above)?$`)
)

// oneOffRow reads an allocation of text (text: its text, the body or the
// first item of or(text, routes)) by the readers below, in listRefRow, with
// the lists known. ok: it read the row; rows: what stands for it.
func (a *compAssembler) oneOffRow(row *compRow, text *types.ReqNode, ls compListRefs) (rows []*compRow, ok bool) {
	var alts []*types.ReqNode
	if n := row.r.Req; n != text {
		alts = n.Items[1:]
	}
	if a.additionalUnion(row, text.Text, alts, ls) {
		return nil, true // Req 1's own pool
	}
	if a.residueAltRow(row, text, ls) {
		return []*compRow{row}, true // completion_residue.go
	}
	if text == row.r.Req && a.gatewayGroups(row) {
		return []*compRow{row}, true
	}
	if a.moreOneOffRow(row, text, alts, ls) {
		return []*compRow{row}, true // completion_oneoffs.go
	}
	if a.unitRows(row, text, ls) {
		return []*compRow{row}, true // completion_units.go
	}
	return a.residueRow(row, text, ls) // completion_residue.go
}

// gatewayGroups reads an item whose courses are grouped by discipline
// (complGatewayRe) as the or of the groups that make the amount, each the
// and of its courses ("( ENG250H1 and ENG270H1)"). Whether one group must
// make the amount or courses of several may is the text's ambiguity: the
// lenient reading, the amount from all the courses, is Review beside them,
// and(credits, the sentence). A group lighter than the amount ("American
// Studies ( AMS100H1)" for 1.0 credit) is only on the lenient side. It
// reports whether it read the row, a row of text alone.
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

// additionalUnion reads an item complAdditionalUnionRe matches as Req 1's
// own pool, as additionalLists does for lists by name: a list of the row's
// own (addUnion) holding the designators and every list of the program
// named by the word ("clusters": "Cluster 1", "Cluster 2", ...). The item is
// no row of its own; its raw joins Req 1's, and its routes (alts) become Req
// 1's. It reports false, reading nothing, when the total isn't the
// program's, Req 1 has a pool already, or no list is named by the word.
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

// oneOffs runs the one-off readings that need the routes placed and the
// lists read (finish).
func (a *compAssembler) oneOffs() {
	a.areaCorrespondence()
	a.languageCourses()
	a.exclusionExceptions()
	a.designatorRules() // completion_units.go
	a.restatedPools()
	a.breadthConditions() // completion_residue.go
	a.subjectRows()
	a.inlineDesignatorFloors() // completion_residue.go
	a.remainingFrom()
	a.consecutiveLanguages()
}

// complExclusionExceptRe: a note lifting an item's exclusions for some
// students: "(c) the exclusion of GER400H1 and GER401H1 does not apply if
// the student's entry level is post- GER100Y1".
var complExclusionExceptRe = regexp.MustCompile(`(?i)\bthe\s+exclusion\s+of\s+(.+?)\s+does\s+not\s+apply\s+if\b`)

// exclusionExceptions reads a program note that lifts a pool's exclusions
// for some students (complExclusionExceptRe) as the pool's lenient
// alternative: or(pool, and(pool without them, text{the note})): Met
// without the excluded courses, Review with them. The note joins the row's
// raw.
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
			// The student's entry level is the placement test's.
			r.r.Req = orNode(n, andNode(&wide, textNode(types.TextPlacement, note)))
			addRaw(r, note)
			return true
		}
		return false
	})
}

var (
	// complLanguageCoursesRe: the title of a program's list of its language
	// courses: "_Language Courses_".
	complLanguageCoursesRe = regexp.MustCompile(`(?i)^language\s+courses$`)
	// complHigherLanguageRe: "a higher level language course" beside the
	// language courses an item names ("e.g., EAS201H1/ ... or a higher level
	// language course").
	complHigherLanguageRe = regexp.MustCompile(`(?i)^an?\s+higher[\s-]+level\s+language\s+course$`)
)

// languageCourses reads "language course" by the program's list of them
// (complLanguageCoursesRe, a list kept as a note): "a 400-level language
// course", and(credits{0.5, 400}, text{"language course"}), is the list's
// 400-level courses, credits{0.5, the list, 400}; "a higher level language
// course" in an or beside language courses is the list's courses above
// their level. Nothing changes without the list, or when a course beside
// the text isn't on it.
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

// complOtherCoursesRe: a route for courses other than the ones listed just
// above it: "Other courses, such as Special Topics courses in other
// Departments or Programs, may be approved by the Director or Undergraduate
// Director on an individual basis".
var complOtherCoursesRe = regexp.MustCompile(`(?i)^other\s+courses\b.*\bmay\s+be\s+approved\b`)

// listRoutes places a program route for other courses (complOtherCoursesRe)
// on the item whose list it follows: the line before the route's is the
// last line of that item's raw, a pool of courses. The route widens that
// pool: or(pool, text{permission}). Before placeProgramRoutes, which would
// find no row it names and keep it as a note.
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
	// complFromOneOfRe: "0.5 credit from one of PHY224H1/ PHY250H1/ ...":
	// the same as "0.5 credit from PHY224H1/ ...".
	complFromOneOfRe = regexp.MustCompile(`(?i)\b(credits?\s+from)\s+one\s+of\s+(` + codePat + `)`)
	// complAllRequiredRe: "2.0 credits comprised of the following core PCJ
	// courses, all of which are required: PCJ200H1, ...", "3.5 credits of
	// the following core PCJ courses, ...": the courses alone.
	complAllRequiredRe = regexp.MustCompile(`(?i)^\d+(?:\.\d+)?\s+credits?\s+(?:comprised\s+)?of\s+the\s+following\s+[\w\s]*?courses,\s+all\s+of\s+which\s+are\s+required:\s*`)
	// complListFloorRe: "At least 3.5 credits from the following list
	// including at least 2.5 with the BMS/JSM/JWB designator: BMS300H1/
	// ...": the floor after the list, the grammar's order ("3.5 credits from
	// BMS300H1/ ..., including at least 2.5 credits from BMS/JSM/JWB
	// courses").
	complListFloorRe = regexp.MustCompile(`(?i)^(?:at\s+least\s+)?(\d+(?:\.\d+)?\s+credits?)\s+from\s+the\s+following\s+list\s+including\s+at\s+least\s+(\d+(?:\.\d+)?)\s+(?:credits?\s+)?with\s+the\s+((?-i:[A-Z]{3})(?:/(?-i:[A-Z]{3}))*)\s+designators?:\s*(.+)$`)
	// complGroupFloorRe: a group set out after its floors: "2.0 credits from
	// the Core Group at the 300+ level, at least 0.5 of which must be at the
	// 400-level: CAR315H1, ...", "3.5 credits from the Core Group (including
	// at least 2.5 credits at the 300+ level, at least 1.0 credit of which
	// must be at the 400-level): CAR215H1, ...". The groups: the amount, the
	// group's level, the floors, the codes.
	complGroupFloorRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?\s+credits?)\s+from\s+the\s+[\w\s]+?\s+group(?:\s+at\s+the\s+([1-4])00\+\s+level)?(?:,\s*|\s*\(\s*)((?:including\s+)?at\s+least\s+[^():]+?)\)?:\s*((?:` + codePat + `,?\s*)+)$`)
	// complWhichFloorRe: "at least 0.5 of which must be at the 400-level".
	complWhichFloorRe = regexp.MustCompile(`(?i)^at\s+least\s+(\d+(?:\.\d+)?)\s+of\s+which\s+must\s+be\s+(at\s+the\s+[1-4]00-level)$`)
	// complDesigButNotRe: "in courses with a GER designator, but cannot be
	// GER400H1 or GER401H1": "in GER courses, excluding GER400H1, GER401H1".
	complDesigButNotRe = regexp.MustCompile(`(?i)\bin\s+courses\s+with\s+an?\s+((?-i:[A-Z]{3}))\s+designator,\s+but\s+cannot\s+be\s+(` + codePat + `)\s+or\s+(` + codePat + `)`)
	// complFurtherDesigRe: "1.0 credit in further ITA courses, including
	// language courses": "in ITA courses" (the language courses are ITA
	// courses too).
	complFurtherDesigRe = regexp.MustCompile(`(?i)\bin\s+further\s+((?-i:[A-Z]{3}))\s+courses(?:,\s+including\s+language\s+courses$)?`)
	// complOnlyOneFromRe: a cap at the end of a "/" list: "CHM210H1/ ... /
	// (only one from ESS261H1/ GGR203H1/ ...)".
	complOnlyOneFromRe = regexp.MustCompile(`(?i)/?\s*\(\s*only\s+one\s+from\s+((?:` + codePat + `\s*/\s*)*` + codePat + `)\s*\)$`)
	// complMeetsAsWellRe: "Ethics and Social Responsibility Requirement: The
	// Physics course JPH441H1 meets this requirement as well as any of the
	// following courses: HPS200H1/ ENV222H1/ ...": one of them, the "/" list
	// "JPH441H1/ HPS200H1/ ...".
	// complSelectedThenPairRe: "FRE272H1, 1.0 credit selected from FRE210H1/
	// ... / JFG388H1, ( FSL320H1, FSL322H1)/ FSL321Y1" (ASMAJ0525): the
	// group after the "/" list is a course of the item, not of the pool (the
	// items make the 7.0 total only so): "; " ends the pool. The groups: the
	// pool, the group.
	complSelectedThenPairRe = regexp.MustCompile(`(?i)(\bcredits?\s+selected\s+from\s+(?:` + codePat + `\s*/\s*)+` + codePat + `),\s*(\(\s*` + codePat + `\s*,\s*` + codePat + `\s*\)\s*/\s*` + codePat + `\.?)$`)
	complMeetsAsWellRe      = regexp.MustCompile(`(?i)^(?:[\w\s&,]+\s+requirement:\s*)?the\s+\w+\s+course\s+(` + codePat + `)\s+meets\s+this\s+requirement\s+as\s+well\s+as\s+any\s+of\s+the\s+following\s+courses:\s*`)
)

// rewriteOneOffs rewrites wordings one or two items use into the grammar's
// (rewriteCompletion).
func rewriteOneOffs(text string) string {
	text = complFromOneOfRe.ReplaceAllString(text, "$1 $2")
	text = complSelectedThenPairRe.ReplaceAllString(text, "$1; $2")
	if m := complGroupFloorRe.FindStringSubmatch(text); m != nil {
		floor := strings.TrimSpace(m[3])
		if f := complWhichFloorRe.FindStringSubmatch(floor); f != nil {
			floor = "including at least " + f[1] + " credit " + f[2]
		}
		codes := codeRe.FindAllString(m[4], -1)
		// The group's level is the codes' own ("at the 300+ level" over
		// 300- and 400-level codes): the grammar's pool of codes says it.
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
		// One course of the bracket, when each is a half course: at most 0.5
		// credit from them ("; up to", splitQualified).
		codes := text[m[2]:m[3]]
		if !slices.ContainsFunc(codeRe.FindAllString(codes, -1), func(c string) bool { return courseWeight(c) != 0.5 }) {
			text = strings.TrimSpace(text[:m[0]]) + "/ " + codes + "; up to 0.5 credit from " + codes
		}
	}
	text = complListFloorRe.ReplaceAllString(text, "$1 from $4, including at least $2 credits from $3 courses")
	return complAllRequiredRe.ReplaceAllString(text, "")
}

// Audit fixes (Task 10, stream C).

// restatedConstraints drops a program-level constraint that restates what
// an item already requires, joining its sentence to that item's raw: "At
// least one 0.5 credit from PCL367H1 or PCL368H1 is required for the
// program, however if desired the alternative course can be taken as a
// program elective" beside "Third Year: ..., at least one 0.5 credit from
// PCL367H1 or PCL368H1". The constraint is credits{w, courses} (with a text
// alternative or not), and the item requires or(those courses), each of
// weight w: as its body or a part of its top and.
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

// complLanguageBanRe: "Language courses cannot be used to fulfil the
// requirements of the Minor program".
var complLanguageBanRe = regexp.MustCompile(`(?i)^language\s+courses\s+cannot\s+be\s+used\s+to\s+(?:fulfil|fulfill|satisfy)\s+the\s+requirements\s+of\s+the\s+(?:minor|major|specialist)\s+program$`)

// languageBanNote makes a ban on language courses a restatement (it joins
// the total's raw; a note when there is no total) when no pool can
// take a course the program doesn't name: every item is courses and pools
// of named courses or lists (an ambiguity's lenient reading too), and the
// items fill the total, so the courses
// counted are the ones the calendar lists (ASMIN1058, whose Society-Culture
// lists hold no language course).
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
// courses or lists (an ambiguity's lenient reading too) and the items fill
// the total: no course the program doesn't name counts.
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
			// An ambiguity's text sits beside its lenient pool, which is named.
			named = named && ok && (x.Type != types.NodeText || x.Kind == types.TextAmbiguity) && (x.Type != types.NodeCredits || x.Designators == nil && x.Levels == nil && (x.Courses != nil || x.Lists != nil))
		})
	}
	return named && (a.total == nil || sum >= a.total.r.Req.Amount-1e-9)
}

// complThroughDanielsRe: "taken through the Daniels Faculty", the clause
// after "2.5 credits in VIS courses".
var complThroughDanielsRe = regexp.MustCompile(`(?i)^taken\s+through\s+the\s+Daniels\s+Faculty$`)

// throughDaniels drops "taken through the Daniels Faculty" from the row's
// body when the pool beside it is VIS courses: the Daniels Faculty offers
// them, so the clause restates the designator (ASMIN0660), and the row's
// raw keeps it.
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

// complPossibilitiesRe: a row note that lists examples of a pool it doesn't
// close: "Possibilities include but are not limited to: ( MUN100H1 and
// MUN110H1) or ( CAS100H1 and MUN120H1) or ... or ECO105Y1". The group: the
// examples.
var complPossibilitiesRe = regexp.MustCompile(`(?i)^possibilities\s+include\s+but\s+are\s+not\s+limited\s+to:\s*(.+)$`)

// possibilities reads a row of text with a complPossibilitiesRe note (and
// routes beside it): the examples are its strict alternatives, and the
// text, which the examples don't exhaust, is content beside them ("1.0
// credit from gateway 100-level courses": which courses are gateway ones):
// or(and(MUN100H1, MUN110H1), ..., text{content}, routes).
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
	// complDistributionRe: a lead that sums up the items below it, "The
	// distribution of the 7.0 credits is as follows:".
	complDistributionRe = regexp.MustCompile(`(?i)^the\s+distribution\s+of\s+the\s+\d+(?:\.\d+)?\s+credits\s+is\s+as\s+follows:?$`)
	// complSingleLanguageRe: the rule such a summary adds, "A two-year
	// progressive sequence of courses in a single language totaling 2.0
	// credits".
	complSingleLanguageRe = regexp.MustCompile(`(?i)\bsequence\s+of\s+courses\s+in\s+a\s+single\s+language\b`)
)

// distributionSummary makes a complDistributionRe item and the bullets
// under it notes: the items below say each bullet again (ASMAJ1626's year
// items). The one rule they add, that the language courses are in one
// language, stays a program-level text constraint.
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

// complApprovedSequencesRe: "Approved sequences of courses include CLT141Y1,
// CLT251H1, and CLT252H1; EAS100Y1 and EAS200Y1; ...". The group: the
// sequences.
var complApprovedSequencesRe = regexp.MustCompile(`(?i)^approved\s+sequences\s+of\s+courses\s+include\s+(.+)$`)

// approvedSequences reads an item and(text, text{complApprovedSequencesRe})
// in an or: each sequence is a strict alternative, and(its courses), and
// the item's own sentence ("Two years of study or its equivalent of one
// non-Germanic, non-Romance language in courses using the spoken language"),
// which the examples don't exhaust ("include"), is content beside them:
// what language a course teaches in decides.
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

// approvalOverAll reads "CHM395Y1/ CHM499Y1/ PHC489Y1 and/or CHM397H0/
// CHM398H0 in areas relevant to the program with prior approval by the
// Department", or(Y courses, and(or(H0 courses), text{permission})), by the
// ambiguity policy: the approval may qualify every course (the strict
// reading: none is Met without it) or only the last group (the lenient
// one, which is Review): or(and(or(all), text{permission}), and(or(the
// first group), text{ambiguity})).
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

// complTrailingCodeRe: an item whose pool ends at a ";" before one more
// course, "FOR305H1; 1.0 credit from FOR300H1, ..., FOR310H1; EEB321H1".
// The group: that course.
var complTrailingCodeRe = regexp.MustCompile(`\d+(?:\.\d+)?\s+credits?\s+from\s+[^;]+;\s*(` + codePat + `)\.?$`)

// complSemicolonPoolRe: a pool whose ";" separates groups of its courses,
// "1.0 credit from MSE101H1; ECO101H1, ECO102H1, ECO105Y1; CHM135H1".
var complSemicolonPoolRe = regexp.MustCompile(`\d+(?:\.\d+)?\s+credits?\s+from\s+[^;]+;`)

// trailingCode reads a complTrailingCodeRe item, and(..., credits{N, the
// pool's courses and the last one}), by the ambiguity policy: the ";" makes
// the last course an item of its own (the strict reading,
// and(..., credits{N, the pool}, course)), and the pool that takes it (the
// reading the total adds up with) is Review beside it. A program that writes
// ";" between groups inside another item's pool reads the same ";" as one
// more group of the pool (coordinator ruling on ASMAJ1098, ASSPE1102,
// ASMAJ1100, ASSPE1100, whose totals add up only that way), so the pool
// reading stands alone there.
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

// complCapstoneSubstituteRe: "May substitute a 0.5 credit in this
// requirement from the list of Capstone courses (all 400-level SDS and JNS
// courses)". Groups: the amount, the level, the designators.
var complCapstoneSubstituteRe = regexp.MustCompile(`(?i)^may\s+substitute\s+an?\s+(\d+(?:\.\d+)?)\s+credits?\s+in\s+this\s+requirement\s+from\s+the\s+list\s+of\s+[\w\s]+\s+courses\s+\(all\s+([1-4])00-level\s+((?-i:[A-Z]{3})(?:\s*(?:,|and)\s*(?-i:[A-Z]{3}))*)\s+courses\)$`)

// capstoneSubstitute reads or(and(part, ...), text{complCapstoneSubstituteRe})
// where each part is that amount from one list: the substitute is
// checkable, one part less and a capstone of its own. The substitute must
// be a course the parts don't use, and a capstone can be on a part's list
// (ASMIN1240: SDS425H1 in Politics & Policy), so each node stays
// independent by cases: and(choose{n-1, parts}, credits{amount,
// designators, level, exclude: the lists' capstones}) for a capstone off
// the lists; and(credits{2 × amount, part}, credits{amount, the part's
// capstones}, choose{n-2, other parts}) for one on a part's list beside
// another course of it (one on an unused part's list meets the parts
// themselves). The lists must hold only courses, share none, and their
// capstones be half courses, or the row stays as it is.
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
	// choose: k of the parts other than skip (-1: none), as an and, an or
	// or a choose.
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

// noteFloorAfterLine reads a floor written as a note after the last item of
// a program with no total ("Planning Electives: 1.5 credits from ...",
// then "Note: At least 1.0 credit must be 300/400-level courses", the
// Geography focuses) by the ambiguity policy: the note may be on the line
// above or on the whole program. The strict reading, on what that item
// claims, is Met; the lenient one, on everything, is Review:
// or(credits{floor, requirements: [the item]}, and(the floor,
// text{ambiguity})).
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

// complFloorIfCompletedRe: "at least 1.0 must be 300+ series if 1.0 credit
// is completed in #7 above" (ASSPE1006, whose item 7 is "1.0 to 1.5
// credits in ..."). The groups: the amount the condition names, the item.
var complFloorIfCompletedRe = regexp.MustCompile(`(?i)^at\s+least\s+\d+(?:\.\d+)?\s+must\s+be\s+[1-4]00\+\s+series\s+if\s+(\d+(?:\.\d+)?)\s+credits?\s+is\s+completed\s+in\s+#(\d+)\s+above$`)

// complRangeLeadRe: an item's "1.0 to 1.5 credits". The group: the upper
// amount.
var complRangeLeadRe = regexp.MustCompile(`(?i)^(?:\d+\.\s+)?\d+(?:\.\d+)?\s+to\s+(\d+(?:\.\d+)?)\s+credits?\b`)

// floorIfCompleted reads a complFloorIfCompletedRe constraint of text: the
// floor holds on "the remaining credits", which no row counts, so it stays
// text, but when the item it names claims its upper amount (more than the
// condition's) the condition is false and the constraint is Met:
// or(credits{upper, requirements: [the item]}, text).
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

// complNamedLanguagesRe: a pool's content "1.0 to 2.0 credits in Modern
// Stream languages: Arabic, Hebrew, Persian, Turkish" (ASMAJ2669). The
// group: the languages.
var complNamedLanguagesRe = regexp.MustCompile(`\blanguages:\s*([A-Z][a-z]+(?:,\s*(?:and\s+)?[A-Z][a-z]+)*)\.?$`)

// languagesNamed gives and(credits{amount}, text{content}) in r's body,
// whose text is a complNamedLanguagesRe, the languages' designators
// (languageDesignators): a language's courses are among them, and which
// are is the course's language (the text stays).
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

// complLevelCodedRe: a route in a pool, "any 100 or 200 level coded course
// from Munk or its constituent entities with prior permission of the
// Director" (ASMAJ2660). The group: the levels.
var complLevelCodedRe = regexp.MustCompile(`(?i)^any\s+([1-4]00(?:\s+or\s+[1-4]00)*)\s+level\s+coded\s+course\s+from\b.*\bwith\s+prior\s+permission\b`)

// levelCodedRoute bounds r's complLevelCodedRe route by its levels:
// or(pool, and(credits{amount, levels}, text{permission})). The unit's
// courses carry its constituents' designators too, which the text doesn't
// list, so the levels are the bound.
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

// approvedOnLists reads or(pool, and(the same pool, text{permission})) whose
// pool is lists that already hold the courses the route names with an
// approval (complSuitableCodesRe: "Up to 1.0 credit can be used from
// GGR493Y1, based on suitability of placement for this program, and
// approved by department", over ASSPE1252's Group C of GGR courses): they
// count only with it, so the strict pool leaves them out (exclude).
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

// complGraduateShareRe: "Students may be permitted to take Math graduate
// courses. A 0.5 credit of these courses may count toward specialist
// program requirements, where relevant (e.g., as courses “at the
// 400-level”)" (ASSPE1165, ASSPE1580). The groups: the designator's
// subject, the share, the level.
var complGraduateShareRe = regexp.MustCompile(`(?i)^students\s+may\s+be\s+permitted\s+to\s+take\s+(\w+)\s+graduate\s+courses\.\s+an?\s+(\d+(?:\.\d+)?)\s+credits?\s+of\s+these\s+courses\s+may\s+count\s+toward\b.*\bat\s+the\s+([1-4])00-level\b`)

// graduateAtLevel gives the graduate-course route the program placed on its
// floor at a level (or(floor, and(floor less the share, text)), namedRoutes)
// to every other row that counts courses at that level (a pool or floor of
// credits with the level among its levels): such a row may take the share
// as a graduate course, which only the approval tells, so Review:
// or(row, and(row less the share, text)). Rows below the share, or with a
// route, stay.
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

// complAsNecessaryRe: the last item's "Additional 1.0 or 1.5 credits (as
// necessary to total 4.0 credits in minor) from SAS and CAS courses or the
// list ... below" (ASMIN1333). The group: the total.
var complAsNecessaryRe = regexp.MustCompile(`(?i)\(as\s+necessary\s+to\s+total\s+(\d+(?:\.\d+)?)\s+credits\b`)

// remainderTotalPool makes the pool of a complAsNecessaryRe item, which
// holds the rest of the total, the total's own: credits{total, lists: [its
// list]}, when every other item's courses are in it (else no student could
// meet the total). The item asks for its lower amount; the total counts the
// rest from the same pool, so no marker is needed for the range.
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

// onlyListsItem reports a list item that is lists and nothing else.
func onlyListsItem(n *types.ReqNode) bool {
	return n.Type == types.NodeCredits && n.Lists != nil && n.Courses == nil && n.Designators == nil && n.Levels == nil &&
		n.Breadth == nil && n.Requirements == nil && n.Exclude == nil && n.Items == nil && n.Weight == 0
}

// complOneDisciplineRe: "1.0 credit in ONE other discipline offered within
// the Faculty of Arts & Science – African Studies, Anthropology, ...,
// Women and Gender Studies, and other departments as approved by the
// program coordinator". Groups: the amount, its unit, the disciplines.
var complOneDisciplineRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+(credits?)\s+in\s+one\s+other\s+discipline\s+offered\s+within\s+the\s+Faculty\s+of\s+Arts\s+(?:&|and)\s+Science\s*[–—-]\s*(.+?),?\s+and\s+other\s+departments\s+as\s+approved\s+by\s+the\s+program\s+coordinator$`)

// oneDiscipline reads a complOneDisciplineRe row (ASMAJ1023, ASSPE1023) as
// one alternative per discipline, all of the amount from it: credits{amount,
// its designator} (disciplineDesignators); for the disciplines no
// designator gives (contentDisciplines), and(credits{amount}, text{content})
// naming them; and for the departments the coordinator approves,
// and(credits{amount}, text{permission}). A name in neither table leaves
// the row as it is.
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

// complSeparatorCapRe: the two pools where a "/" run pairCaps reads as
// alternatives may instead be the list's separator, its group: "at least
// 1.0 from PSY320H1/ PSY321H1/ PSY322H1, PSY324H1/ ..." (ASSPE2270's Social
// Psychology focus; the Cognitive Psychology focus writes the same kind of
// pool as one "/" run) and "1.0 credit from HMB430H1/ HMB470H1/ HMB472H1;
// JPM400Y1; ..." (ASMAJ0482; its Minor lists the three with commas).
var complSeparatorCapRe = regexp.MustCompile(`(?:\bat\s+least\s+1\.0\s+from|\b1\.0\s+credit\s+from)\s+((?:` + codePat + `\s*/\s*){2}` + codePat + `)\s*(?:,\s*PSY324H1\s*/|;\s*JPM400Y1\s*;)`)

// separatorCaps reads the caps complSeparatorCapRe names by the ambiguity
// policy: the cap (at most one of the group counts) is the strict reading,
// Met; the group as plain courses of the pool is the lenient one, Review.
// The limit goes and its pool P becomes or(and(P, credits{P's amount less
// the cap, P's other courses}), and(P, text{ambiguity})): with the cap at
// most the cap's amount of the group counts, so the rest comes from P's
// other courses (P's lists with the group in exclude).
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

// separatorPool returns n with its pool of group (a credits node whose
// courses or lists name every course of group, its amount above the cap)
// read both ways (separatorCaps); ok is false when n has no such pool.
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
	// The pool's listed courses; its lists may hold filters too
	// ("PSL400-series courses").
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
