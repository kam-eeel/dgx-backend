package grammar

import (
	"regexp"
	"slices"
	"strings"

	"degree-explorer/types"
)

// Program requirements ("Specialist or Major in Drama"): parsing turns a phrase
// that only names programs into a program node, and ProgramIndex.Resolve fills
// in its codes. A wrong resolution is a false NotMet, worse than Review, so
// every rule here leaves text rather than guess.

const (
	ProgCodePat = `AS(?:SPE|MAJ|MIN|FOC|CER)\d{4}[A-Z]?\b`
	progVerbPat = `(?i:enrol(?:l)?(?:ed|ment)|registered|active)\s+in\s+` // "enrolled in", "Enrolment in"
)

// typeWordPat is a program type word; typesPat is a list of them: "Major,
// Specialist or Minor".
const (
	typeWordPat = `(?i:specialists?|majors?|minors?|certificates?|focus)\b`
	typesPat    = typeWordPat + `(?:(?:\s*,\s*(?i:or\s+|and\s+)?|\s*/\s*|\s+(?i:or|and)\s+)` + typeWordPat + `)*`

	// capitalized forms for phrases without a lead-in, which the lexer must not
	// confuse with ordinary words
	capTypePat  = `(?:Specialist|Major|Minor|Certificate)s?\b`
	capTypesPat = capTypePat + `(?:(?:\s*,\s*(?:or\s+)?|\s*/\s*|\s+or\s+)` + capTypePat + `)*`
	capNamePat  = `[A-Z][A-Za-z'-]*(?:\s+(?:(?:and|of)\s+|&\s+)?[A-Z][A-Za-z'-]*)*`
)

var (
	ProgCodeRe     = regexp.MustCompile(ProgCodePat)
	progCodeHeadRe = regexp.MustCompile(`^` + ProgCodePat)
	progParenRe    = regexp.MustCompile(`^\(\s*(?:` + ProgCodePat + `|(?i:formerly)\b)`)

	// ---- lexer ----

	// progLeadRe matches a lead-in: "registered in any one of". The empty group
	// marks an announced list ("... of the following programs:"), whose items may
	// be separated by ";".
	progLeadRe = regexp.MustCompile(`^` + progVerbPat +
		`(?:(?i:any\s+one|` + AtLeastPat + `\s+one)\s+of\s+(?:(?i:the\s+following\s+programs?)\s*:\s*())?)?`)
	// progTypesInNameRe matches "Specialist or Major in Drama".
	progTypesInNameRe = regexp.MustCompile(`^` + capTypesPat + `\s+in\s+(?:the\s+)?` + capNamePat)
	// progNameTypesRe matches two or more types after a name: "Drama Major,
	// Specialist or Minor".
	progNameTypesRe = regexp.MustCompile(`^` + capNamePat + `\s+` + capTypePat +
		`(?:(?:\s*,\s*(?:or\s+)?|\s*/\s*|\s+or\s+)` + capTypePat + `)+(?:\s+[Pp]rograms?\b)?`)

	// progStopWords end a program list in the lexer; progEndWords make what's
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
	progListRe      = regexp.MustCompile(`^(?i:(?:any\s+one|` + AtLeastPat + `\s+one)\s+of(?:\s+the\s+following(?:\s+programs?)?)?\s*:?\s*)`)
	progArticleRe   = regexp.MustCompile(`^(?i:(?:the|a|an|any)\s+)`)
	progAltSepRe    = regexp.MustCompile(`(?i)\s*,?\s+or\s+in\s+(?:(?:the|a|an|any)\s+)?`) // "the IR program or in a History major"
	ProgSuffixRe    = regexp.MustCompile(`(?i)\s+(?:programs?|programmes?|posts?)(?:\s+of\s+study)?$`)
	progCodeSepRe   = regexp.MustCompile(`(?i)\s*,\s*(?:or\s+)?|\s*/\s*|\s+or\s+`)
	progCodeRestRe  = regexp.MustCompile(`^[A-Za-z&'’,()\s-]*$`)
	progForm1Re     = regexp.MustCompile(`^(` + typesPat + `)(?:\s+(?i:programs?))?\s+(?i:in)\s+(?:(?i:the)\s+)?(.+)$`)
	progForm2Re     = regexp.MustCompile(`^(.+?)\s+(` + typesPat + `)$`)
	ProgPieceSepRe  = regexp.MustCompile(`(?i)\s*,\s*(?:or\s+)?|\s+or\s+`)
	progNameRe      = regexp.MustCompile(`^[A-Za-z][A-Za-z&'’,/\s-]*$`)
	typeWordRe      = regexp.MustCompile(typeWordPat)

	// progRejectRe matches phrases that aren't a plain "is in program X"
	// requirement, or name something that isn't a catalogue program.
	progRejectRe = regexp.MustCompile(`(?i)\b(?:non|not|except|unless|preference|priority|consideration|may|refer|website|` +
		`recommended|encouraged|years?|status|only|if|invitation|admission|admitted|accepted|guaranteed|requirements?|` +
		`streams?|faculty|college|school|campus|rotman|commerce|engineering|utm|utsc|courses?|credits?|completion|` +
		`completed|students?|who|have|permission|approval)\b`)
	progOnlyIfRe = regexp.MustCompile(`^(.+?)\s+(only\s+if\s+.+)$`)
	progOneRe    = regexp.MustCompile(`\b[A-Z]\w+\s+One\b`) // "Vic One"

	// progAfterStudentRe matches "student" or "status" before a bare "in the X
	// program", which is enrolment only there: "enrolled as a Year 4 student".
	progAfterStudentRe = regexp.MustCompile(`(?i)\b(?:students?|status)\s*$`)
	TitleTypeRes       = map[string]*regexp.Regexp{}
)

