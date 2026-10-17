package programs

import (
	"regexp"
	"slices"
	"strings"

	"degree-explorer/parser/grammar"
	"degree-explorer/types"
)

// Linking steps that read the rest of the catalogue: jointly offered courses,
// equivalents and exclusions.

// Catalog is what those steps need from the catalogue.
type Catalog struct {
	Exclusions map[string]*types.Exclusions // course code -> its exclusions
	joint      map[string][]string          // first area -> joint designators its programs list
}

// jointCodeRe matches a jointly offered course: "JEG100H1".
var jointCodeRe = regexp.MustCompile(`\bJ[A-Z]{2}\d{3}[HY]\d\b`)

func NewCatalog(courses []*types.Course, programs []*types.Program) *Catalog {
	cat := &Catalog{Exclusions: map[string]*types.Exclusions{}, joint: map[string][]string{}}
	for _, c := range courses {
		cat.Exclusions[c.Code] = c.Exclusions
	}
	for _, p := range programs {
		if len(p.Areas) == 0 || p.Completion == nil {
			continue
		}
		area := p.Areas[0]
		for _, code := range jointCodeRe.FindAllString(p.Completion.Raw, -1) {
			cat.joint[area] = append(cat.joint[area], code[:3])
		}
	}
	for area, ds := range cat.joint {
		slices.Sort(ds)
		cat.joint[area] = slices.Compact(ds)
	}
	return cat
}

// jointTextRe matches jointText.
var jointTextRe = regexp.MustCompile(`^a jointly-offered ([A-Z]{3}) course$`)

// LinkJoint replaces the jointText alternative to a subject's pools with the
// joint designators its area's programs list that share the subject's letter
// (G for Geography). With no list for the area the alternative stays.
func (cat *Catalog) LinkJoint(n *types.ReqNode, area string) *types.ReqNode {
	subjects := map[string][]string{}
	grammar.WalkReq(n, func(t *types.ReqNode) {
		m := jointTextRe.FindStringSubmatch(t.Text)
		if t.Type != types.NodeText || m == nil {
			return
		}
		for _, d := range cat.joint[area] {
			if strings.ContainsRune(d[1:], rune(m[1][0])) {
				subjects[m[1]] = append(subjects[m[1]], d)
			}
		}
	})
	if len(subjects) == 0 {
		return n
	}
	grammar.WalkReq(n, func(c *types.ReqNode) {
		if (c.Type == types.NodeCredits || c.Type == types.NodeAverage) && len(c.Designators) > 0 {
			if joint, ok := subjects[c.Designators[0]]; ok {
				c.Designators = grammar.Dedupe(append(c.Designators, joint...))
			}
		}
	})
	resolved := func(it *types.ReqNode) bool {
		m := jointTextRe.FindStringSubmatch(it.Text)
		return it.Type == types.NodeText && m != nil && subjects[m[1]] != nil
	}
	var drop func(t *types.ReqNode) *types.ReqNode
	drop = func(t *types.ReqNode) *types.ReqNode {
		for i, it := range t.Items {
			t.Items[i] = drop(it)
		}
		if t.Type == types.NodeOr && slices.ContainsFunc(t.Items, resolved) {
			return Tidy(OrNode(slices.DeleteFunc(slices.Clone(t.Items), resolved)...))
		}
		return t
	}
	return drop(n)
}

// LinkEquivalent reads "MGT100H1 or equivalent" as the course or one it
// excludes: the calendar lists a course's equivalents as its exclusions.
func (cat *Catalog) LinkEquivalent(n *types.ReqNode) *types.ReqNode {
	if n == nil {
		return nil
	}
	for i, it := range n.Items {
		n.Items[i] = cat.LinkEquivalent(it)
	}
	if n.Type != types.NodeOr || len(n.Items) != 2 {
		return n
	}
	course, text := n.Items[0], n.Items[1]
	if course.Type != types.NodeCourse || text.Type != types.NodeText || !strings.EqualFold(text.Text, "equivalent") {
		return n
	}
	excl := cat.Exclusions[course.Code]
	if excl == nil || len(excl.Items) == 0 {
		return n
	}
	alts := []*types.ReqNode{course}
	for _, it := range excl.Items {
		if len(it.Courses) == 0 || it.Concurrent || it.Note != "" {
			return n
		}
		if !coversWeight(it.Courses, course.Code) {
			continue
		}
		var all []*types.ReqNode
		for _, c := range it.Courses {
			all = append(all, &types.ReqNode{Type: types.NodeCourse, Code: c, MinGrade: course.MinGrade})
		}
		alts = append(alts, AndNode(all...))
	}
	return OrNode(alts...)
}

