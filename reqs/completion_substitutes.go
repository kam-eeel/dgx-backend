package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Substitutes, exemptions and exclusions: sentences the earlier passes leave as
// text, though they name what stands in for a course or what may not count.

var (
	// complExemptAnyRe matches an exemption replaced by any course of some
	// designators: "Students exempted from FSL220H1/ FSL222H1 may replace it with
	// any FRE 200+ or FSL 300+ level course".
	complExemptAnyRe = regexp.MustCompile(`(?i)^students\s+exempted\s+from\s+` + complRelaxCodesPat + `\s+may\s+replace\s+(?:it|them)\s+with\s+any\s+(` + desigPat + `(?:\s+` + levelPat + `\+)?(?:\s+or\s+` + desigPat + `(?:\s+` + levelPat + `\+)?)*)(?:\s+level)?\s+course$`)
	// complExemptCombinationRe matches an exemption replaced by a requirement's
	// pool or a filter: "Students exempted from any of FSL220H1/ ... must replace
	// these credits with any combination of the following two options: ...".
	complExemptCombinationRe = regexp.MustCompile(`(?i)^students\s+exempted\s+from\s+any\s+of\s+` + complRelaxCodesPat + `\s+must\s+replace\s+these\s+credits\s+with\s+any\s+combination\s+of\s+the\s+following\s+two\s+options:\s*(?:\(a\)\s*)?any\s+additional\s+credit\(s\)\s+from\s+category\s+\((\d{1,2})\)\s+above,?\s+or\s+(?:\(b\)\s*)?any\s+(` + desigPat + `)\s+credit\(s\)\s+at\s+the\s+(` + levelPat + `(?:/` + levelPat + `)*)\s+level$`)
	// complExemptFilterRe matches one filter of complExemptAnyRe: "FRE 200+".
	complExemptFilterRe = regexp.MustCompile(`([A-Z]{3})(?:\s+([1-4])00\+)?`)
	// complPairAdditionalRe matches a course and more credits, or a full course:
	// "( FSL420H1, 0.5 additional credit of FRE at the 200+ level)/ FSL421Y1".
	// The groups are the course, the amount, the filters, the full course.
	complPairAdditionalRe = regexp.MustCompile(`(?i)^\(\s*(` + codePat + `),\s*` + amountPat + `\s+additional\s+credits?\s+of\s+(` + desigPat + `\s+at\s+the\s+` + levelPat + `\+\s+level(?:\s+or\s+` + desigPat + `\s+at\s+the\s+` + levelPat + `\+\s+level)*)\s*\)\s*/\s*(` + codePat + `)$`)
	// complLevelFilterRe matches one filter of complPairAdditionalRe: "FSL at
	// the 300+ level".
	complLevelFilterRe = regexp.MustCompile(`([A-Z]{3})\s+at\s+the\s+([1-4])00\+\s+level`)
	// complCountFloorRe matches a count of courses with a floor from a list: "Any
	// 2 (1.0 credit) 400-level GGR courses, at least one of which must be from
	// Group E". The groups are the amount, the level, the designator, the list.
	complCountFloorRe = regexp.MustCompile(`(?i)^any\s+(?:\d+|two|three|four)\s+\(\s*` + amountPat + `\s+credits?\s*\)\s+([1-4])00-level\s+(` + desigPat + `)\s+courses,\s+` + atLeastPat + `\s+one\s+of\s+which\s+must\s+be\s+from\s+(group\s+[A-Z])$`)
)

// exemptReplacements reads the exemptions replaced by any course of some
// designators, once the routes are placed beside the courses, and returns
// lists with the unions they make.
func (a *compAssembler) exemptReplacements(lists []*types.CourseList) []*types.CourseList {
	union := func(r *compRow, items []*types.ReqNode) (string, bool) {
		if r.r.Label == "" {
			return "", false
		}
		base := "Requirement " + r.r.Label + " courses"
		name := base
		for k := 2; slices.ContainsFunc(lists, func(l *types.CourseList) bool { return l.Name == name }); k++ {
			name = base + " (" + strconv.Itoa(k) + ")"
		}
		lists = append(lists, &types.CourseList{Name: name, Items: items})
		return name, true
	}
	visit := func(r *compRow) {
		if r.r.Kind == types.RequirementAllocation && r.r.Req != nil {
			r.r.Req = exemptAny(r, r.r.Req, union)
		}
	}
	walkRowsPost(a.items, visit)
	walkRowsPost(a.program, visit)
	// one union per sentence: the first row's
	made := map[string]string{}
	walkRowsPost(a.items, func(r *compRow) {
		if r.r.Kind == types.RequirementAllocation && r.r.Req != nil {
			r.r.Req = a.exemptCombination(r, r.r.Req, union, made)
		}
	})
	return lists
}

