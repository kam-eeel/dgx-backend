package parser

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"degree-explorer/parser/courses"
	"degree-explorer/parser/programs/completion"
	"degree-explorer/parser/programs/enrolment"
	"degree-explorer/types"
)

func jCr(amount float64) string { return fmt.Sprintf(`{"type":"credits","amount":%g}`, amount) }

func jAdm(text string) string { return jT("admission", text) }

const (
	limitedLead = "This is a limited enrolment program. Students must have completed 4.0 credits and meet the requirements listed below to enrol.\n"
	vmgLead     = "A minimum grade is needed for entry, and this minimum changes each year depending on available spaces and the number of applicants. The following courses must be completed:\n"
	vmgaLead    = "A minimum grade average in required courses is needed for entry, and this minimum changes each year depending on available spaces and the number of applicants. The following courses must be completed:\n"
)

// TestLinkEnrolment resolves program names by title and subject, repairs a
// code against the course list, and drops the program itself from Exclusions.
func TestLinkEnrolment(t *testing.T) {
	var programs []*types.Program
	for _, p := range [][3]string{
		{"ASSPE0652", "Specialist", "History Specialist (Arts Program)"},
		{"ASMAJ0652", "Major", "History Major (Arts Program)"},
		{"ASMAJ1775", "Major", "Anthropology Major (General) (Arts Program)"},
		{"ASMAJ2112", "Major", "Anthropology Major (Society, Culture, and Language) (Arts Program)"},
		{"ASSPE1689", "Specialist", "Computer Science Specialist (Science Program)"},
		{"ASMAJ1689", "Major", "Computer Science Major (Science Program)"},
		{"ASSPE1687", "Specialist", "Data Science Specialist (Science Program)"},
		{"ASMAJ1160", "Major", "Psychology Major (Science Program)"},
		{"ASMIN1160", "Minor", "Psychology Minor (Science Program)"},
		{"ASSPE1160", "Specialist", "Psychology Specialist (Science Program)"},
		{"ASSPE1958", "Specialist", "Psychology Research Specialist - Thesis (Science Program)"},
		{"ASMAJ2082", "Major", "Pharmacology Major (Science Program)"},
		{"ASSPE2082", "Specialist", "Pharmacology Specialist (Science Program)"},
		{"ASMIN0338", "Minor", "Portuguese Minor (Arts Program)"},
		{"ASMAJ0338", "Major", "Portuguese Major (Arts Program)"},
		{"ASSPE0338", "Specialist", "Portuguese Specialist (Arts Program)"},
		{"ASSPE2431", "Specialist", "Management Specialist (BCom)"},
	} {
		programs = append(programs, &types.Program{Code: p[0], Type: p[1], Title: p[2]})
	}
	enrol := map[string]string{
		"ASFOC0652A": "Enrolment in the History Specialist is required.",
		"ASFOC1775A": "Enrolment in the Anthropology Major (General) is required.",
		"ASFOC1689T": "To be admitted to the Technology Leadership Focus, you must:\n- Students who complete the Technology Leadership Focus application and/or interview and are admitted to a Computer Science Specialist or Major or Data Science Specialist program will be considered for admission to the Focus.",
		"ASMIN1551":  limitedLead + "_Special Requirement_\n- Students must be enrolled in the Psychology Minor, Major or Specialist, to be considered for enrolment.",
		"ASMAJ2082":  limitedLead + "Notes:\n- Students cannot combine a Pharmacology Major program with a Pharmacology Major or Specialist program for their degree.",
		"ASCER0338":  "This is an open enrolment Certificate. A student who has completed 4.0 credits may enrol in the Certificate. This Certificate is not open to students enrolled in a Specialist, Major, or Minor in Portuguese.",
		"ASFOC9999A": "Enrolment in the Underwater Basket Weaving Major is required.",
		"ASSPE2038":  limitedLead + "_Completed courses (with minimum grades)_\nThe following courses with the stated minimum grades are required:\n- [ MAT158H1 (55%), MAT159 (55%)]/ MAT157Y1 (55%)",
	}
	byCode := map[string]*types.Program{}
	for code, raw := range enrol {
		p := &types.Program{Code: code, Enrolment: enrolment.Parse(raw)}
		programs = append(programs, p)
		byCode[code] = p
	}
	courses := []*types.Course{{Code: "MAT158H1"}, {Code: "MAT159H1"}, {Code: "MAT157Y1"}}
	Link(courses, programs)

	assertJSON(t, byCode["ASFOC0652A"].Enrolment.Req, jP("History Specialist", "ASSPE0652"))
	assertJSON(t, byCode["ASFOC1775A"].Enrolment.Req, jP("Anthropology Major (General)", "ASMAJ1775"))
	assertJSON(t, byCode["ASFOC1689T"].Enrolment.Req, jOr(
		jP("Computer Science Specialist or Major", "ASSPE1689", "ASMAJ1689"), jP("Data Science Specialist", "ASSPE1687")))
	assertJSON(t, byCode["ASMIN1551"].Enrolment.Req, jAnd(jCr(4),
		jP("Psychology Minor, Major or Specialist", "ASMIN1160", "ASMAJ1160", "ASSPE1160")))
	assertJSON(t, byCode["ASMAJ2082"].Enrolment.Exclusions, `[`+jP("Pharmacology Major or Specialist", "ASSPE2082")+`]`)
	assertJSON(t, byCode["ASCER0338"].Enrolment.Exclusions, `[`+jP("Specialist, Major, or Minor in Portuguese", "ASSPE0338", "ASMAJ0338", "ASMIN0338")+`]`)
	assertJSON(t, byCode["ASFOC9999A"].Enrolment.Req, jT("membership", "Underwater Basket Weaving Major"))

	// "MAT159 (55%)" is MAT159H1; Raw is kept.
	e := byCode["ASSPE2038"].Enrolment
	assertJSON(t, e.Req, jAnd(jCr(4), jOr(jAnd(jCG("MAT158H1", 55), jCG("MAT159H1", 55)), jCG("MAT157Y1", 55))))
	if !strings.Contains(e.Raw, "MAT159 (55%)") {
		t.Errorf("raw changed: %q", e.Raw)
	}
}

