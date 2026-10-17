package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The scraper stores completion text like enrolment text (scraper/blocks.go):
// one block per line, "- " bullets, "1." / "a." numbering, a nested list
// indented two spaces per level and an item's continuation lines indented to
// its text ("1. " gives three spaces), **bold** and _italic_. Calendar authors
// also number by hand ("1)", "(a)", "i)") and put the number inside the
// emphasis ("**1. Students cannot …**").
//
// completionBlocks reads the text into a tree of blocks, one per line. An
// indented line hangs under the line above it. A heading gives the blocks
// after it a section, until the next heading among its siblings. Items carry
// the calendar's own label: "4", "Higher Years 2" when the numbering starts
// again under a heading, "4a" for a nested item.

type blockKind int

const (
	blockProse    blockKind = iota // a paragraph
	blockHeading                   // "First Year:", "_First year_ (2.5 credits):", "**Group A**", "Notes:"
	blockItem                      // a numbered or lettered item: "1.", "1)", "(a)", "i)"
	blockBullet                    // "- "
	blockLabelled                  // "**Group A:** CSC209H1; …", "Note: …", "- Computer Science: …"
)

var blockKindNames = [...]string{"prose", "heading", "item", "bullet", "labelled"}

// isNotes reports whether a heading's name ends in notes: "Notes:", "**Important
// Notes:**", "_Applied Genetics and Biotechnology Major Notes:_".
func (b *compBlock) isNotes() bool { return blkEndsInNotesRe.MatchString(b.section) }

func (k blockKind) String() string { return blockKindNames[k] }

type compBlock struct {
	kind   blockKind
	marker string // items and bullets: the marker as written, "1.", "(a)", "ii)", "-"
	// Items: the calendar's label, "4", "Higher Years 2", "4a". Labelled lines:
	// the text before the colon, "Group A", "First Year (3.0 credits)".
	label  string
	text   string // the line without indent, marker, label and emphasis
	indent int    // leading spaces
	line   int    // index of the line in the raw text
	// The innermost heading the block sits under, without a credit
	// parenthetical or footnote asterisk, for display. A heading's own section
	// is its name, and a labelled line naming a year ("First Year (3.0
	// credits): …") is a section of its own.
	section string
	// The innermost heading the block sits under; for a heading, the heading
	// it sits under (PSY "Cluster A" under "Group 1"). Nil at the top.
	heading *compBlock
	// Headings: the rank that decides which heading ends which, smaller is
	// higher (blkHeadingLevel).
	level    int
	children []*compBlock // the indented lines below, and lists adopted by their lead-in
	// cont: the raw lines a hard-wrapped paragraph continues on (blkJoinWraps).
	cont []int
}

