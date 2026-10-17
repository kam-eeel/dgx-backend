package grammar

import (
	"degree-explorer/types"
)

// ParseRequisite turns a raw prerequisites or corequisites field into its
// stored JSON form.
func ParseRequisite(raw string) *types.Requisite {
	r := &types.Requisite{Raw: raw}
	text, urls := Normalize(raw)
	text, exemptions := extractExemptions(text) // "… requirements do not apply to G"
	text, routes := extractRoutes(text)         // "Students with … may take this course with …"
	text = stripFiller(text, &urls)             // eligibility clauses join the URLs at the end of notes
	reqText, notes := extractNotes(text)
	reqText = rewriteExamples(reqText)
	for _, e := range exemptions {
		notes = append(notes, e.Note)
	}
	for _, rt := range routes {
		notes = append(notes, rt.note)
	}
	r.Notes = append(notes, urls...)
	if reqText == "" {
		return r
	}
	r.Req = applyRoutes(ParseText(reqText), routes)
	for _, e := range exemptions {
		if e.InField() { // one that names a single field waits for ApplyExemptions
			r.Req = Exempt(r.Req, e.Group)
		}
	}
	if r.Req == nil && len(r.Notes) == 0 {
		// Only noise was left; keep the text rather than silently dropping it.
		r.Req = &types.ReqNode{Type: types.NodeText, Kind: types.TextOther, Text: reqText}
	}
	return r
}

// ParseText parses requirement text that has been normalized and had its
// notes taken out.
func ParseText(s string) *types.ReqNode {
	toks := GradCodes(s, Lex(s)) // a graduate code is checkable, as in exclusions
	p := NewParser(s, JoinTitleRuns(s, toks))
	return p.ProgramNodes(p.Post(p.ParseAll()))
}
