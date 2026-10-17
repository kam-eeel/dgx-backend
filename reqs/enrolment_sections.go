package reqs

import (
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// The walk over enrolment lines: headings open sections and audiences, leads
// open lists, items fill them, and other sentences are classified alone.

func (p *enrolParser) line(l enrolLine) {
	switch l.kind {
	case lineHeading:
		p.heading(l)
	case lineItem:
		p.item(l)
	default:
		p.paragraph(l)
	}
}

var (
	asipHeadRe     = regexp.MustCompile(`(?i)Internship\s+Program|\bASIP\b`)
	audienceHeadRe = regexp.MustCompile(`(?i)^for\s+students\b`)
	optionHeadRe   = regexp.MustCompile(`(?i)^option\s+\d+$`)
	notesHeadRe    = regexp.MustCompile(`(?i)^(?:notes?|how\s+to\s+apply)$`)
	specialHeadRe  = regexp.MustCompile(`(?i)^special\s+requirements?$`)
	coursesHeadRe  = regexp.MustCompile(`(?i)\bcompleted\s+courses\b|\bminimum\s+(?:course\s+)?grades?\b|\bgrade\s+average\b`)
	variableHeadRe = regexp.MustCompile(`(?i)\bvariable\b`)
)

func (p *enrolParser) heading(l enrolLine) {
	h := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(l.text), ":"))
	if optionHeadRe.MatchString(h) && p.list != nil {
		p.list.options = append(p.list.options, nil)
		return
	}
	p.closeList()
	switch {
	case asipHeadRe.MatchString(h):
		p.setSection(secASIP)
	case audienceHeadRe.MatchString(h):
		p.newAudience(p.audienceCond(h))
	case notesHeadRe.MatchString(h):
		p.setSection(secNotes)
	case specialHeadRe.MatchString(h):
		p.setSection(secSpecial)
	case coursesHeadRe.MatchString(h):
		p.setSection(secCourses)
		if variableHeadRe.MatchString(h) {
			p.setVariable(variableText(h), varFromHeading)
		}
	default:
		p.diag.unknownHeadings = append(p.diag.unknownHeadings, h)
		p.setSection(secNone)
		p.notes = append(p.notes, h)
	}
}

func (p *enrolParser) setSection(k enrolSection) {
	p.sec++
	p.secKind = k
}

// newAudience starts a route for the students cond describes.
func (p *enrolParser) newAudience(cond []*types.ReqNode) *audience {
	p.closeList()
	a := &audience{cond: cond}
	p.auds = append(p.auds, a)
	p.cur = a
	if p.secKind == secNotes || p.secKind == secASIP {
		p.setSection(secNone)
	}
	return a
}

var (
	// bandRangeRe and bandMinRe match credit bands: "between 4.0 and 8.5
	// credits", "9.0 or more credits".
	bandRangeRe = regexp.MustCompile(`(?i)\b(?:between\s+)?(\d+\.\d+)\s+(?:and|to)\s+(\d+\.\d+)\s+credits\b`)
	bandMinRe   = regexp.MustCompile(`(?i)\b(?:at\s+least\s+)?(\d+\.\d+)\s+(?:or\s+more\s+credits|credits(?:\s+or\s+more)?)\b`)
	// categoryRe matches an admission category: "students NOT admitted to the Year
	// 1 Rotman Commerce admission category".
	categoryRe = regexp.MustCompile(`(?i)admission\s+categor`)
	languageRe = regexp.MustCompile(`(?i)\blanguage\b`)
	// audienceWhoRe strips "students with " or "students who are ": what's left
	// says what they have.
	audienceWhoRe = regexp.MustCompile(`(?i)^students\s+(?:with|who\s+are)\s+|\s*\(i\.e\.[^)]*\)$`)
	forRe         = regexp.MustCompile(`(?i)^for\s+`)
)

