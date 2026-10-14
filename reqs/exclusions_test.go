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
			  {"designators":["FSL"],"levels":[200,300,400]},
			  {"kind":"placement","text":"placement in FSL120H1, FSL122H1 or FSL 200+ level courses based on the results of the French Placement Test"}]`, `null`},

		// Filter items: "excluded if the student took any course matching".
		{"glued level", "FSL221Y1, FSL300+ level courses", `[{"codes":["FSL221Y1"]},{"designators":["FSL"],"levels":[300,400]}]`, `null`},
		{"single level", "FSL375H1, FSL 400-level courses.", `[{"codes":["FSL375H1"]},{"designators":["FSL"],"levels":[400]}]`, `null`},
		{"any designator course", "FSL271H1, any FRE course", `[{"codes":["FSL271H1"]},{"designators":["FRE"]}]`, `null`},
		{"any level designator course", "Any 300+ level STA course,  CSC311H1", `[{"designators":["STA"],"levels":[300,400]},{"codes":["CSC311H1"]}]`, `null`},
		{"except list", "Any CSC course except  CSC104H1,  CSC196H1,  CSC197H1",
			`[{"designators":["CSC"],"exclude":["CSC104H1","CSC196H1","CSC197H1"]}]`, `null`},
		{"except list with transfer credits", "JCC250H1; Any CSC course except  CSC196H1,  CSC197H1, AP, IB, CAPE or GCE Transfer Credits.",
			`[{"codes":["JCC250H1"]},{"designators":["CSC"],"exclude":["CSC196H1","CSC197H1"],"note":"except AP, IB, CAPE or GCE Transfer Credits"}]`, `null`},
		{"bracketed except, taken previously or concurrently", "Any PHY course taken previously or concurrently (except  PHY196H1/  PHY197H1)",
			`[{"designators":["PHY"],"exclude":["PHY196H1","PHY197H1"]}]`, `null`},
		{"level then designator", "Any 300-level PSL course taken previously or concurrently", `[{"designators":["PSL"],"levels":[300]}]`, `null`},
		{"subject names stay text", "any physics course", `[{"kind":"other","text":"any physics course"}]`, `null`},
		{"no designator stays text", "any 300-level course", `[{"kind":"other","text":"any 300-level course"}]`, `null`},
		{"filter with trailing words stays text", "FSL 200+ level courses taken abroad", `[{"kind":"other","text":"FSL 200+ level courses taken abroad"}]`, `null`},
		{"credit amount stays text", "PHL100Y1, or more than 1.0 PHL course", `[{"codes":["PHL100Y1"]},{"kind":"other","text":"more than 1.0 PHL course"}]`, `null`},

		// Codes the splitter used to miss.
		{"also excluded are", "AST101H1. Also excluded are  AST221H1,  AST222H1 if taken previously or concurrently.",
			`[{"codes":["AST101H1"]},{"codes":["AST221H1"]},{"codes":["AST222H1"]}]`, `null`},
		{"bracketed taken previously or concurrently", "AST425Y1 (if taken previously or concurrently)", `[{"codes":["AST425Y1"]}]`, `null`},
		{"or equivalent keeps its qualifier", "( PHY131H1,  PHY132H1 or equivalent, taken previously or concurrently)",
			`[{"codes":["PHY131H1","PHY132H1"],"note":"or equivalent, taken previously or concurrently"}]`, `null`},
		{"mid-sentence NOTE", "CSC347H5,  CSCD27H3 NOTE: Students not enrolled in the Computer Science Major are limited to 1.5 credits.",
			`[{"codes":["CSC347H5"]},{"codes":["CSCD27H3"]}]`, `["NOTE: Students not enrolled in the Computer Science Major are limited to 1.5 credits"]`},
		{"graduate codes", "PHM320H1, PHM1124H", `[{"codes":["PHM320H1"]},{"codes":["PHM1124H"]}]`, `null`},
		{"graduate code with bracketed qualifier", "CRI3256H (when offered as a joint course)", `[{"codes":["CRI3256H"],"note":"(when offered as a joint course)"}]`, `null`},
		{"section and session codes", "BMS301H1S LEC0201 20261.", `[{"codes":["BMS301H1"],"note":"LEC0201 20261"}]`, `null`},
		{"if clause", "LIN479H5 or  LINC61H3 if the same language was analyzed.",
			`[{"codes":["LIN479H5"]},{"codes":["LINC61H3"],"note":"if the same language was analyzed"}]`, `null`},
		{"unbracketed title", "WGS335H1 Women and Revolution in the Middle East", `[{"codes":["WGS335H1"],"note":"Women and Revolution in the Middle East"}]`, `null`},
		{"title after colon", "WGS335H1: The Politics of Girlhood", `[{"codes":["WGS335H1"],"note":"The Politics of Girlhood"}]`, `null`},
		{"title after dash", "RSM418H1 - Catastrophic Failure in Organizations", `[{"codes":["RSM418H1"],"note":"Catastrophic Failure in Organizations"}]`, `null`},
		{"lowercase prose is not a title", "CSC108H1 taught online", `[{"kind":"other","text":"CSC108H1 taught online"}]`, `null`},
		{"missing separator after offering term", "RLG239H1 (Special Topics: Religion), offered in Winter 2026   PHL235H1: Philosophy of Religion",
			`[{"codes":["RLG239H1"],"note":"(Special Topics: Religion), offered in Winter 2026"},{"codes":["PHL235H1"],"note":"Philosophy of Religion"}]`, `null`},
		{"no separator after prose", "ECO250Y0 (Summer 2023) has an exclusion to  ECO225H1",
			`[{"codes":["ECO250Y0","ECO225H1"],"note":"(Summer 2023) has an exclusion to"}]`, `null`},
		{"title taken under course", "Urban Studio: Public Participation taken under  URB432H1 (offered in Fall 2022)/  INI432H1",
			`[{"codes":["URB432H1"],"note":"(Urban Studio: Public Participation) (offered in Fall 2022)"},{"codes":["INI432H1"]}]`, `null`},

		// Text that can't be checked: one item per condition, sensibly classified.
		{"or equivalent", "Senior high school German or equivalent.", `[{"kind":"preparation","text":"Senior high school German or equivalent"}]`, `null`},
		{"program", "Not intended for students in a Mathematics Specialist or Major program.",
			`[{"kind":"program","text":"Not intended for students in a Mathematics Specialist or Major program"}]`, `null`},
		{"speakers", "SLA107Y1, native speakers of BCS and heritage speakers of BCS (unless permitted by the instructor).",
			`[{"codes":["SLA107Y1"]},{"kind":"placement","text":"native speakers of BCS and heritage speakers of BCS (unless permitted by the instructor)"}]`, `null`},
		{"advanced speakers", "Advanced speakers of Turkish", `[{"kind":"placement","text":"Advanced speakers of Turkish"}]`, `null`},
		{"permission beats speakers", "Permission of instructor required for native speakers", `[{"kind":"permission","text":"Permission of instructor required for native speakers"}]`, `null`},
		{"school grade", "Grade 8 Hebrew (or Ulpan level 2 in Israel)/  NML255Y1",
			`[{"kind":"preparation","text":"Grade 8 Hebrew or (Ulpan level 2 in Israel)"},{"codes":["NML255Y1"]}]`, `null`},
		{"OAC", "OAC Hebrew/ NML355H1", `[{"kind":"preparation","text":"OAC Hebrew"},{"codes":["NML355H1"]}]`, `null`},
		{"text runs split by a code stay apart", "Native speakers, PRS210Y5, heritage speakers",
			`[{"kind":"placement","text":"Native speakers"},{"codes":["PRS210Y5"]},{"kind":"placement","text":"heritage speakers"}]`, `null`},
		{"text runs split by a sentence stay apart", "Non-science students. Native speakers",
			`[{"kind":"other","text":"Non-science students"},{"kind":"placement","text":"Native speakers"}]`, `null`},

		// Remarks that are not exclusions go to notes.
		{"does not count", "HMU226H1. Does not count as an HMU elective.", `[{"codes":["HMU226H1"]}]`, `["Does not count as an HMU elective"]`},
		{"not recommended", "HMB490Y1, not recommended to take with another full year project course",
			`[{"codes":["HMB490Y1"]}]`, `["not recommended to take with another full year project course"]`},
		{"none", "None", `[]`, `["None"]`},
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
