package reqs

import (
	"regexp"
	"slices"

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

type span struct{ start, end int }

type parser struct {
	src     string
	toks    []token
	pos     int
	lastEnd int
	spans   map[*types.ReqNode]span // side table, so ReqNode stays a clean JSON type
}

func newParser(src string, toks []token) *parser {
	return &parser{src: src, toks: toks, spans: map[*types.ReqNode]span{}}
}

// parseExpr lexes and parses src without post-processing.
func parseExpr(src string) *types.ReqNode {
	return newParser(src, lex(src)).parseAll()
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
			body := []*types.ReqNode{p.text(types.KindProgram, p.next())}
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
	for {
		switch {
		case p.at(tComma, tCommaAnd, tAnd):
			p.next()
			left = p.and(left, p.parseAlt())
		case p.at(tCommaOr):
			p.next()
			left = p.or(left, p.parseAlt())
		default:
			return left
		}
	}
}

func (p *parser) parseAlt() *types.ReqNode {
	left := p.parseUnit()
	for p.at(tSlash, tOr) {
		p.next()
		left = p.or(left, p.parseUnit())
	}
	return left
}

func (p *parser) parseUnit() *types.ReqNode {
	nodes := []*types.ReqNode{p.parseAtom()}
	for p.atAtomStart() {
		last := nodes[len(nodes)-1]
		// Unclassified TEXT followed directly by COURSE/TEXT: the course is
		// mentioned, not required ("placement in FSL220H1 based on …"), so
		// grow the text node instead.
		if last != nil && last.Type == types.NodeText && last.Kind == "" && p.at(tCourse, tText) {
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
		return p.text(types.KindProgram, t)
	case tLParen:
		n := p.parseField()
		if p.at(tRParen) { // tolerate a missing ")"
			p.next()
		}
		return n
	case tGradePrefix:
		var n *types.ReqNode
		if t.each {
			n = p.parseSeq() // "Minimum 70% in required courses: A, B"
		} else {
			n = p.parseAlt() // "minimum of 77% in A/ B"
		}
		if n != nil && n.Type == types.NodeCredits && n.MinGrade == 0 {
			n.MinGrade = t.grade // "at least 68% in 0.5 credit in IRE …"
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
	default:
		return nil // stray operator ("…, and including …"); constructors drop nils
	}
}

// parsePool parses the list after a pool phrase ("one of", "two of",
// "1.0 credit from:"). If the list uses commas, commas separate items and "/"
// joins alternatives within an item ("two of A/ B, C, D"); otherwise "/" and
// "or" separate items ("two of A/ B/ C"). As in English, a plain "," after an
// ", or" item ends the list ("A, B, or CIN courses, (C/ D)"). "CIN courses" is
// accepted as an item meaning any course with that designator, and a trailing
// ", or <text>" ("…, or another ethics course with prior approval") becomes an
// alternative to the whole pool.
func (p *parser) parsePool(t token) *types.ReqNode {
	n := p.parsePoolList(t)
	if p.at(tOr, tCommaOr) && p.peek(1).kind == tText && !p.poolItemAt(p.pos+1) {
		p.next()
		return p.or(n, p.parseAtom())
	}
	return n
}

func (p *parser) parsePoolList(t token) *types.ReqNode {
	itemSeps := []tokKind{tSlash, tOr}
	if p.poolUsesCommas() {
		itemSeps = []tokKind{tComma, tCommaOr}
	}
	var items []*types.ReqNode
	var desigs []string
	afterCommaOr := false
	for {
		item := p.parseAtom()
		for !slices.Contains(itemSeps, tSlash) && p.at(tSlash, tOr) && p.poolItemAt(p.pos+1) {
			p.next()
			item = p.or(item, p.parseAtom())
		}
		if d := designatorItem(item); d != nil {
			desigs = append(desigs, d...)
		} else if item != nil {
			items = append(items, item)
		}
		if !p.at(itemSeps...) || !p.poolItemAt(p.pos+1) || afterCommaOr && p.at(tComma) {
			break
		}
		afterCommaOr = p.next().kind == tCommaOr
	}

	switch {
	case !slices.ContainsFunc(items, hasCourse) && desigs == nil:
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
		case depth > 0 || k == tCourse || k == tSlash || k == tOr:
		case k == tComma || k == tCommaOr:
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
		t.kind == tText && desigItemRe.MatchString(p.src[t.start:t.end])
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

// applyMinGrade sets MinGrade on every course under n that doesn't have one,
// and returns how many courses it touched. It skips credit pools ("Minimum 70%
// in required courses: A, B, 0.5 credit from …") and groups that carry grades
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
	return p.at(tCourse, tCredits, tGPA, tText, tLParen, tGradePrefix, tPool)
}

// ---- node constructors ----

func (p *parser) leaf(t token) *types.ReqNode {
	p.spans[t.node] = span{t.start, t.end}
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
