package reqs

import (
	"regexp"
	"slices"
	"strings"

	"degree-explorer/types"
)

// Linking a program's enrolment requirements: code repair against the course
// list, and program names resolved to codes against the programs table.

// enrolCodeFollowRe is what may follow a near-code in enrolment text: what
// may follow one in a requisite, or a grade ("MAT159 (55%)").
var enrolCodeFollowRe = regexp.MustCompile(`^\s*(?:$|[/,);.:\]]|(?i:or|and)\b|\(\s*\d{2}\s*%\s*\))`)

// repairEnrolment repairs catalogue-confirmed codes in the raw text and
// parses it again, keeping Raw.
func repairEnrolment(e *types.Enrolment, known map[string]bool) *types.Enrolment {
	if e == nil {
		return nil
	}
	fixed, ok := repairCodesFollowedBy(e.Raw, known, enrolCodeFollowRe)
	if !ok {
		return e
	}
	r := ParseEnrolment(fixed)
	r.Raw = e.Raw
	return r
}

// titleIndex finds programs by their exact title, without the "(Arts
// Program)" / "(Science Program)" suffix: "Anthropology Major (General)" is
// ASMAJ1775, which subject matching can't tell from the other Anthropology
// Majors.
type titleIndex map[string]string // normalized title -> code ("" when two programs share it)

var titleSuffixRe = regexp.MustCompile(`(?i)\s*\((?:arts|science)\s+program\)\s*$|\s*\(BCom\)\s*$`)

func newTitleIndex(programs []*types.Program) titleIndex {
	idx := titleIndex{}
	for _, p := range programs {
		key := normSubject(titleSuffixRe.ReplaceAllString(p.Title, ""))
		if _, dup := idx[key]; dup {
			idx[key] = ""
			continue
		}
		idx[key] = p.Code
	}
	return idx
}

// exact is the program whose title is the whole phrase, or "".
func (idx titleIndex) exact(phrase string) string {
	phrase = progSuffixRe.ReplaceAllString(enrolPhraseLeadRe.ReplaceAllString(strings.TrimSpace(phrase), ""), "")
	return idx[normSubject(phrase)]
}

var enrolPhraseLeadRe = regexp.MustCompile(`(?i)^(?:(?:students?\s+)?(?:must\s+be\s+)?(?:enrol(?:l)?(?:ed|ment)|registered|active)\s+in\s+)?(?:(?:the|a|an)\s+)?`)

// lookupTitle resolves a phrase naming programs by title: the whole phrase
// ("Anthropology Specialist (Society, Culture, and Language)"), or each
// subject and type it lists ("Psychology Minor, Major or Specialist" is the
// Psychology Minor, Major and Specialist). Every program named must exist.
func (idx titleIndex) lookupTitle(phrase string) ([]string, bool) {
	phrase = progSuffixRe.ReplaceAllString(enrolPhraseLeadRe.ReplaceAllString(strings.TrimSpace(phrase), ""), "")
	if code := idx.exact(phrase); code != "" {
		return []string{code}, true
	}
	names, _, ok := parseNameTypes(phrase)
	if !ok {
		return nil, false
	}
	var codes []string
	for _, nr := range names {
		if len(nr.types) == 0 {
			return nil, false
		}
		for _, t := range nr.types {
			code := idx[normSubject(nr.name+" "+t)]
			if code == "" {
				return nil, false
			}
			codes = append(codes, code)
		}
	}
	return codes, len(codes) > 0
}

// resolveEnrolment fills in the programs of every program node in e by
// title, then by subject (ProgramIndex), then by subject area. A node none
// resolves stays membership text. Exclusions never list the program itself
// ("Students cannot combine a Pharmacology Major program with a Pharmacology
// Major or Specialist program") or a program req requires.
func (idx titleIndex) resolveEnrolment(e *types.Enrolment, self string, progs *ProgramIndex, areas areaIndex) {
	if e == nil {
		return
	}
	idx.resolveNode(e.Req, progs, areas)
	// A program req asks for is never also excluded: "a student must first be
	// enrolled in the Economics Major (ASMAJ1478)" with "cannot be
	// simultaneously enrolled in any other Economics program" means the
	// Specialist replaces the Major, not that Majors can't apply.
	required := requiredPrograms(e.Req)
	var keep []*types.ReqNode
	for _, n := range e.Exclusions {
		idx.resolveNode(n, progs, areas)
		if n.Type == types.NodeText && withinRe.MatchString(n.Text) {
			// "the Focus in Data Science in Business within the Rotman
			// Commerce specialist programs" names a program the calendar no
			// longer has, so no student can be in it.
			e.Notes = append(e.Notes, "Not in the calendar: "+n.Text)
			continue
		}
		if n.Type == types.NodeProgram {
			// A restriction on a subject covers all its programs, including
			// ones whose title doesn't name it: "any Computer Science,
			// Statistics, or Actuarial Science programs" includes the
			// "Specialist in Statistical Science: Methods and Practice". An
			// exact title ("Chemistry Specialist") is that program only.
			if idx.exact(n.Text) == "" {
				if codes, ok := areas.lookup(n.Text); ok {
					n.Programs = dedupe(append(n.Programs, codes...))
				}
			}
			n.Programs = slices.DeleteFunc(n.Programs, func(c string) bool { return c == self || required[c] })
			if len(n.Programs) == 0 {
				continue
			}
		}
		keep = append(keep, n)
	}
	e.Exclusions = keep
}

