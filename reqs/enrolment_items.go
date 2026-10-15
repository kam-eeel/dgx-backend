package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// One requirement item (a bullet, or a paragraph that is a course list) is
// parsed with the course grammar, after rewriting the wordings enrolment
// lists use and course requisites don't. The rewrites only touch enrolment
// text; the shared grammar is unchanged.

var (
	// A trailing conjunction joins this item to the next: "Completion of
	// GGR172H1 with a grade of at least 70%, or", "COG100H1 and", "... ;
	// or", "... OR".
	itemConjRe = regexp.MustCompile(`(?i)[\s,;]*\b(or|and)\s*$`)
	// Footnote marks: "PSY100H1 (75%)*****", "IRW230H1*".
	starsRe = regexp.MustCompile(`\*+`)
	// "PSY100H1 (passing grade)": completing it is enough.
	passingRe = regexp.MustCompile(`(?i)\s*\(passing\s+grades?\)`)
	// "Gr. 12 Calculus" is high-school preparation.
	anyCreditsAvgRe = regexp.MustCompile(`(?i)^(\d+\.\d+)\s+credits?\s+\(\s*(?:a\s+)?minimum\s+grade\s+average\s+of\s+(\d{2})%\s*\)$`)
	grade12Re       = regexp.MustCompile(`(?i)^Gr(?:ade|\.)\s*12\b\s*`)
	highSchoolSepRe = regexp.MustCompile(`\s*(?:,|\band\b)\s*`)
	// "URB235H1 (formerly INI235H1)": the old code counts too.
	formerlyRe = regexp.MustCompile(`(` + codePat + `)\s*\(\s*formerly\s+(` + codePat + `)\s*\)`)
	// Campus notes on alternatives: "PSY100Y5 from UTM", "PSYA01H3 and
	// PSYA02H3 (both) from UTSC".
	fromCampusRe = regexp.MustCompile(`(?i)\s*\(both\)|\s+from\s+(?:UTM|UTSC)\b`)
	// "200-series ENG courses" is "200-level ENG courses".
	seriesLevelRe = regexp.MustCompile(`\b([1-4]00)-series\b`)
	// "at least at least 63%" (source typo).
	doubleLeastRe = regexp.MustCompile(`(?i)(at\s+least\s+){2,}`)
	// ", each with a mark of at least 77%" -> " with a mark of at least 77% in each".
	eachWithRe = regexp.MustCompile(`(?i),?\s*each\s+(with\s+an?\s+(?:final\s+)?(?:mark|grade)\s+of\s+(?:at\s+least\s+)?\d{2}%)`)
	// A pool that names its courses after a label or qualifiers: "1.0 credit
	// in Economics: ECO101H1, ...", "1.0 credit in Architectural/Visual
	// Studies: JAV120H1, ...", "1.0 200-level PSY credits from PSY210H1,
	// ...", "1.0 credits in PSY from PSY210H1, ...". The listed courses say
	// it all.
	labelPoolRe = regexp.MustCompile(`(?i)\b(\d+\.\d+)\s+(?:\d00-level\s+)?(?:[A-Z]{3}\s+)?credits?\s+(?:in\s+[^:]+:|(?:in\s+[A-Z]{3}\s+)?from\s*:?)\s*(` + codePat + `)`)
	// "A or the following HIS First-Year Foundations (FYF) courses: B, C"
	// and "excluding the following HIS First-Year Foundations courses: B, C".
	orFollowingRe        = regexp.MustCompile(`(?i),?\s+or\s+the\s+following\s+[^:]*courses:\s*`)
	excludingFollowingRe = regexp.MustCompile(`(?i)\bexcluding\s+the\s+following\s+[^:]*courses:\s*`)
	// "An additional 0.5 credit from ..." is a second pick from a pool named
	// before; see mergeAdditional.
	additionalRe = regexp.MustCompile(`(?i)^an\s+additional\s+`)
	// "1.0 credit from: CRI205H1, CRI210H1 and CRI225H1", "..., PHL275H1,
	// and POL200Y1": the last course is one more pool member, not an extra
	// requirement.
	poolAndRe = regexp.MustCompile(`(?i)(\d+\.\d+\s+credits?\s+from:?\s+` + codePat + `(?:,\s*` + codePat + `)*),?\s+and\s+(` + codePat + `)\b`)
	// A grade after a plain list of codes (or a pool of them) covers the
	// whole list: "ECO101H1 and ECO102H1 with a final mark of at least 63%",
	// "one of CSC108H1/ CSC110Y1 with a grade of at least 60%", "1.0 credit
	// from POL101H1, POL106H1, POL107H1, POL109H1 with a grade of 65% in
	// each". It is rewritten as a bracketed group with "(minimum N%)", which
	// the grammar applies to every course in the group.
	//
	// The list must start the item or follow ", and" / ", or" / "; or": in
	// "(CHM135H1, CHM136H1) or CHM151Y1, with a minimum grade of at least 60%
	// in each course" the grade covers the bracket too, which the grammar
	// already handles.
	listGradeRe = regexp.MustCompile(`(?i)(^(?:completion\s+of\s+)?|,\s+(?:and|or)\s+|;\s+(?:or\s+)?)` +
		`((?:(?:\d+\.\d+\s+(?:additional\s+)?credits?\s+(?:from|in):?|one\s+of)\s+)?` + codePat + `(?:\s*(?:,\s*(?:or\b|and\b)?|/|\bor\b|\band\b)\s*` + codePat + `)*)` +
		`\s*,?\s+with\s+an?\s+(?:minimum\s+|final\s+)*(?:grade|mark)\s+of\s+(?:at\s+least\s+)?(\d{2})%(?:\s+in\s+each(?:\s+course)?)?`)
	// One grade after the last of a slash list with no other grades covers
	// the list: "ANT200Y1/ ARH100Y1 (65%)", "PSY100H1/ SOC100H1/ VIC153H1
	// (70%)", "MAT130H1/ MAT135H1 (60%)".
	slashGradeRe = regexp.MustCompile(`((?:` + codePat + `\s*/\s*)+)(` + codePat + `)\s*\(\s*(\d{2})%\s*\)`)
	// "A, B/ C" where A and B are half courses and C the full course that
	// replaces both: "( MAT148H1, MAT149H1/ MAT137Y1)" is (MAT148H1 and
	// MAT149H1) or MAT137Y1, not MAT148H1 and (MAT149H1 or MAT137Y1).
	// Likewise "ECO101H1 (63%) and ECO102H1 (63%)/ ECO105Y1 (80%)".
	halvesRe = regexp.MustCompile(`(^|[(\[/]\s*)(([A-Z]{3})\d{3}H\d(?:\s*\(\d{2}%\))?)\s*(?:,|\band\b)\s*(([A-Z]{3})\d{3}H\d(?:\s*\(\d{2}%\))?)\s*/\s*(([A-Z]{3})\d{3}Y\d(?:\s*\(\d{2}%\))?)(\s*(?:$|[)\]/]))`)
	// A grade followed straight by another code is missing its "/":
	// "( MAT158H1 (55%), MAT159H1 (55%) MAT157Y1 (55%))".
	gradeThenCodeRe = regexp.MustCompile(`(\(\s*\d{2}%\s*\))\s+(` + codePat + `)`)
	// "3.0 credits from BR=2 and/or BR=3 and/or TRN170Y1": the course counts
	// toward a breadth pool, which can't list courses (they would have to
	// carry the breadth too). It goes to the notes.
	breadthCodeRe = regexp.MustCompile(`(?i)(\bBR\s*=\s*[1-5](?:\s*(?:and/or|/|or)\s*BR\s*=\s*[1-5])*)\s*(?:and/or|/|or)\s*(` + codePat + `)`)
	countTowardRe = regexp.MustCompile(`(?i)^(an?\s+(?:minimum\s+)?grade\s+average\s+of\s+\d{2}%\s+in\s+\d+\.\d+\s+credits\s+that\s+count\s+toward\s+the\s+program),?\s+including\s+(.+)$`)
	// "One of the following courses must be completed: A/ B" -> "One of: A/ B".
	oneOfMustRe = regexp.MustCompile(`(?i)^one\s+of\s+the\s+following\s+courses\s+must\s+be\s+completed:`)
	// "One Y or two H courses of the following (1.0 credit in a single
	// language) with a final grade of 65% in each course: EST100H1 and
	// EST101H1, ( FSL100H1 and FSL102H1 ) or ( FSL120H1 and FSL122H1) ,
	// GER100Y1, ..."
	languagePairsRe = regexp.MustCompile(`(?i)^one\s+Y\s+or\s+two\s+H\s+courses\s+of\s+the\s+following\s*(?:\([^)]*\))?\s*with\s+a\s+final\s+grade\s+of\s+(\d{2})%\s+in\s+each\s+course:\s*(.+)$`)
	pairAndRe       = regexp.MustCompile(`(` + codePat + `)\s+and\s+(` + codePat + `)`)
	// "2.0 credits in 200-level ENG courses or JWE260H1": the course is one
	// more pool member.
	coursesOrCodeRe = regexp.MustCompile(`(?i)\bcourses\s+or\s+(` + codePat + `)\b`)
	// "0.5 credit with a minimum grade of 67% from ECO101H1/ IRW230H1, or a
	// minimum grade of 80% from ECO105Y1" -> "67% in ECO101H1/ IRW230H1, or
	// 80% in ECO105Y1".
	gradeFromRe = regexp.MustCompile(`(?i)(?:\d+\.\d+\s+credits?\s+with\s+|\ba\s+)?a?\s*minimum\s+grade\s+of\s+(\d{2})%\s+from\s+`)
	// "1.5 credits from SOC/ PSY/ IRE/ IRW, at any level, of which no more
	// than 0.5 credit can be IRE/ IRW" -> "1.5 credits from SOC/ PSY/ IRE/
	// IRW, no more than 0.5 credits in IRE/ IRW".
	anyLevelRe     = regexp.MustCompile(`(?i),?\s*at\s+any\s+level\b`)
	ofWhichMaxRe   = regexp.MustCompile(`(?i),?\s*of\s+which\s+no\s+more\s+than\s+(\d+\.\d+)\s+credits?\s+(?:can|may)\s+be\s+`)
	creditsFromDes = regexp.MustCompile(`(?i)\b(\d+\.\d+\s+credits?)\s+from\s+([A-Z]{3}(?:\s*/\s*[A-Z]{3})+)\b`)

	// An average over the group before it: "( ACT240H1 and ACT245H1 and
	// ACT247H1) with an average grade of 63%", "... with a grade average of
	// 73% across all three courses", "... with at least an average of 75%
	// across the three of them", "... with at least a 78% average across all
	// four of them", "... with a combined minimum average grade of 75%".
	itemAvgRe = regexp.MustCompile(`(?i),?\s*with\s+(?:at\s+least\s+)?an?\s+(?:combined\s+)?(?:minimum\s+)?(?:(?:average\s+grade|grade\s+average|average)\s+of\s+(\d{2})%|(\d{2})%\s+average)(?:\s+across\s+(?:all\s+)?(?:the\s+)?\w+\s+(?:courses|of\s+them))?`)
)

