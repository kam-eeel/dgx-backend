package grammar

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// phraseMatcher matches its phrase at the start of s and returns a token whose
// end is the bytes consumed (start is 0; the lexer shifts both).
type phraseMatcher func(s string) (Token, bool)

var (
	courseRe = regexp.MustCompile(`^` + CodePat)

	// gradePrefixRe matches a grade before courses: "minimum of 77% in". The last
	// group marks a list the grade applies to as a whole.
	gradePrefixRe = regexp.MustCompile(`^(?i:(?:a\s+)?(?:(?:minimum|final)\s+)*(?:(?:grade|mark)\s+)?(?:of\s+)?(?:` + AtLeastPat + `\s+)?)?(\d{2})%\s+(?i:(?:or\s+(?:higher|above|better|greater)|is\s+required)\s+)?(?i:in)\b(\s+(?i:each\s+of|all\s+of|required\s+courses\s*:))?`)

	condRe      = regexp.MustCompile(`^(?:For\s+[^:;.()]{1,60}|[A-Z][A-Za-z]+\s+students|[A-Z][a-z]+\s+[A-Z][a-z]+):`)
	condLabelRe = regexp.MustCompile(`^(?:Note|Notes|Recommended|Prerequisites?|Corequisites?|Exclusions?)\b`)
	// condCommaRe is a label that ends in a comma: "For FASE students, A, B".
	condCommaRe = regexp.MustCompile(`^For\s+[^:;.(),]{0,60}?\bstudents,`)

	// poolCountRe matches a count pool: "two of".
	poolCountRe = regexp.MustCompile(`^(?i:(?:completion\s+of\s+)?(?:` + AtLeastPat + `\s+)?)` +
		`(?i:(a|an|one|any|two|three|four|[1-4]))\s+(?:[A-Z]{3}\s+)?(?i:courses?\s+)?(?i:in\s+[a-z]+\s+)?(?i:of|from)\b` +
		`(?i:\s+either)?(?i:\s+the\s+following(?:\s+(?:courses|combinations|options))?)?(?i:\s+is\s+required)?(?:\s*:)?`)
	// poolOfWhichCountRe matches a count after a pool: "of which at least one
	// course needs to be".
	poolOfWhichCountRe = regexp.MustCompile(`^(?i:of\s+which\s+` + AtLeastPat + `\s+)(?i:(one|two|three|four|[1-4]))\s+(?i:(?:courses?\s+)?(?:needs?\s+to\s+be|must\s+be|is|are|from|of))\b(?:\s*:)?`)
	// poolFromRe matches a credit pool: "1.0 credit from:".
	poolFromRe = regexp.MustCompile(`^(?i:` + AtLeastPat + `\s+)?(\d+\.\d+)\s+(?i:additional\s+)?(?:[A-Z]{3}(?:\s*/\s*[A-Z]{3})*\s+)?` +
		`(?i:(?:credits?|courses?)\s+)?(?i:from|among|in)\b(?:(?:\s+(?:[A-Za-z][A-Za-z-]*|\d00(?:-level|\+)?))*\s*:|(?i:\s+the\s+following))?`)
	poolOfWhichRe = regexp.MustCompile(`^(?i:of\s+which\s+(?:` + AtLeastPat + `\s+)?)` + AmountPat + `\s+(?i:(?:credits?\s+)?(?:(?:must|should)\s+be\s+)?(?:in|from))(?:\s*:)?`)
	poolNextRe    = regexp.MustCompile(`^\s*(?:` + CodePat + `|\()`)

	breadthRe = regexp.MustCompile(`^(?i:` + AtLeastPat + `\s+)?` + AmountPat + `\s+(?i:(?:credits?\s+)?of\s+which\s+(?:should|must)\s+be\s+in\s+(?:the\s+)?)(?:[A-Z]{3}\s*/\s*)?BR\s*=\s*([1-5](?:\s*/\s*[1-5])*)`)

	scopePat = `(?:[A-Z]{2,}|[A-Z][a-z]+)(?:\s+(?:and\s+)?(?:[A-Z]{2,}|[A-Z][a-z]+|program|courses?|credits?))*`
	// gpaRe matches a GPA requirement: "Cumulative GPA of at least 2.70".
	gpaRe = regexp.MustCompile(`^(?i:(?:with\s+)?(?:an?\s+)?(?:(?:minimum|cumulative|overall)\s+(?:of\s+)?)*)?(?i:c?gpa)\s+(?i:of\s+)?(?i:` + AtLeastPat + `\s+)?` +
		`(\d\.\d+|[A-D][+-]?)(?:\s+(?i:or\s+(?:higher|above|better)))?(?:\s+(?i:required))?(?:\s+(?i:in|for)\s+(?i:all\s+)?(` + scopePat + `))?`)
	// gpaNumFirstRe matches a GPA written number first: "3.0 CGPA required".
	gpaNumFirstRe = regexp.MustCompile(`^(\d\.\d+)\s+(?i:c?gpa)(?:\s+(?i:required))?(?:\s+(?i:in|for)\s+(?i:all\s+)?(` + scopePat + `))?`)

	yearNumRe = regexp.MustCompile(`^(?i:(?:in\s+|enrolled\s+as\s+a\s+)?year\s+([1-4])(?:\s+(?:students?|standing|or\s+higher|or\s+above))*)\b`)
	yearOrdRe = regexp.MustCompile(`^(?i:(first|second|third|fourth|1st|2nd|3rd|4th)(?:\s*(?:and|or|/|,)\s*(?:first|second|third|fourth|1st|2nd|3rd|4th))*[- ]year\s+(?:students?|standing|status)(?:\s+only)?)\b`)
	ordinalRe = regexp.MustCompile(`(?i)first|second|third|fourth|1st|2nd|3rd|4th`)

	// creditHeadRe matches a credit phrase's head, whose prefixes may repeat or name
	// designators or a level: "Any CSC 0.5 credit".
	creditHeadRe = regexp.MustCompile(
		`^(?:(?i:(?:completion(?:\s+of)?|(?:(?:students\s+)?(?:who\s+|must\s+)?ha(?:ve|s)\s+)?(?:completed|taken)|(?:at\s+)?least|(?:a\s+)?(?:combined\s+)?minimum(?:\s+of)?|any|an\s+additional|additional|another|other|one|a\s+further|a\s+total\s+of)\s+)|` +
			`(?i:any\s+)[A-Z]{3}(?:\s*/\s*[A-Z]{3})*\s+|(?i:an?\s+)\d00\+?(?:\s*-?\s*(?i:level))?\s+)*` +
			AmountPat + `\s+` +
			`(?i:(?:additional|more|full|half|other)\s+)?` +
			`((?:(?:[A-Z]{3}\b|\d00(?:-level\b|\+)?)\s*(?:(?:/|or)\s*)?)*)` + // "HIS or POL ", "ENG ", "200-level CAS "
			`(?:BR\s*=?\s*(` + brNumsPat + `)\s+)?` + // "3.0 BR= 1/ 2 credits"
			`(?i:credits?|courses?|FCEs?)\b`)

	headPrefixDesigRe = regexp.MustCompile(`(?i:any)\s+([A-Z]{3}(?:\s*/\s*[A-Z]{3})*)\b`)

	// topicRe matches words naming a kind of course before "course": the
	// " literature course" of "ITA literature course".
	topicRe     = regexp.MustCompile(`^\s+([A-Za-z]+(?:(?:\s*/\s*|\s+)[A-Za-z]+){0,4})\s+(?i:courses?)\b`)
	topicStopRe = regexp.MustCompile(`^(?i:and|or|in|at|of|from|the|with|any|to|for|by|other|additional|credits?|courses?|levels?|including|plus|which|must|should|be|is|are|not|but|each|all|this|these|that|than|more|higher|above|their|program|programs)$|^an?$`) // "Group A" is a name
	// subjectParenRe matches designators spelling out a subject: " (PHY/ JPE)".
	subjectParenRe = regexp.MustCompile(`^\s*\(\s*([A-Z]{3}(?:\s*/\s*[A-Z]{3})*)\s*\)`)

	anyCourseHeadRe = regexp.MustCompile(`^(?i:(?:(?:a\s+)?minimum\s+of\s+|` + AtLeastPat + `\s+)?(?:a|an|any|one))\s+`)
	// fullCourseRe matches "full course" or "half-course equivalent".
	fullCourseRe = regexp.MustCompile(`^(?i:(full|half)[- ]course(?:\s+equivalent)?)\b`)
	levelLedRe   = regexp.MustCompile(`^\d00`)
	CourseWordRe = regexp.MustCompile(`(?i)\bcourses?\b`)

	// qualSpanRe matches a run of qualifiers. A connector must be followed by a
	// designator or level, so "2.0 ENG credits and any 4.0 credits" isn't one span.
	qualSpanRe = regexp.MustCompile(
		`^(?:\s*(?:(?i:in|at|the|from|of|any|additional)\b|(?i:-?\s*levels?)\b|(?i:courses?)\b|(?i:or\s+(?:higher|above))\b|` +
			qualItemPat + `|\d00\s*\+?|-|` +
			`(?:,\s*(?:(?i:or|and)|/)?|/|(?i:or)|(?i:and))\s*(?:` + qualItemPat + `|\d00)))*`)

	// Subject names and year words may stand in for designators and levels:
	// "first-year math, or physics courses".
	subjectPat     = `(?i:mathematics|math|physics|chemistry|biology|philosophy|economics|history|psychology|sociology|anthropology|astronomy|geography|linguistics|computer\s+science|political\s+science|peace\s+and\s+conflict|study\s+of\s+religion|religion|canadian\s+studies|jewish\s+studies)\b`
	yearWordPat    = `(?i:first|second|third|fourth)-year\b`
	qualItemPat    = `(?:[A-Z]{3}\b|` + subjectPat + `|` + yearWordPat + `)`
	SubjectRe      = regexp.MustCompile(subjectPat)
	subjectEndRe   = regexp.MustCompile(subjectPat + `\s*$`)
	desigSubjectRe = regexp.MustCompile(`\b[A-Z]{3}\s+(` + subjectPat + `)\s*$`)
	yearWordRe     = regexp.MustCompile(`(?i)(first|second|third|fourth)-year`)

	// trailingQualRe matches qualifier words ending a span, which belong to what
	// follows: the " in" of "any 300-level course in Society".
	trailingQualRe = regexp.MustCompile(`(?i)(?:\s*-|\s+(?:in|at|the|from|of|any))+\s*$`)

	ExcludingRe  = regexp.MustCompile(`^\s*\(\s*(?i:excluding|except)\s+([^)]*)\)|^\s*,?\s*(?i:excluding|except)\s+(` + CodePat + `(?:\s*(?:,|/|and|or)\s*` + CodePat + `)*)`)
	anyFieldRe   = regexp.MustCompile(`^\s*(?:\(\s*)?(?i:in\s+any\s+field)(?:\s*\))?`)
	DesigRe      = regexp.MustCompile(`\b[A-Z]{3}\b`)
	levelRe      = regexp.MustCompile(`([1-4])00\s*(\+)?`)
	orUpRe       = regexp.MustCompile(`(?i)or\s+(?:higher|above)`)
	breadthNumRe = regexp.MustCompile(`[1-5]`)
)

