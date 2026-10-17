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

// Relaxations: sentences that let a student meet a requirement without a
// course it names, placed as or(course, the other way) on each row needing it.

// exemptedRe matches "Students exempted from FSL220H1/ FSL222H1 may
// replace it".
var exemptedRe = regexp.MustCompile(`(?i)\bexempted\s+from\s+((?:` + grammar.CodePat + `[\s,/]*(?:and|or)?\s*)+)`)

// relaxedRows returns the rows a loose sentence relaxes and the codes it
// stands in for: rows holding the codes it replaces, the requirement it names,
// or the only item naming one of its codes.
func (a *Assembler) relaxedRows(s string) ([]*compRow, []string) {
	t := a.clean(s)
	for _, re := range []*regexp.Regexp{replacedRe, exemptedRe} {
		if m := re.FindStringSubmatch(t); m != nil {
			replaced := grammar.CodeRe.FindAllString(m[1], -1)
			if out := rowsHolding(a.routeRows(), replaced); out != nil {
				return out, replaced
			}
		}
	}
	if m := equivalentsRe.FindStringSubmatch(t); m != nil {
		// "... RSM course equivalents ... (e.g. RSM100H1 ... equivalent to
		// MGT100H1)": every row requiring a course of the example's designator
		var out []*compRow
		var replaced []string
		for _, r := range a.routeRows() {
			has := false
			grammar.WalkReq(r.r.Req, func(x *types.ReqNode) {
				if x.Type == types.NodeCourse && strings.HasPrefix(x.Code, m[1][:3]) {
					has = true
					replaced = append(replaced, x.Code)
				}
			})
			if has {
				out = append(out, r)
			}
		}
		if out != nil {
			return out, grammar.Dedupe(replaced)
		}
	}
	if rs := a.referencedRows(s); rs != nil {
		return rs, nil
	}
	if rs := a.referenced(s); rs != nil {
		return rs, nil
	}
	if r := a.itemNaming(s); r != nil {
		return []*compRow{r}, nil
	}
	return nil, nil
}

// equivalentsRe matches another unit's courses accepted as equivalents,
// with an example: "any completed RSM course equivalents ... deemed equivalent
// to MGT100H1".
var equivalentsRe = regexp.MustCompile(`(?i)\bany\s+(?:completed\s+)?` + grammar.DesigPat + `\s+course\s+equivalents\b.*\bdeemed\s+equivalent\s+to\s+(` + grammar.CodePat + `)`)

// omitRe matches "may omit CSC108H1".
var omitRe = regexp.MustCompile(`(?i)\bmay\s+omit\s+(` + grammar.CodePat + `)\b`)

// omitCovered reports whether the item naming the course a sentence lets
// students omit already offers a route without it. If so, the sentence joins
// that item's raw: the item encodes it.
func (a *Assembler) omitCovered(s string) bool {
	code := omitRe.FindStringSubmatch(s)[1]
	r := a.itemNaming(code)
	if r == nil || r.r.Req == nil || r.r.Req.Type != types.NodeOr {
		return false
	}
	without := slices.ContainsFunc(r.r.Req.Items, func(it *types.ReqNode) bool {
		has := false
		grammar.WalkReq(it, func(x *types.ReqNode) { has = has || x.Code == code || slices.Contains(x.Courses, code) })
		return !has && it.Type != types.NodeText
	})
	if without {
		addRaw(r, s)
	}
	return without
}

// omitAsCourse reports whether n names code only as a course node, never among
// a pool's courses. In a pool, or(course, not(course)) would let the pool be
// met with nothing.
func omitAsCourse(n *types.ReqNode, code string) bool {
	course, pool := false, false
	grammar.WalkReq(n, func(x *types.ReqNode) {
		course = course || x.Type == types.NodeCourse && x.Code == code
		pool = pool || slices.Contains(x.Courses, code)
	})
	return course && !pool
}

func notCourse(code string) *types.ReqNode {
	return &types.ReqNode{Type: types.NodeNot, Items: []*types.ReqNode{{Type: types.NodeCourse, Code: code}}}
}

