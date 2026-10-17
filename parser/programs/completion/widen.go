package completion

import (
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/parser/grammar"
	"degree-explorer/parser/programs"
	"degree-explorer/types"
)

// Passes that read sentences wherever the program's text has them and encode
// them in the rows they concern: widenings, designator aliases, electives.

// lineSentences returns line i's sentences with their emphasis stripped,
// split once for the whole text. Callers must not change them.
func (a *Assembler) lineSentences(i int) []string {
	if a.lineSents == nil {
		a.lineSents = make([][]string, len(a.lines))
		for k, l := range a.lines {
			a.lineSents[k] = SplitSentences(programs.StripEmphasis(strings.TrimSpace(l)))
		}
	}
	return a.lineSents[i]
}

// widenNamedListRe matches a capped share from a list named below:
// "Students may take up to a 0.5 credit from the Equivalent Courses list below".
var widenNamedListRe = regexp.MustCompile(`(?i)^students\s+may\s+take\s+up\s+to\s+(?:a\s+)?` + grammar.AmountPat + `\s+credits?\s+from\s+the\s+(.+?)\s+list\s+below$`)

var (
	// widenOtherDeptsRe matches "Up to 1.0 credit may be from other
	// departments".
	widenOtherDeptsRe = regexp.MustCompile(`(?i)^(?:up\s+to|a\s+maximum\s+of)\s+(?:a\s+)?` + grammar.AmountPat + `\s+credits?\s+may\s+be\s+from\s+other\s+departments$`)
	// widenRelatedRe matches "1.0 credit of related courses offered by other
	// academic units may be counted towards the fulfillment of the program".
	widenRelatedRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\s+of\s+related\s+courses\s+offered\s+by\s+other\s+academic\s+units\s+may\s+be\s+counted\s+towards?\s+the\s+fulfil+ment\s+of\s+the\s+program$`)
	// widenOutsideRe matches "No more than 1.0 credit of courses from
	// outside of the Department of Political Science may be used".
	widenOutsideRe = regexp.MustCompile(`(?i)^` + grammar.NoMoreThanPat + `\s+(?:a\s+)?` + grammar.AmountPat + `\s+credits?\s+of\s+courses\s+(?:offered\s+)?(?:from\s+)?outside\s+(?:of\s+)?the\s+department\s+of\s+.+?\s+may\s+be\s+used\s+to\s+fulfil+\s+the\s+program\s+requirements$`)
	// widenOnRe matches "1.5 credits may be courses on East Asia offered by
	// other departments", over a list headed with that phrase.
	widenOnRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\s+may\s+be\s+courses\s+on\s+(.+?)\s+offered\s+by\s+other\s+departments$`)
	// The sentences that name the courses: "Approved courses include ...", "...
	// are examples".
	widenApprovedRe = regexp.MustCompile(`(?i)^approved\s+courses\s+include\s+(.+)$`)
	widenEligibleRe = regexp.MustCompile(`(?i)^the\s+list\s+of\s+eligible\s+courses\s+includes:?\s*(.*)$`)
	widenExamplesRe = regexp.MustCompile(`(?i)^(.+?)\s+are\s+examples(?:\s+of\s+such\s+courses)?$`)
	widenFullListRe = regexp.MustCompile(`(?i)^non-[A-Z]{3}\s+courses\s+that\s+may\s+be\s+used\s+to\s+fulfil+\s+the\s+program\s+requirements:\s*(.+)$`)
	// widenOthersRe matches the courses a widening doesn't list: "Other
	// courses might also qualify".
	widenOthersRe = regexp.MustCompile(`(?i)\bother\s+courses\s+might\s+also\s+qualify\b`)
)

// widenLabelRe matches a label before the sentence: "Courses in other
// departments: ".
var widenLabelRe = regexp.MustCompile(`^[A-Z][^:.]{0,60}:\s+`)

// widening is a capped share of courses outside the pools, named in a list.
type widening struct {
	amount  float64
	codes   []string
	others  string // a sentence about courses the text doesn't list
	program bool   // the share is the program's: its limit goes under the total
	line    int
	used    []string // the sentences and lists it was read from
}

// widenLine returns line i's text without its marker and emphasis.
func (a *Assembler) widenLine(i int) string {
	return strings.TrimSpace(a.clean(strings.TrimSpace(a.lines[i])))
}

