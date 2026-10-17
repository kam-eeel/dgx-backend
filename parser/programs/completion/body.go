package completion

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/parser/grammar"
	"degree-explorer/parser/programs"
	"degree-explorer/types"
)

// Item bodies: an item's first requirement sentence read by the grammar, its
// remarks and asides taken out, its wording rewritten and the result checked.

var (
	// gluedMarkerRe matches a list marker glued to a bracket, which the
	// block reader leaves in the text: "4.(".
	gluedMarkerRe = regexp.MustCompile(`^\d{1,2}[.)]\s*\(`)
	// remarkParenRe matches a bracketed remark on the item, not part of its
	// requirement: "(NOTE: PSL201Y1 is not acceptable)".
	remarkParenRe = regexp.MustCompile(`\s*\(((?:[A-Za-z]+:\s*)?[A-Z][^()]*\s[^()]*\s[^()]*)\)`)
)

// itemBody parses an item's first requirement sentence into its body and the
// rows written inside it, such as a bracketed floor or a course that doesn't
// count. Bracketed advice becomes notes.
func (a *Assembler) itemBody(s string) (body *types.ReqNode, rows []*types.Requirement, notes []string) {
	if n, rs, ok := floorFromDesignator(s); ok {
		return n, rs, nil
	}
	body, rows, notes = a.grammarBody(s)
	if body == nil || hasText(body) {
		// a bracketed remark, or a clause on courses already used
		if n, rs, ns, ok := a.remarkItem(s); ok {
			return n, rs, ns
		}
	}
	return body, rows, notes
}

// grammarBody is itemBody without remarkItem.
func (a *Assembler) grammarBody(s string) (body *types.ReqNode, rows []*types.Requirement, notes []string) {
	s = gluedMarkerRe.ReplaceAllString(strings.TrimSpace(s), "(")
	// one-off wordings, as rewriteCompletion reads them, before floors come out
	s = rewriteOneOffWordings(s)
	if n := profileBody(s); n != nil {
		// a learning community or a language proficiency
		return n, nil, nil
	}
	if n, rows, notes, ok := a.pairPool(s); ok {
		// single courses and pairs the item grammar leaves as text
		return n, rows, notes
	}
	if m := requiredSentenceRe.FindStringSubmatch(s); m != nil && !onlySomeRe.MatchString(m[1]) && !onlySomeRe.MatchString(s[len(m[0]):]) {
		// "A, B, and C are required courses that ...": the courses; the rest advice
		if n, _ := parseCompletionText(m[1]); n != nil && fixedCourses(n) {
			if advice := strings.TrimSpace(thatRe.ReplaceAllString(s[len(m[0]):], "")); advice != "" && advice != "." {
				notes = []string{grammar.CleanNote(advice)}
			}
			return n, nil, notes
		}
	}
	// widen: clauses adding capped courses to the pool; if the pool can't take
	// them, the item is text
	var widen []string
	s0 := s
	defer func() {
		for _, w := range widen {
			pool, caps := poolCaps(body)
			wide, limit := widenPool(pool, w)
			if limit == nil {
				body, rows = TextNode(types.TextOther, s0), nil
				return
			}
			body = programs.AndNode(append([]*types.ReqNode{wide}, caps...)...)
			rows = append(rows, limit)
		}
	}()
	if parts := grammar.SplitTop(s, ';'); len(parts) > 1 {
		// "...; up to 1.0 credit at the 100 level will count ...": the item's cap
		var keep []string
		for i, p := range parts {
			if r := capRow(strings.TrimSpace(p)); i > 0 && r != nil && capLeadRe.MatchString(strings.TrimSpace(p)) {
				rows = append(rows, r)
				continue
			}
			if i > 0 && substitutedFromRe.MatchString(strings.TrimSuffix(strings.TrimSpace(a.clean(p)), ".")) {
				widen = append(widen, strings.TrimSpace(p))
				continue
			}
			keep = append(keep, p)
		}
		s = strings.Join(keep, ";")
	}
	// "1.5 credits of any of the following (at least ...)": pool and floor
	s = ofAnyFollowingRe.ReplaceAllString(s, "$1 $2 from:")
	if m := itemFloorParenRe.FindStringSubmatch(s); m != nil {
		s = strings.TrimSpace(m[1] + " " + m[3])
		rows = append(rows, a.floorRows(m[2])...)
	}
	s = lowerRemarkRe.ReplaceAllStringFunc(s, func(p string) string {
		// "(should be taken in first year, ...)": advice on when to take it
		inner := lowerRemarkRe.FindStringSubmatch(p)[1]
		if len(strings.Fields(inner)) < 3 || a.role(inner) != RoleAdvisory && !timingRe.MatchString(inner) {
			return p
		}
		notes = append(notes, grammar.CleanNote(inner))
		return ""
	})
	var alts []string
	var courseAlts []*types.ReqNode
	s = remarkParenRe.ReplaceAllStringFunc(s, func(p string) string {
		inner := remarkParenRe.FindStringSubmatch(p)[1]
		switch role := a.role(inner); {
		case alsoAcceptRe.MatchString(a.clean(inner)) && alsoAccepted(a.clean(inner)) != nil:
			// "(NOTE: We will also accept PSY100Y5 ...)": the courses it names
			courseAlts = append(courseAlts, alsoAccepted(a.clean(inner)))
		case role == RoleRequirement && a.IsAlternative(inner) && !negationRe.MatchString(inner):
			// another way to meet the item
			alts = append(alts, grammar.CleanNote(inner))
		case role == RoleAdvisory:
			notes = append(notes, grammar.CleanNote(inner))
		case role == roleLimit || notCountedRe.MatchString(inner):
			r := capRow(inner)
			if r == nil {
				return p
			}
			rows = append(rows, r)
		default:
			return p
		}
		return ""
	})
	if m := andOrApprovalRe.FindStringSubmatch(strings.TrimSuffix(s, ".")); m != nil {
		// "A/ B and/or C/ D ... with prior approval by the Department": the
		// approval is for the courses after "and/or"
		strict, _ := parseCompletionText(m[1])
		approved, _ := parseCompletionText(m[2])
		return programs.OrNode(strict, programs.AndNode(approved, TextNode(types.TextPermission, m[3]))), rows, notes
	}
	if m := permListRe.FindStringSubmatch(strings.TrimSpace(s)); m != nil {
		// "1.0 credit from: A/ B or with permission of the Director: C/ D": the
		// list, or with permission the same amount from both lists
		return permissionList(m), rows, notes
	}
	if m := withAverageRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(s), ".")); m != nil {
		// "( A and B and C) with an average grade of 63%": courses and average
		codes := grammar.CodeRe.FindAllString(m[1], -1)
		grade, _ := strconv.Atoi(m[2])
		items := programs.CourseNodes(codes)
		items = append(items, &types.ReqNode{Type: types.NodeAverage, MinGrade: grade, Courses: codes})
		return programs.AndNode(items...), rows, notes
	}
	if m := anyRemainingRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(s), ".")); m != nil {
		// "1.0 credit from ..., or any remaining 300+ level STA courses ... - <codes>
		// (<codes> may be considered ... with the approval of ...)": the listed
		// courses are all the pool's; only the bracketed ones need approval
		pool, _ := parseCompletionText(m[1] + ", " + m[2])
		pool, caps := poolCaps(checkedBody(pool, m[1]+", "+m[2]))
		if isPool(pool) && pool.Designators == nil {
			for _, c := range caps {
				rows = append(rows, &types.Requirement{Kind: types.RequirementLimit, Req: c, Raw: s})
			}
			if extra := grammar.CodeRe.FindAllString(m[3], -1); extra != nil && approvalWordRe.MatchString(m[3]) {
				wide := grammar.CloneNode(pool)
				wide.Courses = grammar.Dedupe(append(slices.Clone(pool.Courses), extra...))
				return programs.OrNode(pool, programs.AndNode(wide, TextNode(types.TextPermission, m[3]))), rows, notes
			}
			return pool, rows, notes
		}
	}
	if m := orEquivalentRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(s), ".")); m != nil {
		// "0.5 LAS credit at the 400-level, or an equivalent 0.5 credit ...,
		// approved by ...": the strict pool, or the wider one with approval
		strict, _ := parseCompletionText(m[1])
		wide, _ := parseCompletionText(m[2])
		if strict = checkedBody(strict, m[1]); strict != nil && wide != nil && !hasText(strict) && !hasText(wide) {
			return programs.OrNode(strict, programs.AndNode(wide, TextNode(types.TextPermission, m[3]))), rows, notes
		}
	}
	if m := orAnotherRe.FindStringSubmatchIndex(s); m != nil {
		// "A/ B/ ..., or another course with ... approved by the Undergraduate
		// Chair": the listed courses, or one a person approves
		head, more := parseCompletionText(s[:m[0]])
		head = checkedBody(head, s[:m[0]])
		if head != nil && head.Type == types.NodeAnd && !slices.ContainsFunc(head.Items, func(n *types.ReqNode) bool { return n.Type != types.NodeCourse }) {
			head = programs.OrNode(head.Items...) // "A, B, C, or ...": one of them
		}
		alt := TextNode(types.TextPermission, s[m[2]:m[3]])
		return programs.OrNode(head, alt), rows, append(notes, more...)
	}
	if m := upToFromListRe.FindStringSubmatchIndex(s); m != nil {
		// "Any 4 courses (2.0 credits) from ..., up to one half course from Group
		// H.": a capped share from a list, an alternative until lists are read
		alts = append(alts, strings.TrimSpace(s[m[2]:m[3]]))
		s = strings.TrimSpace(s[:m[0]])
	}
	n, more := parseCompletionText(s)
	n = topicMembers(checkedBody(n, s), topicCodes(s))
	if hasText(n) {
		// "AMS200H1, ... (total of 1.5 credit)": a bracketed aside spoils the
		// reading; without it the item reads, and the aside is its note
		if t, asides := stripAsides(s); asides != nil {
			if m, mo := parseCompletionText(t); m != nil && !hasText(checkedBody(m, t)) {
				m = checkedBody(m, t)
				asides = formerlyAlternatives(m, s, asides)
				s, n, more = t, m, mo
				notes = append(notes, asides...)
			}
		}
	}
	n = listMemberAlt(n)
	n = orToPool(n, s)
	if g := departmentGroups(s); g != nil && isPool(n) {
		n = g
	}
	n = bracketPairCaps(n, s)
	if n != nil && alts != nil {
		for _, alt := range alts {
			n = programs.OrNode(n, TextNode(altKind(alt), alt))
		}
	}
	if n != nil && courseAlts != nil {
		n = programs.OrNode(append([]*types.ReqNode{n}, courseAlts...)...)
	}
	if m := ieRe.FindStringSubmatch(s); m != nil {
		restatedLevels(n, ieLeadRe.ReplaceAllString(m[1], ""))
	}
	return n, rows, append(notes, more...)
}

