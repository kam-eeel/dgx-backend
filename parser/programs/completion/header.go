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

// Header lines: a program's total, which becomes Req 1, and the floors and caps
// stated with it, which become constraint and limit rows under Req 1.

// compHeader is what a header line says about the program as a whole.
type compHeader struct {
	total float64 // the stated total; the lower end of a range
	upper float64 // the upper end of a range ("14.0-15.5 credits"), else 0; it stays in raw
	// Req 1's own pool when the total names one ("7.0 ENG credits"): a credits
	// node with filters and no amount. Nil when the total is any credits.
	pool *types.ReqNode
	rows []*types.Requirement // floors and caps stated with the total, in order; unnumbered, no parent
	// sentences and clauses that aren't the total or its floors, left to the
	// classifier
	tail []string
}

var (
	// hdrPrefixRe matches a label before a total: "Required Courses". Group 1
	// is a "Total" label, after which the unit may be left out.
	hdrPrefixRe = regexp.MustCompile(`^(?i:(total\s+credits\s*:|total\s*[:=])|program\s+course\s+requirements\s*:|required\s+courses\b|completion\s+of\s+the\s+program\s+requires|students\s+(?:must\s+)?complete(?:\s+a\s+total\s+of)?|a\s+total\s+of|this\s+focus\s+requires\s+(?:the\s+)?completion\s+of|the\s+(?:minor\s+|major\s+|specialist\s+)?program\s+requires)\s*`)
	// hdrListMarkRe matches a numbered item's marker, never a header's: "1. ".
	hdrListMarkRe = regexp.MustCompile(`^\s*(?:\d+|[a-z])[.)]\s`)
	// hdrBulletRe matches a bullet or "Note:" label before a floor line: "- ".
	hdrBulletRe = regexp.MustCompile(`^\s*(?:[-*•]\s*)?(?i:notes?\s*:\s*)?`)

	// hdrScopeRe matches "of the" before an amount that restates the scope, not
	// a new phrase: "1.0 credit of the 2.5 credits".
	hdrScopeRe = regexp.MustCompile(`(?i)\bof\s+the\s+(?:student['’]s\s+)?$`)
	// hdrAmountRe matches an amount, "1.0 or 1.5": groups 1 and 2 are its ends,
	// group 3 the "one" of "one of which". At most two digits before the point
	// keep levels and course numbers out.
	hdrAmountRe = regexp.MustCompile(`\b(?:(\d{1,2}(?:\.\d+)?)(?:\s*(?:-|–|to\b|or\b)\s*(\d{1,2}(?:\.\d+)?))?|(?i:(?:one|a)\s+full[- ]course\s+equivalent)|(?i:(one))\s+of\s+which)\b`)

	// Words around a total that say nothing about it: "4.0 credits chosen from
	// those listed below", "meeting the following requirements", a dangling
	// "including:", "as noted below".
	hdrFillerRe   = regexp.MustCompile(`(?i)\b(credits?)\s+(?:(?:specified\s+)?as\s+follows|chosen\s+from\s+those\s+listed\s+below|\(\s*total\s*\))`)
	hdrMeetingRe  = regexp.MustCompile(`(?i),?\s*meeting\s+the\s+following\s+requirements\b`)
	hdrDanglingRe = regexp.MustCompile(`(?i),?\s*including\s*:?\s*$`)
	hdrAsNotedRe  = regexp.MustCompile(`(?i),?\s*as\s+noted\s+below\b`)
	// hdrIfTakenRe matches a course that counts toward the total when taken,
	// not a required one: "including PHL100Y1/ PHL101Y1 if taken".
	hdrIfTakenRe = regexp.MustCompile(`,?\s*(?i:including)\s+` + grammar.CodePat + `(?:\s*/\s*` + grammar.CodePat + `)*\s+(?i:if\s+taken)\b`)
	// hdrPlusRe matches another program added to the total: "plus a
	// disciplinary focus requiring 2.0-3.5 credits".
	hdrPlusRe = regexp.MustCompile(`(?i)\s+(plus\s+.+)$`)
	// hdrToTotalRe matches designators that are the total's pool: "HIS courses
	// to a total of 4.0 credits".
	hdrToTotalRe = regexp.MustCompile(`^(?i:completion\s+requirements\s*:\s*)?([A-Z]{3}(?:\s*(?:/|,|(?i:or|and))\s*[A-Z]{3})*)\s+(?i:courses\s+to\s+a\s+total\s+of)\s+`)
	// hdrSubjectRe matches a subject that may name the total's pool: "credits
	// in Sociology". Group 1 is the unit, group 2 the subject.
	hdrSubjectRe = regexp.MustCompile(`\b((?i:credits?))\s+in\s+([A-Z][a-z]+(?:\s+[A-Z][a-z]+)*)`)
	hdrFromRe    = regexp.MustCompile(`(?i)\bfrom\b`)
	// hdrAsideRe matches a remark closing a floor sentence: "(includes Fourth
	// Year Course requirement)."
	hdrAsideRe    = regexp.MustCompile(`\s*(\([^()]*\))\.?$`)
	hdrLevelTokRe = regexp.MustCompile(grammar.LevelPat + `\s*\+?`)
	// hdrPossessiveRe matches a possessive: "the program’s completion"
	hdrPossessiveRe = regexp.MustCompile(`['’]s\b`)
	// hdrOfWhichRe matches a body that narrows the phrase before it: "credit of
	// which must be".
	hdrOfWhichRe = regexp.MustCompile(`^\s*(?i:credits?\s+)?(?i:of\s+which)\b`)
)

