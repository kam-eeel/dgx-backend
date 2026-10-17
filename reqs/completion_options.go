package reqs

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Options and choices (design 3.4): "Choose one of the options below:" and
// its "Option A: ..." lines, "At least two of the following four options:"
// over bullets, "EITHER ... OR ..." over lines, "X **OR** Y" and "or else"
// in an item, "in at least two of the three following categories: (1) ...".
//
// Options that are each a body of courses are one allocation: the or (or
// choose) of the bodies. Otherwise each option is an allocation under row 1,
// a group (req null) with child allocations when it has parts, and a choice
// row at the top, numbered right after them, picks among them: an or or
// choose over requirement nodes (numberChoices), so that the options the
// student didn't choose count nothing (design 5.4). Options that are rows of
// text naming lists are read with the lists (namedLists), and become one
// allocation's or if each is then a body alone (collapseChoices).

var (
	// complOptionLabelRe: "Option A", an option, never a list.
	complOptionLabelRe = regexp.MustCompile(`(?i)^option\s+\w+\b`)
	// complOptionsLeadRe: "Choose one of the options below:".
	complOptionsLeadRe = regexp.MustCompile(`(?i)^(?:choose|complete|select)\s+(?:one|1)\s+of\s+the\s+(?:following\s+)?options?\b[^:]*:?$`)
	// complCountOptionsLeadRe: "At least two of the following four options:",
	// over bullets that are the options.
	complCountOptionsLeadRe = regexp.MustCompile(`(?i)^(?:at\s+least\s+)?(one|two|three|four|[1-4])\s+of\s+the\s+following\s+(?:(two|three|four|five|six|[2-6])\s+)?options\s*:$`)
	// complAboveOptionsRe: an option that includes others: "LMP305Y1 and one
	// of the above four options".
	complAboveOptionsRe = regexp.MustCompile(`(?i)^(.+?)\s+and\s+one\s+of\s+the\s+above(?:\s+(two|three|four|five|six|[2-6]))?\s+options$`)
	// complEitherRe: the "EITHER" an item's first option starts with.
	complEitherRe = regexp.MustCompile(`^(?:EITHER|Either)\s+`)
	// complIfTakingRe: an option that trades credits with another item's
	// pool: "2.0 credits if taking 1.5 credits from CAR215H1, ...".
	complIfTakingRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+if\s+taking\s+(\d+(?:\.\d+)?\s+credits?\s+from\s+.+)$`)
	// complAmountSourceRe: an option's amount and where it comes from: "2.5
	// credits from Group A or B".
	complAmountSourceRe = regexp.MustCompile(`(?i)^\d+(?:\.\d+)?\s+credits?\s+(from\s+.+)$`)
	// complInlineOrRe: "**OR**", "OR" between an item's options (upper case:
	// a lower-case "or" is the grammar's).
	complInlineOrRe = regexp.MustCompile(`\s*,?\s+OR\s+`)
	// complOptionStartRe: what an option starts with: an amount or a code.
	complOptionStartRe = regexp.MustCompile(`^(?:\d+(?:\.\d+)?\s+(?:additional\s+)?credits?\b|` + codePat + `)`)
	// complOrElseRe: "Additional philosophy courses, to a total of 7.0
	// credits, or else additional philosophy courses to a total of 6.5
	// credits, plus one 0.5 credit from Group 4 (Interdisciplinary)".
	complOrElseRe = regexp.MustCompile(`(?i)^additional\s+(.+?),?\s+to\s+a\s+total\s+of\s+(\d+(?:\.\d+)?)\s+credits?,?\s+or\s+else\s+additional\s+(.+?),?\s+to\s+a\s+total\s+of\s+(\d+(?:\.\d+)?)\s+credits?,?\s+(?:plus|and)\s+(?:one\s+)?(\d+(?:\.\d+)?)\s+credits?\s+from\s+(.+?)(?:\s*\([^()]*\))?\.?$`)
	// complCategoriesRe: "1.0 to 1.5 credits in at least two of the three
	// following categories: (1) ...; (2) ...; and/or (3) ...".
	complCategoriesRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)(?:\s+to\s+\d+(?:\.\d+)?)?\s+credits?\s+in\s+at\s+least\s+(one|two|three|four|[1-4])\s+of\s+the\s+(two|three|four|five|six|[2-6])\s+following\s+(?:categories|areas|groups)\s*:\s*(.+)$`)
	// complCategoryMarkRe: a category's number: "(1)", "; and/or (3)".
	complCategoryMarkRe = regexp.MustCompile(`\s*;?\s*(?:and/or\s+|and\s+|or\s+)?\(\d\)\s*`)
	// complCategoryRe: a category: "one field course (0.5 credit) from
	// EEB401H1, ...", "one independent research course (0.5 credit) from
	// EEB497H1/ (1.0 credit) from EEB498Y1".
	complCategoryRe = regexp.MustCompile(`(?i)^(?:one|an?)\s+[\w\s-]+?\s+(\(\d+(?:\.\d+)?\s+credits?\)\s+from\s+.+)$`)
	// complWeightFromRe: "(0.5 credit) from EEB401H1, EEB403H1".
	complWeightFromRe = regexp.MustCompile(`(?i)^\((\d+(?:\.\d+)?)\s+credits?\)\s+from\s+(.+?)$`)
	// complChooseCoursesRe: "Students should choose four of the following
	// 400-level courses (2.0-2.5 credits): LMP405Y1, LMP410H1, ...".
	complChooseCoursesRe = regexp.MustCompile(`(?i)^(?:students\s+(?:should|must)\s+)?(?:choose|select|take)\s+(two|three|four|five|six|[2-6])\s+of\s+the\s+following\s+(?:([1-4]00)-level\s+)?courses\s*(?:\((\d+(?:\.\d+)?)\s*(?:-|–|to)\s*(\d+(?:\.\d+)?)\s+credits\))?\s*:\s*(.+)$`)
	// complChooseRestatedRe: a note that restates such a count: "If the
	// research course LMP405Y1 is taken, an additional three 400-level
	// courses from the list are still required (totalling 2.5 credits)".
	complChooseRestatedRe = regexp.MustCompile(`(?i)^if\s+(?:the\s+)?[\w\s-]*?\b(` + codePat + `)\s+is\s+taken,\s+an\s+additional\s+(one|two|three|four|five|[1-5])\s+(?:[1-4]00-level\s+)?courses\s+from\s+the\s+list\s+are\s+still\s+required\b`)
	// complIfNotCompletedRe: an item that restates another row's
	// alternative: "If a student has not completed STA130H1 then an
	// additional 0.5 credit 300+ level STA course that is not used towards
	// any other program requirement must be completed".
	complIfNotCompletedRe = regexp.MustCompile(`(?i)^if\s+(?:a\s+)?students?\s+(?:has|have)\s+not\s+completed\s+(` + codePat + `)\s*,?\s+then\s+an\s+additional\s+(.+?)\s+that\s+is\s+not\s+used\s+towards?\s+any\s+other\s+program\s+requirement\s+must\s+be\s+completed$`)
	// complRestatedOrRe: a line under a list that restates an item's
	// alternative: "OR 2.0 credits in any major African language approved by
	// the Program Committee".
	complRestatedOrRe = regexp.MustCompile(`^OR\s+(.+?)\.?$`)
)

