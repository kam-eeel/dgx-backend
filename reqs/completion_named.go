package reqs

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Named lists: the lines that set out lists become lists once every row is
// read, and rows of text that draw on them by name are read again.

// compListDef is one line or block that sets out a list.
type compListDef struct {
	title   string     // the heading or label as written: "Group A (Ethics)"
	members string     // the members as written: "CSC209H1; any 300-/ 400-level CSC course"
	note    string     // the note it was kept as
	block   *compBlock // the line, for the headings over it; nil for a sentence
	row     *compRow   // the row whose note it is; nil for the program's or a sentence's
}

// listDef keeps a line that sets out a list as a note on row (the program's
// when nil) and records it as a list definition.
func (a *compAssembler) listDef(row *compRow, title, members string, b *compBlock, note string) {
	a.notesTo(row, []string{note})
	a.listDefs = append(a.listDefs, &compListDef{title: title, members: members, note: note, block: b, row: row})
}

// compList is a list while the program is read.
type compList struct {
	key    string // complListKey of its name
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
	// complListIDPat matches a list's letter, number or numeral, with a
	// sub-list's numeral after it: "C (ii)".
	complListIDPat = `(?-i:[A-Z]|\d{1,2}|[IVX]{1,4})\b(?:\s*\(\s*(?-i:i{1,3}|iv|v|vi)\s*\))?`
	// complListKindPat matches the kind of a list's name: "Sub-group".
	complListKindPat = `(?:groups?|lists?|clusters?|sub-?groups?|categor(?:y|ies))`
	// complListNameRe matches a list's name at the start of its title: "Cluster
	// B" in "Cluster B - Learning".
	complListNameRe = regexp.MustCompile(`(?i)^(?:the\s+)?(?:course\s+|disciplinary/thematic\s+)?(` + complListKindPat + `)\s+(` + complListIDPat + `)(?:\s*$|\s*[-–:(]|\s+\()`)
	// complCoreGroupRe matches "the Core Group", a list named by a word.
	complCoreGroupRe = regexp.MustCompile(`(?i)^(?:the\s+)?(core)\s+group\b`)
)