func (a *Assembler) readWidenings() []widening {
	var out []widening
	for i := range a.lines {
		sents := a.lineSentences(i)
		for j, s := range sents {
			t := widenLabelRe.ReplaceAllString(a.clean(s), "")
			w := widening{line: i, used: []string{strings.TrimSpace(s)}}
			rest := sents[j+1:]
			var m []string
			switch {
			case widenOtherDeptsRe.MatchString(t):
				m = widenOtherDeptsRe.FindStringSubmatch(t)
				for k, r := range rest {
					if g := widenApprovedRe.FindStringSubmatch(a.clean(r)); g != nil && k == 0 {
						w.codes = grammar.CodeRe.FindAllString(g[1], -1)
						w.used = append(w.used, strings.TrimSpace(r))
					} else if widenOthersRe.MatchString(r) {
						w.others = strings.Join(rest[k:], " ")
						w.used = append(w.used, rest[k:]...)
						break
					}
				}
			case widenRelatedRe.MatchString(t):
				m = widenRelatedRe.FindStringSubmatch(t)
				w.program = true
				if len(rest) > 0 {
					if g := widenEligibleRe.FindStringSubmatch(a.clean(rest[0])); g != nil {
						w.used = append(w.used, strings.TrimSpace(rest[0]))
						w.codes = grammar.CodeRe.FindAllString(g[1], -1)
						for k := i + 1; w.codes == nil && k < len(a.lines) && IsCodeList(a.widenLine(k)); k++ {
							w.codes = append(w.codes, grammar.CodeRe.FindAllString(a.lines[k], -1)...)
							w.used = append(w.used, a.widenLine(k))
						}
					}
				}
			case widenNamedListRe.MatchString(t):
				m = widenNamedListRe.FindStringSubmatch(t)
				name := strings.ToLower(m[2])
				for k := i + 1; k < len(a.lines); k++ {
					if strings.ToLower(strings.Trim(a.widenLine(k), " :*_")) != name {
						continue
					}
					for k++; k < len(a.lines) && IsCodeList(a.widenLine(k)); k++ {
						w.codes = append(w.codes, grammar.CodeRe.FindAllString(a.lines[k], -1)...)
						w.used = append(w.used, a.widenLine(k))
					}
					break
				}
			case widenOnRe.MatchString(t):
				m = widenOnRe.FindStringSubmatch(t)
				want := "courses on " + strings.ToLower(m[2]) + " offered by other departments"
				for k := i + 1; k < len(a.lines); k++ {
					if !strings.HasSuffix(strings.ToLower(strings.Trim(a.widenLine(k), " :*_")), want) {
						continue
					}
					w.used = append(w.used, a.widenLine(k))
					for k++; k < len(a.lines) && mostlyCodes(a.widenLine(k)); k++ {
						w.codes = append(w.codes, grammar.CodeRe.FindAllString(a.lines[k], -1)...)
						w.used = append(w.used, a.widenLine(k))
					}
					break
				}
			case widenOutsideRe.MatchString(t):
				m = widenOutsideRe.FindStringSubmatch(t)
				w.program = true
				if len(rest) > 0 {
					if g := widenExamplesRe.FindStringSubmatch(a.clean(rest[0])); g != nil && IsCodeList(g[1]) {
						w.codes = grammar.CodeRe.FindAllString(g[1], -1)
						w.others = strings.TrimSpace(rest[0])
						w.used = append(w.used, w.others)
					}
				}
				for k := range a.lines {
					if g := widenFullListRe.FindStringSubmatch(a.widenLine(k)); w.codes == nil && g != nil {
						w.codes = grammar.CodeRe.FindAllString(g[1], -1)
						w.used = append(w.used, a.widenLine(k))
					}
				}
			}
			if m == nil || w.codes == nil {
				continue
			}
			w.amount, _ = strconv.ParseFloat(m[1], 64)
			w.codes = grammar.Dedupe(w.codes)
			out = append(out, w)
		}
	}
	return out
}

