package completion

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/parser/grammar"
	"degree-explorer/parser/programs"
	"degree-explorer/types"
)

// Limits beyond floors and caps: transfer and CR/NCR rules, the campus rule,
// profile facts, audiences, program caps and caps counted in courses.

// campusRule puts a sentence restating the campus rule (filters match St.
// George courses only) in Req 1's raw, whose designator pool encodes it. It
// reports false when Req 1 has no such pool.
func (a *Assembler) campusRule(s string) bool {
	if a.total == nil || a.total.r.Req.Type != types.NodeCredits || a.total.r.Req.Designators == nil || a.total.r.Req.Courses != nil {
		return false
	}
	a.total.r.Raw += "\n" + s
	return true
}

var (
	// transferCapRe matches "No more than 1.0 transfer credit(s) will be
	// eligible for consideration in the Certificate".
	transferCapRe = regexp.MustCompile(`(?i)^(?:` + grammar.NoMoreThanPat + `|a\s+maximum\s+of|up\s+to)\s+(?:a\s+)?` + grammar.AmountPat + `\s+(?:credits?\s+in\s+)?transfer\s+credits?\s+(?:will\s+be\s+eligible\s+for\s+consideration|may\s+be\s+(?:counted|applied|used)\s+(?:to(?:wards?)?|for))\s+(?:in\s+)?the\s+(?:certificate|program|minor|major|specialist|focus)$`)
	// transferComprisesRe matches "Transfer credits cannot comprise more
	// than 0.5 credit at the 300-/400-level".
	transferComprisesRe = regexp.MustCompile(`(?i)^transfer\s+credits\s+cannot\s+comprise\s+more\s+than\s+(?:a\s+)?` + grammar.AmountPat + `\s+credits?(?:\s+at\s+the\s+(` + grammar.LevelPat + `[-/\s\d+]*-?\s*levels?))?$`)
)

// transfer reads a cap on transfer credit into a limit row under target.
func (a *Assembler) transfer(target *compRow, s string) {
	t := strings.TrimSpace(strings.TrimSuffix(s, "."))
	if r, text, ok := transferRows(t); ok {
		// with the exchange exception
		r.Raw = s
		a.rowUnder(target, r)
		if text {
			a.textRow(target, s)
		}
		return
	}
	n := &types.ReqNode{Type: types.NodeCredits, Transfer: true}
	if m := transferCapRe.FindStringSubmatch(t); m != nil {
		v, _ := strconv.ParseFloat(m[1], 64)
		n.MaxAmount = grammar.MaxAmount(v)
	} else if m := transferComprisesRe.FindStringSubmatch(t); m != nil {
		v, _ := strconv.ParseFloat(m[1], 64)
		n.MaxAmount = grammar.MaxAmount(v)
		n.Levels = grammar.ParseLevels(m[2])
	} else {
		a.textRow(target, s)
		return
	}
	a.rowUnder(target, &types.Requirement{Kind: types.RequirementLimit, Req: n, Raw: s})
}

// crNcrBanRe matches a rule that courses taken CR/NCR don't count: "We
// do not accept any CR/NCR courses toward our program".
var crNcrBanRe = regexp.MustCompile(`(?i)^(?:courses\s+(?:that\s+have\s+been\s+taken\s+as|with\s+a)\s+(?:credit\s*/\s*no\s+credit|CR\s*/\s*NCR)(?:\s+status)?\s+(?:cannot|may\s+not)\s+be\s+used\s+to\s+(?:fulfill|satisfy)\s+(?:the\s+)?(?:certificate|program)\s+requirements|we\s+do\s+not\s+accept\s+any\s+CR\s*/\s*NCR\s+courses\s+toward\s+our\s+programs?)$`)

// crNcrMayRe matches CR/NCR allowed: "may use".
var crNcrMayRe = regexp.MustCompile(`(?i)\b(?:may|can)\s+(?:use|take)\b`)

// crNcr reads a rule on CR/NCR courses into a limit row under target.
func (a *Assembler) crNcr(target *compRow, s string) {
	if a.crNcrOutside(target, s) {
		// CR/NCR or other coursework, one share
		return
	}
	if rows, text := crNcrRows(a.clean(s)); rows != nil {
		// codes beside the CR/NCR courses, other wordings, a share
		for _, r := range rows {
			r.Raw = s
			a.rowUnder(target, r)
		}
		if text {
			a.textRow(target, s)
		}
		return
	}
	if !crNcrBanRe.MatchString(strings.TrimSpace(strings.TrimSuffix(s, "."))) {
		if t := a.clean(s); crNcrMayRe.MatchString(t) && !negationRe.MatchString(t) && !bindingWordRe.MatchString(t) {
			// "Students may use CR/NCR on the course ...": a permission, a note
			a.notesTo(a.noteTarget(target), []string{grammar.CleanNote(s)})
			return
		}
		a.textRow(target, s)
		return
	}
	a.rowUnder(target, &types.Requirement{Kind: types.RequirementLimit, Raw: s,
		Req: &types.ReqNode{Type: types.NodeCredits, CrNcr: true, MaxAmount: grammar.MaxAmount(0)}})
}

// designatorRules encodes the program rules written with excluded designators:
// the in-and-outside floors (nonDesigRe) as a transcript constraint, and
// the other-units rule as a limit of 0.
func (a *Assembler) designatorRules() {
	for _, r := range a.allRows() {
		n := r.r.Req
		if r.r.Kind != types.RequirementConstraint || n == nil || n.Type != types.NodeText || n.Kind != types.TextOther || len(r.children) > 0 {
			continue
		}
		t := strings.TrimSuffix(strings.TrimSpace(n.Text), ".")
		if m := nonDesigRe.FindStringSubmatch(t); m != nil && m[2] == m[4] {
			in, _ := strconv.ParseFloat(m[1], 64)
			out, _ := strconv.ParseFloat(m[3], 64)
			yes := &types.ReqNode{Type: types.NodeCredits, Amount: in, Designators: []string{m[2]}}
			no := &types.ReqNode{Type: types.NodeCredits, Amount: out, ExcludeDesignators: []string{m[2]}}
			a.notes = slices.DeleteFunc(a.notes, func(note string) bool {
				x := countsAsDesigRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(note), "."))
				if x == nil || x[2] != m[2] || x[1][:3] == m[2] {
					return false
				}
				yes.Courses = append(yes.Courses, x[1])
				no.Exclude = append(no.Exclude, x[1])
				addRaw(r, note)
				return true
			})
			r.r.Req = programs.AndNode(yes, no)
			// the two amounts exceed the program's total, so the transcript
			r.r.Scope = types.ScopeTranscript
			a.transcriptCrNcr(m[1], yes)
			continue
		}
		if otherUnitsRe.MatchString(t) {
			if lim := a.ownUnitsLimit(); lim != nil {
				r.r.Kind, r.r.Req = types.RequirementLimit, lim
			}
		}
	}
}

