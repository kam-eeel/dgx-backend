package reqs

import (
	"regexp"
	"slices"
	"strings"

	"degree-explorer/types"
)

// Linking steps for enrolment that read the rest of the catalogue: which
// jointly offered courses a subject uses, and what a course's equivalents
// are.

// enrolCatalog is what those steps need from the catalogue.
type enrolCatalog struct {
	exclusions map[string]*types.Exclusions // course code -> its exclusions
	joint      map[string][]string          // first area -> joint designators its programs list
}

// jointCodeRe is a jointly offered course: "JEG100H1", "JGU346H1".
var jointCodeRe = regexp.MustCompile(`\bJ[A-Z]{2}\d{3}[HY]\d\b`)

func newEnrolCatalog(courses []*types.Course, programs []*types.Program) *enrolCatalog {
	cat := &enrolCatalog{exclusions: map[string]*types.Exclusions{}, joint: map[string][]string{}}
	for _, c := range courses {
		cat.exclusions[c.Code] = c.Exclusions
	}
	for _, p := range programs {
		if len(p.Areas) == 0 || p.Completion == nil {
			continue
		}
		area := p.Areas[0]
		for _, code := range jointCodeRe.FindAllString(*p.Completion, -1) {
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

// linkJoint replaces the jointText alternative to a subject's pools with the
// subject's joint designators: those the programs of the same area list in
// their completion requirements that share the subject's letter. Geography
// programs list JEG, JGE, JGU, JIG, ... (G for Geography) as well as JAV and
// JQR (other subjects' joint courses) and never JGJ (German) or JGA, so the
// GGR pools and averages get JEG, JGC, JGE, JGI, JGU, JIG and JUG. Without
// a list for the area the alternative stays.
func (cat *enrolCatalog) linkJoint(n *types.ReqNode, area string) *types.ReqNode {
	subjects := map[string][]string{}
	walkReq(n, func(t *types.ReqNode) {
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
	walkReq(n, func(c *types.ReqNode) {
		if (c.Type == types.NodeCredits || c.Type == types.NodeAverage) && len(c.Designators) > 0 {
			if joint, ok := subjects[c.Designators[0]]; ok {
				c.Designators = dedupe(append(c.Designators, joint...))
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
			return tidy(orNode(slices.DeleteFunc(slices.Clone(t.Items), resolved)...))
		}
		return t
	}
	return drop(n)
}

// linkEquivalent reads "MGT100H1 or equivalent" as the course or one of the
// courses it excludes: the calendar lists a course's equivalents as its
// exclusions. It is left alone when an exclusion isn't a list of courses.
func (cat *enrolCatalog) linkEquivalent(n *types.ReqNode) *types.ReqNode {
	if n == nil {
		return nil
	}
	for i, it := range n.Items {
		n.Items[i] = cat.linkEquivalent(it)
	}
	if n.Type != types.NodeOr || len(n.Items) != 2 {
		return n
	}
	course, text := n.Items[0], n.Items[1]
	if course.Type != types.NodeCourse || text.Type != types.NodeText || !strings.EqualFold(text.Text, "equivalent") {
		return n
	}
	excl := cat.exclusions[course.Code]
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
		alts = append(alts, andNode(all...))
	}
	return orNode(alts...)
}

var (
	// "The following courses or their equivalents must be completed:",
	// "Minimum grades in required courses or their equivalents are needed
	// for entry".
	theirEquivalentsRe = regexp.MustCompile(`(?i)\bor\s+their\s+equivalents\b`)
	// "We will accept equivalent/exclusion statistics courses from UTM/UTSC
	// in lieu of PSY201H1 and PSY202H1. Please contact us to verify if your
	// stats course qualifies."
	campusEquivRe = regexp.MustCompile(`(?i)equivalent/exclusion\s+\w+\s+courses\s+from\s+UTM/UTSC\s+in\s+lieu\s+of\s+(` + codePat + `(?:\s*(?:,|and)\s*` + codePat + `)*)`)
	// A UTM (5) or UTSC (3) course code.
	otherCampusRe = regexp.MustCompile(`^[A-Z]{3}[A-Z0-9]\d{2}[HY][35]$`)
)

// campusNoteRe is an exclusion note that only names the courses' campus:
// "CHMB62H3(UTSC)", "[ BIOC12H3(UTSC) and BIOC13H3(UTSC)]" ("(UTSC); (UTSC)").
var campusNoteRe = regexp.MustCompile(`^\(\s*(?:UTM|UTSC)\s*\)(?:;\s*\(\s*(?:UTM|UTSC)\s*\))*$`)

// sharesCourse reports whether two of items accept the same added course,
// one that isn't among the listed ones.
func sharesCourse(items []*types.ReqNode, listed []string) bool {
	seen := map[string]int{}
	for i, it := range items {
		for _, c := range dedupe(nodeCodes(it)) {
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

// onlyCourses reports whether n is courses combined with and/or.
func onlyCourses(n *types.ReqNode) bool {
	switch n.Type {
	case types.NodeCourse:
		return true
	case types.NodeAnd, types.NodeOr:
		return !slices.ContainsFunc(n.Items, func(it *types.ReqNode) bool { return !onlyCourses(it) })
	}
	return false
}

// coversWeight reports whether an excluded set of courses weighs at least as
// much as code. An exclusion marks overlap, not equivalence: CHM151Y1
// excludes CHM135H1, but one half course doesn't stand in for the full-year
// course. The other way round it does (CHM151Y1 for CHM135H1).
func coversWeight(set []string, code string) bool {
	w := 0.0
	for _, c := range set {
		w += courseWeight(c)
	}
	return w >= courseWeight(code)
}

// equivalentConfirmText is the condition on a substitute the department
// confirms.
const equivalentConfirmText = "the department confirms the course qualifies"

// linkEquivalents reads "or their equivalents" as every required course or
// one of the courses it excludes, as linkEquivalent does for "X or
// equivalent". "We will accept equivalent/exclusion statistics courses from
// UTM/UTSC in lieu of PSY201H1" adds PSY201H1's UTM and UTSC exclusions,
// which the department confirms.
func (cat *enrolCatalog) linkEquivalents(e *types.Enrolment) {
	if theirEquivalentsRe.MatchString(e.Raw) {
		e.Req = cat.withEquivalents(e.Req, nil, nil, false)
	}
	for _, m := range campusEquivRe.FindAllStringSubmatch(e.Raw, -1) {
		codes := codeRe.FindAllString(m[1], -1)
		e.Req = cat.withEquivalents(e.Req, codes, otherCampusRe.MatchString, true)
	}
}

// withEquivalents gives the courses in n (only codes, when given) their
// exclusions as alternatives, keeping its minimum grade; keep, when given,
// says which excluded courses count. With confirm, an alternative also needs
// the department's confirmation. Courses under a not are left alone. An
// average over a course averages its equivalents too.
func (cat *enrolCatalog) withEquivalents(n *types.ReqNode, codes []string, keep func(string) bool, confirm bool) *types.ReqNode {
	if n == nil {
		return nil
	}
	alternatives := func(code string) [][]string {
		if codes != nil && !slices.Contains(codes, code) {
			return nil
		}
		excl := cat.exclusions[code]
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
		// A pool's courses: "1.0 credit from MAT130H1/ MAT135H1, MAT136H1,
		// ..., PHY131H1".
		for _, c := range slices.Clone(n.Courses) {
			for _, set := range alternatives(c) {
				n.Courses = append(n.Courses, set...)
			}
		}
		n.Courses = dedupe(n.Courses)
		return n
	case types.NodeAnd:
		// Half courses that share an equivalent ("( MAT130H1/ MAT135H1,
		// MAT136H1)": MATA36H3 excludes both MAT135H1 and MAT136H1) need
		// separate courses, so the pair also needs its credits.
		before := cloneNode(n)
		listed := nodeCodes(before)
		for i, it := range n.Items {
			n.Items[i] = cat.withEquivalents(it, codes, keep, confirm)
		}
		for i := range n.Items {
			for j := i + 1; j < len(before.Items); j++ {
				if onlyCourses(before.Items[i]) && onlyCourses(before.Items[j]) && sharesCourse([]*types.ReqNode{n.Items[i], n.Items[j]}, listed) {
					n.Items = append(n.Items, &types.ReqNode{Type: types.NodeCredits, Amount: minWeight(before.Items[i]) + minWeight(before.Items[j]),
						Courses: dedupe(append(nodeCodes(n.Items[i]), nodeCodes(n.Items[j])...))})
				}
			}
		}
		return tidy(n)
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
			nodes = append(nodes, andNode(all...))
		}
		alt := orNode(nodes...)
		if confirm {
			alt = andNode(alt, &types.ReqNode{Type: types.NodeText, Kind: types.TextPermission, Text: equivalentConfirmText})
		}
		return orNode(n, alt)
	case types.NodeAverage:
		extend := func(courses []string) []string {
			for _, c := range slices.Clone(courses) {
				for _, set := range alternatives(c) {
					courses = append(courses, set...)
				}
			}
			return dedupe(courses)
		}
		n.Courses = extend(n.Courses)
		walkReq(n, func(it *types.ReqNode) {
			if it != n {
				it.Courses = extend(it.Courses)
			}
		})
		return n
	}
	for i, it := range n.Items {
		n.Items[i] = cat.withEquivalents(it, codes, keep, confirm)
	}
	return tidy(n)
}
