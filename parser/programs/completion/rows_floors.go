package completion

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/parser/grammar"
	"degree-explorer/parser/programs"
	"degree-explorer/types"
)

// Floors, caps and the total's pool while reading rows: caps and floors beside
// an item's pool, "Of these:" floors, and "Additional" items.

var (
	// totalPoolLeadRe matches a line giving the total its pool (group 2)
	// and leading into conditions on it: "At least 6.0 FAH credits fulfilling
	// the following distribution requirements:".
	totalPoolLeadRe = regexp.MustCompile(`(?i)^(?:` + grammar.AtLeastPat + `\s+)?` + grammar.AmountPat + `\s+(?:\(` + grammar.NumPat + `\)\s+)?(` + grammar.DesigPat + `(?:\s*(?:/|,|or|and/or)\s*` + grammar.DesigPat + `)*)\s+credits?\s+(?:fulfilling|meeting|that\s+meet|satisfying)\s+the\s+following\b[^.]*:?$`)
	// conditionFloorRe matches an item that is a floor on the total's
	// pool: "3.0 credits at the 300+ level".
	conditionFloorRe = regexp.MustCompile(`(?i)^(?:` + grammar.AtLeastPat + `\s+)?(?:one\s+|an?\s+)?` + grammar.NumPat + `\s+credits?\s+at\s+the\b`)
)

// totalPoolLead reads a totalPoolLeadRe line into the total's pool, and
// reports whether it did: the items after it that are floors are the pool's
// conditions.
func (a *Assembler) totalPoolLead(b *Block) bool {
	m := totalPoolLeadRe.FindStringSubmatch(strings.TrimSpace(b.Text))
	t := a.total
	if m == nil || t == nil || t.r.Req.Type != types.NodeCredits || t.r.Req.Designators != nil || t.r.Req.Courses != nil {
		return false
	}
	t.r.Req.Designators = grammar.DesigRe.FindAllString(m[2], -1)
	t.r.Raw += "\n" + a.line(b)
	a.poolConditions = true
	return true
}

// poolCondition makes an item that is a floor on the total's pool a constraint
// under the total with the pool's designators, after a lead into the pool's
// conditions. It reports whether it did.
func (a *Assembler) poolCondition(row *compRow, text string) bool {
	if !a.poolConditions || a.total == nil {
		return false
	}
	splitFloors(row, text) // "At least 2.0 credits at the 300+ level, 0.5 of which ..."
	n := row.r.Req
	t := a.total
	if !a.poolConditions || t == nil || n == nil || n.Type != types.NodeCredits || n.Levels == nil || n.Designators != nil ||
		n.Courses != nil || n.Items != nil || !conditionFloorRe.MatchString(strings.TrimSpace(text)) {
		return false
	}
	n.Designators = slices.Clone(t.r.Req.Designators)
	row.r.Kind = types.RequirementConstraint
	row.r.Label, row.r.Section = "", ""
	a.floors = append(a.floors, row)
	for _, k := range row.children {
		if k.r.Kind == types.RequirementConstraint && k.r.Req != nil && k.r.Req.Type == types.NodeCredits && k.r.Req.Designators == nil {
			k.r.Req.Designators = slices.Clone(t.r.Req.Designators)
		}
		a.floors = append(a.floors, k)
	}
	row.children = nil
	return true
}

// headingFloorRe matches a floor in a heading's brackets: "Fourth Year:
// (at least 2.0 credits at 400-level)".
var headingFloorRe = regexp.MustCompile(`(?i)\(\s*(?:[^()]*?,\s*)?(` + grammar.AtLeastPat + `\s+[^()]+?)\s*\)`)

// headingFloors keeps a floor stated in a heading as a constraint on the
// program (or the item the heading is in): the heading is for display only, but
// its floor is a rule.
func (a *Assembler) headingFloors(b *Block, ctx compCtx) {
	m := headingFloorRe.FindStringSubmatch(b.Text)
	if m == nil {
		return
	}
	for _, r := range a.floorRows(m[1]) {
		r.Raw = a.line(b)
		a.rowUnder(ctx.owner, r)
	}
}

