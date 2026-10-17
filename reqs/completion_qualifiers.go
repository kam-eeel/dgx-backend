package reqs

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Floors and caps, from sentences and from the qualifiers written in an item,
// read into constraint and limit rows, the pool they leave, and course weights.

// complYHalvesRe matches "Some of the above course options are 1.0 credit
// courses, but only 0.5 credit can be counted".
var complYHalvesRe = regexp.MustCompile(`(?i)^some\s+of\s+the\s+above\s+course\s+options\s+are\s+1\.0\s+credit\s+courses,?\s+but\s+only\s+0\.5\s+credit\s+can\s+be\s+counted\b`)

// yHalves reads a complYHalvesRe sentence into a 0.5 limit on each full-year
// course of the item above it, or joins its raw when it lists none, and reports
// whether it did.
func (a *compAssembler) yHalves(s string) bool {
	if !complYHalvesRe.MatchString(a.clean(s)) {
		return false
	}
	rows := a.routeRows()
	for i := len(rows) - 1; i >= 0; i-- {
		r := rows[i]
		var ys []string
		walkReq(r.r.Req, func(x *types.ReqNode) {
			for _, c := range append(slices.Clone(x.Courses), x.Code) {
				if c != "" && courseWeight(c) == 1 {
					ys = append(ys, c)
				}
			}
		})
		if r == a.total {
			continue
		}
		if ys == nil {
			// no full-year course: the sentence caps nothing, so it is only raw
			addRaw(r, s)
			return true
		}
		for _, c := range dedupe(ys) {
			a.rowUnder(r, &types.Requirement{Kind: types.RequirementLimit, Raw: s,
				Req: &types.ReqNode{Type: types.NodeCredits, Courses: []string{c}, MaxAmount: maxAmount(0.5)}})
		}
		addRaw(r, s)
		return true
	}
	return false
}

func (a *compAssembler) constraint(target *compRow, s string) {
	for _, r := range a.floorRows(s) {
		a.rowUnder(target, r)
	}
}

// floorRows reads a floor into constraint rows, or a row of text.
func (a *compAssembler) floorRows(s string) []*types.Requirement {
	if rows, ok := readFloor(s); ok {
		return rows
	}
	return []*types.Requirement{a.textReq(s)}
}

// complMinUpToRe matches a floor and a cap on some designators' courses: "For
// CSC courses, a minimum of 1.0 credit and up to 2.0 credits must be chosen".
var complMinUpToRe = regexp.MustCompile(`(?i)^for\s+(` + desigPat + `(?:\s*(?:,|/|and|or)\s*` + desigPat + `)*)\s+courses,\s+(?:a\s+minimum\s+of|` + atLeastPat + `)\s+` + amountPat + `\s+credits?\s+and\s+(?:up\s+to|a\s+maximum\s+of|` + noMoreThanPat + `)\s+` + amountPat + `\s+credits?\s+must\s+be\s+(?:chosen|taken|completed)$`)

