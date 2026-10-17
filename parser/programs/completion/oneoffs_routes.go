package completion

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/parser/grammar"
	"degree-explorer/parser/programs"
	"degree-explorer/types"
)

// One-off readings of routes and alternatives: approvals, approved sequences,
// substitutes, alternatives with their own averages, and level-coded routes.

// exclusionAltsRe matches a course or its exclusions: "POL232H1 (or its
// exclusion: GGR270H1)". The third group is a full-year course only half of
// which counts.
var exclusionAltsRe = regexp.MustCompile(`(?i)^(` + grammar.CodePat + `)\s*\(\s*or\s+(?:one\s+of\s+)?its\s+exclusions?:\s*((?:` + grammar.CodePat + `\s*[/,]?\s*)+?)(?:;\s*only\s+(?:a\s+)?0\.5\s+credit\s+of\s+(` + grammar.CodePat + `)\s+(?:can|may|will)\s+count\b[^()]*)?\)\.?$`)

// exclusionAlts reads exclusionAltsRe as the or of the course and its
// exclusions. The half-counting course is credits{0.5, [it], weight 0.5}, so
// its other half may count elsewhere.
func exclusionAlts(text string) *types.ReqNode {
	m := exclusionAltsRe.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil || grammar.CourseWeight(m[1]) != 0.5 {
		return nil
	}
	alts := []*types.ReqNode{{Type: types.NodeCourse, Code: m[1]}}
	for _, c := range grammar.Dedupe(grammar.CodeRe.FindAllString(m[2], -1)) {
		if c == m[3] && grammar.CourseWeight(c) == 1 {
			alts = append(alts, &types.ReqNode{Type: types.NodeCredits, Amount: 0.5, Courses: []string{c}, Weight: 0.5})
			continue
		}
		alts = append(alts, &types.ReqNode{Type: types.NodeCourse, Code: c})
	}
	if m[3] != "" && !slices.Contains(grammar.CodeRe.FindAllString(m[2], -1), m[3]) {
		return nil
	}
	return programs.OrNode(alts...)
}

// halfFromAboveRe matches half of a full-year course an item above
// lists: "Astronomy: AST430H1, or AST425Y1 (0.5 credit from point 1 above)".
// The groups are the course and the point.
var halfFromAboveRe = regexp.MustCompile(`(?i)^(?:[\w\s&]+:\s*)?(?:` + grammar.CodePat + `,?\s+or\s+)?(` + grammar.CodePat + `)\s+\(0\.5\s+credit\s+from\s+point\s+(\d{1,2})\s+above\)\.?$`)

// splitAlternatives reads a halfFromAboveRe alternative as credits{0.5,
// [course], weight 0.5}, and the point's pool takes the other half the same
// way: unweighted, it would claim the whole course and leave this row NotMet.
func (a *Assembler) splitAlternatives() {
	walkRows(a.items, func(r *compRow) {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr {
			return
		}
		for i, it := range n.Items {
			if it.Type != types.NodeText {
				continue
			}
			m := halfFromAboveRe.FindStringSubmatch(strings.TrimSpace(it.Text))
			if m == nil || grammar.CourseWeight(m[1]) != 1 {
				continue
			}
			q := a.pointRow(r, m[2], m[1])
			if q == nil {
				continue
			}
			for _, c := range grammar.CodeRe.FindAllString(it.Text, -1) {
				if c != m[1] && !slices.ContainsFunc(n.Items, func(x *types.ReqNode) bool { return x.Type == types.NodeCourse && x.Code == c }) {
					return
				}
			}
			n.Items[i] = &types.ReqNode{Type: types.NodeCredits, Amount: 0.5, Courses: []string{m[1]}, Weight: 0.5}
			half := &types.ReqNode{Type: types.NodeCredits, Amount: 0.5, Courses: []string{m[1]}, Weight: 0.5}
			if rest := slices.DeleteFunc(slices.Clone(q.r.Req.Courses), func(c string) bool { return c == m[1] }); len(rest) > 0 {
				p := grammar.CloneNode(q.r.Req)
				p.Courses = rest
				q.r.Req = programs.OrNode(p, half)
			} else {
				q.r.Req = half
			}
		}
	})
}

