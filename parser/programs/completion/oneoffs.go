package completion

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/parser/grammar"
	"degree-explorer/parser/programs"
	"degree-explorer/types"
)

// The one-off passes: the entry points that run the readings of wordings one or
// two programs use (in completion_oneoffs_*.go), and the regexes they share.

var (
	// fromLabelLeadRe matches "From Group A: 1.0 credit or DRM220Y1", which
	// becomes the grammar's "1.0 credit from Group A or DRM220Y1".
	fromLabelLeadRe = regexp.MustCompile(`(?i)^from\s+(group\s+[A-Z]):\s*(` + grammar.NumPat + `\s+credits?)(\s+or\s+` + grammar.CodePat + `)?$`)
	// whenEligibleRe matches "other DRM (when eligible)": eligibility to
	// enrol, which a completed course has met.
	whenEligibleRe = regexp.MustCompile(`(?i)(\bother\s+[A-Z]{3})\s+\(when\s+eligible\)`)
)

// rewriteOneOffListWordings rewrites wordings of items that draw on lists into
// the list grammar's.
func rewriteOneOffListWordings(text string) string {
	text = fromLabelLeadRe.ReplaceAllString(strings.TrimSpace(text), "$2 from $1$3")
	return whenEligibleRe.ReplaceAllString(text, "$1")
}

// oneOffsAfterPools runs the one-off readings that need the routes placed and
// the lists read, after the stream, cognate and filter pools are made. It
// returns the lists with any it adds.
func (a *Assembler) oneOffsAfterPools(lists []*types.CourseList) []*types.CourseList {
	a.splitAlternatives()
	a.sharedEssay()
	a.oneOffRowPasses(lists)
	a.remainderTotalPool(lists)
	lists = a.coreGroupsMix(lists)
	lists = a.levelOrAnyLevel(lists)
	return a.totalCourseNote(lists)
}

// oneOffTextBody reads a row of text (alts: the routes beside it) with the
// one-off readers that give it a new body beside the routes, and reports
// whether it read it.
func (a *Assembler) oneOffTextBody(row *compRow, text *types.ReqNode, alts []*types.ReqNode, ls compListRefs) bool {
	s := strings.TrimSuffix(strings.TrimSpace(text.Text), ".")
	set := func(n *types.ReqNode) {
		row.r.Req = programs.OrNode(append([]*types.ReqNode{n}, alts...)...)
	}
	if n := exclusionAlts(s); n != nil && alts == nil {
		set(n)
		return true
	}
	if n := worldMusicAlt(s); n != nil {
		set(n)
		return true
	}
	if n := groupSubgroups(s, ls); n != nil {
		set(n)
		return true
	}
	if n, floor := furtherFloor(s); n != nil {
		set(n)
		k := &types.Requirement{Kind: types.RequirementConstraint, Req: programs.OrNode(append([]*types.ReqNode{floor}, alts...)...), Raw: row.r.Raw}
		a.rowUnder(row, k)
		return true
	}
	if n := a.listAbove(row, s); n != nil {
		set(n)
		return true
	}
	if n := researchAlts(s); n != nil {
		set(n)
		row.r.Notes = slices.DeleteFunc(row.r.Notes, func(x string) bool { return strings.Contains(s, strings.TrimSuffix(x, ".")) })
		return true
	}
	if n := averageAlts(s); n != nil {
		set(n)
		return true
	}
	return false
}

// oneOffRowPasses runs the one-off readings of rows that aren't one
// allocation of text.
func (a *Assembler) oneOffRowPasses(lists []*types.CourseList) {
	a.complementaryCourses()
	a.cognateTotal()
	a.departmentCap()
	a.restatedConstraints()
	a.distributionSummary()
	a.languageBanNote()
	walkRows(a.items, throughDaniels)
	walkRows(a.items, possibilities)
	walkRows(a.items, approvedSequences)
	walkRows(a.items, approvalOverAll)
	walkRows(a.items, a.trailingCode)
	byName := listsByName(lists)
	walkRows(a.items, func(r *compRow) { a.capstoneSubstitute(r, byName) })
	walkRows(a.items, oneDiscipline)
	a.noteFloorAfterLine()
	a.floorIfCompleted()
	walkRows(a.items, languagesNamed)
	a.progressiveSequence()
	walkRows(a.items, levelCodedRoute)
	walkRows(a.items, func(r *compRow) {
		if r.r.Kind != types.RequirementAllocation || r.r.Req == nil || r.r.Req.Type != types.NodeOr {
			return
		}
		if n := languageAtAnyLevel(r.r.Req); n != nil {
			r.r.Req = n
		} else if n := globalExperience(r); n != nil {
			r.r.Req = n
		} else if n := transferEquivalents(r.r.Req); n != nil {
			r.r.Req = n
		} else {
			a.listBelowMissing(r)
		}
	})
}

// dropRow takes r out of the rows.
func (a *Assembler) dropRow(r *compRow) {
	var drop func(rs []*compRow) []*compRow
	drop = func(rs []*compRow) []*compRow {
		rs = slices.DeleteFunc(rs, func(q *compRow) bool { return q == r })
		for _, q := range rs {
			q.children = drop(q.children)
		}
		return rs
	}
	a.items, a.program, a.floors = drop(a.items), drop(a.program), drop(a.floors)
	if a.total != nil {
		a.total.children = drop(a.total.children)
	}
}

