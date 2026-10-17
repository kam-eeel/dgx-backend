package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Substitutes, exemptions and exclusions: sentences the passes before
// substitutes leave as a text alternative or a row of text, though they name
// what stands in for a course or what may not count. substitutes reads them
// once the routes are placed (finish):
//   - an exemption's replacement ("Students exempted from any of the
//     required FSL courses ... must replace these with the equivalent number
//     of credits of additional FSL or FRE courses"), the text alternative of
//     the courses it replaces: bounded by the pool it names, with the same
//     credits, and(credits{amount, designators, levels}, text{placement}).
//     Whether the student is exempted is the department's placement, so the
//     route stays Review;
//   - a named substitute: "A student who has completed CSC110Y1 also
//     fulfills this program requirement" is CSC110Y1 beside the row's
//     courses; "Students may use MAT351Y1 instead of APM346H1 for
//     Requirement 1 in Third Year" is MAT351Y1 beside APM346H1 (relax reads
//     the sentence without "for Requirement ..."); "Students may count
//     ECO210H1 towards the Specialist program in lieu of a 0.5 credit
//     300-level ECO elective" is the 300-level ECO pool's alternative,
//     or(pool, and(the pool less 0.5, ECO210H1)), as is "Students taking one
//     of these MAT courses can have it count in lieu of a 0.5 300-level ECO
//     course" with or(the MAT courses): the listed courses can't join the
//     pool's courses, which must pass its levels;
//   - a named equivalence a row already encodes ("The combination of (
//     STA220H1, STA255H1) is considered equivalent to ECO220Y1 ...", where an
//     item lists ECO220Y1/ ( STA220H1, STA255H1)): the sentence joins that
//     row's raw and its notes go with it, in place of a row of text;
//   - "Such students can instead substitute any 300-level MAT or APM course
//     to meet the MAT246H1 requirement": the course's weight from that pool,
//     with the sentence (who such students are), beside the course;
//   - courses that may not count: "ENG386H1, ... may not be counted toward
//     fulfilling the requirements for the Minor program in English" (each
//     program's text names that program) is a limit of 0 under the total,
//     "MAT221H1 may not be used for this requirement" a limit of 0 under the
//     item it follows;
//   - a chain of fallbacks: "DTS300H1 or a 0.5 credit to satisfy the
//     Quantitative Reasoning competency ..., to be chosen from courses in the
//     ... Handbook ... If none is available, a 0.5 credit from Breadth
//     Requirement Category #5: ..., or a 0.5 credit approved by the
//     Undergraduate Director" is or(DTS300H1, text, and(credits{0.5,
//     breadth: [5]}, text), text{permission}).

