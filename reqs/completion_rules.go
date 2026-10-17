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

// Sentences into rows: what each role does with a sentence, the body of an
// item and the rules written in it (floors, caps, CR/NCR and transfer
// rules, approval routes, alternatives), exclusions, and the checks that
// keep a reading the grammar can't be sure of as text.

// sentence places one sentence of an item (row, nil at the program level)
// and returns the advice to note on it. A first requirement sentence is
// row's body.
func (a *compAssembler) sentence(s string, row *compRow, ctx compCtx) (notes []string) {
	defer func() { a.lastSentence = s }()
	return a.sentenceAs(a.role(s), s, row, ctx)
}

// complThisRequirementRe: an alternative that points at the rule just stated:
// "Students who complete the ASIP stream will also meet this requirement".
var complThisRequirementRe = regexp.MustCompile(`(?i)\bwill\s+also\s+meet\s+this\s+requirement\b`)

// lastRule returns the rule row the sentence before s made under row (a
// constraint of text: "The choices in 3 must satisfy the requirement for an
// integrative, inquiry-based activity by including one of ..."), or nil.
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

// sentenceAs places a sentence with the given role (sentence).
func (a *compAssembler) sentenceAs(role sentenceRole, s string, row *compRow, ctx compCtx) (notes []string) {
	// The row the sentence's rules apply to: the item it is written in once
	// that has a body, else the item around it, else the program.
	target := ctx.owner
	bodied := row != nil && (row.r.Req != nil || len(row.children) > 0)
	if bodied {
		target = row
	}
	if r := courseCountCap(a.clean(s)); r != nil {
		// A cap counted in courses, besides what else the sentence says
		// (completion_limits.go): the program's.
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
			// "The remainder must be selected from the following: ...": a
			// rule over a list, not just the list; Review, not a note.
			a.textRow(target, s)
			return nil
		}
		// A named list ("Group A: CRI300H1/ ..."): a note, unless an item
		// refers to it (namedLists).
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
			// "CSC108H1 is waived for students who complete CSC148H1 first":
			// an alternative on the rows requiring CSC108H1 (relax).
			a.relaxations = append(a.relaxations, s)
		case a.satisfiesAndCounts(s):
			// "PHY479Y1 ... satisfies Requirement 3 in Third or Fourth Year
			// and counts as a 0.5 credit at the PHY 400-level for Requirement
			// 4 in Third or Fourth Year".
		case ctx.advice && !a.isApproval(s):
			return []string{cleanNote(s)}
		case complEquivalentsRe.MatchString(a.clean(s)):
			// "... would be able to use any completed RSM course equivalents
			// ... (e.g. RSM100H1 would be deemed equivalent to MGT100H1, etc)":
			// an alternative on each row requiring a MGT course.
			rs, replaced := a.relaxedRows(s)
			if rs == nil {
				return []string{cleanNote(s)}
			}
			for _, r := range rs {
				a.wrapRoute(r, replaced, textNode(altKind(s), s))
				addRaw(r, s)
			}
		case !bodied && ref != nil && a.isApproval(s):
			// "Notes 1. Additional courses with significant Canadian content
			// may be considered for approval for Requirement 7 by the Program
			// Director": a route on requirement 7, not an item.
			for _, r := range ref {
				a.approval(r, s)
			}
		case !bodied && row != nil && a.isApproval(s) && (complCodeLeadRe.MatchString(a.clean(s)) || isComplAllocLead(a.clean(s))):
			// "4. CHM395Y1/ CHM499Y1/ PHC489Y1 and/or CHM397H0/ CHM398H0 ...
			// with prior approval by the Department": an item that needs an
			// approval.
			body, rows, notes := a.itemBody(s)
			row.r.Req = body
			for _, r := range rows {
				a.rowUnder(row, r)
			}
			return notes
		case bodied && a.substitutedFrom(row, s):
			// "A maximum of 0.5 credit may be substituted from the following
			// first-year courses: POL191H1, ...".
		case complNotSubstituteRe.MatchString(a.clean(s)) && capped() != nil:
			// "MAT133Y1 is not a suitable substitute for ( MAT130H1/ ...)": it
			// doesn't count toward the requirement it names.
			t := target
			if r := a.itemNaming(s[complNotSubstituteRe.FindStringIndex(s)[1]:]); r != nil && !bodied {
				t = r
			}
			a.rowUnder(t, capped())
		case !bodied && ref != nil && a.isAlternative(s):
			// "Some offerings of POL336H1; ... may also count towards
			// requirement 2 of this focus at the department's discretion",
			// "Students may also satisfy requirements (4) and (5) through
			// completion of a senior essay ( FRE490Y1)".
			for _, r := range ref {
				a.alternative(r, s)
			}
		case complCapLeadRe.MatchString(a.clean(s)) && capped() != nil:
			// "Up to 1.0 credit in HPS courses at the 100-level will be
			// counted towards the minor": a cap.
			a.rowUnder(target, capped())
		case complOmitRe.MatchString(s) && a.omitCovered(s):
			// "Students with a strong background in ... may omit CSC108H1
			// and proceed directly with CSC148H1": the item already offers
			// routes without CSC108H1.
		case complCampusRe.MatchString(a.clean(s)) && a.campusRule(s):
		case a.isApproval(s):
			a.approval(target, s)
		case bodied && a.isAlternative(s) && complThisRequirementRe.MatchString(s) && a.lastRule(row) != nil:
			// The ASIP route of the integrative requirement stated just before
			// it, not of the item.
			k := a.lastRule(row)
			k.r.Req = orNode(k.r.Req, textNode(altKind(s), s))
			addRaw(k, s)
		case bodied && a.isAlternative(s):
			a.alternative(row, s)
		case !bodied && complWidenRe.MatchString(a.clean(s)):
			// "Up to 1.0 credit may be from other departments": the pool of
			// the item around it, or the program's.
			a.alternative(ctx.owner, s)
		case !bodied && (ctx.notes || row == nil) && a.isAlternative(s):
			// "Students may count ECO210H1 towards the Specialist program in
			// lieu of a 0.5 credit 300-level ECO elective".
			a.alternative(ctx.owner, s)
		case row == nil || ctx.notes && !bodied:
			// A requirement that isn't an item, or written as a note: a row
			// of text, unless it only permits or defines (looseSentence).
			a.looseSentence(target, s)
		case !bodied && a.appliedTo(s):
			// "*Students admitted to the program with ECO101H1/ ...: The
			// course will be applied to Requirement #3 (Higher Years)": a
			// note on that row (completion_limits.go).
		case !bodied && !ctx.numbered && looseMain(s):
			// An unnumbered line that only permits: "2.0 credits may be taken
			// from courses offered by other departments, pending approval",
			// not an item.
			a.looseSentence(ctx.owner, s)
		case !bodied:
			body, rows, notes := a.itemBody(s)
			if t := a.clean(s); body != nil && body.Type == types.NodeText && !ctx.numbered && !codeRe.MatchString(t) && !complAmountRe.MatchString(t) &&
				!complInheritRe.MatchString(t) {
				// An unnumbered line of prose that names no course or amount
				// ("Permission to count courses towards the Minor ... will be
				// granted on a case-by-case basis", "An Integrative,
				// Inquiry-Based Activity Requirement must be satisfied.") is
				// no allocation: a route, a rule of text or a note.
				switch {
				case body.Kind == types.TextPermission:
					a.approval(nil, s)
				case complRequirementLeadRe.MatchString(t):
					return []string{cleanNote(s)}
				case a.isAlternative(s):
					// "Health-related Anthropology Special Topics courses at
					// the 300 or 400 level ... will count towards program
					// requirements": a route onto the rows it describes.
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
			// Another requirement in the item: its "and", as a sentence
			// break is everywhere, unless the grammar can't read all of it;
			// a sentence that only permits or advises is no part of the and
			// ("Either MUS120Y1 or MUS220Y1 can be counted towards the 2.5
			// credits", "The 1.5 credits can be chosen to reflect ...").
			if a.conditionalExtra(row, s) {
				return nil
			}
			if t := a.clean(s); complCapLeadRe.MatchString(t) && (a.isApproval(s) || complApprovalWordRe.MatchString(t)) {
				// "Up to 0.5 credit from GGR491Y1, ... based on suitability of
				// research topic for this program, and approval by department":
				// a route into the item, not part of it.
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
	// A list marker glued to a bracket, which the block reader leaves in the
	// text: "4.( CHM220H1/ CHM222H1 is strongly recommended.)".
	complGluedMarkerRe = regexp.MustCompile(`^\d{1,2}[.)]\s*\(`)
	// Brackets that are a remark on the item, not part of its requirement:
	// "( CHM135H1 and CHM136H1)/ CHM151Y1 (Only transfer credits that carry
	// exclusions to CHM136H1 and/or CHM135H1 will be accepted)", "(
	// PSL300H1, PSL301H1) (NOTE: PSL201Y1 is not acceptable)".
	complRemarkParenRe = regexp.MustCompile(`\s*\(((?:[A-Za-z]+:\s*)?[A-Z][^()]*\s[^()]*\s[^()]*)\)`)
)

// itemBody parses an item's first requirement sentence into its body, and
// the rows written inside it: a floor in brackets after the amount ("1.5
// credits (at least 0.5 credit at the 400-level) from: ..."), a course in
// brackets that doesn't count ("(NOTE: PSL201Y1 is not acceptable)"). Other
// remarks in brackets that are advice are notes.
func (a *compAssembler) itemBody(s string) (body *types.ReqNode, rows []*types.Requirement, notes []string) {
	if n, rs, ok := floorFromDesignator(s); ok {
		return n, rs, nil
	}
	body, rows, notes = a.grammarBody(s)
	if body == nil || hasText(body) {
		// A remark in brackets, a clause on courses already used
		// (completion_remarks.go).
		if n, rs, ns, ok := a.remarkItem(s); ok {
			return n, rs, ns
		}
	}
	return body, rows, notes
}

// grammarBody is itemBody without remarkItem.
func (a *compAssembler) grammarBody(s string) (body *types.ReqNode, rows []*types.Requirement, notes []string) {
	s = complGluedMarkerRe.ReplaceAllString(strings.TrimSpace(s), "(")
	// Before the floors are taken out (splitQualified): one-off wordings
	// (completion_fixes.go), as rewriteCompletion reads them.
	s = rewriteOneOffs(s)
	if n := profileBody(s); n != nil {
		// A learning community, a language proficiency (completion_limits.go).
		return n, nil, nil
	}
	if n, rows, notes, ok := a.pairPool(s); ok {
		// Single courses and pairs the item grammar leaves as text
		// (completion_limits.go).
		return n, rows, notes
	}
	if m := complRequiredSentenceRe.FindStringSubmatch(s); m != nil && !complOnlySomeRe.MatchString(m[1]) && !complOnlySomeRe.MatchString(s[len(m[0]):]) {
		// "PHL271H1, TRN203H1, and TRN204H1 are required courses that
		// students are recommended to take in second year (...)": the
		// courses, and the sentence as advice.
		if n, _ := parseCompletionText(m[1]); n != nil && fixedCourses(n) {
			if advice := strings.TrimSpace(complThatRe.ReplaceAllString(s[len(m[0]):], "")); advice != "" && advice != "." {
				notes = []string{cleanNote(advice)}
			}
			return n, nil, notes
		}
	}
	// widen: "; up to 0.5 credit of this requirement can be satisfied by
	// taking one of the following ethics courses: ...": more courses for the
	// pool, capped (widenPool). When the pool doesn't take them, the item is
	// text.
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
		// "0.5 credit from any HPS 100-level courses; up to 1.0 credit at
		// the 100 level will count towards the major": a cap clause is the
		// item's own limit.
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
	// "1.5 credits of any of the following (at least a 0.5 credit must be
	// from LIN): A/ B/ ...": the pool, and its floor in brackets.
	s = complOfAnyFollowingRe.ReplaceAllString(s, "$1 $2 from:")
	if m := complItemFloorParenRe.FindStringSubmatch(s); m != nil {
		s = strings.TrimSpace(m[1] + " " + m[3])
		rows = append(rows, a.floorRows(m[2])...)
	}
	s = complLowerRemarkRe.ReplaceAllStringFunc(s, func(p string) string {
		// "MAT223H1/ MAT240H1 (should be taken in first year, enforced as a
		// prerequisite for MAT237Y1)": advice on when to take it.
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
			// "PSY100H1 (NOTE: We will also accept PSY100Y5 from UTM, or both
			// of PSYA01H3 and PSYA02H3 together from UTSC, in place of
			// PSY100H1 for this requirement)": the courses it names.
			courseAlts = append(courseAlts, alsoAccepted(a.clean(inner)))
		case role == roleRequirement && a.isAlternative(inner) && !complNegationRe.MatchString(inner):
			// "PSY100H1 (NOTE: We will also accept PSY100Y5 from UTM, ...)":
			// another way to meet the item.
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
		// "CHM395Y1/ CHM499Y1/ PHC489Y1 and/or CHM397H0/ CHM398H0 in areas
		// relevant to the program with prior approval by the Department": the
		// approval is for the courses after "and/or".
		strict, _ := parseCompletionText(m[1])
		approved, _ := parseCompletionText(m[2])
		return orNode(strict, andNode(approved, textNode(types.TextPermission, m[3]))), rows, notes
	}
	if m := complPermListRe.FindStringSubmatch(strings.TrimSpace(s)); m != nil {
		// "1.0 credit from: AFR353H1/ ... RLG309H1 or with permission of the
		// Director: HIS304H1/ ... TRN377Y1": the list, or with permission the
		// same amount from both lists (design 3.3, bounded).
		return permissionList(m), rows, notes
	}
	if m := complWithAverageRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(s), ".")); m != nil {
		// "( ACT240H1 and ACT245H1 and ACT247H1) with an average grade of
		// 63%": the courses, and their average.
		codes := codeRe.FindAllString(m[1], -1)
		grade, _ := strconv.Atoi(m[2])
		var items []*types.ReqNode
		for _, c := range codes {
			items = append(items, &types.ReqNode{Type: types.NodeCourse, Code: c})
		}
		items = append(items, &types.ReqNode{Type: types.NodeAverage, MinGrade: grade, Courses: codes})
		return andNode(items...), rows, notes
	}
	if m := complAnyRemainingRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(s), ".")); m != nil {
		// "1.0 credit from ACT351H1/ ..., CSC343H1, or any remaining 300+
		// level STA courses not used toward other program requirements -
		// STA304H1, ..., STA496H1/ STA497H1 ( STA399H1 and STA399Y1 may be
		// considered ... with the approval of ...)": the listed courses are
		// all the pool's; only the bracketed ones need approval.
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
		// "0.5 LAS credit at the 400-level, or an equivalent 0.5 credit at
		// the 400-level, approved by the Associate Chair ...": bounded
		// (design 3.3).
		strict, _ := parseCompletionText(m[1])
		wide, _ := parseCompletionText(m[2])
		if strict = checkedBody(strict, m[1]); strict != nil && wide != nil && !hasText(strict) && !hasText(wide) {
			return orNode(strict, andNode(wide, textNode(types.TextPermission, m[3]))), rows, notes
		}
	}
	if m := complOrAnotherRe.FindStringSubmatchIndex(s); m != nil {
		// "AST310H1/ ENV222H1/ ..., or another course with a significant
		// emphasis on Social and Ethical Responsibility approved by the
		// Undergraduate Chair": the listed courses, or one a person approves
		// (design 3.3, a route that names no pool).
		head, more := parseCompletionText(s[:m[0]])
		head = checkedBody(head, s[:m[0]])
		if head != nil && head.Type == types.NodeAnd && !slices.ContainsFunc(head.Items, func(n *types.ReqNode) bool { return n.Type != types.NodeCourse }) {
			head = orNode(head.Items...) // "A, B, C, or ...": one of them
		}
		alt := textNode(types.TextPermission, s[m[2]:m[3]])
		return orNode(head, alt), rows, append(notes, more...)
	}
	if m := complUpToFromListRe.FindStringSubmatchIndex(s); m != nil {
		// "Any 4 courses (2.0 credits) from GGR201H1/ JEG201H1, ..., up to
		// one half course from Group H.": a capped share from a list, which
		// widens the pool; until lists are read, an alternative (Review).
		alts = append(alts, strings.TrimSpace(s[m[2]:m[3]]))
		s = strings.TrimSpace(s[:m[0]])
	}
	n, more := parseCompletionText(s)
	n = topicMembers(checkedBody(n, s), topicCodes(s))
	if hasText(n) {
		// "AMS200H1, AMS201H1, AMS300H1 (total of 1.5 credit)", "POL101H1
		// (formerly POL101Y1), ...", "... (see NOTE 2)", "(Note: this does
		// not fulfil the 400-level requirement as described in #5 above.)":
		// an aside in brackets kills the reading; without it the item reads,
		// and the aside is its note.
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
	// complAlsoAcceptRe: named courses accepted for the item: "We will also
	// accept PSY100Y5 from UTM, or both of PSYA01H3 and PSYA02H3 together
	// from UTSC, in place of PSY100H1 for this requirement".
	complAlsoAcceptRe = regexp.MustCompile(`(?i)^we\s+will\s+also\s+accept\s+(.+?)(?:,?\s+in\s+place\s+of\s+` + codePat + `\b.*)?$`)
	// complAlsoAcceptNoiseRe: the words around the codes that say where and
	// how.
	complAlsoAcceptNoiseRe = regexp.MustCompile(`(?i)\s*\bfrom\s+(?:UTM|UTSC)\b|\bboth\s+of\s+|\s+together\b`)
	// complAlsoAcceptOrRe: what separates the alternatives a complAlsoAcceptRe
	// sentence accepts.
	complAlsoAcceptOrRe = regexp.MustCompile(`(?i),?\s+or\s+`)
	// complAlsoAcceptAndRe: what may join the codes of one alternative.
	complAlsoAcceptAndRe = regexp.MustCompile(`(?i)\band\b|[,\s]`)
)

// alsoAccepted reads a complAlsoAcceptRe sentence into the courses it accepts:
// alternatives joined by "or", each a course or courses joined by "and";
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
		var nodes []*types.ReqNode
		for _, c := range codes {
			nodes = append(nodes, &types.ReqNode{Type: types.NodeCourse, Code: c})
		}
		alts = append(alts, andNode(nodes...))
	}
	return orNode(alts...)
}

