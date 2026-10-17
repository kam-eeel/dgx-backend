package completion

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/parser/grammar"
	"degree-explorer/parser/programs"
	"degree-explorer/types"
)

// Linking program completion: the steps that need the catalogue.

// LinkDesignators keeps in completion filters only designators the
// catalogue has or the program's text names as a prefix, noting those dropped.
// A pool left with nothing becomes text (Review); a limit keeps its credits node.
func LinkDesignators(courses []*types.Course, programs []*types.Program) {
	known := map[string]bool{}
	for _, c := range courses {
		if len(c.Code) >= 3 {
			known[c.Code[:3]] = true
		}
	}
	for _, p := range programs {
		c := p.Completion
		if c == nil {
			continue
		}
		named := map[string]bool{}
		for _, code := range grammar.CodeRe.FindAllString(c.Raw, -1) {
			named[code[:3]] = true
		}
		texted := false
		for _, r := range c.Requirements {
			var dropped []string
			var fix func(n *types.ReqNode) *types.ReqNode
			fix = func(n *types.ReqNode) *types.ReqNode {
				if n == nil {
					return nil
				}
				for i, it := range n.Items {
					n.Items[i] = fix(it)
				}
				if n.ExcludeDesignators != nil {
					keep := slices.DeleteFunc(slices.Clone(n.ExcludeDesignators), func(d string) bool { return !known[d] && !named[d] })
					for _, d := range n.ExcludeDesignators {
						if !slices.Contains(keep, d) {
							dropped = append(dropped, d)
						}
					}
					if n.ExcludeDesignators = keep; len(keep) == 0 {
						n.ExcludeDesignators = nil
					}
				}
				if n.Designators == nil {
					return n
				}
				keep := slices.DeleteFunc(slices.Clone(n.Designators), func(d string) bool { return !known[d] && !named[d] })
				if len(keep) == len(n.Designators) {
					return n
				}
				for _, d := range n.Designators {
					if !slices.Contains(keep, d) {
						dropped = append(dropped, d)
					}
				}
				switch {
				case len(keep) > 0:
					n.Designators = keep
				case n.Courses != nil:
					n.Designators = nil // the listed courses are the pool
				case r.Kind == types.RequirementLimit:
					return n // a limit stays a credits node
				default:
					texted = true
					return TextNode(types.TextOther, RawHead(r))
				}
				return n
			}
			r.Req = fix(r.Req)
			if dropped = grammar.Dedupe(dropped); len(dropped) == 1 {
				r.Notes = append(r.Notes, "No course in the calendar has the designator "+dropped[0]+".")
			} else if dropped != nil {
				r.Notes = append(r.Notes, "No course in the calendar has the designators "+strings.Join(dropped, ", ")+".")
			}
		}
		if texted {
			markIncomplete(c, Total(c), false)
		}
	}
}

// Total reports whether row 1 is a header total: a top allocation
// with a credits body and child allocations.
func Total(c *types.Completion) bool {
	if len(c.Requirements) == 0 {
		return false
	}
	r1 := c.Requirements[0]
	return r1.Parent == 0 && r1.Kind == types.RequirementAllocation && r1.Req != nil &&
		slices.ContainsFunc(c.Requirements, func(r *types.Requirement) bool {
			return r.Parent == 1 && r.Kind == types.RequirementAllocation
		})
}

// ---- Code repair ----

var (
	// noCampusRe matches a code missing its campus digit: "CDN307H". Not
	// "CSC300H-level".
	noCampusRe = regexp.MustCompile(`\b([A-Z]{3}\d{3}[HY])\b(?:[^-+]|$)`)
	// shortDesigRe matches a code with a two-letter designator: "HS324H1".
	shortDesigRe = regexp.MustCompile(`\b([A-Z]{2})(\d{3}[HY]\d)\b`)
)

// Repair repairs catalogue-confirmed codes in the raw text, as for
// requisites and enrolment, and parses it again, keeping Raw. Uncatalogued
// ("CSB332H") and graduate codes are kept.
func Repair(c *types.Completion, known map[string]bool) *types.Completion {
	if c == nil {
		return nil
	}
	fixed, ok := repairCompletionCodes(c.Raw, known)
	if !ok {
		return c
	}
	r := Parse(fixed)
	r.Raw = c.Raw
	return r
}

