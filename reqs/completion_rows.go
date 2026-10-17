package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Reading blocks into rows: the header total, items, their parts and the lines
// under them. Each sentence's role decides where it goes.

var (
	// complCodeListLineRe matches a line that only lists codes: "ECO365H1,
	// ECO368H1, TRN410H1".
	complCodeListLineRe = regexp.MustCompile(`^(?:` + codePat + `|[A-Z]{3}\d{4}[HY]\d?|[\s,;/().*+&]|(?i:and|or)\b)+$`)
	// complTitleParenRe matches brackets, a course's title when they hold no
	// code: "(Nuclear Weapons and International Politics)".
	complTitleParenRe = regexp.MustCompile(`\([^()]*\)`)
	// complEitherLeadRe matches a lead that makes the items under it
	// alternatives: "One of the following:".
	complEitherLeadRe = regexp.MustCompile(`(?i)\beither\b|\bone\s+of\b`)
	// complCountLeadRe matches a lead that picks some of the items under it:
	// "Two of the following selections:". Group 1 is the count.
	complCountLeadRe = regexp.MustCompile(`(?i)\b(two|three|four|five|[2-5])\s+of\s+the\s+following\b`)
	// complTrailingOrRe matches an item's trailing conjunction: "CSC165H1 or".
	complTrailingOrRe = regexp.MustCompile(`(?i)\s+or\s*$`)
	// complTrailingCommaOrRe matches a line ending in ", OR", whose next line
	// is its alternative.
	complTrailingCommaOrRe = regexp.MustCompile(`(?:,\s*|\s+)OR\s*$`)
	// complFloorsLeadRe matches a lead whose items are floors on it: "These 3.0
	// credits must include:".
	complFloorsLeadRe = regexp.MustCompile(`(?i)\b(?:including|must\s+include|of\s+which)\s*:\s*$`)
	// complItemFloorParenRe matches a floor in brackets right after an item's
	// amount: "1.5 credits (at least 0.5 credit at the 400-level) from: ...".
	complItemFloorParenRe = regexp.MustCompile(`^(.*?\d+(?:\.\d+)?\s+credits?)\s*\(([^()]*\b(?:at\s+least|must|minimum)\b[^()]*)\)\s*(.*)$`)
	// complCapScopeRe matches the scope a cap ends with: "can be used towards
	// this requirement". The cap applies where it is written.
	complCapScopeRe = regexp.MustCompile(`(?i)\s+(?:can|may|will)\s+(?:be\s+)?(?:used|counted|count|applied|accepted)\s+(?:to(?:wards?)?|for)\s+(?:fulfill\s+|satisfy\s+)?(?:this|the|any|our)\s+(?:[\w-]+\s+)?(?:requirements?|program|minor|major|specialist|certificate|focus)\b.*$`)
)

