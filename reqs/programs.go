package reqs

import (
	"regexp"
	"slices"
	"strings"

	"degree-explorer/types"
)

// Program requirements ("Enrolment in ASMAJ1618", "Specialist or Major in
// Drama", "Limited to students enrolled in the Bioethics Specialist or
// Bioethics Major programs") become program nodes in two steps:
//
//  1. Parsing stays pure: a program-kind phrase that does nothing but name
//     programs becomes a program node that keeps the phrase in Text. Explicit
//     codes go straight into Programs.
//  2. ResolvePrograms fills in Programs by matching the subject names and
//     program types in Text against the scraped catalogue. A node it can't
//     resolve with confidence goes back to program text (Review), so a
//     resolved tree never has a program node without Programs.
//
// A program node is Met when the student is in any of its Programs and NotMet
// otherwise. A wrong resolution therefore produces a false NotMet, which is
// worse than Review, so every rule here prefers leaving text over guessing.

const (
	progCodePat = `AS(?:SPE|MAJ|MIN|FOC|CER)\d{4}[A-Z]?\b`
	progVerbPat = `(?i:enrol(?:l)?(?:ed|ment)|registered|active)\s+in\s+` // "enrolled in", "Enrolment in"
)

// typeWordPat is a program type word; typesPat is a list of them:
// "Major, Specialist or Minor", "major or specialist".
const (
	typeWordPat = `(?i:specialists?|majors?|minors?|certificates?|focus)\b`
	typesPat    = typeWordPat + `(?:(?:\s*,\s*(?i:or\s+|and\s+)?|\s*/\s*|\s+(?i:or|and)\s+)` + typeWordPat + `)*`

	// Capitalized forms for phrases without a lead-in, which the lexer must
	// not confuse with ordinary words.
	capTypePat  = `(?:Specialist|Major|Minor|Certificate)s?\b`
	capTypesPat = capTypePat + `(?:(?:\s*,\s*(?:or\s+)?|\s*/\s*|\s+or\s+)` + capTypePat + `)*`
	capNamePat  = `[A-Z][A-Za-z'-]*(?:\s+(?:(?:and|of)\s+|&\s+)?[A-Z][A-Za-z'-]*)*`
)

