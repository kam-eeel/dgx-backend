package grammar

import (
	"reflect"
	"testing"
)

func kinds(toks []Token) []TokKind {
	out := make([]TokKind, len(toks))
	for i, t := range toks {
		out[i] = t.Kind
	}
	return out
}

func texts(src string, toks []Token) []string {
	out := make([]string, len(toks))
	for i, t := range toks {
		out[i] = src[t.Start:t.End]
	}
	return out
}

func TestLexOperators(t *testing.T) {
	src := "( MAT130H1/ MAT135H1, MAT136H1), plus INS220Y1, or ACT240H1 and CSC108H1; CSC148H1 & CSC165H1 or CSC111H1"
	want := []TokKind{
		TLParen, TCourse, TSlash, TCourse, TComma, TCourse, TRParen,
		TCommaAnd, TCourse, TCommaOr, TCourse, TAnd, TCourse, TSemi,
		TCourse, TAnd, TCourse, TOr, TCourse,
	}
	if got := kinds(Lex(src)); !reflect.DeepEqual(got, want) {
		t.Errorf("\n got: %v\nwant: %v", got, want)
	}
}

func TestLexText(t *testing.T) {
	tests := []struct {
		src   string
		kinds []TokKind
		texts []string
	}{
		{"placement in FSL220H1 based on the test or permission of the instructor",
			[]TokKind{TText, TCourse, TText, TOr, TText},
			[]string{"placement in", "FSL220H1", "based on the test", "or", "permission of the instructor"}},
		{"permission of the instructor/program administrator",
			[]TokKind{TText, TSlash, TText},
			[]string{"permission of the instructor", "/", "program administrator"}},
		{"Students at A&S only", []TokKind{TText}, []string{"Students at A&S only"}},
		{"ANT207H1 and any 300-level course in Society",
			[]TokKind{TCourse, TAnd, tCredits, TText},
			[]string{"ANT207H1", "and", "any 300-level course", "in Society"}},
	}
	for _, tt := range tests {
		toks := Lex(tt.src)
		if !reflect.DeepEqual(kinds(toks), tt.kinds) || !reflect.DeepEqual(texts(tt.src, toks), tt.texts) {
			t.Errorf("lex(%q)\n got: %v %q\nwant: %v %q", tt.src, kinds(toks), texts(tt.src, toks), tt.kinds, tt.texts)
		}
	}
}
