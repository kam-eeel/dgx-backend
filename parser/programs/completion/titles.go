package completion

import (
	"regexp"
	"strings"

	"degree-explorer/parser/grammar"
)

// Course titles and labels inside course lists: they name the course beside
// them and change nothing a pool requires, so they are dropped.

var (
	// titleCaseRe matches words in title case, small words between them:
	// "Introduction to Physical Geography".
	titleCaseRe = regexp.MustCompile(`^[A-Z][a-z][\w'’-]*(?:,?\s+(?:[A-Z][a-z][\w'’-]*|[A-Z]|of|and|the|in|to|for|on|at|a|an|&|from|with|by|de|du|des|la|le|et)|:\s+[A-Z][a-z][\w'’-]*)*$`)
	// titleRequirementRe matches words that say something about the
	// requirement, never a course's title: "Recommended".
	titleRequirementRe = regexp.MustCompile(`(?i)\b(?:or|and/or|if|unless|only|plus|any|taught|approved|approval|permission|consent|equivalents?|required|requirements?|recommended|note|notes|see|formerly|not|no|must|may|can|should|exclud\w*|includ\w*|except|minimum|maximum|grade|credits?|courses?|level|series|students?|prerequisites?|exclusions?|offered|program|programs|group|list|category|one|two|three|either|both|each)\b`)
	// bareTitleRe matches a code and the words after it, up to the list's
	// next separator: "RLG202H1 Judaism/".
	bareTitleRe = regexp.MustCompile(`(` + grammar.CodePat + `)[ \t]+([A-Z][^/,;()\[\]\n]*?)\s*(?:[/,;)\]]|\.?\s*$)`)
)

// isCourseTitle reports whether s is a course's title.
func isCourseTitle(s string) bool {
	s = strings.TrimSpace(s)
	return titleCaseRe.MatchString(s) && !titleRequirementRe.MatchString(s)
}

// stripBareTitles drops the titles after the codes in a course list:
// "RLG201H1 Indigenous Spiritualities and Religions/ RLG202H1 Judaism". It
// needs two or more: one capitalised phrase after a code may be prose.
func stripBareTitles(s string) string {
	ms := bareTitleRe.FindAllStringSubmatchIndex(s, -1)
	var titled [][]int
	for _, m := range ms {
		if isCourseTitle(s[m[4]:m[5]]) {
			titled = append(titled, m)
		}
	}
	if len(titled) < 2 {
		return s
	}
	var b strings.Builder
	last := 0
	for _, m := range titled {
		b.WriteString(s[last:m[3]])
		last = m[5]
	}
	b.WriteString(s[last:])
	return b.String()
}

var (
	// categoryLabelRe matches a lowercase label of a group of courses
	// before its first code: "; seminar EEB491H1".
	categoryLabelRe = regexp.MustCompile(`([:;,]\s*)((?:[a-z][a-z/-]*\s+){0,4}(?:course|seminar)),?\s+(` + grammar.CodePat + `)`)
	// categoryLabelStopRe matches words that make the label a requirement:
	// "one course".
	categoryLabelStopRe = regexp.MustCompile(`\b(?:and|or|plus|any|one|two|a|an|the|other|additional|each|either|including|excluding|with|without|not|no|of|from|in|at)\b`)
	// concurrentParenRe matches advice on when a course is taken:
	// "EEB498Y1 (concurrent with research issues course EEB488H1)".
	concurrentParenRe = regexp.MustCompile(`(` + grammar.CodePat + `)\s*\((concurrent\s+with\s+[^()]*)\)`)
)

// stripCategoryLabels drops the labels of groups of courses in a list
// ("field course EEB403H1" is "EEB403H1"), and returns notes of when a
// course is taken.
func stripCategoryLabels(s string) (string, []string) {
	ms := categoryLabelRe.FindAllStringSubmatchIndex(s, -1)
	if len(ms) == 0 {
		return s, nil
	}
	var b strings.Builder
	last := 0
	for _, m := range ms {
		if categoryLabelStopRe.MatchString(s[m[4]:m[5]]) {
			continue
		}
		b.WriteString(s[last:m[3]])
		last = m[6]
	}
	if last == 0 {
		return s, nil
	}
	b.WriteString(s[last:])
	t := b.String()
	var notes []string
	for _, m := range concurrentParenRe.FindAllStringSubmatch(t, -1) {
		notes = append(notes, m[1]+" "+m[2])
	}
	return concurrentParenRe.ReplaceAllString(t, "$1"), notes
}
