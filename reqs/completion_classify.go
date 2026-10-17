package reqs

import (
	"regexp"
	"slices"
	"strings"

	"degree-explorer/types"
)

// sentenceRole says what one sentence of program completion text does, read
// alone; the assembler combines it with the kind of block the sentence is in.
// Cue words beat block context, since a "Notes:" block binds in most programs.
type sentenceRole int

const (
	roleRequirement sentenceRole = iota // anything binding without a more specific role
	roleConstraint                      // a floor or a must-include on courses counted elsewhere
	roleLimit                           // "no more than": at most an amount counts, the excess is dropped
	roleListDef                         // a named course list: "Group A: ..."
	roleAdvisory                        // advice, contacts, procedure and timing: notes
	roleExclusion                       // programs that can't be combined with this one
	roleTransfer                        // a cap or ban on transfer credit
	roleCrNcr                           // a rule on courses taken CR/NCR
	roleOverlap                         // a course may count toward two named requirements
)

const complAmountPat = numPat + `(?:\s*[-–]\s*` + numPat + `)?`

var (
	// complLeadRe matches list markers, footnote stars and note labels before a
	// sentence: "Note 2:".
	complLeadRe = regexp.MustCompile(`^(?:[-•*]+\s*|\(?(?:\d{1,2}|[a-zA-Z]|[ivx]{1,4})[.)]\s+|(?i:notes?(?:\s+\d)?|nb|important)\s*:\s*)+`)
	// complNBRe matches "N.B.", rewritten as a label so it doesn't end a
	// sentence.
	complNBRe = regexp.MustCompile(`\bN\.\s?B\.\s*`)
	// complAmountStartRe matches a full stop before a sentence starting with an
	// amount, a level or a bracketed code: "courses.1.0 credit". Group 1 is the
	// word before it; splitSentences only splits before an uppercase letter.
	complAmountStartRe = regexp.MustCompile(`((?:[a-z]+\.)*[a-z]+|\)|` + codePat + `)\.(?:\s+\d+\.\d+\s|\d+\.\d+\s+(?:[A-Z]{3}\s+)?credits?\b|\s+\(\s*` + codePat + `|\s+` + levelPat + `(?:/` + levelPat + `)*(?:\+|-|\s+level))`)
	// complLetterNoteRe matches the start of each inline lettered note after the
	// first: "; (b) ".
	complLetterNoteRe = regexp.MustCompile(`;\s*\([b-z]\)\s`)
	// complItalicNoteRe matches an italic note glued to an item with no full
	// stop: "CHM151Y1 _Transfer credits".
	complItalicNoteRe = regexp.MustCompile(`(` + codePat + `|\))\s+(?:\*\*)?_(?:\*\*)?([A-Z])`)
	// complParenRe matches bracketed text, removed before the advice checks so
	// that "( CHM151Y1 strongly recommended)" doesn't make its item advice.
	complParenRe = regexp.MustCompile(`\([^()]*\)|\[[^\[\]]*\]`)
	// complRecommendedTagRe matches advice in brackets: "(recommended)". With
	// no code in the sentence, it marks a heading, not an item's annotation.
	complRecommendedTagRe = regexp.MustCompile(`(?i)\((?:strongly\s+|highly\s+)?recommended\)`)
	// complAttributiveAdviceRe matches "recommended" naming a set of courses:
	// "the recommended electives". It is removed so it can't silence a cap.
	complAttributiveAdviceRe = regexp.MustCompile(`(?i)\b(?:the|these|those|of|from)\s+(?:list\s+of\s+)?(?:recommended|suggested)\s+(?:[\w-]+\s+)?(?:courses|lists?|electives)\b`)
	// complWordRe matches a letter or digit; a sentence with none says nothing.
	complWordRe = regexp.MustCompile(`[A-Za-z0-9]`)
	// complBareMarkerRe matches a list marker left alone after cleaning: "a)".
	complBareMarkerRe = regexp.MustCompile(`^(?:\d{1,2}|[a-zA-Z]|[ivxIVX]{1,4})[.)]?$`)

	complCodeLeadRe = regexp.MustCompile(`^(?i:in\s+addition,?\s+)?[(\s]*` + codePat)
	complAmountRe   = regexp.MustCompile(`\b\d+\.\d+\b|\b\d+\s+(?:full|half)[-\s]courses?\b|(?i:\b(?:one|two|three|four)\s+years?\s+of\s+study\b)`)
	// complFilterRe matches a filter a requirement can carry: "300-level".
	complFilterRe = regexp.MustCompile(`\b` + levelPat + `(?:\+|\s*-?\s*(?:level|series)|\s*-?\s*/)|\bbreadth\b|\bBR\s*=|\b[A-Z]{3}(?:\s*/\s*[A-Z]{3})*\s*-?\s*(?:courses?|credits?|prefix|designators?|coded)\b|\b(?:Groups?|Lists?|Clusters?)\s+[A-Z0-9]\b`)
	// complBindingRe matches a verb that binds even without a code or an
	// amount: "must".
	complBindingRe = regexp.MustCompile(`(?i)\bmust\b|\brequire[sd]?\b|\bcannot\b|\bmay\s+not\b|\bwill\s+not\b|\bnot\s+(?:be\s+)?(?:accepted|counted|permitted|eligible|included)\b|\bonly\b|\bno\s+(?:more|other|course|courses|credits?)\b|\bwill\s+count\b`)
	// complHeadingRe matches short text with no full stop; with no verb from
	// complHeadingVerbRe it is a heading: "Core Courses". It defaults to
	// requirement on purpose: a heading taken for advice drops the rows under it.
	complHeadingRe     = regexp.MustCompile(`^[^.]{1,100}$`)
	complHeadingVerbRe = regexp.MustCompile(`(?i)\b(?:is|are|was|were|will|may|might|can|should|must|has|have|need|needs)\b`)

	// complCombineRe matches programs that can't be combined, beyond the
	// enrolment phrasings: "cannot also complete". "may not be combined with"
	// never occurs but stays as enrolment's cannot-combine phrasing.
	complCombineRe    = regexp.MustCompile(`(?i)\bmay\s+not\s+be\s+combined\s+with\b|\bcannot\s+be\s+paired\s+with\b|\bmay\s+(?:only\s+)?enrol(?:l)?\s+in\s+only\s+one\b|\bnot\s+(?:be\s+)?allowed\s+to\s+enrol(?:l)?\s+concurrently\b|\bcannot\s+also\s+complete\b|\bcannot\s+be\s+enrol(?:l)?ed\s+in\s+any\s+other\s+program\b|\bmay\s+not\s+simultaneously\s+(?:be\s+)?(?:registered|enrol(?:l)?(?:ed)?)\s+(?:for|in)\b`)
	complExclusionRes = []*regexp.Regexp{complCombineRe, enrolCombinedWithRe, enrolCombineRe, enrolSimultaneousRe, enrolNotOpenRe}

	// complOverlapRe matches a course counting toward two named requirements:
	// "may contribute to both". "may also count toward" is always membership,
	// not sharing.
	complOverlapRe = regexp.MustCompile(`(?i)\b(?:may|can)\s+(?:also\s+)?(?:contribute|count|be\s+(?:used|counted|applied))\s+(?:to(?:wards?)?|for|to\s+satisfy)\s+both\b`)

	complCrNcrRe = regexp.MustCompile(`(?i)\bCR\s*/\s*NCR\b|\bcredit\s*/\s*no\s+credit\b`)
	// complCrNcrRuleRe matches what makes a CR/NCR mention a rule rather than a
	// pointer to details: "cannot".
	complCrNcrRuleRe = regexp.MustCompile(`(?i)\bcannot\b|\bmay\s+not\b|\bnot\s+(?:be\s+)?accepted\b|\bdo\s+not\s+accept\b|\bmay\s+(?:be\s+)?(?:use|used|accepted|taken)\b|\bcan\s+be\s+taken\b|\bonly\b|\bno\s+courses\b`)

	// complCrNcrAllowRe matches a CR/NCR permission with no ban, which is the
	// default: "Students may use the CR/NCR option towards any of the courses".
	complCrNcrAllowRe = regexp.MustCompile(`(?i)^students\s+may\s+use\s+(?:the\s+)?CR\s*/\s*NCR\s+(?:option\s+)?(?:on|towards?|for)\s+(?:the|any)\b[^;]*$`)

	complTransferRe = regexp.MustCompile(`(?i)\btransfer\s+credits?\b|\bin\s+transfer\s+credit\b`)
	// complTransferRuleRe matches a cap or ban on transfer credit: "no more
	// than".
	complTransferRuleRe = regexp.MustCompile(`(?i)\b` + noMoreThanPat + `\b|\bmaximum\b|\bmore\s+than\s+\d|\bup\s+to\b|\bcannot\b|\bmay\s+not\b|\bnot\s+(?:be\s+)?accepted\b|\bwill\s+not\b`)

	// complListLabelRe matches a list label, which names a list even with a cap
	// in it: "Group A:".
	complListLabelRe = regexp.MustCompile(`^(?:[\w/&-]+\s+){0,2}(?:Group|List|Cluster|Category|Sub-?group|Area)\s+(?:[A-Z]|\d{1,2}|[IVX]{1,4}|[a-z]\))(?:\s*$|\s*[-–:(])`)
	// complFromLabelRe matches a label that draws on a list rather than naming
	// one: "From Group A: 1.0 credit".
	complFromLabelRe = regexp.MustCompile(`(?i)^(?:from|in|of)\b`)
	// complListHeadRe matches a label followed by codes: "Hydrology/GIS/Remote
	// Sensing: GGR273H1". complNotListHeadRe rejects a section or a choice,
	// which are items: "First Year".
	complListHeadRe    = regexp.MustCompile(`^([^:.]{1,120}):\s*\(?\s*` + codePat)
	complNotListHeadRe = regexp.MustCompile(`(?i)\byears?\b|\b(?:all|one|two|three|four|any|\d)\b(?:\s+\w+)?\s+(?:of|from)\b|` + codePat)

	// complAllocLeadRe matches a sentence that leads with an amount, after an
	// optional label: "2.0 credits from Groups A-C". A floor or a cap in a
	// later clause belongs to that allocation; the item grammar splits it off.
	complAllocLeadRe = regexp.MustCompile(`(?i)^(?:[^:.()]{1,60}:\s*|[^:.()]{1,40}\(\s*)?` +
		`(?:\(\s*|in\s+addition,?\s+|any\s+|all\s+|` + atLeastPat + `\s+|(?:an\s+)?additional\s+|a\s+total\s+of\s+|a\s+minimum\s+of\s+|complete\s+|` +
		`students\s+(?:must\s+)?complete\s+(?:a\s+total\s+of\s+)?|the\s+(?:[\w&]+\s+){1,3}requires\s+)*` +
		`(?:(?:a\s+|one\s+)?` + complAmountPat + `\s+(?:\(\s*` + numPat + `\s+credits?\s*\)\s+)?(?:[\w/+-]+\s+){0,3}?(?:credits?|(?:half|full)[-\s]courses?|courses?)\b` +
		`|total(?:\s+credits)?\s*:\s*` + complAmountPat +
		`|[^.;:]{0,80}?\b(?:to|for)\s+a\s+total\s+of\b` +
		`|(?:an?\s+)?additional\s+(?:[\w/+-]+\s+){0,3}?(?:credits?|courses?)\b` +
		`|select\s+the\s+remaining\s+credits\b)`)
	// complClauseEndRe matches the end of an amount lead's first clause:
	// "including".
	complClauseEndRe = regexp.MustCompile(`(?i)[,;:(]|\b(?:of\s+which|including|with|to\s+include)\b`)
	// complRelativeRe matches a relative clause, which ends the first clause
	// only after a numeric amount: "2.0 credits in courses where the primary
	// language".
	complRelativeRe = regexp.MustCompile(`(?i)\b(?:where|that|which|who|whose)\b`)
	// complLeadCueRe matches a cue in the lead that makes it a rule, not an
	// allocation: "No more than".
	complLeadCueRe = regexp.MustCompile(`(?i)\b(?:no\s+more|at\s+most|maximum|only|up\s+to|may|can|cannot)\b`)
	// complTakenFromRe matches an allocation's codes after the lead: "1.0
	// credit must be taken from GER270H1". "must be taken at the 300-level" is
	// a floor.
	complTakenFromRe = regexp.MustCompile(`(?i)^\s*must\s+be\s+(?:taken|chosen|selected)\s+from\s+(?:the\s+following:?\s*)?` + codePat)
	// complLeadVerbRe matches a verb in the first clause, which makes the
	// amount a subject rather than an allocation: "1.0 credit must be at the
	// 300-level".
	complLeadVerbRe = regexp.MustCompile(`(?i)\b(?:must|should|needs?\s+to|can|cannot|may|will|is|are|counts?)\b`)

	// complConstraintRes are cues for a floor or a must-include on courses
	// counted under another requirement. A bare "at least" is a constraint only
	// by block context, so here it needs a modal.
	complConstraintRes = []*regexp.Regexp{
		// "These 3.0 credits must include:"
		regexp.MustCompile(`(?i)\bmust\s+include\b`),
		// "Higher Years (must include ...), including at least 0.5 at ..."
		regexp.MustCompile(`(?i)\bincluding\s+` + atLeastPat + `\b`),
		// "At least 2.0 credits at the 300+ level, 0.5 of which must be ..."
		regexp.MustCompile(`(?i)(?:^|,|\b(?:credits?|courses?|` + numPat + `|one))\s*of\s+which\b`),
		// "The choices in 3 must satisfy the requirement for ..."
		regexp.MustCompile(`(?i)\bthe\s+choices\s+(?:in|from)\s+\S+(?:\s+\S+)?\s+must\b`),
		// "At least 1.5 credits must be in FRE or FSL courses"
		regexp.MustCompile(`(?i)\b` + atLeastPat + `\b[^.]*?\b(?:must|needs?\s+to|has\s+to|have\s+to|is\s+required)\b`),
		// "All 4.0 credits must be NMC and/or NML courses"
		regexp.MustCompile(`(?i)\b(?:credits?|courses?)\s+(?:must|needs?\s+to)\s+(?:be\s+(?:taken\s+)?(?:at|in|from|a|an|` + levelPat + `|[A-Z]{3})\b|come\s+from\b|have\s+the\b)`),
	}
	// complScopeRe matches a scope: "Of these 4.0 credits,". The cue after it
	// decides the role; with none, it is a constraint.
	complScopeRe = regexp.MustCompile(`(?i)^of\s+(?:these|the)\b`)

	// complLimitRes are cues for a limit: at most an amount counts.
	complLimitRes = []*regexp.Regexp{
		// "No more than 1.0 credit of 100-level courses may be used"
		regexp.MustCompile(`(?i)\b` + noMoreThanPat + `\b|\bat\s+most\b|\bnot\s+exceed\b|\bcannot\s+comprise\s+more\s+than\b`),
		// "a maximum of 1.0 total program credits may be drawn from courses
		// taught in English"
		regexp.MustCompile(`(?i)\bmaximum\s+(?:of\s+)?(?:a\s+|one\s+|two\s+|\d)`),
		// "Only 1.0 credit from 100-level ENG courses may be counted"
		regexp.MustCompile(`(?i)\bonly\s+(?:a\s+)?(?:maximum\s+of\s+)?(?:` + numPat + `|one|two)\b[^.]*?\b(?:will|may|can)\s+(?:be\s+)?(?:accepted|counted|count|used|applied)\b`),
		// "Up to 1.0 credit can be used from GGR493Y1"
		regexp.MustCompile(`(?i)\bup\s+to\s+(?:a\s+|one\s+|two\s+)?(?:` + numPat + `\s+)?(?:credits?|courses?|half|full)?[^.]*?\b(?:can|may)\b`),
		// "MAT221H1 may not be used for this requirement".
		regexp.MustCompile(`(?i)\b(?:cannot|may\s+not|will\s+not)\s+(?:count|be\s+(?:counted|used|applied|accepted))\s+(?:to|for|toward|towards)\b`),
		// "STA220H1 and STA255H1 are not accepted for the major"
		regexp.MustCompile(`(?i)\bnot\s+(?:be\s+)?(?:accepted|counted)\s+toward|\bdo\s+not\s+accept\b|\bare\s+not\s+accepted\s+for\b|\bnot\s+included\s+for\s+consideration\b|\bnot\s+permitted\s+to\s+take\b`),
		// "Students cannot take more than a 0.5 credit at the 400-level"
		regexp.MustCompile(`(?i)\b(?:cannot|may\s+not|will\s+not)\s+(?:use|take|count|be\s+met\s+by)\s+more\s+than\b`),
	}

	// complRecommendRe matches advice: "We recommend that students take
	// MAT148H1".
	complRecommendRe = regexp.MustCompile(`(?i)\brecommend(?:s|ed|ation|ations)?\b|\bencouraged?\b|\bsuggest(?:s|ed|ion|ions)?\b|\badvis(?:ed|able)\b|\bshould\s+(?:especially\s+|seriously\s+|also\s+)?consider\b|\bshould\s+note\s+that\b|\b(?:may|might)\s+wish\b|\bpreferabl[ey]\b`)
	// complTimingNoteRe matches timing advice on a required item, removed
	// before the advice checks: ", normally taken in the first year".
	complTimingNoteRe = regexp.MustCompile(`(?i),\s*(?:(?:highly|strongly)\s+)?(?:recommended\s+to\s+be\s+taken|normally\s+taken)\b[^;]*`)

	// complAdviceVetoRe vetoes advice: "must" binds, and so do courses called
	// required for the program: "required for the specialist" ...
	complAdviceVetoRe = regexp.MustCompile(`(?i)\bmust\b|\b(?:are|is)\s+(?:an?\s+)?required\s+courses?\b|\brequired\s+for\s+the\s+(?:program|specialist|major|minor|focus|certificate)\b`)
	// ... unless the "must" is procedure or timing (complProcedureRe):
	// "students must request permission" ...
	complProcedureRe = regexp.MustCompile(`(?i)\bmust\s+(?:also\s+)?(?:first\s+)?(?:request|submit|check|consult|contact|apply|speak|receive|obtain\s+(?:prior\s+)?(?:consent|permission)|meet\s+(?:with|(?:all\s+)?(?:the\s+)?(?:specified\s+)?prerequisites)|be\s+enrolled|be\s+made|be\s+submitted)\b|\brequests?\s+must\s+be\b|\bmust\b[^.]*\bbefore\s+(?:starting|taking|enrolling|beginning|graduation)\b`)
	// ... or a degree-level rule (complDegreeRuleRe): "must complete a total of
	// at least 12.0 distinct credits" ...
	complDegreeRuleRe = regexp.MustCompile(`(?i)\bcombination\s+of\s+programs\b|\bdistinct\s+credits\b|\b\d+\.\d\s+credit\s+rule\b|\bin\s+addition\s+to\s+the\s+` + numPat + `\s+credits\s+required\s+for\b|\btowards?\s+their\s+(?:[\w&]+\s+){0,3}degree\b`)
	// ... or a description naming required courses without requiring them here
	// (complVetoDescriptionRe): "ACT390H1 is a required course in the actuarial
	// science specialist program".
	complVetoDescriptionRe = regexp.MustCompile(`(?i)\bas\s+well\s+as\s+suggested\s+courses\b|^a\s+significant\s+part\s+of\s+the\s+course\s+must\s+involve\b|^` + codePat + `\s+is\s+a\s+required\s+course\s+in\s+the\s+(?:[\w-]+\s+){1,4}(?:specialist|major|minor)\s+program$`)
)