// splitCaps makes the caps the grammar puts beside an item's pool (from a group
// of alternatives flattened into it) limit rows on the item.
func splitCaps(row *compRow) {
	var caps []*compRow
	var strip func(n *types.ReqNode) *types.ReqNode
	strip = func(n *types.ReqNode) *types.ReqNode {
		if n == nil || n.Type != types.NodeAnd && n.Type != types.NodeOr {
			return n
		}
		var keep []*types.ReqNode
		for _, it := range n.Items {
			if n.Type == types.NodeAnd && isCourseCap(it) {
				caps = append(caps, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Req: it, Raw: row.r.Raw}})
				continue
			}
			keep = append(keep, strip(it))
		}
		if n.Type == types.NodeAnd {
			return programs.AndNode(keep...)
		}
		return programs.OrNode(keep...)
	}
	row.r.Req = strip(row.r.Req)
	row.children = append(row.children, caps...)
}

// isCourseCap reports a cap on listed courses: credits{courses, max_amount}.
func isCourseCap(n *types.ReqNode) bool {
	return n.Type == types.NodeCredits && n.MaxAmount != nil && n.Amount == 0 && n.Courses != nil && n.Items == nil &&
		n.Designators == nil && n.Levels == nil
}

// poolCaps splits and(pool, caps...) into the pool and its caps; anything else
// is returned whole.
func poolCaps(n *types.ReqNode) (*types.ReqNode, []*types.ReqNode) {
	if n == nil || n.Type != types.NodeAnd || len(n.Items) < 2 || !isPool(n.Items[0]) ||
		slices.ContainsFunc(n.Items[1:], func(x *types.ReqNode) bool { return !isCourseCap(x) }) {
		return n, nil
	}
	return n.Items[0], n.Items[1:]
}

// floorWordsRe matches wording that makes a narrower credit phrase a floor
// on the one before it: "which includes".
var floorWordsRe = regexp.MustCompile(`(?i)\bof\s+which\b|\binclud(?:ing|es?)\b|\b` + grammar.AtLeastPat + `\b|\bmust\s+be\b|\b` + grammar.NoMoreThanPat + `\b`)

// splitFloors makes the floors the grammar puts beside an item's pool
// constraints on the item, never an in-node and: "3.0 SOC credits, which
// includes at least 1.0 SOC credit at the 300-level" is two rows.
func splitFloors(row *compRow, text string) {
	n := row.r.Req
	if n != nil && n.Type == types.NodeOr && len(n.Items) > 1 && n.Items[0].Type == types.NodeAnd &&
		!slices.ContainsFunc(n.Items[1:], func(x *types.ReqNode) bool { return x.Type != types.NodeText }) {
		// or(and(pool, floor), route text): the floors are the item's.
		inner := &compRow{r: &types.Requirement{Req: n.Items[0], Raw: row.r.Raw}}
		splitFloors(inner, text)
		n.Items[0] = inner.r.Req
		row.children = append(inner.children, row.children...)
		return
	}
	if n == nil || n.Type != types.NodeAnd || !floorWordsRe.MatchString(text) {
		return
	}
	var main *types.ReqNode
	for _, it := range n.Items {
		if it.Type == types.NodeCredits && it.Items == nil && it.MaxAmount == nil && it.Amount > 0 && (main == nil || it.Amount > main.Amount) {
			main = it
		}
	}
	if main == nil {
		return
	}
	var keep []*types.ReqNode
	var floors []*compRow
	for _, it := range n.Items {
		switch {
		case it != main && narrowsPool(main, it):
			// a copy: the floor shares its filters with the pool
			floors = append(floors, &compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Req: grammar.CloneNode(it), Raw: row.r.Raw}})
		case it.Type == types.NodeCredits && it.MaxAmount != nil && it.Amount == 0 && it.Items == nil:
			// a cap on the item: "of which no more than 0.5 credit can be ..."
			floors = append(floors, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Req: grammar.CloneNode(it), Raw: row.r.Raw}})
		default:
			keep = append(keep, it)
		}
	}
	if floors == nil {
		return
	}
	row.r.Req = programs.AndNode(keep...)
	row.children = append(floors, row.children...)
}