var (
	relaxCodesPat = `((?:` + grammar.CodePat + `)(?:\s*(?:/|,|or|and)\s*` + grammar.CodePat + `)*)`
	// RelaxWaivedRe matches "CSC108H1 is waived for students who complete
	// CSC148H1 first".
	RelaxWaivedRe = regexp.MustCompile(`(?i)^` + relaxCodesPat + `\s+(?:is|are)\s+waived\b`)
	// relaxWaivedForRe matches the course a waiver needs taken first:
	// "waived for students who complete CSC148H1 first".
	relaxWaivedForRe = regexp.MustCompile(`(?i)\bwaived\s+for\s+students\s+who\s+(?:complete|take)\s+(` + grammar.CodePat + `)\s+first$`)
	// relaxNotOfferedRe matches "If the FOR301H1 is not on offer, a student
	// may use a substitute ...".
	relaxNotOfferedRe = regexp.MustCompile(`(?i)^if\s+(?:the\s+)?` + relaxCodesPat + `\s+(?:is|are)\s+not\s+(?:on\s+offer|offered|available)\b`)
	// relaxNotTakeRe matches "students who take CHM222H1 will not have to
	// take PHY252H1".
	relaxNotTakeRe = regexp.MustCompile(`(?i)\b(?:will|do|does|need)\s+not\s+(?:have|need)\s+to\s+take\s+` + relaxCodesPat)
	// relaxUnableRe matches "If you are unable to complete STA130H1 in your
	// first year of study, this requirement must be fulfilled with ...".
	relaxUnableRe = regexp.MustCompile(`(?i)^if\s+you\s+(?:are\s+unable\s+to|do\s+not|cannot)\s+complete\s+` + relaxCodesPat + `\b[^,]*,\s*this\s+requirement\s+must\s+be\s+(?:fulfilled|met|satisfied)\s+(?:with|by\s+completing)\s+(.+)$`)
	// relaxExemptedRe matches "Students exempted from any of FSL220H1/
	// FSL222H1 ... must replace". Group 1 holds codes, else group 2 a designator.
	relaxExemptedRe = regexp.MustCompile(`(?i)\bexempted\s+from\s+(?:any\s+of\s+)?(?:the\s+)?(?:` + relaxCodesPat + `|(?:required\s+)?(` + grammar.DesigPat + `)\s+(?:program\s+)?courses|required\s+courses)\b.*\b(?:must|may|can)\s+replace\b`)
	// relaxInsteadRe matches "Students may use MAT351Y1 instead of
	// APM346H1".
	relaxInsteadRe = regexp.MustCompile(`(?i)\b(?:may|can)\s+(?:use|take|substitute)\s+` + relaxCodesPat + `\s+(?:instead\s+of|in\s+place\s+of|in\s+lieu\s+of)\s+` + relaxCodesPat + `\s*$`)
	// relaxOnlyHaveRe matches "Students who completed DTS200Y1 for
	// Requirement 1 only have to complete DTS300H1 for Requirement 2".
	relaxOnlyHaveRe = regexp.MustCompile(`(?i)\bwho\s+(?:have\s+)?completed\s+(` + grammar.CodePat + `)\s+for\s+requirement\s+(\d+)\s+only\s+(?:have|need)\s+to\s+complete\s+(` + grammar.CodePat + `)\s+for\s+requirement\s+(\d+)\b`)
	// relaxLevelRe matches "300-level courses may be replaced by 400-level
	// courses".
	relaxLevelRe = regexp.MustCompile(`(?i)^([1-4])00-level\s+courses\s+may\s+be\s+(?:replaced|substituted)\s+(?:by|with)\s+([1-4])00-level\s+courses\b`)
	// noReplaceRe matches "There is no need to replace the missing
	// half-credit": with "may omit", an exception to the total.
	noReplaceRe = regexp.MustCompile(`(?i)\bno\s+need\s+to\s+replace\b|\bdoes\s+not\s+(?:have|need)\s+to\s+be\s+replaced\b`)
)