// hdrIntroWords may come between one credit phrase and the amount of the
// next: ", of which at least".
var hdrIntroWords = map[string]bool{
	"and": true, "with": true, "including": true, "include": true, "of": true,
	"which": true, "these": true, "at": true, "least": true, "a": true,
	"an": true, "minimum": true, "maximum": true, "no": true, "more": true,
	"than": true, "only": true, "one": true, "must": true, "up": true,
	"to": true, "all": true,
}

// hdrUnitWords name what an amount counts.
var hdrUnitWords = map[string]bool{"credit": true, "credits": true, "course": true, "courses": true}

// hdrBodyWords may follow an amount without changing which courses it counts:
// "credit must be at the 400-level". Any other word is a condition the grammar
// can't check.
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

// parseHeader reads a header line: its total, the floors and caps stated with
// it, and the sentences it leaves to the classifier. It reports false when s
// isn't a header: no total at its start, or the amount is an item's.
func parseHeader(s string) (h compHeader, ok bool) {
	s = hdrClean(s)
	if hdrListMarkRe.MatchString(s) {
		return h, false
	}
	var toTotal []string
	if m := hdrToTotalRe.FindStringSubmatch(s); m != nil {
		toTotal = grammar.DesigRe.FindAllString(m[1], -1)
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
		n := grammar.CloneNode(pool)
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
// say nothing about the total, and returns the clauses that aren't requirements
// ("plus a disciplinary focus ...") for the tail.
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

// hdrTotalPool reads the words after the total into Req 1's pool, with desigs;
// text says they name a condition the grammar can't check. It reports false for
// an item's pool, no unit unless labelled, a lone level or a cut phrase.
func hdrTotalPool(body string, labelled bool, desigs []string) (pool *types.ReqNode, text, ok bool) {
	if m := hdrSubjectRe.FindStringSubmatchIndex(body); m != nil {
		// a subject with a designator is the pool; any other ("Visual Studies")
		// names the program, and its items are the pool
		if d := grammar.SubjectDesignators[strings.ToLower(body[m[4]:m[5]])]; d != "" {
			desigs = append(desigs, d)
		}
		body = body[:m[0]] + body[m[2]:m[3]] + body[m[1]:]
	}
	if hdrFromRe.MatchString(body) || grammar.CodeRe.MatchString(body) {
		return nil, false, false // an item: "2.5 credits from the following list:"
	}
	words := grammar.WordRe.FindAllString(body, -1)
	if len(words) > 0 && hdrIntroWords[strings.ToLower(words[len(words)-1])] {
		return nil, false, false // "(7.0 credits, of which)"
	}
	pool, unit, text := hdrReadBody(body)
	pool.Designators = grammar.Dedupe(append(pool.Designators, desigs...))
	noUnit := !unit && !labelled
	levelOnly := pool.Levels != nil && pool.Designators == nil
	if noUnit || levelOnly {
		return nil, false, false
	}
	return pool, text, true
}

// parseHeaderFloors reads a line of program-level floors and caps: an "Of
// these:" bullet after the total, or a line like it. The first sentence must be
// read in full; a later sentence that can't be is returned in tail.
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

// sentence reads a sentence that is only credit phrases and returns their rows,
// and a closing remark in brackets it doesn't read. It reports false, reading
// nothing, when the sentence is anything else or has conditions it can't check.
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

// read adds phrases to the chain: what each narrows, and its credits node.
func (c *hdrChain) read(phrases []*hdrPhrase) {
	for _, p := range phrases {
		k := len(c.phrases)
		words := map[string]bool{}
		for _, w := range grammar.WordRe.FindAllString(strings.ToLower(p.intro), -1) {
			words[w] = true
		}
		p.cap = words["than"] || words["maximum"] || words["only"] || words["up"]
		ofWhich := words["which"] || words["these"] || hdrOfWhichRe.MatchString(p.body)
		including := words["including"] || words["include"] || words["with"]
		// parent (-1 is the total): "of which" narrows the last floor past
		// caps, "including" the phrase before (a cap's parent: caps hold no
		// floors), and anything else is a sibling of the phrase before
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
			// with alternatives, the lightest one is enough
			if len(n.Courses) == 0 {
				p.text = true
			}
			for i, code := range n.Courses {
				if w := grammar.CourseWeight(code); i == 0 || w < p.amount {
					p.amount = w
				}
			}
		}
		if !unit && n.Designators == nil && n.Levels == nil && n.Courses == nil {
			p.text = true // an amount of nothing: "requirements 4 and 5"
		}
		// a cap keeps only its own filters: Req 1's limits count only what Req
		// 1 counts, and a cognate cap lies outside the pool; a floor naming
		// courses keeps them alone, as designators would widen it
		if !p.cap && n.Courses == nil {
			c.narrow(n, p.parent)
		}
		p.node = n
		c.phrases = append(c.phrases, p)
	}
	// a floor inside another counts toward it, so the outer floor takes its
	// levels too
	for _, p := range c.phrases {
		if p.parent < 0 || p.cap || p.node == nil || p.node.Levels == nil {
			continue
		}
		if q := c.phrases[p.parent].node; q != nil && !c.phrases[p.parent].cap && q.Levels != nil &&
			slices.ContainsFunc(p.node.Levels, func(l int) bool { return !slices.Contains(q.Levels, l) }) {
			q.Levels = grammar.Dedupe(append(slices.Clone(q.Levels), p.node.Levels...))
			slices.Sort(q.Levels)
		}
	}
}

// narrow gives n the filters of the phrase it narrows where they agree. Where
// they contradict, the calendar slipped and n keeps its own.
func (c *hdrChain) narrow(n *types.ReqNode, parent int) {
	from := c.root
	if parent >= 0 {
		from = c.phrases[parent].node
	}
	if from == nil {
		return
	}
	// narrow may return the parent's slice
	if d, ok := grammar.Narrow(from.Designators, n.Designators); ok {
		n.Designators = slices.Clone(d)
	}
	if l, ok := grammar.Narrow(from.Levels, n.Levels); ok {
		n.Levels = slices.Clone(l)
	}
}

// row returns the phrase's row: a floor is a constraint, a cap a limit.
func (p *hdrPhrase) row() *types.Requirement {
	n := grammar.CloneNode(p.node)
	if p.cap {
		n.MaxAmount = grammar.MaxAmount(p.amount)
		return &types.Requirement{Kind: types.RequirementLimit, Req: n, Raw: p.raw}
	}
	n.Amount = p.amount
	if p.text {
		return hdrTextRow(n, p.raw)
	}
	return &types.Requirement{Kind: types.RequirementConstraint, Req: n, Raw: p.raw}
}

// hdrTextRow is a floor with conditions the grammar can't check: its credits,
// and the phrase as text.
func hdrTextRow(n *types.ReqNode, raw string) *types.Requirement {
	text := &types.ReqNode{Type: types.NodeText, Kind: types.TextOther, Text: raw}
	return &types.Requirement{Kind: types.RequirementConstraint, Req: programs.AndNode(n, text), Raw: raw}
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
// qualifiers and the second one's intro words: " credits at the 300+ level" and
// ", of which a ".
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

// hdrReadBody reads the qualifiers after an amount. unit says whether it names
// credits or courses; text, that it names a condition the grammar can't check.
func hdrReadBody(body string) (n *types.ReqNode, unit, text bool) {
	n = &types.ReqNode{Type: types.NodeCredits}
	n.Courses = grammar.CodeRe.FindAllString(body, -1)
	rest := grammar.CodeRe.ReplaceAllString(body, " ")
	n.Levels = grammar.ParseLevels(rest)
	rest = hdrLevelTokRe.ReplaceAllString(rest, " ")
	rest = hdrPossessiveRe.ReplaceAllString(rest, "")
	for _, w := range grammar.WordRe.FindAllString(rest, -1) {
		lw := strings.ToLower(w)
		switch {
		case grammar.DesignatorWordRe.MatchString(w):
			n.Designators = append(n.Designators, w)
		case hdrUnitWords[lw]:
			unit = true
		case !hdrBodyWords[lw]:
			text = true
		}
	}
	n.Designators = grammar.Dedupe(n.Designators)
	return n, unit, text
}

// hdrClean drops markdown emphasis and extra spaces.
func hdrClean(s string) string {
	return strings.TrimSpace(grammar.WsRe.ReplaceAllString(grammar.HdrMarkup.Replace(s), " "))
}

// hdrUnwrap removes the labels and brackets around a header's total. It returns
// the text inside, any text after the closing bracket, and whether a "Total"
// label means the unit may be left out.
func hdrUnwrap(s string) (core, after string, labelled bool) {
	// twice: a label may sit outside the brackets or inside them
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
			// not by matching: "a) Politics ... b)" would close it early
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

// hdrSentences splits s at sentence ends and at semicolons outside brackets. A
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
			// a missing space after a word, not a decimal point: "courses.1.0"
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

// hdrTrimRaw trims a phrase to its words: no separators, no "and" before it, no
// full stop or unmatched bracket after it.
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
