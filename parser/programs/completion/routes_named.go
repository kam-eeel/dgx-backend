package completion

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/parser/grammar"
	"degree-explorer/parser/programs"
	"degree-explorer/types"
)

// Bounding routes: approval and named routes are bounded by the wider pool they
// name, and level, exemption and graduate routes are placed on their rows.

// boundPoolRe matches a route naming its wider pool, "any POL course with
// European content"; the lazy theme never spans a designator's own pool.
var boundPoolRe = regexp.MustCompile(`(?i)^(?:some\s+offerings\s+of\s+)?(?:other\s+|any\s+(?:other\s+)?)(?:(` + grammar.LevelPat + `)-level\s+)?(?:[\w,/\s-]+?[-\s]themed\s+)??(` + grammar.DesigPat + `)\s+(?:special\s+topics\s+(?:and\s+` + grammar.DesigPat + `\s+summer\s+abroad\s+)?)?courses?\b`)

// boundRoutes bounds the approval routes that name their wider pool:
// or(strict, text) becomes or(strict, and(the amount from the wider pool,
// text)), so a course outside both pools is NotMet rather than Review.
func (a *Assembler) boundRoutes() {
	a.boundOrRoutes(func(strict *types.ReqNode) *types.ReqNode {
		if strict.Type == types.NodeCredits && strict.Amount > 0 && strict.MaxAmount == nil && strict.Items == nil && strict.Lists == nil {
			return strict
		}
		return nil
	})
}

// boundOrRoutes turns each allocation or(strict, routes...) into
// or(strict, and(wider, route)...), wider bounded by pool(strict); a nil pool
// skips the row.
func (a *Assembler) boundOrRoutes(pool func(strict *types.ReqNode) *types.ReqNode) {
	walkRows(a.items, func(r *compRow) {
		if n := r.r.Req; r.r.Kind == types.RequirementAllocation && n != nil && n.Type == types.NodeOr && len(n.Items) >= 2 {
			if p := pool(n.Items[0]); p != nil {
				for i, it := range n.Items[1:] {
					if wider := routePool(p, it); wider != nil {
						n.Items[i+1] = programs.AndNode(wider, it)
					}
				}
			}
		}
	})
}

// anotherPoolRe matches a second pool after the one boundPoolRe
// read: "and any finance-themed RSM special topics courses".
var anotherPoolRe = regexp.MustCompile(`(?i)\b(?:and|or)\s+(?:any|other)\b`)

// routePool returns the wider pool that permission route names for strict,
// keeping strict's courses, or nil.
func routePool(strict, route *types.ReqNode) *types.ReqNode {
	if route.Type != types.NodeText || route.Kind != types.TextPermission {
		return nil
	}
	t := strings.TrimSpace(route.Text)
	m := boundPoolRe.FindStringSubmatch(t)
	if m == nil || anotherPoolRe.MatchString(t[len(m[0]):]) {
		return nil // a second pool: bounded by one, the other would be NotMet
	}
	w := &types.ReqNode{Type: types.NodeCredits, Amount: strict.Amount, Designators: []string{m[2]}, Courses: slices.Clone(strict.Courses)}
	if m[1] != "" {
		w.Levels = grammar.ParseLevels(m[1] + "-level")
	}
	if slices.ContainsFunc(w.Courses, func(c string) bool { return !grammar.LevelMatches(w.Levels, c) || slices.Contains(strict.Exclude, c) }) ||
		strict.Levels != nil && w.Levels == nil || strict.Breadth != nil || strict.Exclude != nil {
		return nil
	}
	if strict.Designators != nil {
		if !slices.Contains(strict.Designators, m[2]) {
			return nil
		}
		w.Designators = slices.Clone(strict.Designators)
	}
	return w
}

// routeCodesPat matches the codes in a route: "GGR491Y1, GGR492H1, or
// GGR499H1".
const routeCodesPat = `((?:` + grammar.CodePat + `)(?:\s*(?:[/,;]|,?\s*(?:or|and))\s*` + grammar.CodePat + `)*)`