// isRelaxation reports whether s relaxes a course a requirement names.
func isRelaxation(s string) bool {
	t := Clean(s)
	for _, re := range []*regexp.Regexp{RelaxWaivedRe, relaxNotOfferedRe, relaxNotTakeRe, relaxUnableRe, relaxExemptedRe, relaxInsteadRe, relaxOnlyHaveRe, relaxLevelRe, omitRe} {
		if re.MatchString(t) {
			return true
		}
	}
	return false
}

// relax places the relaxations read so far. One whose course no row requires
// is a note; otherwise never a note, or the row would stay NotMet for a
// student the calendar lets through.
func (a *Assembler) relax() {
	for _, s := range a.relaxations {
		if !a.relaxOne(s) {
			a.notes = append(a.notes, grammar.CleanNote(s))
		}
	}
	a.standIns()
	a.omitTotals()
}

// relaxOne places relaxation s and reports whether a row took it.
func (a *Assembler) relaxOne(s string) bool {
	t := a.clean(s)
	kind := types.TextOther
	if approvalWordRe.MatchString(t) || a.IsApproval(s) || relaxNotOfferedRe.MatchString(t) {
		kind = types.TextPermission
	}
	// alt builds the other way afresh for each row: rows never share a node.
	var codes []string
	alt := func(string) *types.ReqNode { return TextNode(kind, s) }
	switch {
	case relaxLevelRe.MatchString(t):
		// The replaced level's pools take the other level too, with the approval:
		// or(pool, and(pool at both levels, permission)).
		m := relaxLevelRe.FindStringSubmatch(t)
		from, to := int(m[1][0]-'0')*100, int(m[2][0]-'0')*100
		done := false
		for _, r := range a.routeRows() {
			n := r.r.Req
			if r == a.total || n == nil || n.Type != types.NodeCredits || !slices.Equal(n.Levels, []int{from}) {
				continue
			}
			wide := grammar.CloneNode(n)
			wide.Levels = []int{from, to}
			wide.Exclude = nil
			r.r.Req = programs.OrNode(n, programs.AndNode(wide, TextNode(types.TextPermission, t)))
			addRaw(r, s)
			done = true
		}
		return done
	case relaxOnlyHaveRe.MatchString(t):
		// The row named last gets or(row, and(DTS200Y1, DTS300H1)), sharing
		// DTS200Y1 with the first row, as claims are exclusive. Without a first
		// row naming DTS200Y1, the condition stays text.
		m := relaxOnlyHaveRe.FindStringSubmatch(t)
		labelled := func(n string) *compRow {
			for _, r := range a.items {
				if r.r.Req != nil && (r.r.Label == n || strings.HasSuffix(r.r.Label, " "+n)) {
					return r
				}
			}
			return nil
		}
		r, first := labelled(m[4]), labelled(m[2])
		if r == nil {
			return false
		}
		cond := TextNode(kind, s)
		if first != nil && first != r && rowNames(first, m[1]) && !rowNames(r, m[1]) {
			cond = &types.ReqNode{Type: types.NodeCourse, Code: m[1]}
			a.overlaps = append(a.overlaps, [2]*compRow{first, r})
		}
		r.r.Req = programs.OrNode(r.r.Req, programs.AndNode(cond, &types.ReqNode{Type: types.NodeCourse, Code: m[3]}))
		addRaw(r, s)
		return true
	case relaxInsteadRe.MatchString(t) && !approvalWordRe.MatchString(t):
		m := relaxInsteadRe.FindStringSubmatch(t)
		codes = grammar.CodeRe.FindAllString(m[2], -1)
		alt = func(string) *types.ReqNode {
			return programs.OrNode(programs.CourseNodes(grammar.CodeRe.FindAllString(m[1], -1))...)
		}
	case RelaxWaivedRe.MatchString(t):
		codes = grammar.CodeRe.FindAllString(RelaxWaivedRe.FindStringSubmatch(t)[1], -1)
	case relaxNotOfferedRe.MatchString(t):
		codes = grammar.CodeRe.FindAllString(relaxNotOfferedRe.FindStringSubmatch(t)[1], -1)
	case relaxNotTakeRe.MatchString(t):
		loc := relaxNotTakeRe.FindStringSubmatchIndex(t)
		codes = grammar.CodeRe.FindAllString(t[loc[2]:loc[3]], -1)
		// The other way is the course taken instead, with the approval.
		alt = func(string) *types.ReqNode {
			return programs.AndNode(append(programs.CourseNodes(grammar.CodeRe.FindAllString(t[:loc[0]], -1)), TextNode(kind, s))...)
		}
	case relaxUnableRe.MatchString(t):
		m := relaxUnableRe.FindStringSubmatch(t)
		codes = grammar.CodeRe.FindAllString(m[1], -1)
		if n := unableAlternative(m[2], codes); n != nil {
			alt = func(string) *types.ReqNode { return grammar.CloneNode(n) }
		}
	case relaxExemptedRe.MatchString(t):
		m := relaxExemptedRe.FindStringSubmatch(t)
		codes = grammar.CodeRe.FindAllString(m[1], -1)
		if codes == nil {
			codes = a.requiredCodes(m[2])
		}
	case omitRe.MatchString(t):
		codes = []string{omitRe.FindStringSubmatch(t)[1]}
	}
	done := false
	for _, r := range a.routeRows() {
		if r == a.total {
			continue
		}
		// Wrap the smallest node naming the row's relaxed courses: "Students
		// exempted from FSL220H1/ FSL222H1 ..." wraps the pair.
		named := slices.DeleteFunc(grammar.Dedupe(slices.Clone(codes)), func(c string) bool { return !rowNames(r, c) })
		if len(named) == 0 {
			continue
		}
		var wrapped bool
		other := alt(named[0])
		waivedFor := relaxWaivedForRe.FindStringSubmatch(t)
		if waivedFor != nil && (len(named) != 1 || !rowNames(r, waivedFor[1]) || !omitAsCourse(r.r.Req, named[0])) {
			waivedFor = nil
		}
		if omitRe.MatchString(t) && omitAsCourse(r.r.Req, named[0]) || waivedFor != nil {
			// "may omit CSC108H1", "CSC108H1 is waived for students who complete
			// CSC148H1 first": the course is needed only when taken, or(course,
			// not(course)).
			other = notCourse(named[0])
		}
		r.r.Req = wrapNode(r.r.Req, named, other, &wrapped)
		if wrapped {
			addRaw(r, s) // a waiver too: or(course, not(course)) says it all
			done = true
		}
	}
	if done && omitRe.MatchString(t) && noReplaceRe.MatchString(a.raw) {
		a.omitTotal(codes[0])
	}
	return done
}

