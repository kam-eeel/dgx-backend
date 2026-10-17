package completion

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/parser/grammar"
	"degree-explorer/parser/programs"
	"degree-explorer/types"
)

// One-off readings of choices: or nodes over groups, sets, languages and areas,
// and choices a row keeps or drops.

// coreGroupsMix reads coreOrGroups' or(credits{N, core}, credits{N, lists})
// by the ambiguity policy: all N from one side is strict, N mixed from a
// union of both is the Review alternative.
func (a *Assembler) coreGroupsMix(lists []*types.CourseList) []*types.CourseList {
	walkRows(a.items, func(r *compRow) {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr || len(n.Items) != 2 || r.r.Label == "" ||
			!coreOrGroupsRe.MatchString(strings.TrimSuffix(Clean(ItemLabelRe.ReplaceAllString(RawHead(r.r), "")), ".")) {
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
		lenient := programs.AndNode(&types.ReqNode{Type: types.NodeCredits, Amount: core.Amount, Lists: []string{name}},
			TextNode(types.TextAmbiguity, "the credits may be drawn from the Core Group and the Groups together"))
		r.r.Req = programs.OrNode(core, groups, lenient)
	})
	return lists
}

// researchAltsRe matches courses, some starred, or an approved project:
// "ENV491Y1*/ ENV492H1* or a minimum of 0.5 credit from any approved ...
// project (*the research topic ... must be related to environment and health)".
var researchAltsRe = regexp.MustCompile(`(?i)^((?:\(?\s*` + grammar.CodePat + `\*?\s*\)?\s*/\s*)*\(?\s*` + grammar.CodePat + `\*?\s*\)?)\s+or\s+(a\s+minimum\s+of\s+[^()]*?\bapproved\b[^()]*?)\s*\(\s*\*\s*([^()]+?)\s*\)$`)

// researchAlts reads researchAltsRe as an or of the plain courses, the
// starred ones with their condition as content, and the approved route.
func researchAlts(s string) *types.ReqNode {
	m := researchAltsRe.FindStringSubmatch(s)
	if m == nil {
		return nil
	}
	var alts, starred []*types.ReqNode
	for _, loc := range grammar.CodeRe.FindAllStringIndex(m[1], -1) {
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
	alts = append(alts, programs.AndNode(programs.OrNode(starred...), TextNode(types.TextContent, m[3])))
	return programs.OrNode(append(alts, TextNode(types.TextPermission, m[2]))...)
}

// areaCorrespondence ties a language-area item to the culture item for the
// chosen area: each area becomes a group of its two parts, under a choice of
// one. Areas pair by name or correspondence; if any doesn't, nothing changes.
func (a *Assembler) areaCorrespondence() {
	find := func(re func(string) []string) *compRow {
		for _, r := range a.items {
			t := strings.TrimSpace(ItemLabelRe.ReplaceAllString(programs.StripEmphasis(RawHead(r.r)), ""))
			if r.r.Kind == types.RequirementAllocation && len(r.children) == 0 && re(t) != nil {
				return r
			}
		}
		return nil
	}
	lang, cult := find(oneAreaRe.FindStringSubmatch), find(chosenAreaRe.FindStringSubmatch)
	if lang == nil || cult == nil {
		return
	}
	type area struct {
		name, other string
		pools       []*types.ReqNode
		line        string
	}
	read := func(row *compRow, re func(string) []string) (areas []area, alts []*types.ReqNode, ok bool) {
		m := re(strings.TrimSpace(ItemLabelRe.ReplaceAllString(programs.StripEmphasis(RawHead(row.r)), "")))
		for _, d := range a.listDefs {
			if d.row != row || d.block == nil {
				continue
			}
			pools, ok := areaPools(m[1], d.members)
			if !ok {
				return nil, nil, false
			}
			title := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(strings.ReplaceAll(programs.StripEmphasis(d.title), "_", "")), ":"))
			ar := area{name: strings.TrimSpace(areaParenRe.ReplaceAllString(title, "")), pools: pools, line: a.line(d.block)}
			if cm := correspondsRe.FindStringSubmatch(title); cm != nil {
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
	las, lalts, ok := read(lang, oneAreaRe.FindStringSubmatch)
	if !ok {
		return
	}
	cas, calts, ok := read(cult, chosenAreaRe.FindStringSubmatch)
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
				body = append(body, TextNode(t.Kind, t.Text)) // a node of each row's own
			}
			row := &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Section: part.section, Req: programs.OrNode(body...), Raw: part.line}}
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
	choice := &compRow{r: &types.Requirement{Kind: types.RequirementChoice, Section: lang.r.Section, Req: choiceNode(1, nodes), Raw: RawHead(lang.r) + "\n" + RawHead(cult.r), Notes: grammar.Dedupe(append(slices.Clone(lang.r.Notes), cult.r.Notes...))}}
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

// oneLanguage reads oneLanguageRe over its language lines as an or of a
// pool per language. A language short of the amount is its courses, as the
// text says; an approved one is permission text; the non-language
// alternative is content text.
func (a *Assembler) oneLanguage(row *compRow, text *types.ReqNode, alts []*types.ReqNode) bool {
	m := oneLanguageRe.FindStringSubmatch(Clean(text.Text))
	if m == nil {
		return false
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	head := slices.IndexFunc(a.lines, func(l string) bool { return rawWords(l) == rawWords(RawHead(row.r)) })
	if head < 0 {
		return false
	}
	var opts []*types.ReqNode
	var lines []string
	for _, l := range a.lines[head+1:] {
		l = strings.TrimSpace(l)
		if !strings.HasPrefix(l, "-") {
			break
		}
		g := languageLineRe.FindStringSubmatch(l)
		if g == nil {
			return false
		}
		lines = append(lines, l)
		codes := grammar.CodeRe.FindAllString(g[2], -1)
		switch {
		case languageCodesRe.MatchString(g[2]):
			n, _ := parseCompletionText(g[2])
			if weightOf(grammar.Dedupe(codes)) < amount-1e-9 {
				// parse literally: the courses named, short of the amount or not
				if n == nil || hasText(n) {
					n = TextNode(types.TextOther, grammar.CleanNote(strings.TrimPrefix(l, "-")))
				}
				opts = append(opts, n)
				continue
			}
			if n == nil || hasText(n) || !weighs(n, amount) {
				n = &types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: grammar.Dedupe(codes)}
			}
			opts = append(opts, n)
		default:
			p := languageApprovedRe.FindStringSubmatch(g[2])
			if p == nil || len(codes) > 1 || len(codes) == 1 && p[1] == "" {
				return false
			}
			perm := TextNode(types.TextPermission, g[1]+": "+p[2])
			if p[1] != "" {
				perm = programs.AndNode(&types.ReqNode{Type: types.NodeCourse, Code: p[1]}, perm)
			}
			opts = append(opts, perm)
		}
	}
	if len(lines) < 2 {
		return false
	}
	if m[2] != "" {
		opts = append(opts, TextNode(types.TextContent, m[2]))
	}
	joined := strings.Join(lines, "\n")
	for _, alt := range alts {
		if alt.Type != types.NodeText || !strings.Contains(rawWords(joined), rawWords(alt.Text)) {
			opts = append(opts, alt)
		}
	}
	row.r.Req = programs.OrNode(opts...)
	for _, l := range lines {
		addRaw(row, l)
	}
	row.r.Notes = slices.DeleteFunc(row.r.Notes, func(n string) bool { return strings.Contains(rawWords(joined), rawWords(n)) })
	if len(row.r.Notes) == 0 {
		row.r.Notes = nil
	}
	return true
}

// weighs reports whether every branch of n, an and/or of courses, weighs
// amount.
func weighs(n *types.ReqNode, amount float64) bool {
	switch n.Type {
	case types.NodeCourse:
		return grammar.CourseWeight(n.Code) == amount
	case types.NodeAnd:
		sum := 0.0
		for _, it := range n.Items {
			if it.Type != types.NodeCourse {
				return false
			}
			sum += grammar.CourseWeight(it.Code)
		}
		return sum == amount
	case types.NodeOr:
		for _, it := range n.Items {
			if !weighs(it, amount) {
				return false
			}
		}
		return true
	}
	return false
}

// breadthOrGroups reads breadthOrGroupsRe as two rows that claim apart:
// or(the course, credits{amount, breadth}), and the groups' pool with its
// floor and cap under it.
func (a *Assembler) breadthOrGroups(row *compRow, ls compListRefs) ([]*compRow, bool) {
	t := strings.TrimSuffix(Clean(row.r.Req.Text), ".")
	m := breadthOrGroupsRe.FindStringSubmatch(t)
	if m == nil {
		return nil, false
	}
	g := groupsThroughRe.FindStringSubmatch(m[4])
	if g == nil || g[2] >= g[3] || g[8] < g[2] || g[8] > g[3] {
		return nil, false
	}
	var keys []string
	for c := g[2][0]; c <= g[3][0]; c++ {
		keys = append(keys, listKey("group", string(c)))
	}
	names := ls.resolve(keys)
	capped := ls.resolve([]string{listKey("group", g[8])})
	if names == nil || capped == nil {
		return nil, false
	}
	amount, _ := strconv.ParseFloat(m[2], 64)
	cat, _ := strconv.Atoi(m[3])
	pool, _ := strconv.ParseFloat(g[1], 64)
	floor, _ := strconv.ParseFloat(g[4], 64)
	limit, _ := strconv.ParseFloat(g[7], 64)
	var levels []int
	for l := int(g[5][0] - '0'); l <= int(g[6][0]-'0'); l++ {
		levels = append(levels, l*100)
	}
	first, second, _ := strings.Cut(row.r.Raw, ";")
	row.r.Req = programs.OrNode(&types.ReqNode{Type: types.NodeCourse, Code: m[1]}, &types.ReqNode{Type: types.NodeCredits, Amount: amount, Breadth: []int{cat}})
	row.r.Raw = strings.TrimSpace(first)
	groups := &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Section: row.r.Section, Raw: strings.TrimSpace(second),
		Req: &types.ReqNode{Type: types.NodeCredits, Amount: pool, Lists: names}}}
	groups.children = []*compRow{
		{r: &types.Requirement{Kind: types.RequirementConstraint, Section: row.r.Section, Raw: groups.r.Raw,
			Req: &types.ReqNode{Type: types.NodeCredits, Amount: floor, Levels: levels}}},
		{r: &types.Requirement{Kind: types.RequirementLimit, Section: row.r.Section, Raw: groups.r.Raw,
			Req: &types.ReqNode{Type: types.NodeCredits, Lists: capped, MaxAmount: grammar.MaxAmount(limit)}}},
	}
	return []*compRow{row, groups}, true
}

