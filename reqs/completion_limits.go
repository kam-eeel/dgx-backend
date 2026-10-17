package reqs

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Transfer and CR/NCR rules the limit grammar of completion_rules.go
// (transfer, crNcr) doesn't read on its own (design 3.5, Task 8).

var (
	// complCrNcrCodesBanRe: courses and CR/NCR courses that don't count:
	// "We do not accept ENG100H1, ENG102H1, or any CR/NCR courses toward our
	// programs", "ENG100H1, ENG102H1, and courses with a CR/NCR final mark
	// are not accepted toward any English program". Group 1 or 2 holds the
	// codes.
	complCrNcrCodesBanRe = regexp.MustCompile(`(?i)^(?:we\s+do\s+not\s+accept\s+(` + codePat + `(?:\s*,\s*` + codePat + `)*),?\s+or\s+any\s+CR\s*/\s*NCR\s+courses\s+towards?\s+our\s+programs?` +
		`|(` + codePat + `(?:\s*,\s*` + codePat + `)*),?\s+and\s+courses\s+with\s+a\s+CR\s*/\s*NCR\s+final\s+(?:mark|grade)\s+are\s+not\s+accepted\s+towards?\s+any\s+[\w&\s]+?\s+program)$`)
	// complCrNcrProgramBanRe: no course taken CR/NCR counts toward the
	// program: "No courses in the Certificate in Business Fundamentals can be
	// taken as Credit/No Credit if they are to fulfill certificate
	// requirements", "The CR/NCR option cannot be used for any program
	// completion requirements, and the 8.0 RSM credit requirement" (a row
	// under the total, which the 8.0 RSM credits are counted under too).
	complCrNcrProgramBanRe = regexp.MustCompile(`(?i)^(?:no\s+courses\s+in\s+the\s+[^,.]+?\s+can\s+be\s+taken\s+as\s+(?:credit\s*/\s*no\s+credit|CR\s*/\s*NCR)\s+if\s+they\s+are\s+to\s+(?:fulfill|satisfy)\s+(?:the\s+)?(?:certificate|program)\s+requirements` +
		`|the\s+CR\s*/\s*NCR\s+option\s+cannot\s+be\s+used\s+for\s+any\s+program\s+(?:completion\s+)?requirements(?:,?\s+and\s+the\s+\d+(?:\.\d+)?\s+(?-i:[A-Z]{3})\s+credit\s+requirement)?)$`)
	// complCrNcrShareRe: a share of CR/NCR courses together with other
	// coursework, approved: "Only 0.5 credit of Credit/No Credit coursework
	// or coursework taken outside of FRE or FSL courses taken with the
	// Department of French may be accepted, and must be pre-approved by the
	// Department of French". At most the share of CR/NCR courses counts, a
	// limit; the share of both and the approval stay the sentence's text.
	complCrNcrShareRe = regexp.MustCompile(`(?i)^only\s+(?:a\s+)?(\d+(?:\.\d+)?)\s+credits?\s+of\s+(?:credit\s*/\s*no\s+credit|CR\s*/\s*NCR)\s+(?:coursework|courses)\b`)
)

// complCrNcrOutsideRe: one share of CR/NCR coursework and of coursework
// outside some designators, approved: "Only 0.5 credit of Credit/No Credit
// coursework or coursework taken outside of FRE or FSL courses taken with
// the Department of French may be accepted, and must be pre-approved by the
// Department of French". The groups: the share, the designators, the
// approval.
var complCrNcrOutsideRe = regexp.MustCompile(`(?i)^only\s+(?:a\s+)?(\d+(?:\.\d+)?)\s+credits?\s+of\s+(?:credit\s*/\s*no\s+credit|CR\s*/\s*NCR)\s+coursework\s+or\s+coursework\s+taken\s+outside\s+of\s+((?-i:[A-Z]{3})(?:\s+or\s+(?-i:[A-Z]{3}))*)\s+courses\b[^,]*?\s+may\s+be\s+accepted,?\s+and\s+(must\s+be\s+pre-?approved\s+by\s+.+)$`)

// crNcrOutside reads complCrNcrOutsideRe under target: the share is a limit
// over a list of its own, the union of CR/NCR courses and the courses
// without the designators (credits{cr_ncr} and credits{exclude_designators}),
// and the approval a constraint on it, or(credits{that list, max_amount 0},
// text{permission}): a student who counts none of it needs no approval.
func (a *compAssembler) crNcrOutside(target *compRow, s string) bool {
	m := complCrNcrOutsideRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(a.clean(s)), "."))
	if m == nil {
		return false
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	limit := &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Raw: s}}
	l := a.rowUnion(limit, []*types.ReqNode{
		{Type: types.NodeCredits, CrNcr: true},
		{Type: types.NodeCredits, ExcludeDesignators: desigRe.FindAllString(m[2], -1)},
	})
	limit.r.Req = &types.ReqNode{Type: types.NodeCredits, Lists: []string{l.Name}, MaxAmount: maxAmount(v)}
	approval := &types.Requirement{Kind: types.RequirementConstraint, Raw: s, Req: orNode(
		&types.ReqNode{Type: types.NodeCredits, Lists: []string{l.Name}, MaxAmount: maxAmount(0)},
		textNode(types.TextPermission, m[3]))}
	if target != nil {
		target.children = append(target.children, limit)
	} else {
		a.program = append(a.program, limit)
	}
	a.rowUnder(target, approval)
	return true
}

// widenOutsideShares lets the coursework crNcrOutside caps count toward the
// total it caps: a total of "2.0 FSL or FRE credits" read with "Only 0.5
// credit of ... coursework taken outside of FRE or FSL courses ... may be
// accepted" would otherwise never count the approved outside 0.5. The
// total's pool, and each node that restates it (same amount and
// designators, a content constraint), becomes a list of its own, the
// designators and the capped list (S1-3, ASCER0120); the floors at a lower
// amount keep the designators.
func (a *compAssembler) widenOutsideShares() {
	if a.total == nil || a.total.r.Req == nil {
		return
	}
	pool := a.total.r.Req
	if pool.Type != types.NodeCredits || len(pool.Designators) == 0 {
		return
	}
	bare := func(n *types.ReqNode) bool {
		c := cloneNode(n)
		c.Amount, c.Designators = 0, nil
		return sameNode(c, &types.ReqNode{Type: types.NodeCredits})
	}
	if !bare(pool) {
		return
	}
	for _, u := range a.rowUnions {
		it := u.list.Items
		if len(it) != 2 || !it[0].CrNcr || !sameSet(it[1].ExcludeDesignators, pool.Designators) {
			continue
		}
		amount, desigs := pool.Amount, pool.Designators
		l := a.rowUnion(a.total, []*types.ReqNode{
			{Type: types.NodeCredits, Designators: desigs},
			{Type: types.NodeCredits, Lists: []string{u.list.Name}},
		})
		widen := func(n *types.ReqNode) {
			walkReq(n, func(n *types.ReqNode) {
				if n.Type == types.NodeCredits && n.Amount == amount && sameSet(n.Designators, desigs) && bare(n) {
					n.Designators, n.Lists = nil, []string{l.Name}
				}
			})
		}
		widen(pool)
		var walk func(rs []*compRow)
		walk = func(rs []*compRow) {
			for _, r := range rs {
				widen(r.r.Req)
				walk(r.children)
			}
		}
		walk(a.total.children)
		walk(a.floors)
		walk(a.items)
		walk(a.program)
		return
	}
}

