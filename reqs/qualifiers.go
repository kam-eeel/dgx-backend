package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Value-carrying qualifier wordings: minimum grades ("(minimum 70%)", "with a
// minimum mark of 73", "(B or higher)"), breadth categories ("in BR=1/ 2",
// "(SOC SCI/BR=3)"), letter-grade averages and year-of-study phrases.

var (
	// The body of a minimum-grade qualifier. Which words are present decides
	// whether it counts as a grade at all (see matchGradeBody).
	gradeBodyRe = regexp.MustCompile(`^(?P<with>(?i:with)\s+)?(?i:an?\s+)?` +
		`(?P<min>(?i:(?:minimum|final)\s+)+)?(?P<word>(?i:(?:grade|mark)\s+))?(?i:of\s+)?` +
		`(?P<least>(?i:at\s+least)\s+)?(?i:an?\s+)?` +
		`(?:(?P<pct>\d{2})\s*%|(?P<num>\d{2})|(?P<letter>[A-D][+-]?))` +
		`(?P<up>\s+(?i:or\s+(?:higher|above|better|greater))(?:\s+(?i:grade))?|\s+(?i:minimum|grade))?` +
		`(?P<each>\s+(?i:in\s+(?:each|all)(?:\s+(?:of\s+(?:these|the)\s+)?courses?)?))?`)
	parenOpenRe  = regexp.MustCompile(`^\s*\(\s*`)
	parenCloseRe = regexp.MustCompile(`^\s*\)`)
	gradeLeadRe  = regexp.MustCompile(`^\s*,?\s*`)
	wordCharRe   = regexp.MustCompile(`^[A-Za-z0-9]`)
	numCharRe    = regexp.MustCompile(`^(?:[A-Za-z0-9]|\.\d)`)
	averageRe    = regexp.MustCompile(`^\s+(?i:average)\b`)
	inWordRe     = regexp.MustCompile(`^\s*(?i:in)\b`)
	decimalRe    = regexp.MustCompile(`^\.\d`)

	// "with at least a B+ average" after a credit phrase.
	letterAverageRe = regexp.MustCompile(`^\s*,?\s*(?i:with\s+(?:at\s+least\s+)?(?:an?\s+)?)([A-D][+-]?)\s+(?i:average)\b`)

	// A list of breadth categories: "BR=1/ 2/ 3", "BR 1 or BR 2", "Breadth
	// Requirement 1 or 2", "breadth requirement (BR) category of 4 or 5",
	// "BR 1 or BR 2 or a combination of BR 1, BR 2, and BR 3".
	brNumsPat = `[1-5](?:\s*(?:/|,\s*(?i:and|or)?|(?i:or|and))\s*(?:BR\s*=?\s*)?[1-5])*`
	brListPat = `(?:BR\s*=?\s*|(?i:breadth\s+requirements?)(?:\s*\(\s*BR\s*\))?(?:\s+(?i:categor(?:y|ies)))?(?:\s+(?i:of))?\s+)` +
		`(` + brNumsPat + `)(?:\s*,?\s+(?i:or\s+a\s+combination\s+of)\s+(?:BR\s*=?\s*)?(` + brNumsPat + `))?\b`
	// A label naming the breadth in words is redundant with the number:
	// "Humanities/BR=1 or 2", "(SOC SCI/BR=3)".
	brLabelPat   = `(?:(?:[A-Z][A-Za-z]*|[A-Z]{3})(?:\s+(?:[A-Z][A-Za-z]*|[A-Z]{3}))?\s*/\s*)?`
	breadthSufRe = regexp.MustCompile(`^\s*(?:(?i:in|from|of)\s+|(?i:combination\s+of\s+courses\s+(?:carrying|with|in))\s+)?(?i:(?:either|an?|the)\s+)?` + brLabelPat + brListPat)
	breadthParRe = regexp.MustCompile(`^\s*\(\s*` + brLabelPat + brListPat + `\s*\)`)

	// "A student must be in third or fourth year", "Must be enrolled in 4th
	// year", "students in their third year of study".
	ordPat      = `(?:first|second|third|fourth|1st|2nd|3rd|4th)`
	yearInRe    = regexp.MustCompile(`^(?i:(?:(?:an?\s+)?students?\s+(?:must|should)\s+be\s+|must\s+be\s+|(?:available|open)\s+(?:only\s+)?to\s+students\s+)?(?:(?:enrolled|registered)\s+)?in\s+(?:their\s+|the\s+)?` + ordPat + `(?:\s*(?:and|or|/|,)\s*` + ordPat + `)*[- ]year(?:\s+of\s+(?:study|studies))?)\b`)
	yearEndRe   = regexp.MustCompile(`^\s*(?:$|[.,;()]|(?i:and|or)\b)`)
	maxHeadRe   = regexp.MustCompile(`^(?i:no\s+more\s+than|not\s+more\s+than|at\s+most|a\s+maximum\s+of|maximum\s+of|maximum|(?:students\s+)?(?:cannot|may\s+not|must\s+not)\s+have\s+(?:completed|taken)\s+more\s+than)\s+`)
	countHeadRe = regexp.MustCompile(`^(?i:(?:at\s+least\s+)?(two|three|four)\s+(?:other\s+|additional\s+|more\s+)?)\d`)
	completedRe = regexp.MustCompile(`^\s+(?i:completed)\b`)

	// A part of a credit requirement that must also match narrower filters:
	// ", of which 1.0 LIN credit must be at the 300+ level", ", 5.0 of which
	// must be GGR/URB", ", 1.0 credit of which should be in CHC/ SMC/ RLG",
	// "with 0.5 at the 300+ level", "with 1.5 credits in PHL".
	ofWhichRe = regexp.MustCompile(`^\s*,?\s*(?:(?i:of\s+which\s+(?:at\s+least\s+)?(?:an?\s+)?)(\d+\.\d+)|(?i:at\s+least\s+)?(\d+\.\d+)\s+(?i:(?:credits?\s+)?of\s+which)|(?i:with\s+(?:at\s+least\s+)?)(\d+\.\d+))` +
		`((?:\s+[A-Z]{3}\b(?:\s*/\s*[A-Z]{3}\b)*)?)(?:\s+(?i:credits?|courses?)\b)?` +
		`(?:\s+(?i:(?:must|should|needs?\s+to)\s+be|is|are))?(?:\s+(?i:an?)\b)?`)
	creditWordRe = regexp.MustCompile(`^\s*(?i:credits?|courses?)\b`)
)

// letterPercent maps a letter grade to the lowest percentage that earns it
// (UofT grading scale).
var letterPercent = map[string]int{
	"A+": 90, "A": 85, "A-": 80, "B+": 77, "B": 73, "B-": 70,
	"C+": 67, "C": 63, "C-": 60, "D+": 57, "D": 53, "D-": 50,
}

// matchGradeBody reads a minimum-grade qualifier at the start of s. In
// parentheses ("(minimum 70%)", "(B or higher)", "(67%)") a bare percentage is
// enough; otherwise the phrase must start with "with" and say what the number
// is ("with a minimum mark of 77", "with a mark of 76% or above"). A phrase
// followed by "in" ("with a minimum of 70% in DRM220Y1") is a grade prefix,
// not a suffix, so it is rejected.
func matchGradeBody(s string, paren bool) (grade int, each bool, end int, ok bool) {
	open := 0
	if paren {
		loc := parenOpenRe.FindStringIndex(s)
		if loc == nil {
			return 0, false, 0, false
		}
		open = loc[1]
	} else {
		loc := gradeLeadRe.FindStringIndex(s)
		if loc == nil {
			return 0, false, 0, false
		}
		open = loc[1]
	}
	m := gradeBodyRe.FindStringSubmatchIndex(s[open:])
	if m == nil {
		return 0, false, 0, false
	}
	group := func(name string) string {
		i := gradeBodyRe.SubexpIndex(name)
		if m[2*i] < 0 {
			return ""
		}
		return s[open+m[2*i] : open+m[2*i+1]]
	}
	end = open + m[1]
	rest := s[end:]
	qualified := group("min") != "" || group("word") != "" || group("least") != "" || group("up") != ""
	switch {
	case group("pct") != "":
		grade, _ = strconv.Atoi(group("pct"))
		ok = qualified || paren
	case group("num") != "":
		// "with a minimum mark of 77": only with "grade"/"mark", and not "70.5".
		grade, _ = strconv.Atoi(group("num"))
		ok = group("word") != "" && !numCharRe.MatchString(rest)
	default:
		grade = letterPercent[group("letter")]
		ok = qualified && !wordCharRe.MatchString(rest) && !averageRe.MatchString(rest)
	}
	if !paren && group("with") == "" {
		ok = false
	}
	each = group("each") != ""
	if paren {
		c := parenCloseRe.FindStringIndex(rest)
		if c == nil {
			return 0, false, 0, false
		}
		end += c[1]
	} else if !each && inWordRe.MatchString(rest) {
		ok = false
	}
	return grade, each, end, ok
}

// matchGradeSuffix lexes a grade qualifier that follows a group or list
// rather than a single course: "( A, B) with a minimum grade of 68% in each",
// "A + B with a minimum grade of 68% in each course".
func matchGradeSuffix(s string) (token, bool) {
	for _, paren := range []bool{true, false} {
		if g, each, end, ok := matchGradeBody(s, paren); ok {
			return token{kind: tGradeSuffix, end: end, grade: g, each: each}, true
		}
	}
	return token{}, false
}

// breadthSuffix reads a breadth qualifier after a credit phrase and returns
// its categories and length.
func breadthSuffix(s string) ([]int, int) {
	for _, re := range []*regexp.Regexp{breadthParRe, breadthSufRe} {
		t := s
		for {
			m := re.FindStringSubmatch(t)
			if m == nil {
				break
			}
			if decimalRe.MatchString(s[len(m[0]):]) {
				// "BR=2 and 1.0 credit from ...": the list ate the "1" of
				// the next amount; try again without it.
				t = s[:len(m[0])-1]
				continue
			}
			return breadthNums(m[1] + " " + m[2]), len(m[0])
		}
	}
	return nil, 0
}

func breadthNums(s string) []int {
	var out []int
	for _, b := range breadthNumRe.FindAllString(s, -1) {
		v, _ := strconv.Atoi(b)
		out = append(out, v)
	}
	return dedupe(out)
}

// averageSuffix turns "<credits> with at least a B+ average" into the credit
// requirement plus a GPA over courses with the same designators. It returns
// nil when the credits name no designators (the average would have no scope).
func averageSuffix(s string, n *types.ReqNode) (*types.ReqNode, int) {
	m := letterAverageRe.FindStringSubmatch(s)
	if m == nil || len(n.Designators) == 0 || wordCharRe.MatchString(s[len(m[0]):]) {
		return nil, 0
	}
	gpa := &types.ReqNode{Type: types.NodeGPA, Min: letterGPA[m[1]], Scope: strings.Join(n.Designators, " and ") + " courses"}
	return &types.ReqNode{Type: types.NodeAnd, Items: []*types.ReqNode{n, gpa}}, len(m[0])
}

// matchYearIn handles year standing said as "in <ordinal> year".
func matchYearIn(s string) (int, int, bool) {
	m := yearInRe.FindString(s)
	if m == "" || !yearEndRe.MatchString(s[len(m):]) {
		return 0, 0, false // "in first year biology"
	}
	year := 4
	for _, o := range ordinalRe.FindAllString(m, -1) {
		year = min(year, ordinalYear(o))
	}
	return year, len(m), true
}

// ofWhichSuffix reads a narrower part of the credit requirement n ("10.0
// credits, of which 2.0 credits must be 300+ level ESS courses") and returns
// n AND that part. The part counts only courses that also pass n's filters,
// so it inherits whatever filters it doesn't narrow itself.
func ofWhichSuffix(s string, n *types.ReqNode) (*types.ReqNode, int) {
	m := ofWhichRe.FindStringSubmatch(s)
	if m == nil {
		return nil, 0
	}
	amount, _ := strconv.ParseFloat(m[1]+m[2]+m[3], 64)
	part := &types.ReqNode{Type: types.NodeCredits, Amount: amount}
	part.Designators = desigRe.FindAllString(m[4], -1)
	end := len(m[0])
	if b, e := breadthSuffix(s[end:]); e > 0 { // "... of which should be in the HUM/BR=1/2"
		part.Breadth = b
		end += e
	} else {
		span := qualSpan(s[end:])
		part.Designators = append(part.Designators, spanDesignators(span)...)
		part.Levels = parseLevels(span)
		end += len(span)
		end += len(creditWordRe.FindString(s[end:])) // "... should be an HPS credit"
	}
	if part.Designators == nil && part.Levels == nil && part.Breadth == nil {
		return nil, 0 // nothing narrower: "of which at least 0.5 credits in A, B" is a pool
	}
	if n.Amount > 0 && amount > n.Amount {
		return nil, 0
	}
	var ok bool
	if part.Designators, ok = narrow(n.Designators, dedupe(part.Designators)); !ok {
		return nil, 0
	}
	if part.Levels, ok = narrow(n.Levels, part.Levels); !ok {
		return nil, 0
	}
	if part.Breadth, ok = narrow(n.Breadth, part.Breadth); !ok {
		return nil, 0
	}
	part.Courses, part.Exclude, part.MinGrade = n.Courses, n.Exclude, n.MinGrade
	return &types.ReqNode{Type: types.NodeAnd, Items: []*types.ReqNode{n, part}}, end
}

// narrow combines a parent filter with a child filter: the child's values if
// the parent has none, the parent's if the child has none, else the values in
// both (ok is false if there are none).
func narrow[T comparable](parent, child []T) ([]T, bool) {
	switch {
	case len(child) == 0:
		return parent, true
	case len(parent) == 0:
		return child, true
	}
	var out []T
	for _, c := range child {
		if slices.Contains(parent, c) {
			out = append(out, c)
		}
	}
	return out, out != nil
}
