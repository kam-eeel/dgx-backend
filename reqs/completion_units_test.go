package reqs

import "testing"

// TestCompletionRuleUnits checks the node-language additions of Task 10
// (completion_units.go: exclude_designators, max_course_amount, pair items,
// a lead's filter joined with its members) on real snippets.
func TestCompletionRuleUnits(t *testing.T) {
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