// complTitledScopeRe: the requirement a cap counts toward, by its quoted
// title: "Only 0.5 credit of HPS390H1/ ... may count toward the major
// completion requirement of "further topics" ...". Group 1 is the title.
var complTitledScopeRe = regexp.MustCompile(`(?i)\btowards?\s+the\s+(?:[\w-]+\s+){0,3}requirement\s+(?:of\s+)?["“]([^"”]+)["”]`)

// titledLimits moves a program-level limit whose sentence names the
// requirement it counts toward by a quoted title (complTitledScopeRe) under
// that row, when exactly one row has the title (titledRows): under the total
// it would let the capped courses fill the named row while surplus courses
// meet the total (S2-5, ASMAJ1165).
func (a *compAssembler) titledLimits() {
	var keep []*compRow
	for _, r := range a.program {
		if r.r.Kind == types.RequirementLimit {
			if m := complTitledScopeRe.FindStringSubmatch(a.clean(r.r.Raw)); m != nil {
				if rows := a.titledRows(m[1]); len(rows) == 1 && rows[0] != a.total {
					rows[0].children = append(rows[0].children, r)
					continue
				}
			}
		}
		keep = append(keep, r)
	}
	a.program = keep
}

// complTakeMoreRe: a cap only permission lifts, "Students cannot take more
// than a 0.5 credit at the 400-level without written permission from the
// Undergraduate Program Administrator", "... more than 1.0 credit of the
// program-only courses without ...". Groups: the cap, the levels, or the
// name of the courses (an item's phrase, "Program-Only").
var complTakeMoreRe = regexp.MustCompile(`(?i)^students\s+cannot\s+take\s+more\s+than\s+(?:an?\s+)?(\d+(?:\.\d+)?)\s+credits?\s+(?:at\s+the\s+([1-4]00)-level|of\s+the\s+([\w-]+)\s+courses)\s+without\s+(?:written\s+)?permission\b`)

// permittedCaps reads each constraint of text that is a complTakeMoreRe
// sentence into or(credits{max_amount N, the courses}, text{permission})
// (R8): within the cap Met, over it Review. The courses are the levels, or
// the range of codes the item that names them gives ("2.0 SOC credits from
// 300-level "Program-Only" courses ( SOC317H1 – SOC354H1; ...)", the codes
// codeRange spans; Link keeps those the catalogue has). Without a range
// the sentence stays text: the item's filters (SOC at 300) count courses
// the name doesn't, which other items need, so that cap would be over for
// every student.
func (a *compAssembler) permittedCaps() {
	all := append(append(append([]*compRow{}, a.floors...), a.items...), a.program...)
	walkRows(all, func(r *compRow) {
		n := r.r.Req
		if r.r.Kind != types.RequirementConstraint || n == nil || n.Type != types.NodeText {
			return
		}
		m := complTakeMoreRe.FindStringSubmatch(strings.TrimSpace(a.clean(n.Text)))
		if m == nil {
			return
		}
		v, _ := strconv.ParseFloat(m[1], 64)
		cap := &types.ReqNode{Type: types.NodeCredits, MaxAmount: maxAmount(v)}
		if m[2] != "" {
			cap.Levels = parseLevels(m[2] + "-level")
		} else {
			name := strings.ToLower(m[3])
			walkRows(a.items, func(it *compRow) {
				if cap.Courses != nil || it.r.Kind != types.RequirementAllocation || !strings.Contains(strings.ToLower(it.r.Raw), name) {
					return
				}
				if rm := complCodeRangeRe.FindStringSubmatch(it.r.Raw); rm != nil {
					cap.Courses = codeRange(rm[1], rm[2])
				}
			})
			if cap.Courses == nil {
				return
			}
		}
		r.r.Req = orNode(cap, textNode(types.TextPermission, n.Text))
	})
}

// complCodeRangeRe: a range of codes in brackets, "( SOC317H1 – SOC354H1;
// Offerings will vary from year to year.)". Groups: the ends.
var complCodeRangeRe = regexp.MustCompile(`\(\s*(` + codePat + `)\s*–\s*(` + codePat + `)\s*;`)

// codeRange returns the codes from lo to hi, half and full courses, of one
// designator, level and campus: SOC317H1, SOC317Y1, ..., SOC354H1,
// SOC354Y1. Most aren't courses; Link keeps those the catalogue has
// (linkCodeRanges). Nil when the ends don't make such a range.
func codeRange(lo, hi string) []string {
	if !sgCodeRe.MatchString(lo) || !sgCodeRe.MatchString(hi) || lo[:4] != hi[:4] || lo[7] != hi[7] {
		return nil
	}
	a, _ := strconv.Atoi(lo[3:6])
	b, _ := strconv.Atoi(hi[3:6])
	if a >= b {
		return nil
	}
	var codes []string
	for i := a; i <= b; i++ {
		for _, w := range "HY" {
			codes = append(codes, fmt.Sprintf("%s%d%c%c", lo[:3], i, w, lo[7]))
		}
	}
	return codes
}

// compRowUnion is a list rowUnion made for a row with no label.
type compRowUnion struct {
	row  *compRow
	list *types.CourseList
}

// rowUnion makes items a list of row's own when row has no label, so
// addUnion's naming rule can't name it: its name waits for the row's number
// (nameRowUnions). The list joins the program's lists then, as namedLists
// has collected its unions before most rows that need one are read.
func (a *compAssembler) rowUnion(row *compRow, items []*types.ReqNode) *types.CourseList {
	l := &types.CourseList{Name: fmt.Sprintf("\x00row union %d", len(a.rowUnions)), Items: items}
	a.rowUnions = append(a.rowUnions, compRowUnion{row: row, list: l})
	return l
}

