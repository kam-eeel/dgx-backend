package grammar

import (
	"testing"
)

// Goldens for averages over a set of courses. A per-course minimum, hedged
// wording or an average over courses that can't be pinned down is not an
// average node.
var averageGoldens = []struct{ name, raw, want string }{
	// a percentage average over an amount of credits
	{"percent average in credits", "9.0 credits (Year 3 or higher) including a 77% average in 3.0 HIS credits (excluding  HIS262H1)",
		jAnd(`{"type":"credits","amount":9}`, `{"type":"average","min_grade":77,"amount":3,"designators":["HIS"],"exclude":["HIS262H1"]}`)},
	{"minimum percent over credits", "Minimum 80% over 3.0 HIS credits at the 200-level or above",
		`{"type":"average","min_grade":80,"amount":3,"designators":["HIS"],"levels":[200,300,400]}`},
	{"average of at least", "IRE244H1,  IRE260H1 and an average of at least 70% in 1.0 credit in IRE at the 300+ level",
		jAnd(jC("IRE244H1"), jC("IRE260H1"), `{"type":"average","min_grade":70,"amount":1,"designators":["IRE"],"levels":[300,400]}`)},
	{"letter average in credits", "a B+ average in 2.0 ENG credits", `{"type":"average","min_grade":77,"amount":2,"designators":["ENG"]}`},

	// no amount: every matching course
	{"overall average across all courses", "ECO372H1; an overall average of 73% across all ECO courses taken.",
		jAnd(jC("ECO372H1"), `{"type":"average","min_grade":73,"designators":["ECO"]}`)},
	{"average in all credits", "an average of 75% in all PHL credits", `{"type":"average","min_grade":75,"designators":["PHL"]}`},

	// an average after a credit phrase includes having that many credits
	{"letter average suffix", "10.0 credits; have taken at least 2.0 CHC credits with at least a B+ average.",
		jAnd(`{"type":"credits","amount":10}`, `{"type":"average","min_grade":77,"amount":2,"designators":["CHC"]}`)},
	{"percent average suffix", "2.0 MST credits with an average of at least 75%",
		`{"type":"average","min_grade":75,"amount":2,"designators":["MST"]}`},

	// an average over the group just before it
	{"average of both", "MAT235Y1(80%)/ [( MAT235H1,  MAT236H1) (average of both must be 80%)]",
		jOr(jCG("MAT235Y1", 80), jAnd(jC("MAT235H1"), jC("MAT236H1"), `{"type":"average","min_grade":80,"courses":["MAT235H1","MAT236H1"]}`))},
	{"average in these courses", "( PHY151H1, PHY152H1, MAT137Y1) with an average of at least 75% in these courses",
		jAnd(jC("PHY151H1"), jC("PHY152H1"), jC("MAT137Y1"), `{"type":"average","min_grade":75,"courses":["PHY151H1","PHY152H1","MAT137Y1"]}`)},
	{"both but three courses stays text", "( MAT235H1, MAT236H1, MAT237H1) (average of both must be 80%)",
		jAnd(jC("MAT235H1"), jC("MAT236H1"), jC("MAT237H1"), jT("other", "average of both must be 80%"))},
	{"both after an alternative stays text", "( MAT235H1/ MAT236H1) (average of both must be 80%)",
		jAnd(jOr(jC("MAT235H1"), jC("MAT236H1")), jT("other", "average of both must be 80%"))},
	{"both after one course stays text", "MAT235H1, MAT236H1 (average of both must be 80%)",
		jAnd(jC("MAT235H1"), jC("MAT236H1"), jT("other", "average of both must be 80%"))},

	// not averages
	{"per-course minimum in each", "At least 1.0 STA credit at the 300+ level with a minimum grade of 80% in each course",
		`{"type":"credits","amount":1,"designators":["STA"],"levels":[300,400],"min_grade":80}`},
	{"grade in credits is per course", "75% or higher in 1.0 HIS credit", `{"type":"credits","amount":1,"designators":["HIS"],"min_grade":75}`},
	{"minimum in all courses stays text", "A minimum of 80% must be achieved in all ENG courses taken at the 200+ level.",
		jT("other", "A minimum of 80% must be achieved in all ENG courses taken at the 200+ level")},
	{"hedged standing stays text", `ANA300Y1. Normally a "B" standing will be required.`,
		jAnd(jC("ANA300Y1"), jT("other", `Normally a "B" standing will be required`))},
	{"courses used to satisfy stays text", "MAT244H1. A minimum grade of 78% is required in each math courses used to satisfy the prerequisite.",
		jAnd(jC("MAT244H1"), jT("other", "A minimum grade of 78% is required in each math courses used to satisfy the prerequisite"))},
	{"average with no filter stays text", "a 77% average in 3.0 credits",
		jAnd(jT("other", "a 77% average in"), `{"type":"credits","amount":3}`)},
	{"average of unnamed courses stays text", "an average of 75% in the required courses",
		jT("other", "an average of 75% in the required courses")},
	{"average with its own grade stays text", "an average of 75% in 2.0 PHL credits with a minimum grade of 70%",
		jT("other", "an average of 75% in 2.0 PHL credits with a minimum grade of 70%")},
	{"GPA stays GPA", "Completion of at least 2.0 CHM credits with a minimum cGPA of 3.0 in all CHM credits",
		jAnd(`{"type":"credits","amount":2,"designators":["CHM"]}`, `{"type":"gpa","min_gpa":3,"scope":"CHM credits"}`)},
}

func TestParseAverages(t *testing.T) {
	for _, g := range averageGoldens {
		t.Run(g.name, func(t *testing.T) {
			assertJSON(t, ParseRequisite(g.raw).Req, g.want)
		})
	}
}