var (
	progCodeRe     = regexp.MustCompile(progCodePat)
	progCodeHeadRe = regexp.MustCompile(`^` + progCodePat)
	progParenRe    = regexp.MustCompile(`^\(\s*(?:` + progCodePat + `|(?i:formerly)\b)`)

	// ---- lexer ----

	// "enrolled in", "Enrolment in", "registered in any one of",
	// "enrolled in at least one of the following programs:". The empty group
	// marks an announced list, whose items may be separated by ";".
	progLeadRe = regexp.MustCompile(`^` + progVerbPat +
		`(?:(?i:any\s+one|at\s+least\s+one)\s+of\s+(?:(?i:the\s+following\s+programs?)\s*:\s*())?)?`)
	// "Specialist or Major in Drama", "Certificate in Sustainability"
	progTypesInNameRe = regexp.MustCompile(`^` + capTypesPat + `\s+in\s+(?:the\s+)?` + capNamePat)
	// "Drama Major, Specialist or Minor" (two or more types)
	progNameTypesRe = regexp.MustCompile(`^` + capNamePat + `\s+` + capTypePat +
		`(?:(?:\s*,\s*(?:or\s+)?|\s*/\s*|\s+or\s+)` + capTypePat + `)+(?:\s+[Pp]rograms?\b)?`)

	// Words that end a program list in the lexer, and words that make what's
	// before them a complete program reference.
	progStopWords = wordSet(`permission approval consent instructor department departmental coordinator
		who which that this with have has having must will should may only if by plus including
		completion completed complete year years student students course courses credit credits
		grade minimum prior before after is are be at for from to than`)
	progEndWords = wordSet(`specialist specialists major majors minor minors certificate certificates
		focus program programs programme programmes post posts study`)

	// ---- phrase grammar ----

	progTrailWithRe = regexp.MustCompile(`(?i)\s+with$`) // "Major in Drama with" (a minimum of 70% in ...)
	progIntroRe     = regexp.MustCompile(`^(?i:(?:students\s+)?must\s+be\s+)?` + progVerbPat)
	progRestrictRe  = regexp.MustCompile(`^(?i:(?:enrol(?:l)?ment\s+(?:is\s+)?)?(?:open\s+only|only\s+open|limited|restricted)\s+to\s+students\s+(?:(?:enrol(?:l)?ed|registered)\s+)?in\s+)`)
	progBareInRe    = regexp.MustCompile(`^(?i:in\s+)`)
	progForRe       = regexp.MustCompile(`^(?i:for\s+)`)
	progListRe      = regexp.MustCompile(`^(?i:(?:any\s+one|at\s+least\s+one)\s+of(?:\s+the\s+following(?:\s+programs?)?)?\s*:?\s*)`)
	progArticleRe   = regexp.MustCompile(`^(?i:(?:the|a|an|any)\s+)`)
	progAltSepRe    = regexp.MustCompile(`(?i)\s*,?\s+or\s+in\s+(?:(?:the|a|an|any)\s+)?`) // "the IR program or in a History major"
	progSuffixRe    = regexp.MustCompile(`(?i)\s+(?:programs?|programmes?|posts?)(?:\s+of\s+study)?$`)
	progCodeSepRe   = regexp.MustCompile(`(?i)\s*,\s*(?:or\s+)?|\s*/\s*|\s+or\s+`)
	progCodeRestRe  = regexp.MustCompile(`^[A-Za-z&'’,()\s-]*$`)
	progForm1Re     = regexp.MustCompile(`^(` + typesPat + `)(?:\s+(?i:programs?))?\s+(?i:in)\s+(?:(?i:the)\s+)?(.+)$`)
	progForm2Re     = regexp.MustCompile(`^(.+?)\s+(` + typesPat + `)$`)
	progPieceSepRe  = regexp.MustCompile(`(?i)\s*,\s*(?:or\s+)?|\s+or\s+`)
	progNameRe      = regexp.MustCompile(`^[A-Za-z][A-Za-z&'’,/\s-]*$`)
	typeWordRe      = regexp.MustCompile(typeWordPat)

	// Phrases that aren't a plain "is in program X" requirement, or name
	// something that isn't a catalogue program.
	progRejectRe = regexp.MustCompile(`(?i)\b(?:non|not|except|unless|preference|priority|consideration|may|refer|website|` +
		`recommended|encouraged|years?|status|only|if|invitation|admission|admitted|accepted|guaranteed|requirements?|` +
		`streams?|faculty|college|school|campus|rotman|commerce|engineering|utm|utsc|courses?|credits?|completion|` +
		`completed|students?|who|have|permission|approval)\b`)
	progOnlyIfRe = regexp.MustCompile(`^(.+?)\s+(only\s+if\s+.+)$`)
	progOneRe    = regexp.MustCompile(`\b[A-Z]\w+\s+One\b`) // "Vic One"

	// A bare "in the X program" is enrolment only right after "student" or
	// "status": "enrolled as a Year 4 student in the Pharmaceutical Chemistry
	// program", not "1.0 additional credit in the Digital Humanities Minor
	// program".
	progAfterStudentRe = regexp.MustCompile(`(?i)\b(?:students?|status)\s*$`)

	// ---- titles ----

	trailingParenRe = regexp.MustCompile(`\s*\([^()]*\)\s*$`)
	titleInRe       = regexp.MustCompile(`^\s*(?i:programs?\s+)?(?i:in)\s+(?:(?i:the)\s+)?`)
	nonAlnumRe      = regexp.MustCompile(`[^a-z0-9]+`)
	titleTypeRes    = map[string]*regexp.Regexp{}
)

