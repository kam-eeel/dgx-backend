package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The block tree of completion text: one block per line, nested by indent,
// with "- " bullets, markers as written ("1)", "(a)", "i)") and emphasis.

type blockKind int

const (
	blockProse    blockKind = iota // a paragraph
	blockHeading                   // "First Year:", "**Group A**", "Notes:"
	blockItem                      // a numbered or lettered item: "1.", "(a)"
	blockBullet                    // "- "
	blockLabelled                  // "**Group A:** CSC209H1; …", "Note: …"
)

// isNotes reports whether a heading's name ends in notes: "Important Notes:".
func (b *compBlock) isNotes() bool { return blkEndsInNotesRe.MatchString(b.section) }

type compBlock struct {
	kind   blockKind
	marker string // items and bullets: the marker as written, "(a)", "-"
	// Items: the calendar's label, "Higher Years 2". Labelled lines: the text
	// before the colon.
	label  string
	text   string // the line without indent, marker, label and emphasis
	indent int    // leading spaces
	line   int    // index of the line in the raw text
	// The innermost heading's name, without credits or footnote asterisk. A
	// heading's section is its name; a labelled line naming a year is its own.
	section string
	// The innermost heading the block sits under (for a heading, the one it
	// sits under); nil at the top.
	heading *compBlock
	// Headings: the rank deciding which heading ends which; smaller is higher.
	level    int
	children []*compBlock // the indented lines below, and lists adopted by their lead-in
	// the raw lines a hard-wrapped paragraph continues on
	cont []int
}

var (
	// blkLeadEmphRe matches emphasis opened before an item's marker: "**1.
	// Students …**".
	blkLeadEmphRe = regexp.MustCompile(`^(?:\*\*|_)+`)
	// blkItemRe matches an item marker: "ii) ".
	blkItemRe     = regexp.MustCompile(`^(\d{1,2}[.)]|\(\d{1,2}\)|[a-zA-Z][.)]|\([a-zA-Z]\)|(?:[ivx]{2,4}|[IVX]{2,4})[.)]|\((?:[ivx]{2,4}|[IVX]{2,4})\))(?:\*\*|_)*\s+`)
	blkBulletRe   = regexp.MustCompile(`^[-•]\s+`)
	blkBoldSpanRe = regexp.MustCompile(`\*\*.*?\*\*`)
	// blkItalicSpanRe matches an italic span, its underscores next to a space
	// or punctuation: "_First year_ (2.5 credits)".
	blkItalicSpanRe = regexp.MustCompile(`(^|[\s(\[/,;:.*])_.*?_($|[\s)\]/,;:.*])`)
	// blkInnerParenRe matches a bracketed span with none inside it, so
	// replacing goes from the inside out: "(3.0 credits)".
	blkInnerParenRe = regexp.MustCompile(`\([^()]*\)|\[[^\[\]]*\]`)
	blkCreditRe     = regexp.MustCompile(`(?i)\bcredits?\b`)
	blkAmountRe     = regexp.MustCompile(`(?i)\d+(?:\.\d+)?\s*(?:credits?|fces?)\b`)
	blkDecimalRe    = regexp.MustCompile(`\d\.\d`) // "7.0", ".50" doesn't count
	// blkStartsAmountRe matches a line starting with an amount: "(8.0 credits)".
	blkStartsAmountRe = regexp.MustCompile(`^\(?\s*\d`)
	// blkLeadInRe matches words that make a line ending in ":" a lead-in, not a
	// heading: "Of these:".
	blkLeadInRe = regexp.MustCompile(`(?i)\b(?:following|follows|below|above|include|includes|including|must|should|may|requires?|choose|chosen|selected|of these|one of|plus|suggest|complete)\b`)
	// blkNoteLabelRe matches a note's label: "Note 1".
	blkNoteLabelRe = regexp.MustCompile(`(?i)^\*?notes?(?:\s+\d+)?$`)
	// blkNotesHeadingRe matches a heading that is only notes: "Notes".
	blkNotesHeadingRe = regexp.MustCompile(`(?i)^\*?(?:(?:important|advising)\s+)?(?:notes?|n\.\s?b\.?)$`)
	// blkYearPhraseRe matches a phrase naming years: "Third or Fourth Year".
	blkYearPhraseRe = regexp.MustCompile(`(?i)^(?:(?:first|second|third|fourth|fifth|1st|2nd|3rd|4th|higher|later|upper|lower|early|senior|subsequent|any|in|and|or|years?|\d)(?:[\s,/&*-]+|$))+$`)
	blkHasYearRe    = regexp.MustCompile(`(?i)\byears?\b`)
	// blkGluedItemRe matches a marker glued to an amount: "1.1.0 credit".
	blkGluedItemRe = regexp.MustCompile(`^(\d{1,2}\.)\d+\.\d`)
	// blkNumberWordRe matches a label starting with a number: "4.5 of".
	blkNumberWordRe = regexp.MustCompile(`^\d+(?:\.\d+)?(?:\s|$)`)
	// blkSubHeadingRe matches a heading ranked below a group: "Cluster A".
	blkSubHeadingRe = regexp.MustCompile(`(?i)^\(?(?:cluster|sub-?group)\b`)
	// blkAdviceRe matches a bullet of advice, which ends a list: "Please note
	// that UTM and UTSC courses …".
	blkAdviceRe = regexp.MustCompile(`(?i)^(?:it is (?:highly |strongly )?recommended|please note|note that|students are (?:encouraged|advised))\b`)
	// blkFociRe matches a heading over focuses: "**Disciplinary Focuses**".
	blkFociRe = regexp.MustCompile(`(?i)\bfocuses\b|\bfoci\b`)
	// blkProgramNotesRe matches the program's notes: "Applied Genetics and
	// Biotechnology Major Notes".
	blkProgramNotesRe = regexp.MustCompile(`(?i)\b(?:major|minor|specialists?|program)\s+notes?$`)
	// blkEndsInNotesRe matches a heading name ending in notes: "… Major Notes".
	blkEndsInNotesRe = regexp.MustCompile(`(?i)(?:^|\s)(?:notes?|n\.\s?b\.?)$`)
	// blkListWordRe matches words that make an item with no colon lead into the
	// bullets after it: "0.5 credit from each of the three Thematic Clusters."
	blkListWordRe = regexp.MustCompile(`(?i)\b(?:following|below|each of the|clusters?)\b`)
)

