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
			{Type: NodeText, Kind: KindPermission, Text: "permission of instructor"},
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
		Raw: "MAT137Y1/ ( MAT137H5, MAT139H5)",
		Items: []ExclusionItem{
			{Codes: []string{"MAT137Y1"}},
			{Codes: []string{"MAT137H5", "MAT139H5"}},
			{Codes: []string{"RSM410H1"}, Note: "offered in Winter 2010"},
			{Kind: KindOther, Text: "FSL 200+ level courses"},
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