// orToPool reads "0.5 credit from the following set of 100-level courses:
// BIO120H1/ ... ( MAT158H1, MAT159H1/ MAT157Y1)/ ..." as the pool it says,
// credits{0.5, every course}, when the grammar made it alternatives and one
// of them weighs more than the amount: and(MAT158H1, MAT159H1) for 0.5
// credit would fail a student who took MAT158H1 alone.
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

// complDepartmentGroupRe: a list of ";" groups of one department each, after an
// amount: "1.0 credit from ECO220Y1, ECO227Y1; GGR270H1, GGR271H1;
// STA220H1, STA221H1".
var complDepartmentGroupRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+from:?\s+(.+)$`)

// complCommaSpaceRe: what may join a group's codes (departmentGroups).
var complCommaSpaceRe = regexp.MustCompile(`[,\s]`)

// departmentGroups reads a complDepartmentGroupRe pool as the or of its groups,
// each a department's sequence: the courses that make up the amount
// together (GGR270H1 and GGR271H1), or each course that does alone
// (ECO220Y1 or ECO227Y1). Nil unless there are three or more groups, each of
// one designator, each reading one of those ways: the union of all their
// courses would count GGR270H1 with STA220H1.
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

// complBracketPairRe: a bracketed group of alternatives inside a "/" list:
// "ENV221H1/ ENV222H1/ ( EEB240H1/ ENV234H1)/ ENV337H1".
var complBracketPairRe = regexp.MustCompile(`\(\s*(` + codePat + `(?:\s*/\s*` + codePat + `)+)\s*\)`)

// bracketPairCaps caps a bracketed group of alternatives flattened into pool
// n at its heaviest course, as pairCaps does for a "/" pair (design 3.2).
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
	// complAsideParenRe: brackets that remark on an item rather than require:
	// "(Note: ...)", "(formerly POL101Y1)", "(see NOTE 2)", "(if taking
	// AST425Y1, ...)", "(total of 1.5 credit)", "(3.0 credits in total)",
	// "(The courses ... are recommended.)", "(History & Society courses)",
	// "(These MIE courses address Human Factors ...)", "(note: some of these
	// courses have prerequisites)".
	complAsideParenRe = regexp.MustCompile(`\(\s*_?((?i:note|nb)\s*:[^()]*|(?i:formerly)\s+[^()]*|(?i:see)\s+(?i:note)[^()]*|(?i:if\s+taking)\b[^()]*|(?i:total\s+of)\s+\d[^()]*|\d+(?:\.\d+)?\s+credits?\s+in\s+total|(?i:the|these)\s+[^()]*\b(?:are|is)\s+(?:recommended|suggested|encouraged)\.?|(?i:these)\s+[A-Z]{3}\s+courses\s+[^()]*|[A-Z][\w&,\s]*\bcourses)_?\s*\)`)
	// complAppositiveRe: a descriptor closing a list of alternatives: ", a
	// capstone integrative course".
	complAppositiveRe = regexp.MustCompile(`,\s+an?\s+[a-z][a-z\s-]*\bcourse\.?$`)
)

// complCategoryAndRe: a category label between two lists of one pool: "(History
// & Society courses) and".
var complCategoryAndRe = regexp.MustCompile(`\(\s*([A-Z][\w&,\s]*\bcourses)\s*\)\s*,?\s*and\s+`)

// formerlyAlternatives gives each course of n that s writes with its old
// code ("POL101H1 (formerly POL101Y1)") the old code too, as a list does
// (remarkItem): a course node becomes or(course, old), a pool takes the
// old code. It returns the asides without the "formerly" ones it read.
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

// stripAsides removes the asides from item text s (complAsideParenRe,
// complAppositiveRe) and returns them as notes; nil when it has none.
func stripAsides(s string) (string, []string) {
	var notes []string
	// "3.0 credits from <codes> (History & Society courses) and <codes>
	// (Religion & Thought courses)": the labels name parts of one pool, so
	// the lists join.
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

// complOfAnyFollowingRe: "1.5 credits of any of the following (...):".
var complOfAnyFollowingRe = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?\s+credits?)\s+of\s+any\s+of\s+the\s+following\s*(\([^()]*\))?\s*:`)

