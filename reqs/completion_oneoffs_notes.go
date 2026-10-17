package reqs

import (
	"regexp"
	"slices"
	"strings"

	"degree-explorer/types"
)

// One-off readings of notes: notes on the total, restated constraints, notes
// that summarise rows, and notes dropped or joined to a row.

// complCanBeUsedNoteRe matches "Note: CHM395Y1 can be used toward this
// requirement", the total's alternative for a course outside its levels.
var complCanBeUsedNoteRe = regexp.MustCompile(`(?i)^(?:note:\s*)?(` + codePat + `)\s+can\s+be\s+used\s+towards?\s+this\s+requirement\.?$`)

// totalCourseNote reads the total's or(pool, text{complCanBeUsedNoteRe}) as
// one pool from a list "Requirement 1 courses" of the pool's filter and the
// course. It returns the lists with it.
func (a *compAssembler) totalCourseNote(lists []*types.CourseList) []*types.CourseList {
	if a.total == nil {
		return lists
	}
	n := a.total.r.Req
	if n == nil || n.Type != types.NodeOr || len(n.Items) != 2 || n.Items[1].Type != types.NodeText {
		return lists
	}
	m := complCanBeUsedNoteRe.FindStringSubmatch(strings.TrimSpace(n.Items[1].Text))
	pool := n.Items[0]
	if m == nil || pool.Type != types.NodeCredits || pool.Lists != nil || pool.Designators == nil && pool.Levels == nil {
		return lists
	}
	const name = "Requirement 1 courses"
	if slices.ContainsFunc(lists, func(l *types.CourseList) bool { return l.Name == name }) {
		return lists
	}
	filter := &types.ReqNode{Type: types.NodeCredits, Designators: pool.Designators, Levels: pool.Levels, Courses: pool.Courses, Exclude: pool.Exclude, MaxCourseAmount: pool.MaxCourseAmount}
	lists = append(lists, &types.CourseList{Name: name, Items: []*types.ReqNode{filter, {Type: types.NodeCredits, Courses: []string{m[1]}}}})
	a.total.r.Req = &types.ReqNode{Type: types.NodeCredits, Amount: pool.Amount, Lists: []string{name}}
	return lists
}

// restatedConstraints drops a program constraint credits{w, courses} that an
// item already requires as or(those courses), each of weight w, joining its
// sentence to the item's raw.
func (a *compAssembler) restatedConstraints() {
	a.program = slices.DeleteFunc(a.program, func(r *compRow) bool {
		n := r.r.Req
		if r.r.Kind != types.RequirementConstraint || n == nil || len(r.children) > 0 {
			return false
		}
		if n.Type == types.NodeOr && len(n.Items) == 2 && n.Items[1].Type == types.NodeText {
			n = n.Items[0]
		}
		if !isPool(n) || n.Courses == nil || n.Designators != nil || n.Levels != nil || n.Lists != nil || n.Amount <= 0 {
			return false
		}
		want := slices.Sorted(slices.Values(n.Courses))
		requires := func(x *types.ReqNode) bool {
			if x.Type != types.NodeOr {
				return false
			}
			var codes []string
			for _, it := range x.Items {
				if it.Type != types.NodeCourse || courseWeight(it.Code) != n.Amount {
					return false
				}
				codes = append(codes, it.Code)
			}
			slices.Sort(codes)
			return slices.Equal(codes, want)
		}
		for _, q := range a.items {
			body := q.r.Req
			if q.r.Kind != types.RequirementAllocation || body == nil {
				continue
			}
			if requires(body) || body.Type == types.NodeAnd && slices.ContainsFunc(body.Items, requires) {
				addRaw(q, r.r.Raw)
				return true
			}
		}
		return false
	})
}

// complLanguageBanRe matches "Language courses cannot be used to fulfil the
// requirements of the Minor program".
var complLanguageBanRe = regexp.MustCompile(`(?i)^language\s+courses\s+cannot\s+be\s+used\s+to\s+(?:fulfil|fulfill|satisfy)\s+the\s+requirements\s+of\s+the\s+(?:minor|major|specialist)\s+program$`)

// languageBanNote joins a complLanguageBanRe row to the total's raw, or makes
// it a note, when namedPoolsFill: the courses counted are the ones the calendar
// lists (ASMIN1058, whose lists hold no language course).
func (a *compAssembler) languageBanNote() {
	if !a.namedPoolsFill() {
		return
	}
	a.program = slices.DeleteFunc(a.program, func(r *compRow) bool {
		if r.r.Req == nil || r.r.Req.Type != types.NodeText || len(r.children) > 0 || !complLanguageBanRe.MatchString(a.clean(r.r.Req.Text)) {
			return false
		}
		if a.total != nil {
			addRaw(a.total, r.r.Req.Text) // restated: the total's items name every course
		} else {
			a.notes = append(a.notes, cleanNote(r.r.Req.Text))
		}
		return true
	})
}

// namedPoolsFill reports whether every item is courses and pools of named
// courses or lists and the items fill the total: no course the program
// doesn't name counts.
func (a *compAssembler) namedPoolsFill() bool {
	named := a.total == nil || a.total.r.Req != nil && a.total.r.Req.Type == types.NodeCredits
	sum := 0.0
	for _, q := range a.items {
		if q.r.Kind != types.RequirementAllocation || q.r.Req == nil {
			continue
		}
		strict := q.r.Req // an ambiguity's strict reading
		if strict.Type == types.NodeOr && strict.Items[0].Type != types.NodeText {
			strict = strict.Items[0]
		}
		c, ok := nodeCredits(strict)
		sum += c
		walkReq(q.r.Req, func(x *types.ReqNode) {
			// an ambiguity's text sits beside its lenient pool, which is named
			named = named && ok && (x.Type != types.NodeText || x.Kind == types.TextAmbiguity) && (x.Type != types.NodeCredits || x.Designators == nil && x.Levels == nil && (x.Courses != nil || x.Lists != nil))
		})
	}
	return named && (a.total == nil || sum >= a.total.r.Req.Amount-1e-9)
}

