package enrolment

import (
	"regexp"
	"strconv"
	"strings"

	"degree-explorer/parser/grammar"
	"degree-explorer/parser/programs"
	"degree-explorer/types"
)

// Sentences giving the program's kind, the credits needed, and requirements
// that aren't course lists: program enrolment, applications, timing, exclusions.

var (
	// nonRe matches "for any non-Rotman Commerce student".
	nonRe = regexp.MustCompile(`(?i)\bfor\s+any\s+non-(.+?)\s+students?\b`)
	// suspendedRe matches "has been administratively suspended ... and is no
	// longer admitting students".
	suspendedRe = regexp.MustCompile(`(?i)administratively\s+suspended|no\s+longer\s+admitting|no\s+new\s+students\s+will\s+be\s+admitted`)

	// thresholdRe matches "A student who has completed 4.0 credits may enrol
	// in the program."
	thresholdRe = regexp.MustCompile(`(?i)^(?:students\s+must\s+have\s+completed|a\s+student\s+who\s+has\s+completed)\s+(?:a\s+minimum\s+of\s+)?` + grammar.AmountPat + `\s+credits?\b(?:\s+including\s+(.+?)\s+may\s+enrol\b)?`)

	// variableRe matches a cutoff that changes each year: "this minimum may
	// change".
	variableRe = regexp.MustCompile(`(?i)\bchanges?\s+each\s+year|\bmay\s+(?:change|vary)\b|\bvar(?:y|ies)\s+(?:each\s+year|year\s+to\s+year)|\bdepend\s+each\s+year`)
	// variableScopeRe matches a variable cutoff naming its courses: "A
	// minimum grade average in STA130H1 and ... is needed for entry."
	variableScopeRe = regexp.MustCompile(`(?i)^(?:a\s+)?minimum\s+grade(?:\s+average)?s?\s+in\s+.*` + grammar.CodePat + `.*\s+(?:is|are)\s+needed\s+for\s+entry$`)

	// inRe matches program enrolment: "Enrolment in the History Specialist
	// is required."
	inRe = regexp.MustCompile(`(?i)^enrol(?:l)?ment\s+in\s+(?:the\s+)?(.+?)(?:\s+is\s+required)?(?:\s+for\s+entry\s+into\s+(?:the|this)\s+focus)?$`)
	// mustBeInRe matches "Students must be enrolled in the Psychology Minor,
	// Major or Specialist, to be considered for enrolment."
	mustBeInRe = regexp.MustCompile(`(?i)^(?:in\s+order\s+to\s+apply\s+for\s+.+?,\s+)?(?:a\s+)?students?\s+must\s+(?:first\s+)?be\s+enrolled\s+in\s+(?:the\s+)?(.+?)(?:,?\s+to\s+be\s+considered\s+for\s+enrolment)?$`)
	// onceInRe matches the parent of an application-based focus: "Once you
	// have enrolled in the Cell and Molecular Biology Major program, you have the
	// option to apply for entry into a focus".
	onceInRe = regexp.MustCompile(`(?i)^once\s+you\s+have\s+(?:been\s+approved\s+for\s+and\s+have\s+)?enrolled\s+in\s+the\s+(.+?)(?:\s+program)?,\s+you\s+have\s+the\s+option\s+to\s+apply\s+for\s+entry\s+into\s+a\s+focus`)
	// admittedToRe matches "and are admitted to a Computer Science Specialist
	// or Major ... program will be considered for admission".
	admittedToRe = regexp.MustCompile(`(?i)\band\s+are\s+admitted\s+to\s+an?\s+(.+?)\s+program\s+will\s+be\s+considered\s+for\s+admission`)
	// selfEnrolRe matches "Students who have completed at least 8.0 credits
	// and who are already enrolled in ... may enrol in ... themselves on ACORN."
	selfEnrolRe = regexp.MustCompile(`(?i)^students\s+who\s+have\s+completed\s+(?:` + grammar.AtLeastPat + `\s+)?(\d+\.\d+)\s+credits\s+and\s+who\s+are\s+already\s+enrolled\s+in\s+(` + grammar.ProgCodePat + `)\s+may\s+enrol\s+in\s+` + grammar.ProgCodePat + `\s+themselves`)
	// onlyRe matches a route for one admission category only: "Only students
	// in the Year 1 Computer Science admission category (CMP1) who meet the
	// criteria ... are eligible to apply".
	onlyRe = regexp.MustCompile(`(?i)^only\s+(students\s+in\s+the\s+.+?)\s+(?:who\s+meet\s+.+?\s+)?are\s+eligible\s+to\s+apply`)

	// admissionRe matches an admission the department decides: "Students must
	// pass an interview".
	admissionRe = regexp.MustCompile(`(?i)supplementa(?:l|ry)\s+application|\bpass\s+an\s+interview\b|application\s+and/or\s+interview|admissions?\s+test|\bportfolio\b|\baudition|statement\s+of\s+interest|student\s+profile|admission\s+is\s+determined\s+by`)

	// gradeFirstRe and gradeElseRe match a grade with a fallback:
	// "Admission will be determined with a minimum grade of 80% in BIO130H1. If
	// the student does not achieve 80% in BIO130H1, ..."
	gradeFirstRe = regexp.MustCompile(`(?i)^admission\s+will\s+be\s+determined\s+with\s+a\s+minimum\s+grade\s+of\s+(\d{2})%\s+in\s+(.+)$`)
	gradeElseRe  = regexp.MustCompile(`(?i)^if\s+the\s+student\s+does\s+not\s+achieve\s+\d{2}%\s+in\s+` + grammar.CodePat + `,\s+admission\s+can\s+be\s+determined\s+with\s+a\s+minimum\s+grade\s+of\s+(\d{2})%\s+in\s+(.+)$`)

	// firstPeriodRe matches a timing rule the checker can't see: "will only
	// be considered in the first program request period".
	firstPeriodRe = regexp.MustCompile(`(?i)will\s+(?:only\s+)?be\s+considered\s+in\s+the\s+first\s+program\s+request\s+period`)
	// withinRe matches "are completed within 12 months of beginning their
	// studies". It needs "complete": reserved spaces "within 12 months" don't count.
	withinRe = regexp.MustCompile(`(?i)\bcomplete[ds]?\b[^.]*\bwithin\s+12\s+months`)
	// deadlineRe matches an application deadline: "must be completed by May
	// 1".
	deadlineRe = regexp.MustCompile(`(?i)must\s+be\s+completed\s+by\s+[A-Z][a-z]+\s+\d`)
	// notYetRe matches "Have not yet completed CSC207H1 prior to beginning
	// Focus requirements."
	notYetRe = regexp.MustCompile(`(?i)^have\s+not\s+(?:yet\s+)?completed\b`)
)

