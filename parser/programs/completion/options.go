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

// Options and choices: option lines, "EITHER ... OR" lines, "X **OR** Y",
// "or else", categories and areas the student chooses among.

var (
	// optionLabelRe matches an option's label, never a list's: "Option A".
	optionLabelRe = regexp.MustCompile(`(?i)^option\s+\w+\b`)
	// optionsLeadRe matches "Choose one of the options below:".
	optionsLeadRe = regexp.MustCompile(`(?i)^(?:choose|complete|select)\s+(?:one|1)\s+of\s+the\s+(?:following\s+)?options?\b[^:]*:?$`)
	// countOptionsLeadRe matches a lead over option bullets: "At least two of
	// the following four options:".
	countOptionsLeadRe = regexp.MustCompile(`(?i)^(?:` + grammar.AtLeastPat + `\s+)?(one|two|three|four|[1-4])\s+of\s+the\s+following\s+(?:(two|three|four|five|six|[2-6])\s+)?options\s*:$`)
	// aboveOptionsRe matches an option that includes others: "LMP305Y1 and
	// one of the above four options".
	aboveOptionsRe = regexp.MustCompile(`(?i)^(.+?)\s+and\s+one\s+of\s+the\s+above(?:\s+(two|three|four|five|six|[2-6]))?\s+options$`)
	// eitherRe matches the "EITHER " an item's first option starts with.
	eitherRe = regexp.MustCompile(`^(?:EITHER|Either)\s+`)
	// ifTakingRe matches an option that trades credits with another item's
	// pool: "2.0 credits if taking 1.5 credits from CAR215H1, CAR220H1".
	ifTakingRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\s+if\s+taking\s+(` + grammar.NumPat + `\s+credits?\s+from\s+.+)$`)
	// amountSourceRe matches an option's amount and source: "2.5 credits from
	// Group A or B".
	amountSourceRe = regexp.MustCompile(`(?i)^` + grammar.NumPat + `\s+credits?\s+(from\s+.+)$`)
	// inlineOrRe matches an upper-case " OR " between an item's options; a
	// lower-case "or" is the grammar's.
	inlineOrRe = regexp.MustCompile(`\s*,?\s+OR\s+`)
	// optionStartRe matches an option's start, an amount or a code: "1.0
	// credit".
	optionStartRe = regexp.MustCompile(`^(?:` + grammar.NumPat + `\s+(?:additional\s+)?credits?\b|` + grammar.CodePat + `)`)
	// orElseRe matches "Additional philosophy courses, to a total of 7.0
	// credits, or else additional philosophy courses to a total of 6.5 credits,
	// plus one 0.5 credit from Group 4 (Interdisciplinary)".
	orElseRe = regexp.MustCompile(`(?i)^additional\s+(.+?),?\s+to\s+a\s+total\s+of\s+` + grammar.AmountPat + `\s+credits?,?\s+or\s+else\s+additional\s+(.+?),?\s+to\s+a\s+total\s+of\s+` + grammar.AmountPat + `\s+credits?,?\s+(?:plus|and)\s+(?:one\s+)?` + grammar.AmountPat + `\s+credits?\s+from\s+(.+?)(?:\s*\([^()]*\))?\.?$`)
	// categoriesRe matches "1.0 to 1.5 credits in at least two of the three
	// following categories: (1) ...".
	categoriesRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `(?:\s+to\s+` + grammar.NumPat + `)?\s+credits?\s+in\s+` + grammar.AtLeastPat + `\s+(one|two|three|four|[1-4])\s+of\s+the\s+(two|three|four|five|six|[2-6])\s+following\s+(?:categories|areas|groups)\s*:\s*(.+)$`)
	// categoryMarkRe matches a category's number: "; and/or (3)".
	categoryMarkRe = regexp.MustCompile(`\s*;?\s*(?:and/or\s+|and\s+|or\s+)?\(\d\)\s*`)
	// categoryRe matches a category: "one seminar (0.5 credit) from EEB491H1,
	// EEB495H1".
	categoryRe = regexp.MustCompile(`(?i)^(?:one|an?)\s+[\w\s-]+?\s+(\(` + grammar.NumPat + `\s+credits?\)\s+from\s+.+)$`)
	// weightFromRe matches "(0.5 credit) from EEB401H1, EEB403H1".
	weightFromRe = regexp.MustCompile(`(?i)^\(` + grammar.AmountPat + `\s+credits?\)\s+from\s+(.+?)$`)
	// chooseCoursesRe matches "Students should choose four of the following
	// 400-level courses (2.0-2.5 credits): LMP405Y1, LMP410H1".
	chooseCoursesRe = regexp.MustCompile(`(?i)^(?:students\s+(?:should|must)\s+)?(?:choose|select|take)\s+(two|three|four|five|six|[2-6])\s+of\s+the\s+following\s+(?:(` + grammar.LevelPat + `)-level\s+)?courses\s*(?:\(` + grammar.AmountPat + `\s*(?:-|–|to)\s*` + grammar.AmountPat + `\s+credits\))?\s*:\s*(.+)$`)
	// chooseRestatedRe matches a note restating such a count: "If the research
	// course LMP405Y1 is taken, an additional three 400-level courses from the list
	// are still required".
	chooseRestatedRe = regexp.MustCompile(`(?i)^if\s+(?:the\s+)?[\w\s-]*?\b(` + grammar.CodePat + `)\s+is\s+taken,\s+an\s+additional\s+(one|two|three|four|five|[1-5])\s+(?:` + grammar.LevelPat + `-level\s+)?courses\s+from\s+the\s+list\s+are\s+still\s+required\b`)
	// ifNotCompletedRe matches an item restating another row's alternative:
	// "If a student has not completed STA130H1 then an additional 0.5 credit ...
	// that is not used towards any other program requirement must be completed".
	ifNotCompletedRe = regexp.MustCompile(`(?i)^if\s+(?:a\s+)?students?\s+(?:has|have)\s+not\s+completed\s+(` + grammar.CodePat + `)\s*,?\s+then\s+an\s+additional\s+(.+?)\s+that\s+is\s+not\s+used\s+towards?\s+any\s+other\s+program\s+requirement\s+must\s+be\s+completed$`)
	// restatedOrRe matches a line restating an item's alternative: "OR 2.0
	// credits in any major African language approved by the Program Committee".
	restatedOrRe = regexp.MustCompile(`^OR\s+(.+?)\.?$`)
)

var (
	// perAreaRe matches an item over chosen areas: "4.0 credits of language
	// coursework from each of the two chosen language areas (2.0 credits per
	// area)".
	perAreaRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\s+.+?\s+(?:from\s+each\s+of|corresponding\s+to)\s+the\s+(?:student.s\s+)?(two|three|[23])\s+chosen\s+(\w+)\s+areas\s*\(` + grammar.AmountPat + `\s+credits?\s+per\s+area\)\s*[.:]?$`)
	// chooseAreasRe matches the choice of areas: "Students must choose any two
	// of the available language areas (Czech and Slovak, Estonian)".
	chooseAreasRe = regexp.MustCompile(`(?i)^students\s+must\s+choose\s+(?:any\s+)?(two|three|[23])\s+of\s+the\s+(?:available\s+)?(\w+)\s+areas\s*\(([^()]+)\)$`)
	// correspondsRe matches an area named differently in the other list:
	// "Yiddish (corresponds to the Jewish Cultures in Eastern Europe area)".
	correspondsRe = regexp.MustCompile(`(?i)^(.+?)\s*\(corresponds\s+to\s+the\s+(.+?)\s+(?:language\s+)?area\)$`)
	// areaParenRe matches a closing aside on an area's name: "South Slavic
	// (Bosnian, Croatian, Serbian, Macedonian)".
	areaParenRe = regexp.MustCompile(`\s*\([^()]*\)$`)
)