// narrowsPool reports whether credits node c is a share of pool p: a smaller
// amount of the courses p counts, narrowed by a level, designator or
// breadth.
func narrowsPool(p, c *types.ReqNode) bool {
	if codes := courseChoice(c); codes != nil {
		// a course the pool must include: "including CLA160H1/ CLA170H1"
		return !slices.ContainsFunc(codes, func(code string) bool { return !inPool(p, code) })
	}
	if c.Type != types.NodeCredits || c.Items != nil || c.MaxAmount != nil || c.Amount <= 0 || c.Amount >= p.Amount {
		return false
	}
	if isAnyCredits(p) {
		return true // "2.5 credits at least 1.5 credits from PCL courses: ..."
	}
	if c.Courses != nil {
		return !slices.ContainsFunc(c.Courses, func(code string) bool { return !inPool(p, code) })
	}
	return (c.Designators == nil || p.Designators == nil || programs.IsSubset(c.Designators, p.Designators)) &&
		(c.Levels != nil || c.Breadth != nil || c.Designators != nil && p.Designators == nil || len(c.Designators) < len(p.Designators))
}

// courseChoice returns the codes of a course or of an "or" of courses
// (CourseChoiceCodes), else nil.
func courseChoice(n *types.ReqNode) []string {
	if n.Type == types.NodeCourse {
		return []string{n.Code}
	}
	return grammar.CourseChoiceCodes(n)
}

// inPool reports whether pool p counts course code.
func inPool(p *types.ReqNode, code string) bool {
	return slices.Contains(p.Courses, code) || (p.Designators != nil || p.Levels != nil) && grammar.FilterMatches(p, code)
}

var (
	// additionalTotalRe matches "Additional <courses> to a total of N
	// credits": "Additional HPS courses to a total of 4.0 credits". Groups: the
	// courses, N, the total's designators and the rest.
	additionalTotalRe = regexp.MustCompile(`(?i)^(?:an?\s+)?additional\s+(.+?)\s+to\s+a\s+total\s+of\s+` + grammar.AmountPat + `\s+(` + grammar.DesigPat + `(?:\s*/\s*` + grammar.DesigPat + `)*\s+)?credits?\b(.*)$`)
	// additionalAmountRe matches an amount before the pool: "1.5 to 2.0
	// credits from".
	additionalAmountRe = regexp.MustCompile(`(?i)^` + grammar.NumPat + `(?:\s*(?:to|-|–)\s*` + grammar.NumPat + `)?\s+credits?\s+from\s+`)
	// additionalExamplesRe matches examples inside the pool: "which may
	// include courses from among CHM325H1".
	additionalExamplesRe = regexp.MustCompile(`(?i),?\s*which\s+may\s+include\s+courses\s+from\s+among\s+(.+)$`)
	// What may follow the total: "( CHM299H1 excluded)", "(Note: CHM395Y1 can
	// be used ...)", "counting for the program", "and meeting the following two
	// conditions:", ", with at least 1.0 credit at the 300+level".
	additionalExcludedRe   = regexp.MustCompile(`(?i)^\s*\(([^()]*?)\s+excluded\)`)
	additionalAlsoRe       = regexp.MustCompile(`(?i)^\s*\.?\s*\(\s*note:\s*([^()]*?)\s+can\s+be\s+used\s+toward\s+this\s+requirement\s*\)`)
	additionalCountingRe   = regexp.MustCompile(`(?i)^\s*counting\s+for\s+the\s+program\b`)
	additionalConditionsRe = regexp.MustCompile(`(?i)^\s*,?\s*and\s+(?:meeting|fulfilling)\s+the\s+following\s+(?:\w+\s+)?(?:conditions?|requirements?)\s*:?\s*$`)
	additionalFloorRe      = regexp.MustCompile(`(?i)^\s*,\s*((?:with\s+` + grammar.AtLeastPat + `|including)\s+.+)$`)
	// alsoCountsRe matches courses that also count: "CSB450H1 is also
	// acceptable".
	alsoCountsRe = regexp.MustCompile(`(?i)^((?:` + grammar.CodePat + `(?:\s*,\s*|\s*,?\s+or\s+|\s*/\s*)?)+)\s+(?:is|are)\s+also\s+acceptable$|^((?:` + grammar.CodePat + `(?:\s*,\s*|\s*,?\s+or\s+|\s*/\s*)?)+)\s+may\s+also\s+be\s+used\s+to\s+fulfil+\s+this\s+requirement$`)
)

