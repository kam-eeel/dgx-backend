package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Relaxations: a sentence that lets a student meet a requirement without a
// course it names ("CSC108H1 is waived for students who complete CSC148H1
// first", "If the FOR301H1 is not on offer, a student may use a substitute
// ...", "students who take CHM222H1 will not have to take PHY252H1") is an
// alternative on that course's node, on each row that requires it
// (wrapNode): or(course, the other way). Never a note (the row would stay
// strict, NotMet for a student the calendar lets through), and never a row
// of text (Review for everyone, the row still strict). The other way is
// structured where the sentence states it ("this requirement must be
// fulfilled with one of the following 0.5 credits: ACT100H1, ..." is
// credits{0.5, courses}; "Students may use MAT351Y1 instead of APM346H1" is
// MAT351Y1), else the sentence as text (Review).

var (
	complRelaxCodesPat = `((?:` + codePat + `)(?:\s*(?:/|,|or|and)\s*` + codePat + `)*)`
	// "CSC108H1 is waived for students who complete CSC148H1 first".
	complRelaxWaivedRe = regexp.MustCompile(`(?i)^` + complRelaxCodesPat + `\s+(?:is|are)\s+waived\b`)
	// "... waived for students who complete CSC148H1 first": the course the
	// waiver needs, on the same row; which came first is not on a transcript
	// checker's terms, so it is a note.
	complRelaxWaivedForRe = regexp.MustCompile(`(?i)\bwaived\s+for\s+students\s+who\s+(?:complete|take)\s+(` + codePat + `)\s+first$`)
	// "If the FOR301H1 is not on offer, a student may use a substitute ...".
	complRelaxNotOfferedRe = regexp.MustCompile(`(?i)^if\s+(?:the\s+)?` + complRelaxCodesPat + `\s+(?:is|are)\s+not\s+(?:on\s+offer|offered|available)\b`)
	// "students who take CHM222H1 will not have to take PHY252H1".
	complRelaxNotTakeRe = regexp.MustCompile(`(?i)\b(?:will|do|does|need)\s+not\s+(?:have|need)\s+to\s+take\s+` + complRelaxCodesPat)
	// "If you are unable to complete STA130H1 in your first year of study,
	// this requirement must be fulfilled with ..." / "... by completing a 300
	// or 400-level 0.5 credit STA course to replace STA130H1".
	complRelaxUnableRe = regexp.MustCompile(`(?i)^if\s+you\s+(?:are\s+unable\s+to|do\s+not|cannot)\s+complete\s+` + complRelaxCodesPat + `\b[^,]*,\s*this\s+requirement\s+must\s+be\s+(?:fulfilled|met|satisfied)\s+(?:with|by\s+completing)\s+(.+)$`)
	// "Students exempted from any of the required FSL courses ... must
	// replace these with ...", "Students exempted from any of FSL220H1/
	// FSL222H1 ... must replace these credits with ...", "Students exempted
	// from any of the required courses must replace these courses with ...
	// FSL courses at the 200+ level".
	complRelaxExemptedRe = regexp.MustCompile(`(?i)\bexempted\s+from\s+(?:any\s+of\s+)?(?:the\s+)?(?:` + complRelaxCodesPat + `|(?:required\s+)?((?-i:[A-Z]{3}))\s+(?:program\s+)?courses|required\s+courses)\b.*\b(?:must|may|can)\s+replace\b`)
	// "Students may use MAT351Y1 instead of APM346H1".
	complRelaxInsteadRe = regexp.MustCompile(`(?i)\b(?:may|can)\s+(?:use|take|substitute)\s+` + complRelaxCodesPat + `\s+(?:instead\s+of|in\s+place\s+of|in\s+lieu\s+of)\s+` + complRelaxCodesPat + `\s*$`)
	// "Students who completed DTS200Y1 for Requirement 1 only have to
	// complete DTS300H1 for Requirement 2".
	complRelaxOnlyHaveRe = regexp.MustCompile(`(?i)\bwho\s+(?:have\s+)?completed\s+(` + codePat + `)\s+for\s+requirement\s+(\d+)\s+only\s+(?:have|need)\s+to\s+complete\s+(` + codePat + `)\s+for\s+requirement\s+(\d+)\b`)
	// "(e) 300-level courses may be replaced by 400-level courses (inquire in
	// advance)".
	complRelaxLevelRe = regexp.MustCompile(`(?i)^([1-4])00-level\s+courses\s+may\s+be\s+(?:replaced|substituted)\s+(?:by|with)\s+([1-4])00-level\s+courses\b`)
	// "There is no need to replace the missing half-credit for program
	// completion": with "may omit", an exception to the total.
	complNoReplaceRe = regexp.MustCompile(`(?i)\bno\s+need\s+to\s+replace\b|\bdoes\s+not\s+(?:have|need)\s+to\s+be\s+replaced\b`)
)