// blocks reads a list of sibling blocks.
func (a *compAssembler) blocks(bs []*compBlock, ctx compCtx) {
	for i := 0; i < len(bs); i++ {
		b := bs[i]
		if n := a.numberedLists(bs[i:], ctx); n > 0 {
			// numbered lists with their floors and caps: one item
			i += n - 1
			continue
		}
		if b.kind != blockHeading && !blkNoteLabelRe.MatchString(b.label) && (a.underAdviceHeading(b) || a.adviceLabel(b)) {
			// advice: "Suggested Related Courses:" then "- CSC367H1"
			a.adviceBlock(b, ctx)
			continue
		}
		if n := a.optionBlocks(bs[i:], ctx); n > 0 {
			// options: "Choose one of the options below:" then "Option A: ..."
			i += n - 1
			continue
		}
		if n := a.adviceZone(bs[i:]); n > 0 {
			// a lead into advice and the lines it lists: one note
			note, rest := a.adviceNote(blockLine(b), bs[i+1:i+1+n])
			a.notesTo(ctx.owner, []string{note})
			a.blocks(rest, ctx)
			i += n
			continue
		}
		if i > 0 && isListTitle(bs[i-1]) && len(b.children) == 0 && mostlyCodes(b.text) && !blkNumbered(b) {
			// a list's members under its title, with their own label if any:
			// "Group A: ..." then "ANT210H1, ANT215H1, ..."
			title := strings.TrimSuffix(cleanNote(blockLine(bs[i-1])), ":")
			a.listDef(ctx.owner, title, blockLine(b), b, title+": "+cleanNote(blockLine(b)))
			continue
		}
		// "X, OR" then "Y": one requirement
		if i+1 < len(bs) && b.kind != blockHeading && bs[i+1].kind != blockHeading && len(b.children) == 0 && complTrailingCommaOrRe.MatchString(b.text) {
			joined := *b
			joined.text = b.text + " " + bs[i+1].text
			joined.children = bs[i+1].children
			a.joined[&joined] = bs[i+1]
			a.block(&joined, ctx, nil)
			i++
			continue
		}
		// a lead-in and the line of codes after it are one requirement
		if i+1 < len(bs) && b.kind != blockHeading && len(b.children) == 0 && (complCodeLineLeadRe.MatchString(b.text) || complPoolListedBelowRe.MatchString(b.text) ||
			complRemainingLeadRe.MatchString(b.text)) && isCodeListLine(bs[i+1]) {
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

// complPoolRuleLineRe matches a line that adds to or caps the pool before it:
// "No more than one from".
var complPoolRuleLineRe = regexp.MustCompile(`(?i)^(?:no\s+more\s+than|up\s+to|at\s+most|any\s+(?:(?-i:[A-Z]{3})\s+)?courses?)\b`)

// poolRuleLine reads b, the line after prev, as a rule on the pool of the lead
// prev ends, and reports whether it did: prose right after the sub-lists that
// adds to or caps them is about the item.
func (a *compAssembler) poolRuleLine(prev, b *compBlock) bool {
	row := a.leadRows[prev]
	for k := prev; row == nil && len(k.children) > 0; {
		k = k.children[len(k.children)-1]
		row = a.leadRows[k]
	}
	if row == nil || row.r.Req == nil || b.kind != blockProse || len(b.children) > 0 || !complPoolRuleLineRe.MatchString(strings.TrimSpace(b.text)) {
		return false
	}
	row.r.Raw += "\n" + a.line(b)
	a.about(b, row, nil)
	a.leadRows[b] = row
	return true
}

// isCodeListLine reports whether b is a line of prose that only lists
// codes.
func isCodeListLine(b *compBlock) bool {
	return b.kind == blockProse && len(b.children) == 0 && isCodeList(b.text)
}

// isCodeList reports whether s only lists codes, titles in brackets aside.
func isCodeList(s string) bool {
	if !codeRe.MatchString(s) {
		return false
	}
	s = complTitleParenRe.ReplaceAllStringFunc(s, func(p string) string {
		if codeRe.MatchString(p) {
			return p
		}
		return ""
	})
	return complCodeListLineRe.MatchString(s)
}

// block reads one block; list is the line of codes its lead-in takes, if
// any.
func (a *compAssembler) block(b *compBlock, ctx compCtx, list *compBlock) {
	if ctx.owner == nil && !ctx.advice && b.kind != blockItem && b.kind != blockBullet && a.header(b, list) {
		a.blocks(b.children, ctx)
		return
	}
	if b.kind == blockHeading {
		if !a.adviceHeading(b) && !a.underAdviceHeading(b) {
			a.headingFloors(b, ctx)
		}
		a.blocks(b.children, ctx)
		return
	}
	a.item(b, ctx, list)
}

// complAdviceHeadingRe matches a heading that names advice: "Suggested Related
// Courses".
var complAdviceHeadingRe = regexp.MustCompile(`(?i)\b(?:recommended|suggested|suggestions?)\b`)

// adviceHeading reports whether heading h is advice: it says so and the
// classifier rates it advisory. A notes heading is not: notes hold rules too.
func (a *compAssembler) adviceHeading(h *compBlock) bool {
	return h.kind == blockHeading && !h.isNotes() && complAdviceHeadingRe.MatchString(h.text) && a.role(h.text) == roleAdvisory
}

// underAdviceHeading reports whether a heading over b, at any depth, is advice.
func (a *compAssembler) underAdviceHeading(b *compBlock) bool {
	for h := b.heading; h != nil; h = h.heading {
		if a.adviceHeading(h) {
			return true
		}
	}
	return false
}

// adviceLabel reports a labelled line whose label is advice: "Other courses
// which might of interest: ESS391H1, ...".
func (a *compAssembler) adviceLabel(b *compBlock) bool {
	return b.kind == blockLabelled && !blkNoteLabelRe.MatchString(b.label) && complAdviceLabelRe.MatchString(b.label) && a.role(b.label) == roleAdvisory
}

// complAdviceLabelRe matches a label that names advice: "might be of interest".
var complAdviceLabelRe = regexp.MustCompile(`(?i)\b(?:recommended|suggested|might\s+(?:be\s+)?of\s+interest)\b`)

// adviceBlock reads an advice block and the blocks under it: its sentences are
// one note, prefixed with the heading they are under. An approval route, an
// exclusion or a cap among them is still read as one.
func (a *compAssembler) adviceBlock(b *compBlock, ctx compCtx) {
	var keep []string
	for _, s := range splitComplSentences(blockLine(b)) {
		switch role := a.role(s); {
		case role == roleExclusion:
			a.exclusion(s)
		case role == roleRequirement && a.isApproval(s):
			a.approval(ctx.owner, s)
		default:
			// a cap in advice is still a cap
			parts := splitTop(s, ';')
			var advice []string
			for i, p := range parts {
				p = strings.TrimSpace(p)
				if r := capRow(p); i > 0 && r != nil && complCapLeadRe.MatchString(p) {
					a.rowUnder(ctx.owner, r)
					continue
				}
				advice = append(advice, p)
			}
			keep = append(keep, strings.TrimSuffix(cleanNote(strings.Join(advice, "; ")), "."))
		}
	}
	if keep != nil {
		note := strings.Join(keep, ". ")
		if h := b.heading; h != nil && h.section != "" && b.label != h.section {
			note = h.section + ": " + note
		}
		a.notesTo(ctx.owner, []string{note})
	}
	for _, c := range b.children {
		a.adviceBlock(c, ctx)
	}
}

// blockLine is a block's text with its label: "Ecology: EEB255H1, ...". A
// note's label ("Note:") is left out.
func blockLine(b *compBlock) string {
	if b.kind == blockLabelled && !blkNoteLabelRe.MatchString(b.label) {
		return b.label + ": " + b.text
	}
	return b.text
}

// complAdviceLeadEndRe matches the end of a lead into what follows: "are as
// follows."
var complAdviceLeadEndRe = regexp.MustCompile(`(?i)(?::|\bas\s+follows\s*[.:]?|\(as\s+listed\s+below\)\.?)\s*$`)

// complNotRequirementLeadRe matches a lead saying that what follows is no
// requirement, which points forward whatever its end: "streams are not an
// official program requirement".
var complNotRequirementLeadRe = regexp.MustCompile(`(?i)\bare\s+not\s+an?\s+official\s+(?:program\s+)?requirement\b`)

// complAdviceForwardRe matches advice pointing at what follows: "we suggest you
// consider the following courses".
var complAdviceForwardRe = regexp.MustCompile(`(?i)\bthe\s+following\b|\bas\s+follows\b|\bbelow\b|:\s*$`)

// adviceZone returns how many blocks after bs[0] are the lines of advice bs[0]
// leads into, or 0: bs[0] ends in ":" or "as follows", with advice pointing
// forward and no rule, and the lines are labelled lines, bullets and codes.
func (a *compAssembler) adviceZone(bs []*compBlock) int {
	b := bs[0]
	notRequired := complNotRequirementLeadRe.MatchString(b.text)
	if b.kind != blockProse && !(b.kind == blockLabelled && blkNoteLabelRe.MatchString(b.label)) || len(b.children) > 0 ||
		!complAdviceLeadEndRe.MatchString(strings.TrimSpace(b.text)) && !notRequired {
		return 0
	}
	forward := notRequired
	for _, s := range splitComplSentences(b.text) {
		switch role := a.role(s); {
		case role == roleAdvisory:
			forward = forward || complAdviceForwardRe.MatchString(s)
		case role == roleRequirement && !a.isApproval(s) && !a.isAlternative(s):
		default:
			return 0
		}
	}
	if !forward {
		return 0
	}
	n := 0
	for _, c := range bs[1:] {
		if c.kind == blockHeading && !c.isNotes() && !blkAmountRe.MatchString(c.text) && len(c.children) == 0 {
			// a heading naming who the advice is for
			n++
			continue
		}
		// a line whose emphasis kept its label from being one is a member too
		_, after, colon := strings.Cut(c.text, ": ")
		member := c.kind == blockLabelled && !blkNoteLabelRe.MatchString(c.label) || c.kind == blockBullet || isCodeListLine(c) ||
			c.kind == blockProse && len(c.children) == 0 && colon && mostlyCodes(after)
		if !member || blkNumbered(c) || !codeRe.MatchString(c.text) && !complAmountRe.MatchString(c.text) {
			break
		}
		n++
	}
	for n > 0 && bs[n].kind == blockHeading {
		n-- // a heading the zone ends on belongs to what follows
	}
	return n
}

// fociHeading returns the heading over b that names the program's focuses
// ("**Disciplinary Focuses**"), or nil.
func fociHeading(b *compBlock) *compBlock {
	for h := b.heading; h != nil; h = h.heading {
		if h.kind == blockHeading && blkFociRe.MatchString(h.section) {
			return h
		}
	}
	return nil
}

// complFocusChoiceRe matches the rule focuses come with: "must enrol in and
// complete at least one disciplinary focus".
var complFocusChoiceRe = regexp.MustCompile(`(?i)\bcomplete\s+at\s+least\s+one\b[^.]*\bfocus\b`)

// focusLine reads a line under a heading of focuses (h) into the one row they
// make, never an allocation per focus, which every student would then need. The
// rule is the row's body, for focusChoices; the other lines are notes.
func (a *compAssembler) focusLine(h, b *compBlock) {
	row := a.focusRows[h]
	if row == nil {
		row = &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Section: h.section, Raw: a.line(h),
			Req: textNode(types.TextOther, "Complete at least one of the "+h.section)}}
		a.focusRows[h] = row
		a.add(row, nil)
	}
	var walk func(c *compBlock)
	walk = func(c *compBlock) {
		row.r.Raw += "\n" + a.line(c)
		prefix := ""
		if c.heading != nil && c.heading != h && c.heading.kind == blockHeading && !c.heading.isNotes() {
			prefix = c.heading.text + " " // the focus's heading leads its first note
		}
		for _, s := range splitComplSentences(blockLine(c)) {
			if complFocusChoiceRe.MatchString(s) {
				row.r.Req = textNode(types.TextOther, s)
				continue
			}
			row.r.Notes = append(row.r.Notes, cleanNote(prefix+s))
			prefix = ""
		}
		for _, k := range c.children {
			walk(k)
		}
	}
	walk(b)
}

var (
	// complFocusCodeRe matches a focus line with its code: "Health Studies
	// (ASFOC2270A): (2.0 credits)". Groups 1 and 2 are the name and the code.
	complFocusCodeRe = regexp.MustCompile(`^(.+?)\s+\(([A-Z]{2}FOC\d{4}[A-Z])\)\s*:`)
	// complFocusNoCodeRe matches a focus line without one: "Immunology (3.0
	// Credits) - ".
	complFocusNoCodeRe = regexp.MustCompile(`^([A-Z][^():]*?)\s+\(\d+(?:\.\d+)?\s+(?i:credits?)\)\s+-\s`)
)

// focusChoices makes a focus heading's rule the or of its focuses' program
// nodes, as enrolment writes a focus. A coded focus's line stays beside its
// node (focusPart); an uncoded one is named only, for Link to resolve by title.
func (a *compAssembler) focusChoices() {
	for _, row := range a.focusRows {
		n := row.r.Req
		if n == nil || n.Type != types.NodeText || !complFocusChoiceRe.MatchString(n.Text) {
			continue
		}
		// the focuses run from the rule's line to the heading's last; the row's
		// raw lacks the focuses' own headings
		own := map[string]bool{}
		for _, l := range strings.Split(row.r.Raw, "\n") {
			own[rawWords(l)] = true
		}
		start := slices.IndexFunc(a.lines, func(l string) bool { return complFocusChoiceRe.MatchString(l) && own[rawWords(l)] })
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
			if l := strings.TrimSpace(a.lines[i]); complFocusCodeRe.MatchString(l) || complFocusNoCodeRe.MatchString(l) {
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
			if m := complFocusCodeRe.FindStringSubmatch(l); m != nil {
				p := &types.ReqNode{Type: types.NodeProgram, Programs: []string{m[2]}, Text: m[1]}
				head, body := a.focusLineAt(i, m)
				part, text, ns, ls := a.focusPart(row, p, head, body)
				foci, texts, notes, limits = append(foci, part), append(texts, text), append(notes, ns), append(limits, ls)
				continue
			}
			m := complFocusNoCodeRe.FindStringSubmatch(l)
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
		row.r.Req = &types.ReqNode{Type: types.NodeOr, Items: dedupeNodes(foci)}
	}
}

// requiredSoFar returns the codes the items read so far name in their raw.
func (a *compAssembler) requiredSoFar() map[string]bool {
	seen := map[string]bool{}
	var walk func(rs []*compRow)
	walk = func(rs []*compRow) {
		for _, r := range rs {
			for _, c := range codeRe.FindAllString(r.r.Raw, -1) {
				seen[c] = true
			}
			walk(r.children)
		}
	}
	walk(a.items)
	return seen
}

var (
	// complTotalPoolLeadRe matches a line giving the total its pool (group 2)
	// and leading into conditions on it: "At least 6.0 FAH credits fulfilling
	// the following distribution requirements:".
	complTotalPoolLeadRe = regexp.MustCompile(`(?i)^(?:at\s+least\s+)?(\d+(?:\.\d+)?)\s+(?:\(\d+(?:\.\d+)?\)\s+)?((?-i:[A-Z]{3})(?:\s*(?:/|,|or|and/or)\s*(?-i:[A-Z]{3}))*)\s+credits?\s+(?:fulfilling|meeting|that\s+meet|satisfying)\s+the\s+following\b[^.]*:?$`)
	// complConditionFloorRe matches an item that is a floor on the total's
	// pool: "3.0 credits at the 300+ level".
	complConditionFloorRe = regexp.MustCompile(`(?i)^(?:at\s+least\s+)?(?:one\s+|an?\s+)?\d+(?:\.\d+)?\s+credits?\s+at\s+the\b`)
)

// totalPoolLead reads a complTotalPoolLeadRe line into the total's pool, and
// reports whether it did: the items after it that are floors are the pool's
// conditions.
func (a *compAssembler) totalPoolLead(b *compBlock) bool {
	m := complTotalPoolLeadRe.FindStringSubmatch(strings.TrimSpace(b.text))
	t := a.total
	if m == nil || t == nil || t.r.Req.Type != types.NodeCredits || t.r.Req.Designators != nil || t.r.Req.Courses != nil {
		return false
	}
	t.r.Req.Designators = desigRe.FindAllString(m[2], -1)
	t.r.Raw += "\n" + a.line(b)
	a.poolConditions = true
	return true
}

// poolCondition makes an item that is a floor on the total's pool a constraint
// under the total with the pool's designators, after a lead into the pool's
// conditions. It reports whether it did.
func (a *compAssembler) poolCondition(row *compRow, text string) bool {
	if !a.poolConditions || a.total == nil {
		return false
	}
	splitFloors(row, text) // "At least 2.0 credits at the 300+ level, 0.5 of which ..."
	n := row.r.Req
	t := a.total
	if !a.poolConditions || t == nil || n == nil || n.Type != types.NodeCredits || n.Levels == nil || n.Designators != nil ||
		n.Courses != nil || n.Items != nil || !complConditionFloorRe.MatchString(strings.TrimSpace(text)) {
		return false
	}
	n.Designators = slices.Clone(t.r.Req.Designators)
	row.r.Kind = types.RequirementConstraint
	row.r.Label, row.r.Section = "", ""
	a.floors = append(a.floors, row)
	for _, k := range row.children {
		if k.r.Kind == types.RequirementConstraint && k.r.Req != nil && k.r.Req.Type == types.NodeCredits && k.r.Req.Designators == nil {
			k.r.Req.Designators = slices.Clone(t.r.Req.Designators)
		}
		a.floors = append(a.floors, k)
	}
	row.children = nil
	return true
}

// complRemainingLeadRe matches a lead whose group is the line of codes after
// it: "Other Electives (1.5-2.5 credits): Remaining credits from the SDS
// Electives Group".
var complRemainingLeadRe = regexp.MustCompile(`(?i)^[^:(]*\((\d+(?:\.\d+)?)(?:\s*[-–]\s*\d+(?:\.\d+)?)?\s+credits?\)\s*:\s*remaining\s+credits\s+(?:selected\s+)?from\s+the\s+[^.]+?\bgroup\.?$`)

// complHeadingFloorRe matches a floor in a heading's brackets: "Fourth Year:
// (at least 2.0 credits at 400-level)".
var complHeadingFloorRe = regexp.MustCompile(`(?i)\(\s*(?:[^()]*?,\s*)?(at\s+least\s+[^()]+?)\s*\)`)

// headingFloors keeps a floor stated in a heading as a constraint on the
// program (or the item the heading is in): the heading is for display only, but
// its floor is a rule.
func (a *compAssembler) headingFloors(b *compBlock, ctx compCtx) {
	m := complHeadingFloorRe.FindStringSubmatch(b.text)
	if m == nil {
		return
	}
	for _, r := range a.floorRows(m[1]) {
		r.Raw = a.line(b)
		a.rowUnder(ctx.owner, r)
	}
}

// header reads a header line into Req 1 and its floors and reports whether b
// was one. A line restating the total ("Students complete 10.0 credits,
// including:" after "(10.0 credits, ...)") adds its floors to it.
func (a *compAssembler) header(b *compBlock, list *compBlock) bool {
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
		n, _ := parseCompletionText(fmtAmount(h.total) + " credits from: " + list.text)
		n, caps := poolCaps(n)
		if complPoolListedBelowRe.MatchString(text) && isPool(n) {
			n.Designators, n.Levels = slices.Clone(a.total.r.Req.Designators), slices.Clone(a.total.r.Req.Levels)
			a.total.r.Req = n
			for _, c := range caps {
				a.floors = append(a.floors, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Req: c, Raw: a.total.r.Raw}})
			}
		} else {
			a.textRow(nil, list.text)
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
		for _, sent := range splitComplSentences(s) {
			a.notesTo(nil, a.sentence(sent, nil, compCtx{}))
		}
	}
	return true
}

