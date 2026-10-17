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

// Spreads over lists: an amount spread over named lists, each of several lists,
// shares that add up, and items and floors on lists named by title.

var countWords = map[string]int{"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8}

// countOf reads a count written as a word or digits; 0 when it is neither.
func countOf(s string) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return countWords[strings.ToLower(strings.TrimSpace(s))]
}

// partsNode returns at least k of parts: their and when k is all, else
// choose{k, parts}. Shared courses count toward one part only: the strict
// reading (parts without them) is Met, the parts as they are only Review.
func partsNode(k int, parts []*types.ReqNode, ls compListRefs) (*types.ReqNode, bool) {
	shared, ok := ls.sharedCourses(parts)
	if !ok {
		return nil, false
	}
	build := func(strict bool) *types.ReqNode {
		var items []*types.ReqNode
		for i, p := range parts {
			it := grammar.CloneNode(p)
			if strict && shared[i] != nil {
				if it.Lists == nil {
					it.Courses = slices.DeleteFunc(slices.Clone(it.Courses), func(c string) bool { return slices.Contains(shared[i], c) })
				} else {
					it.Exclude = grammar.Dedupe(append(slices.Clone(it.Exclude), shared[i]...))
				}
			}
			items = append(items, it)
		}
		if k == len(items) {
			return programs.AndNode(items...)
		}
		return &types.ReqNode{Type: types.NodeChoose, Count: k, Items: items}
	}
	// the parts each shared course is on: "ENV341H1 is on Cluster 3 and 4"
	on := map[string][]string{}
	var all []string
	for i, cs := range shared {
		for _, c := range cs {
			name := "part " + strconv.Itoa(i+1)
			if parts[i].Lists != nil {
				name = parts[i].Lists[0]
			}
			on[c] = append(on[c], name)
			all = append(all, c)
		}
	}
	if all = grammar.Dedupe(all); all == nil {
		return build(false), true
	}
	slices.Sort(all)
	var says []string
	for _, c := range all {
		where := on[c]
		says = append(says, c+" is on "+strings.Join(where[:len(where)-1], ", ")+" and "+where[len(where)-1])
	}
	why := strings.Join(says, "; ") + ": a course counts toward one of them only"
	if parts[0].Lists == nil {
		why = strings.Join(all, ", ") + " counts toward one of the parts only"
	}
	return programs.OrNode(build(true), programs.AndNode(build(false), TextNode(types.TextAmbiguity, why))), true
}

// spreadNode returns each credits from each of at least k of names, at
// levels.
func spreadNode(k int, each float64, names []string, levels []int, ls compListRefs) (*types.ReqNode, bool) {
	if k < 1 || k > len(names) {
		return nil, false
	}
	var parts []*types.ReqNode
	for _, name := range names {
		parts = append(parts, &types.ReqNode{Type: types.NodeCredits, Amount: each, Lists: []string{name}, Levels: slices.Clone(levels)})
	}
	return partsNode(k, parts, ls)
}

// rowLines records in out the line each of rows, and the rows under them, was
// read from: the line equal to its raw head, else the first that holds it.
func (a *Assembler) rowLines(rows []*compRow, out map[*compRow]int) {
	for _, r := range rows {
		head := rawWords(RawHead(r.r))
		i := slices.IndexFunc(a.lines, func(l string) bool { return head != "" && rawWords(l) == head })
		if i < 0 {
			i = slices.IndexFunc(a.lines, func(l string) bool { return head != "" && strings.Contains(rawWords(l), head) })
		}
		if i >= 0 {
			out[r] = i
		}
		a.rowLines(r.children, out)
	}
}

// leadLists returns the lists a lead over lists below draws on: its own
// sub-lists, else the top-level lists defined after its line and before the
// next row's; nil when there are none or one is bad.
func (a *Assembler) leadLists(row *compRow, ls compListRefs) []string {
	return a.leadListsOf(row, ls, false)
}

// leadListsOf is leadLists; with last, lists after every row's line count
// too, for a lead that says how many lists it means.
func (a *Assembler) leadListsOf(row *compRow, ls compListRefs, last bool) []string {
	var own, after []*compList
	lines := map[*compRow]int{}
	a.rowLines(a.items, lines)
	a.rowLines(a.program, lines)
	at, ok := lines[row]
	next := math.MaxInt32
	end := -1
	for r, l := range lines {
		if r != row && l > at && l < next {
			next = l
		}
		end = max(end, l)
	}
	if last {
		at, next = max(at, end), math.MaxInt32
	}
	for _, l := range a.sortedLists(ls) {
		d := l.defs[0]
		switch {
		case d.row == row:
			own = append(own, l)
		case ok && d.row == nil && d.block != nil && l.parent == "" && d.block.line > at && d.block.line < next:
			after = append(after, l)
		}
	}
	if own == nil {
		own = after
	}
	var names []string
	for _, l := range own {
		if l.bad {
			return nil
		}
		names = append(names, l.list.Name)
	}
	return names
}

// spreadRow is listRefRow's first reading, of spreads, choices and shares
// among lists. It returns the rows row becomes, and whether it read row.
func (a *Assembler) spreadRow(row *compRow, ls compListRefs) ([]*compRow, bool) {
	switch row.r.Kind {
	case types.RequirementAllocation:
		if row.r.Req == nil {
			return []*compRow{row}, a.areaGroup(row, ls)
		}
		if rangeParts(row.r.Req, ls) {
			return []*compRow{row}, true
		}
		if t, put, ok := rowText(row); ok {
			if n := titledBelow(t, ls); n != nil {
				put(n)
				return []*compRow{row}, true
			}
		}
		if a.shares(row) || a.pathways(row, ls) || a.selections(row, ls) || a.titledMinimum(row, ls) || a.cognatePool(row, ls) || a.areaChoice(row) || a.spreadAlloc(row, ls) || a.categoryCaps(row, ls) || a.combinationCap(row, ls) || a.traditionLists(row, ls) || a.remainingFromLists(row, ls) || a.websiteLists(row, ls) || a.halfCourseFromList(row, ls) {
			return []*compRow{row}, true
		}
	case types.RequirementConstraint:
		return a.spreadRule(row, ls)
	}
	return nil, false
}