// TestLinkEnrolmentAreas resolves restrictions that name a subject by the
// programs' first subject area.
func TestLinkEnrolmentAreas(t *testing.T) {
	var programs []*types.Program
	for _, p := range [][4]string{
		{"ASMAJ1400", "Major", "German Studies Major (Arts Program)", "German"},
		{"ASMIN2453", "Minor", "Business German Minor (Arts Program)", "German"},
		{"ASCER1401", "Certificate", "Certificate in Global German Studies (Category 2)", "German"},
		{"ASMAJ1478", "Major", "Economics Major (Arts Program)", "Economics"},
		{"ASSPE2722", "Specialist", "Financial Economics Specialist (Science Program)", "Economics"},
		{"ASFOC1478A", "Focus", "Focus in Data Analytics (Specialist)", "Economics"},
		{"ASFOC1478B", "Focus", "Focus in Data Analytics (Major)", "Economics"},
		{"ASMIN1551", "Minor", "Environment & Behaviour Minor (Science Program)", "School of the Environment"},
		{"ASMAJ1160", "Major", "Psychology Major (Science Program)", "Psychology"},
		{"ASMAJ2289", "Major", "Statistics Major (Science Program)", "Statistical Sciences"},
	} {
		programs = append(programs, &types.Program{Code: p[0], Type: p[1], Title: p[2], Areas: types.StringList{p[3]}})
	}
	programs[7].Areas = append(programs[7].Areas, "Psychology") // a second area doesn't count
	enrol := map[string]string{
		"ASCER1402": "This is an open enrolment Certificate. A student who has completed 4.0 credits may enrol in the Certificate. This Certificate is not open to students enrolled in a Specialist, Major, or Minor in German.",
		"ASSPE1478": limitedLead + "_Special Requirement_\n• Students enrolled in this program cannot be simultaneously enrolled in any other Economics program.",
		"ASCER1160": "This is an open enrolment Certificate. A student who has completed 4.0 credits may enrol in the Certificate. This Certificate is NOT open to students enrolled in a Psychology Specialist, Research Specialist, Major, or Minor.",
		"ASMIN0160": limitedLead + "Students enrolled in this program cannot be simultaneously enrolled in or complete any Statistics programs; nor the Focus in Data Analytics within the Economics Major or Specialist; nor the Underwater Basket Weaving Major.",
	}
	byCode := map[string]*types.Program{}
	for code, raw := range enrol {
		p := &types.Program{Code: code, Enrolment: enrolment.Parse(raw)}
		programs = append(programs, p)
		byCode[code] = p
	}
	Link(nil, programs)

	assertJSON(t, byCode["ASCER1402"].Enrolment.Exclusions, `[`+jP("Specialist, Major, or Minor in German", "ASMAJ1400", "ASMIN2453")+`]`)
	assertJSON(t, byCode["ASSPE1478"].Enrolment.Exclusions, `[`+jP("other Economics program", "ASMAJ1478", "ASSPE2722")+`]`)
	assertJSON(t, byCode["ASCER1160"].Enrolment.Exclusions, `[`+jP("Psychology Specialist, Research Specialist, Major, or Minor", "ASMAJ1160")+`]`)
	assertJSON(t, byCode["ASMIN0160"].Enrolment.Exclusions, `[`+jP("Statistics", "ASMAJ2289")+`,`+
		jP("Focus in Data Analytics within the Economics Major or Specialist", "ASFOC1478B", "ASFOC1478A")+`,`+
		jT("membership", "Underwater Basket Weaving Major")+`]`)
}