// nameRowUnions names each list rowUnion made by addUnion's rule, with its
// row's number for the label ("Requirement 4 courses", then "(2)"), renames
// its uses in the rows under roots and in the lists, and adds it to lists.
func (a *compAssembler) nameRowUnions(roots []*compRow, lists []*types.CourseList) []*types.CourseList {
	if a.rowUnions == nil {
		return lists
	}
	names := map[string]string{}
	for _, u := range a.rowUnions {
		base := fmt.Sprintf("Requirement %d courses", u.row.r.Number)
		name := base
		for k := 2; slices.ContainsFunc(lists, func(l *types.CourseList) bool { return l.Name == name }); k++ {
			name = base + " (" + strconv.Itoa(k) + ")"
		}
		names[u.list.Name] = name
		u.list.Name = name
		lists = append(lists, u.list)
	}
	rename := func(n *types.ReqNode) {
		walkReq(n, func(n *types.ReqNode) {
			for i, x := range n.Lists {
				if y, ok := names[x]; ok {
					n.Lists[i] = y
				}
			}
		})
	}
	var walk func(rs []*compRow)
	walk = func(rs []*compRow) {
		for _, r := range rs {
			rename(r.r.Req)
			walk(r.children)
		}
	}
	walk(roots)
	for _, l := range lists {
		for _, it := range l.Items {
			rename(it)
		}
	}
	return lists
}

// crNcrRows reads a CR/NCR rule the ban grammar (complCrNcrBanRe) misses
// into limit rows, and reports whether the sentence must also stay a row of
// text because part of it isn't encoded; nil when it reads none.
func crNcrRows(t string) (rows []*types.Requirement, text bool) {
	ban := &types.Requirement{Kind: types.RequirementLimit, Req: &types.ReqNode{Type: types.NodeCredits, CrNcr: true, MaxAmount: maxAmount(0)}}
	if m := complCrNcrCodesBanRe.FindStringSubmatch(t); m != nil {
		codes := dedupe(codeRe.FindAllString(m[1]+" "+m[2], -1))
		return []*types.Requirement{{Kind: types.RequirementLimit, Req: &types.ReqNode{Type: types.NodeCredits, Courses: codes, MaxAmount: maxAmount(0)}}, ban}, false
	}
	if complCrNcrProgramBanRe.MatchString(t) {
		return []*types.Requirement{ban}, false
	}
	if m := complCrNcrShareRe.FindStringSubmatch(t); m != nil {
		v, _ := strconv.ParseFloat(m[1], 64)
		ban.Req.MaxAmount = maxAmount(v)
		return []*types.Requirement{ban}, true
	}
	return nil, false
}

var (
	// complTransferExchangeRe: the exchange exception of a transfer rule,
	// "(except for those attained through a University of Toronto exchange
	// program)": a note, since the checker can't tell exchange from other
	// transfer credit (design 3.5).
	complTransferExchangeRe = regexp.MustCompile(`(?i)\s*\((except\s+(?:for\s+)?those\s+(?:attained|earned|obtained)\s+through\s+a\s+University\s+of\s+Toronto\s+exchange\s+program)\)`)
	// complTransferAndRe: a second clause on another requirement, "…, and
	// cannot be used to satisfy the requirement for an integrative,
	// inquiry-based activity": the requirement it names is a rule on the
	// choices of another row (a constraint), which no limit can sit under,
	// so the sentence also stays a row of text.
	complTransferAndRe = regexp.MustCompile(`(?i),?\s+and\s+cannot\s+be\s+used\s+to\s+satisfy\s+.*$`)
)

// transferRows reads a transfer cap with the exchange exception and a
// second clause ("Transfer credits (except for those attained through a
// University of Toronto exchange program) cannot comprise more than 1.0
// credit at the 300-/ 400-level, and cannot be used to satisfy the
// requirement for an integrative, inquiry-based activity"): the cap is a
// limit noting the exception, and the sentence stays a row of text when it
// has the second clause (which then holds the exception: the limit has no
// note). ok is false when it isn't one.
func transferRows(t string) (r *types.Requirement, text, ok bool) {
	m := complTransferExchangeRe.FindStringSubmatchIndex(t)
	if m == nil {
		return nil, false, false
	}
	note := cleanNote(t[m[2]:m[3]])
	t = t[:m[0]] + t[m[1]:]
	if loc := complTransferAndRe.FindStringIndex(t); loc != nil {
		t, text = t[:loc[0]], true
	}
	cm := complTransferComprisesRe.FindStringSubmatch(strings.TrimSpace(t))
	if cm == nil {
		return nil, false, false
	}
	v, _ := strconv.ParseFloat(cm[1], 64)
	var notes []string
	if !text {
		notes = []string{note} // with the second clause, the row of text holds the exception
	}
	return &types.Requirement{Kind: types.RequirementLimit, Notes: notes,
		Req: &types.ReqNode{Type: types.NodeCredits, Transfer: true, Levels: parseLevels(cm[2]), MaxAmount: maxAmount(v)}}, text, true
}

// Profile facts in completion (design 2, "profile"): facts about the
// student that aren't courses, named in the attribute scheme of the
// schema's profile table.

var (
	// complLearningCommunityRe: an item that is a learning community, "7.
	// Molecular Network Learning Community (each year of focus enrolment)":
	// the milestone has_learning_community.
	complLearningCommunityRe = regexp.MustCompile(`(?i)^(?:[\w-]+\s+){1,4}learning\s+community(?:\s*\([^()]*\))?\.?$`)
	// complProficiencyRe: proficiency in a language, by a course or by
	// placement: "At least second-year proficiency in Chinese, Korean or
	// Japanese, either by completing an appropriate language course (e.g.,
	// EAS201H1/ ... or a higher level language course) or by demonstrating
	// the required proficiency in the initial placement". Groups: the year,
	// the languages, the courses.
	complProficiencyRe = regexp.MustCompile(`(?i)^at\s+least\s+(first|second|third|fourth)-year\s+proficiency\s+in\s+((?-i:[A-Z][a-z]+)(?:,\s*(?-i:[A-Z][a-z]+))*,?\s+or\s+(?-i:[A-Z][a-z]+)),\s+either\s+by\s+completing\s+an\s+appropriate\s+language\s+course\s+\(e\.g\.,?\s+([^()]+)\)\s+or\s+(by\s+demonstrating\s+the\s+required\s+proficiency\s+in\s+the\s+initial\s+placement)\.?$`)
	// complLanguageNameRe: the languages of complProficiencyRe's list.
	complLanguageNameRe = regexp.MustCompile(`(?-i:[A-Z][a-z]+)`)
)

