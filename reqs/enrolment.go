package reqs

import (
	"regexp"
	"slices"

	"degree-explorer/types"
)

// ParseEnrolment turns a program's enrolment requirements, as scraped, into
// their stored JSON form. Program names stay unresolved until Link.
//
// The text is read line by line (enrolment_lines.go) into a small tree:
//
//   - audiences: "For students who have completed 9.0 or more credits:",
//     "For students admitted to ... (CMP1) ...". Each is one route; the routes
//     are OR'd, and whatever comes before the first applies to everyone;
//   - sections: "_Completed courses_", "_Variable Minimum Grade_", "_Special
//     Requirement_", "_Notes:_", the ASIP block;
//   - lists: a lead sentence ("The following courses must be completed:")
//     and its items, AND'ed unless the lead or a trailing conjunction says
//     otherwise.
//
// Sentences outside lists are classified one at a time: kind, credit
// threshold, variable cutoff, floor, program enrolment, application,
// cannot-combine, boilerplate. Anything left over is a note, except in a
// "Special Requirement" section, where it is a requirement the checker
// can't evaluate.
func ParseEnrolment(raw string) *types.Enrolment {
	e, _ := parseEnrolment(raw)
	return e
}

// enrolDiag is what the parser noticed but couldn't place. The corpus report
// lists it.
type enrolDiag struct {
	unknownHeadings []string
	noKind          bool
}

func parseEnrolment(raw string) (*types.Enrolment, *enrolDiag) {
	p := newEnrolParser(raw)
	for _, l := range enrolLines(raw) {
		p.line(l)
	}
	return p.finish(), p.diag
}

type enrolSection int

const (
	secNone    enrolSection = iota
	secCourses              // "Completed courses", "Variable Minimum Grade", ...
	secSpecial              // "Special Requirement"
	secNotes                // "Notes:", "How to apply:"
	secASIP                 // "Arts & Science Internship Program": all notes
)

type partKind int

const (
	partList  partKind = iota // a course requirement from a list; floors apply to it
	partFloor                 // an average added by a floor
	partOther                 // program enrolment, applications, other conditions
)

// part is one requirement of an audience. Parts are AND'ed.
type part struct {
	node *types.ReqNode
	sec  int
	kind partKind
}

// audience is one route into the program. auds[0] is everyone.
type audience struct {
	cond          []*types.ReqNode // who this route is for
	parts         []*part
	admission     *types.ReqNode // the variable cutoff, if any
	admissionFrom int            // where its wording came from (varFrom...)
	redirect      bool           // "cannot apply directly ... must first apply to ASSPE2340": notes only
}

type listKind int

const (
	listReq        listKind = iota // requirement items
	listExclusions                 // programs that can't be combined with this one
	listSubstitute                 // "In lieu of PSY100H1, we will also accept:"
	listNotes                      // items that are notes (a redirect)
)

type listMode int

const (
	modeAnd listMode = iota
	modeOr
	modeChoose
)

// enrolList is a lead and the items under it.
type enrolList struct {
	kind       listKind
	mode       listMode
	count      int     // choose
	amount     float64 // credits per item, for items that only list courses ("Economics: ECO101H1, ...")
	designator string  // "HIS credits to be made up in one of the following ways": items without a subject are HIS
	depth      int     // depth of the first item
	leadMarker string  // marker of the item the lead was, if it was one
	leadDepth  int
	options    [][]*enrolItem // items, split at "Option N:"
	forceOr    bool           // a standalone "or" between items
	oneHalf    bool           // "One of the following courses (0.5 credit)"
	code       string         // listSubstitute: the course replaced
	aud        *audience
	sec        int
}

type enrolItem struct {
	node       *types.ReqNode
	conj       string // trailing "or" / "and"
	additional bool   // "An additional 0.5 credit from ..."
	children   []*enrolItem
}

type substitution struct {
	code string
	alts []*types.ReqNode
	aud  *audience // the route it is for; nil for everyone
}

type enrolParser struct {
	e          *types.Enrolment
	diag       *enrolDiag
	kind       types.EnrolmentKind
	suspended  bool
	threshold  float64
	auds       []*audience
	cur        *audience
	sec        int
	secKind    enrolSection
	list       *enrolList
	exclusions []*types.ReqNode
	notes      []string
	adjusted   map[string]bool     // courses whose grades are adjusted before comparing
	joint      map[string][]string // designator -> jointly offered designators that also count
	subs       []substitution
	chain      *part            // a grade requirement later sentences add fallbacks to
	category   string           // the last admission category named, for "other ... admission categories"
	firstRound bool             // "considered in the first program request period only"
	inPlace    [][2]string      // {used, replaced}: "STA261H1 will be used in place of STA130H1 ... if they have completed both"
	best       bool             // "the grade average will be based on the higher course grades"
	alts       []*types.ReqNode // other ways in, decided by the department (routeSentence)
	hsAlt      bool             // Grade 12 courses can be fulfilled another way
	// "Students who do not have the Computer Science Admission Guarantee
	// must complete a supplementary application": maybe also students of
	// the guarantee's category who missed it.
	lostGuarantee bool
	softFloor     bool // the floor being applied says "will not usually be considered"
}

