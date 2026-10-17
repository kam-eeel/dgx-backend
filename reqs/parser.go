package reqs

import (
	"math"
	"regexp"
	"slices"
	"strings"

	"degree-explorer/types"
)

// Grammar, lowest precedence first:
//
//	field := segment { (';' | COND) segment }             AND; COND segments are OR'd together
//	seq   := alt { (',' | ', and' | 'and' | ', or') alt } AND (', or' -> OR), left to right
//	alt   := unit { ('/' | 'or') unit }                   OR
//	unit  := atom { atom }                                adjacency -> AND
//	atom  := COURSE | CREDITS | GPA | TEXT | COND
//	       | '(' field ')' | GRADE_PREFIX atom | POOL pool
//	pool  := item { sep item }                            see parsePool
//
// Completion text (parseCompletionText) adds, with p.completion set and the
// completion-only tokens of lexCompletion (marked "completion" below):
//
//	atom  := ... | EITHER seq ('or' | ', or') seq         "Either A, B or C": the "or" splits the whole
//	seq   := ... { EXCLUDE }                              ", excluding A, B": exclude on the pool before
//	pool  := item { (sep | ';') item } [ 'and' COURSE ]   ";" separates items like ","; "A, B and C"
//	item  := ... | WILD                                   "PSL400-series courses": a filter
//
// and, in the pools themselves:
//   - "or" before an item closes the list as ", or" does ("A, B or C; D");
//   - an amount pool of courses and filters is one union (unionPool), and a
//     pool of groups whose credits add up to the amount is their "and"
//     (groupPool);
//   - a pool with a level ("0.5 credit at the 400-level from") right after
//     "including" is a floor, kept as text.
//
// Text rewrites before lexing (phrases_completion.go): labels stripped
// ("Option A: Immunology:"), annotations stripped or set aside as notes
// ("(recommended)", "(no longer offered)", footnotes, course titles), and
// the rewrites of rewriteCompletion: among them "in either GRK or LAT" read
// as "in GRK or LAT", and a Y course after "/" made the alternative to the
// pair before it ("( MAT148H1, MAT149H1/ MAT137Y1)" reads "( MAT148H1,
// MAT149H1)/ MAT137Y1").

type span struct{ start, end int }

type parser struct {
	src     string
	toks    []token
	pos     int
	lastEnd int
	spans   map[*types.ReqNode]span // side table, so ReqNode stays a clean JSON type

	completion bool // completion text: the grammar extensions marked "completion" below apply
	noOr       int  // > 0 while parsing the first branch of "Either A or B": "or" ends it
}

func newParser(src string, toks []token) *parser {
	return &parser{src: src, toks: toks, spans: map[*types.ReqNode]span{}}
}

// parseAll parses every token, stepping over stray ")" so nothing after one
// is lost.
func (p *parser) parseAll() *types.ReqNode {
	var parts []*types.ReqNode
	for !p.done() {
		parts = append(parts, p.parseField())
		if p.at(tRParen) {
			p.next()
		}
	}
	return p.and(parts...)
}

func (p *parser) parseField() *types.ReqNode {
	var plain, branches []*types.ReqNode
	for !p.done() && !p.at(tRParen) {
		switch {
		case p.at(tSemi):
			p.next()
			if p.at(tOr) { // "A; or B": an alternative to everything before it
				p.next()
				plain = []*types.ReqNode{p.or(p.and(plain...), p.parseSeq())}
			}
		case p.at(tCond):
			body := []*types.ReqNode{p.text(types.TextMembership, p.next())}
			for !p.done() && !p.at(tCond, tRParen) {
				if p.at(tSemi) {
					p.next()
					continue
				}
				body = append(body, p.parseSeq())
			}
			branches = append(branches, p.and(body...))
		default:
			plain = append(plain, p.parseSeq())
		}
	}
	if len(branches) > 0 {
		// Anything before the first label is the requirement for everyone
		// else: "<A&S reqs> Engineering students: <eng reqs>".
		return p.or(append([]*types.ReqNode{p.and(plain...)}, branches...)...)
	}
	return p.and(plain...)
}

func (p *parser) parseSeq() *types.ReqNode {
	left := p.parseAlt()
	last := left
	for {
		switch {
		case p.at(tComma, tCommaAnd, tAnd):
			p.next()
			last = p.parseAlt()
			left = p.and(left, last)
		case p.at(tCommaOr) && p.noOr == 0:
			p.next()
			last = p.parseAlt()
			left = p.or(left, last)
		case p.at(tExclude): // completion: "..., excluding JPH311H1, JPH441H1"
			t := p.next()
			if !applyExclude(last, t.codes) {
				left = p.and(left, p.text("", t))
			}
		case p.at(tGradeSuffix):
			// "A + B with a minimum grade of 68% in each course" covers the
			// whole list; without "each" only the item just before it.
			t := p.next()
			target := last
			if t.each {
				target = left
			}
			left = p.and(left, p.applyGradeSuffix(target, t))
		default:
			return left
		}
	}
}

