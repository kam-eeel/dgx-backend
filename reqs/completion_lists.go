package reqs

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// List lines: lines that set out the courses items draw on, kept as list
// definitions, and leads that read the lines under them as one item.

var (
	// complCodeLineLeadRe matches a lead over a line of codes: "2.5 credits
	// from the following list:".
	complCodeLineLeadRe = regexp.MustCompile(`(?i)\b(?:following|listed\s+below|below)\b(?:\s+list)?\s*:?$|:$`)
	// complPoolListedBelowRe matches a pool of the courses listed below: "4.0
	// credits chosen from those listed below".
	complPoolListedBelowRe = regexp.MustCompile(`(?i)\bfrom\s+(?:those|the\s+courses?)\s+listed\s+below\b`)
	// complFromListBelowRe matches a lead over sub-lists: "4.5 credits chosen
	// from the list below".
	complFromListBelowRe = regexp.MustCompile(`(?i)\bfrom\s+the\s+list\s+below\b`)
	// complTheFollowingRe matches the words that the line of codes after a lead
	// stands for: "the following list".
	complTheFollowingRe = regexp.MustCompile(`(?i)\bthe\s+following(?:\s+(?:list(?:\s+of\s+courses)?|courses))?\b`)
	// complSubListsLeadRe matches a lead over the sub-lists after it: "A minimum
	// of 3.5 credits from the following list".
	complSubListsLeadRe = regexp.MustCompile(`(?i)\bfrom\s+the\s+following\b|\bfollowing\s+list\b|\blisted\s+below\b`)
	// complListsElsewhereRe matches a sign that items draw on lists set out
	// elsewhere: "2.5 credits from the groups listed below".
	complListsElsewhereRe = regexp.MustCompile(`(?i)\bbelow\b|\bfollowing\s+(?:groups?|lists?|concentrations?|areas?|subject\s+areas?|clusters?)\b|\bconcentrations?\b|\b(?:language|subject)\s+areas?\b|\bgroups?\s+[A-Z1-9]\b`)

	// complListSetHeadingRe matches a heading over the lists items draw on:
	// "Course Groups", "Language Areas".
	complListSetHeadingRe = regexp.MustCompile(`(?i)\bcourses?\s*:?$|\bcourse\s+groups?\b|\bconcentrations?\b|\bgroups?\b|\bareas?\b|\bclusters?\b|\blists?\b|\bfoc(?:us|uses|i)\b|\bstreams?\b|\bfoundations?\b|\bcategor(?:y|ies)\b|\bthemes?\b|\bsubgroups?\b|\bnot\s+program\s+requirements\b`)
	// complCourseListHeadingRe matches a heading over one course list: "Group
	// A: Ancient, Medieval". Unlike complListSetHeadingRe it takes electives, not
	// areas or themes.
	complCourseListHeadingRe = regexp.MustCompile(`(?i)\b(?:groups?|lists?|clusters?|sub-?groups?|categor(?:y|ies)|electives?|eligible|streams?)\b`)
	// complRequiredHeadingRe matches a heading over required courses: "Core
	// Courses".
	complRequiredHeadingRe = regexp.MustCompile(`(?i)\b(?:required|core|foundation(?:al|s)?|compulsory|program\s+requirements)\b`)
	// complChoiceListLabelRe matches a label naming courses to choose from:
	// "Approved Economics courses".
	complChoiceListLabelRe = regexp.MustCompile(`(?i)\b(?:approved|eligible|accepted|elective|optional)\b.*\bcourses?\b`)
	// complMemberLineRe matches a line that describes a list's members instead of
	// listing them: "Any other GGR courses not listed in Group A".
	complMemberLineRe = regexp.MustCompile(`^Any\s+(?:other\s+)?(?:` + desigPat + `(?:\s*(?:,|or|and|/)\s*` + desigPat + `)*\s+)?courses?\b`)
)

