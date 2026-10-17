package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Item bodies whose pool the grammar can't read for a remark in brackets
// or a clause on courses already used (audit R4, R5, R14). remarkItem takes
// the remark out, reads the rest as an item body (itemBody) and gives the
// remark its own place: a note, a limit, a bounded route, an equivalent
// course. It reads nothing unless the rest reads with no text.

var (
	// complCodeSeeNoteRe: "(see NOTE 3)" after a course of a list:
	// "JPM400Y1 (see NOTE 3)/ PCL298H1". The groups: the course, the note.
	complCodeSeeNoteRe = regexp.MustCompile(`(?i)(` + codePat + `)\s*\((see\s+note\s+\d+)\)`)
	// complCodeFormerlyRe: "SMC366H1 (_formerly SMC218Y1_)": the old code is
	// the same course, so it counts too. The groups: the course, the old one.
	complCodeFormerlyRe = regexp.MustCompile(`(` + codePat + `)\s*\(\s*_?formerly\s+(` + codePat + `)_?\s*\)`)
	// complRemarkAdviceRe: a trailing remark that is advice: "(note that
	// both require BIO130H1 and ...)", "(The courses ... are recommended.)".
	complRemarkAdviceRe = regexp.MustCompile(`(?i)^note\s+that\b|\b(?:is|are)\s+recommended\.?$`)
	// complRemarkCanBeUsedRe: "GGR493Y1 (0.5 credit can be used based on
	// internship)": the course before it counts for that much at most. The
	// group: the amount.
	complRemarkCanBeUsedRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+can\s+be\s+used\b[^()]*$`)
	// complRemarkConsideredRe: "( STA399H1 and STA399Y1 may be considered on
	// a case by case basis with the approval of the Associate Chair, ...)":
	// the courses join the pool with approval. The group: the courses.
	complRemarkConsideredRe = regexp.MustCompile(`(?i)^((?:` + codePat + `)(?:\s*(?:,|and|or)\s*` + codePat + `)*)\s+may\s+be\s+considered\b.*\bapproval\b`)
	// complRemarkOnlyIfRe: "( PHL245H1 may only be counted here if ( MAT158H1,
	// MAT159H1/ MAT157Y1) is not taken)". The groups: the course, the courses
	// whose completion stops it.
	complRemarkOnlyIfRe = regexp.MustCompile(`(?i)^(` + codePat + `)\s+may\s+only\s+be\s+counted\s+here\s+if\s+(.+?)\s+is\s+not\s+taken$`)
	// complAmountFromCodesRe: "0.5 credit in biological diversity and
	// function from: BIO251H1, BIO270H1; EEB266H1, ...", "0.5 credit from
	// the following (internship options): ...", "1.0 credit from remaining
	// STA300+ level courses not used toward other program requirements in
	// the following list: ..." (exclusive allocation). The groups: the
	// amount, the courses.
	complAmountFromCodesRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+(?:(?:in\s+[a-z][a-z\s]+?\s+)?(?:from|of)(?:\s+the\s+following(?:\s+\([^()]*\))?)?|from\s+remaining\s+[^:]*?\bnot\s+used\s+toward\s+other\s+program\s+requirements\s+in\s+the\s+following\s+list):?\s*(` + codePat + `(?:\s*(?:,|;|/|or)\s*` + codePat + `)*)$`)
	// complFloorFromDesignatorRe: "2.5 credits at least 1.5 credits from PCL
	// courses: JPM300H1/ ...", "PCL469H1 and 3.0 credits with at least 1.5
	// credits from PCL courses: ..." (R14). The groups: a course before it,
	// the amount, the floor, the designator, the courses.
	complFloorFromDesignatorRe = regexp.MustCompile(`^(?:[A-Za-z][A-Za-z ]*:\s*)?(?:(` + codePat + `)\s*,?\s+and\s+)?(\d+(?:\.\d+)?)\s+credits,?\s+(?:with\s+)?at\s+least\s+(\d+(?:\.\d+)?)\s+credits?\s+from\s+([A-Z]{3})\s+courses:\s*(` + codePat + `(?:\s*(?:/|or)\s*` + codePat + `)*)$`)
	// complNotUsedFromRe: "Any course (0.5 credit) not used to satisfy first
	// year course requirement from JEG100H1, ..." (R5): exclusive
	// allocation already keeps a course from counting twice. The groups:
	// the amount, the courses.
	complNotUsedFromRe = regexp.MustCompile(`(?i)^any\s+course\s+\((\d+(?:\.\d+)?)\s+credits?\)\s+not\s+used\s+to\s+satisfy\s+[\w\s]+?\s+requirements?\s+from\s+(.+)$`)
	// complRemainingAnyRe: "The remaining 1.5 - 2.0 credits can include any
	// courses not previously taken to fulfill the preceding requirements, or
	// any of the following: ..." (R5): any course, the lower amount. The
	// groups: the amount, the courses.
	// complOtherNotTakenRe: "The other of CHM210H1 or ESS262H1/ JEG100H1 not
	// taken in second year": the choice again; exclusive allocation keeps
	// the course the earlier row used from counting (audit FIX-ONE
	// ASMAJ1076). The group: the choice.
	complOtherNotTakenRe = regexp.MustCompile(`(?i)^the\s+other\s+of\s+(.+?)\s+not\s+taken\s+in\s+(?:first|second|third|fourth)\s+year$`)
	// complUpToByCourseRe: "Up to 1.5 credits from the following courses, up
	// to a total of 4.0 credits to complete the minor program. Students who
	// complete ANT203Y1 need to take 1.0 credit, and students who take
	// ANT204H1 or ANT207H1 need to take 1.5 credits, from the following: ..."
	// (audit FIX-ONE ASMIN1778). The groups: the amount, the course, its
	// amount, the other courses, their amount, the courses.
	complUpToByCourseRe = regexp.MustCompile(`(?i)^(?:\d{1,2}\.\s+)?up\s+to\s+(\d+(?:\.\d+)?)\s+credits\s+from\s+the\s+following\s+courses,\s+up\s+to\s+a\s+total\s+of\s+\d+(?:\.\d+)?\s+credits\s+to\s+complete\s+the\s+\w+\s+program\.\s+students\s+who\s+complete\s+(` + codePat + `)\s+need\s+to\s+take\s+(\d+(?:\.\d+)?)\s+credits?,\s+and\s+students\s+who\s+take\s+(` + codePat + `(?:\s+or\s+` + codePat + `)*)\s+need\s+to\s+take\s+(\d+(?:\.\d+)?)\s+credits,\s+from\s+the\s+following:\s*(.+)$`)
	// complWillCountRe: "Health-related Anthropology Special Topics courses
	// at the 300 or 400 level, ... will count towards program requirements".
	complWillCountRe    = regexp.MustCompile(`(?i)^[A-Z][\w -]*\bcourses\b.*\bwill\s+count\s+towards?\s+program\s+requirements$`)
	complRemainingAnyRe = regexp.MustCompile(`(?i)^(?:\d{1,2}\.\s+)?the\s+remaining\s+(\d+(?:\.\d+)?)\s*[-–]\s*\d+(?:\.\d+)?\s+credits\s+can\s+include\s+any\s+courses\s+not\s+previously\s+taken\b[^,:]*,\s*or\s+any\s+of\s+the\s+following:\s*(.+)$`)
)

