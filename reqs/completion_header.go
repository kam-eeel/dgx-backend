package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Header lines: what a program says about its credits as a whole. "(8.0
// credits, including at least 0.5 credit at the 400-level)", "Required
// Courses (8.0 credits)", "**Total credits: 7.0**", "(14.0-15.5 credits)",
// "7.0 ENG credits including ...". The total becomes Req 1, and the floors
// and caps stated with it become constraint and limit rows under Req 1
// (design 3.1); the assembler numbers and parents them. "Of these:" bullets
// after the total are the same floors and caps on their own lines.
//
// A header is read as a chain of credit phrases, one per amount: the total
// first, then floors ("including at least 2.0 credits at the 300+ level")
// and caps ("no more than 1.0 credit at the 100-level"). The words before an
// amount say what it is ("of which", "including", "no more than"); the words
// after it say which courses it counts ("CRI credits at the 400-level").

// compHeader is what a header line says about the program as a whole.
type compHeader struct {
	total float64 // the stated total; the lower end of a range
	upper float64 // the upper end of a range ("14.0-15.5 credits"), else 0; it stays in raw
	// pool: Req 1's own pool when the total names one ("7.0 ENG credits",
	// "(7.0 credits in Sociology)"): a credits node with filters and no
	// amount. Nil when the total is any credits.
	pool *types.ReqNode
	rows []*types.Requirement // floors and caps stated with the total, in order; unnumbered, no parent
	// tail: sentences and clauses in the header that aren't the total or its
	// floors ("Please note: ...", "Some courses may contribute to both ..."),
	// left to the classifier.
	tail []string
}

var (
	// Markdown emphasis. Every "_" goes, not just paired ones: header lines
	// have no other underscores.
	hdrMarkup = strings.NewReplacer("**", "", "_", "")
	// Labels before a total. Group 1: a "Total" label, after which the unit
	// may be left out ("Total credits: 11.0 of which ...").
	hdrPrefixRe = regexp.MustCompile(`^(?i:(total\s+credits\s*:|total\s*[:=])|program\s+course\s+requirements\s*:|required\s+courses\b|completion\s+of\s+the\s+program\s+requires|students\s+(?:must\s+)?complete(?:\s+a\s+total\s+of)?|a\s+total\s+of|this\s+focus\s+requires\s+(?:the\s+)?completion\s+of|the\s+(?:minor\s+|major\s+|specialist\s+)?program\s+requires)\s*`)
	// A numbered item ("1. 2.0 credits from ..."): never a header.
	hdrListMarkRe = regexp.MustCompile(`^\s*(?:\d+|[a-z])[.)]\s`)
	// A bullet or "Note:" label before a floor line.
	hdrBulletRe = regexp.MustCompile(`^\s*(?:[-*•]\s*)?(?i:notes?\s*:\s*)?`)

	// "At least 1.0 credit of the 2.5 credits must be at the 300-level": the
	// second amount restates what the first is part of.
	hdrScopeRe = regexp.MustCompile(`(?i)\bof\s+the\s+(?:student['’]s\s+)?$`)
	// An amount: "2.0", "14.0-15.5", "13.5 to 14.0", "1.0 or 1.5" (groups 1
	// and 2), "one full-course equivalent", and the "one" of "one of which
	// must be WGS460Y1" (group 3), whose amount is that course's weight.
	// Numbers have at most two digits before the point, so levels ("300")
	// and course numbers are never amounts.
	hdrAmountRe = regexp.MustCompile(`\b(?:(\d{1,2}(?:\.\d+)?)(?:\s*(?:-|–|to\b|or\b)\s*(\d{1,2}(?:\.\d+)?))?|(?i:(?:one|a)\s+full[- ]course\s+equivalent)|(?i:(one))\s+of\s+which)\b`)

	// Words around a total that say nothing about it: "7.0 credits
	// specified as follows:", "4.0 credits chosen from those listed below",
	// "2.5 credits (total)", "requires 7.0 credits, meeting the following
	// requirements:", "Students complete 7.0 credits, including:".
	hdrFillerRe   = regexp.MustCompile(`(?i)\b(credits?)\s+(?:(?:specified\s+)?as\s+follows|chosen\s+from\s+those\s+listed\s+below|\(\s*total\s*\))`)
	hdrMeetingRe  = regexp.MustCompile(`(?i),?\s*meeting\s+the\s+following\s+requirements\b`)
	hdrDanglingRe = regexp.MustCompile(`(?i),?\s*including\s*:?\s*$`)
	hdrAsNotedRe  = regexp.MustCompile(`(?i),?\s*as\s+noted\s+below\b`)
	// "(7.0 credits, including PHL100Y1/ PHL101Y1 if taken)": the course
	// counts toward the total when taken; it isn't required.
	hdrIfTakenRe = regexp.MustCompile(`,?\s*(?i:including)\s+` + codePat + `(?:\s*/\s*` + codePat + `)*\s+(?i:if\s+taken)\b`)
	// "(10.5 or 11.0 credits plus a disciplinary focus requiring 2.0-3.5
	// credits)": the focus is another program.
	hdrPlusRe = regexp.MustCompile(`(?i)\s+(plus\s+.+)$`)
	// "**Completion Requirements:** HIS courses to a total of 4.0 credits":
	// the designators are the total's pool (design 3.1).
	hdrToTotalRe = regexp.MustCompile(`^(?i:completion\s+requirements\s*:\s*)?([A-Z]{3}(?:\s*(?:/|,|(?i:or|and))\s*[A-Z]{3})*)\s+(?i:courses\s+to\s+a\s+total\s+of)\s+`)
	// "(7.0 credits in Sociology)": a subject naming the total's pool.
	hdrSubjectRe = regexp.MustCompile(`\b((?i:credits?))\s+in\s+([A-Z][a-z]+(?:\s+[A-Z][a-z]+)*)`)
	hdrFromRe    = regexp.MustCompile(`(?i)\bfrom\b`)
	// A remark closing a floor sentence: "... must be at the 400-level
	// (includes Fourth Year Course requirement)."
	hdrAsideRe = regexp.MustCompile(`\s*(\([^()]*\))\.?$`)

	hdrWordRe     = regexp.MustCompile(`[A-Za-z]+`)
	hdrLevelTokRe = regexp.MustCompile(`[1-4]00\s*\+?`)
	hdrDesigRe    = regexp.MustCompile(`^[A-Z]{3}$`)
	// "the program’s completion"
	hdrPossessiveRe = regexp.MustCompile(`['’]s\b`)
	// "0.5 of which must be ...", "1.0 credit of which must be ...": the
	// phrase narrows the one before it.
	hdrOfWhichRe = regexp.MustCompile(`^\s*(?i:credits?\s+)?(?i:of\s+which)\b`)
)

