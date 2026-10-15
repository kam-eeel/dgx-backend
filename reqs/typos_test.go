package reqs

import "testing"

// Misprinted course codes: repaired only where the repair can't be anything
// else, otherwise left alone (and so reviewed).
func TestRepairCodes(t *testing.T) {
	tests := []struct{ in, want string }{
		// Missing campus digit on a St. George-style code.
		{"(MAT148H,  MAT149H1/  MAT137Y1)", "(MAT148H1, MAT149H1/ MAT137Y1)"},
		{"PSL300H1, PSL301H", "PSL300H1, PSL301H1"},
		{"LAS200Y/( LAS200H1,  LAS201H1)", "LAS200Y1/( LAS200H1, LAS201H1)"},
		{"MAT159H1/ MAT157Y recommended", "MAT159H1/ MAT157Y1 recommended"},
		// Doubled campus digit.
		{"AFR370H1, HIS322H11, HIS342H11", "AFR370H1, HIS322H1, HIS342H1"},
		// ACORN session letter glued on.
		{"BMS301H1S LEC0201 20261.", "BMS301H1 LEC0201 20261."},
		{"WGS160Y1Y or WGS271Y1Y", "WGS160Y1 or WGS271Y1"},
		{"SOC386H1F (2016/2017)", "SOC386H1 (2016/2017)"},
		// "-series": one course of that designator and level.
		{"PSL301H1, MAT100-series", "PSL301H1, a 100-level MAT course"},
		{"MAT100-series/PHY100-series", "a 100-level MAT course/a 100-level PHY course"},
		{"a MAT100-series course", "a 100-level MAT course"},

		// Left alone.
		{"(MAT1581, MAT159H1)", "(MAT1581, MAT159H1)"},             // H or Y? Same shape as LEC0101.
		{"CSC454H1/ CSC2527H", "CSC454H1/ CSC2527H"},               // graduate code
		{"VPMB76", "VPMB76"},                                       // truncated
		{"HIS322H15", "HIS322H15"},                                 // two different digits: which one?
		{"MATA22H", "MATA22H"},                                     // UTSC-style code: not St. George
		{"two MAT100-series courses", "two MAT100-series courses"}, // how many?
		{"An approved 300-series science course", "An approved 300-series science course"},
		{"Active in ASMAJ1618, ASSPE1165 or ASFOC2015D", "Active in ASMAJ1618, ASSPE1165 or ASFOC2015D"},
		{"Fall 2022 (LEC0101), Winter 2024", "Fall 2022 (LEC0101), Winter 2024"},
		{"SPH4U Physics; MCV4U Calculus", "SPH4U Physics; MCV4U Calculus"},
		{"CSC148H1/ CSC148H5", "CSC148H1/ CSC148H5"},
	}
	for _, tt := range tests {
		if got, _ := normalize(tt.in); got != tt.want {
			t.Errorf("normalize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRepairedCodesParse(t *testing.T) {
	mat100 := `{"type":"credits","amount":0.5,"designators":["MAT"],"levels":[100]}`
	phy100 := `{"type":"credits","amount":0.5,"designators":["PHY"],"levels":[100]}`
	tests := []struct{ raw, want string }{
		{"LAS200Y/( LAS200H1,  LAS201H1)", jOr(jC("LAS200Y1"), jAnd(jC("LAS200H1"), jC("LAS201H1")))},
		{"BCH210H1/ BCH242Y1, MAT100-series/PHY100-series,  PSL300H1, PSL301H",
			jAnd(jOr(jC("BCH210H1"), jC("BCH242Y1")), jOr(mat100, phy100), jC("PSL300H1"), jC("PSL301H1"))},
		{"14.0 credits and 1.0 credit from the following:  HIS319H1,  HIS322H11,  HIS342H11",
			jAnd(`{"type":"credits","amount":14}`, `{"type":"credits","amount":1,"courses":["HIS319H1","HIS322H1","HIS342H1"]}`)},
		{"WGS160Y1Y or  WGS271Y1Y", jOr(jC("WGS160Y1"), jC("WGS271Y1"))},
		// A graduate code can be checked against a transcript, as in exclusions.
		{"CSC454H1/ CSC2527H", jOr(jC("CSC454H1"), jC("CSC2527H"))},
		// Unrepairable codes stay text.
		{"MAT246H1/ (MAT1581,  MAT159H1)", jOr(jC("MAT246H1"), jAnd(jT("other", "MAT1581"), jC("MAT159H1")))},
		{"CSC454H1/ CSC2527H recommended", "null"},
	}
	for _, tt := range tests {
		assertJSON(t, ParseRequisite(tt.raw).Req, tt.want)
	}
}

func TestRepairedExclusionCodes(t *testing.T) {
	tests := []struct{ raw, items string }{
		{"SOC386H1F (2016/2017),  SOC323H1", `[{"courses":["SOC386H1"],"note":"(2016/2017)"},{"courses":["SOC323H1"]}]`},
		{"PSL301H, MAT148H", `[{"courses":["PSL301H1"]},{"courses":["MAT148H1"]}]`},
		{"VPMB76", `[{"kind":"other","text":"VPMB76"}]`},
	}
	for _, tt := range tests {
		assertJSON(t, ParseExclusions(tt.raw).Items, tt.items)
	}
}