// repairCompletionCodes returns s with Repair's repairs, and whether
// there were any.
func repairCompletionCodes(s string, known map[string]bool) (string, bool) {
	changed := false
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		var b strings.Builder
		last := 0
		for _, m := range noCampusRe.FindAllStringSubmatchIndex(line, -1) {
			if code := line[m[2]:m[3]] + "1"; known[code] {
				b.WriteString(line[last:m[2]] + code)
				last = m[3]
			}
		}
		if last > 0 {
			line = b.String() + line[last:]
			changed = true
		}
		if fixed, ok := grammar.RepairCatalogCodes(line, known); ok {
			line, changed = fixed, true
		}
		line = shortDesigRe.ReplaceAllStringFunc(line, func(w string) string {
			onLine := map[string]bool{}
			for _, code := range grammar.CodeRe.FindAllString(line, -1) {
				onLine[code[:3]] = true
			}
			var hits []string
			for p := 0; p <= 2; p++ {
				for l := 'A'; l <= 'Z'; l++ {
					d := w[:p] + string(l) + w[p:2]
					if code := d + w[2:]; known[code] && onLine[d] && !slices.Contains(hits, code) {
						hits = append(hits, code)
					}
				}
			}
			if len(hits) != 1 {
				return w
			}
			changed = true
			return hits[0]
		})
		lines[i] = line
	}
	return strings.Join(lines, "\n"), changed
}

// ---- Single-topic special-topics courses ----

// linkCodeRanges keeps, of the codes a range in a row's raw spans, those the
// catalogue has and the ends, in each pool holding the whole span. The Review
// markers stay: remark would take a cap's permission sentence for one.
func linkCodeRanges(c *types.Completion, titles map[string]string) {
	for _, m := range codeRangeRe.FindAllStringSubmatch(completionRaws(c), -1) {
		span, ends := codeRange(m[1], m[2]), m[1:3]
		if span == nil {
			continue
		}
		for _, r := range c.Requirements {
			grammar.WalkReq(r.Req, func(n *types.ReqNode) {
				if n.Type != types.NodeCredits || slices.ContainsFunc(span, func(x string) bool { return !slices.Contains(n.Courses, x) }) {
					return
				}
				n.Courses = slices.DeleteFunc(n.Courses, func(x string) bool {
					_, known := titles[x]
					return slices.Contains(span, x) && !known && !slices.Contains(ends, x)
				})
			})
		}
	}
}

// completionRaws returns the distinct raws of c's rows, one a line.
func completionRaws(c *types.Completion) string {
	var raws []string
	for _, r := range c.Requirements {
		if !slices.Contains(raws, r.Raw) {
			raws = append(raws, r.Raw)
		}
	}
	return strings.Join(raws, "\n")
}

// topicsTitleRe matches a catalogue title of a special-topics course,
// whose content changes with each offering: "Topics in History".
var topicsTitleRe = regexp.MustCompile(`(?i)^(?:(?:advanced|selected|special|open|current)\s+)*topics\b`)

// linkTopics makes a special-topics course listed with one topic a maybe, as
// the parser does for two or more: or(pool without it, and(pool, text)), or
// and(course, text). It reports whether a row changed.
func linkTopics(c *types.Completion, titles map[string]string) bool {
	changed := false
	for _, r := range c.Requirements {
		// a limit caps a course whatever its topic
		if r.Req == nil || r.Kind == types.RequirementLimit {
			continue
		}
		mentions := map[string][]string{}
		for _, m := range topicParenRe.FindAllStringSubmatch(r.Raw, -1) {
			if !slices.Contains(mentions[m[1]], m[0]) {
				mentions[m[1]] = append(mentions[m[1]], m[0])
			}
		}
		topics := map[string][]string{}
		for code, ms := range mentions {
			topic := strings.TrimSpace(topicParenRe.FindStringSubmatch(ms[0])[2])
			title := titles[code]
			// two or more topics are the parser's
			if len(ms) != 1 || !topicsTitleRe.MatchString(title) || !isTopic(topic, title) {
				continue
			}
			if !slices.Contains(programs.NodeCodes(r.Req), code) || !poolsWithout(r.Req, code) {
				continue
			}
			topics[code] = ms
		}
		if len(topics) == 0 {
			continue
		}
		r.Req = programs.Tidy(topicMembers(r.Req, topics))
		changed = true
	}
	return changed
}

// isTopic reports whether a bracket after a special-topics course names a
// topic, not the course's own title: "Hungarian History".
func isTopic(topic, title string) bool {
	if topic == "" || !titleWordsRe.MatchString(topic) || topic[0] < 'A' || topic[0] > 'Z' {
		return false
	}
	t, n := programs.NormSubject(title), programs.NormSubject(topic)
	return !strings.Contains(" "+t+" ", " "+n+" ")
}

