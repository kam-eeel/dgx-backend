package reqs

import (
	"regexp"
	"slices"
	"strings"

	"degree-explorer/types"
)

// Link finishes a scraped catalogue's parsed requirements with the steps that
// need the whole course list or the programs: misprinted codes ("MAT1581"),
// cross-field exemptions, program names, timing exclusions, enrolment links.
func Link(courses []*types.Course, programs []*types.Program) {
	known := map[string]bool{}
	for _, c := range courses {
		known[c.Code] = true
	}
	idx := NewProgramIndex(programs)
	for _, c := range courses {
		c.Prerequisites = repairRequisite(c.Prerequisites, known)
		c.Corequisites = repairRequisite(c.Corequisites, known)
		c.Exclusions = repairExclusions(c.Exclusions, known)
		ApplyExemptions(c.Prerequisites, c.Corequisites)
		idx.Resolve(c.Prerequisites)
		idx.Resolve(c.Corequisites)
		c.Exclusions = applyTiming(c.Exclusions, c.Prerequisites, c.Corequisites)
	}
	for _, p := range programs {
		p.Completion = repairCompletion(p.Completion, known)
	}
	linkCompletionDesignators(courses, programs)
	titles, areas := newTitleIndex(programs), newAreaIndex(programs)
	cat := newEnrolCatalog(courses, programs)
	linkCompletions(courses, programs, titles, cat)
	for _, p := range programs {
		p.Enrolment = repairEnrolment(p.Enrolment, known)
		e := p.Enrolment
		if e == nil {
			continue
		}
		titles.resolveEnrolment(e, p.Code, idx, areas)
		if e.Kind == types.EnrolmentFocus {
			e.Req = titles.narrowToParent(e.Req, p.Title)
		}
		titles.ambiguousExclusions(e, p.Code, programs, requiredPrograms(e.Req), areas)
		oneFocus(e, p.Code, programs)
		area := ""
		if len(p.Areas) > 0 {
			area = p.Areas[0]
		}
		e.Req = cat.linkEquivalent(cat.linkJoint(e.Req, area))
		cat.linkEquivalents(e)
		linkEnrolmentCompletion(e, p.Completion)
	}
	// after the enrolment loop, which narrows a focus's Req: a program enrolment
	// requires is never a completion exclusion
	for _, p := range programs {
		if p.Completion != nil {
			var req *types.ReqNode
			if p.Enrolment != nil {
				req = p.Enrolment.Req
			}
			titles.linkCompletionExclusions(p.Completion, p.Code, req, programs, idx, areas)
		}
	}
}

var (
	// nearCodeRe matches a code missing its H/Y, or its H/Y and campus digit: "MAT1581".
	nearCodeRe = regexp.MustCompile(`\b([A-Z]{3}\d{3})1?\b`)
	// codeFollowRe matches what may follow a code in a list: " or B". In "consult
	// the BIO120 Office" it is a name, not a course.
	codeFollowRe = regexp.MustCompile(`^\s*(?:$|[/,);.:]|(?i:or|and)\b)`)
	// notConcurrentRe matches a timing restriction: "May not be taken concurrently
	// with FSL312H1".
	notConcurrentRe = regexp.MustCompile(`(?i)\b(?:(?:may|must|should)\s+not|cannot|can\s+not)\s+be\s+taken\s+concurrently\s+with\s+(.+?)\.?$`)
	codeListRe      = regexp.MustCompile(`^` + codePat + `(?:\s*(?:,|/|(?i:and|or))\s*` + codePat + `)*$`)
)

// repairCatalogCodes repairs near-codes in s that match one course in known
// ("MAT1581" is MAT158H1 if there's no MAT158Y1), if what follows can follow a
// code in a list ("MAT100-series" can't), and reports whether anything changed.
func repairCatalogCodes(s string, known map[string]bool) (string, bool) {
	return repairCodesFollowedBy(s, known, codeFollowRe)
}

// repairCodesFollowedBy is repairCatalogCodes with its own rule for what may
// follow a near-code.
func repairCodesFollowedBy(s string, known map[string]bool, follow *regexp.Regexp) (string, bool) {
	var b strings.Builder
	last := 0
	for _, m := range nearCodeRe.FindAllStringIndex(s, -1) {
		if !follow.MatchString(s[m[1]:]) {
			continue
		}
		var hits []string
		for _, w := range []string{"H1", "Y1"} {
			if code := s[m[0]:m[0]+6] + w; known[code] {
				hits = append(hits, code)
			}
		}
		if len(hits) == 1 {
			b.WriteString(s[last:m[0]] + hits[0])
			last = m[1]
		}
	}
	if last == 0 {
		return s, false
	}
	return b.String() + s[last:], true
}

func repairRequisite(q *types.Requisite, known map[string]bool) *types.Requisite {
	if q == nil {
		return nil
	}
	fixed, ok := repairCatalogCodes(q.Raw, known)
	if !ok {
		return q
	}
	r := ParseRequisite(fixed)
	r.Raw = q.Raw
	return r
}

func repairExclusions(e *types.Exclusions, known map[string]bool) *types.Exclusions {
	if e == nil {
		return nil
	}
	fixed, ok := repairCatalogCodes(e.Raw, known)
	if !ok {
		return e
	}
	r := ParseExclusions(fixed)
	r.Raw = e.Raw
	return r
}

// timingItems reads a "may not be taken concurrently with A, B" sentence as
// concurrent exclusion items, one per course. It returns nil for anything
// else, including a list that isn't only course codes.
func timingItems(sentence string) []types.ExclusionItem {
	m := notConcurrentRe.FindStringSubmatch(sentence)
	if m == nil || !codeListRe.MatchString(strings.TrimSpace(m[1])) {
		return nil
	}
	var items []types.ExclusionItem
	for _, c := range codeRe.FindAllString(m[1], -1) {
		items = append(items, types.ExclusionItem{Courses: []string{c}, Concurrent: true})
	}
	return items
}

// applyTiming adds the timing restrictions found in the requisites' notes to
// the exclusions, creating them if the field was empty.
func applyTiming(e *types.Exclusions, qs ...*types.Requisite) *types.Exclusions {
	for _, q := range qs {
		if q == nil {
			continue
		}
		for _, note := range q.Notes {
			items := timingItems(note)
			if items == nil {
				continue
			}
			if e == nil {
				e = &types.Exclusions{Items: []types.ExclusionItem{}}
			}
			for _, it := range items {
				if !slices.ContainsFunc(e.Items, func(x types.ExclusionItem) bool { return itemKey(x) == itemKey(it) }) {
					e.Items = append(e.Items, it)
				}
			}
			if !slices.Contains(e.Notes, note) {
				e.Notes = append(e.Notes, note)
			}
		}
	}
	return e
}