// itemConj strips a trailing conjunction from an item and returns it in
// lower case ("or", "and", or "").
func itemConj(s string) (string, string) {
	s = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(s), "."))
	if m := itemConjRe.FindStringSubmatchIndex(s); m != nil {
		conj := strings.ToLower(s[m[2]:m[3]])
		return strings.TrimSpace(s[:m[0]]), conj
	}
	return s, ""
}

// parseItem parses one requirement item. Its notes go to the enrolment's
// notes.
func (p *enrolParser) parseItem(s string) *types.ReqNode {
	s = strings.TrimSpace(strings.TrimRight(starsRe.ReplaceAllString(s, ""), ". "))
	if grade12Re.MatchString(s) {
		return highSchoolNode(passingRe.ReplaceAllString(s, ""))
	}
	if m := languagePairsRe.FindStringSubmatch(s); m != nil {
		n := p.parseFragmentNotes(strings.ReplaceAll(pairAndRe.ReplaceAllString(m[2], "($1 and $2)"), ",", "/"))
		g, _ := strconv.Atoi(m[1])
		applyMinGrade(n, g)
		return n
	}
	// "3.0 credits (minimum grade average of 73%)": any 3.0 credits, with
	// that average. Amount includes having the credits.
	if m := anyCreditsAvgRe.FindStringSubmatch(s); m != nil {
		amount, _ := strconv.ParseFloat(m[1], 64)
		grade, _ := strconv.Atoi(m[2])
		return &types.ReqNode{Type: types.NodeAverage, MinGrade: grade, Amount: amount}
	}
	// An average over a group: split the item at each average phrase; each
	// segment is a group averaged on its own, and the segments are
	// alternatives if "/" separates them ("( MAT130H1/ MAT135H1, MAT136H1)
	// with an average grade of 73%/ ( MAT148H1, MAT149H1)/ MAT137Y1 with an
	// average grade of 63%/ ...").
	if locs := itemAvgRe.FindAllStringSubmatchIndex(s, -1); locs != nil {
		var alts []*types.ReqNode
		last := 0
		for _, m := range locs {
			grade, _ := strconv.Atoi(subOr(s, m, 2) + subOr(s, m, 4))
			seg := strings.TrimLeft(s[last:m[0]], " /")
			n := p.parseItemText(seg)
			alts = append(alts, withAverage(n, grade))
			last = m[1]
		}
		if rest := strings.TrimSpace(strings.TrimLeft(s[last:], " /")); rest != "" {
			alts = append(alts, p.parseItemText(rest))
		}
		if len(alts) == 1 {
			return alts[0]
		}
		if strings.HasPrefix(strings.TrimSpace(s[locs[0][1]:]), "/") {
			return orNode(alts...)
		}
		return andNode(alts...)
	}
	return p.parseItemText(s)
}

