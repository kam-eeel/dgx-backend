// Package reqs parses U of T calendar requirement text (course requisites and
// exclusions, program enrolment and completion) into requirement trees.
package reqs

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// codePat matches a course code: CSC148H1, MATA22H3, MAT223H5.
const codePat = `[A-Z]{3}[A-Z0-9]\d{2}[HY]\d`

var (
	codeRe = regexp.MustCompile(codePat)

	charReplacer = strings.NewReplacer(
		"​", " ", " ", " ",
		"[", "(", "]", ")",
		"and/or", "/",
	)
	urlRe         = regexp.MustCompile(`https?://\S+`)
	spacedCode    = regexp.MustCompile(`\b([A-Z]{3}) ([A-D0-9]\d{2}[HY]\d)\b`)                           // POL 478H1
	footnoteRe    = regexp.MustCompile(`(` + codePat + `)\*+`)                                           // GGR270H1*
	garbledRe     = regexp.MustCompile(`\b[A-Z]{3}\d{1,3}(` + codePat + `)\b`)                           // MAT1MAT157Y1 (source typo)
	annotationRe  = regexp.MustCompile(`(?i)\(\s*exclusion\s+to\s+` + codePat + `\s*\)`)                 // "(exclusion to CHM135H1)"
	leadingOpRe   = regexp.MustCompile(`\(\s*(or|including)\s+`)                                         // "X (or Y)" -> "X or (Y)"
	completionRe  = regexp.MustCompile(`(?i)\b(?:successful\s+)?completion\s+of\s+(\(|` + codePat + `)`) // "Completion of SOC201H1"
	requiredToRe  = regexp.MustCompile(`(?i)\bstudents\s+are\s+(?:also\s+)?required\s+to\s+(?:complete|take|have\s+completed)\s+`)
	combinationRe = regexp.MustCompile(`(?i)\bthe\s+combination\s+of\s+(` + codePat + `)\s+and\s+(` + codePat + `)`)
	beforeRe      = regexp.MustCompile(`(?i)\s*completed\s+before\s+` + codePat + `\s+is\s+taken`)               // "completed before MUN368H1 is taken"
	ifCompleteRe  = regexp.MustCompile(`(?i)\s+if\s+(` + codePat + `)\s+(?:is|has\s+been)\s+completed?\b`)       // "X if DRM220Y1 is complete"
	includesRe    = regexp.MustCompile(`(?i)\bwhich\s+includes?\b`)                                              // "7.5 credits which include …"
	atLeastInRe   = regexp.MustCompile(`(?i),?\s*\bwith\s+at\s+least\s+(\d+(?:\.\d+)?)\s+(?:credits?\s+)?in\s+`) // "with at least 1.5 in PHL"
	exampleRe     = regexp.MustCompile(`\(\s*(?i:e\.g\.),?\s*([^()]*)\)`)                                        // "(e.g. PSY201H1, SOC202H1)"
	rangeRe       = regexp.MustCompile(`\b([A-Z]{3})(\d{3})([HY]\d)\s*-\s*([A-Z]{3})(\d{3})([HY]\d)\b`)          // PHL344H1 - PHL349H1
	altPathRe     = regexp.MustCompile(`(?i)\b([A-Z][A-Za-z]*)\s+students\s+who\s+have\s+(?:completed|taken)\s+(.+?)\s+(?:will\s+be|are)\s+permitted\s+to\s+(?:enrol|enroll|register)(?:\s+in\s+` + codePat + `)?`)
	engineeringRe = regexp.MustCompile(`(?i)\bPrerequisites?\s+for\s+(?:the\s+)?(?:Faculty\s+of\s+)?Applied\s+Science\s+and\s+Engineering\s+students:`)
	enumStartRe   = regexp.MustCompile(`\(\s*1\s*\)`)
	enumSplitRe   = regexp.MustCompile(`\(\s*\d\s*\)`)
	enumTrailRe   = regexp.MustCompile(`(?i)[\s,;]*\b(?:or|and)?[\s,;]*$`)
	enumRefRe     = regexp.MustCompile(`(?i)\brequirements?\s*$`) // "requirement (1)": a reference
	wsRe          = regexp.MustCompile(`\s+`)
)

