package grammar

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Averages over a set of courses ("a 77% average in 3.0 HIS credits"). A grade
// "in" courses is a per-course minimum, not an average.

var (
	avgGradePat = `(?:(\d{2})\s*%|([A-D][+-]?))`
	avgUpPat    = `(?:\s+(?i:or\s+(?:higher|above|better)))?`
	// avgHeadRe matches the phrase before the courses: "a 77% average in".
	avgHeadRe = regexp.MustCompile(`^(?i:(?:an?\s+)?(?:(?:minimum|overall|cumulative|combined)\s+(?:of\s+)?)*)(?:` +
		avgGradePat + `\s+(?i:average\s+(?:grade\s+|mark\s+)?(?:in|over|across|for))` +
		`|(?i:average\s+(?:grade\s+|mark\s+)?(?:of\s+)?(?:` + AtLeastPat + `\s+)?)` + avgGradePat + avgUpPat + `\s+(?i:in|over|across|for)` +
		`|(\d{2})\s*%\s+(?i:over|across))\b`)
	// avgAllRe, avgCoursesEndRe and avgTakenRe read courses named without an
	// amount: "all ECO courses taken".
	avgAllRe        = regexp.MustCompile(`^(?i:(?:all\s+)?(?:(?:of\s+)?(?:the|their)\s+)?)`)
	avgCoursesEndRe = regexp.MustCompile(`(?i)\bcourses?$`)
	avgTakenRe      = regexp.MustCompile(`^(?:\s*(?i:credits?)\b)?(?:\s+(?i:taken|completed)\b)?`)

	// avgSuffixRe matches an average after a credit phrase: "with a 77% average".
	avgSuffixRe = regexp.MustCompile(`^\s*,?\s*(?i:with\s+(?:` + AtLeastPat + `\s+)?(?:an?\s+)?(?:(?:minimum|overall|combined)\s+)*)(?:` +
		avgGradePat + `\s+(?i:average)` +
		`|(?i:average\s+(?:grade\s+|mark\s+)?(?:of\s+)?(?:` + AtLeastPat + `\s+)?)` + avgGradePat + avgUpPat + `)`)

	// avgOfBody matches an average over the group just before it: "(average of
	// both must be 80%)".
	avgOfBody = `(?i:(?:an?\s+)?(?:(?:combined|overall|minimum)\s+)*average\s+(?:grade\s+|mark\s+)?)(?:` +
		`(?i:of|in|across|for)\s+(?i:(?:the\s+)?(both|these|the\s+two)(?:\s+courses)?\s+(?:must|should)\s+be\s+(?:at\s+least\s+)?)(\d{2})\s*%` + avgUpPat +
		`|(?i:of\s+(?:at\s+least\s+)?)(\d{2})\s*%` + avgUpPat + `\s+(?i:(?:in|across|for)\s+(?:the\s+)?(both|these|the\s+two)(?:\s+courses)?))`
	avgOfParenRe = regexp.MustCompile(`^\(\s*` + avgOfBody + `\s*\)`)
	avgOfWithRe  = regexp.MustCompile(`^\s*,?\s*(?i:with)\s+` + avgOfBody)
)

// avgGrade reads the percentage or letter grade in the first matching pair of
// (percent, letter) groups of m, starting at group i.
func avgGrade(m []string, i int) int {
	for ; i < len(m); i++ {
		if m[i] == "" {
			continue
		}
		if g, err := strconv.Atoi(m[i]); err == nil {
			return g
		}
		return LetterPercent[m[i]]
	}
	return 0
}