// complTopicParenRe: a course and the bracket after it.
var complTopicParenRe = regexp.MustCompile(`(` + codePat + `)\s*\(([^()]+)\)`)

// topicCodes returns the courses s lists with two or more different
// topics: "TRN409H1 (Canadian Defense Policy since the end of the Cold
// War), TRN409H1 (Nuclear Weapons and International Politics)" is a
// special-topics course that counts only on those topics. Each maps to its
// mentions.
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

// topicMembers makes a special-topics course (topicCodes) a "maybe" (design
// 3.2): a course node is and(course, text naming its topics); a pool is
// or(the pool without it, and(the pool, text)), so the course counts only
// as Review. The text is content (design D1): the offering's topic decides.
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

// complIeLeadRe: the "i.e.," before a restatement.
var complIeLeadRe = regexp.MustCompile(`(?i)^i\.e\.,?\s*`)

// restatedLevels narrows the pool of body n to the levels its restatement
// gives: "3.0 SOC credits, which includes at least 1.0 SOC credit at the
// 300-level, i.e., 2.0 SOC credits at the 200 OR 300-level and 1.0 SOC
// credit at the 300-level" counts SOC courses at the 200 and 300 levels
// only. The restatement must split the pool's amount into parts of its
// designators, each with levels.
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

// complPermListRe: an amount from a list, or with permission from a second
// list: "1.0 credit from: A/ B or with permission of the Director: C/ D".
var complPermListRe = regexp.MustCompile(`(?i)^(.*?\bfrom:?\s+)((?:` + codePat + `\s*[/,;]?\s*)*` + codePat + `)\s*,?\s+or\s+((?:with|by)\s+(?:the\s+)?(?:prior\s+)?(?:written\s+)?(?:permission|approval)\s+(?:of|from)\s+[^:]+?)\s*:\s*((?:` + codePat + `\s*[/,;]?\s*)*` + codePat + `)\.?$`)

// permissionList builds a complPermListRe match: or(credits{amount, list},
// and(credits{amount, list and list2}, text permission)). A strict part the
// grammar doesn't read as a pool of courses makes the whole sentence text.
func permissionList(m []string) *types.ReqNode {
	strict, _ := parseCompletionText(m[1] + m[2])
	if !isPool(strict) || strict.Courses == nil || strict.Designators != nil || strict.Levels != nil || strict.MaxAmount != nil {
		return textNode(types.TextOther, m[0])
	}
	wide := cloneNode(strict)
	wide.Courses = dedupe(append(slices.Clone(strict.Courses), codeRe.FindAllString(m[4], -1)...))
	return orNode(strict, andNode(wide, textNode(types.TextPermission, m[3])))
}

// complWithAverageRe: "( A and B and C) with an average grade of 63%".
var complWithAverageRe = regexp.MustCompile(`(?i)^\(?\s*((?:` + codePat + `\s*(?:,|and)?\s*)+)\)?\s+with\s+an\s+average\s+(?:grade\s+)?of\s+(?:at\s+least\s+)?(\d{2})\s*%$`)

// complAnyRemainingRe: "<pool>, or any remaining <filter> not used toward other
// program requirements - <codes> (<approval for more codes>)".
var complAnyRemainingRe = regexp.MustCompile(`(?i)^(.+?),?\s+or\s+any\s+remaining\s+[^-–]+?\s+[-–]\s+((?:\(?\s*` + codePat + `\s*\)?\s*[,/]?\s*)+)\s*(?:\((.*)\))?$`)

// complOrEquivalentRe: "<strict>, or an equivalent <wider>, approved by
// <whom>".
var complOrEquivalentRe = regexp.MustCompile(`(?i)^(.+?),?\s+or\s+an\s+equivalent\s+(.+?),\s+((?:approved\s+by|with\s+(?:the\s+)?approval\s+of)\s+.+)$`)

var (
	// complLowerRemarkRe: a remark in brackets that starts in lowercase with
	// advice or timing: "(should be taken in first year, ...)".
	complLowerRemarkRe = regexp.MustCompile(`\s*\(((?:should|ideally|normally|preferably|recommended|strongly|highly|best|to\s+be\s+taken)\b[^()]*)\)`)
	// complTimingRe: when to take a course.
	complTimingRe = regexp.MustCompile(`(?i)\b(?:should|to)\s+be\s+taken\b|\bin\s+(?:the\s+)?(?:first|second|third|fourth)\s+year\b`)
)

// complAndOrApprovalRe: "A/ B and/or C/ D <qualified, with approval>".
var complAndOrApprovalRe = regexp.MustCompile(`^((?:` + codePat + `\s*/\s*)*` + codePat + `)\s+and/or\s+((?:` + codePat + `\s*/\s*)*` + codePat + `)\s+([^;]*\b(?:approval|permission)\b[^;]*)$`)

// complOrAnotherRe: an approved alternative closing an item's list: ", or
// another course ... approved by the Undergraduate Chair", ", or any research
// course (such as ...), by permission of the Undergraduate Chair".
var complOrAnotherRe = regexp.MustCompile(`(?i),?\s+(?:or|and)\s+((?:another|any)\s+[^;]*?\b(?:approved\s+by|by\s+permission|with\s+(?:the\s+)?(?:prior\s+)?(?:approval|permission)|at\s+the\s+discretion\s+of|as\s+determined\s+by)\b[^;]*?)\.?$`)