var (
	// complPerAreaRe: an item over areas the student chooses: "4.0 credits of
	// language coursework from each of the two chosen language areas (2.0
	// credits per area).", "4.0 credits from culture courses corresponding to
	// the student’s two chosen language areas (2.0 credits per area):".
	complPerAreaRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+.+?\s+(?:from\s+each\s+of|corresponding\s+to)\s+the\s+(?:student.s\s+)?(two|three|[23])\s+chosen\s+(\w+)\s+areas\s*\((\d+(?:\.\d+)?)\s+credits?\s+per\s+area\)\s*[.:]?$`)
	// complChooseAreasRe: the choice of areas: "Students must choose any two
	// of the available language areas (Czech and Slovak, Estonian, ...)".
	complChooseAreasRe = regexp.MustCompile(`(?i)^students\s+must\s+choose\s+(?:any\s+)?(two|three|[23])\s+of\s+the\s+(?:available\s+)?(\w+)\s+areas\s*\(([^()]+)\)$`)
	// complCorrespondsRe: an area named differently in another list: "Yiddish
	// (corresponds to the Jewish Cultures in Eastern Europe area)", "Jewish
	// Cultures in Eastern Europe (corresponds to the Yiddish language area)".
	complCorrespondsRe = regexp.MustCompile(`(?i)^(.+?)\s*\(corresponds\s+to\s+the\s+(.+?)\s+(?:language\s+)?area\)$`)
	// complAreaParenRe: what an area's name adds in brackets: "South Slavic
	// (Bosnian, Croatian, Serbian, Macedonian)".
	complAreaParenRe = regexp.MustCompile(`\s*\([^()]*\)$`)
)

// compChoiceRef is a requirement node of a choice row, numbered once the rows
// are (numberChoices).
type compChoiceRef struct {
	node *types.ReqNode
	rows []*compRow
}

// reqNode returns a requirement node over rows, numbered in finish.
func (a *compAssembler) reqNode(rows ...*compRow) *types.ReqNode {
	n := &types.ReqNode{Type: types.NodeRequirement}
	a.choiceRefs = append(a.choiceRefs, compChoiceRef{node: n, rows: rows})
	return n
}

// numberChoices sets the numbers of the choice rows' requirement nodes once
// the rows are numbered.
func (a *compAssembler) numberChoices() {
	for _, ref := range a.choiceRefs {
		ref.node.Requirements = nil
		for _, r := range ref.rows {
			ref.node.Requirements = append(ref.node.Requirements, r.r.Number)
		}
	}
}

// choiceNode is an or (count 0 or 1) or a choose over items, without the
// flattening and deduplication of orNode: requirement nodes have no numbers
// yet, so they would all look the same.
func choiceNode(count int, items []*types.ReqNode) *types.ReqNode {
	if count > 1 {
		return &types.ReqNode{Type: types.NodeChoose, Count: count, Items: items}
	}
	return &types.ReqNode{Type: types.NodeOr, Items: items}
}

// addChoice adds the options, allocations under row 1, and the choice row
// over them (body), numbered right after them at the top.
func (a *compAssembler) addChoice(opts []*compRow, label, section, raw string, body *types.ReqNode) {
	a.items = append(a.items, opts...)
	a.items = append(a.items, &compRow{r: &types.Requirement{Kind: types.RequirementChoice, Label: label, Section: section, Req: body, Raw: raw}})
}

// optionRow returns an allocation for one option, its body read as an
// item's (itemBody) with the rules it states under it.
func (a *compAssembler) optionRow(text, label, section, raw string) *compRow {
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

// plainOptions reports whether every option is a body of courses alone:
// no text, no rules under it.
func plainOptions(opts []*compRow) bool {
	return !slices.ContainsFunc(opts, func(o *compRow) bool { return o.r.Req == nil || hasText(o.r.Req) || len(o.children) > 0 })
}

// optionBlocks reads options set out over lines, from bs[0], and returns how
// many blocks it read (0: none):
//   - "Choose one of the options below:" and the "Option A: ..." lines under
//     or after it; "At least two of the following four options:" and the
//     bullets under it;
//   - "5. EITHER 2.5 credits from Group A or B OR" and the lines after it,
//     each but the last ending in "OR" (two lines without "EITHER" are the
//     ", OR" join of blocks).
func (a *compAssembler) optionBlocks(bs []*compBlock, ctx compCtx) int {
	b := bs[0]
	if b.kind == blockHeading {
		return 0
	}
	text := strings.TrimSpace(b.text)
	if complOptionsLeadRe.MatchString(text) {
		if len(b.children) >= 2 && optionLines(b.children) == len(b.children) {
			a.options(b, b.children, 1, ctx)
			return 1
		}
		if n := optionLines(bs[1:]); n >= 2 && len(b.children) == 0 {
			a.options(b, bs[1:1+n], 1, ctx)
			return n + 1
		}
		return 0
	}
	if m := complCountOptionsLeadRe.FindStringSubmatch(text); m != nil && len(b.children) >= 2 {
		if m[2] != "" && countOf(m[2]) != len(b.children) || slices.ContainsFunc(b.children, func(c *compBlock) bool { return len(c.children) > 0 }) {
			return 0
		}
		a.options(b, b.children, countOf(m[1]), ctx)
		return 1
	}
	if ctx.owner == nil && blkNumbered(b) {
		if n := a.areaOptions(bs); n > 0 {
			return n
		}
	}
	if ctx.owner != nil || !complTrailingCommaOrRe.MatchString(b.text) || len(b.children) > 0 {
		return 0
	}
	n := 1
	for n < len(bs) && complTrailingCommaOrRe.MatchString(bs[n-1].text) && bs[n].kind != blockHeading && len(bs[n].children) == 0 && !blkNumbered(bs[n]) {
		n++
	}
	if n < 2 || n == 2 && !complEitherRe.MatchString(text) || complTrailingCommaOrRe.MatchString(bs[n-1].text) {
		return 0
	}
	if !a.eitherLines(bs[:n]) {
		return 0
	}
	return n
}

// optionLines returns how many of bs, from the first, are "Option X:" lines.
func optionLines(bs []*compBlock) int {
	n := 0
	for _, c := range bs {
		if !complOptionLabelRe.MatchString(c.label) || len(c.children) > 0 {
			break
		}
		n++
	}
	return n
}

// options reads a lead over option lines ("Choose one of the options
// below:" then "Option A: Immunology: ( IMM341H1, IMM351H1)", ...; "At least
// two of the following four options:" then "- CHM217H1", ...): one
// allocation, the options' or (choose when count > 1), when each reads as
// courses. An option that includes others ("Option E: Research: LMP305Y1
// and one of the above four options") makes the options rows of their own
// and a choice over them: or(A, B, C, D, and(E, any of A-D)). Otherwise the
// lead is text with the options as its notes.
func (a *compAssembler) options(b *compBlock, optLines []*compBlock, count int, ctx compCtx) {
	var opts []*compRow
	above := map[int]int{} // option index -> how many options before it it includes
	ok := true
	for i, c := range optLines {
		text := c.text
		if j := strings.LastIndex(text, ":"); j >= 0 {
			text = text[j+1:] // "Immunology: ( IMM341H1, IMM351H1)"
		}
		text = strings.TrimSpace(text)
		if m := complAboveOptionsRe.FindStringSubmatch(text); m != nil && i > 0 {
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
		if complOptionLabelRe.MatchString(c.label) {
			label = c.label
		}
		opts = append(opts, &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Label: label, Section: c.section, Req: o, Raw: a.line(c)}})
	}
	if ok && count < len(opts) && len(above) == 0 {
		var nodes []*types.ReqNode
		raw := a.line(b)
		for _, o := range opts {
			nodes = append(nodes, o.r.Req)
			raw += "\n" + o.r.Raw
		}
		body := orNode(nodes...)
		if count > 1 {
			body = &types.ReqNode{Type: types.NodeChoose, Count: count, Items: nodes}
		}
		a.add(&compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Label: itemLabel(b), Section: b.section, Req: body, Raw: raw}}, ctx.owner)
		return
	}
	if ok && count == 1 && ctx.owner == nil {
		// Option E includes one of A-D: or(A, ..., D, and(E, any of A-D)).
		var nodes []*types.ReqNode
		for i, o := range opts {
			n := a.reqNode(o)
			if k, ok := above[i]; ok {
				n = &types.ReqNode{Type: types.NodeAnd, Items: []*types.ReqNode{n, a.reqNode(opts[:k]...)}}
			}
			nodes = append(nodes, n)
		}
		a.addChoice(opts, itemLabel(b), b.section, a.line(b), choiceNode(1, nodes))
		return
	}
	row := &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Label: itemLabel(b), Section: b.section, Req: textNode(types.TextOther, b.text), Raw: a.line(b)}}
	for _, c := range optLines {
		row.r.Raw += "\n" + a.line(c)
		row.r.Notes = append(row.r.Notes, cleanNote(blockLine(c)))
	}
	a.add(row, ctx.owner)
}

// eitherLines reads an item whose options are lines, each but the last
// ending in "OR" ("5. EITHER 2.5 credits from Group A or B OR", "2.0
// credits if taking 1.5 credits from CAR215H1, ... OR", "1.5 credits if
// taking 2.0 credits from CAR215H1, ..."): one allocation, the options' or,
// when each reads as courses; else each option is a row and a choice picks
// one. An option "N credits if taking M credits from <pool>" takes N credits
// from the first option's source and, beside them, the credits of the pool
// beyond what the item drawing on that pool claims: a group of the two
// (design 3.4). It reports false, reading nothing, for options it can't
// read.
func (a *compAssembler) eitherLines(bs []*compBlock) bool {
	var parts []string
	var raws []string
	for _, c := range bs {
		t := complTrailingCommaOrRe.ReplaceAllString(strings.TrimSpace(c.text), "")
		parts = append(parts, strings.TrimSpace(complEitherRe.ReplaceAllString(t, "")))
		raws = append(raws, a.line(c))
	}
	source := ""
	if m := complAmountSourceRe.FindStringSubmatch(parts[0]); m != nil {
		source = m[1] // "from Group A or B"
	}
	b := bs[0]
	var opts []*compRow
	for i, p := range parts {
		m := complIfTakingRe.FindStringSubmatch(p)
		if m == nil {
			opts = append(opts, a.optionRow(p, "", b.section, raws[i]))
			continue
		}
		extra := a.poolBeyond(m[2])
		if source == "" || extra == nil {
			return false
		}
		own := a.optionRow(m[1]+" credits "+source, "", b.section, raws[i])
		more := &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Section: b.section, Req: extra, Raw: raws[i]}}
		opts = append(opts, &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Section: b.section, Raw: raws[i]}, children: []*compRow{own, more}})
	}
	raw := strings.Join(raws, "\n")
	if plainOptions(opts) {
		var nodes []*types.ReqNode
		for _, o := range opts {
			nodes = append(nodes, o.r.Req)
		}
		a.add(&compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Label: itemLabel(b), Section: b.section, Req: orNode(nodes...), Raw: raw}}, nil)
		return true
	}
	var nodes []*types.ReqNode
	for _, o := range opts {
		nodes = append(nodes, a.reqNode(o))
	}
	a.addChoice(opts, itemLabel(b), b.section, raw, choiceNode(1, nodes))
	return true
}

// poolBeyond reads "1.5 credits from CAR215H1, CAR220H1, ..." as the credits
// of that pool beyond what the item drawing on it already claims ("2. 1.0
// credit from CAR215H1, CAR220H1, ..."): credits{0.5, the pool}. Nil when
// the pool isn't courses, no item or several draw on it, or nothing is
// beyond.
func (a *compAssembler) poolBeyond(s string) *types.ReqNode {
	n, _ := parseCompletionText(s)
	if n == nil || n.Type != types.NodeCredits || n.Courses == nil || n.Designators != nil || n.Levels != nil || n.Exclude != nil || n.MaxAmount != nil {
		return nil
	}
	var claimed []float64
	for _, r := range a.items {
		p := r.r.Req
		if p != nil && p.Type == types.NodeCredits && p.Designators == nil && p.Levels == nil && sameSet(dedupe(slices.Clone(p.Courses)), dedupe(slices.Clone(n.Courses))) {
			claimed = append(claimed, p.Amount)
		}
	}
	if len(claimed) != 1 || n.Amount-claimed[0] < 0.5-1e-9 {
		return nil
	}
	n.Amount -= claimed[0]
	return n
}

// optionItem reads an item (no lines under it, at the top) whose options are
// in its text, and reports whether it did: "or else" (orElse), "X **OR** Y"
// (inlineOr), "in at least two of the three following categories"
// (categories), "Students should choose four of the following 400-level
// courses" (chooseCourses), and a line that restates another item's
// alternative (restatedOr).
func (a *compAssembler) optionItem(row *compRow, text string) bool {
	t := strings.TrimSuffix(strings.TrimSpace(a.clean(text)), ".")
	return a.restatedOr(row, t) || a.orElse(row, t) || a.categories(row, t) || a.chooseCourses(row, t) || a.inlineOr(row, text) || a.eitherItem(row, text)
}

// complEitherItemRe: "Either GGR491Y1 or one of GGR416H1, GGR492H1, GGR497H1
// and one additional 0.5 credit 400 level course from Group A or Group B
// (1.0 credit)".
var complEitherItemRe = regexp.MustCompile(`(?i)^either\s+(.+?)\s+or\s+(.+?)\s+and\s+(?:one\s+|an\s+)?additional\s+(\d+(?:\.\d+)?\s+credits?\s+.+?)\s*\((\d+(?:\.\d+)?)\s+credits?\)$`)

// eitherItem reads an item "Either X or Y and one additional Z (N credits)"
// whose first sentence is that: X, or a group of Y and Z (Z may name lists,
// and may share courses with Y), and a choice of one; each reads as N
// credits. The item's other sentences that are approvals or other ways to
// meet it ("Up to one 0.5 credit can be used from GGR493Y1, ... approved by
// department") are alternatives on the group's parts, the ones the student
// chooses among.
func (a *compAssembler) eitherItem(row *compRow, text string) bool {
	sents := splitComplSentences(text)
	m := complEitherItemRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(a.clean(sents[0])), "."))
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
	if am := complAmountLeadRe.FindStringSubmatch(m[3]); am != nil {
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
		// "0.5 credit 400 level course from Group A or Group B": read with
		// the lists (namedLists).
		parts[1].r.Req, parts[1].children = textNode(types.TextOther, m[3]), nil
	}
	two := &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Section: row.r.Section, Raw: row.r.Raw}, children: parts}
	for _, s := range sents[1:] {
		if a.isApproval(s) || a.isAlternative(s) {
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

// complAmountLeadRe: the amount an item's text starts with: "0.5 credit 400
// level course from Group A or Group B".
var complAmountLeadRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\b`)

