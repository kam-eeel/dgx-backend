package grammar

import (
	"degree-explorer/types"
)

// Requirement tree helpers: walking and replacing nodes, and a course's weight.

func WalkReq(n *types.ReqNode, f func(*types.ReqNode)) {
	if n == nil {
		return
	}
	f(n)
	for _, it := range n.Items {
		WalkReq(it, f)
	}
}

// CourseWeight is a course's credit weight: H = 0.5, Y = 1.0.
func CourseWeight(code string) float64 {
	if len(code) > 6 && code[6] == 'Y' {
		return 1.0
	}
	return 0.5
}

// ReplaceNodes replaces every node of n that match picks with f of it.
func ReplaceNodes(n *types.ReqNode, match func(*types.ReqNode) bool, f func(*types.ReqNode) *types.ReqNode) *types.ReqNode {
	if n == nil {
		return nil
	}
	if match(n) {
		return f(n)
	}
	for i, it := range n.Items {
		n.Items[i] = ReplaceNodes(it, match, f)
	}
	return n
}
