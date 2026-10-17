package reqs

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"degree-explorer/types"
)

// Sentences into rows: what each role does with a sentence, an item's body
// and the rules written in it.

// sentence places one sentence of item row (nil at the program level) and
// returns the notes on it. The first requirement sentence is row's body.
func (a *compAssembler) sentence(s string, row *compRow, ctx compCtx) (notes []string) {
	defer func() { a.lastSentence = s }()
	return a.sentenceAs(a.role(s), s, row, ctx)
}

// complThisRequirementRe matches an alternative pointing at the rule just
// stated: "will also meet this requirement".
var complThisRequirementRe = regexp.MustCompile(`(?i)\bwill\s+also\s+meet\s+this\s+requirement\b`)

// lastRule returns the constraint row the previous sentence added under row,
// or nil.
func (a *compAssembler) lastRule(row *compRow) *compRow {
	if row == nil || len(row.children) == 0 {
		return nil
	}
	k := row.children[len(row.children)-1]
	if k.r.Kind != types.RequirementConstraint || k.r.Req == nil || rawWords(k.r.Raw) != rawWords(a.lastSentence) {
		return nil
	}
	return k
}

// sentenceAs is sentence with the role already decided.
func (a *compAssembler) sentenceAs(role sentenceRole, s string, row *compRow, ctx compCtx) (notes []string) {
	// rules go on the item once it has a body, else on the item around it
	target := ctx.owner
	bodied := row != nil && (row.r.Req != nil || len(row.children) > 0)
	if bodied {
		target = row
	}
	if r := courseCountCap(a.clean(s)); r != nil {
		// a cap counted in courses is the program's, whatever else s says
		r.Raw = s
		a.rowUnder(nil, r)
	}
	switch role {
	case roleAdvisory:
		return []string{cleanNote(s)}
	case roleExclusion:
		a.exclusion(s)
	case roleListDef:
		if row != nil && ctx.numbered && !bodied && !complListLabelRe.MatchString(a.clean(s)) {
			// "2. Algebra: MAT240H1, MAT247H1": a numbered item with a name.
			return a.sentenceAs(roleRequirement, s, row, ctx)
		}
		if bodied && a.itemList(row, s) {
			return nil
		}
		if complBindingWordRe.MatchString(a.clean(s)) {
			// "The remainder must be selected from ...": a rule, not just a list
			a.textRow(target, s)
			return nil
		}
		// a named list is a note, unless an item refers to it
		note := cleanNote(s)
		if title, members, ok := strings.Cut(a.clean(s), ":"); ok {
			a.listDefs = append(a.listDefs, &compListDef{title: title, members: members, note: note})
		}
		return []string{note}
	case roleConstraint:
		a.constraint(target, s)
	case roleLimit:
		a.limit(target, s)
	case roleTransfer:
		a.transfer(target, s)
	case roleCrNcr:
		a.crNcr(target, s)
	case roleOverlap:
		if !a.overlap(s) {
			a.textRow(target, s)
		}
	case roleRequirement:
		ref := a.referenced(s)
		capped := sync.OnceValue(func() *types.Requirement { return capRow(s) }) // read only if a case asks
		switch {
		case isRelaxation(s) && !(complOmitRe.MatchString(s) && a.omitCovered(s)):
			// "CSC108H1 is waived for students who ...": an alternative on its rows
			a.relaxations = append(a.relaxations, s)
		case a.satisfiesAndCounts(s):
			// "PHY479Y1 ... satisfies Requirement 3 ... and counts as ..."
		case ctx.advice && !a.isApproval(s):
			return []string{cleanNote(s)}
		case complEquivalentsRe.MatchString(a.clean(s)):
			// "... any completed RSM course equivalents": an alternative on each row
			rs, replaced := a.relaxedRows(s)
			if rs == nil {
				return []string{cleanNote(s)}
			}
			for _, r := range rs {
				a.wrapRoute(r, replaced, textNode(altKind(s), s))
				addRaw(r, s)
			}
		case !bodied && ref != nil && a.isApproval(s):
			// "... may be considered for approval for Requirement 7": a route on it
			for _, r := range ref {
				a.approval(r, s)
			}
		case !bodied && row != nil && a.isApproval(s) && (complCodeLeadRe.MatchString(a.clean(s)) || isComplAllocLead(a.clean(s))):
			// "4. CHM395Y1/ ... with prior approval": an item needing approval
			body, rows, notes := a.itemBody(s)
			row.r.Req = body
			for _, r := range rows {
				a.rowUnder(row, r)
			}
			return notes
		case bodied && a.substitutedFrom(row, s):
			// "A maximum of 0.5 credit may be substituted from the following ..."
		case complNotSubstituteRe.MatchString(a.clean(s)) && capped() != nil:
			// "MAT133Y1 is not a suitable substitute for ...": it doesn't count there
			t := target
			if r := a.itemNaming(s[complNotSubstituteRe.FindStringIndex(s)[1]:]); r != nil && !bodied {
				t = r
			}
			a.rowUnder(t, capped())
		case !bodied && ref != nil && a.isAlternative(s):
			// "... may also count towards requirement 2 ...": an alternative on it
			for _, r := range ref {
				a.alternative(r, s)
			}
		case complCapLeadRe.MatchString(a.clean(s)) && capped() != nil:
			// "Up to 1.0 credit in HPS courses at the 100-level ...": a cap
			a.rowUnder(target, capped())
		case complOmitRe.MatchString(s) && a.omitCovered(s):
			// "... may omit CSC108H1": the item already offers routes without it
		case complCampusRe.MatchString(a.clean(s)) && a.campusRule(s):
		case a.isApproval(s):
			a.approval(target, s)
		case bodied && a.isAlternative(s) && complThisRequirementRe.MatchString(s) && a.lastRule(row) != nil:
			// the route belongs to the rule stated just before, not to the item
			k := a.lastRule(row)
			k.r.Req = orNode(k.r.Req, textNode(altKind(s), s))
			addRaw(k, s)
		case bodied && a.isAlternative(s):
			a.alternative(row, s)
		case !bodied && complWidenRe.MatchString(a.clean(s)):
			// "Up to 1.0 credit may be from other departments": the owner's pool
			a.alternative(ctx.owner, s)
		case !bodied && (ctx.notes || row == nil) && a.isAlternative(s):
			// "Students may count ECO210H1 ... in lieu of a 0.5 credit ... elective"
			a.alternative(ctx.owner, s)
		case row == nil || ctx.notes && !bodied:
			// not an item, or a note: text, unless it only permits or defines
			a.looseSentence(target, s)
		case !bodied && a.appliedTo(s):
			// "The course will be applied to Requirement #3": a note on that row
		case !bodied && !ctx.numbered && looseMain(s):
			// an unnumbered line that only permits is not an item
			a.looseSentence(ctx.owner, s)
		case !bodied:
			body, rows, notes := a.itemBody(s)
			if t := a.clean(s); body != nil && body.Type == types.NodeText && !ctx.numbered && !codeRe.MatchString(t) && !complAmountRe.MatchString(t) &&
				!complInheritRe.MatchString(t) {
				// prose naming no course or amount: a route, a rule or a note
				switch {
				case body.Kind == types.TextPermission:
					a.approval(nil, s)
				case complRequirementLeadRe.MatchString(t):
					return []string{cleanNote(s)}
				case a.isAlternative(s):
					// "... will count towards program requirements": a route
					a.programRoutes = append(a.programRoutes, s)
				default:
					a.looseSentence(ctx.owner, s)
				}
				return notes
			}
			row.r.Req = body
			for _, r := range rows {
				a.rowUnder(row, r)
			}
			return notes
		default:
			// another requirement joins the item's and, unless it permits or advises
			if a.conditionalExtra(row, s) {
				return nil
			}
			if t := a.clean(s); complCapLeadRe.MatchString(t) && (a.isApproval(s) || complApprovalWordRe.MatchString(t)) {
				// "Up to 0.5 credit from GGR491Y1, ... and approval": a route
				a.alternative(row, s)
				return nil
			}
			if looseMain(s) {
				if a.isAlternative(s) || a.isApproval(s) {
					a.alternative(row, s)
					return nil
				}
				return []string{cleanNote(s)}
			}
			n, notes := parseCompletionText(s)
			if n = checkedBody(n, s); hasText(n) {
				n = textNode(types.TextOther, s)
			}
			row.r.Req = andNode(row.r.Req, n)
			return notes
		}
	}
	return nil
}

var (
	// complGluedMarkerRe matches a list marker glued to a bracket, which the
	// block reader leaves in the text: "4.(".
	complGluedMarkerRe = regexp.MustCompile(`^\d{1,2}[.)]\s*\(`)
	// complRemarkParenRe matches a bracketed remark on the item, not part of its
	// requirement: "(NOTE: PSL201Y1 is not acceptable)".
	complRemarkParenRe = regexp.MustCompile(`\s*\(((?:[A-Za-z]+:\s*)?[A-Z][^()]*\s[^()]*\s[^()]*)\)`)
)

