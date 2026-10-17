package grammar

import (
	"testing"
)

func TestRepairCatalogCodes(t *testing.T) {
	known := map[string]bool{"MAT158H1": true, "SPA258H1": true, "ECO100H1": true, "ECO100Y1": true, "MAT100H1": true}
	tests := []struct{ in, want string }{
		{"(MAT1581, MAT159H1)", "(MAT158H1, MAT159H1)"},
		{"SPA219Y1/SPA258 or Spanish placement test", "SPA219Y1/SPA258H1 or Spanish placement test"},
		{"ECO100 or ECO101H1", "ECO100 or ECO101H1"},               // H or Y: both exist
		{"MAT100-series", "MAT100-series"},                         // a phrase, not a code
		{"consult the MAT158 Office", "consult the MAT158 Office"}, // a name, not a course
		{"LEC0101, CSC999", "LEC0101, CSC999"},                     // no such course
		{"MAT158H1, ASMAJ1618", "MAT158H1, ASMAJ1618"},             // already codes
	}
	for _, tt := range tests {
		if got, _ := RepairCatalogCodes(tt.in, known); got != tt.want {
			t.Errorf("repairCatalogCodes(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