// transcriptCrNcr adds, after a CR/NCR limit of 0 that also names the
// designator's transcript constraint, the same limit over the transcript on
// pool's courses.
func (a *Assembler) transcriptCrNcr(amount string, pool *types.ReqNode) {
	for i, r := range a.program {
		n := r.r.Req
		if r.r.Kind != types.RequirementLimit || n == nil || n.Type != types.NodeCredits || !n.CrNcr || n.MaxAmount == nil || *n.MaxAmount != 0 || len(n.Designators) > 0 || len(n.Courses) > 0 {
			continue
		}
		m := crNcrTranscriptRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(r.r.Raw), "."))
		if m == nil || m[1] != amount || m[2] != pool.Designators[0] {
			continue
		}
		k := &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Section: r.r.Section, Scope: types.ScopeTranscript, Raw: r.r.Raw,
			Req: &types.ReqNode{Type: types.NodeCredits, Designators: slices.Clone(pool.Designators), Courses: slices.Clone(pool.Courses), CrNcr: true, MaxAmount: grammar.MaxAmount(0)}}}
		a.program = slices.Insert(a.program, i+1, k)
		return
	}
}

// ownUnitsLimit returns the limit of 0 on courses outside the designators the
// allocations draw on, except courses the text names. Nil when no allocation
// draws on a designator.
func (a *Assembler) ownUnitsLimit() *types.ReqNode {
	var own []string
	for _, r := range a.allRows() {
		if r.r.Kind != types.RequirementAllocation {
			continue
		}
		grammar.WalkReq(r.r.Req, func(n *types.ReqNode) {
			if n.Type == types.NodeCredits {
				own = append(own, n.Designators...)
			}
		})
	}
	if own = grammar.Dedupe(own); own == nil {
		return nil
	}
	var named []string
	for _, code := range grammar.CodeRe.FindAllString(a.raw, -1) {
		if !slices.Contains(own, code[:3]) {
			named = append(named, code)
		}
	}
	lim := &types.ReqNode{Type: types.NodeCredits, MaxAmount: grammar.MaxAmount(0)}
	lim.ExcludeDesignators = own
	lim.Exclude = grammar.Dedupe(named)
	return lim
}

var (
	// crNcrCodesBanRe matches "We do not accept ENG100H1, ENG102H1, or any
	// CR/NCR courses toward our programs". Group 1 or 2 holds the codes.
	crNcrCodesBanRe = regexp.MustCompile(`(?i)^(?:we\s+do\s+not\s+accept\s+(` + grammar.CodePat + `(?:\s*,\s*` + grammar.CodePat + `)*),?\s+or\s+any\s+CR\s*/\s*NCR\s+courses\s+towards?\s+our\s+programs?` +
		`|(` + grammar.CodePat + `(?:\s*,\s*` + grammar.CodePat + `)*),?\s+and\s+courses\s+with\s+a\s+CR\s*/\s*NCR\s+final\s+(?:mark|grade)\s+are\s+not\s+accepted\s+towards?\s+any\s+[\w&\s]+?\s+program)$`)
	// crNcrProgramBanRe matches a ban on CR/NCR courses in the program: "The
	// CR/NCR option cannot be used for any program completion requirements".
	crNcrProgramBanRe = regexp.MustCompile(`(?i)^(?:no\s+courses\s+in\s+the\s+[^,.]+?\s+can\s+be\s+taken\s+as\s+(?:credit\s*/\s*no\s+credit|CR\s*/\s*NCR)\s+if\s+they\s+are\s+to\s+(?:fulfill|satisfy)\s+(?:the\s+)?(?:certificate|program)\s+requirements` +
		`|the\s+CR\s*/\s*NCR\s+option\s+cannot\s+be\s+used\s+for\s+any\s+program\s+(?:completion\s+)?requirements(?:,?\s+and\s+the\s+` + grammar.NumPat + `\s+` + grammar.DesigPat + `\s+credit\s+requirement)?)$`)
	// crNcrShareRe matches a share of CR/NCR courses: "Only 0.5 credit of
	// Credit/No Credit coursework ...". The rest of the sentence stays text.
	crNcrShareRe = regexp.MustCompile(`(?i)^only\s+(?:a\s+)?` + grammar.AmountPat + `\s+credits?\s+of\s+(?:credit\s*/\s*no\s+credit|CR\s*/\s*NCR)\s+(?:coursework|courses)\b`)
)

// crNcrOutsideRe matches "Only 0.5 credit of Credit/No Credit coursework
// or coursework taken outside of FRE or FSL courses ... must be pre-approved".
var crNcrOutsideRe = regexp.MustCompile(`(?i)^only\s+(?:a\s+)?` + grammar.AmountPat + `\s+credits?\s+of\s+(?:credit\s*/\s*no\s+credit|CR\s*/\s*NCR)\s+coursework\s+or\s+coursework\s+taken\s+outside\s+of\s+(` + grammar.DesigPat + `(?:\s+or\s+` + grammar.DesigPat + `)*)\s+courses\b[^,]*?\s+may\s+be\s+accepted,?\s+and\s+(must\s+be\s+pre-?approved\s+by\s+.+)$`)

