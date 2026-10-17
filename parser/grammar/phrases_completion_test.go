package grammar

import (
	"slices"
	"testing"
)

// firstCompletionPhrase runs the completion matchers in LexCompletion order.
func firstCompletionPhrase(s string) (Token, bool) {
	for _, m := range completionMatchers {
		if t, ok := m(s); ok {
			return t, true
		}
	}
	return Token{}, false
}

func TestCompletionPhrases(t *testing.T) {
	tests := []struct {
		in       string
		kind     TokKind
		consumed string
		node     string // JSON of token.node; "" for none
	}{
		// wildcard pool members
		{"PSL400-series courses", TWild, "PSL400-series courses", `{"type":"credits","amount":0.5,"designators":["PSL"],"levels":[400]}`},
		{"HMU200+ level courses; JMU210H1", TWild, "HMU200+ level courses", `{"type":"credits","amount":0.5,"designators":["HMU"],"levels":[200,300,400]}`},
		{"TMU200+ courses; SMU400-level courses", TWild, "TMU200+ courses", `{"type":"credits","amount":0.5,"designators":["TMU"],"levels":[200,300,400]}`},
		{"SMU400-level courses", TWild, "SMU400-level courses", `{"type":"credits","amount":0.5,"designators":["SMU"],"levels":[400]}`},
		{"PHY 300-level courses/ PHY 400-level courses", TWild, "PHY 300-level courses", `{"type":"credits","amount":0.5,"designators":["PHY"],"levels":[300]}`},
		{"APM/MAT 200+ courses], ( MAT235H1", TWild, "APM/MAT 200+ courses", `{"type":"credits","amount":0.5,"designators":["APM","MAT"],"levels":[200,300,400]}`},
		{"any ETH200+/ FOR201H1", TWild, "any ETH200+", `{"type":"credits","amount":0.5,"designators":["ETH"],"levels":[200,300,400]}`},
		{"EEB 400-series level course", TWild, "EEB 400-series level course", `{"type":"credits","amount":0.5,"designators":["EEB"],"levels":[400]}`},
		{"EEB (excluding EEB196H1, EEB197H1); ENV432H1", TWild, "EEB (excluding EEB196H1, EEB197H1)",
			`{"type":"credits","amount":0.5,"designators":["EEB"],"exclude":["EEB196H1","EEB197H1"]}`},
		{"ECO/ HIS/ PHL/ POL", TWild, "ECO/ HIS/ PHL/ POL", `{"type":"credits","amount":0.5,"designators":["ECO","HIS","PHL","POL"]}`},

		// pools
		{"Any 2 courses (1.0 credit) from GGR107H1", tPool, "Any 2 courses (1.0 credit) from", ""},
		{"Any course (0.5 credit) from GGR107H1", tPool, "Any course (0.5 credit) from", ""},
		{"Any 4 courses (2.0 credits) from: GGR225H1", tPool, "Any 4 courses (2.0 credits) from:", ""},
		{"any two of: NFS382H1", tPool, "any two of:", ""},
		{"2.0 credits from: ANA; ANT333Y1", tPool, "2.0 credits from:", ""},
		{"1.0 credit at 300+ level from: CJH332H1", tPool, "1.0 credit at 300+ level from:", `{"type":"credits","levels":[300,400]}`},
		{"0.5 credit at the 400-level from CSB426H1", tPool, "0.5 credit at the 400-level from", `{"type":"credits","levels":[400]}`},
		{"2.5 credits from electives in music history, culture, and theory: HMU200+ level courses", tPool,
			"2.5 credits from electives in music history, culture, and theory:", ""},

		// other words
		{"All of: LIN101H1", tSkip, "All of:", ""},
		{"Either GGR491Y1 or", tEither, "Either", ""},
		{"EITHER 2.5 credits from Group A", tEither, "EITHER", ""},
	}
	for _, tt := range tests {
		tok, ok := firstCompletionPhrase(tt.in)
		if !ok {
			t.Errorf("%q: no phrase matched", tt.in)
			continue
		}
		if tok.Kind != tt.kind || tt.in[:tok.End] != tt.consumed {
			t.Errorf("%q: got kind %v consuming %q; want kind %v consuming %q", tt.in, tok.Kind, tt.in[:tok.End], tt.kind, tt.consumed)
			continue
		}
		if tt.node != "" {
			assertJSON(t, tok.Node, tt.node)
		}
		if shared, ok := firstPhrase(tt.in); ok && shared.Kind == tt.kind && shared.End == tok.End {
			t.Errorf("%q: the shared matchers read it the same way; it isn't a completion extension", tt.in)
		}
	}
}

func TestCompletionPhraseNonMatches(t *testing.T) {
	for _, s := range []string{
		"PSL400H1",                     // a course code
		"GGR courses at the 300-level", // a designator followed by prose
		"AND CSC148H1",                 // an upper-case word, not a designator
		"ANY 300-level literature courses",
		"all of which are required: PCJ200H1",
		"either pair of courses is accepted",
		"2.0 credits in GRK or LAT", // a credit phrase: a bare designator starts a list only after a colon
	} {
		if tok, ok := firstCompletionPhrase(s); ok && slices.Contains([]TokKind{TWild, tPool, tSkip, tEither}, tok.Kind) {
			t.Errorf("%q: unexpectedly matched %+v", s, tok)
		}
	}
}
