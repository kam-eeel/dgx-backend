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

// List lines: lines that set out the courses items draw on, kept as list
// definitions, and leads that read the lines under them as one item.

var (
	// codeLineLeadRe matches a lead over a line of codes: "2.5 credits
	// from the following list:".
	codeLineLeadRe = regexp.MustCompile(`(?i)\b(?:following|listed\s+below|below)\b(?:\s+list)?\s*:?$|:$`)
	// poolListedBelowRe matches a pool of the courses listed below: "4.0
	// credits chosen from those listed below".
	poolListedBelowRe = regexp.MustCompile(`(?i)\bfrom\s+(?:those|the\s+courses?)\s+listed\s+below\b`)
	// fromListBelowRe matches a lead over sub-lists: "4.5 credits chosen
	// from the list below".
	fromListBelowRe = regexp.MustCompile(`(?i)\bfrom\s+the\s+list\s+below\b`)
	// theFollowingRe matches the words that the line of codes after a lead
	// stands for: "the following list".
	theFollowingRe = regexp.MustCompile(`(?i)\bthe\s+following(?:\s+(?:list(?:\s+of\s+courses)?|courses))?\b`)
	// subListsLeadRe matches a lead over the sub-lists after it: "A minimum
	// of 3.5 credits from the following list".
	subListsLeadRe = regexp.MustCompile(`(?i)\bfrom\s+the\s+following\b|\bfollowing\s+list\b|\blisted\s+below\b`)
	// listsElsewhereRe matches a sign that items draw on lists set out
	// elsewhere: "2.5 credits from the groups listed below".
	listsElsewhereRe = regexp.MustCompile(`(?i)\bbelow\b|\bfollowing\s+(?:groups?|lists?|concentrations?|areas?|subject\s+areas?|clusters?)\b|\bconcentrations?\b|\b(?:language|subject)\s+areas?\b|\bgroups?\s+[A-Z1-9]\b`)

	// listSetHeadingRe matches a heading over the lists items draw on:
	// "Course Groups", "Language Areas".
	listSetHeadingRe = regexp.MustCompile(`(?i)\bcourses?\s*:?$|\bcourse\s+groups?\b|\bconcentrations?\b|\bgroups?\b|\bareas?\b|\bclusters?\b|\blists?\b|\bfoc(?:us|uses|i)\b|\bstreams?\b|\bfoundations?\b|\bcategor(?:y|ies)\b|\bthemes?\b|\bsubgroups?\b|\bnot\s+program\s+requirements\b`)
	// courseListHeadingRe matches a heading over one course list: "Group
	// A: Ancient, Medieval". Unlike listSetHeadingRe it takes electives, not
	// areas or themes.
	courseListHeadingRe = regexp.MustCompile(`(?i)\b(?:groups?|lists?|clusters?|sub-?groups?|categor(?:y|ies)|electives?|eligible|streams?)\b`)
	// requiredHeadingRe matches a heading over required courses: "Core
	// Courses".
	requiredHeadingRe = regexp.MustCompile(`(?i)\b(?:required|core|foundation(?:al|s)?|compulsory|program\s+requirements)\b`)
	// choiceListLabelRe matches a label naming courses to choose from:
	// "Approved Economics courses".
	choiceListLabelRe = regexp.MustCompile(`(?i)\b(?:approved|eligible|accepted|elective|optional)\b.*\bcourses?\b`)
	// memberLineRe matches a line that describes a list's members instead of
	// listing them: "Any other GGR courses not listed in Group A".
	memberLineRe = regexp.MustCompile(`^Any\s+(?:other\s+)?(?:` + grammar.DesigPat + `(?:\s*(?:,|or|and|/)\s*` + grammar.DesigPat + `)*\s+)?courses?\b`)
)

