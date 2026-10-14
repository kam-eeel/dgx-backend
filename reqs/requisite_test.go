package reqs

import (
	"testing"

	"degree-explorer/types"
)

// reqGoldens: raw catalogue text -> expected JSON of Requisite.Req.
// Add corresponding rows here after fixing parser misses.
var reqGoldens = []struct{ name, raw, want string }{
	// course expressions
	{"zero-width space", "ECO220Y1/\u200b  ECO227Y1", jOr(jC("ECO220Y1"), jC("ECO227Y1"))},
	{"square brackets", "MAT257Y1/ [ MAT224H1/  MAT247H1,  MAT237Y1]",
		jOr(jC("MAT257Y1"), jAnd(jOr(jC("MAT224H1"), jC("MAT247H1")), jC("MAT237Y1")))},
	{"footnote asterisk", "EEB225H1/  GGR270H1*/  STA220H1",
		jOr(jC("EEB225H1"), jC("GGR270H1"), jC("STA220H1"))},
	{"STA347H1 abridged", "STA247H1(70%)/  STA255H1(70%)/  STA257H1;  MAT223H1/  MAT240H1",
		jAnd(jOr(jCG("STA247H1", 70), jCG("STA255H1", 70), jC("STA257H1")), jOr(jC("MAT223H1"), jC("MAT240H1")))},
	{"ACT245H1", "ACT240H1, ( MAT130H1/  MAT135H1,  MAT136H1)/ ( MAT148H1,  MAT149H1)/  MAT137Y1/ ( MAT158H1,  MAT159H1)/  MAT157Y1",
		jAnd(jC("ACT240H1"), jOr(
			jAnd(jOr(jC("MAT130H1"), jC("MAT135H1")), jC("MAT136H1")),
			jAnd(jC("MAT148H1"), jC("MAT149H1")), jC("MAT137Y1"),
			jAnd(jC("MAT158H1"), jC("MAT159H1")), jC("MAT157Y1")))},
	{"dedupe", "CSC108H1/ CSC108H1", jC("CSC108H1")},

	// text
	{"CSC148H1", "CSC108H1/ (equivalent programming experience)",
		jOr(jC("CSC108H1"), jT("preparation", "equivalent programming experience"))},
	{"ANT345H1", "ANT205H1 or  ANT207H1 or permission of the instructor",
		jOr(jC("ANT205H1"), jC("ANT207H1"), jT("permission", "permission of the instructor"))},
	{"slash inside text merges", "EST200Y1 or permission of the instructor/program administrator",
		jOr(jC("EST200Y1"), jT("permission", "permission of the instructor/program administrator"))},
	{"comma keeps texts apart", "Admission to Munk One, permission of the instructor",
		jAnd(jT("program", "Admission to Munk One"), jT("permission", "permission of the instructor"))},
	{"FSL220H1 text swallows mentioned course",
		"FSL121Y1/ FSL122H1 or placement in  FSL220H1 based on the results of the French Placement Test.",
		jOr(jC("FSL121Y1"), jC("FSL122H1"),
			jT("placement", "placement in FSL220H1 based on the results of the French Placement Test"))},
	{"Vic One trailing colon", "Admission to Vic One: https://vic.utoronto.ca/future-students/vic-one/",
		jT("program", "Admission to Vic One")},
	{"high school", "High school math", jT("preparation", "High school math")},

	// credits
	{"completion of", "Completion of 9.0 credits", `{"type":"credits","amount":9}`},
	{"POL350H1", "1.0 credit in POL/ JPA/ JPF/ JPI/ JPR/ JPS/ JRA",
		`{"type":"credits","amount":1,"designators":["POL","JPA","JPF","JPI","JPR","JPS","JRA"]}`},
	{"EUR301H1", "1.0 HIS or POL credit at the 100-level. Students who do not meet the requirement are encouraged to contact the department.",
		`{"type":"credits","amount":1,"designators":["HIS","POL"],"levels":[100]}`},
	{"ENG384H1 span stops at and", "2.0 ENG credits and any 4.0 credits",
		jAnd(`{"type":"credits","amount":2,"designators":["ENG"]}`, `{"type":"credits","amount":4}`)},
	{"INS322H1", "8.0 credits including  INS201Y1/ ( INS201H1,  INS202H1), plus 1.0 additional INS credit",
		jAnd(`{"type":"credits","amount":8}`,
			jOr(jC("INS201Y1"), jAnd(jC("INS201H1"), jC("INS202H1"))),
			`{"type":"credits","amount":1,"designators":["INS"]}`)},
	{"ENG487H1", "4.0 ENG credits (including  ENG202H1,  ENG203H1,  ENG250H1,  ENG252H1) and any 9.0 credits",
		jAnd(`{"type":"credits","amount":4,"designators":["ENG"]}`,
			jC("ENG202H1"), jC("ENG203H1"), jC("ENG250H1"), jC("ENG252H1"),
			`{"type":"credits","amount":9}`)},
	{"PHL378H1", "PHL265H1/ PHL275H1, 7.5 courses (in any field) with at least 1.5 in philosophy",
		jAnd(jOr(jC("PHL265H1"), jC("PHL275H1")), `{"type":"credits","amount":7.5}`,
			jT("other", "with at least 1.5 in philosophy"))},

	// any-course and year standing
	{"ANT456H1", "ANT207H1 and any 300-level course in Society",
		jAnd(jC("ANT207H1"), `{"type":"credits","amount":0.5,"levels":[300]}`, jT("other", "in Society"))},
	{"EUR495H1", "Open only to students enrolled in the European Studies Major Program. Third and Fourth year students only.",
		jAnd(jT("program", "Open only to students enrolled in the European Studies Major Program"), `{"type":"credits","amount":9}`)},
	{"NMC499Y1", "4th year status in NMC Specialist program",
		jAnd(`{"type":"credits","amount":14}`, jT("program", "in NMC Specialist program"))},

	// GPA
	{"APM396H1", "Minimum GPA 3.5 for APM and MAT courses, permission of the Associate Chair for Undergraduate Studies and of the prospective supervisor.",
		jAnd(`{"type":"gpa","min":3.5,"scope":"APM and MAT courses"}`,
			jT("permission", "permission of the Associate Chair for Undergraduate Studies and of the prospective supervisor"))},
	{"AST431H1", "Minimum GPA of 4.0 in Astronomy program courses or with special permission from the Undergraduate Chair",
		jOr(`{"type":"gpa","min":4,"scope":"Astronomy program courses"}`,
			jT("permission", "with special permission from the Undergraduate Chair"))},
	{"cGPA no scope", "BIO130H1, cGPA 3.0", jAnd(jC("BIO130H1"), `{"type":"gpa","min":3}`)},

	// grade prefixes
	{"BCH372Y1", "A final grade of 75% or higher in  BCH242Y1", jCG("BCH242Y1", 75)},
	{"BCH373H1", "(75% or higher in  BCH242Y1)/(80% or higher in  BCH210H1)",
		jOr(jCG("BCH242Y1", 75), jCG("BCH210H1", 80))},
	{"BCH470H1", "( BCH340H1;  BCH377H1;  BCH378H1; 75% or higher in  MGY311Y1)/( BCH370H1; 80% or higher in  BCH311H1)",
		jOr(jAnd(jC("BCH340H1"), jC("BCH377H1"), jC("BCH378H1"), jCG("MGY311Y1", 75)),
			jAnd(jC("BCH370H1"), jCG("BCH311H1", 80)))},
	{"each of", "A final grade of 75% or higher in each of  BCH340H1,  BCH377H1",
		jAnd(jCG("BCH340H1", 75), jCG("BCH377H1", 75))},

	// pools and breadth
	{"HIS452H1", "9.0 credits including one of  HIS241H1,  HIS242H1,  HIS243H1,  HIS244H1, and a minimum 1.0 HIS credit",
		jAnd(`{"type":"credits","amount":9}`,
			jOr(jC("HIS241H1"), jC("HIS242H1"), jC("HIS243H1"), jC("HIS244H1")),
			`{"type":"credits","amount":1,"designators":["HIS"]}`)},
	{"FOR413H1", "1.0 credit from:  FOR300H1,  FOR301H1,  FOR302H1 or with permission of the instructor/program administrator",
		jOr(`{"type":"credits","amount":1,"courses":["FOR300H1","FOR301H1","FOR302H1"]}`,
			jT("permission", "with permission of the instructor/program administrator"))},
	{"pool with group", "8.0 credits including one of  INS200H1,  INS201Y1/ ( INS201H1,  INS202H1),  INS250H1",
		jAnd(`{"type":"credits","amount":8}`,
			jOr(jC("INS200H1"), jC("INS201Y1"), jAnd(jC("INS201H1"), jC("INS202H1")), jC("INS250H1")))},
	{"CIN336H1", "CIN105Y1, or 1.0 credit from:  SDS255H1,  SDS256H1,  SDS279H1,  SDS355H1",
		jOr(jC("CIN105Y1"), `{"type":"credits","amount":1,"courses":["SDS255H1","SDS256H1","SDS279H1","SDS355H1"]}`)},
	{"EEB390H1", "BIO220H1; at least one of  EEB263H1,  EEB267H1,  EEB362H1",
		jAnd(jC("BIO220H1"), jOr(jC("EEB263H1"), jC("EEB267H1"), jC("EEB362H1")))},
	{"HPS372H1 of-which pool", "4.0 credits, of which at least 0.5 credits in  HPS100H1,  HPS200H1, or  PHL255H1",
		jAnd(`{"type":"credits","amount":4}`, `{"type":"credits","amount":0.5,"courses":["HPS100H1","HPS200H1","PHL255H1"]}`)},
	{"NEW302Y1 breadth", "4.0 credits, at least 1.0 of which should be in the HUM/BR=1/2",
		jAnd(`{"type":"credits","amount":4}`, `{"type":"credits","amount":1,"breadth":[1,2]}`)},

	// wording variants found by the corpus report
	{"CHM338H1 plus sign", "CHM236H1 +  CHM237H1 with a minimum grade of 68% in each course.",
		jAnd(jC("CHM236H1"), jCG("CHM237H1", 68))},
	{"or greater", "75% or greater in  CSC336H1/ 75% or greater in  CSC338H5/ equivalent mathematical background",
		jOr(jCG("CSC336H1", 75), jCG("CSC338H5", 75), jT("preparation", "equivalent mathematical background"))},
	{"minimum of N% in", "(minimum of 77% in  MAT130H1/  MAT135H1 and  MAT136H1)",
		jAnd(jOr(jCG("MAT130H1", 77), jC("MAT135H1")), jC("MAT136H1"))},
	{"is required in", "A minimum grade of 75% is required in  PSL300H1", jCG("PSL300H1", 75)},
	{"COG344H1 one of either", "COG250Y1 and one of either  LIN232H1/ LIN241H1 or  JLP315H1/ JLP374H1.",
		jAnd(jC("COG250Y1"), jOr(jC("LIN232H1"), jC("LIN241H1"), jC("JLP315H1"), jC("JLP374H1")))},
	{"NMC372H1 any of", "Any of  NMC103H1/  NMC273Y1/  NMC278H1",
		jOr(jC("NMC103H1"), jC("NMC273Y1"), jC("NMC278H1"))},
	{"HIS317H1 one of the following is required", "One of the following is required:  HIS103Y1/  HIS109Y1/ ( HIS241H1,  HIS242H1)/  EUR200Y1",
		jOr(jC("HIS103Y1"), jC("HIS109Y1"), jAnd(jC("HIS241H1"), jC("HIS242H1")), jC("EUR200Y1"))},
	{"SOC338H1 completion of", "Completion of  SOC201H1,  SOC202H1,  SOC204H1 plus 1.0 credit from:  SOC251H1,  SOC252H1,  SOC254H1",
		jAnd(jC("SOC201H1"), jC("SOC202H1"), jC("SOC204H1"),
			`{"type":"credits","amount":1,"courses":["SOC251H1","SOC252H1","SOC254H1"]}`)},
	{"ESS363H1 pool with OR group", "1.0 credit from:  PHY131H1/  PHY132H1/ ( MAT130H1/  MAT135H1)/  MAT136H1",
		`{"type":"credits","amount":1,"courses":["PHY131H1","PHY132H1","MAT130H1","MAT135H1","MAT136H1"]}`},

	// audience conditionals
	{"BCH374Y1", "For Specialist:  BCH242Y1 (75% or higher);  BIO230H1;  CHM247H1/ CHM249H1. For Major:  BCH210H1 (80% or higher);  BIO230H1;  CHM247H1/ CHM249H1.",
		jOr(
			jAnd(jT("program", "For Specialist"), jCG("BCH242Y1", 75), jC("BIO230H1"), jOr(jC("CHM247H1"), jC("CHM249H1"))),
			jAnd(jT("program", "For Major"), jCG("BCH210H1", 80), jC("BIO230H1"), jOr(jC("CHM247H1"), jC("CHM249H1"))))},
	{"ANA411H1", "For KPE students:  ANA124H1,  ANA125H1, and  ANA126Y1 For Arts & Science students:  ANA300Y1 (78%) or equivalent",
		jOr(
			jAnd(jT("program", "For KPE students"), jC("ANA124H1"), jC("ANA125H1"), jC("ANA126Y1")),
			jAnd(jT("program", "For Arts & Science students"), jOr(jCG("ANA300Y1", 78), jT("preparation", "equivalent"))))},
	{"RSM433H1 capitalized labels", "Rotman Commerce:  RSM333H1; Actuarial Science:  ACT349H1",
		jOr(jAnd(jT("program", "Rotman Commerce"), jC("RSM333H1")), jAnd(jT("program", "Actuarial Science"), jC("ACT349H1")))},
	{"CSC311H1-style engineering branch", "CSC207H1/  CSCB07H3;  MAT223H1/  MAT240H1 Prerequisite for Faculty of Applied Science and Engineering students: APS105H1/ ESC180H1; MAT185H1",
		jOr(
			jAnd(jOr(jC("CSC207H1"), jC("CSCB07H3")), jOr(jC("MAT223H1"), jC("MAT240H1"))),
			jAnd(jT("program", "Engineering students"), jOr(jC("APS105H1"), jC("ESC180H1")), jC("MAT185H1")))},
}