// kindSentence records an open/limited kind sentence and reports whether s was
// one.
func (p *parser) kindSentence(s string) bool {
	m := programs.KindRe.FindStringSubmatch(s)
	if m == nil {
		return false
	}
	if p.kind == "" || p.kind == types.EnrolmentSuspended {
		p.kind = types.EnrolmentKind(strings.ToLower(m[1]))
	}
	if n := nonRe.FindStringSubmatch(m[2]); n != nil {
		// Link resolves the subject
		p.exclusions = append(p.exclusions, &types.ReqNode{Type: types.NodeProgram, Text: n[1]})
	}
	return true
}

// thresholdSentence records the credits needed to enrol or apply.
func (p *parser) thresholdSentence(s string) bool {
	m := thresholdRe.FindStringSubmatch(s)
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
func (p *parser) specialSentence(s string) bool {
	aud := p.cur
	if p.secKind == secNotes {
		aud = p.common() // a rule under "Notes" applies to everyone
	}
	switch {
	case selfEnrolRe.MatchString(s):
		m := selfEnrolRe.FindStringSubmatch(s)
		amount, _ := strconv.ParseFloat(m[1], 64)
		a := p.newAudience([]*types.ReqNode{{Type: types.NodeCredits, Amount: amount}})
		p.addPart(a, &types.ReqNode{Type: types.NodeProgram, Programs: []string{m[2]}, Text: "already enrolled in " + m[2]}, partOther)
		p.cur = aud // a route of its own; what follows doesn't belong to it
		p.notes = append(p.notes, s)
	case inRe.MatchString(s):
		if p.kind == "" {
			p.kind = types.EnrolmentFocus
		}
		p.addPart(aud, programs.ProgramPhraseNode(inRe.FindStringSubmatch(s)[1]), partOther)
	case mustBeInRe.MatchString(s):
		p.addPart(aud, programs.ProgramPhraseNode(mustBeInRe.FindStringSubmatch(s)[1]), partOther)
	case onceInRe.MatchString(s):
		p.kind = types.EnrolmentFocus
		p.addPart(aud, programs.ProgramPhraseNode(onceInRe.FindStringSubmatch(s)[1]), partOther)
	case admittedToRe.MatchString(s):
		p.kind = types.EnrolmentFocus
		p.addPart(aud, programs.ProgramPhraseNode(admittedToRe.FindStringSubmatch(s)[1]), partOther)
	case onlyRe.MatchString(s):
		m := onlyRe.FindStringSubmatch(s)
		p.addPart(aud, p.categoryNode(m[1]), partOther)
	case firstPeriodRe.MatchString(s):
		p.firstRound = true
		p.notes = append(p.notes, s)
	case withinRe.MatchString(s):
		p.addPart(aud, withinNode(), partOther)
		p.notes = append(p.notes, s)
	case deadlineRe.MatchString(s):
		p.notes = append(p.notes, s)
	case gradeFirstRe.MatchString(s):
		m := gradeFirstRe.FindStringSubmatch(s)
		p.chain = &part{node: gradedAlternatives(m[2], m[1]), sec: p.sec, kind: partList}
		aud.parts = append(aud.parts, p.chain)
	case gradeElseRe.MatchString(s) && p.chain != nil:
		m := gradeElseRe.FindStringSubmatch(s)
		p.chain.node = programs.OrNode(p.chain.node, gradedAlternatives(m[2], m[1]))
	case admissionRe.MatchString(s):
		p.addPart(aud, &types.ReqNode{Type: types.NodeText, Kind: types.TextAdmission, Text: s}, partOther)
	case notYetRe.MatchString(s):
		p.addPart(aud, notYetNode(s), partOther)
	default:
		return false
	}
	return true
}

// withinNode is the condition on the admission guarantees: the route's courses
// done in the student's first 12 months.
func withinNode() *types.ReqNode {
	return &types.ReqNode{Type: types.NodeProfile, Attr: "max_months_from_start", Value: "12",
		Text: "required courses completed within 12 months of beginning studies"}
}

// notYetNode reads "Have not yet completed CSC207H1 prior to beginning Focus
// requirements" as none of the courses named.
func notYetNode(s string) *types.ReqNode {
	alts := programs.CourseNodes(grammar.CodeRe.FindAllString(s, -1))
	if alts == nil {
		return &types.ReqNode{Type: types.NodeText, Kind: types.TextOther, Text: s}
	}
	return &types.ReqNode{Type: types.NodeNot, Items: []*types.ReqNode{programs.OrNode(alts...)}}
}

var (
	// categoryCodeRe matches a category's code in brackets, dropped before reading
	// its name: "(CMP1)".
	categoryCodeRe = regexp.MustCompile(`\([A-Z]{2,}\d*\)`)
	// categoryNameRe matches the capitalised words naming a category before
	// "admission category": "the Year 1 Rotman Commerce admission category".
	categoryNameRe = regexp.MustCompile(`((?:[A-Z][\w&]*\s+)*[A-Z][\w&]*)\s+admissions?\s+categor`)
	categoryNotRe  = regexp.MustCompile(`\bNOT\b|\b(?i:other)\b`)
)

// categoryNode reads who an admission-category audience is for as a profile
// condition. "other ... admission categories" is every category but the last
// one named.
func (p *parser) categoryNode(text string) *types.ReqNode {
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

// exclusionSentence records programs the student must not be enrolled in. lead
// is true for a sentence that only introduces a list; its items follow.
func (p *parser) exclusionSentence(s string) (ok, lead bool) {
	nodes, ok, lead := programs.ExclusionNodes(s)
	p.exclusions = append(p.exclusions, nodes...)
	return ok, lead
}

// exclusionPhrase adds the programs named in phrase to Exclusions.
func (p *parser) exclusionPhrase(phrase string) {
	p.exclusions = append(p.exclusions, programs.ExclusionPhraseNodes(phrase)...)
}

// gradedAlternatives reads "BIO230H1, BIO255H1 or CSB349H1" with a grade as
// any one of the courses with that grade.
func gradedAlternatives(list, grade string) *types.ReqNode {
	g, _ := strconv.Atoi(grade)
	var alts []*types.ReqNode
	for _, c := range grammar.CodeRe.FindAllString(list, -1) {
		alts = append(alts, &types.ReqNode{Type: types.NodeCourse, Code: c, MinGrade: g})
	}
	return programs.OrNode(alts...)
}

var (
	// holisticRe matches "lacking numerical grades ... second or later years, may
	// be considered on the basis of their entire academic record".
	holisticRe = regexp.MustCompile(`(?i)\b(?:lacking|do\s+not\s+have)\s+numerical\s+grades\b.*\bsecond\s+or\s+later\s+years?\b.*\bentire\s+academic\s+record`)
	// transferRouteRe matches "utilizing transfer credits will be considered on a
	// case-by-case basis".
	transferRouteRe = regexp.MustCompile(`(?i)\butili[sz]ing\s+transfer\s+credits\s+will\s+be\s+considered\s+on\s+a\s+case[-\s]+by[-\s]+case\s+basis`)
	// afterSecondRe matches "Students who wish to enroll in the program after
	// their second year will be considered on a case-by-case basis."
	afterSecondRe = regexp.MustCompile(`(?i)\b(?:wishing|who\s+wish)\s+to\s+enrol+(?:\s+in\s+the\s+program)?\s+after\s+their\s+second\s+year(?:\s+who\s+have\s+taken\s+(` + grammar.CodePat + `))?\s+will\s+be\s+considered\s+on\s+a\s+case[-\s]+by[-\s]+case\s+basis`)
	// otherCoursesRe matches "If students have not taken a course from this list,
	// they can request approval".
	otherCoursesRe = regexp.MustCompile(`(?i)\bhave\s+not\s+taken\s+a\s+course\s+from\s+this\s+list\b.*\brequest\s+approval\b`)
	// substitutionsRe matches "inquire with the program office about possible
	// appropriate substitutions for the courses above".
	substitutionsRe = regexp.MustCompile(`(?i)\binquire\b.*\bappropriate\s+substitutions\s+for\s+the\s+courses\s+above`)
	// missingHSRe matches "missing Gr. 12 Calculus and/or Biology, information on
	// these requirements and how they can be fulfilled".
	missingHSRe = regexp.MustCompile(`(?i)\bmissing\s+Gr(?:ade|\.)\s*12\b.*\bhow\s+they\s+can\s+be\s+fulfilled`)
	// repeatOnceRe matches "Students may repeat any required Guaranteed Admission
	// course once".
	repeatOnceRe = regexp.MustCompile(`(?i)\brepeat\s+any\s+required\s+Guaranteed\s+Admission\s+course\s+once\b|\battempted\s+a\s+required\s+Guaranteed\s+Admission\s+course\s+twice\b`)
	// noSwitchRe matches "after two attempts within 12 months are not permitted to
	// satisfy the requirement by taking an alternative": two attempts in total.
	noSwitchRe = regexp.MustCompile(`(?i)\bafter\s+two\s+attempts\b.*\bnot\s+permitted\s+to\s+satisfy\s+the\s+requirement\s+by\s+taking\s+an\s+alternative\b`)
	// noCRRe matches "none of the required courses may be completed using the
	// CR/NCR option".
	noCRRe = regexp.MustCompile(`(?i)\bnone\s+of\s+the\s+required\s+courses\s+may\s+be\s+completed\s+using\s+the\s+CR/NCR\s+option`)
	// completedByRe matches "must be completed by May 1".
	completedByRe = regexp.MustCompile(`(?i)\bmust\s+be\s+completed\s+by\s+([A-Z][a-z]+\s+\d{1,2})\b`)
)

// hsAltText is the other way to meet a Grade 12 course.
const hsAltText = "met another way, as the Psychology section of the Calendar describes ('Regarding Psychology Programs')"

// routeSentence handles sentences that open another way in, or add a
// condition to the route they are in. It reports whether s was one.
func (p *parser) routeSentence(s string) bool {
	profile := func(attr, value, text string) *types.ReqNode {
		return &types.ReqNode{Type: types.NodeProfile, Attr: attr, Value: value, Text: text}
	}
	admission := func(text string) *types.ReqNode {
		return &types.ReqNode{Type: types.NodeText, Kind: types.TextAdmission, Text: text}
	}
	switch {
	case holisticRe.MatchString(s):
		p.alts = append(p.alts, programs.AndNode(
			programs.OrNode(profile("has_missing_numeric_grades", "true", "lacking numerical grades in one or more of the specified prerequisites"),
				profile("min_year_of_study", "2", "applying in their second or later years")),
			admission("considered on the basis of their entire academic record")))
	case transferRouteRe.MatchString(s):
		p.alts = append(p.alts, programs.AndNode(profile("has_transfer_credits", "true", "applying utilizing transfer credits"),
			admission("considered on a case-by-case basis")))
	case afterSecondRe.MatchString(s):
		alt := programs.AndNode(profile("min_year_of_study", "3", "enrolling after their second year"), admission("considered on a case-by-case basis"))
		if c := afterSecondRe.FindStringSubmatch(s)[1]; c != "" {
			alt = programs.AndNode(alt, &types.ReqNode{Type: types.NodeCourse, Code: c})
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
		pt.node = programs.OrNode(pt.node, &types.ReqNode{Type: types.NodeText, Kind: types.TextPermission, Text: "the unit approves another relevant course"})
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
func (p *parser) lastListPart() *part {
	for _, a := range []*audience{p.cur, p.common()} {
		for i := len(a.parts) - 1; i >= 0; i-- {
			if a.parts[i].kind == partList {
				return a.parts[i]
			}
		}
	}
	return nil
}