// exemptCombination replaces a complExemptCombinationRe alternative of an or
// whose courses it names with and(credits{the exempted credits, a union of the
// requirement's list pool and the filter}, text{placement}).
func (a *compAssembler) exemptCombination(r *compRow, n *types.ReqNode, union func(*compRow, []*types.ReqNode) (string, bool), made map[string]string) *types.ReqNode {
	for i, it := range n.Items {
		n.Items[i] = a.exemptCombination(r, it, union, made)
	}
	if n.Type != types.NodeOr {
		return n
	}
	for i, it := range n.Items {
		if it.Type != types.NodeText {
			continue
		}
		m := complExemptCombinationRe.FindStringSubmatch(complClean(it.Text))
		if m == nil {
			continue
		}
		v := exemptedCredits(n, codeRe.FindAllString(m[1], -1))
		if v == 0 {
			continue
		}
		name, ok := made[it.Text]
		if !ok {
			var pool *types.ReqNode
			for _, x := range a.items {
				if x.r.Label == m[2] && x.r.Kind == types.RequirementAllocation && x.r.Req != nil && x.r.Req.Type == types.NodeCredits && x.r.Req.Courses == nil && x.r.Req.Designators == nil && x.r.Req.Lists != nil {
					pool = &types.ReqNode{Type: types.NodeCredits, Lists: slices.Clone(x.r.Req.Lists), Levels: slices.Clone(x.r.Req.Levels)}
				}
			}
			if pool == nil {
				continue
			}
			f := &types.ReqNode{Type: types.NodeCredits, Designators: []string{m[3]}}
			for _, l := range strings.Split(m[4], "/") {
				lv, _ := strconv.Atoi(l)
				f.Levels = append(f.Levels, lv)
			}
			if name, ok = union(r, []*types.ReqNode{pool, f}); !ok {
				continue
			}
			made[it.Text] = name
		}
		n.Items[i] = andNode(&types.ReqNode{Type: types.NodeCredits, Amount: v, Lists: []string{name}}, textNode(types.TextPlacement, it.Text))
	}
	return n
}

// exemptedCredits returns the credits of the or n's heaviest alternative made
// only of named courses: 1.0 for "( FSL220H1, FSL222H1)". It is 0 when none is.
func exemptedCredits(n *types.ReqNode, named []string) float64 {
	best := 0.0
	for _, it := range n.Items {
		if it.Type == types.NodeText {
			continue
		}
		sum, ok := 0.0, true
		walkReq(it, func(x *types.ReqNode) {
			switch x.Type {
			case types.NodeCourse:
				ok = ok && slices.Contains(named, x.Code)
				sum += courseWeight(x.Code)
			case types.NodeAnd:
			default:
				ok = false
			}
		})
		if ok && sum > best {
			best = sum
		}
	}
	return best
}

