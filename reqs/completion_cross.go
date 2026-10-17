package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Cross-references between requirements, and rules that tie rows together.

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

var (
	// complReqRefRe matches a pool drawn from other requirements' pools: "from
	// Requirements 1, 3, 4, 5, or 6".
	complReqRefRe = regexp.MustCompile(`(?i)\s*\b(?:either\s+)?from\s+(?:the\s+)?(?:remaining\s+courses?\s+in\s+|courses\s+not\s+already\s+(?:taken|used)\s+to\s+fulfil+\s+)?(?:program\s+)?requirements?\s+(\(?#?\d{1,2}\)?(?:\s*(?:,\s*(?:and|or)?|and|or)\s*\(?\d{1,2}\)?)*)(?:\s+above)?`)
	// complReqRefRestRe matches the amount and the courses a pool adds once the
	// requirements are taken out: "1.5 credits, or from: POL314H1".
	complReqRefRestRe = regexp.MustCompile(`(?i)^(?:complete\s+)?(.+?\bcredits?)(?:\s*,?\s*(?:(?:or|and/or)\s+from(?:\s+the\s+approved\s+list\s+of\s+cognate\s+courses)?|(?:,\s*)?(?:or|and)\s+the\s+following\s+courses|with\s+up\s+to\s+` + amountPat + `\s+credits?\s+from)\s*:?\s*(.+))?$`)
)

// compReqRef is a pool that refers to other rows, numbered once the rows are.
type compReqRef struct {
	node *types.ReqNode
	rows []*compRow
	list *types.CourseList // the list whose item node is; nil when node is a row's body
	// scope: node is a rule on the courses rows claim, not a pool drawn from
	// theirs: it names every row, open text or not.
	scope bool
}

// complReqNumRe matches a requirement number: "3".
var complReqNumRe = regexp.MustCompile(`\d{1,2}`)

// reqRefAlloc reads an allocation's text drawing on other rows' pools by label
// as credits{requirements}, or with courses besides, a list of both and maybe a
// cap on the courses: "1.5 credits from requirement 3, or from: POL314H1".
func (a *compAssembler) reqRefAlloc(row *compRow, text string, ls compListRefs) (*types.ReqNode, []*types.Requirement, bool) {
	t := complClean(text)
	m := complReqRefRe.FindStringSubmatchIndex(t)
	if m == nil || len(complReqRefRe.FindAllString(t, -1)) != 1 || row.r.Label == "" {
		return nil, nil, false
	}
	var rows []*compRow
	for _, l := range complReqNumRe.FindAllString(t[m[2]:m[3]], -1) {
		r := a.labelled(l)
		if r == nil || r == row {
			return nil, nil, false
		}
		rows = append(rows, r)
	}
	left := strings.TrimSpace(t[:m[0]] + " " + t[m[1]:])
	// ", or from any of the following:" over lines of courses by subject, the
	// row's notes: those courses.
	var following, keep []string
	if loc := complFromFollowingTailRe.FindStringIndex(left); loc != nil {
		var ok bool
		if following, keep, ok = a.followingCodes(row); !ok {
			return nil, nil, false
		}
		left = strings.TrimSuffix(strings.TrimSpace(left[:loc[0]]), ",")
	}
	rest := complReqRefRestRe.FindStringSubmatch(left)
	if rest == nil || following != nil && rest[3] != "" {
		return nil, nil, false
	}
	if following != nil {
		rest[3] = strings.Join(following, ", ")
	}
	n, _, ok := listRefPoolOf(rest[1], ls, false, []string{"x"})
	if !ok || n.Designators != nil || n.Levels != nil {
		return nil, nil, false
	}
	n.Lists = nil
	if rest[3] == "" {
		n.Requirements = []int{} // filled in once numbered
		a.reqRefs = append(a.reqRefs, compReqRef{node: n, rows: rows})
		return n, nil, true
	}
	codes := complAnyCodeRe.FindAllString(rest[3], -1)
	if codes == nil || !complListRestRe.MatchString(complAnyCodeRe.ReplaceAllString(rest[3], " ")) {
		return nil, nil, false
	}
	ref := &types.ReqNode{Type: types.NodeCredits}
	name, l, ok := a.addUnion(row, []*types.ReqNode{ref, {Type: types.NodeCredits, Courses: dedupe(codes)}}, ls)
	if !ok {
		return nil, nil, false
	}
	a.reqRefs = append(a.reqRefs, compReqRef{node: ref, rows: rows, list: l.list})
	n.Lists = []string{name}
	if following != nil {
		row.r.Notes = keep // the list's lines are its courses now
	}
	var rules []*types.Requirement
	if rest[2] != "" {
		// ", with up to 1.0 credit from ENG215H1, ...": the courses' share.
		v, _ := strconv.ParseFloat(rest[2], 64)
		rules = append(rules, &types.Requirement{Kind: types.RequirementLimit, Raw: cleanNote(text),
			Req: &types.ReqNode{Type: types.NodeCredits, Courses: dedupe(codes), MaxAmount: maxAmount(v)}})
	}
	return n, rules, true
}