func (idx titleIndex) resolveNode(n *types.ReqNode, progs *ProgramIndex, areas areaIndex) {
	if n == nil {
		return
	}
	for _, it := range n.Items {
		idx.resolveNode(it, progs, areas)
	}
	if n.Type != types.NodeProgram || len(n.Programs) > 0 {
		return
	}
	if codes, ok := idx.lookupTitle(n.Text); ok {
		n.Programs = codes
		return
	}
	if progs != nil {
		if ref, ok := parseProgramRef(n.Text); ok {
			if codes, ok := progs.lookupRef(ref); ok {
				n.Programs = codes
				return
			}
		}
	}
	if codes, ok := idx.lookupWithin(n.Text); ok {
		n.Programs = codes
		return
	}
	if codes, ok := areas.lookup(n.Text); ok {
		n.Programs = codes
		return
	}
	n.Type, n.Kind = types.NodeText, types.TextMembership
}

// focusParentRe is the parent a focus's title names: "Focus in Technology
// Leadership (Computer Science Major)".
var focusParentRe = regexp.MustCompile(`\(([^()]+)\)\s*$`)

// narrowToParent keeps, of the programs a focus's req accepts, only the
// parent its title names. The three Technology Leadership foci share one
// sentence ("admitted to a Computer Science Specialist or Major or Data
// Science Specialist program"), but each belongs to one of those programs.
func (idx titleIndex) narrowToParent(n *types.ReqNode, title string) *types.ReqNode {
	m := focusParentRe.FindStringSubmatch(title)
	if m == nil {
		return n
	}
	parent, ok := idx.lookupTitle(m[1])
	if !ok {
		return n
	}
	in := func(c string) bool { return slices.Contains(parent, c) }
	var narrow func(n *types.ReqNode) *types.ReqNode
	narrow = func(n *types.ReqNode) *types.ReqNode {
		if n == nil {
			return nil
		}
		switch n.Type {
		case types.NodeProgram:
			if slices.ContainsFunc(n.Programs, in) {
				n.Programs = slices.DeleteFunc(n.Programs, func(c string) bool { return !in(c) })
			}
		case types.NodeOr:
			// Drop the alternatives that name only other programs.
			if slices.ContainsFunc(n.Items, func(it *types.ReqNode) bool {
				return it.Type == types.NodeProgram && slices.ContainsFunc(it.Programs, in)
			}) {
				n.Items = slices.DeleteFunc(n.Items, func(it *types.ReqNode) bool {
					return it.Type == types.NodeProgram && !slices.ContainsFunc(it.Programs, in)
				})
			}
			fallthrough
		default:
			for i, it := range n.Items {
				n.Items[i] = narrow(it)
			}
		}
		return tidy(n)
	}
	return narrow(n)
}

// withinRe reads "the Focus in Data Analytics within the Economics Major or
// Specialist": the focus's variants for those programs, titled "Focus in
// Data Analytics (Major)" and "Focus in Data Analytics (Specialist)".
var withinRe = regexp.MustCompile(`(?i)^(?:the\s+)?(focus\s+in\s+.+?)\s+within\s+the\s+.+?\s+((?:major|specialist)(?:\s+or\s+(?:major|specialist))?)(?:\s+programs?)?$`)

func (idx titleIndex) lookupWithin(phrase string) ([]string, bool) {
	m := withinRe.FindStringSubmatch(strings.TrimSpace(phrase))
	if m == nil {
		return nil, false
	}
	var codes []string
	for _, t := range regexp.MustCompile(`(?i)\s+or\s+`).Split(m[2], -1) {
		code := idx[normSubject(m[1]+" "+t)]
		if code == "" {
			return nil, false
		}
		codes = append(codes, code)
	}
	return codes, true
}