// unableAlternative reads what replaces a course a student couldn't take: "one
// of the following 0.5 credits ...: ACT100H1, ..." is credits{0.5, courses};
// other wording goes to the grammar. Nil when it doesn't read.
func unableAlternative(s string, replaced []string) *types.ReqNode {
	s = strings.TrimSuffix(strings.TrimSpace(s), ".")
	if i := strings.LastIndex(s, ":"); i >= 0 {
		amount := amountRe.FindString(s[:i])
		v, err := strconv.ParseFloat(amount, 64)
		codes := slices.DeleteFunc(grammar.CodeRe.FindAllString(s[i+1:], -1), func(c string) bool { return slices.Contains(replaced, c) })
		if err != nil || codes == nil {
			return nil
		}
		n := &types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: grammar.Dedupe(codes)}
		if checkedBody(n, s) != n {
			return nil
		}
		return n
	}
	s = toReplaceTailRe.ReplaceAllString(s, "")
	if m := levelCreditCourseRe.FindStringSubmatch(s); m != nil {
		// "a 300 or 400-level 0.5 credit STA course"
		v, _ := strconv.ParseFloat(m[2], 64)
		return &types.ReqNode{Type: types.NodeCredits, Amount: v, Designators: grammar.DesigRe.FindAllString(m[3], -1), Levels: grammar.ParseLevels(m[1] + " level")}
	}
	n, _ := parseCompletionText(s)
	if n = checkedBody(n, s); n == nil || hasText(n) || isAnyCredits(n) || n.Type != types.NodeCredits {
		return nil
	}
	return n
}

// toReplaceTailRe matches the replaced course after an alternative: "...
// STA course to replace STA130H1".
var toReplaceTailRe = regexp.MustCompile(`(?i)\s+to\s+replace\s+` + grammar.CodePat + `$`)