// crNcrOutside reads crNcrOutsideRe under target: a limit on a list of
// CR/NCR courses and courses outside the designators, and the approval as
// or(credits{that list, max 0}, text{permission}): counting none needs none.
func (a *Assembler) crNcrOutside(target *compRow, s string) bool {
	m := crNcrOutsideRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(a.clean(s)), "."))
	if m == nil {
		return false
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	limit := &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Raw: s}}
	l := a.rowUnion(limit, []*types.ReqNode{
		{Type: types.NodeCredits, CrNcr: true},
		{Type: types.NodeCredits, ExcludeDesignators: grammar.DesigRe.FindAllString(m[2], -1)},
	})
	limit.r.Req = &types.ReqNode{Type: types.NodeCredits, Lists: []string{l.Name}, MaxAmount: grammar.MaxAmount(v)}
	approval := &types.Requirement{Kind: types.RequirementConstraint, Raw: s, Req: programs.OrNode(
		&types.ReqNode{Type: types.NodeCredits, Lists: []string{l.Name}, MaxAmount: grammar.MaxAmount(0)},
		TextNode(types.TextPermission, m[3]))}
	if target != nil {
		target.children = append(target.children, limit)
	} else {
		a.program = append(a.program, limit)
	}
	a.rowUnder(target, approval)
	return true
}

// widenOutsideShares lets the coursework crNcrOutside caps count toward a
// designator total, which would never count it: the total's pool and each node
// restating it become a list of the designators and the capped list.
func (a *Assembler) widenOutsideShares() {
	if a.total == nil || a.total.r.Req == nil {
		return
	}
	pool := a.total.r.Req
	if pool.Type != types.NodeCredits || len(pool.Designators) == 0 {
		return
	}
	bare := func(n *types.ReqNode) bool {
		c := grammar.CloneNode(n)
		c.Amount, c.Designators = 0, nil
		return sameNode(c, &types.ReqNode{Type: types.NodeCredits})
	}
	if !bare(pool) {
		return
	}
	for _, u := range a.rowUnions {
		it := u.list.Items
		if len(it) != 2 || !it[0].CrNcr || !grammar.SameSet(it[1].ExcludeDesignators, pool.Designators) {
			continue
		}
		amount, desigs := pool.Amount, pool.Designators
		l := a.rowUnion(a.total, []*types.ReqNode{
			{Type: types.NodeCredits, Designators: desigs},
			{Type: types.NodeCredits, Lists: []string{u.list.Name}},
		})
		widen := func(n *types.ReqNode) {
			grammar.WalkReq(n, func(n *types.ReqNode) {
				if n.Type == types.NodeCredits && n.Amount == amount && grammar.SameSet(n.Designators, desigs) && bare(n) {
					n.Designators, n.Lists = nil, []string{l.Name}
				}
			})
		}
		widen(pool)
		visit := func(r *compRow) {
			widen(r.r.Req)
		}
		walkRows(a.total.children, visit)
		walkRows(a.floors, visit)
		walkRows(a.items, visit)
		walkRows(a.program, visit)
		return
	}
}

// titledScopeRe matches the row a cap counts toward by quoted title:
// "toward the major completion requirement of "further topics"".
var titledScopeRe = regexp.MustCompile(`(?i)\btowards?\s+the\s+(?:[\w-]+\s+){0,3}requirement\s+(?:of\s+)?["“]([^"”]+)["”]`)

// titledLimits moves a program-level limit whose sentence names its row by a
// quoted title under that row: under the total, the capped courses could fill
// the named row while surplus courses meet the total.
func (a *Assembler) titledLimits() {
	var keep []*compRow
	for _, r := range a.program {
		if r.r.Kind == types.RequirementLimit {
			if m := titledScopeRe.FindStringSubmatch(a.clean(r.r.Raw)); m != nil {
				if rows := a.titledRows(m[1]); len(rows) == 1 && rows[0] != a.total {
					rows[0].children = append(rows[0].children, r)
					continue
				}
			}
		}
		keep = append(keep, r)
	}
	a.program = keep
}

// takeMoreRe matches a cap only permission lifts: "Students cannot take
// more than a 0.5 credit at the 400-level without written permission".
var takeMoreRe = regexp.MustCompile(`(?i)^students\s+cannot\s+take\s+more\s+than\s+(?:an?\s+)?` + grammar.AmountPat + `\s+credits?\s+(?:at\s+the\s+(` + grammar.LevelPat + `)-level|of\s+the\s+([\w-]+)\s+courses)\s+without\s+(?:written\s+)?permission\b`)

// permittedCaps reads takeMoreRe constraints into or(credits{max N, the
// courses}, text{permission}), the courses being the level or the naming item's
// code range; without one the text stays, as the item's filters count more.
func (a *Assembler) permittedCaps() {
	all := append(append(append([]*compRow{}, a.floors...), a.items...), a.program...)
	walkRows(all, func(r *compRow) {
		n := r.r.Req
		if r.r.Kind != types.RequirementConstraint || n == nil || n.Type != types.NodeText {
			return
		}
		m := takeMoreRe.FindStringSubmatch(strings.TrimSpace(a.clean(n.Text)))
		if m == nil {
			return
		}
		v, _ := strconv.ParseFloat(m[1], 64)
		cap := &types.ReqNode{Type: types.NodeCredits, MaxAmount: grammar.MaxAmount(v)}
		if m[2] != "" {
			cap.Levels = grammar.ParseLevels(m[2] + "-level")
		} else {
			name := strings.ToLower(m[3])
			walkRows(a.items, func(it *compRow) {
				if cap.Courses != nil || it.r.Kind != types.RequirementAllocation || !strings.Contains(strings.ToLower(it.r.Raw), name) {
					return
				}
				if rm := codeRangeRe.FindStringSubmatch(it.r.Raw); rm != nil {
					cap.Courses = codeRange(rm[1], rm[2])
				}
			})
			if cap.Courses == nil {
				return
			}
		}
		r.r.Req = programs.OrNode(cap, TextNode(types.TextPermission, n.Text))
	})
}

// codeRangeRe matches a bracketed range of codes: "( SOC317H1 –
// SOC354H1;". Groups: the ends.
var codeRangeRe = regexp.MustCompile(`\(\s*(` + grammar.CodePat + `)\s*–\s*(` + grammar.CodePat + `)\s*;`)

