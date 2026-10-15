package types

import (
	"database/sql/driver"
	"reflect"
	"testing"
)

func TestEnrolmentRoundTrip(t *testing.T) {
	in := Enrolment{
		Raw:  "This is a limited enrolment program. ...",
		Kind: EnrolmentLimited,
		Req: &ReqNode{Type: NodeAnd, Items: []*ReqNode{
			{Type: NodeCredits, Amount: 4, MaxAmount: 8.5},
			{Type: NodeCourse, Code: "BIO130H1", MinGrade: 65},
			{Type: NodeAverage, MinGrade: 70, Amount: 2, Courses: []string{"BIO120H1", "BIO130H1", "CHM151Y1"}},
			{Type: NodeProgram, Programs: []string{"ASMAJ0826"}, Text: "Criminology and Sociolegal Studies Major (ASMAJ0826)"},
			{Type: NodeText, Kind: TextAdmission, Text: "variable minimum grade"},
			{Type: NodeNot, Items: []*ReqNode{{Type: NodeProfile, Attr: "admission_category", Value: "Computer Science", Text: "students admitted to other Arts & Science Year 1 admission categories"}}},
			{Type: NodeNot, Items: []*ReqNode{{Type: NodeCourse, Code: "CSC207H1"}}},
		}},
		Exclusions: []*ReqNode{
			{Type: NodeProgram, Programs: []string{"ASSPE1013", "ASMIN1013"}, Text: "Sociology Specialist or Minor"},
			{Type: NodeText, Kind: TextMembership, Text: "other CIRHR program"},
		},
		FirstRoundOnly: true,
		Notes:          []string{"Please consult the Arts & Science Program Toolkit for more information on program enrolment"},
	}
	v, err := in.Value()
	if err != nil {
		t.Fatal(err)
	}
	var out Enrolment
	if err := out.Scan(v); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round trip mismatch:\n in: %+v\nout: %+v", in, out)
	}
	var fromBytes Enrolment
	if err := fromBytes.Scan([]byte(v.(string))); err != nil || !reflect.DeepEqual(in, fromBytes) {
		t.Errorf("[]byte scan failed: %v", err)
	}

	// A suspended program has no requirement: "req" is null, not missing.
	s := Enrolment{Raw: "**... administratively suspended ...**", Kind: EnrolmentSuspended}
	if v, _ := s.Value(); v != `{"kind":"suspended","req":null,"raw":"**... administratively suspended ...**"}` {
		t.Errorf("suspended JSON = %s", v)
	}
}

func TestNilEnrolmentBecomesNULL(t *testing.T) {
	var e *Enrolment
	got, err := driver.DefaultParameterConverter.ConvertValue(e)
	if err != nil || got != nil {
		t.Errorf("got %v, %v; want nil, nil", got, err)
	}
}