var (
	// suitableCodesRe matches "Up to 1.0 credit can be used from GGR493Y1,
	// based on suitability". Group 1, "up to", makes it a cap.
	suitableCodesRe = regexp.MustCompile(`(?i)^(up\s+to\s+)?(?:an?\s+|one\s+)?` + grammar.AmountPat + `\s+credits?\s+(?:can\s+be\s+used\s+)?from\s+` + routeCodesPat + `,?\s+(?:can\s+be\s+used\s+to\s+meet\s+this\s+requirement,?\s+)?based\s+on\s+(?:the\s+)?suitability\b`)
	// routeOfferingsRe matches "Some offerings of POL336H1; ...; POL497H1
	// may also count towards", with any separator.
	routeOfferingsRe = regexp.MustCompile(`(?i)^(?:note:\s*)?some\s+offerings\s+of\s+` + routeCodesPat + `\s+may\s+also\s+count\s+towards?\b`)
	// discretionRe matches someone deciding, in a route that names its
	// courses: "at the department's discretion".
	discretionRe = regexp.MustCompile(`(?i)\b(?:discretion|approv\w*|permission)\b`)
	// substituteForRe matches "students may substitute ARH495H1 (Research
	// Practicum) for 0.5 credit of this requirement". Groups: the code, the cap.
	substituteForRe = regexp.MustCompile(`(?i)\bstudents\s+may\s+substitute\s+(` + grammar.CodePat + `)(?:\s*\([^)]*\))?\s+for\s+` + grammar.AmountPat + `\s+credits?\s+of\s+this\s+requirement\b`)
	// substituteOneOfRe matches "may request to substitute one of CSC494H1/
	// CSC495H1 for one of the courses in list 2".
	substituteOneOfRe = regexp.MustCompile(`(?i)\bmay\s+request\s+to\s+substitute\s+one\s+of\s+` + routeCodesPat + `\s+for\s+one\s+of\s+the\s+courses\s+in\b`)
	// unitTopicsRe matches special-topics courses by subject: "Health
	// related Anthropology Special Topics courses may be chosen when offered".
	unitTopicsRe = regexp.MustCompile(`(?i)^(?:special\s+topics\s+courses,\s+research\s+opportunity\s+courses,\s+and\s+research\s+excursion\s+courses\s+from\s+([\w&\s]+?)\s+can\s+be\s+used\s+to\s+meet\s+this\s+requirement\b|([\w\s-]+?)\s+special\s+topics\s+courses\s+may\s+be\s+chosen\s+when\s+offered\s+at\s+the\s+[\w\s]+?\s+levels?$)`)
	// otherLevelRe matches any course at a level: "Other 400-level courses
	// ... can also be considered for fulfillment of".
	otherLevelRe = regexp.MustCompile(`(?i)^other\s+(` + grammar.LevelPat + `)-level\s+courses\b.*\bcan\s+(?:also\s+)?be\s+considered\s+for\s+fulfil?lment\s+of\b`)
	// oneOutsideRe matches one course off the lists: "One 0.5 credit course
	// at the 300- or 400-level not listed in the above clusters may count".
	oneOutsideRe = regexp.MustCompile(`(?i)^(?:note:\s*)?one\s+` + grammar.AmountPat + `\s+credit\s+course\s+at\s+the\s+([\w\s-]+?levels?)\s+not\s+listed\s+in\s+the\s+above\s+(?:clusters|lists|groups)\s+may\s+count\s+towards?\b`)
	// otherProgramsRe matches "Approved courses in other programs may be
	// substituted for up to 1.0 FAH credit". Groups: the cap, the designator.
	otherProgramsRe = regexp.MustCompile(`(?i)^approved\s+courses\s+in\s+other\s+programs\s+may\s+be\s+substituted\s+for\s+up\s+to\s+` + grammar.AmountPat + `\s+(` + grammar.DesigPat + `)\s+credits?$`)
	// subjectShareRe matches subjects' courses up to a cap: "students may
	// take 0.5 credit in history or political science courses corresponding".
	subjectShareRe = regexp.MustCompile(`(?i)^in\s+consultation\s+with\s+the\s+undergraduate\s+coordinator,\s+students\s+may\s+take\s+(?:up\s+to\s+)?` + grammar.AmountPat + `\s+credits?\s+in\s+([\w\s]+?)\s+courses\s+corresponding\s+to\s+their\s+(?:two\s+)?chosen\s+language\s+areas?(?:\s+\(` + grammar.AmountPat + `\s+credits?\s+per\s+language\s+area\))?\s+and\s+taught\s+outside\s+the\s+department$`)
	// siblingSubstituteRe matches "some students may substitute language
	// courses with the equivalent number of credits in requirement 3".
	siblingSubstituteRe = regexp.MustCompile(`(?i)\bsome\s+students\s+may\s+substitute\s+language\s+courses\s+with\s+the\s+equivalent\s+number\s+of\s+credits\s+in\s+requirement\s+\d+\b`)
)

