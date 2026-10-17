package completion

import (
	"regexp"
	"slices"
	"strings"

	"degree-explorer/parser/grammar"
	"degree-explorer/parser/programs"
	"degree-explorer/types"
)

// SentenceRole says what one sentence of program completion text does, read
// alone; the assembler combines it with the kind of block the sentence is in.
// Cue words beat block context, since a "Notes:" block binds in most programs.
type SentenceRole int

const (
	RoleRequirement SentenceRole = iota // anything binding without a more specific role
	roleConstraint                      // a floor or a must-include on courses counted elsewhere
	roleLimit                           // "no more than": at most an amount counts, the excess is dropped
	RoleListDef                         // a named course list: "Group A: ..."
	RoleAdvisory                        // advice, contacts, procedure and timing: notes
	roleExclusion                       // programs that can't be combined with this one
	roleTransfer                        // a cap or ban on transfer credit
	roleCrNcr                           // a rule on courses taken CR/NCR
	roleOverlap                         // a course may count toward two named requirements
)

const amountPat = grammar.NumPat + `(?:\s*[-–]\s*` + grammar.NumPat + `)?`

var (
	// leadRe matches list markers, footnote stars and note labels before a
	// sentence: "Note 2:".
	leadRe = regexp.MustCompile(`^(?:[-•*]+\s*|\(?(?:\d{1,2}|[a-zA-Z]|[ivx]{1,4})[.)]\s+|(?i:notes?(?:\s+\d)?|nb|important)\s*:\s*)+`)
	// nBRe matches "N.B.", rewritten as a label so it doesn't end a
	// sentence.
	nBRe = regexp.MustCompile(`\bN\.\s?B\.\s*`)
	// amountStartRe matches a full stop before a sentence starting with an
	// amount, a level or a bracketed code: "courses.1.0 credit". Group 1 is the
	// word before it; SplitSentences only splits before an uppercase letter.
	amountStartRe = regexp.MustCompile(`((?:[a-z]+\.)*[a-z]+|\)|` + grammar.CodePat + `)\.(?:\s+\d+\.\d+\s|\d+\.\d+\s+(?:[A-Z]{3}\s+)?credits?\b|\s+\(\s*` + grammar.CodePat + `|\s+` + grammar.LevelPat + `(?:/` + grammar.LevelPat + `)*(?:\+|-|\s+level))`)
	// letterNoteRe matches the start of each inline lettered note after the
	// first: "; (b) ".
	letterNoteRe = regexp.MustCompile(`;\s*\([b-z]\)\s`)
	// italicNoteRe matches an italic note glued to an item with no full
	// stop: "CHM151Y1 _Transfer credits".
	italicNoteRe = regexp.MustCompile(`(` + grammar.CodePat + `|\))\s+(?:\*\*)?_(?:\*\*)?([A-Z])`)
	// parenRe matches bracketed text, removed before the advice checks so
	// that "( CHM151Y1 strongly recommended)" doesn't make its item advice.
	parenRe = regexp.MustCompile(`\([^()]*\)|\[[^\[\]]*\]`)
	// recommendedTagRe matches advice in brackets: "(recommended)". With
	// no code in the sentence, it marks a heading, not an item's annotation.
	recommendedTagRe = regexp.MustCompile(`(?i)\((?:strongly\s+|highly\s+)?recommended\)`)
	// attributiveAdviceRe matches "recommended" naming a set of courses:
	// "the recommended electives". It is removed so it can't silence a cap.
	attributiveAdviceRe = regexp.MustCompile(`(?i)\b(?:the|these|those|of|from)\s+(?:list\s+of\s+)?(?:recommended|suggested)\s+(?:[\w-]+\s+)?(?:courses|lists?|electives)\b`)
	// wordRe matches a letter or digit; a sentence with none says nothing.
	wordRe = regexp.MustCompile(`[A-Za-z0-9]`)
	// bareMarkerRe matches a list marker left alone after cleaning: "a)".
	bareMarkerRe = regexp.MustCompile(`^(?:\d{1,2}|[a-zA-Z]|[ivxIVX]{1,4})[.)]?$`)

	codeLeadRe = regexp.MustCompile(`^(?i:in\s+addition,?\s+)?[(\s]*` + grammar.CodePat)
	amountRe   = regexp.MustCompile(`\b\d+\.\d+\b|\b\d+\s+(?:full|half)[-\s]courses?\b|(?i:\b(?:one|two|three|four)\s+years?\s+of\s+study\b)`)
	// filterRe matches a filter a requirement can carry: "300-level".
	filterRe = regexp.MustCompile(`\b` + grammar.LevelPat + `(?:\+|\s*-?\s*(?:level|series)|\s*-?\s*/)|\bbreadth\b|\bBR\s*=|\b[A-Z]{3}(?:\s*/\s*[A-Z]{3})*\s*-?\s*(?:courses?|credits?|prefix|designators?|coded)\b|\b(?:Groups?|Lists?|Clusters?)\s+[A-Z0-9]\b`)
	// bindingRe matches a verb that binds even without a code or an
	// amount: "must".
	bindingRe = regexp.MustCompile(`(?i)\bmust\b|\brequire[sd]?\b|\bcannot\b|\bmay\s+not\b|\bwill\s+not\b|\bnot\s+(?:be\s+)?(?:accepted|counted|permitted|eligible|included)\b|\bonly\b|\bno\s+(?:more|other|course|courses|credits?)\b|\bwill\s+count\b`)
	// headingRe matches short text with no full stop; with no verb from
	// headingVerbRe it is a heading: "Core Courses". It defaults to
	// requirement on purpose: a heading taken for advice drops the rows under it.
	headingRe     = regexp.MustCompile(`^[^.]{1,100}$`)
	headingVerbRe = regexp.MustCompile(`(?i)\b(?:is|are|was|were|will|may|might|can|should|must|has|have|need|needs)\b`)

	// combineRe matches programs that can't be combined, beyond the
	// enrolment phrasings: "cannot also complete". "may not be combined with"
	// never occurs but stays as enrolment's cannot-combine phrasing.
	combineRe    = regexp.MustCompile(`(?i)\bmay\s+not\s+be\s+combined\s+with\b|\bcannot\s+be\s+paired\s+with\b|\bmay\s+(?:only\s+)?enrol(?:l)?\s+in\s+only\s+one\b|\bnot\s+(?:be\s+)?allowed\s+to\s+enrol(?:l)?\s+concurrently\b|\bcannot\s+also\s+complete\b|\bcannot\s+be\s+enrol(?:l)?ed\s+in\s+any\s+other\s+program\b|\bmay\s+not\s+simultaneously\s+(?:be\s+)?(?:registered|enrol(?:l)?(?:ed)?)\s+(?:for|in)\b`)
	exclusionRes = []*regexp.Regexp{combineRe, programs.CombinedWithRe, programs.CombineRe, programs.SimultaneousRe, programs.NotOpenRe}

	// overlapRe matches a course counting toward two named requirements:
	// "may contribute to both". "may also count toward" is always membership,
	// not sharing.
	overlapRe = regexp.MustCompile(`(?i)\b(?:may|can)\s+(?:also\s+)?(?:contribute|count|be\s+(?:used|counted|applied))\s+(?:to(?:wards?)?|for|to\s+satisfy)\s+both\b`)

	crNcrRe = regexp.MustCompile(`(?i)\bCR\s*/\s*NCR\b|\bcredit\s*/\s*no\s+credit\b`)
	// crNcrRuleRe matches what makes a CR/NCR mention a rule rather than a
	// pointer to details: "cannot".
	crNcrRuleRe = regexp.MustCompile(`(?i)\bcannot\b|\bmay\s+not\b|\bnot\s+(?:be\s+)?accepted\b|\bdo\s+not\s+accept\b|\bmay\s+(?:be\s+)?(?:use|used|accepted|taken)\b|\bcan\s+be\s+taken\b|\bonly\b|\bno\s+courses\b`)

	// crNcrAllowRe matches a CR/NCR permission with no ban, which is the
	// default: "Students may use the CR/NCR option towards any of the courses".
	crNcrAllowRe = regexp.MustCompile(`(?i)^students\s+may\s+use\s+(?:the\s+)?CR\s*/\s*NCR\s+(?:option\s+)?(?:on|towards?|for)\s+(?:the|any)\b[^;]*$`)

	transferRe = regexp.MustCompile(`(?i)\btransfer\s+credits?\b|\bin\s+transfer\s+credit\b`)
	// transferRuleRe matches a cap or ban on transfer credit: "no more
	// than".
	transferRuleRe = regexp.MustCompile(`(?i)\b` + grammar.NoMoreThanPat + `\b|\bmaximum\b|\bmore\s+than\s+\d|\bup\s+to\b|\bcannot\b|\bmay\s+not\b|\bnot\s+(?:be\s+)?accepted\b|\bwill\s+not\b`)

	// listLabelRe matches a list label, which names a list even with a cap
	// in it: "Group A:".
	listLabelRe = regexp.MustCompile(`^(?:[\w/&-]+\s+){0,2}(?:Group|List|Cluster|Category|Sub-?group|Area)\s+(?:[A-Z]|\d{1,2}|[IVX]{1,4}|[a-z]\))(?:\s*$|\s*[-–:(])`)
	// fromLabelRe matches a label that draws on a list rather than naming
	// one: "From Group A: 1.0 credit".
	fromLabelRe = regexp.MustCompile(`(?i)^(?:from|in|of)\b`)
	// listHeadRe matches a label followed by codes: "Hydrology/GIS/Remote
	// Sensing: GGR273H1". notListHeadRe rejects a section or a choice,
	// which are items: "First Year".
	listHeadRe    = regexp.MustCompile(`^([^:.]{1,120}):\s*\(?\s*` + grammar.CodePat)
	notListHeadRe = regexp.MustCompile(`(?i)\byears?\b|\b(?:all|one|two|three|four|any|\d)\b(?:\s+\w+)?\s+(?:of|from)\b|` + grammar.CodePat)

	// allocLeadRe matches a sentence that leads with an amount, after an
	// optional label: "2.0 credits from Groups A-C". A floor or a cap in a
	// later clause belongs to that allocation; the item grammar splits it off.
	allocLeadRe = regexp.MustCompile(`(?i)^(?:[^:.()]{1,60}:\s*|[^:.()]{1,40}\(\s*)?` +
		`(?:\(\s*|in\s+addition,?\s+|any\s+|all\s+|` + grammar.AtLeastPat + `\s+|(?:an\s+)?additional\s+|a\s+total\s+of\s+|a\s+minimum\s+of\s+|complete\s+|` +
		`students\s+(?:must\s+)?complete\s+(?:a\s+total\s+of\s+)?|the\s+(?:[\w&]+\s+){1,3}requires\s+)*` +
		`(?:(?:a\s+|one\s+)?` + amountPat + `\s+(?:\(\s*` + grammar.NumPat + `\s+credits?\s*\)\s+)?(?:[\w/+-]+\s+){0,3}?(?:credits?|(?:half|full)[-\s]courses?|courses?)\b` +
		`|total(?:\s+credits)?\s*:\s*` + amountPat +
		`|[^.;:]{0,80}?\b(?:to|for)\s+a\s+total\s+of\b` +
		`|(?:an?\s+)?additional\s+(?:[\w/+-]+\s+){0,3}?(?:credits?|courses?)\b` +
		`|select\s+the\s+remaining\s+credits\b)`)
	// clauseEndRe matches the end of an amount lead's first clause:
	// "including".
	clauseEndRe = regexp.MustCompile(`(?i)[,;:(]|\b(?:of\s+which|including|with|to\s+include)\b`)
	// relativeRe matches a relative clause, which ends the first clause
	// only after a numeric amount: "2.0 credits in courses where the primary
	// language".
	relativeRe = regexp.MustCompile(`(?i)\b(?:where|that|which|who|whose)\b`)
	// leadCueRe matches a cue in the lead that makes it a rule, not an
	// allocation: "No more than".
	leadCueRe = regexp.MustCompile(`(?i)\b(?:no\s+more|at\s+most|maximum|only|up\s+to|may|can|cannot)\b`)
	// takenFromRe matches an allocation's codes after the lead: "1.0
	// credit must be taken from GER270H1". "must be taken at the 300-level" is
	// a floor.
	takenFromRe = regexp.MustCompile(`(?i)^\s*must\s+be\s+(?:taken|chosen|selected)\s+from\s+(?:the\s+following:?\s*)?` + grammar.CodePat)
	// leadVerbRe matches a verb in the first clause, which makes the
	// amount a subject rather than an allocation: "1.0 credit must be at the
	// 300-level".
	leadVerbRe = regexp.MustCompile(`(?i)\b(?:must|should|needs?\s+to|can|cannot|may|will|is|are|counts?)\b`)

	// constraintRes are cues for a floor or a must-include on courses
	// counted under another requirement. A bare "at least" is a constraint only
	// by block context, so here it needs a modal.
	constraintRes = []*regexp.Regexp{
		// "These 3.0 credits must include:"
		regexp.MustCompile(`(?i)\bmust\s+include\b`),
		// "Higher Years (must include ...), including at least 0.5 at ..."
		regexp.MustCompile(`(?i)\bincluding\s+` + grammar.AtLeastPat + `\b`),
		// "At least 2.0 credits at the 300+ level, 0.5 of which must be ..."
		regexp.MustCompile(`(?i)(?:^|,|\b(?:credits?|courses?|` + grammar.NumPat + `|one))\s*of\s+which\b`),
		// "The choices in 3 must satisfy the requirement for ..."
		regexp.MustCompile(`(?i)\bthe\s+choices\s+(?:in|from)\s+\S+(?:\s+\S+)?\s+must\b`),
		// "At least 1.5 credits must be in FRE or FSL courses"
		regexp.MustCompile(`(?i)\b` + grammar.AtLeastPat + `\b[^.]*?\b(?:must|needs?\s+to|has\s+to|have\s+to|is\s+required)\b`),
		// "All 4.0 credits must be NMC and/or NML courses"
		regexp.MustCompile(`(?i)\b(?:credits?|courses?)\s+(?:must|needs?\s+to)\s+(?:be\s+(?:taken\s+)?(?:at|in|from|a|an|` + grammar.LevelPat + `|[A-Z]{3})\b|come\s+from\b|have\s+the\b)`),
	}
	// scopeRe matches a scope: "Of these 4.0 credits,". The cue after it
	// decides the role; with none, it is a constraint.
	scopeRe = regexp.MustCompile(`(?i)^of\s+(?:these|the)\b`)

	// limitRes are cues for a limit: at most an amount counts.
	limitRes = []*regexp.Regexp{
		// "No more than 1.0 credit of 100-level courses may be used"
		regexp.MustCompile(`(?i)\b` + grammar.NoMoreThanPat + `\b|\bat\s+most\b|\bnot\s+exceed\b|\bcannot\s+comprise\s+more\s+than\b`),
		// "a maximum of 1.0 total program credits may be drawn from courses
		// taught in English"
		regexp.MustCompile(`(?i)\bmaximum\s+(?:of\s+)?(?:a\s+|one\s+|two\s+|\d)`),
		// "Only 1.0 credit from 100-level ENG courses may be counted"
		regexp.MustCompile(`(?i)\bonly\s+(?:a\s+)?(?:maximum\s+of\s+)?(?:` + grammar.NumPat + `|one|two)\b[^.]*?\b(?:will|may|can)\s+(?:be\s+)?(?:accepted|counted|count|used|applied)\b`),
		// "Up to 1.0 credit can be used from GGR493Y1"
		regexp.MustCompile(`(?i)\bup\s+to\s+(?:a\s+|one\s+|two\s+)?(?:` + grammar.NumPat + `\s+)?(?:credits?|courses?|half|full)?[^.]*?\b(?:can|may)\b`),
		// "MAT221H1 may not be used for this requirement".
		regexp.MustCompile(`(?i)\b(?:cannot|may\s+not|will\s+not)\s+(?:count|be\s+(?:counted|used|applied|accepted))\s+(?:to|for|toward|towards)\b`),
		// "STA220H1 and STA255H1 are not accepted for the major"
		regexp.MustCompile(`(?i)\bnot\s+(?:be\s+)?(?:accepted|counted)\s+toward|\bdo\s+not\s+accept\b|\bare\s+not\s+accepted\s+for\b|\bnot\s+included\s+for\s+consideration\b|\bnot\s+permitted\s+to\s+take\b`),
		// "Students cannot take more than a 0.5 credit at the 400-level"
		regexp.MustCompile(`(?i)\b(?:cannot|may\s+not|will\s+not)\s+(?:use|take|count|be\s+met\s+by)\s+more\s+than\b`),
	}

	// recommendRe matches advice: "We recommend that students take
	// MAT148H1".
	recommendRe = regexp.MustCompile(`(?i)\brecommend(?:s|ed|ation|ations)?\b|\bencouraged?\b|\bsuggest(?:s|ed|ion|ions)?\b|\badvis(?:ed|able)\b|\bshould\s+(?:especially\s+|seriously\s+|also\s+)?consider\b|\bshould\s+note\s+that\b|\b(?:may|might)\s+wish\b|\bpreferabl[ey]\b`)
	// timingNoteRe matches timing advice on a required item, removed
	// before the advice checks: ", normally taken in the first year".
	timingNoteRe = regexp.MustCompile(`(?i),\s*(?:(?:highly|strongly)\s+)?(?:recommended\s+to\s+be\s+taken|normally\s+taken)\b[^;]*`)

	// adviceVetoRe vetoes advice: "must" binds, and so do courses called
	// required for the program: "required for the specialist" ...
	adviceVetoRe = regexp.MustCompile(`(?i)\bmust\b|\b(?:are|is)\s+(?:an?\s+)?required\s+courses?\b|\brequired\s+for\s+the\s+(?:program|specialist|major|minor|focus|certificate)\b`)
	// ... unless the "must" is procedure or timing (procedureRe):
	// "students must request permission" ...
	procedureRe = regexp.MustCompile(`(?i)\bmust\s+(?:also\s+)?(?:first\s+)?(?:request|submit|check|consult|contact|apply|speak|receive|obtain\s+(?:prior\s+)?(?:consent|permission)|meet\s+(?:with|(?:all\s+)?(?:the\s+)?(?:specified\s+)?prerequisites)|be\s+enrolled|be\s+made|be\s+submitted)\b|\brequests?\s+must\s+be\b|\bmust\b[^.]*\bbefore\s+(?:starting|taking|enrolling|beginning|graduation)\b`)
	// ... or a degree-level rule (degreeRuleRe): "must complete a total of
	// at least 12.0 distinct credits" ...
	degreeRuleRe = regexp.MustCompile(`(?i)\bcombination\s+of\s+programs\b|\bdistinct\s+credits\b|\b\d+\.\d\s+credit\s+rule\b|\bin\s+addition\s+to\s+the\s+` + grammar.NumPat + `\s+credits\s+required\s+for\b|\btowards?\s+their\s+(?:[\w&]+\s+){0,3}degree\b`)
	// ... or a description naming required courses without requiring them here
	// (vetoDescriptionRe): "ACT390H1 is a required course in the actuarial
	// science specialist program".
	vetoDescriptionRe = regexp.MustCompile(`(?i)\bas\s+well\s+as\s+suggested\s+courses\b|^a\s+significant\s+part\s+of\s+the\s+course\s+must\s+involve\b|^` + grammar.CodePat + `\s+is\s+a\s+required\s+course\s+in\s+the\s+(?:[\w-]+\s+){1,4}(?:specialist|major|minor)\s+program$`)
)

