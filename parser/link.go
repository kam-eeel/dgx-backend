package parser

import (
	"regexp"
	"slices"
	"strconv"

	"degree-explorer/parser/courses"
	"degree-explorer/parser/grammar"
	"degree-explorer/parser/programs"
	"degree-explorer/parser/programs/completion"
	"degree-explorer/parser/programs/enrolment"
	"degree-explorer/types"
)

// Link finishes a scraped catalogue's parsed requirements with the steps that
// need the whole course list or the programs: misprinted codes ("MAT1581"),
// cross-field exemptions, program names, timing exclusions, enrolment links.
func Link(courseList []*types.Course, programList []*types.Program) {
	known := map[string]bool{}
	for _, c := range courseList {
		known[c.Code] = true
	}
	idx := programs.NewProgramIndex(programList)
	for _, c := range courseList {
		c.Prerequisites = courses.RepairRequisite(c.Prerequisites, known)
		c.Corequisites = courses.RepairRequisite(c.Corequisites, known)
		c.Exclusions = courses.RepairExclusions(c.Exclusions, known)
		courses.ApplyExemptions(c.Prerequisites, c.Corequisites)
		idx.Resolve(c.Prerequisites)
		idx.Resolve(c.Corequisites)
		c.Exclusions = courses.ApplyTiming(c.Exclusions, c.Prerequisites, c.Corequisites)
	}
	for _, p := range programList {
		p.Completion = completion.Repair(p.Completion, known)
	}
	completion.LinkDesignators(courseList, programList)
	titles, areas := programs.NewTitleIndex(programList), programs.NewAreaIndex(programList)
	cat := programs.NewCatalog(courseList, programList)
	completion.Link(courseList, programList, titles, cat)
	for _, p := range programList {
		p.Enrolment = enrolment.Repair(p.Enrolment, known)
		e := p.Enrolment
		if e == nil {
			continue
		}
		titles.ResolveEnrolment(e, p.Code, idx, areas)
		if e.Kind == types.EnrolmentFocus {
			e.Req = titles.NarrowToParent(e.Req, p.Title)
		}
		titles.AmbiguousExclusions(e, p.Code, programList, programs.RequiredPrograms(e.Req), areas)
		enrolment.OneFocus(e, p.Code, programList)
		area := ""
		if len(p.Areas) > 0 {
			area = p.Areas[0]
		}
		e.Req = cat.LinkEquivalent(cat.LinkJoint(e.Req, area))
		cat.LinkEquivalents(e)
		linkEnrolmentCompletion(e, p.Completion)
	}
	// after the enrolment loop, which narrows a focus's Req: a program enrolment
	// requires is never a completion exclusion
	for _, p := range programList {
		if p.Completion != nil {
			var req *types.ReqNode
			if p.Enrolment != nil {
				req = p.Enrolment.Req
			}
			completion.LinkExclusions(titles, p.Completion, p.Code, req, programList, idx, areas)
		}
	}
}

// enrolCountTowardRe matches an average over courses completion counts: "A
// minimum grade average of 77% in 3.0 credits that count toward the program".
var enrolCountTowardRe = regexp.MustCompile(`(?i)^a\s+minimum\s+grade\s+average\s+of\s+(\d{2})\s*%\s+in\s+` + grammar.AmountPat + `\s+credits?\s+that\s+counts?\s+towards?\s+the\s+program$`)

// linkEnrolmentCompletion reads enrolCountTowardRe as a best-grades average
// over the completion rows that claim courses. While such a row has open
// text, it is or(average, text) (Review); with no such row the text stays.
func linkEnrolmentCompletion(e *types.Enrolment, c *types.Completion) {
	if e == nil || c == nil {
		return
	}
	var rows []int
	open := false
	for _, r := range c.Requirements {
		if r.Kind != types.RequirementAllocation || r.Req == nil || r.Number == 1 && completion.Total(c) && !ownPool(r.Req) {
			continue
		}
		rows = append(rows, r.Number)
		open = open || completion.OpenText(r.Req)
	}
	if rows == nil {
		return
	}
	var fix func(n *types.ReqNode) *types.ReqNode
	fix = func(n *types.ReqNode) *types.ReqNode {
		if n == nil {
			return nil
		}
		for i, it := range n.Items {
			n.Items[i] = fix(it)
		}
		m := enrolCountTowardRe.FindStringSubmatch(n.Text)
		if n.Type != types.NodeText || m == nil {
			return n
		}
		grade, _ := strconv.Atoi(m[1])
		amount, _ := strconv.ParseFloat(m[2], 64)
		avg := &types.ReqNode{Type: types.NodeAverage, MinGrade: grade, Amount: amount, Best: true, Requirements: slices.Clone(rows)}
		if open {
			return programs.OrNode(avg, n)
		}
		return avg
	}
	e.Req = fix(e.Req)
}

// ownPool reports whether a total's body counts courses of its own: a credits
// node, or the first item of its Review marker, with a filter.
func ownPool(n *types.ReqNode) bool {
	if n.Type == types.NodeOr && len(n.Items) > 0 {
		n = n.Items[0]
	}
	return n.Type == types.NodeCredits && (n.Designators != nil || n.Levels != nil || n.Courses != nil || n.Breadth != nil || n.Lists != nil)
}
