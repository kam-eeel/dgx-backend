package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Floors: "To ensure that students admitted to the program will be
// successful, applicants with a final grade lower than 60% in each course,
// or a grade average lower than 70% in these required courses will not be
// considered for admission." A grade floor sets min_grade on the courses in
// scope; an average floor adds an average node over them. Falling below a
// floor is NotMet even when the cutoff itself is variable.
//
// Scope: the courses the floor names ("lower than 65% in BIO130H1", "a final
// grade average lower than 70% in ( CHM135H1, CHM136H1)"), else the
// requirement lists of the floor's section, else of its audience. Each
// audience gets its own average: "these required courses" are the ones on
// the student's route.

var (
	floorSentenceRe = regexp.MustCompile(`(?i)\bwill\s+not\s+(?:usually\s+)?be\s+considered\b|\bis\s+needed\s+for\s+admission\s+consideration\b`)
	// One floor: "lower than 70%", "below 70%", "lower than a B". What comes
	// before it, back to the previous floor, says whether it is an average.
	floorRe = regexp.MustCompile(`(?i)\b(?:lower\s+than|below)\s+(?:(\d{2})\s*%|an?\s+([A-D][+-]?)\b)`)
	// "An average of at least 73% in the prerequisite courses is needed for
	// admission consideration"
	floorAtLeastRe = regexp.MustCompile(`(?i)\ban\s+average\s+of\s+at\s+least\s+(\d{2})%`)
	// The courses a floor names, right after it: "in BIO130H1", "in either
	// BIO220H1 or ( BIO230H1/ BIO255H1)", "in ( CHM135H1, CHM136H1)", "in
	// CSC110Y1, STA130H1, or STA261H1". It ends where the next floor starts.
	floorTargetRe = regexp.MustCompile(`^\s*in\s+(?:either\s+)?((?:[\s(),/]|` + codePat + `|\bor\b|\band\b)+)`)
	// "..., or in the average of MAT148H1 or MAT158H1 and MAT149H1 or MAT159H1"
	floorAvgOfRe   = regexp.MustCompile(`(?i)^\s*,?\s*(?:or\s+)?in\s+the\s+average\s+of\s+((?:[\s(),/]|` + codePat + `|\bor\b|\band\b)+)`)
	floorAvgWordRe = regexp.MustCompile(`(?i)\baverages?\b`)
	// "a final grade or grade average lower than ...": a grade floor, which
	// also bounds the average.
	floorEitherRe = regexp.MustCompile(`(?i)\bgrade\s+(?:or|and)\s+(?:grade\s+)?averages?\b`)
)

var (
	// "applicants with a final grade or grade average lower than the minimum
	// grades stated above": each grade, or the average, may be the floor.
	floorOrAverageRe = regexp.MustCompile(`(?i)\bgrade\s+or\s+(?:grade\s+)?averages?\s+lower\s+than\b`)
	// "will not usually be considered"
	notUsuallyRe = regexp.MustCompile(`(?i)\bnot\s+usually\b`)
)

// averageWhy is the lenient reading of "a final grade or grade average lower
// than ...".
const averageWhy = "the minimum may apply to the average of these courses rather than to each"

// averageReading adds the lenient reading of "a final grade or grade average
// lower than ...": where several courses share one minimum, their average
// may be what has to reach it. A 1.0-credit pool with a 63% minimum becomes
// ambiguous(pool, and(pool without the minimum, an average of 63)); two
// required courses at 73 become ambiguous(both at 73, both with an average
// of 73). Courses with different minimums keep them.
func (p *enrolParser) averageReading(scope map[*audience][]*part) {
	for a, parts := range scope {
		var singles []*part // parts that are graded courses: "- JAV120H1 and JAV130H1."
		var courses []*types.ReqNode
		for _, pt := range parts {
			if cs := gradedCourses(pt.node); cs != nil {
				singles = append(singles, pt)
				courses = append(courses, cs...)
				continue
			}
			pt.node = replaceNodes(pt.node, func(n *types.ReqNode) bool {
				return n.Type == types.NodeCredits && n.MinGrade > 0 && n.Amount >= 1
			}, func(n *types.ReqNode) *types.ReqNode {
				plain := cloneNode(n)
				plain.MinGrade = 0
				avg := &types.ReqNode{Type: types.NodeAverage, MinGrade: n.MinGrade, Amount: n.Amount, Courses: n.Courses,
					Designators: n.Designators, Levels: n.Levels, Breadth: n.Breadth, Exclude: n.Exclude}
				return ambiguous(n, andNode(plain, avg), averageWhy)
			})
		}
		if len(courses) < 2 || slices.ContainsFunc(courses, func(c *types.ReqNode) bool { return c.MinGrade != courses[0].MinGrade }) {
			continue
		}
		var strict, plain []*types.ReqNode
		var codes []string
		for _, c := range courses {
			strict = append(strict, c)
			plain = append(plain, &types.ReqNode{Type: types.NodeCourse, Code: c.Code})
			codes = append(codes, c.Code)
		}
		avg := &types.ReqNode{Type: types.NodeAverage, MinGrade: courses[0].MinGrade, Courses: codes}
		combined := ambiguous(andNode(strict...), andNode(append(plain, avg)...), averageWhy)
		a.parts = slices.DeleteFunc(a.parts, func(pt *part) bool { return slices.Contains(singles, pt) })
		p.addPart(a, combined, partList)
	}
}

