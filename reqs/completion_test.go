package reqs

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"degree-explorer/types"
)

// Completion text below is copied from the catalogue (2026-10-15): whole
// programs for the goldens, and snippets of real programs, one per rule.

// jComp builds a completion envelope: its rows, then any other fields
// (`"notes":[...]`), then raw.
func jComp(raw string, rows []string, fields ...string) string {
	s := `{"requirements":[` + strings.Join(rows, ",") + `]`
	for _, f := range fields {
		s += "," + f
	}
	return s + `,"raw":` + jsonString(raw) + `}`
}

// jRow builds a row. parent 0 and an empty label or section are left out,
// as in the stored JSON; rest holds further fields (`"notes":[...]`).
func jRow(number int, kind string, parent int, label, section, req, raw string, rest ...string) string {
	s := fmt.Sprintf(`{"number":%d,"kind":%q`, number, kind)
	if label != "" {
		s += `,"label":` + jsonString(label)
	}
	if section != "" {
		s += `,"section":` + jsonString(section)
	}
	if parent != 0 {
		s += fmt.Sprintf(`,"parent":%d`, parent)
	}
	s += `,"req":` + req
	for _, r := range rest {
		s += "," + r
	}
	return s + `,"raw":` + jsonString(raw) + `}`
}

func jStrs(ss ...string) string { b, _ := json.Marshal(ss); return string(b) }

// complCompact writes c as lines: "N kind pP req" per row (pP for a parent,
// "scope:S" for a scope) with its audience and notes after it, then the named
// lists, program notes and exclusions. It leaves out labels, sections and raws.
func complCompact(c *types.Completion) []string {
	var out []string
	for _, r := range c.Requirements {
		j, _ := json.Marshal(r.Req)
		line := fmt.Sprintf("%d %s", r.Number, r.Kind)
		if r.Parent != 0 {
			line += fmt.Sprintf(" p%d", r.Parent)
		}
		if r.Scope != "" {
			line += " scope:" + string(r.Scope)
		}
		out = append(out, line+" "+string(j))
		if r.Audience != nil {
			a, _ := json.Marshal(r.Audience)
			out = append(out, "  audience: "+string(a))
		}
		for _, n := range r.Notes {
			out = append(out, "  note: "+n)
		}
	}
	for _, l := range c.Lists {
		j, _ := json.Marshal(l.Items)
		name := l.Name
		if l.Title != "" {
			name += " (" + l.Title + ")"
		}
		out = append(out, "list: "+name+": "+string(j))
	}
	for _, n := range c.Notes {
		out = append(out, "note: "+n)
	}
	for _, e := range c.Exclusions {
		j, _ := json.Marshal(e)
		out = append(out, "exclusion: "+string(j))
	}
	return out
}