// yearCredits maps a year of study to the credits needed to be in it
// (Faculty of Arts & Science thresholds).
var yearCredits = map[int]float64{1: 0, 2: 4.0, 3: 9.0, 4: 14.0}

// SubjectDesignators maps subject names used in prose to designators.
var SubjectDesignators = map[string]string{
	"mathematics": "MAT", "math": "MAT", "physics": "PHY", "chemistry": "CHM",
	"biology": "BIO", "philosophy": "PHL", "economics": "ECO", "history": "HIS",
	"psychology": "PSY", "sociology": "SOC", "anthropology": "ANT", "astronomy": "AST",
	"geography": "GGR", "linguistics": "LIN", "computer science": "CSC",
	"political science": "POL", "peace and conflict": "PCS", "religion": "RLG",
	"study of religion": "RLG", "canadian studies": "CDN", "jewish studies": "CJS",
}

// countWords maps the count word of a pool ("two of") to a number.
var CountWords = map[string]int{
	"a": 1, "an": 1, "one": 1, "any": 1, "1": 1,
	"two": 2, "2": 2, "three": 3, "3": 3, "four": 4, "4": 4,
}

// letterGPA maps letter grades to the UofT 4.0 scale.
var letterGPA = map[string]float64{
	"A+": 4.0, "A": 4.0, "A-": 3.7, "B+": 3.3, "B": 3.0, "B-": 2.7,
	"C+": 2.3, "C": 2.0, "C-": 1.7, "D+": 1.3, "D": 1.0, "D-": 0.7,
}

