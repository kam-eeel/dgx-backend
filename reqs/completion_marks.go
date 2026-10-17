package reqs

import (
	"slices"

	"degree-explorer/types"
)

// Review markers: once the rows are numbered, the counts their rows can't be
// seen to make up (a child that is text or has a route, a total its items
// fall short of) get a text alternative, so that they are Review rather
// than NotMet (markIncomplete; Link runs it again when it turns a pool into
// text). Before numbering, checkItemPools marks an item that is a bare
// amount of any credits.

// markIncomplete puts the Review marker on the counts the rows of c can't be
// seen to make up (design 5.4: a row counts what its child allocations
// claim, and text claims nothing); total says row 1 is the header total:
//   - the total gets or(credits, the header as text) when any child
//     allocation is incomplete (its body has open text: a text item, a
//     route; openText), or, with shortfall and no pool of its own, when no
//     item claims courses or the items' fewest credits fall short of it
//     ("(6.5 credits)" over items of 5.5). Met when the children's claims
//     reach it, Review otherwise; "and" would be NotMet whenever they fall
//     short. A total with a pool of its own is marked too when a child is
//     incomplete: what the child claims is unknown;
//   - a constraint under an incomplete count (the header's floors under a
//     marked total, a floor under an item with text) gets or(constraint,
//     its text): it can't be NotMet on a count that is itself Review;
//   - with no total, a top-level constraint is on what all top-level
//     allocations claim (design 5.6): it is marked when one of them is
//     incomplete.
//
// A marker takes its cause's kind (markerKind): the kind of the open text
// that leaves the count incomplete, when every such text is Review by design
// (a permission, admission, ambiguity or content text); else, and for a
// shortfall, it is other. Evaluation is the same whatever the kind.
//
// Link runs it again: without shortfall when its designator check turns a
// pool into text, and with shortfall, after taking the markers off (remark),
// when its other steps change a body.
func markIncomplete(c *types.Completion, total, shortfall bool) {
	m := newMarkCauses(c)
	kids, incomplete, causes, memo := m.kids, m.incomplete, m.causes, m.memo
	mark := func(r *types.Requirement, why []*types.ReqNode) {
		head := textNode(types.TextOther, complItemLabelRe.ReplaceAllString(rawHead(r), "")) // the sentence, not its item number
		if r.Kind == types.RequirementConstraint && r.Req != nil && r.Req.Type == types.NodeAnd &&
			slices.ContainsFunc(r.Req.Items, func(x *types.ReqNode) bool { return x.Type == types.NodeText }) {
			// and(floor, text) ("2.0 PRT credits at the 200+ level taught in
			// Portuguese") is Review at best; marked, it is never NotMet
			// either: or(and(floor, text), text) is Review whatever the
			// courses, which its text says. Its own text is the cause, so
			// the marker takes that text's kind whatever the parent's
			// causes are.
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
		// Options count through their choice: the cheapest choice.
		for _, k := range kids[0] {
			if k.Kind == types.RequirementChoice {
				claims = true
				sum += choiceCredits(k.Req, c, kids)
			}
		}
		if !reviewMarked(n) && (incomplete(t) || shortfall && !pool && (!claims || sum < n.Amount-1e-9)) {
			var why []*types.ReqNode // nil for a shortfall: other
			if incomplete(t) {
				why = causes(t)
			}
			mark(t, why)
			// Its constraints are marked too, for the same causes (its
			// marker is one, of the kind they gave it).
			memo[1] = true
		}
	}
	var top []*types.ReqNode // the open texts of the incomplete top-level allocations
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

// markCauses finds the rows whose counts are incomplete and the open texts
// that leave them so (markIncomplete).
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

// incomplete reports whether r's body has open text (openText) or one of
// its child allocations is incomplete.
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

// markerCauses returns, for each row of c with a Review marker beside its
// count (or(count, marker), markIncomplete's mark), the open texts that
// leave that count incomplete, as markIncomplete found them: the row's own
// (a total's), its parent's (a constraint's), or the incomplete top-level
// allocations' (a top-level constraint's). A marker with none is a
// shortfall's. c is not changed. A constraint the marker replaced (its own
// and(floor, text)) is left out: its texts are gone, and its marker's kind
// is theirs.
func markerCauses(c *types.Completion) map[int][]*types.ReqNode {
	stripped := &types.Completion{}
	marked := map[int]bool{}
	for _, r := range c.Requirements {
		if r == nil {
			stripped.Requirements = append(stripped.Requirements, nil)
			continue
		}
		s := *r
		if n := r.Req; n != nil && n.Type == types.NodeOr && len(n.Items) == 2 && isMarker(n.Items[1], r) {
			s.Req = n.Items[0]
			marked[r.Number] = true
		}
		stripped.Requirements = append(stripped.Requirements, &s)
	}
	m := newMarkCauses(stripped)
	var top []*types.ReqNode
	for _, r := range m.kids[0] {
		if r.Kind == types.RequirementAllocation && m.incomplete(r) {
			top = append(top, m.causes(r)...)
		}
	}
	out := map[int][]*types.ReqNode{}
	for _, r := range stripped.Requirements {
		switch {
		case r == nil || !marked[r.Number]:
		case r.Kind != types.RequirementConstraint:
			out[r.Number] = m.causes(r)
		case r.Parent != 0:
			if p := r.Parent - 1; p < len(stripped.Requirements) && stripped.Requirements[p] != nil {
				out[r.Number] = m.causes(stripped.Requirements[p])
			}
		default:
			out[r.Number] = top
		}
	}
	return out
}

// markerKind is the kind of a Review marker whose count the open texts why
// leave incomplete: theirs when every one is Review by design and of one
// kind, else other ("If causes are mixed, keep other": a permission route
// and a content pool are two causes; a shortfall has no text and is other).
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

// reviewByDesign reports a text kind whose Review is the answer, not a gap
// in the parse: someone decides (permission, admission), the calendar reads
// two ways (ambiguity), the course's content does (content), or a placement
// test or exemption the transcript doesn't show does (placement).
func reviewByDesign(k types.TextKind) bool {
	return k == types.TextPermission || k == types.TextAdmission || k == types.TextAmbiguity || k == types.TextContent ||
		k == types.TextPlacement
}

// isMarker reports whether n is r's Review marker (markIncomplete's mark):
// r's sentence as text, of kind other or a kind it took from its cause.
func isMarker(n *types.ReqNode, r *types.Requirement) bool {
	return n.Type == types.NodeText && (n.Kind == types.TextOther || reviewByDesign(n.Kind)) && n.Text == rawHead(r)
}

// openText reports whether n has text that leaves its count open: any text
// but the "maybe" half of a bounded alternative, or(strict, and(wider pool,
// text)) (design 3.3), next to a strict branch without open text. A student
// who meets the strict branch claims its courses, and one who meets only the
// wider pool is Review on the row itself: either way the row claims what it
// counts.
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

// boundedMaybe reports and(claims..., text...): courses that count only with
// someone's say (the text), the second half of a bounded alternative. A
// claim is a pool of courses, or an amount of any credits: "A maximum of
// 1.0 credit in cognate courses is allowed ... get approved in advance" as
// and(credits{4}, text{permission}) beside credits{4, GER} leaves the count
// closed (4.0 credits, Review on the row), as a pool would.
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

// openTexts appends to out the texts of n that leave its count open
// (openText).
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

// rowCredits returns the fewest credits an allocation row needs: its body's
// (nodeCredits), or its child allocations' for a group (kids: the rows under
// each row number). ok is false when part of it is text or a node whose
// credits aren't known.
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

// nodeCredits returns the fewest credits that meet n: a course's weight, a
// pool's amount, the sum of an "and", the cheapest alternative of an "or"
// that isn't text, the cheapest count of a "choose". Text counts nothing
// (ok false).
//
// It differs from minWeight (enrolment), which counts text as 0 credits and
// so takes an "or" with a text alternative as costing nothing: here a text
// alternative is a route, skipped unless every alternative is text, and a
// permission never counts; a pool of no amount, an average and any other
// node are unknown (ok false). minCredits (the grammar) reads courses only.
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
		// A text alternative is a route (Review), not the cheapest way: it
		// counts only when every alternative is text, and a permission
		// never does.
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

// checkItemPools marks an item whose body is an amount of any credits
// ("2.5 credits at least 1.5 credits from PCL courses: ..." leaves "2.5
// credits"): which courses count isn't stated, so it is Review, not a claim
// on any course. "The remaining 1.5 - 2.0 credits can include any courses
// ..." (complRemainingAnyRe) states it: any course.
func (a *compAssembler) checkItemPools(rows []*compRow) {
	for _, r := range rows {
		if r.r.Kind == types.RequirementAllocation && r.r.Req != nil && r.r.Req.Type == types.NodeCredits && isAnyCredits(r.r.Req) &&
			!complRemainingAnyRe.MatchString(complClean(rawHead(r.r))) {
			r.r.Req = andNode(r.r.Req, textNode(types.TextOther, rawHead(r.r)))
		}
		a.checkItemPools(r.children)
	}
}
