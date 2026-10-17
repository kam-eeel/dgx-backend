package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// ParseCompletion parses a program's completion text into requirement rows.
// The header total is Req 1; each item is an allocation under it, with the
// floors, caps and routes written in it; program-level rules follow the items.
// finish runs the passes that need every row, and Link the steps that need
// the catalogue. Text the grammar can't read stays a text node in its row.
func ParseCompletion(raw string) *types.Completion {
	c, _ := parseCompletion(raw)
	return c
}

// compDiag is what the parser noticed but couldn't place, for the corpus report.
type compDiag struct {
	textRows []string // sentences kept as a row of text
}

func parseCompletion(raw string) (*types.Completion, *compDiag) {
	a := &compAssembler{raw: raw, lines: strings.Split(raw, "\n"), diag: &compDiag{}, joined: map[*compBlock]*compBlock{}, leadRows: map[*compBlock]*compRow{}, focusRows: map[*compBlock]*compRow{},
		roles: map[string]sentenceRole{}, cues: map[string]sentenceCues{}, routeContext: map[string]string{}}
	a.tree = completionBlocks(raw)
	a.blocks(a.tree, compCtx{})
	return a.finish(), a.diag
}

// compRow is a requirement row while the text is read. Its children (floors,
// caps, parts) are numbered right after it.
type compRow struct {
	r        *types.Requirement
	children []*compRow
}

// compAssembler reads one program's blocks into rows.
type compAssembler struct {
	raw   string
	lines []string
	diag  *compDiag

	total      *compRow   // Req 1; nil when the text states no total
	floors     []*compRow // the floors and caps stated with the total
	items      []*compRow // the items, in document order
	program    []*compRow // program-level rules, numbered after the items
	notes      []string
	exclusions []*types.ReqNode
	// programRoutes: approval routes and widened pools stated for the program,
	// not an item. programRows: rows namedLists moves from an item to the program.
	programRows   []*compRow
	programRoutes []string
	// joined maps a block of two lines (one ending in ", OR") to the second.
	joined map[*compBlock]*compBlock
	roles  map[string]sentenceRole // classifySentence, once per sentence
	cues   map[string]sentenceCues
	// cleaned: complClean, once per string.
	cleaned map[string]string
	// lineSents: each line's sentences, emphasis stripped.
	lineSents [][]string
	// routeContext: a program route's previous sentence, when the route points
	// back at its courses ("Students taking one of these MAT courses").
	routeContext map[string]string
	// focusRows: the row a heading of focuses became.
	focusRows map[*compBlock]*compRow
	// lastSentence: the sentence read before the current one.
	lastSentence string
	// leadRows: the item a lead over sub-lists became, by the lead's block and
	// its last sub-list's.
	leadRows map[*compBlock]*compRow
	// relaxations: sentences relaxing a named course, placed once every row is read.
	relaxations []string
	// overlaps: pairs of items a sentence lets share courses.
	overlaps [][2]*compRow
	// omissions: parts of items a student may leave out without replacing them.
	omissions []compOmission
	// poolConditions: a lead gave the total its pool, so the items after it may
	// be floors on it.
	poolConditions bool
	// numberedSeen: a numbered item has been read at the program level; lines of
	// codes after it may be the lists it draws on.
	numberedSeen bool
	// listDefs: the lines that set out lists, kept as notes; the ones an item
	// refers to become named lists.
	listDefs []*compListDef
	// belowRows: how many lines point at the lists below them by none of their
	// names.
	belowRows int
	// unions: the lists made of union pools.
	unions []*compList
	// reqRefs: pools drawn from other requirements, numbered in finish.
	reqRefs []compReqRef
	// choiceRefs: the requirement nodes of choice rows, numbered in finish.
	choiceRefs []compChoiceRef
	// tree: the program's blocks.
	tree []*compBlock
	// inPairPool: pairPool is reading an item with the item grammar.
	inPairPool bool
	// rowUnions: the lists of unlabelled rows, named after their rows' numbers in
	// finish.
	rowUnions []compRowUnion
}

