package grammar

import (
	"regexp"
	"strings"
)

// Filler and pointer text: wording that survives parsing but carries no
// requirement of its own.

var (
	// fillerRe matches a text node of only connective wording between structured
	// nodes: "Students must have". Real conditions never consist of these alone.
	fillerRe = regexp.MustCompile(`(?i)^(?:` + strings.Join([]string{
		`(?:students\s+)?(?:must\s+)?(?:have|be)(?:\s+(?:completed|taken))?`, // "Students must have", "have taken", "must be"
		`(?:successfully\s+)?(?:completed?|completion)(?:\s+(?:a\s+)?minimum)?(?:\s+of)?`,
		`(?:a\s+)?(?:combined\s+)?minimum(?:\s+of)?|(?:at\s+)?least(?:\s+(?:an?|one))?`, // quantity words left before a credit phrase
		`with|in|one|(?:any\s+)?other`,
		`of\s+(?:which|these)(?:\s+an?)?`,
		`towards?\s+(?:their|the|a|your)\s+degree`,
		`.*\bopen\s+to\s+students\s+who\s+have\s+(?:obtained\s+standing\s+in|completed|taken)`, // lead-in to the real list
		`UTSC|UTM|UTSG`,
		`as\s+stated\s+in\s+the\s+calendar(?:\s+description)?`,
	}, "|") + `)$`)

	// pointerRe matches a segment that only points elsewhere or describes the
	// course: "See the Department website for details".
	pointerRe = regexp.MustCompile(`(?i)^(?:please\s+)?(?:see|refer\s+to)\b|\bfor\s+(?:more\s+)?(?:details|information)$|\bare\s+posted\s+on\b|` +
		`^possible\b.*requisites?$|^this\s+is\s+an?\b[^.]*\bcourse\s+and\s+will\b|^(?:none|n/?a)$|` +
		`^any\s+(?:course\s+)?combination\b|\b(?:is|are)\s+limited$|\bdeadlines?\s*:|\bstart\s+date$`)

	// noneRe matches a field that says there is no requirement.
	noneRe = regexp.MustCompile(`(?i)^(?:none|n/?a)$`)

	// campusTagRe matches a campus tag that repeats a code's suffix: "BIOC12H3(UTSC)".
	campusTagRe = regexp.MustCompile(`(` + CodePat + `)\s*\(\s*(?:UTSC|UTM|UTSG)\s*\)`)
	// eligibleRe matches a purpose, not a further condition: " to be eligible for
	// 300-level Independent Research".
	eligibleRe = regexp.MustCompile(`(?i)[\s,]*\b(?:in\s+order\s+)?to\s+be\s+eligible\s+for\b[^.;]*`)
	// haveTakenRe matches "have taken 10.0 credits", rewritten to "completed 10.0
	// credits" for the credit phrase; "who have completed ..." is left alone.
	haveTakenRe = regexp.MustCompile(`(?i)(\bwho\s+)?\b(?:students\s+)?(?:must\s+)?have\s+(?:completed|taken)\s+((?:` + AtLeastPat + `\s+)?\d)`)
	// creditCopulaRe matches a copula between a credit amount and its qualifiers:
	// "1.0 credit is from EEB/ BIO".
	creditCopulaRe = regexp.MustCompile(`\b(\d+\.\d+(?:\s+[A-Z]{3})?(?:\s+(?i:credits?))?)(\s+(?i:of\s+which))?\s+(?i:is|are|(?:must|should|needs?\s+to)\s+be)\s+(?i:an?\s+)?`)
)

// stripFiller removes campus tags from codes, drops filler verbs around
// credit phrases, and moves eligibility clauses to notes.
func stripFiller(s string, notes *[]string) string {
	s = campusTagRe.ReplaceAllString(s, "$1")
	for _, m := range eligibleRe.FindAllString(s, -1) {
		*notes = append(*notes, CleanNote(strings.TrimLeft(m, " ,")))
	}
	s = eligibleRe.ReplaceAllString(s, "")
	s = haveTakenRe.ReplaceAllStringFunc(s, func(m string) string {
		p := haveTakenRe.FindStringSubmatch(m)
		if p[1] != "" {
			return m
		}
		return "completed " + p[2]
	})
	return dropCreditCopulas(s)
}

// dropCreditCopulas rewrites "1.0 LIN credit must be at the 300+ level" to
// "1.0 LIN credit at the 300+ level" so the credit phrase reads its
// qualifiers.
func dropCreditCopulas(s string) string {
	var b strings.Builder
	last := 0
	for _, m := range creditCopulaRe.FindAllStringSubmatchIndex(s, -1) {
		rest := s[m[1]:]
		if strings.Contains(strings.SplitN(rest, ".", 2)[0], "BR") {
			continue // breadth phrases ("of which should be in BR=1") have their own matcher
		}
		amount := s[m[2]:m[3]]
		if m[4] >= 0 && !strings.Contains(strings.ToLower(amount), "credit") {
			amount += " credits" // "5.0 of which must be GGR/URB"
		}
		b.WriteString(s[last:m[0]])
		b.WriteString(amount + " ")
		last = m[1]
	}
	b.WriteString(s[last:])
	return b.String()
}

// isPointer reports whether a segment only points elsewhere for details.
// Anything that also says "must", "only" or "required" stays a requirement.
func isPointer(seg string) bool {
	return pointerRe.MatchString(seg) && !mandatoryRe.MatchString(seg)
}

func isFiller(text string) bool { return fillerRe.MatchString(text) }
