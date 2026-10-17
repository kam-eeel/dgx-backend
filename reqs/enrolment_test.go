package reqs

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"degree-explorer/types"
)

// Enrolment snippets below are copied from the catalogue (2026-10-14), one
// per construct.

func jCr(amount float64) string { return fmt.Sprintf(`{"type":"credits","amount":%g}`, amount) }
func jBand(lo, hi float64) string {
	return fmt.Sprintf(`{"type":"credits","amount":%g,"max_amount":%g}`, lo, hi)
}
func jAdm(text string) string { return jT("admission", text) }
func jNot(item string) string { return `{"type":"not","items":[` + item + `]}` }
func jProf(attr, value, text string) string {
	return fmt.Sprintf(`{"type":"profile","attr":%q,"value":%q,"text":%s}`, attr, value, jsonString(text))
}

const (
	limitedLead = "This is a limited enrolment program. Students must have completed 4.0 credits and meet the requirements listed below to enrol.\n"
	vmgLead     = "A minimum grade is needed for entry, and this minimum changes each year depending on available spaces and the number of applicants. The following courses must be completed:\n"
	vmgaLead    = "A minimum grade average in required courses is needed for entry, and this minimum changes each year depending on available spaces and the number of applicants. The following courses must be completed:\n"
)

func TestParseEnrolment(t *testing.T) {
	tests := []struct{ name, raw, kind, want string }{
		{"open",
			"This is an open enrolment program. A student who has completed 4.0 credits may enrol in the program.",
			"open", jCr(4)},
		{"open certificate, threshold with a course",
			"This is a limited enrolment certificate for any non-Rotman Commerce student in the Faculty of Arts & Science. A student who has completed 4.0 credits including MGT100H1 or equivalent may enrol in the Certificate.",
			"limited", jAnd(jCr(4), jOr(jC("MGT100H1"), jT("preparation", "equivalent")))},
		{"completed courses with grades; H, H/ Y is a pair or the full course",
			limitedLead + "_Completed courses (with minimum grades)_\nThe following courses with the stated minimum grades are required:\n" +
				"• ( ECO101H1 (63%), ECO102H1 (63%))/ ECO105Y1 (80%)\n" +
				"• MAT133Y1 (63%)/ [ MAT148H1 (55%), MAT149H1 (55%)/ MAT137Y1 (55%)]",
			"limited", jAnd(jCr(4),
				jOr(jAnd(jCG("ECO101H1", 63), jCG("ECO102H1", 63)), jCG("ECO105Y1", 80)),
				jOr(jCG("MAT133Y1", 63), jAnd(jCG("MAT148H1", 55), jCG("MAT149H1", 55)), jCG("MAT137Y1", 55)))},
		{"variable minimum grade with a floor",
			limitedLead + "_Variable Minimum Grade_\n" + vmgLead + "• CIN105Y1 or CIN201Y1\n" +
				"To ensure that students admitted to the program will be successful, applicants with a final grade lower than 70% will not be considered for admission. Please note that obtaining this minimum final grade does not guarantee admission to the program.",
			"limited", jAnd(jCr(4), jOr(jCG("CIN105Y1", 70), jCG("CIN201Y1", 70)), jAdm("variable minimum grade"))},
		{"variable average with an average floor over courses and pools",
			limitedLead + "_Variable Minimum Grade Average_\n" + vmgaLead +
				"- COG100H1 and\n- 0.5 credits in PSY ( PSY100H1 recommended) and\n- 0.5 credit in LIN, PHL, or CSC.\n" +
				"For reasons of limited capacity, applicants with a final grade average lower than 70% in these required courses will not be considered for admission.",
			"limited", jAnd(jCr(4), jC("COG100H1"),
				`{"type":"credits","amount":0.5,"designators":["PSY"]}`,
				`{"type":"credits","amount":0.5,"designators":["LIN","PHL","CSC"]}`,
				`{"type":"average","min_grade":70,"amount":1.5,"items":[{"type":"credits","amount":0.5,"courses":["COG100H1"]},{"type":"credits","amount":0.5,"designators":["PSY"]},{"type":"credits","amount":0.5,"designators":["LIN","PHL","CSC"]}]}`,
				jAdm("variable minimum grade average"))},
		{"grade and average floors under an and/or heading",
			limitedLead + "_Variable Minimum Grade and Variable Minimum Grade Average_\n" +
				"Minimum grades combined with a minimum grade average in required courses or their equivalents are needed for entry, and these minimums change each year depending on available spaces and the number of applicants. The following courses must be completed:\n" +
				"- BIO120H1 and BIO130H1\n- CHM135H1 and CHM136H1, or CHM151Y1\n" +
				"To ensure that students admitted to the program will be successful, applicants with a final grade lower than 65% in each course, or a final grade average lower than 70% in these required courses will not be considered for admission.",
			"limited", jAnd(jCr(4), jCG("BIO120H1", 65), jCG("BIO130H1", 65),
				jOr(jAnd(jCG("CHM135H1", 65), jCG("CHM136H1", 65)), jCG("CHM151Y1", 65)),
				`{"type":"average","min_grade":70,"amount":2,"items":[{"type":"credits","amount":1,"courses":["BIO120H1","BIO130H1"]},{"type":"or","items":[{"type":"credits","amount":1,"courses":["CHM135H1","CHM136H1"]},{"type":"credits","amount":1,"courses":["CHM151Y1"]}]}]}`,
				jAdm("variable minimum grade and grade average"))},
		{"a floor that names its courses; an average on one branch",
			limitedLead + "_Variable Minimum Grade_\n" + vmgLead + "- BIO130H1\n- ( CHM135H1, CHM136H1)/ CHM151Y1\n" +
				"To ensure that students admitted to the program will be successful, applicants with a final grade lower than 55% in BIO130H1, a final grade average lower than 70% in ( CHM135H1, CHM136H1), or a final grade lower than 70% in CHM151Y1 will not be considered for admission.",
			"limited", jAnd(jCr(4), jCG("BIO130H1", 55),
				jOr(jAnd(jC("CHM135H1"), jC("CHM136H1"), `{"type":"average","min_grade":70,"courses":["CHM135H1","CHM136H1"]}`), jCG("CHM151Y1", 70)),
				jAdm("variable minimum grade"))},
		{"trailing \", or\" makes the list OR; joint designators",
			limitedLead + "_Variable Minimum Grade_\n" + vmgLead +
				"- 0.5 credit in GGR courses at the 100- or 200-level, with a minimum grade of 67%, or\n" +
				"- 1.0 credit in GGR courses at the 100- or 200-level, with a minimum grade of 63%\n" +
				"To ensure that students admitted to the program will be successful, applicants with a final grade or grade average lower than the minimum grades stated above will not be considered for admission. Jointly-offered GGR courses will also be considered (e.g. JEG, JGI, JGE).",
			"limited", jAnd(jCr(4), jOr(
				`{"type":"credits","amount":0.5,"designators":["GGR","JEG","JGI","JGE"],"levels":[100,200],"min_grade":67}`,
				jT("other", "a jointly-offered GGR course"),
				`{"type":"credits","amount":1,"designators":["GGR","JEG","JGI","JGE"],"levels":[100,200],"min_grade":63}`,
				// "final grade or grade average": the 1.0-credit option may be an average.
				jAnd(jOr(`{"type":"credits","amount":1,"designators":["GGR","JEG","JGI","JGE"],"levels":[100,200]}`, jT("other", "a jointly-offered GGR course")),
					`{"type":"average","min_grade":63,"amount":1,"designators":["GGR","JEG","JGI","JGE"],"levels":[100,200]}`,
					jT("ambiguity", averageWhy))),
				jAdm("variable minimum grade"))},
		{"one of the following",
			limitedLead + "_Variable Minimum Grade_\n" +
				"A minimum grade is needed for entry, and this minimum changes each year depending on available spaces and the number of applicants. One of the following courses (0.5 credit) must be completed:\n" +
				"- ENG100H1/ GER100Y1/ HIS109Y1\n" +
				"To ensure that students admitted to the program will be successful, students with a final grade in the completed course lower than 67% will not be considered for admission.",
			"limited", jAnd(jCr(4), jOr(jCG("ENG100H1", 67), jCG("GER100Y1", 67), jCG("HIS109Y1", 67)), jAdm("variable minimum grade"))},
		{"two of the following groups",
			limitedLead + "_Variable Minimum Grade Average_\n" + vmgaLead +
				"2.0 credits from two of the following groups:\n" +
				"- 1.0 credit in Economics: ECO101H1, ECO102H1, ECO105Y1\n" +
				"- 1.0 credit in Sociology: SOC100H1, SOC150H1\n" +
				"- 1.0 credit in Architectural/Visual Studies: JAV120H1, JAV151H1, JAV152H1, ARC181H1",
			"limited", jAnd(jCr(4), jChoose(2,
				`{"type":"credits","amount":1,"courses":["ECO101H1","ECO102H1","ECO105Y1"]}`,
				`{"type":"credits","amount":1,"courses":["SOC100H1","SOC150H1"]}`,
				`{"type":"credits","amount":1,"courses":["JAV120H1","JAV151H1","JAV152H1","ARC181H1"]}`),
				jAdm("variable minimum grade average"))},
		{"options",
			limitedLead + "_Variable Minimum Grade_\n" + vmgLead +
				"2.0 HIS credits to be made up in one of the following ways:\nOption 1:\n" +
				"- 1.0 credit at the 100-level excluding the following HIS First-Year Foundations (FYF) courses: HIS190H1, HIS191H1\n- 1.0 credit at the 200+ level\n" +
				"Option 2:\n- 0.5 credit from HIS111H1, HIS112H1 or the following HIS First-Year Foundations (FYF) courses: HIS190H1, HIS191H1\n- 1.5 credits at the 200+ level",
			"limited", jAnd(jCr(4), jOr(
				jAnd(`{"type":"credits","amount":1,"designators":["HIS"],"levels":[100],"exclude":["HIS190H1","HIS191H1"]}`,
					`{"type":"credits","amount":1,"designators":["HIS"],"levels":[200,300,400]}`),
				jAnd(`{"type":"credits","amount":0.5,"courses":["HIS111H1","HIS112H1","HIS190H1","HIS191H1"]}`,
					`{"type":"credits","amount":1.5,"designators":["HIS"],"levels":[200,300,400]}`)),
				jAdm("variable minimum grade"))},
		{"an average for each alternative",
			limitedLead + "_Completed courses (with minimum grades)_\nThe following courses with the stated minimum grades are required:\n" +
				"• ( MAT148H1, MAT149H1)/ MAT137Y1 with an average grade of 63%/ ( MAT158H1, MAT159H1)/ MAT157Y1 with an average grade of 60%\n" +
				"• ( ACT240H1 and ACT245H1 and ACT247H1) with an average grade of 63%",
			"limited", jAnd(jCr(4),
				jOr(jAnd(jOr(jAnd(jC("MAT148H1"), jC("MAT149H1")), jC("MAT137Y1")), `{"type":"average","min_grade":63,"amount":1,"items":[{"type":"or","items":[{"type":"credits","amount":1,"courses":["MAT148H1","MAT149H1"]},{"type":"credits","amount":1,"courses":["MAT137Y1"]}]}]}`),
					jAnd(jOr(jAnd(jC("MAT158H1"), jC("MAT159H1")), jC("MAT157Y1")), `{"type":"average","min_grade":60,"amount":1,"items":[{"type":"or","items":[{"type":"credits","amount":1,"courses":["MAT158H1","MAT159H1"]},{"type":"credits","amount":1,"courses":["MAT157Y1"]}]}]}`)),
				jC("ACT240H1"), jC("ACT245H1"), jC("ACT247H1"), `{"type":"average","min_grade":63,"courses":["ACT240H1","ACT245H1","ACT247H1"]}`)},
		{"credit bands",
			limitedLead + "Note there are different options depending on whether a student has completed between 4.0 and 8.5 credits, or 9.0 or more credits.\n" +
				"**For students who have completed between 4.0 and 8.5 credits:**\n_Variable Minimum Grade_\n" +
				"A minimum grade is needed for entry, and this minimum changes each year depending on available spaces and the number of applicants. The following courses or their equivalents must be completed:\n" +
				"- BIO130H1\n- ( CHM135H1, CHM136H1)/ CHM151Y1\n" +
				"To ensure that students admitted to the program will be successful, applicants with a final grade lower than 65% in BIO130H1 will not be considered for admission.\n" +
				"**For students who have completed 9.0 or more credits:**\n_Variable Minimum Grade_\n" +
				"Minimum grades are needed for entry, and these minimums change each year depending on available spaces and the number of applicants. The following courses or their equivalents must be completed:\n" +
				"- BIO220H1\n- BIO230H1/ BIO255H1\n" +
				"To ensure that students admitted to the program will be successful, applicants with a final grade lower than 65% in either BIO220H1 or ( BIO230H1/ BIO255H1) will not be considered for admission.",
			"limited", jOr(
				jAnd(jBand(4, 8.5), jCG("BIO130H1", 65), jOr(jAnd(jC("CHM135H1"), jC("CHM136H1")), jC("CHM151Y1")), jAdm("variable minimum grade")),
				jAnd(jCr(9), jCG("BIO220H1", 65), jOr(jCG("BIO230H1", 65), jCG("BIO255H1", 65)), jAdm("variable minimum grade")))},
		{"a list given before a band is for the students below it",
			"This is a limited enrolment program. Students must have completed 4.0 credits and meet the requirements listed below to enroll.\n" +
				"_Variable Minimum Grade and Variable Minimum Grade Average_\n" +
				"A minimum grade and minimum grade average in required courses are needed for entry, and this minimum changes each year depending on available spaces and the number of applicants. The following courses or their equivalents must be completed:\n" +
				"- BIO120H1 and BIO130H1\n" +
				"Students who have completed at least 8.0 credits but who are not already enrolled in ASMAJ2082 must apply using the following courses:\n" +
				"- BIO120H1 and BIO130H1\n- PCL201H1",
			"limited", jAnd(jAdm("variable minimum grade and grade average"), jOr(
				jAnd(jBand(4, 7.5), jC("BIO120H1"), jC("BIO130H1")),
				jAnd(jCr(8), jC("BIO120H1"), jC("BIO130H1"), jC("PCL201H1"))))},
		{"admission categories: CMP1 guarantee or application",
			limitedLead +
				"**For students admitted to Arts & Science in the Year 1 Computer Science (CMP1) admission category:**\n_Completed courses (with minimum grades)_\n" +
				"Students in the CMP1 admissions category have guaranteed admission to the Computer Science Major, provided the following courses with the stated minimum grades are completed within 12 months of beginning their studies:\n" +
				"- CSC110Y1 (70%)\n- CSC111H1 (77%)\n- ( MAT148H1/ MAT158H1, MAT149H1/ MAT159H1)/ MAT137Y1/ MAT157Y1\n" +
				"**For students admitted to other Arts & Science Year 1 admission categories:**\n_Special Requirement_\n" +
				"- Students who do not have the Computer Science Admission Guarantee must complete a supplementary application to be considered for the program.\n" +
				"_Variable Minimum Grade_\n" + vmgLead + "- CSC148H1\n- CSC165H1/ CSC240H1\n" +
				"To ensure that students admitted to the program will be successful, applicants with a grade below 70% will not be considered for admission. Obtaining this minimum grade does not guarantee admission to the program.\n" +
				"_Notes:_\n1. Requests for admission will be considered in the first program request period only.\n" +
				"2. Due to the limited enrolment nature of this program, students are strongly advised to plan to enrol in backup programs.",
			"limited", jAnd(jCr(4), jOr(
				jAnd(jProf("admission_category", "Computer Science", "students admitted to Arts & Science in the Year 1 Computer Science (CMP1) admission category"),
					jProf("max_months_from_start", "12", "required courses completed within 12 months of beginning studies"),
					jCG("CSC110Y1", 70), jCG("CSC111H1", 77),
					jOr(jAnd(jOr(jC("MAT148H1"), jC("MAT158H1")), jOr(jC("MAT149H1"), jC("MAT159H1"))), jC("MAT137Y1"), jC("MAT157Y1"))),
				jAnd(jOr(jNot(jProf("admission_category", "Computer Science", "students admitted to other Arts & Science Year 1 admission categories")),
					jT("ambiguity", "a Computer Science student who doesn't meet the admission guarantee may also apply this way")),
					jAdm("Students who do not have the Computer Science Admission Guarantee must complete a supplementary application to be considered for the program"),
					jCG("CSC148H1", 70), jOr(jCG("CSC165H1", 70), jCG("CSC240H1", 70)),
					jAdm("variable minimum grade"))))},
		{"in lieu of: alternatives to a course",
			"This is a limited enrolment program. Note there are different options depending on whether a student has completed between 4.0 and 8.5 credits, or 9.0 or more credits.\n" +
				"_Completed courses (with minimum grades)_\nThe following courses must be completed:\n" +
				"• PSY100H1 (75%)*****\n• Gr. 12 Calculus (passing grade)\n" +
				"**Notes:**\n1. In lieu of PSY100H1, we will also accept:\n- PSY100Y5 from UTM, or\n- PSYA01H3 **_and_** PSYA02H3 (both) from UTSC.\n" +
				"However, we will not accept AP or IB Psychology for this requirement.\n" +
				"2. For students who are missing Gr. 12 Calculus and/or Biology, information on these requirements and how they can be fulfilled is provided in the Psychology section of the Calendar under 'Regarding Psychology Programs'.",
			"limited", jAnd(jCr(4), jOr(jCG("PSY100H1", 75), jCG("PSY100Y5", 75), jAnd(jCG("PSYA01H3", 75), jCG("PSYA02H3", 75))),
				jOr(jProf("high_school_course", "Grade 12 Calculus", "Gr. 12 Calculus"), jT("preparation", hsAltText)))},
		{"a stray bracket",
			limitedLead + "_Completed courses (with minimum grades)_\nThe following courses are required:\n" +
				"- ( MAT148H1/ MAT158H1, MAT149H1/ MAT159H1)/ MAT137Y1)/ MAT157Y1",
			"limited", jAnd(jCr(4), jOr(jAnd(jOr(jC("MAT148H1"), jC("MAT158H1")), jOr(jC("MAT149H1"), jC("MAT159H1"))), jC("MAT137Y1"), jC("MAT157Y1")))},
		{"special requirements: program enrolment and an interview",
			limitedLead + "_Special Requirement_\n" +
				"- Students must pass an interview to enrol in the program. The interview is held annually in April-May.\n" +
				"- Student must be enrolled in the Criminology and Sociolegal Studies Major (ASMAJ0826)",
			"limited", jAnd(jCr(4), jAdm("Students must pass an interview to enrol in the program"),
				jP("Criminology and Sociolegal Studies Major (ASMAJ0826)", "ASMAJ0826"))},
		{"an application only",
			"While we welcome students with diverse experiences, this is a limited enrollment program that can accommodate a limited number of students. Admission is determined by a range of criteria, including the student's academic records from their first year of studies, short essays, a writing sample, a resume, and a video submission.",
			"limited", jAnd(jCr(4), jAdm("Admission is determined by a range of criteria, including the student's academic records from their first year of studies, short essays, a writing sample, a resume, and a video submission"))},
		{"focus by code",
			"Enrolment in the Computer Science Major Program (ASMAJ1689).",
			"focus", jP("Computer Science Major Program (ASMAJ1689)", "ASMAJ1689")},
		{"focus with a grade chain and a statement",
			"Once you have enrolled in the Cell and Molecular Biology Major program, you have the option to apply for entry into a focus. **Students can only apply for one focus.** Admission will be determined with a minimum grade of 80% in BIO130H1. If the student does not achieve 80% in BIO130H1, admission can be determined with a minimum grade of 80% in BIO230H1, BIO255H1 or CSB349H1. On the application form, students must include a 300-word statement of interest regarding the topic of the focus.",
			"focus", jAnd(jP("Cell and Molecular Biology Major"),
				jOr(jCG("BIO130H1", 80), jCG("BIO230H1", 80), jCG("BIO255H1", 80), jCG("CSB349H1", 80)),
				jAdm("On the application form, students must include a 300-word statement of interest regarding the topic of the focus"))},
		{"Technology Leadership",
			"To be admitted to the Technology Leadership Focus, you must:\n" +
				"- Have not yet completed CSC207H1 prior to beginning Focus requirements.\n" +
				"- Complete an application and/or interview. Application information will be provided to all Computer Science Specialist and Major and Data Science Specialist program applicants during the Round 1 program admission period.\n" +
				"- Students who complete the Technology Leadership Focus application and/or interview and are admitted to a Computer Science Specialist or Major or Data Science Specialist program will be considered for admission to the Focus.",
			"focus", jAnd(jNot(jC("CSC207H1")),
				jAdm("Complete an application and/or interview"),
				jOr(jP("Computer Science Specialist or Major"), jP("Data Science Specialist")))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := ParseEnrolment(tt.raw)
			if string(e.Kind) != tt.kind {
				t.Errorf("kind = %s, want %s", e.Kind, tt.kind)
			}
			assertJSON(t, e.Req, tt.want)
		})
	}
}