// hdrIntroWords may come between one credit phrase and the amount of the
// next: "including at least", ", of which a", "and of which at least",
// "with a minimum of", "no more than", "of which up to", "must include".
var hdrIntroWords = map[string]bool{
	"and": true, "with": true, "including": true, "include": true, "of": true,
	"which": true, "these": true, "at": true, "least": true, "a": true,
	"an": true, "minimum": true, "maximum": true, "no": true, "more": true,
	"than": true, "only": true, "one": true, "must": true, "up": true,
	"to": true, "all": true,
}

// hdrUnitWords name what an amount counts.
var hdrUnitWords = map[string]bool{"credit": true, "credits": true, "course": true, "courses": true}

// hdrBodyWords may follow an amount without changing which courses it
// counts: "credit must be at the 400-level", "of 100-level courses may be
// used to fulfill the program requirements", "of 100 level courses may count
// toward the program’s completion", "cognate credit may be taken in PRT".
// Any other word is a condition the grammar can't check.
var hdrBodyWords = map[string]bool{
	"full": true, "equivalent": true, "equivalents": true, "their": true,
	"at": true, "the": true, "a": true, "an": true, "of": true, "which": true,
	"must": true, "be": true, "from": true, "in": true, "level": true,
	"levels": true, "series": true, "or": true, "and": true, "above": true,
	"higher": true, "least": true, "is": true, "are": true, "can": true,
	"may": true, "used": true, "to": true, "fulfill": true, "fulfil": true,
	"program": true, "requirements": true, "taken": true, "should": true,
	"cognate": true, "these": true, "all": true, "with": true,
	"including": true, "include": true, "minimum": true, "maximum": true,
	"only": true, "no": true, "more": true, "than": true, "up": true,
	"count": true, "toward": true, "towards": true, "completion": true,
}