// compStrictPool is the strict reading of a row a named route widens: an
// amount of courses, or one of some courses of the same weight, with the
// in-node floors and(pool, floor) carries.
type compStrictPool struct {
	pool   *types.ReqNode
	floors []*types.ReqNode
	byName map[string]*types.CourseList
}

// strictPool reads the non-text alternatives of or node n: pools of the same
// amount, or courses of the same weight. ok is false for any other strict
// reading.
func (a *Assembler) strictPool(n *types.ReqNode, byName map[string]*types.CourseList) (sp compStrictPool, ok bool) {
	plain := func(x *types.ReqNode) bool {
		return x.Type == types.NodeCredits && x.Amount > 0 && x.MaxAmount == nil && x.Items == nil && x.Breadth == nil && x.Exclude == nil &&
			x.ExcludeDesignators == nil && x.Requirements == nil && x.MinGrade == 0 && x.Weight == 0 && !x.Transfer && x.MaxCourseAmount == 0
	}
	var p *types.ReqNode
	for _, it := range n.Items {
		if hasText(it) {
			continue
		}
		switch {
		case it.Type == types.NodeAnd && len(it.Items) >= 2 && plain(it.Items[0]) && p == nil:
			for _, f := range it.Items[1:] {
				if !plain(f) || f.Amount >= it.Items[0].Amount {
					return sp, false
				}
			}
			sp.floors = it.Items[1:]
			it = it.Items[0]
			fallthrough
		case plain(it):
			if p == nil {
				p = grammar.CloneNode(it)
				if it.Lists != nil {
					if it.Designators != nil || it.Courses != nil {
						return sp, false
					}
					// Lists with filters stay a pool of lists, which only a
					// route naming courses already on them widens.
					if codes, ok := a.listCodes(it.Lists, byName); ok {
						codes = slices.DeleteFunc(codes, func(c string) bool { return !grammar.LevelMatches(it.Levels, c) })
						p.Courses, p.Levels, p.Lists = codes, nil, nil
					}
				}
				continue
			}
			if it.Amount != p.Amount || it.Designators != nil || it.Levels != nil || it.Lists != nil || p.Designators != nil || p.Levels != nil || p.Lists != nil || sp.floors != nil {
				return sp, false
			}
			p.Courses = grammar.Dedupe(append(p.Courses, it.Courses...))
		case it.Type == types.NodeCourse:
			w := grammar.CourseWeight(it.Code)
			if p == nil {
				p = &types.ReqNode{Type: types.NodeCredits, Amount: w}
			}
			if p.Amount != w || p.Designators != nil || p.Levels != nil || p.Lists != nil || sp.floors != nil {
				return sp, false
			}
			p.Courses = grammar.Dedupe(append(p.Courses, it.Code))
		default:
			return sp, false
		}
	}
	sp.pool = p
	sp.byName = byName
	return sp, p != nil
}

// namedRoutes bounds routes naming their wider pool by codes, a unit, subjects
// or levels as and(wider, text), so a course outside both pools is NotMet
// rather than Review. A route no reader reads stays bare.
func (a *Assembler) namedRoutes(lists []*types.CourseList) {
	byName := listsByName(lists)
	var walk func(rs []*compRow, parent *compRow)
	walk = func(rs []*compRow, parent *compRow) {
		for _, r := range rs {
			a.namedRoute(r, parent, byName)
			walk(r.children, r)
		}
	}
	if a.total != nil {
		walk([]*compRow{a.total}, nil)
	}
	walk(a.floors, nil)
	walk(a.items, nil)
	walk(a.program, nil)
}