var (
	// "Grades in MAT158H1, MAT159H1, and MAT157Y1 will be adjusted to account
	// for the greater difficulty of those courses."
	adjustedRe = regexp.MustCompile(`(?i)grades\s+in\s+([^.]*?)\s+will\s+be\s+adjusted`)
	// "Jointly-offered GGR courses will also be considered (e.g. JEG, JGI,
	// JGE)."
	// "STA261H1 will be used in place of STA130H1 for program admission
	// purposes if a student has not completed STA130H1 or if they have
	// completed both STA130H1 and STA261H1": with both, the first counts.
	// "If more than 1.0 credits have been completed from LIN, CSC, PHL, or
	// PSY, the grade average will be based on the higher course grades", "the
	// two with the highest grades will be used".
	bestGradesRe = regexp.MustCompile(`(?i)based\s+on\s+the\s+higher\s+course\s+grades?|with\s+the\s+highest\s+grades\s+will\s+be\s+used`)
	// "Students who do not have the Computer Science Admission Guarantee must
	// complete a supplementary application", "failure to complete all the
	// required courses within 12 months ... will result in losing the
	// admission guarantee" (Rotman Commerce).
	lostGuaranteeRe = regexp.MustCompile(`(?i)\bdo\s+not\s+have\s+the\s+.{0,40}\bAdmission\s+Guarantee\b|\blosing\s+the\s+admission\s+guarantee\b`)
	inPlaceRe       = regexp.MustCompile(`(` + codePat + `)\s+will\s+be\s+used\s+in\s+place\s+of\s+(` + codePat + `)\b[^.]*\bcompleted\s+both\b`)
	jointRe         = regexp.MustCompile(`(?i)jointly-offered\s+([A-Z]{3})\s+courses\s+will\s+also\s+be\s+considered\s+\(e\.g\.\s*([A-Z]{3}(?:\s*,\s*[A-Z]{3})*)\)`)
)

func newEnrolParser(raw string) *enrolParser {
	p := &enrolParser{e: &types.Enrolment{Raw: raw}, diag: &enrolDiag{}, adjusted: map[string]bool{}, joint: map[string][]string{}}
	p.auds = []*audience{{}}
	p.cur = p.auds[0]
	plain := stripEmphasis(raw)
	for _, m := range adjustedRe.FindAllStringSubmatch(plain, -1) {
		for _, c := range codeRe.FindAllString(m[1], -1) {
			p.adjusted[c] = true
		}
	}
	for _, m := range jointRe.FindAllStringSubmatch(plain, -1) {
		p.joint[m[1]] = desigRe.FindAllString(m[2], -1)
	}
	p.best = bestGradesRe.MatchString(plain)
	p.lostGuarantee = lostGuaranteeRe.MatchString(plain)
	for _, m := range inPlaceRe.FindAllStringSubmatch(plain, -1) {
		p.inPlace = append(p.inPlace, [2]string{m[1], m[2]})
	}
	return p
}

func (p *enrolParser) common() *audience { return p.auds[0] }

// ---- assembly ----

