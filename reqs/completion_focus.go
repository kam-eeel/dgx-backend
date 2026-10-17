package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// A focus's own requirements, for ASSPE2270: its foci ASFOC2270A-Q aren't in
// the programs table, so each focus's line in its text is read here.

var (
	// complFocusAmountRe matches a focus's amount after its code: "(2.0
	// credits)".
	complFocusAmountRe = regexp.MustCompile(`(?i)^\(` + amountPat + `(?:\s+or\s+` + numPat + `)?\s+credits?\)\s*`)
	// complFocusWordings rewrites a focus line's wordings into the item
	// grammar's: "with a combined minimum grade average of" is "with an average
	// grade of".
	complFocusWordings = []struct {
		re   *regexp.Regexp
		with string
	}{
		{regexp.MustCompile(`(?i)\bwith\s+a\s+combined\s+minimum\s+grade\s+average\s+of\b`), "with an average grade of"},
		{regexp.MustCompile(`(?i)\b(` + numPat + `\s+credits?)\s+from\s+(\([^()]*\b` + atLeastPat + `\b[^()]*\))`), "$1 $2 from"},
		{regexp.MustCompile(`(?i)\b(` + numPat + `\s+credits?)\s+(\d00\+)\s+series\s+(` + desigPat + `)\s+courses?\s+with\s+the\s+exception\s+of\b`), "$1 from $2 level $3 courses excluding"},
	}
)

// focusLineAt returns the focus line at a.lines[i] split into head and
// requirement; a line ending at its amount takes the next line, if no focus.
func (a *compAssembler) focusLineAt(i int, m []string) (head, body string) {
	l := strings.TrimSpace(a.lines[i])
	head, body = l, ""
	if am := complFocusAmountRe.FindStringIndex(strings.TrimSpace(l[len(m[0]):])); am != nil {
		rest := strings.TrimSpace(l[len(m[0]):])
		head, body = strings.TrimSpace(l[:len(m[0])])+" "+strings.TrimSpace(rest[:am[1]]), strings.TrimSpace(rest[am[1]:])
	}
	if body == "" && i+1 < len(a.lines) {
		next := strings.TrimSpace(a.lines[i+1])
		if !complFocusCodeRe.MatchString(next) && !complFocusNoCodeRe.MatchString(next) {
			body = next
		}
	}
	return head, body
}

// focusPart returns the focus alternative and(p, requirement), with its notes
// and limits, or the fallback and(p, text{line}) when the grammar can't read it
// all. A requirement short of the amount gets the amount from all its courses
// beside it.
func (a *compAssembler) focusPart(row *compRow, p *types.ReqNode, head, body string) (part, fallback *types.ReqNode, notes []string, limits []*types.Requirement) {
	note := cleanNote(head + " " + body)
	lead := rawWords(p.Text + " (" + p.Programs[0] + "): (")
	all := append(slices.Clone(row.r.Notes), a.notes...)
	if j := slices.IndexFunc(all, func(s string) bool {
		w := rawWords(s)
		return strings.HasPrefix(w, lead) && codeRe.FindString(w[len(lead):]) == codeRe.FindString(body)
	}); j >= 0 {
		note = all[j]
	}
	addRaw(row, note)
	a.dropNoteAll(note)
	fallback = andNode(p, textNode(types.TextOther, note))
	am := complFocusAmountRe.FindStringSubmatch(strings.TrimSpace(head[strings.LastIndex(head, ":")+1:]))
	if am == nil || body == "" {
		return fallback, fallback, nil, nil
	}
	amount, _ := strconv.ParseFloat(am[1], 64)
	for _, w := range complFocusWordings {
		body = w.re.ReplaceAllString(body, w.with)
	}
	n, rows, ns, ok := a.focusBody(body, note)
	if !ok {
		// read the parts between semicolons one at a time
		n, rows, ns = nil, nil, nil
		for _, s := range splitTop(body, ';') {
			x, rs, xs, ok := a.focusBody(strings.TrimPrefix(strings.TrimSpace(s), "and "), note)
			if !ok {
				return fallback, fallback, nil, nil
			}
			n, rows, ns = andNode(n, x), append(rows, rs...), append(ns, xs...)
		}
	}
	got, ok := focusCredits(n)
	switch {
	case !ok || got > amount+1e-9:
		return fallback, fallback, nil, nil
	case got < amount-1e-9:
		// the text can't say which courses make the rest
		codes := dedupe(nodeCodes(n))
		if !listedOnly(n) || len(codes) == 0 {
			return fallback, fallback, nil, nil
		}
		n = andNode(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: codes}, n)
	}
	for _, s := range ns {
		notes = append(notes, cleanNote(p.Text+" ("+p.Programs[0]+"): "+s))
	}
	return andNode(p, n), fallback, notes, rows
}

