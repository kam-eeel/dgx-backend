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

// Binding notes: notes that read as rules, joined to the raw of the rows that
// restate them, encoded as rows, made text alternatives or dropped.

// restatedPools moves a note restating a lead-filter pool from its own row to
// the allocation of that pool.
func (a *Assembler) restatedPools() {
	for _, r := range a.allRows() {
		n := r.r.Req
		if r.r.Kind != types.RequirementConstraint || n == nil || n.Type != types.NodeText || len(r.children) > 0 {
			continue
		}
		m := anyCombinationRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(n.Text), "."))
		if m == nil {
			continue
		}
		for _, o := range a.allRows() {
			p := o.r.Req
			if p != nil && p.Type == types.NodeOr {
				p = p.Items[0]
			}
			if o.r.Kind != types.RequirementAllocation || p == nil || p.Type != types.NodeCredits || p.Lists == nil ||
				fmtAmount(p.Amount) != m[3] || !strings.Contains(o.r.Raw, m[1]+"/"+m[2]+" credits at the 300+ level") {
				continue
			}
			if a.detach(r) {
				addRaw(o, r.r.Raw)
				o.r.Notes = append(o.r.Notes, r.r.Notes...)
			}
			break
		}
	}
}

var (
	// notFulfilRe matches a note that an item doesn't count toward another:
	// "this does not fulfil the 400-level requirement as described in #5 above".
	notFulfilRe = regexp.MustCompile(`(?i)^this\s+does\s+not\s+fulfil+\s+the\s+` + grammar.LevelPat + `-level\s+requirement\s+as\s+described\s+in\s+#(\d+)\s+above$`)
	// categoryNoteRe matches a bracketed category label after an item's
	// pool: "History & Society courses".
	categoryNoteRe = regexp.MustCompile(`^[A-Z][\w&,\s]*\bcourses$`)
	// announcedRe matches a requirement announced before the sentence that
	// states it: "An Integrative, Inquiry-Based Activity Requirement must be
	// satisfied".
	announcedRe = regexp.MustCompile(`(?i)^an?\s+(\w+)[\w,\s-]*\s+requirement\s+must\s+be\s+satisfied$`)
	// mayCountUpToRe matches a cap the line's next sentence restates:
	// "Students may count up to 1.0 credits from a biochemistry research project
	// course".
	mayCountUpToRe = regexp.MustCompile(`(?i)^students\s+may\s+count\s+up\s+to\s+` + grammar.AmountPat + `\s+credits?\s+from\s+an?\s+[\w\s-]+\s+course\b`)
	// halfMarkRe matches what a list's half-credit mark means: "The HIS
	// courses listed with (.50) indicate that they fulfill half of the Temporal
	// Requirement". The group is the list's name.
	halfMarkRe = regexp.MustCompile(`(?i)^(?:the\s+)?[A-Z]{3}\s+courses\s+listed\s+with\s+\(\.50\)\s+indicate\s+that\s+they\s+fulfil+\s+half\s+of\s+the\s+(.+)$`)
	// mayCountEitherRe matches a note that one of two courses counts:
	// "Students may count either CHM395Y1 or CHM499Y1 towards".
	mayCountEitherRe = regexp.MustCompile(`(?i)^students\s+may\s+count\s+either\s+(` + grammar.CodePat + `)\s+or\s+(` + grammar.CodePat + `)\s+towards?\b`)
	// seekApprovalRe matches an approval route for other courses: "Other
	// 300+ level courses with American content may be allowed; students should
	// seek early approval".
	seekApprovalRe = regexp.MustCompile(`(?i)^other\s+` + grammar.LevelPat + `\+\s+level\s+courses\s+with\s+.+\bmay\s+be\s+allowed;\s+students\s+should\s+seek\s+(?:early\s+)?approval\b`)
	// notExhaustiveRe matches the Review alternative openLists gives a
	// non-exhaustive list: "This list of eligible courses is not exhaustive".
	notExhaustiveRe = regexp.MustCompile(`(?i)^this\s+list\s+of\s+eligible\s+courses\s+is\s+not\s+exhaustive\b`)
	// alreadyChosenRe matches a distribution bullet for a third language
	// credit: "An additional 1.0 credit in the single language already chosen, or
	// 1.0 credit in non-language courses".
	alreadyChosenRe = regexp.MustCompile(`(?i)^an\s+additional\s+` + grammar.AmountPat + `\s+credits?\s+in\s+the\s+single\s+language\s+already\s+chosen,\s+or\s+` + grammar.NumPat + `\s+credits?\s+in\s+non-language\s+courses\b`)
	// remainingListRe matches a distribution bullet for credits from a
	// list: "The remaining 1.5 credits in elective courses chosen from a list".
	remainingListRe = regexp.MustCompile(`(?i)^the\s+remaining\s+` + grammar.AmountPat + `\s+credits\s+in\s+elective\s+courses\s+chosen\s+from\s+a\s+list\b`)
	// mistypedCodeRe matches a token shaped like a code, which may not be
	// one: "CAR32H1".
	mistypedCodeRe = regexp.MustCompile(`\b[A-Z]{3}[0-9][0-9A-Z]{0,3}[HY][0-9]\b`)
)