// TestLinkEnrolmentCatalog reads jointly offered courses from the area's
// completion requirements, "or equivalent" from course exclusions, and drops
// a program from Exclusions that the calendar doesn't have.
func TestLinkEnrolmentCatalog(t *testing.T) {
	comp := "1. JEG100H1, JGU216H1; JAV120H1\n2. 0.5 credit from GGR201H1, JIG322H1"
	ggrMajor := &types.Program{Code: "ASMAJ1667", Type: "Major", Title: "Human Geography Major (Arts Program)",
		Areas: types.StringList{"Geography and Planning"}, Completion: completion.Parse(comp),
		Enrolment: enrolment.Parse(limitedLead + "_Variable Minimum Grade_\n" + vmgLead +
			"- 0.5 credit in GGR courses at the 100- or 200-level, with a minimum grade of 67%\n" +
			"Jointly-offered GGR courses will also be considered (e.g. JEG, JGI, JGE).")}
	cert := &types.Program{Code: "ASCER2400", Type: "Certificate", Title: "Certificate in Business Fundamentals (Category 2)",
		Enrolment: enrolment.Parse("This is a limited enrolment certificate for any non-Rotman Commerce student in the Faculty of Arts & Science. A student who has completed 4.0 credits including MGT100H1 or equivalent may enrol in the Certificate.")}
	minor := &types.Program{Code: "ASMIN0160", Type: "Minor", Title: "Applied Data Science Minor (Science Program)",
		Enrolment: enrolment.Parse(limitedLead + "Students enrolled in this program cannot be simultaneously enrolled in or complete any Mathematics Major; nor the Focus in Data Science in Business within the Rotman Commerce specialist programs.")}
	programs := []*types.Program{ggrMajor, cert, minor,
		{Code: "ASMAJ1165", Type: "Major", Title: "Mathematics Major (Science Program)", Areas: types.StringList{"Mathematics"}}}
	courses := []*types.Course{{Code: "MGT100H1", Exclusions: courses.ParseExclusions("RSM100H1,  RSM100Y1")}}
	Link(courses, programs)

	// JAV isn't a Geography designator; JGI, JGE come from the example list.
	assertJSON(t, ggrMajor.Enrolment.Req, jAnd(jCr(4),
		`{"type":"credits","amount":0.5,"designators":["GGR","JEG","JGI","JGE","JGU","JIG"],"levels":[100,200],"min_grade":67}`,
		jAdm("variable minimum grade")))
	assertJSON(t, cert.Enrolment.Req, jAnd(jCr(4), jOr(jC("MGT100H1"), jC("RSM100H1"), jC("RSM100Y1"))))
	assertJSON(t, minor.Enrolment.Exclusions, `[`+jP("Mathematics Major", "ASMAJ1165")+`]`)
	if !slices.Contains(minor.Enrolment.Notes, "Not in the calendar: Focus in Data Science in Business within the Rotman Commerce specialist programs") {
		t.Errorf("notes = %q", minor.Enrolment.Notes)
	}
}