// item reads a block that isn't a header or a heading, and the blocks under
// it. Its first requirement sentence is a new allocation's body; the others are
// rules on it, notes or alternatives. list is the line its lead-in takes.
func (a *compAssembler) item(b *compBlock, ctx compCtx, list *compBlock) {
	if ctx.owner != nil && ctx.owner.r.Req != nil {
		a.about(b, ctx.owner, list)
		return
	}
	if ctx.owner == nil && !blkNumbered(b) && len(b.children) == 0 && a.totalPoolLead(b) {
		return
	}
	if h := fociHeading(b); ctx.owner == nil && h != nil {
		a.focusLine(h, b)
		return
	}
	if ctx.owner == nil && a.listLine(b) {
		// a list the items draw on, never a requirement: a list definition,
		// kept as a note until a row uses it
		title := b.label
		if b.kind != blockLabelled && b.heading != nil {
			title = b.heading.section
		}
		a.listDef(nil, title, b.text, b, a.listNote(b))
		return
	}
	ctx.notes = ctx.notes || blkNoteLabelRe.MatchString(b.label) || b.heading != nil && b.heading.isNotes()
	ctx.numbered = blkNumbered(b)
	if ctx.owner == nil && ctx.numbered {
		a.numberedSeen = true
	}
	row := &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Label: itemLabel(b), Section: b.section, Raw: a.line(b)}}
	kids := b.children
	if list == nil && a.leadLines(b, row, ctx) {
		return // lines too short for sub-lists
	}
	if n := leadingSubLines(kids); list == nil && n >= 2 && complAmountRe.MatchString(b.text) &&
		(strings.HasSuffix(strings.TrimSpace(b.text), ":") || (complTheFollowingRe.MatchString(b.text) || complFromListBelowRe.MatchString(b.text)) && !codeRe.MatchString(b.text)) {
		// one item over all the sub-lists; the lines after them are about it
		row := a.listLead(b, kids[:n], ctx)
		a.children(kids[n:], b.text, ctx, row)
		return
	}
	if list == nil && len(kids) > 0 && (strings.HasSuffix(b.text, ":") || complTheFollowingRe.MatchString(b.text) && !codeRe.MatchString(b.text) && (len(kids) == 1 || !subLine(kids[1]))) &&
		len(kids[0].children) == 0 && isCodeList(kids[0].text) {
		// a lead-in over one line of codes, which is its list
		list, kids = kids[0], kids[1:]
	}
	text := b.text
	if b.kind == blockLabelled && complListLabelRe.MatchString(b.label+":") {
		// "Group A: CRI300H1/ ...": the label says the line is a list.
		text = b.label + ": " + text
	}
	if list != nil {
		if m := complRemainingLeadRe.FindStringSubmatch(strings.TrimSpace(text)); m != nil {
			// "Remaining credits from the SDS Electives Group" then its codes:
			// the lower end of the range from them
			text = m[1] + " credits from: " + list.text
		} else if loc := complTheFollowingRe.FindStringIndex(text); loc != nil && !strings.HasSuffix(strings.TrimSpace(text), ":") && !codeRe.MatchString(text) {
			// "from the following list, ..." then codes: the codes are the list
			text = text[:loc[0]] + strings.TrimSpace(list.text) + text[loc[1]:]
		} else {
			text = strings.TrimSuffix(strings.TrimSpace(text), ":") + ": " + list.text
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
	if b.kind == blockLabelled && mostlyCodes(b.text) && !blkNumbered(b) && (complChoiceListLabelRe.MatchString(b.label) || a.namedElsewhere(b)) {
		// a list an item draws on, never all required: "Approved Economics
		// courses: ECO313H1; ..."
		a.listDef(ctx.owner, b.label, b.text, b, b.label+": "+cleanNote(b.text))
		return
	}
	if inListHeading(b) && mostlyCodes(b.text) {
		// a list's members under its heading: "Group A: Ancient, Medieval"
		a.listDef(ctx.owner, b.heading.section, text, b, b.heading.section+": "+cleanNote(text))
		return
	}
	if mayBeList(b) {
		// many courses and no amount under a topic heading: most likely a list,
		// maybe all required, so Review
		a.textRow(ctx.owner, a.line(b))
		return
	}
	if ctx.owner == nil && len(kids) > 0 && complOfTheseLeadRe.MatchString(a.clean(text)) {
		// "Of these:" then floors and caps on the total, one per line
		for _, c := range kids {
			a.programFloors(c)
		}
		return
	}
	// a title over parts, with no requirement of its own
	title := len(kids) > 0 && !codeRe.MatchString(text) && !complAmountRe.MatchString(text)
	if next := a.joined[b]; next != nil && ctx.owner == nil {
		// "X, OR" then "Y": the item is either line.
		left, lrows, lnotes := a.itemBody(complTrailingCommaOrRe.ReplaceAllString(strings.TrimSpace(strings.TrimSuffix(b.text, next.text)), ""))
		right, rrows, rnotes := a.itemBody(next.text)
		row.r.Req = orNode(left, right)
		for _, r := range append(lrows, rrows...) {
			a.rowUnder(row, r)
		}
		row.r.Notes = append(lnotes, rnotes...)
		a.add(row, ctx.owner)
		return
	}
	if m := complTitleCodeAdviceRe.FindStringSubmatch(strings.TrimSpace(text)); m != nil && ctx.numbered && len(kids) == 0 && a.allAdvice(m[3]) {
		// "Professional Experience Course ACT390H1: students are strongly
		// recommended ...": the course, with the advice as its note
		if n, _ := parseCompletionText(m[2]); n != nil && !hasText(n) {
			row.r.Req = n
			row.r.Notes = append(row.r.Notes, cleanNote(m[3]))
			a.add(row, ctx.owner)
			return
		}
	}
	var notes []string
	relaxed := len(a.relaxations)
	for _, s := range a.routeSentences(splitComplSentences(text)) {
		if title && a.role(s) == roleRequirement && !a.isApproval(s) {
			continue
		}
		notes = append(notes, a.sentence(s, row, ctx)...)
	}
	if b.kind == blockLabelled && !blkNoteLabelRe.MatchString(b.label) && len(notes) > 0 && row.r.Req == nil && len(row.children) == 0 &&
		!strings.HasPrefix(notes[0], b.label) {
		// the label is part of the note: "Consult Program Administrator: ..."
		notes[0] = b.label + ": " + notes[0]
	}
	if len(a.relaxations) > relaxed && row.r.Req == nil && len(row.children) == 0 && len(kids) > 0 {
		// a relaxation leading into options: the options are the relaxation's
		last := len(a.relaxations) - 1
		for _, k := range kids {
			a.relaxations[last] += " " + blockLine(k)
			for _, kk := range k.children {
				a.relaxations[last] += " " + blockLine(kk)
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
		a.children(kids, b.text, ctx, nil)
		return
	}
	row.r.Notes = append(row.r.Notes, notes...)
	if ctx.owner == nil && a.poolCondition(row, text) {
		return
	}
	a.add(row, ctx.owner)
	a.children(kids, b.text, ctx, row)
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

// splitCaps makes the caps the grammar puts beside an item's pool (from a group
// of alternatives flattened into it) limit rows on the item.
func splitCaps(row *compRow) {
	var caps []*compRow
	var strip func(n *types.ReqNode) *types.ReqNode
	strip = func(n *types.ReqNode) *types.ReqNode {
		if n == nil || n.Type != types.NodeAnd && n.Type != types.NodeOr {
			return n
		}
		var keep []*types.ReqNode
		for _, it := range n.Items {
			if n.Type == types.NodeAnd && isCourseCap(it) {
				caps = append(caps, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Req: it, Raw: row.r.Raw}})
				continue
			}
			keep = append(keep, strip(it))
		}
		if n.Type == types.NodeAnd {
			return andNode(keep...)
		}
		return orNode(keep...)
	}
	row.r.Req = strip(row.r.Req)
	row.children = append(row.children, caps...)
}

// isCourseCap reports a cap on listed courses: credits{courses, max_amount}.
func isCourseCap(n *types.ReqNode) bool {
	return n.Type == types.NodeCredits && n.MaxAmount != nil && n.Amount == 0 && n.Courses != nil && n.Items == nil &&
		n.Designators == nil && n.Levels == nil
}

// poolCaps splits and(pool, caps...) into the pool and its caps; anything else
// is returned whole.
func poolCaps(n *types.ReqNode) (*types.ReqNode, []*types.ReqNode) {
	if n == nil || n.Type != types.NodeAnd || len(n.Items) < 2 || !isPool(n.Items[0]) ||
		slices.ContainsFunc(n.Items[1:], func(x *types.ReqNode) bool { return !isCourseCap(x) }) {
		return n, nil
	}
	return n.Items[0], n.Items[1:]
}

// complFloorWordsRe matches wording that makes a narrower credit phrase a floor
// on the one before it: "which includes".
var complFloorWordsRe = regexp.MustCompile(`(?i)\bof\s+which\b|\binclud(?:ing|es?)\b|\bat\s+least\b|\bmust\s+be\b|\bno\s+more\s+than\b`)

// splitFloors makes the floors the grammar puts beside an item's pool
// constraints on the item, never an in-node and: "3.0 SOC credits, which
// includes at least 1.0 SOC credit at the 300-level" is two rows.
func splitFloors(row *compRow, text string) {
	n := row.r.Req
	if n != nil && n.Type == types.NodeOr && len(n.Items) > 1 && n.Items[0].Type == types.NodeAnd &&
		!slices.ContainsFunc(n.Items[1:], func(x *types.ReqNode) bool { return x.Type != types.NodeText }) {
		// or(and(pool, floor), route text): the floors are the item's.
		inner := &compRow{r: &types.Requirement{Req: n.Items[0], Raw: row.r.Raw}}
		splitFloors(inner, text)
		n.Items[0] = inner.r.Req
		row.children = append(inner.children, row.children...)
		return
	}
	if n == nil || n.Type != types.NodeAnd || !complFloorWordsRe.MatchString(text) {
		return
	}
	var main *types.ReqNode
	for _, it := range n.Items {
		if it.Type == types.NodeCredits && it.Items == nil && it.MaxAmount == nil && it.Amount > 0 && (main == nil || it.Amount > main.Amount) {
			main = it
		}
	}
	if main == nil {
		return
	}
	var keep []*types.ReqNode
	var floors []*compRow
	for _, it := range n.Items {
		switch {
		case it != main && narrowsPool(main, it):
			// a copy: the floor shares its filters with the pool
			floors = append(floors, &compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Req: cloneNode(it), Raw: row.r.Raw}})
		case it.Type == types.NodeCredits && it.MaxAmount != nil && it.Amount == 0 && it.Items == nil:
			// a cap on the item: "of which no more than 0.5 credit can be ..."
			floors = append(floors, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Req: cloneNode(it), Raw: row.r.Raw}})
		default:
			keep = append(keep, it)
		}
	}
	if floors == nil {
		return
	}
	row.r.Req = andNode(keep...)
	row.children = append(floors, row.children...)
}

