package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// A phrase matcher looks at the start of s. If it recognizes its phrase it
// returns a token whose end is the number of bytes consumed (start is 0; the
// lexer shifts both to absolute positions).
type phraseMatcher func(s string) (token, bool)

var (
	courseRe = regexp.MustCompile(`^` + codePat)

	// "75% or higher in", "minimum of 77% in", "Minimum 70% in required courses:".
	// The last group marks a list the grade applies to as a whole.
	gradePrefixRe = regexp.MustCompile(`^(?i:(?:a\s+)?(?:(?:minimum|final)\s+)*(?:(?:grade|mark)\s+)?(?:of\s+)?(?:at\s+least\s+)?)?(\d{2})%\s+(?i:(?:or\s+(?:higher|above|better|greater)|is\s+required)\s+)?(?i:in)\b(\s+(?i:each\s+of|all\s+of|required\s+courses\s*:))?`)

	condRe      = regexp.MustCompile(`^(?:For\s+[^:;.()]{1,60}|[A-Z][A-Za-z]+\s+students|[A-Z][a-z]+\s+[A-Z][a-z]+):`)
	condLabelRe = regexp.MustCompile(`^(?:Note|Notes|Recommended|Prerequisites?|Corequisites?|Exclusions?)\b`)
	condNextRe  = regexp.MustCompile(`^\s*(?:` + codePat + `|\()`)

	// Count pools: "one of", "two of", "Any of", "1 course from", "a course in
	// statistics from", "Two HIS courses from the following:".
	poolCountRe = regexp.MustCompile(`^(?i:(?:completion\s+of\s+)?(?:at\s+least\s+)?)` +
		`(?i:(a|an|one|any|two|three|four|[1-4]))\s+(?:[A-Z]{3}\s+)?(?i:courses?\s+)?(?i:in\s+[a-z]+\s+)?(?i:of|from)\b` +
		`(?i:\s+either)?(?i:\s+the\s+following(?:\s+(?:courses|combinations|options))?)?(?i:\s+is\s+required)?(?:\s*:)?`)
	// "of which at least one course needs to be A/ B/ C"
	poolOfWhichCountRe = regexp.MustCompile(`^(?i:of\s+which\s+at\s+least\s+)(?i:(one|two|three|four|[1-4]))\s+(?i:(?:courses?\s+)?(?:needs?\s+to\s+be|must\s+be|is|are|from|of))\b(?:\s*:)?`)
	// Credit pools: "1.0 credit from: A, B", "at least 1.0 credit from:", "0.5 from A/ B",
	// "0.5 additional credit from A", "0.5 credits in A/ B",
	// "1.0 credit from 300-level French literature courses: A, B".
	poolFromRe = regexp.MustCompile(`^(?i:at\s+least\s+)?(\d+\.\d+)\s+(?i:additional\s+)?(?:[A-Z]{3}(?:\s*/\s*[A-Z]{3})*\s+)?` +
		`(?i:(?:credits?|courses?)\s+)?(?i:from|among|in)\b(?:(?:\s+(?:[A-Za-z][A-Za-z-]*|\d00(?:-level|\+)?))*\s*:|(?i:\s+the\s+following))?`)
	poolOfWhichRe = regexp.MustCompile(`^(?i:of\s+which\s+(?:at\s+least\s+)?)(\d+(?:\.\d+)?)\s+(?i:(?:credits?\s+)?(?:(?:must|should)\s+be\s+)?(?:in|from))(?:\s*:)?`)
	poolNextRe    = regexp.MustCompile(`^\s*(?:` + codePat + `|\()`)

	breadthRe = regexp.MustCompile(`^(?i:at\s+least\s+)?(\d+(?:\.\d+)?)\s+(?i:(?:credits?\s+)?of\s+which\s+(?:should|must)\s+be\s+in\s+(?:the\s+)?)(?:[A-Z]{3}\s*/\s*)?BR\s*=\s*([1-5](?:\s*/\s*[1-5])*)`)

	scopePat = `(?:[A-Z]{2,}|[A-Z][a-z]+)(?:\s+(?:and\s+)?(?:[A-Z]{2,}|[A-Z][a-z]+|program|courses?|credits?))*`
	// "Minimum GPA of 3.5 in ...", "Cumulative GPA of at least 2.70", "with a
	// CGPA of at least 2.5", "a minimum cGPA of 3.0 in all CHM credits".
	gpaRe = regexp.MustCompile(`^(?i:(?:with\s+)?(?:an?\s+)?(?:(?:minimum|cumulative|overall)\s+(?:of\s+)?)*)?(?i:c?gpa)\s+(?i:of\s+)?(?i:at\s+least\s+)?` +
		`(\d\.\d+|[A-D][+-]?)(?:\s+(?i:or\s+(?:higher|above|better)))?(?:\s+(?i:required))?(?:\s+(?i:in|for)\s+(?i:all\s+)?(` + scopePat + `))?`)
	// "3.0 GPA in Political Science courses", "3.0 CGPA required".
	gpaNumFirstRe = regexp.MustCompile(`^(\d\.\d+)\s+(?i:c?gpa)(?:\s+(?i:required))?(?:\s+(?i:in|for)\s+(?i:all\s+)?(` + scopePat + `))?`)

	yearNumRe = regexp.MustCompile(`^(?i:(?:in\s+|enrolled\s+as\s+a\s+)?year\s+([1-4])(?:\s+(?:students?|standing|or\s+higher|or\s+above))*)\b`)
	yearOrdRe = regexp.MustCompile(`^(?i:(first|second|third|fourth|1st|2nd|3rd|4th)(?:\s*(?:and|or|/|,)\s*(?:first|second|third|fourth|1st|2nd|3rd|4th))*[- ]year\s+(?:students?|standing|status)(?:\s+only)?)\b`)
	ordinalRe = regexp.MustCompile(`(?i)first|second|third|fourth|1st|2nd|3rd|4th`)

	// Prefixes may repeat: "Completion of at least 4.0 credits".
	creditHeadRe = regexp.MustCompile(
		`^(?i:(?:completion(?:\s+of)?|(?:(?:students\s+)?(?:who\s+|must\s+)?ha(?:ve|s)\s+)?(?:completed|taken)|(?:at\s+)?least|(?:a\s+)?(?:combined\s+)?minimum(?:\s+of)?|any|an\s+additional|additional|another|other|one|a\s+further|a\s+total\s+of)\s+)*` +
			`(\d+(?:\.\d+)?)\s+` +
			`(?i:(?:additional|more|full|half|other)\s+)?` +
			`((?:(?:[A-Z]{3}\b|\d00(?:-level\b|\+)?)\s*(?:(?:/|or)\s*)?)*)` + // "HIS or POL ", "ENG ", "200-level CAS "
			`(?:BR\s*=?\s*(` + brNumsPat + `)\s+)?` + // "3.0 BR= 1/ 2 credits"
			`(?i:credits?|courses?|FCEs?)\b`)

	anyCourseHeadRe = regexp.MustCompile(`^(?i:(?:(?:a\s+)?minimum\s+of\s+|at\s+least\s+)?(?:a|an|any|one))\s+`)
	// "a full course in PHL/CSC/MAT", "a half-course equivalent in ...".
	fullCourseRe = regexp.MustCompile(`^(?i:(full|half)[- ]course(?:\s+equivalent)?)\b`)
	levelLedRe   = regexp.MustCompile(`^\d00`)
	courseWordRe = regexp.MustCompile(`(?i)\bcourses?\b`)

	// A connector (, / or and ", or" ", and") is only allowed when immediately
	// followed by a designator or level — this is what stops the span from
	// eating "2.0 ENG credits and any 4.0 credits".
	qualSpanRe = regexp.MustCompile(
		`^(?:\s*(?:(?i:in|at|the|from|of|any|additional)\b|(?i:-?\s*levels?)\b|(?i:courses?)\b|(?i:or\s+(?:higher|above))\b|` +
			qualItemPat + `|\d00\s*\+?|-|` +
			`(?:,\s*(?i:or|and)?|/|(?i:or)|(?i:and))\s*(?:` + qualItemPat + `|\d00)))*`)

	// Subject names and year words that may stand in for designators and
	// levels: "first-year math, or physics courses", "HIS/REL/Peace and Conflict".
	subjectPat  = `(?i:mathematics|math|physics|chemistry|biology|philosophy|economics|history|psychology|sociology|anthropology|astronomy|geography|linguistics|computer\s+science|political\s+science|peace\s+and\s+conflict|study\s+of\s+religion|religion|canadian\s+studies|jewish\s+studies)\b`
	yearWordPat = `(?i:first|second|third|fourth)-year\b`
	qualItemPat = `(?:[A-Z]{3}\b|` + subjectPat + `|` + yearWordPat + `)`
	subjectRe   = regexp.MustCompile(subjectPat)
	yearWordRe  = regexp.MustCompile(`(?i)(first|second|third|fourth)-year`)

	// Qualifier words with nothing after them belong to whatever follows:
	// "any 300-level course in Society" must leave "in Society" as text.
	trailingQualRe = regexp.MustCompile(`(?i)(?:\s*-|\s+(?:in|at|the|from|of|any))+\s*$`)

	excludingRe  = regexp.MustCompile(`^\s*\(\s*(?i:excluding|except)\s+([^)]*)\)|^\s*,?\s*(?i:excluding|except)\s+(` + codePat + `(?:\s*(?:,|/|and|or)\s*` + codePat + `)*)`)
	anyFieldRe   = regexp.MustCompile(`^\s*(?:\(\s*)?(?i:in\s+any\s+field)(?:\s*\))?`)
	desigRe      = regexp.MustCompile(`\b[A-Z]{3}\b`)
	levelRe      = regexp.MustCompile(`([1-4])00\s*(\+)?`)
	orUpRe       = regexp.MustCompile(`(?i)or\s+(?:higher|above)`)
	breadthNumRe = regexp.MustCompile(`[1-5]`)
)

