package reqs

import (
	"reflect"
	"testing"
)

// Goldens for wordings that used to fall through to "other" text: breadth,
// grade and GPA variants, year standing, full courses, subject names,
// maximum credits and "of which" parts.
var reqGoldens2 = []struct{ name, raw, want string }{
	// breadth
	{"BR list with bare numbers", "0.5 credit in BR=1/ 2/ 3", `{"type":"credits","amount":0.5,"breadth":[1,2,3]}`},
	{"BR= with spaces", "4.0 credits, including a 0.5 credit in BR= 1/ BR= 2",
		jAnd(`{"type":"credits","amount":4}`, `{"type":"credits","amount":0.5,"breadth":[1,2]}`)},
	{"BR without =", "3.0 credits in BR 1/BR 2", `{"type":"credits","amount":3,"breadth":[1,2]}`},
	{"BR and/or", "2.0 credits in BR 2 and/or BR 3", `{"type":"credits","amount":2,"breadth":[2,3]}`},
	{"BR or a combination", "2.0 credits at the 200+ level in BR 1 or BR 2 or BR 3 or a combination of BR 1, BR 2, and BR 3.",
		`{"type":"credits","amount":2,"levels":[200,300,400],"breadth":[1,2,3]}`},
	{"Breadth Requirement words", "4.0 credits, including a 0.5 credit in either Breadth Requirement 1 or 2",
		jAnd(`{"type":"credits","amount":4}`, `{"type":"credits","amount":0.5,"breadth":[1,2]}`)},
	{"breadth category of", "Any 1.0 credit combination of courses carrying a breadth requirement (BR) category of 4 or 5.",
		`{"type":"credits","amount":1,"breadth":[4,5]}`},
	{"label/BR", "Completion of 4.0 credits in Humanities/BR=1 or 2", `{"type":"credits","amount":4,"breadth":[1,2]}`},
	{"parenthesized label/BR", "8.0 credits including 1.0 credit in Geography (SOC SCI/BR=3)",
		jAnd(`{"type":"credits","amount":8}`, `{"type":"credits","amount":1,"designators":["GGR"],"breadth":[3]}`)},
	{"BR in the credit head", "3.0 BR= 1/ 2 credits at the 200+ level",
		`{"type":"credits","amount":3,"levels":[200,300,400],"breadth":[1,2]}`},
	{"BR list stops before the next amount", "0.5 credit in BR=1/ BR=2 and 1.0 credit from any of AER, ANA",
		jAnd(`{"type":"credits","amount":0.5,"breadth":[1,2]}`, `{"type":"credits","amount":1,"designators":["AER","ANA"]}`)},
	{"bare Humanities stays text", "3.0 credits in Humanities",
		jAnd(`{"type":"credits","amount":3}`, jT("other", "in Humanities"))},

	// grades after a course
	{"(minimum of N%)", "EAS151H1 (minimum of 60%) or equivalent as determined by the placement process",
		jOr(jCG("EAS151H1", 60), jT("placement", "equivalent as determined by the placement process"))},
	{"(minimum grade N%)", "EAS100Y1 (minimum grade 67%)/ EAS101Y1 (minimum 67%)", jOr(jCG("EAS100Y1", 67), jCG("EAS101Y1", 67))},
	{"(N% minimum)", "EAS110Y1 (63% minimum)/ EAS310Y1 (70% minimum)", jOr(jCG("EAS110Y1", 63), jCG("EAS310Y1", 70))},
	{"(N% or higher grade)", "MAT158H1/  MAT148H1(70% or higher grade)", jOr(jC("MAT158H1"), jCG("MAT148H1", 70))},
	{"(with a minimum mark of at least N%)", "CSC110Y1 (with a minimum mark of at least 85%)/  CSC165H1 (with a minimum mark of at least 85%)",
		jOr(jCG("CSC110Y1", 85), jCG("CSC165H1", 85))},
	{"minimum mark without %", "( MAT136H1 with a minimum mark of 77)/ ( MAT149H1 with a minimum mark of 73)",
		jOr(jCG("MAT136H1", 77), jCG("MAT149H1", 73))},
	{"with a mark of N% or above", "HIS282Y1 with a mark of 76% or above", jCG("HIS282Y1", 76)},
	{"letter grade or higher", "HIS250Y1 (B or higher) or  HIS351Y1 (B or higher)", jOr(jCG("HIS250Y1", 73), jCG("HIS351Y1", 73))},

	// grades after a group or list
	{"suffix after a group", "CHM151Y1/ ( CHM135H1,  CHM136H1) with a minimum grade of 63%",
		jOr(jC("CHM151Y1"), jAnd(jCG("CHM135H1", 63), jCG("CHM136H1", 63)))},
	{"in each after a group", "( CHM220H1/  CHM222H1/  MSE202H1,  CHM223H1) with a minimum grade of 68% in each",
		jAnd(jOr(jCG("CHM220H1", 68), jCG("CHM222H1", 68), jCG("MSE202H1", 68)), jCG("CHM223H1", 68))},
	{"grade after text stays with the text", "( MAT158H1 (55%), MAT (55%)/  MAT157Y1(55%))",
		jAnd(jCG("MAT158H1", 55), jOr(jT("other", "MAT (55%)"), jCG("MAT157Y1", 55)))},
	{"with a minimum of N% in is still a prefix", "Major in Drama with a minimum of 70% in  DRM220Y1",
		jAnd(jP("Major in Drama with"), jCG("DRM220Y1", 70))},
	{"letter grade on credits", "Completion of 10.0 credits including at least 2.0 MST credits with at least a B+ grade",
		jAnd(`{"type":"credits","amount":10}`, `{"type":"credits","amount":2,"designators":["MST"],"min_grade":77}`)},

	// "minimum of" before a credit phrase
	{"Minimum of N credit", "Minimum of 1.0 credit in ESS/ JEG/ JGA/ JPE courses at the 200+level",
		`{"type":"credits","amount":1,"designators":["ESS","JEG","JGA","JPE"],"levels":[200,300,400]}`},
	{"A combined minimum of", "A combined minimum of 1.0 credit from POL and/or SDS courses",
		`{"type":"credits","amount":1,"designators":["POL","SDS"]}`},
	{"Minimum of one course", "Minimum of one SOC course at the 300+ level",
		`{"type":"credits","amount":0.5,"designators":["SOC"],"levels":[300,400]}`},
	{"Completion minimum of", "Completion minimum of 9.0 credits", `{"type":"credits","amount":9}`},

	// GPA
	{"Cumulative GPA of at least", "9.0 credits, Cumulative GPA of at least 2.70",
		jAnd(`{"type":"credits","amount":9}`, `{"type":"gpa","min":2.7}`)},
	{"with a CGPA of at least", "5.0 credits with a CGPA of at least 2.5", jAnd(`{"type":"credits","amount":5}`, `{"type":"gpa","min":2.5}`)},
	{"GPA scope in all credits", "Completion of at least 2.0 CHM credits with a minimum cGPA of 3.0 in all CHM credits",
		jAnd(`{"type":"credits","amount":2,"designators":["CHM"]}`, `{"type":"gpa","min":3,"scope":"CHM credits"}`)},
	{"number before GPA", "14.0 credits, 3.0 GPA in Political Science courses",
		jAnd(`{"type":"credits","amount":14}`, `{"type":"gpa","min":3,"scope":"Political Science courses"}`)},
	{"CGPA required", "3.0 CGPA required", `{"type":"gpa","min":3}`},
	{"a CGPA of at least, then permission", "a CGPA of at least 3.0 and permission of the Undergraduate Coordinator",
		jAnd(`{"type":"gpa","min":3}`, jT("permission", "permission of the Undergraduate Coordinator"))},
	{"letter average", "at least 3.0 BMS credits with at least a B+ average",
		jAnd(`{"type":"credits","amount":3,"designators":["BMS"]}`, `{"type":"gpa","min":3.3,"scope":"BMS courses"}`)},
	{"letter average without designators stays text", "2.0 credits with a B+ average",
		jAnd(`{"type":"credits","amount":2}`, jT("other", "with a B+ average"))},
	{"percent average stays text", "a 77% average in 3.0 HIS credits",
		jAnd(jT("other", "a 77% average in"), `{"type":"credits","amount":3,"designators":["HIS"]}`)},

	// year standing
	{"must be in ordinal year", "A student must be in third or fourth year.", `{"type":"credits","amount":9}`},
	{"must be enrolled in Nth year", "Must be enrolled in 4th year. 3.0 CGPA required.",
		jAnd(`{"type":"credits","amount":14}`, `{"type":"gpa","min":3}`)},
	{"students in their year of study", "Available to students in their third year of study (who have completed at least 9.0 credits)",
		`{"type":"credits","amount":9}`},

	// full courses
	{"a full course in", "PHL245H1 and a full course in PHL/CSC/MAT",
		jAnd(jC("PHL245H1"), `{"type":"credits","amount":1,"designators":["PHL","CSC","MAT"]}`)},
	{"One full course in", "One full course in MAT/PHY", `{"type":"credits","amount":1,"designators":["MAT","PHY"]}`},
	{"a full course equivalent in", "a full course equivalent in ECO", `{"type":"credits","amount":1,"designators":["ECO"]}`},

	// subject names
	{"Canadian Studies", "4.0 credits or 1.0 credit in Canadian Studies",
		jOr(`{"type":"credits","amount":4}`, `{"type":"credits","amount":1,"designators":["CDN"]}`)},
	{"Jewish Studies and course counts", "CJS200H1 and at least two other 0.5 credit courses in Jewish Studies",
		jAnd(jC("CJS200H1"), `{"type":"credits","amount":1,"designators":["CJS"]}`)},
	{"Study of Religion", "1.0 credit at the 300-level in the Study of Religion", `{"type":"credits","amount":1,"designators":["RLG"],"levels":[300]}`},
	{"political theory is not a designator", "2.0 credits in Political Theory",
		jAnd(`{"type":"credits","amount":2}`, jT("other", "in Political Theory"))},

	// maximum credits
	{"cannot have completed more than", "Students cannot have completed more than 8.5 credits", `{"type":"credits","max":8.5}`},
	{"no more than", "At least 8.5 credits and no more than 14.0 credits",
		jAnd(`{"type":"credits","amount":8.5}`, `{"type":"credits","max":14}`)},
	{"no more than ... completed", "A minimum of 8.5 credits and no more than 14.0 credits completed",
		jAnd(`{"type":"credits","amount":8.5}`, `{"type":"credits","max":14}`)},

	// "of which" parts inherit the filters they don't narrow
	{"of which N must be", "10.0 credits, of which 2.0 credits must be 300+ level ESS/ JEG/ JGA/ JPE courses",
		jAnd(`{"type":"credits","amount":10}`, `{"type":"credits","amount":2,"designators":["ESS","JEG","JGA","JPE"],"levels":[300,400]}`)},
	{"of which inherits designators", "2.0 LIN credits at the 200+ level, of which 1.0 LIN credit must be at the 300+ level",
		jAnd(`{"type":"credits","amount":2,"designators":["LIN"],"levels":[200,300,400]}`,
			`{"type":"credits","amount":1,"designators":["LIN"],"levels":[300,400]}`)},
	{"N of which must be", "14.5 credits, 5.0 of which must be GGR/URB",
		jAnd(`{"type":"credits","amount":14.5}`, `{"type":"credits","amount":5,"designators":["GGR","URB"]}`)},
	{"N credit of which should be in", "Completion of 14.0 credits, 1.0 credit of which should be in CHC/ SMC/ RLG",
		jAnd(`{"type":"credits","amount":14}`, `{"type":"credits","amount":1,"designators":["CHC","SMC","RLG"]}`)},
	{"of which should be an X credit", "4.0 credits, of which 0.5 should be an HPS credit.",
		jAnd(`{"type":"credits","amount":4}`, `{"type":"credits","amount":0.5,"designators":["HPS"]}`)},
	{"with N at level", "9.0 credits, including 1.5 HPS credits with 0.5 at the 300+ level",
		jAnd(`{"type":"credits","amount":9}`, `{"type":"credits","amount":1.5,"designators":["HPS"]}`,
			`{"type":"credits","amount":0.5,"designators":["HPS"],"levels":[300,400]}`)},
	{"with N credits in", "7.5 credits with 1.5 credits in PHL",
		jAnd(`{"type":"credits","amount":7.5}`, `{"type":"credits","amount":1.5,"designators":["PHL"]}`)},

	// other credit wordings
	{"level before designator", "1.0 200-level CAS credit and 1.0 300-level CAS credit",
		jAnd(`{"type":"credits","amount":1,"designators":["CAS"],"levels":[200]}`, `{"type":"credits","amount":1,"designators":["CAS"],"levels":[300]}`)},
	{"designator before level", "1.0 POL 200-level credit", `{"type":"credits","amount":1,"designators":["POL"],"levels":[200]}`},
	{"credits completed", "TRN250Y1 and at least 14.0 credits completed", jAnd(jC("TRN250Y1"), `{"type":"credits","amount":14}`)},
	{"must have completed", "EEB498Y1 and must have completed 14.0 credits.", jAnd(jC("EEB498Y1"), `{"type":"credits","amount":14}`)},
	{"level-led course", "200-level History course/  DHU235H1",
		jOr(`{"type":"credits","amount":0.5,"designators":["HIS"],"levels":[200]}`, jC("DHU235H1"))},
	{"one additional course", "STA355H1; one additional 400-level STA course",
		jAnd(jC("STA355H1"), `{"type":"credits","amount":0.5,"designators":["STA"],"levels":[400]}`)},
}

