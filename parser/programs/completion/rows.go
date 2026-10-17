package completion

import (
	"regexp"
	"slices"
	"strings"

	"degree-explorer/parser/grammar"
	"degree-explorer/parser/programs"
	"degree-explorer/types"
)

// Reading blocks into rows: the header total, items, their parts and the lines
// under them. Each sentence's role decides where it goes.

var (
	// codeListLineRe matches a line that only lists codes: "ECO365H1,
	// ECO368H1, TRN410H1".
	codeListLineRe = regexp.MustCompile(`^(?:` + grammar.CodePat + `|[A-Z]{3}\d{4}[HY]\d?|[\s,;/().*+&]|(?i:and|or)\b)+$`)
	// titleParenRe matches brackets, a course's title when they hold no
	// code: "(Nuclear Weapons and International Politics)".
	titleParenRe = regexp.MustCompile(`\([^()]*\)`)
	// eitherLeadRe matches a lead that makes the items under it
	// alternatives: "One of the following:".
	eitherLeadRe = regexp.MustCompile(`(?i)\beither\b|\bone\s+of\b`)
	// countLeadRe matches a lead that picks some of the items under it:
	// "Two of the following selections:". Group 1 is the count.
	countLeadRe = regexp.MustCompile(`(?i)\b(two|three|four|five|[2-5])\s+of\s+the\s+following\b`)
	// trailingOrRe matches an item's trailing conjunction: "CSC165H1 or".
	trailingOrRe = regexp.MustCompile(`(?i)\s+or\s*$`)
	// trailingCommaOrRe matches a line ending in ", OR", whose next line
	// is its alternative.
	trailingCommaOrRe = regexp.MustCompile(`(?:,\s*|\s+)OR\s*$`)
	// floorsLeadRe matches a lead whose items are floors on it: "These 3.0
	// credits must include:".
	floorsLeadRe = regexp.MustCompile(`(?i)\b(?:including|must\s+include|of\s+which)\s*:\s*$`)
	// itemFloorParenRe matches a floor in brackets right after an item's
	// amount: "1.5 credits (at least 0.5 credit at the 400-level) from: ...".
	itemFloorParenRe = regexp.MustCompile(`^(.*?` + grammar.NumPat + `\s+credits?)\s*\(([^()]*\b(?:` + grammar.AtLeastPat + `|must|minimum)\b[^()]*)\)\s*(.*)$`)
	// capScopeRe matches the scope a cap ends with: "can be used towards
	// this requirement". The cap applies where it is written.
	capScopeRe = regexp.MustCompile(`(?i)\s+(?:can|may|will)\s+(?:be\s+)?(?:used|counted|count|applied|accepted)\s+(?:to(?:wards?)?|for)\s+(?:fulfill\s+|satisfy\s+)?(?:this|the|any|our)\s+(?:[\w-]+\s+)?(?:requirements?|program|minor|major|specialist|certificate|focus)\b.*$`)
)

// blocks reads a list of sibling blocks.
func (a *Assembler) blocks(bs []*Block, ctx compCtx) {
	for i := 0; i < len(bs); i++ {
		b := bs[i]
		if n := a.numberedLists(bs[i:], ctx); n > 0 {
			// numbered lists with their floors and caps: one item
			i += n - 1
			continue
		}
		if b.Kind != BlockHeading && !BlkNoteLabelRe.MatchString(b.Label) && (a.UnderAdviceHeading(b) || a.AdviceLabel(b)) {
			// advice: "Suggested Related Courses:" then "- CSC367H1"
			a.adviceBlock(b, ctx)
			continue
		}
		if n := a.optionBlocks(bs[i:], ctx); n > 0 {
			// options: "Choose one of the options below:" then "Option A: ..."
			i += n - 1
			continue
		}
		if n := a.AdviceZone(bs[i:]); n > 0 {
			// a lead into advice and the lines it lists: one note
			note, rest := a.adviceNote(BlockLine(b), bs[i+1:i+1+n])
			a.notesTo(ctx.owner, []string{note})
			a.blocks(rest, ctx)
			i += n
			continue
		}
		if i > 0 && isListTitle(bs[i-1]) && len(b.Children) == 0 && mostlyCodes(b.Text) && !blkNumbered(b) {
			// a list's members under its title, with their own label if any:
			// "Group A: ..." then "ANT210H1, ANT215H1, ..."
			title := strings.TrimSuffix(grammar.CleanNote(BlockLine(bs[i-1])), ":")
			a.listDef(ctx.owner, title, BlockLine(b), b, title+": "+grammar.CleanNote(BlockLine(b)))
			continue
		}
		// "X, OR" then "Y": one requirement
		if i+1 < len(bs) && b.Kind != BlockHeading && bs[i+1].Kind != BlockHeading && len(b.Children) == 0 && trailingCommaOrRe.MatchString(b.Text) {
			joined := *b
			joined.Text = b.Text + " " + bs[i+1].Text
			joined.Children = bs[i+1].Children
			a.joined[&joined] = bs[i+1]
			a.block(&joined, ctx, nil)
			i++
			continue
		}
		// a lead-in and the line of codes after it are one requirement
		if i+1 < len(bs) && b.Kind != BlockHeading && len(b.Children) == 0 && (codeLineLeadRe.MatchString(b.Text) || poolListedBelowRe.MatchString(b.Text) ||
			remainingLeadRe.MatchString(b.Text)) && isCodeListLine(bs[i+1]) {
			a.block(b, ctx, bs[i+1])
			i++
			continue
		}
		if n := a.listZone(bs[i:]); n > 0 {
			// a lead and the sub-lists it leads into: one item
			a.listLead(b, bs[i+1:i+1+n], ctx)
			i += n
			for i+1 < len(bs) && a.poolRuleLine(bs[i], bs[i+1]) {
				i++
			}
			continue
		}
		if i > 0 && a.poolRuleLine(bs[i-1], b) {
			continue
		}
		a.block(b, ctx, nil)
	}
}