// labelled returns the one allocation labelled l, or nil.
func (a *compAssembler) labelled(l string) *compRow {
	return a.uniqueAllocation(func(x string) bool { return x == l })
}

// uniqueAllocation returns the one allocation whose label matches, or nil.
func (a *compAssembler) uniqueAllocation(match func(label string) bool) *compRow {
	var found []*compRow
	walkRows(a.items, func(r *compRow) {
		if r.r.Kind == types.RequirementAllocation && match(r.r.Label) {
			found = append(found, r)
		}
	})
	if len(found) != 1 {
		return nil
	}
	return found[0]
}

// numberReqRefs sets the requirement numbers of the pools that refer to other
// rows, once the rows are numbered. A row with open text has no pool the
// checker can tell, so its courses are a maybe: or(credits{the closed rows},
// and(credits{all the rows}, text)), or in a list a maybe item.
func (a *compAssembler) numberReqRefs() {
	for _, ref := range a.reqRefs {
		if ref.scope {
			for _, r := range ref.rows {
				ref.node.Requirements = append(ref.node.Requirements, r.r.Number)
			}
			slices.Sort(ref.node.Requirements)
			continue
		}
		var closed, open []int
		var labels []string
		for _, r := range ref.rows {
			if r.r.Req == nil || openText(r.r.Req) {
				open = append(open, r.r.Number)
				labels = append(labels, r.r.Label)
			} else {
				closed = append(closed, r.r.Number)
			}
		}
		if open == nil {
			ref.node.Requirements = closed
			continue
		}
		text := textNode(types.TextOther, "the courses of "+refLabels(labels)+", which "+map[bool]string{true: "is", false: "are"}[len(labels) == 1]+" partly text")
		if ref.list != nil {
			maybe := andNode(&types.ReqNode{Type: types.NodeCredits, Requirements: open}, text)
			if closed == nil {
				ref.list.Items = slices.DeleteFunc(ref.list.Items, func(it *types.ReqNode) bool { return it == ref.node })
			} else {
				ref.node.Requirements = closed
			}
			ref.list.Items = append(ref.list.Items, maybe)
			continue
		}
		all := cloneNode(ref.node)
		all.Requirements = append(slices.Clone(closed), open...)
		slices.Sort(all.Requirements)
		if closed == nil {
			*ref.node = *andNode(all, text)
			continue
		}
		strict := cloneNode(ref.node)
		strict.Requirements = closed
		*ref.node = *orNode(strict, andNode(all, text))
	}
}

// refLabels names requirements by their labels: "requirements 6 and 9".
func refLabels(labels []string) string {
	if len(labels) == 1 {
		return "requirement " + labels[0]
	}
	return "requirements " + strings.Join(labels[:len(labels)-1], ", ") + " and " + labels[len(labels)-1]
}