// notBindingRes are sentences that read like rules but which a checker of
// this program can't or needn't evaluate. "must" vetoes them unless it is
// procedure or a degree-level rule.
var notBindingRes = []*regexp.Regexp{
	// A placement exception, which names no pool: "After consultation with ...,
	// exceptions may be made ... regarding the sequence of language courses".
	regexp.MustCompile(`(?i)^after\s+consultation\s+with\b[^.]*\bexceptions\s+may\s+be\s+made\b[^.]*\bsequence\s+of\s+language\s+courses\b`),
	// A program-wide consideration naming no requirement: "Courses that are in
	// the current academic Calendar that include significant ... may be
	// considered, in consultation with the ... Director".
	regexp.MustCompile(`(?i)^courses\s+that\s+are\s+in\s+the\s+current\s+academic\s+calendar\s+that\s+include\s+significant\b[^.]*\bmay\s+be\s+considered,\s+in\s+consultation\s+with\b`),
	// Permissions and advice that don't change what counts: "Students are
	// advised to discuss potential course substitutions".
	regexp.MustCompile(`(?i)^(?:[\w-]+\s+){0,2}students\s+can\s+take\s+` + grammar.CodePat + `\b.*\bprovided\s+they\s+have\b|^students\s+can\s+take\s+either\s+course\s+as\s+their\s+required\b.*\bas\s+an\s+additional\s+elective$|^students\s+are\s+advised\s+to\s+discuss\b|^a\s+significant\s+part\s+of\s+the\s+course\s+must\s+involve\b`),
	// Rules about other programs: "IRE/IRW courses cannot be used to fulfill
	// enrolment/program requirements in Economics".
	regexp.MustCompile(`(?i)^except\s+for\s+students\s+enrolled\s+in\b[^,;]*,\s+who\s+may\s+apply\b.*\btowards?\s+their\s+(?:Specialist|Major|Minor)\s+program$|\bprogram\s+requirements\s+in\s+(?-i:[A-Z])|\brequirements\s+for\s+(?:a|another|any)\s+(?:Specialist|Major|Minor)\b|\btowards?\s+an?\s+` + grammar.DesigPat + `\s+(?:major|minor|specialist)\b|\bnot\s+considered\s+equivalent\b`),
	// Timing the checker doesn't enforce: "a maximum of 3.0 credits from Group
	// B per year".
	regexp.MustCompile(`(?i)\bper\s+(?:academic\s+)?(?:year|session|term|semester)\b`),
	degreeRuleRe,
	// The student's own checks: "Students are responsible for checking priority
	// of courses".
	regexp.MustCompile(`(?i)\bstudents?['’]?s?\s+responsibility\b|\bare\s+responsible\s+for\b`),
	// The default sharing rule, restated: "*If not taken for Requirement 13
	// above", "Each course can count toward only one requirement".
	regexp.MustCompile(`(?i)^\*?\s*if\s+not\s+(?:already\s+)?taken\s+(?:in|for)\s+(?:list\s+\d+|requirement\s+\d+(?:\s+above)?|the\s+requirement\s+immediately\s+above)$`),
	regexp.MustCompile(`(?i)\bonly\s+count\s+once\b|\bonly\s+be\s+counted\s+towards?\s+one\b|\bcounted\s+twice\b|\bused\s+(?:more\s+than\s+once|twice)\b|\bcount\s+toward\s+only\s+one\s+requirement\b|\bonly\s+count\s+toward\s+one\s+requirement\b|\bcannot\s+be\s+counted\s+toward\s+another\b|\bcannot\s+overlap\b|\blisted\s+as\s+exclusions\s+to\s+one\s+another\b`),
	// No substitute for a listed course: "HMB200H1 cannot be used in place of
	// PSY290H1".
	regexp.MustCompile(`(?i)\bcannot\s+be\s+used\s+(?:in\s+place\s+of|as\s+substitutes?)\b|\bnot\s+accepted\s+in\s+lieu\s+of\b`),
	// Course enrolment notices, exclusions and caps: "IMM435H1 is capped at 40
	// students".
	regexp.MustCompile(`(?i)\bcannot\s+take\s+` + grammar.CodePat + `|\bcapped\s+at\b|^enrolment\s+in\s+(?:either\s+|any\s+of\s+)?` + grammar.CodePat + `(?:\s*(?:,|or|and|/)\s*` + grammar.CodePat + `)*\s+is\s+limited\b`),
	// Another program described: "The focus that is available for students to
	// pursue is ...".
	regexp.MustCompile(`(?i)\bfoc(?:us|i)\s+that\s+(?:is|are)\s+available\b`),
}

