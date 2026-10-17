package reqs

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Spreads and choices among lists: an amount spread over named lists, a choice
// of lists, each of several lists, shares that add up, and breadth splits.

var complCountWords = map[string]int{"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8}

// countOf reads a count written as a word or digits; 0 when it is neither.
func countOf(s string) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return complCountWords[strings.ToLower(strings.TrimSpace(s))]
}

// byName returns the list named name, or nil.
func (ls compListRefs) byName(name string) *compList {
	for _, l := range ls {
		if l.list.Name == name {
			return l
		}
	}
	return nil
}

// members returns the codes on the named list and the lists under it, and its
// filter members; ok is false when a list is missing or bad, or the lists loop.
func (ls compListRefs) members(name string) (codes []string, filters []*types.ReqNode, ok bool) {
	return ls.membersOf(name, map[string]bool{})
}

// membersOf is members, with the lists already on the path in seen.
func (ls compListRefs) membersOf(name string, seen map[string]bool) (codes []string, filters []*types.ReqNode, ok bool) {
	l := ls.byName(name)
	if l == nil || l.bad || seen[name] {
		return nil, nil, false
	}
	seen[name] = true
	defer delete(seen, name)
	good := true
	for _, it := range l.list.Items {
		walkReq(it, func(n *types.ReqNode) {
			if n.Type != types.NodeCredits {
				return
			}
			codes = append(codes, n.Courses...)
			if n.Designators != nil {
				filters = append(filters, n)
			}
			for _, sub := range n.Lists {
				c, f, sok := ls.membersOf(sub, seen)
				good = good && sok
				codes, filters = append(codes, c...), append(filters, f...)
			}
		})
	}
	return dedupe(codes), filters, good
}

