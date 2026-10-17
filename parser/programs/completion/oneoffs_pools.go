package completion

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/parser/grammar"
	"degree-explorer/parser/programs"
	"degree-explorer/types"
)

// One-off readings of pools: designator, discipline, subject and language
// pools, shares of a total, and the pools a gateway or a remark makes.

var (
	// languageAnyLevelRe matches "1.0 credit in German language at any
	// level, including GER100Y1, GER101H1".
	languageAnyLevelRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\s+in\s+\w+\s+language\s+at\s+any\s+level,\s+including\s+((?:` + grammar.CodePat + `,?\s*)+)\.?$`)
	// advancedTopicsRe matches "Students with very advanced previous
	// knowledge of German may in consultation with the Department take a
	// combination of other 300 or 400-level GER topics courses".
	advancedTopicsRe = regexp.MustCompile(`(?i)^students\s+with\s+very\s+advanced\s+previous\s+knowledge\s+of\s+\w+\s+may\s+in\s+consultation\s+with\s+the\s+department\s+take\s+a\s+combination\s+of\s+other\s+([1-4])00\s+or\s+([1-4])00-level\s+(` + grammar.DesigPat + `)\s+topics\s+courses\b`)
)

// languageAtAnyLevel reads or(text{languageAnyLevelRe},
// text{advancedTopicsRe}) as the listed courses, another language course
// of their designator (content), or the topics courses with consultation.
func languageAtAnyLevel(n *types.ReqNode) *types.ReqNode {
	if len(n.Items) != 2 || n.Items[0].Type != types.NodeText || n.Items[1].Type != types.NodeText {
		return nil
	}
	a, b := n.Items[0].Text, n.Items[1].Text
	m := languageAnyLevelRe.FindStringSubmatch(strings.TrimSpace(a))
	t := advancedTopicsRe.FindStringSubmatch(strings.TrimSpace(b))
	if m == nil || t == nil {
		return nil
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	var levels []int
	for _, l := range []string{t[1], t[2]} {
		levels = append(levels, int(l[0]-'0')*100)
	}
	codes := grammar.CodeRe.FindAllString(m[2], -1)
	// the language's other courses are its designator's (ASCER1401); the content
	// text decides which are language courses
	content := TextNode(types.TextContent, a)
	if !slices.ContainsFunc(codes, func(c string) bool { return c[:3] != codes[0][:3] }) {
		content = programs.AndNode(&types.ReqNode{Type: types.NodeCredits, Amount: v, Designators: []string{codes[0][:3]}}, content)
	}
	return programs.OrNode(
		&types.ReqNode{Type: types.NodeCredits, Amount: v, Courses: codes},
		content,
		programs.AndNode(&types.ReqNode{Type: types.NodeCredits, Amount: v, Designators: []string{t[3]}, Levels: levels}, TextNode(types.TextPermission, b)),
	)
}

var (
	// internshipFormRe matches a note naming a course that meets the item:
	// "May take the form of an internship course, GER391H1".
	internshipFormRe = regexp.MustCompile(`(?i)^may\s+take\s+the\s+form\s+of\s+an\s+internship\s+course,\s+(` + grammar.CodePat + `)\b`)
	// studyAbroadSuchAsRe matches "Students may also complete a study
	// abroad course in a German-speaking country, such as GER354Y0".
	studyAbroadSuchAsRe = regexp.MustCompile(`(?i)^students\s+may\s+also\s+complete\s+a\s+study\s+abroad\s+course\b[^.]*?,\s+such\s+as\s+(` + grammar.CodePat + `)\.?$`)
)

// globalExperience adds the courses internshipFormRe and
// studyAbroadSuchAsRe name as alternatives; the study abroad sentence
// stays as content for the courses it doesn't name.
func globalExperience(r *compRow) *types.ReqNode {
	n := r.r.Req
	i := slices.IndexFunc(r.r.Notes, func(s string) bool { return internshipFormRe.MatchString(s) })
	j := slices.IndexFunc(n.Items, func(x *types.ReqNode) bool {
		return x.Type == types.NodeText && studyAbroadSuchAsRe.MatchString(strings.TrimSpace(x.Text))
	})
	if i < 0 || j < 0 {
		return nil
	}
	internship := internshipFormRe.FindStringSubmatch(r.r.Notes[i])[1]
	abroad := studyAbroadSuchAsRe.FindStringSubmatch(strings.TrimSpace(n.Items[j].Text))[1]
	items := []*types.ReqNode{{Type: types.NodeCourse, Code: internship}, {Type: types.NodeCourse, Code: abroad}}
	for k, x := range n.Items {
		if k == j {
			x = TextNode(types.TextContent, x.Text)
		}
		items = append(items, x)
	}
	addRaw(r, r.r.Notes[i])
	if r.r.Notes = slices.Delete(r.r.Notes, i, i+1); len(r.r.Notes) == 0 {
		r.r.Notes = nil
	}
	return programs.OrNode(items...)
}

var (
	// gatewayRe matches courses grouped under the disciplines that offer
	// them: "1.0 credit from the gateway courses in American Studies ( AMS100H1),
	// English ( ENG250H1 and ENG270H1), ...".
	gatewayRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+credits?\s+from\s+the\s+[\w-]+\s+courses\s+in\s+(.+)$`)
	// gatewayGroupRe matches one discipline and its courses, and the
	// separator after it: "English ( ENG250H1 and ENG270H1), ".
	gatewayGroupRe = regexp.MustCompile(`^\s*[A-Z][A-Za-z]*(?:\s+[A-Z][A-Za-z]*)*\s*\(\s*(` + grammar.CodePat + `(?:\s+and\s+` + grammar.CodePat + `)*)\s*\)\s*(?:,\s*(?:or\s+)?|,?\s+or\s+|$)`)
	// additionalUnionRe matches Req 1's pool as designators or lists:
	// "Additional AMS/USA courses or courses from the American Studies Program
	// disciplinary/thematic clusters to a total of 7.0 credits".
	additionalUnionRe = regexp.MustCompile(`(?i)^additional\s+(` + grammar.DesigPat + `(?:/` + grammar.DesigPat + `)*)\s+courses\s+or\s+courses\s+from\s+the\s+[\w\s/-]*?\b(clusters|groups)\s+to\s+a\s+total\s+of\s+` + grammar.AmountPat + `\s+credits?(?:,\s*including\s+requirement\s+#?\d+\s+above)?$`)
)