// adviceRes is advice, which doesn't bind unless the sentence offers a
// substitution (substitutionRes).
var adviceRes = []*regexp.Regexp{recommendRe,
	// What to take for a focus within the program's choices: "Students who want
	// to focus on ... medical anthropology should take ANT203Y1".
	regexp.MustCompile(`(?i)\bstudents\s+who\s+(?:want|wish)\s+to\s+focus\b[^.]*?\bshould\s+take\b`),
}

// substitutionRes are the approval routes that offer a substitution.
// Unlike the others, they beat advice in the same sentence: "Students are
// advised to discuss potential course substitutions".
var substitutionRes = []*regexp.Regexp{
	// "Up to 2.0 credits in GRK or LAT may be substituted for the 5.5 credits"
	regexp.MustCompile(`(?i)\b(?:may|can)\s+(?:request\s+to\s+)?(?:be\s+)?substitut(?:e|ed)\b`),
	// "Course substitutions are possible with written permission of the
	// Program Director".
	regexp.MustCompile(`(?i)\bsubstitutions?\s+(?:are|is)\s+possible\b`),
	// "Students can request substitutions from the IRP Director"
	regexp.MustCompile(`(?i)\bto\s+substitute\b|\brequest\s+(?:a\s+)?substitutions?\b|\bdiscuss\s+(?:potential\s+|possible\s+)?(?:course\s+)?substitutions?\b`),
}

