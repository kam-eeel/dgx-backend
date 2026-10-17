package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Filter pools: pools that mix a designator's courses at a level with listed
// courses.

// filterPools reads the filter pools in the rows and returns lists with the
// lists they add.
func (a *compAssembler) filterPools(lists []*types.CourseList) []*types.CourseList {
	for _, row := range a.allRows() {
		a.otherSeries(row)
		lists = a.electiveKinds(row, lists)
	}
	return lists
}

// complOtherSeriesRe matches a designator's series plus listed courses: "An
// additional 1.0 credit at the 4th year level from the following list: any
// other BCH 400-series course, BCB420H1".
var complOtherSeriesRe = regexp.MustCompile(`(?i)^(?:an\s+)?additional\s+(\d+(?:\.\d+)?)\s+credits?\s+at\s+the\s+(\d)(?:st|nd|rd|th)\s+year\s+level\s+from\s+the\s+following\s+list:\s*any\s+other\s+((?-i:[A-Z]{3}))\s+(\d)00-series\s+courses?(?:\s+including\s+([^,(]+?))?(?:\s*\(\s*see\s+note\s+\d+\s*\))?\s*,\s*(.+)$`)

// otherSeries encodes a row matching complOtherSeriesRe as credits{amount,
// designators, levels, courses}. "Other" is the claims rule; what the series
// "includes" is already in it.
func (a *compAssembler) otherSeries(row *compRow) {
	n := row.r.Req
	if row.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || len(row.children) > 0 {
		return
	}
	m := complOtherSeriesRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(complClean(n.Text)), "."))
	if m == nil || m[2] != m[4] {
		return
	}
	if m[5] != "" && !regexp.MustCompile(`(?i)^the\s+`+m[2]+`(?:st|nd|rd|th)\s+year\s+`+m[3]+`\s+[\w\s-]*courses$`).MatchString(strings.TrimSpace(m[5])) {
		return
	}
	codes := complAnyCodeRe.FindAllString(m[6], -1)
	level := []int{100 * int(m[4][0]-'0')}
	if codes == nil || !complListRestRe.MatchString(complAnyCodeRe.ReplaceAllString(m[6], " ")) ||
		slices.ContainsFunc(codes, func(c string) bool { return !levelMatches(level, c) }) {
		return
	}
	amount, _ := strconv.ParseFloat(m[1], 64)
	row.r.Req = &types.ReqNode{Type: types.NodeCredits, Amount: amount, Designators: []string{m[3]}, Levels: level, Courses: dedupe(codes)}
}

var (
	// complElectiveKindsRe matches electives with a floor on one kind: "2.5
	// credits from electives in composition, ..., of which at least 1.5 credits
	// must be completed from composition courses: CMU113H1".
	complElectiveKindsRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+credits\s+from\s+electives\s+in\s+.+?,\s+of\s+which\s+at\s+least\s+(\d+(?:\.\d+)?)\s+credits?\s+must\s+be\s+completed\s+from\s+(.+?\s+courses):\s*(.+)$`)
	// complLevelPlusMemberRe matches a member: "HMU200+ level courses".
	complLevelPlusMemberRe = regexp.MustCompile(`^([A-Z]{3})\s*([1-4])00\+\s*level\s+courses$`)
	// complKindCoursesRe matches a note listing one kind of courses: "Music
	// history courses: HMU200+ level courses".
	complKindCoursesRe = regexp.MustCompile(`^([A-Z][^:]*?\scourses):\s*(.+)$`)
)

// electiveKinds encodes a row matching complElectiveKindsRe, with a note
// listing the other kind's courses, as credits over both kinds' lists with the
// floor's kind as a constraint under it.
func (a *compAssembler) electiveKinds(row *compRow, lists []*types.CourseList) []*types.CourseList {
	n := row.r.Req
	if row.r.Kind != types.RequirementAllocation || n == nil || n.Type != types.NodeText || len(row.children) > 0 || len(row.r.Notes) == 0 {
		return lists
	}
	m := complElectiveKindsRe.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(complClean(n.Text)), "."))
	if m == nil {
		return lists
	}
	codes := complAnyCodeRe.FindAllString(m[4], -1)
	if codes == nil || !complListRestRe.MatchString(complAnyCodeRe.ReplaceAllString(m[4], " ")) {
		return lists
	}
	var other *types.CourseList
	var note string
	for _, s := range row.r.Notes {
		g := complKindCoursesRe.FindStringSubmatch(strings.TrimSpace(s))
		if g == nil || other != nil {
			continue
		}
		var items []*types.ReqNode
		var plain []string
		for _, part := range strings.Split(strings.TrimSuffix(g[2], "."), ";") {
			part = strings.TrimSpace(part)
			if f := complLevelPlusMemberRe.FindStringSubmatch(part); f != nil {
				var levels []int
				for lv := int(f[2][0]-'0') * 100; lv <= 400; lv += 100 {
					levels = append(levels, lv)
				}
				items = append(items, &types.ReqNode{Type: types.NodeCredits, Designators: []string{f[1]}, Levels: levels})
				continue
			}
			if !codeRe.MatchString(part) || codeRe.FindString(part) != part {
				return lists
			}
			plain = append(plain, part)
		}
		if plain != nil {
			items = append(items, &types.ReqNode{Type: types.NodeCredits, Courses: plain})
		}
		other, note = &types.CourseList{Name: g[1], Items: items}, s
	}
	floorName := strings.ToUpper(m[3][:1]) + m[3][1:]
	if other == nil || strings.EqualFold(floorName, other.Name) ||
		slices.ContainsFunc(lists, func(l *types.CourseList) bool { return l.Name == floorName || l.Name == other.Name }) {
		return lists
	}
	lists = append(lists, &types.CourseList{Name: floorName, Items: []*types.ReqNode{{Type: types.NodeCredits, Courses: dedupe(codes)}}}, other)
	amount, _ := strconv.ParseFloat(m[1], 64)
	floor, _ := strconv.ParseFloat(m[2], 64)
	row.r.Req = &types.ReqNode{Type: types.NodeCredits, Amount: amount, Lists: []string{floorName, other.Name}}
	row.children = append(row.children, &compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Raw: rawHead(row.r),
		Req: &types.ReqNode{Type: types.NodeCredits, Amount: floor, Lists: []string{floorName}}}})
	row.r.Notes = slices.DeleteFunc(row.r.Notes, func(s string) bool { return s == note })
	addRaw(row, note)
	return lists
}