// exemptAny replaces a complExemptAnyRe alternative of an or, whose courses
// are among the or's other alternatives, with and(credits{the exempted
// credits or 0.5, the filters or their union}, text{placement}).
func exemptAny(r *compRow, n *types.ReqNode, union func(*compRow, []*types.ReqNode) (string, bool)) *types.ReqNode {
	for i, it := range n.Items {
		n.Items[i] = exemptAny(r, it, union)
	}
	if n.Type != types.NodeOr {
		return n
	}
	have := map[string]bool{}
	for _, it := range n.Items {
		if it.Type != types.NodeText {
			walkReq(it, func(x *types.ReqNode) { have[x.Code] = have[x.Code] || x.Code != "" })
		}
	}
	for i, it := range n.Items {
		if it.Type != types.NodeText {
			continue
		}
		m := complExemptAnyRe.FindStringSubmatch(complClean(it.Text))
		if m == nil || slices.ContainsFunc(codeRe.FindAllString(m[1], -1), func(c string) bool { return !have[c] }) {
			continue
		}
		var filters []*types.ReqNode
		for _, f := range complExemptFilterRe.FindAllStringSubmatch(m[2], -1) {
			filters = append(filters, levelFilter(f[1], f[2]))
		}
		pool := &types.ReqNode{Type: types.NodeCredits, Amount: 0.5}
		if v := exemptedCredits(n, codeRe.FindAllString(m[1], -1)); v > 0 {
			pool.Amount = v // the credits the exemption takes
		}
		if len(filters) == 1 {
			pool.Designators, pool.Levels = filters[0].Designators, filters[0].Levels
		} else if name, ok := union(r, filters); ok {
			pool.Lists = []string{name}
		} else {
			continue
		}
		n.Items[i] = andNode(pool, textNode(types.TextPlacement, it.Text))
	}
	return n
}

// levelFilter returns a pool's filter: designator d at level l00 and up (""
// for any level).
func levelFilter(d, l string) *types.ReqNode {
	f := &types.ReqNode{Type: types.NodeCredits, Designators: []string{d}}
	if l != "" {
		for v := int(l[0]-'0') * 100; v <= 400; v += 100 {
			f.Levels = append(f.Levels, v)
		}
	}
	return f
}

var (
	// complExclusionNoteRe matches a note that earlier courses exclude an item's:
	// "If you completed CSC148H1 before being admitted to the Minor, please note
	// that this course is an exclusion to EEB125H1".
	complExclusionNoteRe = regexp.MustCompile(`(?i)^if\s+you\s+completed\s+.+?\s+before\s+being\s+admitted\s+to\s+the\s+\w+,\s+please\s+note\s+that\s+(?:these\s+courses\s+are\s+exclusions|this\s+course\s+is\s+an\s+exclusion)\s+to\b`)
	// complInsteadExtraRe matches the alternative such a note gives: "to meet
	// requirement 1 or can instead complete an additional 0.5 credit from the list
	// of courses in requirement 7, for a total of 1.5 credits for requirement 7".
	complInsteadExtraRe = regexp.MustCompile(`(?i)\bto\s+meet\s+requirement\s+(\d{1,2})\s+or\s+can\s+instead\s+complete\s+an\s+additional\s+` + amountPat + `\s+credits?\s+from\s+the\s+list\s+of\s+courses\s+in\s+requirement\s+(\d{1,2}),\s+for\s+a\s+total\s+of\s+` + numPat + `\s+credits?\s+for\s+requirement\s+(\d{1,2})$`)
	// complCoursesThenRouteRe matches listed courses ending in an approval
	// route: "At least 1.0 credit from the following courses: BCB410H1/ ... with
	// prior approval from the Associate Chair".
	complCoursesThenRouteRe = regexp.MustCompile(`(?i)^` + atLeastPat + `\s+` + amountPat + `\s+credits?\s+from\s+the\s+following\s+courses:\s*((?:` + codePat + `\s*/\s*)+)(` + numPat + `\s+or\s+` + numPat + `\s+credits?\s+from\s+.+\bwith\s+prior\s+approval\s+from\b.+)$`)
)