// itemBody parses an item's first requirement sentence into its body and the
// rows written inside it, such as a bracketed floor or a course that doesn't
// count. Bracketed advice becomes notes.
func (a *compAssembler) itemBody(s string) (body *types.ReqNode, rows []*types.Requirement, notes []string) {
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
func (a *compAssembler) grammarBody(s string) (body *types.ReqNode, rows []*types.Requirement, notes []string) {
	s = complGluedMarkerRe.ReplaceAllString(strings.TrimSpace(s), "(")
	// one-off wordings, as rewriteCompletion reads them, before floors come out
	s = rewriteOneOffs(s)
	if n := profileBody(s); n != nil {
		// a learning community or a language proficiency
		return n, nil, nil
	}
	if n, rows, notes, ok := a.pairPool(s); ok {
		// single courses and pairs the item grammar leaves as text
		return n, rows, notes
	}
	if m := complRequiredSentenceRe.FindStringSubmatch(s); m != nil && !complOnlySomeRe.MatchString(m[1]) && !complOnlySomeRe.MatchString(s[len(m[0]):]) {
		// "A, B, and C are required courses that ...": the courses; the rest advice
		if n, _ := parseCompletionText(m[1]); n != nil && fixedCourses(n) {
			if advice := strings.TrimSpace(complThatRe.ReplaceAllString(s[len(m[0]):], "")); advice != "" && advice != "." {
				notes = []string{cleanNote(advice)}
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
				body, rows = textNode(types.TextOther, s0), nil
				return
			}
			body = andNode(append([]*types.ReqNode{wide}, caps...)...)
			rows = append(rows, limit)
		}
	}()
	if parts := splitTop(s, ';'); len(parts) > 1 {
		// "...; up to 1.0 credit at the 100 level will count ...": the item's cap
		var keep []string
		for i, p := range parts {
			if r := capRow(strings.TrimSpace(p)); i > 0 && r != nil && complCapLeadRe.MatchString(strings.TrimSpace(p)) {
				rows = append(rows, r)
				continue
			}
			if i > 0 && complSubstitutedFromRe.MatchString(strings.TrimSuffix(strings.TrimSpace(a.clean(p)), ".")) {
				widen = append(widen, strings.TrimSpace(p))
				continue
			}
			keep = append(keep, p)
		}
		s = strings.Join(keep, ";")
	}
	// "1.5 credits of any of the following (at least ...)": pool and floor
	s = complOfAnyFollowingRe.ReplaceAllString(s, "$1 $2 from:")
	if m := complItemFloorParenRe.FindStringSubmatch(s); m != nil {
		s = strings.TrimSpace(m[1] + " " + m[3])
		rows = append(rows, a.floorRows(m[2])...)
	}
	s = complLowerRemarkRe.ReplaceAllStringFunc(s, func(p string) string {
		// "(should be taken in first year, ...)": advice on when to take it
		inner := complLowerRemarkRe.FindStringSubmatch(p)[1]
		if len(strings.Fields(inner)) < 3 || a.role(inner) != roleAdvisory && !complTimingRe.MatchString(inner) {
			return p
		}
		notes = append(notes, cleanNote(inner))
		return ""
	})
	var alts []string
	var courseAlts []*types.ReqNode
	s = complRemarkParenRe.ReplaceAllStringFunc(s, func(p string) string {
		inner := complRemarkParenRe.FindStringSubmatch(p)[1]
		switch role := a.role(inner); {
		case complAlsoAcceptRe.MatchString(a.clean(inner)) && alsoAccepted(a.clean(inner)) != nil:
			// "(NOTE: We will also accept PSY100Y5 ...)": the courses it names
			courseAlts = append(courseAlts, alsoAccepted(a.clean(inner)))
		case role == roleRequirement && a.isAlternative(inner) && !complNegationRe.MatchString(inner):
			// another way to meet the item
			alts = append(alts, cleanNote(inner))
		case role == roleAdvisory:
			notes = append(notes, cleanNote(inner))
		case role == roleLimit || complNotCountedRe.MatchString(inner):
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
	if m := complAndOrApprovalRe.FindStringSubmatch(strings.TrimSuffix(s, ".")); m != nil {
		// "A/ B and/or C/ D ... with prior approval by the Department": the
		// approval is for the courses after "and/or"
		strict, _ := parseCompletionText(m[1])
		approved, _ := parseCompletionText(m[2])
		return orNode(strict, andNode(approved, textNode(types.TextPermission, m[3]))), rows, notes
	}
	if m := complPermListRe.FindStringSubmatch(strings.TrimSpace(s)); m != nil {
		// "1.0 credit from: A/ B or with permission of the Director: C/ D": the
		// list, or with permission the same amount from both lists
		return permissionList(m), rows, notes
	}
	if m := complWithAverageRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(s), ".")); m != nil {
		// "( A and B and C) with an average grade of 63%": courses and average
		codes := codeRe.FindAllString(m[1], -1)
		grade, _ := strconv.Atoi(m[2])
		items := courseNodes(codes)
		items = append(items, &types.ReqNode{Type: types.NodeAverage, MinGrade: grade, Courses: codes})
		return andNode(items...), rows, notes
	}
	if m := complAnyRemainingRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(s), ".")); m != nil {
		// "1.0 credit from ..., or any remaining 300+ level STA courses ... - <codes>
		// (<codes> may be considered ... with the approval of ...)": the listed
		// courses are all the pool's; only the bracketed ones need approval
		pool, _ := parseCompletionText(m[1] + ", " + m[2])
		pool, caps := poolCaps(checkedBody(pool, m[1]+", "+m[2]))
		if isPool(pool) && pool.Designators == nil {
			for _, c := range caps {
				rows = append(rows, &types.Requirement{Kind: types.RequirementLimit, Req: c, Raw: s})
			}
			if extra := codeRe.FindAllString(m[3], -1); extra != nil && complApprovalWordRe.MatchString(m[3]) {
				wide := cloneNode(pool)
				wide.Courses = dedupe(append(slices.Clone(pool.Courses), extra...))
				return orNode(pool, andNode(wide, textNode(types.TextPermission, m[3]))), rows, notes
			}
			return pool, rows, notes
		}
	}
	if m := complOrEquivalentRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(s), ".")); m != nil {
		// "0.5 LAS credit at the 400-level, or an equivalent 0.5 credit ...,
		// approved by ...": the strict pool, or the wider one with approval
		strict, _ := parseCompletionText(m[1])
		wide, _ := parseCompletionText(m[2])
		if strict = checkedBody(strict, m[1]); strict != nil && wide != nil && !hasText(strict) && !hasText(wide) {
			return orNode(strict, andNode(wide, textNode(types.TextPermission, m[3]))), rows, notes
		}
	}
	if m := complOrAnotherRe.FindStringSubmatchIndex(s); m != nil {
		// "A/ B/ ..., or another course with ... approved by the Undergraduate
		// Chair": the listed courses, or one a person approves
		head, more := parseCompletionText(s[:m[0]])
		head = checkedBody(head, s[:m[0]])
		if head != nil && head.Type == types.NodeAnd && !slices.ContainsFunc(head.Items, func(n *types.ReqNode) bool { return n.Type != types.NodeCourse }) {
			head = orNode(head.Items...) // "A, B, C, or ...": one of them
		}
		alt := textNode(types.TextPermission, s[m[2]:m[3]])
		return orNode(head, alt), rows, append(notes, more...)
	}
	if m := complUpToFromListRe.FindStringSubmatchIndex(s); m != nil {
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
			n = orNode(n, textNode(altKind(alt), alt))
		}
	}
	if n != nil && courseAlts != nil {
		n = orNode(append([]*types.ReqNode{n}, courseAlts...)...)
	}
	if m := complIeRe.FindStringSubmatch(s); m != nil {
		restatedLevels(n, complIeLeadRe.ReplaceAllString(m[1], ""))
	}
	return n, rows, append(notes, more...)
}

var (
	// complAlsoAcceptRe matches courses accepted in place of the item's: "We will
	// also accept PSY100Y5 from UTM ... in place of PSY100H1 for this requirement".
	complAlsoAcceptRe = regexp.MustCompile(`(?i)^we\s+will\s+also\s+accept\s+(.+?)(?:,?\s+in\s+place\s+of\s+` + codePat + `\b.*)?$`)
	// complAlsoAcceptNoiseRe matches the words around the codes that say where
	// and how: " from UTM".
	complAlsoAcceptNoiseRe = regexp.MustCompile(`(?i)\s*\bfrom\s+(?:UTM|UTSC)\b|\bboth\s+of\s+|\s+together\b`)
	// complAlsoAcceptOrRe matches what separates the alternatives: ", or ".
	complAlsoAcceptOrRe = regexp.MustCompile(`(?i),?\s+or\s+`)
	// complAlsoAcceptAndRe matches what may join one alternative's codes: "and".
	complAlsoAcceptAndRe = regexp.MustCompile(`(?i)\band\b|[,\s]`)
)

// alsoAccepted reads a complAlsoAcceptRe sentence into the courses it
// accepts: alternatives joined by "or", each courses joined by "and". It is
// nil when they don't read as courses alone.
func alsoAccepted(t string) *types.ReqNode {
	m := complAlsoAcceptRe.FindStringSubmatch(t)
	if m == nil {
		return nil
	}
	var alts []*types.ReqNode
	for _, part := range complAlsoAcceptOrRe.Split(complAlsoAcceptNoiseRe.ReplaceAllString(m[1], ""), -1) {
		codes := codeRe.FindAllString(part, -1)
		rest := strings.TrimSpace(complAlsoAcceptAndRe.ReplaceAllString(codeRe.ReplaceAllString(part, ""), ""))
		if codes == nil || rest != "" {
			return nil
		}
		nodes := courseNodes(codes)
		alts = append(alts, andNode(nodes...))
	}
	return orNode(alts...)
}

// orToPool reads alternatives under a lead amount as the pool it names,
// credits{amount, every course}, when one alternative weighs more than the
// amount: and(MAT158H1, MAT159H1) for 0.5 credit would fail MAT158H1 alone.
func orToPool(n *types.ReqNode, s string) *types.ReqNode {
	m := complLeadAmountRe.FindStringSubmatch(s)
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
		walkReq(it, func(x *types.ReqNode) {
			if x.Type == types.NodeCourse {
				codes = append(codes, x.Code)
			}
		})
	}
	if !heavy {
		return n
	}
	return &types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: dedupe(codes)}
}

