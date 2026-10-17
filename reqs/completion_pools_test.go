package reqs

import "testing"

// TestCompletionRulePools checks the pool and list grammar
// (completion_pools.go) on real snippets.
func TestCompletionRulePools(t *testing.T) {
	runComplRuleCases(t, []complRuleCase{
		{"\"may also be used to meet this requirement\" extends the pool: a union list named by the row's label (ASMIN1252, shortened)",
			"_First Year Geography Courses:_ Any course (0.5 credit) from GGR107H1 (recommended), JEG100H1, GGR101H1. 200/300-level GGR courses may also be used to meet this requirement.",
			[]string{
				`1 allocation {"type":"credits","amount":0.5,"lists":["Requirement First Year Geography Courses"]}`,
				`list: Requirement First Year Geography Courses: [{"type":"credits","courses":["GGR107H1","JEG100H1","GGR101H1"]},{"type":"credits","designators":["GGR"],"levels":[200,300]}]`,
			},
			nil},
		{"\"including joint courses\": the department's own designator, and a Review alternative at those levels (ASMAJ1305A, shortened)",
			"2. 0.5 credit from Core Geography Courses: JEG100H1, ESS262H1, GGR101H1. A 200 or 300-level Geography & Planning course, including joint courses, may also be used to meet this requirement.",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":0.5,"lists":["Requirement 2 courses"]},{"type":"and","items":[{"type":"credits","amount":0.5,"levels":[200,300]},{"type":"text","kind":"ambiguity","text":"which joint courses count isn't said: A 200 or 300-level Geography \u0026 Planning course, including joint courses, may also be used to meet this requirement"}]}]}`,
				`list: Requirement 2 courses: [{"type":"credits","courses":["JEG100H1","ESS262H1","GGR101H1"]},{"type":"credits","designators":["GGR"],"levels":[200,300]}]`,
			},
			nil},
		{"a course count with its credits; \"at least two of which must be Math or Physics half courses\" is a floor (ASSPE2030)",
			"_Introductory courses:_ Any 6 half courses or the equivalent (3.0 credits) from JEG100H1, BIO120H1/ BIO130H1, CHM135H1, CHM136H1, PHY131H1/ PHY151H1, PHY132H1/ PHY152H1, MAT130H1/ MAT135H1, at least two of which must be Math or Physics half courses",
			[]string{
				`1 allocation {"type":"credits","amount":3,"courses":["JEG100H1","BIO120H1","BIO130H1","CHM135H1","CHM136H1","PHY131H1","PHY151H1","PHY132H1","PHY152H1","MAT130H1","MAT135H1"]}`,
				`2 constraint p1 {"type":"credits","amount":1,"designators":["MAT","PHY"]}`,
				`3 limit p1 {"type":"credits","courses":["BIO120H1","BIO130H1"],"max_amount":0.5}`,
				`4 limit p1 {"type":"credits","courses":["PHY131H1","PHY151H1"],"max_amount":0.5}`,
				`5 limit p1 {"type":"credits","courses":["PHY132H1","PHY152H1"],"max_amount":0.5}`,
				`6 limit p1 {"type":"credits","courses":["MAT130H1","MAT135H1"],"max_amount":0.5}`,
			},
			nil},
		{"\"and no more\" caps the program (ASSPE1160, shortened)",
			"(10.0 credits)\n10. One lab course (0.5 credit), and no more, taken from the following: PSY319H1, PSY329H1, PSY339H1",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":10},{"type":"text","kind":"other","text":"(10.0 credits)"}]}`,
				`2 allocation p1 {"type":"credits","amount":0.5,"courses":["PSY319H1","PSY329H1","PSY339H1"]}`,
				`3 limit p1 {"type":"credits","courses":["PSY319H1","PSY329H1","PSY339H1"],"max_amount":0.5}`,
			},
			nil},
		{"a subject name is its designator; a floor restated after \"i.e.\" (ASMAJ1013)",
			"6. An additional 2.0 credits in Sociology at the 200 or 300-level (0.5 of which must be taken at the 300-level, i.e., 1.5 credits at the 200 or 300-level and 0.5 at the 300-level)",
			[]string{
				`1 allocation {"type":"credits","amount":2,"designators":["SOC"],"levels":[200,300]}`,
				`2 constraint p1 {"type":"credits","amount":0.5,"levels":[300]}`,
			},
			nil},
		{"courses named by the prefix in an aside (ASMAJ1228)",
			"3. 2.0 additional credits in Peace, Conflict and Justice courses (i.e. PCJ prefix classes) at the 300 or 400 level, including at least 0.5 at the 400 level",
			[]string{
				`1 allocation {"type":"credits","amount":2,"designators":["PCJ"],"levels":[300,400]}`,
				`2 constraint p1 {"type":"credits","amount":0.5,"levels":[400]}`,
			},
			nil},
		{"a breadth category as an alternative (ASMAJ1647A)",
			"7. 0.5 credit from Quantitative Methods: ENG286H1/ ENG287H1 or 0.5 credit in any BR=5 course",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":0.5,"courses":["ENG286H1","ENG287H1"]},{"type":"credits","amount":0.5,"breadth":[5]}]}`,
			},
			nil},
		{"a breadth category at levels (ASMAJ0463)",
			"4. 0.5 credit from CHC232H1/ CHC370H1/ JCA302H1 or any 0.5 credit from a 200/300/400-level course from Breadth Requirement Category 5: The Physical and Mathematical Universes",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":0.5,"courses":["CHC232H1","CHC370H1","JCA302H1"]},{"type":"credits","amount":0.5,"levels":[200,300,400],"breadth":[5]}]}`,
			},
			nil},
		{"filters among a pool's courses, with a floor and a recommended aside (ASSPE2738)",
			"2. 1.5 credits, including at least 0.5 credit at the 400-level, from PSL304H1/ PSL305H1/ PSL372H1/ PSL 400-level courses ( PSL440Y1, PSL445H1, PSL452H1 recommended)",
			[]string{
				`1 allocation {"type":"credits","amount":1.5,"lists":["Requirement 2 courses"]}`,
				`  note: PSL440Y1, PSL445H1, PSL452H1 recommended`,
				`2 constraint p1 {"type":"credits","amount":0.5,"levels":[400]}`,
				`list: Requirement 2 courses: [{"type":"credits","courses":["PSL304H1","PSL305H1","PSL372H1"]},{"type":"credits","designators":["PSL"],"levels":[400]}]`,
			},
			nil},
		{"a filter among courses whose alternatives are capped; the HMB run may be the list's separator, so its cap is the strict reading beside the plain courses as Review (ASMAJ0482, shortened)",
			"3. 1.0 credit from HMB430H1/ HMB470H1/ HMB472H1; JPM400Y1; PSL304H1, PSL399Y1, PSL400-series courses",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"and","items":[{"type":"credits","amount":1,"lists":["Requirement 3 courses"]},{"type":"credits","amount":0.5,"exclude":["HMB430H1","HMB470H1","HMB472H1"],"lists":["Requirement 3 courses"]}]},{"type":"and","items":[{"type":"credits","amount":1,"lists":["Requirement 3 courses"]},{"type":"text","kind":"ambiguity","text":"the \"/\" in HMB430H1/ HMB470H1/ HMB472H1 may separate the pool's courses rather than join alternatives, so more than one of them may count"}]}]}`,
				`list: Requirement 3 courses: [{"type":"credits","courses":["HMB430H1","HMB470H1","HMB472H1","JPM400Y1","PSL304H1","PSL399Y1"]},{"type":"credits","designators":["PSL"],"levels":[400]}]`,
			},
			nil},
		{"\"Excludes\" after a pool with a filter excludes from the filter (ASSPE2737, shortened)",
			"2. 1.5 credits from BCH370H1/ CSB428H1/ BCH 400-level courses ( BCH422H1, BCH425H1 recommended). Excludes BCH472Y1, BCH473Y1.",
			[]string{
				`1 allocation {"type":"credits","amount":1.5,"lists":["Requirement 2 courses"]}`,
				`  note: BCH422H1, BCH425H1 recommended`,
				`list: Requirement 2 courses: [{"type":"credits","courses":["BCH370H1","CSB428H1"]},{"type":"credits","designators":["BCH"],"levels":[400],"exclude":["BCH472Y1","BCH473Y1"]}]`,
			},
			nil},
		{"two filters of one designator are one filter (ASSPE2739)",
			"2. 1.5 credits, including at least 0.5 credit at the 400-level, from PHY 300-level courses, PHY 400-level courses",
			[]string{
				`1 allocation {"type":"credits","amount":1.5,"designators":["PHY"],"levels":[300,400]}`,
				`2 constraint p1 {"type":"credits","amount":0.5,"levels":[400]}`,
			},
			nil},
		{"a count of half courses from a titled list (ASSPE1762, shortened)",
			"2. Two courses from the BCH 400-series lecture courses: BCH422H1, BCH425H1, BCH426H1",
			[]string{
				`1 allocation {"type":"credits","amount":1,"courses":["BCH422H1","BCH425H1","BCH426H1"]}`,
			},
			nil},
		{"\"at least one 0.5 credit from X or Y\" inside a course list (ASSPE2082)",
			"BCH311H1/ PSL350H1, PCL302H1, at least one 0.5 credit from PCL367H1 or PCL368H1 (see NOTE 2)",
			[]string{
				`1 allocation {"type":"and","items":[{"type":"or","items":[{"type":"course","code":"BCH311H1"},{"type":"course","code":"PSL350H1"}]},{"type":"course","code":"PCL302H1"},{"type":"or","items":[{"type":"course","code":"PCL367H1"},{"type":"course","code":"PCL368H1"}]}]}`,
			},
			nil},
		{"\"Any N courses (X credits) from Group E\" and a note letting one course come from Group F (ASMIN1667, shortened)",
			"(4.0 credits)\n_Applications:_ Any 6 courses (3.0 credits) from Group E. One course (0.5 credit) can be from Group F.\nGroup E\nGGR217H1, GGR221H1, GGR223H1\nGroup F\nGGR201H1, GGR203H1",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":4},{"type":"text","kind":"other","text":"(4.0 credits)"}]}`,
				`2 allocation p1 {"type":"credits","amount":3,"lists":["Group E","Group F"]}`,
				`3 limit p2 {"type":"credits","lists":["Group F"],"max_amount":0.5}`,
				`list: Group E: [{"type":"credits","courses":["GGR217H1","GGR221H1","GGR223H1"]}]`,
				`list: Group F: [{"type":"credits","courses":["GGR201H1","GGR203H1"]}]`,
			},
			nil},
	})
}