// compChoiceRef is a requirement node of a choice row and the rows it picks
// among, numbered by numberChoices.
type compChoiceRef struct {
	node *types.ReqNode
	rows []*compRow
}

// reqNode returns a requirement node over rows; numberChoices numbers it.
func (a *Assembler) reqNode(rows ...*compRow) *types.ReqNode {
	n := &types.ReqNode{Type: types.NodeRequirement}
	a.choiceRefs = append(a.choiceRefs, compChoiceRef{node: n, rows: rows})
	return n
}

// numberChoices fills in the choice rows' requirement nodes once the rows are
// numbered.
func (a *Assembler) numberChoices() {
	for _, ref := range a.choiceRefs {
		ref.node.Requirements = nil
		for _, r := range ref.rows {
			ref.node.Requirements = append(ref.node.Requirements, r.r.Number)
		}
	}
}

// choiceNode is an or (count 0 or 1) or a choose over items. It skips OrNode's
// flattening and dedupe: unnumbered requirement nodes would all look the same.
func choiceNode(count int, items []*types.ReqNode) *types.ReqNode {
	if count > 1 {
		return &types.ReqNode{Type: types.NodeChoose, Count: count, Items: items}
	}
	return &types.ReqNode{Type: types.NodeOr, Items: items}
}

// addChoice adds the options as allocations under row 1, then the choice row
// over them, so options the student didn't choose count nothing.
func (a *Assembler) addChoice(opts []*compRow, label, section, raw string, body *types.ReqNode) {
	a.items = append(a.items, opts...)
	a.items = append(a.items, &compRow{r: &types.Requirement{Kind: types.RequirementChoice, Label: label, Section: section, Req: body, Raw: raw}})
}

// optionRow returns an allocation for one option, read as an item's body with
// the rules it states under it.
func (a *Assembler) optionRow(text, label, section, raw string) *compRow {
	body, rows, notes := a.itemBody(text)
	row := &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Label: label, Section: section, Req: body, Notes: notes, Raw: raw}}
	for _, r := range rows {
		a.rowUnder(row, r)
	}
	if body != nil {
		splitCaps(row)
	}
	return row
}

// plainOptions reports whether every option is a body of courses alone: no
// text, no rows under it.
func plainOptions(opts []*compRow) bool {
	return !slices.ContainsFunc(opts, func(o *compRow) bool { return o.r.Req == nil || hasText(o.r.Req) || len(o.children) > 0 })
}

