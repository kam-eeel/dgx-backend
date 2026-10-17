package reqs

import (
	"regexp"
	"slices"
	"strings"

	"degree-explorer/types"
)

// Routes: sentences that read as advice ("Students who ...") but change what
// meets the requirement. Each is applied to the tree and kept in notes.

type routeKind int

const (
	routeNote       routeKind = iota // already applied to the text; only the note is left
	routeAlt                         // or(field, node)
	routeSubstitute                  // codes[i] -> or(codes[i], and(cond, subs[i]))
	routeExtra                       // codes[0] -> and(codes[0], node)
	routeConcurrent                  // codes may be in progress
)

type route struct {
	kind  routeKind
	note  string
	node  *types.ReqNode   // alt, extra: what the route needs
	cond  *types.ReqNode   // substitute: who may substitute
	subs  []*types.ReqNode // substitute: the replacement for each of codes
	codes []string         // the courses the route attaches to
}

var (
	// routeAltRe matches another way to meet the whole field: "Students with A and
	// who are enrolled in P may take this course with Permission of the Instructor".
	routeAltRe = regexp.MustCompile(`(?i)^(?:notes?:\s*)?students\s+(?:with|who\s+have(?:\s+completed)?)\s+(.+?),?\s+may\s+(?:take|enrol(?:l)?\s+in)\s+this\s+course(?:\s+with\s+(.+))?$`)
	// routeFallbackRe matches "Students without these prerequisites may enrol in a
	// 200-level course if they are concurrently enrolled in ENG110Y1".
	routeFallbackRe = regexp.MustCompile(`(?i)^students\s+without\s+(?:these|the|this)\s+prerequisites?\s+may\s+(?:enrol(?:l)?|take)\b.*?\bif\s+they\s+(?:are\s+(concurrently\s+)?enrol(?:l)?ed\s+in|have\s+completed)\s+(.+)$`)
	// routeSubstituteRe matches a substitute for one course: "students enrolled in
	// P who have completed at least 9.0 credits may substitute A for CSC263H1".
	routeSubstituteRe = regexp.MustCompile(`(?i)^(?:notes?:\s*)?students\s+(.+?)\s+may\s+substitute\s+(.+)$`)
	substitutePairRe  = regexp.MustCompile(`(?:^|\s+and\s+)(.+?)\s+for\s+(` + codePat + `)`)
	// routeExtraRe matches more needed on one path: "Students who have completed
	// CSC240H1 must enrol in MAT377H1/ STA237H1 concurrently with CSC265H1".
	routeExtraRe = regexp.MustCompile(`(?i)^(?:notes?:\s*)?students\s+who\s+have\s+completed\s+(` + codePat + `)\s+must\s+enrol(?:l)?\s+in\s+(.+?)\s+concurrently(?:\s+with\s+` + codePat + `)?` +
		`(?:,?\s+if\s+they\s+have\s+not\s+already\s+completed\s+(?:one\s+of\s+)?(?:those|these|that|this|it)(?:\s+courses?)?)?$`)
	// acceptableRe matches "(Note: BCH210H1 may be acceptable with permission of
	// instructor)" after a course: that course, or this one with permission.
	acceptableRe = regexp.MustCompile(`(?i)\(\s*notes?\s*:\s*(` + codePat + `(?:\s*(?:/|,|or)\s*` + codePat + `)*)\s+(?:may\s+be|is|are)\s+acceptable\s+with\s+([^()]+?)\s*\)`)
	// concurrentOKRe matches a course that may be in progress: "(MAT257Y1 can be
	// taken concurrently)".
	concurrentOKRe = regexp.MustCompile(`(?i)\(([^()]*?\b(?:can|may)\s+be\s+taken\s+concurrently)\s*\)`)

	// codeListOrRe matches "ENG110Y1, ENG140Y1 or ENG150Y1" after "enrolled in":
	// any one of them.
	codeListOrRe = regexp.MustCompile(`^` + codePat + `(?:\s*,\s*` + codePat + `)*\s*,?\s+(?i:or)\s+` + codePat + `$`)
	listCommaRe  = regexp.MustCompile(`\s*,\s*(?:(?i:or)\s+)?|\s+(?i:or)\s+`)

	whoRe      = regexp.MustCompile(`(?i)^students\s+|\s+who\s+(?:are\s+)?`)
	andWhoRe   = regexp.MustCompile(`(?i),?\s+and\s+who\s+(?:are|have)\s+`)
	routeEndRe = regexp.MustCompile(`[\s.,;]+$`)
)

// extractRoutes takes route sentences and bracketed route notes out of s.
func extractRoutes(s string) (string, []route) {
	var routes []route
	s = acceptableRe.ReplaceAllStringFunc(s, func(m string) string {
		g := acceptableRe.FindStringSubmatch(m)
		routes = append(routes, route{kind: routeNote, note: cleanNote(m[1 : len(m)-1])})
		return "/ (" + g[1] + ", " + g[2] + ")"
	})
	s = concurrentOKRe.ReplaceAllStringFunc(s, func(m string) string {
		inner := concurrentOKRe.FindStringSubmatch(m)[1]
		if codes := codeRe.FindAllString(inner, -1); codes != nil {
			routes = append(routes, route{kind: routeConcurrent, note: cleanNote(inner), codes: codes})
		}
		return ""
	})

	t := noteBreakRe.ReplaceAllString(s, ". $1:")
	var keep []string
	found := false
	for _, sentence := range splitSentences(t) {
		if r, ok := matchRoute(sentence); ok {
			routes = append(routes, r)
			found = true
			continue
		}
		keep = append(keep, sentence)
	}
	if found {
		s = strings.Join(keep, ". ")
	}
	return s, routes
}

