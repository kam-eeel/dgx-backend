package reqs

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Linking program completion (docs/schema.md, "How completion is produced",
// step 2): the steps that need the catalogue.

// linkCompletionDesignators keeps in completion filters only designators
// the catalogue has, or that the program's own text names as a course's
// prefix ("PCJ-coded courses" beside PCJ210H1): "CSC/ECE courses" counts
// CSC courses; "RLG, JAR, JCR, ..." drops JAR, which no course in the
// catalogue has, from the designators a pool counts or excludes. The row
// notes the designators it dropped. A pool left with
// no designator and no listed course would count any course, so it becomes
// text (Review), and the Review markers are set again on the counts it is
// part of (markIncomplete). A limit left with none keeps them: a limit must
// stay a credits node, and a cap on courses that don't exist caps nothing.
func linkCompletionDesignators(courses []*types.Course, programs []*types.Program) {
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
		for _, code := range codeRe.FindAllString(c.Raw, -1) {
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
					// Excluding a designator no course has excludes nothing.
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
					return textNode(types.TextOther, rawHead(r))
				}
				return n
			}
			r.Req = fix(r.Req)
			if dropped = dedupe(dropped); len(dropped) == 1 {
				r.Notes = append(r.Notes, "No course in the calendar has the designator "+dropped[0]+".")
			} else if dropped != nil {
				r.Notes = append(r.Notes, "No course in the calendar has the designators "+strings.Join(dropped, ", ")+".")
			}
		}
		if texted {
			markIncomplete(c, completionTotal(c), false)
		}
	}
}

// completionTotal reports whether row 1 is a header total: an allocation at
// the top with a credits body and child allocations.
func completionTotal(c *types.Completion) bool {
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
	// A code missing its campus digit: "CDN307H", "HIS283Y (.50)". Not
	// "CSC300H-level".
	complNoCampusRe = regexp.MustCompile(`\b([A-Z]{3}\d{3}[HY])\b(?:[^-+]|$)`)
	// A code with a two-letter designator: "HS324H1" (HIS324H1).
	complShortDesigRe = regexp.MustCompile(`\b([A-Z]{2})(\d{3}[HY]\d)\b`)
)

// repairCompletion repairs catalogue-confirmed codes in the raw text, as for
// requisites and enrolment, and parses it again, keeping Raw:
//   - a code without its campus digit is the St. George course when the
//     catalogue has it ("CDN307H" is CDN307H1; "BIO150Y" stays, as no
//     BIO150Y1 exists);
//   - a code without H/Y and campus digit is the one course that matches
//     ("ITA340," is ITA340H1; repairCatalogCodes);
//   - a two-letter designator is the catalogue code it is one letter short
//     of, when exactly one such code has a designator another code on the
//     same line has: "HS324H1" among HIS codes is HIS324H1 (HPS324H1 also
//     exists, but no HPS course is on that line).
//
// Uncatalogued codes ("CSB332H") and graduate codes ("GLA2050H1") are kept.
func repairCompletion(c *types.Completion, known map[string]bool) *types.Completion {
	if c == nil {
		return nil
	}
	fixed, ok := repairCompletionCodes(c.Raw, known)
	if !ok {
		return c
	}
	r := ParseCompletion(fixed)
	r.Raw = c.Raw
	return r
}