// poolsWithout reports whether every pool in n listing code can still make its
// amount without it.
func poolsWithout(n *types.ReqNode, code string) bool {
	ok := true
	grammar.WalkReq(n, func(p *types.ReqNode) {
		if p.Type != types.NodeCredits || !slices.Contains(p.Courses, code) || p.Designators != nil || p.Levels != nil || p.Breadth != nil {
			return
		}
		sum := 0.0
		for _, c := range grammar.Dedupe(p.Courses) {
			if c != code {
				sum += grammar.CourseWeight(c)
			}
		}
		if sum < p.Amount-1e-9 {
			ok = false
		}
	})
	return ok
}

// otherTopicsRe matches a permission over a designator's other topics
// courses: "Some offerings of other POL Special Topics courses ... may also be
// eligible".
var otherTopicsRe = regexp.MustCompile(`(?i)\bother\s+([A-Z]{3})\s+(?:special\s+)?topics\s+courses\b.*\bmay\s+also\s+be\s+eligible\b`)

// topicsRouteRe matches a designator's special-topics courses in a route:
// "RSM special topics course". Not "RSM special topics and RSM Summer Abroad
// courses": the catalogue doesn't mark summer-abroad courses.
var topicsRouteRe = regexp.MustCompile(`(?i)\b(` + grammar.DesigPat + `)\s+(?:special\s+)?topics\s+courses?\b`)

// linkOtherTopics bounds a permission naming a designator's special-topics
// courses by the courses the catalogue titles "Topics in": and(the pool plus
// those courses, text). It reports whether a row changed.
func linkOtherTopics(c *types.Completion, byDesignator map[string][]string) bool {
	changed := false
	for _, r := range c.Requirements {
		n := r.Req
		if n == nil || n.Type != types.NodeOr || len(n.Items) < 2 {
			continue
		}
		pool := n.Items[0]
		if pool.Type != types.NodeCredits || pool.Courses == nil || pool.Designators != nil || pool.Levels != nil || pool.MaxAmount != nil || pool.Breadth != nil || pool.Exclude != nil {
			continue
		}
		for i, it := range n.Items[1:] {
			var wider *types.ReqNode
			switch {
			case it.Type == types.NodeText && it.Kind == types.TextPermission:
				m := otherTopicsRe.FindStringSubmatch(it.Text)
				if m == nil || byDesignator[m[1]] == nil {
					continue
				}
				wider = grammar.CloneNode(pool)
				wider.Courses = grammar.Dedupe(append(wider.Courses, byDesignator[m[1]]...))
				it = programs.AndNode(wider, it)
			case it.Type == types.NodeAnd && len(it.Items) == 2 && it.Items[1].Type == types.NodeText && it.Items[1].Kind == types.TextPermission:
				w, text := it.Items[0], it.Items[1]
				m := topicsRouteRe.FindStringSubmatch(text.Text)
				if m == nil || w.Type != types.NodeCredits || !slices.Equal(w.Designators, []string{m[1]}) || w.Amount != pool.Amount ||
					w.Breadth != nil || w.Exclude != nil || w.MaxAmount != nil || !slices.Equal(w.Courses, pool.Courses) {
					continue
				}
				var extra []string
				for _, code := range byDesignator[m[1]] {
					if grammar.LevelMatches(w.Levels, code) {
						extra = append(extra, code)
					}
				}
				if extra == nil {
					continue
				}
				wider = grammar.CloneNode(pool)
				wider.Courses = grammar.Dedupe(append(wider.Courses, extra...))
				it.Items[0] = wider
			default:
				continue
			}
			n.Items[i+1] = it
			changed = true
		}
	}
	return changed
}

// ---- List members the catalogue doesn't have ----