// restatedOr drops a line that restates an item's alternative, under the
// list it is an alternative to ("Group C" ... "OR 2.0 credits in any major
// African language approved by the Program Committee", which item 7 says):
// its line joins the item's raw.
func (a *compAssembler) restatedOr(row *compRow, t string) bool {
	m := complRestatedOrRe.FindStringSubmatch(t)
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

// orElse reads "Additional philosophy courses, to a total of 7.0 credits, or
// else additional philosophy courses to a total of 6.5 credits, plus one 0.5
// credit from Group 4 (Interdisciplinary)": the first reading is the
// total's pool (additional); the second lets the group's courses (the lines
// under its title) join that pool, at most the difference (a limit under
// row 1), and when that is more than one half course, all of it or none (a
// constraint: "9.0 credits and 1.0 credit from Group 4" isn't 9.5 and 0.5).
func (a *compAssembler) orElse(row *compRow, t string) bool {
	m := complOrElseRe.FindStringSubmatch(t)
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
	if slices.ContainsFunc(codes, func(c string) bool { return !levelMatches(pool.Levels, c) || slices.Contains(pool.Exclude, c) }) {
		a.total.r.Req = orNode(a.total.r.Req, textNode(types.TextOther, t))
		return true
	}
	pool.Courses = dedupe(append(pool.Courses, codes...))
	a.program = append(a.program, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Raw: row.r.Raw,
		Req: &types.ReqNode{Type: types.NodeCredits, Courses: slices.Clone(codes), MaxAmount: maxAmount(share)}}})
	if share > 0.5+1e-9 {
		a.program = append(a.program, &compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Raw: row.r.Raw,
			Req: orNode(&types.ReqNode{Type: types.NodeCredits, Courses: slices.Clone(codes), MaxAmount: maxAmount(0)},
				&types.ReqNode{Type: types.NodeCredits, Amount: share, Courses: slices.Clone(codes)})}})
	}
	return true
}