// approvalRes are approval routes, which the assembler encodes as a
// permission alternative. They bind even with a cap or a contact in them.
var approvalRes = slices.Concat(substitutionRes, []*regexp.Regexp{
	// "Other introductory language classes may be approved by the Director".
	// The other verbs need an approver after them: "may be accepted from ..."
	// is a cap.
	regexp.MustCompile(`(?i)\bmay\s+(?:also\s+)?be\s+(?:approved|considered)\b|\bmay\s+(?:also\s+)?be\s+(?:accepted|chosen|included|eligible)\s+(?:by|with|as\s+(?:designated|determined)|on\s+a\s+case-by-case\s+basis)\b`),
	// A topic-dependent member: "Special Topics courses may be chosen when
	// offered".
	regexp.MustCompile(`(?i)\bmay\s+be\s+chosen\s+when\s+offered\b`),
	// "Other languages are accepted with special permission of the
	// Undergraduate Director"
	regexp.MustCompile(`(?i)\bwith\s+(?:the\s+)?(?:(?:prior|special|written)\s+)*(?:approval|permission)\b|\bon\s+the\s+approval\b|\bneed\s+written\s+permission\b`),
	// "Students may also petition the Program Coordinator"
	regexp.MustCompile(`(?i)\bmay\s+request\s+to\s+count\b|\bmay\s+(?:also\s+)?petition\b|\b(?:might|may)\s+also\s+qualify\b|\bcourses?\s+qualif(?:y|ies)\b`),
	// "Students may be permitted to take Math graduate courses".
	regexp.MustCompile(`(?i)\bmay\s+be\s+permitted\b`),
	// "Courses taken in Summer Abroad programs ... will need to be
	// pre-approved"
	regexp.MustCompile(`(?i)\b(?:will\s+need\s+to|must)\s+be\s+pre-?approved\b|\bwill\s+be\s+considered\b`),
	// "... would like it to count exceptionally towards your program
	// requirements"
	regexp.MustCompile(`(?i)\bcount\s+exceptionally\b|\bfind\s+out\s+if\b`),
	// "if you wish to request consideration of these courses for
	// requirement 7".
	regexp.MustCompile(`(?i)\brequest\s+consideration\b`),
	// A pointer to a list of approved courses kept elsewhere, which widens the
	// pool: "A complete list of eligible courses is available at the
	// departmental website".
	regexp.MustCompile(`(?i)\blist\s+of\s+courses\s+pre-?approved\s+for\s+program\s+credit\b|\blist\s+of\s+eligible\s+courses\s+is\s+available\b`),
	// Credit by consultation: "In consultation with the Undergraduate
	// Coordinator, students may take 0.5 credit".
	regexp.MustCompile(`(?i)^in\s+consultation\s+with\b[^.]*?,\s*students\s+may\s+take\b`),
	// Approval by the program: "another course approved in joint consultation
	// with ...". Not "pending approval": as a route it would land on an item,
	// not on the program-level share it widens.
	regexp.MustCompile(`(?i)\bapproved\s+in\s+(?:joint\s+)?consultation\b|^permission\s+to\s+count\b`),
	// Approval by contact, which beats the contact advice: "Please consult
	// with CEES and Slavic departments to get approval".
	regexp.MustCompile(`(?i)\bconsultation\b[^.]*\bexceptions?\s+(?:may|can)\s+be\s+(?:made|granted)\b|\b(?:consult(?:ation)?|contact|speak|reach\s+out)\b.*?\bto\s+(?:get|obtain|seek)\s+(?:prior\s+)?approval\s*$|\bwish(?:es|ing)?\s+to\s+apply\b[^.]*\btowards?\s+(?:the|this|their|your)\s+(?:[\w-]+\s+){0,2}(?:requirements?|programs?)\b`),
})

// countQueryRe matches a question about counting another course, an
// approval route even in a sentence of advice: "whether it may be appropriate
// to count a related course towards Focus requirements".
var countQueryRe = regexp.MustCompile(`(?i)\b(?:whether|if)\b[^.]*\bcount\b[^.]*?\btowards?\s+(?:the\s+|your\s+|this\s+)?(?:[\w-]+\s+){0,2}(?:programs?|focus|requirements?)\b`)

// widenRe matches a pool widened with a cap: "Up to 1.0 credit may be from
// other departments". The assembler adds both the wider pool and the cap.
var widenRe = regexp.MustCompile(`(?i)\bup\s+to\b.*?\b(?:may|can)\s+(?:also\s+be\s+(?:taken|chosen|drawn|completed)\s+from|be\s+(?:(?:taken|chosen|drawn)\s+)?from\s+(?:the\s+following\s+)?(?:approved\s+courses\s+offered\s+by\s+)?(?:cognate|other\s+departments))`)

// bindingProseRes are binding sentences that may have no code, amount or
// filter.
var bindingProseRes = []*regexp.Regexp{
	// Pools in prose: "Other courses are also eligible to meet this
	// requirement".
	regexp.MustCompile(`(?i)\b(?:are|is)\s+(?:also\s+)?eligible\b|\bcounts?\s+towards?\s+this\s+requirement\b|\bsatisfied\s+by\b|\b(?:can|may)\s+(?:also\s+)?be\s+used\s+to\s+meet\b|\bmeet\s+this\s+requirement\b`),
	// Inheritance: "begins with the requirements of First Year".
	inheritRe,
	// An exception to the total: "There is no need to replace the missing
	// half-credit for program completion".
	regexp.MustCompile(`(?i)\bno\s+need\s+to\s+replace\b`),
}

// asideRes are advice, explanation and restatement that read like a
// requirement (an amount, a code, "must"), checked before every rule.
var asideRes = []*regexp.Regexp{
	// A course's own corequisite, restated: "EEB498Y1 concurrent with research
	// issues course EEB488H1".
	regexp.MustCompile(`^` + grammar.CodePat + `\s+concurrent\s+with\b`),
	// "Students interested in Mineral Exploration: First Year: BIO120H1"
	regexp.MustCompile(`(?i)^students\s+(?:interested\s+in|with\s+an\s+interest\s+in)\b[^:]*:|\bmight\s+(?:be\s+)?of\s+interest\b`),
	// The courses for a theme: "Neuroscience Centric Courses: HMB200H1".
	regexp.MustCompile(`^[\w&,\s]{1,60}\bCentric\s+Courses\s*:`),
	// Timing: "to be taken as early as possible".
	regexp.MustCompile(`(?i)^(?:normally\s+taken\s+in\s+(?:the\s+)?first\s+year|to\s+be\s+taken\s+as\s+early\s+as\s+possible|first\s+offering\s+in\s+\d{4}(?:-\d{2,4})?)$|^should\s+be\s+taken\s+together\s+with\b|^(?:first|second|third|fourth)-year\s+students\s+should\s+take\b`),
	// A course's own prerequisite or audience: "STA130H1 is restricted to
	// first-year students".
	regexp.MustCompile(`^(?:` + grammar.CodePat + `(?:\s*(?:,|and|/)\s*` + grammar.CodePat + `)*)\s+requires?\s|\bis\s+required\s+for\s+all\s+[A-Z]{3}\s+` + grammar.LevelPat + `\+?[\s-]level\s+courses\b|^Students\s+taking\s+` + grammar.CodePat + `\s+must\s+take\s+` + grammar.CodePat + `\s+prior$|\bis\s+restricted\s+to\s+first-year\s+students\b|\bis\s+for\s+students\s+who\s+have\s+(?:previously\s+)?taken\b`),
	// A course exclusion, which enrolment enforces: "please note that this
	// course is an exclusion to EEB125H1".
	regexp.MustCompile(`(?i)\bplease\s+note\s+that\s+(?:these\s+courses\s+are|this\s+course\s+is)\s+(?:an?\s+)?exclusions?\s+to\b`),
	// Descriptions: "MST300H1 is offered through the Centre for Medieval
	// Studies".
	regexp.MustCompile(`^` + grammar.CodePat + `\s+is\s+offered\s+(?:through|by)\b|\bcourses\s+from\s+other\s+campuses\s+are\s+considered\s+courses\s+from\b|^These\s+[A-Z]{3}\s+courses\s+address\b|^` + grammar.CodePat + `\s+is\s+NOT\s+a\s+substitute\s+for\b|^\*?course(?:\(s\))?\s+no\s+longer\s+offered$|^Requirement\s+\d+\s+[^.]*\brepresents\s+a\b|^The\s+updated\s+requirements\s+are\s+for\b|^Courses\s+with\s+codes\s+that\s+do\s+not\s+start\s+with\b[^.]*\bconducted\b[^.]*\bin\s+English\b`),
	vetoDescriptionRe,
	// Degree-level remarks: "You may choose more than 2.5 credits from the
	// Additional Foundation Geosciences list".
	regexp.MustCompile(`\bcredits\s+needed\s+for\s+your\s+degree$|^Note\s+that\b[^.]*\bcan\s+normally\s+be\s+fulfilled\b|^You\s+may\s+choose\s+more\s+than\b`),
	// A header's "including PHL100Y1/ PHL101Y1 if taken": the courses count
	// toward the total like any other, and its pool already holds them.
	regexp.MustCompile(`^including\s+` + grammar.CodePat + `(?:\s*/\s*` + grammar.CodePat + `)*\s+if\s+taken$`),
	// A total restated in brackets after the items: "(3.0 credits in total)".
	regexp.MustCompile(`(?i)^(?:a\s+)?total\s+of\s+` + grammar.NumPat + `\s+credits?$|^` + grammar.NumPat + `\s+credits\s+in\s+total$`),
	asideMustRe,
	// The freedom an elective item leaves: "The 1.5 credits can be chosen to
	// reflect the particular academic interests of each student".
	regexp.MustCompile(`(?i)^the\s+` + grammar.NumPat + `\s+credits?\s+can\s+be\s+chosen\s+to\s+reflect\b`),
	// A descriptor closing a list of alternatives, kept by stripAsides: "a
	// capstone integrative course".
	regexp.MustCompile(`^an?\s+[a-z][a-z\s-]*\bcourse$`),
}

