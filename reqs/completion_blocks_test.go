package reqs

import (
	"fmt"
	"strings"
	"testing"
)

// Completion snippets below are copied from the catalogue (2026-10-15). Long
// course lists are cut after a few codes.

// blkDump renders a block tree one block per line: kind, label, section, the
// heading a heading sits under ("< …") and the first words of the text,
// children indented.
func blkDump(bs []*compBlock) string {
	var b strings.Builder
	var walk func(bs []*compBlock, depth int)
	walk = func(bs []*compBlock, depth int) {
		for _, x := range bs {
			words := strings.Fields(x.text)
			words = words[:min(len(words), 3)]
			under := ""
			if x.kind == blockHeading && x.heading != nil {
				under = fmt.Sprintf(" < %q", x.heading.section)
			}
			fmt.Fprintf(&b, "%s%s %q %q%s %s\n", strings.Repeat("  ", depth), x.kind, x.label, x.section, under, strings.Join(words, " "))
			walk(x.children, depth+1)
		}
	}
	walk(bs, 0)
	return b.String()
}

// blkSource rebuilds a block's line from its marker, label and text: the raw
// line without indentation and emphasis markers.
func blkSource(b *compBlock) string {
	s := b.text
	if b.kind == blockLabelled {
		s = b.label + ": " + s
	}
	if b.marker != "" {
		s = b.marker + " " + s
	}
	return s
}

// blkSameLine reports whether a block's source matches its raw line, ignoring
// emphasis markers, footnote asterisks (dropped from labels) and whitespace.
// So it can't catch a lost asterisk.
func blkSameLine(b *compBlock, raw string) bool {
	squash := func(s string) string { return strings.ReplaceAll(strings.Join(strings.Fields(s), ""), "*", "") }
	return squash(blkSource(b)) == squash(stripEmphasis(raw))
}

