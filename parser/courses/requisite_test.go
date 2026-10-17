package courses

import (
	"testing"

	"degree-explorer/parser/grammar"
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
		jAnd(jT("membership", "Admission to Munk One"), jT("permission", "permission of the instructor"))},
	{"FSL220H1 text swallows mentioned course",
		"FSL121Y1/ FSL122H1 or placement in  FSL220H1 based on the results of the French Placement Test.",
		jOr(jC("FSL121Y1"), jC("FSL122H1"),
			jT("placement", "placement in FSL220H1 based on the results of the French Placement Test"))},
	{"Vic One trailing colon", "Admission to Vic One: https://vic.utoronto.ca/future-students/vic-one/",
		jT("membership", "Admission to Vic One")},
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
			`{"type":"credits","amount":1.5,"designators":["PHL"]}`)},

	// any-course and year standing
	{"ANT456H1", "ANT207H1 and any 300-level course in Society",
		jAnd(jC("ANT207H1"), `{"type":"credits","amount":0.5,"levels":[300]}`, jT("other", "in Society"))},
	{"EUR495H1", "Open only to students enrolled in the European Studies Major Program. Third and Fourth year students only.",
		jAnd(jP("Open only to students enrolled in the European Studies Major Program"), `{"type":"credits","amount":9}`)}, // names resolved later by Link
	{"NMC499Y1", "4th year status in NMC Specialist program",
		jAnd(`{"type":"credits","amount":14}`, jP("in NMC Specialist program"))},

	// GPA
	{"APM396H1", "Minimum GPA 3.5 for APM and MAT courses, permission of the Associate Chair for Undergraduate Studies and of the prospective supervisor.",
		jAnd(`{"type":"gpa","min_gpa":3.5,"scope":"APM and MAT courses"}`,
			jT("permission", "permission of the Associate Chair for Undergraduate Studies and of the prospective supervisor"))},
	{"AST431H1", "Minimum GPA of 4.0 in Astronomy program courses or with special permission from the Undergraduate Chair",
		jOr(`{"type":"gpa","min_gpa":4,"scope":"Astronomy program courses"}`,
			jT("permission", "with special permission from the Undergraduate Chair"))},
	{"cGPA no scope", "BIO130H1, cGPA 3.0", jAnd(jC("BIO130H1"), `{"type":"gpa","min_gpa":3}`)},

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
		jAnd(jCG("CHM236H1", 68), jCG("CHM237H1", 68))},
	{"or greater", "75% or greater in  CSC336H1/ 75% or greater in  CSC338H5/ equivalent mathematical background",
		jOr(jCG("CSC336H1", 75), jCG("CSC338H5", 75), jT("preparation", "equivalent mathematical background"))},
	{"minimum of N% in", "(minimum of 77% in  MAT130H1/  MAT135H1 and  MAT136H1)",
		jAnd(jOr(jCG("MAT130H1", 77), jCG("MAT135H1", 77)), jC("MAT136H1"))},
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

	// "two of": choose nodes
	{"AST425Y1 two of, commas", "AST320H1, two of  AST325H1/  AST326Y1,  PHY324H1,  PHY407H1/  PHY408H1,  JPE395H1",
		jAnd(jC("AST320H1"), jChoose(2, jOr(jC("AST325H1"), jC("AST326Y1")), jC("PHY324H1"), jOr(jC("PHY407H1"), jC("PHY408H1")), jC("JPE395H1")))},
	{"CSB427H1 two of, slashes", "BIO260H1/ HMB265H1, two of  CSB328H1/ CSB331H1/ CSB349H1/ MGY350H1",
		jAnd(jOr(jC("BIO260H1"), jC("HMB265H1")), jChoose(2, jC("CSB328H1"), jC("CSB331H1"), jC("CSB349H1"), jC("MGY350H1")))},
	{"HIS341Y1 two DESIG courses from the following", "Two HIS courses from the following:  HIS102Y1/ HIS103Y1/ EUR200Y1",
		jChoose(2, jC("HIS102Y1"), jC("HIS103Y1"), jC("EUR200Y1"))},
	{"PHL480H1 course range", "Two of  PHL344H1 -  PHL347H1 and permission of the instructor and Department (by ballot)",
		jAnd(jChoose(2, jC("PHL344H1"), jC("PHL345H1"), jC("PHL346H1"), jC("PHL347H1")),
			jT("permission", "permission of the instructor and Department (by ballot"))},
	{"ENV337H1 two of inside a group", "(Two of  CHM210H1,  EEB240H1/  ENV234H1,  ESS262H1) or ( ENV221H1/  ENV222H1 +  BIO220H1)",
		jOr(jChoose(2, jC("CHM210H1"), jOr(jC("EEB240H1"), jC("ENV234H1")), jC("ESS262H1")),
			jAnd(jOr(jC("ENV221H1"), jC("ENV222H1")), jC("BIO220H1")))},
	{"two of two means both", "two of  CSB328H1/ CSB331H1", jAnd(jC("CSB328H1"), jC("CSB331H1"))},

	// credit pool wordings
	{"FRE410H1 descriptive words before colon", "FRE246H1, 1.0 credit from 300-level French literature courses:  FRE304H1,  FRE310H1,  FRE314H1.",
		jAnd(jC("FRE246H1"), `{"type":"credits","amount":1,"courses":["FRE304H1","FRE310H1","FRE314H1"]}`)},
	{"URB235H1 any of the following courses", "Completion of 4.0 credits, including 1.0 credit from any of the following courses:  ECO101H1,  ECO102H1,  GGR107H1",
		jAnd(`{"type":"credits","amount":4}`, `{"type":"credits","amount":1,"courses":["ECO101H1","ECO102H1","GGR107H1"]}`)},
	{"HIS493H1 designator before credits", "9.0 credits including 2.0 HIS credits from the following:  HIS240H1/  HIS241H1/  EUR200Y1",
		jAnd(`{"type":"credits","amount":9}`, `{"type":"credits","amount":2,"courses":["HIS240H1","HIS241H1","EUR200Y1"]}`)},
	{"HIS352H1 the following without colon", "1.0 credit from the following  AFR150Y1/  HIS295Y1/  HIS297Y1",
		`{"type":"credits","amount":1,"courses":["AFR150Y1","HIS295Y1","HIS297Y1"]}`},
	{"HIS414H1 pool joined by or", "9.0 credits including 1.0 HIS credit from  HIS219Y1 or  HIS220Y1",
		jAnd(`{"type":"credits","amount":9}`, `{"type":"credits","amount":1,"courses":["HIS219Y1","HIS220Y1"]}`)},
	{"ENV342H1 credits in a list of courses", "Completion of 9.0 credits, including 0.5 credits in  ENV200H1/  ENV221H1/  PSY100H1",
		jAnd(`{"type":"credits","amount":9}`, `{"type":"credits","amount":0.5,"courses":["ENV200H1","ENV221H1","PSY100H1"]}`)},
	{"PHL377H1 in any field, with at least N in DESIG, N from", "9.0 credits in any field, with at least 1.5 in PHL including 0.5 from  PHL273H1/  PHL275H1",
		jAnd(`{"type":"credits","amount":9}`, `{"type":"credits","amount":1.5,"designators":["PHL"]}`,
			`{"type":"credits","amount":0.5,"courses":["PHL273H1","PHL275H1"]}`)},
	{"FOR400Y1 at least N credit from, then ; or", "FOR200H1 or  FOR201H1 and at least 1.0 credit from:  FOR300H1/  FOR301H1; or with permission of the instructor or program administrator",
		jOr(jAnd(jOr(jC("FOR200H1"), jC("FOR201H1")), `{"type":"credits","amount":1,"courses":["FOR300H1","FOR301H1"]}`),
			jT("permission", "with permission of the instructor or program administrator"))},
	{"HIS457H1 of which at least one course", "9.0 credits including 2.0 HIS credits, of which at least one course needs to be   HIS243H1/  HIS244H1",
		jAnd(`{"type":"credits","amount":9}`, `{"type":"credits","amount":2,"designators":["HIS"]}`, jOr(jC("HIS243H1"), jC("HIS244H1")))},
	{"HMB406H1 additional credit from, trailing or-text", "14.0 credits including 0.5 additional credit from  HMB306H1,  PHL281H1, or another ethics course with prior approval from the unit",
		jAnd(`{"type":"credits","amount":14}`,
			jOr(`{"type":"credits","amount":0.5,"courses":["HMB306H1","PHL281H1"]}`, jT("permission", "another ethics course with prior approval from the unit")))},
	{"EEB321H1 a course in statistics from", "BIO220H1 and a course in statistics from  EEB225H1/  GGR270H1/  STA220H1",
		jAnd(jC("BIO220H1"), jOr(jC("EEB225H1"), jC("GGR270H1"), jC("STA220H1")))},
	{"ITA453H1 designator item ends list", "At least 1.0 credit from  ITA240Y1,  ITA340H1, or CIN courses, ( ITA350Y1/ ITA351Y1)",
		jAnd(`{"type":"credits","amount":1,"designators":["CIN"],"courses":["ITA240Y1","ITA340H1"]}`, jOr(jC("ITA350Y1"), jC("ITA351Y1")))},
	{"PSY342H1 repeated , or keeps the list going", "PSY201H1, and one of  PSY230H1/  PSYB30H3, or  PSY240H1/  PSYB32H3, or  PSY270H1",
		jAnd(jC("PSY201H1"), jOr(jC("PSY230H1"), jC("PSYB30H3"), jC("PSY240H1"), jC("PSYB32H3"), jC("PSY270H1")))},

	// grades on lists
	{"DRM403Y1 grade on required courses, not the pool", "Minimum 70% in required courses:  DRM220Y1,  DRM300Y1, 0.5 credit from  DRM301H1/  DRM375H1",
		jAnd(jCG("DRM220Y1", 70), jCG("DRM300Y1", 70), `{"type":"credits","amount":0.5,"courses":["DRM301H1","DRM375H1"]}`)},
	{"CSB397Y0 grade over alternatives", "Minimum grade of 73% in  BIO230H1/ BIO255H1 and permission of the  CSB397Y0 coordinator",
		jAnd(jOr(jCG("BIO230H1", 73), jCG("BIO255H1", 73)), jT("permission", "permission of the CSB397Y0 coordinator"))},
	{"CSC265H1 grade skips a group with its own grades", "minimum of 70% in  CSC240H1/ (minimum of 85% in  CSC236H1,  STA237H1/  STA247H1)",
		jOr(jCG("CSC240H1", 70), jAnd(jCG("CSC236H1", 85), jOr(jC("STA237H1"), jC("STA247H1"))))},
	{"IRE395H1 grade on a credit phrase", "IRE244H1 and at least 68% in 0.5 credit in IRE at the 300+ level",
		jAnd(jC("IRE244H1"), `{"type":"credits","amount":0.5,"designators":["IRE"],"levels":[300,400],"min_grade":68}`)},

	// subject names
	{"ESS245H1 first-year subjects except", "1.0 credit from first-year math, or physics courses except  MAT193H1,  PHY196H1",
		`{"type":"credits","amount":1,"designators":["MAT","PHY"],"levels":[100],"exclude":["MAT193H1","PHY196H1"]}`},
	{"HIS464H1 multi-word subject", "one course in HIS/REL/Peace and Conflict (excluding  HIS262H1)",
		`{"type":"credits","amount":0.5,"designators":["HIS","REL","PCS"],"exclude":["HIS262H1"]}`},
	{"ANT491H1 credits in a subject", "14.0 credits, 3.0 credits in Anthropology",
		jAnd(`{"type":"credits","amount":14}`, `{"type":"credits","amount":3,"designators":["ANT"]}`)},

	// one-off phrasings handled by general rules
	{"BCH210H1 alternative paths", "Successful completion of ( CHM135H1,  CHM136H1)/ CHM151Y1 NOTE: CHM1** with COURSE EXCLUSION TO  CHM135H1 meet the Prerequisite requirement. UTM students who have completed  CHM110H5 (exclusion to  CHM135H1) &  CHM242H5 (exclusion to  CHM136H1) will be permitted to enrol in  BCH210H1.",
		jOr(jAnd(jC("CHM135H1"), jC("CHM136H1")), jC("CHM151Y1"), jAnd(jT("membership", "UTM students"), jC("CHM110H5"), jC("CHM242H5")))},
	{"CSC343H1 label followed by a group", "CSC207H1/  CSCB07H3 Prerequisite for Faculty of Applied Science and Engineering students: (ESC194H1, ESC195H1)/ (MAT186H1, MAT187H1)",
		jOr(jC("CSC207H1"), jC("CSCB07H3"),
			jAnd(jT("membership", "Engineering students"), jOr(jAnd(jC("ESC194H1"), jC("ESC195H1")), jAnd(jC("MAT186H1"), jC("MAT187H1")))))},
	{"BMS422H1 students are required to complete", "Completion of 10.0 credits including  BMS100H1. Students are also required to complete  BMS110Y1,  BMS310Y1 or  BMS421H1.",
		jAnd(`{"type":"credits","amount":10}`, jC("BMS100H1"), jC("BMS110Y1"), jOr(jC("BMS310Y1"), jC("BMS421H1")))},
	{"GGR340H1 the combination of", "GGR270H1, or the combination of  STA220H1 and  HST250H1",
		jOr(jC("GGR270H1"), jAnd(jC("STA220H1"), jC("HST250H1")))},
	{"CHM317H1 garbled code", "( MAT158H1,  MAT159H1/ MAT1MAT157Y1)", jAnd(jC("MAT158H1"), jOr(jC("MAT159H1"), jC("MAT157Y1")))},
	{"PHL357H1 numbered alternatives", "7.5 credits which include one of the following: (1)  PHL255H1 and an additional 1.0 PHL credit, (2) 1.5 PHL credits and  HPS250H1, or (3) 1.5 credits in BCH/ BIO.",
		jAnd(`{"type":"credits","amount":7.5}`, jOr(
			jAnd(jC("PHL255H1"), `{"type":"credits","amount":1,"designators":["PHL"]}`),
			jAnd(`{"type":"credits","amount":1.5,"designators":["PHL"]}`, jC("HPS250H1")),
			`{"type":"credits","amount":1.5,"designators":["BCH","BIO"]}`))},
	{"BPM438H1 examples", "A statistics course (e.g.  PSY201H1,  SOC202H1)",
		jOr(jT("other", "A statistics course"), jC("PSY201H1"), jC("SOC202H1"))},
	{"DRM300Y1 if complete", "DRM220Y1 or any course from Group A if  DRM220Y1 is complete",
		jAnd(jOr(jC("DRM220Y1"), jT("other", "any course from Group A")), jC("DRM220Y1"))},
	{"ENV461H1 ; or permission", "Completion of 10.0 credits including  ENV221H1 or  ENV222H1; or permission of the Undergraduate Associate Director",
		jOr(jAnd(`{"type":"credits","amount":10}`, jOr(jC("ENV221H1"), jC("ENV222H1"))), jT("permission", "permission of the Undergraduate Associate Director"))},
	{"HPS412H1 a is noise", "9.0 credits including a 0.5 HPS credit",
		jAnd(`{"type":"credits","amount":9}`, `{"type":"credits","amount":0.5,"designators":["HPS"]}`)},

	// audience conditionals
	{"BCH374Y1", "For Specialist:  BCH242Y1 (75% or higher);  BIO230H1;  CHM247H1/ CHM249H1. For Major:  BCH210H1 (80% or higher);  BIO230H1;  CHM247H1/ CHM249H1.",
		jOr(
			jAnd(jT("membership", "For Specialist"), jCG("BCH242Y1", 75), jC("BIO230H1"), jOr(jC("CHM247H1"), jC("CHM249H1"))),
			jAnd(jT("membership", "For Major"), jCG("BCH210H1", 80), jC("BIO230H1"), jOr(jC("CHM247H1"), jC("CHM249H1"))))},
	{"ANA411H1", "For KPE students:  ANA124H1,  ANA125H1, and  ANA126Y1 For Arts & Science students:  ANA300Y1 (78%) or equivalent",
		jOr(
			jAnd(jT("membership", "For KPE students"), jC("ANA124H1"), jC("ANA125H1"), jC("ANA126Y1")),
			jAnd(jT("membership", "For Arts & Science students"), jOr(jCG("ANA300Y1", 78), jT("preparation", "equivalent"))))},
	{"RSM433H1 capitalized labels", "Rotman Commerce:  RSM333H1; Actuarial Science:  ACT349H1",
		jOr(jAnd(jT("membership", "Rotman Commerce"), jC("RSM333H1")), jAnd(jP("Actuarial Science"), jC("ACT349H1")))},
	{"CSC311H1-style engineering branch", "CSC207H1/  CSCB07H3;  MAT223H1/  MAT240H1 Prerequisite for Faculty of Applied Science and Engineering students: APS105H1/ ESC180H1; MAT185H1",
		jOr(
			jAnd(jOr(jC("CSC207H1"), jC("CSCB07H3")), jOr(jC("MAT223H1"), jC("MAT240H1"))),
			jAnd(jT("membership", "Engineering students"), jOr(jC("APS105H1"), jC("ESC180H1")), jC("MAT185H1")))},
}