// listLine reports whether program-level line b is a list rather than a
// requirement: codes with no amount, under a heading naming lists or after
// items that draw on lists below, or only repeating required courses.
func (a *compAssembler) listLine(b *compBlock) bool {
	// Members that count only with permission are the list's maybe item.
	text, _ := permCodes(strings.TrimSuffix(strings.TrimSpace(b.text), "."))
	members := mostlyCodes(text) || a.numberedSeen && b.heading != nil && complMemberLineRe.MatchString(strings.TrimSpace(text))
	if blkNumbered(b) || len(b.children) > 0 || !members || complAmountRe.MatchString(text) || complOptionLabelRe.MatchString(b.label) ||
		b.kind == blockHeading || blkNoteLabelRe.MatchString(b.label) || blkIsYearPhrase(b.label) || blkHasYearRe.MatchString(b.label) {
		return false
	}
	listHead := false
	for h := b.heading; h != nil; h = h.heading {
		if h.kind != blockHeading || h.isNotes() {
			continue
		}
		if blkIsYearPhrase(h.section) || blkHasYearRe.MatchString(h.section) || blkAmountRe.MatchString(h.text) {
			return false // "First Year:", "Compulsory Foundation Sciences (1.5 credits):"
		}
		if a.numberedSeen {
			// after the numbered items, a heading without an amount names a list
			listHead = true
			continue
		}
		if complRequiredHeadingRe.MatchString(h.section) && !complListSetHeadingRe.MatchString(h.section) {
			return false // "Required Courses", "Program Requirements"
		}
		listHead = listHead || complListSetHeadingRe.MatchString(h.section) && !complRequiredHeadingRe.MatchString(h.section)
	}
	switch {
	case listHead:
		return true
	case a.numberedSeen && complListsElsewhereRe.MatchString(a.raw) && (b.kind == blockLabelled || b.heading != nil):
		return true
	}
	codes := codeRe.FindAllString(b.text, -1)
	seen := a.requiredSoFar()
	return codes != nil && !slices.ContainsFunc(codes, func(c string) bool { return !seen[c] })
}

// listNote is a list line as a note, with the heading it is under:
// "Concentrations: Literature: ITA200H1; ...".
func (a *compAssembler) listNote(b *compBlock) string {
	note := cleanNote(blockLine(b))
	if h := b.heading; h != nil && h.section != "" && h.section != b.label {
		note = h.section + ": " + note
	}
	return note
}

// namedElsewhere reports whether labelled line b names a list that another
// line refers to. A line with the same label is another list, not a reference.
func (a *compAssembler) namedElsewhere(b *compBlock) bool {
	l := strings.ToLower(strings.TrimSpace(b.label))
	if blkIsYearPhrase(b.label) || blkHasYearRe.MatchString(b.label) || complRequiredHeadingRe.MatchString(b.label) || len(strings.Fields(l)) < 2 {
		return false // "Fourth Year: PCL470H1" beside "Third or Fourth Year: ..."
	}
	for h := b.heading; h != nil; h = h.heading {
		if h.kind == blockHeading && blkAmountRe.MatchString(h.text) {
			return false // "Compulsory Foundation Geosciences (3.5 credits):"
		}
	}
	refs := 0
	for _, line := range a.lines {
		t := strings.ToLower(strings.TrimLeft(stripEmphasis(strings.TrimSpace(line)), "-•* "))
		if strings.HasPrefix(t, l) && strings.HasPrefix(strings.TrimSpace(t[len(l):]), ":") {
			continue // a labelled line
		}
		refs += strings.Count(t, l)
	}
	return refs > 0
}

// isListTitle reports a line that names a list and has no course: "Group A:
// Theoretical and Methods 1: Theoretical" before its members.
func isListTitle(b *compBlock) bool {
	text := b.text
	if b.kind == blockLabelled {
		text = b.label + ": " + text
	}
	return b.kind != blockItem && !codeRe.MatchString(text) && complListLabelRe.MatchString(text)
}

// inListHeading reports whether b is an unnumbered line under a heading that
// names a course list (not a year).
func inListHeading(b *compBlock) bool {
	h := b.heading
	return !blkNumbered(b) && h != nil && h.kind == blockHeading && !blkIsYearPhrase(h.section) &&
		(complCourseListHeadingRe.MatchString(h.section) || complListLabelRe.MatchString(h.section))
}

// mayBeList reports an unnumbered, unlabelled line of six or more codes and
// no amount, not under a year or required-courses heading.
func mayBeList(b *compBlock) bool {
	if blkNumbered(b) || b.label != "" || len(b.children) > 0 || !mostlyCodes(b.text) {
		return false
	}
	if blkIsYearPhrase(b.section) || complRequiredHeadingRe.MatchString(b.section) || b.heading != nil && b.heading.isNotes() {
		return false
	}
	return len(codeRe.FindAllString(b.text, -1)) >= 6
}

