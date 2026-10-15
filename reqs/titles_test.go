package reqs

import "testing"

// Names and subject lists with commas, "and", "or" or "/" inside stay one text
// node; real operators between requirements are untouched.
func TestTitleRuns(t *testing.T) {
	credits := func(amount string) string { return `{"type":"credits","amount":` + amount + `}` }
	tests := []struct{ name, raw, want string }{
		// series and pairs of capitalised names
		{"series after text", "10.0 credits, approval of the Centre for Drama, Theatre and Performance Studies (CDTPS).",
			jAnd(credits("10"), jT("permission", "approval of the Centre for Drama, Theatre and Performance Studies (CDTPS)"))},
		{"series with oxford comma", "ANT207H1 and 1.0 credit at the 300-level from Group C: Society, Culture, and Language",
			jAnd(jC("ANT207H1"), `{"type":"credits","amount":1,"levels":[300]}`,
				jT("other", "from Group C: Society, Culture, and Language"))},
		{"series ends before a course", "Any 300 or 400 level course in Society, Culture and Language or  INS350H1 or permission of the instructor",
			jOr(jAnd(`{"type":"credits","amount":0.5,"levels":[300,400]}`, jT("other", "in Society, Culture and Language")),
				jC("INS350H1"), jT("permission", "permission of the instructor"))},
		{"', or' after a series is an operator",
			"0.5 credit at the 300-level from Anthropology Group C: Society, Culture, and Language, or Near and Middle Eastern Civilization or Jewish Studies",
			jOr(jAnd(`{"type":"credits","amount":0.5,"designators":["ANT"],"levels":[300]}`,
				jT("other", "Group C: Society, Culture, and Language")),
				jT("other", "Near and Middle Eastern Civilization or Jewish Studies"))},
		{"pairs and series chain", "1.5 credits from courses that count toward the History and Philosophy of Science (HPS), Science, Technology and Society (STS), History, or Philosophy programs of study",
			jAnd(credits("1.5"), jT("other", "that count toward the History and Philosophy of Science (HPS), Science, Technology and Society (STS), History, or Philosophy programs of study"))},
		{"lower-case head noun ends the last name", "a 0.5 credit from HPS or a course required for the Science, Technology, and Society minor",
			jOr(`{"type":"credits","amount":0.5,"designators":["HPS"]}`,
				jT("membership", "a course required for the Science, Technology, and Society minor"))},
		{"programming languages", "CSC209H1/ proficiency in C, C++, or Fortran",
			jOr(jC("CSC209H1"), jT("placement", "proficiency in C, C++, or Fortran"))},
		{"high-school course names", "MCV4U Calculus & Vectors; SPH4U Physics / SES4U Earth and Space Science",
			jAnd(jT("preparation", "MCV4U Calculus & Vectors"), jT("preparation", "SPH4U Physics / SES4U Earth and Space Science"))},

		// subject lists after a credit amount
		{"lower-case subject series", "1.0 credit in political theory, history of science, or intellectual history",
			jAnd(credits("1"), jT("other", "in political theory, history of science, or intellectual history"))},
		{"subject pair", "1.0 credit in European or Asian history",
			jAnd(credits("1"), jT("other", "in European or Asian history"))},
		{"subject lists between slashes", "UNI255H1/1.0 credit on the politics of 20th century Europe, U.S., or Canada/1.0 credit on gender or sexuality/permission of the instructor",
			jOr(jC("UNI255H1"),
				jAnd(credits("1"), jT("other", "on the politics of 20th century Europe, U.S., or Canada")),
				jAnd(credits("1"), jT("other", "on gender or sexuality")),
				jT("permission", "permission of the instructor"))},
		{"and/or between subjects", "2.0 credits in Political Theory and/or Philosophy",
			jAnd(credits("2"), jT("other", "in Political Theory / Philosophy"))},

		// bracketed names
		{"bracketed pair", "Intermediate-level Hebrew (Modern or Biblical)", jT("other", "Intermediate-level Hebrew (Modern or Biblical)")},
		{"bracketed series", "ANT207H1 and 1.0 credit from Group C (Society, Culture and Language)",
			jAnd(jC("ANT207H1"), credits("1"), jT("other", "from Group C (Society, Culture and Language)"))},
		{"bracketed list with semicolon", "Foundational course from participating programs (Buddhism, Psychology and Mental Health; Critical Studies in Equity and Solidarity)",
			jT("other", "Foundational course from participating programs (Buddhism, Psychology and Mental Health; Critical Studies in Equity and Solidarity)")},
		{"bracketed sessions", "Attendance at a session in May (Summer Session projects) or September (Fall/Winter Session projects)",
			jT("other", "Attendance at a session in May (Summer Session projects) or September (Fall/Winter Session projects)")},
		{"plural marker", "At least 1.0 credit European History course(s)",
			jAnd(credits("1"), jT("other", "European History course(s)"))},

		// real operators stay operators
		{"courses", "CSC148H1, MAT137Y1 and STA247H1", jAnd(jC("CSC148H1"), jC("MAT137Y1"), jC("STA247H1"))},
		{"lower-case end", "Admission to Munk One, permission of the instructor",
			jAnd(jT("membership", "Admission to Munk One"), jT("permission", "permission of the instructor"))},
		{"requirement after a name", "Specialist or Major in Drama, Independent Study Application Form",
			jAnd(`{"type":"program","text":"Specialist or Major in Drama"}`, jT("permission", "Independent Study Application Form"))},
		{"series without a conjunction", "CSC108H1/ Statistics Canada, Data Science",
			jAnd(jOr(jC("CSC108H1"), jT("other", "Statistics Canada")), jT("other", "Data Science"))},
		{"permission is not a subject", "1.0 credit in Political Theory or permission of the instructor",
			jOr(jAnd(credits("1"), jT("other", "in Political Theory")), jT("permission", "permission of the instructor"))},
		{"non-subject alternative", "1.0 credit in political theory or demonstrated interest",
			jOr(jAnd(credits("1"), jT("other", "in political theory")), jT("other", "demonstrated interest"))},
		{"designators are not names", "0.5 credit at the 200+ level in SOC or POL or Women's Studies",
			jOr(`{"type":"credits","amount":0.5,"designators":["SOC","POL"],"levels":[200,300,400]}`, jT("other", "Women's Studies"))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertJSON(t, ParseRequisite(tt.raw).Req, tt.want)
		})
	}
}
