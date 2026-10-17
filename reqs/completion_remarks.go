package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Item bodies the grammar can't read for a bracketed remark or a clause on
// courses already used, and other single wordings of item bodies.

var (
	// complCodeSeeNoteRe matches a note pointer after a course: "JPM400Y1 (see
	// NOTE 3)".
	complCodeSeeNoteRe = regexp.MustCompile(`(?i)(` + codePat + `)\s*\((see\s+note\s+\d+)\)`)
	// complCodeFormerlyRe matches a course's old code, which counts too:
	// "SMC366H1 (_formerly SMC218Y1_)".
	complCodeFormerlyRe = regexp.MustCompile(`(` + codePat + `)\s*\(\s*_?formerly\s+(` + codePat + `)_?\s*\)`)
	// complRemarkAdviceRe matches a trailing remark that is advice: "(The
	// courses ... are recommended.)".
	complRemarkAdviceRe = regexp.MustCompile(`(?i)^note\s+that\b|\b(?:is|are)\s+recommended\.?$`)
	// complRemarkCanBeUsedRe matches a cap on the course before it: "(0.5 credit
	// can be used based on internship)".
	complRemarkCanBeUsedRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+can\s+be\s+used\b[^()]*$`)
	// complRemarkConsideredRe matches courses that join the pool with approval:
	// "( STA399H1 and STA399Y1 may be considered on a case by case basis with the
	// approval of the Associate Chair)".
	complRemarkConsideredRe = regexp.MustCompile(`(?i)^((?:` + codePat + `)(?:\s*(?:,|and|or)\s*` + codePat + `)*)\s+may\s+be\s+considered\b.*\bapproval\b`)
	// complRemarkOnlyIfRe matches "( PHL245H1 may only be counted here if (
	// MAT158H1, MAT159H1/ MAT157Y1) is not taken)".
	complRemarkOnlyIfRe = regexp.MustCompile(`(?i)^(` + codePat + `)\s+may\s+only\s+be\s+counted\s+here\s+if\s+(.+?)\s+is\s+not\s+taken$`)
	// complAmountFromCodesRe matches an amount from a list of courses after a
	// description: "0.5 credit in biological diversity and function from:
	// BIO251H1, BIO270H1; EEB266H1".
	complAmountFromCodesRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+(?:(?:in\s+[a-z][a-z\s]+?\s+)?(?:from|of)(?:\s+the\s+following(?:\s+\([^()]*\))?)?|from\s+remaining\s+[^:]*?\bnot\s+used\s+toward\s+other\s+program\s+requirements\s+in\s+the\s+following\s+list):?\s*(` + codePat + `(?:\s*(?:,|;|/|or)\s*` + codePat + `)*)$`)
	// complFloorFromDesignatorRe matches a pool with a designator floor: "2.5
	// credits at least 1.5 credits from PCL courses: JPM300H1/ ...". The groups
	// are a course before it, the amount, the floor, the designator, the courses.
	complFloorFromDesignatorRe = regexp.MustCompile(`^(?:[A-Za-z][A-Za-z ]*:\s*)?(?:(` + codePat + `)\s*,?\s+and\s+)?` + amountPat + `\s+credits,?\s+(?:with\s+)?` + atLeastPat + `\s+` + amountPat + `\s+credits?\s+from\s+([A-Z]{3})\s+courses:\s*(` + codePat + `(?:\s*(?:/|or)\s*` + codePat + `)*)$`)
	// complNotUsedFromRe matches "Any course (0.5 credit) not used to satisfy
	// first year course requirement from JEG100H1, ...": exclusive allocation
	// already keeps a course from counting twice.
	complNotUsedFromRe = regexp.MustCompile(`(?i)^any\s+course\s+\(` + amountPat + `\s+credits?\)\s+not\s+used\s+to\s+satisfy\s+[\w\s]+?\s+requirements?\s+from\s+(.+)$`)
	// complOtherNotTakenRe matches the choice again: "The other of CHM210H1 or
	// ESS262H1/ JEG100H1 not taken in second year" (ASMAJ1076). Exclusive
	// allocation keeps the earlier row's course from counting.
	complOtherNotTakenRe = regexp.MustCompile(`(?i)^the\s+other\s+of\s+(.+?)\s+not\s+taken\s+in\s+(?:first|second|third|fourth)\s+year$`)
	// complUpToByCourseRe matches an amount that depends on a course taken: "Up
	// to 1.5 credits from the following courses, ... Students who complete ANT203Y1
	// need to take 1.0 credit, ..." (ASMIN1778).
	complUpToByCourseRe = regexp.MustCompile(`(?i)^(?:\d{1,2}\.\s+)?up\s+to\s+` + amountPat + `\s+credits\s+from\s+the\s+following\s+courses,\s+up\s+to\s+a\s+total\s+of\s+` + numPat + `\s+credits\s+to\s+complete\s+the\s+\w+\s+program\.\s+students\s+who\s+complete\s+(` + codePat + `)\s+need\s+to\s+take\s+` + amountPat + `\s+credits?,\s+and\s+students\s+who\s+take\s+(` + codePat + `(?:\s+or\s+` + codePat + `)*)\s+need\s+to\s+take\s+` + amountPat + `\s+credits,\s+from\s+the\s+following:\s*(.+)$`)
	// complWillCountRe matches "... Special Topics courses at the 300 or 400
	// level, ... will count towards program requirements". complRemainingAnyRe
	// matches "The remaining 1.5 - 2.0 credits can include any courses ...".
	complWillCountRe    = regexp.MustCompile(`(?i)^[A-Z][\w -]*\bcourses\b.*\bwill\s+count\s+towards?\s+program\s+requirements$`)
	complRemainingAnyRe = regexp.MustCompile(`(?i)^(?:\d{1,2}\.\s+)?the\s+remaining\s+` + amountPat + `\s*[-–]\s*` + numPat + `\s+credits\s+can\s+include\s+any\s+courses\s+not\s+previously\s+taken\b[^,:]*,\s*or\s+any\s+of\s+the\s+following:\s*(.+)$`)
)

