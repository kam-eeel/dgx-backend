package reqs

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Binding notes: notes that read as rules, either joined to the raw of the
// rows that restate them, encoded as rows, or made text alternatives.

var (
	// complNotFulfilRe matches a note that an item doesn't count toward another:
	// "this does not fulfil the 400-level requirement as described in #5 above".
	complNotFulfilRe = regexp.MustCompile(`(?i)^this\s+does\s+not\s+fulfil+\s+the\s+` + levelPat + `-level\s+requirement\s+as\s+described\s+in\s+#(\d+)\s+above$`)
	// complCategoryNoteRe matches a bracketed category label after an item's
	// pool: "History & Society courses".
	complCategoryNoteRe = regexp.MustCompile(`^[A-Z][\w&,\s]*\bcourses$`)
	// complAnnouncedRe matches a requirement announced before the sentence that
	// states it: "An Integrative, Inquiry-Based Activity Requirement must be
	// satisfied".
	complAnnouncedRe = regexp.MustCompile(`(?i)^an?\s+(\w+)[\w,\s-]*\s+requirement\s+must\s+be\s+satisfied$`)
	// complMayCountUpToRe matches a cap the line's next sentence restates:
	// "Students may count up to 1.0 credits from a biochemistry research project
	// course".
	complMayCountUpToRe = regexp.MustCompile(`(?i)^students\s+may\s+count\s+up\s+to\s+` + amountPat + `\s+credits?\s+from\s+an?\s+[\w\s-]+\s+course\b`)
	// complHalfMarkRe matches what a list's half-credit mark means: "The HIS
	// courses listed with (.50) indicate that they fulfill half of the Temporal
	// Requirement". The group is the list's name.
	complHalfMarkRe = regexp.MustCompile(`(?i)^(?:the\s+)?[A-Z]{3}\s+courses\s+listed\s+with\s+\(\.50\)\s+indicate\s+that\s+they\s+fulfil+\s+half\s+of\s+the\s+(.+)$`)
	// complMayCountEitherRe matches a note that one of two courses counts:
	// "Students may count either CHM395Y1 or CHM499Y1 towards".
	complMayCountEitherRe = regexp.MustCompile(`(?i)^students\s+may\s+count\s+either\s+(` + codePat + `)\s+or\s+(` + codePat + `)\s+towards?\b`)
	// complSeekApprovalRe matches an approval route for other courses: "Other
	// 300+ level courses with American content may be allowed; students should
	// seek early approval".
	complSeekApprovalRe = regexp.MustCompile(`(?i)^other\s+` + levelPat + `\+\s+level\s+courses\s+with\s+.+\bmay\s+be\s+allowed;\s+students\s+should\s+seek\s+(?:early\s+)?approval\b`)
	// complNotExhaustiveRe matches the Review alternative openLists gives a
	// non-exhaustive list: "This list of eligible courses is not exhaustive".
	complNotExhaustiveRe = regexp.MustCompile(`(?i)^this\s+list\s+of\s+eligible\s+courses\s+is\s+not\s+exhaustive\b`)
	// complAlreadyChosenRe matches a distribution bullet for a third language
	// credit: "An additional 1.0 credit in the single language already chosen, or
	// 1.0 credit in non-language courses".
	complAlreadyChosenRe = regexp.MustCompile(`(?i)^an\s+additional\s+` + amountPat + `\s+credits?\s+in\s+the\s+single\s+language\s+already\s+chosen,\s+or\s+` + numPat + `\s+credits?\s+in\s+non-language\s+courses\b`)
	// complRemainingListRe matches a distribution bullet for credits from a
	// list: "The remaining 1.5 credits in elective courses chosen from a list".
	complRemainingListRe = regexp.MustCompile(`(?i)^the\s+remaining\s+` + amountPat + `\s+credits\s+in\s+elective\s+courses\s+chosen\s+from\s+a\s+list\b`)
	// complMistypedCodeRe matches a token shaped like a code, which may not be
	// one: "CAR32H1".
	complMistypedCodeRe = regexp.MustCompile(`\b[A-Z]{3}[0-9][0-9A-Z]{0,3}[HY][0-9]\b`)
)