// asideMustRe matches asides whose "must" is a course's own timing or
// prerequisite, or a lead into the items, so the advice veto doesn't apply:
// "Courses must fulfill the following requirements:".
var asideMustRe = regexp.MustCompile(`(?i)^should\s+be\s+taken\s+together\s+with\s+` + grammar.CodePat + `,\s+must\s+be\s+taken\s+prior\s+to\s+completing\s+` + grammar.CodePat + `$|^Students\s+taking\s+` + grammar.CodePat + `\s+must\s+take\s+` + grammar.CodePat + `\s+prior$|^courses\s+must\s+(?:fulfil|fulfill|satisfy|meet)\s+the\s+following\s+requirements:?$`)

// recommendedHeadRe matches a heading that offers what follows as advice:
// "First year (recommended): PHL100Y1/ PHL101Y1".
var recommendedHeadRe = regexp.MustCompile(`(?i)^[^:()]{0,40}\((?:strongly\s+|highly\s+)?recommended\)\s*:`)

// advisoryRes are recurring sentences of advice, explanation or procedure,
// never a requirement. "must" or "required courses" vetoes them unless it is
// procedure or a degree-level rule.
var advisoryRes = []*regexp.Regexp{
	// "Students in this program have the option to complete the Arts & Science
	// Internship Program (ASIP) stream"
	programs.AsipRe,
	// Contacts and websites: "Please contact the Undergraduate Administrator".
	// Not "consultation": an approval in consultation binds.
	regexp.MustCompile(`(?i)\bconsult(?:ing)?\b|\bcontact(?:ing)?\b|\be-?mail\b|\bweb\s*(?:sites?|pages?)\b|https?://|www\.|@utoronto|reach\s+out|\bcheck\s+(?:with|the)\b|\bspeak\s+(?:with|to)\b|\bdiscuss(?:ed)?\s+with\b|\binquire\b|\bvisit\b|\bappointment\b|\badvice\b|\badvising\b|\btimetable\b`),
	// "not an official program requirement"
	regexp.MustCompile(`(?i)\bnot\s+(?:an?\s+)?(?:official\s+)?(?:program\s+|completion\s+)?requirements?\s+(?:of|for)\b|\bnot\s+an?\s+official\b|\bare\s+not\s+(?:completion|program)\s+requirements\b|\bnot\s+(?:a\s+formal|formally\s+a)\s+part\s+of\b`),
	// Entry to programs and what later courses need: "Many 300-level and higher
	// courses in economics require ECO220Y1".
	regexp.MustCompile(`(?i)\beligibility\s+for\s+(?:all\s+)?(?:[\w&]+\s+){0,4}programs?\s+is\s+based\b|^(?:many|most|some)\s+(?:[\w+-]+\s+){0,6}courses\s+(?:[\w]+\s+){0,3}require\b`),
	// Courses described, and what past students found: "Former BCB students
	// benefitted from taking CSB352H1".
	regexp.MustCompile(`(?i)\bis\s+an?\s+(?:accelerated|enriched|advanced)\s+(?:and\s+\w+\s+)?version\s+of\b|\bbenefit(?:ted|ed|s)?\s+from\s+taking\b|\bfound\s+it\s+(?:to\s+be\s+)?helpful\b`),
	// Enrolment in courses, not what counts: "BCB students are allowed to enrol
	// in up to 2.5 CSC credits".
	regexp.MustCompile(`(?i)^(?:[\w-]+\s+){0,3}students\s+(?:are|will\s+be)\s+(?:allowed|permitted)\s+to\s+enrol+\s+in\s+up\s+to\b`),
	// When to take listed courses, a note: "CSC108H1/ CSC148H1/ CSC110Y1 may be
	// taken in First Year".
	regexp.MustCompile(`(?i)^` + grammar.CodePat + `(?:\s*[/,]\s*` + grammar.CodePat + `)*\s+(?:may|can)\s+be\s+taken\s+in\s+(?:the\s+)?(?:first|second|third|fourth)\s+year$|\bmay\s+choose\s+to\s+(?:enrol|enroll|register)\s+in\s+` + grammar.CodePat + `\s+in\s+(?:their|the)\s+(?:first|second|third|fourth)\s+year$`),
	// Pointers: "Refer to the Department website for a full list".
	regexp.MustCompile(`(?i)\brefer\s+to\b|^see\b|\bplease\s+see\b|\bfor\s+(?:more|further|additional|up-to-date)\s+(?:information|details)\b|\bfor\s+a\s+full\s+(?:list|listing)\b`),
	// Prerequisites: "Some of the courses listed below may have prerequisites".
	regexp.MustCompile(`(?i)\b(?:pre|co)-?requisites?\b|\brequired\s+(?:preparation\s+)?for\s+(?:all\s+)?(?:upper|\d00|later)|\bnot\s+all\s+(?:of\s+the\s+)?(?:[\w-]+\s+){0,3}?courses\b`),
	// Offerings: "some may be offered in alternate years".
	regexp.MustCompile(`(?i)\balternate\s+years\b|\boffered\s+(?:every|each)\s+(?:academic\s+)?year\b|\bmay\s+be\s+offered\s+only\b|\bare\s+offered\s+by\s+other\b|\bon\s+a\s+regular\s+basis\b|\bcurrent\s+offerings\b|\bnot\s+guaranteed\b`),
	// Enrolment controls: "Priority is reserved for Drama Majors".
	regexp.MustCompile(`(?i)\benrolment\s+controls\b|\benrolment\s+restrictions?\b|\bmay\s+be\s+restricted\b|\bavailable\s+only\s+to\s+students\b|\bpriority\b|\bplacement\s+(?:tests?|assessments?)\b`),
	// Notices about the program: "a number of spots are available".
	regexp.MustCompile(`(?i)\bwaivers?\b|\bnot\s+required\s+for\s+(?:entrance|entry|admission)\b|\bspots\s+are\s+available\b|\bwork\s+permits?\b|\bshould\s+be\s+(?:completed|taken)\s+(?:during|in|by|before)\b`),
	// Lists described: "This list of eligible courses is not exhaustive".
	regexp.MustCompile(`(?i)\bnot\s+(?:an?\s+)?(?:complete|exhaustive)\s+list\b|\bnot\s+exhaustive\b|\bnot\s+indicative\b`),
	// How transfer credit is matched to a listed course, which it is anyway:
	// "Only transfer credits that carry exclusions to CHM136H1 and/or CHM135H1
	// will be accepted".
	regexp.MustCompile(`(?i)\btransfer\s+credits?\b[^.]*\b(?:exclusions?|equivalency)\b`),
	// "No specific first-year courses required"
	regexp.MustCompile(`(?i)\bno\s+specific\s+(?:first-year\s+)?(?:courses?\s+)?(?:requirements?|required)\b|\bno\s+specific\s+courses\s+are\s+required\b`),
	// The program's description of itself: "This program offers considerable
	// freedom to choose courses".
	regexp.MustCompile(`(?i)^(?:the|this)\s+(?:[\w&,]+\s+){0,6}?(?:program|focus|minor|major|specialist|certificate)\s+(?:offers|combines|provides|is\s+designed|introduces|explores|allows|gives|prepares|emphasi[sz]es|focuses)\b|\bis\s+committed\s+to\b|\bdesigned\s+for\b`),
	// Course-code changes: "Existing "PHS" coded courses remain unchanged".
	regexp.MustCompile(`(?i)\beffective\s+(?:fall|winter|summer|september|january|may)\b|\bremain\s+unchanged\b`),
	// Assessments, syllabi, fees and Faculty rules: "are charged retroactive
	// program fees".
	regexp.MustCompile(`(?i)\baudition\b|\bportfolio\b|\bsyllabus\b|\bare\s+reminded\b|\bFaculty\s+regulations\b|\bretroactive\b|\bprogram\s+fees\b`),
	procedureRe,
}