// narrowsPool reports whether credits node c is a share of pool p: a smaller
// amount of the courses p counts, narrowed by a level, designator or
// breadth.
func narrowsPool(p, c *types.ReqNode) bool {
	if codes := courseChoice(c); codes != nil {
		// a course the pool must include: "including CLA160H1/ CLA170H1"
		return !slices.ContainsFunc(codes, func(code string) bool { return !inPool(p, code) })
	}
	if c.Type != types.NodeCredits || c.Items != nil || c.MaxAmount != nil || c.Amount <= 0 || c.Amount >= p.Amount {
		return false
	}
	if isAnyCredits(p) {
		return true // "2.5 credits at least 1.5 credits from PCL courses: ..."
	}
	if c.Courses != nil {
		return !slices.ContainsFunc(c.Courses, func(code string) bool { return !inPool(p, code) })
	}
	return (c.Designators == nil || p.Designators == nil || isSubset(c.Designators, p.Designators)) &&
		(c.Levels != nil || c.Breadth != nil || c.Designators != nil && p.Designators == nil || len(c.Designators) < len(p.Designators))
}

// courseChoice returns the codes of a course or of an "or" of courses
// (courseChoiceCodes), else nil.
func courseChoice(n *types.ReqNode) []string {
	if n.Type == types.NodeCourse {
		return []string{n.Code}
	}
	return courseChoiceCodes(n)
}