// hdrPhrase is one credit phrase of a header: an amount, the words that
// introduce it and the words that qualify it.
type hdrPhrase struct {
	intro  string // ", of which at least", "no more than"
	body   string // " credits at the 300+ level"
	raw    string // the phrase as written, without separators
	amount float64
	upper  float64 // a range or alternative amounts: "14.0-15.5", "1.0 or 1.5"
	one    bool    // "one of which must be WGS460Y1": the amount is the course's weight
	cap    bool    // "no more than", "a maximum of", "only", "up to"
	parent int     // the phrase this one narrows; -1 is the total (on a floor line, the program)
	node   *types.ReqNode
	text   bool // the body has conditions the grammar can't check
}

// parseHeader reads a header line: its total, the floors and caps stated
// with it, and the sentences it leaves to the classifier. It reports false
// when s isn't a header: no total at its start, or the amount is an item's
// ("2.0 credits from: SLA117H1; ...", "0.5 credit at the 400-level").
func parseHeader(s string) (h compHeader, ok bool) {
	s = hdrClean(s)
	if hdrListMarkRe.MatchString(s) {
		return h, false
	}
	var toTotal []string
	if m := hdrToTotalRe.FindStringSubmatch(s); m != nil {
		toTotal = desigRe.FindAllString(m[1], -1)
		s = s[len(m[0]):]
	}
	core, after, labelled := hdrUnwrap(s)
	sents := hdrSentences(core)
	if len(sents) == 0 {
		return h, false
	}
	first, aside := hdrStripTotalClauses(sents[0])
	phrases, lead := hdrPhrases(first)
	if len(phrases) == 0 || lead != "" || strings.TrimSpace(phrases[0].intro) != "" || phrases[0].one {
		return h, false
	}
	tot := phrases[0]
	pool, text, ok := hdrTotalPool(tot.body, labelled, toTotal)
	if !ok {
		return h, false
	}
	h.total, h.upper, h.tail = tot.amount, tot.upper, aside
	if pool.Designators != nil || pool.Levels != nil {
		h.pool = pool
	}
	if text { // "2.0 PRT credits at the 200+ level taught in Portuguese"
		n := hdrCloneNode(pool)
		n.Amount = h.total
		h.rows = append(h.rows, hdrTextRow(n, tot.raw))
	}

	c := &hdrChain{root: h.pool}
	c.read(phrases[1:])
	for _, p := range c.phrases {
		if p.cap && p.text {
			h.tail = append(h.tail, p.raw) // a cap over a pool the grammar can't name
			continue
		}
		h.rows = append(h.rows, p.row())
	}
	for _, sent := range sents[1:] {
		rows, aside, ok := c.sentence(sent)
		if !ok {
			h.tail = append(h.tail, sent)
			continue
		}
		h.rows = append(h.rows, rows...)
		h.tail = append(h.tail, aside...)
	}
	if after != "" {
		h.tail = append(h.tail, hdrSentences(after)...)
	}
	return h, true
}

// hdrStripTotalClauses removes from a header's first sentence the words that
// say nothing about the total ("specified as follows", "(total)", "meeting
// the following requirements", a dangling "including:") and the clauses
// that aren't requirements ("including PHL100Y1/ PHL101Y1 if taken", "plus a
// disciplinary focus ..."), which it returns for the tail.
func hdrStripTotalClauses(s string) (string, []string) {
	var aside []string
	if m := hdrIfTakenRe.FindString(s); m != "" {
		aside = append(aside, strings.TrimLeft(m, " ,"))
		s = strings.Replace(s, m, "", 1)
	}
	if m := hdrPlusRe.FindStringSubmatchIndex(s); m != nil {
		aside = append(aside, s[m[2]:m[3]])
		s = s[:m[0]]
	}
	s = hdrFillerRe.ReplaceAllString(s, "$1")
	s = hdrMeetingRe.ReplaceAllString(s, "")
	s = hdrDanglingRe.ReplaceAllString(s, "")
	return hdrAsNotedRe.ReplaceAllString(s, ""), aside
}

