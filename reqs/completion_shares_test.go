package reqs

import "testing"

// TestCompletionShares checks the share routes and the language and stream pools.
func TestCompletionShares(t *testing.T) {
	runComplRuleCases(t, []complRuleCase{
		{"the share a floor leaves over is approved: the floor's designators and the listed courses make the total (ASMAJ2665, shortened)",
			"(6.0 credits)\nAt least 5.0 credits must be NMC and/or NML courses. 1.0 credit may be taken from courses offered by other departments, pending approval by the Associate Chair Undergraduate of NMC.\n1. 0.5 credit from NMC101H1/ NMC102H1/ JSN242H1",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":6,"designators":["NMC","NML"],"courses":["JSN242H1"]},{"type":"and","items":[{"type":"credits","amount":6},{"type":"text","kind":"permission","text":"1.0 credit may be taken from courses offered by other departments, pending approval by the Associate Chair Undergraduate of NMC"}]}]}`,
				`2 allocation p1 {"type":"credits","amount":0.5,"courses":["NMC101H1","NMC102H1","JSN242H1"]}`,
				`3 constraint p1 {"type":"credits","amount":5,"designators":["NMC","NML"]}`,
			},
			nil},
		{"one or two languages of a designator: the designator's credits and the languages as content (ASSPE2669, shortened)",
			"2. 4.0 credits in one or two primary source languages (with NML designator) from among the following languages: Arabic, Hebrew, Persian, Turkish.",
			[]string{
				`1 allocation {"type":"and","items":[{"type":"credits","amount":4,"designators":["NML"]},{"type":"text","kind":"content","text":"4.0 credits in one or two primary source languages (with NML designator) from among the following languages: Arabic, Hebrew, Persian, Turkish"}]}`,
			},
			nil},
		{"two streams or approved courses: the streams' lists, or the approval (ASSPE2669, shortened)",
			"4. 1.0 credit in Ancient or Medieval stream courses or those from a list of pre-approved courses offered by other departments.\n_Ancient Stream:_\nNMC196H1, NMC199H1\n_Medieval Stream:_\nNMC201H1, JSN242H1",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":1,"courses":["NMC196H1","NMC199H1","NMC201H1","JSN242H1"]},{"type":"text","kind":"permission","text":"those from a list of pre-approved courses offered by other departments"}]}`,
			},
			nil},
	})
}