// audienceCond reads who an audience heading is for.
func (p *enrolParser) audienceCond(h string) []*types.ReqNode {
	if b := creditBand(h); b != nil {
		return []*types.ReqNode{b}
	}
	text := forRe.ReplaceAllString(h, "")
	switch {
	case categoryRe.MatchString(text):
		return []*types.ReqNode{p.categoryNode(text)}
	case languageRe.MatchString(text):
		// "students with no prior language experience"
		value := audienceWhoRe.ReplaceAllString(text, "")
		return []*types.ReqNode{{Type: types.NodeProfile, Attr: "language_background", Value: value, Text: text}}
	}
	return []*types.ReqNode{{Type: types.NodeText, Kind: classify(text), Text: text}}
}

// creditBand reads a credit band ("4.0 to 8.5 credits") as a credits node, or
// returns nil.
func creditBand(s string) *types.ReqNode {
	if m := bandRangeRe.FindStringSubmatch(s); m != nil {
		lo, _ := strconv.ParseFloat(m[1], 64)
		hi, _ := strconv.ParseFloat(m[2], 64)
		return &types.ReqNode{Type: types.NodeCredits, Amount: lo, MaxAmount: maxAmount(hi)}
	}
	if m := bandMinRe.FindStringSubmatch(s); m != nil {
		lo, _ := strconv.ParseFloat(m[1], 64)
		return &types.ReqNode{Type: types.NodeCredits, Amount: lo}
	}
	return nil
}

var (
	orLineRe = regexp.MustCompile(`(?i)^(?:or|OR)$`)
	// equivalentRe matches "MGT100H1 will be considered equivalent to RSM100H1".
	equivalentRe = regexp.MustCompile(`(` + codePat + `)\s+will\s+be\s+considered\s+equivalent\s+to\s+(` + codePat + `)`)
	// gluedHeadingRe matches a heading run into its sentence: "Variable Minimum
	// Grade A minimum grade is needed for entry."
	gluedHeadingRe = regexp.MustCompile(`^Variable\s+Minimum\s+Grade(?:\s+Average)?\s+(A\s)`)
	sentenceRe     = regexp.MustCompile(`\.([A-Z])`) // "admission to the program.Obtaining" (missing space)
	// reqParagraphRe matches a paragraph that is a requirement item, starting with
	// a code, a bracket or an amount: "( CHM135H1, CHM136H1)/ CHM151Y1)".
	reqParagraphRe = regexp.MustCompile(`^(?:[(\[]\s*)?(?:` + codePat + `|\d+\.\d+\s+(?:[A-Z]{3}\s+)?credits?\b)`)
	// reqItemRe matches item text that is a course requirement: a course, an
	// amount, a grade in a course, or Grade 12 preparation.
	reqItemRe = regexp.MustCompile(codePat + `|\b\d+\.\d+\s+(?:[A-Z]{3}\s+)?(?:\d00-level\s+)?(?:[A-Z]{3}\s+)?credits?\b|^(?i:Gr(?:ade|\.)\s*12)\b|^\d{2}%\s+in\b`)
)

func (p *enrolParser) paragraph(l enrolLine) {
	text := sentenceRe.ReplaceAllString(l.text, ". $1")
	if p.secKind == secASIP {
		p.notes = append(p.notes, cleanNote(text))
		return
	}
	// the ASIP block runs from its first sentence to the next heading
	if asipHeadRe.MatchString(text) {
		p.closeList()
		p.setSection(secASIP)
		p.notes = append(p.notes, cleanNote(text))
		return
	}
	if orLineRe.MatchString(text) && p.list != nil {
		p.list.forceOr = true
		return
	}
	for _, s := range splitSentences(text) {
		p.sentence(strings.TrimLeft(s, "* "), l)
	}
}