var (
	// complSubExemptedRe: "... exempted from ... must replace these (courses)
	// with the equivalent number of credits of additional FSL or FRE courses
	// (at the 200+ level)". The groups: the designators, the lowest level.
	complSubExemptedRe = regexp.MustCompile(`(?i)\bexempted\s+from\b.*\b(?:must|may|can)\s+replace\s+(?:these|them|it)(?:\s+(?:courses|credits))?\s+with\s+the\s+equivalent\s+number\s+of\s+credits\s+of\s+additional\s+((?-i:[A-Z]{3})(?:\s+or\s+(?-i:[A-Z]{3}))*)\s+courses(?:\s+at\s+the\s+([1-4])00\+\s+level)?$`)
	// complSubAlsoRe: "A student who has completed CSC110Y1 also fulfills
	// this program requirement".
	complSubAlsoRe = regexp.MustCompile(`(?i)^an?\s+student\s+who\s+has\s+completed\s+(` + codePat + `)\s+also\s+fulfil+s\s+this\s+(?:program\s+)?requirement$`)
	// complSubInsteadForRe: "Students may use MAT351Y1 instead of APM346H1
	// for Requirement 1 in Third Year". The groups: the substitute, the
	// course, the requirement's number.
	complSubInsteadForRe = regexp.MustCompile(`(?i)\b(?:may|can)\s+use\s+(` + codePat + `)\s+instead\s+of\s+(` + codePat + `)\s+for\s+requirement\s+(\d{1,2})\b`)
	// complSubInLieuRe: "Students may count ECO210H1 towards the Specialist
	// program in lieu of a 0.5 credit 300-level ECO elective". The groups:
	// the courses, the amount, the level, the designator.
	complSubInLieuRe = regexp.MustCompile(`(?i)^students\s+may\s+count\s+((?:` + codePat + `)(?:\s*(?:/|,|or)\s*` + codePat + `)*)\s+towards?\s+the\s+[a-z ]+?\s+in\s+lieu\s+of\s+an?\s+(\d+(?:\.\d+)?)\s+credit\s+([1-4])00-level\s+((?-i:[A-Z]{3}))\s+(?:elective|course)$`)
	// complSubPreparationRe: "MAT221H1/ MAT223H1/ MAT240H1 is recommended as
	// preparation for ECO375H1. Students taking one of these MAT courses can
	// have it count in lieu of a 0.5 300-level ECO course required for this
	// program". The groups as for complSubInLieuRe.
	complSubPreparationRe = regexp.MustCompile(`(?i)^((?:` + codePat + `)(?:\s*/\s*` + codePat + `)*)\s+(?:is|are)\s+recommended\s+as\s+preparation\s+for\s+` + codePat + `\.?\s+students\s+taking\s+one\s+of\s+these\s+(?-i:[A-Z]{3})\s+courses\s+can\s+have\s+it\s+count\s+in\s+lieu\s+of\s+an?\s+(\d+(?:\.\d+)?)\s+([1-4])00-level\s+((?-i:[A-Z]{3}))\s+course\s+required\s+for\s+this\s+program$`)
	// complSubEquivalentRe: "*The combination of ( STA220H1, STA255H1) is
	// considered equivalent to ECO220Y1 for ...". The groups: the two
	// courses, the course they stand for.
	complSubEquivalentRe = regexp.MustCompile(`(?i)^\*?the\s+combination\s+of\s+\(\s*(` + codePat + `)\s*(?:,|and)\s*(` + codePat + `)\s*\)\s+is\s+considered\s+equivalent\s+to\s+(` + codePat + `)\b`)
	// complSubSuchStudentsRe: "Such students can instead substitute any
	// 300-level MAT or APM course to meet the MAT246H1 requirement", after
	// "Students with MAT157Y1 cannot take MAT246H1" (audit FIX-ONE
	// ASSPE2599). The groups: the level, the designators, the course.
	complSubSuchStudentsRe = regexp.MustCompile(`(?i)^such\s+students\s+can\s+instead\s+substitute\s+any\s+([1-4])00-level\s+((?-i:[A-Z]{3})(?:\s+or\s+(?-i:[A-Z]{3}))*)\s+course\s+to\s+meet\s+the\s+(` + codePat + `)\s+requirement$`)
	// complSubNotCountedRe: "ENG386H1, ..., and ENG498H1 may not be counted
	// toward fulfilling the requirements for the Minor program in English".
	complSubNotCountedRe = regexp.MustCompile(`(?i)^((?:` + codePat + `)(?:\s*(?:,|,?\s*and|/)\s*` + codePat + `)*)\s+may\s+not\s+be\s+counted\s+towards?\s+(?:fulfil+ing\s+)?the\s+requirements\s+(?:for|of)\s+the\s+.*\bprogram\b.*$`)
	// complSubNotThisRe: "MAT221H1 may not be used for this requirement".
	complSubNotThisRe = regexp.MustCompile(`(?i)^((?:` + codePat + `)(?:\s*(?:,|,?\s*and|/|or)\s*` + codePat + `)*)\s+may\s+not\s+be\s+used\s+(?:for|towards?)\s+this\s+requirement$`)
	// complSubFallbackRe, complSubNoneRe: "DTS300H1 or a 0.5 credit to
	// satisfy the Quantitative Reasoning competency of the program, to be
	// chosen from courses in the ... Handbook developing this competency",
	// then "If none is available, a 0.5 credit from Breadth Requirement
	// Category #5: The Physical & Mathematical Universe, or a 0.5 credit
	// approved by the Undergraduate Director". The groups: the course, the
	// rest; the condition, the amount, the category, the approval.
	complSubFallbackRe = regexp.MustCompile(`(?i)^(` + codePat + `)\s+or\s+(an?\s+(\d+(?:\.\d+)?)\s+credit\s+to\s+satisfy\s+.+)$`)
	complSubNoneRe     = regexp.MustCompile(`(?i)^(if\s+none\s+is\s+available),\s+an?\s+(\d+(?:\.\d+)?)\s+credit\s+from\s+breadth\s+requirement\s+category\s+#?([1-5])\b[^,]*,\s+or\s+(an?\s+\d+(?:\.\d+)?\s+credit\s+approved\s+by\s+the\s+.+)$`)
)

