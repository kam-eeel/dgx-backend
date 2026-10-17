package reqs

import (
	"regexp"
	"strings"
)

// Course titles inside course lists (T10-7). A title names the course
// before it and changes nothing a pool requires, so it is dropped:
//   - in brackets: "JEG100H1 (Introduction to Physical Geography)/ ESS262H1
//     (Earth System Processes)" (stripAnnotations, isCourseTitle);
//   - bare, up to the list's next separator: "RLG201H1 Indigenous
//     Spiritualities and Religions/ RLG202H1 Judaism/ ..." (stripBareTitles).
//
// A title is in title case: every word capitalised and holding a lowercase
// letter ("UTM", "NOTE" and "OR" are not title words), except the small
// words between them. Words that say something about the requirement
// ("Recommended", "Approved", "Credits") are never a title.

var (
	// complTitleCaseRe: words in title case, the first capitalised.
	complTitleCaseRe = regexp.MustCompile(`^[A-Z][a-z][\w'’-]*(?:,?\s+(?:[A-Z][a-z][\w'’-]*|[A-Z]|of|and|the|in|to|for|on|at|a|an|&|from|with|by|de|du|des|la|le|et)|:\s+[A-Z][a-z][\w'’-]*)*$`)
	// complTitleRequirementRe: words that say something about the
	// requirement, never a course's title.
	complTitleRequirementRe = regexp.MustCompile(`(?i)\b(?:or|and/or|if|unless|only|plus|any|taught|approved|approval|permission|consent|equivalents?|required|requirements?|recommended|note|notes|see|formerly|not|no|must|may|can|should|exclud\w*|includ\w*|except|minimum|maximum|grade|credits?|courses?|level|series|students?|prerequisites?|exclusions?|offered|program|programs|group|list|category|one|two|three|either|both|each)\b`)
	// complBareTitleRe: a code and the words after it, up to the list's
	// next separator or its end.
	complBareTitleRe = regexp.MustCompile(`(` + codePat + `)[ \t]+([A-Z][^/,;()\[\]\n]*?)\s*(?:[/,;)\]]|\.?\s*$)`)
)

// isCourseTitle reports words that are a course's title.
func isCourseTitle(s string) bool {
	s = strings.TrimSpace(s)
	return complTitleCaseRe.MatchString(s) && !complTitleRequirementRe.MatchString(s)
}

// stripBareTitles drops the titles after the codes in a course list:
// "RLG201H1 Indigenous Spiritualities and Religions/ RLG202H1 Judaism" ->
// "RLG201H1/ RLG202H1". Only when two codes or more have titles: one
// capitalised phrase after a code may be prose.
func stripBareTitles(s string) string {
	ms := complBareTitleRe.FindAllStringSubmatchIndex(s, -1)
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

// complPoolTopicOrRe: a topic of two subjects at the end of a pool's
// description, "in plant or microbial biology": words, not a second pool.
var complPoolTopicOrRe = regexp.MustCompile(`\bin\s+[a-z]+\s+or\s+[a-z]+(?:\s+[a-z]+)?\s*$`)

// poolTopicOr is a pool's description without a topic of two subjects
// ("0.5 credit at the 300+ level in plant or microbial biology from: ..."),
// whose "or" is no second pool (matchDescribedPool).
func poolTopicOr(desc string) string {
	return complPoolTopicOrRe.ReplaceAllString(desc, "")
}

var (
	// complCategoryLabelRe: a lowercase label of a group of courses before
	// its first code: "from: field course, EEB401H1, ...; seminar EEB491H1,
	// ...; independent research project course, EEB497H1".
	complCategoryLabelRe = regexp.MustCompile(`([:;,]\s*)((?:[a-z][a-z/-]*\s+){0,4}(?:course|seminar)),?\s+(` + codePat + `)`)
	// complCategoryLabelStopRe: words that make the label a requirement ("one
	// course", "any other course").
	complCategoryLabelStopRe = regexp.MustCompile(`\b(?:and|or|plus|any|one|two|a|an|the|other|additional|each|either|including|excluding|with|without|not|no|of|from|in|at)\b`)
	// complConcurrentParenRe: "EEB498Y1 (concurrent with research issues
	// course EEB488H1)": when the course is taken, advice.
	complConcurrentParenRe = regexp.MustCompile(`(` + codePat + `)\s*\((concurrent\s+with\s+[^()]*)\)`)
)

// stripCategoryLabels drops the labels of groups of courses in a list, and
// returns a note of when a course is taken: "field course EEB403H1, ...;
// seminar/lecture course EEB428H1, ...; EEB497H1, independent research
// project course EEB498Y1 (concurrent with research issues course
// EEB488H1)" -> "EEB403H1, ...; EEB428H1, ...; EEB497H1, EEB498Y1".
func stripCategoryLabels(s string) (string, []string) {
	ms := complCategoryLabelRe.FindAllStringSubmatchIndex(s, -1)
	if len(ms) == 0 {
		return s, nil
	}
	var b strings.Builder
	last := 0
	for _, m := range ms {
		if complCategoryLabelStopRe.MatchString(s[m[4]:m[5]]) {
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
	for _, m := range complConcurrentParenRe.FindAllStringSubmatch(t, -1) {
		notes = append(notes, m[1]+" "+m[2])
	}
	return complConcurrentParenRe.ReplaceAllString(t, "$1"), notes
}