// complDepartmentGroupRe matches an amount over ";" groups: "1.0 credit from
// ECO220Y1, ECO227Y1; GGR270H1, GGR271H1; STA220H1, STA221H1".
var complDepartmentGroupRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits?\s+from:?\s+(.+)$`)

// complCommaSpaceRe matches what may join a group's codes: ",".
var complCommaSpaceRe = regexp.MustCompile(`[,\s]`)

// departmentGroups reads a complDepartmentGroupRe pool as the or of its 3+
// groups, each one department's courses making up the amount together or each
// alone, or nil. A union of their courses would count GGR270H1 with STA220H1.
func departmentGroups(s string) *types.ReqNode {
	m := complDepartmentGroupRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(s), "."))
	if m == nil {
		return nil
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	groups := splitTop(m[2], ';')
	if len(groups) < 3 {
		return nil
	}
	var alts []*types.ReqNode
	for _, g := range groups {
		if strings.TrimSpace(complCommaSpaceRe.ReplaceAllString(codeRe.ReplaceAllString(g, ""), "")) != "" {
			return nil // more than codes and commas
		}
		codes := codeRe.FindAllString(g, -1)
		if len(codes) < 2 || slices.ContainsFunc(codes, func(c string) bool { return c[:3] != codes[0][:3] }) {
			return nil
		}
		sum, each := 0.0, true
		var nodes []*types.ReqNode
		for _, c := range codes {
			sum += courseWeight(c)
			each = each && courseWeight(c) == amount
			nodes = append(nodes, &types.ReqNode{Type: types.NodeCourse, Code: c})
		}
		switch {
		case math.Abs(sum-amount) < 1e-9:
			alts = append(alts, andNode(nodes...))
		case each:
			alts = append(alts, nodes...)
		default:
			return nil
		}
	}
	return orNode(alts...)
}

// complBracketPairRe matches bracketed alternatives inside a "/" list:
// "( EEB240H1/ ENV234H1)".
var complBracketPairRe = regexp.MustCompile(`\(\s*(` + codePat + `(?:\s*/\s*` + codePat + `)+)\s*\)`)

// bracketPairCaps caps each bracketed group of alternatives flattened into
// pool n at its heaviest course, as pairCaps does for a "/" pair.
func bracketPairCaps(n *types.ReqNode, s string) *types.ReqNode {
	pool, caps := poolCaps(n)
	if !isPool(pool) || pool.Courses == nil {
		return n
	}
	for _, m := range complBracketPairRe.FindAllStringSubmatch(s, -1) {
		codes := dedupe(codeRe.FindAllString(m[1], -1))
		w := heaviestCourse(codes)
		if slices.ContainsFunc(codes, func(c string) bool { return !slices.Contains(pool.Courses, c) }) ||
			slices.ContainsFunc(caps, func(c *types.ReqNode) bool { return isSubset(codes, c.Courses) && *c.MaxAmount <= w }) {
			continue // a cap that holds already
		}
		caps = append(caps, courseCap(codes))
	}
	return andNode(append([]*types.ReqNode{pool}, caps...)...)
}

var (
	// complAsideParenRe matches brackets that remark on an item rather than
	// require: "(formerly POL101Y1)".
	complAsideParenRe = regexp.MustCompile(`\(\s*_?((?i:note|nb)\s*:[^()]*|(?i:formerly)\s+[^()]*|(?i:see)\s+(?i:note)[^()]*|(?i:if\s+taking)\b[^()]*|(?i:total\s+of)\s+\d[^()]*|` + numPat + `\s+credits?\s+in\s+total|(?i:the|these)\s+[^()]*\b(?:are|is)\s+(?:recommended|suggested|encouraged)\.?|(?i:these)\s+[A-Z]{3}\s+courses\s+[^()]*|[A-Z][\w&,\s]*\bcourses)_?\s*\)`)
	// complAppositiveRe matches a descriptor closing a list of alternatives: ", a
	// capstone integrative course".
	complAppositiveRe = regexp.MustCompile(`,\s+an?\s+[a-z][a-z\s-]*\bcourse\.?$`)
)

// complCategoryAndRe matches a category label between two lists of one pool:
// "(History & Society courses) and ".
var complCategoryAndRe = regexp.MustCompile(`\(\s*([A-Z][\w&,\s]*\bcourses)\s*\)\s*,?\s*and\s+`)

// formerlyAlternatives also accepts the old code of each course of n that s
// writes as "POL101H1 (formerly POL101Y1)": a course becomes or(course, old)
// and a pool takes the old code. It returns asides minus those it read.
func formerlyAlternatives(n *types.ReqNode, s string, asides []string) []string {
	for _, m := range complCodeFormerlyRe.FindAllStringSubmatch(s, -1) {
		cur, old := m[1], m[2]
		var found []*types.ReqNode
		walkReq(n, func(x *types.ReqNode) {
			if x.Type == types.NodeCourse && x.Code == cur || x.Type == types.NodeCredits && slices.Contains(x.Courses, cur) {
				found = append(found, x)
			}
		})
		for _, x := range found {
			if x.Type == types.NodeCourse {
				*x = types.ReqNode{Type: types.NodeOr, Items: []*types.ReqNode{{Type: types.NodeCourse, Code: cur}, {Type: types.NodeCourse, Code: old}}}
			} else {
				x.Courses = dedupe(append(x.Courses, old))
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
	s = complCategoryAndRe.ReplaceAllStringFunc(s, func(p string) string {
		notes = append(notes, cleanNote(strings.TrimSpace(complCategoryAndRe.FindStringSubmatch(p)[1])))
		return ", "
	})
	t := complAsideParenRe.ReplaceAllStringFunc(s, func(p string) string {
		inner := strings.Trim(strings.TrimSpace(p[1:len(p)-1]), "_ ")
		notes = append(notes, cleanNote(complClean(inner)))
		return ""
	})
	if loc := complAppositiveRe.FindStringIndex(strings.TrimSpace(t)); loc != nil {
		u := strings.TrimSpace(t)
		notes = append(notes, cleanNote(strings.TrimPrefix(u[loc[0]:], ",")))
		t = u[:loc[0]]
	}
	if notes == nil {
		return s, nil
	}
	return strings.TrimSpace(strings.Join(strings.Fields(t), " ")), notes
}

// complOfAnyFollowingRe matches "1.5 credits of any of the following (...):".
var complOfAnyFollowingRe = regexp.MustCompile(`(?i)(` + numPat + `\s+credits?)\s+of\s+any\s+of\s+the\s+following\s*(\([^()]*\))?\s*:`)

// complTopicParenRe matches a course and the bracket after it: "TRN409H1
// (Nuclear Weapons and International Politics)".
var complTopicParenRe = regexp.MustCompile(`(` + codePat + `)\s*\(([^()]+)\)`)

// topicCodes returns the courses s lists with two or more different topics,
// each mapped to its mentions: a special-topics course counts only on those
// topics.
func topicCodes(s string) map[string][]string {
	topics := map[string][]string{}
	for _, m := range complTopicParenRe.FindAllStringSubmatch(s, -1) {
		if complTitleWordsRe.MatchString(strings.TrimSpace(m[2])) && !slices.Contains(topics[m[1]], m[0]) {
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
		return textNode(types.TextContent, strings.Join(ms, ", "))
	}
	switch n.Type {
	case types.NodeCourse:
		if topics[n.Code] != nil {
			return andNode(n, topicText([]string{n.Code}))
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
		strict := cloneNode(n)
		strict.Courses = slices.DeleteFunc(slices.Clone(n.Courses), func(c string) bool { return topics[c] != nil })
		return orNode(strict, andNode(n, topicText(hit)))
	case types.NodeAnd, types.NodeOr, types.NodeChoose:
		for i, it := range n.Items {
			n.Items[i] = topicMembers(it, topics)
		}
	}
	return n
}

// complIeLeadRe matches the "i.e.," before a restatement.
var complIeLeadRe = regexp.MustCompile(`(?i)^i\.e\.,?\s*`)

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
			!sameSet(p.Designators, pool.Designators) {
			return
		}
		sum += p.Amount
		levels = append(levels, p.Levels...)
	}
	if math.Abs(sum-pool.Amount) > 1e-9 {
		return
	}
	levels = dedupe(levels)
	slices.Sort(levels)
	pool.Levels = levels
}

// complPermListRe matches an amount from a list, or with permission from a
// second list: "1.0 credit from: A/ B or with permission of the Director: C/ D".
var complPermListRe = regexp.MustCompile(`(?i)^(.*?\bfrom:?\s+)((?:` + codePat + `\s*[/,;]?\s*)*` + codePat + `)\s*,?\s+or\s+((?:with|by)\s+(?:the\s+)?(?:prior\s+)?(?:written\s+)?(?:permission|approval)\s+(?:of|from)\s+[^:]+?)\s*:\s*((?:` + codePat + `\s*[/,;]?\s*)*` + codePat + `)\.?$`)

// permissionList builds or(credits{amount, list}, and(credits{amount, both
// lists}, permission text)) from a complPermListRe match. A strict part that
// isn't a plain pool of courses makes the whole sentence text.
func permissionList(m []string) *types.ReqNode {
	strict, _ := parseCompletionText(m[1] + m[2])
	if !isPool(strict) || strict.Courses == nil || strict.Designators != nil || strict.Levels != nil || strict.MaxAmount != nil {
		return textNode(types.TextOther, m[0])
	}
	wide := cloneNode(strict)
	wide.Courses = dedupe(append(slices.Clone(strict.Courses), codeRe.FindAllString(m[4], -1)...))
	return orNode(strict, andNode(wide, textNode(types.TextPermission, m[3])))
}

// complWithAverageRe matches "( A and B and C) with an average grade of 63%".
var complWithAverageRe = regexp.MustCompile(`(?i)^\(?\s*((?:` + codePat + `\s*(?:,|and)?\s*)+)\)?\s+with\s+an\s+average\s+(?:grade\s+)?of\s+(?:` + atLeastPat + `\s+)?(\d{2})\s*%$`)

// complAnyRemainingRe matches "<pool>, or any remaining <filter> not used
// toward other program requirements - <codes> (<approval for more codes>)".
var complAnyRemainingRe = regexp.MustCompile(`(?i)^(.+?),?\s+or\s+any\s+remaining\s+[^-–]+?\s+[-–]\s+((?:\(?\s*` + codePat + `\s*\)?\s*[,/]?\s*)+)\s*(?:\((.*)\))?$`)

// complOrEquivalentRe matches "<strict>, or an equivalent <wider>, approved by
// <whom>".
var complOrEquivalentRe = regexp.MustCompile(`(?i)^(.+?),?\s+or\s+an\s+equivalent\s+(.+?),\s+((?:approved\s+by|with\s+(?:the\s+)?approval\s+of)\s+.+)$`)

var (
	// complLowerRemarkRe matches a lowercase bracketed remark of advice or
	// timing: "(should be taken in first year)".
	complLowerRemarkRe = regexp.MustCompile(`\s*\(((?:should|ideally|normally|preferably|recommended|strongly|highly|best|to\s+be\s+taken)\b[^()]*)\)`)
	// complTimingRe matches when to take a course: "in first year".
	complTimingRe = regexp.MustCompile(`(?i)\b(?:should|to)\s+be\s+taken\b|\bin\s+(?:the\s+)?(?:first|second|third|fourth)\s+year\b`)
)

// complAndOrApprovalRe matches "A/ B and/or C/ D <qualified, with approval>".
var complAndOrApprovalRe = regexp.MustCompile(`^((?:` + codePat + `\s*/\s*)*` + codePat + `)\s+and/or\s+((?:` + codePat + `\s*/\s*)*` + codePat + `)\s+([^;]*\b(?:approval|permission)\b[^;]*)$`)

// complOrAnotherRe matches an approved alternative closing an item's list:
// ", or another course ... approved by the Undergraduate Chair".
var complOrAnotherRe = regexp.MustCompile(`(?i),?\s+(?:or|and)\s+((?:another|any)\s+[^;]*?\b(?:approved\s+by|by\s+permission|with\s+(?:the\s+)?(?:prior\s+)?(?:approval|permission)|at\s+the\s+discretion\s+of|as\s+determined\s+by)\b[^;]*?)\.?$`)

// checkedBody returns n, or s as one text node when the grammar's reading
// can't be trusted, since a wrong structure is worse than text: a union pool,
// text that swallowed a code, and the misreadings marked below.
func checkedBody(n *types.ReqNode, s string) *types.ReqNode {
	if n == nil {
		return nil
	}
	bad, read := false, false
	walkReq(n, func(x *types.ReqNode) {
		switch {
		case x.Type == types.NodeCredits && len(x.Items) > 0, x.Type == types.NodeText && codeRe.MatchString(x.Text):
			bad = true
		case x.Type != types.NodeText && x.Type != types.NodeAnd && x.Type != types.NodeOr:
			read = true
		}
	})
	walkReq(n, func(x *types.ReqNode) {
		if x.Type != types.NodeAnd {
			return
		}
		switch {
		case slices.ContainsFunc(x.Items, isAnyCredits) &&
			slices.ContainsFunc(x.Items, func(y *types.ReqNode) bool { return y.Type == types.NodeCourse || y.Type == types.NodeText }):
			bad = true
		case slices.ContainsFunc(x.Items, hasCourses) && slices.ContainsFunc(x.Items, func(y *types.ReqNode) bool {
			return y.Type == types.NodeText && y.Kind == types.TextOther && complProseRe.MatchString(y.Text)
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
			len(complAmountRe.FindAllString(s, -1)) == 1:
			// "2.0 credits from <codes> and <codes>" read as a pool and required ones
			bad = true
		case slices.ContainsFunc(x.Items, func(y *types.ReqNode) bool { return y.Type == types.NodeCredits && y.Courses != nil }) &&
			slices.ContainsFunc(x.Items, func(y *types.ReqNode) bool { return y.Type == types.NodeText }) &&
			len(slices.DeleteFunc(slices.Clone(x.Items), func(y *types.ReqNode) bool { return y.Type != types.NodeCourse })) >= 2:
			// a pool cut short by a remark, its last members read as required
			bad = true
		}
	})
	walkReq(n, func(x *types.ReqNode) {
		// an empty credits node: a phrase half read ("normally taken in first year")
		bad = bad || x.Type == types.NodeCredits && x.Amount == 0 && x.MaxAmount == nil && x.Items == nil && x.Courses == nil &&
			x.Designators == nil && x.Levels == nil && x.Breadth == nil && x.Lists == nil
	})
	walkReq(n, func(x *types.ReqNode) {
		// "1.0 credit in ONE other discipline": a word in capitals, not a designator
		bad = bad || x.Type == types.NodeCredits && slices.ContainsFunc(x.Designators, func(d string) bool { return complCapsWords[d] })
	})
	walkReq(n, func(x *types.ReqNode) {
		// "3.5 credits of ESS core courses": the designator names a list
		if x.Type != types.NodeAnd {
			return
		}
		bad = bad || slices.ContainsFunc(x.Items, func(y *types.ReqNode) bool {
			return y.Type == types.NodeCredits && y.Designators != nil && y.Courses == nil
		}) && slices.ContainsFunc(x.Items, func(y *types.ReqNode) bool {
			return y.Type == types.NodeText && y.Kind == types.TextOther && complListNounRe.MatchString(y.Text)
		})
	})
	walkReq(n, func(x *types.ReqNode) {
		// listed courses alone can't make up the amount: the rest is a list or text
		if x.Type == types.NodeCredits && x.Amount > 0 && x.Courses != nil && x.Designators == nil && x.Levels == nil &&
			x.Lists == nil && x.Breadth == nil && x.Items == nil && x.Requirements == nil {
			sum := 0.0
			for _, c := range dedupe(x.Courses) {
				sum += courseWeight(c)
			}
			bad = bad || sum < x.Amount-1e-9
		}
	})
	if complCapLeadRe.MatchString(strings.TrimSpace(s)) {
		// "Up to 1.0 credit may be taken from list 2": a cap is no body
		bad = true
	}
	walkReq(n, func(x *types.ReqNode) {
		// "...; fulfilled by any 400-level courses listed in ...": fragments
		bad = bad || x.Type == types.NodeText && x.Kind == types.TextOther && complFragmentRe.MatchString(x.Text)
	})
	if complWordSlashRe.MatchString(s) {
		// "disciplinary/thematic": a "/" between words offers no alternatives
		walkReq(n, func(x *types.ReqNode) {
			bad = bad || x.Type == types.NodeOr && slices.ContainsFunc(x.Items, func(y *types.ReqNode) bool { return y.Type == types.NodeText })
		})
	}
	if m := complLeadAmountRe.FindStringSubmatch(s); m != nil && n.Type == types.NodeAnd &&
		!slices.ContainsFunc(n.Items, func(y *types.ReqNode) bool { return y.Type != types.NodeCourse }) {
		// "1.0 credit: A, B, C" read as three required courses worth 1.5
		amount, _ := strconv.ParseFloat(m[1], 64)
		sum := 0.0
		for _, it := range n.Items {
			sum += courseWeight(it.Code)
		}
		bad = bad || sum > amount+1e-9
	}
	if m := complDesigAtLevelOrRe.FindStringSubmatch(s); m != nil &&
		!sameSet(parseLevels(m[2]), parseLevels(m[4])) && !sameSet(desigRe.FindAllString(m[1], -1), desigRe.FindAllString(m[3], -1)) {
		// "FRE at the 200+ level or FSL at the 300+ level": two filters
		bad = true
	}
	if m := complFilterOrFilterRe.FindStringSubmatch(s); m != nil &&
		!sameSet(parseLevels(m[1]), parseLevels(m[3])) && !sameSet(desigRe.FindAllString(m[2], -1), desigRe.FindAllString(m[4], -1)) {
		// "100- or 200-level ESS courses or 200-level JEG courses": a union one
		// filter can't hold; merged, it would count JEG100H1
		bad = true
	}
	texts := 0
	walkReq(n, func(x *types.ReqNode) {
		if x.Type == types.NodeText && x.Kind == types.TextOther {
			texts++
		}
	})
	switch {
	case bad || texts >= 3:
		// three fragments of text: the grammar read a sentence, not a list
		return textNode(types.TextOther, s)
	case !read && (n.Type == types.NodeAnd || n.Type == types.NodeOr):
		return textNode(classify(cleanNote(s)), s)
	}
	return n
}

// complWordSlashRe matches two lowercase words joined by "/":
// "disciplinary/thematic".
var complWordSlashRe = regexp.MustCompile(`\b[a-z]{3,}/[a-z]{3,}\b`)

// complFragmentRe matches text the grammar cut from a sentence, not a
// condition of its own: "fulfilled by".
var complFragmentRe = regexp.MustCompile(`(?i)^(?:fulfilled\s+by|listed\s+(?:in|below|above)\b|both\s+of\b|one\s+or\s+both\b|plus\b|[A-D]-level\b|on\s+the\b|disciplinary/thematic\b)`)

// complLeadAmountRe matches the amount a requirement starts with: "1.0 credit".
var complLeadAmountRe = regexp.MustCompile(`(?i)^(?:[^:]{0,60}:\s*)?` + amountPat + `\s+credits?\b`)

// complCapsWords are words written in capitals that aren't designators.
var complCapsWords = map[string]bool{"ONE": true, "TWO": true, "ALL": true, "ANY": true, "AND": true, "THE": true, "NOT": true,
	"FOR": true, "BUT": true, "MAY": true, "OUT": true, "SIX": true, "TEN": true, "PER": true, "TBA": true}

// complListNounRe matches text that names a list beside a designator: "core
// courses".
var complListNounRe = regexp.MustCompile(`(?i)^(?:other\s+)?(?:[a-z&-]+\s+){0,3}(?:courses?|concentrations?|disciplines?|streams?|areas?|groups?|lists?)\b`)

// complFilterOrFilterRe matches "<levels> <designators> courses or <levels>
// <designators> courses": "200-level ESS courses or 300-level JEG courses".
var complFilterOrFilterRe = regexp.MustCompile(`(?i)\b(` + levelPat + `(?:\s*[-/,]?\s*(?:or|and)?\s*` + levelPat + `)*[-+]?)\s*-?\s*levels?\s+(` + desigPat + `(?:\s*(?:/|,|or|and)\s*` + desigPat + `)*)\s+courses?\s+or\s+(` + levelPat + `(?:\s*[-/,]?\s*(?:or|and)?\s*` + levelPat + `)*[-+]?)\s*-?\s*levels?\s+(` + desigPat + `(?:\s*(?:/|,|or|and)\s*` + desigPat + `)*)\s+courses?\b`)

// complDesigAtLevelOrRe matches "<designators> at the <levels> or
// <designators> at the <levels>": "FRE at the 200 level or FSL at the 300 level".
var complDesigAtLevelOrRe = regexp.MustCompile(`\b((?:[A-Z]{3})(?:\s*/\s*[A-Z]{3})*)\s+(?:(?i:courses?|credits?)\s+)?(?i:at\s+the)\s+(` + levelPat + `[-+/\s\d]*(?i:levels?)?)\s+(?i:or)\s+((?:[A-Z]{3})(?:\s*/\s*[A-Z]{3})*)\s+(?:(?i:courses?|credits?)\s+)?(?i:at\s+the)\s+(` + levelPat + `[-+/\s\d]*(?i:levels?)?)`)

func hasCourses(n *types.ReqNode) bool {
	found := false
	walkReq(n, func(x *types.ReqNode) { found = found || x.Type == types.NodeCourse || x.Type == types.NodeCredits })
	return found
}

// complProseRe matches a verb, which makes text around courses a sentence
// about them: "should".
var complProseRe = regexp.MustCompile(`(?i)\b(?:should|must|may|can|will|note|limits?|is|are|was|were|have|has)\b`)

// isAnyCredits reports whether n is an amount of any credits, with no filter.
func isAnyCredits(n *types.ReqNode) bool {
	return n.Type == types.NodeCredits && n.Amount > 0 && n.Courses == nil && n.Designators == nil && n.Levels == nil &&
		n.Breadth == nil && n.Lists == nil && n.Requirements == nil && !n.Transfer && !n.CrNcr && n.ExcludeDesignators == nil && n.MaxCourseAmount == 0
}

// itemLabel is the calendar's label for an item: its number ("Higher Years
// 2") or a labelled line's name; not a year, which is the section, nor a
// note's label.
func itemLabel(b *compBlock) string {
	switch {
	case b.kind == blockItem:
		return b.label
	case b.kind == blockLabelled && !blkStartsSection(b) && !blkNoteLabelRe.MatchString(b.label):
		return b.label
	case b.kind == blockLabelled && sectionLabel(b.label):
		// "Fourth Year Courses: Any 2 courses ...": the label names its section
		return b.label
	}
	return ""
}

func (a *compAssembler) allAdvice(s string) bool {
	for _, sent := range splitComplSentences(s) {
		if a.role(sent) != roleAdvisory {
			return false
		}
	}
	return true
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

var (
	// complLooseRe matches a sentence that permits or defines rather than
	// requires: "Students may use CR/NCR on ...".
	complLooseRe = regexp.MustCompile(`(?i)\b(?:may|can|could|might)\b|\b(?:is|are)\s+(?:also\s+)?considered\b|\bare\s+those\b|\bsub-?total\b|\bis\s+waived\b|\bis\s+not\s+a\s+substitute\b|\bconsidered\s+equivalent\b|^total\s*=|\bdoes\s+not\s+(?:have|need)\s+to\s+be\s+replaced\b|\bno\s+need\s+to\s+replace\b`)
	// complBindingWordRe matches words that make a sentence a rule whatever else
	// it says: "must".
	complBindingWordRe = regexp.MustCompile(`(?i)\bmust\b|\brequired?\b|\b` + atLeastPat + `\b|\b` + noMoreThanPat + `\b|\bminimum\b|\bmaximum\b|\bcannot\b|\bonly\b|\bat\s+most\b|\bnot\s+(?:be\s+)?(?:counted|accepted|used|permitted|allowed)\b|\bmay\s+not\b`)
)

// looseMain reports whether s only permits or defines, read in its main
// clause (brackets and what follows ";" are remarks). A line leading with an
// amount or codes the grammar reads as courses is a requirement.
func looseMain(s string) bool {
	t := complClean(s)
	main := strings.TrimSpace(splitTop(complStripParens(t), ';')[0])
	if !complLooseRe.MatchString(main) || complBindingWordRe.MatchString(main) {
		return false
	}
	if complCodeLeadRe.MatchString(main) || complLeadAmountRe.MatchString(main) {
		n, _ := parseCompletionText(main)
		if n = checkedBody(n, main); n != nil && !hasText(n) && !isAnyCredits(n) {
			return false
		}
	}
	return true
}

// looseSentence places a requirement sentence the grammar can't read as a
// rule. One that only permits or defines is no constraint, which would force
// Review on everyone: an alternative on a row it relaxes, else a note.
func (a *compAssembler) looseSentence(target *compRow, s string) {
	t := a.clean(s)
	if !complLooseRe.MatchString(t) || complBindingWordRe.MatchString(t) {
		if complFloorLikeRe.MatchString(t) {
			// "At least one Applications course (0.5 credit) must be ...": a floor
			if rows, ok := readFloor(s); ok {
				for _, r := range rows {
					a.rowUnder(target, r)
				}
				return
			}
		}
		if a.yHalves(s) {
			return
		}
		if r := a.restatedRow(t); r != nil {
			// "CSC110Y1 and CSC111H1 must be completed ...": restates a row
			addRaw(r, s)
		} else if rs := a.referencedRows(s); complWeightSplitRe.MatchString(t) && rs != nil {
			// "If students complete ENV465Y1, 0.5 credit ... contributes to group
			// B in requirement 2": an alternative there, so Review, not NotMet
			for _, r := range rs {
				r.r.Req = orNode(r.r.Req, textNode(types.TextOther, s))
				addRaw(r, s)
			}
		} else if carriesRule(t) {
			a.textRow(target, s)
		} else {
			a.notesTo(a.noteTarget(target), []string{cleanNote(s)})
		}
		return
	}
	if !complNegationRe.MatchString(complNotListedRe.ReplaceAllString(t, "")) && (a.isAlternative(s) || complReplacedRe.MatchString(t) || complExemptedRe.MatchString(t) || complConsiderForRe.MatchString(t)) {
		if rs, replaced := a.relaxedRows(s); rs != nil {
			for _, r := range rs {
				a.wrapRoute(r, replaced, textNode(altKind(s), s))
				addRaw(r, s)
			}
			return
		}
		if target != nil && target != a.total && target.r.Req != nil {
			a.alternative(target, s)
			return
		}
		if target == nil || target == a.total {
			// a program route: placed by what it names, else a note
			a.programRoutes = append(a.programRoutes, s)
			return
		}
	}
	if target != nil && target != a.total {
		target.r.Notes = append(target.r.Notes, cleanNote(s))
		return
	}
	a.notes = append(a.notes, cleanNote(s))
}

// complRequirementLeadRe matches a line announcing the requirement after it:
// "An Integrative, Inquiry-Based Activity Requirement must be satisfied".
var complRequirementLeadRe = regexp.MustCompile(`(?i)^an?\s+[^.]+?\s+requirement\s+must\s+be\s+(?:satisfied|met|completed|fulfilled)$|\bthe\s+following\s+requirements\s*:?$`)

// complInheritRe matches a focus that begins with another program's
// requirements, which Link copies: "begins with the requirements of".
var complInheritRe = regexp.MustCompile(`(?i)\bbegins\s+with\s+the\s+requirements\s+of\b`)

// complYHalvesRe matches "Some of the above course options are 1.0 credit
// courses, but only 0.5 credit can be counted".
var complYHalvesRe = regexp.MustCompile(`(?i)^some\s+of\s+the\s+above\s+course\s+options\s+are\s+1\.0\s+credit\s+courses,?\s+but\s+only\s+0\.5\s+credit\s+can\s+be\s+counted\b`)

// yHalves reads a complYHalvesRe sentence into a 0.5 limit on each full-year
// course of the item above it, or joins its raw when it lists none, and reports
// whether it did.
func (a *compAssembler) yHalves(s string) bool {
	if !complYHalvesRe.MatchString(a.clean(s)) {
		return false
	}
	rows := a.routeRows()
	for i := len(rows) - 1; i >= 0; i-- {
		r := rows[i]
		var ys []string
		walkReq(r.r.Req, func(x *types.ReqNode) {
			for _, c := range append(slices.Clone(x.Courses), x.Code) {
				if c != "" && courseWeight(c) == 1 {
					ys = append(ys, c)
				}
			}
		})
		if r == a.total {
			continue
		}
		if ys == nil {
			// no full-year course: the sentence caps nothing, so it is only raw
			addRaw(r, s)
			return true
		}
		for _, c := range dedupe(ys) {
			a.rowUnder(r, &types.Requirement{Kind: types.RequirementLimit, Raw: s,
				Req: &types.ReqNode{Type: types.NodeCredits, Courses: []string{c}, MaxAmount: maxAmount(0.5)}})
		}
		addRaw(r, s)
		return true
	}
	return false
}

// complRestateRe matches a sentence saying courses are required: "must be
// completed".
var complRestateRe = regexp.MustCompile(`(?i)\bmust\s+be\s+(?:completed|taken)\b|\b(?:is|are)\s+(?:a\s+)?required(?:\s+courses?)?\b`)

// restatedRow returns the one row that already requires every course a
// complRestateRe sentence names (no amount, no other rule), or nil.
func (a *compAssembler) restatedRow(t string) *compRow {
	codes := codeRe.FindAllString(t, -1)
	if codes == nil || !complRestateRe.MatchString(t) || complAmountRe.MatchString(t) || complPrereqRe.MatchString(t) {
		return nil
	}
	var found *compRow
	for _, r := range a.routeRows() {
		if r == a.total || slices.ContainsFunc(codes, func(c string) bool {
			has := false
			walkReq(r.r.Req, func(x *types.ReqNode) { has = has || x.Type == types.NodeCourse && x.Code == c })
			return !has
		}) {
			continue
		}
		if found != nil {
			return nil
		}
		found = r
	}
	return found
}

// complFloorLikeRe matches a sentence worded as a floor: "At least ... must
// be ...".
var complFloorLikeRe = regexp.MustCompile(`(?i)^` + atLeastPat + `\b.*\bmust\s+be\b`)

// complNotListedRe matches a course outside a list, which refuses nothing:
// "not listed in".
var complNotListedRe = regexp.MustCompile(`(?i)\bnot\s+(?:already\s+)?(?:listed|captured|included)\s+in\b`)

// complConsiderForRe matches courses a person may accept for a requirement:
// "can also be considered for".
var complConsiderForRe = regexp.MustCompile(`(?i)\b(?:can|may)\s+(?:also\s+)?be\s+considered\s+for\b|\bmay\s+be\s+able\s+to\s+fulfil+\b`)

// noteTarget is the row a note on target goes to: nil (the program) for the
// total.
func (a *compAssembler) noteTarget(target *compRow) *compRow {
	if target == a.total {
		return nil
	}
	return target
}

var (
	// complPrereqRe matches what a course needs before it, not what the program
	// counts: "Students taking PCL477H1 must take BCH210H1 prior".
	complPrereqRe = regexp.MustCompile(`(?i)^(?:` + codePat + `)(?:\s*(?:,|and|or)\s*` + codePat + `)*\s+requires?\b|\bmust\s+take\s+` + codePat + `\s+(?:prior|first|before)\b|\b(?:is|are)\s+required\s+for\s+all\b|\bprerequisites?\s+for\s+these\s+courses\b`)
	// complExclusiveAgainRe matches the exclusive claims restated: "cannot be one
	// that is already being used".
	complExclusiveAgainRe = regexp.MustCompile(`(?i)\bcannot\s+be\s+(?:one|a\s+course)\s+that\s+is\s+already\s+(?:being\s+)?used\b`)
	// complWeightSplitRe matches how much of a course counts where, which the
	// claims split: "0.5 credit from the course contributes to group B".
	complWeightSplitRe = regexp.MustCompile(`(?i)\bcredit\s+from\s+the\s+course\s+contributes\s+to\b`)
)

// complNoOtherRe matches a refusal of anything else: "No other cognates are
// permitted".
var complNoOtherRe = regexp.MustCompile(`(?i)\bno\s+other\b[^.;]*\b(?:permitted|allowed|accepted|counted)\b`)

// carriesRule reports whether an unread requirement sentence states a rule
// the checker must see (a text row, Review) rather than a note. Prerequisites,
// restated claims and weight splits don't: they'd force Review on everyone.
func carriesRule(t string) bool {
	switch {
	case complPrereqRe.MatchString(t), complExclusiveAgainRe.MatchString(t), complWeightSplitRe.MatchString(t):
		return false
	case complBindingWordRe.MatchString(t), complNoOtherRe.MatchString(t), isComplAllocLead(t):
		return true
	}
	return complAmountRe.MatchString(t) && (codeRe.MatchString(t) || complFilterRe.MatchString(t))
}

// complApprovalWordRe matches a sentence that depends on someone's approval:
// "permission".
var complApprovalWordRe = regexp.MustCompile(`(?i)\bapprov(?:al|ed)\b|\bpermission\b|\bdiscretion\b`)

// complSubstitutedFromRe matches a capped share from named courses: "A
// maximum of 0.5 credit may be substituted from the following first-year
// courses: POL191H1".
var complSubstitutedFromRe = regexp.MustCompile(`(?i)^(?:a\s+maximum\s+of|up\s+to|` + noMoreThanPat + `)\s+(?:a\s+)?` + amountPat + `\s+credits?\s+(?:of\s+this\s+requirement\s+)?(?:may|can)\s+(?:also\s+)?be\s+(?:substituted|taken|chosen|satisfied|completed|fulfilled)\s+(?:from|by\s+taking)\s+(?:(?:one|any)\s+of\s+)?(?:the\s+following[^:]*:\s*)?((?:` + codePat + `\s*[,/;]?\s*(?:and|or)?\s*)+)$`)

// substitutedFrom widens the item's pool with the courses a
// complSubstitutedFromRe sentence names, capped at its amount, and reports
// whether it did.
func (a *compAssembler) substitutedFrom(row *compRow, s string) bool {
	n, limit := widenPool(row.r.Req, s)
	if limit == nil {
		return false
	}
	row.r.Req = n
	a.rowUnder(row, limit)
	addRaw(row, s)
	return true
}

// widenPool returns pool n widened with the courses of complSubstitutedFromRe
// clause s, and the limit capping them. The limit is nil when s isn't one, or
// n isn't a pool of listed courses whose levels the courses fit.
func widenPool(n *types.ReqNode, s string) (*types.ReqNode, *types.Requirement) {
	m := complSubstitutedFromRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(complClean(s)), "."))
	if m == nil || n == nil || n.Type != types.NodeCredits || n.Courses == nil || n.Items != nil {
		return n, nil
	}
	codes := codeRe.FindAllString(m[2], -1)
	if n.Levels != nil && slices.ContainsFunc(codes, func(c string) bool { return !levelMatches(n.Levels, c) }) {
		return n, nil
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	w := cloneNode(n)
	w.Courses = dedupe(append(slices.Clone(n.Courses), codes...))
	return w, &types.Requirement{Kind: types.RequirementLimit, Raw: s,
		Req: &types.ReqNode{Type: types.NodeCredits, Courses: dedupe(codes), MaxAmount: maxAmount(v)}}
}

// complConditionalRe matches "If CLA160H1 is taken, students must take an
// additional 0.5 credit from the following: ...".
var complConditionalRe = regexp.MustCompile(`(?i)^if\s+((?:` + codePat + `\s*(?:/|or|,)?\s*)+?)\s+(?:is|are)\s+taken,?\s+students\s+must\s+(?:also\s+)?take\s+an\s+additional\s+(.+)$`)

// conditionalExtra reads a complConditionalRe sentence into the item it
// follows, making the course and(course, extra), and reports whether it
// matched. The item is text if it lacks that alternative or the extra doesn't read.
func (a *compAssembler) conditionalExtra(row *compRow, s string) bool {
	m := complConditionalRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(a.clean(s)), "."))
	if m == nil {
		return false
	}
	extra, _ := parseCompletionText(m[2])
	extra = checkedBody(extra, m[2])
	done := false
	conds := codeRe.FindAllString(m[1], -1)
	if n := row.r.Req; extra != nil && !hasText(extra) && n != nil && n.Type == types.NodeOr {
		for i, it := range n.Items {
			if it.Type == types.NodeCourse && slices.Contains(conds, it.Code) {
				n.Items[i] = andNode(it, cloneNode(extra))
				done = true
			}
		}
	}
	if !done {
		row.r.Req = textNode(types.TextOther, rawHead(row.r))
	}
	return true
}

// complExemptedRe matches "Students exempted from FSL220H1/ FSL222H1 may
// replace it".
var complExemptedRe = regexp.MustCompile(`(?i)\bexempted\s+from\s+((?:` + codePat + `[\s,/]*(?:and|or)?\s*)+)`)

// relaxedRows returns the rows a loose sentence relaxes and the codes it
// stands in for: rows holding the codes it replaces, the requirement it names,
// or the only item naming one of its codes.
func (a *compAssembler) relaxedRows(s string) ([]*compRow, []string) {
	t := a.clean(s)
	for _, re := range []*regexp.Regexp{complReplacedRe, complExemptedRe} {
		if m := re.FindStringSubmatch(t); m != nil {
			replaced := codeRe.FindAllString(m[1], -1)
			if out := rowsHolding(a.routeRows(), replaced); out != nil {
				return out, replaced
			}
		}
	}
	if m := complEquivalentsRe.FindStringSubmatch(t); m != nil {
		// "... RSM course equivalents ... (e.g. RSM100H1 ... equivalent to
		// MGT100H1)": every row requiring a course of the example's designator
		var out []*compRow
		var replaced []string
		for _, r := range a.routeRows() {
			has := false
			walkReq(r.r.Req, func(x *types.ReqNode) {
				if x.Type == types.NodeCourse && strings.HasPrefix(x.Code, m[1][:3]) {
					has = true
					replaced = append(replaced, x.Code)
				}
			})
			if has {
				out = append(out, r)
			}
		}
		if out != nil {
			return out, dedupe(replaced)
		}
	}
	if rs := a.referencedRows(s); rs != nil {
		return rs, nil
	}
	if rs := a.referenced(s); rs != nil {
		return rs, nil
	}
	if r := a.itemNaming(s); r != nil {
		return []*compRow{r}, nil
	}
	return nil, nil
}

// complEquivalentsRe matches another unit's courses accepted as equivalents,
// with an example: "any completed RSM course equivalents ... deemed equivalent
// to MGT100H1".
var complEquivalentsRe = regexp.MustCompile(`(?i)\bany\s+(?:completed\s+)?` + desigPat + `\s+course\s+equivalents\b.*\bdeemed\s+equivalent\s+to\s+(` + codePat + `)`)

// overlap reads "Some courses may contribute to both ... (in requirements 4
// and 6)" into an overlap of the two rows, and reports whether it named two
// items.
func (a *compAssembler) overlap(s string) bool {
	rows := a.referenced(s)
	if len(rows) != 2 || rows[0] == rows[1] || !slices.Contains(a.items, rows[0]) || !slices.Contains(a.items, rows[1]) {
		return false // overlaps join allocations under one parent: two items
	}
	a.overlaps = append(a.overlaps, [2]*compRow{rows[0], rows[1]})
	addRaw(rows[0], s)
	addRaw(rows[1], s)
	return true
}

// textRow keeps a binding sentence the grammar can't read as a constraint
// whose body is the sentence: Review, never dropped.
func (a *compAssembler) textRow(target *compRow, s string) {
	a.rowUnder(target, a.textReq(s))
}

func (a *compAssembler) textReq(s string) *types.Requirement {
	a.diag.textRows = append(a.diag.textRows, s)
	return &types.Requirement{Kind: types.RequirementConstraint, Req: textNode(types.TextOther, s), Raw: s}
}

func (a *compAssembler) constraint(target *compRow, s string) {
	for _, r := range a.floorRows(s) {
		a.rowUnder(target, r)
	}
}

// floorRows reads a floor into constraint rows, or a row of text.
func (a *compAssembler) floorRows(s string) []*types.Requirement {
	if rows, ok := readFloor(s); ok {
		return rows
	}
	return []*types.Requirement{a.textReq(s)}
}

// complMinUpToRe matches a floor and a cap on some designators' courses: "For
// CSC courses, a minimum of 1.0 credit and up to 2.0 credits must be chosen".
var complMinUpToRe = regexp.MustCompile(`(?i)^for\s+(` + desigPat + `(?:\s*(?:,|/|and|or)\s*` + desigPat + `)*)\s+courses,\s+(?:a\s+minimum\s+of|` + atLeastPat + `)\s+` + amountPat + `\s+credits?\s+and\s+(?:up\s+to|a\s+maximum\s+of|` + noMoreThanPat + `)\s+` + amountPat + `\s+credits?\s+must\s+be\s+(?:chosen|taken|completed)$`)

// readFloor reads a floor into constraint rows; ok is false when the grammar
// can't read it.
func readFloor(s string) (rows []*types.Requirement, ok bool) {
	t := complOutOfRe.ReplaceAllString(strings.TrimSpace(strings.TrimSuffix(s, ".")), "$1 credit")
	// "At least one Applications course (0.5 credit) must be ...": its credit
	t = complOneCourseFloorRe.ReplaceAllString(t, "At least $1 credit must be at the")
	if m := complMinUpToRe.FindStringSubmatch(t); m != nil {
		// a floor and a cap on the same courses
		lo, _ := strconv.ParseFloat(m[2], 64)
		hi, _ := strconv.ParseFloat(m[3], 64)
		desigs := desigRe.FindAllString(m[1], -1)
		return []*types.Requirement{
			{Kind: types.RequirementConstraint, Raw: s, Req: &types.ReqNode{Type: types.NodeCredits, Amount: lo, Designators: desigs}},
			{Kind: types.RequirementLimit, Raw: s, Req: &types.ReqNode{Type: types.NodeCredits, Designators: slices.Clone(desigs), MaxAmount: maxAmount(hi)}},
		}, true
	}
	if m := complEachOfRe.FindStringSubmatch(t); m != nil {
		// "at least 2.0 credits in each of GRK and LAT": a floor for each
		v, _ := strconv.ParseFloat(m[1], 64)
		for _, d := range desigRe.FindAllString(m[2], -1) {
			rows = append(rows, &types.Requirement{Kind: types.RequirementConstraint, Raw: s,
				Req: &types.ReqNode{Type: types.NodeCredits, Amount: v, Designators: []string{d}}})
		}
		return rows, true
	}
	if rows, tail, ok := parseHeaderFloors(t); ok && len(tail) == 0 &&
		!slices.ContainsFunc(rows, func(r *types.Requirement) bool { return r.Kind != types.RequirementConstraint || hasText(r.Req) }) {
		for _, r := range rows {
			r.Raw = s
		}
		return rows, true
	}
	n, _ := parseCompletionText(t)
	n, _ = poolCaps(n) // a floor counts what its parent claimed: no caps
	if n != nil && n.Type == types.NodeCredits && n.Items == nil && n.MaxAmount == nil {
		return []*types.Requirement{{Kind: types.RequirementConstraint, Req: n, Raw: s}}, true
	}
	return nil, false
}

// complOneCourseFloorRe matches "At least one Applications course (0.5
// credit) must be".
var complOneCourseFloorRe = regexp.MustCompile(`(?i)^` + atLeastPat + `\s+one\s+(?:[\w-]+\s+){0,3}?course\s+\(` + amountPat + `\s+credits?\)\s+must\s+be(?:\s+at\s+the)?`)

// complOutOfRe matches a floor scoped to the item: "1.0 out of the 3.5
// credits".
var complOutOfRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+out\s+of\s+the\s+` + numPat + `\s+credits\b`)