// checkedBody returns n, or the sentence as one text node where the
// grammar's reading can't be trusted (a wrong structure is worse than text):
//   - a union pool of courses and filters (namedLists makes it a named list
//     when it can: unionList, listUnion);
//   - text that swallowed a course code, so the codes after it read as
//     required ("Some offerings of POL336H1; POL357H1; ...");
//   - in an "and": an amount of any credits next to courses or text, a pool
//     the grammar didn't recognise ("1.5 credits ... from: BIO, CSB
//     (excluding ...)"); a sentence about courses ("Students should note
//     that completing ( ECO231H1 and ECO232H1) limits ..."); a cap that lost
//     what it caps; a pool cut short by a remark, its last members read as
//     required;
//   - an empty credits node, a phrase the grammar half read;
//   - three fragments of text, or nothing but text.
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
			// "Students should note that completing ( ECO231H1 and
			// ECO232H1) limits ...": courses named in a sentence, not
			// required by it.
			bad = true
		case slices.ContainsFunc(x.Items, func(y *types.ReqNode) bool {
			return y.Type == types.NodeCredits && y.MaxAmount != nil && y.Courses == nil && y.Designators == nil && y.Levels == nil
		}):
			// A cap that lost what it caps.
			bad = true
		case slices.ContainsFunc(x.Items, func(y *types.ReqNode) bool { return y.Type == types.NodeCourse }) &&
			slices.ContainsFunc(x.Items, func(y *types.ReqNode) bool {
				return y.Type == types.NodeOr && slices.ContainsFunc(y.Items, func(z *types.ReqNode) bool {
					return z.Type == types.NodeText && z.Kind == types.TextPermission
				})
			}):
			// A route the grammar cut a list at: "1.0 credit from: A/ B or
			// with permission of the Director: C/ D" read as (A/ B or
			// permission) and C, which one 0.5 course would meet.
			bad = true
		case len(x.Items) >= 3 && isPool(x.Items[0]) && x.Items[0].Courses != nil && x.Items[0].Designators == nil &&
			!slices.ContainsFunc(x.Items[1:], func(y *types.ReqNode) bool { return y.Type != types.NodeCourse }) &&
			len(complAmountRe.FindAllString(s, -1)) == 1:
			// "2.0 credits from <codes> and <codes>" read as the first list's
			// pool and every course of the second required: one pool.
			bad = true
		case slices.ContainsFunc(x.Items, func(y *types.ReqNode) bool { return y.Type == types.NodeCredits && y.Courses != nil }) &&
			slices.ContainsFunc(x.Items, func(y *types.ReqNode) bool { return y.Type == types.NodeText }) &&
			len(slices.DeleteFunc(slices.Clone(x.Items), func(y *types.ReqNode) bool { return y.Type != types.NodeCourse })) >= 2:
			// A pool cut short by a remark: "1.0 credit from: A/ B (...), C,
			// D" read as the pool and then C and D required.
			bad = true
		}
	})
	walkReq(n, func(x *types.ReqNode) {
		// An empty credits node: a phrase the grammar half read ("normally
		// taken in first year").
		bad = bad || x.Type == types.NodeCredits && x.Amount == 0 && x.MaxAmount == nil && x.Items == nil && x.Courses == nil &&
			x.Designators == nil && x.Levels == nil && x.Breadth == nil && x.Lists == nil
	})
	walkReq(n, func(x *types.ReqNode) {
		// "1.0 credit in ONE other discipline": a word in capitals, not a
		// designator.
		bad = bad || x.Type == types.NodeCredits && slices.ContainsFunc(x.Designators, func(d string) bool { return complCapsWords[d] })
	})
	walkReq(n, func(x *types.ReqNode) {
		// "3.5 credits of ESS core courses", "1.0 credit in courses from the
		// Linguistics concentration below": the designator (or subject)
		// names a list, which may hold other designators, not a filter.
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
		// "1.5 credits from PHL271H1 / PHL275H1 / Group 2": listed courses
		// alone that can't make up the amount; the rest is a list or text.
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
		// "Up to 1.0 credit may be taken from list 2: ..." read as a body: a
		// cap is no requirement of its own.
		bad = true
	}
	walkReq(n, func(x *types.ReqNode) {
		// "0.5 credits at the 400-level; fulfilled by any 400-level courses
		// listed in ...": fragments of a sentence the grammar cut up.
		bad = bad || x.Type == types.NodeText && x.Kind == types.TextOther && complFragmentRe.MatchString(x.Text)
	})
	if complWordSlashRe.MatchString(s) {
		// "1.0 credit from the History disciplinary/thematic cluster": "/"
		// between words joins them; it offers no alternatives.
		walkReq(n, func(x *types.ReqNode) {
			bad = bad || x.Type == types.NodeOr && slices.ContainsFunc(x.Items, func(y *types.ReqNode) bool { return y.Type == types.NodeText })
		})
	}
	if m := complLeadAmountRe.FindStringSubmatch(s); m != nil && n.Type == types.NodeAnd &&
		!slices.ContainsFunc(n.Items, func(y *types.ReqNode) bool { return y.Type != types.NodeCourse }) {
		// "1.0 credit: A, B, C" read as three required courses worth 1.5.
		amount, _ := strconv.ParseFloat(m[1], 64)
		sum := 0.0
		for _, it := range n.Items {
			sum += courseWeight(it.Code)
		}
		bad = bad || sum > amount+1e-9
	}
	if m := complDesigAtLevelOrRe.FindStringSubmatch(s); m != nil &&
		!sameSet(parseLevels(m[2]), parseLevels(m[4])) && !sameSet(desigRe.FindAllString(m[1], -1), desigRe.FindAllString(m[3], -1)) {
		// "FRE at the 200+ level or FSL at the 300+ level": two filters.
		bad = true
	}
	if m := complFilterOrFilterRe.FindStringSubmatch(s); m != nil &&
		!sameSet(parseLevels(m[1]), parseLevels(m[3])) && !sameSet(desigRe.FindAllString(m[2], -1), desigRe.FindAllString(m[4], -1)) {
		// "2.0 credits of 100- or 200-level ESS courses or 200-level JEG
		// courses": a union of two filters, which one filter can't hold
		// (namedLists makes it a list: filterUnion). Merged, it would count
		// JEG100H1.
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
		// Three fragments of text: the grammar read a sentence, not a list.
		return textNode(types.TextOther, s)
	case !read && (n.Type == types.NodeAnd || n.Type == types.NodeOr):
		return textNode(classify(cleanNote(s)), s)
	}
	return n
}

// complWordSlashRe: two lowercase words joined by "/": "disciplinary/thematic".
var complWordSlashRe = regexp.MustCompile(`\b[a-z]{3,}/[a-z]{3,}\b`)

// complFragmentRe: text the grammar cut from a sentence, not a condition of its
// own: "fulfilled by", "listed in the Centre for Jewish Studies ...", "both
// of", "D-level courses", "plus".
var complFragmentRe = regexp.MustCompile(`(?i)^(?:fulfilled\s+by|listed\s+(?:in|below|above)\b|both\s+of\b|one\s+or\s+both\b|plus\b|[A-D]-level\b|on\s+the\b|disciplinary/thematic\b)`)

// complLeadAmountRe: the amount a requirement starts with.
var complLeadAmountRe = regexp.MustCompile(`(?i)^(?:[^:]{0,60}:\s*)?(\d+(?:\.\d+)?)\s+credits?\b`)

// complCapsWords: words written in capitals that aren't designators.
var complCapsWords = map[string]bool{"ONE": true, "TWO": true, "ALL": true, "ANY": true, "AND": true, "THE": true, "NOT": true,
	"FOR": true, "BUT": true, "MAY": true, "OUT": true, "SIX": true, "TEN": true, "PER": true, "TBA": true}

// complListNounRe: text that names a list beside a designator: "core courses",
// "concentration below", "other discipline offered within ...".
var complListNounRe = regexp.MustCompile(`(?i)^(?:other\s+)?(?:[a-z&-]+\s+){0,3}(?:courses?|concentrations?|disciplines?|streams?|areas?|groups?|lists?)\b`)

// complFilterOrFilterRe: "<levels> <designators> courses or <levels>
// <designators> courses".
var complFilterOrFilterRe = regexp.MustCompile(`(?i)\b([1-4]00(?:\s*[-/,]?\s*(?:or|and)?\s*[1-4]00)*[-+]?)\s*-?\s*levels?\s+((?-i:[A-Z]{3})(?:\s*(?:/|,|or|and)\s*(?-i:[A-Z]{3}))*)\s+courses?\s+or\s+([1-4]00(?:\s*[-/,]?\s*(?:or|and)?\s*[1-4]00)*[-+]?)\s*-?\s*levels?\s+((?-i:[A-Z]{3})(?:\s*(?:/|,|or|and)\s*(?-i:[A-Z]{3}))*)\s+courses?\b`)

// complDesigAtLevelOrRe: "<designators> at the <levels> or <designators> at the
// <levels>".
var complDesigAtLevelOrRe = regexp.MustCompile(`\b((?:[A-Z]{3})(?:\s*/\s*[A-Z]{3})*)\s+(?:(?i:courses?|credits?)\s+)?(?i:at\s+the)\s+([1-4]00[-+/\s\d]*(?i:levels?)?)\s+(?i:or)\s+((?:[A-Z]{3})(?:\s*/\s*[A-Z]{3})*)\s+(?:(?i:courses?|credits?)\s+)?(?i:at\s+the)\s+([1-4]00[-+/\s\d]*(?i:levels?)?)`)

// hasCourses reports whether n names a course or credits.
func hasCourses(n *types.ReqNode) bool {
	found := false
	walkReq(n, func(x *types.ReqNode) { found = found || x.Type == types.NodeCourse || x.Type == types.NodeCredits })
	return found
}

// complProseRe: a verb, which makes text around courses a sentence about them.
var complProseRe = regexp.MustCompile(`(?i)\b(?:should|must|may|can|will|note|limits?|is|are|was|were|have|has)\b`)

// isAnyCredits reports a credits node with an amount and no filter: any
// credits at all.
func isAnyCredits(n *types.ReqNode) bool {
	return n.Type == types.NodeCredits && n.Amount > 0 && n.Courses == nil && n.Designators == nil && n.Levels == nil &&
		n.Breadth == nil && n.Lists == nil && n.Requirements == nil && !n.Transfer && !n.CrNcr && n.ExcludeDesignators == nil && n.MaxCourseAmount == 0
}

// itemLabel is the calendar's label for an item: its number ("4", "Higher
// Years 2"), or a labelled line's name ("Methods Courses"); not a year,
// which is the section, nor a note's label.
func itemLabel(b *compBlock) string {
	switch {
	case b.kind == blockItem:
		return b.label
	case b.kind == blockLabelled && !blkStartsSection(b) && !blkNoteLabelRe.MatchString(b.label):
		return b.label
	case b.kind == blockLabelled && sectionLabel(b.label):
		// "Fourth Year Courses: Any 2 courses (1.0 credit) from ...": the
		// year is its section, and the label names it (completion_limits.go).
		return b.label
	}
	return ""
}

// allAdvice reports whether every sentence of s is advice.
func (a *compAssembler) allAdvice(s string) bool {
	for _, sent := range splitComplSentences(s) {
		if a.role(sent) != roleAdvisory {
			return false
		}
	}
	return true
}

// notesTo adds notes to row, or to the program's.
func (a *compAssembler) notesTo(row *compRow, notes []string) {
	if row != nil {
		row.r.Notes = append(row.r.Notes, notes...)
	} else {
		a.notes = append(a.notes, notes...)
	}
}

// rowUnder adds a rule row under target: an item, or the program (Req 1, or
// the top when there is no total).
func (a *compAssembler) rowUnder(target *compRow, r *types.Requirement) {
	if target != nil {
		target.children = append(target.children, &compRow{r: r})
	} else {
		a.program = append(a.program, &compRow{r: r})
	}
}

