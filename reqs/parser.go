package reqs

import (
	"slices"

	"degree-explorer/types"
)

// Grammar, lowest precedence first:
//
//	field := segment { (';' | COND) segment }             AND; COND segments are OR'd together
//	seq   := alt { (',' | ', and' | 'and' | ', or') alt } AND (', or' → OR), left to right
//	alt   := unit { ('/' | 'or') unit }                   OR
//	unit  := atom { atom }                                adjacency → AND
//	atom  := COURSE | CREDITS | GPA | TEXT | COND
//	       | '(' field ')' | GRADE_PREFIX atom | POOL pool
//	pool  := atom { (',' | ', or' | '/' | 'or') atom }    every separator is OR

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
			n = p.parseSeq()
		} else {
			n = p.parseAtom()
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

func (p *parser) parsePool(t token) *types.ReqNode {
	items := []*types.ReqNode{p.parseAtom()}
	for p.at(tComma, tCommaOr, tSlash, tOr) && (p.peek(1).kind == tCourse || p.peek(1).kind == tLParen) {
		p.next()
		items = append(items, p.parseAtom())
	}
	items = slices.DeleteFunc(items, func(n *types.ReqNode) bool { return n == nil })

	switch {
	case !slices.ContainsFunc(items, hasCourse):
		return p.textSpan("", t.start, p.lastEnd)
	case t.amount == 0: // "one of"
		return p.or(items...)
	case allCourses(items): // "1.0 credit from: A, B, C"
		n := &types.ReqNode{Type: types.NodeCredits, Amount: t.amount}
		for _, it := range items {
			n.Courses = append(n.Courses, courseCodes(it)...)
		}
		p.spans[n] = span{t.start, p.lastEnd}
		return n
	case t.amount <= 0.5: // a single half-course from a mixed pool = pick one
		return p.or(items...)
	default: // a bigger amount from a mixed pool isn't expressible
		return p.textSpan("", t.start, p.lastEnd)
	}
}

// applyMinGrade sets MinGrade on every course and credits node under n that
// doesn't have one, and returns how many nodes it touched.
func applyMinGrade(n *types.ReqNode, grade int) int {
	if n == nil {
		return 0
	}
	count := 0
	if (n.Type == types.NodeCourse || n.Type == types.NodeCredits) && n.MinGrade == 0 {
		n.MinGrade = grade
		count++
	}
	for _, it := range n.Items {
		count += applyMinGrade(it, grade)
	}
	return count
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