// complThroughDanielsRe matches "taken through the Daniels Faculty".
var complThroughDanielsRe = regexp.MustCompile(`(?i)^taken\s+through\s+the\s+Daniels\s+Faculty$`)

// throughDaniels drops complThroughDanielsRe from a body beside a VIS pool:
// the Daniels Faculty offers VIS courses, so it restates the designator
// (ASMIN0660). The row's raw keeps it.
func throughDaniels(r *compRow) {
	n := r.r.Req
	if n == nil || n.Type != types.NodeAnd {
		return
	}
	vis := slices.ContainsFunc(n.Items, func(x *types.ReqNode) bool {
		return x.Type == types.NodeCredits && slices.Equal(x.Designators, []string{"VIS"})
	})
	for i, it := range n.Items {
		if vis && it.Type == types.NodeText && complThroughDanielsRe.MatchString(complClean(it.Text)) {
			r.r.Req = andNode(slices.Delete(slices.Clone(n.Items), i, i+1)...)
			return
		}
	}
}

// complPossibilitiesRe matches a note listing examples of an open pool:
// "Possibilities include but are not limited to: ( MUN100H1 and MUN110H1) or
// ... or ECO105Y1".
var complPossibilitiesRe = regexp.MustCompile(`(?i)^possibilities\s+include\s+but\s+are\s+not\s+limited\s+to:\s*(.+)$`)

// possibilities reads a row of text with a complPossibilitiesRe note as
// or(the examples, text{content}, routes): the examples don't exhaust it.
func possibilities(r *compRow) {
	n := r.r.Req
	if r.r.Kind != types.RequirementAllocation || n == nil {
		return
	}
	items := []*types.ReqNode{n}
	if n.Type == types.NodeOr {
		items = n.Items
	}
	if items[0].Type != types.NodeText || items[0].Kind != types.TextOther || slices.ContainsFunc(items, func(x *types.ReqNode) bool { return x.Type != types.NodeText }) {
		return
	}
	for i, note := range r.r.Notes {
		m := complPossibilitiesRe.FindStringSubmatch(note)
		if m == nil {
			continue
		}
		ex, _ := parseCompletionText(strings.TrimSuffix(m[1], "."))
		if ex == nil || ex.Type != types.NodeOr || hasText(ex) {
			return
		}
		items[0].Kind = types.TextContent
		r.r.Req = orNode(append(slices.Clone(ex.Items), items...)...)
		if r.r.Notes = slices.Delete(r.r.Notes, i, i+1); len(r.r.Notes) == 0 {
			r.r.Notes = nil
		}
		return
	}
}

var (
	// complDistributionRe matches a lead summing up the items below it: "The
	// distribution of the 7.0 credits is as follows:".
	complDistributionRe = regexp.MustCompile(`(?i)^the\s+distribution\s+of\s+the\s+` + numPat + `\s+credits\s+is\s+as\s+follows:?$`)
	// complSingleLanguageRe matches the rule such a summary adds: "A two-year
	// progressive sequence of courses in a single language".
	complSingleLanguageRe = regexp.MustCompile(`(?i)\bsequence\s+of\s+courses\s+in\s+a\s+single\s+language\b`)
)

// distributionSummary makes a complDistributionRe item and its bullets notes,
// since the items below say each again (ASMAJ1626). The one-language rule
// they add stays a program text constraint.
func (a *compAssembler) distributionSummary() {
	a.items = slices.DeleteFunc(a.items, func(r *compRow) bool {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || !complDistributionRe.MatchString(a.clean(n.Text)) ||
			slices.ContainsFunc(r.children, func(c *compRow) bool { return c.r.Req == nil || c.r.Req.Type != types.NodeText || len(c.children) > 0 }) {
			return false
		}
		sents := slices.Clone(r.r.Notes)
		for _, c := range r.children {
			sents = append(sents, c.r.Req.Text)
		}
		for _, s := range sents {
			if complSingleLanguageRe.MatchString(s) {
				a.program = append(a.program, &compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Req: textNode(types.TextOther, s), Raw: s}})
			} else {
				a.notes = append(a.notes, cleanNote(s))
			}
		}
		return true
	})
}

// noteFloorAfterLine reads a floor noted after the last item of a program
// with no total by the ambiguity policy: on that item is strict (Met), on the
// whole program lenient (Review).
func (a *compAssembler) noteFloorAfterLine() {
	if a.total != nil || len(a.program) != 1 {
		return
	}
	last := a.program[0]
	var above *compRow
	for _, r := range a.items {
		if r.r.Kind == types.RequirementAllocation {
			above = r
		}
	}
	n := last.r.Req
	if above == nil || last.r.Kind != types.RequirementConstraint || last.r.Parent != 0 || len(last.children) > 0 || n == nil ||
		n.Type != types.NodeCredits || n.Amount == 0 || n.Levels == nil || n.Designators != nil || n.Courses != nil || n.Lists != nil ||
		n.Requirements != nil || !strings.Contains(a.raw, above.r.Raw+"\nNote: "+last.r.Raw) {
		return
	}
	ref := cloneNode(n)
	ref.Requirements = []int{}
	a.reqRefs = append(a.reqRefs, compReqRef{node: ref, rows: []*compRow{above}, scope: true})
	last.r.Req = orNode(ref, andNode(n, textNode(types.TextAmbiguity, last.r.Raw)))
}
