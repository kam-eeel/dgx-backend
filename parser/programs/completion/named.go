package completion

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/parser/grammar"
	"degree-explorer/parser/programs"
	"degree-explorer/types"
)

// Named lists: the lines that set out lists become lists once every row is
// read, and the program's lists are kept by key for references to them.

// compListDef is one line or block that sets out a list.
type compListDef struct {
	title   string   // the heading or label as written: "Group A (Ethics)"
	members string   // the members as written: "CSC209H1; any 300-/ 400-level CSC course"
	note    string   // the note it was kept as
	block   *Block   // the line, for the headings over it; nil for a sentence
	row     *compRow // the row whose note it is; nil for the program's or a sentence's
}

// listDef keeps a line that sets out a list as a note on row (the program's
// when nil) and records it as a list definition.
func (a *Assembler) listDef(row *compRow, title, members string, b *Block, note string) {
	a.notesTo(row, []string{note})
	a.listDefs = append(a.listDefs, &compListDef{title: title, members: members, note: note, block: b, row: row})
}

// compList is a list while the program is read.
type compList struct {
	key    string // listKey of its name
	list   *types.CourseList
	notes  []string // its definitions' notes, which leave the notes when it is used
	bad    bool     // a member isn't a code or a filter: the list can't be used
	used   bool
	parent string   // the key of the list over it (Cluster A under Group 1)
	except []string // keys of lists whose codes its filters exclude
	defs   []*compListDef
	titled bool   // named by its title ("Pre-1800 British Literature"), not "Group A"
	line   int    // the first line defining or heading it, -1 for none: the storage order
	perm   string // a member a person decides: a route on the rows using the list
}

var (
	// listIDPat matches a list's letter, number or numeral, with a
	// sub-list's numeral after it: "C (ii)".
	listIDPat = `(?-i:[A-Z]|\d{1,2}|[IVX]{1,4})\b(?:\s*\(\s*(?-i:i{1,3}|iv|v|vi)\s*\))?`
	// listKindPat matches the kind of a list's name: "Sub-group".
	listKindPat = `(?:groups?|lists?|clusters?|sub-?groups?|categor(?:y|ies))`
	// listNameRe matches a list's name at the start of its title: "Cluster
	// B" in "Cluster B - Learning".
	listNameRe = regexp.MustCompile(`(?i)^(?:the\s+)?(?:course\s+|disciplinary/thematic\s+)?(` + listKindPat + `)\s+(` + listIDPat + `)(?:\s*$|\s*[-–:(]|\s+\()`)
	// coreGroupRe matches "the Core Group", a list named by a word.
	coreGroupRe = regexp.MustCompile(`(?i)^(?:the\s+)?(core)\s+group\b`)
)

// listKey is the form a list's name is matched in: "subgroup c(ii)".
func listKey(kind, id string) string {
	kind = strings.ToLower(strings.ReplaceAll(kind, "-", ""))
	switch {
	case strings.HasPrefix(kind, "categor"):
		kind = "category"
	default:
		kind = strings.TrimSuffix(kind, "s")
	}
	return kind + " " + strings.Join(strings.Fields(strings.NewReplacer("(", " (", ")", ") ").Replace(id)), "")
}

// listName returns a list's name and key from its title, or "" when the
// title names none.
func listName(title string) (name, key string) {
	t := strings.TrimSpace(programs.StripEmphasis(title))
	if m := listNameRe.FindStringSubmatch(t); m != nil {
		key := listKey(m[1], m[2])
		// the kind as the key normalises it, capitalised
		kind := strings.Fields(key)[0]
		kind = strings.ToUpper(kind[:1]) + kind[1:]
		id := strings.Join(strings.Fields(strings.NewReplacer("(", " (", ")", ")").Replace(m[2])), " ")
		return kind + " " + id, key
	}
	if m := coreGroupRe.FindStringSubmatch(t); m != nil {
		return "Core Group", "core group"
	}
	return "", ""
}

// at records that line defines or heads l.
func (l *compList) at(line int) {
	if line >= 0 && (l.line < 0 || line < l.line) {
		l.line = line
	}
}

// titleKey is the key of a list named by its title: lower case, without
// "courses".
func titleKey(title string) string {
	t := strings.ToLower(strings.Join(strings.Fields(title), " "))
	return "title:" + strings.TrimSpace(strings.TrimSuffix(t, " courses"))
}