// complNotBindingRes are sentences that read like rules but which a checker of
// this program can't or needn't evaluate. "must" vetoes them unless it is
// procedure or a degree-level rule.
var complNotBindingRes = []*regexp.Regexp{
	// A placement exception, which names no pool: "After consultation with ...,
	// exceptions may be made ... regarding the sequence of language courses".
	regexp.MustCompile(`(?i)^after\s+consultation\s+with\b[^.]*\bexceptions\s+may\s+be\s+made\b[^.]*\bsequence\s+of\s+language\s+courses\b`),
	// A program-wide consideration naming no requirement: "Courses that are in
	// the current academic Calendar that include significant ... may be
	// considered, in consultation with the ... Director".
	regexp.MustCompile(`(?i)^courses\s+that\s+are\s+in\s+the\s+current\s+academic\s+calendar\s+that\s+include\s+significant\b[^.]*\bmay\s+be\s+considered,\s+in\s+consultation\s+with\b`),
	// Permissions and advice that don't change what counts: "Students are
	// advised to discuss potential course substitutions".
	regexp.MustCompile(`(?i)^(?:[\w-]+\s+){0,2}students\s+can\s+take\s+` + codePat + `\b.*\bprovided\s+they\s+have\b|^students\s+can\s+take\s+either\s+course\s+as\s+their\s+required\b.*\bas\s+an\s+additional\s+elective$|^students\s+are\s+advised\s+to\s+discuss\b|^a\s+significant\s+part\s+of\s+the\s+course\s+must\s+involve\b`),
	// Rules about other programs: "IRE/IRW courses cannot be used to fulfill
	// enrolment/program requirements in Economics".
	regexp.MustCompile(`(?i)^except\s+for\s+students\s+enrolled\s+in\b[^,;]*,\s+who\s+may\s+apply\b.*\btowards?\s+their\s+(?:Specialist|Major|Minor)\s+program$|\bprogram\s+requirements\s+in\s+(?-i:[A-Z])|\brequirements\s+for\s+(?:a|another|any)\s+(?:Specialist|Major|Minor)\b|\btowards?\s+an?\s+` + desigPat + `\s+(?:major|minor|specialist)\b|\bnot\s+considered\s+equivalent\b`),
	// Timing the checker doesn't enforce: "a maximum of 3.0 credits from Group
	// B per year".
	regexp.MustCompile(`(?i)\bper\s+(?:academic\s+)?(?:year|session|term|semester)\b`),
	complDegreeRuleRe,
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
	regexp.MustCompile(`(?i)\bcannot\s+take\s+` + codePat + `|\bcapped\s+at\b|^enrolment\s+in\s+(?:either\s+|any\s+of\s+)?` + codePat + `(?:\s*(?:,|or|and|/)\s*` + codePat + `)*\s+is\s+limited\b`),
	// Another program described: "The focus that is available for students to
	// pursue is ...".
	regexp.MustCompile(`(?i)\bfoc(?:us|i)\s+that\s+(?:is|are)\s+available\b`),
}

