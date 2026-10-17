package reqs

import (
	"slices"
	"strings"
	"testing"
)

// TestCompletionOverlaps checks the overlaps the stand-in and
// "only have to complete" rules write: the course that does two items.
func TestCompletionOverlaps(t *testing.T) {
	for _, tt := range []struct {
		name, raw string
		row       int
		want      []int
	}{
		{"ASMAJ1407, shortened", "(2.0 credits)\n1. DTS200H1/ DTS200Y1\n2. 1.0 DTS credit at the 300-level, including DTS300H1\nNotes:\n- Students who completed DTS200Y1 for Requirement 1 only have to complete DTS300H1 for Requirement 2.", 3, []int{2}},
		{"ASMIN1689, shortened", "(1.5 credits)\n1. CSC165H1/ CSC240H1\n2. CSC236H1/ CSC240H1\nIf you take CSC240H1 without CSC165H1, there is no need to replace the missing half-credit for program completion.\nCSC240H1 is an accelerated and enriched version of CSC165H1 plus CSC236H1.", 2, []int{3}},
	} {
		c := ParseCompletion(tt.raw)
		if len(c.Requirements) < tt.row || !slices.Equal(c.Requirements[tt.row-1].Overlaps, tt.want) {
			t.Errorf("%s: row %d overlaps, want %v:\n%s", tt.name, tt.row, tt.want, strings.Join(complCompact(c), "\n"))
		}
	}
}