var (
	// rangeListsPartRe matches a range part of an item over lists: "2.0 to
	// 3.0 credits from Group A and 1.0 credit from Group B/Group C".
	rangeListsPartRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+to\s+` + grammar.NumPat + `\s+credits?\s+from\s+(` + listRefPat + `)\s+and\s+` + grammar.AmountPat + `\s+credits?\s+from\s+(` + listRefPat + `)$`)
	// rangeLanguagesPartRe matches a range in named languages: "1.0 to 2.0
	// credits in Ancient Stream languages: Akkadian, Ancient Egyptian".
	rangeLanguagesPartRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+to\s+` + grammar.NumPat + `\s+credits?\s+in\s+[\w ]+?\s+languages\s*:\s*\S.*$`)
	// rangeDesigPartRe matches a range in a designator's courses: "1.0 to
	// 2.0 credits in language (NML courses)".
	rangeDesigPartRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+to\s+` + grammar.NumPat + `\s+credits?\s+in\s+[a-z]+\s+\(\s*(` + grammar.DesigPat + `)\s+courses\s*\)$`)
)

// rangeParts reads an and's text parts that give a range of credits as the
// range's lower end. It reports whether it read one.
func rangeParts(n *types.ReqNode, ls compListRefs) bool {
	if n.Type != types.NodeAnd {
		return false
	}
	read := false
	for i, it := range n.Items {
		if it.Type != types.NodeText || it.Kind != types.TextOther {
			continue
		}
		t := strings.TrimSuffix(Clean(it.Text), ".")
		if m := rangeDesigPartRe.FindStringSubmatch(t); m != nil {
			v, _ := strconv.ParseFloat(m[1], 64)
			n.Items[i] = &types.ReqNode{Type: types.NodeCredits, Amount: v, Designators: []string{m[2]}}
			read = true
			continue
		}
		if m := rangeLanguagesPartRe.FindStringSubmatch(t); m != nil {
			// no node can say which courses are in those languages
			v, _ := strconv.ParseFloat(m[1], 64)
			n.Items[i] = programs.AndNode(&types.ReqNode{Type: types.NodeCredits, Amount: v}, TextNode(types.TextContent, it.Text))
			read = true
			continue
		}
		if m := rangeListsPartRe.FindStringSubmatch(t); m != nil {
			first, second := ls.resolve(listRefNames(m[2])), ls.resolve(listRefNames(m[4]))
			if first == nil || second == nil {
				continue
			}
			v, _ := strconv.ParseFloat(m[1], 64)
			w, _ := strconv.ParseFloat(m[3], 64)
			parts := []*types.ReqNode{{Type: types.NodeCredits, Amount: v, Lists: first}, {Type: types.NodeCredits, Amount: w, Lists: second}}
			// a shared course counts toward one part: strict is Met, as-is Review
			codes := func(names []string) (out []string, ok bool) {
				for _, name := range names {
					c, f, ok := ls.members(name)
					if !ok || f != nil {
						return nil, false
					}
					out = append(out, c...)
				}
				return out, true
			}
			a, okA := codes(first)
			b, okB := codes(second)
			if !okA || !okB {
				continue
			}
			var shared []string
			for _, c := range grammar.Dedupe(a) {
				if slices.Contains(b, c) {
					shared = append(shared, c)
				}
			}
			n.Items[i] = programs.AndNode(parts...)
			if shared != nil {
				strict := programs.AndNode(grammar.CloneNode(parts[0]), grammar.CloneNode(parts[1]))
				for _, p := range strict.Items {
					p.Exclude = slices.Clone(shared)
				}
				n.Items[i] = programs.OrNode(strict, programs.AndNode(parts[0], parts[1], TextNode(types.TextAmbiguity, strings.Join(shared, ", ")+" counts toward one of the parts only")))
			}
			read = true
		}
	}
	return read
}

// rowText returns the text an allocation's body is (or the first item of
// or(text, routes)), and a function that puts a node in its place.
func rowText(row *compRow) (string, func(*types.ReqNode), bool) {
	n := row.r.Req
	if n == nil {
		return "", nil, false
	}
	if n.Type == types.NodeText && n.Kind == types.TextOther {
		return Clean(n.Text), func(b *types.ReqNode) { row.r.Req = b }, true
	}
	if n.Type == types.NodeOr && len(n.Items) > 1 && n.Items[0].Type == types.NodeText && n.Items[0].Kind == types.TextOther &&
		!slices.ContainsFunc(n.Items[1:], func(it *types.ReqNode) bool { return it.Type != types.NodeText || it.Kind != types.TextPermission }) {
		return Clean(n.Items[0].Text), func(b *types.ReqNode) {
			if b.Type == types.NodeOr {
				n.Items = append(slices.Clone(b.Items), n.Items[1:]...) // one or, not or(or(...), route)
				return
			}
			n.Items[0] = b
		}, true
	}
	return "", nil, false
}

// seeBelowPat matches an optional "(see below for definitions)".
const seeBelowPat = `(?:\s*\(\s*see\s+below(?:\s+for\s+definitions)?\s*\))?`

var (
	// spreadLeadRe matches a pool over lists with how many it must draw on:
	// "1.5 credits, chosen from at least two of the four clusters below".
	spreadLeadRe = regexp.MustCompile(`(?i)^(?:` + grammar.AtLeastPat + `\s+)?` + grammar.AmountPat + `\s+credits?,?\s+(?:(?:chosen|selected|taken)\s+)?(?:from|in)\s+(?:the\s+following,?\s+so\s+that\s+(?:the\s+)?courses\s+are\s+from\s+)?(?:` + grammar.AtLeastPat + `\s+)?(one|two|three|four|five|\d)\s+of\s+the\s+(?:(two|three|four|five|six|seven|eight|\d)\s+)?(?:following\s+)?(?:clusters|groups|areas|lists|categories|concentrations)(?:\s+of\s+concentration)?(?:\s+below)?(.*)$`)
	// spreadTailRe matches a spread after or under the pool: "including one
	// or more credits from at least three of the four groups".
	spreadTailRe = regexp.MustCompile(`(?i)^(?:including\s+)?(?:` + grammar.AtLeastPat + `\s+)?(one\s+or\s+more\s+credits?|` + grammar.NumPat + `\s+credits?)\s+(?:from|in)\s+(?:` + grammar.AtLeastPat + `\s+)?(one|two|three|four|five|\d)\s+of\s+the\s+(two|three|four|five|six|seven|eight|\d)\s+(?:clusters|groups|areas|lists|categories|concentrations)(?:\s*\(\s*` + grammar.AmountPat + `\s+credits?\s+in\s+total\s*\))?$`)
	// spreadInRe matches a spreadTailRe clause inside text: "(at least
	// 1.0 credit from 3 of the 5 areas)".
	spreadInRe = regexp.MustCompile(`(?i)[,;]?\s*\(?\s*\b(?:including\s+)?(?:` + grammar.AtLeastPat + `\s+)?(?:one\s+or\s+more\s+credits?|` + grammar.NumPat + `\s+credits?)\s+(?:from|in)\s+(?:` + grammar.AtLeastPat + `\s+)?(?:one|two|three|four|five|\d)\s+of\s+the\s+(?:two|three|four|five|six|seven|eight|\d)\s+(?:clusters|groups|areas|lists|categories|concentrations)\b(?:\s*\(\s*` + grammar.NumPat + `\s+credits?\s+in\s+total\s*\))?\s*\)?`)
	// spreadEachRe matches the least from each list of a spread: "with at
	// least 0.5 credit in each of the three selected concentrations".
	spreadEachRe = regexp.MustCompile(`(?i)^(?:with\s+)?` + grammar.AtLeastPat + `\s+` + grammar.AmountPat + `\s+credits?\s+in\s+each\s+of\s+the\s+(two|three|four|five|\d)\s+selected\s+\w+$`)
	// followingRequirementsRe matches a lead whose rules are the lines under
	// it: ", with the following requirements:".
	followingRequirementsRe = regexp.MustCompile(`(?i),?\s+with\s+the\s+following\s+requirements\s*:?$`)
	// eachListRe matches each of the lists below: "0.5 credit from each of
	// the three Thematic Clusters".
	eachListRe = regexp.MustCompile(`(?i)^(?:[\w\s&-]+\(\s*` + grammar.AmountPat + `\s+credits?\s*\)\s*:\s*)?` + grammar.AmountPat + `\s+credits?\s+from\s+each\s+of\s+the\s+(two|three|four|five|\d)\s+(?:[\w-]+\s+)?(?:clusters|groups|areas|lists|categories|concentrations)$`)
	// kOfGroupsRe matches k of n lists: "At least one 0.5 credit in three of
	// the four FAH Groups".
	kOfGroupsRe = regexp.MustCompile(`(?i)^(?:` + grammar.AtLeastPat + `\s+)?(?:one\s+)?` + grammar.AmountPat + `\s+credits?\s+(?:in|from)\s+(one|two|three|four|five|\d)\s+of\s+the\s+(two|three|four|five|six|seven|eight|\d)\s+(?:` + grammar.DesigPat + `\s+)?(?:groups|lists|areas|clusters)` + seeBelowPat + `$`)
	// eachGroupLeadRe matches a lead over its own lists with a floor on
	// each: "3.0 credits from among the following courses, including at least 0.5
	// credit from each group".
	eachGroupLeadRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\s+from\s+(?:among\s+)?the\s+following(?:\s+courses)?,?\s+including\s+` + grammar.AtLeastPat + `\s+` + grammar.AmountPat + `\s+credits?\s+from\s+each\s+(?:group|list|area|cluster)$`)
	// followingCoursesRe matches a pool over the lists under it: "1.5
	// credits from the following courses".
	followingCoursesRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\s+(?:from|in)\s+(?:(?:among\s+)?the\s+following\s+(?:courses|lists|groups)|the\s+(?:groups|lists)\s+(?:listed\s+)?(below))$`)
	// applyUnderRe matches a sentence letting a student ask for courses to
	// count: "students may apply to have these courses considered for credit
	// under Requirement 2".
	applyUnderRe = regexp.MustCompile(`(?i)\bstudents\s+may\s+apply\s+to\s+have\s+these\s+courses\s+considered\s+for\s+credit\s+under\s+requirement\s+\d{1,2}$`)
	// rangeFromListsRe matches a range of credits from lists: "3.0 to 3.5
	// additional credits chosen from Groups A, B, and/or C".
	rangeFromListsRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s*(?:to|-|–)\s*` + grammar.AmountPat + `\s+(?:additional\s+)?credits?\s+(?:(?:chosen|selected|taken)\s+)?(?:from|in)\s+(` + listRefPat + `)$`)
	// coreOrListRe matches "1.0 credit from the Core Group or Group A".
	coreOrListRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\s+from\s+the\s+core\s+group\s+or\s+(` + listRefPat + `)$`)
	// desigsOrListBelowRe matches designators or the list below: "Additional
	// 1.0 or 1.5 credits from SAS and CAS courses or the list of eligible courses
	// offered by other departments found below".
	desigsOrListBelowRe = regexp.MustCompile(`(?i)^additional\s+` + grammar.AmountPat + `\s+or\s+` + grammar.NumPat + `\s+credits?\s*(?:\([^()]*\))?\s+from\s+(` + grammar.DesigPat + `(?:\s*(?:,|and)\s*` + grammar.DesigPat + `)*)\s+courses\s+or\s+the\s+list\s+of\s+eligible\s+courses\s+offered\s+by\s+other\s+departments\s+(?:found\s+)?below$`)
	// fromGroupsEachRe matches a pool over lists with a floor on each: "4.5
	// credits from Group A and B courses, with at least two credits from each
	// group".
	fromGroupsEachRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\s+from\s+(` + listRefPat + `)(?:\s+courses)?,?\s+with\s+` + grammar.AtLeastPat + `\s+(` + grammar.NumPat + `|one|two|three|four)\s+credits?\s+(?:from|in)\s+each(?:\s+(?:group|list|area|cluster))?$`)
	// eachOfGroupsRe matches "At least 0.5 credit from each of Groups A, B,
	// C, and D".
	eachOfGroupsRe = regexp.MustCompile(`(?i)^(?:` + grammar.AtLeastPat + `\s+)?` + grammar.AmountPat + `\s+(?:additional\s+)?credits?\s+(?:in|from)\s+each\s+of\s+(` + listRefPat + `)` + seeBelowPat + `$`)
	// oneOrTwoRe matches "1.0 credit from either one concentration below, or
	// 0.5 credit from two concentrations below".
	oneOrTwoRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\s+from\s+either\s+one\s+(?:concentration|cluster|group|area|list)\s+below,?\s+or\s+` + grammar.AmountPat + `\s+credits?\s+from\s+(two|\d)\s+(?:concentrations|clusters|groups|areas|lists)\s+below$`)
)

// orAboveRe matches a level or above: "300-level or above".
var orAboveRe = regexp.MustCompile(`(?i)\b(` + grammar.LevelPat + `)\s*-?\s*level\s+or\s+(?:above|higher)\b`)

// orAbove rewrites "300-level or above" as "300+ level", which listRefRule
// reads.
func orAbove(s string) string { return orAboveRe.ReplaceAllString(s, "$1+ level") }

// spreadAlloc reads an allocation of text that spreads its amount over
// lists, or draws on each of them, or on one or two of them.
func (a *Assembler) spreadAlloc(row *compRow, ls compListRefs) bool {
	t, put, ok := rowText(row)
	if !ok {
		return false
	}
	t = strings.TrimSpace(strings.TrimSuffix(ItemLabelRe.ReplaceAllString(t, ""), "."))
	if m := spreadLeadRe.FindStringSubmatch(t); m != nil {
		names := a.leadLists(row, ls)
		if names == nil && m[3] != "" {
			names = a.leadListsOf(row, ls, true)
		}
		amount, _ := strconv.ParseFloat(m[1], 64)
		k := countOf(m[2])
		if names == nil || m[3] != "" && countOf(m[3]) != len(names) || k >= len(names) {
			return false
		}
		_, tails := splitListTails("x" + m[4])
		each := 0.5
		var rules []*types.Requirement
		for _, tail := range tails {
			if e := spreadEachRe.FindStringSubmatch(tail); e != nil {
				if countOf(e[2]) != k {
					return false
				}
				each, _ = strconv.ParseFloat(e[1], 64)
				continue
			}
			rs, _, ok := listRefRule(normalizeListRef(orAbove(tail), ls), ls, nil)
			if !ok {
				return false
			}
			for _, r := range rs {
				r.Raw = RawHead(row.r)
			}
			rules = append(rules, rs...)
		}
		if a.inlineSpread(row, put, amount, k, each, names, rules, ls) {
			return true
		}
		return a.spread(row, put, amount, k, each, names, nil, rules, ls)
	}
	if m := kOfGroupsRe.FindStringSubmatch(t); m != nil {
		names := a.leadLists(row, ls)
		if names == nil {
			names = a.leadListsOf(row, ls, true)
		}
		each, _ := strconv.ParseFloat(m[1], 64)
		k := countOf(m[2])
		if len(names) != countOf(m[3]) || k >= len(names) {
			return false
		}
		n, ok := spreadNode(k, each, names, nil, ls)
		if !ok {
			return false
		}
		put(n)
		return true
	}
	if m := eachGroupLeadRe.FindStringSubmatch(t); m != nil {
		// the pool of the item's own lists, with the floor on each under it
		names := a.ownLists(row, ls)
		amount, _ := strconv.ParseFloat(m[1], 64)
		each, _ := strconv.ParseFloat(m[2], 64)
		if len(names) < 2 || each*float64(len(names)) > amount+1e-9 {
			return false
		}
		return a.spread(row, put, amount, len(names), each, names, nil, nil, ls)
	}
	if m := followingCoursesRe.FindStringSubmatch(notAlreadyRe.ReplaceAllString(t, "")); m != nil {
		// the pool of the titled lists under the lead
		names := a.leadLists(row, ls)
		if names == nil && m[2] != "" {
			names = a.listsBelow(row, ls)
		}
		amount, _ := strconv.ParseFloat(m[1], 64)
		if len(names) < 2 {
			return false
		}
		pool := &types.ReqNode{Type: types.NodeCredits, Amount: amount, Lists: names}
		// students may apply for other courses: the lists aren't all it counts
		for _, note := range a.notes {
			if row.r.Label != "" && applyUnderRe.MatchString(note) && strings.HasSuffix(strings.ToLower(note), "under requirement "+strings.ToLower(row.r.Label)) {
				pool = programs.OrNode(pool, TextNode(types.TextPermission, note))
				addRaw(row, note)
				a.notes = slices.DeleteFunc(a.notes, func(x string) bool { return x == note })
				break
			}
		}
		put(pool)
		return true
	}
	if m := rangeFromListsRe.FindStringSubmatch(strings.TrimSuffix(t, ":")); m != nil {
		// the pool of the lists, at the range's lower end
		names := ls.resolve(listRefNames(m[3]))
		lo, _ := strconv.ParseFloat(m[1], 64)
		hi, _ := strconv.ParseFloat(m[2], 64)
		if names == nil || hi < lo {
			return false
		}
		put(&types.ReqNode{Type: types.NodeCredits, Amount: lo, Lists: names})
		return true
	}
	if m := coreOrListRe.FindStringSubmatch(strings.TrimSuffix(t, ":")); m != nil {
		// the Core Group is the courses of the item "from the Core Group:"
		names := ls.resolve(listRefNames(m[2]))
		var core []string
		for _, r := range a.items {
			n := r.r.Req
			if r != row && n != nil && n.Type == types.NodeCredits && n.Courses != nil && n.Designators == nil && n.Lists == nil && n.Levels == nil &&
				strings.Contains(strings.ToLower(r.r.Raw), "from the core group:") {
				if core != nil {
					return false
				}
				core = slices.Clone(n.Courses)
			}
		}
		if core == nil || names == nil {
			return false
		}
		items := []*types.ReqNode{{Type: types.NodeCredits, Courses: core}}
		for _, name := range names {
			items = append(items, &types.ReqNode{Type: types.NodeCredits, Lists: []string{name}})
		}
		name, _, ok := a.addUnion(row, items, ls)
		if !ok {
			return false
		}
		amount, _ := strconv.ParseFloat(m[1], 64)
		put(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Lists: []string{name}})
		return true
	}
	if m := desigsOrListBelowRe.FindStringSubmatch(t); m != nil {
		// the lower amount from a union of the designators and the list below
		names := a.listsBelow(row, ls)
		if len(names) != 1 {
			return false
		}
		name, _, ok := a.addUnion(row, []*types.ReqNode{{Type: types.NodeCredits, Designators: grammar.DesigRe.FindAllString(m[2], -1)}, {Type: types.NodeCredits, Lists: names}}, ls)
		if !ok {
			return false
		}
		amount, _ := strconv.ParseFloat(m[1], 64)
		put(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Lists: []string{name}})
		return true
	}
	if m := fromGroupsEachRe.FindStringSubmatch(t); m != nil {
		names := ls.resolve(listRefNames(m[2]))
		amount, _ := strconv.ParseFloat(m[1], 64)
		each, err := strconv.ParseFloat(m[3], 64)
		if err != nil {
			each = float64(countOf(m[3]))
		}
		if len(names) < 2 || each <= 0 || each*float64(len(names)) > amount+1e-9 {
			return false
		}
		return a.spread(row, put, amount, len(names), each, names, nil, nil, ls)
	}
	if m := eachOfGroupsRe.FindStringSubmatch(t); m != nil {
		// the amount from each list, a course toward one only
		names := ls.resolve(listRefNames(m[2]))
		each, _ := strconv.ParseFloat(m[1], 64)
		if len(names) < 2 {
			return false
		}
		n, ok := spreadNode(len(names), each, names, nil, ls)
		if !ok {
			return false
		}
		put(n)
		return true
	}
	if m := eachListRe.FindStringSubmatch(t); m != nil {
		names := a.leadLists(row, ls)
		each, _ := strconv.ParseFloat(m[2], 64)
		if len(names) != countOf(m[3]) {
			return false
		}
		if total, _ := strconv.ParseFloat(m[1], 64); m[1] != "" && math.Abs(total-each*float64(len(names))) > 1e-9 {
			return false
		}
		var parts []*types.ReqNode
		for _, name := range names {
			parts = append(parts, &types.ReqNode{Type: types.NodeCredits, Amount: each, Lists: []string{name}})
		}
		n, ok := partsNode(len(parts), parts, ls)
		if !ok {
			return false
		}
		put(n)
		return true
	}
	if m := oneOrTwoRe.FindStringSubmatch(t); m != nil {
		// 1.0 in half courses is all from one list or 0.5 from two: the pool
		names := a.leadLists(row, ls)
		one, _ := strconv.ParseFloat(m[1], 64)
		half, _ := strconv.ParseFloat(m[2], 64)
		if len(names) < 2 || half != 0.5 || countOf(m[3]) != 2 || one != 1 {
			return false
		}
		put(&types.ReqNode{Type: types.NodeCredits, Amount: one, Lists: names})
		return true
	}
	// a spread clause in the text, or spread rules under the item
	clause := ""
	if loc := spreadInRe.FindStringIndex(t); loc != nil {
		clause = strings.Trim(t[loc[0]:loc[1]], " ,;()")
		t = strings.TrimSpace(t[:loc[0]] + t[loc[1]:])
	}
	head, tails := splitListTails(t)
	under := followingRequirementsRe.MatchString(t) && slices.ContainsFunc(row.children, func(c *compRow) bool {
		return c.r.Kind == types.RequirementConstraint && c.r.Req.Type == types.NodeText && spreadTailRe.MatchString(Clean(c.r.Req.Text))
	})
	if clause == "" && !under {
		return false
	}
	var body *types.ReqNode
	var rules []*types.Requirement
	if under {
		b, rs, notes, ok := a.listRefAlloc(row, followingRequirementsRe.ReplaceAllString(t, ""), ls)
		if !ok || b.Type != types.NodeCredits || b.Lists == nil {
			return false
		}
		body, rules = b, rs
		row.r.Notes = append(row.r.Notes, notes...)
	} else {
		b, notes, ok := listRefPool(normalizeListRef(head, ls), ls, false)
		if !ok || b.Lists == nil {
			return false
		}
		body = b
		row.r.Notes = append(row.r.Notes, notes...)
	}
	var spread []*types.Requirement
	if clause != "" {
		r, ok := spreadTail(clause, body, ls)
		if !ok {
			return false
		}
		r.Raw = RawHead(row.r)
		spread = append(spread, r)
	}
	for _, tail := range tails {
		rs, _, ok := listRefRule(normalizeListRef(orAbove(tail), ls), ls, nil)
		if !ok {
			return false
		}
		for _, r := range rs {
			r.Raw = RawHead(row.r)
		}
		rules = append(rules, rs...)
	}
	for _, c := range row.children {
		if c.r.Kind != types.RequirementConstraint || c.r.Req.Type != types.NodeText {
			continue
		}
		if r, ok := spreadTail(Clean(c.r.Req.Text), body, ls); ok {
			c.r.Req = r.Req
		}
	}
	put(body)
	for _, r := range append(rules, spread...) {
		a.rowUnder(row, r)
	}
	return true
}

// subLabelRe matches a sub-list's name taken from its marker alone, not
// the text: "2a".
var subLabelRe = regexp.MustCompile(`^\d{0,2}[a-z]$`)

// inlineSpread reads a spread over the lead's own unnamed sub-lists ("a.
// CSC401H1, CSC485H1"): the pool of all their courses, with caps on its "/"
// pairs, and the spread over their courses. It reports whether it read it.
func (a *Assembler) inlineSpread(row *compRow, put func(*types.ReqNode), amount float64, k int, each float64, names []string, rules []*types.Requirement, ls compListRefs) bool {
	var parts []*types.ReqNode
	var members, notes []string
	for _, name := range names {
		l := ls.byName(name)
		if !subLabelRe.MatchString(name) || len(l.defs) != 1 || l.defs[0].row != row {
			return false
		}
		codes, filters, ok := ls.members(name)
		if !ok || filters != nil || len(l.list.Items) != 1 {
			return false
		}
		parts = append(parts, &types.ReqNode{Type: types.NodeCredits, Amount: each, Courses: codes})
		members = append(members, l.defs[0].members)
		notes = append(notes, l.defs[0].note)
	}
	n, _ := parseCompletionText(strconv.FormatFloat(amount, 'f', 1, 64) + " credits from: " + strings.Join(members, ", "))
	pool, caps := poolCaps(n)
	if !isPool(pool) || pool.Designators != nil || pool.Levels != nil || pool.Amount != amount {
		return false
	}
	spread, ok := partsNode(k, parts, ls)
	if !ok {
		return false
	}
	put(pool)
	for _, c := range caps {
		a.rowUnder(row, &types.Requirement{Kind: types.RequirementLimit, Req: c, Raw: RawHead(row.r)})
	}
	for _, r := range rules {
		a.rowUnder(row, r)
	}
	a.rowUnder(row, &types.Requirement{Kind: types.RequirementConstraint, Req: spread, Raw: RawHead(row.r)})
	row.r.Notes = slices.DeleteFunc(row.r.Notes, func(n string) bool { return slices.Contains(notes, n) })
	return true
}

// spreadTail reads a spreadTailRe clause over pool's lists into a
// constraint, or ok false.
func spreadTail(clause string, pool *types.ReqNode, ls compListRefs) (*types.Requirement, bool) {
	m := spreadTailRe.FindStringSubmatch(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(clause), ".")))
	if m == nil || countOf(m[3]) != len(pool.Lists) {
		return nil, false
	}
	each := 1.0
	if !strings.HasPrefix(strings.ToLower(m[1]), "one") {
		each, _ = strconv.ParseFloat(strings.Fields(m[1])[0], 64)
	}
	k := countOf(m[2])
	if total, _ := strconv.ParseFloat(m[4], 64); m[4] != "" && math.Abs(total-each*float64(k)) > 1e-9 {
		return nil, false
	}
	n, ok := spreadNode(k, each, pool.Lists, nil, ls)
	if !ok {
		return nil, false
	}
	return &types.Requirement{Kind: types.RequirementConstraint, Req: n, Raw: grammar.CleanNote(clause)}, true
}

// spread puts on row the pool of names (with the levels, if any) and the
// spread over them, with rules, under it.
func (a *Assembler) spread(row *compRow, put func(*types.ReqNode), amount float64, k int, each float64, names []string, levels []int, rules []*types.Requirement, ls compListRefs) bool {
	n, ok := spreadNode(k, each, names, levels, ls)
	if !ok {
		return false
	}
	put(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Lists: slices.Clone(names), Levels: slices.Clone(levels)})
	for _, r := range rules {
		a.rowUnder(row, r)
	}
	a.rowUnder(row, &types.Requirement{Kind: types.RequirementConstraint, Req: n, Raw: RawHead(row.r)})
	return true
}

var (
	// sharesLeadRe matches an amount made of shares set out under it:
	// "4. 1.0 credits including:".
	sharesLeadRe = regexp.MustCompile(`(?i)^(?:\d{1,2}\.\s+)?` + grammar.AmountPat + `\s+credits?\s+including\s*:$`)
	// shareRe matches one share: "Up to 0.5 credits from CSC301H1, CSC302H1
	// (if not taken in list 3)".
	shareRe = regexp.MustCompile(`(?i)^(?:[a-z][.)]\s+)?up\s+to\s+` + grammar.AmountPat + `\s+credits?\s+from\s+(.+?)(?:\s*\(\s*(if\s+not\s+taken\s+in\s+(?:list|requirement)\s+\d+)\s*\))?$`)
)

// shares reads an amount made of shares, which are text constraints under it:
// the pool of all their courses and a limit per share. "(if not taken in list
// 3)" is a note: the rows' exclusive claims already say it.
func (a *Assembler) shares(row *compRow) bool {
	m := sharesLeadRe.FindStringSubmatch(strings.TrimSpace(Clean(RawHead(row.r))))
	if m == nil || len(row.children) < 2 {
		return false
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	var courses, notes []string
	var limits []*compRow
	room := 0.0
	for _, c := range row.children {
		if c.r.Kind != types.RequirementConstraint || c.r.Req.Type != types.NodeText {
			return false
		}
		s := shareRe.FindStringSubmatch(strings.TrimSuffix(Clean(c.r.Req.Text), "."))
		if s == nil {
			return false
		}
		v, _ := strconv.ParseFloat(s[1], 64)
		n, _ := parseCompletionText(s[1] + " credits from " + s[2])
		if !isPool(n) || n.Courses == nil || n.Designators != nil || n.Levels != nil {
			return false
		}
		courses = append(courses, n.Courses...)
		room += v
		if s[3] != "" {
			notes = append(notes, grammar.CleanNote(s[3]))
		}
		limits = append(limits, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Req: &types.ReqNode{Type: types.NodeCredits, Courses: n.Courses, MaxAmount: grammar.MaxAmount(v)}, Raw: c.r.Raw}})
	}
	if room < amount-1e-9 {
		return false
	}
	row.r.Req = &types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: grammar.Dedupe(courses)}
	row.r.Notes = append(row.r.Notes, notes...)
	row.children = limits
	return true
}

// minimumTitledRe matches an amount of a list named by its title, and the
// floors after it: "A minimum of 3.0 Society-Culture credits".
var minimumTitledRe = regexp.MustCompile(`^(?:\d{1,2}\.\s+)?(?:(?i:a\s+minimum\s+of|` + grammar.AtLeastPat + `)\s+)?` + grammar.AmountPat + `\s+([A-Z][\w-]*(?:\s+[A-Z][\w-]*)*)\s+(?i:credits?)\b(.*?)[;.]?$`)

// oneOfWhichLevelRe matches a floor on one of an amount's credits: ", one
// of which must be 300/400-level".
var oneOfWhichLevelRe = regexp.MustCompile(`(?i)^\s*,\s*one\s+of\s+which\s+must\s+be\s+(\d00)\s*/\s*\d00-level$`)

// titledMinimum reads an item on a list named by its title, which the grammar
// split, as listRefAlloc reads "3.0 credits from Society-Culture courses". A
// list of the same courses offered elsewhere may count too: an ambiguity.
func (a *Assembler) titledMinimum(row *compRow, ls compListRefs) bool {
	m := minimumTitledRe.FindStringSubmatch(strings.TrimSpace(Clean(RawHead(row.r))))
	if m == nil {
		return false
	}
	l := ls[titleKey(m[2]+" courses")]
	if l == nil || l.bad {
		return false
	}
	tail := orAbove(m[3])
	one := oneOfWhichLevelRe.FindStringSubmatch(tail)
	if one != nil {
		// "one of which" is 1.0 credit; one course (0.5) is the lenient reading
		tail = ", including 1.0 credit at the " + one[1] + "+ level"
	}
	body, rules, notes, ok := a.listRefAlloc(row, m[1]+" credits from "+l.list.Name+tail, ls)
	if !ok || body.Type != types.NodeCredits || !slices.Equal(body.Lists, []string{l.list.Name}) {
		return false
	}
	if one != nil {
		if len(rules) != 1 || rules[0].Kind != types.RequirementConstraint || rules[0].Req.Type != types.NodeCredits || rules[0].Req.Amount != 1 {
			return false
		}
		half := grammar.CloneNode(rules[0].Req)
		half.Amount = 0.5
		rules[0].Req = programs.OrNode(rules[0].Req, programs.AndNode(half, TextNode(types.TextAmbiguity, `"one of which" may mean one course (0.5 credit)`)))
	}
	for _, o := range a.sortedLists(ls) {
		if o != l && !o.bad && strings.HasPrefix(strings.ToLower(o.list.Name), strings.ToLower(l.list.Name)+" on ") {
			wide := grammar.CloneNode(body)
			wide.Lists = append(wide.Lists, o.list.Name)
			body = programs.OrNode(body, programs.AndNode(wide, TextNode(types.TextAmbiguity, `"`+o.list.Name+`" may count as `+m[2]+" credits")))
		}
	}
	head := RawHead(row.r)
	row.r.Req = body
	row.r.Notes = append(row.r.Notes, notes...)
	row.children = slices.DeleteFunc(row.children, func(c *compRow) bool {
		return c.r.Kind == types.RequirementConstraint && RawHead(c.r) == head && len(c.children) == 0
	})
	for _, r := range rules {
		r.Raw = head // the source line, not the sentence listRefAlloc read
		a.rowUnder(row, r)
	}
	return true
}

var (
	// titledBelowRe matches an item on a list named by its title, the title
	// first: "Temporal Requirement: 1.0 credit in pre-1800 history course(s)
	// listed below".
	titledBelowRe = regexp.MustCompile(`^([A-Z][\w ]+?):\s+` + grammar.AmountPat + `\s+(?i:credits?\s+in\s+[\w\s-]+?\s+course(?:\(s\)|s)?\s+listed\s+below)$`)
	// starredTopicsRe matches a list's footnote marking topics courses:
	// "***** Some “Topics in History”: Various topics each year".
	starredTopicsRe = regexp.MustCompile(`^(\*{2,})\s*(Some\s+.*\bTopics\s+in\b.*)$`)
	// cantCountRe matches "It cannot be counted towards a History program",
	// after a sentence naming the course.
	cantCountRe = regexp.MustCompile(`(?i)^it\s+cannot\s+be\s+counted\s+towards?\s+an?\s+\w+\s+program\b`)
)

// spreadFloors reads the header's text floors with spreadRule, since
// namedLists doesn't walk them. A numbered item read as a program rule then
// takes its number and its place among the items.
func (a *Assembler) spreadFloors(ls compListRefs) {
	for _, r := range a.floors {
		a.spreadRule(r, ls)
	}
	a.program = slices.DeleteFunc(a.program, func(r *compRow) bool {
		if r.r.Kind != types.RequirementConstraint || r.r.Label != "" || len(r.children) > 0 {
			return false
		}
		head := rawWords(RawHead(r.r))
		var num, src string
		for _, l := range a.lines {
			if m := itemNumRe.FindStringSubmatch(l); m != nil && rawWords(m[2]) == head {
				num, src = m[1], strings.TrimSpace(l)
			}
		}
		next, _ := strconv.Atoi(num)
		i := slices.IndexFunc(a.items, func(x *compRow) bool { return x.r.Label == strconv.Itoa(next+1) })
		if num == "" || i < 0 || slices.ContainsFunc(a.items, func(x *compRow) bool { return x.r.Label == num }) {
			return false
		}
		r.r.Label = num
		if _, rest, ok := strings.Cut(r.r.Raw, "\n"); ok {
			src += "\n" + rest
		}
		r.r.Raw = src // the numbered line it was read from
		a.items = slices.Insert(a.items, i, r)
		return true
	})
}

// itemNumRe matches a numbered line: "6. At least 2.0 credits".
var itemNumRe = regexp.MustCompile(`^\s*(\d{1,2})\.\s+(.+)$`)

// spreadRule reads program and item rules of text that need the lists or rows:
// an overlap sentence naming a group, a course that cannot count towards the
// program (a limit of 0), and a spread stated with the total.
func (a *Assembler) spreadRule(row *compRow, ls compListRefs) ([]*compRow, bool) {
	n := row.r.Req
	if n != nil && n.Type == types.NodeAnd && len(n.Items) == 2 && n.Items[0].Type == types.NodeCredits && n.Items[1].Type == types.NodeText {
		n = n.Items[1] // and(amount, text): the header's spread clause is the text
	}
	if n == nil || n.Type != types.NodeText || n.Kind != types.TextOther {
		return nil, false
	}
	t := strings.TrimSuffix(Clean(n.Text), ".")
	if row.r.Req.Type == types.NodeText && a.role(t) == roleOverlap && a.overlap(t) {
		// an overlap sentence naming items, read now that the items exist
		return nil, true
	}
	if row.r.Req.Type == types.NodeText && cantCountRe.MatchString(t) {
		for _, l := range a.lines {
			if i := strings.Index(rawWords(l), rawWords(t)); i > 0 {
				if codes := grammar.Dedupe(grammar.CodeRe.FindAllString(l[:strings.Index(l, "It ")], -1)); len(codes) == 1 {
					row.r.Kind = types.RequirementLimit
					row.r.Req = &types.ReqNode{Type: types.NodeCredits, Courses: codes, MaxAmount: grammar.MaxAmount(0)}
					row.r.Raw = grammar.CleanNote(l)
					return []*compRow{row}, true
				}
			}
		}
	}
	if m := studentFloorRe.FindStringSubmatch(t); m != nil && row.r.Req.Type == types.NodeText {
		// a floor, and one within it as a sibling: a constraint has no children
		v, _ := strconv.ParseFloat(m[1], 64)
		f := &types.ReqNode{Type: types.NodeCredits, Amount: v, Levels: grammar.ParseLevels(m[3] + "+ level")}
		if m[2] != "" {
			f.Designators = []string{m[2]}
		}
		row.r.Req = f
		out := []*compRow{row}
		if m[4] != "" {
			w, _ := strconv.ParseFloat(m[4], 64)
			inner := narrowFilter(&types.ReqNode{Type: types.NodeCredits, Amount: w, Levels: grammar.ParseLevels(m[5] + "-level")}, f)
			if inner == nil {
				return nil, false
			}
			out = append(out, &compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Req: inner, Raw: row.r.Raw}})
		}
		return out, true
	}
	if m := clusterSpreadRe.FindStringSubmatch(t); m != nil {
		var names []string
		for _, l := range a.sortedLists(ls) {
			if strings.HasPrefix(l.key, "cluster ") && l.parent == "" {
				names = append(names, l.list.Name)
			}
		}
		if names == nil || m[4] != "" && len(categoryLetterRe.FindAllString(m[4], -1)) != len(names) {
			return nil, false
		}
		amount, _ := strconv.ParseFloat(m[1], 64)
		k := countOf(m[3])
		levels := grammar.ParseLevels(m[2] + "+ level")
		node, ok := spreadNode(k, 0.5, names, levels, ls)
		if !ok || amount < 0.5*float64(k) {
			return nil, false
		}
		if amount > 0.5*float64(k) {
			node = programs.AndNode(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Lists: names, Levels: levels}, node)
		}
		row.r.Req = node
		return []*compRow{row}, true
	}
	return nil, false
}

var (
	// studentFloorRe matches a floor on the student's courses: "At least 2.0
	// credits of the student’s 7.0 credits must be at the 300-level or above".
	studentFloorRe = regexp.MustCompile(`(?i)^` + grammar.AtLeastPat + `\s+` + grammar.AmountPat + `\s+credits?\s+of\s+the\s+student[’']s\s+(?:` + grammar.NumPat + `\s+credits|program)\s+must\s+be\s+(?:in\s+[^()]+?\(\s*(` + grammar.DesigPat + `)\s+prefix\s+courses\s*\),?\s+)?at\s+the\s+(` + grammar.LevelPat + `)-level\s+or\s+above(?:,?\s+with\s+` + grammar.AtLeastPat + `\s+` + grammar.AmountPat + `\s+credits?\s+of\s+these\s+at\s+the\s+(` + grammar.LevelPat + `)-level)?$`)
	// clusterSpreadRe matches a spread over the clusters, with the total:
	// "including 1.0 credit at the 300+ level in at least two clusters".
	clusterSpreadRe = regexp.MustCompile(`(?i)^including\s+` + grammar.AmountPat + `\s+credits?\s+at\s+the\s+(` + grammar.LevelPat + `)\+\s+level\s+in\s+` + grammar.AtLeastPat + `\s+(two|three|four|\d)\s+[\w\s/]*?clusters\b,?\s*(?:categorized\s+as\s+follows:\s*(.*))?$`)
	// categoryLetterRe matches a category's letter: "b)" in "a) Politics
	// b) Society".
	categoryLetterRe = regexp.MustCompile(`(?:^|\s)[a-z]\)\s`)
)