// hdrTotalPool reads the words after the total ("ENG credits", "credits in
// Sociology", "PRT credits at the 200+ level taught in Portuguese") into
// Req 1's pool, adding desigs (from "HIS courses to a total of"). text says
// the words name a condition the grammar can't check. It reports false when
// the amount isn't a total: an item's pool ("credits from the following
// list"), no unit ("2.0", unless labelled "Total credits:"), a level with no
// designator ("0.5 credit at the 400-level"), or a phrase cut short
// ("credits, of which").
func hdrTotalPool(body string, labelled bool, desigs []string) (pool *types.ReqNode, text, ok bool) {
	if m := hdrSubjectRe.FindStringSubmatchIndex(body); m != nil {
		// A subject with a designator is the pool; any other ("Visual
		// Studies") names the program, and its items are the pool.
		if d := subjectDesignators[strings.ToLower(body[m[4]:m[5]])]; d != "" {
			desigs = append(desigs, d)
		}
		body = body[:m[0]] + body[m[2]:m[3]] + body[m[1]:]
	}
	if hdrFromRe.MatchString(body) || codeRe.MatchString(body) {
		return nil, false, false // an item: "2.5 credits from the following list:"
	}
	words := hdrWordRe.FindAllString(body, -1)
	if len(words) > 0 && hdrIntroWords[strings.ToLower(words[len(words)-1])] {
		return nil, false, false // "(7.0 credits, of which)"
	}
	pool, unit, text := hdrReadBody(body)
	pool.Designators = dedupe(append(pool.Designators, desigs...))
	noUnit := !unit && !labelled
	levelOnly := pool.Levels != nil && pool.Designators == nil
	if noUnit || levelOnly {
		return nil, false, false
	}
	return pool, text, true
}

// parseHeaderFloors reads a line of program-level floors and caps: an "Of
// these:" bullet after the total ("- 2.0 credits at the 300+ level including
// a 0.5 credit at the 400 level.", "- No more than 1.0 credit of 100-level
// courses may be used to fulfill the program requirements.") or a line like
// it ("At least 5.0 credits must be NMC and/or NML courses."). The first
// sentence must be read in full; a later sentence that can't be is returned
// in tail.
func parseHeaderFloors(s string) (rows []*types.Requirement, tail []string, ok bool) {
	s = hdrClean(s)
	if hdrListMarkRe.MatchString(s) {
		return nil, nil, false
	}
	s = s[len(hdrBulletRe.FindString(s)):]
	c := &hdrChain{}
	for i, sent := range hdrSentences(s) {
		r, aside, ok := c.sentence(sent)
		switch {
		case ok:
			rows = append(rows, r...)
			tail = append(tail, aside...)
		case i == 0:
			return nil, nil, false
		default:
			tail = append(tail, sent)
		}
	}
	return rows, tail, rows != nil
}

// hdrChain is the credit phrases after a total, read in order: a phrase may
// narrow an earlier one ("of which", "including").
type hdrChain struct {
	root    *types.ReqNode // the total's pool (nil = any credits)
	phrases []*hdrPhrase
}

// sentence reads a sentence that is only credit phrases ("Of these, at
// least 1.0 credit must be at the 300+ level.") and returns their rows, and
// a closing remark in parentheses it doesn't read ("(includes Fourth Year
// Course requirement)"). It reports false, reading nothing, when the
// sentence is anything else or has conditions the grammar can't check.
func (c *hdrChain) sentence(s string) (rows []*types.Requirement, aside []string, ok bool) {
	s = hdrAsNotedRe.ReplaceAllString(s, "")
	if m := hdrAsideRe.FindStringSubmatchIndex(s); m != nil && !hdrAmountRe.MatchString(s[m[2]:m[3]]) {
		aside = []string{s[m[2]:m[3]]}
		s = s[:m[0]]
	}
	phrases, lead := hdrPhrases(s)
	if len(phrases) == 0 || lead != "" {
		return nil, nil, false
	}
	n := len(c.phrases)
	c.read(phrases)
	for _, p := range c.phrases[n:] {
		if p.text {
			c.phrases = c.phrases[:n]
			return nil, nil, false
		}
		rows = append(rows, p.row())
	}
	return rows, aside, true
}