var (
	// Emphasis opened before an item's marker: "**1. Students …**".
	blkLeadEmphRe = regexp.MustCompile(`^(?:\*\*|_)+`)
	// "1.", "1)", "(1)", "a.", "a)", "(a)", "A.", "ii)", "(iv)", "III.".
	blkItemRe     = regexp.MustCompile(`^(\d{1,2}[.)]|\(\d{1,2}\)|[a-zA-Z][.)]|\([a-zA-Z]\)|(?:[ivx]{2,4}|[IVX]{2,4})[.)]|\((?:[ivx]{2,4}|[IVX]{2,4})\))(?:\*\*|_)*\s+`)
	blkBulletRe   = regexp.MustCompile(`^[-•]\s+`)
	blkBoldSpanRe = regexp.MustCompile(`\*\*.*?\*\*`)
	// An italic span, its underscores next to a space or punctuation:
	// "_First year_ (2.5 credits):", "any 300-/ 400-level CSC course _except_".
	blkItalicSpanRe = regexp.MustCompile(`(^|[\s(\[/,;:.*])_.*?_($|[\s)\]/,;:.*])`)
	// A bracketed span with none inside it: "(3.0 credits)", "[( SLA107Y1/ …)]"
	// goes from the inside out.
	blkInnerParenRe = regexp.MustCompile(`\([^()]*\)|\[[^\[\]]*\]`)
	blkCreditRe     = regexp.MustCompile(`(?i)\bcredits?\b`)
	blkAmountRe     = regexp.MustCompile(`(?i)\d+(?:\.\d+)?\s*(?:credits?|fces?)\b`)
	blkDecimalRe    = regexp.MustCompile(`\d\.\d`) // "7.0", ".50" doesn't count
	// A total or an amount, not a name: "(8.0 credits)", "3.0 credits".
	blkStartsAmountRe = regexp.MustCompile(`^\(?\s*\d`)
	// Words that make a line ending in ":" a lead-in rather than a heading:
	// "At least two of the following four options:", "Of these:".
	blkLeadInRe = regexp.MustCompile(`(?i)\b(?:following|follows|below|above|include|includes|including|must|should|may|requires?|choose|chosen|selected|of these|one of|plus|suggest|complete)\b`)
	// A note's label: "Note", "Note 1", "NOTES".
	blkNoteLabelRe = regexp.MustCompile(`(?i)^\*?notes?(?:\s+\d+)?$`)
	// A heading that is only notes: "Notes", "Important Notes", "N.B.".
	blkNotesHeadingRe = regexp.MustCompile(`(?i)^\*?(?:(?:important|advising)\s+)?(?:notes?|n\.\s?b\.?)$`)
	// "First Year", "Higher Years", "Third or Fourth Year", "Second Year and
	// Higher", "Years 3 & 4", "In first year or higher".
	blkYearPhraseRe = regexp.MustCompile(`(?i)^(?:(?:first|second|third|fourth|fifth|1st|2nd|3rd|4th|higher|later|upper|lower|early|senior|subsequent|any|in|and|or|years?|\d)(?:[\s,/&*-]+|$))+$`)
	blkHasYearRe    = regexp.MustCompile(`(?i)\byears?\b`)
	// "1.1.0 credit from …": a marker glued to an amount.
	blkGluedItemRe = regexp.MustCompile(`^(\d{1,2}\.)\d+\.\d`)
	// A label that is a number, not a name: "2.0 credits", "4.5 of …".
	blkNumberWordRe = regexp.MustCompile(`^\d+(?:\.\d+)?(?:\s|$)`)
	// Headings ranked below a group: PSY "Cluster A", ANT "Subgroup C (i)",
	// HIS "(Sub-group: Africa)".
	blkSubHeadingRe = regexp.MustCompile(`(?i)^\(?(?:cluster|sub-?group)\b`)
	// Bullets of advice that end a list (ASSPE0151 "- It is highly recommended
	// that Religion Specialists …", "- Please note that UTM and UTSC courses …").
	blkAdviceRe = regexp.MustCompile(`(?i)^(?:it is (?:highly |strongly )?recommended|please note|note that|students are (?:encouraged|advised))\b`)
	// A heading over focuses: ASSPE2270 "**Disciplinary Focuses**".
	blkFociRe = regexp.MustCompile(`(?i)\bfocuses\b|\bfoci\b`)
	// The program's notes: ASMAJ1053 "Applied Genetics and Biotechnology Major
	// Notes".
	blkProgramNotesRe = regexp.MustCompile(`(?i)\b(?:major|minor|specialists?|program)\s+notes?$`)
	// A heading name ending in notes: "Notes", "Important Notes", "… Major
	// Notes", "N.B.".
	blkEndsInNotesRe = regexp.MustCompile(`(?i)(?:^|\s)(?:notes?|n\.\s?b\.?)$`)
	// Words that make an item ending without a colon lead into the bullets
	// after it (ASMAJ1240 "0.5 credit from each of the three Thematic
	// Clusters.").
	blkListWordRe = regexp.MustCompile(`(?i)\b(?:following|below|each of the|clusters?)\b`)
)

// completionBlocks reads completion text into a block tree.
func completionBlocks(raw string) []*compBlock {
	var top, open []*compBlock
	for i, l := range strings.Split(raw, "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		b := blkReadLine(i, l)
		for len(open) > 0 && open[len(open)-1].indent >= b.indent {
			open = open[:len(open)-1]
		}
		if len(open) == 0 {
			top = append(top, b)
		} else {
			p := open[len(open)-1]
			p.children = append(p.children, b)
		}
		open = append(open, b)
	}
	top = blkAdoptItemBodies(blkAdoptSubItems(blkJoinWraps(top)))
	blkSetSections(top, nil, "")
	blkLabelItems(top)
	return top
}

// blkAdoptSubItems hangs lists written flush left under the line that leads into
// them:
//   - bullets after a lead-in ending in ":" (ASMAJ1689 "3. 3.0 credits of
//     courses in total selected from among the following groups:" then "-
//     **Group A:** …");
//   - a lettered list after a numbered item or a lead-in (ASSPE0652 "**1.
//     Geographic Distribution (200+ level):**" then "a) …", "b) …").
//
// A heading ending in ":" isn't a lead-in: the lines after it get its section.
// Nor is a bullet whose list is already indented under it: the bullets
// flush with it are the next items of its own list (ASMAJ0151 "- 1.0 credit
// of related courses ... The list of eligible courses includes:", its
// indented list, then "- Up to two language courses ...").
func blkAdoptSubItems(top []*compBlock) []*compBlock {
	var out []*compBlock
	for i := 0; i < len(top); i++ {
		p := top[i]
		out = append(out, p)
		leadIn := p.kind != blockHeading && strings.HasSuffix(p.text, ":") && !(blkIsBullet(p.marker) && len(p.children) > 0)
		j := i + 1
		if j < len(top) && blkIsBullet(top[j].marker) && (leadIn || blkLeadsIntoBullets(p)) {
			for j < len(top) && blkIsBullet(top[j].marker) && !blkAdviceRe.MatchString(top[j].text) {
				j++
			}
		} else if leadIn || blkNumbered(p) {
			prevStyle, prevOrd := 0, 0
			for ; j < len(top) && top[j].kind == blockItem; j++ {
				style, ord := blkItemOrdinal(blkMarkerLabel(top[j].marker), prevStyle, prevOrd)
				if style == blkStyleNumber || ord != prevOrd+1 || (j > i+1 && style != prevStyle) {
					break
				}
				prevStyle, prevOrd = style, ord
			}
		}
		p.children = append(p.children, top[i+1:j]...)
		i = j - 1
	}
	return out
}