// sentence classifies one sentence of a paragraph, or a whole item that isn't
// a course requirement.
func (p *enrolParser) sentence(s string, l enrolLine) {
	s = cleanNote(gluedHeadingRe.ReplaceAllString(s, "$1"))
	if s == "" || enrolNoiseRe.MatchString(s) {
		return
	}
	if p.secKind == secASIP {
		p.notes = append(p.notes, s)
		return
	}
	switch {
	case enrolSuspendedRe.MatchString(s):
		p.suspended = true
		if p.kind == "" {
			p.kind = types.EnrolmentSuspended
		}
		p.notes = append(p.notes, s)
	case p.kindSentence(s):
	case p.thresholdSentence(s):
	case enrolVariableRe.MatchString(s):
		p.closeList()
		p.setVariable(variableText(s), varFromSentence)
		p.notes = append(p.notes, s)
	case enrolVariableScopeRe.MatchString(s):
		p.setVariable(s, varFromScope)
	case strings.HasSuffix(s, ":"):
		p.lead(s, l)
	case p.floorSentence(s):
	case equivalentRe.MatchString(s):
		// said in a route, an equivalence is for that route
		m := equivalentRe.FindStringSubmatch(s)
		sub := substitution{code: m[2], alts: []*types.ReqNode{{Type: types.NodeCourse, Code: m[1]}}}
		if p.cur != p.common() {
			sub.aud = p.cur
		}
		p.subs = append(p.subs, sub)
		p.notes = append(p.notes, s)
	case p.routeSentence(s):
	case isEnrolBoiler(s):
		p.notes = append(p.notes, s)
	default:
		if ok, lead := p.exclusionSentence(s); ok {
			if lead {
				p.openList(&enrolList{kind: listExclusions}, l)
			}
			return
		}
		switch {
		case p.specialSentence(s):
		case p.secKind == secNotes:
			p.notes = append(p.notes, s)
		case l.kind == lineText && reqParagraphRe.MatchString(s):
			p.addItem(l, s, "")
		case p.secKind == secSpecial:
			p.addPart(p.cur, &types.ReqNode{Type: types.NodeText, Kind: types.TextOther, Text: s}, partOther)
		default:
			p.notes = append(p.notes, s)
		}
	}
}

// Where a variable cutoff's wording comes from, weakest first: a sentence, a
// section heading, or a sentence naming the courses averaged.
const (
	varFromSentence = iota + 1
	varFromHeading
	varFromScope
)

// setVariable marks the current audience as having a cutoff the department
// sets each year. Better wording replaces weaker.
func (p *enrolParser) setVariable(text string, from int) {
	a := p.cur
	if from >= a.admissionFrom {
		a.admission = &types.ReqNode{Type: types.NodeText, Kind: types.TextAdmission, Text: text}
		a.admissionFrom = from
	}
}

// variableBothRe matches a cutoff on both grades and the average: "Variable
// Minimum Grade/Grade Average".
var variableBothRe = regexp.MustCompile(`(?i)minimum\s+grade\s+(?:and|or)\b|minimum\s+grades\b|\bgrade\s*/`)

// variableText names a variable cutoff from its heading or sentence: "variable
// minimum grade", with "average" or "and grade average".
func variableText(s string) string {
	switch {
	case !strings.Contains(strings.ToLower(s), "average"):
		return "variable minimum grade"
	case variableBothRe.MatchString(s):
		return "variable minimum grade and grade average"
	}
	return "variable minimum grade average"
}

// ---- leads and lists ----

var (
	leadOneOfRe     = regexp.MustCompile(`(?i)\bone\s+of\s+the\s+following\b`)
	leadGroupsRe    = regexp.MustCompile(`(?i)^(\d+\.\d+)\s+credits?\s+from\s+(one|two|three)\s+of\s+the\s+following\s+groups`)
	leadDesigRe     = regexp.MustCompile(`(?i)^(?:\d+\.\d+\s+)?([A-Z]{3})\s+credits\s+to\s+be\s+made\s+up`)
	leadInLieuRe    = regexp.MustCompile(`(?i)^in\s+lieu\s+of\s+(` + codePat + `),\s+we\s+will\s+also\s+accept`)
	leadRedirectRe  = regexp.MustCompile(`(?i)\bcannot\s+apply\s+directly\b`)
	leadStudentsRe  = regexp.MustCompile(`(?i)^students\s+who\s+have\s+completed\s+.*\bcredits\b.*\b(?:must|can|may)\s+apply\b`)
	leadForBandRe   = regexp.MustCompile(`(?i)\bfor\s+students\s+who\s+have\s+completed\s+(.+?\s+credits)$`)
	leadItemEndRe   = regexp.MustCompile(`(?i)[\s,]*\band:$`)
	leadFollowingRe = regexp.MustCompile(`(?i)\bfollowing\b|\byou\s+must\b|\bwe\s+will\b`)
)