// TestLinkEnrolmentCompletion reads an average over "credits that count toward
// the program" (ASMAJ1618, shortened) as one over the completion rows that claim
// courses: or(average, sentence) while one is text; text with no rows.
func TestLinkEnrolmentCompletion(t *testing.T) {
	enrol := limitedLead + "**For students who have completed 9.0 credits:**\n" +
		"• A minimum grade average of 77% in 3.0 credits that count toward the program, including 1.0 credit from: ETH201H1, ETH210H1, ETH220H1, ETH230H1, PHL265H1, PHL271H1, PHL275H1, and POL200Y1."
	comp := "(7.0 credits)\n3. Third year: TRN303H1, TRN312H1\n4. Fourth year: TRN412H1."
	esl := &types.Program{Code: "ASMAJ1618", Type: "Major", Title: "Ethics, Society, and Law Major (Arts Program)",
		Enrolment: enrolment.Parse(enrol), Completion: completion.Parse(comp)}
	alone := &types.Program{Code: "ASMAJ9999", Type: "Major", Title: "Example Major (Arts Program)", Enrolment: enrolment.Parse(enrol)}
	open := &types.Program{Code: "ASMAJ1618", Type: "Major", Title: "Ethics, Society, and Law Major (Arts Program)",
		Enrolment: enrolment.Parse(enrol), Completion: completion.Parse("(7.0 credits)\n2. 4.0 credits from Groups A-D, including at least a 0.5 credit from each of Groups A-C.\n3. Third year: TRN303H1, TRN312H1")}
	Link(nil, []*types.Program{esl, alone})
	Link(nil, []*types.Program{open})

	pool := `{"type":"credits","amount":1,"courses":["ETH201H1","ETH210H1","ETH220H1","ETH230H1","PHL265H1","PHL271H1","PHL275H1","POL200Y1"]}`
	assertJSON(t, esl.Enrolment.Req, jAnd(jCr(9), `{"type":"average","min_grade":77,"amount":3,"requirements":[2,3],"best":true}`, pool))
	assertJSON(t, open.Enrolment.Req, jAnd(jCr(9), jOr(`{"type":"average","min_grade":77,"amount":3,"requirements":[2,3],"best":true}`,
		jT("other", "A minimum grade average of 77% in 3.0 credits that count toward the program")), pool))
	assertJSON(t, alone.Enrolment.Req, jAnd(jCr(9), jT("other", "A minimum grade average of 77% in 3.0 credits that count toward the program"), pool))
}