// poolRuleLineRe matches a line that adds to or caps the pool before it:
// "No more than one from".
var poolRuleLineRe = regexp.MustCompile(`(?i)^(?:` + grammar.NoMoreThanPat + `|up\s+to|at\s+most|any\s+(?:` + grammar.DesigPat + `\s+)?courses?)\b`)

// poolRuleLine reads b, the line after prev, as a rule on the pool of the lead
// prev ends, and reports whether it did: prose right after the sub-lists that
// adds to or caps them is about the item.
func (a *Assembler) poolRuleLine(prev, b *Block) bool {
	row := a.leadRows[prev]
	for k := prev; row == nil && len(k.Children) > 0; {
		k = k.Children[len(k.Children)-1]
		row = a.leadRows[k]
	}
	if row == nil || row.r.Req == nil || b.Kind != blockProse || len(b.Children) > 0 || !poolRuleLineRe.MatchString(strings.TrimSpace(b.Text)) {
		return false
	}
	row.r.Raw += "\n" + a.line(b)
	a.about(b, row, nil)
	a.leadRows[b] = row
	return true
}

// isCodeListLine reports whether b is a line of prose that only lists
// codes.
func isCodeListLine(b *Block) bool {
	return b.Kind == blockProse && len(b.Children) == 0 && IsCodeList(b.Text)
}

// IsCodeList reports whether s only lists codes, titles in brackets aside.
func IsCodeList(s string) bool {
	if !grammar.CodeRe.MatchString(s) {
		return false
	}
	s = titleParenRe.ReplaceAllStringFunc(s, func(p string) string {
		if grammar.CodeRe.MatchString(p) {
			return p
		}
		return ""
	})
	return codeListLineRe.MatchString(s)
}

// block reads one block; list is the line of codes its lead-in takes, if
// any.
func (a *Assembler) block(b *Block, ctx compCtx, list *Block) {
	if ctx.owner == nil && !ctx.advice && b.Kind != blockItem && b.Kind != blockBullet && a.header(b, list) {
		a.blocks(b.Children, ctx)
		return
	}
	if b.Kind == BlockHeading {
		if !a.adviceHeading(b) && !a.UnderAdviceHeading(b) {
			a.headingFloors(b, ctx)
		}
		a.blocks(b.Children, ctx)
		return
	}
	a.item(b, ctx, list)
}

// BlockLine is a block's text with its label: "Ecology: EEB255H1, ...". A
// note's label ("Note:") is left out.
func BlockLine(b *Block) string {
	if b.Kind == blockLabelled && !BlkNoteLabelRe.MatchString(b.Label) {
		return b.Label + ": " + b.Text
	}
	return b.Text
}

// fociHeading returns the heading over b that names the program's focuses
// ("**Disciplinary Focuses**"), or nil.
func fociHeading(b *Block) *Block {
	for h := b.Heading; h != nil; h = h.Heading {
		if h.Kind == BlockHeading && blkFociRe.MatchString(h.Section) {
			return h
		}
	}
	return nil
}

// focusChoiceRe matches the rule focuses come with: "must enrol in and
// complete at least one disciplinary focus".
var focusChoiceRe = regexp.MustCompile(`(?i)\bcomplete\s+` + grammar.AtLeastPat + `\s+one\b[^.]*\bfocus\b`)

// focusLine reads a line under a heading of focuses (h) into the one row they
// make, never an allocation per focus, which every student would then need. The
// rule is the row's body, for focusChoices; the other lines are notes.
func (a *Assembler) focusLine(h, b *Block) {
	row := a.focusRows[h]
	if row == nil {
		row = &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Section: h.Section, Raw: a.line(h),
			Req: TextNode(types.TextOther, "Complete at least one of the "+h.Section)}}
		a.focusRows[h] = row
		a.add(row, nil)
	}
	var walk func(c *Block)
	walk = func(c *Block) {
		row.r.Raw += "\n" + a.line(c)
		prefix := ""
		if c.Heading != nil && c.Heading != h && c.Heading.Kind == BlockHeading && !c.Heading.IsNotes() {
			prefix = c.Heading.Text + " " // the focus's heading leads its first note
		}
		for _, s := range SplitSentences(BlockLine(c)) {
			if focusChoiceRe.MatchString(s) {
				row.r.Req = TextNode(types.TextOther, s)
				continue
			}
			row.r.Notes = append(row.r.Notes, grammar.CleanNote(prefix+s))
			prefix = ""
		}
		for _, k := range c.Children {
			walk(k)
		}
	}
	walk(b)
}

var (
	// focusCodeRe matches a focus line with its code: "Health Studies
	// (ASFOC2270A): (2.0 credits)". Groups 1 and 2 are the name and the code.
	focusCodeRe = regexp.MustCompile(`^(.+?)\s+\(([A-Z]{2}FOC\d{4}[A-Z])\)\s*:`)
	// focusNoCodeRe matches a focus line without one: "Immunology (3.0
	// Credits) - ".
	focusNoCodeRe = regexp.MustCompile(`^([A-Z][^():]*?)\s+\(` + grammar.NumPat + `\s+(?i:credits?)\)\s+-\s`)
)

