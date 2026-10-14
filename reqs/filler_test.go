package reqs

import (
	"testing"

	"degree-explorer/types"
)

// fillerGoldens: connective wording that used to survive as text nodes and
// make otherwise structured requirements always need review.
var fillerGoldens = []struct{ name, raw, want string }{
	// filler-only text nodes
	{"trailing completed", "8.0 credits completed.", `{"type":"credits","amount":8}`},
	{"completed before including", "9.0 credits completed including  RSM250H1.",
		jAnd(`{"type":"credits","amount":9}`, jC("RSM250H1"))},
	{"must have completed", "EEB498Y1 and must have completed 14.0 credits.",
		jAnd(jC("EEB498Y1"), `{"type":"credits","amount":14}`)},
	{"with between credit phrases", "7.5 credits, with at least 1.5 PHL credits including  PHL237H1",
		jAnd(`{"type":"credits","amount":7.5}`, `{"type":"credits","amount":1.5,"designators":["PHL"]}`, jC("PHL237H1"))},
	{"minimum of before credits", "Minimum of 0.5 WGS credits or permission from department",
		jOr(`{"type":"credits","amount":0.5,"designators":["WGS"]}`, jT("permission", "permission from department"))},
	{"a combined minimum of", "A combined minimum of 1.0 credit from POL and/or SDS courses",
		`{"type":"credits","amount":1,"designators":["POL","SDS"]}`},
	{"any other", "SPA202H1/  PRT205H1 or any other 0.5 credit at 200+ level in ITA, FRE, FSL",
		jOr(jC("SPA202H1"), jC("PRT205H1"), `{"type":"credits","amount":0.5,"designators":["ITA","FRE","FSL"],"levels":[200,300,400]}`)},
	{"open to students who have lead-in", "English 200-level courses are open to students who have obtained standing in 1.0 ENG credit or in any 4.0 credits.",
		jOr(`{"type":"credits","amount":1,"designators":["ENG"]}`, `{"type":"credits","amount":4}`)},

	// "Students must have completed X towards their degree ... to be eligible for ..."
	{"SOC390Y1-style eligibility", "Students must have completed 10.0 credits towards their degree, which includes a 0.5 credit SOC course at the 200+ level, to be eligible for 300-level Independent Research.",
		jAnd(`{"type":"credits","amount":10}`, `{"type":"credits","amount":0.5,"designators":["SOC"],"levels":[200,300,400]}`)},

	// "of which N must be ...": the copula goes, the qualifiers stay on the credit
	{"of which credits must be", "10.0 credits, of which 2.0 credits must be 300+ level ESS/ JEG/ JGA/ JPE courses",
		jAnd(`{"type":"credits","amount":10}`, `{"type":"credits","amount":2,"designators":["ESS","JEG","JGA","JPE"],"levels":[300,400]}`)},
	{"of which credit is from", "6.0 credits, of which 1.0 credit is from EEB/ BIO/ HPS courses",
		jAnd(`{"type":"credits","amount":6}`, `{"type":"credits","amount":1,"designators":["EEB","BIO","HPS"]}`)},
	{"of which designator credit must be at level", "2.0 LIN credits at the 200+ level, of which 1.0 LIN credit must be at the 300+ level",
		jAnd(`{"type":"credits","amount":2,"designators":["LIN"],"levels":[200,300,400]}`, `{"type":"credits","amount":1,"designators":["LIN"],"levels":[300,400]}`)},
	{"N of which must be", "14.5 credits, 5.0 of which must be GGR/URB",
		jAnd(`{"type":"credits","amount":14.5}`, `{"type":"credits","amount":5,"designators":["GGR","URB"]}`)},
	{"of which N should be an", "4.0 credits, of which 0.5 should be an HPS credit.",
		jAnd(`{"type":"credits","amount":4}`, `{"type":"credits","amount":0.5,"designators":["HPS"]}`)},

	// campus tags and session letters on codes
	{"campus tag in brackets", "BCH210H1/( BIOC12H3 and  BIOC13H3 [UTSC])",
		jOr(jC("BCH210H1"), jAnd(jC("BIOC12H3"), jC("BIOC13H3")))},
	{"campus tag glued to code", "BCH242Y1/ [ CHM361H5(UTM) and CHM362H5(UTM)]/ CHMB62H3(UTSC)",
		jOr(jC("BCH242Y1"), jAnd(jC("CHM361H5"), jC("CHM362H5")), jC("CHMB62H3"))},
	{"session letter", "WGS160Y1Y or  WGS271Y1Y or 1.0 WGS credit",
		jOr(jC("WGS160Y1"), jC("WGS271Y1"), `{"type":"credits","amount":1,"designators":["WGS"]}`)},

	// high-school and conservatory preparation
	{"high-school code after subject", "Physics SPH3U", jT("preparation", "Physics SPH3U")},
	{"high-school codes", "Chemistry SCH4U, Mathematics MHF4U + MCV4U",
		jAnd(jT("preparation", "Chemistry SCH4U"), jT("preparation", "Mathematics MHF4U + MCV4U"))},
	{"RCM levels", "RCM Level 8 Theory, RCM Level 9 Harmony, RCM Grade 8 level performing audition",
		jAnd(jT("preparation", "RCM Level 8 Theory"), jT("preparation", "RCM Level 9 Harmony"), jT("permission", "RCM Grade 8 level performing audition"))},
}

func TestFillerGolden(t *testing.T) {
	for _, g := range fillerGoldens {
		t.Run(g.name, func(t *testing.T) {
			assertJSON(t, ParseRequisite(g.raw).Req, g.want)
		})
	}
}

