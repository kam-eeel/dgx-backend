package completion

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"degree-explorer/parser/programs"
	"degree-explorer/types"
)

// TestRepairCompletionCodes repairs codes the calendar misprints only when
// the catalogue confirms them, on real lines.
func TestRepairCompletionCodes(t *testing.T) {
	known := map[string]bool{}
	for _, c := range []string{"CDN307H1", "CDN335H1", "HIS283Y1", "HIS324H1", "HPS324H1", "ITA340H1", "ITA341H1", "PSY399H1", "PSY369H1", "MAT159H1", "MAT158H1", "MAT157Y1"} {
		known[c] = true
	}
	tests := []struct{ in, want string }{
		// ASMIN0728: a code without its campus digit.
		{"2. 0.5 credit from CDN307H, CDN335H1", "2. 0.5 credit from CDN307H1, CDN335H1"},
		// ASMAJ0652: the same, before a weight.
		{"HIS282Y1 (.50), HIS283Y (.50), HIS291H1", "HIS282Y1 (.50), HIS283Y1 (.50), HIS291H1"},
		// ASSPE0652: HIS324H1 and HPS324H1 exist; only HIS is on the line.
		{"(Sub-group: Latin America/The Caribbean): HIS230H1, JHN323H1, HS324H1, HIS336H1", "(Sub-group: Latin America/The Caribbean): HIS230H1, JHN323H1, HIS324H1, HIS336H1"},
		// ASMAJ1626: codes without H/Y and campus digit.
		{"ITA310H1, ITA340, ITA341, ITA410H1", "ITA310H1, ITA340H1, ITA341H1, ITA410H1"},
		// ASMIN0482: PSY399H1 is in the catalogue; CSB332H and HMB220H aren't.
		{"CSB325H1, CSB332H/ CJH332H1, HMB200H1/HMB220H, PSY369H1/PSY399H, PSY490H1", "CSB325H1, CSB332H/ CJH332H1, HMB200H1/HMB220H, PSY369H1/PSY399H1, PSY490H1"},
		// ASMAJ1762.
		{"( MAT158H1, MAT159H/ MAT157Y1)", "( MAT158H1, MAT159H1/ MAT157Y1)"},
		// Kept: a level ("APM400-level"), a series, a room, a graduate code.
		{"any APM400-level course/ PSL300-series/ Room UC173/ GLA2050H1", "any APM400-level course/ PSL300-series/ Room UC173/ GLA2050H1"},
	}
	for _, tt := range tests {
		if got, _ := repairCompletionCodes(tt.in, known); got != tt.want {
			t.Errorf("repairCompletionCodes(%q)\n got %q\nwant %q", tt.in, got, tt.want)
		}
	}
}