// matchRoute reads one route sentence.
func matchRoute(sentence string) (route, bool) {
	r := route{note: sentence}
	if m := routeExtraRe.FindStringSubmatch(sentence); m != nil {
		r.kind, r.codes, r.node = routeExtra, []string{m[1]}, parseFragment(m[2])
		if r.node == nil {
			return route{}, false
		}
		r.node.Concurrent = true
		return r, true
	}
	if m := routeSubstituteRe.FindStringSubmatch(sentence); m != nil {
		r.kind = routeSubstitute
		r.cond = parseFragment(whoRe.ReplaceAllString(m[1], ", "))
		subs := routeEndRe.ReplaceAllString(m[2], "")
		consumed := 0
		for _, p := range substitutePairRe.FindAllStringSubmatch(subs, -1) {
			n := parseFragment(p[1])
			if n == nil {
				return route{}, false
			}
			r.codes, r.subs = append(r.codes, p[2]), append(r.subs, n)
			consumed += len(p[0])
		}
		if r.cond == nil || r.codes == nil || consumed != len(subs) {
			return route{}, false // not just "A for X and B for Y"
		}
		return r, true
	}
	if m := routeFallbackRe.FindStringSubmatch(sentence); m != nil {
		list := routeEndRe.ReplaceAllString(m[2], "")
		if codeListOrRe.MatchString(list) {
			list = listCommaRe.ReplaceAllString(list, "/ ")
		}
		r.kind, r.node = routeAlt, parseFragment(list)
		if r.node == nil {
			return route{}, false
		}
		r.node.Concurrent = m[1] != ""
		return r, true
	}
	if m := routeAltRe.FindStringSubmatch(sentence); m != nil {
		text := andWhoRe.ReplaceAllString(m[1], ", ")
		if m[2] != "" {
			text += ", " + m[2]
		}
		r.kind, r.node = routeAlt, parseFragment(text)
		return r, r.node != nil
	}
	return route{}, false
}

// parseFragment parses part of a sentence as a requirement of its own.
func parseFragment(s string) *types.ReqNode {
	var discard []string
	return parseText(rewriteExamples(stripFiller(normalizeSpace(s), &discard)))
}

func normalizeSpace(s string) string { return strings.TrimSpace(wsRe.ReplaceAllString(s, " ")) }

// applyRoutes applies routes to the parsed field n. One whose course isn't in n
// changes nothing, so it is only a note.
func applyRoutes(n *types.ReqNode, routes []route) *types.ReqNode {
	for _, r := range routes {
		switch r.kind {
		case routeAlt:
			if r.node != nil && n != nil {
				n = &types.ReqNode{Type: types.NodeOr, Items: []*types.ReqNode{n, r.node}}
			}
		case routeSubstitute:
			for i, code := range r.codes {
				sub := r.subs[i]
				n = replaceCourse(n, code, func(c *types.ReqNode) *types.ReqNode {
					return &types.ReqNode{Type: types.NodeOr, Items: []*types.ReqNode{c,
						{Type: types.NodeAnd, Items: []*types.ReqNode{cloneNode(r.cond), cloneNode(sub)}}}}
				})
			}
		case routeExtra:
			n = replaceCourse(n, r.codes[0], func(c *types.ReqNode) *types.ReqNode {
				return &types.ReqNode{Type: types.NodeAnd, Items: []*types.ReqNode{c, cloneNode(r.node)}}
			})
		case routeConcurrent:
			for _, code := range r.codes {
				n = replaceCourse(n, code, func(c *types.ReqNode) *types.ReqNode {
					c.Concurrent = true
					return c
				})
			}
		}
	}
	return flattenSame(n)
}

// replaceCourse replaces every course node for code in n with f(node).
func replaceCourse(n *types.ReqNode, code string, f func(*types.ReqNode) *types.ReqNode) *types.ReqNode {
	return replaceNodes(n, func(c *types.ReqNode) bool { return c.Type == types.NodeCourse && c.Code == code }, f)
}

// cloneNode deep-copies n: its items, its slice fields (each nil stays nil)
// and MaxAmount. A node used in several places, or changed after it was
// copied, shares nothing with the original.
func cloneNode(n *types.ReqNode) *types.ReqNode {
	if n == nil {
		return nil
	}
	c := *n
	if n.MaxAmount != nil {
		v := *n.MaxAmount
		c.MaxAmount = &v
	}
	c.Designators = slices.Clone(n.Designators)
	c.Levels = slices.Clone(n.Levels)
	c.Breadth = slices.Clone(n.Breadth)
	c.Courses = slices.Clone(n.Courses)
	c.Exclude = slices.Clone(n.Exclude)
	c.ExcludeDesignators = slices.Clone(n.ExcludeDesignators)
	c.Lists = slices.Clone(n.Lists)
	c.Requirements = slices.Clone(n.Requirements)
	c.Programs = slices.Clone(n.Programs)
	c.Items = nil
	for _, it := range n.Items {
		c.Items = append(c.Items, cloneNode(it))
	}
	return &c
}

// flattenSame merges an and/or child into a parent of the same type.
func flattenSame(n *types.ReqNode) *types.ReqNode {
	if n == nil {
		return nil
	}
	var items []*types.ReqNode
	for _, it := range n.Items {
		it = flattenSame(it)
		if (n.Type == types.NodeAnd || n.Type == types.NodeOr) && it.Type == n.Type && !it.Concurrent {
			items = append(items, it.Items...)
		} else {
			items = append(items, it)
		}
	}
	n.Items = items
	return n
}