// repairCompletionCodes returns s with the repairs repairCompletion makes,
// and whether there were any.
func repairCompletionCodes(s string, known map[string]bool) (string, bool) {
	changed := false
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		var b strings.Builder
		last := 0
		for _, m := range complNoCampusRe.FindAllStringSubmatchIndex(line, -1) {
			if code := line[m[2]:m[3]] + "1"; known[code] {
				b.WriteString(line[last:m[2]] + code)
				last = m[3]
			}
		}
		if last > 0 {
			line = b.String() + line[last:]
			changed = true
		}
		if fixed, ok := repairCatalogCodes(line, known); ok {
			line, changed = fixed, true
		}
		line = complShortDesigRe.ReplaceAllStringFunc(line, func(w string) string {
			onLine := map[string]bool{}
			for _, code := range codeRe.FindAllString(line, -1) {
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

// linkCodeRanges keeps, of the codes a range in a row's raw spans
// (codeRange: "( SOC317H1 – SOC354H1; ...)"), those the catalogue has and
// the ends, in each pool that holds the whole span. The Review markers
// don't change (remark would take a cap's own permission sentence, the
// row's raw, for one).
func linkCodeRanges(c *types.Completion, titles map[string]string) {
	for _, m := range complCodeRangeRe.FindAllStringSubmatch(completionRaws(c), -1) {
		span, ends := codeRange(m[1], m[2]), m[1:3]
		if span == nil {
			continue
		}
		for _, r := range c.Requirements {
			walkReq(r.Req, func(n *types.ReqNode) {
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

// completionRaws returns the raw of c's rows, one distinct raw a line.
func completionRaws(c *types.Completion) string {
	var raws []string
	for _, r := range c.Requirements {
		if !slices.Contains(raws, r.Raw) {
			raws = append(raws, r.Raw)
		}
	}
	return strings.Join(raws, "\n")
}

// complTopicsTitleRe is a catalogue title of a special-topics course, whose
// content changes with each offering: "Topics in History", "Special Topics
// in Anthropology", "Selected Topics in International Studies", "Advanced
// Topics in Canadian History". "Research Topic in Astronomy" isn't one.
var complTopicsTitleRe = regexp.MustCompile(`(?i)^(?:(?:advanced|selected|special|open|current)\s+)*topics\b`)

// linkTopics makes a special-topics course listed with one topic a "maybe"
// (design 3.2), as the parser does for a course listed with two or more
// (topicCodes): "HIS389H1 (Hungarian History)", where the catalogue titles
// HIS389H1 "Topics in History", counts only in an offering on that topic,
// which the transcript doesn't show. A pool becomes or(the pool without
// it, and(the pool, text)); a required course, and(course, text). Only the
// catalogue tells a topic from a description: "EEB365H1 (applied
// conservation biology)" restates its title, "Topics in Applied
// Conservation Biology", and stays a course. A pool that can't make its
// amount without the course is left alone. It reports whether a row
// changed.
//
// A special-topics course on a named list whose membership depends on the
// topic is the parser's: "ANT390H1 may not be available in Course Group B,
// depending on the topic" (ASMAJ1510) makes it the list's maybe item
// and(credits{courses: [it]}, text) (namedLists, topicMember). ASSPE0652's
// "HIS389H1*****" with "Some “Topics in History” ... Various topics each
// year" is the parser's too: a maybe item of the Methodology Courses list
// (starredTopics).
func linkTopics(c *types.Completion, titles map[string]string) bool {
	changed := false
	for _, r := range c.Requirements {
		// A limit stays a credits node: a course it caps is capped whatever
		// its topic.
		if r.Req == nil || r.Kind == types.RequirementLimit {
			continue
		}
		mentions := map[string][]string{}
		for _, m := range complTopicParenRe.FindAllStringSubmatch(r.Raw, -1) {
			if !slices.Contains(mentions[m[1]], m[0]) {
				mentions[m[1]] = append(mentions[m[1]], m[0])
			}
		}
		topics := map[string][]string{}
		for code, ms := range mentions {
			topic := strings.TrimSpace(complTopicParenRe.FindStringSubmatch(ms[0])[2])
			title := titles[code]
			// Two or more topics are the parser's (topicCodes); a title that
			// isn't a topics course's, or a bracket that isn't a topic, leaves
			// the course as it is.
			if len(ms) != 1 || !complTopicsTitleRe.MatchString(title) || !isTopic(topic, title) {
				continue
			}
			if !slices.Contains(nodeCodes(r.Req), code) || !poolsWithout(r.Req, code) {
				continue
			}
			topics[code] = ms
		}
		if len(topics) == 0 {
			continue
		}
		r.Req = tidy(topicMembers(r.Req, topics))
		changed = true
	}
	return changed
}

// isTopic reports whether a bracket after a special-topics course names a
// topic: words starting with a capital ("Hungarian History"), not the
// course's own title or the part after "Topics in" ("Topics in Critical
// Zone Processes", "applied conservation biology").
func isTopic(topic, title string) bool {
	if topic == "" || !complTitleWordsRe.MatchString(topic) || topic[0] < 'A' || topic[0] > 'Z' {
		return false
	}
	t, n := normSubject(title), normSubject(topic)
	return !strings.Contains(" "+t+" ", " "+n+" ")
}

// poolsWithout reports whether every pool in n that lists code can still
// make its amount without it.
func poolsWithout(n *types.ReqNode, code string) bool {
	ok := true
	walkReq(n, func(p *types.ReqNode) {
		if p.Type != types.NodeCredits || !slices.Contains(p.Courses, code) || p.Designators != nil || p.Levels != nil || p.Breadth != nil {
			return
		}
		sum := 0.0
		for _, c := range dedupe(p.Courses) {
			if c != code {
				sum += courseWeight(c)
			}
		}
		if sum < p.Amount-1e-9 {
			ok = false
		}
	})
	return ok
}

// complOtherTopicsRe: "Some offerings of other POL Special Topics courses
// (that cover Canadian politics) may also be eligible as determined by the
// department" (ASMAJ2015), a permission route on a plain pool.
var complOtherTopicsRe = regexp.MustCompile(`(?i)\bother\s+([A-Z]{3})\s+(?:special\s+)?topics\s+courses\b.*\bmay\s+also\s+be\s+eligible\b`)

// complTopicsRouteRe: a permission route the parser bounded by its
// designator (boundRoutes) that names only that designator's special-topics
// courses: "Some offerings of other POL Special Topics courses ...", "any
// 400-level finance-themed RSM special topics course as determined by the
// Director", "any accounting themed RSM special topics course ...". Not
// "any marketing-themed RSM special topics and RSM Summer Abroad courses":
// the catalogue doesn't mark summer-abroad courses, so that route keeps the
// designator bound.
var complTopicsRouteRe = regexp.MustCompile(`(?i)\b((?-i:[A-Z]{3}))\s+(?:special\s+)?topics\s+courses?\b`)

// linkOtherTopics bounds a permission that names a designator's
// special-topics courses (design 3.3, its own example) by the courses the
// catalogue titles "Topics in …" (complTopicsTitleRe), which the parser
// can't see:
//   - or(pool, text{permission}) becomes or(pool, and(the pool plus the
//     designator's special-topics courses, text));
//   - the parser's designator bound (boundRoutes) or(pool, and(credits{DES,
//     levels, the pool's courses}, text)) narrows to and(the pool plus DES's
//     special-topics courses at those levels, text).
//
// A student with none of them is NotMet, not Review. It reports whether a
// row changed.
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
				m := complOtherTopicsRe.FindStringSubmatch(it.Text)
				if m == nil || byDesignator[m[1]] == nil {
					continue
				}
				wider = cloneNode(pool)
				wider.Courses = dedupe(append(wider.Courses, byDesignator[m[1]]...))
				it = andNode(wider, it)
			case it.Type == types.NodeAnd && len(it.Items) == 2 && it.Items[1].Type == types.NodeText && it.Items[1].Kind == types.TextPermission:
				w, text := it.Items[0], it.Items[1]
				m := complTopicsRouteRe.FindStringSubmatch(text.Text)
				if m == nil || w.Type != types.NodeCredits || !slices.Equal(w.Designators, []string{m[1]}) || w.Amount != pool.Amount ||
					w.Breadth != nil || w.Exclude != nil || w.MaxAmount != nil || !slices.Equal(w.Courses, pool.Courses) {
					continue
				}
				var extra []string
				for _, code := range byDesignator[m[1]] {
					if levelMatches(w.Levels, code) {
						extra = append(extra, code)
					}
				}
				if extra == nil {
					continue
				}
				wider = cloneNode(pool)
				wider.Courses = dedupe(append(wider.Courses, extra...))
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

// linkListCodes checks the codes p's named lists hold against the catalogue
// (titles). A code whose designator no course has ("GGE372H1" in
// ASMIN1252's Group A) is a typo when exactly one catalogued code is one
// character away (GGR372H1), this list doesn't already hold it, and a
// program of p's area lists it on a list of the same name (the Major's
// Group A has GGR372H1): the member becomes that code, as in code repair,
// in the rows' pools too that copied the list's codes (a bounded route's
// wider pool), and the program notes the reading (the raw keeps the typo).
// Otherwise the code stays (an old or another faculty's course: "ECE419H1",
// "HST211H1") and the program notes that the calendar doesn't have it. A
// code with a known designator that the catalogue lacks (a retired course)
// is left alone. It reports whether p changed.
func linkListCodes(p *types.Program, programs []*types.Program, titles map[string]string) bool {
	c := p.Completion
	if c.Lists == nil {
		return false
	}
	designators := map[string]bool{}
	for code := range titles {
		designators[code[:3]] = true
	}
	sibling := func(name, code string) bool {
		for _, q := range programs {
			if q == p || q.Completion == nil || !slices.ContainsFunc(q.Areas, func(x string) bool { return slices.Contains(p.Areas, x) }) {
				continue
			}
			for _, l := range q.Completion.Lists {
				if l.Name == name && slices.ContainsFunc(l.Items, func(it *types.ReqNode) bool { return slices.Contains(nodeCodes(it), code) }) {
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
			held = append(held, nodeCodes(it)...)
		}
		for _, it := range l.Items {
			walkReq(it, func(n *types.ReqNode) {
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
						c.Notes = dedupe(append(c.Notes, "No course in the calendar has the code "+code+" ("+l.Name+"): read as "+near[0]+", which a program of the same area lists on its "+l.Name+"."))
					} else {
						c.Notes = dedupe(append(c.Notes, "No course in the calendar has the code "+code+" ("+l.Name+")."))
					}
					changed = true
				}
			})
		}
	}
	for _, r := range c.Requirements {
		walkReq(r.Req, func(n *types.ReqNode) {
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
// does (enrolCatalog.linkEquivalents):
//   - "X or equivalent" is X or one of the courses X excludes
//     (linkEquivalent);
//   - "Successful completion of the following four courses (or their
//     equivalents):" gives each course of the rows after it its exclusions
//     as alternatives (withEquivalents), and joins their raw instead of
//     being a note; a text alternative saying a designator's "course
//     equivalents" count ("... would be able to use any completed RSM course
//     equivalents ...") is then encoded, and goes (the row's raw holds it);
//   - "We will accept equivalent/exclusion statistics courses from UTM/UTSC
//     in lieu of PSY201H1. Please contact us to verify ...", an
//     alternative on the row requiring PSY201H1, becomes PSY201H1's UTM and
//     UTSC exclusions, which the department confirms: or(PSY201H1,
//     and(or(PSY201H5, ...), text{permission})). The sentence's text
//     alternative goes when every course it names in the row got some;
//     otherwise it stays.
//
// It reports whether a row changed.
func (cat *enrolCatalog) linkCompletionEquivalents(c *types.Completion) bool {
	changed, applied := false, false
	theirs := theirEquivalentsRe.FindStringIndex(c.Raw)
	for _, r := range c.Requirements {
		if r.Req == nil {
			continue
		}
		before := complNodeKey(r.Req)
		r.Req = cat.linkEquivalent(r.Req)
		if theirs != nil && r.Kind == types.RequirementAllocation {
			if at := strings.Index(c.Raw, rawHead(r)); at > theirs[1] {
				r.Req = cat.withEquivalents(r.Req, nil, nil, false)
				applied = true
				// The lead is the sentence the row's alternatives come from.
				lead := c.Raw[strings.LastIndex(c.Raw[:theirs[0]], "\n")+1:]
				lead, _, _ = strings.Cut(lead, "\n")
				r.Raw += "\n" + strings.TrimSpace(lead)
				// "... would be able to use any completed RSM course
				// equivalents ..." says what the equivalents now encode: not
				// a text alternative (Review), and the row's raw holds it.
				codes := nodeCodes(r.Req)
				r.Req = tidy(dropText(r.Req, func(t *types.ReqNode) bool {
					m := complDesigEquivalentsRe.FindStringSubmatch(t.Text)
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
		for _, m := range campusEquivRe.FindAllStringSubmatch(r.Raw, -1) {
			var codes []string
			for _, code := range codeRe.FindAllString(m[1], -1) {
				if slices.Contains(nodeCodes(r.Req), code) {
					codes = append(codes, code)
				}
			}
			if codes == nil {
				continue
			}
			with, all := cloneNode(r.Req), true
			for _, code := range codes {
				n := len(nodeCodes(with))
				with = cat.withEquivalents(with, []string{code}, otherCampusRe.MatchString, true)
				all = all && len(nodeCodes(with)) > n
				if courseWeight(code) == 0.5 {
					halfEquivalents(with)
					cat.halfSubstitutes(with, code)
				}
			}
			if !all {
				continue
			}
			r.Req = tidy(dropText(with, func(t *types.ReqNode) bool {
				return t.Kind == types.TextPermission && campusEquivRe.MatchString(t.Text)
			}))
		}
		if complNodeKey(r.Req) != before {
			changed = true
		}
	}
	if applied {
		// The lead is encoded now: not a note any more.
		c.Notes = slices.DeleteFunc(c.Notes, theirEquivalentsRe.MatchString)
		if len(c.Notes) == 0 {
			c.Notes = nil
		}
	}
	return changed
}

// halfEquivalents makes a full-year course among the confirmed equivalents
// of a half course (ECO220Y5 for PSY201H1) count 0.5 of its weight, as the
// parser reads a Y course that stands for an H course (credits{0.5,
// [ECO220Y1], weight 0.5}): the rest of it can count elsewhere.
func halfEquivalents(n *types.ReqNode) {
	walkReq(n, func(a *types.ReqNode) {
		if a.Type != types.NodeAnd || !slices.ContainsFunc(a.Items, func(t *types.ReqNode) bool {
			return t.Type == types.NodeText && t.Text == equivalentConfirmText
		}) {
			return
		}
		half := func(c *types.ReqNode) *types.ReqNode {
			if c.Type == types.NodeCourse && courseWeight(c.Code) == 1 {
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

// halfSubstitutes gives the full-year courses beside the half course code
// in an or that code's catalogue exclusions name a weight of 0.5, as
// halfEquivalents gives its campus equivalents: in "PSY201H1 (or one of
// ECO220Y1/ EEB225H1/ ...)", where ECO220Y5 stands in for PSY201H1 as 0.5
// credit, so does ECO220Y1 (ASMAJ1160, ASMIN1160; the parser already splits
// it where it stands in for two half courses, splitYAlternatives). The rest
// of the course can count elsewhere (design 2, weight).
func (cat *enrolCatalog) halfSubstitutes(n *types.ReqNode, code string) {
	excl := cat.exclusions[code]
	if excl == nil {
		return
	}
	excluded := map[string]bool{}
	for _, it := range excl.Items {
		if len(it.Courses) == 1 && !it.Concurrent && it.Note == "" {
			excluded[it.Courses[0]] = true
		}
	}
	walkReq(n, func(o *types.ReqNode) {
		if o.Type != types.NodeOr || !slices.ContainsFunc(o.Items, func(it *types.ReqNode) bool { return it.Type == types.NodeCourse && it.Code == code }) {
			return
		}
		for i, it := range o.Items {
			if it.Type == types.NodeCourse && courseWeight(it.Code) == 1 && excluded[it.Code] && it.MinGrade == 0 {
				o.Items[i] = &types.ReqNode{Type: types.NodeCredits, Amount: 0.5, Courses: []string{it.Code}, Weight: 0.5}
			}
		}
	})
}

// complDesigEquivalentsRe: "Students who move from the Rotman Commerce
// program (BCOM) ... would be able to use any completed RSM course
// equivalents towards the completion of the Certificate" (ASCER2400).
var complDesigEquivalentsRe = regexp.MustCompile(`(?i)\bwould\s+be\s+able\s+to\s+use\s+any\s+completed\s+([A-Z]{3})\s+course\s+equivalents\b`)

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

// complNodeKey is n as JSON, to tell whether a pass changed it.
func complNodeKey(n *types.ReqNode) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// ---- Review markers again ----

// remark sets the Review markers of c again after Link changed its bodies
// (markIncomplete): a text alternative Link encoded (a bounded "maybe", a
// copied row) may leave a count it marked complete, and a pool it turned
// into text leaves one incomplete. It takes the markers off first: the
// header text on the total, and the raw text on a constraint, each the
// second item of an or (markIncomplete's mark).
func remark(c *types.Completion) {
	total := completionTotal(c)
	for _, r := range c.Requirements {
		if !(total && r.Number == 1 || r.Kind == types.RequirementConstraint) {
			continue
		}
		if n := r.Req; n != nil && n.Type == types.NodeOr && len(n.Items) == 2 && isMarker(n.Items[1], r) {
			r.Req = n.Items[0]
		}
	}
	markIncomplete(c, total, true)
}

// ---- The completion steps of Link, in order ----

// linkCompletions runs the completion steps of Link that read the catalogue
// or the other programs, after code repair (repairCompletion) and the
// designator check (linkCompletionDesignators):
//  1. single-topic special-topics courses (linkTopics) and a permission over
//     a designator's special-topics courses (linkOtherTopics, which also
//     narrows the parser's designator bound, boundRoutes);
//  2. equivalents (linkCompletionEquivalents), and the focuses a row names
//     without their code, by title (linkFocusPrograms);
//
// then, each step for every program before the next:
//  3. a pool widened by another program's (linkOtherProgramPool), and a
//     pool of another program's list (linkProgramList);
//  4. inheritance (linkInheritance), copying rows the steps above finished;
//  5. requirement, list and overlap references (checkRequirementRefs);
//
// and sets the Review markers again on the programs it changed (remark).
// Link resolves program names in the exclusions (linkCompletionExclusions)
// after the enrolment step, which tells it the programs enrolment requires.
func linkCompletions(courses []*types.Course, programs []*types.Program, titleIdx titleIndex, progs *ProgramIndex, areas areaIndex, cat *enrolCatalog) {
	titles := map[string]string{}
	topics := map[string][]string{} // designator -> its special-topics courses
	for _, c := range courses {
		titles[c.Code] = c.Title
		if complTopicsTitleRe.MatchString(c.Title) && len(c.Code) == 8 && (c.Code[7] == '1' || c.Code[7] == '0') {
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
		ch = cat.linkCompletionEquivalents(p.Completion) || ch
		changed[p] = titleIdx.linkFocusPrograms(p.Completion, p.Title) || ch
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
	for _, w := range cat.slashWarnings(linked) {
		log.Print(w)
	}
}

// slashWarnings returns a warning for each "/" cap whose courses don't all
// exclude each other in the catalogue, then one line with the count: the
// parser caps a slash group in a pool at its larger weight ("CSC438H1/
// MAT309H1": only one counts), which a pair of exclusions confirms; where
// the catalogue doesn't, the "/" may be a plain "or" and the cap is worth a
// look. A "/" cap is a limit with only courses (two or more) and
// max_amount their larger weight. The lines follow the programs' order, so
// two runs print the same.
func (cat *enrolCatalog) slashWarnings(programs []*types.Program) []string {
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
				heaviest = max(heaviest, courseWeight(c))
			}
			if *n.MaxAmount != heaviest {
				continue
			}
			caps++
			if pair := cat.notExclusive(n.Courses); pair != nil {
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

// notExclusive returns the first two of codes neither of which lists the
// other as an exclusion, or nil when every two exclude each other.
func (cat *enrolCatalog) notExclusive(codes []string) []string {
	excludes := func(a, b string) bool {
		e := cat.exclusions[a]
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

// ---- Program names ----

// linkFocusPrograms resolves, by title, the program nodes of c's rows that
// name a focus of the program (self, its title) without its code
// (focusChoices: "Immunology" is "Focus in Immunology (Specialist in
// Statistical Science: Methods and Practice)"). Only the rows under a
// heading of focuses are read. A name no program has stays membership text
// (Review): the focus's line in the row's raw ("Immunology (3.0 Credits) -
// BIO130H1 (70%), ..."), else its name.
func (idx titleIndex) linkFocusPrograms(c *types.Completion, self string) bool {
	changed := false
	for _, r := range c.Requirements {
		if !blkFociRe.MatchString(r.Section) {
			continue
		}
		walkReq(r.Req, func(n *types.ReqNode) {
			if n.Type != types.NodeProgram || len(n.Programs) > 0 {
				return
			}
			changed = true
			if code := idx.exact("Focus in " + n.Text + " (" + titleSuffixRe.ReplaceAllString(self, "") + ")"); code != "" {
				n.Programs = []string{code}
				return
			}
			for _, l := range strings.Split(r.Raw, "\n") {
				if strings.HasPrefix(strings.TrimSpace(l), n.Text+" (") {
					n.Text = cleanNote(l)
					break
				}
			}
			n.Type, n.Kind = types.NodeText, types.TextMembership
		})
	}
	return changed
}

var (
	// "any other Human Biology Program managed major program": the majors
	// of the Human Biology area.
	complManagedRe = regexp.MustCompile(`(?i)^(any\s+other\s+)?(.+?)\s+program\s+managed\s+(.+)$`)
	// "Students may enrol in only one Rotman Commerce Specialist": the
	// area's other specialists.
	complOnlyOneRe = regexp.MustCompile(`(?i)\bmay\s+(?:only\s+)?enrol(?:l)?\s+in\s+only\s+one\s+(.+?)\.?$`)
	// "Specialist of Major": a misprint of "Specialist or Major".
	complTypeOfTypeRe = regexp.MustCompile(`(?i)\b(specialist|major|minor)\s+of\s+(specialist|major|minor)\b`)
	// "the Major Program in Toxicology": the one Major whose title names
	// Toxicology (the Biomedical Toxicology Major).
	complTypeInRe = regexp.MustCompile(`(?i)^(?:the\s+|a\s+)?(specialist|major|minor)\s+program\s+in\s+(.+)$`)
)

// linkCompletionExclusions resolves the program names in c's exclusions as
// enrolment does (resolveExclusions): by title, by subject and type, by
// subject area. Completion text also has:
//   - "Students are not allowed to enroll concurrently in A and B", written
//     in both programs' text and in programs that are neither: it excludes
//     B from A and A from B, and is a note elsewhere (the Biomedical
//     Toxicology Major's "... the Major Program in Pharmacology and a
//     Specialist Program in Toxicology");
//   - "any other X Program managed major program": X's area's majors;
//   - "Students may enrol in only one Rotman Commerce Specialist": the
//     area's other specialists;
//   - "the Major Program in Toxicology": the one Major whose title names
//     Toxicology. When several do ("a Specialist Program in Toxicology":
//     three Specialists), it is those of this program's area, and the
//     others are the maybe below;
//   - "German Studies Specialist of Major Program": "of" misprinted for
//     "or", when both programs exist.
//
// They never include the program itself or a program a requirement needs,
// in completion or to enrol (enrolReq: a focus's parent); a node left with
// no program goes. A name that resolves to nothing stays membership text
// (Review). A subject phrase that may also cover other programs (enrolment's
// rule, splitExclusions: those outside the subject's area whose title names
// it, and the area's programs whose title doesn't; "any other Economics
// program": the Finance and Economics Specialist?) keeps the programs it
// surely names, and the others are an item of their own after it, as in
// enrolment's req: or(not(program[...]), text{ambiguity}), Review only for
// a student in one of them.
func (idx titleIndex) linkCompletionExclusions(c *types.Completion, self string, enrolReq *types.ReqNode, programs []*types.Program, progs *ProgramIndex, areas areaIndex) {
	if c.Exclusions == nil {
		return
	}
	// Programs a requirement needs, in completion or to enrol (a focus's
	// parent), are never excluded.
	required := requiredPrograms(enrolReq)
	for _, r := range c.Requirements {
		for code := range requiredPrograms(r.Req) {
			required[code] = true
		}
	}
	selfArea := ""
	for _, p := range programs {
		if p.Code == self && len(p.Areas) > 0 {
			selfArea = p.Areas[0]
		}
	}
	idx.concurrentExclusions(c, self, selfArea, programs, progs, areas)
	for _, n := range c.Exclusions {
		phrase := n.Text
		if n.Type == types.NodeText {
			m := complOnlyOneRe.FindStringSubmatch(n.Text)
			if m == nil {
				continue
			}
			phrase = m[1]
		}
		if n.Type == types.NodeProgram && n.Programs != nil {
			continue
		}
		if codes, ok := idx.completionProgram(phrase, selfArea, programs, progs, areas); ok {
			n.Type, n.Kind, n.Programs, n.Text = types.NodeProgram, "", codes, phrase
		}
	}
	var notes []string
	c.Exclusions, notes = idx.resolveExclusions(c.Exclusions, self, required, progs, areas)
	c.Notes = dedupe(append(c.Notes, notes...))
	// The programs a subject phrase may also cover are enrolment's
	// or(not(program[...]), text{ambiguity}) (ambiguousExclusion), an item
	// of its own after the phrase's: Review only for a student in one of
	// them.
	for _, s := range idx.splitExclusions(c.Exclusions, self, programs, required, areas) {
		s.node.Programs = s.sure
		i := slices.Index(c.Exclusions, s.node)
		c.Exclusions = slices.Insert(c.Exclusions, i+1, ambiguousExclusion(s))
	}
}

// completionProgram resolves a phrase naming programs: as enrolment does
// (resolveNode), or by the completion phrasings linkCompletionExclusions
// lists.
func (idx titleIndex) completionProgram(phrase, selfArea string, programs []*types.Program, progs *ProgramIndex, areas areaIndex) ([]string, bool) {
	n := &types.ReqNode{Type: types.NodeProgram, Text: phrase}
	if idx.resolveNode(n, progs, areas); n.Type == types.NodeProgram {
		return n.Programs, true
	}
	if fixed := complTypeOfTypeRe.ReplaceAllString(phrase, "$1 or $2"); fixed != phrase {
		// "German Studies Specialist of Major Program": "of" for "or" when
		// the programs exist.
		if codes, ok := idx.lookupTitle(fixed); ok {
			return codes, true
		}
	}
	if m := complManagedRe.FindStringSubmatch(phrase); m != nil {
		return areas.lookup(m[1] + m[3] + " offered by the Department of " + m[2])
	}
	if m := complTypeInRe.FindStringSubmatch(phrase); m != nil {
		var hits []string
		for _, p := range programs {
			if strings.EqualFold(p.Type, m[1]) && strings.Contains(" "+normSubject(p.Title)+" ", " "+normSubject(m[2])+" ") {
				hits = append(hits, p.Code)
			}
		}
		if len(hits) == 1 {
			return hits, true
		}
		// Several: those of this program's area ("a Specialist Program in
		// Toxicology" in the Pharmacology Major: the Biomedical Toxicology
		// Specialist and the Specialist in Pharmacology and Biomedical
		// Toxicology); splitExclusions adds the others (the Environment &
		// Toxicology Specialist) as maybe, as enrolment does.
		var mine []string
		for _, code := range hits {
			for _, p := range programs {
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

// concurrentExclusions reads "Students are not allowed to enroll
// concurrently in A and B" (complConcurrentRe) for this program: the
// parser excluded both A and B; when one is this program, the other is the
// exclusion, and when neither is, the sentence restricts other programs'
// students, so its exclusions go and it is a note. A phrase that doesn't
// resolve leaves the sentence as the parser read it.
func (idx titleIndex) concurrentExclusions(c *types.Completion, self, selfArea string, programs []*types.Program, progs *ProgramIndex, areas areaIndex) {
	keep, drop := map[string]bool{}, map[string]bool{}
	selfType := ""
	for _, p := range programs {
		if p.Code == self {
			selfType = p.Type
		}
	}
	for _, line := range strings.Split(c.Raw, "\n") {
		for _, s := range splitComplSentences(line) {
			t := complExclusionLeadRe.ReplaceAllString(complMarkerRe.ReplaceAllString(strings.TrimSuffix(strings.TrimSpace(s), "."), ""), "")
			m := complConcurrentRe.FindStringSubmatch(t)
			if m == nil {
				continue
			}
			sides := [2][]*types.ReqNode{exclusionPhraseNodes(m[1]), exclusionPhraseNodes(m[2])}
			var in [2]bool
			known := true
			for i, nodes := range sides {
				for _, n := range nodes {
					codes, ok := idx.completionProgram(n.Text, selfArea, programs, progs, areas)
					// A phrase naming only other types of program isn't this
					// one ("a Specialist Program in Toxicology" in a Major).
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
				c.Notes = dedupe(append(c.Notes, cleanNote(t)))
			}
		}
	}
	c.Exclusions = slices.DeleteFunc(c.Exclusions, func(n *types.ReqNode) bool { return drop[n.Text] && !keep[n.Text] })
}

// otherTypes reports whether phrase names program types and none is t.
func otherTypes(phrase, t string) bool {
	ms := areaTypeRe.FindAllStringSubmatch(phrase, -1)
	return t != "" && ms != nil && !slices.ContainsFunc(ms, func(m []string) bool { return strings.EqualFold(m[1], t) })
}

// ---- References ----

// checkRequirementRefs makes a node whose requirements filter names a row c
// doesn't have, or whose lists filter names a list c doesn't have, the text
// of its row (Review): a reference to nothing would match no course, and
// dropping it would match any. The row notes the number ("No requirement 9
// in this program."), or, for complNoRow, that it refers to a requirement
// of the program it was copied from, or the list ("No list \"Group D\" in
// this program."). A list item naming a list c doesn't have loses the name
// and becomes a maybe, and(the rest of its filter, text), noted on the
// program. An overlap with a row c doesn't have, or that the other row
// doesn't list back, goes: overlaps only join two rows that exist. The
// parser never leaves such a reference (the corpus invariants check it);
// this is the guard for what Link changes. It reports whether c changed.
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
				case k == complNoRow:
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
				return textNode(types.TextOther, rawHead(r))
			}
			return n
		}
		r.Req = fix(r.Req)
		if missing != nil {
			r.Notes = append(r.Notes, "No requirement "+strings.Join(dedupe(missing), ", ")+" in this program.")
			changed = true
		}
		if copied {
			r.Notes = append(r.Notes, "It refers to a requirement of the program it was copied from that this program doesn't have.")
			changed = true
		}
		if noList != nil {
			r.Notes = append(r.Notes, "No list "+strings.Join(dedupe(noList), ", ")+" in this program.")
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

// checkListRefs checks the references in c's list items: a name of a list
// c doesn't have, or a requirements number that names no row (complNoRow,
// or a number c doesn't have), goes from the item, which becomes and(the
// rest of its filter, text), a maybe (an item with no filter left matches
// any course, as a maybe only), and the program notes the name or number.
// It reports whether c changed.
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
				case k == complNoRow:
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
				l.Items[i] = andNode(it, textNode(types.TextOther, "a course on "+strings.Join(bad, " or ")))
			}
		}
	}
	c.Notes = dedupe(append(c.Notes, notes...))
	if len(c.Notes) == 0 {
		c.Notes = nil
	}
	return changed
}