var (
	// alsoAcceptRe matches courses accepted in place of the item's: "We will
	// also accept PSY100Y5 from UTM ... in place of PSY100H1 for this requirement".
	alsoAcceptRe = regexp.MustCompile(`(?i)^we\s+will\s+also\s+accept\s+(.+?)(?:,?\s+in\s+place\s+of\s+` + grammar.CodePat + `\b.*)?$`)
	// alsoAcceptNoiseRe matches the words around the codes that say where
	// and how: " from UTM".
	alsoAcceptNoiseRe = regexp.MustCompile(`(?i)\s*\bfrom\s+(?:UTM|UTSC)\b|\bboth\s+of\s+|\s+together\b`)
	// alsoAcceptOrRe matches what separates the alternatives: ", or ".
	alsoAcceptOrRe = regexp.MustCompile(`(?i),?\s+or\s+`)
	// alsoAcceptAndRe matches what may join one alternative's codes: "and".
	alsoAcceptAndRe = regexp.MustCompile(`(?i)\band\b|[,\s]`)
)

// alsoAccepted reads a alsoAcceptRe sentence into the courses it
// accepts: alternatives joined by "or", each courses joined by "and". It is
// nil when they don't read as courses alone.
func alsoAccepted(t string) *types.ReqNode {
	m := alsoAcceptRe.FindStringSubmatch(t)
	if m == nil {
		return nil
	}
	var alts []*types.ReqNode
	for _, part := range alsoAcceptOrRe.Split(alsoAcceptNoiseRe.ReplaceAllString(m[1], ""), -1) {
		codes := grammar.CodeRe.FindAllString(part, -1)
		rest := strings.TrimSpace(alsoAcceptAndRe.ReplaceAllString(grammar.CodeRe.ReplaceAllString(part, ""), ""))
		if codes == nil || rest != "" {
			return nil
		}
		nodes := programs.CourseNodes(codes)
		alts = append(alts, programs.AndNode(nodes...))
	}
	return programs.OrNode(alts...)
}

// orToPool reads alternatives under a lead amount as the pool it names,
// credits{amount, every course}, when one alternative weighs more than the
// amount: and(MAT158H1, MAT159H1) for 0.5 credit would fail MAT158H1 alone.
func orToPool(n *types.ReqNode, s string) *types.ReqNode {
	m := leadAmountRe.FindStringSubmatch(s)
	if n == nil || n.Type != types.NodeOr || m == nil {
		return n
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	heavy := false
	var codes []string
	for _, it := range n.Items {
		v, ok := nodeCredits(it)
		if !ok || it.Type != types.NodeCourse && it.Type != types.NodeAnd {
			return n
		}
		for _, x := range it.Items {
			if x.Type != types.NodeCourse {
				return n
			}
		}
		heavy = heavy || v > amount+1e-9
		grammar.WalkReq(it, func(x *types.ReqNode) {
			if x.Type == types.NodeCourse {
				codes = append(codes, x.Code)
			}
		})
	}
	if !heavy {
		return n
	}
	return &types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: grammar.Dedupe(codes)}
}

