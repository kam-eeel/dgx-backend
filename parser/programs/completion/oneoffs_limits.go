package completion

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/parser/grammar"
	"degree-explorer/parser/programs"
	"degree-explorer/types"
)

// One-off readings of limits: floors, caps, transfer equivalents, breadth
// conditions, sequences and language levels.

// furtherFloorRe matches a pool with a floor: "Advanced Topics: 4.0
// credits of further APM/MAT 300+ level courses including at least 2.5 credits
// of APM/MAT courses at the 400-level".
var furtherFloorRe = regexp.MustCompile(`^(?:[A-Z][\w\s]*:\s*)?` + grammar.AmountPat + `\s+credits\s+of\s+further\s+([A-Z]{3}(?:/[A-Z]{3})*)\s+([1-4])00\+\s+level\s+courses\s+including\s+` + grammar.AtLeastPat + `\s+` + grammar.AmountPat + `\s+credits\s+of\s+([A-Z]{3}(?:/[A-Z]{3})*)\s+courses\s+at\s+the\s+([1-4])00-level$`)

// furtherFloor reads furtherFloorRe as the pool and the floor under it.
func furtherFloor(s string) (pool, floor *types.ReqNode) {
	m := furtherFloorRe.FindStringSubmatch(s)
	if m == nil || m[2] != m[5] || m[6] < m[3] {
		return nil, nil
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	f, _ := strconv.ParseFloat(m[4], 64)
	designators := strings.Split(m[2], "/")
	var levels []int
	for l := int(m[3][0]-'0') * 100; l <= 400; l += 100 {
		levels = append(levels, l)
	}
	pool = &types.ReqNode{Type: types.NodeCredits, Amount: v, Designators: designators, Levels: levels}
	floor = &types.ReqNode{Type: types.NodeCredits, Amount: f, Designators: slices.Clone(designators), Levels: []int{int(m[6][0]-'0') * 100}}
	return pool, floor
}

var (
	// requiresOfWhichRe matches a total stated as an item: "The Minor
	// Program requires 4.0 credits, of which 3.0 credits must be GER courses".
	requiresOfWhichRe = regexp.MustCompile(`(?i)^the\s+(?:minor|major|specialist)\s+program\s+requires\s+` + grammar.AmountPat + `\s+credits,\s+of\s+which\s+` + grammar.AmountPat + `\s+credits\s+must\s+be\s+(` + grammar.DesigPat + `)\s+courses$`)
	// cognateCapRe matches "A maximum of 1.0 credit in cognate courses is
	// allowed (...); please inquire at the Department and get approved in
	// advance".
	cognateCapRe = regexp.MustCompile(`(?i)^a\s+maximum\s+of\s+` + grammar.AmountPat + `\s+credits?\s+in\s+cognate\s+courses\s+is\s+allowed\b.*\bapprov`)
)

// cognateTotal reads a requiresOfWhichRe item, when a cognateCapRe
// note allows the rest, as the total or(credits{N, D}, and(credits{N},
// text{permission})) with the share of D a constraint under it.
func (a *Assembler) cognateTotal() {
	if a.total != nil {
		return
	}
	i := slices.IndexFunc(a.notes, func(s string) bool { return cognateCapRe.MatchString(s) })
	if i < 0 {
		return
	}
	for _, r := range a.items {
		t := r.r.Req
		if t == nil || t.Type != types.NodeText || len(r.children) > 0 {
			continue
		}
		m := requiresOfWhichRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(t.Text), "."))
		if m == nil {
			continue
		}
		total, _ := strconv.ParseFloat(m[1], 64)
		share, _ := strconv.ParseFloat(m[2], 64)
		rest, _ := strconv.ParseFloat(cognateCapRe.FindStringSubmatch(a.notes[i])[1], 64)
		if total-share != rest {
			return
		}
		note := a.notes[i]
		a.total = &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Raw: r.r.Raw + "\n" + note, Req: programs.OrNode(
			&types.ReqNode{Type: types.NodeCredits, Amount: total, Designators: []string{m[3]}},
			programs.AndNode(&types.ReqNode{Type: types.NodeCredits, Amount: total}, TextNode(types.TextPermission, note)),
		)}}
		a.floors = append(a.floors, &compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Req: &types.ReqNode{Type: types.NodeCredits, Amount: share, Designators: []string{m[3]}}, Raw: r.r.Raw}})
		a.notes = slices.Delete(a.notes, i, i+1)
		a.dropRow(r)
		return
	}
}

