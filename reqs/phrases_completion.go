package reqs

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Grammar extensions for program completion text. The phrases here are only
// tried by lexCompletion, and the rewrites only run in parseCompletionText,
// so course requisites and enrolment never see them.

// completionMatchers: the completion phrases first, then the shared ones.
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
	// "2.0 to 2.5 credits from", "1.0-1.5 credits from", "0.5 or 1.0 credit
	// from": a range of amounts before a pool or a credit phrase.
	complAmountRangeRe = regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*(?:-|–|(?i:to|or)\b)\s*(\d+(?:\.\d+)?)\s`)

	// "Either GGR491Y1 or one of ...": "either" before alternatives.
	eitherRe     = regexp.MustCompile(`^(?i:either)\b(?:\s*:)?`)
	eitherNextRe = regexp.MustCompile(`^\s*(?:` + codePat + `|\(|\d+\.\d|(?i:one|two|three|any|all)\s+\w|an?\s+\w)`)

	// ", with at least a 0.5 credit at the 300+ level": a floor, read as
	// "including".
	complWithAtLeastRe = regexp.MustCompile(`(?i),\s*with\s+at\s+least\s+(?:an?\s+)?(\d+(?:\.\d+)?)\s+credits?\b`)
	// "0.5 credit from from PHY252H1" (sic), "2.0 credits further from".
	complDoubledFromRe = regexp.MustCompile(`(?i)\bfrom\s+from\b`)
	complFurtherRe     = regexp.MustCompile(`(?i)\b(credits?)\s+further\s+(from|of)\b`)
	// "SLA322H1 SLA410H1" (sic): a list missing its comma.
	complCodePairRe = regexp.MustCompile(`(` + codePat + `|\b[A-Z]{3}\d{4}[HY]\d?)\s+(` + codePat + `|[A-Z]{3}\d{4}[HY]\d?\b)`)
	// "1.0 credit of the introductory courses: MST210H1 and MST212H1",
	// "1.0 credit from GGR217H1 and JGI346H1/ JGU346H1": an amount over
	// courses joined by "and".
	complAmountAndRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+(?:(?:of|in)\s+[^:]*:|from:?|[^:]*:)\s*((?:` + codePat + `\s*/\s*)*` + codePat + `)\s+and\s+((?:` + codePat + `\s*/\s*)*` + codePat + `)\.?$`)
	// "A set of mandatory courses (3.5 credits): ACT348H1, ...": the amount
	// over the list, its title dropped.
	complTitledAmountRe = regexp.MustCompile(`^(?:[A-Z][^:()]{0,60}?)\s*\(\s*(\d+(?:\.\d+)?)\s+credits?\s*\)\s*:\s*`)
	// "PCJ-coded courses": "PCJ courses".
	complCodedCoursesRe = regexp.MustCompile(`\b([A-Z]{3})-coded\s+courses\b`)
	// "BIO251H1/ ... EEB240H1 (plant or animal form and function/environmental
	// biology)": a lowercase description of the courses before it.
	complDescParenRe = regexp.MustCompile(`(` + codePat + `)\s*\(\s*[a-z][a-z ,/&-]*\)`)
	// "including one credit at the 300+ level": "1.0 credit".
	complOneCreditRe = regexp.MustCompile(`(?i)(\b(?:including|of\s+which|with)\s+(?:at\s+least\s+)?)one\s+credit\b`)
	// "ANT418H5 at UTM": the code says the campus.
	complAtCampusRe = regexp.MustCompile(`(` + codePat + `)\s*(?:\(\s*)?(?:at|from)\s+(?:UTM|UTSC)\b\)?`)
	// "GER courses on the 300-level": "at the 300-level".
	complOnTheLevelRe = regexp.MustCompile(`(?i)\bon\s+the\s+([1-4]00)[-\s]level\b`)
	// "MGY480Y1/ one or both of ( MGY481H1 and MGY482H1)": either course.
	complOneOrBothRe = regexp.MustCompile(`(?i)\bone\s+or\s+both\s+of\s+\(\s*(` + codePat + `)\s+and\s+(` + codePat + `)\s*\)`)
	// "(minimum grade of 63%)" after a course: "(63%)".
	complGradeParenRe = regexp.MustCompile(`(?i)\(\s*(?:a\s+)?(?:minimum|min\.?)\s+(?:final\s+)?(?:grade|mark)\s+of\s+(?:at\s+least\s+)?(\d{2})\s*%\s*\)`)
	// "CHM136H1 ( CHM138H1)": the course or the one in brackets.
	complCodeParenCodeRe = regexp.MustCompile(`(` + codePat + `)\s*\(\s*(` + codePat + `)\s*\)`)
	// "GER290H1 ‘Global Issues / German Contexts’": a title in quotes.
	complQuotedTitleRe = regexp.MustCompile(`(` + codePat + `)\s*‘[^’]*’`)
	// "excluding GER150H1, GER361H1, and GER410H1": the last excluded course.
	complExcludingAndRe = regexp.MustCompile(`(?i)(\bexcluding\s+(?:` + codePat + `\s*,\s*)+)and\s+(` + codePat + `)`)
	// "in music history (HMU) or music theory (TMU)": the subjects'
	// designators.
	complSubjectDesigRe = regexp.MustCompile(`\b(in|or)\s+(?:[a-z]+\s+){1,3}\(\s*([A-Z]{3})\s*\)`)
	// "MAT (or APM) courses", which normalize writes "MAT or (APM)": either
	// designator.
	complOrDesigParenRe = regexp.MustCompile(`\b([A-Z]{3})\s+(?i:or)\s+\(\s*([A-Z]{3})\s*\)`)
	// "(in addition to APM462H1)": a course required elsewhere doesn't count
	// here too.
	complInAdditionRe = regexp.MustCompile(`(?i)\s*\(\s*in\s+addition\s+to\s+((?:` + codePat + `(?:\s*(?:,|and|/)\s*)?)+)\s*\)`)
	// "of which no more than 0.5 credit can be IRE/IRW": a cap on
	// designators.
	complCapDesigRe = regexp.MustCompile(`(?i)(no\s+more\s+than\s+\d+(?:\.\d+)?)\s+credits?\s+(?:can|may)\s+be\s+(?:in\s+)?((?-i:[A-Z]{3})(?:\s*/\s*(?-i:[A-Z]{3}))*)\b`)
	// ", at any level": no level filter.
	complAtAnyLevelRe = regexp.MustCompile(`(?i),?\s*at\s+any\s+level\b`)
	// ", normally taken in the first year", ", highly recommended to be
	// taken in the second year": timing, a note.
	complNormallyTakenRe = regexp.MustCompile(`(?i),\s*((?:normally|usually|typically|(?:(?:highly|strongly)\s+)?recommended\s+to\s+be)\s+taken\s+in\s+(?:the\s+)?(?:first|second|third|fourth)\s+year)\s*[;.]?\s*$`)
	// "HIS268H1: Law and History": a course's title after a colon.
	complCodeTitleRe = regexp.MustCompile(`^(` + codePat + `)\s*:\s+[A-Z][A-Za-z ,&'’-]*\.?$`)
	// "or a 0.5 credit at the 200/300/400-level": the article.
	complArticleAmountRe = regexp.MustCompile(`(?i)\b(or|and)\s+an?\s+(\d+\.\d+\s+credits?)\b`)
	// "Breadth Requirement Category 5: The Physical and Mathematical
	// Universes": the category's name after its number.
	complBreadthNameRe = regexp.MustCompile(`(?i)(\bcategor(?:y|ies)\s+[1-5])\s*[:(]\s*(?:creative\s+and\s+cultural\s+representations|thought,\s+belief,?\s+and\s+behaviou?r|society\s+and\s+its\s+institutions|living\s+things\s+and\s+their\s+environment|the\s+physical\s+(?:and|or)\s+mathematical\s+universes?)\)?`)
	// "MAT221H1(80%+)": a grade with a plus.
	complGradePlusRe = regexp.MustCompile(`\(\s*(\d{2})%\s*\+\s*\)`)
	// "1 credit from", "3 credits from": a whole number.
	complWholeAmountRe = regexp.MustCompile(`(^|[\s(])(\d{1,2})(\s+credits?\b)`)
	// "1.0 credit must be taken from GER270H1/ ...": a pool.
	complMustBeFromRe = regexp.MustCompile(`(?i)^(\d+\.\d+\s+credits?)\s+(?:must\s+be\s+(?:taken|chosen|selected)|chosen|selected)\s+from\b`)
	// ", i.e., 2.0 SOC credits at the 200 OR 300-level and ...": the text
	// before, restated; a note.
	complIeRe = regexp.MustCompile(`(?i),?\s*\b(i\.e\.,?\s+[^;]*)$`)

	// "in either GRK or LAT": a credit phrase's designators, not alternatives.
	eitherDesigRe = regexp.MustCompile(`\b((?i:in|from))\s+(?i:either)\s+([A-Z]{3}\s+(?i:or)\s+[A-Z]{3})\b`)

	// "All of: LIN101H1, LIN102H1": commas already mean "and".
	allOfRe = regexp.MustCompile(`^(?i:all\s+of(?:\s+the\s+following(?:\s+courses)?)?)(?:\s*:)?`)

	// "Any 2 courses (1.0 credit) from", "Any course (0.5 credit) from": the
	// credits in brackets are the pool's amount.
	poolAnyCoursesRe = regexp.MustCompile(`^(?i:any\s+(?:(?:\d+|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve)\s+)?courses?\s*\(\s*(\d+(?:\.\d+)?)\s+credits?\s*\)\s+(?:from|of)\b(?:\s+the\s+following)?)(?:\s*:)?`)
	// "2.5 credits from electives in music history, culture, and theory, and
	// music and health: HMU200+ level courses; ...": a described pool whose
	// description has commas (poolFromRe takes plain words only). Digits and
	// brackets would be conditions ("including at least 1.0 credit ...").
	poolDescRe = regexp.MustCompile(`^(?i:at\s+least\s+)?(\d+\.\d+)\s+(?i:credits?\s+(?:from|in)\b)[^:;.()\d]{1,150}:`)
	// "0.5 credit at the 400-level from CSB426H1, ...", "1.0 credit at 300+
	// level from: CJH332H1; CSB; EEB": the level narrows every member.
	poolLevelRe = regexp.MustCompile(`^(?i:at\s+least\s+)?(\d+\.\d+)\s+(?i:credits?\s+at\s+(?:the\s+)?)([1-4]00(?:\s*\+)?(?:\s*-?\s*(?i:level))?(?:\s+(?i:or\s+(?:higher|above)))?)\s+(?i:from)\b(?:\s*:)?`)
	// "1.5 credit of 300+ ECO elective courses from:", "2.5 credits of core
	// courses:", "0.5 credit laboratory course from:", "1.0 credit selected
	// from", "2.0 credits:": a pool whose list follows a description of it
	// (matchDescribedPool).
	complPoolDescribedRe = regexp.MustCompile(`^(?i:at\s+least\s+)?(\d+\.\d+)\s+(?i:credits?)\b([^:;.()]{0,100}?)(?:\s+(?i:selected|chosen))?(?:\s+(?i:from|among)\b(?:\s+(?i:the\s+following(?:\s+list)?))?\s*:?|(\s*:))`)
	// "1 credit of AST221H1, AST222H1": a pool, when codes follow.
	complPoolOfCodesRe = regexp.MustCompile(`^(\d+\.\d+)\s+(?i:credits?\s+of)\s+`)
	// Words that make a description a condition, not a name: "including at
	// least", "with", "to a total of", or a second pool ("and").
	complPoolDescStopRe = regexp.MustCompile(`(?i)\b(?:including|include|with|which|must|should|may|can|least|than|per|each|and|or|to|total|any)\b`)
	// "One course from the following list:": one of.
	complPoolOneListRe = regexp.MustCompile(`^(?i:(?:one|a)\s+course\s+from\s+the\s+following\s+list)\s*:?`)
	// "any two of: A, B, C"
	poolAnyCountRe = regexp.MustCompile(`^(?i:any\s+(two|three|four|[2-4])\s+of\b(?:\s+the\s+following)?)(?:\s*:)?`)

	// Pool members named by designator and level: "PSL400-series courses",
	// "HMU200+ level courses", "TMU200+ courses", "SMU400-level courses",
	// "PHY 300-level courses", "APM/MAT 200+ courses", "EEB 400-series level
	// course", "any ETH200+".
	wildRe = regexp.MustCompile(`^((?i:any\s+)?)([A-Z]{3}(?:\s*/\s*[A-Z]{3})*)\s?([1-4]00(?:\s*\+)?)((?:\s*-?\s*(?i:series|level))*)(\s+(?i:courses?)\b)?`)
	// A bare designator as a pool member: "EEB (excluding EEB196H1, ...)",
	// "CRI100H1/ ECO/ HIS", "2.0 credits from: ANA; ANT333Y1, ...; BCH; BIO".
	wildDesigRe   = regexp.MustCompile(`^([A-Z]{3}(?:\s*/\s*[A-Z]{3})*)\b(\s+(?i:courses?)\b)?`)
	wildEndRe     = regexp.MustCompile(`^\s*(?:$|[;,/()]|(?i:or|and)\b)`)
	wildStopRe    = regexp.MustCompile(`^(?:AND|THE|ANY|ALL|NOT|NOR|BUT|ARE|MAY|ONE|TWO|ITS|HAS|CAN|SEE|USE)\b`)
	wildSingleRe  = regexp.MustCompile(`(?i)^any\b|\bcourse$`)
	anyPoolItemRe = regexp.MustCompile(`^(?i:any|an?|one)\s+(?:\d00|[A-Z]{3}\b)`)

	// A label before an item's body: "Geography: JGE321H1", "Option A:
	// Immunology: ( IMM341H1, IMM351H1)", "_Ukrainian:_ SLA108Y1", "Groups,
	// Rings and Fields: MAT347Y1". The body starts with a code, a bracket,
	// an amount or another label.
	itemLabelRe       = regexp.MustCompile(`^(?:\*\*|_)?([A-Z][^:;]{0,100}?):(?:\*\*|_)?\s+`)
	itemLabelBodyRe   = regexp.MustCompile(`^(?:` + codePat + `|\(|\d+\.\d|(?i:an?\s+)?(?i:additional|further|any|all|one|two|three)\s+(?:\d+\.\d|\(?\d|[a-z])|(?:\*\*|_)?[A-Z][^:;]{0,100}?:)`)
	itemLabelRejectRe = regexp.MustCompile(`\d\.\d|` + codePat + `|\bAS[A-Z]{3}\d{4}|^For\s|^(?i:from|in|of|at|with|to)\b|(?i:\b(?:sub)?(?:groups?|lists?|clusters?)\s+[a-z0-9]\b|\btotal\b)|(?i:\bstudents?$|\b(?:of|from|in|following)$|recommend|suggest|optional)`)

	// Annotations that say nothing about what is required.
	codeRecommendedRe = regexp.MustCompile(`(` + codePat + `)\s*\(\s*(?i:(?:strongly|highly)\s+)?(?i:recommended)\s*\)`) // "STA288H1 (Recommended)"
	recommendedEndRe  = regexp.MustCompile(`\s+(?i:(?:is|are)\s+)?(?i:(?:strongly|highly)\s+)?(?i:recommended)\.?\s*\)`) // "( CHM151Y1 strongly recommended)"
	recommendedListRe = regexp.MustCompile(`\(\s*(?i:recommended)\s*:\s*([^()]*)\)`)                                     // "(Recommended: HMB433H1)"
	recommendedTailRe = regexp.MustCompile(`,\s*((?:` + codePat + `\s*(?:[,/]|(?i:and|or))?\s*)+)\s+(?i:recommended)\s*\.?$`)
	recommendedItemRe = regexp.MustCompile(`^(?:` + codePat + `|[\s,;/()]|(?i:and|or)\b)*$`)
	// "; while either pair of courses is accepted, we strongly recommend ( STA257H1, STA261H1)"
	weRecommendRe     = regexp.MustCompile(`[;,.]\s*((?i:while\s+[^;]*?,\s*)?(?i:we\s+(?:strongly\s+|highly\s+)?recommend)\b[^;]*)$`)
	noLongerOfferedRe = regexp.MustCompile(`\s*\(\s*(?i:no\s+longer\s+offered)\s*\)`)

	// A Y course after "/" that replaces the pair of H courses before it:
	// "( MAT148H1, MAT149H1/ MAT137Y1)", "( MAT148H1, MAT149H1/ MAT137Y1;
	// PHY131H1" (the calendar never closes the bracket), or a whole item
	// "CSC108H1, CSC148H1/ CSC110Y1", also as an amount's whole pool: "1.0
	// credit from ECO101H1, ECO102H1/ ECO105Y1". In a longer list
	// ("LCT308H1, LCT401H1/ LCT401Y1, LCT403H1") the Y course is the H
	// course's alternative, as "/" binding tighter than "," says.
	complPairYBracketRe = regexp.MustCompile(`\(\s*(` + codePat + `)\s*(?:,|(?i:and))\s*(` + codePat + `)\s*/\s*(` + codePat + `)\s*(\)|;|$)`)
	complPairYWholeRe   = regexp.MustCompile(`^((?i:\d+(?:\.\d+)?\s+credits?\s+from:?\s+)?)(` + codePat + `)\s*,\s*(` + codePat + `)\s*/\s*(` + codePat + `)$`)
	footnoteNoteRe      = regexp.MustCompile(`\s*\(\s*\*+\s*([^()]*)\)`) // "(* if not taken for the requirement immediately above)"
	// Brackets after a course that only name it, point to a note or say
	// when to take it: "HIS389H1 (Hungarian History)", "BIO220H1 (ecology and
	// evolutionary biology)", "CJS200H1/ CJS201H1 (See Note)", "CSC240H1
	// (1)", "BIO130H1 (may be taken in Year 1)". A bracket with a code, a
	// number or a condition word ("(taught in German)", "(or equivalent)")
	// isn't one.
	complAfterCodeParenRe = regexp.MustCompile(`(` + codePat + `|\))\s*\(([^()]*)\)`)
	complTitleWordsRe     = regexp.MustCompile(`^[A-Za-z][A-Za-z&,'’ -]*$`)
	complTitleStopRe      = regexp.MustCompile(`(?i)\b(?:or|and/or|with|without|if|unless|only|taught|in|at|by|from|for|to|approved|approval|permission|consent|equivalents?|required|recommended|not|no|must|may|can|exclud\w*|except|minimum|maximum|grade|credits?|courses?|level|half|full|students?|lecture|lab|online)\b`)
	complNoteRefRe        = regexp.MustCompile(`(?i)^(?:see\s+)?notes?(?:\s+\d+[a-z]?)?$|^\d$`)
	complTakenWhenRe      = regexp.MustCompile(`(?i)^(?:(?:may|can|should)\s+be|normally|usually|typically)\s+taken\s+(?:in|during)\s+(?:the\s+)?(?:year\s+\d|(?:first|second|third|fourth)\s+year)$|^if\s+not\s+(?:already\s+)?(?:taken|used|counted)\b|^first\s+offer(?:ed|ing)\s+in\s+\d{4}(?:-\d{2,4})?$`)
	// "CHM249H1 strongly recommended/ CHM247H1": the course stays an
	// alternative.
	complRecommendedSlashRe = regexp.MustCompile(`(` + codePat + `)\s+(?i:(?:strongly|highly)\s+)?(?i:recommended)\s*/`)
	// "HMU225H1, TMU240Y1 (1.5 credits)": the credits a list adds up to.
	complListCreditsRe = regexp.MustCompile(`(` + codePat + `|\))\s*\(\s*\d+(?:\.\d+)?\s+credits?\s*\)`)
)