// codeRange returns the half and full codes from lo to hi, of one designator,
// level and campus. Most aren't courses; Link keeps those the catalogue has.
// Nil when the ends don't make such a range.
func codeRange(lo, hi string) []string {
	if !grammar.SgCodeRe.MatchString(lo) || !grammar.SgCodeRe.MatchString(hi) || lo[:4] != hi[:4] || lo[7] != hi[7] {
		return nil
	}
	a, _ := strconv.Atoi(lo[3:6])
	b, _ := strconv.Atoi(hi[3:6])
	if a >= b {
		return nil
	}
	var codes []string
	for i := a; i <= b; i++ {
		for _, w := range "HY" {
			codes = append(codes, fmt.Sprintf("%s%d%c%c", lo[:3], i, w, lo[7]))
		}
	}
	return codes
}

// compRowUnion is a list rowUnion made for a row with no label.
type compRowUnion struct {
	row  *compRow
	list *types.CourseList
}

// rowUnion makes items a list of row's own whose name waits for the row's
// number, as an unlabelled row can't be named by addUnion's rule.
func (a *Assembler) rowUnion(row *compRow, items []*types.ReqNode) *types.CourseList {
	l := &types.CourseList{Name: fmt.Sprintf("\x00row union %d", len(a.rowUnions)), Items: items}
	a.rowUnions = append(a.rowUnions, compRowUnion{row: row, list: l})
	return l
}

// crNcrRows reads a CR/NCR rule the ban grammar misses into limit rows, and
// reports whether the sentence must also stay text, being partly unencoded.
func crNcrRows(t string) (rows []*types.Requirement, text bool) {
	ban := &types.Requirement{Kind: types.RequirementLimit, Req: &types.ReqNode{Type: types.NodeCredits, CrNcr: true, MaxAmount: grammar.MaxAmount(0)}}
	if m := crNcrCodesBanRe.FindStringSubmatch(t); m != nil {
		codes := grammar.Dedupe(grammar.CodeRe.FindAllString(m[1]+" "+m[2], -1))
		return []*types.Requirement{{Kind: types.RequirementLimit, Req: &types.ReqNode{Type: types.NodeCredits, Courses: codes, MaxAmount: grammar.MaxAmount(0)}}, ban}, false
	}
	if crNcrProgramBanRe.MatchString(t) {
		return []*types.Requirement{ban}, false
	}
	if m := crNcrShareRe.FindStringSubmatch(t); m != nil {
		v, _ := strconv.ParseFloat(m[1], 64)
		ban.Req.MaxAmount = grammar.MaxAmount(v)
		return []*types.Requirement{ban}, true
	}
	return nil, false
}

var (
	// transferExchangeRe matches the exchange exception, a note: "(except
	// for those attained through a University of Toronto exchange program)".
	transferExchangeRe = regexp.MustCompile(`(?i)\s*\((except\s+(?:for\s+)?those\s+(?:attained|earned|obtained)\s+through\s+a\s+University\s+of\s+Toronto\s+exchange\s+program)\)`)
	// transferAndRe matches a clause no limit can sit under, so the text
	// stays: ", and cannot be used to satisfy the requirement for ...".
	transferAndRe = regexp.MustCompile(`(?i),?\s+and\s+cannot\s+be\s+used\s+to\s+satisfy\s+.*$`)
)

// transferRows reads a transfer cap with the exchange exception and maybe a
// second clause into a limit noting the exception; with the clause the
// sentence stays text, holding the exception. ok is false when it isn't one.
func transferRows(t string) (r *types.Requirement, text, ok bool) {
	m := transferExchangeRe.FindStringSubmatchIndex(t)
	if m == nil {
		return nil, false, false
	}
	note := grammar.CleanNote(t[m[2]:m[3]])
	t = t[:m[0]] + t[m[1]:]
	if loc := transferAndRe.FindStringIndex(t); loc != nil {
		t, text = t[:loc[0]], true
	}
	cm := transferComprisesRe.FindStringSubmatch(strings.TrimSpace(t))
	if cm == nil {
		return nil, false, false
	}
	v, _ := strconv.ParseFloat(cm[1], 64)
	var notes []string
	if !text {
		notes = []string{note} // with the second clause, the row of text holds the exception
	}
	return &types.Requirement{Kind: types.RequirementLimit, Notes: notes,
		Req: &types.ReqNode{Type: types.NodeCredits, Transfer: true, Levels: grammar.ParseLevels(cm[2]), MaxAmount: grammar.MaxAmount(v)}}, text, true
}

// Profile facts: facts about the student that aren't courses, in the schema's
// profile attributes.

var (
	// learningCommunityRe matches an item that is a learning community: "7.
	// Molecular Network Learning Community (each year of focus enrolment)".
	learningCommunityRe = regexp.MustCompile(`(?i)^(?:[\w-]+\s+){1,4}learning\s+community(?:\s*\([^()]*\))?\.?$`)
	// proficiencyRe matches "At least second-year proficiency in Chinese,
	// Korean or Japanese, either by completing an appropriate language course".
	proficiencyRe = regexp.MustCompile(`(?i)^` + grammar.AtLeastPat + `\s+(first|second|third|fourth)-year\s+proficiency\s+in\s+((?-i:[A-Z][a-z]+)(?:,\s*(?-i:[A-Z][a-z]+))*,?\s+or\s+(?-i:[A-Z][a-z]+)),\s+either\s+by\s+completing\s+an\s+appropriate\s+language\s+course\s+\(e\.g\.,?\s+([^()]+)\)\s+or\s+(by\s+demonstrating\s+the\s+required\s+proficiency\s+in\s+the\s+initial\s+placement)\.?$`)
	// languageNameRe matches a language in proficiencyRe's list.
	languageNameRe = regexp.MustCompile(`(?-i:[A-Z][a-z]+)`)
)

// profileBody reads an item that is a profile fact: has_learning_community, or
// proficiency as or(the courses, or(min_<language>_level per language)), the
// level being the year. Nil when it isn't one.
func profileBody(s string) *types.ReqNode {
	t := Clean(s)
	if learningCommunityRe.MatchString(t) {
		return &types.ReqNode{Type: types.NodeProfile, Attr: "has_learning_community", Value: true, Text: t}
	}
	m := proficiencyRe.FindStringSubmatch(t)
	if m == nil {
		return nil
	}
	courses, _ := parseCompletionText(m[3])
	if courses = checkedBody(courses, m[3]); courses == nil || !courseCodesOnly(courses) {
		return nil
	}
	level := grammar.OrdinalYear(m[1])
	var placed []*types.ReqNode
	for _, lang := range languageNameRe.FindAllString(m[2], -1) {
		placed = append(placed, &types.ReqNode{Type: types.NodeProfile, Attr: "min_" + strings.ToLower(lang) + "_level", Value: level, Text: m[4]})
	}
	return programs.OrNode(courses, programs.OrNode(placed...))
}