// sharedCourses returns, for each part (credits from one list, or listed
// courses), the courses another part holds too; ok is false when two parts'
// filters share a designator, so the shared courses can't be listed.
func (ls compListRefs) sharedCourses(parts []*types.ReqNode) (shared [][]string, ok bool) {
	codes := make([][]string, len(parts))
	filters := make([][]*types.ReqNode, len(parts))
	for i, p := range parts {
		if p.Lists == nil {
			codes[i] = p.Courses
			continue
		}
		c, f, ok := ls.members(p.Lists[0])
		if !ok {
			return nil, false
		}
		codes[i], filters[i] = c, f
	}
	holds := func(i int, code string) bool {
		return slices.Contains(codes[i], code) || slices.ContainsFunc(filters[i], func(f *types.ReqNode) bool { return filterHolds(f, code) })
	}
	shared = make([][]string, len(parts))
	for i := range parts {
		for j := range parts {
			if i == j {
				continue
			}
			for _, f := range filters[i] {
				for _, g := range filters[j] {
					if slices.ContainsFunc(f.Designators, func(d string) bool { return slices.Contains(g.Designators, d) }) {
						return nil, false // "any URB course" on two lists: not a list of codes
					}
				}
			}
			for _, c := range codes[j] {
				if holds(i, c) && !slices.Contains(shared[i], c) {
					shared[i] = append(shared[i], c)
				}
			}
		}
	}
	return shared, true
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
			it := cloneNode(p)
			if strict && shared[i] != nil {
				if it.Lists == nil {
					it.Courses = slices.DeleteFunc(slices.Clone(it.Courses), func(c string) bool { return slices.Contains(shared[i], c) })
				} else {
					it.Exclude = dedupe(append(slices.Clone(it.Exclude), shared[i]...))
				}
			}
			items = append(items, it)
		}
		if k == len(items) {
			return andNode(items...)
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
	if all = dedupe(all); all == nil {
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
	return orNode(build(true), andNode(build(false), textNode(types.TextAmbiguity, why))), true
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
func (a *compAssembler) rowLines(rows []*compRow, out map[*compRow]int) {
	for _, r := range rows {
		head := rawWords(rawHead(r.r))
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
func (a *compAssembler) leadLists(row *compRow, ls compListRefs) []string {
	return a.leadListsOf(row, ls, false)
}

// leadListsOf is leadLists; with last, lists after every row's line count
// too, for a lead that says how many lists it means.
func (a *compAssembler) leadListsOf(row *compRow, ls compListRefs, last bool) []string {
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
func (a *compAssembler) spreadRow(row *compRow, ls compListRefs) ([]*compRow, bool) {
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
	// complRangeListsPartRe matches a range part of an item over lists: "2.0 to
	// 3.0 credits from Group A and 1.0 credit from Group B/Group C".
	complRangeListsPartRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+to\s+\d+(?:\.\d+)?\s+credits?\s+from\s+(` + complListRefPat + `)\s+and\s+(\d+(?:\.\d+)?)\s+credits?\s+from\s+(` + complListRefPat + `)$`)
	// complRangeLanguagesPartRe matches a range in named languages: "1.0 to 2.0
	// credits in Ancient Stream languages: Akkadian, Ancient Egyptian".
	complRangeLanguagesPartRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+to\s+\d+(?:\.\d+)?\s+credits?\s+in\s+[\w ]+?\s+languages\s*:\s*\S.*$`)
	// complRangeDesigPartRe matches a range in a designator's courses: "1.0 to
	// 2.0 credits in language (NML courses)".
	complRangeDesigPartRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+to\s+\d+(?:\.\d+)?\s+credits?\s+in\s+[a-z]+\s+\(\s*((?-i:[A-Z]{3}))\s+courses\s*\)$`)
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
		t := strings.TrimSuffix(complClean(it.Text), ".")
		if m := complRangeDesigPartRe.FindStringSubmatch(t); m != nil {
			v, _ := strconv.ParseFloat(m[1], 64)
			n.Items[i] = &types.ReqNode{Type: types.NodeCredits, Amount: v, Designators: []string{m[2]}}
			read = true
			continue
		}
		if m := complRangeLanguagesPartRe.FindStringSubmatch(t); m != nil {
			// no node can say which courses are in those languages
			v, _ := strconv.ParseFloat(m[1], 64)
			n.Items[i] = andNode(&types.ReqNode{Type: types.NodeCredits, Amount: v}, textNode(types.TextContent, it.Text))
			read = true
			continue
		}
		if m := complRangeListsPartRe.FindStringSubmatch(t); m != nil {
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
			for _, c := range dedupe(a) {
				if slices.Contains(b, c) {
					shared = append(shared, c)
				}
			}
			n.Items[i] = andNode(parts...)
			if shared != nil {
				strict := andNode(cloneNode(parts[0]), cloneNode(parts[1]))
				for _, p := range strict.Items {
					p.Exclude = slices.Clone(shared)
				}
				n.Items[i] = orNode(strict, andNode(parts[0], parts[1], textNode(types.TextAmbiguity, strings.Join(shared, ", ")+" counts toward one of the parts only")))
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
		return complClean(n.Text), func(b *types.ReqNode) { row.r.Req = b }, true
	}
	if n.Type == types.NodeOr && len(n.Items) > 1 && n.Items[0].Type == types.NodeText && n.Items[0].Kind == types.TextOther &&
		!slices.ContainsFunc(n.Items[1:], func(it *types.ReqNode) bool { return it.Type != types.NodeText || it.Kind != types.TextPermission }) {
		return complClean(n.Items[0].Text), func(b *types.ReqNode) {
			if b.Type == types.NodeOr {
				n.Items = append(slices.Clone(b.Items), n.Items[1:]...) // one or, not or(or(...), route)
				return
			}
			n.Items[0] = b
		}, true
	}
	return "", nil, false
}

// complSeeBelowPat matches an optional "(see below for definitions)".
const complSeeBelowPat = `(?:\s*\(\s*see\s+below(?:\s+for\s+definitions)?\s*\))?`

var (
	// complSpreadLeadRe matches a pool over lists with how many it must draw on:
	// "1.5 credits, chosen from at least two of the four clusters below".
	complSpreadLeadRe = regexp.MustCompile(`(?i)^(?:at\s+least\s+)?(\d+(?:\.\d+)?)\s+credits?,?\s+(?:(?:chosen|selected|taken)\s+)?(?:from|in)\s+(?:the\s+following,?\s+so\s+that\s+(?:the\s+)?courses\s+are\s+from\s+)?(?:at\s+least\s+)?(one|two|three|four|five|\d)\s+of\s+the\s+(?:(two|three|four|five|six|seven|eight|\d)\s+)?(?:following\s+)?(?:clusters|groups|areas|lists|categories|concentrations)(?:\s+of\s+concentration)?(?:\s+below)?(.*)$`)
	// complSpreadTailRe matches a spread after or under the pool: "including one
	// or more credits from at least three of the four groups".
	complSpreadTailRe = regexp.MustCompile(`(?i)^(?:including\s+)?(?:at\s+least\s+)?(one\s+or\s+more\s+credits?|\d+(?:\.\d+)?\s+credits?)\s+(?:from|in)\s+(?:at\s+least\s+)?(one|two|three|four|five|\d)\s+of\s+the\s+(two|three|four|five|six|seven|eight|\d)\s+(?:clusters|groups|areas|lists|categories|concentrations)(?:\s*\(\s*(\d+(?:\.\d+)?)\s+credits?\s+in\s+total\s*\))?$`)
	// complSpreadInRe matches a complSpreadTailRe clause inside text: "(at least
	// 1.0 credit from 3 of the 5 areas)".
	complSpreadInRe = regexp.MustCompile(`(?i)[,;]?\s*\(?\s*\b(?:including\s+)?(?:at\s+least\s+)?(?:one\s+or\s+more\s+credits?|\d+(?:\.\d+)?\s+credits?)\s+(?:from|in)\s+(?:at\s+least\s+)?(?:one|two|three|four|five|\d)\s+of\s+the\s+(?:two|three|four|five|six|seven|eight|\d)\s+(?:clusters|groups|areas|lists|categories|concentrations)\b(?:\s*\(\s*\d+(?:\.\d+)?\s+credits?\s+in\s+total\s*\))?\s*\)?`)
	// complSpreadEachRe matches the least from each list of a spread: "with at
	// least 0.5 credit in each of the three selected concentrations".
	complSpreadEachRe = regexp.MustCompile(`(?i)^(?:with\s+)?at\s+least\s+(\d+(?:\.\d+)?)\s+credits?\s+in\s+each\s+of\s+the\s+(two|three|four|five|\d)\s+selected\s+\w+$`)
	// complFollowingRequirementsRe matches a lead whose rules are the lines under
	// it: ", with the following requirements:".
	complFollowingRequirementsRe = regexp.MustCompile(`(?i),?\s+with\s+the\s+following\s+requirements\s*:?$`)
	// complEachListRe matches each of the lists below: "0.5 credit from each of
	// the three Thematic Clusters".
	complEachListRe = regexp.MustCompile(`(?i)^(?:[\w\s&-]+\(\s*(\d+(?:\.\d+)?)\s+credits?\s*\)\s*:\s*)?(\d+(?:\.\d+)?)\s+credits?\s+from\s+each\s+of\s+the\s+(two|three|four|five|\d)\s+(?:[\w-]+\s+)?(?:clusters|groups|areas|lists|categories|concentrations)$`)
	// complKOfGroupsRe matches k of n lists: "At least one 0.5 credit in three of
	// the four FAH Groups".
	complKOfGroupsRe = regexp.MustCompile(`(?i)^(?:at\s+least\s+)?(?:one\s+)?(\d+(?:\.\d+)?)\s+credits?\s+(?:in|from)\s+(one|two|three|four|five|\d)\s+of\s+the\s+(two|three|four|five|six|seven|eight|\d)\s+(?:(?-i:[A-Z]{3})\s+)?(?:groups|lists|areas|clusters)` + complSeeBelowPat + `$`)
	// complEachGroupLeadRe matches a lead over its own lists with a floor on
	// each: "3.0 credits from among the following courses, including at least 0.5
	// credit from each group".
	complEachGroupLeadRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+from\s+(?:among\s+)?the\s+following(?:\s+courses)?,?\s+including\s+at\s+least\s+(\d+(?:\.\d+)?)\s+credits?\s+from\s+each\s+(?:group|list|area|cluster)$`)
	// complFollowingCoursesRe matches a pool over the lists under it: "1.5
	// credits from the following courses".
	complFollowingCoursesRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+(?:from|in)\s+(?:(?:among\s+)?the\s+following\s+(?:courses|lists|groups)|the\s+(?:groups|lists)\s+(?:listed\s+)?(below))$`)
	// complApplyUnderRe matches a sentence letting a student ask for courses to
	// count: "students may apply to have these courses considered for credit
	// under Requirement 2".
	complApplyUnderRe = regexp.MustCompile(`(?i)\bstudents\s+may\s+apply\s+to\s+have\s+these\s+courses\s+considered\s+for\s+credit\s+under\s+requirement\s+\d{1,2}$`)
	// complRangeFromListsRe matches a range of credits from lists: "3.0 to 3.5
	// additional credits chosen from Groups A, B, and/or C".
	complRangeFromListsRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s*(?:to|-|–)\s*(\d+(?:\.\d+)?)\s+(?:additional\s+)?credits?\s+(?:(?:chosen|selected|taken)\s+)?(?:from|in)\s+(` + complListRefPat + `)$`)
	// complCoreOrListRe matches "1.0 credit from the Core Group or Group A".
	complCoreOrListRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+from\s+the\s+core\s+group\s+or\s+(` + complListRefPat + `)$`)
	// complDesigsOrListBelowRe matches designators or the list below: "Additional
	// 1.0 or 1.5 credits from SAS and CAS courses or the list of eligible courses
	// offered by other departments found below".
	complDesigsOrListBelowRe = regexp.MustCompile(`(?i)^additional\s+(\d+(?:\.\d+)?)\s+or\s+\d+(?:\.\d+)?\s+credits?\s*(?:\([^()]*\))?\s+from\s+((?-i:[A-Z]{3})(?:\s*(?:,|and)\s*(?-i:[A-Z]{3}))*)\s+courses\s+or\s+the\s+list\s+of\s+eligible\s+courses\s+offered\s+by\s+other\s+departments\s+(?:found\s+)?below$`)
	// complFromGroupsEachRe matches a pool over lists with a floor on each: "4.5
	// credits from Group A and B courses, with at least two credits from each
	// group".
	complFromGroupsEachRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+from\s+(` + complListRefPat + `)(?:\s+courses)?,?\s+with\s+at\s+least\s+(\d+(?:\.\d+)?|one|two|three|four)\s+credits?\s+(?:from|in)\s+each(?:\s+(?:group|list|area|cluster))?$`)
	// complEachOfGroupsRe matches "At least 0.5 credit from each of Groups A, B,
	// C, and D".
	complEachOfGroupsRe = regexp.MustCompile(`(?i)^(?:at\s+least\s+)?(\d+(?:\.\d+)?)\s+(?:additional\s+)?credits?\s+(?:in|from)\s+each\s+of\s+(` + complListRefPat + `)` + complSeeBelowPat + `$`)
	// complOneOrTwoRe matches "1.0 credit from either one concentration below, or
	// 0.5 credit from two concentrations below".
	complOneOrTwoRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+from\s+either\s+one\s+(?:concentration|cluster|group|area|list)\s+below,?\s+or\s+(\d+(?:\.\d+)?)\s+credits?\s+from\s+(two|\d)\s+(?:concentrations|clusters|groups|areas|lists)\s+below$`)
)

// complOrAboveRe matches a level or above: "300-level or above".
var complOrAboveRe = regexp.MustCompile(`(?i)\b([1-4]00)\s*-?\s*level\s+or\s+(?:above|higher)\b`)

// orAbove rewrites "300-level or above" as "300+ level", which listRefRule
// reads.
func orAbove(s string) string { return complOrAboveRe.ReplaceAllString(s, "$1+ level") }

// spreadAlloc reads an allocation of text that spreads its amount over
// lists, or draws on each of them, or on one or two of them.
func (a *compAssembler) spreadAlloc(row *compRow, ls compListRefs) bool {
	t, put, ok := rowText(row)
	if !ok {
		return false
	}
	t = strings.TrimSpace(strings.TrimSuffix(complItemLabelRe.ReplaceAllString(t, ""), "."))
	if m := complSpreadLeadRe.FindStringSubmatch(t); m != nil {
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
			if e := complSpreadEachRe.FindStringSubmatch(tail); e != nil {
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
				r.Raw = rawHead(row.r)
			}
			rules = append(rules, rs...)
		}
		if a.inlineSpread(row, put, amount, k, each, names, rules, ls) {
			return true
		}
		return a.spread(row, put, amount, k, each, names, nil, rules, ls)
	}
	if m := complKOfGroupsRe.FindStringSubmatch(t); m != nil {
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
	if m := complEachGroupLeadRe.FindStringSubmatch(t); m != nil {
		// the pool of the item's own lists, with the floor on each under it
		names := a.ownLists(row, ls)
		amount, _ := strconv.ParseFloat(m[1], 64)
		each, _ := strconv.ParseFloat(m[2], 64)
		if len(names) < 2 || each*float64(len(names)) > amount+1e-9 {
			return false
		}
		return a.spread(row, put, amount, len(names), each, names, nil, nil, ls)
	}
	if m := complFollowingCoursesRe.FindStringSubmatch(complNotAlreadyRe.ReplaceAllString(t, "")); m != nil {
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
			if row.r.Label != "" && complApplyUnderRe.MatchString(note) && strings.HasSuffix(strings.ToLower(note), "under requirement "+strings.ToLower(row.r.Label)) {
				pool = orNode(pool, textNode(types.TextPermission, note))
				addRaw(row, note)
				a.notes = slices.DeleteFunc(a.notes, func(x string) bool { return x == note })
				break
			}
		}
		put(pool)
		return true
	}
	if m := complRangeFromListsRe.FindStringSubmatch(strings.TrimSuffix(t, ":")); m != nil {
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
	if m := complCoreOrListRe.FindStringSubmatch(strings.TrimSuffix(t, ":")); m != nil {
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
	if m := complDesigsOrListBelowRe.FindStringSubmatch(t); m != nil {
		// the lower amount from a union of the designators and the list below
		names := a.listsBelow(row, ls)
		if len(names) != 1 {
			return false
		}
		name, _, ok := a.addUnion(row, []*types.ReqNode{{Type: types.NodeCredits, Designators: desigRe.FindAllString(m[2], -1)}, {Type: types.NodeCredits, Lists: names}}, ls)
		if !ok {
			return false
		}
		amount, _ := strconv.ParseFloat(m[1], 64)
		put(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Lists: []string{name}})
		return true
	}
	if m := complFromGroupsEachRe.FindStringSubmatch(t); m != nil {
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
	if m := complEachOfGroupsRe.FindStringSubmatch(t); m != nil {
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
	if m := complEachListRe.FindStringSubmatch(t); m != nil {
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
	if m := complOneOrTwoRe.FindStringSubmatch(t); m != nil {
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
	if loc := complSpreadInRe.FindStringIndex(t); loc != nil {
		clause = strings.Trim(t[loc[0]:loc[1]], " ,;()")
		t = strings.TrimSpace(t[:loc[0]] + t[loc[1]:])
	}
	head, tails := splitListTails(t)
	under := complFollowingRequirementsRe.MatchString(t) && slices.ContainsFunc(row.children, func(c *compRow) bool {
		return c.r.Kind == types.RequirementConstraint && c.r.Req.Type == types.NodeText && complSpreadTailRe.MatchString(complClean(c.r.Req.Text))
	})
	if clause == "" && !under {
		return false
	}
	var body *types.ReqNode
	var rules []*types.Requirement
	if under {
		b, rs, notes, ok := a.listRefAlloc(row, complFollowingRequirementsRe.ReplaceAllString(t, ""), ls)
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
		r.Raw = rawHead(row.r)
		spread = append(spread, r)
	}
	for _, tail := range tails {
		rs, _, ok := listRefRule(normalizeListRef(orAbove(tail), ls), ls, nil)
		if !ok {
			return false
		}
		for _, r := range rs {
			r.Raw = rawHead(row.r)
		}
		rules = append(rules, rs...)
	}
	for _, c := range row.children {
		if c.r.Kind != types.RequirementConstraint || c.r.Req.Type != types.NodeText {
			continue
		}
		if r, ok := spreadTail(complClean(c.r.Req.Text), body, ls); ok {
			c.r.Req = r.Req
		}
	}
	put(body)
	for _, r := range append(rules, spread...) {
		a.rowUnder(row, r)
	}
	return true
}

// complSubLabelRe matches a sub-list's name taken from its marker alone, not
// the text: "2a".
var complSubLabelRe = regexp.MustCompile(`^\d{0,2}[a-z]$`)

// inlineSpread reads a spread over the lead's own unnamed sub-lists ("a.
// CSC401H1, CSC485H1"): the pool of all their courses, with caps on its "/"
// pairs, and the spread over their courses. It reports whether it read it.
func (a *compAssembler) inlineSpread(row *compRow, put func(*types.ReqNode), amount float64, k int, each float64, names []string, rules []*types.Requirement, ls compListRefs) bool {
	var parts []*types.ReqNode
	var members, notes []string
	for _, name := range names {
		l := ls.byName(name)
		if !complSubLabelRe.MatchString(name) || len(l.defs) != 1 || l.defs[0].row != row {
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
		a.rowUnder(row, &types.Requirement{Kind: types.RequirementLimit, Req: c, Raw: rawHead(row.r)})
	}
	for _, r := range rules {
		a.rowUnder(row, r)
	}
	a.rowUnder(row, &types.Requirement{Kind: types.RequirementConstraint, Req: spread, Raw: rawHead(row.r)})
	row.r.Notes = slices.DeleteFunc(row.r.Notes, func(n string) bool { return slices.Contains(notes, n) })
	return true
}

// spreadTail reads a complSpreadTailRe clause over pool's lists into a
// constraint, or ok false.
func spreadTail(clause string, pool *types.ReqNode, ls compListRefs) (*types.Requirement, bool) {
	m := complSpreadTailRe.FindStringSubmatch(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(clause), ".")))
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
	return &types.Requirement{Kind: types.RequirementConstraint, Req: n, Raw: cleanNote(clause)}, true
}

// spread puts on row the pool of names (with the levels, if any) and the
// spread over them, with rules, under it.
func (a *compAssembler) spread(row *compRow, put func(*types.ReqNode), amount float64, k int, each float64, names []string, levels []int, rules []*types.Requirement, ls compListRefs) bool {
	n, ok := spreadNode(k, each, names, levels, ls)
	if !ok {
		return false
	}
	put(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Lists: slices.Clone(names), Levels: slices.Clone(levels)})
	for _, r := range rules {
		a.rowUnder(row, r)
	}
	a.rowUnder(row, &types.Requirement{Kind: types.RequirementConstraint, Req: n, Raw: rawHead(row.r)})
	return true
}

// complSelectionRe matches one selection of a choice among lists named by
// their subject: "1.0 credit from the Politics and Economics
// disciplinary/thematic cluster".
var complSelectionRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+from\s+the\s+(.+?)\s+(?:(?:disciplinary/thematic|disciplinary|thematic)\s+)?(?:cluster|group|area|list|concentration)$`)

// listSubject returns the subject a list's title names, as a subjectKey:
// "Politics and Economics" in "Cluster 1: Politics and Economics".
func listSubject(l *compList) string {
	t := l.list.Title
	if t == "" {
		t = l.list.Name
	}
	if i := strings.LastIndex(t, ": "); i >= 0 {
		t = t[i+2:]
	} else if i := strings.Index(t, " - "); i >= 0 {
		t = t[i+3:]
	}
	return subjectKey(t)
}

// subjectKey is a subject in a form to compare: lower case, "&" as "and".
func subjectKey(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.ReplaceAll(s, "&", " and "))), " ")
}