// oneOffTextRow reads a row of text (text: the body, or the first item of
// or(text, routes)) with the one-off readers, once the lists are known. ok
// reports whether it read the row; rows is what stands for it.
func (a *Assembler) oneOffTextRow(row *compRow, text *types.ReqNode, ls compListRefs) (rows []*compRow, ok bool) {
	var alts []*types.ReqNode
	if n := row.r.Req; n != text {
		alts = n.Items[1:]
	}
	if a.additionalUnion(row, text.Text, alts, ls) {
		return nil, true // the row joins Req 1
	}
	if a.singleWordingAltRow(row, text, ls) {
		return []*compRow{row}, true
	}
	if text == row.r.Req && a.gatewayGroups(row) {
		return []*compRow{row}, true
	}
	if a.oneOffTextBody(row, text, alts, ls) {
		return []*compRow{row}, true
	}
	if a.unitRows(row, text, ls) {
		return []*compRow{row}, true
	}
	return a.singleWordingRow(row, text, ls)
}

// oneOffsBeforePools runs the one-off readings that need the routes placed and
// the lists read, before the stream, cognate and filter pools are made.
func (a *Assembler) oneOffsBeforePools() {
	a.areaCorrespondence()
	a.languageCourses()
	a.exclusionExceptions()
	a.designatorRules()
	a.restatedPools()
	a.breadthConditions()
	a.subjectRows()
	a.inlineDesignatorFloors()
	a.remainingFrom()
	a.consecutiveLanguages()
}

var (
	// fromOneOfRe matches "credit from one of PHY224H1", which means
	// "credit from PHY224H1".
	fromOneOfRe = regexp.MustCompile(`(?i)\b(credits?\s+from)\s+one\s+of\s+(` + grammar.CodePat + `)`)
	// allRequiredRe matches a lead over courses all required: "2.0 credits
	// comprised of the following core PCJ courses, all of which are required: ".
	allRequiredRe = regexp.MustCompile(`(?i)^` + grammar.NumPat + `\s+credits?\s+(?:comprised\s+)?of\s+the\s+following\s+[\w\s]*?courses,\s+all\s+of\s+which\s+are\s+required:\s*`)
	// listFloorRe matches a floor before its list: "At least 3.5 credits
	// from the following list including at least 2.5 with the BMS/JSM/JWB
	// designator: BMS300H1/ ...".
	listFloorRe = regexp.MustCompile(`(?i)^(?:` + grammar.AtLeastPat + `\s+)?(` + grammar.NumPat + `\s+credits?)\s+from\s+the\s+following\s+list\s+including\s+` + grammar.AtLeastPat + `\s+` + grammar.AmountPat + `\s+(?:credits?\s+)?with\s+the\s+(` + grammar.DesigPat + `(?:/` + grammar.DesigPat + `)*)\s+designators?:\s*(.+)$`)
	// groupFloorRe matches a group set out after its floors: "2.0 credits
	// from the Core Group at the 300+ level, at least 0.5 of which must be at the
	// 400-level: CAR315H1, ...". The groups: amount, level, floors, codes.
	groupFloorRe = regexp.MustCompile(`(?i)^(` + grammar.NumPat + `\s+credits?)\s+from\s+the\s+[\w\s]+?\s+group(?:\s+at\s+the\s+([1-4])00\+\s+level)?(?:,\s*|\s*\(\s*)((?:including\s+)?` + grammar.AtLeastPat + `\s+[^():]+?)\)?:\s*((?:` + grammar.CodePat + `,?\s*)+)$`)
	// whichFloorRe matches "at least 0.5 of which must be at the 400-level".
	whichFloorRe = regexp.MustCompile(`(?i)^` + grammar.AtLeastPat + `\s+` + grammar.AmountPat + `\s+of\s+which\s+must\s+be\s+(at\s+the\s+` + grammar.LevelPat + `-level)$`)
	// desigButNotRe matches "in courses with a GER designator, but cannot be
	// GER400H1 or GER401H1".
	desigButNotRe = regexp.MustCompile(`(?i)\bin\s+courses\s+with\s+an?\s+(` + grammar.DesigPat + `)\s+designator,\s+but\s+cannot\s+be\s+(` + grammar.CodePat + `)\s+or\s+(` + grammar.CodePat + `)`)
	// furtherDesigRe matches "in further ITA courses, including language
	// courses": the language courses are ITA courses too.
	furtherDesigRe = regexp.MustCompile(`(?i)\bin\s+further\s+(` + grammar.DesigPat + `)\s+courses(?:,\s+including\s+language\s+courses$)?`)
	// onlyOneFromRe matches a cap at the end of a "/" list: "/ (only one
	// from ESS261H1/ GGR203H1)".
	onlyOneFromRe = regexp.MustCompile(`(?i)/?\s*\(\s*only\s+one\s+from\s+((?:` + grammar.CodePat + `\s*/\s*)*` + grammar.CodePat + `)\s*\)$`)
	// selectedThenPairRe matches a pair after a "/" pool, which ends the
	// pool (ASMAJ0525): "credit selected from FRE210H1/ JFG388H1, ( FSL320H1,
	// FSL322H1)/ FSL321Y1". meetsAsWellRe matches "The Physics course
	// JPH441H1 meets this requirement as well as any of the following courses:".
	selectedThenPairRe = regexp.MustCompile(`(?i)(\bcredits?\s+selected\s+from\s+(?:` + grammar.CodePat + `\s*/\s*)+` + grammar.CodePat + `),\s*(\(\s*` + grammar.CodePat + `\s*,\s*` + grammar.CodePat + `\s*\)\s*/\s*` + grammar.CodePat + `\.?)$`)
	meetsAsWellRe      = regexp.MustCompile(`(?i)^(?:[\w\s&,]+\s+requirement:\s*)?the\s+\w+\s+course\s+(` + grammar.CodePat + `)\s+meets\s+this\s+requirement\s+as\s+well\s+as\s+any\s+of\s+the\s+following\s+courses:\s*`)
)