// buildLists reads the list definitions into lists, in the order they are
// first defined.
func (a *Assembler) buildLists() []*compList {
	var out []*compList
	byKey := map[string]*compList{}
	get := func(name, key string) *compList {
		l := byKey[key]
		if l == nil {
			l = &compList{key: key, list: &types.CourseList{Name: name}, line: -1}
			byKey[key] = l
			out = append(out, l)
		}
		return l
	}
	titles := map[string][]string{}
	defs := append(slices.Clone(a.listDefs), a.memberLines()...)
	// titleName is the name and key of a list named by its title: "Pre-1800
	// British Literature:".
	titleName := func(d *compListDef) (string, string) {
		name := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(programs.StripEmphasis(d.title)), ":"))
		if name == "" || d.block == nil || BlkNoteLabelRe.MatchString(name) {
			return "", ""
		}
		return name, titleKey(name)
	}
	// sub-lists of different lists with one title ("Geography:" in List A and
	// List B) are different lists, keyed by their parents
	parents := map[string]map[string]bool{}
	for _, d := range defs {
		if _, key := listName(d.title); key == "" {
			if _, tk := titleName(d); tk != "" {
				if _, pk, _ := listParent(d, tk); pk != "" {
					if parents[tk] == nil {
						parents[tk] = map[string]bool{}
					}
					parents[tk][pk] = true
				}
			}
		}
	}
	for _, d := range defs {
		name, key := listName(d.title)
		titled := false
		if key == "" {
			if name, key = titleName(d); key == "" {
				continue
			}
			titled = true
		}
		pn, pk, h := listParent(d, key)
		if titled && pk != "" && len(parents[key]) > 1 {
			name, key = name+" ("+pn+")", key+"@"+pk
		}
		l := get(name, key)
		l.titled = titled
		l.defs = append(l.defs, d)
		l.at(defLine(d))
		l.notes = append(l.notes, d.note)
		if t := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(programs.StripEmphasis(d.title)), ":")); t != name {
			titles[key] = append(titles[key], t)
		}
		if pk != "" {
			p := get(pn, pk)
			p.at(h.line)
			if l.parent == "" {
				l.parent = pk
				p.list.Items = append(p.list.Items, &types.ReqNode{Type: types.NodeCredits, Lists: []string{name}})
				if t := strings.TrimSpace(h.Section); t != pn {
					titles[pk] = append(titles[pk], t)
				}
			}
		}
		items, except, perm, ok := listMembersPerm(d.members)
		if perm != "" {
			l.perm = perm
		}
		switch {
		case !ok && listTitleOnlyRe.MatchString(strings.TrimSpace(d.members)) && !anyCodeRe.MatchString(d.members):
			// "**Group A:** Theoretical and Methods 1: Theoretical": a part's name
		case !ok:
			l.bad = true
		case items == nil && grammar.CodeRe.MatchString(d.members):
			l.bad = true
		default:
			l.list.Items = append(l.list.Items, items...)
			l.except = append(l.except, except...)
		}
	}
	for _, l := range out {
		if ts := grammar.Dedupe(titles[l.key]); len(ts) == 1 {
			l.list.Title = ts[0]
		}
		l.list.Items = mergeListItems(l.list.Items)
		if len(l.list.Items) == 0 {
			l.bad = true
		}
	}
	return out
}

// listParent returns the list over definition d: the first heading over it
// with a list name, when that list is of another kind; "" when there is none.
func listParent(d *compListDef, key string) (name, pkey string, h *Block) {
	if d.block == nil {
		return "", "", nil
	}
	for h := d.block.Heading; h != nil; h = h.Heading {
		if h.Kind != BlockHeading {
			continue
		}
		if pn, pk := listName(h.Section); pk != "" && pk != key {
			if strings.Fields(pk)[0] != strings.Fields(key)[0] {
				return pn, pk, h
			}
			break
		}
	}
	return "", "", nil
}

var (
	// listLevelsPat matches levels in a list's filter: "300-/ 400".
	listLevelsPat = grammar.LevelPat + `(?:\s*\+|(?:\s*-?\s*/\s*|\s*,\s*|\s+(?:or|and)\s+)` + grammar.LevelPat + `)*\+?`
	// listFilterRe matches a member by filter: "any 300-/ 400-level CSC
	// course except those listed in Group B".
	listFilterRe = regexp.MustCompile(`(?i)\bany\s+(?:other\s+)?(?:(` + listLevelsPat + `)\s*-?\s*level\s+)?(` + grammar.DesigPat + `(?:\s*(?:,|/|or|and)\s*` + grammar.DesigPat + `)*)(?:\s*(` + listLevelsPat + `)|\s+courses?(?:\s+at\s+the\s+(` + listLevelsPat + `)\s*-?\s*level)?)(?:\s+(?:(?:except|excluding)\s+(?:((?:` + grammar.CodePat + `)(?:\s*(?:,|and|or)\s*(?:` + grammar.CodePat + `))*),?(?:\s+and\s+)?)?(those\s+listed\s+in\s+` + listKindPat + `\s+` + listIDPat + `)?|(not\s+listed\s+in\s+` + listKindPat + `\s+` + listIDPat + `)))?`)
	// listWeightRe matches a member counting part of its weight:
	// "HIS219Y1(.50)".
	listWeightRe = regexp.MustCompile(`(` + grammar.CodePat + `)\s*\(\s*0?\.50?\s*\)`)
	// listRestRe matches the separators that may be left of a list's
	// members once its codes and filters are read.
	listRestRe = regexp.MustCompile(`(?i)^(?:[\s,;:/()\[\].*+&]|\band\b|\bor\b)*$`)
)

// listTitleOnlyRe matches a definition line that only names a part of
// the list: "Theoretical and Methods 1: Theoretical".
var listTitleOnlyRe = regexp.MustCompile(`^(?:[A-Z][\w'’-]*|and|of|the|in|&|\d{1,2}|[:,–-])(?:\s+(?:[A-Z][\w'’-]*|and|of|the|in|&|\d{1,2}:?|[:,–-]))*$`)

// listPermRe matches a list's last member that a person decides:
// "another suitable course with permission from the Associate Chair".
var listPermRe = regexp.MustCompile(`(?i)(?:[,;/]\s*|\s+)(?:or\s+)?((?:an(?:other)?|any)\s+(?:other\s+)?[\w\s-]*?\bcourses?\s+(?:with\s+(?:the\s+)?(?:prior\s+)?(?:permission|approval)|approved\s+by)\b[^;]*)$`)