// ClassifySentence returns the role of one sentence of completion text; the
// first matching case wins. Advice is matched outside brackets and in the first
// ";" clause only, so an item with "(recommended)" stays an item.
func ClassifySentence(s string) SentenceRole {
	t := Clean(s)
	if !wordRe.MatchString(t) || bareMarkerRe.MatchString(t) {
		return RoleAdvisory
	}
	switch {
	case programs.MatchAny(restatedRes, t):
		return RoleAdvisory
	case recommendedHeadRe.MatchString(t) || isComplAdvisory(t, asideRes):
		return RoleAdvisory
	case programs.MatchAny(exclusionRes, t):
		return roleExclusion
	case overlapRe.MatchString(t):
		return roleOverlap
	case crNcrAllowRe.MatchString(t):
		return RoleAdvisory
	case crNcrRe.MatchString(t) && crNcrRuleRe.MatchString(t):
		return roleCrNcr
	case transferRe.MatchString(t) && transferRuleRe.MatchString(t):
		return roleTransfer
	case listLabelRe.MatchString(t) && !fromLabelRe.MatchString(t):
		return RoleListDef
	case isComplAllocLead(t):
		return RoleRequirement
	case countQueryRe.MatchString(t):
		return RoleRequirement
	case isComplAdvisory(t, notBindingRes):
		return RoleAdvisory
	case isComplAdvisory(t, adviceRes) && !programs.MatchAny(substitutionRes, t):
		return RoleAdvisory
	case programs.MatchAny(approvalRes, t) || widenRe.MatchString(t):
		return RoleRequirement
	}
	if r, ok := ruleRole(t); ok {
		return r
	}
	switch {
	case isComplListHead(t):
		return RoleListDef
	case programs.MatchAny(bindingProseRes, t):
		return RoleRequirement
	case isComplAdvisory(t, advisoryRes):
		return RoleAdvisory
	case grammar.CodeRe.MatchString(t) || amountRe.MatchString(t) || filterRe.MatchString(t) ||
		bindingRe.MatchString(t) || isComplHeading(t):
		return RoleRequirement
	}
	return RoleAdvisory
}

// Clean strips list markers, emphasis, note labels, a trailing full stop
// and brackets around the whole sentence.
func Clean(s string) string {
	t := programs.StripEmphasis(nBRe.ReplaceAllString(s, "NB: "))
	for {
		u := strings.TrimSpace(leadRe.ReplaceAllString(t, ""))
		u = strings.TrimSpace(strings.TrimSuffix(u, "."))
		if wrapped(u) {
			u = strings.TrimSpace(u[1 : len(u)-1])
		}
		if u == t {
			return t
		}
		t = u
	}
}

