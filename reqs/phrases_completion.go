package reqs

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Grammar extensions for program completion text: only lexCompletion and
// parseCompletionText use them, so requisites and enrolment never see them.

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
	complAmountRangeRe = regexp.MustCompile(`^` + amountPat + `\s*(?:-|–|(?i:to|or)\b)\s*` + amountPat + `\s`)

	// eitherRe matches "either" before alternatives: "Either GGR491Y1 or ...".
	eitherRe     = regexp.MustCompile(`^(?i:either)\b(?:\s*:)?`)
	eitherNextRe = regexp.MustCompile(`^\s*(?:` + codePat + `|\(|\d+\.\d|(?i:one|two|three|any|all)\s+\w|an?\s+\w)`)

	// complWithAtLeastRe matches a floor read as "including": ", with at least
	// a 0.5 credit".
	complWithAtLeastRe = regexp.MustCompile(`(?i),\s*with\s+` + atLeastPat + `\s+(?:an?\s+)?` + amountPat + `\s+credits?\b`)
	// complDoubledFromRe matches "from from" (sic); complFurtherRe matches
	// "credits further from".
	complDoubledFromRe = regexp.MustCompile(`(?i)\bfrom\s+from\b`)
	complFurtherRe     = regexp.MustCompile(`(?i)\b(credits?)\s+further\s+(from|of)\b`)
	// complCodePairRe matches a list missing its comma: "SLA322H1 SLA410H1" (sic).
	complCodePairRe = regexp.MustCompile(`(` + codePat + `|\b[A-Z]{3}\d{4}[HY]\d?)\s+(` + codePat + `|[A-Z]{3}\d{4}[HY]\d?\b)`)
	// complAmountAndRe matches an amount over courses joined by "and": "1.0
	// credit from GGR217H1 and JGI346H1/ JGU346H1".
	complAmountAndRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+(?:(?:of|in)\s+[^:]*:|from:?|[^:]*:)\s*((?:` + codePat + `\s*/\s*)*` + codePat + `)\s+and\s+((?:` + codePat + `\s*/\s*)*` + codePat + `)\.?$`)
	// complTitledAmountRe matches a titled amount over a list, the title dropped:
	// "A set of mandatory courses (3.5 credits): ".
	complTitledAmountRe = regexp.MustCompile(`^(?:[A-Z][^:()]{0,60}?)\s*\(\s*` + amountPat + `\s+credits?\s*\)\s*:\s*`)
	// complCodedCoursesRe matches "PCJ-coded courses", read "PCJ courses".
	complCodedCoursesRe = regexp.MustCompile(`\b([A-Z]{3})-coded\s+courses\b`)
	// complDescParenRe matches a lowercase description of the course before it:
	// "EEB240H1 (plant or animal form and function/environmental biology)".
	complDescParenRe = regexp.MustCompile(`(` + codePat + `)\s*\(\s*[a-z][a-z ,/&-]*\)`)
	// complOneCreditRe matches "including one credit", read "1.0 credit".
	complOneCreditRe = regexp.MustCompile(`(?i)(\b(?:including|of\s+which|with)\s+(?:` + atLeastPat + `\s+)?)one\s+credit\b`)
	// complAtCampusRe matches a campus the code already says: "ANT418H5 at UTM".
	complAtCampusRe = regexp.MustCompile(`(` + codePat + `)\s*(?:\(\s*)?(?:at|from)\s+(?:UTM|UTSC)\b\)?`)
	// complOnTheLevelRe matches "on the 300-level", read "at the 300-level".
	complOnTheLevelRe = regexp.MustCompile(`(?i)\bon\s+the\s+(` + levelPat + `)[-\s]level\b`)
	// complOneOrBothRe matches either course: "one or both of ( MGY481H1 and
	// MGY482H1)".
	complOneOrBothRe = regexp.MustCompile(`(?i)\bone\s+or\s+both\s+of\s+\(\s*(` + codePat + `)\s+and\s+(` + codePat + `)\s*\)`)
	// complGradeParenRe matches a grade after a course: "(minimum grade of 63%)".
	complGradeParenRe = regexp.MustCompile(`(?i)\(\s*(?:a\s+)?(?:minimum|min\.?)\s+(?:final\s+)?(?:grade|mark)\s+of\s+(?:` + atLeastPat + `\s+)?(\d{2})\s*%\s*\)`)
	// complCodeParenCodeRe matches the course or the one in brackets: "CHM136H1
	// ( CHM138H1)".
	complCodeParenCodeRe = regexp.MustCompile(`(` + codePat + `)\s*\(\s*(` + codePat + `)\s*\)`)
	// complQuotedTitleRe matches a title in quotes: "GER290H1 ‘Global Issues /
	// German Contexts’".
	complQuotedTitleRe = regexp.MustCompile(`(` + codePat + `)\s*‘[^’]*’`)
	// complExcludingAndRe matches the last excluded course: "excluding GER150H1,
	// GER361H1, and GER410H1".
	complExcludingAndRe = regexp.MustCompile(`(?i)(\bexcluding\s+(?:` + codePat + `\s*,\s*)+)and\s+(` + codePat + `)`)
	// complSubjectDesigRe matches subjects' designators: "in music history (HMU)".
	complSubjectDesigRe = regexp.MustCompile(`\b(in|or)\s+(?:[a-z]+\s+){1,3}\(\s*([A-Z]{3})\s*\)`)
	// complOrDesigParenRe matches either designator in "MAT (or APM) courses",
	// which normalize writes "MAT or (APM)".
	complOrDesigParenRe = regexp.MustCompile(`\b([A-Z]{3})\s+(?i:or)\s+\(\s*([A-Z]{3})\s*\)`)
	// complInAdditionRe matches a course required elsewhere that doesn't count
	// here too: "(in addition to APM462H1)".
	complInAdditionRe = regexp.MustCompile(`(?i)\s*\(\s*in\s+addition\s+to\s+((?:` + codePat + `(?:\s*(?:,|and|/)\s*)?)+)\s*\)`)
	// complCapDesigRe matches a cap on designators: "no more than 0.5 credit can
	// be IRE/IRW".
	complCapDesigRe = regexp.MustCompile(`(?i)(` + noMoreThanPat + `\s+` + numPat + `)\s+credits?\s+(?:can|may)\s+be\s+(?:in\s+)?(` + desigPat + `(?:\s*/\s*` + desigPat + `)*)\b`)
	// complNormallyTakenRe matches timing, a note: ", normally taken in the first
	// year".
	complNormallyTakenRe = regexp.MustCompile(`(?i),\s*((?:normally|usually|typically|(?:(?:highly|strongly)\s+)?recommended\s+to\s+be)\s+taken\s+in\s+(?:the\s+)?(?:first|second|third|fourth)\s+year)\s*[;.]?\s*$`)
	// complCodeTitleRe matches a course's title after a colon: "HIS268H1: Law
	// and History".
	complCodeTitleRe = regexp.MustCompile(`^(` + codePat + `)\s*:\s+[A-Z][A-Za-z ,&'’-]*\.?$`)
	// complArticleAmountRe matches the article in "or a 0.5 credit".
	complArticleAmountRe = regexp.MustCompile(`(?i)\b(or|and)\s+an?\s+(\d+\.\d+\s+credits?)\b`)
	// complBreadthNameRe matches a breadth category's name after its number:
	// "Category 5: The Physical and Mathematical Universes".
	complBreadthNameRe = regexp.MustCompile(`(?i)(\bcategor(?:y|ies)\s+[1-5])\s*[:(]\s*(?:creative\s+and\s+cultural\s+representations|thought,\s+belief,?\s+and\s+behaviou?r|society\s+and\s+its\s+institutions|living\s+things\s+and\s+their\s+environment|the\s+physical\s+(?:and|or)\s+mathematical\s+universes?)\)?`)
	// complGradePlusRe matches a grade with a plus: "(80%+)".
	complGradePlusRe = regexp.MustCompile(`\(\s*(\d{2})%\s*\+\s*\)`)
	// complWholeAmountRe matches a whole-number amount: "3 credits".
	complWholeAmountRe = regexp.MustCompile(`(^|[\s(])(\d{1,2})(\s+credits?\b)`)
	// complMustBeFromRe matches a pool: "1.0 credit must be taken from".
	complMustBeFromRe = regexp.MustCompile(`(?i)^(\d+\.\d+\s+credits?)\s+(?:must\s+be\s+(?:taken|chosen|selected)|chosen|selected)\s+from\b`)
	// complIeRe matches the text before restated, a note: ", i.e., 2.0 SOC
	// credits at the 200 OR 300-level".
	complIeRe = regexp.MustCompile(`(?i),?\s*\b(i\.e\.,?\s+[^;]*)$`)

	// eitherDesigRe matches a credit phrase's designators, not alternatives: "in
	// either GRK or LAT".
	eitherDesigRe = regexp.MustCompile(`\b((?i:in|from))\s+(?i:either)\s+([A-Z]{3}\s+(?i:or)\s+[A-Z]{3})\b`)

	// allOfRe matches "All of:", whose commas already mean "and".
	allOfRe = regexp.MustCompile(`^(?i:all\s+of(?:\s+the\s+following(?:\s+courses)?)?)(?:\s*:)?`)

	// poolAnyCoursesRe matches "Any 2 courses (1.0 credit) from": the credits in
	// brackets are the pool's amount.
	poolAnyCoursesRe = regexp.MustCompile(`^(?i:any\s+(?:(?:\d+|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve)\s+)?courses?\s*\(\s*` + amountPat + `\s+credits?\s*\)\s+(?:from|of)\b(?:\s+the\s+following)?)(?:\s*:)?`)
	// poolDescRe matches a pool whose description has commas, unlike poolFromRe:
	// "2.5 credits from electives in music history, culture, and theory:".
	// Digits and brackets would be conditions.
	poolDescRe = regexp.MustCompile(`^(?i:` + atLeastPat + `\s+)?(\d+\.\d+)\s+(?i:credits?\s+(?:from|in)\b)[^:;.()\d]{1,150}:`)
	// poolLevelRe matches a pool whose level narrows every member: "0.5 credit
	// at the 400-level from".
	poolLevelRe = regexp.MustCompile(`^(?i:` + atLeastPat + `\s+)?(\d+\.\d+)\s+(?i:credits?\s+at\s+(?:the\s+)?)(` + levelPat + `(?:\s*\+)?(?:\s*-?\s*(?i:level))?(?:\s+(?i:or\s+(?:higher|above)))?)\s+(?i:from)\b(?:\s*:)?`)
	// complPoolDescribedRe matches a pool whose list follows a description of
	// it: "2.5 credits of core courses:".
	complPoolDescribedRe = regexp.MustCompile(`^(?i:` + atLeastPat + `\s+)?(\d+\.\d+)\s+(?i:credits?)\b([^:;.()]{0,100}?)(?:\s+(?i:selected|chosen))?(?:\s+(?i:from|among)\b(?:\s+(?i:the\s+following(?:\s+list)?))?\s*:?|(\s*:))`)
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
	wildRe = regexp.MustCompile(`^((?i:any\s+)?)([A-Z]{3}(?:\s*/\s*[A-Z]{3})*)\s?(` + levelPat + `(?:\s*\+)?)((?:\s*-?\s*(?i:series|level))*)(\s+(?i:courses?)\b)?`)
	// wildDesigRe matches a bare designator as a pool member: "EEB".
	wildDesigRe   = regexp.MustCompile(`^([A-Z]{3}(?:\s*/\s*[A-Z]{3})*)\b(\s+(?i:courses?)\b)?`)
	wildEndRe     = regexp.MustCompile(`^\s*(?:$|[;,/()]|(?i:or|and)\b)`)
	wildStopRe    = regexp.MustCompile(`^(?:AND|THE|ANY|ALL|NOT|NOR|BUT|ARE|MAY|ONE|TWO|ITS|HAS|CAN|SEE|USE)\b`)
	wildSingleRe  = regexp.MustCompile(`(?i)^any\b|\bcourse$`)
	anyPoolItemRe = regexp.MustCompile(`^(?i:any|an?|one)\s+(?:\d00|[A-Z]{3}\b)`)

	// itemLabelRe matches a label before an item's body: "Geography: JGE321H1".
	// The body (itemLabelBodyRe) starts with a code, a bracket, an amount or
	// another label.
	itemLabelRe       = regexp.MustCompile(`^(?:\*\*|_)?([A-Z][^:;]{0,100}?):(?:\*\*|_)?\s+`)
	itemLabelBodyRe   = regexp.MustCompile(`^(?:` + codePat + `|\(|\d+\.\d|(?i:an?\s+)?(?i:additional|further|any|all|one|two|three)\s+(?:\d+\.\d|\(?\d|[a-z])|(?:\*\*|_)?[A-Z][^:;]{0,100}?:)`)
	itemLabelRejectRe = regexp.MustCompile(`\d\.\d|` + codePat + `|\bAS[A-Z]{3}\d{4}|^For\s|^(?i:from|in|of|at|with|to)\b|(?i:\b(?:sub)?(?:groups?|lists?|clusters?)\s+[a-z0-9]\b|\btotal\b)|(?i:\bstudents?$|\b(?:of|from|in|following)$|recommend|suggest|optional)`)

	// Annotations that say nothing about what is required.
	codeRecommendedRe = regexp.MustCompile(`(` + codePat + `)\s*\(\s*(?i:(?:strongly|highly)\s+)?(?i:recommended)\s*\)`) // "STA288H1 (Recommended)"
	recommendedEndRe  = regexp.MustCompile(`\s+(?i:(?:is|are)\s+)?(?i:(?:strongly|highly)\s+)?(?i:recommended)\.?\s*\)`) // "( CHM151Y1 strongly recommended)"
	recommendedListRe = regexp.MustCompile(`\(\s*(?i:recommended)\s*:\s*([^()]*)\)`)                                     // "(Recommended: HMB433H1)"
	recommendedTailRe = regexp.MustCompile(`,\s*((?:` + codePat + `\s*(?:[,/]|(?i:and|or))?\s*)+)\s+(?i:recommended)\s*\.?$`)
	recommendedItemRe = regexp.MustCompile(`^(?:` + codePat + `|[\s,;/()]|(?i:and|or)\b)*$`)
	// weRecommendRe matches "; while either pair of courses is accepted, we
	// strongly recommend ( STA257H1, STA261H1)".
	weRecommendRe     = regexp.MustCompile(`[;,.]\s*((?i:while\s+[^;]*?,\s*)?(?i:we\s+(?:strongly\s+|highly\s+)?recommend)\b[^;]*)$`)
	noLongerOfferedRe = regexp.MustCompile(`\s*\(\s*(?i:no\s+longer\s+offered)\s*\)`)

	// complPairYBracketRe and complPairYWholeRe match a Y course after "/" that
	// replaces the H pair before it: "( MAT148H1, MAT149H1/ MAT137Y1)", where the
	// calendar may never close the bracket. In a longer list "/" binds tighter.
	complPairYBracketRe = regexp.MustCompile(`\(\s*(` + codePat + `)\s*(?:,|(?i:and))\s*(` + codePat + `)\s*/\s*(` + codePat + `)\s*(\)|;|$)`)
	complPairYWholeRe   = regexp.MustCompile(`^((?i:` + numPat + `\s+credits?\s+from:?\s+)?)(` + codePat + `)\s*,\s*(` + codePat + `)\s*/\s*(` + codePat + `)$`)
	footnoteNoteRe      = regexp.MustCompile(`\s*\(\s*\*+\s*([^()]*)\)`) // "(* if not taken for the requirement immediately above)"
	// complAfterCodeParenRe matches brackets after a course that may only name
	// it, point to a note or say when to take it: "HIS389H1 (Hungarian History)".
	// One with a code, a number or a condition word isn't one.
	complAfterCodeParenRe = regexp.MustCompile(`(` + codePat + `|\))\s*\(([^()]*)\)`)
	complTitleWordsRe     = regexp.MustCompile(`^[A-Za-z][A-Za-z&,'’ -]*$`)
	complTitleStopRe      = regexp.MustCompile(`(?i)\b(?:or|and/or|with|without|if|unless|only|taught|in|at|by|from|for|to|approved|approval|permission|consent|equivalents?|required|recommended|not|no|must|may|can|exclud\w*|except|minimum|maximum|grade|credits?|courses?|level|half|full|students?|lecture|lab|online)\b`)
	complNoteRefRe        = regexp.MustCompile(`(?i)^(?:see\s+)?notes?(?:\s+\d+[a-z]?)?$|^\d$`)
	complTakenWhenRe      = regexp.MustCompile(`(?i)^(?:(?:may|can|should)\s+be|normally|usually|typically)\s+taken\s+(?:in|during)\s+(?:the\s+)?(?:year\s+\d|(?:first|second|third|fourth)\s+year)$|^if\s+not\s+(?:already\s+)?(?:taken|used|counted)\b|^first\s+offer(?:ed|ing)\s+in\s+\d{4}(?:-\d{2,4})?$`)
	// complRecommendedSlashRe matches "CHM249H1 strongly recommended/": the
	// course stays an alternative.
	complRecommendedSlashRe = regexp.MustCompile(`(` + codePat + `)\s+(?i:(?:strongly|highly)\s+)?(?i:recommended)\s*/`)
	// complListCreditsRe matches the credits a list adds up to: "TMU240Y1 (1.5
	// credits)".
	complListCreditsRe = regexp.MustCompile(`(` + codePat + `|\))\s*\(\s*` + numPat + `\s+credits?\s*\)`)
)

