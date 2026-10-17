package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Pool grammar the item reader leaves as text: extended pools, course counts,
// subject names, breadth categories, filters among members, embedded pools.

// poolGrammar reads again the text of every allocation the rules here know.
// It runs inside namedLists, so the union lists it makes are emitted with the
// others.
func (a *compAssembler) poolGrammar(ls compListRefs) {
	for _, r := range a.allRows() {
		if r == a.total || r.r.Kind != types.RequirementAllocation || r.r.Req == nil {
			continue
		}
		if a.alsoUsed(r, ls) {
			continue
		}
		if n := r.r.Req; n.Type == types.NodeCredits && n.Lists != nil && n.Items == nil {
			// read already; a note may widen the pool: "One course (0.5
			// credit) can be from Group F"
			a.capNotes(r, n, ls)
			continue
		}
		a.poolTexts(r, ls)
	}
}

// complAlsoUsedRe matches a sentence that adds a filter to the row's pool:
// "200/300-level GGR courses may also be used to meet this requirement".
var complAlsoUsedRe = regexp.MustCompile(`(?i)^(?:an?\s+)?(\d00(?:\s*(?:/|or)\s*\d00)*)-level\s+(.+?)\s+courses?(\s*,\s*including\s+joint\s+courses\s*,)?\s+may\s+also\s+be\s+used\s+to\s+meet\s+this\s+requirement$`)

// poolSubjects maps department names in pools to the department's designator.
var poolSubjects = map[string]string{"geography & planning": "GGR", "geography and planning": "GGR"}

// complItemTitleLabelRe matches an italic label at the start of a line:
// "_First Year Geography Courses:_".
var complItemTitleLabelRe = regexp.MustCompile(`^_([^_:]+):?_`)

// alsoUsed reads or(pool, "<filter> may also be used ...") as a union list of
// the pool and the filter. "Including joint courses" is vague: the strict
// reading is the own designator; any course at those levels is Review.
func (a *compAssembler) alsoUsed(r *compRow, ls compListRefs) bool {
	n := r.r.Req
	if n.Type != types.NodeOr || len(n.Items) < 2 {
		return false
	}
	pool := n.Items[0]
	if pool.Type != types.NodeCredits || pool.Amount <= 0 || pool.Courses == nil || pool.Lists != nil || pool.Designators != nil || pool.Levels != nil || pool.MaxAmount != nil {
		return false
	}
	at := -1
	var m []string
	for i, it := range n.Items[1:] {
		if it.Type == types.NodeText && it.Kind == types.TextOther {
			if m = complAlsoUsedRe.FindStringSubmatch(complClean(it.Text)); m != nil {
				at = i + 1
				break
			}
		}
	}
	if at < 0 {
		return false
	}
	subject := strings.TrimSpace(m[2])
	desig := poolSubjects[strings.ToLower(subject)]
	if desigRe.MatchString(subject) && len(desigRe.FindString(subject)) == len(subject) {
		desig = subject
	}
	levels := parseLevels(m[1] + "-level")
	if desig == "" || levels == nil {
		return false
	}
	if r.r.Label == "" {
		// the year is the row's section, so the label names the union
		if l := complItemTitleLabelRe.FindStringSubmatch(r.r.Raw); l != nil {
			r.r.Label = strings.TrimSpace(l[1])
		}
	}
	items := []*types.ReqNode{
		{Type: types.NodeCredits, Courses: slices.Clone(pool.Courses)},
		{Type: types.NodeCredits, Designators: []string{desig}, Levels: slices.Clone(levels)},
	}
	name, _, ok := a.addUnion(r, items, ls)
	if !ok {
		return false
	}
	alts := []*types.ReqNode{{Type: types.NodeCredits, Amount: pool.Amount, Lists: []string{name}}}
	if m[3] != "" {
		alts = append(alts, andNode(&types.ReqNode{Type: types.NodeCredits, Amount: pool.Amount, Levels: levels},
			textNode(types.TextAmbiguity, "which joint courses count isn't said: "+n.Items[at].Text)))
	}
	rest := slices.Delete(slices.Clone(n.Items[1:]), at-1, at)
	r.r.Req = orNode(append(alts, rest...)...)
	return true
}

