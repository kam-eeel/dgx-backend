package reqs

import "testing"

// TestCompletionPoolNodeFeatures checks exclude_designators, max_course_amount,
// pair items and a lead's filter joined with its members.
func TestCompletionPoolNodeFeatures(t *testing.T) {
	runComplRuleCases(t, []complRuleCase{
		{"\"8.0 credits in RSM and 8.0 credits in non-RSM courses\"; a constraint over the transcript (16.0 credits, more than the program's 15.0), so no marker; the note on MGT100H1 joins the row (ASSPE2038, shortened)",
			"(15.0 credits)\n1. ( ECO101H1, ECO102H1), RSM100H1/ MGT100H1\nNotes:\n- Students must take a minimum of 8.0 credits in RSM and 8.0 credits in non-RSM courses. MGT100H1 is considered equivalent to RSM100H1 and counts as 0.5 credit in RSM courses towards this requirement.",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":15},{"type":"text","kind":"other","text":"(15.0 credits)"}]}`,
				`2 allocation p1 {"type":"and","items":[{"type":"course","code":"ECO101H1"},{"type":"course","code":"ECO102H1"},{"type":"or","items":[{"type":"course","code":"RSM100H1"},{"type":"course","code":"MGT100H1"}]}]}`,
				`3 constraint p1 scope:transcript {"type":"and","items":[{"type":"credits","amount":8,"designators":["RSM"],"courses":["MGT100H1"]},{"type":"credits","amount":8,"exclude":["MGT100H1"],"exclude_designators":["RSM"]}]}`,
			},
			map[int]string{3: "Students must take a minimum of 8.0 credits in RSM and 8.0 credits in non-RSM courses\nMGT100H1 is considered equivalent to RSM100H1 and counts as 0.5 credit in RSM courses towards this requirement"}},
		{"\"... and the 8.0 RSM credit requirement\": the CR/NCR limit on the program, and the same limit over the transcript on the courses the RSM constraint counts (ASSPE2431, shortened)",
			"(12.5 credits)\n1. ( ECO101H1, ECO102H1), RSM100H1/ MGT100H1\nNotes:\n- Students must take a minimum of 8.0 credits in RSM and 8.0 credits in non-RSM courses. MGT100H1 is considered equivalent to RSM100H1 and counts as 0.5 credit in RSM courses towards this requirement.\n- The CR/NCR option cannot be used for any program completion requirements, and the 8.0 RSM credit requirement. Refer to the RC website for CR/NCR details.",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":12.5},{"type":"text","kind":"other","text":"(12.5 credits)"}]}`,
				`2 allocation p1 {"type":"and","items":[{"type":"course","code":"ECO101H1"},{"type":"course","code":"ECO102H1"},{"type":"or","items":[{"type":"course","code":"RSM100H1"},{"type":"course","code":"MGT100H1"}]}]}`,
				`3 constraint p1 scope:transcript {"type":"and","items":[{"type":"credits","amount":8,"designators":["RSM"],"courses":["MGT100H1"]},{"type":"credits","amount":8,"exclude":["MGT100H1"],"exclude_designators":["RSM"]}]}`,
				`4 limit p1 {"type":"credits","cr_ncr":true,"max_amount":0}`,
				`5 limit p1 scope:transcript {"type":"credits","designators":["RSM"],"courses":["MGT100H1"],"cr_ncr":true,"max_amount":0}`,
				`note: Refer to the RC website for CR/NCR details`,
			},
			map[int]string{5: "The CR/NCR option cannot be used for any program completion requirements, and the 8.0 RSM credit requirement"}},
		{"\"Courses offered by other academic units may not be counted\": a limit of 0 on other designators, but the language courses the text adds (ASMIN0151, shortened)",
			"(4.0 credits)\n1. RLG200H1\n2. 1.5 additional Department for the Study of Religion credits\nNotes:\n- JCR, JNR and JSR courses will be counted as Department for the Study of Religion courses. Courses offered by other academic units may not be counted towards this program.\n- Up to one language course totaling 0.5 credit in Modern Hebrew ( MHB155H1, MHB156H1) will count toward the Department for the Study of Religion courses required to complete any Minor Program in Religion.",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":4},{"type":"text","kind":"other","text":"(4.0 credits)"}]}`,
				`2 allocation p1 {"type":"course","code":"RLG200H1"}`,
				`3 allocation p1 {"type":"credits","amount":1.5,"designators":["RLG","JCR","JNR","JSR"],"courses":["MHB155H1","MHB156H1"]}`,
				`4 limit p1 {"type":"credits","exclude":["MHB155H1","MHB156H1"],"exclude_designators":["RLG","JCR","JNR","JSR"],"max_amount":0}`,
				`5 limit p1 {"type":"credits","courses":["MHB155H1","MHB156H1"],"max_amount":0.5}`,
			},
			nil},
		{"\"any FRE or FCS half-course at the 200+ level\": max_course_amount (ASMIN1135)",
			"4) 0.5 credit selected from CDN202H1, JFG388H1/ JFL388H1/ JFL484H1, or any FRE or FCS half-course at the 200+ level",
			[]string{
				`1 allocation {"type":"credits","amount":0.5,"lists":["Requirement 4 courses"]}`,
				`2 limit p1 {"type":"credits","courses":["JFG388H1","JFL388H1","JFL484H1"],"max_amount":0.5}`,
				`list: Requirement 4 courses: [{"type":"credits","courses":["CDN202H1","JFG388H1","JFL388H1","JFL484H1"]},{"type":"credits","designators":["FRE","FCS"],"levels":[200,300,400],"max_course_amount":0.5}]`,
			},
			nil},
		{"a pair lighter than the amount is a pair item of the row's list (ASSPE0231)",
			"_Higher Years (required)_:\n1. 2.0 credits from the following: PHL200Y1/( PHL205H1, PHL206H1)/ PHL210Y1",
			[]string{
				`1 allocation {"type":"credits","amount":2,"lists":["Requirement Higher Years (required) 1 courses"]}`,
				`list: Requirement Higher Years (required) 1 courses: [{"type":"credits","courses":["PHL200Y1","PHL210Y1"]},{"type":"and","items":[{"type":"course","code":"PHL205H1"},{"type":"course","code":"PHL206H1"}]}]`,
			},
			nil},
		{"a course, a pool, then a pair or a full course: the pair is not in the pool (ASMAJ0525)",
			"2) FRE272H1, 1.0 credit selected from FRE210H1/ FRE245H1/ FRE273H1/ FRE379H1/ JFL388H1/ JFG388H1, ( FSL320H1, FSL322H1)/ FSL321Y1.",
			[]string{
				`1 allocation {"type":"and","items":[{"type":"course","code":"FRE272H1"},{"type":"credits","amount":1,"courses":["FRE210H1","FRE245H1","FRE273H1","FRE379H1","JFL388H1","JFG388H1"]},{"type":"or","items":[{"type":"and","items":[{"type":"course","code":"FSL320H1"},{"type":"course","code":"FSL322H1"}]},{"type":"course","code":"FSL321Y1"}]}]}`,
			},
			nil},
		{"the lead's filter and the listed courses, as written; the note restating it joins the row (ASMAJ1165, shortened)",
			"7. Further Topics: 2.0 APM/MAT credits at the 300+ level including 0.5 credit at the 400-level: ACT230H1/ ACT240H1/ APM236H1/ APM346H1/ any APM400-level course/ MAT309H1/ any MAT 400-level course/ PSL432H1\n**Notes:**\n- Requirement 7: students can take any combination of 300+ and 400+ level APM or MAT courses that adds up to 2.0 credits, with the condition that at least a 0.5 credit is at the 400+ level.",
			[]string{
				`1 allocation {"type":"credits","amount":2,"lists":["Requirement 7 courses"]}`,
				`2 constraint p1 {"type":"credits","amount":0.5,"levels":[400]}`,
				`list: Requirement 7 courses: [{"type":"credits","courses":["ACT230H1","ACT240H1","APM236H1","APM346H1","MAT309H1","PSL432H1"]},{"type":"credits","designators":["APM"],"levels":[300,400]},{"type":"credits","designators":["MAT"],"levels":[300,400]}]`,
			},
			map[int]string{1: "7. Further Topics: 2.0 APM/MAT credits at the 300+ level including 0.5 credit at the 400-level: ACT230H1/ ACT240H1/ APM236H1/ APM346H1/ any APM400-level course/ MAT309H1/ any MAT 400-level course/ PSL432H1\nstudents can take any combination of 300+ and 400+ level APM or MAT courses that adds up to 2.0 credits, with the condition that at least a 0.5 credit is at the 400+ level"}},
		{"a pair counted in lieu of 0.5 credit: a pair item with weight 0.5 (ASSPE2722, shortened)",
			"_Third Year and Higher (Electives, 5.5 credits):_\n2. 4.5 additional credits from 300+ level ECO courses\n_Notes_:\n4. ( MAT235H1, MAT236H1)/( MAT235Y1/ MAT237Y1)/ ECO210H1 is strongly recommended for this program. Students taking one of these courses can have it count in lieu of 0.5 300-level course required for this program.",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":4.5,"designators":["ECO"],"levels":[300,400]},{"type":"and","items":[{"type":"credits","amount":4,"designators":["ECO"],"levels":[300,400]},{"type":"credits","amount":0.5,"lists":["Requirement 2 substitutes"]}]}]}`,
				`list: Requirement 2 substitutes: [{"type":"and","items":[{"type":"course","code":"MAT235H1"},{"type":"course","code":"MAT236H1"}],"weight":0.5},{"type":"credits","courses":["MAT235Y1","MAT237Y1"],"weight":0.5},{"type":"credits","courses":["ECO210H1"]}]`,
				`note: ( MAT235H1, MAT236H1)/( MAT235Y1/ MAT237Y1)/ ECO210H1 is strongly recommended for this program`,
			},
			nil},
	})
}