// profileBody reads an item whose body is a profile fact (design 2):
// has_learning_community for a learning community; min_<language>_level for
// proficiency shown by a course or by placement, or(the courses, or(one
// profile per language)), where the level is the year of language
// instruction. nil when it isn't one.
func profileBody(s string) *types.ReqNode {
	t := complClean(s)
	if complLearningCommunityRe.MatchString(t) {
		return &types.ReqNode{Type: types.NodeProfile, Attr: "has_learning_community", Value: "true", Text: t}
	}
	m := complProficiencyRe.FindStringSubmatch(t)
	if m == nil {
		return nil
	}
	courses, _ := parseCompletionText(m[3])
	if courses = checkedBody(courses, m[3]); courses == nil || !courseCodesOnly(courses) {
		return nil
	}
	level := strconv.Itoa(ordinalYear(m[1]))
	var placed []*types.ReqNode
	for _, lang := range complLanguageNameRe.FindAllString(m[2], -1) {
		placed = append(placed, &types.ReqNode{Type: types.NodeProfile, Attr: "min_" + strings.ToLower(lang) + "_level", Value: level, Text: m[4]})
	}
	return orNode(courses, orNode(placed...))
}

// courseCodesOnly reports whether n names courses: course nodes, pools of
// listed courses, and the text alternatives an e.g. list ends with ("or a
// higher level language course").
func courseCodesOnly(n *types.ReqNode) bool {
	ok, codes := true, false
	walkReq(n, func(x *types.ReqNode) {
		switch x.Type {
		case types.NodeCourse:
			codes = true
		case types.NodeCredits:
			codes = codes || x.Courses != nil
		case types.NodeOr, types.NodeAnd, types.NodeText:
		default:
			ok = false
		}
	})
	return ok && codes
}

// complAudienceRe: a sentence for one faculty's students, "Daniels students
// enrolled in this program may be able to fulfill 1.5 credits in
// requirement 6 with ARC357H1 and ...": the audience is the profile fact
// faculty (design 2). The value is the faculty as the calendar names it.
var complAudienceRe = regexp.MustCompile(`^(?:[-*]\s*)?(Daniels)\s+students\b`)

// complRowAudienceRe: a rule written for the program's students in one
// faculty, "Visual Studies Minor students in the Faculty of Arts & Science
// are not permitted to take VIS300H0, …" (ASMIN0660): the row applies only
// to them (Requirement.Audience). Group: the faculty as the calendar names
// it.
var complRowAudienceRe = regexp.MustCompile(`^(?:[A-Z][\w-]*\s+){0,4}students\s+in\s+the\s+Faculty\s+of\s+(Arts\s*&\s*Science)\b`)

// audiences puts the audience of each text alternative written for one
// faculty's students beside it: or(strict, text) becomes or(strict,
// and(profile{faculty}, text)), so the route is open only to those students
// and the others are judged on the strict reading. A row whose sentence is
// written for one faculty's students (complRowAudienceRe) gets the profile
// fact as its Audience: it applies only to them.
func (a *compAssembler) audiences(rows []*compRow) {
	var fix func(n *types.ReqNode) *types.ReqNode
	fix = func(n *types.ReqNode) *types.ReqNode {
		if n == nil {
			return nil
		}
		if n.Type == types.NodeText {
			if m := complAudienceRe.FindStringSubmatch(n.Text); m != nil {
				return andNode(&types.ReqNode{Type: types.NodeProfile, Attr: "faculty", Value: m[1], Text: m[1] + " students"}, n)
			}
			return n
		}
		if n.Type != types.NodeOr {
			return n
		}
		for i, it := range n.Items {
			n.Items[i] = fix(it)
		}
		return n
	}
	for _, r := range rows {
		r.r.Req = fix(r.r.Req)
		if m := complRowAudienceRe.FindStringSubmatch(rawHead(r.r)); m != nil && r.r.Req != nil {
			faculty := strings.Join(strings.Fields(m[1]), " ")
			r.r.Audience = &types.ReqNode{Type: types.NodeProfile, Attr: "faculty", Value: faculty, Text: m[0]}
		}
		a.audiences(r.children)
	}
}

// complAppliedToRe: where enrolment courses count, "*Students admitted to
// the program with ECO101H1/ ECO105Y1/ IRW230H1: The course will be applied
// to Requirement #3 (Higher Years)" (ASMAJ1533; its enrolment text says the
// same). Groups: the codes, the requirement's number, its section.
var complAppliedToRe = regexp.MustCompile(`(?i)^\*?\s*students\s+admitted\s+to\s+the\s+program\s+with\s+(` + codePat + `(?:\s*(?:/|,|or)\s*` + codePat + `)*):\s*the\s+course\s+will\s+be\s+applied\s+to\s+requirement\s+#?(\d{1,2})\s*\(([^()]+)\)$`)

// appliedTo reads a complAppliedToRe sentence as a note on the requirement it
// names, when that requirement's pool already lists every course: the
// sentence restates the pool (each course counts once, toward that row),
// so it is no allocation. It reports whether it did.
func (a *compAssembler) appliedTo(s string) bool {
	m := complAppliedToRe.FindStringSubmatch(a.clean(s))
	if m == nil {
		return false
	}
	r := a.labelled(strings.TrimSpace(m[3]) + " " + m[2])
	if r == nil || r.r.Req == nil {
		return false
	}
	have := map[string]bool{}
	walkReq(r.r.Req, func(n *types.ReqNode) {
		if n.Type == types.NodeCredits && n.Designators == nil {
			for _, c := range n.Courses {
				have[c] = true
			}
		}
	})
	for _, c := range codeRe.FindAllString(m[1], -1) {
		if !have[c] {
			return false
		}
	}
	a.notesTo(r, []string{cleanNote(strings.TrimLeft(strings.TrimSpace(s), "* "))})
	return true
}

// complCourseCountCapRe: a cap counted in courses over listed courses, "Up
// to two language courses totaling 1.0 credit in Pali ( RLG264H1, RLG265H1),
// Sanskrit ( ...) or Modern Hebrew ( MHB155H1, ...) will count toward the
// Department for the Study of Religion courses required ...", "Up to one
// language course totaling 0.5 credit in ...". Groups: the number of
// courses, the credits, the span with the codes.
var complCourseCountCapRe = regexp.MustCompile(`(?i)^(?:up\s+to|a\s+maximum\s+of)\s+(one|two|three|four)\s+[\w-]+\s+courses?\s+totall?ing\s+(\d+(?:\.\d+)?)\s+credits?\s+(\(?in\s+.+?)\s+will\s+count\s+towards?\b`)