// focusChoices makes a focus heading's rule the or of its focuses' program
// nodes, as enrolment writes a focus. A coded focus's line stays beside its
// node (focusPart); an uncoded one is named only, for Link to resolve by title.
func (a *Assembler) focusChoices() {
	for _, row := range a.focusRows {
		n := row.r.Req
		if n == nil || n.Type != types.NodeText || !focusChoiceRe.MatchString(n.Text) {
			continue
		}
		// the focuses run from the rule's line to the heading's last; the row's
		// raw lacks the focuses' own headings
		own := map[string]bool{}
		for _, l := range strings.Split(row.r.Raw, "\n") {
			own[rawWords(l)] = true
		}
		start := slices.IndexFunc(a.lines, func(l string) bool { return focusChoiceRe.MatchString(l) && own[rawWords(l)] })
		if start < 0 {
			continue
		}
		end := start
		for i := start + 1; i < len(a.lines); i++ {
			if own[rawWords(a.lines[i])] {
				end = i
			}
		}
		var at []int
		for i := start + 1; i <= end; i++ {
			if l := strings.TrimSpace(a.lines[i]); focusCodeRe.MatchString(l) || focusNoCodeRe.MatchString(l) {
				at = append(at, i)
			}
		}
		if len(at) < 2 {
			continue
		}
		var foci, texts []*types.ReqNode
		var notes [][]string
		var limits [][]*types.Requirement
		for _, i := range at {
			l := strings.TrimSpace(a.lines[i])
			if m := focusCodeRe.FindStringSubmatch(l); m != nil {
				p := &types.ReqNode{Type: types.NodeProgram, Programs: []string{m[2]}, Text: m[1]}
				head, body := a.focusLineAt(i, m)
				part, text, ns, ls := a.focusPart(row, p, head, body)
				foci, texts, notes, limits = append(foci, part), append(texts, text), append(notes, ns), append(limits, ls)
				continue
			}
			m := focusNoCodeRe.FindStringSubmatch(l)
			foci, texts = append(foci, &types.ReqNode{Type: types.NodeProgram, Text: m[1]}), append(texts, nil)
			notes, limits = append(notes, nil), append(limits, nil)
		}
		// a focus's limits trim the row whichever focus is completed, so a
		// focus whose limits could trim another's is its line as text
		for i := range foci {
			if limits[i] != nil && !focusLimitsSafe(limits[i], slices.Delete(slices.Clone(foci), i, i+1)) {
				foci[i], notes[i], limits[i] = texts[i], nil, nil
			}
		}
		for i := range foci {
			row.r.Notes = append(row.r.Notes, notes[i]...)
			for _, l := range limits[i] {
				a.rowUnder(row, l)
			}
		}
		row.r.Req = &types.ReqNode{Type: types.NodeOr, Items: grammar.DedupeNodes(foci)}
	}
}

// requiredSoFar returns the codes the items read so far name in their raw.
func (a *Assembler) requiredSoFar() map[string]bool {
	seen := map[string]bool{}
	walkRows(a.items, func(r *compRow) {
		for _, c := range grammar.CodeRe.FindAllString(r.r.Raw, -1) {
			seen[c] = true
		}
	})
	return seen
}

// remainingLeadRe matches a lead whose group is the line of codes after
// it: "Other Electives (1.5-2.5 credits): Remaining credits from the SDS
// Electives Group".
var remainingLeadRe = regexp.MustCompile(`(?i)^[^:(]*\(` + grammar.AmountPat + `(?:\s*[-–]\s*` + grammar.NumPat + `)?\s+credits?\)\s*:\s*remaining\s+credits\s+(?:selected\s+)?from\s+the\s+[^.]+?\bgroup\.?$`)

// header reads a header line into Req 1 and its floors and reports whether b
// was one. A line restating the total ("Students complete 10.0 credits,
// including:" after "(10.0 credits, ...)") adds its floors to it.
func (a *Assembler) header(b *Block, list *Block) bool {
	text := a.line(b)
	h, ok := parseHeader(text)
	if !ok || a.total != nil && h.total != a.total.r.Req.Amount {
		return false
	}
	raw := text
	if list != nil {
		raw += "\n" + a.line(list)
	}
	if a.total == nil {
		n := &types.ReqNode{Type: types.NodeCredits, Amount: h.total}
		if h.pool != nil {
			n.Designators, n.Levels = slices.Clone(h.pool.Designators), slices.Clone(h.pool.Levels)
		}
		a.total = &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Req: n, Raw: raw}}
	} else {
		a.total.r.Raw += "\n" + raw
		if n := a.total.r.Req; h.pool != nil && n.Type == types.NodeCredits && n.Designators == nil && n.Levels == nil && n.Courses == nil {
			// "Total: 2.0 credits" then "2.0 GER credits taught in German".
			n.Designators, n.Levels = slices.Clone(h.pool.Designators), slices.Clone(h.pool.Levels)
		}
	}
	if list != nil {
		// "chosen from those listed below": the list is the total's own pool
		n, _ := parseCompletionText(fmtAmount(h.total) + " credits from: " + list.Text)
		n, caps := poolCaps(n)
		if poolListedBelowRe.MatchString(text) && isPool(n) {
			n.Designators, n.Levels = slices.Clone(a.total.r.Req.Designators), slices.Clone(a.total.r.Req.Levels)
			a.total.r.Req = n
			for _, c := range caps {
				a.floors = append(a.floors, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Req: c, Raw: a.total.r.Raw}})
			}
		} else {
			a.textRow(nil, list.Text)
		}
	}
	for _, r := range h.rows {
		if n := a.total.r.Req; r.Kind == types.RequirementConstraint && r.Req.Type == types.NodeCredits && r.Req.Amount == n.Amount &&
			n.Type == types.NodeCredits && n.Designators == nil && n.Levels == nil && n.Courses == nil && r.Req.Courses == nil {
			// "All 4.0 credits must be NMC and/or NML courses": the total's
			// pool.
			n.Designators, n.Levels = slices.Clone(r.Req.Designators), slices.Clone(r.Req.Levels)
			continue
		}
		if !slices.ContainsFunc(a.floors, func(f *compRow) bool { return sameNode(f.r.Req, r.Req) }) {
			a.floors = append(a.floors, &compRow{r: r})
		}
	}
	for _, s := range h.tail {
		for _, sent := range SplitSentences(s) {
			a.notesTo(nil, a.sentence(sent, nil, compCtx{}))
		}
	}
	return true
}