// selections reads a choice whose options are text naming a list by its
// subject ("1.0 credit from the Politics and Economics cluster"): each option
// is credits from that list.
func (a *compAssembler) selections(row *compRow, ls compListRefs) bool {
	n := row.r.Req
	if n.Type != types.NodeChoose && n.Type != types.NodeOr && n.Type != types.NodeAnd || len(n.Items) < 2 {
		return false
	}
	var parts []*types.ReqNode
	for _, it := range n.Items {
		if it.Type != types.NodeText || it.Kind != types.TextOther {
			return false
		}
		m := complSelectionRe.FindStringSubmatch(complClean(it.Text))
		if m == nil {
			return false
		}
		var found []*compList
		for _, l := range a.sortedLists(ls) {
			if !l.bad && listSubject(l) == subjectKey(m[2]) {
				found = append(found, l)
			}
		}
		if len(found) != 1 {
			return false
		}
		v, _ := strconv.ParseFloat(m[1], 64)
		parts = append(parts, &types.ReqNode{Type: types.NodeCredits, Amount: v, Lists: []string{found[0].list.Name}})
	}
	k := 1
	switch n.Type {
	case types.NodeChoose:
		k = n.Count
	case types.NodeAnd:
		k = len(parts)
	}
	body, ok := partsNode(k, parts, ls)
	if !ok {
		return false
	}
	if k == 1 && body.Type == types.NodeChoose {
		body = orNode(body.Items...)
	}
	row.r.Req = body
	return true
}

var (
	// complSharesLeadRe matches an amount made of shares set out under it:
	// "4. 1.0 credits including:".
	complSharesLeadRe = regexp.MustCompile(`(?i)^(?:\d{1,2}\.\s+)?(\d+(?:\.\d+)?)\s+credits?\s+including\s*:$`)
	// complShareRe matches one share: "Up to 0.5 credits from CSC301H1, CSC302H1
	// (if not taken in list 3)".
	complShareRe = regexp.MustCompile(`(?i)^(?:[a-z][.)]\s+)?up\s+to\s+(\d+(?:\.\d+)?)\s+credits?\s+from\s+(.+?)(?:\s*\(\s*(if\s+not\s+taken\s+in\s+(?:list|requirement)\s+\d+)\s*\))?$`)
)

// shares reads an amount made of shares, which are text constraints under it:
// the pool of all their courses and a limit per share. "(if not taken in list
// 3)" is a note: the rows' exclusive claims already say it.
func (a *compAssembler) shares(row *compRow) bool {
	m := complSharesLeadRe.FindStringSubmatch(strings.TrimSpace(complClean(rawHead(row.r))))
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
		s := complShareRe.FindStringSubmatch(strings.TrimSuffix(complClean(c.r.Req.Text), "."))
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
			notes = append(notes, cleanNote(s[3]))
		}
		limits = append(limits, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Req: &types.ReqNode{Type: types.NodeCredits, Courses: n.Courses, MaxAmount: maxAmount(v)}, Raw: c.r.Raw}})
	}
	if room < amount-1e-9 {
		return false
	}
	row.r.Req = &types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: dedupe(courses)}
	row.r.Notes = append(row.r.Notes, notes...)
	row.children = limits
	return true
}

var (
	// complPathwaysLeadRe matches "1.0 credit from one of the following pathways
	// below:".
	complPathwaysLeadRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+from\s+one\s+of\s+the\s+following\s+pathways(?:\s+below)?\s*:?$`)
	// complPathwayRe matches one pathway: "Thesis Option: HIS475H1 and HIS476H1".
	complPathwayRe = regexp.MustCompile(`(?i)^(?:[a-z]\)\s*)?(\w+)\s+option:\s*(.+)$`)
	// complInOrderRe matches "taken in that order", which the courses' requisites
	// see to, and a pointer to a note: "(see Note 2)".
	complInOrderRe = regexp.MustCompile(`(?i)\s+taken\s+in\s+that\s+order\b\.?|\s*\(\s*see\s+note\s+\d+\s*\)`)
	// complSeniorListRe matches "1.0 credit of Senior (300 or 400-level)
	// Methodology course(s), listed below".
	complSeniorListRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+of\s+senior\s+\(\s*([1-4]00)\s+or\s+([1-4]00)\s*-?\s*level\s*\)\s+(.+?)\s+course(?:\(s\)|s)?,?\s+listed\s+below$`)
)