// listPermCodesRe matches a list's last members counting only with a
// person's permission, by code: ", and with permission of the director:
// TRN300H1".
var listPermCodesRe = regexp.MustCompile(`(?i)(?:[,;]\s*|\s+)(?:and\s+)?(with\s+(?:the\s+)?(?:prior\s+)?(?:written\s+)?(?:permission|approval)\s+of\s+the\s+[\w\s,]+?:\s*(.+))$`)

// listPermRestRe matches what the codes after listPermCodesRe may
// leave: separators and an amount ("1.0 credit from"), which the text keeps.
var listPermRestRe = regexp.MustCompile(`(?i)^(?:[\s,;/]|\band\b|\bor\b|` + grammar.NumPat + `\s+credits?\s+from\b)*$`)

// permCodes splits from list members s a tail of courses that count only
// with a person's permission, as the list's maybe item: and(credits{courses},
// text{permission}).
func permCodes(s string) (rest string, maybe *types.ReqNode) {
	m := listPermCodesRe.FindStringSubmatchIndex(s)
	if m == nil {
		return s, nil
	}
	tail := s[m[4]:m[5]]
	codes := anyCodeRe.FindAllString(tail, -1)
	if codes == nil || !listPermRestRe.MatchString(anyCodeRe.ReplaceAllString(tail, " ")) {
		return s, nil
	}
	return s[:m[0]], programs.AndNode(&types.ReqNode{Type: types.NodeCredits, Courses: grammar.Dedupe(codes)}, TextNode(types.TextPermission, s[m[2]:m[3]]))
}

// listMembersPerm reads a list's members into items. except holds the keys of
// lists a filter excludes, and perm a member a person decides. ok is false
// when something else is left: the list stays a note.
func listMembersPerm(s string) (items []*types.ReqNode, except []string, perm string, ok bool) {
	s = strings.TrimSpace(programs.StripEmphasis(s))
	s = strings.TrimSuffix(s, ".")
	s = grammar.RepairCodes(grammar.SpacedCode.ReplaceAllString(grammar.CharReplacer.Replace(s), "$1$2"))
	s, maybe := permCodes(s)
	s = notAlreadyRe.ReplaceAllString(s, "") // the claims rule
	if m := listPermRe.FindStringSubmatchIndex(s); m != nil && !anyCodeRe.MatchString(s[m[2]:m[3]]) {
		perm = grammar.CleanNote(s[m[2]:m[3]])
		s = s[:m[0]]
	}
	if m := subListNameRe.FindStringSubmatch(s); m != nil && !anyCodeRe.MatchString(m[1]) && listTitleOnlyRe.MatchString(m[1]) {
		// "Latin American Societies: ANT320H1, ...": the part's name
		s = s[len(m[0]):]
	}
	var codes []string
	var weighted []string
	s = listWeightRe.ReplaceAllStringFunc(s, func(m string) string {
		weighted = append(weighted, grammar.CodeRe.FindString(m))
		return " "
	})
	s, pairs := listPairs(s)
	for _, m := range listFilterRe.FindAllStringSubmatch(s, -1) {
		f := &types.ReqNode{Type: types.NodeCredits, Designators: grammar.DesigRe.FindAllString(m[2], -1)}
		for _, lv := range []string{m[1], m[3], m[4]} {
			if lv != "" {
				f.Levels = grammar.ParseLevels(lv + "-level")
			}
		}
		f.Exclude = grammar.CodeRe.FindAllString(m[5], -1)
		for _, ref := range []string{m[6], m[7]} {
			if ref == "" {
				continue
			}
			names := listRefNames(ref[strings.Index(strings.ToLower(ref), " in ")+4:])
			if len(names) != 1 {
				return nil, nil, "", false
			}
			except = append(except, names[0])
		}
		if f.Designators == nil {
			return nil, nil, "", false
		}
		items = append(items, f)
	}
	s = listFilterRe.ReplaceAllString(s, " ")
	// "CJH332H1; CSB; EEB": a designator between semicolons is its courses
	if strings.Contains(s, ";") {
		var desigs []string
		parts := strings.Split(s, ";")
		for i, p := range parts {
			if d := strings.TrimSpace(p); grammar.DesignatorWordRe.MatchString(d) {
				desigs = append(desigs, d)
				parts[i] = " "
			}
		}
		if desigs != nil {
			items = append(items, &types.ReqNode{Type: types.NodeCredits, Designators: desigs})
			s = strings.Join(parts, ";")
		}
	}
	// a bracketed title says nothing about the courses; a bracket with a code
	// ("(formerly SMC218Y1)") is the old code, which counts too
	s = titleParenRe.ReplaceAllStringFunc(s, func(p string) string {
		if anyCodeRe.MatchString(p) {
			return p
		}
		return " "
	})
	codes = append(codes, anyCodeRe.FindAllString(s, -1)...)
	if rest := anyCodeRe.ReplaceAllString(formerlyRe.ReplaceAllString(s, " "), " "); !listRestRe.MatchString(rest) {
		return nil, nil, "", false
	}
	if codes = grammar.Dedupe(codes); codes != nil {
		items = append([]*types.ReqNode{{Type: types.NodeCredits, Courses: codes}}, items...)
	}
	items = append(items, pairs...)
	for _, c := range grammar.Dedupe(weighted) {
		items = append(items, &types.ReqNode{Type: types.NodeCredits, Courses: []string{c}, Weight: 0.5})
	}
	if maybe != nil {
		items = append(items, maybe)
	}
	return items, except, perm, true
}