// compCtx is what the blocks around a block say about it.
type compCtx struct {
	owner *compRow // the item the block is written in; nil at the program level
	// advice: the block is under advice ("We suggest that students consider"),
	// so its requirement text is a note.
	advice bool
	// notes: the block is a note, so a requirement in it is a program rule.
	notes bool
	// numbered: the block is a numbered item ("2. Algebra: MAT240H1, ...").
	numbered bool
}

// finish runs the passes that need every row, numbers the rows and builds
// the envelope.
func (a *compAssembler) finish() *types.Completion {
	c := &types.Completion{Requirements: []*types.Requirement{}, Raw: a.raw}
	var add func(r *compRow, parent int)
	add = func(r *compRow, parent int) {
		r.r.Number = len(c.Requirements) + 1
		r.r.Parent = parent
		c.Requirements = append(c.Requirements, r.r)
		for _, k := range r.children {
			if k.r.Kind == types.RequirementChoice {
				add(k, 0) // after its options, at the top level
				continue
			}
			add(k, r.r.Number)
		}
	}
	// Pass order matters. The passes run in stages, each headed by a comment
	// after a blank line; a comment above a pass gives its constraint, if any.

	// rows, lists and choices
	// first: floor-worded items become constraints, which routeRows skips
	a.floorItems()
	// after floorItems
	lists := a.namedLists()
	// after namedLists
	a.collapseChoices()
	// after namedLists, before the routes are placed
	a.crossRules(lists)
	a.focusChoices()
	// after floorItems: the header's floors are final
	a.levelReadings()
	// after floorItems; adds an item the passes below see
	a.additionalElectives()

	// widen and relax
	// before widen, whose widenTarget finds the pools this makes
	a.aliasDesignators()
	// before placeProgramRoutes: drops the sentences it encodes from the routes
	a.widen()
	// after widen: a relaxed course may be one widen added to a pool
	a.relax()
	// after relax: an item may restate its alternatives
	a.restatedItems()

	// routes
	// after widen, which prunes the routes it encodes; before placeProgramRoutes
	a.listRoutes()
	a.placeProgramRoutes()
	// after placeProgramRoutes, before boundRoutes
	a.shareRoutes()
	a.languagePools()
	// after placeProgramRoutes: routes naming their wider pool are bounded by it
	a.boundRoutes()
	a.boundListRoutes(lists)
	// after boundListRoutes: pools a note calls not exhaustive keep a Review
	// alternative
	a.openLists(lists)

	// substitutes
	// after the routes and relaxations; before splitYAlternatives
	a.substitutes()
	// after substitutes
	lists = a.pairSubstitutes(lists)
	lists = a.exemptReplacements(lists)
	a.exclusionAlternatives()

	// one-offs and pools
	// after placeProgramRoutes; keep the passes below in this order
	a.oneOffsBeforePools()
	a.streamPools()
	lists = a.cognates(lists)
	lists = a.filterPools(lists)
	lists = a.oneOffsAfterPools(lists)
	// before the routes below, which read its pools
	lists = a.neighbourRows(lists)

	// routes over the pools
	a.widenOutsideShares()
	a.titledLimits()
	a.topicContentRoutes()
	a.dsrLanguagePools()
	a.untargetedRoutes()
	a.placedElsewhere()
	a.namedRoutes(lists)
	// after namedRoutes: approved courses already on a list leave the strict pool
	walkRows(a.items, approvedOnLists)
	a.graduateAtLevel()
	a.otherProgramsFloors()
	a.exemptAtLevel()

	// caps and limits
	a.permittedCaps()
	a.programCaps()
	a.cognateShares()
	a.countNoMore()
	a.transferUnderItem()

	// notes
	// after the routes, caps and lists are placed
	lists = a.encodeBindingNotes(lists)

	// alternatives and checks
	// after relax and placeProgramRoutes: reads the final or nodes
	splitYAlternatives(a.items)
	// after the routes and relaxations, whose text alternatives may count other
	// designators
	a.checkFloorDesignators()
	// after checkFloorDesignators and widen
	a.campusFloors()
	// last on the bodies: marks a body still a bare amount of any credits
	a.checkItemPools(a.items)

	// audiences and limits over all rows
	// after the routes and alternatives are placed
	rest := append(append(append([]*compRow{}, a.floors...), a.items...), a.program...)
	a.audiences(rest)
	roots := rest
	if a.total != nil {
		a.total.children = append(rest, a.total.children...)
		roots = []*compRow{a.total}
	}
	// after every pass that adds a limit, before numbering
	dedupeLimits(roots)
	// after dedupeLimits
	a.unreachableCaps(roots, lists)
	a.separatorCaps(roots, lists)

	// numbering and markers
	for _, r := range roots {
		add(r, 0)
	}
	// after numbering: unlabelled rows' lists take their row numbers
	lists = a.nameRowUnions(roots, lists)
	// after numbering, before markIncomplete
	a.numberReqRefs()
	a.numberChoices()
	// before markIncomplete, whose markers take their causes' kind
	textKinds(c)
	// last: reads the final bodies, numbers and parents
	markIncomplete(c, a.total != nil, true)
	// after numbering: overlaps are row numbers
	for _, o := range a.overlaps {
		if o[0].r.Parent == o[1].r.Parent {
			o[0].r.Overlaps = dedupe(append(o[0].r.Overlaps, o[1].r.Number))
			o[1].r.Overlaps = dedupe(append(o[1].r.Overlaps, o[0].r.Number))
		}
	}
	c.Lists = lists
	c.Notes = dedupe(a.notes)
	c.Exclusions = dedupeNodes(a.exclusions)
	return c
}