// listLine reports whether program-level line b is a list rather than a
// requirement: codes with no amount, under a heading naming lists or after
// items that draw on lists below, or only repeating required courses.
func (a *Assembler) listLine(b *Block) bool {
	// Members that count only with permission are the list's maybe item.
	text, _ := permCodes(strings.TrimSuffix(strings.TrimSpace(b.Text), "."))
	members := mostlyCodes(text) || a.numberedSeen && b.Heading != nil && memberLineRe.MatchString(strings.TrimSpace(text))
	if blkNumbered(b) || len(b.Children) > 0 || !members || amountRe.MatchString(text) || optionLabelRe.MatchString(b.Label) ||
		b.Kind == BlockHeading || BlkNoteLabelRe.MatchString(b.Label) || BlkIsYearPhrase(b.Label) || blkHasYearRe.MatchString(b.Label) {
		return false
	}
	listHead := false
	for h := b.Heading; h != nil; h = h.Heading {
		if h.Kind != BlockHeading || h.IsNotes() {
			continue
		}
		if BlkIsYearPhrase(h.Section) || blkHasYearRe.MatchString(h.Section) || blkAmountRe.MatchString(h.Text) {
			return false // "First Year:", "Compulsory Foundation Sciences (1.5 credits):"
		}
		if a.numberedSeen {
			// after the numbered items, a heading without an amount names a list
			listHead = true
			continue
		}
		if requiredHeadingRe.MatchString(h.Section) && !listSetHeadingRe.MatchString(h.Section) {
			return false // "Required Courses", "Program Requirements"
		}
		listHead = listHead || listSetHeadingRe.MatchString(h.Section) && !requiredHeadingRe.MatchString(h.Section)
	}
	switch {
	case listHead:
		return true
	case a.numberedSeen && listsElsewhereRe.MatchString(a.raw) && (b.Kind == blockLabelled || b.Heading != nil):
		return true
	}
	codes := grammar.CodeRe.FindAllString(b.Text, -1)
	seen := a.requiredSoFar()
	return codes != nil && !slices.ContainsFunc(codes, func(c string) bool { return !seen[c] })
}

// listNote is a list line as a note, with the heading it is under:
// "Concentrations: Literature: ITA200H1; ...".
func (a *Assembler) listNote(b *Block) string {
	note := grammar.CleanNote(BlockLine(b))
	if h := b.Heading; h != nil && h.Section != "" && h.Section != b.Label {
		note = h.Section + ": " + note
	}
	return note
}

// namedElsewhere reports whether labelled line b names a list that another
// line refers to. A line with the same label is another list, not a reference.
func (a *Assembler) namedElsewhere(b *Block) bool {
	l := strings.ToLower(strings.TrimSpace(b.Label))
	if BlkIsYearPhrase(b.Label) || blkHasYearRe.MatchString(b.Label) || requiredHeadingRe.MatchString(b.Label) || len(strings.Fields(l)) < 2 {
		return false // "Fourth Year: PCL470H1" beside "Third or Fourth Year: ..."
	}
	for h := b.Heading; h != nil; h = h.Heading {
		if h.Kind == BlockHeading && blkAmountRe.MatchString(h.Text) {
			return false // "Compulsory Foundation Geosciences (3.5 credits):"
		}
	}
	refs := 0
	for _, line := range a.lines {
		t := strings.ToLower(strings.TrimLeft(programs.StripEmphasis(strings.TrimSpace(line)), "-•* "))
		if strings.HasPrefix(t, l) && strings.HasPrefix(strings.TrimSpace(t[len(l):]), ":") {
			continue // a labelled line
		}
		refs += strings.Count(t, l)
	}
	return refs > 0
}

// isListTitle reports a line that names a list and has no course: "Group A:
// Theoretical and Methods 1: Theoretical" before its members.
func isListTitle(b *Block) bool {
	text := b.Text
	if b.Kind == blockLabelled {
		text = b.Label + ": " + text
	}
	return b.Kind != blockItem && !grammar.CodeRe.MatchString(text) && listLabelRe.MatchString(text)
}

// inListHeading reports whether b is an unnumbered line under a heading that
// names a course list (not a year).
func inListHeading(b *Block) bool {
	h := b.Heading
	return !blkNumbered(b) && h != nil && h.Kind == BlockHeading && !BlkIsYearPhrase(h.Section) &&
		(courseListHeadingRe.MatchString(h.Section) || listLabelRe.MatchString(h.Section))
}

// mayBeList reports an unnumbered, unlabelled line of six or more codes and
// no amount, not under a year or required-courses heading.
func mayBeList(b *Block) bool {
	if blkNumbered(b) || b.Label != "" || len(b.Children) > 0 || !mostlyCodes(b.Text) {
		return false
	}
	if BlkIsYearPhrase(b.Section) || requiredHeadingRe.MatchString(b.Section) || b.Heading != nil && b.Heading.IsNotes() {
		return false
	}
	return len(grammar.CodeRe.FindAllString(b.Text, -1)) >= 6
}