// inPool reports whether pool p counts course code.
func inPool(p *types.ReqNode, code string) bool {
	return slices.Contains(p.Courses, code) || (p.Designators != nil || p.Levels != nil) && filterMatches(p, code)
}

func isAllocation(k *compRow) bool { return k.r.Kind == types.RequirementAllocation }

// dissolve removes a lead-in that turned out to have no parts: its rows and
// notes belong to owner (or the program).
func (a *compAssembler) dissolve(row, owner *compRow) {
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

// adviceNote joins advice with the lines under it that only list or
// describe courses, and returns the note and the lines that are rules.
func (a *compAssembler) adviceNote(lead string, kids []*compBlock) (string, []*compBlock) {
	// the lead ends in ":" to lead into its list in one sentence
	note := strings.TrimSuffix(cleanNote(lead), ":") + ":"
	var rest []*compBlock
	var walk func(bs []*compBlock)
	walk = func(bs []*compBlock) {
		for _, c := range bs {
			if !a.listsOnly(c.text) {
				rest = append(rest, c)
				continue
			}
			if c.kind == blockHeading {
				note += " " + strings.TrimSuffix(cleanNote(c.text), ":") + ":"
				continue
			}
			note += " " + cleanNote(blockLine(c))
			walk(c.children)
		}
	}
	walk(kids)
	return note, rest
}

// listsOnly reports whether every sentence of s is advice, a list or a plain
// requirement: no rule, approval or exclusion.
func (a *compAssembler) listsOnly(s string) bool {
	for _, sent := range splitComplSentences(s) {
		switch a.role(sent) {
		case roleAdvisory, roleListDef:
		case roleRequirement:
			if a.isApproval(sent) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

var (
	// complAdditionalTotalRe matches "Additional <courses> to a total of N
	// credits": "Additional HPS courses to a total of 4.0 credits". Groups: the
	// courses, N, the total's designators and the rest.
	complAdditionalTotalRe = regexp.MustCompile(`(?i)^(?:an?\s+)?additional\s+(.+?)\s+to\s+a\s+total\s+of\s+(\d+(?:\.\d+)?)\s+((?-i:[A-Z]{3})(?:\s*/\s*(?-i:[A-Z]{3}))*\s+)?credits?\b(.*)$`)
	// complAdditionalAmountRe matches an amount before the pool: "1.5 to 2.0
	// credits from".
	complAdditionalAmountRe = regexp.MustCompile(`(?i)^\d+(?:\.\d+)?(?:\s*(?:to|-|–)\s*\d+(?:\.\d+)?)?\s+credits?\s+from\s+`)
	// complAdditionalExamplesRe matches examples inside the pool: "which may
	// include courses from among CHM325H1".
	complAdditionalExamplesRe = regexp.MustCompile(`(?i),?\s*which\s+may\s+include\s+courses\s+from\s+among\s+(.+)$`)
	// What may follow the total: "( CHM299H1 excluded)", "(Note: CHM395Y1 can
	// be used ...)", "counting for the program", "and meeting the following two
	// conditions:", ", with at least 1.0 credit at the 300+level".
	complAdditionalExcludedRe   = regexp.MustCompile(`(?i)^\s*\(([^()]*?)\s+excluded\)`)
	complAdditionalAlsoRe       = regexp.MustCompile(`(?i)^\s*\.?\s*\(\s*note:\s*([^()]*?)\s+can\s+be\s+used\s+toward\s+this\s+requirement\s*\)`)
	complAdditionalCountingRe   = regexp.MustCompile(`(?i)^\s*counting\s+for\s+the\s+program\b`)
	complAdditionalConditionsRe = regexp.MustCompile(`(?i)^\s*,?\s*and\s+(?:meeting|fulfilling)\s+the\s+following\s+(?:\w+\s+)?(?:conditions?|requirements?)\s*:?\s*$`)
	complAdditionalFloorRe      = regexp.MustCompile(`(?i)^\s*,\s*((?:with\s+at\s+least|including)\s+.+)$`)
	// complAlsoCountsRe matches courses that also count: "CSB450H1 is also
	// acceptable".
	complAlsoCountsRe = regexp.MustCompile(`(?i)^((?:` + codePat + `(?:\s*,\s*|\s*,?\s+or\s+|\s*/\s*)?)+)\s+(?:is|are)\s+also\s+acceptable$|^((?:` + codePat + `(?:\s*,\s*|\s*,?\s+or\s+|\s*/\s*)?)+)\s+may\s+also\s+be\s+used\s+to\s+fulfil+\s+this\s+requirement$`)
)

// additional reads "Additional <courses> to a total of N credits" into Req 1's
// pool, and an N other than the total into a constraint under Req 1. It reports
// false, reading nothing, if Req 1 has a pool or the wording is unknown.
func (a *compAssembler) additional(text, raw string) bool {
	if a.total == nil {
		return false
	}
	pool := a.total.r.Req
	sents := splitComplSentences(text)
	first := complAdditionalAndRe.ReplaceAllString(strings.TrimSuffix(a.clean(sents[0]), "."), "")
	m := complAdditionalTotalRe.FindStringSubmatch(first)
	if m == nil || pool.Type != types.NodeCredits || pool.Designators != nil || pool.Levels != nil || pool.Courses != nil {
		return false
	}
	phrase := strings.TrimSuffix(strings.TrimSpace(complAdditionalAmountRe.ReplaceAllString(m[1], "")), ",")
	var also []string
	if e := complAdditionalExamplesRe.FindStringSubmatchIndex(phrase); e != nil {
		also = codeRe.FindAllString(phrase[e[2]:e[3]], -1)
		phrase = phrase[:e[0]]
	}
	// "or Equivalent courses (listed below)": the named list joins the pool
	var listed []string
	if e := complAdditionalListRe.FindStringSubmatchIndex(phrase); e != nil {
		listed = a.namedListCodes(phrase[e[2]:e[3]])
		if listed == nil {
			return false
		}
		phrase = phrase[:e[0]] + " courses"
	}
	// a subject is its designator; "0.5 credit courses" are half courses only
	half := false
	if w := complAdditionalWeightRe.FindString(phrase); w != "" {
		half = strings.Fields(w)[0] == "0.5"
	}
	phrase = complAdditionalWeightRe.ReplaceAllString(phrase, " ")
	phrase = complSubjectWordCIRe.ReplaceAllStringFunc(phrase, func(w string) string { return subjectDesignators[strings.ToLower(w)] })
	n, _, cond := hdrReadBody(phrase)
	if cond || n.Designators == nil && n.Levels == nil || n.Courses != nil {
		return false
	}
	tail := m[4]
	var excl []string
	var floors, rest []string
	var aside string
	for strings.TrimSpace(tail) != "" {
		switch {
		case complAdditionalExcludedRe.MatchString(tail):
			g := complAdditionalExcludedRe.FindStringSubmatch(tail)
			excl = append(excl, codeRe.FindAllString(g[1], -1)...)
			tail = tail[len(g[0]):]
		case complAdditionalAlsoRe.MatchString(tail):
			g := complAdditionalAlsoRe.FindStringSubmatch(tail)
			also = append(also, codeRe.FindAllString(g[1], -1)...)
			aside = strings.Trim(g[0], " .()")
			tail = tail[len(g[0]):]
		case complAdditionalCountingRe.MatchString(tail):
			tail = tail[len(complAdditionalCountingRe.FindString(tail)):]
		case complAdditionalConditionsRe.MatchString(tail):
			// "and meeting the following two conditions:": the items after it
			// are floors on the pool
			a.poolConditions = true
			tail = ""
		case complAdditionalFloorRe.MatchString(tail):
			floors = append(floors, complAdditionalFloorRe.FindStringSubmatch(tail)[1])
			tail = ""
		case strings.Trim(tail, " .") == "":
			tail = ""
		default:
			return false
		}
	}
	for _, sent := range sents[1:] {
		c := strings.TrimSuffix(a.clean(sent), ".")
		if g := complAlsoCountsRe.FindStringSubmatch(c); g != nil {
			also = append(also, codeRe.FindAllString(g[1]+g[2], -1)...)
			continue
		}
		rest = append(rest, sent) // read as the program's rules below
	}
	// a listed course the pool's levels or exclusions drop can't join it: the
	// note is the total's alternative, for Review
	var outside []string
	also = slices.DeleteFunc(dedupe(also), func(c string) bool {
		if !levelMatches(n.Levels, c) || slices.Contains(excl, c) {
			outside = append(outside, c)
			return true
		}
		return false
	})
	total, _ := strconv.ParseFloat(m[2], 64)
	pool.Designators, pool.Levels = slices.Clone(n.Designators), slices.Clone(n.Levels)
	if half {
		pool.MaxCourseAmount = 0.5
	}
	pool.Exclude, pool.Courses = dedupe(excl), dedupe(append(also, listed...))
	a.total.r.Raw += "\n" + raw
	if outside != nil && aside != "" {
		a.total.r.Req = orNode(pool, textNode(types.TextOther, aside))
	}
	if total != pool.Amount {
		// a sub-total counts its own designators, else the pool's at every
		// level (the level words describe the additional courses), else the
		// pool as is
		sub := &types.ReqNode{Type: types.NodeCredits, Amount: total, Designators: desigRe.FindAllString(m[3], -1)}
		switch {
		case sub.Designators != nil:
		case pool.Designators != nil:
			sub.Designators, sub.Courses = slices.Clone(pool.Designators), slices.Clone(pool.Courses)
		default:
			sub.Levels, sub.Exclude, sub.Courses = slices.Clone(pool.Levels), slices.Clone(pool.Exclude), slices.Clone(pool.Courses)
		}
		a.program = append(a.program, &compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Req: sub, Raw: raw}})
	}
	for _, f := range floors {
		for _, r := range a.floorRows(f) {
			a.program = append(a.program, &compRow{r: r})
		}
	}
	for _, s := range rest {
		// a cap on the named list's courses
		if listed != nil && complAdditionalListCapRe.MatchString(a.clean(s)) {
			v, _ := strconv.ParseFloat(complAdditionalListCapRe.FindStringSubmatch(a.clean(s))[1], 64)
			a.program = append(a.program, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Raw: s,
				Req: &types.ReqNode{Type: types.NodeCredits, Courses: slices.Clone(listed), MaxAmount: maxAmount(v)}}})
			continue
		}
		a.notesTo(nil, a.sentence(s, nil, compCtx{}))
	}
	return true
}

