package programs

import (
	"regexp"
	"slices"
	"strings"

	"degree-explorer/parser/grammar"
	"degree-explorer/types"
)

var (

	// ---- titles ----

	trailingParenRe = regexp.MustCompile(`\s*\([^()]*\)\s*$`)
	titleInRe       = regexp.MustCompile(`^\s*(?i:programs?\s+)?(?i:in)\s+(?:(?i:the)\s+)?`)
	nonAlnumRe      = regexp.MustCompile(`[^a-z0-9]+`)
)

// ---- resolution ----

// ProgramIndex looks catalogue programs up by subject and type. Build it once
// per scrape with NewProgramIndex.
type ProgramIndex struct {
	bySubject map[string]map[string][]string // normalized subject → type → codes
}

// NewProgramIndex indexes programs by the subject in their title: "Cognitive
// Science Major - Arts (... Stream) (Arts Program)" is subject "cognitive
// science", type Major.
func NewProgramIndex(programs []*types.Program) *ProgramIndex {
	idx := &ProgramIndex{bySubject: map[string]map[string][]string{}}
	for _, p := range programs {
		subj := NormSubject(titleSubject(p.Title, p.Type))
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

// Resolve fills in Programs for every program node in r from the catalogue,
// and turns nodes it can't resolve back into membership text. r may be nil.
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
	if ref, ok := grammar.ParseProgramRef(n.Text); ok {
		if codes, ok := idx.lookupRef(ref); ok {
			n.Programs = codes
			return
		}
	}
	n.Type, n.Kind = types.NodeText, types.TextMembership
}

// lookupRef resolves every name in ref; if any fails, the whole phrase fails,
// as a partial list would make the missing programs NotMet.
func (idx *ProgramIndex) lookupRef(ref grammar.ProgramRef) ([]string, bool) {
	var codes []string
	for _, nr := range ref.Names {
		c, ok := idx.lookup(nr.Name, nr.Types)
		if !ok { // a list of names: "History or Political Science major"
			parts := grammar.ProgPieceSepRe.Split(nr.Name, -1)
			if len(parts) < 2 {
				return nil, false
			}
			for _, part := range parts {
				pc, ok := idx.lookup(part, nr.Types)
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

// lookup finds the programs of one subject; every type asked for must exist.
// It refuses when a longer subject starts with the name: with no type, any;
// with types, one of those types that isn't a separate "X and Y" subject.
func (idx *ProgramIndex) lookup(name string, types []string) ([]string, bool) {
	key := NormSubject(name)
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
		for _, t := range grammar.ProgramTypes {
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
// word, trailing brackets, streams and foci after the type word, and ": ..."
// after a leading type.
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
	if re := grammar.TitleTypeRes[typ]; re != nil {
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

// NormSubject makes names comparable: "Cell & Molecular Biology" and "cell
// and molecular biology".
func NormSubject(s string) string {
	s = strings.ReplaceAll(strings.ToLower(s), "&", " and ")
	s = strings.TrimSpace(nonAlnumRe.ReplaceAllString(s, " "))
	return strings.TrimPrefix(s, "the ")
}