// subLine reports whether b is a sub-list under a lead: codes with no amount
// of its own. Lines under it are rules on the item, not more lists.
func subLine(b *Block) bool {
	return b.Kind != BlockHeading && !BlkNoteLabelRe.MatchString(b.Label) && mostlyCodes(b.Text) && !amountRe.MatchString(b.Text) &&
		!slices.ContainsFunc(b.Children, func(c *Block) bool { return mostlyCodes(c.Text) || len(c.Children) > 0 })
}

// listZone returns how many blocks after bs[0] are sub-lists that bs[0] leads
// into (two or more), or 0.
func (a *Assembler) listZone(bs []*Block) int {
	b := bs[0]
	if b.Kind == BlockHeading || len(b.Children) > 0 || !amountRe.MatchString(b.Text) ||
		!subListsLeadRe.MatchString(b.Text) && !strings.HasSuffix(strings.TrimSpace(b.Text), ":") {
		return 0
	}
	n := 0
	for _, c := range bs[1:] {
		if !subLine(c) || blkNumbered(c) && c.Kind == blockItem && !blkLetteredRe.MatchString(c.marker) {
			break
		}
		n++
	}
	if n < 2 {
		return 0
	}
	return n
}

// blkLetteredRe matches a lettered marker: "a.", "(b)".
var blkLetteredRe = regexp.MustCompile(`^\(?[a-z]\)?[.)]?$`)

// leadingSubLines returns how many of bs, from the first, are sub-lists.
func leadingSubLines(bs []*Block) int {
	n := 0
	for n < len(bs) && subLine(bs[n]) {
		n++
	}
	return n
}

var (
	// oneOfLeadRe matches a lead whose sub-lists are alternatives: "1.0
	// credit from one of the following languages:".
	oneOfLeadRe = regexp.MustCompile(`(?i)\bone\s+(?:of\s+)?the\s+following\b`)
)