// TestCompletionRuleStreams checks pools written inside an item.
func TestCompletionRuleStreams(t *testing.T) {
	runComplRuleCases(t, []complRuleCase{
		{"a stream lead over its parts and a prerequisite line (ASMAJ1445A, shortened)",
			"Second Year and Higher:\n- 3.0 credits from Perception and Attention stream courses\n  - PHL232H1/ PSY280H1; and 2.0 credits of any of the following: COG260H1/ COG344H1/ PSY496H1\n  - For those with the appropriate prerequisites: CSC207H1/ CSC320H1",
			[]string{
				`1 allocation {"type":"credits","amount":3,"courses":["PHL232H1","PSY280H1","COG260H1","COG344H1","PSY496H1","CSC207H1","CSC320H1"]}`,
				`2 constraint p1 {"type":"and","items":[{"type":"or","items":[{"type":"course","code":"PHL232H1"},{"type":"course","code":"PSY280H1"}]},{"type":"credits","amount":2,"courses":["COG260H1","COG344H1","PSY496H1","CSC207H1","CSC320H1"]}]}`,
			},
			nil},
		{"a prerequisite line joins the pool above it (ASMAJ1445B, shortened)",
			"Second Year and Higher:\n- 1.5 credits of any of the following: COG260H1/ LIN232H1/ PSY496H1\n  - For those with the appropriate prerequisites: CSC401H1/ CSC485H1",
			[]string{
				`1 allocation {"type":"credits","amount":1.5,"courses":["COG260H1","LIN232H1","PSY496H1","CSC401H1","CSC485H1"]}`,
			},
			nil},
		{"courses, a pool with floors, and the credit a full-year course gives in lieu (ASMAJ1387, shortened)",
			"Third and Fourth Years:\nBCH311H1/ CSB349H1/ MGY311Y1; 2.5 credits from MGY314H1, MGY315H1, MGY420H1, BCH428H1 (at least 0.5 credit must be a laboratory course and at least 0.5 credit must be a 400-level course, 0.5 credit additional are given if taking MGY311Y1 in lieu of BCH311H1/ CSB349H1)",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"and","items":[{"type":"or","items":[{"type":"course","code":"BCH311H1"},{"type":"course","code":"CSB349H1"}]},{"type":"credits","amount":2.5,"courses":["MGY314H1","MGY315H1","MGY420H1","BCH428H1"]}]},{"type":"and","items":[{"type":"course","code":"MGY311Y1"},{"type":"credits","amount":2,"courses":["MGY314H1","MGY315H1","MGY420H1","BCH428H1"]}]}]}`,
				`2 constraint p1 {"type":"and","items":[{"type":"credits","amount":0.5,"courses":["MGY314H1","MGY315H1","MGY420H1","BCH428H1"]},{"type":"text","kind":"content","text":"at least 0.5 credit must be a laboratory course"}]}`,
				`3 constraint p1 {"type":"credits","amount":0.5,"levels":[400]}`,
			},
			nil},
		{"parts joined by \"plus\", one of them additional credits of a designator with a floor on a list (ASSPE2584, shortened)",
			"Any Year: (5.0 credits)\n( PHL345H1/ PHL347H1), PHL355H1, plus 1.0 credit of ( PHL325H1/ PHL331H1/ PHL332H1) plus 2.0 additional PHL credits, at least 0.5 credit of which must be from Course Group 2 - Value Theory\n_Group 2 - Value Theory:_\nPHL265H1, PHL295H1, PHL365H1",
			[]string{
				`1 allocation null`,
				`2 allocation p1 {"type":"and","items":[{"type":"or","items":[{"type":"course","code":"PHL345H1"},{"type":"course","code":"PHL347H1"}]},{"type":"course","code":"PHL355H1"},{"type":"credits","amount":1,"courses":["PHL325H1","PHL331H1","PHL332H1"]}]}`,
				`3 allocation p1 {"type":"credits","amount":2,"designators":["PHL"]}`,
				`4 constraint p3 {"type":"credits","amount":0.5,"lists":["Group 2"]}`,
				`list: Group 2 (Group 2 - Value Theory): [{"type":"credits","courses":["PHL265H1","PHL295H1","PHL365H1"]}]`,
			},
			nil},
	})
}

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