// blkLeadsIntoBullets reports whether a numbered item that doesn't end in ":"
// still owns the bullets after it: it doesn't end a sentence ("1. Completion
// of either", "7. 4.0 additional SPA credits"), or it points at them ("0.5
// credit from each of the three Thematic Clusters."). It is deliberately
// permissive: bullets right after an item are about it far more often than
// not, and the advice bullets that end the run are cut off by blkAdviceRe.
func blkLeadsIntoBullets(p *compBlock) bool {
	return blkNumbered(p) && (!strings.HasSuffix(p.text, ".") || blkListWordRe.MatchString(p.text))
}

// blkAdoptItemBodies hangs the flush-left lines after a numbered item ending
// in ":" under it, up to the next numbered item, a heading or a year line:
// labelled lines and lettered items (ASMAJ1205 "2. 2.0 credits of language
// coursework in one language area chosen from the list below:" then
// "_Czech and Slovak:_ …"), lead-ins, and prose with a course code or an
// amount (ASMAJ1019 "2. Higher years:" then "3.0 to 4.0 credits from any NMC
// courses"). Other prose ends the body, unless it is the first line of an
// item with nothing under it yet (ASMAJ2745 "3. 2.5 credits of elective
// courses:" then "**In addition to the above core courses, …**"). A lettered
// item in the body takes the prose after it (ASMAJ2745 "a) Earth Surface
// Processes" then its courses).
func blkAdoptItemBodies(top []*compBlock) []*compBlock {
	var out []*compBlock
	for i := 0; i < len(top); i++ {
		p := top[i]
		out = append(out, p)
		if !blkNumbered(p) || !strings.HasSuffix(p.text, ":") {
			continue
		}
		j := i + 1
		for j < len(top) && !blkEndsItemBody(p, top[j], j == i+1) {
			j++
		}
		p.children = append(p.children, blkLetteredBodies(top[i+1:j])...)
		i = j - 1
	}
	return out
}

// blkEndsItemBody reports whether line c ends the body of item p: a heading, a
// numbered item, a year line, a footnote (ASMAJ1533 "*Students admitted to
// the program with ECO101H1/ …"), or prose with no course code or amount
// that isn't a lead-in, unless it is the first line of an item with nothing
// under it yet.
func blkEndsItemBody(p, c *compBlock, first bool) bool {
	switch {
	case c.kind == blockHeading || blkNumbered(c) || blkStartsSection(c) || strings.HasPrefix(c.text, "*"):
		return true
	case c.kind != blockProse || first && len(p.children) == 0:
		return false
	}
	return !codeRe.MatchString(c.text) && !blkStartsAmountRe.MatchString(c.text) && !strings.HasSuffix(c.text, ":")
}

// blkLetteredBodies hangs the prose and bullets after a lettered item under
// it.
func blkLetteredBodies(bs []*compBlock) []*compBlock {
	var out []*compBlock
	var item *compBlock
	for _, b := range bs {
		switch {
		case b.kind == blockItem:
			item = b
		case item != nil && (b.kind == blockProse || b.kind == blockBullet):
			item.children = append(item.children, b)
			continue
		default:
			item = nil
		}
		out = append(out, b)
	}
	return out
}

func blkIsBullet(m string) bool { return m == "-" || m == "•" }

// blkNumbered reports whether a block is an item with a number: "1.", "(2)".
func blkNumbered(b *compBlock) bool {
	if b.kind != blockItem {
		return false
	}
	style, _ := blkItemOrdinal(blkMarkerLabel(b.marker), 0, 0)
	return style == blkStyleNumber
}

// blkWrapEndRe: a line that ends mid-sentence: no full stop, colon or
// semicolon.
var blkWrapEndRe = regexp.MustCompile(`[^.:;!?)]\s*$|,\s*$`)

// blkWrapStartRe: a line that continues the sentence above: a lowercase word
// or a code.
var blkWrapStartRe = regexp.MustCompile(`^(?:[a-z]|` + codePat + `)`)

