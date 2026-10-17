package reqs

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"degree-explorer/types"
)

// sentenceRole says what one sentence of program completion text does. The
// classifier looks at the sentence alone: the assembler combines its role with
// the kind of block the sentence sits in. Cue words beat block context, since
// a "Notes:" block is binding in most programs.
type sentenceRole int

const (
	roleRequirement sentenceRole = iota // an allocation, an item or a heading: everything binding without a more specific role
	roleConstraint                      // a floor or a must-include on courses counted elsewhere
	roleLimit                           // "no more than": at most an amount counts, the excess is dropped
	roleListDef                         // a named course list: "Group A: ..."
	roleAdvisory                        // advice, contacts, procedure and timing: notes
	roleExclusion                       // programs that can't be combined with this one
	roleTransfer                        // a cap or ban on transfer credit
	roleCrNcr                           // a rule on courses taken CR/NCR
	roleOverlap                         // a course may count toward two named requirements
)

var sentenceRoleNames = [...]string{"requirement", "constraint", "limit", "listDef", "advisory", "exclusion", "transfer", "crNcr", "overlap"}

func (r sentenceRole) String() string {
	if r >= 0 && int(r) < len(sentenceRoleNames) {
		return sentenceRoleNames[r]
	}
	return fmt.Sprintf("sentenceRole(%d)", int(r))
}

const complAmountPat = `\d+(?:\.\d+)?(?:\s*[-–]\s*\d+(?:\.\d+)?)?`