// trailingParen splits s at its last bracket group when s ends with it:
// the text before and the group's inside.
func trailingParen(s string) (head, inner string, ok bool) {
	if !strings.HasSuffix(s, ")") {
		return "", "", false
	}
	depth := 0
	for i := len(s) - 1; i >= 0; i-- {
		switch s[i] {
		case ')':
			depth++
		case '(':
			depth--
			if depth == 0 {
				return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1 : len(s)-1]), true
			}
		}
	}
	return "", "", false
}

// remarkItem reads an item body the grammar leaves as text for a remark
// (itemBody):
//   - "(see NOTE n)" after a course: dropped, the notes are the envelope's;
//   - "(formerly X)": X counts as the course;
//   - a trailing remark of advice: a note;
//   - "(N credit can be used ...)" after a course: a limit of N on it;
//   - "( X may be considered ... with the approval of ...)": the bounded
//     route or(pool, and(credits{pool and X}, text{permission}));
//   - "( X may only be counted here if Y is not taken)": or(credits{pool
//     without X}, and(credits{pool}, not(Y)));
//   - an amount from a list of courses after a description;
//   - complNotUsedFromRe, complRemainingAnyRe (R5), complOtherNotTakenRe.
func (a *compAssembler) remarkItem(s string) (body *types.ReqNode, rows []*types.Requirement, notes []string, ok bool) {
	t := strings.TrimSuffix(strings.TrimSpace(complClean(s)), ".")
	if m := complRemainingAnyRe.FindStringSubmatch(t); m != nil && isCodeList(m[2]) {
		v, _ := strconv.ParseFloat(m[1], 64)
		// The courses listed are among any: a note.
		return &types.ReqNode{Type: types.NodeCredits, Amount: v}, nil, []string{cleanNote("Or any of the following: " + m[2])}, true
	}
	if m := complOtherNotTakenRe.FindStringSubmatch(t); m != nil {
		if n, more, more2 := a.grammarBody(m[1]); n != nil && !hasText(n) && len(more) == 0 && len(more2) == 0 {
			return n, nil, nil, true
		}
	}
	if m := complNotUsedFromRe.FindStringSubmatch(t); m != nil {
		if n, caps := codesCredits(m[1], m[2]); n != nil {
			return n, caps, nil, true
		}
	}
	changed := false
	if complCodeSeeNoteRe.MatchString(t) {
		t, changed = complCodeSeeNoteRe.ReplaceAllString(t, "$1"), true
	}
	if complCodeFormerlyRe.MatchString(t) {
		t, changed = complCodeFormerlyRe.ReplaceAllString(t, "$1/ $2"), true
	}
	head, inner, paren := trailingParen(t)
	var after func(n *types.ReqNode) *types.ReqNode
	switch {
	case !paren:
	case complRemarkAdviceRe.MatchString(inner):
		t, changed = head, true
		notes = append(notes, cleanNote(inner))
	case complRemarkCanBeUsedRe.MatchString(inner) && codeAtEnd(head) != "":
		v, _ := strconv.ParseFloat(complRemarkCanBeUsedRe.FindStringSubmatch(inner)[1], 64)
		t, changed = head, true
		rows = append(rows, &types.Requirement{Kind: types.RequirementLimit, Raw: cleanNote(inner),
			Req: &types.ReqNode{Type: types.NodeCredits, Courses: []string{codeAtEnd(head)}, MaxAmount: maxAmount(v)}})
	case complRemarkConsideredRe.MatchString(inner):
		extra := codeRe.FindAllString(complRemarkConsideredRe.FindStringSubmatch(inner)[1], -1)
		t, changed = head, true
		after = func(n *types.ReqNode) *types.ReqNode {
			if n.Type != types.NodeCredits || n.Courses == nil || n.Amount == 0 {
				return nil
			}
			wide := cloneNode(n)
			wide.Courses = dedupe(append(wide.Courses, extra...))
			return orNode(n, andNode(wide, textNode(types.TextPermission, cleanNote(inner))))
		}
	case complRemarkOnlyIfRe.MatchString(inner):
		m := complRemarkOnlyIfRe.FindStringSubmatch(inner)
		cond, _ := parseCompletionText(m[2])
		if cond == nil || hasText(cond) {
			return nil, nil, nil, false
		}
		t, changed = head, true
		after = func(n *types.ReqNode) *types.ReqNode {
			if n.Type != types.NodeCredits || !slices.Contains(n.Courses, m[1]) || n.Amount == 0 {
				return nil
			}
			without := cloneNode(n)
			without.Courses = slices.DeleteFunc(slices.Clone(n.Courses), func(c string) bool { return c == m[1] })
			return orNode(without, andNode(n, &types.ReqNode{Type: types.NodeNot, Items: []*types.ReqNode{cond}}))
		}
	}
	if !changed {
		return nil, nil, nil, false
	}
	body, more, more2 := a.grammarBody(t)
	if body == nil || hasText(body) {
		body, more, more2 = nil, nil, nil
		if m := complAmountFromCodesRe.FindStringSubmatch(t); m != nil {
			body, more = codesCredits(m[1], m[2])
		}
		if body == nil {
			return nil, nil, nil, false
		}
	}
	for _, r := range more {
		if r.Raw == t {
			r.Raw = strings.TrimSpace(s) // the remark is the row's too
		}
	}
	if after != nil {
		if body = after(body); body == nil {
			return nil, nil, nil, false
		}
	}
	return body, append(rows, more...), append(notes, more2...), true
}

// codesCredits reads "N credit(s) from <courses>" as credits{N, the
// courses}, or nil when list isn't only courses. A "/" between courses of
// a "," or ";" list makes them one choice: a limit of the larger course's
// weight on them, when the choice can matter (caps).
func codesCredits(amount, list string) (n *types.ReqNode, caps []*types.Requirement) {
	list = strings.TrimSuffix(strings.TrimSpace(list), ".")
	if !isCodeList(list) {
		return nil, nil
	}
	v, _ := strconv.ParseFloat(amount, 64)
	if strings.Contains(list, "/") && (strings.Contains(list, ",") || strings.Contains(list, ";")) {
		for _, part := range regexp.MustCompile(`[,;]`).Split(list, -1) {
			codes := codeRe.FindAllString(part, -1)
			if len(codes) < 2 {
				continue
			}
			w := 0.0
			for _, c := range codes {
				w = max(w, courseWeight(c))
			}
			if w < v {
				caps = append(caps, &types.Requirement{Kind: types.RequirementLimit, Raw: strings.TrimSpace(part),
					Req: &types.ReqNode{Type: types.NodeCredits, Courses: codes, MaxAmount: maxAmount(w)}})
			}
		}
	}
	return &types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: dedupe(codeRe.FindAllString(list, -1))}, caps
}

// codeAtEnd returns the course s ends with, or "".
func codeAtEnd(s string) string {
	m := regexp.MustCompile(`(` + codePat + `)$`).FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	return m[1]
}

