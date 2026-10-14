package reqs

import "degree-explorer/types"

// ParseRequisite turns a raw prerequisites or corequisites field into its
// stored JSON form.
func ParseRequisite(raw string) *types.Requisite {
	r := &types.Requisite{Raw: raw}
	text, urls := normalize(raw)
	reqText, notes := extractNotes(text)
	reqText = rewriteExamples(reqText)
	r.Notes = append(notes, urls...)
	if reqText == "" {
		return r
	}
	p := newParser(reqText, lex(reqText))
	r.Req = p.post(p.parseAll())
	if r.Req == nil && len(r.Notes) == 0 {
		// Only noise was left; keep the text rather than silently dropping it.
		r.Req = &types.ReqNode{Type: types.NodeText, Kind: types.KindOther, Text: reqText}
	}
	return r
}
