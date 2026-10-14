package reqs

import "testing"

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
