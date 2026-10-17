package reqs

import "testing"

// TestCompletionRuleFilterPools checks pools that mix a designator's
// courses at a level with listed courses (completion_filterpools.go) on
// real snippets.
func TestCompletionRuleFilterPools(t *testing.T) {
	runComplRuleCases(t, []complRuleCase{
		{"any other courses of a series and listed courses (ASSPE1762, shortened)",
			"Fourth Year:\n1. Two courses from the BCH 400-series lecture courses: BCH422H1, BCH425H1\n2. An additional 1.0 credit at the 4th year level from the following list: any other BCH 400-series course including the 4th year BCH research project courses (see NOTE 2), BCB420H1, CHM447H1, MGY420H1",
			[]string{
				`1 allocation {"type":"credits","amount":1,"courses":["BCH422H1","BCH425H1"]}`,
				`2 allocation {"type":"credits","amount":1,"designators":["BCH"],"levels":[400],"courses":["BCB420H1","CHM447H1","MGY420H1"]}`,
			},
			nil},
		{"electives of two kinds, a floor on one, the other a list of filters (ASMAJ2027B, shortened)",
			"1. 2.0 credits from ensemble courses: MUS120Y1, MUS220Y1\n2. 2.5 credits from electives in composition, music technology, music history, culture and theory, and music and health, of which at least 1.5 credits must be completed from composition or music technology courses:\n  - _Composition or music technology courses_: CMU113H1; CMU314Y1; DMU430H1\n  - _Music history, culture and theory, and music and health courses:_ HMU200+ level courses; JMU210H1; TMU300+ level courses; SMU400+ level courses",
			[]string{
				`1 allocation {"type":"credits","amount":2,"courses":["MUS120Y1","MUS220Y1"]}`,
				`2 allocation {"type":"credits","amount":2.5,"lists":["Composition or music technology courses","Music history, culture and theory, and music and health courses"]}`,
				`3 constraint p2 {"type":"credits","amount":1.5,"lists":["Composition or music technology courses"]}`,
				`list: Composition or music technology courses: [{"type":"credits","courses":["CMU113H1","CMU314Y1","DMU430H1"]}]`,
				`list: Music history, culture and theory, and music and health courses: [{"type":"credits","designators":["HMU"],"levels":[200,300,400]},{"type":"credits","designators":["TMU"],"levels":[300,400]},{"type":"credits","designators":["SMU"],"levels":[400]},{"type":"credits","courses":["JMU210H1"]}]`,
			},
			nil},
	})
}