// programTypes are the catalogue's program types, in the order used when a
// phrase doesn't name one.
var programTypes = []string{"Specialist", "Major", "Minor", "Focus", "Certificate"}

func init() {
	for _, t := range programTypes {
		titleTypeRes[t] = regexp.MustCompile(`(?i)\b` + t + `\b`)
	}
}

func wordSet(s string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(s) {
		m[w] = true
	}
	return m
}

// ---- lexer ----

// matchProgram keeps a program reference together as one text token, so the
// commas, "or"s and "/"s inside it aren't read as operators: "enrolled in the
// MST minor, major, or Specialist program", "registered in any one of
// ASMAJ0365, ASSPE0365", "Drama Major, Specialist or Minor". The token is
// plain text; ParseRequisite decides afterwards whether it is a program node.
func matchProgram(s string) (token, bool) {
	if m := progLeadRe.FindStringSubmatchIndex(s); m != nil {
		if n := scanProgramList(s[m[1]:], m[2] >= 0); n > 0 {
			return token{kind: tText, end: m[1] + n}, true
		}
		return token{}, false
	}
	for _, re := range []*regexp.Regexp{progTypesInNameRe, progNameTypesRe} {
		if loc := re.FindStringIndex(s); loc != nil {
			return token{kind: tText, end: loc[1]}, true
		}
	}
	return token{}, false
}

// scanProgramList returns the length of the program list at the start of s:
// words, ",", "/", program codes and bracketed codes, up to the last word that
// can end a program reference ("Major", "program", a code). It stops at a
// stop word, a digit or punctuation; ";" separates items only in an announced
// list. It returns 0 if there is no such word or the brackets don't balance.
func scanProgramList(s string, semis bool) int {
	end, last, depth, endDepth := 0, 0, 0, 0
	for i := 0; i < len(s); {
		if loc := progCodeHeadRe.FindStringIndex(s[i:]); loc != nil {
			i += loc[1]
			end, last, endDepth = i, i, depth
			continue
		}
		c := s[i]
		switch {
		case c == ' ' || c == ',' || c == '/' || c == ';' && semis:
			i++
			continue
		case c == '(' && progParenRe.MatchString(s[i:]):
			depth++
			i++
		case c == ')' && depth > 0:
			depth--
			i++
			if end == last { // ")" right after the end of a reference
				end, endDepth = i, depth
			}
		case isWordByte(c):
			j := i
			for j < len(s) && isWordByte(s[j]) {
				j++
			}
			w := strings.ToLower(s[i:j])
			if progStopWords[w] {
				return balanced(end, endDepth)
			}
			i = j
			if progEndWords[w] {
				end, endDepth = i, depth
			}
		default:
			return balanced(end, endDepth)
		}
		last = i
	}
	return balanced(end, endDepth)
}

func balanced(end, depth int) int {
	if depth != 0 {
		return 0
	}
	return end
}

func isWordByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '&' || c == '\'' || c == '-'
}

// ---- phrase grammar ----

// nameRef is one subject name with the program types it asks for: "History
// or Political Science" + [Major Specialist]. No types means any type.
type nameRef struct {
	name  string
	types []string
}

type leadKind int

const (
	leadNone  leadKind = iota // "Drama Major, Specialist or Minor"
	leadIntro                 // "Enrolment in ...", "Limited to students enrolled in ..."
	leadIn                    // "in the X program"
)

// programRef is what a program phrase says: explicit codes, or names to
// resolve against the catalogue.
type programRef struct {
	codes  []string
	names  []nameRef
	lead   leadKind
	strong bool // without a lead-in, still clearly a program: "Specialist or Major in Drama"
}

// refContext is what the parser knows about a phrase's surroundings.
type refContext struct {
	label        bool // a label ending in ":" ("Actuarial Science: ACT349H1")
	afterStudent bool // right after "student"/"status", so a bare "in the X program" is enrolment
}