// TestCheckRequirementRefs: a requirements filter naming a row the program
// doesn't have becomes the row's text, with a note; one that resolves is kept
// (ASMAJ0728, shortened, references set by hand).
func TestCheckRequirementRefs(t *testing.T) {
	c := Parse("(1.5 credits)\n1. CDN267H1, CDN268H1\n2. 0.5 credit from CDN307H1, CDN335H1, CDN367H1")
	c.Requirements[1].Req.Items[0].Requirements = []int{3}
	c.Requirements[2].Req = &types.ReqNode{Type: types.NodeCredits, Amount: 0.5, Requirements: []int{2, 9}}
	if !checkRequirementRefs(c) {
		t.Fatal("no change reported")
	}
	got := compact(c)
	want := []string{
		`1 allocation {"type":"credits","amount":1.5}`,
		`2 allocation p1 {"type":"and","items":[{"type":"course","code":"CDN267H1","requirements":[3]},{"type":"course","code":"CDN268H1"}]}`,
		`3 allocation p1 {"type":"text","kind":"other","text":"2. 0.5 credit from CDN307H1, CDN335H1, CDN367H1"}`,
		`  note: No requirement 9 in this program.`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("\n got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestCheckListRefs: a row naming a list the program doesn't have becomes its
// text with a note; a list item naming one loses the name and becomes a maybe,
// noted on the program; a known name stays.
func TestCheckListRefs(t *testing.T) {
	c := Parse("(1.0 credit)\n1. 0.5 credit from Group A\n2. 0.5 credit from Group D")
	c.Lists = []*types.CourseList{
		{Name: "Group A", Items: []*types.ReqNode{{Type: types.NodeCredits, Courses: []string{"CDN267H1"}}, {Type: types.NodeCredits, Lists: []string{"Group C"}}}},
	}
	c.Lists = append(c.Lists, &types.CourseList{Name: "Requirement 2 courses", Items: []*types.ReqNode{
		{Type: types.NodeCredits, Requirements: []int{2, 9}}, {Type: types.NodeCredits, Requirements: []int{noRow}}}})
	c.Requirements[1].Req = &types.ReqNode{Type: types.NodeCredits, Amount: 0.5, Lists: []string{"Group A"}}
	c.Requirements[2].Req = &types.ReqNode{Type: types.NodeCredits, Amount: 0.5, Lists: []string{"Group D"}}
	if !checkRequirementRefs(c) {
		t.Fatal("no change reported")
	}
	got := compact(c)
	want := []string{
		`1 allocation {"type":"or","items":[{"type":"credits","amount":1},{"type":"text","kind":"other","text":"(1.0 credit)"}]}`,
		`2 allocation p1 {"type":"credits","amount":0.5,"lists":["Group A"]}`,
		`3 allocation p1 {"type":"text","kind":"other","text":"2. 0.5 credit from Group D"}`,
		`  note: No list "Group D" in this program.`,
		`list: Group A: [{"type":"credits","courses":["CDN267H1"]},{"type":"and","items":[{"type":"credits"},{"type":"text","kind":"other","text":"a course on \"Group C\""}]}]`,
		`list: Requirement 2 courses: [{"type":"and","items":[{"type":"credits","requirements":[2]},{"type":"text","kind":"other","text":"a course on requirement 9"}]},{"type":"and","items":[{"type":"credits"},{"type":"text","kind":"other","text":"a course on a requirement of the program it was copied from"}]}]`,
		`note: No list "Group C" in this program.`,
		`note: No requirement 9 in this program.`,
		`note: A list refers to a requirement of the program it was copied from that this program doesn't have.`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("\n got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestLinkListCodes checks list members the catalogue lacks: ASMIN1252's
// "GGE372H1" is GGR372H1, which its area's Major lists; ASMAJ0571's "HST211H1"
// has no such confirmation and stays, noted.
func TestLinkListCodes(t *testing.T) {
	titles := map[string]string{}
	for _, c := range []string{"GGR201H1", "JEG201H1", "GGR203H1", "GGR372H1", "ENV200H1", "ENV234H1", "MST211H1", "CSE240H1", "ANT329H1"} {
		titles[c] = "x"
	}
	minor := &types.Program{Code: "ASMIN1252", Areas: types.StringList{"School of the Environment", "Geography and Planning"},
		Completion: Parse("(4.0 credits)\n_Applications_: Any 5 courses (2.5 credits) from Group A; up to 1.0 credit can be from Group B.\n_Group A (Environmental Geography Courses):_\nGGR201H1/ JEG201H1, GGR203H1, GGE372H1\n_Group B (School of the Environment Courses):_\nENV200H1, ENV234H1")}
	major := &types.Program{Code: "ASMAJ1252", Areas: types.StringList{"Geography and Planning"},
		Completion: Parse("_Applications:_ Any 5 courses (2.5 credits) from Group A; up to 1.0 credit can be from Group B; up to 0.5 credit can be from Group C.\nGroup A (Environmental Geography Courses)\nGGR201H1/ JEG201H1, GGR203H1, GGR372H1\nGroup B (EEB and ENV Courses)\nEEB240H1, ENV200H1\nGroup C\nAny other GGR, JGE, JIG, JUG courses at the 200/300/400-level not listed in Group A.")}
	wgs := &types.Program{Code: "ASMAJ0571", Areas: types.StringList{"Women and Gender Studies"},
		Completion: Parse("(7.0 credits, including 3.0 credits at the 300+ level including at least 0.5 WGS credits at the 400 level)\n5. 2.0 credits from group A or B\n**Group A**\nCSE240H1, ANT329H1\n**Group B**\nANT329H1, HST211H1/ PHS211H1")}
	programs := []*types.Program{minor, major, wgs}
	for _, p := range programs {
		linkListCodes(p, programs, titles)
	}
	got := func(c *types.Completion, name string) string {
		for _, l := range c.Lists {
			if l.Name == name {
				return NodeKey(l.Items[0])
			}
		}
		return ""
	}
	if g := got(minor.Completion, "Group A"); g != `{"type":"credits","courses":["GGR201H1","JEG201H1","GGR203H1","GGR372H1"]}` {
		t.Errorf("minor Group A = %s", g)
	}
	if !slices.Contains(minor.Completion.Notes, "No course in the calendar has the code GGE372H1 (Group A): read as GGR372H1, which a program of the same area lists on its Group A.") {
		t.Errorf("minor notes = %q", minor.Completion.Notes)
	}
	if g := got(wgs.Completion, "Group B"); g != `{"type":"credits","courses":["ANT329H1","HST211H1","PHS211H1"]}` {
		t.Errorf("WGS Group B = %s", g)
	}
	if !slices.Contains(wgs.Completion.Notes, "No course in the calendar has the code HST211H1 (Group B).") {
		t.Errorf("WGS notes = %q", wgs.Completion.Notes)
	}
}

// TestRenumberListRefs checks that renumbering rows renumbers the
// requirements a list item names, and that a reference to a row that is
// gone becomes noRow.
func TestRenumberListRefs(t *testing.T) {
	r1, r2, r3 := &types.Requirement{Number: 1}, &types.Requirement{Number: 2}, &types.Requirement{Number: 3}
	item := &types.ReqNode{Type: types.NodeCredits, Requirements: []int{3, 2}}
	c := &types.Completion{Requirements: []*types.Requirement{r1, r2, r3}, Lists: []*types.CourseList{{Name: "L", Items: []*types.ReqNode{item}}}}
	old := map[int]*types.Requirement{1: r1, 3: r3}
	renumber(c, []*types.Requirement{r3, r1}, func(r *types.Requirement, n int) *types.Requirement { return old[n] },
		func(r *types.Requirement) *types.Requirement { return nil })
	if !slices.Equal(item.Requirements, []int{1, noRow}) {
		t.Errorf("list item requirements = %v, want [1 %d]", item.Requirements, noRow)
	}
}

// TestInheritLists checks that copied rows bring, as copies, the source's lists
// they name and the lists those name; an identical list is shared by name, and
// a different one, or one naming the source's requirements, is a conflict.
func TestInheritLists(t *testing.T) {
	item := func(codes ...string) *types.ReqNode { return &types.ReqNode{Type: types.NodeCredits, Courses: codes} }
	src := &types.Completion{Lists: []*types.CourseList{
		{Name: "Group A", Items: []*types.ReqNode{item("CSB327H1"), {Type: types.NodeCredits, Lists: []string{"Group B"}}}},
		{Name: "Group B", Title: "Group B (Seminars)", Items: []*types.ReqNode{item("CSB427H1")}},
		{Name: "Group C", Items: []*types.ReqNode{item("CSB428H1")}},
	}}
	copies := []*types.Requirement{{Req: &types.ReqNode{Type: types.NodeCredits, Amount: 0.5, Lists: []string{"Group A"}}}}
	got, ok := inheritLists(&types.Completion{}, src, copies)
	if !ok || len(got) != 2 || got[0].Name != "Group A" || got[1].Name != "Group B" || got[1].Title != "Group B (Seminars)" || got[0].Items[0] == src.Lists[0].Items[0] {
		t.Fatalf("copy: got %v, %v", got, ok)
	}
	same := &types.Completion{Lists: []*types.CourseList{{Name: "Group B", Items: []*types.ReqNode{item("CSB427H1")}}}}
	if got, ok := inheritLists(same, src, copies); !ok || len(got) != 1 || got[0].Name != "Group A" {
		t.Errorf("same list: got %v, %v", got, ok)
	}
	other := &types.Completion{Lists: []*types.CourseList{{Name: "Group B", Items: []*types.ReqNode{item("CSB999H1")}}}}
	if _, ok := inheritLists(other, src, copies); ok {
		t.Errorf("conflict not reported")
	}
	src.Lists[1].Items = append(src.Lists[1].Items, &types.ReqNode{Type: types.NodeCredits, Requirements: []int{4}})
	if _, ok := inheritLists(&types.Completion{}, src, copies); ok {
		t.Errorf("a list referring to the source's requirements was copied")
	}
}

// TestLinkInheritanceRefs checks how copied rows keep their references (CMB
// Major, shortened): overlaps and filters naming copied rows are renumbered;
// naming rows not copied, an overlap goes and a filter makes the copy text.
func TestLinkInheritanceRefs(t *testing.T) {
	major := &types.Program{Code: "ASMAJ1003", Type: "Major", Title: "Cell & Molecular Biology Major (Science Program)",
		Completion: Parse("(8.0 credits)\nFirst Year:\n1. BIO120H1, BIO130H1; ( CHM135H1, CHM136H1)/ CHM151Y1\n2. 1.0 credit from JMB170Y1, MAT135H1, MAT136H1, PHY131H1\nHigher Years:\n1. BIO220H1, BIO230H1/ BIO255H1\n2. BIO260H1/ HMB265H1; BCH210H1\n3. CSB349H1\n4. 1.0 credit from: CSB327H1, CSB328H1, CSB329H1, CSB331H1, CSB340H1, CSB353H1\n5. 1.5 credits (at least 0.5 credit at the 400-level) from: BCH422H1, BCH426H1, CSB327H1, CSB328H1, CSB427H1, CSB428H1, HMB496Y1/ HMB499Y1. No more than 0.5 credit in BCH can be used towards this requirement.")}
	src := major.Completion
	label := func(l string) *types.Requirement {
		for _, r := range src.Requirements {
			if r.Label == l {
				return r
			}
		}
		t.Fatalf("no row %q in the Major", l)
		return nil
	}
	fy1, fy2, hy1, hy2, hy3, hy4, hy5 := label("First Year 1"), label("First Year 2"), label("Higher Years 1"), label("Higher Years 2"), label("Higher Years 3"), label("Higher Years 4"), label("Higher Years 5")
	hy3.Overlaps, hy4.Overlaps = []int{hy4.Number}, []int{hy3.Number, hy5.Number}
	hy5.Overlaps, fy1.Overlaps = []int{hy4.Number, fy1.Number}, []int{hy5.Number}
	hy1.Req = &types.ReqNode{Type: types.NodeCredits, Amount: 0.5, Requirements: []int{hy2.Number}}
	hy2.Req = &types.ReqNode{Type: types.NodeCredits, Amount: 0.5, Requirements: []int{fy2.Number}}
	focus := &types.Program{Code: "ASMAJ1003A", Type: "Major", Title: "Cell & Molecular Biology Major: Focus in Molecular Networks of the Cell",
		Completion: Parse("(2.0 credits)\nThis Focus is part of the Cell and Molecular Biology Major and begins with the requirements of lines 1-5 of Higher Years of the Cell and Molecular Biology Major Program.\n6. 0.5 credit from: CSB427H1, CSB428H1, CSB429H1")}
	byCode := map[string]*types.Program{"ASMAJ1003": major, "ASMAJ1003A": focus}
	if !linkInheritance(focus.Completion, focus.Code, byCode, programs.NewTitleIndex([]*types.Program{major, focus})) {
		t.Fatal("nothing copied")
	}
	checkRequirementRefs(focus.Completion)
	var got []string
	for _, r := range focus.Completion.Requirements {
		line := fmt.Sprintf("%d %s p%d %q ov%v %s", r.Number, r.Kind, r.Parent, r.Label, r.Overlaps, NodeKey(r.Req))
		for _, n := range r.Notes {
			line += " note: " + n
		}
		got = append(got, line)
	}
	want := []string{
		`1 allocation p0 "" ov[] {"type":"or","items":[{"type":"credits","amount":2},{"type":"text","kind":"other","text":"(2.0 credits)"}]}`,
		`2 allocation p1 "Higher Years 1" ov[] {"type":"credits","amount":0.5,"requirements":[3]}`,
		`3 allocation p1 "Higher Years 2" ov[] {"type":"text","kind":"other","text":"2. BIO260H1/ HMB265H1; BCH210H1"} note: It refers to a requirement of the program it was copied from that this program doesn't have.`,
		`4 allocation p1 "Higher Years 3" ov[5] {"type":"course","code":"CSB349H1"}`,
		`5 allocation p1 "Higher Years 4" ov[4 6] {"type":"credits","amount":1,"courses":["CSB327H1","CSB328H1","CSB329H1","CSB331H1","CSB340H1","CSB353H1"]}`,
		`6 allocation p1 "Higher Years 5" ov[5] {"type":"credits","amount":1.5,"courses":["BCH422H1","BCH426H1","CSB327H1","CSB328H1","CSB427H1","CSB428H1","HMB496Y1","HMB499Y1"]}`,
		`7 constraint p6 "" ov[] {"type":"credits","amount":0.5,"levels":[400]}`,
		`8 limit p6 "" ov[] {"type":"credits","designators":["BCH"],"max_amount":0.5}`,
		`9 limit p6 "" ov[] {"type":"credits","courses":["HMB496Y1","HMB499Y1"],"max_amount":1}`,
		`10 allocation p1 "6" ov[] {"type":"credits","amount":0.5,"courses":["CSB427H1","CSB428H1","CSB429H1"]}`,
	}
	if *updateReview {
		fmt.Printf("=== refs\n%s\n", strings.Join(got, "\n"))
		t.Errorf("-update-review: output printed, not checked")
	} else if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("\n got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestFocusChoices checks the rule a heading of focuses comes with (ASSPE2270,
// shortened): the or of the focuses' program nodes. Immunology, written without
// its code, is resolved by title by Link, or stays membership text (Review).
func TestFocusChoices(t *testing.T) {
	raw := "(10.5 or 11.0 credits plus a disciplinary focus requiring 2.0-3.5 credits)\n**Disciplinary Focuses**\nStudents in the Specialist Program in Statistical Science: Methods and Practice program must enrol in and complete at least one disciplinary focus.\nHealth Studies (ASFOC2270A): (2.0 credits) HMB342H1, at least 0.5 credit from HST209H1/ PHS209H1/ HST211H1/ PHS211H1/ HST250H1/ PHS250H1\nBiochemistry (ASFOC2270O): (3.0 credits)\nCHM135H1, CHM136H1, BCH210H1, BCH311H1, BCH370H1, BCH441H1\nImmunology (3.0 Credits) - BIO130H1 (70%), IMM250H1, IMM340H1, IMM350H1, IMM360H1, IMM429H1/ IMM430H1/ IMM431H1/ IMM435H1"
	self := "Specialist in Statistical Science: Methods and Practice (Science Program)"
	foci := func(c *types.Completion) string {
		for _, r := range c.Requirements {
			if r.Section == "Disciplinary Focuses" {
				return NodeKey(r.Req)
			}
		}
		return ""
	}
	c := Parse(raw)
	if got, want := foci(c), `{"type":"or","items":[{"type":"and","items":[{"type":"program","programs":["ASFOC2270A"],"text":"Health Studies"},{"type":"credits","amount":2,"courses":["HMB342H1","HST209H1","PHS209H1","HST211H1","PHS211H1","HST250H1","PHS250H1"]},{"type":"course","code":"HMB342H1"},{"type":"credits","amount":0.5,"courses":["HST209H1","PHS209H1","HST211H1","PHS211H1","HST250H1","PHS250H1"]}]},{"type":"and","items":[{"type":"program","programs":["ASFOC2270O"],"text":"Biochemistry"},{"type":"course","code":"CHM135H1"},{"type":"course","code":"CHM136H1"},{"type":"course","code":"BCH210H1"},{"type":"course","code":"BCH311H1"},{"type":"course","code":"BCH370H1"},{"type":"course","code":"BCH441H1"}]},{"type":"program","text":"Immunology"}]}`; got != want {
		t.Errorf("parsed:\n got: %s\nwant: %s", got, want)
	}
	// A stub catalogue entry: the real catalogue has no Immunology focus.
	linkFocusPrograms(programs.NewTitleIndex([]*types.Program{{Code: "ASFOC2270R", Title: "Focus in Immunology (Specialist in Statistical Science: Methods and Practice)"}}), c, self)
	if got, want := foci(c), `{"type":"or","items":[{"type":"and","items":[{"type":"program","programs":["ASFOC2270A"],"text":"Health Studies"},{"type":"credits","amount":2,"courses":["HMB342H1","HST209H1","PHS209H1","HST211H1","PHS211H1","HST250H1","PHS250H1"]},{"type":"course","code":"HMB342H1"},{"type":"credits","amount":0.5,"courses":["HST209H1","PHS209H1","HST211H1","PHS211H1","HST250H1","PHS250H1"]}]},{"type":"and","items":[{"type":"program","programs":["ASFOC2270O"],"text":"Biochemistry"},{"type":"course","code":"CHM135H1"},{"type":"course","code":"CHM136H1"},{"type":"course","code":"BCH210H1"},{"type":"course","code":"BCH311H1"},{"type":"course","code":"BCH370H1"},{"type":"course","code":"BCH441H1"}]},{"type":"program","programs":["ASFOC2270R"],"text":"Immunology"}]}`; got != want {
		t.Errorf("linked by title (stub ASFOC2270R):\n got: %s\nwant: %s", got, want)
	}
	c = Parse(raw)
	linkFocusPrograms(programs.NewTitleIndex(nil), c, self)
	if got, want := foci(c), `{"type":"or","items":[{"type":"and","items":[{"type":"program","programs":["ASFOC2270A"],"text":"Health Studies"},{"type":"credits","amount":2,"courses":["HMB342H1","HST209H1","PHS209H1","HST211H1","PHS211H1","HST250H1","PHS250H1"]},{"type":"course","code":"HMB342H1"},{"type":"credits","amount":0.5,"courses":["HST209H1","PHS209H1","HST211H1","PHS211H1","HST250H1","PHS250H1"]}]},{"type":"and","items":[{"type":"program","programs":["ASFOC2270O"],"text":"Biochemistry"},{"type":"course","code":"CHM135H1"},{"type":"course","code":"CHM136H1"},{"type":"course","code":"BCH210H1"},{"type":"course","code":"BCH311H1"},{"type":"course","code":"BCH370H1"},{"type":"course","code":"BCH441H1"}]},{"type":"text","kind":"membership","text":"Immunology (3.0 Credits) - BIO130H1 (70%), IMM250H1, IMM340H1, IMM350H1, IMM360H1, IMM429H1/ IMM430H1/ IMM431H1/ IMM435H1"}]}`; got != want {
		t.Errorf("no such title:\n got: %s\nwant: %s", got, want)
	}
}

// TestSlashWarnings checks the warning for "/" caps whose courses don't exclude
// each other ("STA414H1/ CSC412H1", ASSPE1687): none when the catalogue has
// them excluding each other, else one line naming the program, row and pair.
func TestSlashWarnings(t *testing.T) {
	raw := "(2.0 credits)\n3. 2.0 credits from the following list: STA414H1/ CSC412H1, CSC413H1; JSC470H1, CSC454H1, CSC490H1"
	c := Parse(raw)
	programList := []*types.Program{{Code: "ASSPE1687", Completion: c}}
	excl := func(code string) *types.Exclusions {
		return &types.Exclusions{Items: []types.ExclusionItem{{Courses: []string{code}}}}
	}
	both := &programs.Catalog{Exclusions: map[string]*types.Exclusions{"CSC412H1": excl("STA414H1")}}
	if got, want := slashWarnings(both, programList), []string{`completion: 0 of 1 "/" caps name courses that don't exclude each other`}; !slices.Equal(got, want) {
		t.Errorf("exclusive pair: got %q, want %q", got, want)
	}
	none := &programs.Catalog{Exclusions: map[string]*types.Exclusions{}}
	want := []string{
		"completion ASSPE1687: requirement 3 caps STA414H1/CSC412H1 at 0.5 credit, but STA414H1 and CSC412H1 don't exclude each other in the calendar",
		`completion: 1 of 1 "/" caps name courses that don't exclude each other`,
	}
	if got := slashWarnings(none, programList); !slices.Equal(got, want) {
		t.Errorf("pair that isn't exclusive: got %q, want %q", got, want)
	}
}
