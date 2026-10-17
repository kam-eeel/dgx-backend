package reqs

import (
	"regexp"
	"slices"
	"strings"
	"sync"

	"degree-explorer/types"
)

// What each sentence role does with a sentence: requirement, loose and
// restated sentences, conditions, exclusions and text rows.

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

// textRow keeps a binding sentence the grammar can't read as a constraint
// whose body is the sentence: Review, never dropped.
func (a *compAssembler) textRow(target *compRow, s string) {
	a.rowUnder(target, a.textReq(s))
}

func (a *compAssembler) textReq(s string) *types.Requirement {
	a.diag.textRows = append(a.diag.textRows, s)
	return &types.Requirement{Kind: types.RequirementConstraint, Req: textNode(types.TextOther, s), Raw: s}
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
