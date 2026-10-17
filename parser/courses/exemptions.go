package courses

import (
	"slices"

	"degree-explorer/parser/grammar"
	"degree-explorer/types"
)

// ApplyExemptions applies the exemptions one requisite field states about the
// other, as exempt does, and copies each sentence to its notes. Exemptions that
// ParseRequisite applied are left alone; either argument may be nil.
func ApplyExemptions(prereq, coreq *types.Requisite) {
	for _, src := range []*types.Requisite{prereq, coreq} {
		if src == nil {
			continue
		}
		for _, note := range slices.Clone(src.Notes) {
			e, ok := grammar.FindExemption(note)
			if !ok {
				continue
			}
			for _, dst := range []*types.Requisite{prereq, coreq} {
				names := dst == prereq && e.Pre || dst == coreq && e.Co
				if dst == nil || dst.Req == nil || !(names || dst == src && e.InField()) {
					continue
				}
				dst.Req = grammar.Exempt(dst.Req, e.Group)
				if !slices.Contains(dst.Notes, e.Note) {
					dst.Notes = append(dst.Notes, e.Note)
				}
			}
		}
	}
}