// listLead reads a lead and its sub-lists as one item: their or under "one of
// the following", the pool of their courses under a bare amount, all of them
// when they add up to the amount, and otherwise text with the sub-lists as
// list definitions.
func (a *Assembler) listLead(b *Block, subs []*Block, ctx compCtx) (row *compRow) {
	defer func() {
		// The lines under a sub-list are rules on the item, or on its part.
		for i, c := range subs {
			owner := row
			if row.r.Req == nil && i < len(row.children) {
				owner = row.children[i]
			}
			if len(c.Children) > 0 {
				a.blocks(c.Children, compCtx{owner: owner, advice: ctx.advice, notes: ctx.notes})
			}
		}
	}()
	raw := a.line(b)
	defer func() {
		// a rule line after the lead or its last sub-list is about this item
		a.leadRows[b] = row
		if len(subs) > 0 {
			a.leadRows[subs[len(subs)-1]] = row
		}
	}()
	var leadNotes []string
	var codes, texts []string
	for _, c := range subs {
		codes = append(codes, anyCodeRe.FindAllString(c.Text, -1)...)
		raw += "\n" + a.line(c)
		// keep the sub-list as written for its "/" pairs; a long bracketed remark
		// is the item's note
		t := titleParenRe.ReplaceAllStringFunc(c.Text, func(p string) string {
			if anyCodeRe.MatchString(p) {
				return p
			}
			if inner := strings.TrimSpace(p[1 : len(p)-1]); len(strings.Fields(inner)) >= 3 {
				leadNotes = append(leadNotes, grammar.CleanNote(inner))
			}
			return ""
		})
		// the sub-list's name says nothing about its courses
		if m := subListNameRe.FindStringSubmatch(t); m != nil && !anyCodeRe.MatchString(m[1]) {
			t = t[len(m[0]):]
		}
		texts = append(texts, strings.TrimRight(t, ". "))
	}
	row = &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Label: itemLabel(b), Section: b.Section, Raw: raw}}
	lead := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(b.Text), ":"))
	// bracketed advice in the lead is the item's note, not a rule on its parts
	lead = strings.TrimSpace(titleParenRe.ReplaceAllStringFunc(lead, func(p string) string {
		if inner := strings.TrimSpace(p[1 : len(p)-1]); len(strings.Fields(inner)) >= 3 && !grammar.CodeRe.MatchString(inner) && a.role(inner) == RoleAdvisory {
			leadNotes = append(leadNotes, grammar.CleanNote(inner))
			return ""
		}
		return p
	}))
	defer func() { row.r.Notes = append(leadNotes, row.r.Notes...) }()
	amount := 0.0
	if m := amountRe.FindString(lead); m != "" {
		amount, _ = strconv.ParseFloat(strings.Fields(m)[0], 64)
	}
	switch {
	case oneOfLeadRe.MatchString(lead) && !slices.ContainsFunc(subs, func(c *Block) bool { return !subLine(c) }):
		var opts []*types.ReqNode
		for _, c := range subs {
			n, _ := parseCompletionText(c.Text)
			n = checkedBody(n, c.Text)
			if v, ok := grammar.MinCredits(n); n == nil || !ok || hasText(n) || math.Abs(v-amount) > 1e-9 {
				opts = nil
				break
			}
			opts = append(opts, n)
		}
		if opts != nil {
			row.r.Req = programs.OrNode(opts...)
		}
	case !slices.ContainsFunc(subs, func(c *Block) bool { return !subLine(c) }):
		// a bare amount: the pool of all the sub-lists' courses, with the lead's
		// other clauses as rules on it
		head, rest := poolLeadHead(lead)
		if head == "" {
			break
		}
		n, _ := parseCompletionText(head + ": " + strings.Join(texts, ", "))
		if pool, _ := poolCaps(n); !isPool(pool) || slices.ContainsFunc(grammar.Dedupe(codes), func(c string) bool { return !slices.Contains(pool.Courses, c) }) {
			n, _ = parseCompletionText(head + ": " + strings.Join(grammar.Dedupe(codes), ", ")) // the codes alone
		}
		if isPool(n) || isPool(firstOf(poolCaps(n))) {
			row.r.Req = n
			a.add(row, ctx.owner)
			for _, r := range rest {
				row.r.Notes = append(row.r.Notes, a.sentence(r, row, compCtx{owner: ctx.owner})...)
			}
			splitCaps(row)
			return row
		}
	}
	if row.r.Req == nil && !oneOfLeadRe.MatchString(lead) {
		// sub-lists that each read as courses and add up to the amount are all
		// required, each a part of the item
		var parts []*compRow
		sum := 0.0
		for _, c := range subs {
			n, _ := parseCompletionText(c.Text)
			n = checkedBody(n, c.Text)
			v, ok := grammar.MinCredits(n)
			if n == nil || !ok || hasText(n) {
				parts = nil
				break
			}
			sum += v
			parts = append(parts, &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Label: itemLabel(c), Req: n, Raw: a.line(c)}})
		}
		if parts != nil && math.Abs(sum-amount) < 1e-9 {
			row.r.Raw = a.line(b)
			row.children = parts
			a.add(row, ctx.owner)
			return row
		}
	}
	if row.r.Req == nil {
		row.r.Req = TextNode(types.TextOther, lead)
		for _, c := range subs {
			// a list's name may come before its courses: "Group 1 – Literary History: ..."
			title, members := c.Label, c.Text
			if m := subListNameRe.FindStringSubmatch(c.Text); title == "" && m != nil {
				if _, key := listName(m[1]); key != "" {
					title, members = m[1], c.Text[len(m[0]):]
				}
			}
			a.listDef(row, title, members, c, grammar.CleanNote(BlockLine(c)))
		}
	}
	a.add(row, ctx.owner)
	splitCaps(row)
	return row
}

// subListNameRe matches the name before a sub-list's courses:
// "Hydrology/GIS/Remote Sensing: ".
var subListNameRe = regexp.MustCompile(`^([^:]{1,80}):\s+`)

// poolLeadClauseRe matches a lead that is only an amount over the lists
// after it: "2.0 credits from the following".
var poolLeadClauseRe = regexp.MustCompile(`(?i)^(?:any\s+\d+\s+(?:half\s+)?courses\s+\(\s*)?` + grammar.AmountPat + `\s+credits?\)?\s+(?:(?:chosen|selected|taken)\s+)?(?:in\s+[A-Z][\w ,]*?(?:,?\s+or\s+[A-Z]\w*)\s+)?(?:from|of)(?:\s+(?:the\s+following(?:\s+(?:(?:two|three|four|five|six|\d)\s+)?(?:list|courses|groups?|categories))?|one\s+or\s+more\s+of\s+the\s+groups\s+below|the\s+groups\s+below|the\s+list\s+below))?\s*:?$`)

