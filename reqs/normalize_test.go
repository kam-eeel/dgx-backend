package reqs

import (
	"reflect"
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		in, want string
		urls     []string
	}{
		{"ECO220Y1/\u200b  ECO227Y1", "ECO220Y1/ ECO227Y1", nil},
		{"MAT257Y1/ [ MAT224H1/  MAT247H1]", "MAT257Y1/ ( MAT224H1/ MAT247H1)", nil},
		{"POL 478H1", "POL478H1", nil},
		{"GGR270H1*/ STA220H1", "GGR270H1/ STA220H1", nil},
		{"Grade 11 and/or 12 Italian", "Grade 11 / 12 Italian", nil},
		{"AST222H1 (or equivalent AST readings)", "AST222H1 or (equivalent AST readings)", nil},
		{"4.0 ENG credits (including ENG202H1, ENG203H1)", "4.0 ENG credits including (ENG202H1, ENG203H1)", nil},
		{"Admission to Vic One: https://vic.utoronto.ca/vic-one/", "Admission to Vic One:", []string{"https://vic.utoronto.ca/vic-one/"}},
		{"CSC108H1\u00a0", "CSC108H1", nil},
	}
	for _, tt := range tests {
		got, urls := normalize(tt.in)
		if got != tt.want || !reflect.DeepEqual(urls, tt.urls) {
			t.Errorf("normalize(%q) = %q, %v; want %q, %v", tt.in, got, urls, tt.want, tt.urls)
		}
	}
}