// lead handles a sentence ending in ":". Most open a list; some also start an
// audience ("Students who have completed at least 8.0 credits ... must apply
// using the following courses:").
func (p *enrolParser) lead(s string, l enrolLine) {
	body := strings.TrimSuffix(s, ":")
	// "URB235H1 (formerly INI235H1) and URB236H1 (formerly INI236H1), and:" is an
	// item of its own, followed by more
	if leadItemEndRe.MatchString(s) && codeRe.MatchString(s) && !leadFollowingRe.MatchString(s) {
		p.addItem(l, leadItemEndRe.ReplaceAllString(s, ""), "and")
		p.closeList()
		return
	}
	if p.secKind == secNotes && !leadInLieuRe.MatchString(body) {
		if ok, lead := p.exclusionSentence(body); ok {
			if lead {
				p.openList(&enrolList{kind: listExclusions}, l)
			}
			return
		}
		p.notes = append(p.notes, s)
		return
	}
	list := &enrolList{kind: listReq}
	switch {
	case leadInLieuRe.MatchString(body):
		list.kind, list.code = listSubstitute, leadInLieuRe.FindStringSubmatch(body)[1]
	case leadRedirectRe.MatchString(body):
		a := p.newAudience(nil)
		a.redirect = true
		list.kind = listNotes
		p.notes = append(p.notes, s)
	case leadStudentsRe.MatchString(body):
		if b := creditBand(body); b != nil {
			p.newAudience([]*types.ReqNode{b})
		}
		if progCodeRe.MatchString(body) {
			p.notes = append(p.notes, s)
		}
	case leadForBandRe.MatchString(body):
		if b := creditBand(leadForBandRe.FindStringSubmatch(body)[1]); b != nil {
			p.newAudience([]*types.ReqNode{b})
		}
	default:
		if ok, lead := p.exclusionSentence(body); ok {
			if lead {
				p.openList(&enrolList{kind: listExclusions}, l)
			}
			return
		}
	}
	if m := leadGroupsRe.FindStringSubmatch(body); m != nil {
		amount, _ := strconv.ParseFloat(m[1], 64)
		k := countWords[strings.ToLower(m[2])]
		list.mode, list.count, list.amount = modeChoose, k, amount/float64(k)
		if k == 1 {
			list.mode = modeOr
		}
	} else if leadOneOfRe.MatchString(body) {
		list.mode = modeOr
		list.oneHalf = leadHalfCreditRe.MatchString(body)
	}
	if m := leadDesigRe.FindStringSubmatch(body); m != nil {
		list.designator = m[1]
	}
	if enrolWithinRe.MatchString(body) {
		p.addPart(p.cur, withinNode(), partOther)
	}
	if strings.Contains(strings.ToLower(body), "focus") && p.kind == "" {
		p.kind = types.EnrolmentFocus
	}
	p.openList(list, l)
}

func (p *enrolParser) openList(list *enrolList, l enrolLine) {
	p.closeList()
	list.depth = -1
	if l.kind == lineItem {
		list.leadMarker, list.leadDepth = l.marker, l.depth
	}
	list.aud, list.sec = p.cur, p.sec
	list.options = [][]*enrolItem{nil}
	p.list = list
}