// trailingParen splits s, when it ends with a bracket group, into the text
// before it and the group's inside.
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

// remarkItem takes a remark out of an item body the grammar leaves as text,
// reads the rest as the body, and gives the remark its own place: a note, a
// limit, a bounded route or an equivalent course. It reads nothing unless the
// rest reads with no text.
func (a *compAssembler) remarkItem(s string) (body *types.ReqNode, rows []*types.Requirement, notes []string, ok bool) {
	t := strings.TrimSuffix(strings.TrimSpace(complClean(s)), ".")
	if m := complRemainingAnyRe.FindStringSubmatch(t); m != nil && isCodeList(m[2]) {
		v, _ := strconv.ParseFloat(m[1], 64)
		// the courses listed are among any: a note
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

// complListSepRe matches a list separator: ";".
var complListSepRe = regexp.MustCompile(`[,;]`)

// codesCredits reads "N credit(s) from <courses>" as credits{N, the courses},
// or nil. A "/" choice inside a "," or ";" list gets a limit of its heaviest
// course's weight, when the choice can matter.
func codesCredits(amount, list string) (n *types.ReqNode, caps []*types.Requirement) {
	list = strings.TrimSuffix(strings.TrimSpace(list), ".")
	if !isCodeList(list) {
		return nil, nil
	}
	v, _ := strconv.ParseFloat(amount, 64)
	if strings.Contains(list, "/") && (strings.Contains(list, ",") || strings.Contains(list, ";")) {
		for _, part := range complListSepRe.Split(list, -1) {
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

// complCodeAtEndRe matches the course a text ends with: "... or MAT137Y1".
var complCodeAtEndRe = regexp.MustCompile(`(` + codePat + `)$`)

// codeAtEnd returns the course s ends with, or "".
func codeAtEnd(s string) string {
	m := complCodeAtEndRe.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	return m[1]
}

// floorFromDesignator reads complFloorFromDesignatorRe as credits{N, the
// list} and a constraint of the floor on the list's courses of the designator
// and joint (J) ones. before the grammar, which reads the floor as a pool
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

// remarkRows reads, once the routes are placed, rows that need the rows
// around them: a language placement route, floors on sections or rows, a
// choice from lines below, and the single wordings above.
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

// listAnyLevel reads complListAnyLevelRe as credits from a list of the row's
// own, of the courses and the designator's at the level, with the floor and
// the "/" caps under it. It returns the list, or nil.
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

// complShareDesigRe matches a designator: "FRE".
var complShareDesigRe = regexp.MustCompile(`[A-Z]{3}`)

// complDesigCoursesRe matches a designator's courses: "FRE language courses".
var complDesigCoursesRe = regexp.MustCompile(`\b([A-Z]{3})\s+(?:language\s+)?courses\b`)

// shareOfTotal reads complShareOfTotalRe on a total of any credits: the
// program's one designator and the share's are the strict pool, any course
// the lenient one.
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
	shared := complShareDesigRe.FindAllString(m[2], -1)
	var own []string
	for _, c := range codeRe.FindAllString(a.raw, -1) {
		own = append(own, c[:3])
	}
	for _, d := range complDesigCoursesRe.FindAllStringSubmatch(a.raw, -1) {
		own = append(own, d[1])
	}
	own = slices.DeleteFunc(dedupe(own), func(d string) bool { return slices.Contains(shared, d) })
	if len(own) != 1 {
		return
	}
	strict := &types.ReqNode{Type: types.NodeCredits, Amount: n.Amount, Designators: append(own, shared...)}
	a.total.r.Req = orNode(strict, andNode(&types.ReqNode{Type: types.NodeCredits, Amount: n.Amount}, textNode(types.TextAmbiguity, sent)))
}

// fromRowsDesignator adds to a list pool the designator's courses of the two
// rows complFromRowsDesigRe names.
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

// listedCodes returns the courses n names, its lists' included; texts and
// filters name none.
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

// languageLevel reads complLanguageLevelRe as the amount at the grade from
// either language's list or the other list at the 200+ level, or the rule as
// placement (a person judges equivalent proficiency). The rule's row goes.
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

// upToByCourse reads complUpToByCourseRe as or(the larger amount, and(the
// smaller, not(the courses that need the larger))): a course node for the one
// course would claim it from the row that does.
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
		// the routes placed on the item stay its alternatives
		alts = append(alts, n.Items[1:]...)
	} else {
		// a program sentence letting more courses count: the list isn't all the row
		// counts, so it keeps a Review alternative
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

// openToLevel reads complOpenToLevelRe as or(the courses, and(credits from a
// list of the courses and the designators' at the level, text{permission})),
// with "one of A or B" capped. It returns the list, or nil.
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

// complMinimumTailRe matches the text from "minimum" on: "minimum grade of 70%".
var complMinimumTailRe = regexp.MustCompile(`(?i)minimum\s.*$`)

// additionalScience reads complAdditionalScienceRe: the row keeps its choice
// of courses; the additional credits become Req 1's pool, the designators'
// courses or any at the levels as content; the minimum is a constraint under it.
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
	a.total.children = append(a.total.children, &compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Raw: strings.TrimSpace(complMinimumTailRe.FindString(rawHead(r.r))),
		Req: &types.ReqNode{Type: types.NodeCredits, Amount: floor, Designators: []string{m[4]}, Levels: parseLevels(m[5] + "00-level")}}})
}

// streamCourses reads complStreamCoursesRe on a header floor as a level floor
// on Req 1's pool, when a limit of 0 already keeps out what isn't the
// stream's.
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
		// the floor counts what Req 1 counts: the level is all it adds
		r.r.Req = &types.ReqNode{Type: types.NodeCredits, Amount: v, Levels: parseLevels(m[2] + "-level")}
	}
}

