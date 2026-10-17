package programs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/parser/grammar"
	"degree-explorer/types"
)

// Item helpers: slash grades, course levels, averages over courses and the
// fewest credits that meet a node.

var (
	// ItemConjRe matches a trailing conjunction joining an item to the next:
	// "COG100H1 and".
	ItemConjRe = regexp.MustCompile(`(?i)[\s,;]*\b(or|and)\s*$`)
	// SlashGradeRe matches one grade after a slash list, which covers the list:
	// "MAT130H1/ MAT135H1 (60%)".
	SlashGradeRe = regexp.MustCompile(`((?:` + grammar.CodePat + `\s*/\s*)+)(` + grammar.CodePat + `)\s*\(\s*(\d{2})%\s*\)`)
	// anyLevelRe and ofWhichMaxRe rewrite "1.5 credits from SOC/ PSY/ IRE/ IRW,
	// at any level, of which no more than 0.5 credit can be IRE/ IRW" as a cap.
	AnyLevelRe = regexp.MustCompile(`(?i),?\s*at\s+any\s+level\b`)

	// ItemAvgRe matches an average over the group before it: "( ACT240H1 and
	// ACT245H1) with an average grade of 63%".
	ItemAvgRe = regexp.MustCompile(`(?i),?\s*with\s+(?:` + grammar.AtLeastPat + `\s+)?an?\s+(?:combined\s+)?(?:minimum\s+)?(?:(?:average\s+grade|grade\s+average|average)\s+of\s+(\d{2})%|(\d{2})%\s+average)(?:\s+across\s+(?:all\s+)?(?:the\s+)?\w+\s+(?:courses|of\s+them))?`)
)

func SubOr(s string, m []int, i int) string {
	if m[i] < 0 {
		return ""
	}
	return s[m[i]:m[i+1]]
}

// SpreadSlashGrade gives every code of "A/ B/ C (65%)" the grade.
func SpreadSlashGrade(m string) string {
	g := SlashGradeRe.FindStringSubmatch(m)
	var out []string
	for _, c := range append(grammar.CodeRe.FindAllString(g[1], -1), g[2]) {
		out = append(out, c+" ("+g[3]+"%)")
	}
	return strings.Join(out, "/ ")
}

// CourseLevel reads a code's level: CSC148H1 -> 100, MATB41H3 -> 200.
func CourseLevel(code string) int {
	switch code[3] {
	case '1', 'A':
		return 100
	case '2', 'B':
		return 200
	case '3', 'C':
		return 300
	case '4', 'D':
		return 400
	}
	return 0
}

// WithAverage adds an average of grade over the courses of n.
func WithAverage(n *types.ReqNode, grade int) *types.ReqNode {
	avg := AverageOver([]*types.ReqNode{n}, grade)
	if avg == nil {
		return AndNode(n, &types.ReqNode{Type: types.NodeText, Kind: types.TextOther, Text: "an average of at least " + strconv.Itoa(grade) + "% in these courses"})
	}
	return AndNode(n, avg)
}

// AverageOver builds an average of grade over the courses nodes require, or
// nil if one average can't say which count. Several parts average the credits
// each needs; a cap limits which courses count but adds none.
func AverageOver(nodes []*types.ReqNode, grade int) *types.ReqNode {
	var fixed []string
	var items []*types.ReqNode
	var add func(n *types.ReqNode) bool
	add = func(n *types.ReqNode) bool {
		if n == nil {
			return true
		}
		switch n.Type {
		case types.NodeCourse:
			fixed = append(fixed, n.Code)
		case types.NodeAnd:
			for _, it := range n.Items {
				if !add(it) {
					return false
				}
			}
		case types.NodeCredits:
			if n.Amount == 0 && n.MaxAmount != nil {
				return true
			}
			items = append(items, &types.ReqNode{Type: types.NodeCredits, Amount: n.Amount, Courses: n.Courses,
				Designators: n.Designators, Levels: n.Levels, Breadth: n.Breadth, Exclude: n.Exclude})
		case types.NodeOr, types.NodeChoose:
			f := alternativesItem(n)
			if f == nil {
				return false
			}
			items = append(items, f)
		case types.NodeAverage:
		default:
			return false
		}
		return true
	}
	for _, n := range nodes {
		if !add(n) {
			return nil
		}
	}
	if fixed = grammar.Dedupe(fixed); fixed != nil {
		total := 0.0
		for _, c := range fixed {
			total += grammar.CourseWeight(c)
		}
		items = append([]*types.ReqNode{{Type: types.NodeCredits, Courses: fixed, Amount: total}}, items...)
	}
	switch {
	case len(items) == 0:
		return nil
	case len(items) == 1 && items[0].Type == types.NodeCredits:
		it := items[0]
		avg := &types.ReqNode{Type: types.NodeAverage, MinGrade: grade, Courses: it.Courses,
			Designators: it.Designators, Levels: it.Levels, Breadth: it.Breadth, Exclude: it.Exclude}
		total := 0.0
		for _, c := range it.Courses {
			total += grammar.CourseWeight(c)
		}
		// all the listed courses count unless fewer credits meet the part
		if len(it.Designators) > 0 || len(it.Levels) > 0 || len(it.Breadth) > 0 || it.Amount < total {
			avg.Amount = it.Amount
		}
		return avg
	}
	avg := &types.ReqNode{Type: types.NodeAverage, MinGrade: grade, Items: items}
	for _, it := range items {
		avg.Amount += MinWeight(it)
	}
	return avg
}