var (
	// List markers, footnote stars and note labels in front of a sentence:
	// "- ", "1. ", "a. ", "ii) ", "(1) ", "* ", "Note:", "NB:", "Note 2:".
	complLeadRe = regexp.MustCompile(`^(?:[-•*]+\s*|\(?(?:\d{1,2}|[a-zA-Z]|[ivx]{1,4})[.)]\s+|(?i:notes?(?:\s+\d)?|nb|important)\s*:\s*)+`)
	// "N.B." before a sentence; written as a label so it doesn't end one.
	complNBRe = regexp.MustCompile(`\bN\.\s?B\.\s*`)
	// A numbered list marker, which splitSentences would take for a
	// sentence: "5. ", "a. ".
	complMarkerRe = regexp.MustCompile(`^(?:\d{1,2}|[a-zA-Z])\.\s+`)
	// A full stop before an amount that starts the next sentence, with or
	// without its space: "count towards this requirement. 1.0 out of the 3.5
	// credits", "NMC and/or NML courses.1.0 credit may be taken", "(RLG, ...,
	// or JSR designators). 3.0 credits must be taken"; before a level
	// ("GGR199H1. 200/300-level GGR courses may also be used"); or after a
	// code before a bracketed one: "CSC148H1. ( MAT130H1/ MAT135H1, MAT136H1)
	// ... is strongly recommended". splitSentences only splits before an
	// uppercase letter.
	complAmountStartRe = regexp.MustCompile(`((?:[a-z]+\.)*[a-z]+|\)|` + codePat + `)\.(?:\s+\d+\.\d+\s|\d+\.\d+\s+(?:[A-Z]{3}\s+)?credits?\b|\s+\(\s*` + codePat + `|\s+[1-4]00(?:/[1-4]00)*(?:\+|-|\s+level))`)
	// Inline lettered notes: "(a) ...; (b) ...; (c) ...". Each letter after
	// the first starts a sentence.
	complLetterNoteRe = regexp.MustCompile(`;\s*\([b-z]\)\s`)
	// An italic note glued to an item with no full stop: "( CHM135H1 and
	// CHM136H1)/ CHM151Y1 _Transfer credits will be accepted ..._".
	complItalicNoteRe = regexp.MustCompile(`(` + codePat + `|\))\s+(?:\*\*)?_(?:\*\*)?([A-Z])`)
	// Bracketed text, removed before the advice checks: "( CHM151Y1 strongly
	// recommended)" doesn't make the item around it advice.
	complParenRe = regexp.MustCompile(`\([^()]*\)|\[[^\[\]]*\]`)
	// A heading marked as advice: "First year (recommended):". With no code
	// in the sentence, the bracket isn't an item's annotation.
	complRecommendedTagRe = regexp.MustCompile(`(?i)\((?:strongly\s+|highly\s+)?recommended\)`)
	// "recommended" or "suggested" naming a set of courses, removed before the
	// advice checks so it can't silence a cap: "No more than 1.0 credit from
	// the recommended electives may count towards the program", "No more than
	// 1.0 credit from recommended courses may count" (both constructed).
	complAttributiveAdviceRe = regexp.MustCompile(`(?i)\b(?:the|these|those|of|from)\s+(?:list\s+of\s+)?(?:recommended|suggested)\s+(?:[\w-]+\s+)?(?:courses|lists?|electives)\b`)
	// A sentence with no letter or digit says nothing ...
	complWordRe = regexp.MustCompile(`[A-Za-z0-9]`)
	// ... nor does a list marker alone, left after cleaning: "1." gives "1",
	// "a)" stays, "(i)" gives "i".
	complBareMarkerRe = regexp.MustCompile(`^(?:\d{1,2}|[a-zA-Z]|[ivxIVX]{1,4})[.)]?$`)

	complCodeRe     = regexp.MustCompile(codePat)
	complCodeLeadRe = regexp.MustCompile(`^(?i:in\s+addition,?\s+)?[(\s]*` + codePat)
	complAmountRe   = regexp.MustCompile(`\b\d+\.\d+\b|\b\d+\s+(?:full|half)[-\s]courses?\b|(?i:\b(?:one|two|three|four)\s+years?\s+of\s+study\b)`)
	// Filters a requirement can carry: levels, breadth, designators, lists.
	complFilterRe = regexp.MustCompile(`\b[1-4]00(?:\+|\s*-?\s*(?:level|series)|\s*-?\s*/)|\bbreadth\b|\bBR\s*=|\b[A-Z]{3}(?:\s*/\s*[A-Z]{3})*\s*-?\s*(?:courses?|credits?|prefix|designators?|coded)\b|\b(?:Groups?|Lists?|Clusters?)\s+[A-Z0-9]\b`)
	// Verbs that bind even without a code or an amount.
	complBindingRe = regexp.MustCompile(`(?i)\bmust\b|\brequire[sd]?\b|\bcannot\b|\bmay\s+not\b|\bwill\s+not\b|\bnot\s+(?:be\s+)?(?:accepted|counted|permitted|eligible|included)\b|\bonly\b|\bno\s+(?:more|other|course|courses|credits?)\b|\bwill\s+count\b`)
	// A heading: short, with no full stop and no verb from a closed list ("Data
	// Analysis and Courses in Advanced Research, Laboratory, or Cellular
	// Molecular Topics"). Short text that misses the list defaults to
	// requirement on purpose: a heading taken for advice would drop the
	// requirements under it.
	complHeadingRe     = regexp.MustCompile(`^[^.]{1,100}$`)
	complHeadingVerbRe = regexp.MustCompile(`(?i)\b(?:is|are|was|were|will|may|might|can|should|must|has|have|need|needs)\b`)

	// Programs that can't be combined, beyond the enrolment phrasings: "The
	// Neuroscience major cannot be paired with any other Human Biology
	// Program managed major program", "Students may enrol in only one Rotman
	// Commerce Specialist", "Students are not allowed to enroll concurrently
	// in the Major Program in Pharmacology and the Major Program in
	// Toxicology", "cannot also complete the Applied Genetics and
	// Biotechnology Major", "cannot be enrolled in any other program in
	// Economics", "may not simultaneously be registered for a German Minor".
	// "may not be combined with" has no match in the calendar; the task's cue
	// table names it as enrolment's cannot-combine phrasing, so it stays.
	complCombineRe    = regexp.MustCompile(`(?i)\bmay\s+not\s+be\s+combined\s+with\b|\bcannot\s+be\s+paired\s+with\b|\bmay\s+(?:only\s+)?enrol(?:l)?\s+in\s+only\s+one\b|\bnot\s+(?:be\s+)?allowed\s+to\s+enrol(?:l)?\s+concurrently\b|\bcannot\s+also\s+complete\b|\bcannot\s+be\s+enrol(?:l)?ed\s+in\s+any\s+other\s+program\b|\bmay\s+not\s+simultaneously\s+(?:be\s+)?(?:registered|enrol(?:l)?(?:ed)?)\s+(?:for|in)\b`)
	complExclusionRes = []*regexp.Regexp{complCombineRe, enrolCombinedWithRe, enrolCombineRe, enrolSimultaneousRe, enrolNotOpenRe}

	// "Some courses may contribute to both the Canadian Politics and
	// Diversity requirements". "Advanced courses ... can be used as credits
	// for both foundation sciences sections, meaning, half of the credit will
	// count for ..." is a weight split, not sharing. "Some offerings of
	// HIS357H1 ... may also count towards this focus" and "... may also count
	// towards requirement 2 of this focus at the department's discretion" are
	// membership, not sharing: every "may also count toward" in the calendar
	// is.
	complOverlapRe = regexp.MustCompile(`(?i)\b(?:may|can)\s+(?:also\s+)?(?:contribute|count|be\s+(?:used|counted|applied))\s+(?:to(?:wards?)?|for|to\s+satisfy)\s+both\b`)

	complCrNcrRe = regexp.MustCompile(`(?i)\bCR\s*/\s*NCR\b|\bcredit\s*/\s*no\s+credit\b`)
	// What makes a CR/NCR mention a rule, not a pointer to details.
	complCrNcrRuleRe = regexp.MustCompile(`(?i)\bcannot\b|\bmay\s+not\b|\bnot\s+(?:be\s+)?accepted\b|\bdo\s+not\s+accept\b|\bmay\s+(?:be\s+)?(?:use|used|accepted|taken)\b|\bcan\s+be\s+taken\b|\bonly\b|\bno\s+courses\b`)

	// A CR/NCR permission with no ban, which is the default (schema): "Students
	// may use CR/NCR on the course they use toward the ethics and social
	// responsibility credit", "Students may use the CR/NCR option towards
	// any of the courses listed in Requirement 1".
	complCrNcrAllowRe = regexp.MustCompile(`(?i)^students\s+may\s+use\s+(?:the\s+)?CR\s*/\s*NCR\s+(?:option\s+)?(?:on|towards?|for)\s+(?:the|any)\b[^;]*$`)

	complTransferRe = regexp.MustCompile(`(?i)\btransfer\s+credits?\b|\bin\s+transfer\s+credit\b`)
	// A cap or ban on transfer credit. "Only transfer credits that carry
	// exclusions to CHM135H1 will be accepted" is neither: the course is
	// required either way.
	complTransferRuleRe = regexp.MustCompile(`(?i)\bno\s+more\s+than\b|\bmaximum\b|\bmore\s+than\s+\d|\bup\s+to\b|\bcannot\b|\bmay\s+not\b|\bnot\s+(?:be\s+)?accepted\b|\bwill\s+not\b`)

	// A list label: "Group A:", "List 2:", "Cluster B - Learning,
	// Perception, ...", "Group C (Modern Stream)", "Disciplinary/Thematic
	// Cluster 1: Politics and Economics", "Group I:". It names a list even
	// with a cap in it: "Group C: Environmental Policy & Society (no more than
	// 0.5 credit from Group C)".
	complListLabelRe = regexp.MustCompile(`^(?:[\w/&-]+\s+){0,2}(?:Group|List|Cluster|Category|Sub-?group|Area)\s+(?:[A-Z]|\d{1,2}|[IVX]{1,4}|[a-z]\))(?:\s*$|\s*[-–:(])`)
	// "From Group A: 1.0 credit or DRM220Y1" draws on a list; it doesn't
	// name one.
	complFromLabelRe = regexp.MustCompile(`(?i)^(?:from|in|of)\b`)
	// "Hydrology/GIS/Remote Sensing: GGR273H1, ...", "The following courses
	// fulfill the Diversity and Identity requirement: POL101H1, ...": a label
	// followed by codes. Not a section ("First Year: FAH101H1/ FAH102H1") or a
	// choice ("One of: ...", "Any two of: ..."): those are items.
	complListHeadRe    = regexp.MustCompile(`^([^:.]{1,120}):\s*\(?\s*` + codePat)
	complNotListHeadRe = regexp.MustCompile(`(?i)\byears?\b|\b(?:all|one|two|three|four|any|\d)\b(?:\s+\w+)?\s+(?:of|from)\b|` + codePat)

	// A sentence that leads with an amount allocates it, after an optional
	// label ("Applications: Any 5 courses (2.5 credits) from Group A",
	// "Required Courses (12.0 credits, ...)"). A floor or a cap in a later
	// clause ("2.0 credits from Groups A-C, with no more than 0.5 from Group
	// C") belongs to that allocation; the item grammar splits it off.
	complAllocLeadRe = regexp.MustCompile(`(?i)^(?:[^:.()]{1,60}:\s*|[^:.()]{1,40}\(\s*)?` +
		`(?:\(\s*|in\s+addition,?\s+|any\s+|all\s+|at\s+least\s+|(?:an\s+)?additional\s+|a\s+total\s+of\s+|a\s+minimum\s+of\s+|complete\s+|` +
		`students\s+(?:must\s+)?complete\s+(?:a\s+total\s+of\s+)?|the\s+(?:[\w&]+\s+){1,3}requires\s+)*` +
		`(?:(?:a\s+|one\s+)?` + complAmountPat + `\s+(?:\(\s*\d+(?:\.\d+)?\s+credits?\s*\)\s+)?(?:[\w/+-]+\s+){0,3}?(?:credits?|(?:half|full)[-\s]courses?|courses?)\b` +
		`|total(?:\s+credits)?\s*:\s*` + complAmountPat +
		`|[^.;:]{0,80}?\b(?:to|for)\s+a\s+total\s+of\b` +
		`|(?:an?\s+)?additional\s+(?:[\w/+-]+\s+){0,3}?(?:credits?|courses?)\b` +
		`|select\s+the\s+remaining\s+credits\b)`)
	// The first clause of an amount lead ends at punctuation or a
	// subordinate "of which", "including", "with" ...
	complClauseEndRe = regexp.MustCompile(`(?i)[,;:(]|\b(?:of\s+which|including|with|to\s+include)\b`)
	// ... or, after a numeric amount, a relative clause: "2.0 credits in
	// courses where the primary language of study is not English". Not
	// after "Additional courses that may strengthen your background ... can
	// be taken, but will not count".
	complRelativeRe = regexp.MustCompile(`(?i)\b(?:where|that|which|who|whose)\b`)
	// A cue in the lead itself makes it a rule, not an allocation: "No more
	// than 1.0 credit ... to a total of".
	complLeadCueRe = regexp.MustCompile(`(?i)\b(?:no\s+more|at\s+most|maximum|only|up\s+to|may|can|cannot)\b`)
	// "1.0 credit must be taken from GER270H1/ GER272H1/ ..." allocates, among
	// sibling items; "3.0 credits must be taken at the 300-level" is a floor.
	complTakenFromRe = regexp.MustCompile(`(?i)^\s*must\s+be\s+(?:taken|chosen|selected)\s+from\s+(?:the\s+following:?\s*)?` + codePat)
	// A verb in the first clause makes the amount a subject, not an
	// allocation: "1.0 credit must be at the 300-level" is a floor,
	// "Additional BCH research project courses can be taken ... but cannot be
	// counted" a cap.
	complLeadVerbRe = regexp.MustCompile(`(?i)\b(?:must|should|needs?\s+to|can|cannot|may|will|is|are|counts?)\b`)

	// Constraint cues: a floor or a must-include on courses counted under
	// another requirement. "at least" binds only with a modal; a bare "at
	// least 1.0 credit from Group A" is a constraint only by block context
	// (under "These 3.0 credits must include:").
	complConstraintRes = []*regexp.Regexp{
		// "These 3.0 credits must include:"
		regexp.MustCompile(`(?i)\bmust\s+include\b`),
		// "Higher Years (must include ...), including at least 0.5 at ..."
		regexp.MustCompile(`(?i)\bincluding\s+at\s+least\b`),
		// "At least 2.0 credits at the 300+ level, 0.5 of which must be ..."
		regexp.MustCompile(`(?i)(?:^|,|\b(?:credits?|courses?|\d+(?:\.\d+)?|one))\s*of\s+which\b`),
		// "The choices in 3 must satisfy the requirement for an integrative,
		// inquiry-based activity by including one of ..."
		regexp.MustCompile(`(?i)\bthe\s+choices\s+(?:in|from)\s+\S+(?:\s+\S+)?\s+must\b`),
		// "At least 1.5 credits must be in FRE or FSL courses"
		regexp.MustCompile(`(?i)\bat\s+least\b[^.]*?\b(?:must|needs?\s+to|has\s+to|have\s+to|is\s+required)\b`),
		// "All 4.0 credits must be NMC and/or NML courses"
		regexp.MustCompile(`(?i)\b(?:credits?|courses?)\s+(?:must|needs?\s+to)\s+(?:be\s+(?:taken\s+)?(?:at|in|from|a|an|[1-4]00|[A-Z]{3})\b|come\s+from\b|have\s+the\b)`),
	}
	// "Of these 4.0 credits, ...", "Of the courses chosen, ...", "Of these:":
	// a scope. The cue after it decides ("up to 1.0 credit may be taken" is
	// a cap); with none, it is a constraint.
	complScopeRe = regexp.MustCompile(`(?i)^of\s+(?:these|the)\b`)

	// Limit cues: at most an amount counts.
	complLimitRes = []*regexp.Regexp{
		// "No more than 1.0 credit of 100-level courses may be used", "Transfer
		// credits ... cannot comprise more than 1.0 credit".
		regexp.MustCompile(`(?i)\bno\s+more\s+than\b|\bat\s+most\b|\bnot\s+exceed\b|\bcannot\s+comprise\s+more\s+than\b`),
		// "Regardless of exemptions, a maximum of 1.0 total program credits may
		// be drawn from courses taught in English".
		regexp.MustCompile(`(?i)\bmaximum\s+(?:of\s+)?(?:a\s+|one\s+|two\s+|\d)`),
		// "Please note only 0.5 of ECO220Y1 will be accepted towards the
		// program", "Only 1.0 credit from 100-level ENG courses may be counted".
		regexp.MustCompile(`(?i)\bonly\s+(?:a\s+)?(?:maximum\s+of\s+)?(?:\d+(?:\.\d+)?|one|two)\b[^.]*?\b(?:will|may|can)\s+(?:be\s+)?(?:accepted|counted|count|used|applied)\b`),
		// "Up to 1.0 credit can be used from GGR493Y1, based on suitability of
		// placement for this program".
		regexp.MustCompile(`(?i)\bup\s+to\s+(?:a\s+|one\s+|two\s+)?(?:\d+(?:\.\d+)?\s+)?(?:credits?|courses?|half|full)?[^.]*?\b(?:can|may)\b`),
		// "MAT221H1 may not be used for this requirement".
		regexp.MustCompile(`(?i)\b(?:cannot|may\s+not|will\s+not)\s+(?:count|be\s+(?:counted|used|applied|accepted))\s+(?:to|for|toward|towards)\b`),
		// "ENG100H1, ENG102H1, and courses with a CR/NCR final mark are not
		// accepted toward any English program", "STA220H1 and STA255H1 are not
		// accepted for the major", "Courses taught at UTM or UTSC are not
		// included for consideration", "Students are not permitted to take
		// PCL472Y1 or PCL474Y1".
		regexp.MustCompile(`(?i)\bnot\s+(?:be\s+)?(?:accepted|counted)\s+toward|\bdo\s+not\s+accept\b|\bare\s+not\s+accepted\s+for\b|\bnot\s+included\s+for\s+consideration\b|\bnot\s+permitted\s+to\s+take\b`),
		// "Students may not use more than 2.0 credits towards their program
		// from ...", "Students cannot take more than a 0.5 credit at the
		// 400-level without written permission", "This requirement cannot be
		// met by more than 1.0 credit from any area".
		regexp.MustCompile(`(?i)\b(?:cannot|may\s+not|will\s+not)\s+(?:use|take|count|be\s+met\s+by)\s+more\s+than\b`),
	}

	// Advice: "We recommend that students take MAT148H1 ...", "First-year
	// students are encouraged, but not required, to take AST121H1",
	// "Suggested Related Courses:", "Students interested in actuarial
	// practices should consider taking ACT371H1". Not binding, like
	// complNotBindingRes, unless the sentence offers a substitution
	// (complSubstitutionRes).
	complRecommendRe = regexp.MustCompile(`(?i)\brecommend(?:s|ed|ation|ations)?\b|\bencouraged?\b|\bsuggest(?:s|ed|ion|ions)?\b|\badvis(?:ed|able)\b|\bshould\s+(?:especially\s+|seriously\s+|also\s+)?consider\b|\bshould\s+note\s+that\b|\b(?:may|might)\s+wish\b|\bpreferabl[ey]\b`)
	// Timing advice on a required item: "EAS209H1, highly recommended to be
	// taken in the second year", "EAS103H1 and EAS105H1, normally taken in
	// the first year". Removed before the advice checks.
	complTimingNoteRe = regexp.MustCompile(`(?i),\s*(?:(?:highly|strongly)\s+)?(?:recommended\s+to\s+be\s+taken|normally\s+taken)\b[^;]*`)

	// complAdviceVetoRe vetoes advice: a sentence with "must" binds, and so
	// does one that calls courses required for the program ("PHL271H1,
	// TRN203H1, and TRN204H1 are required courses that students are
	// recommended to take in second year", "courses which are required for
	// the specialist") ...
	complAdviceVetoRe = regexp.MustCompile(`(?i)\bmust\b|\b(?:are|is)\s+(?:an?\s+)?required\s+courses?\b|\brequired\s+for\s+the\s+(?:program|specialist|major|minor|focus|certificate)\b`)
	// ... unless the "must" is procedure or timing: "students must request
	// permission", "Requests must be reviewed", "must first complete FSL122H1
	// before starting this program", "before taking any fourth-year courses",
	// "A nomination ... must be submitted at least one semester in advance",
	// "Students must also consult with the Department", "Students must meet
	// all prerequisites for upper-year math courses" ...
	complProcedureRe = regexp.MustCompile(`(?i)\bmust\s+(?:also\s+)?(?:first\s+)?(?:request|submit|check|consult|contact|apply|speak|receive|obtain\s+(?:prior\s+)?(?:consent|permission)|meet\s+(?:with|(?:all\s+)?(?:the\s+)?(?:specified\s+)?prerequisites)|be\s+enrolled|be\s+made|be\s+submitted)\b|\brequests?\s+must\s+be\b|\bmust\b[^.]*\bbefore\s+(?:starting|taking|enrolling|beginning|graduation)\b`)
	// ... or a degree-level rule: "The University of Toronto requires that
	// any student who is using a combination of programs to graduate ... must
	// complete a total of at least 12.0 distinct credits", "may not comply
	// with this 12.0 credit rule", "this Focus requires completion of a 0.5
	// credit or more in addition to the 7.5 credits required for the
	// International Relations Major", "would be able to count these MGT
	// courses towards their Rotman Commerce degree".
	complDegreeRuleRe = regexp.MustCompile(`(?i)\bcombination\s+of\s+programs\b|\bdistinct\s+credits\b|\b\d+\.\d\s+credit\s+rule\b|\bin\s+addition\s+to\s+the\s+\d+(?:\.\d+)?\s+credits\s+required\s+for\b|\btowards?\s+their\s+(?:[\w&]+\s+){0,3}degree\b`)
	// ... or a description that names required courses without requiring
	// them here: "This list comprises courses which are required for the
	// specialist as well as suggested courses", "ACT390H1 is a required
	// course in the actuarial science specialist program" (another program),
	// "A significant part of the course must involve ..." (what makes a
	// course eligible).
	complVetoDescriptionRe = regexp.MustCompile(`(?i)\bas\s+well\s+as\s+suggested\s+courses\b|^a\s+significant\s+part\s+of\s+the\s+course\s+must\s+involve\b|^` + codePat + `\s+is\s+a\s+required\s+course\s+in\s+the\s+(?:[\w-]+\s+){1,4}(?:specialist|major|minor)\s+program$`)
)