var (
	// TheirEquivalentsRe matches "The following courses or their equivalents must
	// be completed:".
	TheirEquivalentsRe = regexp.MustCompile(`(?i)\bor\s+their\s+equivalents\b`)
	// CampusEquivRe matches "We will accept equivalent/exclusion statistics
	// courses from UTM/UTSC in lieu of PSY201H1".
	CampusEquivRe = regexp.MustCompile(`(?i)equivalent/exclusion\s+\w+\s+courses\s+from\s+UTM/UTSC\s+in\s+lieu\s+of\s+(` + grammar.CodePat + `(?:\s*(?:,|and)\s*` + grammar.CodePat + `)*)`)
	// OtherCampusRe matches a UTM (5) or UTSC (3) course code: "MATA36H3".
	OtherCampusRe = regexp.MustCompile(`^[A-Z]{3}[A-Z0-9]\d{2}[HY][35]$`)
)

// campusNoteRe matches an exclusion note that only names the courses' campus:
// "(UTSC); (UTSC)".
var campusNoteRe = regexp.MustCompile(`^\(\s*(?:UTM|UTSC)\s*\)(?:;\s*\(\s*(?:UTM|UTSC)\s*\))*$`)

// sharesCourse reports whether two of items accept the same added course, one
// not among the listed ones.
func sharesCourse(items []*types.ReqNode, listed []string) bool {
	seen := map[string]int{}
	for i, it := range items {
		for _, c := range grammar.Dedupe(NodeCodes(it)) {
			if slices.Contains(listed, c) {
				continue
			}
			if j, ok := seen[c]; ok && j != i {
				return true
			}
			seen[c] = i
		}
	}
	return false
}

// OnlyCourses reports whether n is courses combined with and/or.
func OnlyCourses(n *types.ReqNode) bool {
	switch n.Type {
	case types.NodeCourse:
		return true
	case types.NodeAnd, types.NodeOr:
		return !slices.ContainsFunc(n.Items, func(it *types.ReqNode) bool { return !OnlyCourses(it) })
	}
	return false
}

// coversWeight reports whether an excluded set weighs at least as much as
// code: an exclusion marks overlap, so a half course doesn't stand in for a
// full-year one.
func coversWeight(set []string, code string) bool {
	w := 0.0
	for _, c := range set {
		w += grammar.CourseWeight(c)
	}
	return w >= grammar.CourseWeight(code)
}

// EquivalentConfirmText is the condition on a substitute the department
// confirms.
const EquivalentConfirmText = "the department confirms the course qualifies"

// LinkEquivalents reads "or their equivalents" as each required course or one
// it excludes, and adds the department-confirmed UTM and UTSC exclusions
// CampusEquivRe names.
func (cat *Catalog) LinkEquivalents(e *types.Enrolment) {
	if TheirEquivalentsRe.MatchString(e.Raw) {
		e.Req = cat.WithEquivalents(e.Req, nil, nil, false)
	}
	for _, m := range CampusEquivRe.FindAllStringSubmatch(e.Raw, -1) {
		codes := grammar.CodeRe.FindAllString(m[1], -1)
		e.Req = cat.WithEquivalents(e.Req, codes, OtherCampusRe.MatchString, true)
	}
}