// subLine reports whether b is a sub-list under a lead: codes with no amount
// of its own. Lines under it are rules on the item, not more lists.
func subLine(b *compBlock) bool {
	return b.kind != blockHeading && !blkNoteLabelRe.MatchString(b.label) && mostlyCodes(b.text) && !complAmountRe.MatchString(b.text) &&
		!slices.ContainsFunc(b.children, func(c *compBlock) bool { return mostlyCodes(c.text) || len(c.children) > 0 })
}

// listZone returns how many blocks after bs[0] are sub-lists that bs[0] leads
// into (two or more), or 0.
func (a *compAssembler) listZone(bs []*compBlock) int {
	b := bs[0]
	if b.kind == blockHeading || len(b.children) > 0 || !complAmountRe.MatchString(b.text) ||
		!complSubListsLeadRe.MatchString(b.text) && !strings.HasSuffix(strings.TrimSpace(b.text), ":") {
		return 0
	}
	n := 0
	for _, c := range bs[1:] {
		if !subLine(c) || blkNumbered(c) && c.kind == blockItem && !blkLetteredRe.MatchString(c.marker) {
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
func leadingSubLines(bs []*compBlock) int {
	n := 0
	for n < len(bs) && subLine(bs[n]) {
		n++
	}
	return n
}

var (
	// complOneOfLeadRe matches a lead whose sub-lists are alternatives: "1.0
	// credit from one of the following languages:".
	complOneOfLeadRe = regexp.MustCompile(`(?i)\bone\s+(?:of\s+)?the\s+following\b`)
)

// listLead reads a lead and its sub-lists as one item: their or under "one of
// the following", the pool of their courses under a bare amount, all of them
// when they add up to the amount, and otherwise text with the sub-lists as
// list definitions.
func (a *compAssembler) listLead(b *compBlock, subs []*compBlock, ctx compCtx) (row *compRow) {
	defer func() {
		// The lines under a sub-list are rules on the item, or on its part.
		for i, c := range subs {
			owner := row
			if row.r.Req == nil && i < len(row.children) {
				owner = row.children[i]
			}
			if len(c.children) > 0 {
				a.blocks(c.children, compCtx{owner: owner, advice: ctx.advice, notes: ctx.notes})
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
		codes = append(codes, complAnyCodeRe.FindAllString(c.text, -1)...)
		raw += "\n" + a.line(c)
		// keep the sub-list as written for its "/" pairs; a long bracketed remark
		// is the item's note
		t := complTitleParenRe.ReplaceAllStringFunc(c.text, func(p string) string {
			if complAnyCodeRe.MatchString(p) {
				return p
			}
			if inner := strings.TrimSpace(p[1 : len(p)-1]); len(strings.Fields(inner)) >= 3 {
				leadNotes = append(leadNotes, cleanNote(inner))
			}
			return ""
		})
		// the sub-list's name says nothing about its courses
		if m := complSubListNameRe.FindStringSubmatch(t); m != nil && !complAnyCodeRe.MatchString(m[1]) {
			t = t[len(m[0]):]
		}
		texts = append(texts, strings.TrimRight(t, ". "))
	}
	row = &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Label: itemLabel(b), Section: b.section, Raw: raw}}
	lead := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(b.text), ":"))
	// bracketed advice in the lead is the item's note, not a rule on its parts
	lead = strings.TrimSpace(complTitleParenRe.ReplaceAllStringFunc(lead, func(p string) string {
		if inner := strings.TrimSpace(p[1 : len(p)-1]); len(strings.Fields(inner)) >= 3 && !codeRe.MatchString(inner) && a.role(inner) == roleAdvisory {
			leadNotes = append(leadNotes, cleanNote(inner))
			return ""
		}
		return p
	}))
	defer func() { row.r.Notes = append(leadNotes, row.r.Notes...) }()
	amount := 0.0
	if m := complAmountRe.FindString(lead); m != "" {
		amount, _ = strconv.ParseFloat(strings.Fields(m)[0], 64)
	}
	switch {
	case complOneOfLeadRe.MatchString(lead) && !slices.ContainsFunc(subs, func(c *compBlock) bool { return !subLine(c) }):
		var opts []*types.ReqNode
		for _, c := range subs {
			n, _ := parseCompletionText(c.text)
			n = checkedBody(n, c.text)
			if v, ok := minCredits(n); n == nil || !ok || hasText(n) || math.Abs(v-amount) > 1e-9 {
				opts = nil
				break
			}
			opts = append(opts, n)
		}
		if opts != nil {
			row.r.Req = orNode(opts...)
		}
	case !slices.ContainsFunc(subs, func(c *compBlock) bool { return !subLine(c) }):
		// a bare amount: the pool of all the sub-lists' courses, with the lead's
		// other clauses as rules on it
		head, rest := poolLeadHead(lead)
		if head == "" {
			break
		}
		n, _ := parseCompletionText(head + ": " + strings.Join(texts, ", "))
		if pool, _ := poolCaps(n); !isPool(pool) || slices.ContainsFunc(dedupe(codes), func(c string) bool { return !slices.Contains(pool.Courses, c) }) {
			n, _ = parseCompletionText(head + ": " + strings.Join(dedupe(codes), ", ")) // the codes alone
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
	if row.r.Req == nil && !complOneOfLeadRe.MatchString(lead) {
		// sub-lists that each read as courses and add up to the amount are all
		// required, each a part of the item
		var parts []*compRow
		sum := 0.0
		for _, c := range subs {
			n, _ := parseCompletionText(c.text)
			n = checkedBody(n, c.text)
			v, ok := minCredits(n)
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
		row.r.Req = textNode(types.TextOther, lead)
		for _, c := range subs {
			// a list's name may come before its courses: "Group 1 – Literary History: ..."
			title, members := c.label, c.text
			if m := complSubListNameRe.FindStringSubmatch(c.text); title == "" && m != nil {
				if _, key := listName(m[1]); key != "" {
					title, members = m[1], c.text[len(m[0]):]
				}
			}
			a.listDef(row, title, members, c, cleanNote(blockLine(c)))
		}
	}
	a.add(row, ctx.owner)
	splitCaps(row)
	return row
}

// complSubListNameRe matches the name before a sub-list's courses:
// "Hydrology/GIS/Remote Sensing: ".
var complSubListNameRe = regexp.MustCompile(`^([^:]{1,80}):\s+`)

// complPoolLeadClauseRe matches a lead that is only an amount over the lists
// after it: "2.0 credits from the following".
var complPoolLeadClauseRe = regexp.MustCompile(`(?i)^(?:any\s+\d+\s+(?:half\s+)?courses\s+\(\s*)?` + amountPat + `\s+credits?\)?\s+(?:(?:chosen|selected|taken)\s+)?(?:in\s+[A-Z][\w ,]*?(?:,?\s+or\s+[A-Z]\w*)\s+)?(?:from|of)(?:\s+(?:the\s+following(?:\s+(?:(?:two|three|four|five|six|\d)\s+)?(?:list|courses|groups?|categories))?|one\s+or\s+more\s+of\s+the\s+groups\s+below|the\s+groups\s+below|the\s+list\s+below))?\s*:?$`)

// poolLeadHead splits a lead into its pool clause ("4.0 credits from") and the
// rules after it. head is empty when lead isn't a pool lead.
func poolLeadHead(lead string) (head string, rest []string) {
	sents := splitComplSentences(lead)
	if len(sents) == 0 {
		return "", nil
	}
	clauses := splitTop(strings.TrimSuffix(strings.TrimSpace(sents[0]), "."), ';')
	first := strings.TrimSpace(clauses[0])
	m := complPoolLeadClauseRe.FindStringSubmatch(strings.TrimSuffix(first, ":"))
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
	// complNumberedListsRe matches a lead over two numbered lists: "2.0 credits
	// to be selected from lists 1 and 2".
	complNumberedListsRe = regexp.MustCompile(`(?i)^(?:plus\s+)?` + amountPat + `\s+credits?\s+(?:to\s+be\s+selected\s+|selected\s+|chosen\s+)?from\s+(?:[\w-]+\s+)?lists\s+\(?(\d)\)?\s+and\s+\(?(\d)\)?`)
	// complListNumRuleRe matches a floor or cap on one of them: "Up to 1.0
	// credit may be taken from list 2".
	complListNumRuleRe = regexp.MustCompile(`(?i)\b(` + atLeastPat + `|up\s+to|(?:at\s+)?a\s+maximum(?:\s+of)?|` + noMoreThanPat + `)\s+` + amountPat + `\s+credits?\b[^.;:]*?\blist\s+\(?(\d)\)?`)
	// complListNumDefRe matches a list's members: "(1) ACT350H1, ...".
	complListNumDefRe = regexp.MustCompile(`(?i)^\((\d)\)\s*(.*)$|\blist\s+\(?(\d)\)?\s*:\s*(.*)$`)
)

// numberedLists reads a lead over two numbered lists, and the lines defining
// them, as one allocation from their union with a row for each floor and cap.
// It returns how many blocks it read, or 0.
func (a *compAssembler) numberedLists(bs []*compBlock, ctx compCtx) int {
	b := bs[0]
	m := complNumberedListsRe.FindStringSubmatch(strings.TrimSpace(b.text))
	if m == nil || b.kind == blockHeading {
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
		for _, r := range complListNumRuleRe.FindAllStringSubmatch(text, -1) {
			v, _ := strconv.ParseFloat(r[2], 64)
			rules = append(rules, rule{floor: strings.EqualFold(r[1], "at least"), amount: v, list: r[3], raw: text})
		}
	}
	read(b.text)
	var lines []*compBlock
	lines = append(lines, b.children...)
	n := 1
	for _, c := range bs[1:] {
		if c.kind == blockHeading || !complListNumDefRe.MatchString(c.marker+" "+stripEmphasis(blockLine(c))) && !complListNumRuleRe.MatchString(c.text) {
			break
		}
		lines = append(lines, c)
		n++
	}
	last := ""
	var walk func(cs []*compBlock)
	walk = func(cs []*compBlock) {
		for _, c := range cs {
			text := strings.TrimSpace(c.text)
			if c.kind == blockItem && strings.HasPrefix(c.marker, "(") {
				text = c.marker + " " + text
			}
			raw = append(raw, a.line(c))
			read(text)
			if d := complListNumDefRe.FindStringSubmatch(stripEmphasis(text)); d != nil {
				num, codes := d[1]+d[3], d[2]+d[4]
				last = num
				lists[num] = append(lists[num], codeRe.FindAllString(codes, -1)...)
				listTexts = append(listTexts, codes)
			} else if last != "" && mostlyCodes(text) {
				lists[last] = append(lists[last], codeRe.FindAllString(text, -1)...) // the codes on the line after "list 2:"
				listTexts = append(listTexts, text)
			}
			walk(c.children)
		}
	}
	walk(lines)
	if len(lists[m[2]]) == 0 || len(lists[m[3]]) == 0 {
		return 0
	}
	all := dedupe(append(slices.Clone(lists[m[2]]), lists[m[3]]...))
	row := &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Label: itemLabel(b), Section: b.section, Raw: strings.Join(raw, "\n"),
		Req: &types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: all}}}
	// a "/" pair in a list is capped as in any pool
	for i, t := range listTexts {
		t = complTitleParenRe.ReplaceAllStringFunc(t, func(p string) string {
			if codeRe.MatchString(p) {
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
		codes := dedupe(lists[r.list])
		if codes == nil {
			continue
		}
		if r.floor {
			a.rowUnder(row, &types.Requirement{Kind: types.RequirementConstraint, Raw: r.raw, Req: &types.ReqNode{Type: types.NodeCredits, Amount: r.amount, Courses: codes}})
		} else {
			a.rowUnder(row, &types.Requirement{Kind: types.RequirementLimit, Raw: r.raw, Req: &types.ReqNode{Type: types.NodeCredits, Courses: codes, MaxAmount: maxAmount(r.amount)}})
		}
	}
	a.add(row, ctx.owner)
	return n
}

// namedListCodes returns the codes under the heading "<name> Courses", or nil.
func (a *compAssembler) namedListCodes(name string) []string {
	want := strings.ToLower(strings.TrimSpace(name)) + " courses"
	for k := range a.lines {
		if strings.ToLower(strings.Trim(a.widenLine(k), " :*_")) != want {
			continue
		}
		var codes []string
		for k++; k < len(a.lines) && (isCodeList(a.widenLine(k)) || mostlyCodes(a.widenLine(k))); k++ {
			codes = append(codes, codeRe.FindAllString(a.lines[k], -1)...)
		}
		return dedupe(codes)
	}
	return nil
}

// complItemListRe matches a sentence listing the courses of the requirement
// before it: "The following courses fulfill the Diversity and Identity
// requirement: ...".
var complItemListRe = regexp.MustCompile(`(?i)^the\s+following\s+courses\s+(?:fulfil+|satisfy|meet|count\s+toward)\s+the\s+(.+?)\s+requirement\s*:\s*(.+)$`)

// itemList reads a complItemListRe sentence as the pool of the item before it,
// whose body is text naming the requirement. A list that isn't codes alone
// joins the item's text. It reports whether s was the item's.
func (a *compAssembler) itemList(row *compRow, s string) bool {
	m := complItemListRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(a.clean(s)), "."))
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
	if am := complLeadAmountRe.FindStringSubmatch(text.Text); am != nil {
		amount, _ = strconv.ParseFloat(am[1], 64)
	}
	pool, _ := parseCompletionText(fmtAmount(amount) + " credit from: " + m[2])
	if pool, _ = poolCaps(pool); amount > 0 && isPool(pool) && pool.Designators == nil {
		*text = *pool
	} else {
		text.Text += ". " + cleanNote(s)
	}
	return true
}

var (
	// complUpToFromListRe matches a list's capped share closing a pool: ", up to
	// one half course from Group H".
	complUpToFromListRe = regexp.MustCompile(`(?i),\s+(up\s+to\s+[^,;]*?\bfrom\s+(?:group|list)\s+\w+)\s*\.?$`)
	// complListMemberTextRe matches text naming more of a pool's members:
	// "400-level courses in Group I".
	complListMemberTextRe = regexp.MustCompile(`(?i)^(?:and\s+)?(?:any\s+)?(?:` + levelPat + `(?:[-/+\s]*` + levelPat + `)*[-+]?\s*-?\s*level\s+)?(?:` + desigPat + `\s+)?courses?\s+(?:in|from)\s+(?:group|list)\s+\w+$`)
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
		case text == nil && it.Type == types.NodeText && complListMemberTextRe.MatchString(strings.TrimSpace(it.Text)):
			text = it
		default:
			return n
		}
	}
	if text == nil {
		return n
	}
	return andNode(append([]*types.ReqNode{orNode(n.Items[0], text)}, caps...)...)
}