// gatewayGroups reads gatewayRe as the or of the groups that make the
// amount, each the and of its courses. Whether courses of several groups may
// make it is the text's ambiguity: the pool of all of them is the Review
// alternative. It reports whether it read the row.
func (a *Assembler) gatewayGroups(row *compRow) bool {
	n := row.r.Req
	if row.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || n.Kind != types.TextOther || len(row.children) > 0 {
		return false
	}
	m := gatewayRe.FindStringSubmatch(strings.TrimSuffix(Clean(n.Text), "."))
	if m == nil {
		return false
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	var strict []*types.ReqNode
	var all []string
	for rest := m[2]; rest != ""; {
		g := gatewayGroupRe.FindStringSubmatch(rest)
		if g == nil {
			return false
		}
		rest = rest[len(g[0]):]
		codes := grammar.CodeRe.FindAllString(g[1], -1)
		all = append(all, codes...)
		sum := 0.0
		var group []*types.ReqNode
		for _, c := range codes {
			sum += grammar.CourseWeight(c)
			group = append(group, &types.ReqNode{Type: types.NodeCourse, Code: c})
		}
		if math.Abs(sum-amount) < 1e-9 {
			strict = append(strict, programs.AndNode(group...))
		}
	}
	if len(strict) == 0 || len(all) < 2 {
		return false
	}
	lenient := programs.AndNode(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: grammar.Dedupe(all)}, TextNode(types.TextAmbiguity, n.Text))
	row.r.Req = programs.OrNode(append(strict, lenient)...)
	return true
}