// WithEquivalents gives the courses in n (only codes, when given) their
// exclusions as alternatives, keeping grades; keep filters them, and confirm
// adds the department's confirmation. Courses under a not are left alone.
func (cat *Catalog) WithEquivalents(n *types.ReqNode, codes []string, keep func(string) bool, confirm bool) *types.ReqNode {
	if n == nil {
		return nil
	}
	alternatives := func(code string) [][]string {
		if codes != nil && !slices.Contains(codes, code) {
			return nil
		}
		excl := cat.Exclusions[code]
		if excl == nil {
			return nil
		}
		var out [][]string
		for _, it := range excl.Items {
			if len(it.Courses) == 0 || it.Concurrent || it.Note != "" && !campusNoteRe.MatchString(it.Note) || slices.Contains(it.Courses, code) {
				continue
			}
			if keep != nil && slices.ContainsFunc(it.Courses, func(c string) bool { return !keep(c) }) {
				continue
			}
			if !coversWeight(it.Courses, code) {
				continue
			}
			out = append(out, it.Courses)
		}
		return out
	}
	switch n.Type {
	case types.NodeNot:
		return n
	case types.NodeCredits:
		// a pool's courses: "1.0 credit from MAT130H1/ MAT135H1, MAT136H1, ..."
		for _, c := range slices.Clone(n.Courses) {
			for _, set := range alternatives(c) {
				n.Courses = append(n.Courses, set...)
			}
		}
		n.Courses = grammar.Dedupe(n.Courses)
		return n
	case types.NodeAnd:
		// half courses sharing an equivalent need separate courses, so the pair also
		// needs its credits
		before := grammar.CloneNode(n)
		listed := NodeCodes(before)
		for i, it := range n.Items {
			n.Items[i] = cat.WithEquivalents(it, codes, keep, confirm)
		}
		for i := range n.Items {
			for j := i + 1; j < len(before.Items); j++ {
				if OnlyCourses(before.Items[i]) && OnlyCourses(before.Items[j]) && sharesCourse([]*types.ReqNode{n.Items[i], n.Items[j]}, listed) {
					n.Items = append(n.Items, &types.ReqNode{Type: types.NodeCredits, Amount: MinWeight(before.Items[i]) + MinWeight(before.Items[j]),
						Courses: grammar.Dedupe(append(NodeCodes(n.Items[i]), NodeCodes(n.Items[j])...))})
				}
			}
		}
		return Tidy(n)
	case types.NodeCourse:
		alts := alternatives(n.Code)
		if alts == nil {
			return n
		}
		var nodes []*types.ReqNode
		for _, set := range alts {
			var all []*types.ReqNode
			for _, c := range set {
				all = append(all, &types.ReqNode{Type: types.NodeCourse, Code: c, MinGrade: n.MinGrade})
			}
			nodes = append(nodes, AndNode(all...))
		}
		alt := OrNode(nodes...)
		if confirm {
			alt = AndNode(alt, &types.ReqNode{Type: types.NodeText, Kind: types.TextPermission, Text: EquivalentConfirmText})
		}
		return OrNode(n, alt)
	case types.NodeAverage:
		extend := func(courses []string) []string {
			for _, c := range slices.Clone(courses) {
				for _, set := range alternatives(c) {
					courses = append(courses, set...)
				}
			}
			return grammar.Dedupe(courses)
		}
		n.Courses = extend(n.Courses)
		grammar.WalkReq(n, func(it *types.ReqNode) {
			if it != n {
				it.Courses = extend(it.Courses)
			}
		})
		return n
	}
	for i, it := range n.Items {
		n.Items[i] = cat.WithEquivalents(it, codes, keep, confirm)
	}
	return Tidy(n)
}

// HalfSubstitutes gives the full-year courses beside half course code in an or
// a weight of 0.5 when code's catalogue exclusions name them.
func (cat *Catalog) HalfSubstitutes(n *types.ReqNode, code string) {
	excl := cat.Exclusions[code]
	if excl == nil {
		return
	}
	excluded := map[string]bool{}
	for _, it := range excl.Items {
		if len(it.Courses) == 1 && !it.Concurrent && it.Note == "" {
			excluded[it.Courses[0]] = true
		}
	}
	grammar.WalkReq(n, func(o *types.ReqNode) {
		if o.Type != types.NodeOr || !slices.ContainsFunc(o.Items, func(it *types.ReqNode) bool { return it.Type == types.NodeCourse && it.Code == code }) {
			return
		}
		for i, it := range o.Items {
			if it.Type == types.NodeCourse && grammar.CourseWeight(it.Code) == 1 && excluded[it.Code] && it.MinGrade == 0 {
				o.Items[i] = &types.ReqNode{Type: types.NodeCredits, Amount: 0.5, Courses: []string{it.Code}, Weight: 0.5}
			}
		}
	})
}

// NotExclusive returns the first two of codes neither of which lists the other
// as an exclusion, or nil.
func (cat *Catalog) NotExclusive(codes []string) []string {
	excludes := func(a, b string) bool {
		e := cat.Exclusions[a]
		return e != nil && slices.ContainsFunc(e.Items, func(it types.ExclusionItem) bool { return slices.Equal(it.Courses, []string{b}) })
	}
	for i, a := range codes {
		for _, b := range codes[i+1:] {
			if !excludes(a, b) && !excludes(b, a) {
				return []string{a, b}
			}
		}
	}
	return nil
}