// complListKey is the form a list's name is matched in: "subgroup c(ii)".
func complListKey(kind, id string) string {
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
	t := strings.TrimSpace(stripEmphasis(title))
	if m := complListNameRe.FindStringSubmatch(t); m != nil {
		key := complListKey(m[1], m[2])
		// the kind as the key normalises it, capitalised
		kind := strings.Fields(key)[0]
		kind = strings.ToUpper(kind[:1]) + kind[1:]
		id := strings.Join(strings.Fields(strings.NewReplacer("(", " (", ")", ")").Replace(m[2])), " ")
		return kind + " " + id, key
	}
	if m := complCoreGroupRe.FindStringSubmatch(t); m != nil {
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

// complTitleKey is the key of a list named by its title: lower case, without
// "courses".
func complTitleKey(title string) string {
	t := strings.ToLower(strings.Join(strings.Fields(title), " "))
	return "title:" + strings.TrimSpace(strings.TrimSuffix(t, " courses"))
}

// buildLists reads the list definitions into lists, in the order they are
// first defined.
func (a *compAssembler) buildLists() []*compList {
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
		name := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(stripEmphasis(d.title)), ":"))
		if name == "" || d.block == nil || blkNoteLabelRe.MatchString(name) {
			return "", ""
		}
		return name, complTitleKey(name)
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
		l.at(complDefLine(d))
		l.notes = append(l.notes, d.note)
		if t := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(stripEmphasis(d.title)), ":")); t != name {
			titles[key] = append(titles[key], t)
		}
		if pk != "" {
			p := get(pn, pk)
			p.at(h.line)
			if l.parent == "" {
				l.parent = pk
				p.list.Items = append(p.list.Items, &types.ReqNode{Type: types.NodeCredits, Lists: []string{name}})
				if t := strings.TrimSpace(h.section); t != pn {
					titles[pk] = append(titles[pk], t)
				}
			}
		}
		items, except, perm, ok := listMembersPerm(d.members)
		if perm != "" {
			l.perm = perm
		}
		switch {
		case !ok && complListTitleOnlyRe.MatchString(strings.TrimSpace(d.members)) && !complAnyCodeRe.MatchString(d.members):
			// "**Group A:** Theoretical and Methods 1: Theoretical": a part's name
		case !ok:
			l.bad = true
		case items == nil && codeRe.MatchString(d.members):
			l.bad = true
		default:
			l.list.Items = append(l.list.Items, items...)
			l.except = append(l.except, except...)
		}
	}
	for _, l := range out {
		if ts := dedupe(titles[l.key]); len(ts) == 1 {
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
func listParent(d *compListDef, key string) (name, pkey string, h *compBlock) {
	if d.block == nil {
		return "", "", nil
	}
	for h := d.block.heading; h != nil; h = h.heading {
		if h.kind != blockHeading {
			continue
		}
		if pn, pk := listName(h.section); pk != "" && pk != key {
			if strings.Fields(pk)[0] != strings.Fields(key)[0] {
				return pn, pk, h
			}
			break
		}
	}
	return "", "", nil
}

var (
	// complListLevelsPat matches levels in a list's filter: "300-/ 400".
	complListLevelsPat = `[1-4]00(?:\s*\+|(?:\s*-?\s*/\s*|\s*,\s*|\s+(?:or|and)\s+)[1-4]00)*\+?`
	// complListFilterRe matches a member by filter: "any 300-/ 400-level CSC
	// course except those listed in Group B".
	complListFilterRe = regexp.MustCompile(`(?i)\bany\s+(?:other\s+)?(?:(` + complListLevelsPat + `)\s*-?\s*level\s+)?((?-i:[A-Z]{3})(?:\s*(?:,|/|or|and)\s*(?-i:[A-Z]{3}))*)(?:\s*(` + complListLevelsPat + `)|\s+courses?(?:\s+at\s+the\s+(` + complListLevelsPat + `)\s*-?\s*level)?)(?:\s+(?:(?:except|excluding)\s+(?:((?:` + codePat + `)(?:\s*(?:,|and|or)\s*(?:` + codePat + `))*),?(?:\s+and\s+)?)?(those\s+listed\s+in\s+` + complListKindPat + `\s+` + complListIDPat + `)?|(not\s+listed\s+in\s+` + complListKindPat + `\s+` + complListIDPat + `)))?`)
	// complListWeightRe matches a member counting part of its weight:
	// "HIS219Y1(.50)".
	complListWeightRe = regexp.MustCompile(`(` + codePat + `)\s*\(\s*0?\.50?\s*\)`)
	// complBareDesigRe matches a designator alone, as a list member: "CSB".
	complBareDesigRe = regexp.MustCompile(`^[A-Z]{3}$`)
	// complListRestRe matches the separators that may be left of a list's
	// members once its codes and filters are read.
	complListRestRe = regexp.MustCompile(`(?i)^(?:[\s,;:/()\[\].*+&]|\band\b|\bor\b)*$`)
)

// complListTitleOnlyRe matches a definition line that only names a part of
// the list: "Theoretical and Methods 1: Theoretical".
var complListTitleOnlyRe = regexp.MustCompile(`^(?:[A-Z][\w'’-]*|and|of|the|in|&|\d{1,2}|[:,–-])(?:\s+(?:[A-Z][\w'’-]*|and|of|the|in|&|\d{1,2}:?|[:,–-]))*$`)

// complListPermRe matches a list's last member that a person decides:
// "another suitable course with permission from the Associate Chair".
var complListPermRe = regexp.MustCompile(`(?i)(?:[,;/]\s*|\s+)(?:or\s+)?((?:an(?:other)?|any)\s+(?:other\s+)?[\w\s-]*?\bcourses?\s+(?:with\s+(?:the\s+)?(?:prior\s+)?(?:permission|approval)|approved\s+by)\b[^;]*)$`)

// complListPermCodesRe matches a list's last members counting only with a
// person's permission, by code: ", and with permission of the director:
// TRN300H1".
var complListPermCodesRe = regexp.MustCompile(`(?i)(?:[,;]\s*|\s+)(?:and\s+)?(with\s+(?:the\s+)?(?:prior\s+)?(?:written\s+)?(?:permission|approval)\s+of\s+the\s+[\w\s,]+?:\s*(.+))$`)

// complListPermRestRe matches what the codes after complListPermCodesRe may
// leave: separators and an amount ("1.0 credit from"), which the text keeps.
var complListPermRestRe = regexp.MustCompile(`(?i)^(?:[\s,;/]|\band\b|\bor\b|\d+(?:\.\d+)?\s+credits?\s+from\b)*$`)

// permCodes splits from list members s a tail of courses that count only
// with a person's permission, as the list's maybe item: and(credits{courses},
// text{permission}).
func permCodes(s string) (rest string, maybe *types.ReqNode) {
	m := complListPermCodesRe.FindStringSubmatchIndex(s)
	if m == nil {
		return s, nil
	}
	tail := s[m[4]:m[5]]
	codes := complAnyCodeRe.FindAllString(tail, -1)
	if codes == nil || !complListPermRestRe.MatchString(complAnyCodeRe.ReplaceAllString(tail, " ")) {
		return s, nil
	}
	return s[:m[0]], andNode(&types.ReqNode{Type: types.NodeCredits, Courses: dedupe(codes)}, textNode(types.TextPermission, s[m[2]:m[3]]))
}

// listMembersPerm reads a list's members into items. except holds the keys of
// lists a filter excludes, and perm a member a person decides. ok is false
// when something else is left: the list stays a note.
func listMembersPerm(s string) (items []*types.ReqNode, except []string, perm string, ok bool) {
	s = strings.TrimSpace(stripEmphasis(s))
	s = strings.TrimSuffix(s, ".")
	s = repairCodes(spacedCode.ReplaceAllString(charReplacer.Replace(s), "$1$2"))
	s, maybe := permCodes(s)
	s = complNotAlreadyRe.ReplaceAllString(s, "") // the claims rule
	if m := complListPermRe.FindStringSubmatchIndex(s); m != nil && !complAnyCodeRe.MatchString(s[m[2]:m[3]]) {
		perm = cleanNote(s[m[2]:m[3]])
		s = s[:m[0]]
	}
	if m := complSubListNameRe.FindStringSubmatch(s); m != nil && !complAnyCodeRe.MatchString(m[1]) && complListTitleOnlyRe.MatchString(m[1]) {
		// "Latin American Societies: ANT320H1, ...": the part's name
		s = s[len(m[0]):]
	}
	var codes []string
	var weighted []string
	s = complListWeightRe.ReplaceAllStringFunc(s, func(m string) string {
		weighted = append(weighted, codeRe.FindString(m))
		return " "
	})
	s, pairs := listPairs(s)
	for _, m := range complListFilterRe.FindAllStringSubmatch(s, -1) {
		f := &types.ReqNode{Type: types.NodeCredits, Designators: desigRe.FindAllString(m[2], -1)}
		for _, lv := range []string{m[1], m[3], m[4]} {
			if lv != "" {
				f.Levels = parseLevels(lv + "-level")
			}
		}
		f.Exclude = codeRe.FindAllString(m[5], -1)
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
	s = complListFilterRe.ReplaceAllString(s, " ")
	// "CJH332H1; CSB; EEB": a designator between semicolons is its courses
	if strings.Contains(s, ";") {
		var desigs []string
		parts := strings.Split(s, ";")
		for i, p := range parts {
			if d := strings.TrimSpace(p); complBareDesigRe.MatchString(d) {
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
	s = complTitleParenRe.ReplaceAllStringFunc(s, func(p string) string {
		if complAnyCodeRe.MatchString(p) {
			return p
		}
		return " "
	})
	codes = append(codes, complAnyCodeRe.FindAllString(s, -1)...)
	if rest := complAnyCodeRe.ReplaceAllString(complFormerlyRe.ReplaceAllString(s, " "), " "); !complListRestRe.MatchString(rest) {
		return nil, nil, "", false
	}
	if codes = dedupe(codes); codes != nil {
		items = append([]*types.ReqNode{{Type: types.NodeCredits, Courses: codes}}, items...)
	}
	items = append(items, pairs...)
	for _, c := range dedupe(weighted) {
		items = append(items, &types.ReqNode{Type: types.NodeCredits, Courses: []string{c}, Weight: 0.5})
	}
	if maybe != nil {
		items = append(items, maybe)
	}
	return items, except, perm, true
}

// complListPairRe matches two courses that count only together among a
// list's members: "( MAT235H1, MAT236H1)".
var complListPairRe = regexp.MustCompile(`\(\s*(` + codePat + `)\s*(?:,|\+|\band\b)\s*(` + codePat + `)\s*\)|(` + codePat + `)\s*\+\s*(` + codePat + `)`)

// listPairs takes the pairs out of list members s as and(course, course):
// flattened, either half would count alone. A "+" chain of three or more is
// left.
func listPairs(s string) (string, []*types.ReqNode) {
	var pairs []*types.ReqNode
	var b strings.Builder
	last := 0
	for _, m := range complListPairRe.FindAllStringSubmatchIndex(s, -1) {
		if strings.HasPrefix(strings.TrimSpace(s[m[1]:]), "+") || strings.HasSuffix(strings.TrimSpace(s[:m[0]]), "+") {
			continue
		}
		g := m[2:6]
		if g[0] < 0 {
			g = m[6:10]
		}
		x, y := s[g[0]:g[1]], s[g[2]:g[3]]
		pairs = append(pairs, andNode(&types.ReqNode{Type: types.NodeCourse, Code: x}, &types.ReqNode{Type: types.NodeCourse, Code: y}))
		b.WriteString(s[last:m[0]])
		b.WriteString(" ")
		last = m[1]
	}
	b.WriteString(s[last:])
	return b.String(), pairs
}

// complFormerlyRe matches the word in "(formerly SMC218Y1)".
var complFormerlyRe = regexp.MustCompile(`(?i)\b(?:formerly|previously)\b`)

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
			plain.Courses = dedupe(append(plain.Courses, it.Courses...))
			continue
		}
		if slices.ContainsFunc(out, func(o *types.ReqNode) bool { return sameNode(o, it) }) {
			continue
		}
		out = append(out, it)
	}
	return out
}

// complListRefPat matches a reference to lists by name: "Groups A and/or B".
var complListRefPat = `\b(?:(?:the\s+)?core\s+group|` + complListKindPat + `\s+` + complListIDPat + `)(?:\s*(?:,\s*(?:and/or|and|or)?|/|\s+and/or|\s+and|\s+or|\s*-|\s*–|\s+through|\s+to)\s*(?:(?:the\s+)?core\s+group|(?:` + complListKindPat + `\s+)?` + complListIDPat + `))*`

var complListRefRe = regexp.MustCompile(`(?i)` + complListRefPat)

// complListRefTokRe matches one word of a reference: "the Core Group", a
// kind, a range word, a separator or an ID.
var complListRefTokRe = regexp.MustCompile(`(?i)^\s*(?:((?:the\s+)?core\s+group\b)|(` + complListKindPat + `)\s+|(-|–|through\b|to\b)|(,|/|and/or\b|and\b|or\b)|(` + complListIDPat + `))`)

// listRefNames returns the keys of the lists a reference names, in order;
// nil when it isn't one.
func listRefNames(ref string) []string {
	ref = strings.TrimSpace(ref)
	if m := complListRefRe.FindStringIndex(ref); m == nil || m[0] != 0 || m[1] != len(ref) {
		return nil
	}
	var keys []string
	kind, from := "", ""
	for rest := ref; strings.TrimSpace(rest) != ""; {
		m := complListRefTokRe.FindStringSubmatch(rest)
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
			key := complListKey(kind, m[5])
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
	return dedupe(keys)
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

var (
	// complFromListRefRe matches a reference after "from" or "in", with what
	// may come around it: "from either Group A, B or C".
	complFromListRefRe = regexp.MustCompile(`(?i)\s*\b(?:from|in)\s+(?:among\s+|either\s+|any\s+of\s+)?(?:the\s+)?(?:courses\s+(?:listed\s+)?in\s+(?:the\s+)?)?(` + complListRefPat + `)(?:\s+in\s+(` + complListRefPat + `))?(?:\s+courses)?(?:\s+(?:below|above|listed\s+below))?(?:\s*\(([^()]*)\))?`)
	// complListParenDropRe matches a bracket after a reference that restates
	// it, inside the brackets: "any Cluster".
	complListParenDropRe = regexp.MustCompile(`(?i)^(?:any(?:\s+of)?\s+)?(?:clusters?|sub-?groups?)\b[a-z\s,]*$`)
	// complExcludingCodesRe matches the codes a bracket after a reference leaves
	// out: "excluding PSY304Y1, and PSY406Y1".
	complExcludingCodesRe = regexp.MustCompile(`(?i)(?:^|[,;]?\s+)excluding\s+((?:` + codePat + `)(?:\s*(?:,\s*(?:and\b|or\b)?|/|\band\b|\bor\b)\s*(?:` + codePat + `))*)\s*$`)
	// complListNoteParenRe matches a remark in brackets, inside them: "see below
	// for definitions".
	complListNoteParenRe = regexp.MustCompile(`(?i)^(?:note\b|see\b)`)
	// complAlreadyCountedRe matches the claims rule restated: "(excluding those
	// already counted)".
	complAlreadyCountedRe = regexp.MustCompile(`(?i)\s*\(?\s*excluding\s+those\s+already\s+counted\s*\)?|\s+(?:that|which)\s+(?:has|have)\s+not\s+(?:previously\s+)?been\s+(?:used|taken)\s+to\s+fulfill\s+(?:the\s+above|program)\s+requirements\b`)
	// complAnyCoursesAmountRe matches "Any 5 courses (2.5 credits)": the amount
	// is the one in brackets.
	complAnyCoursesAmountRe = regexp.MustCompile(`(?i)^any\s+(?:\d+\s+|one\s+)?courses?\s*\(\s*(\d+(?:\.\d+)?\s+credits?)\s*\)`)
	// complListPoolWordRe matches a word a pool drawn from lists may have
	// besides the lists: "credits".
	complListPoolWordRe = regexp.MustCompile(`(?i)^(?:an?|one|additional|more|further|credits?|courses?|of|in|total|at|the|levels?|any|selected|chosen|taken|from|(?-i:[A-Z]{3}(?:/[A-Z]{3})*/?)|\d+(?:\.\d+)?|[-/+]*(?:[1-4]00[-/+]*)+(?:-?level)?|-level|\+|-|/)$`)
	// complListTailRe matches where a floor or cap clause after a pool begins:
	// ", including at least".
	complListTailRe = regexp.MustCompile(`(?i)(?:,\s*|\s+|;\s*|\(\s*)(?:(?:and\s+|with\s+|but\s+)?(?:(?:including|of\s+which)(?:\s+at\s+least|\s+a\s+minimum\s+of)?|no\s+more\s+than|only\s+a\s+maximum|a\s+maximum|up\s+to|at\s+most|at\s+least)\b)`)
	// complListFillerRe matches pool words that say nothing about its courses:
	// " in total".
	complListFillerRe = regexp.MustCompile(`(?i)\s+(?:of\s+courses|in\s+total|selected|chosen|taken|of\s+which)\b`)
	// complLevelOrRe matches "300- or 400-level": one level phrase.
	complLevelOrRe = regexp.MustCompile(`(?i)\b([1-4]00)\s*-?\s+(?:or|and)\s+([1-4]00)\b`)
	// complListCapLeadRe matches a clause that caps: "no more than 1.5 credits
	// from Group B".
	complListCapLeadRe = regexp.MustCompile(`(?i)^(?:and\s+|with\s+|but\s+)?(?:only\s+)?(?:no\s+more\s+than|a\s+maximum(?:\s+of)?|up\s+to|at\s+most)\s+`)
	// complListFloorLeadRe matches the lead of a floor clause: "including at
	// least".
	complListFloorLeadRe = regexp.MustCompile(`(?i)^(?:and\s+|with\s+)?(?:including|of\s+which)?\s*(?:at\s+least|a\s+minimum\s+of)?\s*`)
	// complListModalRe matches "must be" inside a clause, which says nothing
	// about the courses.
	complListModalRe = regexp.MustCompile(`(?i)\s+(?:must|can|may|should)\s+(?:be|come)\b(?:\s+taken)?|\s+(?:must|can|may)\s+be\s+(?:counted|used)\b`)
)

// listRefPool reads text drawing an amount from lists ("1.5 credits from
// Group A") into a credits node with the lists filter; with optional, also
// text naming no list. It returns bracketed notes; ok is false if any is unread.
func listRefPool(text string, ls compListRefs, optional bool) (n *types.ReqNode, notes []string, ok bool) {
	return listRefPoolOf(text, ls, optional, nil)
}

// listRefPoolOf is listRefPool for text whose lists names are already known,
// the reference taken out of it.
func listRefPoolOf(text string, ls compListRefs, optional bool, names []string) (n *types.ReqNode, notes []string, ok bool) {
	t := strings.TrimSpace(complAlreadyCountedRe.ReplaceAllString(text, ""))
	if names == nil {
		if u, name, ok := ls.titleRef(t); ok {
			// "from Pre-1800 British Literature courses": a list named by its title
			t, names = u, []string{name}
		}
	}
	locs := complFromListRefRe.FindAllStringSubmatchIndex(t, -1)
	if names != nil && len(locs) > 0 {
		return nil, nil, false
	}
	if len(locs) > 1 || len(locs) == 0 && names == nil && !optional {
		return nil, nil, false
	}
	var excludeLevels []int
	var excludeCodes []string
	if len(locs) == 1 && names == nil {
		m := locs[0]
		keys := listRefNames(t[m[2]:m[3]])
		if m[4] >= 0 {
			// "Group 1 in Cluster A": the cluster, which is in the group
			inner := listRefNames(t[m[4]:m[5]])
			if len(keys) != 1 || len(inner) == 0 || slices.ContainsFunc(inner, func(k string) bool { return ls[k] == nil || ls[k].parent != keys[0] }) {
				return nil, nil, false
			}
			keys = inner
		}
		if names = ls.resolve(keys); names == nil {
			return nil, nil, false
		}
		if m[6] >= 0 {
			p := strings.TrimSpace(t[m[6]:m[7]])
			if e := complExcludingCodesRe.FindStringSubmatchIndex(p); e != nil {
				// "(any Cluster, excluding PSY304Y1, ...)": codes left out
				excludeCodes = codeRe.FindAllString(p[e[2]:e[3]], -1)
				p = strings.TrimSpace(strings.TrimRight(p[:e[0]], ",; "))
			}
			switch {
			case p == "" && excludeCodes != nil:
			case complListParenDropRe.MatchString(p):
			case complListNoteParenRe.MatchString(p):
				notes = append(notes, cleanNote(p))
			case len(keys) == 1 && ls[keys[0]].list.Title != "" && strings.Contains(strings.ToLower(ls[keys[0]].list.Title), strings.ToLower(p)):
				// "Group 2 (Value Theory)": the list's title
			default:
				if lv := complExcludingLevelRe.FindStringSubmatch(p); lv != nil && len(lv[0]) == len(p) {
					excludeLevels = parseLevels(lv[1] + "-level")
					break
				}
				return nil, nil, false
			}
		}
		t = t[:m[0]] + " " + t[m[1]:]
	}
	t = complDesignatorPhraseRe.ReplaceAllString(t, " in $1 courses")
	t = complLevelOrRe.ReplaceAllString(t, "$1/$2")
	t = strings.TrimSpace(complListFillerRe.ReplaceAllString(complListModalRe.ReplaceAllString(t, ""), ""))
	if m := complAnyCoursesAmountRe.FindStringSubmatchIndex(t); m != nil {
		t = t[m[2]:m[3]] + t[m[1]:]
	}
	if codeRe.MatchString(t) || strings.ContainsAny(t, "():;") {
		return nil, nil, false
	}
	for _, w := range strings.Fields(strings.NewReplacer(",", " ").Replace(t)) {
		if !complListPoolWordRe.MatchString(w) {
			return nil, nil, false
		}
	}
	p, _ := parseCompletionText(t)
	if p == nil || p.Type != types.NodeCredits || p.Amount <= 0 || p.Courses != nil || p.Items != nil || p.MaxAmount != nil || p.Lists != nil || p.Breadth != nil {
		return nil, nil, false
	}
	if names == nil && p.Designators == nil && p.Levels == nil {
		return nil, nil, false // "1.0 credit": no pool at all
	}
	p.Lists = names
	p.Exclude = excludeCodes
	if excludeLevels != nil {
		levels := []int{100, 200, 300, 400}
		if p.Levels != nil {
			levels = p.Levels
		}
		p.Levels = slices.DeleteFunc(slices.Clone(levels), func(l int) bool { return slices.Contains(excludeLevels, l) })
	}
	return p, notes, true
}

// namedLists reads the rows of text that draw on lists by name and builds
// the lists they use. It returns the lists to store.
func (a *compAssembler) namedLists() []*types.CourseList {
	a.inlineListNotes()
	built := areaLists(a.buildLists())
	ls := compListRefs{}
	for _, l := range built {
		ls[l.key] = l
	}
	a.belowRows = 0
	for _, l := range a.lines {
		if loc := complBelowListsRe.FindStringIndex(l); loc != nil && !codeRe.MatchString(l[loc[1]:]) {
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
	a.leftoverPools(ls)
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
		walkReq(r.r.Req, func(n *types.ReqNode) {
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
						it.Exclude = dedupe(append(it.Exclude, x.Courses...))
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
		if !complTopicMemberRe.MatchString(t) || !topicMember(used, t) {
			return false
		}
		drop[t] = true
		return true
	}
	// "If ARH312Y1 is completed, it will be considered a Group A course": a
	// member the list's lines don't name
	member := func(t string) bool {
		m := complListMemberRe.FindStringSubmatch(complClean(t))
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
			return r.r.Kind == types.RequirementConstraint && r.r.Req != nil && r.r.Req.Type == types.NodeText && len(r.children) == 0 && topical(complClean(r.r.Req.Text))
		})
	}
	a.items, a.program = prune(a.items), prune(a.program)
	a.notes = slices.DeleteFunc(a.notes, func(n string) bool { return drop[n] })
	for _, r := range rows {
		r.r.Notes = dedupe(slices.DeleteFunc(r.r.Notes, func(n string) bool { return drop[n] }))
		if len(r.r.Notes) == 0 {
			r.r.Notes = nil
		}
	}
	if len(a.notes) == 0 {
		a.notes = nil
	}
	return out
}

// listRefRow reads row again when its body is text that draws on lists by
// name, and returns the rows it becomes: itself, or for a text constraint the
// floors and caps it states.
func (a *compAssembler) listRefRow(row *compRow, ls compListRefs) []*compRow {
	if rows, ok := a.spreadRow(row, ls); ok {
		return rows // spreads, choices and shares among lists
	}
	n := row.r.Req
	if n == nil {
		return []*compRow{row}
	}
	switch row.r.Kind {
	case types.RequirementAllocation:
		a.poolListAlts(row, ls)
		n = row.r.Req
		text := n
		if n.Type == types.NodeOr && len(n.Items) > 1 && !slices.ContainsFunc(n.Items[1:], func(it *types.ReqNode) bool {
			return it.Type != types.NodeText || it.Kind != types.TextPermission
		}) {
			text = n.Items[0] // or(text, a route)
		}
		if text.Type == types.NodeText && text.Kind == types.TextOther {
			if text != n && a.additionalLists(row, text.Text, n.Items[1:], ls) || text == n && a.additionalLists(row, text.Text, nil, ls) {
				return nil // Req 1's own pool
			}
			if rows, ok := a.oneOffRow(row, text, ls); ok {
				return rows
			}
			t := rewriteListOneOffs(text.Text)
			body, rules, notes, ok := a.listRefAlloc(row, t, ls)
			if !ok {
				body, rules, ok = a.unionList(row, t, ls)
			}
			if !ok {
				body, rules, ok = a.reqRefAlloc(row, t, ls)
			}
			if ok {
				if text == n {
					row.r.Req = body
				} else {
					n.Items[0] = body
				}
				for _, r := range rules {
					a.rowUnder(row, r)
				}
				row.r.Notes = append(row.r.Notes, notes...)
				for _, perm := range ls.perms(body.Lists) {
					// the list's permission route, on the row
					row.r.Req = orNode(row.r.Req, textNode(types.TextPermission, perm))
				}
			}
			return []*compRow{row}
		}
		if n.Type == types.NodeAnd && len(n.Items) == 2 && n.Items[0].Type == types.NodeCredits && n.Items[1].Type == types.NodeText && complFromListRefRe.MatchString(n.Items[1].Text) {
			// "0.5 credit at the 400-level from Group B", read as and(credits, text
			// "from Group B"): the sentence again
			if sents := splitComplSentences(complClean(rawHead(row.r))); len(sents) == 1 {
				if body, rules, notes, ok := a.listRefAlloc(row, sents[0], ls); ok {
					row.r.Req = body
					for _, r := range rules {
						a.rowUnder(row, r)
					}
					row.r.Notes = append(row.r.Notes, notes...)
					return []*compRow{row}
				}
			}
		}
		if n.Type == types.NodeAnd && len(n.Items) == 2 && n.Items[1].Type == types.NodeText && complFromTextRe.MatchString(n.Items[1].Text) {
			// "1.0 ENG credits from Pre-1800 British Literature courses", read as
			// and(credits{ENG}, text): the item's whole text again
			if body, rules, notes, ok := a.listRefAlloc(row, complItemLabelRe.ReplaceAllString(complClean(rawHead(row.r)), ""), ls); ok && rules == nil {
				row.r.Req = body
				row.r.Notes = append(row.r.Notes, notes...)
				return []*compRow{row}
			}
		}
		if n.Type == types.NodeAnd {
			// "2.5 credits from Group A below" and a note: each part on its own
			for i, it := range n.Items {
				if it.Type != types.NodeText || it.Kind != types.TextOther {
					continue
				}
				if body, rules, notes, ok := a.listRefAlloc(row, it.Text, ls); ok && rules == nil {
					n.Items[i] = body
					row.r.Notes = append(row.r.Notes, notes...)
				}
			}
			// "Students taking ENV421Y1 ... only need to take 2.0 credits ...": a
			// smaller amount for some, Review for them, not text for everyone
			if len(n.Items) == 2 && n.Items[0].Type == types.NodeCredits && n.Items[1].Type == types.NodeText {
				if m := complOnlyNeedRe.FindStringSubmatch(n.Items[1].Text); m != nil {
					if v, _ := strconv.ParseFloat(m[1], 64); v > 0 && v < n.Items[0].Amount {
						less := cloneNode(n.Items[0])
						less.Amount = v
						row.r.Req = orNode(n.Items[0], andNode(less, n.Items[1]))
					}
				}
			}
		}
		return []*compRow{row}
	case types.RequirementConstraint:
		if n.Type != types.NodeText || n.Kind != types.TextOther {
			return []*compRow{row}
		}
		if m := complHalfCoursesCapRe.FindStringSubmatch(complClean(n.Text)); m != nil {
			// "The total number of half courses from Group I (including 300-level
			// courses) must not exceed 3": it counts the list's courses wherever
			// used, so it is the program's limit
			if names := ls.resolve(listRefNames(m[1])); names != nil {
				v, _ := strconv.ParseFloat(m[2], 64)
				row.r.Kind = types.RequirementLimit
				row.r.Req = &types.ReqNode{Type: types.NodeCredits, Lists: names, MaxAmount: maxAmount(v)}
				a.programRows = append(a.programRows, row)
				return nil
			}
			return []*compRow{row}
		}
		head, tails := splitListTails(complClean(n.Text))
		firsts, notes, ok := listRefRule(head, ls, nil)
		if !ok {
			return []*compRow{row}
		}
		var out []*compRow
		for _, r := range firsts {
			r.Raw = row.r.Raw
			out = append(out, &compRow{r: r})
		}
		first := firsts[0]
		for _, t := range tails {
			var within *types.ReqNode
			if len(firsts) == 1 && first.Kind == types.RequirementConstraint && !complListCapLeadRe.MatchString(t) {
				within = first.Req
			}
			rs, more, ok := listRefRule(t, ls, within)
			if !ok {
				return []*compRow{row}
			}
			for _, r := range rs {
				r.Raw = row.r.Raw
				out = append(out, &compRow{r: r})
			}
			notes = append(notes, more...)
		}
		out[0].r.Notes = append(slices.Clone(row.r.Notes), notes...)
		out[0].r.Label, out[0].r.Section = row.r.Label, row.r.Section
		out[0].children = row.children
		return out
	}
	return []*compRow{row}
}

// complFollowingListsRe matches a lead over the lists set out under it: "from
// among the following groups".
var complFollowingListsRe = regexp.MustCompile(`(?i)\bfrom\s+(?:among\s+)?the\s+following\s+(?:groups|lists?)\s*:?\s*$`)

// followingLists rewrites a lead over the lists set out under it to name the
// lists whose lines are row's notes: "from Group A, Group B, Group C".
func (a *compAssembler) followingLists(row *compRow, head string, ls compListRefs) string {
	loc := complFollowingListsRe.FindStringIndex(head)
	if loc == nil {
		return head
	}
	var names []string
	for _, d := range a.listDefs {
		if name, key := listName(d.title); key != "" && slices.Contains(row.r.Notes, d.note) && ls[key] != nil {
			names = append(names, name)
		}
	}
	if names = dedupe(names); names == nil {
		return head
	}
	return strings.TrimSpace(head[:loc[0]]) + " from " + strings.Join(names, ", ")
}

var (
	// complFromColonRe matches "from: " in "2.5 credits from: Group A".
	complFromColonRe = regexp.MustCompile(`(?i)\bfrom\s*:\s*`)
	// complSpacedSlashRe matches designators joined by a spaced slash: "ENG /JWE".
	complSpacedSlashRe = regexp.MustCompile(`\b([A-Z]{3})\s+/\s*([A-Z]{3})\b`)
	// complRefParenRe matches a bracket in a reference restating its sub-lists:
	// "(any Cluster)". Not one naming codes, which it leaves out.
	complRefParenRe = regexp.MustCompile(`(?i)\s*\((?:any(?:\s+of)?\s+)?(?:clusters?|sub-?groups?)\b[^()0-9]*\)`)
	// complAnyLevelRe matches ", at any level", which says nothing.
	complAnyLevelRe = regexp.MustCompile(`(?i),?\s+at\s+any\s+level\b`)
	// complLeadFloorRe matches an item's "At least" before its amount: what it
	// claims is at least that anyway.
	complLeadFloorRe = regexp.MustCompile(`(?i)^(?:at\s+least|a\s+minimum\s+of)\s+`)
	// complFromRefCodesRe matches a pool of courses on a list: "1.0 credit (from
	// Cluster A) PSY210H1, PSY220H1".
	complFromRefCodesRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?\s+credits?)\s*\(\s*from\s+(` + complListRefPat + `)\s*\)\s*(.+)$`)
	// complDescRefRe matches a list named after its description: "from Drama,
	// Theatre, and Performance Studies courses (Group A)".
	complDescRefRe = regexp.MustCompile(`(?i)\bfrom\s+([A-Z][^()]*?)\s+courses\s*\(\s*(` + complListRefPat + `)\s*\)`)
	// complRefTitleRe matches a reference with its list's title after a colon:
	// "Group G: Cross-Listed".
	complRefTitleRe = regexp.MustCompile(`((?i:` + complListRefPat + `))\s*:\s*([A-Z][\w ,&'’-]*)$`)
	// complOrFromRe matches one amount from either list: "One 0.5 credit from
	// Group A or one 0.5 credit from Group B below".
	complOrFromRe = regexp.MustCompile(`(?i)^(.+?)\s+from\s+(` + complListRefPat + `),?\s+or\s+(.+?)\s+from\s+(` + complListRefPat + `)(.*)$`)
	// complAmountWordsRe matches the words of an amount that don't change it:
	// "additional".
	complAmountWordsRe = regexp.MustCompile(`(?i)\b(?:one|an?|additional|in\s+any\s+area)\b`)
)

// titleWords reports whether every word of desc is in the title of the one
// list ref names.
func titleWords(desc, ref string, ls compListRefs) bool {
	keys := listRefNames(ref)
	if len(keys) != 1 || ls[keys[0]] == nil {
		return false
	}
	title := strings.ToLower(ls[keys[0]].list.Title)
	for _, w := range strings.FieldsFunc(strings.ToLower(desc), func(r rune) bool { return r == ' ' || r == ',' }) {
		if w != "and" && w != "&" && !strings.Contains(title, w) {
			return false
		}
	}
	return title != ""
}

// normalizeListRef rewrites the ways of writing a reference that listRefPool
// doesn't read into ones it does.
func normalizeListRef(s string, ls compListRefs) string {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), ":"))
	s = complFromColonRe.ReplaceAllString(s, "from ")
	s = complSpacedSlashRe.ReplaceAllString(s, "$1/$2") // "ENG /JWE credit"
	s = complRefParenRe.ReplaceAllString(s, "")
	s = complAnyLevelRe.ReplaceAllString(s, "")
	if m := complDescRefRe.FindStringSubmatchIndex(s); m != nil && titleWords(s[m[2]:m[3]], s[m[4]:m[5]], ls) {
		s = s[:m[0]] + "from " + s[m[4]:m[5]] + s[m[1]:]
	}
	if m := complRefTitleRe.FindStringSubmatchIndex(s); m != nil && titleWords(s[m[4]:m[5]], s[m[2]:m[3]], ls) {
		s = s[:m[3]]
	}
	if m := complOrFromRe.FindStringSubmatch(s); m != nil {
		amount := func(x string) string {
			return strings.Join(strings.Fields(complAmountWordsRe.ReplaceAllString(x, "")), " ")
		}
		if a, b := amount(m[1]), amount(m[3]); a == b || a == b+"s" || a+"s" == b {
			s = m[1] + " from " + m[2] + " or " + m[4] + m[5]
		}
	}
	return s
}

// listRefAlloc reads an allocation's text that draws on lists by name, or a
// pool with floors and caps the grammar doesn't read, into its body and the
// floors and caps under it.
func (a *compAssembler) listRefAlloc(row *compRow, text string, ls compListRefs) (body *types.ReqNode, rules []*types.Requirement, notes []string, ok bool) {
	t := normalizeListRef(complClean(text), ls)
	if m := complFromRefCodesRe.FindStringSubmatch(t); m != nil {
		// "1.0 credit (from Cluster A) PSY210H1, ...": courses all on the list
		names := ls.resolve(listRefNames(m[2]))
		n, _ := parseCompletionText(m[1] + " from: " + m[3])
		if names == nil || !isPool(n) || n.Designators != nil || n.Levels != nil || slices.ContainsFunc(n.Courses, func(c string) bool { return !ls.lists(names, c) }) {
			return nil, nil, nil, false
		}
		n.Lists = names
		return n, nil, nil, true
	}
	head, tails := splitQualified(t)
	head = complLeadFloorRe.ReplaceAllString(a.followingLists(row, strings.TrimSpace(complAlreadyCountedRe.ReplaceAllString(head, "")), ls), "")
	if rest, names, below := a.belowLists(row, head, ls); below {
		body, notes, ok = listRefPoolOf(rest, ls, false, names)
	}
	if !ok {
		// the lists below, or else by name or title
		body, notes, ok = listRefPool(head, ls, false)
	}
	if !ok && tails != nil && codeRe.MatchString(head) {
		// a pool of courses with its floors and caps written in it
		var caps []*types.ReqNode
		var more []string
		body, caps, more, ok = qualifiedPool(head)
		notes = append(notes, more...)
		for _, c := range caps {
			rules = append(rules, &types.Requirement{Kind: types.RequirementLimit, Req: c, Raw: cleanNote(text)})
		}
	} else if !ok {
		body, ok = a.listUnion(row, head, ls)
	}
	if !ok {
		return nil, nil, nil, false
	}
	if body.Lists != nil && len(body.Lists) > 1 && complFromEitherRe.MatchString(head) {
		// "... from either Group A, B or C": from one of the lists, not their
		// union. A sub-list whose list is named too is part of that choice.
		var alts []*types.ReqNode
		for _, name := range body.Lists {
			if ls.within(name, body.Lists) {
				continue
			}
			alt := cloneNode(body)
			alt.Lists = []string{name}
			alts = append(alts, alt)
		}
		body = orNode(alts...)
	}
	var last *types.Requirement
	for _, tail := range tails {
		var within *types.ReqNode
		if last != nil && last.Kind == types.RequirementConstraint && complOfWhichRe.MatchString(tail) {
			within = last.Req // "..., at least 1.0 credit of which must be at the 400-level"
		}
		rs, more, ok := listRefRule(normalizeListRef(tail, ls), ls, within)
		if !ok {
			return nil, nil, nil, false
		}
		for _, r := range rs {
			if r.Kind == types.RequirementLimit && r.Req.Lists != nil && slices.ContainsFunc(r.Req.Lists, func(l string) bool { return !slices.Contains(body.Lists, l) }) {
				// "...; up to 1.0 credit can be from Group B": B's share joins the
				// pool, capped
				if !complUpToRe.MatchString(tail) || body.Lists == nil || body.Courses != nil || body.Designators != nil || body.Levels != nil || r.Req.Designators != nil || r.Req.Levels != nil {
					return nil, nil, nil, false
				}
				body.Lists = dedupe(append(body.Lists, r.Req.Lists...))
			}
			r.Raw = cleanNote(text)
			rules = append(rules, r)
		}
		if len(rs) == 1 {
			last = rs[0]
		}
		notes = append(notes, more...)
	}
	// "... of which 0.5 credit must be at the 400-level: ..., MSE540H1": a
	// listed course above the floor's level may be meant to count (two readings)
	var above []string
	for _, c := range body.Courses {
		if len(c) == 8 && c[3] >= '5' && c[3] <= '9' {
			above = append(above, c)
		}
	}
	for _, r := range rules {
		if r.Kind != types.RequirementConstraint || r.Req.Type != types.NodeCredits || above == nil || !slices.Contains(r.Req.Levels, 400) {
			continue
		}
		maybe := &types.ReqNode{Type: types.NodeCredits, Amount: r.Req.Amount, Courses: slices.Clone(above)}
		r.Req = orNode(r.Req, andNode(maybe, textNode(types.TextAmbiguity, strings.Join(above, ", ")+" is above the 400-level the floor names: it may count toward it")))
	}
	return body, rules, notes, true
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

// titleRef finds in t a reference to one list named by its title ("from
// Pre-1800 British Literature courses from the list below") and returns t
// without it.
func (ls compListRefs) titleRef(t string) (rest, name string, ok bool) {
	found := 0
	for _, l := range ls {
		if !l.titled || l.bad {
			continue
		}
		title := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(l.list.Name), " courses"))
		title = strings.TrimSuffix(title, " Courses")
		// "Cultural, Postcolonial, and Transnational studies" names "Cultural,
		// Postcolonial & Transnational Studies"
		q := strings.NewReplacer(" & ", `,?\s+(?:&|and)\s+`, " and ", `,?\s+(?:&|and)\s+`).Replace(regexp.QuoteMeta(title))
		re := regexp.MustCompile(`(?i)\s*\b(?:from|in)\s+(?:courses\s+(?:from|in)\s+)?(?:the\s+)?` + q +
			`(?:\s+(?:courses?|concentration|area|group|list))?(?:\s+(?:from\s+)?(?:the\s+)?list\s+below|\s+below|\s+listed\s+below)?\b`)
		if loc := re.FindStringIndex(t); loc != nil {
			found++
			rest, name = t[:loc[0]]+" "+t[loc[1]:], l.list.Name
		}
	}
	if found != 1 {
		return "", "", false
	}
	return rest, name, true
}

var (
	// complBelowListsRe matches a pointer to the lists set out below an item, by
	// none of their names: "from the following elective courses".
	complBelowListsRe = regexp.MustCompile(`(?i)\s*(?:\bfrom\s+(?:any\s+of\s+)?the\s+(?:courses\s+)?(?:list(?:ed)?\s+)?below\b|\bfrom\s+any\s+of\s+the\s+courses\s+(?:listed\s+)?below\b|\bfrom\s+the\s+following(?:\s+(?:elective|eligible|approved))?(?:\s+(?:courses|list))?\b|\bfrom\s+(?:approved\s+)?(?-i:[A-Z])[\w\s,&-]*?\s+courses\s+as\s+listed\s+below\b|\(\s*list\s+below\s*\))`)
	// complListBelowParenRe matches "(list below)" after a description of the
	// list.
	complListBelowParenRe = regexp.MustCompile(`(?i)\(\s*list\s+below\s*\)`)
	// complAmountOnlyRe matches an amount and a description of its list: "0.5
	// credit with a significant emphasis on ethics".
	complAmountOnlyRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?\s+credits?)\s+with\s+[^()]*$`)
)

// belowLists reads text pointing at the lists set out below it as drawing on
// the program-level lists defined after row, when row alone points below.
// "(list below)" names one list. ok is false when text doesn't point below.
func (a *compAssembler) belowLists(row *compRow, text string, ls compListRefs) (rest string, names []string, ok bool) {
	loc := complBelowListsRe.FindStringIndex(text)
	if loc == nil || a.belowRows != 1 {
		return text, nil, false
	}
	line := slices.IndexFunc(a.lines, func(l string) bool { return strings.TrimSpace(l) == rawHead(row.r) })
	if line < 0 {
		return text, nil, false
	}
	var after []*compList
	for _, l := range a.sortedLists(ls) {
		first := l.defs[0]
		if first.block == nil || first.row != nil && first.row != row || first.block.line < line {
			continue
		}
		if l.bad {
			return text, nil, false
		}
		after = append(after, l)
	}
	single := complListBelowParenRe.MatchString(text)
	if len(after) == 0 || single && len(after) != 1 {
		return text, nil, false
	}
	for _, l := range after {
		names = append(names, l.list.Name)
	}
	rest = strings.TrimSpace(text[:loc[0]] + " " + text[loc[1]:])
	if single {
		if m := complAmountOnlyRe.FindStringSubmatch(rest); m != nil {
			rest = m[1]
		}
	}
	return rest, names, true
}

// sortedLists returns the lists that have definitions, in the order of their
// first definition's line.
func (a *compAssembler) sortedLists(ls compListRefs) []*compList {
	var out []*compList
	for _, l := range ls {
		if len(l.defs) > 0 {
			out = append(out, l)
		}
	}
	slices.SortFunc(out, func(x, y *compList) int {
		if d := complDefLine(x.defs[0]) - complDefLine(y.defs[0]); d != 0 {
			return d
		}
		return strings.Compare(x.key, y.key) // ls is a map: no order of its own
	})
	return out
}

// complDefLine is the line a definition is on, -1 for a sentence.
func complDefLine(d *compListDef) int {
	if d.block == nil {
		return -1
	}
	return d.block.line
}

// perms returns the permission routes of the named lists and the lists under
// them.
func (ls compListRefs) perms(names []string) []string {
	var out []string
	for _, l := range ls {
		if !slices.Contains(names, l.list.Name) {
			continue
		}
		if l.perm != "" {
			out = append(out, l.perm)
		}
		for _, it := range l.list.Items {
			out = append(out, ls.perms(it.Lists)...)
		}
	}
	slices.Sort(out)
	return dedupe(out)
}

// unionList reads text the grammar reads as a union pool of courses and
// filters, which one credits node can't hold, as a list of its own the row
// draws its amount from. Caps on its "/" pairs are limits under the row.
func (a *compAssembler) unionList(row *compRow, text string, ls compListRefs) (*types.ReqNode, []*types.Requirement, bool) {
	t := complClean(text)
	n, _ := parseCompletionText(t)
	if f := filterUnion(t); f != nil {
		n = f // "100- or 200-level ESS courses or 200-level JEG courses"
	}
	if n == nil || hasText(n) {
		return nil, nil, false
	}
	pool, caps := poolCaps(n)
	if pool == nil || pool.Type != types.NodeCredits || pool.Items == nil || pool.Amount <= 0 || pool.MaxAmount != nil {
		return nil, nil, false
	}
	for _, it := range pool.Items {
		if it.Type != types.NodeCredits || it.Items != nil || it.Amount != 0 || it.MaxAmount != nil {
			return nil, nil, false
		}
	}
	name, _, ok := a.addUnion(row, pool.Items, ls)
	if !ok {
		return nil, nil, false
	}
	body := &types.ReqNode{Type: types.NodeCredits, Amount: pool.Amount, Lists: []string{name}}
	var rules []*types.Requirement
	for _, c := range caps {
		rules = append(rules, &types.Requirement{Kind: types.RequirementLimit, Req: c, Raw: cleanNote(text)})
	}
	return body, rules, true
}

// complFromEitherRe matches a choice of one of the lists after it: "from
// either Group A, B or C".
var complFromEitherRe = regexp.MustCompile(`(?i)\bfrom\s+either\s+(?:the\s+)?` + complListRefPat)

func (ls compListRefs) within(name string, names []string) bool {
	for _, l := range ls {
		if l.list.Name != name || l.parent == "" || ls[l.parent] == nil {
			continue
		}
		return slices.Contains(names, ls[l.parent].list.Name)
	}
	return false
}

// complOnlyNeedRe matches a smaller amount for some students: "only need to
// take 2.0 credits".
var complOnlyNeedRe = regexp.MustCompile(`(?i)\b(?:only\s+need\s+to|need\s+only)\s+(?:take|complete)\s+(?:an\s+additional\s+)?(\d+(?:\.\d+)?)\s+credits?\b`)

// complFromTextRe matches the text the grammar left of an item that names
// where its courses come from: "from Pre-1800 British Literature courses".
var complFromTextRe = regexp.MustCompile(`(?i)^from\s+\S`)

// complItemLabelRe matches an item's number before its text: "5. ".
var complItemLabelRe = regexp.MustCompile(`^\s*(?:\d{1,2}[a-z]?|[a-z])[.)]\s+`)

// complListMemberRe matches a sentence that puts a course on a list: "If
// ARH312Y1 is completed, it will be considered a Group A course".
var complListMemberRe = regexp.MustCompile(`(?i)^if\s+(` + codePat + `)\s+is\s+(?:completed|taken),\s+it\s+will\s+be\s+considered\s+(?:an?\s+)?(` + complListRefPat + `)\s+course\.?$`)

// complTopicMemberRe matches a course whose list depends on its topic:
// "ANT390H1 may be placed in either Group A or B or C depending on the topic".
var complTopicMemberRe = regexp.MustCompile(`(?i)^(` + codePat + `)\s+may\s+(?:be\s+placed\s+in|not\s+be\s+available\s+in)\b[^.]*\bdepending\s+on\s+the\s+topic\b`)

// topicMember makes the course of a complTopicMemberRe sentence a maybe
// member, and(course, permission text), of each used list that lists it. It
// reports whether a list took it.
func topicMember(used []*compList, sentence string) bool {
	code := complTopicMemberRe.FindStringSubmatch(sentence)[1]
	took := false
	for _, l := range used {
		for i, it := range l.list.Items {
			if it.Type != types.NodeCredits || it.Designators != nil || it.Levels != nil || it.Weight != 0 || !slices.Contains(it.Courses, code) {
				continue
			}
			it.Courses = slices.DeleteFunc(slices.Clone(it.Courses), func(c string) bool { return c == code })
			maybe := andNode(&types.ReqNode{Type: types.NodeCredits, Courses: []string{code}}, textNode(types.TextPermission, sentence))
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

var (
	// complUnionFromRe matches a pool whose source follows "from": "1.5
	// additional credits from: Group B and/or ANT406H1". complUnionLeadFromRe
	// matches one it leads: "From Group A, B, C or other DRM: 2.0 credits".
	complUnionFromRe     = regexp.MustCompile(`(?i)^(.*?)\s+from:?\s+(.+)$`)
	complUnionLeadFromRe = regexp.MustCompile(`(?i)^from\s+(.+?):\s*(\d+(?:\.\d+)?\s+credits?)(?:\s+to\s+make\s+up\s+the\s+total\s+of\s+\d+(?:\.\d+)?\s+credits?)?$`)
	// complUnionRestRe matches the separators a union's source may have besides
	// its lists, codes and "other DRM".
	complUnionRestRe = regexp.MustCompile(`(?i)^(?:[\s,/]|\band/or\b|\band\b|\bor\b)*$`)
	// complOtherDesigRe matches "other DRM": the designator's courses.
	complOtherDesigRe = regexp.MustCompile(`\b(?:(?i:other)\s+)([A-Z]{3})\b(?:\s+(?i:courses))?`)
)

// listUnion reads a pool drawn from lists and courses together as a list of
// its own, named after the row, with an item for each list, the courses and
// any "other DRM" designator.
func (a *compAssembler) listUnion(row *compRow, head string, ls compListRefs) (*types.ReqNode, bool) {
	var amount, source string
	if m := complUnionLeadFromRe.FindStringSubmatch(head); m != nil {
		amount, source = m[2], m[1]
	} else if m := complUnionFromRe.FindStringSubmatch(head); m != nil {
		amount, source = m[1], m[2]
	} else {
		return nil, false
	}
	var items []*types.ReqNode
	refs := complListRefRe.FindAllStringIndex(source, -1)
	if refs == nil || row.r.Label == "" {
		return nil, false
	}
	rest := source
	for i := len(refs) - 1; i >= 0; i-- {
		m := refs[i]
		names := ls.resolve(listRefNames(source[m[0]:m[1]]))
		if names == nil {
			return nil, false
		}
		items = append(items, &types.ReqNode{Type: types.NodeCredits, Lists: names})
		end := m[1]
		if p := complTitleParenRe.FindStringIndex(source[end:]); p != nil && strings.TrimSpace(source[end:end+p[0]]) == "" && len(names) == 1 &&
			titleWords(strings.Trim(source[end+p[0]:end+p[1]], "()"), source[m[0]:m[1]], ls) {
			end += p[1] // "Group 2 (Value Theory)"
		}
		rest = rest[:m[0]] + " " + rest[end:]
	}
	slices.Reverse(items)
	if codes := complAnyCodeRe.FindAllString(rest, -1); codes != nil {
		items = append(items, &types.ReqNode{Type: types.NodeCredits, Courses: dedupe(codes)})
	}
	for _, m := range complOtherDesigRe.FindAllStringSubmatch(rest, -1) {
		items = append(items, &types.ReqNode{Type: types.NodeCredits, Designators: []string{m[1]}})
		// "other DRM" beside a list titled "Other Topics ...": the program's other
		// courses are that list's too
		for _, l := range a.sortedLists(ls) {
			if l.titled && !l.bad && strings.HasPrefix(strings.ToLower(l.list.Name), "other ") {
				items = append(items, &types.ReqNode{Type: types.NodeCredits, Lists: []string{l.list.Name}})
			}
		}
	}
	rest = complOtherDesigRe.ReplaceAllString(complAnyCodeRe.ReplaceAllString(rest, " "), " ")
	if !complUnionRestRe.MatchString(rest) || len(items) < 2 {
		return nil, false
	}
	n, _, ok := listRefPoolOf(amount, ls, false, []string{"x"})
	if !ok {
		return nil, false
	}
	name, _, ok := a.addUnion(row, items, ls)
	if !ok {
		return nil, false
	}
	n.Lists = []string{name}
	return n, true
}

// memberLines returns, as list definitions, lines under a heading naming a
// list that the reading kept as some other note: a line of codes, or one
// saying what the list holds. Its note is whichever form the reading kept.
func (a *compAssembler) memberLines() []*compListDef {
	seen := map[*compBlock]bool{}
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
	var walk func(bs []*compBlock)
	walk = func(bs []*compBlock) {
		for _, b := range bs {
			walk(b.children)
			h := b.heading
			if seen[b] || b.kind == blockHeading || h == nil || h.kind != blockHeading || len(b.children) > 0 {
				continue
			}
			if _, key := listName(h.section); key == "" || !complMemberLineRe.MatchString(strings.TrimSpace(b.text)) && !mostlyCodes(b.text) {
				continue
			}
			for _, note := range []string{cleanNote(b.text), cleanNote(blockLine(b)), h.section + ": " + cleanNote(b.text)} {
				if notes[note] {
					out = append(out, &compListDef{title: h.section, members: b.text, note: note, block: b})
					break
				}
			}
		}
	}
	walk(a.tree)
	return out
}

var (
	// complUpToListRe matches an alternative adding a list's share to a pool:
	// "Up to 1.5 credits from Group I".
	complUpToListRe = regexp.MustCompile(`(?i)^(?:up\s+to\s+(\d+(?:\.\d+)?)\s+credits?|any\s+course\s+\(\s*(\d+(?:\.\d+)?)\s+credits?\s*\))\s+(?:can\s+be\s+(?:used\s+)?)?from\s+(` + complListRefPat + `)$`)
	// complReqAltRe matches an alternative adding another requirement's courses
	// to a pool: "any courses from program requirement (1) above".
	complReqAltRe = regexp.MustCompile(`(?i)^any\s+courses?\s+from\s+(?:program\s+)?requirement\s+\(?#?(\d{1,2})\)?(?:\s+above)?$`)
	// complListMembersAltRe matches an alternative adding a list's members to a
	// pool: "400-level courses in Group I".
	complListMembersAltRe = regexp.MustCompile(`(?i)^(?:and\s+)?(?:any\s+)?(?:([1-4]00(?:[-/+\s]*[1-4]00)*[-+]?)\s*-?\s*level\s+)?courses?\s+(?:in|from)\s+(` + complListRefPat + `)$`)
	// complBareListAltRe matches an alternative that is a list, with its title
	// in brackets: "Group 2 (Value Theory)".
	complBareListAltRe = regexp.MustCompile(`(?i)^(` + complListRefPat + `)(?:\s*\(\s*([^()]*?)\s*\))?$`)
)

// poolListAlts merges row's pool with the or'd alternatives that add
// courses from lists into one list the row draws its amount from; a capped
// share is a limit under the row. Other alternatives stay.
func (a *compAssembler) poolListAlts(row *compRow, ls compListRefs) {
	n := row.r.Req
	if row.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeOr || len(n.Items) < 2 {
		return
	}
	pool := n.Items[0]
	if !isPool(pool) || pool.Lists != nil || pool.Requirements != nil || row.r.Label == "" {
		return
	}
	member := cloneNode(pool)
	member.Amount = 0
	items := []*types.ReqNode{member}
	var limits []*types.Requirement
	var rest []*types.ReqNode
	var refs []compReqRef
	for _, it := range n.Items[1:] {
		t := ""
		if it.Type == types.NodeText && it.Kind == types.TextOther {
			t = strings.TrimSpace(strings.TrimSuffix(complClean(it.Text), "."))
		}
		if m := complUpToListRe.FindStringSubmatch(t); m != nil {
			names := ls.resolve(listRefNames(m[3]))
			if names == nil {
				return
			}
			v, _ := strconv.ParseFloat(m[1]+m[2], 64)
			items = append(items, &types.ReqNode{Type: types.NodeCredits, Lists: names})
			limits = append(limits, &types.Requirement{Kind: types.RequirementLimit, Raw: cleanNote(it.Text),
				Req: &types.ReqNode{Type: types.NodeCredits, Lists: slices.Clone(names), MaxAmount: maxAmount(v)}})
			continue
		}
		if m := complReqAltRe.FindStringSubmatch(t); m != nil {
			r := a.labelled(m[1])
			if r == nil || r == row {
				return
			}
			ref := &types.ReqNode{Type: types.NodeCredits, Requirements: []int{}}
			refs = append(refs, compReqRef{node: ref, rows: []*compRow{r}})
			items = append(items, ref)
			continue
		}
		if m := complBareListAltRe.FindStringSubmatch(t); m != nil && (m[2] == "" || titleWords(m[2], m[1], ls)) {
			// "... / Group 2 (Value Theory)": the list itself, with its title
			names := ls.resolve(listRefNames(m[1]))
			if names == nil {
				return
			}
			items = append(items, &types.ReqNode{Type: types.NodeCredits, Lists: names})
			continue
		}
		if m := complListMembersAltRe.FindStringSubmatch(t); m != nil {
			names := ls.resolve(listRefNames(m[2]))
			if names == nil {
				return
			}
			f := &types.ReqNode{Type: types.NodeCredits, Lists: names}
			if m[1] != "" {
				f.Levels = parseLevels(m[1] + "-level")
			}
			items = append(items, f)
			continue
		}
		rest = append(rest, it)
	}
	if len(items) == 1 {
		return
	}
	name, l, ok := a.addUnion(row, items, ls)
	if !ok {
		return
	}
	for _, ref := range refs {
		ref.list = l.list
		a.reqRefs = append(a.reqRefs, ref)
	}
	body := &types.ReqNode{Type: types.NodeCredits, Amount: pool.Amount, Lists: []string{name}}
	row.r.Req = orNode(append([]*types.ReqNode{body}, rest...)...)
	for _, r := range limits {
		a.rowUnder(row, r)
	}
}

// complFilterUnionRe matches an amount of two filters joined by "or", with
// the codes either excludes: "2.0 credits of 100- or 200-level ESS courses or
// 200-level JEG courses (excluding ESS103H1)".
var complFilterUnionRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits?\s+(?:of|from|in)\s+(.+?)(?:\s*\(\s*excluding\s+([^()]*)\))?$`)

// filterUnion reads a complFilterUnionRe pool as credits whose items are the
// two filters, or returns nil.
func filterUnion(t string) *types.ReqNode {
	m := complFilterUnionRe.FindStringSubmatch(t)
	if m == nil {
		return nil
	}
	f := complFilterOrFilterRe.FindStringSubmatch(m[2])
	if f == nil || len(f[0]) != len(strings.TrimSpace(m[2])) {
		return nil
	}
	excluded := codeRe.FindAllString(m[3], -1)
	if m[3] != "" && !complListRestRe.MatchString(codeRe.ReplaceAllString(m[3], " ")) {
		return nil
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	n := &types.ReqNode{Type: types.NodeCredits, Amount: amount}
	for _, part := range [][2]string{{f[1], f[2]}, {f[3], f[4]}} {
		it := &types.ReqNode{Type: types.NodeCredits, Designators: desigRe.FindAllString(part[1], -1), Levels: parseLevels(part[0] + "-level")}
		for _, c := range excluded {
			if slices.Contains(it.Designators, c[:3]) {
				it.Exclude = append(it.Exclude, c)
			}
		}
		n.Items = append(n.Items, it)
	}
	if slices.ContainsFunc(excluded, func(c string) bool {
		return !slices.ContainsFunc(n.Items, func(it *types.ReqNode) bool { return slices.Contains(it.Exclude, c) })
	}) {
		return nil // an excluded course neither filter matches
	}
	return n
}

// addUnion records items as a list of row's own and returns its name:
// "Requirement <label> courses", then "(2)", "(3)" for more of one row. A row
// with no label gets none (ok false).
func (a *compAssembler) addUnion(row *compRow, items []*types.ReqNode, ls compListRefs) (name string, l *compList, ok bool) {
	if row.r.Label == "" {
		return "", nil, false
	}
	base := "Requirement " + row.r.Label
	if !strings.HasSuffix(strings.ToLower(base), " courses") {
		base += " courses"
	}
	name = base
	for k := 2; ls[complUnionKey(name)] != nil; k++ {
		name = base + " (" + strconv.Itoa(k) + ")"
	}
	l = &compList{key: complUnionKey(name), list: &types.CourseList{Name: name, Items: items}, line: math.MaxInt32 - 1}
	ls[l.key] = l
	a.unions = append(a.unions, l)
	return name, l, true
}

// complUnionKey is the key a list addUnion made is stored under.
func complUnionKey(name string) string { return "union:" + strings.ToLower(name) }

// complIncludeRecommendRe matches advice to include courses: "recommended to
// include".
var complIncludeRecommendRe = regexp.MustCompile(`(?i)\brecommended\s+to\s+include\b`)

// recommendedMembers adds to the list a row's pool became the courses the
// row's advice recommends that the list doesn't hold, as an ambiguity maybe
// item.
func (a *compAssembler) recommendedMembers(r *compRow, ls compListRefs) {
	n := r.r.Req
	if r.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeCredits || len(n.Lists) != 1 {
		return
	}
	l := ls[complUnionKey(n.Lists[0])]
	if l == nil {
		return
	}
	for _, note := range r.r.Notes {
		if !complIncludeRecommendRe.MatchString(note) {
			continue
		}
		var missing []string
		for _, code := range codeRe.FindAllString(note, -1) {
			if !slices.ContainsFunc(l.list.Items, func(it *types.ReqNode) bool { return filterHolds(it, code) }) {
				missing = append(missing, code)
			}
		}
		if missing = dedupe(missing); missing != nil {
			l.list.Items = append(l.list.Items, andNode(&types.ReqNode{Type: types.NodeCredits, Courses: missing},
				textNode(types.TextAmbiguity, "the item recommends "+strings.Join(missing, " and ")+", which its pool doesn't name")))
		}
	}
}

// filterHolds reports whether list item it matches code by its courses or
// by its designators and levels (not its lists).
func filterHolds(it *types.ReqNode, code string) bool {
	if it.Type != types.NodeCredits || slices.Contains(it.Exclude, code) || slices.Contains(it.ExcludeDesignators, code[:3]) ||
		it.MaxCourseAmount != 0 && courseWeight(code) > it.MaxCourseAmount {
		return false
	}
	if slices.Contains(it.Courses, code) {
		return true
	}
	return it.Designators != nil && slices.Contains(it.Designators, code[:3]) && levelMatches(it.Levels, code)
}

// complInlineListRe matches the total's pool naming a list below whose codes
// it reads inline: "or Equivalent courses (listed below)".
var complInlineListRe = regexp.MustCompile(`(?i)\bor\s+([A-Z][\w\s]*?)\s+courses\s+\(listed\s+below\)`)

// inlineListNotes drops a complInlineListRe list's definition from the notes
// once the total's pool holds all its codes: it is encoded.
func (a *compAssembler) inlineListNotes() {
	if a.total == nil || a.total.r.Req == nil {
		return
	}
	m := complInlineListRe.FindStringSubmatch(a.total.r.Raw)
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
		for k++; k < len(a.lines) && (isCodeList(a.widenLine(k)) || mostlyCodes(a.widenLine(k))); k++ {
			if slices.ContainsFunc(codeRe.FindAllString(a.lines[k], -1), func(c string) bool { return !slices.Contains(pool.Courses, c) }) {
				return
			}
			used = append(used, a.widenLine(k))
		}
		a.dropEncoded(used)
		return
	}
}

// complListSubCapRe matches a cap on some of a list's members at the end of
// its definition: ", 1.0 credit from TRN160Y1, TRN161Y1".
var complListSubCapRe = regexp.MustCompile(`(?i),\s*(\d+(?:\.\d+)?)\s+credits?\s+from\s+((?:` + codePat + `)(?:\s*,\s*(?:` + codePat + `))+)\s*\.?\s*$`)

// listSubCaps makes a complListSubCapRe cap a limit under each allocation
// drawing on the list by name, since the list's items can't hold it.
func (a *compAssembler) listSubCaps(ls compListRefs) {
	for _, l := range a.sortedLists(ls) {
		if l.bad || len(l.defs) != 1 {
			continue
		}
		m := complListSubCapRe.FindStringSubmatch(l.defs[0].members)
		if m == nil {
			continue
		}
		v, _ := strconv.ParseFloat(m[1], 64)
		codes := dedupe(codeRe.FindAllString(m[2], -1))
		if got, _, ok := ls.members(l.list.Name); !ok || slices.ContainsFunc(codes, func(c string) bool { return !slices.Contains(got, c) }) {
			continue
		}
		cap := &types.ReqNode{Type: types.NodeCredits, Courses: codes, MaxAmount: maxAmount(v)}
		for _, r := range a.items {
			uses := false
			walkReq(r.r.Req, func(x *types.ReqNode) {
				uses = uses || x.Type == types.NodeCredits && slices.Contains(x.Lists, l.list.Name)
			})
			if r.r.Kind == types.RequirementAllocation && uses {
				a.rowUnder(r, &types.Requirement{Kind: types.RequirementLimit, Req: cloneNode(cap), Raw: cleanNote(l.defs[0].members)})
			}
		}
	}
}