// additionalUnion reads additionalUnionRe as Req 1's pool: a union of
// the designators and every list named by the word ("Cluster 1", ...). The
// row joins Req 1's raw, routes and notes. It reads nothing unless Req 1 is a
// bare pool of the total's amount and two lists are named.
func (a *Assembler) additionalUnion(row *compRow, text string, alts []*types.ReqNode, ls compListRefs) bool {
	m := additionalUnionRe.FindStringSubmatch(strings.TrimSuffix(Clean(text), "."))
	if m == nil || a.total == nil || len(row.children) > 0 || !slices.Contains(a.items, row) {
		return false
	}
	pool := a.total.r.Req
	v, _ := strconv.ParseFloat(m[3], 64)
	if pool == nil || pool.Type != types.NodeCredits || pool.Amount != v || pool.Designators != nil || pool.Levels != nil || pool.Courses != nil || pool.Lists != nil {
		return false
	}
	word := strings.TrimSuffix(strings.ToLower(m[2]), "s")
	var names []string
	for _, l := range ls {
		if l.list != nil && !l.bad && strings.HasPrefix(strings.ToLower(l.list.Name), word+" ") {
			names = append(names, l.list.Name)
		}
	}
	if names = grammar.Dedupe(names); len(names) < 2 {
		return false
	}
	slices.Sort(names)
	name, _, ok := a.addUnion(row, []*types.ReqNode{
		{Type: types.NodeCredits, Designators: strings.Split(m[1], "/")},
		{Type: types.NodeCredits, Lists: names},
	}, ls)
	if !ok {
		return false
	}
	pool.Lists = []string{name}
	if alts != nil {
		a.total.r.Req = programs.OrNode(append([]*types.ReqNode{pool}, alts...)...)
	}
	a.total.r.Raw += "\n" + row.r.Raw
	a.total.r.Notes = append(a.total.r.Notes, row.r.Notes...)
	return true
}

// exclusionExceptRe matches a note lifting an item's exclusions: "the
// exclusion of GER400H1 and GER401H1 does not apply if the student's entry
// level is post- GER100Y1".
var exclusionExceptRe = regexp.MustCompile(`(?i)\bthe\s+exclusion\s+of\s+(.+?)\s+does\s+not\s+apply\s+if\b`)

// exclusionExceptions reads exclusionExceptRe as the pool's lenient
// alternative, or(pool, and(pool without the exclusions, text)): Met without
// the excluded courses, Review with them.
func (a *Assembler) exclusionExceptions() {
	a.notes = slices.DeleteFunc(a.notes, func(note string) bool {
		m := exclusionExceptRe.FindStringSubmatch(note)
		if m == nil {
			return false
		}
		codes := grammar.CodeRe.FindAllString(m[1], -1)
		for _, r := range a.items {
			n := r.r.Req
			if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeCredits || len(codes) == 0 || slices.ContainsFunc(codes, func(c string) bool { return !slices.Contains(n.Exclude, c) }) {
				continue
			}
			wide := *n
			wide.Exclude = slices.DeleteFunc(slices.Clone(n.Exclude), func(c string) bool { return slices.Contains(codes, c) })
			if len(wide.Exclude) == 0 {
				wide.Exclude = nil // omitempty
			}
			// the student's entry level is the placement test's
			r.r.Req = programs.OrNode(n, programs.AndNode(&wide, TextNode(types.TextPlacement, note)))
			addRaw(r, note)
			return true
		}
		return false
	})
}

// trailingCodeRe matches a pool ending at a ";" before one more
// course: "1.0 credit from FOR300H1, ..., FOR310H1; EEB321H1".
var trailingCodeRe = regexp.MustCompile(grammar.NumPat + `\s+credits?\s+from\s+[^;]+;\s*(` + grammar.CodePat + `)\.?$`)

// semicolonPoolRe matches a pool whose ";" separates groups of its
// courses: "1.0 credit from MSE101H1; ECO101H1, ECO102H1".
var semicolonPoolRe = regexp.MustCompile(grammar.NumPat + `\s+credits?\s+from\s+[^;]+;`)

