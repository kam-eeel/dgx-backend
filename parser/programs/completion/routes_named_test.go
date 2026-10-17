package completion

import (
	"testing"
)

// TestCompletionBoundRoutes checks bounded routes and the pools beside them on
// snippets of real programs (and one built from a real one to show a guard).
func TestCompletionBoundRoutes(t *testing.T) {
	runComplRuleCases(t, []ruleCase{
		{"approved courses of other programs for part of a designator's floor: the floor or that much less of it, as permission (ASSPE0908, shortened)",
			"(11.0 credits)\nAt least 9.0 FAH credits, and 2.0 credits in one or more languages including at least one German, French, or Italian, though an acceptable alternative modern language such as Dutch or Russian (or Chinese and/or another Asian language) may be acceptable.\nFirst Year:\nFAH101H1/ FAH102H1\nHigher Years:\n6. Approved courses in other programs may be substituted for up to 2.0 FAH credits. See Department website for details.",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":11},{"type":"text","kind":"other","text":"(11.0 credits)"}]}`,
				`2 allocation p1 {"type":"or","items":[{"type":"and","items":[{"type":"credits","amount":2,"designators":["GER","FRE","FSL","ITA"]},{"type":"text","kind":"content","text":"2.0 credits in one or more languages including at least one German, French, or Italian"}]},{"type":"text","kind":"permission","text":"though an acceptable alternative modern language such as Dutch or Russian (or Chinese and/or another Asian language) may be acceptable"}]}`,
				`3 allocation p1 {"type":"or","items":[{"type":"course","code":"FAH101H1"},{"type":"course","code":"FAH102H1"}]}`,
				`4 constraint p1 {"type":"or","items":[{"type":"credits","amount":9,"designators":["FAH"]},{"type":"and","items":[{"type":"credits","amount":7,"designators":["FAH"]},{"type":"text","kind":"permission","text":"Approved courses in other programs may be substituted for up to 2.0 FAH credits"}]},{"type":"text","kind":"other","text":"At least 9.0 FAH credits"}]}`,
				`note: See Department website for details`,
			},
			nil},
		{"a course after the pool's \";\": the course of its own, or one of the pool's (ASMAJ1100, shortened)",
			"(2.5 credits)\nThird Year:\nFOR305H1; 1.0 credit from FOR300H1, FOR301H1, FOR302H1, FOR303H1, FOR306H1, FOR307H1, FOR310H1; EEB321H1\nFourth Year:\nFOR400Y1",
			[]string{
				`1 allocation {"type":"credits","amount":2.5}`,
				`2 allocation p1 {"type":"or","items":[{"type":"and","items":[{"type":"course","code":"FOR305H1"},{"type":"credits","amount":1,"courses":["FOR300H1","FOR301H1","FOR302H1","FOR303H1","FOR306H1","FOR307H1","FOR310H1"]},{"type":"course","code":"EEB321H1"}]},{"type":"and","items":[{"type":"course","code":"FOR305H1"},{"type":"credits","amount":1,"courses":["FOR300H1","FOR301H1","FOR302H1","FOR303H1","FOR306H1","FOR307H1","FOR310H1","EEB321H1"]},{"type":"text","kind":"ambiguity","text":"EEB321H1 may be one of the pool's courses rather than a course of its own"}]}]}`,
				`3 allocation p1 {"type":"course","code":"FOR400Y1"}`,
			},
			nil},
		{"a course after the pool's \";\" where another pool's \";\" separates its groups: one of the pool's (ASMAJ1100, shortened)",
			"(3.5 credits)\nSecond Year:\n2. 1.0 credit from ANT204H1; ECO220Y1, ECO227Y1; GGR270H1; INS201Y1; STA221H1\nThird Year:\nFOR305H1; 1.0 credit from FOR300H1, FOR301H1, FOR302H1, FOR303H1, FOR306H1, FOR307H1, FOR310H1; EEB321H1\nFourth Year:\nFOR400Y1",
			[]string{
				`1 allocation {"type":"credits","amount":3.5}`,
				`2 allocation p1 {"type":"credits","amount":1,"courses":["ANT204H1","ECO220Y1","ECO227Y1","GGR270H1","INS201Y1","STA221H1"]}`,
				`3 allocation p1 {"type":"and","items":[{"type":"course","code":"FOR305H1"},{"type":"credits","amount":1,"courses":["FOR300H1","FOR301H1","FOR302H1","FOR303H1","FOR306H1","FOR307H1","FOR310H1","EEB321H1"]}]}`,
				`4 allocation p1 {"type":"course","code":"FOR400Y1"}`,
			},
			nil},
		{"caps on the \"/\" pairs that leave a pool short of its amount: the pool as the lenient reading, no caps (built from ASMAJ1295)",
			"(1.5 credits)\n3) 1.5 credits from the following two groups:\nGroup 1 – Literary History: FRE318H1/ FRE319H1\nGroup 2 – Genres and Gender: FRE304H1/ FRE310H1",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":1.5},{"type":"text","kind":"ambiguity","text":"(1.5 credits)"}]}`,
				`2 allocation p1 {"type":"and","items":[{"type":"credits","amount":1.5,"courses":["FRE318H1","FRE319H1","FRE304H1","FRE310H1"]},{"type":"text","kind":"ambiguity","text":"the caps on the pool's courses leave less than its amount, so they may not apply as written"}]}`,
			},
			nil},
	})
}
