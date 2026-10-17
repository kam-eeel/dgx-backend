package reqs

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Share routes (Task 10E): an approval route for the part of the total a
// program-level floor leaves over. "At least 5.0 credits must be NMC and/or
// NML courses. 1.0 credit may be taken from courses offered by other
// departments, pending approval by the Associate Chair Undergraduate of
// NMC" (6.0 credits): the floor's designators (and the courses the program
// lists) make the total strictly, and the other 1.0 credit is approved, so
// the total is
//
//	or(credits{6, designators: NMC, NML, courses: <listed others>},
//	   and(credits{6}, text{permission: the route}))
//
// The floor bounds the route: at most 1.0 credit is outside it. The route
// leaves the notes.

// complShareRouteRe: the route, with its amount (group 1).
var complShareRouteRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+may\s+be\s+taken\s+from\s+courses\s+offered\s+by\s+other\s+departments,?\s+pending\s+approval\b`)

// shareRoutes places the share routes read so far (finish).
func (a *compAssembler) shareRoutes() {
	if a.total == nil || a.total.r.Req == nil {
		return
	}
	t := a.total.r.Req
	if t.Type != types.NodeCredits || t.Designators != nil || t.Courses != nil || t.Lists != nil || t.Levels != nil {
		return
	}
	for i, s := range a.notes {
		m := complShareRouteRe.FindStringSubmatch(a.clean(s))
		if m == nil {
			continue
		}
		v, _ := strconv.ParseFloat(m[1], 64)
		floor := a.shareFloor(t.Amount - v)
		if floor == nil {
			continue
		}
		strict := &types.ReqNode{Type: types.NodeCredits, Amount: t.Amount, Designators: slices.Clone(floor.Designators), Courses: a.listedOutside(floor.Designators)}
		a.total.r.Req = orNode(strict, andNode(&types.ReqNode{Type: types.NodeCredits, Amount: t.Amount}, textNode(types.TextPermission, cleanNote(s))))
		addRaw(a.total, s)
		a.notes = slices.Delete(a.notes, i, i+1)
		return
	}
}

// shareFloor returns the program-level floor of designators, and nothing
// else, at amount: "At least 5.0 credits must be NMC and/or NML courses".
func (a *compAssembler) shareFloor(amount float64) *types.ReqNode {
	for _, r := range slices.Concat(a.floors, a.program) {
		n := r.r.Req
		if r.r.Kind == types.RequirementConstraint && n != nil && n.Type == types.NodeCredits && n.Designators != nil &&
			n.Courses == nil && n.Lists == nil && n.Levels == nil && n.Exclude == nil && math.Abs(n.Amount-amount) < 1e-9 {
			return n
		}
	}
	return nil
}

// listedOutside returns the courses the program lists (in its rows or in
// lines of codes) outside the designators: JSN274H1 in an NMC group.
func (a *compAssembler) listedOutside(desigs []string) []string {
	var out []string
	add := func(c string) {
		if !slices.Contains(desigs, c[:3]) {
			out = append(out, c)
		}
	}
	for _, r := range a.allRows() {
		walkReq(r.r.Req, func(n *types.ReqNode) {
			if n.Type == types.NodeCourse {
				add(n.Code)
			}
			for _, c := range n.Courses {
				add(c)
			}
		})
	}
	for _, l := range a.lines {
		if mostlyCodes(l) {
			for _, c := range codeRe.FindAllString(l, -1) {
				add(strings.TrimSpace(c))
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// complLanguageCountRe: an amount in a few languages of one designator, "4.0
// credits in one or two primary source languages (with NML designator) from
// among the following languages: Akkadian, ...". Groups: the amount, the
// designator.
var complLanguageCountRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+in\s+one\s+or\s+two\s+(?:[\w-]+\s+){0,3}languages\s+\(with\s+((?-i:[A-Z]{3}))\s+designator\)`)

// complStreamPoolRe: a pool of two named streams or approved courses, "1.0
// credit in Medieval or Modern stream courses or those from a list of
// pre-approved courses offered by other departments". Groups: the amount,
// the two streams, the approval.
var complStreamPoolRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+in\s+(\w+)\s+or\s+(\w+)\s+stream\s+courses\s+or\s+(those\s+from\s+a\s+list\s+of\s+pre-approved\s+courses\s+offered\s+by\s+other\s+departments)$`)

// languagePools reads the rows of text above (finish):
//   - the language count becomes and(credits{4, designators: NML},
//     text{content: the row}): the designator is checked, the languages
//     (one or two, from those named) are not in the catalogue;
//   - the stream pool becomes or(credits{1, courses: both streams' lists},
//     text{permission: the approved courses}), the streams read from their
//     headings ("_Medieval Stream:_" and the codes under it), whose notes
//     then leave the notes. A stream without a list leaves the row as it
//     is.
func (a *compAssembler) languagePools() {
	for _, r := range a.allRows() {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText {
			continue
		}
		t := a.clean(n.Text)
		if m := complLanguageCountRe.FindStringSubmatch(t); m != nil {
			v, _ := strconv.ParseFloat(m[1], 64)
			r.r.Req = andNode(&types.ReqNode{Type: types.NodeCredits, Amount: v, Designators: []string{m[2]}}, textNode(types.TextContent, n.Text))
			continue
		}
		m := complStreamPoolRe.FindStringSubmatch(t)
		if m == nil {
			continue
		}
		var codes, heads, used []string
		ok := true
		for _, name := range m[2:4] {
			head := strings.ToLower(name) + " stream"
			c, lines := a.streamList(head)
			if c == nil {
				ok = false
				break
			}
			codes = append(codes, c...)
			heads = append(heads, head)
			used = append(used, lines...)
		}
		if !ok {
			continue
		}
		v, _ := strconv.ParseFloat(m[1], 64)
		r.r.Req = orNode(&types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: dedupe(codes)}, textNode(types.TextPermission, m[4]))
		for _, l := range used {
			addRaw(r, l)
		}
		a.notes = slices.DeleteFunc(a.notes, func(s string) bool {
			return slices.ContainsFunc(heads, func(h string) bool { return strings.HasPrefix(strings.ToLower(s), h+":") })
		})
	}
}

// streamList returns the codes on the lines under the heading name
// ("_Medieval Stream:_", lowercased) and the lines read; nil when there is
// none.
func (a *compAssembler) streamList(name string) (codes, lines []string) {
	for i, l := range a.lines {
		if strings.ToLower(strings.Trim(stripEmphasis(l), " :*_")) != name {
			continue
		}
		lines = append(lines, l)
		for k := i + 1; k < len(a.lines) && mostlyCodes(a.lines[k]); k++ {
			codes = append(codes, codeRe.FindAllString(a.lines[k], -1)...)
			lines = append(lines, a.lines[k])
		}
		return codes, lines
	}
	return nil, nil
}