// listPairRe matches two courses that count only together among a
// list's members: "( MAT235H1, MAT236H1)".
var listPairRe = regexp.MustCompile(`\(\s*(` + grammar.CodePat + `)\s*(?:,|\+|\band\b)\s*(` + grammar.CodePat + `)\s*\)|(` + grammar.CodePat + `)\s*\+\s*(` + grammar.CodePat + `)`)

// listPairs takes the pairs out of list members s as and(course, course):
// flattened, either half would count alone. A "+" chain of three or more is
// left.
func listPairs(s string) (string, []*types.ReqNode) {
	var pairs []*types.ReqNode
	var b strings.Builder
	last := 0
	for _, m := range listPairRe.FindAllStringSubmatchIndex(s, -1) {
		if strings.HasPrefix(strings.TrimSpace(s[m[1]:]), "+") || strings.HasSuffix(strings.TrimSpace(s[:m[0]]), "+") {
			continue
		}
		g := m[2:6]
		if g[0] < 0 {
			g = m[6:10]
		}
		x, y := s[g[0]:g[1]], s[g[2]:g[3]]
		pairs = append(pairs, programs.AndNode(&types.ReqNode{Type: types.NodeCourse, Code: x}, &types.ReqNode{Type: types.NodeCourse, Code: y}))
		b.WriteString(s[last:m[0]])
		b.WriteString(" ")
		last = m[1]
	}
	b.WriteString(s[last:])
	return b.String(), pairs
}

// formerlyRe matches the word in "(formerly SMC218Y1)".
var formerlyRe = regexp.MustCompile(`(?i)\b(?:formerly|previously)\b`)

// mergeListItems joins a list's plain course items into the first one,
// keeping the other items in order.
func mergeListItems(items []*types.ReqNode) []*types.ReqNode {
	var out []*types.ReqNode
	var plain *types.ReqNode
	for _, it := range items {
		if it.Type == types.NodeCredits && it.Courses != nil && it.Designators == nil && it.Levels == nil && it.Lists == nil && it.Weight == 0 && it.Exclude == nil {
			if plain == nil {
				plain = &types.ReqNode{Type: types.NodeCredits}
				out = append(out, plain)
			}
			plain.Courses = grammar.Dedupe(append(plain.Courses, it.Courses...))
			continue
		}
		if slices.ContainsFunc(out, func(o *types.ReqNode) bool { return sameNode(o, it) }) {
			continue
		}
		out = append(out, it)
	}
	return out
}

// listRefPat matches a reference to lists by name: "Groups A and/or B".
var listRefPat = `\b(?:(?:the\s+)?core\s+group|` + listKindPat + `\s+` + listIDPat + `)(?:\s*(?:,\s*(?:and/or|and|or)?|/|\s+and/or|\s+and|\s+or|\s*-|\s*–|\s+through|\s+to)\s*(?:(?:the\s+)?core\s+group|(?:` + listKindPat + `\s+)?` + listIDPat + `))*`

var listRefRe = regexp.MustCompile(`(?i)` + listRefPat)

// listRefTokRe matches one word of a reference: "the Core Group", a
// kind, a range word, a separator or an ID.
var listRefTokRe = regexp.MustCompile(`(?i)^\s*(?:((?:the\s+)?core\s+group\b)|(` + listKindPat + `)\s+|(-|–|through\b|to\b)|(,|/|and/or\b|and\b|or\b)|(` + listIDPat + `))`)

// listRefNames returns the keys of the lists a reference names, in order;
// nil when it isn't one.
func listRefNames(ref string) []string {
	ref = strings.TrimSpace(ref)
	if m := listRefRe.FindStringIndex(ref); m == nil || m[0] != 0 || m[1] != len(ref) {
		return nil
	}
	var keys []string
	kind, from := "", ""
	for rest := ref; strings.TrimSpace(rest) != ""; {
		m := listRefTokRe.FindStringSubmatch(rest)
		if m == nil {
			return nil
		}
		rest = rest[len(m[0]):]
		switch {
		case m[1] != "":
			keys = append(keys, "core group")
		case m[2] != "":
			kind = m[2]
		case m[3] != "":
			if len(keys) == 0 || from != "" {
				return nil
			}
			from = keys[len(keys)-1]
		case m[4] != "":
		case kind == "":
			return nil
		default:
			key := listKey(kind, m[5])
			if from != "" {
				// "Groups B through G": the letters or numbers between
				lo, hi := strings.Fields(from), strings.Fields(key)
				if lo[0] != hi[0] || len(lo[1]) != 1 || len(hi[1]) != 1 || hi[1][0] <= lo[1][0] || hi[1][0]-lo[1][0] > 12 {
					return nil
				}
				for c := lo[1][0] + 1; c < hi[1][0]; c++ {
					keys = append(keys, lo[0]+" "+string(c))
				}
				from = ""
			}
			keys = append(keys, key)
		}
	}
	if from != "" {
		return nil
	}
	return grammar.Dedupe(keys)
}

// compListRefs is the program's lists by key, for reading references.
type compListRefs map[string]*compList

// resolve returns the names of the lists keys name, or nil when one isn't a
// list that can be used.
func (ls compListRefs) resolve(keys []string) []string {
	var names []string
	for _, k := range keys {
		l := ls[k]
		if l == nil || l.bad {
			return nil
		}
		names = append(names, l.list.Name)
	}
	return names
}