// namedRoute bounds the named routes of row r.
func (a *Assembler) namedRoute(r, parent *compRow, byName map[string]*types.CourseList) {
	n := r.r.Req
	if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr || len(n.Items) < 2 {
		return
	}
	// The row's floors, taken before the limits a route adds.
	floors := slices.DeleteFunc(slices.Clone(r.children), func(c *compRow) bool { return c.r.Kind != types.RequirementConstraint || c.r.Req == nil })
	sp, ok := a.strictPool(n, byName)
	if !ok {
		return
	}
	strict := sp.pool
	bounded := false
	for i, it := range n.Items {
		// Kind other too: textKinds, which makes such a route permission, runs
		// later; the readers are specific.
		if it.Type != types.NodeText || it.Kind != types.TextPermission && it.Kind != types.TextOther {
			continue
		}
		wider, kind, limits := a.routeWider(sp, a.clean(it.Text), r, parent)
		if wider == nil {
			continue
		}
		text := grammar.CloneNode(it)
		text.Kind = kind
		n.Items[i] = programs.AndNode(wider, text)
		// A floor on the row's courses gets or(floor, and(floor with the named
		// courses, text)); a level floor they all pass stays; any other gets the
		// route as its Review alternative.
		var added []string
		if wider.Type == types.NodeCredits && wider.Designators == nil && wider.Lists == nil && wider.Levels == nil {
			added = slices.DeleteFunc(slices.Clone(wider.Courses), func(c string) bool { return slices.Contains(strict.Courses, c) })
		}
		for _, f := range floors {
			switch {
			case added != nil && a.floorWith(f.r.Req, strict.Courses, added, byName) != nil:
				f.r.Req = programs.OrNode(f.r.Req, programs.AndNode(a.floorWith(f.r.Req, strict.Courses, added, byName), grammar.CloneNode(text)))
			case added != nil && levelFloor(f.r.Req) && !slices.ContainsFunc(added, func(c string) bool { return !grammar.LevelMatches(f.r.Req.Levels, c) }):
			case wider.Type == types.NodeCredits && levelFloor(f.r.Req) && slices.Equal(wider.Levels, f.r.Req.Levels):
			case wider.Type == types.NodeAnd && spreadFloor(f.r.Req) != nil:
				// One course off the lists, in "the relevant cluster": the
				// spread over one list fewer.
				c := grammar.CloneNode(spreadFloor(f.r.Req))
				c.Count--
				if c.Count == 1 {
					// One part: a course on two lists can't count twice, so the
					// strict spread's exclusions go.
					for _, it := range c.Items {
						it.Exclude = nil
					}
				}
				f.r.Req = programs.OrNode(f.r.Req, programs.AndNode(c, grammar.CloneNode(text)))
			default:
				f.r.Req = programs.OrNode(f.r.Req, grammar.CloneNode(text))
			}
		}
		for _, l := range limits {
			r.children = append(r.children, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Req: l, Raw: it.Text}})
		}
		bounded = true
	}
	if !bounded || sp.floors == nil {
		return
	}
	// The in-node floor becomes a constraint under the row, now that no route is
	// bare beside it.
	for k, it := range n.Items {
		if it.Type == types.NodeAnd && !hasText(it) {
			n.Items[k] = it.Items[0]
			for _, f := range it.Items[1:] {
				r.children = append(r.children, &compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Req: f, Raw: RawHead(r.r)}})
			}
		}
	}
}