// floorFromDesignator reads complFloorFromDesignatorRe (R14), before the
// grammar, which reads the floor as a second pool: credits{N, the list}
// and a constraint of the floor on the list's courses of the designator
// and the joint (J) ones. "(see NOTE n)" after a course is dropped.
func floorFromDesignator(s string) (body *types.ReqNode, rows []*types.Requirement, ok bool) {
	t := complCodeSeeNoteRe.ReplaceAllString(strings.TrimSuffix(strings.TrimSpace(complClean(s)), "."), "$1")
	if m := complFloorFromDesignatorRe.FindStringSubmatch(t); m != nil {
		v, _ := strconv.ParseFloat(m[2], 64)
		floor, _ := strconv.ParseFloat(m[3], 64)
		codes := dedupe(codeRe.FindAllString(m[5], -1))
		var own []string
		for _, c := range codes {
			if c[:3] == m[4] || c[0] == 'J' {
				own = append(own, c)
			}
		}
		if floor >= v || weightOf(own) < floor {
			return nil, nil, false
		}
		pool := &types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: codes}
		body = pool
		if m[1] != "" {
			body = andNode(&types.ReqNode{Type: types.NodeCourse, Code: m[1]}, pool)
		}
		rows = []*types.Requirement{{Kind: types.RequirementConstraint, Raw: cleanNote(t),
			Req: &types.ReqNode{Type: types.NodeCredits, Amount: floor, Courses: own}}}
		return body, rows, true
	}
	return nil, nil, false
}