var (
	// complAdditionalAndRe matches a conjunction with the next item: "; and".
	complAdditionalAndRe = regexp.MustCompile(`(?i)\s*;\s*and$`)
	// complAdditionalListRe matches a named list below: "or Equivalent courses
	// (listed below)".
	complAdditionalListRe = regexp.MustCompile(`(?i)\s+or\s+([A-Z][\w\s]*?)\s+courses\s+\(listed\s+below\)\s*$`)
	// complAdditionalWeightRe matches a course weight inside the phrase: "0.5
	// credit".
	complAdditionalWeightRe = regexp.MustCompile(`(?i)\s+\d+(?:\.\d+)?\s+credits?\s+`)
	// complAdditionalListCapRe matches a cap on the named list: "No more than
	// 1.0 credit from the Equivalent courses can contribute".
	complAdditionalListCapRe = regexp.MustCompile(`(?i)^no\s+more\s+than\s+(\d+(?:\.\d+)?)\s+credits?\s+from\s+the\s+[\w\s]+?\s+courses\s+can\s+contribute\b`)
)

// routeSentences joins to a route (an approval or a widened pool) the plain
// sentences after it that say what it lets count. A route pointing back at
// "these courses" joins the route before it, else keeps it as context.
func (a *compAssembler) routeSentences(sents []string) []string {
	var out []string
	route, anyRoute := false, false
	for _, s := range sents {
		isRoute := a.role(s) == roleRequirement && (a.isApproval(s) || a.isAlternative(s))
		plain := a.role(s) == roleRequirement && !isRoute
		followUp := anyRoute && isRoute && a.isApproval(s) && !codeRe.MatchString(s) && !complRefPhraseRe.MatchString(s) && !complReplacedRe.MatchString(s)
		if route && (plain && !complAllocLeadRe.MatchString(a.clean(s)) || isRoute && complRouteBackRe.MatchString(s)) || followUp {
			// one route: "in lieu of PSY201H1. Please contact us to verify ..."
			out[len(out)-1] = strings.TrimRight(out[len(out)-1], ".") + ". " + s
			continue
		}
		anyRoute = isRoute
		if isRoute && complRouteBackRe.MatchString(s) && len(out) > 0 && codeRe.MatchString(out[len(out)-1]) {
			a.routeContext[s] = out[len(out)-1]
		}
		route = a.role(s) == roleRequirement && (a.isApproval(s) || complWidenRe.MatchString(a.clean(s)) || complRouteBackRe.MatchString(s) && a.isAlternative(s))
		out = append(out, s)
	}
	return out
}