// trailingCode reads trailingCodeRe by the ambiguity policy: the last
// course as its own item is strict, the pool taking it is Review. Where another
// item's pool has ";" between groups, the pool reading stands alone
// (coordinator ruling on ASMAJ1098, ASSPE1102, ASMAJ1100, ASSPE1100).
func (a *Assembler) trailingCode(r *compRow) {
	n := r.r.Req
	m := trailingCodeRe.FindStringSubmatch(programs.StripEmphasis(RawHead(r.r)))
	if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeAnd || m == nil {
		return
	}
	if slices.ContainsFunc(a.items, func(o *compRow) bool {
		return o != r && semicolonPoolRe.MatchString(programs.StripEmphasis(RawHead(o.r)))
	}) {
		return
	}
	for i, it := range n.Items {
		if !isPool(it) || it.Designators != nil || it.Levels != nil || len(it.Courses) < 3 || it.Courses[len(it.Courses)-1] != m[1] {
			continue
		}
		pool := grammar.CloneNode(it)
		pool.Courses = pool.Courses[:len(pool.Courses)-1]
		strict := slices.Clone(n.Items)
		strict[i] = pool
		strict = append(strict, &types.ReqNode{Type: types.NodeCourse, Code: m[1]})
		r.r.Req = programs.OrNode(programs.AndNode(strict...), programs.AndNode(n, TextNode(types.TextAmbiguity, m[1]+" may be one of the pool's courses rather than a course of its own")))
		return
	}
}

// namedLanguagesRe matches a pool's languages: "in Modern Stream
// languages: Arabic, Hebrew, Persian, Turkish" (ASMAJ2669).
var namedLanguagesRe = regexp.MustCompile(`\blanguages:\s*([A-Z][a-z]+(?:,\s*(?:and\s+)?[A-Z][a-z]+)*)\.?$`)

// namedLanguageRe matches a capitalised word: "Spanish".
var namedLanguageRe = regexp.MustCompile(`[A-Z][a-z]+`)

// languagesNamed gives and(credits{amount}, text{namedLanguagesRe}) in
// r's body the languages' designators; the text stays, since which of their
// courses count is the course's language.
func languagesNamed(r *compRow) {
	grammar.WalkReq(r.r.Req, func(n *types.ReqNode) {
		if n.Type != types.NodeAnd || len(n.Items) != 2 || n.Items[0].Type != types.NodeCredits || n.Items[1].Type != types.NodeText ||
			n.Items[1].Kind != types.TextContent {
			return
		}
		pool := n.Items[0]
		m := namedLanguagesRe.FindStringSubmatch(n.Items[1].Text)
		if m == nil || pool.Designators != nil || pool.Courses != nil || pool.Lists != nil || pool.Levels != nil || pool.Requirements != nil {
			return
		}
		var desigs []string
		for _, l := range namedLanguageRe.FindAllString(m[1], -1) {
			d := languageDesignators[strings.ToLower(l)]
			if d == nil {
				return
			}
			desigs = append(desigs, d...)
		}
		pool.Designators = grammar.Dedupe(desigs)
	})
}

// oneDisciplineRe matches "1.0 credit in ONE other discipline offered
// within the Faculty of Arts & Science – African Studies, ..., and other
// departments as approved by the program coordinator".
var oneDisciplineRe = regexp.MustCompile(`(?i)^` + grammar.AmountPat + `\s+(credits?)\s+in\s+one\s+other\s+discipline\s+offered\s+within\s+the\s+Faculty\s+of\s+Arts\s+(?:&|and)\s+Science\s*[–—-]\s*(.+?),?\s+and\s+other\s+departments\s+as\s+approved\s+by\s+the\s+program\s+coordinator$`)

