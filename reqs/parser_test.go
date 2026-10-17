package reqs

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"degree-explorer/types"
)

func TestParseExpr(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"spec example",
			"(AAA101H1/ AAA102H1/ AAA103H1), BBB101H1, CCC101H1/ CCC102H1, (DDD101H1, EEE101H1/ EEE102H1)",
			jAnd(jOr(jC("AAA101H1"), jC("AAA102H1"), jC("AAA103H1")), jC("BBB101H1"),
				jOr(jC("CCC101H1"), jC("CCC102H1")), jC("DDD101H1"), jOr(jC("EEE101H1"), jC("EEE102H1")))},
		{"ACT245H1",
			"ACT240H1, ( MAT130H1/ MAT135H1, MAT136H1)/ ( MAT148H1, MAT149H1)/ MAT137Y1/ ( MAT158H1, MAT159H1)/ MAT157Y1",
			jAnd(jC("ACT240H1"), jOr(
				jAnd(jOr(jC("MAT130H1"), jC("MAT135H1")), jC("MAT136H1")),
				jAnd(jC("MAT148H1"), jC("MAT149H1")),
				jC("MAT137Y1"),
				jAnd(jC("MAT158H1"), jC("MAT159H1")),
				jC("MAT157Y1")))},
		{"ECO324H1 semicolon",
			"ECO200Y1/ ECO204Y1/ ECO206Y1; ECO220Y1/ ECO227Y1/ ( STA237H1, STA238H1)",
			jAnd(jOr(jC("ECO200Y1"), jC("ECO204Y1"), jC("ECO206Y1")),
				jOr(jC("ECO220Y1"), jC("ECO227Y1"), jAnd(jC("STA237H1"), jC("STA238H1"))))},
		{"APM348H1 slash binds tighter",
			"MAT244H1/ MAT267H1, MAT224H1/ MAT247H1, STA237H1/ STA247H1",
			jAnd(jOr(jC("MAT244H1"), jC("MAT267H1")), jOr(jC("MAT224H1"), jC("MAT247H1")), jOr(jC("STA237H1"), jC("STA247H1")))},
		{"INS420H1 plus and comma-and",
			"INS201Y1/ ( INS201H1, INS202H1), plus INS220Y1, INS320Y1, INS303H1, and INS304H1",
			jAnd(jOr(jC("INS201Y1"), jAnd(jC("INS201H1"), jC("INS202H1"))),
				jC("INS220Y1"), jC("INS320Y1"), jC("INS303H1"), jC("INS304H1"))},
		{"grade suffixes",
			"STA247H1(70%)/ BCH311H1 (75% or higher)/ STA257H1",
			jOr(jCG("STA247H1", 70), jCG("BCH311H1", 75), jC("STA257H1"))},
		{"comma-or", "CIN105Y1, or CIN201Y1", jOr(jC("CIN105Y1"), jC("CIN201Y1"))},
		{"text alternative", "ANT205H1 or permission of the instructor",
			jOr(jC("ANT205H1"), jT("", "permission of the instructor"))},
		{"text swallows a mentioned course", "placement in FSL220H1 based on the test",
			jT("", "placement in FSL220H1 based on the test")},
		{"trailing operator", "CSC108H1, and", jC("CSC108H1")},
		{"missing close paren", "( CSC108H1/ CSC148H1", jOr(jC("CSC108H1"), jC("CSC148H1"))},
		{"stray close paren", "CSC108H1) CSC148H1", jAnd(jC("CSC108H1"), jC("CSC148H1"))},
		{"adjacent atoms", "CSC108H1 CSC148H1", jAnd(jC("CSC108H1"), jC("CSC148H1"))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertJSON(t, parseExpr(tt.src), tt.want)
		})
	}
}