// complNotBindingRes are sentences that read like rules but which a checker
// of this program can't or needn't evaluate: "Students can take a maximum of
// 3.0 credits from Group B per year" is timing, not a cap. See
// classifySentence for where they apply; "must" vetoes them unless it is
// procedure or a degree-level rule (isComplAdvisory).
var complNotBindingRes = []*regexp.Regexp{
	// A placement after consultation (advisory; it names no pool): "After
	// consultation with the Program Coordinator, exceptions may be made in
	// certain cases regarding the sequence of language courses based on
	// prior and documented exposure to the language in question".
	regexp.MustCompile(`(?i)^after\s+consultation\s+with\b[^.]*\bexceptions\s+may\s+be\s+made\b[^.]*\bsequence\s+of\s+language\s+courses\b`),
	// A program-wide consideration that names no requirement (a note; the
	// item that takes approved courses has its own route): "Courses that are
	// in the current academic Calendar that include significant Asian
	// Canadian or Canadian content may be considered, in consultation with
	// the Canadian Studies Program Director" (ASMIN2728).
	regexp.MustCompile(`(?i)^courses\s+that\s+are\s+in\s+the\s+current\s+academic\s+calendar\s+that\s+include\s+significant\b[^.]*\bmay\s+be\s+considered,\s+in\s+consultation\s+with\b`),
	// Permissions and advice that don't change what counts: "Third-year
	// students can take AST424H1 (Introduction to Research) provided they
	// have at least a B+ average in program courses" (enrolment), "Students
	// can take either course as their required independent project, or may
	// take JPM400Y1 as an additional elective" (the items say both),
	// "Students are advised to discuss potential course substitutions for
	// First Year with the Program Office", "A significant part of the course
	// must involve the application of digital technologies to humanities
	// scholarship, ..." (what an approver looks for in a course a note
	// lets students submit).
	regexp.MustCompile(`(?i)^(?:[\w-]+\s+){0,2}students\s+can\s+take\s+` + codePat + `\b.*\bprovided\s+they\s+have\b|^students\s+can\s+take\s+either\s+course\s+as\s+their\s+required\b.*\bas\s+an\s+additional\s+elective$|^students\s+are\s+advised\s+to\s+discuss\b|^a\s+significant\s+part\s+of\s+the\s+course\s+must\s+involve\b`),
	// Rules about other programs: "IRE/IRW courses cannot be used to fulfill
	// enrolment/program requirements in Economics", "... cannot be used to
	// satisfy coursework requirements for a Specialist, Major, or Minor in the
	// Department of French", "(STA220H1, STA255H1) will not count towards an
	// ECO major", the clause "except for students enrolled in the Education
	// and Society Minor, who may apply 1.0 credit of MAT329Y1 toward their
	// Minor program" (the cap it follows is this program's).
	regexp.MustCompile(`(?i)^except\s+for\s+students\s+enrolled\s+in\b[^,;]*,\s+who\s+may\s+apply\b.*\btowards?\s+their\s+(?:Specialist|Major|Minor)\s+program$|\bprogram\s+requirements\s+in\s+(?-i:[A-Z])|\brequirements\s+for\s+(?:a|another|any)\s+(?:Specialist|Major|Minor)\b|\btowards?\s+an?\s+(?-i:[A-Z]{3})\s+(?:major|minor|specialist)\b|\bnot\s+considered\s+equivalent\b`),
	// Timing the checker doesn't enforce: "a maximum of 3.0 credits from
	// Group B per year".
	regexp.MustCompile(`(?i)\bper\s+(?:academic\s+)?(?:year|session|term|semester)\b`),
	// Degree-level rules (see complDegreeRuleRe).
	complDegreeRuleRe,
	// The student's own checks: "It is the student's responsibility to
	// review the ... Timetable and Academic Calendar to confirm that they
	// meet the requirements and are eligible", "Students are responsible for
	// checking priority of courses".
	regexp.MustCompile(`(?i)\bstudents?['’]?s?\s+responsibility\b|\bare\s+responsible\s+for\b`),
	// The default sharing rule, restated: "Courses may appear in more than
	// one requirement but may only count once toward program completion",
	// "Each course can count toward only one requirement", "courses used to
	// satisfy one group requirement cannot be counted toward another group
	// requirement", "These are all individual requirements and cannot
	// overlap or be used twice", "(* if not taken for the requirement
	// immediately above)", "(if not taken in list 3)", "*If not taken for
	// Requirement 13 above".
	regexp.MustCompile(`(?i)^\*?\s*if\s+not\s+(?:already\s+)?taken\s+(?:in|for)\s+(?:list\s+\d+|requirement\s+\d+(?:\s+above)?|the\s+requirement\s+immediately\s+above)$`),
	regexp.MustCompile(`(?i)\bonly\s+count\s+once\b|\bonly\s+be\s+counted\s+towards?\s+one\b|\bcounted\s+twice\b|\bused\s+(?:more\s+than\s+once|twice)\b|\bcount\s+toward\s+only\s+one\s+requirement\b|\bonly\s+count\s+toward\s+one\s+requirement\b|\bcannot\s+be\s+counted\s+toward\s+another\b|\bcannot\s+overlap\b|\blisted\s+as\s+exclusions\s+to\s+one\s+another\b`),
	// No substitute for a listed course: "HMB200H1 cannot be used in place of
	// PSY290H1", "they cannot be used as substitutes for economics courses",
	// "STA302H1/ STA303H1 are not accepted in lieu of ECO375H1".
	regexp.MustCompile(`(?i)\bcannot\s+be\s+used\s+(?:in\s+place\s+of|as\s+substitutes?)\b|\bnot\s+accepted\s+in\s+lieu\s+of\b`),
	// Course enrolment notices: course exclusions ("Students with MAT157Y1
	// cannot take MAT246H1 (it is an exclusion)", "Students who took GER463Y1
	// cannot take GER260Y1/ ..."), caps ("IMM435H1 is capped at 40
	// students", "Enrolment in any of PCL474Y1 or JPM400Y1 is limited and
	// requires permission from the Department of Pharmacology and
	// Toxicology", "Enrolment in either PCL472Y1 or JPM400Y1 is limited").
	regexp.MustCompile(`(?i)\bcannot\s+take\s+` + codePat + `|\bcapped\s+at\b|^enrolment\s+in\s+(?:either\s+|any\s+of\s+)?` + codePat + `(?:\s*(?:,|or|and|/)\s*` + codePat + `)*\s+is\s+limited\b`),
	// Another program described: "The focus that is available for students
	// to pursue is "Focus in Quantitative Finance - ASFOC2290A" and requires
	// completion of 3.5 credits".
	regexp.MustCompile(`(?i)\bfoc(?:us|i)\s+that\s+(?:is|are)\s+available\b`),
}

// complAdviceRes is advice (complRecommendRe), not binding unless the sentence
// offers a substitution (complSubstitutionRes); see classifySentence.
var complAdviceRes = []*regexp.Regexp{complRecommendRe,
	// What to take for a focus within the program's choices: "(Students who
	// want to focus on evolutionary/biological approaches to medical
	// anthropology should take ANT203Y1)".
	regexp.MustCompile(`(?i)\bstudents\s+who\s+(?:want|wish)\s+to\s+focus\b[^.]*?\bshould\s+take\b`),
}

// complSubstitutionRes are the substitution routes among the approval routes
// (complApprovalRes). Unlike the others, they beat advice in the same
// sentence: "Students are advised to discuss potential course substitutions
// for First Year with the Program Office" offers a substitution, while in
// "Students are encouraged to take a graduate course with permission of the
// instructor" (constructed) the permission only comes with the advice.
var complSubstitutionRes = []*regexp.Regexp{
	// "Up to 2.0 credits in GRK or LAT may be substituted for the 5.5 credits
	// in CLA courses", "Students may request to substitute 0.5 credit from
	// requirement 2 with an alternate course".
	regexp.MustCompile(`(?i)\b(?:may|can)\s+(?:request\s+to\s+)?(?:be\s+)?substitut(?:e|ed)\b`),
	// "Course substitutions are possible with written permission of the
	// Program Director".
	regexp.MustCompile(`(?i)\bsubstitutions?\s+(?:are|is)\s+possible\b`),
	// "To substitute courses not currently on offer, please contact ...",
	// "Students can request substitutions from the IRP Director", "Students
	// are advised to discuss potential course substitutions for First Year
	// with the Program Office".
	regexp.MustCompile(`(?i)\bto\s+substitute\b|\brequest\s+(?:a\s+)?substitutions?\b|\bdiscuss\s+(?:potential\s+|possible\s+)?(?:course\s+)?substitutions?\b`),
}