// accepts reports whether a phrase shape is clearly a program requirement in ctx.
func (r programRef) accepts(ctx refContext) bool {
	switch r.lead {
	case leadIntro:
		return true
	case leadIn:
		return ctx.afterStudent
	}
	return ctx.label || r.strong
}

// parseProgramRef parses a phrase that does nothing but name programs. It
// fails on anything else, e.g. a qualifier ("only if enrolling in the
// Fall/Winter session"), a non-catalogue program (Rotman Commerce, Vic One,
// Engineering, "admission to ...") or a list it can't split cleanly.
func parseProgramRef(text string) (programRef, bool) {
	var ref programRef
	s := progTrailWithRe.ReplaceAllString(strings.TrimSpace(text), "")
	switch {
	case progIntroRe.MatchString(s):
		s, ref.lead = s[len(progIntroRe.FindString(s)):], leadIntro
	case progRestrictRe.MatchString(s):
		s, ref.lead = s[len(progRestrictRe.FindString(s)):], leadIntro
	case progBareInRe.MatchString(s):
		s, ref.lead = s[len(progBareInRe.FindString(s)):], leadIn
	default:
		s = progForRe.ReplaceAllString(s, "") // label "For Drama Majors"
	}
	if ref.lead != leadNone {
		s = progListRe.ReplaceAllString(s, "")
	}
	s = progArticleRe.ReplaceAllString(s, "")
	if s == "" || progRejectRe.MatchString(s) || progOneRe.MatchString(s) {
		return ref, false
	}

	if progCodeRe.MatchString(s) {
		ref.codes = parseCodeList(s)
		return ref, ref.codes != nil && ref.lead != leadNone
	}

	alts := progAltSepRe.Split(s, -1)
	if len(alts) > 1 && ref.lead == leadNone {
		return ref, false
	}
	for _, a := range alts {
		a = progArticleRe.ReplaceAllString(a, "")
		named := progSuffixRe.ReplaceAllString(a, "")
		names, strong, ok := parseNameTypes(named)
		if !ok {
			return ref, false
		}
		ref.names = append(ref.names, names...)
		ref.strong = strong && len(alts) == 1
	}
	return ref, true
}

// parseCodeList returns the codes of a list where every item has a code:
// "ASMAJ1254/ASMIN1254", "American Studies Major (ASMAJ0135); ...". Items are
// separated by ";" if there are any, else by "," "/" and "or".
func parseCodeList(s string) []string {
	items := strings.Split(s, ";")
	if len(items) == 1 {
		items = progCodeSepRe.Split(s, -1)
	}
	var codes []string
	for _, it := range items {
		cs := progCodeRe.FindAllString(it, -1)
		if cs == nil || !progCodeRestRe.MatchString(progCodeRe.ReplaceAllString(it, "")) {
			return nil
		}
		for _, c := range cs {
			if !slices.Contains(codes, c) {
				codes = append(codes, c)
			}
		}
	}
	return codes
}

// parseNameTypes parses one alternative: "Specialist or Major in Drama",
// "Drama Major, Specialist or Minor", "Bioethics Specialist or Bioethics
// Major" or a bare name ("Sociology"). strong is true for the first two
// shapes, which are unmistakable even without a lead-in.
func parseNameTypes(s string) (refs []nameRef, strong, ok bool) {
	if m := progForm1Re.FindStringSubmatch(s); m != nil {
		return []nameRef{{m[2], typeList(m[1])}}, true, validName(m[2])
	}
	if m := progForm2Re.FindStringSubmatch(s); m != nil {
		if validName(m[1]) {
			types := typeList(m[2])
			return []nameRef{{m[1], types}}, len(types) > 1, true
		}
		// "Bioethics Specialist or Bioethics Major": each piece has its own type.
		for _, piece := range progPieceSepRe.Split(s, -1) {
			pm := progForm2Re.FindStringSubmatch(piece)
			if pm == nil || !validName(pm[1]) {
				return nil, false, false
			}
			refs = append(refs, nameRef{pm[1], typeList(pm[2])})
		}
		return refs, true, true
	}
	return []nameRef{{s, nil}}, false, validName(s)
}

