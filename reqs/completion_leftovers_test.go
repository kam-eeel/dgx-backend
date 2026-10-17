package reqs

import "testing"

// TestCompletionRuleLeftovers checks the leftover readings.
func TestCompletionRuleLeftovers(t *testing.T) {
	runComplRuleCases(t, []complRuleCase{
		{"a lead naming the courses on lines of one course and its title: their pool; one line of all courses at a level: that pool (ASMAJ1240, shortened)",
			"(1.5 credits)\n2. 1.0 credit from the SDS Foundational Courses:\n- SDS255H1: Histories of Sexuality\n- SDS256H1: Methods in Sexual Diversity Studies\n- SDS355H1: Theories of Sexuality\n4. Capstone requirement (a minimum of one 0.5 credit from the following list):\nAll 400-level SDS and JNS courses.",
			[]string{
				`1 allocation {"type":"credits","amount":1.5}`,
				`2 allocation p1 {"type":"credits","amount":1,"courses":["SDS255H1","SDS256H1","SDS355H1"]}`,
				`3 allocation p1 {"type":"credits","amount":0.5,"designators":["SDS","JNS"],"levels":[400]}`,
			},
			nil},
		{"a lead naming courses and their credits over lines that make them up: all of them (ASSPE1240, shortened)",
			"(1.5 credits)\n2. SDS Foundational Courses (1.5 credits):\n- SDS255H1: Histories of Sexuality\n- SDS256H1: Methods in Sexual Diversity Studies\n- SDS355H1: Theories of Sexuality",
			[]string{
				`1 allocation {"type":"credits","amount":1.5}`,
				`2 allocation p1 {"type":"and","items":[{"type":"course","code":"SDS255H1"},{"type":"course","code":"SDS256H1"},{"type":"course","code":"SDS355H1"}]}`,
			},
			nil},
		{"one of the following subject areas: a pool per area; an area with a rule of its own is its course and the line (ASMAJ1423, shortened)",
			"(0.5 credits)\n3. 0.5 credit from one of the following subject areas:\n- Statistics: STA302H1, STA313H1;\n- Mathematics: APM346H1, MAT327H1, MAT334H1, MAT363H1;\n- Astronomy: AST430H1, or AST425Y1 (0.5 credit from point 1 above).",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":0.5},{"type":"text","kind":"other","text":"(0.5 credits)"}]}`,
				`2 allocation p1 {"type":"or","items":[{"type":"credits","amount":0.5,"courses":["STA302H1","STA313H1"]},{"type":"credits","amount":0.5,"courses":["APM346H1","MAT327H1","MAT334H1","MAT363H1"]},{"type":"course","code":"AST430H1"},{"type":"text","kind":"other","text":"Astronomy: AST430H1, or AST425Y1 (0.5 credit from point 1 above)"}]}`,
			},
			nil},
		{"an exemption replaced by any course of a designator: the credits exempted, with the sentence (ASMAJ1295, shortened)",
			"(1.0 credits)\n2) ( FSL320H1, FSL322H1)/ FSL321Y1.\nN.B.\niii) Students exempted from FSL320H1/ FSL322H1 may replace it with any FRE course.",
			[]string{
				`1 allocation {"type":"credits","amount":1}`,
				`2 allocation p1 {"type":"or","items":[{"type":"and","items":[{"type":"course","code":"FSL320H1"},{"type":"course","code":"FSL322H1"}]},{"type":"and","items":[{"type":"credits","amount":1,"designators":["FRE"]},{"type":"text","kind":"placement","text":"Students exempted from FSL320H1/ FSL322H1 may replace it with any FRE course"}]},{"type":"course","code":"FSL321Y1"}]}`,
			},
			nil},
		{"a course and additional credits from a union, or a full course (ASMAJ0556, shortened)",
			"(1.0 credits)\n5) ( FSL420H1, 0.5 additional credit of FRE at the 200+ level or FSL at the 300+ level)/ FSL421Y1",
			[]string{
				`1 allocation {"type":"credits","amount":1}`,
				`2 allocation p1 {"type":"or","items":[{"type":"and","items":[{"type":"course","code":"FSL420H1"},{"type":"credits","amount":0.5,"exclude":["FSL420H1"],"lists":["Requirement 5 courses"]}]},{"type":"course","code":"FSL421Y1"}]}`,
				`list: Requirement 5 courses: [{"type":"credits","designators":["FRE"],"levels":[200,300,400]},{"type":"credits","designators":["FSL"],"levels":[300,400]}]`,
			},
			nil},
		{"a count of courses with a floor from a list, beside a route: and(pool, floor) as the alternative (ASSPE1667, shortened)",
			"(1.0 credits)\n_Fourth Year Courses:_ Any 2 (1.0 credit) 400-level GGR courses, at least one of which must be from Group E. Up to one 0.5 credit can be used from GGR493Y1, based on suitability of placement for this program, and approved by department.\n_Group E (Human Geography Courses):_\nGGR400H1, GGR418H1, GGR424H1, GGR321H1",
			[]string{
				`1 allocation {"type":"credits","amount":1}`,
				`2 allocation p1 {"type":"or","items":[{"type":"credits","amount":1,"designators":["GGR"],"levels":[400]},{"type":"and","items":[{"type":"credits","amount":1,"designators":["GGR"],"levels":[400],"courses":["GGR493Y1"]},{"type":"text","kind":"permission","text":"Up to one 0.5 credit can be used from GGR493Y1, based on suitability of placement for this program, and approved by department"}]}]}`,
				`3 limit p2 {"type":"credits","courses":["GGR493Y1"],"max_amount":0.5}`,
				`4 constraint p2 {"type":"credits","amount":0.5,"designators":["GGR"],"levels":[400],"lists":["Group E"]}`,
				`list: Group E (Group E (Human Geography Courses)): [{"type":"credits","courses":["GGR400H1","GGR418H1","GGR424H1","GGR321H1"]}]`,
			},
			nil},
		{"options of courses, another requirement's courses not already taken and a list below: a union (ASMIN2731, shortened)",
			"(1.5 credits)\n3. 0.5 credit from MCS226H1, MCS320H1, MCS322H1\n4. 1.0 credit from the following options: MCS329H1/ MCS329Y1; courses listed in requirement 3 not already taken; VIC109H1, VIC163H1; or cognate courses listed below\nCognate courses:\nANT412H1, ANT457H1, ARH309H1",
			[]string{
				`1 allocation {"type":"credits","amount":1.5}`,
				`2 allocation p1 {"type":"credits","amount":0.5,"courses":["MCS226H1","MCS320H1","MCS322H1"]}`,
				`3 allocation p1 {"type":"credits","amount":1,"lists":["Requirement 4 courses"]}`,
				`list: Cognate courses: [{"type":"credits","courses":["ANT412H1","ANT457H1","ARH309H1"]}]`,
				`list: Requirement 4 courses: [{"type":"credits","requirements":[2]},{"type":"credits","lists":["Cognate courses"]},{"type":"credits","courses":["MCS329H1","MCS329Y1","VIC109H1","VIC163H1"]}]`,
			},
			nil},
		{"a designator with its joint ones; options naming lists below (ASMAJ0235, ASMIN0235, shortened)",
			"(2.5 credits)\n2. 1.0 credit in 300-level CAS courses (including JHA, JPA, and SAS)\n3. 1.0 credit from 300-level CAS courses and/or 400-level courses (including JHA, JPA, and SAS)\n7. CAS450H1 **OR** 0.5 credit in a quantitative or qualitative methods course from Group C to fulfill the Research Competency required in the program.\n_Group C: Methodology_\nANT380H1, CAS450H1, CRI350H1",
			[]string{
				`1 allocation {"type":"credits","amount":2.5}`,
				`2 allocation p1 {"type":"credits","amount":1,"designators":["CAS","JHA","JPA","SAS"],"levels":[300]}`,
				`3 allocation p1 {"type":"or","items":[{"type":"credits","amount":1,"designators":["CAS","JHA","JPA","SAS"],"levels":[300,400]},{"type":"and","items":[{"type":"credits","amount":1,"lists":["Requirement 3 courses"]},{"type":"text","kind":"ambiguity","text":"1.0 credit from 300-level CAS courses and/or 400-level courses (including JHA, JPA, and SAS)"}]}]}`,
				`4 allocation p1 {"type":"or","items":[{"type":"course","code":"CAS450H1"},{"type":"credits","amount":0.5,"lists":["Group C"]}]}`,
				`list: Group C (Group C: Methodology): [{"type":"credits","courses":["ANT380H1","CAS450H1","CRI350H1"]}]`,
				`list: Requirement 3 courses: [{"type":"credits","designators":["CAS","JHA","JPA","SAS"],"levels":[300]},{"type":"credits","levels":[400]}]`,
			},
			nil},
		{"a hard-wrapped note on exclusions joins its lines; the alternative it gives is extra credits from another item's list, beside the item's courses (ASMIN0160, shortened)",
			"(1.5 credits)\n1. EEB125H1/ ENG286H1/ ESS245H1\n**Note:** If you completed STA238H1, STA248H1, and/or STA261H1 before being admitted\nto the Minor, please note that these courses are exclusions to EEB125H1, ENG286H1,\nGGR274H1, and STA130H1. You can complete ESS245H1 to meet requirement 1 or can\ninstead complete an additional 0.5 credit from the list of courses in requirement 7, for a\ntotal of 1.5 credits for requirement 7.\nIf you completed CSC148H1 before being admitted to the Minor, please note that this course is an exclusion to EEB125H1, ENG286H1, and GGR274H1. You can complete ESS245H1 or STA130H1 to meet requirement 1 or can instead complete an additional 0.5 credit from the list of courses in requirement 7, for a total of 1.5 credits for requirement 7.\n7. At least 1.0 credit from the following courses: BCB410H1/ BCB420H1/ SOC303H1/ 0.5 or 1.0 credits from a 300-/400-level capstone, topics, or independent study course(s) in which students apply data science methodologies, and with prior approval from the Associate Chair, Undergraduate Studies (Statistics).",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":1.5},{"type":"text","kind":"permission","text":"(1.5 credits)"}]}`,
				`2 allocation p1 {"type":"or","items":[{"type":"course","code":"EEB125H1"},{"type":"course","code":"ENG286H1"},{"type":"course","code":"ESS245H1"},{"type":"and","items":[{"type":"credits","amount":0.5,"courses":["BCB410H1","BCB420H1","SOC303H1"]},{"type":"text","kind":"other","text":"You can complete ESS245H1 to meet requirement 1 or can instead complete an additional 0.5 credit from the list of courses in requirement 7, for a total of 1.5 credits for requirement 7"}]}]}`,
				`3 allocation p1 {"type":"or","items":[{"type":"credits","amount":1,"courses":["BCB410H1","BCB420H1","SOC303H1"]},{"type":"text","kind":"permission","text":"0.5 or 1.0 credits from a 300-/400-level capstone, topics, or independent study course(s) in which students apply data science methodologies, and with prior approval from the Associate Chair, Undergraduate Studies (Statistics)"}]}`,
				`note: If you completed STA238H1, STA248H1, and/or STA261H1 before being admitted to the Minor, please note that these courses are exclusions to EEB125H1, ENG286H1, GGR274H1, and STA130H1`,
				`note: If you completed CSC148H1 before being admitted to the Minor, please note that this course is an exclusion to EEB125H1, ENG286H1, and GGR274H1`,
			},
			nil},
	})
}
