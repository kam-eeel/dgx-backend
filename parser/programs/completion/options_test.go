package completion

import (
	"testing"
)

// TestCompletionRuleOptions checks options and choices: options that are each
// a body of courses are one allocation's or or choose; otherwise each option is
// an allocation under row 1 and a choice row at the top picks among them.
func TestCompletionRuleOptions(t *testing.T) {
	runComplRuleCases(t, []ruleCase{
		// Option E includes one of the others: or(A, ..., D, and(E, any of A-D)). The
		// total counts the cheapest choice (1.0); its items fall short of it (2.0 of
		// 3.0), so it carries the Review marker.
		{"options, one including the others (ASSPE2025)",
			"(3.0 credits)\nThird Year:\n- BCH370H1, BCH311H1\nChoose one of the options below:\n- Option A: Immunology: ( IMM341H1, IMM351H1)\n- Option B: Microbiology: ( MGY377H1, MGY378H1)\n- Option C: Anatomy: ANA300Y1\n- Option D: Metabolic and Signaling Pathways: PSL425H1 and PCL386H1\n- Option E: Research: LMP305Y1 and one of the above four options",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":3},{"type":"text","kind":"other","text":"(3.0 credits)"}]}`,
				`2 allocation p1 {"type":"and","items":[{"type":"course","code":"BCH370H1"},{"type":"course","code":"BCH311H1"}]}`,
				`3 allocation p1 {"type":"and","items":[{"type":"course","code":"IMM341H1"},{"type":"course","code":"IMM351H1"}]}`,
				`4 allocation p1 {"type":"and","items":[{"type":"course","code":"MGY377H1"},{"type":"course","code":"MGY378H1"}]}`,
				`5 allocation p1 {"type":"course","code":"ANA300Y1"}`,
				`6 allocation p1 {"type":"and","items":[{"type":"course","code":"PSL425H1"},{"type":"course","code":"PCL386H1"}]}`,
				`7 allocation p1 {"type":"course","code":"LMP305Y1"}`,
				`8 choice {"type":"or","items":[{"type":"requirement","requirements":[3]},{"type":"requirement","requirements":[4]},{"type":"requirement","requirements":[5]},{"type":"requirement","requirements":[6]},{"type":"and","items":[{"type":"requirement","requirements":[7]},{"type":"requirement","requirements":[3,4,5,6]}]}]}`,
			},
			nil},
		// The total counts the rows outside the options and the cheapest choice
		// (option E is LMP305Y1 and another option): row 1 has no shortfall marker.
		{"the total counts the cheapest choice (ASSPE2025)",
			"(14.0-15.5 credits, including at least 2.0 credits at the 400-level)\nFirst Year:\n- BIO120H1, BIO130H1\n- ( CHM135H1, CHM136H1)/ CHM151Y1\n- ( MAT130H1/ MAT135H1, MAT136H1)/ ( MAT148H1, MAT149H1/ MAT137Y1)/ (( MAT158H1, MAT159H1/ MAT157Y1)\nFirst or Second Year:\n- ( PHY131H1, PHY132H1)/ ( PHY151H1, PHY152H1)\nSecond Year:\n- BCH210H1\n- BIO230H1\n- BIO260H1/ HMB265H1\n- CHM220H1\n- CHM247H1/ CHM249H1\n- PSL300H1, PSL301H1\n- STA288H1/ STA220H1/ BIO220H1\n- LMP200H1\nThird Year:\n- BCH370H1, BCH311H1\n- LMP310H1, LMP320H1, LMP330H1\nChoose one of the options below:\n- Option A: Immunology: ( IMM341H1, IMM351H1)\n- Option B: Microbiology: ( MGY377H1, MGY378H1)\n- Option C: Anatomy: ANA300Y1\n- Option D: Metabolic and Signaling Pathways: PSL425H1 and PCL386H1\n- Option E: Research: LMP305Y1 and one of the above four options\nFourth Year:\nStudents should choose four of the following 400-level courses (2.0-2.5 credits):\n- LMP405Y1, LMP410H1, LMP415H1, LMP420H1, LMP430H1, LMP440H1, LMP450H1, LMP460H1\nNote: If the research course LMP405Y1 is taken, an additional three 400-level courses from the list are still required (totalling 2.5 credits).",
			[]string{
				`1 allocation {"type":"credits","amount":14}`,
				`2 constraint p1 {"type":"credits","amount":2,"levels":[400]}`,
				`3 allocation p1 {"type":"and","items":[{"type":"course","code":"BIO120H1"},{"type":"course","code":"BIO130H1"}]}`,
				`4 allocation p1 {"type":"or","items":[{"type":"and","items":[{"type":"course","code":"CHM135H1"},{"type":"course","code":"CHM136H1"}]},{"type":"course","code":"CHM151Y1"}]}`,
				`5 allocation p1 {"type":"or","items":[{"type":"and","items":[{"type":"or","items":[{"type":"course","code":"MAT130H1"},{"type":"course","code":"MAT135H1"}]},{"type":"course","code":"MAT136H1"}]},{"type":"and","items":[{"type":"course","code":"MAT148H1"},{"type":"course","code":"MAT149H1"}]},{"type":"course","code":"MAT137Y1"},{"type":"and","items":[{"type":"course","code":"MAT158H1"},{"type":"course","code":"MAT159H1"}]},{"type":"course","code":"MAT157Y1"}]}`,
				`6 allocation p1 {"type":"or","items":[{"type":"and","items":[{"type":"course","code":"PHY131H1"},{"type":"course","code":"PHY132H1"}]},{"type":"and","items":[{"type":"course","code":"PHY151H1"},{"type":"course","code":"PHY152H1"}]}]}`,
				`7 allocation p1 {"type":"course","code":"BCH210H1"}`,
				`8 allocation p1 {"type":"course","code":"BIO230H1"}`,
				`9 allocation p1 {"type":"or","items":[{"type":"course","code":"BIO260H1"},{"type":"course","code":"HMB265H1"}]}`,
				`10 allocation p1 {"type":"course","code":"CHM220H1"}`,
				`11 allocation p1 {"type":"or","items":[{"type":"course","code":"CHM247H1"},{"type":"course","code":"CHM249H1"}]}`,
				`12 allocation p1 {"type":"and","items":[{"type":"course","code":"PSL300H1"},{"type":"course","code":"PSL301H1"}]}`,
				`13 allocation p1 {"type":"or","items":[{"type":"course","code":"STA288H1"},{"type":"course","code":"STA220H1"},{"type":"course","code":"BIO220H1"}]}`,
				`14 allocation p1 {"type":"course","code":"LMP200H1"}`,
				`15 allocation p1 {"type":"and","items":[{"type":"course","code":"BCH370H1"},{"type":"course","code":"BCH311H1"}]}`,
				`16 allocation p1 {"type":"and","items":[{"type":"course","code":"LMP310H1"},{"type":"course","code":"LMP320H1"},{"type":"course","code":"LMP330H1"}]}`,
				`17 allocation p1 {"type":"and","items":[{"type":"course","code":"IMM341H1"},{"type":"course","code":"IMM351H1"}]}`,
				`18 allocation p1 {"type":"and","items":[{"type":"course","code":"MGY377H1"},{"type":"course","code":"MGY378H1"}]}`,
				`19 allocation p1 {"type":"course","code":"ANA300Y1"}`,
				`20 allocation p1 {"type":"and","items":[{"type":"course","code":"PSL425H1"},{"type":"course","code":"PCL386H1"}]}`,
				`21 allocation p1 {"type":"course","code":"LMP305Y1"}`,
				`22 choice {"type":"or","items":[{"type":"requirement","requirements":[17]},{"type":"requirement","requirements":[18]},{"type":"requirement","requirements":[19]},{"type":"requirement","requirements":[20]},{"type":"and","items":[{"type":"requirement","requirements":[21]},{"type":"requirement","requirements":[17,18,19,20]}]}]}`,
				`23 allocation p1 {"type":"choose","items":[{"type":"course","code":"LMP405Y1"},{"type":"course","code":"LMP410H1"},{"type":"course","code":"LMP415H1"},{"type":"course","code":"LMP420H1"},{"type":"course","code":"LMP430H1"},{"type":"course","code":"LMP440H1"},{"type":"course","code":"LMP450H1"},{"type":"course","code":"LMP460H1"}],"count":4}`,
			},
			nil},
		{"two of the following four options (ASMAJ1376)",
			"Second Year:\nAt least two of the following four options:\n- CHM217H1\n- CHM220H1/ CHM222H1\n- ( CHM236H1 and CHM237H1)/ CHM238Y1\n- CHM249H1/ CHM247H1",
			[]string{
				`1 allocation {"type":"choose","items":[{"type":"course","code":"CHM217H1"},{"type":"or","items":[{"type":"course","code":"CHM220H1"},{"type":"course","code":"CHM222H1"}]},{"type":"or","items":[{"type":"and","items":[{"type":"course","code":"CHM236H1"},{"type":"course","code":"CHM237H1"}]},{"type":"course","code":"CHM238Y1"}]},{"type":"or","items":[{"type":"course","code":"CHM249H1"},{"type":"course","code":"CHM247H1"}]}],"count":2}`,
			},
			nil},
		// The second and third options trade credits with item 2's pool: 2.0
		// credits from the groups and 0.5 of the pool beyond item 2's 1.0.
		{"EITHER ... OR over lines (ASMAJ1545)",
			"2. 1.0 credit from CAR215H1, CAR220H1, CAR221H1, CAR225H1, CAR226H1, HIS230H1, HIS231H1\n5. EITHER 2.5 credits from Group A or B OR\n2.0 credits if taking 1.5 credits from CAR215H1, CAR220H1, CAR221H1, CAR225H1, CAR226H1, HIS230H1, HIS231H1 OR\n1.5 credits if taking 2.0 credits from CAR215H1, CAR220H1, CAR221H1, CAR225H1, CAR226H1, HIS230H1, HIS231H1\n**Group A: Courses addressing central preoccupations in Caribbean Studies**\nCRI493H1, FAH251H1, HIS106Y1, HIS221H1, HIS222H1\n**Group B: Courses complementary to Caribbean Studies**\nAFR150Y1, AFR322H1, AFR351Y1, AMS200H1, AMS300H1",
			[]string{
				`1 allocation {"type":"credits","amount":1,"courses":["CAR215H1","CAR220H1","CAR221H1","CAR225H1","CAR226H1","HIS230H1","HIS231H1"]}`,
				`2 allocation {"type":"credits","amount":2.5,"lists":["Group A","Group B"]}`,
				"3 allocation null",
				`4 allocation p3 {"type":"credits","amount":2,"lists":["Group A","Group B"]}`,
				`5 allocation p3 {"type":"credits","amount":0.5,"courses":["CAR215H1","CAR220H1","CAR221H1","CAR225H1","CAR226H1","HIS230H1","HIS231H1"]}`,
				"6 allocation null",
				`7 allocation p6 {"type":"credits","amount":1.5,"lists":["Group A","Group B"]}`,
				`8 allocation p6 {"type":"credits","amount":1,"courses":["CAR215H1","CAR220H1","CAR221H1","CAR225H1","CAR226H1","HIS230H1","HIS231H1"]}`,
				`9 choice {"type":"or","items":[{"type":"requirement","requirements":[2]},{"type":"requirement","requirements":[3]},{"type":"requirement","requirements":[6]}]}`,
				`list: Group A (Group A: Courses addressing central preoccupations in Caribbean Studies): [{"type":"credits","courses":["CRI493H1","FAH251H1","HIS106Y1","HIS221H1","HIS222H1"]}]`,
				`list: Group B (Group B: Courses complementary to Caribbean Studies): [{"type":"credits","courses":["AFR150Y1","AFR322H1","AFR351Y1","AMS200H1","AMS300H1"]}]`,
			},
			nil},
		// The options are read with the lists, then are one allocation's or;
		// an option the lists can't read stays its text.
		{"X **OR** Y in an item (ASMAJ0235)",
			"6. 1.0 additional credit from Group A or B, **OR** 1.0 credit at any level of an Asian language (see _Accepted Language Courses_ below)\n7. CAS450H1 **OR** 0.5 credit in a quantitative or qualitative methods course from Group C to fulfill the Research Competency required in the program.\n_Group A: Social, Political and Economic Topics (Social Science)_\nAFR389H1, CAS200H1, CAS201H1, CAS202H1\n_Group B: Cultural Topics (Humanities)_\nBPM214H1, CAS310H1, CAS320H1, CAS350H1",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":1,"lists":["Group A","Group B"]},{"type":"text","kind":"other","text":"1.0 credit at any level of an Asian language (see Accepted Language Courses below)"}]}`,
				`2 allocation {"type":"or","items":[{"type":"course","code":"CAS450H1"},{"type":"text","kind":"other","text":"0.5 credit in a quantitative or qualitative methods course from Group C to fulfill the Research Competency required in the program"}]}`,
				`list: Group A (Group A: Social, Political and Economic Topics (Social Science)): [{"type":"credits","courses":["AFR389H1","CAS200H1","CAS201H1","CAS202H1"]}]`,
				`list: Group B (Group B: Cultural Topics (Humanities)): [{"type":"credits","courses":["BPM214H1","CAS310H1","CAS320H1","CAS350H1"]}]`,
			},
			nil},
		// The first reading is the total's pool; the second adds Group 4's
		// courses to it, at most 1.0, and all of 1.0 or none.
		{"or else (ASSPE1001)",
			"10.0 credits, including at least 4.0 credits at the 300+ level\n1. PHL281H1, PHL271H1, PHL275H1\n6. Additional philosophy courses, to a total of 10.0 credits, or else additional philosophy courses to a total of 9.0 credits and 1.0 credit from Group 4 (Interdisciplinary)\n_Group 4 - Interdisciplinary:_\nANT348H1, GER338H1, GGR340H1, HIS423H1",
			[]string{
				`1 allocation {"type":"credits","amount":10,"designators":["PHL"],"courses":["ANT348H1","GER338H1","GGR340H1","HIS423H1"]}`,
				`2 constraint p1 {"type":"credits","amount":4,"levels":[300,400]}`,
				`3 allocation p1 {"type":"and","items":[{"type":"course","code":"PHL281H1"},{"type":"course","code":"PHL271H1"},{"type":"course","code":"PHL275H1"}]}`,
				`4 limit p1 {"type":"credits","courses":["ANT348H1","GER338H1","GGR340H1","HIS423H1"],"max_amount":1}`,
				`5 constraint p1 {"type":"or","items":[{"type":"credits","courses":["ANT348H1","GER338H1","GGR340H1","HIS423H1"],"max_amount":0},{"type":"credits","amount":1,"courses":["ANT348H1","GER338H1","GGR340H1","HIS423H1"]}]}`,
			},
			nil},
		{"at least two of the three following categories (ASSPE1006)",
			"7. 1.0 to 1.5 credits in at least two of the three following categories: (1) one field course (0.5 credit) from EEB401H1, EEB403H1, EEB405H1, EEB406H1; (2) one seminar (0.5 credit) from EEB491H1, EEB495H1; and/or (3) one independent research course (0.5 credit) from EEB497H1/ (1.0 credit) from EEB498Y1",
			[]string{
				`1 allocation {"type":"choose","items":[{"type":"credits","amount":0.5,"courses":["EEB401H1","EEB403H1","EEB405H1","EEB406H1"]},{"type":"credits","amount":0.5,"courses":["EEB491H1","EEB495H1"]},{"type":"or","items":[{"type":"credits","amount":0.5,"courses":["EEB497H1"]},{"type":"credits","amount":1,"courses":["EEB498Y1"]}]}],"count":2}`,
			},
			nil},
		// The note restates the count: it joins the row's raw.
		{"four of the following courses, and a note restating it (ASSPE2025)",
			"Fourth Year:\nStudents should choose four of the following 400-level courses (2.0-2.5 credits):\n- LMP405Y1, LMP410H1, LMP415H1, LMP420H1, LMP430H1, LMP440H1, LMP450H1, LMP460H1\nNote: If the research course LMP405Y1 is taken, an additional three 400-level courses from the list are still required (totalling 2.5 credits).",
			[]string{
				`1 allocation {"type":"choose","items":[{"type":"course","code":"LMP405Y1"},{"type":"course","code":"LMP410H1"},{"type":"course","code":"LMP415H1"},{"type":"course","code":"LMP420H1"},{"type":"course","code":"LMP430H1"},{"type":"course","code":"LMP440H1"},{"type":"course","code":"LMP450H1"},{"type":"course","code":"LMP460H1"}],"count":4}`,
			},
			map[int]string{1: "Students should choose four of the following 400-level courses (2.0-2.5 credits):\n- LMP405Y1, LMP410H1, LMP415H1, LMP420H1, LMP430H1, LMP440H1, LMP450H1, LMP460H1\nIf the research course LMP405Y1 is taken, an additional three 400-level courses from the list are still required (totalling 2.5 credits)"}},
		// Item 4 restates the alternative the note puts on STA130H1: no row
		// of its own.
		{"an item restating an alternative (ASSPE1687)",
			"_First year (3.5 credits)_\nMAT223H1/ MAT240H1, STA130H1\n_Later years_\n4. If a student has not completed STA130H1 then an additional 0.5 credit 300+ level STA course that is not used towards any other program requirement must be completed.\nNote: If you do not complete STA130H1 in your first year of study, this requirement must be fulfilled by completing a 300 or 400-level 0.5 credit STA course to replace STA130H1.",
			[]string{
				`1 allocation {"type":"and","items":[{"type":"or","items":[{"type":"course","code":"MAT223H1"},{"type":"course","code":"MAT240H1"}]},{"type":"or","items":[{"type":"course","code":"STA130H1"},{"type":"credits","amount":0.5,"designators":["STA"],"levels":[300,400]}]}]}`,
			},
			map[int]string{1: "MAT223H1/ MAT240H1, STA130H1\nIf you do not complete STA130H1 in your first year of study, this requirement must be fulfilled by completing a 300 or 400-level 0.5 credit STA course to replace STA130H1\n4. If a student has not completed STA130H1 then an additional 0.5 credit 300+ level STA course that is not used towards any other program requirement must be completed."}},
		// The line under Group C restates item 7's alternative: it joins
		// item 7's raw.
		{"a line restating an item's alternative (ASSPE1707)",
			"7. 2.0 credits in language courses from Group C (_a progression in one language_) or 2.0 credits in any major African language approved by the Program Committee.\n**Group C (Core African Languages & other Language Options):**\nSwahili: AFR280Y1, AFR380Y1\nArabic: NML110Y1, NML210Y1\nOR 2.0 credits in any major African language approved by the Program Committee",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":2,"courses":["AFR280Y1","AFR380Y1"]},{"type":"credits","amount":2,"courses":["NML110Y1","NML210Y1"]},{"type":"text","kind":"permission","text":"2.0 credits in any major African language approved by the Program Committee"}]}`,
			},
			map[int]string{1: "7. 2.0 credits in language courses from Group C (_a progression in one language_) or 2.0 credits in any major African language approved by the Program Committee.\nOR 2.0 credits in any major African language approved by the Program Committee"}},
		// The second option's parts may share courses: a group. The approval
		// is an alternative on its parts.
		{"Either X or Y and one additional Z (ASSPE1252)",
			"_Fourth Year Courses_: Either GGR491Y1 or one of GGR416H1, GGR492H1, GGR497H1 and one additional 0.5 credit 400 level course from Group A or Group B (1.0 credit). Up to one 0.5 credit can be used from GGR493Y1, based on suitability of placement for this program, and approved by department.\n_Group A (Environmental Geography Courses):_\nGGR416H1, GGR418H1, GGR419H1\n_Group B (School of the Environment Courses):_\nENV421H1, ENV422H1",
			[]string{
				`1 allocation {"type":"course","code":"GGR491Y1"}`,
				`2 allocation null`,
				`3 allocation p2 {"type":"or","items":[{"type":"course","code":"GGR416H1"},{"type":"course","code":"GGR492H1"},{"type":"course","code":"GGR497H1"},{"type":"and","items":[{"type":"credits","amount":0.5,"courses":["GGR416H1","GGR492H1","GGR497H1","GGR493Y1"]},{"type":"text","kind":"permission","text":"Up to one 0.5 credit can be used from GGR493Y1, based on suitability of placement for this program, and approved by department"}]}]}`,
				`4 limit p3 {"type":"credits","courses":["GGR493Y1"],"max_amount":0.5}`,
				`5 allocation p2 {"type":"or","items":[{"type":"credits","amount":0.5,"levels":[400],"lists":["Group A","Group B"]},{"type":"and","items":[{"type":"credits","amount":0.5,"courses":["GGR416H1","GGR418H1","GGR419H1","ENV421H1","ENV422H1","GGR493Y1"]},{"type":"text","kind":"permission","text":"Up to one 0.5 credit can be used from GGR493Y1, based on suitability of placement for this program, and approved by department"}]}]}`,
				`6 limit p5 {"type":"credits","courses":["GGR493Y1"],"max_amount":0.5}`,
				`7 choice {"type":"or","items":[{"type":"requirement","requirements":[1]},{"type":"requirement","requirements":[2]}]}`,
				`list: Group A (Group A (Environmental Geography Courses)): [{"type":"credits","courses":["GGR416H1","GGR418H1","GGR419H1"]}]`,
				`list: Group B (Group B (School of the Environment Courses)): [{"type":"credits","courses":["ENV421H1","ENV422H1"]}]`,
			},
			nil},
	})
}