// matchRange reads a range of amounts before a pool or a credit phrase. The
// requirement is the lower end, the upper stays in raw; a pool keeps it in
// token.upper so groups adding up within the range read as their "and".
func matchRange(s string) (token, bool) {
	m := complAmountRangeRe.FindStringSubmatchIndex(s)
	if m == nil {
		return token{}, false
	}
	lo, _ := strconv.ParseFloat(s[m[2]:m[3]], 64)
	hi, _ := strconv.ParseFloat(s[m[4]:m[5]], 64)
	if hi <= lo {
		return token{}, false
	}
	rest := s[m[4]:]
	for _, match := range []phraseMatcher{matchCompletionPool, matchDescribedPool, matchPool, matchCredits} {
		t, ok := match(rest)
		switch {
		case !ok:
			continue
		case t.kind == tPool && t.amount == hi:
			t.amount, t.upper = lo, hi
		case t.kind == tCredits && t.node.Type == types.NodeCredits && t.node.Amount == hi && t.node.MaxAmount == nil:
			t.node.Amount = lo
		default:
			continue
		}
		t.end += m[4]
		return t, true
	}
	return token{}, false
}

// matchExcluding reads an exclusion after a list: ", excluding JPH311H1,
// JPH441H1". The parser applies the codes to the pool before it.
func matchExcluding(s string) (token, bool) {
	m := excludingListRe.FindStringSubmatch(s)
	if m == nil {
		return token{}, false
	}
	return token{kind: tExclude, end: len(m[0]), codes: codeRe.FindAllString(m[1], -1)}, true
}