// matchRange reads a range of amounts before a pool or a credit phrase:
// "2.0 to 2.5 credits from ...". The requirement is the lower end, as for a
// header total (design 3.1); the upper end stays in raw. A pool keeps it
// (token.upper), so that groups adding up to an amount in the range are
// read as their "and" (groupPool).
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
// JPH441H1", ". Excludes BCH472Y1, BCH473Y1.". The parser applies the codes
// to the pool before it.
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
	// "2.0 credits from: ANA; ANT333Y1, ...": a pool whose first member is a
	// wildcard (matchPool wants a code or a bracket). A bare designator only
	// starts a list after a colon: "2.0 credits in GRK or LAT" is a credit
	// phrase.
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

// matchDescribedPool reads a pool whose list follows a description of it
// (complPoolDescribedRe). The description names the pool; a level in it ("300+
// ECO elective courses") narrows every member, as in "0.5 credit at the
// 400-level from" (unionPool keeps the pool text when a listed course
// isn't at that level).
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
	// "2.0 credits: A; B/ C; D" names no pool: a list that adds up to the
	// amount is all needed (groupPool).
	t := token{kind: tPool, end: m[1], amount: amount, sum: m[6] >= 0 && strings.TrimSpace(s[m[4]:m[5]]) == ""}
	if levels := parseLevels(s[m[4]:m[5]]); levels != nil {
		t.node = &types.ReqNode{Type: types.NodeCredits, Levels: levels}
	}
	return t, true
}