// optionBlocks reads options set out over lines from bs[0], and returns how
// many blocks it read: an options lead and its "Option A:" lines or bullets,
// or "EITHER ... OR" lines, each but the last ending in "OR".
func (a *Assembler) optionBlocks(bs []*Block, ctx compCtx) int {
	b := bs[0]
	if b.Kind == BlockHeading {
		return 0
	}
	text := strings.TrimSpace(b.Text)
	if optionsLeadRe.MatchString(text) {
		if len(b.Children) >= 2 && optionLines(b.Children) == len(b.Children) {
			a.options(b, b.Children, 1, ctx)
			return 1
		}
		if n := optionLines(bs[1:]); n >= 2 && len(b.Children) == 0 {
			a.options(b, bs[1:1+n], 1, ctx)
			return n + 1
		}
		return 0
	}
	if m := countOptionsLeadRe.FindStringSubmatch(text); m != nil && len(b.Children) >= 2 {
		if m[2] != "" && countOf(m[2]) != len(b.Children) || slices.ContainsFunc(b.Children, func(c *Block) bool { return len(c.Children) > 0 }) {
			return 0
		}
		a.options(b, b.Children, countOf(m[1]), ctx)
		return 1
	}
	if ctx.owner == nil && blkNumbered(b) {
		if n := a.areaOptions(bs); n > 0 {
			return n
		}
	}
	if ctx.owner != nil || !trailingCommaOrRe.MatchString(b.Text) || len(b.Children) > 0 {
		return 0
	}
	n := 1
	for n < len(bs) && trailingCommaOrRe.MatchString(bs[n-1].Text) && bs[n].Kind != BlockHeading && len(bs[n].Children) == 0 && !blkNumbered(bs[n]) {
		n++
	}
	if n < 2 || n == 2 && !eitherRe.MatchString(text) || trailingCommaOrRe.MatchString(bs[n-1].Text) {
		return 0
	}
	if !a.eitherLines(bs[:n]) {
		return 0
	}
	return n
}

func optionLines(bs []*Block) int {
	n := 0
	for _, c := range bs {
		if !optionLabelRe.MatchString(c.Label) || len(c.Children) > 0 {
			break
		}
		n++
	}
	return n
}

// options reads a lead over option lines as one allocation, the options' or
// (choose when count > 1). An option including the others gets rows and a
// choice; else the lead is text with the options as notes.
func (a *Assembler) options(b *Block, optLines []*Block, count int, ctx compCtx) {
	var opts []*compRow
	above := map[int]int{} // option index -> how many options before it it includes
	ok := true
	for i, c := range optLines {
		text := c.Text
		if j := strings.LastIndex(text, ":"); j >= 0 {
			text = text[j+1:] // "Immunology: ( IMM341H1, IMM351H1)"
		}
		text = strings.TrimSpace(text)
		if m := aboveOptionsRe.FindStringSubmatch(text); m != nil && i > 0 {
			if m[2] != "" && countOf(m[2]) != i {
				ok = false
				break
			}
			above[i], text = i, m[1]
		}
		o, _ := parseCompletionText(text)
		if o = checkedBody(o, text); o == nil || hasText(o) {
			ok = false
			break
		}
		label := ""
		if optionLabelRe.MatchString(c.Label) {
			label = c.Label
		}
		opts = append(opts, &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Label: label, Section: c.Section, Req: o, Raw: a.line(c)}})
	}
	if ok && count < len(opts) && len(above) == 0 {
		var nodes []*types.ReqNode
		raw := a.line(b)
		for _, o := range opts {
			nodes = append(nodes, o.r.Req)
			raw += "\n" + o.r.Raw
		}
		body := programs.OrNode(nodes...)
		if count > 1 {
			body = &types.ReqNode{Type: types.NodeChoose, Count: count, Items: nodes}
		}
		a.add(&compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Label: itemLabel(b), Section: b.Section, Req: body, Raw: raw}}, ctx.owner)
		return
	}
	if ok && count == 1 && ctx.owner == nil {
		// "Option E" includes one of A-D: or(A, ..., D, and(E, any of A-D))
		var nodes []*types.ReqNode
		for i, o := range opts {
			n := a.reqNode(o)
			if k, ok := above[i]; ok {
				n = &types.ReqNode{Type: types.NodeAnd, Items: []*types.ReqNode{n, a.reqNode(opts[:k]...)}}
			}
			nodes = append(nodes, n)
		}
		a.addChoice(opts, itemLabel(b), b.Section, a.line(b), choiceNode(1, nodes))
		return
	}
	row := &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Label: itemLabel(b), Section: b.Section, Req: TextNode(types.TextOther, b.Text), Raw: a.line(b)}}
	for _, c := range optLines {
		row.r.Raw += "\n" + a.line(c)
		row.r.Notes = append(row.r.Notes, grammar.CleanNote(BlockLine(c)))
	}
	a.add(row, ctx.owner)
}

