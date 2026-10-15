package reqs

import "degree-explorer/types"

// ParseCompletion parses a program's completion requirements. For now it
// only stores the text: every program gets no requirements and its Raw.
func ParseCompletion(raw string) *types.Completion {
	return &types.Completion{Requirements: []*types.Requirement{}, Raw: raw}
}