var (
	// complChoicesIncludeRe matches a must-include on a row's choices: "The
	// choices in 3 must satisfy the requirement for an integrative ...".
	complChoicesIncludeRe = regexp.MustCompile(`(?i)^the\s+choices\s+(?:in|from)\s+(?:(\d{1,2})|the\s+program)\s+must\s+satisfy\s+the\s+requirement\s+for\s+an\s+integrative,\s+inquiry-based(?:\s+and/or\s+experiential)?\s+activity\s+by\s+including\s+(?:(?:` + atLeastPat + `\s+)?one|` + atLeastPat + `\s+` + amountPat + `\s+credits?)\s+(?:of|from)\s+the\s+following(?:\s+courses)?\s*:\s*(.+)$`)
	// complIntegrativeMetByRe matches the program's must-include as an item: "The
	// requirement for an integrative ... must be met by completing ...".
	complIntegrativeMetByRe = regexp.MustCompile(`(?i)^the\s+requirement\s+for\s+an\s+integrative,\s+inquiry-based(?:\s+and/or\s+experiential)?\s+activity\s+must\s+be\s+met\s+by\s+completing\s+(?:` + atLeastPat + `\s+)?one\s+of\s+the\s+following(?:\s+courses)?\s*:\s*(.+)$`)
	// complIntegrativeByRe matches "The requirement for an integrative,
	// inquiry-based activity is satisfied by the required course PHY424H1".
	complIntegrativeByRe = regexp.MustCompile(`(?i)^the\s+requirement\s+for\s+an\s+integrative,\s+inquiry-based(?:\s+and/or\s+experiential)?\s+activity\s+is\s+(?:satisfied|met)\s+by\s+the\s+required\s+course\s+(` + codePat + `)$`)
	// complReqScopeRe matches a rule on two numbered rows: "No more than 0.5
	// credit in BCH can be used towards requirements 5. and 6".
	complReqScopeRe = regexp.MustCompile(`(?i)^(?:(.+?)\s+towards?\s+requirements\s+(\d{1,2})\.?\s+and\s+(\d{1,2})\.?|for\s+numbers\s+(\d{1,2})\s+and\s+(\d{1,2})\s+above,\s+(.+))$`)
	// complFloorAndFloorRe matches two floors joined by "and": "2.0 credits must
	// be at the 300-level and a 0.5 credit must be at the 400-level".
	complFloorAndFloorRe = regexp.MustCompile(`(?i)^(` + numPat + `\s+credits?\s+must\s+be\s+.+?)\s+and\s+an?\s+(` + numPat + `\s+credits?\s+must\s+be\s+.+)$`)
	// complPerDesignatorCapRe matches "No more than 1.5 credits of the 2.5
	// credits required from this list ... may come from any single designator".
	complPerDesignatorCapRe = regexp.MustCompile(`(?i)^` + noMoreThanPat + `\s+` + amountPat + `\s+credits?\s+of\s+the\s+` + amountPat + `\s+credits?\s+required\s+from\s+this\s+list(?:\s+of\s+options)?\s+may\s+come\s+from\s+any\s+single\s+(?:3-letter\s+)?(?:course\s+)?designator(?:,?\s+except\s+(?:for\s+)?(` + desigPat + `(?:\s*(?:,|and|or)\s*` + desigPat + `)*)\s+courses)?$`)
	// complNotCountListRe matches courses that don't count toward the program:
	// "... can be taken, but will not count towards your program: ANA300Y1/ ...".
	complNotCountListRe = regexp.MustCompile(`(?i)\bbut\s+will\s+not\s+count\s+towards?\s+(?:your|the|this)\s+program\s*:\s*(.+)$`)
	// complOfTheCreditsRe matches a floor on the item's own amount: "Of the 4.5
	// credits, at least 1.5 credits must come from the following: CHC203Y1/ ...".
	complOfTheCreditsRe = regexp.MustCompile(`(?i)^of\s+the\s+` + amountPat + `\s+credits,\s+` + atLeastPat + `\s+` + amountPat + `\s+credits?\s+must\s+come\s+from\s+the\s+following\s*:\s*(.+)$`)
	// complLevelsListsFloorRe matches "4.0 credits must be 300/400-level
	// (including ...) of which at least 1.0 credit must be from Group A and ...".
	complLevelsListsFloorRe = regexp.MustCompile(`(?i)^` + amountPat + `\s+credits\s+must\s+be\s+(` + levelPat + `(?:\s*/\s*` + levelPat + `)*)-level\s+\(including\s+` + atLeastPat + `\s+` + amountPat + `\s+credits?\s+at\s+the\s+(` + levelPat + `)-level\)\s+of\s+which\s+` + atLeastPat + `\s+` + amountPat + `\s+credits?\s+must\s+be\s+from\s+(` + complListRefPat + `)\s+and\s+another\s+from\s+(` + complListRefPat + `)$`)
	// complAdditionalListsRe matches Req 1's own pool from lists: "Additional
	// courses from Groups A and C for a total of 8.0 credits".
	complAdditionalListsRe = regexp.MustCompile(`(?i)^additional\s+(?:courses|credits)\s+from\s+(` + complListRefPat + `)\s+(?:for|to)\s+a\s+total\s+of\s+` + amountPat + `\s+credits?$`)
	// complNotAlreadyRe matches " not already taken to fulfill requirement 3".
	complNotAlreadyRe = regexp.MustCompile(`(?i)\s+not\s+already\s+(?:taken|used)\s+to\s+fulfil+\s+requirement\s+\d{1,2}\b`)
	// complFromFollowingTailRe matches a lead over a pool's courses set out under
	// it: ", or from any of the following:".
	complFromFollowingTailRe = regexp.MustCompile(`(?i),?\s*(?:or|and/or)\s+from\s+(?:any\s+of\s+)?the\s+following\s*:?\s*$`)
)