// routeWider returns the wider pool route s names for the strict pool, the
// kind of its text and the limits it states, or nil.
func (a *Assembler) routeWider(sp compStrictPool, s string, r, parent *compRow) (*types.ReqNode, types.TextKind, []*types.ReqNode) {
	strict := sp.pool
	wider := grammar.CloneNode(strict)
	// withCodes widens wider by codes at the pool's levels. A pool of lists with
	// filters takes only codes already on its lists: the route adds only its cap.
	withCodes := func(codes []string) bool {
		if slices.ContainsFunc(codes, func(c string) bool { return !grammar.LevelMatches(strict.Levels, c) }) {
			return false
		}
		if strict.Lists != nil {
			return !slices.ContainsFunc(codes, func(c string) bool { return !listHas(strict.Lists, c, sp.byName, 0) })
		}
		wider.Courses = grammar.Dedupe(append(wider.Courses, codes...))
		return true
	}
	capOf := func(codes []string, v float64) []*types.ReqNode {
		if v <= 0 || v >= strict.Amount && v >= weightOf(codes) {
			return nil
		}
		return []*types.ReqNode{{Type: types.NodeCredits, Courses: grammar.Dedupe(slices.Clone(codes)), MaxAmount: grammar.MaxAmount(v)}}
	}
	if m := suitableCodesRe.FindStringSubmatch(s); m != nil {
		codes := grammar.CodeRe.FindAllString(m[3], -1)
		if !withCodes(codes) {
			return nil, "", nil
		}
		var limits []*types.ReqNode
		if m[1] != "" {
			v, _ := strconv.ParseFloat(m[2], 64)
			limits = capOf(codes, v)
		}
		return wider, types.TextPermission, limits
	}
	if m := routeOfferingsRe.FindStringSubmatch(s); m != nil {
		if !withCodes(grammar.CodeRe.FindAllString(m[1], -1)) {
			return nil, "", nil
		}
		// The offering's topic decides, unless someone does ("at the department's
		// discretion").
		if discretionRe.MatchString(s) {
			return wider, types.TextPermission, nil
		}
		return wider, types.TextContent, nil
	}
	if m := substituteForRe.FindStringSubmatch(s); m != nil {
		v, _ := strconv.ParseFloat(m[2], 64)
		if !withCodes([]string{m[1]}) {
			return nil, "", nil
		}
		return wider, types.TextPermission, capOf([]string{m[1]}, v)
	}
	if m := substituteOneOfRe.FindStringSubmatch(s); m != nil {
		codes := grammar.CodeRe.FindAllString(m[1], -1)
		one := 0.0
		for _, c := range codes {
			one = max(one, grammar.CourseWeight(c))
		}
		if !withCodes(codes) {
			return nil, "", nil
		}
		return wider, types.TextPermission, capOf(codes, one)
	}
	if strict.Lists != nil && !oneOutsideRe.MatchString(s) {
		return nil, "", nil // the readers below widen pools of courses or filters
	}
	if m := unitTopicsRe.FindStringSubmatch(s); m != nil {
		desigs, _ := routeFilters(m[1] + m[2])
		_, levels := routeFilters(s)
		if len(desigs) != 1 || strict.Levels != nil {
			return nil, "", nil
		}
		switch {
		case levels != nil && strict.Designators == nil && strict.Lists == nil && strict.Courses != nil &&
			slices.ContainsFunc(strict.Courses, func(c string) bool { return !grammar.LevelMatches(levels, c) }):
			// A pool course below the levels ("ANT208H1" beside "at the third
			// or fourth year levels"): a list of the row's own, the pool's
			// courses and the designator's at the levels.
			l := a.rowUnion(r, []*types.ReqNode{
				{Type: types.NodeCredits, Courses: slices.Clone(strict.Courses)},
				{Type: types.NodeCredits, Designators: desigs, Levels: levels},
			})
			wider = &types.ReqNode{Type: types.NodeCredits, Amount: strict.Amount, Lists: []string{l.Name}}
		default:
			wider.Designators = grammar.Dedupe(append(wider.Designators, desigs...))
			if strict.Designators == nil && !slices.ContainsFunc(strict.Courses, func(c string) bool { return !grammar.LevelMatches(levels, c) }) {
				wider.Levels = levels
			}
		}
		kind := types.TextPermission
		if m[2] != "" {
			kind = types.TextContent // the course's topic decides
		}
		return wider, kind, nil
	}
	if m := otherLevelRe.FindStringSubmatch(s); m != nil {
		levels := grammar.ParseLevels(m[1] + "-level")
		if d, _ := routeFilters(s); d != nil || strict.Designators != nil || strict.Levels != nil ||
			slices.ContainsFunc(strict.Courses, func(c string) bool { return !grammar.LevelMatches(levels, c) }) {
			return nil, "", nil
		}
		return &types.ReqNode{Type: types.NodeCredits, Amount: strict.Amount, Levels: levels}, types.TextPermission, nil
	}
	if m := oneOutsideRe.FindStringSubmatch(s); m != nil {
		v, _ := strconv.ParseFloat(m[1], 64)
		if v >= strict.Amount {
			return nil, "", nil
		}
		// The amount of any courses, the rest from the strict pool as stated:
		// and(credits{amount}, credits{amount − v, pool}).
		w := grammar.CloneNode(r.r.Req.Items[0])
		if w.Type != types.NodeCredits {
			return nil, "", nil
		}
		w.Amount = strict.Amount - v
		total := &types.ReqNode{Type: types.NodeCredits, Amount: strict.Amount}
		if levels := grammar.ParseLevels(m[2]); levels != nil && w.MaxAmount == nil {
			// "at the 300- or 400-level": the total from a list of the row's
			// own, the pool and any course at those levels.
			item := grammar.CloneNode(w)
			item.Amount = 0
			l := a.rowUnion(r, []*types.ReqNode{item, {Type: types.NodeCredits, Levels: levels}})
			total.Lists = []string{l.Name}
		}
		return programs.AndNode(total, w), types.TextPermission, nil
	}
	if m := otherProgramsRe.FindStringSubmatch(s); m != nil {
		v, _ := strconv.ParseFloat(m[1], 64)
		if !slices.Equal(strict.Designators, []string{m[2]}) || strict.Courses != nil || strict.Levels != nil || v >= strict.Amount {
			return nil, "", nil
		}
		return &types.ReqNode{Type: types.NodeCredits, Amount: strict.Amount}, types.TextPermission,
			[]*types.ReqNode{{Type: types.NodeCredits, ExcludeDesignators: []string{m[2]}, MaxAmount: grammar.MaxAmount(v)}}
	}
	if m := subjectShareRe.FindStringSubmatch(s); m != nil {
		desigs, levels := routeFilters(m[2])
		if len(desigs) < 1 || levels != nil || strict.Designators != nil || strict.Levels != nil {
			return nil, "", nil
		}
		v, _ := strconv.ParseFloat(m[1], 64)
		if m[3] != "" {
			v, _ = strconv.ParseFloat(m[3], 64) // per area: per row
		}
		wider.Designators = desigs
		var listed []string
		for _, c := range strict.Courses {
			if slices.Contains(desigs, c[:3]) {
				listed = append(listed, c)
			}
		}
		return wider, types.TextPermission, []*types.ReqNode{{Type: types.NodeCredits, Designators: slices.Clone(desigs), Exclude: listed, MaxAmount: grammar.MaxAmount(v)}}
	}
	if siblingSubstituteRe.MatchString(s) && parent != nil {
		var other *compRow
		for _, c := range parent.children {
			if c != r && c.r.Kind == types.RequirementAllocation {
				if other != nil {
					return nil, "", nil
				}
				other = c
			}
		}
		if other == nil || other.r.Req == nil {
			return nil, "", nil
		}
		on := other.r.Req
		if on.Type != types.NodeOr {
			on = programs.OrNode(on)
		}
		osp, ok := a.strictPool(on, sp.byName)
		if !ok || osp.pool.Designators != nil || osp.pool.Levels != nil || !withCodes(osp.pool.Courses) {
			return nil, "", nil
		}
		return wider, types.TextPermission, nil
	}
	return nil, "", nil
}