// yearCredits maps a year of study to the credits needed to be in it
// (Faculty of Arts & Science thresholds).
var yearCredits = map[int]float64{1: 0, 2: 4.0, 3: 9.0, 4: 14.0}

// subjectDesignators maps subject names used in prose to designators.
var subjectDesignators = map[string]string{
	"mathematics": "MAT", "math": "MAT", "physics": "PHY", "chemistry": "CHM",
	"biology": "BIO", "philosophy": "PHL", "economics": "ECO", "history": "HIS",
	"psychology": "PSY", "sociology": "SOC", "anthropology": "ANT", "astronomy": "AST",
	"geography": "GGR", "linguistics": "LIN", "computer science": "CSC",
	"political science": "POL", "peace and conflict": "PCS", "religion": "RLG",
	"study of religion": "RLG", "canadian studies": "CDN", "jewish studies": "CJS",
}

// countWords maps the count word of a pool ("two of") to a number.
var countWords = map[string]int{
	"a": 1, "an": 1, "one": 1, "any": 1, "1": 1,
	"two": 2, "2": 2, "three": 3, "3": 3, "four": 4, "4": 4,
}

// letterGPA maps letter grades to the UofT 4.0 scale.
var letterGPA = map[string]float64{
	"A+": 4.0, "A": 4.0, "A-": 3.7, "B+": 3.3, "B": 3.0, "B-": 2.7,
	"C+": 2.3, "C": 2.0, "C-": 1.7, "D+": 1.3, "D": 1.0, "D-": 0.7,
}