// item reads a block that isn't a header or a heading, and the blocks under
// it. Its first requirement sentence is a new allocation's body; the others are
// rules on it, notes or alternatives. list is the line its lead-in takes.
func (a *Assembler) item(b *Block, ctx compCtx, list *Block) {
	if ctx.owner != nil && ctx.owner.r.Req != nil {
		a.about(b, ctx.owner, list)
		return
	}
	if ctx.owner == nil && !blkNumbered(b) && len(b.Children) == 0 && a.totalPoolLead(b) {
		return
	}
	if h := fociHeading(b); ctx.owner == nil && h != nil {
		a.focusLine(h, b)
		return
	}
	if ctx.owner == nil && a.listLine(b) {
		// a list the items draw on, never a requirement: a list definition,
		// kept as a note until a row uses it
		title := b.Label
		if b.Kind != blockLabelled && b.Heading != nil {
			title = b.Heading.Section
		}
		a.listDef(nil, title, b.Text, b, a.listNote(b))
		return
	}
	ctx.notes = ctx.notes || BlkNoteLabelRe.MatchString(b.Label) || b.Heading != nil && b.Heading.IsNotes()
	ctx.numbered = blkNumbered(b)
	if ctx.owner == nil && ctx.numbered {
		a.numberedSeen = true
	}
	row := &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Label: itemLabel(b), Section: b.Section, Raw: a.line(b)}}
	kids := b.Children
	if list == nil && a.leadLines(b, row, ctx) {
		return // lines too short for sub-lists
	}
	if n := leadingSubLines(kids); list == nil && n >= 2 && amountRe.MatchString(b.Text) &&
		(strings.HasSuffix(strings.TrimSpace(b.Text), ":") || (theFollowingRe.MatchString(b.Text) || fromListBelowRe.MatchString(b.Text)) && !grammar.CodeRe.MatchString(b.Text)) {
		// one item over all the sub-lists; the lines after them are about it
		row := a.listLead(b, kids[:n], ctx)
		a.children(kids[n:], b.Text, ctx, row)
		return
	}
	if list == nil && len(kids) > 0 && (strings.HasSuffix(b.Text, ":") || theFollowingRe.MatchString(b.Text) && !grammar.CodeRe.MatchString(b.Text) && (len(kids) == 1 || !subLine(kids[1]))) &&
		len(kids[0].Children) == 0 && IsCodeList(kids[0].Text) {
		// a lead-in over one line of codes, which is its list
		list, kids = kids[0], kids[1:]
	}
	text := b.Text
	if b.Kind == blockLabelled && listLabelRe.MatchString(b.Label+":") {
		// "Group A: CRI300H1/ ...": the label says the line is a list.
		text = b.Label + ": " + text
	}
	if list != nil {
		if m := remainingLeadRe.FindStringSubmatch(strings.TrimSpace(text)); m != nil {
			// "Remaining credits from the SDS Electives Group" then its codes:
			// the lower end of the range from them
			text = m[1] + " credits from: " + list.Text
		} else if loc := theFollowingRe.FindStringIndex(text); loc != nil && !strings.HasSuffix(strings.TrimSpace(text), ":") && !grammar.CodeRe.MatchString(text) {
			// "from the following list, ..." then codes: the codes are the list
			text = text[:loc[0]] + strings.TrimSpace(list.Text) + text[loc[1]:]
		} else {
			text = strings.TrimSuffix(strings.TrimSpace(text), ":") + ": " + list.Text
		}
		row.r.Raw += "\n" + a.line(list)
	}
	if ctx.owner == nil && len(kids) == 0 && a.optionItem(row, text) {
		// options in an item: "X **OR** Y"
		return
	}
	if ctx.owner == nil && len(kids) == 0 && a.additional(text, row.r.Raw) {
		return
	}
	if b.Kind == blockLabelled && mostlyCodes(b.Text) && !blkNumbered(b) && (choiceListLabelRe.MatchString(b.Label) || a.namedElsewhere(b)) {
		// a list an item draws on, never all required: "Approved Economics
		// courses: ECO313H1; ..."
		a.listDef(ctx.owner, b.Label, b.Text, b, b.Label+": "+grammar.CleanNote(b.Text))
		return
	}
	if inListHeading(b) && mostlyCodes(b.Text) {
		// a list's members under its heading: "Group A: Ancient, Medieval"
		a.listDef(ctx.owner, b.Heading.Section, text, b, b.Heading.Section+": "+grammar.CleanNote(text))
		return
	}
	if mayBeList(b) {
		// many courses and no amount under a topic heading: most likely a list,
		// maybe all required, so Review
		a.textRow(ctx.owner, a.line(b))
		return
	}
	if ctx.owner == nil && len(kids) > 0 && ofTheseLeadRe.MatchString(a.clean(text)) {
		// "Of these:" then floors and caps on the total, one per line
		for _, c := range kids {
			a.programFloors(c)
		}
		return
	}
	// a title over parts, with no requirement of its own
	title := len(kids) > 0 && !grammar.CodeRe.MatchString(text) && !amountRe.MatchString(text)
	if next := a.joined[b]; next != nil && ctx.owner == nil {
		// "X, OR" then "Y": the item is either line.
		left, lrows, lnotes := a.itemBody(trailingCommaOrRe.ReplaceAllString(strings.TrimSpace(strings.TrimSuffix(b.Text, next.Text)), ""))
		right, rrows, rnotes := a.itemBody(next.Text)
		row.r.Req = programs.OrNode(left, right)
		for _, r := range append(lrows, rrows...) {
			a.rowUnder(row, r)
		}
		row.r.Notes = append(lnotes, rnotes...)
		a.add(row, ctx.owner)
		return
	}
	if m := titleCodeAdviceRe.FindStringSubmatch(strings.TrimSpace(text)); m != nil && ctx.numbered && len(kids) == 0 && a.allAdvice(m[3]) {
		// "Professional Experience Course ACT390H1: students are strongly
		// recommended ...": the course, with the advice as its note
		if n, _ := parseCompletionText(m[2]); n != nil && !hasText(n) {
			row.r.Req = n
			row.r.Notes = append(row.r.Notes, grammar.CleanNote(m[3]))
			a.add(row, ctx.owner)
			return
		}
	}
	var notes []string
	relaxed := len(a.relaxations)
	for _, s := range a.routeSentences(SplitSentences(text)) {
		if title && a.role(s) == RoleRequirement && !a.IsApproval(s) {
			continue
		}
		notes = append(notes, a.sentence(s, row, ctx)...)
	}
	if b.Kind == blockLabelled && !BlkNoteLabelRe.MatchString(b.Label) && len(notes) > 0 && row.r.Req == nil && len(row.children) == 0 &&
		!strings.HasPrefix(notes[0], b.Label) {
		// the label is part of the note: "Consult Program Administrator: ..."
		notes[0] = b.Label + ": " + notes[0]
	}
	if len(a.relaxations) > relaxed && row.r.Req == nil && len(row.children) == 0 && len(kids) > 0 {
		// a relaxation leading into options: the options are the relaxation's
		last := len(a.relaxations) - 1
		for _, k := range kids {
			a.relaxations[last] += " " + BlockLine(k)
			for _, kk := range k.Children {
				a.relaxations[last] += " " + BlockLine(kk)
			}
		}
		a.notesTo(ctx.owner, notes)
		return
	}
	switch {
	case row.r.Req != nil || len(row.children) > 0:
	case len(kids) > 0 && !a.allAdvice(text):
		// a lead-in: its parts are under it
	case len(kids) > 0 && a.allAdvice(text):
		// advice and what it lists are one note; rules among the lines under it
		// are read as usual
		note, rest := a.adviceNote(text, kids)
		a.notesTo(ctx.owner, []string{note})
		a.blocks(rest, ctx)
		return
	default:
		// No requirement: its rules went to the program.
		a.notesTo(ctx.owner, notes)
		a.children(kids, b.Text, ctx, nil)
		return
	}
	row.r.Notes = append(row.r.Notes, notes...)
	if ctx.owner == nil && a.poolCondition(row, text) {
		return
	}
	a.add(row, ctx.owner)
	a.children(kids, b.Text, ctx, row)
	if row.r.Req == nil {
		a.parts(row, b)
	}
	if row.r.Req == nil && !slices.ContainsFunc(row.children, isAllocation) {
		a.dissolve(row, ctx.owner)
		return
	}
	splitCaps(row)
	splitFloors(row, text)
	a.splitShared(row)
}