// substitutes encodes the sentences above (finish).
func (a *compAssembler) substitutes() {
	roots := append(append(append([]*compRow{}, a.floors...), a.items...), a.program...)
	var rows []*compRow
	var walk func([]*compRow)
	walk = func(rs []*compRow) {
		for _, r := range rs {
			rows = append(rows, r)
			walk(r.children)
		}
	}
	walk(roots)
	for _, r := range rows {
		if r.r.Kind == types.RequirementAllocation && r.r.Req != nil {
			r.r.Req = a.substituteNode(r, r.r.Req)
		}
	}
	for _, r := range rows {
		n := r.r.Req
		if n == nil {
			continue
		}
		switch {
		case n.Type == types.NodeText && r.r.Kind == types.RequirementAllocation:
			a.equivalence(r, rows)
		case n.Type == types.NodeText && r.r.Kind == types.RequirementConstraint:
			a.notCounted(r)
		case n.Type == types.NodeAnd && r.r.Kind == types.RequirementAllocation:
			if f := fallbacks(n); f != nil {
				r.r.Req = f
			}
		}
	}
}

// substituteNode reads the text alternatives of n's "or" nodes that name a
// replacement or a substitute (row r's body) and returns n.
func (a *compAssembler) substituteNode(r *compRow, n *types.ReqNode) *types.ReqNode {
	for i, it := range n.Items {
		n.Items[i] = a.substituteNode(r, it)
	}
	if n.Type != types.NodeOr {
		return n
	}
	for i := 0; i < len(n.Items); i++ {
		it := n.Items[i]
		if it.Type != types.NodeText {
			continue
		}
		others := slices.DeleteFunc(slices.Clone(n.Items), func(x *types.ReqNode) bool { return x.Type == types.NodeText })
		t := it.Text
		switch {
		case complSubExemptedRe.MatchString(t):
			if alt := exemptedRoute(t, others); alt != nil {
				n.Items[i] = alt
			}
		case complSubSuchStudentsRe.MatchString(t):
			m := complSubSuchStudentsRe.FindStringSubmatch(t)
			if !slices.ContainsFunc(others, func(x *types.ReqNode) bool { return x.Type == types.NodeCourse && x.Code == m[3] }) {
				break
			}
			// Who "such students" are is a course on the transcript, which
			// a course node can't say without claiming it (the course is
			// another row's): the sentence is the bounded route's text.
			pool := &types.ReqNode{Type: types.NodeCredits, Amount: courseWeight(m[3]), Designators: desigRe.FindAllString(m[2], -1), Levels: []int{int(m[1][0]-'0') * 100}}
			text := t
			if w := regexp.MustCompile(`Students\s+with\s+` + codePat + `\s+cannot\s+take\s+` + m[3] + `\b[^.]*`).FindString(a.raw); w != "" {
				text = w + ". " + t
			}
			n.Items[i] = andNode(pool, textNode(types.TextOther, text))
		case complSubAlsoRe.MatchString(t):
			n.Items[i] = &types.ReqNode{Type: types.NodeCourse, Code: complSubAlsoRe.FindStringSubmatch(t)[1]}
		case complSubInsteadForRe.MatchString(t):
			m := complSubInsteadForRe.FindStringSubmatch(t)
			if (r.r.Label == m[3] || strings.HasSuffix(r.r.Label, " "+m[3])) && slices.ContainsFunc(others, func(x *types.ReqNode) bool { return x.Type == types.NodeCourse && x.Code == m[2] }) {
				n.Items[i] = &types.ReqNode{Type: types.NodeCourse, Code: m[1]}
			}
		case complSubInLieuRe.MatchString(t) || complSubPreparationRe.MatchString(t):
			m := complSubInLieuRe.FindStringSubmatch(t)
			if m == nil {
				m = complSubPreparationRe.FindStringSubmatch(t)
			}
			codes := codeRe.FindAllString(m[1], -1)
			v, _ := strconv.ParseFloat(m[2], 64)
			if pool := inLieuPool(m, others); pool != nil && n == r.r.Req && pool.Amount > v &&
				!slices.ContainsFunc(codes, func(c string) bool { return courseWeight(c) != v }) {
				less := cloneNode(pool)
				less.Amount -= v
				n.Items[i] = andNode(less, orNode(courseNodes(codes)...))
				addRaw(r, t)
			}
		}
	}
	if len(n.Items) == 1 {
		return n.Items[0]
	}
	return n
}