func validName(s string) bool {
	return progNameRe.MatchString(s) && !typeWordRe.MatchString(s) && len(strings.Fields(s)) <= 8
}

// typeList turns "major or specialist" into [Major Specialist].
func typeList(s string) []string {
	var out []string
	for _, w := range typeWordRe.FindAllString(s, -1) {
		w = strings.ToLower(w)
		for _, t := range programTypes {
			if lt := strings.ToLower(t); (w == lt || w == lt+"s") && !slices.Contains(out, t) {
				out = append(out, t)
			}
		}
	}
	return out
}

// programNodes turns program text that only names programs into program
// nodes, in place.
func (p *parser) programNodes(n *types.ReqNode) *types.ReqNode {
	if n == nil {
		return nil
	}
	for _, it := range n.Items {
		p.programNodes(it)
	}
	if n.Type != types.NodeText || n.Kind != types.KindProgram {
		return n
	}
	var ctx refContext
	if sp, ok := p.spans[n]; ok {
		ctx.label = strings.HasSuffix(strings.TrimSpace(p.src[sp.start:sp.end]), ":")
		ctx.afterStudent = progAfterStudentRe.MatchString(p.src[:sp.start])
	}
	if ref, ok := parseProgramRef(n.Text); ok && ref.accepts(ctx) {
		*n = types.ReqNode{Type: types.NodeProgram, Text: n.Text, Programs: ref.codes}
	} else if m := progOnlyIfRe.FindStringSubmatch(n.Text); m != nil {
		// "Active in ASMAJ1618 only if enrolling in the Fall/Winter
		// session": the program, or else the condition may not apply.
		if ref, ok := parseProgramRef(m[1]); ok && ref.accepts(ctx) {
			*n = types.ReqNode{Type: types.NodeOr, Items: []*types.ReqNode{
				{Type: types.NodeProgram, Text: m[1], Programs: ref.codes},
				{Type: types.NodeText, Kind: types.KindOther, Text: m[2]},
			}}
		}
	}
	return n
}

// ---- resolution ----

// ProgramIndex looks catalogue programs up by subject and type. Build it once
// per scrape with NewProgramIndex.
type ProgramIndex struct {
	bySubject map[string]map[string][]string // normalized subject → type → codes
}

// NewProgramIndex indexes programs by the subject in their title:
// "Cognitive Science Major - Arts (Thinking and Reasoning Stream) (Arts
// Program)" is subject "cognitive science", type Major.
func NewProgramIndex(programs []*types.Program) *ProgramIndex {
	idx := &ProgramIndex{bySubject: map[string]map[string][]string{}}
	for _, p := range programs {
		subj := normSubject(titleSubject(p.Title, p.Type))
		if subj == "" || p.Type == "" {
			continue
		}
		if idx.bySubject[subj] == nil {
			idx.bySubject[subj] = map[string][]string{}
		}
		idx.bySubject[subj][p.Type] = append(idx.bySubject[subj][p.Type], p.Code)
	}
	for _, byType := range idx.bySubject {
		for _, codes := range byType {
			slices.Sort(codes)
		}
	}
	return idx
}

// ResolvePrograms fills in Programs for every program node in r from the
// catalogue, and turns nodes it can't resolve back into program text.
func ResolvePrograms(r *types.Requisite, programs []*types.Program) {
	NewProgramIndex(programs).Resolve(r)
}

// Resolve is ResolvePrograms with a prebuilt index. r may be nil.
func (idx *ProgramIndex) Resolve(r *types.Requisite) {
	if r != nil {
		idx.resolveNode(r.Req)
	}
}

func (idx *ProgramIndex) resolveNode(n *types.ReqNode) {
	if n == nil {
		return
	}
	for _, it := range n.Items {
		idx.resolveNode(it)
	}
	if n.Type != types.NodeProgram || len(n.Programs) > 0 {
		return
	}
	if ref, ok := parseProgramRef(n.Text); ok {
		if codes, ok := idx.lookupRef(ref); ok {
			n.Programs = codes
			return
		}
	}
	n.Type, n.Kind = types.NodeText, types.KindProgram
}

