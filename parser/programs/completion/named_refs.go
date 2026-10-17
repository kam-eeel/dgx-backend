package completion

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/parser/grammar"
	"degree-explorer/parser/programs"
	"degree-explorer/types"
)

// References to named lists: rows of text that draw on lists by name are read
// again, as pools, unions and allocations over the lists.

// halfCoursesCapRe matches a cap on a list counted in half courses, with
// its credits: "The total number of half courses from Group I must not exceed
// 3 (1.5 credits)".
var halfCoursesCapRe = regexp.MustCompile(`(?i)^the\s+total\s+number\s+of\s+(?:half\s+)?courses\s+from\s+(` + listRefPat + `)(?:\s*\(\s*including\s+[^()]*\))?\s+must\s+not\s+exceed\s+\w+\s*\(\s*` + grammar.AmountPat + `\s+credits?\s*\)$`)

// designatorPhraseRe matches a floor's designators in words: "carrying
// the CHC/JCA/JCR designator".
var designatorPhraseRe = regexp.MustCompile(`(?i)\s+(?:must\s+(?:have|carry)|carrying|having|with)\s+(?:the\s+|an?\s+)?(` + grammar.DesigPat + `(?:\s*/\s*` + grammar.DesigPat + `)*)\s+designators?\b`)

// excludingLevelRe matches "excluding 100-level courses".
var excludingLevelRe = regexp.MustCompile(`(?i)^excluding\s+(` + listLevelsPat + `)\s*-?\s*level\s+courses$`)

var (
	// fromListRefRe matches a reference after "from" or "in", with what
	// may come around it: "from either Group A, B or C".
	fromListRefRe = regexp.MustCompile(`(?i)\s*\b(?:from|in)\s+(?:among\s+|either\s+|any\s+of\s+)?(?:the\s+)?(?:courses\s+(?:listed\s+)?in\s+(?:the\s+)?)?(` + listRefPat + `)(?:\s+in\s+(` + listRefPat + `))?(?:\s+courses)?(?:\s+(?:below|above|listed\s+below))?(?:\s*\(([^()]*)\))?`)
	// listParenDropRe matches a bracket after a reference that restates
	// it, inside the brackets: "any Cluster".
	listParenDropRe = regexp.MustCompile(`(?i)^(?:any(?:\s+of)?\s+)?(?:clusters?|sub-?groups?)\b[a-z\s,]*$`)
	// excludingCodesRe matches the codes a bracket after a reference leaves
	// out: "excluding PSY304Y1, and PSY406Y1".
	excludingCodesRe = regexp.MustCompile(`(?i)(?:^|[,;]?\s+)excluding\s+((?:` + grammar.CodePat + `)(?:\s*(?:,\s*(?:and\b|or\b)?|/|\band\b|\bor\b)\s*(?:` + grammar.CodePat + `))*)\s*$`)
	// listNoteParenRe matches a remark in brackets, inside them: "see below
	// for definitions".
	listNoteParenRe = regexp.MustCompile(`(?i)^(?:note\b|see\b)`)
	// alreadyCountedRe matches the claims rule restated: "(excluding those
	// already counted)".
	alreadyCountedRe = regexp.MustCompile(`(?i)\s*\(?\s*excluding\s+those\s+already\s+counted\s*\)?|\s+(?:that|which)\s+(?:has|have)\s+not\s+(?:previously\s+)?been\s+(?:used|taken)\s+to\s+fulfill\s+(?:the\s+above|program)\s+requirements\b`)
	// anyCoursesAmountRe matches "Any 5 courses (2.5 credits)": the amount
	// is the one in brackets.
	anyCoursesAmountRe = regexp.MustCompile(`(?i)^any\s+(?:\d+\s+|one\s+)?courses?\s*\(\s*(` + grammar.NumPat + `\s+credits?)\s*\)`)
	// listPoolWordRe matches a word a pool drawn from lists may have
	// besides the lists: "credits".
	listPoolWordRe = regexp.MustCompile(`(?i)^(?:an?|one|additional|more|further|credits?|courses?|of|in|total|at|the|levels?|any|selected|chosen|taken|from|(?-i:[A-Z]{3}(?:/[A-Z]{3})*/?)|` + grammar.NumPat + `|[-/+]*(?:` + grammar.LevelPat + `[-/+]*)+(?:-?level)?|-level|\+|-|/)$`)
	// listTailRe matches where a floor or cap clause after a pool begins:
	// ", including at least".
	listTailRe = regexp.MustCompile(`(?i)(?:,\s*|\s+|;\s*|\(\s*)(?:(?:and\s+|with\s+|but\s+)?(?:(?:including|of\s+which)(?:\s+` + grammar.AtLeastPat + `|\s+a\s+minimum\s+of)?|` + grammar.NoMoreThanPat + `|only\s+a\s+maximum|a\s+maximum|up\s+to|at\s+most|` + grammar.AtLeastPat + `)\b)`)
	// listFillerRe matches pool words that say nothing about its courses:
	// " in total".
	listFillerRe = regexp.MustCompile(`(?i)\s+(?:of\s+courses|in\s+total|selected|chosen|taken|of\s+which)\b`)
	// levelOrRe matches "300- or 400-level": one level phrase.
	levelOrRe = regexp.MustCompile(`(?i)\b(` + grammar.LevelPat + `)\s*-?\s+(?:or|and)\s+(` + grammar.LevelPat + `)\b`)
	// listCapLeadRe matches a clause that caps: "no more than 1.5 credits
	// from Group B".
	listCapLeadRe = regexp.MustCompile(`(?i)^(?:and\s+|with\s+|but\s+)?(?:only\s+)?(?:` + grammar.NoMoreThanPat + `|a\s+maximum(?:\s+of)?|up\s+to|at\s+most)\s+`)
	// listFloorLeadRe matches the lead of a floor clause: "including at
	// least".
	listFloorLeadRe = regexp.MustCompile(`(?i)^(?:and\s+|with\s+)?(?:including|of\s+which)?\s*(?:` + grammar.AtLeastPat + `|a\s+minimum\s+of)?\s*`)
	// listModalRe matches "must be" inside a clause, which says nothing
	// about the courses.
	listModalRe = regexp.MustCompile(`(?i)\s+(?:must|can|may|should)\s+(?:be|come)\b(?:\s+taken)?|\s+(?:must|can|may)\s+be\s+(?:counted|used)\b`)
)

// listRefPool reads text drawing an amount from lists ("1.5 credits from
// Group A") into a credits node with the lists filter; with optional, also
// text naming no list. It returns bracketed notes; ok is false if any is unread.
func listRefPool(text string, ls compListRefs, optional bool) (n *types.ReqNode, notes []string, ok bool) {
	return listRefPoolOf(text, ls, optional, nil)
}