// exclusionAlternatives turns complExclusionNoteRe rows into notes and gives
// the item named by complInsteadExtraRe or(its body, and(credits from the
// other item's courses, text)): Review, since admission dates aren't on the
// transcript. ASMIN0160.
func (a *compAssembler) exclusionAlternatives() {
	var walk func(rs []*compRow)
	walk = func(rs []*compRow) {
		for _, r := range slices.Clone(rs) {
			walk(r.children)
			if n := r.r.Req; n != nil && n.Type == types.NodeText && r.r.Kind == types.RequirementAllocation &&
				len(r.children) == 0 && complExclusionNoteRe.MatchString(complClean(n.Text)) && a.detach(r) {
				a.notes = append(append(a.notes, cleanNote(n.Text)), r.r.Notes...)
			}
		}
	}
	walk(a.items)
	for _, s := range slices.Clone(a.notes) {
		m := complInsteadExtraRe.FindStringSubmatch(complClean(s))
		if m == nil || m[3] != m[4] {
			continue
		}
		item, other := a.labelled(m[1]), a.labelled(m[3])
		if item == nil || other == nil || item == other || item.r.Req == nil || other.r.Req == nil {
			continue
		}
		if t := other.r.Req; t.Type == types.NodeText {
			p := complCoursesThenRouteRe.FindStringSubmatch(complClean(t.Text))
			if p == nil {
				continue
			}
			v, _ := strconv.ParseFloat(p[1], 64)
			other.r.Req = orNode(&types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: dedupe(codeRe.FindAllString(p[2], -1))},
				textNode(types.TextPermission, strings.TrimSuffix(p[3], ".")))
		}
		pool := other.r.Req
		if pool.Type == types.NodeOr {
			pool = pool.Items[0]
		}
		if !isPool(pool) || pool.Courses == nil || pool.Lists != nil {
			continue
		}
		v, _ := strconv.ParseFloat(m[2], 64)
		alt := andNode(&types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: slices.Clone(pool.Courses)}, textNode(types.TextOther, cleanNote(s)))
		if !slices.ContainsFunc(item.r.Req.Items, func(x *types.ReqNode) bool { return x.Type == types.NodeAnd && sameNode(x.Items[0], alt.Items[0]) }) {
			item.r.Req = orNode(item.r.Req, alt) // once for notes that give the same alternative
		}
		addRaw(item, cleanNote(s))
		a.notes = slices.DeleteFunc(a.notes, func(n string) bool { return n == s })
	}
}

// pairSubstitutes reads, as substitutes does for single courses, an in-lieu
// alternative whose courses include a pair: or(pool, and(pool less v,
// credits{v, lists: [the courses]})). It returns lists with the lists it made.
func (a *compAssembler) pairSubstitutes(lists []*types.CourseList) []*types.CourseList {
	for _, r := range a.allRows() {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr || r.r.Label == "" {
			continue
		}
		for i, it := range n.Items {
			if it.Type != types.NodeText {
				continue
			}
			m := complPairInLieuRe.FindStringSubmatch(complClean(it.Text))
			if m == nil {
				continue
			}
			pool := n.Items[0]
			level, _ := strconv.Atoi(m[3] + "00")
			if pool.Type != types.NodeCredits || pool.Designators == nil || pool.Lists != nil || !slices.Contains(pool.Levels, level) {
				continue
			}
			v, _ := strconv.ParseFloat(m[2], 64)
			items, ok := inLieuItems(m[1], v)
			if !ok || pool.Amount <= v {
				continue
			}
			name := "Requirement " + r.r.Label + " substitutes"
			if slices.ContainsFunc(lists, func(l *types.CourseList) bool { return l.Name == name }) {
				continue
			}
			lists = append(lists, &types.CourseList{Name: name, Items: items})
			less := cloneNode(pool)
			less.Amount -= v
			n.Items[i] = andNode(less, &types.ReqNode{Type: types.NodeCredits, Amount: v, Lists: []string{name}})
			addRaw(r, it.Text)
		}
	}
	return lists
}

// inLieuItems reads the members of an in-lieu sentence as list items each worth
// at most v. ok is false unless one member is a pair and every single course
// weighs at least v.
func inLieuItems(s string, v float64) ([]*types.ReqNode, bool) {
	units, ok := splitUnitsSlash(s)
	if !ok {
		return nil, false
	}
	var items []*types.ReqNode
	pair := false
	single := func(codes []string) *types.ReqNode {
		x := &types.ReqNode{Type: types.NodeCredits, Courses: codes}
		if courseWeight(codes[0]) > v {
			x.Weight = v
		}
		return x
	}
	for _, u := range units {
		switch {
		case codeRe.FindString(u) == u:
			if courseWeight(u) < v {
				return nil, false
			}
			items = append(items, single([]string{u}))
		case complPairUnitRe.MatchString(u):
			x := complPairUnitRe.FindStringSubmatch(u)
			p := andNode(&types.ReqNode{Type: types.NodeCourse, Code: x[1]}, &types.ReqNode{Type: types.NodeCourse, Code: x[2]})
			if courseWeight(x[1])+courseWeight(x[2]) > v {
				p.Weight = v
			}
			items = append(items, p)
			pair = true
		case complAltUnitRe.MatchString(u):
			codes := codeRe.FindAllString(u, -1)
			if slices.ContainsFunc(codes, func(c string) bool { return courseWeight(c) != courseWeight(codes[0]) || courseWeight(c) < v }) {
				return nil, false
			}
			items = append(items, single(codes))
		default:
			return nil, false
		}
	}
	return items, pair
}