// readFloor reads a floor into constraint rows; ok is false when the grammar
// can't read it.
func readFloor(s string) (rows []*types.Requirement, ok bool) {
	t := complOutOfRe.ReplaceAllString(strings.TrimSpace(strings.TrimSuffix(s, ".")), "$1 credit")
	// "At least one Applications course (0.5 credit) must be ...": its credit
	t = complOneCourseFloorRe.ReplaceAllString(t, "At least $1 credit must be at the")
	if m := complMinUpToRe.FindStringSubmatch(t); m != nil {
		// a floor and a cap on the same courses
		lo, _ := strconv.ParseFloat(m[2], 64)
		hi, _ := strconv.ParseFloat(m[3], 64)
		desigs := desigRe.FindAllString(m[1], -1)
		return []*types.Requirement{
			{Kind: types.RequirementConstraint, Raw: s, Req: &types.ReqNode{Type: types.NodeCredits, Amount: lo, Designators: desigs}},
			{Kind: types.RequirementLimit, Raw: s, Req: &types.ReqNode{Type: types.NodeCredits, Designators: slices.Clone(desigs), MaxAmount: maxAmount(hi)}},
		}, true
	}
	if m := complEachOfRe.FindStringSubmatch(t); m != nil {
		// "at least 2.0 credits in each of GRK and LAT": a floor for each
		v, _ := strconv.ParseFloat(m[1], 64)
		for _, d := range desigRe.FindAllString(m[2], -1) {
			rows = append(rows, &types.Requirement{Kind: types.RequirementConstraint, Raw: s,
				Req: &types.ReqNode{Type: types.NodeCredits, Amount: v, Designators: []string{d}}})
		}
		return rows, true
	}
	if rows, tail, ok := parseHeaderFloors(t); ok && len(tail) == 0 &&
		!slices.ContainsFunc(rows, func(r *types.Requirement) bool { return r.Kind != types.RequirementConstraint || hasText(r.Req) }) {
		for _, r := range rows {
			r.Raw = s
		}
		return rows, true
	}
	n, _ := parseCompletionText(t)
	n, _ = poolCaps(n) // a floor counts what its parent claimed: no caps
	if n != nil && n.Type == types.NodeCredits && n.Items == nil && n.MaxAmount == nil {
		return []*types.Requirement{{Kind: types.RequirementConstraint, Req: n, Raw: s}}, true
	}
	return nil, false
}

// complOneCourseFloorRe matches "At least one Applications course (0.5
// credit) must be".
var complOneCourseFloorRe = regexp.MustCompile(`(?i)^` + atLeastPat + `\s+one\s+(?:[\w-]+\s+){0,3}?course\s+\(` + amountPat + `\s+credits?\)\s+must\s+be(?:\s+at\s+the)?`)