// listRefPoolOf is listRefPool for text whose lists names are already known,
// the reference taken out of it.
func listRefPoolOf(text string, ls compListRefs, optional bool, names []string) (n *types.ReqNode, notes []string, ok bool) {
	t := strings.TrimSpace(alreadyCountedRe.ReplaceAllString(text, ""))
	if names == nil {
		if u, name, ok := ls.titleRef(t); ok {
			// "from Pre-1800 British Literature courses": a list named by its title
			t, names = u, []string{name}
		}
	}
	locs := fromListRefRe.FindAllStringSubmatchIndex(t, -1)
	if names != nil && len(locs) > 0 {
		return nil, nil, false
	}
	if len(locs) > 1 || len(locs) == 0 && names == nil && !optional {
		return nil, nil, false
	}
	var excludeLevels []int
	var excludeCodes []string
	if len(locs) == 1 && names == nil {
		m := locs[0]
		keys := listRefNames(t[m[2]:m[3]])
		if m[4] >= 0 {
			// "Group 1 in Cluster A": the cluster, which is in the group
			inner := listRefNames(t[m[4]:m[5]])
			if len(keys) != 1 || len(inner) == 0 || slices.ContainsFunc(inner, func(k string) bool { return ls[k] == nil || ls[k].parent != keys[0] }) {
				return nil, nil, false
			}
			keys = inner
		}
		if names = ls.resolve(keys); names == nil {
			return nil, nil, false
		}
		if m[6] >= 0 {
			p := strings.TrimSpace(t[m[6]:m[7]])
			if e := excludingCodesRe.FindStringSubmatchIndex(p); e != nil {
				// "(any Cluster, excluding PSY304Y1, ...)": codes left out
				excludeCodes = grammar.CodeRe.FindAllString(p[e[2]:e[3]], -1)
				p = strings.TrimSpace(strings.TrimRight(p[:e[0]], ",; "))
			}
			switch {
			case p == "" && excludeCodes != nil:
			case listParenDropRe.MatchString(p):
			case listNoteParenRe.MatchString(p):
				notes = append(notes, grammar.CleanNote(p))
			case len(keys) == 1 && ls[keys[0]].list.Title != "" && strings.Contains(strings.ToLower(ls[keys[0]].list.Title), strings.ToLower(p)):
				// "Group 2 (Value Theory)": the list's title
			default:
				if lv := excludingLevelRe.FindStringSubmatch(p); lv != nil && len(lv[0]) == len(p) {
					excludeLevels = grammar.ParseLevels(lv[1] + "-level")
					break
				}
				return nil, nil, false
			}
		}
		t = t[:m[0]] + " " + t[m[1]:]
	}
	t = designatorPhraseRe.ReplaceAllString(t, " in $1 courses")
	t = levelOrRe.ReplaceAllString(t, "$1/$2")
	t = strings.TrimSpace(listFillerRe.ReplaceAllString(listModalRe.ReplaceAllString(t, ""), ""))
	if m := anyCoursesAmountRe.FindStringSubmatchIndex(t); m != nil {
		t = t[m[2]:m[3]] + t[m[1]:]
	}
	if grammar.CodeRe.MatchString(t) || strings.ContainsAny(t, "():;") {
		return nil, nil, false
	}
	for _, w := range strings.Fields(strings.NewReplacer(",", " ").Replace(t)) {
		if !listPoolWordRe.MatchString(w) {
			return nil, nil, false
		}
	}
	p, _ := parseCompletionText(t)
	if p == nil || p.Type != types.NodeCredits || p.Amount <= 0 || p.Courses != nil || p.Items != nil || p.MaxAmount != nil || p.Lists != nil || p.Breadth != nil {
		return nil, nil, false
	}
	if names == nil && p.Designators == nil && p.Levels == nil {
		return nil, nil, false // "1.0 credit": no pool at all
	}
	p.Lists = names
	p.Exclude = excludeCodes
	if excludeLevels != nil {
		levels := []int{100, 200, 300, 400}
		if p.Levels != nil {
			levels = p.Levels
		}
		p.Levels = slices.DeleteFunc(slices.Clone(levels), func(l int) bool { return slices.Contains(excludeLevels, l) })
	}
	return p, notes, true
}