// pathways reads a choice of pathways whose options are notes or text rules
// under it: their or, each option courses or credits from a list named by its
// title.
func (a *compAssembler) pathways(row *compRow, ls compListRefs) bool {
	t, put, ok := rowText(row)
	if !ok {
		return false
	}
	m := complPathwaysLeadRe.FindStringSubmatch(strings.TrimSpace(complItemLabelRe.ReplaceAllString(t, "")))
	if m == nil {
		return false
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	var opts []*types.ReqNode
	var used []string
	var kids []*compRow
	read := func(s string) bool {
		p := complPathwayRe.FindStringSubmatch(strings.TrimSpace(s))
		if p == nil {
			return false
		}
		body := strings.TrimSpace(strings.TrimSuffix(complInOrderRe.ReplaceAllString(p[2], ""), "."))
		if l := complSeniorListRe.FindStringSubmatch(body); l != nil {
			list := ls[complTitleKey(l[4]+" courses")]
			v, _ := strconv.ParseFloat(l[1], 64)
			if list == nil || list.bad || v != amount {
				return false
			}
			opts = append(opts, &types.ReqNode{Type: types.NodeCredits, Amount: v, Lists: []string{list.list.Name}, Levels: parseLevels(l[2] + "/" + l[3] + "-level")})
			return true
		}
		n, _ := parseCompletionText(body)
		if v, ok := minCredits(n); n == nil || hasText(n) || !ok || v != amount {
			return false
		}
		opts = append(opts, n)
		return true
	}
	for _, n := range row.r.Notes {
		if read(n) {
			used = append(used, n)
		}
	}
	for _, c := range row.children {
		if c.r.Kind == types.RequirementConstraint && c.r.Req.Type == types.NodeText && read(complClean(c.r.Req.Text)) {
			continue
		}
		kids = append(kids, c)
	}
	if len(opts) < 2 || len(opts) != len(used)+len(row.children)-len(kids) {
		return false
	}
	for _, s := range used {
		if i := slices.IndexFunc(a.lines, func(l string) bool { return strings.Contains(rawWords(l), rawWords(s)) }); i >= 0 {
			addRaw(row, strings.TrimSpace(a.lines[i]))
		}
	}
	for _, c := range row.children {
		if !slices.Contains(kids, c) {
			addRaw(row, c.r.Raw)
		}
	}
	put(orNode(opts...))
	row.children = kids
	return true
}

// complMinimumTitledRe matches an amount of a list named by its title, and the
// floors after it: "A minimum of 3.0 Society-Culture credits".
var complMinimumTitledRe = regexp.MustCompile(`^(?:\d{1,2}\.\s+)?(?:(?i:a\s+minimum\s+of|at\s+least)\s+)?(\d+(?:\.\d+)?)\s+([A-Z][\w-]*(?:\s+[A-Z][\w-]*)*)\s+(?i:credits?)\b(.*?)[;.]?$`)

// complOneOfWhichLevelRe matches a floor on one of an amount's credits: ", one
// of which must be 300/400-level".
var complOneOfWhichLevelRe = regexp.MustCompile(`(?i)^\s*,\s*one\s+of\s+which\s+must\s+be\s+(\d00)\s*/\s*\d00-level$`)

// titledMinimum reads an item on a list named by its title, which the grammar
// split, as listRefAlloc reads "3.0 credits from Society-Culture courses". A
// list of the same courses offered elsewhere may count too: an ambiguity.
func (a *compAssembler) titledMinimum(row *compRow, ls compListRefs) bool {
	m := complMinimumTitledRe.FindStringSubmatch(strings.TrimSpace(complClean(rawHead(row.r))))
	if m == nil {
		return false
	}
	l := ls[complTitleKey(m[2]+" courses")]
	if l == nil || l.bad {
		return false
	}
	tail := orAbove(m[3])
	one := complOneOfWhichLevelRe.FindStringSubmatch(tail)
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
		half := cloneNode(rules[0].Req)
		half.Amount = 0.5
		rules[0].Req = orNode(rules[0].Req, andNode(half, textNode(types.TextAmbiguity, `"one of which" may mean one course (0.5 credit)`)))
	}
	for _, o := range a.sortedLists(ls) {
		if o != l && !o.bad && strings.HasPrefix(strings.ToLower(o.list.Name), strings.ToLower(l.list.Name)+" on ") {
			wide := cloneNode(body)
			wide.Lists = append(wide.Lists, o.list.Name)
			body = orNode(body, andNode(wide, textNode(types.TextAmbiguity, `"`+o.list.Name+`" may count as `+m[2]+" credits")))
		}
	}
	head := rawHead(row.r)
	row.r.Req = body
	row.r.Notes = append(row.r.Notes, notes...)
	row.children = slices.DeleteFunc(row.children, func(c *compRow) bool {
		return c.r.Kind == types.RequirementConstraint && rawHead(c.r) == head && len(c.children) == 0
	})
	for _, r := range rules {
		r.Raw = head // the source line, not the sentence listRefAlloc read
		a.rowUnder(row, r)
	}
	return true
}

var (
	// complEligibleBelowRe matches a pool's alternative of the lists below it:
	// "from the eligible courses listed below".
	complEligibleBelowRe = regexp.MustCompile(`(?i)^(?:from\s+)?(?:the\s+)?(?:eligible|pre-approved)\s+courses\s+(?:from\s+cognate\s+units\s+)?listed\s+below(?:\s*\(\s*(` + complListRefPat + `)\s*\))?$`)
	// complMustIncludeFloorRe matches "These credits must include 1.0 credit at
	// the 300+ level from AMS courses".
	complMustIncludeFloorRe = regexp.MustCompile(`(?i)^these\s+credits\s+must\s+include\s+(\d+(?:\.\d+)?)\s+credits?\s+at\s+the\s+([1-4]00)\+\s+level\s+from\s+((?-i:[A-Z]{3}))\s+courses$`)
	// complBreadthSplitRe matches a split over breadth categories: "Of the 2.5
	// credits, a minimum of 0.5 credit must be from the Social Sciences/BR=3;
	// another 0.5 credit must be from the Humanities/BR=1 or 2".
	complBreadthSplitRe = regexp.MustCompile(`(?i)^of\s+the\s+(\d+(?:\.\d+)?)\s+credits,\s+a\s+minimum\s+of\s+(\d+(?:\.\d+)?)\s+credits?\s+must\s+be\s+from\s+the\s+[\w\s]+?/BR=(\d)((?:\s+or\s+\d)?);\s+another\s+(\d+(?:\.\d+)?)\s+credits?\s+must\s+be\s+from\s+the\s+[\w\s]+?/BR=(\d)((?:\s+or\s+\d)?)\.?\s*(?:\(\s*(.+?)\s+can\s+be\s+used\s+to\s+cover\s+the\s+[\w\s]+?\s+requirement\s+and\s+(.+?)\s+for\s+the\s+[\w\s]+?\s+requirement\s*\)\.?)?$`)
)

// cognatePool reads a pool whose alternative is the lists below it ("or from
// the eligible courses listed below"): one list of both, with the text rules
// under it read: a floor, and a breadth split as a group of parts.
func (a *compAssembler) cognatePool(row *compRow, ls compListRefs) bool {
	n := row.r.Req
	if n.Type != types.NodeOr || len(n.Items) != 2 || !isPool(n.Items[0]) || n.Items[0].Lists != nil || n.Items[1].Type != types.NodeText || row.r.Label == "" {
		return false
	}
	m := complEligibleBelowRe.FindStringSubmatch(strings.TrimSuffix(complClean(n.Items[1].Text), "."))
	if m == nil {
		return false
	}
	names := a.leadLists(row, ls)
	if m[1] != "" {
		names = ls.resolve(listRefNames(m[1]))
	}
	if names == nil {
		return false
	}
	pool := n.Items[0]
	var split *compRow
	var kids []*compRow
	for _, c := range row.children {
		if c.r.Kind == types.RequirementConstraint && c.r.Req.Type == types.NodeText {
			t := strings.TrimSuffix(complClean(c.r.Req.Text), ".")
			if f := complMustIncludeFloorRe.FindStringSubmatch(t); f != nil {
				v, _ := strconv.ParseFloat(f[1], 64)
				c.r.Req = &types.ReqNode{Type: types.NodeCredits, Amount: v, Designators: []string{f[3]}, Levels: parseLevels(f[2] + "+ level")}
			} else if complBreadthSplitRe.MatchString(t) {
				split = c
				continue
			}
		}
		kids = append(kids, c)
	}
	member := cloneNode(pool)
	member.Amount = 0
	name, _, ok := a.addUnion(row, []*types.ReqNode{member, {Type: types.NodeCredits, Lists: names}}, ls)
	if !ok {
		return false
	}
	body := &types.ReqNode{Type: types.NodeCredits, Amount: pool.Amount, Lists: []string{name}}
	row.r.Req = body
	if split == nil {
		return true
	}
	s := complBreadthSplitRe.FindStringSubmatch(strings.TrimSuffix(complClean(split.r.Req.Text), "."))
	total, _ := strconv.ParseFloat(s[1], 64)
	first, _ := strconv.ParseFloat(s[2], 64)
	second, _ := strconv.ParseFloat(s[5], 64)
	if math.Abs(total-pool.Amount) > 1e-9 || first+second > total {
		row.children = append(kids, split)
		return true
	}
	part := func(v float64, br, more, codes string) *compRow {
		p := &types.ReqNode{Type: types.NodeCredits, Amount: v, Lists: []string{name}, Breadth: parseBreadths(br + more)}
		if cs := codeRe.FindAllString(codes, -1); cs != nil {
			// courses the split names as able to cover this part
			p = orNode(p, &types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: cs})
		}
		return &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Req: p, Raw: split.r.Raw}}
	}
	rest := &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Req: &types.ReqNode{Type: types.NodeCredits, Amount: total - first - second, Lists: []string{name}}, Raw: rawHead(row.r)}}
	row.r.Req = nil
	row.children = append([]*compRow{part(first, s[3], s[4], s[8]), part(second, s[6], s[7], s[9]), rest}, kids...)
	return true
}

// parseBreadths reads "3", "1 or 2" as breadth categories.
func parseBreadths(s string) []int {
	var out []int
	for _, f := range regexp.MustCompile(`\d`).FindAllString(s, -1) {
		v, _ := strconv.Atoi(f)
		out = append(out, v)
	}
	return out
}