func subOr(s string, m []int, i int) string {
	if m[i] < 0 {
		return ""
	}
	return s[m[i]:m[i+1]]
}

// parseItemText rewrites and parses item text that has no average phrase.
func (p *enrolParser) parseItemText(s string) *types.ReqNode {
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
		// "0.5 credit in a Geography & Planning course, including joint
		// courses, at the 100- or 200-level": the joint courses' designators
		// aren't named, so they are an alternative Link fills in (jointText).
		n := p.parseItemText(jointCourseRe.ReplaceAllString(s, "in GGR courses"))
		alt := jointText("GGR")
		return replaceFirst(n, func(c *types.ReqNode) bool {
			return c.Type == types.NodeCredits && slices.Contains(c.Designators, "GGR")
		},
			func(c *types.ReqNode) *types.ReqNode { return orNode(c, alt) })
	}
	extra := ""
	if m := coursesOrCodeRe.FindStringSubmatch(s); m != nil {
		extra, s = m[1], strings.Replace(s, m[0], "courses", 1)
	}
	if m := countTowardRe.FindStringSubmatch(s); m != nil {
		// "A minimum grade average of 77% in 3.0 credits that count toward
		// the program, including 1.0 credit from: ...": which credits count
		// toward the program isn't known here.
		return andNode(&types.ReqNode{Type: types.NodeText, Kind: types.TextOther, Text: m[1]}, p.parseItemText(m[2]))
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
	// "Economics: ECO101H1, ECO102H1, ECO105Y1": the label names the
	// group; the lead says how much of it is needed.
	labelRe = regexp.MustCompile(`^([A-Z][A-Za-z]*(?:[ /][A-Z][A-Za-z]*)*:)\s*` + codePat)
	// "0.5 credit in a Geography & Planning course, including joint courses,"
	jointCourseRe = regexp.MustCompile(`(?i)\bin\s+an?\s+(Geography\s*&\s*Planning)\s+course,\s+including\s+joint\s+courses,`)
	// "1.0 credit from ( MAT130H1/ MAT135H1 and MAT136H1)/ ( MAT148H1,
	// MAT149H1/ MAT137Y1)/ ...": a pool whose members are bundles.
	poolOfAltsRe = regexp.MustCompile(`(?i)^(\d+\.\d+)\s+credits?\s+from:?\s+(.+)$`)
	// "2.0 credits from CRI100H1, ECO, HIS, PHL, POL, PSY, or SOC courses
	// (any course level and combination)": courses and designators mixed.
	mixedPoolRe     = regexp.MustCompile(`^(\d+\.\d+)\s+credits?\s+from\s+((?:(?:` + codePat + `|[A-Z]{3}),\s+)+(?:or\s+)?(?:` + codePat + `|[A-Z]{3}))\s+courses(?:\s*\(any\s+course\s+level\s+and\s+combination\))?$`)
	mixedPoolItemRe = regexp.MustCompile(codePat + `|\b[A-Z]{3}\b`)
)

