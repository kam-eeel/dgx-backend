package reqs

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	parenNoteRe   = regexp.MustCompile(`(?i)\(\s*note\b[^()]*\)`)
	recommendedRe = regexp.MustCompile(`(?i)\brecommended\b`)

	contactRe   = regexp.MustCompile(`(?i)encouraged|\bcontact\b|\bconsult\b|\bvisit\b|\bemail\b|may be required|strongly recommended`)
	mandatoryRe = regexp.MustCompile(`(?i)\b(must|only|required)\b`)
	advStartRe  = regexp.MustCompile(`(?i)^(students\b|note\b|please\b|this course\b|it is\b|priority\b|further prerequisites\b)`)
)

// abbreviations whose trailing "." does not end a sentence.
var abbreviations = map[string]bool{"e.g": true, "i.e": true, "dept": true, "st": true, "etc": true, "approx": true, "vs": true}

// extractNotes takes normalized text and separates the parts that are
// requirements from advisory text. The returned requirement text has its
// sentences joined with "; " so sentence boundaries become the lowest-
// precedence AND. notes is nil when there are none.
func extractNotes(s string) (req string, notes []string) {
	for _, m := range parenNoteRe.FindAllString(s, -1) {
		notes = append(notes, cleanNote(m[1:len(m)-1]))
	}
	s = parenNoteRe.ReplaceAllString(s, "")

	var reqs []string
	for _, sentence := range splitSentences(s) {
		if isAdvisory(sentence) {
			notes = append(notes, sentence)
			continue
		}
		var keep []string
		for _, seg := range splitTop(sentence, ';') {
			seg = strings.TrimSpace(seg)
			switch {
			case seg == "":
			case recommendedRe.MatchString(seg):
				notes = append(notes, cleanNote(seg))
			default:
				keep = append(keep, seg)
			}
		}
		if len(keep) > 0 {
			reqs = append(reqs, strings.Join(keep, "; "))
		}
	}
	return strings.Join(reqs, "; "), notes
}

// isAdvisory reports whether a sentence is advice rather than a requirement.
func isAdvisory(sentence string) bool {
	if contactRe.MatchString(sentence) {
		return true // "Students who do not meet … are encouraged to contact …"
	}
	if mandatoryRe.MatchString(sentence) {
		return false // "Open only to students enrolled in …"
	}
	return advStartRe.MatchString(sentence)
}

// splitSentences splits on ". " followed by an uppercase letter, outside
// brackets, skipping known abbreviations. Each sentence is trimmed and loses
// one trailing ".".
func splitSentences(s string) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth = max(0, depth-1)
		case '.':
			if depth > 0 || i+2 >= len(s) || s[i+1] != ' ' || !unicode.IsUpper(rune(s[i+2])) {
				continue
			}
			word := s[strings.LastIndex(s[:i], " ")+1 : i]
			if abbreviations[strings.ToLower(word)] {
				continue
			}
			out = append(out, cleanNote(s[start:i]))
			start = i + 1
		}
	}
	if last := cleanNote(s[start:]); last != "" {
		out = append(out, last)
	}
	return out
}

// splitTop splits s on sep, ignoring separators inside brackets.
func splitTop(s string, sep byte) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth = max(0, depth-1)
		case sep:
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}

// cleanNote collapses whitespace, trims, and drops one trailing ".".
func cleanNote(s string) string {
	s = strings.TrimSpace(wsRe.ReplaceAllString(s, " "))
	return strings.TrimSpace(strings.TrimSuffix(s, "."))
}
