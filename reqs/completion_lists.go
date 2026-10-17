package reqs

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Lists: lines that set out the courses items draw on, rather than
// requirements. A list read here is recorded as a definition (listDef) and
// kept as a note, or as the text of the item over it with the list as its
// notes, never required courses; once every row is read, the lists rows
// refer to become named lists and their notes go (completion_named.go).
// Also the leads over sub-lists (listLead), numbered lists (numberedLists),
// a requirement's own list (itemList) and text naming more members of a
// pool (listMemberAlt).
//
// The regexes below tell lists from requirements: the ones a lead points
// down with, then the headings and labels that name lists.

var (
	// complCodeLineLeadRe: a lead-in ending in "following", "listed below",
	// "below" (with "list" or not) or ":", which takes the line of codes
	// after it (blocks): "2.5 credits from the following list:".
	complCodeLineLeadRe = regexp.MustCompile(`(?i)\b(?:following|listed\s+below|below)\b(?:\s+list)?\s*:?$|:$`)
	// complPoolListedBelowRe: a pool drawn from the courses listed below:
	// "4.0 credits chosen from those listed below, ..." (blocks, and header,
	// where the list is the total's own pool).
	complPoolListedBelowRe = regexp.MustCompile(`(?i)\bfrom\s+(?:those|the\s+courses?)\s+listed\s+below\b`)
	// complFromListBelowRe: "from the list below": an item's lead over the
	// sub-lists under it: "4.5 credits chosen from the list below" (item).
	complFromListBelowRe = regexp.MustCompile(`(?i)\bfrom\s+the\s+list\s+below\b`)
	// complTheFollowingRe: "the following", with "list", "list of courses" or
	// "courses" after it: the words the line of codes after a lead stands for
	// ("2.5 credits from the following list, including ..."; item).
	complTheFollowingRe = regexp.MustCompile(`(?i)\bthe\s+following(?:\s+(?:list(?:\s+of\s+courses)?|courses))?\b`)
	// complSubListsLeadRe: "from the following", "following list" or "listed
	// below": a lead with an amount over the sub-lists after it: "A minimum
	// of 3.5 credits from the following list ..." (listZone).
	complSubListsLeadRe = regexp.MustCompile(`(?i)\bfrom\s+the\s+following\b|\bfollowing\s+list\b|\blisted\s+below\b`)
	// complListsElsewhereRe: anywhere in the program's text, a sign that its
	// items draw on lists set out elsewhere: "from either one concentration
	// below", "2.5 credits from the groups listed below", "from each of the
	// two chosen language areas" (listLine).
	complListsElsewhereRe = regexp.MustCompile(`(?i)\bbelow\b|\bfollowing\s+(?:groups?|lists?|concentrations?|areas?|subject\s+areas?|clusters?)\b|\bconcentrations?\b|\b(?:language|subject)\s+areas?\b|\bgroups?\s+[A-Z1-9]\b`)

	// complListSetHeadingRe: a heading over the lists the items draw on:
	// "Course Groups", "Concentrations", "_Society-Culture Courses_", "Group
	// C", "Language Areas", "Disciplinary Focuses" (listLine).
	complListSetHeadingRe = regexp.MustCompile(`(?i)\bcourses?\s*:?$|\bcourse\s+groups?\b|\bconcentrations?\b|\bgroups?\b|\bareas?\b|\bclusters?\b|\blists?\b|\bfoc(?:us|uses|i)\b|\bstreams?\b|\bfoundations?\b|\bcategor(?:y|ies)\b|\bthemes?\b|\bsubgroups?\b|\bnot\s+program\s+requirements\b`)
	// complCourseListHeadingRe: a heading over one course list: "Group A:
	// Ancient, Medieval", "Cluster B - Learning, Perception, ...",
	// "(Sub-group: Africa)", "Core Group", "Current Eligible Electives",
	// "Medieval Stream" (inListHeading). Unlike complListSetHeadingRe it
	// takes electives and eligible courses, and not areas, concentrations,
	// focuses or themes.
	complCourseListHeadingRe = regexp.MustCompile(`(?i)\b(?:groups?|lists?|clusters?|sub-?groups?|categor(?:y|ies)|electives?|eligible|streams?)\b`)
	// complRequiredHeadingRe: a heading over required courses, not a list:
	// "Required Courses (8.0 credits)", "Core Courses", "Foundational courses".
	complRequiredHeadingRe = regexp.MustCompile(`(?i)\b(?:required|core|foundation(?:al|s)?|compulsory|program\s+requirements)\b`)
	// complChoiceListLabelRe: a label naming a list of courses to choose
	// from: "Approved Economics courses", "Eligible courses".
	complChoiceListLabelRe = regexp.MustCompile(`(?i)\b(?:approved|eligible|accepted|elective|optional)\b.*\bcourses?\b`)
	// complMemberLineRe: a line that says what a list holds rather than listing
	// it: "Any URB or JGU courses not already taken to fulfill requirement 3"
	// (under "Urban Studies:"), "Any other GGR, JGE, JIG, JUG courses at the
	// 200/300/400-level not listed in Group A" (under "Group C").
	complMemberLineRe = regexp.MustCompile(`^Any\s+(?:other\s+)?(?:(?-i:[A-Z]{3})(?:\s*(?:,|or|and|/)\s*(?-i:[A-Z]{3}))*\s+)?courses?\b`)
)