// linkListCodes fixes a list code with an unknown designator when one
// catalogued code is a character away and a same-area program's same-named
// list has it; otherwise it notes the code is missing. It reports a change.
func linkListCodes(p *types.Program, programList []*types.Program, titles map[string]string) bool {
	c := p.Completion
	if c.Lists == nil {
		return false
	}
	designators := map[string]bool{}
	for code := range titles {
		designators[code[:3]] = true
	}
	sibling := func(name, code string) bool {
		for _, q := range programList {
			if q == p || q.Completion == nil || !slices.ContainsFunc(q.Areas, func(x string) bool { return slices.Contains(p.Areas, x) }) {
				continue
			}
			for _, l := range q.Completion.Lists {
				if l.Name == name && slices.ContainsFunc(l.Items, func(it *types.ReqNode) bool { return slices.Contains(programs.NodeCodes(it), code) }) {
					return true
				}
			}
		}
		return false
	}
	changed := false
	repaired := map[string]string{}
	for _, l := range c.Lists {
		var held []string
		for _, it := range l.Items {
			held = append(held, programs.NodeCodes(it)...)
		}
		for _, it := range l.Items {
			grammar.WalkReq(it, func(n *types.ReqNode) {
				for i, code := range n.Courses {
					if _, ok := titles[code]; ok || designators[code[:3]] || len(code) != 8 {
						continue
					}
					var near []string
					for k := range titles {
						if len(k) == 8 && k[3:] == code[3:] && oneApart(k[:3], code[:3]) {
							near = append(near, k)
						}
					}
					if len(near) == 1 && !slices.Contains(held, near[0]) && sibling(l.Name, near[0]) {
						n.Courses[i] = near[0]
						repaired[code] = near[0]
						c.Notes = grammar.Dedupe(append(c.Notes, "No course in the calendar has the code "+code+" ("+l.Name+"): read as "+near[0]+", which a program of the same area lists on its "+l.Name+"."))
					} else {
						c.Notes = grammar.Dedupe(append(c.Notes, "No course in the calendar has the code "+code+" ("+l.Name+")."))
					}
					changed = true
				}
			})
		}
	}
	for _, r := range c.Requirements {
		grammar.WalkReq(r.Req, func(n *types.ReqNode) {
			for _, codes := range [][]string{n.Courses, n.Exclude} {
				for i, code := range codes {
					if to, ok := repaired[code]; ok {
						codes[i] = to
					}
				}
			}
		})
	}
	return changed
}

// oneApart reports whether two equal-length strings differ in one place.
func oneApart(a, b string) bool {
	d := 0
	for i := range a {
		if a[i] != b[i] {
			d++
		}
	}
	return d == 1
}

// ---- Equivalents ----

// linkCompletionEquivalents reads the catalogue's equivalents, as enrolment
// does: "X or equivalent", "(or their equivalents)" leads, and accepted
// UTM/UTSC equivalents. It reports whether a row changed.
func linkCompletionEquivalents(cat *programs.Catalog, c *types.Completion) bool {
	changed, applied := false, false
	theirs := programs.TheirEquivalentsRe.FindStringIndex(c.Raw)
	for _, r := range c.Requirements {
		if r.Req == nil {
			continue
		}
		before := NodeKey(r.Req)
		r.Req = cat.LinkEquivalent(r.Req)
		if theirs != nil && r.Kind == types.RequirementAllocation {
			if at := strings.Index(c.Raw, RawHead(r)); at > theirs[1] {
				r.Req = cat.WithEquivalents(r.Req, nil, nil, false)
				applied = true
				lead := c.Raw[strings.LastIndex(c.Raw[:theirs[0]], "\n")+1:]
				lead, _, _ = strings.Cut(lead, "\n")
				r.Raw += "\n" + strings.TrimSpace(lead)
				// the text alternative is encoded now, and the row's raw holds it
				codes := programs.NodeCodes(r.Req)
				r.Req = programs.Tidy(dropText(r.Req, func(t *types.ReqNode) bool {
					m := desigEquivalentsRe.FindStringSubmatch(t.Text)
					if m == nil || !slices.ContainsFunc(codes, func(x string) bool { return strings.HasPrefix(x, m[1]) }) {
						return false
					}
					if !strings.Contains(rawWords(r.Raw), rawWords(t.Text)) {
						r.Raw += "\n" + strings.TrimSpace(t.Text)
					}
					return true
				}))
			}
		}
		for _, m := range programs.CampusEquivRe.FindAllStringSubmatch(r.Raw, -1) {
			var codes []string
			for _, code := range grammar.CodeRe.FindAllString(m[1], -1) {
				if slices.Contains(programs.NodeCodes(r.Req), code) {
					codes = append(codes, code)
				}
			}
			if codes == nil {
				continue
			}
			with, all := grammar.CloneNode(r.Req), true
			for _, code := range codes {
				n := len(programs.NodeCodes(with))
				with = cat.WithEquivalents(with, []string{code}, programs.OtherCampusRe.MatchString, true)
				all = all && len(programs.NodeCodes(with)) > n
				if grammar.CourseWeight(code) == 0.5 {
					halfEquivalents(with)
					cat.HalfSubstitutes(with, code)
				}
			}
			if !all {
				continue
			}
			r.Req = programs.Tidy(dropText(with, func(t *types.ReqNode) bool {
				return t.Kind == types.TextPermission && programs.CampusEquivRe.MatchString(t.Text)
			}))
		}
		if NodeKey(r.Req) != before {
			changed = true
		}
	}
	if applied {
		// the lead is encoded now: not a note any more
		c.Notes = slices.DeleteFunc(c.Notes, programs.TheirEquivalentsRe.MatchString)
		if len(c.Notes) == 0 {
			c.Notes = nil
		}
	}
	return changed
}