// anyCourseMember reports whether s starts with one course by filter, as a
// pool member: "a 400-level PSL course", which normalize makes of
// "PSL400-series" ("1.0-1.5 credits from PSL400-series or HMB430H1/ ...").
func anyCourseMember(s string) bool {
	t, ok := matchAnyCourse(s)
	return ok && isFilter(t.node) && anyPoolItemRe.MatchString(s[:t.end])
}

// matchWild reads a pool member named by designator and level, or by
// designator alone, with any "(excluding ...)" after it. Its node is half a
// credit matching those filters: one such course, as in matchAnyCourse. A
// pool turns it into a filter (see unionPool); elsewhere only the singular
// forms ("any ETH200+", "EEB 400-series level course") are requirements.
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
// designators taking part as words, as they do in course text: "in either
// GRK or LAT", "taught at UTM or UTSC" stay one phrase.
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
// Immunology: ( IMM341H1, IMM351H1)" -> "( IMM341H1, IMM351H1)". Labels that
// read as a condition ("For students with ...:"), a pool ("One of:") or advice
// ("Recommended:") stay.
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

// stripAnnotations removes annotations that don't change what is required
// and returns the advice among them as notes:
//   - "STA288H1 (Recommended)", "( CHM151Y1 strongly recommended)/ ( ... )",
//     "CHM249H1 strongly recommended/ CHM247H1": the course stays an
//     alternative;
//   - "HMU225H1, TMU240Y1 (1.5 credits)": the sum, dropped;
//   - "( MAT237Y1 recommended)", "(Recommended: HMB433H1)" after a list, and
//     ", MAT401H1/ MAT402H1 recommended" at the end: notes;
//   - "WGS350H1 (no longer offered)": the code stays;
//   - "(* if not taken for the requirement immediately above)": a footnote
//     to a starred code (normalize drops the star), kept as a note;
//   - a course's title, a note reference or when to take it, in brackets
//     after it (complAfterCodeParenRe): dropped, the timing kept as a note.
func stripAnnotations(s string) (string, []string) {
	var notes []string
	s = stripBareTitles(s) // completion_titles.go
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
	// "( X, Y recommended)": an alternative of its own, or advice about the
	// list before it.
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
		// An alternative only next to "/" or "or": "( CHM151Y1 strongly
		// recommended)/ ( CHM135H1, CHM136H1)". Alone ("[ PHY131H1, PHY132H1
		// recommended]") or after a list it is advice.
		before, after := strings.TrimRight(s[:open], " "), strings.TrimLeft(s[end:], " ")
		if strings.HasSuffix(before, "/") || orEndRe.MatchString(before) || strings.HasPrefix(after, "/") || orStartRe.MatchString(after) {
			s = s[:start] + ")" + s[end:] // keep the alternative, drop the words
			from = start + 1
			continue
		}
		// "( GGR101H1; CHM135H1, CHM136H1 recommended)": one piece of advice,
		// kept as one sentence.
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
// replaces (complPairYBracketRe, complPairYWholeRe): "( MAT148H1, MAT149H1/
// MAT137Y1)" -> "( MAT148H1, MAT149H1)/ MAT137Y1". The three courses share a
// designator, and the Y course is worth the pair.
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
		// An unclosed bracket ("...; PHY131H1") only when nothing closes it
		// later.
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
// and returns what it sets aside as notes:
//   - ", i.e., ..." restates what comes before: dropped, a note if it names
//     a course;
//   - a grade after a slash list covers every course in it;
//   - "CHM136H1 ( CHM138H1)" is "CHM136H1/ CHM138H1"; "excluding A, B, and
//     C" loses the "and"; "in music history (HMU) or music theory (TMU)" is
//     "in HMU or TMU"; "MAT (or APM)" is "MAT/APM"; "(in addition to
//     APM462H1)" excludes APM462H1; "no more than 0.5 credit can be IRE/IRW"
//     is a cap on IRE/IRW credits; "at any level" goes;
//   - "SLA322H1 SLA410H1" (sic) gets its list's separator (joinCodePairs);
//   - ", with at least a 0.5 credit" is ", including at least 0.5 credit";
//     "1 credit" is "1.0 credit"; "1.0 credit must be taken from" and "3.0
//     credits chosen from" lose the verb; "from from" (sic) and "credits
//     further from" lose a word;
//   - ", normally taken in the first year": a note;
//   - "HIS268H1: Law and History" loses the title; "Category 5: The
//     Physical and Mathematical Universes" loses the name; "or a 0.5
//     credit" loses the article; "(80%+)" is "(80%)";
//   - "in either GRK or LAT" is "in GRK or LAT", and a Y course after "/"
//     replaces the pair before it (groupPairY).
func rewriteCompletion(text string) (string, []string) {
	var notes []string
	if m := complIeRe.FindStringSubmatchIndex(text); m != nil {
		if codeRe.MatchString(text[m[2]:m[3]]) {
			notes = append(notes, cleanNote(text[m[2]:m[3]]))
		}
		text = text[:m[0]]
	}
	// "RSM100H1/ MGT100H1/ ECO101H1 (minimum grade of 63%)": the grade
	// covers the slash list, as in enrolment (spreadSlashGrade). After
	// complGradeParenRe: slashGradeRe reads only the short form "(63%)".
	text = slashGradeRe.ReplaceAllStringFunc(complGradeParenRe.ReplaceAllString(text, "($1%)"), spreadSlashGrade)
	// One-off wordings (completion_fixes.go).
	text = rewriteOneOffs(text)
	text = complCodedCoursesRe.ReplaceAllString(text, "$1 courses") // "PCJ-coded courses"
	text = complDescParenRe.ReplaceAllString(text, "$1")
	text = complOnTheLevelRe.ReplaceAllString(text, "at the $1-level")
	text = complOneCreditRe.ReplaceAllString(text, "${1}1.0 credit")
	text = complAtCampusRe.ReplaceAllString(text, "$1")
	text = complOneOrBothRe.ReplaceAllString(text, "$1/ $2")
	text = complTitledAmountRe.ReplaceAllString(text, "$1 credits: ")
	text = amountAndCodes(text)
	text = allAmountOf(text)
	// After complDescParenRe and complAtCampusRe: a description or a campus
	// between a course and its bracketed alternative ("CHM136H1 (at UTM) (
	// CHM138H1)") would hide the pair.
	text = complCodeParenCodeRe.ReplaceAllString(text, "$1/ $2")
	text = complExcludingAndRe.ReplaceAllString(text, "$1 $2")
	text = complSubjectDesigRe.ReplaceAllString(text, "$1 $2")
	text = complOrDesigParenRe.ReplaceAllString(text, "$1/$2")
	text = complInAdditionRe.ReplaceAllString(text, ", excluding $1")
	text = complAtAnyLevelRe.ReplaceAllString(complCapDesigRe.ReplaceAllString(text, "$1 $2 credits"), "")
	text = joinCodePairs(text)
	text = complWithAtLeastRe.ReplaceAllString(text, ", including at least $1 credit")
	text = complWholeAmountRe.ReplaceAllString(text, "$1$2.0$3")
	// After complWholeAmountRe: complMustBeFromRe reads only a decimal amount
	// ("1 credit must be taken from" is "1.0 credit ..." by then).
	text = complMustBeFromRe.ReplaceAllString(text, "$1 from")
	text = complFurtherRe.ReplaceAllString(complDoubledFromRe.ReplaceAllString(text, "from"), "$1 $2")
	if m := complNormallyTakenRe.FindStringSubmatchIndex(text); m != nil {
		notes = append(notes, text[m[2]:m[3]])
		text = text[:m[0]]
	}
	text = complCodeTitleRe.ReplaceAllString(text, "$1")
	text = complArticleAmountRe.ReplaceAllString(complBreadthNameRe.ReplaceAllString(text, "$1"), "$1 $2")
	text = complGradePlusRe.ReplaceAllString(text, "($1%)")
	// groupPairY last: it reads the list as the rewrites above leave it ("1.0
	// credit must be taken from MAT148H1, MAT149H1/ MAT137Y1" reads as a
	// whole only once complMustBeFromRe has dropped the verb).
	return groupPairY(eitherDesigRe.ReplaceAllString(text, "$1 $2")), notes
}

// amountAndCodes reads "N credit(s) <of/from ...> A and B" (complAmountAndRe)
// as "A and B" when each side's lightest course adds up to N: the amount
// restates them.
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

// complAllAmountOfRe: "All (2.0 credits) of GGR270H1, GGR272H1, GGR273H1,
// GGR373H1".
var complAllAmountOfRe = regexp.MustCompile(`(?i)^all\s*\(\s*(\d+(?:\.\d+)?)\s+credits?\s*\)\s+of:?\s+((?:` + codePat + `\s*,?\s*(?:and\s+)?)+)\.?$`)

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

// joinCodePairs separates two codes the calendar wrote with only a space
// ("SLA322H1 SLA410H1", sic) by the list's own separator: "/" in a list of
// alternatives ("GGR343H1 GGR344H1/ JPA331H1" in a "/" list, ASSPE1469),
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

// complSlashSepRe: text ending in a code and "/": the code before a pair is
// another alternative.
var complSlashSepRe = regexp.MustCompile(codePat + `\s*/\s*$`)

var (
	orEndRe   = regexp.MustCompile(`(?i)\bor$`)
	orStartRe = regexp.MustCompile(`(?i)^or\b`)
)

// matchingOpen returns the index of the "(" that the ")" at close closes, or
// -1.
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