// complRouteBackRe matches a sentence pointing back at the courses before it:
// "one of these".
var complRouteBackRe = regexp.MustCompile(`(?i)\b(?:these|such)\s+(?:\w+\s+)?courses\b|\bone\s+of\s+these\b`)

// mostlyCodes reports whether s lists codes with little else: at least three
// codes, and a few words besides ("..., AFR498H1, an independent studies
// course approved by the Program Committee"), or only codes.
func mostlyCodes(s string) bool {
	if isCodeList(s) {
		return true
	}
	codes := complAnyCodeRe.FindAllString(s, -1)
	rest := strings.Fields(hdrWordRe.ReplaceAllStringFunc(complAnyCodeRe.ReplaceAllString(complTitleParenRe.ReplaceAllString(s, " "), " "), func(w string) string {
		if strings.EqualFold(w, "and") || strings.EqualFold(w, "or") {
			return " "
		}
		return w
	}))
	words := 0
	for _, w := range rest {
		if hdrWordRe.MatchString(w) {
			words++
		}
	}
	return len(codes) >= 3 && words <= 12 && !complAmountRe.MatchString(s)
}

// complAnyCodeRe matches a course code, undergraduate or graduate: "CSC2221H".
var complAnyCodeRe = regexp.MustCompile(codePat + `|\b[A-Z]{3}\d{4}[HY]\d?\b`)

// add makes row a child of owner, or an item of the program.
func (a *compAssembler) add(row, owner *compRow) {
	if owner != nil {
		owner.children = append(owner.children, row)
	} else {
		a.items = append(a.items, row)
	}
}

