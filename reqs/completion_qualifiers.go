package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Item qualifiers: the floors and caps written in an item over its pool, read
// into constraint and limit rows, the pool they leave, and course weights.

// complHalfCoursesCapRe matches a cap on a list counted in half courses, with
// its credits: "The total number of half courses from Group I must not exceed
// 3 (1.5 credits)".
var complHalfCoursesCapRe = regexp.MustCompile(`(?i)^the\s+total\s+number\s+of\s+(?:half\s+)?courses\s+from\s+(` + complListRefPat + `)(?:\s*\(\s*including\s+[^()]*\))?\s+must\s+not\s+exceed\s+\w+\s*\(\s*` + amountPat + `\s+credits?\s*\)$`)

// complDesignatorPhraseRe matches a floor's designators in words: "carrying
// the CHC/JCA/JCR designator".
var complDesignatorPhraseRe = regexp.MustCompile(`(?i)\s+(?:must\s+(?:have|carry)|carrying|having|with)\s+(?:the\s+|an?\s+)?(` + desigPat + `(?:\s*/\s*` + desigPat + `)*)\s+designators?\b`)

// complExcludingLevelRe matches "excluding 100-level courses".
var complExcludingLevelRe = regexp.MustCompile(`(?i)^excluding\s+(` + complListLevelsPat + `)\s*-?\s*level\s+courses$`)

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