var (
	// complLanguagePlacementRe: the placement route of a language requirement:
	// "Students entering the program with proof of requisite linguistic
	// proficiency in Aramaic, Hebrew, Yiddish will choose any 3.0 credits in
	// an area of study (see above) ...". The group: the amount.
	complLanguagePlacementRe = regexp.MustCompile(`(?i)^students\s+entering\s+the\s+program\s+with\s+proof\s+of\s+requisite\s+linguistic\s+proficiency\b.*?\bwill\s+choose\s+any\s+(\d+(?:\.\d+)?)\s+credits\b`)
	// complSectionsFloorRe: "5. Note: From section 3 and 4, at least 4.0
	// credits at the 300+ level." The groups: the sections, the amount, the
	// level.
	complSectionsFloorRe = regexp.MustCompile(`(?i)^(?:note:\s*)?from\s+sections?\s+(\d{1,2})\s+and\s+(\d{1,2}),\s+at\s+least\s+(\d+(?:\.\d+)?)\s+credits?\s+at\s+the\s+([1-4])00\+\s+level$`)
	// complReqsFloorRe: "At least 2.0 credits from 400-level/ D-level courses
	// & Graduate courses in Req5 or Req6 or Req7 or Req8 or Req9." The groups:
	// the amount, the level, the rows.
	complReqsFloorRe = regexp.MustCompile(`(?i)^at\s+least\s+(\d+(?:\.\d+)?)\s+credits?\s+from\s+([1-4])00-level\s*/\s*D-level\s+courses\s*&\s*graduate\s+courses\s+in\s+(req\s*\d{1,2}(?:\s+or\s+req\s*\d{1,2})+)$`)
	// complLinesBelowRe: "1.0 credit from AST425Y1/ ( AST424H1, and 0.5
	// credit from lines 2 or 3 below; see notes);" (audit FIX-ONE ASSPE0271).
	// The groups: the amount, the course, the other course, its extra
	// amount, the two lines.
	complLinesBelowRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+from\s+(` + codePat + `)\s*/\s*\(\s*(` + codePat + `),?\s+and\s+(\d+(?:\.\d+)?)\s+credits?\s+from\s+lines\s+(\d)\s+or\s+(\d)\s+below(?:;\s*see\s+notes)?\s*\);?$`)
	// complListAnyLevelRe: "2.0 credits from the following list, including
	// at least 1.0 credit at the 400-level (see below for additional
	// conditions): STA303H1/ STA305H1 (whichever one was not taken
	// previously), STA313H1/ CSC316H1, ..., any 400-level STA course;
	// JSC470H1 (Data Science III); ..." (audit FIX-ONE ASSPE1687). The
	// groups: the amount, the floor, its level, the list.
	complListAnyLevelRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits\s+from\s+the\s+following\s+list,\s+including\s+at\s+least\s+(\d+(?:\.\d+)?)\s+credits?\s+at\s+the\s+([1-4])00-level(?:\s*\(see\s+below\s+for\s+additional\s+conditions\))?:\s*(.+)$`)
	// complAnyLevelCourseRe: "any 400-level STA course" among courses. The
	// groups: the level, the designator.
	complAnyLevelCourseRe = regexp.MustCompile(`(?:^|[,;]\s*)any\s+([1-4])00-level\s+((?-i:[A-Z]{3}))\s+course\s*(?:[,;]|$)`)
	// complWhicheverRe: "(whichever one was not taken previously)" after a
	// choice of courses: exclusive allocation says it.
	complWhicheverRe = regexp.MustCompile(`(?i)\s*\(whichever\s+one\s+was\s+not\s+taken\s+previously\)`)
	// complLanguageLevelRe: "Students must successfully complete (with a
	// minimum grade of 65%) at least the second level of language
	// instruction in Spanish or Portuguese by the end of the third year of
	// study. This requirement is normally met through 1.0 credit from either
	// the list of Spanish courses in Group A or from the list of Portuguese
	// courses in List A. Students can also demonstrate equivalent proficiency
	// in the given language through completion of 1.0 credit from the list of
	// Latin American Cultural Expressions courses in List A at a second-year
	// level of instruction or higher." (audit FIX-ONE ASMAJ0552). The groups:
	// the rule, the grade, the languages, the amount, the other amount, the
	// other list.
	complLanguageLevelRe = regexp.MustCompile(`^-?\s*(Students\s+must\s+successfully\s+complete\s+\(with\s+a\s+minimum\s+grade\s+of\s+(\d{2})%\)\s+at\s+least\s+the\s+second\s+level\s+of\s+language\s+instruction\s+in\s+(\w+)\s+or\s+(\w+)\s+by\s+the\s+end\s+of\s+the\s+\w+\s+year\s+of\s+study)\.\s+This\s+requirement\s+is\s+normally\s+met\s+through\s+(\d+(?:\.\d+)?)\s+credits?\s+from\s+either\s+the\s+list\s+of\s+\w+\s+courses\s+in\s+(?:Group|List)\s+A\s+or\s+from\s+the\s+list\s+of\s+\w+\s+courses\s+in\s+List\s+A\.\s+Students\s+can\s+also\s+demonstrate\s+equivalent\s+proficiency\s+in\s+the\s+given\s+language\s+through\s+completion\s+of\s+(\d+(?:\.\d+)?)\s+credits?\s+from\s+the\s+list\s+of\s+([A-Z][\w ]+?)\s+courses\s+in\s+List\s+A\s+at\s+a\s+second-year\s+level\s+of\s+instruction\s+or\s+higher\.?$`)
	// complOpenToLevelRe: "1.0 credit from BIO220H1/ one of BIO260H1 or
	// HMB265H1/ JPM300H1 or a biochemistry research project course: BCH372Y1/
	// ... (See NOTE). The department is also open to the use of 300-level
	// lecture-based course(s) in CHM/ CSB/ ... Students must first seek
	// approval from the department prior to enrolling in any lecture-based
	// 300-level course(s) not listed." (audit FIX-ONE ASMAJ1762). The groups:
	// the amount, the courses, the approval (the level, the designators).
	complOpenToLevelRe = regexp.MustCompile(`(?i)^(?:\d{1,2}\.\s+)?(\d+(?:\.\d+)?)\s+credits?\s+from\s+(` + codePat + `(?:\s*/\s*(?:one\s+of\s+)?` + codePat + `(?:\s+or\s+` + codePat + `)?|\s+or\s+a\s+[a-z ]+course:\s*` + codePat + `)*)\s*(?:\(see\s+note\))?\.\s+(the\s+department\s+is\s+also\s+open\s+to\s+the\s+use\s+of\s+([1-4])00-level\s+lecture-based\s+course\(s\)\s+in\s+((?-i:[A-Z]{3})(?:\s*/\s*(?-i:[A-Z]{3}))*)\.\s+students\s+must\s+first\s+seek\s+approval\s+from\s+the\s+department\s+prior\s+to\s+enrolling\s+in\s+any\s+lecture-based\s+[1-4]00-level\s+course\(s\)\s+not\s+listed)\.?$`)
	// complOneOfOrRe: "one of BIO260H1 or HMB265H1" in a list: one choice.
	complOneOfOrRe = regexp.MustCompile(`(?i)\bone\s+of\s+(` + codePat + `)\s+or\s+(` + codePat + `)`)
	// complShareOfTotalRe: a header's share of other designators: "Students
	// complete 4.0 credits, of which up to 1.0 credit may be taken in
	// SPA/LAS courses." The groups: the amount, the designators.
	complShareOfTotalRe = regexp.MustCompile(`(?i)^students\s+complete\s+(\d+(?:\.\d+)?)\s+credits,\s+of\s+which\s+up\s+to\s+\d+(?:\.\d+)?\s+credits?\s+may\s+be\s+taken\s+in\s+((?-i:[A-Z]{3})(?:\s*/\s*(?-i:[A-Z]{3}))*)\s+courses\.?$`)
	// complAdditionalScienceRe: "2. Additional credits from 300/400-level
	// CHM/ MAT/ another science, including at least three of CHM317H1, ...,
	// CHM410H1. Minimum 3.0 CHM credits at the 400-level to a total of 14.0
	// credits." (audit FIX-ONE ASSPE1376). The groups: the levels, the
	// designators, the floor, its designator, its level, the total.
	complAdditionalScienceRe = regexp.MustCompile(`(?i)^(?:\d{1,2}\.\s+)?additional\s+credits\s+from\s+([1-4]00(?:/[1-4]00)*)-level\s+((?-i:[A-Z]{3})(?:\s*/\s*(?-i:[A-Z]{3}))*)\s*/\s*another\s+science,\s+including\s+at\s+least\s+\w+\s+of\s+[^.]+\.\s+minimum\s+(\d+(?:\.\d+)?)\s+((?-i:[A-Z]{3}))\s+credits\s+at\s+the\s+([1-4])00-level\s+to\s+a\s+total\s+of\s+(\d+(?:\.\d+)?)\s+credits\.?$`)
	// complStreamCoursesRe: a header floor on the courses of the program's
	// stream: "including 2.0 credits from 300+ level Literature and Language
	// courses", when a note's limit of 0 names the courses that "may not be
	// counted toward fulfilling the requirements for the Stream in
	// Literature and Language" (audit FIX-ONE ASMAJ1647A). The groups: the
	// amount, the level, the stream.
	complStreamCoursesRe = regexp.MustCompile(`(?i)^including\s+(\d+(?:\.\d+)?)\s+credits?\s+from\s+([1-4]00\+)\s+level\s+([A-Z][\w ]+?)\s+courses$`)
	// complLanguagesFloorRe: "At least 9.0 FAH credits, and 2.0 credits in
	// one or more languages including at least one German, French, or
	// Italian, though an acceptable alternative modern language such as
	// Dutch or Russian (or Chinese and/or another Asian language) may be
	// acceptable" (audit FIX-ONE ASSPE0908). The groups: the floor, its
	// designator, the languages' sentence (the amount, the languages), the
	// alternative.
	complLanguagesFloorRe = regexp.MustCompile(`(?i)^at\s+least\s+(\d+(?:\.\d+)?)\s+((?-i:[A-Z]{3}))\s+credits,\s+and\s+((\d+(?:\.\d+)?)\s+credits\s+in\s+one\s+or\s+more\s+languages\s+including\s+at\s+least\s+one\s+((?:[A-Z]\w+,\s+)*[A-Z]\w+,?\s+or\s+[A-Z]\w+)),\s+(though\s+an\s+acceptable\s+alternative\s+modern\s+language\b.*\bmay\s+be\s+acceptable)\.?$`)
	// complSeriesRe: "PSL300-series (excluding PSL300H1, PSL301H1)" among an
	// item's courses: the designator's courses at the level, less those
	// (audit FIX-ONE ASMIN0482). The groups: the designator, the level, the
	// courses excluded.
	complSeriesRe = regexp.MustCompile(`\b([A-Z]{3})([1-4])00-series(?:\s*\(excluding\s+([^()]*)\))?`)
	// complCodeChoiceRe: one member of a list of courses, or a "/" choice;
	// a code with no campus ("CSB332H") names no catalogue course.
	complCodeChoiceRe = regexp.MustCompile(`^(?:` + codePat + `|[A-Z]{3}\d{3}[HY])(?:\s*/\s*(?:` + codePat + `|[A-Z]{3}\d{3}[HY]))*$`)
	// complTwoSetsRe: "2.0 credits from: [( SLA107Y1/ SLA117H1, SLA118H1);
	// SLA214H1; ...]; / [ SLA109Y1, SLA209H1, SLA213H1]": the amount from
	// one set or the other (audit FIX-ONE ASSPE1205 r24). The groups: the
	// amount, the sets.
	complTwoSetsRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+from:\s*\[([^\[\]]+)\]\s*;?\s*/\s*\[([^\[\]]+)\]$`)
	// complGroupChoiceRe: "( SLA107Y1/ SLA117H1, SLA118H1)" in a set: one
	// course or the two. The groups: the course, the two.
	complGroupChoiceRe = regexp.MustCompile(`\(\s*(` + codePat + `)\s*/\s*(` + codePat + `)\s*,\s*(` + codePat + `)\s*\)`)
	// complRequiredIfBeganRe: "Note that PRT420H1 is required for students
	// who began their Portuguese studies with 200+ level PRT language
	// courses" (audit FIX-ONE ASMAJ0338, ASMIN0338 r4). The group: the
	// course.
	complRequiredIfBeganRe = regexp.MustCompile(`(?i)^(?:note\s+that\s+)?(` + codePat + `)\s+is\s+required\s+for\s+students\s+who\s+began\s+their\s+\w+\s+studies\s+with\s+[1-4]00\+\s+level\s+(?-i:[A-Z]{3})\s+language\s+courses\.?$`)
	// complSelectRemainingRe: "8. Select the remaining credits for a total of
	// 12.0 credits from: BIO251H1, ...; all EEB courses (excluding EEB196H1,
	// ...); EHJ352H1; ...; and no more than 1.0 credit from the following
	// (note that ...): ANT336H1, ..., PSY497H1 (note that ...)" (audit
	// FIX-ONE ASSPE1006). The groups: the total, the pool, the cap, the
	// capped courses.
	complSelectRemainingRe = regexp.MustCompile(`(?i)^(?:\d{1,2}\.\s+)?select\s+the\s+remaining\s+credits\s+for\s+a\s+total\s+of\s+(\d+(?:\.\d+)?)\s+credits\s+from:\s*(.+?);\s*and\s+no\s+more\s+than\s+(\d+(?:\.\d+)?)\s+credits?\s+from\s+the\s+following(?:\s*\([^()]*\))?:\s*(.+?)(?:\s*\(note\s+that[^()]*\))?$`)
	// complAllDesigExceptRe: "all EEB courses (excluding EEB196H1, ...)".
	// The groups: the designator, the courses excluded.
	complAllDesigExceptRe = regexp.MustCompile(`(?i)(?:^|;\s*)all\s+((?-i:[A-Z]{3}))\s+courses\s*\(excluding\s+([^()]+)\)\s*(?:;|$)`)
	// complRelatedSelectedRe: "2.5 credits in RSM courses at the 300+-level,
	// with at least 1.0 credit at the 400-level, and at least 0.5 credit must
	// be related to International Business, selected from RSM437H1, ...,
	// RSM491H1, RSM Summer Abroad courses, and special topics courses as
	// determined by the program office" (audit FIX-ONE ASSPE2431). The
	// groups: the amount, the designator, the level, the floor, its level,
	// the share, its courses, the courses the office decides.
	complRelatedSelectedRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits\s+in\s+((?-i:[A-Z]{3}))\s+courses\s+at\s+the\s+([1-4])00\+-level,\s+(with\s+at\s+least\s+(\d+(?:\.\d+)?)\s+credits?\s+at\s+the\s+([1-4])00-level),\s+and\s+(at\s+least\s+(\d+(?:\.\d+)?)\s+credits?\s+must\s+be\s+related\s+to\s+[\w ]+?,\s+selected\s+from\s+((?:` + codePat + `,\s+)+)((?-i:[A-Z]{3})\b.+?\bas\s+determined\s+by\s+the\s+program\s+office))$`)
	// complFromRowsDesigRe: "Any GGR course from the list for Core Courses and
	// Fourth Year Courses not already used". The groups: the designator, the
	// two rows.
	complFromRowsDesigRe = regexp.MustCompile(`(?i)^any\s+((?-i:[A-Z]{3}))\s+course\s+from\s+the\s+list\s+for\s+([\w ]+?)\s+and\s+([\w ]+?)\s+not\s+already\s+used\.?$`)
)