// bindingNotes encodes or drops the binding notes left once the rows, routes
// and caps are placed.
func (a *compAssembler) bindingNotes(lists []*types.CourseList) []*types.CourseList {
	a.mistypedMembers()
	lists = a.restatedListDefs(lists)
	a.restatedRowNotes(lists)
	a.restatedProgramNotes(lists)
	a.eitherCounts()
	a.noteRoutes()
	a.distributionBullets()
	a.restatedOmission()
	a.cognateRouteTotal()
	a.enrolledException()
	a.repeatTopic(lists)
	return lists
}

// dropNoteAll takes note out of the program's and the rows' notes.
func (a *compAssembler) dropNoteAll(note string) {
	same := func(s string) bool { return rawWords(s) == rawWords(note) }
	a.notes = slices.DeleteFunc(a.notes, same)
	for _, r := range a.allRows() {
		if r.r.Notes = slices.DeleteFunc(r.r.Notes, same); len(r.r.Notes) == 0 {
			r.r.Notes = nil
		}
	}
}

// hasNote reports whether note is still one of the program's or the rows'
// notes.
func (a *compAssembler) hasNote(note string) bool {
	same := func(s string) bool { return rawWords(s) == rawWords(note) }
	if slices.ContainsFunc(a.notes, same) {
		return true
	}
	return slices.ContainsFunc(a.allRows(), func(r *compRow) bool { return slices.ContainsFunc(r.r.Notes, same) })
}

// namedCodes returns the codes n names: its courses and its pools' courses.
func namedCodes(n *types.ReqNode) []string {
	var out []string
	walkReq(n, func(x *types.ReqNode) {
		switch x.Type {
		case types.NodeCourse:
			out = append(out, x.Code)
		case types.NodeCredits:
			out = append(out, x.Courses...)
		}
	})
	return out
}

// mistypedMembers makes a list definition kept as a note because a member is
// mistyped ("CAR32H1") a text alternative of its row: a student who took the
// course meant is Review, not NotMet.
func (a *compAssembler) mistypedMembers() {
	for _, d := range a.listDefs {
		if d.row == nil || d.row.r.Req == nil || !slices.Contains(d.row.r.Notes, d.note) || !slices.ContainsFunc(complMistypedCodeRe.FindAllString(d.members, -1), func(c string) bool { return !codeRe.MatchString(c) }) {
			continue
		}
		d.row.r.Req = orNode(d.row.r.Req, textNode(types.TextOther, d.note))
		a.dropNoteAll(d.note)
	}
}

// restatedListDefs drops the binding definitions of lists no row uses by
// name whose courses the rows already name, and makes a pool whose courses
// are the list's members at its levels draw on the list instead. Any other
// stays a note as written. It returns the lists with any it adds.
func (a *compAssembler) restatedListDefs(lists []*types.CourseList) []*types.CourseList {
	named := map[string]bool{}
	var texts []string
	for _, r := range a.allRows() {
		for _, c := range namedCodes(r.r.Req) {
			named[c] = true
		}
		walkReq(r.r.Req, func(x *types.ReqNode) {
			if x.Type == types.NodeText {
				texts = append(texts, strings.ToLower(x.Text))
			}
		})
	}
	for _, l := range lists {
		for _, it := range l.Items {
			for _, c := range namedCodes(it) {
				named[c] = true
			}
		}
	}
	for _, d := range a.listDefs {
		if !a.hasNote(d.note) || !slices.ContainsFunc(splitComplSentences(d.note), func(x string) bool { return a.role(x) != roleAdvisory }) || complListFilterRe.MatchString(d.members) || complListPermRe.MatchString(d.members) {
			continue
		}
		codes := dedupe(codeRe.FindAllString(d.members, -1))
		if codes == nil || slices.ContainsFunc(complMistypedCodeRe.FindAllString(d.members, -1), func(c string) bool { return !codeRe.MatchString(c) }) {
			continue
		}
		title := strings.ToLower(strings.TrimSpace(d.title))
		name, _ := listName(d.title)
		if slices.ContainsFunc(texts, func(t string) bool {
			return strings.Contains(t, title) || name != "" && strings.Contains(t, strings.ToLower(name))
		}) {
			continue // a row of text names the list: it should use it
		}
		if !slices.ContainsFunc(codes, func(c string) bool { return !named[c] }) {
			a.dropNoteAll(d.note)
			continue
		}
		stem := strings.TrimSuffix(title, "s")
		if l := a.listPool(d, codes, stem, name); l != nil {
			lists = append(lists, l)
			a.dropNoteAll(d.note)
			continue
		}
	}
	return lists
}