// poolTexts replaces each text node of kind other in r's body that
// poolRewrite makes readable by the item grammar.
func (a *compAssembler) poolTexts(r *compRow, ls compListRefs) {
	if n := r.r.Req; n.Type == types.NodeAnd && len(n.Items) == 2 && n.Items[0].Type == types.NodeText && n.Items[1].Type == types.NodeText &&
		n.Items[0].Kind == types.TextOther && complExcludesRe.MatchString(complClean(n.Items[1].Text)) {
		// a pool then "Excludes BCH472Y1, ...": the pool's exclusions
		t := strings.TrimSuffix(complClean(n.Items[0].Text), ".") + ", excluding " + complExcludesRe.FindStringSubmatch(complClean(n.Items[1].Text))[1]
		if x, rows, notes, ok := a.poolText(r, t, ls); ok {
			r.r.Req = textNode(types.TextOther, t)
			r.r.Req, r.r.Notes = x, dedupe(append(r.r.Notes, notes...))
			for _, k := range rows {
				a.rowUnder(r, k)
			}
			splitFloors(r, r.r.Raw)
		}
		return
	}
	if r.r.Req.Type == types.NodeText {
		if r.r.Req.Kind != types.TextOther {
			return
		}
		text, kept := r.r.Req.Text, r.r.Notes
		for i, note := range r.r.Notes {
			if m := complExcludesRe.FindStringSubmatch(complClean(note)); m != nil {
				// "Excludes BCH472Y1, ..." on its own line: the pool's exclusions
				text = strings.TrimSuffix(complClean(text), ".") + ", excluding " + m[1]
				kept = slices.Delete(slices.Clone(r.r.Notes), i, i+1)
				break
			}
		}
		if n, rows, notes, ok := a.poolText(r, text, ls); ok {
			r.r.Notes = kept
			r.r.Req = n
			for _, x := range rows {
				a.rowUnder(r, x)
			}
			r.r.Notes = dedupe(append(r.r.Notes, notes...))
			// as the item reader does
			splitCaps(r)
			splitFloors(r, r.r.Raw)
			a.splitShared(r)
		}
		return
	}
	if r.r.Req.Type != types.NodeAnd && r.r.Req.Type != types.NodeOr {
		return
	}
	var walk func(n *types.ReqNode)
	walk = func(n *types.ReqNode) {
		for i, it := range n.Items {
			switch {
			case it.Type == types.NodeAnd || it.Type == types.NodeOr:
				walk(it)
			case it.Type == types.NodeText && it.Kind == types.TextOther:
				if x, rows, notes, ok := a.poolText(r, it.Text, ls); ok && rows == nil && x.Type != types.NodeAnd {
					n.Items[i] = x
					r.r.Notes = dedupe(append(r.r.Notes, notes...))
				}
			}
		}
	}
	walk(r.r.Req)
}