var excludingListRe = regexp.MustCompile(`^[.,]?\s*(?i:excludes|excluding|except)\s+(` + codePat + `(?:(?:\s*,\s*(?:(?i:and|or)\s+)?|\s*/\s*|\s+(?i:and|or)\s+)` + codePat + `)*)\.?`)

func matchEither(s string) (token, bool) {
	loc := eitherRe.FindStringIndex(s)
	if loc == nil || !eitherNextRe.MatchString(s[loc[1]:]) {
		return token{}, false
	}
	return token{kind: tEither, end: loc[1]}, true
}

func matchAllOf(s string) (token, bool) {
	loc := allOfRe.FindStringIndex(s)
	if loc == nil || !poolNextRe.MatchString(s[loc[1]:]) {
		return token{}, false
	}
	return token{kind: tSkip, end: loc[1]}, true
}

func matchCompletionPool(s string) (token, bool) {
	if m := poolAnyCoursesRe.FindStringSubmatchIndex(s); m != nil {
		amount, _ := strconv.ParseFloat(s[m[2]:m[3]], 64)
		return token{kind: tPool, end: m[1], amount: amount}, true
	}
	if m := complPoolOneListRe.FindString(s); m != "" && poolNextRe.MatchString(s[len(m):]) {
		return token{kind: tPool, end: len(m), count: 1}, true
	}
	if m := poolAnyCountRe.FindStringSubmatch(s); m != nil && poolNextRe.MatchString(s[len(m[0]):]) {
		return token{kind: tPool, end: len(m[0]), count: countWords[strings.ToLower(m[1])]}, true
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
			!(wild && (w.node.Levels != nil || strings.HasSuffix(s[:m[1]], ":"))) {
			continue
		}
		amount, _ := strconv.ParseFloat(s[m[2]:m[3]], 64)
		t := token{kind: tPool, end: m[1], amount: amount}
		if re == poolLevelRe {
			t.node = &types.ReqNode{Type: types.NodeCredits, Levels: parseLevels(s[m[4]:m[5]])}
		}
		return t, true
	}
	return token{}, false
}