// complApprovalRes are approval routes, which the assembler encodes as a
// permission alternative (design 3.3). They bind even with a cap or a contact
// in them; see classifySentence.
var complApprovalRes = slices.Concat(complSubstitutionRes, []*regexp.Regexp{
	// "Other introductory language classes may be approved by the Director",
	// "Courses that include significant Canadian content may be considered",
	// "A research project from a different unit may be accepted with prior
	// written approval", "Courses outside these lists ... may be included as
	// designated by the Program Director", "Some offerings of other POL
	// Special Topics courses may also be eligible as determined by the
	// department", "Courses from other departments may be accepted on a
	// case-by-case basis". "accepted", "chosen", "included" and "eligible" need an
	// approver after them: "No more than 1.0 credit may be accepted from
	// other universities" (constructed) is a cap.
	regexp.MustCompile(`(?i)\bmay\s+(?:also\s+)?be\s+(?:approved|considered)\b|\bmay\s+(?:also\s+)?be\s+(?:accepted|chosen|included|eligible)\s+(?:by|with|as\s+(?:designated|determined)|on\s+a\s+case-by-case\s+basis)\b`),
	// "Health related Anthropology Special Topics courses may be chosen when
	// offered at the third or fourth year levels": a topic-dependent member.
	regexp.MustCompile(`(?i)\bmay\s+be\s+chosen\s+when\s+offered\b`),
	// "Other languages are accepted with special permission of the
	// Undergraduate Director", "other relevant courses are eligible on the
	// approval of the program director", "Students need written permission of
	// the Digital Humanities program Coordinator".
	regexp.MustCompile(`(?i)\bwith\s+(?:the\s+)?(?:(?:prior|special|written)\s+)*(?:approval|permission)\b|\bon\s+the\s+approval\b|\bneed\s+written\s+permission\b`),
	// "Students who complete a graduate Topics course in Theory may request
	// to count it towards the completion of list 2", "Students may also
	// petition the Program Coordinator", "Other courses might also qualify",
	// "Please contact us to verify if your stats course qualifies".
	regexp.MustCompile(`(?i)\bmay\s+request\s+to\s+count\b|\bmay\s+(?:also\s+)?petition\b|\b(?:might|may)\s+also\s+qualify\b|\bcourses?\s+qualif(?:y|ies)\b`),
	// "Students may be permitted to take Math graduate courses".
	regexp.MustCompile(`(?i)\bmay\s+be\s+permitted\b`),
	// "Courses taken in Summer Abroad programs or at UTM or UTSC will need to
	// be pre-approved", "Courses taken in our unit-run Summer Abroad programs
	// will be considered".
	regexp.MustCompile(`(?i)\b(?:will\s+need\s+to|must)\s+be\s+pre-?approved\b|\bwill\s+be\s+considered\b`),
	// "... would like it to count exceptionally towards your program
	// requirements, please e-mail ...", "may email the Program Coordinator to
	// find out if the courses can be used towards the Group C program
	// requirements".
	regexp.MustCompile(`(?i)\bcount\s+exceptionally\b|\bfind\s+out\s+if\b`),
	// "if you wish to request consideration of these courses for
	// requirement 7".
	regexp.MustCompile(`(?i)\brequest\s+consideration\b`),
	// A pointer to a list of approved courses kept elsewhere, which widens
	// the pool: "Please, consult with Associate Chair ... or our website for
	// an updated list of courses pre-approved for program credit", "A
	// complete list of eligible courses is available at the departmental
	// website or by contacting ...". Not "This list of eligible courses is
	// not exhaustive": it points nowhere.
	regexp.MustCompile(`(?i)\blist\s+of\s+courses\s+pre-?approved\s+for\s+program\s+credit\b|\blist\s+of\s+eligible\s+courses\s+is\s+available\b`),
	// Approval by contact, which beats the contact advice (complAdvisoryRes):
	// "After consultation with the Program Coordinator, exceptions may be
	// made in certain cases regarding the sequence of language courses",
	// "Please consult with CEES and Slavic departments to get approval",
	// "Students wishing to apply their Arts & Science Internship Program
	// (ASIP) towards the requirement to complete PCL472Y1/ JPM400Y1 should
	// reach out to undergrad.pharmtox@utoronto.ca". Not "Please contact the
	// Undergraduate Administrator ... to request that these exceptions be
	// posted on Degree Explorer": they are granted already. Not "No
	// exceptions can be made" or "Please contact the department to get
	// approval to enrol in the course" (both constructed): the exception
	// needs a consultation, and the approval must end the sentence.
	//
	// Credit by consultation: "In consultation with the Undergraduate
	// Coordinator, students may take 0.5 credit in history or political
	// science courses corresponding to their chosen language area".
	regexp.MustCompile(`(?i)^in\s+consultation\s+with\b[^.]*?,\s*students\s+may\s+take\b`),
	// Approval by the program: "another course approved in joint
	// consultation with CERES and Slavic undergraduate coordinators",
	// "Permission to count courses towards the Minor ... that are not in the
	// Completion Requirements must be received by the Program Coordinator".
	// Not "1.0 credit may be taken from courses offered by other
	// departments, pending approval" (NMC) or "students should seek early
	// approval of program credit for such courses" (CSUS): placed as routes,
	// they would land on an item, not on the program-level share they widen;
	// nor "Up to 1.0 credit in identified or pre-approved cognate courses may
	// count (...); please inquire at the Department and get approved", a cap.
	regexp.MustCompile(`(?i)\bapproved\s+in\s+(?:joint\s+)?consultation\b|^permission\s+to\s+count\b`),
	regexp.MustCompile(`(?i)\bconsultation\b[^.]*\bexceptions?\s+(?:may|can)\s+be\s+(?:made|granted)\b|\b(?:consult(?:ation)?|contact|speak|reach\s+out)\b.*?\bto\s+(?:get|obtain|seek)\s+(?:prior\s+)?approval\s*$|\bwish(?:es|ing)?\s+to\s+apply\b[^.]*\btowards?\s+(?:the|this|their|your)\s+(?:[\w-]+\s+){0,2}(?:requirements?|programs?)\b`),
})

// complCountQueryRe matches a question about counting another course, an
// approval route even in a sentence of advice: "If you have completed any of
// these suggested related courses, please contact cs.undergrad@utoronto.ca to
// determine whether it may be appropriate to count a related course towards
// Focus requirements", "If a student wishes a different language ... to count
// toward completing these programs they should consult with the Associate
// Chair, Undergraduate".
var complCountQueryRe = regexp.MustCompile(`(?i)\b(?:whether|if)\b[^.]*\bcount\b[^.]*?\btowards?\s+(?:the\s+|your\s+|this\s+)?(?:[\w-]+\s+){0,2}(?:programs?|focus|requirements?)\b`)

// complWidenRe matches a pool widened with a cap: "Of these 4.0 credits, up to
// 1.0 credit may be taken from cognate departmental or college offerings",
// "Up to 1.0 credit may be from other departments", "Up to 1.0 credit of this
// requirement can also be completed from HST211H1/ ...". The assembler adds
// both the wider pool and the cap; the role is requirement.
var complWidenRe = regexp.MustCompile(`(?i)\bup\s+to\b.*?\b(?:may|can)\s+(?:also\s+be\s+(?:taken|chosen|drawn|completed)\s+from|be\s+(?:(?:taken|chosen|drawn)\s+)?from\s+(?:the\s+following\s+)?(?:approved\s+courses\s+offered\s+by\s+)?(?:cognate|other\s+departments))`)

// complBindingProseRes are binding sentences that may have no code, amount or
// filter: pools named in prose, inherited requirements and exceptions to the
// total. See classifySentence.
var complBindingProseRes = []*regexp.Regexp{
	// Pools in prose: "Other courses are also eligible to meet this
	// requirement", "All courses in the Centre for Jewish Studies
	// Undergraduate Course Handbook count towards this requirement", "This
	// requirement can be satisfied by any of the Earth Sciences Courses
	// listed in categories A to F below", "Students who complete the Arts &
	// Science Internship Program (ASIP) stream will also meet this
	// requirement".
	regexp.MustCompile(`(?i)\b(?:are|is)\s+(?:also\s+)?eligible\b|\bcounts?\s+towards?\s+this\s+requirement\b|\bsatisfied\s+by\b|\b(?:can|may)\s+(?:also\s+)?be\s+used\s+to\s+meet\b|\bmeet\s+this\s+requirement\b`),
	// Inheritance: "This Focus is part of the Cell and Molecular Biology
	// Major and begins with the requirements of First Year and of lines 1-3
	// of Higher Years of the Cell and Molecular Biology Major Program".
	regexp.MustCompile(`(?i)\bbegins\s+with\s+the\s+requirements\s+of\b`),
	// An exception to the total: "There is no need to replace the missing
	// half-credit for program completion".
	regexp.MustCompile(`(?i)\bno\s+need\s+to\s+replace\b`),
}