// oneDiscipline reads oneDisciplineRe (ASMAJ1023, ASSPE1023) as an or of
// the amount from each discipline's designator, content text for those with
// none, and a permission route. An unknown name leaves the row.
func oneDiscipline(r *compRow) {
	n := r.r.Req
	if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || n.Kind != types.TextOther || len(r.children) > 0 {
		return
	}
	m := oneDisciplineRe.FindStringSubmatch(strings.TrimSuffix(Clean(n.Text), "."))
	if m == nil {
		return
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	var alts []*types.ReqNode
	var content []string
	for _, name := range strings.Split(m[3], ",") {
		name = strings.TrimSpace(name)
		if d, ok := disciplineDesignators[strings.ToLower(name)]; ok {
			alts = append(alts, &types.ReqNode{Type: types.NodeCredits, Amount: amount, Designators: []string{d}})
		} else if contentDisciplines[strings.ToLower(name)] {
			content = append(content, name)
		} else {
			return
		}
	}
	any := func() *types.ReqNode { return &types.ReqNode{Type: types.NodeCredits, Amount: amount} }
	if content != nil {
		alts = append(alts, programs.AndNode(any(), TextNode(types.TextContent, m[1]+" "+m[2]+" in ONE of "+strings.Join(content, ", "))))
	}
	alts = append(alts, programs.AndNode(any(), TextNode(types.TextPermission, "other departments as approved by the program coordinator")))
	r.r.Req = programs.OrNode(alts...)
}

// categoryElectives reads electivesLeadRe as the pool of the categories
// its notes set out. A notLimitRe note keeps any course beside it as the
// lenient reading (Review).
func (a *Assembler) categoryElectives(row *compRow) bool {
	m := electivesLeadRe.FindStringSubmatch(Clean(row.r.Req.Text))
	if m == nil {
		return false
	}
	var letters []string
	var codes []string
	var read []string
	cats, open := 0, ""
	for _, n := range row.r.Notes {
		t := Clean(n)
		switch {
		case categoriesNoteRe.MatchString(t):
			letters = categoriesNoteRe.FindStringSubmatch(t)[1:]
			read = append(read, n)
		case categoryCodesRe.MatchString(t):
			codes = append(codes, grammar.CodeRe.FindAllString(t, -1)...)
			read = append(read, n)
			cats++
		case notLimitRe.MatchString(t):
			open = t
		}
	}
	if letters == nil || letters[0] >= letters[1] || cats != int(letters[1][0]-letters[0][0])+1 {
		return false
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	body := &types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: grammar.Dedupe(codes)}
	if open != "" {
		body = programs.OrNode(body, programs.AndNode(&types.ReqNode{Type: types.NodeCredits, Amount: amount}, TextNode(types.TextAmbiguity, open)))
	}
	row.r.Req = body
	for _, n := range read {
		addRaw(row, n)
	}
	row.r.Notes = slices.DeleteFunc(row.r.Notes, func(n string) bool { return slices.Contains(read, n) })
	return true
}

// disciplineDesignators maps a discipline to the one designator its courses
// carry. Disciplines without one are in contentDisciplines.
var disciplineDesignators = map[string]string{
	"african studies": "AFR", "anthropology": "ANT", "art history": "FAH", "caribbean studies": "CAR",
	"cinema studies": "CIN", "critical studies in equity and solidarity": "CSE", "drama": "DRM",
	"east asian studies": "EAS", "history": "HIS", "political science": "POL", "philosophy": "PHL",
	"psychology": "PSY", "sociology": "SOC", "women and gender studies": "WGS",
}

// contentDisciplines holds the disciplines no one designator gives: which
// courses are theirs is the course's content.
var contentDisciplines = map[string]bool{
	"classics": true, "material culture and semiotics": true, "music": true, "renaissance studies": true,
}

// orCommaSepRe matches an "or" or comma separator: " or ".
var orCommaSepRe = regexp.MustCompile(`\s+or\s+|,\s+`)

// subjectRows reads rows of text whose subject decides their courses:
// inLanguagesRe as the listed courses or and(credits{the languages'
// designators}, text{content}), and researchCourseRe as or(the codes,
// text{content}).
func (a *Assembler) subjectRows() {
	for _, r := range a.allRows() {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || n.Kind != types.TextOther || len(r.children) > 0 {
			continue
		}
		t := strings.TrimSuffix(Clean(n.Text), ".")
		if m := inLanguagesRe.FindStringSubmatch(t); m != nil {
			names := orCommaSepRe.Split(m[2]+m[3], -1)
			var desigs []string
			for _, name := range names {
				d := languageDesignators[strings.ToLower(name)]
				if d == nil {
					desigs = nil
					break
				}
				desigs = append(desigs, d...)
			}
			if desigs == nil {
				continue
			}
			amount, _ := strconv.ParseFloat(m[1], 64)
			if codes, keep := languageCodeLines(names, r.r.Notes); codes != nil {
				// "Hebrew language courses: MHB155H1/ ..." lists them
				r.r.Req, r.r.Notes = &types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: codes}, keep
				continue
			}
			r.r.Req = programs.AndNode(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Designators: grammar.Dedupe(desigs)}, TextNode(types.TextContent, n.Text))
			continue
		}
		if m := researchCourseRe.FindStringSubmatch(t); m != nil {
			amount, _ := strconv.ParseFloat(m[1], 64)
			r.r.Req = programs.OrNode(&types.ReqNode{Type: types.NodeCredits, Amount: amount, Courses: grammar.Dedupe(grammar.CodeRe.FindAllString(m[3], -1))}, TextNode(types.TextContent, m[2]))
		}
	}
}