func TestParseCompletionPrograms(t *testing.T) {
	tests := []struct{ code, raw string }{
		// Year headings give sections and item labels; a bracketed floor and an item's
		// cap are the item's rows, as is the cap on a "/" pair flattened into item 5's
		// pool. First Year 2 pools single courses and pairs, each pair alone the amount.
		{"ASMAJ1003",
			"(8.0 credits)\nFirst Year:\n1. BIO120H1, BIO130H1; ( CHM135H1, CHM136H1)/ CHM151Y1\n2. 1.0 credit from JMB170Y1/ MAT130H1/ MAT135H1/ MAT136H1/ ( MAT148H1, MAT149H1)/ MAT137Y1/ ( MAT158H1, MAT159H1)/ MAT157Y1/ PHY131H1/ PHY132H1/ PHY151H1/ PHY152H1\nHigher Years:\n1. BIO220H1, BIO230H1/ BIO255H1\n2. BIO260H1/ HMB265H1; BCH210H1\n3. CSB349H1\n4. 1.0 credit from: CSB327H1, CSB328H1, CSB329H1, CSB331H1, CSB340H1, CSB353H1\n5. 1.5 credits (at least 0.5 credit at the 400-level) from: BCH422H1, BCH426H1, BCH440H1, BCH444H1, BCH445H1, CJH332H1, CSB299Y1, CSB327H1, CSB328H1, CSB329H1, CSB330H1, CSB331H1, CSB340H1, CSB350H1, CSB351Y1, CSB352H1, CSB353H1, CSB360H1, CSB397Y0, CSB399Y1, CSB427H1, CSB428H1, CSB429H1, CSB430H1, CSB431H1, CSB435H1, CSB447H1, CSB450H1, CSB451H1, CSB452H1, CSB453H1, CSB454H1, CSB457H1, CSB458H1, CSB459H1, CSB460H1, CSB471H1, CSB472H1, CSB473H1, CSB474H1, CSB475H1, CSB483H1, CSB490H1, CSB491H1, CSB492H1, CSB497H1, CSB498Y1, CSB499Y1, HMB496Y1/ HMB499Y1, MGY441H1, MGY480Y1. No more than 0.5 credit in BCH can be used towards this requirement.\nThe Cell & Molecular Biology Major Program has the additional option of a Disciplinary Focus.\nStudents in this program have the option to complete the Arts & Science Internship Program (ASIP) stream."},
		// A header total with its pool and a text floor; notes that are rules (courses
		// that don't count, the campus rule, CR/NCR and transfer caps) and an approval
		// route, which makes the count Review, so the floors carry the marker.
		{"ASCER0338",
			"2.0 PRT credits at the 200+ level taught in Portuguese, including at least 0.5 credit at the 300+ level.\nNotes:\n- Courses with lectures taught in English – PRT250H1 or PRT375H1, for instance – are not included for consideration in the Certificate.\n- Courses taught at UTM or UTSC are not included for consideration.\n- Courses taken in our unit-run Summer Abroad programs will be considered; however, they are not guaranteed to be approved.\n- Courses that have been taken as Credit/No Credit cannot be used to fulfill Certificate requirements.\n- No more than 1.0 transfer credits will be eligible for consideration in the Certificate."},
		// A total with no pool counts what its items claim; a described pool.
		{"ASFOC1478B",
			"(3.0 credits)\n1. 0.5 credit from: CSC108H1, CSC110Y1, CSC148H1\n2. ECO372H1\n3. 1.5 credit of 300+ ECO elective courses from: ECO310H1, ECO334H1, ECO339H1, ECO340H1, ECO353H1, ECO354H1, ECO357H1, ECO367H1, ECO374H1, ECO375H1, ECO380H1, ECO401H1, ECO403H1, ECO404H1, ECO418H1, ECO439H1, ECO446H1, ECO452H1, ECO464H1, ECO466H1, ECO475H1, ECO480H1, ECO481H1, ECO482H1, ECO483H1, ECO484H1, ECO499H1\n4. ECO489H1"},
		// Bullets are items with no label; "1.0 credits:" lists courses that add
		// up to it; a designator pool with exclusions and listed courses.
		{"ASMIN1006",
			"(4.0 credits including at least 1.0 credits at 300+ level)\n- 1.0 credits: BIO120H1; BIO220H1\n- 0.5 credits in organismal biology from: BIO251H1; EEB263H1; EEB266H1; EEB267H1; EEB268H1; EEB340H1; EEB380H1; EEB382H1; EEB384H1; EEB386H1; EEB388H1\n- 0.5 credits in core ecology and evolution from: EEB314H1; EEB318H1; EEB319H1; EEB320H1; EEB321H1; EEB322H1; EEB323H1; EEB324H1; EEB328H1; EEB362H1\n- 2.0 credits from: EEB courses (excluding EEB196H1, EEB197H1, EEB198H1, EEB199H1, EEB202H1, EEB208H1, EEB214H1, EEB215H1); BIO251H1; BIO260H1; EHJ352H1; HMB265H1"},
		// Named lists set out as bullets under the item drawing on them, a group's
		// filter excluding another's codes, floors and caps from the lists, an exception
		// to the total ("may omit CSC108H1"), and a permission route making floors Review.
		{"ASMAJ1689",
			"(8.0 credits, including at least one 0.5 credit at the 400-level)\n_First year_ (2.5 credits):\n1. ( CSC108H1, CSC148H1, CSC165H1/ CSC240H1)/ ( CSC110Y1, CSC111H1); ( MAT148H1/ MAT158H1/ MAT130H1/ MAT135H1, MAT149H1/ MAT159H1/ MAT136H1)/ MAT137Y1/ MAT157Y1\nStudents in this program have the option to enrol in the Arts & Science Internship Program (ASIP) stream.\nNotes:\na. Students with a strong background in an object-oriented language such as Python, Java or C++ may omit CSC108H1 and proceed directly with CSC148H1. [There is no need to replace the missing 0.5 credit for program completion; however, please base your course choice on what you are ready to take, not on “saving” a 0.5 credit].\nb. CSC240H1 is an accelerated and enriched version of CSC165H1 plus CSC236H1, intended for students with a strong mathematical background, or who develop an interest after taking CSC165H1. If you take CSC240H1 without CSC165H1, there is no need to replace the missing half-credit for program completion; but please see Note (a).\nc. Consult the Undergraduate Office for advice about choosing among CSC108H1 and CSC148H1, and between CSC165H1 and CSC240H1.\nd. We recommend that students take MAT148H1/ MAT158H1 and MAT149H1/ MAT159H1, as they provide the best preparation for upper-year courses in computer science and benefit students in CSC165H1/ CSC240H1.\n_Second year_ (2.5 credits):\n2. CSC207H1, CSC236H1/ CSC240H1, CSC258H1, CSC263H1/ CSC265H1, STA247H1/ STA237H1/ STA255H1/ STA257H1\n_Later years_ (3.0 credits):\n3. 3.0 credits of courses in total selected from among the following groups:\n- **Group A:** CSC209H1; any 300-/ 400-level CSC course _except_ those listed in Group B\n- **Group B:** CSC299H1, CSC299Y1, CSC394H1, CSC395H1, CSC396Y0, CSC398H0, CSC398Y0, CSC399H1, CSC399Y1, CSC490H1, CSC491H1, CSC494H1, CSC494Y1, CSC495H1\n- **Group C:** BCB330Y1, BCB410H1, BCB420H1, BCB430Y1; MAT223H1/ MAT240H1, ( MAT235H1, MAT236H1)/ MAT235Y1/ MAT237Y1/ MAT257Y1, any 300-/400-level MAT course except MAT329Y1, MAT390H1, MAT391H1; STA248H1/ STA238H1/ STA261H1, any 300-/400-level STA course; ECE419H1, ECE454H1\nThese 3.0 credits must include:\n- at least 2.0 credits from Groups A and/or B at the 300- /400-level, including at least 0.5 credit from a 400-level CSC course\n- at least 1.0 credit at the 300-/400-level from Group A\n- no more than 1.5 credits from Group B, and no more than 1.0 credit from Group B at the 400-level\n- no more than 0.5 credit from CSC311H1/ STA314H1\n- no more than 0.5 credit from CSC412H1/ STA414H1\n- no more than 0.5 credit from CSC438H1/ MAT309H1\nThe choices in 3 must satisfy the requirement for an integrative, inquiry-based activity by including one of the following courses: CSC301H1, CSC302H1, CSC316H1, CSC318H1, CSC394H1, CSC395H1, CSC396Y0, CSC404H1, CSC413H1, CSC417H1, CSC419H1, CSC428H1, CSC454H1, CSC485H1, CSC490H1, CSC491H1, CSC494H1, CSC495H1, CSC494Y1. Students who complete the Arts & Science Internship Program (ASIP) stream will also meet this requirement.\nStudents enrolled in the Computer Science Major (ASMAJ1689) program may request to count other University of Toronto Faculty of Applied Science and Engineering courses within Group C. Requests must be reviewed and approved by the Computer Science Undergraduate Office and will be evaluated on a case-by-case basis. For information about this process, please contact the Computer Science Undergraduate Office.\nTransfer credits (except for those attained through a University of Toronto exchange program) cannot comprise more than 1.0 credit at the 300-/ 400-level, and cannot be used to satisfy the requirement for an integrative, inquiry-based activity. In addition, transfer credits (except for those attained through a University of Toronto exchange program) cannot be used to satisfy the requirement for 0.5 credit at the 400-level in CSC.\nStudents cannot combine the Computer Science Major (ASMAJ1689) program with the Computer Science Specialist (ASSPE1689) or the Computer Science Minor (ASMIN1689) for their degree."},
		// A hierarchy of lists (Group 1 over Clusters A-C), references to a cluster
		// through its group, an item's courses all on a cluster, and a cap as a note.
		// Item 2's UTM/UTSC equivalents stay permission text until Link.
		{"ASMAJ1160",
			"**(7.0 credits):**\n1. PSY100H1 (**NOTE:** We will also accept PSY100Y5 from UTM, or both of PSYA01H3 **_and_** PSYA02H3 together from UTSC)\n2. PSY201H1 (or one of ECO220Y1/ EEB225H1/ GGR270H1/ IRW220H1/ POL222H1/ SOC202H1/ STA220H1/ STA238H1/ STA248H1/ STA288H1)\n3. 1.0 credit (from Cluster A) PSY210H1, PSY220H1, PSY230H1, PSY240H1\n4. 1.0 credit (from Cluster B) PSY260H1, PSY270H1, PSY280H1, PSY290H1 (**NOTE:** HMB200H1 cannot be used in place of PSY290H1)\n5. 0.5 credit in PSY at the 300/400-level from Group 1 in Cluster A\n6. 0.5 credit in PSY at the 300/400-level from Group 1 in Cluster B\n7. 1.0 credit in PSY at the 300/400-level from Group 1 (any Cluster)\n8. 0.5 credit in PSY at the 400-level from Group 1 (any Cluster)\n9. 1.5 credits from Group 1 (any Cluster) and/or Group 2, at any level\nStudents in this program have the option to complete the Arts & Science Internship Program (ASIP) stream.\n**Notes:**\n1. No more than 2.0 credits from PSY304Y1, PSY306Y0, PSY299H1, PSY299Y1, PSY399H1, PSY399Y1, PSY405H1, PSY406H1, PSY405Y1, and PSY406Y1 can be used for program requirements.\n2. We will accept equivalent/exclusion statistics courses from UTM/UTSC in lieu of PSY201H1. Please contact us to verify if your stats course qualifies.\n3. These are all individual requirements and cannot overlap or be used twice _within_ the Major.\n**Group 1 - Psychology Courses**\n**Cluster A - Development, Social, Personality, and Psychopathology and Clinical Science**\nJLP315H1, PSY194H1, PSY195H1, PSY197H1, PSY210H1, PSY220H1, PSY230H1, PSY240H1, PSY311H1, PSY312H1, PSY313H1, PSY316H1, PSY319H1, PSY320H1, PSY321H1, PSY322H1, PSY323H1, PSY324H1, PSY326H1, PSY328H1, PSY329H1, PSY330H1, PSY331H1, PSY332H1, PSY333H1, PSY336H1, PSY337H1, PSY339H1, PSY341H1, PSY342H1, PSY343H1, PSY349H1, PSY410H1, PSY414H1, PSY417H1, PSY420H1, PSY421H1, PSY422H1, PSY424H1, PSY425H1, PSY426H1, PSY427H1, PSY428H1, PSY430H1, PSY435H1, PSY440H1, PSY450H1, PSY455H1\n**Cluster B - Learning, Perception, Cognition, and the Brain**\nJLP374H1, JLP471H1, PSY196H1, PSY198H1, PSY199H1, PSY260H1, PSY270H1, PSY280H1, PSY290H1, PSY312H1, PSY316H1, PSY359H1, PSY360H1, PSY362H1, PSY369H1, PSY370H1, PSY371H1, PSY372H1, PSY378H1, PSY379H1, PSY380H1, PSY389H1, PSY390H1, PSY395H1, PSY396H1, PSY397H1, PSY450H1, PSY455H1, PSY460H1, PSY470H1, PSY471H1, PSY473H1, PSY475H1, PSY480H1, PSY490H1, PSY492H1, PSY493H1, PSY494H1, PSY495H1, PSY496H1, PSY497H1\n**Cluster C - Methods and Applications**\nPSY201H1, PSY202H1, PSY203H1, PSY204H1, PSY205H1, PSY299H1, PSY299Y1, PSY304Y1, PSY305H1, PSY306Y0, PSY307H1, PSY308H1, PSY399H1, PSY399Y1, PSY400Y1, PSY402H1, PSY403H1, PSY404H1, PSY405H1, PSY405Y1, PSY406H1, PSY406Y1, PSY407H1, PSY408H1, PSY409H1, PSY422H1, PSY455H1\n**Group 2 - Related Courses**\nANT331H1, ANT442H1, BIO120H1, BIO130H1, BIO220H1, BIO270H1, BIO271H1, BPM232H1, BPM330H1, BPM332H1, BPM333H1, BPM335H1, BPM336H1, BPM337H1, BPM338H1, BPM339H1, BPM432H1, BPM433H1, BPM438H1, BPM499H1, CJH332H1, COG250Y1, COG260H1, COG341H1, COG342H1, COG343H1, CRE345H1, CRI365H1, CRI431H1, ECO316H1. ECO326H1, ECO423H1, EDS261H1, EEB313H1, ENG384H1, HIS489H1, HMB200H1, HMB300H1, HMB310H1, HMB320H1, HMB420H1, HMB440H1, HMB460H1, HMB473H1, HPS110H1, HPS120H1, HPS200H1, HPS300H1, HPS347H1, IRE260H1, JFL388H1, JFP450H1, JHA410H1, JLS472H1, JLS473H1, JLS475H1, JLS476H1, LIN101H1, LIN102H1, LIN194H1, LIN195H1, LIN200H1, LIN208H1, NEW302Y1, NEW303H1, NFS489H1, PCL200H1, PCL475H1, PCL476H1, PHL240H1, PHL243H1, PHL244H1, PHL319H1, PHL340H1, PHL342H1, PHL358H1, PHL383H1, PSL201Y1, PSL300H1, PSL440Y1, PSL441H1, PSL442H1, PSL472H1, RLG106H1, RLG211H1, RLG380H1, RLG390H1, RLG417H1, RLG421H1, RSM260H1, RSM353H1, SOC213H1, SOC363H1, SOC412H1, VIC106H1, VIC206H1, WGS372H1, WRR307H1\n**Note:** Some Group 2 courses may have enrolment restrictions. PSY students should not assume priority access to these courses unless they are enrolled in the host program."},
		// Options: Option E includes one of the others, so a choice row picks one,
		// or(A, B, C, D, and(E, any of A-D)), and the total counts the cheapest. "choose
		// four of the following" is choose{4}; the note restating it joins its raw.
		{"ASSPE2025",
			"(14.0-15.5 credits, including at least 2.0 credits at the 400-level)\nFirst Year:\n- BIO120H1, BIO130H1\n- ( CHM135H1, CHM136H1)/ CHM151Y1\n- ( MAT130H1/ MAT135H1, MAT136H1)/ ( MAT148H1, MAT149H1/ MAT137Y1)/ (( MAT158H1, MAT159H1/ MAT157Y1)\nFirst or Second Year:\n- ( PHY131H1, PHY132H1)/ ( PHY151H1, PHY152H1)\nSecond Year:\n- BCH210H1\n- BIO230H1\n- BIO260H1/ HMB265H1\n- CHM220H1\n- CHM247H1/ CHM249H1\n- PSL300H1, PSL301H1\n- STA288H1/ STA220H1/ BIO220H1\n- LMP200H1\nThird Year:\n- BCH370H1, BCH311H1\n- LMP310H1, LMP320H1, LMP330H1\nChoose one of the options below:\n- Option A: Immunology: ( IMM341H1, IMM351H1)\n- Option B: Microbiology: ( MGY377H1, MGY378H1)\n- Option C: Anatomy: ANA300Y1\n- Option D: Metabolic and Signaling Pathways: PSL425H1 and PCL386H1\n- Option E: Research: LMP305Y1 and one of the above four options\nFourth Year:\nStudents should choose four of the following 400-level courses (2.0-2.5 credits):\n- LMP405Y1, LMP410H1, LMP415H1, LMP420H1, LMP430H1, LMP440H1, LMP450H1, LMP460H1\nNote: If the research course LMP405Y1 is taken, an additional three 400-level courses from the list are still required (totalling 2.5 credits)."},
		// Areas the student chooses: each area is an option grouping its language and
		// culture credits, and a choice row picks two. Yiddish, not in the sentence, is
		// a lenient reading; the South Slavic line stays text.
		{"ASSPE1205",
			"(10.5 credits, with at least 4.0 credits at the 300+ level, including 1.0 credit at the 400-level)\nStudents must choose any two of the available language areas (Czech and Slovak, Estonian, Finnish, Polish, Russian, South Slavic, Ukrainian).\nPlease note: Some courses may appear in more than one group thanks to their content. No course, however, may be counted twice for the purposes of program completion.\n1. 1.0 credit of Common Core: SLA200H1, SLA201H1, SLA231H1, SLA232H1, and SLA256H1\n2. 4.0 credits of language coursework from each of the two chosen language areas (2.0 credits per area).\n_Czech and Slovak:_ SLA105Y1, SLA204Y1, SLA214H1\n_Estonian:_ EST100H1, EST101H1, EST200H1, EST201H1, EST300H1, EST301H1\n_Finnish:_ FIN100H1, FIN110H1, FIN200H1, FIN210H1, FIN300H1, FIN340H1, FIN400H1; SWE100H1, SWE101H1\n_Polish:_ SLA106H1, SLA116H1, SLA206H1, SLA207H1, SLA306H1, SLA336H1, SLA426H1\n_Russian:_ SLA100H1, SLA101H1, SLA210H1, SLA214H1, SLA220Y1, SLA221H1, SLA320Y1, SLA321H1, SLA322H1, SLA410H1, SLA420Y1\n_South Slavic (Bosnian, Croatian, Serbian, Macedonian):_ [( SLA107Y1/ SLA117H1, SLA118H1); SLA214H1; SLA257H1; SLA277H1; SLA337H1]; / [ SLA109Y1, SLA209H1, SLA213H1]\n_Ukrainian:_ SLA108Y1, SLA208Y1, SLA308H1\n_Yiddish (corresponds to the Jewish Cultures in Eastern Europe area):_ GER261H1, GER262H1, GER360H1, GER460H1\nNote: In consultation with the Undergraduate Coordinator, and based on results in language placement tests, some students may substitute language courses with the equivalent number of credits in requirement 3 (below).\n3. 4.0 credits from culture courses corresponding to the student’s two chosen language areas (2.0 credits per area):\n_Czech and Slovak:_ SLA200H1, SLA215H1, SLA225H1, SLA254H1/ JSM354H1, SLA333H1, SLA335H1, SLA405H1\n_Estonian:_ EST205H1, EST206H1; EST207H1, EST208H1, FIN225H1, FIN240H1, FIN260H1, FIN310H1; HIS331H1; SLA200H1, SLA254H1/ JSM354H1\n_Finnish:_ FIN220H1, FIN225H1, FIN230H1, FIN236H1, FIN238H1, FIN240H1, FIN250H1, FIN260H1, FIN270H1, FIN280H1, FIN310H1, FIN320H1, FIN350H1, FIN360H1, FIN415H1; EST203H1, EST204H1, EST206H1; EST207H1, HIS331H1\n_Jewish Cultures in Eastern Europe (corresponds to the Yiddish language area):_ GER361H1; JJH370H1; SLA202H1; SLA224H1; SLA268H1; SLA302H1; SLA303H1; SLA318H1\n_Polish:_ SLA216H1, SLA226H1, SLA229H1, SLA236H1, SLA266H1, SLA286H1, SLA346H1, SLA356H1, SLA406H1, SLA427H1, SLA436H1\n_Russian:_ SLA121H1, SLA194H1, SLA195H1, SLA196H1, SLA203H1, SLA212H1, SLA224H1, SLA234H1, SLA245H1, SLA252H1, SLA253H1, SLA254H1/ JSM354H1, SLA260H1, SLA263H1, SLA268H1, SLA272H1, SLA280H1, JSH300H1, SLA300H1, SLA310H1, SLA311H1, SLA312H1, SLA314H1, SLA317H1, SLA318H1, SLA320Y1, SLA321H1, SLA322H1, SLA323H1, SLA330H1, SLA331H1, SLA334H1, SLA342H1, SLA343H1, SLA345H1, SLA367H1, SLA400H1, SLA404H1, SLA412H1, SLA413H1, SLA414H1, SLA415H1, SLA420Y1, SLA423H1, SLA424Y1, SLA430H1, SLA433H1, SLA434H1, SLA449H1, SLA463H1\n_South Slavic:_ SLA200H1, SLA217H1, SLA227H1, SLA235H1, SLA247H1, SLA330H1, SLA347H1, SLA357H1, SLA377H1, SLA380H1/ SLA232H1\n_Ukrainian:_ SLA199H1, SLA200H1, SLA203H1, SLA218H1, SLA228H1, SLA238H1, SLA248H1, SLA253H1, SLA254H1/ JSM354H1, SLA268H1, SLA311H1, SLA318H1, SLA328H1, SLA331H1, SLA338H1, SLA358H1, SLA428H1, SLA429H1\nNote: In consultation with the Undergraduate Coordinator, students may take up to 1.0 credit in history or political science courses corresponding to their two chosen language areas (0.5 credit per language area) and taught outside the department.\n4. 1.0 credit from the pool of comparative courses offered by the Department: EST205H1; EST206H1; EST207H1; FIN225H1, FIN238H1, FIN240H1, FIN260H1, SLA104H1, SLA190H1, SLA191H1, SLA192H1, SLA200H1, SLA201H1, SLA202H1, SLA203H1, SLA222H1, SLA229H1, SLA231H1, SLA232H1, SLA235H1, SLA254H1/ JSM354H1, SLA256H1, SLA268H1, SLA272H1, SLA286H1, SLA302H1, SLA303H1, SLA318H1, SLA330H1, SLA331H1, SLA333H1, SLA340H1, SLA380H1, SLA425H1, SLA435H1, SLA492H1\n5. 0.5 credit in Breadth Requirement Category 5: The Physical and Mathematical Universes or another 0.5 credit approved by the Undergraduate Coordinator, to fulfill the Quantitative Reasoning Competency required in the program."},
		// The reference program: "Of these:" bullets are Req 1's floors and caps, item
		// 7's "Additional ... to a total of" is Req 1's pool, and a sentence that
		// courses count for requirements 4 and 6 sets overlaps on both.
		{"ASMAJ2015",
			"(7.0 credits)\nOf these:\n- 2.0 credits at the 300+ level including a 0.5 credit at the 400 level.\n- No more than 1.0 credit of 100-level courses may be used to fulfill the program requirements.\n- No more than 1.0 credit of courses from outside of the Department of Political Science may be used to fulfill the program requirements. See the bottom of the completion requirements section for a full list of such acceptable courses.\nFirst Year:\n- 1.0 credit from: POL101H1, POL106H1, POL107H1, POL109H1. A maximum of 0.5 credit may be substituted from the following first-year courses: POL191H1, POL192H1, POL193H1, POL195H1, POL196H1, POL197H1, POL198H1, MUN100H1, MUN110H1, MUN130H1, TRN136Y1, TRN151Y1, TRN152Y1, TRN160Y1, TRN162Y1, TRN171Y1, TRN172Y1, TRN192H1, TRN193H1, TRN199H1, VIC110H1, VIC121H1, VIC167H1, VIC168H1, VIC181H1, VIC183H1, VIC184H1, VIC185H1, WDW151H1, WDW152H1.\nHigher Years:\n1. POL200Y1\n2. 0.5 credit from: POL201H1, POL208H1, POL218H1, POL219H1\n3. POL214H1/ POL224H1\n4. 0.5 credit at the 200+ level in Canadian Politics from JPI201H1, POL220H1, POL312H1, POL314H1, POL316H1, POL319H1, POL334H1, POL336H1, POL337H1, POL344H1, POL351H1, POL356H1, POL363H1, POL365H1, POL382H1, POL428H1, POL439H1, POL467H1, POL470H1, POL474H1, POL490H1, POL491H1\n   Some offerings of other POL Special Topics courses that cover Canadian politics may also be eligible as determined by the department. Please see Department’s website https://politics.utoronto.ca/undergraduate/courses/canadian-politics-co… for a full listing of Canadian politics courses.\n5. POL222H1\n   (or one of ECO220Y1, PSY201H1, SOC202H1, STA220H1). Please note only 0.5 of ECO220Y1 will be accepted towards the program.\n6. 1.0 credit in Diversity and Identity. The following courses fulfill the Diversity and Identity requirement: POL101H1, POL195H1, POL197H1, POL198H1, JPI201H1, POL219H1, POL220H1, POL223H1, POL224H1, POL301H1, POL303H1, POL305H1, POL309H1, POL310H1, JPS315H1, JPS316H1, POL325H1, POL329H1, POL331H1, POL333H1, POL338H1, POL344H1, POL348H1, POL351H1, POL358H1, POL360H1, POL365H1, JPR364H1, JPR365H1, JPR374H1, JPS378H1, POL387H1, POL418H1, POL428H1, POL432H1, POL442H1, POL444H1, POL447H1, POL453H1, JPR458H1, JPR459H1, POL467H1, POL469H1.\n   Some offerings of other POL Special Topics courses may also be eligible as determined by the department. Please see Department’s website https://politics.utoronto.ca/undergraduate/courses/diversity-identity-p… for a full listing of Diversity and Identity courses.\n7. Additional POL/ JPA/ JPF/ JPI/ JPR/ JPS/ JRA courses to a total of 7.0 credits.\nStudents in this program have the option to complete the Arts & Science Internship Program (ASIP) stream.\nNote:\n- Non-POL courses that may be used to fulfill the program requirements: AFR353H1, AFR455H1, CAR226H1, CAR324H1, CAR426H1, CRI390H1, CRI393H1, DTS314H1, EAS307H1, GER275H1, GER290H1, GGR324H1, GGR339H1, GGR344H1, GGR429H1, MUN100H1, MUN110H1, MUN130H1, PCJ260H1, PCJ261H1, RLG434H1, TRN136Y1, TRN151Y1, TRN152Y1, TRN160Y1, TRN162Y1, TRN171Y1, TRN172Y1, TRN192H1, TRN193H1, TRN199H1, URB339H1/ INI339H1, VIC110H1, VIC121H1, VIC167H1, VIC168H1, VIC181H1, VIC183H1, VIC184H1, VIC185H1, VIC476H1, WDW151H1, WDW152H1\n- Some courses may contribute to both the Canadian Politics and Diversity and Identity requirements (in requirements 4 and 6). Refer to the Department website for a full list of courses that meet these requirements."},
		// A reference to other requirements ("from Requirements 1, 3, 4, 5, or 6") is a
		// list of those rows' claims; "up to 1.0 credit from" is a limit on item 7, and
		// the notes' permission sentence is item 7's route.
		{"ASMAJ0728",
			"(7.0 credits, of which at least 2.5 must be at the 300+ level, including at least 0.5 credit at the 400-level).\nFirst Year: No specific first-year requirements.\n1. 1.0 credit from CDN265H1, CDN267H1, CDN268H1\n2. INS200H1\n3. 0.5 credit from any 200-level course offered by the Canadian Studies program.\n4. 0.5 credit from CDN307H1, CDN450H1, CDN455H1\n5. 0.5 credit from CDN335H1, CDN440H1\n6. 1.5 credits from any 300+ level course offered by the Canadian Studies program.\n7. An additional 2.5 credits from Requirements 1, 3, 4, 5, or 6, with up to 1.0 credit from ENG215H1, ENG252H1, ENG254H1, FAH248H1, FOR200H1, FRE210H1, GGR240H1, GGR246H1, HIS262H1, HIS264H1, INS231H1, LIN201H1, POL214Y1, SLA238H1, SOC218H1, SOC220H1, TRN235H1.\nNotes:\n1. Additional courses with significant Canadian content may be considered for approval for Requirement 7 by the Program Director (canadian.studies@utoronto.ca).\n2. Please note that not all courses are offered on a regular basis.\n3. Students are responsible for checking co- and prerequisites for all courses."},
	}
	want := map[string]string{}
	{
		raw := tests[0].raw
		bio := `{"type":"credits","amount":1.5,"courses":["BCH422H1","BCH426H1","BCH440H1","BCH444H1","BCH445H1","CJH332H1","CSB299Y1","CSB327H1","CSB328H1","CSB329H1","CSB330H1","CSB331H1","CSB340H1","CSB350H1","CSB351Y1","CSB352H1","CSB353H1","CSB360H1","CSB397Y0","CSB399Y1","CSB427H1","CSB428H1","CSB429H1","CSB430H1","CSB431H1","CSB435H1","CSB447H1","CSB450H1","CSB451H1","CSB452H1","CSB453H1","CSB454H1","CSB457H1","CSB458H1","CSB459H1","CSB460H1","CSB471H1","CSB472H1","CSB473H1","CSB474H1","CSB475H1","CSB483H1","CSB490H1","CSB491H1","CSB492H1","CSB497H1","CSB498Y1","CSB499Y1","HMB496Y1","HMB499Y1","MGY441H1","MGY480Y1"]}`
		lines := strings.Split(raw, "\n")
		want["ASMAJ1003"] = jComp(raw, []string{
			jRow(1, "allocation", 0, "", "", jCr(8), "(8.0 credits)"),
			jRow(2, "allocation", 1, "First Year 1", "First Year",
				jAnd(jC("BIO120H1"), jC("BIO130H1"), jOr(jAnd(jC("CHM135H1"), jC("CHM136H1")), jC("CHM151Y1"))), lines[2]),
			jRow(3, "allocation", 1, "First Year 2", "First Year",
				jOr(`{"type":"credits","amount":1,"courses":["JMB170Y1","MAT130H1","MAT135H1","MAT136H1","MAT137Y1","MAT157Y1","PHY131H1","PHY132H1","PHY151H1","PHY152H1"]}`,
					jAnd(jC("MAT148H1"), jC("MAT149H1")), jAnd(jC("MAT158H1"), jC("MAT159H1"))), lines[3]),
			jRow(4, "allocation", 1, "Higher Years 1", "Higher Years", jAnd(jC("BIO220H1"), jOr(jC("BIO230H1"), jC("BIO255H1"))), lines[5]),
			jRow(5, "allocation", 1, "Higher Years 2", "Higher Years", jAnd(jOr(jC("BIO260H1"), jC("HMB265H1")), jC("BCH210H1")), lines[6]),
			jRow(6, "allocation", 1, "Higher Years 3", "Higher Years", jC("CSB349H1"), lines[7]),
			jRow(7, "allocation", 1, "Higher Years 4", "Higher Years",
				`{"type":"credits","amount":1,"courses":["CSB327H1","CSB328H1","CSB329H1","CSB331H1","CSB340H1","CSB353H1"]}`, lines[8]),
			jRow(8, "allocation", 1, "Higher Years 5", "Higher Years", bio, lines[9]),
			jRow(9, "constraint", 8, "", "", `{"type":"credits","amount":0.5,"levels":[400]}`, "at least 0.5 credit at the 400-level"),
			jRow(10, "limit", 8, "", "", `{"type":"credits","designators":["BCH"],"max_amount":0.5}`,
				"No more than 0.5 credit in BCH can be used towards this requirement"),
			jRow(11, "limit", 8, "", "", `{"type":"credits","courses":["HMB496Y1","HMB499Y1"],"max_amount":1}`, lines[9]),
		}, `"notes":`+jStrs("The Cell & Molecular Biology Major Program has the additional option of a Disciplinary Focus",
			"Students in this program have the option to complete the Arts & Science Internship Program (ASIP) stream"))
	}
	{
		raw := tests[1].raw
		prt := `{"type":"credits","amount":2,"designators":["PRT"],"levels":[200,300,400]}`
		want["ASCER0338"] = jComp(raw, []string{
			jRow(1, "allocation", 0, "", "",
				jOr(prt, jT("permission", "Courses taken in our unit-run Summer Abroad programs will be considered; however, they are not guaranteed to be approved")),
				"2.0 PRT credits at the 200+ level taught in Portuguese, including at least 0.5 credit at the 300+ level.\nCourses taught at UTM or UTSC are not included for consideration\n"+
					"Courses taken in our unit-run Summer Abroad programs will be considered; however, they are not guaranteed to be approved"),
			jRow(2, "constraint", 1, "", "", jT("content", "2.0 PRT credits at the 200+ level taught in Portuguese"),
				"2.0 PRT credits at the 200+ level taught in Portuguese"),
			jRow(3, "constraint", 1, "", "", jOr(`{"type":"credits","amount":0.5,"designators":["PRT"],"levels":[300,400]}`, jT("permission", "including at least 0.5 credit at the 300+ level")),
				"including at least 0.5 credit at the 300+ level"),
			jRow(4, "limit", 1, "", "", `{"type":"credits","courses":["PRT250H1","PRT375H1"],"max_amount":0}`,
				"Courses with lectures taught in English – PRT250H1 or PRT375H1, for instance – are not included for consideration in the Certificate"),
			jRow(5, "limit", 1, "", "", `{"type":"credits","cr_ncr":true,"max_amount":0}`,
				"Courses that have been taken as Credit/No Credit cannot be used to fulfill Certificate requirements"),
			jRow(6, "limit", 1, "", "", `{"type":"credits","transfer":true,"max_amount":1}`,
				"No more than 1.0 transfer credits will be eligible for consideration in the Certificate"),
		})
	}
	{
		raw := tests[2].raw
		lines := strings.Split(raw, "\n")
		eco := `{"type":"credits","amount":1.5,"courses":["ECO310H1","ECO334H1","ECO339H1","ECO340H1","ECO353H1","ECO354H1","ECO357H1","ECO367H1","ECO374H1","ECO375H1","ECO380H1","ECO401H1","ECO403H1","ECO404H1","ECO418H1","ECO439H1","ECO446H1","ECO452H1","ECO464H1","ECO466H1","ECO475H1","ECO480H1","ECO481H1","ECO482H1","ECO483H1","ECO484H1","ECO499H1"]}`
		want["ASFOC1478B"] = jComp(raw, []string{
			jRow(1, "allocation", 0, "", "", jCr(3), "(3.0 credits)"),
			jRow(2, "allocation", 1, "1", "", `{"type":"credits","amount":0.5,"courses":["CSC108H1","CSC110Y1","CSC148H1"]}`, lines[1]),
			jRow(3, "allocation", 1, "2", "", jC("ECO372H1"), lines[2]),
			jRow(4, "allocation", 1, "3", "", eco, lines[3]),
			jRow(5, "allocation", 1, "4", "", jC("ECO489H1"), lines[4]),
		})
	}
	{
		raw := tests[3].raw
		lines := strings.Split(raw, "\n")
		want["ASMIN1006"] = jComp(raw, []string{
			jRow(1, "allocation", 0, "", "", jCr(4), lines[0]),
			jRow(2, "constraint", 1, "", "", `{"type":"credits","amount":1,"levels":[300,400]}`, "including at least 1.0 credits at 300+ level"),
			jRow(3, "allocation", 1, "", "", jAnd(jC("BIO120H1"), jC("BIO220H1")), lines[1]),
			jRow(4, "allocation", 1, "", "",
				`{"type":"credits","amount":0.5,"courses":["BIO251H1","EEB263H1","EEB266H1","EEB267H1","EEB268H1","EEB340H1","EEB380H1","EEB382H1","EEB384H1","EEB386H1","EEB388H1"]}`, lines[2]),
			jRow(5, "allocation", 1, "", "",
				`{"type":"credits","amount":0.5,"courses":["EEB314H1","EEB318H1","EEB319H1","EEB320H1","EEB321H1","EEB322H1","EEB323H1","EEB324H1","EEB328H1","EEB362H1"]}`, lines[3]),
			jRow(6, "allocation", 1, "", "",
				`{"type":"credits","amount":2,"designators":["EEB"],"courses":["BIO251H1","BIO260H1","EHJ352H1","HMB265H1"],"exclude":["EEB196H1","EEB197H1","EEB198H1","EEB199H1","EEB202H1","EEB208H1","EEB214H1","EEB215H1"]}`, lines[4]),
		})
	}
	{
		raw := tests[4].raw
		lines := strings.Split(raw, "\n")
		want["ASMAJ1689"] = jComp(raw, []string{
			jRow(1, "allocation", 0, "", "", `{"type":"or","items":[{"type":"credits","amount":8},{"type":"and","items":[{"type":"credits","amount":7.5},{"type":"not","items":[{"type":"course","code":"CSC108H1"}]}]},{"type":"and","items":[{"type":"credits","amount":7.5},{"type":"course","code":"CSC240H1"},{"type":"not","items":[{"type":"course","code":"CSC165H1"}]}]},{"type":"and","items":[{"type":"credits","amount":7},{"type":"not","items":[{"type":"course","code":"CSC108H1"}]},{"type":"course","code":"CSC240H1"},{"type":"not","items":[{"type":"course","code":"CSC165H1"}]}]},{"type":"text","kind":"permission","text":"(8.0 credits, including at least one 0.5 credit at the 400-level)"}]}`, "(8.0 credits, including at least one 0.5 credit at the 400-level)\nStudents with a strong background in an object-oriented language such as Python, Java or C++ may omit CSC108H1 and proceed directly with CSC148H1. [There is no need to replace the missing 0.5 credit for program completion; however, please base your course choice on what you are ready to take, not on “saving” a 0.5 credit]. b\nIf you take CSC240H1 without CSC165H1, there is no need to replace the missing half-credit for program completion; but please see Note (a)"),
			jRow(2, "constraint", 1, "", "", `{"type":"or","items":[{"type":"credits","amount":0.5,"levels":[400]},{"type":"text","kind":"permission","text":"including at least one 0.5 credit at the 400-level"}]}`, "including at least one 0.5 credit at the 400-level"),
			jRow(3, "allocation", 1, "1", "First year", `{"type":"and","items":[{"type":"or","items":[{"type":"and","items":[{"type":"or","items":[{"type":"course","code":"CSC108H1"},{"type":"not","items":[{"type":"course","code":"CSC108H1"}]}]},{"type":"course","code":"CSC148H1"},{"type":"or","items":[{"type":"course","code":"CSC165H1"},{"type":"course","code":"CSC240H1"}]}]},{"type":"and","items":[{"type":"course","code":"CSC110Y1"},{"type":"course","code":"CSC111H1"}]}]},{"type":"or","items":[{"type":"and","items":[{"type":"or","items":[{"type":"course","code":"MAT148H1"},{"type":"course","code":"MAT158H1"},{"type":"course","code":"MAT130H1"},{"type":"course","code":"MAT135H1"}]},{"type":"or","items":[{"type":"course","code":"MAT149H1"},{"type":"course","code":"MAT159H1"},{"type":"course","code":"MAT136H1"}]}]},{"type":"course","code":"MAT137Y1"},{"type":"course","code":"MAT157Y1"}]}]}`, "1. ( CSC108H1, CSC148H1, CSC165H1/ CSC240H1)/ ( CSC110Y1, CSC111H1); ( MAT148H1/ MAT158H1/ MAT130H1/ MAT135H1, MAT149H1/ MAT159H1/ MAT136H1)/ MAT137Y1/ MAT157Y1\nStudents with a strong background in an object-oriented language such as Python, Java or C++ may omit CSC108H1 and proceed directly with CSC148H1. [There is no need to replace the missing 0.5 credit for program completion; however, please base your course choice on what you are ready to take, not on “saving” a 0.5 credit]\nb. CSC240H1 is an accelerated and enriched version of CSC165H1 plus CSC236H1, intended for students with a strong mathematical background, or who develop an interest after taking CSC165H1\nIf you take CSC240H1 without CSC165H1, there is no need to replace the missing half-credit for program completion; but please see Note (a)", `"overlaps":[4]`),
			jRow(4, "allocation", 1, "2", "Second year", `{"type":"and","items":[{"type":"course","code":"CSC207H1"},{"type":"or","items":[{"type":"course","code":"CSC236H1"},{"type":"course","code":"CSC240H1"}]},{"type":"course","code":"CSC258H1"},{"type":"or","items":[{"type":"course","code":"CSC263H1"},{"type":"course","code":"CSC265H1"}]},{"type":"or","items":[{"type":"course","code":"STA247H1"},{"type":"course","code":"STA237H1"},{"type":"course","code":"STA255H1"},{"type":"course","code":"STA257H1"}]}]}`, "2. CSC207H1, CSC236H1/ CSC240H1, CSC258H1, CSC263H1/ CSC265H1, STA247H1/ STA237H1/ STA255H1/ STA257H1\nb. CSC240H1 is an accelerated and enriched version of CSC165H1 plus CSC236H1, intended for students with a strong mathematical background, or who develop an interest after taking CSC165H1\nIf you take CSC240H1 without CSC165H1, there is no need to replace the missing half-credit for program completion; but please see Note (a)", `"overlaps":[3]`),
			jRow(5, "allocation", 1, "3", "Later years", `{"type":"or","items":[{"type":"credits","amount":3,"lists":["Group A","Group B","Group C"]},{"type":"text","kind":"permission","text":"Students enrolled in the Computer Science Major (ASMAJ1689) program may request to count other University of Toronto Faculty of Applied Science and Engineering courses within Group C"}]}`, "3. 3.0 credits of courses in total selected from among the following groups:\nStudents enrolled in the Computer Science Major (ASMAJ1689) program may request to count other University of Toronto Faculty of Applied Science and Engineering courses within Group C"),
			jRow(6, "constraint", 5, "", "", `{"type":"or","items":[{"type":"credits","amount":3},{"type":"text","kind":"permission","text":"These 3.0 credits must include:"}]}`, lines[16]),
			jRow(7, "constraint", 5, "", "", `{"type":"or","items":[{"type":"credits","amount":2,"levels":[300,400],"lists":["Group A","Group B"]},{"type":"text","kind":"permission","text":"at least 2.0 credits from Groups A and/or B at the 300- /400-level, including at least 0.5 credit from a 400-level CSC course"}]}`, "at least 2.0 credits from Groups A and/or B at the 300- /400-level, including at least 0.5 credit from a 400-level CSC course"),
			jRow(8, "constraint", 5, "", "", `{"type":"or","items":[{"type":"credits","amount":0.5,"designators":["CSC"],"levels":[400],"lists":["Group A","Group B"]},{"type":"text","kind":"permission","text":"at least 2.0 credits from Groups A and/or B at the 300- /400-level, including at least 0.5 credit from a 400-level CSC course"}]}`, "at least 2.0 credits from Groups A and/or B at the 300- /400-level, including at least 0.5 credit from a 400-level CSC course"),
			jRow(9, "constraint", 5, "", "", `{"type":"or","items":[{"type":"credits","amount":1,"levels":[300,400],"lists":["Group A"]},{"type":"text","kind":"permission","text":"at least 1.0 credit at the 300-/400-level from Group A"}]}`, "at least 1.0 credit at the 300-/400-level from Group A"),
			jRow(10, "limit", 5, "", "", `{"type":"credits","lists":["Group B"],"max_amount":1.5}`, "no more than 1.5 credits from Group B, and no more than 1.0 credit from Group B at the 400-level"),
			jRow(11, "limit", 5, "", "", `{"type":"credits","levels":[400],"lists":["Group B"],"max_amount":1}`, "no more than 1.5 credits from Group B, and no more than 1.0 credit from Group B at the 400-level"),
			jRow(12, "limit", 5, "", "", `{"type":"credits","courses":["CSC311H1","STA314H1"],"max_amount":0.5}`, "no more than 0.5 credit from CSC311H1/ STA314H1"),
			jRow(13, "limit", 5, "", "", `{"type":"credits","courses":["CSC412H1","STA414H1"],"max_amount":0.5}`, "no more than 0.5 credit from CSC412H1/ STA414H1"),
			jRow(14, "limit", 5, "", "", `{"type":"credits","courses":["CSC438H1","MAT309H1"],"max_amount":0.5}`, "no more than 0.5 credit from CSC438H1/ MAT309H1"),
			jRow(15, "constraint", 5, "", "", `{"type":"or","items":[{"type":"credits","amount":0.5,"courses":["CSC301H1","CSC302H1","CSC316H1","CSC318H1","CSC394H1","CSC395H1","CSC396Y0","CSC404H1","CSC413H1","CSC417H1","CSC419H1","CSC428H1","CSC454H1","CSC485H1","CSC490H1","CSC491H1","CSC494H1","CSC495H1","CSC494Y1"]},{"type":"text","kind":"other","text":"Students who complete the Arts & Science Internship Program (ASIP) stream will also meet this requirement"}]}`, "The choices in 3 must satisfy the requirement for an integrative, inquiry-based activity by including one of the following courses: CSC301H1, CSC302H1, CSC316H1, CSC318H1, CSC394H1, CSC395H1, CSC396Y0, CSC404H1, CSC413H1, CSC417H1, CSC419H1, CSC428H1, CSC454H1, CSC485H1, CSC490H1, CSC491H1, CSC494H1, CSC495H1, CSC494Y1\nStudents who complete the Arts & Science Internship Program (ASIP) stream will also meet this requirement"),
			jRow(16, "limit", 1, "", "", `{"type":"credits","levels":[300,400],"transfer":true,"max_amount":1}`, "Transfer credits (except for those attained through a University of Toronto exchange program) cannot comprise more than 1.0 credit at the 300-/ 400-level, and cannot be used to satisfy the requirement for an integrative, inquiry-based activity"),
			jRow(17, "constraint", 1, "", "", `{"type":"text","kind":"other","text":"Transfer credits (except for those attained through a University of Toronto exchange program) cannot comprise more than 1.0 credit at the 300-/ 400-level, and cannot be used to satisfy the requirement for an integrative, inquiry-based activity"}`, "Transfer credits (except for those attained through a University of Toronto exchange program) cannot comprise more than 1.0 credit at the 300-/ 400-level, and cannot be used to satisfy the requirement for an integrative, inquiry-based activity"),
			jRow(18, "constraint", 1, "", "", `{"type":"text","kind":"other","text":"In addition, transfer credits (except for those attained through a University of Toronto exchange program) cannot be used to satisfy the requirement for 0.5 credit at the 400-level in CSC"}`, "In addition, transfer credits (except for those attained through a University of Toronto exchange program) cannot be used to satisfy the requirement for 0.5 credit at the 400-level in CSC"),
		}, `"lists":[{"name":"Group A","items":[{"type":"credits","courses":["CSC209H1"]},{"type":"credits","designators":["CSC"],"levels":[300,400],"exclude":["CSC299H1","CSC299Y1","CSC394H1","CSC395H1","CSC396Y0","CSC398H0","CSC398Y0","CSC399H1","CSC399Y1","CSC490H1","CSC491H1","CSC494H1","CSC494Y1","CSC495H1"]}]},{"name":"Group B","items":[{"type":"credits","courses":["CSC299H1","CSC299Y1","CSC394H1","CSC395H1","CSC396Y0","CSC398H0","CSC398Y0","CSC399H1","CSC399Y1","CSC490H1","CSC491H1","CSC494H1","CSC494Y1","CSC495H1"]}]},{"name":"Group C","items":[{"type":"credits","courses":["BCB330Y1","BCB410H1","BCB420H1","BCB430Y1","MAT223H1","MAT240H1","MAT235Y1","MAT237Y1","MAT257Y1","STA248H1","STA238H1","STA261H1","ECE419H1","ECE454H1"]},{"type":"credits","designators":["MAT"],"levels":[300,400],"exclude":["MAT329Y1","MAT390H1","MAT391H1"]},{"type":"credits","designators":["STA"],"levels":[300,400]},{"type":"and","items":[{"type":"course","code":"MAT235H1"},{"type":"course","code":"MAT236H1"}]}]}]`, `"exclusions":[{"type":"program","programs":["ASSPE1689"],"text":"Computer Science Specialist (ASSPE1689)"},{"type":"program","programs":["ASMIN1689"],"text":"Computer Science Minor (ASMIN1689)"}]`, `"notes":`+jStrs("Students in this program have the option to enrol in the Arts & Science Internship Program (ASIP) stream", "Consult the Undergraduate Office for advice about choosing among CSC108H1 and CSC148H1, and between CSC165H1 and CSC240H1", "We recommend that students take MAT148H1/ MAT158H1 and MAT149H1/ MAT159H1, as they provide the best preparation for upper-year courses in computer science and benefit students in CSC165H1/ CSC240H1", "Requests must be reviewed and approved by the Computer Science Undergraduate Office and will be evaluated on a case-by-case basis", "For information about this process, please contact the Computer Science Undergraduate Office"))
	}
	{
		raw := tests[5].raw
		lines := strings.Split(raw, "\n")
		want["ASMAJ1160"] = jComp(raw, []string{
			jRow(1, "allocation", 0, "", "", `{"type":"or","items":[{"type":"credits","amount":7},{"type":"text","kind":"permission","text":"**(7.0 credits):**"}]}`, lines[0]),
			jRow(2, "allocation", 1, "1", "", `{"type":"or","items":[{"type":"course","code":"PSY100H1"},{"type":"course","code":"PSY100Y5"},{"type":"and","items":[{"type":"course","code":"PSYA01H3"},{"type":"course","code":"PSYA02H3"}]}]}`, lines[1]),
			jRow(3, "allocation", 1, "2", "", `{"type":"or","items":[{"type":"course","code":"PSY201H1"},{"type":"text","kind":"permission","text":"We will accept equivalent/exclusion statistics courses from UTM/UTSC in lieu of PSY201H1. Please contact us to verify if your stats course qualifies"},{"type":"course","code":"ECO220Y1"},{"type":"course","code":"EEB225H1"},{"type":"course","code":"GGR270H1"},{"type":"course","code":"IRW220H1"},{"type":"course","code":"POL222H1"},{"type":"course","code":"SOC202H1"},{"type":"course","code":"STA220H1"},{"type":"course","code":"STA238H1"},{"type":"course","code":"STA248H1"},{"type":"course","code":"STA288H1"}]}`, "2. PSY201H1 (or one of ECO220Y1/ EEB225H1/ GGR270H1/ IRW220H1/ POL222H1/ SOC202H1/ STA220H1/ STA238H1/ STA248H1/ STA288H1)\nWe will accept equivalent/exclusion statistics courses from UTM/UTSC in lieu of PSY201H1. Please contact us to verify if your stats course qualifies"),
			jRow(4, "allocation", 1, "3", "", `{"type":"credits","amount":1,"courses":["PSY210H1","PSY220H1","PSY230H1","PSY240H1"],"lists":["Cluster A"]}`, lines[3]),
			jRow(5, "allocation", 1, "4", "", `{"type":"credits","amount":1,"courses":["PSY260H1","PSY270H1","PSY280H1","PSY290H1"],"lists":["Cluster B"]}`, lines[4], `"notes":`+jStrs("NOTE: HMB200H1 cannot be used in place of PSY290H1")),
			jRow(6, "allocation", 1, "5", "", `{"type":"credits","amount":0.5,"designators":["PSY"],"levels":[300,400],"lists":["Cluster A"]}`, lines[5]),
			jRow(7, "allocation", 1, "6", "", `{"type":"credits","amount":0.5,"designators":["PSY"],"levels":[300,400],"lists":["Cluster B"]}`, lines[6]),
			jRow(8, "allocation", 1, "7", "", `{"type":"credits","amount":1,"designators":["PSY"],"levels":[300,400],"lists":["Group 1"]}`, lines[7]),
			jRow(9, "allocation", 1, "8", "", `{"type":"credits","amount":0.5,"designators":["PSY"],"levels":[400],"lists":["Group 1"]}`, lines[8]),
			jRow(10, "allocation", 1, "9", "", `{"type":"credits","amount":1.5,"lists":["Group 1","Group 2"]}`, lines[9]),
			jRow(11, "limit", 1, "", "", `{"type":"credits","courses":["PSY304Y1","PSY306Y0","PSY299H1","PSY299Y1","PSY399H1","PSY399Y1","PSY405H1","PSY406H1","PSY405Y1","PSY406Y1"],"max_amount":2}`, "No more than 2.0 credits from PSY304Y1, PSY306Y0, PSY299H1, PSY299Y1, PSY399H1, PSY399Y1, PSY405H1, PSY406H1, PSY405Y1, and PSY406Y1 can be used for program requirements"),
		}, `"lists":[{"name":"Group 1","title":"Group 1 - Psychology Courses","items":[{"type":"credits","lists":["Cluster A"]},{"type":"credits","lists":["Cluster B"]},{"type":"credits","lists":["Cluster C"]}]},{"name":"Cluster A","title":"Cluster A - Development, Social, Personality, and Psychopathology and Clinical Science","items":[{"type":"credits","courses":["JLP315H1","PSY194H1","PSY195H1","PSY197H1","PSY210H1","PSY220H1","PSY230H1","PSY240H1","PSY311H1","PSY312H1","PSY313H1","PSY316H1","PSY319H1","PSY320H1","PSY321H1","PSY322H1","PSY323H1","PSY324H1","PSY326H1","PSY328H1","PSY329H1","PSY330H1","PSY331H1","PSY332H1","PSY333H1","PSY336H1","PSY337H1","PSY339H1","PSY341H1","PSY342H1","PSY343H1","PSY349H1","PSY410H1","PSY414H1","PSY417H1","PSY420H1","PSY421H1","PSY422H1","PSY424H1","PSY425H1","PSY426H1","PSY427H1","PSY428H1","PSY430H1","PSY435H1","PSY440H1","PSY450H1","PSY455H1"]}]},{"name":"Cluster B","title":"Cluster B - Learning, Perception, Cognition, and the Brain","items":[{"type":"credits","courses":["JLP374H1","JLP471H1","PSY196H1","PSY198H1","PSY199H1","PSY260H1","PSY270H1","PSY280H1","PSY290H1","PSY312H1","PSY316H1","PSY359H1","PSY360H1","PSY362H1","PSY369H1","PSY370H1","PSY371H1","PSY372H1","PSY378H1","PSY379H1","PSY380H1","PSY389H1","PSY390H1","PSY395H1","PSY396H1","PSY397H1","PSY450H1","PSY455H1","PSY460H1","PSY470H1","PSY471H1","PSY473H1","PSY475H1","PSY480H1","PSY490H1","PSY492H1","PSY493H1","PSY494H1","PSY495H1","PSY496H1","PSY497H1"]}]},{"name":"Cluster C","title":"Cluster C - Methods and Applications","items":[{"type":"credits","courses":["PSY201H1","PSY202H1","PSY203H1","PSY204H1","PSY205H1","PSY299H1","PSY299Y1","PSY304Y1","PSY305H1","PSY306Y0","PSY307H1","PSY308H1","PSY399H1","PSY399Y1","PSY400Y1","PSY402H1","PSY403H1","PSY404H1","PSY405H1","PSY405Y1","PSY406H1","PSY406Y1","PSY407H1","PSY408H1","PSY409H1","PSY422H1","PSY455H1"]}]},{"name":"Group 2","title":"Group 2 - Related Courses","items":[{"type":"credits","courses":["ANT331H1","ANT442H1","BIO120H1","BIO130H1","BIO220H1","BIO270H1","BIO271H1","BPM232H1","BPM330H1","BPM332H1","BPM333H1","BPM335H1","BPM336H1","BPM337H1","BPM338H1","BPM339H1","BPM432H1","BPM433H1","BPM438H1","BPM499H1","CJH332H1","COG250Y1","COG260H1","COG341H1","COG342H1","COG343H1","CRE345H1","CRI365H1","CRI431H1","ECO316H1","ECO326H1","ECO423H1","EDS261H1","EEB313H1","ENG384H1","HIS489H1","HMB200H1","HMB300H1","HMB310H1","HMB320H1","HMB420H1","HMB440H1","HMB460H1","HMB473H1","HPS110H1","HPS120H1","HPS200H1","HPS300H1","HPS347H1","IRE260H1","JFL388H1","JFP450H1","JHA410H1","JLS472H1","JLS473H1","JLS475H1","JLS476H1","LIN101H1","LIN102H1","LIN194H1","LIN195H1","LIN200H1","LIN208H1","NEW302Y1","NEW303H1","NFS489H1","PCL200H1","PCL475H1","PCL476H1","PHL240H1","PHL243H1","PHL244H1","PHL319H1","PHL340H1","PHL342H1","PHL358H1","PHL383H1","PSL201Y1","PSL300H1","PSL440Y1","PSL441H1","PSL442H1","PSL472H1","RLG106H1","RLG211H1","RLG380H1","RLG390H1","RLG417H1","RLG421H1","RSM260H1","RSM353H1","SOC213H1","SOC363H1","SOC412H1","VIC106H1","VIC206H1","WGS372H1","WRR307H1"]}]}]`, `"notes":`+jStrs("Students in this program have the option to complete the Arts & Science Internship Program (ASIP) stream", "These are all individual requirements and cannot overlap or be used twice within the Major", "Some Group 2 courses may have enrolment restrictions", "PSY students should not assume priority access to these courses unless they are enrolled in the host program"))
	}
	{
		raw := tests[6].raw
		lines := strings.Split(raw, "\n")
		want["ASSPE2025"] = jComp(raw, []string{
			jRow(1, "allocation", 0, "", "", `{"type":"credits","amount":14}`, lines[0]),
			jRow(2, "constraint", 1, "", "", `{"type":"credits","amount":2,"levels":[400]}`, "including at least 2.0 credits at the 400-level"),
			jRow(3, "allocation", 1, "", "First Year", `{"type":"and","items":[{"type":"course","code":"BIO120H1"},{"type":"course","code":"BIO130H1"}]}`, lines[2]),
			jRow(4, "allocation", 1, "", "First Year", `{"type":"or","items":[{"type":"and","items":[{"type":"course","code":"CHM135H1"},{"type":"course","code":"CHM136H1"}]},{"type":"course","code":"CHM151Y1"}]}`, lines[3]),
			jRow(5, "allocation", 1, "", "First Year", `{"type":"or","items":[{"type":"and","items":[{"type":"or","items":[{"type":"course","code":"MAT130H1"},{"type":"course","code":"MAT135H1"}]},{"type":"course","code":"MAT136H1"}]},{"type":"and","items":[{"type":"course","code":"MAT148H1"},{"type":"course","code":"MAT149H1"}]},{"type":"course","code":"MAT137Y1"},{"type":"and","items":[{"type":"course","code":"MAT158H1"},{"type":"course","code":"MAT159H1"}]},{"type":"course","code":"MAT157Y1"}]}`, lines[4]),
			jRow(6, "allocation", 1, "", "First or Second Year", `{"type":"or","items":[{"type":"and","items":[{"type":"course","code":"PHY131H1"},{"type":"course","code":"PHY132H1"}]},{"type":"and","items":[{"type":"course","code":"PHY151H1"},{"type":"course","code":"PHY152H1"}]}]}`, lines[6]),
			jRow(7, "allocation", 1, "", "Second Year", `{"type":"course","code":"BCH210H1"}`, lines[8]),
			jRow(8, "allocation", 1, "", "Second Year", `{"type":"course","code":"BIO230H1"}`, lines[9]),
			jRow(9, "allocation", 1, "", "Second Year", `{"type":"or","items":[{"type":"course","code":"BIO260H1"},{"type":"course","code":"HMB265H1"}]}`, lines[10]),
			jRow(10, "allocation", 1, "", "Second Year", `{"type":"course","code":"CHM220H1"}`, lines[11]),
			jRow(11, "allocation", 1, "", "Second Year", `{"type":"or","items":[{"type":"course","code":"CHM247H1"},{"type":"course","code":"CHM249H1"}]}`, lines[12]),
			jRow(12, "allocation", 1, "", "Second Year", `{"type":"and","items":[{"type":"course","code":"PSL300H1"},{"type":"course","code":"PSL301H1"}]}`, lines[13]),
			jRow(13, "allocation", 1, "", "Second Year", `{"type":"or","items":[{"type":"course","code":"STA288H1"},{"type":"course","code":"STA220H1"},{"type":"course","code":"BIO220H1"}]}`, lines[14]),
			jRow(14, "allocation", 1, "", "Second Year", `{"type":"course","code":"LMP200H1"}`, lines[15]),
			jRow(15, "allocation", 1, "", "Third Year", `{"type":"and","items":[{"type":"course","code":"BCH370H1"},{"type":"course","code":"BCH311H1"}]}`, lines[17]),
			jRow(16, "allocation", 1, "", "Third Year", `{"type":"and","items":[{"type":"course","code":"LMP310H1"},{"type":"course","code":"LMP320H1"},{"type":"course","code":"LMP330H1"}]}`, lines[18]),
			jRow(17, "allocation", 1, "Option A", "Third Year", `{"type":"and","items":[{"type":"course","code":"IMM341H1"},{"type":"course","code":"IMM351H1"}]}`, lines[20]),
			jRow(18, "allocation", 1, "Option B", "Third Year", `{"type":"and","items":[{"type":"course","code":"MGY377H1"},{"type":"course","code":"MGY378H1"}]}`, lines[21]),
			jRow(19, "allocation", 1, "Option C", "Third Year", `{"type":"course","code":"ANA300Y1"}`, lines[22]),
			jRow(20, "allocation", 1, "Option D", "Third Year", `{"type":"and","items":[{"type":"course","code":"PSL425H1"},{"type":"course","code":"PCL386H1"}]}`, lines[23]),
			jRow(21, "allocation", 1, "Option E", "Third Year", `{"type":"course","code":"LMP305Y1"}`, lines[24]),
			jRow(22, "choice", 0, "", "Third Year", `{"type":"or","items":[{"type":"requirement","requirements":[17]},{"type":"requirement","requirements":[18]},{"type":"requirement","requirements":[19]},{"type":"requirement","requirements":[20]},{"type":"and","items":[{"type":"requirement","requirements":[21]},{"type":"requirement","requirements":[17,18,19,20]}]}]}`, lines[19]),
			jRow(23, "allocation", 1, "", "Fourth Year", `{"type":"choose","items":[{"type":"course","code":"LMP405Y1"},{"type":"course","code":"LMP410H1"},{"type":"course","code":"LMP415H1"},{"type":"course","code":"LMP420H1"},{"type":"course","code":"LMP430H1"},{"type":"course","code":"LMP440H1"},{"type":"course","code":"LMP450H1"},{"type":"course","code":"LMP460H1"}],"count":4}`, "Students should choose four of the following 400-level courses (2.0-2.5 credits):\n- LMP405Y1, LMP410H1, LMP415H1, LMP420H1, LMP430H1, LMP440H1, LMP450H1, LMP460H1\nIf the research course LMP405Y1 is taken, an additional three 400-level courses from the list are still required (totalling 2.5 credits)"),
		})
	}
	{
		raw := tests[7].raw
		lines := strings.Split(raw, "\n")
		want["ASSPE1205"] = jComp(raw, []string{
			jRow(1, "allocation", 0, "", "", `{"type":"or","items":[{"type":"credits","amount":10.5},{"type":"text","kind":"permission","text":"(10.5 credits, with at least 4.0 credits at the 300+ level, including 1.0 credit at the 400-level)"}]}`, lines[0]),
			jRow(2, "constraint", 1, "", "", `{"type":"or","items":[{"type":"credits","amount":4,"levels":[300,400]},{"type":"text","kind":"permission","text":"with at least 4.0 credits at the 300+ level"}]}`, "with at least 4.0 credits at the 300+ level"),
			jRow(3, "constraint", 1, "", "", `{"type":"or","items":[{"type":"credits","amount":1,"levels":[400]},{"type":"text","kind":"permission","text":"including 1.0 credit at the 400-level"}]}`, "including 1.0 credit at the 400-level"),
			jRow(4, "allocation", 1, "1", "", `{"type":"credits","amount":1,"courses":["SLA200H1","SLA201H1","SLA231H1","SLA232H1","SLA256H1"]}`, lines[3]),
			jRow(5, "allocation", 1, "Czech and Slovak", "", `null`, lines[5]+"\n"+lines[15]),
			jRow(6, "allocation", 5, "", "", `{"type":"or","items":[{"type":"credits","amount":2,"courses":["SLA105Y1","SLA204Y1","SLA214H1"]},{"type":"and","items":[{"type":"credits","amount":2,"courses":["SLA105Y1","SLA204Y1","SLA214H1","SLA200H1","SLA215H1","SLA225H1","SLA254H1","JSM354H1","SLA333H1","SLA335H1","SLA405H1"]},{"type":"text","kind":"permission","text":"In consultation with the Undergraduate Coordinator, and based on results in language placement tests, some students may substitute language courses with the equivalent number of credits in requirement 3 (below)"}]}]}`, "_Czech and Slovak:_ SLA105Y1, SLA204Y1, SLA214H1\nIn consultation with the Undergraduate Coordinator, and based on results in language placement tests, some students may substitute language courses with the equivalent number of credits in requirement 3 (below)"),
			jRow(7, "allocation", 5, "", "", `{"type":"or","items":[{"type":"credits","amount":2,"courses":["SLA200H1","SLA215H1","SLA225H1","SLA254H1","JSM354H1","SLA333H1","SLA335H1","SLA405H1"]},{"type":"and","items":[{"type":"credits","amount":2,"designators":["HIS","POL"],"courses":["SLA200H1","SLA215H1","SLA225H1","SLA254H1","JSM354H1","SLA333H1","SLA335H1","SLA405H1"]},{"type":"text","kind":"permission","text":"In consultation with the Undergraduate Coordinator, students may take up to 1.0 credit in history or political science courses corresponding to their two chosen language areas (0.5 credit per language area) and taught outside the department"}]}]}`, "_Czech and Slovak:_ SLA200H1, SLA215H1, SLA225H1, SLA254H1/ JSM354H1, SLA333H1, SLA335H1, SLA405H1\nIn consultation with the Undergraduate Coordinator, students may take up to 1.0 credit in history or political science courses corresponding to their two chosen language areas (0.5 credit per language area) and taught outside the department"),
			jRow(8, "limit", 7, "", "", `{"type":"credits","courses":["SLA254H1","JSM354H1"],"max_amount":0.5}`, lines[15]),
			jRow(9, "limit", 7, "", "", `{"type":"credits","designators":["HIS","POL"],"max_amount":0.5}`, "In consultation with the Undergraduate Coordinator, students may take up to 1.0 credit in history or political science courses corresponding to their two chosen language areas (0.5 credit per language area) and taught outside the department"),
			jRow(10, "allocation", 1, "Estonian", "", `null`, lines[6]+"\n"+lines[16]),
			jRow(11, "allocation", 10, "", "", `{"type":"or","items":[{"type":"credits","amount":2,"courses":["EST100H1","EST101H1","EST200H1","EST201H1","EST300H1","EST301H1"]},{"type":"and","items":[{"type":"credits","amount":2,"courses":["EST100H1","EST101H1","EST200H1","EST201H1","EST300H1","EST301H1","EST205H1","EST206H1","EST207H1","EST208H1","FIN225H1","FIN240H1","FIN260H1","FIN310H1","HIS331H1","SLA200H1","SLA254H1","JSM354H1"]},{"type":"text","kind":"permission","text":"In consultation with the Undergraduate Coordinator, and based on results in language placement tests, some students may substitute language courses with the equivalent number of credits in requirement 3 (below)"}]}]}`, "_Estonian:_ EST100H1, EST101H1, EST200H1, EST201H1, EST300H1, EST301H1\nIn consultation with the Undergraduate Coordinator, and based on results in language placement tests, some students may substitute language courses with the equivalent number of credits in requirement 3 (below)"),
			jRow(12, "allocation", 10, "", "", `{"type":"or","items":[{"type":"credits","amount":2,"courses":["EST205H1","EST206H1","EST207H1","EST208H1","FIN225H1","FIN240H1","FIN260H1","FIN310H1","HIS331H1","SLA200H1","SLA254H1","JSM354H1"]},{"type":"and","items":[{"type":"credits","amount":2,"designators":["HIS","POL"],"courses":["EST205H1","EST206H1","EST207H1","EST208H1","FIN225H1","FIN240H1","FIN260H1","FIN310H1","HIS331H1","SLA200H1","SLA254H1","JSM354H1"]},{"type":"text","kind":"permission","text":"In consultation with the Undergraduate Coordinator, students may take up to 1.0 credit in history or political science courses corresponding to their two chosen language areas (0.5 credit per language area) and taught outside the department"}]}]}`, "_Estonian:_ EST205H1, EST206H1; EST207H1, EST208H1, FIN225H1, FIN240H1, FIN260H1, FIN310H1; HIS331H1; SLA200H1, SLA254H1/ JSM354H1\nIn consultation with the Undergraduate Coordinator, students may take up to 1.0 credit in history or political science courses corresponding to their two chosen language areas (0.5 credit per language area) and taught outside the department"),
			jRow(13, "limit", 12, "", "", `{"type":"credits","courses":["SLA254H1","JSM354H1"],"max_amount":0.5}`, lines[16]),
			jRow(14, "limit", 12, "", "", `{"type":"credits","designators":["HIS","POL"],"exclude":["HIS331H1"],"max_amount":0.5}`, "In consultation with the Undergraduate Coordinator, students may take up to 1.0 credit in history or political science courses corresponding to their two chosen language areas (0.5 credit per language area) and taught outside the department"),
			jRow(15, "allocation", 1, "Finnish", "", `null`, lines[7]+"\n"+lines[17]),
			jRow(16, "allocation", 15, "", "", `{"type":"or","items":[{"type":"credits","amount":2,"courses":["FIN100H1","FIN110H1","FIN200H1","FIN210H1","FIN300H1","FIN340H1","FIN400H1","SWE100H1","SWE101H1"]},{"type":"and","items":[{"type":"credits","amount":2,"courses":["FIN100H1","FIN110H1","FIN200H1","FIN210H1","FIN300H1","FIN340H1","FIN400H1","SWE100H1","SWE101H1","FIN220H1","FIN225H1","FIN230H1","FIN236H1","FIN238H1","FIN240H1","FIN250H1","FIN260H1","FIN270H1","FIN280H1","FIN310H1","FIN320H1","FIN350H1","FIN360H1","FIN415H1","EST203H1","EST204H1","EST206H1","EST207H1","HIS331H1"]},{"type":"text","kind":"permission","text":"In consultation with the Undergraduate Coordinator, and based on results in language placement tests, some students may substitute language courses with the equivalent number of credits in requirement 3 (below)"}]}]}`, "_Finnish:_ FIN100H1, FIN110H1, FIN200H1, FIN210H1, FIN300H1, FIN340H1, FIN400H1; SWE100H1, SWE101H1\nIn consultation with the Undergraduate Coordinator, and based on results in language placement tests, some students may substitute language courses with the equivalent number of credits in requirement 3 (below)"),
			jRow(17, "allocation", 15, "", "", `{"type":"or","items":[{"type":"credits","amount":2,"courses":["FIN220H1","FIN225H1","FIN230H1","FIN236H1","FIN238H1","FIN240H1","FIN250H1","FIN260H1","FIN270H1","FIN280H1","FIN310H1","FIN320H1","FIN350H1","FIN360H1","FIN415H1","EST203H1","EST204H1","EST206H1","EST207H1","HIS331H1"]},{"type":"and","items":[{"type":"credits","amount":2,"designators":["HIS","POL"],"courses":["FIN220H1","FIN225H1","FIN230H1","FIN236H1","FIN238H1","FIN240H1","FIN250H1","FIN260H1","FIN270H1","FIN280H1","FIN310H1","FIN320H1","FIN350H1","FIN360H1","FIN415H1","EST203H1","EST204H1","EST206H1","EST207H1","HIS331H1"]},{"type":"text","kind":"permission","text":"In consultation with the Undergraduate Coordinator, students may take up to 1.0 credit in history or political science courses corresponding to their two chosen language areas (0.5 credit per language area) and taught outside the department"}]}]}`, "_Finnish:_ FIN220H1, FIN225H1, FIN230H1, FIN236H1, FIN238H1, FIN240H1, FIN250H1, FIN260H1, FIN270H1, FIN280H1, FIN310H1, FIN320H1, FIN350H1, FIN360H1, FIN415H1; EST203H1, EST204H1, EST206H1; EST207H1, HIS331H1\nIn consultation with the Undergraduate Coordinator, students may take up to 1.0 credit in history or political science courses corresponding to their two chosen language areas (0.5 credit per language area) and taught outside the department"),
			jRow(18, "limit", 17, "", "", `{"type":"credits","designators":["HIS","POL"],"exclude":["HIS331H1"],"max_amount":0.5}`, "In consultation with the Undergraduate Coordinator, students may take up to 1.0 credit in history or political science courses corresponding to their two chosen language areas (0.5 credit per language area) and taught outside the department"),
			jRow(19, "allocation", 1, "Polish", "", `null`, lines[8]+"\n"+lines[19]),
			jRow(20, "allocation", 19, "", "", `{"type":"or","items":[{"type":"credits","amount":2,"courses":["SLA106H1","SLA116H1","SLA206H1","SLA207H1","SLA306H1","SLA336H1","SLA426H1"]},{"type":"and","items":[{"type":"credits","amount":2,"courses":["SLA106H1","SLA116H1","SLA206H1","SLA207H1","SLA306H1","SLA336H1","SLA426H1","SLA216H1","SLA226H1","SLA229H1","SLA236H1","SLA266H1","SLA286H1","SLA346H1","SLA356H1","SLA406H1","SLA427H1","SLA436H1"]},{"type":"text","kind":"permission","text":"In consultation with the Undergraduate Coordinator, and based on results in language placement tests, some students may substitute language courses with the equivalent number of credits in requirement 3 (below)"}]}]}`, "_Polish:_ SLA106H1, SLA116H1, SLA206H1, SLA207H1, SLA306H1, SLA336H1, SLA426H1\nIn consultation with the Undergraduate Coordinator, and based on results in language placement tests, some students may substitute language courses with the equivalent number of credits in requirement 3 (below)"),
			jRow(21, "allocation", 19, "", "", `{"type":"or","items":[{"type":"credits","amount":2,"courses":["SLA216H1","SLA226H1","SLA229H1","SLA236H1","SLA266H1","SLA286H1","SLA346H1","SLA356H1","SLA406H1","SLA427H1","SLA436H1"]},{"type":"and","items":[{"type":"credits","amount":2,"designators":["HIS","POL"],"courses":["SLA216H1","SLA226H1","SLA229H1","SLA236H1","SLA266H1","SLA286H1","SLA346H1","SLA356H1","SLA406H1","SLA427H1","SLA436H1"]},{"type":"text","kind":"permission","text":"In consultation with the Undergraduate Coordinator, students may take up to 1.0 credit in history or political science courses corresponding to their two chosen language areas (0.5 credit per language area) and taught outside the department"}]}]}`, "_Polish:_ SLA216H1, SLA226H1, SLA229H1, SLA236H1, SLA266H1, SLA286H1, SLA346H1, SLA356H1, SLA406H1, SLA427H1, SLA436H1\nIn consultation with the Undergraduate Coordinator, students may take up to 1.0 credit in history or political science courses corresponding to their two chosen language areas (0.5 credit per language area) and taught outside the department"),
			jRow(22, "limit", 21, "", "", `{"type":"credits","designators":["HIS","POL"],"max_amount":0.5}`, "In consultation with the Undergraduate Coordinator, students may take up to 1.0 credit in history or political science courses corresponding to their two chosen language areas (0.5 credit per language area) and taught outside the department"),
			jRow(23, "allocation", 1, "Russian", "", `null`, lines[9]+"\n"+lines[20]),
			jRow(24, "allocation", 23, "", "", `{"type":"or","items":[{"type":"credits","amount":2,"courses":["SLA100H1","SLA101H1","SLA210H1","SLA214H1","SLA220Y1","SLA221H1","SLA320Y1","SLA321H1","SLA322H1","SLA410H1","SLA420Y1"]},{"type":"and","items":[{"type":"credits","amount":2,"courses":["SLA100H1","SLA101H1","SLA210H1","SLA214H1","SLA220Y1","SLA221H1","SLA320Y1","SLA321H1","SLA322H1","SLA410H1","SLA420Y1","SLA121H1","SLA194H1","SLA195H1","SLA196H1","SLA203H1","SLA212H1","SLA224H1","SLA234H1","SLA245H1","SLA252H1","SLA253H1","SLA254H1","JSM354H1","SLA260H1","SLA263H1","SLA268H1","SLA272H1","SLA280H1","JSH300H1","SLA300H1","SLA310H1","SLA311H1","SLA312H1","SLA314H1","SLA317H1","SLA318H1","SLA323H1","SLA330H1","SLA331H1","SLA334H1","SLA342H1","SLA343H1","SLA345H1","SLA367H1","SLA400H1","SLA404H1","SLA412H1","SLA413H1","SLA414H1","SLA415H1","SLA423H1","SLA424Y1","SLA430H1","SLA433H1","SLA434H1","SLA449H1","SLA463H1"]},{"type":"text","kind":"permission","text":"In consultation with the Undergraduate Coordinator, and based on results in language placement tests, some students may substitute language courses with the equivalent number of credits in requirement 3 (below)"}]}]}`, "_Russian:_ SLA100H1, SLA101H1, SLA210H1, SLA214H1, SLA220Y1, SLA221H1, SLA320Y1, SLA321H1, SLA322H1, SLA410H1, SLA420Y1\nIn consultation with the Undergraduate Coordinator, and based on results in language placement tests, some students may substitute language courses with the equivalent number of credits in requirement 3 (below)"),
			jRow(25, "allocation", 23, "", "", `{"type":"or","items":[{"type":"credits","amount":2,"courses":["SLA121H1","SLA194H1","SLA195H1","SLA196H1","SLA203H1","SLA212H1","SLA224H1","SLA234H1","SLA245H1","SLA252H1","SLA253H1","SLA254H1","JSM354H1","SLA260H1","SLA263H1","SLA268H1","SLA272H1","SLA280H1","JSH300H1","SLA300H1","SLA310H1","SLA311H1","SLA312H1","SLA314H1","SLA317H1","SLA318H1","SLA320Y1","SLA321H1","SLA322H1","SLA323H1","SLA330H1","SLA331H1","SLA334H1","SLA342H1","SLA343H1","SLA345H1","SLA367H1","SLA400H1","SLA404H1","SLA412H1","SLA413H1","SLA414H1","SLA415H1","SLA420Y1","SLA423H1","SLA424Y1","SLA430H1","SLA433H1","SLA434H1","SLA449H1","SLA463H1"]},{"type":"and","items":[{"type":"credits","amount":2,"designators":["HIS","POL"],"courses":["SLA121H1","SLA194H1","SLA195H1","SLA196H1","SLA203H1","SLA212H1","SLA224H1","SLA234H1","SLA245H1","SLA252H1","SLA253H1","SLA254H1","JSM354H1","SLA260H1","SLA263H1","SLA268H1","SLA272H1","SLA280H1","JSH300H1","SLA300H1","SLA310H1","SLA311H1","SLA312H1","SLA314H1","SLA317H1","SLA318H1","SLA320Y1","SLA321H1","SLA322H1","SLA323H1","SLA330H1","SLA331H1","SLA334H1","SLA342H1","SLA343H1","SLA345H1","SLA367H1","SLA400H1","SLA404H1","SLA412H1","SLA413H1","SLA414H1","SLA415H1","SLA420Y1","SLA423H1","SLA424Y1","SLA430H1","SLA433H1","SLA434H1","SLA449H1","SLA463H1"]},{"type":"text","kind":"permission","text":"In consultation with the Undergraduate Coordinator, students may take up to 1.0 credit in history or political science courses corresponding to their two chosen language areas (0.5 credit per language area) and taught outside the department"}]}]}`, "_Russian:_ SLA121H1, SLA194H1, SLA195H1, SLA196H1, SLA203H1, SLA212H1, SLA224H1, SLA234H1, SLA245H1, SLA252H1, SLA253H1, SLA254H1/ JSM354H1, SLA260H1, SLA263H1, SLA268H1, SLA272H1, SLA280H1, JSH300H1, SLA300H1, SLA310H1, SLA311H1, SLA312H1, SLA314H1, SLA317H1, SLA318H1, SLA320Y1, SLA321H1, SLA322H1, SLA323H1, SLA330H1, SLA331H1, SLA334H1, SLA342H1, SLA343H1, SLA345H1, SLA367H1, SLA400H1, SLA404H1, SLA412H1, SLA413H1, SLA414H1, SLA415H1, SLA420Y1, SLA423H1, SLA424Y1, SLA430H1, SLA433H1, SLA434H1, SLA449H1, SLA463H1\nIn consultation with the Undergraduate Coordinator, students may take up to 1.0 credit in history or political science courses corresponding to their two chosen language areas (0.5 credit per language area) and taught outside the department"),
			jRow(26, "limit", 25, "", "", `{"type":"credits","courses":["SLA254H1","JSM354H1"],"max_amount":0.5}`, lines[20]),
			jRow(27, "limit", 25, "", "", `{"type":"credits","designators":["HIS","POL"],"max_amount":0.5}`, "In consultation with the Undergraduate Coordinator, students may take up to 1.0 credit in history or political science courses corresponding to their two chosen language areas (0.5 credit per language area) and taught outside the department"),
			jRow(28, "allocation", 1, "South Slavic", "", `null`, lines[10]+"\n"+lines[21]),
			jRow(29, "allocation", 28, "", "", `{"type":"or","items":[{"type":"credits","amount":2,"courses":["SLA107Y1","SLA117H1","SLA118H1","SLA214H1","SLA257H1","SLA277H1","SLA337H1"]},{"type":"credits","amount":2,"courses":["SLA109Y1","SLA209H1","SLA213H1"]},{"type":"and","items":[{"type":"credits","amount":2,"courses":["SLA107Y1","SLA117H1","SLA118H1","SLA214H1","SLA257H1","SLA277H1","SLA337H1","SLA109Y1","SLA209H1","SLA213H1","SLA200H1","SLA217H1","SLA227H1","SLA235H1","SLA247H1","SLA330H1","SLA347H1","SLA357H1","SLA377H1","SLA380H1","SLA232H1"]},{"type":"text","kind":"permission","text":"In consultation with the Undergraduate Coordinator, and based on results in language placement tests, some students may substitute language courses with the equivalent number of credits in requirement 3 (below)"}]}]}`, "_South Slavic (Bosnian, Croatian, Serbian, Macedonian):_ [( SLA107Y1/ SLA117H1, SLA118H1); SLA214H1; SLA257H1; SLA277H1; SLA337H1]; / [ SLA109Y1, SLA209H1, SLA213H1]\nIn consultation with the Undergraduate Coordinator, and based on results in language placement tests, some students may substitute language courses with the equivalent number of credits in requirement 3 (below)"),
			jRow(30, "limit", 29, "", "", `{"type":"credits","courses":["SLA107Y1","SLA117H1","SLA118H1"],"max_amount":1}`, "( SLA107Y1/ SLA117H1, SLA118H1)"),
			jRow(31, "allocation", 28, "", "", `{"type":"or","items":[{"type":"credits","amount":2,"courses":["SLA200H1","SLA217H1","SLA227H1","SLA235H1","SLA247H1","SLA330H1","SLA347H1","SLA357H1","SLA377H1","SLA380H1","SLA232H1"]},{"type":"and","items":[{"type":"credits","amount":2,"designators":["HIS","POL"],"courses":["SLA200H1","SLA217H1","SLA227H1","SLA235H1","SLA247H1","SLA330H1","SLA347H1","SLA357H1","SLA377H1","SLA380H1","SLA232H1"]},{"type":"text","kind":"permission","text":"In consultation with the Undergraduate Coordinator, students may take up to 1.0 credit in history or political science courses corresponding to their two chosen language areas (0.5 credit per language area) and taught outside the department"}]}]}`, "_South Slavic:_ SLA200H1, SLA217H1, SLA227H1, SLA235H1, SLA247H1, SLA330H1, SLA347H1, SLA357H1, SLA377H1, SLA380H1/ SLA232H1\nIn consultation with the Undergraduate Coordinator, students may take up to 1.0 credit in history or political science courses corresponding to their two chosen language areas (0.5 credit per language area) and taught outside the department"),
			jRow(32, "limit", 31, "", "", `{"type":"credits","courses":["SLA380H1","SLA232H1"],"max_amount":0.5}`, lines[21]),
			jRow(33, "limit", 31, "", "", `{"type":"credits","designators":["HIS","POL"],"max_amount":0.5}`, "In consultation with the Undergraduate Coordinator, students may take up to 1.0 credit in history or political science courses corresponding to their two chosen language areas (0.5 credit per language area) and taught outside the department"),
			jRow(34, "allocation", 1, "Ukrainian", "", `null`, lines[11]+"\n"+lines[22]),
			jRow(35, "allocation", 34, "", "", `{"type":"or","items":[{"type":"credits","amount":2,"courses":["SLA108Y1","SLA208Y1","SLA308H1"]},{"type":"and","items":[{"type":"credits","amount":2,"courses":["SLA108Y1","SLA208Y1","SLA308H1","SLA199H1","SLA200H1","SLA203H1","SLA218H1","SLA228H1","SLA238H1","SLA248H1","SLA253H1","SLA254H1","JSM354H1","SLA268H1","SLA311H1","SLA318H1","SLA328H1","SLA331H1","SLA338H1","SLA358H1","SLA428H1","SLA429H1"]},{"type":"text","kind":"permission","text":"In consultation with the Undergraduate Coordinator, and based on results in language placement tests, some students may substitute language courses with the equivalent number of credits in requirement 3 (below)"}]}]}`, "_Ukrainian:_ SLA108Y1, SLA208Y1, SLA308H1\nIn consultation with the Undergraduate Coordinator, and based on results in language placement tests, some students may substitute language courses with the equivalent number of credits in requirement 3 (below)"),
			jRow(36, "allocation", 34, "", "", `{"type":"or","items":[{"type":"credits","amount":2,"courses":["SLA199H1","SLA200H1","SLA203H1","SLA218H1","SLA228H1","SLA238H1","SLA248H1","SLA253H1","SLA254H1","JSM354H1","SLA268H1","SLA311H1","SLA318H1","SLA328H1","SLA331H1","SLA338H1","SLA358H1","SLA428H1","SLA429H1"]},{"type":"and","items":[{"type":"credits","amount":2,"designators":["HIS","POL"],"courses":["SLA199H1","SLA200H1","SLA203H1","SLA218H1","SLA228H1","SLA238H1","SLA248H1","SLA253H1","SLA254H1","JSM354H1","SLA268H1","SLA311H1","SLA318H1","SLA328H1","SLA331H1","SLA338H1","SLA358H1","SLA428H1","SLA429H1"]},{"type":"text","kind":"permission","text":"In consultation with the Undergraduate Coordinator, students may take up to 1.0 credit in history or political science courses corresponding to their two chosen language areas (0.5 credit per language area) and taught outside the department"}]}]}`, "_Ukrainian:_ SLA199H1, SLA200H1, SLA203H1, SLA218H1, SLA228H1, SLA238H1, SLA248H1, SLA253H1, SLA254H1/ JSM354H1, SLA268H1, SLA311H1, SLA318H1, SLA328H1, SLA331H1, SLA338H1, SLA358H1, SLA428H1, SLA429H1\nIn consultation with the Undergraduate Coordinator, students may take up to 1.0 credit in history or political science courses corresponding to their two chosen language areas (0.5 credit per language area) and taught outside the department"),
			jRow(37, "limit", 36, "", "", `{"type":"credits","courses":["SLA254H1","JSM354H1"],"max_amount":0.5}`, lines[22]),
			jRow(38, "limit", 36, "", "", `{"type":"credits","designators":["HIS","POL"],"max_amount":0.5}`, "In consultation with the Undergraduate Coordinator, students may take up to 1.0 credit in history or political science courses corresponding to their two chosen language areas (0.5 credit per language area) and taught outside the department"),
			jRow(39, "allocation", 1, "Yiddish", "", `null`, lines[12]+"\n"+lines[18]),
			jRow(40, "allocation", 39, "", "", `{"type":"or","items":[{"type":"credits","amount":2,"courses":["GER261H1","GER262H1","GER360H1","GER460H1"]},{"type":"and","items":[{"type":"credits","amount":2,"courses":["GER261H1","GER262H1","GER360H1","GER460H1","GER361H1","JJH370H1","SLA202H1","SLA224H1","SLA268H1","SLA302H1","SLA303H1","SLA318H1"]},{"type":"text","kind":"permission","text":"In consultation with the Undergraduate Coordinator, and based on results in language placement tests, some students may substitute language courses with the equivalent number of credits in requirement 3 (below)"}]}]}`, "_Yiddish (corresponds to the Jewish Cultures in Eastern Europe area):_ GER261H1, GER262H1, GER360H1, GER460H1\nIn consultation with the Undergraduate Coordinator, and based on results in language placement tests, some students may substitute language courses with the equivalent number of credits in requirement 3 (below)"),
			jRow(41, "allocation", 39, "", "", `{"type":"or","items":[{"type":"credits","amount":2,"courses":["GER361H1","JJH370H1","SLA202H1","SLA224H1","SLA268H1","SLA302H1","SLA303H1","SLA318H1"]},{"type":"and","items":[{"type":"credits","amount":2,"designators":["HIS","POL"],"courses":["GER361H1","JJH370H1","SLA202H1","SLA224H1","SLA268H1","SLA302H1","SLA303H1","SLA318H1"]},{"type":"text","kind":"permission","text":"In consultation with the Undergraduate Coordinator, students may take up to 1.0 credit in history or political science courses corresponding to their two chosen language areas (0.5 credit per language area) and taught outside the department"}]}]}`, "_Jewish Cultures in Eastern Europe (corresponds to the Yiddish language area):_ GER361H1; JJH370H1; SLA202H1; SLA224H1; SLA268H1; SLA302H1; SLA303H1; SLA318H1\nIn consultation with the Undergraduate Coordinator, students may take up to 1.0 credit in history or political science courses corresponding to their two chosen language areas (0.5 credit per language area) and taught outside the department"),
			jRow(42, "limit", 41, "", "", `{"type":"credits","designators":["HIS","POL"],"max_amount":0.5}`, "In consultation with the Undergraduate Coordinator, students may take up to 1.0 credit in history or political science courses corresponding to their two chosen language areas (0.5 credit per language area) and taught outside the department"),
			jRow(43, "choice", 0, "", "", `{"type":"choose","items":[{"type":"requirement","requirements":[5]},{"type":"requirement","requirements":[10]},{"type":"requirement","requirements":[15]},{"type":"requirement","requirements":[19]},{"type":"requirement","requirements":[23]},{"type":"requirement","requirements":[28]},{"type":"requirement","requirements":[34]},{"type":"requirement","requirements":[39]}],"count":2}`, "Students must choose any two of the available language areas (Czech and Slovak, Estonian, Finnish, Polish, Russian, South Slavic, Ukrainian)\n2. 4.0 credits of language coursework from each of the two chosen language areas (2.0 credits per area).\n3. 4.0 credits from culture courses corresponding to the student’s two chosen language areas (2.0 credits per area):"),
			jRow(44, "allocation", 1, "4", "", `{"type":"credits","amount":1,"courses":["EST205H1","EST206H1","EST207H1","FIN225H1","FIN238H1","FIN240H1","FIN260H1","SLA104H1","SLA190H1","SLA191H1","SLA192H1","SLA200H1","SLA201H1","SLA202H1","SLA203H1","SLA222H1","SLA229H1","SLA231H1","SLA232H1","SLA235H1","SLA254H1","JSM354H1","SLA256H1","SLA268H1","SLA272H1","SLA286H1","SLA302H1","SLA303H1","SLA318H1","SLA330H1","SLA331H1","SLA333H1","SLA340H1","SLA380H1","SLA425H1","SLA435H1","SLA492H1"]}`, lines[24]),
			jRow(45, "limit", 44, "", "", `{"type":"credits","courses":["SLA254H1","JSM354H1"],"max_amount":0.5}`, lines[24]),
			jRow(46, "allocation", 1, "5", "", `{"type":"or","items":[{"type":"credits","amount":0.5,"breadth":[5]},{"type":"text","kind":"permission","text":"another 0.5 credit approved by the Undergraduate Coordinator, to fulfill the Quantitative Reasoning Competency required in the program"}]}`, lines[25]),
		}, `"notes":`+jStrs("Please note: Some courses may appear in more than one group thanks to their content", "No course, however, may be counted twice for the purposes of program completion"))
	}
	{
		raw := tests[8].raw
		lines := strings.Split(raw, "\n")
		want["ASMAJ2015"] = jComp(raw, []string{
			jRow(1, "allocation", 0, "", "", `{"type":"credits","amount":7,"designators":["POL","JPA","JPF","JPI","JPR","JPS","JRA"],"courses":["AFR353H1","AFR455H1","CAR226H1","CAR324H1","CAR426H1","CRI390H1","CRI393H1","DTS314H1","EAS307H1","GER275H1","GER290H1","GGR324H1","GGR339H1","GGR344H1","GGR429H1","MUN100H1","MUN110H1","MUN130H1","PCJ260H1","PCJ261H1","RLG434H1","TRN136Y1","TRN151Y1","TRN152Y1","TRN160Y1","TRN162Y1","TRN171Y1","TRN172Y1","TRN192H1","TRN193H1","TRN199H1","URB339H1","INI339H1","VIC110H1","VIC121H1","VIC167H1","VIC168H1","VIC181H1","VIC183H1","VIC184H1","VIC185H1","VIC476H1","WDW151H1","WDW152H1"]}`, "(7.0 credits)\n7. Additional POL/ JPA/ JPF/ JPI/ JPR/ JPS/ JRA courses to a total of 7.0 credits.\n- No more than 1.0 credit of courses from outside of the Department of Political Science may be used to fulfill the program requirements\nNon-POL courses that may be used to fulfill the program requirements: AFR353H1, AFR455H1, CAR226H1, CAR324H1, CAR426H1, CRI390H1, CRI393H1, DTS314H1, EAS307H1, GER275H1, GER290H1, GGR324H1, GGR339H1, GGR344H1, GGR429H1, MUN100H1, MUN110H1, MUN130H1, PCJ260H1, PCJ261H1, RLG434H1, TRN136Y1, TRN151Y1, TRN152Y1, TRN160Y1, TRN162Y1, TRN171Y1, TRN172Y1, TRN192H1, TRN193H1, TRN199H1, URB339H1/ INI339H1, VIC110H1, VIC121H1, VIC167H1, VIC168H1, VIC181H1, VIC183H1, VIC184H1, VIC185H1, VIC476H1, WDW151H1, WDW152H1"),
			jRow(2, "constraint", 1, "", "", `{"type":"credits","amount":2,"levels":[300,400]}`, "2.0 credits at the 300+ level including a 0.5 credit at the 400 level"),
			jRow(3, "constraint", 1, "", "", `{"type":"credits","amount":0.5,"levels":[400]}`, "2.0 credits at the 300+ level including a 0.5 credit at the 400 level"),
			jRow(4, "limit", 1, "", "", `{"type":"credits","levels":[100],"max_amount":1}`, "No more than 1.0 credit of 100-level courses may be used to fulfill the program requirements"),
			jRow(5, "allocation", 1, "", "First Year", `{"type":"credits","amount":1,"courses":["POL101H1","POL106H1","POL107H1","POL109H1","POL191H1","POL192H1","POL193H1","POL195H1","POL196H1","POL197H1","POL198H1","MUN100H1","MUN110H1","MUN130H1","TRN136Y1","TRN151Y1","TRN152Y1","TRN160Y1","TRN162Y1","TRN171Y1","TRN172Y1","TRN192H1","TRN193H1","TRN199H1","VIC110H1","VIC121H1","VIC167H1","VIC168H1","VIC181H1","VIC183H1","VIC184H1","VIC185H1","WDW151H1","WDW152H1"]}`, lines[6]),
			jRow(6, "limit", 5, "", "", `{"type":"credits","courses":["POL191H1","POL192H1","POL193H1","POL195H1","POL196H1","POL197H1","POL198H1","MUN100H1","MUN110H1","MUN130H1","TRN136Y1","TRN151Y1","TRN152Y1","TRN160Y1","TRN162Y1","TRN171Y1","TRN172Y1","TRN192H1","TRN193H1","TRN199H1","VIC110H1","VIC121H1","VIC167H1","VIC168H1","VIC181H1","VIC183H1","VIC184H1","VIC185H1","WDW151H1","WDW152H1"],"max_amount":0.5}`, "A maximum of 0.5 credit may be substituted from the following first-year courses: POL191H1, POL192H1, POL193H1, POL195H1, POL196H1, POL197H1, POL198H1, MUN100H1, MUN110H1, MUN130H1, TRN136Y1, TRN151Y1, TRN152Y1, TRN160Y1, TRN162Y1, TRN171Y1, TRN172Y1, TRN192H1, TRN193H1, TRN199H1, VIC110H1, VIC121H1, VIC167H1, VIC168H1, VIC181H1, VIC183H1, VIC184H1, VIC185H1, WDW151H1, WDW152H1"),
			jRow(7, "allocation", 1, "Higher Years 1", "Higher Years", `{"type":"course","code":"POL200Y1"}`, lines[8]),
			jRow(8, "allocation", 1, "Higher Years 2", "Higher Years", `{"type":"credits","amount":0.5,"courses":["POL201H1","POL208H1","POL218H1","POL219H1"]}`, lines[9]),
			jRow(9, "allocation", 1, "Higher Years 3", "Higher Years", `{"type":"or","items":[{"type":"course","code":"POL214H1"},{"type":"course","code":"POL224H1"}]}`, lines[10]),
			jRow(10, "allocation", 1, "Higher Years 4", "Higher Years", `{"type":"or","items":[{"type":"credits","amount":0.5,"courses":["JPI201H1","POL220H1","POL312H1","POL314H1","POL316H1","POL319H1","POL334H1","POL336H1","POL337H1","POL344H1","POL351H1","POL356H1","POL363H1","POL365H1","POL382H1","POL428H1","POL439H1","POL467H1","POL470H1","POL474H1","POL490H1","POL491H1"]},{"type":"and","items":[{"type":"credits","amount":0.5,"designators":["POL"],"courses":["JPI201H1","POL220H1","POL312H1","POL314H1","POL316H1","POL319H1","POL334H1","POL336H1","POL337H1","POL344H1","POL351H1","POL356H1","POL363H1","POL365H1","POL382H1","POL428H1","POL439H1","POL467H1","POL470H1","POL474H1","POL490H1","POL491H1"]},{"type":"text","kind":"permission","text":"Some offerings of other POL Special Topics courses that cover Canadian politics may also be eligible as determined by the department"}]}]}`, "4. 0.5 credit at the 200+ level in Canadian Politics from JPI201H1, POL220H1, POL312H1, POL314H1, POL316H1, POL319H1, POL334H1, POL336H1, POL337H1, POL344H1, POL351H1, POL356H1, POL363H1, POL365H1, POL382H1, POL428H1, POL439H1, POL467H1, POL470H1, POL474H1, POL490H1, POL491H1\nSome offerings of other POL Special Topics courses that cover Canadian politics may also be eligible as determined by the department\nSome courses may contribute to both the Canadian Politics and Diversity and Identity requirements (in requirements 4 and 6)", `"notes":`+jStrs("Please see Department’s website https://politics.utoronto.ca/undergraduate/courses/canadian-politics-co… for a full listing of Canadian politics courses"), `"overlaps":[12]`),
			jRow(11, "allocation", 1, "Higher Years 5", "Higher Years", `{"type":"or","items":[{"type":"course","code":"POL222H1"},{"type":"course","code":"ECO220Y1"},{"type":"course","code":"PSY201H1"},{"type":"course","code":"SOC202H1"},{"type":"course","code":"STA220H1"}]}`, "5. POL222H1\n(or one of ECO220Y1, PSY201H1, SOC202H1, STA220H1)"),
			jRow(12, "allocation", 1, "Higher Years 6", "Higher Years", `{"type":"or","items":[{"type":"credits","amount":1,"courses":["POL101H1","POL195H1","POL197H1","POL198H1","JPI201H1","POL219H1","POL220H1","POL223H1","POL224H1","POL301H1","POL303H1","POL305H1","POL309H1","POL310H1","JPS315H1","JPS316H1","POL325H1","POL329H1","POL331H1","POL333H1","POL338H1","POL344H1","POL348H1","POL351H1","POL358H1","POL360H1","POL365H1","JPR364H1","JPR365H1","JPR374H1","JPS378H1","POL387H1","POL418H1","POL428H1","POL432H1","POL442H1","POL444H1","POL447H1","POL453H1","JPR458H1","JPR459H1","POL467H1","POL469H1"]},{"type":"and","items":[{"type":"credits","amount":1,"designators":["POL"],"courses":["POL101H1","POL195H1","POL197H1","POL198H1","JPI201H1","POL219H1","POL220H1","POL223H1","POL224H1","POL301H1","POL303H1","POL305H1","POL309H1","POL310H1","JPS315H1","JPS316H1","POL325H1","POL329H1","POL331H1","POL333H1","POL338H1","POL344H1","POL348H1","POL351H1","POL358H1","POL360H1","POL365H1","JPR364H1","JPR365H1","JPR374H1","JPS378H1","POL387H1","POL418H1","POL428H1","POL432H1","POL442H1","POL444H1","POL447H1","POL453H1","JPR458H1","JPR459H1","POL467H1","POL469H1"]},{"type":"text","kind":"permission","text":"Some offerings of other POL Special Topics courses may also be eligible as determined by the department"}]}]}`, "6. 1.0 credit in Diversity and Identity. The following courses fulfill the Diversity and Identity requirement: POL101H1, POL195H1, POL197H1, POL198H1, JPI201H1, POL219H1, POL220H1, POL223H1, POL224H1, POL301H1, POL303H1, POL305H1, POL309H1, POL310H1, JPS315H1, JPS316H1, POL325H1, POL329H1, POL331H1, POL333H1, POL338H1, POL344H1, POL348H1, POL351H1, POL358H1, POL360H1, POL365H1, JPR364H1, JPR365H1, JPR374H1, JPS378H1, POL387H1, POL418H1, POL428H1, POL432H1, POL442H1, POL444H1, POL447H1, POL453H1, JPR458H1, JPR459H1, POL467H1, POL469H1.\nSome offerings of other POL Special Topics courses may also be eligible as determined by the department\nSome courses may contribute to both the Canadian Politics and Diversity and Identity requirements (in requirements 4 and 6)", `"notes":`+jStrs("Please see Department’s website https://politics.utoronto.ca/undergraduate/courses/diversity-identity-p… for a full listing of Diversity and Identity courses"), `"overlaps":[10]`),
			jRow(13, "limit", 1, "", "", `{"type":"credits","courses":["ECO220Y1"],"max_amount":0.5}`, "Please note only 0.5 of ECO220Y1 will be accepted towards the program"),
			jRow(14, "limit", 1, "", "", `{"type":"credits","courses":["AFR353H1","AFR455H1","CAR226H1","CAR324H1","CAR426H1","CRI390H1","CRI393H1","DTS314H1","EAS307H1","GER275H1","GER290H1","GGR324H1","GGR339H1","GGR344H1","GGR429H1","MUN100H1","MUN110H1","MUN130H1","PCJ260H1","PCJ261H1","RLG434H1","TRN136Y1","TRN151Y1","TRN152Y1","TRN160Y1","TRN162Y1","TRN171Y1","TRN172Y1","TRN192H1","TRN193H1","TRN199H1","URB339H1","INI339H1","VIC110H1","VIC121H1","VIC167H1","VIC168H1","VIC181H1","VIC183H1","VIC184H1","VIC185H1","VIC476H1","WDW151H1","WDW152H1"],"max_amount":1}`, "- No more than 1.0 credit of courses from outside of the Department of Political Science may be used to fulfill the program requirements"),
		}, `"notes":`+jStrs("See the bottom of the completion requirements section for a full list of such acceptable courses", "Students in this program have the option to complete the Arts & Science Internship Program (ASIP) stream", "Refer to the Department website for a full list of courses that meet these requirements"))
	}
	{
		raw := tests[9].raw
		lines := strings.Split(raw, "\n")
		want["ASMAJ0728"] = jComp(raw, []string{
			jRow(1, "allocation", 0, "", "", `{"type":"or","items":[{"type":"credits","amount":7},{"type":"text","kind":"other","text":"(7.0 credits, of which at least 2.5 must be at the 300+ level, including at least 0.5 credit at the 400-level)"}]}`, lines[0]),
			jRow(2, "constraint", 1, "", "", `{"type":"or","items":[{"type":"credits","amount":2.5,"levels":[300,400]},{"type":"text","kind":"other","text":"of which at least 2.5 must be at the 300+ level"}]}`, "of which at least 2.5 must be at the 300+ level"),
			jRow(3, "constraint", 1, "", "", `{"type":"or","items":[{"type":"credits","amount":0.5,"levels":[400]},{"type":"text","kind":"other","text":"including at least 0.5 credit at the 400-level"}]}`, "including at least 0.5 credit at the 400-level"),
			jRow(4, "allocation", 1, "1", "", `{"type":"credits","amount":1,"courses":["CDN265H1","CDN267H1","CDN268H1"]}`, lines[2]),
			jRow(5, "allocation", 1, "2", "", `{"type":"course","code":"INS200H1"}`, lines[3]),
			jRow(6, "allocation", 1, "3", "", `{"type":"and","items":[{"type":"credits","amount":0.5,"levels":[200]},{"type":"text","kind":"content","text":"offered by the Canadian Studies program"}]}`, lines[4]),
			jRow(7, "allocation", 1, "4", "", `{"type":"credits","amount":0.5,"courses":["CDN307H1","CDN450H1","CDN455H1"]}`, lines[5]),
			jRow(8, "allocation", 1, "5", "", `{"type":"credits","amount":0.5,"courses":["CDN335H1","CDN440H1"]}`, lines[6]),
			jRow(9, "allocation", 1, "6", "", `{"type":"and","items":[{"type":"credits","amount":1.5,"levels":[300,400]},{"type":"text","kind":"content","text":"offered by the Canadian Studies program"}]}`, lines[7]),
			jRow(10, "allocation", 1, "7", "", `{"type":"or","items":[{"type":"credits","amount":2.5,"lists":["Requirement 7 courses"]},{"type":"text","kind":"permission","text":"Additional courses with significant Canadian content may be considered for approval for Requirement 7 by the Program Director (canadian.studies@utoronto.ca)"}]}`, "7. An additional 2.5 credits from Requirements 1, 3, 4, 5, or 6, with up to 1.0 credit from ENG215H1, ENG252H1, ENG254H1, FAH248H1, FOR200H1, FRE210H1, GGR240H1, GGR246H1, HIS262H1, HIS264H1, INS231H1, LIN201H1, POL214Y1, SLA238H1, SOC218H1, SOC220H1, TRN235H1.\nAdditional courses with significant Canadian content may be considered for approval for Requirement 7 by the Program Director (canadian.studies@utoronto.ca)"),
			jRow(11, "limit", 10, "", "", `{"type":"credits","courses":["ENG215H1","ENG252H1","ENG254H1","FAH248H1","FOR200H1","FRE210H1","GGR240H1","GGR246H1","HIS262H1","HIS264H1","INS231H1","LIN201H1","POL214Y1","SLA238H1","SOC218H1","SOC220H1","TRN235H1"],"max_amount":1}`, "An additional 2.5 credits from Requirements 1, 3, 4, 5, or 6, with up to 1.0 credit from ENG215H1, ENG252H1, ENG254H1, FAH248H1, FOR200H1, FRE210H1, GGR240H1, GGR246H1, HIS262H1, HIS264H1, INS231H1, LIN201H1, POL214Y1, SLA238H1, SOC218H1, SOC220H1, TRN235H1"),
		}, `"lists":[{"name":"Requirement 7 courses","items":[{"type":"credits","requirements":[4,7,8]},{"type":"credits","courses":["ENG215H1","ENG252H1","ENG254H1","FAH248H1","FOR200H1","FRE210H1","GGR240H1","GGR246H1","HIS262H1","HIS264H1","INS231H1","LIN201H1","POL214Y1","SLA238H1","SOC218H1","SOC220H1","TRN235H1"]},{"type":"and","items":[{"type":"credits","requirements":[6,9]},{"type":"text","kind":"other","text":"the courses of requirements 3 and 6, which are partly text"}]}]}]`, `"notes":`+jStrs("First Year: No specific first-year requirements", "Please note that not all courses are offered on a regular basis", "Students are responsible for checking co- and prerequisites for all courses"))
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			assertJSON(t, ParseCompletion(tt.raw), want[tt.code])
		})
	}
}

