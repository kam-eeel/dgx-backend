package types

import (
	"database/sql/driver"
	"reflect"
	"testing"
)

func TestRequisiteRoundTrip(t *testing.T) {
	in := Requisite{
		Raw: "CSC108H1/ (permission of instructor)",
		Req: &ReqNode{Type: NodeOr, Items: []*ReqNode{
			{Type: NodeCourse, Code: "CSC108H1", MinGrade: 70},
			{Type: NodeText, Kind: TextPermission, Text: "permission of instructor"},
			{Type: NodeProgram, Programs: []string{"ASMAJ1618", "ASMIN1254"}, Text: "Enrolment in ASMAJ1618/ASMIN1254"},
			{Type: NodeCourse, Code: "MAT257Y1", Concurrent: true},
			{Type: NodeCredits, MaxAmount: maxAmount(14), Designators: []string{"CSC"}},
			{Type: NodeAverage, MinGrade: 77, Amount: 3, Designators: []string{"HIS"}, Levels: []int{200, 300, 400}, Exclude: []string{"HIS262H1"}},
			{Type: NodeAverage, MinGrade: 80, Courses: []string{"MAT235H1", "MAT236H1"}},
		}},
		Notes: []string{"Students are encouraged to contact the department"},
	}
	v, err := in.Value()
	if err != nil {
		t.Fatal(err)
	}
	var out Requisite
	if err := out.Scan(v); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round trip mismatch:\n in: %+v\nout: %+v", in, out)
	}
	var fromBytes Requisite
	if err := fromBytes.Scan([]byte(v.(string))); err != nil || !reflect.DeepEqual(in, fromBytes) {
		t.Errorf("[]byte scan failed: %v", err)
	}
}

func TestExclusionsRoundTrip(t *testing.T) {
	in := Exclusions{
		Raw: "MAT137Y1/ ( MAT137H5, MAT139H5), FSL 200+ level courses",
		Items: []ExclusionItem{
			{Courses: []string{"MAT137Y1"}},
			{Courses: []string{"MAT137H5", "MAT139H5"}},
			{Courses: []string{"RSM410H1"}, Note: "offered in Winter 2010"},
			{Courses: []string{"FSL312H1"}, Concurrent: true},
			{Designators: []string{"FSL"}, Levels: []int{200, 300, 400}},
			{Designators: []string{"CSC"}, Exclude: []string{"CSC196H1"}, Note: "except AP Transfer Credits"},
			{Kind: TextPlacement, Text: "native speakers"},
		},
	}
	v, err := in.Value()
	if err != nil {
		t.Fatal(err)
	}
	var out Exclusions
	if err := out.Scan(v); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round trip mismatch:\n in: %+v\nout: %+v", in, out)
	}
}

func TestNilPointersBecomeNULL(t *testing.T) {
	var r *Requisite
	var e *Exclusions
	for _, v := range []any{r, e} {
		got, err := driver.DefaultParameterConverter.ConvertValue(v)
		if err != nil || got != nil {
			t.Errorf("%T: got %v, %v; want nil, nil", v, got, err)
		}
	}
}

func maxAmount(v float64) *float64 { return &v }
