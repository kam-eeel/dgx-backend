package parser

import (
	"flag"
	"fmt"
	"strings"
	"testing"

	"degree-explorer/parser/courses"
	"degree-explorer/parser/grammar"
	"degree-explorer/parser/programs/completion"
	"degree-explorer/types"
)

// TestLinkCompletionDesignators checks that Link keeps in completion filters
// only designators the catalogue has or the program names in a code: JAR is
// dropped with a note, and a pool left with no designator is text.
func TestLinkCompletionDesignators(t *testing.T) {
	tests := []struct{ code, raw string }{
		{"ASMAJ0151", "5. 2.0 credits chosen from other Department for the Study of Religion courses carrying any of the following designators: RLG, JAR, JCR, JRC, JNR, JRN, JPR, JSR"},
		{"ASMIN1689", "(4.0 credits)\n1. CSC108H1\nNotes:\n- Students enrolled in the Computer Science Minor program are limited to at most 1.5 credits from CSC/ECE courses at the 300-/400-level."},
		{"ASMAJ1228", "2. 2.0 credits comprised of the following core PCJ courses, all of which are required: PCJ200H1, PCJ210H1, PCJ350H1/ PCJ261H1, PCJ362H1.\n4. An additional 2.5 credits of PCJ-coded courses or complementary courses offered in other programs."},
		{"ASMIN1019", "4.0 credits, including at least 1.0 credit at the 300+ level. All 4.0 credits must be NMC and/or NML courses."},
		{"ASMIN0151", "(4.0 credits)\n1. 1.5 additional Department for the Study of Religion credits\nNotes:\n- JAR, JCR and JSR courses will be counted as Department for the Study of Religion courses. Courses offered by other academic units may not be counted towards this program."},
	}
	var catalogue []*types.Course
	for _, d := range []string{"CSC", "RLG", "JCR", "JRC", "JNR", "JRN", "JPR", "JSR"} {
		catalogue = append(catalogue, &types.Course{Code: d + "300H1"})
	}
	var programs []*types.Program
	for _, tt := range tests {
		programs = append(programs, &types.Program{Code: tt.code, Completion: completion.Parse(tt.raw)})
	}
	Link(catalogue, programs)
	want := map[string][]string{
		"ASMIN0151": {
			`1 allocation {"type":"or","items":[{"type":"credits","amount":4},{"type":"text","kind":"other","text":"(4.0 credits)"}]}`,
			`2 allocation p1 {"type":"credits","amount":1.5,"designators":["RLG","JCR","JSR"]}`,
			`  note: No course in the calendar has the designator JAR.`,
			`3 limit p1 {"type":"credits","exclude_designators":["RLG","JCR","JSR"],"max_amount":0}`,
			`  note: No course in the calendar has the designator JAR.`,
		},
		"ASMAJ0151": {
			`1 allocation {"type":"credits","amount":2,"designators":["RLG","JCR","JRC","JNR","JRN","JPR","JSR"]}`,
			`  note: No course in the calendar has the designator JAR.`,
		},
		"ASMIN1689": {
			`1 allocation {"type":"or","items":[{"type":"credits","amount":4},{"type":"text","kind":"other","text":"(4.0 credits)"}]}`,
			`2 allocation p1 {"type":"course","code":"CSC108H1"}`,
			`3 limit p1 {"type":"credits","designators":["CSC"],"levels":[300,400],"max_amount":1.5}`,
			`  note: No course in the calendar has the designator ECE.`,
		},
		"ASMAJ1228": {
			`1 allocation {"type":"and","items":[{"type":"course","code":"PCJ200H1"},{"type":"course","code":"PCJ210H1"},{"type":"or","items":[{"type":"course","code":"PCJ350H1"},{"type":"course","code":"PCJ261H1"}]},{"type":"course","code":"PCJ362H1"}]}`,
			`2 allocation {"type":"or","items":[{"type":"credits","amount":2.5,"designators":["PCJ"]},{"type":"text","kind":"content","text":"complementary courses offered in other programs"}]}`,
		},
		"ASMIN1019": {
			`1 allocation {"type":"text","kind":"other","text":"4.0 credits, including at least 1.0 credit at the 300+ level. All 4.0 credits must be NMC and/or NML courses"}`,
			`  note: No course in the calendar has the designators NMC, NML.`,
			`2 constraint p1 {"type":"or","items":[{"type":"credits","amount":1,"levels":[300,400]},{"type":"text","kind":"other","text":"including at least 1.0 credit at the 300+ level"}]}`,
		},
	}
	for i, tt := range tests {
		got := complCompact(programs[i].Completion)
		if *updateReview {
			fmt.Printf("=== %s\n%s\n", tt.code, strings.Join(got, "\n"))
			t.Errorf("%s: -update-review: output printed, not checked", tt.code)
			continue
		}
		if strings.Join(got, "\n") != strings.Join(want[tt.code], "\n") {
			t.Errorf("%s:\n got:\n%s\nwant:\n%s", tt.code, strings.Join(got, "\n"), strings.Join(want[tt.code], "\n"))
		}
	}
}