func isAllocation(k *compRow) bool { return k.r.Kind == types.RequirementAllocation }

// dissolve removes a lead-in that turned out to have no parts: its rows and
// notes belong to owner (or the program).
func (a *Assembler) dissolve(row, owner *compRow) {
	if owner != nil {
		owner.children = slices.DeleteFunc(owner.children, func(k *compRow) bool { return k == row })
	} else {
		a.items = slices.DeleteFunc(a.items, func(k *compRow) bool { return k == row })
	}
	for _, k := range row.children {
		a.rowUnder(owner, k.r)
	}
	a.notesTo(owner, row.r.Notes)
}

// routeSentences joins to a route (an approval or a widened pool) the plain
// sentences after it that say what it lets count. A route pointing back at
// "these courses" joins the route before it, else keeps it as context.
func (a *Assembler) routeSentences(sents []string) []string {
	var out []string
	route, anyRoute := false, false
	for _, s := range sents {
		isRoute := a.role(s) == RoleRequirement && (a.IsApproval(s) || a.IsAlternative(s))
		plain := a.role(s) == RoleRequirement && !isRoute
		followUp := anyRoute && isRoute && a.IsApproval(s) && !grammar.CodeRe.MatchString(s) && !refPhraseRe.MatchString(s) && !replacedRe.MatchString(s)
		if route && (plain && !allocLeadRe.MatchString(a.clean(s)) || isRoute && routeBackRe.MatchString(s)) || followUp {
			// one route: "in lieu of PSY201H1. Please contact us to verify ..."
			out[len(out)-1] = strings.TrimRight(out[len(out)-1], ".") + ". " + s
			continue
		}
		anyRoute = isRoute
		if isRoute && routeBackRe.MatchString(s) && len(out) > 0 && grammar.CodeRe.MatchString(out[len(out)-1]) {
			a.routeContext[s] = out[len(out)-1]
		}
		route = a.role(s) == RoleRequirement && (a.IsApproval(s) || widenRe.MatchString(a.clean(s)) || routeBackRe.MatchString(s) && a.IsAlternative(s))
		out = append(out, s)
	}
	return out
}

