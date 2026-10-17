package enrolment

import (
	"regexp"
	"strings"

	"degree-explorer/parser/programs"
)

// Enrolment text lines, one block per line: bullets, numbering, nested lists
// indented two spaces per level, and headings, emphasised or not.

type lineKind int

const (
	lineText    lineKind = iota // a paragraph
	lineItem                    // a bullet or numbered item
	lineHeading                 // a heading
)

type line struct {
	kind   lineKind
	depth  int    // nesting level: two spaces of indent per level
	marker string // items: "-" for bullets, "#" for numbers and letters
	text   string // emphasis markers removed, trimmed
}

var (
	// wholeEmphRe matches a heading: a whole line in one emphasis, with an
	// optional colon: "**Notes**:".
	wholeEmphRe = regexp.MustCompile(`^(?:\*\*|_)+.*(?:\*\*|_)+\s*:?$`)
	// plainHeadingRe matches a heading that may come without emphasis: "Note:".
	plainHeadingRe = regexp.MustCompile(`^(?i:(?:variable\s+)?minimum\s+grade(?:\s+average)?|notes?|special\s+requirements?|completed\s+courses.*|how\s+to\s+apply)\s*:?$|` +
		`^For\s+students\b.{0,160}:$|^Option\s+\d+\s*:$`)
	// lineEndRe matches a sentence end; a line without one may continue on the
	// next.
	lineEndRe = regexp.MustCompile(`[.:;!?)]\s*$`)
)

// lines splits raw enrolment text into classified lines, joining a
// paragraph broken mid-sentence.
func lines(raw string) []line {
	var out []line
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

func classifyLine(raw string) line {
	t := strings.TrimRight(raw, " ")
	l := line{depth: (len(t) - len(strings.TrimLeft(t, " "))) / 2}
	t = strings.TrimSpace(t)
	switch {
	case programs.BulletRe.MatchString(t):
		l.kind, l.marker = lineItem, "-"
		t = programs.BulletRe.ReplaceAllString(t, "")
	case programs.MarkerRe.MatchString(t):
		l.kind, l.marker = lineItem, "#"
		t = programs.MarkerRe.ReplaceAllString(t, "")
	}
	emph := wholeEmphRe.MatchString(t)
	l.text = programs.StripEmphasis(t)
	if l.kind == lineText && l.text != "" &&
		(emph && len(l.text) <= 150 && !strings.HasSuffix(l.text, ".") || plainHeadingRe.MatchString(l.text)) {
		l.kind = lineHeading
	}
	return l
}

func startsLower(s string) bool { return s != "" && s[0] >= 'a' && s[0] <= 'z' }
