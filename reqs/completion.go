package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// ParseCompletion turns a program's completion requirements, as scraped,
// into their stored JSON form (docs/schema.md, "Program completion").
//
// The text is read into a block tree (completion_blocks.go) and each
// sentence is classified (completion_classify.go). Then:
//
//   - the header total ("(8.0 credits, including at least 0.5 credit at the
//     400-level)") is Req 1, an allocation counting what its children claim
//     plus its own pool, if it names one; the floors and caps stated with it
//     are constraint and limit rows under it (completion_header.go);
//   - each item (a numbered or lettered item, a bullet, a labelled line, a
//     line of codes) is an allocation under Req 1, or at the top when there
//     is no total; its body comes from the completion grammar, and the
//     floors, caps and approval routes written in it are its own
//     (completion_rows.go, completion_rules.go);
//   - lines that set out lists the items draw on are recorded as list
//     definitions (completion_lists.go); once every row is read, rows of
//     text that draw on lists by name are read again with the lists filter,
//     the lists they use join the envelope, and their notes go
//     (completion_named.go);
//   - program-level caps, CR/NCR and transfer rules are rows under Req 1;
//   - advice is a note: the item's own when it is written in the item, the
//     program's otherwise; sentences naming programs that can't be combined
//     with this one are exclusions, read as in enrolment;
//   - once every row is read (finish), sentences that relax a course a row
//     requires become alternatives on that row (completion_relax.go), lists
//     that widen a pool add their courses and a limit, and definitions of
//     designators widen the filters (completion_lines.go), routes stated
//     for the program go to the rows they concern (completion_routes.go),
//     and the counts the rows can't be seen to make up get Review markers
//     (completion_marks.go).
//
// Link then does the steps that need the catalogue (completion_link.go).
//
// Rows are numbered in Degree Explorer's order: the total and its floors,
// each item followed by its own rows, then the program-level rows. Text the
// grammar can't read stays, as a text node, in the row it belongs to.
func ParseCompletion(raw string) *types.Completion {
	c, _ := parseCompletion(raw)
	return c
}

// compDiag is what the parser noticed but couldn't place. The corpus report
// lists it.
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

// compRow is a requirement row while the text is read. Its children (the
// rows under it: floors, caps, parts) are numbered right after it.
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
	// programRoutes: approval routes and widened pools stated for the
	// program, not an item ("Students can request substitutions from the IRP
	// Director", "Up to 1.0 credit may be from other departments").
	// programRows: rows namedLists moves from an item to the program (a
	// cap that counts a list's courses wherever they are used).
	programRows   []*compRow
	programRoutes []string
	// joined maps a block made of two lines (one ending in ", OR") to the
	// second, for its raw.
	joined map[*compBlock]*compBlock
	roles  map[string]sentenceRole // classifySentence, once per sentence
	cues   map[string]sentenceCues
	// cleaned: complClean, once per string (clean).
	cleaned map[string]string
	// lineSents: each line's sentences, emphasis stripped (lineSentences).
	lineSents [][]string
	// routeContext: a program route's previous sentence, when the route
	// points back at its courses ("Students taking one of these MAT courses
	// ...").
	routeContext map[string]string
	// focusRows: the row a heading of focuses became (focusLine).
	focusRows map[*compBlock]*compRow
	// lastSentence: the sentence read before the current one (lastRule).
	lastSentence string
	// leadRows: the item a lead over sub-lists became, by the lead's block
	// and its last sub-list's (poolRuleLine).
	leadRows map[*compBlock]*compRow
	// relaxations: sentences that relax a course a requirement names,
	// placed once every row is read (relax).
	relaxations []string
	// overlaps: pairs of items a sentence lets share courses (overlap),
	// set once the rows are numbered.
	overlaps [][2]*compRow
	// omissions: parts of the items a student may leave out with no need to
	// replace them, written on the total once all are read (omitTotals).
	omissions []compOmission
	// poolConditions: a lead gave the total its pool and the items after it
	// may be floors on it (poolCondition).
	poolConditions bool
	// numberedSeen: a numbered item has been read at the program level;
	// lines of codes after it may be the lists it draws on (listLine).
	numberedSeen bool
	// listDefs: the lines that set out lists, kept as notes (listDef); the
	// ones an item refers to become named lists (namedLists).
	listDefs []*compListDef
	// belowRows: how many lines point at the lists below them by none of
	// their names (belowLists).
	belowRows int
	// unions: the lists made of union pools (unionList).
	unions []*compList
	// reqRefs: pools drawn from other requirements, numbered in finish.
	reqRefs []compReqRef
	// choiceRefs: the requirement nodes of choice rows (options), numbered
	// in finish (numberChoices).
	choiceRefs []compChoiceRef
	// tree: the program's blocks.
	tree []*compBlock
	// inPairPool: pairPool is reading an item with the item grammar.
	inPairPool bool
	// rowUnions: the lists of rows with no label (rowUnion), named after
	// their rows' numbers in finish (nameRowUnions).
	rowUnions []compRowUnion
}