// halfEquivalents makes a full-year course among a half course's confirmed
// equivalents count 0.5 of its weight: the rest of it can count elsewhere.
func halfEquivalents(n *types.ReqNode) {
	grammar.WalkReq(n, func(a *types.ReqNode) {
		if a.Type != types.NodeAnd || !slices.ContainsFunc(a.Items, func(t *types.ReqNode) bool {
			return t.Type == types.NodeText && t.Text == programs.EquivalentConfirmText
		}) {
			return
		}
		half := func(c *types.ReqNode) *types.ReqNode {
			if c.Type == types.NodeCourse && grammar.CourseWeight(c.Code) == 1 {
				return &types.ReqNode{Type: types.NodeCredits, Amount: 0.5, Courses: []string{c.Code}, Weight: 0.5}
			}
			return c
		}
		for i, it := range a.Items {
			a.Items[i] = half(it)
			if it.Type == types.NodeOr {
				for j, c := range it.Items {
					it.Items[j] = half(c)
				}
			}
		}
	})
}

// desigEquivalentsRe matches "would be able to use any completed RSM
// course equivalents".
var desigEquivalentsRe = regexp.MustCompile(`(?i)\bwould\s+be\s+able\s+to\s+use\s+any\s+completed\s+([A-Z]{3})\s+course\s+equivalents\b`)

// dropText removes from n's or nodes the text alternatives drop matches.
func dropText(n *types.ReqNode, drop func(*types.ReqNode) bool) *types.ReqNode {
	if n == nil {
		return nil
	}
	for i, it := range n.Items {
		n.Items[i] = dropText(it, drop)
	}
	if n.Type == types.NodeOr {
		n.Items = slices.DeleteFunc(n.Items, func(it *types.ReqNode) bool { return it.Type == types.NodeText && drop(it) })
	}
	return n
}

// NodeKey is n as JSON, to tell whether a pass changed it.
func NodeKey(n *types.ReqNode) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// ---- Review markers again ----

// remark sets c's Review markers again after Link changed its bodies, taking
// markIncomplete's old markers off first.
func remark(c *types.Completion) {
	total := Total(c)
	for _, r := range c.Requirements {
		if !(total && r.Number == 1 || r.Kind == types.RequirementConstraint) {
			continue
		}
		if n := r.Req; n != nil && n.Type == types.NodeOr && len(n.Items) == 2 && IsMarker(n.Items[1], r) {
			r.Req = n.Items[0]
		}
	}
	markIncomplete(c, total, true)
}

// ---- The completion steps of Link, in order ----

// Link runs the completion steps of Link that read the catalogue or
// other programs. Each loop runs for every program before the next: inheritance
// copies rows the steps above finished. Changed programs are remarked.
func Link(courses []*types.Course, programs []*types.Program, titleIdx programs.TitleIndex, cat *programs.Catalog) {
	titles := map[string]string{}
	topics := map[string][]string{} // designator -> its special-topics courses
	for _, c := range courses {
		titles[c.Code] = c.Title
		if topicsTitleRe.MatchString(c.Title) && len(c.Code) == 8 && (c.Code[7] == '1' || c.Code[7] == '0') {
			topics[c.Code[:3]] = append(topics[c.Code[:3]], c.Code)
		}
	}
	for _, ds := range topics {
		slices.Sort(ds)
	}
	byCode := map[string]*types.Program{}
	var linked []*types.Program
	for _, p := range programs {
		if p.Completion != nil {
			byCode[p.Code] = p
			linked = append(linked, p)
		}
	}
	changed := map[*types.Program]bool{}
	for _, p := range linked {
		changed[p] = linkListCodes(p, linked, titles)
	}
	for _, p := range linked {
		linkCodeRanges(p.Completion, titles)
		ch := linkTopics(p.Completion, titles) || changed[p]
		ch = linkOtherTopics(p.Completion, topics) || ch
		ch = linkCompletionEquivalents(cat, p.Completion) || ch
		changed[p] = linkFocusPrograms(titleIdx, p.Completion, p.Title) || ch
	}
	for _, p := range linked {
		changed[p] = linkOtherProgramPool(p.Completion, p.Code, byCode, titleIdx) || changed[p]
		changed[p] = linkProgramList(p.Completion, p.Code, byCode, titleIdx) || changed[p]
	}
	for _, p := range linked {
		changed[p] = linkInheritance(p.Completion, p.Code, byCode, titleIdx) || changed[p]
	}
	for _, p := range linked {
		if checkRequirementRefs(p.Completion) || changed[p] {
			remark(p.Completion)
		}
	}
	for _, w := range slashWarnings(cat, linked) {
		log.Print(w)
	}
}