// widen adds each widening's courses to the nearest pool of designators before
// it, capped at the share under the total or the pool; courses it doesn't list
// are a text alternative on the pool.
func (a *Assembler) widen() {
	for _, w := range a.readWidenings() {
		row, pool := a.widenTarget(w.line)
		if pool == nil || slices.ContainsFunc(w.codes, func(c string) bool { return !grammar.LevelMatches(pool.Levels, c) || slices.Contains(pool.Exclude, c) }) {
			continue // the listed courses would fail the pool: left as written
		}
		pool.Courses = grammar.Dedupe(append(slices.Clone(pool.Courses), w.codes...))
		limit := &types.Requirement{Kind: types.RequirementLimit, Raw: w.used[0],
			Req: &types.ReqNode{Type: types.NodeCredits, Courses: slices.Clone(w.codes), MaxAmount: grammar.MaxAmount(w.amount)}}
		if w.program && a.total != nil {
			a.program = append(a.program, &compRow{r: limit})
		} else {
			a.rowUnder(row, limit)
		}
		if w.others != "" {
			kind := types.TextOther
			if approvalWordRe.MatchString(w.others) || a.IsApproval(w.others) {
				kind = types.TextPermission
			}
			row.r.Req = programs.OrNode(row.r.Req, TextNode(kind, w.others))
		}
		for _, u := range w.used {
			addRaw(row, u)
		}
		a.dropEncoded(w.used)
	}
}

// widenTarget returns the row and pool a widening on line i widens: the last
// pool of designators written before the line.
func (a *Assembler) widenTarget(i int) (*compRow, *types.ReqNode) {
	var best *compRow
	var bestPool *types.ReqNode
	bestLine := -1
	for _, r := range a.routeRows() {
		var pool *types.ReqNode
		grammar.WalkReq(r.r.Req, func(x *types.ReqNode) {
			if pool == nil && x.Type == types.NodeCredits && x.Amount > 0 && x.Designators != nil && x.Lists == nil {
				pool = x
			}
		})
		if pool == nil {
			continue
		}
		first := strings.TrimSpace(RawHead(r.r))
		for k := i - 1; k > bestLine; k-- {
			if strings.TrimSpace(a.lines[k]) == first || strings.Contains(a.lines[k], first) {
				best, bestPool, bestLine = r, pool, k
				break
			}
		}
	}
	return best, bestPool
}

// dropEncoded removes the notes, routes and rows of text that hold the
// sentences used, now encoded in a row.
func (a *Assembler) dropEncoded(used []string) {
	enc := map[string]bool{}
	for _, u := range used {
		enc[rawWords(grammar.CleanNote(u))] = true
	}
	has := func(s string) bool {
		w := rawWords(grammar.CleanNote(s))
		for u := range enc {
			if w == u || strings.HasSuffix(w, u) || strings.HasSuffix(u, w) && len(w) > 20 {
				return true
			}
		}
		return false
	}
	a.notes = slices.DeleteFunc(a.notes, has)
	a.programRoutes = slices.DeleteFunc(a.programRoutes, func(s string) bool {
		for _, part := range SplitSentences(s) {
			if has(part) {
				return true
			}
		}
		return false
	})
	textRow := func(r *compRow) bool {
		return r.r.Req != nil && r.r.Req.Type == types.NodeText && len(r.children) == 0 && has(blkItemRe.ReplaceAllString(r.r.Raw, ""))
	}
	a.program = slices.DeleteFunc(a.program, textRow)
	a.items = slices.DeleteFunc(a.items, textRow) // "6. 1.5 credits may be courses on East Asia ..."
	for _, r := range a.items {
		r.r.Notes = slices.DeleteFunc(r.r.Notes, has)
	}
}

var (
	aliasDesigListPat = `(` + grammar.DesigPat + `(?:\s*(?:,|,?\s*and|,?\s*or)\s*` + grammar.DesigPat + `)*)`
	// aliasPrefixesRe matches "ANT courses are those offered with the
	// following prefixes: ANT, ARH, INS, JAL, and JGA".
	aliasPrefixesRe = regexp.MustCompile(`(?i)^(` + grammar.DesigPat + `)\s+courses\s+are\s+those\s+offered\s+with\s+the\s+following\s+prefixes:\s*` + aliasDesigListPat + `$`)
	// aliasExcludingRe matches "(excluding the following language courses:
	// MHB255H1, ..., RLG265H1)", read as ", excluding MHB255H1, ...".
	aliasExcludingRe = regexp.MustCompile(`(?i)\s*\(\s*excluding\s+the\s+following(?:\s+[a-z]+)?\s+courses:\s*((?:` + grammar.CodePat + `)(?:\s*,\s*(?:` + grammar.CodePat + `))*)\s*\)`)
	aliasAnyOtherRe  = regexp.MustCompile(`(?i)(\bin\s+)any\s+other\s+`)
	// aliasCountedAsRe matches "Courses with RLG, JAR, ... and JSR
	// designators count as Department for the Study of Religion courses".
	aliasCountedAsRe = regexp.MustCompile(`(?i)^(?:courses\s+with\s+)?` + aliasDesigListPat + `\s+(?:courses|designators)\s+(?:will\s+be\s+counted|count)\s+as\s+(.+?)\s+courses$`)
)