// alternativesItem is the average item for a list of alternatives: one filter
// over single courses, or one per group, so a choice can't mix half a group
// with half another.
func alternativesItem(n *types.ReqNode) *types.ReqNode {
	single := !slices.ContainsFunc(n.Items, func(it *types.ReqNode) bool {
		return it.Type != types.NodeCourse && !(it.Type == types.NodeText && it.Kind == types.TextAmbiguity)
	})
	if single {
		f := alternativesFilter(n)
		if f != nil {
			f.Amount = MinWeight(n)
		}
		return f
	}
	alt := &types.ReqNode{Type: n.Type, Count: n.Count}
	for _, it := range n.Items {
		if it.Type == types.NodeText && it.Kind == types.TextAmbiguity {
			continue
		}
		f := alternativesFilter(it)
		if f == nil {
			return nil
		}
		f.Amount = MinWeight(it)
		alt.Items = append(alt.Items, f)
	}
	if len(alt.Items) == 1 {
		return alt.Items[0]
	}
	return alt
}

// alternativesFilter is one filter for a list of alternatives: its courses and
// its plain pools' designators. It returns nil for a pool with its own filter
// or a node that isn't a course or a pool.
func alternativesFilter(n *types.ReqNode) *types.ReqNode {
	f := &types.ReqNode{Type: types.NodeCredits}
	var walk func(n *types.ReqNode) bool
	walk = func(n *types.ReqNode) bool {
		switch n.Type {
		case types.NodeCourse:
			f.Courses = append(f.Courses, n.Code)
		case types.NodeCredits:
			if ownFilter(n) {
				return false
			}
			f.Courses = append(f.Courses, n.Courses...)
			f.Designators = append(f.Designators, n.Designators...)
		case types.NodeAnd, types.NodeOr, types.NodeChoose:
			for _, it := range n.Items {
				if !walk(it) {
					return false
				}
			}
		case types.NodeText:
			return n.Kind == types.TextAmbiguity // marks a branch; adds no courses
		default:
			return false
		}
		return true
	}
	if !walk(n) {
		return nil
	}
	f.Courses, f.Designators = grammar.Dedupe(f.Courses), grammar.Dedupe(f.Designators)
	return f
}

// ownFilter reports whether a credit pool filters by more than courses and
// designators, so it can't merge into another pool's filter.
func ownFilter(c *types.ReqNode) bool {
	return len(c.Levels) > 0 || len(c.Breadth) > 0 || len(c.Exclude) > 0
}

// MinWeight is the fewest credits that can meet n, for enrolment. Text, a
// program or a GPA counts 0, so an or with a text alternative costs nothing;
// completion's nodeCredits instead skips text alternatives.
func MinWeight(n *types.ReqNode) float64 {
	if n == nil {
		return 0
	}
	switch n.Type {
	case types.NodeCourse:
		return grammar.CourseWeight(n.Code)
	case types.NodeCredits, types.NodeAverage:
		return n.Amount
	case types.NodeAnd:
		sum := 0.0
		for _, it := range n.Items {
			sum += MinWeight(it)
		}
		return sum
	case types.NodeOr, types.NodeChoose:
		var ws []float64
		for _, it := range n.Items {
			ws = append(ws, MinWeight(it))
		}
		k := 1
		if n.Type == types.NodeChoose {
			k = n.Count
		}
		return SumCheapest(ws, k)
	}
	return 0
}

// SumCheapest returns the sum of the k smallest of ws, or of all when fewer.
// It sorts ws.
func SumCheapest(ws []float64, k int) float64 {
	slices.Sort(ws)
	sum := 0.0
	for _, w := range ws[:max(0, min(k, len(ws)))] {
		sum += w
	}
	return sum
}