// poolLeadHead splits a lead into its pool clause ("4.0 credits from") and the
// rules after it. head is empty when lead isn't a pool lead.
func poolLeadHead(lead string) (head string, rest []string) {
	sents := SplitSentences(lead)
	if len(sents) == 0 {
		return "", nil
	}
	clauses := grammar.SplitTop(strings.TrimSuffix(strings.TrimSpace(sents[0]), "."), ';')
	first := strings.TrimSpace(clauses[0])
	m := poolLeadClauseRe.FindStringSubmatch(strings.TrimSuffix(first, ":"))
	if m == nil {
		return "", nil
	}
	for _, c := range clauses[1:] {
		rest = append(rest, strings.TrimSpace(c))
	}
	return m[1] + " credits from", append(rest, sents[1:]...)
}

// firstOf returns its first argument.
func firstOf(n *types.ReqNode, _ []*types.ReqNode) *types.ReqNode { return n }

var (
	// numberedListsRe matches a lead over two numbered lists: "2.0 credits
	// to be selected from lists 1 and 2".
	numberedListsRe = regexp.MustCompile(`(?i)^(?:plus\s+)?` + grammar.AmountPat + `\s+credits?\s+(?:to\s+be\s+selected\s+|selected\s+|chosen\s+)?from\s+(?:[\w-]+\s+)?lists\s+\(?(\d)\)?\s+and\s+\(?(\d)\)?`)
	// listNumRuleRe matches a floor or cap on one of them: "Up to 1.0
	// credit may be taken from list 2".
	listNumRuleRe = regexp.MustCompile(`(?i)\b(` + grammar.AtLeastPat + `|up\s+to|(?:at\s+)?a\s+maximum(?:\s+of)?|` + grammar.NoMoreThanPat + `)\s+` + grammar.AmountPat + `\s+credits?\b[^.;:]*?\blist\s+\(?(\d)\)?`)
	// listNumDefRe matches a list's members: "(1) ACT350H1, ...".
	listNumDefRe = regexp.MustCompile(`(?i)^\((\d)\)\s*(.*)$|\blist\s+\(?(\d)\)?\s*:\s*(.*)$`)
)