// namedLists reads the rows of text that draw on lists by name and builds
// the lists they use. It returns the lists to store.
func (a *Assembler) namedLists() []*types.CourseList {
	a.inlineListNotes()
	built := areaLists(a.buildLists())
	ls := compListRefs{}
	for _, l := range built {
		ls[l.key] = l
	}
	a.belowRows = 0
	for _, l := range a.lines {
		if loc := belowListsRe.FindStringIndex(l); loc != nil && !grammar.CodeRe.MatchString(l[loc[1]:]) {
			a.belowRows++ // not "1.0 credit from the following: MST406H1/ ..."
		}
	}
	// a sub-list that can't be used makes the list over it unusable too
	for _, l := range built {
		if l.bad && l.parent != "" && ls[l.parent] != nil {
			ls[l.parent].bad = true
		}
	}
	a.unions, a.reqRefs = nil, nil
	var walk func(rows []*compRow) []*compRow
	walk = func(rows []*compRow) []*compRow {
		var out []*compRow
		for _, r := range rows {
			r.children = walk(r.children)
			out = append(out, a.listRefRow(r, ls)...)
		}
		return out
	}
	a.programRows = nil
	a.items = walk(a.items)
	a.program = append(walk(a.program), a.programRows...)
	a.spreadFloors(ls)
	a.weights(ls)
	a.poolGrammar(ls)
	a.textPoolsOverLists(ls)
	a.groupPools(ls) // groups of alternatives and one-off pools
	a.plusParts(ls)
	a.titledFloors(ls)
	a.listSubCaps(ls)
	// the lists used, and the lists they use
	built = append(built, a.unions...)
	var mark func(name string)
	mark = func(name string) {
		for _, l := range built {
			if l.list.Name == name && !l.used {
				l.used = true
				for _, it := range l.list.Items {
					for _, n := range it.Lists {
						mark(n)
					}
				}
			}
		}
	}
	var rows []*compRow
	var collect func(rs []*compRow)
	collect = func(rs []*compRow) {
		for _, r := range rs {
			rows = append(rows, r)
			collect(r.children)
		}
	}
	collect(a.items)
	collect(a.program)
	collect(a.floors)
	if a.total != nil {
		rows = append(rows, a.total) // its own pool may draw on lists
	}
	for _, r := range rows {
		grammar.WalkReq(r.r.Req, func(n *types.ReqNode) {
			for _, name := range n.Lists {
				mark(name)
			}
		})
	}
	var out []*types.CourseList
	drop := map[string]bool{}
	built = slices.Clone(built)
	order := func(l *compList) int {
		if l.line < 0 {
			return math.MaxInt32 // a sentence's list: after the lines', in the order read
		}
		return l.line
	}
	slices.SortStableFunc(built, func(x, y *compList) int { return order(x) - order(y) })
	for _, l := range built {
		if !l.used {
			continue
		}
		for _, k := range l.except {
			// "any 300-/ 400-level CSC course except those listed in Group B"
			for _, it := range l.list.Items {
				if it.Designators != nil && ls[k] != nil {
					for _, x := range ls[k].list.Items {
						it.Exclude = grammar.Dedupe(append(it.Exclude, x.Courses...))
					}
				}
			}
		}
		out = append(out, l.list)
		for _, n := range l.notes {
			drop[n] = true
		}
	}
	// "ANT390H1 may be placed in either Group A or B ...": a maybe member
	var used []*compList
	for _, l := range built {
		if l.used {
			used = append(used, l)
		}
	}
	topical := func(t string) bool {
		if !topicMemberRe.MatchString(t) || !topicMember(used, t) {
			return false
		}
		drop[t] = true
		return true
	}
	// "If ARH312Y1 is completed, it will be considered a Group A course": a
	// member the list's lines don't name
	member := func(t string) bool {
		m := listMemberRe.FindStringSubmatch(Clean(t))
		if m == nil {
			return false
		}
		names := ls.resolve(listRefNames(m[2]))
		if len(names) != 1 {
			return false
		}
		for _, l := range used {
			if l.list.Name != names[0] {
				continue
			}
			l.list.Items = mergeListItems(append(slices.Clone(l.list.Items), &types.ReqNode{Type: types.NodeCredits, Courses: []string{m[1]}}))
			drop[t] = true
			return true
		}
		return false
	}
	for _, n := range a.notes {
		_ = topical(n) || member(n)
	}
	for _, n := range a.starredTopics(used) {
		drop[n] = true
	}
	// "It is however recommended to include at least ESS262H1/ ...": courses
	// the pool doesn't name may be meant to count, so a maybe member (Review)
	for _, r := range rows {
		a.recommendedMembers(r, ls)
	}
	// a membership sentence with no target may wait as a program route
	a.programRoutes = slices.DeleteFunc(a.programRoutes, member)
	for _, r := range rows {
		for _, n := range r.r.Notes {
			_ = topical(n) || member(n)
		}
	}
	var prune func(rs []*compRow) []*compRow
	prune = func(rs []*compRow) []*compRow {
		return slices.DeleteFunc(rs, func(r *compRow) bool {
			r.children = prune(r.children)
			// "ANT390H1 may not be available in Course Group B ...": now on the list
			return r.r.Kind == types.RequirementConstraint && r.r.Req != nil && r.r.Req.Type == types.NodeText && len(r.children) == 0 && topical(Clean(r.r.Req.Text))
		})
	}
	a.items, a.program = prune(a.items), prune(a.program)
	a.notes = slices.DeleteFunc(a.notes, func(n string) bool { return drop[n] })
	for _, r := range rows {
		r.r.Notes = grammar.Dedupe(slices.DeleteFunc(r.r.Notes, func(n string) bool { return drop[n] }))
		if len(r.r.Notes) == 0 {
			r.r.Notes = nil
		}
	}
	if len(a.notes) == 0 {
		a.notes = nil
	}
	return out
}