// remarkRows reads, once the routes are placed, rows that need the rows
// around them (audit S2-10, S2-11, R5):
//   - a language placement route as the row after the language row
//     (complLanguagePlacementRe): an alternative of that row,
//     and(credits{N}, text{placement}), with the row's other routes;
//   - complSectionsFloorRe: a constraint on the courses the sections claim
//     (a scope reference), under Req 1;
//   - complShareOfTotalRe on a total of any credits: the program's own
//     designator (the one other designator its text names) and the
//     share's are the strict pool, any course the lenient one (ambiguity);
//   - complReqsFloorRe: a constraint on the courses the rows named claim
//     at the level, or the text for the courses of no level the parser
//     reads;
//   - complLinesBelowRe: or(course, and(the other course, credits{the
//     rest, from the pools of the lines named, in the row's section}));
//   - complListAnyLevelRe: listAnyLevel; complUpToByCourseRe: upToByCourse;
//     complOpenToLevelRe: openToLevel;
//   - complAdditionalScienceRe: additionalScience; complStreamCoursesRe:
//     streamCourses; complLanguagesFloorRe: languagesFloor; complSeriesRe:
//     seriesList; complTwoSetsRe: twoSets;
//   - dropChoiceText: the sentence of a choice row is no text of another;
//   - complRequiredIfBeganRe: a constraint or(the course, text{placement}),
//     a must-include that claims nothing (the pool beside it counts the
//     course);
//   - complSelectRemainingRe: selectRemaining; complRelatedSelectedRe:
//     relatedSelected;
//   - complFromRowsDesigRe beside a list pool: the designator's courses of
//     the rows named join the list.
func (a *compAssembler) remarkRows(lists []*types.CourseList) []*types.CourseList {
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
		for _, l := range regexp.MustCompile(`\d+`).FindAllString(m[3], -1) {
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
		// The 400 level the parser reads; the D-level (UTSC) and graduate
		// courses it can't: the text.
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
		// complRequiredIfBeganRe: the course, or the level the student
		// began at, which the transcript doesn't show (placement).
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

// listAnyLevel reads complListAnyLevelRe as row r's body (remarkRows): a list
// of its own, of the courses and the designator's courses at the level,
// whose "/" choices are capped, and a constraint of the floor. It returns
// the list, or nil.
func (a *compAssembler) listAnyLevel(r *compRow, lists []*types.CourseList) *types.CourseList {
	n := r.r.Req
	if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || r.r.Label == "" {
		return nil
	}
	m := complListAnyLevelRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(complClean(n.Text)), "."))
	if m == nil {
		return nil
	}
	rest := complWhicheverRe.ReplaceAllString(m[4], "")
	anyLoc := complAnyLevelCourseRe.FindStringSubmatchIndex(rest)
	if anyLoc == nil {
		return nil
	}
	level, desig := rest[anyLoc[2]:anyLoc[3]], rest[anyLoc[4]:anyLoc[5]]
	rest = strings.TrimSpace(rest[:anyLoc[0]] + ", " + rest[anyLoc[1]:])
	rest = strings.Trim(rest, ",; ")
	v, _ := strconv.ParseFloat(m[1], 64)
	floor, _ := strconv.ParseFloat(m[2], 64)
	pool, caps := codesCredits(m[1], rest)
	name := "Requirement " + r.r.Label + " courses"
	if pool == nil || floor >= v || slices.ContainsFunc(lists, func(l *types.CourseList) bool { return l.Name == name }) {
		return nil
	}
	l := &types.CourseList{Name: name, Items: []*types.ReqNode{
		{Type: types.NodeCredits, Courses: pool.Courses},
		{Type: types.NodeCredits, Designators: []string{desig}, Levels: []int{int(level[0]-'0') * 100}},
	}}
	r.r.Req = &types.ReqNode{Type: types.NodeCredits, Amount: v, Lists: []string{name}}
	kids := []*compRow{{r: &types.Requirement{Kind: types.RequirementConstraint, Section: r.r.Section, Raw: rawHead(r.r),
		Req: &types.ReqNode{Type: types.NodeCredits, Amount: floor, Levels: parseLevels(m[3] + "00-level")}}}}
	for _, c := range caps {
		c.Section = r.r.Section
		kids = append(kids, &compRow{r: c})
	}
	r.children = append(kids, r.children...)
	return l
}

// shareOfTotal reads complShareOfTotalRe (remarkRows).
func (a *compAssembler) shareOfTotal() {
	if a.total == nil {
		return
	}
	n := a.total.r.Req
	if n == nil || n.Type != types.NodeCredits || !isAnyCredits(n) {
		return
	}
	var sent string
	for _, l := range strings.Split(a.total.r.Raw, "\n") {
		if complShareOfTotalRe.MatchString(strings.TrimSpace(l)) {
			sent = strings.TrimSuffix(strings.TrimSpace(l), ".")
		}
	}
	if sent == "" {
		return
	}
	m := complShareOfTotalRe.FindStringSubmatch(sent)
	shared := regexp.MustCompile(`[A-Z]{3}`).FindAllString(m[2], -1)
	var own []string
	for _, c := range codeRe.FindAllString(a.raw, -1) {
		own = append(own, c[:3])
	}
	for _, d := range regexp.MustCompile(`\b([A-Z]{3})\s+(?:language\s+)?courses\b`).FindAllStringSubmatch(a.raw, -1) {
		own = append(own, d[1])
	}
	own = slices.DeleteFunc(dedupe(own), func(d string) bool { return slices.Contains(shared, d) })
	if len(own) != 1 {
		return
	}
	strict := &types.ReqNode{Type: types.NodeCredits, Amount: n.Amount, Designators: append(own, shared...)}
	a.total.r.Req = orNode(strict, andNode(&types.ReqNode{Type: types.NodeCredits, Amount: n.Amount}, textNode(types.TextAmbiguity, sent)))
}

