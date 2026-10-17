package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// The one-off passes: the entry points that run the readings of wordings one or
// two programs use (in completion_oneoffs_*.go), and the regexes they share.

var (
	// complFromLabelLeadRe matches "From Group A: 1.0 credit or DRM220Y1", which
	// becomes the grammar's "1.0 credit from Group A or DRM220Y1".
	complFromLabelLeadRe = regexp.MustCompile(`(?i)^from\s+(group\s+[A-Z]):\s*(` + numPat + `\s+credits?)(\s+or\s+` + codePat + `)?$`)
	// complWhenEligibleRe matches "other DRM (when eligible)": eligibility to
	// enrol, which a completed course has met.
	complWhenEligibleRe = regexp.MustCompile(`(?i)(\bother\s+[A-Z]{3})\s+\(when\s+eligible\)`)
)

// rewriteOneOffListWordings rewrites wordings of items that draw on lists into
// the list grammar's.
func rewriteOneOffListWordings(text string) string {
	text = complFromLabelLeadRe.ReplaceAllString(strings.TrimSpace(text), "$2 from $1$3")
	return complWhenEligibleRe.ReplaceAllString(text, "$1")
}

// oneOffsAfterPools runs the one-off readings that need the routes placed and
// the lists read, after the stream, cognate and filter pools are made. It
// returns the lists with any it adds.
func (a *compAssembler) oneOffsAfterPools(lists []*types.CourseList) []*types.CourseList {
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
func (a *compAssembler) oneOffTextBody(row *compRow, text *types.ReqNode, alts []*types.ReqNode, ls compListRefs) bool {
	s := strings.TrimSuffix(strings.TrimSpace(text.Text), ".")
	set := func(n *types.ReqNode) {
		row.r.Req = orNode(append([]*types.ReqNode{n}, alts...)...)
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
		k := &types.Requirement{Kind: types.RequirementConstraint, Req: orNode(append([]*types.ReqNode{floor}, alts...)...), Raw: row.r.Raw}
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
func (a *compAssembler) oneOffRowPasses(lists []*types.CourseList) {
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
func (a *compAssembler) dropRow(r *compRow) {
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
func (a *compAssembler) oneOffTextRow(row *compRow, text *types.ReqNode, ls compListRefs) (rows []*compRow, ok bool) {
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
func (a *compAssembler) oneOffsBeforePools() {
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
	// complFromOneOfRe matches "credit from one of PHY224H1", which means
	// "credit from PHY224H1".
	complFromOneOfRe = regexp.MustCompile(`(?i)\b(credits?\s+from)\s+one\s+of\s+(` + codePat + `)`)
	// complAllRequiredRe matches a lead over courses all required: "2.0 credits
	// comprised of the following core PCJ courses, all of which are required: ".
	complAllRequiredRe = regexp.MustCompile(`(?i)^` + numPat + `\s+credits?\s+(?:comprised\s+)?of\s+the\s+following\s+[\w\s]*?courses,\s+all\s+of\s+which\s+are\s+required:\s*`)
	// complListFloorRe matches a floor before its list: "At least 3.5 credits
	// from the following list including at least 2.5 with the BMS/JSM/JWB
	// designator: BMS300H1/ ...".
	complListFloorRe = regexp.MustCompile(`(?i)^(?:` + atLeastPat + `\s+)?(` + numPat + `\s+credits?)\s+from\s+the\s+following\s+list\s+including\s+` + atLeastPat + `\s+` + amountPat + `\s+(?:credits?\s+)?with\s+the\s+(` + desigPat + `(?:/` + desigPat + `)*)\s+designators?:\s*(.+)$`)
	// complGroupFloorRe matches a group set out after its floors: "2.0 credits
	// from the Core Group at the 300+ level, at least 0.5 of which must be at the
	// 400-level: CAR315H1, ...". The groups: amount, level, floors, codes.
	complGroupFloorRe = regexp.MustCompile(`(?i)^(` + numPat + `\s+credits?)\s+from\s+the\s+[\w\s]+?\s+group(?:\s+at\s+the\s+([1-4])00\+\s+level)?(?:,\s*|\s*\(\s*)((?:including\s+)?` + atLeastPat + `\s+[^():]+?)\)?:\s*((?:` + codePat + `,?\s*)+)$`)
	// complWhichFloorRe matches "at least 0.5 of which must be at the 400-level".
	complWhichFloorRe = regexp.MustCompile(`(?i)^` + atLeastPat + `\s+` + amountPat + `\s+of\s+which\s+must\s+be\s+(at\s+the\s+` + levelPat + `-level)$`)
	// complDesigButNotRe matches "in courses with a GER designator, but cannot be
	// GER400H1 or GER401H1".
	complDesigButNotRe = regexp.MustCompile(`(?i)\bin\s+courses\s+with\s+an?\s+(` + desigPat + `)\s+designator,\s+but\s+cannot\s+be\s+(` + codePat + `)\s+or\s+(` + codePat + `)`)
	// complFurtherDesigRe matches "in further ITA courses, including language
	// courses": the language courses are ITA courses too.
	complFurtherDesigRe = regexp.MustCompile(`(?i)\bin\s+further\s+(` + desigPat + `)\s+courses(?:,\s+including\s+language\s+courses$)?`)
	// complOnlyOneFromRe matches a cap at the end of a "/" list: "/ (only one
	// from ESS261H1/ GGR203H1)".
	complOnlyOneFromRe = regexp.MustCompile(`(?i)/?\s*\(\s*only\s+one\s+from\s+((?:` + codePat + `\s*/\s*)*` + codePat + `)\s*\)$`)
	// complSelectedThenPairRe matches a pair after a "/" pool, which ends the
	// pool (ASMAJ0525): "credit selected from FRE210H1/ JFG388H1, ( FSL320H1,
	// FSL322H1)/ FSL321Y1". complMeetsAsWellRe matches "The Physics course
	// JPH441H1 meets this requirement as well as any of the following courses:".
	complSelectedThenPairRe = regexp.MustCompile(`(?i)(\bcredits?\s+selected\s+from\s+(?:` + codePat + `\s*/\s*)+` + codePat + `),\s*(\(\s*` + codePat + `\s*,\s*` + codePat + `\s*\)\s*/\s*` + codePat + `\.?)$`)
	complMeetsAsWellRe      = regexp.MustCompile(`(?i)^(?:[\w\s&,]+\s+requirement:\s*)?the\s+\w+\s+course\s+(` + codePat + `)\s+meets\s+this\s+requirement\s+as\s+well\s+as\s+any\s+of\s+the\s+following\s+courses:\s*`)
)

// rewriteOneOffWordings rewrites wordings one or two items use into the
// grammar's.
func rewriteOneOffWordings(text string) string {
	text = complFromOneOfRe.ReplaceAllString(text, "$1 $2")
	text = complSelectedThenPairRe.ReplaceAllString(text, "$1; $2")
	if m := complGroupFloorRe.FindStringSubmatch(text); m != nil {
		floor := strings.TrimSpace(m[3])
		if f := complWhichFloorRe.FindStringSubmatch(floor); f != nil {
			floor = "including at least " + f[1] + " credit " + f[2]
		}
		codes := codeRe.FindAllString(m[4], -1)
		// the group's level is the codes' own, which the grammar's pool says
		low := 0
		if m[2] != "" {
			low = int(m[2][0]-'0') * 100
		}
		if strings.HasPrefix(strings.ToLower(floor), "including") && !slices.ContainsFunc(codes, func(c string) bool { return courseLevel(c) < low }) {
			text = m[1] + " from " + strings.Join(codes, ", ") + ", " + floor
		}
	}
	text = complMeetsAsWellRe.ReplaceAllString(text, "$1/ ")
	text = complDesigButNotRe.ReplaceAllString(text, "in $1 courses, excluding $2, $3")
	text = complFurtherDesigRe.ReplaceAllString(text, "in $1 courses")
	if m := complOnlyOneFromRe.FindStringSubmatchIndex(text); m != nil {
		// one course of the bracket, when each is a half course: at most 0.5 credit
		codes := text[m[2]:m[3]]
		if !slices.ContainsFunc(codeRe.FindAllString(codes, -1), func(c string) bool { return courseWeight(c) != 0.5 }) {
			text = strings.TrimSpace(text[:m[0]]) + "/ " + codes + "; up to 0.5 credit from " + codes
		}
	}
	text = complListFloorRe.ReplaceAllString(text, "$1 from $4, including at least $2 credits from $3 courses")
	return complAllRequiredRe.ReplaceAllString(text, "")
}

var (
	// complBreadthUnlessRe matches the Faculty's breadth rule on a program's
	// students: "... requires Art History Majors who do not complete FAH275H1, ...
	// to complete at least one 0.5 credit ... in Breadth Category 5".
	complBreadthUnlessRe = regexp.MustCompile(`(?i)^in\s+addition,\s+the\s+faculty\s+of\s+arts\s+and\s+science\s+requires\s+[\w\s]+?\s+who\s+do\s+not\s+complete\s+((?:` + codePat + `)(?:,?\s*(?:or\s+)?(?:` + codePat + `))*)\s+to\s+complete\s+` + atLeastPat + `\s+(?:one\s+)?` + amountPat + `\s+credits?\s+in\s+arts\s+and\s+science\s+courses\s+in\s+breadth\s+category\s+([1-5])\b`)
	// complBreadthOrGroupsRe matches Cinema Studies' three-part item: "In
	// addition, CIN230H1 (a Breadth Category 3 course) or 0.5 credit from Breadth
	// Category 5; and at least 3.0 credits from Groups B through G, ...".
	complBreadthOrGroupsRe = regexp.MustCompile(`(?i)^in\s+addition,\s+(` + codePat + `)\s+\(a\s+breadth\s+category\s+[1-5]\s+course\)\s+or\s+(?:a\s+)?` + amountPat + `\s+credits?\s+from\s+breadth\s+category\s+([1-5]);\s+and\s+(.+)$`)
	// complGroupsThroughRe matches "at least 3.0 credits from Groups B through
	// G, of which 2.0 credits must be at the 300/400-level, and only a maximum of
	// 1.5 credits can be from Group G".
	complGroupsThroughRe = regexp.MustCompile(`(?i)^(?:` + atLeastPat + `\s+)?` + amountPat + `\s+credits\s+from\s+groups\s+((?-i:[A-Z]))\s+through\s+((?-i:[A-Z])),\s+of\s+which\s+` + amountPat + `\s+credits\s+must\s+be\s+at\s+the\s+(\d)00/(\d)00-level,\s+and\s+(?:only\s+)?a\s+maximum\s+of\s+` + amountPat + `\s+credits\s+can\s+be\s+from\s+group\s+((?-i:[A-Z]))\b[^,;]*$`)
	// complNamedAndBelowRe matches "2.0 credits drawn from the foundational
	// courses list and from the elective courses listed below".
	complNamedAndBelowRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+drawn\s+from\s+the\s+(\w+)\s+courses\s+list\s+and\s+from\s+the\s+elective\s+courses\s+listed\s+below$`)
	// complRemainingBelowRe matches "Remaining courses to be selected from the
	// list of eligible electives below", whose amount a note gives.
	complRemainingBelowRe = regexp.MustCompile(`(?i)^remaining\s+courses\s+to\s+be\s+selected\s+from\s+the\s+list\s+of\s+eligible\s+electives\s+below$`)
	// complRemainingAmountRe matches that note: "The remaining 1.5 credits in
	// elective courses chosen from a list of eligible electives".
	complRemainingAmountRe = regexp.MustCompile(`(?i)^the\s+remaining\s+` + amountPat + `\s+credits\s+in\s+elective\s+courses\s+chosen\s+from\s+a\s+list\s+of\s+eligible\s+electives\b`)
	// complOneLanguageRe matches "1.0 credit from one of the following
	// languages", with an optional non-language alternative after it. "one the"
	// is ASMAJ1626's typo.
	complOneLanguageRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+from\s+one\s+(?:of\s+)?the\s+following\s+languages(?:,\s+or\s+(` + numPat + `\s+credits?\s+in\s+non-language\s+courses\s+related\s+to\s+your\s+chosen\s+language\s+of\s+study,\s+selected\s+in\s+consultation\s+with\s+the\s+program\s+coordinator))?:?$`)
	// complLanguageLineRe matches a language and its courses: "- German:
	// GER200Y1/ GER300Y1/ ( GER370H1 and GER372H1)".
	complLanguageLineRe = regexp.MustCompile(`^-\s*([A-Z][A-Za-z ,]+?):\s*(.+?)\s*$`)
	// complLanguageCodesRe matches a language's courses joined by "/" and "and",
	// in brackets or not: "GER200Y1/ ( GER370H1 and GER372H1)".
	complLanguageCodesRe = regexp.MustCompile(`^(?:\(?\s*` + codePat + `(?:\s+and\s+` + codePat + `)*\s*\)?)(?:\s*/\s*\(?\s*` + codePat + `(?:\s+and\s+` + codePat + `)*\s*\)?)*$`)
	// complCoreLeadRe matches a lead whose lines are its parts: "3.5 credits of
	// ESS core courses:".
	complCoreLeadRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits\s+of\s+` + desigPat + `\s+core\s+courses:$`)
	// complCorePartRe matches a part of courses: "0.5 credit of field courses:
	// ESS410H1 (...)/ ...".
	complCorePartRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+(?:chosen\s+from\s+the\s+following\s+courses|of\s+\w+\s+courses):\s*(.+)$`)
	// complCodeTitledRe matches a course and its title in brackets: "ESS224H1
	// (Intro to Mineralogy and Petrology)".
	complCodeTitledRe = regexp.MustCompile(`(` + codePat + `)\s*\([^()]*\)`)
	// complCategoriesPartRe matches a part drawn from categories under the lead:
	// "1.5 credit at the 200 and 300-level (0.5 credits must be taken from each
	// category):".
	complCategoriesPartRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+at\s+the\s+\d00\s+and\s+\d00-level\s+\(` + amountPat + `\s+credits\s+must\s+be\s+taken\s+from\s+each\s+category\):$`)
	// complCategoryLineRe matches a category and its courses: "Biology
	// processes: ESS261H1/ ESS363H1".
	complCategoryLineRe = regexp.MustCompile(`^[A-Z][a-z]+\s+processes:\s*((?:` + codePat + `)(?:\s*/\s*` + codePat + `)*)$`)
	// complElectivesLeadRe matches "2.5 credits of elective courses:".
	complElectivesLeadRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits\s+of\s+elective\s+courses:$`)
	// complCategoriesNoteRe matches its note: "This requirement can be satisfied
	// by any of the Earth Sciences Courses listed in categories A to F below".
	complCategoriesNoteRe = regexp.MustCompile(`(?i)^this\s+requirement\s+can\s+be\s+satisfied\s+by\s+any\s+of\s+the\s+[\w\s]+?\s+courses\s+listed\s+in\s+categories\s+((?-i:[A-Z]))\s+to\s+((?-i:[A-Z]))\s+below$`)
	// complNotLimitRe matches a note that may widen the categories: "neither
	// mutually exclusive nor meant to limit choice".
	complNotLimitRe = regexp.MustCompile(`(?i)\bneither\s+mutually\s+exclusive\s+nor\s+meant\s+to\s+limit\s+choice\b`)
	// complCategoryCodesRe matches a category kept as a note: "Paleoclimate:
	// ESS245H1/ ESS345H1, ESS261H1".
	complCategoryCodesRe = regexp.MustCompile(`^[A-Z][A-Za-z ]+:\s*((?:` + codePat + `)(?:\s*[/,]\s*` + codePat + `)*)$`)
	// complAmountFromRe matches a lead whose pool its route gives: "0.5 credit
	// from:".
	complAmountFromRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+from:$`)
	// complAboveNotCountedRe matches that route: "any of the above from Year 3
	// Req. 2-4 not previously counted or with permission of the Director:
	// ECO351H1/ TRN307H1".
	complAboveNotCountedRe = regexp.MustCompile(`(?i)^any\s+of\s+the\s+above\s+from\s+(year\s+\d)\s+req\.\s*(\d)\s*-\s*(\d)\s+not\s+previously\s+counted\s+or\s+(with\s+permission\s+of\s+the\s+\w+:\s*((?:` + codePat + `)(?:\s*/\s*` + codePat + `)*))$`)
	// complInLanguagesRe matches credits in languages the text names: "3.0
	// credits in Hebrew or Yiddish". Groups 2 and 3 are the two forms of the list.
	complInLanguagesRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits\s+in\s+(?:([A-Z][a-z]+(?:\s+or\s+[A-Z][a-z]+)+)|a\s+language\s+relevant\s+to\s+[\w\s]+?\s+[–-]\s+([A-Z][a-z]+(?:,\s+[A-Z][a-z]+)+)(?:\s+\([^()]*\))?)$`)
	// complResearchCourseRe matches "1.0 credit in a Research Course (a 299Y1,
	// 399Y1, on a Renaissance Studies topic, or REN392H1/ REN392Y1)".
	complResearchCourseRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+in\s+a\s+research\s+course\s+\((a\s+299Y1,\s+399Y1,\s+on\s+a\s+[\w\s]+?\s+topic),\s+or\s+((?:` + codePat + `)(?:\s*[/,]\s*` + codePat + `)*)\)$`)
	// complCodeRunRe matches codes joined by "/" and ",". complLanguageApprovedRe
	// matches an approved language credit: "SLA337H1, and additional 0.5 credit
	// can be fulfilled through ... approved in joint consultation with ...".
	complCodeRunRe          = regexp.MustCompile(`^` + codePat + `(?:\s*[/,]\s*` + codePat + `)*$`)
	complLanguageApprovedRe = regexp.MustCompile(`(?i)^(?:(` + codePat + `),\s+and\s+)?((?:additional\s+)?` + numPat + `\s+credits?\s+can\s+be\s+fulfilled\s+through\s+[^.]+?\bapproved\s+in\s+joint\s+consultation\s+with\s+[^.]+)\.`)
)

// singleWordingRow reads a row of text by the single-wording rules for rows. ok
// reports whether it read the row; rows is what stands for it.
func (a *compAssembler) singleWordingRow(row *compRow, text *types.ReqNode, ls compListRefs) (rows []*compRow, ok bool) {
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
func (a *compAssembler) singleWordingAltRow(row *compRow, text *types.ReqNode, ls compListRefs) bool {
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
	// complLanguagePlacementRe matches a language requirement's placement route:
	// "Students entering the program with proof of requisite linguistic
	// proficiency in Aramaic, ... will choose any 3.0 credits".
	complLanguagePlacementRe = regexp.MustCompile(`(?i)^students\s+entering\s+the\s+program\s+with\s+proof\s+of\s+requisite\s+linguistic\s+proficiency\b.*?\bwill\s+choose\s+any\s+` + amountPat + `\s+credits\b`)
	// complSectionsFloorRe matches "5. Note: From section 3 and 4, at least 4.0
	// credits at the 300+ level.".
	complSectionsFloorRe = regexp.MustCompile(`(?i)^(?:note:\s*)?from\s+sections?\s+(\d{1,2})\s+and\s+(\d{1,2}),\s+` + atLeastPat + `\s+` + amountPat + `\s+credits?\s+at\s+the\s+([1-4])00\+\s+level$`)
	// complReqsFloorRe matches "At least 2.0 credits from 400-level/ D-level
	// courses & Graduate courses in Req5 or Req6 or Req7".
	complReqsFloorRe = regexp.MustCompile(`(?i)^` + atLeastPat + `\s+` + amountPat + `\s+credits?\s+from\s+([1-4])00-level\s*/\s*D-level\s+courses\s*&\s*graduate\s+courses\s+in\s+(req\s*\d{1,2}(?:\s+or\s+req\s*\d{1,2})+)$`)
	// complLinesBelowRe matches "1.0 credit from AST425Y1/ ( AST424H1, and 0.5
	// credit from lines 2 or 3 below; see notes);" (ASSPE0271). The groups are the
	// amount, the course, the other course, its extra amount, the two lines.
	complLinesBelowRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+from\s+(` + codePat + `)\s*/\s*\(\s*(` + codePat + `),?\s+and\s+` + amountPat + `\s+credits?\s+from\s+lines\s+(\d)\s+or\s+(\d)\s+below(?:;\s*see\s+notes)?\s*\);?$`)
	// complListAnyLevelRe matches "2.0 credits from the following list, including
	// at least 1.0 credit at the 400-level (see below for additional conditions):
	// ..." (ASSPE1687).
	complListAnyLevelRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits\s+from\s+the\s+following\s+list,\s+including\s+` + atLeastPat + `\s+` + amountPat + `\s+credits?\s+at\s+the\s+([1-4])00-level(?:\s*\(see\s+below\s+for\s+additional\s+conditions\))?:\s*(.+)$`)
	// complAnyLevelCourseRe matches "any 400-level STA course" among courses.
	complAnyLevelCourseRe = regexp.MustCompile(`(?:^|[,;]\s*)any\s+([1-4])00-level\s+(` + desigPat + `)\s+course\s*(?:[,;]|$)`)
	// complWhicheverRe matches "(whichever one was not taken previously)", which
	// exclusive allocation says.
	complWhicheverRe = regexp.MustCompile(`(?i)\s*\(whichever\s+one\s+was\s+not\s+taken\s+previously\)`)
	// complLanguageLevelRe matches ASMAJ0552's language rule: "Students must
	// successfully complete (with a minimum grade of 65%) at least the second
	// level of language instruction in Spanish or Portuguese ...".
	complLanguageLevelRe = regexp.MustCompile(`^-?\s*(Students\s+must\s+successfully\s+complete\s+\(with\s+a\s+minimum\s+grade\s+of\s+(\d{2})%\)\s+` + atLeastPat + `\s+the\s+second\s+level\s+of\s+language\s+instruction\s+in\s+(\w+)\s+or\s+(\w+)\s+by\s+the\s+end\s+of\s+the\s+\w+\s+year\s+of\s+study)\.\s+This\s+requirement\s+is\s+normally\s+met\s+through\s+` + amountPat + `\s+credits?\s+from\s+either\s+the\s+list\s+of\s+\w+\s+courses\s+in\s+(?:Group|List)\s+A\s+or\s+from\s+the\s+list\s+of\s+\w+\s+courses\s+in\s+List\s+A\.\s+Students\s+can\s+also\s+demonstrate\s+equivalent\s+proficiency\s+in\s+the\s+given\s+language\s+through\s+completion\s+of\s+` + amountPat + `\s+credits?\s+from\s+the\s+list\s+of\s+([A-Z][\w ]+?)\s+courses\s+in\s+List\s+A\s+at\s+a\s+second-year\s+level\s+of\s+instruction\s+or\s+higher\.?$`)
	// complOpenToLevelRe matches a pool with an approval route by level: "1.0
	// credit from BIO220H1/ ... The department is also open to the use of
	// 300-level lecture-based course(s) in CHM/ CSB/ ..." (ASMAJ1762).
	complOpenToLevelRe = regexp.MustCompile(`(?i)^(?:\d{1,2}\.\s+)?` + amountPat + `\s+credits?\s+from\s+(` + codePat + `(?:\s*/\s*(?:one\s+of\s+)?` + codePat + `(?:\s+or\s+` + codePat + `)?|\s+or\s+a\s+[a-z ]+course:\s*` + codePat + `)*)\s*(?:\(see\s+note\))?\.\s+(the\s+department\s+is\s+also\s+open\s+to\s+the\s+use\s+of\s+([1-4])00-level\s+lecture-based\s+course\(s\)\s+in\s+(` + desigPat + `(?:\s*/\s*` + desigPat + `)*)\.\s+students\s+must\s+first\s+seek\s+approval\s+from\s+the\s+department\s+prior\s+to\s+enrolling\s+in\s+any\s+lecture-based\s+` + levelPat + `-level\s+course\(s\)\s+not\s+listed)\.?$`)
	// complOneOfOrRe matches one choice in a list: "one of BIO260H1 or HMB265H1".
	complOneOfOrRe = regexp.MustCompile(`(?i)\bone\s+of\s+(` + codePat + `)\s+or\s+(` + codePat + `)`)
	// complShareOfTotalRe matches a header's share of other designators:
	// "Students complete 4.0 credits, of which up to 1.0 credit may be taken in
	// SPA/LAS courses.".
	complShareOfTotalRe = regexp.MustCompile(`(?i)^students\s+complete\s+` + amountPat + `\s+credits,\s+of\s+which\s+up\s+to\s+` + numPat + `\s+credits?\s+may\s+be\s+taken\s+in\s+(` + desigPat + `(?:\s*/\s*` + desigPat + `)*)\s+courses\.?$`)
	// complAdditionalScienceRe matches "2. Additional credits from 300/400-level
	// CHM/ MAT/ another science, ... Minimum 3.0 CHM credits at the 400-level to a
	// total of 14.0 credits." (ASSPE1376).
	complAdditionalScienceRe = regexp.MustCompile(`(?i)^(?:\d{1,2}\.\s+)?additional\s+credits\s+from\s+(` + levelPat + `(?:/` + levelPat + `)*)-level\s+(` + desigPat + `(?:\s*/\s*` + desigPat + `)*)\s*/\s*another\s+science,\s+including\s+` + atLeastPat + `\s+\w+\s+of\s+[^.]+\.\s+minimum\s+` + amountPat + `\s+(` + desigPat + `)\s+credits\s+at\s+the\s+([1-4])00-level\s+to\s+a\s+total\s+of\s+` + amountPat + `\s+credits\.?$`)
	// complStreamCoursesRe matches a header floor on the stream's courses:
	// "including 2.0 credits from 300+ level Literature and Language courses"
	// (ASMAJ1647A).
	complStreamCoursesRe = regexp.MustCompile(`(?i)^including\s+` + amountPat + `\s+credits?\s+from\s+(` + levelPat + `\+)\s+level\s+([A-Z][\w ]+?)\s+courses$`)
	// complLanguagesFloorRe matches "At least 9.0 FAH credits, and 2.0 credits in
	// one or more languages including at least one German, French, or Italian,
	// though ... may be acceptable" (ASSPE0908). Group 3 holds groups 4 and 5.
	complLanguagesFloorRe = regexp.MustCompile(`(?i)^` + atLeastPat + `\s+` + amountPat + `\s+(` + desigPat + `)\s+credits,\s+and\s+(` + amountPat + `\s+credits\s+in\s+one\s+or\s+more\s+languages\s+including\s+` + atLeastPat + `\s+one\s+((?:[A-Z]\w+,\s+)*[A-Z]\w+,?\s+or\s+[A-Z]\w+)),\s+(though\s+an\s+acceptable\s+alternative\s+modern\s+language\b.*\bmay\s+be\s+acceptable)\.?$`)
	// complSeriesRe matches a level's courses of a designator, less some:
	// "PSL300-series (excluding PSL300H1, PSL301H1)" (ASMIN0482).
	complSeriesRe = regexp.MustCompile(`\b([A-Z]{3})([1-4])00-series(?:\s*\(excluding\s+([^()]*)\))?`)
	// complCodeChoiceRe matches a course or a "/" choice; a code with no campus
	// ("CSB332H") names no catalogue course.
	complCodeChoiceRe = regexp.MustCompile(`^(?:` + codePat + `|[A-Z]{3}\d{3}[HY])(?:\s*/\s*(?:` + codePat + `|[A-Z]{3}\d{3}[HY]))*$`)
	// complTwoSetsRe matches an amount from one set or the other: "2.0 credits
	// from: [( SLA107Y1/ SLA117H1, SLA118H1); SLA214H1; ...]; / [ SLA109Y1, ...]"
	// (ASSPE1205).
	complTwoSetsRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+from:\s*\[([^\[\]]+)\]\s*;?\s*/\s*\[([^\[\]]+)\]$`)
	// complGroupChoiceRe matches one course or the two: "( SLA107Y1/ SLA117H1,
	// SLA118H1)".
	complGroupChoiceRe = regexp.MustCompile(`\(\s*(` + codePat + `)\s*/\s*(` + codePat + `)\s*,\s*(` + codePat + `)\s*\)`)
	// complRequiredIfBeganRe matches "Note that PRT420H1 is required for students
	// who began their Portuguese studies with 200+ level PRT language courses"
	// (ASMAJ0338, ASMIN0338).
	complRequiredIfBeganRe = regexp.MustCompile(`(?i)^(?:note\s+that\s+)?(` + codePat + `)\s+is\s+required\s+for\s+students\s+who\s+began\s+their\s+\w+\s+studies\s+with\s+` + levelPat + `\+\s+level\s+` + desigPat + `\s+language\s+courses\.?$`)
	// complSelectRemainingRe matches "Select the remaining credits for a total of
	// 12.0 credits from: BIO251H1, ...; and no more than 1.0 credit from the
	// following ...: ANT336H1, ..." (ASSPE1006).
	complSelectRemainingRe = regexp.MustCompile(`(?i)^(?:\d{1,2}\.\s+)?select\s+the\s+remaining\s+credits\s+for\s+a\s+total\s+of\s+` + amountPat + `\s+credits\s+from:\s*(.+?);\s*and\s+` + noMoreThanPat + `\s+` + amountPat + `\s+credits?\s+from\s+the\s+following(?:\s*\([^()]*\))?:\s*(.+?)(?:\s*\(note\s+that[^()]*\))?$`)
	// complAllDesigExceptRe matches "all EEB courses (excluding EEB196H1, ...)".
	complAllDesigExceptRe = regexp.MustCompile(`(?i)(?:^|;\s*)all\s+(` + desigPat + `)\s+courses\s*\(excluding\s+([^()]+)\)\s*(?:;|$)`)
	// complRelatedSelectedRe matches "2.5 credits in RSM courses at the
	// 300+-level, with at least 1.0 credit at the 400-level, and at least 0.5
	// credit must be related to International Business, ..." (ASSPE2431).
	complRelatedSelectedRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits\s+in\s+(` + desigPat + `)\s+courses\s+at\s+the\s+([1-4])00\+-level,\s+(with\s+` + atLeastPat + `\s+` + amountPat + `\s+credits?\s+at\s+the\s+([1-4])00-level),\s+and\s+(` + atLeastPat + `\s+` + amountPat + `\s+credits?\s+must\s+be\s+related\s+to\s+[\w ]+?,\s+selected\s+from\s+((?:` + codePat + `,\s+)+)(` + desigPat + `\b.+?\bas\s+determined\s+by\s+the\s+program\s+office))$`)
	// complFromRowsDesigRe matches "Any GGR course from the list for Core Courses
	// and Fourth Year Courses not already used".
	complFromRowsDesigRe = regexp.MustCompile(`(?i)^any\s+(` + desigPat + `)\s+course\s+from\s+the\s+list\s+for\s+([\w ]+?)\s+and\s+([\w ]+?)\s+not\s+already\s+used\.?$`)
)

// complNumberRe matches a number: "2".
var complNumberRe = regexp.MustCompile(`\d+`)

// neighbourRows reads, once the routes are placed, rows that need the rows
// around them: a language placement route, floors on sections or rows, a
// choice from lines below, and the single wordings above.
func (a *compAssembler) neighbourRows(lists []*types.CourseList) []*types.CourseList {
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
		m := complLanguagePlacementRe.FindStringSubmatch(texts[0].Text)
		if m == nil || slices.ContainsFunc(texts, func(t *types.ReqNode) bool { return t.Type != types.NodeText }) {
			continue
		}
		v, _ := strconv.ParseFloat(m[1], 64)
		alts := []*types.ReqNode{r.r.Req, andNode(&types.ReqNode{Type: types.NodeCredits, Amount: v}, textNode(types.TextPlacement, texts[0].Text))}
		for _, t := range texts[1:] {
			alts = append(alts, textNode(types.TextPermission, t.Text))
		}
		r.r.Req = orNode(alts...)
		addRaw(r, next.r.Raw)
		a.dropRow(next)
	}
	for _, r := range slices.Clone(a.items) {
		n := r.r.Req
		if a.total == nil || r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || len(r.children) > 0 {
			continue
		}
		m := complSectionsFloorRe.FindStringSubmatch(strings.TrimSuffix(complClean(n.Text), "."))
		if m == nil {
			continue
		}
		x, y := a.labelled(m[1]), a.labelled(m[2])
		if x == nil || y == nil || x == r || y == r {
			continue
		}
		v, _ := strconv.ParseFloat(m[3], 64)
		ref := &types.ReqNode{Type: types.NodeCredits, Amount: v, Levels: parseLevels(m[4] + "00+-level"), Requirements: []int{}}
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
		m := complReqsFloorRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(complClean(n.Text)), "."))
		if m == nil {
			continue
		}
		var rows []*compRow
		for _, l := range complNumberRe.FindAllString(m[3], -1) {
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
		ref := &types.ReqNode{Type: types.NodeCredits, Amount: v, Levels: parseLevels(m[2] + "00-level"), Requirements: []int{}}
		a.reqRefs = append(a.reqRefs, compReqRef{node: ref, rows: rows, scope: true})
		r.r.Req = orNode(ref, textNode(types.TextOther, n.Text))
	}
	walkRows(a.items, func(r *compRow) {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || r.r.Section == "" || len(r.children) > 0 {
			return
		}
		m := complLinesBelowRe.FindStringSubmatch(strings.TrimSpace(complClean(n.Text)))
		if m == nil {
			return
		}
		v, _ := strconv.ParseFloat(m[1], 64)
		extra, _ := strconv.ParseFloat(m[4], 64)
		x, y := a.labelled(r.r.Section+" "+m[5]), a.labelled(r.r.Section+" "+m[6])
		if x == nil || y == nil || x == r || y == r || courseWeight(m[2]) != v || courseWeight(m[3])+extra != v {
			return
		}
		ref := &types.ReqNode{Type: types.NodeCredits, Amount: extra, Requirements: []int{}}
		a.reqRefs = append(a.reqRefs, compReqRef{node: ref, rows: []*compRow{x, y}})
		r.r.Req = orNode(&types.ReqNode{Type: types.NodeCourse, Code: m[2]}, andNode(&types.ReqNode{Type: types.NodeCourse, Code: m[3]}, ref))
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
			if m := complRequiredIfBeganRe.FindStringSubmatch(strings.TrimSpace(complClean(n.Text))); m != nil {
				r.r.Kind = types.RequirementConstraint
				r.r.Req = orNode(&types.ReqNode{Type: types.NodeCourse, Code: m[1]}, textNode(types.TextPlacement, n.Text))
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