func TestEnrolmentASIPIsANote(t *testing.T) {
	raw := limitedLead + "_Completed courses (with minimum grades)_\nThe following course with the stated minimum grade is required:\n- BMS100H1 (70%)\n" +
		"**Arts & Science Internship Program**\n" +
		"Students in this program have the option to request enrolment in the Arts & Science Internship Program (ASIP) stream. Space is more limited for Year 3 entry and there are program-specific year 3 entry enrolment requirements. Students applying for Year 3 entry must have been admitted to the Book & Media Studies Major in the Summer after Year 2 and have completed BMS100H1, BMS200Y1 and BMS201H1."
	e := ParseEnrolment(raw)
	assertJSON(t, e.Req, jAnd(jCr(4), jCG("BMS100H1", 70)))
	if len(e.Notes) != 1 || !strings.Contains(e.Notes[0], "BMS200Y1") {
		t.Errorf("notes = %q, want the ASIP paragraph", e.Notes)
	}
	// Without a heading the paragraph is a note too, and so is the rest of
	// it in a "Special Requirement" section.
	raw = limitedLead + "_Special Requirement_\n• Students must be enrolled in the Economics Major (ASMAJ1478)\n" +
		"Students in this program have the option to request enrolment in the Arts & Science Internship Program (ASIP) stream. Space is more limited for Year 3 entry. Students applying for Year 3 entry must have been admitted to the Economics Specialist in the Summer after Year 2."
	assertJSON(t, ParseEnrolment(raw).Req, jAnd(jCr(4), jP("Economics Major (ASMAJ1478)", "ASMAJ1478")))
}

