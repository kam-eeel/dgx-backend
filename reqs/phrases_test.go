package reqs

import (
	"reflect"
	"testing"
)

// firstPhrase runs the phrase matchers in lexer order and returns the first hit.
func firstPhrase(s string) (token, bool) {
	for _, m := range phraseMatchers {
		if t, ok := m(s); ok {
			return t, true
		}
	}
	return token{}, false
}

func TestPhraseNodes(t *testing.T) {
	tests := []struct {
		in       string
		kind     tokKind
		consumed string
		node     string // JSON of token.node
	}{
		// courses with grade suffixes
		{"STA247H1(70%)/ STA255H1", tCourse, "STA247H1(70%)", jCG("STA247H1", 70)},
		{"BCH311H1 (75% or higher)/ MGY311Y1", tCourse, "BCH311H1 (75% or higher)", jCG("BCH311H1", 75)},
		{"DRM220Y1 with a minimum of 70% or", tCourse, "DRM220Y1 with a minimum of 70%", jCG("DRM220Y1", 70)},
		{"CSC108H1/ CSC148H1", tCourse, "CSC108H1", jC("CSC108H1")},

		// credit phrases
		{"Completion of at least 4.0 credits", tCredits, "Completion of at least 4.0 credits", `{"type":"credits","amount":4}`},
		{"1.0 HIS or POL credit at the 100-level. Students", tCredits, "1.0 HIS or POL credit at the 100-level",
			`{"type":"credits","amount":1,"designators":["HIS","POL"],"levels":[100]}`},
		{"1.0 credit at the 200+ level in LIN/JAL/JUP/PSL/PSY/COG", tCredits, "1.0 credit at the 200+ level in LIN/JAL/JUP/PSL/PSY/COG",
			`{"type":"credits","amount":1,"designators":["LIN","JAL","JUP","PSL","PSY","COG"],"levels":[200,300,400]}`},
		{"1.0 credit in any 100-level course in ECO, HIS or POL", tCredits, "1.0 credit in any 100-level course in ECO, HIS or POL",
			`{"type":"credits","amount":1,"designators":["ECO","HIS","POL"],"levels":[100]}`},
		{"0.5 credit of any 200-level course in ANT, GGR, HIS, POL, or SOC", tCredits, "0.5 credit of any 200-level course in ANT, GGR, HIS, POL, or SOC",
			`{"type":"credits","amount":0.5,"designators":["ANT","GGR","HIS","POL","SOC"],"levels":[200]}`},
		{"2.0 ENG credits and any 4.0 credits", tCredits, "2.0 ENG credits", `{"type":"credits","amount":2,"designators":["ENG"]}`},
		{"At least 1.0 STA credit at the 300+ level with a minimum grade of 80% in each course", tCredits,
			"At least 1.0 STA credit at the 300+ level with a minimum grade of 80% in each course",
			`{"type":"credits","amount":1,"designators":["STA"],"levels":[300,400],"min_grade":80}`},
		{"3.0 HIS credits (excluding HIS262H1)", tCredits, "3.0 HIS credits (excluding HIS262H1)",
			`{"type":"credits","amount":3,"designators":["HIS"],"exclude":["HIS262H1"]}`},
		{"7.5 courses (in any field) with at least 1.5 in philosophy", tCredits, "7.5 courses (in any field) with at least 1.5 in philosophy",
			jAnd(`{"type":"credits","amount":7.5}`, `{"type":"credits","amount":1.5,"designators":["PHL"]}`)},
		{"an additional 1.0 FAH credit at the 300-level", tCredits, "an additional 1.0 FAH credit at the 300-level",
			`{"type":"credits","amount":1,"designators":["FAH"],"levels":[300]}`},
		{"1.5 credits from 200-level Chemistry courses", tCredits, "1.5 credits from 200-level Chemistry courses",
			`{"type":"credits","amount":1.5,"designators":["CHM"],"levels":[200]}`},

		// any-course
		{"A 300-level CDN course", tCredits, "A 300-level CDN course", `{"type":"credits","amount":0.5,"designators":["CDN"],"levels":[300]}`},
		{"Any AST 100-level or AST 200-level course", tCredits, "Any AST 100-level or AST 200-level course",
			`{"type":"credits","amount":0.5,"designators":["AST"],"levels":[100,200]}`},
		{"any 300-level course in Society", tCredits, "any 300-level course", `{"type":"credits","amount":0.5,"levels":[300]}`},

		// year standing
		{"Third and Fourth year students only", tCredits, "Third and Fourth year students only", `{"type":"credits","amount":9}`},
		{"In Year 2", tCredits, "In Year 2", `{"type":"credits","amount":4}`},
		{"4th year status in NMC Specialist program", tCredits, "4th year status", `{"type":"credits","amount":14}`},

		// breadth
		{"at least 1.0 of which should be in the HUM/BR=1/2", tCredits, "at least 1.0 of which should be in the HUM/BR=1/2",
			`{"type":"credits","amount":1,"breadth":[1,2]}`},

		// GPA
		{"Minimum GPA 3.5 for APM and MAT courses, permission", tGPA, "Minimum GPA 3.5 for APM and MAT courses",
			`{"type":"gpa","min_gpa":3.5,"scope":"APM and MAT courses"}`},
		{"a minimum GPA of B+ in Astronomy program courses and the permission", tGPA, "a minimum GPA of B+ in Astronomy program courses",
			`{"type":"gpa","min_gpa":3.3,"scope":"Astronomy program courses"}`},
		{"cGPA 3.0", tGPA, "cGPA 3.0", `{"type":"gpa","min_gpa":3}`},
	}
	for _, tt := range tests {
		tok, ok := firstPhrase(tt.in)
		if !ok {
			t.Errorf("%q: no phrase matched", tt.in)
			continue
		}
		if tok.kind != tt.kind || tt.in[:tok.end] != tt.consumed {
			t.Errorf("%q: got kind %v consuming %q; want kind %v consuming %q", tt.in, tok.kind, tt.in[:tok.end], tt.kind, tt.consumed)
			continue
		}
		assertJSON(t, tok.node, tt.node)
	}
}