// complOutOfRe matches a floor scoped to the item: "1.0 out of the 3.5
// credits".
var complOutOfRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+out\s+of\s+the\s+` + numPat + `\s+credits\b`)

// complEachOfRe matches "at least 2.0 credits in each of GRK and LAT".
var complEachOfRe = regexp.MustCompile(`(?i)^(?:` + atLeastPat + `\s+)?` + amountPat + `\s+credits?\s+in\s+each\s+of\s+(` + desigPat + `(?:\s*(?:,|and|or|/)\s*` + desigPat + `)+)$`)

// limit reads a cap into a limit row under target: "No more than 1.0 credit
// of 100-level courses may be used". Courses that don't count are a cap of 0.
func (a *compAssembler) limit(target *compRow, s string) {
	if complCampusRe.MatchString(a.clean(s)) && a.campusRule(s) || a.yHalves(s) {
		return
	}
	if r := capRow(s); r != nil {
		a.rowUnder(target, r)
		return
	}
	if target != nil && target != a.total && a.poolAddition(target, s) {
		// "Up to 1.0 credit can be used from GGR493Y1, ... approved by department":
		// more courses for the item, not a cap on it
		return
	}
	if t := a.clean(s); (target == nil || target == a.total) && complCognateRe.MatchString(t) && complApprovalWordRe.MatchString(t) {
		// "Up to 1.0 credit in ... pre-approved cognate courses may count ...": a
		// program route, placed by what it names, not a cap
		a.programRoutes = append(a.programRoutes, s)
		return
	}
	if complPrereqRe.MatchString(a.clean(s)) {
		// "... prerequisites for these courses cannot be used towards ...": a note
		a.notesTo(a.noteTarget(target), []string{cleanNote(s)})
		return
	}
	a.textRow(target, s)
}

// complCognateRe matches courses of other departments a program accepts:
// "cognate".
var complCognateRe = regexp.MustCompile(`(?i)\bcognates?\b`)

var (
	// complLimitedToRe matches a cap worded as a limit on students: "Students
	// enrolled in the Computer Science Minor program are limited to at most ".
	complLimitedToRe = regexp.MustCompile(`(?i)^students\s+(?:enrolled\s+in\s+.+?\s+)?are\s+limited\s+to\s+(?:at\s+most|a\s+maximum\s+of)\s+`)
	// complCapLeadRe matches a cap leading a sentence: "Up to".
	complCapLeadRe = regexp.MustCompile(`(?i)^(?:up\s+to|` + noMoreThanPat + `|a\s+maximum\s+of|at\s+most)\b`)
	// complCampusRe matches the campus rule restated: "Courses taught at UTM or
	// UTSC are not included for consideration".
	complCampusRe = regexp.MustCompile(`(?i)^courses\s+taught\s+at\s+UTM\s+or\s+UTSC\s+are\s+not\s+included\s+for\s+consideration\.?$`)
	// complNotCountedRe matches courses that never count, named before the verb:
	// "STA220H1 and STA255H1 are not accepted for the major".
	complNotCountedRe = regexp.MustCompile(`(?i)\b(?:are|is)\s+not\s+(?:accepted|acceptable|included\s+for\s+consideration|an?\s+suitable\s+substitutes?\s+for)\b`)
	// complNotSubstituteRe matches "is not a suitable substitute for": the course
	// doesn't count toward what follows.
	complNotSubstituteRe = regexp.MustCompile(`(?i)\b(?:are|is)\s+not\s+an?\s+suitable\s+substitutes?\s+for\b`)
	// complNotTakenRe matches courses that never count, named after the verb:
	// "We do not accept ENG100H1".
	complNotTakenRe = regexp.MustCompile(`(?i)\b(?:are|is)\s+not\s+permitted\s+to\s+take\b|\bdo\s+not\s+accept\b`)
)

// complOnlyOfRe matches "only 0.5 of ECO220Y1 will be accepted".
var complOnlyOfRe = regexp.MustCompile(`(?i)^(?:please\s+note\s+(?:that\s+)?)?only\s+(?:a\s+)?` + amountPat + `\s+(?:credits?\s+)?of\s+(` + codePat + `)\s+(?:will\s+be|is|may\s+be|can\s+be)\s+(?:accepted|counted|used)\b`)

// complNoMoreThanOneRe matches "No more than one from GGR273H1, GGR373H1".
var complNoMoreThanOneRe = regexp.MustCompile(`(?i)^` + noMoreThanPat + `\s+one\s+(?:of|from)\s+((?:` + codePat + `)(?:\s*(?:,|or|and|/)\s*` + codePat + `)+)$`)

// complCapExceptRe matches the exception a cap states: "except for students
// ...".
var complCapExceptRe = regexp.MustCompile(`(?i)\b(except\s+(?:for|that|when|if)\b.*)$`)

// hasFilter reports whether a limit's node says what it caps: a cap on
// "credits" alone caps nothing.
func hasFilter(n *types.ReqNode) bool {
	return n != nil && (n.Courses != nil || n.Designators != nil || n.Levels != nil || n.Breadth != nil || n.Lists != nil ||
		n.Transfer || n.CrNcr || n.Items != nil || n.ExcludeDesignators != nil || n.MaxCourseAmount != 0)
}

var (
	// complCapWordingRe matches a cap sentence's lead into its amount meaning "No
	// more than": "A maximum of a 0.5 credit from".
	complCapWordingRe = regexp.MustCompile(`(?i)^-?\s*(?:students\s+(?:may|can)\s+use\s+)?(?:(?:a\s+)?maximum\s+of\s+(?:an?\s+)?|at\s+most\s+(?:an?\s+)?|only\s+(?:an?\s+)?)(` + numPat + `\s+credits?\s+(?:from|in|at)\b)`)
	// complCapAnyScopeRe matches where a cap sentence says what it counts toward:
	// " may be counted".
	complCapAnyScopeRe = regexp.MustCompile(`(?i)\s+(?:(?:can|may|will)\s+(?:be\s+)?(?:used|counted|count|applied|accepted)\b|towards?\s+(?:the\s+)?[\w&\s-]*?\b(?:program|requirements?)\b).*$`)
	// complCapNumberedScopeRe matches a cap on numbered requirements: "towards
	// requirements 5". No one limit row can say that, so the sentence stays text.
	complCapNumberedScopeRe = regexp.MustCompile(`(?i)\btowards?\s+(?:the\s+)?requirements?\s+#?\d`)
	// complCapNoteParenRe matches a pointer in brackets: "(See Note 3)".
	complCapNoteParenRe = regexp.MustCompile(`(?i)\s*\((?:see|note)\b[^()]*\)`)
	// complCodeDashTitleRe matches a course's title after it: "CRI389Y0 - Topics
	// in Criminology and Sociolegal Studies Abroad courses".
	complCodeDashTitleRe = regexp.MustCompile(`(` + codePat + `)\s+-\s+[A-Z][^,;]*?\s+courses\b`)
)