// applyGradeSuffix sets a trailing grade qualifier on n with the
// applyMinGrade rules. If there is nothing to attach it to, the qualifier is
// returned as text.
func (p *parser) applyGradeSuffix(n *types.ReqNode, t token) *types.ReqNode {
	if n != nil && n.Type == types.NodeCredits {
		if n.MinGrade == 0 {
			n.MinGrade = t.grade
			return nil
		}
	} else if applyMinGrade(n, t.grade) > 0 {
		return nil
	}
	return p.text("", t)
}

func (p *parser) parseAlt() *types.ReqNode {
	left := p.parseUnit()
	for p.at(tSlash) || p.at(tOr) && p.noOr == 0 {
		p.next()
		left = p.or(left, p.parseUnit())
	}
	return left
}

func (p *parser) parseUnit() *types.ReqNode {
	nodes := []*types.ReqNode{p.parseAtom()}
	for p.atAtomStart() || p.at(tGradeSuffix) && !p.peek(0).each || p.at(tAvgSuffix) {
		last := nodes[len(nodes)-1]
		if p.at(tAvgSuffix) { // "( A, B) (average of both must be 80%)"
			nodes = append(nodes, p.applyAvgSuffix(last, p.next()))
			continue
		}
		if p.at(tGradeSuffix) {
			// A grade after something other than a course or a group
			// ("MAT (55%)"): it belongs to that item only.
			t := p.next()
			if last != nil && last.Type == types.NodeText && last.Kind == "" {
				p.extendText(last, t)
			} else {
				nodes = append(nodes, p.applyGradeSuffix(last, t))
			}
			continue
		}
		// Unclassified TEXT followed directly by COURSE/TEXT: the course is
		// mentioned, not required ("placement in FSL220H1 based on ..."), so
		// grow the text node instead.
		// (completion: a wildcard that reads as text, "UTM" in "taught at UTM").
		if last != nil && last.Type == types.NodeText && last.Kind == "" && (p.at(tCourse, tText) || p.atWildText()) {
			p.extendText(last, p.next())
			continue
		}
		nodes = append(nodes, p.parseAtom())
	}
	return p.and(nodes...)
}

func (p *parser) parseAtom() *types.ReqNode {
	if p.at(tRParen, tEOF) {
		return nil // leave ")" for the group that owns it
	}
	t := p.next()
	switch t.kind {
	case tCourse, tCredits, tGPA:
		return p.leaf(t)
	case tText:
		return p.text("", t) // kind is assigned in post-processing
	case tCond:
		return p.text(types.TextMembership, t)
	case tLParen:
		noOr := p.noOr
		p.noOr = 0 // completion: "Either (A or B) or C"
		n := p.parseField()
		p.noOr = noOr
		if p.at(tRParen) { // tolerate a missing ")"
			p.next()
			if p.at(tGradeSuffix) { // "( A, B) with a minimum grade of 68% in each"
				return p.and(n, p.applyGradeSuffix(n, p.next()))
			}
		}
		return n
	case tGradeSuffix:
		return p.text("", t) // nothing before it to attach to
	case tAvgSuffix:
		return p.avgText(t)
	case tGradePrefix:
		var n *types.ReqNode
		if t.each {
			n = p.parseSeq() // "Minimum 70% in required courses: A, B"
		} else {
			n = p.parseAlt() // "minimum of 77% in A/ B"
		}
		if n != nil && n.Type == types.NodeCredits && n.MinGrade == 0 {
			n.MinGrade = t.grade // "at least 68% in 0.5 credit in IRE ..."
			return n
		}
		if applyMinGrade(n, t.grade) == 0 {
			// "70% or higher in introductory statistics": nothing to attach
			// the grade to, so keep the whole phrase as text.
			return p.textSpan("", t.start, p.lastEnd)
		}
		return n
	case tPool:
		return p.parsePool(t)
	case tWild: // completion
		// Outside a pool only one course is meant: "any ETH200+". "PSL400-series
		// courses" doesn't say how many, and a bare designator is a word.
		if wildSingleRe.MatchString(p.src[t.start:t.end]) {
			return p.leaf(t)
		}
		return p.text("", t)
	case tExclude: // completion: nothing before it to apply to
		return p.text("", t)
	case tEither: // completion: "Either A, B or C, D"
		p.noOr++
		n := p.parseSeq()
		p.noOr--
		if p.at(tOr, tCommaOr) {
			p.next()
			return p.or(n, p.parseSeq())
		}
		return n
	default:
		return nil // stray operator ("..., and including ..."); constructors drop nils
	}
}

// parsePool parses the list after a pool phrase ("one of", "two of",
// "1.0 credit from:"). If the list uses commas, commas separate items and "/"
// joins alternatives within an item ("two of A/ B, C, D"); otherwise "/" and
// "or" separate items ("two of A/ B/ C"). As in English, a plain "," after an
// ", or" item ends the list ("A, B, or CIN courses, (C/ D)"). "CIN courses" is
// accepted as an item meaning any course with that designator, and a trailing
// ", or <text>" ("..., or another ethics course with prior approval") becomes an
// alternative to the whole pool.
func (p *parser) parsePool(t token) *types.ReqNode {
	ti := p.pos - 1 // t was just read
	n := p.parsePoolList(t, ti)
	if p.at(tExclude) && applyExclude(n, p.peek(0).codes) { // completion: "..., excluding A, B"
		p.next()
	}
	if p.at(tOr, tCommaOr) && p.noOr == 0 && p.peek(1).kind == tText && !p.poolItemAt(p.pos+1) {
		p.next()
		return p.or(n, p.parseAtom())
	}
	return n
}