// departmentCap drops a dSRCountRe course-count cap's text row when its
// limit is a row and no other row speaks of Study of Religion courses: the
// limit's raw holds the sentence, which names no requirement of this program.
func (a *Assembler) departmentCap() {
	all := append(append(append([]*compRow{}, a.items...), a.program...), a.floors...)
	var text, limit *compRow
	other := false
	walkRows(all, func(r *compRow) {
		t := r.r.Req
		switch {
		case t != nil && t.Type == types.NodeText && r.r.Kind == types.RequirementConstraint && dSRCountRe.MatchString(t.Text) && courseCountCap(t.Text) != nil:
			text = r
		case r.r.Kind == types.RequirementLimit && t != nil:
			if limit == nil || r.r.Raw != limit.r.Raw {
				limit = r
			}
		case strings.Contains(strings.ToLower(r.r.Raw), "study of religion"):
			other = true
		}
	})
	if text == nil || limit == nil || limit.r.Raw != text.r.Raw || other || a.total != nil && strings.Contains(strings.ToLower(a.total.r.Raw), "study of religion") {
		return
	}
	a.dropRow(text) // the limit's raw holds the sentence

}

// transferEquivRe matches "Transfer credit equivalents of the following
// courses ... will qualify for this requirement: ECO324H1/ ...".
var transferEquivRe = regexp.MustCompile(`(?i)^transfer\s+credit\s+equivalents\s+of\s+the\s+following\s+courses\b[^:]*\bwill\s+qualify\s+for\s+this\s+requirement:\s*((?:` + grammar.CodePat + `\s*[/,]?\s*)+)\.?$`)

// transferAmountRe matches the amount a text starts with: "1.0 credit".
var transferAmountRe = regexp.MustCompile(`^` + grammar.AmountPat + `\s+credits?\b`)

// transferEquivalents reads a transferEquivRe alternative as transfer
// credits of the listed courses, of the amount the first text states. The
// sentence stays as content for the Summer Abroad courses.
func transferEquivalents(n *types.ReqNode) *types.ReqNode {
	if n.Items[0].Type != types.NodeText {
		return nil
	}
	am := transferAmountRe.FindStringSubmatch(strings.TrimSpace(n.Items[0].Text))
	if am == nil {
		return nil
	}
	v, _ := strconv.ParseFloat(am[1], 64)
	var items []*types.ReqNode
	found := false
	for _, x := range n.Items {
		if m := transferEquivRe.FindStringSubmatch(strings.TrimSpace(x.Text)); x.Type == types.NodeText && m != nil {
			items = append(items, &types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: grammar.Dedupe(grammar.CodeRe.FindAllString(m[1], -1)), Transfer: true}, TextNode(types.TextContent, x.Text))
			found = true
			continue
		}
		items = append(items, x)
	}
	if !found {
		return nil
	}
	return programs.OrNode(items...)
}

// floorIfCompletedRe matches "at least 1.0 must be 300+ series if 1.0
// credit is completed in #7 above" (ASSPE1006). The groups are the amount
// the condition names and the item.
var floorIfCompletedRe = regexp.MustCompile(`(?i)^` + grammar.AtLeastPat + `\s+` + grammar.NumPat + `\s+must\s+be\s+` + grammar.LevelPat + `\+\s+series\s+if\s+` + grammar.AmountPat + `\s+credits?\s+is\s+completed\s+in\s+#(\d+)\s+above$`)

// rangeLeadRe matches an item's range: "1.0 to 1.5 credits". The group
// is the upper amount.
var rangeLeadRe = regexp.MustCompile(`(?i)^(?:\d+\.\s+)?` + grammar.NumPat + `\s+to\s+` + grammar.AmountPat + `\s+credits?\b`)

