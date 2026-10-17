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

// Requirement items, parsed with the course grammar after rewriting the
// wordings only enrolment uses.

var (
	// starsRe matches footnote marks: "IRW230H1*".
	starsRe = regexp.MustCompile(`\*+`)
	// passingRe matches "PSY100H1 (passing grade)": completing it is enough.
	passingRe = regexp.MustCompile(`(?i)\s*\(passing\s+grades?\)`)
	// anyCreditsAvgRe matches "3.0 credits (minimum grade average of 73%)";
	// grade12Re matches high-school preparation: "Gr. 12 Calculus".
	anyCreditsAvgRe = regexp.MustCompile(`(?i)^(\d+\.\d+)\s+credits?\s+\(\s*(?:a\s+)?minimum\s+grade\s+average\s+of\s+(\d{2})%\s*\)$`)
	grade12Re       = regexp.MustCompile(`(?i)^Gr(?:ade|\.)\s*12\b\s*`)
	highSchoolSepRe = regexp.MustCompile(`\s*(?:,|\band\b)\s*`)
	// formerlyRe matches "URB235H1 (formerly INI235H1)": the old code counts too.
	formerlyRe = regexp.MustCompile(`(` + grammar.CodePat + `)\s*\(\s*formerly\s+(` + grammar.CodePat + `)\s*\)`)
	// fromCampusRe matches campus notes on alternatives: "PSY100Y5 from UTM".
	fromCampusRe = regexp.MustCompile(`(?i)\s*\(both\)|\s+from\s+(?:UTM|UTSC)\b`)
	// seriesLevelRe matches "200-series", read as "200-level".
	seriesLevelRe = regexp.MustCompile(`\b(` + grammar.LevelPat + `)-series\b`)
	// doubleLeastRe matches a source typo: "at least at least 63%".
	doubleLeastRe = regexp.MustCompile(`(?i)(` + grammar.AtLeastPat + `\s+){2,}`)
	// eachWithRe matches ", each with a mark of at least 77%", moved after the
	// courses as "in each".
	eachWithRe = regexp.MustCompile(`(?i),?\s*each\s+(with\s+an?\s+(?:final\s+)?(?:mark|grade)\s+of\s+(?:` + grammar.AtLeastPat + `\s+)?\d{2}%)`)
	// labelPoolRe matches a pool naming its courses after a label or qualifiers:
	// "1.0 credit in Economics: ECO101H1, ...". The listed courses say it all.
	labelPoolRe = regexp.MustCompile(`(?i)\b(\d+\.\d+)\s+(?:\d00-level\s+)?(?:[A-Z]{3}\s+)?credits?\s+(?:in\s+[^:]+:|(?:in\s+[A-Z]{3}\s+)?from\s*:?)\s*(` + grammar.CodePat + `)`)
	// orFollowingRe matches ", or the following HIS First-Year Foundations (FYF)
	// courses:"; excludingFollowingRe the same after "excluding".
	orFollowingRe        = regexp.MustCompile(`(?i),?\s+or\s+the\s+following\s+[^:]*courses:\s*`)
	excludingFollowingRe = regexp.MustCompile(`(?i)\bexcluding\s+the\s+following\s+[^:]*courses:\s*`)
	// additionalRe matches "An additional 0.5 credit from ...": a second pick from
	// a pool named before.
	additionalRe = regexp.MustCompile(`(?i)^an\s+additional\s+`)
	// poolAndRe matches "1.0 credit from: CRI205H1, CRI210H1 and CRI225H1": the
	// last course is one more pool member, not an extra requirement.
	poolAndRe = regexp.MustCompile(`(?i)(\d+\.\d+\s+credits?\s+from:?\s+` + grammar.CodePat + `(?:,\s*` + grammar.CodePat + `)*),?\s+and\s+(` + grammar.CodePat + `)\b`)
	// listGradeRe matches a grade after a list of codes, or a pool of them, which
	// covers the list: "ECO101H1 and ECO102H1 with a final mark of at least 63%".
	// The list must start the item or follow ", and", ", or" or "; or".
	listGradeRe = regexp.MustCompile(`(?i)(^(?:completion\s+of\s+)?|,\s+(?:and|or)\s+|;\s+(?:or\s+)?)` +
		`((?:(?:\d+\.\d+\s+(?:additional\s+)?credits?\s+(?:from|in):?|one\s+of)\s+)?` + grammar.CodePat + `(?:\s*(?:,\s*(?:or\b|and\b)?|/|\bor\b|\band\b)\s*` + grammar.CodePat + `)*)` +
		`\s*,?\s+with\s+an?\s+(?:minimum\s+|final\s+)*(?:grade|mark)\s+of\s+(?:` + grammar.AtLeastPat + `\s+)?(\d{2})%(?:\s+in\s+each(?:\s+course)?)?`)
	// halvesRe matches "A, B/ C" where half courses A and B are replaced by C:
	// "( MAT148H1, MAT149H1/ MAT137Y1)" is (MAT148H1 and MAT149H1) or MAT137Y1.
	halvesRe = regexp.MustCompile(`(^|[(\[/]\s*)(([A-Z]{3})\d{3}H\d(?:\s*\(\d{2}%\))?)\s*(?:,|\band\b)\s*(([A-Z]{3})\d{3}H\d(?:\s*\(\d{2}%\))?)\s*/\s*(([A-Z]{3})\d{3}Y\d(?:\s*\(\d{2}%\))?)(\s*(?:$|[)\]/]))`)
	// gradeThenCodeRe matches a grade followed straight by a code, missing its
	// "/": "MAT159H1 (55%) MAT157Y1".
	gradeThenCodeRe = regexp.MustCompile(`(\(\s*\d{2}%\s*\))\s+(` + grammar.CodePat + `)`)
	// breadthCodeRe matches "BR=2 and/or BR=3 and/or TRN170Y1": a breadth pool
	// can't list courses, so the course goes to the notes.
	breadthCodeRe = regexp.MustCompile(`(?i)(\bBR\s*=\s*[1-5](?:\s*(?:and/or|/|or)\s*BR\s*=\s*[1-5])*)\s*(?:and/or|/|or)\s*(` + grammar.CodePat + `)`)
	countTowardRe = regexp.MustCompile(`(?i)^(an?\s+(?:minimum\s+)?grade\s+average\s+of\s+\d{2}%\s+in\s+\d+\.\d+\s+credits\s+that\s+count\s+toward\s+the\s+program),?\s+including\s+(.+)$`)
	// oneOfMustRe matches "One of the following courses must be completed:",
	// rewritten as "One of:".
	oneOfMustRe = regexp.MustCompile(`(?i)^one\s+of\s+the\s+following\s+courses\s+must\s+be\s+completed:`)
	// languagePairsRe matches "One Y or two H courses of the following ... with a
	// final grade of 65% in each course: EST100H1 and EST101H1, ...".
	languagePairsRe = regexp.MustCompile(`(?i)^one\s+Y\s+or\s+two\s+H\s+courses\s+of\s+the\s+following\s*(?:\([^)]*\))?\s*with\s+a\s+final\s+grade\s+of\s+(\d{2})%\s+in\s+each\s+course:\s*(.+)$`)
	pairAndRe       = regexp.MustCompile(`(` + grammar.CodePat + `)\s+and\s+(` + grammar.CodePat + `)`)
	// coursesOrCodeRe matches "ENG courses or JWE260H1": the course is one more
	// pool member.
	coursesOrCodeRe = regexp.MustCompile(`(?i)\bcourses\s+or\s+(` + grammar.CodePat + `)\b`)
	// gradeFromRe matches "0.5 credit with a minimum grade of 67% from ECO101H1",
	// rewritten as "67% in ECO101H1".
	gradeFromRe    = regexp.MustCompile(`(?i)(?:\d+\.\d+\s+credits?\s+with\s+|\ba\s+)?a?\s*minimum\s+grade\s+of\s+(\d{2})%\s+from\s+`)
	ofWhichMaxRe   = regexp.MustCompile(`(?i),?\s*of\s+which\s+` + grammar.NoMoreThanPat + `\s+(\d+\.\d+)\s+credits?\s+(?:can|may)\s+be\s+`)
	creditsFromDes = regexp.MustCompile(`(?i)\b(\d+\.\d+\s+credits?)\s+from\s+([A-Z]{3}(?:\s*/\s*[A-Z]{3})+)\b`)
)

