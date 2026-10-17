package enrolment

import (
	"regexp"
	"slices"

	"degree-explorer/parser/grammar"
	"degree-explorer/parser/programs"
	"degree-explorer/types"
)

// codeFollowRe matches what may follow a near-code in enrolment text: what
// may follow one in a requisite, or a grade: "MAT159 (55%)".
var codeFollowRe = regexp.MustCompile(`^\s*(?:$|[/,);.:\]]|(?i:or|and)\b|\(\s*\d{2}\s*%\s*\))`)

// Repair repairs catalogue-confirmed codes in the raw text and parses
// it again, keeping Raw.
func Repair(e *types.Enrolment, known map[string]bool) *types.Enrolment {
	if e == nil {
		return nil
	}
	fixed, ok := grammar.RepairCodesFollowedBy(e.Raw, known, codeFollowRe)
	if !ok {
		return e
	}
	r := Parse(fixed)
	r.Raw = e.Raw
	return r
}

// oneFocusRe matches "Students can only apply for one focus."
var oneFocusRe = regexp.MustCompile(`(?i)\bcan\s+only\s+apply\s+for\s+one\s+focus\b`)

// OneFocus reads "Students can only apply for one focus" both ways: a student
// in a sibling focus is Review.
func OneFocus(e *types.Enrolment, self string, programList []*types.Program) {
	if !oneFocusRe.MatchString(e.Raw) || len(self) != 10 {
		return
	}
	var siblings []string
	for _, p := range programList {
		if len(p.Code) == 10 && p.Code[:9] == self[:9] && p.Code != self {
			siblings = append(siblings, p.Code)
		}
	}
	if siblings == nil {
		return
	}
	slices.Sort(siblings)
	e.Req = programs.AndNode(e.Req, programs.OrNode(
		&types.ReqNode{Type: types.NodeNot, Items: []*types.ReqNode{{Type: types.NodeProgram, Programs: siblings, Text: "another focus of this program"}}},
		programs.AmbiguousText(`"Students can only apply for one focus" may mean a student already in another focus can't apply`)))
}