// capSentence rewrites a cap written as a sentence into the form the header
// grammar reads ("No more than 0.5 credit from A/ B"), or "" when it isn't one.
func capSentence(t string) string {
	m := complCapWordingRe.FindStringSubmatchIndex(t)
	if m == nil && !complCapLeadRe.MatchString(t) || complCapNumberedScopeRe.MatchString(t) {
		return ""
	}
	if m != nil {
		t = "No more than " + t[m[2]:]
	}
	t = complCapNoteParenRe.ReplaceAllString(t, "")
	t = complCodeDashTitleRe.ReplaceAllString(t, "$1")
	return strings.TrimSpace(complCapAnyScopeRe.ReplaceAllString(t, ""))
}

func capRow(s string) *types.Requirement {
	t := complLimitedToRe.ReplaceAllString(strings.TrimSpace(strings.TrimSuffix(s, ".")), "No more than ")
	for _, u := range []string{t, complCapScopeRe.ReplaceAllString(t, ""), capSentence(t)} {
		rows, tail, ok := parseHeaderFloors(u)
		if ok && len(tail) == 0 && len(rows) == 1 && rows[0].Kind == types.RequirementLimit && hasFilter(rows[0].Req) {
			rows[0].Raw = s
			if m := complCapExceptRe.FindStringSubmatch(s); m != nil {
				// a limit can't hold an alternative: the exception is a note
				rows[0].Notes = []string{cleanNote(m[1])}
			}
			return rows[0]
		}
	}
	if m := complNoMoreThanOneRe.FindStringSubmatch(t); m != nil {
		// at most one course
		return &types.Requirement{Kind: types.RequirementLimit, Raw: s, Req: courseCap(codeRe.FindAllString(m[1], -1))}
	}
	if m := complOnlyOfRe.FindStringSubmatch(t); m != nil {
		// "Please note only 0.5 of ECO220Y1 will be accepted ..."
		v, _ := strconv.ParseFloat(m[1], 64)
		return &types.Requirement{Kind: types.RequirementLimit, Raw: s,
			Req: &types.ReqNode{Type: types.NodeCredits, Courses: []string{m[2]}, MaxAmount: maxAmount(v)}}
	}
	var codes []string
	if loc := complNotCountedRe.FindStringIndex(t); loc != nil && (!codeRe.MatchString(t[loc[1]:]) || complNotSubstituteRe.MatchString(t)) {
		codes = codeRe.FindAllString(t[:loc[0]], -1)
	} else if loc := complNotTakenRe.FindStringIndex(t); loc != nil && !codeRe.MatchString(t[:loc[0]]) {
		codes = codeRe.FindAllString(t[loc[1]:], -1)
	}
	if codes == nil {
		return nil
	}
	return &types.Requirement{Kind: types.RequirementLimit, Raw: s,
		Req: &types.ReqNode{Type: types.NodeCredits, Courses: dedupe(codes), MaxAmount: maxAmount(0)}}
}

// splitListTails splits text at the clauses that floor or cap its pool: "2.0
// credits from Groups A and/or B, including at least 0.5 credit from a
// 400-level CSC course" is the pool and one floor.
func splitListTails(text string) (head string, tails []string) {
	text = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(text), "."))
	locs := complListTailRe.FindAllStringIndex(text, -1)
	if locs == nil {
		return text, nil
	}
	head = text[:locs[0][0]]
	for i, l := range locs {
		end := len(text)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		tail := strings.TrimSpace(strings.Trim(strings.TrimSpace(text[l[0]:end]), ",;"))
		tail = strings.TrimSpace(strings.TrimPrefix(tail, "("))
		if strings.Count(tail, ")") > strings.Count(tail, "(") {
			tail = strings.TrimSpace(strings.Replace(tail, ")", "", 1))
		}
		tails = append(tails, tail)
	}
	return strings.TrimSpace(head), tails
}