// Rewrites that bring a pool's wording to the item grammar's.
var (
	// complCountCreditsRe matches a course count with its credits: "Any 6
	// courses (3.0 credits) from".
	complCountCreditsRe = regexp.MustCompile(`(?i)^any\s+(?:\d+|one|two|three|four|five|six)\s+(?:half[- ]?)?courses?\s+(?:or\s+the\s+equivalent\s+)?\((\d+(?:\.\d+)?)\s+credits?\)\s+from\b`)
	// complCountFromTheRe matches a count of courses: "Two courses from the BCH
	// 400-series lecture courses:".
	complCountFromTheRe = regexp.MustCompile(`(?i)^(two|three|four)\s+courses\s+from\s+the\s+[^:]+:\s*`)
	// complNoMoreRe matches a capped pool: "One lab course (0.5 credit), and no
	// more, taken from the following:".
	complNoMoreRe = regexp.MustCompile(`(?i)^one\s+[a-z]+\s+course\s+\((\d+(?:\.\d+)?)\s+credits?\),\s+and\s+no\s+more,\s+taken\s+from(?:\s+the\s+following)?\s*:?`)
	// complHalfCoursesOfWhichRe matches a floor in two subjects: ", at least two
	// of which must be Math or Physics half courses".
	complHalfCoursesOfWhichRe = regexp.MustCompile(`(?i),\s*at\s+least\s+(one|two|three|four)\s+of\s+which\s+must\s+be\s+([A-Za-z]+)\s+or\s+([A-Za-z]+)\s+half\s+courses$`)
	// complSubjectCreditsRe matches a subject named in words: "2.0 credits in
	// Sociology".
	complSubjectCreditsRe = regexp.MustCompile(`(?i)\bcredits\s+in\s+([A-Z][a-z]+)\b`)
	// complOfWhichIeRe matches a floor with its restatement: "(0.5 of which must
	// be taken at the 300-level, i.e., ...)".
	complOfWhichIeRe = regexp.MustCompile(`(?i)\s*\((\d+(?:\.\d+)?)\s+of\s+which\s+must\s+be\s+taken\s+at\s+the\s+(\d00)-level,\s*i\.e\.,[^()]*\)$`)
	// complPrefixCoursesRe matches courses named with their designator: "Peace,
	// Conflict and Justice courses (i.e. PCJ prefix classes)".
	complPrefixCoursesRe = regexp.MustCompile(`(?i)\b[A-Z][A-Za-z,\s]+?\s+courses\s+\(i\.e\.\s+([A-Z]{3})\s+prefix\s+classes\)`)
	// complBareFloorRe matches a floor without the word credit: "at least 0.5 at
	// the 400 level".
	complBareFloorRe = regexp.MustCompile(`(?i)\b(at\s+least\s+\d+(?:\.\d+)?)(\s+at\s+the\s+\d00)`)
	// complBreadthOrRe matches a pool or a breadth category: "<pool> or 0.5
	// credit in any BR=5 course".
	complBreadthOrRe = regexp.MustCompile(`(?i)^(.+?)\s+or\s+(?:any\s+)?(\d+(?:\.\d+)?)\s+credits?\s+(?:in|from)\s+(?:any|an?)\s+(?:(\d00(?:/\d00)*)-level\s+)?(?:course\s+(?:from\s+)?)?(?:BR\s*=\s*|Breadth\s+(?:Requirement\s+)?Category\s+#?)(\d)\b(?:\s+course)?(?::\s*The\s+[A-Za-z&\s]+)?$`)
	// complTitledFromRe matches a title before the courses: "0.5 credit from
	// Quantitative Methods: ENG286H1/ ENG287H1".
	complTitledFromRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?\s+credits?\s+from)\s+[A-Z][A-Za-z\s]+:\s+(` + codePat + `)`)
)

// poolRewrite rewrites s into the item grammar's wording; caps reports an
// "and no more" cap on the program, and ok is false when no rule applies.
func poolRewrite(s string) (t string, caps bool, ok bool) {
	t = s
	if m := complCountCreditsRe.FindStringSubmatch(t); m != nil {
		t = m[1] + " credits from" + t[len(m[0]):]
	}
	if m := complCountFromTheRe.FindStringSubmatch(t); m != nil {
		// half courses only, so the count gives the credits
		if codes := codeRe.FindAllString(t, -1); codes != nil && !slices.ContainsFunc(codes, func(c string) bool { return c[6] != 'H' }) {
			k := float64(countWords[strings.ToLower(m[1])]) * 0.5
			t = strconv.FormatFloat(k, 'f', 1, 64) + " credits from " + t[len(m[0]):]
		}
	}
	if m := complNoMoreRe.FindStringSubmatch(t); m != nil {
		t = m[1] + " credit from" + t[len(m[0]):]
		caps = true
	}
	if m := complHalfCoursesOfWhichRe.FindStringSubmatchIndex(t); m != nil {
		d1, d2 := subjectDesignators[strings.ToLower(t[m[4]:m[5]])], subjectDesignators[strings.ToLower(t[m[6]:m[7]])]
		if d1 == "" || d2 == "" {
			return s, false, false
		}
		k := float64(countWords[strings.ToLower(t[m[2]:m[3]])]) * 0.5
		t = t[:m[0]] + ", including at least " + strconv.FormatFloat(k, 'f', 1, 64) + " credit in " + d1 + " or " + d2 + " courses"
	}
	if m := complSubjectCreditsRe.FindStringSubmatchIndex(t); m != nil {
		if d := subjectDesignators[strings.ToLower(t[m[2]:m[3]])]; d != "" {
			t = t[:m[0]] + d + " credits" + t[m[1]:]
			t = strings.Replace(t, "additional "+d, d, 1)
		}
	}
	if m := complOfWhichIeRe.FindStringSubmatch(t); m != nil {
		t = t[:len(t)-len(m[0])] + ", including at least " + m[1] + " credit at the " + m[2] + "-level"
	}
	if m := complPrefixCoursesRe.FindStringSubmatchIndex(t); m != nil {
		t = t[:m[0]] + t[m[2]:m[3]] + " courses" + t[m[1]:]
	}
	t = complBareFloorRe.ReplaceAllString(t, "$1 credit$2")
	return t, caps, t != s
}

// poolText reads a text node of row r again: as a breadth alternative, filter
// members, embedded pools, or poolRewrite then the item grammar. A reading
// that loses a course code is no reading.
func (a *compAssembler) poolText(r *compRow, text string, ls compListRefs) (n *types.ReqNode, rows []*types.Requirement, notes []string, ok bool) {
	s := complClean(text)
	defer func() {
		if ok && !a.keepsCodes(s, n, rows, notes) {
			n, rows, notes, ok = nil, nil, nil, false
		}
	}()
	if m := complBreadthOrRe.FindStringSubmatch(s); m != nil {
		left := complTitledFromRe.ReplaceAllString(m[1], "$1 $2")
		body, rs, ns := a.itemBody(left)
		if body == nil || hasText(body) || rs != nil {
			return nil, nil, nil, false
		}
		amount, _ := strconv.ParseFloat(m[2], 64)
		cat, _ := strconv.Atoi(m[4])
		var levels []int
		if m[3] != "" {
			levels = parseLevels(m[3] + "-level")
		}
		return orNode(body, &types.ReqNode{Type: types.NodeCredits, Amount: amount, Breadth: []int{cat}, Levels: levels}), nil, ns, true
	}
	if n, rows, notes, ok = a.filterMembers(r, s, ls); ok {
		return n, rows, notes, true
	}
	if n, notes, ok = a.embeddedPools(s); ok {
		return n, nil, notes, true
	}
	t, caps, changed := poolRewrite(s)
	if !changed {
		return nil, nil, nil, false
	}
	n, rows, notes = a.itemBody(t)
	if n == nil || hasText(n) {
		if ls == nil {
			return nil, nil, nil, false
		}
		var ok2 bool
		if n, notes, ok2 = listRefPool(t, ls, false); !ok2 || n == nil || hasText(n) {
			return nil, nil, nil, false
		}
		rows = nil
		if !a.capNotes(r, n, ls) {
			return nil, nil, nil, false
		}
	}
	if caps {
		pool, _ := poolCaps(n)
		if pool == nil || pool.Type != types.NodeCredits || pool.Courses == nil || pool.Lists != nil || pool.Designators != nil {
			return nil, nil, nil, false
		}
		// "and no more": the program counts no more of these courses
		max := pool.Amount
		a.rowUnder(nil, &types.Requirement{Kind: types.RequirementLimit, Req: &types.ReqNode{Type: types.NodeCredits, Courses: slices.Clone(pool.Courses), MaxAmount: &max}, Raw: r.r.Raw})
	}
	return n, rows, notes, true
}

var (
	// complAtLeastOneOfRe matches "at least one 0.5 credit from PCL367H1 or
	// PCL368H1".
	complAtLeastOneOfRe = regexp.MustCompile(`,?\s*at\s+least\s+one\s+0\.5\s+credit\s+from\s+(` + codePat + `)\s+or\s+(` + codePat + `)`)
	// complSeeNoteRe matches "(see NOTE 2)".
	complSeeNoteRe = regexp.MustCompile(`(?i)\s*\(see\s+note(?:\s+\d+)?\)`)
)

// embeddedPools reads a course list with a pool inside it ("BCH311H1/ CSB349H1/
// MGY311Y1; 2.5 credits from MGY314H1, ...") as the and of its parts.
func (a *compAssembler) embeddedPools(s string) (*types.ReqNode, []string, bool) {
	t := complSeeNoteRe.ReplaceAllString(s, "")
	t = complAtLeastOneOfRe.ReplaceAllString(t, ", $1/ $2")
	parts := splitTop(t, ';')
	if len(parts) < 2 && t == s {
		return nil, nil, false
	}
	var nodes []*types.ReqNode
	var notes []string
	for _, p := range parts {
		p = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(p), "and "))
		n, rows, ns := a.itemBody(p)
		if n == nil || hasText(n) || rows != nil {
			return nil, nil, false
		}
		nodes = append(nodes, n)
		notes = append(notes, ns...)
	}
	return andNode(nodes...), notes, true
}

// complExcludesRe matches exclusions after a pool: "Excludes BCH472Y1,
// BCH473Y1".
var complExcludesRe = regexp.MustCompile(`^Excludes\s+((?:` + codePat + `(?:\s*,\s*|\s+and\s+)?)+)$`)

// complFromListCapNoteRe matches a note letting part of a pool come from
// another list: "One course (0.5 credit) can be from Group F".
var complFromListCapNoteRe = regexp.MustCompile(`(?i)^(?:one|a)\s+course\s+\((\d+(?:\.\d+)?)\s+credits?\)\s+can\s+be\s+from\s+(.+)$`)

// capNotes adds to pool n the lists r's notes let part of it come from, with a
// limit on that part. An unresolved list leaves the pool unread (false): read
// alone, it would be NotMet for a student who used it.
func (a *compAssembler) capNotes(r *compRow, n *types.ReqNode, ls compListRefs) bool {
	var keep []string
	for _, note := range r.r.Notes {
		m := complFromListCapNoteRe.FindStringSubmatch(complClean(note))
		if m == nil {
			keep = append(keep, note)
			continue
		}
		names := ls.resolve(listRefNames(m[2]))
		if len(names) != 1 || n.Type != types.NodeCredits || n.Lists == nil {
			return false
		}
		max, _ := strconv.ParseFloat(m[1], 64)
		n.Lists = append(n.Lists, names[0])
		a.rowUnder(r, &types.Requirement{Kind: types.RequirementLimit, Req: &types.ReqNode{Type: types.NodeCredits, Lists: names, MaxAmount: &max}, Raw: cleanNote(note)})
	}
	r.r.Notes = keep
	return true
}

// keepsCodes reports whether every course code of s is in the reading or in a
// union list it made.
func (a *compAssembler) keepsCodes(s string, n *types.ReqNode, rows []*types.Requirement, notes []string) bool {
	have := map[string]bool{}
	var add func(x *types.ReqNode)
	add = func(x *types.ReqNode) {
		walkReq(x, func(y *types.ReqNode) {
			have[y.Code] = true
			for _, c := range append(slices.Clone(y.Courses), y.Exclude...) {
				have[c] = true
			}
			for _, l := range a.unions {
				if slices.Contains(y.Lists, l.list.Name) {
					for _, it := range l.list.Items {
						add(it)
					}
				}
			}
		})
	}
	add(n)
	for _, r := range rows {
		add(r.Req)
	}
	for _, c := range codeRe.FindAllString(strings.Join(notes, " "), -1) {
		have[c] = true
	}
	for _, c := range codeRe.FindAllString(s, -1) {
		if !have[c] {
			return false
		}
	}
	return true
}

var (
	// complFilterUnionLeadRe matches a pool lead with an optional floor: "1.5
	// credits, including at least 0.5 credit at the 400-level, from <members>".
	complFilterUnionLeadRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+(?:additional\s+)?credits?(?:\s*,?\s*(including\s+at\s+least\s+[^,:]+?)\s*,?)?\s+(?:from|of|selected\s+from)\s*:?\s+(.+)$`)
	// complMemberFilterRe matches a filter among a pool's members: "PSL400-series
	// courses". Each alternative has its own groups; filterMembers indexes them.
	complMemberFilterRe = regexp.MustCompile(`(?:[Aa]ny\s+(?:other\s+)?|further\s+)?(?:\b([A-Z]{3})\s?([1-4])00(\+)?[- ]?(?:series|level)\s+courses?|\b([A-Z]{3})\s+at\s+the\s+([1-4])00(\+)?\s+level|\b([A-Z]{3})\s+coded\s*([1-4])00\s+level\s+courses|courses\s+carrying\s+the\s+([A-Z]{3})\s+designator|\b([A-Z]{3}(?:\s+or\s+[A-Z]{3})*)\s+(half-)?courses?\s+at\s+the\s+([1-4])00(\+)?\s+level)`)
	// complRecommendedAsideRe matches advice after a filter: "( BCH422H1, BCH425H1
	// recommended)".
	complRecommendedAsideRe = regexp.MustCompile(`\s*\(([^()]*\brecommended)\)`)
	// complExcludingTailRe matches a closing exclusion: ", excluding JPH311H1,
	// JPH441H1".
	complExcludingTailRe = regexp.MustCompile(`(?i),?\s*excluding\s+((?:` + codePat + `(?:\s*,\s*|\s+and\s+|\s*$))+)$`)
	// complBrokenCodeRe matches a code with its last digit missing, which leaves
	// the members unreadable: "CSB344H".
	complBrokenCodeRe = regexp.MustCompile(`\b[A-Z]{3}\d{3}[HY](?:[^0-9]|$)`)
	// complMemberSepRe matches what may sit between members once the filters are
	// taken out: ", or ".
	complMemberSepRe = regexp.MustCompile(`^[\s,;/]*(?:(?:or|and)\b[\s,;/]*)?$`)
)