// mixedPool reads a pool of courses and designators, or returns nil.
func (p *enrolParser) mixedPool(s string) *types.ReqNode {
	m := mixedPoolRe.FindStringSubmatch(s)
	if m == nil {
		return nil
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	n := &types.ReqNode{Type: types.NodeCredits, Amount: amount}
	for _, it := range mixedPoolItemRe.FindAllString(m[2], -1) {
		if codeRe.MatchString(it) {
			n.Courses = append(n.Courses, it)
		} else {
			n.Designators = append(n.Designators, it)
		}
	}
	return n
}

// poolOfAlternatives reads "amount credit from X" where X is not a plain
// course list. If X is alternatives that each weigh amount ("1.0 credit
// from ( MAT130H1/ MAT135H1 and MAT136H1)/ ( PHY131H1 and PHY132H1)"), it is
// one of them, or, read loosely, amount credits from any of their courses
// (Review). Otherwise it is amount credits from any of X's courses ("1.0
// credit from MAT130H1/ MAT135H1, MAT136H1, ( MAT148H1, MAT149H1)/
// MAT137Y1, PHY131H1, ...").
func (p *enrolParser) poolOfAlternatives(amountText, list string) *types.ReqNode {
	amount, _ := strconv.ParseFloat(amountText, 64)
	var discard []string
	alts := parseText(normalizeAndKeep(list, &discard))
	if alts != nil && !containsType(alts, types.NodeText) && alts.Type == types.NodeOr &&
		!slices.ContainsFunc(alts.Items, func(it *types.ReqNode) bool { return minWeight(it) != amount }) {
		any := &types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: dedupe(codeRe.FindAllString(list, -1))}
		return ambiguous(alts, any, "any "+amountText+" credit from the listed courses may count, not only one whole group")
	}
	flat := &types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: dedupe(codeRe.FindAllString(list, -1))}
	if strict := sequencesOrSingles(alts, amount); strict != nil {
		// "1.0 credit from ( MAT130H1/ MAT135H1, MAT136H1)/ ( MAT148H1,
		// MAT149H1)/ MAT137Y1, PHY131H1, PHY132H1, ...": a whole sequence,
		// or 1.0 credit of the single courses; loosely, any 1.0 credit.
		return ambiguous(strict, flat, "any "+amountText+" credit from the listed courses may count, not only a whole sequence")
	}
	return flat
}

