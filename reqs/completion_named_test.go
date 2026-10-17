package reqs

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestListName(t *testing.T) {
	cases := []struct{ title, name, key string }{
		{"Group A", "Group A", "group A"},
		{"Group 1 - Psychology Courses", "Group 1", "group 1"},
		{"Cluster B - Learning, Perception, Cognition, and the Brain", "Cluster B", "cluster B"},
		{"Subgroup C (ii): (Society, Culture and Language – Linguistic)", "Subgroup C (ii)", "subgroup C(ii)"},
		{"Group A (Core African Studies Courses & by Africanist Faculty)", "Group A", "group A"},
		{"Group C: Society, Culture, and Language", "Group C", "group C"},
		{"List A (Thematic Group)", "List A", "list A"},
		{"Core Group", "Core Group", "core group"},
		{"Disciplinary/Thematic Cluster 1: Politics and Economics", "Cluster 1", "cluster 1"},
		{"(Sub-group: Western Europe)", "", ""},
		{"Society-Culture Courses", "", ""},
		{"Group Work", "", ""},
	}
	for _, c := range cases {
		name, key := listName(c.title)
		if name != c.name || key != c.key {
			t.Errorf("listName(%q) = %q, %q; want %q, %q", c.title, name, key, c.name, c.key)
		}
	}
}

func TestListRefNames(t *testing.T) {
	cases := []struct {
		ref  string
		want []string
	}{
		{"Group A", []string{"group A"}},
		{"Groups A and/or B", []string{"group A", "group B"}},
		{"Groups A, B, C, D", []string{"group A", "group B", "group C", "group D"}},
		{"Groups A-D", []string{"group A", "group B", "group C", "group D"}},
		{"Groups B through G", []string{"group B", "group C", "group D", "group E", "group F", "group G"}},
		{"Group A or Group B", []string{"group A", "group B"}},
		{"group A or B", []string{"group A", "group B"}},
		{"Group B/Group C", []string{"group B", "group C"}},
		{"Group A, B or C, or Subgroups C(i) or C(ii)", []string{"group A", "group B", "group C", "subgroup C(i)", "subgroup C(ii)"}},
		{"the Core Group or Group A", []string{"core group", "group A"}},
		{"Group 1", []string{"group 1"}},
		{"Groups A and", nil},
		{"Group a", nil},
	}
	for _, c := range cases {
		if got := listRefNames(c.ref); !reflect.DeepEqual(got, c.want) {
			t.Errorf("listRefNames(%q) = %q, want %q", c.ref, got, c.want)
		}
	}
}

func TestListMembers(t *testing.T) {
	cases := []struct {
		members string
		items   string // JSON
		except  []string
		ok      bool
	}{
		{"CSC209H1; any 300-/ 400-level CSC course _except_ those listed in Group B",
			`[{"type":"credits","courses":["CSC209H1"]},{"type":"credits","designators":["CSC"],"levels":[300,400]}]`, []string{"group B"}, true},
		{"BCB330Y1, BCB410H1; MAT223H1/ MAT240H1, ( MAT235H1, MAT236H1)/ MAT235Y1, any 300-/400-level MAT course except MAT329Y1, MAT390H1, MAT391H1; STA248H1, any 300-/400-level STA course; ECE419H1",
			`[{"type":"credits","courses":["BCB330Y1","BCB410H1","MAT223H1","MAT240H1","MAT235Y1","STA248H1","ECE419H1"]},{"type":"credits","designators":["MAT"],"levels":[300,400],"exclude":["MAT329Y1","MAT390H1","MAT391H1"]},{"type":"credits","designators":["STA"],"levels":[300,400]},{"type":"and","items":[{"type":"course","code":"MAT235H1"},{"type":"course","code":"MAT236H1"}]}]`, nil, true},
		{"CRI493H1, ENG356Y1/ ENG358H1 + ENG359H1, FAH251H1",
			`[{"type":"credits","courses":["CRI493H1","ENG356Y1","FAH251H1"]},{"type":"and","items":[{"type":"course","code":"ENG358H1"},{"type":"course","code":"ENG359H1"}]}]`, nil, true},
		{"Any 300-/ 400-level CSC course except CSC369H1, CSC373H1, and those listed in Group B",
			`[{"type":"credits","designators":["CSC"],"levels":[300,400],"exclude":["CSC369H1","CSC373H1"]}]`, []string{"group B"}, true},
		{"HIS219Y1(.50), HIS220Y1, HIS241H1",
			`[{"type":"credits","courses":["HIS220Y1","HIS241H1"]},{"type":"credits","courses":["HIS219Y1"],"weight":0.5}]`, nil, true},
		{"SMC366H1 (formerly SMC218Y1), CHC471H1",
			`[{"type":"credits","courses":["SMC366H1","SMC218Y1","CHC471H1"]}]`, nil, true},
		{"CSC300H1/ CSE240H1/ any ETH200+/ FOR201H1",
			`[{"type":"credits","courses":["CSC300H1","CSE240H1","FOR201H1"]},{"type":"credits","designators":["ETH"],"levels":[200,300,400]}]`, nil, true},
		{"Latin American Societies: ANT320H1, ANT340H1", `[{"type":"credits","courses":["ANT320H1","ANT340H1"]}]`, nil, true},
		{"ANT320H1, an independent studies course approved by the Program Committee", `[{"type":"credits","courses":["ANT320H1"]}]`, nil, true},
		{"POL314H1; POL334H; MUN421H1: MUN310H1", `[{"type":"credits","courses":["POL314H1","POL334H1","MUN421H1","MUN310H1"]}]`, nil, true},
		{"CJH332H1; CSB; EEB; EHJ352H1", `[{"type":"credits","courses":["CJH332H1","EHJ352H1"]},{"type":"credits","designators":["CSB","EEB"]}]`, nil, true},
		{"Pre-requisites: see the department", `null`, nil, false},
	}
	for _, c := range cases {
		items, except, _, ok := listMembersPerm(c.members)
		got, _ := json.Marshal(items)
		if string(got) != c.items || !reflect.DeepEqual(except, c.except) || ok != c.ok {
			t.Errorf("listMembersPerm(%q)\n got  %s %q %v\n want %s %q %v", c.members, got, except, ok, c.items, c.except, c.ok)
		}
	}
}

func TestListMembersPerm(t *testing.T) {
	_, _, perm, ok := listMembersPerm("CSC300H1/ any ETH200+/ SDS256H1/ another suitable course with permission from the Associate Chair, Undergraduate")
	if want := "another suitable course with permission from the Associate Chair, Undergraduate"; !ok || perm != want {
		t.Errorf("perm = %q, %v; want %q", perm, ok, want)
	}
}
