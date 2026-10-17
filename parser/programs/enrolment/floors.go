package enrolment

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/parser/grammar"
	"degree-explorer/parser/programs"
	"degree-explorer/types"
)

// Floors: "applicants with a final grade lower than 60% ... will not be
// considered". Falling below one is NotMet even when the cutoff is variable.

var (
	floorSentenceRe = regexp.MustCompile(`(?i)\bwill\s+not\s+(?:usually\s+)?be\s+considered\b|\bis\s+needed\s+for\s+admission\s+consideration\b`)
	// floorRe matches one floor: "lower than a B". The text before it, back to the
	// previous floor, says whether it is an average.
	floorRe = regexp.MustCompile(`(?i)\b(?:lower\s+than|below)\s+(?:(\d{2})\s*%|an?\s+([A-D][+-]?)\b)`)
	// floorAtLeastRe matches "An average of at least 73% in the prerequisite
	// courses".
	floorAtLeastRe = regexp.MustCompile(`(?i)\ban\s+average\s+of\s+` + grammar.AtLeastPat + `\s+(\d{2})%`)
	// floorTargetRe matches the courses right after a floor, up to the next one:
	// "in either BIO220H1 or ( BIO230H1/ BIO255H1)".
	floorTargetRe = regexp.MustCompile(`^\s*in\s+(?:either\s+)?((?:[\s(),/]|` + grammar.CodePat + `|\bor\b|\band\b)+)`)
	// floorAvgOfRe matches ", or in the average of MAT148H1 or MAT158H1".
	floorAvgOfRe   = regexp.MustCompile(`(?i)^\s*,?\s*(?:or\s+)?in\s+the\s+average\s+of\s+((?:[\s(),/]|` + grammar.CodePat + `|\bor\b|\band\b)+)`)
	floorAvgWordRe = regexp.MustCompile(`(?i)\baverages?\b`)
	// floorEitherRe matches a grade floor that also bounds the average: "a final
	// grade or grade average".
	floorEitherRe = regexp.MustCompile(`(?i)\bgrade\s+(?:or|and)\s+(?:grade\s+)?averages?\b`)
)

var (
	// floorOrAverageRe matches "grade or grade average lower than": each grade,
	// or the average, may be the floor.
	floorOrAverageRe = regexp.MustCompile(`(?i)\bgrade\s+or\s+(?:grade\s+)?averages?\s+lower\s+than\b`)
	// notUsuallyRe matches "will not usually be considered".
	notUsuallyRe = regexp.MustCompile(`(?i)\bnot\s+usually\b`)
)

// averageWhy is the lenient reading of "a final grade or grade average lower
// than ...".
const averageWhy = "the minimum may apply to the average of these courses rather than to each"