// wrapped reports whether u is in one pair of brackets: "[ PHY131H1,
// PHY132H1 recommended]".
func wrapped(u string) bool {
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

// stripParens removes bracketed text, innermost first.
func stripParens(t string) string {
	for {
		u := parenRe.ReplaceAllString(t, "")
		if u == t {
			return t
		}
		t = u
	}
}

// isComplHeading reports a heading or a lead-in ending with a colon
// ("Courses accepted towards this program's ethics requirement:").
func isComplHeading(t string) bool {
	return strings.HasSuffix(t, ":") || headingRe.MatchString(t) && !headingVerbRe.MatchString(t)
}

// isComplListHead reports a list label followed by codes, with no verb after
// the colon.
func isComplListHead(t string) bool {
	m := listHeadRe.FindStringSubmatchIndex(t)
	if m == nil || notListHeadRe.MatchString(t[m[2]:m[3]]) {
		return false
	}
	return !leadVerbRe.MatchString(parenRe.ReplaceAllString(t[m[3]:], ""))
}

// isComplAllocLead reports a sentence that leads with an amount and has no
// verb in its first clause.
func isComplAllocLead(t string) bool {
	loc := allocLeadRe.FindStringIndex(t)
	if loc == nil {
		return false
	}
	if recommendRe.MatchString(t[:loc[1]]) || leadCueRe.MatchString(t[:loc[1]]) {
		return false // "Recommended: 0.5 credit from any HPS 100-level courses"
	}
	rest := t[loc[1]:]
	if takenFromRe.MatchString(rest) {
		return true
	}
	if i := clauseEndRe.FindStringIndex(rest); i != nil {
		rest = rest[:i[0]]
	}
	if i := relativeRe.FindStringIndex(rest); i != nil && strings.ContainsAny(t[:loc[1]], "0123456789") {
		rest = rest[:i[0]]
	}
	return !leadVerbRe.MatchString(rest)
}

// ruleRole returns constraint or limit for the earliest cue of either
// kind, and false when there is none.
func ruleRole(t string) (SentenceRole, bool) {
	best, role := -1, RoleRequirement
	codeLead := codeLeadRe.MatchString(t)
	if !codeLead {
		for _, re := range constraintRes {
			if loc := re.FindStringIndex(t); loc != nil && (best < 0 || loc[0] < best) {
				best, role = loc[0], roleConstraint
			}
		}
	}
	// a code-led item's caps in brackets are its own
	lt := t
	if codeLead {
		lt = stripParens(t)
	}
	for _, re := range limitRes {
		loc := re.FindStringIndex(lt)
		if loc == nil || best >= 0 && loc[0] >= best {
			continue
		}
		// a code-led sentence allocating an amount before the cap is an item
		if codeLead && amountRe.MatchString(lt[:loc[0]]) {
			continue
		}
		best, role = loc[0], roleLimit
	}
	if best < 0 && !codeLead && scopeRe.MatchString(t) && !isComplAdvisory(t, advisoryRes) {
		return roleConstraint, true // "Of these:", unless it is advice
	}
	return role, best >= 0
}

// isComplAdvisory matches res outside brackets, in the first ";" clause, with
// attributive "recommended"/"suggested" removed. The advice veto applies
// unless the "must" is procedure or a degree-level rule.
func isComplAdvisory(t string, res []*regexp.Regexp) bool {
	if adviceVetoRe.MatchString(t) && !procedureRe.MatchString(t) && !degreeRuleRe.MatchString(t) &&
		!vetoDescriptionRe.MatchString(t) && !asideMustRe.MatchString(t) {
		return false
	}
	if recommendedTagRe.MatchString(t) && !grammar.CodeRe.MatchString(t) {
		return true
	}
	bare := stripParens(timingNoteRe.ReplaceAllString(t, ""))
	bare = attributiveAdviceRe.ReplaceAllString(bare, "")
	// advice in a later ";" clause doesn't make the sentence advice
	bare = grammar.SplitTop(bare, ';')[0]
	return programs.MatchAny(res, bare)
}

// SplitSentences splits a line of completion text into sentences as
// SplitSentences does, and also before a sentence that starts with an amount
// and at inline lettered notes. A list marker stays with the first sentence.
func SplitSentences(s string) []string {
	s = nBRe.ReplaceAllString(strings.TrimSpace(s), "NB: ")
	s = programs.StripEmphasis(italicNoteRe.ReplaceAllString(s, "$1. $2"))
	marker := programs.MarkerRe.FindString(s)
	s = s[len(marker):]
	if wrapped(s) && strings.Contains(s, ". ") {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	out := splitAtAmounts(s)
	if marker != "" && len(out) > 0 {
		out[0] = marker + out[0]
	}
	var kept []string
	for _, sent := range splitLetterNotes(out) {
		if sent != "" {
			kept = append(kept, sent)
		}
	}
	return kept
}

// splitAtAmounts splits s with SplitSentences, and also, outside brackets,
// before a sentence that starts with an amount, a level or a bracketed code.
func splitAtAmounts(s string) []string {
	parens := grammar.TopParens(s)
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
	for _, m := range amountStartRe.FindAllStringSubmatchIndex(s, -1) {
		dot := m[3]
		if grammar.Abbreviations[strings.ToLower(s[m[2]:m[3]])] || inParens(dot) {
			continue
		}
		out = append(out, grammar.SplitSentences(s[start:dot])...)
		start = dot + 1
	}
	return append(out, grammar.SplitSentences(s[start:])...)
}

// splitLetterNotes splits each sentence that holds inline lettered notes
// ("(a) ...; (b) ...") into one sentence per letter.
func splitLetterNotes(sentences []string) []string {
	var out []string
	for _, sent := range sentences {
		if !strings.Contains(sent, "(a) ") {
			out = append(out, sent)
			continue
		}
		from := 0
		for _, loc := range letterNoteRe.FindAllStringIndex(sent, -1) {
			out = append(out, strings.TrimSpace(sent[from:loc[0]]))
			from = loc[0] + 1
		}
		out = append(out, strings.TrimSpace(sent[from:]))
	}
	return out
}

// ---- Text kinds ----

// contentRes are clauses saying what a course's content, topic, type or
// language decides, which the catalogue doesn't record. contentRest removes
// each (or puts repl in its place); the rest is what a checker can read.
var contentRes = []struct {
	re   *regexp.Regexp
	repl string
}{
	// Language of instruction: "No more than 2.0 credits taught in English".
	{regexp.MustCompile(`\s*\b(?:(?:from\s+)?courses\s+)?(?:taught|taken)\s+in\s+(?:English|French|Spanish|Portuguese|German|Italian)\b`), ""},
	// "where the primary language of study is not English", with any floor on
	// those courses after it
	{regexp.MustCompile(`\s*\bwhere\s+the\s+primary\s+language\s+of\s+study\s+is\s+not\s+English\b(?:,\s*including\s+` + grammar.AtLeastPat + `\s+` + grammar.NumPat + `\s+credits?\s+at\s+the\s+\d00\+?[\s-]level)?`), ""},
	// "1.0 credit at the 100-level or above in a language other than English"
	{regexp.MustCompile(`,?\s*\b(?:(?:that\s+)?(?:must\s+)?involve\s+reading\s+texts\s+)?in\s+a\s+language\s+other\s+than\s+English\b`), ""},
	// Course types: "PRT language courses", "Language courses cannot be used",
	// "2.0 credits in Hungarian language study".
	{regexp.MustCompile(`\b([A-Z]{3})\s+language\s+(courses)\b`), "$1 $2"},
	{regexp.MustCompile(`^Language\s+(courses\s+cannot\s+be\s+used)\b`), "$1"},
	{regexp.MustCompile(`\b[A-Z][a-z]+\s+language\s+study\b|\bconsecutive\s+language\s+courses\b|\bin\s+languages\s+or\s+literatures\b`), ""},
	// "2.5 first year Science credits": first year is the 100 level, which a
	// checker reads; which courses are Science is content.
	{regexp.MustCompile(`\bfirst\s+year\s+(?:Social\s+)?Science\s+credits\b`), "100-level credits"},
	{regexp.MustCompile(`,\s*to\s+include\b.*\bCreative\s+Writing\s+courses\b.*$`), ""},
	{regexp.MustCompile(`\b(?:Literature\s+and\s+Language|Creative\s+Writing)\s+courses\b`), ""},
	// Content, theme and experience: "0.5 credit in global experience",
	// "Industry-engaged version of CSC207H1: Software Design", 300-level
	// "Program-Only" courses ( SOC317H1 – SOC354H1; ...).
	{regexp.MustCompile(`\b(?:global|international)\s+experience\b|\b[Ss]ome\s+offerings\s+of\b|\bdiasporic\s+communities\b|\bapplication\s+of\s+digital\s+technologies\b|\bfocus(?:ed)?\s+on\s+a\s+(?:particular\s+)?(?:global\s+region|theme)\b|\bcomplementary\s+courses\b[^.]*\boffered\s+(?:in|by)\s+other\s+programs\b`), ""},
	{regexp.MustCompile(`^Industry-engaged\s+version\s+of\s+(` + grammar.CodePat + `)(?::.*)?$`), "$1"},
	{regexp.MustCompile(`["“”]Program-Only["“”]\s*|\s*\(\s*` + grammar.CodePat + `\s*[–-]\s*` + grammar.CodePat + `;[^()]*\)`), ""},
	// Lists kept elsewhere: "5.0 credits in any of the four areas of Jewish
	// Studies".
	{regexp.MustCompile(`;\s*fulfilled\s+by\b[^;]*\bHandbook\b|\bUndergraduate\s+Course\s+Handbook\b|\bfour\s+areas\s+of\s+Jewish\s+Studies\b|^offered\s+by\s+the\s+[\w\s]+\s+program$`), ""},
}

// contentRest reports whether t says what a course's content, topic, type or
// language decides (contentRes), and returns t without those clauses.
func contentRest(t string) (string, bool) {
	found := false
	for _, c := range contentRes {
		if c.re.MatchString(t) {
			found = true
			t = c.re.ReplaceAllString(t, c.repl)
		}
	}
	return strings.TrimSpace(t), found
}

// contentFilterRe matches what a checker could read besides a bare amount:
// a course, a level, a designator or a named list such as "Group A".
var contentFilterRe = regexp.MustCompile(grammar.CodePat + `|\b` + grammar.LevelPat + `(?:\+|\s*-?\s*(?:level|series))|\b[A-Z]{3}\b|\b(?:Groups?|Lists?|Categor(?:y|ies))\s+[A-Z0-9#]`)

// textKinds sets the kind of texts left as other: content, permission or
// placement, never hiding a rule a checker could read. It runs after every pass
// that looks for kind other, before the Review markers take their causes' kind.
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

// listBelowContent marks as content the texts of n that draw on "the list
// below" in a program with no lists: the department's list, which the calendar
// doesn't give, decides.
func listBelowContent(n *types.ReqNode) {
	grammar.WalkReq(n, func(x *types.ReqNode) {
		if x.Type == types.NodeText && x.Kind == types.TextOther && fromListBelowRe.MatchString(x.Text) {
			x.Kind = types.TextContent
		}
	})
}

// listBelowGiven reports whether a course code follows r's first line in raw:
// then the calendar gives the list, though it was kept as notes (ASMIN1210).
func listBelowGiven(raw string, r *types.Requirement) bool {
	head, _, _ := strings.Cut(r.Raw, "\n")
	i := strings.Index(raw, head)
	return head != "" && i >= 0 && grammar.CodeRe.MatchString(raw[i+len(head):])
}

// wholeTextKind marks a row's text as content when its rest reads as a pool,
// giving and(pool, text), or names no course or filter. A code range within the
// pool's filters becomes the pool itself: "( SOC317H1 – SOC354H1; ...)".
func wholeTextKind(n *types.ReqNode) *types.ReqNode {
	if n.Kind != types.TextOther && n.Kind != types.TextMembership {
		return n
	}
	rest, ok := contentRest(Clean(n.Text))
	if !ok {
		return n
	}
	if pool := contentPool(rest); pool != nil {
		if m := codeRangeRe.FindStringSubmatch(n.Text); m != nil && pool.Type == types.NodeCredits && pool.Amount > 0 && pool.Courses == nil {
			if codes := codeRange(m[1], m[2]); codes != nil && !slices.ContainsFunc(codes, func(c string) bool { return !grammar.FilterMatches(pool, c) }) {
				return &types.ReqNode{Type: types.NodeCredits, Amount: pool.Amount, Courses: codes}
			}
		}
		n.Kind = types.TextContent
		return programs.AndNode(pool, n)
	}
	if !contentFilterRe.MatchString(rest) {
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
// when the codes a text names are beside it, permission for an approval
// alternative beside a strict branch, and placement.
func partTextKinds(n *types.ReqNode) {
	if n == nil {
		return
	}
	codes := programs.NodeCodes(n)
	strict := n.Type == types.NodeOr && slices.ContainsFunc(n.Items, func(x *types.ReqNode) bool { return x.Type != types.NodeText })
	for i, it := range n.Items {
		if it.Type != types.NodeText {
			partTextKinds(it)
			continue
		}
		t := Clean(it.Text)
		named := !slices.ContainsFunc(grammar.CodeRe.FindAllString(t, -1), func(c string) bool { return !slices.Contains(codes, c) })
		switch {
		case it.Kind != types.TextOther && it.Kind != types.TextMembership:
		case isComplApprovalText(t):
			// a person decides, whatever the content; with no strict branch
			// beside it, it is the requirement itself and stays other
			if strict && it.Kind == types.TextOther {
				it.Kind = types.TextPermission
			}
		case placementTextRe.MatchString(t):
			it.Kind = types.TextPlacement
		case strict && anyOtherApprovedRe.MatchString(t):
			it.Kind = types.TextPermission
		case named && contentText(t):
			it.Kind = types.TextContent
			// an and's part gets its pool beside it, as a whole row does
			pooled := slices.ContainsFunc(n.Items, func(x *types.ReqNode) bool { return x.Type == types.NodeCredits })
			if rest, _ := contentRest(t); n.Type == types.NodeAnd && !pooled {
				if pool := contentPool(rest); pool != nil && pool.Type == types.NodeCredits {
					n.Items[i] = programs.AndNode(pool, it)
				}
			}
		}
	}
}

var (
	// placementTextRe matches what a placement or proof of level decides:
	// "Students whose placement assessment exempts them".
	placementTextRe = regexp.MustCompile(`(?i)\bplacement\s+(?:assessment|test)\s+exempts?\b|^proof\s+of\s+(?:comparable|equivalent)\s+[A-Z][a-z]+\s+(?:level|proficiency)$`)
	// anyOtherApprovedRe matches an approval beside the listed courses:
	// "any other approved course".
	anyOtherApprovedRe = regexp.MustCompile(`(?i)^(?:or\s+)?any\s+other\s+approved\s+courses?\b`)
)

// contentText reports a content sentence (contentRes).
func contentText(t string) bool {
	_, ok := contentRest(t)
	return ok
}

// consultRe matches approval by consultation: "selected in consultation
// with the Undergraduate Coordinator".
var consultRe = regexp.MustCompile(`(?i)\bin\s+(?:joint\s+)?consultation\s+with\b|\bnomination\s+process\b|\bpending\s+approval\b|\bpre-?approved\b|\bapproved\s+by\s+the\b`)

// isComplApprovalText reports a sentence in which someone approves what counts.
func isComplApprovalText(t string) bool {
	return programs.MatchAny(approvalRes, t) || countQueryRe.MatchString(t) || consultRe.MatchString(t)
}

// someOfferingsRe matches courses that count in some offerings only: "Some
// offerings of HIS357H1 and HIS389H1 may also count towards this focus".
var someOfferingsRe = regexp.MustCompile(`^[Ss]ome\s+offerings\s+of\s+(` + grammar.CodePat + `(?:(?:,\s*|,?\s+and\s+)` + grammar.CodePat + `)*)\s+may\s+also\s+count\b[^.]*$`)

// someOfferings bounds a pool's "some offerings of X may also count"
// alternative: or(pool, text) becomes or(pool, and(credits{amount, pool ∪ X},
// text{content})).
func someOfferings(n *types.ReqNode) {
	if n.Type != types.NodeOr || len(n.Items) < 2 || n.Items[0].Type != types.NodeCredits {
		return
	}
	pool := n.Items[0]
	if pool.Amount <= 0 || pool.Courses == nil || pool.Designators != nil || pool.Levels != nil || pool.Lists != nil {
		return
	}
	for i, it := range n.Items[1:] {
		m := someOfferingsRe.FindStringSubmatch(Clean(it.Text))
		if it.Type != types.NodeText || it.Kind != types.TextOther || m == nil {
			continue
		}
		it.Kind = types.TextContent
		wide := &types.ReqNode{Type: types.NodeCredits, Amount: pool.Amount, Courses: grammar.Dedupe(append(slices.Clone(pool.Courses), grammar.CodeRe.FindAllString(m[1], -1)...))}
		n.Items[i+1] = programs.AndNode(wide, it)
	}
}

// egLevelRe matches a route that counts a course at a level: "Students may
// be permitted to take Math graduate courses. A 0.5 credit of these courses may
// count ... (e.g., as courses "at the 400-level")".
var egLevelRe = regexp.MustCompile(`(?i)\bmay\s+be\s+permitted\b.*\bA\s+0\.5\s+credit\s+of\s+these\s+courses\s+may\s+count\b.*\be\.g\.,?\s+as\s+courses\s+["“]at\s+the\s+\d00-level`)

// egLevelRoute bounds the e.g. level route on a floor: or(floor, text) becomes
// or(floor, and(floor less 0.5, text{permission})), as only 0.5 credit may
// count at the level; with nothing left, or(floor, text{permission}).
func egLevelRoute(r *types.Requirement) {
	n := r.Req
	if r.Kind != types.RequirementConstraint || n.Type != types.NodeOr || len(n.Items) != 2 {
		return
	}
	floor, text := n.Items[0], n.Items[1]
	if floor.Type != types.NodeCredits || floor.Levels == nil || floor.Amount <= 0 || text.Type != types.NodeText ||
		text.Kind != types.TextOther || !egLevelRe.MatchString(text.Text) {
		return
	}
	text.Kind = types.TextPermission
	if rest := floor.Amount - 0.5; rest > 1e-9 {
		less := *floor
		less.Amount = rest
		less.Levels = slices.Clone(floor.Levels)
		n.Items[1] = programs.AndNode(&less, text)
	}
}

// IsApproval reports whether s is an approval route: "Other introductory
// language classes may be approved by the Director".
func (a *Assembler) IsApproval(s string) bool {
	return a.cue(s).approval
}

// alternativeRe matches a sentence offering another way to meet a
// requirement: "CSB450H1 is also acceptable".
var alternativeRe = regexp.MustCompile(`(?i)\balso\s+(?:be\s+)?(?:used|count(?:s|ed)?|accepted|acceptable|eligible|qualif\w*|fulfil\w*|satisf\w*|meet|complete)\b|\b(?:is|are)\s+also\s+acceptable\b|\bsubstitute\b|\bqualif(?:y|ies)\b|\b(?:meet|fulfil+|satisfy)\s+(?:this|the)\s+(?:[\w-]+\s+){0,3}requirement\b|\bcounts?\s+towards?\b|\bbe\s+counted\s+towards?\b|\bin\s+lieu\b|\b(?:can|may)\s+be\s+used\b|\bwill\s+be\s+accepted\b|\bacceptable\b`)

// IsAlternative reports whether s offers another way to meet a requirement,
// or widens its pool with a cap.
func (a *Assembler) IsAlternative(s string) bool {
	return a.cue(s).alternative
}

// SentenceCues is what the assembler asks about a sentence besides its role.
type SentenceCues struct{ approval, alternative bool }

// cue reads s's cues once.
func (a *Assembler) cue(s string) SentenceCues {
	c, ok := a.Cues[s]
	if !ok {
		t := a.clean(s)
		c.approval = programs.MatchAny(approvalRes, t) || countQueryRe.MatchString(t)
		c.alternative = !negationRe.MatchString(notListedRe.ReplaceAllString(t, "")) && alternativeRe.MatchString(t) || widenRe.MatchString(t)
		a.Cues[s] = c
	}
	return c
}

// negationRe matches a sentence that refuses an alternative: "cannot".
var negationRe = regexp.MustCompile(`(?i)\bnot\b|\bcannot\b|n['’]t\b`)

// restatedRes match sentences that restate what the rows say, or are
// advice, though a binding word or a code is in them. Unlike asideRes,
// "must" doesn't veto them: each is one wording.
var restatedRes = []*regexp.Regexp{
	// a course's overlap with others, which their exclusions encode: "MGT100H1
	// have considerable overlap with RSM100H1"
	regexp.MustCompile(`^` + grammar.CodePat + `(?:,\s*` + grammar.CodePat + `)*\s+have\s+considerable\s+overlap\s+with\b`),
	// the claims rule restated: "The course taken in lieu of STA130H1 cannot be
	// one that is already being used to satisfy Requirement 9"
	regexp.MustCompile(`(?i)\b(?:taken\s+in\s+lieu\s+of|used\s+to\s+replace)\s+` + grammar.CodePat + `\s+cannot\s+be\s+(?:one|a\s+course)\s+that\s+is\s+already\s+being\s+used\s+to\s+(?:satisfy|meet)\b|^students\s+admitted\s+to\s+the\s+program\s+with\s+[^:]+:\s+the\s+course\s+will\s+be\s+applied\s+to\s+requirement\b`),
	// a pointer to the note an alternative comes from: "If you are unable to
	// complete STA130H1 in first year, see Note A below for ..."
	regexp.MustCompile(`(?i)^if\s+you\s+are\s+unable\s+to\s+complete\s+` + grammar.CodePat + `\b[^.]*,\s+see\s+note\s+[A-Z]\s+below\s+for\b`),
	// a course's prerequisites, which the list leaves out: "(note that both
	// require BIO130H1 and BIO230H1/ BIO255H1)"
	regexp.MustCompile(`(?i)\bprerequisites\s+for\s+these\s+courses\s+cannot\s+be\s+used\s+towards\b|^note\s+that\s+(?:both|these(?:\s+courses)?|this\s+course|they|it)\s+requires?\s|^` + grammar.CodePat + `\s+may\s+be\s+taken\s+without\s+` + grammar.CodePat + `\s+for\s+students\s+who\b`),
	// amounts the rows encode: "Sub-total = 8.5 credits"
	regexp.MustCompile(`(?i)^if\s+taking\s+` + grammar.CodePat + `,\s+the\s+extra\s+` + grammar.NumPat + `\s+credit\s+can\s+go\s+to\b|^or\s+any\s+of\s+the\s+following:\s*` + grammar.CodePat + `(?:\s*[/,]\s*` + grammar.CodePat + `)*$|^sub-?total\s*=\s*` + grammar.NumPat + `(?:\s+or\s+` + grammar.NumPat + `)?\s+credits?(?:\s*\([^)]*\))?$|^in\s+addition\s+to\s+the\s+above\s+core\s+courses,\s+you\s+need\s+to\s+take\s+` + grammar.NumPat + `\s+credits?\s+of\s+electives$|^plus\s+a\s+disciplinary\s+focus\s+requiring\s+` + grammar.NumPat + `\s*-\s*` + grammar.NumPat + `\s+credits$`),
	// a focus's own requirements, which focusChoices reads: "Health Studies
	// (ASFOC2270A): (2.0 credits) HMB342H1, ..."
	regexp.MustCompile(`(?i)^[^:()]{1,80}\((?-i:[A-Z]{2}FOC\d{4}[A-Z])\):\s*\(` + grammar.NumPat + `(?:\s+or\s+` + grammar.NumPat + `)?\s+credits\)`),
	// a designator definition, which aliasDesignators applies
	aliasPrefixesRe,
	// enrolment in courses, not what counts: "... may request to enroll in the
	// following RSM courses"
	regexp.MustCompile(`(?i)\bmay\s+request\s+to\s+enrol+\s+in\s+the\s+following\b`),
}
