package reqs

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Share routes: the part of the total that a program-level floor leaves over,
// taken with approval.

// complShareRouteRe matches a share route: "1.0 credit may be taken from
// courses offered by other departments, pending approval".
var complShareRouteRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+may\s+be\s+taken\s+from\s+courses\s+offered\s+by\s+other\s+departments,?\s+pending\s+approval\b`)

// shareRoutes makes a plain credits total either the floor's designators plus
// the courses listed outside them, or the total with the route's approval.
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

// shareFloor returns the program-level floor of designators alone at amount:
// "At least 5.0 credits must be NMC and/or NML courses".
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

// listedOutside returns the courses the program lists, in rows or lines of
// codes, outside desigs.
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

// complLanguageCountRe matches an amount in a few languages of one designator:
// "4.0 credits in one or two primary source languages (with NML designator)".
var complLanguageCountRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+in\s+one\s+or\s+two\s+(?:[\w-]+\s+){0,3}languages\s+\(with\s+(` + desigPat + `)\s+designator\)`)

// complStreamPoolRe matches a pool of two streams or approved courses: "1.0
// credit in Medieval or Modern stream courses or those from a list of
// pre-approved courses offered by other departments".
var complStreamPoolRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+in\s+(\w+)\s+or\s+(\w+)\s+stream\s+courses\s+or\s+(those\s+from\s+a\s+list\s+of\s+pre-approved\s+courses\s+offered\s+by\s+other\s+departments)$`)

// languagePools encodes a language count as its designator plus the row's text,
// since the languages aren't in the catalogue, and a stream pool as the
// streams' lists or the approval. A stream without a list leaves the row as it
// is.
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

// streamList returns the codes under the heading name ("_Medieval Stream:_",
// lowercased) and the lines read; nil when there is none.
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