// eitherLines reads an item whose options are lines ending in "OR": the
// options' or when each is courses, else a choice. "N credits if taking M
// credits from <pool>" is N from the first option's source plus poolBeyond.
func (a *Assembler) eitherLines(bs []*Block) bool {
	var parts []string
	var raws []string
	for _, c := range bs {
		t := trailingCommaOrRe.ReplaceAllString(strings.TrimSpace(c.Text), "")
		parts = append(parts, strings.TrimSpace(eitherRe.ReplaceAllString(t, "")))
		raws = append(raws, a.line(c))
	}
	source := ""
	if m := amountSourceRe.FindStringSubmatch(parts[0]); m != nil {
		source = m[1] // "from Group A or B"
	}
	b := bs[0]
	var opts []*compRow
	for i, p := range parts {
		m := ifTakingRe.FindStringSubmatch(p)
		if m == nil {
			opts = append(opts, a.optionRow(p, "", b.Section, raws[i]))
			continue
		}
		extra := a.poolBeyond(m[2])
		if source == "" || extra == nil {
			return false
		}
		own := a.optionRow(m[1]+" credits "+source, "", b.Section, raws[i])
		more := &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Section: b.Section, Req: extra, Raw: raws[i]}}
		opts = append(opts, &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Section: b.Section, Raw: raws[i]}, children: []*compRow{own, more}})
	}
	raw := strings.Join(raws, "\n")
	if plainOptions(opts) {
		var nodes []*types.ReqNode
		for _, o := range opts {
			nodes = append(nodes, o.r.Req)
		}
		a.add(&compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Label: itemLabel(b), Section: b.Section, Req: programs.OrNode(nodes...), Raw: raw}}, nil)
		return true
	}
	var nodes []*types.ReqNode
	for _, o := range opts {
		nodes = append(nodes, a.reqNode(o))
	}
	a.addChoice(opts, itemLabel(b), b.Section, raw, choiceNode(1, nodes))
	return true
}

// poolBeyond reads "1.5 credits from CAR215H1, ..." as the credits of that pool
// beyond what the one item drawing on it claims; nil when nothing is beyond.
func (a *Assembler) poolBeyond(s string) *types.ReqNode {
	n, _ := parseCompletionText(s)
	if n == nil || n.Type != types.NodeCredits || n.Courses == nil || n.Designators != nil || n.Levels != nil || n.Exclude != nil || n.MaxAmount != nil {
		return nil
	}
	var claimed []float64
	for _, r := range a.items {
		p := r.r.Req
		if p != nil && p.Type == types.NodeCredits && p.Designators == nil && p.Levels == nil && grammar.SameSet(grammar.Dedupe(slices.Clone(p.Courses)), grammar.Dedupe(slices.Clone(n.Courses))) {
			claimed = append(claimed, p.Amount)
		}
	}
	if len(claimed) != 1 || n.Amount-claimed[0] < 0.5-1e-9 {
		return nil
	}
	n.Amount -= claimed[0]
	return n
}

// optionItem reads an item at the top whose options are in its text, and
// reports whether it did.
func (a *Assembler) optionItem(row *compRow, text string) bool {
	t := strings.TrimSuffix(strings.TrimSpace(a.clean(text)), ".")
	return a.restatedOr(row, t) || a.orElse(row, t) || a.categories(row, t) || a.chooseCourses(row, t) || a.inlineOr(row, text) || a.eitherItem(row, text)
}

// eitherItemRe matches "Either GGR491Y1 or one of GGR416H1, GGR492H1 and
// one additional 0.5 credit 400 level course from Group A (1.0 credit)".
var eitherItemRe = regexp.MustCompile(`(?i)^either\s+(.+?)\s+or\s+(.+?)\s+and\s+(?:one\s+|an\s+)?additional\s+(` + grammar.NumPat + `\s+credits?\s+.+?)\s*\(` + grammar.AmountPat + `\s+credits?\)$`)

// eitherItem reads "Either X or Y and one additional Z (N credits)" as a choice
// of X or a group of Y and Z, each N credits. Later approval or alternative
// sentences become alternatives on the group's parts.
func (a *Assembler) eitherItem(row *compRow, text string) bool {
	sents := SplitSentences(text)
	m := eitherItemRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(a.clean(sents[0])), "."))
	if m == nil {
		return false
	}
	amount, _ := strconv.ParseFloat(m[4], 64)
	x, _ := parseCompletionText(m[1])
	y, _ := parseCompletionText(m[2])
	if x == nil || hasText(x) || y == nil || hasText(y) {
		return false
	}
	vx, okx := nodeCredits(x)
	vy, oky := nodeCredits(y)
	vz := 0.0
	if am := amountLeadRe.FindStringSubmatch(m[3]); am != nil {
		vz, _ = strconv.ParseFloat(am[1], 64)
	}
	if !okx || !oky || math.Abs(vx-amount) > 1e-9 || math.Abs(vy+vz-amount) > 1e-9 {
		return false
	}
	one := &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Section: row.r.Section, Req: x, Raw: row.r.Raw}}
	parts := []*compRow{
		{r: &types.Requirement{Kind: types.RequirementAllocation, Req: y, Raw: row.r.Raw}},
		a.optionRow(m[3], "", "", row.r.Raw),
	}
	if hasText(parts[1].r.Req) {
		// "0.5 credit 400 level course from Group A": read later with the lists
		parts[1].r.Req, parts[1].children = TextNode(types.TextOther, m[3]), nil
	}
	two := &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Section: row.r.Section, Raw: row.r.Raw}, children: parts}
	for _, s := range sents[1:] {
		if a.IsApproval(s) || a.IsAlternative(s) {
			for _, p := range parts {
				a.alternative(p, s)
			}
			continue
		}
		a.notesTo(nil, a.sentence(s, nil, compCtx{}))
	}
	a.addChoice([]*compRow{one, two}, row.r.Label, row.r.Section, row.r.Raw, choiceNode(1, []*types.ReqNode{a.reqNode(one), a.reqNode(two)}))
	return true
}

