package reqs

import (
	"slices"
	"strings"
	"testing"

	"degree-explorer/types"
)

// TestCompletionAuditFixesC checks the audit fixes of Task 10 stream C on
// snippets of real programs.
func TestCompletionAuditFixesC(t *testing.T) {
	runComplRuleCases(t, []complRuleCase{
		{"a footnoted stand-in that needn't be replaced: the total less the missing share (ASSPE1868, shortened)",
			"(1.5 credits)\nFirst or second year:\n( CSC111H1/ CSC165H1, CSC236H1)/ CSC240H1 (1)\nCSC263H1/ CSC265H1\nNotes:\n(1) The \"missing\" 0.5 credit of the substitution does not have to be replaced. For details, please refer to the Computer Science Program instructions in this Calendar.",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":1.5},{"type":"and","items":[{"type":"credits","amount":1},{"type":"course","code":"CSC240H1"},{"type":"not","items":[{"type":"course","code":"CSC236H1"}]}]}]}`,
				`2 allocation p1 {"type":"or","items":[{"type":"and","items":[{"type":"or","items":[{"type":"course","code":"CSC111H1"},{"type":"course","code":"CSC165H1"}]},{"type":"course","code":"CSC236H1"}]},{"type":"course","code":"CSC240H1"}]}`,
				`3 allocation p1 {"type":"or","items":[{"type":"course","code":"CSC263H1"},{"type":"course","code":"CSC265H1"}]}`,
				`note: For details, please refer to the Computer Science Program instructions in this Calendar`,
			},
			nil},
		{"an omission under a total range: the range's lower end absorbs it (ASSPE2270, shortened)",
			"(2.0 or 2.5 credits)\nFirst year:\n1. 2.0 to 2.5 credits from STA130H1, ( CSC108H1, CSC148H1)/ ( CSC110Y1, CSC111H1)/ ( CSC110Y1, CSC148H1), ( MAT130H1/ MAT135H1, MAT136H1)/ MAT137Y1.\nNotes:\n- Students with a strong background in an object-oriented language such as Python, Java or C++ may omit CSC108H1 and proceed directly with CSC148H1. [There is no need to replace the missing 0.5 credit for program completion; however, please base your course choice on what you are ready to take, not on “saving” a 0.5 credit].",
			[]string{
				`1 allocation {"type":"credits","amount":2}`,
				`2 allocation p1 {"type":"and","items":[{"type":"course","code":"STA130H1"},{"type":"or","items":[{"type":"and","items":[{"type":"or","items":[{"type":"course","code":"CSC108H1"},{"type":"not","items":[{"type":"course","code":"CSC108H1"}]}]},{"type":"course","code":"CSC148H1"}]},{"type":"and","items":[{"type":"course","code":"CSC110Y1"},{"type":"course","code":"CSC111H1"}]},{"type":"and","items":[{"type":"course","code":"CSC110Y1"},{"type":"course","code":"CSC148H1"}]}]},{"type":"or","items":[{"type":"and","items":[{"type":"or","items":[{"type":"course","code":"MAT130H1"},{"type":"course","code":"MAT135H1"}]},{"type":"course","code":"MAT136H1"}]},{"type":"course","code":"MAT137Y1"}]}]}`,
			},
			nil},
		{"N first year Science credits: 100-level credits whose subject is content (ASMAJ1102, shortened)",
			"(3.0 credits)\nFirst Year:\nBIO120H1; plus 2.5 first year Science credits ( JEG100H1, GGR101H1, CHM135H1, CHM136H1 recommended)",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":3},{"type":"text","kind":"content","text":"(3.0 credits)"}]}`,
				`2 allocation p1 {"type":"and","items":[{"type":"course","code":"BIO120H1"},{"type":"and","items":[{"type":"credits","amount":2.5,"levels":[100]},{"type":"text","kind":"content","text":"2.5 first year Science credits"}]}]}`,
				`  note: JEG100H1, GGR101H1, CHM135H1, CHM136H1 recommended`,
			},
			nil},
		{"a constraint an item already requires joins that item's raw (ASSPE2082, shortened)",
			"(1.5 credits)\nThird Year: BCH311H1/ PSL350H1, PCL302H1, at least one 0.5 credit from PCL367H1 or PCL368H1 (see NOTE 2)\n**NOTES**:\n2. At least one 0.5 credit from PCL367H1 or PCL368H1 is required for the program, however if desired the alternative course can be taken as a program elective.",
			[]string{
				`1 allocation {"type":"credits","amount":1.5}`,
				`2 allocation p1 {"type":"and","items":[{"type":"or","items":[{"type":"course","code":"BCH311H1"},{"type":"course","code":"PSL350H1"}]},{"type":"course","code":"PCL302H1"},{"type":"or","items":[{"type":"course","code":"PCL367H1"},{"type":"course","code":"PCL368H1"}]}]}`,
			},
			nil},
		{"a ban on language courses no pool can take restates the items: it joins the total's raw (ASMIN1058, shortened)",
			"Completion of the program requires 1.5 credits, meeting the following requirements:\n1. EAS103H1 and EAS105H1;\n2. 0.5 Society-Culture credits;\nLanguage courses cannot be used to fulfil the requirements of the Minor program.\n_Society-Culture Courses_\nEAS193H1, EAS194H1, EAS195H1",
			[]string{
				`1 allocation {"type":"credits","amount":1.5}`,
				`2 allocation p1 {"type":"and","items":[{"type":"course","code":"EAS103H1"},{"type":"course","code":"EAS105H1"}]}`,
				`3 allocation p1 {"type":"credits","amount":0.5,"lists":["Society-Culture Courses"]}`,
				`list: Society-Culture Courses: [{"type":"credits","courses":["EAS193H1","EAS194H1","EAS195H1"]}]`,
			},
			nil},
		{"\"taken through the Daniels Faculty\" beside VIS courses restates the designator: the row's raw keeps it (ASMIN0660, shortened)",
			"(3.0 credits in Visual Studies.)\n2. JAV200H1 and 2.5 credits in VIS courses taken through the Daniels Faculty.",
			[]string{
				`1 allocation {"type":"credits","amount":3}`,
				`2 allocation p1 {"type":"and","items":[{"type":"course","code":"JAV200H1"},{"type":"credits","amount":2.5,"designators":["VIS"]}]}`,
			},
			nil},
		{"examples that don't close a pool are its strict alternatives, the pool content (ASMAJ1228, shortened)",
			"(1.0 credit)\n1. 1.0 credit from gateway 100-level courses. Possibilities include but are not limited to: ( MUN100H1 and MUN110H1) or ECO105Y1 or ( SOC100H1 and SOC150H1).",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":1},{"type":"text","kind":"content","text":"(1.0 credit)"}]}`,
				`2 allocation p1 {"type":"or","items":[{"type":"and","items":[{"type":"course","code":"MUN100H1"},{"type":"course","code":"MUN110H1"}]},{"type":"course","code":"ECO105Y1"},{"type":"and","items":[{"type":"course","code":"SOC100H1"},{"type":"course","code":"SOC150H1"}]},{"type":"text","kind":"content","text":"1.0 credit from gateway 100-level courses"}]}`,
			},
			nil},
		{"approved sequences that don't close a language requirement are its strict alternatives (ASSPE0506, shortened)",
			"(2.0 credits)\nLanguage Requirement (2.0 credits):\nTwo years of study or its equivalent of one non-Germanic, non-Romance language in courses using the spoken language. We encourage you to begin this language study in your first or second year. Approved sequences of courses include CLT141Y1, CLT251H1, and CLT252H1; EAS100Y1 and EAS200Y1. Other courses might also qualify, but must be pre-approved by the Department of Linguistics.",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":2},{"type":"text","kind":"other","text":"(2.0 credits)"}]}`,
				`2 allocation p1 {"type":"or","items":[{"type":"and","items":[{"type":"course","code":"CLT141Y1"},{"type":"course","code":"CLT251H1"},{"type":"course","code":"CLT252H1"}]},{"type":"and","items":[{"type":"course","code":"EAS100Y1"},{"type":"course","code":"EAS200Y1"}]},{"type":"text","kind":"content","text":"Two years of study or its equivalent of one non-Germanic, non-Romance language in courses using the spoken language"},{"type":"text","kind":"permission","text":"Other courses might also qualify, but must be pre-approved by the Department of Linguistics"}]}`,
				`  note: We encourage you to begin this language study in your first or second year`,
			},
			nil},
		{"an approval after \"and/or\" may qualify every course: strict over all, lenient over the last group (ASSPE1377, shortened)",
			"(1.0 credit)\n4. CHM395Y1/ CHM499Y1/ PHC489Y1 and/or CHM397H0/ CHM398H0 in areas relevant to the program with prior approval by the Department",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":1},{"type":"text","kind":"other","text":"(1.0 credit)"}]}`,
				`2 allocation p1 {"type":"or","items":[{"type":"and","items":[{"type":"or","items":[{"type":"course","code":"CHM395Y1"},{"type":"course","code":"CHM499Y1"},{"type":"course","code":"PHC489Y1"},{"type":"course","code":"CHM397H0"},{"type":"course","code":"CHM398H0"}]},{"type":"text","kind":"permission","text":"in areas relevant to the program with prior approval by the Department"}]},{"type":"and","items":[{"type":"or","items":[{"type":"course","code":"CHM395Y1"},{"type":"course","code":"CHM499Y1"},{"type":"course","code":"PHC489Y1"}]},{"type":"text","kind":"ambiguity","text":"the approval may be needed for the courses before \"and/or\" too"}]}]}`,
			},
			nil},
		{"a credit from the list of courses in two items is their rows' pool (ASMAJ1295, shortened)",
			"(2.5 credits)\n3) 1.5 credits from the following three groups (students are strongly encouraged to include 0.5 credit from each group):\nGroup 1 – Literary History: FRE318H1/ FRE319H1\nGroup 2 – Genres and Gender: FRE304H1/ FRE310H1\nGroup 3 – Quebec and Francophone Literatures: FRE314H1/ FRE332H1\n4) 0.5 credit from the following 400-level FRE literature courses: FRE410H1/ FRE438H1\n5) 0.5 additional credit from the list of courses in (3) or (4).\nN.B.\ni) Students may also satisfy requirements (4) and (5) through completion of a senior essay ( FRE490Y1).",
			[]string{
				`1 allocation {"type":"credits","amount":2.5}`,
				`2 allocation p1 {"type":"credits","amount":1.5,"courses":["FRE318H1","FRE319H1","FRE304H1","FRE310H1","FRE314H1","FRE332H1"]}`,
				`  note: students are strongly encouraged to include 0.5 credit from each group`,
				`3 allocation p1 {"type":"or","items":[{"type":"credits","amount":0.5,"courses":["FRE410H1","FRE438H1"]},{"type":"credits","amount":0.5,"courses":["FRE490Y1"],"weight":0.5}]}`,
				`4 allocation p1 {"type":"or","items":[{"type":"credits","amount":0.5,"requirements":[2,3]},{"type":"credits","amount":0.5,"courses":["FRE490Y1"],"weight":0.5}]}`,
			},
			nil},
		{"\"only have to complete DTS300H1\": the course that does both items, an overlap (ASMAJ1407, shortened)",
			"(2.0 credits)\n1. DTS200H1/ DTS200Y1\n2. 1.0 DTS credit at the 300-level, including DTS300H1\nNotes:\n- Students who completed DTS200Y1 for Requirement 1 only have to complete DTS300H1 for Requirement 2.",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":2},{"type":"text","kind":"other","text":"(2.0 credits)"}]}`,
				`2 allocation p1 {"type":"or","items":[{"type":"course","code":"DTS200H1"},{"type":"course","code":"DTS200Y1"}]}`,
				`3 allocation p1 {"type":"or","items":[{"type":"credits","amount":1,"designators":["DTS"],"levels":[300]},{"type":"and","items":[{"type":"course","code":"DTS200Y1"},{"type":"course","code":"DTS300H1"}]}]}`,
				`4 constraint p3 {"type":"course","code":"DTS300H1"}`,
			},
			nil},
		{"one discipline's credits: a pool per discipline a designator gives, content for the rest, the coordinator's route (ASMAJ1023, shortened)",
			"(1.0 credit)\n5. 1.0 credit in ONE other discipline offered within the Faculty of Arts & Science – African Studies, Anthropology, Classics, Music, Women and Gender Studies, and other departments as approved by the program coordinator.",
			[]string{
				`1 allocation {"type":"credits","amount":1}`,
				`2 allocation p1 {"type":"or","items":[{"type":"credits","amount":1,"designators":["AFR"]},{"type":"credits","amount":1,"designators":["ANT"]},{"type":"credits","amount":1,"designators":["WGS"]},{"type":"and","items":[{"type":"credits","amount":1},{"type":"text","kind":"content","text":"1.0 credit in ONE of Classics, Music"}]},{"type":"and","items":[{"type":"credits","amount":1},{"type":"text","kind":"permission","text":"other departments as approved by the program coordinator"}]}]}`,
			},
			nil},
		{"\"from the list below\" with no list below is content (ASMIN0695, shortened)",
			"(3.0 credits)\n1. 2.5 credits in MUS courses from the list below, including one credit at the 300+ level. Either MUS120Y1 or MUS220Y1 can be counted towards the 2.5 credits in MUS courses",
			[]string{
				`1 allocation {"type":"or","items":[{"type":"credits","amount":3},{"type":"text","kind":"content","text":"(3.0 credits)"}]}`,
				`2 allocation p1 {"type":"and","items":[{"type":"credits","amount":2.5,"designators":["MUS"]},{"type":"text","kind":"content","text":"2.5 credits in MUS courses from the list below, including one credit at the 300+ level"}]}`,
				`3 constraint p2 {"type":"or","items":[{"type":"credits","amount":1,"levels":[300,400]},{"type":"text","kind":"content","text":"2.5 credits in MUS courses from the list below, including one credit at the 300+ level. Either MUS120Y1 or MUS220Y1 can be counted towards the 2.5 credits in MUS courses"}]}`,
				`4 limit p2 {"type":"credits","courses":["MUS120Y1","MUS220Y1"],"max_amount":1}`,
			},
			nil},
		{"\"in one cultural area chosen from the list below\" with the areas as lines below: the or of the areas' pools (ASMIN1210, shortened)",
			"(2.0 credits)\n2. 2.0 credits in one cultural area chosen from the list below:\n_Polish_: SLA216H1, SLA226H1, SLA229H1, SLA236H1.\n_South Slavic_: SLA200H1, SLA217H1, SLA227H1, SLA235H1.",
			[]string{
				`1 allocation {"type":"credits","amount":2}`,
				`2 allocation p1 {"type":"or","items":[{"type":"credits","amount":2,"courses":["SLA216H1","SLA226H1","SLA229H1","SLA236H1"]},{"type":"credits","amount":2,"courses":["SLA200H1","SLA217H1","SLA227H1","SLA235H1"]}]}`,
			},
			nil},
	})
}