// dedupeLimits merges limits under one row that cap the same courses the same
// way for the same audience: one row, both sentences in its raw.
func dedupeLimits(rows []*compRow) {
	for _, r := range rows {
		var keep []*compRow
		for _, k := range r.children {
			if k.r.Kind == types.RequirementLimit {
				if i := slices.IndexFunc(keep, func(x *compRow) bool { return sameLimit(x.r, k.r) }); i >= 0 {
					addRaw(keep[i], k.r.Raw)
					continue
				}
			}
			keep = append(keep, k)
		}
		r.children = keep
		dedupeLimits(r.children)
	}
}

// sameLimit reports whether limits x and y cap the same courses the same way
// for the same Audience and Scope.
func sameLimit(x, y *types.Requirement) bool {
	if x.Kind != types.RequirementLimit || y.Kind != types.RequirementLimit || x.Scope != y.Scope || !sameNode(x.Req, y.Req) {
		return false
	}
	return x.Audience == nil && y.Audience == nil || x.Audience != nil && y.Audience != nil && sameNode(x.Audience, y.Audience)
}

// rawHead is the first line of r's raw: the line the row was read from.
func rawHead(r *types.Requirement) string {
	head, _, _ := strings.Cut(r.Raw, "\n")
	return head
}

// checkFloorDesignators drops the total's designators from its floors when an
// item counts courses of other designators: the floor is then on levels only.
// A floor that names the designator itself keeps it.
func (a *compAssembler) checkFloorDesignators() {
	if a.total == nil || a.total.r.Req.Type != types.NodeCredits || a.total.r.Req.Designators == nil {
		return
	}
	desigs := a.total.r.Req.Designators
	outside := false
	for _, r := range a.routeRows() {
		if r == a.total {
			continue
		}
		codes, filters := poolParts(r.r.Req)
		// an item of text may count anything
		outside = outside || hasText(r.r.Req) || slices.ContainsFunc(codes, func(c string) bool { return !slices.Contains(desigs, c[:3]) }) ||
			slices.ContainsFunc(filters, func(f *types.ReqNode) bool {
				return f.Designators == nil || slices.ContainsFunc(f.Designators, func(d string) bool { return !slices.Contains(desigs, d) })
			})
	}
	if !outside {
		return
	}
	for _, f := range a.floors {
		n := f.r.Req
		if n != nil && n.Type == types.NodeAnd {
			// "including 2.0 credits from 300+ level Creative Writing courses"
			for _, it := range n.Items {
				if it.Type == types.NodeCredits {
					n = it
					break
				}
			}
		}
		if f.r.Kind != types.RequirementConstraint || n == nil || n.Type != types.NodeCredits || !sameSet(n.Designators, desigs) {
			continue
		}
		named := slices.ContainsFunc(desigRe.FindAllString(codeRe.ReplaceAllString(f.r.Raw, " "), -1), func(d string) bool { return slices.Contains(desigs, d) }) ||
			slices.ContainsFunc(complSubjectWordRe.FindAllString(strings.ToLower(f.r.Raw), -1), func(w string) bool { return slices.Contains(desigs, subjectDesignators[w]) })
		if !named && (n.Levels != nil || n.Breadth != nil) {
			n.Designators = nil
		}
	}
}