// courseCodesOnly reports whether n names courses only: course nodes, pools of
// listed courses, and text such as "or a higher level language course".
func courseCodesOnly(n *types.ReqNode) bool {
	ok, codes := true, false
	grammar.WalkReq(n, func(x *types.ReqNode) {
		switch x.Type {
		case types.NodeCourse:
			codes = true
		case types.NodeCredits:
			codes = codes || x.Courses != nil
		case types.NodeOr, types.NodeAnd, types.NodeText:
		default:
			ok = false
		}
	})
	return ok && codes
}

// audienceRe matches a sentence for one faculty's students: "Daniels
// students enrolled in this program may be able to fulfill ...".
var audienceRe = regexp.MustCompile(`^(?:[-*]\s*)?(Daniels)\s+students\b`)

// rowAudienceRe matches a rule for one faculty's students: "Visual Studies
// Minor students in the Faculty of Arts & Science are not permitted ...".
var rowAudienceRe = regexp.MustCompile(`^(?:[A-Z][\w-]*\s+){0,4}students\s+in\s+the\s+Faculty\s+of\s+(Arts\s*&\s*Science)\b`)

// audiences gives each text alternative for one faculty's students its
// audience, or(strict, and(profile{faculty}, text)), so the others are judged
// strictly. A row written for one faculty's students gets it as its Audience.
func (a *Assembler) audiences(rows []*compRow) {
	var fix func(n *types.ReqNode) *types.ReqNode
	fix = func(n *types.ReqNode) *types.ReqNode {
		if n == nil {
			return nil
		}
		if n.Type == types.NodeText {
			if m := audienceRe.FindStringSubmatch(n.Text); m != nil {
				return programs.AndNode(&types.ReqNode{Type: types.NodeProfile, Attr: "faculty", Value: m[1], Text: m[1] + " students"}, n)
			}
			return n
		}
		if n.Type != types.NodeOr {
			return n
		}
		for i, it := range n.Items {
			n.Items[i] = fix(it)
		}
		return n
	}
	for _, r := range rows {
		r.r.Req = fix(r.r.Req)
		if m := rowAudienceRe.FindStringSubmatch(RawHead(r.r)); m != nil && r.r.Req != nil {
			faculty := strings.Join(strings.Fields(m[1]), " ")
			r.r.Audience = &types.ReqNode{Type: types.NodeProfile, Attr: "faculty", Value: faculty, Text: m[0]}
		}
		a.audiences(r.children)
	}
}

// appliedToRe matches "Students admitted to the program with ECO101H1/
// ECO105Y1: The course will be applied to Requirement #3 (Higher Years)".
var appliedToRe = regexp.MustCompile(`(?i)^\*?\s*students\s+admitted\s+to\s+the\s+program\s+with\s+(` + grammar.CodePat + `(?:\s*(?:/|,|or)\s*` + grammar.CodePat + `)*):\s*the\s+course\s+will\s+be\s+applied\s+to\s+requirement\s+#?(\d{1,2})\s*\(([^()]+)\)$`)

// appliedTo makes a appliedToRe sentence a note on the row it names when
// that row's pool lists every course already: it restates the pool. It
// reports whether it did.
func (a *Assembler) appliedTo(s string) bool {
	m := appliedToRe.FindStringSubmatch(a.clean(s))
	if m == nil {
		return false
	}
	r := a.labelled(strings.TrimSpace(m[3]) + " " + m[2])
	if r == nil || r.r.Req == nil {
		return false
	}
	have := map[string]bool{}
	grammar.WalkReq(r.r.Req, func(n *types.ReqNode) {
		if n.Type == types.NodeCredits && n.Designators == nil {
			for _, c := range n.Courses {
				have[c] = true
			}
		}
	})
	for _, c := range grammar.CodeRe.FindAllString(m[1], -1) {
		if !have[c] {
			return false
		}
	}
	a.notesTo(r, []string{grammar.CleanNote(strings.TrimLeft(strings.TrimSpace(s), "* "))})
	return true
}

// courseCountCapRe matches a cap counted in courses: "Up to two language
// courses totaling 1.0 credit in Pali ( RLG264H1, RLG265H1) ... will count".
var courseCountCapRe = regexp.MustCompile(`(?i)^(?:up\s+to|a\s+maximum\s+of)\s+(one|two|three|four)\s+[\w-]+\s+courses?\s+totall?ing\s+` + grammar.AmountPat + `\s+credits?\s+(\(?in\s+.+?)\s+will\s+count\s+towards?\b`)

// courseCountCap reads a courseCountCapRe sentence into a limit on its
// courses when the count and the credits agree: every course is a half course.
// The rest of the sentence stays where it goes. Nil when it isn't one.
func courseCountCap(t string) *types.Requirement {
	m := courseCountCapRe.FindStringSubmatch(t)
	if m == nil {
		return nil
	}
	codes := grammar.Dedupe(grammar.CodeRe.FindAllString(m[3], -1))
	v, _ := strconv.ParseFloat(m[2], 64)
	if codes == nil || float64(countWords[strings.ToLower(m[1])])*0.5 != v {
		return nil
	}
	for _, c := range codes {
		if grammar.CourseWeight(c) != 0.5 {
			return nil
		}
	}
	return &types.Requirement{Kind: types.RequirementLimit, Req: &types.ReqNode{Type: types.NodeCredits, Courses: codes, MaxAmount: grammar.MaxAmount(v)}}
}

// openListRe matches a note that the program's lists leave courses out:
// "This list of eligible courses is not exhaustive".
var openListRe = regexp.MustCompile(`(?i)\blists?\s+of\s+(?:[\w-]+\s+){0,2}courses\s+(?:is|are)\s+not\s+(?:an?\s+)?(?:exhaustive|complete)\b`)

