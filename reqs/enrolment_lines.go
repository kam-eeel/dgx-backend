package reqs

import (
	"regexp"
	"strings"
)

// The scraper stores enrolment text one block per line (scraper/blocks.go):
// "- " and "• " bullets, "1." / "a." / "A." numbering, nested lists indented
// two spaces per level, **bold** and _italic_. A line emphasised as a whole
// is a heading: "_Variable Minimum Grade_", "**For students who have
// completed 9.0 or more credits:**". A few headings come without emphasis
// ("Variable Minimum Grade", "Note:", "For students who have completed 9.0 or
// more credits:", "Option 1:").

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
	// A whole line in one emphasis, with an optional colon after it:
	// "_Completed courses (with minimum grades)_", "**Notes**:".
	wholeEmphRe = regexp.MustCompile(`^(?:\*\*|_)+.*(?:\*\*|_)+\s*:?$`)
	// Headings that may come without emphasis.
	plainHeadingRe = regexp.MustCompile(`^(?i:(?:variable\s+)?minimum\s+grade(?:\s+average)?|notes?|special\s+requirements?|completed\s+courses.*|how\s+to\s+apply)\s*:?$|` +
		`^For\s+students\b.{0,160}:$|^Option\s+\d+\s*:$`)
	boldRe = regexp.MustCompile(`\*\*`)
	// Italic markers sit next to spaces or punctuation; an "_" inside a word
	// is kept.
	italicRe = regexp.MustCompile(`(^|[\s(\[/,;:.*])_+|_+($|[\s)\]/,;:.*])`)
	// A sentence end, so a line that doesn't end with one may continue on the
	// next line.
	lineEndRe = regexp.MustCompile(`[.:;!?)]\s*$`)
)

// enrolLines splits raw enrolment text into classified lines. A paragraph
// broken across lines mid-sentence ("... may change depending on" /
// "available spaces ...") is joined again.
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