// complAdviceRes is advice, which doesn't bind unless the sentence offers a
// substitution (complSubstitutionRes).
var complAdviceRes = []*regexp.Regexp{complRecommendRe,
	// What to take for a focus within the program's choices: "Students who want
	// to focus on ... medical anthropology should take ANT203Y1".
	regexp.MustCompile(`(?i)\bstudents\s+who\s+(?:want|wish)\s+to\s+focus\b[^.]*?\bshould\s+take\b`),
}

// complSubstitutionRes are the approval routes that offer a substitution.
// Unlike the others, they beat advice in the same sentence: "Students are
// advised to discuss potential course substitutions".
var complSubstitutionRes = []*regexp.Regexp{
	// "Up to 2.0 credits in GRK or LAT may be substituted for the 5.5 credits"
	regexp.MustCompile(`(?i)\b(?:may|can)\s+(?:request\s+to\s+)?(?:be\s+)?substitut(?:e|ed)\b`),
	// "Course substitutions are possible with written permission of the
	// Program Director".
	regexp.MustCompile(`(?i)\bsubstitutions?\s+(?:are|is)\s+possible\b`),
	// "Students can request substitutions from the IRP Director"
	regexp.MustCompile(`(?i)\bto\s+substitute\b|\brequest\s+(?:a\s+)?substitutions?\b|\bdiscuss\s+(?:potential\s+|possible\s+)?(?:course\s+)?substitutions?\b`),
}