// listRefRow reads row again when its body is text that draws on lists by
// name, and returns the rows it becomes: itself, or for a text constraint the
// floors and caps it states.
func (a *Assembler) listRefRow(row *compRow, ls compListRefs) []*compRow {
	if rows, ok := a.spreadRow(row, ls); ok {
		return rows // spreads, choices and shares among lists
	}
	n := row.r.Req
	if n == nil {
		return []*compRow{row}
	}
	switch row.r.Kind {
	case types.RequirementAllocation:
		a.poolListAlts(row, ls)
		n = row.r.Req
		text := n
		if n.Type == types.NodeOr && len(n.Items) > 1 && !slices.ContainsFunc(n.Items[1:], func(it *types.ReqNode) bool {
			return it.Type != types.NodeText || it.Kind != types.TextPermission
		}) {
			text = n.Items[0] // or(text, a route)
		}
		if text.Type == types.NodeText && text.Kind == types.TextOther {
			if text != n && a.additionalLists(row, text.Text, n.Items[1:], ls) || text == n && a.additionalLists(row, text.Text, nil, ls) {
				return nil // Req 1's own pool
			}
			if rows, ok := a.oneOffTextRow(row, text, ls); ok {
				return rows
			}
			t := rewriteOneOffListWordings(text.Text)
			body, rules, notes, ok := a.listRefAlloc(row, t, ls)
			if !ok {
				body, rules, ok = a.unionList(row, t, ls)
			}
			if !ok {
				body, rules, ok = a.reqRefAlloc(row, t, ls)
			}
			if ok {
				if text == n {
					row.r.Req = body
				} else {
					n.Items[0] = body
				}
				for _, r := range rules {
					a.rowUnder(row, r)
				}
				row.r.Notes = append(row.r.Notes, notes...)
				for _, perm := range ls.perms(body.Lists) {
					// the list's permission route, on the row
					row.r.Req = programs.OrNode(row.r.Req, TextNode(types.TextPermission, perm))
				}
			}
			return []*compRow{row}
		}
		if n.Type == types.NodeAnd && len(n.Items) == 2 && n.Items[0].Type == types.NodeCredits && n.Items[1].Type == types.NodeText && fromListRefRe.MatchString(n.Items[1].Text) {
			// "0.5 credit at the 400-level from Group B", read as and(credits, text
			// "from Group B"): the sentence again
			if sents := SplitSentences(Clean(RawHead(row.r))); len(sents) == 1 {
				if body, rules, notes, ok := a.listRefAlloc(row, sents[0], ls); ok {
					row.r.Req = body
					for _, r := range rules {
						a.rowUnder(row, r)
					}
					row.r.Notes = append(row.r.Notes, notes...)
					return []*compRow{row}
				}
			}
		}
		if n.Type == types.NodeAnd && len(n.Items) == 2 && n.Items[1].Type == types.NodeText && fromTextRe.MatchString(n.Items[1].Text) {
			// "1.0 ENG credits from Pre-1800 British Literature courses", read as
			// and(credits{ENG}, text): the item's whole text again
			if body, rules, notes, ok := a.listRefAlloc(row, ItemLabelRe.ReplaceAllString(Clean(RawHead(row.r)), ""), ls); ok && rules == nil {
				row.r.Req = body
				row.r.Notes = append(row.r.Notes, notes...)
				return []*compRow{row}
			}
		}
		if n.Type == types.NodeAnd {
			// "2.5 credits from Group A below" and a note: each part on its own
			for i, it := range n.Items {
				if it.Type != types.NodeText || it.Kind != types.TextOther {
					continue
				}
				if body, rules, notes, ok := a.listRefAlloc(row, it.Text, ls); ok && rules == nil {
					n.Items[i] = body
					row.r.Notes = append(row.r.Notes, notes...)
				}
			}
			// "Students taking ENV421Y1 ... only need to take 2.0 credits ...": a
			// smaller amount for some, Review for them, not text for everyone
			if len(n.Items) == 2 && n.Items[0].Type == types.NodeCredits && n.Items[1].Type == types.NodeText {
				if m := onlyNeedRe.FindStringSubmatch(n.Items[1].Text); m != nil {
					if v, _ := strconv.ParseFloat(m[1], 64); v > 0 && v < n.Items[0].Amount {
						less := grammar.CloneNode(n.Items[0])
						less.Amount = v
						row.r.Req = programs.OrNode(n.Items[0], programs.AndNode(less, n.Items[1]))
					}
				}
			}
		}
		return []*compRow{row}
	case types.RequirementConstraint:
		if n.Type != types.NodeText || n.Kind != types.TextOther {
			return []*compRow{row}
		}
		if m := halfCoursesCapRe.FindStringSubmatch(Clean(n.Text)); m != nil {
			// "The total number of half courses from Group I (including 300-level
			// courses) must not exceed 3": it counts the list's courses wherever
			// used, so it is the program's limit
			if names := ls.resolve(listRefNames(m[1])); names != nil {
				v, _ := strconv.ParseFloat(m[2], 64)
				row.r.Kind = types.RequirementLimit
				row.r.Req = &types.ReqNode{Type: types.NodeCredits, Lists: names, MaxAmount: grammar.MaxAmount(v)}
				a.programRows = append(a.programRows, row)
				return nil
			}
			return []*compRow{row}
		}
		head, tails := splitListTails(Clean(n.Text))
		firsts, notes, ok := listRefRule(head, ls, nil)
		if !ok {
			return []*compRow{row}
		}
		var out []*compRow
		for _, r := range firsts {
			r.Raw = row.r.Raw
			out = append(out, &compRow{r: r})
		}
		first := firsts[0]
		for _, t := range tails {
			var within *types.ReqNode
			if len(firsts) == 1 && first.Kind == types.RequirementConstraint && !listCapLeadRe.MatchString(t) {
				within = first.Req
			}
			rs, more, ok := listRefRule(t, ls, within)
			if !ok {
				return []*compRow{row}
			}
			for _, r := range rs {
				r.Raw = row.r.Raw
				out = append(out, &compRow{r: r})
			}
			notes = append(notes, more...)
		}
		out[0].r.Notes = append(slices.Clone(row.r.Notes), notes...)
		out[0].r.Label, out[0].r.Section = row.r.Label, row.r.Section
		out[0].children = row.children
		return out
	}
	return []*compRow{row}
}

// followingListsRe matches a lead over the lists set out under it: "from
// among the following groups".
var followingListsRe = regexp.MustCompile(`(?i)\bfrom\s+(?:among\s+)?the\s+following\s+(?:groups|lists?)\s*:?\s*$`)

// followingLists rewrites a lead over the lists set out under it to name the
// lists whose lines are row's notes: "from Group A, Group B, Group C".
func (a *Assembler) followingLists(row *compRow, head string, ls compListRefs) string {
	loc := followingListsRe.FindStringIndex(head)
	if loc == nil {
		return head
	}
	var names []string
	for _, d := range a.listDefs {
		if name, key := listName(d.title); key != "" && slices.Contains(row.r.Notes, d.note) && ls[key] != nil {
			names = append(names, name)
		}
	}
	if names = grammar.Dedupe(names); names == nil {
		return head
	}
	return strings.TrimSpace(head[:loc[0]]) + " from " + strings.Join(names, ", ")
}

var (
	// fromColonRe matches "from: " in "2.5 credits from: Group A".
	fromColonRe = regexp.MustCompile(`(?i)\bfrom\s*:\s*`)
	// spacedSlashRe matches designators joined by a spaced slash: "ENG /JWE".
	spacedSlashRe = regexp.MustCompile(`\b([A-Z]{3})\s+/\s*([A-Z]{3})\b`)
	// refParenRe matches a bracket in a reference restating its sub-lists:
	// "(any Cluster)". Not one naming codes, which it leaves out.
	refParenRe = regexp.MustCompile(`(?i)\s*\((?:any(?:\s+of)?\s+)?(?:clusters?|sub-?groups?)\b[^()0-9]*\)`)
	// anyLevelRe matches ", at any level", which says nothing.
	anyLevelRe = regexp.MustCompile(`(?i),?\s+at\s+any\s+level\b`)
	// leadFloorRe matches an item's "At least" before its amount: what it
	// claims is at least that anyway.
	leadFloorRe = regexp.MustCompile(`(?i)^(?:` + grammar.AtLeastPat + `|a\s+minimum\s+of)\s+`)
	// fromRefCodesRe matches a pool of courses on a list: "1.0 credit (from
	// Cluster A) PSY210H1, PSY220H1".
	fromRefCodesRe = regexp.MustCompile(`(?i)^(` + grammar.NumPat + `\s+credits?)\s*\(\s*from\s+(` + listRefPat + `)\s*\)\s*(.+)$`)
	// descRefRe matches a list named after its description: "from Drama,
	// Theatre, and Performance Studies courses (Group A)".
	descRefRe = regexp.MustCompile(`(?i)\bfrom\s+([A-Z][^()]*?)\s+courses\s*\(\s*(` + listRefPat + `)\s*\)`)
	// refTitleRe matches a reference with its list's title after a colon:
	// "Group G: Cross-Listed".
	refTitleRe = regexp.MustCompile(`((?i:` + listRefPat + `))\s*:\s*([A-Z][\w ,&'’-]*)$`)
	// orFromRe matches one amount from either list: "One 0.5 credit from
	// Group A or one 0.5 credit from Group B below".
	orFromRe = regexp.MustCompile(`(?i)^(.+?)\s+from\s+(` + listRefPat + `),?\s+or\s+(.+?)\s+from\s+(` + listRefPat + `)(.*)$`)
	// amountWordsRe matches the words of an amount that don't change it:
	// "additional".
	amountWordsRe = regexp.MustCompile(`(?i)\b(?:one|an?|additional|in\s+any\s+area)\b`)
)