func TestParseRequisiteGolden(t *testing.T) {
	for _, g := range reqGoldens {
		t.Run(g.name, func(t *testing.T) {
			assertJSON(t, grammar.ParseRequisite(g.raw).Req, g.want)
		})
	}
}

func TestParseRequisiteEnvelope(t *testing.T) {
	raw := "ANT380H1. Students who do not meet the prerequisite are encouraged to contact the Department."
	assertJSON(t, grammar.ParseRequisite(raw), `{"raw":`+jsonString(raw)+`,"req":`+jC("ANT380H1")+
		`,"notes":["Students who do not meet the prerequisite are encouraged to contact the Department"]}`)

	raw = "MAT130H1/  MAT135H1 recommended"
	assertJSON(t, grammar.ParseRequisite(raw), `{"raw":`+jsonString(raw)+`,"req":null,"notes":["MAT130H1/ MAT135H1 recommended"]}`)

	raw = "Admission to Vic One: https://vic.utoronto.ca/future-students/vic-one/"
	assertJSON(t, grammar.ParseRequisite(raw).Notes, `["https://vic.utoronto.ca/future-students/vic-one/"]`)

	raw = "CSC108H1"
	assertJSON(t, grammar.ParseRequisite(raw), `{"raw":"CSC108H1","req":`+jC("CSC108H1")+`}`)

	// advice that must end up in notes, not in the tree
	raw = "ECO375H1(70%)/ ECO374H1(80%), NOTE:  STA302H1 is not accepted preparation."
	assertJSON(t, grammar.ParseRequisite(raw), `{"raw":`+jsonString(raw)+`,"req":`+jOr(jCG("ECO375H1", 70), jCG("ECO374H1", 80))+
		`,"notes":["NOTE: STA302H1 is not accepted preparation"]}`)

	raw = "BCH375H1 can be combined with  BCH373H1."
	assertJSON(t, grammar.ParseRequisite(raw), `{"raw":`+jsonString(raw)+`,"req":null,"notes":["BCH375H1 can be combined with BCH373H1"]}`)

	raw = "Minimum GPA of 3.5 in Astronomy program courses. Course should not be taken concurrently with  AST425Y1."
	assertJSON(t, grammar.ParseRequisite(raw).Notes, `["Course should not be taken concurrently with AST425Y1"]`)

	raw = "10.0 credits, including 1.0 credit from  MUN200H1/  MUN210H1 completed before  MUN368H1 is taken, and an application/interview."
	assertJSON(t, grammar.ParseRequisite(raw), `{"raw":`+jsonString(raw)+`,"req":`+jAnd(`{"type":"credits","amount":10}`,
		`{"type":"credits","amount":1,"courses":["MUN200H1","MUN210H1"]}`, jT("permission", "an application/interview"))+
		`,"notes":["completed before MUN368H1 is taken"]}`)

	raw = "CSC236H1. Notes: students enrolled in ASMAJ1446A may substitute  CSC111H1 for  CSC263H1."
	assertJSON(t, grammar.ParseRequisite(raw).Notes, `["Notes: students enrolled in ASMAJ1446A may substitute CSC111H1 for CSC263H1"]`)

	raw = "ECO200Y1. Further prerequisites (e.g.  ECO225H1/ GGR272H1) may be required, consult the Department."
	assertJSON(t, grammar.ParseRequisite(raw).Notes, `["Further prerequisites (e.g. ECO225H1/ GGR272H1) may be required, consult the Department"]`)
}

func TestClassify(t *testing.T) {
	for text, want := range map[string]types.TextKind{
		"permission of the instructor": types.TextPermission,
		"by ballot":                    types.TextPermission,
		"Enrolment in the Actuarial Science Specialist program": types.TextMembership,
		"Admission to Munk One":                                 types.TextMembership,
		"Active in ASMAJ1618":                                   types.TextMembership,
		"French Placement Test":                                 types.TextPlacement,
		"No previous knowledge of French":                       types.TextPlacement,
		"High school math":                                      types.TextPreparation,
		"equivalent programming experience":                     types.TextPreparation,
		"in Society":                                            types.TextOther,
		"Any one of the following":                              types.TextOther,
		"Placement in FSL420H1 or higher through the French Placement Test or by Department permission": types.TextPlacement,
		"Placement audition and permission of the Department required":                                  types.TextPermission,
	} {
		if got := grammar.Classify(text); got != want {
			t.Errorf("classify(%q) = %q, want %q", text, got, want)
		}
	}
}