// titledBelow reads a titledBelowRe item as credits from the list it
// names, or nil.
func titledBelow(t string, ls compListRefs) *types.ReqNode {
	m := titledBelowRe.FindStringSubmatch(strings.TrimSuffix(t, "."))
	if m == nil {
		return nil
	}
	l := ls[titleKey(m[1])]
	if l == nil || l.bad {
		return nil
	}
	v, _ := strconv.ParseFloat(m[2], 64)
	return &types.ReqNode{Type: types.NodeCredits, Amount: v, Lists: []string{l.list.Name}}
}

// starredTopics makes the members of a used list marked with a footnote's
// stars ("HIS389H1*****") maybe items, since only some offerings count. It
// returns the footnote's notes when a list took them.
func (a *Assembler) starredTopics(used []*compList) []string {
	var dropped []string
	for _, line := range a.lines {
		m := starredTopicsRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		starred := regexp.MustCompile(`(` + grammar.CodePat + `)` + regexp.QuoteMeta(m[1]) + `(?:[^*]|$)`)
		first := strings.TrimSpace(SplitSentences(m[2])[0])
		took := false
		for _, l := range used {
			var codes []string
			for _, d := range l.defs {
				if d.block == nil {
					continue
				}
				// the raw line: the block's text has its stars collapsed
				for _, s := range starred.FindAllStringSubmatch(a.lines[d.block.line], -1) {
					codes = append(codes, s[1])
				}
			}
			if codes = grammar.Dedupe(codes); codes == nil {
				continue
			}
			var items []*types.ReqNode
			for _, it := range l.list.Items {
				if it.Type == types.NodeCredits && it.Designators == nil && it.Weight == 0 && it.Courses != nil {
					it.Courses = slices.DeleteFunc(slices.Clone(it.Courses), func(c string) bool { return slices.Contains(codes, c) })
					if len(it.Courses) == 0 {
						continue
					}
				}
				items = append(items, it)
			}
			l.list.Items = append(items, programs.AndNode(&types.ReqNode{Type: types.NodeCredits, Courses: codes}, TextNode(types.TextPermission, grammar.CleanNote(first))))
			took = true
		}
		if !took {
			continue
		}
		for _, n := range a.notes {
			if t := strings.TrimSpace(strings.TrimLeft(n, "* ")); t != "" && strings.Contains(rawWords(first), rawWords(t)) {
				dropped = append(dropped, n)
			}
		}
	}
	return dropped
}