// rewriteOneOffWordings rewrites wordings one or two items use into the
// grammar's.
func rewriteOneOffWordings(text string) string {
	text = fromOneOfRe.ReplaceAllString(text, "$1 $2")
	text = selectedThenPairRe.ReplaceAllString(text, "$1; $2")
	if m := groupFloorRe.FindStringSubmatch(text); m != nil {
		floor := strings.TrimSpace(m[3])
		if f := whichFloorRe.FindStringSubmatch(floor); f != nil {
			floor = "including at least " + f[1] + " credit " + f[2]
		}
		codes := grammar.CodeRe.FindAllString(m[4], -1)
		// the group's level is the codes' own, which the grammar's pool says
		low := 0
		if m[2] != "" {
			low = int(m[2][0]-'0') * 100
		}
		if strings.HasPrefix(strings.ToLower(floor), "including") && !slices.ContainsFunc(codes, func(c string) bool { return programs.CourseLevel(c) < low }) {
			text = m[1] + " from " + strings.Join(codes, ", ") + ", " + floor
		}
	}
	text = meetsAsWellRe.ReplaceAllString(text, "$1/ ")
	text = desigButNotRe.ReplaceAllString(text, "in $1 courses, excluding $2, $3")
	text = furtherDesigRe.ReplaceAllString(text, "in $1 courses")
	if m := onlyOneFromRe.FindStringSubmatchIndex(text); m != nil {
		// one course of the bracket, when each is a half course: at most 0.5 credit
		codes := text[m[2]:m[3]]
		if !slices.ContainsFunc(grammar.CodeRe.FindAllString(codes, -1), func(c string) bool { return grammar.CourseWeight(c) != 0.5 }) {
			text = strings.TrimSpace(text[:m[0]]) + "/ " + codes + "; up to 0.5 credit from " + codes
		}
	}
	text = listFloorRe.ReplaceAllString(text, "$1 from $4, including at least $2 credits from $3 courses")
	return allRequiredRe.ReplaceAllString(text, "")
}