func (p *enrolParser) finish() *types.Enrolment {
	p.closeList()
	e := p.e
	e.Notes = dedupe(p.notes)
	e.Exclusions = dedupeNodes(p.exclusions)
	e.FirstRoundOnly = p.firstRound
	switch {
	case p.kind != "":
		e.Kind = p.kind
	case p.suspended:
		e.Kind = types.EnrolmentSuspended
	default:
		e.Kind = types.EnrolmentLimited
		p.diag.noKind = true
	}
	if e.Kind == types.EnrolmentSuspended {
		return e
	}

	common := p.common()
	if p.lostGuarantee {
		// "For students admitted to other ... admission categories" is not
		// CMP1, but its "Students who do not have the ... Admission
		// Guarantee" may include CMP1 students who missed it.
		for _, a := range p.auds[1:] {
			for i, c := range a.cond {
				if c.Type == types.NodeNot && c.Items[0].Type == types.NodeProfile && c.Items[0].Attr == "admission_category" {
					a.cond[i] = orNode(c, ambiguousText("a "+c.Items[0].Value+" student who doesn't meet the admission guarantee may also apply this way"))
				}
			}
		}
	}
	for _, a := range p.auds[1:] {
		for _, pt := range a.parts {
			pt.node = p.substitute(pt.node, a)
		}
	}
	var branches []*types.ReqNode
	minBand := 0.0
	allBands := true
	for _, a := range p.auds[1:] {
		if a.redirect {
			continue
		}
		if n := a.node(); n != nil {
			branches = append(branches, n)
		}
		if b := bandOf(a); b != nil && (minBand == 0 || b.Amount < minBand) {
			minBand = b.Amount
		} else if b == nil {
			allBands = false
		}
	}
	threshold := p.threshold
	if threshold == 0 && e.Kind != types.EnrolmentFocus {
		threshold = 4.0
	}
	// Lists given before a band that starts above the threshold are for the
	// students below it: "Students who have completed at least 8.0 credits
	// but who are not already enrolled in ASMAJ2082 must apply using the
	// following courses:" after the general list.
	if len(branches) > 0 && allBands && minBand > threshold && threshold > 0 {
		var listed, rest []*part
		for _, pt := range common.parts {
			if pt.kind == partOther {
				rest = append(rest, pt)
			} else {
				listed = append(listed, pt)
			}
		}
		if listed != nil {
			low := &audience{cond: []*types.ReqNode{{Type: types.NodeCredits, Amount: threshold, MaxAmount: minBand - 0.5}}, parts: listed}
			branches = append([]*types.ReqNode{low.node()}, branches...)
			common.parts = rest
		}
	}
	var items []*types.ReqNode
	for _, pt := range common.parts {
		items = append(items, pt.node)
	}
	items = append(items, common.admission)
	if len(branches) > 0 {
		items = append(items, orNode(branches...))
	}
	req := andNode(items...)
	if threshold > 0 && !coversCredits(req, threshold) {
		req = andNode(&types.ReqNode{Type: types.NodeCredits, Amount: threshold}, req)
	}
	// Other ways in ("may be considered on the basis of their entire academic
	// record") still need the credits.
	for _, alt := range p.alts {
		req = orNode(req, andNode(&types.ReqNode{Type: types.NodeCredits, Amount: max(threshold, 4)}, alt))
	}
	if p.hsAlt {
		req = replaceNodes(req, func(n *types.ReqNode) bool { return n.Type == types.NodeProfile && n.Attr == "high_school_course" },
			func(n *types.ReqNode) *types.ReqNode {
				return orNode(n, &types.ReqNode{Type: types.NodeText, Kind: types.TextPreparation, Text: hsAltText})
			})
	}
	req = p.substitute(req, nil)
	req = p.usedInPlace(req)
	sharedCourses(req)
	capsWithinPools(req)
	if p.best {
		walkReq(req, func(n *types.ReqNode) {
			n.Best = n.Type == types.NodeAverage && n.Amount > 0
		})
	}
	req = tidy(p.extendJoint(req))
	e.Req = req
	return e
}

// node builds an audience's route: who it is for, its requirements and
// its variable cutoff.
func (a *audience) node() *types.ReqNode {
	items := append([]*types.ReqNode{}, a.cond...)
	for _, pt := range a.parts {
		items = append(items, pt.node)
	}
	items = append(items, a.admission)
	if len(a.parts) == 0 && a.admission == nil {
		return nil
	}
	return andNode(items...)
}

func bandOf(a *audience) *types.ReqNode {
	for _, c := range a.cond {
		if c.Type == types.NodeCredits {
			return c
		}
	}
	return nil
}

// coversCredits reports whether n already requires at least amount credits
// on every route, so a separate threshold would repeat it.
func coversCredits(n *types.ReqNode, amount float64) bool {
	if n == nil {
		return false
	}
	switch n.Type {
	case types.NodeCredits:
		return n.Amount >= amount && len(n.Designators) == 0 && len(n.Courses) == 0 && len(n.Levels) == 0 && len(n.Breadth) == 0
	case types.NodeAnd:
		for _, it := range n.Items {
			if coversCredits(it, amount) {
				return true
			}
		}
	case types.NodeOr:
		for _, it := range n.Items {
			if !coversCredits(it, amount) {
				return false
			}
		}
		return len(n.Items) > 0
	}
	return false
}