// linkCompact is complCompact with each row's label and section, and its
// raw's first line.
func linkCompact(c *types.Completion) []string {
	var out []string
	for _, line := range complCompact(c) {
		out = append(out, line)
	}
	for _, r := range c.Requirements {
		if r.Label != "" || r.Section != "" {
			out = append(out, fmt.Sprintf("row %d label %q section %q raw %q", r.Number, r.Label, r.Section, completion.RawHead(r)))
		}
	}
	return out
}

// TestLinkCompletion checks the completion steps of Link on real snippets
// (shortened) and a catalogue of the courses they need: topics courses,
// equivalents, program references and lists copied from other programs.
func TestLinkCompletion(t *testing.T) {
	type prog struct{ code, typ, title, area, raw string }
	progs := []prog{
		{"ASMAJ1124", "Major", "Hungarian Studies Major (Arts Program)", "Slavic Languages and Literatures", "(4.0 credits)\n1. 2.0 credits from: EUR200Y1, HIS241H1, HIS242H1, HIS251Y1, HIS389H1 (Hungarian History), HIS453H1, HUN355H1"},
		{"ASMAJ0110", "Major", "Conservation and Biodiversity Major (Science Program)", "Ecology and Evolutionary Biology", "4. 0.5 credit: EEB365H1 (applied conservation biology)"},
		{"ASMAJ2015", "Major", "Political Science Major (Arts Program)", "Political Science", "(1.5 credits, including at least 0.5 credit at the 300+ level)\n1. 1.0 credit from POL101H1, POL106H1, POL107H1\n2. 0.5 credit from JPI201H1, POL220H1, POL312H1, POL382H1, POL490H1\n   Some offerings of other POL Special Topics courses that cover Canadian politics may also be eligible as determined by the department."},
		{"ASCER2400", "Certificate", "Certificate in Business Fundamentals (Category 2)", "Rotman Commerce", "(1.0 credit)\nSuccessful completion of the following four courses (or their equivalents):\n1. MGT100H1\n2. MGT201H1\nNotes:\n- Students who move from the Rotman Commerce program (BCOM) to another degree program (HBA/HBSc) within Arts & Science would be able to use any completed RSM course equivalents towards the completion of the Certificate in Business Fundamentals (e.g. RSM100H1 would be deemed equivalent to MGT100H1 as part of the Certificate in Business Fundamentals, etc)."},
		{"ASMAJ1160", "Major", "Psychology Major (Science Program)", "Psychology", "2. PSY201H1 (or one of ECO220Y1/ EEB225H1)\nWe will accept equivalent/exclusion statistics courses from UTM/UTSC in lieu of PSY201H1. Please contact us to verify if your stats course qualifies."},
		{"ASMAJ2082", "Major", "Pharmacology Major (Science Program)", "Pharmacology and Toxicology", "1. PCL201H1\nNotes:\n1. Students are not allowed to enroll concurrently in the Major Program in Pharmacology and the Major Program in Toxicology.\n2. Students are not allowed to enroll concurrently in the Major Program in Pharmacology and a Specialist Program in Toxicology."},
		{"ASMAJ2573", "Major", "Biomedical Toxicology Major (Science Program)", "Pharmacology and Toxicology", "1. PCL201H1\nNotes:\n1. Students are not allowed to enroll concurrently in the Major Program in Pharmacology and the Major Program in Toxicology.\n2. Students are not allowed to enroll concurrently in the Major Program in Pharmacology and a Specialist Program in Toxicology."},
		{"ASSPE2573", "Specialist", "Biomedical Toxicology Specialist (Science Program)", "Pharmacology and Toxicology", ""},
		{"ASSPE0605", "Specialist", "Environment & Toxicology Specialist (Science Program)", "School of the Environment", ""},
		{"ASSPE2340", "Specialist", "Specialist in Pharmacology and Biomedical Toxicology (Science Program)", "Pharmacology and Toxicology", ""},
		{"ASMAJ1053", "Major", "Applied Genetics and Biotechnology Major (Science Program)", "Human Biology", "1. HMB265H1\n3. _The Applied Genetics and Biotechnology major cannot be paired with any other Human Biology Program managed major program._"},
		{"ASMAJ1472", "Major", "Neuroscience Major (Science Program)", "Human Biology", ""},
		{"ASSPE1472", "Specialist", "Neuroscience Specialist (Science Program)", "Human Biology", ""},
		{"ASSPE2431", "Specialist", "Management Specialist (BCom)", "Rotman Commerce", "1. RSM100H1\n- Students may enrol in only one Rotman Commerce Specialist."},
		{"ASSPE2038", "Specialist", "Finance and Economics Specialist (BCom)", "Rotman Commerce", ""},
		{"ASMIN1478", "Minor", "Economics Minor (Arts Program)", "Economics", "1. ECO101H1\nNotes:\n1. Students enrolled in this program cannot be simultaneously enrolled in any other Economics program or the Environmental Economics minor."},
		{"ASMAJ1478", "Major", "Economics Major (Arts Program)", "Economics", ""},
		{"ASMIN1438", "Minor", "Environmental Economics Minor (Arts Program)", "School of the Environment", ""},
		{"ASMIN2453", "Minor", "Business German Minor (Arts Program)", "German", "1. GER100Y1\nNote: Students enrolled in the German Studies Specialist of Major Program may not simultaneously enroll for a Business German Minor Program."},
		{"ASSPE1400", "Specialist", "German Studies Specialist (Arts Program)", "German", ""},
		{"ASMAJ1400", "Major", "German Studies Major (Arts Program)", "German", ""},
		{"ASMAJ1100", "Major", "Forest Conservation Major (Arts Program)", "Forest Conservation and Forest Biomaterials Science", "(1.5 credits)\nThird Year:\nFOR305H1; 1.0 credit from FOR300H1, FOR301H1\nAn additional 1.0 credit can be taken from any 3rd or 4th year elective listed above or under the Forest Conservation Specialist (Arts Program)."},
		{"ASSPE9038", "Specialist", "Finance Specialist (Test)", "Finance Test", "9. 2.0 credits from: RSM430H1, RSM432H1, RSM433H1, RSM434H1, RSM435H1, RSM436H1, RSM437H1, RSM438H1, RSM439H1, or any 400-level finance-themed RSM special topics course as determined by the Director of Rotman Commerce."},
		{"ASFOC9431", "Focus", "Marketing Focus (Test)", "Marketing Test", "2. Complete 1.0 credit from the following: RSM351H1, RSM355H1, RSM356H1 and any marketing-themed RSM special topics and RSM Summer Abroad courses as determined by the Director of Rotman Commerce."},
		{"ASSPE1100", "Specialist", "Forest Conservation Specialist (Arts Program)", "Forest Conservation and Forest Biomaterials Science", "(2.0 credits)\nThird Year:\n1. FOR301H1, FOR305H1; At least 1.0 credit from FOR300H1, FOR302H1\nFourth Year:\n2. 1.0 credit from ANT450H1, FOR403H1\nAn additional 1.0 credit can be taken from any 3rd or 4th year elective listed above."},
		{"ASMAJ1626", "Major", "European Affairs Major (Arts Program)", "European Affairs", "(2.0 credits)\nThe distribution of the 2.0 credits is as follows:\n- EUR498H1 - Special Topics in European Affairs: European Union, worth 0.5 credit.\n- The remaining 1.5 credits in elective courses chosen from a list of eligible electives or any course with European content approved by the Program Coordinator.\nFourth Year:\n1. EUR498H1\n2. Remaining courses to be selected from the list of eligible electives below. The most up-to date list of electives is on the program website. Students may also petition the Program Coordinator to take other courses in the calendar as electives, provided they have substantial European and/or Eurasian content.\nCurrent Eligible Electives (for approval on courses outside the list or for transfer credits, please consult the Program Coordinator):\nEUR300H1, EUR301H1, MUN200H1, ENG329H1"},
		{"ASMIN1626", "Minor", "European Affairs Minor (Arts Program)", "European Affairs", "(3.0 credits)\n1. EUR200Y1\n3. 2.0 credits in eligible elective courses listed in the European Affairs Major"},
	}
	courses := []*types.Course{
		{Code: "HIS389H1", Title: "Topics in History"},
		{Code: "EEB365H1", Title: "Topics in Applied Conservation Biology"},
		{Code: "POL382H1", Title: "Topics in Canadian Politics"},
		{Code: "POL490H1", Title: "Topics in Canadian Politics I"},
		{Code: "POL377H1", Title: "Topics in Comparative Politics I"},
		{Code: "POL101H1", Title: "Democracy, Dictatorship, War, and Peace"},
		{Code: "RSM310H1", Title: "Special Topics in Management"},
		{Code: "RSM406H1", Title: "Special Topics in Management"},
		{Code: "RSM407H1", Title: "Special Topics in Management"},
		{Code: "RSM430H1", Title: "Mergers and Acquisitions"},
		{Code: "MGT100H1", Exclusions: courses.ParseExclusions("RSM100H1,  RSM100Y1")},
		{Code: "MGT201H1", Exclusions: courses.ParseExclusions("RSM219H1")},
		{Code: "PSY201H1", Exclusions: courses.ParseExclusions("ECO220Y1/  EEB225H1/  STA220H1/  ECO220Y5/  PSY201H5/  STAB22H3")},
	}
	var programs []*types.Program
	byCode := map[string]*types.Program{}
	for _, p := range progs {
		pr := &types.Program{Code: p.code, Type: p.typ, Title: p.title, Areas: types.StringList{p.area}}
		if p.raw != "" {
			pr.Completion = completion.Parse(p.raw)
		}
		programs = append(programs, pr)
		byCode[p.code] = pr
	}
	Link(courses, programs)
	want := map[string][]string{
		"ASMAJ1124": {
			`1 allocation {"type":"or","items":[{"type":"credits","amount":4},{"type":"text","kind":"other","text":"(4.0 credits)"}]}`,
			`2 allocation p1 {"type":"or","items":[{"type":"credits","amount":2,"courses":["EUR200Y1","HIS241H1","HIS242H1","HIS251Y1","HIS453H1","HUN355H1"]},{"type":"and","items":[{"type":"credits","amount":2,"courses":["EUR200Y1","HIS241H1","HIS242H1","HIS251Y1","HIS389H1","HIS453H1","HUN355H1"]},{"type":"text","kind":"content","text":"HIS389H1 (Hungarian History)"}]}]}`,
			`row 2 label "1" section "" raw "1. 2.0 credits from: EUR200Y1, HIS241H1, HIS242H1, HIS251Y1, HIS389H1 (Hungarian History), HIS453H1, HUN355H1"`,
		},
		"ASSPE9038": {
			`1 allocation {"type":"or","items":[{"type":"credits","amount":2,"courses":["RSM430H1","RSM432H1","RSM433H1","RSM434H1","RSM435H1","RSM436H1","RSM437H1","RSM438H1","RSM439H1"]},{"type":"and","items":[{"type":"credits","amount":2,"courses":["RSM430H1","RSM432H1","RSM433H1","RSM434H1","RSM435H1","RSM436H1","RSM437H1","RSM438H1","RSM439H1","RSM406H1","RSM407H1"]},{"type":"text","kind":"permission","text":"any 400-level finance-themed RSM special topics course as determined by the Director of Rotman Commerce"}]}]}`,
			`row 1 label "9" section "" raw "9. 2.0 credits from: RSM430H1, RSM432H1, RSM433H1, RSM434H1, RSM435H1, RSM436H1, RSM437H1, RSM438H1, RSM439H1, or any 400-level finance-themed RSM special topics course as determined by the Director of Rotman Commerce."`,
		},
		"ASFOC9431": {
			`1 allocation {"type":"or","items":[{"type":"credits","amount":1,"courses":["RSM351H1","RSM355H1","RSM356H1"]},{"type":"and","items":[{"type":"credits","amount":1,"designators":["RSM"],"courses":["RSM351H1","RSM355H1","RSM356H1"]},{"type":"text","kind":"permission","text":"any marketing-themed RSM special topics and RSM Summer Abroad courses as determined by the Director of Rotman Commerce"}]}]}`,
			`row 1 label "2" section "" raw "2. Complete 1.0 credit from the following: RSM351H1, RSM355H1, RSM356H1 and any marketing-themed RSM special topics and RSM Summer Abroad courses as determined by the Director of Rotman Commerce."`,
		},
		"ASMAJ0110": {
			`1 allocation {"type":"course","code":"EEB365H1"}`,
			`row 1 label "4" section "" raw "4. 0.5 credit: EEB365H1 (applied conservation biology)"`,
		},
		"ASMAJ2015": {
			`1 allocation {"type":"credits","amount":1.5}`,
			`2 constraint p1 {"type":"credits","amount":0.5,"levels":[300,400]}`,
			`3 allocation p1 {"type":"credits","amount":1,"courses":["POL101H1","POL106H1","POL107H1"]}`,
			`4 allocation p1 {"type":"or","items":[{"type":"credits","amount":0.5,"courses":["JPI201H1","POL220H1","POL312H1","POL382H1","POL490H1"]},{"type":"and","items":[{"type":"credits","amount":0.5,"courses":["JPI201H1","POL220H1","POL312H1","POL382H1","POL490H1","POL377H1"]},{"type":"text","kind":"permission","text":"Some offerings of other POL Special Topics courses that cover Canadian politics may also be eligible as determined by the department"}]}]}`,
			`row 3 label "1" section "" raw "1. 1.0 credit from POL101H1, POL106H1, POL107H1"`,
			`row 4 label "2" section "" raw "2. 0.5 credit from JPI201H1, POL220H1, POL312H1, POL382H1, POL490H1"`,
		},
		"ASCER2400": {
			`1 allocation {"type":"credits","amount":1}`,
			`2 allocation p1 {"type":"or","items":[{"type":"course","code":"MGT100H1"},{"type":"course","code":"RSM100H1"},{"type":"course","code":"RSM100Y1"}]}`,
			`3 allocation p1 {"type":"or","items":[{"type":"course","code":"MGT201H1"},{"type":"course","code":"RSM219H1"}]}`,
			`row 2 label "1" section "" raw "1. MGT100H1"`,
			`row 3 label "2" section "" raw "2. MGT201H1"`,
		},
		"ASMAJ1160": {
			`1 allocation {"type":"or","items":[{"type":"course","code":"PSY201H1"},{"type":"and","items":[{"type":"or","items":[{"type":"credits","amount":0.5,"courses":["ECO220Y5"],"weight":0.5},{"type":"course","code":"PSY201H5"},{"type":"course","code":"STAB22H3"}]},{"type":"text","kind":"permission","text":"the department confirms the course qualifies"}]},{"type":"credits","amount":0.5,"courses":["ECO220Y1"],"weight":0.5},{"type":"course","code":"EEB225H1"}]}`,
			`row 1 label "2" section "" raw "2. PSY201H1 (or one of ECO220Y1/ EEB225H1)"`,
		},
		"ASMAJ2082": {
			`1 allocation {"type":"course","code":"PCL201H1"}`,
			`exclusion: {"type":"program","programs":["ASMAJ2573"],"text":"Major Program in Toxicology"}`,
			`exclusion: {"type":"program","programs":["ASSPE2573","ASSPE2340"],"text":"Specialist Program in Toxicology"}`,
			`exclusion: {"type":"or","items":[{"type":"not","items":[{"type":"program","programs":["ASSPE0605"],"text":"Specialist Program in Toxicology"}]},{"type":"text","kind":"ambiguity","text":"\"Specialist Program in Toxicology\" may also cover the Environment \u0026 Toxicology Specialist"}]}`,
			`row 1 label "1" section "" raw "1. PCL201H1"`,
		},
		"ASMAJ2573": {
			`1 allocation {"type":"course","code":"PCL201H1"}`,
			`note: Students are not allowed to enroll concurrently in the Major Program in Pharmacology and a Specialist Program in Toxicology`,
			`exclusion: {"type":"program","programs":["ASMAJ2082"],"text":"Major Program in Pharmacology"}`,
			`row 1 label "1" section "" raw "1. PCL201H1"`,
		},
		"ASMAJ1053": {
			`1 allocation {"type":"course","code":"HMB265H1"}`,
			`exclusion: {"type":"program","programs":["ASMAJ1472"],"text":"any other Human Biology Program managed major program"}`,
			`row 1 label "1" section "" raw "1. HMB265H1"`,
		},
		"ASSPE2431": {
			`1 allocation {"type":"course","code":"RSM100H1"}`,
			`exclusion: {"type":"program","programs":["ASSPE2038"],"text":"Rotman Commerce Specialist"}`,
			`row 1 label "1" section "" raw "1. RSM100H1"`,
		},
		"ASMIN1478": {
			`1 allocation {"type":"course","code":"ECO101H1"}`,
			`exclusion: {"type":"program","programs":["ASMAJ1478"],"text":"other Economics program"}`,
			`exclusion: {"type":"or","items":[{"type":"not","items":[{"type":"program","programs":["ASSPE2038"],"text":"other Economics program"}]},{"type":"text","kind":"ambiguity","text":"\"other Economics program\" may also cover the Finance and Economics Specialist"}]}`,
			`exclusion: {"type":"program","programs":["ASMIN1438"],"text":"Environmental Economics minor"}`,
			`row 1 label "1" section "" raw "1. ECO101H1"`,
		},
		"ASMIN2453": {
			`1 allocation {"type":"course","code":"GER100Y1"}`,
			`exclusion: {"type":"program","programs":["ASSPE1400","ASMAJ1400"],"text":"German Studies Specialist of Major Program"}`,
			`row 1 label "1" section "" raw "1. GER100Y1"`,
		},
		"ASMAJ1100": {
			`1 allocation {"type":"credits","amount":1.5}`,
			`2 allocation p1 {"type":"and","items":[{"type":"course","code":"FOR305H1"},{"type":"credits","amount":1,"courses":["FOR300H1","FOR301H1"]}]}`,
			`3 allocation p1 {"type":"credits","amount":1,"courses":["FOR300H1","FOR301H1","FOR302H1","ANT450H1","FOR403H1"]}`,
			`row 2 label "" section "Third Year" raw "FOR305H1; 1.0 credit from FOR300H1, FOR301H1"`,
		},
		"ASMAJ1626": {
			`1 allocation {"type":"or","items":[{"type":"credits","amount":2},{"type":"text","kind":"permission","text":"(2.0 credits)"}]}`,
			`2 allocation p1 {"type":"course","code":"EUR498H1"}`,
			`3 allocation p1 {"type":"or","items":[{"type":"credits","amount":1.5,"lists":["Current Eligible Electives"]},{"type":"text","kind":"permission","text":"Students may also petition the Program Coordinator to take other courses in the calendar as electives, provided they have substantial European and/or Eurasian content"}]}`,
			`  note: The most up-to date list of electives is on the program website`,
			`list: Current Eligible Electives: [{"type":"credits","courses":["EUR300H1","EUR301H1","MUN200H1","ENG329H1"]}]`,
			`note: No course in the calendar has the code EUR300H1 (Current Eligible Electives).`,
			`note: No course in the calendar has the code EUR301H1 (Current Eligible Electives).`,
			`note: No course in the calendar has the code MUN200H1 (Current Eligible Electives).`,
			`note: No course in the calendar has the code ENG329H1 (Current Eligible Electives).`,
			`row 2 label "Fourth Year 1" section "Fourth Year" raw "1. EUR498H1"`,
			`row 3 label "Fourth Year 2" section "Fourth Year" raw "2. Remaining courses to be selected from the list of eligible electives below. The most up-to date list of electives is on the program website. Students may also petition the Program Coordinator to take other courses in the calendar as electives, provided they have substantial European and/or Eurasian content."`,
		},
		"ASMIN1626": {
			`1 allocation {"type":"or","items":[{"type":"credits","amount":3},{"type":"text","kind":"permission","text":"(3.0 credits)"}]}`,
			`2 allocation p1 {"type":"course","code":"EUR200Y1"}`,
			`3 allocation p1 {"type":"or","items":[{"type":"credits","amount":2,"lists":["Current Eligible Electives"]},{"type":"text","kind":"permission","text":"Students may also petition the Program Coordinator to take other courses in the calendar as electives, provided they have substantial European and/or Eurasian content"}]}`,
			`list: Current Eligible Electives: [{"type":"credits","courses":["EUR300H1","EUR301H1","MUN200H1","ENG329H1"]}]`,
			`row 2 label "1" section "" raw "1. EUR200Y1"`,
			`row 3 label "3" section "" raw "3. 2.0 credits in eligible elective courses listed in the European Affairs Major"`,
		},
		"ASSPE1100": {
			`1 allocation {"type":"credits","amount":2}`,
			`2 allocation p1 {"type":"and","items":[{"type":"course","code":"FOR301H1"},{"type":"course","code":"FOR305H1"},{"type":"credits","amount":1,"courses":["FOR300H1","FOR302H1"]}]}`,
			`3 allocation p1 {"type":"credits","amount":1,"courses":["ANT450H1","FOR403H1"]}`,
			`4 allocation p1 {"type":"credits","amount":1,"courses":["FOR300H1","FOR302H1","ANT450H1","FOR403H1"]}`,
			`row 2 label "1" section "Third Year" raw "1. FOR301H1, FOR305H1; At least 1.0 credit from FOR300H1, FOR302H1"`,
			`row 3 label "2" section "Fourth Year" raw "2. 1.0 credit from ANT450H1, FOR403H1"`,
		},
	}

	if r := byCode["ASCER2400"].Completion.Requirements[1]; !strings.HasSuffix(r.Raw, "\nSuccessful completion of the following four courses (or their equivalents):") {
		t.Errorf("ASCER2400 row 2 raw = %q", r.Raw)
	}
	for _, p := range progs {
		if p.raw == "" {
			continue
		}
		got := linkCompact(byCode[p.code].Completion)
		if *updateReview {
			fmt.Printf("=== %s\n%s\n", p.code, strings.Join(got, "\n"))
			t.Errorf("%s: -update-review: output printed, not checked", p.code)
			continue
		}
		if strings.Join(got, "\n") != strings.Join(want[p.code], "\n") {
			t.Errorf("%s:\n got:\n%s\nwant:\n%s", p.code, strings.Join(got, "\n"), strings.Join(want[p.code], "\n"))
		}
	}
}