// encodeBindingNotes encodes or drops the binding notes left once the rows,
// routes and caps are placed.
func (a *Assembler) encodeBindingNotes(lists []*types.CourseList) []*types.CourseList {
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
func (a *Assembler) dropNoteAll(note string) {
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
func (a *Assembler) hasNote(note string) bool {
	same := func(s string) bool { return rawWords(s) == rawWords(note) }
	if slices.ContainsFunc(a.notes, same) {
		return true
	}
	return slices.ContainsFunc(a.allRows(), func(r *compRow) bool { return slices.ContainsFunc(r.r.Notes, same) })
}

// namedCodes returns the codes n names: its courses and its pools' courses.
func namedCodes(n *types.ReqNode) []string {
	var out []string
	grammar.WalkReq(n, func(x *types.ReqNode) {
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
func (a *Assembler) mistypedMembers() {
	for _, d := range a.listDefs {
		if d.row == nil || d.row.r.Req == nil || !slices.Contains(d.row.r.Notes, d.note) || !slices.ContainsFunc(mistypedCodeRe.FindAllString(d.members, -1), func(c string) bool { return !grammar.CodeRe.MatchString(c) }) {
			continue
		}
		d.row.r.Req = programs.OrNode(d.row.r.Req, TextNode(types.TextOther, d.note))
		a.dropNoteAll(d.note)
	}
}

// restatedListDefs drops the binding definitions of lists no row uses by
// name whose courses the rows already name, and makes a pool whose courses
// are the list's members at its levels draw on the list instead. Any other
// stays a note as written. It returns the lists with any it adds.
func (a *Assembler) restatedListDefs(lists []*types.CourseList) []*types.CourseList {
	named := map[string]bool{}
	var texts []string
	for _, r := range a.allRows() {
		for _, c := range namedCodes(r.r.Req) {
			named[c] = true
		}
		grammar.WalkReq(r.r.Req, func(x *types.ReqNode) {
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
		if !a.hasNote(d.note) || !slices.ContainsFunc(SplitSentences(d.note), func(x string) bool { return a.role(x) != RoleAdvisory }) || listFilterRe.MatchString(d.members) || listPermRe.MatchString(d.members) {
			continue
		}
		codes := grammar.Dedupe(grammar.CodeRe.FindAllString(d.members, -1))
		if codes == nil || slices.ContainsFunc(mistypedCodeRe.FindAllString(d.members, -1), func(c string) bool { return !grammar.CodeRe.MatchString(c) }) {
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
func (a *Assembler) listPool(d *compListDef, codes []string, stem, name string) *types.CourseList {
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
		grammar.WalkReq(r.r.Req, func(x *types.ReqNode) {
			if pool != nil || x.Type != types.NodeCredits || x.Levels == nil || len(x.Courses) < 2 || x.Lists != nil || x.Designators != nil {
				return
			}
			at := slices.DeleteFunc(slices.Clone(codes), func(c string) bool { return !grammar.LevelMatches(x.Levels, c) })
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
func (a *Assembler) restatedRowNotes(lists []*types.CourseList) {
	byName := listsByName(lists)
	// codes returns the courses n names, its pools' lists' too; all is false when
	// a pool takes courses it doesn't name (a designator's, any courses).
	codes := func(n *types.ReqNode) (out []string, all bool) {
		out, all = namedCodes(n), true
		grammar.WalkReq(n, func(x *types.ReqNode) {
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
			if categoryNoteRe.MatchString(n) && strings.Contains(rawWords(r.r.Raw), "("+n+")") {
				return true
			}
			m := notFulfilRe.FindStringSubmatch(n)
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
// restate them: announcedRe, mayCountUpToRe and halfMarkRe.
func (a *Assembler) restatedProgramNotes(lists []*types.CourseList) {
	rows := a.allRows()
	a.notes = slices.DeleteFunc(a.notes, func(n string) bool {
		t := a.clean(n)
		var target *compRow
		switch {
		case announcedRe.MatchString(t):
			word := strings.ToLower(announcedRe.FindStringSubmatch(t)[1])
			for _, r := range rows {
				if strings.Contains(strings.ToLower(r.r.Raw), "requirement for an "+word) {
					target = r
					break
				}
			}
		case mayCountUpToRe.MatchString(t):
			v, _ := strconv.ParseFloat(mayCountUpToRe.FindStringSubmatch(t)[1], 64)
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
		case halfMarkRe.MatchString(t):
			name := halfMarkRe.FindStringSubmatch(t)[1]
			i := slices.IndexFunc(lists, func(l *types.CourseList) bool { return strings.EqualFold(l.Name, name) })
			if i < 0 || !slices.ContainsFunc(lists[i].Items, func(x *types.ReqNode) bool { return x.Weight == 0.5 }) {
				return false
			}
			for _, r := range rows {
				grammar.WalkReq(r.r.Req, func(x *types.ReqNode) {
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

// eitherCounts reads mayCountEitherRe as a limit of one course's weight
// on the two, under the total.
func (a *Assembler) eitherCounts() {
	if a.total == nil {
		return
	}
	a.notes = slices.DeleteFunc(a.notes, func(n string) bool {
		m := mayCountEitherRe.FindStringSubmatch(a.clean(n))
		if m == nil || grammar.CourseWeight(m[1]) != grammar.CourseWeight(m[2]) {
			return false
		}
		a.rowUnder(a.total, &types.Requirement{Kind: types.RequirementLimit, Raw: n,
			Req: &types.ReqNode{Type: types.NodeCredits, Courses: []string{m[1], m[2]}, MaxAmount: grammar.MaxAmount(grammar.CourseWeight(m[1]))}})
		return true
	})
}

// noteRoutes adds a seekApprovalRe note as a permission alternative
// beside every not-exhaustive Review alternative, which it explains.
func (a *Assembler) noteRoutes() {
	a.notes = slices.DeleteFunc(a.notes, func(n string) bool {
		t := a.clean(n)
		switch {
		case seekApprovalRe.MatchString(t):
			done := false
			for _, r := range a.allRows() {
				grammar.WalkReq(r.r.Req, func(x *types.ReqNode) {
					if x.Type == types.NodeOr && slices.ContainsFunc(x.Items, func(it *types.ReqNode) bool {
						return it.Type == types.NodeText && notExhaustiveRe.MatchString(it.Text)
					}) {
						x.Items = append(x.Items, TextNode(types.TextPermission, n))
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
// a list, joins those items' raw; alreadyChosenRe adds a same-language
// constraint (sameLanguage).
func (a *Assembler) distributionBullets() {
	lead := slices.IndexFunc(a.lines, func(l string) bool { return distributionRe.MatchString(Clean(strings.TrimSpace(l))) })
	if lead < 0 {
		return
	}
	var bullets []string
	for _, l := range a.lines[lead+1:] {
		l = strings.TrimSpace(l)
		if !strings.HasPrefix(l, "-") {
			break
		}
		bullets = append(bullets, SplitSentences(strings.TrimSpace(strings.TrimPrefix(l, "-")))...)
	}
	for _, s := range bullets {
		note := grammar.CleanNote(s)
		if !slices.ContainsFunc(a.notes, func(x string) bool { return rawWords(x) == rawWords(note) }) {
			continue
		}
		t := a.clean(s)
		if m := alreadyChosenRe.FindStringSubmatch(t); m != nil {
			if c := a.sameLanguage(m[1], note); c != nil {
				a.program = append(a.program, &compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Req: c, Raw: s}})
				a.dropNoteAll(note)
			}
			continue
		}
		var targets []*compRow
		codes := grammar.Dedupe(grammar.CodeRe.FindAllString(t, -1))
		for _, r := range a.items {
			if r.r.Kind != types.RequirementAllocation {
				continue
			}
			own := namedCodes(r.r.Req)
			if codes != nil && slices.ContainsFunc(codes, func(c string) bool { return slices.Contains(own, c) }) {
				targets = append(targets, r)
			}
			if m := remainingListRe.FindStringSubmatch(t); m != nil && codes == nil && r.r.Req != nil {
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

// sameLanguage builds the constraint of alreadyChosenRe: an or with, per
// language, and(its first, second and third year alternatives), and the
// bullet as content text. Nil when the blocks or the third item don't line up.
func (a *Assembler) sameLanguage(amount, bullet string) *types.ReqNode {
	blocks := a.languageBlocks(3)
	if len(blocks) < 3 || fmtAmount(blocks[2].amount) != amount {
		return nil
	}
	head := rawWords(Clean(strings.TrimSpace(a.lines[blocks[2].line])))
	var third *compRow
	for _, r := range a.items {
		if rawWords(Clean(RawHead(r.r))) == head {
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
		alts = append(alts, programs.AndNode(grammar.CloneNode(blocks[0].options[lang]), grammar.CloneNode(y2), grammar.CloneNode(third.r.Req.Items[i])))
	}
	if len(alts) < 2 {
		return nil
	}
	return programs.OrNode(append(alts, TextNode(types.TextContent, bullet))...)
}

// restatedOmission drops the note of a sentence the total's omission encodes,
// keeping only the advice after its ";", if any.
func (a *Assembler) restatedOmission() {
	t := a.total
	if t == nil || len(a.omissions) == 0 {
		return
	}
	raw := rawWords(t.r.Raw)
	for i, n := range a.notes {
		if !noReplaceRe.MatchString(n) || !strings.Contains(raw, rawWords(n)) {
			continue
		}
		parts := grammar.SplitTop(n, ';')
		if rest := strings.TrimSpace(strings.Join(parts[1:], ";")); rest != "" && ClassifySentence(rest) == RoleAdvisory {
			a.notes[i] = grammar.CleanNote(rest)
		} else {
			a.notes[i] = ""
		}
	}
	a.notes = slices.DeleteFunc(a.notes, func(n string) bool { return n == "" })
	if len(a.notes) == 0 {
		a.notes = nil
	}
}

// cognateMayCountRe matches a share of approved cognate courses: "Up to
// 2.0 credits in identified or preapproved cognate courses may count ...".
var cognateMayCountRe = regexp.MustCompile(`(?i)^up\s+to\s+` + grammar.AmountPat + `\s+credits?\s+in\s+identified\s+or\s+pre-?approved\s+cognate\s+courses\s+may\s+count\b.*\bapprov`)

// cognateRouteTotal reads a cognateMayCountRe note beside a total whose
// allocations all draw on one designator: the total becomes or(credits{N, D},
// and(credits{N}, text{permission})) with credits{N less the share, D} under it.
func (a *Assembler) cognateRouteTotal() {
	t := a.total
	if t == nil || t.r.Req == nil {
		return
	}
	n := t.r.Req
	if n.Type != types.NodeCredits || n.Amount == 0 || !isAnyCredits(n) {
		return
	}
	i := slices.IndexFunc(a.notes, func(s string) bool { return cognateMayCountRe.MatchString(Clean(s)) })
	if i < 0 {
		return
	}
	desig, ok := "", true
	walkRows(a.items, func(r *compRow) {
		if r.r.Kind != types.RequirementAllocation {
			return
		}
		grammar.WalkReq(r.r.Req, func(x *types.ReqNode) {
			var ds []string
			switch x.Type {
			case types.NodeCourse:
				ds = []string{x.Code[:3]}
			case types.NodeCredits:
				ds = x.Designators
				for _, c := range x.Courses {
					ds = append(ds, c[:3])
				}
				ok = ok && x.Lists == nil
			}
			for _, d := range ds {
				if desig == "" {
					desig = d
				}
				ok = ok && d == desig
			}
		})
	})
	share, _ := strconv.ParseFloat(cognateMayCountRe.FindStringSubmatch(Clean(a.notes[i]))[1], 64)
	if !ok || desig == "" || share >= n.Amount {
		return
	}
	note := a.notes[i]
	t.r.Req = programs.OrNode(
		&types.ReqNode{Type: types.NodeCredits, Amount: n.Amount, Designators: []string{desig}},
		programs.AndNode(&types.ReqNode{Type: types.NodeCredits, Amount: n.Amount}, TextNode(types.TextPermission, note)))
	addRaw(t, note)
	a.floors = append(a.floors, &compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Raw: note,
		Req: &types.ReqNode{Type: types.NodeCredits, Amount: n.Amount - share, Designators: []string{desig}}}})
	a.notes = slices.Delete(a.notes, i, i+1)
	if len(a.notes) == 0 {
		a.notes = nil
	}
}

// enrolledExceptRe matches a cap's exception for another program's
// students: "except for student enrolled in the Education and Society Minor
// may apply 1.0 credit of MAT329Y1 toward the Minor program".
var enrolledExceptRe = regexp.MustCompile(`(?i)^except\s+for\s+students?\s+enrolled\s+in\s+the\s+.+?\s+may\s+apply\s+` + grammar.AmountPat + `\s+credits?\s+of\s+(` + grammar.CodePat + `)\s+toward\s+the\s+minor\s+program$`)

// enrolledException reads a enrolledExceptRe note on a limit as the
// total's lenient alternative, or(credits{N}, and(credits{N less the trimmed
// credits}, course, text{other})): Review, since no node can name the program.
func (a *Assembler) enrolledException() {
	t := a.total
	if t == nil || t.r.Req == nil || t.r.Req.Type != types.NodeCredits || t.r.Req.Amount == 0 {
		return
	}
	// only where the items make the total: a shortfall keeps its marker
	sum := 0.0
	for _, r := range a.items {
		if r.r.Kind == types.RequirementAllocation {
			v, ok := 0.0, false
			if r.r.Req != nil {
				v, ok = nodeCredits(r.r.Req)
			}
			if !ok {
				return
			}
			sum += v
		}
	}
	if sum < t.r.Req.Amount-1e-9 {
		return
	}
	for _, r := range a.allRows() {
		n := r.r.Req
		if r.r.Kind != types.RequirementLimit || n == nil || n.MaxAmount == nil {
			continue
		}
		for _, note := range r.r.Notes {
			m := enrolledExceptRe.FindStringSubmatch(Clean(note))
			if m == nil || !slices.Contains(n.Courses, m[2]) {
				continue
			}
			v, _ := strconv.ParseFloat(m[1], 64)
			trimmed := v - *n.MaxAmount
			if trimmed <= 0 || trimmed >= t.r.Req.Amount {
				continue
			}
			less := grammar.CloneNode(t.r.Req)
			less.Amount -= trimmed
			t.r.Req = programs.OrNode(t.r.Req, programs.AndNode(less, &types.ReqNode{Type: types.NodeCourse, Code: m[2]}, TextNode(types.TextOther, note)))
			addRaw(t, note)
			addRaw(r, note)
			a.dropNoteAll(note)
			return
		}
	}
}

// repeatTopicRe matches a course that counts twice: "DHU339H1 can be
// taken twice if the topic is different".
var repeatTopicRe = regexp.MustCompile(`(?i)^(` + grammar.CodePat + `)\s+can\s+be\s+taken\s+twice\s+if\s+the\s+topic\s+is\s+different$`)

// repeatTopic reads a repeatTopicRe note as a content text alternative
// on the one allocation whose pool holds the course: the transcript's codes
// can't tell topics apart, so it is Review.
func (a *Assembler) repeatTopic(lists []*types.CourseList) {
	byName := listsByName(lists)
	all := slices.Clone(a.notes)
	for _, r := range a.allRows() {
		all = append(all, r.r.Notes...)
	}
	for _, note := range all {
		m := repeatTopicRe.FindStringSubmatch(Clean(note))
		if m == nil {
			continue
		}
		var hits []*compRow
		for _, r := range a.allRows() {
			if r.r.Kind != types.RequirementAllocation || r == a.total {
				continue
			}
			found := false
			grammar.WalkReq(r.r.Req, func(x *types.ReqNode) {
				if x.Type != types.NodeCredits {
					return
				}
				found = found || slices.Contains(x.Courses, m[1])
				if codes, ok := a.listCodes(x.Lists, byName); ok {
					found = found || slices.Contains(codes, m[1])
				}
			})
			if found {
				hits = append(hits, r)
			}
		}
		if len(hits) != 1 {
			continue
		}
		hits[0].r.Req = programs.OrNode(hits[0].r.Req, TextNode(types.TextContent, grammar.CleanNote(note)))
		addRaw(hits[0], note)
		a.dropNoteAll(note)
	}
}
