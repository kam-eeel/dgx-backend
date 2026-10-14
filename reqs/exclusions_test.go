package reqs

import "testing"

func TestParseExclusions(t *testing.T) {
	tests := []struct{ name, raw, items, notes string }{
		{"plain list", "SMC440Y1,  SMC440H1", `[{"codes":["SMC440Y1"]},{"codes":["SMC440H1"]}]`, `null`},
		{"MAT148H1", "MAT137Y1/  MAT137Y5/ ( MAT137H5,  MAT139H5)/ ( MATA30H3/  MATA31H3)",
			`[{"codes":["MAT137Y1"]},{"codes":["MAT137Y5"]},{"codes":["MAT137H5","MAT139H5"]},{"codes":["MATA30H3"]},{"codes":["MATA31H3"]}]`, `null`},
		{"DRM200Y1", "( DRS121H5/  DRE201H5,  DRS122H5), ( DRS221H5,  DRS222H5)",
			`[{"codes":["DRS121H5","DRS122H5"]},{"codes":["DRE201H5","DRS122H5"]},{"codes":["DRS221H5","DRS222H5"]}]`, `null`},
		{"RSM465H1", "RSM410H1 (Special Topics in Management: Incentives and Motivation), offered in Winter 2010/Winter 2011/Winter 2012",
			`[{"codes":["RSM410H1"],"note":"(Special Topics in Management: Incentives and Motivation), offered in Winter 2010/Winter 2011/Winter 2012"}]`, `null`},
		{"SLA268H1", "CCR199H1 (First-Year Seminar: The Cossacks), offered in Winter 2012, Winter 2013, Fall 2013, Fall 2015, Fall 2016, and Fall 2017",
			`[{"codes":["CCR199H1"],"note":"(First-Year Seminar: The Cossacks), offered in Winter 2012, Winter 2013, Fall 2013, Fall 2015, Fall 2016, and Fall 2017"}]`, `null`},
		{"HPS330H1 nested parens", "HPS301H1 (Topics in the History of Science: From Ordering Nature to the Anthropocene (1400-present)), offered in Fall 2024 and Fall 2025.",
			`[{"codes":["HPS301H1"],"note":"(Topics in the History of Science: From Ordering Nature to the Anthropocene (1400-present)), offered in Fall 2024 and Fall 2025"}]`, `null`},
		{"POL338H1 section codes", "POL380H1 (Topics in International Politics: Queer IR), offered in Winter 2020 (L5101), Winter 2021 (L0201) and Winter 2022 (L0201)",
			`[{"codes":["POL380H1"],"note":"(Topics in International Politics: Queer IR), offered in Winter 2020 (L5101), Winter 2021 (L0201) and Winter 2022 (L0201)"}]`, `null`},
		{"POL470H1 qualifier then next course", "POL410H1 (Topics in Comparative Politics III - Media and Politics) Taken in Fall 2021 (LEC0101), Fall 2022 (LEC0101),  POL369Y5",
			`[{"codes":["POL410H1"],"note":"(Topics in Comparative Politics III - Media and Politics) Taken in Fall 2021 (LEC0101), Fall 2022 (LEC0101)"},{"codes":["POL369Y5"]}]`, `null`},
		{"CSC420H1", "CSC420H5. NOTE: Students not enrolled in the Computer Science Major or Specialist program at A&S, UTM, or UTSC, or the Data Science Specialist at A&S, are limited to a maximum of 1.5 credits in 300-/400-level CSC/ECE courses.",
			`[{"codes":["CSC420H5"]}]`,
			`["NOTE: Students not enrolled in the Computer Science Major or Specialist program at A&S, UTM, or UTSC, or the Data Science Specialist at A&S, are limited to a maximum of 1.5 credits in 300-/400-level CSC/ECE courses"]`},
		{"notes only", "NOTE: Students not enrolled in the Computer Science Major are limited to a maximum of 1.5 credits.",
			`[]`, `["NOTE: Students not enrolled in the Computer Science Major are limited to a maximum of 1.5 credits"]`},
		{"FSL110Y1", "FSL102H1,  FSL120H1,  FSL121Y1,  FSL122H1, FSL 200+ level courses, placement in  FSL120H1,  FSL122H1 or FSL200+ level courses based on the results of the French Placement Test.",
			`[{"codes":["FSL102H1"]},{"codes":["FSL120H1"]},{"codes":["FSL121Y1"]},{"codes":["FSL122H1"]},
			  {"kind":"other","text":"FSL 200+ level courses"},
			  {"kind":"placement","text":"placement in FSL120H1"},
			  {"kind":"placement","text":"FSL200+ level courses based on the results of the French Placement Test"}]`, `null`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := ParseExclusions(tt.raw)
			if e.Raw != tt.raw {
				t.Errorf("raw not preserved")
			}
			assertJSON(t, e.Items, tt.items)
			assertJSON(t, e.Notes, tt.notes)
		})
	}
}