// courseCountCap reads a complCourseCountCapRe sentence into a limit on its
// courses when the count and the credits say the same: every course listed
// is a half course and the courses at 0.5 make the credits. What else the
// sentence says (that the courses count toward the requirements, approval
// for other courses) stays where the sentence goes; nil when it isn't one.
func courseCountCap(t string) *types.Requirement {
	m := complCourseCountCapRe.FindStringSubmatch(t)
	if m == nil {
		return nil
	}
	codes := dedupe(codeRe.FindAllString(m[3], -1))
	v, _ := strconv.ParseFloat(m[2], 64)
	if codes == nil || float64(complCountWords[strings.ToLower(m[1])])*0.5 != v {
		return nil
	}
	for _, c := range codes {
		if courseWeight(c) != 0.5 {
			return nil
		}
	}
	return &types.Requirement{Kind: types.RequirementLimit, Req: &types.ReqNode{Type: types.NodeCredits, Courses: codes, MaxAmount: maxAmount(v)}}
}

// boundListRoutes bounds, like boundRoutes, the approval routes that name
// their wider pool on a row whose strict pool is lists (ASFOC2431B: "1.5
// credits from the remaining course in requirement 1 and the following
// courses: RSM329H1, ... and any finance-themed RSM special topics and RSM
// Summer Abroad courses as determined by the Director"): when the lists
// hold only listed courses (directly or through the requirements they draw
// on), the wider pool is those courses or the route's designator, a
// credits node like boundRoutes' (design 3.3). Lists with filters stay
// bare: their union with the designator would need a list of its own.
func (a *compAssembler) boundListRoutes(lists []*types.CourseList) {
	byName := map[string]*types.CourseList{}
	for _, l := range lists {
		byName[l.Name] = l
	}
	var walk func(rs []*compRow)
	walk = func(rs []*compRow) {
		for _, r := range rs {
			if n := r.r.Req; r.r.Kind == types.RequirementAllocation && n != nil && n.Type == types.NodeOr && len(n.Items) >= 2 {
				if strict := n.Items[0]; onlyLists(strict) {
					if codes, ok := a.listCodes(strict.Lists, byName); ok {
						pool := &types.ReqNode{Type: types.NodeCredits, Amount: strict.Amount, Courses: codes}
						for i, it := range n.Items[1:] {
							if wider := routePool(pool, it); wider != nil {
								n.Items[i+1] = andNode(wider, it)
							}
						}
					}
				}
			}
			walk(r.children)
		}
	}
	walk(a.items)
}

// complOpenListRe: a note that says the program's lists leave courses out:
// "This list of eligible courses is not exhaustive and some courses may not
// be offered every year".
var complOpenListRe = regexp.MustCompile(`(?i)\blists?\s+of\s+(?:[\w-]+\s+){0,2}courses\s+(?:is|are)\s+not\s+(?:an?\s+)?(?:exhaustive|complete)\b`)

// openLists reads a note that says the program's lists are not exhaustive:
// a course they leave out may count, so a pool or floor that draws on them
// is met strictly or, by a course off the lists, maybe: or(node, text{the
// note}), Review rather than NotMet. A row that is or(..., text) already is
// Review when its strict reading fails, and limits only trim, so both are
// left alone. The note leaves the notes once a row holds it.
func (a *compAssembler) openLists(lists []*types.CourseList) {
	i := slices.IndexFunc(a.notes, func(s string) bool { return complOpenListRe.MatchString(a.clean(s)) })
	if i < 0 || len(lists) == 0 {
		return
	}
	s := a.notes[i]
	var wrap func(n *types.ReqNode) *types.ReqNode // nil: unchanged
	wrap = func(n *types.ReqNode) *types.ReqNode {
		if n.Type == types.NodeCredits && n.Lists != nil && n.MaxAmount == nil {
			return orNode(cloneNode(n), textNode(altKind(s), s))
		}
		var c *types.ReqNode
		for k, it := range n.Items {
			if w := wrap(it); w != nil {
				if c == nil {
					c = cloneNode(n)
				}
				c.Items[k] = w
			}
		}
		return c
	}
	placed := false
	for _, r := range a.allRows() {
		n := r.r.Req
		if n == nil || r.r.Kind == types.RequirementLimit || n.Type == types.NodeOr && slices.ContainsFunc(n.Items, func(it *types.ReqNode) bool { return it.Type == types.NodeText }) {
			continue
		}
		if w := wrap(n); w != nil {
			r.r.Req = w
			addRaw(r, s)
			placed = true
		}
	}
	if placed {
		a.notes = slices.DeleteFunc(a.notes, func(x string) bool { return x == s })
	}
}

// onlyLists reports a pool that is an amount from lists and nothing else.
func onlyLists(n *types.ReqNode) bool {
	return n.Type == types.NodeCredits && n.Amount > 0 && n.MaxAmount == nil && n.Lists != nil && n.Courses == nil && n.Designators == nil &&
		n.Levels == nil && n.Breadth == nil && n.Requirements == nil && n.Exclude == nil && n.Items == nil && n.MinGrade == 0 && n.Weight == 0
}

// listCodes returns the courses the named lists hold when each item is a
// pool of listed courses, or draws on requirements (reqRefAlloc) whose
// bodies only list courses, or is a pool of such lists (at levels: "Any
// course (0.5 credit) from Group I", "400-level courses from Group I"); ok
// is false otherwise.
func (a *compAssembler) listCodes(names []string, byName map[string]*types.CourseList) (codes []string, ok bool) {
	return a.listCodesDepth(names, byName, 0)
}

func (a *compAssembler) listCodesDepth(names []string, byName map[string]*types.CourseList, depth int) (codes []string, ok bool) {
	if depth > 4 {
		return nil, false
	}
	for _, name := range names {
		l := byName[name]
		if l == nil {
			return nil, false
		}
		for _, it := range l.Items {
			switch {
			case it.Type == types.NodeCredits && it.Courses != nil && it.Designators == nil && it.Levels == nil && it.Breadth == nil &&
				it.Lists == nil && it.Requirements == nil && it.Exclude == nil && it.Weight == 0:
				codes = append(codes, it.Courses...)
			case it.Type == types.NodeCredits && it.Lists != nil && it.Courses == nil && it.Designators == nil && it.Breadth == nil &&
				it.Requirements == nil && it.Exclude == nil && it.Weight == 0:
				cs, ok := a.listCodesDepth(it.Lists, byName, depth+1)
				if !ok {
					return nil, false
				}
				codes = append(codes, slices.DeleteFunc(cs, func(c string) bool { return !levelMatches(it.Levels, c) })...)
			case it.Type == types.NodeCredits && it.Courses == nil && it.Designators == nil:
				i := slices.IndexFunc(a.reqRefs, func(x compReqRef) bool { return x.node == it })
				if i < 0 {
					return nil, false
				}
				for _, r := range a.reqRefs[i].rows {
					cs, fixed := listedCourses(r.r.Req)
					if !fixed {
						return nil, false
					}
					codes = append(codes, cs...)
				}
			default:
				return nil, false
			}
		}
	}
	return dedupe(codes), codes != nil
}