// titleWords reports whether every word of desc is in the title of the one
// list ref names.
func titleWords(desc, ref string, ls compListRefs) bool {
	keys := listRefNames(ref)
	if len(keys) != 1 || ls[keys[0]] == nil {
		return false
	}
	title := strings.ToLower(ls[keys[0]].list.Title)
	for _, w := range strings.FieldsFunc(strings.ToLower(desc), func(r rune) bool { return r == ' ' || r == ',' }) {
		if w != "and" && w != "&" && !strings.Contains(title, w) {
			return false
		}
	}
	return title != ""
}

// normalizeListRef rewrites the ways of writing a reference that listRefPool
// doesn't read into ones it does.
func normalizeListRef(s string, ls compListRefs) string {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), ":"))
	s = fromColonRe.ReplaceAllString(s, "from ")
	s = spacedSlashRe.ReplaceAllString(s, "$1/$2") // "ENG /JWE credit"
	s = refParenRe.ReplaceAllString(s, "")
	s = anyLevelRe.ReplaceAllString(s, "")
	if m := descRefRe.FindStringSubmatchIndex(s); m != nil && titleWords(s[m[2]:m[3]], s[m[4]:m[5]], ls) {
		s = s[:m[0]] + "from " + s[m[4]:m[5]] + s[m[1]:]
	}
	if m := refTitleRe.FindStringSubmatchIndex(s); m != nil && titleWords(s[m[4]:m[5]], s[m[2]:m[3]], ls) {
		s = s[:m[3]]
	}
	if m := orFromRe.FindStringSubmatch(s); m != nil {
		amount := func(x string) string {
			return strings.Join(strings.Fields(amountWordsRe.ReplaceAllString(x, "")), " ")
		}
		if a, b := amount(m[1]), amount(m[3]); a == b || a == b+"s" || a+"s" == b {
			s = m[1] + " from " + m[2] + " or " + m[4] + m[5]
		}
	}
	return s
}

// listRefAlloc reads an allocation's text that draws on lists by name, or a
// pool with floors and caps the grammar doesn't read, into its body and the
// floors and caps under it.
func (a *Assembler) listRefAlloc(row *compRow, text string, ls compListRefs) (body *types.ReqNode, rules []*types.Requirement, notes []string, ok bool) {
	t := normalizeListRef(Clean(text), ls)
	if m := fromRefCodesRe.FindStringSubmatch(t); m != nil {
		// "1.0 credit (from Cluster A) PSY210H1, ...": courses all on the list
		names := ls.resolve(listRefNames(m[2]))
		n, _ := parseCompletionText(m[1] + " from: " + m[3])
		if names == nil || !isPool(n) || n.Designators != nil || n.Levels != nil || slices.ContainsFunc(n.Courses, func(c string) bool { return !ls.lists(names, c) }) {
			return nil, nil, nil, false
		}
		n.Lists = names
		return n, nil, nil, true
	}
	head, tails := splitQualified(t)
	head = leadFloorRe.ReplaceAllString(a.followingLists(row, strings.TrimSpace(alreadyCountedRe.ReplaceAllString(head, "")), ls), "")
	if rest, names, below := a.belowLists(row, head, ls); below {
		body, notes, ok = listRefPoolOf(rest, ls, false, names)
	}
	if !ok {
		// the lists below, or else by name or title
		body, notes, ok = listRefPool(head, ls, false)
	}
	if !ok && tails != nil && grammar.CodeRe.MatchString(head) {
		// a pool of courses with its floors and caps written in it
		var caps []*types.ReqNode
		var more []string
		body, caps, more, ok = qualifiedPool(head)
		notes = append(notes, more...)
		for _, c := range caps {
			rules = append(rules, &types.Requirement{Kind: types.RequirementLimit, Req: c, Raw: grammar.CleanNote(text)})
		}
	} else if !ok {
		body, ok = a.listUnion(row, head, ls)
	}
	if !ok {
		return nil, nil, nil, false
	}
	if body.Lists != nil && len(body.Lists) > 1 && fromEitherRe.MatchString(head) {
		// "... from either Group A, B or C": from one of the lists, not their
		// union. A sub-list whose list is named too is part of that choice.
		var alts []*types.ReqNode
		for _, name := range body.Lists {
			if ls.within(name, body.Lists) {
				continue
			}
			alt := grammar.CloneNode(body)
			alt.Lists = []string{name}
			alts = append(alts, alt)
		}
		body = programs.OrNode(alts...)
	}
	var last *types.Requirement
	for _, tail := range tails {
		var within *types.ReqNode
		if last != nil && last.Kind == types.RequirementConstraint && ofWhichRe.MatchString(tail) {
			within = last.Req // "..., at least 1.0 credit of which must be at the 400-level"
		}
		rs, more, ok := listRefRule(normalizeListRef(tail, ls), ls, within)
		if !ok {
			return nil, nil, nil, false
		}
		for _, r := range rs {
			if r.Kind == types.RequirementLimit && r.Req.Lists != nil && slices.ContainsFunc(r.Req.Lists, func(l string) bool { return !slices.Contains(body.Lists, l) }) {
				// "...; up to 1.0 credit can be from Group B": B's share joins the
				// pool, capped
				if !upToRe.MatchString(tail) || body.Lists == nil || body.Courses != nil || body.Designators != nil || body.Levels != nil || r.Req.Designators != nil || r.Req.Levels != nil {
					return nil, nil, nil, false
				}
				body.Lists = grammar.Dedupe(append(body.Lists, r.Req.Lists...))
			}
			r.Raw = grammar.CleanNote(text)
			rules = append(rules, r)
		}
		if len(rs) == 1 {
			last = rs[0]
		}
		notes = append(notes, more...)
	}
	// "... of which 0.5 credit must be at the 400-level: ..., MSE540H1": a
	// listed course above the floor's level may be meant to count (two readings)
	var above []string
	for _, c := range body.Courses {
		if len(c) == 8 && c[3] >= '5' && c[3] <= '9' {
			above = append(above, c)
		}
	}
	for _, r := range rules {
		if r.Kind != types.RequirementConstraint || r.Req.Type != types.NodeCredits || above == nil || !slices.Contains(r.Req.Levels, 400) {
			continue
		}
		maybe := &types.ReqNode{Type: types.NodeCredits, Amount: r.Req.Amount, Courses: slices.Clone(above)}
		r.Req = programs.OrNode(r.Req, programs.AndNode(maybe, TextNode(types.TextAmbiguity, strings.Join(above, ", ")+" is above the 400-level the floor names: it may count toward it")))
	}
	return body, rules, notes, true
}

