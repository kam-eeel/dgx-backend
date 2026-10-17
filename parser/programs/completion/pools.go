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

// Pool readings the item grammar leaves as text: embedded pools, pools of
// courses and filters, and the filter pools of the rows.

// poolGrammar reads again, as pools, the text of every allocation.
// It runs inside namedLists, so the union lists it makes are emitted with the
// others.
func (a *Assembler) poolGrammar(ls compListRefs) {
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

// alsoUsedRe matches a sentence that adds a filter to the row's pool:
// "200/300-level GGR courses may also be used to meet this requirement".
var alsoUsedRe = regexp.MustCompile(`(?i)^(?:an?\s+)?(\d00(?:\s*(?:/|or)\s*\d00)*)-level\s+(.+?)\s+courses?(\s*,\s*including\s+joint\s+courses\s*,)?\s+may\s+also\s+be\s+used\s+to\s+meet\s+this\s+requirement$`)

// poolSubjects maps department names in pools to the department's designator.
var poolSubjects = map[string]string{"geography & planning": "GGR", "geography and planning": "GGR"}

// itemTitleLabelRe matches an italic label at the start of a line:
// "_First Year Geography Courses:_".
var itemTitleLabelRe = regexp.MustCompile(`^_([^_:]+):?_`)

// alsoUsed reads or(pool, "<filter> may also be used ...") as a union list of
// the pool and the filter. "Including joint courses" is vague: the strict
// reading is the own designator; any course at those levels is Review.
func (a *Assembler) alsoUsed(r *compRow, ls compListRefs) bool {
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
			if m = alsoUsedRe.FindStringSubmatch(Clean(it.Text)); m != nil {
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
	if grammar.DesigRe.MatchString(subject) && len(grammar.DesigRe.FindString(subject)) == len(subject) {
		desig = subject
	}
	levels := grammar.ParseLevels(m[1] + "-level")
	if desig == "" || levels == nil {
		return false
	}
	if r.r.Label == "" {
		// the year is the row's section, so the label names the union
		if l := itemTitleLabelRe.FindStringSubmatch(r.r.Raw); l != nil {
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
		alts = append(alts, programs.AndNode(&types.ReqNode{Type: types.NodeCredits, Amount: pool.Amount, Levels: levels},
			TextNode(types.TextAmbiguity, "which joint courses count isn't said: "+n.Items[at].Text)))
	}
	rest := slices.Delete(slices.Clone(n.Items[1:]), at-1, at)
	r.r.Req = programs.OrNode(append(alts, rest...)...)
	return true
}

// poolTexts replaces each text node of kind other in r's body that
// poolRewrite makes readable by the item grammar.
func (a *Assembler) poolTexts(r *compRow, ls compListRefs) {
	if n := r.r.Req; n.Type == types.NodeAnd && len(n.Items) == 2 && n.Items[0].Type == types.NodeText && n.Items[1].Type == types.NodeText &&
		n.Items[0].Kind == types.TextOther && excludesRe.MatchString(Clean(n.Items[1].Text)) {
		// a pool then "Excludes BCH472Y1, ...": the pool's exclusions
		t := strings.TrimSuffix(Clean(n.Items[0].Text), ".") + ", excluding " + excludesRe.FindStringSubmatch(Clean(n.Items[1].Text))[1]
		if x, rows, notes, ok := a.poolText(r, t, ls); ok {
			r.r.Req = TextNode(types.TextOther, t)
			r.r.Req, r.r.Notes = x, grammar.Dedupe(append(r.r.Notes, notes...))
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
			if m := excludesRe.FindStringSubmatch(Clean(note)); m != nil {
				// "Excludes BCH472Y1, ..." on its own line: the pool's exclusions
				text = strings.TrimSuffix(Clean(text), ".") + ", excluding " + m[1]
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
			r.r.Notes = grammar.Dedupe(append(r.r.Notes, notes...))
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
					r.r.Notes = grammar.Dedupe(append(r.r.Notes, notes...))
				}
			}
		}
	}
	walk(r.r.Req)
}

// Rewrites that bring a pool's wording to the item grammar's.
var (
	// countCreditsRe matches a course count with its credits: "Any 6
	// courses (3.0 credits) from".
	countCreditsRe = regexp.MustCompile(`(?i)^any\s+(?:\d+|one|two|three|four|five|six)\s+(?:half[- ]?)?courses?\s+(?:or\s+the\s+equivalent\s+)?\(` + grammar.AmountPat + `\s+credits?\)\s+from\b`)
	// countFromTheRe matches a count of courses: "Two courses from the BCH
	// 400-series lecture courses:".
	countFromTheRe = regexp.MustCompile(`(?i)^(two|three|four)\s+courses\s+from\s+the\s+[^:]+:\s*`)
	// noMoreRe matches a capped pool: "One lab course (0.5 credit), and no
	// more, taken from the following:".
	noMoreRe = regexp.MustCompile(`(?i)^one\s+[a-z]+\s+course\s+\(` + grammar.AmountPat + `\s+credits?\),\s+and\s+no\s+more,\s+taken\s+from(?:\s+the\s+following)?\s*:?`)
	// halfCoursesOfWhichRe matches a floor in two subjects: ", at least two
	// of which must be Math or Physics half courses".
	halfCoursesOfWhichRe = regexp.MustCompile(`(?i),\s*` + grammar.AtLeastPat + `\s+(one|two|three|four)\s+of\s+which\s+must\s+be\s+([A-Za-z]+)\s+or\s+([A-Za-z]+)\s+half\s+courses$`)
	// subjectCreditsRe matches a subject named in words: "2.0 credits in
	// Sociology".
	subjectCreditsRe = regexp.MustCompile(`(?i)\bcredits\s+in\s+([A-Z][a-z]+)\b`)
	// ofWhichIeRe matches a floor with its restatement: "(0.5 of which must
	// be taken at the 300-level, i.e., ...)".
	ofWhichIeRe = regexp.MustCompile(`(?i)\s*\(` + grammar.AmountPat + `\s+of\s+which\s+must\s+be\s+taken\s+at\s+the\s+(\d00)-level,\s*i\.e\.,[^()]*\)$`)
	// prefixCoursesRe matches courses named with their designator: "Peace,
	// Conflict and Justice courses (i.e. PCJ prefix classes)".
	prefixCoursesRe = regexp.MustCompile(`(?i)\b[A-Z][A-Za-z,\s]+?\s+courses\s+\(i\.e\.\s+([A-Z]{3})\s+prefix\s+classes\)`)
	// bareFloorRe matches a floor without the word credit: "at least 0.5 at
	// the 400 level".
	bareFloorRe = regexp.MustCompile(`(?i)\b(` + grammar.AtLeastPat + `\s+` + grammar.NumPat + `)(\s+at\s+the\s+\d00)`)
	// breadthOrRe matches a pool or a breadth category: "<pool> or 0.5
	// credit in any BR=5 course".
	breadthOrRe = regexp.MustCompile(`(?i)^(.+?)\s+or\s+(?:any\s+)?` + grammar.AmountPat + `\s+credits?\s+(?:in|from)\s+(?:any|an?)\s+(?:(\d00(?:/\d00)*)-level\s+)?(?:course\s+(?:from\s+)?)?(?:BR\s*=\s*|Breadth\s+(?:Requirement\s+)?Category\s+#?)(\d)\b(?:\s+course)?(?::\s*The\s+[A-Za-z&\s]+)?$`)
	// titledFromRe matches a title before the courses: "0.5 credit from
	// Quantitative Methods: ENG286H1/ ENG287H1".
	titledFromRe = regexp.MustCompile(`(?i)^(` + grammar.NumPat + `\s+credits?\s+from)\s+[A-Z][A-Za-z\s]+:\s+(` + grammar.CodePat + `)`)
)

// poolRewrite rewrites s into the item grammar's wording; caps reports an
// "and no more" cap on the program, and ok is false when no rule applies.
func poolRewrite(s string) (t string, caps bool, ok bool) {
	t = s
	if m := countCreditsRe.FindStringSubmatch(t); m != nil {
		t = m[1] + " credits from" + t[len(m[0]):]
	}
	if m := countFromTheRe.FindStringSubmatch(t); m != nil {
		// half courses only, so the count gives the credits
		if codes := grammar.CodeRe.FindAllString(t, -1); codes != nil && !slices.ContainsFunc(codes, func(c string) bool { return c[6] != 'H' }) {
			k := float64(grammar.CountWords[strings.ToLower(m[1])]) * 0.5
			t = strconv.FormatFloat(k, 'f', 1, 64) + " credits from " + t[len(m[0]):]
		}
	}
	if m := noMoreRe.FindStringSubmatch(t); m != nil {
		t = m[1] + " credit from" + t[len(m[0]):]
		caps = true
	}
	if m := halfCoursesOfWhichRe.FindStringSubmatchIndex(t); m != nil {
		d1, d2 := grammar.SubjectDesignators[strings.ToLower(t[m[4]:m[5]])], grammar.SubjectDesignators[strings.ToLower(t[m[6]:m[7]])]
		if d1 == "" || d2 == "" {
			return s, false, false
		}
		k := float64(grammar.CountWords[strings.ToLower(t[m[2]:m[3]])]) * 0.5
		t = t[:m[0]] + ", including at least " + strconv.FormatFloat(k, 'f', 1, 64) + " credit in " + d1 + " or " + d2 + " courses"
	}
	if m := subjectCreditsRe.FindStringSubmatchIndex(t); m != nil {
		if d := grammar.SubjectDesignators[strings.ToLower(t[m[2]:m[3]])]; d != "" {
			t = t[:m[0]] + d + " credits" + t[m[1]:]
			t = strings.Replace(t, "additional "+d, d, 1)
		}
	}
	if m := ofWhichIeRe.FindStringSubmatch(t); m != nil {
		t = t[:len(t)-len(m[0])] + ", including at least " + m[1] + " credit at the " + m[2] + "-level"
	}
	if m := prefixCoursesRe.FindStringSubmatchIndex(t); m != nil {
		t = t[:m[0]] + t[m[2]:m[3]] + " courses" + t[m[1]:]
	}
	t = bareFloorRe.ReplaceAllString(t, "$1 credit$2")
	return t, caps, t != s
}

// poolText reads a text node of row r again: as a breadth alternative, filter
// members, embedded pools, or poolRewrite then the item grammar. A reading
// that loses a course code is no reading.
func (a *Assembler) poolText(r *compRow, text string, ls compListRefs) (n *types.ReqNode, rows []*types.Requirement, notes []string, ok bool) {
	s := Clean(text)
	defer func() {
		if ok && !a.keepsCodes(s, n, rows, notes) {
			n, rows, notes, ok = nil, nil, nil, false
		}
	}()
	if m := breadthOrRe.FindStringSubmatch(s); m != nil {
		left := titledFromRe.ReplaceAllString(m[1], "$1 $2")
		body, rs, ns := a.itemBody(left)
		if body == nil || hasText(body) || rs != nil {
			return nil, nil, nil, false
		}
		amount, _ := strconv.ParseFloat(m[2], 64)
		cat, _ := strconv.Atoi(m[4])
		var levels []int
		if m[3] != "" {
			levels = grammar.ParseLevels(m[3] + "-level")
		}
		return programs.OrNode(body, &types.ReqNode{Type: types.NodeCredits, Amount: amount, Breadth: []int{cat}, Levels: levels}), nil, ns, true
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
	// atLeastOneOfRe matches "at least one 0.5 credit from PCL367H1 or
	// PCL368H1".
	atLeastOneOfRe = regexp.MustCompile(`,?\s*` + grammar.AtLeastPat + `\s+one\s+0\.5\s+credit\s+from\s+(` + grammar.CodePat + `)\s+or\s+(` + grammar.CodePat + `)`)
	// seeNoteRe matches "(see NOTE 2)".
	seeNoteRe = regexp.MustCompile(`(?i)\s*\(see\s+note(?:\s+\d+)?\)`)
)

// embeddedPools reads a course list with a pool inside it ("BCH311H1/ CSB349H1/
// MGY311Y1; 2.5 credits from MGY314H1, ...") as the and of its parts.
func (a *Assembler) embeddedPools(s string) (*types.ReqNode, []string, bool) {
	t := seeNoteRe.ReplaceAllString(s, "")
	t = atLeastOneOfRe.ReplaceAllString(t, ", $1/ $2")
	parts := grammar.SplitTop(t, ';')
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
	return programs.AndNode(nodes...), notes, true
}

// excludesRe matches exclusions after a pool: "Excludes BCH472Y1,
// BCH473Y1".
var excludesRe = regexp.MustCompile(`^Excludes\s+((?:` + grammar.CodePat + `(?:\s*,\s*|\s+and\s+)?)+)$`)

// fromListCapNoteRe matches a note letting part of a pool come from
// another list: "One course (0.5 credit) can be from Group F".
var fromListCapNoteRe = regexp.MustCompile(`(?i)^(?:one|a)\s+course\s+\(` + grammar.AmountPat + `\s+credits?\)\s+can\s+be\s+from\s+(.+)$`)

// capNotes adds to pool n the lists r's notes let part of it come from, with a
// limit on that part. An unresolved list leaves the pool unread (false): read
// alone, it would be NotMet for a student who used it.
func (a *Assembler) capNotes(r *compRow, n *types.ReqNode, ls compListRefs) bool {
	var keep []string
	for _, note := range r.r.Notes {
		m := fromListCapNoteRe.FindStringSubmatch(Clean(note))
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
		a.rowUnder(r, &types.Requirement{Kind: types.RequirementLimit, Req: &types.ReqNode{Type: types.NodeCredits, Lists: names, MaxAmount: &max}, Raw: grammar.CleanNote(note)})
	}
	r.r.Notes = keep
	return true
}

// keepsCodes reports whether every course code of s is in the reading or in a
// union list it made.
func (a *Assembler) keepsCodes(s string, n *types.ReqNode, rows []*types.Requirement, notes []string) bool {
	have := map[string]bool{}
	var add func(x *types.ReqNode)
	add = func(x *types.ReqNode) {
		grammar.WalkReq(x, func(y *types.ReqNode) {
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
	for _, c := range grammar.CodeRe.FindAllString(strings.Join(notes, " "), -1) {
		have[c] = true
	}
	for _, c := range grammar.CodeRe.FindAllString(s, -1) {
		if !have[c] {
			return false
		}
	}
	return true
}

var (
	// filterUnionLeadRe matches a pool lead with an optional floor: "1.5
	// credits, including at least 0.5 credit at the 400-level, from <members>".
	filterUnionLeadRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+(?:additional\s+)?credits?(?:\s*,?\s*(including\s+` + grammar.AtLeastPat + `\s+[^,:]+?)\s*,?)?\s+(?:from|of|selected\s+from)\s*:?\s+(.+)$`)
	// memberFilterRe matches a filter among a pool's members: "PSL400-series
	// courses". Each alternative has its own groups; filterMembers indexes them.
	memberFilterRe = regexp.MustCompile(`(?:[Aa]ny\s+(?:other\s+)?|further\s+)?(?:\b([A-Z]{3})\s?([1-4])00(\+)?[- ]?(?:series|level)\s+courses?|\b([A-Z]{3})\s+at\s+the\s+([1-4])00(\+)?\s+level|\b([A-Z]{3})\s+coded\s*([1-4])00\s+level\s+courses|courses\s+carrying\s+the\s+([A-Z]{3})\s+designator|\b([A-Z]{3}(?:\s+or\s+[A-Z]{3})*)\s+(half-)?courses?\s+at\s+the\s+([1-4])00(\+)?\s+level)`)
	// recommendedAsideRe matches advice after a filter: "( BCH422H1, BCH425H1
	// recommended)".
	recommendedAsideRe = regexp.MustCompile(`\s*\(([^()]*\brecommended)\)`)
	// excludingTailRe matches a closing exclusion: ", excluding JPH311H1,
	// JPH441H1".
	excludingTailRe = regexp.MustCompile(`(?i),?\s*excluding\s+((?:` + grammar.CodePat + `(?:\s*,\s*|\s+and\s+|\s*$))+)$`)
	// brokenCodeRe matches a code with its last digit missing, which leaves
	// the members unreadable: "CSB344H".
	brokenCodeRe = regexp.MustCompile(`\b[A-Z]{3}\d{3}[HY](?:[^0-9]|$)`)
	// memberSepRe matches what may sit between members once the filters are
	// taken out: ", or ".
	memberSepRe = regexp.MustCompile(`^[\s,;/]*(?:(?:or|and)\b[\s,;/]*)?$`)
)

// filterMembers reads a pool of courses and filters ("1.5 credits from
// BCH370H1/ CSB428H1/ BCH 400-level courses") as the row's own union list.
// A lead floor is a constraint row; members that are only courses are left.
func (a *Assembler) filterMembers(r *compRow, s string, ls compListRefs) (n *types.ReqNode, rows []*types.Requirement, notes []string, ok bool) {
	m := filterUnionLeadRe.FindStringSubmatch(s)
	if m == nil || brokenCodeRe.MatchString(s) {
		return nil, nil, nil, false
	}
	members := m[3]
	for _, x := range recommendedAsideRe.FindAllStringSubmatch(members, -1) {
		notes = append(notes, strings.TrimSpace(x[1]))
	}
	members = recommendedAsideRe.ReplaceAllString(members, "")
	var excluded []string
	if x := excludingTailRe.FindStringSubmatchIndex(members); x != nil {
		excluded = grammar.CodeRe.FindAllString(members[x[2]:x[3]], -1)
		members = members[:x[0]]
	}
	locs := memberFilterRe.FindAllStringSubmatchIndex(members, -1)
	if locs == nil {
		return nil, nil, nil, false
	}
	var items []*types.ReqNode
	var rest strings.Builder
	last := 0
	for _, l := range locs {
		gap := members[last:l[0]]
		if !grammar.CodeRe.MatchString(gap) && !memberSepRe.MatchString(gap) {
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
			it.Levels = grammar.ParseLevels(level + "00" + plus + "-level")
		}
		for _, c := range excluded {
			if c[:3] == desig {
				it.Exclude = append(it.Exclude, c)
			}
		}
		items = append(items, it)
	}
	tail := members[last:]
	if !grammar.CodeRe.MatchString(tail) && !memberSepRe.MatchString(tail) {
		return nil, nil, nil, false
	}
	rest.WriteString(tail)
	if codes := grammar.CodeRe.FindAllString(rest.String(), -1); codes != nil {
		if strings.Trim(grammar.CodeRe.ReplaceAllString(rest.String(), ""), " ,;/()orand") != "" {
			return nil, nil, nil, false
		}
		items = append([]*types.ReqNode{{Type: types.NodeCredits, Courses: grammar.Dedupe(codes)}}, items...)
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
				x.Levels = grammar.Dedupe(append(x.Levels, y.Levels...))
				slices.Sort(x.Levels)
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
		f, _, _ := a.itemBody(bareFloorRe.ReplaceAllString(strings.TrimPrefix(strings.TrimSpace(strings.Replace(m[2], "including at least", "", 1)), "a "), "$1 credit$2"))
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

// filterPools reads the filter pools in the rows and returns lists with the
// lists they add.
func (a *Assembler) filterPools(lists []*types.CourseList) []*types.CourseList {
	for _, row := range a.allRows() {
		a.otherSeries(row)
		lists = a.electiveKinds(row, lists)
	}
	return lists
}

// otherSeriesRe matches a designator's series plus listed courses: "An
// additional 1.0 credit at the 4th year level from the following list: any
// other BCH 400-series course, BCB420H1".
var otherSeriesRe = regexp.MustCompile(`(?i)^(?:an\s+)?additional\s+` + grammar.AmountPat + `\s+credits?\s+at\s+the\s+(\d)(?:st|nd|rd|th)\s+year\s+level\s+from\s+the\s+following\s+list:\s*any\s+other\s+(` + grammar.DesigPat + `)\s+(\d)00-series\s+courses?(?:\s+including\s+([^,(]+?))?(?:\s*\(\s*see\s+note\s+\d+\s*\))?\s*,\s*(.+)$`)

// otherSeries encodes a row matching otherSeriesRe as credits{amount,
// designators, levels, courses}. "Other" is the claims rule; what the series
// "includes" is already in it.
func (a *Assembler) otherSeries(row *compRow) {
	n := row.r.Req
	if row.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || len(row.children) > 0 {
		return
	}
	m := otherSeriesRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(Clean(n.Text)), "."))
	if m == nil || m[2] != m[4] {
		return
	}
	if m[5] != "" && !regexp.MustCompile(`(?i)^the\s+`+m[2]+`(?:st|nd|rd|th)\s+year\s+`+m[3]+`\s+[\w\s-]*courses$`).MatchString(strings.TrimSpace(m[5])) {
		return
	}
	codes := anyCodeRe.FindAllString(m[6], -1)
	level := []int{100 * int(m[4][0]-'0')}
	if codes == nil || !listRestRe.MatchString(anyCodeRe.ReplaceAllString(m[6], " ")) ||
		slices.ContainsFunc(codes, func(c string) bool { return !grammar.LevelMatches(level, c) }) {
		return
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	row.r.Req = &types.ReqNode{Type: types.NodeCredits, Amount: amount, Designators: []string{m[3]}, Levels: level, Courses: grammar.Dedupe(codes)}
}

var (
	// electiveKindsRe matches electives with a floor on one kind: "2.5
	// credits from electives in composition, ..., of which at least 1.5 credits
	// must be completed from composition courses: CMU113H1".
	electiveKindsRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits\s+from\s+electives\s+in\s+.+?,\s+of\s+which\s+` + grammar.AtLeastPat + `\s+` + grammar.AmountPat + `\s+credits?\s+must\s+be\s+completed\s+from\s+(.+?\s+courses):\s*(.+)$`)
	// levelPlusMemberRe matches a member: "HMU200+ level courses".
	levelPlusMemberRe = regexp.MustCompile(`^([A-Z]{3})\s*([1-4])00\+\s*level\s+courses$`)
	// kindCoursesRe matches a note listing one kind of courses: "Music
	// history courses: HMU200+ level courses".
	kindCoursesRe = regexp.MustCompile(`^([A-Z][^:]*?\scourses):\s*(.+)$`)
)

// electiveKinds encodes a row matching electiveKindsRe, with a note
// listing the other kind's courses, as credits over both kinds' lists with the
// floor's kind as a constraint under it.
func (a *Assembler) electiveKinds(row *compRow, lists []*types.CourseList) []*types.CourseList {
	n := row.r.Req
	if row.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || len(row.children) > 0 || len(row.r.Notes) == 0 {
		return lists
	}
	m := electiveKindsRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(Clean(n.Text)), "."))
	if m == nil {
		return lists
	}
	codes := anyCodeRe.FindAllString(m[4], -1)
	if codes == nil || !listRestRe.MatchString(anyCodeRe.ReplaceAllString(m[4], " ")) {
		return lists
	}
	var other *types.CourseList
	var note string
	for _, s := range row.r.Notes {
		g := kindCoursesRe.FindStringSubmatch(strings.TrimSpace(s))
		if g == nil || other != nil {
			continue
		}
		var items []*types.ReqNode
		var plain []string
		for _, part := range strings.Split(strings.TrimSuffix(g[2], "."), ";") {
			part = strings.TrimSpace(part)
			if f := levelPlusMemberRe.FindStringSubmatch(part); f != nil {
				var levels []int
				for lv := int(f[2][0]-'0') * 100; lv <= 400; lv += 100 {
					levels = append(levels, lv)
				}
				items = append(items, &types.ReqNode{Type: types.NodeCredits, Designators: []string{f[1]}, Levels: levels})
				continue
			}
			if !grammar.CodeRe.MatchString(part) || grammar.CodeRe.FindString(part) != part {
				return lists
			}
			plain = append(plain, part)
		}
		if plain != nil {
			items = append(items, &types.ReqNode{Type: types.NodeCredits, Courses: plain})
		}
		other, note = &types.CourseList{Name: g[1], Items: items}, s
	}
	floorName := strings.ToUpper(m[3][:1]) + m[3][1:]
	if other == nil || strings.EqualFold(floorName, other.Name) ||
		slices.ContainsFunc(lists, func(l *types.CourseList) bool { return l.Name == floorName || l.Name == other.Name }) {
		return lists
	}
	lists = append(lists, &types.CourseList{Name: floorName, Items: []*types.ReqNode{{Type: types.NodeCredits, Courses: grammar.Dedupe(codes)}}}, other)
	amount, _ := strconv.ParseFloat(m[1], 64)
	floor, _ := strconv.ParseFloat(m[2], 64)
	row.r.Req = &types.ReqNode{Type: types.NodeCredits, Amount: amount, Lists: []string{floorName, other.Name}}
	row.children = append(row.children, &compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Raw: RawHead(row.r),
		Req: &types.ReqNode{Type: types.NodeCredits, Amount: floor, Lists: []string{floorName}}}})
	row.r.Notes = slices.DeleteFunc(row.r.Notes, func(s string) bool { return s == note })
	addRaw(row, note)
	return lists
}
