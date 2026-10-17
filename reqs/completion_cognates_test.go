package reqs

import "testing"

// TestCompletionRuleCognates checks cognate caps read as union pools
// (completion_cognates.go) on real snippets.
func TestCompletionRuleCognates(t *testing.T) {
	runComplRuleCases(t, []complRuleCase{
		{"substitutions for a pool's credits: a union with a limit on each share, the approved courses the route (ASMIN0382, shortened)",
			"(4.0 credits)\n1. CLA160H1/ CLA170H1\n2. 3.5 credits in CLA courses at the 200+ level, including 1.0 credit at the 300+ level\nSubstitutions:\n1. Up to 2.0 credits from approved courses involving the ancient Greek and Roman civilizations taught by other departments, notably Art History and Philosophy, may be substituted for the 3.5 credits in CLA courses from Requirement 2.\n_Art History:_\nFAH206H1, FAH207H1\n_Philosophy:_\nPHL200Y1, PHL301H1\n2. Up to 1.0 credit in GRK or LAT may be substituted for the 3.5 credits in CLA courses from Requirement 2.",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":4},{"type":"text","kind":"permission","text":"(4.0 credits)"}]}`,
				`2 allocation p1 {"type":"or","items":[{"type":"course","code":"CLA160H1"},{"type":"course","code":"CLA170H1"}]}`,
				`3 allocation p1 {"type":"or","items":[{"type":"credits","amount":3.5,"lists":["Requirement 2 courses"]},{"type":"text","kind":"permission","text":"Up to 2.0 credits from approved courses involving the ancient Greek and Roman civilizations taught by other departments, notably Art History and Philosophy, may be substituted for the 3.5 credits in CLA courses from Requirement 2"}]}`,
				`4 constraint p3 {"type":"or","items":[{"type":"credits","amount":1,"designators":["CLA"],"levels":[300,400]},{"type":"text","kind":"permission","text":"3.5 credits in CLA courses at the 200+ level, including 1.0 credit at the 300+ level"}]}`,
				`5 limit p3 {"type":"credits","courses":["FAH206H1","FAH207H1","PHL200Y1","PHL301H1"],"max_amount":2}`,
				`6 limit p3 {"type":"credits","designators":["GRK","LAT"],"max_amount":1}`,
				`list: Requirement 2 courses: [{"type":"credits","designators":["CLA"],"levels":[200,300,400]},{"type":"credits","courses":["FAH206H1","FAH207H1","PHL200Y1","PHL301H1"]},{"type":"credits","designators":["GRK","LAT"],"levels":[200,300,400]}]`,
			},
			nil},
		{"a group's share and the remainder's courses, starred ones a maybe item (ASMIN1254, shortened)",
			"(4.0 credits)\nHigher Years:\n1. ENV200H1\n2. Additional 2.5 credits, of which a maximum 1.0 credit may be environmental courses offered by academic units other than the School of the Environment (from Group A below). The remainder must be selected from the following: ENV261H1/ ENV360H1, ENV281H1, ENV411H1*, ENV421Y1/ ENV421H1*, PHL273H1.\n*with permission of Undergraduate Associate Director\n**Group A**\nANT364H1, EEB240H1/ ENV234H1, WGS442H1, other courses approved by School of the Environment.",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":4},{"type":"text","kind":"permission","text":"(4.0 credits)"}]}`,
				`2 allocation p1 {"type":"course","code":"ENV200H1"}`,
				`3 allocation p1 {"type":"or","items":[{"type":"credits","amount":2.5,"lists":["Requirement Higher Years 2 courses"]},{"type":"text","kind":"permission","text":"other courses approved by School of the Environment"}]}`,
				`4 limit p3 {"type":"credits","lists":["Group A"],"max_amount":1}`,
				`list: Group A: [{"type":"credits","courses":["ANT364H1","EEB240H1","ENV234H1","WGS442H1"]}]`,
				`list: Requirement Higher Years 2 courses: [{"type":"credits","courses":["ENV261H1","ENV360H1","ENV281H1","PHL273H1"]},{"type":"and","items":[{"type":"credits","courses":["ENV411H1","ENV421Y1","ENV421H1"]},{"type":"text","kind":"permission","text":"with permission of Undergraduate Associate Director"}]},{"type":"credits","lists":["Group A"]}]`,
			},
			nil},
		{"the program's courses, the rows named and capped courses (ASMIN0728, shortened)",
			"(4.0 credits)\n1. 0.5 credit from CDN265H1, CDN267H1, CDN268H1\n2. 0.5 credit from CDN307H1, CDN335H1\n3. 3.0 credits from courses offered by the Canadian Studies Program, including additional courses from Requirements 1 and 2. Up to 1.0 credit of this requirement can also be completed from HST211H1/ PHS211H1, INS200H1, SOC218H1.\nAdditional courses with significant Canadian content may be considered for approval for Requirement 3 by the Program Director (canadian.studies@utoronto.ca)",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":4},{"type":"text","kind":"permission","text":"(4.0 credits)"}]}`,
				`2 allocation p1 {"type":"credits","amount":0.5,"courses":["CDN265H1","CDN267H1","CDN268H1"]}`,
				`3 allocation p1 {"type":"credits","amount":0.5,"courses":["CDN307H1","CDN335H1"]}`,
				`4 allocation p1 {"type":"or","items":[{"type":"credits","amount":3,"lists":["Requirement 3 courses"]},{"type":"text","kind":"permission","text":"Additional courses with significant Canadian content may be considered for approval for Requirement 3 by the Program Director (canadian.studies@utoronto.ca)"}]}`,
				`5 limit p4 {"type":"credits","courses":["HST211H1","PHS211H1","INS200H1","SOC218H1"],"max_amount":1}`,
				`list: Requirement 3 courses: [{"type":"and","items":[{"type":"credits"},{"type":"text","kind":"content","text":"offered by the Canadian Studies Program"}]},{"type":"credits","requirements":[2,3]},{"type":"credits","courses":["HST211H1","PHS211H1","INS200H1","SOC218H1"]}]`,
			},
			nil},
		{"an item of cognates caps the total's pool (ASMIN2750, shortened)",
			"4.0 credits including at least 1.0 credit at the 300+ level. Up to 1.0 credit may be chosen from approved courses offered by other departments (see list of Cognate Courses).\n1. CRE201H1\n2. At least 1.0 credit from Group A – Forms of Creativity: VIC191Y1, CRE210H1, CRE269H1.\n3. At least 1.0 credit from Group B – Social Contexts and Interpretation: VIC164H1, CRE209H1, MCS223H1.\n4. Up to 1.0 credit from Cognate Courses:\n   SMC219Y1, SMC229H1, WRR414H1, or another creativity-themed course approved by the program coordinator.",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":4,"lists":["Requirement 4 courses"]},{"type":"text","kind":"permission","text":"another creativity-themed course approved by the program coordinator"}]}`,
				`2 constraint p1 {"type":"or","items":[{"type":"credits","amount":1,"levels":[300,400]},{"type":"text","kind":"permission","text":"including at least 1.0 credit at the 300+ level"}]}`,
				`3 allocation p1 {"type":"course","code":"CRE201H1"}`,
				`4 allocation p1 {"type":"credits","amount":1,"courses":["VIC191Y1","CRE210H1","CRE269H1"]}`,
				`5 allocation p1 {"type":"credits","amount":1,"courses":["VIC164H1","CRE209H1","MCS223H1"]}`,
				`6 limit p1 {"type":"credits","courses":["SMC219Y1","SMC229H1","WRR414H1"],"max_amount":1}`,
				`list: Requirement 4 courses: [{"type":"credits","courses":["CRE201H1","VIC191Y1","CRE210H1","CRE269H1","VIC164H1","CRE209H1","MCS223H1"]},{"type":"credits","courses":["SMC219Y1","SMC229H1","WRR414H1"]}]`,
			},
			nil},
		{"courses offered by other departments capped, one credit of an amount at a level (ASMIN1058, shortened)",
			"Completion of the program requires 4.0 credits, meeting the following requirements:\n1. EAS103H1 and EAS105H1;\n2. 3.0 Society-Culture credits, one of which must be 300/400-level;\n3. 1.0 credit may be courses on East Asia offered by other departments.\n_Society-Culture Courses_\nEAS193H1, EAS194H1, EAS307H1\n_Society-Culture Courses on East Asia Offered by Other Departments_\nANT341H1, ANT472H1",
			[]string{
				`1 allocation {"type":"credits","amount":4}`,
				`2 allocation p1 {"type":"and","items":[{"type":"course","code":"EAS103H1"},{"type":"course","code":"EAS105H1"}]}`,
				`3 allocation p1 {"type":"or","items":[{"type":"credits","amount":3,"lists":["Society-Culture Courses"]},{"type":"and","items":[{"type":"credits","amount":3,"lists":["Society-Culture Courses","Society-Culture Courses on East Asia Offered by Other Departments"]},{"type":"text","kind":"ambiguity","text":"\"Society-Culture Courses on East Asia Offered by Other Departments\" may count as Society-Culture credits"}]}]}`,
				`4 constraint p3 {"type":"or","items":[{"type":"credits","amount":1,"levels":[300,400]},{"type":"and","items":[{"type":"credits","amount":0.5,"levels":[300,400]},{"type":"text","kind":"ambiguity","text":"\"one of which\" may mean one course (0.5 credit)"}]}]}`,
				`5 limit p1 {"type":"credits","lists":["Society-Culture Courses on East Asia Offered by Other Departments"],"max_amount":1}`,
				`list: Society-Culture Courses: [{"type":"credits","courses":["EAS193H1","EAS194H1","EAS307H1"]}]`,
				`list: Society-Culture Courses on East Asia Offered by Other Departments: [{"type":"credits","courses":["ANT341H1","ANT472H1"]}]`,
			},
			nil},
	})
}
