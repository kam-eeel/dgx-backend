package reqs

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	parenNoteRe   = regexp.MustCompile(`(?i)\(\s*note\b(?:[^()]|\([^()]*\))*\)`) // may hold one level of brackets
	recommendedRe = regexp.MustCompile(`(?i)\brecommended\b`)
	// bareRecRe matches bracket content that only says "recommended" about the code
	// before it.
	bareRecRe = regexp.MustCompile(`(?i)^\s*recommended\s*$`)
	// missingStopRe matches a sentence starting right after a code with its "."
	// missing: "BCH377H1 Students in the BCH Majors need to email ...".
	missingStopRe = regexp.MustCompile(`(` + codePat + `)\s+((?:Students|Please)\b)`)

	// noteBreakRe matches a "NOTE:" that starts mid-sentence, so it can become a
	// sentence of its own.
	noteBreakRe = regexp.MustCompile(`[\s,;]*\s\*?(NOTES?|Notes?):`)

	contactRe   = regexp.MustCompile(`(?i)encouraged|\bcontact\b|\bconsult\b|\bvisit\b|\bemail\b|may be required|strongly recommended|can be combined|not accepted|do(?:es)? not provide|not be taken concurrently|\bwaiver\b`)
	mandatoryRe = regexp.MustCompile(`(?i)\b(must|only|required|cannot)\b`)
	noteStartRe = regexp.MustCompile(`(?i)^notes?\s*:`)
	advStartRe  = regexp.MustCompile(`(?i)^(students\b|notes?\b|please\b|this course\b|it is\b|however\b|priority\b|further prerequisites\b)`)
)

// abbreviations whose trailing "." does not end a sentence.
var abbreviations = map[string]bool{"e.g": true, "i.e": true, "dept": true, "st": true, "etc": true, "approx": true, "vs": true}

// extractNotes splits normalized text into requirements and advisory notes (nil
// if none). Requirement sentences are joined with "; ", the lowest-precedence
// AND.
func extractNotes(s string) (req string, notes []string) {
	for _, m := range parenNoteRe.FindAllString(s, -1) {
		notes = append(notes, cleanNote(m[1:len(m)-1]))
	}
	s = parenNoteRe.ReplaceAllString(s, "")
	s = noteBreakRe.ReplaceAllString(s, ". $1:")
	s = missingStopRe.ReplaceAllString(s, "$1. $2")
	s = advisoryParens(s, &notes)

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
			case isPointer(seg):
				notes = append(notes, cleanNote(seg)) // "see the website for details"
			default:
				keep = append(keep, seg)
			}
		}
		if len(keep) > 0 {
			reqs = append(reqs, strings.Join(keep, "; "))
		}
	}
	if len(reqs) == 0 {
		// A field that only points elsewhere ("See the Department website
		// for details") still has requirements; they just aren't listed, so
		// keep the pointer as the requirement (it becomes Review).
		var rest []string
		for _, n := range notes {
			if isPointer(n) && !noneRe.MatchString(n) {
				reqs = append(reqs, n)
			} else {
				rest = append(rest, n)
			}
		}
		notes = rest
	}
	return strings.Join(reqs, "; "), notes
}

// advisoryParens moves advice in brackets to notes, so it can't make the whole
// sentence advisory. A bare "(recommended)" on an alternative is dropped; on a
// course standing alone the course itself goes to notes with it.
func advisoryParens(s string, notes *[]string) string {
	var b strings.Builder
	last := 0
	for _, g := range topParens(s) {
		inner := s[g[0]+1 : g[1]-1]
		if !contactRe.MatchString(inner) && !recommendedRe.MatchString(inner) {
			continue
		}
		before := strings.TrimRight(s[last:g[0]], " ")
		if bareRecRe.MatchString(inner) {
			code := codeRe.FindAllString(before, -1)
			if code == nil || !strings.HasSuffix(before, code[len(code)-1]) {
				continue
			}
			c := code[len(code)-1]
			*notes = append(*notes, c+" recommended")
			after := strings.TrimLeft(s[g[1]:], " ")
			prefix := strings.TrimRight(strings.TrimSuffix(before, c), " ")
			if !alternativeRe.MatchString(after) && !altBeforeRe.MatchString(prefix) {
				before = prefix // the course is only recommended
			}
			b.WriteString(before)
			last = g[1]
			continue
		}
		var keep []string
		for _, piece := range splitPieces(inner) {
			if contactRe.MatchString(piece.text) || recommendedRe.MatchString(piece.text) {
				*notes = append(*notes, cleanNote(piece.text))
			} else if len(keep) == 0 {
				keep = append(keep, strings.TrimSpace(piece.text))
			} else {
				keep = append(keep, piece.sep+" "+strings.TrimSpace(piece.text))
			}
		}
		b.WriteString(before)
		if len(keep) > 0 {
			b.WriteString(" (" + strings.Join(keep, "") + ")")
		}
		last = g[1]
	}
	b.WriteString(s[last:])
	return b.String()
}

var (
	alternativeRe = regexp.MustCompile(`(?i)^(?:/|or\b)`)
	altBeforeRe   = regexp.MustCompile(`(?i)(?:/|\bor)$`)
)

// topParens returns the [start, end) spans of the top-level bracket groups
// in s that are closed.
func topParens(s string) [][2]int {
	var out [][2]int
	depth, open := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			if depth == 0 {
				open = i
			}
			depth++
		case ')':
			if depth > 0 {
				depth--
				if depth == 0 {
					out = append(out, [2]int{open, i + 1})
				}
			}
		}
	}
	return out
}

type piece struct{ sep, text string }

// splitPieces splits bracket content on top-level ";" and ",", keeping the
// separator before each piece.
func splitPieces(s string) []piece {
	var out []piece
	depth, start, sep := 0, 0, ""
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth = max(0, depth-1)
		case ';', ',':
			if depth == 0 {
				out = append(out, piece{sep, s[start:i]})
				start, sep = i+1, s[i:i+1]
			}
		}
	}
	return append(out, piece{sep, s[start:]})
}

// isAdvisory reports whether a sentence is advice rather than a requirement.
func isAdvisory(sentence string) bool {
	if contactRe.MatchString(sentence) || noteStartRe.MatchString(sentence) {
		return true // "Students who do not meet ... are encouraged to contact ...", "Notes: ..."
	}
	if mandatoryRe.MatchString(sentence) {
		return false // "Open only to students enrolled in ..."
	}
	return advStartRe.MatchString(sentence)
}

// splitSentences splits on ". " followed by an uppercase letter, outside
// closed brackets, skipping known abbreviations. Each sentence is trimmed and loses
// one trailing ".".
func splitSentences(s string) []string {
	var out []string
	closed := map[int]bool{} // "(" that have a matching ")"; an unclosed one doesn't hide later sentences
	for _, g := range topParens(s) {
		closed[g[0]] = true
	}
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			if depth > 0 || closed[i] {
				depth++
			}
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
