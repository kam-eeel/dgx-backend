package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Linking program completion: the steps that read another program's rows,
// inheritance and a pool widened by another program's.

var (
	// complInheritPartsRe matches an inheritance sentence: "begins with the
	// requirements of First Year and of lines 1-3 of Higher Years of the Cell and
	// Molecular Biology Major Program".
	complInheritPartsRe = regexp.MustCompile(`(?i)\bbegins\s+with\s+the\s+requirements\s+of\s+(.+)\s+of\s+the\s+(.+?)\.?$`)
	// complInheritPartSepRe splits what it begins with into parts: "First Year",
	// "lines 1-3 of Higher Years".
	complInheritPartSepRe = regexp.MustCompile(`(?i)\s*(?:,\s*(?:and\s+)?(?:of\s+)?|\s+and\s+(?:of\s+)?)`)
	complInheritLinesRe   = regexp.MustCompile(`(?i)^lines?\s+(\d+)\s*(?:-|–|to)\s*(\d+)\s+of\s+(.+)$`)
	// complOrUnderRe matches a pool widened by another program's: "any 3rd or 4th
	// year elective listed above or under the Forest Conservation Specialist".
	complOrUnderRe = regexp.MustCompile(`(?i)^(.+?)\s+or\s+under\s+the\s+(.+?)\.?$`)
	// complListedInRe matches a pool another program lists: "2.0 credits in
	// eligible elective courses listed in the European Affairs Major".
	complListedInRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+in\s+eligible\s+elective\s+courses\s+listed\s+in\s+the\s+(.+?)\.?$`)
)

// inheritSource is one part of an inheritance sentence: the rows of a section,
// or of its lines from..to.
type inheritSource struct {
	section  string
	from, to int // 0: the whole section
}

// linkInheritance replaces an inheritance sentence's row with deep copies of
// the rows it names, under this program's total if any, and renumbers. It
// reports whether c changed; a sentence it can't resolve stays text.
func linkInheritance(c *types.Completion, self string, byCode map[string]*types.Program, titles titleIndex) bool {
	at := -1
	var m []string
	for i, r := range c.Requirements {
		if m = complInheritPartsRe.FindStringSubmatch(rawHead(r)); m != nil && r.Req != nil && r.Req.Type == types.NodeText {
			at = i
			break
		}
	}
	if at < 0 {
		return false
	}
	codes, ok := titles.lookupTitle(titleSuffixRe.ReplaceAllString(m[2], ""))
	if !ok || len(codes) != 1 || codes[0] == self || byCode[codes[0]] == nil || byCode[codes[0]].Completion == nil {
		return false
	}
	src := byCode[codes[0]].Completion
	var parts []inheritSource
	for _, part := range complInheritPartSepRe.Split(strings.TrimSpace(m[1]), -1) {
		if lm := complInheritLinesRe.FindStringSubmatch(part); lm != nil {
			from, _ := strconv.Atoi(lm[1])
			to, _ := strconv.Atoi(lm[2])
			parts = append(parts, inheritSource{section: lm[3], from: from, to: to})
		} else {
			parts = append(parts, inheritSource{section: part})
		}
	}
	picked := inheritRows(src, parts)
	if picked == nil {
		return false
	}
	sentence := c.Requirements[at]
	parent := 0
	if completionTotal(c) {
		parent = 1
	}
	under := map[int]bool{}
	for _, r := range picked {
		under[r.Number] = true
	}
	var copies []*types.Requirement
	old := map[*types.Requirement]int{} // copy -> its number in the source
	for _, r := range src.Requirements {
		if !under[r.Number] && !under[r.Parent] {
			continue
		}
		under[r.Number] = true
		k := &types.Requirement{Number: r.Number, Kind: r.Kind, Label: r.Label, Section: r.Section, Parent: r.Parent,
			Overlaps: slices.Clone(r.Overlaps), Req: cloneNode(r.Req), Audience: cloneNode(r.Audience), Scope: r.Scope, Notes: slices.Clone(r.Notes), Raw: r.Raw}
		if slices.Contains(picked, r) {
			k.Parent = -1 // set below
			k.Raw = r.Raw + "\n" + strings.TrimSpace(sentence.Raw)
		}
		old[k] = r.Number
		copies = append(copies, k)
	}
	lists, ok := inheritLists(c, src, copies)
	if !ok {
		return false
	}
	c.Lists = append(c.Lists, lists...)
	copies[0].Notes = slices.Concat(copies[0].Notes, sentence.Notes)
	// references among the copies are to the source's rows; among this program's,
	// to its own
	srcRow := map[int]*types.Requirement{}
	for _, k := range copies {
		srcRow[old[k]] = k
	}
	ownRow := map[int]*types.Requirement{}
	for _, r := range c.Requirements {
		ownRow[r.Number] = r
	}
	var total *types.Requirement
	if parent == 1 {
		total = c.Requirements[0]
	}
	rows := slices.Concat(slices.Clone(c.Requirements[:at]), copies, slices.Clone(c.Requirements[at+1:]))
	renumber(c, rows, func(r *types.Requirement, n int) *types.Requirement {
		if r == nil {
			// a list item's reference is to this program's rows
			if n == sentence.Number {
				return nil
			}
			return ownRow[n]
		}
		if _, copied := old[r]; copied {
			return srcRow[n]
		}
		if n == sentence.Number {
			return ownRow[sentence.Parent]
		}
		return ownRow[n]
	}, func(r *types.Requirement) *types.Requirement {
		if r.Parent == -1 {
			return total
		}
		return nil
	})
	return true
}

// inheritLists returns copies of the source's lists the copied rows name, and
// the lists those name, that c lacks. A name c gives another list, or a list
// referring to source rows, is a conflict (not ok): the sentence stays text.
func inheritLists(c *types.Completion, src *types.Completion, copies []*types.Requirement) ([]*types.CourseList, bool) {
	own := map[string]*types.CourseList{}
	for _, l := range c.Lists {
		own[l.Name] = l
	}
	theirs := map[string]*types.CourseList{}
	for _, l := range src.Lists {
		theirs[l.Name] = l
	}
	var names []string
	for _, k := range copies {
		walkReq(k.Req, func(n *types.ReqNode) { names = append(names, n.Lists...) })
	}
	var add []*types.CourseList
	seen := map[string]bool{}
	for len(names) > 0 {
		name := names[0]
		names = names[1:]
		if seen[name] {
			continue
		}
		seen[name] = true
		l := theirs[name]
		if l == nil {
			continue // checkRequirementRefs makes the row text
		}
		if o := own[name]; o != nil {
			if !slices.EqualFunc(o.Items, l.Items, func(x, y *types.ReqNode) bool { return complNodeKey(x) == complNodeKey(y) }) {
				return nil, false
			}
			continue
		}
		k := &types.CourseList{Name: l.Name, Title: l.Title}
		refers := false
		for _, it := range l.Items {
			walkReq(it, func(n *types.ReqNode) { refers = refers || n.Requirements != nil })
		}
		if refers {
			// its requirements are source rows the copies can't map
			return nil, false
		}
		for _, it := range l.Items {
			k.Items = append(k.Items, cloneNode(it))
			walkReq(it, func(n *types.ReqNode) { names = append(names, n.Lists...) })
		}
		add = append(add, k)
	}
	return add, true
}

// inheritRows picks the source's rows for the parts: a section's allocations
// under the total or the top, or those labelled "<section> <n>" for lines
// from..to. Every part must pick something.
func inheritRows(src *types.Completion, parts []inheritSource) []*types.Requirement {
	total := completionTotal(src)
	var picked []*types.Requirement
	for _, part := range parts {
		var got []*types.Requirement
		for _, r := range src.Requirements {
			if r.Kind != types.RequirementAllocation || !strings.EqualFold(r.Section, strings.TrimSpace(part.section)) ||
				!(r.Parent == 0 || total && r.Parent == 1) {
				continue
			}
			if part.from > 0 {
				n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(strings.ToLower(r.Label), strings.ToLower(r.Section))))
				if err != nil || n < part.from || n > part.to {
					continue
				}
			}
			got = append(got, r)
		}
		if part.from > 0 && len(got) != part.to-part.from+1 || got == nil {
			return nil
		}
		picked = append(picked, got...)
	}
	return picked
}

// complNoRow stands for a row a reference named that the program doesn't
// have. checkRequirementRefs makes it text: dropping it would widen a pool.
const complNoRow = -1

// renumber puts rows in c, numbered 1..N, and rewrites every reference through
// resolve: a name resolving to nothing becomes complNoRow, or the top for a
// parent. override, when it gives a row, is r's parent.
func renumber(c *types.Completion, rows []*types.Requirement, resolve func(r *types.Requirement, n int) *types.Requirement, override func(r *types.Requirement) *types.Requirement) {
	type refs struct {
		parent   *types.Requirement
		overlaps []*types.Requirement
		nodes    map[*types.ReqNode][]*types.Requirement
	}
	all := map[*types.Requirement]*refs{}
	for _, r := range rows {
		x := &refs{nodes: map[*types.ReqNode][]*types.Requirement{}}
		if x.parent = override(r); x.parent == nil && r.Parent > 0 {
			x.parent = resolve(r, r.Parent)
		}
		for _, o := range r.Overlaps {
			x.overlaps = append(x.overlaps, resolve(r, o))
		}
		walkReq(r.Req, func(n *types.ReqNode) {
			for _, k := range n.Requirements {
				x.nodes[n] = append(x.nodes[n], resolve(r, k))
			}
		})
		all[r] = x
	}
	listRefs := map[*types.ReqNode][]*types.Requirement{}
	for _, l := range c.Lists {
		for _, it := range l.Items {
			walkReq(it, func(n *types.ReqNode) {
				for _, k := range n.Requirements {
					listRefs[n] = append(listRefs[n], resolve(nil, k))
				}
			})
		}
	}
	for i, r := range rows {
		r.Number = i + 1
	}
	num := func(q *types.Requirement) int {
		if q == nil {
			return complNoRow
		}
		return q.Number
	}
	for n, qs := range listRefs {
		n.Requirements = nil
		for _, q := range qs {
			n.Requirements = append(n.Requirements, num(q))
		}
	}
	for _, r := range rows {
		x := all[r]
		r.Parent = 0
		if x.parent != nil {
			r.Parent = x.parent.Number
		}
		r.Overlaps = nil
		for _, q := range x.overlaps {
			r.Overlaps = append(r.Overlaps, num(q))
		}
		walkReq(r.Req, func(n *types.ReqNode) {
			if n.Requirements == nil {
				return
			}
			n.Requirements = nil
			for _, q := range x.nodes[n] {
				n.Requirements = append(n.Requirements, num(q))
			}
		})
	}
	c.Requirements = rows
}

// linkOtherProgramPool widens a pool by another program's ("listed above or
// under the X"): the row becomes one pool of both programs' courses. Anything
// else stays as it is. It reports whether c changed.
func linkOtherProgramPool(c *types.Completion, self string, byCode map[string]*types.Program, titles titleIndex) bool {
	changed := false
	for _, r := range c.Requirements {
		n := r.Req
		if n == nil || n.Type != types.NodeOr || len(n.Items) != 2 || n.Items[1].Type != types.NodeText {
			continue
		}
		own := n.Items[0]
		m := complOrUnderRe.FindStringSubmatch(n.Items[1].Text)
		if m == nil || !plainPool(own) {
			continue
		}
		codes, ok := titles.lookupTitle(titleSuffixRe.ReplaceAllString(m[2], ""))
		if !ok || len(codes) != 1 || codes[0] == self || byCode[codes[0]] == nil || byCode[codes[0]].Completion == nil {
			continue
		}
		for _, q := range byCode[codes[0]].Completion.Requirements {
			head := strings.TrimSuffix(strings.TrimSpace(rawHead(q)), ".")
			if !strings.EqualFold(head, strings.TrimSpace(m[1])) || q.Req == nil || !plainPool(q.Req) || q.Req.Amount != own.Amount {
				continue
			}
			pool := cloneNode(own)
			pool.Courses = dedupe(append(pool.Courses, q.Req.Courses...))
			r.Req = pool
			changed = true
			break
		}
	}
	return changed
}

// linkProgramList reads a row of text drawing on another program's list: c
// gets a copy of the list, and the row takes that program's approval route
// with it. Anything else stays text. It reports whether c changed.
func linkProgramList(c *types.Completion, self string, byCode map[string]*types.Program, titles titleIndex) bool {
	changed := false
	for _, r := range c.Requirements {
		if r.Kind != types.RequirementAllocation || r.Req == nil || r.Req.Type != types.NodeText {
			continue
		}
		m := complListedInRe.FindStringSubmatch(strings.TrimSpace(r.Req.Text))
		if m == nil {
			continue
		}
		codes, ok := titles.lookupTitle(titleSuffixRe.ReplaceAllString(m[2], ""))
		if !ok || len(codes) != 1 || codes[0] == self || byCode[codes[0]] == nil || byCode[codes[0]].Completion == nil {
			continue
		}
		src := byCode[codes[0]].Completion
		var from *types.ReqNode
		n := 0
		for _, q := range src.Requirements {
			b := q.Req
			if q.Kind == types.RequirementAllocation && b != nil && b.Type == types.NodeOr && len(b.Items) == 2 &&
				b.Items[0].Type == types.NodeCredits && len(b.Items[0].Lists) == 1 && b.Items[1].Type == types.NodeText && b.Items[1].Kind == types.TextPermission {
				from, n = b, n+1
			}
		}
		if n != 1 {
			continue
		}
		amount, _ := strconv.ParseFloat(m[1], 64)
		req := orNode(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Lists: slices.Clone(from.Items[0].Lists)}, cloneNode(from.Items[1]))
		lists, ok := inheritLists(c, src, []*types.Requirement{{Req: req}})
		if !ok {
			continue
		}
		c.Lists = append(c.Lists, lists...)
		r.Req = req
		changed = true
	}
	return changed
}

// plainPool reports whether n is an amount from listed courses only.
func plainPool(n *types.ReqNode) bool {
	return n.Type == types.NodeCredits && n.Amount > 0 && n.Courses != nil && n.Designators == nil && n.Levels == nil &&
		n.Breadth == nil && n.Lists == nil && n.Requirements == nil && n.Exclude == nil && n.MaxAmount == nil && n.Items == nil
}