// listLine reports whether b, at the program level, is a line of a list
// rather than a requirement: an unnumbered line of codes with no amount of
// its own, not under a year or an amount, that comes under a heading naming
// lists (not one naming required courses), or after the numbered items when
// they draw on lists set out below, or that only repeats courses earlier
// rows already require.
func (a *compAssembler) listLine(b *compBlock) bool {
	// Members that count only with permission ("and with permission of the
	// director: TRN300H1, ..., 1.0 credit from TRN160Y1, ...") are the
	// list's maybe item (permCodes): their amount isn't the line's.
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
			// After the numbered items, a heading without an amount names a
			// list ("_Core Courses_", "_Foundations_"): the items are the
			// requirements.
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

// namedElsewhere reports whether labelled line b names a list another line
// refers to: its label, of two or more words, appears in the program's text
// other than as the label of a line ("Pre-1800 British Literature: ENG220H1;
// ..." and "2.0 ENG credits from Pre-1800 British Literature courses from
// list below"). Another line with the same label is another list, not a
// reference ("Mineralogy and Petrology:" under two headings in ASSPE0509),
// and a line under a heading that states an amount is part of what the
// heading requires.
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

// subLine reports whether b is a sub-list under (or after) a lead: a
// labelled line, a bullet, a lettered item or a line of codes, listing
// codes with no amount of its own. Lines under it are rules on the item
// ("d. Biogeography: ..." then "No more than one (0.5) of GGR273H1, ...",
// listLead), not more lists.
func subLine(b *compBlock) bool {
	return b.kind != blockHeading && !blkNoteLabelRe.MatchString(b.label) && mostlyCodes(b.text) && !complAmountRe.MatchString(b.text) &&
		!slices.ContainsFunc(b.children, func(c *compBlock) bool { return mostlyCodes(c.text) || len(c.children) > 0 })
}

// listZone returns how many blocks after bs[0] are the sub-lists bs[0]
// leads into, or 0: bs[0] is an unnumbered line of prose or an item with an
// amount that ends in ":" or takes "the following list", and the sub-lists
// are two or more lines after it (subLine). One line of codes is read with
// its lead (blocks).
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

// blkLetteredRe: a lettered marker: "a.", "(b)".
var blkLetteredRe = regexp.MustCompile(`^\(?[a-z]\)?[.)]?$`)

// leadingSubLines returns how many of bs, from the first, are sub-lists
// (subLine).
func leadingSubLines(bs []*compBlock) int {
	n := 0
	for n < len(bs) && subLine(bs[n]) {
		n++
	}
	return n
}

var (
	// complOneOfLeadRe: a lead whose sub-lists are alternatives: "1.0 credit
	// from one of the following languages:".
	complOneOfLeadRe = regexp.MustCompile(`(?i)\bone\s+(?:of\s+)?the\s+following\b`)
)

// listLead reads a lead and its sub-lists (listZone, or the lines under an
// item) as one item, never the first sub-list alone:
//   - under "one of the following", their or, when each sub-list reads as
//     courses worth the lead's amount ("- Estonian: EST100H1 and EST101H1");
//   - under a lead that is only an amount ("2.0 credits from the
//     following:"), the pool of all their courses;
//   - sub-lists that each read as courses and add up to the amount ("10.0
//     credits of core courses:"), a group of them, all required;
//   - otherwise the lead is text and the sub-lists its notes ("2.5 credits
//     from the following, so that courses are from at least two of the four
//     areas:", "A minimum of 3.5 credits from the following list that have
//     not been used ...").
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
		// The lead and its last sub-list end the item: a rule line after them
		// is about it (poolRuleLine).
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
		// The sub-list as written, for its "/" pairs; a remark in brackets
		// ("(Note: for additional details ... visit the graduate timetable
		// webpage)") is the item's note.
		t := complTitleParenRe.ReplaceAllStringFunc(c.text, func(p string) string {
			if complAnyCodeRe.MatchString(p) {
				return p
			}
			if inner := strings.TrimSpace(p[1 : len(p)-1]); len(strings.Fields(inner)) >= 3 {
				leadNotes = append(leadNotes, cleanNote(inner))
			}
			return ""
		})
		// "a. Hydrology/GIS/Remote Sensing: GGR273H1, ...": the sub-list's
		// name says nothing about its courses.
		if m := complSubListNameRe.FindStringSubmatch(t); m != nil && !complAnyCodeRe.MatchString(m[1]) {
			t = t[len(m[0]):]
		}
		texts = append(texts, strings.TrimRight(t, ". "))
	}
	row = &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Label: itemLabel(b), Section: b.section, Raw: raw}}
	lead := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(b.text), ":"))
	// "1.5 credits from the following three groups (students are strongly
	// encouraged to include 0.5 credit from each group)": advice in brackets
	// is the item's note, not a rule on its parts.
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
		// "2.0 credits from the following:", "4.0 credits chosen from the
		// following list; at least 3.0 credits of which must be ...": the
		// pool of all the sub-lists' courses, and the lead's other clauses
		// rules on it.
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
		// "10.0 credits of core courses:" then "Foundation courses: ...",
		// "200-level physics, math, and earth sciences: ...": sub-lists
		// that each read as courses and together make up the amount are
		// all required, each a part of the item.
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
			// "Group 1 – Literary History: FRE318H1/ ...": the list's name
			// is before its courses.
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