var (
	// twoGroupsRe matches "5.0 credits from Groups A and B, of which 2.0
	// credits must be from Group A and 2.0 credits must be from Group B and 1.0
	// credit from either group".
	twoGroupsRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits\s+from\s+groups\s+((?-i:[A-Z]))\s+and\s+((?-i:[A-Z])),\s+of\s+which\s+` + grammar.AmountPat + `\s+credits\s+must\s+be\s+from\s+group\s+((?-i:[A-Z]))\s+and\s+` + grammar.AmountPat + `\s+credits\s+must\s+be\s+from\s+group\s+((?-i:[A-Z]))\s+and\s+` + grammar.AmountPat + `\s+credits?\s+from\s+either\s+group$`)
	// groupNoteRe matches a note listing one of a group's parts: "Group A:
	// Theoretical and Methods 2: Archaeometry: ARH440H1, CHM317H1".
	groupNoteRe = regexp.MustCompile(`^Group\s+((?-i:[A-Z])):\s*.+?:\s*(` + grammar.CodePat + `(?:\s*[/,]\s*` + grammar.CodePat + `)*)$`)
	// coreOrGroupsRe matches "1.5 additional credits in any area from the
	// Core Group, or 1.5 credits from Groups A, B, C, D".
	coreOrGroupsRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+additional\s+credits\s+in\s+any\s+area\s+from\s+the\s+([a-z]+\s+group),\s+or\s+` + grammar.AmountPat + `\s+credits\s+from\s+groups\s+((?-i:[A-Z])(?:\s*,\s*(?-i:[A-Z]))*)$`)
)