// parsePoolList parses the items of the pool that token t (at index ti)
// starts and builds its node.
func (p *parser) parsePoolList(t token, ti int) *types.ReqNode {
	seps := p.poolSeps()
	var items []*types.ReqNode
	var desigs []string
	closing := false // after ", or B": a further "," ends the list
	alts := false    // completion: an item is alternatives joined by "/" ("BIO230H1/ BIO255H1")
	var groups []*types.ReqNode
	for {
		item, closes := p.poolAlternatives(seps)
		closing = closing || closes
		alts = alts || item != nil && item.Type == types.NodeOr && !closes
		if item != nil && item.Type == types.NodeOr && !closes {
			groups = append(groups, item) // "CSC108H1/ CSC148H1" (completion: pairCaps)
		}
		if d := designatorItem(item); d != nil {
			desigs = append(desigs, d...)
		} else if item != nil {
			items = append(items, item)
		}
		if !p.at(seps...) || !p.poolItemAt(p.pos+1) || closing && p.at(tComma, tSemi) {
			if last := p.lastAndItem(seps, closing); last != nil {
				items = append(items, last)
			}
			break
		}
		closing = p.next().kind == tCommaOr
	}

	if n := p.completionPool(t, ti, items, desigs, seps, alts, groups); n != nil {
		return n
	}
	switch {
	case !slices.ContainsFunc(items, hasCourse) && desigs == nil && !p.hasFilters(items):
		return p.textSpan("", t.start, p.lastEnd)
	case t.amount > 0 && allCourses(items): // "1.0 credit from: A, B, C"
		n := &types.ReqNode{Type: types.NodeCredits, Amount: t.amount, Designators: desigs}
		for _, it := range items {
			n.Courses = append(n.Courses, courseCodes(it)...)
		}
		p.spans[n] = span{t.start, p.lastEnd}
		return n
	case t.amount > 0 && t.amount <= 0.5 && desigs == nil: // a single half-course from a mixed pool = pick one
		return p.or(items...)
	case t.amount > 0: // a bigger amount from a mixed pool isn't expressible
		return p.textSpan("", t.start, p.lastEnd)
	}

	if desigs != nil {
		n := &types.ReqNode{Type: types.NodeCredits, Amount: 0.5, Designators: desigs}
		p.spans[n] = span{t.start, p.lastEnd}
		items = append(items, n)
	}
	if t.count <= 1 {
		return p.or(items...)
	}
	if t.count >= len(items) { // "two of A, B" means both
		return p.and(items...)
	}
	n := &types.ReqNode{Type: types.NodeChoose, Count: t.count, Items: items}
	p.spans[n] = span{t.start, p.lastEnd}
	return n
}

// poolSeps returns what separates pool items: "/" and "or", or, when the
// list uses commas, "," and ", or" (completion: and ";", as in "2.0 credits
// from: SLA117H1; SLA118H1").
func (p *parser) poolSeps() []tokKind {
	switch {
	case !p.poolUsesCommas():
		return []tokKind{tSlash, tOr}
	case p.completion:
		return []tokKind{tComma, tCommaOr, tSemi}
	}
	return []tokKind{tComma, tCommaOr}
}

// listSep reports whether k separates the items of a comma list (completion:
// ";" too).
func (p *parser) listSep(k tokKind) bool {
	return k == tComma || k == tCommaOr || k == tSemi && p.completion
}

// poolAlternatives parses one pool item with the alternatives that "/" and
// "or" join to it in a comma list ("two of A/ B, C"). closes reports a plain
// "or" there (completion: "A, B or C; D" ends the list after C, as ", or"
// does).
func (p *parser) poolAlternatives(seps []tokKind) (item *types.ReqNode, closes bool) {
	item = p.parsePoolItem()
	for !slices.Contains(seps, tSlash) && (p.at(tSlash) || p.at(tOr) && p.noOr == 0) && p.poolItemAt(p.pos+1) {
		or := p.next().kind == tOr
		closes = closes || or && p.completion
		item = p.or(item, p.parsePoolItem())
	}
	return item, closes
}

// lastAndItem parses "and D" after a comma list that hasn't closed, when D is
// its last course (completion: "HMU111H1, ..., TMU230H1 and TMU231H1"), or
// returns nil.
func (p *parser) lastAndItem(seps []tokKind, closing bool) *types.ReqNode {
	if !p.completion || closing || !slices.Contains(seps, tComma) || !p.atLastAndCourse() {
		return nil
	}
	p.next()
	return p.parsePoolItem()
}