// matchDescribedPool reads a pool whose list follows a description of it. A
// level in the description ("300+ ECO elective courses") narrows every member.
func matchDescribedPool(s string) (token, bool) {
	if m := complPoolOfCodesRe.FindStringSubmatchIndex(s); m != nil && poolNextRe.MatchString(s[m[1]:]) {
		amount, _ := strconv.ParseFloat(s[m[2]:m[3]], 64)
		return token{kind: tPool, end: m[1], amount: amount}, true
	}
	m := complPoolDescribedRe.FindStringSubmatchIndex(s)
	if m == nil || complPoolDescStopRe.MatchString(poolTopicOr(s[m[4]:m[5]])) {
		return token{}, false
	}
	rest := strings.TrimLeft(s[m[1]:], " ")
	if w, wild := matchWild(rest); !poolNextRe.MatchString(rest) && !anyCourseMember(rest) && !(wild && w.node.Levels != nil) {
		return token{}, false
	}
	amount, _ := strconv.ParseFloat(s[m[2]:m[3]], 64)
	// "2.0 credits: A; B/ C; D" names no pool: a list adding up to the amount
	// is all needed
	t := token{kind: tPool, end: m[1], amount: amount, sum: m[6] >= 0 && strings.TrimSpace(s[m[4]:m[5]]) == ""}
	if levels := parseLevels(s[m[4]:m[5]]); levels != nil {
		t.node = &types.ReqNode{Type: types.NodeCredits, Levels: levels}
	}
	return t, true
}