var (
	// complCodeTitleLineRe matches a course and its title: "SDS255H1:
	// Histories of Sexuality".
	complCodeTitleLineRe = regexp.MustCompile(`^(` + codePat + `)\s*:\s*[A-Za-z][^\d;/]*$`)
	// complAreaLineRe matches an area and its courses: "Statistics: STA302H1,
	// STA313H1;". The groups are the area and the codes.
	complAreaLineRe = regexp.MustCompile(`^([A-Z][A-Za-z&' ]{1,60}):\s*(` + codePat + `(?:\s*,\s*` + codePat + `)*)\s*[;.]?$`)
	// complAreaOrLineRe matches an area line with a rule of its own: "Astronomy:
	// AST430H1, or AST425Y1 (0.5 credit from point 1 above).".
	complAreaOrLineRe = regexp.MustCompile(`^[A-Z][A-Za-z&' ]{1,60}:\s*(` + codePat + `),?\s+or\s+` + codePat + `\b.*$`)
	// complNamedCoursesLeadRe matches a lead naming the courses under it: "1.0
	// credit from the SDS Foundational Courses:".
	complNamedCoursesLeadRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+from\s+the\s+(?-i:[A-Z])[\w&' -]*?\s+courses\s*:$`)
	// complCoursesSumLeadRe matches a lead naming courses and their total: "SDS
	// Foundational Courses (1.5 credits):".
	complCoursesSumLeadRe = regexp.MustCompile(`(?i)^(?-i:[A-Z])[\w&' -]*?\s+courses\s+\(` + amountPat + `\s+credits?\)\s*:$`)
	// complFollowingListLeadRe matches a lead over one line of courses: "(a
	// minimum of one 0.5 credit from the following list):".
	complFollowingListLeadRe = regexp.MustCompile(`(?i)\((?:a\s+minimum\s+of\s+)?(?:one\s+)?` + amountPat + `\s+credits?\s+from\s+the\s+following\s+list\)\s*:$`)
	// complAllCoursesLineRe matches that line: "All 400-level SDS and JNS
	// courses.".
	complAllCoursesLineRe = regexp.MustCompile(`^All\s+(.+?\s+courses)\.?$`)
)

// leadLines reads an item whose lead names the courses on the lines under it,
// when they have too few codes for listLead: a pool of them, all of them, an
// or of a pool per area, or one line's pool. row is the item's allocation, not
// yet added. It reports whether it read the item.
func (a *compAssembler) leadLines(b *compBlock, row *compRow, ctx compCtx) bool {
	lead := strings.TrimSpace(b.text)
	kids := b.children
	if len(kids) == 0 || !strings.HasSuffix(lead, ":") {
		return false
	}
	plain := func(c *compBlock) bool {
		return c.kind != blockHeading && len(c.children) == 0 && !complAmountRe.MatchString(c.text)
	}
	// text is a line as written, with its label: "Statistics: STA302H1, ..."
	text := func(c *compBlock) string {
		if c.kind == blockLabelled && c.label != "" {
			return c.label + ": " + strings.TrimSpace(c.text)
		}
		return strings.TrimSpace(c.text)
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
	case complNamedCoursesLeadRe.MatchString(lead):
		amount, _ := strconv.ParseFloat(complNamedCoursesLeadRe.FindStringSubmatch(lead)[1], 64)
		if n = run(complCodeTitleLineRe); n < 2 || n < len(kids) && complCodeTitleLineRe.MatchString(text(kids[n])) {
			return false
		}
		var codes []string
		for _, c := range kids[:n] {
			codes = append(codes, complCodeTitleLineRe.FindStringSubmatch(text(c))[1])
		}
		if weightOf(codes) < amount-1e-9 {
			return false
		}
		body = &types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: dedupe(codes)}
	case complCoursesSumLeadRe.MatchString(lead):
		amount, _ := strconv.ParseFloat(complCoursesSumLeadRe.FindStringSubmatch(lead)[1], 64)
		if n = run(complCodeTitleLineRe); n < 2 || n < len(kids) && complCodeTitleLineRe.MatchString(text(kids[n])) {
			return false
		}
		var codes []string
		for _, c := range kids[:n] {
			codes = append(codes, complCodeTitleLineRe.FindStringSubmatch(text(c))[1])
		}
		if math.Abs(weightOf(codes)-amount) > 1e-9 {
			return false // all of them, so they must add up to the amount
		}
		body = andNode(courseNodes(dedupe(codes))...)
	case complOneOfLeadRe.MatchString(lead) && complAmountRe.MatchString(lead):
		amount, err := strconv.ParseFloat(strings.Fields(complAmountRe.FindString(lead))[0], 64)
		if n = run(complAreaLineRe); err != nil || n < 2 {
			return false
		}
		var opts []*types.ReqNode
		for _, c := range kids[:n] {
			codes := codeRe.FindAllString(complAreaLineRe.FindStringSubmatch(text(c))[2], -1)
			switch w := weightOf(codes); {
			case len(codes) == 1 && math.Abs(w-amount) < 1e-9:
				opts = append(opts, &types.ReqNode{Type: types.NodeCourse, Code: codes[0]})
			case w >= amount-1e-9:
				opts = append(opts, &types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: dedupe(codes)})
			default:
				return false // an area that can't make the amount
			}
		}
		if c := kids[min(n, len(kids)-1)]; n < len(kids) && c.kind != blockHeading && len(c.children) == 0 {
			// an area with a rule of its own: its first course, and the line
			if m := complAreaOrLineRe.FindStringSubmatch(text(c)); m != nil {
				if math.Abs(courseWeight(m[1])-amount) < 1e-9 {
					opts = append(opts, &types.ReqNode{Type: types.NodeCourse, Code: m[1]})
				}
				opts = append(opts, textNode(types.TextOther, cleanNote(text(c))))
				n++
			}
		}
		body = orNode(opts...)
	case complFollowingListLeadRe.MatchString(lead) && len(kids) >= 1 && plain(kids[0]) && complAllCoursesLineRe.MatchString(text(kids[0])):
		m := complFollowingListLeadRe.FindStringSubmatch(lead)
		p, _ := parseCompletionText(m[1] + " credit from " + complAllCoursesLineRe.FindStringSubmatch(text(kids[0]))[1])
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
	a.children(kids[n:], b.text, ctx, row)
	return true
}