// averageReading adds the lenient reading of "a final grade or grade average
// lower than": courses sharing one minimum may need only their average to
// reach it. Courses with different minimums keep them.
func (p *parser) averageReading(scope map[*audience][]*part) {
	for a, parts := range scope {
		var singles []*part // parts that are graded courses: "- JAV120H1 and JAV130H1."
		var courses []*types.ReqNode
		for _, pt := range parts {
			if cs := gradedCourses(pt.node); cs != nil {
				singles = append(singles, pt)
				courses = append(courses, cs...)
				continue
			}
			pt.node = grammar.ReplaceNodes(pt.node, func(n *types.ReqNode) bool {
				return n.Type == types.NodeCredits && n.MinGrade > 0 && n.Amount >= 1
			}, func(n *types.ReqNode) *types.ReqNode {
				plain := grammar.CloneNode(n)
				plain.MinGrade = 0
				avg := &types.ReqNode{Type: types.NodeAverage, MinGrade: n.MinGrade, Amount: n.Amount, Courses: n.Courses,
					Designators: n.Designators, Levels: n.Levels, Breadth: n.Breadth, Exclude: n.Exclude}
				return ambiguous(n, programs.AndNode(plain, avg), averageWhy)
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
		combined := ambiguous(programs.AndNode(strict...), programs.AndNode(append(plain, avg)...), averageWhy)
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

// parseFloors reads the floors in a floor sentence, or nil for one that only
// refers to the grades already stated ("lower than the minimum grades stated
// above").
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
			g = grammar.LetterPercent[strings.ToUpper(s[m[4]:m[5]])]
		}
		before := s[prev:m[0]]
		f := floor{grade: g, average: floorAvgWordRe.MatchString(before) && !floorEitherRe.MatchString(before)}
		end := len(s)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		rest := s[m[1]:end]
		if t := floorTargetRe.FindStringSubmatch(rest); t != nil {
			f.codes = grammar.CodeRe.FindAllString(t[1], -1)
			rest = rest[len(t[0]):]
		}
		out = append(out, f)
		if a := floorAvgOfRe.FindStringSubmatch(rest); a != nil {
			out = append(out, floor{grade: g, average: true, codes: grammar.CodeRe.FindAllString(a[1], -1)})
		}
		prev = m[1]
	}
	return out
}

// floorSentence applies the floors in s. It reports false when s isn't a floor
// sentence, and leaves s as text when a floor can't be placed.
func (p *parser) floorSentence(s string) bool {
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
// applies to, by audience: its section's lists, else its audience's. Each
// audience gets its own average, over the courses on its route.
func (p *parser) floorScope() map[*audience][]*part {
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
func (p *parser) applyFloor(f floor, scope map[*audience][]*part) bool {
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
		avg := programs.AverageOver(nodes, f.grade)
		if avg == nil {
			return false
		}
		if p.softFloor {
			// "will not usually be considered": below the floor is Review
			avg = programs.OrNode(avg, programs.AmbiguousText("applicants below the minimum are not usually considered, so some may be"))
		}
		p.addPart(a, avg, partFloor)
	}
	return true
}

// applyNamedFloor applies a floor on named courses to every part in scope that
// requires them; with no such part, to every part of every audience.
func (p *parser) applyNamedFloor(f floor, scope map[*audience][]*part) bool {
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

// raiseGrades sets min_grade to at least grade on the courses of n (only codes,
// when given), or on plain credit pools when no codes are given. An adjusted
// course below the floor on its raw grade is Review.
func (p *parser) raiseGrades(n *types.ReqNode, codes []string, grade int) {
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
		if codes == nil && n.MaxAmount == nil {
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

// adjustedFloor is or(met, and(plain, text)): met is the floor on raw grades,
// and below it the adjusted grade might still meet it (Review).
func adjustedFloor(plain, met *types.ReqNode) *types.ReqNode {
	return programs.OrNode(met, programs.AndNode(plain, &types.ReqNode{Type: types.NodeText, Kind: types.TextAdmission, Text: adjustedText}))
}

// attachAdjustedAverage applies "in the average of MAT148H1 or MAT158H1 and
// MAT149H1 or MAT159H1" when some are adjusted: one branch per pair, each with
// its average; a pair with an adjusted course gets adjustedFloor.
func (p *parser) attachAdjustedAverage(pt *part, codes []string, grade int) bool {
	want := grammar.Dedupe(codes)
	slices.Sort(want)
	found := false
	var walk func(n *types.ReqNode) *types.ReqNode
	walk = func(n *types.ReqNode) *types.ReqNode {
		if n == nil || found {
			return n
		}
		have := grammar.Dedupe(programs.NodeCodes(n))
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
					plain := programs.AndNode(courses...)
					if adjusted {
						branches = append(branches, adjustedFloor(plain, programs.AndNode(plain, avg)))
					} else {
						branches = append(branches, programs.AndNode(plain, avg))
					}
				}
				return programs.OrNode(branches...)
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

// courseCombos lists the course sets that meet n when n is an and of courses
// and ors of courses: and(or(A, B), or(C, D)) is AC, AD, BC, BD. Else nil.
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

func (p *parser) adjustedIn(nodes []*types.ReqNode) bool {
	var codes []string
	for _, n := range nodes {
		codes = append(codes, programs.NodeCodes(n)...)
	}
	return slices.ContainsFunc(codes, func(c string) bool { return p.adjusted[c] })
}

// attachAverage adds an average over codes to the smallest node of pt that
// requires exactly those courses, so other alternatives still meet it. A
// single course gets the grade as its min_grade.
func attachAverage(pt *part, codes []string, grade int) bool {
	want := grammar.Dedupe(codes)
	slices.Sort(want)
	var found bool
	var walk func(n *types.ReqNode) *types.ReqNode
	walk = func(n *types.ReqNode) *types.ReqNode {
		if n == nil || found {
			return n
		}
		have := grammar.Dedupe(programs.NodeCodes(n))
		slices.Sort(have)
		if slices.Equal(have, want) {
			found = true
			if n.Type == types.NodeCourse {
				n.MinGrade = max(n.MinGrade, grade)
				return n
			}
			return programs.AndNode(n, &types.ReqNode{Type: types.NodeAverage, MinGrade: grade, Courses: want, Amount: avgAmount(n, want)})
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
		total += grammar.CourseWeight(c)
	}
	if w := programs.MinWeight(n); w < total {
		return w
	}
	return 0
}

func hasAnyCourse(n *types.ReqNode, codes []string) bool {
	return slices.ContainsFunc(programs.NodeCodes(n), func(c string) bool { return slices.Contains(codes, c) })
}

// statedInText reports whether a text node in scope already states the grade,
// making the floor redundant.
func statedInText(scope map[*audience][]*part, grade int) bool {
	pct := strconv.Itoa(grade) + "%"
	for _, parts := range scope {
		for _, pt := range parts {
			found := false
			grammar.WalkReq(pt.node, func(n *types.ReqNode) {
				found = found || n.Type == types.NodeText && strings.Contains(n.Text, pct)
			})
			if found {
				return true
			}
		}
	}
	return false
}