// numberedLists reads a lead over two numbered lists, and the lines defining
// them, as one allocation from their union with a row for each floor and cap.
// It returns how many blocks it read, or 0.
func (a *Assembler) numberedLists(bs []*Block, ctx compCtx) int {
	b := bs[0]
	m := numberedListsRe.FindStringSubmatch(strings.TrimSpace(b.Text))
	if m == nil || b.Kind == BlockHeading {
		return 0
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	lists := map[string][]string{}
	var listTexts []string // the lists as written, for their "/" pairs
	type rule struct {
		floor  bool
		amount float64
		list   string
		raw    string
	}
	var rules []rule
	raw := []string{a.line(b)}
	read := func(text string) {
		for _, r := range listNumRuleRe.FindAllStringSubmatch(text, -1) {
			v, _ := strconv.ParseFloat(r[2], 64)
			rules = append(rules, rule{floor: strings.EqualFold(r[1], "at least"), amount: v, list: r[3], raw: text})
		}
	}
	read(b.Text)
	var lines []*Block
	lines = append(lines, b.Children...)
	n := 1
	for _, c := range bs[1:] {
		if c.Kind == BlockHeading || !listNumDefRe.MatchString(c.marker+" "+programs.StripEmphasis(BlockLine(c))) && !listNumRuleRe.MatchString(c.Text) {
			break
		}
		lines = append(lines, c)
		n++
	}
	last := ""
	var walk func(cs []*Block)
	walk = func(cs []*Block) {
		for _, c := range cs {
			text := strings.TrimSpace(c.Text)
			if c.Kind == blockItem && strings.HasPrefix(c.marker, "(") {
				text = c.marker + " " + text
			}
			raw = append(raw, a.line(c))
			read(text)
			if d := listNumDefRe.FindStringSubmatch(programs.StripEmphasis(text)); d != nil {
				num, codes := d[1]+d[3], d[2]+d[4]
				last = num
				lists[num] = append(lists[num], grammar.CodeRe.FindAllString(codes, -1)...)
				listTexts = append(listTexts, codes)
			} else if last != "" && mostlyCodes(text) {
				lists[last] = append(lists[last], grammar.CodeRe.FindAllString(text, -1)...) // the codes on the line after "list 2:"
				listTexts = append(listTexts, text)
			}
			walk(c.Children)
		}
	}
	walk(lines)
	if len(lists[m[2]]) == 0 || len(lists[m[3]]) == 0 {
		return 0
	}
	all := grammar.Dedupe(append(slices.Clone(lists[m[2]]), lists[m[3]]...))
	row := &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Label: itemLabel(b), Section: b.Section, Raw: strings.Join(raw, "\n"),
		Req: &types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: all}}}
	// a "/" pair in a list is capped as in any pool
	for i, t := range listTexts {
		t = titleParenRe.ReplaceAllStringFunc(t, func(p string) string {
			if grammar.CodeRe.MatchString(p) {
				return p
			}
			return ""
		})
		listTexts[i] = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(t), "."), ", and")
	}
	if n, _ := parseCompletionText(fmtAmount(amount) + " credits from: " + strings.Join(listTexts, ", ")); n != nil {
		if pool, caps := poolCaps(n); isPool(pool) {
			for _, c := range caps {
				a.rowUnder(row, &types.Requirement{Kind: types.RequirementLimit, Raw: row.r.Raw, Req: c})
			}
		}
	}
	for _, r := range rules {
		codes := grammar.Dedupe(lists[r.list])
		if codes == nil {
			continue
		}
		if r.floor {
			a.rowUnder(row, &types.Requirement{Kind: types.RequirementConstraint, Raw: r.raw, Req: &types.ReqNode{Type: types.NodeCredits, Amount: r.amount, Courses: codes}})
		} else {
			a.rowUnder(row, &types.Requirement{Kind: types.RequirementLimit, Raw: r.raw, Req: &types.ReqNode{Type: types.NodeCredits, Courses: codes, MaxAmount: grammar.MaxAmount(r.amount)}})
		}
	}
	a.add(row, ctx.owner)
	return n
}

// namedListCodes returns the codes under the heading "<name> Courses", or nil.
func (a *Assembler) namedListCodes(name string) []string {
	want := strings.ToLower(strings.TrimSpace(name)) + " courses"
	for k := range a.lines {
		if strings.ToLower(strings.Trim(a.widenLine(k), " :*_")) != want {
			continue
		}
		var codes []string
		for k++; k < len(a.lines) && (IsCodeList(a.widenLine(k)) || mostlyCodes(a.widenLine(k))); k++ {
			codes = append(codes, grammar.CodeRe.FindAllString(a.lines[k], -1)...)
		}
		return grammar.Dedupe(codes)
	}
	return nil
}

// itemListRe matches a sentence listing the courses of the requirement
// before it: "The following courses fulfill the Diversity and Identity
// requirement: ...".
var itemListRe = regexp.MustCompile(`(?i)^the\s+following\s+courses\s+(?:fulfil+|satisfy|meet|count\s+toward)\s+the\s+(.+?)\s+requirement\s*:\s*(.+)$`)

// itemList reads a itemListRe sentence as the pool of the item before it,
// whose body is text naming the requirement. A list that isn't codes alone
// joins the item's text. It reports whether s was the item's.
func (a *Assembler) itemList(row *compRow, s string) bool {
	m := itemListRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(a.clean(s)), "."))
	n := row.r.Req
	if m == nil || n == nil {
		return false
	}
	text := n
	if n.Type == types.NodeOr && len(n.Items) > 0 {
		text = n.Items[0] // "or(text, a route)"
	}
	if text.Type != types.NodeText || !strings.Contains(strings.ToLower(text.Text), strings.ToLower(m[1])) {
		return false
	}
	amount := 0.0
	if am := leadAmountRe.FindStringSubmatch(text.Text); am != nil {
		amount, _ = strconv.ParseFloat(am[1], 64)
	}
	pool, _ := parseCompletionText(fmtAmount(amount) + " credit from: " + m[2])
	if pool, _ = poolCaps(pool); amount > 0 && isPool(pool) && pool.Designators == nil {
		*text = *pool
	} else {
		text.Text += ". " + grammar.CleanNote(s)
	}
	return true
}