// splitUnitsSlash splits members at the "/" outside brackets.
func splitUnitsSlash(s string) ([]string, bool) {
	units, ok := splitUnits(strings.ReplaceAll(s, ",", "\x00"))
	for i := range units {
		units[i] = strings.ReplaceAll(units[i], "\x00", ",")
	}
	return units, ok
}

var (
	// complSubExemptedRe matches "exempted from ... must replace these with the
	// equivalent number of credits of additional FSL or FRE courses".
	complSubExemptedRe = regexp.MustCompile(`(?i)\bexempted\s+from\b.*\b(?:must|may|can)\s+replace\s+(?:these|them|it)(?:\s+(?:courses|credits))?\s+with\s+the\s+equivalent\s+number\s+of\s+credits\s+of\s+additional\s+(` + desigPat + `(?:\s+or\s+` + desigPat + `)*)\s+courses(?:\s+at\s+the\s+([1-4])00\+\s+level)?$`)
	// complSubAlsoRe matches a named substitute: "A student who has completed
	// CSC110Y1 also fulfills this program requirement".
	complSubAlsoRe = regexp.MustCompile(`(?i)^an?\s+student\s+who\s+has\s+completed\s+(` + codePat + `)\s+also\s+fulfil+s\s+this\s+(?:program\s+)?requirement$`)
	// complSubInsteadForRe matches "Students may use MAT351Y1 instead of APM346H1
	// for Requirement 1". Groups: the substitute, the course, the row's number.
	complSubInsteadForRe = regexp.MustCompile(`(?i)\b(?:may|can)\s+use\s+(` + codePat + `)\s+instead\s+of\s+(` + codePat + `)\s+for\s+requirement\s+(\d{1,2})\b`)
	// complSubInLieuRe matches "Students may count ECO210H1 towards the
	// Specialist program in lieu of a 0.5 credit 300-level ECO elective".
	complSubInLieuRe = regexp.MustCompile(`(?i)^students\s+may\s+count\s+((?:` + codePat + `)(?:\s*(?:/|,|or)\s*` + codePat + `)*)\s+towards?\s+the\s+[a-z ]+?\s+in\s+lieu\s+of\s+an?\s+` + amountPat + `\s+credit\s+([1-4])00-level\s+(` + desigPat + `)\s+(?:elective|course)$`)
	// complSubPreparationRe matches "... Students taking one of these MAT courses
	// can have it count in lieu of a 0.5 300-level ECO course ...".
	complSubPreparationRe = regexp.MustCompile(`(?i)^((?:` + codePat + `)(?:\s*/\s*` + codePat + `)*)\s+(?:is|are)\s+recommended\s+as\s+preparation\s+for\s+` + codePat + `\.?\s+students\s+taking\s+one\s+of\s+these\s+` + desigPat + `\s+courses\s+can\s+have\s+it\s+count\s+in\s+lieu\s+of\s+an?\s+` + amountPat + `\s+([1-4])00-level\s+(` + desigPat + `)\s+course\s+required\s+for\s+this\s+program$`)
	// complSubEquivalentRe matches "The combination of ( STA220H1, STA255H1) is
	// considered equivalent to ECO220Y1". Groups: the two courses, the course.
	complSubEquivalentRe = regexp.MustCompile(`(?i)^\*?the\s+combination\s+of\s+\(\s*(` + codePat + `)\s*(?:,|and)\s*(` + codePat + `)\s*\)\s+is\s+considered\s+equivalent\s+to\s+(` + codePat + `)\b`)
	// complSubSuchStudentsRe matches "Such students can instead substitute any
	// 300-level MAT or APM course to meet the MAT246H1 requirement".
	complSubSuchStudentsRe = regexp.MustCompile(`(?i)^such\s+students\s+can\s+instead\s+substitute\s+any\s+([1-4])00-level\s+(` + desigPat + `(?:\s+or\s+` + desigPat + `)*)\s+course\s+to\s+meet\s+the\s+(` + codePat + `)\s+requirement$`)
	// complSubNotCountedRe matches "ENG386H1, ..., and ENG498H1 may not be
	// counted toward fulfilling the requirements for the Minor program".
	complSubNotCountedRe = regexp.MustCompile(`(?i)^((?:` + codePat + `)(?:\s*(?:,|,?\s*and|/)\s*` + codePat + `)*)\s+may\s+not\s+be\s+counted\s+towards?\s+(?:fulfil+ing\s+)?the\s+requirements\s+(?:for|of)\s+the\s+.*\bprogram\b.*$`)
	// complSubNotThisRe matches "MAT221H1 may not be used for this requirement".
	complSubNotThisRe = regexp.MustCompile(`(?i)^((?:` + codePat + `)(?:\s*(?:,|,?\s*and|/|or)\s*` + codePat + `)*)\s+may\s+not\s+be\s+used\s+(?:for|towards?)\s+this\s+requirement$`)
	// complSubFallbackRe and complSubNoneRe match "DTS300H1 or a 0.5 credit to
	// satisfy ...", then "If none is available, a 0.5 credit from Breadth ...".
	complSubFallbackRe = regexp.MustCompile(`(?i)^(` + codePat + `)\s+or\s+(an?\s+` + amountPat + `\s+credit\s+to\s+satisfy\s+.+)$`)
	complSubNoneRe     = regexp.MustCompile(`(?i)^(if\s+none\s+is\s+available),\s+an?\s+` + amountPat + `\s+credit\s+from\s+breadth\s+requirement\s+category\s+#?([1-5])\b[^,]*,\s+or\s+(an?\s+` + numPat + `\s+credit\s+approved\s+by\s+the\s+.+)$`)
)

