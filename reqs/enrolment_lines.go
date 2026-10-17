package reqs

import (
	"regexp"
	"strings"
)

// Enrolment text lines, one block per line: bullets, numbering, nested lists
// indented two spaces per level, and headings, emphasised or not.

type lineKind int

const (
	lineText    lineKind = iota // a paragraph
	lineItem                    // a bullet or numbered item
	lineHeading                 // a heading
)

type enrolLine struct {
	kind   lineKind
	depth  int    // nesting level: two spaces of indent per level
	marker string // items: "-" for bullets, "#" for numbers and letters
	text   string // emphasis markers removed, trimmed
}

var (
	bulletRe = regexp.MustCompile(`^_?[-•]\s+`)                 // "- ", "• ", "_• " (a bullet glued to italics)
	numberRe = regexp.MustCompile(`^(?:\d{1,2}|[a-zA-Z])\.\s+`) // "1. ", "a. ", "A. "
	// wholeEmphRe matches a heading: a whole line in one emphasis, with an
	// optional colon: "**Notes**:".
	wholeEmphRe = regexp.MustCompile(`^(?:\*\*|_)+.*(?:\*\*|_)+\s*:?$`)
	// plainHeadingRe matches a heading that may come without emphasis: "Note:".
	plainHeadingRe = regexp.MustCompile(`^(?i:(?:variable\s+)?minimum\s+grade(?:\s+average)?|notes?|special\s+requirements?|completed\s+courses.*|how\s+to\s+apply)\s*:?$|` +
		`^For\s+students\b.{0,160}:$|^Option\s+\d+\s*:$`)
	boldRe = regexp.MustCompile(`\*\*`)
	// italicRe matches italic markers, next to spaces or punctuation; an "_"
	// inside a word is kept.
	italicRe = regexp.MustCompile(`(^|[\s(\[/,;:.*])_+|_+($|[\s)\]/,;:.*])`)
	// lineEndRe matches a sentence end; a line without one may continue on the
	// next.
	lineEndRe = regexp.MustCompile(`[.:;!?)]\s*$`)
)

// enrolLines splits raw enrolment text into classified lines, joining a
// paragraph broken mid-sentence.
func enrolLines(raw string) []enrolLine {
	var out []enrolLine
	for _, rawLine := range strings.Split(raw, "\n") {
		if strings.TrimSpace(rawLine) == "" {
			continue
		}
		l := classifyLine(rawLine)
		if n := len(out); n > 0 && l.kind == lineText && out[n-1].kind == lineText &&
			!lineEndRe.MatchString(out[n-1].text) && startsLower(l.text) {
			out[n-1].text += " " + l.text
			continue
		}
		out = append(out, l)
	}
	return out
}

func classifyLine(raw string) enrolLine {
	t := strings.TrimRight(raw, " ")
	l := enrolLine{depth: (len(t) - len(strings.TrimLeft(t, " "))) / 2}
	t = strings.TrimSpace(t)
	switch {
	case bulletRe.MatchString(t):
		l.kind, l.marker = lineItem, "-"
		t = bulletRe.ReplaceAllString(t, "")
	case numberRe.MatchString(t):
		l.kind, l.marker = lineItem, "#"
		t = numberRe.ReplaceAllString(t, "")
	}
	emph := wholeEmphRe.MatchString(t)
	l.text = stripEmphasis(t)
	if l.kind == lineText && l.text != "" &&
		(emph && len(l.text) <= 150 && !strings.HasSuffix(l.text, ".") || plainHeadingRe.MatchString(l.text)) {
		l.kind = lineHeading
	}
	return l
}

// stripEmphasis removes **bold** and _italic_ markers.
func stripEmphasis(s string) string {
	s = boldRe.ReplaceAllString(s, "")
	for {
		t := italicRe.ReplaceAllString(s, "$1$2")
		if t == s {
			break
		}
		s = t
	}
	return strings.TrimSpace(wsRe.ReplaceAllString(s, " "))
}

func startsLower(s string) bool { return s != "" && s[0] >= 'a' && s[0] <= 'z' }