// twoGroups reads twoGroupsRe over the groupNoteRe notes as the
// pool of both groups, with a constraint of the two floors and a limit on
// each "/" pair. The routes stay its alternatives.
func (a *Assembler) twoGroups(row *compRow, text *types.ReqNode, alts []*types.ReqNode, ls compListRefs) bool {
	m := twoGroupsRe.FindStringSubmatch(strings.TrimSuffix(Clean(text.Text), "."))
	if m == nil || m[2] == m[3] || m[5] != m[2] || m[7] != m[3] {
		return false
	}
	var v [4]float64
	for i, g := range []string{m[1], m[4], m[6], m[8]} {
		v[i], _ = strconv.ParseFloat(g, 64)
	}
	if math.Abs(v[1]+v[2]+v[3]-v[0]) > 1e-9 {
		return false
	}
	codes := map[string][]string{}
	var pairs [][]string
	for _, n := range a.notes {
		g := groupNoteRe.FindStringSubmatch(Clean(n))
		if g == nil || g[1] != m[2] && g[1] != m[3] {
			continue
		}
		codes[g[1]] = append(codes[g[1]], grammar.CodeRe.FindAllString(g[2], -1)...)
		for _, part := range strings.Split(g[2], ",") {
			if alt := grammar.CodeRe.FindAllString(part, -1); len(alt) > 1 {
				pairs = append(pairs, alt)
			}
		}
	}
	ca, cb := grammar.Dedupe(codes[m[2]]), grammar.Dedupe(codes[m[3]])
	if weightOf(ca) < v[1]-1e-9 || weightOf(cb) < v[2]-1e-9 {
		return false
	}
	pa, pb := &types.ReqNode{Type: types.NodeCredits, Amount: v[1], Courses: ca}, &types.ReqNode{Type: types.NodeCredits, Amount: v[2], Courses: cb}
	pool := &types.ReqNode{Type: types.NodeCredits, Amount: v[0], Courses: grammar.Dedupe(append(slices.Clone(ca), cb...))}
	// the groups as the program's lists when they hold just these courses
	if same := func(name string, codes []string) bool {
		got, filters, ok := ls.members(name)
		return ok && filters == nil && ls.byName(name) != nil && !ls.byName(name).bad && slices.Equal(slices.Sorted(slices.Values(grammar.Dedupe(got))), slices.Sorted(slices.Values(codes)))
	}; same("Group "+m[2], ca) && same("Group "+m[3], cb) {
		pa = &types.ReqNode{Type: types.NodeCredits, Amount: v[1], Lists: []string{"Group " + m[2]}}
		pb = &types.ReqNode{Type: types.NodeCredits, Amount: v[2], Lists: []string{"Group " + m[3]}}
		pool = &types.ReqNode{Type: types.NodeCredits, Amount: v[0], Lists: []string{"Group " + m[2], "Group " + m[3]}}
	}
	floor, ok := partsNode(2, []*types.ReqNode{pa, pb}, ls)
	if !ok {
		return false
	}
	row.r.Req = programs.OrNode(append([]*types.ReqNode{pool}, alts...)...)
	row.children = []*compRow{{r: &types.Requirement{Kind: types.RequirementConstraint, Section: row.r.Section, Raw: row.r.Raw, Req: floor}}}
	for _, alt := range pairs {
		row.children = append(row.children, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Section: row.r.Section, Raw: row.r.Raw,
			Req: &types.ReqNode{Type: types.NodeCredits, Courses: grammar.Dedupe(alt), MaxAmount: grammar.MaxAmount(grammar.HeaviestCourse(alt))}}})
	}
	return true
}