// listRefRule reads a floor or cap clause into constraint or limit rows: one,
// or one for each list of "from each of Groups A-C". within is the outer
// floor's filter when the clause is a floor inside it, which it narrows.
func listRefRule(clause string, ls compListRefs, within *types.ReqNode) (rows []*types.Requirement, notes []string, ok bool) {
	c := strings.TrimSpace(clause)
	if m := complListCapLeadRe.FindString(c); m != "" {
		n, notes, ok := listRefPool(c[len(m):], ls, true)
		if !ok {
			return nil, nil, false
		}
		n.MaxAmount, n.Amount = maxAmount(n.Amount), 0
		return []*types.Requirement{{Kind: types.RequirementLimit, Req: n, Raw: cleanNote(clause)}}, notes, true
	}
	c = c[len(complListFloorLeadRe.FindString(c)):]
	c = complFloorArticleRe.ReplaceAllString(c, "$1")
	c = complFloorBeingRe.ReplaceAllString(c, "$1 credits at the ")
	c = complFloorAtRe.ReplaceAllString(c, "$1 credits at ")
	c = complMustBeACourseRe.ReplaceAllString(c, "$1 at the $2")
	c = complFloorWithRe.ReplaceAllString(c, "$1 credits in ")
	var each []string
	if m := complEachOfListsRe.FindStringSubmatchIndex(c); m != nil {
		// "a 0.5 credit from each of Groups A-C": a floor on each list
		if each = ls.resolve(listRefNames(c[m[2]:m[3]])); each == nil {
			return nil, nil, false
		}
		c = c[:m[0]] + c[m[1]:]
	}
	n, notes, ok := listRefPool(c, ls, true)
	if m := complBareAmountRe.FindStringSubmatch(strings.TrimSpace(c)); !ok && each != nil && m != nil {
		// the lists are the source
		v, _ := strconv.ParseFloat(m[1], 64)
		n, notes, ok = &types.ReqNode{Type: types.NodeCredits, Amount: v}, nil, true
	}
	if !ok || each != nil && n.Lists != nil {
		return nil, nil, false
	}
	if within != nil {
		if n = narrowFilter(n, within); n == nil {
			return nil, nil, false
		}
	}
	if each == nil {
		return []*types.Requirement{{Kind: types.RequirementConstraint, Req: n, Raw: cleanNote(clause)}}, notes, true
	}
	for _, name := range each {
		f := cloneNode(n)
		f.Lists = []string{name}
		rows = append(rows, &types.Requirement{Kind: types.RequirementConstraint, Req: f, Raw: cleanNote(clause)})
	}
	return rows, notes, true
}

var (
	// complEachOfListsRe matches "from each of Groups A-C".
	complEachOfListsRe = regexp.MustCompile(`(?i)\s*\bfrom\s+each\s+of\s+(` + complListRefPat + `)`)
	// complFloorBeingRe matches "1.0 being at the at the "; the calendar may
	// double the words.
	complFloorBeingRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+(?:credits?\s+)?being\s+(?:at\s+the\s+)+`)
	// complMustBeACourseRe matches the floor's level in "1.0 credit must be a
	// 300/400-level course".
	complMustBeACourseRe = regexp.MustCompile(`(?i)^(` + numPat + `\s+credits?)\s+must\s+be\s+an?\s+(` + levelPat + `(?:\s*[-/+]\s*[1-4]?0?0?)*\s*-?\s*level)\s+courses?\b`)
	// complFloorAtRe matches a floor's amount without its unit before a level:
	// "0.5 at ".
	complFloorAtRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+at\s+`)
	// complBareAmountRe matches an amount and nothing else: "0.5 credit".
	complBareAmountRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?$`)
	// complFloorArticleRe matches the article before a floor's amount: "a 0.5 ".
	complFloorArticleRe = regexp.MustCompile(`(?i)^an?\s+(` + numPat + `\s)`)
	// complFloorWithRe matches a floor's amount without its unit before what it
	// counts: "2.5 with ".
	complFloorWithRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+(?:with|in|from)\s+`)
)