// titleRef finds in t a reference to one list named by its title ("from
// Pre-1800 British Literature courses from the list below") and returns t
// without it.
func (ls compListRefs) titleRef(t string) (rest, name string, ok bool) {
	found := 0
	for _, l := range ls {
		if !l.titled || l.bad {
			continue
		}
		title := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(l.list.Name), " courses"))
		title = strings.TrimSuffix(title, " Courses")
		// "Cultural, Postcolonial, and Transnational studies" names "Cultural,
		// Postcolonial & Transnational Studies"
		q := strings.NewReplacer(" & ", `,?\s+(?:&|and)\s+`, " and ", `,?\s+(?:&|and)\s+`).Replace(regexp.QuoteMeta(title))
		re := regexp.MustCompile(`(?i)\s*\b(?:from|in)\s+(?:courses\s+(?:from|in)\s+)?(?:the\s+)?` + q +
			`(?:\s+(?:courses?|concentration|area|group|list))?(?:\s+(?:from\s+)?(?:the\s+)?list\s+below|\s+below|\s+listed\s+below)?\b`)
		if loc := re.FindStringIndex(t); loc != nil {
			found++
			rest, name = t[:loc[0]]+" "+t[loc[1]:], l.list.Name
		}
	}
	if found != 1 {
		return "", "", false
	}
	return rest, name, true
}

// belowLists reads text pointing at the lists set out below it as drawing on
// the program-level lists defined after row, when row alone points below.
// "(list below)" names one list. ok is false when text doesn't point below.
func (a *Assembler) belowLists(row *compRow, text string, ls compListRefs) (rest string, names []string, ok bool) {
	loc := belowListsRe.FindStringIndex(text)
	if loc == nil || a.belowRows != 1 {
		return text, nil, false
	}
	line := slices.IndexFunc(a.lines, func(l string) bool { return strings.TrimSpace(l) == RawHead(row.r) })
	if line < 0 {
		return text, nil, false
	}
	var after []*compList
	for _, l := range a.sortedLists(ls) {
		first := l.defs[0]
		if first.block == nil || first.row != nil && first.row != row || first.block.line < line {
			continue
		}
		if l.bad {
			return text, nil, false
		}
		after = append(after, l)
	}
	single := listBelowParenRe.MatchString(text)
	if len(after) == 0 || single && len(after) != 1 {
		return text, nil, false
	}
	for _, l := range after {
		names = append(names, l.list.Name)
	}
	rest = strings.TrimSpace(text[:loc[0]] + " " + text[loc[1]:])
	if single {
		if m := amountOnlyRe.FindStringSubmatch(rest); m != nil {
			rest = m[1]
		}
	}
	return rest, names, true
}

// perms returns the permission routes of the named lists and the lists under
// them.
func (ls compListRefs) perms(names []string) []string {
	var out []string
	for _, l := range ls {
		if !slices.Contains(names, l.list.Name) {
			continue
		}
		if l.perm != "" {
			out = append(out, l.perm)
		}
		for _, it := range l.list.Items {
			out = append(out, ls.perms(it.Lists)...)
		}
	}
	slices.Sort(out)
	return grammar.Dedupe(out)
}

// unionList reads text the grammar reads as a union pool of courses and
// filters, which one credits node can't hold, as a list of its own the row
// draws its amount from. Caps on its "/" pairs are limits under the row.
func (a *Assembler) unionList(row *compRow, text string, ls compListRefs) (*types.ReqNode, []*types.Requirement, bool) {
	t := Clean(text)
	n, _ := parseCompletionText(t)
	if f := filterUnion(t); f != nil {
		n = f // "100- or 200-level ESS courses or 200-level JEG courses"
	}
	if n == nil || hasText(n) {
		return nil, nil, false
	}
	pool, caps := poolCaps(n)
	if pool == nil || pool.Type != types.NodeCredits || pool.Items == nil || pool.Amount <= 0 || pool.MaxAmount != nil {
		return nil, nil, false
	}
	for _, it := range pool.Items {
		if it.Type != types.NodeCredits || it.Items != nil || it.Amount != 0 || it.MaxAmount != nil {
			return nil, nil, false
		}
	}
	name, _, ok := a.addUnion(row, pool.Items, ls)
	if !ok {
		return nil, nil, false
	}
	body := &types.ReqNode{Type: types.NodeCredits, Amount: pool.Amount, Lists: []string{name}}
	var rules []*types.Requirement
	for _, c := range caps {
		rules = append(rules, &types.Requirement{Kind: types.RequirementLimit, Req: c, Raw: grammar.CleanNote(text)})
	}
	return body, rules, true
}

// fromEitherRe matches a choice of one of the lists after it: "from
// either Group A, B or C".
var fromEitherRe = regexp.MustCompile(`(?i)\bfrom\s+either\s+(?:the\s+)?` + listRefPat)

// onlyNeedRe matches a smaller amount for some students: "only need to
// take 2.0 credits".
var onlyNeedRe = regexp.MustCompile(`(?i)\b(?:only\s+need\s+to|need\s+only)\s+(?:take|complete)\s+(?:an\s+additional\s+)?` + grammar.AmountPat + `\s+credits?\b`)

// fromTextRe matches the text the grammar left of an item that names
// where its courses come from: "from Pre-1800 British Literature courses".
var fromTextRe = regexp.MustCompile(`(?i)^from\s+\S`)

var (
	// unionFromRe matches a pool whose source follows "from": "1.5
	// additional credits from: Group B and/or ANT406H1". unionLeadFromRe
	// matches one it leads: "From Group A, B, C or other DRM: 2.0 credits".
	unionFromRe     = regexp.MustCompile(`(?i)^(.*?)\s+from:?\s+(.+)$`)
	unionLeadFromRe = regexp.MustCompile(`(?i)^from\s+(.+?):\s*(` + grammar.NumPat + `\s+credits?)(?:\s+to\s+make\s+up\s+the\s+total\s+of\s+` + grammar.NumPat + `\s+credits?)?$`)
	// unionRestRe matches the separators a union's source may have besides
	// its lists, codes and "other DRM".
	unionRestRe = regexp.MustCompile(`(?i)^(?:[\s,/]|\band/or\b|\band\b|\bor\b)*$`)
	// otherDesigRe matches "other DRM": the designator's courses.
	otherDesigRe = regexp.MustCompile(`\b(?:(?i:other)\s+)([A-Z]{3})\b(?:\s+(?i:courses))?`)
)