func TestEnrolmentSuspended(t *testing.T) {
	e := ParseEnrolment("**Enrolment in the Forest Biomaterials Science Major is being administratively suspended as of 1 October 2020 and no new students will be admitted thereafter. Students presently enrolled in the Major will be able to complete the program requirements as described below.**")
	if e.Kind != types.EnrolmentSuspended || e.Req != nil {
		t.Errorf("got kind %s, req %v; want suspended, nil", e.Kind, e.Req)
	}
	// A banner about another program (ASMAJ2085) on a live program is a note.
	e = ParseEnrolment("**The Health Studies Major (ASMAJ2085) was last available for enrolment during the second round of program enrolment in Summer 2024. The program completion requirements for the Health Studies Major can be found in the archived 2023-24 Academic Calendar.**\n" +
		"This is a limited enrolment program. Note there are different options depending on whether a student has completed between 4.0 and 8.5 credits, or 9.0 or more credits.\n" +
		"_Variable Minimum Grade Average_\n" + vmgaLead +
		"**For students who have completed between 4.0 and 8.5 credits:**\n" +
		"- 1.5 credits from: ANT100Y1/ BIO130H1/ CSB201H1. If more than 1.5 credits have been completed, the minimum grade average will be based on the higher course grades.\n" +
		"  To ensure that students admitted to the program will be successful, applicants with a grade average lower than 70% in required courses will not usually be considered for admission.\n" +
		"**For students who have completed 9.0 credits or more:**\n" +
		"- 1.5 credits from: ANT100Y1/ BIO130H1/ PHS209H1.")
	if e.Kind != types.EnrolmentLimited {
		t.Errorf("kind = %s, want limited", e.Kind)
	}
	pool := func(codes ...string) string {
		return fmt.Sprintf(`{"type":"credits","amount":1.5,"courses":["%s"]}`, strings.Join(codes, `","`))
	}
	assertJSON(t, e.Req, jAnd(jAdm("variable minimum grade average"), jOr(
		jAnd(jBand(4, 8.5), pool("ANT100Y1", "BIO130H1", "CSB201H1"),
			jOr(`{"type":"average","min_grade":70,"amount":1.5,"courses":["ANT100Y1","BIO130H1","CSB201H1"],"best":true}`,
				jT("ambiguity", "applicants below the minimum are not usually considered, so some may be"))),
		jAnd(jCr(9), pool("ANT100Y1", "BIO130H1", "PHS209H1")))))
	if !slices.ContainsFunc(e.Notes, func(n string) bool { return strings.Contains(n, "ASMAJ2085") }) {
		t.Errorf("stale banner not in notes: %q", e.Notes)
	}
}