// floorIfCompleted reads a floorIfCompletedRe text constraint as
// or(credits{upper, requirements: [the item]}, text): the floor is on credits
// no row counts, but an item claiming its upper amount makes the condition false.
func (a *Assembler) floorIfCompleted() {
	all := append(append(append([]*compRow{}, a.floors...), a.items...), a.program...)
	walkRows(all, func(r *compRow) {
		n := r.r.Req
		if r.r.Kind != types.RequirementConstraint || n == nil || n.Type != types.NodeText {
			return
		}
		m := floorIfCompletedRe.FindStringSubmatch(strings.TrimSpace(Clean(n.Text)))
		if m == nil {
			return
		}
		var item *compRow
		walkRows(a.items, func(x *compRow) {
			if x.r.Kind == types.RequirementAllocation && x.r.Label == m[2] {
				item = x
			}
		})
		if item == nil {
			return
		}
		rm := rangeLeadRe.FindStringSubmatch(item.r.Raw)
		cond, _ := strconv.ParseFloat(m[1], 64)
		if rm == nil {
			return
		}
		upper, _ := strconv.ParseFloat(rm[1], 64)
		if upper <= cond {
			return
		}
		ref := &types.ReqNode{Type: types.NodeCredits, Amount: upper, Requirements: []int{}}
		a.reqRefs = append(a.reqRefs, compReqRef{node: ref, rows: []*compRow{item}, scope: true})
		r.r.Req = programs.OrNode(ref, n)
	})
}

// breadthConditions reads a breadthUnlessRe row as a transcript
// constraint, or(the courses, credits{amount, breadth}). It claims nothing:
// the breadth course needn't be one the program counts.
func (a *Assembler) breadthConditions() {
	for _, r := range a.allRows() {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || n.Kind != types.TextOther || len(r.children) > 0 {
			continue
		}
		m := breadthUnlessRe.FindStringSubmatch(Clean(n.Text))
		if m == nil {
			continue
		}
		amount, _ := strconv.ParseFloat(m[2], 64)
		cat, _ := strconv.Atoi(m[3])
		alts := programs.CourseNodes(grammar.CodeRe.FindAllString(m[1], -1))
		alts = append(alts, &types.ReqNode{Type: types.NodeCredits, Amount: amount, Breadth: []int{cat}})
		r.r.Kind, r.r.Scope, r.r.Req = types.RequirementConstraint, types.ScopeTranscript, programs.OrNode(alts...)
	}
}

// compLanguageBlock is a oneLanguageRe lead and its language lines.
type compLanguageBlock struct {
	line    int     // the lead's line
	amount  float64 // the lead's amount
	options map[string]*types.ReqNode
	order   []string // the languages whose courses read
	names   []string // every language line's language, in order
}

// languageBlocks returns the program's first n oneLanguageRe leads with
// each language's courses as written.
func (a *Assembler) languageBlocks(n int) []compLanguageBlock {
	var blocks []compLanguageBlock
	for i := 0; i < len(a.lines) && len(blocks) < n; i++ {
		m := oneLanguageRe.FindStringSubmatch(Clean(strings.TrimSpace(a.lines[i])))
		if m == nil {
			continue
		}
		b := compLanguageBlock{line: i, options: map[string]*types.ReqNode{}}
		b.amount, _ = strconv.ParseFloat(m[1], 64)
		for _, l := range a.lines[i+1:] {
			g := languageLineRe.FindStringSubmatch(strings.TrimSpace(l))
			if g == nil {
				break
			}
			b.names = append(b.names, g[1])
			if opt, _ := parseCompletionText(g[2]); languageCodesRe.MatchString(g[2]) && opt != nil && !hasText(opt) {
				b.options[g[1]] = opt
				b.order = append(b.order, g[1])
			}
		}
		blocks = append(blocks, b)
	}
	return blocks
}

// progressiveSequenceRe matches "A two-year progressive sequence of
// courses in a single language totaling 2.0 credits" (ASMAJ1626).
var progressiveSequenceRe = regexp.MustCompile(`(?i)^a\s+two-year\s+progressive\s+sequence\s+of\s+courses\s+in\s+a\s+single\s+language\s+totall?ing\s+` + grammar.AmountPat + `\s+credits?\.?$`)