// isRelaxation reports whether s relaxes a course a requirement names (the
// patterns above, and "may omit CSC108H1").
func isRelaxation(s string) bool {
	t := complClean(s)
	for _, re := range []*regexp.Regexp{complRelaxWaivedRe, complRelaxNotOfferedRe, complRelaxNotTakeRe, complRelaxUnableRe, complRelaxExemptedRe, complRelaxInsteadRe, complRelaxOnlyHaveRe, complRelaxLevelRe, complOmitRe} {
		if re.MatchString(t) {
			return true
		}
	}
	return false
}

// relax places the relaxations read so far (finish). A sentence whose
// course no row requires is a note.
func (a *compAssembler) relax() {
	for _, s := range a.relaxations {
		if !a.relaxOne(s) {
			a.notes = append(a.notes, cleanNote(s))
		}
	}
	a.standIns()
	a.omitTotals()
}

// relaxOne places one relaxation and reports whether a row took it.
func (a *compAssembler) relaxOne(s string) bool {
	t := a.clean(s)
	kind := types.TextOther
	if complApprovalWordRe.MatchString(t) || a.isApproval(s) || complRelaxNotOfferedRe.MatchString(t) {
		kind = types.TextPermission
	}
	// alt builds the other way afresh for each row it wraps: rows never
	// share a node.
	var codes []string
	alt := func(string) *types.ReqNode { return textNode(kind, s) }
	switch {
	case complRelaxLevelRe.MatchString(t):
		// The pools of the replaced level take the other one too, with the
		// approval: or(pool, and(pool at both levels, permission)).
		m := complRelaxLevelRe.FindStringSubmatch(t)
		from, to := int(m[1][0]-'0')*100, int(m[2][0]-'0')*100
		done := false
		for _, r := range a.routeRows() {
			n := r.r.Req
			if r == a.total || n == nil || n.Type != types.NodeCredits || !slices.Equal(n.Levels, []int{from}) {
				continue
			}
			wide := cloneNode(n)
			wide.Levels = []int{from, to}
			wide.Exclude = nil
			r.r.Req = orNode(n, andNode(wide, textNode(types.TextPermission, t)))
			addRaw(r, s)
			done = true
		}
		return done
	case complRelaxOnlyHaveRe.MatchString(t):
		// The row the sentence names gets the reduced route: "Students who
		// completed DTS200Y1 for Requirement 1 only have to complete DTS300H1
		// for Requirement 2" is or(row 2, and(DTS200Y1, DTS300H1)), and rows 1
		// and 2 share DTS200Y1 (overlaps; claims are exclusive). Without a
		// row 1 that names DTS200Y1, the condition stays text.
		m := complRelaxOnlyHaveRe.FindStringSubmatch(t)
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
		cond := textNode(kind, s)
		if first != nil && first != r && rowNames(first, m[1]) && !rowNames(r, m[1]) {
			cond = &types.ReqNode{Type: types.NodeCourse, Code: m[1]}
			a.overlaps = append(a.overlaps, [2]*compRow{first, r})
		}
		r.r.Req = orNode(r.r.Req, andNode(cond, &types.ReqNode{Type: types.NodeCourse, Code: m[3]}))
		addRaw(r, s)
		return true
	case complRelaxInsteadRe.MatchString(t) && !complApprovalWordRe.MatchString(t):
		m := complRelaxInsteadRe.FindStringSubmatch(t)
		codes = codeRe.FindAllString(m[2], -1)
		alt = func(string) *types.ReqNode { return orNode(courseNodes(codeRe.FindAllString(m[1], -1))...) }
	case complRelaxWaivedRe.MatchString(t):
		codes = codeRe.FindAllString(complRelaxWaivedRe.FindStringSubmatch(t)[1], -1)
	case complRelaxNotOfferedRe.MatchString(t):
		codes = codeRe.FindAllString(complRelaxNotOfferedRe.FindStringSubmatch(t)[1], -1)
	case complRelaxNotTakeRe.MatchString(t):
		loc := complRelaxNotTakeRe.FindStringSubmatchIndex(t)
		codes = codeRe.FindAllString(t[loc[2]:loc[3]], -1)
		// "students who take CHM222H1 will not have to take PHY252H1": the
		// course they take instead, with the approval.
		alt = func(string) *types.ReqNode {
			return andNode(append(courseNodes(codeRe.FindAllString(t[:loc[0]], -1)), textNode(kind, s))...)
		}
	case complRelaxUnableRe.MatchString(t):
		m := complRelaxUnableRe.FindStringSubmatch(t)
		codes = codeRe.FindAllString(m[1], -1)
		if n := unableAlternative(m[2], codes); n != nil {
			alt = func(string) *types.ReqNode { return cloneNode(n) }
		}
	case complRelaxExemptedRe.MatchString(t):
		m := complRelaxExemptedRe.FindStringSubmatch(t)
		codes = codeRe.FindAllString(m[1], -1)
		if codes == nil {
			codes = a.requiredCodes(m[2])
		}
	case complOmitRe.MatchString(t):
		codes = []string{complOmitRe.FindStringSubmatch(t)[1]}
	}
	done := false
	for _, r := range a.routeRows() {
		if r == a.total {
			continue
		}
		// The smallest node naming the row's relaxed courses: "Students
		// exempted from FSL220H1/ FSL222H1 ..." wraps the pair.
		named := slices.DeleteFunc(dedupe(slices.Clone(codes)), func(c string) bool { return !rowNames(r, c) })
		if len(named) == 0 {
			continue
		}
		var wrapped bool
		other := alt(named[0])
		waivedFor := complRelaxWaivedForRe.FindStringSubmatch(t)
		if waivedFor != nil && (len(named) != 1 || !rowNames(r, waivedFor[1]) || !omitAsCourse(r.r.Req, named[0])) {
			waivedFor = nil
		}
		if complOmitRe.MatchString(t) && omitAsCourse(r.r.Req, named[0]) || waivedFor != nil {
			// "may omit CSC108H1 and proceed directly with CSC148H1", "CSC108H1
			// is waived for students who complete CSC148H1 first": the item
			// needs the course only when it is taken, or(course,
			// not(course)) (completion_cross.go).
			other = notCourse(named[0])
		}
		r.r.Req = wrapNode(r.r.Req, named, other, &wrapped)
		if wrapped {
			addRaw(r, s) // a waiver too: or(course, not(course)) says it all
			done = true
		}
	}
	if done && complOmitRe.MatchString(t) && complNoReplaceRe.MatchString(a.raw) {
		a.omitTotal(codes[0])
	}
	return done
}