// substitutes encodes the substitutes, exemptions and exclusions the rows'
// text names.
func (a *compAssembler) substitutes() {
	roots := append(append(append([]*compRow{}, a.floors...), a.items...), a.program...)
	var rows []*compRow
	walkRows(roots, func(r *compRow) {
		rows = append(rows, r)
	})
	for _, r := range rows {
		if r.r.Kind == types.RequirementAllocation && r.r.Req != nil {
			r.r.Req = a.substituteNode(r, r.r.Req)
		}
	}
	for _, r := range rows {
		n := r.r.Req
		if n == nil {
			continue
		}
		switch {
		case n.Type == types.NodeText && r.r.Kind == types.RequirementAllocation:
			a.equivalence(r, rows)
		case n.Type == types.NodeText && r.r.Kind == types.RequirementConstraint:
			a.notCounted(r)
		case n.Type == types.NodeAnd && r.r.Kind == types.RequirementAllocation:
			if f := fallbacks(n); f != nil {
				r.r.Req = f
			}
		}
	}
}

// substituteNode encodes the text alternatives of n's "or" nodes that name a
// replacement or a substitute, in row r's body, and returns n.
func (a *compAssembler) substituteNode(r *compRow, n *types.ReqNode) *types.ReqNode {
	for i, it := range n.Items {
		n.Items[i] = a.substituteNode(r, it)
	}
	if n.Type != types.NodeOr {
		return n
	}
	for i := 0; i < len(n.Items); i++ {
		it := n.Items[i]
		if it.Type != types.NodeText {
			continue
		}
		others := slices.DeleteFunc(slices.Clone(n.Items), func(x *types.ReqNode) bool { return x.Type == types.NodeText })
		t := it.Text
		switch {
		case complSubExemptedRe.MatchString(t):
			if alt := exemptedRoute(t, others); alt != nil {
				n.Items[i] = alt
			}
		case complSubSuchStudentsRe.MatchString(t):
			m := complSubSuchStudentsRe.FindStringSubmatch(t)
			if !slices.ContainsFunc(others, func(x *types.ReqNode) bool { return x.Type == types.NodeCourse && x.Code == m[3] }) {
				break
			}
			// Who "such students" are is on the transcript, and a course node
			// would claim that course, another row's: the sentence is the
			// route's text.
			pool := &types.ReqNode{Type: types.NodeCredits, Amount: courseWeight(m[3]), Designators: desigRe.FindAllString(m[2], -1), Levels: []int{int(m[1][0]-'0') * 100}}
			text := t
			if w := regexp.MustCompile(`Students\s+with\s+` + codePat + `\s+cannot\s+take\s+` + m[3] + `\b[^.]*`).FindString(a.raw); w != "" {
				text = w + ". " + t
			}
			n.Items[i] = andNode(pool, textNode(types.TextOther, text))
		case complSubAlsoRe.MatchString(t):
			n.Items[i] = &types.ReqNode{Type: types.NodeCourse, Code: complSubAlsoRe.FindStringSubmatch(t)[1]}
		case complSubInsteadForRe.MatchString(t):
			m := complSubInsteadForRe.FindStringSubmatch(t)
			if (r.r.Label == m[3] || strings.HasSuffix(r.r.Label, " "+m[3])) && slices.ContainsFunc(others, func(x *types.ReqNode) bool { return x.Type == types.NodeCourse && x.Code == m[2] }) {
				n.Items[i] = &types.ReqNode{Type: types.NodeCourse, Code: m[1]}
			}
		case complSubInLieuRe.MatchString(t) || complSubPreparationRe.MatchString(t):
			m := complSubInLieuRe.FindStringSubmatch(t)
			if m == nil {
				m = complSubPreparationRe.FindStringSubmatch(t)
			}
			codes := codeRe.FindAllString(m[1], -1)
			v, _ := strconv.ParseFloat(m[2], 64)
			// The courses can't join the pool, whose levels they may not
			// pass: or(pool, and(the pool less their credits, the courses)).
			if pool := inLieuPool(m, others); pool != nil && n == r.r.Req && pool.Amount > v &&
				!slices.ContainsFunc(codes, func(c string) bool { return courseWeight(c) != v }) {
				less := cloneNode(pool)
				less.Amount -= v
				n.Items[i] = andNode(less, orNode(courseNodes(codes)...))
				addRaw(r, t)
			}
		}
	}
	if len(n.Items) == 1 {
		return n.Items[0]
	}
	return n
}