var (
	// complLooseRe: a sentence that permits or defines rather than requires:
	// "LIN courses from other campuses are considered courses from other
	// Departments", "Students may use CR/NCR on ...", "ANT390H1 may be placed
	// in either Group A or B", "Sub-total = 8.5 credits".
	complLooseRe = regexp.MustCompile(`(?i)\b(?:may|can|could|might)\b|\b(?:is|are)\s+(?:also\s+)?considered\b|\bare\s+those\b|\bsub-?total\b|\bis\s+waived\b|\bis\s+not\s+a\s+substitute\b|\bconsidered\s+equivalent\b|^total\s*=|\bdoes\s+not\s+(?:have|need)\s+to\s+be\s+replaced\b|\bno\s+need\s+to\s+replace\b`)
	// complBindingWordRe: words that make a sentence a rule whatever else it
	// says.
	complBindingWordRe = regexp.MustCompile(`(?i)\bmust\b|\brequired?\b|\bat\s+least\b|\bno\s+more\s+than\b|\bminimum\b|\bmaximum\b|\bcannot\b|\bonly\b|\bat\s+most\b|\bnot\s+(?:be\s+)?(?:counted|accepted|used|permitted|allowed)\b|\bmay\s+not\b`)
)

// looseMain reports whether sentence s only permits or defines (complLooseRe
// and no complBindingWordRe), read in its main clause: brackets and what
// follows a ";" are remarks on it ("BIO120H1/ BIO130H1 (may be taken in Year
// 1), BIO220H1 and 1.5 credits from ...", "1.0 credit from ESS224H1, ...; up
// to 0.5 credit of this requirement can be satisfied by ..."). A line that
// leads with an amount or a code the grammar reads as courses is a
// requirement, whatever else it permits.
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
// rule. One that only permits or defines (complLooseRe, no binding word)
// isn't a constraint (which would force Review on everyone): when it relaxes
// a requirement it names ("Students exempted from FSL220H1/ FSL222H1 may
// replace it with any FRE 200+ ...", "If the FOR301H1 is not on offer, a
// student may use a substitute ...") it is an alternative on that row; else a
// note. Other sentences are a row of text.
func (a *compAssembler) looseSentence(target *compRow, s string) {
	t := a.clean(s)
	if !complLooseRe.MatchString(t) || complBindingWordRe.MatchString(t) {
		if complFloorLikeRe.MatchString(t) {
			// "At least one Applications course (0.5 credit) must be
			// 300-level or higher": a floor the classifier read as an amount.
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
			// "CSC110Y1 and CSC111H1 must be completed in order to complete the
			// Specialist program" restates the row requiring them: its raw.
			addRaw(r, s)
		} else if rs := a.referencedRows(s); complWeightSplitRe.MatchString(t) && rs != nil {
			// "If students complete ENV465Y1, 0.5 credit from the course
			// contributes to group B in requirement 2": the half the claims
			// can't split goes to the row it names, as its alternative: Review,
			// not NotMet, for a student who counts it there.
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
			// A program route: placed by what it names, else a note
			// (placeProgramRoutes).
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

// complRequirementLeadRe: a line that announces the requirement stated after
// it: "An Integrative, Inquiry-Based Activity Requirement must be satisfied."
var complRequirementLeadRe = regexp.MustCompile(`(?i)^an?\s+[^.]+?\s+requirement\s+must\s+be\s+(?:satisfied|met|completed|fulfilled)$|\bthe\s+following\s+requirements\s*:?$`)

// complInheritRe: a focus that begins with another program's requirements,
// which Link copies (design 3.4): a row of its own.
var complInheritRe = regexp.MustCompile(`(?i)\bbegins\s+with\s+the\s+requirements\s+of\b`)

// complYHalvesRe: "Some of the above course options are 1.0 credit courses, but
// only 0.5 credit can be counted toward the completion of this major".
var complYHalvesRe = regexp.MustCompile(`(?i)^some\s+of\s+the\s+above\s+course\s+options\s+are\s+1\.0\s+credit\s+courses,?\s+but\s+only\s+0\.5\s+credit\s+can\s+be\s+counted\b`)

// yHalves reads a complYHalvesRe sentence into a limit of 0.5 on each full-year
// course of the item above it ("Only 0.5 of ECO220Y1 will be accepted",
// design 3.1), or joins its raw when it lists none, and reports whether it
// did.
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
			// The item above lists no full-year course (ASMAJ1533, whose
			// list is all half courses): the sentence caps nothing there,
			// and the item's amount says it; its raw holds it.
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

// complRestateRe: a sentence that says courses are required.
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

// complFloorLikeRe: a sentence worded as a floor: "At least ... must be ...".
var complFloorLikeRe = regexp.MustCompile(`(?i)^at\s+least\b.*\bmust\s+be\b`)

// complNotListedRe: a course outside a list, which refuses nothing: "not listed
// in the above clusters", "not captured in the above list".
var complNotListedRe = regexp.MustCompile(`(?i)\bnot\s+(?:already\s+)?(?:listed|captured|included)\s+in\b`)

// complConsiderForRe: courses a person may accept for a requirement: "Other
// 400-level courses ... can also be considered for fulfillment of the
// capstone requirement, if students receive written permission ...",
// "Daniels students ... may be able to fulfill up to 1.5 credits in the
// Third/Fourth Year Requirement with three of the following ARC courses".
var complConsiderForRe = regexp.MustCompile(`(?i)\b(?:can|may)\s+(?:also\s+)?be\s+considered\s+for\b|\bmay\s+be\s+able\s+to\s+fulfil+\b`)

// noteTarget is the row a note on target goes to: target, or the program's
// notes for the total and the program.
func (a *compAssembler) noteTarget(target *compRow) *compRow {
	if target == a.total {
		return nil
	}
	return target
}

var (
	// complPrereqRe: what a course needs before it, not what the program
	// counts: "PSL300H1 and PSL301H1 require MAT 100-level/ PHY 100-level
	// courses", "Students taking PCL477H1 must take BCH210H1 prior",
	// "BIO220H1 is required for all EEB 300+ level courses", "prerequisites
	// for these courses cannot be used towards Group A requirements".
	complPrereqRe = regexp.MustCompile(`(?i)^(?:` + codePat + `)(?:\s*(?:,|and|or)\s*` + codePat + `)*\s+requires?\b|\bmust\s+take\s+` + codePat + `\s+(?:prior|first|before)\b|\b(?:is|are)\s+required\s+for\s+all\b|\bprerequisites?\s+for\s+these\s+courses\b`)
	// complExclusiveAgainRe: the exclusive claims restated: "The course taken
	// in lieu of STA130H1 cannot be one that is already being used to satisfy
	// Requirement 2 ...".
	complExclusiveAgainRe = regexp.MustCompile(`(?i)\bcannot\s+be\s+(?:one|a\s+course)\s+that\s+is\s+already\s+(?:being\s+)?used\b`)
	// complWeightSplitRe: how much of a course counts where, which the claims
	// split: "If students complete CSC110Y1, 0.5 credit from the course
	// contributes to the First Year requirements", "If students complete
	// ENV465Y1, 0.5 credit from the course contributes to group B".
	complWeightSplitRe = regexp.MustCompile(`(?i)\bcredit\s+from\s+the\s+course\s+contributes\s+to\b`)
)

// complNoOtherRe: a refusal of anything else: "No other cognates are permitted
// for a Spanish Minor".
var complNoOtherRe = regexp.MustCompile(`(?i)\bno\s+other\b[^.;]*\b(?:permitted|allowed|accepted|counted)\b`)

// carriesRule reports whether a requirement sentence the grammar can't read
// states a rule the checker must see, so that it is a row of text (Review)
// rather than a note: a binding word (complBindingWordRe: "must", "only",
// "cannot", "no more than", "at least", ...) or an amount of courses or of a
// filter. Definitions ("For the purpose of program completion, JWE206H1
// counts as an English course"), information ("MST300H1 is offered through
// the Centre for Medieval Studies", "STA130H1 is restricted to first-year
// students"), pointers ("see Note A below"), prerequisites (complPrereqRe), the
// exclusive claims restated (complExclusiveAgainRe) and weight splits
// (complWeightSplitRe) don't: as rows of text they would force Review on
// everyone.
func carriesRule(t string) bool {
	switch {
	case complPrereqRe.MatchString(t), complExclusiveAgainRe.MatchString(t), complWeightSplitRe.MatchString(t):
		return false
	case complBindingWordRe.MatchString(t), complNoOtherRe.MatchString(t), isComplAllocLead(t):
		return true
	}
	return complAmountRe.MatchString(t) && (codeRe.MatchString(t) || complFilterRe.MatchString(t))
}

// complApprovalWordRe: a sentence that depends on someone's approval.
var complApprovalWordRe = regexp.MustCompile(`(?i)\bapprov(?:al|ed)\b|\bpermission\b|\bdiscretion\b`)

// complSubstitutedFromRe: "A maximum of 0.5 credit may be substituted from the
// following first-year courses: POL191H1, ...", "up to 0.5 credit of this
// requirement can be satisfied by taking one of the following ethics
// courses: IMC200H1/ ...", "Up to 1.0 credit of this requirement can also be
// completed from HST211H1/ PHS211H1, INS200H1, ...".
var complSubstitutedFromRe = regexp.MustCompile(`(?i)^(?:a\s+maximum\s+of|up\s+to|no\s+more\s+than)\s+(?:a\s+)?(\d+(?:\.\d+)?)\s+credits?\s+(?:of\s+this\s+requirement\s+)?(?:may|can)\s+(?:also\s+)?be\s+(?:substituted|taken|chosen|satisfied|completed|fulfilled)\s+(?:from|by\s+taking)\s+(?:(?:one|any)\s+of\s+)?(?:the\s+following[^:]*:\s*)?((?:` + codePat + `\s*[,/;]?\s*(?:and|or)?\s*)+)$`)

// substitutedFrom widens the item's pool with the courses a
// complSubstitutedFromRe sentence names, capped at its amount (a limit under
// the item), and reports whether it did.
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

// widenPool returns pool n widened with the courses of a complSubstitutedFromRe
// clause s, and the limit that caps them at its amount; a nil limit when s
// isn't one or n isn't a pool of listed courses the courses fit (by level).
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

// complConditionalRe: "If CLA160H1 is taken, students must take an additional
// 0.5 credit from the following: ( CLA210H1/ ...)/ GGR101H1".
var complConditionalRe = regexp.MustCompile(`(?i)^if\s+((?:` + codePat + `\s*(?:/|or|,)?\s*)+?)\s+(?:is|are)\s+taken,?\s+students\s+must\s+(?:also\s+)?take\s+an\s+additional\s+(.+)$`)

// conditionalExtra reads a complConditionalRe sentence into the item it
// follows: the course becomes and(course, the extra credits), and reports
// whether it did. When the item doesn't offer the course as one alternative,
// or the extra doesn't read, the whole item is text.
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

// complExemptedRe: "Students exempted from FSL220H1/ FSL222H1 may replace it".
var complExemptedRe = regexp.MustCompile(`(?i)\bexempted\s+from\s+((?:` + codePat + `[\s,/]*(?:and|or)?\s*)+)`)

// relaxedRows returns the rows a loose sentence relaxes and the codes it
// stands in for: those holding the codes it replaces ("in lieu of ...",
// "exempted from ..."), the requirement it names, or the only item naming
// one of its codes.
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
		// "... would be able to use any completed RSM course equivalents
		// towards the completion of the Certificate (e.g. RSM100H1 would be
		// deemed equivalent to MGT100H1 ..., etc)": every row that requires
		// a course of the example's designator (MGT).
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

// complEquivalentsRe: courses of another unit accepted as equivalents, with an
// example: "any completed RSM course equivalents ... (e.g. RSM100H1 would
// be deemed equivalent to MGT100H1 ..., etc)".
var complEquivalentsRe = regexp.MustCompile(`(?i)\bany\s+(?:completed\s+)?(?-i:[A-Z]{3})\s+course\s+equivalents\b.*\bdeemed\s+equivalent\s+to\s+(` + codePat + `)`)

// overlap reads "Some courses may contribute to both the Canadian Politics
// and Diversity requirements (in requirements 4 and 6)" into overlaps on
// both rows (design 3.3), and reports whether it named two items under one
// parent.
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

// textReq is the row textRow adds.
func (a *compAssembler) textReq(s string) *types.Requirement {
	a.diag.textRows = append(a.diag.textRows, s)
	return &types.Requirement{Kind: types.RequirementConstraint, Req: textNode(types.TextOther, s), Raw: s}
}

// constraint reads a floor into constraint rows under target.
func (a *compAssembler) constraint(target *compRow, s string) {
	for _, r := range a.floorRows(s) {
		a.rowUnder(target, r)
	}
}

// floorRows reads a floor ("At least 1.0 credit must be at the 300+ level",
// "at least 2.0 credits in each of GRK and LAT") into constraint rows
// (readFloor), or a row of text (textReq).
func (a *compAssembler) floorRows(s string) []*types.Requirement {
	if rows, ok := readFloor(s); ok {
		return rows
	}
	return []*types.Requirement{a.textReq(s)}
}

// complMinUpToRe: a floor and a cap on the courses of some designators:
// "For CSC courses, a minimum of 1.0 credit and up to 2.0 credits must be
// chosen".
var complMinUpToRe = regexp.MustCompile(`(?i)^for\s+((?-i:[A-Z]{3})(?:\s*(?:,|/|and|or)\s*(?-i:[A-Z]{3}))*)\s+courses,\s+(?:a\s+minimum\s+of|at\s+least)\s+(\d+(?:\.\d+)?)\s+credits?\s+and\s+(?:up\s+to|a\s+maximum\s+of|no\s+more\s+than)\s+(\d+(?:\.\d+)?)\s+credits?\s+must\s+be\s+(?:chosen|taken|completed)$`)

// readFloor reads a floor into constraint rows; ok is false when the
// grammar can't read it.
func readFloor(s string) (rows []*types.Requirement, ok bool) {
	t := complOutOfRe.ReplaceAllString(strings.TrimSpace(strings.TrimSuffix(s, ".")), "$1 credit")
	// "At least one Applications course (0.5 credit) must be 300-level or
	// higher": the floor is the course's credit.
	t = complOneCourseFloorRe.ReplaceAllString(t, "At least $1 credit must be at the")
	if m := complMinUpToRe.FindStringSubmatch(t); m != nil {
		// "For CSC courses, a minimum of 1.0 credit and up to 2.0 credits
		// must be chosen": a floor and a cap on the same courses.
		lo, _ := strconv.ParseFloat(m[2], 64)
		hi, _ := strconv.ParseFloat(m[3], 64)
		desigs := desigRe.FindAllString(m[1], -1)
		return []*types.Requirement{
			{Kind: types.RequirementConstraint, Raw: s, Req: &types.ReqNode{Type: types.NodeCredits, Amount: lo, Designators: desigs}},
			{Kind: types.RequirementLimit, Raw: s, Req: &types.ReqNode{Type: types.NodeCredits, Designators: slices.Clone(desigs), MaxAmount: maxAmount(hi)}},
		}, true
	}
	if m := complEachOfRe.FindStringSubmatch(t); m != nil {
		// "at least 2.0 credits in each of GRK and LAT": a floor for each.
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

// complOneCourseFloorRe: "At least one Applications course (0.5 credit) must
// be".
var complOneCourseFloorRe = regexp.MustCompile(`(?i)^at\s+least\s+one\s+(?:[\w-]+\s+){0,3}?course\s+\((\d+(?:\.\d+)?)\s+credits?\)\s+must\s+be(?:\s+at\s+the)?`)

// complOutOfRe: "1.0 out of the 3.5 credits must be ...": the scope is the
// item.
var complOutOfRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+out\s+of\s+the\s+\d+(?:\.\d+)?\s+credits\b`)

// complEachOfRe: "at least 2.0 credits in each of GRK and LAT".
var complEachOfRe = regexp.MustCompile(`(?i)^(?:at\s+least\s+)?(\d+(?:\.\d+)?)\s+credits?\s+in\s+each\s+of\s+((?-i:[A-Z]{3})(?:\s*(?:,|and|or|/)\s*(?-i:[A-Z]{3}))+)$`)

// limit reads a cap ("No more than 1.0 credit of 100-level courses may be
// used", "No more than 0.5 credit in BCH can be used towards this
// requirement") into a limit row under target. Courses that don't count
// ("STA220H1 and STA255H1 are not accepted for the major") are a cap of 0.
func (a *compAssembler) limit(target *compRow, s string) {
	if complCampusRe.MatchString(a.clean(s)) && a.campusRule(s) || a.yHalves(s) {
		return
	}
	if r := capRow(s); r != nil {
		a.rowUnder(target, r)
		return
	}
	if target != nil && target != a.total && a.poolAddition(target, s) {
		// "Up to 1.0 credit can be used from GGR493Y1, based on suitability
		// of placement for this program, and approved by department": more
		// courses for the item, not a cap on it.
		return
	}
	if t := a.clean(s); (target == nil || target == a.total) && complCognateRe.MatchString(t) && complApprovalWordRe.MatchString(t) {
		// "Up to 1.0 credit in identified or pre-approved cognate courses may
		// count (...); please inquire at the Department and get approved in
		// advance": a route into the program, placed by what it names
		// (placeProgramRoutes), not a cap.
		a.programRoutes = append(a.programRoutes, s)
		return
	}
	if complPrereqRe.MatchString(a.clean(s)) {
		// "PHY392H1 and PHY492H1 include relevant environmental content, but
		// prerequisites for these courses cannot be used towards Group A
		// requirements": what a list's courses need, a note (carriesRule).
		a.notesTo(a.noteTarget(target), []string{cleanNote(s)})
		return
	}
	a.textRow(target, s)
}

// complCognateRe: courses of other departments a program accepts.
var complCognateRe = regexp.MustCompile(`(?i)\bcognates?\b`)

var (
	// "Students enrolled in the Computer Science Minor program are limited to
	// at most 1.5 credits from ...": a cap.
	complLimitedToRe = regexp.MustCompile(`(?i)^students\s+(?:enrolled\s+in\s+.+?\s+)?are\s+limited\s+to\s+(?:at\s+most|a\s+maximum\s+of)\s+`)
	// A cap leading a sentence: "Up to 1.0 credit ...", "No more than ...".
	complCapLeadRe = regexp.MustCompile(`(?i)^(?:up\s+to|no\s+more\s+than|a\s+maximum\s+of|at\s+most)\b`)
	// The campus rule restated: "Courses taught at UTM or UTSC are not
	// included for consideration".
	complCampusRe = regexp.MustCompile(`(?i)^courses\s+taught\s+at\s+UTM\s+or\s+UTSC\s+are\s+not\s+included\s+for\s+consideration\.?$`)
	// Courses that never count, named before the verb: "STA220H1 and
	// STA255H1 are not accepted for the major", "PSL201Y1 is not
	// acceptable", "Courses with lectures taught in English – PRT250H1 or
	// PRT375H1, for instance – are not included for consideration in the
	// Certificate", "MAT133Y1 is not a suitable substitute for ...".
	complNotCountedRe = regexp.MustCompile(`(?i)\b(?:are|is)\s+not\s+(?:accepted|acceptable|included\s+for\s+consideration|an?\s+suitable\s+substitutes?\s+for)\b`)
	// "... is not a suitable substitute for": the course doesn't count
	// toward what follows.
	complNotSubstituteRe = regexp.MustCompile(`(?i)\b(?:are|is)\s+not\s+an?\s+suitable\s+substitutes?\s+for\b`)
	// ... or after it: "Students are not permitted to take PCL472Y1 or
	// PCL474Y1", "We do not accept ENG100H1, ENG102H1".
	complNotTakenRe = regexp.MustCompile(`(?i)\b(?:are|is)\s+not\s+permitted\s+to\s+take\b|\bdo\s+not\s+accept\b`)
)

// complOmitRe: "may omit CSC108H1".
var complOmitRe = regexp.MustCompile(`(?i)\bmay\s+omit\s+(` + codePat + `)\b`)

// omitCovered reports whether the course a sentence lets students omit is
// one the item naming it already offers routes without (an "or" with an
// alternative that doesn't need it). The sentence then joins that item's
// raw: the item encodes it.
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

// campusRule places a sentence restating the campus rule (design 5.9: a
// designator or level filter matches St. George courses only) on Req 1,
// whose pool is such a filter: it is encoded there, so it joins Req 1's raw.
// It reports false when Req 1 has no such pool.
func (a *compAssembler) campusRule(s string) bool {
	if a.total == nil || a.total.r.Req.Type != types.NodeCredits || a.total.r.Req.Designators == nil || a.total.r.Req.Courses != nil {
		return false
	}
	a.total.r.Raw += "\n" + s
	return true
}

// complOnlyOfRe: "only 0.5 of ECO220Y1 will be accepted".
var complOnlyOfRe = regexp.MustCompile(`(?i)^(?:please\s+note\s+(?:that\s+)?)?only\s+(?:a\s+)?(\d+(?:\.\d+)?)\s+(?:credits?\s+)?of\s+(` + codePat + `)\s+(?:will\s+be|is|may\s+be|can\s+be)\s+(?:accepted|counted|used)\b`)

// complNoMoreThanOneRe: "No more than one from GGR273H1, GGR373H1", "No more
// than one of GGR273H1, GGR373H1".
var complNoMoreThanOneRe = regexp.MustCompile(`(?i)^no\s+more\s+than\s+one\s+(?:of|from)\s+((?:` + codePat + `)(?:\s*(?:,|or|and|/)\s*` + codePat + `)+)$`)

// complCapExceptRe: the exception a cap states: "... except for students ...".
var complCapExceptRe = regexp.MustCompile(`(?i)\b(except\s+(?:for|that|when|if)\b.*)$`)

// hasFilter reports whether a limit's node says what it caps: a cap on
// "credits" alone ("Up to 1.0 credit from Cognate Courses:") caps nothing.
func hasFilter(n *types.ReqNode) bool {
	return n != nil && (n.Courses != nil || n.Designators != nil || n.Levels != nil || n.Breadth != nil || n.Lists != nil ||
		n.Transfer || n.CrNcr || n.Items != nil || n.ExcludeDesignators != nil || n.MaxCourseAmount != 0)
}

var (
	// complCapWordingRe: the ways a cap sentence leads into its amount that
	// mean "No more than": "A maximum of a 0.5 credit", "At most a 0.5
	// credit", "Students may use a maximum of 1.0 credit", "Only 1.0 credit
	// from".
	complCapWordingRe = regexp.MustCompile(`(?i)^-?\s*(?:students\s+(?:may|can)\s+use\s+)?(?:(?:a\s+)?maximum\s+of\s+(?:an?\s+)?|at\s+most\s+(?:an?\s+)?|only\s+(?:an?\s+)?)(\d+(?:\.\d+)?\s+credits?\s+(?:from|in|at)\b)`)
	// complCapAnyScopeRe: where a cap sentence says what it counts toward:
	// "... may be counted", "... can be used for program requirements", "...
	// towards the Criminology & Sociolegal Studies program", "... can be used
	// to satisfy Physics Major program requirements".
	complCapAnyScopeRe = regexp.MustCompile(`(?i)\s+(?:(?:can|may|will)\s+(?:be\s+)?(?:used|counted|count|applied|accepted)\b|towards?\s+(?:the\s+)?[\w&\s-]*?\b(?:program|requirements?)\b).*$`)
	// complCapNumberedScopeRe: a cap on numbered requirements, "... can be
	// used towards requirements 5. and 6" (ASSPE1003): it holds for those
	// rows together, which no one limit row can say (one under the total
	// would cap the whole program), so the sentence stays text.
	complCapNumberedScopeRe = regexp.MustCompile(`(?i)\btowards?\s+(?:the\s+)?requirements?\s+#?\d`)
	// complCapNoteParenRe: a pointer in brackets: "(See Note 3)".
	complCapNoteParenRe = regexp.MustCompile(`(?i)\s*\((?:see|note)\b[^()]*\)`)
	// complCodeDashTitleRe: a course's title after it: "CRI389Y0 - Topics in
	// Criminology and Sociolegal Studies Abroad courses".
	complCodeDashTitleRe = regexp.MustCompile(`(` + codePat + `)\s+-\s+[A-Z][^,;]*?\s+courses\b`)
)

// capSentence rewrites a cap written as a sentence into the form the header
// grammar reads: "A maximum of a 0.5 credit from APM346H1/ MAT334H1/
// MAT354H1 may be used to fulfil this requirement (See Note 3)" is "No more
// than 0.5 credit from APM346H1/ MAT334H1/ MAT354H1"; "" when it isn't one.
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

// capRow reads a cap into a limit row, or returns nil.
func capRow(s string) *types.Requirement {
	t := complLimitedToRe.ReplaceAllString(strings.TrimSpace(strings.TrimSuffix(s, ".")), "No more than ")
	for _, u := range []string{t, complCapScopeRe.ReplaceAllString(t, ""), capSentence(t)} {
		rows, tail, ok := parseHeaderFloors(u)
		if ok && len(tail) == 0 && len(rows) == 1 && rows[0].Kind == types.RequirementLimit && hasFilter(rows[0].Req) {
			rows[0].Raw = s
			if m := complCapExceptRe.FindStringSubmatch(s); m != nil {
				// "Only 0.5 credit of APM306Y1/ MAT329Y1 may count ... except
				// for student enrolled in the Education and Society Minor may
				// apply 1.0 credit": a limit can't hold an alternative, so the
				// exception is the row's note.
				rows[0].Notes = []string{cleanNote(m[1])}
			}
			return rows[0]
		}
	}
	if m := complNoMoreThanOneRe.FindStringSubmatch(t); m != nil {
		// "No more than one from GGR273H1, GGR373H1": at most one course.
		return &types.Requirement{Kind: types.RequirementLimit, Raw: s, Req: courseCap(codeRe.FindAllString(m[1], -1))}
	}
	if m := complOnlyOfRe.FindStringSubmatch(t); m != nil {
		// "Please note only 0.5 of ECO220Y1 will be accepted towards the
		// program".
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
	// "No more than 1.0 transfer credit(s) will be eligible for consideration
	// in the Certificate", "No more than a 0.5 credit in transfer credit may
	// be counted towards the program".
	complTransferCapRe = regexp.MustCompile(`(?i)^(?:no\s+more\s+than|a\s+maximum\s+of|up\s+to)\s+(?:a\s+)?(\d+(?:\.\d+)?)\s+(?:credits?\s+in\s+)?transfer\s+credits?\s+(?:will\s+be\s+eligible\s+for\s+consideration|may\s+be\s+(?:counted|applied|used)\s+(?:to(?:wards?)?|for))\s+(?:in\s+)?the\s+(?:certificate|program|minor|major|specialist|focus)$`)
	// "Transfer credits cannot comprise more than 0.5 credit at the
	// 300-/400-level."
	complTransferComprisesRe = regexp.MustCompile(`(?i)^transfer\s+credits\s+cannot\s+comprise\s+more\s+than\s+(?:a\s+)?(\d+(?:\.\d+)?)\s+credits?(?:\s+at\s+the\s+([1-4]00[-/\s\d+]*-?\s*levels?))?$`)
)

// transfer reads a cap on transfer credit (design 3.5) into a limit row
// under target.
func (a *compAssembler) transfer(target *compRow, s string) {
	t := strings.TrimSpace(strings.TrimSuffix(s, "."))
	if r, text, ok := transferRows(t); ok {
		// With the exchange exception (completion_limits.go).
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

// complCrNcrBanRe: courses taken CR/NCR don't count (design 3.5): "Courses that
// have been taken as Credit/No Credit cannot be used to fulfill Certificate
// requirements", "We do not accept any CR/NCR courses toward our program",
// "Courses with a CR/NCR status may not be used to satisfy program
// requirements".
var complCrNcrBanRe = regexp.MustCompile(`(?i)^(?:courses\s+(?:that\s+have\s+been\s+taken\s+as|with\s+a)\s+(?:credit\s*/\s*no\s+credit|CR\s*/\s*NCR)(?:\s+status)?\s+(?:cannot|may\s+not)\s+be\s+used\s+to\s+(?:fulfill|satisfy)\s+(?:the\s+)?(?:certificate|program)\s+requirements|we\s+do\s+not\s+accept\s+any\s+CR\s*/\s*NCR\s+courses\s+toward\s+our\s+programs?)$`)

// complCrNcrMayRe: CR/NCR allowed: "Students may use CR/NCR on ...", "Students
// may use the CR/NCR option towards ...".
var complCrNcrMayRe = regexp.MustCompile(`(?i)\b(?:may|can)\s+(?:use|take)\b`)

// crNcr reads a rule on CR/NCR courses into a limit row under target.
func (a *compAssembler) crNcr(target *compRow, s string) {
	if a.crNcrOutside(target, s) {
		// CR/NCR or other coursework, one share (completion_limits.go).
		return
	}
	if rows, text := crNcrRows(a.clean(s)); rows != nil {
		// Codes beside the CR/NCR courses, other wordings, a share
		// (completion_limits.go).
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
			// "Students may use CR/NCR on the course they use toward the
			// ethics and social responsibility credit": a permission, a note.
			a.notesTo(a.noteTarget(target), []string{cleanNote(s)})
			return
		}
		a.textRow(target, s)
		return
	}
	a.rowUnder(target, &types.Requirement{Kind: types.RequirementLimit, Raw: s,
		Req: &types.ReqNode{Type: types.NodeCredits, CrNcr: true, MaxAmount: maxAmount(0)}})
}

// isApproval reports an approval route: "Other introductory language classes
// may be approved by the Director", "Students can request substitutions from
// the IRP Director".
func (a *compAssembler) isApproval(s string) bool {
	return a.cue(s).approval
}

// complAlternativeRe: a sentence that offers another way to meet a requirement:
// "A 200 or 300-level Geography & Planning course ... may also be used to
// meet this requirement", "0.5 credit from GGR493Y1 can be used to meet this
// requirement", "Transfer credit equivalents of the following courses ...
// will qualify for this requirement", "CSB450H1 is also acceptable".
var complAlternativeRe = regexp.MustCompile(`(?i)\balso\s+(?:be\s+)?(?:used|count(?:s|ed)?|accepted|acceptable|eligible|qualif\w*|fulfil\w*|satisf\w*|meet|complete)\b|\b(?:is|are)\s+also\s+acceptable\b|\bsubstitute\b|\bqualif(?:y|ies)\b|\b(?:meet|fulfil+|satisfy)\s+(?:this|the)\s+(?:[\w-]+\s+){0,3}requirement\b|\bcounts?\s+towards?\b|\bbe\s+counted\s+towards?\b|\bin\s+lieu\b|\b(?:can|may)\s+be\s+used\b|\bwill\s+be\s+accepted\b|\bacceptable\b`)

// isAlternative reports a sentence that offers another way to meet a
// requirement (complAlternativeRe), or widens its pool with a cap ("Of these
// 9.0 credits, up to 2.0 credits ... may be taken from the following cognate
// departmental or college offerings", complWidenRe).
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
		c.approval = complMatchAny(complApprovalRes, t) || complCountQueryRe.MatchString(t)
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

// complNegationRe: a sentence that refuses an alternative ("MAT133Y1 is not
// a suitable substitute for ...", "they cannot be used as substitutes").
var complNegationRe = regexp.MustCompile(`(?i)\bnot\b|\bcannot\b|n['’]t\b`)

// alternative adds a sentence as another way to meet row: or(strict, text).
// Its kind says what decides it (altKind). A permission that names its
// wider pool is bounded by it once every row is read (boundRoutes, design
// 3.3).
func (a *compAssembler) alternative(row *compRow, s string) {
	if row == nil {
		a.programRoutes = append(a.programRoutes, s)
		return
	}
	if row.r.Req == nil {
		a.textRow(row, s)
		return
	}
	row.r.Req = orNode(row.r.Req, textNode(altKind(s), s))
	addRaw(row, s)
}

// addRaw adds sentence s to row's raw unless the raw has it already: a row's
// raw holds every sentence encoded in it.
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
// decides ("at the department's discretion"), else other. Not membership:
// "count toward the program" is about courses, not programs.
func altKind(s string) types.TextKind {
	if classify(cleanNote(s)) == types.TextPermission {
		return types.TextPermission
	}
	return types.TextOther
}

var (
	// complRefRe: references to numbered requirements: "requirement 2 of this
	// focus", "requirements (4) and (5)".
	complRefRe    = regexp.MustCompile(`(?i)\brequirements?\s+\(?\d{1,2}[a-z]?\)?(?:\s*(?:,|and|or)\s*\(?\d{1,2}[a-z]?\)?)*`)
	complRefNumRe = regexp.MustCompile(`\d{1,2}[a-z]?`)
)

// referenced returns the items a sentence names by their numbers ("may also
// count towards requirement 2"), or nil when a number names no item or more
// than one.
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

// complSatisfiesCountsRe: a course that meets one requirement and counts part
// of its credit toward another: "PHY479Y1 (Undergraduate Research Project)
// satisfies Requirement 3 in Third or Fourth Year and counts as a 0.5 credit
// at the PHY 400-level for Requirement 4 in Third or Fourth Year".
var complSatisfiesCountsRe = regexp.MustCompile(`(?i)^(` + codePat + `)(?:\s*\([^()]*\))?\s+satisfies\s+(requirement\s+\d{1,2}[a-z]?(?:\s+in\s+.+?)?)\s+and\s+counts\s+as\s+an?\s+(\d+(?:\.\d+)?)\s+credit\b.*?\bfor\s+(requirement\s+\d{1,2}[a-z]?(?:\s+in\s+.+?)?)\.?$`)

// satisfiesAndCounts places a complSatisfiesCountsRe sentence and reports
// whether it did. The course's credit is split (design 2, weight): the
// share it counts toward the second requirement, and the rest toward the
// first, where it is an alternative: credits{rest, [course], weight rest}.
// The second requirement takes the sentence as an alternative,
// or(strict, text): the course counts only part of its credit there, which
// a pool can't say of one of its courses.
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

// referencedIn returns the one item a phrase names by number, "Requirement 3
// in Third or Fourth Year": among the items with that number, the one under
// the section named after "in" when there are several. Nil when none or
// several remain.
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

// approval adds an approval route to the row it concerns, as an alternative
// that only a person can grant: or(strict, text permission). This is the
// design's form for a route that names no pool (3.3); one that names its
// pool is bounded by it after placeProgramRoutes (boundRoutes), and Link
// narrows a special-topics pool by the catalogue's titles
// (linkOtherTopics). A route at the program level waits until every
// row is read (placeProgramRoutes): it goes to the rows it names or
// describes, else it is a note.
func (a *compAssembler) approval(target *compRow, s string) {
	if target == nil {
		a.programRoutes = append(a.programRoutes, s)
		return
	}
	alt := textNode(types.TextPermission, cleanNote(s))
	if target.r.Req == nil {
		// A group: the route is a part's alternative, not the group's.
		a.textRow(target, s)
		return
	}
	target.r.Req = orNode(target.r.Req, alt)
	addRaw(target, s)
}

// exclusion reads a sentence naming programs that can't be combined with
// this one, as enrolment does; a phrasing only completion text has stays a
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
	// "Students are not allowed to enroll concurrently in the Major Program
	// in Pharmacology and the Major Program in Toxicology": one of the two is
	// this program, which Link drops.
	complConcurrentRe = regexp.MustCompile(`(?i)\bnot\s+(?:be\s+)?allowed\s+to\s+enrol(?:l)?\s+concurrently\s+in\s+(?:the\s+|a\s+)?(.+?)\s+and\s+(?:the\s+|a\s+)?(.+)$`)
	// "Students registered for a German Specialist or Major may not
	// simultaneously be registered for a German Minor program": the
	// programs in the subject.
	complSimultaneousRe = regexp.MustCompile(`(?i)^students\s+(?:enrolled|registered)\s+(?:in|for)\s+(?:the\s+|a\s+|an\s+)?(.+?)\s+may\s+not\s+simultaneously\b`)
	// "Students completing the Global Health Major (ASMAJ2575) cannot also
	// complete the Applied Genetics and Biotechnology Major (ASMAJ1053), ...".
	complAlsoCompleteRe = regexp.MustCompile(`(?i)\bcannot\s+also\s+complete\s+(.+)$`)
	// "Students enrolled in this Minor program cannot be enrolled in any
	// other program in Economics", "The Neuroscience major cannot be paired
	// with any other Human Biology Program managed major program".
	complOtherProgramRe = regexp.MustCompile(`(?i)\b(?:cannot\s+be\s+enrol(?:l)?ed\s+in|cannot\s+be\s+paired\s+with)\s+(any\s+other\s+.+)$`)
	// A label or list marker before an exclusion: "Note: (a)", "3.".
	complExclusionLeadRe = regexp.MustCompile(`(?i)^(?:note:\s*)?(?:\([a-z]\)\s*)?`)
)

// complExclusionNodes reads the cannot-combine phrasings only completion
// text has, or returns nil.
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

// claimable reports whether n only needs courses: courses, amounts of
// credits and their and, or and choose. No text, cap or average.
func claimable(n *types.ReqNode) bool {
	ok := true
	walkReq(n, func(x *types.ReqNode) {
		switch x.Type {
		case types.NodeCourse, types.NodeAnd, types.NodeOr, types.NodeChoose:
		case types.NodeCredits:
			ok = ok && x.Amount > 0 && x.MaxAmount == nil && !isAnyCredits(x)
		default:
			ok = false
		}
	})
	return ok
}

// sharesCourses reports whether a course could count toward both x and y:
// a code in both, or a code or filter of one that a filter of the other
// matches. Text doesn't count: it is Review whatever courses it is given.
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

// isPool reports a credits node with an amount and a source: a pool.
func isPool(n *types.ReqNode) bool {
	return n != nil && n.Type == types.NodeCredits && n.Amount > 0 && n.Items == nil && (n.Courses != nil || n.Designators != nil)
}

// hasText reports whether n has a text node.
func hasText(n *types.ReqNode) bool {
	found := false
	walkReq(n, func(x *types.ReqNode) { found = found || x.Type == types.NodeText })
	return found
}

func textNode(kind types.TextKind, s string) *types.ReqNode {
	return &types.ReqNode{Type: types.NodeText, Kind: kind, Text: cleanNote(s)}
}

// sameNode reports whether two nodes are the same tree.
func sameNode(x, y *types.ReqNode) bool {
	return len(dedupeNodes([]*types.ReqNode{x, y})) == 1
}

// fmtAmount writes a credit amount as the calendar does: "4.0", "0.5".
func fmtAmount(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }

// complRequiredSentenceRe: courses a sentence says are required, with
// advice after: "PHL271H1, TRN203H1, and TRN204H1 are required courses that
// students are recommended to take in second year".
var complRequiredSentenceRe = regexp.MustCompile(`(?i)^(.*\b` + codePat + `)\s+(?:are|is)\s+(?:the\s+)?required(?:\s+courses?)?\b`)

// complOnlySomeRe: a requirement only some students have: "PRT420H1 is
// required for students who began their Portuguese studies with 200+ level
// PRT language courses", "if …", "unless …".
var complOnlySomeRe = regexp.MustCompile(`(?i)\b(?:for\s+(?:students|those)|if|unless|when|only\s+for|in\s+case)\b`)

// complThatRe: the "that" before the advice after complRequiredSentenceRe.
var complThatRe = regexp.MustCompile(`(?i)^\s*,?\s*(?:that|which)\s+`)

// fixedCourses reports whether n is a course or an and of courses.
func fixedCourses(n *types.ReqNode) bool {
	if n.Type == types.NodeCourse {
		return true
	}
	return n.Type == types.NodeAnd && !slices.ContainsFunc(n.Items, func(it *types.ReqNode) bool { return it.Type != types.NodeCourse })
}