// courseNodes returns a course node for each code.
func courseNodes(codes []string) []*types.ReqNode {
	var out []*types.ReqNode
	for _, c := range codes {
		out = append(out, &types.ReqNode{Type: types.NodeCourse, Code: c})
	}
	return out
}

// unableAlternative reads what replaces a course a student couldn't take:
// "one of the following 0.5 credits to fulfill your 13.0-credit program
// requirements: ACT100H1 (...), ACT350H1, ..." is credits{0.5, those
// courses}; "a 300 or 400-level 0.5 credit STA course to replace STA130H1"
// is read by the grammar. Nil when it doesn't read.
func unableAlternative(s string, replaced []string) *types.ReqNode {
	s = strings.TrimSuffix(strings.TrimSpace(s), ".")
	if i := strings.LastIndex(s, ":"); i >= 0 {
		amount := complAmountRe.FindString(s[:i])
		v, err := strconv.ParseFloat(amount, 64)
		codes := slices.DeleteFunc(codeRe.FindAllString(s[i+1:], -1), func(c string) bool { return slices.Contains(replaced, c) })
		if err != nil || codes == nil {
			return nil
		}
		n := &types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: dedupe(codes)}
		if checkedBody(n, s) != n {
			return nil
		}
		return n
	}
	s = complToReplaceTailRe.ReplaceAllString(s, "")
	if m := complLevelCreditCourseRe.FindStringSubmatch(s); m != nil {
		// "a 300 or 400-level 0.5 credit STA course".
		v, _ := strconv.ParseFloat(m[2], 64)
		return &types.ReqNode{Type: types.NodeCredits, Amount: v, Designators: desigRe.FindAllString(m[3], -1), Levels: parseLevels(m[1] + " level")}
	}
	n, _ := parseCompletionText(s)
	if n = checkedBody(n, s); n == nil || hasText(n) || isAnyCredits(n) || n.Type != types.NodeCredits {
		return nil
	}
	return n
}