// itemConj strips a trailing conjunction from an item and returns it in lower
// case ("or", "and", or "").
func itemConj(s string) (string, string) {
	s = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(s), "."))
	if m := programs.ItemConjRe.FindStringSubmatchIndex(s); m != nil {
		conj := strings.ToLower(s[m[2]:m[3]])
		return strings.TrimSpace(s[:m[0]]), conj
	}
	return s, ""
}

// parseItem parses one requirement item. Its notes go to the enrolment's notes.
func (p *parser) parseItem(s string) *types.ReqNode {
	s = strings.TrimSpace(strings.TrimRight(starsRe.ReplaceAllString(s, ""), ". "))
	if grade12Re.MatchString(s) {
		return highSchoolNode(passingRe.ReplaceAllString(s, ""))
	}
	if m := languagePairsRe.FindStringSubmatch(s); m != nil {
		n := p.parseFragmentNotes(strings.ReplaceAll(pairAndRe.ReplaceAllString(m[2], "($1 and $2)"), ",", "/"))
		g, _ := strconv.Atoi(m[1])
		grammar.ApplyMinGrade(n, g)
		return n
	}
	// any 3.0 credits, with that average; the amount includes having the credits
	if m := anyCreditsAvgRe.FindStringSubmatch(s); m != nil {
		amount, _ := strconv.ParseFloat(m[1], 64)
		grade, _ := strconv.Atoi(m[2])
		return &types.ReqNode{Type: types.NodeAverage, MinGrade: grade, Amount: amount}
	}
	// each average phrase ends a group averaged on its own; groups that "/"
	// separates are alternatives
	if locs := programs.ItemAvgRe.FindAllStringSubmatchIndex(s, -1); locs != nil {
		var alts []*types.ReqNode
		last := 0
		for _, m := range locs {
			grade, _ := strconv.Atoi(programs.SubOr(s, m, 2) + programs.SubOr(s, m, 4))
			seg := strings.TrimLeft(s[last:m[0]], " /")
			n := p.parseItemText(seg)
			alts = append(alts, programs.WithAverage(n, grade))
			last = m[1]
		}
		if rest := strings.TrimSpace(strings.TrimLeft(s[last:], " /")); rest != "" {
			alts = append(alts, p.parseItemText(rest))
		}
		if len(alts) == 1 {
			return alts[0]
		}
		if strings.HasPrefix(strings.TrimSpace(s[locs[0][1]:]), "/") {
			return programs.OrNode(alts...)
		}
		return programs.AndNode(alts...)
	}
	return p.parseItemText(s)
}