// complAsideRes are advice, explanation and restatement that read like a
// requirement (an amount, a code, "must"), checked before every rule: a
// sequence offered as advice, a course's own prerequisite or timing, a
// list's description, a total restated (T10-2).
var complAsideRes = []*regexp.Regexp{
	// "EEB498Y1 concurrent with research issues course EEB488H1": the
	// course's own corequisite, restated (stripCategoryLabels).
	regexp.MustCompile(`^` + codePat + `\s+concurrent\s+with\b`),
	// "Students interested in Mineral Exploration: First Year: BIO120H1,
	// ...", "Students with an interest in GIS and Geology: ...", "Other
	// courses which might of interest: ESS391H1, ...".
	regexp.MustCompile(`(?i)^students\s+(?:interested\s+in|with\s+an\s+interest\s+in)\b[^:]*:|\bmight\s+(?:be\s+)?of\s+interest\b`),
	// "Neuroscience Centric Courses: HMB200H1, ...": the courses a student
	// focusing on a theme might pick.
	regexp.MustCompile(`^[\w&,\s]{1,60}\bCentric\s+Courses\s*:`),
	// Timing: "normally taken in first year", "to be taken as early as
	// possible", "first offering in 2025-26", "should be taken together with
	// GER200Y1, must be taken prior to completing GER300Y1", "First-year
	// students should take EAS103H1 and EAS105H1, a first-year language
	// course ..., and may take up to 1.0 Society-Culture credit at the
	// 200-level".
	regexp.MustCompile(`(?i)^(?:normally\s+taken\s+in\s+(?:the\s+)?first\s+year|to\s+be\s+taken\s+as\s+early\s+as\s+possible|first\s+offering\s+in\s+\d{4}(?:-\d{2,4})?)$|^should\s+be\s+taken\s+together\s+with\b|^(?:first|second|third|fourth)-year\s+students\s+should\s+take\b`),
	// A course's own prerequisite or audience: "PSL300H1 and PSL301H1
	// require MAT 100-level/ PHY 100-level courses", "BIO220H1 is required
	// for all EEB 300+ level courses", "Students taking PCL477H1 must take
	// BCH210H1 prior", "STA130H1 is restricted to first-year students",
	// "ENV238H1 is for students who have previously taken PHY131H1".
	regexp.MustCompile(`^(?:` + codePat + `(?:\s*(?:,|and|/)\s*` + codePat + `)*)\s+requires?\s|\bis\s+required\s+for\s+all\s+[A-Z]{3}\s+[1-4]00\+?[\s-]level\s+courses\b|^Students\s+taking\s+` + codePat + `\s+must\s+take\s+` + codePat + `\s+prior$|\bis\s+restricted\s+to\s+first-year\s+students\b|\bis\s+for\s+students\s+who\s+have\s+(?:previously\s+)?taken\b`),
	// A course exclusion, which enrolment enforces: "If you completed
	// CSC148H1 before being admitted to the Minor, please note that this
	// course is an exclusion to EEB125H1, ENG286H1, and GGR274H1".
	regexp.MustCompile(`(?i)\bplease\s+note\s+that\s+(?:these\s+courses\s+are|this\s+course\s+is)\s+(?:an?\s+)?exclusions?\s+to\b`),
	// Descriptions: "MST300H1 is offered through the Centre for Medieval
	// Studies", "LIN courses from other campuses are considered courses from
	// other Departments", "These MIE courses address Human Factors",
	// "GGR223H1 is NOT a substitute for ENV222H1", "*course(s) no longer
	// offered", "Requirement 1 in Third or Fourth Year represents a 0.5
	// credit with a significant emphasis on ...", "The updated requirements are for students entering the program as of
	// 2026", "Courses with codes that do not start with FSL or FRE (e.g., FCS185H1 or JFL388H1) are typically
	// conducted partially or entirely in English and are therefore not
	// included" (the designator filter restated).
	regexp.MustCompile(`^` + codePat + `\s+is\s+offered\s+(?:through|by)\b|\bcourses\s+from\s+other\s+campuses\s+are\s+considered\s+courses\s+from\b|^These\s+[A-Z]{3}\s+courses\s+address\b|^` + codePat + `\s+is\s+NOT\s+a\s+substitute\s+for\b|^\*?course(?:\(s\))?\s+no\s+longer\s+offered$|^Requirement\s+\d+\s+[^.]*\brepresents\s+a\b|^The\s+updated\s+requirements\s+are\s+for\b|^Courses\s+with\s+codes\s+that\s+do\s+not\s+start\s+with\b[^.]*\bconducted\b[^.]*\bin\s+English\b`),
	// Descriptions that name required courses (see complVetoDescriptionRe).
	complVetoDescriptionRe,
	// Degree-level remarks: "Additional BCH research project courses can be
	// taken as part of the 20.0 credits needed for your degree", "Note that
	// the requirements for a co-sponsoring Department’s major can normally be
	// fulfilled with 0.5 to 3.5 additional credits", "You may choose more
	// than 2.5 credits from the Additional Foundation Geosciences list". Not
	// "..., but cannot be counted towards the program completion" (ASMAJ1762).
	regexp.MustCompile(`\bcredits\s+needed\s+for\s+your\s+degree$|^Note\s+that\b[^.]*\bcan\s+normally\s+be\s+fulfilled\b|^You\s+may\s+choose\s+more\s+than\b`),
	// A header's "(7.0 credits, including PHL100Y1/ PHL101Y1 if taken)": the
	// courses count like any other toward the total, whose pool (PHL)
	// already holds them (ASMAJ0231, ASMIN0231, ASSPE0231; the only uses).
	regexp.MustCompile(`^including\s+` + codePat + `(?:\s*/\s*` + codePat + `)*\s+if\s+taken$`),
	// A total restated in brackets after the items: "(total of 1.5
	// credit)", "(3.0 credits in total)".
	regexp.MustCompile(`(?i)^(?:a\s+)?total\s+of\s+\d+(?:\.\d+)?\s+credits?$|^\d+(?:\.\d+)?\s+credits\s+in\s+total$`),
	// Asides with "must" that the advice veto lets through (complAsideMustRe).
	complAsideMustRe,
	// The freedom an elective item leaves: "The 1.5 credits can be chosen to
	// reflect the particular academic interests of each student" (with the
	// restated "*If not taken for Requirement 13 above" after it).
	regexp.MustCompile(`(?i)^the\s+\d+(?:\.\d+)?\s+credits?\s+can\s+be\s+chosen\s+to\s+reflect\b`),
	// A descriptor closing a list of alternatives, kept by stripAsides: "a
	// capstone integrative course".
	regexp.MustCompile(`^an?\s+[a-z][a-z\s-]*\bcourse$`),
}

// complAsideMustRe are asides whose "must" is a course's own timing or
// prerequisite, which enrolment enforces, or a lead into the items, so the
// advice veto doesn't apply: "should be taken together with GER200Y1, must
// be taken prior to completing GER300Y1", "Students taking PCL477H1 must
// take BCH210H1 prior", "Courses must fulfill the following requirements:".
var complAsideMustRe = regexp.MustCompile(`(?i)^should\s+be\s+taken\s+together\s+with\s+` + codePat + `,\s+must\s+be\s+taken\s+prior\s+to\s+completing\s+` + codePat + `$|^Students\s+taking\s+` + codePat + `\s+must\s+take\s+` + codePat + `\s+prior$|^courses\s+must\s+(?:fulfil|fulfill|satisfy|meet)\s+the\s+following\s+requirements:?$`)

// complRecommendedHeadRe: a heading that offers what follows as advice,
// "First year (recommended): PHL100Y1/ PHL101Y1" (read with its brackets,
// which isComplAdvisory takes out).
var complRecommendedHeadRe = regexp.MustCompile(`(?i)^[^:()]{0,40}\((?:strongly\s+|highly\s+)?recommended\)\s*:`)

// complAdvisoryRes are recurring completion sentences that are advice,
// explanation or procedure, never a requirement. Each becomes a note. See
// classifySentence; "must" (or "required courses") vetoes them unless it is
// procedure or a degree-level rule (isComplAdvisory).
var complAdvisoryRes = []*regexp.Regexp{
	// "Students in this program have the option to complete the Arts &
	// Science Internship Program (ASIP) stream."
	regexp.MustCompile(`(?i)Arts\s*&\s*Science\s+Internship\s+Program|\bASIP\b`),
	// Contacts and websites: "Consult the Undergraduate Office for advice
	// ...", "Please contact the Undergraduate Administrator", "Check with the
	// department for current offerings", "please visit the graduate
	// timetable webpage", "Variations on this can be discussed with the
	// Program Coordinator". Not "consultation": "another course approved in
	// joint consultation with CEES" is an approval, which binds.
	regexp.MustCompile(`(?i)\bconsult(?:ing)?\b|\bcontact(?:ing)?\b|\be-?mail\b|\bweb\s*(?:sites?|pages?)\b|https?://|www\.|@utoronto|reach\s+out|\bcheck\s+(?:with|the)\b|\bspeak\s+(?:with|to)\b|\bdiscuss(?:ed)?\s+with\b|\binquire\b|\bvisit\b|\bappointment\b|\badvice\b|\badvising\b|\btimetable\b`),
	// "They are not completion requirements for the Environment and Energy
	// Minor", "not an official program requirement", "While not a formal
	// part of the Economics Minor program, ...", "it is not formally a part
	// of this Specialist Program's requirements".
	regexp.MustCompile(`(?i)\bnot\s+(?:an?\s+)?(?:official\s+)?(?:program\s+|completion\s+)?requirements?\s+(?:of|for)\b|\bnot\s+an?\s+official\b|\bare\s+not\s+(?:completion|program)\s+requirements\b|\bnot\s+(?:a\s+formal|formally\s+a)\s+part\s+of\b`),
	// Entry to programs and what later courses need: "Eligibility for all
	// Economics programs is based, in part, on attaining a minimum grade in
	// ECO101H1 and ECO102H1", "Many 300-level and higher courses in
	// economics require ECO220Y1".
	regexp.MustCompile(`(?i)\beligibility\s+for\s+(?:all\s+)?(?:[\w&]+\s+){0,4}programs?\s+is\s+based\b|^(?:many|most|some)\s+(?:[\w+-]+\s+){0,6}courses\s+(?:[\w]+\s+){0,3}require\b`),
	// Courses described, and what past students found: "CSC240H1 is an
	// accelerated and enriched version of CSC165H1 plus CSC236H1", "Former
	// BCB students benefitted from taking CSB352H1", "They also found it to
	// be helpful to take BCH441H1/ MGY441H1 prior to BCB410H1".
	regexp.MustCompile(`(?i)\bis\s+an?\s+(?:accelerated|enriched|advanced)\s+(?:and\s+\w+\s+)?version\s+of\b|\bbenefit(?:ted|ed|s)?\s+from\s+taking\b|\bfound\s+it\s+(?:to\s+be\s+)?helpful\b`),
	// Enrolment in courses, not what counts: "BCB students are allowed to
	// enrol in up to 2.5 CSC credits at the 300/400-level".
	regexp.MustCompile(`(?i)^(?:[\w-]+\s+){0,3}students\s+(?:are|will\s+be)\s+(?:allowed|permitted)\s+to\s+enrol+\s+in\s+up\s+to\b`),
	// When to take listed courses (design 3.6: timing is a note):
	// "CSC108H1/ CSC148H1/ CSC110Y1 may be taken in First Year", "Students
	// seeking an enriched introduction to the theory of computing may choose
	// to enrol in CSC240H1 in their first year".
	regexp.MustCompile(`(?i)^` + codePat + `(?:\s*[/,]\s*` + codePat + `)*\s+(?:may|can)\s+be\s+taken\s+in\s+(?:the\s+)?(?:first|second|third|fourth)\s+year$|\bmay\s+choose\s+to\s+(?:enrol|enroll|register)\s+in\s+` + codePat + `\s+in\s+(?:their|the)\s+(?:first|second|third|fourth)\s+year$`),
	// Pointers: "Refer to the Department website for a full list", "See the
	// bottom of the completion requirements section", "for more information".
	regexp.MustCompile(`(?i)\brefer\s+to\b|^see\b|\bplease\s+see\b|\bfor\s+(?:more|further|additional|up-to-date)\s+(?:information|details)\b|\bfor\s+a\s+full\s+(?:list|listing)\b`),
	// Prerequisites: "Some of the courses listed below may have
	// prerequisites", "BIO220H1 is required for all upper-year 300+ EEB series
	// courses", "Not all courses may be available at all times without
	// scheduling conflicts", "Please note that not all courses are offered
	// on a regular basis".
	regexp.MustCompile(`(?i)\b(?:pre|co)-?requisites?\b|\brequired\s+(?:preparation\s+)?for\s+(?:all\s+)?(?:upper|\d00|later)|\bnot\s+all\s+(?:of\s+the\s+)?(?:[\w-]+\s+){0,3}?courses\b`),
	// Offerings: "some may be offered in alternate years", "Note that not all
	// of the 400-level PHC courses are offered every academic year", "Some
	// required courses ... may be offered only on the St. George or on the
	// University of Toronto Mississauga campus", "These courses are offered
	// by other academic units", "Check with the department for current
	// offerings", "they are not guaranteed to be approved".
	regexp.MustCompile(`(?i)\balternate\s+years\b|\boffered\s+(?:every|each)\s+(?:academic\s+)?year\b|\bmay\s+be\s+offered\s+only\b|\bare\s+offered\s+by\s+other\b|\bon\s+a\s+regular\s+basis\b|\bcurrent\s+offerings\b|\bnot\s+guaranteed\b`),
	// Enrolment controls: "check the pre-requisites and enrolment controls",
	// "Note: Some Group 2 courses may have enrolment restrictions", "Non-IRE/
	// IRW courses ... may be restricted to students enrolled in their
	// programs", "may be available only to students who have completed
	// specified prerequisites", "Priority is reserved for Drama Majors",
	// "students with prior German knowledge are required to take a placement
	// test".
	regexp.MustCompile(`(?i)\benrolment\s+controls\b|\benrolment\s+restrictions?\b|\bmay\s+be\s+restricted\b|\bavailable\s+only\s+to\s+students\b|\bpriority\b|\bplacement\s+(?:tests?|assessments?)\b`),
	// Notices about the program: "These waivers will only be considered for
	// ...", "are not required for entrance into the specialist program", "a
	// number of spots are available", "not eligible for work permits for
	// internships", "Any PHY/MAT courses should be completed during the first
	// year".
	regexp.MustCompile(`(?i)\bwaivers?\b|\bnot\s+required\s+for\s+(?:entrance|entry|admission)\b|\bspots\s+are\s+available\b|\bwork\s+permits?\b|\bshould\s+be\s+(?:completed|taken)\s+(?:during|in|by|before)\b`),
	// Lists described: "This list of eligible courses is not exhaustive",
	// "This is not a complete list", "The ordering is alphabetical and not
	// indicative of importance".
	regexp.MustCompile(`(?i)\bnot\s+(?:an?\s+)?(?:complete|exhaustive)\s+list\b|\bnot\s+exhaustive\b|\bnot\s+indicative\b`),
	// How transfer credit is matched to a listed course, which it is anyway:
	// "Transfer credits will be accepted in lieu of the chemistry
	// requirements only if they carry a direct exclusion or equivalency to a
	// pre-approved chemistry course", "Only transfer credits that carry
	// exclusions to CHM136H1 and/or CHM135H1 will be accepted".
	regexp.MustCompile(`(?i)\btransfer\s+credits?\b[^.]*\b(?:exclusions?|equivalency)\b`),
	// "There are no specific requirements for first year", "No specific
	// first-year courses required".
	regexp.MustCompile(`(?i)\bno\s+specific\s+(?:first-year\s+)?(?:courses?\s+)?(?:requirements?|required)\b|\bno\s+specific\s+courses\s+are\s+required\b`),
	// The program's description of itself: "The Astronomy & Physics
	// Specialist program combines rigorous training ...", "This program
	// offers considerable freedom to choose courses", "The Centre is
	// committed to working with any student", "A program designed for students
	// looking to add breadth", "HIS262H1 ... is designed for non-History
	// students".
	regexp.MustCompile(`(?i)^(?:the|this)\s+(?:[\w&,]+\s+){0,6}?(?:program|focus|minor|major|specialist|certificate)\s+(?:offers|combines|provides|is\s+designed|introduces|explores|allows|gives|prepares|emphasi[sz]es|focuses)\b|\bis\s+committed\s+to\b|\bdesigned\s+for\b`),
	// Course-code changes: "Effective Fall 2026, all courses associated with
	// University College's Public Health program will have the "PHS"
	// designator. Existing "PHS" coded courses remain unchanged."
	regexp.MustCompile(`(?i)\beffective\s+(?:fall|winter|summer|september|january|may)\b|\bremain\s+unchanged\b`),
	// Assessments, syllabi, fees and Faculty rules: "Some Studio (Group B)
	// courses will require a separate assessment either through an audition
	// ...", "It will help to have a syllabus for the course being
	// considered", "Students are reminded that ... they may repeat a specific
	// passed course only once as per Faculty regulations", "are charged
	// retroactive program fees".
	regexp.MustCompile(`(?i)\baudition\b|\bportfolio\b|\bsyllabus\b|\bare\s+reminded\b|\bFaculty\s+regulations\b|\bretroactive\b|\bprogram\s+fees\b`),
	// Procedure (see complProcedureRe): "Requests must be reviewed and
	// approved by the Computer Science Undergraduate Office".
	complProcedureRe,
}