// floorRewrites reword the floors and caps the sentence grammar leaves as text
// into wording readFloor and capRow read: "You need a minimum of 4.0 credits of
// 300/400-level courses" becomes "At least 4.0 credits must be at ...".
var floorRewrites = []struct {
	re   *regexp.Regexp
	repl string
}{
	{regexp.MustCompile(`(?i)^of\s+(?:these|the\s+courses\s+chosen),\s+`), ""},
	// "(includes ... requirement)" goes: the item's course counts toward it.
	{regexp.MustCompile(`(?i)\s*\(includes\s+[^()]*\brequirement\)$`), ""},
	{regexp.MustCompile(`(?i)^` + atLeastPat + `\s+one\s+(` + numPat + `\s+credits?)\b`), "at least $1"},
	{regexp.MustCompile(`(?i)^you\s+need\s+a\s+minimum\s+of\s+(` + numPat + `\s+credits?)\s+of\s+(` + levelPat + `(?:\s*[/+]\s*(?:` + levelPat + `)?)*)-level\s+courses$`), "At least $1 must be at the $2-level"},
	{regexp.MustCompile(`(?i)^` + atLeastPat + `\s+(` + numPat + `\s+out\s+of\s+the\s+)`), "$1"},
	{regexp.MustCompile(`(?i)^these\s+credits\s+must\s+include\s+(` + numPat + `\s+credits?)\s+(at\s+the\s+.+?\s+level\s+from\s+` + desigPat + `\s+courses)$`), "At least $1 must be $2"},
	{regexp.MustCompile(`(?i)^students\s+may\s+not\s+use\s+more\s+than\s+(` + numPat + `\s+credits?)\s+(?:towards?\s+their\s+program\s+(?:from|of)\s+(.+)|(?:from|of)\s+(.+?)\s+to\s+fulfil+\s+program\s+requirements)$`), "No more than $1 from $2$3 may be used"},
	// "taken in the Faculty of Arts & Science" goes: FRE and FSL are St.
	// George designators.
	{regexp.MustCompile(`(?i)^(` + atLeastPat + `\s+` + numPat + `\s+credits?\s+must\s+be\s+in\s+` + desigPat + `(?:\s+or\s+` + desigPat + `)*\s+courses)\s+taken\s+in\s+the\s+faculty\s+of\s+arts\s+(?:&|and)\s+science$`), "$1"},
	// "however ... as a program elective" goes: the electives' pool says it.
	{regexp.MustCompile(`(?i)^(` + atLeastPat + `)\s+(?:one\s+)?(` + numPat + `\s+credits?\s+from\s+` + codePat + `\s+or\s+` + codePat + `)\s+is\s+required\s+for\s+the\s+program,\s+however\s+if\s+desired\s+the\s+alternative\s+course\s+can\s+be\s+taken\s+as\s+a\s+program\s+elective$`), "$1 $2"},
}

// compCrossMove is a row crossRules moves under row to, or to the program when
// to is nil.
type compCrossMove struct {
	row *compRow
	to  *compRow
}

// crossRules turns the rows of text that are rules tying rows together into
// constraints and limits where the text puts them. names are the lists in the
// envelope.
func (a *compAssembler) crossRules(lists []*types.CourseList) {
	var names []string
	for _, l := range lists {
		names = append(names, l.Name)
	}
	var moves []compCrossMove
	var walk func(parent *compRow, rows []*compRow) []*compRow
	walk = func(parent *compRow, rows []*compRow) []*compRow {
		var out []*compRow
		for _, r := range rows {
			r.children = walk(r, r.children)
			rs, to, move := a.crossRule(parent, r, names)
			if move {
				for _, x := range rs {
					moves = append(moves, compCrossMove{x, to})
				}
				continue
			}
			out = append(out, rs...)
		}
		return out
	}
	a.items = walk(a.total, a.items)
	a.program = walk(a.total, a.program)
	for _, m := range moves {
		if m.to != nil {
			m.to.children = append(m.to.children, m.row)
		} else {
			a.program = append(a.program, m.row)
		}
	}
	a.totalListRoutes()
}