// listHas reports whether code is on one of the lists names: listed, or
// matched by an item's designators and levels and not excluded. An item with
// other filters is read as not holding it.
func listHas(names []string, code string, byName map[string]*types.CourseList, depth int) bool {
	if depth > 4 {
		return false
	}
	for _, name := range names {
		l := byName[name]
		if l == nil {
			continue
		}
		for _, it := range l.Items {
			if it.Type != types.NodeCredits || it.Breadth != nil || it.Requirements != nil || slices.Contains(it.Exclude, code) || !grammar.LevelMatches(it.Levels, code) {
				continue
			}
			switch {
			case slices.Contains(it.Courses, code):
				return true
			case it.Lists != nil && it.Courses == nil && it.Designators == nil:
				if listHas(it.Lists, code, byName, depth+1) {
					return true
				}
			case it.Designators != nil && it.Courses == nil && it.Lists == nil && len(code) >= 3 && slices.Contains(it.Designators, code[:3]):
				return true
			}
		}
	}
	return false
}

// floorWith returns floor n with added joined to each of its pools listing
// only some of courses, lists read as their codes. Nil when it has no such
// pool or a node other than and, or and text.
func (a *Assembler) floorWith(n *types.ReqNode, courses, added []string, byName map[string]*types.CourseList) *types.ReqNode {
	if n.Type == types.NodeOr {
		for _, it := range n.Items {
			if !hasText(it) {
				return a.floorWith(it, courses, added, byName)
			}
		}
		return nil
	}
	w := grammar.CloneNode(n)
	found, ok := false, true
	grammar.WalkReq(w, func(x *types.ReqNode) {
		switch x.Type {
		case types.NodeAnd, types.NodeOr, types.NodeText:
		case types.NodeCredits:
			if x.Lists != nil && x.Courses == nil && x.Designators == nil && x.Levels == nil {
				codes, lok := a.listCodes(x.Lists, byName)
				if !lok {
					ok = false
					return
				}
				x.Courses = slices.DeleteFunc(codes, func(c string) bool { return slices.Contains(x.Exclude, c) })
				x.Lists, x.Exclude = nil, nil
			}
			if x.Courses == nil || x.Designators != nil || x.Lists != nil || slices.ContainsFunc(x.Courses, func(c string) bool { return !slices.Contains(courses, c) }) {
				ok = false
				return
			}
			x.Courses = grammar.Dedupe(append(x.Courses, added...))
			found = true
		default:
			ok = false
		}
	})
	if !found || !ok {
		return nil
	}
	return w
}