// complEachOfRe matches "at least 2.0 credits in each of GRK and LAT".
var complEachOfRe = regexp.MustCompile(`(?i)^(?:` + atLeastPat + `\s+)?` + amountPat + `\s+credits?\s+in\s+each\s+of\s+(` + desigPat + `(?:\s*(?:,|and|or|/)\s*` + desigPat + `)+)$`)

// limit reads a cap into a limit row under target: "No more than 1.0 credit
// of 100-level courses may be used". Courses that don't count are a cap of 0.
func (a *compAssembler) limit(target *compRow, s string) {
	if complCampusRe.MatchString(a.clean(s)) && a.campusRule(s) || a.yHalves(s) {
		return
	}
	if r := capRow(s); r != nil {
		a.rowUnder(target, r)
		return
	}
	if target != nil && target != a.total && a.poolAddition(target, s) {
		// "Up to 1.0 credit can be used from GGR493Y1, ... approved by department":
		// more courses for the item, not a cap on it
		return
	}
	if t := a.clean(s); (target == nil || target == a.total) && complCognateRe.MatchString(t) && complApprovalWordRe.MatchString(t) {
		// "Up to 1.0 credit in ... pre-approved cognate courses may count ...": a
		// program route, placed by what it names, not a cap
		a.programRoutes = append(a.programRoutes, s)
		return
	}
	if complPrereqRe.MatchString(a.clean(s)) {
		// "... prerequisites for these courses cannot be used towards ...": a note
		a.notesTo(a.noteTarget(target), []string{cleanNote(s)})
		return
	}
	a.textRow(target, s)
}