// narrowFilter returns floor n within filter w: the courses matching both,
// or nil when they can't be combined into one filter.
func narrowFilter(n, w *types.ReqNode) *types.ReqNode {
	out := cloneNode(n)
	if out.Lists == nil {
		out.Lists = slices.Clone(w.Lists)
	} else if w.Lists != nil && !sameSet(out.Lists, w.Lists) {
		return nil
	}
	switch {
	case out.Designators == nil:
		out.Designators = slices.Clone(w.Designators)
	case w.Designators != nil:
		out.Designators = slices.DeleteFunc(out.Designators, func(d string) bool { return !slices.Contains(w.Designators, d) })
		if out.Designators == nil {
			return nil
		}
	}
	switch {
	case out.Levels == nil:
		out.Levels = slices.Clone(w.Levels)
	case w.Levels != nil:
		out.Levels = slices.DeleteFunc(out.Levels, func(l int) bool { return !slices.Contains(w.Levels, l) })
		if out.Levels == nil {
			return nil
		}
	}
	return out
}

// complOfWhichRe matches a floor within the floor before it: "of which".
var complOfWhichRe = regexp.MustCompile(`(?i)\bof\s+which\b`)

var (
	// complQualifiedHeadRe matches a pool's lead before its courses once its
	// floors are out: "3.5 credits from the Core Group:".
	complQualifiedHeadRe = regexp.MustCompile(`(?i)^(?:` + atLeastPat + `\s+)?(` + numPat + `\s+(?:additional\s+)?credits?)\s+(?:of\s+(?:electives|courses)\s+)?(?:(?:chosen|selected|taken)\s+)?(?:from\s+)?(?:the\s+following(?:\s+(?:courses|list))?(?:\s+below)?|the\s+(?:core|following)\s+group|any\s+of\s+the\s+following)?\s*:\s*`)
	// complFromCommaRe matches ", from" left once a floor between the amount and
	// its pool is out: "1.5 credits, from ".
	complFromCommaRe = regexp.MustCompile(`(?i)^(` + numPat + `\s+credits?)\s*,\s*from\s+`)
)

// qualifiedPool reads the pool of courses left once its floors and caps are
// taken out, with the caps on its "/" pairs.
func qualifiedPool(head string) (pool *types.ReqNode, caps []*types.ReqNode, notes []string, ok bool) {
	h := complFromCommaRe.ReplaceAllString(head, "$1 from ")
	if m := complQualifiedHeadRe.FindStringSubmatch(h); m != nil {
		h = m[1] + " from: " + h[len(m[0]):]
	}
	n, notes := parseCompletionText(h)
	if n = checkedBody(n, h); n == nil || hasText(n) {
		return nil, nil, nil, false
	}
	if pool, caps = poolCaps(n); !isPool(pool) {
		return nil, nil, nil, false
	}
	return pool, caps, notes, true
}

// splitQualified takes the floors and caps out of a pool's text, wherever
// they are: after it, between its lead and its courses, between its amount
// and its source, or in brackets.
func splitQualified(t string) (rest string, tails []string) {
	t = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(t), "."))
	locs := complListTailRe.FindAllStringIndex(t, -1)
	if locs == nil {
		return t, nil
	}
	var b strings.Builder
	prev := 0
	for i, l := range locs {
		if l[0] < prev {
			continue
		}
		b.WriteString(t[prev:l[0]])
		end := len(t)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		seg := t[l[0]:end]
		paren := strings.HasPrefix(strings.TrimSpace(seg), "(")
		if paren {
			// a bracketed floor runs to its closing bracket
			if k := strings.Index(t[l[0]:], ")"); k >= 0 {
				end = l[0] + k + 1
				seg = t[l[0]:end]
			}
		} else if m := complTailEndRe.FindStringIndex(seg); m != nil {
			end = l[0] + m[0]
			seg = t[l[0]:end]
		}
		if paren {
			inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(seg), "("), ")"))
			_, more := splitListTails("x " + inner)
			tails = append(tails, more...)
		} else {
			tails = append(tails, strings.TrimSpace(strings.Trim(strings.TrimSpace(seg), ",;")))
		}
		prev = end
	}
	b.WriteString(t[prev:])
	return strings.TrimSpace(b.String()), tails
}

