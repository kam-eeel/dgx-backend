package reqs

import "degree-explorer/types"

// ParseCompletion parses a program's completion requirements. For now it
// only stores the text: every program gets no requirements and its Raw.
func ParseCompletion(raw string) *types.Completion {
	return &types.Completion{Requirements: []*types.Requirement{}, Raw: raw}
}

// parseCompletionText parses the text of one completion requirement (an
// item's body, without its number) with the completion grammar: the shared
// requisite grammar plus the extensions in phrases_completion.go and those
// marked "completion" in parser.go. Notes holds what it set aside: URLs and
// advice ("( MAT237Y1 recommended)", footnotes).
func parseCompletionText(s string) (req *types.ReqNode, notes []string) {
	text, notes := normalize(s)
	text, advice := stripAnnotations(stripLabels(text))
	notes = append(notes, advice...)
	text = eitherDesigRe.ReplaceAllString(text, "$1 $2")
	if text == "" {
		return nil, notes
	}
	toks := gradCodes(text, lexCompletion(text))
	p := newParser(text, joinCompletionTitleRuns(text, toks))
	p.completion = true
	return p.programNodes(p.post(p.parseAll())), notes
}