// levelCreditCourseRe matches "a 300 or 400-level 0.5 credit STA
// course".
var levelCreditCourseRe = regexp.MustCompile(`(?i)^(?:an?\s+)?(` + grammar.LevelPat + `(?:\s*(?:or|/|-|and)\s*` + grammar.LevelPat + `)*)[-\s]*level\s+` + grammar.AmountPat + `\s+credits?\s+(` + grammar.DesigPat + `(?:\s*/\s*` + grammar.DesigPat + `)*)\s+courses?$`)

// requiredCodes returns the courses of designator desig (any when empty) that
// the items require by code.
func (a *Assembler) requiredCodes(desig string) []string {
	var out []string
	for _, r := range a.routeRows() {
		if r == a.total {
			continue
		}
		grammar.WalkReq(r.r.Req, func(x *types.ReqNode) {
			if x.Type == types.NodeCourse && (desig == "" || strings.HasPrefix(x.Code, desig)) {
				out = append(out, x.Code)
			}
		})
	}
	return grammar.Dedupe(out)
}

// rowNames reports whether row r's body names course code.
func rowNames(r *compRow, code string) bool {
	has := false
	grammar.WalkReq(r.r.Req, func(x *types.ReqNode) { has = has || x.Code == code || slices.Contains(x.Courses, code) })
	return has
}

// compOmission is a part of the items a student may leave out with no need to
// replace it: the total is weight less for the student conds describe.
type compOmission struct {
	conds  []*types.ReqNode // not(CSC108H1); CSC240H1, not(CSC165H1)
	weight float64
}

// omitTotal records the exception to the total that leaving out course code
// with no need to replace it makes; omitTotals writes it.
func (a *Assembler) omitTotal(code string) {
	t := a.total
	if t == nil || t.r.Req == nil || t.r.Req.Type != types.NodeCredits || t.r.Req.Amount <= grammar.CourseWeight(code) {
		return
	}
	a.omissions = append(a.omissions, compOmission{conds: []*types.ReqNode{notCourse(code)}, weight: grammar.CourseWeight(code)})
	for _, s := range SplitSentences(a.raw) {
		if noReplaceRe.MatchString(s) && !standInWithoutRe.MatchString(Clean(s)) {
			addRaw(t, strings.TrimSpace(s))
		}
	}
}

// omitTotals gives the total one branch per set of omissions: or(credits{N},
// and(credits{N less the omitted}, conds), ...). A range total's omissions
// count down from its upper end; a branch its lower end meets is left out.
func (a *Assembler) omitTotals() {
	t := a.total
	if t == nil || len(a.omissions) == 0 || t.r.Req == nil || t.r.Req.Type != types.NodeCredits {
		return
	}
	lower, upper := t.r.Req.Amount, t.r.Req.Amount
	if h, ok := parseHeader(strings.SplitN(t.r.Raw, "\n", 2)[0]); ok && h.total == lower && h.upper > lower {
		upper = h.upper
	}
	alts := []*types.ReqNode{t.r.Req}
	for set := 1; set < 1<<len(a.omissions); set++ {
		less := grammar.CloneNode(t.r.Req)
		less.Amount = upper
		items := []*types.ReqNode{less}
		for i, o := range a.omissions {
			if set&(1<<i) != 0 {
				less.Amount -= o.weight
				for _, c := range o.conds {
					items = append(items, grammar.CloneNode(c))
				}
			}
		}
		if less.Amount < lower && less.Amount > 0 {
			alts = append(alts, programs.AndNode(items...))
		}
	}
	t.r.Req = programs.OrNode(alts...)
}

// Stand-ins: one course that counts for two required courses, "CSC240H1 is an
// accelerated and enriched version of CSC165H1 plus CSC236H1".
var (
	standInRe        = regexp.MustCompile(`(?i)^(` + grammar.CodePat + `)\s+is\s+an?\s+(?:[a-z]+(?:\s+and\s+[a-z]+)?\s+)?version\s+of\s+(` + grammar.CodePat + `)\s+(?:plus|and)\s+(` + grammar.CodePat + `)\b`)
	standInWithoutRe = regexp.MustCompile(`(?i)^if\s+you\s+take\s+(` + grammar.CodePat + `)\s+without\s+(` + grammar.CodePat + `)\s*,`)
	footNoReplaceRe  = regexp.MustCompile(`(?i)^\((\d)\)\s+the\s+["“]?missing["”]?\s+` + grammar.AmountPat + `\s+credits?\s+of\s+the\s+substitution\s+does\s+not\s+(?:have|need)\s+to\s+be\s+replaced\b`)
)