// item handles a bullet or numbered line.
func (p *enrolParser) item(l enrolLine) {
	text, conj := itemConj(l.text)
	if p.secKind == secASIP {
		p.notes = append(p.notes, cleanNote(l.text))
		return
	}
	// a numbered note after an "In lieu of ..." list closes it
	if p.list != nil && p.list.leadMarker != "" && l.marker == p.list.leadMarker && l.depth <= p.list.leadDepth {
		p.closeList()
	}
	if p.list != nil {
		switch p.list.kind {
		case listExclusions:
			p.exclusionPhrase(text)
			return
		case listNotes:
			p.notes = append(p.notes, text)
			return
		case listSubstitute:
			if n := p.parseItem(text); n != nil {
				p.list.options[0] = append(p.list.options[0], &enrolItem{node: n, conj: conj})
			}
			return
		}
	}
	switch {
	case strings.HasSuffix(text, ":"):
		p.lead(text, l)
		return
	case strings.EqualFold(text, "either"): // its alternatives are nested under it
		p.addItem(l, "", conj)
		return
	}
	// an item may carry advice in a later sentence
	sentences := splitSentences(sentenceRe.ReplaceAllString(text, ". $1"))
	if len(sentences) == 0 {
		return
	}
	first := sentences[0]
	if p.secKind == secNotes || p.secKind == secSpecial || !reqItemRe.MatchString(first) ||
		enrolNotYetRe.MatchString(first) || enrolAdmissionRe.MatchString(first) {
		for _, s := range sentences {
			p.sentence(s, l)
		}
		return
	}
	p.addItem(l, first, conj)
	for _, s := range sentences[1:] {
		p.sentence(s, l)
	}
}

// addItem adds a requirement item to the open list, opening an AND list if
// there is none.
func (p *enrolParser) addItem(l enrolLine, text, conj string) {
	if p.list == nil || p.list.kind != listReq {
		p.openList(&enrolList{kind: listReq}, enrolLine{})
	}
	list := p.list
	it := &enrolItem{conj: conj, additional: additionalRe.MatchString(text)}
	if text != "" && !strings.EqualFold(text, "either") {
		it.node = p.parseItem(text)
		if list.amount > 0 {
			it.node = poolAmount(it.node, list.amount)
		}
		if list.designator != "" {
			addDesignator(it.node, list.designator)
		}
	}
	items := &list.options[len(list.options)-1]
	if list.depth < 0 {
		list.depth = l.depth
	}
	if l.depth > list.depth && len(*items) > 0 {
		last := (*items)[len(*items)-1]
		last.children = append(last.children, it)
		return
	}
	*items = append(*items, it)
}

// poolAmount turns an item that only lists courses into a pool of amount
// credits from them, as the lead says ("1.0 credit from one of the following
// groups:").
func poolAmount(n *types.ReqNode, amount float64) *types.ReqNode {
	if n == nil || n.Type == types.NodeCredits {
		return n
	}
	codes := nodeCodes(n)
	if len(codes) == 0 || containsType(n, types.NodeText, types.NodeCredits, types.NodeAverage) {
		return n
	}
	return &types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: dedupe(codes)}
}

func containsType(n *types.ReqNode, ts ...types.NodeType) bool {
	if n == nil {
		return false
	}
	for _, t := range ts {
		if n.Type == t {
			return true
		}
	}
	for _, it := range n.Items {
		if containsType(it, ts...) {
			return true
		}
	}
	return false
}

// addDesignator gives credit pools without a subject the list's designator:
// "2.0 HIS credits to be made up in one of the following ways".
func addDesignator(n *types.ReqNode, d string) {
	if n == nil {
		return
	}
	if n.Type == types.NodeCredits && len(n.Designators) == 0 && len(n.Courses) == 0 {
		n.Designators = []string{d}
	}
	for _, it := range n.Items {
		addDesignator(it, d)
	}
}