// phraseMatchers in the order the lexer tries them (design §4.3). Order
// matters: earlier phrases protect "or"/"and" words from the operator
// matchers, and pools must be tried before plain credit phrases.
var phraseMatchers = []phraseMatcher{
	matchCond,
	matchProgram,
	matchGradePrefix,
	matchCourse,
	matchGradeSuffix,
	matchGPA, // before pools: "a GPA of at least 3.5" is not "a ... of"
	matchPool,
	matchBreadth,
	matchYear,
	matchCredits,
	matchAnyCourse,
}

// matchCourse reads a course code and a grade qualifier right after it:
// "STA247H1(70%)", "EAS110Y1 (63% minimum)", "HIS250Y1 (B or higher)",
// "MAT136H1 with a minimum mark of 77". A qualifier that ends in "in each"
// covers more than this course, so it is left for the parser.
func matchCourse(s string) (token, bool) {
	code := courseRe.FindString(s)
	if code == "" {
		return token{}, false
	}
	n := &types.ReqNode{Type: types.NodeCourse, Code: code}
	end := len(code)
	afterComma := strings.HasPrefix(strings.TrimSpace(s[end:]), ",")
	for _, paren := range []bool{true, false} {
		// "A, B, with a minimum grade of ..." may mean the whole list: leave it.
		if g, each, e, ok := matchGradeBody(s[end:], paren); ok && (paren || !each && !afterComma) {
			n.MinGrade = g
			end += e
			break
		}
	}
	return token{kind: tCourse, end: end, node: n}, true
}

