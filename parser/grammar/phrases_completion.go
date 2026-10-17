package grammar

import (
	"regexp"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Phrases only completion text has: amount ranges, "excluding", "either",
// "All of:", described pools and wildcard series.

// completionMatchers tries the completion phrases first, then the shared ones.
var completionMatchers = append([]phraseMatcher{
	matchRange,
	matchExcluding,
	matchEither,
	matchAllOf,
	matchCompletionPool,
	matchDescribedPool,
	matchWild,
}, phraseMatchers...)

var (
	// complAmountRangeRe matches a range of amounts before a pool or a credit
	// phrase: "2.0 to 2.5 credits".
	complAmountRangeRe = regexp.MustCompile(`^` + AmountPat + `\s*(?:-|–|(?i:to|or)\b)\s*` + AmountPat + `\s`)

	// eitherRe matches "either" before alternatives: "Either GGR491Y1 or ...".
	eitherRe     = regexp.MustCompile(`^(?i:either)\b(?:\s*:)?`)
	eitherNextRe = regexp.MustCompile(`^\s*(?:` + CodePat + `|\(|\d+\.\d|(?i:one|two|three|any|all)\s+\w|an?\s+\w)`)

	// allOfRe matches "All of:", whose commas already mean "and".
	allOfRe = regexp.MustCompile(`^(?i:all\s+of(?:\s+the\s+following(?:\s+courses)?)?)(?:\s*:)?`)

	// poolAnyCoursesRe matches "Any 2 courses (1.0 credit) from": the credits in
	// brackets are the pool's amount.
	poolAnyCoursesRe = regexp.MustCompile(`^(?i:any\s+(?:(?:\d+|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve)\s+)?courses?\s*\(\s*` + AmountPat + `\s+credits?\s*\)\s+(?:from|of)\b(?:\s+the\s+following)?)(?:\s*:)?`)
	// poolDescRe matches a pool whose description has commas, unlike poolFromRe:
	// "2.5 credits from electives in music history, culture, and theory:".
	// Digits and brackets would be conditions.
	poolDescRe = regexp.MustCompile(`^(?i:` + AtLeastPat + `\s+)?(\d+\.\d+)\s+(?i:credits?\s+(?:from|in)\b)[^:;.()\d]{1,150}:`)
	// poolLevelRe matches a pool whose level narrows every member: "0.5 credit
	// at the 400-level from".
	poolLevelRe = regexp.MustCompile(`^(?i:` + AtLeastPat + `\s+)?(\d+\.\d+)\s+(?i:credits?\s+at\s+(?:the\s+)?)(` + LevelPat + `(?:\s*\+)?(?:\s*-?\s*(?i:level))?(?:\s+(?i:or\s+(?:higher|above)))?)\s+(?i:from)\b(?:\s*:)?`)
	// complPoolDescribedRe matches a pool whose list follows a description of
	// it: "2.5 credits of core courses:".
	complPoolDescribedRe = regexp.MustCompile(`^(?i:` + AtLeastPat + `\s+)?(\d+\.\d+)\s+(?i:credits?)\b([^:;.()]{0,100}?)(?:\s+(?i:selected|chosen))?(?:\s+(?i:from|among)\b(?:\s+(?i:the\s+following(?:\s+list)?))?\s*:?|(\s*:))`)
	// complPoolOfCodesRe matches a pool when codes follow: "1.0 credit of ".
	complPoolOfCodesRe = regexp.MustCompile(`^(\d+\.\d+)\s+(?i:credits?\s+of)\s+`)
	// complPoolDescStopRe matches words that make a description a condition,
	// not a name: "including", or "and" for a second pool.
	complPoolDescStopRe = regexp.MustCompile(`(?i)\b(?:including|include|with|which|must|should|may|can|least|than|per|each|and|or|to|total|any)\b`)
	// complPoolOneListRe matches one of: "One course from the following list:".
	complPoolOneListRe = regexp.MustCompile(`^(?i:(?:one|a)\s+course\s+from\s+the\s+following\s+list)\s*:?`)
	// poolAnyCountRe matches "any two of:".
	poolAnyCountRe = regexp.MustCompile(`^(?i:any\s+(two|three|four|[2-4])\s+of\b(?:\s+the\s+following)?)(?:\s*:)?`)

	// wildRe matches a pool member named by designator and level:
	// "PSL400-series courses".
	wildRe = regexp.MustCompile(`^((?i:any\s+)?)([A-Z]{3}(?:\s*/\s*[A-Z]{3})*)\s?(` + LevelPat + `(?:\s*\+)?)((?:\s*-?\s*(?i:series|level))*)(\s+(?i:courses?)\b)?`)
	// wildDesigRe matches a bare designator as a pool member: "EEB".
	wildDesigRe   = regexp.MustCompile(`^([A-Z]{3}(?:\s*/\s*[A-Z]{3})*)\b(\s+(?i:courses?)\b)?`)
	wildEndRe     = regexp.MustCompile(`^\s*(?:$|[;,/()]|(?i:or|and)\b)`)
	wildStopRe    = regexp.MustCompile(`^(?:AND|THE|ANY|ALL|NOT|NOR|BUT|ARE|MAY|ONE|TWO|ITS|HAS|CAN|SEE|USE)\b`)
	wildSingleRe  = regexp.MustCompile(`(?i)^any\b|\bcourse$`)
	anyPoolItemRe = regexp.MustCompile(`^(?i:any|an?|one)\s+(?:\d00|[A-Z]{3}\b)`)
)

// matchRange reads a range of amounts before a pool or a credit phrase. The
// requirement is the lower end, the upper stays in raw; a pool keeps it in
// token.upper so groups adding up within the range read as their "and".
func matchRange(s string) (Token, bool) {
	m := complAmountRangeRe.FindStringSubmatchIndex(s)
	if m == nil {
		return Token{}, false
	}
	lo, _ := strconv.ParseFloat(s[m[2]:m[3]], 64)
	hi, _ := strconv.ParseFloat(s[m[4]:m[5]], 64)
	if hi <= lo {
		return Token{}, false
	}
	rest := s[m[4]:]
	for _, match := range []phraseMatcher{matchCompletionPool, matchDescribedPool, MatchPool, matchCredits} {
		t, ok := match(rest)
		switch {
		case !ok:
			continue
		case t.Kind == tPool && t.amount == hi:
			t.amount, t.upper = lo, hi
		case t.Kind == tCredits && t.Node.Type == types.NodeCredits && t.Node.Amount == hi && t.Node.MaxAmount == nil:
			t.Node.Amount = lo
		default:
			continue
		}
		t.End += m[4]
		return t, true
	}
	return Token{}, false
}

// matchExcluding reads an exclusion after a list: ", excluding JPH311H1,
// JPH441H1". The parser applies the codes to the pool before it.
func matchExcluding(s string) (Token, bool) {
	m := excludingListRe.FindStringSubmatch(s)
	if m == nil {
		return Token{}, false
	}
	return Token{Kind: tExclude, End: len(m[0]), codes: CodeRe.FindAllString(m[1], -1)}, true
}

var excludingListRe = regexp.MustCompile(`^[.,]?\s*(?i:excludes|excluding|except)\s+(` + CodePat + `(?:(?:\s*,\s*(?:(?i:and|or)\s+)?|\s*/\s*|\s+(?i:and|or)\s+)` + CodePat + `)*)\.?`)

func matchEither(s string) (Token, bool) {
	loc := eitherRe.FindStringIndex(s)
	if loc == nil || !eitherNextRe.MatchString(s[loc[1]:]) {
		return Token{}, false
	}
	return Token{Kind: tEither, End: loc[1]}, true
}

func matchAllOf(s string) (Token, bool) {
	loc := allOfRe.FindStringIndex(s)
	if loc == nil || !poolNextRe.MatchString(s[loc[1]:]) {
		return Token{}, false
	}
	return Token{Kind: tSkip, End: loc[1]}, true
}

func matchCompletionPool(s string) (Token, bool) {
	if m := poolAnyCoursesRe.FindStringSubmatchIndex(s); m != nil {
		amount, _ := strconv.ParseFloat(s[m[2]:m[3]], 64)
		return Token{Kind: tPool, End: m[1], amount: amount}, true
	}
	if m := complPoolOneListRe.FindString(s); m != "" && poolNextRe.MatchString(s[len(m):]) {
		return Token{Kind: tPool, End: len(m), count: 1}, true
	}
	if m := poolAnyCountRe.FindStringSubmatch(s); m != nil && poolNextRe.MatchString(s[len(m[0]):]) {
		return Token{Kind: tPool, End: len(m[0]), count: CountWords[strings.ToLower(m[1])]}, true
	}
	// a pool whose first member is a wildcard: "2.0 credits from: ANA; ...". A
	// bare designator starts a list only after a colon: "in GRK or LAT" isn't one
	for _, re := range []*regexp.Regexp{poolFromRe, poolDescRe, poolLevelRe} {
		m := re.FindStringSubmatchIndex(s)
		if m == nil {
			continue
		}
		rest := strings.TrimLeft(s[m[1]:], " ")
		if w, wild := matchWild(rest); !poolNextRe.MatchString(rest) && !anyCourseMember(rest) &&
			!(wild && (w.Node.Levels != nil || strings.HasSuffix(s[:m[1]], ":"))) {
			continue
		}
		amount, _ := strconv.ParseFloat(s[m[2]:m[3]], 64)
		t := Token{Kind: tPool, End: m[1], amount: amount}
		if re == poolLevelRe {
			t.Node = &types.ReqNode{Type: types.NodeCredits, Levels: ParseLevels(s[m[4]:m[5]])}
		}
		return t, true
	}
	return Token{}, false
}

// matchDescribedPool reads a pool whose list follows a description of it. A
// level in the description ("300+ ECO elective courses") narrows every member.
func matchDescribedPool(s string) (Token, bool) {
	if m := complPoolOfCodesRe.FindStringSubmatchIndex(s); m != nil && poolNextRe.MatchString(s[m[1]:]) {
		amount, _ := strconv.ParseFloat(s[m[2]:m[3]], 64)
		return Token{Kind: tPool, End: m[1], amount: amount}, true
	}
	m := complPoolDescribedRe.FindStringSubmatchIndex(s)
	if m == nil || complPoolDescStopRe.MatchString(poolTopicOr(s[m[4]:m[5]])) {
		return Token{}, false
	}
	rest := strings.TrimLeft(s[m[1]:], " ")
	if w, wild := matchWild(rest); !poolNextRe.MatchString(rest) && !anyCourseMember(rest) && !(wild && w.Node.Levels != nil) {
		return Token{}, false
	}
	amount, _ := strconv.ParseFloat(s[m[2]:m[3]], 64)
	// "2.0 credits: A; B/ C; D" names no pool: a list adding up to the amount
	// is all needed
	t := Token{Kind: tPool, End: m[1], amount: amount, sum: m[6] >= 0 && strings.TrimSpace(s[m[4]:m[5]]) == ""}
	if levels := ParseLevels(s[m[4]:m[5]]); levels != nil {
		t.Node = &types.ReqNode{Type: types.NodeCredits, Levels: levels}
	}
	return t, true
}

// anyCourseMember reports whether s starts with one course by filter, as a
// pool member: "a 400-level PSL course", which normalize makes of
// "PSL400-series".
func anyCourseMember(s string) bool {
	t, ok := matchAnyCourse(s)
	return ok && isFilter(t.Node) && anyPoolItemRe.MatchString(s[:t.End])
}

// matchWild reads a pool member named by designator and level, or designator
// alone, as half a credit matching those filters. A pool turns it into a
// filter; elsewhere only singular forms ("any ETH200+") are requirements.
func matchWild(s string) (Token, bool) {
	if wildStopRe.MatchString(strings.TrimPrefix(strings.TrimPrefix(s, "any "), "Any ")) {
		return Token{}, false // "ANY 300-level literature courses"
	}
	n := &types.ReqNode{Type: types.NodeCredits, Amount: 0.5}
	end, bare := 0, false
	if m := wildRe.FindStringSubmatchIndex(s); m != nil &&
		(strings.Contains(s[m[6]:m[7]], "+") || m[9] > m[8] || m[10] >= 0) { // "+", "-series"/"-level" or "courses"
		n.Designators = DesigRe.FindAllString(s[m[4]:m[5]], -1)
		n.Levels = ParseLevels(s[m[6]:m[7]])
		end = m[1]
	} else if m := wildDesigRe.FindStringSubmatchIndex(s); m != nil {
		n.Designators = DesigRe.FindAllString(s[m[2]:m[3]], -1)
		end, bare = m[1], true
	} else {
		return Token{}, false
	}
	if e := ExcludingRe.FindStringSubmatch(s[end:]); e != nil {
		n.Exclude = CodeRe.FindAllString(e[1]+e[2], -1)
		end += len(e[0])
	}
	if bare && !wildEndRe.MatchString(s[end:]) {
		return Token{}, false // a word in prose, not a list member
	}
	return Token{Kind: TWild, End: end, Node: n}, true
}

// complPoolTopicOrRe matches a topic of two subjects ending a pool's
// description: "in plant or microbial biology".
var complPoolTopicOrRe = regexp.MustCompile(`\bin\s+[a-z]+\s+or\s+[a-z]+(?:\s+[a-z]+)?\s*$`)

// poolTopicOr is a pool's description without a topic of two subjects,
// whose "or" is no second pool.
func poolTopicOr(desc string) string {
	return complPoolTopicOrRe.ReplaceAllString(desc, "")
}