// filterMembers reads a pool of courses and filters ("1.5 credits from
// BCH370H1/ CSB428H1/ BCH 400-level courses") as the row's own union list.
// A lead floor is a constraint row; members that are only courses are left.
func (a *compAssembler) filterMembers(r *compRow, s string, ls compListRefs) (n *types.ReqNode, rows []*types.Requirement, notes []string, ok bool) {
	m := complFilterUnionLeadRe.FindStringSubmatch(s)
	if m == nil || complBrokenCodeRe.MatchString(s) {
		return nil, nil, nil, false
	}
	members := m[3]
	for _, x := range complRecommendedAsideRe.FindAllStringSubmatch(members, -1) {
		notes = append(notes, strings.TrimSpace(x[1]))
	}
	members = complRecommendedAsideRe.ReplaceAllString(members, "")
	var excluded []string
	if x := complExcludingTailRe.FindStringSubmatchIndex(members); x != nil {
		excluded = codeRe.FindAllString(members[x[2]:x[3]], -1)
		members = members[:x[0]]
	}
	locs := complMemberFilterRe.FindAllStringSubmatchIndex(members, -1)
	if locs == nil {
		return nil, nil, nil, false
	}
	var items []*types.ReqNode
	var rest strings.Builder
	last := 0
	for _, l := range locs {
		gap := members[last:l[0]]
		if !codeRe.MatchString(gap) && !complMemberSepRe.MatchString(gap) {
			return nil, nil, nil, false // words between members: not a list
		}
		rest.WriteString(gap)
		last = l[1]
		g := func(k int) string {
			if l[2*k] < 0 {
				return ""
			}
			return members[l[2*k]:l[2*k+1]]
		}
		var desig, level, plus string
		desigs, half := []string(nil), false
		switch {
		case g(10) != "":
			desigs, half, level, plus = strings.Split(g(10), " or "), g(11) != "", g(12), g(13)
			desig = desigs[0]
		case g(1) != "":
			desig, level, plus = g(1), g(2), g(3)
		case g(4) != "":
			desig, level, plus = g(4), g(5), g(6)
		case g(7) != "":
			desig, level = g(7), g(8)
		default:
			desig = g(9)
		}
		if desig == "" || strings.ToUpper(desig) != desig {
			return nil, nil, nil, false
		}
		it := &types.ReqNode{Type: types.NodeCredits, Designators: []string{desig}}
		if desigs != nil {
			it.Designators = desigs
		}
		if half {
			it.MaxCourseAmount = 0.5
		}
		if level != "" {
			it.Levels = parseLevels(level + "00" + plus + "-level")
		}
		for _, c := range excluded {
			if c[:3] == desig {
				it.Exclude = append(it.Exclude, c)
			}
		}
		items = append(items, it)
	}
	tail := members[last:]
	if !codeRe.MatchString(tail) && !complMemberSepRe.MatchString(tail) {
		return nil, nil, nil, false
	}
	rest.WriteString(tail)
	if codes := codeRe.FindAllString(rest.String(), -1); codes != nil {
		if strings.Trim(codeRe.ReplaceAllString(rest.String(), ""), " ,;/()orand") != "" {
			return nil, nil, nil, false
		}
		items = append([]*types.ReqNode{{Type: types.NodeCredits, Courses: dedupe(codes)}}, items...)
		// alternatives among the courses are capped as the item grammar caps them
		list := strings.TrimRight(strings.TrimSpace(rest.String()), " ,;/")
		list = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(list), " or"), " and")
		if x, _, _ := a.itemBody(m[1] + " credits from " + list); x != nil && !hasText(x) {
			if _, caps := poolCaps(x); caps != nil {
				for _, c := range caps {
					rows = append(rows, &types.Requirement{Kind: types.RequirementLimit, Req: c, Raw: r.r.Raw})
				}
			}
		}
	}
	// "PHY 300-level courses, PHY 400-level courses": one filter
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			x, y := items[i], items[j]
			if x.Courses == nil && y.Courses == nil && x.Levels != nil && y.Levels != nil && slices.Equal(x.Designators, y.Designators) && slices.Equal(x.Exclude, y.Exclude) &&
				x.MaxCourseAmount == y.MaxCourseAmount {
				x.Levels = dedupeInts(append(x.Levels, y.Levels...))
				items = slices.Delete(items, j, j+1)
				j--
			}
		}
	}
	for _, c := range excluded {
		if !slices.ContainsFunc(items, func(it *types.ReqNode) bool { return slices.Contains(it.Exclude, c) }) {
			notes = append(notes, "excluding "+c) // a course no filter of the pool counts
		}
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	if m[2] != "" {
		// the lead's floor is a constraint on the row
		f, _, _ := a.itemBody(complBareFloorRe.ReplaceAllString(strings.TrimPrefix(strings.TrimSpace(strings.Replace(m[2], "including at least", "", 1)), "a "), "$1 credit$2"))
		if f == nil || f.Type != types.NodeCredits || hasText(f) || f.Amount <= 0 || f.Amount >= amount {
			return nil, nil, nil, false
		}
		rows = append(rows, &types.Requirement{Kind: types.RequirementConstraint, Req: f, Raw: r.r.Raw})
	}
	if len(items) == 1 && items[0].Courses == nil {
		n = items[0]
		n.Amount = amount
		return n, rows, notes, true // one filter: no list needed
	}
	name, _, ok := a.addUnion(r, items, ls)
	if !ok {
		return nil, nil, nil, false
	}
	return &types.ReqNode{Type: types.NodeCredits, Amount: amount, Lists: []string{name}}, rows, notes, true
}

// dedupeInts returns xs sorted, without repeats.
func dedupeInts(xs []int) []int {
	slices.Sort(xs)
	return slices.Compact(xs)
}