func TestEnrolmentExclusions(t *testing.T) {
	tests := []struct{ raw, want string }{
		{"This is an open enrolment Certificate. A student who has completed 4.0 credits may enrol in the Certificate. This Certificate is not open to students enrolled in a Specialist, Major, or Minor in Portuguese.",
			`[` + jP("Specialist, Major, or Minor in Portuguese") + `]`},
		{limitedLead + "Please note, the Biochemistry Major cannot be combined with the:\n- Immunology Specialist\n- Chemistry Specialist",
			`[` + jP("Immunology Specialist") + `,` + jP("Chemistry Specialist") + `]`},
		{limitedLead + "**Note:**\nStudents cannot combine a Sociology Major program with another Sociology program (Specialist or Minor) for their degree.",
			`[` + jP("Sociology Specialist or Minor") + `]`},
		{limitedLead + "Notes:\n- Students cannot combine the Pharmacology Specialist with either departmental Major programs (Biomedical Toxicology or Pharmacology).",
			`[` + jP("Biomedical Toxicology Major") + `,` + jP("Pharmacology Major") + `]`},
		{limitedLead + "_Special Requirement_\n• Students enrolled in this program cannot be simultaneously enrolled in any other Economics program or the Environmental Economics minor.",
			`[` + jP("other Economics program") + `,` + jP("Environmental Economics minor") + `]`},
		{"This is an open enrolment Certificate. A student who has completed 4.0 credits may enrol in the Certificate. This Certificate is not open to students enrolled in any Minor, Major, or Specialist offered by the Department of French.",
			`[` + jP("Minor, Major, or Specialist offered by the Department of French") + `]`},
		{limitedLead + "Students enrolled in this program cannot be simultaneously enrolled in or complete any Computer Science, Statistics, or Actuarial Science programs, including the Computer Science Minor, Statistics Minor, and Data Science Specialist; nor any Mathematics Major or Specialist program; nor the Focus in Data Analytics within the Economics Major or Specialist.",
			`[` + jP("Computer Science, Statistics, or Actuarial Science") + `,` + jP("Mathematics Major or Specialist") + `,` + jP("Focus in Data Analytics within the Economics Major or Specialist") + `]`},
	}
	for _, tt := range tests {
		e := ParseEnrolment(tt.raw)
		assertJSON(t, e.Exclusions, tt.want)
	}
}

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
		p := &types.Program{Code: code, Enrolment: ParseEnrolment(raw)}
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
		p := &types.Program{Code: code, Enrolment: ParseEnrolment(raw)}
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
	completion := "1. JEG100H1, JGU216H1; JAV120H1\n2. 0.5 credit from GGR201H1, JIG322H1"
	ggrMajor := &types.Program{Code: "ASMAJ1667", Type: "Major", Title: "Human Geography Major (Arts Program)",
		Areas: types.StringList{"Geography and Planning"}, Completion: ParseCompletion(completion),
		Enrolment: ParseEnrolment(limitedLead + "_Variable Minimum Grade_\n" + vmgLead +
			"- 0.5 credit in GGR courses at the 100- or 200-level, with a minimum grade of 67%\n" +
			"Jointly-offered GGR courses will also be considered (e.g. JEG, JGI, JGE).")}
	cert := &types.Program{Code: "ASCER2400", Type: "Certificate", Title: "Certificate in Business Fundamentals (Category 2)",
		Enrolment: ParseEnrolment("This is a limited enrolment certificate for any non-Rotman Commerce student in the Faculty of Arts & Science. A student who has completed 4.0 credits including MGT100H1 or equivalent may enrol in the Certificate.")}
	minor := &types.Program{Code: "ASMIN0160", Type: "Minor", Title: "Applied Data Science Minor (Science Program)",
		Enrolment: ParseEnrolment(limitedLead + "Students enrolled in this program cannot be simultaneously enrolled in or complete any Mathematics Major; nor the Focus in Data Science in Business within the Rotman Commerce specialist programs.")}
	programs := []*types.Program{ggrMajor, cert, minor,
		{Code: "ASMAJ1165", Type: "Major", Title: "Mathematics Major (Science Program)", Areas: types.StringList{"Mathematics"}}}
	courses := []*types.Course{{Code: "MGT100H1", Exclusions: ParseExclusions("RSM100H1,  RSM100Y1")}}
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