// listedCourses returns the courses a body lists when it is made of course
// nodes and pools of listed courses only.
func listedCourses(n *types.ReqNode) (codes []string, ok bool) {
	ok = n != nil
	walkReq(n, func(x *types.ReqNode) {
		switch {
		case x.Type == types.NodeCourse:
			codes = append(codes, x.Code)
		case x.Type == types.NodeCredits && x.Courses != nil && x.Designators == nil && x.Levels == nil && x.Lists == nil && x.Requirements == nil && x.Breadth == nil:
			codes = append(codes, x.Courses...)
		case x.Type == types.NodeAnd || x.Type == types.NodeOr:
		default:
			ok = false
		}
	})
	return codes, ok && codes != nil
}

// complYearCoursesRe: a label that is a year phrase and "Courses", the
// name of a group of courses among labels of the same kind (ASSPE2030:
// "Core Courses", "Methods Courses", "Fourth Year Courses").
var complYearCoursesRe = regexp.MustCompile(`(?i)^\s*courses\s*$`)

// sectionLabel reports a label that starts a section (blkStartsSection) and
// also names the item: the year phrase followed by "Courses".
func sectionLabel(label string) bool {
	p := blkYearPrefix(label)
	if p == "" {
		return false
	}
	rest := strings.TrimSpace(blkOutsideParens(label))
	return strings.HasPrefix(strings.ToLower(rest), strings.ToLower(p)) && complYearCoursesRe.MatchString(rest[len(p):])
}