// read adds phrases to the chain: it decides what each one narrows, and
// builds its credits node.
func (c *hdrChain) read(phrases []*hdrPhrase) {
	for _, p := range phrases {
		k := len(c.phrases)
		words := map[string]bool{}
		for _, w := range hdrWordRe.FindAllString(strings.ToLower(p.intro), -1) {
			words[w] = true
		}
		p.cap = words["than"] || words["maximum"] || words["only"] || words["up"]
		ofWhich := words["which"] || words["these"] || hdrOfWhichRe.MatchString(p.body)
		including := words["including"] || words["include"] || words["with"]
		// What a phrase narrows (its parent; -1 is the total):
		//
		//	intro                   parent                          example
		//	"of which", "of these"  the last floor, walking up      "2.0 credits at the 300+ level, of which
		//	(or "N of which")       past caps to their parent       a 0.5 credit must be at the 400-level";
		//	                                                        "no more than 1.0 credit at the 100-level
		//	                                                        and of which at least 2.0 ..." (the total)
		//	"including", "with"     the phrase just before          "4.0 credits at the 300+ level including
		//	                                                        1.0 credit at the 400 level"
		//	"including" after a     the cap's parent: a cap         "no more than 1.0 credit in SPA courses,
		//	cap                     holds no floors                 including 1.0 credit at the 300+ level"
		//	anything else ("and",   the parent of the phrase        "at least 1.5 credits at the 300-level
		//	",", "at least", "All") just before (a sibling)         and 1.0 credit at the 400-level"
		p.parent = -1
		if k > 0 {
			prev := c.phrases[k-1]
			switch {
			case ofWhich:
				q := k - 1
				for q >= 0 && c.phrases[q].cap {
					q = c.phrases[q].parent
				}
				p.parent = q
			case including && prev.cap:
				p.parent = prev.parent
			case including:
				p.parent = k - 1
			default:
				p.parent = prev.parent
			}
		}
		n, unit, text := hdrReadBody(p.body)
		p.text = text
		if p.one {
			// "one of which must be WGS460Y1"; with alternatives ("ENG400H1
			// or ENG401H1"), the lightest one is enough.
			if len(n.Courses) == 0 {
				p.text = true
			}
			for i, code := range n.Courses {
				if w := courseWeight(code); i == 0 || w < p.amount {
					p.amount = w
				}
			}
		}
		if !unit && n.Designators == nil && n.Levels == nil && n.Courses == nil {
			p.text = true // an amount of nothing: "requirements 4 and 5"
		}
		// A cap keeps only its own filters: a limit under Req 1 already
		// counts only what Req 1 counts (its pool and children), and a
		// cognate cap ("up to 1.0 credit may be taken in SPA/LAS
		// courses") must not be narrowed by the pool it lies outside of.
		// A floor naming courses keeps them alone: adding designators
		// would widen it, since sources combine with "or".
		if !p.cap && n.Courses == nil {
			c.narrow(n, p.parent)
		}
		p.node = n
		c.phrases = append(c.phrases, p)
	}
}

// narrow gives n the filters of the phrase it narrows, where they agree:
// "2.0 SDS credits at the 300+ level, 1.0 credit of which must be at the
// 400-level" is 1.0 SDS credit at the 400-level. Filters that contradict
// ("at the 300 level, including 0.5 credit at the 400 level") are the
// calendar's slip, and n keeps its own.
func (c *hdrChain) narrow(n *types.ReqNode, parent int) {
	from := c.root
	if parent >= 0 {
		from = c.phrases[parent].node
	}
	if from == nil {
		return
	}
	// narrow may return the parent's slice; each row gets its own.
	if d, ok := narrow(from.Designators, n.Designators); ok {
		n.Designators = slices.Clone(d)
	}
	if l, ok := narrow(from.Levels, n.Levels); ok {
		n.Levels = slices.Clone(l)
	}
}

// row returns the phrase's requirement row: a floor is a constraint, a cap
// a limit.
func (p *hdrPhrase) row() *types.Requirement {
	n := hdrCloneNode(p.node)
	if p.cap {
		n.MaxAmount = maxAmount(p.amount)
		return &types.Requirement{Kind: types.RequirementLimit, Req: n, Raw: p.raw}
	}
	n.Amount = p.amount
	if p.text {
		return hdrTextRow(n, p.raw)
	}
	return &types.Requirement{Kind: types.RequirementConstraint, Req: n, Raw: p.raw}
}