// openLists reads a note that the lists are not exhaustive: each pool or floor
// drawing on them becomes or(node, text{the note}), Review rather than NotMet.
// Rows that are or(..., text) already, and limits, are left alone.
func (a *Assembler) openLists(lists []*types.CourseList) {
	i := slices.IndexFunc(a.notes, func(s string) bool { return openListRe.MatchString(a.clean(s)) })
	if i < 0 || len(lists) == 0 {
		return
	}
	s := a.notes[i]
	var wrap func(n *types.ReqNode) *types.ReqNode // nil: unchanged
	wrap = func(n *types.ReqNode) *types.ReqNode {
		if n.Type == types.NodeCredits && n.Lists != nil && n.MaxAmount == nil {
			return programs.OrNode(grammar.CloneNode(n), TextNode(altKind(s), s))
		}
		var c *types.ReqNode
		for k, it := range n.Items {
			if w := wrap(it); w != nil {
				if c == nil {
					c = grammar.CloneNode(n)
				}
				c.Items[k] = w
			}
		}
		return c
	}
	placed := false
	for _, r := range a.allRows() {
		n := r.r.Req
		if n == nil || r.r.Kind == types.RequirementLimit || n.Type == types.NodeOr && slices.ContainsFunc(n.Items, func(it *types.ReqNode) bool { return it.Type == types.NodeText }) {
			continue
		}
		if w := wrap(n); w != nil {
			r.r.Req = w
			addRaw(r, s)
			placed = true
		}
	}
	if placed {
		a.notes = slices.DeleteFunc(a.notes, func(x string) bool { return x == s })
	}
}

// onlyLists reports whether n is an amount from lists and nothing else.
func onlyLists(n *types.ReqNode) bool {
	return n.Type == types.NodeCredits && n.Amount > 0 && n.MaxAmount == nil && n.Lists != nil && n.Courses == nil && n.Designators == nil &&
		n.Levels == nil && n.Breadth == nil && n.Requirements == nil && n.Exclude == nil && n.Items == nil && n.MinGrade == 0 && n.Weight == 0
}

// listCodes returns the courses the named lists hold when each item is a pool
// of listed courses, a pool of such lists, or draws on rows whose bodies only
// list courses; ok is false otherwise.
func (a *Assembler) listCodes(names []string, byName map[string]*types.CourseList) (codes []string, ok bool) {
	return a.listCodesDepth(names, byName, 0)
}

func (a *Assembler) listCodesDepth(names []string, byName map[string]*types.CourseList, depth int) (codes []string, ok bool) {
	if depth > 4 {
		return nil, false
	}
	for _, name := range names {
		l := byName[name]
		if l == nil {
			return nil, false
		}
		for _, it := range l.Items {
			switch {
			case it.Type == types.NodeCredits && it.Courses != nil && it.Designators == nil && it.Levels == nil && it.Breadth == nil &&
				it.Lists == nil && it.Requirements == nil && it.Exclude == nil && it.Weight == 0:
				codes = append(codes, it.Courses...)
			case it.Type == types.NodeCredits && it.Lists != nil && it.Courses == nil && it.Designators == nil && it.Breadth == nil &&
				it.Requirements == nil && it.Exclude == nil && it.Weight == 0:
				cs, ok := a.listCodesDepth(it.Lists, byName, depth+1)
				if !ok {
					return nil, false
				}
				codes = append(codes, slices.DeleteFunc(cs, func(c string) bool { return !grammar.LevelMatches(it.Levels, c) })...)
			case it.Type == types.NodeCredits && it.Courses == nil && it.Designators == nil:
				i := slices.IndexFunc(a.reqRefs, func(x compReqRef) bool { return x.node == it })
				if i < 0 {
					return nil, false
				}
				for _, r := range a.reqRefs[i].rows {
					cs, fixed := listedCourses(r.r.Req)
					if !fixed {
						return nil, false
					}
					codes = append(codes, cs...)
				}
			default:
				return nil, false
			}
		}
	}
	return grammar.Dedupe(codes), codes != nil
}

// listedCourses returns the courses n lists when it is made of course nodes
// and pools of listed courses only.
func listedCourses(n *types.ReqNode) (codes []string, ok bool) {
	ok = n != nil
	grammar.WalkReq(n, func(x *types.ReqNode) {
		switch {
		case x.Type == types.NodeCourse:
			codes = append(codes, x.Code)
		case x.Type == types.NodeCredits && x.Courses != nil && x.Designators == nil && x.Levels == nil && x.Lists == nil && x.Requirements == nil && x.Breadth == nil:
			codes = append(codes, x.Courses...)
		case x.Type == types.NodeAnd || x.Type == types.NodeOr:
		default:
			ok = false
		}
	})
	return codes, ok && codes != nil
}

// yearCoursesRe matches what follows the year phrase in a label naming a
// group of courses: "Fourth Year Courses".
var yearCoursesRe = regexp.MustCompile(`(?i)^\s*courses\s*$`)