// TestCompletionAuditOverlapsC checks the overlaps the stand-in and
// "only have to complete" rules write: the course that does two items.
func TestCompletionAuditOverlapsC(t *testing.T) {
	for _, tt := range []struct {
		name, raw string
		row       int
		want      []int
	}{
		{"ASMAJ1407, shortened", "(2.0 credits)\n1. DTS200H1/ DTS200Y1\n2. 1.0 DTS credit at the 300-level, including DTS300H1\nNotes:\n- Students who completed DTS200Y1 for Requirement 1 only have to complete DTS300H1 for Requirement 2.", 3, []int{2}},
		{"ASMIN1689, shortened", "(1.5 credits)\n1. CSC165H1/ CSC240H1\n2. CSC236H1/ CSC240H1\nIf you take CSC240H1 without CSC165H1, there is no need to replace the missing half-credit for program completion.\nCSC240H1 is an accelerated and enriched version of CSC165H1 plus CSC236H1.", 2, []int{3}},
	} {
		c := ParseCompletion(tt.raw)
		if len(c.Requirements) < tt.row || !slices.Equal(c.Requirements[tt.row-1].Overlaps, tt.want) {
			t.Errorf("%s: row %d overlaps, want %v:\n%s", tt.name, tt.row, tt.want, strings.Join(complCompact(c), "\n"))
		}
	}
}

