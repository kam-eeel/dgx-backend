package programs

import (
	"slices"

	"degree-explorer/parser/grammar"
	"degree-explorer/types"
)

// Requirement node builders and helpers shared by enrolment and completion.

// AndNode and OrNode combine nodes, dropping nils and flattening children of
// the same type.
func AndNode(items ...*types.ReqNode) *types.ReqNode { return combineNodes(types.NodeAnd, items) }

func OrNode(items ...*types.ReqNode) *types.ReqNode { return combineNodes(types.NodeOr, items) }

func combineNodes(t types.NodeType, items []*types.ReqNode) *types.ReqNode {
	var flat []*types.ReqNode
	for _, it := range items {
		switch {
		case it == nil:
		case it.Type == t && !it.Concurrent: // a concurrent subtree keeps its own scope
			flat = append(flat, it.Items...)
		default:
			flat = append(flat, it)
		}
	}
	flat = grammar.DedupeNodes(flat)
	switch len(flat) {
	case 0:
		return nil
	case 1:
		return flat[0]
	}
	return &types.ReqNode{Type: t, Items: flat}
}

// CourseNodes returns a course node for each code.
func CourseNodes(codes []string) []*types.ReqNode {
	var out []*types.ReqNode
	for _, c := range codes {
		out = append(out, &types.ReqNode{Type: types.NodeCourse, Code: c})
	}
	return out
}

// NodeCodes lists the course codes a node names (courses and pools).
func NodeCodes(n *types.ReqNode) []string {
	if n == nil {
		return nil
	}
	var out []string
	if n.Code != "" {
		out = append(out, n.Code)
	}
	out = append(out, n.Courses...)
	for _, it := range n.Items {
		out = append(out, NodeCodes(it)...)
	}
	return out
}

func IsSubset(sub, of []string) bool {
	return !slices.ContainsFunc(sub, func(s string) bool { return !slices.Contains(of, s) })
}

// AmbiguousText is the lenient branch alone: or(x, AmbiguousText(why)) is x,
// or Review without it.
func AmbiguousText(why string) *types.ReqNode {
	return &types.ReqNode{Type: types.NodeText, Kind: types.TextAmbiguity, Text: why}
}

// tidy flattens nested and/or nodes of the same type and drops duplicate
// children, which substitutions can leave behind.
func Tidy(n *types.ReqNode) *types.ReqNode {
	if n == nil {
		return nil
	}
	for i, it := range n.Items {
		n.Items[i] = Tidy(it)
	}
	if n.Type == types.NodeAnd || n.Type == types.NodeOr {
		return combineNodes(n.Type, n.Items)
	}
	return n
}