// parseItemText rewrites and parses item text that has no average phrase.
func (p *parser) parseItemText(s string) *types.ReqNode {
	if s == "" {
		return nil
	}
	if n := p.mixedPool(s); n != nil {
		return n
	}
	if m := labelRe.FindStringSubmatchIndex(s); m != nil {
		s = strings.TrimSpace(s[m[3]:])
	}
	if m := jointCourseRe.FindStringSubmatch(s); m != nil {
		// the joint courses' designators aren't named: Link fills them in
		n := p.parseItemText(jointCourseRe.ReplaceAllString(s, "in GGR courses"))
		alt := jointText("GGR")
		return replaceFirst(n, func(c *types.ReqNode) bool {
			return c.Type == types.NodeCredits && slices.Contains(c.Designators, "GGR")
		},
			func(c *types.ReqNode) *types.ReqNode { return programs.OrNode(c, alt) })
	}
	extra := ""
	if m := coursesOrCodeRe.FindStringSubmatch(s); m != nil {
		extra, s = m[1], strings.Replace(s, m[0], "courses", 1)
	}
	if m := countTowardRe.FindStringSubmatch(s); m != nil {
		// which credits count toward the program isn't known here
		return programs.AndNode(&types.ReqNode{Type: types.NodeText, Kind: types.TextOther, Text: m[1]}, p.parseItemText(m[2]))
	}
	if m := breadthCodeRe.FindStringSubmatch(s); m != nil {
		p.notes = append(p.notes, m[2]+" also counts toward "+strings.TrimSpace(s))
		s = strings.Replace(s, m[0], m[1], 1)
	}
	s = rewriteItem(s)
	n := p.parseFragmentNotes(s)
	if extra != "" {
		n = addToPool(n, extra)
	}
	if m := poolOfAltsRe.FindStringSubmatch(s); m != nil && containsType(n, types.NodeText) {
		n = p.poolOfAlternatives(m[1], m[2])
	}
	return n
}

var (
	// labelRe matches a label naming a group: "Economics: ECO101H1".
	labelRe = regexp.MustCompile(`^([A-Z][A-Za-z]*(?:[ /][A-Z][A-Za-z]*)*:)\s*` + grammar.CodePat)
	// jointCourseRe matches "in a Geography & Planning course, including joint
	// courses,".
	jointCourseRe = regexp.MustCompile(`(?i)\bin\s+an?\s+(Geography\s*&\s*Planning)\s+course,\s+including\s+joint\s+courses,`)
	// poolOfAltsRe matches a pool whose members may be bundles: "1.0 credit from (
	// MAT130H1/ MAT135H1 and MAT136H1)/ ...".
	poolOfAltsRe = regexp.MustCompile(`(?i)^(\d+\.\d+)\s+credits?\s+from:?\s+(.+)$`)
	// mixedPoolRe matches a pool of courses and designators: "2.0 credits from
	// CRI100H1, ECO, HIS, PHL, POL, PSY, or SOC courses".
	mixedPoolRe     = regexp.MustCompile(`^(\d+\.\d+)\s+credits?\s+from\s+((?:(?:` + grammar.CodePat + `|[A-Z]{3}),\s+)+(?:or\s+)?(?:` + grammar.CodePat + `|[A-Z]{3}))\s+courses(?:\s*\(any\s+course\s+level\s+and\s+combination\))?$`)
	mixedPoolItemRe = regexp.MustCompile(grammar.CodePat + `|\b[A-Z]{3}\b`)
)