// campusFloors adds to a floor on a row the row's listed UTSC and UTM courses
// that meet it, as designator and level filters match St. George courses only.
// A floor of levels alone takes them only over a pool that just lists courses.
func (a *compAssembler) campusFloors() {
	if a.total != nil {
		if pool := listedPool(a.total.r.Req); pool != nil {
			for _, f := range a.floors {
				if f.r.Kind == types.RequirementConstraint {
					campusFloor(f.r.Req, pool)
				}
			}
		}
	}
	walkRows(a.items, func(r *compRow) {
		if pool := listedPool(r.r.Req); pool != nil {
			for _, k := range r.children {
				if k.r.Kind == types.RequirementConstraint {
					campusFloor(k.r.Req, pool)
				}
			}
		}
	})
}

// listedPool returns the first pool in n that lists courses by code, or nil.
func listedPool(n *types.ReqNode) *types.ReqNode {
	var pool *types.ReqNode
	walkReq(n, func(x *types.ReqNode) {
		if pool == nil && x.Type == types.NodeCredits && x.Amount > 0 && x.Courses != nil && x.MaxAmount == nil {
			pool = x
		}
	})
	return pool
}

// otherCampus reports whether code is a UTSC (suffix 3) or UTM (suffix 5) course.
func otherCampus(code string) bool {
	return strings.HasSuffix(code, "3") || strings.HasSuffix(code, "5")
}

func campusFloor(f, pool *types.ReqNode) {
	if f == nil || f.Type != types.NodeCredits || f.Amount <= 0 || f.MaxAmount != nil || f.Items != nil ||
		f.Lists != nil || f.Requirements != nil || f.Breadth != nil || f.Designators == nil && f.Levels == nil {
		return
	}
	meets := func(c string) bool {
		return (f.Designators == nil || slices.Contains(f.Designators, c[:3])) &&
			(f.Levels == nil || slices.Contains(f.Levels, courseLevel(c))) && !slices.Contains(f.Exclude, c)
	}
	var off []string
	for _, c := range pool.Courses {
		if otherCampus(c) && meets(c) && !slices.Contains(f.Courses, c) {
			off = append(off, c)
		}
	}
	switch {
	case off == nil:
	case f.Designators != nil:
		f.Courses = dedupe(append(slices.Clone(f.Courses), off...))
	case f.Courses == nil && pool.Designators == nil && pool.Levels == nil:
		// a floor of levels over a list: the listed courses at those levels
		f.Courses = slices.DeleteFunc(slices.Clone(pool.Courses), func(c string) bool { return !meets(c) })
	}
}

// role is classifySentence(s), cached: the same sentence is asked about often.
func (a *compAssembler) role(s string) sentenceRole {
	r, ok := a.roles[s]
	if !ok {
		r = classifySentence(s)
		a.roles[s] = r
	}
	return r
}