var (
	// complHeadingLevelRe matches a heading's level filter: "(200+ level)".
	complHeadingLevelRe = regexp.MustCompile(`(?i)\(\s*([1-4]00)\+\s*level\s*\)`)
	// complAreaRowRe matches an area's share: "2.0 credits in Africa/Asia/Middle
	// East with at least one 0.5 credit in 2 of the 3 areas".
	complAreaRowRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+(?:in\s+)?((?-i:[A-Z])[\w/ ]*?)(?:\s+with\s+at\s+least\s+one\s+(\d+(?:\.\d+)?)\s+credits?\s+in\s+(\w+)\s+of\s+the\s+(\w+)\s+areas)?$`)
	// complAnyAreaCapRe matches "This requirement cannot be met by more than 1.0
	// credit from any area".
	complAnyAreaCapRe = regexp.MustCompile(`(?i)^this\s+requirement\s+cannot\s+be\s+met\s+by\s+more\s+than\s+(\d+(?:\.\d+)?)\s+credits?\s+from\s+any\s+(?:one\s+)?(?:area|group|cluster|list)$`)
	// complAreaHeadingRe matches a heading over an area's sub-lists: "Geographic
	// Area a): Africa, Asia, Middle East".
	complAreaHeadingRe = regexp.MustCompile(`^(Geographic\s+Area\s+([a-z])\))\s*:\s*(.+)$`)
	// complSubGroupTitleRe matches a sub-list's title: "(Sub-group: Africa)".
	complSubGroupTitleRe = regexp.MustCompile(`^\(\s*Sub-group:\s*(.+?)\s*\)$`)
)

// areaLists names the History programs' sub-lists by subject ("(Sub-group:
// Africa)" is "Africa") and makes the area headings over them lists of them.
// It runs on the lists buildLists read, before they are looked up by key.
func areaLists(built []*compList) []*compList {
	byKey := map[string]*compList{}
	for _, l := range built {
		byKey[l.key] = l
	}
	var out []*compList
	for _, l := range built {
		m := complSubGroupTitleRe.FindStringSubmatch(l.list.Name)
		if m == nil || !l.titled || len(l.defs) != 1 || l.defs[0].block == nil || byKey[complTitleKey(m[1])] != nil {
			out = append(out, l)
			continue
		}
		l.list.Title, l.list.Name = l.list.Name, m[1]
		l.key = complTitleKey(m[1])
		byKey[l.key] = l
		out = append(out, l)
		var h *compBlock
		for h = l.defs[0].block.heading; h != nil && !complAreaHeadingRe.MatchString(h.section); h = h.heading {
		}
		if h == nil || l.parent != "" {
			continue
		}
		a := complAreaHeadingRe.FindStringSubmatch(h.section)
		key := complTitleKey(a[1])
		p := byKey[key]
		if p == nil {
			p = &compList{key: key, list: &types.CourseList{Name: a[1], Title: h.section}, line: -1, titled: true}
			byKey[key] = p
			out = append(out, p)
		}
		p.at(h.line)
		l.parent = key
		p.list.Items = append(p.list.Items, &types.ReqNode{Type: types.NodeCredits, Lists: []string{l.list.Name}})
	}
	return out
}

// areaGroup reads the History programs' "Geographic Distribution" group: each
// part draws on the area list its letter names, at the heading's levels, with
// its spread and caps per sub-list. It reports whether it read every part.
func (a *compAssembler) areaGroup(row *compRow, ls compListRefs) bool {
	lv := complHeadingLevelRe.FindStringSubmatch(rawHead(row.r))
	if lv == nil || len(row.children) == 0 {
		return false
	}
	levels := parseLevels(lv[1] + "+ level")
	type read struct {
		body  *types.ReqNode
		rules []*types.Requirement
	}
	var reads []read
	for _, c := range row.children {
		t, _, ok := rowText(c)
		m := complAreaRowRe.FindStringSubmatch(strings.TrimSuffix(t, "."))
		if !ok || c.r.Kind != types.RequirementAllocation || m == nil || c.r.Label == "" {
			return false
		}
		area := ls[complTitleKey("Geographic Area "+c.r.Label[len(c.r.Label)-1:]+")")]
		if area == nil || area.bad || !strings.Contains(strings.ToLower(area.list.Title), strings.ToLower(strings.Split(m[2], "/")[0])) {
			return false
		}
		var subs []string
		for _, it := range area.list.Items {
			subs = append(subs, it.Lists...)
		}
		if ls.resolve(keysOf(ls, subs)) == nil {
			return false
		}
		amount, _ := strconv.ParseFloat(m[1], 64)
		r := read{body: &types.ReqNode{Type: types.NodeCredits, Amount: amount, Lists: []string{area.list.Name}, Levels: slices.Clone(levels)}}
		if m[3] != "" {
			each, _ := strconv.ParseFloat(m[3], 64)
			if countOf(m[5]) != len(subs) {
				return false
			}
			n, ok := spreadNode(countOf(m[4]), each, subs, levels, ls)
			if !ok {
				return false
			}
			r.rules = append(r.rules, &types.Requirement{Kind: types.RequirementConstraint, Req: n, Raw: rawHead(c.r)})
		}
		for _, k := range c.children {
			if k.r.Kind != types.RequirementConstraint || k.r.Req.Type != types.NodeText {
				return false
			}
			cm := complAnyAreaCapRe.FindStringSubmatch(strings.TrimSuffix(complClean(k.r.Req.Text), "."))
			if cm == nil {
				return false
			}
			v, _ := strconv.ParseFloat(cm[1], 64)
			for _, s := range subs {
				r.rules = append(r.rules, &types.Requirement{Kind: types.RequirementLimit, Req: &types.ReqNode{Type: types.NodeCredits, Lists: []string{s}, MaxAmount: maxAmount(v)}, Raw: k.r.Raw})
			}
		}
		reads = append(reads, r)
	}
	for i, c := range row.children {
		c.r.Req = reads[i].body
		c.children = nil
		for _, r := range reads[i].rules {
			a.rowUnder(c, r)
		}
	}
	return true
}

// keysOf returns the keys of the lists named names, nil if one is missing.
func keysOf(ls compListRefs, names []string) []string {
	var out []string
	for _, n := range names {
		l := ls.byName(n)
		if l == nil {
			return nil
		}
		out = append(out, l.key)
	}
	return out
}

var (
	// complTitledBelowRe matches an item on a list named by its title, the title
	// first: "Temporal Requirement: 1.0 credit in pre-1800 history course(s)
	// listed below".
	complTitledBelowRe = regexp.MustCompile(`^([A-Z][\w ]+?):\s+(\d+(?:\.\d+)?)\s+(?i:credits?\s+in\s+[\w\s-]+?\s+course(?:\(s\)|s)?\s+listed\s+below)$`)
	// complStarredTopicsRe matches a list's footnote marking topics courses:
	// "***** Some “Topics in History”: Various topics each year".
	complStarredTopicsRe = regexp.MustCompile(`^(\*{2,})\s*(Some\s+.*\bTopics\s+in\b.*)$`)
	// complCantCountRe matches "It cannot be counted towards a History program",
	// after a sentence naming the course.
	complCantCountRe = regexp.MustCompile(`(?i)^it\s+cannot\s+be\s+counted\s+towards?\s+an?\s+\w+\s+program\b`)
)

// spreadFloors reads the header's text floors with spreadRule, since
// namedLists doesn't walk them. A numbered item read as a program rule then
// takes its number and its place among the items.
func (a *compAssembler) spreadFloors(ls compListRefs) {
	for _, r := range a.floors {
		a.spreadRule(r, ls)
	}
	a.program = slices.DeleteFunc(a.program, func(r *compRow) bool {
		if r.r.Kind != types.RequirementConstraint || r.r.Label != "" || len(r.children) > 0 {
			return false
		}
		head := rawWords(rawHead(r.r))
		var num, src string
		for _, l := range a.lines {
			if m := complItemNumRe.FindStringSubmatch(l); m != nil && rawWords(m[2]) == head {
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

// complItemNumRe matches a numbered line: "6. At least 2.0 credits".
var complItemNumRe = regexp.MustCompile(`^\s*(\d{1,2})\.\s+(.+)$`)

// spreadRule reads program and item rules of text that need the lists or rows:
// an overlap sentence naming a group, a course that cannot count towards the
// program (a limit of 0), and a spread stated with the total.
func (a *compAssembler) spreadRule(row *compRow, ls compListRefs) ([]*compRow, bool) {
	n := row.r.Req
	if n != nil && n.Type == types.NodeAnd && len(n.Items) == 2 && n.Items[0].Type == types.NodeCredits && n.Items[1].Type == types.NodeText {
		n = n.Items[1] // and(amount, text): the header's spread clause is the text
	}
	if n == nil || n.Type != types.NodeText || n.Kind != types.TextOther {
		return nil, false
	}
	t := strings.TrimSuffix(complClean(n.Text), ".")
	if row.r.Req.Type == types.NodeText && a.role(t) == roleOverlap && a.overlap(t) {
		// an overlap sentence naming items, read now that the items exist
		return nil, true
	}
	if row.r.Req.Type == types.NodeText && complCantCountRe.MatchString(t) {
		for _, l := range a.lines {
			if i := strings.Index(rawWords(l), rawWords(t)); i > 0 {
				if codes := dedupe(codeRe.FindAllString(l[:strings.Index(l, "It ")], -1)); len(codes) == 1 {
					row.r.Kind = types.RequirementLimit
					row.r.Req = &types.ReqNode{Type: types.NodeCredits, Courses: codes, MaxAmount: maxAmount(0)}
					row.r.Raw = cleanNote(l)
					return []*compRow{row}, true
				}
			}
		}
	}
	if m := complStudentFloorRe.FindStringSubmatch(t); m != nil && row.r.Req.Type == types.NodeText {
		// a floor, and one within it as a sibling: a constraint has no children
		v, _ := strconv.ParseFloat(m[1], 64)
		f := &types.ReqNode{Type: types.NodeCredits, Amount: v, Levels: parseLevels(m[3] + "+ level")}
		if m[2] != "" {
			f.Designators = []string{m[2]}
		}
		row.r.Req = f
		out := []*compRow{row}
		if m[4] != "" {
			w, _ := strconv.ParseFloat(m[4], 64)
			inner := narrowFilter(&types.ReqNode{Type: types.NodeCredits, Amount: w, Levels: parseLevels(m[5] + "-level")}, f)
			if inner == nil {
				return nil, false
			}
			out = append(out, &compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Req: inner, Raw: row.r.Raw}})
		}
		return out, true
	}
	if m := complClusterSpreadRe.FindStringSubmatch(t); m != nil {
		var names []string
		for _, l := range a.sortedLists(ls) {
			if strings.HasPrefix(l.key, "cluster ") && l.parent == "" {
				names = append(names, l.list.Name)
			}
		}
		if names == nil || m[4] != "" && len(complCategoryLetterRe.FindAllString(m[4], -1)) != len(names) {
			return nil, false
		}
		amount, _ := strconv.ParseFloat(m[1], 64)
		k := countOf(m[3])
		levels := parseLevels(m[2] + "+ level")
		node, ok := spreadNode(k, 0.5, names, levels, ls)
		if !ok || amount < 0.5*float64(k) {
			return nil, false
		}
		if amount > 0.5*float64(k) {
			node = andNode(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Lists: names, Levels: levels}, node)
		}
		row.r.Req = node
		return []*compRow{row}, true
	}
	return nil, false
}

var (
	// complStudentFloorRe matches a floor on the student's courses: "At least 2.0
	// credits of the student’s 7.0 credits must be at the 300-level or above".
	complStudentFloorRe = regexp.MustCompile(`(?i)^at\s+least\s+(\d+(?:\.\d+)?)\s+credits?\s+of\s+the\s+student[’']s\s+(?:\d+(?:\.\d+)?\s+credits|program)\s+must\s+be\s+(?:in\s+[^()]+?\(\s*((?-i:[A-Z]{3}))\s+prefix\s+courses\s*\),?\s+)?at\s+the\s+([1-4]00)-level\s+or\s+above(?:,?\s+with\s+at\s+least\s+(\d+(?:\.\d+)?)\s+credits?\s+of\s+these\s+at\s+the\s+([1-4]00)-level)?$`)
	// complClusterSpreadRe matches a spread over the clusters, with the total:
	// "including 1.0 credit at the 300+ level in at least two clusters".
	complClusterSpreadRe = regexp.MustCompile(`(?i)^including\s+(\d+(?:\.\d+)?)\s+credits?\s+at\s+the\s+([1-4]00)\+\s+level\s+in\s+at\s+least\s+(two|three|four|\d)\s+[\w\s/]*?clusters\b,?\s*(?:categorized\s+as\s+follows:\s*(.*))?$`)
	// complCategoryLetterRe matches a category's letter: "b)" in "a) Politics
	// b) Society".
	complCategoryLetterRe = regexp.MustCompile(`(?:^|\s)[a-z]\)\s`)
)