// totalListRoutes puts a permission route that names only lists Req 1's own
// pool draws on, and no item does, on Req 1: or(pool, text{permission}).
func (a *compAssembler) totalListRoutes() {
	if a.total == nil || a.total.r.Req == nil || a.total.r.Req.Type != types.NodeCredits || a.total.r.Req.Lists == nil {
		return
	}
	pool := a.total.r.Req
	keys := map[string]string{}
	for _, name := range pool.Lists {
		if _, key := listName(name); key != "" {
			keys[key] = name
		}
	}
	visit := func(r *compRow) {
		walkReq(r.r.Req, func(n *types.ReqNode) {
			for _, name := range n.Lists {
				for k, v := range keys {
					if v == name {
						delete(keys, k)
					}
				}
			}
		})
	}
	walkRows(a.items, visit)
	walkRows(a.program, visit)
	a.programRoutes = slices.DeleteFunc(a.programRoutes, func(s string) bool {
		refs := complListRefRe.FindAllString(a.clean(s), -1)
		if refs == nil || !a.isApproval(s) {
			return false
		}
		for _, ref := range refs {
			for _, k := range listRefNames(ref) {
				if keys[k] == "" {
					return false
				}
			}
		}
		a.total.r.Req = orNode(a.total.r.Req, textNode(types.TextPermission, s))
		addRaw(a.total, s)
		return true
	})
}

// crossRule rereads row r under parent (nil: the program) and returns the rows
// it becomes and whether they move: under to, or with to nil, to the program.
func (a *compAssembler) crossRule(parent, r *compRow, names []string) (rows []*compRow, to *compRow, move bool) {
	n := r.r.Req
	if n == nil {
		return []*compRow{r}, nil, false
	}
	// The rule's text, and the alternatives after it: or(text, the ASIP route).
	text, alts := n, []*types.ReqNode(nil)
	if n.Type == types.NodeOr && len(n.Items) > 1 && !slices.ContainsFunc(n.Items, func(it *types.ReqNode) bool { return it.Type != types.NodeText }) {
		text, alts = n.Items[0], n.Items[1:]
	}
	if text.Type != types.NodeText || text.Kind != types.TextOther {
		return []*compRow{r}, nil, false
	}
	t := strings.TrimSuffix(complClean(text.Text), ".")
	switch r.r.Kind {
	case types.RequirementAllocation:
		m := complIntegrativeMetByRe.FindStringSubmatch(t)
		if m == nil || len(r.children) > 0 || parent != a.total || a.total == nil {
			return []*compRow{r}, nil, false
		}
		// The program's must-include as an item is a constraint, never an
		// allocation, which would take the course from the row that requires it.
		codes, ok := includedCodes(m[1])
		if !ok {
			return []*compRow{r}, nil, false
		}
		r.r.Kind, r.r.Label = types.RequirementConstraint, ""
		r.r.Req = orNode(append([]*types.ReqNode{{Type: types.NodeCredits, Amount: 0.5, Courses: codes}}, alts...)...)
		return []*compRow{r}, nil, true
	case types.RequirementConstraint:
	default:
		return []*compRow{r}, nil, false
	}
	if m := complChoicesIncludeRe.FindStringSubmatch(t); m != nil {
		codes, ok := includedCodes(m[3])
		if !ok {
			return []*compRow{r}, nil, false
		}
		amount := 0.5
		if m[2] != "" {
			amount, _ = strconv.ParseFloat(m[2], 64)
		}
		target := parent
		if m[1] != "" {
			if target = a.labelledRef(m[1]); target == nil {
				return []*compRow{r}, nil, false
			}
		}
		if alts == nil {
			alts = a.nextAlternative(r, text.Text)
		}
		r.r.Req = orNode(append([]*types.ReqNode{{Type: types.NodeCredits, Amount: amount, Courses: codes}}, alts...)...)
		if target == parent {
			return []*compRow{r}, nil, false
		}
		return []*compRow{r}, target, true
	}
	if alts != nil {
		return []*compRow{r}, nil, false
	}
	if m := complIntegrativeByRe.FindStringSubmatch(t); m != nil && a.requiresCourse(m[1]) {
		// Met by a course an item requires: it holds whenever the item does.
		r.r.Req = &types.ReqNode{Type: types.NodeCredits, Amount: courseWeight(m[1]), Courses: []string{m[1]}}
		return []*compRow{r}, nil, false
	}
	if m := complReqScopeRe.FindStringSubmatch(t); m != nil {
		if rs, ok := a.scopeRules(r, m); ok {
			return rs, nil, false
		}
		return []*compRow{r}, nil, false
	}
	if m := complNotCountListRe.FindStringSubmatch(t); m != nil {
		// Courses that don't count toward the program: the program's cap of 0.
		codes, ok := includedCodes(m[1])
		if !ok {
			return []*compRow{r}, nil, false
		}
		r.r.Kind = types.RequirementLimit
		r.r.Req = &types.ReqNode{Type: types.NodeCredits, Courses: codes, MaxAmount: maxAmount(0)}
		return []*compRow{r}, nil, parent != a.total
	}
	if m := complPerDesignatorCapRe.FindStringSubmatch(t); m != nil {
		if rs, ok := perDesignatorCaps(parent, r, m); ok {
			return rs, nil, false
		}
		return []*compRow{r}, nil, false
	}
	if m := complOfTheCreditsRe.FindStringSubmatch(t); m != nil {
		total, _ := strconv.ParseFloat(m[1], 64)
		v, _ := strconv.ParseFloat(m[2], 64)
		codes, ok := includedCodes(m[3])
		if !ok || parent == nil || itemAmount(parent.r.Req) != total {
			return []*compRow{r}, nil, false
		}
		r.r.Req = &types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: codes}
		return []*compRow{r}, nil, false
	}
	if m := complLevelsListsFloorRe.FindStringSubmatch(t); m != nil {
		return a.levelsListsFloors(parent, r, m, names), nil, false
	}
	u := t
	for _, rw := range floorRewrites {
		u = rw.re.ReplaceAllString(u, rw.repl)
	}
	if fs, ok := readFloor(u); ok {
		return a.newRules(parent, r, fs), nil, false
	}
	if c := capRow(u); c != nil && u != t {
		return a.newRules(parent, r, []*types.Requirement{c}), nil, false
	}
	return []*compRow{r}, nil, false
}

