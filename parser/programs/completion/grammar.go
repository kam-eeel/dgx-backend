package completion

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"degree-explorer/parser/grammar"
	"degree-explorer/parser/programs"
)

// Grammar extensions for program completion text: only LexCompletion and
// parseCompletionText use them, so requisites and enrolment never see them.

var (

	// withAtLeastRe matches a floor read as "including": ", with at least
	// a 0.5 credit".
	withAtLeastRe = regexp.MustCompile(`(?i),\s*with\s+` + grammar.AtLeastPat + `\s+(?:an?\s+)?` + grammar.AmountPat + `\s+credits?\b`)
	// doubledFromRe matches "from from" (sic); furtherRe matches
	// "credits further from".
	doubledFromRe = regexp.MustCompile(`(?i)\bfrom\s+from\b`)
	furtherRe     = regexp.MustCompile(`(?i)\b(credits?)\s+further\s+(from|of)\b`)
	// codePairRe matches a list missing its comma: "SLA322H1 SLA410H1" (sic).
	codePairRe = regexp.MustCompile(`(` + grammar.CodePat + `|\b[A-Z]{3}\d{4}[HY]\d?)\s+(` + grammar.CodePat + `|[A-Z]{3}\d{4}[HY]\d?\b)`)
	// amountAndRe matches an amount over courses joined by "and": "1.0
	// credit from GGR217H1 and JGI346H1/ JGU346H1".
	amountAndRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\s+(?:(?:of|in)\s+[^:]*:|from:?|[^:]*:)\s*((?:` + grammar.CodePat + `\s*/\s*)*` + grammar.CodePat + `)\s+and\s+((?:` + grammar.CodePat + `\s*/\s*)*` + grammar.CodePat + `)\.?$`)
	// titledAmountRe matches a titled amount over a list, the title dropped:
	// "A set of mandatory courses (3.5 credits): ".
	titledAmountRe = regexp.MustCompile(`^(?:[A-Z][^:()]{0,60}?)\s*\(\s*` + grammar.AmountPat + `\s+credits?\s*\)\s*:\s*`)
	// codedCoursesRe matches "PCJ-coded courses", read "PCJ courses".
	codedCoursesRe = regexp.MustCompile(`\b([A-Z]{3})-coded\s+courses\b`)
	// descParenRe matches a lowercase description of the course before it:
	// "EEB240H1 (plant or animal form and function/environmental biology)".
	descParenRe = regexp.MustCompile(`(` + grammar.CodePat + `)\s*\(\s*[a-z][a-z ,/&-]*\)`)
	// oneCreditRe matches "including one credit", read "1.0 credit".
	oneCreditRe = regexp.MustCompile(`(?i)(\b(?:including|of\s+which|with)\s+(?:` + grammar.AtLeastPat + `\s+)?)one\s+credit\b`)
	// atCampusRe matches a campus the code already says: "ANT418H5 at UTM".
	atCampusRe = regexp.MustCompile(`(` + grammar.CodePat + `)\s*(?:\(\s*)?(?:at|from)\s+(?:UTM|UTSC)\b\)?`)
	// onTheLevelRe matches "on the 300-level", read "at the 300-level".
	onTheLevelRe = regexp.MustCompile(`(?i)\bon\s+the\s+(` + grammar.LevelPat + `)[-\s]level\b`)
	// oneOrBothRe matches either course: "one or both of ( MGY481H1 and
	// MGY482H1)".
	oneOrBothRe = regexp.MustCompile(`(?i)\bone\s+or\s+both\s+of\s+\(\s*(` + grammar.CodePat + `)\s+and\s+(` + grammar.CodePat + `)\s*\)`)
	// gradeParenRe matches a grade after a course: "(minimum grade of 63%)".
	gradeParenRe = regexp.MustCompile(`(?i)\(\s*(?:a\s+)?(?:minimum|min\.?)\s+(?:final\s+)?(?:grade|mark)\s+of\s+(?:` + grammar.AtLeastPat + `\s+)?(\d{2})\s*%\s*\)`)
	// codeParenCodeRe matches the course or the one in brackets: "CHM136H1
	// ( CHM138H1)".
	codeParenCodeRe = regexp.MustCompile(`(` + grammar.CodePat + `)\s*\(\s*(` + grammar.CodePat + `)\s*\)`)
	// quotedTitleRe matches a title in quotes: "GER290H1 ‘Global Issues /
	// German Contexts’".
	quotedTitleRe = regexp.MustCompile(`(` + grammar.CodePat + `)\s*‘[^’]*’`)
	// excludingAndRe matches the last excluded course: "excluding GER150H1,
	// GER361H1, and GER410H1".
	excludingAndRe = regexp.MustCompile(`(?i)(\bexcluding\s+(?:` + grammar.CodePat + `\s*,\s*)+)and\s+(` + grammar.CodePat + `)`)
	// subjectDesigRe matches subjects' designators: "in music history (HMU)".
	subjectDesigRe = regexp.MustCompile(`\b(in|or)\s+(?:[a-z]+\s+){1,3}\(\s*([A-Z]{3})\s*\)`)
	// orDesigParenRe matches either designator in "MAT (or APM) courses",
	// which normalize writes "MAT or (APM)".
	orDesigParenRe = regexp.MustCompile(`\b([A-Z]{3})\s+(?i:or)\s+\(\s*([A-Z]{3})\s*\)`)
	// inAdditionRe matches a course required elsewhere that doesn't count
	// here too: "(in addition to APM462H1)".
	inAdditionRe = regexp.MustCompile(`(?i)\s*\(\s*in\s+addition\s+to\s+((?:` + grammar.CodePat + `(?:\s*(?:,|and|/)\s*)?)+)\s*\)`)
	// capDesigRe matches a cap on designators: "no more than 0.5 credit can
	// be IRE/IRW".
	capDesigRe = regexp.MustCompile(`(?i)(` + grammar.NoMoreThanPat + `\s+` + grammar.NumPat + `)\s+credits?\s+(?:can|may)\s+be\s+(?:in\s+)?(` + grammar.DesigPat + `(?:\s*/\s*` + grammar.DesigPat + `)*)\b`)
	// normallyTakenRe matches timing, a note: ", normally taken in the first
	// year".
	normallyTakenRe = regexp.MustCompile(`(?i),\s*((?:normally|usually|typically|(?:(?:highly|strongly)\s+)?recommended\s+to\s+be)\s+taken\s+in\s+(?:the\s+)?(?:first|second|third|fourth)\s+year)\s*[;.]?\s*$`)
	// codeTitleRe matches a course's title after a colon: "HIS268H1: Law
	// and History".
	codeTitleRe = regexp.MustCompile(`^(` + grammar.CodePat + `)\s*:\s+[A-Z][A-Za-z ,&'’-]*\.?$`)
	// articleAmountRe matches the article in "or a 0.5 credit".
	articleAmountRe = regexp.MustCompile(`(?i)\b(or|and)\s+an?\s+(\d+\.\d+\s+credits?)\b`)
	// breadthNameRe matches a breadth category's name after its number:
	// "Category 5: The Physical and Mathematical Universes".
	breadthNameRe = regexp.MustCompile(`(?i)(\bcategor(?:y|ies)\s+[1-5])\s*[:(]\s*(?:creative\s+and\s+cultural\s+representations|thought,\s+belief,?\s+and\s+behaviou?r|society\s+and\s+its\s+institutions|living\s+things\s+and\s+their\s+environment|the\s+physical\s+(?:and|or)\s+mathematical\s+universes?)\)?`)
	// gradePlusRe matches a grade with a plus: "(80%+)".
	gradePlusRe = regexp.MustCompile(`\(\s*(\d{2})%\s*\+\s*\)`)
	// wholeAmountRe matches a whole-number amount: "3 credits".
	wholeAmountRe = regexp.MustCompile(`(^|[\s(])(\d{1,2})(\s+credits?\b)`)
	// mustBeFromRe matches a pool: "1.0 credit must be taken from".
	mustBeFromRe = regexp.MustCompile(`(?i)^(\d+\.\d+\s+credits?)\s+(?:must\s+be\s+(?:taken|chosen|selected)|chosen|selected)\s+from\b`)
	// ieRe matches the text before restated, a note: ", i.e., 2.0 SOC
	// credits at the 200 OR 300-level".
	ieRe = regexp.MustCompile(`(?i),?\s*\b(i\.e\.,?\s+[^;]*)$`)

	// eitherDesigRe matches a credit phrase's designators, not alternatives: "in
	// either GRK or LAT".
	eitherDesigRe = regexp.MustCompile(`\b((?i:in|from))\s+(?i:either)\s+([A-Z]{3}\s+(?i:or)\s+[A-Z]{3})\b`)

	// itemLabelRe matches a label before an item's body: "Geography: JGE321H1".
	// The body (itemLabelBodyRe) starts with a code, a bracket, an amount or
	// another label.
	itemLabelRe       = regexp.MustCompile(`^(?:\*\*|_)?([A-Z][^:;]{0,100}?):(?:\*\*|_)?\s+`)
	itemLabelBodyRe   = regexp.MustCompile(`^(?:` + grammar.CodePat + `|\(|\d+\.\d|(?i:an?\s+)?(?i:additional|further|any|all|one|two|three)\s+(?:\d+\.\d|\(?\d|[a-z])|(?:\*\*|_)?[A-Z][^:;]{0,100}?:)`)
	itemLabelRejectRe = regexp.MustCompile(`\d\.\d|` + grammar.CodePat + `|\bAS[A-Z]{3}\d{4}|^For\s|^(?i:from|in|of|at|with|to)\b|(?i:\b(?:sub)?(?:groups?|lists?|clusters?)\s+[a-z0-9]\b|\btotal\b)|(?i:\bstudents?$|\b(?:of|from|in|following)$|recommend|suggest|optional)`)

	// Annotations that say nothing about what is required.
	codeRecommendedRe = regexp.MustCompile(`(` + grammar.CodePat + `)\s*\(\s*(?i:(?:strongly|highly)\s+)?(?i:recommended)\s*\)`) // "STA288H1 (Recommended)"
	recommendedEndRe  = regexp.MustCompile(`\s+(?i:(?:is|are)\s+)?(?i:(?:strongly|highly)\s+)?(?i:recommended)\.?\s*\)`)         // "( CHM151Y1 strongly recommended)"
	recommendedListRe = regexp.MustCompile(`\(\s*(?i:recommended)\s*:\s*([^()]*)\)`)                                             // "(Recommended: HMB433H1)"
	recommendedTailRe = regexp.MustCompile(`,\s*((?:` + grammar.CodePat + `\s*(?:[,/]|(?i:and|or))?\s*)+)\s+(?i:recommended)\s*\.?$`)
	recommendedItemRe = regexp.MustCompile(`^(?:` + grammar.CodePat + `|[\s,;/()]|(?i:and|or)\b)*$`)
	// weRecommendRe matches "; while either pair of courses is accepted, we
	// strongly recommend ( STA257H1, STA261H1)".
	weRecommendRe     = regexp.MustCompile(`[;,.]\s*((?i:while\s+[^;]*?,\s*)?(?i:we\s+(?:strongly\s+|highly\s+)?recommend)\b[^;]*)$`)
	noLongerOfferedRe = regexp.MustCompile(`\s*\(\s*(?i:no\s+longer\s+offered)\s*\)`)

	// pairYBracketRe and pairYWholeRe match a Y course after "/" that
	// replaces the H pair before it: "( MAT148H1, MAT149H1/ MAT137Y1)", where the
	// calendar may never close the bracket. In a longer list "/" binds tighter.
	pairYBracketRe = regexp.MustCompile(`\(\s*(` + grammar.CodePat + `)\s*(?:,|(?i:and))\s*(` + grammar.CodePat + `)\s*/\s*(` + grammar.CodePat + `)\s*(\)|;|$)`)
	pairYWholeRe   = regexp.MustCompile(`^((?i:` + grammar.NumPat + `\s+credits?\s+from:?\s+)?)(` + grammar.CodePat + `)\s*,\s*(` + grammar.CodePat + `)\s*/\s*(` + grammar.CodePat + `)$`)
	footnoteNoteRe = regexp.MustCompile(`\s*\(\s*\*+\s*([^()]*)\)`) // "(* if not taken for the requirement immediately above)"
	// afterCodeParenRe matches brackets after a course that may only name
	// it, point to a note or say when to take it: "HIS389H1 (Hungarian History)".
	// One with a code, a number or a condition word isn't one.
	afterCodeParenRe = regexp.MustCompile(`(` + grammar.CodePat + `|\))\s*\(([^()]*)\)`)
	titleWordsRe     = regexp.MustCompile(`^[A-Za-z][A-Za-z&,'’ -]*$`)
	titleStopRe      = regexp.MustCompile(`(?i)\b(?:or|and/or|with|without|if|unless|only|taught|in|at|by|from|for|to|approved|approval|permission|consent|equivalents?|required|recommended|not|no|must|may|can|exclud\w*|except|minimum|maximum|grade|credits?|courses?|level|half|full|students?|lecture|lab|online)\b`)
	noteRefRe        = regexp.MustCompile(`(?i)^(?:see\s+)?notes?(?:\s+\d+[a-z]?)?$|^\d$`)
	takenWhenRe      = regexp.MustCompile(`(?i)^(?:(?:may|can|should)\s+be|normally|usually|typically)\s+taken\s+(?:in|during)\s+(?:the\s+)?(?:year\s+\d|(?:first|second|third|fourth)\s+year)$|^if\s+not\s+(?:already\s+)?(?:taken|used|counted)\b|^first\s+offer(?:ed|ing)\s+in\s+\d{4}(?:-\d{2,4})?$`)
	// recommendedSlashRe matches "CHM249H1 strongly recommended/": the
	// course stays an alternative.
	recommendedSlashRe = regexp.MustCompile(`(` + grammar.CodePat + `)\s+(?i:(?:strongly|highly)\s+)?(?i:recommended)\s*/`)
	// listCreditsRe matches the credits a list adds up to: "TMU240Y1 (1.5
	// credits)".
	listCreditsRe = regexp.MustCompile(`(` + grammar.CodePat + `|\))\s*\(\s*` + grammar.NumPat + `\s+credits?\s*\)`)
)

// joinCompletionTitleRuns joins name runs as JoinTitleRuns does, with bare
// designators as words: "in either GRK or LAT" stays one phrase.
func joinCompletionTitleRuns(src string, toks []grammar.Token) []grammar.Token {
	bare := map[[2]int]grammar.Token{}
	for i, t := range toks {
		if t.Kind == grammar.TWild && t.Node.Levels == nil {
			bare[[2]int{t.Start, t.End}] = t
			toks[i].Kind = grammar.TText
		}
	}
	toks = grammar.JoinTitleRuns(src, toks)
	for i, t := range toks {
		if w, ok := bare[[2]int{t.Start, t.End}]; ok && t.Kind == grammar.TText {
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
		if _, ok := grammar.MatchPool(s); ok {
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
	s = recommendedSlashRe.ReplaceAllString(s, "$1/")
	s = listCreditsRe.ReplaceAllString(s, "$1")
	s = noLongerOfferedRe.ReplaceAllString(s, "")
	for _, m := range footnoteNoteRe.FindAllStringSubmatch(s, -1) {
		notes = append(notes, grammar.CleanNote(m[1]))
	}
	s = footnoteNoteRe.ReplaceAllString(s, "")
	s = afterCodeParenRe.ReplaceAllStringFunc(s, func(m string) string {
		g := afterCodeParenRe.FindStringSubmatch(m)
		inner := strings.TrimSpace(g[2])
		switch {
		case takenWhenRe.MatchString(inner):
			notes = append(notes, inner)
		case noteRefRe.MatchString(inner), titleWordsRe.MatchString(inner) && !titleStopRe.MatchString(inner), isCourseTitle(inner):
		default:
			return m
		}
		return g[1]
	})
	for _, m := range recommendedListRe.FindAllStringSubmatch(s, -1) {
		notes = append(notes, "Recommended: "+grammar.CleanNote(m[1]))
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
		notes = append(notes, strings.ReplaceAll(grammar.CleanNote(s[open+1:end-1]), ";", ","))
		s = before + s[end:]
		from = len(before)
	}
	if m := weRecommendRe.FindStringSubmatchIndex(s); m != nil {
		notes = append(notes, grammar.CleanNote(s[m[2]:m[3]]))
		s = s[:m[0]]
	}
	if m := recommendedTailRe.FindStringSubmatchIndex(s); m != nil {
		notes = append(notes, grammar.CleanNote(s[m[2]:m[1]]))
		s = s[:m[0]]
	}
	return strings.TrimSpace(grammar.WsRe.ReplaceAllString(s, " ")), notes
}

// groupPairY brackets a pair of H courses that a Y course after "/"
// replaces: "( MAT148H1, MAT149H1)/ MAT137Y1". The three share a designator
// and the Y course is worth the pair.
func groupPairY(s string) string {
	pair := func(a, b, y string) bool {
		return a[:3] == y[:3] && b[:3] == y[:3] && grammar.CourseWeight(a) == 0.5 && grammar.CourseWeight(b) == 0.5 && grammar.CourseWeight(y) == 1
	}
	if m := pairYWholeRe.FindStringSubmatch(s); m != nil && pair(m[2], m[3], m[4]) {
		return m[1] + "( " + m[2] + ", " + m[3] + ")/ " + m[4]
	}
	var b strings.Builder
	last := 0
	for _, m := range pairYBracketRe.FindAllStringSubmatchIndex(s, -1) {
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
	if m := ieRe.FindStringSubmatchIndex(text); m != nil {
		if grammar.CodeRe.MatchString(text[m[2]:m[3]]) {
			notes = append(notes, grammar.CleanNote(text[m[2]:m[3]]))
		}
		text = text[:m[0]]
	}
	// the grade covers the slash list, as in enrolment; gradeParenRe first:
	// SlashGradeRe reads only the short form "(63%)"
	text = programs.SlashGradeRe.ReplaceAllStringFunc(gradeParenRe.ReplaceAllString(text, "($1%)"), programs.SpreadSlashGrade)
	text = rewriteOneOffWordings(text)
	text = codedCoursesRe.ReplaceAllString(text, "$1 courses")
	text = descParenRe.ReplaceAllString(text, "$1")
	text = onTheLevelRe.ReplaceAllString(text, "at the $1-level")
	text = oneCreditRe.ReplaceAllString(text, "${1}1.0 credit")
	text = atCampusRe.ReplaceAllString(text, "$1")
	text = oneOrBothRe.ReplaceAllString(text, "$1/ $2")
	text = titledAmountRe.ReplaceAllString(text, "$1 credits: ")
	text = amountAndCodes(text)
	text = allAmountOf(text)
	// after descParenRe and atCampusRe, which would hide the pair
	text = codeParenCodeRe.ReplaceAllString(text, "$1/ $2")
	text = excludingAndRe.ReplaceAllString(text, "$1 $2")
	text = subjectDesigRe.ReplaceAllString(text, "$1 $2")
	text = orDesigParenRe.ReplaceAllString(text, "$1/$2")
	text = inAdditionRe.ReplaceAllString(text, ", excluding $1")
	text = programs.AnyLevelRe.ReplaceAllString(capDesigRe.ReplaceAllString(text, "$1 $2 credits"), "")
	text = joinCodePairs(text)
	text = withAtLeastRe.ReplaceAllString(text, ", including at least $1 credit")
	text = wholeAmountRe.ReplaceAllString(text, "$1$2.0$3")
	// after wholeAmountRe: mustBeFromRe reads only a decimal amount
	text = mustBeFromRe.ReplaceAllString(text, "$1 from")
	text = furtherRe.ReplaceAllString(doubledFromRe.ReplaceAllString(text, "from"), "$1 $2")
	if m := normallyTakenRe.FindStringSubmatchIndex(text); m != nil {
		notes = append(notes, text[m[2]:m[3]])
		text = text[:m[0]]
	}
	text = codeTitleRe.ReplaceAllString(text, "$1")
	text = articleAmountRe.ReplaceAllString(breadthNameRe.ReplaceAllString(text, "$1"), "$1 $2")
	text = gradePlusRe.ReplaceAllString(text, "($1%)")
	// groupPairY last: it reads the list as the rewrites above leave it
	return groupPairY(eitherDesigRe.ReplaceAllString(text, "$1 $2")), notes
}

// amountAndCodes reads "N credits <of/from ...> A and B" as "A and B" when
// each side's lightest course adds up to N: the amount restates them.
func amountAndCodes(text string) string {
	m := amountAndRe.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil {
		return text
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	lightest := func(s string) float64 {
		w := 0.0
		for i, c := range grammar.CodeRe.FindAllString(s, -1) {
			if x := grammar.CourseWeight(c); i == 0 || x < w {
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

// allAmountOfRe matches "All (2.0 credits) of GGR270H1, GGR272H1,
// GGR273H1, GGR373H1".
var allAmountOfRe = regexp.MustCompile(`(?i)^all\s*\(\s*` + grammar.AmountPat + `\s+credits?\s*\)\s+of:?\s+((?:` + grammar.CodePat + `\s*,?\s*(?:and\s+)?)+)\.?$`)

// allAmountOf reads "All (N credits) of A, B, ..." as "All of A, B, ..."
// when the courses weigh N.
func allAmountOf(text string) string {
	m := allAmountOfRe.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil {
		return text
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	sum := 0.0
	for _, c := range grammar.CodeRe.FindAllString(m[2], -1) {
		sum += grammar.CourseWeight(c)
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
		m := codePairRe.FindStringSubmatchIndex(text)
		if m == nil {
			return text
		}
		sep := ", "
		if slashSepRe.MatchString(text[:m[2]]) || strings.HasPrefix(strings.TrimSpace(text[m[1]:]), "/") {
			sep = "/ "
		}
		text = text[:m[2]] + text[m[2]:m[3]] + sep + text[m[4]:]
	}
}

// slashSepRe matches text ending in a code and "/": the code before a
// pair is another alternative.
var slashSepRe = regexp.MustCompile(grammar.CodePat + `\s*/\s*$`)

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