// listLineCodes returns the codes of the list with key (complListKey) as
// its lines set it out: the line of codes under its title ("_Group 4 -
// Interdisciplinary:_" then "ANT348H1, GER338H1, ..."). Nil when no title
// or several name it, or a line under it isn't only codes.
func (a *compAssembler) listLineCodes(key string) []string {
	var codes []string
	found := 0
	for k := 0; k < len(a.lines); k++ {
		if _, kk := listName(strings.Trim(a.lines[k], " :*_")); kk != key {
			continue
		}
		found++
		for k++; k < len(a.lines) && isCodeList(a.lines[k]); k++ {
			codes = append(codes, codeRe.FindAllString(a.lines[k], -1)...)
		}
		k--
	}
	if found != 1 {
		return nil
	}
	return dedupe(codes)
}

// categories reads "1.0 to 1.5 credits in at least two of the three
// following categories: (1) one field course (0.5 credit) from EEB401H1,
// ...; (2) one seminar (0.5 credit) from EEB491H1, EEB495H1; and/or (3) one
// independent research course (0.5 credit) from EEB497H1/ (1.0 credit) from
// EEB498Y1": choose{2} over the categories, each the credits of its
// courses, an or of its alternatives. The fewest credits of the count must
// be the amount.
func (a *compAssembler) categories(row *compRow, t string) bool {
	m := complCategoriesRe.FindStringSubmatch(t)
	if m == nil {
		return false
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	count, of := countOf(m[2]), countOf(m[3])
	var parts []string
	for _, p := range complCategoryMarkRe.Split(m[4], -1) {
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
		c := complCategoryRe.FindStringSubmatch(p)
		if c == nil {
			return false
		}
		var alts []*types.ReqNode
		for _, alt := range strings.Split(c[1], "/") {
			w := complWeightFromRe.FindStringSubmatch(strings.TrimSpace(alt))
			if w == nil || !isCodeList(w[2]) {
				return false
			}
			v, _ := strconv.ParseFloat(w[1], 64)
			alts = append(alts, &types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: codeRe.FindAllString(w[2], -1)})
		}
		n := orNode(alts...)
		v, ok := nodeCredits(n)
		if !ok {
			return false
		}
		nodes, ws = append(nodes, n), append(ws, v)
	}
	if math.Abs(sumCheapest(ws, count)-amount) > 1e-9 {
		return false
	}
	row.r.Req = choiceNode(count, nodes)
	a.add(row, nil)
	return true
}

