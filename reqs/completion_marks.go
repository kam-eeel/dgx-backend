package reqs

import (
	"slices"

	"degree-explorer/types"
)

// Review markers: text alternatives on the counts the numbered rows can't be
// seen to make up, so they are Review rather than NotMet.

// markIncomplete marks each count c's rows can't be seen to make up with
// or(count, its sentence): a total over an incomplete child or, with
// shortfall, items that fall short of it; a constraint under an incomplete
// count. Text claims nothing. total says row 1 is the header total.
func markIncomplete(c *types.Completion, total, shortfall bool) {
	m := newMarkCauses(c)
	kids, incomplete, causes, memo := m.kids, m.incomplete, m.causes, m.memo
	mark := func(r *types.Requirement, why []*types.ReqNode) {
		head := textNode(types.TextOther, complItemLabelRe.ReplaceAllString(rawHead(r), "")) // the sentence, not its item number
		if r.Kind == types.RequirementConstraint && r.Req != nil && r.Req.Type == types.NodeAnd &&
			slices.ContainsFunc(r.Req.Items, func(x *types.ReqNode) bool { return x.Type == types.NodeText }) {
			// and(floor, text) is Review at best: the marker replaces it,
			// taking its own text's kind.
			head.Kind = markerKind(r.Req.Items)
			r.Req = head
			return
		}
		head.Kind = markerKind(why)
		r.Req = orNode(r.Req, head)
	}
	if total && len(c.Requirements) > 0 {
		t := c.Requirements[0]
		n := t.Req
		pool := n.Type != types.NodeCredits || n.Designators != nil || n.Levels != nil || n.Courses != nil || n.Lists != nil
		sum, claims := 0.0, false
		chosen := chosenRows(c)
		for _, k := range kids[1] {
			if k.Kind == types.RequirementAllocation && !chosen[k.Number] {
				claims = true
				v, _ := rowCredits(k, kids)
				sum += v
			}
		}
		// Options count through their choice, at its cheapest.
		for _, k := range kids[0] {
			if k.Kind == types.RequirementChoice {
				claims = true
				sum += choiceCredits(k.Req, c, kids)
			}
		}
		if !reviewMarked(n) && (incomplete(t) || shortfall && !pool && (!claims || sum < n.Amount-1e-9)) {
			var why []*types.ReqNode // nil for a shortfall: kind other
			if incomplete(t) {
				why = causes(t)
			}
			mark(t, why)
			// Its constraints are marked too, for the same causes.
			memo[1] = true
		}
	}
	var top []*types.ReqNode // with no total: the open texts of incomplete top-level allocations
	for _, r := range kids[0] {
		if !total && r.Kind == types.RequirementAllocation && incomplete(r) {
			top = append(top, causes(r)...)
		}
	}
	for _, r := range c.Requirements {
		// A constraint over the transcript doesn't check its parent's count.
		if r.Kind != types.RequirementConstraint || r.Scope != "" || reviewMarked(r.Req) {
			continue
		}
		switch {
		case r.Parent != 0 && incomplete(c.Requirements[r.Parent-1]):
			mark(r, causes(c.Requirements[r.Parent-1]))
		case r.Parent == 0 && top != nil:
			mark(r, top)
		}
	}
}

// markCauses memoizes which rows are incomplete and finds the open texts that
// leave them so.
type markCauses struct {
	kids map[int][]*types.Requirement // the rows under each row number
	memo map[int]bool
}

func newMarkCauses(c *types.Completion) *markCauses {
	m := &markCauses{kids: map[int][]*types.Requirement{}, memo: map[int]bool{}}
	for _, r := range c.Requirements {
		if r != nil {
			m.kids[r.Parent] = append(m.kids[r.Parent], r)
		}
	}
	return m
}

// incomplete reports whether r's body has open text or one of its child
// allocations is incomplete.
func (m *markCauses) incomplete(r *types.Requirement) bool {
	if v, ok := m.memo[r.Number]; ok {
		return v
	}
	v := r.Req != nil && openText(r.Req)
	for _, k := range m.kids[r.Number] {
		if k.Kind == types.RequirementAllocation && m.incomplete(k) {
			v = true
		}
	}
	m.memo[r.Number] = v
	return v
}

// causes returns the open texts that leave r incomplete: its body's and
// its incomplete child allocations'.
func (m *markCauses) causes(r *types.Requirement) []*types.ReqNode {
	var out []*types.ReqNode
	openTexts(r.Req, &out)
	for _, k := range m.kids[r.Number] {
		if k.Kind == types.RequirementAllocation && m.incomplete(k) {
			out = append(out, m.causes(k)...)
		}
	}
	return out
}

// markerKind returns the kind the open texts in why share when each is Review
// by design, else other: mixed causes and a shortfall are other.
func markerKind(why []*types.ReqNode) types.TextKind {
	kind := types.TextOther
	for _, n := range why {
		if n.Type != types.NodeText {
			continue
		}
		if !reviewByDesign(n.Kind) || kind != types.TextOther && n.Kind != kind {
			return types.TextOther
		}
		kind = n.Kind
	}
	return kind
}

// reviewByDesign reports whether Review is the answer for a text of kind k,
// not a gap in the parse: someone decides, the calendar reads two ways, or
// the transcript can't show it.
func reviewByDesign(k types.TextKind) bool {
	return k == types.TextPermission || k == types.TextAdmission || k == types.TextAmbiguity || k == types.TextContent ||
		k == types.TextPlacement
}