// ownLists returns the names of the lists row's own lines define, in order,
// or nil when one of them is unsound.
func (a *Assembler) ownLists(row *compRow, ls compListRefs) []string {
	var names []string
	for _, l := range a.sortedLists(ls) {
		if l.defs[0].row == row {
			if l.bad {
				return nil
			}
			names = append(names, l.list.Name)
		}
	}
	return names
}

// courseGroupsRe matches the heading over the lists of a unit's courses:
// "**Course Groups**".
var courseGroupsRe = regexp.MustCompile(`(?i)^\W*course\s+groups\W*$`)

// remainingFromLists reads remainingFrom's item when the lists below it, under
// a "Course Groups" heading, are the unit's courses: the pool of a union of
// the item's codes and the lists, or the unit's courses as content text.
func (a *Assembler) remainingFromLists(row *compRow, ls compListRefs) bool {
	r, m, amount := a.remainingFromItem()
	if r != row {
		return false
	}
	names := a.listsBelow(row, ls)
	if len(names) < 2 {
		return false
	}
	first := slices.IndexFunc(a.lines, func(l string) bool { return rawWords(l) == rawWords(RawHead(row.r)) })
	heads := 0
	for _, l := range a.lines[first+1:] {
		if courseGroupsRe.MatchString(l) {
			heads++
		}
	}
	if heads != 1 {
		return false
	}
	// every list is under the heading: none is defined before it
	at := first + 1 + slices.IndexFunc(a.lines[first+1:], func(l string) bool { return courseGroupsRe.MatchString(l) })
	for _, name := range names {
		if l := ls.byName(name); l == nil || len(l.defs) != 1 || l.defs[0].block == nil || l.defs[0].block.line < at {
			return false
		}
	}
	codes := grammar.Dedupe(grammar.CodeRe.FindAllString(m[1], -1))
	name, _, ok := a.addUnion(row, []*types.ReqNode{{Type: types.NodeCredits, Courses: codes}, {Type: types.NodeCredits, Lists: names}}, ls)
	if !ok {
		return false
	}
	row.r.Req = programs.OrNode(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Lists: []string{name}},
		programs.AndNode(&types.ReqNode{Type: types.NodeCredits, Amount: amount}, TextNode(types.TextContent, m[2])))
	return true
}

// listsBelow returns the names of the program's top-level lists defined
// after row's line, or nil when there are none or one can't be used.
func (a *Assembler) listsBelow(row *compRow, ls compListRefs) []string {
	line := slices.IndexFunc(a.lines, func(l string) bool { return rawWords(l) == rawWords(RawHead(row.r)) })
	if line < 0 {
		return nil
	}
	var names []string
	for _, l := range a.sortedLists(ls) {
		first := l.defs[0]
		if l.parent != "" || first.block == nil || first.row != nil && first.row != row || first.block.line < line {
			continue
		}
		if l.bad {
			return nil
		}
		names = append(names, l.list.Name)
	}
	return names
}