// chooseCourses reads "Students should choose four of the following
// 400-level courses (2.0-2.5 credits): LMP405Y1, LMP410H1, ...":
// choose{4} over the courses, when the courses pass the level and the
// range is what the count weighs (the four lightest, the four heaviest).
func (a *compAssembler) chooseCourses(row *compRow, t string) bool {
	m := complChooseCoursesRe.FindStringSubmatch(t)
	if m == nil || !isCodeList(m[5]) {
		return false
	}
	count := countOf(m[1])
	codes := dedupe(codeRe.FindAllString(m[5], -1))
	if count >= len(codes) {
		return false
	}
	var ws []float64
	var nodes []*types.ReqNode
	for _, c := range codes {
		if m[2] != "" && strconv.Itoa(courseLevel(c)) != m[2] {
			return false
		}
		ws = append(ws, courseWeight(c))
		nodes = append(nodes, &types.ReqNode{Type: types.NodeCourse, Code: c})
	}
	if m[3] != "" {
		lo, _ := strconv.ParseFloat(m[3], 64)
		hi, _ := strconv.ParseFloat(m[4], 64)
		neg := make([]float64, len(ws))
		for i, w := range ws {
			neg[i] = -w
		}
		heavy := -sumCheapest(neg, count)
		if math.Abs(sumCheapest(slices.Clone(ws), count)-lo) > 1e-9 || math.Abs(heavy-hi) > 1e-9 {
			return false
		}
	}
	row.r.Req = choiceNode(count, nodes)
	a.add(row, nil)
	return true
}

