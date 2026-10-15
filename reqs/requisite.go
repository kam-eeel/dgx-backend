package reqs

import "degree-explorer/types"

// ParseRequisite turns a raw prerequisites or corequisites field into its
// stored JSON form.
func ParseRequisite(raw string) *types.Requisite {
	r := &types.Requisite{Raw: raw}
	text, urls := normalize(raw)
	text, exemptions := extractExemptions(text) // "… requirements do not apply to G"
	text, routes := extractRoutes(text)         // "Students with … may take this course with …"
	text = stripFiller(text, &urls)             // eligibility clauses join the URLs at the end of notes
	reqText, notes := extractNotes(text)
	reqText = rewriteExamples(reqText)
	for _, e := range exemptions {
		notes = append(notes, e.note)
	}
	for _, rt := range routes {
		notes = append(notes, rt.note)
	}
	r.Notes = append(notes, urls...)
	if reqText == "" {
		return r
	}
	r.Req = applyRoutes(parseText(reqText), routes)
	for _, e := range exemptions {
		if e.inField() { // one that names a single field waits for ApplyExemptions
			r.Req = exempt(r.Req, e.group)
		}
	}
	if r.Req == nil && len(r.Notes) == 0 {
		// Only noise was left; keep the text rather than silently dropping it.
		r.Req = &types.ReqNode{Type: types.NodeText, Kind: types.TextOther, Text: reqText}
	}
	return r
}

// parseText parses requirement text that has been normalized and had its
// notes taken out.
func parseText(s string) *types.ReqNode {
	toks := gradCodes(s, lex(s)) // a graduate code is checkable, as in exclusions
	p := newParser(s, joinTitleRuns(s, toks))
	return p.programNodes(p.post(p.parseAll()))
}