// anyCourseMember reports whether s starts with one course by filter, as a
// pool member: "a 400-level PSL course", which normalize makes of
// "PSL400-series".
func anyCourseMember(s string) bool {
	t, ok := matchAnyCourse(s)
	return ok && isFilter(t.node) && anyPoolItemRe.MatchString(s[:t.end])
}

// matchWild reads a pool member named by designator and level, or designator
// alone, as half a credit matching those filters. A pool turns it into a
// filter; elsewhere only singular forms ("any ETH200+") are requirements.
func matchWild(s string) (token, bool) {
	if wildStopRe.MatchString(strings.TrimPrefix(strings.TrimPrefix(s, "any "), "Any ")) {
		return token{}, false // "ANY 300-level literature courses"
	}
	n := &types.ReqNode{Type: types.NodeCredits, Amount: 0.5}
	end, bare := 0, false
	if m := wildRe.FindStringSubmatchIndex(s); m != nil &&
		(strings.Contains(s[m[6]:m[7]], "+") || m[9] > m[8] || m[10] >= 0) { // "+", "-series"/"-level" or "courses"
		n.Designators = desigRe.FindAllString(s[m[4]:m[5]], -1)
		n.Levels = parseLevels(s[m[6]:m[7]])
		end = m[1]
	} else if m := wildDesigRe.FindStringSubmatchIndex(s); m != nil {
		n.Designators = desigRe.FindAllString(s[m[2]:m[3]], -1)
		end, bare = m[1], true
	} else {
		return token{}, false
	}
	if e := excludingRe.FindStringSubmatch(s[end:]); e != nil {
		n.Exclude = codeRe.FindAllString(e[1]+e[2], -1)
		end += len(e[0])
	}
	if bare && !wildEndRe.MatchString(s[end:]) {
		return token{}, false // a word in prose, not a list member
	}
	return token{kind: tWild, end: end, node: n}, true
}