// focusBody reads s, a focus's requirement or a part of it, with the item
// grammar. ok is false when the node has text or a row other than a limit is
// left.
func (a *compAssembler) focusBody(s, raw string) (n *types.ReqNode, limits []*types.Requirement, notes []string, ok bool) {
	n, rows, notes := a.itemBody(s)
	if n == nil || hasText(n) {
		return nil, nil, nil, false
	}
	n, rows = poolFloors(n, rows)
	inner := &compRow{r: &types.Requirement{Req: n, Raw: raw}}
	splitCaps(inner)
	for _, k := range inner.children {
		rows = append(rows, k.r)
	}
	for _, r := range rows {
		if r.Kind != types.RequirementLimit {
			return nil, nil, nil, false
		}
	}
	return inner.r.Req, rows, notes, true
}

// poolFloors moves a pool's level floors into its node as credits from its
// courses at those levels: a constraint row would hold for every focus.
func poolFloors(n *types.ReqNode, rows []*types.Requirement) (*types.ReqNode, []*types.Requirement) {
	var keep []*types.Requirement
	for _, r := range rows {
		f := r.Req
		if r.Kind != types.RequirementConstraint || n == nil || n.Type != types.NodeCredits || len(n.Courses) == 0 ||
			f == nil || f.Type != types.NodeCredits || f.Levels == nil || f.Courses != nil || f.Designators != nil || f.Items != nil || f.Requirements != nil {
			keep = append(keep, r)
			continue
		}
		var at []string
		for _, c := range n.Courses {
			if slices.Contains(f.Levels, courseLevel(c)) {
				at = append(at, c)
			}
		}
		if len(at) == 0 {
			keep = append(keep, r)
			continue
		}
		n = andNode(n, &types.ReqNode{Type: types.NodeCredits, Amount: f.Amount, Courses: at})
	}
	return n, keep
}

// focusCredits is nodeCredits, except that an average and a floor on some of a
// pool's courses add nothing.
func focusCredits(n *types.ReqNode) (float64, bool) {
	if n.Type != types.NodeAnd {
		return nodeCredits(n)
	}
	floor := func(it *types.ReqNode) bool {
		return it.Type == types.NodeCredits && len(it.Courses) > 0 && slices.ContainsFunc(n.Items, func(p *types.ReqNode) bool {
			return p != it && p.Type == types.NodeCredits && len(p.Courses) > len(it.Courses) &&
				!slices.ContainsFunc(it.Courses, func(c string) bool { return !slices.Contains(p.Courses, c) })
		})
	}
	sum := 0.0
	for _, it := range n.Items {
		if it.Type == types.NodeAverage || floor(it) {
			continue
		}
		c, ok := nodeCredits(it)
		if !ok {
			return 0, false
		}
		sum += c
	}
	return sum, true
}

// listedOnly reports whether n names courses only: courses, and pools of listed
// courses with no other filter.
func listedOnly(n *types.ReqNode) bool {
	return structureOnly(n, func(x *types.ReqNode) bool {
		return x.Type == types.NodeCredits && len(x.Courses) > 0 && x.Designators == nil && x.Levels == nil && x.Breadth == nil && x.Exclude == nil && x.Items == nil && x.Requirements == nil
	})
}

// focusLimitsSafe reports whether the limits a focus adds under the focus row,
// which apply whichever focus is taken, still let every other focus count the
// credits it needs from the limited courses.
func focusLimitsSafe(limits []*types.Requirement, others []*types.ReqNode) bool {
	for _, l := range limits {
		set := map[string]bool{}
		for _, c := range l.Req.Courses {
			set[c] = true
		}
		if l.Req.MaxAmount == nil || len(set) == 0 || l.Req.Designators != nil || l.Req.Levels != nil {
			return false
		}
		for _, o := range others {
			if need, ok := sharedNeed(o, set); !ok || need > *l.Req.MaxAmount+1e-9 {
				return false
			}
		}
	}
	return true
}

// sharedNeed returns the most credits from set that n may need. ok is false for
// a node it can't weigh, such as an average over those courses.
func sharedNeed(n *types.ReqNode, set map[string]bool) (float64, bool) {
	switch n.Type {
	case types.NodeProgram, types.NodeText:
		return 0, true
	case types.NodeCourse:
		if set[n.Code] {
			return courseWeight(n.Code), true
		}
		return 0, true
	case types.NodeCredits:
		if slices.ContainsFunc(n.Courses, func(c string) bool { return set[c] }) {
			return n.Amount, n.Items == nil
		}
		return 0, n.Items == nil
	case types.NodeAverage:
		return 0, !slices.ContainsFunc(n.Courses, func(c string) bool { return set[c] })
	case types.NodeAnd, types.NodeOr:
		total := 0.0
		for _, it := range n.Items {
			x, ok := sharedNeed(it, set)
			if !ok {
				return 0, false
			}
			if n.Type == types.NodeAnd {
				total += x
			} else {
				total = max(total, x)
			}
		}
		return total, true
	}
	return 0, false
}