// newRules makes rules rs, read from row r's sentence, rows in r's place,
// keeping r's raw, label and notes. A rule a sibling states already gives that
// sibling the raw instead of a second row.
func (a *compAssembler) newRules(parent, r *compRow, rs []*types.Requirement) []*compRow {
	var siblings []*compRow
	if parent == a.total {
		siblings = append(slices.Clone(a.floors), a.program...)
	}
	if parent != nil {
		siblings = append(siblings, parent.children...)
	}
	var out []*compRow
	for _, x := range rs {
		if i := slices.IndexFunc(siblings, func(s *compRow) bool {
			return s != r && s.r.Kind == x.Kind && s.r.Req != nil && sameNode(s.r.Req, x.Req)
		}); i >= 0 {
			addRaw(siblings[i], r.r.Raw)
			continue
		}
		x.Raw = r.r.Raw
		out = append(out, &compRow{r: x})
	}
	if out != nil {
		out[0].r.Label, out[0].r.Section, out[0].r.Notes = r.r.Label, r.r.Section, r.r.Notes
		out[0].children = r.children
	}
	return out
}

// includedCodes reads a must-include's courses, codes with titles in brackets:
// "JSC470H1 (Data Science III)". ok is false when anything else is left.
func includedCodes(s string) ([]string, bool) {
	s = strings.TrimSuffix(strings.TrimSpace(stripEmphasis(s)), ".")
	s = complTitleParenRe.ReplaceAllStringFunc(s, func(p string) string {
		if complAnyCodeRe.MatchString(p) {
			return p
		}
		return " "
	})
	codes := complAnyCodeRe.FindAllString(s, -1)
	if codes == nil || !complListRestRe.MatchString(complAnyCodeRe.ReplaceAllString(s, " ")) {
		return nil, false
	}
	return dedupe(codes), true
}

// labelledRef returns the one allocation label l names: the row labelled l,
// else the one row whose label ends in it ("Later years 3"), or nil.
func (a *compAssembler) labelledRef(l string) *compRow {
	if r := a.labelled(l); r != nil {
		return r
	}
	return a.uniqueAllocation(func(x string) bool { return strings.HasSuffix(x, " "+l) })
}

// nextAlternative returns the sentence after rule text s on its line, as the
// rule's alternative, when it points back at it: "Students who complete the
// ASIP stream will also meet this requirement". Nil when there is none.
func (a *compAssembler) nextAlternative(r *compRow, s string) []*types.ReqNode {
	for i := range a.lines {
		sents := a.lineSentences(i)
		for k, x := range sents[:max(len(sents)-1, 0)] {
			next := sents[k+1]
			if rawWords(cleanNote(x)) != rawWords(s) || !complThisRequirementRe.MatchString(next) || !a.isAlternative(next) {
				continue
			}
			// The sentence waits as a program route, having named no row, or a
			// note.
			same := func(n string) bool { return rawWords(cleanNote(n)) == rawWords(cleanNote(next)) }
			if !slices.ContainsFunc(a.notes, same) && !slices.ContainsFunc(a.programRoutes, same) {
				return nil
			}
			a.notes = slices.DeleteFunc(a.notes, same)
			a.programRoutes = slices.DeleteFunc(a.programRoutes, same)
			addRaw(r, next)
			return []*types.ReqNode{textNode(altKind(next), next)}
		}
	}
	return nil
}