// joinCompletionTitleRuns joins name runs as joinTitleRuns does, with bare
// designators as words: "in either GRK or LAT" stays one phrase.
func joinCompletionTitleRuns(src string, toks []token) []token {
	bare := map[[2]int]token{}
	for i, t := range toks {
		if t.kind == tWild && t.node.Levels == nil {
			bare[[2]int{t.start, t.end}] = t
			toks[i].kind = tText
		}
	}
	toks = joinTitleRuns(src, toks)
	for i, t := range toks {
		if w, ok := bare[[2]int{t.start, t.end}]; ok && t.kind == tText {
			toks[i] = w
		}
	}
	return toks
}

// stripLabels removes the labels before an item's body: "Option A:
// Immunology: ( IMM341H1, IMM351H1)". Labels reading as a condition, a pool
// or advice stay.
func stripLabels(s string) string {
	for {
		m := itemLabelRe.FindStringSubmatchIndex(s)
		if m == nil || itemLabelRejectRe.MatchString(s[m[2]:m[3]]) || len(strings.Fields(s[m[2]:m[3]])) > 12 ||
			!itemLabelBodyRe.MatchString(s[m[1]:]) {
			return s
		}
		if _, ok := matchPool(s); ok {
			return s
		}
		s = s[m[1]:]
	}
}