// hdrCloneNode copies a credits node and its filter slices, so rows and the
// pool share no backing arrays (cloneNode copies only the struct).
func hdrCloneNode(n *types.ReqNode) *types.ReqNode {
	c := cloneNode(n)
	c.Designators = slices.Clone(n.Designators)
	c.Levels = slices.Clone(n.Levels)
	c.Courses = slices.Clone(n.Courses)
	return c
}

// hdrTextRow is a floor with conditions the grammar can't check ("2.0
// credits in Hungarian language study"): its credits, and the phrase as
// text.
func hdrTextRow(n *types.ReqNode, raw string) *types.Requirement {
	text := &types.ReqNode{Type: types.NodeText, Kind: types.TextOther, Text: raw}
	return &types.Requirement{Kind: types.RequirementConstraint, Req: andNode(n, text), Raw: raw}
}

// hdrPhrases splits a sentence into credit phrases, one per amount. lead is
// what comes before the first phrase's intro words; a header or floor line
// has none.
func hdrPhrases(s string) (phrases []*hdrPhrase, lead string) {
	locs := hdrAmountRe.FindAllStringSubmatchIndex(s, -1)
	start, prevEnd := 0, 0
	for _, m := range locs {
		if hdrScopeRe.MatchString(s[:m[0]]) {
			continue // "of the 2.5 credits": the scope, not another amount
		}
		end := m[1]
		if m[6] >= 0 {
			end = m[7] // "one", of "one of which"
		}
		body, intro := hdrSplitIntro(s[prevEnd:m[0]])
		if len(phrases) == 0 {
			lead = strings.TrimSpace(body)
		} else {
			p := phrases[len(phrases)-1]
			p.body = body
			p.raw = hdrTrimRaw(s[start : prevEnd+len(body)])
		}
		p := &hdrPhrase{intro: intro}
		switch {
		case m[2] >= 0:
			p.amount, _ = strconv.ParseFloat(s[m[2]:m[3]], 64)
			if m[4] >= 0 {
				p.upper, _ = strconv.ParseFloat(s[m[4]:m[5]], 64)
			}
			switch {
			case p.upper == p.amount:
				p.upper = 0
			case p.upper > 0 && p.upper < p.amount: // "15.5-14.0"
				p.amount, p.upper = p.upper, p.amount
			}
		case m[6] >= 0:
			p.one = true
		default:
			p.amount = 1 // one full-course equivalent
		}
		phrases = append(phrases, p)
		start, prevEnd = m[0]-len(intro), end
	}
	if len(phrases) == 0 {
		return nil, strings.TrimSpace(s)
	}
	p := phrases[len(phrases)-1]
	p.body = s[prevEnd:]
	p.raw = hdrTrimRaw(s[start:])
	return phrases, lead
}

// hdrSplitIntro splits the text between two amounts into the first one's
// qualifiers and the second one's intro words: " credits at the 300+ level,
// of which a " gives " credits at the 300+ level" and ", of which a ".
func hdrSplitIntro(b string) (body, intro string) {
	i := len(b)
	for {
		j := i
		for j > 0 && b[j-1] == ' ' {
			j--
		}
		if j > 0 && strings.IndexByte(",;(:", b[j-1]) >= 0 {
			i = j - 1
			continue
		}
		k := j
		for k > 0 && hdrIsLetter(b[k-1]) {
			k--
		}
		word := b[k:j]
		wholeWord := k == 0 || b[k-1] == ' ' || strings.IndexByte(",;(:", b[k-1]) >= 0 // not the "level" of "400-level"
		if word == "" || !hdrIntroWords[strings.ToLower(word)] || !wholeWord {
			return b[:i], b[i:]
		}
		i = k
	}
}