// additional reads "Additional <courses> to a total of N credits" into Req 1's
// pool, and an N other than the total into a constraint under Req 1. It reports
// false, reading nothing, if Req 1 has a pool or the wording is unknown.
func (a *Assembler) additional(text, raw string) bool {
	if a.total == nil {
		return false
	}
	pool := a.total.r.Req
	sents := SplitSentences(text)
	first := additionalAndRe.ReplaceAllString(strings.TrimSuffix(a.clean(sents[0]), "."), "")
	m := additionalTotalRe.FindStringSubmatch(first)
	if m == nil || pool.Type != types.NodeCredits || pool.Designators != nil || pool.Levels != nil || pool.Courses != nil {
		return false
	}
	phrase := strings.TrimSuffix(strings.TrimSpace(additionalAmountRe.ReplaceAllString(m[1], "")), ",")
	var also []string
	if e := additionalExamplesRe.FindStringSubmatchIndex(phrase); e != nil {
		also = grammar.CodeRe.FindAllString(phrase[e[2]:e[3]], -1)
		phrase = phrase[:e[0]]
	}
	// "or Equivalent courses (listed below)": the named list joins the pool
	var listed []string
	if e := additionalListRe.FindStringSubmatchIndex(phrase); e != nil {
		listed = a.namedListCodes(phrase[e[2]:e[3]])
		if listed == nil {
			return false
		}
		phrase = phrase[:e[0]] + " courses"
	}
	// a subject is its designator; "0.5 credit courses" are half courses only
	half := false
	if w := additionalWeightRe.FindString(phrase); w != "" {
		half = strings.Fields(w)[0] == "0.5"
	}
	phrase = additionalWeightRe.ReplaceAllString(phrase, " ")
	phrase = subjectWordCIRe.ReplaceAllStringFunc(phrase, func(w string) string { return grammar.SubjectDesignators[strings.ToLower(w)] })
	n, _, cond := hdrReadBody(phrase)
	if cond || n.Designators == nil && n.Levels == nil || n.Courses != nil {
		return false
	}
	tail := m[4]
	var excl []string
	var floors, rest []string
	var aside string
	for strings.TrimSpace(tail) != "" {
		switch {
		case additionalExcludedRe.MatchString(tail):
			g := additionalExcludedRe.FindStringSubmatch(tail)
			excl = append(excl, grammar.CodeRe.FindAllString(g[1], -1)...)
			tail = tail[len(g[0]):]
		case additionalAlsoRe.MatchString(tail):
			g := additionalAlsoRe.FindStringSubmatch(tail)
			also = append(also, grammar.CodeRe.FindAllString(g[1], -1)...)
			aside = strings.Trim(g[0], " .()")
			tail = tail[len(g[0]):]
		case additionalCountingRe.MatchString(tail):
			tail = tail[len(additionalCountingRe.FindString(tail)):]
		case additionalConditionsRe.MatchString(tail):
			// "and meeting the following two conditions:": the items after it
			// are floors on the pool
			a.poolConditions = true
			tail = ""
		case additionalFloorRe.MatchString(tail):
			floors = append(floors, additionalFloorRe.FindStringSubmatch(tail)[1])
			tail = ""
		case strings.Trim(tail, " .") == "":
			tail = ""
		default:
			return false
		}
	}
	for _, sent := range sents[1:] {
		c := strings.TrimSuffix(a.clean(sent), ".")
		if g := alsoCountsRe.FindStringSubmatch(c); g != nil {
			also = append(also, grammar.CodeRe.FindAllString(g[1]+g[2], -1)...)
			continue
		}
		rest = append(rest, sent) // read as the program's rules below
	}
	// a listed course the pool's levels or exclusions drop can't join it: the
	// note is the total's alternative, for Review
	var outside []string
	also = slices.DeleteFunc(grammar.Dedupe(also), func(c string) bool {
		if !grammar.LevelMatches(n.Levels, c) || slices.Contains(excl, c) {
			outside = append(outside, c)
			return true
		}
		return false
	})
	total, _ := strconv.ParseFloat(m[2], 64)
	pool.Designators, pool.Levels = slices.Clone(n.Designators), slices.Clone(n.Levels)
	if half {
		pool.MaxCourseAmount = 0.5
	}
	pool.Exclude, pool.Courses = grammar.Dedupe(excl), grammar.Dedupe(append(also, listed...))
	a.total.r.Raw += "\n" + raw
	if outside != nil && aside != "" {
		a.total.r.Req = programs.OrNode(pool, TextNode(types.TextOther, aside))
	}
	if total != pool.Amount {
		// a sub-total counts its own designators, else the pool's at every
		// level (the level words describe the additional courses), else the
		// pool as is
		sub := &types.ReqNode{Type: types.NodeCredits, Amount: total, Designators: grammar.DesigRe.FindAllString(m[3], -1)}
		switch {
		case sub.Designators != nil:
		case pool.Designators != nil:
			sub.Designators, sub.Courses = slices.Clone(pool.Designators), slices.Clone(pool.Courses)
		default:
			sub.Levels, sub.Exclude, sub.Courses = slices.Clone(pool.Levels), slices.Clone(pool.Exclude), slices.Clone(pool.Courses)
		}
		a.program = append(a.program, &compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Req: sub, Raw: raw}})
	}
	for _, f := range floors {
		for _, r := range a.floorRows(f) {
			a.program = append(a.program, &compRow{r: r})
		}
	}
	for _, s := range rest {
		// a cap on the named list's courses
		if listed != nil && additionalListCapRe.MatchString(a.clean(s)) {
			v, _ := strconv.ParseFloat(additionalListCapRe.FindStringSubmatch(a.clean(s))[1], 64)
			a.program = append(a.program, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Raw: s,
				Req: &types.ReqNode{Type: types.NodeCredits, Courses: slices.Clone(listed), MaxAmount: grammar.MaxAmount(v)}}})
			continue
		}
		a.notesTo(nil, a.sentence(s, nil, compCtx{}))
	}
	return true
}