func matchGradePrefix(s string) (token, bool) {
	m := gradePrefixRe.FindStringSubmatch(s)
	if m == nil {
		return token{}, false
	}
	g, _ := strconv.Atoi(m[1])
	return token{kind: tGradePrefix, end: len(m[0]), grade: g, each: m[2] != ""}, true
}

func matchCond(s string) (token, bool) {
	loc := condRe.FindStringIndex(s)
	if loc == nil || condLabelRe.MatchString(s) {
		return token{}, false
	}
	// "Rotman Commerce:" only counts as a label when a course or group follows it.
	if !strings.HasPrefix(s, "For") && !condNextRe.MatchString(s[loc[1]:]) {
		return token{}, false
	}
	return token{kind: tCond, end: loc[1]}, true
}

func matchPool(s string) (token, bool) {
	if m := poolCountRe.FindStringSubmatch(s); m != nil {
		return token{kind: tPool, end: len(m[0]), count: countWords[strings.ToLower(m[1])]}, true
	}
	if m := poolOfWhichCountRe.FindStringSubmatch(s); m != nil && poolNextRe.MatchString(s[len(m[0]):]) {
		return token{kind: tPool, end: len(m[0]), count: countWords[strings.ToLower(m[1])]}, true
	}
	for _, re := range []*regexp.Regexp{poolFromRe, poolOfWhichRe} {
		m := re.FindStringSubmatchIndex(s)
		if m == nil || !poolNextRe.MatchString(s[m[1]:]) {
			continue // "1.0 credit from 200-level courses" is a credit phrase, not a pool
		}
		amount, _ := strconv.ParseFloat(s[m[2]:m[3]], 64)
		return token{kind: tPool, end: m[1], amount: amount}, true
	}
	return token{}, false
}