// commaSepRe matches a comma separator: ", ".
var commaSepRe = regexp.MustCompile(`\s*,\s*`)

// coreOrGroups reads coreOrGroupsRe as or(credits{amount, the courses
// another item lists from the group}, credits{amount, the lists}).
// "Additional" is the claims rule.
func (a *Assembler) coreOrGroups(row *compRow, text *types.ReqNode, alts []*types.ReqNode, ls compListRefs) bool {
	m := coreOrGroupsRe.FindStringSubmatch(strings.TrimSuffix(Clean(text.Text), "."))
	if m == nil || m[1] != m[3] {
		return false
	}
	var core []string
	for _, r := range a.items {
		n := r.r.Req
		if r != row && n != nil && n.Type == types.NodeCredits && n.Courses != nil && n.Designators == nil && n.Lists == nil &&
			strings.Contains(strings.ToLower(r.r.Raw), "from the "+strings.ToLower(m[2])+":") {
			if core != nil {
				return false
			}
			core = slices.Clone(n.Courses)
		}
	}
	var names []string
	for _, g := range commaSepRe.Split(m[4], -1) {
		if ls.byName("Group "+g) == nil {
			return false
		}
		names = append(names, "Group "+g)
	}
	if core == nil {
		return false
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	row.r.Req = programs.OrNode(append([]*types.ReqNode{{Type: types.NodeCredits, Amount: amount, Courses: core}, {Type: types.NodeCredits, Amount: amount, Lists: names}}, alts...)...)
	return true
}

var (
	// consecutiveLanguagesRe matches "Two consecutive language courses (2.0
	// credits) in one of: Chinese, Japanese, Korean".
	consecutiveLanguagesRe = regexp.MustCompile(`(?i)^(two\s+consecutive\s+language\s+courses)\s+\(` + grammar.AmountPat + `\s+credits\)\s+in\s+one\s+of:\s*[A-Z][a-z]+(?:(?:,\s*|\s+or\s+)[A-Z][a-z]+)+$`)
	// boldLanguageLineRe matches a language and its courses: "**Chinese:**
	// EAS100Y1, EAS101Y1".
	boldLanguageLineRe = regexp.MustCompile(`^\*\*([A-Z][a-z]+):\*\*\s*(.+?)\s*$`)
	// consultedCreditRe matches "RLG264H1, RLG265H1, and an additional 1.0
	// credit in consultation with the Department for the Study of Religion".
	consultedCreditRe = regexp.MustCompile(`(?i)^(` + grammar.CodePat + `(?:\s*,\s*` + grammar.CodePat + `)*),?\s+and\s+(an\s+additional\s+` + grammar.AmountPat + `\s+credits?\s+in\s+consultation\s+with\s+the\s+.+)$`)
)

// consecutiveLanguages reads consecutiveLanguagesRe over its language
// lines as and(an or of a pool per language, text{content}): the catalogue
// doesn't record which courses are consecutive.
func (a *Assembler) consecutiveLanguages() {
	for _, row := range a.items {
		n := row.r.Req
		if row.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText {
			continue
		}
		m := consecutiveLanguagesRe.FindStringSubmatch(strings.TrimSuffix(Clean(n.Text), "."))
		if m == nil {
			continue
		}
		amount, _ := strconv.ParseFloat(m[2], 64)
		head := slices.IndexFunc(a.lines, func(l string) bool { return rawWords(l) == rawWords(RawHead(row.r)) })
		if head < 0 {
			continue
		}
		var opts []*types.ReqNode
		var lines []string
		ok := true
		for _, l := range a.lines[head+1:] {
			g := boldLanguageLineRe.FindStringSubmatch(strings.TrimSpace(l))
			if g == nil {
				break
			}
			lines = append(lines, strings.TrimSpace(l))
			if c := consultedCreditRe.FindStringSubmatch(g[2]); c != nil {
				v, _ := strconv.ParseFloat(c[3], 64)
				codes := grammar.CodeRe.FindAllString(c[1], -1)
				items := programs.CourseNodes(codes)
				if math.Abs(weightOf(codes)+v-amount) > 1e-9 {
					ok = false
					break
				}
				opts = append(opts, programs.AndNode(append(items, programs.AndNode(&types.ReqNode{Type: types.NodeCredits, Amount: v}, TextNode(types.TextPermission, g[1]+": "+c[2])))...))
				continue
			}
			codes := grammar.Dedupe(grammar.CodeRe.FindAllString(g[2], -1))
			if p, _ := parseCompletionText(g[2]); p != nil && !hasText(p) && grammar.MaxCredits(p) == amount {
				if lo, okMin := grammar.MinCredits(p); okMin && lo == amount {
					opts = append(opts, p)
					continue
				}
			}
			if !codeRunRe.MatchString(g[2]) || strings.Contains(g[2], "/") || weightOf(codes) < amount-1e-9 {
				ok = false
				break
			}
			opts = append(opts, &types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: codes})
		}
		if !ok || len(lines) < 2 {
			continue
		}
		row.r.Req = programs.AndNode(programs.OrNode(opts...), TextNode(types.TextContent, m[1]))
		joined := rawWords(strings.Join(lines, "\n"))
		for _, l := range lines {
			addRaw(row, l)
		}
		row.r.Notes = slices.DeleteFunc(row.r.Notes, func(s string) bool { return strings.Contains(joined, rawWords(s)) })
		if len(row.r.Notes) == 0 {
			row.r.Notes = nil
		}
		row.children = slices.DeleteFunc(row.children, func(k *compRow) bool {
			return k.r.Req != nil && k.r.Req.Type == types.NodeText && len(k.children) == 0 && strings.Contains(joined, rawWords(k.r.Req.Text))
		})
	}
}

