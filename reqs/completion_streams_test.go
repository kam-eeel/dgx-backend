package reqs

import "testing"

// TestCompletionRuleTitles checks course titles and labels in lists, and pools
// written inside an item.
func TestCompletionRuleTitles(t *testing.T) {
	runComplRuleCases(t, []complRuleCase{
		{"bare course titles after the codes are dropped (ASMAJ0151)",
			"3. 0.5 credit from one of the following traditions: RLG201H1 Indigenous Spiritualities and Religions/ RLG202H1 Judaism/ RLG241H1 The Earliest Christians",
			[]string{
				`1 allocation {"type":"credits","amount":0.5,"courses":["RLG201H1","RLG202H1","RLG241H1"]}`,
			},
			nil},
		{"course titles in brackets are dropped (ASMAJ2745)",
			"2. 0.5 credit of field courses: ESS410H1 (Field Techniques in Hydrogeochemistry)/ ESS450H1 (Geophysical Field Techniques)/ GGR390H1 (Field Methods)",
			[]string{
				`1 allocation {"type":"credits","amount":0.5,"courses":["ESS410H1","ESS450H1","GGR390H1"]}`,
			},
			nil},
		{"category labels in a list are dropped, a restated corequisite is a note (ASMAJ1390, shortened)",
			"6. 0.5 credit at the 400-level from: field course EEB403H1, EEB405H1; seminar/lecture course EEB428H1, ENV432H1; EEB497H1, independent research project course EEB498Y1 (concurrent with research issues course EEB488H1), EEB499Y1",
			[]string{
				`1 allocation {"type":"credits","amount":0.5,"courses":["EEB403H1","EEB405H1","EEB428H1","ENV432H1","EEB497H1","EEB498Y1","EEB499Y1"]}`,
				`  note: EEB498Y1 concurrent with research issues course EEB488H1`,
			},
			nil},
		{"a topic of two subjects describes the pool (ASSPE2364)",
			"5. 0.5 credit at the 300+ level in **plant or microbial biology** from: CSB340H1, CSB350H1, EEB328H1",
			[]string{
				`1 allocation {"type":"credits","amount":0.5,"courses":["CSB340H1","CSB350H1","EEB328H1"]}`,
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