func matchBreadth(s string) (token, bool) {
	m := breadthRe.FindStringSubmatch(s)
	if m == nil {
		return token{}, false
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	n := &types.ReqNode{Type: types.NodeCredits, Amount: amount}
	for _, b := range breadthNumRe.FindAllString(m[2], -1) {
		v, _ := strconv.Atoi(b)
		n.Breadth = append(n.Breadth, v)
	}
	return token{kind: tCredits, end: len(m[0]), node: n}, true
}

func matchGPA(s string) (token, bool) {
	m := gpaRe.FindStringSubmatch(s)
	if m == nil {
		if m = gpaNumFirstRe.FindStringSubmatch(s); m == nil {
			return token{}, false
		}
	}
	gpa, ok := letterGPA[m[1]]
	if !ok {
		gpa, _ = strconv.ParseFloat(m[1], 64)
	}
	n := &types.ReqNode{Type: types.NodeGPA, Min: gpa, Scope: m[2]}
	return token{kind: tGPA, end: len(m[0]), node: n}, true
}

func matchYear(s string) (token, bool) {
	year, end := 0, 0
	if m := yearNumRe.FindStringSubmatch(s); m != nil {
		year, _ = strconv.Atoi(m[1])
		end = len(m[0])
	} else if m := yearOrdRe.FindString(s); m != "" {
		year = 4
		for _, o := range ordinalRe.FindAllString(m, -1) {
			year = min(year, ordinalYear(o))
		}
		end = len(m)
	} else if y, e, ok := matchYearIn(s); ok {
		year, end = y, e
	} else {
		return token{}, false
	}
	n := &types.ReqNode{Type: types.NodeCredits, Amount: yearCredits[year]}
	return token{kind: tCredits, end: end, node: n}, true
}

func ordinalYear(o string) int {
	switch strings.ToLower(o) {
	case "first", "1st":
		return 1
	case "second", "2nd":
		return 2
	case "third", "3rd":
		return 3
	}
	return 4
}

func matchCredits(s string) (token, bool) {
	// "no more than 14.0 credits": an upper bound (Max) instead of Amount.
	head, isMax := 0, false
	if loc := maxHeadRe.FindStringIndex(s); loc != nil {
		head, isMax = loc[1], true
	}
	// "two other 0.5 credit courses" = 1.0 credit.
	count := 1
	if m := countHeadRe.FindStringSubmatch(s[head:]); m != nil && !isMax {
		count = countWords[strings.ToLower(m[1])]
		head += len(m[0]) - 1
	}
	m := creditHeadRe.FindStringSubmatchIndex(s[head:])
	if m == nil {
		return token{}, false
	}
	for i := range m {
		if m[i] >= 0 {
			m[i] += head
		}
	}
	amount, _ := strconv.ParseFloat(s[m[2]:m[3]], 64)
	n := &types.ReqNode{Type: types.NodeCredits, Amount: amount * float64(count)}
	if isMax {
		n.Max, n.Amount = n.Amount, 0
	}
	headQual := s[m[4]:m[5]]
	n.Designators = desigRe.FindAllString(headQual, -1)
	if m[6] >= 0 {
		n.Breadth = breadthNums(s[m[6]:m[7]])
	}
	end := m[1]

	span := qualSpan(s[end:])
	if count > 1 && !courseWordRe.MatchString(s[m[0]:end+len(span)]) {
		return token{}, false // "two 0.5 credits" is not a phrase we know
	}
	n.Designators = append(n.Designators, spanDesignators(span)...)
	n.Levels = parseLevels(headQual + " " + span)
	end += len(span)

	end += creditSuffixes(s[end:], n)
	n.Designators = dedupe(n.Designators)
	tok := token{kind: tCredits, end: end, node: n}
	if both, l := ofWhichSuffix(s[end:], n); both != nil {
		tok.node, tok.end = both, end+l
	} else if both, l := averageSuffix(s[end:], n); both != nil {
		tok.node, tok.end = both, end+l
	}
	return tok, true
}

// matchAnyCourse handles "A 300-level CDN course", "Any AST 100-level or AST
// 200-level course": half a credit matching the designators and levels.
//
// "a full course in PHL/CSC/MAT" is 1.0 credit, and "200-level History
// course" (no article) is half a credit when it starts with a level.
func matchAnyCourse(s string) (token, bool) {
	start := 0
	if loc := anyCourseHeadRe.FindStringIndex(s); loc != nil {
		start = loc[1]
	} else if !levelLedRe.MatchString(s) {
		return token{}, false
	}
	amount := 0.5
	full := fullCourseRe.FindStringSubmatch(s[start:])
	if full != nil {
		if start == 0 {
			return token{}, false
		}
		if strings.EqualFold(full[1], "full") {
			amount = 1.0
		}
		start += len(full[0])
	}
	span := qualSpan(s[start:])
	if full == nil && !courseWordRe.MatchString(span) {
		return token{}, false
	}
	n := &types.ReqNode{Type: types.NodeCredits, Amount: amount}
	n.Designators = dedupe(spanDesignators(span))
	n.Levels = parseLevels(span)
	if len(n.Designators) == 0 && len(n.Levels) == 0 {
		return token{}, false // a bare "a course" in prose
	}
	if start == 0 && (len(n.Designators) == 0 || !strings.HasSuffix(strings.ToLower(span), "course")) {
		// Without an article only "200-level History course" is safe: not
		// "200-level courses" (how many?) or the tail of a longer name
		// ("... Core Group 300+ level course").
		return token{}, false
	}
	end := start + len(span)
	end += creditSuffixes(s[end:], n)
	return token{kind: tCredits, end: end, node: n}, true
}

// spanDesignators returns the designators named in a qualifier span, either
// directly (HIS) or by subject name (history).
func spanDesignators(span string) []string {
	desigs := desigRe.FindAllString(span, -1)
	for _, m := range subjectRe.FindAllString(span, -1) {
		desigs = append(desigs, subjectDesignators[strings.ToLower(wsRe.ReplaceAllString(m, " "))])
	}
	return desigs
}

// qualSpan returns the run of qualifier vocabulary at the start of s, minus
// any dangling qualifier words at its end.
func qualSpan(s string) string {
	span := qualSpanRe.FindString(s)
	return trailingQualRe.ReplaceAllString(span, "")
}

// creditSuffixes consumes value-carrying qualifiers (in any order) after a
// credit phrase, sets them on n, and returns how many bytes it consumed.
func creditSuffixes(s string, n *types.ReqNode) int {
	end := 0
	for {
		rest := s[end:]
		if g, _, e, ok := matchGradeBody(rest, false); ok {
			n.MinGrade = g
			end += e
		} else if b, e := breadthSuffix(rest); e > 0 && n.Breadth == nil {
			n.Breadth = b
			end += e
		} else if m := completedRe.FindString(rest); m != "" {
			end += len(m) // "9.0 credits completed"
		} else if m := excludingRe.FindStringSubmatch(rest); m != nil {
			n.Exclude = append(n.Exclude, codeRe.FindAllString(m[1]+m[2], -1)...)
			end += len(m[0])
		} else if m := anyFieldRe.FindString(rest); m != "" {
			end += len(m)
		} else {
			return end
		}
	}
}

// parseLevels reads course levels from a qualifier span.
// "300-level" -> [300]; "300+ level" / "300-level or higher" -> [300 400];
// "300-/400-level", "300 or 400 level", "200-level or 300-level" -> listed levels.
func parseLevels(span string) []int {
	var out []int
	up := orUpRe.MatchString(span)
	for _, m := range yearWordRe.FindAllStringSubmatch(span, -1) {
		out = append(out, ordinalYear(m[1])*100)
	}
	for _, m := range levelRe.FindAllStringSubmatch(span, -1) {
		lvl, _ := strconv.Atoi(m[1])
		out = append(out, lvl*100)
		up = up || m[2] == "+"
	}
	if up && len(out) > 0 {
		for l := slices.Max(out) + 100; l <= 400; l += 100 {
			out = append(out, l)
		}
	}
	return dedupe(out)
}

// dedupe removes repeated values, keeping first-seen order.
func dedupe[T comparable](xs []T) []T {
	var out []T
	seen := map[T]bool{}
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}
