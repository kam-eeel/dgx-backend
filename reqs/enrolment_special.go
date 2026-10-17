package reqs

import (
	"regexp"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Sentences that say what kind of program this is, how many credits a
// student needs, and the requirements that aren't course lists: program
// enrolment, applications and interviews, timing rules and programs that
// can't be combined.

var (
	// "This is an open enrolment program.", "This is a limited enrolment
	// Certificate.", "this is a limited enrollment program that can
	// accommodate ...", "The Research Specialist is a limited program".
	enrolKindRe = regexp.MustCompile(`(?i)\b(?:this\s+is\s+an?|is\s+an?)\s+(open|limited)\s+(?:enrol(?:l)?ment\s+)?(?:program|certificate)\b(.*)$`)
	// "... limited enrolment certificate for any non-Rotman Commerce student
	// in the Faculty of Arts & Science"
	enrolNonRe = regexp.MustCompile(`(?i)\bfor\s+any\s+non-(.+?)\s+students?\b`)
	// "**Enrolment in the Forest Biomaterials Science Major is being
	// administratively suspended as of 1 October 2020 ...**", "Admission to
	// the Environmental Geosciences Minor has been administratively suspended
	// ... and is no longer admitting students."
	enrolSuspendedRe = regexp.MustCompile(`(?i)administratively\s+suspended|no\s+longer\s+admitting|no\s+new\s+students\s+will\s+be\s+admitted`)

	// "Students must have completed 4.0 credits and meet the requirements
	// listed below to enrol.", "... a minimum of 4.0 credits ... to apply for
	// enrolment.", "A student who has completed 4.0 credits may enrol in the
	// program.", "A student who has completed 4.0 credits including MGT100H1
	// or equivalent may enrol in the Certificate."
	enrolThresholdRe = regexp.MustCompile(`(?i)^(?:students\s+must\s+have\s+completed|a\s+student\s+who\s+has\s+completed)\s+(?:a\s+minimum\s+of\s+)?(\d+(?:\.\d+)?)\s+credits?\b(?:\s+including\s+(.+?)\s+may\s+enrol\b)?`)

	// The cutoff changes each year: "... and this minimum changes each year
	// depending on available spaces and the number of applicants", "this
	// minimum may change", "this average varies each year", "the minimum
	// grade average in required courses may vary year to year", "a higher
	// grade/grade average than the posted minimums listed below, which will
	// depend each year on ...".
	enrolVariableRe = regexp.MustCompile(`(?i)\bchanges?\s+each\s+year|\bmay\s+(?:change|vary)\b|\bvar(?:y|ies)\s+(?:each\s+year|year\s+to\s+year)|\bdepend\s+each\s+year`)
	// A variable cutoff that names its courses: "A minimum grade average in
	// STA130H1 and ( MAT148H1, MAT149H1)/ MAT137Y1 is needed for entry."
	enrolVariableScopeRe = regexp.MustCompile(`(?i)^(?:a\s+)?minimum\s+grade(?:\s+average)?s?\s+in\s+.*` + codePat + `.*\s+(?:is|are)\s+needed\s+for\s+entry$`)

	// Program enrolment: "Enrolment in the Computer Science Major Program
	// (ASMAJ1689).", "Enrolment in the History Specialist is required.",
	// "Enrolment in the International Relations Major is required for entry
	// into the Focus.", "Enrolment in the Human Geography Major (ASMAJ1667)
	// program is required."
	enrolInRe = regexp.MustCompile(`(?i)^enrol(?:l)?ment\s+in\s+(?:the\s+)?(.+?)(?:\s+is\s+required)?(?:\s+for\s+entry\s+into\s+(?:the|this)\s+focus)?$`)
	// "Students must be enrolled in the Psychology Minor, Major or
	// Specialist, to be considered for enrolment.", "Student must be enrolled
	// in the Criminology and Sociolegal Studies Major (ASMAJ0826)", "In order
	// to apply for the Economics Specialist Program, a student must first be
	// enrolled in the Economics Major (ASMAJ1478)".
	enrolMustBeInRe = regexp.MustCompile(`(?i)^(?:in\s+order\s+to\s+apply\s+for\s+.+?,\s+)?(?:a\s+)?students?\s+must\s+(?:first\s+)?be\s+enrolled\s+in\s+(?:the\s+)?(.+?)(?:,?\s+to\s+be\s+considered\s+for\s+enrolment)?$`)
	// The parent of an application-based focus: "Once you have enrolled in
	// the Cell and Molecular Biology Major program, you have the option to
	// apply for entry into a focus."
	enrolOnceInRe = regexp.MustCompile(`(?i)^once\s+you\s+have\s+(?:been\s+approved\s+for\s+and\s+have\s+)?enrolled\s+in\s+the\s+(.+?)(?:\s+program)?,\s+you\s+have\s+the\s+option\s+to\s+apply\s+for\s+entry\s+into\s+a\s+focus`)
	// "Students who complete the Technology Leadership Focus application
	// and/or interview and are admitted to a Computer Science Specialist or
	// Major or Data Science Specialist program will be considered for
	// admission to the Focus."
	enrolAdmittedToRe = regexp.MustCompile(`(?i)\band\s+are\s+admitted\s+to\s+an?\s+(.+?)\s+program\s+will\s+be\s+considered\s+for\s+admission`)
	// "Students who have completed at least 8.0 credits and who are already
	// enrolled in ASSPE2340 may enrol in ASSPE2082 themselves on ACORN."
	enrolSelfEnrolRe = regexp.MustCompile(`(?i)^students\s+who\s+have\s+completed\s+(?:at\s+least\s+)?(\d+\.\d+)\s+credits\s+and\s+who\s+are\s+already\s+enrolled\s+in\s+(` + progCodePat + `)\s+may\s+enrol\s+in\s+` + progCodePat + `\s+themselves`)
	// Only one admission category may apply: "Only students in the Year 1
	// Computer Science admission category (CMP1) who meet the criteria of the
	// Computer Science program admission guarantee are eligible to apply ..."
	enrolOnlyRe = regexp.MustCompile(`(?i)^only\s+(students\s+in\s+the\s+.+?)\s+(?:who\s+meet\s+.+?\s+)?are\s+eligible\s+to\s+apply`)

	// Decided by the department: "Students who do not have the Computer
	// Science Admission Guarantee must complete a supplementary application
	// to be considered", "Students must pass an interview", "passing a short
	// admissions test", "submit a portfolio of creative works", "an
	// additional audition-interview", "students must include a 300-word
	// statement of interest", "Complete an application and/or interview",
	// "Students must complete the Rotman Commerce Supplemental Application",
	// "Program applicants will be required to submit a supplemental
	// application", "Admission is determined by a range of criteria, including
	// ... short essays, a writing sample, a resume, and a video submission".
	enrolAdmissionRe = regexp.MustCompile(`(?i)supplementa(?:l|ry)\s+application|\bpass\s+an\s+interview\b|application\s+and/or\s+interview|admissions?\s+test|\bportfolio\b|\baudition|statement\s+of\s+interest|student\s+profile|admission\s+is\s+determined\s+by`)

	// An admission decided by a grade with a fallback (the CSB foci):
	// "Admission will be determined with a minimum grade of 80% in BIO130H1.
	// If the student does not achieve 80% in BIO130H1, admission can be
	// determined with a minimum grade of 80% in BIO230H1, BIO255H1 or
	// CSB349H1."
	enrolGradeFirstRe = regexp.MustCompile(`(?i)^admission\s+will\s+be\s+determined\s+with\s+a\s+minimum\s+grade\s+of\s+(\d{2})%\s+in\s+(.+)$`)
	enrolGradeElseRe  = regexp.MustCompile(`(?i)^if\s+the\s+student\s+does\s+not\s+achieve\s+\d{2}%\s+in\s+` + codePat + `,\s+admission\s+can\s+be\s+determined\s+with\s+a\s+minimum\s+grade\s+of\s+(\d{2})%\s+in\s+(.+)$`)

	// Timing rules the checker can't see: "Requests for admission will be
	// considered in the first program request period only.", "Requests for
	// admission will only be considered in the first program request period."
	enrolFirstPeriodRe = regexp.MustCompile(`(?i)will\s+(?:only\s+)?be\s+considered\s+in\s+the\s+first\s+program\s+request\s+period`)
	// "... provided the following courses ... are completed within 12 months
	// of beginning their studies", "failure to complete all the required
	// courses within 12 months (a student’s first Fall/Winter and Summer
	// sessions) ... will result in losing the admission guarantee".
	// "At least 20 spaces will be available each year for students applying
	// from Year 1 Computer Science (CMP1) within 12 months of beginning their
	// studies" is about reserved spaces, not when the courses are done.
	enrolWithinRe = regexp.MustCompile(`(?i)\bcomplete[ds]?\b[^.]*\bwithin\s+12\s+months`)
	// Deadlines for the application itself: "All course admission
	// requirements must be completed by May 1 to be eligible."
	enrolDeadlineRe = regexp.MustCompile(`(?i)must\s+be\s+completed\s+by\s+[A-Z][a-z]+\s+\d`)
	// "Have not yet completed CSC207H1 prior to beginning Focus requirements."
	enrolNotYetRe = regexp.MustCompile(`(?i)^have\s+not\s+(?:yet\s+)?completed\b`)

	// Programs that can't be combined with this one.
	enrolNotOpenRe      = regexp.MustCompile(`(?i)\bnot\s+open\s+to\s+students\s+enrolled\s+in\s+(?:a\s+|an\s+)?(.+)$`)
	enrolCombinedWithRe = regexp.MustCompile(`(?i)\bcannot\s+be\s+combined\s+with(?:\s+the)?\s*:?\s*(.*)$`)
	enrolCombineRe      = regexp.MustCompile(`(?i)\bcannot\s+combine\s+(?:a|an|the)\s+.+?\s+with\s+(.+?)(?:\s+for\s+their\s+degree)?$`)
	enrolSimultaneousRe = regexp.MustCompile(`(?i)\bcannot\s+be\s+simultaneously\s+enrolled\s+in\s+(?:or\s+complete\s+)?(.+)$`)
	// "another Sociology program (Specialist or Minor)" -> "Sociology
	// Specialist or Minor"; "either departmental Major programs (Biomedical
	// Toxicology or Pharmacology)" -> "Biomedical Toxicology Major",
	// "Pharmacology Major".
	enrolProgTypesParenRe = regexp.MustCompile(`(?i)^(?:another|any\s+other)\s+(.+?)\s+program\s+\(((?:Specialist|Major|Minor)(?:\s+or\s+(?:Specialist|Major|Minor))*)\)$`)
	enrolDeptParenRe      = regexp.MustCompile(`(?i)^(?:either\s+)?departmental\s+(Major|Specialist|Minor)\s+programs?\s+\((.+)\)$`)
	enrolProgSplitRe      = regexp.MustCompile(`(?i)\s*,?\s+or\s+the\s+|\s*,\s+the\s+|\s+or\s+(?:the\s+)?(?:Environmental|Biomedical)\b`)
	enrolProgArticleRe    = regexp.MustCompile(`(?i)^(?:the|a|an|either|any)\s+`)
	enrolTypeOnlyRe       = regexp.MustCompile(`^(?i:specialist|major|minor)$`)
	enrolAltSepRe         = regexp.MustCompile(`\s*,\s*(?:or\s+)?|\s+or\s+`)
	// A phrase about a subject or department, not named programs: "other
	// CIRHR program", "any Minor, Major, or Specialist offered by the
	// Department of French".
	exclusionIncludingRe = regexp.MustCompile(`(?i),\s+including\s+[^;]*`)
	exclusionPartsRe     = regexp.MustCompile(`\s*;\s*(?:nor\s+)?|\s+nor\s+`)
	enrolSubjectOnlyRe   = regexp.MustCompile(`(?i)^other\b|\boffered\s+by\b|\bdepartment\b`)
)

// kindSentence records an open/limited kind sentence and reports whether s
// was one.
func (p *enrolParser) kindSentence(s string) bool {
	m := enrolKindRe.FindStringSubmatch(s)
	if m == nil {
		return false
	}
	if p.kind == "" || p.kind == types.EnrolmentSuspended {
		p.kind = types.EnrolmentKind(strings.ToLower(m[1]))
	}
	if n := enrolNonRe.FindStringSubmatch(m[2]); n != nil {
		// "for any non-Rotman Commerce student": Link resolves the subject.
		p.exclusions = append(p.exclusions, &types.ReqNode{Type: types.NodeProgram, Text: n[1]})
	}
	return true
}

// thresholdSentence records the credits needed to enrol or apply.
func (p *enrolParser) thresholdSentence(s string) bool {
	m := enrolThresholdRe.FindStringSubmatch(s)
	if m == nil {
		return false
	}
	p.threshold, _ = strconv.ParseFloat(m[1], 64)
	if m[2] != "" { // "including MGT100H1 or equivalent"
		p.addPart(p.common(), p.parseFragmentNotes(m[2]), partOther)
	}
	return true
}

// specialSentence handles program enrolment, applications, timing rules and
// "have not yet completed". It reports whether s was one of them.
func (p *enrolParser) specialSentence(s string) bool {
	aud := p.cur
	if p.secKind == secNotes {
		aud = p.common() // a rule under "Notes" applies to everyone
	}
	switch {
	case enrolSelfEnrolRe.MatchString(s):
		m := enrolSelfEnrolRe.FindStringSubmatch(s)
		amount, _ := strconv.ParseFloat(m[1], 64)
		a := p.newAudience([]*types.ReqNode{{Type: types.NodeCredits, Amount: amount}})
		p.addPart(a, &types.ReqNode{Type: types.NodeProgram, Programs: []string{m[2]}, Text: "already enrolled in " + m[2]}, partOther)
		p.cur = aud // a route of its own; what follows doesn't belong to it
		p.notes = append(p.notes, s)
	case enrolInRe.MatchString(s):
		if p.kind == "" {
			p.kind = types.EnrolmentFocus
		}
		p.addPart(aud, programPhraseNode(enrolInRe.FindStringSubmatch(s)[1]), partOther)
	case enrolMustBeInRe.MatchString(s):
		p.addPart(aud, programPhraseNode(enrolMustBeInRe.FindStringSubmatch(s)[1]), partOther)
	case enrolOnceInRe.MatchString(s):
		p.kind = types.EnrolmentFocus
		p.addPart(aud, programPhraseNode(enrolOnceInRe.FindStringSubmatch(s)[1]), partOther)
	case enrolAdmittedToRe.MatchString(s):
		p.kind = types.EnrolmentFocus
		p.addPart(aud, programPhraseNode(enrolAdmittedToRe.FindStringSubmatch(s)[1]), partOther)
	case enrolOnlyRe.MatchString(s):
		m := enrolOnlyRe.FindStringSubmatch(s)
		p.addPart(aud, p.categoryNode(m[1]), partOther)
	case enrolFirstPeriodRe.MatchString(s):
		p.firstRound = true
		p.notes = append(p.notes, s)
	case enrolWithinRe.MatchString(s):
		p.addPart(aud, withinNode(), partOther)
		p.notes = append(p.notes, s)
	case enrolDeadlineRe.MatchString(s):
		p.notes = append(p.notes, s)
	case enrolGradeFirstRe.MatchString(s):
		m := enrolGradeFirstRe.FindStringSubmatch(s)
		p.chain = &part{node: gradedAlternatives(m[2], m[1]), sec: p.sec, kind: partList}
		aud.parts = append(aud.parts, p.chain)
	case enrolGradeElseRe.MatchString(s) && p.chain != nil:
		m := enrolGradeElseRe.FindStringSubmatch(s)
		p.chain.node = orNode(p.chain.node, gradedAlternatives(m[2], m[1]))
	case enrolAdmissionRe.MatchString(s):
		p.addPart(aud, &types.ReqNode{Type: types.NodeText, Kind: types.TextAdmission, Text: s}, partOther)
	case enrolNotYetRe.MatchString(s):
		p.addPart(aud, notYetNode(s), partOther)
	default:
		return false
	}
	return true
}

// withinNode is the condition on the admission guarantees: the courses of
// the route must be done in the student's first 12 months.
func withinNode() *types.ReqNode {
	return &types.ReqNode{Type: types.NodeProfile, Attr: "max_months_from_start", Value: "12",
		Text: "required courses completed within 12 months of beginning studies"}
}

// notYetNode reads "Have not yet completed CSC207H1 prior to beginning Focus
// requirements" as none of the courses named.
func notYetNode(s string) *types.ReqNode {
	var alts []*types.ReqNode
	for _, c := range codeRe.FindAllString(s, -1) {
		alts = append(alts, &types.ReqNode{Type: types.NodeCourse, Code: c})
	}
	if alts == nil {
		return &types.ReqNode{Type: types.NodeText, Kind: types.TextOther, Text: s}
	}
	return &types.ReqNode{Type: types.NodeNot, Items: []*types.ReqNode{orNode(alts...)}}
}

var (
	// "students admitted to Arts & Science in the Year 1 Computer Science
	// (CMP1) admission category": a code in brackets, dropped before reading
	// the name (the calendar gives one only for some categories).
	categoryCodeRe = regexp.MustCompile(`\([A-Z]{2,}\d*\)`)
	// "students admitted to the Rotman Commerce admission category in Year 1",
	// "students NOT admitted to the Year 1 Rotman Commerce admission
	// category": the capitalised words before "admission category" name the
	// category.
	categoryNameRe = regexp.MustCompile(`((?:[A-Z][\w&]*\s+)*[A-Z][\w&]*)\s+admissions?\s+categor`)
	categoryNotRe  = regexp.MustCompile(`\bNOT\b|\b(?i:other)\b`)
)

// categoryNode reads who an admission-category audience is for as a
// profile condition. "other ... admission categories" is every category but
// the last one named.
func (p *enrolParser) categoryNode(text string) *types.ReqNode {
	cat := ""
	if m := categoryNameRe.FindStringSubmatch(categoryCodeRe.ReplaceAllString(text, "")); m != nil {
		cat = strings.TrimSpace(strings.TrimPrefix(m[1], "Year 1 "))
		if strings.HasPrefix(cat, "NOT ") || cat == "Arts & Science Year 1" {
			cat = ""
		}
	}
	neg := categoryNotRe.MatchString(text)
	if cat == "" {
		cat = p.category
	}
	if cat == "" {
		return &types.ReqNode{Type: types.NodeText, Kind: types.TextMembership, Text: text}
	}
	p.category = cat
	n := &types.ReqNode{Type: types.NodeProfile, Attr: "admission_category", Value: cat, Text: text}
	if neg {
		return &types.ReqNode{Type: types.NodeNot, Items: []*types.ReqNode{n}}
	}
	return n
}

// exclusionSentence records programs the student must not be enrolled in.
// A sentence that only introduces a list ("the Biochemistry Major cannot be
// combined with the:") returns lead = true; the list items follow.
func (p *enrolParser) exclusionSentence(s string) (ok, lead bool) {
	nodes, ok, lead := exclusionNodes(s)
	p.exclusions = append(p.exclusions, nodes...)
	return ok, lead
}

// exclusionPhrase adds the programs named in phrase to Exclusions
// (exclusionPhraseNodes).
func (p *enrolParser) exclusionPhrase(phrase string) {
	p.exclusions = append(p.exclusions, exclusionPhraseNodes(phrase)...)
}

// exclusionNodes reads a sentence naming programs the student must not be
// enrolled in, as enrolment and completion text write it: "not open to
// students enrolled in ...", "cannot be combined with ...", "cannot combine
// the X with ...", "cannot be simultaneously enrolled in ...". ok reports
// whether s is one; lead, that it only introduces a list ("the Biochemistry
// Major cannot be combined with the:"), whose items follow.
func exclusionNodes(s string) (nodes []*types.ReqNode, ok, lead bool) {
	for _, re := range []*regexp.Regexp{enrolNotOpenRe, enrolCombinedWithRe, enrolCombineRe, enrolSimultaneousRe} {
		m := re.FindStringSubmatch(s)
		if m == nil {
			continue
		}
		phrase := strings.TrimSpace(strings.TrimRight(m[1], ".:"))
		if phrase == "" {
			return nil, true, true
		}
		return exclusionPhraseNodes(phrase), true, false
	}
	return nil, false, false
}

// exclusionPhraseNodes returns the programs named in phrase. Explicit
// alternatives ("the Biomedical Toxicology Major or the Pharmacology Major")
// become one node each. A phrase about a subject or a department ("any other
// CIRHR program", "any Minor, Major, or Specialist offered by the Department
// of French") stays membership text.
func exclusionPhraseNodes(phrase string) []*types.ReqNode {
	var out []*types.ReqNode
	phrase = enrolProgArticleRe.ReplaceAllString(phrase, "")
	// "any Computer Science, Statistics, or Actuarial Science programs,
	// including the Computer Science Minor, ...; nor any Mathematics Major or
	// Specialist program; nor the Focus in ...": each part on its own. What
	// "including" lists is already in the part before it.
	phrase = exclusionIncludingRe.ReplaceAllString(phrase, "")
	if parts := exclusionPartsRe.Split(phrase, -1); len(parts) > 1 {
		for _, part := range parts {
			out = append(out, exclusionPhraseNodes(part)...)
		}
		return out
	}
	if m := enrolDeptParenRe.FindStringSubmatch(phrase); m != nil {
		for _, name := range enrolAltSepRe.Split(m[2], -1) {
			out = append(out, &types.ReqNode{Type: types.NodeProgram, Text: name + " " + m[1]})
		}
		return out
	}
	for _, piece := range enrolProgSplitRe.Split(phrase, -1) {
		piece = enrolProgArticleRe.ReplaceAllString(strings.TrimSpace(piece), "")
		if m := enrolProgTypesParenRe.FindStringSubmatch(piece); m != nil {
			out = append(out, &types.ReqNode{Type: types.NodeProgram, Text: m[1] + " " + m[2]})
			continue
		}
		_, ok := parseProgramRef(piece)
		if enrolSubjectOnlyRe.MatchString(piece) || !ok && !strings.Contains(piece, "(") {
			// A subject or a department, not a list the program grammar can
			// read: "any other Economics program", "Psychology Specialist,
			// Research Specialist, Major, or Minor". Link resolves it by
			// subject area, or leaves it as text.
			out = append(out, &types.ReqNode{Type: types.NodeProgram, Text: piece})
			continue
		}
		out = append(out, programPhraseNode(piece))
	}
	return out
}

// programPhraseNode turns a phrase naming programs into program nodes:
// explicit codes go into Programs ("Computer Science Major Program
// (ASMAJ1689)"); names are left for Link to resolve ("History Specialist",
// "Psychology Minor, Major or Specialist"). A list that mixes subjects with
// bare types ("Computer Science Specialist or Major or Data Science
// Specialist") is split into one node per subject, OR'd.
func programPhraseNode(phrase string) *types.ReqNode {
	phrase = strings.TrimSpace(strings.TrimRight(phrase, ".,"))
	phrase = progSuffixRe.ReplaceAllString(phrase, "")
	if codes := progCodeRe.FindAllString(phrase, -1); codes != nil {
		return &types.ReqNode{Type: types.NodeProgram, Programs: dedupe(codes), Text: phrase}
	}
	if _, ok := parseProgramRef(phrase); ok || strings.Contains(phrase, "(") {
		// A bracket qualifies a title ("Anthropology Major (Society,
		// Culture, and Language)"); Link resolves it by title.
		return &types.ReqNode{Type: types.NodeProgram, Text: phrase}
	}
	var groups []string
	for _, piece := range enrolAltSepRe.Split(phrase, -1) {
		if enrolTypeOnlyRe.MatchString(piece) && len(groups) > 0 {
			groups[len(groups)-1] += " or " + piece
		} else {
			groups = append(groups, piece)
		}
	}
	var nodes []*types.ReqNode
	for _, g := range groups {
		nodes = append(nodes, &types.ReqNode{Type: types.NodeProgram, Text: g})
	}
	if len(nodes) == 1 {
		return nodes[0]
	}
	return &types.ReqNode{Type: types.NodeOr, Items: nodes}
}

// gradedAlternatives reads "BIO230H1, BIO255H1 or CSB349H1" with a grade as
// any one of the courses with that grade.
func gradedAlternatives(list, grade string) *types.ReqNode {
	g, _ := strconv.Atoi(grade)
	var alts []*types.ReqNode
	for _, c := range codeRe.FindAllString(list, -1) {
		alts = append(alts, &types.ReqNode{Type: types.NodeCourse, Code: c, MinGrade: g})
	}
	return orNode(alts...)
}

var (
	// "Applications from students lacking numerical grades in one or more of
	// the specified prerequisites, or students applying in their second or
	// later years, may be considered on the basis of their entire academic
	// record and the availability of space in the program."
	holisticRe = regexp.MustCompile(`(?i)\b(?:lacking|do\s+not\s+have)\s+numerical\s+grades\b.*\bsecond\s+or\s+later\s+years?\b.*\bentire\s+academic\s+record`)
	// "Students applying for admissions to the program utilizing transfer
	// credits will be considered on a case-by-case basis."
	transferRouteRe = regexp.MustCompile(`(?i)\butili[sz]ing\s+transfer\s+credits\s+will\s+be\s+considered\s+on\s+a\s+case[-\s]+by[-\s]+case\s+basis`)
	// "Students wishing to enroll after their second year who have taken
	// PCL201H1 will be considered on a case by case basis", "Students who
	// wish to enroll in the program after their second year will be
	// considered on a case-by-case basis."
	afterSecondRe = regexp.MustCompile(`(?i)\b(?:wishing|who\s+wish)\s+to\s+enrol+(?:\s+in\s+the\s+program)?\s+after\s+their\s+second\s+year(?:\s+who\s+have\s+taken\s+(` + codePat + `))?\s+will\s+be\s+considered\s+on\s+a\s+case[-\s]+by[-\s]+case\s+basis`)
	// "If students have not taken a course from this list, they can request
	// approval from the unit to consider other relevant courses for
	// admission."
	otherCoursesRe = regexp.MustCompile(`(?i)\bhave\s+not\s+taken\s+a\s+course\s+from\s+this\s+list\b.*\brequest\s+approval\b`)
	// "Students are invited to inquire with the program office about possible
	// appropriate substitutions for the courses above."
	substitutionsRe = regexp.MustCompile(`(?i)\binquire\b.*\bappropriate\s+substitutions\s+for\s+the\s+courses\s+above`)
	// "For students who are missing Gr. 12 Calculus and/or Biology,
	// information on these requirements and how they can be fulfilled is
	// provided in the Psychology section of the Calendar"
	missingHSRe = regexp.MustCompile(`(?i)\bmissing\s+Gr(?:ade|\.)\s*12\b.*\bhow\s+they\s+can\s+be\s+fulfilled`)
	// The Rotman Commerce guarantee: "Students may repeat any required
	// Guaranteed Admission course once", "Students who have attempted a
	// required Guaranteed Admission course twice and have not achieved the
	// minimum required grade will be ineligible".
	repeatOnceRe = regexp.MustCompile(`(?i)\brepeat\s+any\s+required\s+Guaranteed\s+Admission\s+course\s+once\b|\battempted\s+a\s+required\s+Guaranteed\s+Admission\s+course\s+twice\b`)
	// "Students who do not achieve the minimum required grade in MAT133Y1,
	// ... after two attempts within 12 months are not permitted to satisfy
	// the requirement by taking an alternative mathematics course." Two
	// attempts in total at the mathematics requirement, whichever courses
	// they were in: no switching to another mathematics course after two
	// failed attempts (max_attempts_per_requirement).
	noSwitchRe = regexp.MustCompile(`(?i)\bafter\s+two\s+attempts\b.*\bnot\s+permitted\s+to\s+satisfy\s+the\s+requirement\s+by\s+taking\s+an\s+alternative\b`)
	// "Note that none of the required courses may be completed using the
	// CR/NCR option."
	noCRRe = regexp.MustCompile(`(?i)\bnone\s+of\s+the\s+required\s+courses\s+may\s+be\s+completed\s+using\s+the\s+CR/NCR\s+option`)
	// "All course admission requirements must be completed by May 1 to be
	// eligible."
	completedByRe = regexp.MustCompile(`(?i)\bmust\s+be\s+completed\s+by\s+([A-Z][a-z]+\s+\d{1,2})\b`)
)

// hsAltText is the other way to meet a Grade 12 course.
const hsAltText = "met another way, as the Psychology section of the Calendar describes ('Regarding Psychology Programs')"

// routeSentence handles sentences that open another way in, or add a
// condition to the route they are in. It reports whether s was one.
func (p *enrolParser) routeSentence(s string) bool {
	profile := func(attr, value, text string) *types.ReqNode {
		return &types.ReqNode{Type: types.NodeProfile, Attr: attr, Value: value, Text: text}
	}
	admission := func(text string) *types.ReqNode {
		return &types.ReqNode{Type: types.NodeText, Kind: types.TextAdmission, Text: text}
	}
	switch {
	case holisticRe.MatchString(s):
		p.alts = append(p.alts, andNode(
			orNode(profile("has_missing_numeric_grades", "true", "lacking numerical grades in one or more of the specified prerequisites"),
				profile("min_year_of_study", "2", "applying in their second or later years")),
			admission("considered on the basis of their entire academic record")))
	case transferRouteRe.MatchString(s):
		p.alts = append(p.alts, andNode(profile("has_transfer_credits", "true", "applying utilizing transfer credits"),
			admission("considered on a case-by-case basis")))
	case afterSecondRe.MatchString(s):
		alt := andNode(profile("min_year_of_study", "3", "enrolling after their second year"), admission("considered on a case-by-case basis"))
		if c := afterSecondRe.FindStringSubmatch(s)[1]; c != "" {
			alt = andNode(alt, &types.ReqNode{Type: types.NodeCourse, Code: c})
		}
		p.alts = append(p.alts, alt)
	case substitutionsRe.MatchString(s):
		p.alts = append(p.alts, &types.ReqNode{Type: types.NodeText, Kind: types.TextPermission, Text: "an approved substitution for the required courses"})
	case otherCoursesRe.MatchString(s):
		p.closeList()
		pt := p.lastListPart()
		if pt == nil {
			return false
		}
		pt.node = orNode(pt.node, &types.ReqNode{Type: types.NodeText, Kind: types.TextPermission, Text: "the unit approves another relevant course"})
	case missingHSRe.MatchString(s):
		p.hsAlt = true
	case repeatOnceRe.MatchString(s):
		p.addPart(p.cur, profile("max_attempts_per_course", "2", "at most two attempts at each required course"), partOther)
	case noSwitchRe.MatchString(s):
		p.addPart(p.cur, profile("max_attempts_per_requirement", "2", "at most two attempts in total at the mathematics requirement: no switching to another mathematics course after two failed attempts"), partOther)
	case noCRRe.MatchString(s):
		p.addPart(p.cur, profile("max_cr_ncr", "0", "none of the required courses completed using the CR/NCR option"), partOther)
	case completedByRe.MatchString(s):
		p.addPart(p.cur, profile("complete_by", completedByRe.FindStringSubmatch(s)[1], s), partOther)
	default:
		return false
	}
	p.notes = append(p.notes, s)
	return true
}

// lastListPart is the last course requirement of the current route, or of
// everyone.
func (p *enrolParser) lastListPart() *part {
	for _, a := range []*audience{p.cur, p.common()} {
		for i := len(a.parts) - 1; i >= 0; i-- {
			if a.parts[i].kind == partList {
				return a.parts[i]
			}
		}
	}
	return nil
}

// replaceNodes replaces every node of n that match picks with f of it.
func replaceNodes(n *types.ReqNode, match func(*types.ReqNode) bool, f func(*types.ReqNode) *types.ReqNode) *types.ReqNode {
	if n == nil {
		return nil
	}
	if match(n) {
		return f(n)
	}
	for i, it := range n.Items {
		n.Items[i] = replaceNodes(it, match, f)
	}
	return n
}