// listPool makes the pool of an item whose raw names the list (stem), and
// whose courses are the list's members at the pool's levels, draw on the list.
// It returns that list, or nil.
func (a *compAssembler) listPool(d *compListDef, codes []string, stem, name string) *types.CourseList {
	if name == "" {
		name = strings.TrimSpace(d.title)
	}
	items, except, perm, ok := listMembersPerm(d.members)
	if !ok || except != nil || perm != "" || len(items) != 1 || items[0].Type != types.NodeCredits || items[0].Courses == nil {
		return nil
	}
	for _, r := range a.items {
		if r.r.Kind != types.RequirementAllocation || !strings.Contains(strings.ToLower(r.r.Raw), stem) {
			continue
		}
		var pool *types.ReqNode
		walkReq(r.r.Req, func(x *types.ReqNode) {
			if pool != nil || x.Type != types.NodeCredits || x.Levels == nil || len(x.Courses) < 2 || x.Lists != nil || x.Designators != nil {
				return
			}
			at := slices.DeleteFunc(slices.Clone(codes), func(c string) bool { return !levelMatches(x.Levels, c) })
			if slices.Equal(slices.Sorted(slices.Values(at)), slices.Sorted(slices.Values(x.Courses))) {
				pool = x
			}
		})
		if pool != nil {
			pool.Courses, pool.Lists = nil, []string{name}
			return &types.CourseList{Name: name, Items: items}
		}
	}
	return nil
}

// restatedRowNotes drops a row's notes that its raw holds and its rows say: a
// bracketed category label, or "this does not fulfil ... #5 above" when item
// 5's pool holds none of the row's courses.
func (a *compAssembler) restatedRowNotes(lists []*types.CourseList) {
	byName := listsByName(lists)
	// codes returns the courses n names, its pools' lists' too; all is false when
	// a pool takes courses it doesn't name (a designator's, any courses).
	codes := func(n *types.ReqNode) (out []string, all bool) {
		out, all = namedCodes(n), true
		walkReq(n, func(x *types.ReqNode) {
			if x.Type != types.NodeCredits {
				return
			}
			cs, ok := a.listCodes(x.Lists, byName)
			out = append(out, cs...)
			all = all && x.Designators == nil && (x.Courses != nil || x.Lists != nil) && (x.Lists == nil || ok)
		})
		return out, all
	}
	for _, r := range a.allRows() {
		r.r.Notes = slices.DeleteFunc(r.r.Notes, func(n string) bool {
			if complCategoryNoteRe.MatchString(n) && strings.Contains(rawWords(r.r.Raw), "("+n+")") {
				return true
			}
			m := complNotFulfilRe.FindStringSubmatch(n)
			if m == nil {
				return false
			}
			own, _ := codes(r.r.Req)
			for _, q := range a.items {
				if q != r && q.r.Label == m[1] && q.r.Kind == types.RequirementAllocation {
					theirs, all := codes(q.r.Req)
					return own != nil && theirs != nil && all && !slices.ContainsFunc(own, func(c string) bool { return slices.Contains(theirs, c) })
				}
			}
			return false
		})
		if len(r.r.Notes) == 0 {
			r.r.Notes = nil
		}
	}
}