// blkWrapWordEndRe, blkWrapWordStartRe: a sentence broken between two
// lowercase words ("... and/or STA261H1 before being admitted" then "to the
// Minor, ..."), a wrap even when the line above has codes enough to look
// like a list (mostlyCodes).
var (
	blkWrapWordEndRe   = regexp.MustCompile(`\b[a-z]{2,}\s*$`)
	blkWrapWordStartRe = regexp.MustCompile(`^[a-z]{2,}\b`)
)

// blkJoinWraps joins a paragraph hard-wrapped over several lines ("**Note:**
// If you completed STA238H1 ... before being admitted" then "to the Minor,
// please note that ...") into one block: a line of prose (or a note) that
// ends mid-sentence, followed at the same indent by a line of prose that
// starts with a lowercase word or a code. Lines of codes are steps of a
// list, not a wrapped sentence.
func blkJoinWraps(bs []*compBlock) []*compBlock {
	var out []*compBlock
	for _, b := range bs {
		b.children = blkJoinWraps(b.children)
		if n := len(out); n > 0 {
			p := out[n-1]
			if (p.kind == blockProse || p.kind == blockLabelled && blkNoteLabelRe.MatchString(p.label)) && len(p.children) == 0 &&
				b.kind == blockProse && b.indent == p.indent && blkWrapEndRe.MatchString(p.text) && blkWrapStartRe.MatchString(b.text) &&
				(!mostlyCodes(p.text) || blkWrapWordEndRe.MatchString(p.text) && blkWrapWordStartRe.MatchString(b.text)) && !isCodeList(b.text) {
				p.text += " " + b.text
				p.cont = append(append(p.cont, b.line), b.cont...)
				p.children = b.children
				continue
			}
		}
		out = append(out, b)
	}
	return out
}

// blkItemText is an item's or bullet's text without emphasis. An italic
// note after a code ("( CHM135H1 and CHM136H1)/ CHM151Y1 _Transfer credits
// will be accepted ..._") becomes a sentence of its own, so it can't
// swallow the item.
func blkItemText(s string) string {
	return stripEmphasis(complItalicNoteRe.ReplaceAllString(s, "$1. $2"))
}

// blkReadLine classifies one line. Its section and label come later.
func blkReadLine(i int, raw string) *compBlock {
	t := strings.TrimRight(raw, " ")
	b := &compBlock{indent: len(t) - len(strings.TrimLeft(t, " ")), line: i}
	t = strings.TrimSpace(t)
	lead := blkLeadEmphRe.FindString(t)
	rest := t[len(lead):]
	if m := blkItemRe.FindStringSubmatch(rest); m != nil {
		b.kind, b.marker = blockItem, m[1]
		b.text = blkItemText(lead + rest[len(m[0]):])
		return b
	}
	if m := blkGluedItemRe.FindStringSubmatch(rest); m != nil {
		b.kind, b.marker = blockItem, m[1]
		b.text = blkItemText(lead + rest[len(m[1]):])
		return b
	}
	if m := blkBulletRe.FindString(rest); m != "" {
		b.kind, b.marker = blockBullet, strings.TrimSpace(m)
		b.text = blkItemText(lead + rest[len(m):])
		if label, body, ok := blkSplitLabel(b.text); ok {
			b.kind, b.label, b.text = blockLabelled, label, body
		}
		return b
	}
	text := stripEmphasis(t)
	emph := blkWholeEmphasis(t)
	label, body, labelled := blkSplitLabel(text)
	switch {
	case labelled && blkIsYearPhrase(label) && blkYearHeadingBody(body):
		b.kind, b.text, b.section = blockHeading, text, blkSectionName(label)
		b.level = blkHeadingLevel(blkEmphOf(t, emph), b.section)
	// "Second Year: (3.0 credits)" names a section.
	case labelled && !blkOnlyParens(body) && (!emph || blkEmphLabelled(label, body)):
		b.kind, b.label, b.text = blockLabelled, label, body
	case blkIsHeading(text, emph):
		b.kind, b.text, b.section = blockHeading, text, blkSectionName(text)
		b.level = blkHeadingLevel(blkEmphOf(t, emph), b.section)
	default:
		b.kind, b.text = blockProse, text
	}
	return b
}

// blkYearHeadingBody reports whether the body after a year label only titles
// the year, so the line is the year's heading: "Year 2: Foundations in
// environment and health". "First year: No specific first-year requirements"
// and "Fourth Year: Additional … to a total of 7.0 credits" don't.
func blkYearHeadingBody(body string) bool {
	return !codeRe.MatchString(body) && !blkAmountRe.MatchString(body) && !blkDecimalRe.MatchString(body) &&
		!strings.HasPrefix(body, "No ")
}