// requiresCourse reports whether an item requires course code by itself,
// outside any or.
func (a *compAssembler) requiresCourse(code string) bool {
	var req func(n *types.ReqNode) bool
	req = func(n *types.ReqNode) bool {
		switch {
		case n == nil:
			return false
		case n.Type == types.NodeCourse:
			return n.Code == code
		case n.Type == types.NodeAnd:
			return slices.ContainsFunc(n.Items, req)
		}
		return false
	}
	return slices.ContainsFunc(a.items, func(r *compRow) bool { return r.r.Kind == types.RequirementAllocation && req(r.r.Req) })
}

// scopeRules reads a rule on two sibling rows into rules over the courses they
// claim together. "Must be at the 300-level" may mean 300+: or(strict,
// and(lenient, text{ambiguity})), strict Met, lenient-only Review.
func (a *compAssembler) scopeRules(r *compRow, m []string) ([]*compRow, bool) {
	rule, l1, l2 := m[1], m[2], m[3]
	if rule == "" {
		rule, l1, l2 = m[6], m[4], m[5]
	}
	x, y := a.labelledRef(l1), a.labelledRef(l2)
	if x == nil || y == nil || x == y || !a.siblings(x, y) {
		return nil, false
	}
	var rs []*types.Requirement
	if c := capRow(rule); c != nil {
		rs = []*types.Requirement{c}
	} else {
		parts := []string{rule}
		if f := complFloorAndFloorRe.FindStringSubmatch(rule); f != nil {
			parts = f[1:]
		}
		for _, p := range parts {
			fs, ok := readFloor("At least " + p)
			if !ok || len(fs) != 1 {
				return nil, false
			}
			rs = append(rs, fs[0])
		}
	}
	for _, x := range rs {
		if x.Req.Type != types.NodeCredits || x.Req.Lists != nil || x.Req.Requirements != nil {
			return nil, false
		}
	}
	var out []*compRow
	for _, q := range rs {
		q.Raw = r.r.Raw
		ref := q.Req
		if q.Kind == types.RequirementConstraint && slices.Equal(ref.Levels, []int{300}) {
			wide := cloneNode(ref)
			wide.Levels = []int{300, 400}
			a.reqRefs = append(a.reqRefs, compReqRef{node: wide, rows: []*compRow{x, y}, scope: true})
			q.Req = orNode(ref, andNode(wide, textNode(types.TextAmbiguity, "at the 300-level or above")))
		}
		ref.Requirements = []int{}
		a.reqRefs = append(a.reqRefs, compReqRef{node: ref, rows: []*compRow{x, y}, scope: true})
		out = append(out, &compRow{r: q})
	}
	out[0].r.Notes = r.r.Notes
	return out, true
}

// siblings reports whether rows x and y have the same parent.
func (a *compAssembler) siblings(x, y *compRow) bool {
	if slices.Contains(a.items, x) && slices.Contains(a.items, y) {
		return true
	}
	var found bool
	walkRows(a.items, func(r *compRow) {
		found = found || slices.Contains(r.children, x) && slices.Contains(r.children, y)
	})
	return found
}

// perDesignatorCaps reads a complPerDesignatorCapRe cap on item parent's pool
// of listed courses: a limit on each designator's courses that could pass it.
func perDesignatorCaps(parent, r *compRow, m []string) ([]*compRow, bool) {
	v, _ := strconv.ParseFloat(m[1], 64)
	total, _ := strconv.ParseFloat(m[2], 64)
	if parent == nil || parent.r.Req == nil {
		return nil, false
	}
	p := parent.r.Req
	if p.Type != types.NodeCredits || p.Amount != total || p.Courses == nil || p.Designators != nil || p.Lists != nil || p.Requirements != nil {
		return nil, false
	}
	except := desigRe.FindAllString(m[3], -1)
	by := map[string][]string{}
	var order []string
	for _, c := range p.Courses {
		d := c[:3]
		if slices.Contains(except, d) {
			continue
		}
		if by[d] == nil {
			order = append(order, d)
		}
		by[d] = append(by[d], c)
	}
	var out []*compRow
	for _, d := range order {
		sum := 0.0
		for _, c := range by[d] {
			sum += courseWeight(c)
		}
		if sum <= v {
			continue // the designator's courses can't pass the cap
		}
		out = append(out, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Raw: r.r.Raw,
			Req: &types.ReqNode{Type: types.NodeCredits, Courses: slices.Clone(by[d]), MaxAmount: maxAmount(v)}}})
	}
	if out == nil {
		return nil, false
	}
	out[0].r.Notes = r.r.Notes
	return out, true
}