// complSubListNameRe: the name before a sub-list's courses:
// "Hydrology/GIS/Remote Sensing: ".
var complSubListNameRe = regexp.MustCompile(`^([^:]{1,80}):\s+`)

// complPoolLeadClauseRe: the first clause of a lead that is only an amount
// drawing on the lists after it: "2.0 credits from the following", "4.0
// credits chosen from the following list", "1.5 credits from one or more of
// the groups below", "1.5 credits in Literature, History, or Art from",
// "Any 2 courses (1.0 credit) from".
var complPoolLeadClauseRe = regexp.MustCompile(`(?i)^(?:any\s+\d+\s+(?:half\s+)?courses\s+\(\s*)?(\d+(?:\.\d+)?)\s+credits?\)?\s+(?:(?:chosen|selected|taken)\s+)?(?:in\s+[A-Z][\w ,]*?(?:,?\s+or\s+[A-Z]\w*)\s+)?(?:from|of)(?:\s+(?:the\s+following(?:\s+(?:(?:two|three|four|five|six|\d)\s+)?(?:list|courses|groups?|categories))?|one\s+or\s+more\s+of\s+the\s+groups\s+below|the\s+groups\s+below|the\s+list\s+below))?\s*:?$`)

// poolLeadHead splits a lead into its pool clause ("4.0 credits from") and
// the clauses and sentences after it, which are rules on the pool; head is
// empty when the lead isn't a pool lead.
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
	// complNumberedListsRe: a lead drawing on two numbered lists: "plus 2.5
	// credits from Genetics Lists 1 and 2, distributed as follows:", "2.0
	// credits to be selected from lists 1 and 2 (...):".
	complNumberedListsRe = regexp.MustCompile(`(?i)^(?:plus\s+)?(\d+(?:\.\d+)?)\s+credits?\s+(?:to\s+be\s+selected\s+|selected\s+|chosen\s+)?from\s+(?:[\w-]+\s+)?lists\s+\(?(\d)\)?\s+and\s+\(?(\d)\)?`)
	// complListNumRuleRe: a floor or cap on one of them: "At least 1.5 credits
	// must be taken from list 1", "Up to 1.0 credit may be taken from list
	// 2", "students can only use at a maximum 1.0 credit from list (2)".
	complListNumRuleRe = regexp.MustCompile(`(?i)\b(at\s+least|up\s+to|(?:at\s+)?a\s+maximum(?:\s+of)?|no\s+more\s+than)\s+(\d+(?:\.\d+)?)\s+credits?\b[^.;:]*?\blist\s+\(?(\d)\)?`)
	// complListNumDefRe: a list's members: "(1) ACT350H1, ...", "At least 1.5
	// credits must be taken from list 1: MGY425H1, ...".
	complListNumDefRe = regexp.MustCompile(`(?i)^\((\d)\)\s*(.*)$|\blist\s+\(?(\d)\)?\s*:\s*(.*)$`)
)