// complTailEndRe matches where a floor between a pool's lead and its courses
// ends: ": ".
var complTailEndRe = regexp.MustCompile(`(?i):\s*|,\s*from\s+`)

// complUpToRe matches a cap that widens the pool with a share: "up to".
var complUpToRe = regexp.MustCompile(`(?i)^(?:;\s*)?up\s+to\b`)

// complWeightRe matches a full-year course counting part of its weight
// toward a requirement: "If students complete CSC110Y1, 0.5 credit from the
// course contributes to the First Year requirements".
var complWeightRe = regexp.MustCompile(`(?i)^if\s+students\s+(?:complete|take)\s+(` + codePat + `),\s+` + amountPat + `\s+credits?\s+from\s+the\s+course\s+contributes?\s+to\s+(?:the\s+)?(?:group\s+[A-Z]\s+in\s+)?(.+?)\s*\.?$`)

// complRequirementLabelRe matches "requirement 2".
var complRequirementLabelRe = regexp.MustCompile(`(?i)^requirement\s+(\d+)$`)

// weights places complWeightRe sentences on the rows they name: the course
// counts only that much there and the rest can count elsewhere. A sentence
// whose row doesn't list the course stays.
func (a *compAssembler) weights(ls compListRefs) {
	place := func(s string) bool {
		m := complWeightRe.FindStringSubmatch(complClean(s))
		if m == nil {
			return false
		}
		w, _ := strconv.ParseFloat(m[2], 64)
		target := strings.ToLower(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(m[3]), "s"), " requirement"))
		label := ""
		if lm := complRequirementLabelRe.FindStringSubmatch(target); lm != nil {
			label = lm[1] // the row labelled 2
		}
		var rows []*compRow
		walkRows(a.items, func(r *compRow) {
			if r.r.Kind == types.RequirementAllocation && (label == "" && strings.ToLower(r.r.Section) == target || label != "" && r.r.Label == label) {
				rows = append(rows, r)
			}
		})
		done := false
		for _, r := range rows {
			if a.weigh(r, m[1], w, ls) {
				addRaw(r, cleanNote(s))
				done = true
			}
		}
		return done
	}
	a.notes = slices.DeleteFunc(a.notes, place)
	walkRows(a.items, func(r *compRow) {
		r.r.Notes = slices.DeleteFunc(r.r.Notes, place)
		if n := r.r.Req; n != nil && n.Type == types.NodeOr {
			n.Items = slices.DeleteFunc(n.Items, func(it *types.ReqNode) bool { return it.Type == types.NodeText && place(it.Text) })
			if len(n.Items) == 1 {
				r.r.Req = n.Items[0]
			}
		}
	})
}

// weigh makes code count w toward row and reports whether row lists it.
func (a *compAssembler) weigh(row *compRow, code string, w float64, ls compListRefs) bool {
	weighted := &types.ReqNode{Type: types.NodeCredits, Amount: w, Courses: []string{code}, Weight: w}
	found := false
	var walk func(n *types.ReqNode)
	walk = func(n *types.ReqNode) {
		if n.Type != types.NodeOr && n.Type != types.NodeAnd {
			return
		}
		for i, it := range n.Items {
			if it.Type == types.NodeCourse && it.Code == code && it.MinGrade == 0 {
				n.Items[i] = cloneNode(weighted)
				found = true
			}
			walk(it)
		}
	}
	walk(row.r.Req)
	if found {
		return true
	}
	pool := row.r.Req
	if pool != nil && pool.Type == types.NodeOr && len(pool.Items) > 0 {
		pool = pool.Items[0]
	}
	if pool == nil || !isPool(pool) || pool.Designators != nil || pool.Levels != nil || pool.Lists != nil || !slices.Contains(pool.Courses, code) || row.r.Label == "" {
		return false
	}
	// in a pool of listed courses: the pool as a list, the course weighted
	others := slices.DeleteFunc(slices.Clone(pool.Courses), func(c string) bool { return c == code })
	name, _, _ := a.addUnion(row, []*types.ReqNode{{Type: types.NodeCredits, Courses: others}, {Type: types.NodeCredits, Courses: []string{code}, Weight: w}}, ls)
	pool.Courses, pool.Lists = nil, []string{name}
	return true
}