// fromRowsDesignator reads complFromRowsDesigRe (remarkRows).
func (a *compAssembler) fromRowsDesignator(lists []*types.CourseList) {
	named := func(name string) *compRow {
		var found []*compRow
		walkRows(a.items, func(r *compRow) {
			if r.r.Kind == types.RequirementAllocation && (strings.EqualFold(r.r.Label, name) || r.r.Label == "" && strings.EqualFold(r.r.Section+" Courses", name)) {
				found = append(found, r)
			}
		})
		if len(found) != 1 {
			return nil
		}
		return found[0]
	}
	walkRows(a.items, func(r *compRow) {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr || n.Items[0].Type != types.NodeCredits || len(n.Items[0].Lists) != 1 {
			return
		}
		li := slices.IndexFunc(lists, func(l *types.CourseList) bool { return l.Name == n.Items[0].Lists[0] })
		for i, it := range n.Items {
			m := complFromRowsDesigRe.FindStringSubmatch(strings.TrimSpace(it.Text))
			if it.Type != types.NodeText || m == nil || li < 0 {
				continue
			}
			x, y := named(m[2]), named(m[3])
			if x == nil || y == nil {
				return
			}
			var codes []string
			for _, row := range []*compRow{x, y} {
				for _, c := range listedCodes(row.r.Req, lists) {
					if c[:3] == m[1] {
						codes = append(codes, c)
					}
				}
			}
			if codes == nil {
				return
			}
			lists[li].Items = append(lists[li].Items, &types.ReqNode{Type: types.NodeCredits, Courses: dedupe(codes)})
			n.Items = slices.Delete(n.Items, i, i+1)
			r.r.Req = orNode(n.Items...)
			return
		}
	})
}

// listedCodes returns the courses n names, those of the lists it draws on
// included; texts and filters name none.
func listedCodes(n *types.ReqNode, lists []*types.CourseList) []string {
	if n == nil || n.Type == types.NodeText {
		return nil
	}
	var out []string
	if n.Type == types.NodeCourse {
		out = append(out, n.Code)
	}
	out = append(out, n.Courses...)
	for _, name := range n.Lists {
		for _, l := range lists {
			if l.Name == name {
				for _, it := range l.Items {
					out = append(out, listedCodes(it, lists)...)
				}
			}
		}
	}
	for _, it := range n.Items {
		out = append(out, listedCodes(it, lists)...)
	}
	return out
}

// languageLevel reads complLanguageLevelRe, an allocation of its middle
// sentence and a constraint of its first (remarkRows): the allocation is
// the amount from either language's list or the other list at the 200+
// level, at the grade, or the rule as placement (equivalent proficiency a
// person judges); the constraint goes.
func (a *compAssembler) languageLevel(lists []*types.CourseList) {
	has := func(name string) bool {
		return slices.ContainsFunc(lists, func(l *types.CourseList) bool { return l.Name == name })
	}
	walkRows(a.items, func(r *compRow) {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || len(r.children) > 0 {
			return
		}
		m := complLanguageLevelRe.FindStringSubmatch(strings.TrimSpace(complClean(r.r.Raw)))
		if m == nil || !has(m[3]) || !has(m[4]) || !has(m[7]) || m[5] != m[6] {
			return
		}
		var rule *compRow
		walkRows(append(slices.Clone(a.items), a.program...), func(q *compRow) {
			if q.r.Kind == types.RequirementConstraint && q.r.Req != nil && q.r.Req.Type == types.NodeText && rawWords(q.r.Req.Text) == rawWords(m[1]) {
				rule = q
			}
		})
		if a.total != nil {
			for _, q := range a.total.children {
				if q.r.Kind == types.RequirementConstraint && q.r.Req != nil && q.r.Req.Type == types.NodeText && rawWords(q.r.Req.Text) == rawWords(m[1]) {
					rule = q
				}
			}
		}
		v, _ := strconv.ParseFloat(m[5], 64)
		g, _ := strconv.Atoi(m[2])
		pool := func(list string, levels []int) *types.ReqNode {
			return &types.ReqNode{Type: types.NodeCredits, Amount: v, Lists: []string{list}, Levels: levels, MinGrade: g}
		}
		r.r.Req = orNode(pool(m[3], nil), pool(m[4], nil), pool(m[7], []int{200, 300, 400}), textNode(types.TextPlacement, m[1]))
		r.r.Notes = nil
		if rule != nil {
			a.dropRow(rule)
		}
	})
}

// upToByCourse reads complUpToByCourseRe, an item of two sentences, as row
// r's body (remarkRows): or(the larger amount, and(the smaller, not(the
// courses that need the larger))). Who completed the one course is a fact of
// the transcript that a course node would claim, though another row claims
// it; not having taken the others says it, as that row needs one of them.
func (a *compAssembler) upToByCourse(r *compRow) {
	if r.r.Kind != types.RequirementAllocation || r.r.Req == nil || !hasText(r.r.Req) {
		return
	}
	m := complUpToByCourseRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(complClean(rawHead(r.r))), "."))
	if m == nil || m[1] != m[5] {
		return
	}
	big, caps := codesCredits(m[5], m[6])
	if big == nil {
		return
	}
	small := cloneNode(big)
	small.Amount, _ = strconv.ParseFloat(m[3], 64)
	if small.Amount >= big.Amount {
		return
	}
	others := courseNodes(codeRe.FindAllString(m[4], -1))
	alts := []*types.ReqNode{big, andNode(small, &types.ReqNode{Type: types.NodeNot, Items: []*types.ReqNode{orNode(others...)}})}
	if n := r.r.Req; n.Type == types.NodeOr {
		// The routes placed on the item ("Health-related Anthropology
		// Special Topics courses ... will count") stay its alternatives.
		alts = append(alts, n.Items[1:]...)
	} else {
		// A program sentence that lets more courses count, left a note: the
		// list isn't all the row counts, so it keeps a Review alternative
		// (content: the course's topic decides).
		for _, l := range strings.Split(a.raw, "\n") {
			if l = strings.TrimSuffix(strings.TrimSpace(l), "."); complWillCountRe.MatchString(l) {
				alts = append(alts, textNode(types.TextContent, l))
			}
		}
	}
	r.r.Req = orNode(alts...)
	for _, c := range caps {
		c.Section = r.r.Section
		r.children = append(r.children, &compRow{r: c})
	}
}

// openToLevel reads complOpenToLevelRe as row r's body (remarkRows): the
// courses, "one of A or B" capped, or with the department's approval the
// courses and the designators' courses at the level, a list of their own.
// It returns the list, or nil.
func (a *compAssembler) openToLevel(r *compRow, lists []*types.CourseList) *types.CourseList {
	if r.r.Kind != types.RequirementAllocation || r.r.Req == nil || !hasText(r.r.Req) || r.r.Label == "" || len(r.children) > 0 {
		return nil
	}
	m := complOpenToLevelRe.FindStringSubmatch(strings.TrimSpace(complClean(r.r.Raw)))
	name := "Requirement " + r.r.Label + " courses"
	if m == nil || slices.ContainsFunc(lists, func(l *types.CourseList) bool { return l.Name == name }) {
		return nil
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	codes := dedupe(codeRe.FindAllString(m[2], -1))
	strict := &types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: codes}
	l := &types.CourseList{Name: name, Items: []*types.ReqNode{
		{Type: types.NodeCredits, Courses: slices.Clone(codes)},
		{Type: types.NodeCredits, Designators: desigRe.FindAllString(m[5], -1), Levels: []int{int(m[4][0]-'0') * 100}},
	}}
	wide := &types.ReqNode{Type: types.NodeCredits, Amount: v, Lists: []string{name}}
	r.r.Req = orNode(strict, andNode(wide, textNode(types.TextPermission, m[3])))
	for _, p := range complOneOfOrRe.FindAllStringSubmatch(m[2], -1) {
		w := max(courseWeight(p[1]), courseWeight(p[2]))
		if w < v {
			r.children = append(r.children, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Section: r.r.Section, Raw: strings.TrimSpace(p[0]),
				Req: &types.ReqNode{Type: types.NodeCredits, Courses: []string{p[1], p[2]}, MaxAmount: maxAmount(w)}}})
		}
	}
	return l
}