// TestLinkEnrolmentCompletion reads an enrolment average over "credits
// that count toward the program" (ASMAJ1618, shortened) as an average over
// the program's completion rows that claim courses: not the total, which
// has no pool of its own. While one of those rows is text (here "4.0
// credits from Groups A-D ..." without the groups), it is or(average, the
// sentence): Review, not NotMet. Without completion rows it stays text. In
// the corpus every row of ASMAJ1618 is read, and the average is bare.
func TestLinkEnrolmentCompletion(t *testing.T) {
	enrol := limitedLead + "**For students who have completed 9.0 credits:**\n" +
		"• A minimum grade average of 77% in 3.0 credits that count toward the program, including 1.0 credit from: ETH201H1, ETH210H1, ETH220H1, ETH230H1, PHL265H1, PHL271H1, PHL275H1, and POL200Y1."
	completion := "(7.0 credits)\n3. Third year: TRN303H1, TRN312H1\n4. Fourth year: TRN412H1."
	esl := &types.Program{Code: "ASMAJ1618", Type: "Major", Title: "Ethics, Society, and Law Major (Arts Program)",
		Enrolment: ParseEnrolment(enrol), Completion: ParseCompletion(completion)}
	alone := &types.Program{Code: "ASMAJ9999", Type: "Major", Title: "Example Major (Arts Program)", Enrolment: ParseEnrolment(enrol)}
	open := &types.Program{Code: "ASMAJ1618", Type: "Major", Title: "Ethics, Society, and Law Major (Arts Program)",
		Enrolment: ParseEnrolment(enrol), Completion: ParseCompletion("(7.0 credits)\n2. 4.0 credits from Groups A-D, including at least a 0.5 credit from each of Groups A-C.\n3. Third year: TRN303H1, TRN312H1")}
	Link(nil, []*types.Program{esl, alone})
	Link(nil, []*types.Program{open})

	pool := `{"type":"credits","amount":1,"courses":["ETH201H1","ETH210H1","ETH220H1","ETH230H1","PHL265H1","PHL271H1","PHL275H1","POL200Y1"]}`
	assertJSON(t, esl.Enrolment.Req, jAnd(jCr(9), `{"type":"average","min_grade":77,"amount":3,"requirements":[2,3],"best":true}`, pool))
	assertJSON(t, open.Enrolment.Req, jAnd(jCr(9), jOr(`{"type":"average","min_grade":77,"amount":3,"requirements":[2,3],"best":true}`,
		jT("other", "A minimum grade average of 77% in 3.0 credits that count toward the program")), pool))
	assertJSON(t, alone.Enrolment.Req, jAnd(jCr(9), jT("other", "A minimum grade average of 77% in 3.0 credits that count toward the program"), pool))
}