// routeBackRe matches a sentence pointing back at the courses before it:
// "one of these".
var routeBackRe = regexp.MustCompile(`(?i)\b(?:these|such)\s+(?:\w+\s+)?courses\b|\bone\s+of\s+these\b`)

// mostlyCodes reports whether s lists codes with little else: at least three
// codes, and a few words besides ("..., AFR498H1, an independent studies
// course approved by the Program Committee"), or only codes.
func mostlyCodes(s string) bool {
	if IsCodeList(s) {
		return true
	}
	codes := anyCodeRe.FindAllString(s, -1)
	rest := strings.Fields(grammar.WordRe.ReplaceAllStringFunc(anyCodeRe.ReplaceAllString(titleParenRe.ReplaceAllString(s, " "), " "), func(w string) string {
		if strings.EqualFold(w, "and") || strings.EqualFold(w, "or") {
			return " "
		}
		return w
	}))
	words := 0
	for _, w := range rest {
		if grammar.WordRe.MatchString(w) {
			words++
		}
	}
	return len(codes) >= 3 && words <= 12 && !amountRe.MatchString(s)
}

// anyCodeRe matches a course code, undergraduate or graduate: "CSC2221H".
var anyCodeRe = regexp.MustCompile(grammar.CodePat + `|\b[A-Z]{3}\d{4}[HY]\d?\b`)

// add makes row a child of owner, or an item of the program.
func (a *Assembler) add(row, owner *compRow) {
	if owner != nil {
		owner.children = append(owner.children, row)
	} else {
		a.items = append(a.items, row)
	}
}

// children reads the blocks kids under a block with text lead: under an item
// (row), its floors, parts or sentences about it; under advice, advice.
func (a *Assembler) children(kids []*Block, lead string, ctx compCtx, row *compRow) {
	if len(kids) == 0 {
		return
	}
	switch {
	case row == nil && a.allAdvice(lead):
		a.blocks(kids, compCtx{owner: ctx.owner, advice: true, notes: ctx.notes})
	case row == nil:
		a.blocks(kids, ctx)
	case row.r.Req != nil && floorsLeadRe.MatchString(lead):
		for _, c := range kids {
			a.floorBlock(c, row)
		}
	default:
		a.blocks(kids, compCtx{owner: row, advice: ctx.advice, notes: ctx.notes})
	}
}

// about reads a block under an item that has a body: its sentences are rules,
// alternatives and notes on the item. A requirement it doesn't qualify as an
// alternative is a row of text under the item, which may add to or widen it.
func (a *Assembler) about(b *Block, owner *compRow, list *Block) {
	text := b.Text
	if list != nil {
		text += " " + list.Text
	}
	if len(b.Children) > 0 && a.allAdvice(text) {
		note, rest := a.adviceNote(text, b.Children)
		owner.r.Notes = append(owner.r.Notes, note)
		for _, c := range rest {
			a.about(c, owner, nil)
		}
		return
	}
	if a.prerequisiteMembers(b, owner) {
		return
	}
	if k := b.Children; b.Kind != blockLabelled && b.Kind != BlockHeading && len(k) == 1 && len(k[0].Children) == 0 && mostlyCodes(k[0].Text) &&
		subListTitleRe.MatchString(strings.TrimSpace(b.Text)) && !anyCodeRe.MatchString(b.Text) && !amountRe.MatchString(b.Text) {
		// a sub-list and its name, one note: "- Politics & Policy" then codes
		a.listDef(owner, grammar.CleanNote(b.Text), k[0].Text, k[0], grammar.CleanNote(b.Text)+": "+grammar.CleanNote(BlockLine(k[0])))
		return
	}
	notesBefore := len(owner.r.Notes)
	defer func() {
		// a labelled line kept as a note keeps its label, which routes refer to
		if b.Kind == blockLabelled && !BlkNoteLabelRe.MatchString(b.Label) && len(owner.r.Notes) > notesBefore &&
			!strings.HasPrefix(owner.r.Notes[notesBefore], b.Label) {
			owner.r.Notes[notesBefore] = b.Label + ": " + owner.r.Notes[notesBefore]
		}
		// a line of codes kept as a note keeps the sub-heading naming its list
		if h := b.Heading; b.Kind != blockLabelled && h != nil && h.Section != "" && h.Section != owner.r.Section &&
			len(owner.r.Notes) > notesBefore && mostlyCodes(b.Text) && !strings.HasPrefix(owner.r.Notes[notesBefore], h.Section) {
			owner.r.Notes[notesBefore] = h.Section + ": " + owner.r.Notes[notesBefore]
		}
		// A list's line kept as one note: a list an item may refer to.
		if len(owner.r.Notes) == notesBefore+1 && (b.Kind == blockLabelled && !BlkNoteLabelRe.MatchString(b.Label) || mostlyCodes(b.Text)) {
			title := b.Label
			if b.Kind != blockLabelled && b.Heading != nil {
				title = b.Heading.Section
			}
			a.listDefs = append(a.listDefs, &compListDef{title: title, members: b.Text, note: owner.r.Notes[notesBefore], block: b, row: owner})
		}
	}()
	for _, s := range SplitSentences(text) {
		role := a.role(s)
		if a.poolAddition(owner, s) {
			continue
		}
		if m := orContinuationRe.FindStringSubmatch(strings.TrimSpace(a.clean(s))); m != nil && owner.r.Req != nil {
			// more alternatives for the item: "(or one of ECO220Y1, PSY201H1)"
			alts := append([]*types.ReqNode{owner.r.Req}, programs.CourseNodes(grammar.CodeRe.FindAllString(m[1], -1))...)
			owner.r.Req = programs.OrNode(alts...)
			owner.r.Raw += "\n" + strings.TrimSpace(s)
			continue
		}
		if r := capRow(s); r != nil && onlyOfRe.MatchString(a.clean(s)) {
			a.rowUnder(nil, r) // "only 0.5 of ECO220Y1 will be accepted towards the program"
			continue
		}
		switch {
		case role == RoleRequirement && isRelaxation(s) && !(omitRe.MatchString(s) && a.omitCovered(s)):
			a.relaxations = append(a.relaxations, s)
		case role == RoleRequirement && thisRequirementRe.MatchString(s) && a.lastRule(owner) != nil:
			// "will also meet this requirement": a route for the rule before it
			k := a.lastRule(owner)
			k.r.Req = programs.OrNode(k.r.Req, TextNode(altKind(s), s))
			addRaw(k, s)
		case role == RoleRequirement && !a.IsApproval(s) && a.IsAlternative(s):
			a.alternative(owner, s)
		case role == RoleRequirement && !a.IsApproval(s):
			a.looseSentence(owner, s)
		default:
			owner.r.Notes = append(owner.r.Notes, a.sentence(s, nil, compCtx{owner: owner})...)
		}
	}
	for _, c := range b.Children {
		a.about(c, owner, nil)
	}
}