// TestParseCompletionRules checks one assembler rule per case on real snippets
// (shortened where marked). Shortened items rarely add up to the total, so Req 1
// is then or(credits, the header as text), Review unless the claims reach it.
func TestParseCompletionRules(t *testing.T) {
	tests := []struct {
		name, raw string
		rows      []string
		fields    []string
	}{
		{"header: the total is Req 1 with its own pool; floors are constraints under it (ASMIN1646, shortened)",
			"4.0 ENG credits as follows, including at least 1.5 credits at the 300 level or above\n1. 0.5 credit from ENG289H1",
			[]string{
				jRow(1, "allocation", 0, "", "", `{"type":"credits","amount":4,"designators":["ENG"]}`,
					"4.0 ENG credits as follows, including at least 1.5 credits at the 300 level or above"),
				jRow(2, "constraint", 1, "", "", `{"type":"credits","amount":1.5,"designators":["ENG"],"levels":[300,400]}`,
					"including at least 1.5 credits at the 300 level or above"),
				jRow(3, "allocation", 1, "1", "", `{"type":"credits","amount":0.5,"courses":["ENG289H1"]}`, "1. 0.5 credit from ENG289H1"),
			}, nil},
		{"no total: items are at the top, with their year as section (ASMAJ1068, shortened)",
			"First Year:\n( BIO120H1, BIO130H1); ( CHM135H1, CHM136H1)/ CHM151Y1\nFourth Year:\nAny three of: NFS400H1, NFS481H1, NFS482H1, NFS484H1",
			[]string{
				jRow(1, "allocation", 0, "", "First Year", jAnd(jC("BIO120H1"), jC("BIO130H1"), jOr(jAnd(jC("CHM135H1"), jC("CHM136H1")), jC("CHM151Y1"))),
					"( BIO120H1, BIO130H1); ( CHM135H1, CHM136H1)/ CHM151Y1"),
				jRow(2, "allocation", 0, "", "Fourth Year", jChoose(3, jC("NFS400H1"), jC("NFS481H1"), jC("NFS482H1"), jC("NFS484H1")),
					"Any three of: NFS400H1, NFS481H1, NFS482H1, NFS484H1"),
			}, nil},
		{"parts: one part is the item's body; parts that can share a course are a group (ASMIN1423, shortened)",
			"(4 full courses or their equivalent)\n1. Year 1: Introductory Physics\n  - 1 credit from ( PHY131H1/ PHY151H1), ( PHY132H1/ PHY152H1)\n3. Year 3: Physical Sciences\n  - 0.5 credit from PHY252H1 (recommended), PHY224H1, ESS262H1\n  - 0.5 credit from from PHY252H1, CHM210H1, ESS262H1",
			[]string{
				jRow(1, "allocation", 0, "", "", jOr(jCr(4), jT("other", "(4 full courses or their equivalent)")), "(4 full courses or their equivalent)"),
				jRow(2, "allocation", 1, "1", "", jAnd(jOr(jC("PHY131H1"), jC("PHY151H1")), jOr(jC("PHY132H1"), jC("PHY152H1"))),
					"1. Year 1: Introductory Physics\n- 1 credit from ( PHY131H1/ PHY151H1), ( PHY132H1/ PHY152H1)"),
				jRow(3, "allocation", 1, "3", "", "null", "3. Year 3: Physical Sciences"),
				jRow(4, "allocation", 3, "", "", `{"type":"credits","amount":0.5,"courses":["PHY252H1","PHY224H1","ESS262H1"]}`,
					"- 0.5 credit from PHY252H1 (recommended), PHY224H1, ESS262H1"),
				jRow(5, "allocation", 3, "", "", `{"type":"credits","amount":0.5,"courses":["PHY252H1","CHM210H1","ESS262H1"]}`,
					"- 0.5 credit from from PHY252H1, CHM210H1, ESS262H1"),
			}, nil},
		{"parts: alternatives under \"either\" (ASMIN1689, shortened)",
			"(4.0 credits with at least 1.0 credit at the 300+ level)\n1. Completion of either\n- CSC108H1, CSC148H1, CSC165H1 or\n- CSC110Y1, CSC111H1",
			[]string{
				jRow(1, "allocation", 0, "", "", jOr(jCr(4), jT("other", "(4.0 credits with at least 1.0 credit at the 300+ level)")), "(4.0 credits with at least 1.0 credit at the 300+ level)"),
				jRow(2, "constraint", 1, "", "", jOr(`{"type":"credits","amount":1,"levels":[300,400]}`, jT("other", "with at least 1.0 credit at the 300+ level")), "with at least 1.0 credit at the 300+ level"),
				jRow(3, "allocation", 1, "1", "", jOr(jAnd(jC("CSC108H1"), jC("CSC148H1"), jC("CSC165H1")), jAnd(jC("CSC110Y1"), jC("CSC111H1"))),
					"1. Completion of either\n- CSC108H1, CSC148H1, CSC165H1 or\n- CSC110Y1, CSC111H1"),
			}, nil},
		{"notes: advice in an item is its note; advice outside items is the program's (ASMAJ0365, shortened)",
			"(8.0 credits)\n2. ( CHM135H1 and CHM136H1)/ CHM151Y1 (Only transfer credits that carry exclusions to CHM136H1 and/or CHM135H1 will be accepted)\nStudents in this program have the option to complete the Arts & Science Internship Program (ASIP) stream.",
			[]string{
				jRow(1, "allocation", 0, "", "", jOr(jCr(8), jT("other", "(8.0 credits)")), "(8.0 credits)"),
				jRow(2, "allocation", 1, "2", "", jOr(jAnd(jC("CHM135H1"), jC("CHM136H1")), jC("CHM151Y1")),
					"2. ( CHM135H1 and CHM136H1)/ CHM151Y1 (Only transfer credits that carry exclusions to CHM136H1 and/or CHM135H1 will be accepted)",
					`"notes":`+jStrs("Only transfer credits that carry exclusions to CHM136H1 and/or CHM135H1 will be accepted")),
			}, []string{`"notes":` + jStrs("Students in this program have the option to complete the Arts & Science Internship Program (ASIP) stream")}},
		{"exclusions: programs that can't be combined, read as in enrolment (ASMIN1689, shortened)",
			"(4.0 credits)\n1. CSC207H1\n- Students cannot combine the Computer Science Minor (ASMIN1689) program with the Computer Science Major (ASMAJ1689) or the Computer Science Specialist (ASSPE1689) for their degree.",
			[]string{
				jRow(1, "allocation", 0, "", "", jOr(jCr(4), jT("other", "(4.0 credits)")), "(4.0 credits)"),
				jRow(2, "allocation", 1, "1", "", jC("CSC207H1"), "1. CSC207H1"),
			}, []string{`"exclusions":[` + jP("Computer Science Major (ASMAJ1689)", "ASMAJ1689") + "," + jP("Computer Science Specialist (ASSPE1689)", "ASSPE1689") + "]"}},
		{"numbering: the total and its floors, each item and its own rows, then the program's (ASMAJ1003 and ASCER1200, shortened)",
			"(8.0 credits, including at least 2.0 credits at the 300+ level)\n1. 1.5 credits (at least 0.5 credit at the 400-level) from: BCH422H1, CSB327H1, CSB328H1, CSB427H1. No more than 0.5 credit in BCH can be used towards this requirement.\n2. CSB349H1\nNotes:\n- No more than 1.0 transfer credit will be eligible for consideration in the Certificate.",
			[]string{
				jRow(1, "allocation", 0, "", "", jOr(jCr(8), jT("other", "(8.0 credits, including at least 2.0 credits at the 300+ level)")), "(8.0 credits, including at least 2.0 credits at the 300+ level)"),
				jRow(2, "constraint", 1, "", "", jOr(`{"type":"credits","amount":2,"levels":[300,400]}`, jT("other", "including at least 2.0 credits at the 300+ level")), "including at least 2.0 credits at the 300+ level"),
				jRow(3, "allocation", 1, "1", "", `{"type":"credits","amount":1.5,"courses":["BCH422H1","CSB327H1","CSB328H1","CSB427H1"]}`,
					"1. 1.5 credits (at least 0.5 credit at the 400-level) from: BCH422H1, CSB327H1, CSB328H1, CSB427H1. No more than 0.5 credit in BCH can be used towards this requirement."),
				jRow(4, "constraint", 3, "", "", `{"type":"credits","amount":0.5,"levels":[400]}`, "at least 0.5 credit at the 400-level"),
				jRow(5, "limit", 3, "", "", `{"type":"credits","designators":["BCH"],"max_amount":0.5}`, "No more than 0.5 credit in BCH can be used towards this requirement"),
				jRow(6, "allocation", 1, "2", "", jC("CSB349H1"), "2. CSB349H1"),
				jRow(7, "limit", 1, "", "", `{"type":"credits","transfer":true,"max_amount":1}`,
					"No more than 1.0 transfer credit will be eligible for consideration in the Certificate"),
			}, nil},
		{"\"Additional ... to a total of\": Req 1's pool, and a sub-total (ASMAJ2543, shortened)",
			"(8.0 credits)\n1. CHM210H1, CHM217H1\n2. Additional 200/300/400-level CHM courses to a total of 7.0 CHM credits ( CHM299H1, CHM299Y1 excluded)",
			[]string{
				jRow(1, "allocation", 0, "", "", `{"type":"credits","amount":8,"designators":["CHM"],"levels":[200,300,400],"exclude":["CHM299H1","CHM299Y1"]}`,
					"(8.0 credits)\n2. Additional 200/300/400-level CHM courses to a total of 7.0 CHM credits ( CHM299H1, CHM299Y1 excluded)"),
				jRow(2, "allocation", 1, "1", "", jAnd(jC("CHM210H1"), jC("CHM217H1")), "1. CHM210H1, CHM217H1"),
				jRow(3, "constraint", 1, "", "", `{"type":"credits","amount":7,"designators":["CHM"]}`,
					"2. Additional 200/300/400-level CHM courses to a total of 7.0 CHM credits ( CHM299H1, CHM299Y1 excluded)"),
			}, nil},
		{"\"of which\" in an item: an allocation and a constraint, never an in-node and (ASMIN1013, shortened)",
			"(4.0 credits in Sociology)\n2. 3.0 SOC credits, which includes at least 1.0 SOC credit at the 300-level",
			[]string{
				jRow(1, "allocation", 0, "", "", `{"type":"credits","amount":4,"designators":["SOC"]}`, "(4.0 credits in Sociology)"),
				jRow(2, "allocation", 1, "2", "", `{"type":"credits","amount":3,"designators":["SOC"]}`,
					"2. 3.0 SOC credits, which includes at least 1.0 SOC credit at the 300-level"),
				jRow(3, "constraint", 2, "", "", `{"type":"credits","amount":1,"designators":["SOC"],"levels":[300]}`,
					"2. 3.0 SOC credits, which includes at least 1.0 SOC credit at the 300-level"),
			}, nil},
		{"approval: another way to meet the item, which a person grants (ASCER1469, shortened)",
			"(Total: 2.5 credits)\n1. 1.0 credit from EAS100Y1, EST100H1, EST101H1. Other introductory language classes may be approved by the Director of the International Relations Program.",
			[]string{
				jRow(1, "allocation", 0, "", "", jOr(jCr(2.5), jT("permission", "(Total: 2.5 credits)")), "(Total: 2.5 credits)"),
				jRow(2, "allocation", 1, "1", "",
					jOr(`{"type":"credits","amount":1,"courses":["EAS100Y1","EST100H1","EST101H1"]}`,
						jT("permission", "Other introductory language classes may be approved by the Director of the International Relations Program")),
					"1. 1.0 credit from EAS100Y1, EST100H1, EST101H1. Other introductory language classes may be approved by the Director of the International Relations Program."),
			}, nil},
		{"a rule the grammar can't read stays, as a row of text (ASMAJ0525, shortened)",
			"(7.0 credits)\n1. LIN101H1\nRegardless of exemptions, a maximum of 1.0 total program credits may be drawn from courses taught in English.",
			[]string{
				jRow(1, "allocation", 0, "", "", jOr(jCr(7), jT("other", "(7.0 credits)")), "(7.0 credits)"),
				jRow(2, "allocation", 1, "1", "", jC("LIN101H1"), "1. LIN101H1"),
				jRow(3, "constraint", 1, "", "", jT("content", "Regardless of exemptions, a maximum of 1.0 total program credits may be drawn from courses taught in English"),
					"Regardless of exemptions, a maximum of 1.0 total program credits may be drawn from courses taught in English"),
			}, nil},
		{"a named list an item draws on is a list, and the item draws on it by name (ASMAJ0826, shortened)",
			"4. At least 0.5 credit from Group B\n**Group B:** PSY201H1/ PSY202H1/ SOC202H1/ CRI350H1/ POL222H1",
			[]string{
				jRow(1, "allocation", 0, "4", "", `{"type":"credits","amount":0.5,"lists":["Group B"]}`, "4. At least 0.5 credit from Group B"),
			}, []string{`"lists":[{"name":"Group B","items":[{"type":"credits","courses":["PSY201H1","PSY202H1","SOC202H1","CRI350H1","POL222H1"]}]}]`}},
		{"a list nothing refers to is a note (ASMAJ0826, shortened)",
			"4. At least 0.5 credit from PSY201H1/ PSY202H1/ SOC202H1\n**Group B:** PSY201H1/ PSY202H1/ SOC202H1/ CRI350H1/ POL222H1",
			[]string{
				jRow(1, "allocation", 0, "4", "", `{"type":"credits","amount":0.5,"courses":["PSY201H1","PSY202H1","SOC202H1"]}`, "4. At least 0.5 credit from PSY201H1/ PSY202H1/ SOC202H1"),
			}, []string{`"notes":` + jStrs("Group B: PSY201H1/ PSY202H1/ SOC202H1/ CRI350H1/ POL222H1")}},
		{"a reference to a list the program doesn't define is text (ASMAJ0826, shortened)",
			"4. At least 0.5 credit from Group B",
			[]string{
				jRow(1, "allocation", 0, "4", "", jT("other", "At least 0.5 credit from Group B"), "4. At least 0.5 credit from Group B"),
			}, nil},
		{"a total with nothing to count is Review (ASMIN0338, shortened)",
			"(4.0 credits, including 1.0 credit at the 300+ level)\nStudents in this program have the option to complete the Arts & Science Internship Program (ASIP) stream.",
			[]string{
				jRow(1, "allocation", 0, "", "", jOr(jCr(4), jT("other", "(4.0 credits, including 1.0 credit at the 300+ level)")),
					"(4.0 credits, including 1.0 credit at the 300+ level)"),
				jRow(2, "constraint", 1, "", "", jOr(`{"type":"credits","amount":1,"levels":[300,400]}`, jT("other", "including 1.0 credit at the 300+ level")), "including 1.0 credit at the 300+ level"),
			}, []string{`"notes":` + jStrs("Students in this program have the option to complete the Arts & Science Internship Program (ASIP) stream")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertJSON(t, ParseCompletion(tt.raw), jComp(tt.raw, tt.rows, tt.fields...))
		})
	}
}

// The envelope round-trips through JSON: optional slices are nil, never
// empty, and requirements is always present.
func TestParseCompletionRoundTrip(t *testing.T) {
	for _, raw := range []string{"(8.0 credits)\n1. CSB349H1", "", "Students in this program have the option to complete the Arts & Science Internship Program (ASIP) stream."} {
		c := ParseCompletion(raw)
		b, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		var back types.Completion
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(*c, back) {
			t.Errorf("%q: round trip changed the envelope:\n%s", raw, b)
		}
		if c.Requirements == nil {
			t.Errorf("%q: requirements is nil", raw)
		}
	}
}