// sequencesOrSingles reads an and of lists of alternatives that each weigh
// amount and of single courses as one of those alternatives, or amount
// credits of the single courses. It returns nil for any other shape.
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
		case it.Type == types.NodeOr && !slices.ContainsFunc(it.Items, func(c *types.ReqNode) bool { return minWeight(c) != amount }):
			alts = append(alts, it.Items...)
		default:
			return nil
		}
	}
	if alts == nil || singles == nil {
		return nil
	}
	return orNode(append(alts, &types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: singles})...)
}

// normalizeAndKeep normalizes s, adding its notes to notes.
func normalizeAndKeep(s string, notes *[]string) string {
	out, n := normalize(s)
	*notes = append(*notes, n...)
	return out
}

// addToPool adds code to the credit pool in n ("2.0 credits in 200-level
// ENG courses or JWE260H1"), when it passes the pool's level filter.
func addToPool(n *types.ReqNode, code string) *types.ReqNode {
	added := false
	walkReq(n, func(c *types.ReqNode) {
		if !added && c.Type == types.NodeCredits && (len(c.Levels) == 0 || slices.Contains(c.Levels, courseLevel(code))) {
			c.Courses = append(c.Courses, code)
			added = true
		}
	})
	if !added {
		return orNode(n, &types.ReqNode{Type: types.NodeCourse, Code: code})
	}
	return n
}