// compCtx is what the blocks around a block say about it.
type compCtx struct {
	owner *compRow // the item the block is written in; nil at the program level
	// advice: the block sits under advice ("We suggest that students
	// consider the following logical course clusters ...:"), so its plain
	// requirement text is a note.
	advice bool
	// notes: the block is a note ("Note: ...", under "Notes:"), so a
	// requirement in it is a rule on the program, not an item.
	notes bool
	// numbered: the block is a numbered item ("2. Algebra: MAT240H1, ...").
	numbered bool
}

// finish numbers the rows (design 4) and builds the envelope. The passes,
// in order (the assembler notes list them too): floorItems, namedLists,
// collapseChoices, crossRules, focusChoices, levelReadings,
// additionalElectives, aliasDesignators, widen, relax, restatedItems,
// listRoutes, placeProgramRoutes, shareRoutes, languagePools, boundRoutes,
// boundListRoutes, openLists, substitutes, pairSubstitutes,
// exemptReplacements, exclusionAlternatives, oneOffs, streamPools,
// cognates, filterPools, moreOneOffs, bindingNotes, splitYAlternatives,
// checkFloorDesignators, campusFloors, checkItemPools, audiences,
// dedupeLimits; then numbering, nameRowUnions,
// numberReqRefs, numberChoices, textKinds, markIncomplete and the overlaps.
func (a *compAssembler) finish() *types.Completion {
	c := &types.Completion{Requirements: []*types.Requirement{}, Raw: a.raw}
	var add func(r *compRow, parent int)
	add = func(r *compRow, parent int) {
		r.r.Number = len(c.Requirements) + 1
		r.r.Parent = parent
		c.Requirements = append(c.Requirements, r.r)
		for _, k := range r.children {
			if k.r.Kind == types.RequirementChoice {
				add(k, 0) // after its options, but at the top (design 4)
				continue
			}
			add(k, r.r.Number)
		}
	}
	// The passes run in this order; each comment says what it must follow.
	// First: floor-worded items become constraints, which routeRows (the
	// rows the passes below may change) leaves out.
	a.floorItems()
	// After floorItems, whose floor-worded items are constraints by now:
	// rows of text that draw on lists by name read again with the lists.
	lists := a.namedLists()
	// After namedLists, which reads the options' rows of text with the
	// lists: options that are single bodies become one allocation's or.
	a.collapseChoices()
	// After namedLists, with the lists known; before the routes are placed:
	// rules that tie rows together and the program's floors and caps of
	// text read again (crossRules).
	a.crossRules(lists)
	// Any time before numbering: the rule a heading of focuses comes with
	// becomes the or of the focuses' program nodes.
	a.focusChoices()
	// After floorItems, with the header's floors final.
	a.levelReadings()
	// After floorItems: the elective share gathers the pools of the year
	// items still allocations, and adds an item the passes below see.
	a.additionalElectives()
	// Before widen: it widens designator filters and reads rows of text
	// again as designator pools, which widenTarget then finds.
	a.aliasDesignators()
	// After aliasDesignators (above). Like it, it drops the sentences it
	// encodes from the notes and program routes (dropEncoded), so it must run
	// before placeProgramRoutes.
	a.widen()
	// After the pools are final (aliasDesignators, widen): a relaxed course
	// may be one a widening added to a pool's courses (rowNames).
	a.relax()
	// After relax, whose alternatives an item may restate ("If a student has
	// not completed STA130H1 then an additional 0.5 credit ...").
	a.restatedItems()
	// After aliasDesignators and widen, which prune a.programRoutes of the
	// sentences they encoded: a pruned route would be placed twice.
	// Before placeProgramRoutes, which would keep the routes it places as
	// notes (completion_fixes.go).
	a.listRoutes()
	a.placeProgramRoutes()
	// After placeProgramRoutes, which leaves the share route a note; before
	// boundRoutes: the share route and the language pools of text
	// (completion_shares.go).
	a.shareRoutes()
	a.languagePools()
	// After placeProgramRoutes, which places the last approval routes: the
	// ones that name their wider pool are bounded by it.
	a.boundRoutes()
	// Like boundRoutes, on pools of lists (completion_limits.go).
	a.boundListRoutes(lists)
	// After boundListRoutes, which reads or(pool of lists, routes) as the
	// routes left it: pools of lists a note calls not exhaustive get their
	// Review alternative (completion_limits.go).
	a.openLists(lists)
	// After the routes and relaxations are placed (placeProgramRoutes,
	// relax), whose text alternatives it reads; before splitYAlternatives,
	// which may weigh a substitute it adds: substitutes, exemptions and
	// exclusions (completion_substitutes.go).
	a.substitutes()
	// After substitutes, which reads the in-lieu sentences of single
	// courses: one whose courses include a pair, with a list of its own.
	lists = a.pairSubstitutes(lists)
	// After substitutes: an exemption's replacement course, and the
	// exclusion advice that becomes an alternative on another requirement
	// (completion_leftovers.go).
	lists = a.exemptReplacements(lists)
	a.exclusionAlternatives()
	// After placeProgramRoutes and the lists: one-off readings
	// (completion_fixes.go).
	a.oneOffs()
	// After oneOffs: pools written inside an item (completion_streams.go).
	a.streamPools()
	// After streamPools: cognate caps as union pools (completion_cognates.go).
	lists = a.cognates(lists)
	// And filter pools (completion_filterpools.go).
	lists = a.filterPools(lists)
	// After the filter pools: more one-off readings (completion_oneoffs.go).
	lists = a.moreOneOffs(lists)
	// And the item wording the rules above left as text (completion_remarks.go);
	// before the routes below, which read its pools.
	lists = a.remarkRows(lists)
	// After the rows are built: the total's pool takes in the outside share
	// a limit caps (completion_limits.go).
	a.widenOutsideShares()
	// And a program-level cap that names its row by title.
	a.titledLimits()
	// And a unit's special-topics courses on a topic, from the notes
	// (completion_routes.go).
	a.topicContentRoutes()
	// And the language courses of the Department for the Study of Religion.
	a.dsrLanguagePools()
	// And the routes that name no target, to the notes.
	a.untargetedRoutes()
	a.placedElsewhere()
	// And the routes that name their wider pool by codes, a unit, subjects or
	// levels (completion_routes.go).
	a.namedRoutes(lists)
	// After them: a route's approved courses its lists already hold leave
	// the strict pool (completion_fixes.go).
	walkRows(a.items, approvedOnLists)
	a.graduateAtLevel()
	// And a note of other programs' courses for part of a designator's floor.
	a.otherProgramsFloors()
	a.exemptAtLevel()
	// And the caps only permission lifts (completion_limits.go).
	a.permittedCaps()
	// And the caps over the program a sentence sets (completion_limits.go).
	a.programCaps()
	// And the shares of cognate subjects (completion_limits.go).
	a.cognateShares()
	// And the counts of courses "and no more" (completion_limits.go).
	a.countNoMore()
	// And a refusal of transfer credits, under its item.
	a.transferUnderItem()
	// After the routes, caps and lists are placed: the notes the report
	// reads as rules (completion_binding.go).
	lists = a.bindingNotes(lists)
	// After relax and placeProgramRoutes, which wrap nodes in or(node,
	// alternative): it reads the or nodes as they end up, and a relaxation's
	// alternative may be a full-year course ("Students may use MAT351Y1
	// instead of APM346H1").
	splitYAlternatives(a.items)
	// After the routes and relaxations: their text alternatives make an item
	// one that may count courses outside the total's designators.
	a.checkFloorDesignators()
	// After checkFloorDesignators, which may drop a floor's designators, and
	// after widen, which may list more courses in a pool.
	a.campusFloors()
	// Last on the bodies: it marks a body that is still a bare amount of any
	// credits; an item a route or alternative above made or(credits, text)
	// is left alone.
	a.checkItemPools(a.items)
	// After the routes and alternatives are placed (placeProgramRoutes,
	// boundRoutes): a text alternative for one faculty's students gets its
	// audience (completion_limits.go).
	rest := append(append(append([]*compRow{}, a.floors...), a.items...), a.program...)
	a.audiences(rest)
	roots := rest
	if a.total != nil {
		a.total.children = append(rest, a.total.children...)
		roots = []*compRow{a.total}
	}
	// After every pass that adds a limit (widen adds some), before numbering.
	dedupeLimits(roots)
	// After dedupeLimits: a pool its limits leave short of its amount
	// (completion_spreads.go).
	a.unreachableCaps(roots, lists)
	// After unreachableCaps: two "/" caps read by the ambiguity policy
	// (completion_fixes.go).
	a.separatorCaps(roots, lists)
	for _, r := range roots {
		add(r, 0)
	}
	// After numbering: the lists of rows with no label take their rows'
	// numbers (completion_limits.go).
	lists = a.nameRowUnions(roots, lists)
	// After numbering, before markIncomplete: "from Requirements 1, 3, 4"
	// as row numbers.
	a.numberReqRefs()
	// After numbering, before markIncomplete, which reads the choices.
	a.numberChoices()
	// After every pass that reads texts of kind other, before
	// markIncomplete, whose markers take their causes' kind.
	textKinds(c)
	// Last: it reads the finished bodies and the numbers and parents that
	// add just set.
	markIncomplete(c, a.total != nil, true)
	// After numbering: overlaps are row numbers.
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

// dedupeLimits merges limits under one row that cap the same courses the
// same way, for the same audience ("No more than one from GGR273H1, GGR373H1" and, later, "No more
// than one of GGR273H1, GGR373H1"): one row, both sentences in its raw.
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

// sameLimit reports whether x and y are limits that cap the same courses the
// same way for the same students (Audience), over the same courses (Scope).
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

// checkFloorDesignators keeps a floor's designators only where they hold:
// a floor stated with the total takes the total's designators ("4.0 ENG
// credits as follows, including at least 1.5 credits at the 300 level or
// above"), but when an item counts courses of other designators (CRE, DRM
// and WRR courses in ASMIN1646), the floor is on the program's courses: its
// levels only. A floor that names the designator itself keeps it.
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
		// An item that is text may count anything (ASMAJ1647B's Creative
		// Writing pool): it isn't seen to be within the designators.
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
			// "including 2.0 credits from 300+ level Creative Writing
			// courses": and(credits, the condition as text).
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

// campusFloors lists, in a floor on what a row claims, the courses from
// another campus that the row's pool lists by code and that meet the floor.
// Designator and level filters match St. George courses only (the campus
// rule), so "At least 2.5 credits must be RLG courses" under a pool listing
// RLG371H5 would leave a student who uses it NotMet:
//   - a floor with designators takes those codes beside them (courses and
//     designators are either-or);
//   - a floor of levels alone has no source to add to: over a pool that is
//     a list of courses only, it counts the pool's courses at its levels;
//     over any other pool it is left as it is.
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
	var walk func(rs []*compRow)
	walk = func(rs []*compRow) {
		for _, r := range rs {
			if pool := listedPool(r.r.Req); pool != nil {
				for _, k := range r.children {
					if k.r.Kind == types.RequirementConstraint {
						campusFloor(k.r.Req, pool)
					}
				}
			}
			walk(r.children)
		}
	}
	walk(a.items)
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

// otherCampus reports whether code is a UTSC (suffix 3) or UTM (suffix 5)
// course.
func otherCampus(code string) bool {
	return strings.HasSuffix(code, "3") || strings.HasSuffix(code, "5")
}

// campusFloor adds to floor f the courses of pool that campusFloors gives
// it.
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
		// "At least 2.5 credits must be taken at the 300-level or higher"
		// over a list: the listed courses at those levels.
		f.Courses = slices.DeleteFunc(slices.Clone(pool.Courses), func(c string) bool { return !meets(c) })
	}
}