// departmentGroupRe matches an amount over ";" groups: "1.0 credit from
// ECO220Y1, ECO227Y1; GGR270H1, GGR271H1; STA220H1, STA221H1".
var departmentGroupRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\s+from:?\s+(.+)$`)

// commaSpaceRe matches what may join a group's codes: ",".
var commaSpaceRe = regexp.MustCompile(`[,\s]`)

// departmentGroups reads a departmentGroupRe pool as the or of its 3+
// groups, each one department's courses making up the amount together or each
// alone, or nil. A union of their courses would count GGR270H1 with STA220H1.
func departmentGroups(s string) *types.ReqNode {
	m := departmentGroupRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(s), "."))
	if m == nil {
		return nil
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	groups := grammar.SplitTop(m[2], ';')
	if len(groups) < 3 {
		return nil
	}
	var alts []*types.ReqNode
	for _, g := range groups {
		if strings.TrimSpace(commaSpaceRe.ReplaceAllString(grammar.CodeRe.ReplaceAllString(g, ""), "")) != "" {
			return nil // more than codes and commas
		}
		codes := grammar.CodeRe.FindAllString(g, -1)
		if len(codes) < 2 || slices.ContainsFunc(codes, func(c string) bool { return c[:3] != codes[0][:3] }) {
			return nil
		}
		sum, each := 0.0, true
		var nodes []*types.ReqNode
		for _, c := range codes {
			sum += grammar.CourseWeight(c)
			each = each && grammar.CourseWeight(c) == amount
			nodes = append(nodes, &types.ReqNode{Type: types.NodeCourse, Code: c})
		}
		switch {
		case math.Abs(sum-amount) < 1e-9:
			alts = append(alts, programs.AndNode(nodes...))
		case each:
			alts = append(alts, nodes...)
		default:
			return nil
		}
	}
	return programs.OrNode(alts...)
}

// bracketPairRe matches bracketed alternatives inside a "/" list:
// "( EEB240H1/ ENV234H1)".
var bracketPairRe = regexp.MustCompile(`\(\s*(` + grammar.CodePat + `(?:\s*/\s*` + grammar.CodePat + `)+)\s*\)`)

// bracketPairCaps caps each bracketed group of alternatives flattened into
// pool n at its heaviest course, as pairCaps does for a "/" pair.
func bracketPairCaps(n *types.ReqNode, s string) *types.ReqNode {
	pool, caps := poolCaps(n)
	if !isPool(pool) || pool.Courses == nil {
		return n
	}
	for _, m := range bracketPairRe.FindAllStringSubmatch(s, -1) {
		codes := grammar.Dedupe(grammar.CodeRe.FindAllString(m[1], -1))
		w := grammar.HeaviestCourse(codes)
		if slices.ContainsFunc(codes, func(c string) bool { return !slices.Contains(pool.Courses, c) }) ||
			slices.ContainsFunc(caps, func(c *types.ReqNode) bool { return programs.IsSubset(codes, c.Courses) && *c.MaxAmount <= w }) {
			continue // a cap that holds already
		}
		caps = append(caps, grammar.CourseCap(codes))
	}
	return programs.AndNode(append([]*types.ReqNode{pool}, caps...)...)
}

var (
	// asideParenRe matches brackets that remark on an item rather than
	// require: "(formerly POL101Y1)".
	asideParenRe = regexp.MustCompile(`\(\s*_?((?i:note|nb)\s*:[^()]*|(?i:formerly)\s+[^()]*|(?i:see)\s+(?i:note)[^()]*|(?i:if\s+taking)\b[^()]*|(?i:total\s+of)\s+\d[^()]*|` + grammar.NumPat + `\s+credits?\s+in\s+total|(?i:the|these)\s+[^()]*\b(?:are|is)\s+(?:recommended|suggested|encouraged)\.?|(?i:these)\s+[A-Z]{3}\s+courses\s+[^()]*|[A-Z][\w&,\s]*\bcourses)_?\s*\)`)
	// appositiveRe matches a descriptor closing a list of alternatives: ", a
	// capstone integrative course".
	appositiveRe = regexp.MustCompile(`,\s+an?\s+[a-z][a-z\s-]*\bcourse\.?$`)
)

// categoryAndRe matches a category label between two lists of one pool:
// "(History & Society courses) and ".
var categoryAndRe = regexp.MustCompile(`\(\s*([A-Z][\w&,\s]*\bcourses)\s*\)\s*,?\s*and\s+`)

// formerlyAlternatives also accepts the old code of each course of n that s
// writes as "POL101H1 (formerly POL101Y1)": a course becomes or(course, old)
// and a pool takes the old code. It returns asides minus those it read.
func formerlyAlternatives(n *types.ReqNode, s string, asides []string) []string {
	for _, m := range codeFormerlyRe.FindAllStringSubmatch(s, -1) {
		cur, old := m[1], m[2]
		var found []*types.ReqNode
		grammar.WalkReq(n, func(x *types.ReqNode) {
			if x.Type == types.NodeCourse && x.Code == cur || x.Type == types.NodeCredits && slices.Contains(x.Courses, cur) {
				found = append(found, x)
			}
		})
		for _, x := range found {
			if x.Type == types.NodeCourse {
				*x = types.ReqNode{Type: types.NodeOr, Items: []*types.ReqNode{{Type: types.NodeCourse, Code: cur}, {Type: types.NodeCourse, Code: old}}}
			} else {
				x.Courses = grammar.Dedupe(append(x.Courses, old))
			}
		}
		if found != nil {
			asides = slices.DeleteFunc(asides, func(a string) bool { return strings.EqualFold(rawWords(a), "formerly "+old) })
		}
	}
	return asides
}

// stripAsides removes the asides from item text s and returns them as notes,
// or nil when it has none.
func stripAsides(s string) (string, []string) {
	var notes []string
	// "<codes> (History & Society courses) and <codes> (...)": one pool
	s = categoryAndRe.ReplaceAllStringFunc(s, func(p string) string {
		notes = append(notes, grammar.CleanNote(strings.TrimSpace(categoryAndRe.FindStringSubmatch(p)[1])))
		return ", "
	})
	t := asideParenRe.ReplaceAllStringFunc(s, func(p string) string {
		inner := strings.Trim(strings.TrimSpace(p[1:len(p)-1]), "_ ")
		notes = append(notes, grammar.CleanNote(Clean(inner)))
		return ""
	})
	if loc := appositiveRe.FindStringIndex(strings.TrimSpace(t)); loc != nil {
		u := strings.TrimSpace(t)
		notes = append(notes, grammar.CleanNote(strings.TrimPrefix(u[loc[0]:], ",")))
		t = u[:loc[0]]
	}
	if notes == nil {
		return s, nil
	}
	return strings.TrimSpace(strings.Join(strings.Fields(t), " ")), notes
}

// ofAnyFollowingRe matches "1.5 credits of any of the following (...):".
var ofAnyFollowingRe = regexp.MustCompile(`(?i)(` + grammar.NumPat + `\s+credits?)\s+of\s+any\s+of\s+the\s+following\s*(\([^()]*\))?\s*:`)

// topicParenRe matches a course and the bracket after it: "TRN409H1
// (Nuclear Weapons and International Politics)".
var topicParenRe = regexp.MustCompile(`(` + grammar.CodePat + `)\s*\(([^()]+)\)`)

// topicCodes returns the courses s lists with two or more different topics,
// each mapped to its mentions: a special-topics course counts only on those
// topics.
func topicCodes(s string) map[string][]string {
	topics := map[string][]string{}
	for _, m := range topicParenRe.FindAllStringSubmatch(s, -1) {
		if titleWordsRe.MatchString(strings.TrimSpace(m[2])) && !slices.Contains(topics[m[1]], m[0]) {
			topics[m[1]] = append(topics[m[1]], m[0])
		}
	}
	for code, ms := range topics {
		if len(ms) < 2 {
			delete(topics, code)
		}
	}
	return topics
}

// topicMembers makes a special-topics course a maybe: a course becomes
// and(course, topic text), a pool or(pool without it, and(pool, text)). The
// text is content: the offering's topic decides.
func topicMembers(n *types.ReqNode, topics map[string][]string) *types.ReqNode {
	if n == nil || len(topics) == 0 {
		return n
	}
	topicText := func(codes []string) *types.ReqNode {
		var ms []string
		for _, c := range codes {
			ms = append(ms, topics[c]...)
		}
		return TextNode(types.TextContent, strings.Join(ms, ", "))
	}
	switch n.Type {
	case types.NodeCourse:
		if topics[n.Code] != nil {
			return programs.AndNode(n, topicText([]string{n.Code}))
		}
	case types.NodeCredits:
		var hit []string
		for _, c := range n.Courses {
			if topics[c] != nil {
				hit = append(hit, c)
			}
		}
		if hit == nil || n.Items != nil {
			return n
		}
		strict := grammar.CloneNode(n)
		strict.Courses = slices.DeleteFunc(slices.Clone(n.Courses), func(c string) bool { return topics[c] != nil })
		return programs.OrNode(strict, programs.AndNode(n, topicText(hit)))
	case types.NodeAnd, types.NodeOr, types.NodeChoose:
		for i, it := range n.Items {
			n.Items[i] = topicMembers(it, topics)
		}
	}
	return n
}

// ieLeadRe matches the "i.e.," before a restatement.
var ieLeadRe = regexp.MustCompile(`(?i)^i\.e\.,?\s*`)

// restatedLevels narrows the pool of body n to the levels its "i.e."
// restatement gives, when the restatement splits the pool's amount into parts
// of its designators, each with levels.
func restatedLevels(n *types.ReqNode, restated string) {
	var pool *types.ReqNode
	switch {
	case n == nil:
		return
	case n.Type == types.NodeCredits:
		pool = n
	case n.Type == types.NodeAnd:
		for _, it := range n.Items {
			if it.Type == types.NodeCredits && it.MaxAmount == nil && (pool == nil || it.Amount > pool.Amount) {
				pool = it
			}
		}
	}
	if pool == nil || pool.Levels != nil || pool.Courses != nil || pool.Items != nil {
		return
	}
	r, _ := parseCompletionText(restated)
	if r == nil {
		return
	}
	parts := []*types.ReqNode{r}
	if r.Type == types.NodeAnd {
		parts = r.Items
	}
	sum := 0.0
	var levels []int
	for _, p := range parts {
		if p.Type != types.NodeCredits || p.Levels == nil || p.Courses != nil || p.Items != nil || p.MaxAmount != nil ||
			!grammar.SameSet(p.Designators, pool.Designators) {
			return
		}
		sum += p.Amount
		levels = append(levels, p.Levels...)
	}
	if math.Abs(sum-pool.Amount) > 1e-9 {
		return
	}
	levels = grammar.Dedupe(levels)
	slices.Sort(levels)
	pool.Levels = levels
}

// permListRe matches an amount from a list, or with permission from a
// second list: "1.0 credit from: A/ B or with permission of the Director: C/ D".
var permListRe = regexp.MustCompile(`(?i)^(.*?\bfrom:?\s+)((?:` + grammar.CodePat + `\s*[/,;]?\s*)*` + grammar.CodePat + `)\s*,?\s+or\s+((?:with|by)\s+(?:the\s+)?(?:prior\s+)?(?:written\s+)?(?:permission|approval)\s+(?:of|from)\s+[^:]+?)\s*:\s*((?:` + grammar.CodePat + `\s*[/,;]?\s*)*` + grammar.CodePat + `)\.?$`)

// permissionList builds or(credits{amount, list}, and(credits{amount, both
// lists}, permission text)) from a permListRe match. A strict part that
// isn't a plain pool of courses makes the whole sentence text.
func permissionList(m []string) *types.ReqNode {
	strict, _ := parseCompletionText(m[1] + m[2])
	if !isPool(strict) || strict.Courses == nil || strict.Designators != nil || strict.Levels != nil || strict.MaxAmount != nil {
		return TextNode(types.TextOther, m[0])
	}
	wide := grammar.CloneNode(strict)
	wide.Courses = grammar.Dedupe(append(slices.Clone(strict.Courses), grammar.CodeRe.FindAllString(m[4], -1)...))
	return programs.OrNode(strict, programs.AndNode(wide, TextNode(types.TextPermission, m[3])))
}

// withAverageRe matches "( A and B and C) with an average grade of 63%".
var withAverageRe = regexp.MustCompile(`(?i)^\(?\s*((?:` + grammar.CodePat + `\s*(?:,|and)?\s*)+)\)?\s+with\s+an\s+average\s+(?:grade\s+)?of\s+(?:` + grammar.AtLeastPat + `\s+)?(\d{2})\s*%$`)

// anyRemainingRe matches "<pool>, or any remaining <filter> not used
// toward other program requirements - <codes> (<approval for more codes>)".
var anyRemainingRe = regexp.MustCompile(`(?i)^(.+?),?\s+or\s+any\s+remaining\s+[^-–]+?\s+[-–]\s+((?:\(?\s*` + grammar.CodePat + `\s*\)?\s*[,/]?\s*)+)\s*(?:\((.*)\))?$`)

// orEquivalentRe matches "<strict>, or an equivalent <wider>, approved by
// <whom>".
var orEquivalentRe = regexp.MustCompile(`(?i)^(.+?),?\s+or\s+an\s+equivalent\s+(.+?),\s+((?:approved\s+by|with\s+(?:the\s+)?approval\s+of)\s+.+)$`)

var (
	// lowerRemarkRe matches a lowercase bracketed remark of advice or
	// timing: "(should be taken in first year)".
	lowerRemarkRe = regexp.MustCompile(`\s*\(((?:should|ideally|normally|preferably|recommended|strongly|highly|best|to\s+be\s+taken)\b[^()]*)\)`)
	// timingRe matches when to take a course: "in first year".
	timingRe = regexp.MustCompile(`(?i)\b(?:should|to)\s+be\s+taken\b|\bin\s+(?:the\s+)?(?:first|second|third|fourth)\s+year\b`)
)

// andOrApprovalRe matches "A/ B and/or C/ D <qualified, with approval>".
var andOrApprovalRe = regexp.MustCompile(`^((?:` + grammar.CodePat + `\s*/\s*)*` + grammar.CodePat + `)\s+and/or\s+((?:` + grammar.CodePat + `\s*/\s*)*` + grammar.CodePat + `)\s+([^;]*\b(?:approval|permission)\b[^;]*)$`)

// orAnotherRe matches an approved alternative closing an item's list:
// ", or another course ... approved by the Undergraduate Chair".
var orAnotherRe = regexp.MustCompile(`(?i),?\s+(?:or|and)\s+((?:another|any)\s+[^;]*?\b(?:approved\s+by|by\s+permission|with\s+(?:the\s+)?(?:prior\s+)?(?:approval|permission)|at\s+the\s+discretion\s+of|as\s+determined\s+by)\b[^;]*?)\.?$`)

// checkedBody returns n, or s as one text node when the grammar's reading
// can't be trusted, since a wrong structure is worse than text: a union pool,
// text that swallowed a code, and the misreadings marked below.
func checkedBody(n *types.ReqNode, s string) *types.ReqNode {
	if n == nil {
		return nil
	}
	bad, read := false, false
	grammar.WalkReq(n, func(x *types.ReqNode) {
		switch {
		case x.Type == types.NodeCredits && len(x.Items) > 0, x.Type == types.NodeText && grammar.CodeRe.MatchString(x.Text):
			bad = true
		case x.Type != types.NodeText && x.Type != types.NodeAnd && x.Type != types.NodeOr:
			read = true
		}
	})
	grammar.WalkReq(n, func(x *types.ReqNode) {
		if x.Type != types.NodeAnd {
			return
		}
		switch {
		case slices.ContainsFunc(x.Items, isAnyCredits) &&
			slices.ContainsFunc(x.Items, func(y *types.ReqNode) bool { return y.Type == types.NodeCourse || y.Type == types.NodeText }):
			bad = true
		case slices.ContainsFunc(x.Items, hasCourses) && slices.ContainsFunc(x.Items, func(y *types.ReqNode) bool {
			return y.Type == types.NodeText && y.Kind == types.TextOther && proseRe.MatchString(y.Text)
		}):
			// "Students should note that completing ( A and B) ...": not required
			bad = true
		case slices.ContainsFunc(x.Items, func(y *types.ReqNode) bool {
			return y.Type == types.NodeCredits && y.MaxAmount != nil && y.Courses == nil && y.Designators == nil && y.Levels == nil
		}):
			// a cap that lost what it caps
			bad = true
		case slices.ContainsFunc(x.Items, func(y *types.ReqNode) bool { return y.Type == types.NodeCourse }) &&
			slices.ContainsFunc(x.Items, func(y *types.ReqNode) bool {
				return y.Type == types.NodeOr && slices.ContainsFunc(y.Items, func(z *types.ReqNode) bool {
					return z.Type == types.NodeText && z.Kind == types.TextPermission
				})
			}):
			// "1.0 credit from: A/ B or with permission ...: C/ D" cut into (A/ B or
			// permission) and C, which one 0.5 course would meet
			bad = true
		case len(x.Items) >= 3 && isPool(x.Items[0]) && x.Items[0].Courses != nil && x.Items[0].Designators == nil &&
			!slices.ContainsFunc(x.Items[1:], func(y *types.ReqNode) bool { return y.Type != types.NodeCourse }) &&
			len(amountRe.FindAllString(s, -1)) == 1:
			// "2.0 credits from <codes> and <codes>" read as a pool and required ones
			bad = true
		case slices.ContainsFunc(x.Items, func(y *types.ReqNode) bool { return y.Type == types.NodeCredits && y.Courses != nil }) &&
			slices.ContainsFunc(x.Items, func(y *types.ReqNode) bool { return y.Type == types.NodeText }) &&
			len(slices.DeleteFunc(slices.Clone(x.Items), func(y *types.ReqNode) bool { return y.Type != types.NodeCourse })) >= 2:
			// a pool cut short by a remark, its last members read as required
			bad = true
		}
	})
	grammar.WalkReq(n, func(x *types.ReqNode) {
		// an empty credits node: a phrase half read ("normally taken in first year")
		bad = bad || x.Type == types.NodeCredits && x.Amount == 0 && x.MaxAmount == nil && x.Items == nil && x.Courses == nil &&
			x.Designators == nil && x.Levels == nil && x.Breadth == nil && x.Lists == nil
	})
	grammar.WalkReq(n, func(x *types.ReqNode) {
		// "1.0 credit in ONE other discipline": a word in capitals, not a designator
		bad = bad || x.Type == types.NodeCredits && slices.ContainsFunc(x.Designators, func(d string) bool { return capsWords[d] })
	})
	grammar.WalkReq(n, func(x *types.ReqNode) {
		// "3.5 credits of ESS core courses": the designator names a list
		if x.Type != types.NodeAnd {
			return
		}
		bad = bad || slices.ContainsFunc(x.Items, func(y *types.ReqNode) bool {
			return y.Type == types.NodeCredits && y.Designators != nil && y.Courses == nil
		}) && slices.ContainsFunc(x.Items, func(y *types.ReqNode) bool {
			return y.Type == types.NodeText && y.Kind == types.TextOther && listNounRe.MatchString(y.Text)
		})
	})
	grammar.WalkReq(n, func(x *types.ReqNode) {
		// listed courses alone can't make up the amount: the rest is a list or text
		if x.Type == types.NodeCredits && x.Amount > 0 && x.Courses != nil && x.Designators == nil && x.Levels == nil &&
			x.Lists == nil && x.Breadth == nil && x.Items == nil && x.Requirements == nil {
			sum := 0.0
			for _, c := range grammar.Dedupe(x.Courses) {
				sum += grammar.CourseWeight(c)
			}
			bad = bad || sum < x.Amount-1e-9
		}
	})
	if capLeadRe.MatchString(strings.TrimSpace(s)) {
		// "Up to 1.0 credit may be taken from list 2": a cap is no body
		bad = true
	}
	grammar.WalkReq(n, func(x *types.ReqNode) {
		// "...; fulfilled by any 400-level courses listed in ...": fragments
		bad = bad || x.Type == types.NodeText && x.Kind == types.TextOther && fragmentRe.MatchString(x.Text)
	})
	if wordSlashRe.MatchString(s) {
		// "disciplinary/thematic": a "/" between words offers no alternatives
		grammar.WalkReq(n, func(x *types.ReqNode) {
			bad = bad || x.Type == types.NodeOr && slices.ContainsFunc(x.Items, func(y *types.ReqNode) bool { return y.Type == types.NodeText })
		})
	}
	if m := leadAmountRe.FindStringSubmatch(s); m != nil && n.Type == types.NodeAnd &&
		!slices.ContainsFunc(n.Items, func(y *types.ReqNode) bool { return y.Type != types.NodeCourse }) {
		// "1.0 credit: A, B, C" read as three required courses worth 1.5
		amount, _ := strconv.ParseFloat(m[1], 64)
		sum := 0.0
		for _, it := range n.Items {
			sum += grammar.CourseWeight(it.Code)
		}
		bad = bad || sum > amount+1e-9
	}
	if m := desigAtLevelOrRe.FindStringSubmatch(s); m != nil &&
		!grammar.SameSet(grammar.ParseLevels(m[2]), grammar.ParseLevels(m[4])) && !grammar.SameSet(grammar.DesigRe.FindAllString(m[1], -1), grammar.DesigRe.FindAllString(m[3], -1)) {
		// "FRE at the 200+ level or FSL at the 300+ level": two filters
		bad = true
	}
	if m := filterOrFilterRe.FindStringSubmatch(s); m != nil &&
		!grammar.SameSet(grammar.ParseLevels(m[1]), grammar.ParseLevels(m[3])) && !grammar.SameSet(grammar.DesigRe.FindAllString(m[2], -1), grammar.DesigRe.FindAllString(m[4], -1)) {
		// "100- or 200-level ESS courses or 200-level JEG courses": a union one
		// filter can't hold; merged, it would count JEG100H1
		bad = true
	}
	texts := 0
	grammar.WalkReq(n, func(x *types.ReqNode) {
		if x.Type == types.NodeText && x.Kind == types.TextOther {
			texts++
		}
	})
	switch {
	case bad || texts >= 3:
		// three fragments of text: the grammar read a sentence, not a list
		return TextNode(types.TextOther, s)
	case !read && (n.Type == types.NodeAnd || n.Type == types.NodeOr):
		return TextNode(grammar.Classify(grammar.CleanNote(s)), s)
	}
	return n
}

// wordSlashRe matches two lowercase words joined by "/":
// "disciplinary/thematic".
var wordSlashRe = regexp.MustCompile(`\b[a-z]{3,}/[a-z]{3,}\b`)

// fragmentRe matches text the grammar cut from a sentence, not a
// condition of its own: "fulfilled by".
var fragmentRe = regexp.MustCompile(`(?i)^(?:fulfilled\s+by|listed\s+(?:in|below|above)\b|both\s+of\b|one\s+or\s+both\b|plus\b|[A-D]-level\b|on\s+the\b|disciplinary/thematic\b)`)

// leadAmountRe matches the amount a requirement starts with: "1.0 credit".
var leadAmountRe = regexp.MustCompile(`(?i)^(?:[^:]{0,60}:\s*)?` + grammar.AmountPat + `\s+credits?\b`)

// capsWords are words written in capitals that aren't designators.
var capsWords = map[string]bool{"ONE": true, "TWO": true, "ALL": true, "ANY": true, "AND": true, "THE": true, "NOT": true,
	"FOR": true, "BUT": true, "MAY": true, "OUT": true, "SIX": true, "TEN": true, "PER": true, "TBA": true}

// listNounRe matches text that names a list beside a designator: "core
// courses".
var listNounRe = regexp.MustCompile(`(?i)^(?:other\s+)?(?:[a-z&-]+\s+){0,3}(?:courses?|concentrations?|disciplines?|streams?|areas?|groups?|lists?)\b`)

// filterOrFilterRe matches "<levels> <designators> courses or <levels>
// <designators> courses": "200-level ESS courses or 300-level JEG courses".
var filterOrFilterRe = regexp.MustCompile(`(?i)\b(` + grammar.LevelPat + `(?:\s*[-/,]?\s*(?:or|and)?\s*` + grammar.LevelPat + `)*[-+]?)\s*-?\s*levels?\s+(` + grammar.DesigPat + `(?:\s*(?:/|,|or|and)\s*` + grammar.DesigPat + `)*)\s+courses?\s+or\s+(` + grammar.LevelPat + `(?:\s*[-/,]?\s*(?:or|and)?\s*` + grammar.LevelPat + `)*[-+]?)\s*-?\s*levels?\s+(` + grammar.DesigPat + `(?:\s*(?:/|,|or|and)\s*` + grammar.DesigPat + `)*)\s+courses?\b`)

// desigAtLevelOrRe matches "<designators> at the <levels> or
// <designators> at the <levels>": "FRE at the 200 level or FSL at the 300 level".
var desigAtLevelOrRe = regexp.MustCompile(`\b((?:[A-Z]{3})(?:\s*/\s*[A-Z]{3})*)\s+(?:(?i:courses?|credits?)\s+)?(?i:at\s+the)\s+(` + grammar.LevelPat + `[-+/\s\d]*(?i:levels?)?)\s+(?i:or)\s+((?:[A-Z]{3})(?:\s*/\s*[A-Z]{3})*)\s+(?:(?i:courses?|credits?)\s+)?(?i:at\s+the)\s+(` + grammar.LevelPat + `[-+/\s\d]*(?i:levels?)?)`)

func hasCourses(n *types.ReqNode) bool {
	found := false
	grammar.WalkReq(n, func(x *types.ReqNode) { found = found || x.Type == types.NodeCourse || x.Type == types.NodeCredits })
	return found
}

// proseRe matches a verb, which makes text around courses a sentence
// about them: "should".
var proseRe = regexp.MustCompile(`(?i)\b(?:should|must|may|can|will|note|limits?|is|are|was|were|have|has)\b`)

// substitutedFromRe matches a capped share from named courses: "A
// maximum of 0.5 credit may be substituted from the following first-year
// courses: POL191H1".
var substitutedFromRe = regexp.MustCompile(`(?i)^(?:a\s+maximum\s+of|up\s+to|` + grammar.NoMoreThanPat + `)\s+(?:a\s+)?` + grammar.AmountPat + `\s+credits?\s+(?:of\s+this\s+requirement\s+)?(?:may|can)\s+(?:also\s+)?be\s+(?:substituted|taken|chosen|satisfied|completed|fulfilled)\s+(?:from|by\s+taking)\s+(?:(?:one|any)\s+of\s+)?(?:the\s+following[^:]*:\s*)?((?:` + grammar.CodePat + `\s*[,/;]?\s*(?:and|or)?\s*)+)$`)

// widenPool returns pool n widened with the courses of substitutedFromRe
// clause s, and the limit capping them. The limit is nil when s isn't one, or
// n isn't a pool of listed courses whose levels the courses fit.
func widenPool(n *types.ReqNode, s string) (*types.ReqNode, *types.Requirement) {
	m := substitutedFromRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(Clean(s)), "."))
	if m == nil || n == nil || n.Type != types.NodeCredits || n.Courses == nil || n.Items != nil {
		return n, nil
	}
	codes := grammar.CodeRe.FindAllString(m[2], -1)
	if n.Levels != nil && slices.ContainsFunc(codes, func(c string) bool { return !grammar.LevelMatches(n.Levels, c) }) {
		return n, nil
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	w := grammar.CloneNode(n)
	w.Courses = grammar.Dedupe(append(slices.Clone(n.Courses), codes...))
	return w, &types.Requirement{Kind: types.RequirementLimit, Raw: s,
		Req: &types.ReqNode{Type: types.NodeCredits, Courses: grammar.Dedupe(codes), MaxAmount: grammar.MaxAmount(v)}}
}

// requiredSentenceRe matches courses a sentence says are required:
// "PHL271H1, TRN203H1, and TRN204H1 are required courses".
var requiredSentenceRe = regexp.MustCompile(`(?i)^(.*\b` + grammar.CodePat + `)\s+(?:are|is)\s+(?:the\s+)?required(?:\s+courses?)?\b`)

// onlySomeRe matches a requirement only some students have: "for
// students who began".
var onlySomeRe = regexp.MustCompile(`(?i)\b(?:for\s+(?:students|those)|if|unless|when|only\s+for|in\s+case)\b`)

// thatRe matches the "that" before advice after requiredSentenceRe.
var thatRe = regexp.MustCompile(`(?i)^\s*,?\s*(?:that|which)\s+`)

func fixedCourses(n *types.ReqNode) bool {
	if n.Type == types.NodeCourse {
		return true
	}
	return n.Type == types.NodeAnd && !slices.ContainsFunc(n.Items, func(it *types.ReqNode) bool { return it.Type != types.NodeCourse })
}

var (
	// codeSeeNoteRe matches a note pointer after a course: "JPM400Y1 (see
	// NOTE 3)".
	codeSeeNoteRe = regexp.MustCompile(`(?i)(` + grammar.CodePat + `)\s*\((see\s+note\s+\d+)\)`)
	// codeFormerlyRe matches a course's old code, which counts too:
	// "SMC366H1 (_formerly SMC218Y1_)".
	codeFormerlyRe = regexp.MustCompile(`(` + grammar.CodePat + `)\s*\(\s*_?formerly\s+(` + grammar.CodePat + `)_?\s*\)`)
	// remarkAdviceRe matches a trailing remark that is advice: "(The
	// courses ... are recommended.)".
	remarkAdviceRe = regexp.MustCompile(`(?i)^note\s+that\b|\b(?:is|are)\s+recommended\.?$`)
	// remarkCanBeUsedRe matches a cap on the course before it: "(0.5 credit
	// can be used based on internship)".
	remarkCanBeUsedRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\s+can\s+be\s+used\b[^()]*$`)
	// remarkConsideredRe matches courses that join the pool with approval:
	// "( STA399H1 and STA399Y1 may be considered on a case by case basis with the
	// approval of the Associate Chair)".
	remarkConsideredRe = regexp.MustCompile(`(?i)^((?:` + grammar.CodePat + `)(?:\s*(?:,|and|or)\s*` + grammar.CodePat + `)*)\s+may\s+be\s+considered\b.*\bapproval\b`)
	// remarkOnlyIfRe matches "( PHL245H1 may only be counted here if (
	// MAT158H1, MAT159H1/ MAT157Y1) is not taken)".
	remarkOnlyIfRe = regexp.MustCompile(`(?i)^(` + grammar.CodePat + `)\s+may\s+only\s+be\s+counted\s+here\s+if\s+(.+?)\s+is\s+not\s+taken$`)
	// amountFromCodesRe matches an amount from a list of courses after a
	// description: "0.5 credit in biological diversity and function from:
	// BIO251H1, BIO270H1; EEB266H1".
	amountFromCodesRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\s+(?:(?:in\s+[a-z][a-z\s]+?\s+)?(?:from|of)(?:\s+the\s+following(?:\s+\([^()]*\))?)?|from\s+remaining\s+[^:]*?\bnot\s+used\s+toward\s+other\s+program\s+requirements\s+in\s+the\s+following\s+list):?\s*(` + grammar.CodePat + `(?:\s*(?:,|;|/|or)\s*` + grammar.CodePat + `)*)$`)
	// floorFromDesignatorRe matches a pool with a designator floor: "2.5
	// credits at least 1.5 credits from PCL courses: JPM300H1/ ...". The groups
	// are a course before it, the amount, the floor, the designator, the courses.
	floorFromDesignatorRe = regexp.MustCompile(`^(?:[A-Za-z][A-Za-z ]*:\s*)?(?:(` + grammar.CodePat + `)\s*,?\s+and\s+)?` + grammar.AmountPat + `\s+credits,?\s+(?:with\s+)?` + grammar.AtLeastPat + `\s+` + grammar.AmountPat + `\s+credits?\s+from\s+([A-Z]{3})\s+courses:\s*(` + grammar.CodePat + `(?:\s*(?:/|or)\s*` + grammar.CodePat + `)*)$`)
	// notUsedFromRe matches "Any course (0.5 credit) not used to satisfy
	// first year course requirement from JEG100H1, ...": exclusive allocation
	// already keeps a course from counting twice.
	notUsedFromRe = regexp.MustCompile(`(?i)^any\s+course\s+\(` + grammar.AmountPat + `\s+credits?\)\s+not\s+used\s+to\s+satisfy\s+[\w\s]+?\s+requirements?\s+from\s+(.+)$`)
	// otherNotTakenRe matches the choice again: "The other of CHM210H1 or
	// ESS262H1/ JEG100H1 not taken in second year" (ASMAJ1076). Exclusive
	// allocation keeps the earlier row's course from counting.
	otherNotTakenRe = regexp.MustCompile(`(?i)^the\s+other\s+of\s+(.+?)\s+not\s+taken\s+in\s+(?:first|second|third|fourth)\s+year$`)
	// upToByCourseRe matches an amount that depends on a course taken: "Up
	// to 1.5 credits from the following courses, ... Students who complete ANT203Y1
	// need to take 1.0 credit, ..." (ASMIN1778).
	upToByCourseRe = regexp.MustCompile(`(?i)^(?:\d{1,2}\.\s+)?up\s+to\s+` + grammar.AmountPat + `\s+credits\s+from\s+the\s+following\s+courses,\s+up\s+to\s+a\s+total\s+of\s+` + grammar.NumPat + `\s+credits\s+to\s+complete\s+the\s+\w+\s+program\.\s+students\s+who\s+complete\s+(` + grammar.CodePat + `)\s+need\s+to\s+take\s+` + grammar.AmountPat + `\s+credits?,\s+and\s+students\s+who\s+take\s+(` + grammar.CodePat + `(?:\s+or\s+` + grammar.CodePat + `)*)\s+need\s+to\s+take\s+` + grammar.AmountPat + `\s+credits,\s+from\s+the\s+following:\s*(.+)$`)
	// willCountRe matches "... Special Topics courses at the 300 or 400
	// level, ... will count towards program requirements". remainingAnyRe
	// matches "The remaining 1.5 - 2.0 credits can include any courses ...".
	willCountRe    = regexp.MustCompile(`(?i)^[A-Z][\w -]*\bcourses\b.*\bwill\s+count\s+towards?\s+program\s+requirements$`)
	remainingAnyRe = regexp.MustCompile(`(?i)^(?:\d{1,2}\.\s+)?the\s+remaining\s+` + grammar.AmountPat + `\s*[-–]\s*` + grammar.NumPat + `\s+credits\s+can\s+include\s+any\s+courses\s+not\s+previously\s+taken\b[^,:]*,\s*or\s+any\s+of\s+the\s+following:\s*(.+)$`)
)

// trailingParen splits s, when it ends with a bracket group, into the text
// before it and the group's inside.
func trailingParen(s string) (head, inner string, ok bool) {
	if !strings.HasSuffix(s, ")") {
		return "", "", false
	}
	depth := 0
	for i := len(s) - 1; i >= 0; i-- {
		switch s[i] {
		case ')':
			depth++
		case '(':
			depth--
			if depth == 0 {
				return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1 : len(s)-1]), true
			}
		}
	}
	return "", "", false
}

// remarkItem takes a remark out of an item body the grammar leaves as text,
// reads the rest as the body, and gives the remark its own place: a note, a
// limit, a bounded route or an equivalent course. It reads nothing unless the
// rest reads with no text.
func (a *Assembler) remarkItem(s string) (body *types.ReqNode, rows []*types.Requirement, notes []string, ok bool) {
	t := strings.TrimSuffix(strings.TrimSpace(Clean(s)), ".")
	if m := remainingAnyRe.FindStringSubmatch(t); m != nil && IsCodeList(m[2]) {
		v, _ := strconv.ParseFloat(m[1], 64)
		// the courses listed are among any: a note
		return &types.ReqNode{Type: types.NodeCredits, Amount: v}, nil, []string{grammar.CleanNote("Or any of the following: " + m[2])}, true
	}
	if m := otherNotTakenRe.FindStringSubmatch(t); m != nil {
		if n, more, more2 := a.grammarBody(m[1]); n != nil && !hasText(n) && len(more) == 0 && len(more2) == 0 {
			return n, nil, nil, true
		}
	}
	if m := notUsedFromRe.FindStringSubmatch(t); m != nil {
		if n, caps := codesCredits(m[1], m[2]); n != nil {
			return n, caps, nil, true
		}
	}
	changed := false
	if codeSeeNoteRe.MatchString(t) {
		t, changed = codeSeeNoteRe.ReplaceAllString(t, "$1"), true
	}
	if codeFormerlyRe.MatchString(t) {
		t, changed = codeFormerlyRe.ReplaceAllString(t, "$1/ $2"), true
	}
	head, inner, paren := trailingParen(t)
	var after func(n *types.ReqNode) *types.ReqNode
	switch {
	case !paren:
	case remarkAdviceRe.MatchString(inner):
		t, changed = head, true
		notes = append(notes, grammar.CleanNote(inner))
	case remarkCanBeUsedRe.MatchString(inner) && codeAtEnd(head) != "":
		v, _ := strconv.ParseFloat(remarkCanBeUsedRe.FindStringSubmatch(inner)[1], 64)
		t, changed = head, true
		rows = append(rows, &types.Requirement{Kind: types.RequirementLimit, Raw: grammar.CleanNote(inner),
			Req: &types.ReqNode{Type: types.NodeCredits, Courses: []string{codeAtEnd(head)}, MaxAmount: grammar.MaxAmount(v)}})
	case remarkConsideredRe.MatchString(inner):
		extra := grammar.CodeRe.FindAllString(remarkConsideredRe.FindStringSubmatch(inner)[1], -1)
		t, changed = head, true
		after = func(n *types.ReqNode) *types.ReqNode {
			if n.Type != types.NodeCredits || n.Courses == nil || n.Amount == 0 {
				return nil
			}
			wide := grammar.CloneNode(n)
			wide.Courses = grammar.Dedupe(append(wide.Courses, extra...))
			return programs.OrNode(n, programs.AndNode(wide, TextNode(types.TextPermission, grammar.CleanNote(inner))))
		}
	case remarkOnlyIfRe.MatchString(inner):
		m := remarkOnlyIfRe.FindStringSubmatch(inner)
		cond, _ := parseCompletionText(m[2])
		if cond == nil || hasText(cond) {
			return nil, nil, nil, false
		}
		t, changed = head, true
		after = func(n *types.ReqNode) *types.ReqNode {
			if n.Type != types.NodeCredits || !slices.Contains(n.Courses, m[1]) || n.Amount == 0 {
				return nil
			}
			without := grammar.CloneNode(n)
			without.Courses = slices.DeleteFunc(slices.Clone(n.Courses), func(c string) bool { return c == m[1] })
			return programs.OrNode(without, programs.AndNode(n, &types.ReqNode{Type: types.NodeNot, Items: []*types.ReqNode{cond}}))
		}
	}
	if !changed {
		return nil, nil, nil, false
	}
	body, more, more2 := a.grammarBody(t)
	if body == nil || hasText(body) {
		body, more, more2 = nil, nil, nil
		if m := amountFromCodesRe.FindStringSubmatch(t); m != nil {
			body, more = codesCredits(m[1], m[2])
		}
		if body == nil {
			return nil, nil, nil, false
		}
	}
	for _, r := range more {
		if r.Raw == t {
			r.Raw = strings.TrimSpace(s) // the remark is the row's too
		}
	}
	if after != nil {
		if body = after(body); body == nil {
			return nil, nil, nil, false
		}
	}
	return body, append(rows, more...), append(notes, more2...), true
}

// listSepRe matches a list separator: ";".
var listSepRe = regexp.MustCompile(`[,;]`)

// codesCredits reads "N credit(s) from <courses>" as credits{N, the courses},
// or nil. A "/" choice inside a "," or ";" list gets a limit of its heaviest
// course's weight, when the choice can matter.
func codesCredits(amount, list string) (n *types.ReqNode, caps []*types.Requirement) {
	list = strings.TrimSuffix(strings.TrimSpace(list), ".")
	if !IsCodeList(list) {
		return nil, nil
	}
	v, _ := strconv.ParseFloat(amount, 64)
	if strings.Contains(list, "/") && (strings.Contains(list, ",") || strings.Contains(list, ";")) {
		for _, part := range listSepRe.Split(list, -1) {
			codes := grammar.CodeRe.FindAllString(part, -1)
			if len(codes) < 2 {
				continue
			}
			w := 0.0
			for _, c := range codes {
				w = max(w, grammar.CourseWeight(c))
			}
			if w < v {
				caps = append(caps, &types.Requirement{Kind: types.RequirementLimit, Raw: strings.TrimSpace(part),
					Req: &types.ReqNode{Type: types.NodeCredits, Courses: codes, MaxAmount: grammar.MaxAmount(w)}})
			}
		}
	}
	return &types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: grammar.Dedupe(grammar.CodeRe.FindAllString(list, -1))}, caps
}

// codeAtEndRe matches the course a text ends with: "... or MAT137Y1".
var codeAtEndRe = regexp.MustCompile(`(` + grammar.CodePat + `)$`)

// codeAtEnd returns the course s ends with, or "".
func codeAtEnd(s string) string {
	m := codeAtEndRe.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	return m[1]
}

// floorFromDesignator reads floorFromDesignatorRe as credits{N, the
// list} and a constraint of the floor on the list's courses of the designator
// and joint (J) ones. before the grammar, which reads the floor as a pool
func floorFromDesignator(s string) (body *types.ReqNode, rows []*types.Requirement, ok bool) {
	t := codeSeeNoteRe.ReplaceAllString(strings.TrimSuffix(strings.TrimSpace(Clean(s)), "."), "$1")
	if m := floorFromDesignatorRe.FindStringSubmatch(t); m != nil {
		v, _ := strconv.ParseFloat(m[2], 64)
		floor, _ := strconv.ParseFloat(m[3], 64)
		codes := grammar.Dedupe(grammar.CodeRe.FindAllString(m[5], -1))
		var own []string
		for _, c := range codes {
			if c[:3] == m[4] || c[0] == 'J' {
				own = append(own, c)
			}
		}
		if floor >= v || weightOf(own) < floor {
			return nil, nil, false
		}
		pool := &types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: codes}
		body = pool
		if m[1] != "" {
			body = programs.AndNode(&types.ReqNode{Type: types.NodeCourse, Code: m[1]}, pool)
		}
		rows = []*types.Requirement{{Kind: types.RequirementConstraint, Raw: grammar.CleanNote(t),
			Req: &types.ReqNode{Type: types.NodeCredits, Amount: floor, Courses: own}}}
		return body, rows, true
	}
	return nil, nil, false
}