// upToByCourse reads upToByCourseRe as or(the larger amount, and(the
// smaller, not(the courses that need the larger))): a course node for the one
// course would claim it from the row that does.
func (a *Assembler) upToByCourse(r *compRow) {
	if r.r.Kind != types.RequirementAllocation || r.r.Req == nil || !hasText(r.r.Req) {
		return
	}
	m := upToByCourseRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(Clean(RawHead(r.r))), "."))
	if m == nil || m[1] != m[5] {
		return
	}
	big, caps := codesCredits(m[5], m[6])
	if big == nil {
		return
	}
	small := grammar.CloneNode(big)
	small.Amount, _ = strconv.ParseFloat(m[3], 64)
	if small.Amount >= big.Amount {
		return
	}
	others := programs.CourseNodes(grammar.CodeRe.FindAllString(m[4], -1))
	alts := []*types.ReqNode{big, programs.AndNode(small, &types.ReqNode{Type: types.NodeNot, Items: []*types.ReqNode{programs.OrNode(others...)}})}
	if n := r.r.Req; n.Type == types.NodeOr {
		// the routes placed on the item stay its alternatives
		alts = append(alts, n.Items[1:]...)
	} else {
		// a program sentence letting more courses count: the list isn't all the row
		// counts, so it keeps a Review alternative
		for _, l := range strings.Split(a.raw, "\n") {
			if l = strings.TrimSuffix(strings.TrimSpace(l), "."); willCountRe.MatchString(l) {
				alts = append(alts, TextNode(types.TextContent, l))
			}
		}
	}
	r.r.Req = programs.OrNode(alts...)
	for _, c := range caps {
		c.Section = r.r.Section
		r.children = append(r.children, &compRow{r: c})
	}
}

