package completion

import (
	"testing"
)

// TestCompletionRuleGroups checks groups of alternatives, remainder pools and
// one-off pools over lists.
func TestCompletionRuleGroups(t *testing.T) {
	runComplRuleCases(t, []ruleCase{
		{"groups of alternatives with nested brackets: the and of the groups, the amount checked (ASMAJ1165, shortened)",
			"(7.5 credits)\n1. 2.5 credits from ( MAT130H1/ MAT135H1, MAT136H1, MAT246H1)/ [( MAT148H1, MAT149H1)/ MAT137Y1, MAT246H1]/ [( MAT158H1, MAT159H1)/ MAT157Y1, 0.5 credit from APM/MAT 200+ courses], ( MAT235H1, MAT236H1)/ MAT235Y1/ MAT237Y1/ MAT257Y1",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":7.5},{"type":"text","kind":"other","text":"(7.5 credits)"}]}`,
				`2 allocation p1 {"type":"and","items":[{"type":"or","items":[{"type":"and","items":[{"type":"or","items":[{"type":"course","code":"MAT130H1"},{"type":"course","code":"MAT135H1"}]},{"type":"course","code":"MAT136H1"},{"type":"course","code":"MAT246H1"}]},{"type":"and","items":[{"type":"or","items":[{"type":"and","items":[{"type":"course","code":"MAT148H1"},{"type":"course","code":"MAT149H1"}]},{"type":"course","code":"MAT137Y1"}]},{"type":"course","code":"MAT246H1"}]},{"type":"and","items":[{"type":"or","items":[{"type":"and","items":[{"type":"course","code":"MAT158H1"},{"type":"course","code":"MAT159H1"}]},{"type":"course","code":"MAT157Y1"}]},{"type":"credits","amount":0.5,"designators":["APM","MAT"],"levels":[200,300,400]}]}]},{"type":"or","items":[{"type":"and","items":[{"type":"course","code":"MAT235H1"},{"type":"course","code":"MAT236H1"}]},{"type":"course","code":"MAT235Y1"},{"type":"course","code":"MAT237Y1"},{"type":"course","code":"MAT257Y1"}]}]}`,
			},
			nil},
		{"any combination of groups: the groups counted whole, any of their courses the lenient reading (ASSPE2082, shortened)",
			"(14.0 credits)\nFirst Year: BIO120H1, BIO130H1, ( CHM135H1, CHM136H1)/ CHM151Y1, and 1.0 credit from any combination of ( MAT130H1/ MAT135H1, MAT136H1)/ ( MAT148H1, MAT149H1/ MAT137Y1)/ ( MAT158H1, MAT159H1/ MAT157Y1), PHY131H1/ PHY151H1, PHY132H1/ PHY152H1 (see NOTE 1)",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":14},{"type":"text","kind":"other","text":"(14.0 credits)"}]}`,
				`2 allocation p1 {"type":"and","items":[{"type":"course","code":"BIO120H1"},{"type":"course","code":"BIO130H1"},{"type":"or","items":[{"type":"and","items":[{"type":"course","code":"CHM135H1"},{"type":"course","code":"CHM136H1"}]},{"type":"course","code":"CHM151Y1"}]},{"type":"or","items":[{"type":"and","items":[{"type":"or","items":[{"type":"course","code":"MAT130H1"},{"type":"course","code":"MAT135H1"}]},{"type":"course","code":"MAT136H1"}]},{"type":"and","items":[{"type":"course","code":"MAT148H1"},{"type":"course","code":"MAT149H1"}]},{"type":"course","code":"MAT137Y1"},{"type":"and","items":[{"type":"course","code":"MAT158H1"},{"type":"course","code":"MAT159H1"}]},{"type":"course","code":"MAT157Y1"},{"type":"and","items":[{"type":"or","items":[{"type":"course","code":"PHY131H1"},{"type":"course","code":"PHY151H1"}]},{"type":"or","items":[{"type":"course","code":"PHY132H1"},{"type":"course","code":"PHY152H1"}]}]},{"type":"and","items":[{"type":"credits","amount":1,"courses":["MAT130H1","MAT135H1","MAT136H1","MAT148H1","MAT149H1","MAT137Y1","MAT158H1","MAT159H1","MAT157Y1","PHY131H1","PHY151H1","PHY132H1","PHY152H1"]},{"type":"text","kind":"ambiguity","text":"1.0 credit from any combination of ( MAT130H1/ MAT135H1, MAT136H1)/ ( MAT148H1, MAT149H1/ MAT137Y1)/ ( MAT158H1, MAT159H1/ MAT157Y1), PHY131H1/ PHY151H1, PHY132H1/ PHY152H1"}]}]}]}`,
				"  note: see NOTE 1",
			},
			nil},
		{"groups split by \";\", one bracket left open (ASMAJ0509)",
			"(8.0 credits)\n2.0 credits chosen from BIO120H1; ( CHM135H1, CHM136H1); MAT130H1/ MAT135H1, MAT136H1, ( MAT148H1, MAT149H1/ MAT137Y1; PHY131H1, PHY132H1",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":8},{"type":"text","kind":"other","text":"(8.0 credits)"}]}`,
				`2 allocation p1 {"type":"or","items":[{"type":"and","items":[{"type":"course","code":"CHM135H1"},{"type":"course","code":"CHM136H1"},{"type":"or","items":[{"type":"and","items":[{"type":"or","items":[{"type":"course","code":"MAT130H1"},{"type":"course","code":"MAT135H1"}]},{"type":"course","code":"MAT136H1"}]},{"type":"and","items":[{"type":"course","code":"MAT148H1"},{"type":"course","code":"MAT149H1"}]},{"type":"course","code":"MAT137Y1"}]}]},{"type":"and","items":[{"type":"course","code":"CHM135H1"},{"type":"course","code":"CHM136H1"},{"type":"course","code":"PHY131H1"},{"type":"course","code":"PHY132H1"}]},{"type":"and","items":[{"type":"or","items":[{"type":"and","items":[{"type":"or","items":[{"type":"course","code":"MAT130H1"},{"type":"course","code":"MAT135H1"}]},{"type":"course","code":"MAT136H1"}]},{"type":"and","items":[{"type":"course","code":"MAT148H1"},{"type":"course","code":"MAT149H1"}]},{"type":"course","code":"MAT137Y1"}]},{"type":"course","code":"PHY131H1"},{"type":"course","code":"PHY132H1"}]},{"type":"and","items":[{"type":"credits","amount":2,"courses":["BIO120H1","CHM135H1","CHM136H1","MAT130H1","MAT135H1","MAT136H1","MAT148H1","MAT149H1","MAT137Y1","PHY131H1","PHY132H1"]},{"type":"text","kind":"ambiguity","text":"2.0 credits chosen from BIO120H1; ( CHM135H1, CHM136H1); MAT130H1/ MAT135H1, MAT136H1, ( MAT148H1, MAT149H1/ MAT137Y1; PHY131H1, PHY132H1"}]}]}`,
			},
			nil},
		{"the courses that bring the program to its total: Req 1's own pool (ASSPE1762, shortened)",
			"(2.0 credits)\nFourth Year:\n1. BCH478H1\n2. To bring the program total to 2.0 credits, students may select from the following list: BIO220H1/ CHM217H1/ one of CSC108H1 or CSC148H1/ ( MAT235H1, MAT236H1)/ MAT235Y1/ any suitable 300-level lecture-based course from CHM/ CSB/ EEB (departmental approval required prior to enrolling)",
			[]string{
				`1 allocation {"type":"credits","amount":2,"lists":["Requirement Fourth Year 2 courses"]}`,
				`2 allocation p1 {"type":"course","code":"BCH478H1"}`,
				`3 limit p1 {"type":"credits","courses":["CSC108H1","CSC148H1"],"max_amount":0.5}`,
				`list: Requirement Fourth Year 2 courses: [{"type":"credits","courses":["BIO220H1","CHM217H1","CSC108H1","CSC148H1","MAT235Y1"]},{"type":"and","items":[{"type":"course","code":"MAT235H1"},{"type":"course","code":"MAT236H1"}]},{"type":"and","items":[{"type":"credits","designators":["CHM","CSB","EEB"],"levels":[300]},{"type":"text","kind":"permission","text":"any suitable 300-level lecture-based course from CHM/ CSB/ EEB (departmental approval required prior to enrolling)"}]}]`,
			},
			nil},
		{"an exemption from any of four courses replaced by a requirement's pool or a filter, for the credits exempted (ASMIN1135, shortened)",
			"(4.0 credits)\n1) ( FSL220H1, FSL222H1)/ FSL221Y1\n2) FSL224H1/ FSL225H1\n3) 0.5 credit selected from CDN202H1, JFL484H1, or any FRE or FCS half-course at the 200+ level\nN.B. Students exempted from any of FSL220H1/ FSL222H1/ FSL224H1/ FSL225H1 must replace these credits with any combination of the following two options:\n(a) any additional credit(s) from category (3) above, or\n(b) any FSL credit(s) at the 300/400 level.",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":4},{"type":"text","kind":"other","text":"(4.0 credits)"}]}`,
				`2 allocation p1 {"type":"or","items":[{"type":"and","items":[{"type":"course","code":"FSL220H1"},{"type":"course","code":"FSL222H1"}]},{"type":"and","items":[{"type":"credits","amount":1,"lists":["Requirement 1 courses"]},{"type":"text","kind":"placement","text":"NB: Students exempted from any of FSL220H1/ FSL222H1/ FSL224H1/ FSL225H1 must replace these credits with any combination of the following two options: any additional credit(s) from category (3) above, or any FSL credit(s) at the 300/400 level"}]},{"type":"course","code":"FSL221Y1"}]}`,
				`3 allocation p1 {"type":"or","items":[{"type":"course","code":"FSL224H1"},{"type":"course","code":"FSL225H1"},{"type":"and","items":[{"type":"credits","amount":0.5,"lists":["Requirement 1 courses"]},{"type":"text","kind":"placement","text":"NB: Students exempted from any of FSL220H1/ FSL222H1/ FSL224H1/ FSL225H1 must replace these credits with any combination of the following two options: any additional credit(s) from category (3) above, or any FSL credit(s) at the 300/400 level"}]}]}`,
				`4 allocation p1 {"type":"credits","amount":0.5,"lists":["Requirement 3 courses"]}`,
				`list: Requirement 3 courses: [{"type":"credits","courses":["CDN202H1","JFL484H1"]},{"type":"credits","designators":["FRE","FCS"],"levels":[200,300,400],"max_course_amount":0.5}]`,
				`list: Requirement 1 courses: [{"type":"credits","lists":["Requirement 3 courses"]},{"type":"credits","designators":["FSL"],"levels":[300,400]}]`,
			},
			nil},
		{"further courses of a designator at a level, or listed courses at it, or a course its content decides (ASMAJ2660, shortened)",
			"(8.0 credits)\nIntroductory Courses\n- 0.5 credit from further MUN coded100 level courses or POL101H1/ POL106H1/ TRN160Y1/ VIC122H1/ NEW106H1 or another First Year Foundation/One course relevant to the study of Public Policy",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":8},{"type":"text","kind":"content","text":"(8.0 credits)"}]}`,
				`2 allocation p1 {"type":"or","items":[{"type":"credits","amount":0.5,"designators":["MUN"],"levels":[100],"courses":["POL101H1","POL106H1","TRN160Y1","VIC122H1","NEW106H1"]},{"type":"text","kind":"content","text":"another First Year Foundation/One course relevant to the study of Public Policy"}]}`,
			},
			nil},
		{"two lists with a level excluded from one or both, and lists or a titled list (ASMIN0235, shortened)",
			"(4.0 credits)\n3. 0.5 credit from Group A and 0.5 credit from Group B (excluding 100-level courses)\n4. 1.0 credit from Group A or B (excluding 100-level courses), or at any level in an Asian language (see _Accepted Language Courses_ below)\n_Group A: Social, Political and Economic Topics (Social Science)_\nCAS200H1, CAS310H1, EAS105H1\n_Group B: Cultural Topics (Humanities)_\nCAS310H1, EAS231H1\n_Accepted Language Courses_\nEAS100Y1, EAS201H1",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":4},{"type":"text","kind":"other","text":"(4.0 credits)"}]}`,
				`2 allocation p1 {"type":"or","items":[{"type":"and","items":[{"type":"credits","amount":1,"levels":[200,300,400],"lists":["Group A","Group B"]},{"type":"credits","amount":0.5,"levels":[200,300,400],"lists":["Group A"]},{"type":"credits","amount":0.5,"levels":[200,300,400],"lists":["Group B"]}]},{"type":"and","items":[{"type":"credits","amount":0.5,"lists":["Group A"]},{"type":"credits","amount":0.5,"levels":[200,300,400],"lists":["Group B"]},{"type":"text","kind":"ambiguity","text":"0.5 credit from Group A and 0.5 credit from Group B (excluding 100-level courses)"}]}]}`,
				`3 allocation p1 {"type":"or","items":[{"type":"credits","amount":1,"levels":[200,300,400],"lists":["Group A","Group B"]},{"type":"credits","amount":1,"lists":["Accepted Language Courses"]},{"type":"and","items":[{"type":"credits","amount":1,"lists":["Requirement 4 courses"]},{"type":"text","kind":"ambiguity","text":"1.0 credit from Group A or B (excluding 100-level courses), or at any level in an Asian language (see Accepted Language Courses below)"}]}]}`,
				`list: Group A (Group A: Social, Political and Economic Topics (Social Science)): [{"type":"credits","courses":["CAS200H1","CAS310H1","EAS105H1"]}]`,
				`list: Group B (Group B: Cultural Topics (Humanities)): [{"type":"credits","courses":["CAS310H1","EAS231H1"]}]`,
				`list: Accepted Language Courses: [{"type":"credits","courses":["EAS100Y1","EAS201H1"]}]`,
				`list: Requirement 4 courses: [{"type":"credits","levels":[200,300,400],"lists":["Group A","Group B"]},{"type":"credits","lists":["Accepted Language Courses"]}]`,
			},
			nil},
		{"a designator or two lists, with a cap on one list; a sub-list title both lists use is two lists (ASMAJ0552, shortened)",
			"(7.0 credits)\n5. 2.5 LAS credits or pre-approved courses from Lists A and B, of which a maximum of 0.5 credit may be from List B.\n**List A (Thematic Group):**\nLatin American Societies: ANT320H1, ANT340H1\nGeography: GGR341H1\n**List B (Disciplinary Group):**\nEconomics: ECO324H1, ECO403H1\nGeography: JGE321H1",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":7},{"type":"text","kind":"other","text":"(7.0 credits)"}]}`,
				`2 allocation p1 {"type":"credits","amount":2.5,"lists":["Requirement 5 courses"]}`,
				`3 limit p2 {"type":"credits","lists":["List B"],"max_amount":0.5}`,
				`list: List A (List A (Thematic Group)): [{"type":"credits","courses":["ANT320H1","ANT340H1"]},{"type":"credits","lists":["Geography (List A)"]}]`,
				`list: Geography (List A) (Geography): [{"type":"credits","courses":["GGR341H1"]}]`,
				`list: List B (List B (Disciplinary Group)): [{"type":"credits","courses":["ECO324H1","ECO403H1"]},{"type":"credits","lists":["Geography (List B)"]}]`,
				`list: Geography (List B) (Geography): [{"type":"credits","courses":["JGE321H1"]}]`,
				`list: Requirement 5 courses: [{"type":"credits","designators":["LAS"]},{"type":"credits","lists":["List A","List B"]}]`,
			},
			nil},
	})
}