// classifySentence returns the role of one sentence of completion text. List
// markers, emphasis and "Note:" labels are ignored. Text with no letter or
// digit, or only a list marker ("1.", "a)", "(i)"), is advisory before any
// step, and so is a restatement of the rows or advice in one wording
// (complRestatedRes), and an aside (complAsideRes). Otherwise the first
// match wins:
//
//  1. exclusion: "cannot combine", "cannot be paired with", ... (complExclusionRes).
//  2. overlap: "may contribute to both" (complOverlapRe).
//  3. crNcr: CR/NCR with a rule verb; a pointer to details falls through.
//  4. transfer: transfer credit with a cap or a ban; it beats limit.
//  5. listDef: a list label, "Group A:", even with a cap in it.
//  6. requirement: an amount lead with no verb in its first clause.
//  7. requirement: a question about counting another course (complCountQueryRe).
//  8. advisory: the not-binding table (complNotBindingRes), and advice
//     (complAdviceRes) unless the sentence offers a substitution
//     (complSubstitutionRes).
//  9. requirement: an approval route or a widened pool (complApprovalRes, complWidenRe).
//  10. constraint or limit by cue word: the earliest cue wins; a code lead
//     drops constraint cues; then an "Of these" scope is a constraint
//     (complRuleRole).
//  11. listDef: a label followed by codes.
//  12. requirement: binding prose (complBindingProseRes).
//  13. advisory: the advice table (complAdvisoryRes).
//  14. requirement for a code, an amount, a filter, a binding verb or a
//     heading; advisory otherwise.
//
// The advisory steps (8, 13) are vetoed by "must", "are required courses" or
// "required for the program", unless the "must" is procedure or a
// degree-level rule: an advisory sentence with "must" binds. Advice is
// matched outside brackets and in the first ";" clause only, so an item with
// "(recommended)" stays an item.
func classifySentence(s string) sentenceRole {
	t := complClean(s)
	if !complWordRe.MatchString(t) || complBareMarkerRe.MatchString(t) {
		return roleAdvisory
	}
	switch {
	case complMatchAny(complRestatedRes, t):
		// A restatement of the rows, or advice (completion_restated.go).
		return roleAdvisory
	case complRecommendedHeadRe.MatchString(t) || isComplAdvisory(t, complAsideRes):
		return roleAdvisory
	case complMatchAny(complExclusionRes, t):
		return roleExclusion
	case complOverlapRe.MatchString(t):
		return roleOverlap
	case complCrNcrAllowRe.MatchString(t):
		return roleAdvisory
	case complCrNcrRe.MatchString(t) && complCrNcrRuleRe.MatchString(t):
		return roleCrNcr
	case complTransferRe.MatchString(t) && complTransferRuleRe.MatchString(t):
		return roleTransfer
	case complListLabelRe.MatchString(t) && !complFromLabelRe.MatchString(t):
		return roleListDef
	case isComplAllocLead(t):
		return roleRequirement
	case complCountQueryRe.MatchString(t):
		return roleRequirement
	case isComplAdvisory(t, complNotBindingRes):
		return roleAdvisory
	case isComplAdvisory(t, complAdviceRes) && !complMatchAny(complSubstitutionRes, t):
		return roleAdvisory
	case complMatchAny(complApprovalRes, t) || complWidenRe.MatchString(t):
		return roleRequirement
	}
	if r, ok := complRuleRole(t); ok {
		return r
	}
	switch {
	case isComplListHead(t):
		return roleListDef
	case complMatchAny(complBindingProseRes, t):
		return roleRequirement
	case isComplAdvisory(t, complAdvisoryRes):
		return roleAdvisory
	case complCodeRe.MatchString(t) || complAmountRe.MatchString(t) || complFilterRe.MatchString(t) ||
		complBindingRe.MatchString(t) || isComplHeading(t):
		return roleRequirement
	}
	return roleAdvisory
}

// complClean strips list markers, emphasis, note labels, a trailing full stop
// and brackets around the whole sentence.
func complClean(s string) string {
	t := stripEmphasis(complNBRe.ReplaceAllString(s, "NB: "))
	for {
		u := strings.TrimSpace(complLeadRe.ReplaceAllString(t, ""))
		u = strings.TrimSpace(strings.TrimSuffix(u, "."))
		if complWrapped(u) {
			u = strings.TrimSpace(u[1 : len(u)-1])
		}
		if u == t {
			return t
		}
		t = u
	}
}

// complWrapped reports a sentence in one pair of brackets: "( MAT223H1/
// MAT224H1/ MAT240H1 recommended in first year)", "[ PHY131H1, PHY132H1
// recommended]".
func complWrapped(u string) bool {
	n := len(u)
	if n < 2 || !(u[0] == '(' && u[n-1] == ')' || u[0] == '[' && u[n-1] == ']') {
		return false
	}
	open, closing := u[0], u[n-1]
	depth := 0
	for i := 0; i < n; i++ {
		switch u[i] {
		case open:
			depth++
		case closing:
			depth--
			if depth == 0 && i < n-1 {
				return false // "(a) ... (b)"
			}
		}
	}
	return depth == 0
}