// openToLevel reads openToLevelRe as or(the courses, and(credits from a
// list of the courses and the designators' at the level, text{permission})),
// with "one of A or B" capped. It returns the list, or nil.
func (a *Assembler) openToLevel(r *compRow, lists []*types.CourseList) *types.CourseList {
	if r.r.Kind != types.RequirementAllocation || r.r.Req == nil || !hasText(r.r.Req) || r.r.Label == "" || len(r.children) > 0 {
		return nil
	}
	m := openToLevelRe.FindStringSubmatch(strings.TrimSpace(Clean(r.r.Raw)))
	name := "Requirement " + r.r.Label + " courses"
	if m == nil || slices.ContainsFunc(lists, func(l *types.CourseList) bool { return l.Name == name }) {
		return nil
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	codes := grammar.Dedupe(grammar.CodeRe.FindAllString(m[2], -1))
	strict := &types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: codes}
	l := &types.CourseList{Name: name, Items: []*types.ReqNode{
		{Type: types.NodeCredits, Courses: slices.Clone(codes)},
		{Type: types.NodeCredits, Designators: grammar.DesigRe.FindAllString(m[5], -1), Levels: []int{int(m[4][0]-'0') * 100}},
	}}
	wide := &types.ReqNode{Type: types.NodeCredits, Amount: v, Lists: []string{name}}
	r.r.Req = programs.OrNode(strict, programs.AndNode(wide, TextNode(types.TextPermission, m[3])))
	for _, p := range oneOfOrRe.FindAllStringSubmatch(m[2], -1) {
		w := max(grammar.CourseWeight(p[1]), grammar.CourseWeight(p[2]))
		if w < v {
			r.children = append(r.children, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Section: r.r.Section, Raw: strings.TrimSpace(p[0]),
				Req: &types.ReqNode{Type: types.NodeCredits, Courses: []string{p[1], p[2]}, MaxAmount: grammar.MaxAmount(w)}}})
		}
	}
	return l
}

// minimumTailRe matches the text from "minimum" on: "minimum grade of 70%".
var minimumTailRe = regexp.MustCompile(`(?i)minimum\s.*$`)