// ProgramTypes are the catalogue's program types, in the order used when a
// phrase doesn't name one.
var ProgramTypes = []string{"Specialist", "Major", "Minor", "Focus", "Certificate"}

func init() {
	for _, t := range ProgramTypes {
		TitleTypeRes[t] = regexp.MustCompile(`(?i)\b` + t + `\b`)
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

// matchProgram keeps a program reference together as one text token, so its
// commas, "or"s and "/"s aren't operators: "Drama Major, Specialist or
// Minor". ParseRequisite decides afterwards whether it is a program node.
func matchProgram(s string) (Token, bool) {
	if m := progLeadRe.FindStringSubmatchIndex(s); m != nil {
		if n := scanProgramList(s[m[1]:], m[2] >= 0); n > 0 {
			return Token{Kind: TText, End: m[1] + n}, true
		}
		return Token{}, false
	}
	for _, re := range []*regexp.Regexp{progTypesInNameRe, progNameTypesRe} {
		if loc := re.FindStringIndex(s); loc != nil {
			return Token{Kind: TText, End: loc[1]}, true
		}
	}
	return Token{}, false
}

// scanProgramList returns the length of the program list at the start of s, up
// to the last word that can end a reference. ";" separates items only in an
// announced list. It returns 0 if there is none or the brackets don't balance.
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

// nameRef is one subject name with the program types it asks for: "History or
// Political Science" + [Major Specialist]. No types means any type.
type nameRef struct {
	Name  string
	Types []string
}

type leadKind int

const (
	leadNone  leadKind = iota // "Drama Major, Specialist or Minor"
	leadIntro                 // "Enrolment in ...", "Limited to students enrolled in ..."
	leadIn                    // "in the X program"
)

// ProgramRef is what a program phrase says: explicit codes, or names to
// resolve against the catalogue.
type ProgramRef struct {
	codes  []string
	Names  []nameRef
	lead   leadKind
	strong bool // without a lead-in, still clearly a program: "Specialist or Major in Drama"
}

// refContext is what the parser knows about a phrase's surroundings.
type refContext struct {
	label        bool // a label ending in ":" ("Actuarial Science: ACT349H1")
	afterStudent bool // right after "student"/"status", so a bare "in the X program" is enrolment
}

// accepts reports whether a phrase shape is clearly a program requirement in ctx.
func (r ProgramRef) accepts(ctx refContext) bool {
	switch r.lead {
	case leadIntro:
		return true
	case leadIn:
		return ctx.afterStudent
	}
	return ctx.label || r.strong
}

// ParseProgramRef parses a phrase that does nothing but name programs. It
// fails on anything else: a qualifier, a non-catalogue program ("Vic One",
// Engineering), or a list it can't split cleanly.
func ParseProgramRef(text string) (ProgramRef, bool) {
	var ref ProgramRef
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

	if ProgCodeRe.MatchString(s) {
		ref.codes = parseCodeList(s)
		return ref, ref.codes != nil && ref.lead != leadNone
	}

	alts := progAltSepRe.Split(s, -1)
	if len(alts) > 1 && ref.lead == leadNone {
		return ref, false
	}
	for _, a := range alts {
		a = progArticleRe.ReplaceAllString(a, "")
		named := ProgSuffixRe.ReplaceAllString(a, "")
		names, strong, ok := ParseNameTypes(named)
		if !ok {
			return ref, false
		}
		ref.Names = append(ref.Names, names...)
		ref.strong = strong && len(alts) == 1
	}
	return ref, true
}

// parseCodeList returns the codes of a list where every item has a code:
// "ASMAJ1254/ASMIN1254". Items are separated by ";" if there are any, else by
// ",", "/" and "or".
func parseCodeList(s string) []string {
	items := strings.Split(s, ";")
	if len(items) == 1 {
		items = progCodeSepRe.Split(s, -1)
	}
	var codes []string
	for _, it := range items {
		cs := ProgCodeRe.FindAllString(it, -1)
		if cs == nil || !progCodeRestRe.MatchString(ProgCodeRe.ReplaceAllString(it, "")) {
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

// ParseNameTypes parses one alternative: "Specialist or Major in Drama",
// "Drama Major, Specialist or Minor", "Bioethics Specialist or Bioethics
// Major" or a bare name. strong is true for the first two shapes.
func ParseNameTypes(s string) (refs []nameRef, strong, ok bool) {
	if m := progForm1Re.FindStringSubmatch(s); m != nil {
		return []nameRef{{m[2], typeList(m[1])}}, true, validName(m[2])
	}
	if m := progForm2Re.FindStringSubmatch(s); m != nil {
		if validName(m[1]) {
			types := typeList(m[2])
			return []nameRef{{m[1], types}}, len(types) > 1, true
		}
		// "Bioethics Specialist or Bioethics Major": each piece has its own type
		for _, piece := range ProgPieceSepRe.Split(s, -1) {
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
		for _, t := range ProgramTypes {
			if lt := strings.ToLower(t); (w == lt || w == lt+"s") && !slices.Contains(out, t) {
				out = append(out, t)
			}
		}
	}
	return out
}

// ProgramNodes turns membership text that only names programs into program
// nodes, in place.
func (p *parser) ProgramNodes(n *types.ReqNode) *types.ReqNode {
	if n == nil {
		return nil
	}
	for _, it := range n.Items {
		p.ProgramNodes(it)
	}
	if n.Type != types.NodeText || n.Kind != types.TextMembership {
		return n
	}
	var ctx refContext
	if sp, ok := p.Spans[n]; ok {
		ctx.label = strings.HasSuffix(strings.TrimSpace(p.src[sp.Start:sp.End]), ":")
		ctx.afterStudent = progAfterStudentRe.MatchString(p.src[:sp.Start])
	}
	if ref, ok := ParseProgramRef(n.Text); ok && ref.accepts(ctx) {
		*n = types.ReqNode{Type: types.NodeProgram, Text: n.Text, Programs: ref.codes}
	} else if m := progOnlyIfRe.FindStringSubmatch(n.Text); m != nil {
		// "... only if enrolling in the Fall/Winter session": the program, or else
		// the condition may not apply
		if ref, ok := ParseProgramRef(m[1]); ok && ref.accepts(ctx) {
			*n = types.ReqNode{Type: types.NodeOr, Items: []*types.ReqNode{
				{Type: types.NodeProgram, Text: m[1], Programs: ref.codes},
				{Type: types.NodeText, Kind: types.TextOther, Text: m[2]},
			}}
		}
	}
	return n
}