// exemptedRoute returns the route an exemption sentence t gives the courses
// others (the alternatives it replaces): the same credits from the pool it
// names, with the sentence. Nil when the alternatives' credits differ or
// aren't known.
func exemptedRoute(t string, others []*types.ReqNode) *types.ReqNode {
	m := complSubExemptedRe.FindStringSubmatch(t)
	amount := -1.0
	for _, o := range others {
		v, ok := nodeCredits(o)
		if !ok || v <= 0 || amount >= 0 && v != amount {
			return nil
		}
		amount = v
	}
	if amount <= 0 {
		return nil
	}
	pool := &types.ReqNode{Type: types.NodeCredits, Amount: amount, Designators: desigRe.FindAllString(m[1], -1)}
	if m[2] != "" {
		for l := int(m[2][0]-'0') * 100; l <= 400; l += 100 {
			pool.Levels = append(pool.Levels, l)
		}
	}
	return andNode(pool, textNode(types.TextPlacement, t))
}

// inLieuPool returns the pool among others (an "or"'s alternatives that
// aren't text) that the in-lieu sentence's match m names: the only
// alternative, a pool of designator m[4] at level m[3]00.
func inLieuPool(m []string, others []*types.ReqNode) *types.ReqNode {
	if len(others) != 1 {
		return nil
	}
	p := others[0]
	level, _ := strconv.Atoi(m[3] + "00")
	if p.Type != types.NodeCredits || p.Lists != nil || !slices.Equal(p.Designators, []string{m[4]}) || !slices.Contains(p.Levels, level) {
		return nil
	}
	return p
}

// equivalence reads row r, a row of text, as a named equivalence another
// of rows already encodes: the sentence joins that row's raw and r's notes
// its notes, and r goes.
func (a *compAssembler) equivalence(r *compRow, rows []*compRow) {
	m := complSubEquivalentRe.FindStringSubmatch(r.r.Req.Text)
	if m == nil || len(r.children) > 0 {
		return
	}
	pair := andNode(&types.ReqNode{Type: types.NodeCourse, Code: m[1]}, &types.ReqNode{Type: types.NodeCourse, Code: m[2]})
	for _, o := range rows {
		if o == r || o.r.Kind != types.RequirementAllocation {
			continue
		}
		found := false
		walkReq(o.r.Req, func(x *types.ReqNode) {
			found = found || x.Type == types.NodeOr &&
				slices.ContainsFunc(x.Items, func(y *types.ReqNode) bool { return y.Type == types.NodeCourse && y.Code == m[3] }) &&
				slices.ContainsFunc(x.Items, func(y *types.ReqNode) bool { return sameNode(y, pair) })
		})
		if !found {
			continue
		}
		if a.detach(r) {
			o.r.Raw += "\n" + r.r.Raw
			o.r.Notes = append(o.r.Notes, r.r.Notes...)
		}
		return
	}
}