// mixedPool reads a pool of courses and designators, or returns nil.
func (p *parser) mixedPool(s string) *types.ReqNode {
	m := mixedPoolRe.FindStringSubmatch(s)
	if m == nil {
		return nil
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	n := &types.ReqNode{Type: types.NodeCredits, Amount: amount}
	for _, it := range mixedPoolItemRe.FindAllString(m[2], -1) {
		if grammar.CodeRe.MatchString(it) {
			n.Courses = append(n.Courses, it)
		} else {
			n.Designators = append(n.Designators, it)
		}
	}
	return n
}

// poolOfAlternatives reads "amount credit from X" for X not a plain course
// list: one of X's alternatives when each weighs amount, loosely any of their
// courses (Review); otherwise amount credits from any of X's courses.
func (p *parser) poolOfAlternatives(amountText, list string) *types.ReqNode {
	amount, _ := strconv.ParseFloat(amountText, 64)
	var discard []string
	alts := grammar.ParseText(normalizeAndKeep(list, &discard))
	if alts != nil && !containsType(alts, types.NodeText) && alts.Type == types.NodeOr &&
		!slices.ContainsFunc(alts.Items, func(it *types.ReqNode) bool { return programs.MinWeight(it) != amount }) {
		any := &types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: grammar.Dedupe(grammar.CodeRe.FindAllString(list, -1))}
		return ambiguous(alts, any, "any "+amountText+" credit from the listed courses may count, not only one whole group")
	}
	flat := &types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: grammar.Dedupe(grammar.CodeRe.FindAllString(list, -1))}
	if strict := sequencesOrSingles(alts, amount); strict != nil {
		// a whole sequence, or the amount of the single courses; loosely, any of them
		return ambiguous(strict, flat, "any "+amountText+" credit from the listed courses may count, not only a whole sequence")
	}
	return flat
}

// sequencesOrSingles reads an and of alternatives weighing amount and single
// courses as one of those alternatives, or amount credits of the single
// courses. It returns nil for any other shape.
func sequencesOrSingles(n *types.ReqNode, amount float64) *types.ReqNode {
	if n == nil || n.Type != types.NodeAnd {
		return nil
	}
	var alts []*types.ReqNode
	var singles []string
	for _, it := range n.Items {
		switch {
		case it.Type == types.NodeCourse:
			singles = append(singles, it.Code)
		case it.Type == types.NodeOr && !slices.ContainsFunc(it.Items, func(c *types.ReqNode) bool { return programs.MinWeight(c) != amount }):
			alts = append(alts, it.Items...)
		default:
			return nil
		}
	}
	if alts == nil || singles == nil {
		return nil
	}
	return programs.OrNode(append(alts, &types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: singles})...)
}

// normalizeAndKeep normalizes s, adding its notes to notes.
func normalizeAndKeep(s string, notes *[]string) string {
	out, n := grammar.Normalize(s)
	*notes = append(*notes, n...)
	return out
}

// addToPool adds code to the credit pool in n when it passes the pool's level
// filter.
func addToPool(n *types.ReqNode, code string) *types.ReqNode {
	added := false
	grammar.WalkReq(n, func(c *types.ReqNode) {
		if !added && c.Type == types.NodeCredits && (len(c.Levels) == 0 || slices.Contains(c.Levels, programs.CourseLevel(code))) {
			c.Courses = append(c.Courses, code)
			added = true
		}
	})
	if !added {
		return programs.OrNode(n, &types.ReqNode{Type: types.NodeCourse, Code: code})
	}
	return n
}

// replaceFirst replaces the first node matching match with f(node).
func replaceFirst(n *types.ReqNode, match func(*types.ReqNode) bool, f func(*types.ReqNode) *types.ReqNode) *types.ReqNode {
	if n == nil {
		return nil
	}
	if match(n) {
		return f(n)
	}
	for i, it := range n.Items {
		if r := replaceFirst(it, match, f); r != it {
			n.Items[i] = r
			return n
		}
	}
	return n
}

