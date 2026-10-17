package programs

import (
	"regexp"
	"strings"

	"degree-explorer/parser/grammar"
)

// Calendar text helpers shared by enrolment and completion: list markers,
// emphasis and the ASIP stream.

var (
	BulletRe = regexp.MustCompile(`^_?[-•]\s+`)                 // "- ", "• ", "_• " (a bullet glued to italics)
	MarkerRe = regexp.MustCompile(`^(?:\d{1,2}|[a-zA-Z])\.\s+`) // "1. ", "a. ", "A. "
	boldRe   = regexp.MustCompile(`\*\*`)
	// italicRe matches italic markers, next to spaces or punctuation; an "_"
	// inside a word is kept.
	italicRe = regexp.MustCompile(`(^|[\s(\[/,;:.*])_+|_+($|[\s)\]/,;:.*])`)
)

// StripEmphasis removes **bold** and _italic_ markers.
func StripEmphasis(s string) string {
	s = boldRe.ReplaceAllString(s, "")
	for {
		t := italicRe.ReplaceAllString(s, "$1$2")
		if t == s {
			break
		}
		s = t
	}
	return strings.TrimSpace(grammar.WsRe.ReplaceAllString(s, " "))
}

// AsipRe matches the Arts & Science Internship Program stream: "ASIP".
var AsipRe = regexp.MustCompile(`(?i)Arts\s*&\s*Science\s+Internship\s+Program|\bASIP\b`)

func MatchAny(res []*regexp.Regexp, s string) bool {
	for _, re := range res {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}