// inlineOr reads an item whose options are joined by an upper-case "OR"
// ("1.0 additional credit from Group A or B, **OR** 1.0 credit at any level
// of an Asian language (see _Accepted Language Courses_ below)", "CAS450H1
// **OR** 0.5 credit in a quantitative or qualitative methods course from
// Group C ..."), when the grammar can't read the whole: each option is a
// row (an item's body), and a choice picks one; the lists they name are read
// later (namedLists), and options that are each a body alone become this
// item's or (collapseChoices).
func (a *compAssembler) inlineOr(row *compRow, text string) bool {
	sents := splitComplSentences(text)
	if len(sents) != 1 || strings.Count(text, "OR") == 0 {
		return false
	}
	s := strings.TrimSuffix(strings.TrimSpace(a.clean(sents[0])), ".")
	parts := complInlineOrRe.Split(s, -1)
	if len(parts) < 2 || slices.ContainsFunc(parts, func(p string) bool { return !complOptionStartRe.MatchString(p) }) {
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
		row.r.Req = orNode(nodes...)
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

// collapseChoices makes a choice whose options are each a body alone (an
// allocation with no rows under it, right before the choice) one
// allocation: the or (choose) of the bodies, with the choice's label, section
// and raw. The options' text is read by now (namedLists).
func (a *compAssembler) collapseChoices() {
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
		body := orNode(nodes...)
		if c.r.Req.Type == types.NodeChoose {
			body = choiceNode(c.r.Req.Count, nodes)
		}
		row := &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Label: c.r.Label, Section: c.r.Section, Req: body, Notes: append(c.r.Notes, notes...), Raw: c.r.Raw}}
		a.choiceRefs = slices.DeleteFunc(a.choiceRefs, func(x compChoiceRef) bool { return slices.Contains(c.r.Req.Items, x.node) })
		a.items = slices.Concat(a.items[:i-k], []*compRow{row}, a.items[i+1:])
		i -= k
	}
}