// complCognateRe matches courses of other departments a program accepts:
// "cognate".
var complCognateRe = regexp.MustCompile(`(?i)\bcognates?\b`)

var (
	// complLimitedToRe matches a cap worded as a limit on students: "Students
	// enrolled in the Computer Science Minor program are limited to at most ".
	complLimitedToRe = regexp.MustCompile(`(?i)^students\s+(?:enrolled\s+in\s+.+?\s+)?are\s+limited\s+to\s+(?:at\s+most|a\s+maximum\s+of)\s+`)
	// complCapLeadRe matches a cap leading a sentence: "Up to".
	complCapLeadRe = regexp.MustCompile(`(?i)^(?:up\s+to|` + noMoreThanPat + `|a\s+maximum\s+of|at\s+most)\b`)
	// complCampusRe matches the campus rule restated: "Courses taught at UTM or
	// UTSC are not included for consideration".
	complCampusRe = regexp.MustCompile(`(?i)^courses\s+taught\s+at\s+UTM\s+or\s+UTSC\s+are\s+not\s+included\s+for\s+consideration\.?$`)
	// complNotCountedRe matches courses that never count, named before the verb:
	// "STA220H1 and STA255H1 are not accepted for the major".
	complNotCountedRe = regexp.MustCompile(`(?i)\b(?:are|is)\s+not\s+(?:accepted|acceptable|included\s+for\s+consideration|an?\s+suitable\s+substitutes?\s+for)\b`)
	// complNotSubstituteRe matches "is not a suitable substitute for": the course
	// doesn't count toward what follows.
	complNotSubstituteRe = regexp.MustCompile(`(?i)\b(?:are|is)\s+not\s+an?\s+suitable\s+substitutes?\s+for\b`)
	// complNotTakenRe matches courses that never count, named after the verb:
	// "We do not accept ENG100H1".
	complNotTakenRe = regexp.MustCompile(`(?i)\b(?:are|is)\s+not\s+permitted\s+to\s+take\b|\bdo\s+not\s+accept\b`)
)

