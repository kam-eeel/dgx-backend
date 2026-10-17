package completion

import (
	"testing"
)

// TestCompletionBindingNotes checks the notes read as rules (the "formerly"
// codes in item asides, the advice zones) on snippets of real programs.
func TestCompletionBindingNotes(t *testing.T) {
	runComplRuleCases(t, []ruleCase{
		{"\"(formerly X)\" in a pool of a choice: X joins the pool, the choice stays (ASMAJ2207, shortened)",
			"(2.0 credits)\n1. Two of the following selections:\n- 1.0 credit from the following First year Economics courses: ECO101H1, ECO102H1, ECO105Y1;\n- 1.0 credit from the following First year Political Science courses: POL101H1 (formerly POL101Y1), POL106H1, POL107H1, POL109H1;\n- 1.0 credit from the following First year Sociology courses: SOC100H1, SOC150H1",
			[]string{
				`1 allocation {"type":"credits","amount":2}`,
				`2 allocation p1 {"type":"choose","items":[{"type":"credits","amount":1,"courses":["ECO101H1","ECO102H1","ECO105Y1"]},{"type":"credits","amount":1,"courses":["POL101H1","POL106H1","POL107H1","POL109H1","POL101Y1"]},{"type":"credits","amount":1,"courses":["SOC100H1","SOC150H1"]}],"count":2}`,
			},
			nil},
		{"\"(_formerly X_)\" after required courses: or(course, X) (ASMAJ1021, shortened)",
			"(2.0 credits)\n1. CHC313H1, JSV200H1, JSV201H1 (_formerly SMC272H1_), JSV202H1 (_formerly SMC271H1_)",
			[]string{
				`1 allocation {"type":"credits","amount":2}`,
				`2 allocation p1 {"type":"and","items":[{"type":"course","code":"CHC313H1"},{"type":"course","code":"JSV200H1"},{"type":"or","items":[{"type":"course","code":"JSV201H1"},{"type":"course","code":"SMC272H1"}]},{"type":"or","items":[{"type":"course","code":"JSV202H1"},{"type":"course","code":"SMC271H1"}]}]}`,
			},
			nil},
		{"a list line with a mistyped code is a text alternative of its row (ASCER0828, shortened)",
			"(2.5 credits)\n1. HIS265Y1\n2. 0.5 credit from CAR429H1/ CDN335H1/ CDN440H1\n3. 1.0 additional credit, either from courses not already taken to fulfill Requirement 2, or from any of the following:\n- Anthropology: ANT464H1\n- Caribbean Studies: CAR120Y1/ CAR215H1/ CAR32H1/ CAR428H1",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":2.5},{"type":"text","kind":"other","text":"(2.5 credits)"}]}`,
				`2 allocation p1 {"type":"course","code":"HIS265Y1"}`,
				`3 allocation p1 {"type":"credits","amount":0.5,"courses":["CAR429H1","CDN335H1","CDN440H1"]}`,
				`4 allocation p1 {"type":"or","items":[{"type":"credits","amount":1,"lists":["Requirement 3 courses"]},{"type":"text","kind":"other","text":"Caribbean Studies: CAR120Y1/ CAR215H1/ CAR32H1/ CAR428H1"}]}`,
				`list: Requirement 3 courses: [{"type":"credits","requirements":[3]},{"type":"credits","courses":["ANT464H1","CAR120Y1","CAR215H1","CAR428H1"]}]`,
			},
			nil},
		{"\"Students may count either X or Y\": a limit of one course's weight (ASMAJ1376, shortened)",
			"(8.0 credits)\nFourth Year: Additional 200/300/400-level CHM courses to a total of 8.0 credits ( CHM299H1, CHM299Y1 excluded). Students may count either CHM395Y1 or CHM499Y1 towards Chemistry Major requirements.",
			[]string{
				`1 allocation {"type":"credits","amount":8,"designators":["CHM"],"levels":[200,300,400],"exclude":["CHM299H1","CHM299Y1"]}`,
				`2 limit p1 {"type":"credits","courses":["CHM395Y1","CHM499Y1"],"max_amount":1}`,
			},
			nil},
		{"\"seek early approval\" for other courses sits beside each \"not exhaustive\" alternative (ASMIN0135, shortened)",
			"(1.0 credit)\n1. 1.0 credit from AMS courses or from the eligible courses listed below.\n*NOTE 1: Other 300+ level courses with American content may be allowed; students should seek early approval of program credit for such courses.\n**Disciplinary/Thematic Cluster 1: Politics and Economics**\nECO306H1/ POL315H1/ POL326H1\n*NOTE: This list of eligible courses is not exhaustive and some courses may not be offered every year.",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":1},{"type":"text","kind":"other","text":"(1.0 credit)"}]}`,
				`2 allocation p1 {"type":"or","items":[{"type":"credits","amount":1,"lists":["Requirement 1 courses"]},{"type":"text","kind":"other","text":"This list of eligible courses is not exhaustive and some courses may not be offered every year"},{"type":"text","kind":"permission","text":"Other 300+ level courses with American content may be allowed; students should seek early approval of program credit for such courses"}]}`,
				`list: Cluster 1 (Disciplinary/Thematic Cluster 1: Politics and Economics): [{"type":"credits","courses":["ECO306H1","POL315H1","POL326H1"]}]`,
				`list: Requirement 1 courses: [{"type":"credits","designators":["AMS"]},{"type":"credits","lists":["Cluster 1"]}]`,
			},
			nil},
		{"a list no row uses, in a program whose items fill the total, stays a note as written (ASMIN1058, shortened)",
			"Completion of the program requires 1.5 credits, meeting the following requirements:\n1. EAS103H1 and EAS105H1;\n2. 0.5 Society-Culture credits;\n_Core Courses_\nEAS103H1, EAS105H1, EAS209H1\n_Society-Culture Courses_\nEAS193H1, EAS194H1, EAS195H1",
			[]string{
				`1 allocation {"type":"credits","amount":1.5}`,
				`2 allocation p1 {"type":"and","items":[{"type":"course","code":"EAS103H1"},{"type":"course","code":"EAS105H1"}]}`,
				`3 allocation p1 {"type":"credits","amount":0.5,"lists":["Society-Culture Courses"]}`,
				`list: Society-Culture Courses: [{"type":"credits","courses":["EAS193H1","EAS194H1","EAS195H1"]}]`,
				`note: Core Courses: EAS103H1, EAS105H1, EAS209H1`,
			},
			nil},
		{"a note that the capstone doesn't fulfil item 1, whose pool doesn't hold it, restates the claims (ASMAJ1525, shortened)",
			"(1.0 credit)\n1. 0.5 credit at the 400-level, selected from the following: RLG461H1, RLG462H1.\n2. 0.5 credit from the following capstone integrative courses: RLG404H1, RLG405H1 (Note: this does not fulfil the 400-level requirement as described in #1 above.)",
			[]string{
				`1 allocation {"type":"credits","amount":1}`,
				`2 allocation p1 {"type":"credits","amount":0.5,"courses":["RLG461H1","RLG462H1"]}`,
				`3 allocation p1 {"type":"credits","amount":0.5,"courses":["RLG404H1","RLG405H1"]}`,
			},
			nil},
		{"a note that the capstone doesn't fulfil item 1, whose lists don't hold it, restates the claims (ASSPE0151, shortened)",
			"(1.0 credit)\n1. 0.5 credit at the 400-level in your area of focus selected from Department for the Study of Religion courses.\n   **Judaism:** RLG430H1, RLG431H1\n   **Islam:** RLG458H1, RLG481H1\n2. 0.5 credit from the following capstone integrative courses: RLG404H1, RLG405H1 (note: this does not fulfil the 400-level requirement as described in #1 above.)",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":1},{"type":"text","kind":"content","text":"(1.0 credit)"}]}`,
				`2 allocation p1 {"type":"and","items":[{"type":"credits","amount":0.5,"levels":[400],"lists":["Judaism","Islam"]},{"type":"text","kind":"content","text":"in your area of focus"}]}`,
				`3 allocation p1 {"type":"credits","amount":0.5,"courses":["RLG404H1","RLG405H1"]}`,
				`list: Judaism: [{"type":"credits","courses":["RLG430H1","RLG431H1"]}]`,
				`list: Islam: [{"type":"credits","courses":["RLG458H1","RLG481H1"]}]`,
			},
			nil},
		{"category labels in brackets name parts of the pool: no notes (ASSPE1019, shortened)",
			"(2.0 credits)\n1. Higher years: 2.0 credits from NMC230H1, NMC238H1 (History & Society courses) and JHM307H1, JMH385H1 (Religion & Thought courses)",
			[]string{
				`1 allocation {"type":"credits","amount":2}`,
				`2 allocation p1 {"type":"credits","amount":2,"courses":["NMC230H1","NMC238H1","JHM307H1","JMH385H1"]}`,
			},
			nil},
		{"a lead that says the streams below are not a requirement makes them advice (ASMAJ0652, shortened)",
			"(1.0 credit)\n1. 1.0 credit from HIS106Y1, HIS208Y1\n**Thematic Streams**\nAlthough thematic streams are not an official program requirement, you may find it enriching to follow a particular theme across different time periods or geographical divisions.\n_Medieval_\nHIS220Y1, HIS303H1\n_Women and Gender_\nHIS297Y1, HIS302H1",
			[]string{
				`1 allocation {"type":"credits","amount":1}`,
				`2 allocation p1 {"type":"credits","amount":1,"courses":["HIS106Y1","HIS208Y1"]}`,
				`note: Although thematic streams are not an official program requirement, you may find it enriching to follow a particular theme across different time periods or geographical divisions: Medieval: HIS220Y1, HIS303H1 Women and Gender: HIS297Y1, HIS302H1`,
			},
			nil},
		{"\"(as listed below)\" ends a lead into advice lines (ASMAJ2364, shortened)",
			"(1.0 credit)\n1. 1.0 credit at 300+ level from: CSB; EEB\nNOTE: Students who wish to focus on either **plant or microbial biology**, or **animal biology** should take courses in 1. that concentrate in these subject areas (as listed below).\nCSB and EEB courses in **plant or microbial biology**: CSB340H1, CSB350H1\nCSB and EEB courses in **animal biology**: CSB325H1, CSB327H1",
			[]string{
				`1 allocation {"type":"credits","amount":1}`,
				`2 allocation p1 {"type":"credits","amount":1,"designators":["CSB","EEB"],"levels":[300,400]}`,
				`note: Students who wish to focus on either plant or microbial biology, or animal biology should take courses in 1. that concentrate in these subject areas (as listed below): CSB and EEB courses in plant or microbial biology: CSB340H1, CSB350H1 CSB and EEB courses in animal biology: CSB325H1, CSB327H1`,
			},
			nil},
	})
}

// TestCompletionRestatedNotes checks the passes over the last binding notes.
func TestCompletionRestatedNotes(t *testing.T) {
	runComplRuleCases(t, []ruleCase{
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