// substitute applies "In lieu of PSY100H1, we will also accept: PSY100Y5,
// or PSYA01H3 and PSYA02H3": every PSY100H1 node becomes an OR with the
// alternatives, keeping its grade on each alternative course.
func (p *enrolParser) substitute(n *types.ReqNode, aud *audience) *types.ReqNode {
	for _, s := range p.subs {
		if s.aud != aud {
			continue
		}
		n = replaceCourse(n, s.code, func(c *types.ReqNode) *types.ReqNode {
			items := []*types.ReqNode{c}
			for _, alt := range s.alts {
				alt = cloneNode(alt)
				if c.MinGrade > 0 {
					applyMinGrade(alt, c.MinGrade)
				}
				items = append(items, alt)
			}
			return &types.ReqNode{Type: types.NodeOr, Items: items}
		})
	}
	return tidy(n)
}

// usedInPlace applies "STA261H1 will be used in place of STA130H1 ... if
// they have completed both": in an or of the two, the replaced course only
// counts without the other, or(STA130H1, STA261H1) becomes
// or(and(STA130H1, not(STA261H1)), STA261H1).
func (p *enrolParser) usedInPlace(n *types.ReqNode) *types.ReqNode {
	if n == nil {
		return nil
	}
	for i, it := range n.Items {
		n.Items[i] = p.usedInPlace(it)
	}
	if n.Type != types.NodeOr {
		return n
	}
	for _, ip := range p.inPlace {
		used, replaced := ip[0], ip[1]
		if !slices.ContainsFunc(n.Items, func(c *types.ReqNode) bool { return c.Type == types.NodeCourse && c.Code == used }) {
			continue
		}
		for i, c := range n.Items {
			if c.Type == types.NodeCourse && c.Code == replaced {
				n.Items[i] = andNode(c, &types.ReqNode{Type: types.NodeNot, Items: []*types.ReqNode{{Type: types.NodeCourse, Code: used}}})
			}
		}
	}
	return n
}

// sharedCourses reads two requirements whose pools overlap ("1. 0.5 credit
// from ECO101H1/ ECO105Y1/ IRW230H1" and "2. 1.5 credits from SOC/ PSY/
// IRE/ IRW": IRW230H1 fits both) both ways: separate courses for each (the
// two together need their combined credits from the union), or a course
// counting for both (Review).
func sharedCourses(n *types.ReqNode) {
	if n == nil {
		return
	}
	for _, it := range n.Items {
		sharedCourses(it)
	}
	if n.Type != types.NodeAnd {
		return
	}
	var pools []*types.ReqNode
	for _, it := range n.Items {
		switch {
		case it.Type == types.NodeCredits && it.Amount > 0 && it.MaxAmount == 0 && len(it.Levels) == 0 && len(it.Breadth) == 0 && len(it.Exclude) == 0:
			pools = append(pools, it)
		case it.Type == types.NodeOr && !slices.ContainsFunc(it.Items, func(c *types.ReqNode) bool { return c.Type != types.NodeCourse }):
			// "ECO101H1 (67%)/ IRW230H1 (67%)/ ECO105Y1 (80%)": one of them.
			pools = append(pools, &types.ReqNode{Type: types.NodeCredits, Amount: minWeight(it), Courses: nodeCodes(it)})
		}
	}
	for i, a := range pools {
		for _, b := range pools[i+1:] {
			if !poolsOverlap(a, b) {
				continue
			}
			both := &types.ReqNode{Type: types.NodeCredits, Amount: a.Amount + b.Amount,
				Courses: dedupe(append(slices.Clone(a.Courses), b.Courses...)), Designators: dedupe(append(slices.Clone(a.Designators), b.Designators...))}
			n.Items = append(n.Items, orNode(both, ambiguousText("a course may count toward both requirements")))
		}
	}
}

// poolsOverlap reports whether a course could count for both pools and
// make a difference: a shared full-year course that alone carries both
// pools' credits (ECO220Y1 for "PSY201H1 or ECO220Y1" and "PSY202H1 or
// ECO220Y1") meets both either way.
func poolsOverlap(a, b *types.ReqNode) bool {
	if slices.ContainsFunc(a.Designators, func(d string) bool { return slices.Contains(b.Designators, d) }) {
		return true
	}
	fits := func(code string, p *types.ReqNode) bool {
		return slices.Contains(p.Courses, code) || slices.Contains(p.Designators, code[:3])
	}
	matters := func(code string) bool { return courseWeight(code) < a.Amount+b.Amount }
	return slices.ContainsFunc(a.Courses, func(c string) bool { return fits(c, b) && matters(c) }) ||
		slices.ContainsFunc(b.Courses, func(c string) bool { return fits(c, a) && matters(c) })
}