// children reads the blocks kids under a block with text lead: under an item
// (row), its floors, parts or sentences about it; under advice, advice.
func (a *compAssembler) children(kids []*compBlock, lead string, ctx compCtx, row *compRow) {
	if len(kids) == 0 {
		return
	}
	switch {
	case row == nil && a.allAdvice(lead):
		a.blocks(kids, compCtx{owner: ctx.owner, advice: true, notes: ctx.notes})
	case row == nil:
		a.blocks(kids, ctx)
	case row.r.Req != nil && complFloorsLeadRe.MatchString(lead):
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
func (a *compAssembler) about(b *compBlock, owner *compRow, list *compBlock) {
	text := b.text
	if list != nil {
		text += " " + list.text
	}
	if len(b.children) > 0 && a.allAdvice(text) {
		note, rest := a.adviceNote(text, b.children)
		owner.r.Notes = append(owner.r.Notes, note)
		for _, c := range rest {
			a.about(c, owner, nil)
		}
		return
	}
	if a.prerequisiteMembers(b, owner) {
		return
	}
	if k := b.children; b.kind != blockLabelled && b.kind != blockHeading && len(k) == 1 && len(k[0].children) == 0 && mostlyCodes(k[0].text) &&
		complSubListTitleRe.MatchString(strings.TrimSpace(b.text)) && !complAnyCodeRe.MatchString(b.text) && !complAmountRe.MatchString(b.text) {
		// a sub-list and its name, one note: "- Politics & Policy" then codes
		a.listDef(owner, cleanNote(b.text), k[0].text, k[0], cleanNote(b.text)+": "+cleanNote(blockLine(k[0])))
		return
	}
	notesBefore := len(owner.r.Notes)
	defer func() {
		// a labelled line kept as a note keeps its label, which routes refer to
		if b.kind == blockLabelled && !blkNoteLabelRe.MatchString(b.label) && len(owner.r.Notes) > notesBefore &&
			!strings.HasPrefix(owner.r.Notes[notesBefore], b.label) {
			owner.r.Notes[notesBefore] = b.label + ": " + owner.r.Notes[notesBefore]
		}
		// a line of codes kept as a note keeps the sub-heading naming its list
		if h := b.heading; b.kind != blockLabelled && h != nil && h.section != "" && h.section != owner.r.Section &&
			len(owner.r.Notes) > notesBefore && mostlyCodes(b.text) && !strings.HasPrefix(owner.r.Notes[notesBefore], h.section) {
			owner.r.Notes[notesBefore] = h.section + ": " + owner.r.Notes[notesBefore]
		}
		// A list's line kept as one note: a list an item may refer to.
		if len(owner.r.Notes) == notesBefore+1 && (b.kind == blockLabelled && !blkNoteLabelRe.MatchString(b.label) || mostlyCodes(b.text)) {
			title := b.label
			if b.kind != blockLabelled && b.heading != nil {
				title = b.heading.section
			}
			a.listDefs = append(a.listDefs, &compListDef{title: title, members: b.text, note: owner.r.Notes[notesBefore], block: b, row: owner})
		}
	}()
	for _, s := range splitComplSentences(text) {
		role := a.role(s)
		if a.poolAddition(owner, s) {
			continue
		}
		if m := complOrContinuationRe.FindStringSubmatch(strings.TrimSpace(a.clean(s))); m != nil && owner.r.Req != nil {
			// more alternatives for the item: "(or one of ECO220Y1, PSY201H1)"
			alts := []*types.ReqNode{owner.r.Req}
			for _, c := range codeRe.FindAllString(m[1], -1) {
				alts = append(alts, &types.ReqNode{Type: types.NodeCourse, Code: c})
			}
			owner.r.Req = orNode(alts...)
			owner.r.Raw += "\n" + strings.TrimSpace(s)
			continue
		}
		if r := capRow(s); r != nil && complOnlyOfRe.MatchString(a.clean(s)) {
			a.rowUnder(nil, r) // "only 0.5 of ECO220Y1 will be accepted towards the program"
			continue
		}
		switch {
		case role == roleRequirement && isRelaxation(s) && !(complOmitRe.MatchString(s) && a.omitCovered(s)):
			a.relaxations = append(a.relaxations, s)
		case role == roleRequirement && complThisRequirementRe.MatchString(s) && a.lastRule(owner) != nil:
			// "will also meet this requirement": a route for the rule before it
			k := a.lastRule(owner)
			k.r.Req = orNode(k.r.Req, textNode(altKind(s), s))
			addRaw(k, s)
		case role == roleRequirement && !a.isApproval(s) && a.isAlternative(s):
			a.alternative(owner, s)
		case role == roleRequirement && !a.isApproval(s):
			a.looseSentence(owner, s)
		default:
			owner.r.Notes = append(owner.r.Notes, a.sentence(s, nil, compCtx{owner: owner})...)
		}
	}
	for _, c := range b.children {
		a.about(c, owner, nil)
	}
}

// complSubListTitleRe matches a short name over a sub-list: "Politics &
// Policy".
var complSubListTitleRe = regexp.MustCompile(`^[A-Z][\w&,/'’ -]{0,60}$`)

// complPrerequisiteLabelRe matches a sub-list of more members for some
// students: "For those with the appropriate prerequisites".
var complPrerequisiteLabelRe = regexp.MustCompile(`(?i)^for\s+(?:those|students)\s+with\b`)

// prerequisiteMembers adds the courses of a complPrerequisiteLabelRe sub-list
// to owner's pool of listed courses (the label is the row's note), and
// reports whether it did.
func (a *compAssembler) prerequisiteMembers(b *compBlock, owner *compRow) bool {
	pool, caps := poolCaps(owner.r.Req)
	if b.kind != blockLabelled || !complPrerequisiteLabelRe.MatchString(b.label) || !isCodeList(b.text) || len(b.children) > 0 ||
		!isPool(pool) || pool.Courses == nil {
		return false
	}
	wide := cloneNode(pool)
	wide.Courses = dedupe(append(slices.Clone(pool.Courses), codeRe.FindAllString(b.text, -1)...))
	owner.r.Req = andNode(append([]*types.ReqNode{wide}, caps...)...)
	owner.r.Notes = append(owner.r.Notes, cleanNote(blockLine(b)))
	addRaw(owner, a.line(b))
	return true
}

// complPoolAdditionRe matches a line under a pool that adds to it: "Any course
// (0.5 credit) from Group I".
var complPoolAdditionRe = regexp.MustCompile(`(?i)^(?:any\s+(?:(?-i:[A-Z]{3})\s+)?courses?\b|up\s+to\b)[^;]*?\bfrom\b`)

// poolAddition reads a complPoolAdditionRe sentence about owner's pool and
// reports whether it did: a cap on courses the pool counts is a limit; other
// courses widen it as or(pool, text), so a student using them is Review.
func (a *compAssembler) poolAddition(owner *compRow, s string) bool {
	t := a.clean(s)
	if owner.r.Req == nil || !complPoolAdditionRe.MatchString(t) || complBindingWordRe.MatchString(t) && !complCapLeadRe.MatchString(t) {
		return false
	}
	r := capRow(s)
	if r != nil && r.Req.Courses != nil && !slices.ContainsFunc(r.Req.Courses, func(c string) bool { return !rowNames(owner, c) }) {
		a.rowUnder(owner, r)
		return true
	}
	if complCapLeadRe.MatchString(t) && r != nil && !codeRe.MatchString(t) {
		return false // "Up to 1.0 credit at the 400-level ...": a cap on the pool
	}
	a.alternative(owner, s)
	return true
}

// complTitleCodeAdviceRe matches a titled course and advice on it:
// "Professional Experience Course ACT390H1: students are strongly recommended".
// Groups 2 and 3 are the course and the advice.
var complTitleCodeAdviceRe = regexp.MustCompile(`^([^:]*?\b((?:` + codePat + `\s*/\s*)*` + codePat + `))\s*:\s*(.+)$`)

// complOrContinuationRe matches a line adding alternatives to the item above
// it: "(or one of ECO220Y1, PSY201H1)".
var complOrContinuationRe = regexp.MustCompile(`(?i)^\(?\s*or\s+(?:one\s+of\s+)?((?:` + codePat + `\s*(?:,|/|or)?\s*)+)\)?\.?$`)

// complOfTheseLeadRe matches a lead into floors on the total: "Of these:".
var complOfTheseLeadRe = regexp.MustCompile(`(?i)^of\s+these\s*:?$`)

// programFloors reads a line under "Of these:": its floors and caps are the
// total's, numbered with the floors stated with it; other sentences are read as
// usual.
func (a *compAssembler) programFloors(b *compBlock) {
	for _, s := range splitComplSentences(b.text) {
		switch a.role(s) {
		case roleRequirement, roleConstraint:
			if rows, _, ok := parseHeaderFloors(s); ok && len(rows) == 1 && rows[0].Kind == types.RequirementLimit {
				rows[0].Raw = s
				a.totalFloor(rows[0])
				continue
			}
			for _, r := range a.floorRows(s) {
				a.totalFloor(r)
			}
		case roleLimit:
			if r := capRow(s); r != nil {
				a.totalFloor(r) // "No more than 1.0 credit of 100-level courses may be used ..."
				continue
			}
			a.notesTo(nil, a.sentence(s, nil, compCtx{}))
		default:
			a.notesTo(nil, a.sentence(s, nil, compCtx{}))
		}
	}
	for _, c := range b.children {
		a.programFloors(c)
	}
}

// totalFloor adds a floor or cap on the total: with Req 1's own floors, or a
// program row when there is no total.
func (a *compAssembler) totalFloor(r *types.Requirement) {
	if a.total == nil {
		a.rowUnder(nil, r)
		return
	}
	a.floors = append(a.floors, &compRow{r: r})
}

// floorBlock reads a block under a lead into floors ("including:"): each
// requirement sentence is a constraint on row.
func (a *compAssembler) floorBlock(b *compBlock, row *compRow) {
	for _, s := range splitComplSentences(b.text) {
		switch a.role(s) {
		case roleRequirement, roleConstraint:
			a.constraint(row, s)
		default:
			a.sentence(s, row, compCtx{owner: row})
		}
	}
	for _, c := range b.children {
		a.floorBlock(c, row)
	}
}

// parts gives an item with no body its parts' requirement: a choose under a
// count lead, an or of alternatives, an and of parts that share no course;
// parts that can share stay child allocations, so no course counts for two.
func (a *compAssembler) parts(row *compRow, b *compBlock) {
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
	alternatives := complEitherLeadRe.MatchString(b.text) || slices.ContainsFunc(parts, func(k *compRow) bool { return complTrailingOrRe.MatchString(k.r.Raw) })
	count := 0
	if m := complCountLeadRe.FindStringSubmatch(b.text); m != nil {
		count = countWords[strings.ToLower(m[1])] // "Two of the following selections:"
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
		row.r.Req = orNode(nodes...)
	case !slices.ContainsFunc(nodes[1:], func(n *types.ReqNode) bool { return sharesCourses(andNode(nodes[:slices.Index(nodes, n)]...), n) }):
		row.r.Req = andNode(nodes...)
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
func (a *compAssembler) splitShared(row *compRow) {
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