// amountLeadRe matches the amount an item's text starts with: "0.5 credit
// 400 level course".
var amountLeadRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\b`)

// restatedOr drops a line under a list that restates an item's alternative,
// joining it to that item's raw.
func (a *Assembler) restatedOr(row *compRow, t string) bool {
	m := restatedOrRe.FindStringSubmatch(t)
	if m == nil || row.r.Label != "" {
		return false
	}
	want := strings.ToLower(rawWords(m[1]))
	for _, r := range a.items {
		if r.r.Kind == types.RequirementAllocation && strings.Contains(strings.ToLower(rawWords(r.r.Raw)), "or "+want) {
			addRaw(r, row.r.Raw)
			return true
		}
	}
	return false
}

// orElse reads orElseRe: the first reading is the total's pool; the second
// lets the group's courses join it, capped at the difference, all or none when
// over a half course ("9.0 and 1.0 from Group 4" isn't "9.5 and 0.5").
func (a *Assembler) orElse(row *compRow, t string) bool {
	m := orElseRe.FindStringSubmatch(t)
	if m == nil || a.total == nil || !strings.EqualFold(strings.TrimSpace(m[1]), strings.TrimSpace(m[3])) {
		return false
	}
	total, _ := strconv.ParseFloat(m[2], 64)
	part, _ := strconv.ParseFloat(m[4], 64)
	share, _ := strconv.ParseFloat(m[5], 64)
	names := listRefNames(m[6])
	if math.Abs(total-part-share) > 1e-9 || len(names) != 1 {
		return false
	}
	codes := a.listLineCodes(names[0])
	if codes == nil || !a.additional("Additional "+m[1]+" to a total of "+m[2]+" credits", row.r.Raw) {
		return false
	}
	pool := a.total.r.Req
	if pool.Type == types.NodeOr {
		pool = pool.Items[0]
	}
	if slices.ContainsFunc(codes, func(c string) bool { return !grammar.LevelMatches(pool.Levels, c) || slices.Contains(pool.Exclude, c) }) {
		a.total.r.Req = programs.OrNode(a.total.r.Req, TextNode(types.TextOther, t))
		return true
	}
	pool.Courses = grammar.Dedupe(append(pool.Courses, codes...))
	a.program = append(a.program, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Raw: row.r.Raw,
		Req: &types.ReqNode{Type: types.NodeCredits, Courses: slices.Clone(codes), MaxAmount: grammar.MaxAmount(share)}}})
	if share > 0.5+1e-9 {
		a.program = append(a.program, &compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Raw: row.r.Raw,
			Req: programs.OrNode(&types.ReqNode{Type: types.NodeCredits, Courses: slices.Clone(codes), MaxAmount: grammar.MaxAmount(0)},
				&types.ReqNode{Type: types.NodeCredits, Amount: share, Courses: slices.Clone(codes)})}})
	}
	return true
}

// listLineCodes returns the codes on the lines under the one title naming list
// key: "_Group 4 - Interdisciplinary:_" then "ANT348H1, GER338H1". Nil when not
// exactly one title names it.
func (a *Assembler) listLineCodes(key string) []string {
	var codes []string
	found := 0
	for k := 0; k < len(a.lines); k++ {
		if _, kk := listName(strings.Trim(a.lines[k], " :*_")); kk != key {
			continue
		}
		found++
		for k++; k < len(a.lines) && IsCodeList(a.lines[k]); k++ {
			codes = append(codes, grammar.CodeRe.FindAllString(a.lines[k], -1)...)
		}
		k--
	}
	if found != 1 {
		return nil
	}
	return grammar.Dedupe(codes)
}

// categories reads categoriesRe as choose{count} over the categories, each
// the or of its alternatives. The cheapest count of them must make the amount.
func (a *Assembler) categories(row *compRow, t string) bool {
	m := categoriesRe.FindStringSubmatch(t)
	if m == nil {
		return false
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	count, of := countOf(m[2]), countOf(m[3])
	var parts []string
	for _, p := range categoryMarkRe.Split(m[4], -1) {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, strings.TrimSuffix(p, ";"))
		}
	}
	if len(parts) != of || count < 1 || count >= of {
		return false
	}
	var nodes []*types.ReqNode
	var ws []float64
	for _, p := range parts {
		c := categoryRe.FindStringSubmatch(p)
		if c == nil {
			return false
		}
		var alts []*types.ReqNode
		for _, alt := range strings.Split(c[1], "/") {
			w := weightFromRe.FindStringSubmatch(strings.TrimSpace(alt))
			if w == nil || !IsCodeList(w[2]) {
				return false
			}
			v, _ := strconv.ParseFloat(w[1], 64)
			alts = append(alts, &types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: grammar.CodeRe.FindAllString(w[2], -1)})
		}
		n := programs.OrNode(alts...)
		v, ok := nodeCredits(n)
		if !ok {
			return false
		}
		nodes, ws = append(nodes, n), append(ws, v)
	}
	if math.Abs(programs.SumCheapest(ws, count)-amount) > 1e-9 {
		return false
	}
	row.r.Req = choiceNode(count, nodes)
	a.add(row, nil)
	return true
}

// chooseCourses reads chooseCoursesRe as choose{count} over the courses,
// when they are at the level and the range is the count's lightest to heaviest.
func (a *Assembler) chooseCourses(row *compRow, t string) bool {
	m := chooseCoursesRe.FindStringSubmatch(t)
	if m == nil || !IsCodeList(m[5]) {
		return false
	}
	count := countOf(m[1])
	codes := grammar.Dedupe(grammar.CodeRe.FindAllString(m[5], -1))
	if count >= len(codes) {
		return false
	}
	var ws []float64
	var nodes []*types.ReqNode
	for _, c := range codes {
		if m[2] != "" && strconv.Itoa(programs.CourseLevel(c)) != m[2] {
			return false
		}
		ws = append(ws, grammar.CourseWeight(c))
		nodes = append(nodes, &types.ReqNode{Type: types.NodeCourse, Code: c})
	}
	if m[3] != "" {
		lo, _ := strconv.ParseFloat(m[3], 64)
		hi, _ := strconv.ParseFloat(m[4], 64)
		neg := make([]float64, len(ws))
		for i, w := range ws {
			neg[i] = -w
		}
		heavy := -programs.SumCheapest(neg, count)
		if math.Abs(programs.SumCheapest(slices.Clone(ws), count)-lo) > 1e-9 || math.Abs(heavy-hi) > 1e-9 {
			return false
		}
	}
	row.r.Req = choiceNode(count, nodes)
	a.add(row, nil)
	return true
}

// inlineOr reads an item whose options are joined by an upper-case "OR", when
// the grammar can't read the whole: the options' or when each is a body alone,
// else a choice; collapseChoices may later make that choice one or.
func (a *Assembler) inlineOr(row *compRow, text string) bool {
	sents := SplitSentences(text)
	if len(sents) != 1 || strings.Count(text, "OR") == 0 {
		return false
	}
	s := strings.TrimSuffix(strings.TrimSpace(a.clean(sents[0])), ".")
	parts := inlineOrRe.Split(s, -1)
	if len(parts) < 2 || slices.ContainsFunc(parts, func(p string) bool { return !optionStartRe.MatchString(p) }) {
		return false
	}
	if n, _ := parseCompletionText(s); n != nil && !hasText(n) {
		return false // "ECO101H1 and ECO102H1, OR ECO105Y1": the grammar's
	}
	var opts []*compRow
	for _, p := range parts {
		opts = append(opts, a.optionRow(p, "", row.r.Section, p))
	}
	if plainOptions(opts) {
		var nodes []*types.ReqNode
		for _, o := range opts {
			nodes = append(nodes, o.r.Req)
		}
		row.r.Req = programs.OrNode(nodes...)
		a.add(row, nil)
		return true
	}
	var nodes []*types.ReqNode
	for _, o := range opts {
		nodes = append(nodes, a.reqNode(o))
	}
	a.addChoice(opts, row.r.Label, row.r.Section, row.r.Raw, choiceNode(1, nodes))
	return true
}

// collapseChoices makes a choice whose options are each a body alone, right
// before it, one allocation: the options' or (choose). It runs after namedLists.
func (a *Assembler) collapseChoices() {
	for i := 0; i < len(a.items); i++ {
		c := a.items[i]
		if c.r.Kind != types.RequirementChoice || (c.r.Req.Type != types.NodeOr && c.r.Req.Type != types.NodeChoose) {
			continue
		}
		var opts []*compRow
		for _, n := range c.r.Req.Items {
			ref := slices.IndexFunc(a.choiceRefs, func(x compChoiceRef) bool { return x.node == n })
			if ref < 0 || len(a.choiceRefs[ref].rows) != 1 {
				opts = nil
				break
			}
			opts = append(opts, a.choiceRefs[ref].rows[0])
		}
		k := len(opts)
		if k == 0 || i < k || !slices.Equal(a.items[i-k:i], opts) || slices.ContainsFunc(opts, func(o *compRow) bool {
			return o.r.Req == nil || len(o.children) > 0 || a.reqRefRow(o) || a.overlapRow(o)
		}) {
			continue
		}
		var nodes []*types.ReqNode
		var notes []string
		for _, o := range opts {
			nodes = append(nodes, o.r.Req)
			notes = append(notes, o.r.Notes...)
		}
		body := programs.OrNode(nodes...)
		if c.r.Req.Type == types.NodeChoose {
			body = choiceNode(c.r.Req.Count, nodes)
		}
		row := &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Label: c.r.Label, Section: c.r.Section, Req: body, Notes: append(c.r.Notes, notes...), Raw: c.r.Raw}}
		a.choiceRefs = slices.DeleteFunc(a.choiceRefs, func(x compChoiceRef) bool { return slices.Contains(c.r.Req.Items, x.node) })
		a.items = slices.Concat(a.items[:i-k], []*compRow{row}, a.items[i+1:])
		i -= k
	}
}

// reqRefRow reports whether a pool refers to row.
func (a *Assembler) reqRefRow(row *compRow) bool {
	return slices.ContainsFunc(a.reqRefs, func(ref compReqRef) bool { return slices.Contains(ref.rows, row) })
}

// overlapRow reports whether row is one of a pair that may share courses.
func (a *Assembler) overlapRow(row *compRow) bool {
	return slices.ContainsFunc(a.overlaps, func(o [2]*compRow) bool { return o[0] == row || o[1] == row })
}

// choiceRow reports whether a choice picks row.
func (a *Assembler) choiceRow(row *compRow) bool {
	return slices.ContainsFunc(a.choiceRefs, func(ref compChoiceRef) bool { return slices.Contains(ref.rows, row) })
}

// restatedItems drops rows that restate another row's alternative or a
// choose's count, joining their sentence to that row's raw: "If a student has
// not completed STA130H1 then ...", "If the research course LMP405Y1 is taken".
func (a *Assembler) restatedItems() {
	var all []*compRow
	walkRows(a.items, func(r *compRow) {
		all = append(all, r)
	})
	target := func(row *compRow) *compRow {
		if row.r.Req == nil || row.r.Req.Type != types.NodeText || len(row.children) > 0 {
			return nil
		}
		t := strings.TrimSuffix(strings.TrimSpace(a.clean(row.r.Req.Text)), ".")
		if m := ifNotCompletedRe.FindStringSubmatch(t); m != nil {
			alt, _ := parseCompletionText(m[2])
			if alt == nil || hasText(alt) {
				return nil
			}
			for _, r := range all {
				found := false
				grammar.WalkReq(r.r.Req, func(n *types.ReqNode) {
					found = found || n.Type == types.NodeOr && slices.ContainsFunc(n.Items, func(x *types.ReqNode) bool { return x.Type == types.NodeCourse && x.Code == m[1] }) &&
						slices.ContainsFunc(n.Items, func(x *types.ReqNode) bool { return sameNode(x, alt) })
				})
				if found && r != row {
					return r
				}
			}
		}
		if m := chooseRestatedRe.FindStringSubmatch(t); m != nil {
			for _, r := range all {
				n := r.r.Req
				if n != nil && n.Type == types.NodeChoose && n.Count-1 == countOf(m[2]) &&
					slices.ContainsFunc(n.Items, func(x *types.ReqNode) bool { return x.Type == types.NodeCourse && x.Code == m[1] }) {
					return r
				}
			}
		}
		return nil
	}
	drop := func(rs []*compRow) []*compRow {
		return slices.DeleteFunc(rs, func(r *compRow) bool {
			t := target(r)
			if t == nil || a.reqRefRow(r) || a.overlapRow(r) || a.choiceRow(r) {
				return false // another row names it: dropped, it would be numbered 0
			}
			addRaw(t, r.r.Raw)
			return true
		})
	}
	a.items, a.program = drop(a.items), drop(a.program)
}

// ChosenRows returns the numbers of the rows c's choices pick among: they count
// only through their choice.
func ChosenRows(c *types.Completion) map[int]bool {
	out := map[int]bool{}
	for _, r := range c.Requirements {
		if r != nil && r.Kind == types.RequirementChoice {
			grammar.WalkReq(r.Req, func(n *types.ReqNode) {
				if n.Type == types.NodeRequirement {
					for _, k := range n.Requirements {
						out[k] = true
					}
				}
			})
		}
	}
	return out
}

// choiceCredits returns the fewest credits a choice's options need.
func choiceCredits(n *types.ReqNode, c *types.Completion, kids map[int][]*types.Requirement) float64 {
	switch n.Type {
	case types.NodeRequirement:
		best := math.Inf(1)
		for _, k := range n.Requirements {
			if k >= 1 && k <= len(c.Requirements) {
				v, _ := rowCredits(c.Requirements[k-1], kids)
				best = min(best, v)
			}
		}
		if math.IsInf(best, 1) {
			return 0
		}
		return best
	case types.NodeAnd:
		sum := 0.0
		for _, it := range n.Items {
			sum += choiceCredits(it, c, kids)
		}
		return sum
	case types.NodeOr, types.NodeChoose:
		var ws []float64
		for _, it := range n.Items {
			ws = append(ws, choiceCredits(it, c, kids))
		}
		return programs.SumCheapest(ws, max(1, n.Count))
	}
	return 0
}

// compArea is one area of an areaOptions item: its name and its line.
type compArea struct {
	name, other string // other: the name it corresponds to in the other item
	b           *Block
}

// areaOptions reads two items over chosen areas, from bs[0], as a choice of
// count areas, each a group of its two lines, and returns how many blocks it
// read. An area the sentence doesn't name is lenient-only: and(credits, text).
func (a *Assembler) areaOptions(bs []*Block) int {
	type areaItem struct {
		b      *Block
		amount float64
		areas  []compArea
		notes  []*Block
	}
	read := func(i int) (it areaItem, next int, ok bool) {
		b := bs[i]
		m := perAreaRe.FindStringSubmatch(strings.TrimSpace(a.clean(b.Text)))
		if m == nil || !blkNumbered(b) || !strings.EqualFold(m[3], "language") {
			return it, 0, false
		}
		it.b = b
		it.amount, _ = strconv.ParseFloat(m[4], 64)
		if total, _ := strconv.ParseFloat(m[1], 64); math.Abs(total-it.amount*float64(countOf(m[2]))) > 1e-9 {
			return it, 0, false // "4.0 credits ... (2.0 credits per area)" over two areas
		}
		lines, next := b.Children, i+1
		if len(lines) == 0 {
			for next < len(bs) && bs[next].Kind == blockLabelled && len(bs[next].Children) == 0 {
				next++
			}
			lines = bs[i+1 : next]
		}
		for _, c := range lines {
			switch {
			case BlkNoteLabelRe.MatchString(c.Label):
				it.notes = append(it.notes, c)
			case c.Kind == blockLabelled && len(c.Children) == 0 && grammar.CodeRe.MatchString(c.Text) && len(it.notes) == 0:
				ar := compArea{name: strings.TrimSpace(areaParenRe.ReplaceAllString(programs.StripEmphasis(c.Label), "")), b: c}
				if cm := correspondsRe.FindStringSubmatch(programs.StripEmphasis(c.Label)); cm != nil {
					ar.name, ar.other = strings.TrimSpace(cm[1]), strings.TrimSpace(cm[2])
				}
				it.areas = append(it.areas, ar)
			default:
				return it, 0, false
			}
		}
		return it, next, len(it.areas) >= 2
	}
	first, next, ok := read(0)
	if !ok {
		return 0
	}
	for next < len(bs) && BlkNoteLabelRe.MatchString(bs[next].Label) && len(bs[next].Children) == 0 {
		first.notes = append(first.notes, bs[next])
		next++
	}
	if next >= len(bs) {
		return 0
	}
	second, end, ok := read(next)
	if !ok || len(second.areas) != len(first.areas) {
		return 0
	}
	// the sentence that sets the choice, read already as a row of text
	var sentence *compRow
	var count int
	var named []string
	for _, rs := range [][]*compRow{a.program, a.items} {
		for _, r := range rs {
			if r.r.Req == nil || r.r.Req.Type != types.NodeText || len(r.children) > 0 {
				continue
			}
			if m := chooseAreasRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(a.clean(r.r.Req.Text)), ".")); m != nil && strings.EqualFold(m[2], "language") {
				sentence, count = r, countOf(m[1])
				for _, n := range strings.Split(m[3], ",") {
					named = append(named, strings.ToLower(strings.TrimSpace(n)))
				}
			}
		}
	}
	if sentence == nil || count < 2 || count >= len(first.areas) {
		return 0
	}
	// each area of the first item, with its line in the second
	pair := func(x compArea) int {
		return slices.IndexFunc(second.areas, func(y compArea) bool {
			return strings.EqualFold(y.name, x.name) || x.other != "" && strings.EqualFold(y.name, x.other) || y.other != "" && strings.EqualFold(y.other, x.name)
		})
	}
	var groups []*compRow
	var firsts, seconds []*compRow
	for _, x := range first.areas {
		k := pair(x)
		if k < 0 {
			return 0
		}
		g := &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Label: x.name, Section: first.b.Section, Raw: a.line(x.b) + "\n" + a.line(second.areas[k].b)}}
		for _, part := range []struct {
			it  areaItem
			b   *Block
			out *[]*compRow
		}{{first, x.b, &firsts}, {second, second.areas[k].b, &seconds}} {
			row := a.optionRow(fmtAmount(part.it.amount)+" credits from: "+part.b.Text, "", part.it.b.Section, a.line(part.b))
			g.children = append(g.children, row)
			*part.out = append(*part.out, row)
		}
		if !slices.Contains(named, strings.ToLower(x.name)) {
			// two readings: the sentence's areas, or every area listed
			p := g.children[0]
			p.r.Req = programs.AndNode(p.r.Req, TextNode(types.TextOther, sentence.r.Req.Text))
		}
		groups = append(groups, g)
	}
	var notes []string // the items' notes: the choice's
	for _, part := range []struct {
		it   areaItem
		rows []*compRow
	}{{first, firsts}, {second, seconds}} {
		for _, nb := range part.it.notes {
			for _, s := range SplitSentences(nb.Text) {
				if a.IsApproval(s) || a.IsAlternative(s) {
					kind := altKind(s)
					if a.IsApproval(s) {
						kind = types.TextPermission
					}
					for _, r := range part.rows {
						r.r.Req = programs.OrNode(r.r.Req, TextNode(kind, s))
						addRaw(r, s)
					}
					continue
				}
				notes = append(notes, a.sentence(s, nil, compCtx{notes: true})...)
			}
		}
	}
	var nodes []*types.ReqNode
	for _, g := range groups {
		nodes = append(nodes, a.reqNode(g))
	}
	raw := sentence.r.Raw + "\n" + a.line(first.b) + "\n" + a.line(second.b)
	a.program = slices.DeleteFunc(a.program, func(r *compRow) bool { return r == sentence })
	a.items = slices.DeleteFunc(a.items, func(r *compRow) bool { return r == sentence })
	a.addChoice(groups, "", first.b.Section, raw, choiceNode(count, nodes))
	a.items[len(a.items)-1].r.Notes = notes
	return end
}
