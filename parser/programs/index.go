package programs

import (
	"regexp"
	"slices"
	"strings"

	"degree-explorer/parser/grammar"
	"degree-explorer/types"
)

// Program lookup by exact title and by subject area, for the programs that
// enrolment text names.

// TitleIndex finds programs by exact title, without the "(Arts Program)"
// suffix: "Anthropology Major (General)", which subject matching can't tell
// from the other Anthropology Majors.
type TitleIndex map[string]string // normalized title -> code ("" when two programs share it)

var TitleSuffixRe = regexp.MustCompile(`(?i)\s*\((?:arts|science)\s+program\)\s*$|\s*\(BCom\)\s*$`)

func NewTitleIndex(programs []*types.Program) TitleIndex {
	idx := TitleIndex{}
	for _, p := range programs {
		key := NormSubject(TitleSuffixRe.ReplaceAllString(p.Title, ""))
		if _, dup := idx[key]; dup {
			idx[key] = ""
			continue
		}
		idx[key] = p.Code
	}
	return idx
}

// exact is the program whose title is the whole phrase, or "".
func (idx TitleIndex) Exact(phrase string) string {
	phrase = grammar.ProgSuffixRe.ReplaceAllString(enrolPhraseLeadRe.ReplaceAllString(strings.TrimSpace(phrase), ""), "")
	return idx[NormSubject(phrase)]
}

var enrolPhraseLeadRe = regexp.MustCompile(`(?i)^(?:(?:students?\s+)?(?:must\s+be\s+)?(?:enrol(?:l)?(?:ed|ment)|registered|active)\s+in\s+)?(?:(?:the|a|an)\s+)?`)

// LookupTitle resolves a phrase naming programs by title: the whole phrase, or
// each subject and type it lists ("Psychology Minor, Major or Specialist").
// Every program named must exist.
func (idx TitleIndex) LookupTitle(phrase string) ([]string, bool) {
	phrase = grammar.ProgSuffixRe.ReplaceAllString(enrolPhraseLeadRe.ReplaceAllString(strings.TrimSpace(phrase), ""), "")
	if code := idx.Exact(phrase); code != "" {
		return []string{code}, true
	}
	names, _, ok := grammar.ParseNameTypes(phrase)
	if !ok {
		return nil, false
	}
	var codes []string
	for _, nr := range names {
		if len(nr.Types) == 0 {
			return nil, false
		}
		for _, t := range nr.Types {
			code := idx[NormSubject(nr.Name+" "+t)]
			if code == "" {
				return nil, false
			}
			codes = append(codes, code)
		}
	}
	return codes, len(codes) > 0
}

// ResolveEnrolment resolves e's program nodes by title, then subject, then
// subject area; one none resolves stays membership text. Exclusions never list
// the program itself or a program req requires.
func (idx TitleIndex) ResolveEnrolment(e *types.Enrolment, self string, progs *ProgramIndex, areas AreaIndex) {
	if e == nil {
		return
	}
	idx.ResolveNode(e.Req, progs, areas)
	// a required program is never also excluded: "any other Economics program"
	// beside a required Major means the Specialist replaces it
	var notes []string
	e.Exclusions, notes = idx.ResolveExclusions(e.Exclusions, self, RequiredPrograms(e.Req), progs, areas)
	e.Notes = append(e.Notes, notes...)
}

// ResolveExclusions resolves an exclusions list's program nodes, adding the rest
// of a subject's area unless the phrase is an exact title. It drops self, the
// required programs and empty nodes; a focus the calendar lacks is a note.
func (idx TitleIndex) ResolveExclusions(excl []*types.ReqNode, self string, required map[string]bool, progs *ProgramIndex, areas AreaIndex) (keep []*types.ReqNode, notes []string) {
	for _, n := range excl {
		idx.ResolveNode(n, progs, areas)
		if n.Type == types.NodeText && withinRe.MatchString(n.Text) {
			// a focus the calendar no longer has: no student can be in it
			notes = append(notes, "Not in the calendar: "+n.Text)
			continue
		}
		if n.Type == types.NodeProgram {
			// a restriction on a subject covers all its programs, including ones whose
			// title doesn't name it; an exact title is that program only
			if idx.Exact(n.Text) == "" {
				if codes, ok := areas.Lookup(n.Text); ok {
					n.Programs = grammar.Dedupe(append(n.Programs, codes...))
				}
			}
			n.Programs = slices.DeleteFunc(n.Programs, func(c string) bool { return c == self || required[c] })
			if len(n.Programs) == 0 {
				continue
			}
		}
		keep = append(keep, n)
	}
	return keep, notes
}

