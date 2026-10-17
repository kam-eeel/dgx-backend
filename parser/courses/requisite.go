package courses

import (
	"degree-explorer/parser/grammar"
	"degree-explorer/types"
)

// ParseRequisite turns a raw prerequisites or corequisites field into its
// stored JSON form. The pipeline is in grammar, since enrolment uses it too.
func ParseRequisite(raw string) *types.Requisite { return grammar.ParseRequisite(raw) }