// lookupRef resolves every name in ref; if any name fails, the whole phrase
// fails, since a partial list would turn the missing programs into NotMet.
func (idx *ProgramIndex) lookupRef(ref programRef) ([]string, bool) {
	var codes []string
	for _, nr := range ref.names {
		c, ok := idx.lookup(nr.name, nr.types)
		if !ok { // a list of names: "History or Political Science major"
			parts := progPieceSepRe.Split(nr.name, -1)
			if len(parts) < 2 {
				return nil, false
			}
			for _, part := range parts {
				pc, ok := idx.lookup(part, nr.types)
				if !ok {
					return nil, false
				}
				c = append(c, pc...)
			}
		}
		for _, code := range c {
			if !slices.Contains(codes, code) {
				codes = append(codes, code)
			}
		}
	}
	return codes, len(codes) > 0
}

// lookup finds the programs of one subject. It refuses when the name is also
// the start of another subject the phrase might mean too:
//   - with no type, any longer subject: "a Psychology program" could be the
//     Psychology Research Specialist, "a Music program" Music History & Culture;
//   - with types, a longer subject of one of those types that isn't a
//     separate "X and Y" subject: "Psychology Specialist" could be the
//     Psychology Research Specialist, but "History Major" isn't the History
//     and Philosophy of Science and Technology Major.
//
// Every type asked for must exist.
func (idx *ProgramIndex) lookup(name string, types []string) ([]string, bool) {
	key := normSubject(name)
	byType, ok := idx.bySubject[key]
	if !ok {
		return nil, false
	}
	for other, otherTypes := range idx.bySubject {
		rest, ok := strings.CutPrefix(other, key+" ")
		if !ok {
			continue
		}
		if len(types) == 0 ||
			!strings.HasPrefix(rest, "and ") && slices.ContainsFunc(types, func(t string) bool { return len(otherTypes[t]) > 0 }) {
			return nil, false
		}
	}
	var codes []string
	if len(types) == 0 {
		for _, t := range programTypes {
			codes = append(codes, byType[t]...)
		}
		return codes, len(codes) > 0
	}
	for _, t := range types {
		if len(byType[t]) == 0 {
			return nil, false
		}
		codes = append(codes, byType[t]...)
	}
	return codes, true
}

// titleSubject extracts the subject from a program title, dropping the type
// word and variants: trailing brackets ("(Science Program)", "(General)",
// "(Thinking and Reasoning Stream)"), streams and foci after the type word
// ("Major- Stream in Creative Writing", "Specialist: Genetics Stream") and
// ": ..." after a leading type ("Specialist in Statistical Science: Theory
// and Methods").
func titleSubject(title, typ string) string {
	t := title
	for {
		u := trailingParenRe.ReplaceAllString(t, "")
		if u == t {
			break
		}
		t = u
	}
	subj := t
	if re := titleTypeRes[typ]; re != nil {
		if loc := re.FindStringIndex(t); loc != nil {
			if loc[0] == 0 {
				subj = titleInRe.ReplaceAllString(t[loc[1]:], "") // "Major in Public Policy"
			} else {
				subj = t[:loc[0]]
			}
		}
	}
	if i := strings.Index(subj, ":"); i >= 0 {
		subj = subj[:i]
	}
	return strings.Trim(subj, " -:")
}

// normSubject makes names comparable: "Peace, Conflict and Justice" and
// "peace conflict and justice", "Cell & Molecular Biology" and "Cell and
// Molecular Biology".
func normSubject(s string) string {
	s = strings.ReplaceAll(strings.ToLower(s), "&", " and ")
	s = strings.TrimSpace(nonAlnumRe.ReplaceAllString(s, " "))
	return strings.TrimPrefix(s, "the ")
}
