package reqs

import (
	"reflect"
	"testing"
)

func TestExtractNotes(t *testing.T) {
	tests := []struct {
		in, wantReq string
		wantNotes   []string
	}{
		{"ANT380H1. Students who do not meet the prerequisite are encouraged to contact the Department.",
			"ANT380H1", []string{"Students who do not meet the prerequisite are encouraged to contact the Department"}},
		{"Open only to students enrolled in the European Studies Major Program. Third and Fourth year students only.",
			"Open only to students enrolled in the European Studies Major Program; Third and Fourth year students only", nil},
		{"STA257H1; MAT223H1/ MAT240H1 (Note: STA257H1 is very strongly recommended)",
			"STA257H1; MAT223H1/ MAT240H1", []string{"Note: STA257H1 is very strongly recommended"}},
		{"MAT130H1/ MAT135H1/ MAT137Y1 recommended",
			"", []string{"MAT130H1/ MAT135H1/ MAT137Y1 recommended"}},
		{"BIO220H1; EEB263H1 recommended",
			"BIO220H1", []string{"EEB263H1 recommended"}},
		{"CSC108H1 or programming experience, e.g. Python",
			"CSC108H1 or programming experience, e.g. Python", nil},
		{"ANT348H1. Note: ANT208H1 is not accepted preparation for this course.",
			"ANT348H1", []string{"Note: ANT208H1 is not accepted preparation for this course"}},
	}
	for _, tt := range tests {
		req, notes := extractNotes(tt.in)
		if req != tt.wantReq || !reflect.DeepEqual(notes, tt.wantNotes) {
			t.Errorf("extractNotes(%q)\n got: %q %q\nwant: %q %q", tt.in, req, notes, tt.wantReq, tt.wantNotes)
		}
	}
}

func TestIsAdvisory(t *testing.T) {
	for s, want := range map[string]bool{
		"Students who do not meet the prerequisites are encouraged to contact the department": true,
		"Note: ANT208H1 is not accepted preparation for this course":                          true,
		"Students in the BCH Majors and Specialists need to email the Department":             true,
		"Open only to students enrolled in the European Studies Major Program":                false,
		"Third and Fourth year students only":                                                 false,
		"ANT380H1":                                                                            false,
	} {
		if got := isAdvisory(s); got != want {
			t.Errorf("isAdvisory(%q) = %v, want %v", s, got, want)
		}
	}
}