// areaIndex finds programs by subject area (the areas column), for
// restrictions that name a subject rather than programs: "not open to
// students enrolled in a Specialist, Major, or Minor in German", "any other
// Economics program", "any Minor, Major, or Specialist offered by the
// Department of French". A program belongs to its first area only: the
// Environment & Behaviour Minor lists Psychology second and isn't a
// Psychology program.
type areaIndex map[string][]*types.Program

// areaAliases are other names the calendar uses for an area.
var areaAliases = map[string]string{
	"statistics": "statistical sciences",
	"cirhr":      "industrial relations and human resources",
}

func newAreaIndex(programs []*types.Program) areaIndex {
	idx := areaIndex{}
	for _, p := range programs {
		if len(p.Areas) > 0 {
			k := normSubject(p.Areas[0])
			idx[k] = append(idx[k], p)
		}
	}
	return idx
}

var (
	areaLeadRe  = regexp.MustCompile(`(?i)^(?:any\s+other|another|other|any|all)\s+`)
	areaTypeRe  = regexp.MustCompile(`(?i)\b(?:research\s+)?(specialist|major|minor|certificate|focus)(?:es|s)?\b`)
	areaNoiseRe = regexp.MustCompile(`(?i)\boffered\s+by\s+the\s+department\s+of\b|\b(?:programs?|in|the)\b`)
	sepEdgesRe  = regexp.MustCompile(`(?i)^(?:[\s,/]|\bor\b|\band\b)+|(?:[\s,/]|\bor\b|\band\b)+$`)
	areaSepRe   = regexp.MustCompile(`(?i)\s*(?:,|/|\bor\b|\band\b)\s*`)
)

// lookup resolves a phrase naming subjects and, optionally, program types.
// With no types it means the subjects' programs, not their focuses or
// certificates. Every subject must be an area.
func (idx areaIndex) lookup(phrase string) ([]string, bool) {
	phrase = areaLeadRe.ReplaceAllString(strings.TrimSpace(phrase), "")
	want := map[string]bool{}
	for _, m := range areaTypeRe.FindAllStringSubmatch(phrase, -1) {
		want[strings.ToLower(m[1])] = true
	}
	rest := areaNoiseRe.ReplaceAllString(areaTypeRe.ReplaceAllString(phrase, " "), " ")
	subjects := areaSepRe.Split(rest, -1)
	if whole := normSubject(sepEdgesRe.ReplaceAllString(rest, "")); idx[whole] != nil {
		subjects = []string{whole} // "Slavic and East European Languages and Cultures"
	}
	var codes []string
	for _, subject := range subjects {
		k := normSubject(subject)
		if k == "" {
			continue
		}
		if alias, ok := areaAliases[k]; ok {
			k = alias
		}
		progs, ok := idx[k]
		if !ok {
			return nil, false
		}
		for _, p := range progs {
			t := strings.ToLower(p.Type)
			if len(want) > 0 && !want[t] || len(want) == 0 && (t == "focus" || t == "certificate") {
				continue
			}
			codes = append(codes, p.Code)
		}
	}
	return codes, len(codes) > 0
}

// requiredPrograms lists the programs n requires: those its program nodes
// name, outside a not.
func requiredPrograms(n *types.ReqNode) map[string]bool {
	required := map[string]bool{}
	var walk func(n *types.ReqNode)
	walk = func(n *types.ReqNode) {
		if n == nil || n.Type == types.NodeNot {
			return
		}
		for _, c := range n.Programs {
			required[c] = true
		}
		for _, it := range n.Items {
			walk(it)
		}
	}
	walk(n)
	return required
}