// phraseMatchers in the order the lexer tries them. Earlier phrases protect
// "or"/"and" words from the operators; pools come before plain credit phrases.
var phraseMatchers = []phraseMatcher{
	matchCond,
	matchProgram,
	matchAverage, // before grade prefixes: "an average of at least 70% in ..."
	matchGradePrefix,
	matchCourse,
	matchGradeSuffix,
	matchAvgSuffix,
	matchGPA, // before pools: "a GPA of at least 3.5" is not "a ... of"
	MatchPool,
	matchBreadth,
	matchYear,
	matchCredits,
	matchAnyCourse,
}

// matchCourse reads a course code and a grade qualifier right after it:
// "STA247H1(70%)". A qualifier ending in "in each" covers more than this
// course, so it is left for the parser.
func matchCourse(s string) (Token, bool) {
	code := courseRe.FindString(s)
	if code == "" {
		return Token{}, false
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
	return Token{Kind: TCourse, End: end, Node: n}, true
}

func matchGradePrefix(s string) (Token, bool) {
	m := gradePrefixRe.FindStringSubmatch(s)
	if m == nil {
		return Token{}, false
	}
	g, _ := strconv.Atoi(m[1])
	return Token{Kind: tGradePrefix, End: len(m[0]), grade: g, each: m[2] != ""}, true
}

func matchCond(s string) (Token, bool) {
	if loc := condCommaRe.FindStringIndex(s); loc != nil && poolNextRe.MatchString(s[loc[1]:]) {
		return Token{Kind: TCond, End: loc[1]}, true
	}
	loc := condRe.FindStringIndex(s)
	if loc == nil || condLabelRe.MatchString(s) {
		return Token{}, false
	}
	// "Rotman Commerce:" only counts as a label when a course or group follows it.
	if !strings.HasPrefix(s, "For") && !poolNextRe.MatchString(s[loc[1]:]) {
		return Token{}, false
	}
	return Token{Kind: TCond, End: loc[1]}, true
}

func MatchPool(s string) (Token, bool) {
	if m := poolCountRe.FindStringSubmatch(s); m != nil {
		return Token{Kind: tPool, End: len(m[0]), count: CountWords[strings.ToLower(m[1])]}, true
	}
	if m := poolOfWhichCountRe.FindStringSubmatch(s); m != nil && poolNextRe.MatchString(s[len(m[0]):]) {
		return Token{Kind: tPool, End: len(m[0]), count: CountWords[strings.ToLower(m[1])]}, true
	}
	for _, re := range []*regexp.Regexp{poolFromRe, poolOfWhichRe} {
		m := re.FindStringSubmatchIndex(s)
		if m == nil || !poolNextRe.MatchString(s[m[1]:]) {
			continue // "1.0 credit from 200-level courses" is a credit phrase, not a pool
		}
		amount, _ := strconv.ParseFloat(s[m[2]:m[3]], 64)
		return Token{Kind: tPool, End: m[1], amount: amount}, true
	}
	return Token{}, false
}

func matchBreadth(s string) (Token, bool) {
	m := breadthRe.FindStringSubmatch(s)
	if m == nil {
		return Token{}, false
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	n := &types.ReqNode{Type: types.NodeCredits, Amount: amount}
	for _, b := range breadthNumRe.FindAllString(m[2], -1) {
		v, _ := strconv.Atoi(b)
		n.Breadth = append(n.Breadth, v)
	}
	return Token{Kind: tCredits, End: len(m[0]), Node: n}, true
}

func matchGPA(s string) (Token, bool) {
	m := gpaRe.FindStringSubmatch(s)
	if m == nil {
		if m = gpaNumFirstRe.FindStringSubmatch(s); m == nil {
			return Token{}, false
		}
	}
	gpa, ok := letterGPA[m[1]]
	if !ok {
		gpa, _ = strconv.ParseFloat(m[1], 64)
	}
	n := &types.ReqNode{Type: types.NodeGPA, MinGPA: gpa, Scope: m[2]}
	return Token{Kind: tGPA, End: len(m[0]), Node: n}, true
}

func matchYear(s string) (Token, bool) {
	year, end := 0, 0
	if m := yearNumRe.FindStringSubmatch(s); m != nil {
		year, _ = strconv.Atoi(m[1])
		end = len(m[0])
	} else if m := yearOrdRe.FindString(s); m != "" {
		year = 4
		for _, o := range ordinalRe.FindAllString(m, -1) {
			year = min(year, OrdinalYear(o))
		}
		end = len(m)
	} else if y, e, ok := matchYearIn(s); ok {
		year, end = y, e
	} else {
		return Token{}, false
	}
	n := &types.ReqNode{Type: types.NodeCredits, Amount: yearCredits[year]}
	return Token{Kind: tCredits, End: end, Node: n}, true
}

func OrdinalYear(o string) int {
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

func matchCredits(s string) (Token, bool) {
	// "no more than 14.0 credits": an upper bound (Max) instead of Amount.
	head, isMax := 0, false
	if loc := maxHeadRe.FindStringIndex(s); loc != nil {
		head, isMax = loc[1], true
	}
	// "two other 0.5 credit courses" = 1.0 credit.
	count := 1
	if m := countHeadRe.FindStringSubmatch(s[head:]); m != nil && !isMax {
		count = CountWords[strings.ToLower(m[1])]
		head += len(m[0]) - 1
	}
	m := creditHeadRe.FindStringSubmatchIndex(s[head:])
	if m == nil {
		return Token{}, false
	}
	for i := range m {
		if m[i] >= 0 {
			m[i] += head
		}
	}
	amount, _ := strconv.ParseFloat(s[m[2]:m[3]], 64)
	n := &types.ReqNode{Type: types.NodeCredits, Amount: amount * float64(count)}
	if isMax {
		n.MaxAmount, n.Amount = MaxAmount(n.Amount), 0
	}
	prefix, headQual := s[m[0]:m[2]], s[m[4]:m[5]]
	if d := headPrefixDesigRe.FindStringSubmatch(prefix); d != nil {
		n.Designators = DesigRe.FindAllString(d[1], -1) // "Any CSC 0.5 credit"
	}
	n.Designators = append(n.Designators, DesigRe.FindAllString(headQual, -1)...)
	if m[6] >= 0 {
		n.Breadth = breadthNums(s[m[6]:m[7]])
	}
	end := m[1]

	span := QualSpan(s[end:])
	if count > 1 && !CourseWordRe.MatchString(s[m[0]:end+len(span)]) {
		return Token{}, false // "two 0.5 credits" is not a phrase we know
	}
	n.Designators = append(n.Designators, spanDesignators(span)...)
	n.Levels = ParseLevels(prefix + " " + headQual + " " + span) // "a 300+ level 0.5 credit"
	end += len(span)
	var topic *types.ReqNode
	if !CourseWordRe.MatchString(span) && (len(n.Designators) > 0 || len(n.Levels) > 0) {
		var l int
		topic, l = courseTopic(s[end:], n) // "0.5 credit ITA literature course at the 300-level"
		end += l
	}
	end += subjectParen(s[m[0]:end], s[end:], n)

	end += creditSuffixes(s[end:], n)
	n.Designators = Dedupe(n.Designators)
	tok := Token{Kind: tCredits, End: end, Node: n}
	if topic != nil {
		tok.Node = &types.ReqNode{Type: types.NodeAnd, Items: []*types.ReqNode{n, topic}}
	} else if both, l := ofWhichSuffix(s[end:], n); both != nil {
		tok.Node, tok.End = both, end+l
	} else if both, l := averageSuffix(s[end:], n); both != nil {
		tok.Node, tok.End = both, end+l
	}
	return tok, true
}

// matchAnyCourse reads "Any AST 100-level or AST 200-level course": half a
// credit matching the designators and levels. "a full course in PHL" is 1.0,
// and without an article only a level-first "200-level History course" counts.
func matchAnyCourse(s string) (Token, bool) {
	start := 0
	if loc := anyCourseHeadRe.FindStringIndex(s); loc != nil {
		start = loc[1]
	} else if !levelLedRe.MatchString(s) {
		return Token{}, false
	}
	amount := 0.5
	full := fullCourseRe.FindStringSubmatch(s[start:])
	if full != nil {
		if start == 0 {
			return Token{}, false
		}
		if strings.EqualFold(full[1], "full") {
			amount = 1.0
		}
		start += len(full[0])
	}
	span := QualSpan(s[start:])
	if loc := desigSubjectRe.FindStringSubmatchIndex(span); loc != nil && full == nil && !CourseWordRe.MatchString(span) {
		// "300+ FRE Linguistics/Literature course": a subject name right
		// after a designator is a topic, not another designator.
		span = strings.TrimRight(span[:loc[2]], " ")
	}
	n := &types.ReqNode{Type: types.NodeCredits, Amount: amount}
	n.Designators = spanDesignators(span)
	n.Levels = ParseLevels(span)
	if len(n.Designators) == 0 && len(n.Levels) == 0 {
		return Token{}, false // a bare "a course" in prose
	}
	if start == 0 && (len(n.Designators) == 0 || !strings.HasSuffix(strings.ToLower(span), "course")) {
		// Without an article only "200-level History course" is safe: not
		// "200-level courses" (how many?) or the tail of a longer name
		// ("... Core Group 300+ level course").
		return Token{}, false
	}
	end := start + len(span)
	var topic *types.ReqNode
	if full == nil && !CourseWordRe.MatchString(span) {
		// "Any 300+ FRE Linguistics/Literature course": without a topic
		// before "course" this is not a course phrase.
		var l int
		if topic, l = courseTopic(s[end:], n); topic == nil {
			return Token{}, false
		}
		end += l
	}
	end += subjectParen(s[start:end], s[end:], n)
	n.Designators = Dedupe(n.Designators)
	end += creditSuffixes(s[end:], n)
	tok := Token{Kind: tCredits, End: end, Node: n}
	if topic != nil {
		tok.Node = &types.ReqNode{Type: types.NodeAnd, Items: []*types.ReqNode{n, topic}}
	}
	return tok, true
}

// courseTopic reads the words naming a kind of course after credit phrase n's
// qualifiers ("literature course") and any qualifiers after it, which it adds
// to n. The topic is returned as a text node beside n, or nil if there is none.
func courseTopic(s string, n *types.ReqNode) (*types.ReqNode, int) {
	m := topicRe.FindStringSubmatchIndex(s)
	if m == nil {
		return nil, 0
	}
	for _, w := range WordRe.FindAllString(s[m[2]:m[3]], -1) {
		if topicStopRe.MatchString(w) || DesigRe.MatchString(w) {
			return nil, 0 // not just topic words: "PHL or equivalent courses"
		}
	}
	text := strings.TrimSpace(s[:m[1]])
	end := m[1]
	span := QualSpan(s[end:])
	n.Designators = append(n.Designators, spanDesignators(span)...)
	n.Levels = Dedupe(append(n.Levels, ParseLevels(span)...))
	end += len(span)
	return &types.ReqNode{Type: types.NodeText, Kind: Classify(text), Text: text}, end
}

// subjectParen reads a list of designators in parentheses that spells out a
// subject named at the end of span ("in Physics (PHY/ JPE)"), adds them to n,
// and returns its length.
func subjectParen(span, s string, n *types.ReqNode) int {
	m := subjectParenRe.FindStringSubmatch(s)
	if m == nil || !subjectEndRe.MatchString(span) {
		return 0
	}
	n.Designators = append(n.Designators, DesigRe.FindAllString(m[1], -1)...)
	return len(m[0])
}

// spanDesignators returns the designators named in a qualifier span, either
// directly (HIS) or by subject name (history).
func spanDesignators(span string) []string {
	desigs := DesigRe.FindAllString(span, -1)
	for _, m := range SubjectRe.FindAllString(span, -1) {
		desigs = append(desigs, SubjectDesignators[strings.ToLower(WsRe.ReplaceAllString(m, " "))])
	}
	return desigs
}

// QualSpan returns the run of qualifier vocabulary at the start of s, minus
// any dangling qualifier words at its end.
func QualSpan(s string) string {
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
		} else if m := ExcludingRe.FindStringSubmatch(rest); m != nil {
			n.Exclude = append(n.Exclude, CodeRe.FindAllString(m[1]+m[2], -1)...)
			end += len(m[0])
		} else if m := anyFieldRe.FindString(rest); m != "" {
			end += len(m)
		} else {
			return end
		}
	}
}

// ParseLevels reads course levels from a qualifier span.
// "300-level" -> [300]; "300+ level" / "300-level or higher" -> [300 400];
// "300-/400-level", "300 or 400 level", "200-level or 300-level" -> listed levels.
func ParseLevels(span string) []int {
	var out []int
	up := orUpRe.MatchString(span)
	for _, m := range yearWordRe.FindAllStringSubmatch(span, -1) {
		out = append(out, OrdinalYear(m[1])*100)
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
	return Dedupe(out)
}

// MaxAmount returns a pointer to v, for ReqNode.MaxAmount (nil = no cap).
func MaxAmount(v float64) *float64 { return &v }

// dedupe removes repeated values, keeping first-seen order.
func Dedupe[T comparable](xs []T) []T {
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