// complOmitRe matches "may omit CSC108H1".
var complOmitRe = regexp.MustCompile(`(?i)\bmay\s+omit\s+(` + codePat + `)\b`)

// omitCovered reports whether the item naming the course a sentence lets
// students omit already offers a route without it. If so, the sentence joins
// that item's raw: the item encodes it.
func (a *compAssembler) omitCovered(s string) bool {
	code := complOmitRe.FindStringSubmatch(s)[1]
	r := a.itemNaming(code)
	if r == nil || r.r.Req == nil || r.r.Req.Type != types.NodeOr {
		return false
	}
	without := slices.ContainsFunc(r.r.Req.Items, func(it *types.ReqNode) bool {
		has := false
		walkReq(it, func(x *types.ReqNode) { has = has || x.Code == code || slices.Contains(x.Courses, code) })
		return !has && it.Type != types.NodeText
	})
	if without {
		addRaw(r, s)
	}
	return without
}

// campusRule puts a sentence restating the campus rule (filters match St.
// George courses only) in Req 1's raw, whose designator pool encodes it. It
// reports false when Req 1 has no such pool.
func (a *compAssembler) campusRule(s string) bool {
	if a.total == nil || a.total.r.Req.Type != types.NodeCredits || a.total.r.Req.Designators == nil || a.total.r.Req.Courses != nil {
		return false
	}
	a.total.r.Raw += "\n" + s
	return true
}

// complOnlyOfRe matches "only 0.5 of ECO220Y1 will be accepted".
var complOnlyOfRe = regexp.MustCompile(`(?i)^(?:please\s+note\s+(?:that\s+)?)?only\s+(?:a\s+)?` + amountPat + `\s+(?:credits?\s+)?of\s+(` + codePat + `)\s+(?:will\s+be|is|may\s+be|can\s+be)\s+(?:accepted|counted|used)\b`)

// complNoMoreThanOneRe matches "No more than one from GGR273H1, GGR373H1".
var complNoMoreThanOneRe = regexp.MustCompile(`(?i)^` + noMoreThanPat + `\s+one\s+(?:of|from)\s+((?:` + codePat + `)(?:\s*(?:,|or|and|/)\s*` + codePat + `)+)$`)

// complCapExceptRe matches the exception a cap states: "except for students
// ...".
var complCapExceptRe = regexp.MustCompile(`(?i)\b(except\s+(?:for|that|when|if)\b.*)$`)