// numberedLists reads a lead over two numbered lists and the lines that
// define them into one allocation (bs[0] the lead; it reports how many
// blocks it read, 0 when bs[0] isn't one): the lead's amount from the
// union of the lists, a constraint for each list's floor ("At least 1.5
// credits must be taken from list 1") and a limit for each cap ("Up to 1.0
// credit may be taken from list 2"), never the lists' courses all required.
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
	// A "/" pair in a list ("ACT455H1/ ACT466H1 (whichever course that was
	// not taken as a mandatory course)") is capped as in any pool.
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

// namedListCodes returns the codes of the list named name in the program's
// text: the lines of codes under the heading "<name> Courses" ("**Equivalent
// Courses**"), or nil.
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

// complWidenNamedListRe: a capped share from a list named below (widen,
// readWidenings): "Students may take up to a 0.5 credit from the Equivalent
// Courses list below." The list is the lines of codes under its name.
var complWidenNamedListRe = regexp.MustCompile(`(?i)^students\s+may\s+take\s+up\s+to\s+(?:a\s+)?` + complWidenNumPat + `\s+credits?\s+from\s+the\s+(.+?)\s+list\s+below$`)

// complItemListRe: a sentence that lists the courses of the requirement it
// follows: "The following courses fulfill the Diversity and Identity
// requirement: POL101H1, POL195H1, ...".
var complItemListRe = regexp.MustCompile(`(?i)^the\s+following\s+courses\s+(?:fulfil+|satisfy|meet|count\s+toward)\s+the\s+(.+?)\s+requirement\s*:\s*(.+)$`)

// itemList reads a complItemListRe sentence into the item before it, whose
// body is text naming the requirement ("1.0 credit in Diversity and
// Identity"): its pool is those courses. When the list doesn't read as
// codes alone, it joins the item's text, never a note that hides it. It
// reports whether the sentence was the item's.
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
	// complUpToFromListRe: a list's capped share closing a pool: ", up to one
	// half course from Group H".
	complUpToFromListRe = regexp.MustCompile(`(?i),\s+(up\s+to\s+[^,;]*?\bfrom\s+(?:group|list)\s+\w+)\s*\.?$`)
	// complListMemberTextRe: text the grammar left beside a pool that names
	// more of its members: "400-level courses in Group I", "courses from
	// Group C".
	complListMemberTextRe = regexp.MustCompile(`(?i)^(?:and\s+)?(?:any\s+)?(?:[1-4]00(?:[-/+\s]*[1-4]00)*[-+]?\s*-?\s*level\s+)?(?:(?-i:[A-Z]{3})\s+)?courses?\s+(?:in|from)\s+(?:group|list)\s+\w+$`)
)

// listMemberAlt makes and(pool, text naming more of its members) the
// pool or, with the text, Review: "Any 2 courses (1.0 credit) from GGR401H1,
// ..., and 400-level courses in Group I" read as the courses and the text
// would be NotMet for a student who uses a Group I course, and Review for
// everyone else.
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
