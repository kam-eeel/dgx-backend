package reqs

import "testing"

// Goldens for credit phrases that name designators or levels in places the
// credit matchers used to miss: "and/or" lists, designators or a level before
// the amount, topic words before "course", and a subject spelled out as
// designators in parentheses.
var designatorGoldens = []struct{ name, raw, want string }{
	{"and/or list", "Completion of 1.0 credit from MUN, PPG, and/or PCJ courses at the 200+ levels.",
		`{"type":"credits","amount":1,"designators":["MUN","PPG","PCJ"],"levels":[200,300,400]}`},
	{"designator before amount", "Any CSC 0.5 credit, and balloting",
		jAnd(`{"type":"credits","amount":0.5,"designators":["CSC"]}`, jT("permission", "balloting"))},
	{"level before amount", "CSE240H1 and a 300+ level 0.5 credit from Some Studies Core Group",
		jAnd(jC("CSE240H1"), `{"type":"credits","amount":0.5,"levels":[300,400]}`, jT("other", "from Some Studies Core Group"))},
	{"topic then level", "ITA250Y1, and at least one 0.5 credit ITA literature course at the 300-level",
		jAnd(jC("ITA250Y1"), `{"type":"credits","amount":0.5,"designators":["ITA"],"levels":[300]}`, jT("other", "literature course"))},
	{"topic after in", "at least 0.5 credit in ITA literature courses at the 300-level",
		jAnd(`{"type":"credits","amount":0.5,"designators":["ITA"],"levels":[300]}`, jT("other", "literature courses"))},
	{"any course with topic list", "Any 300+ FRE Linguistics/Literature/Second Language Learning course. Permission of Department.",
		jAnd(`{"type":"credits","amount":0.5,"designators":["FRE"],"levels":[300,400]}`,
			jT("other", "Linguistics/Literature/Second Language Learning course"), jT("permission", "Permission of Department"))},
	{"subject with designators in parens", "Any 300/ 400-level lecture course in Physics (PHY/ JPE)",
		jAnd(`{"type":"credits","amount":0.5,"designators":["PHY","JPE"],"levels":[300,400]}`, jT("other", "lecture course"))},
	{"topic named group", "AFR150Y1 or any 200-level or 300-level Group A Course",
		jOr(jC("AFR150Y1"), jAnd(`{"type":"credits","amount":0.5,"levels":[200,300]}`, jT("other", "Group A Course")))},
	{"topic with grade", "a 300-level Korean course with a minimum grade of A- OR any 400-level Korean language course",
		jOr(jAnd(`{"type":"credits","amount":0.5,"levels":[300],"min_grade":80}`, jT("other", "Korean course")),
			jAnd(`{"type":"credits","amount":0.5,"levels":[400]}`, jT("other", "Korean language course")))},
	// No amount: how many courses is "courses"? Left for review.
	{"plural courses without amount", "Appropriate 300-level ESS/ JEG/ JGA/ JPE courses",
		jT("other", "Appropriate 300-level ESS/ JEG/ JGA/ JPE courses")},
	// The topic stops at operators: "or equivalent" is an alternative.
	{"no topic across or", "0.5 credit in PHL or equivalent courses",
		jOr(`{"type":"credits","amount":0.5,"designators":["PHL"]}`, jT("preparation", "equivalent courses"))},
}

func TestParseRequisiteDesignators(t *testing.T) {
	for _, g := range designatorGoldens {
		t.Run(g.name, func(t *testing.T) {
			assertJSON(t, ParseRequisite(g.raw).Req, g.want)
		})
	}
}

func TestDesignatorNonMatches(t *testing.T) {
	for _, s := range []string{
		"any 300-level or 400-level",   // no course word or topic after the levels
		"Any 300",                      // a level alone
		"any of the following courses", // a pool, not a topic
		"AST courses",                  // no amount
	} {
		if tok, ok := firstPhrase(s); ok && tok.kind == tCredits {
			t.Errorf("%q: unexpectedly matched %+v", s, tok)
		}
	}
}