// levelsListsFloors reads complLevelsListsFloorRe: a floor at levels, one at
// a level within it, and one from each of two lists within the first.
func (a *compAssembler) levelsListsFloors(parent, r *compRow, m []string, names []string) []*compRow {
	byKey := map[string]string{}
	for _, name := range names {
		if _, key := listName(name); key != "" {
			byKey[key] = name
		}
	}
	resolve := func(ref string) []string {
		keys := listRefNames(ref)
		if len(keys) != 1 || byKey[keys[0]] == "" {
			return nil
		}
		return []string{byKey[keys[0]]}
	}
	n1, n2 := resolve(m[6]), resolve(m[7])
	if n1 == nil || n2 == nil {
		return []*compRow{r}
	}
	num := func(s string) float64 { v, _ := strconv.ParseFloat(s, 64); return v }
	levels := parseLevels(m[2] + "-level")
	rs := []*types.Requirement{
		{Kind: types.RequirementConstraint, Req: &types.ReqNode{Type: types.NodeCredits, Amount: num(m[1]), Levels: levels}},
		{Kind: types.RequirementConstraint, Req: &types.ReqNode{Type: types.NodeCredits, Amount: num(m[3]), Levels: parseLevels(m[4] + "-level")}},
		{Kind: types.RequirementConstraint, Req: &types.ReqNode{Type: types.NodeCredits, Amount: num(m[5]), Levels: slices.Clone(levels), Lists: n1}},
		{Kind: types.RequirementConstraint, Req: &types.ReqNode{Type: types.NodeCredits, Amount: num(m[5]), Levels: slices.Clone(levels), Lists: n2}},
	}
	return a.newRules(parent, r, rs)
}

// additionalLists reads a complAdditionalListsRe item as Req 1's own pool:
// its raw and routes alts join Req 1's. It reports false, reading nothing,
// when the item doesn't fit or Req 1 has a pool already.
func (a *compAssembler) additionalLists(row *compRow, text string, alts []*types.ReqNode, ls compListRefs) bool {
	t := strings.TrimSpace(complAlreadyCountedRe.ReplaceAllString(strings.TrimSuffix(complClean(text), "."), ""))
	m := complAdditionalListsRe.FindStringSubmatch(t)
	if m == nil || a.total == nil || len(row.children) > 0 || !slices.Contains(a.items, row) {
		return false
	}
	pool := a.total.r.Req
	v, _ := strconv.ParseFloat(m[2], 64)
	if pool == nil || pool.Type != types.NodeCredits || pool.Amount != v || pool.Designators != nil || pool.Levels != nil || pool.Courses != nil || pool.Lists != nil {
		return false
	}
	names := ls.resolve(listRefNames(m[1]))
	if names == nil {
		return false
	}
	pool.Lists = names
	if alts != nil {
		a.total.r.Req = orNode(append([]*types.ReqNode{pool}, alts...)...)
	}
	a.total.r.Raw += "\n" + row.r.Raw
	a.total.r.Notes = append(a.total.r.Notes, row.r.Notes...)
	return true
}

// followingCodes returns the courses of the list definitions that are all of
// row's notes, and the notes to keep: lines with more than codes ("CAR32H1").
// ok is false when a line has no code or names a filter or a person.
func (a *compAssembler) followingCodes(row *compRow) (codes, keep []string, ok bool) {
	var n int
	for _, d := range a.listDefs {
		if !slices.Contains(row.r.Notes, d.note) {
			continue
		}
		n++
		items, except, perm, ok := listMembersPerm(d.members)
		switch {
		case !ok:
			c := complAnyCodeRe.FindAllString(d.members, -1)
			if c == nil || complListFilterRe.MatchString(d.members) || complListPermRe.MatchString(d.members) {
				return nil, nil, false
			}
			codes = append(codes, c...)
			keep = append(keep, d.note)
		case except != nil || perm != "" || len(items) != 1 || items[0].Courses == nil || items[0].Weight != 0:
			return nil, nil, false
		default:
			codes = append(codes, items[0].Courses...)
		}
	}
	if n == 0 || n != len(row.r.Notes) {
		return nil, nil, false
	}
	return dedupe(codes), keep, true
}

// itemAmount returns the amount an item's body asks for: a pool's, or the one
// a text item starts with; 0 when it has none.
func itemAmount(n *types.ReqNode) float64 {
	switch {
	case n == nil:
	case n.Type == types.NodeCredits:
		return n.Amount
	case n.Type == types.NodeText:
		if m := complAmountLeadRe.FindStringSubmatch(n.Text); m != nil {
			v, _ := strconv.ParseFloat(m[1], 64)
			return v
		}
	}
	return 0
}
