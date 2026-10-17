package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// A focus's own requirements (ASSPE2270, decision 2026-10-17). The foci
// ASFOC2270A-Q are not in the programs table: the calendar writes their
// requirements only in the Specialist's text, one line each, "Health Studies
// (ASFOC2270A): (2.0 credits) HMB342H1, at least 0.5 credit from ...". So
// focusChoices makes each focus's alternative and(program{focus}, its line's
// requirement), read with the item grammar (focusPart), or and(program,
// text{the line}) where the grammar can't read all of it. The alternative is
// the focus row's: an allocation, so what it claims is claimed against the
// Specialist's other rows ("plus a disciplinary focus requiring 2.0-3.5
// credits": the focus's credits are on top of the Specialist's).

var (
	// complFocusAmountRe: the focus's amount after its code: "(2.0 credits)",
	// "(2.5 or 3.0 credits)".
	complFocusAmountRe = regexp.MustCompile(`(?i)^\((\d+(?:\.\d+)?)(?:\s+or\s+\d+(?:\.\d+)?)?\s+credits?\)\s*`)
	// complFocusWordings: a focus line's wordings the item grammar writes
	// otherwise. "( SOC100H1, SOC150H1) with a combined minimum grade
	// average of 65%" is "with an average grade of 65%"; "0.5 credit 300+
	// series ECO course with the exception of ECO374H1 and ECO375H1" is "0.5
	// credit from 300+ level ECO courses excluding ..."; "2.0 credits from
	// (with at least a 0.5 credit at the 400-level) ..." is "2.0 credits
	// (with ...) from ...".
	complFocusWordings = []struct {
		re   *regexp.Regexp
		with string
	}{
		{regexp.MustCompile(`(?i)\bwith\s+a\s+combined\s+minimum\s+grade\s+average\s+of\b`), "with an average grade of"},
		{regexp.MustCompile(`(?i)\b(\d+(?:\.\d+)?\s+credits?)\s+from\s+(\([^()]*\bat\s+least\b[^()]*\))`), "$1 $2 from"},
		{regexp.MustCompile(`(?i)\b(\d+(?:\.\d+)?\s+credits?)\s+(\d00\+)\s+series\s+((?-i:[A-Z]{3}))\s+courses?\s+with\s+the\s+exception\s+of\b`), "$1 from $2 level $3 courses excluding"},
	}
)

// focusLineAt returns the focus line at a.lines[i] with its requirement: the
// rest of the line, or, when the line ends at the amount ("Biochemistry
// (ASFOC2270O): (3.0 credits)"), the next line, when that is no focus.
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

// focusPart is the alternative of the focus (its program node p) whose line
// is head and body: and(p, the requirement), its notes and limits (the rows
// it adds under the focus row), or and(p, text{the line}) when the item
// grammar leaves text, a row it can't place, or an amount the requirement
// doesn't make. A requirement that falls short of the amount ("(2.0 credits)
// HMB342H1, at least 0.5 credit from A, and at least 0.5 credit from B") has
// the amount from all its courses beside it. The focus's note (focusLine:
// the line) goes; the row's raw holds it.
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
		// "BIO120H1, BIO220H1; 2.0 credits from (with at least a 0.5 credit
		// at the 400-level) ...": its parts, one at a time.
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
		// The amount from the requirement's courses, beside it: it can't
		// say which courses make the rest.
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
// grammar: its node, its limits (the caps beside a pool, from a group of
// alternatives in it: "at least 1.0 from PSY320H1/ PSY321H1/ PSY322H1,
// PSY324H1/ ..."), and its notes. ok is false when the node has text or a
// row other than a limit is left (poolFloors).
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

// poolFloors puts a pool's floor rows into the pool's node, as the pool's
// courses at the floor's levels beside it: "2.0 credits from (with at least
// a 0.5 credit at the 400-level) EEB319H1/ ..., EEB428H1/ EEB440H1 or
// EEB240H1/ ENV432H1" is and(credits{2.0, the courses}, credits{0.5,
// EEB428H1, EEB440H1, ENV432H1}). A focus's
// requirement is an alternative of the focus row, where a constraint row
// would hold for every focus. Other rows are kept.
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

// focusCredits is nodeCredits with an average beside the courses it is over
// ("( SOC100H1, SOC150H1) with an average grade of 65%") and a floor on a
// pool beside it, some of its courses (poolFloors), which add none.
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

// listedOnly reports whether n names courses only: courses, and pools of
// listed courses with no other filter.
func listedOnly(n *types.ReqNode) bool {
	ok := true
	walkReq(n, func(x *types.ReqNode) {
		switch x.Type {
		case types.NodeAnd, types.NodeOr, types.NodeCourse:
		case types.NodeCredits:
			ok = ok && len(x.Courses) > 0 && x.Designators == nil && x.Levels == nil && x.Breadth == nil && x.Exclude == nil && x.Items == nil && x.Requirements == nil
		default:
			ok = false
		}
	})
	return ok
}

// focusLimitsSafe reports whether the limits a focus's requirement adds
// under the focus row (they trim what the row counts, whichever focus the
// student completes) leave every other focus's alternative (others) able to
// count what it needs from the limited courses: at most the limit's amount.
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

// sharedNeed is the most credits from set that n may need: a course's
// weight, a pool's amount when it lists one of them, the heaviest of an or's
// alternatives. ok is false for a node it can't weigh (text over the
// courses, an average).
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