// complLanguageWordRe matches a capitalised word, digits and "_" too: "Italian".
var complLanguageWordRe = regexp.MustCompile(`[A-Z]\w+`)

// languagesFloor reads complLanguagesFloorRe: the floor is a constraint under
// Req 1, and the row is the languages' designators with content text, or
// another language with permission.
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
	for _, l := range complLanguageWordRe.FindAllString(m[5], -1) {
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

// complSeriesFromRe matches a list lead: "2.0 credits from: ANT100Y1, ...".
var complSeriesFromRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+from:\s*(.+)$`)

// seriesList reads "N credits from: <courses and series>" as credits from a
// list of its own, with the "/" choices capped. A code with no campus makes
// the sentence a bounded alternative. It returns the list, or nil.
func (a *compAssembler) seriesList(r *compRow, lists []*types.CourseList) *types.CourseList {
	n := r.r.Req
	if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || r.r.Label == "" || len(r.children) > 0 {
		return nil
	}
	m := complSeriesFromRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(complClean(n.Text)), "."))
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
	for _, part := range complListSepRe.Split(rest, -1) {
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

// twoSets reads complTwoSetsRe as or(credits{N, one set}, credits{N, the
// other}) with the row's routes, and a limit on each complGroupChoiceRe.
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

// dropChoiceText drops, from an allocation's and, a text that is a choice
// row's sentence: it is that row's.
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

// selectRemaining reads complSelectRemainingRe as Req 1's pool, with a limit
// on the capped courses; the row goes. A "/" choice isn't capped: Req 1 counts
// what the rows above claim too, which may hold the other course.
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

// relatedSelected reads complRelatedSelectedRe as credits of the designator
// at the level, with a constraint of the floor and one of the share from the
// courses named or, with permission, the designator's.
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
