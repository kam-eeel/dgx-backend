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
			{Type: NodeCredits, Amount: 4, MaxAmount: maxAmount(8.5)},
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

func TestCompletionJSON(t *testing.T) {
	in := Completion{
		Raw: "(7.0 credits, including at least 0.5 at the 400-level) ...",
		Requirements: []*Requirement{
			{Number: 1, Kind: RequirementAllocation, Req: &ReqNode{Type: NodeCredits, Amount: 7, Designators: []string{"POL", "JPI"}},
				Raw: "(7.0 credits, including at least 0.5 at the 400-level)"},
			{Number: 2, Kind: RequirementConstraint, Parent: 1, Req: &ReqNode{Type: NodeCredits, Amount: 0.5, Levels: []int{400}},
				Raw: "at least 0.5 at the 400-level"},
			{Number: 3, Kind: RequirementLimit, Parent: 1, Req: &ReqNode{Type: NodeCredits, Levels: []int{100}, MaxAmount: maxAmount(1)},
				Raw: "No more than 1.0 credit of 100-level courses may be used"},
			{Number: 4, Kind: RequirementAllocation, Label: "4", Section: "Higher Years", Parent: 1, Overlaps: []int{5},
				Req:   &ReqNode{Type: NodeCredits, Amount: 0.5, Lists: []string{"Canadian Politics"}},
				Notes: []string{"Consult the department for courses that count."}, Raw: "4. 0.5 credit in Canadian Politics"},
			{Number: 5, Kind: RequirementAllocation, Label: "6", Parent: 1, Overlaps: []int{4},
				Req: &ReqNode{Type: NodeCredits, Amount: 1, Lists: []string{"Group A"}, Requirements: []int{4}}, Raw: "6. 1.0 credit in Diversity"},
			{Number: 6, Kind: RequirementLimit, Parent: 1, Req: &ReqNode{Type: NodeCredits, Transfer: true, Levels: []int{300, 400}, MaxAmount: maxAmount(1)},
				Raw: "Transfer credits cannot comprise more than 1.0 credit at the 300/400-level"},
			{Number: 7, Kind: RequirementAllocation, Label: "Option A", Parent: 1, Raw: "Option A"},
			{Number: 8, Kind: RequirementAllocation, Parent: 7, Req: &ReqNode{Type: NodeCredits, Amount: 0.5, Courses: []string{"HIS208Y1"}, Weight: 0.5},
				Raw: "HIS208Y1 counts 0.5 toward each area"},
			{Number: 9, Kind: RequirementChoice, Req: &ReqNode{Type: NodeOr, Items: []*ReqNode{{Type: NodeRequirement, Requirements: []int{7}}}},
				Raw: "Option A or Option B"},
		},
		Lists: []*CourseList{
			{Name: "Group A", Title: "Group A (Diversity Courses)", Items: []*ReqNode{
				{Type: NodeCredits, Courses: []string{"POL224H1", "JPI201H1"}},
				{Type: NodeAnd, Items: []*ReqNode{
					{Type: NodeCredits, Designators: []string{"POL"}, Levels: []int{300, 400}},
					{Type: NodeText, Kind: TextPermission, Text: "as determined by the department"},
				}},
			}},
		},
		Exclusions: []*ReqNode{{Type: NodeProgram, Programs: []string{"ASSPE2015"}, Text: "Political Science Specialist"}},
		Notes:      []string{"Students are encouraged to consult the program office."},
	}
	v, err := in.Value()
	if err != nil {
		t.Fatal(err)
	}
	var out Completion
	if err := out.Scan(v); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round trip mismatch:\n in: %+v\nout: %+v", in, out)
	}
	var fromBytes Completion
	if err := fromBytes.Scan([]byte(v.(string))); err != nil || !reflect.DeepEqual(in, fromBytes) {
		t.Errorf("[]byte scan failed: %v", err)
	}

	// A cap of 0 is kept ("max_amount":0): the pointer is only omitted when nil.
	c := Completion{
		Raw: "...",
		Requirements: []*Requirement{
			{Number: 1, Kind: RequirementAllocation, Req: &ReqNode{Type: NodeCredits, Amount: 7, Designators: []string{"POL"}}, Raw: "(7.0 credits)"},
			{Number: 2, Kind: RequirementLimit, Parent: 1, Req: &ReqNode{Type: NodeCredits, CrNcr: true, MaxAmount: maxAmount(0)}, Raw: "CR/NCR courses cannot count"},
		},
		Lists: []*CourseList{{Name: "Group A", Items: []*ReqNode{{Type: NodeCredits, Courses: []string{"CSC209H1"}}}}},
	}
	want := `{"requirements":[{"number":1,"kind":"allocation","req":{"type":"credits","amount":7,"designators":["POL"]},"raw":"(7.0 credits)"},` +
		`{"number":2,"kind":"limit","parent":1,"req":{"type":"credits","cr_ncr":true,"max_amount":0},"raw":"CR/NCR courses cannot count"}],` +
		`"lists":[{"name":"Group A","items":[{"type":"credits","courses":["CSC209H1"]}]}],"raw":"..."}`
	if v, _ := c.Value(); v != want {
		t.Errorf("completion JSON =\n%s\nwant\n%s", v, want)
	}
}

// Nil slices are stored as [], never null: "requirements" and a list's
// "items" are always arrays. The caller's Completion is unchanged.
func TestCompletionEmptySlices(t *testing.T) {
	c := Completion{Lists: []*CourseList{{Name: "Group A"}}, Raw: "..."}
	want := `{"requirements":[],"lists":[{"name":"Group A","items":[]}],"raw":"..."}`
	if v, _ := c.Value(); v != want {
		t.Errorf("completion JSON =\n%s\nwant\n%s", v, want)
	}
	if c.Requirements != nil || c.Lists[0].Items != nil {
		t.Error("Value changed its receiver's lists")
	}
}

func TestNilCompletionBecomesNULL(t *testing.T) {
	var c *Completion
	got, err := driver.DefaultParameterConverter.ConvertValue(c)
	if err != nil || got != nil {
		t.Errorf("got %v, %v; want nil, nil", got, err)
	}
}