// listUnion reads a pool drawn from lists and courses together as a list of
// its own, named after the row, with an item for each list, the courses and
// any "other DRM" designator.
func (a *Assembler) listUnion(row *compRow, head string, ls compListRefs) (*types.ReqNode, bool) {
	var amount, source string
	if m := unionLeadFromRe.FindStringSubmatch(head); m != nil {
		amount, source = m[2], m[1]
	} else if m := unionFromRe.FindStringSubmatch(head); m != nil {
		amount, source = m[1], m[2]
	} else {
		return nil, false
	}
	var items []*types.ReqNode
	refs := listRefRe.FindAllStringIndex(source, -1)
	if refs == nil || row.r.Label == "" {
		return nil, false
	}
	rest := source
	for i := len(refs) - 1; i >= 0; i-- {
		m := refs[i]
		names := ls.resolve(listRefNames(source[m[0]:m[1]]))
		if names == nil {
			return nil, false
		}
		items = append(items, &types.ReqNode{Type: types.NodeCredits, Lists: names})
		end := m[1]
		if p := titleParenRe.FindStringIndex(source[end:]); p != nil && strings.TrimSpace(source[end:end+p[0]]) == "" && len(names) == 1 &&
			titleWords(strings.Trim(source[end+p[0]:end+p[1]], "()"), source[m[0]:m[1]], ls) {
			end += p[1] // "Group 2 (Value Theory)"
		}
		rest = rest[:m[0]] + " " + rest[end:]
	}
	slices.Reverse(items)
	if codes := anyCodeRe.FindAllString(rest, -1); codes != nil {
		items = append(items, &types.ReqNode{Type: types.NodeCredits, Courses: grammar.Dedupe(codes)})
	}
	for _, m := range otherDesigRe.FindAllStringSubmatch(rest, -1) {
		items = append(items, &types.ReqNode{Type: types.NodeCredits, Designators: []string{m[1]}})
		// "other DRM" beside a list titled "Other Topics ...": the program's other
		// courses are that list's too
		for _, l := range a.sortedLists(ls) {
			if l.titled && !l.bad && strings.HasPrefix(strings.ToLower(l.list.Name), "other ") {
				items = append(items, &types.ReqNode{Type: types.NodeCredits, Lists: []string{l.list.Name}})
			}
		}
	}
	rest = otherDesigRe.ReplaceAllString(anyCodeRe.ReplaceAllString(rest, " "), " ")
	if !unionRestRe.MatchString(rest) || len(items) < 2 {
		return nil, false
	}
	n, _, ok := listRefPoolOf(amount, ls, false, []string{"x"})
	if !ok {
		return nil, false
	}
	name, _, ok := a.addUnion(row, items, ls)
	if !ok {
		return nil, false
	}
	n.Lists = []string{name}
	return n, true
}

var (
	// upToListRe matches an alternative adding a list's share to a pool:
	// "Up to 1.5 credits from Group I".
	upToListRe = regexp.MustCompile(`(?i)^(?:up\s+to\s+` + grammar.AmountPat + `\s+credits?|any\s+course\s+\(\s*` + grammar.AmountPat + `\s+credits?\s*\))\s+(?:can\s+be\s+(?:used\s+)?)?from\s+(` + listRefPat + `)$`)
	// reqAltRe matches an alternative adding another requirement's courses
	// to a pool: "any courses from program requirement (1) above".
	reqAltRe = regexp.MustCompile(`(?i)^any\s+courses?\s+from\s+(?:program\s+)?requirement\s+\(?#?(\d{1,2})\)?(?:\s+above)?$`)
	// listMembersAltRe matches an alternative adding a list's members to a
	// pool: "400-level courses in Group I".
	listMembersAltRe = regexp.MustCompile(`(?i)^(?:and\s+)?(?:any\s+)?(?:(` + grammar.LevelPat + `(?:[-/+\s]*` + grammar.LevelPat + `)*[-+]?)\s*-?\s*level\s+)?courses?\s+(?:in|from)\s+(` + listRefPat + `)$`)
	// bareListAltRe matches an alternative that is a list, with its title
	// in brackets: "Group 2 (Value Theory)".
	bareListAltRe = regexp.MustCompile(`(?i)^(` + listRefPat + `)(?:\s*\(\s*([^()]*?)\s*\))?$`)
)

// poolListAlts merges row's pool with the or'd alternatives that add
// courses from lists into one list the row draws its amount from; a capped
// share is a limit under the row. Other alternatives stay.
func (a *Assembler) poolListAlts(row *compRow, ls compListRefs) {
	n := row.r.Req
	if row.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr || len(n.Items) < 2 {
		return
	}
	pool := n.Items[0]
	if !isPool(pool) || pool.Lists != nil || pool.Requirements != nil || row.r.Label == "" {
		return
	}
	member := grammar.CloneNode(pool)
	member.Amount = 0
	items := []*types.ReqNode{member}
	var limits []*types.Requirement
	var rest []*types.ReqNode
	var refs []compReqRef
	for _, it := range n.Items[1:] {
		t := ""
		if it.Type == types.NodeText && it.Kind == types.TextOther {
			t = strings.TrimSpace(strings.TrimSuffix(Clean(it.Text), "."))
		}
		if m := upToListRe.FindStringSubmatch(t); m != nil {
			names := ls.resolve(listRefNames(m[3]))
			if names == nil {
				return
			}
			v, _ := strconv.ParseFloat(m[1]+m[2], 64)
			items = append(items, &types.ReqNode{Type: types.NodeCredits, Lists: names})
			limits = append(limits, &types.Requirement{Kind: types.RequirementLimit, Raw: grammar.CleanNote(it.Text),
				Req: &types.ReqNode{Type: types.NodeCredits, Lists: slices.Clone(names), MaxAmount: grammar.MaxAmount(v)}})
			continue
		}
		if m := reqAltRe.FindStringSubmatch(t); m != nil {
			r := a.labelled(m[1])
			if r == nil || r == row {
				return
			}
			ref := &types.ReqNode{Type: types.NodeCredits, Requirements: []int{}}
			refs = append(refs, compReqRef{node: ref, rows: []*compRow{r}})
			items = append(items, ref)
			continue
		}
		if m := bareListAltRe.FindStringSubmatch(t); m != nil && (m[2] == "" || titleWords(m[2], m[1], ls)) {
			// "... / Group 2 (Value Theory)": the list itself, with its title
			names := ls.resolve(listRefNames(m[1]))
			if names == nil {
				return
			}
			items = append(items, &types.ReqNode{Type: types.NodeCredits, Lists: names})
			continue
		}
		if m := listMembersAltRe.FindStringSubmatch(t); m != nil {
			names := ls.resolve(listRefNames(m[2]))
			if names == nil {
				return
			}
			f := &types.ReqNode{Type: types.NodeCredits, Lists: names}
			if m[1] != "" {
				f.Levels = grammar.ParseLevels(m[1] + "-level")
			}
			items = append(items, f)
			continue
		}
		rest = append(rest, it)
	}
	if len(items) == 1 {
		return
	}
	name, l, ok := a.addUnion(row, items, ls)
	if !ok {
		return
	}
	for _, ref := range refs {
		ref.list = l.list
		a.reqRefs = append(a.reqRefs, ref)
	}
	body := &types.ReqNode{Type: types.NodeCredits, Amount: pool.Amount, Lists: []string{name}}
	row.r.Req = programs.OrNode(append([]*types.ReqNode{body}, rest...)...)
	for _, r := range limits {
		a.rowUnder(row, r)
	}
}