// titledBelow reads a complTitledBelowRe item as credits from the list it
// names, or nil.
func titledBelow(t string, ls compListRefs) *types.ReqNode {
	m := complTitledBelowRe.FindStringSubmatch(strings.TrimSuffix(t, "."))
	if m == nil {
		return nil
	}
	l := ls[complTitleKey(m[1])]
	if l == nil || l.bad {
		return nil
	}
	v, _ := strconv.ParseFloat(m[2], 64)
	return &types.ReqNode{Type: types.NodeCredits, Amount: v, Lists: []string{l.list.Name}}
}

// starredTopics makes the members of a used list marked with a footnote's
// stars ("HIS389H1*****") maybe items, since only some offerings count. It
// returns the footnote's notes when a list took them.
func (a *compAssembler) starredTopics(used []*compList) []string {
	var dropped []string
	for _, line := range a.lines {
		m := complStarredTopicsRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		starred := regexp.MustCompile(`(` + codePat + `)` + regexp.QuoteMeta(m[1]) + `(?:[^*]|$)`)
		first := strings.TrimSpace(splitComplSentences(m[2])[0])
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
			if codes = dedupe(codes); codes == nil {
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
			l.list.Items = append(items, andNode(&types.ReqNode{Type: types.NodeCredits, Courses: codes}, textNode(types.TextPermission, cleanNote(first))))
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

var (
	// complOneAreaRe matches a lead over a sub-list per area: "2.0 credits in one
	// cultural area chosen from the list below:".
	complOneAreaRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+(?:of\s+[\w\s]+?\s+)?in\s+one\s+(?:language\s+|cultural\s+)?area\s+chosen\s+from\s+the\s+list\s+below\s*:?$`)
	// complChosenAreaRe matches "2.0 credits from culture courses corresponding
	// to the student’s chosen language area:".
	complChosenAreaRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+from\s+[\w\s]+?\s+courses\s+corresponding\s+to\s+the\s+student[’']s\s+chosen\s+language\s+area\s*:?$`)
	// complAltSetsRe matches a sub-list of two sets, one or the other:
	// "[ SLA107Y1, SLA118H1]; /[ SLA109Y1, SLA209H1]".
	complAltSetsRe = regexp.MustCompile(`^\s*\[([^\[\]]+)\]\s*;?\s*/\s*\[([^\[\]]+)\]\s*$`)
)

// areaChoice reads a lead over a sub-list per area: "one area" is the or of
// the areas' pools; "the chosen area" adds the correspondence as text, which
// no node can check: NotMet when no area has the amount, else Review.
func (a *compAssembler) areaChoice(row *compRow) bool {
	t, put, ok := rowText(row)
	if !ok {
		return false
	}
	t = strings.TrimSpace(complItemLabelRe.ReplaceAllString(t, ""))
	m := complOneAreaRe.FindStringSubmatch(t)
	chosen := false
	if m == nil {
		if m = complChosenAreaRe.FindStringSubmatch(t); m == nil {
			return false
		}
		chosen = true
	}
	var opts, caps []*types.ReqNode
	var notes []string
	for _, d := range a.listDefs {
		if d.row != row {
			continue
		}
		alts, ok := areaPools(m[1], d.members)
		if !ok {
			return false
		}
		opts = append(opts, alts...)
		notes = append(notes, d.note)
		if n, _ := parseCompletionText("0.5 credit from: " + d.members); !strings.Contains(d.members, "[") {
			_, c := poolCaps(n)
			caps = append(caps, c...)
		}
	}
	if len(opts) < 2 {
		return false
	}
	body := orNode(opts...)
	if chosen {
		body = andNode(body, textNode(types.TextOther, "corresponding to the student’s chosen language area"))
	}
	put(body)
	// the areas' "/" pair caps, unless a chosen-area item makes this a choice
	if !slices.ContainsFunc(a.items, func(r *compRow) bool {
		return r != row && complChosenAreaRe.MatchString(strings.TrimSpace(complItemLabelRe.ReplaceAllString(complClean(rawHead(r.r)), "")))
	}) && !chosen {
		for _, c := range dedupeNodes(caps) {
			a.rowUnder(row, &types.Requirement{Kind: types.RequirementLimit, Req: c, Raw: rawHead(row.r)})
		}
	}
	row.r.Notes = slices.DeleteFunc(row.r.Notes, func(n string) bool { return slices.Contains(notes, n) })
	return true
}

// ownLists returns the names of the lists row's own lines define, in order,
// or nil when one of them is unsound.
func (a *compAssembler) ownLists(row *compRow, ls compListRefs) []string {
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

var (
	// complSameCategoryCapRe matches a pool over the item's own lists with a cap
	// on each: "Any 3.0 credits from the following, with no more than 1.0 credit
	// from the same category".
	complSameCategoryCapRe = regexp.MustCompile(`(?i)^(?:any\s+)?(\d+(?:\.\d+)?)\s+credits?\s+from\s+the\s+following,?\s+with\s+no\s+more\s+than\s+(\d+(?:\.\d+)?)\s+credits?\s+from\s+the\s+same\s+(?:category|group|list|area|sub-?group)$`)
	// complFloorCapEachRe matches a floor and a cap on each of the item's own
	// lists, with the heading's amount: "At least 0.5 credit, but no more than
	// 1.0 credit from each subgroup".
	complFloorCapEachRe = regexp.MustCompile(`(?i)^at\s+least\s+(\d+(?:\.\d+)?)\s+credits?,?\s+but\s+no\s+more\s+than\s+(\d+(?:\.\d+)?)\s+credits?\s+from\s+each\s+(?:category|group|list|area|sub-?group)$`)
	// complListLevelRulesRe matches a pool over the item's own lists with a floor
	// and a cap by level: "... At least 1.0 credit needs to be at the 400 level,
	// and no more than 1.5 credits from the 200 level".
	complListLevelRulesRe = regexp.MustCompile(`(?i)^a\s+minimum\s+of\s+(\d+(?:\.\d+)?)\s+credits?\s+from\s+the\s+following\s+list\s+that\s+have\s+not\s+been\s+used\s+to\s+fulfil+\s+the\s+above\s+requirements\.\s+at\s+least\s+(\d+(?:\.\d+)?)\s+credits?\s+needs?\s+to\s+be\s+at\s+the\s+([1-4]00)\s*-?\s*level,?\s+and\s+no\s+more\s+than\s+(\d+(?:\.\d+)?)\s+credits?\s+from\s+the\s+([1-4]00)\s*-?\s*level$`)
	// complCountTowardsRe matches a note adding an earlier item's extra courses
	// to this one: "In that case, the additional courses will count towards the
	// Other Geosciences"; complChooseMoreRe the sentence before it: "You may
	// choose more than 2.5 credits from the Additional Foundation list".
	complCountTowardsRe = regexp.MustCompile(`(?i)^in\s+that\s+case,\s+the\s+additional\s+courses\s+will\s+count\s+towards?\s+the\s+(.+?)$`)
	complChooseMoreRe   = regexp.MustCompile(`(?i)^you\s+may\s+choose\s+more\s+than\s+\d+(?:\.\d+)?\s+credits?\s+from\s+the\s+(.+?)\s+list$`)
	// complLevelFloorFromListRe matches a pool at a level over the item's own
	// lists with a floor on one: "2.0 credits at the 300+ level (at least 1.0
	// credit must be from Group 1) from:".
	complLevelFloorFromListRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+at\s+the\s+([1-4])00\+\s+level\s+\(\s*at\s+least\s+(\d+(?:\.\d+)?)\s+credits?\s+must\s+be\s+from\s+(` + complListRefPat + `)\s*\)\s+from\s*:?$`)
	// complHeadingAmountRe matches a heading's amount: "Additional Foundation
	// Geosciences (2.5 credits):".
	complHeadingAmountRe = regexp.MustCompile(`(?i)\(\s*(\d+(?:\.\d+)?)\s+credits?\s*\)\s*:?\s*(?:\*\*)?\s*$`)
)

// listPairCaps returns the caps on the "/" pairs of the lists names (each
// defined once), or false when their members don't read as a pool. The amount
// is 0.5 so the grammar doesn't read the members as all required.
func listPairCaps(names []string, ls compListRefs) ([]*types.ReqNode, bool) {
	var members []string
	for _, name := range names {
		l := ls.byName(name)
		if l == nil || len(l.defs) != 1 {
			return nil, false
		}
		members = append(members, strings.TrimSuffix(strings.TrimSpace(l.defs[0].members), "."))
	}
	all := strings.Join(members, ", ")
	n, _ := parseCompletionText("0.5 credit from: " + all)
	if pool, caps := poolCaps(n); isPool(pool) {
		return caps, true
	}
	return nil, !strings.Contains(all, "/") // no pairs to cap
}

// headingAmount returns the amount of the heading row's lead sits under.
func (a *compAssembler) headingAmount(row *compRow) (float64, bool) {
	found, amount := false, 0.0
	for b, r := range a.leadRows {
		if r != row || b.heading == nil {
			continue
		}
		m := complHeadingAmountRe.FindStringSubmatch(b.heading.text)
		if m == nil {
			return 0, false
		}
		v, _ := strconv.ParseFloat(m[1], 64)
		if found && v != amount {
			return 0, false
		}
		found, amount = true, v
	}
	return amount, found
}

// categoryCaps reads an item over its own lists with a cap or floor on each, or
// level rules. A note letting an earlier item's extra courses count here adds
// its lists to the pool: exclusive allocation leaves only the extra ones.
func (a *compAssembler) categoryCaps(row *compRow, ls compListRefs) bool {
	t, put, ok := rowText(row)
	if !ok {
		return false
	}
	t = strings.TrimSpace(strings.TrimSuffix(t, "."))
	names := a.ownLists(row, ls)
	if len(names) < 2 {
		return false
	}
	limits := func(v float64, caps []*types.ReqNode) {
		for _, c := range caps {
			a.rowUnder(row, &types.Requirement{Kind: types.RequirementLimit, Req: c, Raw: rawHead(row.r)})
		}
		for _, name := range names {
			a.rowUnder(row, &types.Requirement{Kind: types.RequirementLimit, Req: &types.ReqNode{Type: types.NodeCredits, Lists: []string{name}, MaxAmount: maxAmount(v)}, Raw: rawHead(row.r)})
		}
	}
	if m := complSameCategoryCapRe.FindStringSubmatch(t); m != nil {
		amount, _ := strconv.ParseFloat(m[1], 64)
		v, _ := strconv.ParseFloat(m[2], 64)
		caps, ok := listPairCaps(names, ls)
		if !ok || v*float64(len(names)) < amount-1e-9 {
			return false
		}
		put(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Lists: names})
		limits(v, caps)
		return true
	}
	if m := complFloorCapEachRe.FindStringSubmatch(t); m != nil {
		amount, ok := a.headingAmount(row)
		each, _ := strconv.ParseFloat(m[1], 64)
		v, _ := strconv.ParseFloat(m[2], 64)
		if !ok || each*float64(len(names)) > amount+1e-9 || v*float64(len(names)) < amount-1e-9 {
			return false
		}
		caps, ok := listPairCaps(names, ls)
		if !ok || !a.spread(row, put, amount, len(names), each, names, nil, nil, ls) {
			return false
		}
		limits(v, caps)
		return true
	}
	if m := complLevelFloorFromListRe.FindStringSubmatch(t); m != nil {
		// the floor's list is one of the item's; the pool is at the level already
		amount, _ := strconv.ParseFloat(m[1], 64)
		lo, _ := strconv.Atoi(m[2])
		floor, _ := strconv.ParseFloat(m[3], 64)
		on := ls.resolve(listRefNames(m[4]))
		caps, ok := listPairCaps(names, ls)
		if !ok || len(on) != 1 || !slices.Contains(names, on[0]) || floor > amount {
			return false
		}
		var levels []int
		for l := lo * 100; l <= 400; l += 100 {
			levels = append(levels, l)
		}
		put(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Levels: levels, Lists: names})
		for _, c := range caps {
			a.rowUnder(row, &types.Requirement{Kind: types.RequirementLimit, Req: c, Raw: rawHead(row.r)})
		}
		a.rowUnder(row, &types.Requirement{Kind: types.RequirementConstraint, Req: &types.ReqNode{Type: types.NodeCredits, Amount: floor, Lists: on}, Raw: rawHead(row.r)})
		return true
	}
	if m := complListLevelRulesRe.FindStringSubmatch(t); m != nil {
		amount, _ := strconv.ParseFloat(m[1], 64)
		floor, _ := strconv.ParseFloat(m[2], 64)
		hi, _ := strconv.Atoi(m[3])
		v, _ := strconv.ParseFloat(m[4], 64)
		lo, _ := strconv.Atoi(m[5])
		caps, ok := listPairCaps(names, ls)
		if !ok {
			return false
		}
		pool := slices.Clone(names)
		var route, note string
		for _, s := range a.programRoutes {
			if m := complCountTowardsRe.FindStringSubmatch(strings.TrimSuffix(complClean(s), ".")); m != nil && strings.EqualFold(m[1], row.r.Section) {
				route = s
			}
		}
		for _, x := range a.notes {
			if c := complChooseMoreRe.FindStringSubmatch(strings.TrimSuffix(x, ".")); c != nil && route != "" {
				for _, r := range a.items {
					if r != row && strings.EqualFold(r.r.Section, c[1]) {
						if more := a.ownLists(r, ls); more != nil && note == "" {
							note, pool = x, append(pool, more...)
						}
					}
				}
			}
		}
		put(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Lists: pool})
		for _, c := range caps {
			a.rowUnder(row, &types.Requirement{Kind: types.RequirementLimit, Req: c, Raw: rawHead(row.r)})
		}
		a.rowUnder(row, &types.Requirement{Kind: types.RequirementConstraint, Req: &types.ReqNode{Type: types.NodeCredits, Amount: floor, Levels: []int{hi}}, Raw: rawHead(row.r)})
		a.rowUnder(row, &types.Requirement{Kind: types.RequirementLimit, Req: &types.ReqNode{Type: types.NodeCredits, Levels: []int{lo}, MaxAmount: maxAmount(v)}, Raw: rawHead(row.r)})
		if note != "" {
			addRaw(row, note+". "+route)
			a.notes = slices.DeleteFunc(a.notes, func(x string) bool { return x == note })
			a.programRoutes = slices.DeleteFunc(a.programRoutes, func(x string) bool { return x == route })
		}
		return true
	}
	return false
}

// complCombinationCapRe matches a pool over the lists below with a cap on one:
// "An additional 3.0 credits from any combination of courses listed in the
// three groups below, with no more than 1.0 credit from Group C".
var complCombinationCapRe = regexp.MustCompile(`(?i)^(?:an\s+)?additional\s+(\d+(?:\.\d+)?)\s+credits?\s+from\s+any\s+combination\s+of\s+courses\s+listed\s+in\s+the\s+(two|three|four|five|\d)\s+groups\s+below\*?,?\s+with\s+no\s+more\s+than\s+(\d+(?:\.\d+)?)\s+credits?\s+from\s+(` + complListRefPat + `)(?::\s+[^,;]+)?$`)

// combinationCap reads a complCombinationCapRe item: the pool of the lists
// below and a limit on the one it caps. A note that lets some students take
// less (complOnlyNeedRe) adds the smaller pool with the note, Review for them.
func (a *compAssembler) combinationCap(row *compRow, ls compListRefs) bool {
	n := row.r.Req
	var alts []*types.ReqNode
	if n != nil && n.Type == types.NodeOr && len(n.Items) > 1 {
		n, alts = n.Items[0], n.Items[1:]
	}
	var only *types.ReqNode
	if n != nil && n.Type == types.NodeAnd && len(n.Items) == 2 && n.Items[1].Type == types.NodeText && complOnlyNeedRe.MatchString(n.Items[1].Text) {
		n, only = n.Items[0], n.Items[1]
	}
	if n == nil || n.Type != types.NodeText || n.Kind != types.TextOther {
		return false
	}
	m := complCombinationCapRe.FindStringSubmatch(strings.TrimSuffix(complClean(n.Text), "."))
	if m == nil {
		return false
	}
	names := a.listsBelow(row, ls)
	capped := ls.resolve(listRefNames(m[4]))
	if len(names) != countOf(m[2]) || len(capped) != 1 || !slices.Contains(names, capped[0]) {
		return false
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	v, _ := strconv.ParseFloat(m[3], 64)
	caps, ok := listPairCaps(names, ls)
	if !ok {
		return false
	}
	body := &types.ReqNode{Type: types.NodeCredits, Amount: amount, Lists: names}
	if only != nil {
		less, _ := strconv.ParseFloat(complOnlyNeedRe.FindStringSubmatch(only.Text)[1], 64)
		if less <= 0 || less >= amount {
			return false
		}
		smaller := cloneNode(body)
		smaller.Amount = less
		body = orNode(body, andNode(smaller, only))
	}
	row.r.Req = orNode(append([]*types.ReqNode{body}, alts...)...)
	for _, c := range caps {
		a.rowUnder(row, &types.Requirement{Kind: types.RequirementLimit, Req: c, Raw: rawHead(row.r)})
	}
	a.rowUnder(row, &types.Requirement{Kind: types.RequirementLimit, Req: &types.ReqNode{Type: types.NodeCredits, Lists: capped, MaxAmount: maxAmount(v)}, Raw: rawHead(row.r)})
	return true
}

var (
	// complSameTraditionRe matches a department pool with a floor in the
	// student's tradition: "... at least 0.5 credit of which must be in the same
	// tradition as was chosen from #3 above".
	complSameTraditionRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+from\s+([1-4])00\+\s+level\s+department\s+for\s+the\s+study\s+of\s+religion\s+courses,\s+(at\s+least\s+(\d+(?:\.\d+)?)\s+credits?\s+of\s+which\s+must\s+be\s+in\s+the\s+same\s+tradition\s+as\s+was\s+chosen\s+from\s+#\d+\s+above)$`)
	// complFocusListsRe matches the text the grammar leaves beside an amount in
	// the student's area of focus: "in your area of focus selected from
	// Department for the Study of Religion courses".
	complFocusListsRe = regexp.MustCompile(`(?i)^(in\s+your\s+area\s+of\s+focus)\s+selected\s+from\s+department\s+for\s+the\s+study\s+of\s+religion\s+courses$`)
	// complDSRDesignatorsRe matches another item's list of the department's
	// designators: "Department for the Study of Religion courses carrying any of
	// the following designators: RLG, JAR, JCR".
	complDSRDesignatorsRe = regexp.MustCompile(`(?i)\bdepartment\s+for\s+the\s+study\s+of\s+religion\s+courses\s+carrying\s+any\s+of\s+the\s+following\s+designators:\s*` + complAliasDesigListPat + `\s*\.?\s*$`)
)

// traditionLists reads an item over its lists of each tradition's (or area of
// focus's) courses, where the student chooses which counts: the lists are the
// pool, and the choice is content text beside them.
func (a *compAssembler) traditionLists(row *compRow, ls compListRefs) bool {
	names := a.ownLists(row, ls)
	if len(names) < 2 {
		return false
	}
	n := row.r.Req
	if n != nil && n.Type == types.NodeOr && len(n.Items) > 1 && !slices.ContainsFunc(n.Items[1:], func(it *types.ReqNode) bool { return it.Type != types.NodeText || it.Kind != types.TextPermission }) {
		n = n.Items[0] // or(body, a route)
	}
	if n != nil && n.Type == types.NodeAnd && len(n.Items) == 2 && n.Items[1].Type == types.NodeText && n.Items[1].Kind == types.TextOther {
		c := n.Items[0]
		m := complFocusListsRe.FindStringSubmatch(strings.TrimSuffix(complClean(n.Items[1].Text), "."))
		if m == nil || c.Type != types.NodeCredits || c.Amount <= 0 || c.Levels == nil || c.Courses != nil || c.Designators != nil || c.Lists != nil || c.Items != nil {
			return false
		}
		c.Lists = names
		n.Items[1] = textNode(types.TextContent, m[1])
		return true
	}
	t, put, ok := rowText(row)
	if !ok {
		return false
	}
	t = strings.TrimSpace(strings.TrimSuffix(t, "."))
	levelsFrom := func(d string) []int {
		lo, _ := strconv.Atoi(d)
		var levels []int
		for l := lo * 100; l <= 400; l += 100 {
			levels = append(levels, l)
		}
		return levels
	}
	if m := complSameTraditionRe.FindStringSubmatch(t); m != nil {
		var desigs []string
		for _, l := range a.lines {
			if d := complDSRDesignatorsRe.FindStringSubmatch(complClean(l)); d != nil {
				if desigs != nil {
					return false
				}
				desigs = desigRe.FindAllString(d[1], -1)
			}
		}
		if desigs == nil {
			return false
		}
		amount, _ := strconv.ParseFloat(m[1], 64)
		floor, _ := strconv.ParseFloat(m[4], 64)
		put(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Designators: desigs, Levels: levelsFrom(m[2])})
		a.rowUnder(row, &types.Requirement{Kind: types.RequirementConstraint, Req: andNode(&types.ReqNode{Type: types.NodeCredits, Amount: floor, Lists: names}, textNode(types.TextContent, m[3])), Raw: rawHead(row.r)})
		return true
	}
	return false
}

// complCourseGroupsRe matches the heading over the lists of a unit's courses:
// "**Course Groups**".
var complCourseGroupsRe = regexp.MustCompile(`(?i)^\W*course\s+groups\W*$`)

// remainingFromLists reads remainingFrom's item when the lists below it, under
// a "Course Groups" heading, are the unit's courses: the pool of a union of
// the item's codes and the lists, or the unit's courses as content text.
func (a *compAssembler) remainingFromLists(row *compRow, ls compListRefs) bool {
	r, m, amount := a.remainingFromItem()
	if r != row {
		return false
	}
	names := a.listsBelow(row, ls)
	if len(names) < 2 {
		return false
	}
	first := slices.IndexFunc(a.lines, func(l string) bool { return rawWords(l) == rawWords(rawHead(row.r)) })
	heads := 0
	for _, l := range a.lines[first+1:] {
		if complCourseGroupsRe.MatchString(l) {
			heads++
		}
	}
	if heads != 1 {
		return false
	}
	// every list is under the heading: none is defined before it
	at := first + 1 + slices.IndexFunc(a.lines[first+1:], func(l string) bool { return complCourseGroupsRe.MatchString(l) })
	for _, name := range names {
		if l := ls.byName(name); l == nil || len(l.defs) != 1 || l.defs[0].block == nil || l.defs[0].block.line < at {
			return false
		}
	}
	codes := dedupe(codeRe.FindAllString(m[1], -1))
	name, _, ok := a.addUnion(row, []*types.ReqNode{{Type: types.NodeCredits, Courses: codes}, {Type: types.NodeCredits, Lists: names}}, ls)
	if !ok {
		return false
	}
	row.r.Req = orNode(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Lists: []string{name}},
		andNode(&types.ReqNode{Type: types.NodeCredits, Amount: amount}, textNode(types.TextContent, m[2])))
	return true
}