// atLastAndCourse reports whether the tokens ahead are "and" and a course
// that ends the list.
func (p *parser) atLastAndCourse() bool {
	if !p.at(tAnd, tCommaAnd) || p.peek(1).kind != tCourse {
		return false
	}
	switch after := p.peek(2); after.kind {
	case tEOF, tSemi, tRParen, tExclude:
		return true
	case tText:
		return strings.HasPrefix(p.src[after.start:after.end], ".")
	}
	return false
}

// poolUsesCommas looks ahead through the pool list and reports whether
// commas separate its items.
func (p *parser) poolUsesCommas() bool {
	depth := 0
	for i := p.pos; i < len(p.toks); i++ {
		switch k := p.toks[i].kind; {
		case k == tLParen:
			depth++
		case k == tRParen:
			if depth == 0 {
				return false
			}
			depth--
		case depth > 0 || k == tCourse || k == tSlash || k == tOr || p.completionItemAt(i):
		case p.listSep(k):
			return p.poolItemAt(i + 1)
		default:
			return false
		}
	}
	return false
}

// poolItemAt reports whether the token at i can start a pool item.
func (p *parser) poolItemAt(i int) bool {
	if i >= len(p.toks) {
		return false
	}
	t := p.toks[i]
	return t.kind == tCourse || t.kind == tLParen ||
		t.kind == tText && desigItemRe.MatchString(p.src[t.start:t.end]) ||
		p.completionItemAt(i)
}

// completionItemAt reports whether the token at i is a pool member that only
// completion text has: a wildcard ("PSL400-series courses") or one course by
// filter ("any 400-level STA course"), followed by the end of the item. "any
// POL course with European content approved by ..." is not one.
func (p *parser) completionItemAt(i int) bool {
	t := p.toks[i]
	if !p.completion || !(t.kind == tWild || t.kind == tCredits && isFilter(t.node) && anyPoolItemRe.MatchString(p.src[t.start:t.end])) {
		return false
	}
	return i+1 >= len(p.toks) || slices.Contains(memberEnds, p.toks[i+1].kind)
}

// memberEnds are the tokens that may follow a completion pool member.
var memberEnds = []tokKind{tComma, tCommaOr, tCommaAnd, tSemi, tSlash, tOr, tAnd, tRParen, tExclude}

// hasFilters reports whether a pool has filter members (completion).
func (p *parser) hasFilters(items []*types.ReqNode) bool {
	return p.completion && slices.ContainsFunc(items, isFilter)
}

// completionPool builds the completion forms of an amount pool, or returns
// nil (always, for course text):
//   - a level floor after "including" ("1.0 credit, including at least 0.5
//     credit at the 400-level from A/ B") is text;
//   - courses and filters: unionPool;
//   - groups whose credits add up to the amount: groupPool, which a list
//     that names no pool ("2.0 credits: A; B/ C; D") or has alternatives
//     in a comma list tries first.
func (p *parser) completionPool(t token, ti int, items []*types.ReqNode, desigs []string, seps []tokKind, alts bool, groups []*types.ReqNode) *types.ReqNode {
	if !p.completion || t.amount == 0 {
		return nil
	}
	if t.node != nil && ti > 0 && includingRe.MatchString(p.src[p.toks[ti-1].start:p.toks[ti-1].end]) {
		return p.textSpan("", t.start, p.lastEnd)
	}
	// Alternatives in a list that adds up to the amount ("2.0 credits from
	// BCH210H1; BIO230H1/ BIO255H1; PSL300H1, PSL301H1") are each needed, one
	// way or the other; read as a union, both alternatives could count.
	if t.sum || alts && !slices.Contains(seps, tSlash) {
		if n := p.groupPool(t, items, desigs, seps); n != nil {
			return n
		}
	}
	if n := p.unionPool(t, items, desigs); n != nil {
		return p.pairCaps(n, groups, seps)
	}
	return p.groupPool(t, items, desigs, seps)
}

// pairCaps adds to a union pool a cap for each group of alternatives it
// flattened (design 3.2): in "1.0 credit from: CHM135H1, CHM136H1, CSC108H1/
// CSC148H1/ EEB125H1, JEG100H1" at most one of CSC108H1, CSC148H1 and
// EEB125H1 counts, so and(pool, credits{courses: [the group], max_amount:
// the heaviest}). The completion assembler makes each cap a limit row on
// the item. A "/" list ("A/ B/ C") is all alternatives: no caps; nor is an
// "or" that closes a comma list ("LIN228H1, LIN232H1 or LIN241H1").
func (p *parser) pairCaps(n *types.ReqNode, groups []*types.ReqNode, seps []tokKind) *types.ReqNode {
	if n.Type != types.NodeCredits || slices.Contains(seps, tSlash) {
		return n
	}
	caps := []*types.ReqNode{n}
	for _, it := range groups {
		if codes := courseChoiceCodes(it); len(codes) >= 2 {
			caps = append(caps, courseCap(codes))
		}
	}
	if len(caps) == 1 {
		return n
	}
	return p.and(caps...)
}