// gradedCourses returns the courses of n when n is a course with a minimum
// grade or an and of them, else nil.
func gradedCourses(n *types.ReqNode) []*types.ReqNode {
	switch n.Type {
	case types.NodeCourse:
		if n.MinGrade > 0 {
			return []*types.ReqNode{n}
		}
	case types.NodeAnd:
		for _, it := range n.Items {
			if it.Type != types.NodeCourse || it.MinGrade == 0 {
				return nil
			}
		}
		return n.Items
	}
	return nil
}

type floor struct {
	grade   int
	average bool
	codes   []string // the courses named; empty means the scope
}

// parseFloors reads the floors in a floor sentence. It returns nil for a
// sentence that only refers to the grades already stated ("lower than the
// minimum grades stated above").
func parseFloors(s string) []floor {
	var out []floor
	if m := floorAtLeastRe.FindStringSubmatch(s); m != nil && !floorRe.MatchString(s) {
		g, _ := strconv.Atoi(m[1])
		return []floor{{grade: g, average: true}}
	}
	locs := floorRe.FindAllStringSubmatchIndex(s, -1)
	prev := 0
	for i, m := range locs {
		g := 0
		if m[2] >= 0 {
			g, _ = strconv.Atoi(s[m[2]:m[3]])
		} else {
			g = letterPercent[strings.ToUpper(s[m[4]:m[5]])]
		}
		before := s[prev:m[0]]
		f := floor{grade: g, average: floorAvgWordRe.MatchString(before) && !floorEitherRe.MatchString(before)}
		end := len(s)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		rest := s[m[1]:end]
		if t := floorTargetRe.FindStringSubmatch(rest); t != nil {
			f.codes = codeRe.FindAllString(t[1], -1)
			rest = rest[len(t[0]):]
		}
		out = append(out, f)
		if a := floorAvgOfRe.FindStringSubmatch(rest); a != nil {
			out = append(out, floor{grade: g, average: true, codes: codeRe.FindAllString(a[1], -1)})
		}
		prev = m[1]
	}
	return out
}

// floorSentence applies the floors in s. It reports false when s isn't a
// floor sentence, and leaves s as text when a floor can't be placed.
func (p *enrolParser) floorSentence(s string) bool {
	if !floorSentenceRe.MatchString(s) {
		return false
	}
	p.closeList()
	floors := parseFloors(s)
	scope := p.floorScope()
	if floorOrAverageRe.MatchString(s) {
		defer p.averageReading(scope)
	}
	if floors == nil {
		return true // "lower than the minimum grades stated above": the stated grades are the floors
	}
	p.softFloor = notUsuallyRe.MatchString(s)
	defer func() { p.softFloor = false }()
	for _, f := range floors {
		if !p.applyFloor(f, scope) && !statedInText(scope, f.grade) {
			p.addPart(p.cur, &types.ReqNode{Type: types.NodeText, Kind: types.TextOther, Text: s}, partOther)
			return true
		}
	}
	return true
}

// floorScope returns the requirement parts a floor without named courses
// applies to, grouped by audience.
func (p *enrolParser) floorScope() map[*audience][]*part {
	pick := func(match func(*part) bool) map[*audience][]*part {
		out := map[*audience][]*part{}
		for _, a := range p.auds {
			for _, pt := range a.parts {
				if pt.kind == partList && match(pt) {
					out[a] = append(out[a], pt)
				}
			}
		}
		return out
	}
	if s := pick(func(pt *part) bool { return pt.sec == p.sec }); len(s) > 0 {
		return s
	}
	cur := p.cur
	s := pick(func(pt *part) bool { return true })
	for a := range s {
		if a != cur {
			delete(s, a)
		}
	}
	return s
}

