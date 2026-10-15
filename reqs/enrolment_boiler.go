package reqs

import "regexp"

// Recurring enrolment sentences that are advice or explanation, never a
// requirement. Each becomes a note. They are checked after the sentences that
// carry structure (kind, threshold, variable cutoff, leads, floors), so a
// floor sentence followed by "Please note that obtaining this minimum ... does
// not guarantee admission" keeps its floor: the two are separate sentences.
var enrolBoilerRes = []*regexp.Regexp{
	// "Students in this program have the option to request enrolment in the
	// Arts & Science Internship Program (ASIP) stream. ... Students applying
	// for Year 3 entry must have ... completed RLG200H1 prior to ASIP entry."
	// Its Year-3 course conditions are for the ASIP stream, not the program.
	regexp.MustCompile(`(?i)Arts\s*&\s*Science\s+Internship\s+Program|\bASIP\b`),
	// "Please consult the Arts & Science Program Toolkit for more information
	// on program enrolment."
	regexp.MustCompile(`(?i)Program\s+Toolkit`),
	// "Applications from students lacking numerical grades in one or more of
	// the specified prerequisites, or students applying in their second or
	// later years, may be considered on the basis of their entire academic
	// record ...", "In addition to applying through ACORN, students applying
	// in their second year or later are asked to please contact ..."
	regexp.MustCompile(`(?i)lacking\s+(?:a\s+)?numerical\s+grades?|do\s+not\s+have\s+numerical\s+grades|applying\s+in\s+their\s+second\s+year\s+or\s+later`),
	// "Please note that obtaining this minimum final grade does not guarantee
	// admission to the program.", "Meeting the minimum admission requirements
	// does not guarantee entrance to the program."
	regexp.MustCompile(`(?i)\bdo(?:es)?\s+not\s+(?:necessarily\s+)?guarantee\b`),
	// "The underlying numeric final mark in each of your courses required for
	// program enrolment will be used by the relevant academic unit ..."
	regexp.MustCompile(`(?i)numeric\s+final\s+mark`),
	// "While it is difficult to predict what will be competitive course marks
	// ...", "In past admission cycles, the average grade cut-off ... ranged
	// from approximately 77-79%", "Admission is competitive and in recent
	// years the combined average ... has been in the 68% to 70% range",
	// "Admission to Rotman Commerce is competitive."
	regexp.MustCompile(`(?i)difficult\s+to\s+predict|in\s+past\s+admission\s+cycles|\bin\s+recent\s+years\b|\bis\s+competitive\b|extremely\s+competitive`),
	// "Due to the limited enrolment nature of this program, students are
	// strongly advised to apply to backup programs."
	regexp.MustCompile(`(?i)backup\s+programs`),
	// "Students admitted to the program after second or third year will be
	// required to pay retroactive deregulated program fees."
	regexp.MustCompile(`(?i)retroactive|program\s+fees`),
	// "Students applying for admissions to the program utilizing transfer
	// credits will be considered on a case-by-case basis. Students entering
	// from CEGEP ..."
	regexp.MustCompile(`(?i)transfer\s+credits?|\bCEGEP\b|case[-\s]+by[-\s]+case`),
	// Substitutions the department decides on request: "Students are invited
	// to inquire with the program office about possible appropriate
	// substitutions", "they can request approval from the unit to consider
	// other relevant courses", "we will not accept AP or IB Psychology", "We
	// will accept equivalent/exclusion statistics courses from UTM/UTSC in
	// lieu of PSY201H1", "MGT100H1 will be considered equivalent to RSM100H1".
	regexp.MustCompile(`(?i)\bsubstitutions?\b|request\s+approval|we\s+will\s+(?:not\s+)?accept\s+(?:AP|equivalent)|considered\s+equivalent`),
	// Contacts and websites: "please contact the department directly at
	// physiology.undergrad@utoronto.ca", "For information regarding admission
	// to the program, contact Rotman Commerce", "Please reach out to Rotman
	// Commerce Academic Services for more details."
	regexp.MustCompile(`(?i)\bcontact(?:ing)?\b|\be-?mail\b|\bwebsites?\b|https?://|www\.|\bconsult\b|reach\s+out|@utoronto`),
	// Focus information: "Enrolment in this Focus is recommended after second
	// year ...", "Please note normal course prerequisites will apply ...",
	// "Students in a focus complete the requirements of First Year, ...",
	// "... active participant in the faculty-led learning community ...",
	// "Students admitted to the Focus will be enrolled in an industry-engaged
	// version of CSC207H1".
	regexp.MustCompile(`(?i)recommended\s+after\s+second\s+year|normal\s+course\s+prerequisites|students\s+in\s+a\s+focus\s+complete|learning\s+community|industry-engaged|students\s+can\s+only\s+apply\s+for\s+one\s+focus|focuses\s+have\s+a\s+limited\s+enrolment`),
	// How enrolment courses count later: "only 0.5 credit will count toward
	// the completion requirements", "IRW100H1 cannot count for the First Year
	// or elective program requirements", "as double-counting courses is not
	// allowed", "IRW230H1 cannot be used for enrolment/program requirements in
	// Economics", "Students may use IRW100H1 to meet the admission
	// requirement ...".
	regexp.MustCompile(`(?i)double-?counting|(?:will|can)\s+(?:only\s+)?(?:count|be\s+counted)\s+toward|cannot\s+count\s+for|cannot\s+be\s+used\s+for\s+enrolment|may\s+be\s+used\s+for\s+requesting|students\s+may\s+use\s+` + codePat),
	// "Note there are different options depending on whether a student has
	// completed between 4.0 and 8.5 credits, or 9.0 or more credits.", "There
	// are two program request periods ...", "students are strongly advised to
	// apply in the first program request period".
	regexp.MustCompile(`(?i)there\s+are\s+different\s+(?:options|admission\s+criteria)|there\s+are\s+two\s+program\s+request\s+periods|strongly\s+advised\s+to\s+apply\s+in\s+the\s+first`),
	// Admission-guarantee rules the checker can't see: "Students may repeat
	// any required Guaranteed Admission course once ...", "Any posted grade
	// ... constitutes an attempt", "none of the required courses may be
	// completed using the CR/NCR option", "Students who do not achieve the
	// minimum required grade in MAT133Y1, ... after two attempts within 12
	// months are not permitted to satisfy the requirement by taking an
	// alternative mathematics course", "Students admitted with the guarantee
	// are invited to enrol in the program after first year ...".
	regexp.MustCompile(`(?i)\bCR/NCR\b|\brepeat\s+any\b|repeated\s+attempts|constitutes\s+an\s+attempt|count\s+as\s+an\s+attempt|attempted\s+a\s+required|not\s+permitted\s+to\s+satisfy|may\s+not\s+extend\s+the\s+guarantee|admitted\s+with\s+the\s+guarantee|with\s+an\s+admission\s+guarantee|A\s+limited\s+number\s+of\s+spaces`),
	// The best-grades rule: "If more than 1.0 credits have been completed
	// from LIN, CSC, PHL, or PSY, the grade average will be based on the
	// higher course grades.", "the two with the highest grades will be used".
	regexp.MustCompile(`(?i)based\s+on\s+the\s+higher\s+course\s+grades?|with\s+the\s+highest\s+grades\s+will\s+be\s+used|have\s+completed\s+more\s+than\s+one\s+of\s+the\s+above`),
	// Explanations of the admission process: "Admission will be based on
	// marks and the Rotman Commerce Supplemental Application.", "Admission to
	// the program is based on strong academic performance and expressed
	// interest in research.", "Admission is determined by ..." is handled as
	// an application (see enrolAdmissionRe).
	regexp.MustCompile(`(?i)^admission\s+(?:will\s+be|is)\s+based\s+on`),
	// Advice: "Students are encouraged to take their introductory calculus
	// ...", "Students interested in the Sociology Specialist typically apply
	// and enroll from the Sociology Major.", "Students wishing to enroll after
	// their second year ...", "Interested students should ideally apply ...".
	regexp.MustCompile(`(?i)\bencouraged\b|\btypically\b|\bideally\b|\bshould\s+(?:initially\s+)?(?:apply|enrol)|students\s+wishing\s+to|will\s+have\s+the\s+opportunity|have\s+the\s+option\s+of|\bdemanding\b|strong\s+applications|prior\s+research\s+experience|limits\s+future\s+enrolment|prerequisites?\s+for\s+many|strongly\s+advised\s+to\s+enrol|students\s+who\s+take\s+.*\btypically\b|restricted\s+to\s+first-year\s+students|will\s+be\s+used\s+in\s+place\s+of|responsible\s+for\s+checking|not\s+all\s+courses\s+are\s+offered|must\s+meet\s+prerequisites|in\s+order\s+to\s+pursue|no\s+specific\s+course\s+requirements|will\s+work\s+with\s+any\s+student|missing\s+Gr(?:ade|\.)\s+12`),
	// A stale banner for another program: "The Health Studies Major
	// (ASMAJ2085) was last available for enrolment during the second round of
	// program enrolment in Summer 2024."
	regexp.MustCompile(`(?i)last\s+available\s+for\s+enrolment|archived\s+\d{4}-\d{2}\s+Academic\s+Calendar`),
	// The program's own description of itself: "this is a limited enrollment
	// program that can accommodate a limited number of students", "enrolment
	// is restricted to no more than 15 students per year".
	regexp.MustCompile(`(?i)restricted\s+to\s+no\s+more\s+than|welcome\s+students\s+with\s+diverse`),
	// Studio courses in an open program: "completion of the Studio (Group B)
	// requirement will normally require either an audition, ..."
	regexp.MustCompile(`(?i)Studio\s+(?:courses\s+)?\(Group\s+B\)`),
	// Grade adjustments and other notes on how grades are read: "Grades in
	// MAT158H1, MAT159H1, and MAT157Y1 will be adjusted to account for the
	// greater difficulty of those courses."
	regexp.MustCompile(`(?i)will\s+be\s+adjusted\s+to\s+account`),
	// "Upon acceptance to the Economics Specialist Program, your Specialist
	// enrolment will replace your Economics Major.", "ASIP students enrolled
	// in the Economics Major can transfer ..."
	regexp.MustCompile(`(?i)will\s+replace\s+your|can\s+transfer\s+to|may\s+not\s+transfer\s+to|successful\s+completion\s+of\s+(?:required\s+)?pre-?requisite\s+courses\s+is\s+required`),
	// Timing advice: "Students apply at the end of their second year and begin
	// the program in their third.", "All students accepted into or
	// considering the Research Specialist program should enrol into a
	// 300-level PSY lab course".
	regexp.MustCompile(`(?i)students\s+apply\s+at\s+the\s+end\s+of|should\s+enrol\s+into|accept\s+the\s+invitation|request\s+program\s+code|fill\s+out\s+the\s+online`),
	// "Credit for higher-level SOC courses will not waive these requirements"
	regexp.MustCompile(`(?i)will\s+not\s+waive`),
	// Details of an application: "The interview is held annually in
	// April-May.", "Please refer to MUS120Y1.", "Application information will
	// be provided to all ... applicants during the Round 1 program admission
	// period."
	regexp.MustCompile(`(?i)\bis\s+held\s+annually|\brefer\s+to\b|application\s+information\s+will\s+be\s+provided|detailed\s+information`),
	// Who the guaranteed admission is for, said in prose: "Students are
	// admitted to Year 1 of the Faculty of Arts & Science in the Rotman
	// Commerce admission category ..."
	regexp.MustCompile(`(?i)^students\s+are\s+admitted\s+to\s+Year\s+1`),
}

// enrolNoiseRe matches sentences that say nothing beyond what the structure
// already says: "Students must meet the requirements listed below to enrol."
var enrolNoiseRe = regexp.MustCompile(`(?i)^students\s+must\s+meet\s+the\s+requirements?\s+listed\s+below|^(?:a\s+)?minimum\s+grades?(?:\s+(?:and|or)\s+(?:a\s+)?(?:minimum\s+)?grade\s+average)?\s+(?:is|are)\s+needed\s+for\s+entry$`)

func isEnrolBoiler(s string) bool {
	for _, re := range enrolBoilerRes {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}