// normalize cleans raw catalogue text into one spelling of everything for the
// lexer. Text that must not be parsed is returned as notes: URLs (their "/" is
// never OR), "(exclusion to X)" annotations and "completed before X is taken".
func normalize(s string) (out string, notes []string) {
	notes = urlRe.FindAllString(s, -1)
	s = urlRe.ReplaceAllString(s, "")
	s = charReplacer.Replace(s)
	s = spacedCode.ReplaceAllString(s, "$1$2")
	s = footnoteRe.ReplaceAllString(s, "$1")
	s = garbledRe.ReplaceAllString(s, "$1")
	s = repairCodes(s)
	for _, m := range annotationRe.FindAllString(s, -1) {
		notes = append(notes, cleanNote(m[1:len(m)-1]))
	}
	s = annotationRe.ReplaceAllString(s, "")

	s = leadingOpRe.ReplaceAllString(s, " $1 (")
	s = completionRe.ReplaceAllString(s, "$1")
	s = requiredToRe.ReplaceAllString(s, "")
	s = combinationRe.ReplaceAllString(s, "($1 and $2)")
	for _, m := range beforeRe.FindAllString(s, -1) {
		notes = append(notes, cleanNote(m))
	}
	s = beforeRe.ReplaceAllString(s, "")
	s = ifCompleteRe.ReplaceAllString(s, " and $1")
	s = includesRe.ReplaceAllString(s, "including")
	s = atLeastInRe.ReplaceAllString(s, ", at least $1 credits in ")
	s = rangeRe.ReplaceAllStringFunc(s, expandRange)
	s = altPathRe.ReplaceAllString(s, "$1 students: $2")
	s = engineeringRe.ReplaceAllString(s, "Engineering students:")

	s = strings.TrimSpace(wsRe.ReplaceAllString(s, " "))
	return rewriteEnumerations(s), notes
}

var (
	doubledRe     = regexp.MustCompile(`\b([A-Z]{3}[A-Z0-9]\d{2}[HY])(\d)(\d)\b`)                                       // HIS322H11 (source typo)
	sessionCodeRe = regexp.MustCompile(`\b(` + codePat + `)[FSY]\b`)                                                    // BMS301H1S, WGS160Y1Y: ACORN session letter
	noCampusRe    = regexp.MustCompile(`\b([A-Z]{3}\d{3}[HY])\b`)                                                       // MAT148H, LAS200Y (source typo)
	seriesRe      = regexp.MustCompile(`(?:\b(?i:an?|any|one)\s+)?\b([A-Z]{3})([1-4]00)-series\b(\s+(?i:courses?)\b)?`) // MAT100-series
)

// repairCodes fixes misprinted codes where the repair is certain: a doubled or
// missing campus digit (this catalogue always writes UTM and UTSC campuses out),
// a glued session letter, and "MAT100-series" (not the plural "... courses").
func repairCodes(s string) string {
	s = doubledRe.ReplaceAllStringFunc(s, func(m string) string {
		if g := doubledRe.FindStringSubmatch(m); g[2] == g[3] {
			return g[1] + g[2]
		}
		return m
	})
	s = sessionCodeRe.ReplaceAllString(s, "$1")
	s = noCampusRe.ReplaceAllString(s, "${1}1")
	return seriesRe.ReplaceAllStringFunc(s, func(m string) string {
		g := seriesRe.FindStringSubmatch(m)
		if strings.HasSuffix(strings.ToLower(g[3]), "s") {
			return m
		}
		return "a " + g[2] + "-level " + g[1] + " course"
	})
}

// rewriteExamples turns "A statistics course (e.g. PSY201H1, SOC202H1)" into
// "... or one of PSY201H1, SOC202H1": a listed example meets it, anything else
// is Review. It runs on requirement text only, after notes are split off.
func rewriteExamples(s string) string {
	return exampleRe.ReplaceAllStringFunc(s, rewriteExample)
}

func rewriteExample(m string) string {
	inner := exampleRe.FindStringSubmatch(m)[1]
	if !codeRe.MatchString(inner) {
		return m
	}
	return " or one of " + inner
}

// expandRange turns "PHL344H1 - PHL349H1" into "PHL344H1/ PHL345H1/ … / PHL349H1".
func expandRange(m string) string {
	p := rangeRe.FindStringSubmatch(m)
	lo, _ := strconv.Atoi(p[2])
	hi, _ := strconv.Atoi(p[5])
	if p[1] != p[4] || p[3] != p[6] || hi <= lo || hi-lo > 20 {
		return m
	}
	codes := make([]string, 0, hi-lo+1)
	for n := lo; n <= hi; n++ {
		codes = append(codes, fmt.Sprintf("%s%03d%s", p[1], n, p[3]))
	}
	return strings.Join(codes, "/ ")
}

// rewriteEnumerations turns numbered alternatives into slash-separated groups:
// "one of the following: (1) A and B or (2) C, or (3) D." ->
// "one of the following: (A and B)/ (C)/ (D)."
func rewriteEnumerations(s string) string {
	loc := enumStartRe.FindStringIndex(s)
	if loc == nil || len(enumSplitRe.FindAllString(s[loc[0]:], -1)) < 2 || enumRefRe.MatchString(s[:loc[0]]) {
		// A lone "(1)" or "requirement (1)" is a reference, not an enumeration: "any
		// courses from program requirement (1) above".
		return s
	}
	end := strings.Index(s[loc[0]:], ". ")
	if end < 0 {
		end = len(s)
	} else {
		end += loc[0]
	}
	var items []string
	for _, it := range enumSplitRe.Split(s[loc[0]:end], -1) {
		if it = strings.TrimSpace(enumTrailRe.ReplaceAllString(it, "")); it != "" {
			items = append(items, "("+strings.TrimSuffix(it, ".")+")")
		}
	}
	tail := s[end:]
	if end == len(s) && strings.HasSuffix(s, ".") {
		tail = "."
	}
	return s[:loc[0]] + strings.Join(items, "/ ") + tail
}