// lists reports whether code is listed by code on one of the named lists or
// the lists under them.
func (ls compListRefs) lists(names []string, code string) bool {
	for _, l := range ls {
		if !slices.Contains(names, l.list.Name) {
			continue
		}
		for _, it := range l.list.Items {
			if slices.Contains(it.Courses, code) || it.Lists != nil && ls.lists(it.Lists, code) {
				return true
			}
		}
	}
	return false
}

var (
	// belowListsRe matches a pointer to the lists set out below an item, by
	// none of their names: "from the following elective courses".
	belowListsRe = regexp.MustCompile(`(?i)\s*(?:\bfrom\s+(?:any\s+of\s+)?the\s+(?:courses\s+)?(?:list(?:ed)?\s+)?below\b|\bfrom\s+any\s+of\s+the\s+courses\s+(?:listed\s+)?below\b|\bfrom\s+the\s+following(?:\s+(?:elective|eligible|approved))?(?:\s+(?:courses|list))?\b|\bfrom\s+(?:approved\s+)?(?-i:[A-Z])[\w\s,&-]*?\s+courses\s+as\s+listed\s+below\b|\(\s*list\s+below\s*\))`)
	// listBelowParenRe matches "(list below)" after a description of the
	// list.
	listBelowParenRe = regexp.MustCompile(`(?i)\(\s*list\s+below\s*\)`)
	// amountOnlyRe matches an amount and a description of its list: "0.5
	// credit with a significant emphasis on ethics".
	amountOnlyRe = regexp.MustCompile(`(?i)^(` + grammar.NumPat + `\s+credits?)\s+with\s+[^()]*$`)
)

// sortedLists returns the lists that have definitions, in the order of their
// first definition's line.
func (a *Assembler) sortedLists(ls compListRefs) []*compList {
	var out []*compList
	for _, l := range ls {
		if len(l.defs) > 0 {
			out = append(out, l)
		}
	}
	slices.SortFunc(out, func(x, y *compList) int {
		if d := defLine(x.defs[0]) - defLine(y.defs[0]); d != 0 {
			return d
		}
		return strings.Compare(x.key, y.key) // ls is a map: no order of its own
	})
	return out
}

// defLine is the line a definition is on, -1 for a sentence.
func defLine(d *compListDef) int {
	if d.block == nil {
		return -1
	}
	return d.block.line
}

func (ls compListRefs) within(name string, names []string) bool {
	for _, l := range ls {
		if l.list.Name != name || l.parent == "" || ls[l.parent] == nil {
			continue
		}
		return slices.Contains(names, ls[l.parent].list.Name)
	}
	return false
}

// ItemLabelRe matches an item's number before its text: "5. ".
var ItemLabelRe = regexp.MustCompile(`^\s*(?:\d{1,2}[a-z]?|[a-z])[.)]\s+`)

// listMemberRe matches a sentence that puts a course on a list: "If
// ARH312Y1 is completed, it will be considered a Group A course".
var listMemberRe = regexp.MustCompile(`(?i)^if\s+(` + grammar.CodePat + `)\s+is\s+(?:completed|taken),\s+it\s+will\s+be\s+considered\s+(?:an?\s+)?(` + listRefPat + `)\s+course\.?$`)

// topicMemberRe matches a course whose list depends on its topic:
// "ANT390H1 may be placed in either Group A or B or C depending on the topic".
var topicMemberRe = regexp.MustCompile(`(?i)^(` + grammar.CodePat + `)\s+may\s+(?:be\s+placed\s+in|not\s+be\s+available\s+in)\b[^.]*\bdepending\s+on\s+the\s+topic\b`)

// topicMember makes the course of a topicMemberRe sentence a maybe
// member, and(course, permission text), of each used list that lists it. It
// reports whether a list took it.
func topicMember(used []*compList, sentence string) bool {
	code := topicMemberRe.FindStringSubmatch(sentence)[1]
	took := false
	for _, l := range used {
		for i, it := range l.list.Items {
			if it.Type != types.NodeCredits || it.Designators != nil || it.Levels != nil || it.Weight != 0 || !slices.Contains(it.Courses, code) {
				continue
			}
			it.Courses = slices.DeleteFunc(slices.Clone(it.Courses), func(c string) bool { return c == code })
			maybe := programs.AndNode(&types.ReqNode{Type: types.NodeCredits, Courses: []string{code}}, TextNode(types.TextPermission, sentence))
			if len(it.Courses) == 0 {
				l.list.Items[i] = maybe
			} else {
				l.list.Items = append(l.list.Items, maybe)
			}
			took = true
			break
		}
	}
	return took
}

// memberLines returns, as list definitions, lines under a heading naming a
// list that the reading kept as some other note: a line of codes, or one
// saying what the list holds. Its note is whichever form the reading kept.
func (a *Assembler) memberLines() []*compListDef {
	seen := map[*Block]bool{}
	for _, d := range a.listDefs {
		seen[d.block] = true
	}
	notes := map[string]bool{}
	for _, n := range a.notes {
		notes[n] = true
	}
	var rows func(rs []*compRow)
	rows = func(rs []*compRow) {
		for _, r := range rs {
			for _, n := range r.r.Notes {
				notes[n] = true
			}
			rows(r.children)
		}
	}
	rows(a.items)
	var out []*compListDef
	var walk func(bs []*Block)
	walk = func(bs []*Block) {
		for _, b := range bs {
			walk(b.Children)
			h := b.Heading
			if seen[b] || b.Kind == BlockHeading || h == nil || h.Kind != BlockHeading || len(b.Children) > 0 {
				continue
			}
			if _, key := listName(h.Section); key == "" || !memberLineRe.MatchString(strings.TrimSpace(b.Text)) && !mostlyCodes(b.Text) {
				continue
			}
			for _, note := range []string{grammar.CleanNote(b.Text), grammar.CleanNote(BlockLine(b)), h.Section + ": " + grammar.CleanNote(b.Text)} {
				if notes[note] {
					out = append(out, &compListDef{title: h.Section, members: b.Text, note: note, block: b})
					break
				}
			}
		}
	}
	walk(a.tree)
	return out
}