// blkSplitLabel splits "Label: body" at the first colon outside brackets.
// The label is at most six words outside brackets, not counting "&" or
// dashes, with no course code anywhere and no amount outside brackets ("First
// Year (3.0 credits)" is a label, "2.0 credits from" isn't), and doesn't end
// in a word that leads into a list ("One of:", "Choose from:"). Footnote
// asterisks are dropped from it ("*NOTE 1").
func blkSplitLabel(s string) (label, body string, ok bool) {
	depth := 0
	for i, r := range s {
		switch r {
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case ':':
			if depth != 0 || i+1 >= len(s) || s[i+1] != ' ' {
				continue
			}
			label, body = strings.Trim(s[:i], " *"), strings.TrimSpace(s[i+1:])
			return label, body, body != "" && blkIsLabel(label)
		}
	}
	return "", "", false
}

func blkIsLabel(s string) bool {
	out := blkOutsideParens(s)
	var words []string
	for _, w := range strings.Fields(out) {
		switch w {
		case "&", "–", "—", "-", "--": // "History & Philosophy of Science & Technology"
		default:
			words = append(words, w)
		}
	}
	if len(words) == 0 || len(words) > 6 || codeRe.MatchString(s) || blkAmountRe.MatchString(out) ||
		blkNumberWordRe.MatchString(s) || strings.Contains(s, ". ") || strings.Count(s, "(") != strings.Count(s, ")") {
		return false
	}
	// "1st year required courses", "200-level physics, math, and earth sciences".
	if r, _ := utf8.DecodeRuneInString(s); !unicode.IsLetter(r) && !unicode.IsDigit(r) {
		return false
	}
	switch strings.ToLower(words[len(words)-1]) {
	case "of", "from", "in", "include", "includes", "including", "following", "are", "is", "be",
		"by", "with", "and", "or", "to", "the", "that", "as", "at", "for":
		return false
	}
	return true
}

// blkIsHeading reports whether a line without a marker is a heading:
//   - a line in emphasis as a whole, with credits after it allowed
//     ("_First year_ (2.5 credits):", "**Group A**");
//   - a short line ending in ":" that isn't a lead-in ("Higher Years:");
//   - a short line naming years, notes, or a group with its credits ("Year 4
//     (0.5 credit)", "N.B.", "Required Courses (8.0 credits)", "Group C").
func blkIsHeading(text string, emph bool) bool {
	if blkStartsAmountRe.MatchString(text) {
		return false
	}
	bare := blkStripCreditParens(text)
	colon := strings.HasSuffix(strings.TrimSpace(bare), ":")
	name := blkSectionName(text)
	out := strings.TrimSpace(blkOutsideParens(name))
	// A code in a credit parenthetical doesn't make the line an item:
	// "_Later years (6.5 credits/7.0 credits for students who have not
	// completed STA130H1 (see 4.))_".
	if name == "" || codeRe.MatchString(bare) {
		return false
	}
	if blkNotesHeadingRe.MatchString(name) {
		return true
	}
	words := len(strings.Fields(out))
	switch {
	case emph:
		return words <= 12 && !strings.HasSuffix(name, ".")
	case colon:
		return words <= 12 && !blkLeadInRe.MatchString(out) && !blkAmountRe.MatchString(out)
	}
	return words <= 8 && blkStartsUpper(name) && !strings.HasSuffix(name, ".") && !strings.HasSuffix(name, ",") &&
		!blkAmountRe.MatchString(out) && (blkIsYearPhrase(name) || bare != text || blkTitleCase(out))
}

// blkWholeEmphasis reports whether a line is emphasised as a whole, apart from a
// parenthetical and a colon after the emphasis.
func blkWholeEmphasis(t string) bool {
	if !strings.HasPrefix(t, "**") && !strings.HasPrefix(t, "_") {
		return false
	}
	s := blkBoldSpanRe.ReplaceAllString(t, " ")
	for {
		u := blkItalicSpanRe.ReplaceAllString(s, "$1 $2")
		if u == s {
			break
		}
		s = u
	}
	return strings.Trim(blkOutsideParens(s), " :.") == ""
}

// blkEmphLabelled reports whether a line in emphasis as a whole splits into a
// label and a body: the body states a course code or an amount outside
// brackets ("**Total credits: 7.0**"; a date isn't one), or the label is a
// note's ("_Note 1: …_"). Otherwise the line names a section ("_Group A:
// Ancient, Medieval_") or is a sentence with a colon in it ("**Admission to
// the Christianity and Culture: Major Program in Religious Education has
// …**").
func blkEmphLabelled(label, body string) bool {
	out := blkOutsideParens(body)
	return codeRe.MatchString(body) || blkAmountRe.MatchString(out) || blkDecimalRe.MatchString(out) ||
		blkNoteLabelRe.MatchString(label)
}

func blkOnlyParens(s string) bool { return strings.Trim(blkOutsideParens(s), " :.,;") == "" }