var (
	// additionalAndRe matches a conjunction with the next item: "; and".
	additionalAndRe = regexp.MustCompile(`(?i)\s*;\s*and$`)
	// additionalListRe matches a named list below: "or Equivalent courses
	// (listed below)".
	additionalListRe = regexp.MustCompile(`(?i)\s+or\s+([A-Z][\w\s]*?)\s+courses\s+\(listed\s+below\)\s*$`)
	// additionalWeightRe matches a course weight inside the phrase: "0.5
	// credit".
	additionalWeightRe = regexp.MustCompile(`(?i)\s+` + grammar.NumPat + `\s+credits?\s+`)
	// additionalListCapRe matches a cap on the named list: "No more than
	// 1.0 credit from the Equivalent courses can contribute".
	additionalListCapRe = regexp.MustCompile(`(?i)^` + grammar.NoMoreThanPat + `\s+` + grammar.AmountPat + `\s+credits?\s+from\s+the\s+[\w\s]+?\s+courses\s+can\s+contribute\b`)
)

// ofTheseLeadRe matches a lead into floors on the total: "Of these:".
var ofTheseLeadRe = regexp.MustCompile(`(?i)^of\s+these\s*:?$`)

// programFloors reads a line under "Of these:": its floors and caps are the
// total's, numbered with the floors stated with it; other sentences are read as
// usual.
func (a *Assembler) programFloors(b *Block) {
	for _, s := range SplitSentences(b.Text) {
		switch a.role(s) {
		case RoleRequirement, roleConstraint:
			if rows, _, ok := parseHeaderFloors(s); ok && len(rows) == 1 && rows[0].Kind == types.RequirementLimit {
				rows[0].Raw = s
				a.totalFloor(rows[0])
				continue
			}
			for _, r := range a.floorRows(s) {
				a.totalFloor(r)
			}
		case roleLimit:
			if r := capRow(s); r != nil {
				a.totalFloor(r) // "No more than 1.0 credit of 100-level courses may be used ..."
				continue
			}
			a.notesTo(nil, a.sentence(s, nil, compCtx{}))
		default:
			a.notesTo(nil, a.sentence(s, nil, compCtx{}))
		}
	}
	for _, c := range b.Children {
		a.programFloors(c)
	}
}

// totalFloor adds a floor or cap on the total: with Req 1's own floors, or a
// program row when there is no total.
func (a *Assembler) totalFloor(r *types.Requirement) {
	if a.total == nil {
		a.rowUnder(nil, r)
		return
	}
	a.floors = append(a.floors, &compRow{r: r})
}

// floorBlock reads a block under a lead into floors ("including:"): each
// requirement sentence is a constraint on row.
func (a *Assembler) floorBlock(b *Block, row *compRow) {
	for _, s := range SplitSentences(b.Text) {
		switch a.role(s) {
		case RoleRequirement, roleConstraint:
			a.constraint(row, s)
		default:
			a.sentence(s, row, compCtx{owner: row})
		}
	}
	for _, c := range b.Children {
		a.floorBlock(c, row)
	}
}

// subjectWordCIRe matches the subject names of SubjectDesignators in any
// case: "Additional Philosophy courses".
var subjectWordCIRe = regexp.MustCompile(`(?i)` + subjectWordRe.String())