// standIns lets the two rows a stand-in counts for share it, since claims are
// exclusive, and lowers the total for a student who has it without the
// course it replaces when the text says there is no need to replace it.
func (a *Assembler) standIns() {
	var sents []string
	for _, line := range strings.Split(a.raw, "\n") {
		if m := footNoReplaceRe.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			w, _ := strconv.ParseFloat(m[2], 64)
			a.footnoteStandIn(m[1], w, line)
		}
		sents = append(sents, SplitSentences(line)...)
	}
	for _, s := range sents {
		m := standInRe.FindStringSubmatch(a.clean(s))
		if m == nil {
			continue
		}
		x, rows := m[1], []*compRow{nil, nil}
		for i, c := range m[2:4] {
			for _, r := range a.items {
				if r.r.Kind == types.RequirementAllocation && rowNames(r, x) && rowNames(r, c) {
					if rows[i] != nil {
						return // two rows hold it: which one stands in is unclear
					}
					rows[i] = r
				}
			}
		}
		if rows[0] == nil || rows[1] == nil || rows[0] == rows[1] ||
			!slices.Equal(sharedCodes(rows[0], rows[1]), []string{x}) {
			return
		}
		a.overlaps = append(a.overlaps, [2]*compRow{rows[0], rows[1]})
		moved := []string{s}
		for _, w := range sents {
			if n := standInWithoutRe.FindStringSubmatch(a.clean(w)); n != nil && n[1] == x && n[2] == m[2] && noReplaceRe.MatchString(w) {
				if lost := grammar.CourseWeight(m[2]) + grammar.CourseWeight(m[3]) - grammar.CourseWeight(x); lost > 0 && a.total != nil {
					a.omissions = append(a.omissions, compOmission{conds: []*types.ReqNode{{Type: types.NodeCourse, Code: x}, notCourse(m[2])}, weight: lost})
					addRaw(a.total, strings.TrimSpace(w))
				}
				moved = append(moved, w)
			}
		}
		for _, w := range moved {
			addRaw(rows[0], strings.TrimSpace(w))
			addRaw(rows[1], strings.TrimSpace(w))
			a.dropNote(w)
		}
		return
	}
}

// footnoteStandIn reads the footnote "(n) The "missing" w credit of the
// substitution does not have to be replaced" on the item ending in "(n)":
// or(and(..., B), X) with X lighter than the pair by w.
func (a *Assembler) footnoteStandIn(mark string, w float64, line string) {
	if a.total == nil {
		return
	}
	for _, r := range a.items {
		first := strings.TrimSpace(strings.SplitN(r.r.Raw, "\n", 2)[0])
		n := r.r.Req
		if !strings.HasSuffix(first, "("+mark+")") || n == nil || n.Type != types.NodeOr || len(n.Items) != 2 ||
			n.Items[0].Type != types.NodeAnd || n.Items[1].Type != types.NodeCourse {
			continue
		}
		x := n.Items[1].Code
		conds := []*types.ReqNode{{Type: types.NodeCourse, Code: x}}
		for _, c := range n.Items[0].Items {
			if c.Type == types.NodeCourse {
				conds = append(conds, notCourse(c.Code))
			}
		}
		if pair, ok := nodeCredits(n.Items[0]); len(conds) == 1 || !ok || pair-grammar.CourseWeight(x) != w {
			return
		}
		a.omissions = append(a.omissions, compOmission{conds: conds, weight: w})
		s := SplitSentences(strings.TrimSpace(line))[0]
		addRaw(a.total, s)
		addRaw(r, s)
		a.dropNote(s)
		return
	}
}