// courseCap caps courses at their heaviest: credits{courses, max_amount},
// so that at most one of them counts (a "/" pair flattened into a pool,
// pairCaps and bracketPairCaps; "No more than one from GGR273H1, GGR373H1",
// capRow). The courses are deduplicated.
func courseCap(codes []string) *types.ReqNode {
	return &types.ReqNode{Type: types.NodeCredits, Courses: dedupe(codes), MaxAmount: maxAmount(heaviestCourse(codes))}
}

// heaviestCourse returns the largest weight among codes (courseWeight), 0
// for none.
func heaviestCourse(codes []string) float64 {
	w := 0.0
	for _, c := range codes {
		w = max(w, courseWeight(c))
	}
	return w
}

// courseChoiceCodes returns the codes of an "or" of courses, deduplicated,
// else nil. courseChoice (completion) also takes a single course.
func courseChoiceCodes(n *types.ReqNode) []string {
	if n == nil || n.Type != types.NodeOr {
		return nil
	}
	var codes []string
	for _, it := range n.Items {
		if it.Type != types.NodeCourse {
			return nil
		}
		codes = append(codes, it.Code)
	}
	return dedupe(codes)
}

// includingRe matches the token "including" or ", including".
var includingRe = regexp.MustCompile(`^,?\s*(?i:including)$`)

// atWildText reports whether the next token is a tWild that parseAtom makes
// text: "UTM" in "Courses taught at UTM or UTSC".
func (p *parser) atWildText() bool {
	t := p.peek(0)
	return t.kind == tWild && !wildSingleRe.MatchString(p.src[t.start:t.end])
}

// parsePoolItem parses one pool item. A tWild member is kept as a filter
// here (parseAtom would make the plural forms text).
func (p *parser) parsePoolItem() *types.ReqNode {
	if p.at(tWild) {
		return p.leaf(p.next())
	}
	return p.parseAtom()
}

// desigItemRe matches a pool item naming a designator: "CIN courses".
var desigItemRe = regexp.MustCompile(`^(?i:any\s+)?([A-Z]{3}(?:\s*/\s*[A-Z]{3})*)\s+(?i:courses?)$`)

// designatorItem returns the designators of a "CIN courses" item, else nil.
func designatorItem(n *types.ReqNode) []string {
	if n == nil || n.Type != types.NodeText {
		return nil
	}
	m := desigItemRe.FindStringSubmatch(n.Text)
	if m == nil {
		return nil
	}
	return desigRe.FindAllString(m[1], -1)
}

// unionPool builds the completion form of an amount pool whose items are
// listed courses and filters ("1.0 credit from APM346H1/ PHY 300-level
// courses/ PHY 400-level courses"). The amount applies to the union:
//   - when the filters combine into one without changing what matches, and
//     every listed code passes it, the result is one credits node:
//     credits{1.0, courses [APM346H1], designators [PHY], levels [300, 400]}
//     is exact because APM346H1 is at the 300 level;
//   - otherwise ("HMU200+ level courses; JMU210H1; SMU400-level courses") it
//     is credits{amount, items: [the listed courses, the merged filters]}: a
//     course counts if it matches any item, as in average. This is an
//     interim form: docs/schema.md writes such a union as a course list,
//     which the completion assembly is to build from these items.
//
// A pool level ("0.5 credit at the 400-level from ...", t.node) narrows
// every member; if that would drop a listed code or leave a member no level,
// the pool is text instead. It returns nil if an item is neither a course nor
// a filter.
func (p *parser) unionPool(t token, items []*types.ReqNode, desigs []string) *types.ReqNode {
	var codes []string
	var filters []*types.ReqNode
	for _, it := range items {
		c, f, ok := poolMembers(it)
		if !ok {
			return nil
		}
		codes, filters = append(codes, c...), append(filters, f...)
	}
	codes = dedupe(codes)
	if desigs != nil {
		filters = append(filters, &types.ReqNode{Type: types.NodeCredits, Designators: desigs})
	}
	if codes == nil && filters == nil {
		return nil
	}
	n := &types.ReqNode{Type: types.NodeCredits, Amount: t.amount}
	f := mergeFilters(filters)
	if f != nil && !slices.ContainsFunc(codes, func(c string) bool { return !levelMatches(f.Levels, c) || slices.Contains(f.Exclude, c) }) {
		n.Designators, n.Levels, n.Exclude, n.Courses = f.Designators, f.Levels, f.Exclude, codes
	} else {
		if codes != nil {
			n.Items = append(n.Items, &types.ReqNode{Type: types.NodeCredits, Courses: codes})
		}
		n.Items = append(n.Items, groupFilters(filters)...)
	}
	if t.node != nil { // "0.5 credit at the 400-level from ..."
		// The level must not silently drop a listed course or a member.
		if slices.ContainsFunc(codes, func(c string) bool { return !levelMatches(t.node.Levels, c) }) {
			return p.textSpan("", t.start, p.lastEnd)
		}
		for _, it := range append([]*types.ReqNode{n}, n.Items...) {
			if narrowLevels(it, t.node.Levels) {
				return p.textSpan("", t.start, p.lastEnd)
			}
		}
	}
	p.spans[n] = span{t.start, p.lastEnd}
	return n
}

