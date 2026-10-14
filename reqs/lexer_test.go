package reqs

import (
	"reflect"
	"testing"
)

func kinds(toks []token) []tokKind {
	out := make([]tokKind, len(toks))
	for i, t := range toks {
		out[i] = t.kind
	}
	return out
}

func texts(src string, toks []token) []string {
	out := make([]string, len(toks))
	for i, t := range toks {
		out[i] = src[t.start:t.end]
	}
	return out
}

func TestLexOperators(t *testing.T) {
	src := "( MAT130H1/ MAT135H1, MAT136H1), plus INS220Y1, or ACT240H1 and CSC108H1; CSC148H1 & CSC165H1 or CSC111H1"
	want := []tokKind{
		tLParen, tCourse, tSlash, tCourse, tComma, tCourse, tRParen,
		tCommaAnd, tCourse, tCommaOr, tCourse, tAnd, tCourse, tSemi,
		tCourse, tAnd, tCourse, tOr, tCourse,
	}
	if got := kinds(lex(src)); !reflect.DeepEqual(got, want) {
		t.Errorf("\n got: %v\nwant: %v", got, want)
	}
}

func TestLexText(t *testing.T) {
	tests := []struct {
		src   string
		kinds []tokKind
		texts []string
	}{
		{"placement in FSL220H1 based on the test or permission of the instructor",
			[]tokKind{tText, tCourse, tText, tOr, tText},
			[]string{"placement in", "FSL220H1", "based on the test", "or", "permission of the instructor"}},
		{"permission of the instructor/program administrator",
			[]tokKind{tText, tSlash, tText},
			[]string{"permission of the instructor", "/", "program administrator"}},
		{"Students at A&S only", []tokKind{tText}, []string{"Students at A&S only"}},
		{"ANT207H1 and any 300-level course in Society",
			[]tokKind{tCourse, tAnd, tCredits, tText},
			[]string{"ANT207H1", "and", "any 300-level course", "in Society"}},
	}
	for _, tt := range tests {
		toks := lex(tt.src)
		if !reflect.DeepEqual(kinds(toks), tt.kinds) || !reflect.DeepEqual(texts(tt.src, toks), tt.texts) {
			t.Errorf("lex(%q)\n got: %v %q\nwant: %v %q", tt.src, kinds(toks), texts(tt.src, toks), tt.kinds, tt.texts)
		}
	}
}