// applyFloor applies one floor and reports whether it could be placed.
func (p *enrolParser) applyFloor(f floor, scope map[*audience][]*part) bool {
	if len(f.codes) > 0 {
		return p.applyNamedFloor(f, scope)
	}
	if len(scope) == 0 {
		return false
	}
	for a, parts := range scope {
		if !f.average {
			for _, pt := range parts {
				p.raiseGrades(pt.node, nil, f.grade)
			}
			continue
		}
		var nodes []*types.ReqNode
		for _, pt := range parts {
			nodes = append(nodes, pt.node)
		}
		if p.adjustedIn(nodes) {
			continue // the grades are adjusted before they are compared
		}
		avg := averageOver(nodes, f.grade)
		if avg == nil {
			return false
		}
		if p.softFloor {
			// "will not usually be considered": below the floor is Review.
			avg = orNode(avg, ambiguousText("applicants below the minimum are not usually considered, so some may be"))
		}
		p.addPart(a, avg, partFloor)
	}
	return true
}

// applyNamedFloor applies a floor on named courses to every part in scope
// that requires them; with no such part, to every part of every audience.
func (p *enrolParser) applyNamedFloor(f floor, scope map[*audience][]*part) bool {
	var parts []*part
	for _, ps := range scope {
		parts = append(parts, ps...)
	}
	if !slices.ContainsFunc(parts, func(pt *part) bool { return hasAnyCourse(pt.node, f.codes) }) {
		parts = nil
		for _, a := range p.auds {
			for _, pt := range a.parts {
				if pt.kind == partList {
					parts = append(parts, pt)
				}
			}
		}
	}
	placed := false
	for _, pt := range parts {
		if !f.average {
			if hasAnyCourse(pt.node, f.codes) {
				p.raiseGrades(pt.node, f.codes, f.grade)
				placed = true
			}
			continue
		}
		if slices.ContainsFunc(f.codes, func(c string) bool { return p.adjusted[c] }) {
			if p.attachAdjustedAverage(pt, f.codes, f.grade) {
				placed = true
			}
			continue
		}
		if attachAverage(pt, f.codes, f.grade) {
			placed = true
		}
	}
	return placed
}

// raiseGrades sets min_grade to at least grade on the courses of n (only
// codes, when given) and on plain credit pools when no codes are given. A
// course whose grade is adjusted before comparing ("Grades in MAT158H1 ...
// will be adjusted") meets the floor with its raw grade; below it, it is
// Review (adjustedFloor).
func (p *enrolParser) raiseGrades(n *types.ReqNode, codes []string, grade int) {
	if n == nil {
		return
	}
	switch n.Type {
	case types.NodeCourse:
		if codes == nil || slices.Contains(codes, n.Code) {
			if p.adjusted[n.Code] {
				*n = *adjustedFloor(&types.ReqNode{Type: types.NodeCourse, Code: n.Code}, &types.ReqNode{Type: types.NodeCourse, Code: n.Code, MinGrade: grade})
				return
			}
			n.MinGrade = max(n.MinGrade, grade)
		}
	case types.NodeCredits:
		if codes == nil && n.MaxAmount == 0 {
			n.MinGrade = max(n.MinGrade, grade)
		}
	}
	for _, it := range n.Items {
		p.raiseGrades(it, codes, grade)
	}
}

// adjustedText is why a floor on an adjusted grade can't be decided from the
// raw grade.
const adjustedText = "grade adjusted for the course's difficulty before the minimum applies"

// adjustedFloor is plain (the courses) meeting a floor that applies to
// adjusted grades: met is the floor on the raw grades. A raw grade at or
// above the floor meets it; below it, the adjusted grade might, so Review.
func adjustedFloor(plain, met *types.ReqNode) *types.ReqNode {
	return orNode(met, andNode(plain, &types.ReqNode{Type: types.NodeText, Kind: types.TextAdmission, Text: adjustedText}))
}

// attachAdjustedAverage applies "lower than 70% ... in the average of
// MAT148H1 or MAT158H1 and MAT149H1 or MAT159H1" when some of the courses
// are adjusted: the and(or(MAT148H1, MAT158H1), or(MAT149H1, MAT159H1))
// that requires exactly those courses becomes one branch per pair, each with
// the average over its pair; a pair with an adjusted course gets
// adjustedFloor.
func (p *enrolParser) attachAdjustedAverage(pt *part, codes []string, grade int) bool {
	want := dedupe(codes)
	slices.Sort(want)
	found := false
	var walk func(n *types.ReqNode) *types.ReqNode
	walk = func(n *types.ReqNode) *types.ReqNode {
		if n == nil || found {
			return n
		}
		have := dedupe(nodeCodes(n))
		slices.Sort(have)
		if slices.Equal(have, want) {
			if combos := courseCombos(n); combos != nil {
				found = true
				var branches []*types.ReqNode
				for _, combo := range combos {
					var courses []*types.ReqNode
					adjusted := false
					for _, c := range combo {
						courses = append(courses, &types.ReqNode{Type: types.NodeCourse, Code: c})
						adjusted = adjusted || p.adjusted[c]
					}
					avg := &types.ReqNode{Type: types.NodeAverage, MinGrade: grade, Courses: combo}
					plain := andNode(courses...)
					if adjusted {
						branches = append(branches, adjustedFloor(plain, andNode(plain, avg)))
					} else {
						branches = append(branches, andNode(plain, avg))
					}
				}
				return orNode(branches...)
			}
		}
		for i, it := range n.Items {
			n.Items[i] = walk(it)
		}
		return n
	}
	pt.node = walk(pt.node)
	return found
}