// subListTitleRe matches a short name over a sub-list: "Politics &
// Policy".
var subListTitleRe = regexp.MustCompile(`^[A-Z][\w&,/'’ -]{0,60}$`)

// prerequisiteLabelRe matches a sub-list of more members for some
// students: "For those with the appropriate prerequisites".
var prerequisiteLabelRe = regexp.MustCompile(`(?i)^for\s+(?:those|students)\s+with\b`)

// prerequisiteMembers adds the courses of a prerequisiteLabelRe sub-list
// to owner's pool of listed courses (the label is the row's note), and
// reports whether it did.
func (a *Assembler) prerequisiteMembers(b *Block, owner *compRow) bool {
	pool, caps := poolCaps(owner.r.Req)
	if b.Kind != blockLabelled || !prerequisiteLabelRe.MatchString(b.Label) || !IsCodeList(b.Text) || len(b.Children) > 0 ||
		!isPool(pool) || pool.Courses == nil {
		return false
	}
	wide := grammar.CloneNode(pool)
	wide.Courses = grammar.Dedupe(append(slices.Clone(pool.Courses), grammar.CodeRe.FindAllString(b.Text, -1)...))
	owner.r.Req = programs.AndNode(append([]*types.ReqNode{wide}, caps...)...)
	owner.r.Notes = append(owner.r.Notes, grammar.CleanNote(BlockLine(b)))
	addRaw(owner, a.line(b))
	return true
}

// poolAdditionRe matches a line under a pool that adds to it: "Any course
// (0.5 credit) from Group I".
var poolAdditionRe = regexp.MustCompile(`(?i)^(?:any\s+(?:` + grammar.DesigPat + `\s+)?courses?\b|up\s+to\b)[^;]*?\bfrom\b`)

// poolAddition reads a poolAdditionRe sentence about owner's pool and
// reports whether it did: a cap on courses the pool counts is a limit; other
// courses widen it as or(pool, text), so a student using them is Review.
func (a *Assembler) poolAddition(owner *compRow, s string) bool {
	t := a.clean(s)
	if owner.r.Req == nil || !poolAdditionRe.MatchString(t) || bindingWordRe.MatchString(t) && !capLeadRe.MatchString(t) {
		return false
	}
	r := capRow(s)
	if r != nil && r.Req.Courses != nil && !slices.ContainsFunc(r.Req.Courses, func(c string) bool { return !rowNames(owner, c) }) {
		a.rowUnder(owner, r)
		return true
	}
	if capLeadRe.MatchString(t) && r != nil && !grammar.CodeRe.MatchString(t) {
		return false // "Up to 1.0 credit at the 400-level ...": a cap on the pool
	}
	a.alternative(owner, s)
	return true
}

// titleCodeAdviceRe matches a titled course and advice on it:
// "Professional Experience Course ACT390H1: students are strongly recommended".
// Groups 2 and 3 are the course and the advice.
var titleCodeAdviceRe = regexp.MustCompile(`^([^:]*?\b((?:` + grammar.CodePat + `\s*/\s*)*` + grammar.CodePat + `))\s*:\s*(.+)$`)

// orContinuationRe matches a line adding alternatives to the item above
// it: "(or one of ECO220Y1, PSY201H1)".
var orContinuationRe = regexp.MustCompile(`(?i)^\(?\s*or\s+(?:one\s+of\s+)?((?:` + grammar.CodePat + `\s*(?:,|/|or)?\s*)+)\)?\.?$`)

