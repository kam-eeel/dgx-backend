package enrolment

import (
	"regexp"

	"degree-explorer/parser/grammar"
	"degree-explorer/parser/programs"
)

// boilerRes match enrolment sentences that are advice, never requirements;
// each becomes a note. They are checked after the sentences that carry
// structure, so a floor followed by "... does not guarantee admission" keeps it.
var boilerRes = []*regexp.Regexp{
	// the ASIP stream: its Year-3 course conditions are the stream's, not the
	// program's
	programs.AsipRe,
	// "Please consult the Arts & Science Program Toolkit"
	regexp.MustCompile(`(?i)Program\s+Toolkit`),
	// "Applications from students lacking numerical grades ... may be considered"
	regexp.MustCompile(`(?i)lacking\s+(?:a\s+)?numerical\s+grades?|do\s+not\s+have\s+numerical\s+grades|applying\s+in\s+their\s+second\s+year\s+or\s+later`),
	// "Meeting the minimum admission requirements does not guarantee entrance"
	regexp.MustCompile(`(?i)\bdo(?:es)?\s+not\s+(?:necessarily\s+)?guarantee\b`),
	// "The underlying numeric final mark in each of your courses"
	regexp.MustCompile(`(?i)numeric\s+final\s+mark`),
	// past cut-offs: "In past admission cycles, the average grade cut-off ..."
	regexp.MustCompile(`(?i)difficult\s+to\s+predict|in\s+past\s+admission\s+cycles|\bin\s+recent\s+years\b|\bis\s+competitive\b|extremely\s+competitive`),
	// "students are strongly advised to apply to backup programs"
	regexp.MustCompile(`(?i)backup\s+programs`),
	// "will be required to pay retroactive deregulated program fees"
	regexp.MustCompile(`(?i)retroactive|program\s+fees`),
	// "utilizing transfer credits will be considered on a case-by-case basis"
	regexp.MustCompile(`(?i)transfer\s+credits?|\bCEGEP\b|case[-\s]+by[-\s]+case`),
	// substitutions the department decides on request: "we will not accept AP or
	// IB Psychology"
	regexp.MustCompile(`(?i)\bsubstitutions?\b|request\s+approval|we\s+will\s+(?:not\s+)?accept\s+(?:AP|equivalent)|considered\s+equivalent`),
	// contacts and websites: "please contact the department directly"
	regexp.MustCompile(`(?i)\bcontact(?:ing)?\b|\be-?mail\b|\bwebsites?\b|https?://|www\.|\bconsult\b|reach\s+out|@utoronto`),
	// focus information: "Enrolment in this Focus is recommended after second
	// year"
	regexp.MustCompile(`(?i)recommended\s+after\s+second\s+year|normal\s+course\s+prerequisites|students\s+in\s+a\s+focus\s+complete|learning\s+community|industry-engaged|students\s+can\s+only\s+apply\s+for\s+one\s+focus|focuses\s+have\s+a\s+limited\s+enrolment`),
	// how enrolment courses count later: "as double-counting courses is not
	// allowed"
	regexp.MustCompile(`(?i)double-?counting|(?:will|can)\s+(?:only\s+)?(?:count|be\s+counted)\s+toward|cannot\s+count\s+for|cannot\s+be\s+used\s+for\s+enrolment|may\s+be\s+used\s+for\s+requesting|students\s+may\s+use\s+` + grammar.CodePat),
	// "There are two program request periods ..."
	regexp.MustCompile(`(?i)there\s+are\s+different\s+(?:options|admission\s+criteria)|there\s+are\s+two\s+program\s+request\s+periods|strongly\s+advised\s+to\s+apply\s+in\s+the\s+first`),
	// admission-guarantee rules the checker can't see: "none of the required
	// courses may be completed using the CR/NCR option"
	regexp.MustCompile(`(?i)\bCR/NCR\b|\brepeat\s+any\b|repeated\s+attempts|constitutes\s+an\s+attempt|count\s+as\s+an\s+attempt|attempted\s+a\s+required|not\s+permitted\s+to\s+satisfy|may\s+not\s+extend\s+the\s+guarantee|admitted\s+with\s+the\s+guarantee|with\s+an\s+admission\s+guarantee|A\s+limited\s+number\s+of\s+spaces`),
	// the best-grades rule, which bestGradesRe reads: "the grade average will be
	// based on the higher course grades"
	regexp.MustCompile(`(?i)based\s+on\s+the\s+higher\s+course\s+grades?|with\s+the\s+highest\s+grades\s+will\s+be\s+used|have\s+completed\s+more\s+than\s+one\s+of\s+the\s+above`),
	// "Admission will be based on marks and ..."; "Admission is determined by" is
	// an application
	regexp.MustCompile(`(?i)^admission\s+(?:will\s+be|is)\s+based\s+on`),
	// advice: "Students are encouraged to take their introductory calculus ..."
	regexp.MustCompile(`(?i)\bencouraged\b|\btypically\b|\bideally\b|\bshould\s+(?:initially\s+)?(?:apply|enrol)|students\s+wishing\s+to|will\s+have\s+the\s+opportunity|have\s+the\s+option\s+of|\bdemanding\b|strong\s+applications|prior\s+research\s+experience|limits\s+future\s+enrolment|prerequisites?\s+for\s+many|strongly\s+advised\s+to\s+enrol|students\s+who\s+take\s+.*\btypically\b|restricted\s+to\s+first-year\s+students|will\s+be\s+used\s+in\s+place\s+of|responsible\s+for\s+checking|not\s+all\s+courses\s+are\s+offered|must\s+meet\s+prerequisites|in\s+order\s+to\s+pursue|no\s+specific\s+course\s+requirements|will\s+work\s+with\s+any\s+student|missing\s+Gr(?:ade|\.)\s+12`),
	// a stale banner: "... was last available for enrolment during the second
	// round"
	regexp.MustCompile(`(?i)last\s+available\s+for\s+enrolment|archived\s+\d{4}-\d{2}\s+Academic\s+Calendar`),
	// "enrolment is restricted to no more than 15 students per year"
	regexp.MustCompile(`(?i)restricted\s+to\s+` + grammar.NoMoreThanPat + `|welcome\s+students\s+with\s+diverse`),
	// "completion of the Studio (Group B) requirement will normally require"
	regexp.MustCompile(`(?i)Studio\s+(?:courses\s+)?\(Group\s+B\)`),
	// "Grades in MAT158H1 ... will be adjusted to account for the greater
	// difficulty"
	regexp.MustCompile(`(?i)will\s+be\s+adjusted\s+to\s+account`),
	// "your Specialist enrolment will replace your Economics Major"
	regexp.MustCompile(`(?i)will\s+replace\s+your|can\s+transfer\s+to|may\s+not\s+transfer\s+to|successful\s+completion\s+of\s+(?:required\s+)?pre-?requisite\s+courses\s+is\s+required`),
	// timing: "Students apply at the end of their second year"
	regexp.MustCompile(`(?i)students\s+apply\s+at\s+the\s+end\s+of|should\s+enrol\s+into|accept\s+the\s+invitation|request\s+program\s+code|fill\s+out\s+the\s+online`),
	// "Credit for higher-level SOC courses will not waive these requirements"
	regexp.MustCompile(`(?i)will\s+not\s+waive`),
	// application details: "The interview is held annually in April-May."
	regexp.MustCompile(`(?i)\bis\s+held\s+annually|\brefer\s+to\b|application\s+information\s+will\s+be\s+provided|detailed\s+information`),
	// "Students are admitted to Year 1 of the Faculty of Arts & Science in the
	// Rotman Commerce admission category"
	regexp.MustCompile(`(?i)^students\s+are\s+admitted\s+to\s+Year\s+1`),
}

// noiseRe matches sentences that say nothing the structure doesn't:
// "Students must meet the requirements listed below to enrol."
var noiseRe = regexp.MustCompile(`(?i)^students\s+must\s+meet\s+the\s+requirements?\s+listed\s+below|^(?:a\s+)?minimum\s+grades?(?:\s+(?:and|or)\s+(?:a\s+)?(?:minimum\s+)?grade\s+average)?\s+(?:is|are)\s+needed\s+for\s+entry$`)