func TestParseRequisiteGolden2(t *testing.T) {
	for _, g := range reqGoldens2 {
		t.Run(g.name, func(t *testing.T) {
			assertJSON(t, ParseRequisite(g.raw).Req, g.want)
		})
	}
}

func TestPhraseNodes2(t *testing.T) {
	tests := []struct {
		in       string
		kind     tokKind
		consumed string
		node     string // JSON of token.node
	}{
		{"EAS110Y1 (70% minimum)/ EAS211Y1", tCourse, "EAS110Y1 (70% minimum)", jCG("EAS110Y1", 70)},
		{"CSB330H1 with a minimum grade of 77% and approval", tCourse, "CSB330H1 with a minimum grade of 77%", jCG("CSB330H1", 77)},
		{"EAS301H1 with a minimum grade of A- OR", tCourse, "EAS301H1 with a minimum grade of A-", jCG("EAS301H1", 80)},
		// "in each" belongs to the list, so the course leaves it alone
		{"CHM237H1 with a minimum grade of 68% in each course", tCourse, "CHM237H1", jC("CHM237H1")},
		// not a grade: "with a minimum of 70% in" is a prefix for what follows
		{"DRM220Y1 with a minimum of 70% in DRM300Y1", tCourse, "DRM220Y1", jC("DRM220Y1")},

		{"0.5 credit in BR=1/ 2/ 3", tCredits, "0.5 credit in BR=1/ 2/ 3", `{"type":"credits","amount":0.5,"breadth":[1,2,3]}`},
		{"no more than 14.0 credits, including", tCredits, "no more than 14.0 credits", `{"type":"credits","max":14}`},
		{"a full course in PHL/CSC/MAT", tCredits, "a full course in PHL/CSC/MAT", `{"type":"credits","amount":1,"designators":["PHL","CSC","MAT"]}`},
		{"a half-course in HIS", tCredits, "a half-course in HIS", `{"type":"credits","amount":0.5,"designators":["HIS"]}`},
		{"at least two other 0.5 credit courses in Jewish Studies", tCredits, "at least two other 0.5 credit courses in Jewish Studies",
			`{"type":"credits","amount":1,"designators":["CJS"]}`},

		{"A student must be in third or fourth year.", tCredits, "A student must be in third or fourth year", `{"type":"credits","amount":9}`},
		{"Must be enrolled in 4th year", tCredits, "Must be enrolled in 4th year", `{"type":"credits","amount":14}`},

		{"Cumulative GPA of at least 2.70", tGPA, "Cumulative GPA of at least 2.70", `{"type":"gpa","min":2.7}`},
		{"a GPA of at least 3.5 in CSE courses", tGPA, "a GPA of at least 3.5 in CSE courses", `{"type":"gpa","min":3.5,"scope":"CSE courses"}`},
		{"3.0 GPA in Political Science courses, supervisor", tGPA, "3.0 GPA in Political Science courses",
			`{"type":"gpa","min":3,"scope":"Political Science courses"}`},
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

func TestGradeSuffixMarkers(t *testing.T) {
	tests := []struct {
		in       string
		want     token
		consumed string
	}{
		{"with a minimum grade of 68% in each course.", token{kind: tGradeSuffix, grade: 68, each: true}, "with a minimum grade of 68% in each course"},
		{"with a minimum grade of 63%, ( MAT130H1", token{kind: tGradeSuffix, grade: 63}, "with a minimum grade of 63%"},
		{"(minimum 70%)/ EAS401H1", token{kind: tGradeSuffix, grade: 70}, "(minimum 70%)"},
		{"(B or higher)", token{kind: tGradeSuffix, grade: 73}, "(B or higher)"},
	}
	for _, tt := range tests {
		tok, ok := firstPhrase(tt.in)
		tt.want.end = len(tt.consumed)
		if !ok || !reflect.DeepEqual(tok, tt.want) {
			t.Errorf("%q: got %+v (ok=%v), want %+v", tt.in, tok, ok, tt.want)
		}
	}
}

func TestPhraseNonMatches2(t *testing.T) {
	for _, s := range []string{
		"with a minimum of 70% in DRM220Y1", // a grade prefix, not a suffix
		"with a B+ average",                 // an average, not a grade
		"(A)",                               // a letter alone isn't a grade
		"with a minimum mark of 70.5",       // not a whole-number grade
		"in first year biology",             // not year standing
		"in the final year of registration", // no year number
		"200-level courses",                 // how many?
		"Two HPS courses",                   // course count, not credits
		"no more than two courses",          // no amount
	} {
		if tok, ok := firstPhrase(s); ok {
			t.Errorf("%q: unexpectedly matched %+v", s, tok)
		}
	}
}