// aliasDesignators applies a definition of designators: a prefix list widens
// every filter on its designator; a "count as <phrase> courses" sentence
// rereads the rows of text that name the phrase with those designators.
func (a *Assembler) aliasDesignators() {
	for i := range a.lines {
		for _, s := range a.lineSentences(i) {
			t := a.clean(s)
			if m := aliasPrefixesRe.FindStringSubmatch(t); m != nil {
				set := grammar.DesigRe.FindAllString(m[2], -1)
				if !slices.Contains(set, m[1]) {
					continue
				}
				done := false
				for _, r := range a.allRows() {
					grammar.WalkReq(r.r.Req, func(x *types.ReqNode) {
						if x.Type == types.NodeCredits && slices.Contains(x.Designators, m[1]) {
							x.Designators = grammar.Dedupe(append(slices.Clone(x.Designators), set...))
							addRaw(r, s)
							done = true
						}
					})
				}
				if done {
					a.dropEncoded([]string{s})
				}
				continue
			}
			if a.courseCountsAs(s) {
				continue
			}
			m := aliasCountedAsRe.FindStringSubmatch(t)
			if m == nil {
				continue
			}
			set := grammar.DesigRe.FindAllString(m[1], -1)
			phrase := strings.ToLower(m[2])
			for _, name := range slices.Sorted(maps.Keys(grammar.SubjectDesignators)) { // in a fixed order: each match goes first
				if d := grammar.SubjectDesignators[name]; strings.Contains(phrase, name) && !slices.Contains(set, d) {
					set = append([]string{d}, set...)
				}
			}
			phraseRe := regexp.MustCompile(`(?i)\b(?:` + regexp.QuoteMeta(m[2]) + `|DSR)\s+(courses?|credits?)\b`)
			done := false
			for _, r := range a.allRows() {
				n := r.r.Req
				if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || !phraseRe.MatchString(n.Text) {
					continue
				}
				// "any other" means other than the courses the rows already
				// use, which allocation keeps apart.
				text := aliasAnyOtherRe.ReplaceAllString(aliasExcludingRe.ReplaceAllString(n.Text, ", excluding $1"), "${1}any ")
				body, rows, notes := a.itemBody(phraseRe.ReplaceAllString(text, strings.Join(set, "/")+" $1"))
				if body == nil || hasText(body) || isAnyCredits(body) {
					continue
				}
				r.r.Req = body
				for _, k := range rows {
					a.rowUnder(r, k)
				}
				r.r.Notes = append(r.r.Notes, notes...)
				splitFloors(r, n.Text)
				addRaw(r, s)
				done = true
			}
			for _, r := range a.allRows() {
				if r.r.Kind == types.RequirementAllocation && aliasQualified(r.r.Req, m[2], set) {
					addRaw(r, s)
					done = true
				}
			}
			if done {
				a.dropEncoded([]string{s})
			}
		}
	}
}

// areaOfFocusRe matches a qualifier the course's topic decides: "in your
// area of focus".
var areaOfFocusRe = regexp.MustCompile(`(?i)^in\s+your\s+area\s+of\s+focus$`)

// aliasQualified reads and(credits, text) whose credits name no source and
// whose text ends in "selected from <phrase> courses": the credits count set
// and the text keeps the rest. It reports whether it did.
func aliasQualified(n *types.ReqNode, phrase string, set []string) bool {
	if n == nil || n.Type != types.NodeAnd || len(n.Items) != 2 {
		return false
	}
	c, t := n.Items[0], n.Items[1]
	if c.Type != types.NodeCredits || len(c.Courses) > 0 || len(c.Designators) > 0 || len(c.Lists) > 0 || t.Type != types.NodeText {
		return false
	}
	re := regexp.MustCompile(`(?i)^(.*?)\s*(?:selected\s+)?from\s+(?:` + regexp.QuoteMeta(phrase) + `|DSR)\s+courses\.?$`)
	m := re.FindStringSubmatch(t.Text)
	if m == nil || m[1] == "" {
		return false
	}
	c.Designators = slices.Clone(set)
	t.Text = m[1]
	if areaOfFocusRe.MatchString(m[1]) {
		t.Kind = types.TextContent
	}
	return true
}

