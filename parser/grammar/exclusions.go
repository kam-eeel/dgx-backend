package grammar

import (
	"regexp"

	"degree-explorer/types"
)

const PrevConcurrentPat = `(?i:(?:if\s+)?taken\s+previously\s+or\s+concurrently)\b`

var (
	// TermRe matches an offering qualifier: "offered in Winter 2010".
	TermRe = regexp.MustCompile(`(?i)\b(?:offered|taken)\b|\b(?:fall|winter|summer)(?:\s+session)?\s+\d{4}\b|^\d{4}(?:-\d{2,4})?$|\b(?:LEC|TUT|PRA)\d{4}\b|\b20\d{2}[159]\b`)

	// gradCodeRe matches a graduate course code: "PHM1130H".
	gradCodeRe = regexp.MustCompile(`^[A-Z]{3}\d{4}[HY]\d?$`)
)

// GradCodes turns text tokens that are a whole graduate course code into
// course tokens.
func GradCodes(src string, toks []Token) []Token {
	for i, t := range toks {
		if t.Kind == TText && gradCodeRe.MatchString(src[t.Start:t.End]) {
			toks[i] = Token{Kind: TCourse, Start: t.Start, End: t.End,
				Node: &types.ReqNode{Type: types.NodeCourse, Code: src[t.Start:t.End]}}
		}
	}
	return toks
}