// additionalScience reads complAdditionalScienceRe as row r's body
// (remarkRows): the row is its choice of courses; the additional credits
// are Req 1's pool, as "Additional <courses> to a total of N credits" is
// (additional), the designators' courses at the levels, or any course at
// the levels as content (what "another science" is); the minimum is a
// constraint under Req 1.
func (a *compAssembler) additionalScience(r *compRow) {
	n := r.r.Req
	if a.total == nil || r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeAnd || len(r.children) > 0 {
		return
	}
	t := a.total.r.Req
	m := complAdditionalScienceRe.FindStringSubmatch(strings.TrimSpace(complClean(rawHead(r.r))))
	if m == nil || t == nil || t.Type != types.NodeCredits || !isAnyCredits(t) || strconv.FormatFloat(t.Amount, 'f', 1, 64) != m[6] {
		return
	}
	var choice *types.ReqNode
	for _, it := range n.Items {
		switch {
		case it.Type == types.NodeChoose && choice == nil:
			choice = it
		case it.Type != types.NodeText:
			return
		}
	}
	if choice == nil {
		return
	}
	levels := parseLevels(m[1] + "-level")
	floor, _ := strconv.ParseFloat(m[3], 64)
	r.r.Req = choice
	a.total.r.Req = orNode(&types.ReqNode{Type: types.NodeCredits, Amount: t.Amount, Designators: desigRe.FindAllString(m[2], -1), Levels: levels},
		andNode(&types.ReqNode{Type: types.NodeCredits, Amount: t.Amount, Levels: slices.Clone(levels)}, textNode(types.TextContent, strings.SplitN(complClean(n.Items[0].Text), ",", 2)[0])))
	addRaw(a.total, rawHead(r.r))
	a.total.children = append(a.total.children, &compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Raw: strings.TrimSpace(regexp.MustCompile(`(?i)minimum\s.*$`).FindString(rawHead(r.r))),
		Req: &types.ReqNode{Type: types.NodeCredits, Amount: floor, Designators: []string{m[4]}, Levels: parseLevels(m[5] + "00-level")}}})
}

// streamCourses reads complStreamCoursesRe on a header floor, and(credits,
// text) (remarkRows): the stream's courses are Req 1's pool, which the limit
// of 0 already keeps the courses it names out of.
func (a *compAssembler) streamCourses() {
	if a.total == nil {
		return
	}
	t := a.total.r.Req
	if t == nil || t.Type != types.NodeCredits || t.Designators == nil {
		return
	}
	for _, r := range append(slices.Clone(a.floors), a.total.children...) {
		n := r.r.Req
		if r.r.Kind != types.RequirementConstraint || n == nil || n.Type != types.NodeAnd || len(n.Items) != 2 || n.Items[0].Type != types.NodeCredits || n.Items[1].Type != types.NodeText {
			continue
		}
		m := complStreamCoursesRe.FindStringSubmatch(strings.TrimSpace(n.Items[1].Text))
		if m == nil {
			continue
		}
		zero := false
		walkRows(append(slices.Clone(a.items), a.program...), func(q *compRow) {
			c := q.r.Req
			zero = zero || q.r.Kind == types.RequirementLimit && c != nil && c.Type == types.NodeCredits && c.MaxAmount != nil && *c.MaxAmount == 0 &&
				strings.Contains(strings.ToLower(q.r.Raw), "requirements for the stream in "+strings.ToLower(m[3]))
		})
		if !zero {
			continue
		}
		v, _ := strconv.ParseFloat(m[1], 64)
		// The floor counts what Req 1 counts: the level is all it adds.
		r.r.Req = &types.ReqNode{Type: types.NodeCredits, Amount: v, Levels: parseLevels(m[2] + "-level")}
	}
}

// languagesFloor reads complLanguagesFloorRe as row r's body (remarkRows):
// the floor is a constraint under Req 1; the row is the amount in the named
// languages' designators, which hold other courses too (content: the course
// is a language course), or another language (permission: "may be
// acceptable").
func (a *compAssembler) languagesFloor(r *compRow) {
	n := r.r.Req
	if a.total == nil || r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || len(r.children) > 0 {
		return
	}
	m := complLanguagesFloorRe.FindStringSubmatch(strings.TrimSpace(complClean(n.Text)))
	if m == nil {
		return
	}
	var desigs []string
	for _, l := range regexp.MustCompile(`[A-Z]\w+`).FindAllString(m[5], -1) {
		d := languageDesignators[strings.ToLower(l)]
		if d == nil {
			return
		}
		desigs = append(desigs, d...)
	}
	floor, _ := strconv.ParseFloat(m[1], 64)
	v, _ := strconv.ParseFloat(m[4], 64)
	r.r.Req = orNode(andNode(&types.ReqNode{Type: types.NodeCredits, Amount: v, Designators: desigs}, textNode(types.TextContent, m[3])), textNode(types.TextPermission, m[6]))
	a.total.children = append(a.total.children, &compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Raw: "At least " + m[1] + " " + m[2] + " credits",
		Req: &types.ReqNode{Type: types.NodeCredits, Amount: floor, Designators: []string{m[2]}}}})
}