// narrowLevels restricts a credits filter to levels as well. It reports
// whether that leaves the filter no level at all. Listed courses (checked by
// the caller) and unions are left alone.
func narrowLevels(n *types.ReqNode, levels []int) (empty bool) {
	if n.Items != nil || n.Designators == nil && n.Levels == nil {
		return false
	}
	if n.Levels == nil {
		n.Levels = slices.Clone(levels)
		return false
	}
	n.Levels = slices.DeleteFunc(slices.Clone(n.Levels), func(l int) bool { return !slices.Contains(levels, l) })
	return len(n.Levels) == 0
}

// groupPool reads an amount pool of groups, using their smallest credits
// (minCredits), or returns nil:
//   - in a comma or ";" list, groups that add up to the amount are all
//     needed: "2.0 credits from ( BIO120H1, BIO130H1)/ BIO150Y1; ( CHM135H1,
//     CHM136H1)/ CHM151Y1" needs one of each pair. A group of alternatives
//     must weigh the same either way: in "1.0 credit from ECO101H1, ECO102H1/
//     ECO105Y1" (ASMAJ2660) ECO105Y1 alone is the amount, so the "and" would
//     reject a student who took only it. With a range ("2.5 to 3.0 credits
//     from STA130H1, CSC108H1/ CSC110Y1, ...") the groups' smallest credits
//     must add up to its lower end and their largest to at most its upper
//     end: the range is what the uneven alternatives allow. "0.5 to 1.0
//     credit from STA492H1, STA496H1/ ... STA499Y1" adds up to 1.0, so it is
//     not "both" (ASSPE2290);
//   - in a "/" list the items are alternatives, never added up: when each
//     one is the amount (or within the range), the pool is their "or":
//     "1.0 credit from ( MAT235H1, MAT236H1)/ MAT235Y1/ MAT237Y1/ MAT257Y1".
func (p *parser) groupPool(t token, items []*types.ReqNode, desigs []string, seps []tokKind) *types.ReqNode {
	if desigs != nil || len(items) == 0 {
		return nil
	}
	alternatives := slices.Contains(seps, tSlash)
	sum, most := 0.0, 0.0
	uneven := false
	for _, it := range items {
		c, ok := minCredits(it)
		if !ok || alternatives && !inRange(c, t) {
			return nil
		}
		sum += c
		most += maxCredits(it)
		uneven = uneven || !evenTree(it)
	}
	switch {
	case alternatives:
		return p.or(items...)
	case math.Abs(sum-t.amount) > 1e-9:
		return nil
	case uneven && !t.sum && (t.upper == 0 || most > t.upper+1e-9):
		// "N credits:" (t.sum) lists the courses that make up N: their
		// lightest routes add up to it ("1.0 credit: AST320H1, AST325H1/
		// AST326Y1").
		return nil
	}
	return p.and(items...)
}

// maxCredits returns the most credits a route through n takes: the
// heaviest alternative of each "or" (minCredits has checked the courses). It
// is minCredits' upper end, for the same trees of courses, "and" and "or";
// anything else counts 0.
func maxCredits(n *types.ReqNode) float64 {
	switch n.Type {
	case types.NodeCourse:
		return courseWeight(n.Code)
	case types.NodeAnd, types.NodeOr:
		c := 0.0
		for _, it := range n.Items {
			x := maxCredits(it)
			switch {
			case n.Type == types.NodeAnd:
				c += x
			case x > c:
				c = x
			}
		}
		return c
	}
	return 0
}

// evenTree reports whether every "or" in n has alternatives of the same
// credits.
func evenTree(n *types.ReqNode) bool {
	even := true
	walkReq(n, func(x *types.ReqNode) {
		even = even && (x.Type != types.NodeOr || evenCredits(x))
	})
	return even
}

// inRange reports whether credits c are the pool's amount, or within its
// range ("2.0 to 2.5 credits from").
func inRange(c float64, t token) bool {
	if t.upper > 0 {
		return c > t.amount-1e-9 && c < t.upper+1e-9
	}
	return math.Abs(c-t.amount) < 1e-9
}

// minCredits returns the fewest credits that meet n, from the H/Y weights of
// its courses. ok is false if n has anything but courses, "and" and "or", or
// if an "and" holds an "or" whose alternatives differ in credits: in
// "ECO101H1, ECO102H1/ ECO105Y1" (ASMAJ2660) the Y course replaces the pair,
// which "/" binding tighter than "," misreads, so the sum would be wrong
// (groupPairY rewrites only the unambiguous forms).
//
// Unlike minWeight (enrolment) and nodeCredits (completion), which also
// count pools, text and "choose", it reads only courses with a valid code,
// "and" and "or": the grammar uses it to check that the courses of a list
// add up to its amount, where anything else means the list isn't one.
func minCredits(n *types.ReqNode) (c float64, ok bool) {
	switch n.Type {
	case types.NodeCourse:
		if !codeRe.MatchString(n.Code) || len(n.Code) != 8 {
			return 0, false
		}
		return courseWeight(n.Code), true
	case types.NodeAnd, types.NodeOr:
		for i, it := range n.Items {
			x, ok := minCredits(it)
			if !ok || n.Type == types.NodeAnd && it.Type == types.NodeOr && !evenCredits(it) {
				return 0, false
			}
			switch {
			case n.Type == types.NodeAnd:
				c += x
			case i == 0 || x < c:
				c = x
			}
		}
		return c, true
	}
	return 0, false
}