// ambiguousExclusions splits an Exclusions subject phrase with program types
// ("Specialist, Major, or Minor in German", "Pharmacology Major or
// Specialist") into the programs it surely names, those whose title names
// the subject, and those it may also cover: the area's other programs of
// those types (the Al and Malka Green Yiddish Program Minor is a German-area
// minor), and the area's programs of those types whose title names the
// subject without being the exact title (the Specialist in Pharmacology and
// Biomedical Toxicology). Being in one of the latter is Review.
func (idx titleIndex) ambiguousExclusions(e *types.Enrolment, self string, programs []*types.Program, required map[string]bool, areas areaIndex) {
	byCode := map[string]*types.Program{}
	for _, p := range programs {
		byCode[p.Code] = p
	}
	listed := map[string]bool{} // programs an Exclusions node already names
	for _, n := range e.Exclusions {
		for _, c := range n.Programs {
			listed[c] = true
		}
	}
	for _, n := range e.Exclusions {
		if n.Type != types.NodeProgram {
			continue
		}
		subject, kinds := exclusionSubject(n.Text, areas)
		if subject == "" || idx.exact(n.Text) != "" { // "Chemistry Specialist" is that program
			continue
		}
		names := func(p *types.Program) bool {
			return strings.Contains(" "+normSubject(p.Title)+" ", " "+subject+" ")
		}
		var sure, maybe []string
		for _, code := range n.Programs {
			p := byCode[code]
			if p == nil || names(p) {
				sure = append(sure, code)
			} else {
				maybe = append(maybe, code)
			}
		}
		if len(sure) == 0 {
			continue
		}
		// Programs of those types anywhere whose title names the subject:
		// "Economics & Mathematics Specialist" for "any Mathematics Major or
		// Specialist program".
		for _, p := range programs {
			if !kinds[strings.ToLower(p.Type)] || !names(p) || p.Code == self || required[p.Code] ||
				slices.Contains(sure, p.Code) || listed[p.Code] {
				continue
			}
			maybe = append(maybe, p.Code)
		}
		if len(maybe) == 0 {
			continue
		}
		n.Programs = sure
		var titles []string
		for _, code := range maybe {
			titles = append(titles, titleSuffixRe.ReplaceAllString(byCode[code].Title, ""))
		}
		e.Req = andNode(e.Req, orNode(
			&types.ReqNode{Type: types.NodeNot, Items: []*types.ReqNode{{Type: types.NodeProgram, Programs: dedupe(maybe), Text: n.Text}}},
			ambiguousText(`"`+n.Text+`" may also cover the `+strings.Join(titles, ", the "))))
	}
}

// exclusionSubject reads a phrase naming one subject and program types ("a
// Specialist, Major, or Minor in German"): the subject, normalized, and the
// types. It returns "" for any other phrase: several subjects, a
// department, or no types.
func exclusionSubject(phrase string, areas areaIndex) (string, map[string]bool) {
	if strings.Contains(strings.ToLower(phrase), "department") {
		return "", nil
	}
	kinds := map[string]bool{}
	for _, m := range areaTypeRe.FindAllStringSubmatch(phrase, -1) {
		kinds[strings.ToLower(m[1])] = true
	}
	if len(kinds) == 0 {
		if !programWordRe.MatchString(phrase) {
			return "", nil
		}
		// "any other Economics program": its specialists, majors and minors.
		kinds = map[string]bool{"specialist": true, "major": true, "minor": true}
	}
	rest := areaNoiseRe.ReplaceAllString(areaTypeRe.ReplaceAllString(areaLeadRe.ReplaceAllString(phrase, ""), " "), " ")
	if whole := normSubject(sepEdgesRe.ReplaceAllString(rest, "")); areas[whole] != nil {
		return whole, kinds
	}
	var subjects []string
	for _, s := range areaSepRe.Split(rest, -1) {
		if s = normSubject(s); s != "" {
			subjects = append(subjects, s)
		}
	}
	if len(subjects) != 1 {
		return "", nil
	}
	if _, alias := areaAliases[subjects[0]]; alias {
		return "", nil
	}
	return subjects[0], kinds
}

// programWordRe: "any other Economics program".
var programWordRe = regexp.MustCompile(`(?i)\bprograms?\b`)

// oneFocusRe: "Students can only apply for one focus."
var oneFocusRe = regexp.MustCompile(`(?i)\bcan\s+only\s+apply\s+for\s+one\s+focus\b`)

// oneFocus reads "Students can only apply for one focus" both ways: a
// student in a sibling focus (ASMAJ1003B for ASMAJ1003A) is Review, as it
// may mean they can't apply.
func oneFocus(e *types.Enrolment, self string, programs []*types.Program) {
	if !oneFocusRe.MatchString(e.Raw) || len(self) != 10 {
		return
	}
	var siblings []string
	for _, p := range programs {
		if len(p.Code) == 10 && p.Code[:9] == self[:9] && p.Code != self {
			siblings = append(siblings, p.Code)
		}
	}
	if siblings == nil {
		return
	}
	slices.Sort(siblings)
	e.Req = andNode(e.Req, orNode(
		&types.ReqNode{Type: types.NodeNot, Items: []*types.ReqNode{{Type: types.NodeProgram, Programs: siblings, Text: "another focus of this program"}}},
		ambiguousText(`"Students can only apply for one focus" may mean a student already in another focus can't apply`)))
}