func TestCompletionBlocks(t *testing.T) {
	tests := []struct{ name, raw, want string }{
		{"empty", "", ""},
		{"whitespace only", "  \n\n   \n", ""},
		{"ASMAJ1689: year headings, numbering that runs on across them, lettered notes, bullets under a lead-in",
			"(8.0 credits, including at least one 0.5 credit at the 400-level)\n" +
				"_First year_ (2.5 credits):\n" +
				"1. ( CSC108H1, CSC148H1, CSC165H1/ CSC240H1)/ ( CSC110Y1, CSC111H1); ( MAT148H1/ MAT158H1/ MAT130H1/ MAT135H1, MAT149H1/ MAT159H1/ MAT136H1)/ MAT137Y1/ MAT157Y1\n" +
				"Notes:\n" +
				"a. Students with a strong background in an object-oriented language such as Python, Java or C++ may omit CSC108H1 and proceed directly with CSC148H1.\n" +
				"b. CSC240H1 is an accelerated and enriched version of CSC165H1 plus CSC236H1, intended for students with a strong mathematical background, or who develop an interest after taking CSC165H1.\n" +
				"_Second year_ (2.5 credits):\n" +
				"2. CSC207H1, CSC236H1/ CSC240H1, CSC258H1, CSC263H1/ CSC265H1, STA247H1/ STA237H1/ STA255H1/ STA257H1\n" +
				"_Later years_ (3.0 credits):\n" +
				"3. 3.0 credits of courses in total selected from among the following groups:\n" +
				"- **Group A:** CSC209H1; any 300-/ 400-level CSC course _except_ those listed in Group B\n" +
				"- **Group B:** CSC299H1, CSC299Y1, CSC394H1, CSC395H1\n" +
				"These 3.0 credits must include:\n" +
				"- at least 1.0 credit at the 300-/400-level from Group A",
			`prose "" "" (8.0 credits, including
heading "" "First year" First year (2.5
item "1" "First year" ( CSC108H1, CSC148H1,
heading "" "Notes" < "First year" Notes:
item "a" "Notes" Students with a
item "b" "Notes" CSC240H1 is an
heading "" "Second year" Second year (2.5
item "2" "Second year" CSC207H1, CSC236H1/ CSC240H1,
heading "" "Later years" Later years (3.0
item "3" "Later years" 3.0 credits of
  labelled "Group A" "Later years" CSC209H1; any 300-/
  labelled "Group B" "Later years" CSC299H1, CSC299Y1, CSC394H1,
  prose "" "Later years" These 3.0 credits
    bullet "" "Later years" at least 1.0
`},
		{"ASMAJ1003A: items under no heading",
			"This Focus is part of the Cell and Molecular Biology Major and begins with the requirements of First Year and of lines 1-3 of Higher Years of the Cell and Molecular Biology Major Program.\n" +
				"4. 1.0 credit from: CJH332H1, CSB327H1, CSB331H1, CSB353H1, CSB360H1\n" +
				"7. Molecular Network Learning Community (each year of focus enrolment)",
			`prose "" "" This Focus is
item "4" "" 1.0 credit from:
item "7" "" Molecular Network Learning
`},
		{"ASMAJ2015: numbering that starts under Higher Years; continuation lines; a heading takes no bullets",
			"(7.0 credits)\n" +
				"Of these:\n" +
				"- 2.0 credits at the 300+ level including a 0.5 credit at the 400 level.\n" +
				"First Year:\n" +
				"- 1.0 credit from: POL101H1, POL106H1, POL107H1, POL109H1. A maximum of 0.5 credit may be substituted from the following first-year courses: POL191H1, POL192H1, POL193H1\n" +
				"Higher Years:\n" +
				"1. POL200Y1\n" +
				"4. 0.5 credit at the 200+ level in Canadian Politics from JPI201H1, POL220H1, POL312H1\n" +
				"   Some offerings of other POL Special Topics courses that cover Canadian politics may also be eligible as determined by the department.\n" +
				"5. POL222H1\n" +
				"   (or one of ECO220Y1, PSY201H1, SOC202H1, STA220H1). Please note only 0.5 of ECO220Y1 will be accepted towards the program.\n" +
				"Note:\n" +
				"- Some courses may contribute to both the Canadian Politics and Diversity and Identity requirements (in requirements 4 and 6).",
			`prose "" "" (7.0 credits)
prose "" "" Of these:
  bullet "" "" 2.0 credits at
heading "" "First Year" First Year:
bullet "" "First Year" 1.0 credit from:
heading "" "Higher Years" Higher Years:
item "Higher Years 1" "Higher Years" POL200Y1
item "Higher Years 4" "Higher Years" 0.5 credit at
  prose "" "Higher Years" Some offerings of
item "Higher Years 5" "Higher Years" POL222H1
  prose "" "Higher Years" (or one of
heading "" "Note" Note:
bullet "" "Note" Some courses may
`},
		{"ASSPE0608: numbering restarts under each year heading; nested (1) items; lettered NOTES",
			"(13.0 credits)\n" +
				"First Year:\n" +
				"1. ( MAT130H1/ MAT135H1, MAT136H1) with an average grade of 73%/ ( MAT148H1, MAT149H1)/ MAT137Y1 with an average grade of 63%\n" +
				"4. STA130H1\n" +
				"Note: STA130H1 is restricted to first-year students. If you are unable to complete STA130H1 in first year, see Note A below for accepted substitutions for this requirement.\n" +
				"To be completed before the end of Second Year:\n" +
				"1. CSC108H1/ CSC120H1/ CSC148H1. A student who has completed CSC110Y1 also fulfills this program requirement.\n" +
				"Higher Years:\n" +
				"1. A set of mandatory courses (3.5 credits): ACT348H1, ACT349H1, ACT351H1, ACT352H1, ACT370H1, ACT455H1/ ACT466H1, STA302H1\n" +
				"2. 2.0 credits to be selected from lists 1 and 2 (students can only use at a maximum 1.0 credit from list (2), the practice oriented courses, to fulfill program completion requirements):\n" +
				"   (1) ACT350H1, ACT371H1, ACT455H1/ ACT466H1 (whichever course that was not taken as a mandatory course), ACT460H1, STA314H1, STA457H1, STA414H1, and\n" +
				"   (2) ACT372H1, ACT470H1, ACT473H1, ACT475H1.\n" +
				"NOTES:\n" +
				"A. If you are unable to complete STA130H1 in your first year of study, this requirement must be fulfilled with one of the following 0.5 credits",
			`prose "" "" (13.0 credits)
heading "" "First Year" First Year:
item "First Year 1" "First Year" ( MAT130H1/ MAT135H1,
item "First Year 4" "First Year" STA130H1
labelled "Note" "First Year" STA130H1 is restricted
heading "" "To be completed before the end of Second Year" To be completed
item "To be completed before the end of Second Year 1" "To be completed before the end of Second Year" CSC108H1/ CSC120H1/ CSC148H1.
heading "" "Higher Years" Higher Years:
item "Higher Years 1" "Higher Years" A set of
item "Higher Years 2" "Higher Years" 2.0 credits to
  item "Higher Years 2.1" "Higher Years" ACT350H1, ACT371H1, ACT455H1/
  item "Higher Years 2.2" "Higher Years" ACT372H1, ACT470H1, ACT473H1,
heading "" "NOTES" NOTES:
item "A" "NOTES" If you are
`},
		{"ASMAJ1944: credits after the heading's colon; numbered notes",
			"First Year: (2.0 credits)\n" +
				"( MAT130H1/ MAT135H1, MAT136H1)/ ( MAT148H1, MAT149H1)/ MAT137Y1/ ( MAT158H1, MAT159H1)/ MAT157Y1, PHY131H1/ PHY151H1, PHY132H1/ PHY152H1\n" +
				"Third or Fourth Year: (0.5 credit)\n" +
				"1. Ethics and Social Responsibility Requirement: The Physics course JPH441H1 meets this requirement as well as any of the following courses: HPS200H1/ ENV222H1/ ENV333H1/ ESS205H1/ PHL273H1/ VIC172Y1/ AST310H1. See Note 2.\n" +
				"Notes:\n" +
				"1. Students in the Physics Major program who are intending to pursue graduate studies in Physics should consult with the Associate Chair (Undergraduate Studies).\n" +
				"3. At most a 0.5 credit from PHY407H1 and PHY408H1 can be used to satisfy Physics Major program requirements.",
			`heading "" "First Year" First Year: (2.0
prose "" "First Year" ( MAT130H1/ MAT135H1,
heading "" "Third or Fourth Year" Third or Fourth
item "Third or Fourth Year 1" "Third or Fourth Year" Ethics and Social
heading "" "Notes" Notes:
item "Notes 1" "Notes" Students in the
item "Notes 3" "Notes" At most a
`},
		{"ASSPE2737: year-labelled lines, bold headings with credits",
			"**Core Biological Physics Courses (12.0 credits):**\n" +
				"First Year (3.0 credits): ( CHM135H1, CHM136H1)/ CHM151Y1, ( MAT130H1/ MAT135H1, MAT136H1)/ ( MAT148H1, MAT149H1)/ MAT137Y1\n" +
				"First or Second Year (1.0 credit): BIO130H1, MAT223H1\n" +
				"Third or Fourth Year (2.5 credits):\n" +
				"1. PHY331H1, PHY431H1\n" +
				"2. PHY324H1/ BCH370H1/ CHM327H1/ PSL372H1\n" +
				"**Additional Courses for the Biochemistry Stream (2.5 credits):**\n" +
				"1. BCH311H1, BCH340H1\n" +
				"_Integrative, Inquiry-Based Activity Requirement_\n" +
				"The choices in the program must satisfy the requirement for an integrative, inquiry-based activity by including at least one of the following courses:\n" +
				"**Notes:**\n" +
				"On approval of the Department of Physics, students who take CHM222H1 will not have to take PHY252H1.",
			`heading "" "Core Biological Physics Courses" Core Biological Physics
labelled "First Year (3.0 credits)" "First Year" ( CHM135H1, CHM136H1)/
labelled "First or Second Year (1.0 credit)" "First or Second Year" BIO130H1, MAT223H1
heading "" "Third or Fourth Year" < "Core Biological Physics Courses" Third or Fourth
item "Third or Fourth Year 1" "Third or Fourth Year" PHY331H1, PHY431H1
item "Third or Fourth Year 2" "Third or Fourth Year" PHY324H1/ BCH370H1/ CHM327H1/
heading "" "Additional Courses for the Biochemistry Stream" Additional Courses for
item "Additional Courses for the Biochemistry Stream 1" "Additional Courses for the Biochemistry Stream" BCH311H1, BCH340H1
heading "" "Integrative, Inquiry-Based Activity Requirement" < "Additional Courses for the Biochemistry Stream" Integrative, Inquiry-Based Activity
prose "" "Integrative, Inquiry-Based Activity Requirement" The choices in
heading "" "Notes" Notes:
prose "" "Notes" On approval of
`},
		{"ASMAJ0463: a year-labelled line is a section of its own; headings inside an item",
			"(6.0 credits, including at least 2.0 credits at the 300+ level, of which a 0.5 credit must be at the 400-level.)\n" +
				"First year: No specific first-year requirements\n" +
				"1. CHC203Y1\n" +
				"3. 3.0 credits from any of the courses below, of which 2.0 credits must have the CHC/JCA/JCR designator.\n" +
				"   Christianity and Society:\n" +
				"   CHC120H1/ CHC130H1/ CHC215H1/ CHC303H1\n" +
				"   Christianity and the Arts:\n" +
				"   CHC200H1/ CHC305H1/ CHC364H1/ CHC365H1\n" +
				"4. 0.5 credit from CHC232H1/ CHC370H1/ CHC371H1/ CHC383H1/ JCA302H1/ SMC385H1",
			`prose "" "" (6.0 credits, including
labelled "First year" "First year" No specific first-year
item "1" "" CHC203Y1
item "3" "" 3.0 credits from
  heading "" "Christianity and Society" Christianity and Society:
  prose "" "Christianity and Society" CHC120H1/ CHC130H1/ CHC215H1/
  heading "" "Christianity and the Arts" Christianity and the
  prose "" "Christianity and the Arts" CHC200H1/ CHC305H1/ CHC364H1/
item "4" "" 0.5 credit from
`},
		{"ASMAJ0151, ASMAJ1305A: labelled lines inside items",
			"4. 2.0 credits from 300+ level Department for the Study of Religion courses, at least 0.5 credit of which must be in the same tradition as was chosen from #3 above.\n" +
				"   **Indigenous Spiritualities and Religions:** RLG331H1\n" +
				"   **Judaism:** JRN301H1, RLG313H1, RLG320H1, RLG321H1\n" +
				"5. 2.0 credits chosen from other Department for the Study of Religion courses carrying any of the following designators: RLG, JAR, JCR, JRC, JNR, JRN, JPR, JSR\n" +
				"6. 0.5 credit from Core Geography Courses: JEG100H1, ESS262H1, GGR101H1, GGR107H1\n" +
				"   _Note:_ A 200 or 300-level Geography & Planning course, including joint courses, may also be used to meet this requirement.",
			`item "4" "" 2.0 credits from
  labelled "Indigenous Spiritualities and Religions" "" RLG331H1
  labelled "Judaism" "" JRN301H1, RLG313H1, RLG320H1,
item "5" "" 2.0 credits chosen
item "6" "" 0.5 credit from
  labelled "Note" "" A 200 or
`},
		{"ASFOC1687A, ASMAJ1423: nested items take the item's label; a labelled nested bullet",
			"3. 0.5 credits from CSC301H1, CSC302H1, CSC309H1, CSC318H1, and CSC343H1\n" +
				"4. 1.0 credits including:\n" +
				"  a. Up to 0.5 credits from CSC301H1, CSC302H1, CSC309H1, CSC318H1, and CSC343H1 (if not taken in list 3)\n" +
				"  b. Up to 1.0 credits from CSC404H1, CSC413H1, CSC417H1, CSC419H1\n" +
				"5. 0.5 credit from one of the following subject areas:\n" +
				"  - Computer Science: CSC336H1, CSC385H1, CSC413H1, CSC456H1;\n" +
				"  - Astronomy: AST430H1, or AST425Y1 (0.5 credit from point 1 above).",
			`item "3" "" 0.5 credits from
item "4" "" 1.0 credits including:
  item "4a" "" Up to 0.5
  item "4b" "" Up to 1.0
item "5" "" 0.5 credit from
  labelled "Computer Science" "" CSC336H1, CSC385H1, CSC413H1,
  labelled "Astronomy" "" AST430H1, or AST425Y1
`},
		{"ASFOC2015A: items nested under a total line",
			"3.0 credits\n" +
				"  1. POL222H1 (or one of its exclusions: ECO220Y1/ PSY201H1/ SOC202H1/ STA220H1; only 0.5 credit of ECO220Y1 can count toward this Focus)\n" +
				"  2. POL232H1 (or its exclusion: GGR270H1)",
			`prose "" "" 3.0 credits
  item "1" "" POL222H1 (or one
  item "2" "" POL232H1 (or its
`},
		{"ASMAJ0525: an N.B. heading; roman items",
			"5) 0.5 additional credit of FRE at the 200+ level or FSL at the 300+ level.\n" +
				"N.B.\n" +
				"i) Students may also satisfy requirements (4) and (5) through completion of a senior essay ( FRE490Y1).\n" +
				"ii) Students not placed in FSL220H1 or higher by the Placement test must first complete FSL122H1 before starting this program.\n" +
				"Regardless of exemptions, a maximum of 1.0 total program credits may be drawn from courses taught in English.",
			`item "5" "" 0.5 additional credit
heading "" "N.B." N.B.
item "i" "N.B." Students may also
item "ii" "N.B." Students not placed
prose "" "N.B." Regardless of exemptions,
`},
		{"ASMAJ1053: numbering carried on under bold headings, restarted under a notes heading; emphasised numbers; a footnote",
			"**Chemical and Physical Foundations of Biological Systems**\n" +
				"1. ( CHM135H1 and CHM136H1)/ CHM151Y1\n" +
				"9. 0.5 credit from: HMB360H1/ HMB421H1/ HMB435H1\n" +
				"**Courses in Applied Genetics**\n" +
				"10. HMB201H1\n" +
				"14. 0.5 credit from: HMB311H1/ HMB314H1/ MGY315H1/ HMB342H1/ HMB360H1/ HMB490Y1/ HMB491Y1/ HMB496Y1*/ HST373H1\n" +
				"_* A research project from a different unit may be accepted with prior written approval from Human Biology._\n" +
				"_Applied Genetics and Biotechnology Major Notes:_\n" +
				"_1. Courses can only count toward one requirement, even if listed as options to multiple requisites of the program._\n" +
				"3. _The Applied Genetics and Biotechnology major cannot be paired with any other Human Biology Program managed major program._",
			`heading "" "Chemical and Physical Foundations of Biological Systems" Chemical and Physical
item "1" "Chemical and Physical Foundations of Biological Systems" ( CHM135H1 and
item "9" "Chemical and Physical Foundations of Biological Systems" 0.5 credit from:
heading "" "Courses in Applied Genetics" Courses in Applied
item "10" "Courses in Applied Genetics" HMB201H1
item "14" "Courses in Applied Genetics" 0.5 credit from:
prose "" "Courses in Applied Genetics" * A research
heading "" "Applied Genetics and Biotechnology Major Notes" Applied Genetics and
item "Applied Genetics and Biotechnology Major Notes 1" "Applied Genetics and Biotechnology Major Notes" Courses can only
item "Applied Genetics and Biotechnology Major Notes 3" "Applied Genetics and Biotechnology Major Notes" The Applied Genetics
`},
		{"ASMAJ1013: a bold notes heading restarts the numbering",
			"(7.0 credits in Sociology)\n" +
				"1. SOC100H1 and SOC150H1\n" +
				"6. An additional 2.0 credits in Sociology at the 200 or 300-level (0.5 of which must be taken at the 300-level, i.e., 1.5 credits at the 200 or 300-level and 0.5 at the 300-level).\n" +
				"**NOTES:**\n" +
				"**1. Students cannot take more than a 0.5 credit at the 400-level without written permission from the Undergraduate Program Administrator.**",
			`prose "" "" (7.0 credits in
item "1" "" SOC100H1 and SOC150H1
item "6" "" An additional 2.0
heading "" "NOTES" NOTES:
item "NOTES 1" "NOTES" Students cannot take
`},
		{"ASFOC1689A: the first list, under Required Courses, keeps its numbers",
			"Required Courses:\n" +
				"1. ( MAT235H1, MAT236H1)/ MAT235Y1/ MAT237Y1/ MAT257Y1\n" +
				"2. 1.5 credits from the following: CSC336H1, CSC436H1, CSC446H1, CSC456H1, CSC466H1\n" +
				"Suggested Related Courses:\n" +
				"- CSC367H1",
			`heading "" "Required Courses" Required Courses:
item "1" "Required Courses" ( MAT235H1, MAT236H1)/
item "2" "Required Courses" 1.5 credits from
heading "" "Suggested Related Courses" Suggested Related Courses:
bullet "" "Suggested Related Courses" CSC367H1
`},
		{"ASSPE0652: lettered items written flush left belong to the numbered item above",
			"**1. Geographic Distribution (200+ level):**\n" +
				"a) 2.0 credits in Africa/Asia/Middle East with at least one 0.5 credit in 2 of the 3 areas.\n" +
				"b) 1.0 credit in Canada/US/Latin America/The Caribbean\n" +
				"c) 1.0 credit in Europe\n" +
				"**2. Temporal Requirement:** 1.0 credit in pre-1800 history course(s) listed below.\n" +
				"_Note 1: HIS courses listed with (.50) indicate that they fulfill half of the Temporal Requirement._\n" +
				"**4.** Additional HIS, JIH, JHA, JHM, JHN, JMH, JSH or Equivalent courses (listed below) to a total of 10.0 credits.\n" +
				"**Geographic Area a): Africa, Asia, Middle East**\n" +
				"_(Sub-group: Africa)_\n" +
				"AFR290H1, AFR370H1, HIS295Y1",
			`item "1" "" Geographic Distribution (200+
  item "1a" "" 2.0 credits in
  item "1b" "" 1.0 credit in
  item "1c" "" 1.0 credit in
item "2" "" Temporal Requirement: 1.0
labelled "Note 1" "" HIS courses listed
item "4" "" Additional HIS, JIH,
heading "" "Geographic Area a): Africa, Asia, Middle East" Geographic Area a):
heading "" "(Sub-group: Africa)" < "Geographic Area a): Africa, Asia, Middle East" (Sub-group: Africa)
prose "" "(Sub-group: Africa)" AFR290H1, AFR370H1, HIS295Y1
`},
		{"ASMAJ2030: a lettered list under a lead-in",
			"_Applications_: Any 4 courses (2.0 credits) from:\n" +
				"a. Hydrology/GIS/Remote Sensing: GGR273H1, GGR274H1, GGR315H1/ GGR337H1\n" +
				"b. Climatology/Energy/Resources: GGR314H1, GGR334H1, GGR347H1, GGR348H1, GGR416H1\n" +
				"   Up to 1.0 credit can be used from GGR493Y1, based on suitability of placement for this program, and approved by department.\n" +
				"**Total credits: 8.0**",
			`labelled "Applications" "" Any 4 courses
  item "a" "" Hydrology/GIS/Remote Sensing: GGR273H1,
  item "b" "" Climatology/Energy/Resources: GGR314H1, GGR334H1,
    prose "" "" Up to 1.0
labelled "Total credits" "" 8.0
`},
		{"ASMAJ0382: a restarted numbering carried on past sub-headings keeps its prefix",
			"(7.0 credits)\n" +
				"1. CLA160H1/ CLA170H1\n" +
				"4. Additional 5.5 credits in CLA, including 2.0 credits at the 300+-level\n" +
				"Substitutions:\n" +
				"1. Up to 3.0 credits from approved courses involving the ancient Greek and Roman civilizations taught by other departments, notably Art History and Philosophy, may be substituted for the 5.5 credits in CLA courses from Requirement 4.\n" +
				"_Art History:_\n" +
				"FAH206H1, FAH207H1, FAH208H1\n" +
				"2. Up to 2.0 credits in GRK or LAT may be substituted for the 5.5 credits in CLA courses from Requirement 4.",
			`prose "" "" (7.0 credits)
item "1" "" CLA160H1/ CLA170H1
item "4" "" Additional 5.5 credits
heading "" "Substitutions" Substitutions:
item "Substitutions 1" "Substitutions" Up to 3.0
heading "" "Art History" Art History:
prose "" "Art History" FAH206H1, FAH207H1, FAH208H1
item "Substitutions 2" "Art History" Up to 2.0
`},
		{"ASMAJ1021: a sentence in emphasis with a colon in it is prose, not a label",
			"**Admission to the Christianity and Culture: Major Program in Religious Education has been administratively suspended as of 1 April 2015 and is no longer admitting students. Students presently enrolled in the minor will be able to complete the program requirements as described below.**",
			`prose "" "" Admission to the
`},
		{"ASMAJ1160: bold notes, then Group 1 holding Clusters A and B (heading levels)",
			"9. 1.5 credits from Group 1 (any Cluster) and/or Group 2, at any level\n" +
				"**Notes:**\n" +
				"3. These are all individual requirements and cannot overlap or be used twice _within_ the Major.\n" +
				"**Group 1 - Psychology Courses**\n" +
				"**Cluster A - Development, Social, Personality, and Psychopathology and Clinical Science**\n" +
				"JLP315H1, PSY194H1, PSY195H1\n" +
				"**Cluster B - Learning, Perception, Cognition, and the Brain**\n" +
				"JLP374H1, JLP471H1, PSY196H1\n" +
				"**Group 2 - Related Courses**\n" +
				"ANT331H1, ANT442H1, BIO120H1\n" +
				"**Note:** Some Group 2 courses may have enrolment restrictions.",
			`item "9" "" 1.5 credits from
heading "" "Notes" Notes:
item "3" "Notes" These are all
heading "" "Group 1 - Psychology Courses" Group 1 -
heading "" "Cluster A - Development, Social, Personality, and Psychopathology and Clinical Science" < "Group 1 - Psychology Courses" Cluster A -
prose "" "Cluster A - Development, Social, Personality, and Psychopathology and Clinical Science" JLP315H1, PSY194H1, PSY195H1
heading "" "Cluster B - Learning, Perception, Cognition, and the Brain" < "Group 1 - Psychology Courses" Cluster B -
prose "" "Cluster B - Learning, Perception, Cognition, and the Brain" JLP374H1, JLP471H1, PSY196H1
heading "" "Group 2 - Related Courses" Group 2 -
prose "" "Group 2 - Related Courses" ANT331H1, ANT442H1, BIO120H1
labelled "Note" "Group 2 - Related Courses" Some Group 2
`},
		{"ASMAJ1775: italic Group C holds its subgroups; a plain year heading doesn't hold the groups",
			"Upper Years:\n" +
				"5. 2.5 credits at the 300+ level from either Group A, B or C, or Subgroups C(i) or C(ii)\n" +
				"_Group C: Society, Culture, and Language_\n" +
				"ANT204H1, ANT205H1, ANT207H1\n" +
				"Note: ANT390H1 may be placed in either Group A or B or C depending on the topic which will vary from year to year.\n" +
				"_Subgroup C (i): (Society, Culture and Language - Area)_\n" +
				"ANT455H1, ANT458H1, ANT472H1, AFR298H1, CAR316H1, NMC241H1\n" +
				"_Subgroup C (ii): (Society, Culture and Language – Linguistic)_\n" +
				"ANT253H1, ANT329H1, ANT425H1",
			`heading "" "Upper Years" Upper Years:
item "5" "Upper Years" 2.5 credits at
heading "" "Group C: Society, Culture, and Language" Group C: Society,
prose "" "Group C: Society, Culture, and Language" ANT204H1, ANT205H1, ANT207H1
labelled "Note" "Group C: Society, Culture, and Language" ANT390H1 may be
heading "" "Subgroup C (i): (Society, Culture and Language - Area)" < "Group C: Society, Culture, and Language" Subgroup C (i):
prose "" "Subgroup C (i): (Society, Culture and Language - Area)" ANT455H1, ANT458H1, ANT472H1,
heading "" "Subgroup C (ii): (Society, Culture and Language – Linguistic)" < "Group C: Society, Culture, and Language" Subgroup C (ii):
prose "" "Subgroup C (ii): (Society, Culture and Language – Linguistic)" ANT253H1, ANT329H1, ANT425H1
`},
		{"ASMAJ0826: a labelled list ends the notes before it",
			"5. Additional courses (excluding those already counted) from Groups A and C for a total of 8.0 credits\n" +
				"Note:\n" +
				"1. Students may use a maximum of 1.0 credit from Independent Study courses and 1.0 credit from Research Participation courses.\n" +
				"Students in this program have the option to complete the Arts & Science Internship Program (ASIP) stream.\n" +
				"**Group A:** CRI300H1/ CRI322H1/ CRI335H1\n" +
				"**Group B:** PSY201H1/ PSY202H1/ SOC202H1/ CRI350H1/ POL222H1",
			`item "5" "" Additional courses (excluding
heading "" "Note" Note:
item "1" "Note" Students may use
prose "" "Note" Students in this
labelled "Group A" "" CRI300H1/ CRI322H1/ CRI335H1
labelled "Group B" "" PSY201H1/ PSY202H1/ SOC202H1/
`},
		{"ASSPE2270: a labelled bullet stays in the notes, a focus line ends them; focus headings under Disciplinary Focuses",
			"Notes:\n" +
				"- Recommended: introductory course in disciplinary focus. MAT223H1/ MAT224H1/ MAT240H1 is also strongly recommended.\n" +
				"**Disciplinary Focuses**\n" +
				"Evolutionary Biology (ASFOC2270M): (3.5 credits) BIO120H1, BIO130H1, BIO220H1\n" +
				"Notes:\n" +
				"- BIO260H1 requires BIO230H1 as a prerequisite.\n" +
				"Economics (ASFOC2270N): (3.5 credits) ( ECO101H1, ECO102H1), ECO200Y1/ ECO206Y1, ECO202Y1/ ECO208Y1\n" +
				"Biochemistry (ASFOC2270O): (3.0 credits)\n" +
				"CHM135H1, CHM136H1, BCH210H1, BCH311H1, BCH370H1, BCH441H1",
			`heading "" "Notes" Notes:
labelled "Recommended" "Notes" introductory course in
heading "" "Disciplinary Focuses" Disciplinary Focuses
labelled "Evolutionary Biology (ASFOC2270M)" "Disciplinary Focuses" (3.5 credits) BIO120H1,
heading "" "Notes" < "Disciplinary Focuses" Notes:
bullet "" "Notes" BIO260H1 requires BIO230H1
labelled "Economics (ASFOC2270N)" "Disciplinary Focuses" (3.5 credits) (
heading "" "Biochemistry (ASFOC2270O)" < "Disciplinary Focuses" Biochemistry (ASFOC2270O): (3.0
prose "" "Biochemistry (ASFOC2270O)" CHM135H1, CHM136H1, BCH210H1,
`},
		{"ASMAJ0135: year headings under a recommended sequence; a bold cluster ends it; asterisks off labels",
			"7. At least 1.5 credits of the student’s program must be in American Studies (AMS prefix courses), at the 300- or 400-level.\n" +
				"**Recommended Sequence of Courses:**\n" +
				"Second Year:\n" +
				"- AMS200H1 and AMS201H1\n" +
				"*NOTE 1: Other 300+ level courses with 50% or more American content may be allowed; students should seek early approval.\n" +
				"**Disciplinary/Thematic Cluster 1: Politics and Economics**\n" +
				"ECO306H1/ POL315H1/ POL326H1",
			`item "7" "" At least 1.5
heading "" "Recommended Sequence of Courses" Recommended Sequence of
heading "" "Second Year" < "Recommended Sequence of Courses" Second Year:
bullet "" "Second Year" AMS200H1 and AMS201H1
labelled "NOTE 1" "Second Year" Other 300+ level
heading "" "Disciplinary/Thematic Cluster 1: Politics and Economics" Disciplinary/Thematic Cluster 1:
prose "" "Disciplinary/Thematic Cluster 1: Politics and Economics" ECO306H1/ POL315H1/ POL326H1
`},
		{"ASMAJ1533: a footnote asterisk is dropped from the section",
			"First Year*:\n" +
				"1. 0.5 credit from the following set of 100-level courses: GGR112H1/ GGR124H1/ POL101H1/ POL106H1/ POL107H1/ POL109H1/ PSY100H1/ SOC100H1/ SOC150H1",
			`heading "" "First Year" First Year*:
item "First Year 1" "First Year" 0.5 credit from
`},
		{"ASSPE1253, ASSPE1650: labels that start with a digit; 1st/2nd year labels name a year",
			"1st year required courses (2.0 credits): CHM151Y1/ ( CHM135H1, CHM136H1), MAT130H1/ MAT135H1, PHY131H1/ PHY151H1\n" +
				"3rd and 4th year elective courses: 4.0 credits from JGA305H1, ESS234H1, ESS313H1\n" +
				"10.0 credits of core courses:\n" +
				"200-level physics, math, and earth sciences: PHY250H1, PHY254H1, MAT224H1\n" +
				"0.5 to 1.0 credit ethics course chosen from the following list: JPH441H1, ENV333H1, PHL273H1, PHL275H1, INS201Y1",
			`labelled "1st year required courses (2.0 credits)" "1st year" CHM151Y1/ ( CHM135H1,
labelled "3rd and 4th year elective courses" "3rd and 4th year" 4.0 credits from
prose "" "" 10.0 credits of
labelled "200-level physics, math, and earth sciences" "" PHY250H1, PHY254H1, MAT224H1
prose "" "" 0.5 to 1.0
`},
		{"ASSPE2665, ASMAJ1019: an item ending in a colon holds the lines after it",
			"2. 4.0 credits in one or two primary source languages (with NML designator) from among the following languages: Akkadian, Arabic.\n" +
				"3. Higher years:\n" +
				"3.0 credits from NMC201H1, NMC238H1, NMC239H1\n" +
				"2.0 credits from NMC231H1, NMC249H1, NMC250H1\n" +
				"4. 1.0 credit in Medieval or Modern stream courses or those from a list of pre-approved courses offered by other departments.\n" +
				"2. Higher years:\n" +
				"3.0 to 4.0 credits from any NMC courses\n" +
				"1.0 to 2.0 credits in language (NML courses)",
			`item "2" "" 4.0 credits in
item "3" "" Higher years:
  prose "" "" 3.0 credits from
  prose "" "" 2.0 credits from
item "4" "" 1.0 credit in
item "2" "" Higher years:
  prose "" "" 3.0 to 4.0
  prose "" "" 1.0 to 2.0
`},
		{"ASMAJ1205: language lists belong to their item; labels counted outside brackets",
			"2. 2.0 credits of language coursework in one language area chosen from the list below:\n" +
				"_Czech and Slovak:_ SLA105Y1, SLA204Y1, SLA214H1\n" +
				"_Yiddish (corresponds to the Jewish Cultures in Eastern Europe area_): GER261H1, GER262H1, GER360H1, GER460H1\n" +
				"Note: In consultation with the Undergraduate Coordinator, and based on results in language placement tests, some students may substitute language courses.\n" +
				"3. 2.0 credits from culture courses corresponding to the student’s chosen language area:\n" +
				"_Czech and Slovak:_ SLA200H1, SLA215H1, SLA225H1\n" +
				"_Jewish Cultures in Eastern Europe (corresponds to the Yiddish language area):_ GER361H1; JJH370H1; SLA202H1\n" +
				"4. 1.0 credit from the pool of comparative courses offered by the Department: EST205H1, EST206H1, EST207H1",
			`item "2" "" 2.0 credits of
  labelled "Czech and Slovak" "" SLA105Y1, SLA204Y1, SLA214H1
  labelled "Yiddish (corresponds to the Jewish Cultures in Eastern Europe area)" "" GER261H1, GER262H1, GER360H1,
  labelled "Note" "" In consultation with
item "3" "" 2.0 credits from
  labelled "Czech and Slovak" "" SLA200H1, SLA215H1, SLA225H1
  labelled "Jewish Cultures in Eastern Europe (corresponds to the Yiddish language area)" "" GER361H1; JJH370H1; SLA202H1
item "4" "" 1.0 credit from
`},
		{"ASMIN1337: \"&\" isn't counted as a word in a label",
			"History & Philosophy of Science & Technology: HPS202H1/ HPS203H1",
			`labelled "History & Philosophy of Science & Technology" "" HPS202H1/ HPS203H1
`},
		{"ASMAJ2745: lettered categories with their lists under the item, after its first sentence",
			"3. 2.5 credits of elective courses:\n" +
				"**In addition to the above core courses, you need to take 2.5 credits of electives. This requirement can be satisfied by any of the Earth Sciences Courses listed in categories A to F below.** The following clusters of courses are neither mutually exclusive nor meant to limit choice.\n" +
				"a) Earth Surface Processes\n" +
				"ESS224H1, ESS241H1, ESS245H1/ ESS345H1\n" +
				"b) Paleoclimate\n" +
				"ESS245H1/ ESS345H1, ESS261H1, ESS331H1\n" +
				"Students in this program have the option to complete the Arts & Science Internship Program (ASIP) stream.",
			`item "3" "" 2.5 credits of
  prose "" "" In addition to
  item "3a" "" Earth Surface Processes
    prose "" "" ESS224H1, ESS241H1, ESS245H1/
  item "3b" "" Paleoclimate
    prose "" "" ESS245H1/ ESS345H1, ESS261H1,
prose "" "" Students in this
`},
		{"ASMAJ1240, ASSPE1388: an item pointing at the clusters takes them; a list line after \"from list 2:\"",
			"3. Thematic Clusters (1.5 credits): 0.5 credit from each of the three Thematic Clusters.\n" +
				"- Politics & Policy\n" +
				"  JSU237H1, SDS345H1, SDS365H1\n" +
				"- Media & Aesthetics\n" +
				"  CIN336H1, ENG273Y1, JSU325H1\n" +
				"4. Capstone requirement (a minimum of one 0.5 credit from the following list):\n" +
				"All 400-level SDS and JNS courses.\n" +
				"3. Up to 1.0 credit may be taken from list 2:\n" +
				"MGY440H1, MGY480Y1/ one or both of ( MGY481H1 and MGY482H1), BCH428H1\n" +
				"Students in this program have the option to complete the Arts & Science Internship Program (ASIP) stream.",
			`item "3" "" Thematic Clusters (1.5
  bullet "" "" Politics & Policy
    prose "" "" JSU237H1, SDS345H1, SDS365H1
  bullet "" "" Media & Aesthetics
    prose "" "" CIN336H1, ENG273Y1, JSU325H1
item "4" "" Capstone requirement (a
  prose "" "" All 400-level SDS
item "3" "" Up to 1.0
  prose "" "" MGY440H1, MGY480Y1/ one
prose "" "" Students in this
`},
		{"ASMAJ1707: a marker glued to an amount",
			"Early Years:\n" +
				"1.1.0 credit from AFR150Y1/ AFR199H1/ AFR250Y1/ AFR251H1/ AFR260H1/ AFR270H1/ AFR298H1/ AFR299H1/ AFR299Y1\n" +
				"Higher Years:\n" +
				"2. 0.5 credit from research design courses: JQR360H1/ AFR260H1/ AFR454H1/ AFR460H1/ JAH350H1",
			`heading "" "Early Years" Early Years:
item "1" "Early Years" 1.0 credit from
heading "" "Higher Years" Higher Years:
item "2" "Higher Years" 0.5 credit from
`},
		{"ASMAJ1707: notes close the group list before them",
			"**Group B (Courses with African Studies Content):**\n" +
				"ANT204H1, ANT205H1, ANT208H1\n" +
				"Students in this program have the option to complete the Arts & Science Internship Program (ASIP) stream\n" +
				"Notes:\n" +
				"- At least 2.0 credits from Groups A and/or B must be at the 300/400-level",
			`heading "" "Group B (Courses with African Studies Content)" Group B (Courses
prose "" "Group B (Courses with African Studies Content)" ANT204H1, ANT205H1, ANT208H1
prose "" "Group B (Courses with African Studies Content)" Students in this
heading "" "Notes" Notes:
bullet "" "Notes" At least 2.0
`},
		{"ASMAJ1231: a heading with nothing under it holds the headings after it",
			"_Course Groups_\n" +
				"_Medievalism:_\n" +
				"MST110H1/ MST211H1/ MST230H1\n" +
				"_History:_\n" +
				"MST231H1/ MST232H1/ MST233H1\n" +
				"Note:\n" +
				"- MST300H1 is offered through the Centre for Medieval Studies.",
			`heading "" "Course Groups" Course Groups
heading "" "Medievalism" < "Course Groups" Medievalism:
prose "" "Medievalism" MST110H1/ MST211H1/ MST230H1
heading "" "History" < "Course Groups" History:
prose "" "History" MST231H1/ MST232H1/ MST233H1
heading "" "Note" Note:
bullet "" "Note" MST300H1 is offered
`},
		{"ASSPE0509: an empty recommended heading holds bold headings and their year lines",
			"Recommended Course Combinations:\n" +
				"**Students interested in Mineral Exploration:**\n" +
				"First Year: BIO120H1, CHM135H1, CHM136H1, MAT130H1/ MAT135H1, MAT136H1, PHY131H1, PHY132H1\n" +
				"**Students with an interest in GIS and Geology:**\n" +
				"First Year: CHM135H1, CHM136H1, MAT130H1/ MAT135H1, PHY131H1, PHY132H1, CSC108H1",
			`heading "" "Recommended Course Combinations" Recommended Course Combinations:
heading "" "Students interested in Mineral Exploration" < "Recommended Course Combinations" Students interested in
labelled "First Year" "First Year" BIO120H1, CHM135H1, CHM136H1,
heading "" "Students with an interest in GIS and Geology" < "Recommended Course Combinations" Students with an
labelled "First Year" "First Year" CHM135H1, CHM136H1, MAT130H1/
`},
		{"ASSPE1868: an empty year heading holds the heading after it until the next year heading",
			"First or second year:\n" +
				"Foundational courses (5.0 credits total)\n" +
				"BIO130H1\n" +
				"Upper years:\n" +
				"Program requirements (7.5 credits total)\n" +
				"CSC207H1",
			`heading "" "First or second year" First or second
heading "" "Foundational courses" < "First or second year" Foundational courses (5.0
prose "" "Foundational courses" BIO130H1
heading "" "Upper years" Upper years:
heading "" "Program requirements" < "Upper years" Program requirements (7.5
prose "" "Program requirements" CSC207H1
`},
		{"ASMAJ1533: a footnote after an item isn't part of it",
			"6. 2.0 credits of electives chosen from the following courses below, of which at least 1.0 credit must be at the 300- or 400-level and no more than 0.5 credit can be from IRE courses:\n" +
				"   GGR221H1/ GGR223H1/ GGR251H1\n" +
				"*Students admitted to the program with ECO101H1/ ECO105Y1/ IRW230H1: The course will be applied to Requirement #3 (Higher Years). Students are advised to discuss potential course substitutions for First Year with the Program Office.\n" +
				"Students in this program have the option to complete the Arts & Science Internship Program (ASIP) stream.",
			`item "6" "" 2.0 credits of
  prose "" "" GGR221H1/ GGR223H1/ GGR251H1
prose "" "" *Students admitted to
prose "" "" Students in this
`},
		{"ASMAJ1295: a dash in a label isn't a word",
			"3) 1.5 credits from the following three groups (students are strongly encouraged to include 0.5 credit from each group):\n" +
				"Group 1 – Literary History: FRE318H1/ FRE319H1/ FRE320H1/ FRE324H1/ FRE326H1/\n" +
				"Group 3 – Quebec and Francophone Literatures: FRE314H1/ FRE332H1/ FRE334H1/ FRE336H1",
			`item "3" "" 1.5 credits from
  labelled "Group 1 – Literary History" "" FRE318H1/ FRE319H1/ FRE320H1/
  labelled "Group 3 – Quebec and Francophone Literatures" "" FRE314H1/ FRE332H1/ FRE334H1/
`},
		{"ASSPE0151: advice bullets end the list a lead-in bullet takes",
			"Notes:\n" +
				"- 2.0 credits of related courses offered by other academic units may be counted towards the fulfillment of the program. The list of eligible courses includes:\n" +
				"  - ANT356H1, ANT386H1, ANT426H1\n" +
				"- It is highly recommended that Religion Specialists who are interested in pursuing graduate training also complete two full years of a relevant foreign language.\n" +
				"- Please note that UTM and UTSC courses do not automatically count towards completion of a RLG program (except the Buddhist Studies Major and Specialist).",
			`heading "" "Notes" Notes:
bullet "" "Notes" 2.0 credits of
  bullet "" "Notes" ANT356H1, ANT386H1, ANT426H1
bullet "" "Notes" It is highly
bullet "" "Notes" Please note that
`},
		{"ASMAJ0365: a year label with a title and no courses is a year heading",
			"Year 1:\n" +
				"3. JEG100H1/ GGR101H1/ MAT130H1/ MAT135H1/ PHY131H1/ PSY100H1\n" +
				"Year 2: Foundations in environment and health\n" +
				"4. BCH210H1/ CHM247H1\n" +
				"7. 1.0 credit from environmental core courses: ENV221H1/ ENV222H1/ ( EEB240H1/ ENV234H1)/ ENV337H1",
			`heading "" "Year 1" Year 1:
item "3" "Year 1" JEG100H1/ GGR101H1/ MAT130H1/
heading "" "Year 2" Year 2: Foundations
item "4" "Year 2" BCH210H1/ CHM247H1
item "7" "Year 2" 1.0 credit from
`},
		{"ASMAJ1252, ASMAJ0908, ASMAJ0571: a total line; group headings with and without emphasis",
			"_Applications:_ Any 5 courses (2.5 credits) from Group A; up to 1.0 credit can be from Group B; up to 0.5 credit can be from Group C.\n" +
				"Note: At least 2.0 credits must be 300/400-level courses. Of these, at least one 0.5 credit must be at the 400-level (includes Fourth Year Course requirement).\n" +
				"**Total credits: 7.0**\n" +
				"Group A (Environmental Geography Courses)\n" +
				"GGR201H1/ JEG201H1, GGR203H1, GGR205H1/ JEG205H1\n" +
				"Group C\n" +
				"Any other GGR, JGE, JIG, JUG courses at the 200/300/400-level not listed in Group A.\n" +
				"_Group A: Ancient, Medieval_\n" +
				"FAH205H1, FAH206H1, FAH207H1\n" +
				"**Group A**\n" +
				"AFR454H1, ANT343H1, ANT456H1",
			`labelled "Applications" "" Any 5 courses
labelled "Note" "" At least 2.0
labelled "Total credits" "" 7.0
heading "" "Group A (Environmental Geography Courses)" Group A (Environmental
prose "" "Group A (Environmental Geography Courses)" GGR201H1/ JEG201H1, GGR203H1,
heading "" "Group C" Group C
prose "" "Group C" Any other GGR,
heading "" "Group A: Ancient, Medieval" Group A: Ancient,
prose "" "Group A: Ancient, Medieval" FAH205H1, FAH206H1, FAH207H1
heading "" "Group A" Group A
prose "" "Group A" AFR454H1, ANT343H1, ANT456H1
`},
		{"ASSPE0605, ASMAJ1376, ASMAJ1469: lead-ins are prose and take the bullets after them; year lines with and without a colon",
			"Second Year: BCH210H1, BIO230H1/ BIO255H1, ENV221H1, ENV222H1, PCL201H1, PSL300H1, PSL301H1 and CHM247H1 (see NOTE 1)\n" +
				"Third and Fourth Years:\n" +
				"BIO220H1, EEB240H1, JGE321H1, CHM210H1, PCL302H1, PCL381H1, PCL382H1, PCL482H1\n" +
				"One of: ENV341H1/ ENV337H1/ EEB320H1\n" +
				"At least two of the following four options:\n" +
				"- CHM217H1\n" +
				"Year 4 (0.5 credit)\n" +
				"TRN409H1/ TRN410H1",
			`labelled "Second Year" "Second Year" BCH210H1, BIO230H1/ BIO255H1,
heading "" "Third and Fourth Years" Third and Fourth
prose "" "Third and Fourth Years" BIO220H1, EEB240H1, JGE321H1,
prose "" "Third and Fourth Years" One of: ENV341H1/
prose "" "Third and Fourth Years" At least two
  bullet "" "Third and Fourth Years" CHM217H1
heading "" "Year 4" Year 4 (0.5
prose "" "Year 4" TRN409H1/ TRN410H1
`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bs := completionBlocks(tt.raw)
			if got := blkDump(bs); got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
			lines := strings.Split(tt.raw, "\n")
			var check func(bs []*compBlock)
			check = func(bs []*compBlock) {
				for _, b := range bs {
					if !blkSameLine(b, lines[b.line]) {
						t.Errorf("line %d: block %q doesn't match %q", b.line, blkSource(b), lines[b.line])
					}
					check(b.children)
				}
			}
			check(bs)
		})
	}
}