// slashWarnings returns a warning for each "/" cap whose courses don't all
// exclude each other in the catalogue, where the "/" may be a plain "or",
// then a count. Programs' order keeps the output stable.
func slashWarnings(cat *programs.Catalog, programs []*types.Program) []string {
	var out []string
	caps := 0
	for _, p := range programs {
		for _, r := range p.Completion.Requirements {
			n := r.Req
			if r.Kind != types.RequirementLimit || n == nil || len(n.Courses) < 2 || n.MaxAmount == nil || n.Designators != nil ||
				n.Levels != nil || n.Lists != nil || n.Breadth != nil || n.Transfer || n.CrNcr || n.Requirements != nil ||
				n.ExcludeDesignators != nil || n.MaxCourseAmount != 0 {
				continue
			}
			heaviest := 0.0
			for _, c := range n.Courses {
				heaviest = max(heaviest, grammar.CourseWeight(c))
			}
			if *n.MaxAmount != heaviest {
				continue
			}
			caps++
			if pair := cat.NotExclusive(n.Courses); pair != nil {
				out = append(out, fmt.Sprintf("completion %s: requirement %d caps %s at %s credit, but %s and %s don't exclude each other in the calendar",
					p.Code, r.Number, strings.Join(n.Courses, "/"), fmtAmount(heaviest), pair[0], pair[1]))
			}
		}
	}
	if caps > 0 {
		out = append(out, fmt.Sprintf("completion: %d of %d \"/\" caps name courses that don't exclude each other", len(out), caps))
	}
	return out
}

// ---- Program names ----

// linkFocusPrograms resolves by title the program nodes of rows under a heading
// of focuses that name a focus of self without its code. A name no program has
// stays membership text (Review).
func linkFocusPrograms(idx programs.TitleIndex, c *types.Completion, self string) bool {
	changed := false
	for _, r := range c.Requirements {
		if !blkFociRe.MatchString(r.Section) {
			continue
		}
		grammar.WalkReq(r.Req, func(n *types.ReqNode) {
			if n.Type != types.NodeProgram || len(n.Programs) > 0 {
				return
			}
			changed = true
			if code := idx.Exact("Focus in " + n.Text + " (" + programs.TitleSuffixRe.ReplaceAllString(self, "") + ")"); code != "" {
				n.Programs = []string{code}
				return
			}
			for _, l := range strings.Split(r.Raw, "\n") {
				if strings.HasPrefix(strings.TrimSpace(l), n.Text+" (") {
					n.Text = grammar.CleanNote(l)
					break
				}
			}
			n.Type, n.Kind = types.NodeText, types.TextMembership
		})
	}
	return changed
}

var (
	// managedRe matches "any other Human Biology Program managed major
	// program": the area's majors.
	managedRe = regexp.MustCompile(`(?i)^(any\s+other\s+)?(.+?)\s+program\s+managed\s+(.+)$`)
	// onlyOneRe matches "may enrol in only one Rotman Commerce Specialist":
	// the area's other specialists.
	onlyOneRe = regexp.MustCompile(`(?i)\bmay\s+(?:only\s+)?enrol(?:l)?\s+in\s+only\s+one\s+(.+?)\.?$`)
	// typeOfTypeRe matches "Specialist of Major", a misprint of "or".
	typeOfTypeRe = regexp.MustCompile(`(?i)\b(specialist|major|minor)\s+of\s+(specialist|major|minor)\b`)
	// typeInRe matches "the Major Program in Toxicology": the one Major
	// whose title names it.
	typeInRe = regexp.MustCompile(`(?i)^(?:the\s+|a\s+)?(specialist|major|minor)\s+program\s+in\s+(.+)$`)
)

// LinkExclusions resolves the program names in c's exclusions as
// enrolment does, plus completion's phrasings. It never excludes the program or
// one a requirement needs; programs a phrase may also cover are a Review item.
func LinkExclusions(idx programs.TitleIndex, c *types.Completion, self string, enrolReq *types.ReqNode, programList []*types.Program, progs *programs.ProgramIndex, areas programs.AreaIndex) {
	if c.Exclusions == nil {
		return
	}
	// a focus's parent is never excluded
	required := programs.RequiredPrograms(enrolReq)
	for _, r := range c.Requirements {
		for code := range programs.RequiredPrograms(r.Req) {
			required[code] = true
		}
	}
	selfArea := ""
	for _, p := range programList {
		if p.Code == self && len(p.Areas) > 0 {
			selfArea = p.Areas[0]
		}
	}
	concurrentExclusions(idx, c, self, selfArea, programList, progs, areas)
	for _, n := range c.Exclusions {
		phrase := n.Text
		if n.Type == types.NodeText {
			m := onlyOneRe.FindStringSubmatch(n.Text)
			if m == nil {
				continue
			}
			phrase = m[1]
		}
		if n.Type == types.NodeProgram && n.Programs != nil {
			continue
		}
		if codes, ok := completionProgram(idx, phrase, selfArea, programList, progs, areas); ok {
			n.Type, n.Kind, n.Programs, n.Text = types.NodeProgram, "", codes, phrase
		}
	}
	var notes []string
	c.Exclusions, notes = idx.ResolveExclusions(c.Exclusions, self, required, progs, areas)
	c.Notes = grammar.Dedupe(append(c.Notes, notes...))
	// the programs a phrase may also cover: or(not(program[...]), text), an item
	// of its own
	for _, s := range idx.SplitExclusions(c.Exclusions, self, programList, required, areas) {
		s.Node.Programs = s.Sure
		i := slices.Index(c.Exclusions, s.Node)
		c.Exclusions = slices.Insert(c.Exclusions, i+1, programs.AmbiguousExclusion(s))
	}
}