func TestParseRequisiteGolden(t *testing.T) {
	for _, g := range reqGoldens {
		t.Run(g.name, func(t *testing.T) {
			assertJSON(t, ParseRequisite(g.raw).Req, g.want)
		})
	}
}

func TestParseRequisiteEnvelope(t *testing.T) {
	raw := "ANT380H1. Students who do not meet the prerequisite are encouraged to contact the Department."
	assertJSON(t, ParseRequisite(raw), `{"raw":`+jsonString(raw)+`,"req":`+jC("ANT380H1")+
		`,"notes":["Students who do not meet the prerequisite are encouraged to contact the Department"]}`)

	raw = "MAT130H1/  MAT135H1 recommended"
	assertJSON(t, ParseRequisite(raw), `{"raw":`+jsonString(raw)+`,"req":null,"notes":["MAT130H1/ MAT135H1 recommended"]}`)

	raw = "Admission to Vic One: https://vic.utoronto.ca/future-students/vic-one/"
	assertJSON(t, ParseRequisite(raw).Notes, `["https://vic.utoronto.ca/future-students/vic-one/"]`)

	raw = "CSC108H1"
	assertJSON(t, ParseRequisite(raw), `{"raw":"CSC108H1","req":`+jC("CSC108H1")+`}`)
}

func TestClassify(t *testing.T) {
	for text, want := range map[string]types.TextKind{
		"permission of the instructor": types.KindPermission,
		"by ballot":                    types.KindPermission,
		"Enrolment in the Actuarial Science Specialist program": types.KindProgram,
		"Admission to Munk One":                                 types.KindProgram,
		"Active in ASMAJ1618":                                   types.KindProgram,
		"French Placement Test":                                 types.KindPlacement,
		"No previous knowledge of French":                       types.KindPlacement,
		"High school math":                                      types.KindPreparation,
		"equivalent programming experience":                     types.KindPreparation,
		"in Society":                                            types.KindOther,
	} {
		if got := classify(text); got != want {
			t.Errorf("classify(%q) = %q, want %q", text, got, want)
		}
	}
}