var (
	// breadthUnlessRe matches the Faculty's breadth rule on a program's
	// students: "... requires Art History Majors who do not complete FAH275H1, ...
	// to complete at least one 0.5 credit ... in Breadth Category 5".
	breadthUnlessRe = regexp.MustCompile(`(?i)^in\s+addition,\s+the\s+faculty\s+of\s+arts\s+and\s+science\s+requires\s+[\w\s]+?\s+who\s+do\s+not\s+complete\s+((?:` + grammar.CodePat + `)(?:,?\s*(?:or\s+)?(?:` + grammar.CodePat + `))*)\s+to\s+complete\s+` + grammar.AtLeastPat + `\s+(?:one\s+)?` + grammar.AmountPat + `\s+credits?\s+in\s+arts\s+and\s+science\s+courses\s+in\s+breadth\s+category\s+([1-5])\b`)
	// breadthOrGroupsRe matches Cinema Studies' three-part item: "In
	// addition, CIN230H1 (a Breadth Category 3 course) or 0.5 credit from Breadth
	// Category 5; and at least 3.0 credits from Groups B through G, ...".
	breadthOrGroupsRe = regexp.MustCompile(`(?i)^in\s+addition,\s+(` + grammar.CodePat + `)\s+\(a\s+breadth\s+category\s+[1-5]\s+course\)\s+or\s+(?:a\s+)?` + grammar.AmountPat + `\s+credits?\s+from\s+breadth\s+category\s+([1-5]);\s+and\s+(.+)$`)
	// groupsThroughRe matches "at least 3.0 credits from Groups B through
	// G, of which 2.0 credits must be at the 300/400-level, and only a maximum of
	// 1.5 credits can be from Group G".
	groupsThroughRe = regexp.MustCompile(`(?i)^(?:` + grammar.AtLeastPat + `\s+)?` + grammar.AmountPat + `\s+credits\s+from\s+groups\s+((?-i:[A-Z]))\s+through\s+((?-i:[A-Z])),\s+of\s+which\s+` + grammar.AmountPat + `\s+credits\s+must\s+be\s+at\s+the\s+(\d)00/(\d)00-level,\s+and\s+(?:only\s+)?a\s+maximum\s+of\s+` + grammar.AmountPat + `\s+credits\s+can\s+be\s+from\s+group\s+((?-i:[A-Z]))\b[^,;]*$`)
	// namedAndBelowRe matches "2.0 credits drawn from the foundational
	// courses list and from the elective courses listed below".
	namedAndBelowRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\s+drawn\s+from\s+the\s+(\w+)\s+courses\s+list\s+and\s+from\s+the\s+elective\s+courses\s+listed\s+below$`)
	// remainingBelowRe matches "Remaining courses to be selected from the
	// list of eligible electives below", whose amount a note gives.
	remainingBelowRe = regexp.MustCompile(`(?i)^remaining\s+courses\s+to\s+be\s+selected\s+from\s+the\s+list\s+of\s+eligible\s+electives\s+below$`)
	// remainingAmountRe matches that note: "The remaining 1.5 credits in
	// elective courses chosen from a list of eligible electives".
	remainingAmountRe = regexp.MustCompile(`(?i)^the\s+remaining\s+` + grammar.AmountPat + `\s+credits\s+in\s+elective\s+courses\s+chosen\s+from\s+a\s+list\s+of\s+eligible\s+electives\b`)
	// oneLanguageRe matches "1.0 credit from one of the following
	// languages", with an optional non-language alternative after it. "one the"
	// is ASMAJ1626's typo.
	oneLanguageRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\s+from\s+one\s+(?:of\s+)?the\s+following\s+languages(?:,\s+or\s+(` + grammar.NumPat + `\s+credits?\s+in\s+non-language\s+courses\s+related\s+to\s+your\s+chosen\s+language\s+of\s+study,\s+selected\s+in\s+consultation\s+with\s+the\s+program\s+coordinator))?:?$`)
	// languageLineRe matches a language and its courses: "- German:
	// GER200Y1/ GER300Y1/ ( GER370H1 and GER372H1)".
	languageLineRe = regexp.MustCompile(`^-\s*([A-Z][A-Za-z ,]+?):\s*(.+?)\s*$`)
	// languageCodesRe matches a language's courses joined by "/" and "and",
	// in brackets or not: "GER200Y1/ ( GER370H1 and GER372H1)".
	languageCodesRe = regexp.MustCompile(`^(?:\(?\s*` + grammar.CodePat + `(?:\s+and\s+` + grammar.CodePat + `)*\s*\)?)(?:\s*/\s*\(?\s*` + grammar.CodePat + `(?:\s+and\s+` + grammar.CodePat + `)*\s*\)?)*$`)
	// coreLeadRe matches a lead whose lines are its parts: "3.5 credits of
	// ESS core courses:".
	coreLeadRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits\s+of\s+` + grammar.DesigPat + `\s+core\s+courses:$`)
	// corePartRe matches a part of courses: "0.5 credit of field courses:
	// ESS410H1 (...)/ ...".
	corePartRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\s+(?:chosen\s+from\s+the\s+following\s+courses|of\s+\w+\s+courses):\s*(.+)$`)
	// codeTitledRe matches a course and its title in brackets: "ESS224H1
	// (Intro to Mineralogy and Petrology)".
	codeTitledRe = regexp.MustCompile(`(` + grammar.CodePat + `)\s*\([^()]*\)`)
	// categoriesPartRe matches a part drawn from categories under the lead:
	// "1.5 credit at the 200 and 300-level (0.5 credits must be taken from each
	// category):".
	categoriesPartRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\s+at\s+the\s+\d00\s+and\s+\d00-level\s+\(` + grammar.AmountPat + `\s+credits\s+must\s+be\s+taken\s+from\s+each\s+category\):$`)
	// categoryLineRe matches a category and its courses: "Biology
	// processes: ESS261H1/ ESS363H1".
	categoryLineRe = regexp.MustCompile(`^[A-Z][a-z]+\s+processes:\s*((?:` + grammar.CodePat + `)(?:\s*/\s*` + grammar.CodePat + `)*)$`)
	// electivesLeadRe matches "2.5 credits of elective courses:".
	electivesLeadRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits\s+of\s+elective\s+courses:$`)
	// categoriesNoteRe matches its note: "This requirement can be satisfied
	// by any of the Earth Sciences Courses listed in categories A to F below".
	categoriesNoteRe = regexp.MustCompile(`(?i)^this\s+requirement\s+can\s+be\s+satisfied\s+by\s+any\s+of\s+the\s+[\w\s]+?\s+courses\s+listed\s+in\s+categories\s+((?-i:[A-Z]))\s+to\s+((?-i:[A-Z]))\s+below$`)
	// notLimitRe matches a note that may widen the categories: "neither
	// mutually exclusive nor meant to limit choice".
	notLimitRe = regexp.MustCompile(`(?i)\bneither\s+mutually\s+exclusive\s+nor\s+meant\s+to\s+limit\s+choice\b`)
	// categoryCodesRe matches a category kept as a note: "Paleoclimate:
	// ESS245H1/ ESS345H1, ESS261H1".
	categoryCodesRe = regexp.MustCompile(`^[A-Z][A-Za-z ]+:\s*((?:` + grammar.CodePat + `)(?:\s*[/,]\s*` + grammar.CodePat + `)*)$`)
	// amountFromRe matches a lead whose pool its route gives: "0.5 credit
	// from:".
	amountFromRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\s+from:$`)
	// aboveNotCountedRe matches that route: "any of the above from Year 3
	// Req. 2-4 not previously counted or with permission of the Director:
	// ECO351H1/ TRN307H1".
	aboveNotCountedRe = regexp.MustCompile(`(?i)^any\s+of\s+the\s+above\s+from\s+(year\s+\d)\s+req\.\s*(\d)\s*-\s*(\d)\s+not\s+previously\s+counted\s+or\s+(with\s+permission\s+of\s+the\s+\w+:\s*((?:` + grammar.CodePat + `)(?:\s*/\s*` + grammar.CodePat + `)*))$`)
	// inLanguagesRe matches credits in languages the text names: "3.0
	// credits in Hebrew or Yiddish". Groups 2 and 3 are the two forms of the list.
	inLanguagesRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits\s+in\s+(?:([A-Z][a-z]+(?:\s+or\s+[A-Z][a-z]+)+)|a\s+language\s+relevant\s+to\s+[\w\s]+?\s+[–-]\s+([A-Z][a-z]+(?:,\s+[A-Z][a-z]+)+)(?:\s+\([^()]*\))?)$`)
	// researchCourseRe matches "1.0 credit in a Research Course (a 299Y1,
	// 399Y1, on a Renaissance Studies topic, or REN392H1/ REN392Y1)".
	researchCourseRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\s+in\s+a\s+research\s+course\s+\((a\s+299Y1,\s+399Y1,\s+on\s+a\s+[\w\s]+?\s+topic),\s+or\s+((?:` + grammar.CodePat + `)(?:\s*[/,]\s*` + grammar.CodePat + `)*)\)$`)
	// codeRunRe matches codes joined by "/" and ",". languageApprovedRe
	// matches an approved language credit: "SLA337H1, and additional 0.5 credit
	// can be fulfilled through ... approved in joint consultation with ...".
	codeRunRe          = regexp.MustCompile(`^` + grammar.CodePat + `(?:\s*[/,]\s*` + grammar.CodePat + `)*$`)
	languageApprovedRe = regexp.MustCompile(`(?i)^(?:(` + grammar.CodePat + `),\s+and\s+)?((?:additional\s+)?` + grammar.NumPat + `\s+credits?\s+can\s+be\s+fulfilled\s+through\s+[^.]+?\bapproved\s+in\s+joint\s+consultation\s+with\s+[^.]+)\.`)
)

// singleWordingRow reads a row of text by the single-wording rules for rows. ok
// reports whether it read the row; rows is what stands for it.
func (a *Assembler) singleWordingRow(row *compRow, text *types.ReqNode, ls compListRefs) (rows []*compRow, ok bool) {
	if text == row.r.Req && a.coreParts(row, ls) {
		return []*compRow{row}, true
	}
	if text != row.r.Req || len(row.children) > 0 {
		return nil, false
	}
	if rows, ok := a.breadthOrGroups(row, ls); ok {
		return rows, true
	}
	if a.namedAndBelow(row, ls) || a.categoryElectives(row) {
		return []*compRow{row}, true
	}
	return nil, false
}

// singleWordingAltRow reads a row whose body is or(text, routes) by the
// single-wording rules for such rows; the routes stay its alternatives.
func (a *Assembler) singleWordingAltRow(row *compRow, text *types.ReqNode, ls compListRefs) bool {
	if len(row.children) > 0 {
		return false
	}
	var alts []*types.ReqNode
	if n := row.r.Req; n != text {
		alts = n.Items[1:]
	}
	return a.remainingBelow(row, text, alts, ls) || a.oneLanguage(row, text, alts) || a.aboveNotCounted(row, text, alts) ||
		a.twoGroups(row, text, alts, ls) || a.coreOrGroups(row, text, alts, ls)
}

var (
	// languagePlacementRe matches a language requirement's placement route:
	// "Students entering the program with proof of requisite linguistic
	// proficiency in Aramaic, ... will choose any 3.0 credits".
	languagePlacementRe = regexp.MustCompile(`(?i)^students\s+entering\s+the\s+program\s+with\s+proof\s+of\s+requisite\s+linguistic\s+proficiency\b.*?\bwill\s+choose\s+any\s+` + grammar.AmountPat + `\s+credits\b`)
	// sectionsFloorRe matches "5. Note: From section 3 and 4, at least 4.0
	// credits at the 300+ level.".
	sectionsFloorRe = regexp.MustCompile(`(?i)^(?:note:\s*)?from\s+sections?\s+(\d{1,2})\s+and\s+(\d{1,2}),\s+` + grammar.AtLeastPat + `\s+` + grammar.AmountPat + `\s+credits?\s+at\s+the\s+([1-4])00\+\s+level$`)
	// reqsFloorRe matches "At least 2.0 credits from 400-level/ D-level
	// courses & Graduate courses in Req5 or Req6 or Req7".
	reqsFloorRe = regexp.MustCompile(`(?i)^` + grammar.AtLeastPat + `\s+` + grammar.AmountPat + `\s+credits?\s+from\s+([1-4])00-level\s*/\s*D-level\s+courses\s*&\s*graduate\s+courses\s+in\s+(req\s*\d{1,2}(?:\s+or\s+req\s*\d{1,2})+)$`)
	// linesBelowRe matches "1.0 credit from AST425Y1/ ( AST424H1, and 0.5
	// credit from lines 2 or 3 below; see notes);" (ASSPE0271). The groups are the
	// amount, the course, the other course, its extra amount, the two lines.
	linesBelowRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\s+from\s+(` + grammar.CodePat + `)\s*/\s*\(\s*(` + grammar.CodePat + `),?\s+and\s+` + grammar.AmountPat + `\s+credits?\s+from\s+lines\s+(\d)\s+or\s+(\d)\s+below(?:;\s*see\s+notes)?\s*\);?$`)
	// listAnyLevelRe matches "2.0 credits from the following list, including
	// at least 1.0 credit at the 400-level (see below for additional conditions):
	// ..." (ASSPE1687).
	listAnyLevelRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits\s+from\s+the\s+following\s+list,\s+including\s+` + grammar.AtLeastPat + `\s+` + grammar.AmountPat + `\s+credits?\s+at\s+the\s+([1-4])00-level(?:\s*\(see\s+below\s+for\s+additional\s+conditions\))?:\s*(.+)$`)
	// anyLevelCourseRe matches "any 400-level STA course" among courses.
	anyLevelCourseRe = regexp.MustCompile(`(?:^|[,;]\s*)any\s+([1-4])00-level\s+(` + grammar.DesigPat + `)\s+course\s*(?:[,;]|$)`)
	// whicheverRe matches "(whichever one was not taken previously)", which
	// exclusive allocation says.
	whicheverRe = regexp.MustCompile(`(?i)\s*\(whichever\s+one\s+was\s+not\s+taken\s+previously\)`)
	// languageLevelRe matches ASMAJ0552's language rule: "Students must
	// successfully complete (with a minimum grade of 65%) at least the second
	// level of language instruction in Spanish or Portuguese ...".
	languageLevelRe = regexp.MustCompile(`^-?\s*(Students\s+must\s+successfully\s+complete\s+\(with\s+a\s+minimum\s+grade\s+of\s+(\d{2})%\)\s+` + grammar.AtLeastPat + `\s+the\s+second\s+level\s+of\s+language\s+instruction\s+in\s+(\w+)\s+or\s+(\w+)\s+by\s+the\s+end\s+of\s+the\s+\w+\s+year\s+of\s+study)\.\s+This\s+requirement\s+is\s+normally\s+met\s+through\s+` + grammar.AmountPat + `\s+credits?\s+from\s+either\s+the\s+list\s+of\s+\w+\s+courses\s+in\s+(?:Group|List)\s+A\s+or\s+from\s+the\s+list\s+of\s+\w+\s+courses\s+in\s+List\s+A\.\s+Students\s+can\s+also\s+demonstrate\s+equivalent\s+proficiency\s+in\s+the\s+given\s+language\s+through\s+completion\s+of\s+` + grammar.AmountPat + `\s+credits?\s+from\s+the\s+list\s+of\s+([A-Z][\w ]+?)\s+courses\s+in\s+List\s+A\s+at\s+a\s+second-year\s+level\s+of\s+instruction\s+or\s+higher\.?$`)
	// openToLevelRe matches a pool with an approval route by level: "1.0
	// credit from BIO220H1/ ... The department is also open to the use of
	// 300-level lecture-based course(s) in CHM/ CSB/ ..." (ASMAJ1762).
	openToLevelRe = regexp.MustCompile(`(?i)^(?:\d{1,2}\.\s+)?` + grammar.AmountPat + `\s+credits?\s+from\s+(` + grammar.CodePat + `(?:\s*/\s*(?:one\s+of\s+)?` + grammar.CodePat + `(?:\s+or\s+` + grammar.CodePat + `)?|\s+or\s+a\s+[a-z ]+course:\s*` + grammar.CodePat + `)*)\s*(?:\(see\s+note\))?\.\s+(the\s+department\s+is\s+also\s+open\s+to\s+the\s+use\s+of\s+([1-4])00-level\s+lecture-based\s+course\(s\)\s+in\s+(` + grammar.DesigPat + `(?:\s*/\s*` + grammar.DesigPat + `)*)\.\s+students\s+must\s+first\s+seek\s+approval\s+from\s+the\s+department\s+prior\s+to\s+enrolling\s+in\s+any\s+lecture-based\s+` + grammar.LevelPat + `-level\s+course\(s\)\s+not\s+listed)\.?$`)
	// oneOfOrRe matches one choice in a list: "one of BIO260H1 or HMB265H1".
	oneOfOrRe = regexp.MustCompile(`(?i)\bone\s+of\s+(` + grammar.CodePat + `)\s+or\s+(` + grammar.CodePat + `)`)
	// shareOfTotalRe matches a header's share of other designators:
	// "Students complete 4.0 credits, of which up to 1.0 credit may be taken in
	// SPA/LAS courses.".
	shareOfTotalRe = regexp.MustCompile(`(?i)^students\s+complete\s+` + grammar.AmountPat + `\s+credits,\s+of\s+which\s+up\s+to\s+` + grammar.NumPat + `\s+credits?\s+may\s+be\s+taken\s+in\s+(` + grammar.DesigPat + `(?:\s*/\s*` + grammar.DesigPat + `)*)\s+courses\.?$`)
	// additionalScienceRe matches "2. Additional credits from 300/400-level
	// CHM/ MAT/ another science, ... Minimum 3.0 CHM credits at the 400-level to a
	// total of 14.0 credits." (ASSPE1376).
	additionalScienceRe = regexp.MustCompile(`(?i)^(?:\d{1,2}\.\s+)?additional\s+credits\s+from\s+(` + grammar.LevelPat + `(?:/` + grammar.LevelPat + `)*)-level\s+(` + grammar.DesigPat + `(?:\s*/\s*` + grammar.DesigPat + `)*)\s*/\s*another\s+science,\s+including\s+` + grammar.AtLeastPat + `\s+\w+\s+of\s+[^.]+\.\s+minimum\s+` + grammar.AmountPat + `\s+(` + grammar.DesigPat + `)\s+credits\s+at\s+the\s+([1-4])00-level\s+to\s+a\s+total\s+of\s+` + grammar.AmountPat + `\s+credits\.?$`)
	// streamCoursesRe matches a header floor on the stream's courses:
	// "including 2.0 credits from 300+ level Literature and Language courses"
	// (ASMAJ1647A).
	streamCoursesRe = regexp.MustCompile(`(?i)^including\s+` + grammar.AmountPat + `\s+credits?\s+from\s+(` + grammar.LevelPat + `\+)\s+level\s+([A-Z][\w ]+?)\s+courses$`)
	// languagesFloorRe matches "At least 9.0 FAH credits, and 2.0 credits in
	// one or more languages including at least one German, French, or Italian,
	// though ... may be acceptable" (ASSPE0908). Group 3 holds groups 4 and 5.
	languagesFloorRe = regexp.MustCompile(`(?i)^` + grammar.AtLeastPat + `\s+` + grammar.AmountPat + `\s+(` + grammar.DesigPat + `)\s+credits,\s+and\s+(` + grammar.AmountPat + `\s+credits\s+in\s+one\s+or\s+more\s+languages\s+including\s+` + grammar.AtLeastPat + `\s+one\s+((?:[A-Z]\w+,\s+)*[A-Z]\w+,?\s+or\s+[A-Z]\w+)),\s+(though\s+an\s+acceptable\s+alternative\s+modern\s+language\b.*\bmay\s+be\s+acceptable)\.?$`)
	// seriesRe matches a level's courses of a designator, less some:
	// "PSL300-series (excluding PSL300H1, PSL301H1)" (ASMIN0482).
	seriesRe = regexp.MustCompile(`\b([A-Z]{3})([1-4])00-series(?:\s*\(excluding\s+([^()]*)\))?`)
	// codeChoiceRe matches a course or a "/" choice; a code with no campus
	// ("CSB332H") names no catalogue course.
	codeChoiceRe = regexp.MustCompile(`^(?:` + grammar.CodePat + `|[A-Z]{3}\d{3}[HY])(?:\s*/\s*(?:` + grammar.CodePat + `|[A-Z]{3}\d{3}[HY]))*$`)
	// twoSetsRe matches an amount from one set or the other: "2.0 credits
	// from: [( SLA107Y1/ SLA117H1, SLA118H1); SLA214H1; ...]; / [ SLA109Y1, ...]"
	// (ASSPE1205).
	twoSetsRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\s+from:\s*\[([^\[\]]+)\]\s*;?\s*/\s*\[([^\[\]]+)\]$`)
	// groupChoiceRe matches one course or the two: "( SLA107Y1/ SLA117H1,
	// SLA118H1)".
	groupChoiceRe = regexp.MustCompile(`\(\s*(` + grammar.CodePat + `)\s*/\s*(` + grammar.CodePat + `)\s*,\s*(` + grammar.CodePat + `)\s*\)`)
	// requiredIfBeganRe matches "Note that PRT420H1 is required for students
	// who began their Portuguese studies with 200+ level PRT language courses"
	// (ASMAJ0338, ASMIN0338).
	requiredIfBeganRe = regexp.MustCompile(`(?i)^(?:note\s+that\s+)?(` + grammar.CodePat + `)\s+is\s+required\s+for\s+students\s+who\s+began\s+their\s+\w+\s+studies\s+with\s+` + grammar.LevelPat + `\+\s+level\s+` + grammar.DesigPat + `\s+language\s+courses\.?$`)
	// selectRemainingRe matches "Select the remaining credits for a total of
	// 12.0 credits from: BIO251H1, ...; and no more than 1.0 credit from the
	// following ...: ANT336H1, ..." (ASSPE1006).
	selectRemainingRe = regexp.MustCompile(`(?i)^(?:\d{1,2}\.\s+)?select\s+the\s+remaining\s+credits\s+for\s+a\s+total\s+of\s+` + grammar.AmountPat + `\s+credits\s+from:\s*(.+?);\s*and\s+` + grammar.NoMoreThanPat + `\s+` + grammar.AmountPat + `\s+credits?\s+from\s+the\s+following(?:\s*\([^()]*\))?:\s*(.+?)(?:\s*\(note\s+that[^()]*\))?$`)
	// allDesigExceptRe matches "all EEB courses (excluding EEB196H1, ...)".
	allDesigExceptRe = regexp.MustCompile(`(?i)(?:^|;\s*)all\s+(` + grammar.DesigPat + `)\s+courses\s*\(excluding\s+([^()]+)\)\s*(?:;|$)`)
	// relatedSelectedRe matches "2.5 credits in RSM courses at the
	// 300+-level, with at least 1.0 credit at the 400-level, and at least 0.5
	// credit must be related to International Business, ..." (ASSPE2431).
	relatedSelectedRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits\s+in\s+(` + grammar.DesigPat + `)\s+courses\s+at\s+the\s+([1-4])00\+-level,\s+(with\s+` + grammar.AtLeastPat + `\s+` + grammar.AmountPat + `\s+credits?\s+at\s+the\s+([1-4])00-level),\s+and\s+(` + grammar.AtLeastPat + `\s+` + grammar.AmountPat + `\s+credits?\s+must\s+be\s+related\s+to\s+[\w ]+?,\s+selected\s+from\s+((?:` + grammar.CodePat + `,\s+)+)(` + grammar.DesigPat + `\b.+?\bas\s+determined\s+by\s+the\s+program\s+office))$`)
	// fromRowsDesigRe matches "Any GGR course from the list for Core Courses
	// and Fourth Year Courses not already used".
	fromRowsDesigRe = regexp.MustCompile(`(?i)^any\s+(` + grammar.DesigPat + `)\s+course\s+from\s+the\s+list\s+for\s+([\w ]+?)\s+and\s+([\w ]+?)\s+not\s+already\s+used\.?$`)
)

// numberRe matches a number: "2".
var numberRe = regexp.MustCompile(`\d+`)

// neighbourRows reads, once the routes are placed, rows that need the rows
// around them: a language placement route, floors on sections or rows, a
// choice from lines below, and the single wordings above.
func (a *Assembler) neighbourRows(lists []*types.CourseList) []*types.CourseList {
	for i := 0; i+1 < len(a.items); i++ {
		r, next := a.items[i], a.items[i+1]
		n := next.r.Req
		if r.r.Req == nil || r.r.Req.Type != types.NodeCredits || r.r.Req.Courses == nil || next.r.Label != "" || n == nil || len(next.children) > 0 {
			continue
		}
		texts := []*types.ReqNode{n}
		if n.Type == types.NodeOr {
			texts = n.Items
		}
		m := languagePlacementRe.FindStringSubmatch(texts[0].Text)
		if m == nil || slices.ContainsFunc(texts, func(t *types.ReqNode) bool { return t.Type != types.NodeText }) {
			continue
		}
		v, _ := strconv.ParseFloat(m[1], 64)
		alts := []*types.ReqNode{r.r.Req, programs.AndNode(&types.ReqNode{Type: types.NodeCredits, Amount: v}, TextNode(types.TextPlacement, texts[0].Text))}
		for _, t := range texts[1:] {
			alts = append(alts, TextNode(types.TextPermission, t.Text))
		}
		r.r.Req = programs.OrNode(alts...)
		addRaw(r, next.r.Raw)
		a.dropRow(next)
	}
	for _, r := range slices.Clone(a.items) {
		n := r.r.Req
		if a.total == nil || r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || len(r.children) > 0 {
			continue
		}
		m := sectionsFloorRe.FindStringSubmatch(strings.TrimSuffix(Clean(n.Text), "."))
		if m == nil {
			continue
		}
		x, y := a.labelled(m[1]), a.labelled(m[2])
		if x == nil || y == nil || x == r || y == r {
			continue
		}
		v, _ := strconv.ParseFloat(m[3], 64)
		ref := &types.ReqNode{Type: types.NodeCredits, Amount: v, Levels: grammar.ParseLevels(m[4] + "00+-level"), Requirements: []int{}}
		a.reqRefs = append(a.reqRefs, compReqRef{node: ref, rows: []*compRow{x, y}, scope: true})
		a.dropRow(r)
		r.r.Kind, r.r.Req, r.r.Label, r.children = types.RequirementConstraint, ref, "", nil
		a.total.children = append(a.total.children, r)
	}
	for _, r := range slices.Clone(a.items) {
		n := r.r.Req
		if r.r.Kind != types.RequirementConstraint || n == nil || n.Type != types.NodeText || len(r.children) > 0 {
			continue
		}
		m := reqsFloorRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(Clean(n.Text)), "."))
		if m == nil {
			continue
		}
		var rows []*compRow
		for _, l := range numberRe.FindAllString(m[3], -1) {
			x := a.labelled(l)
			if x == nil || x == r || slices.Contains(rows, x) {
				rows = nil
				break
			}
			rows = append(rows, x)
		}
		if rows == nil {
			continue
		}
		v, _ := strconv.ParseFloat(m[1], 64)
		// the parser reads the 400 level but not D-level (UTSC) or graduate
		// courses: those stay text
		ref := &types.ReqNode{Type: types.NodeCredits, Amount: v, Levels: grammar.ParseLevels(m[2] + "00-level"), Requirements: []int{}}
		a.reqRefs = append(a.reqRefs, compReqRef{node: ref, rows: rows, scope: true})
		r.r.Req = programs.OrNode(ref, TextNode(types.TextOther, n.Text))
	}
	walkRows(a.items, func(r *compRow) {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || r.r.Section == "" || len(r.children) > 0 {
			return
		}
		m := linesBelowRe.FindStringSubmatch(strings.TrimSpace(Clean(n.Text)))
		if m == nil {
			return
		}
		v, _ := strconv.ParseFloat(m[1], 64)
		extra, _ := strconv.ParseFloat(m[4], 64)
		x, y := a.labelled(r.r.Section+" "+m[5]), a.labelled(r.r.Section+" "+m[6])
		if x == nil || y == nil || x == r || y == r || grammar.CourseWeight(m[2]) != v || grammar.CourseWeight(m[3])+extra != v {
			return
		}
		ref := &types.ReqNode{Type: types.NodeCredits, Amount: extra, Requirements: []int{}}
		a.reqRefs = append(a.reqRefs, compReqRef{node: ref, rows: []*compRow{x, y}})
		r.r.Req = programs.OrNode(&types.ReqNode{Type: types.NodeCourse, Code: m[2]}, programs.AndNode(&types.ReqNode{Type: types.NodeCourse, Code: m[3]}, ref))
	})
	walkRows(a.items, func(r *compRow) {
		if l := a.listAnyLevel(r, lists); l != nil {
			lists = append(lists, l)
		}
	})
	walkRows(a.items, func(r *compRow) {
		if l := a.openToLevel(r, lists); l != nil {
			lists = append(lists, l)
		}
	})
	walkRows(a.items, a.upToByCourse)
	a.languageLevel(lists)
	walkRows(a.items, a.additionalScience)
	a.streamCourses()
	walkRows(a.items, a.languagesFloor)
	walkRows(a.items, a.twoSets)
	a.dropChoiceText()
	walkRows(a.items, a.selectRemaining)
	walkRows(a.items, a.relatedSelected)
	walkRows(a.items, func(r *compRow) {
		// the course, or the level the student began at, which the transcript
		// doesn't show: a constraint that claims nothing
		n := r.r.Req
		if r.r.Kind == types.RequirementAllocation && n != nil && n.Type == types.NodeText && len(r.children) == 0 {
			if m := requiredIfBeganRe.FindStringSubmatch(strings.TrimSpace(Clean(n.Text))); m != nil {
				r.r.Kind = types.RequirementConstraint
				r.r.Req = programs.OrNode(&types.ReqNode{Type: types.NodeCourse, Code: m[1]}, TextNode(types.TextPlacement, n.Text))
			}
		}
	})
	walkRows(a.items, func(r *compRow) {
		if l := a.seriesList(r, lists); l != nil {
			lists = append(lists, l)
		}
	})
	a.shareOfTotal()
	a.fromRowsDesignator(lists)
	return lists
}