// filterUnionRe matches an amount of two filters joined by "or", with
// the codes either excludes: "2.0 credits of 100- or 200-level ESS courses or
// 200-level JEG courses (excluding ESS103H1)".
var filterUnionRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\s+(?:of|from|in)\s+(.+?)(?:\s*\(\s*excluding\s+([^()]*)\))?$`)

// filterUnion reads a filterUnionRe pool as credits whose items are the
// two filters, or returns nil.
func filterUnion(t string) *types.ReqNode {
	m := filterUnionRe.FindStringSubmatch(t)
	if m == nil {
		return nil
	}
	f := filterOrFilterRe.FindStringSubmatch(m[2])
	if f == nil || len(f[0]) != len(strings.TrimSpace(m[2])) {
		return nil
	}
	excluded := grammar.CodeRe.FindAllString(m[3], -1)
	if m[3] != "" && !listRestRe.MatchString(grammar.CodeRe.ReplaceAllString(m[3], " ")) {
		return nil
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	n := &types.ReqNode{Type: types.NodeCredits, Amount: amount}
	for _, part := range [][2]string{{f[1], f[2]}, {f[3], f[4]}} {
		it := &types.ReqNode{Type: types.NodeCredits, Designators: grammar.DesigRe.FindAllString(part[1], -1), Levels: grammar.ParseLevels(part[0] + "-level")}
		for _, c := range excluded {
			if slices.Contains(it.Designators, c[:3]) {
				it.Exclude = append(it.Exclude, c)
			}
		}
		n.Items = append(n.Items, it)
	}
	if slices.ContainsFunc(excluded, func(c string) bool {
		return !slices.ContainsFunc(n.Items, func(it *types.ReqNode) bool { return slices.Contains(it.Exclude, c) })
	}) {
		return nil // an excluded course neither filter matches
	}
	return n
}

// textPoolsOverLists reads, once the lists are known, the text pools that need
// a list of their own or a list by name: pairAdditional, optionsPool,
// countFloor, jointDesignators and listAlternative.
func (a *Assembler) textPoolsOverLists(ls compListRefs) {
	visit := func(r *compRow) {
		if r.r.Kind != types.RequirementAllocation || r.r.Req == nil {
			return
		}
		r.r.Req = listAlternatives(r.r.Req, ls)
		n := r.r.Req
		text := n
		if n.Type == types.NodeOr && len(n.Items) > 1 && n.Items[0].Type == types.NodeText &&
			!slices.ContainsFunc(n.Items[1:], func(it *types.ReqNode) bool { return it.Type != types.NodeText || it.Kind != types.TextPermission }) {
			text = n.Items[0] // or(text, routes)
		}
		if text.Type != types.NodeText || text.Kind != types.TextOther {
			return
		}
		body := a.pairAdditional(r, text.Text, ls)
		if body == nil {
			body = a.optionsPool(r, text.Text, ls)
		}
		if body == nil {
			body = countFloor(text.Text, ls)
		}
		if body == nil {
			body = jointDesignators(text.Text)
		}
		if body == nil {
			body = listAlternative(text.Text, ls)
		}
		if body == nil {
			return
		}
		if text == n {
			r.r.Req = body
		} else {
			n.Items[0] = body
		}
	}
	walkRowsPost(a.items, visit)
	walkRowsPost(a.program, visit)
}

// pairAdditional reads pairAdditionalRe as or(and(course, credits{the
// filters, excluding the course}), full course): the credits are additional.
func (a *Assembler) pairAdditional(r *compRow, t string, ls compListRefs) *types.ReqNode {
	m := pairAdditionalRe.FindStringSubmatch(Clean(t))
	if m == nil {
		return nil
	}
	amount, _ := strconv.ParseFloat(m[2], 64)
	var filters []*types.ReqNode
	for _, f := range levelFilterRe.FindAllStringSubmatch(m[3], -1) {
		filters = append(filters, levelFilter(f[1], f[2]))
	}
	pool := &types.ReqNode{Type: types.NodeCredits, Amount: amount, Exclude: []string{m[1]}}
	if len(filters) == 1 {
		pool.Designators, pool.Levels = filters[0].Designators, filters[0].Levels
	} else if name, _, ok := a.addUnion(r, filters, ls); ok {
		pool.Lists = []string{name}
	} else {
		return nil
	}
	return programs.OrNode(programs.AndNode(&types.ReqNode{Type: types.NodeCourse, Code: m[1]}, pool), &types.ReqNode{Type: types.NodeCourse, Code: m[4]})
}

// countFloor reads countFloorRe as and(credits{amount, designator,
// level}, credits{0.5 of those from the list}): the floor's course is one of
// those counted.
func countFloor(t string, ls compListRefs) *types.ReqNode {
	m := countFloorRe.FindStringSubmatch(Clean(t))
	if m == nil {
		return nil
	}
	l := ls.byName(strings.Replace(m[4], "group", "Group", 1))
	if l == nil || l.bad {
		return nil
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	level := int(m[2][0]-'0') * 100
	pool := &types.ReqNode{Type: types.NodeCredits, Amount: amount, Designators: []string{m[3]}, Levels: []int{level}}
	floor := &types.ReqNode{Type: types.NodeCredits, Amount: 0.5, Designators: []string{m[3]}, Levels: []int{level}, Lists: []string{l.list.Name}}
	return programs.AndNode(pool, floor)
}

var (
	// optionsPoolRe matches an amount from options separated by ";": "1.0
	// credit from the following options: MCS329H1/ MCS329Y1; ...".
	optionsPoolRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\s+from\s+the\s+following\s+options:\s*(.+)$`)
	// notTakenRefRe matches another requirement's courses not used there:
	// "courses listed in requirement 3 not already taken".
	notTakenRefRe = regexp.MustCompile(`(?i)^courses\s+listed\s+in\s+requirement\s+(\d{1,2})\s+not\s+already\s+taken$`)
	// relatedOptionsRe matches options ending in a course the topic decides:
	// "1.0 credit from: EDS356H1, ..., or a course related to Education and
	// Society at the 300+ level ...". Group 4 is that course's level.
	relatedOptionsRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\s+from:\s*(.+?),\s+or\s+(an?\s+course\s+related\s+to\s+.+?\s+at\s+the\s+([1-4])00\+\s+level\b.*)$`)
	// listBelowRe matches a list by name: "cognate courses listed below".
	listBelowRe = regexp.MustCompile(`(?i)^(\w+\s+courses)\s+listed\s+below$`)
)

// optionsPool reads optionsPoolRe as credits from a union of its
// options, every one of which must read. With relatedOptionsRe the
// topic's course is the lenient alternative: and(credits{amount, level},
// text{content}).
func (a *Assembler) optionsPool(r *compRow, t string, ls compListRefs) *types.ReqNode {
	m := optionsPoolRe.FindStringSubmatch(Clean(t))
	var opts []string
	var related *types.ReqNode
	switch rm := relatedOptionsRe.FindStringSubmatch(strings.TrimSuffix(Clean(t), ".")); {
	case m != nil:
		opts = grammar.SplitTop(m[2], ';')
	case rm != nil:
		m, opts = rm, strings.Split(rm[2], ", ")
		v, _ := strconv.ParseFloat(rm[1], 64)
		related = programs.AndNode(&types.ReqNode{Type: types.NodeCredits, Amount: v, Levels: grammar.ParseLevels(rm[4] + "00+-level")}, TextNode(types.TextContent, rm[3]))
	default:
		return nil
	}
	var items []*types.ReqNode
	var codes []string
	var refs []compReqRef
	for _, o := range opts {
		o = strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(o), "or ")), ".")
		if rm := notTakenRefRe.FindStringSubmatch(o); rm != nil {
			row := a.labelled(rm[1])
			if row == nil || row == r {
				return nil
			}
			ref := &types.ReqNode{Type: types.NodeCredits}
			items = append(items, ref)
			refs = append(refs, compReqRef{node: ref, rows: []*compRow{row}})
			continue
		}
		if lm := listBelowRe.FindStringSubmatch(o); lm != nil {
			var found *compList
			for _, l := range ls {
				if strings.EqualFold(l.list.Name, lm[1]) && !l.bad {
					found = l
				}
			}
			if found == nil {
				return nil
			}
			items = append(items, &types.ReqNode{Type: types.NodeCredits, Lists: []string{found.list.Name}})
			continue
		}
		if !IsCodeList(o) {
			return nil
		}
		codes = append(codes, anyCodeRe.FindAllString(o, -1)...)
	}
	if codes != nil {
		items = append(items, &types.ReqNode{Type: types.NodeCredits, Courses: grammar.Dedupe(codes)})
	}
	if len(items) < 2 {
		return nil
	}
	name, l, ok := a.addUnion(r, items, ls)
	if !ok {
		return nil
	}
	for _, ref := range refs {
		ref.list = l.list
		a.reqRefs = append(a.reqRefs, ref)
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	return programs.OrNode(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Lists: []string{name}}, related)
}

var (
	// languageListAltRe matches a list of language courses: "1.0 credit at
	// any level of an Asian language (see Accepted Language Courses below)".
	languageListAltRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\s+at\s+any\s+level\s+(?:of|in)\s+an?\s+\w+\s+language\s+\(see\s+(.+?)\s+below\)$`)
	// groupCourseAltRe matches a course from a group: "0.5 credit in a
	// quantitative or qualitative methods course from Group C".
	groupCourseAltRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\s+in\s+an?\s+[\w ]+?\s+course\s+from\s+(group\s+[A-Z])(?:\s+to\s+fulfil+\s+the\s+[\w ]+)?$`)
	// jointDesignatorsRe matches a level's courses of a designator and its
	// joint ones: "1.0 credit in 300-level CAS courses (including JHA, JPA, and
	// SAS)". Group 4 is an optional second level.
	jointDesignatorsRe = regexp.MustCompile(`^` + grammar.AmountPat + `\s+credits?\s+(?:in|from)\s+([1-4])00-level\s+([A-Z]{3})\s+courses(?:\s+and/or\s+([1-4])00-level\s+courses)?\s+\(including\s+([A-Z]{3}(?:,\s*[A-Z]{3})*,?\s+and\s+[A-Z]{3})\)$`)
)

// listAlternatives reads the text alternatives of n's or nodes that draw on a
// list by name.
func listAlternatives(n *types.ReqNode, ls compListRefs) *types.ReqNode {
	for i, it := range n.Items {
		n.Items[i] = listAlternatives(it, ls)
	}
	if n.Type != types.NodeOr {
		return n
	}
	for i, it := range n.Items {
		if it.Type == types.NodeText && it.Kind == types.TextOther {
			if body := listAlternative(it.Text, ls); body != nil {
				n.Items[i] = body
			}
		}
	}
	return n
}

// listAlternative reads text that draws on a list by name as credits from
// it, or nil.
func listAlternative(t string, ls compListRefs) *types.ReqNode {
	t = Clean(t)
	m := languageListAltRe.FindStringSubmatch(t)
	if m == nil {
		m = groupCourseAltRe.FindStringSubmatch(t)
	}
	if m == nil {
		return nil
	}
	var found *compList
	for _, l := range ls {
		if strings.EqualFold(l.list.Name, strings.Trim(m[2], "_")) && !l.bad {
			found = l
		}
	}
	if found == nil {
		return nil
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	return &types.ReqNode{Type: types.NodeCredits, Amount: amount, Lists: []string{found.list.Name}}
}

// jointDesignators reads jointDesignatorsRe. With "and/or 400-level
// courses", both levels of the designator is the strict reading and the
// sentence the lenient one (Review).
func jointDesignators(t string) *types.ReqNode {
	m := jointDesignatorsRe.FindStringSubmatch(Clean(t))
	if m == nil {
		return nil
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	pool := &types.ReqNode{Type: types.NodeCredits, Amount: amount, Designators: append([]string{m[3]}, grammar.DesigRe.FindAllString(m[5], -1)...), Levels: []int{int(m[2][0]-'0') * 100}}
	if m[4] == "" {
		return pool
	}
	pool.Levels = grammar.Dedupe(append(pool.Levels, int(m[4][0]-'0')*100))
	slices.Sort(pool.Levels)
	return programs.OrNode(pool, TextNode(types.TextAmbiguity, t))
}