// Advice and pointers end up in notes, and the requirement keeps the rest.
func TestFillerEnvelope(t *testing.T) {
	for _, tt := range []struct{ raw, want string }{
		// A field that only points elsewhere has unlisted requirements: Review.
		{"See the Department of Economics website for details.",
			`{"req":` + jT("other", "See the Department of Economics website for details") + `}`},
		{"Consult the Rotman Commerce website for details.",
			`{"req":` + jT("other", "Consult the Rotman Commerce website for details") + `}`},
		{"None", `{"req":null,"notes":["None"]}`},
		{"NA", `{"req":null,"notes":["NA"]}`},
		{"1.0 MAT credit at the 100-level. Possible additional topic-specific prerequisites.",
			`{"req":{"type":"credits","amount":1,"designators":["MAT"],"levels":[100]},"notes":["Possible additional topic-specific prerequisites"]}`},
		{"9.0 credits and an application. See http://www.example.ca/program for details.",
			`{"req":` + jAnd(`{"type":"credits","amount":9}`, jT("permission", "an application")) + `,"notes":["See for details","http://www.example.ca/program"]}`},
		{"Permission of the Human Biology Program; please refer to the Human Biology Program website for more details",
			`{"req":` + jT("permission", "Permission of the Human Biology Program") + `,"notes":["please refer to the Human Biology Program website for more details"]}`},
		{"CSC108H1/  CSC110Y1/  CSC148H1 *Note: the corequisite may be completed either concurrently or in advance.",
			`{"req":` + jOr(jC("CSC108H1"), jC("CSC110Y1"), jC("CSC148H1")) + `,"notes":["Note: the corequisite may be completed either concurrently or in advance"]}`},
		{"MAT237Y1/  MAT257Y1 ( MAT257Y1 can be taken concurrently)",
			`{"req":` + jOr(jC("MAT237Y1"), `{"type":"course","code":"MAT257Y1","concurrent":true}`) + `,"notes":["MAT257Y1 can be taken concurrently"]}`},
		{"BCH210H1/ BCH242Y1. This is a problem-solving course and will require use of skills from introductory physics.",
			`{"req":` + jOr(jC("BCH210H1"), jC("BCH242Y1")) + `,"notes":["This is a problem-solving course and will require use of skills from introductory physics"]}`},
		{"URB235H1,  URB236H1. However, consideration may be given to students with suitable course background.",
			`{"req":` + jAnd(jC("URB235H1"), jC("URB236H1")) + `,"notes":["However, consideration may be given to students with suitable course background"]}`},
		{"Placement test. Research positions are limited.",
			`{"req":` + jT("placement", "Placement test") + `,"notes":["Research positions are limited"]}`},
		{"10.0 credits. Application deadlines: August 10th for September start date; November 10th for January start date.",
			`{"req":{"type":"credits","amount":10},"notes":["Application deadlines: August 10th for September start date","November 10th for January start date"]}`},
		{"Students must have completed 15.0 credits towards their degree to be eligible for 400-level Independent Research.",
			`{"req":{"type":"credits","amount":15},"notes":["to be eligible for 400-level Independent Research"]}`},
	} {
		r := ParseRequisite(tt.raw)
		assertJSON(t, struct {
			Req   *types.ReqNode `json:"req"`
			Notes []string       `json:"notes,omitempty"`
		}{r.Req, r.Notes}, tt.want)
	}
}

func TestIsFiller(t *testing.T) {
	for s, want := range map[string]bool{
		"completed":             true,
		"Students must have":    true,
		"have taken":            true,
		"must be":               true,
		"with":                  true,
		"of which":              true,
		"Of these":              true,
		"at least a":            true,
		"Completion minimum of": true,
		"towards their degree":  true,
		"UTSC":                  true,
		// real conditions stay text
		"with lab":     false,
		"in Society":   false,
		"no more than": false,
		"Students cannot have completed more than": false,
		"must be at the 300+ level":                false,
		"with at least a B+ average":               false,
	} {
		if got := isFiller(s); got != want {
			t.Errorf("isFiller(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestIsPointer(t *testing.T) {
	for s, want := range map[string]bool{
		"See the Department's website for POL courses by area group":                                true,
		"Details related to the timetable and deadlines are posted on the Summer Abroad website at": true,
		"Possible additional topic-specific prerequisites":                                          true,
		"None": true,
		// a pointer that also states a requirement stays a requirement
		"Students must submit an application, see the website for details":    false,
		"To be specified annually on the course website":                      false,
		"This is a program-only course and is restricted to sociology majors": false,
	} {
		if got := isPointer(s); got != want {
			t.Errorf("isPointer(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestStripFillerKeepsMeaning(t *testing.T) {
	for in, want := range map[string]string{
		// "who have completed" belongs to a larger phrase; leave it for the year rules
		"Available to students in their third year of study (who have completed at least 9.0 credits)": "Available to students in their third year of study (who have completed at least 9.0 credits)",
		// breadth phrases have their own matcher
		"4.0 credits, at least 1.0 of which should be in the HUM/BR=1/2": "4.0 credits, at least 1.0 of which should be in the HUM/BR=1/2",
		"Students cannot have completed more than 8.5 credits":           "Students cannot have completed more than 8.5 credits",
		"and have taken at least 10.0 credits":                           "and completed at least 10.0 credits",
	} {
		var notes []string
		if got := stripFiller(in, &notes); got != want || notes != nil {
			t.Errorf("stripFiller(%q) = %q %q, want %q", in, got, notes, want)
		}
	}
}

func TestClassifyPreparation(t *testing.T) {
	for _, s := range []string{"SPH4U Physics", "MCV4U Calculus & Vectors", "SBI4U", "RCM Level 9 Harmony", "U or M level"} {
		if got := classify(s); got != types.KindPreparation {
			t.Errorf("classify(%q) = %q, want preparation", s, got)
		}
	}
}