var (
	// complPairPoolRe: a pool of single courses and pairs, "1.0 credit from
	// JMB170Y1/ MAT130H1/ ... ( MAT148H1, MAT149H1)/ MAT137Y1/ ( MAT158H1,
	// MAT159H1)/ ...". Groups: the amount, the members.
	complPairPoolRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+from(?:\s+(?:any\s+of\s+)?the\s+following)?:?\s+(.+)$`)
	// complPairRe: a pair in a pool, "( MAT148H1, MAT149H1)".
	complPairRe = regexp.MustCompile(`\(\s*(` + codePat + `)\s*(?:,|and)\s*(` + codePat + `)\s*\)`)
	// complPairAfterSlashRe, complPairOutRe: a pair with its separator,
	// taken out of the pool's members: the "/" before it, which ties it to
	// the course before ("PRT100Y1/ ( PRT101H1, PRT102H1), PRT120H1"), or
	// the separator after it ("( MAT148H1, MAT149H1)/ MAT137Y1").
	complPairAfterSlashRe = regexp.MustCompile(`\s*/\s*\(\s*` + codePat + `\s*(?:,|and)\s*` + codePat + `\s*\)`)
	complPairOutRe        = regexp.MustCompile(`\(\s*` + codePat + `\s*(?:,|and)\s*` + codePat + `\s*\)\s*[/,]?\s*`)
)

// pairPool reads a pool of single courses and pairs that the item grammar
// leaves as text, where each pair makes the amount alone: or(the pool of the
// single courses, and(pair)...). A pair counts as a unit (both courses), so
// one half of it with a single course doesn't make the amount, and nothing
// else is needed beside a whole pair. A pair lighter than the amount (it
// would combine with single courses) is a pair item of a list
// (unitPool, completion_units.go). ok is false when s isn't such a pool
// or the item grammar reads it.
func (a *compAssembler) pairPool(s string) (body *types.ReqNode, rows []*types.Requirement, notes []string, ok bool) {
	t := strings.TrimSuffix(complClean(s), ".")
	m := complPairPoolRe.FindStringSubmatch(t)
	if a.inPairPool || m == nil || !complPairRe.MatchString(m[2]) {
		return nil, nil, nil, false
	}
	a.inPairPool = true
	body, rows, notes = a.itemBody(s)
	defer func() { a.inPairPool = false }()
	if !hasText(body) {
		// The item grammar reads it ("( PHL205H1, PHL206H1)/ PHL210Y1").
		return body, rows, notes, true
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	var pairs []*types.ReqNode
	for _, p := range complPairRe.FindAllStringSubmatch(m[2], -1) {
		if courseWeight(p[1])+courseWeight(p[2]) != amount {
			return body, rows, notes, true
		}
		pairs = append(pairs, andNode(&types.ReqNode{Type: types.NodeCourse, Code: p[1]}, &types.ReqNode{Type: types.NodeCourse, Code: p[2]}))
	}
	rest := strings.Trim(complPairOutRe.ReplaceAllString(complPairAfterSlashRe.ReplaceAllString(m[2], ""), ""), ", /")
	if strings.ContainsAny(rest, "()[]") || !codeRe.MatchString(rest) {
		return body, rows, notes, true
	}
	singles, srows, snotes := a.itemBody(m[1] + " credit from " + rest)
	// The single courses' "/" groups are caps beside the pool (and(pool,
	// caps), which splitCaps makes limit rows of): they cap single courses
	// only.
	if pool, _ := poolCaps(singles); hasText(singles) || !isPool(pool) || pool.Designators != nil {
		return body, rows, notes, true
	}
	return orNode(append([]*types.ReqNode{singles}, pairs...)...), srows, snotes, true
}

var (
	// complOnlyTotalFromRe: a subject's cap with a cap on its lower level,
	// "-Only 1.0 credit total from Computer Science may count toward the
	// Digital Humanities Minor, with only 0.5 credit at the 100-level being
	// accepted toward program completion". The groups: the cap, the
	// subject, the level's cap, the level.
	complOnlyTotalFromRe = regexp.MustCompile(`(?i)^-?\s*only\s+(\d+(?:\.\d+)?)\s+credits?\s+total\s+from\s+([\w&\s]+?)\s+may\s+count\s+towards?\s+the\s+[\w\s]+?,\s+with\s+only\s+(\d+(?:\.\d+)?)\s+credits?\s+at\s+the\s+([1-4]00)-level\s+being\s+accepted\s+towards?\s+program\s+completion$`)
	// complAdditionalProjectsRe: "Additional BCH research project courses
	// can be taken as part of the 20.0 credits needed for your degree, but
	// cannot be counted towards the program completion". The group: the
	// designator.
	complAdditionalProjectsRe = regexp.MustCompile(`(?i)^additional\s+((?-i:[A-Z]{3}))\s+research\s+project\s+courses\s+can\s+be\s+taken\b.*\bcannot\s+be\s+counted\s+towards?\s+the\s+program(?:\s+completion)?$`)
	// complProjectCoursesRe: the research project courses an item names,
	// "a biochemistry research project course: BCH372Y1/ BCH374Y1/
	// BCH472Y1/ BCH473Y1".
	complProjectCoursesRe = regexp.MustCompile(`(?i)\bresearch\s+project\s+courses?:\s*((?:` + codePat + `)(?:\s*/\s*` + codePat + `)*)`)
	// complProjectCapRe: the note that caps them, "Students may count up to
	// 1.0 credits from a biochemistry research project course taken in 3rd
	// or 4th year to fulfill the program requirements".
	complProjectCapRe = regexp.MustCompile(`(?i)\bmay\s+count\s+up\s+to\s+(\d+(?:\.\d+)?)\s+credits?\s+from\s+an?\s+[\w\s]*?research\s+project\s+course\b`)
)

// programCaps reads constraints of text that cap some courses over the
// program into limits under their row (R8):
//   - complOnlyTotalFromRe: limit credits{the subject's designators, max N}
//     and limit credits{the designators, the level, max M};
//   - complAdditionalProjectsRe: limit credits{the research project courses
//     an item names (complProjectCoursesRe), max the note's cap
//     (complProjectCapRe)}: no further one counts.
//
// A sentence whose courses or cap aren't found stays text.
func (a *compAssembler) programCaps() {
	var projects []string
	walkRows(a.items, func(r *compRow) {
		if m := complProjectCoursesRe.FindStringSubmatch(r.r.Raw); m != nil && projects == nil {
			projects = codeRe.FindAllString(m[1], -1)
		}
	})
	projectCap := 0.0
	for _, s := range a.notes {
		if m := complProjectCapRe.FindStringSubmatch(s); m != nil {
			projectCap, _ = strconv.ParseFloat(m[1], 64)
		}
	}
	var fix func(rs []*compRow) []*compRow
	fix = func(rs []*compRow) []*compRow {
		var out []*compRow
		for _, r := range rs {
			r.children = fix(r.children)
			out = append(out, r)
			n := r.r.Req
			if r.r.Kind != types.RequirementConstraint || n == nil || n.Type != types.NodeText {
				continue
			}
			s := strings.TrimSpace(a.clean(n.Text))
			if m := complOnlyTotalFromRe.FindStringSubmatch(s); m != nil {
				desigs, levels := routeFilters(m[2])
				if desigs == nil || levels != nil {
					continue
				}
				v, _ := strconv.ParseFloat(m[1], 64)
				w, _ := strconv.ParseFloat(m[3], 64)
				r.r.Kind, r.r.Req = types.RequirementLimit, &types.ReqNode{Type: types.NodeCredits, Designators: desigs, MaxAmount: maxAmount(v)}
				out = append(out, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Raw: r.r.Raw,
					Req: &types.ReqNode{Type: types.NodeCredits, Designators: slices.Clone(desigs), Levels: parseLevels(m[4] + "-level"), MaxAmount: maxAmount(w)}}})
				continue
			}
			if m := complAdditionalProjectsRe.FindStringSubmatch(s); m != nil && projects != nil && projectCap > 0 &&
				!slices.ContainsFunc(projects, func(c string) bool { return c[:3] != m[1] }) {
				r.r.Kind, r.r.Req = types.RequirementLimit, &types.ReqNode{Type: types.NodeCredits, Courses: slices.Clone(projects), MaxAmount: maxAmount(projectCap)}
			}
		}
		return out
	}
	a.floors, a.items, a.program = fix(a.floors), fix(a.items), fix(a.program)
	if a.total != nil {
		a.total.children = fix(a.total.children)
	}
}

var (
	// complCognatePartRe: a share of an item's credits from other subjects,
	// some at any level and some cognate offerings only: "6.5 additional
	// PRT credits, of which up to 1.0 credit may be taken from: SPA courses
	// at any level, LAS courses at any level, or cognate course offerings in
	// EUR, GGR, HIS, POL". The groups: the amount, the designator, the cap,
	// the subjects at any level, the cognate subjects.
	complCognatePartRe = regexp.MustCompile(`^(\d+(?:\.\d+)?)\s+additional\s+([A-Z]{3})\s+credits,\s+of\s+which\s+up\s+to\s+(\d+(?:\.\d+)?)\s+credits?\s+may\s+be\s+taken\s+from:\s+((?:[A-Z]{3}\s+courses\s+at\s+any\s+level,\s+)+)or\s+cognate\s+course\s+offerings\s+in\s+([A-Z]{3}(?:,\s*[A-Z]{3})*)$`)
	// complCognateShareRe: the same as a sentence under a pool, all cognate
	// offerings, at a level: "Of these 9.0 credits, up to 2.0 credits at the
	// 200+ level may be taken from the following cognate departmental or
	// college offerings: EUR, GGR, HIS, LAS, POL, SPA". The groups: the
	// amount, the cap, the level, the subjects.
	complCognateShareRe = regexp.MustCompile(`(?i)^of\s+these\s+(\d+(?:\.\d+)?)\s+credits,\s+up\s+to\s+(\d+(?:\.\d+)?)\s+credits?\s+at\s+the\s+([1-4]00)\+\s+level\s+may\s+be\s+taken\s+from\s+the\s+following\s+cognate\s+departmental\s+or\s+college\s+offerings:\s+((?-i:[A-Z]{3})(?:,\s*(?-i:[A-Z]{3}))*)$`)
	// complEligibleListRe: where the cognate offerings are listed, a note
	// beside them: "Please, consult with Associate Chair of Undergraduate
	// Studies or our website for an updated list of courses pre-approved for
	// program credit", "A complete list of eligible courses is available at
	// the departmental website or by contacting the Associate Chair ...".
	complEligibleListRe = regexp.MustCompile(`(?i)^(?:please,?\s+consult\s+with\b.*\bupdated\s+list\s+of\s+courses\s+pre-approved\b|a\s+complete\s+list\s+of\s+eligible\s+courses\s+is\s+available\b)`)
)

// cognateShares reads an item's share from cognate subjects (FIX-ONE
// ASMAJ0338 r3, ASSPE0338 r5): the strict pool is the designator and the
// subjects at any level, or(credits{N, them}, and(credits{N, them and the
// cognate subjects}, text{permission})), as which offerings are cognate is
// the department's list; a limit under the row caps the other subjects (and
// takes none below the level the share names). The note on where the list
// is goes to the notes.
func (a *compAssembler) cognateShares() {
	walkRows(a.items, func(r *compRow) {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr {
			return
		}
		var strict, share, list *types.ReqNode
		var own, open, others []string
		var amount, cap float64
		var level int
		for _, it := range n.Items {
			switch {
			case it.Type == types.NodeCredits && strict == nil:
				strict = it
			case it.Type != types.NodeText:
				return
			case complEligibleListRe.MatchString(a.clean(it.Text)):
				list = it
			default:
				s := strings.TrimSpace(a.clean(it.Text))
				if m := complCognatePartRe.FindStringSubmatch(s); m != nil {
					amount, _ = strconv.ParseFloat(m[1], 64)
					cap, _ = strconv.ParseFloat(m[3], 64)
					own = append([]string{m[2]}, desigRe.FindAllString(m[4], -1)...)
					open = desigRe.FindAllString(m[5], -1)
					others = append(slices.Clone(own[1:]), open...)
					share = it
				} else if m := complCognateShareRe.FindStringSubmatch(s); m != nil {
					amount, _ = strconv.ParseFloat(m[1], 64)
					cap, _ = strconv.ParseFloat(m[2], 64)
					level, _ = strconv.Atoi(m[3])
					open = desigRe.FindAllString(m[4], -1)
					others = slices.Clone(open)
					share = it
				} else {
					return
				}
			}
		}
		if share == nil {
			return
		}
		if strict != nil {
			// The sentence under a pool: the pool is its own designator's.
			if strict.Amount != amount || len(strict.Designators) != 1 || strict.Courses != nil || strict.Levels != nil || strict.Lists != nil {
				return
			}
			own = slices.Clone(strict.Designators)
		} else if own == nil {
			return
		}
		pool := &types.ReqNode{Type: types.NodeCredits, Amount: amount, Designators: own}
		wide := &types.ReqNode{Type: types.NodeCredits, Amount: amount, Designators: dedupe(append(slices.Clone(own), open...))}
		text := cloneNode(share)
		text.Kind = types.TextPermission
		r.r.Req = orNode(pool, andNode(wide, text))
		r.children = append(r.children, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Raw: share.Text,
			Req: &types.ReqNode{Type: types.NodeCredits, Designators: others, MaxAmount: maxAmount(cap)}}})
		if level > 100 {
			var below []int
			for l := 100; l < level; l += 100 {
				below = append(below, l)
			}
			r.children = append(r.children, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Raw: share.Text,
				Req: &types.ReqNode{Type: types.NodeCredits, Designators: slices.Clone(others), Levels: below, MaxAmount: maxAmount(0)}}})
		}
		if list != nil {
			a.notes = append(a.notes, cleanNote(list.Text))
		}
	})
}

// complCountNoMoreRe: a count of courses "and no more" with alternatives
// among them (the pool reader's complNoMoreRe reads "One lab course"
// over plain codes): "Two seminar courses (1.0 credit), and no more, taken
// from the following: JLP471H1, PSY402H1/ PSY403H1/ PSY404H1, PSY410H1,
// ...". The groups: the credits, the codes.
var complCountNoMoreRe = regexp.MustCompile(`(?i)^(?:two|three|four)\s+[a-z]+\s+courses\s+\((\d+(?:\.\d+)?)\s+credits?\),\s+and\s+no\s+more,\s+taken\s+from(?:\s+the\s+following)?\s*:\s*` + complRouteCodesPat + `$`)

// countNoMore reads an item of text complCountNoMoreRe matches (FIX-ONE
// ASSPE1160 r12): credits{N, the codes}, a limit of N on them under row 1
// (the program counts no more), and a limit of one course on each "/"
// group under the item ("PSY402H1/ PSY403H1/ PSY404H1": one of them).
func (a *compAssembler) countNoMore() {
	walkRows(a.items, func(r *compRow) {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText {
			return
		}
		m := complCountNoMoreRe.FindStringSubmatch(strings.TrimSpace(a.clean(n.Text)))
		if m == nil {
			return
		}
		v, _ := strconv.ParseFloat(m[1], 64)
		codes := codeRe.FindAllString(m[2], -1)
		if v <= 0 || slices.ContainsFunc(codes, func(c string) bool { return !strings.HasSuffix(c, "H1") }) {
			return
		}
		r.r.Req = &types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: codes}
		for _, part := range strings.Split(m[2], ",") {
			if g := codeRe.FindAllString(part, -1); len(g) > 1 && strings.Contains(part, "/") {
				r.children = append(r.children, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Raw: r.r.Raw,
					Req: &types.ReqNode{Type: types.NodeCredits, Courses: g, MaxAmount: maxAmount(courseWeight(g[0]))}}})
			}
		}
		a.rowUnder(nil, &types.Requirement{Kind: types.RequirementLimit, Raw: r.r.Raw,
			Req: &types.ReqNode{Type: types.NodeCredits, Courses: slices.Clone(codes), MaxAmount: maxAmount(v)}})
	})
}

// complTransferRefusedRe: "Transfer credits from AP and IB psychology are
// not accepted".
var complTransferRefusedRe = regexp.MustCompile(`(?i)^transfer\s+credits\s+from\b.*\bare\s+not\s+accepted$`)

// transferUnderItem moves a program-level constraint of text that refuses
// a subject's transfer credits (complTransferRefusedRe) under the item on
// the line before it, when that item lists the subject's courses (FIX-ONE
// ASMAJ2575 r15: the sentence is its own line after item 8, where ASSPE2575
// has it inside its item). It stays text.
func (a *compAssembler) transferUnderItem() {
	keep := a.program[:0:0]
	for _, r := range a.program {
		if !a.moveTransfer(r) {
			keep = append(keep, r)
		}
	}
	a.program = keep
}

// moveTransfer moves r under its item (transferUnderItem) and reports
// whether it did.
func (a *compAssembler) moveTransfer(r *compRow) bool {
	n := r.r.Req
	if r.r.Kind != types.RequirementConstraint || n == nil || n.Type != types.NodeText {
		return false
	}
	s := strings.TrimSpace(a.clean(n.Text))
	if !complTransferRefusedRe.MatchString(s) {
		return false
	}
	desigs, _ := routeFilters(s)
	if desigs == nil {
		return false
	}
	prev := ""
	for _, l := range a.lines {
		if strings.Contains(a.clean(l), s) {
			break
		}
		if strings.TrimSpace(l) != "" {
			prev = strings.TrimSpace(l)
		}
	}
	if prev == "" {
		return false
	}
	var item *compRow
	walkRows(a.items, func(it *compRow) {
		lines := strings.Split(strings.TrimSpace(it.r.Raw), "\n")
		if item == nil && it.r.Kind == types.RequirementAllocation && strings.TrimSpace(lines[len(lines)-1]) == prev &&
			slices.ContainsFunc(codeRe.FindAllString(it.r.Raw, -1), func(c string) bool { return slices.Contains(desigs, c[:3]) }) {
			item = it
		}
	})
	if item == nil {
		return false
	}
	item.children = append(item.children, r)
	return true
}