// matchAverage reads an average phrase with the courses it is over: a credit
// phrase ("3.0 HIS credits") or courses with no amount ("all ECO courses
// taken"). It doesn't match if the courses can't be read.
func matchAverage(s string) (Token, bool) {
	m := avgHeadRe.FindStringSubmatch(s)
	if m == nil {
		return Token{}, false
	}
	grade := avgGrade(m, 1)
	end := skipSpace(s, len(m[0]))
	if t, ok := matchCredits(s[end:]); ok {
		if n := toAverage(t.Node, grade); n != nil {
			return Token{Kind: tCredits, End: end + t.End, Node: n}, true
		}
		return Token{}, false
	}
	// No amount: "all ECO courses taken".
	end += len(avgAllRe.FindString(s[end:]))
	span := QualSpan(s[end:])
	if !avgCoursesEndRe.MatchString(span) && !creditWordRe.MatchString(s[end+len(span):]) {
		return Token{}, false
	}
	c := &types.ReqNode{Type: types.NodeCredits}
	c.Designators = Dedupe(spanDesignators(span))
	c.Levels = ParseLevels(span)
	end += len(span)
	end += len(avgTakenRe.FindString(s[end:]))
	end += creditSuffixes(s[end:], c)
	n := toAverage(c, grade)
	if n == nil {
		return Token{}, false
	}
	return Token{Kind: tCredits, End: end, Node: n}, true
}

// toAverage turns credit requirement c into an average of grade over the same
// courses, or nil unless c is a credit filter with no grade or upper bound of
// its own.
func toAverage(c *types.ReqNode, grade int) *types.ReqNode {
	if c == nil || c.Type != types.NodeCredits || grade == 0 || c.MinGrade > 0 || c.MaxAmount != nil ||
		len(c.Designators) == 0 && len(c.Levels) == 0 && len(c.Breadth) == 0 && len(c.Courses) == 0 {
		return nil
	}
	return &types.ReqNode{Type: types.NodeAverage, MinGrade: grade, Amount: c.Amount,
		Designators: c.Designators, Levels: c.Levels, Breadth: c.Breadth, Courses: c.Courses, Exclude: c.Exclude}
}

// averageSuffix turns "<credits> with at least a B+ average" into an average
// over the same courses. Having that many credits is part of the average's
// meaning (Amount), so no separate credit node is needed.
func averageSuffix(s string, n *types.ReqNode) (*types.ReqNode, int) {
	m := avgSuffixRe.FindStringSubmatch(s)
	if m == nil || wordCharRe.MatchString(s[len(m[0]):]) {
		return nil, 0
	}
	avg := toAverage(n, avgGrade(m, 1))
	if avg == nil {
		return nil, 0
	}
	return avg, len(m[0])
}

// matchAvgSuffix lexes an average over the group before it. The parser
// decides whether that group is unambiguous.
func matchAvgSuffix(s string) (Token, bool) {
	for _, re := range []*regexp.Regexp{avgOfParenRe, avgOfWithRe} {
		m := re.FindStringSubmatch(s)
		if m == nil || wordCharRe.MatchString(s[len(m[0]):]) {
			continue
		}
		which, grade := strings.ToLower(m[1]+m[4]), avgGrade(m, 2)
		count := 0
		if which != "these" {
			count = 2 // "both", "the two"
		}
		return Token{Kind: tAvgSuffix, End: len(m[0]), grade: grade, count: count}, true
	}
	return Token{}, false
}

// applyAvgSuffix adds an average over the courses of n, which must be an "and"
// of courses ("( A, B) (average of both must be 80%)"); anything else leaves
// the phrase as text.
func (p *parser) applyAvgSuffix(n *types.ReqNode, t Token) *types.ReqNode {
	var codes []string
	if n != nil && n.Type == types.NodeAnd && !slices.ContainsFunc(n.Items, func(it *types.ReqNode) bool { return it.Type != types.NodeCourse }) {
		for _, it := range n.Items {
			codes = append(codes, it.Code)
		}
	}
	if len(codes) < 2 || t.count > 0 && len(codes) != t.count {
		return p.avgText(t)
	}
	avg := &types.ReqNode{Type: types.NodeAverage, MinGrade: t.grade, Courses: codes}
	p.Spans[avg] = Span{t.Start, t.End}
	return avg
}

// avgText keeps an average suffix as text, without its parentheses.
func (p *parser) avgText(t Token) *types.ReqNode {
	n := p.text("", t)
	n.Text = strings.Trim(n.Text, "() ,")
	return n
}