// line returns block b's line as written, without its indent: a row's raw.
func (a *compAssembler) line(b *compBlock) string {
	s := strings.TrimSpace(a.lines[b.line])
	for _, i := range b.cont {
		s += " " + strings.TrimSpace(a.lines[i]) // a hard-wrapped paragraph
	}
	if next := a.joined[b]; next != nil {
		return s + "\n" + a.line(next)
	}
	return s
}

// parseCompletionText parses one requirement's text (an item's body, without
// its number) with the completion grammar. notes holds what it set aside: URLs
// and advice ("( MAT237Y1 recommended)").
func parseCompletionText(s string) (req *types.ReqNode, notes []string) {
	// "CRE247H1, STS248H1, , STS301H1": an empty list member
	s = complEmptyMemberRe.ReplaceAllString(s, ",")
	text, notes := normalize(complQuotedTitleRe.ReplaceAllString(s, "$1"))
	text, advice := stripAnnotations(stripLabels(text))
	text, more := rewriteCompletion(text)
	notes = append(append(notes, advice...), more...)
	if text == "" {
		return nil, notes
	}
	toks := gradCodes(text, lexCompletion(text))
	p := newParser(text, joinCompletionTitleRuns(text, toks))
	p.completion = true
	n := poolMembersByOr(p.programNodes(p.post(p.parseAll())))
	walkReq(n, func(x *types.ReqNode) {
		if x.Type == types.NodeText && x.Kind == types.TextMembership && complRefRe.MatchString(x.Text) {
			// "any courses from program requirement (1) above" names this program's
			// requirement, not a program to be in
			x.Kind = types.TextOther
		}
	})
	return n, notes
}

// complEmptyMemberRe matches an empty list member: ", ,".
var complEmptyMemberRe = regexp.MustCompile(`,\s*,`)

// poolMembersByOr puts into a pool the courses "or" joins to it that are worth
// less than its amount: "1.0 credits in any EEB or ENV432H1". A course worth
// the amount stays an alternative.
func poolMembersByOr(n *types.ReqNode) *types.ReqNode {
	if n == nil {
		return nil
	}
	for i, it := range n.Items {
		n.Items[i] = poolMembersByOr(it)
	}
	if n.Type != types.NodeOr {
		return n
	}
	var pool *types.ReqNode
	var codes []string
	for _, it := range n.Items {
		switch {
		case it.Type == types.NodeCredits && pool == nil && it.Amount > 0 && it.Items == nil && it.MaxAmount == nil &&
			it.MinGrade == 0 && (it.Designators != nil || it.Courses != nil):
			pool = it
		case it.Type == types.NodeCourse && it.MinGrade == 0:
			codes = append(codes, it.Code)
		default:
			return n
		}
	}
	if pool == nil || codes == nil || slices.ContainsFunc(codes, func(c string) bool { return courseWeight(c) >= pool.Amount }) {
		return n
	}
	merged := *pool
	merged.Courses = dedupe(append(slices.Clone(pool.Courses), codes...))
	if merged.Levels != nil && slices.ContainsFunc(codes, func(c string) bool { return !levelMatches(merged.Levels, c) }) {
		return n // the level would drop a listed course
	}
	return &merged
}

// levelReadings gives a header floor at one level two readings when the items
// say that level "and up": the floor as written (Met), or the floor counting
// the levels above too (Review).
func (a *compAssembler) levelReadings() {
	for _, f := range a.floors {
		n := f.r.Req
		if f.r.Kind != types.RequirementConstraint || n == nil || n.Type != types.NodeCredits || len(n.Levels) != 1 || n.Levels[0] >= 400 {
			continue
		}
		lv := strconv.Itoa(n.Levels[0])
		if !regexp.MustCompile(`\b` + lv + `\s*\+\s*-?\s*level`).MatchString(a.raw) {
			continue
		}
		wide := cloneNode(n)
		wide.Levels = nil
		for l := n.Levels[0]; l <= 400; l += 100 {
			wide.Levels = append(wide.Levels, l)
		}
		f.r.Req = orNode(n, andNode(wide, textNode(types.TextAmbiguity, `"at the `+lv+`-level" may mean the `+lv+`+ level, as the program's items say`)))
	}
}