// completionProgram resolves a phrase naming programs as enrolment does, or by
// completion's phrasings.
func completionProgram(idx programs.TitleIndex, phrase, selfArea string, programList []*types.Program, progs *programs.ProgramIndex, areas programs.AreaIndex) ([]string, bool) {
	n := &types.ReqNode{Type: types.NodeProgram, Text: phrase}
	if idx.ResolveNode(n, progs, areas); n.Type == types.NodeProgram {
		return n.Programs, true
	}
	if fixed := typeOfTypeRe.ReplaceAllString(phrase, "$1 or $2"); fixed != phrase {
		// "of" misprinted for "or", when the programs exist
		if codes, ok := idx.LookupTitle(fixed); ok {
			return codes, true
		}
	}
	if m := managedRe.FindStringSubmatch(phrase); m != nil {
		return areas.Lookup(m[1] + m[3] + " offered by the Department of " + m[2])
	}
	if m := typeInRe.FindStringSubmatch(phrase); m != nil {
		var hits []string
		for _, p := range programList {
			if strings.EqualFold(p.Type, m[1]) && strings.Contains(" "+programs.NormSubject(p.Title)+" ", " "+programs.NormSubject(m[2])+" ") {
				hits = append(hits, p.Code)
			}
		}
		if len(hits) == 1 {
			return hits, true
		}
		// several: those of this program's area; SplitExclusions adds the others as
		// maybe
		var mine []string
		for _, code := range hits {
			for _, p := range programList {
				if p.Code == code && selfArea != "" && len(p.Areas) > 0 && p.Areas[0] == selfArea {
					mine = append(mine, code)
				}
			}
		}
		if mine != nil {
			return mine, true
		}
	}
	return nil, false
}

// concurrentExclusions reads "not allowed to enroll concurrently in A and B"
// for this program: the other program is the exclusion; when neither is this
// one, the sentence restricts others' students and is a note.
func concurrentExclusions(idx programs.TitleIndex, c *types.Completion, self, selfArea string, programList []*types.Program, progs *programs.ProgramIndex, areas programs.AreaIndex) {
	keep, drop := map[string]bool{}, map[string]bool{}
	selfType := ""
	for _, p := range programList {
		if p.Code == self {
			selfType = p.Type
		}
	}
	for _, line := range strings.Split(c.Raw, "\n") {
		for _, s := range SplitSentences(line) {
			t := ExclusionLeadRe.ReplaceAllString(programs.MarkerRe.ReplaceAllString(strings.TrimSuffix(strings.TrimSpace(s), "."), ""), "")
			m := ConcurrentRe.FindStringSubmatch(t)
			if m == nil {
				continue
			}
			sides := [2][]*types.ReqNode{programs.ExclusionPhraseNodes(m[1]), programs.ExclusionPhraseNodes(m[2])}
			var in [2]bool
			known := true
			for i, nodes := range sides {
				for _, n := range nodes {
					codes, ok := completionProgram(idx, n.Text, selfArea, programList, progs, areas)
					// a phrase naming only other program types isn't this one
					known = known && (ok || otherTypes(n.Text, selfType))
					in[i] = in[i] || slices.Contains(codes, self)
				}
			}
			switch {
			case in[0] || in[1]:
				for i, nodes := range sides {
					for _, n := range nodes {
						if in[1-i] {
							keep[n.Text] = true
						}
					}
				}
			case known:
				for _, nodes := range sides {
					for _, n := range nodes {
						drop[n.Text] = true
					}
				}
				c.Notes = grammar.Dedupe(append(c.Notes, grammar.CleanNote(t)))
			}
		}
	}
	c.Exclusions = slices.DeleteFunc(c.Exclusions, func(n *types.ReqNode) bool { return drop[n.Text] && !keep[n.Text] })
}