// reqRefRow reports whether a pool refers to row (reqRefs).
func (a *compAssembler) reqRefRow(row *compRow) bool {
	return slices.ContainsFunc(a.reqRefs, func(ref compReqRef) bool { return slices.Contains(ref.rows, row) })
}

// overlapRow reports whether row is one of a pair that may share courses
// (a.overlaps): its number is written on the other row.
func (a *compAssembler) overlapRow(row *compRow) bool {
	return slices.ContainsFunc(a.overlaps, func(o [2]*compRow) bool { return o[0] == row || o[1] == row })
}

// choiceRow reports whether a choice picks row (choiceRefs).
func (a *compAssembler) choiceRow(row *compRow) bool {
	return slices.ContainsFunc(a.choiceRefs, func(ref compChoiceRef) bool { return slices.Contains(ref.rows, row) })
}

// restatedItems drops rows that restate what another row encodes, joining
// their sentence to that row's raw:
//   - an item "If a student has not completed STA130H1 then an additional
//     0.5 credit 300+ level STA course that is not used towards any other
//     program requirement must be completed", where a row already has
//     or(STA130H1, credits{0.5, STA, 300/400}) (a relaxation): it is that
//     alternative, and claims are exclusive anyway;
//   - "If the research course LMP405Y1 is taken, an additional three
//     400-level courses from the list are still required" after a
//     choose{4} over a list with LMP405Y1.
func (a *compAssembler) restatedItems() {
	var all []*compRow
	var walk func(rs []*compRow)
	walk = func(rs []*compRow) {
		for _, r := range rs {
			all = append(all, r)
			walk(r.children)
		}
	}
	walk(a.items)
	target := func(row *compRow) *compRow {
		if row.r.Req == nil || row.r.Req.Type != types.NodeText || len(row.children) > 0 {
			return nil
		}
		t := strings.TrimSuffix(strings.TrimSpace(a.clean(row.r.Req.Text)), ".")
		if m := complIfNotCompletedRe.FindStringSubmatch(t); m != nil {
			alt, _ := parseCompletionText(m[2])
			if alt == nil || hasText(alt) {
				return nil
			}
			for _, r := range all {
				found := false
				walkReq(r.r.Req, func(n *types.ReqNode) {
					found = found || n.Type == types.NodeOr && slices.ContainsFunc(n.Items, func(x *types.ReqNode) bool { return x.Type == types.NodeCourse && x.Code == m[1] }) &&
						slices.ContainsFunc(n.Items, func(x *types.ReqNode) bool { return sameNode(x, alt) })
				})
				if found && r != row {
					return r
				}
			}
		}
		if m := complChooseRestatedRe.FindStringSubmatch(t); m != nil {
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

// chosenRows returns the numbers of the rows c's choices pick among: they
// count only through their choice.
func chosenRows(c *types.Completion) map[int]bool {
	out := map[int]bool{}
	for _, r := range c.Requirements {
		if r != nil && r.Kind == types.RequirementChoice {
			walkReq(r.Req, func(n *types.ReqNode) {
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

// choiceCredits returns the fewest credits a choice's options need (the
// cheapest choice): a requirement node's cheapest row (rowCredits), the sum
// of an "and", the cheapest alternative of an "or", the cheapest count of a
// "choose".
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
		return sumCheapest(ws, max(1, n.Count))
	}
	return 0
}

// compArea is one area of an areaOptions item: its name and its line.
type compArea struct {
	name, other string // other: the name it corresponds to in the other item
	b           *compBlock
}

// areaOptions reads two items over areas the student chooses ("Students
// must choose any two of the available language areas (Czech and Slovak,
// ...)", "2. 4.0 credits of language coursework from each of the two
// chosen language areas (2.0 credits per area)." then "_Czech and Slovak:_
// SLA105Y1, ...", "3. 4.0 credits from culture courses corresponding to the
// student’s two chosen language areas (2.0 credits per area):" then the
// areas' culture lines): each area is an option, a group of its two parts
// (2.0 credits from each of its lines), and a choice row picks two of them
// (choose over requirement nodes). An area the sentence doesn't name
// ("Yiddish (corresponds to the Jewish Cultures in Eastern Europe area)",
// in both lists) is one only the lenient reading allows: its first part is
// and(credits, the sentence), Review. A "Note:" after an item's lines that
// is an approval or another way to meet it is an alternative on that item's
// parts. bs[0] is the first item; it returns how many blocks it read (0:
// none).
func (a *compAssembler) areaOptions(bs []*compBlock) int {
	type areaItem struct {
		b      *compBlock
		amount float64
		areas  []compArea
		notes  []*compBlock
	}
	read := func(i int) (it areaItem, next int, ok bool) {
		b := bs[i]
		m := complPerAreaRe.FindStringSubmatch(strings.TrimSpace(a.clean(b.text)))
		if m == nil || !blkNumbered(b) || !strings.EqualFold(m[3], "language") {
			return it, 0, false
		}
		it.b = b
		it.amount, _ = strconv.ParseFloat(m[4], 64)
		if total, _ := strconv.ParseFloat(m[1], 64); math.Abs(total-it.amount*float64(countOf(m[2]))) > 1e-9 {
			return it, 0, false // "4.0 credits ... (2.0 credits per area)" over two areas
		}
		lines, next := b.children, i+1
		if len(lines) == 0 {
			for next < len(bs) && bs[next].kind == blockLabelled && len(bs[next].children) == 0 {
				next++
			}
			lines = bs[i+1 : next]
		}
		for _, c := range lines {
			switch {
			case blkNoteLabelRe.MatchString(c.label):
				it.notes = append(it.notes, c)
			case c.kind == blockLabelled && len(c.children) == 0 && codeRe.MatchString(c.text) && len(it.notes) == 0:
				ar := compArea{name: strings.TrimSpace(complAreaParenRe.ReplaceAllString(stripEmphasis(c.label), "")), b: c}
				if cm := complCorrespondsRe.FindStringSubmatch(stripEmphasis(c.label)); cm != nil {
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
	for next < len(bs) && blkNoteLabelRe.MatchString(bs[next].label) && len(bs[next].children) == 0 {
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
	// The sentence that sets the choice, read already as a row of text.
	var sentence *compRow
	var count int
	var named []string
	for _, rs := range [][]*compRow{a.program, a.items} {
		for _, r := range rs {
			if r.r.Req == nil || r.r.Req.Type != types.NodeText || len(r.children) > 0 {
				continue
			}
			if m := complChooseAreasRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(a.clean(r.r.Req.Text)), ".")); m != nil && strings.EqualFold(m[2], "language") {
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
	// Each area of the first item, with its line in the second.
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
		g := &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Label: x.name, Section: first.b.section, Raw: a.line(x.b) + "\n" + a.line(second.areas[k].b)}}
		for _, part := range []struct {
			it  areaItem
			b   *compBlock
			out *[]*compRow
		}{{first, x.b, &firsts}, {second, second.areas[k].b, &seconds}} {
			row := a.optionRow(fmtAmount(part.it.amount)+" credits from: "+part.b.text, "", part.it.b.section, a.line(part.b))
			g.children = append(g.children, row)
			*part.out = append(*part.out, row)
		}
		if !slices.Contains(named, strings.ToLower(x.name)) {
			// Two readings: the sentence's areas, or every area listed.
			p := g.children[0]
			p.r.Req = andNode(p.r.Req, textNode(types.TextOther, sentence.r.Req.Text))
		}
		groups = append(groups, g)
	}
	var notes []string // the items' notes: the choice's
	for _, part := range []struct {
		it   areaItem
		rows []*compRow
	}{{first, firsts}, {second, seconds}} {
		for _, nb := range part.it.notes {
			for _, s := range splitComplSentences(nb.text) {
				if a.isApproval(s) || a.isAlternative(s) {
					kind := altKind(s)
					if a.isApproval(s) {
						kind = types.TextPermission
					}
					for _, r := range part.rows {
						r.r.Req = orNode(r.r.Req, textNode(kind, s))
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
	a.addChoice(groups, "", first.b.section, raw, choiceNode(count, nodes))
	a.items[len(a.items)-1].r.Notes = notes
	return end
}