// languageCodeLines returns the courses of the notes that list each
// language's courses ("Yiddish language courses: GER260Y1/ ...") and the other
// notes; nil when a language has no such line.
func languageCodeLines(names, notes []string) (codes, keep []string) {
	found := map[string]bool{}
	for _, n := range notes {
		name, list, ok := strings.Cut(n, " language courses:")
		if ok && slices.ContainsFunc(names, func(x string) bool { return strings.EqualFold(x, strings.TrimSpace(name)) }) && IsCodeList(strings.TrimSpace(list)) {
			found[strings.ToLower(strings.TrimSpace(name))] = true
			codes = append(codes, grammar.CodeRe.FindAllString(list, -1)...)
			continue
		}
		keep = append(keep, n)
	}
	if len(found) != len(names) {
		return nil, notes
	}
	return grammar.Dedupe(codes), keep
}

// shareDesigRe matches a designator: "FRE".
var shareDesigRe = regexp.MustCompile(`[A-Z]{3}`)

// desigCoursesRe matches a designator's courses: "FRE language courses".
var desigCoursesRe = regexp.MustCompile(`\b([A-Z]{3})\s+(?:language\s+)?courses\b`)

// shareOfTotal reads shareOfTotalRe on a total of any credits: the
// program's one designator and the share's are the strict pool, any course
// the lenient one.
func (a *Assembler) shareOfTotal() {
	if a.total == nil {
		return
	}
	n := a.total.r.Req
	if n == nil || n.Type != types.NodeCredits || !isAnyCredits(n) {
		return
	}
	var sent string
	for _, l := range strings.Split(a.total.r.Raw, "\n") {
		if shareOfTotalRe.MatchString(strings.TrimSpace(l)) {
			sent = strings.TrimSuffix(strings.TrimSpace(l), ".")
		}
	}
	if sent == "" {
		return
	}
	m := shareOfTotalRe.FindStringSubmatch(sent)
	shared := shareDesigRe.FindAllString(m[2], -1)
	var own []string
	for _, c := range grammar.CodeRe.FindAllString(a.raw, -1) {
		own = append(own, c[:3])
	}
	for _, d := range desigCoursesRe.FindAllStringSubmatch(a.raw, -1) {
		own = append(own, d[1])
	}
	own = slices.DeleteFunc(grammar.Dedupe(own), func(d string) bool { return slices.Contains(shared, d) })
	if len(own) != 1 {
		return
	}
	strict := &types.ReqNode{Type: types.NodeCredits, Amount: n.Amount, Designators: append(own, shared...)}
	a.total.r.Req = programs.OrNode(strict, programs.AndNode(&types.ReqNode{Type: types.NodeCredits, Amount: n.Amount}, TextNode(types.TextAmbiguity, sent)))
}

// fromRowsDesignator adds to a list pool the designator's courses of the two
// rows fromRowsDesigRe names.
func (a *Assembler) fromRowsDesignator(lists []*types.CourseList) {
	named := func(name string) *compRow {
		var found []*compRow
		walkRows(a.items, func(r *compRow) {
			if r.r.Kind == types.RequirementAllocation && (strings.EqualFold(r.r.Label, name) || r.r.Label == "" && strings.EqualFold(r.r.Section+" Courses", name)) {
				found = append(found, r)
			}
		})
		if len(found) != 1 {
			return nil
		}
		return found[0]
	}
	walkRows(a.items, func(r *compRow) {
		n := r.r.Req
		if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr || n.Items[0].Type != types.NodeCredits || len(n.Items[0].Lists) != 1 {
			return
		}
		li := slices.IndexFunc(lists, func(l *types.CourseList) bool { return l.Name == n.Items[0].Lists[0] })
		for i, it := range n.Items {
			m := fromRowsDesigRe.FindStringSubmatch(strings.TrimSpace(it.Text))
			if it.Type != types.NodeText || m == nil || li < 0 {
				continue
			}
			x, y := named(m[2]), named(m[3])
			if x == nil || y == nil {
				return
			}
			var codes []string
			for _, row := range []*compRow{x, y} {
				for _, c := range listedCodes(row.r.Req, lists) {
					if c[:3] == m[1] {
						codes = append(codes, c)
					}
				}
			}
			if codes == nil {
				return
			}
			lists[li].Items = append(lists[li].Items, &types.ReqNode{Type: types.NodeCredits, Courses: grammar.Dedupe(codes)})
			n.Items = slices.Delete(n.Items, i, i+1)
			r.r.Req = programs.OrNode(n.Items...)
			return
		}
	})
}