// addUnion records items as a list of row's own and returns its name:
// "Requirement <label> courses", then "(2)", "(3)" for more of one row. A row
// with no label gets none (ok false).
func (a *Assembler) addUnion(row *compRow, items []*types.ReqNode, ls compListRefs) (name string, l *compList, ok bool) {
	if row.r.Label == "" {
		return "", nil, false
	}
	base := "Requirement " + row.r.Label
	if !strings.HasSuffix(strings.ToLower(base), " courses") {
		base += " courses"
	}
	name = base
	for k := 2; ls[unionKey(name)] != nil; k++ {
		name = base + " (" + strconv.Itoa(k) + ")"
	}
	l = &compList{key: unionKey(name), list: &types.CourseList{Name: name, Items: items}, line: math.MaxInt32 - 1}
	ls[l.key] = l
	a.unions = append(a.unions, l)
	return name, l, true
}

// unionKey is the key a list addUnion made is stored under.
func unionKey(name string) string { return "union:" + strings.ToLower(name) }

// includeRecommendRe matches advice to include courses: "recommended to
// include".
var includeRecommendRe = regexp.MustCompile(`(?i)\brecommended\s+to\s+include\b`)

// recommendedMembers adds to the list a row's pool became the courses the
// row's advice recommends that the list doesn't hold, as an ambiguity maybe
// item.
func (a *Assembler) recommendedMembers(r *compRow, ls compListRefs) {
	n := r.r.Req
	if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeCredits || len(n.Lists) != 1 {
		return
	}
	l := ls[unionKey(n.Lists[0])]
	if l == nil {
		return
	}
	for _, note := range r.r.Notes {
		if !includeRecommendRe.MatchString(note) {
			continue
		}
		var missing []string
		for _, code := range grammar.CodeRe.FindAllString(note, -1) {
			if !slices.ContainsFunc(l.list.Items, func(it *types.ReqNode) bool { return filterHolds(it, code) }) {
				missing = append(missing, code)
			}
		}
		if missing = grammar.Dedupe(missing); missing != nil {
			l.list.Items = append(l.list.Items, programs.AndNode(&types.ReqNode{Type: types.NodeCredits, Courses: missing},
				TextNode(types.TextAmbiguity, "the item recommends "+strings.Join(missing, " and ")+", which its pool doesn't name")))
		}
	}
}

// filterHolds reports whether list item it matches code by its courses or
// by its designators and levels (not its lists).
func filterHolds(it *types.ReqNode, code string) bool {
	if it.Type != types.NodeCredits || slices.Contains(it.Exclude, code) || slices.Contains(it.ExcludeDesignators, code[:3]) ||
		it.MaxCourseAmount != 0 && grammar.CourseWeight(code) > it.MaxCourseAmount {
		return false
	}
	if slices.Contains(it.Courses, code) {
		return true
	}
	return it.Designators != nil && slices.Contains(it.Designators, code[:3]) && grammar.LevelMatches(it.Levels, code)
}

// inlineListRe matches the total's pool naming a list below whose codes
// it reads inline: "or Equivalent courses (listed below)".
var inlineListRe = regexp.MustCompile(`(?i)\bor\s+([A-Z][\w\s]*?)\s+courses\s+\(listed\s+below\)`)

// inlineListNotes drops a inlineListRe list's definition from the notes
// once the total's pool holds all its codes: it is encoded.
func (a *Assembler) inlineListNotes() {
	if a.total == nil || a.total.r.Req == nil {
		return
	}
	m := inlineListRe.FindStringSubmatch(a.total.r.Raw)
	if m == nil {
		return
	}
	pool := a.total.r.Req
	if pool.Type == types.NodeOr && len(pool.Items) > 0 {
		pool = pool.Items[0]
	}
	want := strings.ToLower(strings.TrimSpace(m[1])) + " courses"
	for k := range a.lines {
		if strings.ToLower(strings.Trim(a.widenLine(k), " :*_")) != want {
			continue
		}
		var used []string
		for k++; k < len(a.lines) && (IsCodeList(a.widenLine(k)) || mostlyCodes(a.widenLine(k))); k++ {
			if slices.ContainsFunc(grammar.CodeRe.FindAllString(a.lines[k], -1), func(c string) bool { return !slices.Contains(pool.Courses, c) }) {
				return
			}
			used = append(used, a.widenLine(k))
		}
		a.dropEncoded(used)
		return
	}
}

// listSubCapRe matches a cap on some of a list's members at the end of
// its definition: ", 1.0 credit from TRN160Y1, TRN161Y1".
var listSubCapRe = regexp.MustCompile(`(?i),\s*` + grammar.AmountPat + `\s+credits?\s+from\s+((?:` + grammar.CodePat + `)(?:\s*,\s*(?:` + grammar.CodePat + `))+)\s*\.?\s*$`)