var (
	// upToFromListRe matches a list's capped share closing a pool: ", up to
	// one half course from Group H".
	upToFromListRe = regexp.MustCompile(`(?i),\s+(up\s+to\s+[^,;]*?\bfrom\s+(?:group|list)\s+\w+)\s*\.?$`)
	// listMemberTextRe matches text naming more of a pool's members:
	// "400-level courses in Group I".
	listMemberTextRe = regexp.MustCompile(`(?i)^(?:and\s+)?(?:any\s+)?(?:` + grammar.LevelPat + `(?:[-/+\s]*` + grammar.LevelPat + `)*[-+]?\s*-?\s*level\s+)?(?:` + grammar.DesigPat + `\s+)?courses?\s+(?:in|from)\s+(?:group|list)\s+\w+$`)
)

// listMemberAlt turns and(pool, text naming more of its members) into
// or(pool, text). Read as and, a student using one of those members would get
// NotMet.
func listMemberAlt(n *types.ReqNode) *types.ReqNode {
	if n == nil || n.Type != types.NodeAnd || len(n.Items) < 2 || !isPool(n.Items[0]) {
		return n
	}
	var text *types.ReqNode
	var caps []*types.ReqNode
	for _, it := range n.Items[1:] {
		switch {
		case isCourseCap(it):
			caps = append(caps, it) // a pair flattened into the pool
		case text == nil && it.Type == types.NodeText && listMemberTextRe.MatchString(strings.TrimSpace(it.Text)):
			text = it
		default:
			return n
		}
	}
	if text == nil {
		return n
	}
	return programs.AndNode(append([]*types.ReqNode{programs.OrNode(n.Items[0], text)}, caps...)...)
}