// TestMarkerCauses checks that a Review marker's causes are found on the
// final envelope: a permission route and an ambiguity make the total's
// marker other (markerKind), and both are its causes, all Review by design.
func TestMarkerCauses(t *testing.T) {
	c := ParseCompletion("(1.0 credit)\n4. CHM395Y1/ CHM499Y1/ PHC489Y1 and/or CHM397H0/ CHM398H0 in areas relevant to the program with prior approval by the Department")
	n := c.Requirements[0].Req
	if n.Type != types.NodeOr || len(n.Items) != 2 || n.Items[1].Kind != types.TextOther || !isMarker(n.Items[1], c.Requirements[0]) {
		t.Fatalf("row 1 = %s, want or(credits, marker of kind other)", strings.Join(complCompact(c), "\n"))
	}
	var kinds []types.TextKind
	for _, x := range markerCauses(c)[1] {
		kinds = append(kinds, x.Kind)
	}
	if !slices.Equal(kinds, []types.TextKind{types.TextPermission, types.TextAmbiguity}) {
		t.Errorf("row 1 marker causes = %v, want [permission ambiguity]", kinds)
	}
}

// markerCauses returns, for each row of c with a Review marker beside its
// count (or(count, marker), markIncomplete's mark), the open texts that
// leave that count incomplete, as markIncomplete found them: the row's own
// (a total's), its parent's (a constraint's), or the incomplete top-level
// allocations' (a top-level constraint's). A marker with none is a
// shortfall's. c is not changed. A constraint the marker replaced (its own
// and(floor, text)) is left out: its texts are gone, and its marker's kind
// is theirs.
func markerCauses(c *types.Completion) map[int][]*types.ReqNode {
	stripped := &types.Completion{}
	marked := map[int]bool{}
	for _, r := range c.Requirements {
		if r == nil {
			stripped.Requirements = append(stripped.Requirements, nil)
			continue
		}
		s := *r
		if n := r.Req; n != nil && n.Type == types.NodeOr && len(n.Items) == 2 && isMarker(n.Items[1], r) {
			s.Req = n.Items[0]
			marked[r.Number] = true
		}
		stripped.Requirements = append(stripped.Requirements, &s)
	}
	m := newMarkCauses(stripped)
	var top []*types.ReqNode
	for _, r := range m.kids[0] {
		if r.Kind == types.RequirementAllocation && m.incomplete(r) {
			top = append(top, m.causes(r)...)
		}
	}
	out := map[int][]*types.ReqNode{}
	for _, r := range stripped.Requirements {
		switch {
		case r == nil || !marked[r.Number]:
		case r.Kind != types.RequirementConstraint:
			out[r.Number] = m.causes(r)
		case r.Parent != 0:
			if p := r.Parent - 1; p < len(stripped.Requirements) && stripped.Requirements[p] != nil {
				out[r.Number] = m.causes(stripped.Requirements[p])
			}
		default:
			out[r.Number] = top
		}
	}
	return out
}