// closeList builds the open list and adds it to its audience.
func (p *enrolParser) closeList() {
	list := p.list
	if list == nil {
		return
	}
	p.list = nil
	switch list.kind {
	case listSubstitute:
		var alts []*types.ReqNode
		for _, it := range list.options[0] {
			alts = append(alts, it.node)
		}
		if alts != nil {
			p.subs = append(p.subs, substitution{code: list.code, alts: alts})
		}
		return
	case listReq:
	default:
		return
	}
	var n *types.ReqNode
	if len(list.options) > 1 {
		// each option's items are all needed; the options are alternatives
		var opts []*types.ReqNode
		for _, items := range list.options {
			opts = append(opts, combineItems(items, modeAnd, 0, false))
		}
		n = orNode(opts...)
	} else {
		n = combineItems(list.options[0], list.mode, list.count, list.forceOr)
	}
	if n == nil {
		return
	}
	if list.oneHalf {
		n = oneHalfReading(n)
	}
	p.addPartSec(list.aud, n, partList, list.sec)
}

// leadHalfCreditRe matches "One of the following courses (0.5 credit)".
var leadHalfCreditRe = regexp.MustCompile(`(?i)\(0\.5\s+credits?\)`)

// oneHalfReading reads a pair of half courses in a list asking for one course
// (0.5 credit) both ways: the pair may be needed, or one may be enough.
func oneHalfReading(n *types.ReqNode) *types.ReqNode {
	return replaceNodes(n, func(t *types.ReqNode) bool {
		if t.Type != types.NodeAnd || len(t.Items) < 2 {
			return false
		}
		return !slices.ContainsFunc(t.Items, func(c *types.ReqNode) bool { return c.Type != types.NodeCourse || courseWeight(c.Code) != 0.5 })
	}, func(t *types.ReqNode) *types.ReqNode {
		return ambiguous(t, orNode(cloneNode(t).Items...), "the list asks for one course (0.5 credit), so one of these half courses may be enough")
	})
}

// combineItems combines a list's items: a trailing "or" on any item but the
// last, or a lead with "one of the following", makes it OR; "two of the
// following groups" makes it choose 2. Sub-items combine the same way.
func combineItems(items []*enrolItem, mode listMode, count int, forceOr bool) *types.ReqNode {
	if len(items) == 0 {
		return nil
	}
	for i, it := range items {
		if i < len(items)-1 && it.conj == "or" {
			mode = modeOr
		}
	}
	if forceOr {
		mode = modeOr
	}
	var nodes []*types.ReqNode
	for _, it := range items {
		n := it.node
		if len(it.children) > 0 {
			n = andNode(n, combineItems(it.children, modeAnd, 0, false))
		}
		if n != nil {
			nodes = append(nodes, n)
		}
	}
	switch mode {
	case modeOr:
		return orNode(nodes...)
	case modeChoose:
		if count >= len(nodes) {
			return andNode(nodes...)
		}
		return &types.ReqNode{Type: types.NodeChoose, Count: count, Items: nodes}
	}
	return mergeAdditional(items, andNode(nodes...))
}

// mergeAdditional reads "An additional 0.5 credit from" a pool named before as
// both amounts from the pool on that alternative, so a course can't count
// twice.
func mergeAdditional(items []*enrolItem, n *types.ReqNode) *types.ReqNode {
	var pools []*types.ReqNode
	for _, it := range items {
		if it.node != nil && it.node.Type == types.NodeCredits {
			pools = append(pools, it.node)
		}
		for _, ch := range it.children {
			if !ch.additional || ch.node == nil || ch.node.Type != types.NodeCredits {
				continue
			}
			for _, pool := range pools {
				if strings.Join(pool.Courses, ",") == strings.Join(ch.node.Courses, ",") {
					ch.node.Amount += pool.Amount
					break
				}
			}
		}
	}
	return n
}

// addPart adds a requirement to an audience, skipping exact duplicates.
func (p *enrolParser) addPart(a *audience, n *types.ReqNode, kind partKind) {
	p.addPartSec(a, n, kind, p.sec)
}

func (p *enrolParser) addPartSec(a *audience, n *types.ReqNode, kind partKind, sec int) {
	if n == nil {
		return
	}
	b, _ := json.Marshal(n)
	for _, pt := range a.parts {
		if c, _ := json.Marshal(pt.node); string(c) == string(b) {
			return
		}
	}
	a.parts = append(a.parts, &part{node: n, sec: sec, kind: kind})
}