// exemptAtLevelRe matches "Students exempted from FSL320H1/ FSL322H1 may
// replace it with any FRE course at the 300+ level".
var exemptAtLevelRe = regexp.MustCompile(`(?i)^students\s+exempted\s+from\s+` + relaxCodesPat + `\s+may\s+replace\s+(?:it|them)\s+with\s+any\s+(` + grammar.DesigPat + `)\s+course\s+at\s+the\s+(` + grammar.LevelPat + `)\+\s+level$`)

// exemptAtLevel bounds the exemptAtLevelRe alternatives of the rows' or
// nodes: and(credits{the exempted credits, DES, the level and up},
// text{placement}).
func (a *Assembler) exemptAtLevel() {
	var walk func(n *types.ReqNode)
	walk = func(n *types.ReqNode) {
		if n == nil {
			return
		}
		for _, it := range n.Items {
			walk(it)
		}
		if n.Type != types.NodeOr {
			return
		}
		for i, it := range n.Items {
			if it.Type != types.NodeText {
				continue
			}
			m := exemptAtLevelRe.FindStringSubmatch(a.clean(it.Text))
			if m == nil {
				continue
			}
			v := exemptedCredits(n, grammar.CodeRe.FindAllString(m[1], -1))
			if v == 0 {
				continue
			}
			from, _ := strconv.Atoi(m[3])
			var levels []int
			for l := from; l <= 400; l += 100 {
				levels = append(levels, l)
			}
			n.Items[i] = programs.AndNode(&types.ReqNode{Type: types.NodeCredits, Amount: v, Designators: []string{m[2]}, Levels: levels}, TextNode(types.TextPlacement, it.Text))
		}
	}
	for _, rs := range [][]*compRow{a.floors, a.items, a.program} {
		var rows func(rs []*compRow)
		rows = func(rs []*compRow) {
			for _, r := range rs {
				walk(r.r.Req)
				rows(r.children)
			}
		}
		rows(rs)
	}
}

// levelFloor reports whether n is a floor on levels only: credits{amount,
// levels}.
func levelFloor(n *types.ReqNode) bool {
	return n.Type == types.NodeCredits && n.Amount > 0 && n.Levels != nil && n.Courses == nil && n.Designators == nil && n.Lists == nil &&
		n.Breadth == nil && n.Requirements == nil && n.Exclude == nil && n.Items == nil && n.MaxAmount == nil
}

// spreadFloor returns the choose node of a spread floor, choose{k ≥ 2, ...},
// alone or as the strict alternative of an or, or nil.
func spreadFloor(n *types.ReqNode) *types.ReqNode {
	if n.Type == types.NodeOr && len(n.Items) > 0 {
		n = n.Items[0]
	}
	if n.Type == types.NodeChoose && n.Count >= 2 {
		return n
	}
	return nil
}

// otherProgramsFloors places each otherProgramsRe note no row's pool took
// on the one floor of the designator's credits: or(floor, and(credits{M − N,
// DES}, text{permission})). It counts toward the floor only, not the total.
func (a *Assembler) otherProgramsFloors() {
	var keep []string
	for _, s := range a.notes {
		m := otherProgramsRe.FindStringSubmatch(strings.TrimSuffix(a.clean(s), "."))
		if m == nil || !a.otherProgramsFloor(s, m) {
			keep = append(keep, s)
		}
	}
	a.notes = keep
}