// parts gives an item with no body its parts' requirement: a choose under a
// count lead, an or of alternatives, an and of parts that share no course;
// parts that can share stay child allocations, so no course counts for two.
func (a *Assembler) parts(row *compRow, b *Block) {
	var parts, rest []*compRow
	for _, k := range row.children {
		if k.r.Kind == types.RequirementAllocation && k.r.Req != nil {
			parts = append(parts, k)
		} else {
			rest = append(rest, k)
		}
	}
	if len(parts) == 0 {
		return
	}
	alternatives := eitherLeadRe.MatchString(b.Text) || slices.ContainsFunc(parts, func(k *compRow) bool { return trailingOrRe.MatchString(k.r.Raw) })
	count := 0
	if m := countLeadRe.FindStringSubmatch(b.Text); m != nil {
		count = grammar.CountWords[strings.ToLower(m[1])] // "Two of the following selections:"
	}
	var nodes []*types.ReqNode
	nested := false
	for _, k := range parts {
		nodes = append(nodes, k.r.Req)
		nested = nested || len(k.children) > 0
	}
	switch {
	case nested:
		return // a part has rows of its own: keep the group
	case count > 1 && count < len(parts):
		row.r.Req = &types.ReqNode{Type: types.NodeChoose, Count: count, Items: nodes}
	case len(parts) == 1 || alternatives:
		row.r.Req = programs.OrNode(nodes...)
	case !slices.ContainsFunc(nodes[1:], func(n *types.ReqNode) bool {
		return sharesCourses(programs.AndNode(nodes[:slices.Index(nodes, n)]...), n)
	}):
		row.r.Req = programs.AndNode(nodes...)
	default:
		return
	}
	for _, k := range parts {
		row.r.Raw += "\n" + k.r.Raw
		row.r.Notes = append(row.r.Notes, k.r.Notes...)
	}
	row.children = rest
}

// splitShared turns an item whose parts can share a course ("BIO220H1 and
// 1.0 credit from BIO220H1, ...") into a group of child allocations, one per
// part, so that no course counts for two.
func (a *Assembler) splitShared(row *compRow) {
	n := row.r.Req
	if n == nil || n.Type != types.NodeAnd {
		return
	}
	shared := false
	for i, it := range n.Items {
		if !claimable(it) {
			return // text or a cap: not parts to claim courses for
		}
		for _, other := range n.Items[i+1:] {
			shared = shared || sharesCourses(it, other)
		}
	}
	if !shared {
		return
	}
	var parts []*compRow
	for _, it := range n.Items {
		parts = append(parts, &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Req: it, Raw: row.r.Raw}})
	}
	row.r.Req = nil
	row.children = append(parts, row.children...)
}

// itemLabel is the calendar's label for an item: its number ("Higher Years
// 2") or a labelled line's name; not a year, which is the section, nor a
// note's label.
func itemLabel(b *Block) string {
	switch {
	case b.Kind == blockItem:
		return b.Label
	case b.Kind == blockLabelled && !blkStartsSection(b) && !BlkNoteLabelRe.MatchString(b.Label):
		return b.Label
	case b.Kind == blockLabelled && sectionLabel(b.Label):
		// "Fourth Year Courses: Any 2 courses ...": the label names its section
		return b.Label
	}
	return ""
}

// claimable reports whether n only needs courses: courses, credit amounts and
// their and, or and choose. No text, cap or average.
func claimable(n *types.ReqNode) bool {
	return structureOnly(n, func(x *types.ReqNode) bool {
		return x.Type == types.NodeChoose ||
			x.Type == types.NodeCredits && x.Amount > 0 && x.MaxAmount == nil && !isAnyCredits(x)
	})
}

// sharesCourses reports whether a course could count toward both x and y.
// Text doesn't count: it is Review whatever courses it is given.
func sharesCourses(x, y *types.ReqNode) bool {
	cx, fx := poolParts(x)
	cy, fy := poolParts(y)
	for _, c := range cx {
		if slices.Contains(cy, c) || slices.ContainsFunc(fy, func(f *types.ReqNode) bool { return grammar.FilterMatches(f, c) }) {
			return true
		}
	}
	for _, c := range cy {
		if slices.ContainsFunc(fx, func(f *types.ReqNode) bool { return grammar.FilterMatches(f, c) }) {
			return true
		}
	}
	for _, f := range fx {
		for _, g := range fy {
			if filtersMeet(f, g) {
				return true
			}
		}
	}
	return false
}

// poolParts collects the codes and the filters (credits nodes matching by
// designator, level or breadth, or any course) in n.
func poolParts(n *types.ReqNode) (codes []string, filters []*types.ReqNode) {
	grammar.WalkReq(n, func(x *types.ReqNode) {
		switch x.Type {
		case types.NodeCourse:
			codes = append(codes, x.Code)
		case types.NodeCredits, types.NodeAverage:
			codes = append(codes, x.Courses...)
			if x.Designators != nil || x.Levels != nil || x.Breadth != nil || x.Courses == nil {
				filters = append(filters, x)
			}
		}
	})
	return codes, filters
}

// filtersMeet reports whether some course could match both filters.
func filtersMeet(f, g *types.ReqNode) bool {
	if f.Designators != nil && g.Designators != nil && !slices.ContainsFunc(f.Designators, func(d string) bool { return slices.Contains(g.Designators, d) }) {
		return false
	}
	if f.Levels != nil && g.Levels != nil && !slices.ContainsFunc(f.Levels, func(l int) bool { return slices.Contains(g.Levels, l) }) {
		return false
	}
	return true
}

// sectionLabel reports whether label starts a section and also names the item:
// the year phrase followed by "Courses".
func sectionLabel(label string) bool {
	p := blkYearPrefix(label)
	if p == "" {
		return false
	}
	rest := strings.TrimSpace(blkOutsideParens(label))
	return strings.HasPrefix(strings.ToLower(rest), strings.ToLower(p)) && yearCoursesRe.MatchString(rest[len(p):])
}