// isMarker reports whether n is r's Review marker: r's sentence as text.
func isMarker(n *types.ReqNode, r *types.Requirement) bool {
	return n.Type == types.NodeText && (n.Kind == types.TextOther || reviewByDesign(n.Kind)) && n.Text == rawHead(r)
}

// openText reports whether n has text that leaves its count open. The maybe
// half of a bounded alternative, or(strict, and(pool, text)), doesn't when
// the strict branch is closed: the row still claims what it counts.
func openText(n *types.ReqNode) bool {
	if n == nil {
		return false
	}
	switch n.Type {
	case types.NodeText:
		return true
	case types.NodeOr:
		strict := slices.ContainsFunc(n.Items, func(x *types.ReqNode) bool { return x.Type != types.NodeText && !openText(x) })
		for _, it := range n.Items {
			if strict && boundedMaybe(it) {
				continue
			}
			if openText(it) {
				return true
			}
		}
		return false
	}
	return slices.ContainsFunc(n.Items, openText)
}

// boundedMaybe reports whether n is and(claims..., text...): courses that
// count only with someone's say. A claim is a pool of courses or an amount of
// any credits.
func boundedMaybe(n *types.ReqNode) bool {
	if n.Type != types.NodeAnd {
		return false
	}
	text, claims := false, false
	for _, it := range n.Items {
		switch {
		case it.Type == types.NodeText:
			text = true
		case openText(it):
			return false
		case !isAnyCredits(it) && hasCourses(it), it.Type == types.NodeCredits && it.Amount > 0:
			claims = true
		}
	}
	return text && claims
}

// openTexts appends to out the texts of n that leave its count open.
func openTexts(n *types.ReqNode, out *[]*types.ReqNode) {
	if n == nil {
		return
	}
	switch n.Type {
	case types.NodeText:
		*out = append(*out, n)
		return
	case types.NodeOr:
		strict := slices.ContainsFunc(n.Items, func(x *types.ReqNode) bool { return x.Type != types.NodeText && !openText(x) })
		for _, it := range n.Items {
			if !(strict && boundedMaybe(it)) {
				openTexts(it, out)
			}
		}
		return
	}
	for _, it := range n.Items {
		openTexts(it, out)
	}
}

// reviewMarked reports whether n can't be NotMet for want of credits: it is
// text, or has a text alternative.
func reviewMarked(n *types.ReqNode) bool {
	return n == nil || n.Type == types.NodeText ||
		n.Type == types.NodeOr && slices.ContainsFunc(n.Items, func(x *types.ReqNode) bool { return x.Type == types.NodeText })
}

// rowCredits returns the fewest credits allocation row r needs: its body's,
// or its child allocations' for a group. ok is false when any part's credits
// aren't known.
func rowCredits(r *types.Requirement, kids map[int][]*types.Requirement) (float64, bool) {
	if r.Req != nil {
		return nodeCredits(r.Req)
	}
	sum, ok := 0.0, true
	for _, k := range kids[r.Number] {
		if k.Kind == types.RequirementAllocation {
			c, kok := rowCredits(k, kids)
			sum += c
			ok = ok && kok
		}
	}
	return sum, ok
}

// nodeCredits returns the fewest credits that meet n; ok is false when they
// aren't known. Unlike minWeight, a text alternative isn't free: it counts
// only when every alternative is text, and a permission never does.
func nodeCredits(n *types.ReqNode) (float64, bool) {
	switch n.Type {
	case types.NodeText:
		return 0, false
	case types.NodeCourse:
		return courseWeight(n.Code), true
	case types.NodeCredits:
		return n.Amount, n.Amount > 0
	case types.NodeAnd:
		sum, ok := 0.0, true
		for _, it := range n.Items {
			c, iok := nodeCredits(it)
			sum += c
			ok = ok && iok
		}
		return sum, ok
	case types.NodeOr, types.NodeChoose:
		var cs []float64
		ok := true
		for _, text := range []bool{false, true} {
			for _, it := range n.Items {
				if (it.Type == types.NodeText) != text || it.Type == types.NodeText && it.Kind == types.TextPermission {
					continue
				}
				c, iok := nodeCredits(it)
				cs = append(cs, c)
				ok = ok && iok
			}
			if cs != nil {
				break
			}
		}
		if cs == nil {
			return 0, false
		}
		k := 1
		if n.Type == types.NodeChoose {
			k = n.Count
		}
		return sumCheapest(cs, k), ok
	}
	return 0, false
}

// checkItemPools adds a Review text to an item whose body is an amount of any
// credits, since which courses count isn't stated. "The remaining 1.5 - 2.0
// credits can include any courses ..." states it.
func (a *compAssembler) checkItemPools(rows []*compRow) {
	for _, r := range rows {
		if r.r.Kind == types.RequirementAllocation && r.r.Req != nil && r.r.Req.Type == types.NodeCredits && isAnyCredits(r.r.Req) &&
			!complRemainingAnyRe.MatchString(complClean(rawHead(r.r))) {
			r.r.Req = andNode(r.r.Req, textNode(types.TextOther, rawHead(r.r)))
		}
		a.checkItemPools(r.children)
	}
}
