package completion

import (
	"testing"
)

func TestStripLabels(t *testing.T) {
	for in, want := range map[string]string{
		"Geography: JGE321H1":                            "JGE321H1",
		"Option A: Immunology: ( IMM341H1, IMM351H1)":    "( IMM341H1, IMM351H1)",
		"_Ukrainian:_ SLA108Y1, SLA208Y1":                "SLA108Y1, SLA208Y1",
		"Groups, Rings and Fields: MAT347Y1":             "MAT347Y1",
		"Cluster 1 - Environment/ Ecology: ENV221H1":     "Cluster 1 - Environment/ Ecology: ENV221H1", // a list name
		"Algebra: 1.0 credit from MAT223H1/ MAT240H1":    "1.0 credit from MAT223H1/ MAT240H1",
		"Core (required) Courses: 1.0 credit from X":     "1.0 credit from X",
		"One of: PHL373H1, ENV333H1":                     "One of: PHL373H1, ENV333H1",
		"One course from: HIS101H1":                      "One course from: HIS101H1",
		"For students with previous knowledge: SPA220Y1": "For students with previous knowledge: SPA220Y1",
		"Recommended: 0.5 credit from any HPS courses":   "Recommended: 0.5 credit from any HPS courses",
		"From Group A: 1.0 credit":                       "From Group A: 1.0 credit",
		"Psycholinguistics (ASFOC2270I): (3.0 credits)":  "Psycholinguistics (ASFOC2270I): (3.0 credits)",
		"Requirement 7: students can take any APM":       "Requirement 7: students can take any APM",
	} {
		if got := stripLabels(in); got != want {
			t.Errorf("stripLabels(%q) = %q, want %q", in, got, want)
		}
	}
}
