package grammar

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
		{types.TextPlacement, regexp.MustCompile(`(?i)^placement\s+in\b`)}, // "Placement in FSL420H1 ... or by Department permission"
		{types.TextPermission, regexp.MustCompile(`(?i)permission|approval|approved by|consent|ballot|application|interview|audition|portfolio|selection|supervis|discretion|by arrangement`)},
		{types.TextMembership, regexp.MustCompile(`(?i)admission|admitted|enrol|registered|accepted into|\b(specialist|major|minor|program|programme)\b|\bAS(MAJ|SPE|MIN|FOC|CER)\d{4}|(?-i:\b[A-Z]\w+ One\b)`)}, // "Vic One", not "Any one"
		{types.TextPlacement, regexp.MustCompile(`(?i)placement|\btest\b|assessment|proficien|fluen|native|knowledge of|formal education`)},
		{types.TextPreparation, regexp.MustCompile(`(?i)high school|grade 1[0-2]|\b4U\b|secondary|equivalent|background|experience|familiar|exposure|readings`)},
		{types.TextPreparation, regexp.MustCompile(`\b[A-Z]{3}[1-4][UMCEO]\b|\b[UM](?: or [UM])? level\b|\bRCM\b|(?i:conservatory)`)}, // Ontario high-school codes (SPH4U), RCM levels
	}
)

// post cleans up a parsed tree: merges text fragments that the
// lexer split apart, trims and drops noise text, dedupes children, and
// classifies text nodes.
func (p *parser) Post(n *types.ReqNode) *types.ReqNode {
	if n == nil {
		return nil
	}
	switch n.Type {
	case types.NodeText:
		return postText(n)
	case types.NodeAnd, types.NodeOr:
		items := p.mergeTexts(n.Items)
		for i := range items {
			items[i] = p.Post(items[i])
		}
		return p.combine(n.Type, DedupeNodes(items))
	case types.NodeChoose:
		for i := range n.Items {
			n.Items[i] = p.Post(n.Items[i])
		}
		n.Items = DedupeNodes(n.Items)
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
			a, b := p.Spans[last], p.Spans[it]
			if mergeable(last) && mergeable(it) && a.End <= b.Start {
				gap := p.src[a.End:b.Start]
				if !strings.ContainsAny(gap, ",;") && !CodeRe.MatchString(gap) {
					out[len(out)-1] = p.textSpan("", a.Start, b.End)
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
		n.Kind = Classify(n.Text)
	}
	return n
}

func Classify(text string) types.TextKind {
	for _, r := range kindRules {
		if r.re.MatchString(text) {
			return r.kind
		}
	}
	return types.TextOther
}

// DedupeNodes drops nils and structurally identical siblings.
func DedupeNodes(items []*types.ReqNode) []*types.ReqNode {
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