// rewriteItem applies the enrolment-only rewrites to an item.
func rewriteItem(s string) string {
	s = passingRe.ReplaceAllString(s, "")
	s = formerlyRe.ReplaceAllString(s, "($1/ $2)")
	s = fromCampusRe.ReplaceAllString(s, "")
	s = seriesLevelRe.ReplaceAllString(s, "$1-level")
	s = doubleLeastRe.ReplaceAllString(s, "at least ")
	s = eachWithRe.ReplaceAllString(s, " $1 in each")
	s = additionalRe.ReplaceAllString(s, "")
	s = oneOfMustRe.ReplaceAllString(s, "One of:")
	s = gradeFromRe.ReplaceAllString(s, "$1% in ")
	s = orFollowingRe.ReplaceAllString(s, ", ")
	s = excludingFollowingRe.ReplaceAllString(s, "excluding ")
	s = labelPoolRe.ReplaceAllString(s, "$1 credit from: $2")
	s = programs.AnyLevelRe.ReplaceAllString(s, "")
	s = ofWhichMaxRe.ReplaceAllString(s, ", no more than $1 credits in ")
	s = creditsFromDes.ReplaceAllString(s, "$1 in $2 courses")
	s = poolAndRe.ReplaceAllString(s, "$1, $2")
	s = gradeThenCodeRe.ReplaceAllString(s, "$1/ $2")
	s = closeStrayParens(s)
	s = programs.SlashGradeRe.ReplaceAllStringFunc(s, programs.SpreadSlashGrade)
	s = halvesRe.ReplaceAllStringFunc(s, groupHalves)
	s = listGradeRe.ReplaceAllString(s, "$1($2) (minimum $3%)")
	return s
}

// groupHalves rewrites "A, B/ C" as "(A, B)/ C" when A and B are half courses
// of C's designator.
func groupHalves(m string) string {
	g := halvesRe.FindStringSubmatch(m)
	if g[3] != g[5] || g[5] != g[7] {
		return m
	}
	return g[1] + "(" + g[2] + ", " + g[4] + ")/ " + g[6] + g[8]
}

// parseFragmentNotes parses s with the course grammar and keeps its notes.
func (p *parser) parseFragmentNotes(s string) *types.ReqNode {
	r := grammar.ParseRequisite(s)
	p.notes = append(p.notes, r.Notes...)
	return r.Req
}

// highSchoolNode reads "Gr. 12 Calculus and Biology" as high-school courses
// the student must have.
func highSchoolNode(s string) *types.ReqNode {
	subjects := grade12Re.ReplaceAllString(s, "")
	var items []*types.ReqNode
	for _, sub := range highSchoolSepRe.Split(strings.TrimSpace(subjects), -1) {
		items = append(items, &types.ReqNode{Type: types.NodeProfile, Attr: "high_school_course", Value: "Grade 12 " + sub, Text: s})
	}
	return programs.AndNode(items...)
}

// closeStrayParens fixes a bracket closed without being opened: "MAT133Y1/
// MAT130H1/ MAT135H1, MAT136H1)/ ...". The "(" goes after the "/" where the
// alternatives then weigh the same and split into the most, else the first.
func closeStrayParens(s string) string {
	for {
		depth, stray := 0, -1
		var slashes []int
		for i := 0; i < len(s) && stray < 0; i++ {
			switch s[i] {
			case '(', '[':
				depth++
			case ')', ']':
				if depth == 0 {
					stray = i
				} else {
					depth--
				}
			case '/':
				if depth == 0 {
					slashes = append(slashes, i)
				}
			}
		}
		if stray < 0 {
			return s
		}
		if len(slashes) == 0 {
			s = s[:stray] + s[stray+1:] // nothing to pair it with
			continue
		}
		best, bestAlts := "", -1
		for _, at := range slashes {
			fixed := s[:at+1] + " (" + s[at+1:]
			n := grammar.ParseText(normalizeAndKeep(fixed, new([]string)))
			if n == nil || n.Type != types.NodeOr || containsType(n, types.NodeText) {
				continue
			}
			w := programs.MinWeight(n.Items[0])
			if slices.ContainsFunc(n.Items, func(it *types.ReqNode) bool { return programs.MinWeight(it) != w }) {
				continue
			}
			if len(n.Items) > bestAlts {
				best, bestAlts = fixed, len(n.Items)
			}
		}
		if best == "" {
			best = s[:slashes[0]+1] + " (" + s[slashes[0]+1:]
		}
		s = best
	}
}
