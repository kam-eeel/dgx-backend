package reqs

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Rules for single wordings: lead lines, breadth sentences, subject pools
// and choices among lists.

var (
	// complBreadthUnlessRe matches the Faculty's breadth rule on a program's
	// students: "... requires Art History Majors who do not complete FAH275H1, ...
	// to complete at least one 0.5 credit ... in Breadth Category 5".
	complBreadthUnlessRe = regexp.MustCompile(`(?i)^in\s+addition,\s+the\s+faculty\s+of\s+arts\s+and\s+science\s+requires\s+[\w\s]+?\s+who\s+do\s+not\s+complete\s+((?:` + codePat + `)(?:,?\s*(?:or\s+)?(?:` + codePat + `))*)\s+to\s+complete\s+at\s+least\s+(?:one\s+)?(\d+(?:\.\d+)?)\s+credits?\s+in\s+arts\s+and\s+science\s+courses\s+in\s+breadth\s+category\s+([1-5])\b`)
	// complBreadthOrGroupsRe matches Cinema Studies' three-part item: "In
	// addition, CIN230H1 (a Breadth Category 3 course) or 0.5 credit from Breadth
	// Category 5; and at least 3.0 credits from Groups B through G, ...".
	complBreadthOrGroupsRe = regexp.MustCompile(`(?i)^in\s+addition,\s+(` + codePat + `)\s+\(a\s+breadth\s+category\s+[1-5]\s+course\)\s+or\s+(?:a\s+)?(\d+(?:\.\d+)?)\s+credits?\s+from\s+breadth\s+category\s+([1-5]);\s+and\s+(.+)$`)
	// complGroupsThroughRe matches "at least 3.0 credits from Groups B through
	// G, of which 2.0 credits must be at the 300/400-level, and only a maximum of
	// 1.5 credits can be from Group G".
	complGroupsThroughRe = regexp.MustCompile(`(?i)^(?:at\s+least\s+)?(\d+(?:\.\d+)?)\s+credits\s+from\s+groups\s+((?-i:[A-Z]))\s+through\s+((?-i:[A-Z])),\s+of\s+which\s+(\d+(?:\.\d+)?)\s+credits\s+must\s+be\s+at\s+the\s+(\d)00/(\d)00-level,\s+and\s+(?:only\s+)?a\s+maximum\s+of\s+(\d+(?:\.\d+)?)\s+credits\s+can\s+be\s+from\s+group\s+((?-i:[A-Z]))\b[^,;]*$`)
	// complNamedAndBelowRe matches "2.0 credits drawn from the foundational
	// courses list and from the elective courses listed below".
	complNamedAndBelowRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+drawn\s+from\s+the\s+(\w+)\s+courses\s+list\s+and\s+from\s+the\s+elective\s+courses\s+listed\s+below$`)
	// complRemainingBelowRe matches "Remaining courses to be selected from the
	// list of eligible electives below", whose amount a note gives.
	complRemainingBelowRe = regexp.MustCompile(`(?i)^remaining\s+courses\s+to\s+be\s+selected\s+from\s+the\s+list\s+of\s+eligible\s+electives\s+below$`)
	// complRemainingAmountRe matches that note: "The remaining 1.5 credits in
	// elective courses chosen from a list of eligible electives".
	complRemainingAmountRe = regexp.MustCompile(`(?i)^the\s+remaining\s+(\d+(?:\.\d+)?)\s+credits\s+in\s+elective\s+courses\s+chosen\s+from\s+a\s+list\s+of\s+eligible\s+electives\b`)
	// complOneLanguageRe matches "1.0 credit from one of the following
	// languages", with an optional non-language alternative after it. "one the"
	// is ASMAJ1626's typo.
	complOneLanguageRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+from\s+one\s+(?:of\s+)?the\s+following\s+languages(?:,\s+or\s+(\d+(?:\.\d+)?\s+credits?\s+in\s+non-language\s+courses\s+related\s+to\s+your\s+chosen\s+language\s+of\s+study,\s+selected\s+in\s+consultation\s+with\s+the\s+program\s+coordinator))?:?$`)
	// complLanguageLineRe matches a language and its courses: "- German:
	// GER200Y1/ GER300Y1/ ( GER370H1 and GER372H1)".
	complLanguageLineRe = regexp.MustCompile(`^-\s*([A-Z][A-Za-z ,]+?):\s*(.+?)\s*$`)
	// complLanguageCodesRe matches a language's courses joined by "/" and "and",
	// in brackets or not: "GER200Y1/ ( GER370H1 and GER372H1)".
	complLanguageCodesRe = regexp.MustCompile(`^(?:\(?\s*` + codePat + `(?:\s+and\s+` + codePat + `)*\s*\)?)(?:\s*/\s*\(?\s*` + codePat + `(?:\s+and\s+` + codePat + `)*\s*\)?)*$`)
	// complCoreLeadRe matches a lead whose lines are its parts: "3.5 credits of
	// ESS core courses:".
	complCoreLeadRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits\s+of\s+(?-i:[A-Z]{3})\s+core\s+courses:$`)
	// complCorePartRe matches a part of courses: "0.5 credit of field courses:
	// ESS410H1 (...)/ ...".
	complCorePartRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+(?:chosen\s+from\s+the\s+following\s+courses|of\s+\w+\s+courses):\s*(.+)$`)
	// complCodeTitledRe matches a course and its title in brackets: "ESS224H1
	// (Intro to Mineralogy and Petrology)".
	complCodeTitledRe = regexp.MustCompile(`(` + codePat + `)\s*\([^()]*\)`)
	// complCategoriesPartRe matches a part drawn from categories under the lead:
	// "1.5 credit at the 200 and 300-level (0.5 credits must be taken from each
	// category):".
	complCategoriesPartRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+at\s+the\s+\d00\s+and\s+\d00-level\s+\((\d+(?:\.\d+)?)\s+credits\s+must\s+be\s+taken\s+from\s+each\s+category\):$`)
	// complCategoryLineRe matches a category and its courses: "Biology
	// processes: ESS261H1/ ESS363H1".
	complCategoryLineRe = regexp.MustCompile(`^[A-Z][a-z]+\s+processes:\s*((?:` + codePat + `)(?:\s*/\s*` + codePat + `)*)$`)
	// complElectivesLeadRe matches "2.5 credits of elective courses:".
	complElectivesLeadRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits\s+of\s+elective\s+courses:$`)
	// complCategoriesNoteRe matches its note: "This requirement can be satisfied
	// by any of the Earth Sciences Courses listed in categories A to F below".
	complCategoriesNoteRe = regexp.MustCompile(`(?i)^this\s+requirement\s+can\s+be\s+satisfied\s+by\s+any\s+of\s+the\s+[\w\s]+?\s+courses\s+listed\s+in\s+categories\s+((?-i:[A-Z]))\s+to\s+((?-i:[A-Z]))\s+below$`)
	// complNotLimitRe matches a note that may widen the categories: "neither
	// mutually exclusive nor meant to limit choice".
	complNotLimitRe = regexp.MustCompile(`(?i)\bneither\s+mutually\s+exclusive\s+nor\s+meant\s+to\s+limit\s+choice\b`)
	// complCategoryCodesRe matches a category kept as a note: "Paleoclimate:
	// ESS245H1/ ESS345H1, ESS261H1".
	complCategoryCodesRe = regexp.MustCompile(`^[A-Z][A-Za-z ]+:\s*((?:` + codePat + `)(?:\s*[/,]\s*` + codePat + `)*)$`)
	// complAmountFromRe matches a lead whose pool its route gives: "0.5 credit
	// from:".
	complAmountFromRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+from:$`)
	// complAboveNotCountedRe matches that route: "any of the above from Year 3
	// Req. 2-4 not previously counted or with permission of the Director:
	// ECO351H1/ TRN307H1".
	complAboveNotCountedRe = regexp.MustCompile(`(?i)^any\s+of\s+the\s+above\s+from\s+(year\s+\d)\s+req\.\s*(\d)\s*-\s*(\d)\s+not\s+previously\s+counted\s+or\s+(with\s+permission\s+of\s+the\s+\w+:\s*((?:` + codePat + `)(?:\s*/\s*` + codePat + `)*))$`)
	// complInLanguagesRe matches credits in languages the text names: "3.0
	// credits in Hebrew or Yiddish". Groups 2 and 3 are the two forms of the list.
	complInLanguagesRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits\s+in\s+(?:([A-Z][a-z]+(?:\s+or\s+[A-Z][a-z]+)+)|a\s+language\s+relevant\s+to\s+[\w\s]+?\s+[–-]\s+([A-Z][a-z]+(?:,\s+[A-Z][a-z]+)+)(?:\s+\([^()]*\))?)$`)
	// complResearchCourseRe matches "1.0 credit in a Research Course (a 299Y1,
	// 399Y1, on a Renaissance Studies topic, or REN392H1/ REN392Y1)".
	complResearchCourseRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+in\s+a\s+research\s+course\s+\((a\s+299Y1,\s+399Y1,\s+on\s+a\s+[\w\s]+?\s+topic),\s+or\s+((?:` + codePat + `)(?:\s*[/,]\s*` + codePat + `)*)\)$`)
	// complCodeRunRe matches codes joined by "/" and ",". complLanguageApprovedRe
	// matches an approved language credit: "SLA337H1, and additional 0.5 credit
	// can be fulfilled through ... approved in joint consultation with ...".
	complCodeRunRe          = regexp.MustCompile(`^` + codePat + `(?:\s*[/,]\s*` + codePat + `)*$`)
	complLanguageApprovedRe = regexp.MustCompile(`(?i)^(?:(` + codePat + `),\s+and\s+)?((?:additional\s+)?\d+(?:\.\d+)?\s+credits?\s+can\s+be\s+fulfilled\s+through\s+[^.]+?\bapproved\s+in\s+joint\s+consultation\s+with\s+[^.]+)\.`)
)

// breadthConditions reads a complBreadthUnlessRe row as a transcript
// constraint, or(the courses, credits{amount, breadth}). It claims nothing:
// the breadth course needn't be one the program counts.
func (a *compAssembler) breadthConditions() {
	for _, r := range a.allRows() {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || n.Kind != types.TextOther || len(r.children) > 0 {
			continue
		}
		m := complBreadthUnlessRe.FindStringSubmatch(complClean(n.Text))
		if m == nil {
			continue
		}
		amount, _ := strconv.ParseFloat(m[2], 64)
		cat, _ := strconv.Atoi(m[3])
		var alts []*types.ReqNode
		for _, c := range codeRe.FindAllString(m[1], -1) {
			alts = append(alts, &types.ReqNode{Type: types.NodeCourse, Code: c})
		}
		alts = append(alts, &types.ReqNode{Type: types.NodeCredits, Amount: amount, Breadth: []int{cat}})
		r.r.Kind, r.r.Scope, r.r.Req = types.RequirementConstraint, types.ScopeTranscript, orNode(alts...)
	}
}

// residueRow reads a row of text with the rules of this file. ok reports
// whether it read the row; rows is what stands for it.
func (a *compAssembler) residueRow(row *compRow, text *types.ReqNode, ls compListRefs) (rows []*compRow, ok bool) {
	if text == row.r.Req && a.coreParts(row, ls) {
		return []*compRow{row}, true
	}
	if text != row.r.Req || len(row.children) > 0 {
		return nil, false
	}
	if rows, ok := a.breadthOrGroups(row, ls); ok {
		return rows, true
	}
	if a.namedAndBelow(row, ls) || a.categoryElectives(row) {
		return []*compRow{row}, true
	}
	return nil, false
}

// residueAltRow reads a row whose body is or(text, routes) with the rules of
// this file; the routes stay its alternatives.
func (a *compAssembler) residueAltRow(row *compRow, text *types.ReqNode, ls compListRefs) bool {
	if len(row.children) > 0 {
		return false
	}
	var alts []*types.ReqNode
	if n := row.r.Req; n != text {
		alts = n.Items[1:]
	}
	return a.remainingBelow(row, text, alts, ls) || a.oneLanguage(row, text, alts) || a.aboveNotCounted(row, text, alts) ||
		a.twoGroups(row, text, alts, ls) || a.coreOrGroups(row, text, alts, ls)
}

// listsBelow returns the names of the program's top-level lists defined
// after row's line, or nil when there are none or one can't be used.
func (a *compAssembler) listsBelow(row *compRow, ls compListRefs) []string {
	line := slices.IndexFunc(a.lines, func(l string) bool { return rawWords(l) == rawWords(rawHead(row.r)) })
	if line < 0 {
		return nil
	}
	var names []string
	for _, l := range a.sortedLists(ls) {
		first := l.defs[0]
		if l.parent != "" || first.block == nil || first.row != nil && first.row != row || first.block.line < line {
			continue
		}
		if l.bad {
			return nil
		}
		names = append(names, l.list.Name)
	}
	return names
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

// compLanguageBlock is a complOneLanguageRe lead and its language lines.
type compLanguageBlock struct {
	line    int     // the lead's line
	amount  float64 // the lead's amount
	options map[string]*types.ReqNode
	order   []string // the languages whose courses read
	names   []string // every language line's language, in order
}

// languageBlocks returns the program's first n complOneLanguageRe leads with
// each language's courses as written.
func (a *compAssembler) languageBlocks(n int) []compLanguageBlock {
	var blocks []compLanguageBlock
	for i := 0; i < len(a.lines) && len(blocks) < n; i++ {
		m := complOneLanguageRe.FindStringSubmatch(complClean(strings.TrimSpace(a.lines[i])))
		if m == nil {
			continue
		}
		b := compLanguageBlock{line: i, options: map[string]*types.ReqNode{}}
		b.amount, _ = strconv.ParseFloat(m[1], 64)
		for _, l := range a.lines[i+1:] {
			g := complLanguageLineRe.FindStringSubmatch(strings.TrimSpace(l))
			if g == nil {
				break
			}
			b.names = append(b.names, g[1])
			if opt, _ := parseCompletionText(g[2]); complLanguageCodesRe.MatchString(g[2]) && opt != nil && !hasText(opt) {
				b.options[g[1]] = opt
				b.order = append(b.order, g[1])
			}
		}
		blocks = append(blocks, b)
	}
	return blocks
}

// complProgressiveSequenceRe matches "A two-year progressive sequence of
// courses in a single language totaling 2.0 credits" (ASMAJ1626).
var complProgressiveSequenceRe = regexp.MustCompile(`(?i)^a\s+two-year\s+progressive\s+sequence\s+of\s+courses\s+in\s+a\s+single\s+language\s+totall?ing\s+(\d+(?:\.\d+)?)\s+credits?\.?$`)

// progressiveSequence reads a complProgressiveSequenceRe text constraint
// over the first two language leads as an or, per language both give, of
// and(its first year's courses, its second's). It claims nothing.
func (a *compAssembler) progressiveSequence() {
	blocks := a.languageBlocks(2)
	if len(blocks) < 2 {
		return
	}
	all := append(append(append([]*compRow{}, a.floors...), a.items...), a.program...)
	walkRows(all, func(r *compRow) {
		n := r.r.Req
		if r.r.Kind != types.RequirementConstraint || n == nil || n.Type != types.NodeText {
			return
		}
		m := complProgressiveSequenceRe.FindStringSubmatch(strings.TrimSpace(complClean(n.Text)))
		if m == nil {
			return
		}
		if v, _ := strconv.ParseFloat(m[1], 64); math.Abs(blocks[0].amount+blocks[1].amount-v) > 1e-9 {
			return
		}
		var alts []*types.ReqNode
		for _, lang := range blocks[0].order {
			if y := blocks[1].options[lang]; y != nil {
				alts = append(alts, andNode(cloneNode(blocks[0].options[lang]), cloneNode(y)))
			}
		}
		if len(alts) >= 2 {
			r.r.Req = orNode(alts...)
		}
	})
}

// oneLanguage reads complOneLanguageRe over its language lines as an or of a
// pool per language. A language short of the amount is its courses, as the
// text says; an approved one is permission text; the non-language
// alternative is content text.
func (a *compAssembler) oneLanguage(row *compRow, text *types.ReqNode, alts []*types.ReqNode) bool {
	m := complOneLanguageRe.FindStringSubmatch(complClean(text.Text))
	if m == nil {
		return false
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	head := slices.IndexFunc(a.lines, func(l string) bool { return rawWords(l) == rawWords(rawHead(row.r)) })
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
		g := complLanguageLineRe.FindStringSubmatch(l)
		if g == nil {
			return false
		}
		lines = append(lines, l)
		codes := codeRe.FindAllString(g[2], -1)
		switch {
		case complLanguageCodesRe.MatchString(g[2]):
			n, _ := parseCompletionText(g[2])
			if weightOf(dedupe(codes)) < amount-1e-9 {
				// parse literally: the courses named, short of the amount or not
				if n == nil || hasText(n) {
					n = textNode(types.TextOther, cleanNote(strings.TrimPrefix(l, "-")))
				}
				opts = append(opts, n)
				continue
			}
			if n == nil || hasText(n) || !weighs(n, amount) {
				n = &types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: dedupe(codes)}
			}
			opts = append(opts, n)
		default:
			p := complLanguageApprovedRe.FindStringSubmatch(g[2])
			if p == nil || len(codes) > 1 || len(codes) == 1 && p[1] == "" {
				return false
			}
			perm := textNode(types.TextPermission, g[1]+": "+p[2])
			if p[1] != "" {
				perm = andNode(&types.ReqNode{Type: types.NodeCourse, Code: p[1]}, perm)
			}
			opts = append(opts, perm)
		}
	}
	if len(lines) < 2 {
		return false
	}
	if m[2] != "" {
		opts = append(opts, textNode(types.TextContent, m[2]))
	}
	joined := strings.Join(lines, "\n")
	for _, alt := range alts {
		if alt.Type != types.NodeText || !strings.Contains(rawWords(joined), rawWords(alt.Text)) {
			opts = append(opts, alt)
		}
	}
	row.r.Req = orNode(opts...)
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
		return courseWeight(n.Code) == amount
	case types.NodeAnd:
		sum := 0.0
		for _, it := range n.Items {
			if it.Type != types.NodeCourse {
				return false
			}
			sum += courseWeight(it.Code)
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

// breadthOrGroups reads complBreadthOrGroupsRe as two rows that claim apart:
// or(the course, credits{amount, breadth}), and the groups' pool with its
// floor and cap under it.
func (a *compAssembler) breadthOrGroups(row *compRow, ls compListRefs) ([]*compRow, bool) {
	t := strings.TrimSuffix(complClean(row.r.Req.Text), ".")
	m := complBreadthOrGroupsRe.FindStringSubmatch(t)
	if m == nil {
		return nil, false
	}
	g := complGroupsThroughRe.FindStringSubmatch(m[4])
	if g == nil || g[2] >= g[3] || g[8] < g[2] || g[8] > g[3] {
		return nil, false
	}
	var keys []string
	for c := g[2][0]; c <= g[3][0]; c++ {
		keys = append(keys, complListKey("group", string(c)))
	}
	names := ls.resolve(keys)
	capped := ls.resolve([]string{complListKey("group", g[8])})
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
	row.r.Req = orNode(&types.ReqNode{Type: types.NodeCourse, Code: m[1]}, &types.ReqNode{Type: types.NodeCredits, Amount: amount, Breadth: []int{cat}})
	row.r.Raw = strings.TrimSpace(first)
	groups := &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Section: row.r.Section, Raw: strings.TrimSpace(second),
		Req: &types.ReqNode{Type: types.NodeCredits, Amount: pool, Lists: names}}}
	groups.children = []*compRow{
		{r: &types.Requirement{Kind: types.RequirementConstraint, Section: row.r.Section, Raw: groups.r.Raw,
			Req: &types.ReqNode{Type: types.NodeCredits, Amount: floor, Levels: levels}}},
		{r: &types.Requirement{Kind: types.RequirementLimit, Section: row.r.Section, Raw: groups.r.Raw,
			Req: &types.ReqNode{Type: types.NodeCredits, Lists: capped, MaxAmount: maxAmount(limit)}}},
	}
	return []*compRow{row, groups}, true
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
				if most := heaviest(alts); len(alts) > 1 && most < v-1e-9 {
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

// heaviest returns the credit of the heaviest of codes.
func heaviest(codes []string) float64 {
	most := 0.0
	for _, c := range codes {
		most = max(most, courseWeight(c))
	}
	return most
}

// categoryElectives reads complElectivesLeadRe as the pool of the categories
// its notes set out. A complNotLimitRe note keeps any course beside it as the
// lenient reading (Review).
func (a *compAssembler) categoryElectives(row *compRow) bool {
	m := complElectivesLeadRe.FindStringSubmatch(complClean(row.r.Req.Text))
	if m == nil {
		return false
	}
	var letters []string
	var codes []string
	var read []string
	cats, open := 0, ""
	for _, n := range row.r.Notes {
		t := complClean(n)
		switch {
		case complCategoriesNoteRe.MatchString(t):
			letters = complCategoriesNoteRe.FindStringSubmatch(t)[1:]
			read = append(read, n)
		case complCategoryCodesRe.MatchString(t):
			codes = append(codes, codeRe.FindAllString(t, -1)...)
			read = append(read, n)
			cats++
		case complNotLimitRe.MatchString(t):
			open = t
		}
	}
	if letters == nil || letters[0] >= letters[1] || cats != int(letters[1][0]-letters[0][0])+1 {
		return false
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	body := &types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: dedupe(codes)}
	if open != "" {
		body = orNode(body, andNode(&types.ReqNode{Type: types.NodeCredits, Amount: amount}, textNode(types.TextAmbiguity, open)))
	}
	row.r.Req = body
	for _, n := range read {
		addRaw(row, n)
	}
	row.r.Notes = slices.DeleteFunc(row.r.Notes, func(n string) bool { return slices.Contains(read, n) })
	return true
}

// aboveNotCounted reads complAmountFromRe whose route is
// complAboveNotCountedRe as or(credits{amount, requirements: the section's
// items}, and(credits{amount, the courses}, text{permission})).
func (a *compAssembler) aboveNotCounted(row *compRow, text *types.ReqNode, alts []*types.ReqNode) bool {
	m := complAmountFromRe.FindStringSubmatch(complClean(text.Text))
	if m == nil || len(alts) != 1 || alts[0].Type != types.NodeText {
		return false
	}
	p := complAboveNotCountedRe.FindStringSubmatch(complClean(alts[0].Text))
	if p == nil || p[2] >= p[3] {
		return false
	}
	var rows []*compRow
	for i := p[2][0]; i <= p[3][0]; i++ {
		label := strings.ToLower(p[1] + " " + string(i))
		k := slices.IndexFunc(a.allRows(), func(r *compRow) bool { return strings.ToLower(r.r.Label) == label })
		if k < 0 || a.allRows()[k] == row {
			return false
		}
		rows = append(rows, a.allRows()[k])
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	ref := &types.ReqNode{Type: types.NodeCredits, Amount: amount, Requirements: []int{}} // filled in once numbered
	a.reqRefs = append(a.reqRefs, compReqRef{node: ref, rows: rows})
	perm := andNode(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: codeRe.FindAllString(p[5], -1)}, textNode(types.TextPermission, p[4]))
	row.r.Req = orNode(ref, perm)
	return true
}

// languageDesignators maps a language to the designators its courses carry.
// A language's courses are among them, not all of them: NML and MHB carry
// other languages and courses in translation too.
var languageDesignators = map[string][]string{
	"arabic": {"NML"}, "persian": {"NML"}, "turkish": {"NML"}, "french": {"FRE", "FSL"}, "german": {"GER"}, "hebrew": {"MHB", "NML"}, "italian": {"ITA"},
	"latin": {"LAT"}, "portuguese": {"PRT"}, "spanish": {"SPA"}, "yiddish": {"GER"},
}

// disciplineDesignators maps a discipline to the one designator its courses
// carry. Disciplines without one are in contentDisciplines.
var disciplineDesignators = map[string]string{
	"african studies": "AFR", "anthropology": "ANT", "art history": "FAH", "caribbean studies": "CAR",
	"cinema studies": "CIN", "critical studies in equity and solidarity": "CSE", "drama": "DRM",
	"east asian studies": "EAS", "history": "HIS", "political science": "POL", "philosophy": "PHL",
	"psychology": "PSY", "sociology": "SOC", "women and gender studies": "WGS",
}

// contentDisciplines holds the disciplines no one designator gives: which
// courses are theirs is the course's content.
var contentDisciplines = map[string]bool{
	"classics": true, "material culture and semiotics": true, "music": true, "renaissance studies": true,
}

// subjectRows reads rows of text whose subject decides their courses:
// complInLanguagesRe as the listed courses or and(credits{the languages'
// designators}, text{content}), and complResearchCourseRe as or(the codes,
// text{content}).
func (a *compAssembler) subjectRows() {
	for _, r := range a.allRows() {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || n.Kind != types.TextOther || len(r.children) > 0 {
			continue
		}
		t := strings.TrimSuffix(complClean(n.Text), ".")
		if m := complInLanguagesRe.FindStringSubmatch(t); m != nil {
			names := regexp.MustCompile(`\s+or\s+|,\s+`).Split(m[2]+m[3], -1)
			var desigs []string
			for _, name := range names {
				d := languageDesignators[strings.ToLower(name)]
				if d == nil {
					desigs = nil
					break
				}
				desigs = append(desigs, d...)
			}
			if desigs == nil {
				continue
			}
			amount, _ := strconv.ParseFloat(m[1], 64)
			if codes, keep := languageCodeLines(names, r.r.Notes); codes != nil {
				// "Hebrew language courses: MHB155H1/ ..." lists them
				r.r.Req, r.r.Notes = &types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: codes}, keep
				continue
			}
			r.r.Req = andNode(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Designators: dedupe(desigs)}, textNode(types.TextContent, n.Text))
			continue
		}
		if m := complResearchCourseRe.FindStringSubmatch(t); m != nil {
			amount, _ := strconv.ParseFloat(m[1], 64)
			r.r.Req = orNode(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: dedupe(codeRe.FindAllString(m[3], -1))}, textNode(types.TextContent, m[2]))
		}
	}
}

// languageCodeLines returns the courses of the notes that list each
// language's courses ("Yiddish language courses: GER260Y1/ ...") and the other
// notes; nil when a language has no such line.
func languageCodeLines(names, notes []string) (codes, keep []string) {
	found := map[string]bool{}
	for _, n := range notes {
		name, list, ok := strings.Cut(n, " language courses:")
		if ok && slices.ContainsFunc(names, func(x string) bool { return strings.EqualFold(x, strings.TrimSpace(name)) }) && isCodeList(strings.TrimSpace(list)) {
			found[strings.ToLower(strings.TrimSpace(name))] = true
			codes = append(codes, codeRe.FindAllString(list, -1)...)
			continue
		}
		keep = append(keep, n)
	}
	if len(found) != len(names) {
		return nil, notes
	}
	return dedupe(codes), keep
}

// complInlineDesigFloorRe matches a floor defining its subject by
// designators: "at least 3.0 credits of which must be Department for the Study
// of Religion courses (RLG, JAR, ... or JSR designators)".
var complInlineDesigFloorRe = regexp.MustCompile(`(?i)^at\s+least\s+(\d+(?:\.\d+)?)\s+credits?\s+of\s+which\s+must\s+be\s+[\w\s]+?\s+courses\s+\(\s*((?-i:[A-Z]{3})(?:\s*,\s*(?:or\s+)?(?-i:[A-Z]{3}))*)\s+designators\s*\)$`)

// inlineDesignatorFloors reads and(credits, text{complInlineDesigFloorRe}),
// alone or first beside route texts, as the credits with a constraint of the
// amount in the named designators.
func (a *compAssembler) inlineDesignatorFloors() {
	for _, r := range a.allRows() {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil {
			continue
		}
		at := &r.r.Req
		if n.Type == types.NodeOr && len(n.Items) > 1 && !slices.ContainsFunc(n.Items[1:], func(x *types.ReqNode) bool { return x.Type != types.NodeText }) {
			at = &n.Items[0] // or(and(pool, floor), route text): the floor is the pool's
		}
		p := *at
		if p.Type != types.NodeAnd || len(p.Items) != 2 || p.Items[0].Type != types.NodeCredits || p.Items[1].Type != types.NodeText {
			continue
		}
		m := complInlineDesigFloorRe.FindStringSubmatch(strings.TrimSuffix(complClean(p.Items[1].Text), "."))
		if m == nil {
			continue
		}
		amount, _ := strconv.ParseFloat(m[1], 64)
		if amount <= 0 || amount > p.Items[0].Amount {
			continue
		}
		*at = p.Items[0]
		floor := &compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Req: &types.ReqNode{Type: types.NodeCredits, Amount: amount, Designators: desigRe.FindAllString(m[2], -1)}, Raw: r.r.Raw}}
		r.children = append([]*compRow{floor}, r.children...)
	}
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

var (
	// complTwoGroupsRe matches "5.0 credits from Groups A and B, of which 2.0
	// credits must be from Group A and 2.0 credits must be from Group B and 1.0
	// credit from either group".
	complTwoGroupsRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits\s+from\s+groups\s+((?-i:[A-Z]))\s+and\s+((?-i:[A-Z])),\s+of\s+which\s+(\d+(?:\.\d+)?)\s+credits\s+must\s+be\s+from\s+group\s+((?-i:[A-Z]))\s+and\s+(\d+(?:\.\d+)?)\s+credits\s+must\s+be\s+from\s+group\s+((?-i:[A-Z]))\s+and\s+(\d+(?:\.\d+)?)\s+credits?\s+from\s+either\s+group$`)
	// complGroupNoteRe matches a note listing one of a group's parts: "Group A:
	// Theoretical and Methods 2: Archaeometry: ARH440H1, CHM317H1".
	complGroupNoteRe = regexp.MustCompile(`^Group\s+((?-i:[A-Z])):\s*.+?:\s*(` + codePat + `(?:\s*[/,]\s*` + codePat + `)*)$`)
	// complCoreOrGroupsRe matches "1.5 additional credits in any area from the
	// Core Group, or 1.5 credits from Groups A, B, C, D".
	complCoreOrGroupsRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+additional\s+credits\s+in\s+any\s+area\s+from\s+the\s+([a-z]+\s+group),\s+or\s+(\d+(?:\.\d+)?)\s+credits\s+from\s+groups\s+((?-i:[A-Z])(?:\s*,\s*(?-i:[A-Z]))*)$`)
)

// twoGroups reads complTwoGroupsRe over the complGroupNoteRe notes as the
// pool of both groups, with a constraint of the two floors and a limit on
// each "/" pair. The routes stay its alternatives.
func (a *compAssembler) twoGroups(row *compRow, text *types.ReqNode, alts []*types.ReqNode, ls compListRefs) bool {
	m := complTwoGroupsRe.FindStringSubmatch(strings.TrimSuffix(complClean(text.Text), "."))
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
		g := complGroupNoteRe.FindStringSubmatch(complClean(n))
		if g == nil || g[1] != m[2] && g[1] != m[3] {
			continue
		}
		codes[g[1]] = append(codes[g[1]], codeRe.FindAllString(g[2], -1)...)
		for _, part := range strings.Split(g[2], ",") {
			if alt := codeRe.FindAllString(part, -1); len(alt) > 1 {
				pairs = append(pairs, alt)
			}
		}
	}
	ca, cb := dedupe(codes[m[2]]), dedupe(codes[m[3]])
	if weightOf(ca) < v[1]-1e-9 || weightOf(cb) < v[2]-1e-9 {
		return false
	}
	pa, pb := &types.ReqNode{Type: types.NodeCredits, Amount: v[1], Courses: ca}, &types.ReqNode{Type: types.NodeCredits, Amount: v[2], Courses: cb}
	pool := &types.ReqNode{Type: types.NodeCredits, Amount: v[0], Courses: dedupe(append(slices.Clone(ca), cb...))}
	// the groups as the program's lists when they hold just these courses
	if same := func(name string, codes []string) bool {
		got, filters, ok := ls.members(name)
		return ok && filters == nil && ls.byName(name) != nil && !ls.byName(name).bad && slices.Equal(slices.Sorted(slices.Values(dedupe(got))), slices.Sorted(slices.Values(codes)))
	}; same("Group "+m[2], ca) && same("Group "+m[3], cb) {
		pa = &types.ReqNode{Type: types.NodeCredits, Amount: v[1], Lists: []string{"Group " + m[2]}}
		pb = &types.ReqNode{Type: types.NodeCredits, Amount: v[2], Lists: []string{"Group " + m[3]}}
		pool = &types.ReqNode{Type: types.NodeCredits, Amount: v[0], Lists: []string{"Group " + m[2], "Group " + m[3]}}
	}
	floor, ok := partsNode(2, []*types.ReqNode{pa, pb}, ls)
	if !ok {
		return false
	}
	row.r.Req = orNode(append([]*types.ReqNode{pool}, alts...)...)
	row.children = []*compRow{{r: &types.Requirement{Kind: types.RequirementConstraint, Section: row.r.Section, Raw: row.r.Raw, Req: floor}}}
	for _, alt := range pairs {
		row.children = append(row.children, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Section: row.r.Section, Raw: row.r.Raw,
			Req: &types.ReqNode{Type: types.NodeCredits, Courses: dedupe(alt), MaxAmount: maxAmount(heaviest(alt))}}})
	}
	return true
}

// coreOrGroups reads complCoreOrGroupsRe as or(credits{amount, the courses
// another item lists from the group}, credits{amount, the lists}).
// "Additional" is the claims rule.
func (a *compAssembler) coreOrGroups(row *compRow, text *types.ReqNode, alts []*types.ReqNode, ls compListRefs) bool {
	m := complCoreOrGroupsRe.FindStringSubmatch(strings.TrimSuffix(complClean(text.Text), "."))
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
	for _, g := range regexp.MustCompile(`\s*,\s*`).Split(m[4], -1) {
		if ls.byName("Group "+g) == nil {
			return false
		}
		names = append(names, "Group "+g)
	}
	if core == nil {
		return false
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	row.r.Req = orNode(append([]*types.ReqNode{{Type: types.NodeCredits, Amount: amount, Courses: core}, {Type: types.NodeCredits, Amount: amount, Lists: names}}, alts...)...)
	return true
}

var (
	// complConsecutiveLanguagesRe matches "Two consecutive language courses (2.0
	// credits) in one of: Chinese, Japanese, Korean".
	complConsecutiveLanguagesRe = regexp.MustCompile(`(?i)^(two\s+consecutive\s+language\s+courses)\s+\((\d+(?:\.\d+)?)\s+credits\)\s+in\s+one\s+of:\s*[A-Z][a-z]+(?:(?:,\s*|\s+or\s+)[A-Z][a-z]+)+$`)
	// complBoldLanguageLineRe matches a language and its courses: "**Chinese:**
	// EAS100Y1, EAS101Y1".
	complBoldLanguageLineRe = regexp.MustCompile(`^\*\*([A-Z][a-z]+):\*\*\s*(.+?)\s*$`)
	// complConsultedCreditRe matches "RLG264H1, RLG265H1, and an additional 1.0
	// credit in consultation with the Department for the Study of Religion".
	complConsultedCreditRe = regexp.MustCompile(`(?i)^(` + codePat + `(?:\s*,\s*` + codePat + `)*),?\s+and\s+(an\s+additional\s+(\d+(?:\.\d+)?)\s+credits?\s+in\s+consultation\s+with\s+the\s+.+)$`)
)

// consecutiveLanguages reads complConsecutiveLanguagesRe over its language
// lines as and(an or of a pool per language, text{content}): the catalogue
// doesn't record which courses are consecutive.
func (a *compAssembler) consecutiveLanguages() {
	for _, row := range a.items {
		n := row.r.Req
		if row.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText {
			continue
		}
		m := complConsecutiveLanguagesRe.FindStringSubmatch(strings.TrimSuffix(complClean(n.Text), "."))
		if m == nil {
			continue
		}
		amount, _ := strconv.ParseFloat(m[2], 64)
		head := slices.IndexFunc(a.lines, func(l string) bool { return rawWords(l) == rawWords(rawHead(row.r)) })
		if head < 0 {
			continue
		}
		var opts []*types.ReqNode
		var lines []string
		ok := true
		for _, l := range a.lines[head+1:] {
			g := complBoldLanguageLineRe.FindStringSubmatch(strings.TrimSpace(l))
			if g == nil {
				break
			}
			lines = append(lines, strings.TrimSpace(l))
			if c := complConsultedCreditRe.FindStringSubmatch(g[2]); c != nil {
				v, _ := strconv.ParseFloat(c[3], 64)
				var items []*types.ReqNode
				for _, code := range codeRe.FindAllString(c[1], -1) {
					items = append(items, &types.ReqNode{Type: types.NodeCourse, Code: code})
				}
				if math.Abs(weightOf(codeRe.FindAllString(c[1], -1))+v-amount) > 1e-9 {
					ok = false
					break
				}
				opts = append(opts, andNode(append(items, andNode(&types.ReqNode{Type: types.NodeCredits, Amount: v}, textNode(types.TextPermission, g[1]+": "+c[2])))...))
				continue
			}
			codes := dedupe(codeRe.FindAllString(g[2], -1))
			if p, _ := parseCompletionText(g[2]); p != nil && !hasText(p) && maxCredits(p) == amount {
				if lo, okMin := minCredits(p); okMin && lo == amount {
					opts = append(opts, p)
					continue
				}
			}
			if !complCodeRunRe.MatchString(g[2]) || strings.Contains(g[2], "/") || weightOf(codes) < amount-1e-9 {
				ok = false
				break
			}
			opts = append(opts, &types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: codes})
		}
		if !ok || len(lines) < 2 {
			continue
		}
		row.r.Req = andNode(orNode(opts...), textNode(types.TextContent, m[1]))
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