// pointRow returns r's sibling with the same label but number point, when it
// is a 0.5-credit pool of courses that lists code, else nil.
func (a *Assembler) pointRow(r *compRow, point, code string) *compRow {
	label := r.r.Label
	if i := strings.LastIndexByte(label, ' '); i >= 0 {
		label = label[:i+1] + point
	} else {
		label = point
	}
	var found *compRow
	walkRows(a.items, func(q *compRow) {
		n := q.r.Req
		if q.r.Label == label && q.r.Section == r.r.Section && n != nil && n.Type == types.NodeCredits && n.Amount == 0.5 && n.Weight == 0 &&
			slices.Contains(n.Courses, code) && n.Designators == nil && n.Levels == nil && n.Lists == nil && n.Requirements == nil {
			found = q
		}
	})
	return found
}

// sharedEssayRe matches one full-year course for two requirements:
// "Students may also satisfy requirements (4) and (5) through completion of a
// senior essay ( FRE490Y1)". The groups are the two labels and the course.
var sharedEssayRe = regexp.MustCompile(`(?i)^students\s+may\s+also\s+satisfy\s+requirements\s+\((\d{1,2})\)\s+and\s+\((\d{1,2})\)\s+through\s+(?:the\s+)?completion\s+of\s+[^()]*\(\s*(` + grammar.CodePat + `)\s*\)\.?$`)

// sharedEssay reads a sharedEssayRe alternative on both rows it names as
// half of the course in each, credits{0.5, [it], weight 0.5}. Both rows must
// need 0.5 credit.
func (a *Assembler) sharedEssay() {
	var rows []*compRow
	var m []string
	walkRows(a.items, a.fromListsIn)
	walkRows(a.items, func(r *compRow) {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr || len(n.Items) != 2 || n.Items[1].Type != types.NodeText {
			return
		}
		if g := sharedEssayRe.FindStringSubmatch(n.Items[1].Text); g != nil && (r.r.Label == g[1] || r.r.Label == g[2]) {
			if v, ok := nodeCredits(n.Items[0]); ok && v == 0.5 && grammar.CourseWeight(g[3]) == 1 {
				rows, m = append(rows, r), g
			}
		}
	})
	if len(rows) != 2 || rows[0].r.Label == rows[1].r.Label {
		return
	}
	for _, r := range rows {
		r.r.Req.Items[1] = &types.ReqNode{Type: types.NodeCredits, Amount: 0.5, Courses: []string{m[3]}, Weight: 0.5}
	}
}