// complToReplaceTailRe: the replaced course after an unableAlternative: "...
// STA course to replace STA130H1".
var complToReplaceTailRe = regexp.MustCompile(`(?i)\s+to\s+replace\s+` + codePat + `$`)

// complLevelCreditCourseRe: "a 300 or 400-level 0.5 credit STA course".
var complLevelCreditCourseRe = regexp.MustCompile(`(?i)^(?:an?\s+)?([1-4]00(?:\s*(?:or|/|-|and)\s*[1-4]00)*)[-\s]*level\s+(\d+(?:\.\d+)?)\s+credits?\s+((?-i:[A-Z]{3})(?:\s*/\s*(?-i:[A-Z]{3}))*)\s+courses?$`)

// requiredCodes returns the courses of designator desig (any designator when
// empty) that the items require by code.
func (a *compAssembler) requiredCodes(desig string) []string {
	var out []string
	for _, r := range a.routeRows() {
		if r == a.total {
			continue
		}
		walkReq(r.r.Req, func(x *types.ReqNode) {
			if x.Type == types.NodeCourse && (desig == "" || strings.HasPrefix(x.Code, desig)) {
				out = append(out, x.Code)
			}
		})
	}
	return dedupe(out)
}

// rowNames reports whether row r's body names course code.
func rowNames(r *compRow, code string) bool {
	has := false
	walkReq(r.r.Req, func(x *types.ReqNode) { has = has || x.Code == code || slices.Contains(x.Courses, code) })
	return has
}

// compOmission is a part of the items a student may leave out with no need
// to replace it (omitTotal, standIns): the total is that much less for the
// student conds describe.
type compOmission struct {
	conds  []*types.ReqNode // not(CSC108H1); CSC240H1, not(CSC165H1)
	weight float64
}

// omitTotal records the exception to the total that leaving out a course
// with no need to replace it makes (design 3.1); omitTotals writes it.
func (a *compAssembler) omitTotal(code string) {
	t := a.total
	if t == nil || t.r.Req == nil || t.r.Req.Type != types.NodeCredits || t.r.Req.Amount <= courseWeight(code) {
		return
	}
	a.omissions = append(a.omissions, compOmission{conds: []*types.ReqNode{notCourse(code)}, weight: courseWeight(code)})
	for _, s := range splitComplSentences(a.raw) {
		if complNoReplaceRe.MatchString(s) && !complStandInWithoutRe.MatchString(complClean(s)) {
			addRaw(t, strings.TrimSpace(s))
		}
	}
}

