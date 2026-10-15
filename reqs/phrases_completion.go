package reqs

import (
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
	matchExcluding,
	matchEither,
	matchAllOf,
	matchCompletionPool,
	matchWild,
}, phraseMatchers...)

var (
	// "Either GGR491Y1 or one of ...": "either" before alternatives.
	eitherRe     = regexp.MustCompile(`^(?i:either)\b(?:\s*:)?`)
	eitherNextRe = regexp.MustCompile(`^\s*(?:` + codePat + `|\(|\d+\.\d|(?i:one|two|three|any|all)\s+\w|an?\s+\w)`)

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
	itemLabelBodyRe   = regexp.MustCompile(`^(?:` + codePat + `|\(|\d+\.\d|(?:\*\*|_)?[A-Z][^:;]{0,100}?:)`)
	itemLabelRejectRe = regexp.MustCompile(`\d\.\d|` + codePat + `|\bAS[A-Z]{3}\d{4}|^For\s|^(?i:from|in|of|at|with|to)\b|(?i:\b(?:sub)?(?:groups?|lists?|clusters?)\s+[a-z0-9]\b|\btotal\b)|(?i:\bstudents?$|\b(?:of|from|in|following)$|recommend|suggest|optional)`)

	// Annotations that say nothing about what is required.
	codeRecommendedRe = regexp.MustCompile(`(` + codePat + `)\s*\(\s*(?i:(?:strongly|highly)\s+)?(?i:recommended)\s*\)`) // "STA288H1 (Recommended)"
	recommendedEndRe  = regexp.MustCompile(`\s+(?i:(?:is|are)\s+)?(?i:(?:strongly|highly)\s+)?(?i:recommended)\s*\)`)    // "( CHM151Y1 strongly recommended)"
	recommendedListRe = regexp.MustCompile(`\(\s*(?i:recommended)\s*:\s*([^()]*)\)`)                                     // "(Recommended: HMB433H1)"
	recommendedTailRe = regexp.MustCompile(`,\s*((?:` + codePat + `\s*(?:[,/]|(?i:and|or))?\s*)+)\s+(?i:recommended)\s*\.?$`)
	recommendedItemRe = regexp.MustCompile(`^(?:` + codePat + `|[\s,/()]|(?i:and|or)\b)*$`)
	// "; while either pair of courses is accepted, we strongly recommend ( STA257H1, STA261H1)"
	weRecommendRe     = regexp.MustCompile(`[;,.]\s*((?i:while\s+[^;]*?,\s*)?(?i:we\s+(?:strongly\s+|highly\s+)?recommend)\b[^;]*)$`)
	noLongerOfferedRe = regexp.MustCompile(`\s*\(\s*(?i:no\s+longer\s+offered)\s*\)`)
	footnoteNoteRe    = regexp.MustCompile(`\s*\(\s*\*+\s*([^()]*)\)`) // "(* if not taken for the requirement immediately above)"
)

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
		if w, wild := matchWild(rest); !poolNextRe.MatchString(rest) &&
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
//   - "STA288H1 (Recommended)", "( CHM151Y1 strongly recommended)/ ( ... )":
//     the course stays an alternative;
//   - "( MAT237Y1 recommended)", "(Recommended: HMB433H1)" after a list, and
//     ", MAT401H1/ MAT402H1 recommended" at the end: notes;
//   - "WGS350H1 (no longer offered)": the code stays;
//   - "(* if not taken for the requirement immediately above)": a footnote
//     to a starred code (normalize drops the star), kept as a note.
func stripAnnotations(s string) (string, []string) {
	var notes []string
	s = codeRecommendedRe.ReplaceAllString(s, "$1")
	s = noLongerOfferedRe.ReplaceAllString(s, "")
	for _, m := range footnoteNoteRe.FindAllStringSubmatch(s, -1) {
		notes = append(notes, cleanNote(m[1]))
	}
	s = footnoteNoteRe.ReplaceAllString(s, "")
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
		notes = append(notes, cleanNote(s[open+1:end-1]))
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