// completionBlocks reads completion text into a tree of blocks, one per line;
// an indented line hangs under the line above. Headings set sections, and items
// get the calendar's own label: "4", "Higher Years 2", "4a".
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

// blkAdoptSubItems hangs flush-left lists under the line leading into them:
// bullets after a lead-in or a numbered item, a lettered list after either.
// A heading leads into nothing: its section covers the lines after it.
func blkAdoptSubItems(top []*compBlock) []*compBlock {
	var out []*compBlock
	for i := 0; i < len(top); i++ {
		p := top[i]
		out = append(out, p)
		// a bullet with its own list indented under it leads into nothing: the
		// flush bullets after it are its peers
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

// blkLeadsIntoBullets reports whether a numbered item not ending in ":" owns
// the bullets after it: it ends mid-sentence or points at them. Permissive on
// purpose: such bullets are nearly always the item's; blkAdviceRe ends them.
func blkLeadsIntoBullets(p *compBlock) bool {
	return blkNumbered(p) && (!strings.HasSuffix(p.text, ".") || blkListWordRe.MatchString(p.text))
}

// blkAdoptItemBodies hangs the flush-left lines after a numbered item ending in
// ":" under it, up to a line that ends its body (blkEndsItemBody). A lettered
// item in the body takes the prose after it.
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
// numbered item, a year line, a footnote, or prose with no code or amount that
// isn't a lead-in, unless c comes first and p has no children yet.
func blkEndsItemBody(p, c *compBlock, first bool) bool {
	switch {
	case c.kind == blockHeading || blkNumbered(c) || blkStartsSection(c) || strings.HasPrefix(c.text, "*"):
		return true
	case c.kind != blockProse || first && len(p.children) == 0:
		return false
	}
	return !codeRe.MatchString(c.text) && !blkStartsAmountRe.MatchString(c.text) && !strings.HasSuffix(c.text, ":")
}

// blkLetteredBodies hangs the prose and bullets after a lettered item under it.
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

// blkWrapEndRe matches a line that ends mid-sentence, with no full stop, colon
// or semicolon: "before being admitted".
var blkWrapEndRe = regexp.MustCompile(`[^.:;!?)]\s*$|,\s*$`)

// blkWrapStartRe matches a line that continues the sentence above: "to the
// Minor".
var blkWrapStartRe = regexp.MustCompile(`^(?:[a-z]|` + codePat + `)`)

// blkWrapWordEndRe and blkWrapWordStartRe match a sentence broken between two
// lowercase words, a wrap even after a line of codes: "admitted" then "to".
var (
	blkWrapWordEndRe   = regexp.MustCompile(`\b[a-z]{2,}\s*$`)
	blkWrapWordStartRe = regexp.MustCompile(`^[a-z]{2,}\b`)
)

// blkJoinWraps joins a hard-wrapped paragraph into one block: prose (or a note)
// ending mid-sentence, then prose at the same indent that starts lowercase or
// with a code. Lines of codes are steps of a list, not a wrapped sentence.
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

// blkItemText is an item's or bullet's text without emphasis. An italic note
// after a code becomes a sentence of its own, so it can't swallow the item.
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
	// a body of only brackets is a heading's: "Second Year: (3.0 credits)"
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
// the year, making the line its heading: "Year 2: Foundations in environment
// and health". "First year: No specific first-year requirements" doesn't.
func blkYearHeadingBody(body string) bool {
	return !codeRe.MatchString(body) && !blkAmountRe.MatchString(body) && !blkDecimalRe.MatchString(body) &&
		!strings.HasPrefix(body, "No ")
}

// blkSplitLabel splits "Label: body" at the first colon outside brackets, when
// the label passes blkIsLabel. Footnote asterisks are dropped from the label.
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
	// a label starts with a letter or a digit: "1st year required courses"
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

// blkIsHeading reports whether a line without a marker is a heading: in
// emphasis as a whole, short and ending in ":" with no lead-in word, or short
// and in title case, naming years or with its credits: "Year 4 (0.5 credit)".
func blkIsHeading(text string, emph bool) bool {
	if blkStartsAmountRe.MatchString(text) {
		return false
	}
	bare := blkStripCreditParens(text)
	colon := strings.HasSuffix(strings.TrimSpace(bare), ":")
	name := blkSectionName(text)
	out := strings.TrimSpace(blkOutsideParens(name))
	// a code in a credit parenthetical doesn't make the line an item: "_Later
	// years (6.5 credits/7.0 credits for students who have not ... STA130H1)_"
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

// blkWholeEmphasis reports whether a line is emphasised as a whole, apart from
// a parenthetical and a colon after the emphasis.
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
// label and a body: the body has a code or an amount outside brackets, or the
// label is a note's. Otherwise the line names a section: "_Group A: Ancient_".
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
// included.
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

// blkHeadingLevel ranks a heading by its emphasis and name; smaller is higher.
// Year headings (any name with a year) nest under the heading before them in
// the same style, and a notes heading never holds a year or a group.
func blkHeadingLevel(emph int, name string) int {
	sub := blkSubHeadingRe.MatchString(name) || blkHasYearRe.MatchString(name) || blkEndsInNotesRe.MatchString(name)
	return blkLevel(emph, sub)
}

// blkSetSections links each block to the heading it sits under and sets its
// section. A heading lasts until a heading of the same or a higher level among
// its siblings, so one inside an item doesn't reach past the item.
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
			// a labelled list closes the notes before it; a labelled note or
			// bullet doesn't
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
// higher year heading or one over focuses that each have their notes.
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
	// a heading with nothing under it holds the next heading, whatever their
	// styles, and that heading's peers; its own peers still close it
	case prev != nil && len(w.open) > 0 && w.open[len(w.open)-1] == prev && !b.isNotes() && !prev.isNotes():
		w.holds[prev] = b.level
	// the program's notes close every heading
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
// (3.0 credits): …". It is a section of its own and closes the plain year and
// notes headings before it, as the next lines needn't belong to that year.
func blkStartsSection(b *compBlock) bool {
	return b.kind == blockLabelled && blkYearPrefix(b.label) != ""
}

// blkYearPrefix is the year phrase a label starts with: "1st year" in "1st
// year required courses (2.0 credits)".
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

// blkLabelItems labels items by their markers; runs whose numbers follow on
// across headings are one numbering. A numbering from 1 that restarts, or is
// one run under a year heading, is prefixed with the heading: "Higher Years 2".
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

// blkItemOrdinal gives a label's numbering style and position. A lone "i" is a
// numeral unless it follows "h"; a lone "v" or "x" is one only after a numeral.
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