// omitTotals gives the total one branch per set of omissions:
// or(credits{N}, and(credits{N less the omitted credits}, conds), ...). A
// range total ("13.0-13.5 credits", "10.5 or 11.0 credits") already allows
// for what its lower end leaves out: the omissions count down from the
// upper end, and a branch the lower end meets is left out.
func (a *compAssembler) omitTotals() {
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
		less := cloneNode(t.r.Req)
		less.Amount = upper
		items := []*types.ReqNode{less}
		for i, o := range a.omissions {
			if set&(1<<i) != 0 {
				less.Amount -= o.weight
				for _, c := range o.conds {
					items = append(items, cloneNode(c))
				}
			}
		}
		if less.Amount < lower && less.Amount > 0 {
			alts = append(alts, andNode(items...))
		}
	}
	t.r.Req = orNode(alts...)
}

// Stand-ins: one course that counts for two required courses. "CSC240H1 is
// an accelerated and enriched version of CSC165H1 plus CSC236H1" lets the
// rows holding or(CSC165H1, CSC240H1) and or(CSC236H1, CSC240H1) share
// CSC240H1 (overlaps; allocation is exclusive, so one of them would be
// NotMet), and "If you take CSC240H1 without CSC165H1, there is no need to
// replace the missing half-credit" lowers the total for that student:
// and(credits{N-0.5}, CSC240H1, not(CSC165H1)). A footnote on an item
// ("( CSC111H1/ CSC165H1, CSC236H1)/ CSC240H1 (1)" with "(1) The
// "missing" 0.5 credit of the substitution does not have to be replaced")
// lowers it for the student who took the lighter alternative and not the
// pair's required course: and(credits{N-0.5}, CSC240H1, not(CSC236H1)).
var (
	complStandInRe        = regexp.MustCompile(`(?i)^(` + codePat + `)\s+is\s+an?\s+(?:[a-z]+(?:\s+and\s+[a-z]+)?\s+)?version\s+of\s+(` + codePat + `)\s+(?:plus|and)\s+(` + codePat + `)\b`)
	complStandInWithoutRe = regexp.MustCompile(`(?i)^if\s+you\s+take\s+(` + codePat + `)\s+without\s+(` + codePat + `)\s*,`)
	complFootNoReplaceRe  = regexp.MustCompile(`(?i)^\((\d)\)\s+the\s+["“]?missing["”]?\s+(\d+(?:\.\d+)?)\s+credits?\s+of\s+the\s+substitution\s+does\s+not\s+(?:have|need)\s+to\s+be\s+replaced\b`)
)