// hasFilter reports whether a limit's node says what it caps: a cap on
// "credits" alone caps nothing.
func hasFilter(n *types.ReqNode) bool {
	return n != nil && (n.Courses != nil || n.Designators != nil || n.Levels != nil || n.Breadth != nil || n.Lists != nil ||
		n.Transfer || n.CrNcr || n.Items != nil || n.ExcludeDesignators != nil || n.MaxCourseAmount != 0)
}

var (
	// complCapWordingRe matches a cap sentence's lead into its amount meaning "No
	// more than": "A maximum of a 0.5 credit from".
	complCapWordingRe = regexp.MustCompile(`(?i)^-?\s*(?:students\s+(?:may|can)\s+use\s+)?(?:(?:a\s+)?maximum\s+of\s+(?:an?\s+)?|at\s+most\s+(?:an?\s+)?|only\s+(?:an?\s+)?)(` + numPat + `\s+credits?\s+(?:from|in|at)\b)`)
	// complCapAnyScopeRe matches where a cap sentence says what it counts toward:
	// " may be counted".
	complCapAnyScopeRe = regexp.MustCompile(`(?i)\s+(?:(?:can|may|will)\s+(?:be\s+)?(?:used|counted|count|applied|accepted)\b|towards?\s+(?:the\s+)?[\w&\s-]*?\b(?:program|requirements?)\b).*$`)
	// complCapNumberedScopeRe matches a cap on numbered requirements: "towards
	// requirements 5". No one limit row can say that, so the sentence stays text.
	complCapNumberedScopeRe = regexp.MustCompile(`(?i)\btowards?\s+(?:the\s+)?requirements?\s+#?\d`)
	// complCapNoteParenRe matches a pointer in brackets: "(See Note 3)".
	complCapNoteParenRe = regexp.MustCompile(`(?i)\s*\((?:see|note)\b[^()]*\)`)
	// complCodeDashTitleRe matches a course's title after it: "CRI389Y0 - Topics
	// in Criminology and Sociolegal Studies Abroad courses".
	complCodeDashTitleRe = regexp.MustCompile(`(` + codePat + `)\s+-\s+[A-Z][^,;]*?\s+courses\b`)
)

// capSentence rewrites a cap written as a sentence into the form the header
// grammar reads ("No more than 0.5 credit from A/ B"), or "" when it isn't one.
func capSentence(t string) string {
	m := complCapWordingRe.FindStringSubmatchIndex(t)
	if m == nil && !complCapLeadRe.MatchString(t) || complCapNumberedScopeRe.MatchString(t) {
		return ""
	}
	if m != nil {
		t = "No more than " + t[m[2]:]
	}
	t = complCapNoteParenRe.ReplaceAllString(t, "")
	t = complCodeDashTitleRe.ReplaceAllString(t, "$1")
	return strings.TrimSpace(complCapAnyScopeRe.ReplaceAllString(t, ""))
}

func capRow(s string) *types.Requirement {
	t := complLimitedToRe.ReplaceAllString(strings.TrimSpace(strings.TrimSuffix(s, ".")), "No more than ")
	for _, u := range []string{t, complCapScopeRe.ReplaceAllString(t, ""), capSentence(t)} {
		rows, tail, ok := parseHeaderFloors(u)
		if ok && len(tail) == 0 && len(rows) == 1 && rows[0].Kind == types.RequirementLimit && hasFilter(rows[0].Req) {
			rows[0].Raw = s
			if m := complCapExceptRe.FindStringSubmatch(s); m != nil {
				// a limit can't hold an alternative: the exception is a note
				rows[0].Notes = []string{cleanNote(m[1])}
			}
			return rows[0]
		}
	}
	if m := complNoMoreThanOneRe.FindStringSubmatch(t); m != nil {
		// at most one course
		return &types.Requirement{Kind: types.RequirementLimit, Raw: s, Req: courseCap(codeRe.FindAllString(m[1], -1))}
	}
	if m := complOnlyOfRe.FindStringSubmatch(t); m != nil {
		// "Please note only 0.5 of ECO220Y1 will be accepted ..."
		v, _ := strconv.ParseFloat(m[1], 64)
		return &types.Requirement{Kind: types.RequirementLimit, Raw: s,
			Req: &types.ReqNode{Type: types.NodeCredits, Courses: []string{m[2]}, MaxAmount: maxAmount(v)}}
	}
	var codes []string
	if loc := complNotCountedRe.FindStringIndex(t); loc != nil && (!codeRe.MatchString(t[loc[1]:]) || complNotSubstituteRe.MatchString(t)) {
		codes = codeRe.FindAllString(t[:loc[0]], -1)
	} else if loc := complNotTakenRe.FindStringIndex(t); loc != nil && !codeRe.MatchString(t[:loc[0]]) {
		codes = codeRe.FindAllString(t[loc[1]:], -1)
	}
	if codes == nil {
		return nil
	}
	return &types.Requirement{Kind: types.RequirementLimit, Raw: s,
		Req: &types.ReqNode{Type: types.NodeCredits, Courses: dedupe(codes), MaxAmount: maxAmount(0)}}
}

var (
	// complTransferCapRe matches "No more than 1.0 transfer credit(s) will be
	// eligible for consideration in the Certificate".
	complTransferCapRe = regexp.MustCompile(`(?i)^(?:` + noMoreThanPat + `|a\s+maximum\s+of|up\s+to)\s+(?:a\s+)?` + amountPat + `\s+(?:credits?\s+in\s+)?transfer\s+credits?\s+(?:will\s+be\s+eligible\s+for\s+consideration|may\s+be\s+(?:counted|applied|used)\s+(?:to(?:wards?)?|for))\s+(?:in\s+)?the\s+(?:certificate|program|minor|major|specialist|focus)$`)
	// complTransferComprisesRe matches "Transfer credits cannot comprise more
	// than 0.5 credit at the 300-/400-level".
	complTransferComprisesRe = regexp.MustCompile(`(?i)^transfer\s+credits\s+cannot\s+comprise\s+more\s+than\s+(?:a\s+)?` + amountPat + `\s+credits?(?:\s+at\s+the\s+(` + levelPat + `[-/\s\d+]*-?\s*levels?))?$`)
)

// transfer reads a cap on transfer credit into a limit row under target.
func (a *compAssembler) transfer(target *compRow, s string) {
	t := strings.TrimSpace(strings.TrimSuffix(s, "."))
	if r, text, ok := transferRows(t); ok {
		// with the exchange exception
		r.Raw = s
		a.rowUnder(target, r)
		if text {
			a.textRow(target, s)
		}
		return
	}
	n := &types.ReqNode{Type: types.NodeCredits, Transfer: true}
	if m := complTransferCapRe.FindStringSubmatch(t); m != nil {
		v, _ := strconv.ParseFloat(m[1], 64)
		n.MaxAmount = maxAmount(v)
	} else if m := complTransferComprisesRe.FindStringSubmatch(t); m != nil {
		v, _ := strconv.ParseFloat(m[1], 64)
		n.MaxAmount = maxAmount(v)
		n.Levels = parseLevels(m[2])
	} else {
		a.textRow(target, s)
		return
	}
	a.rowUnder(target, &types.Requirement{Kind: types.RequirementLimit, Req: n, Raw: s})
}

// complCrNcrBanRe matches a rule that courses taken CR/NCR don't count: "We
// do not accept any CR/NCR courses toward our program".
var complCrNcrBanRe = regexp.MustCompile(`(?i)^(?:courses\s+(?:that\s+have\s+been\s+taken\s+as|with\s+a)\s+(?:credit\s*/\s*no\s+credit|CR\s*/\s*NCR)(?:\s+status)?\s+(?:cannot|may\s+not)\s+be\s+used\s+to\s+(?:fulfill|satisfy)\s+(?:the\s+)?(?:certificate|program)\s+requirements|we\s+do\s+not\s+accept\s+any\s+CR\s*/\s*NCR\s+courses\s+toward\s+our\s+programs?)$`)

// complCrNcrMayRe matches CR/NCR allowed: "may use".
var complCrNcrMayRe = regexp.MustCompile(`(?i)\b(?:may|can)\s+(?:use|take)\b`)

// crNcr reads a rule on CR/NCR courses into a limit row under target.
func (a *compAssembler) crNcr(target *compRow, s string) {
	if a.crNcrOutside(target, s) {
		// CR/NCR or other coursework, one share
		return
	}
	if rows, text := crNcrRows(a.clean(s)); rows != nil {
		// codes beside the CR/NCR courses, other wordings, a share
		for _, r := range rows {
			r.Raw = s
			a.rowUnder(target, r)
		}
		if text {
			a.textRow(target, s)
		}
		return
	}
	if !complCrNcrBanRe.MatchString(strings.TrimSpace(strings.TrimSuffix(s, "."))) {
		if t := a.clean(s); complCrNcrMayRe.MatchString(t) && !complNegationRe.MatchString(t) && !complBindingWordRe.MatchString(t) {
			// "Students may use CR/NCR on the course ...": a permission, a note
			a.notesTo(a.noteTarget(target), []string{cleanNote(s)})
			return
		}
		a.textRow(target, s)
		return
	}
	a.rowUnder(target, &types.Requirement{Kind: types.RequirementLimit, Raw: s,
		Req: &types.ReqNode{Type: types.NodeCredits, CrNcr: true, MaxAmount: maxAmount(0)}})
}

// isApproval reports whether s is an approval route: "Other introductory
// language classes may be approved by the Director".
func (a *compAssembler) isApproval(s string) bool {
	return a.cue(s).approval
}

// complAlternativeRe matches a sentence offering another way to meet a
// requirement: "CSB450H1 is also acceptable".
var complAlternativeRe = regexp.MustCompile(`(?i)\balso\s+(?:be\s+)?(?:used|count(?:s|ed)?|accepted|acceptable|eligible|qualif\w*|fulfil\w*|satisf\w*|meet|complete)\b|\b(?:is|are)\s+also\s+acceptable\b|\bsubstitute\b|\bqualif(?:y|ies)\b|\b(?:meet|fulfil+|satisfy)\s+(?:this|the)\s+(?:[\w-]+\s+){0,3}requirement\b|\bcounts?\s+towards?\b|\bbe\s+counted\s+towards?\b|\bin\s+lieu\b|\b(?:can|may)\s+be\s+used\b|\bwill\s+be\s+accepted\b|\bacceptable\b`)

// isAlternative reports whether s offers another way to meet a requirement,
// or widens its pool with a cap.
func (a *compAssembler) isAlternative(s string) bool {
	return a.cue(s).alternative
}

// sentenceCues is what the assembler asks about a sentence besides its role.
type sentenceCues struct{ approval, alternative bool }