func blkIsYearPhrase(s string) bool {
	out := strings.TrimSpace(blkOutsideParens(s))
	return blkYearPhraseRe.MatchString(out) && blkHasYearRe.MatchString(out)
}

// blkSectionName is a heading's text without its credits, colon and footnote
// asterisks: "_First year_ (2.5 credits):" is "First year", "First Year*:" is
// "First Year".
func blkSectionName(s string) string {
	s = strings.ReplaceAll(strings.Join(strings.Fields(blkStripCreditParens(s)), " "), " :", ":")
	return strings.Trim(strings.TrimSuffix(s, ":"), " *")
}

// blkStripCreditParens removes parentheticals that mention credits, nested ones
// included: "Later years (6.5 credits/7.0 credits for students who have not
// completed STA130H1 (see 4.))".
func blkStripCreditParens(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '(' {
			if j := blkMatchingParen(s, i); j > 0 && blkCreditRe.MatchString(s[i:j+1]) {
				i = j
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func blkMatchingParen(s string, i int) int {
	depth := 0
	for j := i; j < len(s); j++ {
		switch s[j] {
		case '(':
			depth++
		case ')':
			if depth--; depth == 0 {
				return j
			}
		}
	}
	return -1
}

// blkOutsideParens blanks out bracketed text.
func blkOutsideParens(s string) string {
	for {
		t := blkInnerParenRe.ReplaceAllString(s, " ")
		if t == s {
			return s
		}
		s = t
	}
}

func blkStartsUpper(s string) bool {
	r := []rune(s)
	return len(r) > 0 && unicode.IsUpper(r[0])
}

// blkTitleCase reports whether every word of four letters or more is
// capitalised: "Other Courses", "Health & Disease Centric Courses".
func blkTitleCase(s string) bool {
	n := 0
	for _, w := range strings.Fields(s) {
		letters := []rune(strings.TrimFunc(w, func(r rune) bool { return !unicode.IsLetter(r) }))
		if len(letters) < 4 {
			continue
		}
		if !unicode.IsUpper(letters[0]) {
			return false
		}
		n++
	}
	return n > 0
}

// Heading emphasis, from the highest.
const (
	blkEmphBold = iota
	blkEmphItalic
	blkEmphPlain
)

// blkLevel is the level of a heading in an emphasis style: a name ranks above
// a cluster, subgroup, year or notes heading ("sub") in the same style.
func blkLevel(emph int, sub bool) int {
	if sub {
		return emph*2 + 1
	}
	return emph * 2
}

// blkLevelPlainSub is the level of a plain year heading, which a labelled
// line naming a year acts as.
var blkLevelPlainSub = blkLevel(blkEmphPlain, true)

// blkEmphOf is a line's emphasis style, given whether it is in emphasis as a
// whole.
func blkEmphOf(line string, whole bool) int {
	switch {
	case !whole:
		return blkEmphPlain
	case strings.Contains(line, "**"):
		return blkEmphBold
	}
	return blkEmphItalic
}

// blkHeadingLevel ranks a heading by its emphasis and name; a smaller level
// is higher. So year headings nest under the heading before them in the same
// style (ASMAJ0135 "**Recommended Sequence of Courses:**" holds "First Year:"
// and "Second Year:", ASSPE2737 "Core Biological Physics Courses" holds "Third
// or Fourth Year"), and a notes heading never holds a year or a group. A year
// in the name counts: "To be completed before the end of Second Year" is a
// year heading.
func blkHeadingLevel(emph int, name string) int {
	sub := blkSubHeadingRe.MatchString(name) || blkHasYearRe.MatchString(name) || blkEndsInNotesRe.MatchString(name)
	return blkLevel(emph, sub)
}

// blkSetSections links each block to the heading it sits under and sets its
// section. A heading lasts until a heading of the same or a higher level among
// its siblings, so a heading inside an item doesn't reach past the item, and
// headings nest: PSY "Group 1 - Psychology Courses" holds "Cluster A …" and
// "Cluster B …". A labelled line naming a year is a plain year heading for one
// line.
func blkSetSections(bs []*compBlock, parent *compBlock, outer string) {
	w := &blkSectionWalk{parent: parent, outer: outer, holds: map[*compBlock]int{}}
	var prev *compBlock
	for _, b := range bs {
		switch {
		case b.kind == blockHeading:
			w.openHeading(b, prev)
		case blkStartsSection(b):
			w.closeTo(blkLevelPlainSub)
			b.heading, _ = w.current()
			b.section = blkYearPrefix(b.label)
		default:
			// A labelled list closes the notes before it, though a labelled note
			// or bullet doesn't: ASMAJ0826 "Note:", its items, then "**Group A:**
			// CRI300H1/ …".
			if blkListLine(b) {
				w.closeOpenNotes()
			}
			b.heading, b.section = w.current()
		}
		if b.kind == blockHeading {
			blkSetSections(b.children, b, b.section)
		} else {
			blkSetSections(b.children, b.heading, b.section)
		}
		prev = b
	}
}

// blkListLine reports whether a block is a labelled list that isn't a bullet
// or a note: "**Group A:** CRI300H1/ …".
func blkListLine(b *compBlock) bool {
	return b.kind == blockLabelled && b.marker == "" && !blkNoteLabelRe.MatchString(b.label) && codeRe.MatchString(b.text)
}

// blkSectionWalk tracks the open headings of one sibling list.
type blkSectionWalk struct {
	parent *compBlock // the heading the list sits under
	outer  string     // the list's section
	open   []*compBlock
	// holds maps a heading with nothing under it to the level of the heading
	// right after it, which it holds along with that heading's peers.
	holds map[*compBlock]int
}

// current is the innermost open heading and its section.
func (w *blkSectionWalk) current() (*compBlock, string) {
	if n := len(w.open); n > 0 {
		return w.open[n-1], w.open[n-1].section
	}
	return w.parent, w.outer
}

// closeTo closes the outermost open heading that a heading at level ends,
// and those inside it.
func (w *blkSectionWalk) closeTo(level int) {
	for k, h := range w.open {
		if l, held := w.holds[h]; h.level >= level && !(held && l == level) {
			w.open = w.open[:k]
			return
		}
	}
}

// closeNotes closes what a notes heading at level ends: every heading but a
// higher year heading (ASMAJ1689 "Notes:" under "_First year_ (2.5
// credits):") or a heading over focuses that each have their notes (ASSPE2270
// "**Disciplinary Focuses**"). Notes aren't part of a list (ASMAJ1707 "Notes:"
// after "**Group B (Courses with African Studies Content):**").
func (w *blkSectionWalk) closeNotes(level int) {
	for n := len(w.open); n > 0; n = len(w.open) {
		if h := w.open[n-1]; h.level < level && !h.isNotes() &&
			(blkHasYearRe.MatchString(h.section) || blkFociRe.MatchString(h.section)) {
			return
		}
		w.open = w.open[:n-1]
	}
}

// closeOpenNotes closes the notes headings open innermost.
func (w *blkSectionWalk) closeOpenNotes() {
	for n := len(w.open); n > 0 && w.open[n-1].isNotes(); n = len(w.open) {
		w.open = w.open[:n-1]
	}
}

// openHeading opens heading b, which follows sibling prev, after closing the
// headings it ends.
func (w *blkSectionWalk) openHeading(b, prev *compBlock) {
	switch {
	// A heading with nothing under it holds the heading after it, whatever
	// their styles, and that heading's peers: ASMAJ1231 "_Course Groups_" then
	// "_Medievalism:_", "_History:_"; ASSPE0509 "Recommended Course
	// Combinations:" then "**Students interested in Mineral Exploration:**". Its
	// own peers still close it (ASSPE1868 "First or second year:" holds
	// "Foundational courses (5.0 credits total)" until "Upper years:").
	case prev != nil && len(w.open) > 0 && w.open[len(w.open)-1] == prev && !b.isNotes() && !prev.isNotes():
		w.holds[prev] = b.level
	// The program's notes close every heading: ASMAJ1053 "_Applied Genetics
	// and Biotechnology Major Notes:_".
	case blkProgramNotesRe.MatchString(b.section):
		w.open = w.open[:0]
	case b.isNotes():
		w.closeNotes(b.level)
	default:
		w.closeTo(b.level)
	}
	b.heading, _ = w.current()
	w.open = append(w.open, b)
}

// blkStartsSection reports whether a labelled line names a year: "First Year
// (3.0 credits): …", "1st year required courses (2.0 credits): …", "First
// year: No specific first-year requirements". It is a section of its own and
// closes the plain year and notes headings before it, as the next lines
// needn't belong to that year.
func blkStartsSection(b *compBlock) bool {
	return b.kind == blockLabelled && blkYearPrefix(b.label) != ""
}

// blkYearPrefix is the year phrase a label starts with: "1st year" in "1st
// year required courses (2.0 credits)", "First Year" in "First Year (3.0
// credits)".
func blkYearPrefix(label string) string {
	words := strings.Fields(blkOutsideParens(label))
	for n := len(words); n > 0; n-- {
		if p := strings.Join(words[:n], " "); blkIsYearPhrase(p) {
			return strings.Trim(p, " *")
		}
	}
	return ""
}

// Numbering styles.
const (
	blkStyleNumber = iota + 1
	blkStyleLower
	blkStyleUpper
	blkStyleRoman
)

var blkRomans = []string{"i", "ii", "iii", "iv", "v", "vi", "vii", "viii", "ix", "x",
	"xi", "xii", "xiii", "xiv", "xv", "xvi", "xvii", "xviii", "xix", "xx"}

// blkItemRun is a run of top-level items in one numbering style under one
// heading.
type blkItemRun struct {
	section string
	group   int // counts the headings before the run
	style   int
	items   []*compBlock
	ords    []int
}

// blkLabelItems labels items by their markers. Runs of top-level items whose
// numbers follow on from each other across headings are one numbering
// (ASMAJ1689: 1 under First year, 2 under Second year). A numbering that
// starts at 1 (or a, or i) under a heading is prefixed with the heading when
// its numbers are the heading's own:
//   - it restarts, as an earlier numbering in the style also started at 1
//     ("Notes 1" after the program's items 1–6);
//   - or it is a single run under a heading that names years, as these number
//     their own lines ("Higher Years 2"; ASMAJ1003A cites "lines 1-3 of Higher
//     Years").
//
// So a program's first list under "Required Courses:" keeps its numbers. A
// nested item is prefixed with its parent item's label ("4a", "Higher Years
// 2.1").
func blkLabelItems(top []*compBlock) {
	var runs []*blkItemRun
	group, prevStyle, prevOrd := 0, 0, 0
	for _, b := range top {
		if b.kind == blockHeading || blkStartsSection(b) {
			group++
			continue
		}
		if b.kind != blockItem {
			continue
		}
		b.label = blkMarkerLabel(b.marker)
		style, ord := blkItemOrdinal(b.label, prevStyle, prevOrd)
		prevStyle, prevOrd = style, ord
		var r *blkItemRun
		for j := len(runs) - 1; j >= 0 && runs[j].group == group; j-- {
			if runs[j].style == style {
				r = runs[j]
				break
			}
		}
		if r == nil {
			r = &blkItemRun{section: b.section, group: group, style: style}
			runs = append(runs, r)
		}
		r.items = append(r.items, b)
		r.ords = append(r.ords, ord)
	}
	// Chain the runs into numberings.
	var chains [][]*blkItemRun
	last := map[int]int{} // style -> index of its latest chain
	for _, r := range runs {
		if i, ok := last[r.style]; ok {
			prev := chains[i][len(chains[i])-1]
			if r.ords[0] == prev.ords[len(prev.ords)-1]+1 {
				chains[i] = append(chains[i], r)
				continue
			}
		}
		last[r.style] = len(chains)
		chains = append(chains, []*blkItemRun{r})
	}
	started := map[int]bool{} // styles with a numbering that started at 1
	for _, c := range chains {
		first := c[0]
		if first.ords[0] != 1 {
			continue
		}
		restart := started[first.style]
		started[first.style] = true
		if first.section == "" || !restart && !(len(c) == 1 && blkIsYearPhrase(first.section)) {
			continue
		}
		for _, r := range c {
			for _, b := range r.items {
				b.label = first.section + " " + b.label
			}
		}
	}
	for _, b := range top {
		blkLabelChildren(b)
	}
}

func blkLabelChildren(p *compBlock) {
	for _, b := range p.children {
		if b.kind == blockItem {
			b.label = blkMarkerLabel(b.marker)
			if p.kind == blockItem {
				b.label = blkJoinLabel(p.label, b.label)
			}
		}
		blkLabelChildren(b)
	}
}

// blkMarkerLabel is a marker without its punctuation: "(a)" is "a".
func blkMarkerLabel(marker string) string { return strings.Trim(marker, "().") }

// blkItemOrdinal gives a label's numbering style and position. Roman
// numerals are read in either case ("ii", "III"). A single "i", "v" or "x"
// is a letter or a numeral depending on the item before: "i" after "h" is a
// letter, "v" after "iv" a numeral; "I", "V" and "X" likewise.
func blkItemOrdinal(label string, prevStyle, prevOrd int) (style, ord int) {
	if n, err := strconv.Atoi(label); err == nil {
		return blkStyleNumber, n
	}
	letter := blkStyleLower
	if c := label[0]; c >= 'A' && c <= 'Z' {
		letter = blkStyleUpper
	}
	lower := strings.ToLower(label)
	if i := slices.Index(blkRomans, lower); i >= 0 &&
		(len(label) > 1 || prevStyle == blkStyleRoman || lower == "i" && !(prevStyle == letter && prevOrd == 8)) {
		return blkStyleRoman, i + 1
	}
	return letter, int(lower[0]-'a') + 1
}

// blkJoinLabel appends a nested item's label to its parent's: "4" and "a" give
// "4a", "2" and "1" give "2.1".
func blkJoinLabel(parent, child string) string {
	digit := func(c byte) bool { return c >= '0' && c <= '9' }
	if digit(parent[len(parent)-1]) == digit(child[0]) {
		return parent + "." + child
	}
	return parent + child
}