// notCounted makes row r, a row of text, a limit of 0 when it says courses
// may not count: under the total for the program, under the item it
// follows for "this requirement".
func (a *compAssembler) notCounted(r *compRow) {
	t := r.r.Req.Text
	if m := complSubNotCountedRe.FindStringSubmatch(t); m != nil && len(r.children) == 0 {
		r.r.Kind = types.RequirementLimit
		r.r.Req = &types.ReqNode{Type: types.NodeCredits, Courses: dedupe(codeRe.FindAllString(m[1], -1)), MaxAmount: maxAmount(0)}
		return
	}
	m := complSubNotThisRe.FindStringSubmatch(t)
	if m == nil || len(r.children) > 0 {
		return
	}
	item := a.itemBefore(t)
	if item == nil || item.r.Kind != types.RequirementAllocation || !a.detach(r) {
		return
	}
	r.r.Kind = types.RequirementLimit
	r.r.Req = &types.ReqNode{Type: types.NodeCredits, Courses: dedupe(codeRe.FindAllString(m[1], -1)), MaxAmount: maxAmount(0)}
	item.children = append(item.children, r)
}

// itemBefore returns the item whose line comes last before the line that
// holds sentence s, or nil.
func (a *compAssembler) itemBefore(s string) *compRow {
	at := slices.IndexFunc(a.lines, func(l string) bool { return strings.Contains(rawWords(l), rawWords(s)) })
	for i := at - 1; at >= 0 && i >= 0; i-- {
		line := strings.TrimSpace(a.lines[i])
		for _, r := range a.items {
			if line != "" && strings.TrimSpace(rawHead(r.r)) == line {
				return r
			}
		}
	}
	return nil
}

// detach takes row r out of the rows (the floors, items and program rules,
// and the rows under them) and reports whether it was there.
func (a *compAssembler) detach(r *compRow) bool {
	var from func(rs []*compRow) ([]*compRow, bool)
	from = func(rs []*compRow) ([]*compRow, bool) {
		for i, x := range rs {
			if x == r {
				return slices.Delete(rs, i, i+1), true
			}
			if kids, ok := from(x.children); ok {
				x.children = kids
				return rs, true
			}
		}
		return rs, false
	}
	for _, rs := range []*[]*compRow{&a.floors, &a.items, &a.program} {
		if out, ok := from(*rs); ok {
			*rs = out
			return true
		}
	}
	return false
}

// fallbacks reads a row's and(text, text) as a chain of fallbacks
// (complSubFallbackRe, complSubNoneRe), or returns nil.
func fallbacks(n *types.ReqNode) *types.ReqNode {
	if len(n.Items) != 2 || n.Items[0].Type != types.NodeText || n.Items[1].Type != types.NodeText {
		return nil
	}
	m1 := complSubFallbackRe.FindStringSubmatch(n.Items[0].Text)
	m2 := complSubNoneRe.FindStringSubmatch(n.Items[1].Text)
	if m1 == nil || m2 == nil || m1[3] != m2[2] {
		return nil
	}
	v, _ := strconv.ParseFloat(m2[2], 64)
	category, _ := strconv.Atoi(m2[3])
	return orNode(
		&types.ReqNode{Type: types.NodeCourse, Code: m1[1]},
		textNode(types.TextOther, m1[2]),
		andNode(&types.ReqNode{Type: types.NodeCredits, Amount: v, Breadth: []int{category}}, textNode(types.TextOther, m2[1])),
		textNode(types.TextPermission, m2[4]),
	)
}