// otherTypes reports whether phrase names program types and none is t.
func otherTypes(phrase, t string) bool {
	ms := programs.AreaTypeRe.FindAllStringSubmatch(phrase, -1)
	return t != "" && ms != nil && !slices.ContainsFunc(ms, func(m []string) bool { return strings.EqualFold(m[1], t) })
}

// ---- References ----

// checkRequirementRefs makes a node referring to a row or list c doesn't have
// the text of its row (Review), and drops overlaps with missing or one-way
// rows. The parser never leaves these; this guards Link's changes.
func checkRequirementRefs(c *types.Completion) bool {
	byNum := map[int]*types.Requirement{}
	for _, r := range c.Requirements {
		byNum[r.Number] = r
	}
	changed := checkListRefs(c)
	known := map[string]bool{}
	for _, l := range c.Lists {
		known[l.Name] = true
	}
	for _, r := range c.Requirements {
		var missing, noList []string
		copied := false
		var fix func(n *types.ReqNode) *types.ReqNode
		fix = func(n *types.ReqNode) *types.ReqNode {
			if n == nil {
				return nil
			}
			for i, it := range n.Items {
				n.Items[i] = fix(it)
			}
			bad := false
			for _, k := range n.Requirements {
				switch {
				case k == noRow:
					copied, bad = true, true
				case byNum[k] == nil:
					missing, bad = append(missing, strconv.Itoa(k)), true
				}
			}
			for _, name := range n.Lists {
				if !known[name] {
					noList, bad = append(noList, strconv.Quote(name)), true
				}
			}
			if bad {
				return TextNode(types.TextOther, RawHead(r))
			}
			return n
		}
		r.Req = fix(r.Req)
		if missing != nil {
			r.Notes = append(r.Notes, "No requirement "+strings.Join(grammar.Dedupe(missing), ", ")+" in this program.")
			changed = true
		}
		if copied {
			r.Notes = append(r.Notes, "It refers to a requirement of the program it was copied from that this program doesn't have.")
			changed = true
		}
		if noList != nil {
			r.Notes = append(r.Notes, "No list "+strings.Join(grammar.Dedupe(noList), ", ")+" in this program.")
			changed = true
		}
		keep := slices.DeleteFunc(slices.Clone(r.Overlaps), func(o int) bool {
			q := byNum[o]
			return q == nil || !slices.Contains(q.Overlaps, r.Number)
		})
		if len(keep) != len(r.Overlaps) {
			if len(keep) == 0 {
				keep = nil
			}
			r.Overlaps = keep
			changed = true
		}
	}
	return changed
}

// checkListRefs drops from c's list items names of missing lists and numbers of
// missing rows, making the item a maybe and noting the name or number. It
// reports whether c changed.
func checkListRefs(c *types.Completion) bool {
	known := map[string]bool{}
	for _, l := range c.Lists {
		known[l.Name] = true
	}
	rows := map[int]bool{}
	for _, r := range c.Requirements {
		rows[r.Number] = true
	}
	var notes []string
	changed := false
	for _, l := range c.Lists {
		for i, it := range l.Items {
			filter := it
			if it.Type == types.NodeAnd && len(it.Items) > 0 {
				filter = it.Items[0]
			}
			var bad []string
			filter.Lists = slices.DeleteFunc(filter.Lists, func(name string) bool {
				if !known[name] {
					bad = append(bad, strconv.Quote(name))
					notes = append(notes, "No list "+strconv.Quote(name)+" in this program.")
					return true
				}
				return false
			})
			filter.Requirements = slices.DeleteFunc(filter.Requirements, func(k int) bool {
				switch {
				case k == noRow:
					bad = append(bad, "a requirement of the program it was copied from")
					notes = append(notes, "A list refers to a requirement of the program it was copied from that this program doesn't have.")
				case !rows[k]:
					bad = append(bad, "requirement "+strconv.Itoa(k))
					notes = append(notes, "No requirement "+strconv.Itoa(k)+" in this program.")
				default:
					return false
				}
				return true
			})
			if bad == nil {
				continue
			}
			changed = true
			if len(filter.Lists) == 0 {
				filter.Lists = nil
			}
			if len(filter.Requirements) == 0 {
				filter.Requirements = nil
			}
			if filter == it {
				l.Items[i] = programs.AndNode(it, TextNode(types.TextOther, "a course on "+strings.Join(bad, " or ")))
			}
		}
	}
	c.Notes = grammar.Dedupe(append(c.Notes, notes...))
	if len(c.Notes) == 0 {
		c.Notes = nil
	}
	return changed
}