func TestBlkItemOrdinal(t *testing.T) {
	tests := []struct {
		label              string
		prevStyle, prevOrd int
		style, ord         int
	}{
		{"3", 0, 0, blkStyleNumber, 3},
		{"i", blkStyleLower, 8, blkStyleLower, 9}, // after "h"
		{"i", 0, 0, blkStyleRoman, 1},
		{"v", blkStyleRoman, 4, blkStyleRoman, 5}, // after "iv"
		{"v", blkStyleLower, 21, blkStyleLower, 22},
		{"ii", 0, 0, blkStyleRoman, 2},
		{"xx", 0, 0, blkStyleRoman, 20},
		{"III", 0, 0, blkStyleRoman, 3},
		{"I", blkStyleUpper, 8, blkStyleUpper, 9}, // after "H"
		{"I", 0, 0, blkStyleRoman, 1},
		{"C", 0, 0, blkStyleUpper, 3},
	}
	for _, tt := range tests {
		style, ord := blkItemOrdinal(tt.label, tt.prevStyle, tt.prevOrd)
		if style != tt.style || ord != tt.ord {
			t.Errorf("blkItemOrdinal(%q, %d, %d) = %d, %d; want %d, %d", tt.label, tt.prevStyle, tt.prevOrd, style, ord, tt.style, tt.ord)
		}
	}
}