// exemptedRoute returns the route exemption sentence t gives the alternatives
// others: their credits from the pool it names, with t as placement text,
// since the exemption is the department's call. Nil when the credits differ.
func exemptedRoute(t string, others []*types.ReqNode) *types.ReqNode {
	m := complSubExemptedRe.FindStringSubmatch(t)
	amount := -1.0
	for _, o := range others {
		v, ok := nodeCredits(o)
		if !ok || v <= 0 || amount >= 0 && v != amount {
			return nil
		}
		amount = v
	}
	if amount <= 0 {
		return nil
	}
	pool := &types.ReqNode{Type: types.NodeCredits, Amount: amount, Designators: desigRe.FindAllString(m[1], -1)}
	if m[2] != "" {
		for l := int(m[2][0]-'0') * 100; l <= 400; l += 100 {
			pool.Levels = append(pool.Levels, l)
		}
	}
	return andNode(pool, textNode(types.TextPlacement, t))
}

// inLieuPool returns the only alternative in others when it is the pool of
// designator m[4] at level m[3]00 that the in-lieu match m names, or nil.
func inLieuPool(m []string, others []*types.ReqNode) *types.ReqNode {
	if len(others) != 1 {
		return nil
	}
	p := others[0]
	level, _ := strconv.Atoi(m[3] + "00")
	if p.Type != types.NodeCredits || p.Lists != nil || !slices.Equal(p.Designators, []string{m[4]}) || !slices.Contains(p.Levels, level) {
		return nil
	}
	return p
}