// complSeparatorCapRe matches a "/" run that pairCaps capped but may be the
// list's separator: "at least 1.0 from PSY320H1/ PSY321H1/ PSY322H1,
// PSY324H1/" (ASSPE2270), "1.0 credit from HMB430H1/ ...; JPM400Y1;" (ASMAJ0482).
var complSeparatorCapRe = regexp.MustCompile(`(?:\b` + atLeastPat + `\s+1\.0\s+from|\b1\.0\s+credit\s+from)\s+((?:` + codePat + `\s*/\s*){2}` + codePat + `)\s*(?:,\s*PSY324H1\s*/|;\s*JPM400Y1\s*;)`)

// separatorCaps reads complSeparatorCapRe by the ambiguity policy: the cap is
// strict (Met), the group as plain courses lenient (Review). The limit goes,
// and its pool P becomes or(and(P, P less the cap from its other courses),
// and(P, text{ambiguity})).
func (a *compAssembler) separatorCaps(roots []*compRow, lists []*types.CourseList) {
	byName := listsByName(lists)
	for _, u := range a.rowUnions {
		byName[u.list.Name] = u.list // named after numbering (nameRowUnions)
	}
	walkRowsPost(roots, func(r *compRow) {
		for _, k := range slices.Clone(r.children) {
			l := k.r.Req
			if k.r.Kind != types.RequirementLimit || l == nil || !isCourseCap(l) || len(l.Courses) < 2 {
				continue
			}
			m := complSeparatorCapRe.FindStringSubmatch(k.r.Raw)
			if m == nil || !slices.Equal(codeRe.FindAllString(m[1], -1), l.Courses) {
				continue
			}
			n, ok := a.separatorPool(r.r.Req, l.Courses, *l.MaxAmount, byName)
			if !ok {
				continue
			}
			r.r.Req = n
			r.children = slices.DeleteFunc(r.children, func(x *compRow) bool { return x == k })
		}
	})
}

// separatorPool returns n with the pool that names every course of group,
// above the cap, read both ways; ok is false when n has no such pool.
func (a *compAssembler) separatorPool(n *types.ReqNode, group []string, capAmount float64, byName map[string]*types.CourseList) (*types.ReqNode, bool) {
	if n == nil {
		return nil, false
	}
	if n.Type == types.NodeAnd || n.Type == types.NodeOr {
		for i, it := range n.Items {
			if x, ok := a.separatorPool(it, group, capAmount, byName); ok {
				n.Items[i] = x
				return n, true
			}
		}
		return n, false
	}
	if n.Type != types.NodeCredits || n.MaxAmount != nil || n.Amount <= capAmount+1e-9 || n.Exclude != nil || n.Items != nil {
		return n, false
	}
	// its lists may hold filters too: "PSL400-series courses"
	codes := slices.Clone(n.Courses)
	for _, name := range n.Lists {
		l := byName[name]
		if l == nil {
			return n, false
		}
		for _, it := range l.Items {
			if it.Type == types.NodeCredits && it.Exclude == nil {
				codes = append(codes, it.Courses...)
			}
		}
	}
	if slices.ContainsFunc(group, func(c string) bool { return !slices.Contains(codes, c) }) {
		return n, false
	}
	rest := cloneNode(n)
	rest.Amount = n.Amount - capAmount
	if n.Lists == nil {
		rest.Courses = slices.DeleteFunc(rest.Courses, func(c string) bool { return slices.Contains(group, c) })
	} else {
		rest.Exclude = slices.Clone(group)
	}
	text := fmt.Sprintf("the \"/\" in %s may separate the pool's courses rather than join alternatives, so more than one of them may count", strings.Join(group, "/ "))
	return orNode(andNode(n, rest), andNode(cloneNode(n), textNode(types.TextAmbiguity, text))), true
}