// standIns places the stand-in sentences (above).
func (a *compAssembler) standIns() {
	var sents []string
	for _, line := range strings.Split(a.raw, "\n") {
		if m := complFootNoReplaceRe.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			w, _ := strconv.ParseFloat(m[2], 64)
			a.footnoteStandIn(m[1], w, line)
		}
		sents = append(sents, splitComplSentences(line)...)
	}
	for _, s := range sents {
		m := complStandInRe.FindStringSubmatch(a.clean(s))
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
			if n := complStandInWithoutRe.FindStringSubmatch(a.clean(w)); n != nil && n[1] == x && n[2] == m[2] && complNoReplaceRe.MatchString(w) {
				if lost := courseWeight(m[2]) + courseWeight(m[3]) - courseWeight(x); lost > 0 && a.total != nil {
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
// substitution does not have to be replaced" on the item that ends in
// "(n)": or(and(..., B), X) with X lighter than the pair by w.
func (a *compAssembler) footnoteStandIn(mark string, w float64, line string) {
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
		if pair, ok := nodeCredits(n.Items[0]); len(conds) == 1 || !ok || pair-courseWeight(x) != w {
			return
		}
		a.omissions = append(a.omissions, compOmission{conds: conds, weight: w})
		s := splitComplSentences(strings.TrimSpace(line))[0]
		addRaw(a.total, s)
		addRaw(r, s)
		a.dropNote(s)
		return
	}
}

// sharedCodes returns the codes both rows name, sorted.
func sharedCodes(r, q *compRow) []string {
	var out []string
	walkReq(r.r.Req, func(x *types.ReqNode) {
		if x.Code != "" && rowNames(q, x.Code) {
			out = append(out, x.Code)
		}
	})
	slices.Sort(out)
	return slices.Compact(out)
}

// dropNote removes the sentence s from the notes: a row encodes it.
func (a *compAssembler) dropNote(s string) {
	c := cleanNote(complClean(s))
	a.notes = slices.DeleteFunc(a.notes, func(n string) bool { return n == c })
}

// splitYAlternatives gives a full-year course offered as the alternative of
// two half-credit rows under one parent ("PSY201H1 (or one of ECO220Y1/
// ...)" and "PSY202H1 (or one of ECO220Y1/ SOC252H1/ STA221H1)") a weight of
// 0.5 in each (design 2, weight): ECO220Y1 stands in for both halves, and as
// a plain course node the first row would claim all of it and leave the
// second NotMet.
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
			if it.Type == types.NodeCourse && courseWeight(it.Code) == 1 {
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
	// complFloorItemRe: an item worded as a floor: "11. At least 2.0 credits
	// from 400-level/ D-level courses & Graduate courses in Req5 or ...".
	complFloorItemRe = regexp.MustCompile(`(?i)^(?:\d{1,2}[.)]\s*)?at\s+least\b`)
	// complFloorOverRe: a floor worded over other requirements or the program:
	// "in Req5 or Req6", "of the student's program".
	complFloorOverRe = regexp.MustCompile(`(?i)\breq(?:uirements?)?\s*\(?\d|\bof\s+the\s+student['’]s\s+(?:\d+(?:\.\d+)?\s+credits|program)\b`)
	// complWithinCreditsRe: a condition on the program's courses counted
	// elsewhere: "5. Within the 12.0 credits required for the specialist
	// program, 3.0 credits must focus on a particular global region and ...".
	complWithinCreditsRe = regexp.MustCompile(`(?i)^(?:\d{1,2}[.)]\s*)?within\s+the\s+\d+(?:\.\d+)?\s+credits\s+required\b`)
)

// floorItems makes the items that are floors on the program's courses
// constraints under the total, not allocations (design 3.1): an allocation
// would claim the courses its siblings need, so a student who meets both
// with the same courses would fail. They are:
//   - a pool of levels alone ("4. 3.5 credits at the 300+level", "5. 1.0
//     credit at the 400 level");
//   - an item of text worded as a floor over other requirements or the
//     program ("11. At least 2.0 credits from 400-level/ D-level courses &
//     Graduate courses in Req5 or Req6 ...", "At least 1.5 credits of the
//     student’s program must be in American Studies ..."); "At least 1.0
//     credit from the following courses: ..." allocates;
//   - "Within the 12.0 credits required for the specialist program, 3.0
//     credits must focus on ..." and the lines under headings after it that
//     spell out its parts (unnumbered, of text).
//
// What was under such an item moves to the total.
func (a *compAssembler) floorItems() {
	if a.total == nil {
		return
	}
	within := false
	var keep []*compRow
	for _, r := range a.items {
		n := r.r.Req
		first := strings.TrimSpace(rawHead(r.r))
		floor := false
		switch {
		case r.r.Kind != types.RequirementAllocation || n == nil:
		case n.Type == types.NodeCredits && n.Amount > 0 && n.Levels != nil && n.Designators == nil && n.Courses == nil && n.Breadth == nil &&
			n.Lists == nil && n.Requirements == nil:
			floor = true
		case complWithinCreditsRe.MatchString(first) && hasText(n):
			floor, within = true, true
		case within && r.r.Label == "" && hasText(n) && !codeRe.MatchString(first):
			floor = true
		case n.Type == types.NodeText && complFloorItemRe.MatchString(first) && complFloorOverRe.MatchString(first):
			floor = true
		}
		if r.r.Label != "" && !complWithinCreditsRe.MatchString(first) {
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