// TestLinkEnrolmentAuditFixes: exclusions never block what req requires, a
// focus keeps the parent its title names, and "or their equivalents" uses
// exclusions that weigh at least as much as the course.
func TestLinkEnrolmentAuditFixes(t *testing.T) {
	econ := []*types.Program{
		{Code: "ASMAJ1478", Type: "Major", Title: "Economics Major (Arts Program)", Areas: types.StringList{"Economics"}},
		{Code: "ASMIN1478", Type: "Minor", Title: "Economics Minor (Arts Program)", Areas: types.StringList{"Economics"}},
		{Code: "ASSPE1478", Type: "Specialist", Title: "Economics Specialist (Arts Program)", Areas: types.StringList{"Economics"},
			Enrolment: enrolment.Parse(limitedLead + "_Special Requirement_\n" +
				"• In order to apply for the Economics Specialist Program, a student must first be enrolled in the Economics Major (ASMAJ1478).\n" +
				"• Students enrolled in this program cannot be simultaneously enrolled in any other Economics program.")},
		{Code: "ASSPE1689", Type: "Specialist", Title: "Computer Science Specialist (Science Program)"},
		{Code: "ASMAJ1689", Type: "Major", Title: "Computer Science Major (Science Program)"},
		{Code: "ASFOC1689U", Type: "Focus", Title: "Focus in Technology Leadership (Computer Science Major)",
			Enrolment: enrolment.Parse("To be admitted to the Technology Leadership Focus, you must:\n" +
				"- Students who complete the Technology Leadership Focus application and/or interview and are admitted to a Computer Science Specialist or Major program will be considered for admission to the Focus.")},
		{Code: "ASMAJ1002", Type: "Major", Title: "Immunology Major (Science Program)",
			Enrolment: enrolment.Parse(limitedLead + "_Completed Courses_\nThe following courses or their equivalents must be completed:\n- CHM151Y1\n- CHM135H1")},
	}
	courseList := []*types.Course{
		{Code: "CHM151Y1", Exclusions: courses.ParseExclusions("CHM135H1, CHM136H1, CHMA10H3")},
		{Code: "CHM135H1", Exclusions: courses.ParseExclusions("CHM151Y1, CHMA11H3")},
	}
	Link(courseList, econ)

	assertJSON(t, econ[2].Enrolment.Exclusions, `[`+jP("other Economics program", "ASMIN1478")+`]`)
	assertJSON(t, econ[5].Enrolment.Req, jP("Computer Science Specialist or Major", "ASMAJ1689"))
	// "can only apply for one focus" and "in German" may cover more programs.
	focusA := &types.Program{Code: "ASMAJ1003A", Type: "Focus", Title: "Focus in Genome Biology",
		Enrolment: enrolment.Parse("Enrolment in the Computer Science Major Program (ASMAJ1689).\n**Students can only apply for one focus.**")}
	german := &types.Program{Code: "ASCER1402", Type: "Certificate", Title: "Certificate in German Language (Category 2)",
		Enrolment: enrolment.Parse("This is an open enrolment Certificate. A student who has completed 4.0 credits may enrol in the Certificate. This Certificate is not open to students enrolled in a Specialist, Major, or Minor in German.")}
	more := []*types.Program{focusA, german,
		{Code: "ASMAJ1003B", Type: "Focus", Title: "Focus in Neurobiology"},
		{Code: "ASMAJ1689", Type: "Major", Title: "Computer Science Major (Science Program)"},
		{Code: "ASMAJ1400", Type: "Major", Title: "German Studies Major (Arts Program)", Areas: types.StringList{"German"}},
		{Code: "ASMIN1163", Type: "Minor", Title: "Al and Malka Green Yiddish Program Minor (Arts Program)", Areas: types.StringList{"German", "Yiddish Studies"}},
	}
	Link(nil, more)
	assertJSON(t, focusA.Enrolment.Req, jAnd(jP("Computer Science Major Program (ASMAJ1689)", "ASMAJ1689"),
		jOr(`{"type":"not","items":[`+jP("another focus of this program", "ASMAJ1003B")+`]}`,
			jT("ambiguity", `"Students can only apply for one focus" may mean a student already in another focus can't apply`))))
	assertJSON(t, german.Enrolment.Exclusions, `[`+jP("Specialist, Major, or Minor in German", "ASMAJ1400")+`]`)
	assertJSON(t, german.Enrolment.Req, jAnd(jCr(4), jOr(`{"type":"not","items":[`+jP("Specialist, Major, or Minor in German", "ASMIN1163")+`]}`,
		jT("ambiguity", `"Specialist, Major, or Minor in German" may also cover the Al and Malka Green Yiddish Program Minor`))))

	// Equivalents reach pools and campus-tagged exclusions, and a shared
	// equivalent can't fill both halves of a pair.
	eq := &types.Program{Code: "ASSPE2575", Type: "Specialist", Title: "Neuroscience Specialist (Science Program)",
		Enrolment: enrolment.Parse(limitedLead + "_Completed Courses_\nThe following courses or their equivalents must be completed:\n" +
			"- BCH210H1\n- ( MAT135H1, MAT136H1)\n- 0.5 credit from BIO220H1, PSL300H1")}
	Link([]*types.Course{
		{Code: "BCH210H1", Exclusions: courses.ParseExclusions("BCH242Y1, [ BIOC12H3(UTSC) and  BIOC13H3(UTSC)],  CHMB62H3(UTSC)")},
		{Code: "MAT135H1", Exclusions: courses.ParseExclusions("MATA36H3")},
		{Code: "MAT136H1", Exclusions: courses.ParseExclusions("MATA36H3")},
		{Code: "PSL300H1", Exclusions: courses.ParseExclusions("PSL302Y1")},
	}, []*types.Program{eq})
	assertJSON(t, eq.Enrolment.Req, jAnd(jCr(4),
		jOr(jC("BCH210H1"), jC("BCH242Y1"), jAnd(jC("BIOC12H3"), jC("BIOC13H3")), jC("CHMB62H3")),
		jOr(jC("MAT135H1"), jC("MATA36H3")), jOr(jC("MAT136H1"), jC("MATA36H3")),
		`{"type":"credits","amount":0.5,"courses":["BIO220H1","PSL300H1","PSL302Y1"]}`,
		`{"type":"credits","amount":1,"courses":["MAT135H1","MATA36H3","MAT136H1"]}`))

	// CHM151Y1 has no equivalent half course; CHM135H1 has CHM151Y1 and CHMA11H3.
	assertJSON(t, econ[6].Enrolment.Req, jAnd(jCr(4), jC("CHM151Y1"), jOr(jC("CHM135H1"), jC("CHM151Y1"), jC("CHMA11H3"))))
}