// stripAnnotations removes annotations that don't change what is required,
// such as "(Recommended)", titles and list sums, and returns the advice among
// them as notes. A recommended course next to "/" stays an alternative.
func stripAnnotations(s string) (string, []string) {
	var notes []string
	s = stripBareTitles(s)
	s, notes = stripCategoryLabels(s)
	s = codeRecommendedRe.ReplaceAllString(s, "$1")
	s = complRecommendedSlashRe.ReplaceAllString(s, "$1/")
	s = complListCreditsRe.ReplaceAllString(s, "$1")
	s = noLongerOfferedRe.ReplaceAllString(s, "")
	for _, m := range footnoteNoteRe.FindAllStringSubmatch(s, -1) {
		notes = append(notes, cleanNote(m[1]))
	}
	s = footnoteNoteRe.ReplaceAllString(s, "")
	s = complAfterCodeParenRe.ReplaceAllStringFunc(s, func(m string) string {
		g := complAfterCodeParenRe.FindStringSubmatch(m)
		inner := strings.TrimSpace(g[2])
		switch {
		case complTakenWhenRe.MatchString(inner):
			notes = append(notes, inner)
		case complNoteRefRe.MatchString(inner), complTitleWordsRe.MatchString(inner) && !complTitleStopRe.MatchString(inner), isCourseTitle(inner):
		default:
			return m
		}
		return g[1]
	})
	for _, m := range recommendedListRe.FindAllStringSubmatch(s, -1) {
		notes = append(notes, "Recommended: "+cleanNote(m[1]))
	}
	s = recommendedListRe.ReplaceAllString(s, "")
	// "( X, Y recommended)": an alternative of its own, or advice on the list
	for from := 0; ; {
		loc := recommendedEndRe.FindStringIndex(s[from:])
		if loc == nil {
			break
		}
		start, end := from+loc[0], from+loc[1]
		open := matchingOpen(s, end-1)
		if open < 0 || !recommendedItemRe.MatchString(s[open+1:start]) {
			from = end
			continue
		}
		// an alternative only next to "/" or "or"; alone or after a list, advice
		before, after := strings.TrimRight(s[:open], " "), strings.TrimLeft(s[end:], " ")
		if strings.HasSuffix(before, "/") || orEndRe.MatchString(before) || strings.HasPrefix(after, "/") || orStartRe.MatchString(after) {
			s = s[:start] + ")" + s[end:] // keep the alternative, drop the words
			from = start + 1
			continue
		}
		// "( GGR101H1; CHM135H1, CHM136H1 recommended)": one note
		notes = append(notes, strings.ReplaceAll(cleanNote(s[open+1:end-1]), ";", ","))
		s = before + s[end:]
		from = len(before)
	}
	if m := weRecommendRe.FindStringSubmatchIndex(s); m != nil {
		notes = append(notes, cleanNote(s[m[2]:m[3]]))
		s = s[:m[0]]
	}
	if m := recommendedTailRe.FindStringSubmatchIndex(s); m != nil {
		notes = append(notes, cleanNote(s[m[2]:m[1]]))
		s = s[:m[0]]
	}
	return strings.TrimSpace(wsRe.ReplaceAllString(s, " ")), notes
}

// groupPairY brackets a pair of H courses that a Y course after "/"
// replaces: "( MAT148H1, MAT149H1)/ MAT137Y1". The three share a designator
// and the Y course is worth the pair.
func groupPairY(s string) string {
	pair := func(a, b, y string) bool {
		return a[:3] == y[:3] && b[:3] == y[:3] && courseWeight(a) == 0.5 && courseWeight(b) == 0.5 && courseWeight(y) == 1
	}
	if m := complPairYWholeRe.FindStringSubmatch(s); m != nil && pair(m[2], m[3], m[4]) {
		return m[1] + "( " + m[2] + ", " + m[3] + ")/ " + m[4]
	}
	var b strings.Builder
	last := 0
	for _, m := range complPairYBracketRe.FindAllStringSubmatchIndex(s, -1) {
		a, c, y, end := s[m[2]:m[3]], s[m[4]:m[5]], s[m[6]:m[7]], s[m[8]:m[9]]
		// an unclosed bracket ("...; PHY131H1") only when nothing closes it later
		if !pair(a, c, y) || end != ")" && strings.Count(s[m[1]:], ")") > strings.Count(s[m[1]:], "(") {
			continue
		}
		b.WriteString(s[last:m[0]])
		b.WriteString("( " + a + ", " + c + ")/ " + y)
		if end == ";" {
			b.WriteString(";")
		}
		last = m[1]
	}
	b.WriteString(s[last:])
	return b.String()
}