// Completion grammar extensions on real completion text. Each is gated:
// TestCompletionGrammarGated shows that course text still parses as before.
func TestParseCompletionText(t *testing.T) {
	tests := []struct{ name, src, want string }{
		// ";" is the lowest-precedence "and"; in a pool it separates items.
		{"ASMAJ1689 semicolon",
			"( CSC108H1, CSC148H1, CSC165H1/ CSC240H1)/ ( CSC110Y1, CSC111H1); ( MAT148H1/ MAT158H1/ MAT130H1/ MAT135H1, MAT149H1/ MAT159H1/ MAT136H1)/ MAT137Y1/ MAT157Y1",
			jAnd(
				jOr(jAnd(jC("CSC108H1"), jC("CSC148H1"), jOr(jC("CSC165H1"), jC("CSC240H1"))), jAnd(jC("CSC110Y1"), jC("CSC111H1"))),
				jOr(jAnd(jOr(jC("MAT148H1"), jC("MAT158H1"), jC("MAT130H1"), jC("MAT135H1")), jOr(jC("MAT149H1"), jC("MAT159H1"), jC("MAT136H1"))),
					jC("MAT137Y1"), jC("MAT157Y1")))},
		{"ASCER1200 semicolons in a pool", "2.0 credits from: SLA117H1; SLA118H1; SLA257H1; SLA277H1; SLA337H1",
			`{"type":"credits","amount":2,"courses":["SLA117H1","SLA118H1","SLA257H1","SLA277H1","SLA337H1"]}`},
		{"ASMAJ1098 a pool after a semicolon",
			"BIO120H1; 1.0 credit from MSE101H1; ECO101H1, ECO102H1, ECO105Y1; CHM135H1, CHM136H1; MAT130H1/ MAT135H1",
			jAnd(jC("BIO120H1"), `{"type":"credits","amount":1,"courses":["MSE101H1","ECO101H1","ECO102H1","ECO105Y1","CHM135H1","CHM136H1","MAT130H1","MAT135H1"]}`,
				jPairCap(0.5, "MAT130H1", "MAT135H1"))},
		// "and" over groups of alternatives only when each group weighs the same either
		// way (or, with a range, when the range accounts for the difference).
		{"ASMAJ2660 a Y course after \"/\" replaces the H pair in a pool", "1.0 credit from ECO101H1, ECO102H1/ ECO105Y1",
			jOr(jAnd(jC("ECO101H1"), jC("ECO102H1")), jC("ECO105Y1"))},
		{"ASSPE2290 uneven alternatives the range doesn't allow are a union",
			"0.5 to 1.0 credit from STA492H1, STA496H1/ STA497H1/ STA498Y1/ STA499Y1",
			jAnd(`{"type":"credits","amount":0.5,"courses":["STA492H1","STA496H1","STA497H1","STA498Y1","STA499Y1"]}`,
				jPairCap(1, "STA496H1", "STA497H1", "STA498Y1", "STA499Y1"))},
		{"ASSPE2290 uneven alternatives the range allows are each needed",
			"2.5 to 3.0 credits from STA130H1, CSC108H1/ CSC110Y1/ CSC111H1/ CSC148H1, ( MAT148H1, MAT149H1)/ MAT137Y1/ ( MAT158H1, MAT159H1)/ MAT157Y1, MAT223H1/ MAT224H1/ MAT240H1",
			jAnd(jC("STA130H1"), jOr(jC("CSC108H1"), jC("CSC110Y1"), jC("CSC111H1"), jC("CSC148H1")),
				jOr(jAnd(jC("MAT148H1"), jC("MAT149H1")), jC("MAT137Y1"), jAnd(jC("MAT158H1"), jC("MAT159H1")), jC("MAT157Y1")),
				jOr(jC("MAT223H1"), jC("MAT224H1"), jC("MAT240H1")))},
		{"ASSPE1469 two codes with only a space between them in a \"/\" list", "1.0 credit from: GGR342H1/ GGR343H1 GGR344H1/ JPA331H1",
			`{"type":"credits","amount":1,"courses":["GGR342H1","GGR343H1","GGR344H1","JPA331H1"]}`},
		{"ASCER1160 one grade after a slash list covers it", "RSM100H1/ MGT100H1/ ECO101H1 (minimum grade of 63%)",
			jOr(`{"type":"course","code":"RSM100H1","min_grade":63}`, `{"type":"course","code":"MGT100H1","min_grade":63}`,
				`{"type":"course","code":"ECO101H1","min_grade":63}`)},
		{"ASMAJ0509 \"chosen from\"", "3.0 credits chosen from 300/400-level ESS/ JGA/ JPE courses or JEG301H1",
			`{"type":"credits","amount":3,"designators":["ESS","JGA","JPE"],"levels":[300,400],"courses":["JEG301H1"]}`},
		{"ASMIN1107 semicolon in a count pool", "One of PHL373H1, ENV333H1; JGE321H1, JGE331H1",
			jOr(jC("PHL373H1"), jC("ENV333H1"), jC("JGE321H1"), jC("JGE331H1"))},
		{"ASSPE2270 \"or\" before the last item closes the pool",
			"( LIN101H1, LIN102H1); 1.0 credit from LIN228H1, LIN229H1, LIN232H1 or LIN241H1; LIN351H1 and LIN456H1",
			jAnd(jC("LIN101H1"), jC("LIN102H1"), `{"type":"credits","amount":1,"courses":["LIN228H1","LIN229H1","LIN232H1","LIN241H1"]}`,
				jC("LIN351H1"), jC("LIN456H1"))},
		{"ASSPE1687 one of stops after its \"or\"",
			"STA302H1, one of STA303H1 or STA305H1, STA355H1, CSC209H1, CSC263H1/ CSC265H1 ( CSC265H1 is recommended), CSC343H1",
			jAnd(jC("STA302H1"), jOr(jC("STA303H1"), jC("STA305H1")), jC("STA355H1"), jC("CSC209H1"), jOr(jC("CSC263H1"), jC("CSC265H1")), jC("CSC343H1"))},
		{"ASMAJ2276A \"and\" before the last item",
			"4.0 credits from music history, culture, and theory courses: HMU111H1, HMU126H1, TMU130H1, TMU131H1, TMU145Y1, TMU230H1 and TMU231H1",
			`{"type":"credits","amount":4,"courses":["HMU111H1","HMU126H1","TMU130H1","TMU131H1","TMU145Y1","TMU230H1","TMU231H1"]}`},
		{"ASMAJ0482 groups whose credits add up to the amount",
			"2.0 credits from ( BIO120H1, BIO130H1)/ BIO150Y; ( CHM135H1, CHM136H1)/ CHM151Y1",
			jAnd(jOr(jAnd(jC("BIO120H1"), jC("BIO130H1")), jC("BIO150Y1")), jOr(jAnd(jC("CHM135H1"), jC("CHM136H1")), jC("CHM151Y1")))},
		{"ASMIN0482 groups whose credits add up to the amount",
			"2.0 credits from: ( BIO120H1, BIO130H1); PSL201Y1/( PSL300H1, PSL301H1)",
			jAnd(jC("BIO120H1"), jC("BIO130H1"), jOr(jC("PSL201Y1"), jAnd(jC("PSL300H1"), jC("PSL301H1"))))},
		{"ASMAJ2289 groups whose credits add up to the amount",
			"2.5 credits from MAT223H1/ MAT224H1/ MAT240H1, MAT235Y1/ ( MAT235H1, MAT236H1)/ MAT237Y1/ MAT257Y1; ( STA247H1, STA248H1)/ ( STA237H1, STA238H1)/ ( STA257H1, STA261H1)/ ECO227Y1",
			jAnd(jOr(jC("MAT223H1"), jC("MAT224H1"), jC("MAT240H1")),
				jOr(jC("MAT235Y1"), jAnd(jC("MAT235H1"), jC("MAT236H1")), jC("MAT237Y1"), jC("MAT257Y1")),
				jOr(jAnd(jC("STA247H1"), jC("STA248H1")), jAnd(jC("STA237H1"), jC("STA238H1")), jAnd(jC("STA257H1"), jC("STA261H1")), jC("ECO227Y1")))},
		{"ASMAJ1423 \"/\" separates alternatives, which aren't added up",
			"1.0 credit from ( MAT235H1, MAT236H1)/ MAT235Y1/ MAT237Y1/ MAT257Y1",
			jOr(jAnd(jC("MAT235H1"), jC("MAT236H1")), jC("MAT235Y1"), jC("MAT237Y1"), jC("MAT257Y1"))},
		{"ASSPE1469 \"/\" alternatives after a colon", "1.0 credit from: ( TRN409H1, TRN410H1)/ TRN411Y1/ TRN419Y1/ TRN421Y1",
			jOr(jAnd(jC("TRN409H1"), jC("TRN410H1")), jC("TRN411Y1"), jC("TRN419Y1"), jC("TRN421Y1"))},
		// A Y course after "/" replaces the pair of H courses before it.
		{"ASMAJ1423 a Y course after \"/\" inside a group replaces the pair",
			"1.0 credit from ( MAT130H1/ MAT135H1, MAT136H1)/ ( MAT148H1, MAT149H1/ MAT137Y1)/ ( MAT158H1, MAT159H1/ MAT157Y1) (( MAT148H1, MAT149H1/ MAT137Y1) recommended)",
			jOr(jAnd(jOr(jC("MAT130H1"), jC("MAT135H1")), jC("MAT136H1")), jAnd(jC("MAT148H1"), jC("MAT149H1")), jC("MAT137Y1"),
				jAnd(jC("MAT158H1"), jC("MAT159H1")), jC("MAT157Y1"))},
		{"ASMAJ2543 pair or Y course in brackets",
			"( MAT130H1/ MAT135H1, MAT136H1)/ ( MAT148H1, MAT149H1/ MAT137Y1)/ ( MAT158H1, MAT159H1/ MAT157Y1)",
			jOr(jAnd(jOr(jC("MAT130H1"), jC("MAT135H1")), jC("MAT136H1")), jAnd(jC("MAT148H1"), jC("MAT149H1")), jC("MAT137Y1"),
				jAnd(jC("MAT158H1"), jC("MAT159H1")), jC("MAT157Y1"))},
		{"ASMAJ0509 (shortened) unclosed bracket before the Y course",
			"( MAT148H1, MAT149H1/ MAT137Y1; PHY131H1, PHY132H1",
			jAnd(jOr(jAnd(jC("MAT148H1"), jC("MAT149H1")), jC("MAT137Y1")), jC("PHY131H1"), jC("PHY132H1"))},
		{"ASSPE1758 a whole item of a pair or a Y course", "Computer Programming: CSC108H1, CSC148H1/ CSC110Y1",
			jOr(jAnd(jC("CSC108H1"), jC("CSC148H1")), jC("CSC110Y1"))},
		{"ASMAJ1023 a pair in a longer list stays a list",
			"LCT308H1, LCT401H1/ LCT401Y1, LCT403H1",
			jAnd(jC("LCT308H1"), jOr(jC("LCT401H1"), jC("LCT401Y1")), jC("LCT403H1"))},
		{"ASSPE0482 groups that don't add up stay text",
			"1.0 credit from BIO220H1, BIO260H1/ HMB265H1; ( MAT235H1, MAT236H1)/ MAT235Y1, PHY231H1, PSL299Y1/ PSL399Y1",
			jT("other", "1.0 credit from BIO220H1, BIO260H1/ HMB265H1; ( MAT235H1, MAT236H1)/ MAT235Y1, PHY231H1, PSL299Y1/ PSL399Y1")},
		{"ASSPE2270 a semicolon ends the pool before and",
			"( LIN101H1, LIN102H1); 1.0 credit from LIN228H1, LIN229H1, LIN232H1 or LIN241H1; and 1.0 credit from JLP374H1, JLP315H1 or JLP471H1",
			jAnd(jC("LIN101H1"), jC("LIN102H1"),
				`{"type":"credits","amount":1,"courses":["LIN228H1","LIN229H1","LIN232H1","LIN241H1"]}`,
				`{"type":"credits","amount":1,"courses":["JLP374H1","JLP315H1","JLP471H1"]}`)},

		// A range of amounts needs the lower end; the upper end stays in raw.
		{"ASSPE0482 a range before a pool", "1.0-1.5 credits from PSL400-series or HMB430H1/ HMB470H1/ HMB472H1",
			`{"type":"credits","amount":1,"designators":["PSL"],"levels":[400],"courses":["HMB430H1","HMB470H1","HMB472H1"]}`},
		{"ASMAJ1019 a range before a credit phrase", "3.0 to 4.0 credits from any NMC courses",
			`{"type":"credits","amount":3,"designators":["NMC"]}`},
		{"ASMIN1029 (shortened) amounts joined by \"or\"", "0.5 or 1.0 credit from EDS360H1, EDS360Y1, VIC435H1",
			`{"type":"credits","amount":0.5,"courses":["EDS360H1","EDS360Y1","VIC435H1"]}`},
		{"ASSPE2270 groups whose smallest credits pass the range's lower end: text",
			// 2.5 at least (STA130H1, a CSC pair, a MAT pair or Y course) where
			// the range starts at 2.0, and the CSC alternatives weigh 1.0 or
			// 1.5: no reading of "each needed" is sure.
			"2.0 to 2.5 credits from STA130H1, ( CSC108H1, CSC148H1)/ ( CSC110Y1, CSC111H1)/ ( CSC110Y1, CSC148H1), ( MAT130H1/ MAT135H1, MAT136H1)/ ( MAT148H1, MAT149H1)/ MAT137Y1/ ( MAT158H1, MAT159H1) / MAT157Y1",
			jT("other", "2.0 to 2.5 credits from STA130H1, ( CSC108H1, CSC148H1)/ ( CSC110Y1, CSC111H1)/ ( CSC110Y1, CSC148H1), ( MAT130H1/ MAT135H1, MAT136H1)/ ( MAT148H1, MAT149H1)/ MAT137Y1/ ( MAT158H1, MAT159H1) / MAT157Y1")},
		{"ASFOC1478B described pool", "1.5 credit of 300+ ECO elective courses from: ECO310H1, ECO334H1, ECO339H1",
			`{"type":"credits","amount":1.5,"courses":["ECO310H1","ECO334H1","ECO339H1"]}`},
		{"ASMAJ0509 described pool after \"of\"", "2.5 credits of core courses: JEG100H1/ ENV234H1/ EEB240H1/ ESS262H1, ESS223H1",
			jAnd(`{"type":"credits","amount":2.5,"courses":["JEG100H1","ENV234H1","EEB240H1","ESS262H1","ESS223H1"]}`,
				jPairCap(0.5, "JEG100H1", "ENV234H1", "EEB240H1", "ESS262H1"))},
		{"ASMAJ2655 described pool before \"from\"", "0.5 credit laboratory course from: CSB472H1, CSB474H1; EEB460H1",
			`{"type":"credits","amount":0.5,"courses":["CSB472H1","CSB474H1","EEB460H1"]}`},
		{"ASMAJ0525 \"selected from\"", "1.0 credit selected from FRE210H1/ FRE245H1/ FRE273H1",
			`{"type":"credits","amount":1,"courses":["FRE210H1","FRE245H1","FRE273H1"]}`},
		{"ASMAJ0110 courses after \"credits:\" that add up to the amount",
			"2.0 credits: BIO220H1 (ecology and evolutionary biology); EEB225H1/ GGR270H1 (statistics); EEB255H1 (conservation biology); EEB240H1 (environmental biology).",
			jAnd(jC("BIO220H1"), jOr(jC("EEB225H1"), jC("GGR270H1")), jC("EEB255H1"), jC("EEB240H1"))},
		{"constructed: a description with a level is a filter the list must pass",
			"0.5 credit of 400-level ECO courses from: ECO310H1, ECO410H1",
			jT("other", "0.5 credit of 400-level ECO courses from: ECO310H1, ECO410H1")},
		{"ASMIN1006 courses after \"credits:\" that add up to the amount", "1.0 credits: BIO120H1; BIO220H1",
			jAnd(jC("BIO120H1"), jC("BIO220H1"))},
		{"ASMAJ0509 a described pool larger than its amount",
			"0.5 credit of field courses chosen from: ESS234H1/ ESS324H1/ ESS410H1/ ESS450H1",
			`{"type":"credits","amount":0.5,"courses":["ESS234H1","ESS324H1","ESS410H1","ESS450H1"]}`},

		// Whole-number amounts and "must be taken from".
		{"ASSPE0482 a whole-number amount", "3 credits from BCH370H1; PSL304H1, PSL305H1, PSL350H1/ CSB349H1/ BCH311H1, PSL372H1, PSL374H1",
			jAnd(jC("BCH370H1"), jC("PSL304H1"), jC("PSL305H1"), jOr(jC("PSL350H1"), jC("CSB349H1"), jC("BCH311H1")), jC("PSL372H1"), jC("PSL374H1"))},
		{"ASMIN2453 must be taken from", "1.0 credit must be taken from GER270H1/ GER272H1/ GER290H1/ GER391H1",
			`{"type":"credits","amount":1,"courses":["GER270H1","GER272H1","GER290H1","GER391H1"]}`},
		{"ASMAJ2027A a description with \"additional\"", "0.5 credits of additional music history: HMU225H1/ JMU210H1",
			`{"type":"credits","amount":0.5,"courses":["HMU225H1","JMU210H1"]}`},

		{"ASMIN1423 \"credit of\" before codes", "1 credit of AST221H1, AST222H1",
			`{"type":"credits","amount":1,"courses":["AST221H1","AST222H1"]}`},
		{"ASMIN1423 a doubled \"from\"", "0.5 credit from from PHY252H1, PHY224H1",
			`{"type":"credits","amount":0.5,"courses":["PHY252H1","PHY224H1"]}`},
		{"ASSPE1002 one course from the following list", "One course from the following list: BIO220H1/ TRN225Y1/ CHM220H1",
			jOr(jC("BIO220H1"), jC("TRN225Y1"), jC("CHM220H1"))},
		{"ASSPE1361 \"further\"", "2.0 credits further from PHL325H1/ PHL331H1",
			`{"type":"credits","amount":2,"courses":["PHL325H1","PHL331H1"]}`},
		{"ASSPE2027 the credits a list adds up to", "HMU225H1, TMU240Y1 (1.5 credits)", jAnd(jC("HMU225H1"), jC("TMU240Y1"))},
		{"ASSPE1211 recommended before \"/\"", "CHM220H1/ CHM222H1, CHM223H1, ( CHM249H1 strongly recommended/ CHM247H1)",
			jAnd(jOr(jC("CHM220H1"), jC("CHM222H1")), jC("CHM223H1"), jOr(jC("CHM249H1"), jC("CHM247H1")))},

		{"ASMAJ0482 groups with alternatives that add up to the amount", "2.0 credits from BCH210H1; BIO230H1/ BIO255H1; PSL300H1, PSL301H1",
			jAnd(jC("BCH210H1"), jOr(jC("BIO230H1"), jC("BIO255H1")), jC("PSL300H1"), jC("PSL301H1"))},
		{"ASMIN1423 pairs of alternatives that add up to the amount", "1 credit from ( PHY131H1/ PHY151H1), ( PHY132H1/ PHY152H1)",
			jAnd(jOr(jC("PHY131H1"), jC("PHY151H1")), jOr(jC("PHY132H1"), jC("PHY152H1")))},
		{"ASCER1206 a missing comma", "2.0 credits from: SLA321H1; SLA322H1 SLA410H1; SLA420Y1",
			`{"type":"credits","amount":2,"courses":["SLA321H1","SLA322H1","SLA410H1","SLA420Y1"]}`},
		{"ASMIN1253 a course in brackets after a course", "CHM136H1 ( CHM138H1)", jOr(jC("CHM136H1"), jC("CHM138H1"))},
		{"ASCER1401 a title in quotes", "GER290H1 ‘Global Issues / German Contexts’.", jC("GER290H1")},

		{"ASMIN2453 \"and\" before the last excluded course", "1.0 additional GER or JGJ credit excluding GER150H1, GER361H1, and GER410H1",
			`{"type":"credits","amount":1,"designators":["GER","JGJ"],"exclude":["GER150H1","GER361H1","GER410H1"]}`},
		{"ASSPE2027 subjects named with their designators", "1.5 credit in music history (HMU) or music theory (TMU) at the 300+ level",
			`{"type":"credits","amount":1.5,"designators":["HMU","TMU"],"levels":[300,400]}`},
		{"ASSPE2599 an alternative designator and a course counted elsewhere",
			"1.0 credit from 300-level or higher MAT (or APM) courses (in addition to APM462H1)",
			`{"type":"credits","amount":1,"designators":["MAT","APM"],"levels":[300,400],"exclude":["APM462H1"]}`},

		{"ASFOC1689F a remark on double counting", "1.0 credit from the following: CSC457H1/ CSC458H1 (if not taken in list 1), CSC324H1, CSC364H1",
			jAnd(`{"type":"credits","amount":1,"courses":["CSC457H1","CSC458H1","CSC324H1","CSC364H1"]}`, jPairCap(0.5, "CSC457H1", "CSC458H1"))},
		{"ASFOC0652A a course's title after a colon", "Methodological Training Requirement: HIS268H1: Law and History.", jC("HIS268H1")},

		{"ASMAJ1682 an article before an alternative amount, a breadth category's name",
			"0.5 credit from MST341H1/ SMC385H1 or a 0.5 credit at the 200/300/400-level from Breadth Requirement Category 5: The Physical and Mathematical Universes.",
			jOr(`{"type":"credits","amount":0.5,"courses":["MST341H1","SMC385H1"]}`, `{"type":"credits","amount":0.5,"levels":[200,300,400],"breadth":[5]}`)},
		{"ASMIN1165 a grade written with a plus", "MAT221H1(80%+)/ MAT223H1/ MAT240H1",
			jOr(`{"type":"course","code":"MAT221H1","min_grade":80}`, jC("MAT223H1"), jC("MAT240H1"))},

		{"ASMIN1390 a course joined to a pool by \"or\" is in the pool", "1.0 credits in any EEB or ENV432H1",
			`{"type":"credits","amount":1,"designators":["EEB"],"courses":["ENV432H1"]}`},
		{"ASSPE1002 a course worth the amount stays an alternative", "IMM450Y1 or 1.0 credit at the 400-level in ANA, BCH, IMM",
			jOr(jC("IMM450Y1"), `{"type":"credits","amount":1,"designators":["ANA","BCH","IMM"],"levels":[400]}`)},

		// Floors after an amount read as "including", whatever the wording.
		{"ASMIN1107 \"with at least\"", "Additional 2.0 PHL credits, with at least a 0.5 credit at the 300+ level",
			jAnd(`{"type":"credits","amount":2,"designators":["PHL"]}`, `{"type":"credits","amount":0.5,"levels":[300,400]}`)},

		// Course titles, footnote references and timing in brackets.
		{"ASMIN1124 a course title", "HIS389H1 (Hungarian History), HIS453H1", jAnd(jC("HIS389H1"), jC("HIS453H1"))},
		{"ASSPE1868 a footnote reference", "( CSC111H1/ CSC165H1, CSC236H1)/ CSC240H1 (1)",
			jOr(jAnd(jOr(jC("CSC111H1"), jC("CSC165H1")), jC("CSC236H1")), jC("CSC240H1"))},
		{"ASMIN0385 see note", "CJS200H1/ CJS201H1 (See Note)", jOr(jC("CJS200H1"), jC("CJS201H1"))},
		{"a condition in brackets stays", "1.0 credit of GER courses (taught in German)",
			jAnd(`{"type":"credits","amount":1,"designators":["GER"]}`, jT("other", "taught in German"))},

		// labels
		{"ASMAJ0552 label", "Geography: JGE321H1", jC("JGE321H1")},
		{"ASSPE2025 two labels", "Option A: Immunology: ( IMM341H1, IMM351H1)", jAnd(jC("IMM341H1"), jC("IMM351H1"))},
		{"ASMAJ1205 italic label", "_Ukrainian:_ SLA108Y1, SLA208Y1, SLA308H1", jAnd(jC("SLA108Y1"), jC("SLA208Y1"), jC("SLA308H1"))},
		{"ASSPE1580 label before a pool", "Algebra: 1.0 credit from MAT223H1/ MAT240H1, MAT224H1/ MAT247H1",
			jAnd(jOr(jC("MAT223H1"), jC("MAT240H1")), jOr(jC("MAT224H1"), jC("MAT247H1")))},
		{"ASMIN0667 advice keeps its label", "Recommended: 0.5 credit from any HPS 100-level courses.",
			jAnd(jT("other", "Recommended"), `{"type":"credits","amount":0.5,"designators":["HPS"],"levels":[100]}`)},

		// wildcard pool members
		{"ASMAJ0482 PSL400-series courses",
			"1.0 credit from HMB430H1/ HMB470H1/ HMB472H1; JPM400Y1; PSL304H1, PSL399Y1, PSL400-series courses",
			jAnd(`{"type":"credits","amount":1,"items":[{"type":"credits","courses":["HMB430H1","HMB470H1","HMB472H1","JPM400Y1","PSL304H1","PSL399Y1"]},{"type":"credits","designators":["PSL"],"levels":[400]}]}`,
				jPairCap(0.5, "HMB430H1", "HMB470H1", "HMB472H1"))},
		{"ASMAJ2027A described pool of wildcards",
			"2.5 credits from electives in music history, culture, and theory, and music and health: HMU200+ level courses; JMU210H1; JMU215H1; TMU200+ courses; SMU400-level courses",
			`{"type":"credits","amount":2.5,"items":[{"type":"credits","courses":["JMU210H1","JMU215H1"]},{"type":"credits","designators":["HMU","TMU"],"levels":[200,300,400]},{"type":"credits","designators":["SMU"],"levels":[400]}]}`},
		{"ASSPE0397 any ETH200+ as an alternative", "EEB215H1/ ENV200H1/ any ETH200+/ FOR201H1",
			jOr(jC("EEB215H1"), jC("ENV200H1"), `{"type":"credits","amount":0.5,"designators":["ETH"],"levels":[200,300,400]}`, jC("FOR201H1"))},
		{"ASSPE2737 listed code inside the filter", "1.0 credit from APM346H1/ PHY 300-level courses/ PHY 400-level courses",
			`{"type":"credits","amount":1,"courses":["APM346H1"],"designators":["PHY"],"levels":[300,400]}`},
		{"ASMAJ0826 bare designators", "2.0 credits from CRI100H1/ ECO/ HIS/ PHL/ POL/ PSY/ SOC",
			`{"type":"credits","amount":2,"courses":["CRI100H1"],"designators":["ECO","HIS","PHL","POL","PSY","SOC"]}`},
		{"ASMAJ1006 bare designator with exclusions",
			"0.5 credit from: BIO251H1; BIO270H1/ PSL300H1; EEB (excluding EEB196H1, EEB197H1); ENV432H1",
			jAnd(`{"type":"credits","amount":0.5,"courses":["BIO251H1","BIO270H1","PSL300H1","ENV432H1"],"designators":["EEB"],"exclude":["EEB196H1","EEB197H1"]}`,
				jPairCap(0.5, "BIO270H1", "PSL300H1"))},
		{"ASMAJ2364 the pool's level narrows every member", "1.0 credit at 300+ level from: CJH332H1; CSB; EEB; EHJ352H1; ENV432H1",
			`{"type":"credits","amount":1,"courses":["CJH332H1","EHJ352H1","ENV432H1"],"designators":["CSB","EEB"],"levels":[300,400]}`},
		{"a qualified member ends the pool",
			"1.0 credit from POL219H1, POL324H1, or any POL course with European content approved by the Program Coordinator.",
			jOr(`{"type":"credits","amount":1,"courses":["POL219H1","POL324H1"]}`,
				jAnd(`{"type":"credits","amount":0.5,"designators":["POL"]}`, jT("permission", "with European content approved by the Program Coordinator")))},
		{"ASMAJ0962 either between designators", "at least 1.0 credit at the 400-level in either GRK or LAT",
			`{"type":"credits","amount":1,"designators":["GRK","LAT"],"levels":[400]}`},
		{"ASSPE2740 a level floor after including is not a pool",
			"1.0 credit, including at least 0.5 credit at the 400-level from APM346H1/ PHY 300-level courses/ PHY 400-level courses ( APM346H1, PHY350H1, PHY356H1, PHY407H1, PHY452H1, PHY454H1, PHY460H1 recommended)",
			jAnd(`{"type":"credits","amount":1}`, jT("other", "at least 0.5 credit at the 400-level from APM346H1/ PHY 300-level courses/ PHY 400-level courses"))},
		{"a pool level that would drop a listed code is text", "0.5 credit at the 400-level from CSB326H1, CSB427H1",
			jT("other", "0.5 credit at the 400-level from CSB326H1, CSB427H1")},
		{"a pool level that would leave a member no level is text", "0.5 credit at the 400-level from: CSB426H1; PHY 300-level courses; EEB",
			jT("other", "0.5 credit at the 400-level from: CSB426H1; PHY 300-level courses; EEB")},
		{"filters that would change if merged stay apart",
			"1.0 credit from PSL300-series courses, PSL400-series courses (excluding PSL350H1)",
			`{"type":"credits","amount":1,"items":[{"type":"credits","designators":["PSL"],"levels":[300]},{"type":"credits","designators":["PSL"],"levels":[400],"exclude":["PSL350H1"]}]}`},
		{"ASSPE2737 Excludes after the pool",
			"1.5 credits from BCH370H1/ CSB428H1/ BCH 400-level courses ( BCH422H1, BCH425H1, BCH426H1, BCH428H1, BCH440H1, BCH450H1 recommended). Excludes BCH472Y1, BCH473Y1, BCH478H1, BCH479H1.",
			`{"type":"credits","amount":1.5,"items":[{"type":"credits","courses":["BCH370H1","CSB428H1"]},{"type":"credits","designators":["BCH"],"levels":[400],"exclude":["BCH472Y1","BCH473Y1","BCH478H1","BCH479H1"]}]}`},
		{"ASMIN1944 excluding after the pool",
			"0.5 credit from: PHY 300-level course/PHY 400-level course/ JPE395H1/ JPE493H1, excluding JPH311H1, JPH441H1",
			`{"type":"credits","amount":0.5,"courses":["JPE395H1","JPE493H1"],"designators":["PHY"],"levels":[300,400],"exclude":["JPH311H1","JPH441H1"]}`},
		{"ASSPE1944 excluding after alternatives",
			"Additional 0.5 credit from PHY 300-level courses/ PHY 400-level courses/ JPE395H1/ JPE493H1, excluding JPH311H1, JPH441H1",
			jOr(`{"type":"credits","amount":0.5,"designators":["PHY"],"levels":[300,400],"exclude":["JPH311H1","JPH441H1"]}`, jC("JPE395H1"), jC("JPE493H1"))},
		{"a bare designator in prose stays text", "Courses taught at UTM or UTSC are not included for consideration.",
			jT("other", "Courses taught at UTM or UTSC are not included for consideration")},

		// pool wordings
		{"ASMAJ1252 Any N courses (X credits) from", "Any 2 courses (1.0 credit) from GGR107H1, JEG100H1, GGR101H1",
			`{"type":"credits","amount":1,"courses":["GGR107H1","JEG100H1","GGR101H1"]}`},
		{"ASMAJ2030 slash pairs in the pool", "Any 3 courses (1.5 credits) from GGR201H1/ JEG201H1, GGR203H1, GGR205H1/ JEG205H1, GGR206H1/ JEG206H1",
			jAnd(`{"type":"credits","amount":1.5,"courses":["GGR201H1","JEG201H1","GGR203H1","GGR205H1","JEG205H1","GGR206H1","JEG206H1"]}`,
				jPairCap(0.5, "GGR201H1", "JEG201H1"), jPairCap(0.5, "GGR205H1", "JEG205H1"), jPairCap(0.5, "GGR206H1", "JEG206H1"))},
		{"ASMAJ0506 All of:", "All of: LIN101H1, LIN102H1", jAnd(jC("LIN101H1"), jC("LIN102H1"))},
		{"ASMAJ1536 All of", "All of IRE242H1, IRE244H1, ( IRE379H1/ IRW220H1), IRE430H1",
			jAnd(jC("IRE242H1"), jC("IRE244H1"), jOr(jC("IRE379H1"), jC("IRW220H1")), jC("IRE430H1"))},
		{"ASMAJ1068 any two of", "any two of: NFS382H1, NFS386H1; NFS302H1",
			jChoose(2, jC("NFS382H1"), jC("NFS386H1"), jC("NFS302H1"))},
		{"Either with a group on the left", "Either ( PRT101H1, PRT102H1)/ PRT100Y1 or PRT120H1, PRT220Y1",
			jOr(jAnd(jC("PRT101H1"), jC("PRT102H1")), jC("PRT100Y1"), jAnd(jC("PRT120H1"), jC("PRT220Y1")))},
		{"ASMIN1107 One of", "One of CHM210H1 (recommended) or ESS262H1/ JEG100H1",
			jOr(jC("CHM210H1"), jC("ESS262H1"), jC("JEG100H1"))},

		// annotations
		{"ASSPE0600 strongly recommended alternative", "( CHM151Y1 strongly recommended)/ ( CHM135H1, CHM136H1)",
			jOr(jC("CHM151Y1"), jAnd(jC("CHM135H1"), jC("CHM136H1")))},
		{"ASMAJ1376 after a label", "First Year: ( CHM151Y1 strongly recommended)/( CHM135H1 and CHM136H1)",
			jOr(jC("CHM151Y1"), jAnd(jC("CHM135H1"), jC("CHM136H1")))},
		{"ASMAJ1053 (Recommended)", "0.5 credit in statistics: STA288H1 (Recommended)/ STA220H1/ EEB225H1",
			`{"type":"credits","amount":0.5,"courses":["STA288H1","STA220H1","EEB225H1"]}`},
		{"ASMAJ0571 (no longer offered)", "WGS336H1, WGS350H1 (no longer offered) , WGS355H1",
			jAnd(jC("WGS336H1"), jC("WGS350H1"), jC("WGS355H1"))},
		{"ASMAJ0608 we strongly recommend",
			"( STA257H1, STA261H1)/ ( STA237H1, STA238H1); while either pair of courses is accepted, we strongly recommend ( STA257H1, STA261H1)",
			jOr(jAnd(jC("STA257H1"), jC("STA261H1")), jAnd(jC("STA237H1"), jC("STA238H1")))},
		{"ASMIN0571 (no longer offered) before a comma", "WGS350H1 (no longer offered), WGS355H1", jAnd(jC("WGS350H1"), jC("WGS355H1"))},
		{"ASSPE1053 footnote star", "1.0 credit from project course: HMB490Y1/ HMB491Y1/ HMB496Y1*",
			`{"type":"credits","amount":1,"courses":["HMB490Y1","HMB491Y1","HMB496Y1"]}`},
		{"ASMAJ1076 footnote stars", "0.5 credit from the following: ENV432H1*/ ENV440H1/ ENV452H1*",
			`{"type":"credits","amount":0.5,"courses":["ENV432H1","ENV440H1","ENV452H1"]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := parseCompletionText(tt.src)
			assertJSON(t, got, tt.want)
		})
	}
}

// ASSPE1252: "Either" makes the "or" split everything before and after it.
// (The tail of this line is other wording; only the split is checked here.)
func TestParseCompletionEither(t *testing.T) {
	n, _ := parseCompletionText("Either GGR491Y1 or one of GGR416H1, GGR492H1, GGR497H1 and one additional 0.5 credit 400 level course from Group A or Group B (1.0 credit). Up to one 0.5 credit can be used from GGR493Y1, based on suitability of placement for this program, and approved by department.")
	if n == nil || n.Type != types.NodeOr || len(n.Items) != 2 || n.Items[1].Type != types.NodeAnd {
		t.Fatalf("want or(GGR491Y1, and(...)), got %s", mustJSON(n))
	}
	assertJSON(t, n.Items[0], jC("GGR491Y1"))
	assertJSON(t, n.Items[1].Items[0], jOr(jC("GGR416H1"), jC("GGR492H1"), jC("GGR497H1")))
	assertJSON(t, n.Items[1].Items[1], `{"type":"credits","amount":0.5,"levels":[400]}`)
}

// Advice set aside by parseCompletionText comes back as notes.
func TestParseCompletionTextNotes(t *testing.T) {
	tests := []struct {
		src, want string
		notes     []string
	}{
		{"1.0 credit from PHY131H1/ PHY151H1, PHY132H1/ PHY152H1 ( PHY151H1, PHY152H1 recommended)",
			jAnd(jOr(jC("PHY131H1"), jC("PHY151H1")), jOr(jC("PHY132H1"), jC("PHY152H1"))),
			[]string{"PHY151H1, PHY152H1 recommended"}},
		{"0.5 credit from MAT223H1/ MAT240H1 (( MAT223H1) recommended)",
			`{"type":"credits","amount":0.5,"courses":["MAT223H1","MAT240H1"]}`,
			[]string{"( MAT223H1) recommended"}},
		{"HMB265H1/ BIO260H1, HMB421H1/ HMB441H1 (Recommended: HMB421H1)",
			jAnd(jOr(jC("HMB265H1"), jC("BIO260H1")), jOr(jC("HMB421H1"), jC("HMB441H1"))),
			[]string{"Recommended: HMB421H1"}},
		{"1.0 credit of any APM/MAT 400-level courses, MAT401H1/ MAT402H1 recommended",
			`{"type":"credits","amount":1,"designators":["APM","MAT"],"levels":[400]}`,
			[]string{"MAT401H1/ MAT402H1 recommended"}},
		{"[ PHY131H1, PHY132H1 recommended]", "null", []string{"PHY131H1, PHY132H1 recommended"}},
		{"(( STA257H1, STA261H1) recommended)", "null", []string{"( STA257H1, STA261H1) recommended"}},
		{"CSC263H1/ CSC265H1 ( CSC265H1 is recommended)", jOr(jC("CSC263H1"), jC("CSC265H1")), []string{"CSC265H1 is recommended"}},
		{"( STA257H1, STA261H1)/ ( STA237H1, STA238H1); while either pair of courses is accepted, we strongly recommend ( STA257H1, STA261H1)",
			jOr(jAnd(jC("STA257H1"), jC("STA261H1")), jAnd(jC("STA237H1"), jC("STA238H1"))),
			[]string{"while either pair of courses is accepted, we strongly recommend ( STA257H1, STA261H1)"}},
		{"BIO120H1/ BIO130H1 (may be taken in Year 1), BIO220H1", jAnd(jOr(jC("BIO120H1"), jC("BIO130H1")), jC("BIO220H1")),
			[]string{"may be taken in Year 1"}},
		{"( CHM220H1/ CHM222H1 is strongly recommended.)", "null", []string{"CHM220H1/ CHM222H1 is strongly recommended"}},
		{"WGS160Y1 (normally taken in first year)", jC("WGS160Y1"), []string{"normally taken in first year"}},
		{"CSC271H1 (first offering in 2025-26)", jC("CSC271H1"), []string{"first offering in 2025-26"}},
		{"EAS103H1 and EAS105H1, normally taken in the first year;", jAnd(jC("EAS103H1"), jC("EAS105H1")), []string{"normally taken in the first year"}},
		{"BIO120H1; plus 2.5 first year Science credits ( GGR101H1; CHM135H1, CHM136H1 recommended)",
			jAnd(jC("BIO120H1"), jT("other", "2.5 first year Science credits")), []string{"GGR101H1, CHM135H1, CHM136H1 recommended"}},
		{"3.0 SOC credits, which includes at least 1.0 SOC credit at the 300-level, i.e., 2.0 SOC credits at the 200 OR 300-level and 1.0 SOC credit at the 300-level",
			jAnd(`{"type":"credits","amount":3,"designators":["SOC"]}`, `{"type":"credits","amount":1,"designators":["SOC"],"levels":[300]}`),
			nil},
		{"Any two of: ENV450H1, FOR310H1*/ GGR310H1*, FOR410H1 (* if not taken for the requirement immediately above)",
			jChoose(2, jC("ENV450H1"), jOr(jC("FOR310H1"), jC("GGR310H1")), jC("FOR410H1")),
			[]string{"if not taken for the requirement immediately above"}},
		{"CSC457H1/ CSC458H1 (if not taken in list 1), CSC324H1", jAnd(jOr(jC("CSC457H1"), jC("CSC458H1")), jC("CSC324H1")), []string{"if not taken in list 1"}},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			got, notes := parseCompletionText(tt.src)
			assertJSON(t, got, tt.want)
			if !reflect.DeepEqual(notes, tt.notes) {
				t.Errorf("notes %q, want %q", notes, tt.notes)
			}
		})
	}
}

// Course text never sees the completion extensions: the same wording parses
// as it always has.
func TestCompletionGrammarGated(t *testing.T) {
	tests := []struct{ src, want string }{
		{"2.0 credits from: SLA117H1; SLA118H1",
			jAnd(`{"type":"credits","amount":2,"courses":["SLA117H1"]}`, jC("SLA118H1"))},
		{"Geography: JGE321H1", jT("other", "Geography: JGE321H1")},
		{"1.0 credit from APM346H1/ PHY 300-level courses",
			jOr(`{"type":"credits","amount":1,"courses":["APM346H1"]}`, jT("other", "PHY 300-level courses"))},
		{"All of: LIN101H1, LIN102H1", jAnd(jT("other", "All of: LIN101H1"), jC("LIN102H1"))},
		{"CHM135H1(recommended)/ CHM151Y1", jOr(jAnd(jC("CHM135H1"), jT("other", "recommended")), jC("CHM151Y1"))},
		// ";" doesn't make a pool a comma list, so "/" still separates its items.
		// Constructed from ASMAJ0482's line, rewritten as a count; no corpus text
		// depends on the gate.
		{"Two of HMB430H1/ HMB470H1/ HMB472H1; JPM400Y1",
			jAnd(jChoose(2, jC("HMB430H1"), jC("HMB470H1"), jC("HMB472H1")), jC("JPM400Y1"))},
	}
	for _, tt := range tests {
		s, _ := normalize(tt.src)
		assertJSON(t, parseText(s), tt.want)
	}

	// ParseRequisite end to end, pinned as it parses today: each of these
	// shapes reads differently in completion text.
	requisites := []struct{ src, want string }{
		// CRI345H1's prerequisite: a code list and a wildcard stay two nodes
		// (completion: one union pool).
		{"1.0 credit from:  CRI215H1,  CRI210H1,  SOC212H1,  SOC313H1,  HIS268H1, any HIS 300+ level course.  Any course combination from this list is acceptable.",
			jAnd(`{"type":"credits","amount":1,"courses":["CRI215H1","CRI210H1","SOC212H1","SOC313H1","HIS268H1"]}`,
				`{"type":"credits","amount":0.5,"designators":["HIS"],"levels":[300,400]}`)},
		// "and" before the last course ends the list (completion: it is the last item).
		{"1.0 credit from HMU111H1, HMU126H1, TMU130H1 and TMU131H1",
			jAnd(`{"type":"credits","amount":1,"courses":["HMU111H1","HMU126H1","TMU130H1"]}`, jC("TMU131H1"))},
		// A plain "or" doesn't close the list (completion: LIN351H1 is outside it).
		{"1.0 credit from LIN228H1, LIN229H1 or LIN241H1, LIN351H1",
			`{"type":"credits","amount":1,"courses":["LIN228H1","LIN229H1","LIN241H1","LIN351H1"]}`},
		// A pool of alternative groups is text (completion: their "or").
		{"1.0 credit from ( MAT235H1, MAT236H1)/ MAT235Y1/ MAT237Y1",
			jT("other", "1.0 credit from ( MAT235H1, MAT236H1)/ MAT235Y1/ MAT237Y1")},
		// A pool of only filters is text (completion: the filter).
		{"One of: any 300-level CSC course", jT("other", "One of: any 300-level CSC course")},
		// A Y course after "/" in brackets is an alternative to the second H
		// course only (completion: to the pair).
		{"( MAT148H1, MAT149H1/ MAT137Y1)", jAnd(jC("MAT148H1"), jOr(jC("MAT149H1"), jC("MAT137Y1")))},
		// A pool of groups stays text up to ";" (completion: and of the groups).
		{"2.0 credits from ( BIO120H1, BIO130H1)/ BIO150Y1; ( CHM135H1, CHM136H1)/ CHM151Y1",
			jAnd(jT("other", "2.0 credits from ( BIO120H1, BIO130H1)/ BIO150Y1"), jOr(jAnd(jC("CHM135H1"), jC("CHM136H1")), jC("CHM151Y1")))},
	}
	for _, tt := range requisites {
		assertJSON(t, ParseRequisite(tt.src).Req, tt.want)
	}
}

// jPairCap builds a cap on listed courses: credits{courses, max_amount}.
func jPairCap(most float64, codes ...string) string {
	b, _ := json.Marshal(codes)
	return fmt.Sprintf(`{"type":"credits","courses":%s,"max_amount":%g}`, b, most)
}

// parseExpr lexes and parses src without post-processing.
func parseExpr(src string) *types.ReqNode {
	return newParser(src, lex(src)).parseAll()
}