// progressiveSequence reads a progressiveSequenceRe text constraint
// over the first two language leads as an or, per language both give, of
// and(its first year's courses, its second's). It claims nothing.
func (a *Assembler) progressiveSequence() {
	blocks := a.languageBlocks(2)
	if len(blocks) < 2 {
		return
	}
	all := append(append(append([]*compRow{}, a.floors...), a.items...), a.program...)
	walkRows(all, func(r *compRow) {
		n := r.r.Req
		if r.r.Kind != types.RequirementConstraint || n == nil || n.Type != types.NodeText {
			return
		}
		m := progressiveSequenceRe.FindStringSubmatch(strings.TrimSpace(Clean(n.Text)))
		if m == nil {
			return
		}
		if v, _ := strconv.ParseFloat(m[1], 64); math.Abs(blocks[0].amount+blocks[1].amount-v) > 1e-9 {
			return
		}
		var alts []*types.ReqNode
		for _, lang := range blocks[0].order {
			if y := blocks[1].options[lang]; y != nil {
				alts = append(alts, programs.AndNode(grammar.CloneNode(blocks[0].options[lang]), grammar.CloneNode(y)))
			}
		}
		if len(alts) >= 2 {
			r.r.Req = programs.OrNode(alts...)
		}
	})
}

// inlineDesigFloorRe matches a floor defining its subject by
// designators: "at least 3.0 credits of which must be Department for the Study
// of Religion courses (RLG, JAR, ... or JSR designators)".
var inlineDesigFloorRe = regexp.MustCompile(`(?i)^` + grammar.AtLeastPat + `\s+` + grammar.AmountPat + `\s+credits?\s+of\s+which\s+must\s+be\s+[\w\s]+?\s+courses\s+\(\s*(` + grammar.DesigPat + `(?:\s*,\s*(?:or\s+)?` + grammar.DesigPat + `)*)\s+designators\s*\)$`)

// inlineDesignatorFloors reads and(credits, text{inlineDesigFloorRe}),
// alone or first beside route texts, as the credits with a constraint of the
// amount in the named designators.
func (a *Assembler) inlineDesignatorFloors() {
	for _, r := range a.allRows() {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil {
			continue
		}
		at := &r.r.Req
		if n.Type == types.NodeOr && len(n.Items) > 1 && !slices.ContainsFunc(n.Items[1:], func(x *types.ReqNode) bool { return x.Type != types.NodeText }) {
			at = &n.Items[0] // or(and(pool, floor), route text): the floor is the pool's
		}
		p := *at
		if p.Type != types.NodeAnd || len(p.Items) != 2 || p.Items[0].Type != types.NodeCredits || p.Items[1].Type != types.NodeText {
			continue
		}
		m := inlineDesigFloorRe.FindStringSubmatch(strings.TrimSuffix(Clean(p.Items[1].Text), "."))
		if m == nil {
			continue
		}
		amount, _ := strconv.ParseFloat(m[1], 64)
		if amount <= 0 || amount > p.Items[0].Amount {
			continue
		}
		*at = p.Items[0]
		floor := &compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Req: &types.ReqNode{Type: types.NodeCredits, Amount: amount, Designators: grammar.DesigRe.FindAllString(m[2], -1)}, Raw: r.r.Raw}}
		r.children = append([]*compRow{floor}, r.children...)
	}
}

