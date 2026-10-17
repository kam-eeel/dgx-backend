package grammar

import (
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
		s, _ := Normalize(tt.src)
		assertJSON(t, ParseText(s), tt.want)
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

// parseExpr lexes and parses src without post-processing.
func parseExpr(src string) *types.ReqNode {
	return NewParser(src, Lex(src)).ParseAll()
}