func TestParseFloors(t *testing.T) {
	tests := []struct {
		s    string
		want []floor
	}{
		{"To ensure that students admitted to the program will be successful, applicants with a final grade lower than 60% in each course, or a grade average lower than 70% in these required courses will not be considered for admission",
			[]floor{{grade: 60}, {grade: 70, average: true}}},
		{"applicants with a final grade or grade average lower than a B in these required courses will not be considered for admission",
			[]floor{{grade: 73}}},
		{"applicants with a final grade lower than 55% in BIO130H1, a final grade average lower than 70% in ( CHM135H1, CHM136H1), or a final grade lower than 70% in CHM151Y1 will not be considered for admission",
			[]floor{{grade: 55, codes: []string{"BIO130H1"}}, {grade: 70, average: true, codes: []string{"CHM135H1", "CHM136H1"}}, {grade: 70, codes: []string{"CHM151Y1"}}}},
		{"applicants will not be considered for admission with a grade lower than 77% in CSC111H1; or lower than 70% in either MAT137Y1 or MAT157Y1, or in the average of MAT148H1 or MAT158H1 and MAT149H1 or MAT159H1",
			[]floor{{grade: 77, codes: []string{"CSC111H1"}}, {grade: 70, codes: []string{"MAT137Y1", "MAT157Y1"}},
				{grade: 70, average: true, codes: []string{"MAT148H1", "MAT158H1", "MAT149H1", "MAT159H1"}}}},
		{"An average of at least 73% in the prerequisite courses is needed for admission consideration and preference will be given to students whose marks are the result of a single attempt in each course",
			[]floor{{grade: 73, average: true}}},
		{"applicants with a final grade or grade average lower than the minimum grades stated above will not be considered for admission", nil},
	}
	for _, tt := range tests {
		got := parseFloors(tt.s)
		if fmt.Sprint(got) != fmt.Sprint(tt.want) {
			t.Errorf("parseFloors(%q)\n got %v\nwant %v", tt.s, got, tt.want)
		}
	}
}