// isAnyCredits reports whether n is an amount of any credits, with no filter.
func isAnyCredits(n *types.ReqNode) bool {
	return n.Type == types.NodeCredits && n.Amount > 0 && n.Courses == nil && n.Designators == nil && n.Levels == nil &&
		n.Breadth == nil && n.Lists == nil && n.Requirements == nil && !n.Transfer && !n.CrNcr && n.ExcludeDesignators == nil && n.MaxCourseAmount == 0
}

// notesTo adds notes to row, or to the program when row is nil.
func (a *compAssembler) notesTo(row *compRow, notes []string) {
	if row != nil {
		row.r.Notes = append(row.r.Notes, notes...)
	} else {
		a.notes = append(a.notes, notes...)
	}
}

// rowUnder adds a rule row under target, or under the program when target is
// nil.
func (a *compAssembler) rowUnder(target *compRow, r *types.Requirement) {
	if target != nil {
		target.children = append(target.children, &compRow{r: r})
	} else {
		a.program = append(a.program, &compRow{r: r})
	}
}

// clean is complClean(s), read once per string.
func (a *compAssembler) clean(s string) string {
	t, ok := a.cleaned[s]
	if !ok {
		if a.cleaned == nil {
			a.cleaned = map[string]string{}
		}
		t = complClean(s)
		a.cleaned[s] = t
	}
	return t
}

// addRaw adds sentence s to row's raw unless it is there already: a row's raw
// holds every sentence encoded in it.
func addRaw(row *compRow, s string) {
	s = strings.TrimSpace(s)
	if !strings.Contains(rawWords(row.r.Raw), rawWords(s)) {
		row.r.Raw += "\n" + s
	}
}

// rawWords is s without emphasis, extra spaces and a final period, to
// compare raws.
func rawWords(s string) string {
	s = strings.NewReplacer("**", "", "_", "").Replace(s)
	return strings.TrimSuffix(strings.Join(strings.Fields(s), " "), ".")
}

// structureOnly reports whether n is courses and their and/or, plus nodes
// leaf accepts.
func structureOnly(n *types.ReqNode, leaf func(*types.ReqNode) bool) bool {
	ok := true
	walkReq(n, func(x *types.ReqNode) {
		switch x.Type {
		case types.NodeAnd, types.NodeOr, types.NodeCourse:
		default:
			ok = ok && leaf(x)
		}
	})
	return ok
}

// isPool reports whether n is a credits node with an amount and a source.
func isPool(n *types.ReqNode) bool {
	return n != nil && n.Type == types.NodeCredits && n.Amount > 0 && n.Items == nil && (n.Courses != nil || n.Designators != nil)
}

func hasText(n *types.ReqNode) bool {
	found := false
	walkReq(n, func(x *types.ReqNode) { found = found || x.Type == types.NodeText })
	return found
}

func textNode(kind types.TextKind, s string) *types.ReqNode {
	return &types.ReqNode{Type: types.NodeText, Kind: kind, Text: cleanNote(s)}
}

func sameNode(x, y *types.ReqNode) bool {
	return len(dedupeNodes([]*types.ReqNode{x, y})) == 1
}

// fmtAmount writes a credit amount as the calendar does: "0.5".
func fmtAmount(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }

// weightOf returns the credits of codes together.
func weightOf(codes []string) float64 {
	sum := 0.0
	for _, c := range dedupe(slices.Clone(codes)) {
		sum += courseWeight(c)
	}
	return sum
}

// walkRows calls f on every row of rs and their children.
func walkRows(rs []*compRow, f func(r *compRow)) {
	for _, r := range rs {
		f(r)
		walkRows(r.children, f)
	}
}

// walkRowsPost calls f on every row of rs, after their children.
func walkRowsPost(rs []*compRow, f func(r *compRow)) {
	for _, r := range rs {
		walkRowsPost(r.children, f)
		f(r)
	}
}