// listedCodes returns the courses n names, its lists' included; texts and
// filters name none.
func listedCodes(n *types.ReqNode, lists []*types.CourseList) []string {
	if n == nil || n.Type == types.NodeText {
		return nil
	}
	var out []string
	if n.Type == types.NodeCourse {
		out = append(out, n.Code)
	}
	out = append(out, n.Courses...)
	for _, name := range n.Lists {
		for _, l := range lists {
			if l.Name == name {
				for _, it := range l.Items {
					out = append(out, listedCodes(it, lists)...)
				}
			}
		}
	}
	for _, it := range n.Items {
		out = append(out, listedCodes(it, lists)...)
	}
	return out
}

// selectRemaining reads selectRemainingRe as Req 1's pool, with a limit
// on the capped courses; the row goes. A "/" choice isn't capped: Req 1 counts
// what the rows above claim too, which may hold the other course.
func (a *Assembler) selectRemaining(r *compRow) {
	n := r.r.Req
	if a.total == nil || r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText {
		return
	}
	t := a.total.r.Req
	m := selectRemainingRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(Clean(n.Text)), "."))
	if m == nil || t == nil || t.Type != types.NodeCredits || !isAnyCredits(t) || fmtAmount(t.Amount) != m[1] || !IsCodeList(m[4]) {
		return
	}
	pool := m[2]
	var desigs, excl []string
	for _, g := range allDesigExceptRe.FindAllStringSubmatch(pool, -1) {
		desigs = append(desigs, g[1])
		excl = append(excl, grammar.CodeRe.FindAllString(g[2], -1)...)
	}
	pool = allDesigExceptRe.ReplaceAllString(pool, "; ")
	if desigs == nil || !IsCodeList(strings.Trim(pool, "; ")) {
		return
	}
	capped := grammar.Dedupe(grammar.CodeRe.FindAllString(m[4], -1))
	v, _ := strconv.ParseFloat(m[3], 64)
	t.Designators, t.Exclude = desigs, excl
	t.Courses = grammar.Dedupe(append(grammar.CodeRe.FindAllString(pool, -1), capped...))
	addRaw(a.total, RawHead(r.r))
	a.dropRow(r)
	a.program = append(a.program, r.children...)
	a.program = append(a.program, &compRow{r: &types.Requirement{Kind: types.RequirementLimit, Raw: "no more than " + m[3] + " credit from the following",
		Req: &types.ReqNode{Type: types.NodeCredits, Courses: capped, MaxAmount: grammar.MaxAmount(v)}}})
}

// relatedSelected reads relatedSelectedRe as credits of the designator
// at the level, with a constraint of the floor and one of the share from the
// courses named or, with permission, the designator's.
func (a *Assembler) relatedSelected(r *compRow) {
	n := r.r.Req
	if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || len(r.children) > 0 {
		return
	}
	m := relatedSelectedRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(Clean(n.Text)), "."))
	if m == nil {
		return
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	floor, _ := strconv.ParseFloat(m[5], 64)
	share, _ := strconv.ParseFloat(m[8], 64)
	r.r.Req = &types.ReqNode{Type: types.NodeCredits, Amount: v, Designators: []string{m[2]}, Levels: grammar.ParseLevels(m[3] + "00+-level")}
	r.children = append(r.children,
		&compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Section: r.r.Section, Raw: m[4],
			Req: &types.ReqNode{Type: types.NodeCredits, Amount: floor, Levels: grammar.ParseLevels(m[6] + "00-level")}}},
		&compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Section: r.r.Section, Raw: m[7],
			Req: programs.OrNode(&types.ReqNode{Type: types.NodeCredits, Amount: share, Courses: grammar.CodeRe.FindAllString(m[9], -1)},
				programs.AndNode(&types.ReqNode{Type: types.NodeCredits, Amount: share, Designators: []string{m[2]}}, TextNode(types.TextPermission, m[10])))}})
}