// complListsOrWebsiteRe matches a pool of two named lists and a list kept on a
// website: "2.0 credits from Group A and/or Group B and/or from the additional
// list of approved courses as listed on the program website".
var complListsOrWebsiteRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+(?:additional\s+)?credits?\s+from\s+(.+?)\s+and/or\s+(.+?)\s+and/or\s+(from\s+the\s+additional\s+list\s+of\s+approved\s+courses\b.*?\s+as\s+listed\s+on\s+the\s+.+?\s+website\b.*)$`)

// websiteLists reads a complListsOrWebsiteRe item: the pool of the two lists,
// or the amount with the website's list as content text (the catalogue doesn't
// hold it), and the caps on the lists' "/" pairs.
func (a *compAssembler) websiteLists(row *compRow, ls compListRefs) bool {
	t, put, ok := rowText(row)
	if !ok {
		return false
	}
	m := complListsOrWebsiteRe.FindStringSubmatch(strings.TrimSpace(strings.TrimSuffix(t, ".")))
	if m == nil {
		return false
	}
	var names []string
	for _, title := range []string{m[2], m[3]} {
		l := ls.byName(strings.TrimSpace(title))
		if l == nil || l.bad || !l.titled {
			return false
		}
		names = append(names, l.list.Name)
	}
	caps, ok := listPairCaps(names, ls)
	if !ok {
		return false
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	put(orNode(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Lists: names},
		andNode(&types.ReqNode{Type: types.NodeCredits, Amount: amount}, textNode(types.TextContent, m[4]))))
	for _, c := range caps {
		a.rowUnder(row, &types.Requirement{Kind: types.RequirementLimit, Req: c, Raw: rawHead(row.r)})
	}
	return true
}

// complHalfFromListRe matches a list that may stand in for one of an item's
// half courses: "up to one half course from Group H".
var complHalfFromListRe = regexp.MustCompile(`(?i)^up\s+to\s+one\s+half\s+course\s+from\s+(` + complListRefPat + `)$`)

// halfCourseFromList reads or(the item's half courses, a complHalfFromListRe
// clause): the clause is all but a half course from the courses and one from
// the list, with caps on the courses' "/" pairs.
func (a *compAssembler) halfCourseFromList(row *compRow, ls compListRefs) bool {
	n := row.r.Req
	if n == nil || n.Type != types.NodeOr || len(n.Items) != 2 || n.Items[1].Type != types.NodeText || n.Items[1].Kind != types.TextOther {
		return false
	}
	m := complHalfFromListRe.FindStringSubmatch(strings.TrimSuffix(complClean(n.Items[1].Text), "."))
	if m == nil {
		return false
	}
	names := ls.resolve(listRefNames(m[1]))
	if len(names) != 1 {
		return false
	}
	strict := n.Items[0]
	if strict.Type != types.NodeAnd || len(strict.Items) < 2 {
		return false
	}
	var codes []string
	var caps []*types.ReqNode
	for _, part := range strict.Items {
		var alt []string
		switch {
		case part.Type == types.NodeCourse:
			alt = []string{part.Code}
		case part.Type == types.NodeOr && !slices.ContainsFunc(part.Items, func(x *types.ReqNode) bool { return x.Type != types.NodeCourse }):
			for _, x := range part.Items {
				alt = append(alt, x.Code)
			}
			caps = append(caps, &types.ReqNode{Type: types.NodeCredits, Courses: alt, MaxAmount: maxAmount(0.5)})
		default:
			return false
		}
		for _, c := range alt {
			if courseWeight(c) != 0.5 {
				return false
			}
		}
		codes = append(codes, alt...)
	}
	amount := 0.5 * float64(len(strict.Items))
	row.r.Req = orNode(strict, andNode(&types.ReqNode{Type: types.NodeCredits, Amount: amount - 0.5, Courses: dedupe(codes)}, &types.ReqNode{Type: types.NodeCredits, Amount: 0.5, Lists: names}))
	for _, c := range caps {
		a.rowUnder(row, &types.Requirement{Kind: types.RequirementLimit, Req: c, Raw: rawHead(row.r)})
	}
	return true
}

// unreachableCaps drops the limits under a pool that leave it short of its
// amount, so it can never be Met: the "/" or cap can't mean what they say. The
// pool becomes and(pool, ambiguity text): Review, never NotMet.
func (a *compAssembler) unreachableCaps(roots []*compRow, lists []*types.CourseList) {
	byName := map[string]*types.CourseList{}
	for _, l := range lists {
		byName[l.Name] = l
	}
	var walk func(rs []*compRow)
	walk = func(rs []*compRow) {
		for _, r := range rs {
			walk(r.children)
			n := r.r.Req
			if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeCredits || n.Amount == 0 ||
				n.Designators != nil || n.Levels != nil || n.Breadth != nil || n.Requirements != nil || n.Exclude != nil {
				continue
			}
			codes := n.Courses
			if n.Lists != nil {
				cs, ok := a.listCodes(n.Lists, byName)
				if !ok || n.Courses != nil {
					continue
				}
				codes = cs
			}
			codes = dedupe(slices.Clone(codes))
			if codes == nil {
				continue
			}
			var caps []*compRow
			used := map[string]bool{}
			reach := 0.0
			for _, k := range r.children {
				l := k.r.Req
				if k.r.Kind != types.RequirementLimit || l == nil || l.Type != types.NodeCredits || l.Courses == nil ||
					l.MaxAmount == nil || l.Designators != nil || l.Levels != nil || l.Lists != nil || l.Transfer ||
					slices.ContainsFunc(l.Courses, func(c string) bool { return !slices.Contains(codes, c) || used[c] }) {
					continue
				}
				caps = append(caps, k)
				for _, c := range l.Courses {
					used[c] = true
				}
				reach += min(weightOf(l.Courses), *l.MaxAmount)
			}
			if caps == nil {
				continue
			}
			for _, c := range codes {
				if !used[c] {
					reach += courseWeight(c)
				}
			}
			// only when the caps are why: short without them, the list is partial
			if reach >= n.Amount-1e-9 || weightOf(codes) < n.Amount-1e-9 {
				continue
			}
			r.children = slices.DeleteFunc(r.children, func(k *compRow) bool { return slices.Contains(caps, k) })
			r.r.Req = andNode(n, textNode(types.TextAmbiguity, "the caps on the pool's courses leave less than its amount, so they may not apply as written"))
		}
	}
	walk(roots)
}
