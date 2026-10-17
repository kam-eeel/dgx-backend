package grammar

import (
	"regexp"
	"strings"
)

// Misprinted codes repaired against the catalogue: "MAT1581" is MAT158H1.

var (
	// nearCodeRe matches a code missing its H/Y, or its H/Y and campus digit: "MAT1581".
	nearCodeRe = regexp.MustCompile(`\b([A-Z]{3}\d{3})1?\b`)
	// codeFollowRe matches what may follow a code in a list: " or B". In "consult
	// the BIO120 Office" it is a name, not a course.
	codeFollowRe = regexp.MustCompile(`^\s*(?:$|[/,);.:]|(?i:or|and)\b)`)
)

// RepairCatalogCodes repairs near-codes in s that match one course in known
// ("MAT1581" is MAT158H1 if there's no MAT158Y1), if what follows can follow a
// code in a list ("MAT100-series" can't), and reports whether anything changed.
func RepairCatalogCodes(s string, known map[string]bool) (string, bool) {
	return RepairCodesFollowedBy(s, known, codeFollowRe)
}

// RepairCodesFollowedBy is RepairCatalogCodes with its own rule for what may
// follow a near-code.
func RepairCodesFollowedBy(s string, known map[string]bool, follow *regexp.Regexp) (string, bool) {
	var b strings.Builder
	last := 0
	for _, m := range nearCodeRe.FindAllStringIndex(s, -1) {
		if !follow.MatchString(s[m[1]:]) {
			continue
		}
		var hits []string
		for _, w := range []string{"H1", "Y1"} {
			if code := s[m[0]:m[0]+6] + w; known[code] {
				hits = append(hits, code)
			}
		}
		if len(hits) == 1 {
			b.WriteString(s[last:m[0]] + hits[0])
			last = m[1]
		}
	}
	if last == 0 {
		return s, false
	}
	return b.String() + s[last:], true
}