// cue reads s's cues once.
func (a *compAssembler) cue(s string) sentenceCues {
	c, ok := a.cues[s]
	if !ok {
		t := a.clean(s)
		c.approval = matchAny(complApprovalRes, t) || complCountQueryRe.MatchString(t)
		c.alternative = !complNegationRe.MatchString(complNotListedRe.ReplaceAllString(t, "")) && complAlternativeRe.MatchString(t) || complWidenRe.MatchString(t)
		a.cues[s] = c
	}
	return c
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

// lineSentences returns line i's sentences with their emphasis stripped,
// split once for the whole text. Callers must not change them.
func (a *compAssembler) lineSentences(i int) []string {
	if a.lineSents == nil {
		a.lineSents = make([][]string, len(a.lines))
		for k, l := range a.lines {
			a.lineSents[k] = splitComplSentences(stripEmphasis(strings.TrimSpace(l)))
		}
	}
	return a.lineSents[i]
}

// complNegationRe matches a sentence that refuses an alternative: "cannot".
var complNegationRe = regexp.MustCompile(`(?i)\bnot\b|\bcannot\b|n['’]t\b`)

// alternative adds s as another way to meet row: or(strict, text), its kind
// saying what decides it. A permission naming its wider pool is bounded by it
// once every row is read.
func (a *compAssembler) alternative(row *compRow, s string) {
	a.addRoute(row, s, textNode(altKind(s), s))
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

// altKind is the kind of an alternative's text: a permission when a person
// decides, else other. Never membership: "count toward the program" is about
// courses, not programs.
func altKind(s string) types.TextKind {
	if classify(cleanNote(s)) == types.TextPermission {
		return types.TextPermission
	}
	return types.TextOther
}

var (
	// complRefRe matches references to numbered requirements: "requirements (4)
	// and (5)".
	complRefRe    = regexp.MustCompile(`(?i)\brequirements?\s+\(?\d{1,2}[a-z]?\)?(?:\s*(?:,|and|or)\s*\(?\d{1,2}[a-z]?\)?)*`)
	complRefNumRe = regexp.MustCompile(`\d{1,2}[a-z]?`)
)

// referenced returns the items a sentence names by number ("may also count
// towards requirement 2"), or nil when a number names no item or several.
func (a *compAssembler) referenced(s string) []*compRow {
	var out []*compRow
	for _, num := range complRefNumRe.FindAllString(complRefRe.FindString(s), -1) {
		var found *compRow
		for _, r := range a.items {
			if r.r.Label != num && !strings.HasSuffix(r.r.Label, " "+num) {
				continue
			}
			if found != nil {
				return nil
			}
			found = r
		}
		if found == nil {
			return nil
		}
		out = append(out, found)
	}
	return out
}

// complSatisfiesCountsRe matches a course that meets one requirement and
// counts part of its credit toward another: "PHY479Y1 ... satisfies
// Requirement 3 ... and counts as a 0.5 credit ... for Requirement 4 ...".
var complSatisfiesCountsRe = regexp.MustCompile(`(?i)^(` + codePat + `)(?:\s*\([^()]*\))?\s+satisfies\s+(requirement\s+\d{1,2}[a-z]?(?:\s+in\s+.+?)?)\s+and\s+counts\s+as\s+an?\s+` + amountPat + `\s+credit\b.*?\bfor\s+(requirement\s+\d{1,2}[a-z]?(?:\s+in\s+.+?)?)\.?$`)

// satisfiesAndCounts splits a complSatisfiesCountsRe course's credit: the
// rest is an alternative on the first requirement, and the second takes the
// sentence as or(strict, text), since a pool can't count part of one course.
func (a *compAssembler) satisfiesAndCounts(s string) bool {
	m := complSatisfiesCountsRe.FindStringSubmatch(strings.TrimSpace(a.clean(s)))
	if m == nil {
		return false
	}
	meets, counts := a.referencedIn(m[2]), a.referencedIn(m[4])
	v, _ := strconv.ParseFloat(m[3], 64)
	rest := courseWeight(m[1]) - v
	if meets == nil || counts == nil || meets == counts || meets.r.Req == nil || counts.r.Req == nil || rest <= 0 {
		return false
	}
	meets.r.Req = orNode(meets.r.Req, &types.ReqNode{Type: types.NodeCredits, Amount: rest, Courses: []string{m[1]}, Weight: rest})
	addRaw(meets, s)
	a.wrapRoute(counts, nil, textNode(types.TextOther, s))
	addRaw(counts, s)
	return true
}

// referencedIn returns the one item a phrase names by number ("Requirement 3
// in Third or Fourth Year"), narrowed by the section after "in" when several
// share it; nil when none or several remain.
func (a *compAssembler) referencedIn(phrase string) *compRow {
	num := complRefNumRe.FindString(phrase)
	sec := ""
	if i := strings.Index(strings.ToLower(phrase), " in "); i >= 0 {
		sec = strings.ToLower(strings.TrimSpace(phrase[i+4:]))
	}
	var found []*compRow
	for _, r := range a.items {
		if r.r.Label == num || strings.HasSuffix(r.r.Label, " "+num) {
			found = append(found, r)
		}
	}
	if len(found) > 1 && sec != "" {
		found = slices.DeleteFunc(found, func(r *compRow) bool { return strings.ToLower(r.r.Section) != sec })
	}
	if len(found) != 1 {
		return nil
	}
	return found[0]
}

// approval adds a route only a person can grant to target as or(strict,
// permission text); a pool it names bounds it later. A program-level route
// waits until every row is read, then goes to the rows it names, else a note.
func (a *compAssembler) approval(target *compRow, s string) {
	a.addRoute(target, s, textNode(types.TextPermission, cleanNote(s)))
}

// addRoute adds route s to row as or(strict, alt). A nil row is the
// program's, held until every row is read.
func (a *compAssembler) addRoute(row *compRow, s string, alt *types.ReqNode) {
	if row == nil {
		a.programRoutes = append(a.programRoutes, s)
		return
	}
	if row.r.Req == nil {
		// a group: the route is a part's alternative, not the group's
		a.textRow(row, s)
		return
	}
	row.r.Req = orNode(row.r.Req, alt)
	addRaw(row, s)
}

// exclusion reads a sentence naming programs that can't be combined with
// this one, as enrolment does; a phrasing only completion text has stays
// membership text.
func (a *compAssembler) exclusion(s string) {
	t := strings.TrimSpace(strings.TrimSuffix(a.clean(s), "."))
	if nodes, ok, _ := exclusionNodes(t); ok && len(nodes) > 0 {
		a.exclusions = append(a.exclusions, nodes...)
		return
	}
	if nodes := complExclusionNodes(t); nodes != nil {
		a.exclusions = append(a.exclusions, nodes...)
		return
	}
	a.exclusions = append(a.exclusions, textNode(types.TextMembership, cleanNote(s)))
}

var (
	// complConcurrentRe matches "Students are not allowed to enroll concurrently
	// in the Major Program in Pharmacology and the Major Program in Toxicology".
	// One of the two is this program, which Link drops.
	complConcurrentRe = regexp.MustCompile(`(?i)\bnot\s+(?:be\s+)?allowed\s+to\s+enrol(?:l)?\s+concurrently\s+in\s+(?:the\s+|a\s+)?(.+?)\s+and\s+(?:the\s+|a\s+)?(.+)$`)
	// complSimultaneousRe matches "Students registered for a German Specialist or
	// Major may not simultaneously": the programs in the subject.
	complSimultaneousRe = regexp.MustCompile(`(?i)^students\s+(?:enrolled|registered)\s+(?:in|for)\s+(?:the\s+|a\s+|an\s+)?(.+?)\s+may\s+not\s+simultaneously\b`)
	// complAlsoCompleteRe matches "cannot also complete the Applied Genetics and
	// Biotechnology Major".
	complAlsoCompleteRe = regexp.MustCompile(`(?i)\bcannot\s+also\s+complete\s+(.+)$`)
	// complOtherProgramRe matches "cannot be enrolled in any other program in
	// Economics".
	complOtherProgramRe = regexp.MustCompile(`(?i)\b(?:cannot\s+be\s+enrol(?:l)?ed\s+in|cannot\s+be\s+paired\s+with)\s+(any\s+other\s+.+)$`)
	// complExclusionLeadRe matches a label or list marker before an exclusion:
	// "Note: (a)".
	complExclusionLeadRe = regexp.MustCompile(`(?i)^(?:note:\s*)?(?:\([a-z]\)\s*)?`)
)

// complExclusionNodes reads the cannot-combine phrasings only completion text
// has, or returns nil.
func complExclusionNodes(t string) []*types.ReqNode {
	t = complExclusionLeadRe.ReplaceAllString(t, "")
	switch {
	case complConcurrentRe.MatchString(t):
		m := complConcurrentRe.FindStringSubmatch(t)
		return append(exclusionPhraseNodes(m[1]), exclusionPhraseNodes(m[2])...)
	case complSimultaneousRe.MatchString(t):
		return exclusionPhraseNodes(complSimultaneousRe.FindStringSubmatch(t)[1])
	case complAlsoCompleteRe.MatchString(t):
		return exclusionPhraseNodes(complAlsoCompleteRe.FindStringSubmatch(t)[1])
	case complOtherProgramRe.MatchString(t):
		return []*types.ReqNode{{Type: types.NodeProgram, Text: complOtherProgramRe.FindStringSubmatch(t)[1]}}
	}
	return nil
}

// claimable reports whether n only needs courses: courses, credit amounts and
// their and, or and choose. No text, cap or average.
func claimable(n *types.ReqNode) bool {
	return structureOnly(n, func(x *types.ReqNode) bool {
		return x.Type == types.NodeChoose ||
			x.Type == types.NodeCredits && x.Amount > 0 && x.MaxAmount == nil && !isAnyCredits(x)
	})
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

// sharesCourses reports whether a course could count toward both x and y.
// Text doesn't count: it is Review whatever courses it is given.
func sharesCourses(x, y *types.ReqNode) bool {
	cx, fx := poolParts(x)
	cy, fy := poolParts(y)
	for _, c := range cx {
		if slices.Contains(cy, c) || slices.ContainsFunc(fy, func(f *types.ReqNode) bool { return filterMatches(f, c) }) {
			return true
		}
	}
	for _, c := range cy {
		if slices.ContainsFunc(fx, func(f *types.ReqNode) bool { return filterMatches(f, c) }) {
			return true
		}
	}
	for _, f := range fx {
		for _, g := range fy {
			if filtersMeet(f, g) {
				return true
			}
		}
	}
	return false
}

// poolParts collects the codes and the filters (credits nodes matching by
// designator, level or breadth, or any course) in n.
func poolParts(n *types.ReqNode) (codes []string, filters []*types.ReqNode) {
	walkReq(n, func(x *types.ReqNode) {
		switch x.Type {
		case types.NodeCourse:
			codes = append(codes, x.Code)
		case types.NodeCredits, types.NodeAverage:
			codes = append(codes, x.Courses...)
			if x.Designators != nil || x.Levels != nil || x.Breadth != nil || x.Courses == nil {
				filters = append(filters, x)
			}
		}
	})
	return codes, filters
}

// filtersMeet reports whether some course could match both filters.
func filtersMeet(f, g *types.ReqNode) bool {
	if f.Designators != nil && g.Designators != nil && !slices.ContainsFunc(f.Designators, func(d string) bool { return slices.Contains(g.Designators, d) }) {
		return false
	}
	if f.Levels != nil && g.Levels != nil && !slices.ContainsFunc(f.Levels, func(l int) bool { return slices.Contains(g.Levels, l) }) {
		return false
	}
	return true
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

// complRequiredSentenceRe matches courses a sentence says are required:
// "PHL271H1, TRN203H1, and TRN204H1 are required courses".
var complRequiredSentenceRe = regexp.MustCompile(`(?i)^(.*\b` + codePat + `)\s+(?:are|is)\s+(?:the\s+)?required(?:\s+courses?)?\b`)

// complOnlySomeRe matches a requirement only some students have: "for
// students who began".
var complOnlySomeRe = regexp.MustCompile(`(?i)\b(?:for\s+(?:students|those)|if|unless|when|only\s+for|in\s+case)\b`)

// complThatRe matches the "that" before advice after complRequiredSentenceRe.
var complThatRe = regexp.MustCompile(`(?i)^\s*,?\s*(?:that|which)\s+`)

func fixedCourses(n *types.ReqNode) bool {
	if n.Type == types.NodeCourse {
		return true
	}
	return n.Type == types.NodeAnd && !slices.ContainsFunc(n.Items, func(it *types.ReqNode) bool { return it.Type != types.NodeCourse })
}