// seriesList reads "N credits from: <courses and series>" (complSeriesRe) as
// row r's body (remarkRows): a list of its own, of the courses and each
// series, whose "/" choices are capped. A code with no campus is no course
// the pool can hold: the sentence is then the bounded alternative, with
// any N credits. It returns the list, or nil.
func (a *compAssembler) seriesList(r *compRow, lists []*types.CourseList) *types.CourseList {
	n := r.r.Req
	if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || r.r.Label == "" || len(r.children) > 0 {
		return nil
	}
	m := regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+from:\s*(.+)$`).FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(complClean(n.Text)), "."))
	name := "Requirement " + r.r.Label + " courses"
	if m == nil || !complSeriesRe.MatchString(m[2]) || slices.ContainsFunc(lists, func(l *types.CourseList) bool { return l.Name == name }) {
		return nil
	}
	var items []*types.ReqNode
	for _, g := range complSeriesRe.FindAllStringSubmatch(m[2], -1) {
		items = append(items, &types.ReqNode{Type: types.NodeCredits, Designators: []string{g[1]}, Levels: []int{int(g[2][0]-'0') * 100}, Exclude: codeRe.FindAllString(g[3], -1)})
	}
	rest := complSeriesRe.ReplaceAllString(m[2], "")
	partial := false
	var kept []string
	for _, part := range regexp.MustCompile(`[,;]`).Split(rest, -1) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !complCodeChoiceRe.MatchString(part) {
			return nil
		}
		codes := codeRe.FindAllString(part, -1)
		partial = partial || len(codes) < len(strings.Split(part, "/"))
		if codes != nil {
			kept = append(kept, strings.Join(codes, "/ "))
		}
	}
	pool, caps := codesCredits(m[1], strings.Join(kept, ", "))
	if pool == nil {
		return nil
	}
	l := &types.CourseList{Name: name, Items: append([]*types.ReqNode{{Type: types.NodeCredits, Courses: pool.Courses}}, items...)}
	body := &types.ReqNode{Type: types.NodeCredits, Amount: pool.Amount, Lists: []string{name}}
	r.r.Req = body
	if partial {
		r.r.Req = orNode(body, andNode(&types.ReqNode{Type: types.NodeCredits, Amount: pool.Amount}, textNode(types.TextOther, n.Text)))
	}
	for _, c := range caps {
		c.Section = r.r.Section
		r.children = append(r.children, &compRow{r: c})
	}
	return l
}

// twoSets reads complTwoSetsRe as row r's body (remarkRows): or(credits{N,
// the one set}, credits{N, the other}), with the row's routes. A choice of a
// course or two in a set ("( SLA107Y1/ SLA117H1, SLA118H1)") is a limit of
// the larger weight on the three.
func (a *compAssembler) twoSets(r *compRow) {
	n := r.r.Req
	if r.r.Kind != types.RequirementAllocation || n == nil || len(r.children) > 0 {
		return
	}
	head, routes := n, []*types.ReqNode(nil)
	if n.Type == types.NodeOr {
		head, routes = n.Items[0], n.Items[1:]
	}
	if head.Type != types.NodeText || slices.ContainsFunc(routes, func(x *types.ReqNode) bool { return x.Type != types.NodeText }) {
		return
	}
	m := complTwoSetsRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(complClean(head.Text)), "."))
	if m == nil {
		return
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	var caps []*types.Requirement
	var sets []*types.ReqNode
	for _, set := range m[2:4] {
		for _, g := range complGroupChoiceRe.FindAllStringSubmatch(set, -1) {
			w := max(courseWeight(g[1]), courseWeight(g[2])+courseWeight(g[3]))
			caps = append(caps, &types.Requirement{Kind: types.RequirementLimit, Section: r.r.Section, Raw: strings.TrimSpace(g[0]),
				Req: &types.ReqNode{Type: types.NodeCredits, Courses: []string{g[1], g[2], g[3]}, MaxAmount: maxAmount(w)}})
		}
		plain := strings.NewReplacer("(", " ", ")", " ", "/", ",").Replace(set)
		if !isCodeList(plain) {
			return
		}
		sets = append(sets, &types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: dedupe(codeRe.FindAllString(set, -1))})
	}
	r.r.Req = orNode(append(sets, routes...)...)
	for _, c := range caps {
		r.children = append(r.children, &compRow{r: c})
	}
}

// dropChoiceText drops, from an allocation's and(), a text that is the
// sentence of a choice row ("Students must choose any two of the available
// language areas (...)", which the choice of areas reads): it is that row's.
func (a *compAssembler) dropChoiceText() {
	var said []string
	walkRows(append(slices.Clone(a.items), a.program...), func(r *compRow) {
		if r.r.Kind == types.RequirementChoice {
			said = append(said, rawWords(strings.Split(r.r.Raw, "\n")[0]))
		}
	})
	if said == nil {
		return
	}
	var drop func(n *types.ReqNode) *types.ReqNode
	drop = func(n *types.ReqNode) *types.ReqNode {
		switch n.Type {
		case types.NodeOr:
			for i, it := range n.Items {
				n.Items[i] = drop(it)
			}
		case types.NodeAnd:
			n.Items = slices.DeleteFunc(n.Items, func(x *types.ReqNode) bool {
				return x.Type == types.NodeText && slices.Contains(said, rawWords(x.Text))
			})
			if len(n.Items) == 1 {
				return n.Items[0]
			}
		}
		return n
	}
	walkRows(a.items, func(r *compRow) {
		if r.r.Kind == types.RequirementAllocation && r.r.Req != nil && hasText(r.r.Req) {
			r.r.Req = drop(r.r.Req)
		}
	})
}

// selectRemaining reads complSelectRemainingRe as Req 1's pool (remarkRows), as
// "Additional <courses> to a total of N credits" is (additional): the
// designator's courses less those excluded, and the courses, with a limit
// on the capped ones. The row goes; its raw joins Req 1's, its children
// (the floor it states) become the program's. A "/" choice in the pool
// isn't capped: Req 1 counts what the rows above claim too, which may hold
// the other course.
func (a *compAssembler) selectRemaining(r *compRow) {
	n := r.r.Req
	if a.total == nil || r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText {
		return
	}
	t := a.total.r.Req
	m := complSelectRemainingRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(complClean(n.Text)), "."))
	if m == nil || t == nil || t.Type != types.NodeCredits || !isAnyCredits(t) || fmtAmount(t.Amount) != m[1] || !isCodeList(m[4]) {
		return
	}
	pool := m[2]
	var desigs, excl []string
	for _, g := range complAllDesigExceptRe.FindAllStringSubmatch(pool, -1) {
		desigs = append(desigs, g[1])
		excl = append(excl, codeRe.FindAllString(g[2], -1)...)
	}
	pool = complAllDesigExceptRe.ReplaceAllString(pool, "; ")
	if desigs == nil || !isCodeList(strings.Trim(pool, "; ")) {
		return
	}
	capped := dedupe(codeRe.FindAllString(m[4], -1))
	v, _ := strconv.ParseFloat(m[3], 64)
	t.Designators, t.Exclude = desigs, excl
	t.Courses = dedupe(append(codeRe.FindAllString(pool, -1), capped...))
	addRaw(a.total, rawHead(r.r))
	a.dropRow(r)
	a.program = append(a.program, r.children...)
	a.program = append(a.program, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Raw: "no more than " + m[3] + " credit from the following",
		Req: &types.ReqNode{Type: types.NodeCredits, Courses: capped, MaxAmount: maxAmount(v)}}})
}

// relatedSelected reads complRelatedSelectedRe as row r's body (remarkRows):
// the amount in the designator at the level, a constraint of the floor, and
// one of the share from the courses named, or from the designator's courses
// the program office decides (permission).
func (a *compAssembler) relatedSelected(r *compRow) {
	n := r.r.Req
	if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || len(r.children) > 0 {
		return
	}
	m := complRelatedSelectedRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(complClean(n.Text)), "."))
	if m == nil {
		return
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	floor, _ := strconv.ParseFloat(m[5], 64)
	share, _ := strconv.ParseFloat(m[8], 64)
	r.r.Req = &types.ReqNode{Type: types.NodeCredits, Amount: v, Designators: []string{m[2]}, Levels: parseLevels(m[3] + "00+-level")}
	r.children = append(r.children,
		&compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Section: r.r.Section, Raw: m[4],
			Req: &types.ReqNode{Type: types.NodeCredits, Amount: floor, Levels: parseLevels(m[6] + "00-level")}}},
		&compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Section: r.r.Section, Raw: m[7],
			Req: orNode(&types.ReqNode{Type: types.NodeCredits, Amount: share, Courses: codeRe.FindAllString(m[9], -1)},
				andNode(&types.ReqNode{Type: types.NodeCredits, Amount: share, Designators: []string{m[2]}}, textNode(types.TextPermission, m[10])))}})
}