func walkReq(n *types.ReqNode, f func(*types.ReqNode)) {
	if n == nil {
		return
	}
	f(n)
	for _, it := range n.Items {
		walkReq(it, f)
	}
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
	s = anyLevelRe.ReplaceAllString(s, "")
	s = ofWhichMaxRe.ReplaceAllString(s, ", no more than $1 credits in ")
	s = creditsFromDes.ReplaceAllString(s, "$1 in $2 courses")
	s = poolAndRe.ReplaceAllString(s, "$1, $2")
	s = gradeThenCodeRe.ReplaceAllString(s, "$1/ $2")
	s = closeStrayParens(s)
	s = slashGradeRe.ReplaceAllStringFunc(s, spreadSlashGrade)
	s = halvesRe.ReplaceAllStringFunc(s, groupHalves)
	s = listGradeRe.ReplaceAllString(s, "$1($2) (minimum $3%)")
	return s
}

// spreadSlashGrade gives every code of "A/ B/ C (65%)" the grade.
func spreadSlashGrade(m string) string {
	g := slashGradeRe.FindStringSubmatch(m)
	var out []string
	for _, c := range append(codeRe.FindAllString(g[1], -1), g[2]) {
		out = append(out, c+" ("+g[3]+"%)")
	}
	return strings.Join(out, "/ ")
}

// groupHalves rewrites "A, B/ C" as "(A, B)/ C" when A and B are half
// courses of C's designator.
func groupHalves(m string) string {
	g := halvesRe.FindStringSubmatch(m)
	if g[3] != g[5] || g[5] != g[7] {
		return m
	}
	return g[1] + "(" + g[2] + ", " + g[4] + ")/ " + g[6] + g[8]
}

// parseFragmentNotes parses s with the course grammar and keeps its notes.
func (p *enrolParser) parseFragmentNotes(s string) *types.ReqNode {
	r := ParseRequisite(s)
	p.notes = append(p.notes, r.Notes...)
	return r.Req
}

// highSchoolNode reads "Gr. 12 Calculus", "Gr. 12 Calculus and Biology" as
// high-school courses the student must have.
func highSchoolNode(s string) *types.ReqNode {
	subjects := grade12Re.ReplaceAllString(s, "")
	var items []*types.ReqNode
	for _, sub := range highSchoolSepRe.Split(strings.TrimSpace(subjects), -1) {
		items = append(items, &types.ReqNode{Type: types.NodeProfile, Attr: "high_school_course", Value: "Grade 12 " + sub, Text: s})
	}
	return andNode(items...)
}

// courseLevel reads a code's level: CSC148H1 -> 100, MATB41H3 -> 200.
func courseLevel(code string) int {
	switch code[3] {
	case '1', 'A':
		return 100
	case '2', 'B':
		return 200
	case '3', 'C':
		return 300
	case '4', 'D':
		return 400
	}
	return 0
}

// courseWeight is a course's credit weight: H = 0.5, Y = 1.0.
func courseWeight(code string) float64 {
	if len(code) > 6 && code[6] == 'Y' {
		return 1.0
	}
	return 0.5
}

// withAverage adds an average of grade over the courses of n.
func withAverage(n *types.ReqNode, grade int) *types.ReqNode {
	avg := averageOver([]*types.ReqNode{n}, grade)
	if avg == nil {
		return andNode(n, &types.ReqNode{Type: types.NodeText, Kind: types.TextOther, Text: "an average of at least " + strconv.Itoa(grade) + "% in these courses"})
	}
	return andNode(n, avg)
}

