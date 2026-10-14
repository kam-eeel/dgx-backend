package reqs

import (
	"encoding/json"
	"regexp"
	"strings"

	"degree-explorer/types"
)

var (
	noiseRe = regexp.MustCompile(`(?i)^\W*(a|an|and|or|the|of|courses?)?\W*$`)

	// kindRules: the first rule that matches decides a text node's kind.
	kindRules = []struct {
		kind types.TextKind
		re   *regexp.Regexp
	}{
		{types.KindPlacement, regexp.MustCompile(`(?i)^placement\s+in\b`)}, // "Placement in FSL420H1 ... or by Department permission"
		{types.KindPermission, regexp.MustCompile(`(?i)permission|approval|approved by|consent|ballot|application|interview|audition|portfolio|selection|supervis|discretion|by arrangement`)},
		{types.KindProgram, regexp.MustCompile(`(?i)admission|admitted|enrol|registered|accepted into|\b(specialist|major|minor|program|programme)\b|\bAS(MAJ|SPE|MIN|FOC|CER)\d{4}|(?-i:\b[A-Z]\w+ One\b)`)}, // "Vic One", not "Any one"
		{types.KindPlacement, regexp.MustCompile(`(?i)placement|\btest\b|assessment|proficien|fluen|native|knowledge of|formal education`)},
		{types.KindPreparation, regexp.MustCompile(`(?i)high school|grade 1[0-2]|\b4U\b|secondary|equivalent|background|experience|familiar|exposure|readings`)},
		{types.KindPreparation, regexp.MustCompile(`\b[A-Z]{3}[1-4][UMCEO]\b|\b[UM](?: or [UM])? level\b|\bRCM\b|(?i:conservatory)`)}, // Ontario high-school codes (SPH4U), RCM levels
	}
)

// post cleans up a parsed tree: merges text fragments that the
// lexer split apart, trims and drops noise text, dedupes children, and
// classifies text nodes.
func (p *parser) post(n *types.ReqNode) *types.ReqNode {
	if n == nil {
		return nil
	}
	switch n.Type {
	case types.NodeText:
		return postText(n)
	case types.NodeAnd, types.NodeOr:
		items := p.mergeTexts(n.Items)
		for i := range items {
			items[i] = p.post(items[i])
		}
		return p.combine(n.Type, dedupeNodes(items))
	case types.NodeChoose:
		for i := range n.Items {
			n.Items[i] = p.post(n.Items[i])
		}
		n.Items = dedupeNodes(n.Items)
	}
	return n
}

// mergeTexts joins consecutive unclassified text siblings when the source
// between them has no comma, semicolon or course code — e.g. "permission of
// the instructor" / "program administrator" split at "/".
func (p *parser) mergeTexts(items []*types.ReqNode) []*types.ReqNode {
	mergeable := func(n *types.ReqNode) bool { return n != nil && n.Type == types.NodeText && n.Kind == "" }
	var out []*types.ReqNode
	for _, it := range items {
		if len(out) > 0 {
			last := out[len(out)-1]
			a, b := p.spans[last], p.spans[it]
			if mergeable(last) && mergeable(it) && a.end <= b.start {
				gap := p.src[a.end:b.start]
				if !strings.ContainsAny(gap, ",;") && !codeRe.MatchString(gap) {
					out[len(out)-1] = p.textSpan("", a.start, b.end)
					continue
				}
			}
		}
		out = append(out, it)
	}
	return out
}

func postText(n *types.ReqNode) *types.ReqNode {
	n.Text = strings.Trim(n.Text, " ,;:.")
	if noiseRe.MatchString(n.Text) || isFiller(n.Text) {
		return nil
	}
	if n.Kind == "" {
		n.Kind = classify(n.Text)
	}
	return n
}

func classify(text string) types.TextKind {
	for _, r := range kindRules {
		if r.re.MatchString(text) {
			return r.kind
		}
	}
	return types.KindOther
}

// dedupeNodes drops nils and structurally identical siblings.
func dedupeNodes(items []*types.ReqNode) []*types.ReqNode {
	var out []*types.ReqNode
	seen := map[string]bool{}
	for _, it := range items {
		if it == nil {
			continue
		}
		b, _ := json.Marshal(it)
		if !seen[string(b)] {
			seen[string(b)] = true
			out = append(out, it)
		}
	}
	return out
}