var (
	// pairPoolRe matches a pool of single courses and pairs: "1.0 credit
	// from JMB170Y1/ ( MAT148H1, MAT149H1)/ MAT137Y1".
	pairPoolRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\s+from(?:\s+(?:any\s+of\s+)?the\s+following)?:?\s+(.+)$`)
	// pairRe matches a pair in a pool: "( MAT148H1, MAT149H1)".
	pairRe = regexp.MustCompile(`\(\s*(` + grammar.CodePat + `)\s*(?:,|and)\s*(` + grammar.CodePat + `)\s*\)`)
	// pairAfterSlashRe and pairOutRe match a pair with the "/" before
	// it, "/ ( PRT101H1, PRT102H1)", or the separator after it.
	pairAfterSlashRe = regexp.MustCompile(`\s*/\s*\(\s*` + grammar.CodePat + `\s*(?:,|and)\s*` + grammar.CodePat + `\s*\)`)
	pairOutRe        = regexp.MustCompile(`\(\s*` + grammar.CodePat + `\s*(?:,|and)\s*` + grammar.CodePat + `\s*\)\s*[/,]?\s*`)
)

var (
	// onlyTotalFromRe matches "Only 1.0 credit total from Computer Science
	// may count toward the ..., with only 0.5 credit at the 100-level being ...".
	onlyTotalFromRe = regexp.MustCompile(`(?i)^-?\s*only\s+` + grammar.AmountPat + `\s+credits?\s+total\s+from\s+([\w&\s]+?)\s+may\s+count\s+towards?\s+the\s+[\w\s]+?,\s+with\s+only\s+` + grammar.AmountPat + `\s+credits?\s+at\s+the\s+(` + grammar.LevelPat + `)-level\s+being\s+accepted\s+towards?\s+program\s+completion$`)
	// additionalProjectsRe matches "Additional BCH research project courses
	// can be taken ..., but cannot be counted towards the program completion".
	additionalProjectsRe = regexp.MustCompile(`(?i)^additional\s+(` + grammar.DesigPat + `)\s+research\s+project\s+courses\s+can\s+be\s+taken\b.*\bcannot\s+be\s+counted\s+towards?\s+the\s+program(?:\s+completion)?$`)
	// projectCoursesRe matches the research project courses an item names:
	// "research project course: BCH372Y1/ BCH374Y1".
	projectCoursesRe = regexp.MustCompile(`(?i)\bresearch\s+project\s+courses?:\s*((?:` + grammar.CodePat + `)(?:\s*/\s*` + grammar.CodePat + `)*)`)
	// projectCapRe matches the note that caps them: "may count up to 1.0
	// credits from a biochemistry research project course".
	projectCapRe = regexp.MustCompile(`(?i)\bmay\s+count\s+up\s+to\s+` + grammar.AmountPat + `\s+credits?\s+from\s+an?\s+[\w\s]*?research\s+project\s+course\b`)
)

// programCaps reads text constraints that cap courses over the program into
// limits: onlyTotalFromRe's two caps, and additionalProjectsRe's cap
// on the items' project courses at the note's amount. Else the text stays.
func (a *Assembler) programCaps() {
	var projects []string
	walkRows(a.items, func(r *compRow) {
		if m := projectCoursesRe.FindStringSubmatch(r.r.Raw); m != nil && projects == nil {
			projects = grammar.CodeRe.FindAllString(m[1], -1)
		}
	})
	projectCap := 0.0
	for _, s := range a.notes {
		if m := projectCapRe.FindStringSubmatch(s); m != nil {
			projectCap, _ = strconv.ParseFloat(m[1], 64)
		}
	}
	var fix func(rs []*compRow) []*compRow
	fix = func(rs []*compRow) []*compRow {
		var out []*compRow
		for _, r := range rs {
			r.children = fix(r.children)
			out = append(out, r)
			n := r.r.Req
			if r.r.Kind != types.RequirementConstraint || n == nil || n.Type != types.NodeText {
				continue
			}
			s := strings.TrimSpace(a.clean(n.Text))
			if m := onlyTotalFromRe.FindStringSubmatch(s); m != nil {
				desigs, levels := routeFilters(m[2])
				if desigs == nil || levels != nil {
					continue
				}
				v, _ := strconv.ParseFloat(m[1], 64)
				w, _ := strconv.ParseFloat(m[3], 64)
				r.r.Kind, r.r.Req = types.RequirementLimit, &types.ReqNode{Type: types.NodeCredits, Designators: desigs, MaxAmount: grammar.MaxAmount(v)}
				out = append(out, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Raw: r.r.Raw,
					Req: &types.ReqNode{Type: types.NodeCredits, Designators: slices.Clone(desigs), Levels: grammar.ParseLevels(m[4] + "-level"), MaxAmount: grammar.MaxAmount(w)}}})
				continue
			}
			if m := additionalProjectsRe.FindStringSubmatch(s); m != nil && projects != nil && projectCap > 0 &&
				!slices.ContainsFunc(projects, func(c string) bool { return c[:3] != m[1] }) {
				r.r.Kind, r.r.Req = types.RequirementLimit, &types.ReqNode{Type: types.NodeCredits, Courses: slices.Clone(projects), MaxAmount: grammar.MaxAmount(projectCap)}
			}
		}
		return out
	}
	a.floors, a.items, a.program = fix(a.floors), fix(a.items), fix(a.program)
	if a.total != nil {
		a.total.children = fix(a.total.children)
	}
}

var (
	// cognatePartRe matches "6.5 additional PRT credits, of which up to 1.0
	// credit may be taken from: SPA courses at any level, ... or cognate ...".
	cognatePartRe = regexp.MustCompile(`^` + grammar.AmountPat + `\s+additional\s+([A-Z]{3})\s+credits,\s+of\s+which\s+up\s+to\s+` + grammar.AmountPat + `\s+credits?\s+may\s+be\s+taken\s+from:\s+((?:[A-Z]{3}\s+courses\s+at\s+any\s+level,\s+)+)or\s+cognate\s+course\s+offerings\s+in\s+([A-Z]{3}(?:,\s*[A-Z]{3})*)$`)
	// cognateShareRe matches "Of these 9.0 credits, up to 2.0 credits at the
	// 200+ level may be taken from the following cognate ... offerings: EUR".
	cognateShareRe = regexp.MustCompile(`(?i)^of\s+these\s+` + grammar.AmountPat + `\s+credits,\s+up\s+to\s+` + grammar.AmountPat + `\s+credits?\s+at\s+the\s+(` + grammar.LevelPat + `)\+\s+level\s+may\s+be\s+taken\s+from\s+the\s+following\s+cognate\s+departmental\s+or\s+college\s+offerings:\s+(` + grammar.DesigPat + `(?:,\s*` + grammar.DesigPat + `)*)$`)
	// eligibleListRe matches a note on where the cognate offerings are
	// listed: "A complete list of eligible courses is available at ...".
	eligibleListRe = regexp.MustCompile(`(?i)^(?:please,?\s+consult\s+with\b.*\bupdated\s+list\s+of\s+courses\s+pre-approved\b|a\s+complete\s+list\s+of\s+eligible\s+courses\s+is\s+available\b)`)
)

// cognateShares reads an item's cognate share as or(credits{own subjects},
// and(credits{with the cognate}, text{permission})), as the department decides
// what is cognate, with limits on the other subjects under the row.
func (a *Assembler) cognateShares() {
	walkRows(a.items, func(r *compRow) {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr {
			return
		}
		var strict, share, list *types.ReqNode
		var own, open, others []string
		var amount, cap float64
		var level int
		for _, it := range n.Items {
			switch {
			case it.Type == types.NodeCredits && strict == nil:
				strict = it
			case it.Type != types.NodeText:
				return
			case eligibleListRe.MatchString(a.clean(it.Text)):
				list = it
			default:
				s := strings.TrimSpace(a.clean(it.Text))
				if m := cognatePartRe.FindStringSubmatch(s); m != nil {
					amount, _ = strconv.ParseFloat(m[1], 64)
					cap, _ = strconv.ParseFloat(m[3], 64)
					own = append([]string{m[2]}, grammar.DesigRe.FindAllString(m[4], -1)...)
					open = grammar.DesigRe.FindAllString(m[5], -1)
					others = append(slices.Clone(own[1:]), open...)
					share = it
				} else if m := cognateShareRe.FindStringSubmatch(s); m != nil {
					amount, _ = strconv.ParseFloat(m[1], 64)
					cap, _ = strconv.ParseFloat(m[2], 64)
					level, _ = strconv.Atoi(m[3])
					open = grammar.DesigRe.FindAllString(m[4], -1)
					others = slices.Clone(open)
					share = it
				} else {
					return
				}
			}
		}
		if share == nil {
			return
		}
		if strict != nil {
			// The sentence under a pool: the pool is its own designator's.
			if strict.Amount != amount || len(strict.Designators) != 1 || strict.Courses != nil || strict.Levels != nil || strict.Lists != nil {
				return
			}
			own = slices.Clone(strict.Designators)
		} else if own == nil {
			return
		}
		pool := &types.ReqNode{Type: types.NodeCredits, Amount: amount, Designators: own}
		wide := &types.ReqNode{Type: types.NodeCredits, Amount: amount, Designators: grammar.Dedupe(append(slices.Clone(own), open...))}
		text := grammar.CloneNode(share)
		text.Kind = types.TextPermission
		r.r.Req = programs.OrNode(pool, programs.AndNode(wide, text))
		r.children = append(r.children, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Raw: share.Text,
			Req: &types.ReqNode{Type: types.NodeCredits, Designators: others, MaxAmount: grammar.MaxAmount(cap)}}})
		if level > 100 {
			var below []int
			for l := 100; l < level; l += 100 {
				below = append(below, l)
			}
			r.children = append(r.children, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Raw: share.Text,
				Req: &types.ReqNode{Type: types.NodeCredits, Designators: slices.Clone(others), Levels: below, MaxAmount: grammar.MaxAmount(0)}}})
		}
		if list != nil {
			a.notes = append(a.notes, grammar.CleanNote(list.Text))
		}
	})
}

// countNoMoreRe matches "Two seminar courses (1.0 credit), and no more,
// taken from the following: JLP471H1, PSY402H1/ PSY403H1, ...".
var countNoMoreRe = regexp.MustCompile(`(?i)^(?:two|three|four)\s+[a-z]+\s+courses\s+\(` + grammar.AmountPat + `\s+credits?\),\s+and\s+no\s+more,\s+taken\s+from(?:\s+the\s+following)?\s*:\s*` + routeCodesPat + `$`)

// countNoMore reads a countNoMoreRe item: credits{N, the codes}, a
// program limit of N on them, and a one-course limit on each "/" group.
func (a *Assembler) countNoMore() {
	walkRows(a.items, func(r *compRow) {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText {
			return
		}
		m := countNoMoreRe.FindStringSubmatch(strings.TrimSpace(a.clean(n.Text)))
		if m == nil {
			return
		}
		v, _ := strconv.ParseFloat(m[1], 64)
		codes := grammar.CodeRe.FindAllString(m[2], -1)
		if v <= 0 || slices.ContainsFunc(codes, func(c string) bool { return !strings.HasSuffix(c, "H1") }) {
			return
		}
		r.r.Req = &types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: codes}
		for _, part := range strings.Split(m[2], ",") {
			if g := grammar.CodeRe.FindAllString(part, -1); len(g) > 1 && strings.Contains(part, "/") {
				r.children = append(r.children, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Raw: r.r.Raw,
					Req: &types.ReqNode{Type: types.NodeCredits, Courses: g, MaxAmount: grammar.MaxAmount(grammar.CourseWeight(g[0]))}}})
			}
		}
		a.rowUnder(nil, &types.Requirement{Kind: types.RequirementLimit, Raw: r.r.Raw,
			Req: &types.ReqNode{Type: types.NodeCredits, Courses: slices.Clone(codes), MaxAmount: grammar.MaxAmount(v)}})
	})
}

// transferRefusedRe matches "Transfer credits from AP and IB psychology
// are not accepted".
var transferRefusedRe = regexp.MustCompile(`(?i)^transfer\s+credits\s+from\b.*\bare\s+not\s+accepted$`)

// transferUnderItem moves each program-level transferRefusedRe constraint
// under the item on the line before it, when that item lists the subject's
// courses. It stays text.
func (a *Assembler) transferUnderItem() {
	keep := a.program[:0:0]
	for _, r := range a.program {
		if !a.moveTransfer(r) {
			keep = append(keep, r)
		}
	}
	a.program = keep
}

// moveTransfer moves r under its item and reports whether it did.
func (a *Assembler) moveTransfer(r *compRow) bool {
	n := r.r.Req
	if r.r.Kind != types.RequirementConstraint || n == nil || n.Type != types.NodeText {
		return false
	}
	s := strings.TrimSpace(a.clean(n.Text))
	if !transferRefusedRe.MatchString(s) {
		return false
	}
	desigs, _ := routeFilters(s)
	if desigs == nil {
		return false
	}
	prev := ""
	for _, l := range a.lines {
		if strings.Contains(a.clean(l), s) {
			break
		}
		if strings.TrimSpace(l) != "" {
			prev = strings.TrimSpace(l)
		}
	}
	if prev == "" {
		return false
	}
	var item *compRow
	walkRows(a.items, func(it *compRow) {
		lines := strings.Split(strings.TrimSpace(it.r.Raw), "\n")
		if item == nil && it.r.Kind == types.RequirementAllocation && strings.TrimSpace(lines[len(lines)-1]) == prev &&
			slices.ContainsFunc(grammar.CodeRe.FindAllString(it.r.Raw, -1), func(c string) bool { return slices.Contains(desigs, c[:3]) }) {
			item = it
		}
	})
	if item == nil {
		return false
	}
	item.children = append(item.children, r)
	return true
}