var (
	// codeTitleLineRe matches a course and its title: "SDS255H1:
	// Histories of Sexuality".
	codeTitleLineRe = regexp.MustCompile(`^(` + grammar.CodePat + `)\s*:\s*[A-Za-z][^\d;/]*$`)
	// areaLineRe matches an area and its courses: "Statistics: STA302H1,
	// STA313H1;". The groups are the area and the codes.
	areaLineRe = regexp.MustCompile(`^([A-Z][A-Za-z&' ]{1,60}):\s*(` + grammar.CodePat + `(?:\s*,\s*` + grammar.CodePat + `)*)\s*[;.]?$`)
	// areaOrLineRe matches an area line with a rule of its own: "Astronomy:
	// AST430H1, or AST425Y1 (0.5 credit from point 1 above).".
	areaOrLineRe = regexp.MustCompile(`^[A-Z][A-Za-z&' ]{1,60}:\s*(` + grammar.CodePat + `),?\s+or\s+` + grammar.CodePat + `\b.*$`)
	// namedCoursesLeadRe matches a lead naming the courses under it: "1.0
	// credit from the SDS Foundational Courses:".
	namedCoursesLeadRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\s+from\s+the\s+(?-i:[A-Z])[\w&' -]*?\s+courses\s*:$`)
	// coursesSumLeadRe matches a lead naming courses and their total: "SDS
	// Foundational Courses (1.5 credits):".
	coursesSumLeadRe = regexp.MustCompile(`(?i)^(?-i:[A-Z])[\w&' -]*?\s+courses\s+\(` + grammar.AmountPat + `\s+credits?\)\s*:$`)
	// followingListLeadRe matches a lead over one line of courses: "(a
	// minimum of one 0.5 credit from the following list):".
	followingListLeadRe = regexp.MustCompile(`(?i)\((?:a\s+minimum\s+of\s+)?(?:one\s+)?` + grammar.AmountPat + `\s+credits?\s+from\s+the\s+following\s+list\)\s*:$`)
	// allCoursesLineRe matches that line: "All 400-level SDS and JNS
	// courses.".
	allCoursesLineRe = regexp.MustCompile(`^All\s+(.+?\s+courses)\.?$`)
)

// leadLines reads an item whose lead names the courses on the lines under it,
// when they have too few codes for listLead: a pool of them, all of them, an
// or of a pool per area, or one line's pool. row is the item's allocation, not
// yet added. It reports whether it read the item.
func (a *Assembler) leadLines(b *Block, row *compRow, ctx compCtx) bool {
	lead := strings.TrimSpace(b.Text)
	kids := b.Children
	if len(kids) == 0 || !strings.HasSuffix(lead, ":") {
		return false
	}
	plain := func(c *Block) bool {
		return c.Kind != BlockHeading && len(c.Children) == 0 && !amountRe.MatchString(c.Text)
	}
	// text is a line as written, with its label: "Statistics: STA302H1, ..."
	text := func(c *Block) string {
		if c.Kind == blockLabelled && c.Label != "" {
			return c.Label + ": " + strings.TrimSpace(c.Text)
		}
		return strings.TrimSpace(c.Text)
	}
	run := func(re *regexp.Regexp) int {
		n := 0
		for n < len(kids) && plain(kids[n]) && re.MatchString(text(kids[n])) {
			n++
		}
		return n
	}
	var body *types.ReqNode
	n := 0
	switch {
	case namedCoursesLeadRe.MatchString(lead):
		amount, _ := strconv.ParseFloat(namedCoursesLeadRe.FindStringSubmatch(lead)[1], 64)
		if n = run(codeTitleLineRe); n < 2 || n < len(kids) && codeTitleLineRe.MatchString(text(kids[n])) {
			return false
		}
		var codes []string
		for _, c := range kids[:n] {
			codes = append(codes, codeTitleLineRe.FindStringSubmatch(text(c))[1])
		}
		if weightOf(codes) < amount-1e-9 {
			return false
		}
		body = &types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: grammar.Dedupe(codes)}
	case coursesSumLeadRe.MatchString(lead):
		amount, _ := strconv.ParseFloat(coursesSumLeadRe.FindStringSubmatch(lead)[1], 64)
		if n = run(codeTitleLineRe); n < 2 || n < len(kids) && codeTitleLineRe.MatchString(text(kids[n])) {
			return false
		}
		var codes []string
		for _, c := range kids[:n] {
			codes = append(codes, codeTitleLineRe.FindStringSubmatch(text(c))[1])
		}
		if math.Abs(weightOf(codes)-amount) > 1e-9 {
			return false // all of them, so they must add up to the amount
		}
		body = programs.AndNode(programs.CourseNodes(grammar.Dedupe(codes))...)
	case oneOfLeadRe.MatchString(lead) && amountRe.MatchString(lead):
		amount, err := strconv.ParseFloat(strings.Fields(amountRe.FindString(lead))[0], 64)
		if n = run(areaLineRe); err != nil || n < 2 {
			return false
		}
		var opts []*types.ReqNode
		for _, c := range kids[:n] {
			codes := grammar.CodeRe.FindAllString(areaLineRe.FindStringSubmatch(text(c))[2], -1)
			switch w := weightOf(codes); {
			case len(codes) == 1 && math.Abs(w-amount) < 1e-9:
				opts = append(opts, &types.ReqNode{Type: types.NodeCourse, Code: codes[0]})
			case w >= amount-1e-9:
				opts = append(opts, &types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: grammar.Dedupe(codes)})
			default:
				return false // an area that can't make the amount
			}
		}
		if c := kids[min(n, len(kids)-1)]; n < len(kids) && c.Kind != BlockHeading && len(c.Children) == 0 {
			// an area with a rule of its own: its first course, and the line
			if m := areaOrLineRe.FindStringSubmatch(text(c)); m != nil {
				if math.Abs(grammar.CourseWeight(m[1])-amount) < 1e-9 {
					opts = append(opts, &types.ReqNode{Type: types.NodeCourse, Code: m[1]})
				}
				opts = append(opts, TextNode(types.TextOther, grammar.CleanNote(text(c))))
				n++
			}
		}
		body = programs.OrNode(opts...)
	case followingListLeadRe.MatchString(lead) && len(kids) >= 1 && plain(kids[0]) && allCoursesLineRe.MatchString(text(kids[0])):
		m := followingListLeadRe.FindStringSubmatch(lead)
		p, _ := parseCompletionText(m[1] + " credit from " + allCoursesLineRe.FindStringSubmatch(text(kids[0]))[1])
		if !isPool(p) || p.Courses != nil || p.Designators == nil || hasText(p) {
			return false
		}
		body, n = p, 1
	default:
		return false
	}
	row.r.Req = body
	for _, c := range kids[:n] {
		row.r.Raw += "\n" + a.line(c)
	}
	a.add(row, ctx.owner)
	a.leadRows[b] = row
	a.leadRows[kids[n-1]] = row
	a.children(kids[n:], b.Text, ctx, row)
	return true
}