// courseCombos lists the course sets that meet n when n is an and of
// courses and ors of courses: and(or(A, B), or(C, D)) is AC, AD, BC, BD. It
// returns nil for any other shape.
func courseCombos(n *types.ReqNode) [][]string {
	if n.Type != types.NodeAnd {
		return nil
	}
	combos := [][]string{nil}
	for _, it := range n.Items {
		var alts []string
		switch it.Type {
		case types.NodeCourse:
			alts = []string{it.Code}
		case types.NodeOr:
			for _, c := range it.Items {
				if c.Type != types.NodeCourse {
					return nil
				}
				alts = append(alts, c.Code)
			}
		default:
			return nil
		}
		var next [][]string
		for _, combo := range combos {
			for _, a := range alts {
				next = append(next, append(slices.Clone(combo), a))
			}
		}
		combos = next
	}
	return combos
}

func (p *enrolParser) adjustedIn(nodes []*types.ReqNode) bool {
	var codes []string
	for _, n := range nodes {
		codes = append(codes, nodeCodes(n)...)
	}
	return slices.ContainsFunc(codes, func(c string) bool { return p.adjusted[c] })
}

// attachAverage adds an average over codes to the smallest node of pt that
// requires exactly those courses: the and(CHM135H1, CHM136H1) branch of
// "( CHM135H1, CHM136H1)/ CHM151Y1" gets the average, so CHM151Y1 alone
// still meets the requirement. A single course gets the grade as its
// min_grade.
func attachAverage(pt *part, codes []string, grade int) bool {
	want := dedupe(codes)
	slices.Sort(want)
	var found bool
	var walk func(n *types.ReqNode) *types.ReqNode
	walk = func(n *types.ReqNode) *types.ReqNode {
		if n == nil || found {
			return n
		}
		have := dedupe(nodeCodes(n))
		slices.Sort(have)
		if slices.Equal(have, want) {
			found = true
			if n.Type == types.NodeCourse {
				n.MinGrade = max(n.MinGrade, grade)
				return n
			}
			return andNode(n, &types.ReqNode{Type: types.NodeAverage, MinGrade: grade, Courses: want, Amount: avgAmount(n, want)})
		}
		for i, it := range n.Items {
			n.Items[i] = walk(it)
		}
		return n
	}
	pt.node = walk(pt.node)
	return found
}

// avgAmount is the credits an average over courses needs when n can be met
// with fewer than all of them, else 0.
func avgAmount(n *types.ReqNode, courses []string) float64 {
	total := 0.0
	for _, c := range courses {
		total += courseWeight(c)
	}
	if w := minWeight(n); w < total {
		return w
	}
	return 0
}

// nodeCodes lists the course codes a node names (courses and pools).
func nodeCodes(n *types.ReqNode) []string {
	if n == nil {
		return nil
	}
	var out []string
	if n.Code != "" {
		out = append(out, n.Code)
	}
	out = append(out, n.Courses...)
	for _, it := range n.Items {
		out = append(out, nodeCodes(it)...)
	}
	return out
}

func hasAnyCourse(n *types.ReqNode, codes []string) bool {
	return slices.ContainsFunc(nodeCodes(n), func(c string) bool { return slices.Contains(codes, c) })
}

// statedInText reports whether a text node in scope already states the
// grade: "a minimum grade average of 77% in 3.0 credits that count toward
// the program" makes the floor "lower than 77%" redundant.
func statedInText(scope map[*audience][]*part, grade int) bool {
	pct := strconv.Itoa(grade) + "%"
	for _, parts := range scope {
		for _, pt := range parts {
			found := false
			walkReq(pt.node, func(n *types.ReqNode) {
				found = found || n.Type == types.NodeText && strings.Contains(n.Text, pct)
			})
			if found {
				return true
			}
		}
	}
	return false
}