// rewriteCompletion rewrites completion text into what the grammar reads,
// and returns what it sets aside (restatements, timing) as notes.
func rewriteCompletion(text string) (string, []string) {
	var notes []string
	if m := complIeRe.FindStringSubmatchIndex(text); m != nil {
		if codeRe.MatchString(text[m[2]:m[3]]) {
			notes = append(notes, cleanNote(text[m[2]:m[3]]))
		}
		text = text[:m[0]]
	}
	// the grade covers the slash list, as in enrolment; complGradeParenRe first:
	// slashGradeRe reads only the short form "(63%)"
	text = slashGradeRe.ReplaceAllStringFunc(complGradeParenRe.ReplaceAllString(text, "($1%)"), spreadSlashGrade)
	text = rewriteOneOffs(text)
	text = complCodedCoursesRe.ReplaceAllString(text, "$1 courses")
	text = complDescParenRe.ReplaceAllString(text, "$1")
	text = complOnTheLevelRe.ReplaceAllString(text, "at the $1-level")
	text = complOneCreditRe.ReplaceAllString(text, "${1}1.0 credit")
	text = complAtCampusRe.ReplaceAllString(text, "$1")
	text = complOneOrBothRe.ReplaceAllString(text, "$1/ $2")
	text = complTitledAmountRe.ReplaceAllString(text, "$1 credits: ")
	text = amountAndCodes(text)
	text = allAmountOf(text)
	// after complDescParenRe and complAtCampusRe, which would hide the pair
	text = complCodeParenCodeRe.ReplaceAllString(text, "$1/ $2")
	text = complExcludingAndRe.ReplaceAllString(text, "$1 $2")
	text = complSubjectDesigRe.ReplaceAllString(text, "$1 $2")
	text = complOrDesigParenRe.ReplaceAllString(text, "$1/$2")
	text = complInAdditionRe.ReplaceAllString(text, ", excluding $1")
	text = anyLevelRe.ReplaceAllString(complCapDesigRe.ReplaceAllString(text, "$1 $2 credits"), "")
	text = joinCodePairs(text)
	text = complWithAtLeastRe.ReplaceAllString(text, ", including at least $1 credit")
	text = complWholeAmountRe.ReplaceAllString(text, "$1$2.0$3")
	// after complWholeAmountRe: complMustBeFromRe reads only a decimal amount
	text = complMustBeFromRe.ReplaceAllString(text, "$1 from")
	text = complFurtherRe.ReplaceAllString(complDoubledFromRe.ReplaceAllString(text, "from"), "$1 $2")
	if m := complNormallyTakenRe.FindStringSubmatchIndex(text); m != nil {
		notes = append(notes, text[m[2]:m[3]])
		text = text[:m[0]]
	}
	text = complCodeTitleRe.ReplaceAllString(text, "$1")
	text = complArticleAmountRe.ReplaceAllString(complBreadthNameRe.ReplaceAllString(text, "$1"), "$1 $2")
	text = complGradePlusRe.ReplaceAllString(text, "($1%)")
	// groupPairY last: it reads the list as the rewrites above leave it
	return groupPairY(eitherDesigRe.ReplaceAllString(text, "$1 $2")), notes
}

// amountAndCodes reads "N credits <of/from ...> A and B" as "A and B" when
// each side's lightest course adds up to N: the amount restates them.
func amountAndCodes(text string) string {
	m := complAmountAndRe.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil {
		return text
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	lightest := func(s string) float64 {
		w := 0.0
		for i, c := range codeRe.FindAllString(s, -1) {
			if x := courseWeight(c); i == 0 || x < w {
				w = x
			}
		}
		return w
	}
	if math.Abs(lightest(m[2])+lightest(m[3])-amount) > 1e-9 {
		return text
	}
	return m[2] + " and " + m[3]
}

// complAllAmountOfRe matches "All (2.0 credits) of GGR270H1, GGR272H1,
// GGR273H1, GGR373H1".
var complAllAmountOfRe = regexp.MustCompile(`(?i)^all\s*\(\s*` + amountPat + `\s+credits?\s*\)\s+of:?\s+((?:` + codePat + `\s*,?\s*(?:and\s+)?)+)\.?$`)

// allAmountOf reads "All (N credits) of A, B, ..." as "All of A, B, ..."
// when the courses weigh N.
func allAmountOf(text string) string {
	m := complAllAmountOfRe.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil {
		return text
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	sum := 0.0
	for _, c := range codeRe.FindAllString(m[2], -1) {
		sum += courseWeight(c)
	}
	if math.Abs(sum-amount) > 1e-9 {
		return text
	}
	return "All of " + m[2]
}

// joinCodePairs separates two codes written with only a space ("SLA322H1
// SLA410H1", sic) by the list's separator: "/" in a list of alternatives,
// else ",".
func joinCodePairs(text string) string {
	for {
		m := complCodePairRe.FindStringSubmatchIndex(text)
		if m == nil {
			return text
		}
		sep := ", "
		if complSlashSepRe.MatchString(text[:m[2]]) || strings.HasPrefix(strings.TrimSpace(text[m[1]:]), "/") {
			sep = "/ "
		}
		text = text[:m[2]] + text[m[2]:m[3]] + sep + text[m[4]:]
	}
}

// complSlashSepRe matches text ending in a code and "/": the code before a
// pair is another alternative.
var complSlashSepRe = regexp.MustCompile(codePat + `\s*/\s*$`)

var (
	orEndRe   = regexp.MustCompile(`(?i)\bor$`)
	orStartRe = regexp.MustCompile(`(?i)^or\b`)
)

// matchingOpen returns the index of the "(" that the ")" at close closes, or -1.
func matchingOpen(s string, close int) int {
	depth := 0
	for i := close; i >= 0; i-- {
		switch s[i] {
		case ')':
			depth++
		case '(':
			if depth--; depth == 0 {
				return i
			}
		}
	}
	return -1
}
