package parser

import (
	"slices"
	"strings"
	"testing"

	"degree-explorer/parser/programs/completion"
	"degree-explorer/types"
)

// TestMarkerCauses checks that a Review marker's causes are found on the final
// envelope: a permission route and an ambiguity make the total's marker other,
// and both are its causes.
func TestMarkerCauses(t *testing.T) {
	c := completion.Parse("(1.0 credit)\n4. CHM395Y1/ CHM499Y1/ PHC489Y1 and/or CHM397H0/ CHM398H0 in areas relevant to the program with prior approval by the Department")
	n := c.Requirements[0].Req
	if n.Type != types.NodeOr || len(n.Items) != 2 || n.Items[1].Kind != types.TextOther || !completion.IsMarker(n.Items[1], c.Requirements[0]) {
		t.Fatalf("row 1 = %s, want or(credits, marker of kind other)", strings.Join(complCompact(c), "\n"))
	}
	var kinds []types.TextKind
	for _, x := range markerCauses(c)[1] {
		kinds = append(kinds, x.Kind)
	}
	if !slices.Equal(kinds, []types.TextKind{types.TextPermission, types.TextAmbiguity}) {
		t.Errorf("row 1 marker causes = %v, want [permission ambiguity]", kinds)
	}
}

// markerCauses returns, per row of c with a Review marker beside its count, the
// open texts that leave the count incomplete, as markIncomplete found them (none
// for a shortfall). A constraint the marker replaced is left out. c is unchanged.
func markerCauses(c *types.Completion) map[int][]*types.ReqNode {
	stripped := &types.Completion{}
	marked := map[int]bool{}
	for _, r := range c.Requirements {
		if r == nil {
			stripped.Requirements = append(stripped.Requirements, nil)
			continue
		}
		s := *r
		if n := r.Req; n != nil && n.Type == types.NodeOr && len(n.Items) == 2 && completion.IsMarker(n.Items[1], r) {
			s.Req = n.Items[0]
			marked[r.Number] = true
		}
		stripped.Requirements = append(stripped.Requirements, &s)
	}
	m := completion.NewMarkCauses(stripped)
	var top []*types.ReqNode
	for _, r := range m.Kids[0] {
		if r.Kind == types.RequirementAllocation && m.Incomplete(r) {
			top = append(top, m.Causes(r)...)
		}
	}
	out := map[int][]*types.ReqNode{}
	for _, r := range stripped.Requirements {
		switch {
		case r == nil || !marked[r.Number]:
		case r.Kind != types.RequirementConstraint:
			out[r.Number] = m.Causes(r)
		case r.Parent != 0:
			if p := r.Parent - 1; p < len(stripped.Requirements) && stripped.Requirements[p] != nil {
				out[r.Number] = m.Causes(stripped.Requirements[p])
			}
		default:
			out[r.Number] = top
		}
	}
	return out
}