func (idx TitleIndex) ResolveNode(n *types.ReqNode, progs *ProgramIndex, areas AreaIndex) {
	if n == nil {
		return
	}
	for _, it := range n.Items {
		idx.ResolveNode(it, progs, areas)
	}
	if n.Type != types.NodeProgram || len(n.Programs) > 0 {
		return
	}
	if codes, ok := idx.LookupTitle(n.Text); ok {
		n.Programs = codes
		return
	}
	if progs != nil {
		if ref, ok := grammar.ParseProgramRef(n.Text); ok {
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
	if codes, ok := areas.Lookup(n.Text); ok {
		n.Programs = codes
		return
	}
	n.Type, n.Kind = types.NodeText, types.TextMembership
}

// focusParentRe matches the parent a focus's title names: "Focus in Technology
// Leadership (Computer Science Major)".
var focusParentRe = regexp.MustCompile(`\(([^()]+)\)\s*$`)

// NarrowToParent keeps, of the programs a focus's req accepts, only the parent
// its title names: sibling foci share one sentence, but each has one parent.
func (idx TitleIndex) NarrowToParent(n *types.ReqNode, title string) *types.ReqNode {
	m := focusParentRe.FindStringSubmatch(title)
	if m == nil {
		return n
	}
	parent, ok := idx.LookupTitle(m[1])
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
		return Tidy(n)
	}
	return narrow(n)
}

// withinRe matches "the Focus in Data Analytics within the Economics Major or
// Specialist": the focus's variants for those programs.
var withinRe = regexp.MustCompile(`(?i)^(?:the\s+)?(focus\s+in\s+.+?)\s+within\s+the\s+.+?\s+((?:major|specialist)(?:\s+or\s+(?:major|specialist))?)(?:\s+programs?)?$`)

// withinOrRe matches the "or" between programs: " or ".
var withinOrRe = regexp.MustCompile(`(?i)\s+or\s+`)

func (idx TitleIndex) lookupWithin(phrase string) ([]string, bool) {
	m := withinRe.FindStringSubmatch(strings.TrimSpace(phrase))
	if m == nil {
		return nil, false
	}
	var codes []string
	for _, t := range withinOrRe.Split(m[2], -1) {
		code := idx[NormSubject(m[1]+" "+t)]
		if code == "" {
			return nil, false
		}
		codes = append(codes, code)
	}
	return codes, true
}

// AreaIndex finds programs by subject area, for restrictions naming a subject:
// "any other Economics program". A program belongs to its first area only.
type AreaIndex map[string][]*types.Program

// areaAliases are other names the calendar uses for an area.
var areaAliases = map[string]string{
	"statistics": "statistical sciences",
	"cirhr":      "industrial relations and human resources",
}

func NewAreaIndex(programs []*types.Program) AreaIndex {
	idx := AreaIndex{}
	for _, p := range programs {
		if len(p.Areas) > 0 {
			k := NormSubject(p.Areas[0])
			idx[k] = append(idx[k], p)
		}
	}
	return idx
}

var (
	areaLeadRe  = regexp.MustCompile(`(?i)^(?:any\s+other|another|other|any|all)\s+`)
	AreaTypeRe  = regexp.MustCompile(`(?i)\b(?:research\s+)?(specialist|major|minor|certificate|focus)(?:es|s)?\b`)
	areaNoiseRe = regexp.MustCompile(`(?i)\boffered\s+by\s+the\s+department\s+of\b|\b(?:programs?|in|the)\b`)
	sepEdgesRe  = regexp.MustCompile(`(?i)^(?:[\s,/]|\bor\b|\band\b)+|(?:[\s,/]|\bor\b|\band\b)+$`)
	areaSepRe   = regexp.MustCompile(`(?i)\s*(?:,|/|\bor\b|\band\b)\s*`)
)

// lookup resolves a phrase naming subjects and, optionally, program types.
// With no types it means the subjects' programs, not their focuses or
// certificates. Every subject must be an area.
func (idx AreaIndex) Lookup(phrase string) ([]string, bool) {
	phrase = areaLeadRe.ReplaceAllString(strings.TrimSpace(phrase), "")
	want := map[string]bool{}
	for _, m := range AreaTypeRe.FindAllStringSubmatch(phrase, -1) {
		want[strings.ToLower(m[1])] = true
	}
	rest := areaNoiseRe.ReplaceAllString(AreaTypeRe.ReplaceAllString(phrase, " "), " ")
	subjects := areaSepRe.Split(rest, -1)
	if whole := NormSubject(sepEdgesRe.ReplaceAllString(rest, "")); idx[whole] != nil {
		subjects = []string{whole} // "Slavic and East European Languages and Cultures"
	}
	var codes []string
	for _, subject := range subjects {
		k := NormSubject(subject)
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

// RequiredPrograms lists the programs n's program nodes name outside a not.
func RequiredPrograms(n *types.ReqNode) map[string]bool {
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

// AmbiguousExclusions splits an Exclusions subject phrase with program types
// into the programs whose title names the subject and those it may also
// cover; being in one of the latter is Review.
func (idx TitleIndex) AmbiguousExclusions(e *types.Enrolment, self string, programs []*types.Program, required map[string]bool, areas AreaIndex) {
	for _, s := range idx.SplitExclusions(e.Exclusions, self, programs, required, areas) {
		s.Node.Programs = s.Sure
		e.Req = AndNode(e.Req, AmbiguousExclusion(s))
	}
}

// AmbiguousExclusion is the lenient reading of a split subject phrase:
// or(not(program[maybe]), text{ambiguity}).
func AmbiguousExclusion(s exclusionSplit) *types.ReqNode {
	return OrNode(
		&types.ReqNode{Type: types.NodeNot, Items: []*types.ReqNode{{Type: types.NodeProgram, Programs: s.maybe, Text: s.Node.Text}}},
		AmbiguousText(`"`+s.Node.Text+`" may also cover the `+strings.Join(s.titles, ", the ")))
}

// exclusionSplit is a subject phrase split by SplitExclusions: the programs it
// surely names and those it may also cover, with their titles.
type exclusionSplit struct {
	Node        *types.ReqNode
	Sure, maybe []string
	titles      []string // of maybe, without the "(Arts Program)" suffix
}

// SplitExclusions finds the subject phrases of excl that also cover programs
// they may not mean. It changes nothing.
func (idx TitleIndex) SplitExclusions(excl []*types.ReqNode, self string, programs []*types.Program, required map[string]bool, areas AreaIndex) []exclusionSplit {
	byCode := map[string]*types.Program{}
	for _, p := range programs {
		byCode[p.Code] = p
	}
	listed := map[string]bool{} // programs an Exclusions node already names
	for _, n := range excl {
		for _, c := range n.Programs {
			listed[c] = true
		}
	}
	var out []exclusionSplit
	for _, n := range excl {
		if n.Type != types.NodeProgram {
			continue
		}
		subject, kinds := exclusionSubject(n.Text, areas)
		if subject == "" || idx.Exact(n.Text) != "" { // "Chemistry Specialist" is that program
			continue
		}
		names := func(p *types.Program) bool {
			return strings.Contains(" "+NormSubject(p.Title)+" ", " "+subject+" ")
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
		// programs of those types anywhere whose title names the subject:
		// "Economics & Mathematics Specialist" for "any Mathematics Major"
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
		var titles []string
		for _, code := range maybe {
			titles = append(titles, TitleSuffixRe.ReplaceAllString(byCode[code].Title, ""))
		}
		out = append(out, exclusionSplit{Node: n, Sure: sure, maybe: grammar.Dedupe(maybe), titles: titles})
	}
	return out
}

// exclusionSubject reads a phrase naming one subject and program types: the
// subject, normalized, and the types. It returns "" for any other phrase.
func exclusionSubject(phrase string, areas AreaIndex) (string, map[string]bool) {
	if strings.Contains(strings.ToLower(phrase), "department") {
		return "", nil
	}
	kinds := map[string]bool{}
	for _, m := range AreaTypeRe.FindAllStringSubmatch(phrase, -1) {
		kinds[strings.ToLower(m[1])] = true
	}
	if len(kinds) == 0 {
		if !programWordRe.MatchString(phrase) {
			return "", nil
		}
		// "any other Economics program": its specialists, majors and minors
		kinds = map[string]bool{"specialist": true, "major": true, "minor": true}
	}
	rest := areaNoiseRe.ReplaceAllString(AreaTypeRe.ReplaceAllString(areaLeadRe.ReplaceAllString(phrase, ""), " "), " ")
	if whole := NormSubject(sepEdgesRe.ReplaceAllString(rest, "")); areas[whole] != nil {
		return whole, kinds
	}
	var subjects []string
	for _, s := range areaSepRe.Split(rest, -1) {
		if s = NormSubject(s); s != "" {
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

// programWordRe matches "any other Economics program".
var programWordRe = regexp.MustCompile(`(?i)\bprograms?\b`)