// averageOver builds an average of grade over the courses that nodes
// require, or returns nil if one average can't say which courses count.
// Each requirement is a part: the courses every student takes, and each list
// of alternatives or credit pool, with the credits it needs. One part is a
// plain average over its filter. Several are items, so the average is over
// the credits each part needs rather than over every course that could
// match: "COG100H1 and 0.5 credits in PSY and 0.5 credit in LIN, PHL, or
// CSC" averages COG100H1, 0.5 PSY credits and 0.5 LIN/PHL/CSC credits. A cap
// ("no more than 0.5 credits in IRE/ IRW") limits which courses count but
// adds none.
func averageOver(nodes []*types.ReqNode, grade int) *types.ReqNode {
	var fixed []string
	var items []*types.ReqNode
	var add func(n *types.ReqNode) bool
	add = func(n *types.ReqNode) bool {
		if n == nil {
			return true
		}
		switch n.Type {
		case types.NodeCourse:
			fixed = append(fixed, n.Code)
		case types.NodeAnd:
			for _, it := range n.Items {
				if !add(it) {
					return false
				}
			}
		case types.NodeCredits:
			if n.Amount == 0 && n.MaxAmount != nil {
				return true
			}
			items = append(items, &types.ReqNode{Type: types.NodeCredits, Amount: n.Amount, Courses: n.Courses,
				Designators: n.Designators, Levels: n.Levels, Breadth: n.Breadth, Exclude: n.Exclude})
		case types.NodeOr, types.NodeChoose:
			f := alternativesItem(n)
			if f == nil {
				return false
			}
			items = append(items, f)
		case types.NodeAverage:
		default:
			return false
		}
		return true
	}
	for _, n := range nodes {
		if !add(n) {
			return nil
		}
	}
	if fixed = dedupe(fixed); fixed != nil {
		total := 0.0
		for _, c := range fixed {
			total += courseWeight(c)
		}
		items = append([]*types.ReqNode{{Type: types.NodeCredits, Courses: fixed, Amount: total}}, items...)
	}
	switch {
	case len(items) == 0:
		return nil
	case len(items) == 1 && items[0].Type == types.NodeCredits:
		it := items[0]
		avg := &types.ReqNode{Type: types.NodeAverage, MinGrade: grade, Courses: it.Courses,
			Designators: it.Designators, Levels: it.Levels, Breadth: it.Breadth, Exclude: it.Exclude}
		total := 0.0
		for _, c := range it.Courses {
			total += courseWeight(c)
		}
		// All the listed courses count unless fewer credits meet the part.
		if len(it.Designators) > 0 || len(it.Levels) > 0 || len(it.Breadth) > 0 || it.Amount < total {
			avg.Amount = it.Amount
		}
		return avg
	}
	avg := &types.ReqNode{Type: types.NodeAverage, MinGrade: grade, Items: items}
	for _, it := range items {
		avg.Amount += minWeight(it)
	}
	return avg
}

// alternativesItem is the average item for a list of alternatives. Single
// courses ("BIO230H1/ BIO255H1") make one filter over them. Groups ("(
// CHM135H1, CHM136H1)/ CHM151Y1", "1.0 credit from one of the following
// groups: - Economics: ... - Geography: ...") stay apart, an or (or choose)
// of one filter per group, so a choice can't mix half a group with half
// another.
func alternativesItem(n *types.ReqNode) *types.ReqNode {
	single := !slices.ContainsFunc(n.Items, func(it *types.ReqNode) bool {
		return it.Type != types.NodeCourse && !(it.Type == types.NodeText && it.Kind == types.TextAmbiguity)
	})
	if single {
		f := alternativesFilter(n)
		if f != nil {
			f.Amount = minWeight(n)
		}
		return f
	}
	alt := &types.ReqNode{Type: n.Type, Count: n.Count}
	for _, it := range n.Items {
		if it.Type == types.NodeText && it.Kind == types.TextAmbiguity {
			continue
		}
		f := alternativesFilter(it)
		if f == nil {
			return nil
		}
		f.Amount = minWeight(it)
		alt.Items = append(alt.Items, f)
	}
	if len(alt.Items) == 1 {
		return alt.Items[0]
	}
	return alt
}