// hdrReadBody reads the qualifiers after an amount: designators, levels and
// course codes. unit says whether it names credits or courses; text, that
// it has words naming a condition the grammar can't check ("in Hungarian
// language study", "taught in Portuguese").
func hdrReadBody(body string) (n *types.ReqNode, unit, text bool) {
	n = &types.ReqNode{Type: types.NodeCredits}
	n.Courses = codeRe.FindAllString(body, -1)
	rest := codeRe.ReplaceAllString(body, " ")
	n.Levels = parseLevels(rest)
	rest = hdrLevelTokRe.ReplaceAllString(rest, " ")
	rest = hdrPossessiveRe.ReplaceAllString(rest, "")
	for _, w := range hdrWordRe.FindAllString(rest, -1) {
		lw := strings.ToLower(w)
		switch {
		case hdrDesigRe.MatchString(w):
			n.Designators = append(n.Designators, w)
		case hdrUnitWords[lw]:
			unit = true
		case !hdrBodyWords[lw]:
			text = true
		}
	}
	n.Designators = dedupe(n.Designators)
	return n, unit, text
}

// hdrClean drops markdown emphasis and extra spaces.
func hdrClean(s string) string {
	return strings.TrimSpace(wsRe.ReplaceAllString(hdrMarkup.Replace(s), " "))
}

// hdrUnwrap removes what wraps a header's total: labels ("Total:",
// "Required Courses", "This Focus requires completion of") and the
// parentheses around it. It returns the text inside, any text after the
// closing parenthesis, and whether a "Total" label means the unit may be
// left out ("Total credits: 7.0").
func hdrUnwrap(s string) (core, after string, labelled bool) {
	// Twice: a label may sit outside the parentheses ("Required Courses
	// (8.0 credits)") or inside them ("(Total: 2.0 credits)").
	for range 2 {
		if m := hdrPrefixRe.FindStringSubmatch(s); m != nil {
			s = s[len(m[0]):]
			labelled = labelled || m[1] != ""
		}
		t := strings.TrimRight(s, " .:")
		if !strings.HasPrefix(t, "(") {
			continue
		}
		if strings.HasSuffix(t, ")") {
			// Not by matching: "(... categorized as follows: a) Politics and
			// Economics b) ...)" closes early.
			s = t[1 : len(t)-1]
			continue
		}
		depth := 0
		for i := 0; i < len(t); i++ {
			if t[i] == '(' {
				depth++
			} else if t[i] == ')' {
				if depth--; depth == 0 {
					after = strings.TrimSpace(strings.TrimLeft(s[i+1:], " .,;:)"))
					s = t[1:i]
					break
				}
			}
		}
	}
	return strings.TrimSpace(s), after, labelled
}

// hdrSentences splits s at sentence ends (". " before a capital or a digit,
// or "." glued to the next amount) and semicolons outside parentheses. A
// sentence keeps its full stop.
func hdrSentences(s string) []string {
	var out []string
	add := func(t string) {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth = max(depth-1, 0)
		case ';':
			if depth == 0 {
				add(s[start:i])
				start = i + 1
			}
		case '.':
			next := i + 1
			if next < len(s) && s[next] == ' ' {
				next++
			}
			if depth > 0 || next >= len(s) {
				continue
			}
			word := s[strings.LastIndexByte(s[:i], ' ')+1 : i]
			abbrev := strings.Contains(word, ".") // "e.g. Anthropology"
			digit := s[next] >= '0' && s[next] <= '9'
			capital := s[next] >= 'A' && s[next] <= 'Z'
			spaced := next == i+2 // ". Some", ". 1.0"
			// "courses.1.0 credit may be ...": a missing space after a word,
			// not a decimal point ("7.0").
			glued := next == i+1 && i > 0 && hdrIsLetter(s[i-1]) && digit
			if (spaced || glued) && (capital || digit) && !abbrev {
				add(s[start : i+1])
				start = i + 1
			}
		}
	}
	add(s[start:])
	return out
}

// hdrTrimRaw trims a phrase to its words: no separators, no "and" before
// it, no full stop or unmatched parenthesis after it.
func hdrTrimRaw(s string) string {
	s = strings.TrimLeft(s, " ,;:(")
	s = strings.TrimPrefix(s, "and ")
	s = strings.TrimRight(s, " ,;:.")
	for strings.HasSuffix(s, ")") && strings.Count(s, ")") > strings.Count(s, "(") {
		s = strings.TrimRight(strings.TrimSuffix(s, ")"), " ,;:.")
	}
	return s
}

// hdrIsLetter reports whether b is an ASCII letter.
func hdrIsLetter(b byte) bool { return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' }