// equivalence drops row r, a named equivalence another of rows already
// encodes: its sentence and notes join that row's.
func (a *compAssembler) equivalence(r *compRow, rows []*compRow) {
	m := complSubEquivalentRe.FindStringSubmatch(r.r.Req.Text)
	if m == nil || len(r.children) > 0 {
		return
	}
	pair := andNode(&types.ReqNode{Type: types.NodeCourse, Code: m[1]}, &types.ReqNode{Type: types.NodeCourse, Code: m[2]})
	for _, o := range rows {
		if o == r || o.r.Kind != types.RequirementAllocation {
			continue
		}
		found := false
		walkReq(o.r.Req, func(x *types.ReqNode) {
			found = found || x.Type == types.NodeOr &&
				slices.ContainsFunc(x.Items, func(y *types.ReqNode) bool { return y.Type == types.NodeCourse && y.Code == m[3] }) &&
				slices.ContainsFunc(x.Items, func(y *types.ReqNode) bool { return sameNode(y, pair) })
		})
		if !found {
			continue
		}
		if a.detach(r) {
			o.r.Raw += "\n" + r.r.Raw
			o.r.Notes = append(o.r.Notes, r.r.Notes...)
		}
		return
	}
}

// notCounted makes row r, a row of text, a limit of 0 when it says courses may
// not count: under the total for the program, under the item it follows for
// "this requirement".
func (a *compAssembler) notCounted(r *compRow) {
	t := r.r.Req.Text
	if m := complSubNotCountedRe.FindStringSubmatch(t); m != nil && len(r.children) == 0 {
		r.r.Kind = types.RequirementLimit
		r.r.Req = &types.ReqNode{Type: types.NodeCredits, Courses: dedupe(codeRe.FindAllString(m[1], -1)), MaxAmount: maxAmount(0)}
		return
	}
	m := complSubNotThisRe.FindStringSubmatch(t)
	if m == nil || len(r.children) > 0 {
		return
	}
	item := a.itemBefore(t)
	if item == nil || item.r.Kind != types.RequirementAllocation || !a.detach(r) {
		return
	}
	r.r.Kind = types.RequirementLimit
	r.r.Req = &types.ReqNode{Type: types.NodeCredits, Courses: dedupe(codeRe.FindAllString(m[1], -1)), MaxAmount: maxAmount(0)}
	item.children = append(item.children, r)
}

// itemBefore returns the item whose line comes last before the line that
// holds sentence s, or nil.
func (a *compAssembler) itemBefore(s string) *compRow {
	at := slices.IndexFunc(a.lines, func(l string) bool { return strings.Contains(rawWords(l), rawWords(s)) })
	for i := at - 1; at >= 0 && i >= 0; i-- {
		line := strings.TrimSpace(a.lines[i])
		for _, r := range a.items {
			if line != "" && strings.TrimSpace(rawHead(r.r)) == line {
				return r
			}
		}
	}
	return nil
}

// detach takes row r out of the rows, at any depth, and reports whether it
// was there.
func (a *compAssembler) detach(r *compRow) bool {
	var from func(rs []*compRow) ([]*compRow, bool)
	from = func(rs []*compRow) ([]*compRow, bool) {
		for i, x := range rs {
			if x == r {
				return slices.Delete(rs, i, i+1), true
			}
			if kids, ok := from(x.children); ok {
				x.children = kids
				return rs, true
			}
		}
		return rs, false
	}
	for _, rs := range []*[]*compRow{&a.floors, &a.items, &a.program} {
		if out, ok := from(*rs); ok {
			*rs = out
			return true
		}
	}
	return false
}

// fallbacks reads a row's and(text, text) as a chain of fallbacks, or returns
// nil.
func fallbacks(n *types.ReqNode) *types.ReqNode {
	if len(n.Items) != 2 || n.Items[0].Type != types.NodeText || n.Items[1].Type != types.NodeText {
		return nil
	}
	m1 := complSubFallbackRe.FindStringSubmatch(n.Items[0].Text)
	m2 := complSubNoneRe.FindStringSubmatch(n.Items[1].Text)
	if m1 == nil || m2 == nil || m1[3] != m2[2] {
		return nil
	}
	v, _ := strconv.ParseFloat(m2[2], 64)
	category, _ := strconv.Atoi(m2[3])
	return orNode(
		&types.ReqNode{Type: types.NodeCourse, Code: m1[1]},
		textNode(types.TextOther, m1[2]),
		andNode(&types.ReqNode{Type: types.NodeCredits, Amount: v, Breadth: []int{category}}, textNode(types.TextOther, m2[1])),
		textNode(types.TextPermission, m2[4]),
	)
}