// languageLevel reads languageLevelRe as the amount at the grade from
// either language's list or the other list at the 200+ level, or the rule as
// placement (a person judges equivalent proficiency). The rule's row goes.
func (a *Assembler) languageLevel(lists []*types.CourseList) {
	has := func(name string) bool {
		return slices.ContainsFunc(lists, func(l *types.CourseList) bool { return l.Name == name })
	}
	walkRows(a.items, func(r *compRow) {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || len(r.children) > 0 {
			return
		}
		m := languageLevelRe.FindStringSubmatch(strings.TrimSpace(Clean(r.r.Raw)))
		if m == nil || !has(m[3]) || !has(m[4]) || !has(m[7]) || m[5] != m[6] {
			return
		}
		var rule *compRow
		walkRows(append(slices.Clone(a.items), a.program...), func(q *compRow) {
			if q.r.Kind == types.RequirementConstraint && q.r.Req != nil && q.r.Req.Type == types.NodeText && rawWords(q.r.Req.Text) == rawWords(m[1]) {
				rule = q
			}
		})
		if a.total != nil {
			for _, q := range a.total.children {
				if q.r.Kind == types.RequirementConstraint && q.r.Req != nil && q.r.Req.Type == types.NodeText && rawWords(q.r.Req.Text) == rawWords(m[1]) {
					rule = q
				}
			}
		}
		v, _ := strconv.ParseFloat(m[5], 64)
		g, _ := strconv.Atoi(m[2])
		pool := func(list string, levels []int) *types.ReqNode {
			return &types.ReqNode{Type: types.NodeCredits, Amount: v, Lists: []string{list}, Levels: levels, MinGrade: g}
		}
		r.r.Req = programs.OrNode(pool(m[3], nil), pool(m[4], nil), pool(m[7], []int{200, 300, 400}), TextNode(types.TextPlacement, m[1]))
		r.r.Notes = nil
		if rule != nil {
			a.dropRow(rule)
		}
	})
}

// streamCourses reads streamCoursesRe on a header floor as a level floor
// on Req 1's pool, when a limit of 0 already keeps out what isn't the
// stream's.
func (a *Assembler) streamCourses() {
	if a.total == nil {
		return
	}
	t := a.total.r.Req
	if t == nil || t.Type != types.NodeCredits || t.Designators == nil {
		return
	}
	for _, r := range append(slices.Clone(a.floors), a.total.children...) {
		n := r.r.Req
		if r.r.Kind != types.RequirementConstraint || n == nil || n.Type != types.NodeAnd || len(n.Items) != 2 || n.Items[0].Type != types.NodeCredits || n.Items[1].Type != types.NodeText {
			continue
		}
		m := streamCoursesRe.FindStringSubmatch(strings.TrimSpace(n.Items[1].Text))
		if m == nil {
			continue
		}
		zero := false
		walkRows(append(slices.Clone(a.items), a.program...), func(q *compRow) {
			c := q.r.Req
			zero = zero || q.r.Kind == types.RequirementLimit && c != nil && c.Type == types.NodeCredits && c.MaxAmount != nil && *c.MaxAmount == 0 &&
				strings.Contains(strings.ToLower(q.r.Raw), "requirements for the stream in "+strings.ToLower(m[3]))
		})
		if !zero {
			continue
		}
		v, _ := strconv.ParseFloat(m[1], 64)
		// the floor counts what Req 1 counts: the level is all it adds
		r.r.Req = &types.ReqNode{Type: types.NodeCredits, Amount: v, Levels: grammar.ParseLevels(m[2] + "-level")}
	}
}

// languageWordRe matches a capitalised word, digits and "_" too: "Italian".
var languageWordRe = regexp.MustCompile(`[A-Z]\w+`)

// languagesFloor reads languagesFloorRe: the floor is a constraint under
// Req 1, and the row is the languages' designators with content text, or
// another language with permission.
func (a *Assembler) languagesFloor(r *compRow) {
	n := r.r.Req
	if a.total == nil || r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || len(r.children) > 0 {
		return
	}
	m := languagesFloorRe.FindStringSubmatch(strings.TrimSpace(Clean(n.Text)))
	if m == nil {
		return
	}
	var desigs []string
	for _, l := range languageWordRe.FindAllString(m[5], -1) {
		d := languageDesignators[strings.ToLower(l)]
		if d == nil {
			return
		}
		desigs = append(desigs, d...)
	}
	floor, _ := strconv.ParseFloat(m[1], 64)
	v, _ := strconv.ParseFloat(m[4], 64)
	r.r.Req = programs.OrNode(programs.AndNode(&types.ReqNode{Type: types.NodeCredits, Amount: v, Designators: desigs}, TextNode(types.TextContent, m[3])), TextNode(types.TextPermission, m[6]))
	a.total.children = append(a.total.children, &compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Raw: "At least " + m[1] + " " + m[2] + " credits",
		Req: &types.ReqNode{Type: types.NodeCredits, Amount: floor, Designators: []string{m[2]}}}})
}