// countsAsRe matches a course defined as one of a subject's: "For the
// purpose of program completion, JWE206H1 counts as an English course".
var countsAsRe = regexp.MustCompile(`(?i)^(?:for\s+the\s+purpose\s+of\s+program\s+completion,?\s*)?((?:` + grammar.CodePat + `)(?:\s*(?:,|and|or)\s*` + grammar.CodePat + `)*)\s+counts?\s+as\s+an?\s+([a-z]+(?:\s+[a-z]+)?)\s+courses?$`)

// countsAsSubjects maps subjects countsAsRe names that
// SubjectDesignators lacks.
var countsAsSubjects = map[string]string{"english": "ENG"}

// courseCountsAs adds the courses of a countsAsRe sentence s to every
// filter on the subject's designator whose levels they fit. It reports
// whether it did.
func (a *Assembler) courseCountsAs(s string) bool {
	m := countsAsRe.FindStringSubmatch(strings.TrimSuffix(a.clean(s), "."))
	if m == nil {
		return false
	}
	subject := strings.ToLower(m[2])
	d := countsAsSubjects[subject]
	if d == "" {
		d = grammar.SubjectDesignators[subject]
	}
	codes := grammar.CodeRe.FindAllString(m[1], -1)
	if d == "" {
		return false
	}
	done := false
	for _, r := range a.allRows() {
		hit := false
		grammar.WalkReq(r.r.Req, func(x *types.ReqNode) {
			if x.Type != types.NodeCredits || !slices.Contains(x.Designators, d) {
				return
			}
			for _, c := range codes {
				if grammar.LevelMatches(x.Levels, c) && !slices.Contains(x.Exclude, c) {
					x.Courses = grammar.Dedupe(append(slices.Clone(x.Courses), c))
					hit = true
				}
			}
		})
		if hit {
			addRaw(r, s)
			done = true
		}
	}
	if done {
		a.dropEncoded([]string{s})
	}
	return done
}

// allRows returns every row read so far, in document order.
func (a *Assembler) allRows() []*compRow {
	var out []*compRow
	visit := func(r *compRow) {
		out = append(out, r)
	}
	if a.total != nil {
		out = append(out, a.total)
	}
	walkRows(a.floors, visit)
	walkRows(a.items, visit)
	walkRows(a.program, visit)
	return out
}

// additionalElectiveRe matches "An additional 0.5 credit can be taken
// from any 3rd or 4th year elective listed above".
var additionalElectiveRe = regexp.MustCompile(`(?i)^an?\s+additional\s+` + grammar.AmountPat + `\s+credits?\s+(?:can|may|must)\s+be\s+taken\s+from\s+any\s+(?:3rd|third)\s+or\s+(?:4th|fourth)\s+year\s+electives?\s+listed\s+above\s*(.*)$`)

// additionalElectives makes a additionalElectiveRe note an item: the
// amount from the third- and fourth-year items' pools. As a note it would
// leave every student Review, as the items fall short of the total.
func (a *Assembler) additionalElectives() {
	for i, note := range a.notes {
		m := additionalElectiveRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(note), "."))
		if m == nil {
			continue
		}
		var codes []string
		for _, r := range a.routeRows() {
			sec := strings.ToLower(r.r.Section)
			year := strings.Contains(sec, "third") || strings.Contains(sec, "fourth") || strings.Contains(sec, "year 3") || strings.Contains(sec, "year 4") ||
				sec == "" && (r.r.Label == "3" || r.r.Label == "4")
			if r == a.total || !year {
				continue
			}
			grammar.WalkReq(r.r.Req, func(x *types.ReqNode) {
				if x.Type == types.NodeCredits && x.Amount > 0 && x.Courses != nil && x.Designators == nil {
					codes = append(codes, x.Courses...)
				}
			})
		}
		if codes == nil {
			continue
		}
		v, _ := strconv.ParseFloat(m[1], 64)
		row := &compRow{r: &types.Requirement{Kind: types.RequirementAllocation, Raw: note,
			Req: &types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: grammar.Dedupe(codes)}}}
		if rest := strings.TrimSpace(m[2]); rest != "" {
			row.r.Req = programs.OrNode(row.r.Req, TextNode(types.TextOther, note)) // "or under the Forest Conservation Specialist"
		}
		a.items = append(a.items, row)
		a.notes = slices.Delete(a.notes, i, i+1)
		return
	}
}
