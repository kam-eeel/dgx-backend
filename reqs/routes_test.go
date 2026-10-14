package reqs

import (
	"testing"

	"degree-explorer/types"
)

// Sentences that read like advice but change what meets the requirement.
func TestRoutes(t *testing.T) {
	conc := func(node string) string { return node[:len(node)-1] + `,"concurrent":true}` }
	tests := []struct{ name, raw, want string }{
		{"substitute for one course",
			"CSC263H1/ CSC265H1, STA237H1/ STA247H1. Notes: students enrolled in ASMAJ1446A who have completed at least 9.0 credits may substitute CSC111H1 for CSC263H1 and STA220H1/ PSY201H1 for STA237H1.",
			jAnd(
				jOr(jC("CSC263H1"), jAnd(jP("enrolled in ASMAJ1446A", "ASMAJ1446A"), `{"type":"credits","amount":9}`, jC("CSC111H1")), jC("CSC265H1")),
				jOr(jC("STA237H1"), jAnd(jP("enrolled in ASMAJ1446A", "ASMAJ1446A"), `{"type":"credits","amount":9}`, jOr(jC("STA220H1"), jC("PSY201H1"))), jC("STA247H1")))},
		{"more needed on one path",
			"minimum of 70% in CSC240H1/ (minimum of 85% in CSC236H1, MAT377H1/ STA237H1) Notes: Students who have completed CSC240H1 must enrol in MAT377H1/ STA237H1 concurrently with CSC265H1, if they have not already completed one of those courses.",
			jOr(jAnd(jCG("CSC240H1", 70), conc(jOr(jC("MAT377H1"), jC("STA237H1")))),
				jAnd(jCG("CSC236H1", 85), jOr(jC("MAT377H1"), jC("STA237H1"))))},
		{"concurrent fallback, any one of a list",
			"1.0 ENG credit or any 4.0 credits. Students without these prerequisites may enrol in a 200-level course if they are concurrently enrolled in ENG110Y1, ENG140Y1 or ENG150Y1.",
			jOr(`{"type":"credits","amount":1,"designators":["ENG"]}`, `{"type":"credits","amount":4}`,
				conc(jOr(jC("ENG110Y1"), jC("ENG140Y1"), jC("ENG150Y1"))))},
		{"another way with permission",
			"ECO200Y1/ ECO204Y1. Note: Students with ECO101H1(63%), plus a full-year of statistics, and who are enrolled in the Drama Major may take this course with Permission of the Instructor.",
			jOr(jC("ECO200Y1"), jC("ECO204Y1"),
				jAnd(jCG("ECO101H1", 63), jT("other", "a full-year of statistics"), jP("enrolled in the Drama Major"), jT("permission", "Permission of the Instructor")))},
		{"acceptable with permission",
			"BIO120H1, BCH242Y1 (Note: BCH210H1 may be acceptable with permission of instructor)",
			jAnd(jC("BIO120H1"), jOr(jC("BCH242Y1"), jAnd(jC("BCH210H1"), jT("permission", "permission of instructor"))))},
		{"acceptable after an alternative list",
			"BCH311H1/ MGY311Y1 (Note: CSB349H1 is acceptable with permission of the instructor)",
			jOr(jC("BCH311H1"), jC("MGY311Y1"), jAnd(jC("CSB349H1"), jT("permission", "permission of the instructor")))},
		{"may be taken concurrently",
			"MAT237Y1/ MAT257Y1 ( MAT257Y1 can be taken concurrently)",
			jOr(jC("MAT237Y1"), conc(jC("MAT257Y1")))},
		{"program only if",
			"Active in ASMAJ1618 only if enrolling in the Fall/Winter session.",
			jOr(jP("Active in ASMAJ1618", "ASMAJ1618"), jT("other", "only if enrolling in the Fall/Winter session"))},
		// Unchanged: advice that doesn't change what meets the field, and a
		// substitution for a course the field doesn't name.
		{"plain advice stays a note",
			"CSC236H1. Students who have completed additional Mathematics courses may submit a prerequisite waiver request.",
			jC("CSC236H1")},
		{"substitute for a course not in the field",
			"CSC236H1. Notes: students in ASMAJ1446A may substitute CSC111H1 for CSC263H1.",
			jC("CSC236H1")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertJSON(t, ParseRequisite(tt.raw).Req, tt.want)
		})
	}
}

func TestRouteNotesKept(t *testing.T) {
	raw := "BCH311H1 (Note: CSB349H1 is acceptable with permission of the instructor)"
	assertJSON(t, ParseRequisite(raw).Notes, `["Note: CSB349H1 is acceptable with permission of the instructor"]`)
}

func TestTimingExclusions(t *testing.T) {
	e := ParseExclusions("FSL321Y1. May not be taken concurrently with FSL312H1, FSL313H1.")
	assertJSON(t, e.Items, `[{"codes":["FSL321Y1"]},{"codes":["FSL312H1"],"concurrent":true},{"codes":["FSL313H1"],"concurrent":true}]`)
	if e.Notes != nil {
		t.Errorf("notes = %q, want none", e.Notes)
	}
	if timingItems("May not be taken concurrently with another research project") != nil {
		t.Error("a list that isn't course codes became items")
	}
}

func TestRepairCatalogCodes(t *testing.T) {
	known := map[string]bool{"MAT158H1": true, "SPA258H1": true, "ECO100H1": true, "ECO100Y1": true, "MAT100H1": true}
	tests := []struct{ in, want string }{
		{"(MAT1581, MAT159H1)", "(MAT158H1, MAT159H1)"},
		{"SPA219Y1/SPA258 or Spanish placement test", "SPA219Y1/SPA258H1 or Spanish placement test"},
		{"ECO100 or ECO101H1", "ECO100 or ECO101H1"},               // H or Y: both exist
		{"MAT100-series", "MAT100-series"},                         // a phrase, not a code
		{"consult the MAT158 Office", "consult the MAT158 Office"}, // a name, not a course
		{"LEC0101, CSC999", "LEC0101, CSC999"},                     // no such course
		{"MAT158H1, ASMAJ1618", "MAT158H1, ASMAJ1618"},             // already codes
	}
	for _, tt := range tests {
		if got, _ := repairCatalogCodes(tt.in, known); got != tt.want {
			t.Errorf("repairCatalogCodes(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestLink(t *testing.T) {
	ast := &types.Course{Code: "AST430H1",
		Prerequisites: ParseRequisite("AST320H1. Course should not be taken concurrently with AST425Y1.")}
	mat := &types.Course{Code: "MAT309H1", Prerequisites: ParseRequisite("MAT246H1/ (MAT1581, MAT159H1)")}
	courses := []*types.Course{ast, mat,
		{Code: "MAT158H1"}, {Code: "MAT159H1"}, {Code: "MAT246H1"}, {Code: "AST320H1"}, {Code: "AST425Y1"}}
	Link(courses, nil)

	assertJSON(t, ast.Exclusions, `{"raw":"","items":[{"codes":["AST425Y1"],"concurrent":true}],"notes":["Course should not be taken concurrently with AST425Y1"]}`)
	assertJSON(t, mat.Prerequisites.Req, jOr(jC("MAT246H1"), jAnd(jC("MAT158H1"), jC("MAT159H1"))))
	if mat.Prerequisites.Raw != "MAT246H1/ (MAT1581, MAT159H1)" {
		t.Errorf("raw changed: %q", mat.Prerequisites.Raw)
	}
}