// alternativesFilter is one filter for the courses of a list of
// alternatives: the courses named, and the designators of plain pools in it.
// It returns nil when a pool has a filter of its own (levels, breadth,
// exclusions) or the list has a node that isn't a course or a pool.
func alternativesFilter(n *types.ReqNode) *types.ReqNode {
	f := &types.ReqNode{Type: types.NodeCredits}
	var walk func(n *types.ReqNode) bool
	walk = func(n *types.ReqNode) bool {
		switch n.Type {
		case types.NodeCourse:
			f.Courses = append(f.Courses, n.Code)
		case types.NodeCredits:
			if ownFilter(n) {
				return false
			}
			f.Courses = append(f.Courses, n.Courses...)
			f.Designators = append(f.Designators, n.Designators...)
		case types.NodeAnd, types.NodeOr, types.NodeChoose:
			for _, it := range n.Items {
				if !walk(it) {
					return false
				}
			}
		case types.NodeText:
			return n.Kind == types.TextAmbiguity // marks a branch; adds no courses
		default:
			return false
		}
		return true
	}
	if !walk(n) {
		return nil
	}
	f.Courses, f.Designators = dedupe(f.Courses), dedupe(f.Designators)
	return f
}

// ownFilter reports whether a credit pool filters by more than courses and
// designators, so it can't merge into another pool's filter.
func ownFilter(c *types.ReqNode) bool {
	return len(c.Levels) > 0 || len(c.Breadth) > 0 || len(c.Exclude) > 0
}

// minWeight is the fewest credits that can meet n.
func minWeight(n *types.ReqNode) float64 {
	if n == nil {
		return 0
	}
	switch n.Type {
	case types.NodeCourse:
		return courseWeight(n.Code)
	case types.NodeCredits, types.NodeAverage:
		return n.Amount
	case types.NodeAnd:
		sum := 0.0
		for _, it := range n.Items {
			sum += minWeight(it)
		}
		return sum
	case types.NodeOr:
		best := -1.0
		for _, it := range n.Items {
			if w := minWeight(it); best < 0 || w < best {
				best = w
			}
		}
		return max(best, 0)
	case types.NodeChoose:
		var ws []float64
		for _, it := range n.Items {
			ws = append(ws, minWeight(it))
		}
		slices.Sort(ws)
		sum := 0.0
		for i := 0; i < n.Count && i < len(ws); i++ {
			sum += ws[i]
		}
		return sum
	}
	return 0
}

// andNode and orNode combine nodes, dropping nils and flattening children
// of the same type.
func andNode(items ...*types.ReqNode) *types.ReqNode { return combineNodes(types.NodeAnd, items) }
func orNode(items ...*types.ReqNode) *types.ReqNode  { return combineNodes(types.NodeOr, items) }

func combineNodes(t types.NodeType, items []*types.ReqNode) *types.ReqNode {
	var flat []*types.ReqNode
	for _, it := range items {
		switch {
		case it == nil:
		case it.Type == t && !it.Concurrent: // a concurrent subtree keeps its own scope
			flat = append(flat, it.Items...)
		default:
			flat = append(flat, it)
		}
	}
	flat = dedupeNodes(flat)
	switch len(flat) {
	case 0:
		return nil
	case 1:
		return flat[0]
	}
	return &types.ReqNode{Type: t, Items: flat}
}

// closeStrayParens fixes a bracket closed without being opened: "( MAT148H1/
// MAT158H1, MAT149H1/ MAT159H1)/ MAT137Y1)/ MAT157Y1", "MAT133Y1/ MAT130H1/
// MAT135H1, MAT136H1)/ ...". The missing "(" goes after one of the "/"
// before the stray ")": where the alternatives then all weigh the same, and
// split into the most of them; else after the first.
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
			n := parseText(normalizeAndKeep(fixed, new([]string)))
			if n == nil || n.Type != types.NodeOr || containsType(n, types.NodeText) {
				continue
			}
			w := minWeight(n.Items[0])
			if slices.ContainsFunc(n.Items, func(it *types.ReqNode) bool { return minWeight(it) != w }) {
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