// TestLinkInheritance is the golden test of inheritance: the Molecular Networks
// focus (ASMAJ1003A) starts with copies of the Major's First Year and Higher
// Years 1-3 rows, then its own, renumbered; an unknown program stays text.
func TestLinkInheritance(t *testing.T) {
	major := &types.Program{Code: "ASMAJ1003", Type: "Major", Title: "Cell & Molecular Biology Major (Science Program)",
		Completion: completion.Parse("(8.0 credits)\nFirst Year:\n1. BIO120H1, BIO130H1; ( CHM135H1, CHM136H1)/ CHM151Y1\n2. 1.0 credit from JMB170Y1/ MAT130H1/ MAT135H1/ MAT136H1/ ( MAT148H1, MAT149H1)/ MAT137Y1/ ( MAT158H1, MAT159H1)/ MAT157Y1/ PHY131H1/ PHY132H1/ PHY151H1/ PHY152H1\nHigher Years:\n1. BIO220H1, BIO230H1/ BIO255H1\n2. BIO260H1/ HMB265H1; BCH210H1\n3. CSB349H1\n4. 1.0 credit from: CSB327H1, CSB328H1, CSB329H1, CSB331H1, CSB340H1, CSB353H1\n5. 1.5 credits (at least 0.5 credit at the 400-level) from: BCH422H1, BCH426H1, BCH440H1, BCH444H1, BCH445H1, CJH332H1, CSB299Y1, CSB327H1, CSB328H1, CSB329H1, CSB330H1, CSB331H1, CSB340H1, CSB350H1, CSB351Y1, CSB352H1, CSB353H1, CSB360H1, CSB397Y0, CSB399Y1, CSB427H1, CSB428H1, CSB429H1, CSB430H1, CSB431H1, CSB435H1, CSB447H1, CSB450H1, CSB451H1, CSB452H1, CSB453H1, CSB454H1, CSB457H1, CSB458H1, CSB459H1, CSB460H1, CSB471H1, CSB472H1, CSB473H1, CSB474H1, CSB475H1, CSB483H1, CSB490H1, CSB491H1, CSB492H1, CSB497H1, CSB498Y1, CSB499Y1, HMB496Y1/ HMB499Y1, MGY441H1, MGY480Y1. No more than 0.5 credit in BCH can be used towards this requirement.\nThe Cell & Molecular Biology Major Program has the additional option of a Disciplinary Focus.\nStudents in this program have the option to complete the Arts & Science Internship Program (ASIP) stream.")}
	focusRaw := "This Focus is part of the Cell and Molecular Biology Major and begins with the requirements of First Year and of lines 1-3 of Higher Years of the Cell and Molecular Biology Major Program.\n4. 1.0 credit from: CJH332H1, CSB327H1, CSB331H1, CSB353H1, CSB360H1\n5. 0.5 credit from: CSB427H1, CSB428H1, CSB429H1, CSB435H1, CSB451H1, CSB453H1, CSB454H1, CSB457H1, CSB458H1, CSB459H1, CSB460H1, CSB475H1\n6. 1.0 credit from: BCH422H1, BCH426H1, BCH444H1, BCH445H1, CJH332H1, CSB299Y1, CSB327H1, CSB330H1, CSB331H1, CSB353H1, CSB360H1, CSB397Y0, CSB399Y1, CSB427H1, CSB428H1, CSB429H1, CSB435H1, CSB450H1, CSB451H1, CSB452H1, CSB453H1, CSB454H1, CSB457H1, CSB458H1, CSB459H1, CSB460H1, CSB475H1, CSB490H1, CSB491H1, CSB497H1, CSB498Y1, CSB499Y1. No more than 0.5 credit in BCH can be used towards this requirement.\n7. Molecular Network Learning Community (each year of focus enrolment)"
	focus := &types.Program{Code: "ASMAJ1003A", Type: "Major", Title: "Cell & Molecular Biology Major: Focus in Molecular Networks of the Cell",
		Completion: completion.Parse(focusRaw)}
	orphanRaw := strings.Replace(focusRaw, "Cell and Molecular Biology Major Program", "Underwater Basket Weaving Major Program", 1)
	orphan := &types.Program{Code: "ASMAJ9999A", Type: "Major", Title: "Underwater Basket Weaving Major: Focus in Reeds",
		Completion: completion.Parse(orphanRaw)}
	Link(nil, []*types.Program{major, focus, orphan})

	sentence := "This Focus is part of the Cell and Molecular Biology Major and begins with the requirements of First Year and of lines 1-3 of Higher Years of the Cell and Molecular Biology Major Program."
	want := []string{
		`1 allocation {"type":"and","items":[{"type":"course","code":"BIO120H1"},{"type":"course","code":"BIO130H1"},{"type":"or","items":[{"type":"and","items":[{"type":"course","code":"CHM135H1"},{"type":"course","code":"CHM136H1"}]},{"type":"course","code":"CHM151Y1"}]}]}`,
		`2 allocation {"type":"or","items":[{"type":"credits","amount":1,"courses":["JMB170Y1","MAT130H1","MAT135H1","MAT136H1","MAT137Y1","MAT157Y1","PHY131H1","PHY132H1","PHY151H1","PHY152H1"]},{"type":"and","items":[{"type":"course","code":"MAT148H1"},{"type":"course","code":"MAT149H1"}]},{"type":"and","items":[{"type":"course","code":"MAT158H1"},{"type":"course","code":"MAT159H1"}]}]}`,
		`3 allocation {"type":"and","items":[{"type":"course","code":"BIO220H1"},{"type":"or","items":[{"type":"course","code":"BIO230H1"},{"type":"course","code":"BIO255H1"}]}]}`,
		`4 allocation {"type":"and","items":[{"type":"or","items":[{"type":"course","code":"BIO260H1"},{"type":"course","code":"HMB265H1"}]},{"type":"course","code":"BCH210H1"}]}`,
		`5 allocation {"type":"course","code":"CSB349H1"}`,
		`6 allocation {"type":"credits","amount":1,"courses":["CJH332H1","CSB327H1","CSB331H1","CSB353H1","CSB360H1"]}`,
		`7 allocation {"type":"credits","amount":0.5,"courses":["CSB427H1","CSB428H1","CSB429H1","CSB435H1","CSB451H1","CSB453H1","CSB454H1","CSB457H1","CSB458H1","CSB459H1","CSB460H1","CSB475H1"]}`,
		`8 allocation {"type":"credits","amount":1,"courses":["BCH422H1","BCH426H1","BCH444H1","BCH445H1","CJH332H1","CSB299Y1","CSB327H1","CSB330H1","CSB331H1","CSB353H1","CSB360H1","CSB397Y0","CSB399Y1","CSB427H1","CSB428H1","CSB429H1","CSB435H1","CSB450H1","CSB451H1","CSB452H1","CSB453H1","CSB454H1","CSB457H1","CSB458H1","CSB459H1","CSB460H1","CSB475H1","CSB490H1","CSB491H1","CSB497H1","CSB498Y1","CSB499Y1"]}`,
		`9 limit p8 {"type":"credits","designators":["BCH"],"max_amount":0.5}`,
		`10 allocation {"type":"profile","attr":"has_learning_community","value":true,"text":"Molecular Network Learning Community (each year of focus enrolment)"}`,
		`row 1 label "First Year 1" section "First Year" raw "1. BIO120H1, BIO130H1; ( CHM135H1, CHM136H1)/ CHM151Y1"`,
		`row 2 label "First Year 2" section "First Year" raw "2. 1.0 credit from JMB170Y1/ MAT130H1/ MAT135H1/ MAT136H1/ ( MAT148H1, MAT149H1)/ MAT137Y1/ ( MAT158H1, MAT159H1)/ MAT157Y1/ PHY131H1/ PHY132H1/ PHY151H1/ PHY152H1"`,
		`row 3 label "Higher Years 1" section "Higher Years" raw "1. BIO220H1, BIO230H1/ BIO255H1"`,
		`row 4 label "Higher Years 2" section "Higher Years" raw "2. BIO260H1/ HMB265H1; BCH210H1"`,
		`row 5 label "Higher Years 3" section "Higher Years" raw "3. CSB349H1"`,
		`row 6 label "4" section "" raw "4. 1.0 credit from: CJH332H1, CSB327H1, CSB331H1, CSB353H1, CSB360H1"`,
		`row 7 label "5" section "" raw "5. 0.5 credit from: CSB427H1, CSB428H1, CSB429H1, CSB435H1, CSB451H1, CSB453H1, CSB454H1, CSB457H1, CSB458H1, CSB459H1, CSB460H1, CSB475H1"`,
		`row 8 label "6" section "" raw "6. 1.0 credit from: BCH422H1, BCH426H1, BCH444H1, BCH445H1, CJH332H1, CSB299Y1, CSB327H1, CSB330H1, CSB331H1, CSB353H1, CSB360H1, CSB397Y0, CSB399Y1, CSB427H1, CSB428H1, CSB429H1, CSB435H1, CSB450H1, CSB451H1, CSB452H1, CSB453H1, CSB454H1, CSB457H1, CSB458H1, CSB459H1, CSB460H1, CSB475H1, CSB490H1, CSB491H1, CSB497H1, CSB498Y1, CSB499Y1. No more than 0.5 credit in BCH can be used towards this requirement."`,
		`row 10 label "7" section "" raw "7. Molecular Network Learning Community (each year of focus enrolment)"`,
	}
	got := linkCompact(focus.Completion)
	if *updateReview {
		fmt.Printf("=== ASMAJ1003A\n%s\n", strings.Join(got, "\n"))
		t.Errorf("-update-review: output printed, not checked")
	} else if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("ASMAJ1003A:\n got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for _, r := range focus.Completion.Requirements[:5] {
		if !strings.HasSuffix(r.Raw, "\n"+sentence) {
			t.Errorf("row %d: raw %q doesn't end with the sentence", r.Number, r.Raw)
		}
	}
	// No aliasing: changing the Major's rows leaves the copies alone.
	for _, r := range major.Completion.Requirements {
		grammar.WalkReq(r.Req, func(n *types.ReqNode) {
			n.Code, n.Text = "XXX000H1", "changed"
			if n.Courses != nil {
				n.Courses[0] = "XXX000H1"
			}
		})
	}
	if got2 := linkCompact(focus.Completion); strings.Join(got2, "\n") != strings.Join(got, "\n") {
		t.Errorf("the copies changed with the Major's rows:\n%s", strings.Join(got2, "\n"))
	}
	// Unresolved: the sentence stays a row of text.
	if r := orphan.Completion.Requirements[0]; r.Req.Type != types.NodeText || !strings.HasPrefix(r.Raw, "This Focus is part of the Cell") {
		t.Errorf("orphan focus row 1 = %s %q", completion.NodeKey(r.Req), r.Raw)
	}
}

var updateReview = flag.Bool("update-review", false, "print the completion cases' compact output and raws instead of checking them (the cases fail)")
