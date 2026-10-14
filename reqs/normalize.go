// Package reqs parses course prerequisite, corequisite and exclusion text into
// structured requirement trees, and evaluates them against a student record.
package reqs

import (
	"regexp"
	"strings"
)

// codePat matches a course code: CSC148H1, MATA22H3, MAT223H5.
const codePat = `[A-Z]{3}[A-Z0-9]\d{2}[HY]\d`

var (
	codeRe = regexp.MustCompile(codePat)

	charReplacer = strings.NewReplacer(
		"\u200b", " ", "\u00a0", " ",
		"[", "(", "]", ")",
		"and/or", "/",
	)
	urlRe         = regexp.MustCompile(`https?://\S+`)
	spacedCode    = regexp.MustCompile(`\b([A-Z]{3}) ([A-D0-9]\d{2}[HY]\d)\b`)      // POL 478H1
	footnoteRe    = regexp.MustCompile(`(` + codePat + `)\*+`)                      // GGR270H1*
	leadingOpRe   = regexp.MustCompile(`\(\s*(or|including)\s+`)                    // "X (or Y)" → "X or (Y)"
	completionRe  = regexp.MustCompile(`(?i)\bcompletion\s+of\s+(` + codePat + `)`) // "Completion of SOC201H1"
	engineeringRe = regexp.MustCompile(`(?i)\bPrerequisites?\s+for\s+(?:the\s+)?(?:Faculty\s+of\s+)?Applied\s+Science\s+and\s+Engineering\s+students:`)
	wsRe          = regexp.MustCompile(`\s+`)
)

// normalize cleans up raw catalogue text so the lexer only has to deal with one
// spelling of everything. URLs are removed and returned separately: their "/"
// must never be read as OR.
func normalize(s string) (out string, urls []string) {
	urls = urlRe.FindAllString(s, -1)
	s = urlRe.ReplaceAllString(s, "")
	s = charReplacer.Replace(s)
	s = spacedCode.ReplaceAllString(s, "$1$2")
	s = footnoteRe.ReplaceAllString(s, "$1")
	s = leadingOpRe.ReplaceAllString(s, " $1 (")
	s = completionRe.ReplaceAllString(s, "$1")
	s = engineeringRe.ReplaceAllString(s, "; Engineering students:")
	return strings.TrimSpace(wsRe.ReplaceAllString(s, " ")), urls
}