// sharedCodes returns the codes both rows name, sorted.
func sharedCodes(r, q *compRow) []string {
	var out []string
	grammar.WalkReq(r.r.Req, func(x *types.ReqNode) {
		if x.Code != "" && rowNames(q, x.Code) {
			out = append(out, x.Code)
		}
	})
	slices.Sort(out)
	return slices.Compact(out)
}

// dropNote removes sentence s from the notes: a row encodes it.
func (a *Assembler) dropNote(s string) {
	c := grammar.CleanNote(Clean(s))
	a.notes = slices.DeleteFunc(a.notes, func(n string) bool { return n == c })
}

// splitYAlternatives gives a full-year course offered as the alternative of
// two half-credit rows a weight of 0.5 in each: as a plain course node, the
// first row would claim all of it and leave the second NotMet.
func splitYAlternatives(rows []*compRow) {
	halves := map[string][]*compRow{}
	for _, r := range rows {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr {
			continue
		}
		if v, ok := nodeCredits(n); !ok || v != 0.5 {
			continue
		}
		for _, it := range n.Items {
			if it.Type == types.NodeCourse && grammar.CourseWeight(it.Code) == 1 {
				halves[it.Code] = append(halves[it.Code], r)
			}
		}
	}
	for code, rs := range halves {
		if len(rs) < 2 {
			continue
		}
		for _, r := range rs {
			for i, it := range r.r.Req.Items {
				if it.Type == types.NodeCourse && it.Code == code {
					r.r.Req.Items[i] = &types.ReqNode{Type: types.NodeCredits, Amount: 0.5, Courses: []string{code}, Weight: 0.5}
				}
			}
		}
	}
	for _, r := range rows {
		splitYAlternatives(r.children)
	}
}

var (
	// floorItemRe matches an item worded as a floor: "11. At least 2.0
	// credits from 400-level/ D-level courses".
	floorItemRe = regexp.MustCompile(`(?i)^(?:\d{1,2}[.)]\s*)?` + grammar.AtLeastPat + `\b`)
	// floorOverRe matches a floor over other requirements or the program:
	// "in Req5 or Req6", "of the student's program".
	floorOverRe = regexp.MustCompile(`(?i)\breq(?:uirements?)?\s*\(?\d|\bof\s+the\s+student['’]s\s+(?:` + grammar.NumPat + `\s+credits|program)\b`)
	// withinCreditsRe matches a condition on courses counted elsewhere: "5.
	// Within the 12.0 credits required for the specialist program".
	withinCreditsRe = regexp.MustCompile(`(?i)^(?:\d{1,2}[.)]\s*)?within\s+the\s+` + grammar.NumPat + `\s+credits\s+required\b`)
)

// floorItems makes the items that are floors on the program's courses
// constraints under the total: as allocations they would claim the courses
// their siblings need. What was under such an item moves to the total.
func (a *Assembler) floorItems() {
	if a.total == nil {
		return
	}
	within := false
	var keep []*compRow
	for _, r := range a.items {
		n := r.r.Req
		first := strings.TrimSpace(RawHead(r.r))
		floor := false
		switch {
		case r.r.Kind != types.RequirementAllocation || n == nil:
		case n.Type == types.NodeCredits && n.Amount > 0 && n.Levels != nil && n.Designators == nil && n.Courses == nil && n.Breadth == nil &&
			n.Lists == nil && n.Requirements == nil: // "4. 3.5 credits at the 300+level"
			floor = true
		case withinCreditsRe.MatchString(first) && hasText(n):
			floor, within = true, true
		case within && r.r.Label == "" && hasText(n) && !grammar.CodeRe.MatchString(first): // a part spelled out under "Within ..."
			floor = true
		case n.Type == types.NodeText && floorItemRe.MatchString(first) && floorOverRe.MatchString(first):
			floor = true
		}
		if r.r.Label != "" && !withinCreditsRe.MatchString(first) {
			within = false
		}
		if !floor {
			keep = append(keep, r)
			continue
		}
		r.r.Kind = types.RequirementConstraint
		keep = append(keep, r)
		for _, k := range r.children {
			a.program = append(a.program, k)
		}
		r.children = nil
	}
	a.items = keep
}