// evenCredits reports whether every alternative of an "or" needs the same
// credits.
func evenCredits(n *types.ReqNode) bool {
	first, _ := minCredits(n.Items[0])
	return !slices.ContainsFunc(n.Items[1:], func(it *types.ReqNode) bool {
		x, _ := minCredits(it)
		return math.Abs(x-first) > 1e-9
	})
}

// applyExclude adds codes to the exclusions of the credits pools in n
// (through "or"), and reports whether there was one.
func applyExclude(n *types.ReqNode, codes []string) bool {
	if n == nil {
		return false
	}
	switch n.Type {
	case types.NodeCredits:
		if n.Items == nil {
			n.Exclude = dedupe(append(slices.Clone(n.Exclude), codes...))
		}
		for _, it := range n.Items {
			if it.Courses == nil {
				it.Exclude = dedupe(append(slices.Clone(it.Exclude), codes...))
			}
		}
		return true
	case types.NodeOr:
		applied := false
		for _, it := range n.Items {
			applied = applyExclude(it, codes) || applied
		}
		return applied
	}
	return false
}

// poolMembers splits a pool item into listed codes and filters (copies
// without an amount). An "or" of members is a union too. ok is false for
// anything else.
func poolMembers(n *types.ReqNode) (codes []string, filters []*types.ReqNode, ok bool) {
	switch {
	case n == nil:
		return nil, nil, true
	case n.Type == types.NodeCourse:
		return []string{n.Code}, nil, true
	case isFilter(n):
		f := *n
		f.Amount = 0
		return nil, []*types.ReqNode{&f}, true
	case n.Type == types.NodeOr:
		for _, it := range n.Items {
			c, f, ok := poolMembers(it)
			if !ok {
				return nil, nil, false
			}
			codes, filters = append(codes, c...), append(filters, f...)
		}
		return codes, filters, true
	}
	return nil, nil, false
}

// isFilter reports whether n is one course matching designators and levels
// ("any 300-level CSC course", "PSL400-series courses"), which a pool reads
// as a filter.
func isFilter(n *types.ReqNode) bool {
	return n != nil && n.Type == types.NodeCredits && n.Amount <= 0.5 && n.MaxAmount == nil &&
		n.Courses == nil && n.Items == nil && n.Breadth == nil && n.MinGrade == 0 &&
		(n.Designators != nil || n.Levels != nil)
}

// mergeFilters combines filters into one that matches exactly the courses
// any of them matches, or returns nil. Filters combine when they share their
// levels (the designators are joined) or their designators (the levels are
// joined). Excluded codes are joined when no other filter matches them.
func mergeFilters(filters []*types.ReqNode) *types.ReqNode {
	m := &types.ReqNode{Type: types.NodeCredits}
	if len(filters) <= 1 {
		if len(filters) == 1 {
			return filters[0]
		}
		return m
	}
	sameLevels, sameDesigs := true, true
	for _, f := range filters[1:] {
		sameLevels = sameLevels && sameSet(f.Levels, filters[0].Levels)
		sameDesigs = sameDesigs && sameSet(f.Designators, filters[0].Designators)
	}
	switch {
	case sameLevels:
		m.Levels = filters[0].Levels
		for _, f := range filters {
			if f.Designators == nil { // any designator
				m.Designators = nil
				break
			}
			m.Designators = append(m.Designators, f.Designators...)
		}
	case sameDesigs:
		m.Designators = filters[0].Designators
		for _, f := range filters {
			if f.Levels == nil { // any level
				m.Levels = nil
				break
			}
			m.Levels = append(m.Levels, f.Levels...)
		}
		slices.Sort(m.Levels)
	default:
		return nil
	}
	m.Designators, m.Levels = dedupe(m.Designators), dedupe(m.Levels)
	for i, f := range filters {
		for _, x := range f.Exclude {
			if slices.ContainsFunc(filters, func(g *types.ReqNode) bool {
				return g != filters[i] && filterMatches(g, x)
			}) {
				return nil
			}
			m.Exclude = append(m.Exclude, x)
		}
	}
	m.Exclude = dedupe(m.Exclude)
	return m
}

// groupFilters merges the filters that can be merged ("HMU200+ level
// courses; TMU200+ courses" is one filter), keeping their order.
func groupFilters(filters []*types.ReqNode) []*types.ReqNode {
	var out []*types.ReqNode
next:
	for _, f := range filters {
		for i, g := range out {
			if m := mergeFilters([]*types.ReqNode{g, f}); m != nil {
				out[i] = m
				continue next
			}
		}
		out = append(out, f)
	}
	return out
}

// filterMatches reports whether the course code matches filter f.
func filterMatches(f *types.ReqNode, code string) bool {
	return (f.Designators == nil || slices.Contains(f.Designators, code[:3])) &&
		levelMatches(f.Levels, code) && !slices.Contains(f.Exclude, code)
}