// fromListsInRe matches "0.5 additional credit from the list of courses
// in (3) or (4)". The groups are the amount and the two labels.
var fromListsInRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+additional\s+credits?\s+from\s+the\s+list\s+of\s+courses\s+in\s+\((\d{1,2})\)\s+or\s+\((\d{1,2})\)\.?$`)

// fromListsIn reads an or led by fromListsInRe text as credits{amount,
// requirements: [the two rows]}. "Additional" is the claims rule.
func (a *Assembler) fromListsIn(r *compRow) {
	n := r.r.Req
	if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr || n.Items[0].Type != types.NodeText {
		return
	}
	m := fromListsInRe.FindStringSubmatch(strings.TrimSpace(n.Items[0].Text))
	if m == nil || m[2] == m[3] {
		return
	}
	var rows []*compRow
	for _, l := range m[2:4] {
		q := a.labelled(l)
		if q == nil || q == r || q.r.Kind != types.RequirementAllocation {
			return
		}
		rows = append(rows, q)
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	ref := &types.ReqNode{Type: types.NodeCredits, Amount: amount}
	a.reqRefs = append(a.reqRefs, compReqRef{node: ref, rows: rows})
	n.Items[0] = ref
}

// worldMusicRe matches a course or credit from a bracket: "MUS200H1 or
// an alternative 200-level 0.5 credit in world music ( MUS209H1, MUS211H1)".
// The groups are the course, the level, the amount, the bracket.
var worldMusicRe = regexp.MustCompile(`(?i)^(` + grammar.CodePat + `)\s+or\s+an\s+alternative\s+([1-4])00-level\s+` + grammar.AmountPat + `\s+credits?\s+in\s+[\w\s]+?\(\s*((?:` + grammar.CodePat + `,?\s*(?:or\s+)?)+)\)$`)

// worldMusicAlt reads worldMusicRe; nil when a bracketed course isn't at
// the level.
func worldMusicAlt(s string) *types.ReqNode {
	m := worldMusicRe.FindStringSubmatch(s)
	if m == nil {
		return nil
	}
	codes := grammar.CodeRe.FindAllString(m[4], -1)
	level := int(m[2][0]-'0') * 100
	if slices.ContainsFunc(codes, func(c string) bool { return programs.CourseLevel(c) != level }) {
		return nil
	}
	v, _ := strconv.ParseFloat(m[3], 64)
	return programs.OrNode(&types.ReqNode{Type: types.NodeCourse, Code: m[1]}, &types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: codes})
}

// averageAlts reads alternatives each with its own average, as the enrolment
// grammar does: "( MAT130H1/ MAT135H1, MAT136H1) with an average grade of 73%/
// MAT137Y1 with an average grade of 63%". Nil unless each is courses alone.
func averageAlts(s string) *types.ReqNode {
	locs := programs.ItemAvgRe.FindAllStringSubmatchIndex(s, -1)
	if len(locs) < 2 {
		return nil
	}
	var alts []*types.ReqNode
	last := 0
	for _, m := range locs {
		grade, _ := strconv.Atoi(programs.SubOr(s, m, 2) + programs.SubOr(s, m, 4))
		seg := strings.TrimSpace(strings.TrimLeft(s[last:m[0]], " /"))
		n, _ := parseCompletionText(seg)
		if n == nil || !programs.OnlyCourses(n) {
			return nil
		}
		if n.Type == types.NodeOr {
			// each alternative averaged on its own
			var each []*types.ReqNode
			for _, part := range n.Items {
				each = append(each, programs.WithAverage(part, grade))
			}
			alts = append(alts, each...)
		} else {
			alts = append(alts, programs.WithAverage(n, grade))
		}
		last = m[1]
		if rest := strings.TrimSpace(s[last:]); rest != "" && !strings.HasPrefix(rest, "/") {
			return nil
		}
	}
	if strings.TrimSpace(s[last:]) != "" {
		return nil
	}
	for _, alt := range alts {
		grammar.WalkReq(alt, func(n *types.ReqNode) {
			if n.Type == types.NodeAverage && n.Amount > 0 {
				n.Best = true // the student chooses which course of "/" applies
			}
		})
	}
	return programs.OrNode(alts...)
}

// otherCoursesRe matches a route for courses other than those listed
// above it: "Other courses, such as Special Topics courses ..., may be
// approved by the Director".
var otherCoursesRe = regexp.MustCompile(`(?i)^other\s+courses\b.*\bmay\s+be\s+approved\b`)

// listRoutes places a otherCoursesRe route on the pool of courses whose
// raw ends on the line before it: or(pool, text{permission}).
// before placeProgramRoutes, which would keep it as a note
func (a *Assembler) listRoutes() {
	a.programRoutes = slices.DeleteFunc(a.programRoutes, func(s string) bool {
		if !otherCoursesRe.MatchString(Clean(s)) {
			return false
		}
		at := slices.IndexFunc(a.lines, func(l string) bool { return strings.HasPrefix(strings.TrimSpace(l), strings.TrimSpace(s)) })
		if at < 1 {
			return false
		}
		prev := strings.TrimSpace(a.lines[at-1])
		for _, r := range a.items {
			lines := strings.Split(r.r.Raw, "\n")
			n := r.r.Req
			if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeCredits || n.Courses == nil || strings.TrimSpace(lines[len(lines)-1]) != prev {
				continue
			}
			r.r.Req = programs.OrNode(n, TextNode(types.TextPermission, s))
			addRaw(r, s)
			return true
		}
		return false
	})
}

// approvedSequencesRe matches "Approved sequences of courses include
// CLT141Y1, CLT251H1, and CLT252H1; EAS100Y1 and EAS200Y1".
var approvedSequencesRe = regexp.MustCompile(`(?i)^approved\s+sequences\s+of\s+courses\s+include\s+(.+)$`)

// approvedSequences reads and(text, text{approvedSequencesRe}) in an or:
// each sequence is a strict alternative, and the item's own sentence, which
// "include" doesn't exhaust, is content beside them.
func approvedSequences(r *compRow) {
	n := r.r.Req
	if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr {
		return
	}
	for i, it := range n.Items {
		if it.Type != types.NodeAnd || len(it.Items) != 2 || it.Items[0].Type != types.NodeText || it.Items[1].Type != types.NodeText {
			continue
		}
		m := approvedSequencesRe.FindStringSubmatch(strings.TrimSuffix(Clean(it.Items[1].Text), "."))
		if m == nil {
			continue
		}
		var seqs []*types.ReqNode
		for _, part := range strings.Split(m[1], ";") {
			s, _ := parseCompletionText(strings.TrimSpace(part))
			if s == nil || hasText(s) || s.Type != types.NodeAnd {
				return
			}
			seqs = append(seqs, s)
		}
		own := it.Items[0]
		own.Kind = types.TextContent
		items := append(append(append([]*types.ReqNode{}, n.Items[:i]...), seqs...), own)
		r.r.Req = programs.OrNode(append(items, n.Items[i+1:]...)...)
		return
	}
}

// approvalOverAll reads "CHM395Y1/ ... and/or CHM397H0/ CHM398H0 ... with
// prior approval" by the ambiguity policy: the approval over every course is
// the strict reading, over the last group only the lenient one (Review).
func approvalOverAll(r *compRow) {
	n := r.r.Req
	if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr || len(n.Items) < 2 ||
		!strings.Contains(RawHead(r.r), " and/or ") {
		return
	}
	first, last := n.Items[:len(n.Items)-1], n.Items[len(n.Items)-1]
	if slices.ContainsFunc(first, func(x *types.ReqNode) bool { return x.Type != types.NodeCourse }) ||
		last.Type != types.NodeAnd || len(last.Items) != 2 || last.Items[0].Type != types.NodeOr || last.Items[1].Type != types.NodeText ||
		last.Items[1].Kind != types.TextPermission || slices.ContainsFunc(last.Items[0].Items, func(x *types.ReqNode) bool { return x.Type != types.NodeCourse }) {
		return
	}
	all := programs.OrNode(append(slices.Clone(first), last.Items[0].Items...)...)
	r.r.Req = programs.OrNode(programs.AndNode(all, last.Items[1]),
		programs.AndNode(programs.OrNode(slices.Clone(first)...), TextNode(types.TextAmbiguity, "the approval may be needed for the courses before \"and/or\" too")))
}

// capstoneSubstituteRe matches "May substitute a 0.5 credit in this
// requirement from the list of Capstone courses (all 400-level SDS and JNS
// courses)". The groups are the amount, the level, the designators.
var capstoneSubstituteRe = regexp.MustCompile(`(?i)^may\s+substitute\s+an?\s+` + grammar.AmountPat + `\s+credits?\s+in\s+this\s+requirement\s+from\s+the\s+list\s+of\s+[\w\s]+\s+courses\s+\(all\s+([1-4])00-level\s+(` + grammar.DesigPat + `(?:\s*(?:,|and)\s*` + grammar.DesigPat + `)*)\s+courses\)$`)

// capstoneSubstitute reads or(and(parts), text{capstoneSubstituteRe}),
// each part an amount from one list, as one part less and a capstone. A
// capstone can be on a part's list (ASMIN1240), so each case is its own
// alternative. The lists must hold only courses, share none, and have
// half-course capstones.
func (a *Assembler) capstoneSubstitute(r *compRow, byName map[string]*types.CourseList) {
	n := r.r.Req
	if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr || len(n.Items) != 2 ||
		n.Items[0].Type != types.NodeAnd || n.Items[1].Type != types.NodeText {
		return
	}
	m := capstoneSubstituteRe.FindStringSubmatch(strings.TrimSuffix(Clean(n.Items[1].Text), "."))
	if m == nil {
		return
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	parts := n.Items[0].Items
	if len(parts) < 2 || slices.ContainsFunc(parts, func(x *types.ReqNode) bool { return !onlyLists(x) || x.Amount != amount }) {
		return
	}
	desigs := grammar.DesigRe.FindAllString(m[3], -1)
	level := int(m[2][0]-'0') * 100
	seen := map[string]bool{}
	var capstones [][]string
	var all []string
	for _, p := range parts {
		codes, ok := a.listCodes(p.Lists, byName)
		if !ok {
			return
		}
		var cs []string
		for _, c := range codes {
			if seen[c] {
				return
			}
			seen[c] = true
			if slices.Contains(desigs, c[:3]) && grammar.LevelMatches([]int{level}, c) {
				if c[6] != 'H' {
					return
				}
				cs = append(cs, c)
			}
		}
		capstones = append(capstones, cs)
		all = append(all, cs...)
	}
	// choose returns k of the parts other than skip (-1: none)
	choose := func(k, skip int) *types.ReqNode {
		var xs []*types.ReqNode
		for i, p := range parts {
			if i != skip {
				xs = append(xs, grammar.CloneNode(p))
			}
		}
		switch k {
		case len(xs):
			return programs.AndNode(xs...)
		case 1:
			return programs.OrNode(xs...)
		}
		return &types.ReqNode{Type: types.NodeChoose, Count: k, Items: xs}
	}
	alts := []*types.ReqNode{n.Items[0], programs.AndNode(choose(len(parts)-1, -1), &types.ReqNode{Type: types.NodeCredits, Amount: amount, Designators: desigs, Levels: []int{level}, Exclude: all})}
	for i, p := range parts {
		if capstones[i] == nil {
			continue
		}
		two := grammar.CloneNode(p)
		two.Amount = 2 * amount
		in := []*types.ReqNode{two, {Type: types.NodeCredits, Amount: amount, Courses: capstones[i]}}
		if len(parts) > 2 {
			in = append(in, choose(len(parts)-2, i))
		}
		alts = append(alts, programs.AndNode(in...))
	}
	r.r.Req = programs.OrNode(alts...)
}

// levelCodedRe matches a route by level: "any 100 or 200 level coded
// course from Munk ... with prior permission of the Director" (ASMAJ2660).
var levelCodedRe = regexp.MustCompile(`(?i)^any\s+(` + grammar.LevelPat + `(?:\s+or\s+` + grammar.LevelPat + `)*)\s+level\s+coded\s+course\s+from\b.*\bwith\s+prior\s+permission\b`)

// levelHundredRe matches a level: "300".
var levelHundredRe = regexp.MustCompile(grammar.LevelPat)

// levelCodedRoute bounds r's levelCodedRe route by its levels: the
// unit's courses carry designators the text doesn't list, so the levels are
// the bound.
func levelCodedRoute(r *compRow) {
	n := r.r.Req
	if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr || len(n.Items) != 2 ||
		n.Items[0].Type != types.NodeCredits || n.Items[0].Amount == 0 || n.Items[1].Type != types.NodeText || n.Items[1].Kind != types.TextPermission {
		return
	}
	m := levelCodedRe.FindStringSubmatch(Clean(n.Items[1].Text))
	if m == nil {
		return
	}
	var levels []int
	for _, l := range levelHundredRe.FindAllString(m[1], -1) {
		levels = append(levels, int(l[0]-'0')*100)
	}
	n.Items[1] = programs.AndNode(&types.ReqNode{Type: types.NodeCredits, Amount: n.Items[0].Amount, Levels: levels}, n.Items[1])
}

// aboveNotCounted reads amountFromRe whose route is
// aboveNotCountedRe as or(credits{amount, requirements: the section's
// items}, and(credits{amount, the courses}, text{permission})).
func (a *Assembler) aboveNotCounted(row *compRow, text *types.ReqNode, alts []*types.ReqNode) bool {
	m := amountFromRe.FindStringSubmatch(Clean(text.Text))
	if m == nil || len(alts) != 1 || alts[0].Type != types.NodeText {
		return false
	}
	p := aboveNotCountedRe.FindStringSubmatch(Clean(alts[0].Text))
	if p == nil || p[2] >= p[3] {
		return false
	}
	var rows []*compRow
	for i := p[2][0]; i <= p[3][0]; i++ {
		label := strings.ToLower(p[1] + " " + string(i))
		k := slices.IndexFunc(a.allRows(), func(r *compRow) bool { return strings.ToLower(r.r.Label) == label })
		if k < 0 || a.allRows()[k] == row {
			return false
		}
		rows = append(rows, a.allRows()[k])
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	ref := &types.ReqNode{Type: types.NodeCredits, Amount: amount, Requirements: []int{}} // filled in once numbered
	a.reqRefs = append(a.reqRefs, compReqRef{node: ref, rows: rows})
	perm := programs.AndNode(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: grammar.CodeRe.FindAllString(p[5], -1)}, TextNode(types.TextPermission, p[4]))
	row.r.Req = programs.OrNode(ref, perm)
	return true
}
