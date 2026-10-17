package reqs

import (
	"strings"
	"testing"
)

const (
	faseGroup = "students in the Faculty of Applied Science and Engineering"
	bothNote  = "Faculty of Arts and Science co-requisite and pre-requisite requirements do not apply to the students in the Faculty of Applied Science and Engineering who choose to take this course"
	preNote   = "Faculty of Arts and Science pre-requisite requirements do not apply to the students in the Faculty of Applied Science and Engineering who choose to take this course"
)

func TestExemptionInField(t *testing.T) {
	tests := []struct {
		name, raw, want string
		notes           []string
	}{
		// Names both fields, so it covers the field it is in, whatever that is.
		{"names both fields", "( ECO101H1,  ECO102H1)/  ECO105Y1. " + bothNote + ".",
			jOr(jAnd(jC("ECO101H1"), jC("ECO102H1")), jC("ECO105Y1"), jT("membership", faseGroup)),
			[]string{bothNote}},
		// Names no field: the field it is in.
		{"names no field", "CSC209H1; STA247H1. These requirements do not apply to Engineering students.",
			jOr(jAnd(jC("CSC209H1"), jC("STA247H1")), jT("membership", "Engineering students")),
			[]string{"These requirements do not apply to Engineering students"}},
		{"does not apply", "MAT137Y1. This requirement does not apply to FASE students.",
			jOr(jC("MAT137Y1"), jT("membership", "FASE students")),
			[]string{"This requirement does not apply to FASE students"}},
		// The clause is glued to the last code with its "." missing; a
		// label branch for the same group already says what it needs.
		{"glued to a code, group has a label",
			"For students in the Faculty of Arts and Science:  MUN221H1/  ECO200Y1 For students in the Faculty of Applied Science and Engineering:  CHE249H1/  MIE358H1 " + bothNote + ".",
			jOr(
				jAnd(jT("membership", "For students in the Faculty of Arts and Science"), jOr(jC("MUN221H1"), jC("ECO200Y1"))),
				jAnd(jT("membership", "For students in the Faculty of Applied Science and Engineering"), jOr(jC("CHE249H1"), jC("MIE358H1")))),
			[]string{bothNote}},
		{"engineering label covers FASE", "CSC209H1 Engineering students: ESC180H1. These requirements do not apply to FASE students.",
			jOr(jC("CSC209H1"), jAnd(jT("membership", "Engineering students"), jC("ESC180H1"))),
			[]string{"These requirements do not apply to FASE students"}},
		// Names one field: ParseRequisite can't tell which field it is in, so it only
		// moves the sentence to notes for ApplyExemptions.
		{"names one field", "For students in the Faculty of Applied Science and Engineering:  MUN302H1.  " + preNote + ".",
			jAnd(jT("membership", "For students in the Faculty of Applied Science and Engineering"), jC("MUN302H1")),
			[]string{preNote}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := ParseRequisite(tt.raw)
			assertJSON(t, r.Req, tt.want)
			assertJSON(t, r.Notes, jsonList(tt.notes))
		})
	}
}

func TestExemptionOnlySentence(t *testing.T) {
	r := ParseRequisite(preNote + ".")
	assertJSON(t, r.Req, `null`)
	assertJSON(t, r.Notes, jsonList([]string{preNote}))
}

func TestExemptionNotMatched(t *testing.T) {
	for _, s := range []string{
		"These requirements do not apply to this course",                                                                         // no group
		"Prerequisites do not apply to students who have completed CSC108H1",                                                     // a course condition, not an audience
		"CSC108H1; these requirements do not apply to FASE students",                                                             // not a sentence start
		"Students who completed MAT137Y1 in an earlier term and then took a gap year requirements do not apply to FASE students", // subject too long
		"CSC108H1 (prerequisites do not apply to FASE students)",
		"Geography pre- or co-requisites waived for Anthropology and Archaeology students",
	} {
		if e, ok := findExemption(s); ok {
			t.Errorf("findExemption(%q) = %+v, want none", s, e)
		}
	}
}

func TestApplyExemptions(t *testing.T) {
	pre := ParseRequisite("( ECO101H1,  ECO102H1)/  ECO105Y1")
	co := ParseRequisite("For students in the Faculty of Applied Science and Engineering:  CHE249H1/  MIE358H1 " + preNote + ".")
	wantPre := jOr(jAnd(jC("ECO101H1"), jC("ECO102H1")), jC("ECO105Y1"), jT("membership", faseGroup))
	wantCo := jAnd(jT("membership", "For students in the Faculty of Applied Science and Engineering"), jOr(jC("CHE249H1"), jC("MIE358H1")))
	for range 2 { // applying twice changes nothing
		ApplyExemptions(pre, co)
		assertJSON(t, pre.Req, wantPre)
		assertJSON(t, pre.Notes, jsonList([]string{preNote}))
		assertJSON(t, co.Req, wantCo)
		assertJSON(t, co.Notes, jsonList([]string{preNote}))
	}

	// Naming both fields: the corequisites' own label covers the group, the
	// prerequisites get the alternative.
	pre = ParseRequisite("PPG301H1/  MUN320H1")
	co = ParseRequisite("For students in the Faculty of Applied Science and Engineering:  MUN302H1. " + bothNote)
	ApplyExemptions(pre, co)
	assertJSON(t, pre.Req, jOr(jC("PPG301H1"), jC("MUN320H1"), jT("membership", faseGroup)))
	assertJSON(t, co.Req, jAnd(jT("membership", "For students in the Faculty of Applied Science and Engineering"), jC("MUN302H1")))

	// An exemption in the field it names.
	pre = ParseRequisite("MAT137Y1. Faculty of Arts and Science pre-requisite requirements do not apply to FASE students.")
	assertJSON(t, pre.Req, jC("MAT137Y1"))
	ApplyExemptions(pre, nil)
	assertJSON(t, pre.Req, jOr(jC("MAT137Y1"), jT("membership", "FASE students")))

	// A field with no requirement has nothing to be exempt from.
	co = ParseRequisite(preNote)
	ApplyExemptions(nil, co)
	assertJSON(t, co.Req, `null`)
}

// Audience labels and exemptions give the group the same node: a program
// text alternative to the general requirement.
func TestAudienceLabelShapes(t *testing.T) {
	assertJSON(t, ParseRequisite("MAT224H1. For FASE students, MAT185H1,  AER210H1.").Req,
		jOr(jC("MAT224H1"), jAnd(jT("membership", "For FASE students"), jC("MAT185H1"), jC("AER210H1"))))
	assertJSON(t, ParseRequisite("MAT224H1 Prerequisite for Faculty of Applied Science and Engineering students: MAT185H1, AER210H1").Req,
		jOr(jC("MAT224H1"), jAnd(jT("membership", "Engineering students"), jC("MAT185H1"), jC("AER210H1"))))

	// A comma label needs a course or group after it.
	for _, tok := range lex("For FASE students, permission of the instructor") {
		if tok.kind == tCond {
			t.Errorf("comma label without a course after it lexed as a label")
		}
	}

	key := audienceKey(faseGroup)
	for _, g := range []string{"Engineering students", "FASE students", "For students in the Faculty of Applied Science and Engineering", "Applied Science and Engineering students"} {
		if got := audienceKey(g); got != key {
			t.Errorf("audienceKey(%q) = %q, want %q", g, got, key)
		}
	}
	if audienceKey("For students in the Faculty of Arts and Science") == key {
		t.Errorf("Arts and Science matched Engineering")
	}
}

func jsonList(ss []string) string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = jsonString(s)
	}
	return "[" + strings.Join(out, ",") + "]"
}
