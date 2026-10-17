package reqs

import "testing"

// TestCompletionRestatedNotes checks the passes over the last binding notes
// (completion_restated.go) on snippets of real programs.
func TestCompletionRestatedNotes(t *testing.T) {
	runComplRuleCases(t, []complRuleCase{
		{"an unused list that only names required courses restates them: its note goes (ASMAJ0797, shortened)",
			"(3.0 credits)\nFirst Year:\nCIN105Y1\nSecond Year:\nCIN201Y1\nThird Year:\nCIN301Y1\n_Group A: Foundations_\nCIN105Y1, CIN201Y1, CIN301Y1",
			[]string{
				`1 allocation {"type":"credits","amount":3}`,
				`2 allocation p1 {"type":"course","code":"CIN105Y1"}`,
				`3 allocation p1 {"type":"course","code":"CIN201Y1"}`,
				`4 allocation p1 {"type":"course","code":"CIN301Y1"}`,
			},
			nil},
		{"a cognate share beside a total of any credits: the items' designator, or any with the approval, and the rest of the designator a floor (ASSPE1400, shortened)",
			"(10.0 credits)\n1. GER100Y1, GER200Y1, GER300Y1, GER400H1\n2. At least 2.5 additional credits in GER courses on the 300-level\n3. Up to 2.0 credits in identified or preapproved cognate courses may count (i.e. courses offered by other departments containing a relevant Germanic component and a final paper on a Germanic topic); please inquire at the Department and get approved in advance.",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":10,"designators":["GER"]},{"type":"and","items":[{"type":"credits","amount":10},{"type":"text","kind":"permission","text":"Up to 2.0 credits in identified or preapproved cognate courses may count (i.e. courses offered by other departments containing a relevant Germanic component and a final paper on a Germanic topic); please inquire at the Department and get approved in advance"}]}]}`,
				`2 constraint p1 {"type":"credits","amount":8,"designators":["GER"]}`,
				`3 allocation p1 {"type":"and","items":[{"type":"course","code":"GER100Y1"},{"type":"course","code":"GER200Y1"},{"type":"course","code":"GER300Y1"},{"type":"course","code":"GER400H1"}]}`,
				`4 allocation p1 {"type":"credits","amount":2.5,"designators":["GER"],"levels":[300]}`,
			},
			nil},
		{"a cap lifted for another program's students is the total's lenient reading (ASMIN1165, shortened)",
			"**4.0 credits, including 1.0 at the 300+ level**\n1. Calculus: 2.0 credits from ( MAT130H1/ MAT135H1, MAT136H1)/ MAT137Y1, MAT235Y1/ MAT237Y1\n2. Algebra: MAT223H1/ MAT240H1\n3. Further Fundamentals in Mathematics: MAT224H1/ MAT244H1\n4. Advanced Topics: 1.0 credit at the 300+ level from APM/MAT courses\n**Notes:**\n- Only 0.5 credit of APM306Y1/ MAT329Y1 may count toward the Minor program except for student enrolled in the Education and Society Minor may apply 1.0 credit of MAT329Y1 toward the Minor program.",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":4},{"type":"and","items":[{"type":"credits","amount":3.5},{"type":"course","code":"MAT329Y1"},{"type":"text","kind":"other","text":"except for student enrolled in the Education and Society Minor may apply 1.0 credit of MAT329Y1 toward the Minor program"}]}]}`,
				`2 constraint p1 {"type":"credits","amount":1,"levels":[300,400]}`,
				`3 allocation p1 {"type":"and","items":[{"type":"or","items":[{"type":"and","items":[{"type":"or","items":[{"type":"course","code":"MAT130H1"},{"type":"course","code":"MAT135H1"}]},{"type":"course","code":"MAT136H1"}]},{"type":"course","code":"MAT137Y1"}]},{"type":"or","items":[{"type":"course","code":"MAT235Y1"},{"type":"course","code":"MAT237Y1"}]}]}`,
				`4 allocation p1 {"type":"or","items":[{"type":"course","code":"MAT223H1"},{"type":"course","code":"MAT240H1"}]}`,
				`5 allocation p1 {"type":"or","items":[{"type":"course","code":"MAT224H1"},{"type":"course","code":"MAT244H1"}]}`,
				`6 allocation p1 {"type":"credits","amount":1,"designators":["APM","MAT"],"levels":[300,400]}`,
				`7 limit p1 {"type":"credits","courses":["APM306Y1","MAT329Y1"],"max_amount":0.5}`,
			},
			nil},
		{"a topics course that may count twice is the content alternative of the pool that holds it (ASMIN1337, shortened)",
			"(3.0 credits)\n1. 1.0 credits in introductory courses: DHU235H1 and DHU236H1\n2. 2.0 credits from the groups listed below.\n- DHU339H1 can be taken twice if the topic is different.\nThe following courses may count towards program completion:\nDigital Humanities: DHU334H1/ DHU335H1/ DHU339H1\nEnglish: ENG286H1/ ENG287H1",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":3},{"type":"text","kind":"content","text":"(3.0 credits)"}]}`,
				`2 allocation p1 {"type":"and","items":[{"type":"course","code":"DHU235H1"},{"type":"course","code":"DHU236H1"}]}`,
				`3 allocation p1 {"type":"or","items":[{"type":"credits","amount":2,"lists":["Digital Humanities","English"]},{"type":"text","kind":"content","text":"DHU339H1 can be taken twice if the topic is different"}]}`,
				`list: Digital Humanities: [{"type":"credits","courses":["DHU334H1","DHU335H1","DHU339H1"]}]`,
				`list: English: [{"type":"credits","courses":["ENG286H1","ENG287H1"]}]`,
				`note: The following courses may count towards program completion:`,
			},
			nil},
		{"the omission's sentence joins the total; its advice stays a note (ASSPE1687, shortened)",
			"(1.0 credit)\n- Students with a strong background in an object-oriented language such as Python, Java or C++ may omit CSC108H1 and proceed directly with CSC148H1.\n- There is no need to replace the missing half-credit for program completion; however, please base your course choice on what you are ready to take, not on \"saving\" a half-credit.\nFirst year:\nCSC108H1, CSC148H1",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":1},{"type":"and","items":[{"type":"credits","amount":0.5},{"type":"not","items":[{"type":"course","code":"CSC108H1"}]}]}]}`,
				`2 allocation p1 {"type":"and","items":[{"type":"or","items":[{"type":"course","code":"CSC108H1"},{"type":"not","items":[{"type":"course","code":"CSC108H1"}]}]},{"type":"course","code":"CSC148H1"}]}`,
				`note: however, please base your course choice on what you are ready to take, not on "saving" a half-credit`,
			},
			nil},
	})
}