// role is classifySentence(s), which the assembler asks about the same
// sentence several times.
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

// parseCompletionText parses the text of one completion requirement (an
// item's body, without its number) with the completion grammar: the shared
// requisite grammar plus the extensions in phrases_completion.go and those
// marked "completion" in parser.go. Notes holds what it set aside: URLs and
// advice ("( MAT237Y1 recommended)", footnotes).
func parseCompletionText(s string) (req *types.ReqNode, notes []string) {
	// "CRE247H1, STS248H1, , STS301H1": an empty member of a list.
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
			// "any courses from program requirement (1) above" names a
			// requirement of this program, not a program to be in.
			x.Kind = types.TextOther
		}
	})
	return n, notes
}

// complEmptyMemberRe: a comma with nothing before the next one.
var complEmptyMemberRe = regexp.MustCompile(`,\s*,`)

// poolMembersByOr puts into a pool the courses "or" joins to it that are
// worth less than its amount: "1.0 credits in any EEB or ENV432H1" is 1.0
// credit from EEB courses and ENV432H1, as ENV432H1 alone couldn't meet it.
// A course worth the amount stays an alternative ("IMM450Y1 or 1.0 credit at
// the 400-level in ...").
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

// levelReadings gives a header floor at one level two readings when the
// program's items say that level "and up": "(4.0 credits including at least
// 1.0 credit at the 300-level)" over "1.0 credit at the 300+ level from
// ...". The strict reading is the floor as written (Met); the lenient one
// counts the levels above too (Review): or(floor, and(wider floor,
// text{ambiguity})).
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