// capsWithinPools reads a cap next to the pool it limits as a share of that
// pool: "1.5 credits from SOC/ PSY/ IRE/ IRW, of which no more than 0.5
// credit can be IRE/ IRW" limits what counts toward the 1.5, not what the
// student has taken, so it means at least 1.0 of the 1.5 is SOC/ PSY. The
// cap becomes that: credits{1.0, [SOC, PSY]}. Averages are built before
// this, over the whole pool.
func capsWithinPools(n *types.ReqNode) {
	if n == nil {
		return
	}
	for _, it := range n.Items {
		capsWithinPools(it)
	}
	if n.Type != types.NodeAnd {
		return
	}
	for i, c := range n.Items {
		if c.Type != types.NodeCredits || c.Amount > 0 || c.MaxAmount == 0 || len(c.Designators) == 0 || len(c.Courses) > 0 {
			continue
		}
		for _, pool := range n.Items {
			if pool.Type != types.NodeCredits || pool.Amount <= c.MaxAmount || !isSubset(c.Designators, pool.Designators) {
				continue
			}
			rest := slices.DeleteFunc(slices.Clone(pool.Designators), func(d string) bool { return slices.Contains(c.Designators, d) })
			if len(rest) == 0 {
				continue
			}
			share := &types.ReqNode{Type: types.NodeCredits, Amount: pool.Amount - c.MaxAmount, Designators: rest,
				Levels: pool.Levels, Breadth: pool.Breadth, Exclude: pool.Exclude, MinGrade: pool.MinGrade}
			n.Items[i] = share
			// An average over the pool averages what counts toward it: the
			// share, and the rest from the whole pool.
			for _, avg := range n.Items {
				if avg.Type == types.NodeAverage && avg.Items == nil && slices.Equal(avg.Designators, pool.Designators) && len(avg.Courses) == 0 {
					avg.Items = []*types.ReqNode{
						{Type: types.NodeCredits, Amount: share.Amount, Designators: rest, Levels: pool.Levels},
						{Type: types.NodeCredits, Amount: c.MaxAmount, Designators: pool.Designators, Levels: pool.Levels},
					}
					avg.Designators, avg.Levels = nil, nil
				}
			}
			break
		}
	}
}

func isSubset(sub, of []string) bool {
	return !slices.ContainsFunc(sub, func(s string) bool { return !slices.Contains(of, s) })
}

// ambiguous encodes wording with two readings: meeting the strict one is
// Met, meeting only the lenient one is Review (the text says why), meeting
// neither is NotMet.
func ambiguous(strict, lenient *types.ReqNode, why string) *types.ReqNode {
	return orNode(strict, andNode(lenient, &types.ReqNode{Type: types.NodeText, Kind: types.TextAmbiguity, Text: why}))
}

// ambiguousText is the lenient branch alone: or(x, ambiguousText(why)) is x,
// or Review without it.
func ambiguousText(why string) *types.ReqNode {
	return &types.ReqNode{Type: types.NodeText, Kind: types.TextAmbiguity, Text: why}
}

// tidy flattens nested and/or nodes of the same type and drops duplicate
// children, which substitutions can leave behind.
func tidy(n *types.ReqNode) *types.ReqNode {
	if n == nil {
		return nil
	}
	for i, it := range n.Items {
		n.Items[i] = tidy(it)
	}
	if n.Type == types.NodeAnd || n.Type == types.NodeOr {
		return combineNodes(n.Type, n.Items)
	}
	return n
}

// extendJoint widens pools of a subject to its jointly offered courses:
// "Jointly-offered GGR courses will also be considered (e.g. JEG, JGI,
// JGE)". The designators are only examples, so each pool also gets the
// jointText alternative, which Link replaces with the full list.
func (p *enrolParser) extendJoint(n *types.ReqNode) *types.ReqNode {
	if n == nil || len(p.joint) == 0 {
		return n
	}
	for i, it := range n.Items {
		n.Items[i] = p.extendJoint(it)
	}
	if (n.Type != types.NodeCredits && n.Type != types.NodeAverage) || len(n.Designators) == 0 {
		return n
	}
	d := n.Designators[0]
	extra, ok := p.joint[d]
	if !ok {
		return n
	}
	n.Designators = dedupe(append(n.Designators, extra...))
	if n.Type == types.NodeAverage {
		return n
	}
	return orNode(n, jointText(d))
}

// jointText stands for the jointly offered courses of subject d until Link
// names their designators (linkJoint).
func jointText(d string) *types.ReqNode {
	return &types.ReqNode{Type: types.NodeText, Kind: types.TextOther, Text: "a jointly-offered " + d + " course"}
}