func TestPhraseMarkers(t *testing.T) {
	tests := []struct {
		in       string
		want     token
		consumed string
	}{
		{"75% or higher in BCH242Y1", token{kind: tGradePrefix, grade: 75}, "75% or higher in"},
		{"A final grade of 75% or higher in each of BCH340H1", token{kind: tGradePrefix, grade: 75, each: true}, "A final grade of 75% or higher in each of"},
		{"For Specialist: BCH242Y1", token{kind: tCond}, "For Specialist:"},
		{"one of HIS241H1", token{kind: tPool, count: 1}, "one of"},
		{"at least one of EEB263H1", token{kind: tPool, count: 1}, "at least one of"},
		{"Two HIS courses from the following: HIS102Y1", token{kind: tPool, count: 2}, "Two HIS courses from the following:"},
		{"a course in statistics from EEB225H1", token{kind: tPool, count: 1}, "a course in statistics from"},
		{"of which at least one course needs to be HIS243H1", token{kind: tPool, count: 1}, "of which at least one course needs to be"},
		{"1.0 credit from 300-level French literature courses: FRE304H1", token{kind: tPool, amount: 1}, "1.0 credit from 300-level French literature courses:"},
		{"0.5 from PHL273H1", token{kind: tPool, amount: 0.5}, "0.5 from"},
		{"1.0 credit from: FOR300H1", token{kind: tPool, amount: 1}, "1.0 credit from:"},
		{"of which at least 0.5 credits in HPS100H1", token{kind: tPool, amount: 0.5}, "of which at least 0.5 credits in"},
	}
	for _, tt := range tests {
		tok, ok := firstPhrase(tt.in)
		tt.want.end = len(tt.consumed)
		if !ok || !reflect.DeepEqual(tok, tt.want) {
			t.Errorf("%q: got %+v (ok=%v), want %+v", tt.in, tok, ok, tt.want)
		}
	}
}

func TestPhraseNonMatches(t *testing.T) {
	for _, s := range []string{
		"first-year course offerings in the sciences", // not year standing
		"a course in the humanities",                  // no designator or level
		"permission of the instructor",
	} {
		if tok, ok := firstPhrase(s); ok {
			t.Errorf("%q: unexpectedly matched %+v", s, tok)
		}
	}
}

func TestParseLevels(t *testing.T) {
	for span, want := range map[string][]int{
		"at the 300-level":       {300},
		"at the 300+ level":      {300, 400},
		"300-level or higher":    {300, 400},
		"in 300-/400-level":      {300, 400},
		"300 or 400 level":       {300, 400},
		"200-level or 300-level": {200, 300},
		"at the 200+":            {200, 300, 400},
		"in POL/ JPA":            nil,
	} {
		if got := parseLevels(span); !reflect.DeepEqual(got, want) {
			t.Errorf("parseLevels(%q) = %v, want %v", span, got, want)
		}
	}
}