// levelMatches reports whether code's level is one of levels (any if none).
// Only St. George-style codes ("CSC148H1") have a level in their digits here.
func levelMatches(levels []int, code string) bool {
	if levels == nil {
		return true
	}
	return sgCodeRe.MatchString(code) && slices.Contains(levels, int(code[3]-'0')*100)
}

var sgCodeRe = regexp.MustCompile(`^[A-Z]{3}[1-4]\d{2}[HY]\d$`)

func sameSet[T comparable](a, b []T) bool {
	return len(a) == len(b) && !slices.ContainsFunc(a, func(x T) bool { return !slices.Contains(b, x) })
}

// applyMinGrade sets MinGrade on every course under n that doesn't have one,
// and returns how many courses it touched. It skips credit pools ("Minimum 70%
// in required courses: A, B, 0.5 credit from ...") and groups that carry grades
// of their own ("minimum of 70% in A/ (minimum of 85% in B, C)").
func applyMinGrade(n *types.ReqNode, grade int) int {
	if n == nil {
		return 0
	}
	if n.Type == types.NodeCourse {
		if n.MinGrade == 0 {
			n.MinGrade = grade
			return 1
		}
		return 0
	}
	count := 0
	for _, it := range n.Items {
		if it.Type != types.NodeCourse && hasGrade(it) {
			continue
		}
		count += applyMinGrade(it, grade)
	}
	return count
}

func hasGrade(n *types.ReqNode) bool {
	return n.MinGrade > 0 || slices.ContainsFunc(n.Items, hasGrade)
}

func hasCourse(n *types.ReqNode) bool {
	return n.Type == types.NodeCourse || slices.ContainsFunc(n.Items, hasCourse)
}

// allCourses reports whether every item is a course, or an OR of courses.
func allCourses(items []*types.ReqNode) bool {
	return !slices.ContainsFunc(items, func(n *types.ReqNode) bool { return courseCodes(n) == nil })
}

// courseCodes returns the codes of a course or an OR of courses, else nil.
func courseCodes(n *types.ReqNode) []string {
	switch n.Type {
	case types.NodeCourse:
		return []string{n.Code}
	case types.NodeOr:
		var codes []string
		for _, it := range n.Items {
			if it.Type != types.NodeCourse {
				return nil
			}
			codes = append(codes, it.Code)
		}
		return codes
	}
	return nil
}

// ---- token helpers ----

func (p *parser) peek(i int) token {
	if p.pos+i < len(p.toks) {
		return p.toks[p.pos+i]
	}
	return token{kind: tEOF}
}

func (p *parser) at(kinds ...tokKind) bool { return slices.Contains(kinds, p.peek(0).kind) }

func (p *parser) next() token {
	t := p.peek(0)
	if t.kind != tEOF {
		p.pos++
		p.lastEnd = t.end
	}
	return t
}

func (p *parser) done() bool { return p.pos >= len(p.toks) }

func (p *parser) atAtomStart() bool {
	return p.at(tCourse, tCredits, tGPA, tText, tLParen, tGradePrefix, tPool, tWild, tEither)
}

// ---- node constructors ----

func (p *parser) leaf(t token) *types.ReqNode {
	p.spans[t.node] = span{t.start, t.end}
	for _, it := range t.node.Items { // prebuilt "credits with a B+ average"
		p.spans[it] = span{t.start, t.end}
	}
	return t.node
}

func (p *parser) text(kind types.TextKind, t token) *types.ReqNode {
	return p.textSpan(kind, t.start, t.end)
}

func (p *parser) textSpan(kind types.TextKind, start, end int) *types.ReqNode {
	n := &types.ReqNode{Type: types.NodeText, Kind: kind, Text: p.src[start:end]}
	p.spans[n] = span{start, end}
	return n
}

func (p *parser) extendText(n *types.ReqNode, t token) {
	s := p.spans[n]
	s.end = t.end
	p.spans[n] = s
	n.Text = p.src[s.start:s.end]
}

func (p *parser) and(items ...*types.ReqNode) *types.ReqNode { return p.combine(types.NodeAnd, items) }
func (p *parser) or(items ...*types.ReqNode) *types.ReqNode  { return p.combine(types.NodeOr, items) }

// combine builds an and/or node: nils are dropped, children of the same type
// are flattened into it, and a single child is returned as is.
func (p *parser) combine(t types.NodeType, items []*types.ReqNode) *types.ReqNode {
	var flat []*types.ReqNode
	for _, it := range items {
		switch {
		case it == nil:
		case it.Type == t:
			flat = append(flat, it.Items...)
		default:
			flat = append(flat, it)
		}
	}
	switch len(flat) {
	case 0:
		return nil
	case 1:
		return flat[0]
	}
	n := &types.ReqNode{Type: t, Items: flat}
	s := p.spans[flat[0]]
	for _, it := range flat[1:] {
		s.start = min(s.start, p.spans[it].start)
		s.end = max(s.end, p.spans[it].end)
	}
	p.spans[n] = s
	return n
}