// restatedProgramNotes joins to their rows' raw the program notes that
// restate them: complAnnouncedRe, complMayCountUpToRe and complHalfMarkRe.
func (a *compAssembler) restatedProgramNotes(lists []*types.CourseList) {
	rows := a.allRows()
	a.notes = slices.DeleteFunc(a.notes, func(n string) bool {
		t := a.clean(n)
		var target *compRow
		switch {
		case complAnnouncedRe.MatchString(t):
			word := strings.ToLower(complAnnouncedRe.FindStringSubmatch(t)[1])
			for _, r := range rows {
				if strings.Contains(strings.ToLower(r.r.Raw), "requirement for an "+word) {
					target = r
					break
				}
			}
		case complMayCountUpToRe.MatchString(t):
			v, _ := strconv.ParseFloat(complMayCountUpToRe.FindStringSubmatch(t)[1], 64)
			for _, r := range rows {
				if r.r.Kind != types.RequirementLimit || r.r.Req == nil || r.r.Req.MaxAmount == nil || math.Abs(*r.r.Req.MaxAmount-v) > 1e-9 {
					continue
				}
				if slices.ContainsFunc(a.lines, func(l string) bool {
					w := rawWords(l)
					return strings.Contains(w, rawWords(n)) && strings.Contains(w, rawWords(r.r.Raw))
				}) {
					target = r
					break
				}
			}
		case complHalfMarkRe.MatchString(t):
			name := complHalfMarkRe.FindStringSubmatch(t)[1]
			i := slices.IndexFunc(lists, func(l *types.CourseList) bool { return strings.EqualFold(l.Name, name) })
			if i < 0 || !slices.ContainsFunc(lists[i].Items, func(x *types.ReqNode) bool { return x.Weight == 0.5 }) {
				return false
			}
			for _, r := range rows {
				walkReq(r.r.Req, func(x *types.ReqNode) {
					if target == nil && x.Type == types.NodeCredits && slices.Contains(x.Lists, lists[i].Name) {
						target = r
					}
				})
			}
		}
		if target == nil {
			return false
		}
		addRaw(target, n)
		return true
	})
}

// eitherCounts reads complMayCountEitherRe as a limit of one course's weight
// on the two, under the total.
func (a *compAssembler) eitherCounts() {
	if a.total == nil {
		return
	}
	a.notes = slices.DeleteFunc(a.notes, func(n string) bool {
		m := complMayCountEitherRe.FindStringSubmatch(a.clean(n))
		if m == nil || courseWeight(m[1]) != courseWeight(m[2]) {
			return false
		}
		a.rowUnder(a.total, &types.Requirement{Kind: types.RequirementLimit, Raw: n,
			Req: &types.ReqNode{Type: types.NodeCredits, Courses: []string{m[1], m[2]}, MaxAmount: maxAmount(courseWeight(m[1]))}})
		return true
	})
}

// noteRoutes adds a complSeekApprovalRe note as a permission alternative
// beside every not-exhaustive Review alternative, which it explains.
func (a *compAssembler) noteRoutes() {
	a.notes = slices.DeleteFunc(a.notes, func(n string) bool {
		t := a.clean(n)
		switch {
		case complSeekApprovalRe.MatchString(t):
			done := false
			for _, r := range a.allRows() {
				walkReq(r.r.Req, func(x *types.ReqNode) {
					if x.Type == types.NodeOr && slices.ContainsFunc(x.Items, func(it *types.ReqNode) bool {
						return it.Type == types.NodeText && complNotExhaustiveRe.MatchString(it.Text)
					}) {
						x.Items = append(x.Items, textNode(types.TextPermission, n))
						done = true
					}
				})
			}
			return done
		}
		return false
	})
}