// complApprovalRes are approval routes, which the assembler encodes as a
// permission alternative. They bind even with a cap or a contact in them.
var complApprovalRes = slices.Concat(complSubstitutionRes, []*regexp.Regexp{
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

// complCountQueryRe matches a question about counting another course, an
// approval route even in a sentence of advice: "whether it may be appropriate
// to count a related course towards Focus requirements".
var complCountQueryRe = regexp.MustCompile(`(?i)\b(?:whether|if)\b[^.]*\bcount\b[^.]*?\btowards?\s+(?:the\s+|your\s+|this\s+)?(?:[\w-]+\s+){0,2}(?:programs?|focus|requirements?)\b`)

// complWidenRe matches a pool widened with a cap: "Up to 1.0 credit may be from
// other departments". The assembler adds both the wider pool and the cap.
var complWidenRe = regexp.MustCompile(`(?i)\bup\s+to\b.*?\b(?:may|can)\s+(?:also\s+be\s+(?:taken|chosen|drawn|completed)\s+from|be\s+(?:(?:taken|chosen|drawn)\s+)?from\s+(?:the\s+following\s+)?(?:approved\s+courses\s+offered\s+by\s+)?(?:cognate|other\s+departments))`)

// complBindingProseRes are binding sentences that may have no code, amount or
// filter.
var complBindingProseRes = []*regexp.Regexp{
	// Pools in prose: "Other courses are also eligible to meet this
	// requirement".
	regexp.MustCompile(`(?i)\b(?:are|is)\s+(?:also\s+)?eligible\b|\bcounts?\s+towards?\s+this\s+requirement\b|\bsatisfied\s+by\b|\b(?:can|may)\s+(?:also\s+)?be\s+used\s+to\s+meet\b|\bmeet\s+this\s+requirement\b`),
	// Inheritance: "begins with the requirements of First Year".
	complInheritRe,
	// An exception to the total: "There is no need to replace the missing
	// half-credit for program completion".
	regexp.MustCompile(`(?i)\bno\s+need\s+to\s+replace\b`),
}

// complAsideRes are advice, explanation and restatement that read like a
// requirement (an amount, a code, "must"), checked before every rule.
var complAsideRes = []*regexp.Regexp{
	// A course's own corequisite, restated: "EEB498Y1 concurrent with research
	// issues course EEB488H1".
	regexp.MustCompile(`^` + codePat + `\s+concurrent\s+with\b`),
	// "Students interested in Mineral Exploration: First Year: BIO120H1"
	regexp.MustCompile(`(?i)^students\s+(?:interested\s+in|with\s+an\s+interest\s+in)\b[^:]*:|\bmight\s+(?:be\s+)?of\s+interest\b`),
	// The courses for a theme: "Neuroscience Centric Courses: HMB200H1".
	regexp.MustCompile(`^[\w&,\s]{1,60}\bCentric\s+Courses\s*:`),
	// Timing: "to be taken as early as possible".
	regexp.MustCompile(`(?i)^(?:normally\s+taken\s+in\s+(?:the\s+)?first\s+year|to\s+be\s+taken\s+as\s+early\s+as\s+possible|first\s+offering\s+in\s+\d{4}(?:-\d{2,4})?)$|^should\s+be\s+taken\s+together\s+with\b|^(?:first|second|third|fourth)-year\s+students\s+should\s+take\b`),
	// A course's own prerequisite or audience: "STA130H1 is restricted to
	// first-year students".
	regexp.MustCompile(`^(?:` + codePat + `(?:\s*(?:,|and|/)\s*` + codePat + `)*)\s+requires?\s|\bis\s+required\s+for\s+all\s+[A-Z]{3}\s+` + levelPat + `\+?[\s-]level\s+courses\b|^Students\s+taking\s+` + codePat + `\s+must\s+take\s+` + codePat + `\s+prior$|\bis\s+restricted\s+to\s+first-year\s+students\b|\bis\s+for\s+students\s+who\s+have\s+(?:previously\s+)?taken\b`),
	// A course exclusion, which enrolment enforces: "please note that this
	// course is an exclusion to EEB125H1".
	regexp.MustCompile(`(?i)\bplease\s+note\s+that\s+(?:these\s+courses\s+are|this\s+course\s+is)\s+(?:an?\s+)?exclusions?\s+to\b`),
	// Descriptions: "MST300H1 is offered through the Centre for Medieval
	// Studies".
	regexp.MustCompile(`^` + codePat + `\s+is\s+offered\s+(?:through|by)\b|\bcourses\s+from\s+other\s+campuses\s+are\s+considered\s+courses\s+from\b|^These\s+[A-Z]{3}\s+courses\s+address\b|^` + codePat + `\s+is\s+NOT\s+a\s+substitute\s+for\b|^\*?course(?:\(s\))?\s+no\s+longer\s+offered$|^Requirement\s+\d+\s+[^.]*\brepresents\s+a\b|^The\s+updated\s+requirements\s+are\s+for\b|^Courses\s+with\s+codes\s+that\s+do\s+not\s+start\s+with\b[^.]*\bconducted\b[^.]*\bin\s+English\b`),
	complVetoDescriptionRe,
	// Degree-level remarks: "You may choose more than 2.5 credits from the
	// Additional Foundation Geosciences list".
	regexp.MustCompile(`\bcredits\s+needed\s+for\s+your\s+degree$|^Note\s+that\b[^.]*\bcan\s+normally\s+be\s+fulfilled\b|^You\s+may\s+choose\s+more\s+than\b`),
	// A header's "including PHL100Y1/ PHL101Y1 if taken": the courses count
	// toward the total like any other, and its pool already holds them.
	regexp.MustCompile(`^including\s+` + codePat + `(?:\s*/\s*` + codePat + `)*\s+if\s+taken$`),
	// A total restated in brackets after the items: "(3.0 credits in total)".
	regexp.MustCompile(`(?i)^(?:a\s+)?total\s+of\s+` + numPat + `\s+credits?$|^` + numPat + `\s+credits\s+in\s+total$`),
	complAsideMustRe,
	// The freedom an elective item leaves: "The 1.5 credits can be chosen to
	// reflect the particular academic interests of each student".
	regexp.MustCompile(`(?i)^the\s+` + numPat + `\s+credits?\s+can\s+be\s+chosen\s+to\s+reflect\b`),
	// A descriptor closing a list of alternatives, kept by stripAsides: "a
	// capstone integrative course".
	regexp.MustCompile(`^an?\s+[a-z][a-z\s-]*\bcourse$`),
}

// complAsideMustRe matches asides whose "must" is a course's own timing or
// prerequisite, or a lead into the items, so the advice veto doesn't apply:
// "Courses must fulfill the following requirements:".
var complAsideMustRe = regexp.MustCompile(`(?i)^should\s+be\s+taken\s+together\s+with\s+` + codePat + `,\s+must\s+be\s+taken\s+prior\s+to\s+completing\s+` + codePat + `$|^Students\s+taking\s+` + codePat + `\s+must\s+take\s+` + codePat + `\s+prior$|^courses\s+must\s+(?:fulfil|fulfill|satisfy|meet)\s+the\s+following\s+requirements:?$`)

// complRecommendedHeadRe matches a heading that offers what follows as advice:
// "First year (recommended): PHL100Y1/ PHL101Y1".
var complRecommendedHeadRe = regexp.MustCompile(`(?i)^[^:()]{0,40}\((?:strongly\s+|highly\s+)?recommended\)\s*:`)

// complAdvisoryRes are recurring sentences of advice, explanation or procedure,
// never a requirement. "must" or "required courses" vetoes them unless it is
// procedure or a degree-level rule.
var complAdvisoryRes = []*regexp.Regexp{
	// "Students in this program have the option to complete the Arts & Science
	// Internship Program (ASIP) stream"
	asipRe,
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
	regexp.MustCompile(`(?i)^` + codePat + `(?:\s*[/,]\s*` + codePat + `)*\s+(?:may|can)\s+be\s+taken\s+in\s+(?:the\s+)?(?:first|second|third|fourth)\s+year$|\bmay\s+choose\s+to\s+(?:enrol|enroll|register)\s+in\s+` + codePat + `\s+in\s+(?:their|the)\s+(?:first|second|third|fourth)\s+year$`),
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
	complProcedureRe,
}

// classifySentence returns the role of one sentence of completion text; the
// first matching case wins. Advice is matched outside brackets and in the first
// ";" clause only, so an item with "(recommended)" stays an item.
func classifySentence(s string) sentenceRole {
	t := complClean(s)
	if !complWordRe.MatchString(t) || complBareMarkerRe.MatchString(t) {
		return roleAdvisory
	}
	switch {
	case matchAny(complRestatedRes, t):
		return roleAdvisory
	case complRecommendedHeadRe.MatchString(t) || isComplAdvisory(t, complAsideRes):
		return roleAdvisory
	case matchAny(complExclusionRes, t):
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
	case isComplAdvisory(t, complAdviceRes) && !matchAny(complSubstitutionRes, t):
		return roleAdvisory
	case matchAny(complApprovalRes, t) || complWidenRe.MatchString(t):
		return roleRequirement
	}
	if r, ok := complRuleRole(t); ok {
		return r
	}
	switch {
	case isComplListHead(t):
		return roleListDef
	case matchAny(complBindingProseRes, t):
		return roleRequirement
	case isComplAdvisory(t, complAdvisoryRes):
		return roleAdvisory
	case codeRe.MatchString(t) || complAmountRe.MatchString(t) || complFilterRe.MatchString(t) ||
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

// complWrapped reports whether u is in one pair of brackets: "[ PHY131H1,
// PHY132H1 recommended]".
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

func matchAny(res []*regexp.Regexp, s string) bool {
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
// kind, and false when there is none.
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
	// a code-led item's caps in brackets are its own
	lt := t
	if codeLead {
		lt = complStripParens(t)
	}
	for _, re := range complLimitRes {
		loc := re.FindStringIndex(lt)
		if loc == nil || best >= 0 && loc[0] >= best {
			continue
		}
		// a code-led sentence allocating an amount before the cap is an item
		if codeLead && complAmountRe.MatchString(lt[:loc[0]]) {
			continue
		}
		best, role = loc[0], roleLimit
	}
	if best < 0 && !codeLead && complScopeRe.MatchString(t) && !isComplAdvisory(t, complAdvisoryRes) {
		return roleConstraint, true // "Of these:", unless it is advice
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
	if complRecommendedTagRe.MatchString(t) && !codeRe.MatchString(t) {
		return true
	}
	bare := complStripParens(complTimingNoteRe.ReplaceAllString(t, ""))
	bare = complAttributiveAdviceRe.ReplaceAllString(bare, "")
	// advice in a later ";" clause doesn't make the sentence advice
	bare = splitTop(bare, ';')[0]
	return matchAny(res, bare)
}

// splitComplSentences splits a line of completion text into sentences as
// splitSentences does, and also before a sentence that starts with an amount
// and at inline lettered notes. A list marker stays with the first sentence.
func splitComplSentences(s string) []string {
	s = complNBRe.ReplaceAllString(strings.TrimSpace(s), "NB: ")
	s = stripEmphasis(complItalicNoteRe.ReplaceAllString(s, "$1. $2"))
	marker := markerRe.FindString(s)
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

// complSplitAtAmounts splits s with splitSentences, and also, outside brackets,
// before a sentence that starts with an amount, a level or a bracketed code.
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

// complContentRes are clauses saying what a course's content, topic, type or
// language decides, which the catalogue doesn't record. contentRest removes
// each (or puts repl in its place); the rest is what a checker can read.
var complContentRes = []struct {
	re   *regexp.Regexp
	repl string
}{
	// Language of instruction: "No more than 2.0 credits taught in English".
	{regexp.MustCompile(`\s*\b(?:(?:from\s+)?courses\s+)?(?:taught|taken)\s+in\s+(?:English|French|Spanish|Portuguese|German|Italian)\b`), ""},
	// "where the primary language of study is not English", with any floor on
	// those courses after it
	{regexp.MustCompile(`\s*\bwhere\s+the\s+primary\s+language\s+of\s+study\s+is\s+not\s+English\b(?:,\s*including\s+` + atLeastPat + `\s+` + numPat + `\s+credits?\s+at\s+the\s+\d00\+?[\s-]level)?`), ""},
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
	{regexp.MustCompile(`^Industry-engaged\s+version\s+of\s+(` + codePat + `)(?::.*)?$`), "$1"},
	{regexp.MustCompile(`["“”]Program-Only["“”]\s*|\s*\(\s*` + codePat + `\s*[–-]\s*` + codePat + `;[^()]*\)`), ""},
	// Lists kept elsewhere: "5.0 credits in any of the four areas of Jewish
	// Studies".
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

// complContentFilterRe matches what a checker could read besides a bare amount:
// a course, a level, a designator or a named list such as "Group A".
var complContentFilterRe = regexp.MustCompile(codePat + `|\b` + levelPat + `(?:\+|\s*-?\s*(?:level|series))|\b[A-Z]{3}\b|\b(?:Groups?|Lists?|Categor(?:y|ies))\s+[A-Z0-9#]`)

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
	walkReq(n, func(x *types.ReqNode) {
		if x.Type == types.NodeText && x.Kind == types.TextOther && complFromListBelowRe.MatchString(x.Text) {
			x.Kind = types.TextContent
		}
	})
}

// listBelowGiven reports whether a course code follows r's first line in raw:
// then the calendar gives the list, though it was kept as notes (ASMIN1210).
func listBelowGiven(raw string, r *types.Requirement) bool {
	head, _, _ := strings.Cut(r.Raw, "\n")
	i := strings.Index(raw, head)
	return head != "" && i >= 0 && codeRe.MatchString(raw[i+len(head):])
}

// wholeTextKind marks a row's text as content when its rest reads as a pool,
// giving and(pool, text), or names no course or filter. A code range within the
// pool's filters becomes the pool itself: "( SOC317H1 – SOC354H1; ...)".
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
// when the codes a text names are beside it, permission for an approval
// alternative beside a strict branch, and placement.
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
		named := !slices.ContainsFunc(codeRe.FindAllString(t, -1), func(c string) bool { return !slices.Contains(codes, c) })
		switch {
		case it.Kind != types.TextOther && it.Kind != types.TextMembership:
		case isComplApprovalText(t):
			// a person decides, whatever the content; with no strict branch
			// beside it, it is the requirement itself and stays other
			if strict && it.Kind == types.TextOther {
				it.Kind = types.TextPermission
			}
		case complPlacementTextRe.MatchString(t):
			it.Kind = types.TextPlacement
		case strict && complAnyOtherApprovedRe.MatchString(t):
			it.Kind = types.TextPermission
		case named && contentText(t):
			it.Kind = types.TextContent
			// an and's part gets its pool beside it, as a whole row does
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
	// complPlacementTextRe matches what a placement or proof of level decides:
	// "Students whose placement assessment exempts them".
	complPlacementTextRe = regexp.MustCompile(`(?i)\bplacement\s+(?:assessment|test)\s+exempts?\b|^proof\s+of\s+(?:comparable|equivalent)\s+[A-Z][a-z]+\s+(?:level|proficiency)$`)
	// complAnyOtherApprovedRe matches an approval beside the listed courses:
	// "any other approved course".
	complAnyOtherApprovedRe = regexp.MustCompile(`(?i)^(?:or\s+)?any\s+other\s+approved\s+courses?\b`)
)

// contentText reports a content sentence (complContentRes).
func contentText(t string) bool {
	_, ok := contentRest(t)
	return ok
}

// complConsultRe matches approval by consultation: "selected in consultation
// with the Undergraduate Coordinator".
var complConsultRe = regexp.MustCompile(`(?i)\bin\s+(?:joint\s+)?consultation\s+with\b|\bnomination\s+process\b|\bpending\s+approval\b|\bpre-?approved\b|\bapproved\s+by\s+the\b`)

// isComplApprovalText reports a sentence in which someone approves what counts.
func isComplApprovalText(t string) bool {
	return matchAny(complApprovalRes, t) || complCountQueryRe.MatchString(t) || complConsultRe.MatchString(t)
}

// complSomeOfferingsRe matches courses that count in some offerings only: "Some
// offerings of HIS357H1 and HIS389H1 may also count towards this focus".
var complSomeOfferingsRe = regexp.MustCompile(`^[Ss]ome\s+offerings\s+of\s+(` + codePat + `(?:(?:,\s*|,?\s+and\s+)` + codePat + `)*)\s+may\s+also\s+count\b[^.]*$`)

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
		m := complSomeOfferingsRe.FindStringSubmatch(complClean(it.Text))
		if it.Type != types.NodeText || it.Kind != types.TextOther || m == nil {
			continue
		}
		it.Kind = types.TextContent
		wide := &types.ReqNode{Type: types.NodeCredits, Amount: pool.Amount, Courses: dedupe(append(slices.Clone(pool.Courses), codeRe.FindAllString(m[1], -1)...))}
		n.Items[i+1] = andNode(wide, it)
	}
}

// complEgLevelRe matches a route that counts a course at a level: "Students may
// be permitted to take Math graduate courses. A 0.5 credit of these courses may
// count ... (e.g., as courses "at the 400-level")".
var complEgLevelRe = regexp.MustCompile(`(?i)\bmay\s+be\s+permitted\b.*\bA\s+0\.5\s+credit\s+of\s+these\s+courses\s+may\s+count\b.*\be\.g\.,?\s+as\s+courses\s+["“]at\s+the\s+\d00-level`)

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

// isApproval reports whether s is an approval route: "Other introductory
// language classes may be approved by the Director".
func (a *compAssembler) isApproval(s string) bool {
	return a.cue(s).approval
}

// complAlternativeRe matches a sentence offering another way to meet a
// requirement: "CSB450H1 is also acceptable".
var complAlternativeRe = regexp.MustCompile(`(?i)\balso\s+(?:be\s+)?(?:used|count(?:s|ed)?|accepted|acceptable|eligible|qualif\w*|fulfil\w*|satisf\w*|meet|complete)\b|\b(?:is|are)\s+also\s+acceptable\b|\bsubstitute\b|\bqualif(?:y|ies)\b|\b(?:meet|fulfil+|satisfy)\s+(?:this|the)\s+(?:[\w-]+\s+){0,3}requirement\b|\bcounts?\s+towards?\b|\bbe\s+counted\s+towards?\b|\bin\s+lieu\b|\b(?:can|may)\s+be\s+used\b|\bwill\s+be\s+accepted\b|\bacceptable\b`)

// isAlternative reports whether s offers another way to meet a requirement,
// or widens its pool with a cap.
func (a *compAssembler) isAlternative(s string) bool {
	return a.cue(s).alternative
}

// sentenceCues is what the assembler asks about a sentence besides its role.
type sentenceCues struct{ approval, alternative bool }

// cue reads s's cues once.
func (a *compAssembler) cue(s string) sentenceCues {
	c, ok := a.cues[s]
	if !ok {
		t := a.clean(s)
		c.approval = matchAny(complApprovalRes, t) || complCountQueryRe.MatchString(t)
		c.alternative = !complNegationRe.MatchString(complNotListedRe.ReplaceAllString(t, "")) && complAlternativeRe.MatchString(t) || complWidenRe.MatchString(t)
		a.cues[s] = c
	}
	return c
}

// complNegationRe matches a sentence that refuses an alternative: "cannot".
var complNegationRe = regexp.MustCompile(`(?i)\bnot\b|\bcannot\b|n['’]t\b`)

// complRestatedRes match sentences that restate what the rows say, or are
// advice, though a binding word or a code is in them. Unlike complAsideRes,
// "must" doesn't veto them: each is one wording.
var complRestatedRes = []*regexp.Regexp{
	// a course's overlap with others, which their exclusions encode: "MGT100H1
	// have considerable overlap with RSM100H1"
	regexp.MustCompile(`^` + codePat + `(?:,\s*` + codePat + `)*\s+have\s+considerable\s+overlap\s+with\b`),
	// the claims rule restated: "The course taken in lieu of STA130H1 cannot be
	// one that is already being used to satisfy Requirement 9"
	regexp.MustCompile(`(?i)\b(?:taken\s+in\s+lieu\s+of|used\s+to\s+replace)\s+` + codePat + `\s+cannot\s+be\s+(?:one|a\s+course)\s+that\s+is\s+already\s+being\s+used\s+to\s+(?:satisfy|meet)\b|^students\s+admitted\s+to\s+the\s+program\s+with\s+[^:]+:\s+the\s+course\s+will\s+be\s+applied\s+to\s+requirement\b`),
	// a pointer to the note an alternative comes from: "If you are unable to
	// complete STA130H1 in first year, see Note A below for ..."
	regexp.MustCompile(`(?i)^if\s+you\s+are\s+unable\s+to\s+complete\s+` + codePat + `\b[^.]*,\s+see\s+note\s+[A-Z]\s+below\s+for\b`),
	// a course's prerequisites, which the list leaves out: "(note that both
	// require BIO130H1 and BIO230H1/ BIO255H1)"
	regexp.MustCompile(`(?i)\bprerequisites\s+for\s+these\s+courses\s+cannot\s+be\s+used\s+towards\b|^note\s+that\s+(?:both|these(?:\s+courses)?|this\s+course|they|it)\s+requires?\s|^` + codePat + `\s+may\s+be\s+taken\s+without\s+` + codePat + `\s+for\s+students\s+who\b`),
	// amounts the rows encode: "Sub-total = 8.5 credits"
	regexp.MustCompile(`(?i)^if\s+taking\s+` + codePat + `,\s+the\s+extra\s+` + numPat + `\s+credit\s+can\s+go\s+to\b|^or\s+any\s+of\s+the\s+following:\s*` + codePat + `(?:\s*[/,]\s*` + codePat + `)*$|^sub-?total\s*=\s*` + numPat + `(?:\s+or\s+` + numPat + `)?\s+credits?(?:\s*\([^)]*\))?$|^in\s+addition\s+to\s+the\s+above\s+core\s+courses,\s+you\s+need\s+to\s+take\s+` + numPat + `\s+credits?\s+of\s+electives$|^plus\s+a\s+disciplinary\s+focus\s+requiring\s+` + numPat + `\s*-\s*` + numPat + `\s+credits$`),
	// a focus's own requirements, which focusChoices reads: "Health Studies
	// (ASFOC2270A): (2.0 credits) HMB342H1, ..."
	regexp.MustCompile(`(?i)^[^:()]{1,80}\((?-i:[A-Z]{2}FOC\d{4}[A-Z])\):\s*\(` + numPat + `(?:\s+or\s+` + numPat + `)?\s+credits\)`),
	// a designator definition, which aliasDesignators applies
	complAliasPrefixesRe,
	// enrolment in courses, not what counts: "... may request to enroll in the
	// following RSM courses"
	regexp.MustCompile(`(?i)\bmay\s+request\s+to\s+enrol+\s+in\s+the\s+following\b`),
}
