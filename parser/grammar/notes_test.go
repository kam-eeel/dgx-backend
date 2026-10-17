package grammar

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
		// Advice inside brackets doesn't take the sentence around it along.
		{"AST221H1, AST222H1 (or equivalent readings; consult the instructor)",
			"AST221H1, AST222H1 (or equivalent readings)", []string{"consult the instructor"}},
		{"PHY250H1, PHY256H1/ (CHM222H1, CHM223H1), (PHY256H1 recommended)",
			"PHY250H1, PHY256H1/ (CHM222H1, CHM223H1),", []string{"PHY256H1 recommended"}},
		{"1.0 SOC credit at the 300+ level (preferably selected from the Recommended Preparation list for this course)",
			"1.0 SOC credit at the 300+ level", []string{"preferably selected from the Recommended Preparation list for this course"}},
		{"STA347H1. (Note: MAT237Y1/ MAT257Y1; (MAT223H1, MAT224H1)/ MAT240H1 very strongly recommended.)",
			"STA347H1", []string{"Note: MAT237Y1/ MAT257Y1; (MAT223H1, MAT224H1)/ MAT240H1 very strongly recommended"}},
		// "(recommended)" on one alternative only marks it; on a lone course
		// the course itself is the recommendation.
		{"14.0 credits, STA288H1 (Recommended)/ STA220H1/ PSY201H1",
			"14.0 credits, STA288H1/ STA220H1/ PSY201H1", []string{"STA288H1 recommended"}},
		{"BCH370H1 (recommended)", "", []string{"BCH370H1 recommended"}},
		// A missing "." before "Students …", and an unclosed "(" that would
		// otherwise hide the next sentence.
		{"BCH340H1; BCH377H1 Students in the BCH Majors need to email the Department.",
			"BCH340H1; BCH377H1", []string{"Students in the BCH Majors need to email the Department"}},
		{"ENV221H1 or ( BIO230H1, and enrolment in the PCL Major. Students who do not meet the prerequisites are encouraged to contact the Department.",
			"ENV221H1 or ( BIO230H1, and enrolment in the PCL Major", []string{"Students who do not meet the prerequisites are encouraged to contact the Department"}},
		{"Students cannot have completed more than 8.5 credits", "Students cannot have completed more than 8.5 credits", nil},
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
		if got := IsAdvisory(s); got != want {
			t.Errorf("isAdvisory(%q) = %v, want %v", s, got, want)
		}
	}
}