// distributionBullets reads the bullets under a distribution summary: one
// whose courses the items name, or whose remaining credits an item draws from
// a list, joins those items' raw; complAlreadyChosenRe adds a same-language
// constraint (sameLanguage).
func (a *compAssembler) distributionBullets() {
	lead := slices.IndexFunc(a.lines, func(l string) bool { return complDistributionRe.MatchString(complClean(strings.TrimSpace(l))) })
	if lead < 0 {
		return
	}
	var bullets []string
	for _, l := range a.lines[lead+1:] {
		l = strings.TrimSpace(l)
		if !strings.HasPrefix(l, "-") {
			break
		}
		bullets = append(bullets, splitComplSentences(strings.TrimSpace(strings.TrimPrefix(l, "-")))...)
	}
	for _, s := range bullets {
		note := cleanNote(s)
		if !slices.ContainsFunc(a.notes, func(x string) bool { return rawWords(x) == rawWords(note) }) {
			continue
		}
		t := a.clean(s)
		if m := complAlreadyChosenRe.FindStringSubmatch(t); m != nil {
			if c := a.sameLanguage(m[1], note); c != nil {
				a.program = append(a.program, &compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Req: c, Raw: s}})
				a.dropNoteAll(note)
			}
			continue
		}
		var targets []*compRow
		codes := dedupe(codeRe.FindAllString(t, -1))
		for _, r := range a.items {
			if r.r.Kind != types.RequirementAllocation {
				continue
			}
			own := namedCodes(r.r.Req)
			if codes != nil && slices.ContainsFunc(codes, func(c string) bool { return slices.Contains(own, c) }) {
				targets = append(targets, r)
			}
			if m := complRemainingListRe.FindStringSubmatch(t); m != nil && codes == nil && r.r.Req != nil {
				pool := r.r.Req
				if pool.Type == types.NodeOr {
					pool = pool.Items[0]
				}
				if v, _ := strconv.ParseFloat(m[1], 64); pool.Type == types.NodeCredits && pool.Lists != nil && math.Abs(pool.Amount-v) < 1e-9 {
					targets = append(targets, r)
				}
			}
		}
		covered := func(c string) bool {
			return slices.ContainsFunc(targets, func(r *compRow) bool { return slices.Contains(namedCodes(r.r.Req), c) })
		}
		if targets == nil || slices.ContainsFunc(codes, func(c string) bool { return !covered(c) }) {
			continue
		}
		for _, r := range targets {
			addRaw(r, s)
		}
		a.dropNoteAll(note)
	}
}

// sameLanguage builds the constraint of complAlreadyChosenRe: an or with, per
// language, and(its first, second and third year alternatives), and the
// bullet as content text. Nil when the blocks or the third item don't line up.
func (a *compAssembler) sameLanguage(amount, bullet string) *types.ReqNode {
	blocks := a.languageBlocks(3)
	if len(blocks) < 3 || fmtAmount(blocks[2].amount) != amount {
		return nil
	}
	head := rawWords(complClean(strings.TrimSpace(a.lines[blocks[2].line])))
	var third *compRow
	for _, r := range a.items {
		if rawWords(complClean(rawHead(r.r))) == head {
			third = r
		}
	}
	if third == nil || third.r.Req == nil || third.r.Req.Type != types.NodeOr || len(third.r.Req.Items) != len(blocks[2].names)+1 {
		return nil
	}
	var alts []*types.ReqNode
	for _, lang := range blocks[0].order {
		y2 := blocks[1].options[lang]
		i := slices.Index(blocks[2].names, lang)
		if y2 == nil || i < 0 {
			continue
		}
		alts = append(alts, andNode(cloneNode(blocks[0].options[lang]), cloneNode(y2), cloneNode(third.r.Req.Items[i])))
	}
	if len(alts) < 2 {
		return nil
	}
	return orNode(append(alts, textNode(types.TextContent, bullet))...)
}