// additionalScience reads additionalScienceRe: the row keeps its choice
// of courses; the additional credits become Req 1's pool, the designators'
// courses or any at the levels as content; the minimum is a constraint under it.
func (a *Assembler) additionalScience(r *compRow) {
	n := r.r.Req
	if a.total == nil || r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeAnd || len(r.children) > 0 {
		return
	}
	t := a.total.r.Req
	m := additionalScienceRe.FindStringSubmatch(strings.TrimSpace(Clean(RawHead(r.r))))
	if m == nil || t == nil || t.Type != types.NodeCredits || !isAnyCredits(t) || strconv.FormatFloat(t.Amount, 'f', 1, 64) != m[6] {
		return
	}
	var choice *types.ReqNode
	for _, it := range n.Items {
		switch {
		case it.Type == types.NodeChoose && choice == nil:
			choice = it
		case it.Type != types.NodeText:
			return
		}
	}
	if choice == nil {
		return
	}
	levels := grammar.ParseLevels(m[1] + "-level")
	floor, _ := strconv.ParseFloat(m[3], 64)
	r.r.Req = choice
	a.total.r.Req = programs.OrNode(&types.ReqNode{Type: types.NodeCredits, Amount: t.Amount, Designators: grammar.DesigRe.FindAllString(m[2], -1), Levels: levels},
		programs.AndNode(&types.ReqNode{Type: types.NodeCredits, Amount: t.Amount, Levels: slices.Clone(levels)}, TextNode(types.TextContent, strings.SplitN(Clean(n.Items[0].Text), ",", 2)[0])))
	addRaw(a.total, RawHead(r.r))
	a.total.children = append(a.total.children, &compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Raw: strings.TrimSpace(minimumTailRe.FindString(RawHead(r.r))),
		Req: &types.ReqNode{Type: types.NodeCredits, Amount: floor, Designators: []string{m[4]}, Levels: grammar.ParseLevels(m[5] + "00-level")}}})
}

// twoSets reads twoSetsRe as or(credits{N, one set}, credits{N, the
// other}) with the row's routes, and a limit on each groupChoiceRe.
func (a *Assembler) twoSets(r *compRow) {
	n := r.r.Req
	if r.r.Kind != types.RequirementAllocation || n == nil || len(r.children) > 0 {
		return
	}
	head, routes := n, []*types.ReqNode(nil)
	if n.Type == types.NodeOr {
		head, routes = n.Items[0], n.Items[1:]
	}
	if head.Type != types.NodeText || slices.ContainsFunc(routes, func(x *types.ReqNode) bool { return x.Type != types.NodeText }) {
		return
	}
	m := twoSetsRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(Clean(head.Text)), "."))
	if m == nil {
		return
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	var caps []*types.Requirement
	var sets []*types.ReqNode
	for _, set := range m[2:4] {
		for _, g := range groupChoiceRe.FindAllStringSubmatch(set, -1) {
			w := max(grammar.CourseWeight(g[1]), grammar.CourseWeight(g[2])+grammar.CourseWeight(g[3]))
			caps = append(caps, &types.Requirement{Kind: types.RequirementLimit, Section: r.r.Section, Raw: strings.TrimSpace(g[0]),
				Req: &types.ReqNode{Type: types.NodeCredits, Courses: []string{g[1], g[2], g[3]}, MaxAmount: grammar.MaxAmount(w)}})
		}
		plain := strings.NewReplacer("(", " ", ")", " ", "/", ",").Replace(set)
		if !IsCodeList(plain) {
			return
		}
		sets = append(sets, &types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: grammar.Dedupe(grammar.CodeRe.FindAllString(set, -1))})
	}
	r.r.Req = programs.OrNode(append(sets, routes...)...)
	for _, c := range caps {
		r.children = append(r.children, &compRow{r: c})
	}
}

// dropChoiceText drops, from an allocation's and, a text that is a choice
// row's sentence: it is that row's.
func (a *Assembler) dropChoiceText() {
	var said []string
	walkRows(append(slices.Clone(a.items), a.program...), func(r *compRow) {
		if r.r.Kind == types.RequirementChoice {
			said = append(said, rawWords(strings.Split(r.r.Raw, "\n")[0]))
		}
	})
	if said == nil {
		return
	}
	var drop func(n *types.ReqNode) *types.ReqNode
	drop = func(n *types.ReqNode) *types.ReqNode {
		switch n.Type {
		case types.NodeOr:
			for i, it := range n.Items {
				n.Items[i] = drop(it)
			}
		case types.NodeAnd:
			n.Items = slices.DeleteFunc(n.Items, func(x *types.ReqNode) bool {
				return x.Type == types.NodeText && slices.Contains(said, rawWords(x.Text))
			})
			if len(n.Items) == 1 {
				return n.Items[0]
			}
		}
		return n
	}
	walkRows(a.items, func(r *compRow) {
		if r.r.Kind == types.RequirementAllocation && r.r.Req != nil && hasText(r.r.Req) {
			r.r.Req = drop(r.r.Req)
		}
	})
}