// listSubCaps makes a listSubCapRe cap a limit under each allocation
// drawing on the list by name, since the list's items can't hold it.
func (a *Assembler) listSubCaps(ls compListRefs) {
	for _, l := range a.sortedLists(ls) {
		if l.bad || len(l.defs) != 1 {
			continue
		}
		m := listSubCapRe.FindStringSubmatch(l.defs[0].members)
		if m == nil {
			continue
		}
		v, _ := strconv.ParseFloat(m[1], 64)
		codes := grammar.Dedupe(grammar.CodeRe.FindAllString(m[2], -1))
		if got, _, ok := ls.members(l.list.Name); !ok || slices.ContainsFunc(codes, func(c string) bool { return !slices.Contains(got, c) }) {
			continue
		}
		cap := &types.ReqNode{Type: types.NodeCredits, Courses: codes, MaxAmount: grammar.MaxAmount(v)}
		for _, r := range a.items {
			uses := false
			grammar.WalkReq(r.r.Req, func(x *types.ReqNode) {
				uses = uses || x.Type == types.NodeCredits && slices.Contains(x.Lists, l.list.Name)
			})
			if r.r.Kind == types.RequirementAllocation && uses {
				a.rowUnder(r, &types.Requirement{Kind: types.RequirementLimit, Req: grammar.CloneNode(cap), Raw: grammar.CleanNote(l.defs[0].members)})
			}
		}
	}
}

// listsByName indexes lists by name; a later list of the same name wins.
func listsByName(lists []*types.CourseList) map[string]*types.CourseList {
	m := map[string]*types.CourseList{}
	for _, l := range lists {
		m[l.Name] = l
	}
	return m
}

// byName returns the list named name, or nil.
func (ls compListRefs) byName(name string) *compList {
	for _, l := range ls {
		if l.list.Name == name {
			return l
		}
	}
	return nil
}

// members returns the codes on the named list and the lists under it, and its
// filter members; ok is false when a list is missing or bad, or the lists loop.
func (ls compListRefs) members(name string) (codes []string, filters []*types.ReqNode, ok bool) {
	return ls.membersOf(name, map[string]bool{})
}

// membersOf is members, with the lists already on the path in seen.
func (ls compListRefs) membersOf(name string, seen map[string]bool) (codes []string, filters []*types.ReqNode, ok bool) {
	l := ls.byName(name)
	if l == nil || l.bad || seen[name] {
		return nil, nil, false
	}
	seen[name] = true
	defer delete(seen, name)
	good := true
	for _, it := range l.list.Items {
		grammar.WalkReq(it, func(n *types.ReqNode) {
			if n.Type != types.NodeCredits {
				return
			}
			codes = append(codes, n.Courses...)
			if n.Designators != nil {
				filters = append(filters, n)
			}
			for _, sub := range n.Lists {
				c, f, sok := ls.membersOf(sub, seen)
				good = good && sok
				codes, filters = append(codes, c...), append(filters, f...)
			}
		})
	}
	return grammar.Dedupe(codes), filters, good
}

// sharedCourses returns, for each part (credits from one list, or listed
// courses), the courses another part holds too; ok is false when two parts'
// filters share a designator, so the shared courses can't be listed.
func (ls compListRefs) sharedCourses(parts []*types.ReqNode) (shared [][]string, ok bool) {
	codes := make([][]string, len(parts))
	filters := make([][]*types.ReqNode, len(parts))
	for i, p := range parts {
		if p.Lists == nil {
			codes[i] = p.Courses
			continue
		}
		c, f, ok := ls.members(p.Lists[0])
		if !ok {
			return nil, false
		}
		codes[i], filters[i] = c, f
	}
	holds := func(i int, code string) bool {
		return slices.Contains(codes[i], code) || slices.ContainsFunc(filters[i], func(f *types.ReqNode) bool { return filterHolds(f, code) })
	}
	shared = make([][]string, len(parts))
	for i := range parts {
		for j := range parts {
			if i == j {
				continue
			}
			for _, f := range filters[i] {
				for _, g := range filters[j] {
					if slices.ContainsFunc(f.Designators, func(d string) bool { return slices.Contains(g.Designators, d) }) {
						return nil, false // "any URB course" on two lists: not a list of codes
					}
				}
			}
			for _, c := range codes[j] {
				if holds(i, c) && !slices.Contains(shared[i], c) {
					shared[i] = append(shared[i], c)
				}
			}
		}
	}
	return shared, true
}

// nameRowUnions names each rowUnion list by its row's number, "Requirement 4
// courses", renames its uses under roots and in lists, and adds it to lists.
func (a *Assembler) nameRowUnions(roots []*compRow, lists []*types.CourseList) []*types.CourseList {
	if a.rowUnions == nil {
		return lists
	}
	names := map[string]string{}
	for _, u := range a.rowUnions {
		base := fmt.Sprintf("Requirement %d courses", u.row.r.Number)
		name := base
		for k := 2; slices.ContainsFunc(lists, func(l *types.CourseList) bool { return l.Name == name }); k++ {
			name = base + " (" + strconv.Itoa(k) + ")"
		}
		names[u.list.Name] = name
		u.list.Name = name
		lists = append(lists, u.list)
	}
	rename := func(n *types.ReqNode) {
		grammar.WalkReq(n, func(n *types.ReqNode) {
			for i, x := range n.Lists {
				if y, ok := names[x]; ok {
					n.Lists[i] = y
				}
			}
		})
	}
	walkRows(roots, func(r *compRow) {
		rename(r.r.Req)
	})
	for _, l := range lists {
		for _, it := range l.Items {
			rename(it)
		}
	}
	return lists
}