func complMatchAny(res []*regexp.Regexp, s string) bool {
	for _, re := range res {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

// complStripParens removes bracketed text, innermost first.
func complStripParens(t string) string {
	for {
		u := complParenRe.ReplaceAllString(t, "")
		if u == t {
			return t
		}
		t = u
	}
}

// isComplHeading reports a heading or a lead-in ending with a colon
// ("Courses accepted towards this program's ethics requirement:").
func isComplHeading(t string) bool {
	return strings.HasSuffix(t, ":") || complHeadingRe.MatchString(t) && !complHeadingVerbRe.MatchString(t)
}

// isComplListHead reports a list label followed by codes, with no verb after
// the colon.
func isComplListHead(t string) bool {
	m := complListHeadRe.FindStringSubmatchIndex(t)
	if m == nil || complNotListHeadRe.MatchString(t[m[2]:m[3]]) {
		return false
	}
	return !complLeadVerbRe.MatchString(complParenRe.ReplaceAllString(t[m[3]:], ""))
}

// isComplAllocLead reports a sentence that leads with an amount and has no
// verb in its first clause.
func isComplAllocLead(t string) bool {
	loc := complAllocLeadRe.FindStringIndex(t)
	if loc == nil {
		return false
	}
	if complRecommendRe.MatchString(t[:loc[1]]) || complLeadCueRe.MatchString(t[:loc[1]]) {
		return false // "Recommended: 0.5 credit from any HPS 100-level courses"
	}
	rest := t[loc[1]:]
	if complTakenFromRe.MatchString(rest) {
		return true
	}
	if i := complClauseEndRe.FindStringIndex(rest); i != nil {
		rest = rest[:i[0]]
	}
	if i := complRelativeRe.FindStringIndex(rest); i != nil && strings.ContainsAny(t[:loc[1]], "0123456789") {
		rest = rest[:i[0]]
	}
	return !complLeadVerbRe.MatchString(rest)
}

// complRuleRole returns constraint or limit for the earliest cue of either
// kind: "At least 1.0 credit must be at the 300-level, and no more than 0.5
// credit from Group C may count" is a constraint, the reverse order a limit
// (both constructed).
func complRuleRole(t string) (sentenceRole, bool) {
	best, role := -1, roleRequirement
	codeLead := complCodeLeadRe.MatchString(t)
	if !codeLead {
		for _, re := range complConstraintRes {
			if loc := re.FindStringIndex(t); loc != nil && (best < 0 || loc[0] < best) {
				best, role = loc[0], roleConstraint
			}
		}
	}
	// A code-led item's caps in brackets are its own: "POL222H1 (or one of
	// its exclusions: ...; only 0.5 credit of ECO220Y1 can count toward this
	// Focus)".
	lt := t
	if codeLead {
		lt = complStripParens(t)
	}
	for _, re := range complLimitRes {
		loc := re.FindStringIndex(lt)
		if loc == nil || best >= 0 && loc[0] >= best {
			continue
		}
		// A code-led sentence that allocates an amount before the cap is an
		// item: "In addition, CIN230H1 ... or 0.5 credit from Breadth
		// Category 5; and at least 3.0 credits from Groups B through G, ...,
		// and only a maximum of 1.5 credits can be from Group G".
		if codeLead && complAmountRe.MatchString(lt[:loc[0]]) {
			continue
		}
		best, role = loc[0], roleLimit
	}
	if best < 0 && !codeLead && complScopeRe.MatchString(t) && !isComplAdvisory(t, complAdvisoryRes) {
		return roleConstraint, true // "Of these:"; but not "Of the required gateway courses, ... have pre-requisites"
	}
	return role, best >= 0
}

// isComplAdvisory matches res outside brackets, in the first ";" clause, with
// attributive "recommended"/"suggested" removed. The advice veto applies
// unless the "must" is procedure or a degree-level rule.
func isComplAdvisory(t string, res []*regexp.Regexp) bool {
	if complAdviceVetoRe.MatchString(t) && !complProcedureRe.MatchString(t) && !complDegreeRuleRe.MatchString(t) &&
		!complVetoDescriptionRe.MatchString(t) && !complAsideMustRe.MatchString(t) {
		return false
	}
	if complRecommendedTagRe.MatchString(t) && !complCodeRe.MatchString(t) {
		return true
	}
	bare := complStripParens(complTimingNoteRe.ReplaceAllString(t, ""))
	bare = complAttributiveAdviceRe.ReplaceAllString(bare, "")
	// Advice in a later clause doesn't make the sentence advice: "If you
	// take CSC240H1 without CSC165H1, there is no need to replace the missing
	// half-credit for program completion; but please see Note (a)".
	bare = splitTop(bare, ';')[0]
	return complMatchAny(res, bare)
}

// splitComplSentences splits a line of completion text into sentences, as
// splitSentences does: at ". " before an uppercase letter, outside closed
// brackets, skipping abbreviations ("e.g.", "i.e.", "St."), so "CSC108H1."
// ends a sentence only when one follows. Emphasis is removed and empty
// sentences are dropped. Beyond that:
//   - a list marker ("5. ") stays with the first sentence;
//   - "N.B." becomes the label "NB:";
//   - a line wholly in brackets is split inside them;
//   - a sentence may start with an amount, even after a full stop with no
//     space ("courses.1.0 credit may be taken");
//   - an italic note glued to an item starts a sentence ("CHM151Y1
//     _Transfer credits ..._");
//   - inline lettered notes "(a) ...; (b) ..." become one sentence each.
func splitComplSentences(s string) []string {
	s = complNBRe.ReplaceAllString(strings.TrimSpace(s), "NB: ")
	s = stripEmphasis(complItalicNoteRe.ReplaceAllString(s, "$1. $2"))
	marker := complMarkerRe.FindString(s)
	s = s[len(marker):]
	if complWrapped(s) && strings.Contains(s, ". ") {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	out := complSplitAtAmounts(s)
	if marker != "" && len(out) > 0 {
		out[0] = marker + out[0]
	}
	var kept []string
	for _, sent := range complSplitLetterNotes(out) {
		if sent != "" {
			kept = append(kept, sent)
		}
	}
	return kept
}

// complSplitAtAmounts splits s with splitSentences, and also before a
// sentence that starts with an amount, a level or a bracketed code
// (complAmountStartRe), outside brackets.
func complSplitAtAmounts(s string) []string {
	parens := topParens(s)
	inParens := func(i int) bool {
		for _, g := range parens {
			if g[0] < i && i < g[1] {
				return true
			}
		}
		return false
	}
	var out []string
	start := 0
	for _, m := range complAmountStartRe.FindAllStringSubmatchIndex(s, -1) {
		dot := m[3]
		if abbreviations[strings.ToLower(s[m[2]:m[3]])] || inParens(dot) {
			continue
		}
		out = append(out, splitSentences(s[start:dot])...)
		start = dot + 1
	}
	return append(out, splitSentences(s[start:])...)
}

// complSplitLetterNotes splits each sentence that holds inline lettered notes
// ("(a) ...; (b) ...") into one sentence per letter.
func complSplitLetterNotes(sentences []string) []string {
	var out []string
	for _, sent := range sentences {
		if !strings.Contains(sent, "(a) ") {
			out = append(out, sent)
			continue
		}
		from := 0
		for _, loc := range complLetterNoteRe.FindAllStringIndex(sent, -1) {
			out = append(out, strings.TrimSpace(sent[from:loc[0]]))
			from = loc[0] + 1
		}
		out = append(out, strings.TrimSpace(sent[from:]))
	}
	return out
}

// ---- Text kinds ----

// complContentRes are what a course's content, topic, type or language of
// instruction decides, which the catalogue doesn't record (kind content,
// design D1): each match is the clause that says so; with it taken out (or
// replaced by the regexp's template), the rest of the sentence is the part
// a checker can read (contentPool).
var complContentRes = []struct {
	re   *regexp.Regexp
	repl string
}{
	// Language of instruction: "2.0 PRT credits at the 200+ level taught in
	// Portuguese", "No more than 2.0 credits taught in English", "a maximum
	// of 1.0 total program credits may be drawn from courses taught in
	// English", "No more than a 0.5 credit may be taken in English".
	{regexp.MustCompile(`\s*\b(?:(?:from\s+)?courses\s+)?(?:taught|taken)\s+in\s+(?:English|French|Spanish|Portuguese|German|Italian)\b`), ""},
	// "1.0 credit in courses at the 200+ level where the primary language of
	// study is not English", "2.0 credits in courses where ..., including at
	// least 1.0 credit at the 300+ level" (a floor on those courses).
	{regexp.MustCompile(`\s*\bwhere\s+the\s+primary\s+language\s+of\s+study\s+is\s+not\s+English\b(?:,\s*including\s+at\s+least\s+\d+(?:\.\d+)?\s+credits?\s+at\s+the\s+\d00\+?[\s-]level)?`), ""},
	// "1.0 credit at the 100-level or above in a language other than
	// English", "at least 4.0 credits must involve reading texts in a
	// language other than English".
	{regexp.MustCompile(`,?\s*\b(?:(?:that\s+)?(?:must\s+)?involve\s+reading\s+texts\s+)?in\s+a\s+language\s+other\s+than\s+English\b`), ""},
	// Course types: "1.0 credit of SPA and/or PRT language courses",
	// "Language courses cannot be used to fulfil the requirements", "2.0
	// credits in Hungarian language study", "Two consecutive language
	// courses", "credits in languages or literatures drawn from other
	// departments", "1.5 first year Social Science credits", "300+ level
	// Literature and Language courses", "Creative Writing courses".
	{regexp.MustCompile(`\b([A-Z]{3})\s+language\s+(courses)\b`), "$1 $2"},
	{regexp.MustCompile(`^Language\s+(courses\s+cannot\s+be\s+used)\b`), "$1"},
	{regexp.MustCompile(`\b[A-Z][a-z]+\s+language\s+study\b|\bconsecutive\s+language\s+courses\b|\bin\s+languages\s+or\s+literatures\b`), ""},
	// "2.5 first year Science credits": first year is the 100 level, which a
	// checker reads; which courses are Science is content.
	{regexp.MustCompile(`\bfirst\s+year\s+(?:Social\s+)?Science\s+credits\b`), "100-level credits"},
	{regexp.MustCompile(`,\s*to\s+include\b.*\bCreative\s+Writing\s+courses\b.*$`), ""},
	{regexp.MustCompile(`\b(?:Literature\s+and\s+Language|Creative\s+Writing)\s+courses\b`), ""},
	// Content, theme and experience: "0.5 credit in global experience",
	// "as part of an international experience", "Some offerings of HIS357H1,
	// ... may also count towards this focus", "Industry-engaged version of
	// CSC207H1: Software Design", 300-level "Program-Only" courses ( SOC317H1
	// – SOC354H1; Offerings will vary from year to year.), "complementary
	// courses offered in other programs", "Coverage must include at least two
	// diasporic communities or regions", "the application of digital
	// technologies to humanities scholarship", "3.0 credits focused on a
	// particular global region". A special-topics offering named by its
	// topic ("HIS389H1 (Hungarian History)") is content where it is made
	// (topicMembers).
	{regexp.MustCompile(`\b(?:global|international)\s+experience\b|\b[Ss]ome\s+offerings\s+of\b|\bdiasporic\s+communities\b|\bapplication\s+of\s+digital\s+technologies\b|\bfocus(?:ed)?\s+on\s+a\s+(?:particular\s+)?(?:global\s+region|theme)\b|\bcomplementary\s+courses\b[^.]*\boffered\s+(?:in|by)\s+other\s+programs\b`), ""},
	{regexp.MustCompile(`^Industry-engaged\s+version\s+of\s+(` + codePat + `)(?::.*)?$`), "$1"},
	{regexp.MustCompile(`["“”]Program-Only["“”]\s*|\s*\(\s*` + codePat + `\s*[–-]\s*` + codePat + `;[^()]*\)`), ""},
	// Lists kept elsewhere: "fulfilled by any 400-level courses listed in the
	// Centre for Jewish Studies Undergraduate Course Handbook", "5.0 credits
	// in any of the four areas of Jewish Studies", "offered by the Canadian
	// Studies program" (design D5).
	{regexp.MustCompile(`;\s*fulfilled\s+by\b[^;]*\bHandbook\b|\bUndergraduate\s+Course\s+Handbook\b|\bfour\s+areas\s+of\s+Jewish\s+Studies\b|^offered\s+by\s+the\s+[\w\s]+\s+program$`), ""},
}

// contentRest reports whether t says what a course's content, topic, type or
// language decides (complContentRes), and returns t without those clauses.
func contentRest(t string) (string, bool) {
	found := false
	for _, c := range complContentRes {
		if c.re.MatchString(t) {
			found = true
			t = c.re.ReplaceAllString(t, c.repl)
		}
	}
	return strings.TrimSpace(t), found
}

// complContentFilterRe: what a checker could read in a sentence, besides a
// bare amount: a course, a level, a designator, a named list.
var complContentFilterRe = regexp.MustCompile(codePat + `|\b[1-4]00(?:\+|\s*-?\s*(?:level|series))|\b[A-Z]{3}\b|\b(?:Groups?|Lists?|Categor(?:y|ies))\s+[A-Z0-9#]`)

// textKinds sets the kind of the texts other passes left as other, once
// they have all read them (they look for kind other), before the Review
// markers (which take their causes' kind):
//   - content (design D1): what the course's content, topic, type or language
//     of instruction decides (complContentRes). A row that is only such a
//     sentence keeps what a checker can read of it, as
//     and(credits{amount, designators, levels}, text) (contentPool); one
//     whose rest names courses or filters it can't read stays other, so the
//     kind never hides a rule;
//   - permission: an alternative a person approves ("in consultation with
//     the Department", complApprovalRes), next to a strict branch;
//   - the "e.g." level route on a floor ("Students may be permitted to take
//     Math graduate courses. A 0.5 credit of these courses may count ...
//     (e.g., as courses "at the 400-level")"): bounded, or(floor,
//     and(floor less 0.5, text{permission})) (egLevelRoute).
func textKinds(c *types.Completion) {
	for _, r := range c.Requirements {
		if r.Req == nil {
			continue
		}
		if c.Lists == nil && !listBelowGiven(c.Raw, r) {
			listBelowContent(r.Req)
		}
		if r.Req.Type == types.NodeText {
			r.Req = wholeTextKind(r.Req)
			continue
		}
		egLevelRoute(r)
		someOfferings(r.Req)
		partTextKinds(r.Req)
	}
	for _, l := range c.Lists {
		for i, it := range l.Items {
			if it.Type == types.NodeText {
				l.Items[i] = wholeTextKind(it)
			} else {
				partTextKinds(it)
			}
		}
	}
}

// listBelowContent makes the texts of n that draw on "the list below" of a
// program with no list content: the department's list, which the calendar
// doesn't give, decides.
//
// The calendar gives the list when course codes follow the row's line in
// the raw, though the parser kept it as notes (ASMIN1210's cultural areas,
// "_Czech and Slovak_: SLA200H1, ..."): listBelowGiven, and the text stays
// other.
func listBelowContent(n *types.ReqNode) {
	walkReq(n, func(x *types.ReqNode) {
		if x.Type == types.NodeText && x.Kind == types.TextOther && complNoListBelowRe.MatchString(x.Text) {
			x.Kind = types.TextContent
		}
	})
}

// listBelowGiven reports whether a course code follows r's first line in
// raw (listBelowContent).
func listBelowGiven(raw string, r *types.Requirement) bool {
	head, _, _ := strings.Cut(r.Raw, "\n")
	i := strings.Index(raw, head)
	return head != "" && i >= 0 && complCodeRe.MatchString(raw[i+len(head):])
}

// wholeTextKind gives a row's text body its kind: content, with what a
// checker can read of it put beside it (and(pool, text)), when the rest
// reads as a pool (contentPool) or names no course or filter. A range of
// codes the content names within the pool's filters is the pool, with no
// text ("2.0 SOC credits from 300-level "Program-Only" courses ( SOC317H1 –
// SOC354H1; ...)": credits{2, the codes codeRange spans}).
func wholeTextKind(n *types.ReqNode) *types.ReqNode {
	if n.Kind != types.TextOther && n.Kind != types.TextMembership {
		return n
	}
	rest, ok := contentRest(complClean(n.Text))
	if !ok {
		return n
	}
	if pool := contentPool(rest); pool != nil {
		if m := complCodeRangeRe.FindStringSubmatch(n.Text); m != nil && pool.Type == types.NodeCredits && pool.Amount > 0 && pool.Courses == nil {
			if codes := codeRange(m[1], m[2]); codes != nil && !slices.ContainsFunc(codes, func(c string) bool { return !filterMatches(pool, c) }) {
				return &types.ReqNode{Type: types.NodeCredits, Amount: pool.Amount, Courses: codes}
			}
		}
		n.Kind = types.TextContent
		return andNode(pool, n)
	}
	if !complContentFilterRe.MatchString(rest) {
		n.Kind = types.TextContent
	}
	return n
}

// contentPool parses what is left of a content sentence (contentRest): a
// course, or an amount of credits with designators or levels and nothing
// else; nil otherwise.
func contentPool(rest string) *types.ReqNode {
	if rest == "" {
		return nil
	}
	n, _ := parseCompletionText(rest)
	switch {
	case n == nil:
		return nil
	case n.Type == types.NodeCourse:
		return n
	case n.Type == types.NodeCredits && n.Amount > 0 && n.MaxAmount == nil && (n.Designators != nil || n.Levels != nil) &&
		n.Courses == nil && n.Lists == nil && n.Requirements == nil && n.Breadth == nil && !n.Transfer && !n.CrNcr &&
		n.ExcludeDesignators == nil && n.MaxCourseAmount == 0:
		return n
	}
	return nil
}

// partTextKinds gives the texts inside n (not n itself) their kind: content
// when the courses the text names are named beside it too (a topic offering
// whose code is in the pool), permission for an approval alternative of an
// or with a strict branch.
func partTextKinds(n *types.ReqNode) {
	if n == nil {
		return
	}
	codes := nodeCodes(n)
	strict := n.Type == types.NodeOr && slices.ContainsFunc(n.Items, func(x *types.ReqNode) bool { return x.Type != types.NodeText })
	for i, it := range n.Items {
		if it.Type != types.NodeText {
			partTextKinds(it)
			continue
		}
		t := complClean(it.Text)
		named := !slices.ContainsFunc(complCodeRe.FindAllString(t, -1), func(c string) bool { return !slices.Contains(codes, c) })
		switch {
		case it.Kind != types.TextOther && it.Kind != types.TextMembership:
		case isComplApprovalText(t):
			// A person decides, whatever the course's content ("may in
			// consultation with the Department take ... GER topics courses
			// (taught in German)"); without a strict branch beside it, it is
			// the requirement itself, and stays other.
			if strict && it.Kind == types.TextOther {
				it.Kind = types.TextPermission
			}
		case complPlacementTextRe.MatchString(t):
			it.Kind = types.TextPlacement
		case strict && complAnyOtherApprovedRe.MatchString(t):
			it.Kind = types.TextPermission
		case named && contentText(t):
			it.Kind = types.TextContent
			// A part of an and keeps what a checker can read of it beside
			// it, as a whole row does (wholeTextKind): "BIO120H1; plus 2.5
			// first year Science credits" is and(BIO120H1,
			// credits{2.5, levels [100]}, text{content}). One with its pool
			// beside it already (hdrTextRow) is left alone.
			pooled := slices.ContainsFunc(n.Items, func(x *types.ReqNode) bool { return x.Type == types.NodeCredits })
			if rest, _ := contentRest(t); n.Type == types.NodeAnd && !pooled {
				if pool := contentPool(rest); pool != nil && pool.Type == types.NodeCredits {
					n.Items[i] = andNode(pool, it)
				}
			}
		}
	}
}

var (
	// complPlacementTextRe: what a placement or proof of level decides:
	// "Students whose placement assessment exempts them from the requirement
	// must substitute ...", "OR proof of comparable German level".
	complPlacementTextRe = regexp.MustCompile(`(?i)\bplacement\s+(?:assessment|test)\s+exempts?\b|^proof\s+of\s+(?:comparable|equivalent)\s+[A-Z][a-z]+\s+(?:level|proficiency)$`)
	// complAnyOtherApprovedRe: "any other approved course for which the student
	// has appropriate prerequisites", an approval beside the listed courses.
	complAnyOtherApprovedRe = regexp.MustCompile(`(?i)^(?:or\s+)?any\s+other\s+approved\s+courses?\b`)
	// complNoListBelowRe: "2.5 credits in MUS courses from the list below",
	// in a program with no list: which courses the department lists is
	// content.
	complNoListBelowRe = regexp.MustCompile(`(?i)\bfrom\s+the\s+list\s+below\b`)
)

// contentText reports a content sentence (complContentRes).
func contentText(t string) bool {
	_, ok := contentRest(t)
	return ok
}

// complConsultRe: approval by consultation, beside complApprovalRes: "may in
// consultation with the Department take", "selected in consultation with
// the Undergraduate Coordinator", "through a nomination process".
var complConsultRe = regexp.MustCompile(`(?i)\bin\s+(?:joint\s+)?consultation\s+with\b|\bnomination\s+process\b|\bpending\s+approval\b|\bpre-?approved\b|\bapproved\s+by\s+the\b`)

// isComplApprovalText reports a sentence in which someone approves what
// counts (complApprovalRes, complConsultRe).
func isComplApprovalText(t string) bool {
	return complMatchAny(complApprovalRes, t) || complCountQueryRe.MatchString(t) || complConsultRe.MatchString(t)
}

// complSomeOfferingsRe: courses that count in some offerings only, by
// topic: "Some offerings of HIS357H1, HIS389H1, HIS466H1 and HIS496H1 may
// also count towards this focus".
var complSomeOfferingsRe = regexp.MustCompile(`^[Ss]ome\s+offerings\s+of\s+(` + codePat + `(?:(?:,\s*|,?\s+and\s+)` + codePat + `)*)\s+may\s+also\s+count\b[^.]*$`)

// someOfferings bounds a pool's "some offerings of X may also count"
// alternative, or(pool, text): the courses it names count as Review,
// or(pool, and(credits{amount, pool ∪ X}, text{content})) (design 3.3).
func someOfferings(n *types.ReqNode) {
	if n.Type != types.NodeOr || len(n.Items) < 2 || n.Items[0].Type != types.NodeCredits {
		return
	}
	pool := n.Items[0]
	if pool.Amount <= 0 || pool.Courses == nil || pool.Designators != nil || pool.Levels != nil || pool.Lists != nil {
		return
	}
	for i, it := range n.Items[1:] {
		m := complSomeOfferingsRe.FindStringSubmatch(complClean(it.Text))
		if it.Type != types.NodeText || it.Kind != types.TextOther || m == nil {
			continue
		}
		it.Kind = types.TextContent
		wide := &types.ReqNode{Type: types.NodeCredits, Amount: pool.Amount, Courses: dedupe(append(slices.Clone(pool.Courses), complCodeRe.FindAllString(m[1], -1)...))}
		n.Items[i+1] = andNode(wide, it)
	}
}

// complEgLevelRe: a route that counts a course "as courses at the 400-level"
// for example: "Students may be permitted to take Math graduate courses. A
// 0.5 credit of these courses may count toward specialist program
// requirements, where relevant (e.g., as courses "at the 400-level")".
var complEgLevelRe = regexp.MustCompile(`(?i)\bmay\s+be\s+permitted\b.*\bA\s+0\.5\s+credit\s+of\s+these\s+courses\s+may\s+count\b.*\be\.g\.,?\s+as\s+courses\s+["“]at\s+the\s+\d00-level`)

// egLevelRoute bounds the e.g. level route on a floor, or(floor{amount,
// levels}, text): 0.5 credit of the route's courses may count at the level,
// so the floor less 0.5 is still needed, or(floor, and(floor less 0.5,
// text{permission})); with nothing left, or(floor, text{permission}).
func egLevelRoute(r *types.Requirement) {
	n := r.Req
	if r.Kind != types.RequirementConstraint || n.Type != types.NodeOr || len(n.Items) != 2 {
		return
	}
	floor, text := n.Items[0], n.Items[1]
	if floor.Type != types.NodeCredits || floor.Levels == nil || floor.Amount <= 0 || text.Type != types.NodeText ||
		text.Kind != types.TextOther || !complEgLevelRe.MatchString(text.Text) {
		return
	}
	text.Kind = types.TextPermission
	if rest := floor.Amount - 0.5; rest > 1e-9 {
		less := *floor
		less.Amount = rest
		less.Levels = slices.Clone(floor.Levels)
		n.Items[1] = andNode(&less, text)
	}
}