func TestEnrolLines(t *testing.T) {
	raw := "_Variable Minimum Grade_\n**For students who have completed 4.0 to 8.5 credits:**\n" +
		"For students who are exempted from a first-year language course (i.e. those with some prior language skills):\n" +
		"Option 1:\nNote:\n**Notes**:\n" +
		"**Enrolment in the Hungarian Studies Major will be administratively suspended as of 7 July 2023.**\n" +
		"- PSYA01H3 **_and_** PSYA02H3 (both) from UTSC.\n  - Nested item\n1. Numbered\nA. Lettered\n" +
		"A minimum grade is needed for entry, and this minimum may change depending on\navailable spaces and the number of applicants."
	var got []string
	for _, l := range enrolLines(raw) {
		got = append(got, fmt.Sprintf("%d/%d/%s/%s", l.kind, l.depth, l.marker, l.text))
	}
	want := []string{
		"2/0//Variable Minimum Grade",
		"2/0//For students who have completed 4.0 to 8.5 credits:",
		"2/0//For students who are exempted from a first-year language course (i.e. those with some prior language skills):",
		"2/0//Option 1:", "2/0//Note:", "2/0//Notes:",
		"0/0//Enrolment in the Hungarian Studies Major will be administratively suspended as of 7 July 2023.",
		"1/0/-/PSYA01H3 and PSYA02H3 (both) from UTSC.",
		"1/1/-/Nested item", "1/0/#/Numbered", "1/0/#/Lettered",
		"0/0//A minimum grade is needed for entry, and this minimum may change depending on available spaces and the number of applicants.",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// Every boilerplate family, with a real sentence, and binding sentences
// that must not be mistaken for boilerplate.
func TestEnrolBoiler(t *testing.T) {
	for _, s := range []string{
		"Students in this program have the option to request enrolment in the Arts & Science Internship Program (ASIP) stream",
		"Please consult the Arts & Science Program Toolkit for more information on program enrolment",
		"Applications from students lacking numerical grades in one or more of the specified prerequisites, or students applying in their second or later years, may be considered on the basis of their entire academic record and the availability of space in the program",
		"Please note that obtaining this minimum final grade does not guarantee admission to the program",
		"The underlying numeric final mark in each of your courses required for program enrolment will be used by the relevant academic unit to determine your eligibility",
		"While it is difficult to predict what will be competitive course marks and average in a given year, based on previous years, the estimate is: course marks = mid 80s; average = mid 80s",
		"In past admission cycles, the average grade cut-off for the PSY Major program ranged from approximately 77-79%",
		"Due to the limited enrolment nature of this program, students are strongly advised to apply to backup programs",
		"Students admitted to the program after second or third year will be required to pay retroactive deregulated program fees",
		"Students applying for admissions to the program utilizing transfer credits will be considered on a case-by-case basis",
		"Students are invited to inquire with the program office about possible appropriate substitutions for the courses above",
		"If students have not taken a course from this list, they can request approval from the unit to consider other relevant courses for admission",
		"For information regarding admission to the program, contact Rotman Commerce at rc.admissions@utoronto.ca",
		"Enrolment in this Focus is recommended after second year to guide choice of courses in the higher years",
		"Students who fail to contribute to the faculty-led learning community will be removed from the focus",
		"Please note that IRW100H1 cannot count for the First Year or elective program requirements",
		"Note there are different options depending on whether a student has completed between 4.0 and 8.5 credits, or 9.0 or more credits",
		"Note that none of the required courses may be completed using the CR/NCR option",
		"If more than 1.0 credits have been completed from LIN, CSC, PHL, or PSY, the grade average will be based on the higher course grades",
		"Admission will be based on marks and the Rotman Commerce Supplemental Application",
		"Students are encouraged to take their introductory calculus/analysis and algebra in their first year of study",
		"The Health Studies Major (ASMAJ2085) was last available for enrolment during the second round of program enrolment in Summer 2024",
		"Grades in MAT158H1, MAT159H1, and MAT157Y1 will be adjusted to account for the greater difficulty of those courses",
		"Upon acceptance to the Economics Specialist Program, your Specialist enrolment will replace your Economics Major",
		"Credit for higher-level SOC courses will not waive these requirements",
		"The interview is held annually in April-May",
	} {
		if !isEnrolBoiler(s) {
			t.Errorf("not boilerplate: %q", s)
		}
	}
	for _, s := range []string{
		"Students must pass an interview to enrol in the program",
		"Requests for admission will be considered in the first program request period only",
		"Students cannot combine a Sociology Major program with another Sociology program (Specialist or Minor) for their degree",
		"Student must be enrolled in the Criminology and Sociolegal Studies Major (ASMAJ0826)",
		"To ensure that students admitted to the program will be successful, applicants with a final grade lower than 70% will not be considered for admission",
	} {
		if isEnrolBoiler(s) {
			t.Errorf("binding sentence is boilerplate: %q", s)
		}
	}
}

func TestVariableText(t *testing.T) {
	for in, want := range map[string]string{
		"Variable Minimum Grade":                                    "variable minimum grade",
		"Variable Minimum Grade Average":                            "variable minimum grade average",
		"Variable Minimum Grade and Variable Minimum Grade Average": "variable minimum grade and grade average",
		"Variable Minimum Grade/Grade Average":                      "variable minimum grade and grade average",
		"Minimum grades combined with a minimum grade average in required courses or their equivalents are needed for entry, and these minimums change each year": "variable minimum grade and grade average",
	} {
		if got := variableText(in); got != want {
			t.Errorf("variableText(%q) = %q, want %q", in, got, want)
		}
	}
}

// Fixes from the 2026-10-14 audit, one case each.
func TestEnrolmentAuditFixes(t *testing.T) {
	tests := []struct{ name, raw, want string }{
		{"a cap inside a pool is a share of it",
			limitedLead + "_Variable Minimum Grade Average_\n" + vmgaLead +
				"- 1.5 credits from SOC/ PSY/ IRE/ IRW, at any level, of which no more than 0.5 credit can be IRE/ IRW",
			jAnd(jCr(4), `{"type":"credits","amount":1.5,"designators":["SOC","PSY","IRE","IRW"]}`,
				`{"type":"credits","amount":1,"designators":["SOC","PSY"]}`, jAdm("variable minimum grade average"))},
		{"used in place of: with both, the replacement counts",
			limitedLead + "_Completed courses (with minimum grades)_\nThe following courses are required:\n- STA130H1/ STA261H1*\n" +
				"* STA261H1 will be used in place of STA130H1 for program admission purposes if a student has not completed STA130H1 or if they have completed both STA130H1 and STA261H1 by the time they are being considered for admission.",
			jAnd(jCr(4), jOr(jAnd(jC("STA130H1"), jNot(jC("STA261H1"))), jC("STA261H1")))},
		{"adjusted grades: below the floor is Review",
			limitedLead + "_Completed courses (with minimum grades)_\nThe following courses are required:\n- MAT137Y1/ MAT157Y1\n" +
				"To ensure that students admitted to the program will be successful, applicants will not be considered for admission with a grade lower than 70% in either MAT137Y1 or MAT157Y1. Grades in MAT157Y1 will be adjusted to account for the greater difficulty of those courses.",
			jAnd(jCr(4), jOr(jCG("MAT137Y1", 70), jCG("MAT157Y1", 70), jAnd(jC("MAT157Y1"), jAdm(adjustedText))))},
		{"reserved spaces aren't a timing condition",
			limitedLead + "**For students admitted to Arts & Science in the Year 1 Computer Science (CMP1) admission category:**\n" +
				"At least 20 spaces will be available each year for students applying from Year 1 Computer Science (CMP1) within 12 months of beginning their studies:\n- CSC110Y1",
			jAnd(jCr(4), jProf("admission_category", "Computer Science", "students admitted to Arts & Science in the Year 1 Computer Science (CMP1) admission category"), jC("CSC110Y1"))},
		{"a whole-record route for later and ungraded applicants",
			"This is an open enrolment program. A student who has completed 4.0 credits may enrol in the program.\n" +
				"Applications from students lacking numerical grades in one or more of the specified prerequisites, or students applying in their second or later years, may be considered on the basis of their entire academic record and the availability of space in the program.",
			jOr(jCr(4), jAnd(jCr(4), jOr(
				jProf("has_missing_numeric_grades", "true", "lacking numerical grades in one or more of the specified prerequisites"),
				jProf("min_year_of_study", "2", "applying in their second or later years")),
				jAdm("considered on the basis of their entire academic record")))},
		{"one course (0.5 credit): one half of a pair may be enough",
			limitedLead + "_Variable Minimum Grade_\nA minimum grade is needed for entry, and this minimum changes each year depending on available spaces and the number of applicants. One of the following courses (0.5 credit) must be completed:\n" +
				"- BIO120H1/ ( MAT158H1, MAT159H1/ MAT157Y1)",
			jAnd(jCr(4), jOr(jC("BIO120H1"),
				jAnd(jC("MAT158H1"), jC("MAT159H1")),
				jAnd(jOr(jC("MAT158H1"), jC("MAT159H1")), jT("ambiguity", "the list asks for one course (0.5 credit), so one of these half courses may be enough")),
				jC("MAT157Y1")), jAdm("variable minimum grade"))},
		{"1.0 credit from whole groups, or any 1.0 credit of their courses",
			limitedLead + "_Completed Courses_\nThe following courses are required:\n" +
				"- 1.0 credit from ( MAT135H1 and MAT136H1)/ ( PHY131H1 and PHY132H1)",
			jAnd(jCr(4), jOr(jAnd(jC("MAT135H1"), jC("MAT136H1")), jAnd(jC("PHY131H1"), jC("PHY132H1")),
				jAnd(`{"type":"credits","amount":1,"courses":["MAT135H1","MAT136H1","PHY131H1","PHY132H1"]}`,
					jT("ambiguity", "any 1.0 credit from the listed courses may count, not only one whole group"))))},
		{"final grade or grade average: per course, or the average",
			limitedLead + "_Variable Minimum Grade_\n" + vmgLead + "- JAV120H1 and JAV130H1.\n" +
				"To ensure that students admitted to the program will be successful, applicants with a final grade or grade average lower than a B in these required courses will not be considered for admission.",
			jAnd(jCr(4), jOr(jAnd(jCG("JAV120H1", 73), jCG("JAV130H1", 73)),
				jAnd(jC("JAV120H1"), jC("JAV130H1"), `{"type":"average","min_grade":73,"courses":["JAV120H1","JAV130H1"]}`, jT("ambiguity", averageWhy))),
				jAdm("variable minimum grade"))},
		{"two requirements a course could share",
			limitedLead + "_Variable Minimum Grade Average_\n" + vmgaLead +
				"1. 0.5 credit from ECO101H1/ ECO105Y1/ IRW230H1\n2. 1.5 credits from SOC/ PSY/ IRE/ IRW, at any level",
			jAnd(jCr(4), `{"type":"credits","amount":0.5,"courses":["ECO101H1","ECO105Y1","IRW230H1"]}`,
				`{"type":"credits","amount":1.5,"designators":["SOC","PSY","IRE","IRW"]}`, jAdm("variable minimum grade average"),
				jOr(`{"type":"credits","amount":2,"courses":["ECO101H1","ECO105Y1","IRW230H1"],"designators":["SOC","PSY","IRE","IRW"]}`,
					jT("ambiguity", "a course may count toward both requirements")))},
		{"a whole sequence or the single courses, or loosely any 1.0 credit",
			limitedLead + "_Completed Courses_\nThe following courses are required:\n" +
				"- 1.0 credit from ( MAT135H1, MAT136H1)/ MAT137Y1, PHY131H1, PHY132H1",
			jAnd(jCr(4), jOr(jAnd(jC("MAT135H1"), jC("MAT136H1")), jC("MAT137Y1"),
				`{"type":"credits","amount":1,"courses":["PHY131H1","PHY132H1"]}`,
				jAnd(`{"type":"credits","amount":1,"courses":["MAT135H1","MAT136H1","MAT137Y1","PHY131H1","PHY132H1"]}`,
					jT("ambiguity", "any 1.0 credit from the listed courses may count, not only a whole sequence"))))},
		{"approval for another course from the list",
			limitedLead + "_Variable Minimum Grade_\n" + "A minimum grade is needed for entry, and this minimum changes each year depending on available spaces and the number of applicants. One of the following courses (0.5 credit) must be completed:\n" +
				"- ENG100H1/ GER100Y1\nIf students have not taken a course from this list, they can request approval from the unit to consider other relevant courses for admission.",
			jAnd(jCr(4), jOr(jC("ENG100H1"), jC("GER100Y1"), jT("permission", "the unit approves another relevant course")), jAdm("variable minimum grade"))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertJSON(t, ParseEnrolment(tt.raw).Req, tt.want)
		})
	}
}

// TestLinkEnrolmentAuditFixes: exclusions never block what req requires, a
// focus keeps the parent its title names, and "or their equivalents" uses
// exclusions that weigh at least as much as the course.
func TestLinkEnrolmentAuditFixes(t *testing.T) {
	econ := []*types.Program{
		{Code: "ASMAJ1478", Type: "Major", Title: "Economics Major (Arts Program)", Areas: types.StringList{"Economics"}},
		{Code: "ASMIN1478", Type: "Minor", Title: "Economics Minor (Arts Program)", Areas: types.StringList{"Economics"}},
		{Code: "ASSPE1478", Type: "Specialist", Title: "Economics Specialist (Arts Program)", Areas: types.StringList{"Economics"},
			Enrolment: ParseEnrolment(limitedLead + "_Special Requirement_\n" +
				"• In order to apply for the Economics Specialist Program, a student must first be enrolled in the Economics Major (ASMAJ1478).\n" +
				"• Students enrolled in this program cannot be simultaneously enrolled in any other Economics program.")},
		{Code: "ASSPE1689", Type: "Specialist", Title: "Computer Science Specialist (Science Program)"},
		{Code: "ASMAJ1689", Type: "Major", Title: "Computer Science Major (Science Program)"},
		{Code: "ASFOC1689U", Type: "Focus", Title: "Focus in Technology Leadership (Computer Science Major)",
			Enrolment: ParseEnrolment("To be admitted to the Technology Leadership Focus, you must:\n" +
				"- Students who complete the Technology Leadership Focus application and/or interview and are admitted to a Computer Science Specialist or Major program will be considered for admission to the Focus.")},
		{Code: "ASMAJ1002", Type: "Major", Title: "Immunology Major (Science Program)",
			Enrolment: ParseEnrolment(limitedLead + "_Completed Courses_\nThe following courses or their equivalents must be completed:\n- CHM151Y1\n- CHM135H1")},
	}
	courses := []*types.Course{
		{Code: "CHM151Y1", Exclusions: ParseExclusions("CHM135H1, CHM136H1, CHMA10H3")},
		{Code: "CHM135H1", Exclusions: ParseExclusions("CHM151Y1, CHMA11H3")},
	}
	Link(courses, econ)

	assertJSON(t, econ[2].Enrolment.Exclusions, `[`+jP("other Economics program", "ASMIN1478")+`]`)
	assertJSON(t, econ[5].Enrolment.Req, jP("Computer Science Specialist or Major", "ASMAJ1689"))
	// "can only apply for one focus" and "in German" may cover more programs.
	focusA := &types.Program{Code: "ASMAJ1003A", Type: "Focus", Title: "Focus in Genome Biology",
		Enrolment: ParseEnrolment("Enrolment in the Computer Science Major Program (ASMAJ1689).\n**Students can only apply for one focus.**")}
	german := &types.Program{Code: "ASCER1402", Type: "Certificate", Title: "Certificate in German Language (Category 2)",
		Enrolment: ParseEnrolment("This is an open enrolment Certificate. A student who has completed 4.0 credits may enrol in the Certificate. This Certificate is not open to students enrolled in a Specialist, Major, or Minor in German.")}
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

	// Second pass: equivalents reach pools and campus-tagged exclusions, and
	// a shared equivalent can't fill both halves of a pair.
	eq := &types.Program{Code: "ASSPE2575", Type: "Specialist", Title: "Neuroscience Specialist (Science Program)",
		Enrolment: ParseEnrolment(limitedLead + "_Completed Courses_\nThe following courses or their equivalents must be completed:\n" +
			"- BCH210H1\n- ( MAT135H1, MAT136H1)\n- 0.5 credit from BIO220H1, PSL300H1")}
	Link([]*types.Course{
		{Code: "BCH210H1", Exclusions: ParseExclusions("BCH242Y1, [ BIOC12H3(UTSC) and  BIOC13H3(UTSC)],  CHMB62H3(UTSC)")},
		{Code: "MAT135H1", Exclusions: ParseExclusions("MATA36H3")},
		{Code: "MAT136H1", Exclusions: ParseExclusions("MATA36H3")},
		{Code: "PSL300H1", Exclusions: ParseExclusions("PSL302Y1")},
	}, []*types.Program{eq})
	assertJSON(t, eq.Enrolment.Req, jAnd(jCr(4),
		jOr(jC("BCH210H1"), jC("BCH242Y1"), jAnd(jC("BIOC12H3"), jC("BIOC13H3")), jC("CHMB62H3")),
		jOr(jC("MAT135H1"), jC("MATA36H3")), jOr(jC("MAT136H1"), jC("MATA36H3")),
		`{"type":"credits","amount":0.5,"courses":["BIO220H1","PSL300H1","PSL302Y1"]}`,
		`{"type":"credits","amount":1,"courses":["MAT135H1","MATA36H3","MAT136H1"]}`))

	// CHM151Y1 has no equivalent half course; CHM135H1 has CHM151Y1 and CHMA11H3.
	assertJSON(t, econ[6].Enrolment.Req, jAnd(jCr(4), jC("CHM151Y1"), jOr(jC("CHM135H1"), jC("CHM151Y1"), jC("CHMA11H3"))))
}