// otherProgramsFloor places note s on the one floor that fits, and reports
// whether it did.
func (a *Assembler) otherProgramsFloor(s string, m []string) bool {
	v, _ := strconv.ParseFloat(m[1], 64)
	var fit []*compRow
	groups := [][]*compRow{a.floors, a.items, a.program}
	if a.total != nil {
		groups = append(groups, a.total.children)
	}
	for _, rs := range groups {
		for _, r := range rs {
			n := r.r.Req
			if r.r.Kind == types.RequirementConstraint && n != nil && n.Type == types.NodeCredits && slices.Equal(n.Designators, []string{m[2]}) &&
				n.Courses == nil && n.Levels == nil && n.Lists == nil && n.Exclude == nil && n.Amount > v && len(r.children) == 0 {
				fit = append(fit, r)
			}
		}
	}
	if len(fit) != 1 {
		return false
	}
	r := fit[0]
	less := grammar.CloneNode(r.r.Req)
	less.Amount -= v
	r.r.Req = programs.OrNode(r.r.Req, programs.AndNode(less, TextNode(types.TextPermission, strings.TrimSuffix(s, "."))))
	addRaw(r, s)
	return true
}

// boundListRoutes bounds, as boundRoutes does, the approval routes on a row
// whose strict pool is lists of listed courses only. Lists with filters stay
// bare: their union with the designator would need a list of its own.
func (a *Assembler) boundListRoutes(lists []*types.CourseList) {
	byName := listsByName(lists)
	a.boundOrRoutes(func(strict *types.ReqNode) *types.ReqNode {
		if onlyLists(strict) {
			if codes, ok := a.listCodes(strict.Lists, byName); ok {
				return &types.ReqNode{Type: types.NodeCredits, Amount: strict.Amount, Courses: codes}
			}
		}
		return nil
	})
}

// approvedOnLists reads or(pool, and(pool, text{suitableCodesRe})) on
// lists that already hold the route's courses (ASSPE1252's Group C): they
// count only with the approval, so the strict pool excludes them.
func approvedOnLists(r *compRow) {
	n := r.r.Req
	if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr || len(n.Items) != 2 {
		return
	}
	strict, alt := n.Items[0], n.Items[1]
	if strict.Type != types.NodeCredits || strict.Lists == nil || alt.Type != types.NodeAnd || len(alt.Items) != 2 ||
		!sameNode(alt.Items[0], strict) || alt.Items[1].Type != types.NodeText {
		return
	}
	m := suitableCodesRe.FindStringSubmatch(Clean(alt.Items[1].Text))
	if m == nil {
		return
	}
	strict.Exclude = grammar.Dedupe(append(slices.Clone(strict.Exclude), grammar.CodeRe.FindAllString(m[3], -1)...))
}

// graduateShareRe matches a share of graduate courses at a level:
// "Students may be permitted to take Math graduate courses. A 0.5 credit of
// these courses may count toward ... at the 400-level" (ASSPE1165, ASSPE1580).
var graduateShareRe = regexp.MustCompile(`(?i)^students\s+may\s+be\s+permitted\s+to\s+take\s+(\w+)\s+graduate\s+courses\.\s+an?\s+` + grammar.AmountPat + `\s+credits?\s+of\s+these\s+courses\s+may\s+count\s+toward\b.*\bat\s+the\s+([1-4])00-level\b`)

// graduateAtLevel gives the program's graduate-course route to every other
// pool or floor at its level, as or(row, and(row less the share, text)):
// Review, since only the approval tells.
func (a *Assembler) graduateAtLevel() {
	all := append(append(append([]*compRow{}, a.floors...), a.items...), a.program...)
	var route *types.ReqNode
	var share float64
	var level int
	walkRows(all, func(r *compRow) {
		grammar.WalkReq(r.r.Req, func(n *types.ReqNode) {
			if route != nil || n.Type != types.NodeText {
				return
			}
			if m := graduateShareRe.FindStringSubmatch(Clean(n.Text)); m != nil {
				route = n
				share, _ = strconv.ParseFloat(m[2], 64)
				level = int(m[3][0]-'0') * 100
			}
		})
	})
	if route == nil {
		return
	}
	walkRows(all, func(r *compRow) {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation && r.r.Kind != types.RequirementConstraint || n == nil || n.Type != types.NodeCredits ||
			n.Amount <= share || !slices.Contains(n.Levels, level) || n.Courses != nil || n.Lists != nil || n.Requirements != nil || n.MaxAmount != nil {
			return
		}
		less := grammar.CloneNode(n)
		less.Amount = n.Amount - share
		text := grammar.CloneNode(route)
		text.Kind = types.TextPermission
		r.r.Req = programs.OrNode(n, programs.AndNode(less, text))
	})
}
