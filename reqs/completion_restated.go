package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// Sentences that restate the rows or are advice despite a binding word, and
// passes that encode the last binding notes.

// complRestatedRes match sentences that restate what the rows say, or are
// advice, though a binding word or a code is in them. Unlike complAsideRes,
// "must" doesn't veto them: each is one wording.
var complRestatedRes = []*regexp.Regexp{
	// a course's overlap with others, which their exclusions encode: "MGT100H1
	// have considerable overlap with RSM100H1"
	regexp.MustCompile(`^` + codePat + `(?:,\s*` + codePat + `)*\s+have\s+considerable\s+overlap\s+with\b`),
	// the claims rule restated: "The course taken in lieu of STA130H1 cannot be
	// one that is already being used to satisfy Requirement 9"
	regexp.MustCompile(`(?i)\b(?:taken\s+in\s+lieu\s+of|used\s+to\s+replace)\s+` + codePat + `\s+cannot\s+be\s+(?:one|a\s+course)\s+that\s+is\s+already\s+being\s+used\s+to\s+(?:satisfy|meet)\b|^students\s+admitted\s+to\s+the\s+program\s+with\s+[^:]+:\s+the\s+course\s+will\s+be\s+applied\s+to\s+requirement\b`),
	// a pointer to the note an alternative comes from: "If you are unable to
	// complete STA130H1 in first year, see Note A below for ..."
	regexp.MustCompile(`(?i)^if\s+you\s+are\s+unable\s+to\s+complete\s+` + codePat + `\b[^.]*,\s+see\s+note\s+[A-Z]\s+below\s+for\b`),
	// a course's prerequisites, which the list leaves out: "(note that both
	// require BIO130H1 and BIO230H1/ BIO255H1)"
	regexp.MustCompile(`(?i)\bprerequisites\s+for\s+these\s+courses\s+cannot\s+be\s+used\s+towards\b|^note\s+that\s+(?:both|these(?:\s+courses)?|this\s+course|they|it)\s+requires?\s|^` + codePat + `\s+may\s+be\s+taken\s+without\s+` + codePat + `\s+for\s+students\s+who\b`),
	// amounts the rows encode: "Sub-total = 8.5 credits"
	regexp.MustCompile(`(?i)^if\s+taking\s+` + codePat + `,\s+the\s+extra\s+` + numPat + `\s+credit\s+can\s+go\s+to\b|^or\s+any\s+of\s+the\s+following:\s*` + codePat + `(?:\s*[/,]\s*` + codePat + `)*$|^sub-?total\s*=\s*` + numPat + `(?:\s+or\s+` + numPat + `)?\s+credits?(?:\s*\([^)]*\))?$|^in\s+addition\s+to\s+the\s+above\s+core\s+courses,\s+you\s+need\s+to\s+take\s+` + numPat + `\s+credits?\s+of\s+electives$|^plus\s+a\s+disciplinary\s+focus\s+requiring\s+` + numPat + `\s*-\s*` + numPat + `\s+credits$`),
	// a focus's own requirements, which focusChoices reads: "Health Studies
	// (ASFOC2270A): (2.0 credits) HMB342H1, ..."
	regexp.MustCompile(`(?i)^[^:()]{1,80}\((?-i:[A-Z]{2}FOC\d{4}[A-Z])\):\s*\(` + numPat + `(?:\s+or\s+` + numPat + `)?\s+credits\)`),
	// a designator definition, which aliasDesignators applies
	complAliasPrefixesRe,
	// enrolment in courses, not what counts: "... may request to enroll in the
	// following RSM courses"
	regexp.MustCompile(`(?i)\bmay\s+request\s+to\s+enrol+\s+in\s+the\s+following\b`),
}

// restatedOmission drops the note of a sentence the total's omission encodes,
// keeping only the advice after its ";", if any.
func (a *compAssembler) restatedOmission() {
	t := a.total
	if t == nil || len(a.omissions) == 0 {
		return
	}
	raw := rawWords(t.r.Raw)
	for i, n := range a.notes {
		if !complNoReplaceRe.MatchString(n) || !strings.Contains(raw, rawWords(n)) {
			continue
		}
		parts := splitTop(n, ';')
		if rest := strings.TrimSpace(strings.Join(parts[1:], ";")); rest != "" && classifySentence(rest) == roleAdvisory {
			a.notes[i] = cleanNote(rest)
		} else {
			a.notes[i] = ""
		}
	}
	a.notes = slices.DeleteFunc(a.notes, func(n string) bool { return n == "" })
	if len(a.notes) == 0 {
		a.notes = nil
	}
}

// complCognateMayCountRe matches a share of approved cognate courses: "Up to
// 2.0 credits in identified or preapproved cognate courses may count ...".
var complCognateMayCountRe = regexp.MustCompile(`(?i)^up\s+to\s+` + amountPat + `\s+credits?\s+in\s+identified\s+or\s+pre-?approved\s+cognate\s+courses\s+may\s+count\b.*\bapprov`)

// cognateRouteTotal reads a complCognateMayCountRe note beside a total whose
// allocations all draw on one designator: the total becomes or(credits{N, D},
// and(credits{N}, text{permission})) with credits{N less the share, D} under it.
func (a *compAssembler) cognateRouteTotal() {
	t := a.total
	if t == nil || t.r.Req == nil {
		return
	}
	n := t.r.Req
	if n.Type != types.NodeCredits || n.Amount == 0 || !isAnyCredits(n) {
		return
	}
	i := slices.IndexFunc(a.notes, func(s string) bool { return complCognateMayCountRe.MatchString(complClean(s)) })
	if i < 0 {
		return
	}
	desig, ok := "", true
	walkRows(a.items, func(r *compRow) {
		if r.r.Kind != types.RequirementAllocation {
			return
		}
		walkReq(r.r.Req, func(x *types.ReqNode) {
			var ds []string
			switch x.Type {
			case types.NodeCourse:
				ds = []string{x.Code[:3]}
			case types.NodeCredits:
				ds = x.Designators
				for _, c := range x.Courses {
					ds = append(ds, c[:3])
				}
				ok = ok && x.Lists == nil
			}
			for _, d := range ds {
				if desig == "" {
					desig = d
				}
				ok = ok && d == desig
			}
		})
	})
	share, _ := strconv.ParseFloat(complCognateMayCountRe.FindStringSubmatch(complClean(a.notes[i]))[1], 64)
	if !ok || desig == "" || share >= n.Amount {
		return
	}
	note := a.notes[i]
	t.r.Req = orNode(
		&types.ReqNode{Type: types.NodeCredits, Amount: n.Amount, Designators: []string{desig}},
		andNode(&types.ReqNode{Type: types.NodeCredits, Amount: n.Amount}, textNode(types.TextPermission, note)))
	addRaw(t, note)
	a.floors = append(a.floors, &compRow{r: &types.Requirement{Kind: types.RequirementConstraint, Raw: note,
		Req: &types.ReqNode{Type: types.NodeCredits, Amount: n.Amount - share, Designators: []string{desig}}}})
	a.notes = slices.Delete(a.notes, i, i+1)
	if len(a.notes) == 0 {
		a.notes = nil
	}
}

// complEnrolledExceptRe matches a cap's exception for another program's
// students: "except for student enrolled in the Education and Society Minor
// may apply 1.0 credit of MAT329Y1 toward the Minor program".
var complEnrolledExceptRe = regexp.MustCompile(`(?i)^except\s+for\s+students?\s+enrolled\s+in\s+the\s+.+?\s+may\s+apply\s+` + amountPat + `\s+credits?\s+of\s+(` + codePat + `)\s+toward\s+the\s+minor\s+program$`)

// enrolledException reads a complEnrolledExceptRe note on a limit as the
// total's lenient alternative, or(credits{N}, and(credits{N less the trimmed
// credits}, course, text{other})): Review, since no node can name the program.
func (a *compAssembler) enrolledException() {
	t := a.total
	if t == nil || t.r.Req == nil || t.r.Req.Type != types.NodeCredits || t.r.Req.Amount == 0 {
		return
	}
	// only where the items make the total: a shortfall keeps its marker
	sum := 0.0
	for _, r := range a.items {
		if r.r.Kind == types.RequirementAllocation {
			v, ok := 0.0, false
			if r.r.Req != nil {
				v, ok = nodeCredits(r.r.Req)
			}
			if !ok {
				return
			}
			sum += v
		}
	}
	if sum < t.r.Req.Amount-1e-9 {
		return
	}
	for _, r := range a.allRows() {
		n := r.r.Req
		if r.r.Kind != types.RequirementLimit || n == nil || n.MaxAmount == nil {
			continue
		}
		for _, note := range r.r.Notes {
			m := complEnrolledExceptRe.FindStringSubmatch(complClean(note))
			if m == nil || !slices.Contains(n.Courses, m[2]) {
				continue
			}
			v, _ := strconv.ParseFloat(m[1], 64)
			trimmed := v - *n.MaxAmount
			if trimmed <= 0 || trimmed >= t.r.Req.Amount {
				continue
			}
			less := cloneNode(t.r.Req)
			less.Amount -= trimmed
			t.r.Req = orNode(t.r.Req, andNode(less, &types.ReqNode{Type: types.NodeCourse, Code: m[2]}, textNode(types.TextOther, note)))
			addRaw(t, note)
			addRaw(r, note)
			a.dropNoteAll(note)
			return
		}
	}
}

// complRepeatTopicRe matches a course that counts twice: "DHU339H1 can be
// taken twice if the topic is different".
var complRepeatTopicRe = regexp.MustCompile(`(?i)^(` + codePat + `)\s+can\s+be\s+taken\s+twice\s+if\s+the\s+topic\s+is\s+different$`)

// repeatTopic reads a complRepeatTopicRe note as a content text alternative
// on the one allocation whose pool holds the course: the transcript's codes
// can't tell topics apart, so it is Review.
func (a *compAssembler) repeatTopic(lists []*types.CourseList) {
	byName := listsByName(lists)
	all := slices.Clone(a.notes)
	for _, r := range a.allRows() {
		all = append(all, r.r.Notes...)
	}
	for _, note := range all {
		m := complRepeatTopicRe.FindStringSubmatch(complClean(note))
		if m == nil {
			continue
		}
		var hits []*compRow
		for _, r := range a.allRows() {
			if r.r.Kind != types.RequirementAllocation || r == a.total {
				continue
			}
			found := false
			walkReq(r.r.Req, func(x *types.ReqNode) {
				if x.Type != types.NodeCredits {
					return
				}
				found = found || slices.Contains(x.Courses, m[1])
				if codes, ok := a.listCodes(x.Lists, byName); ok {
					found = found || slices.Contains(codes, m[1])
				}
			})
			if found {
				hits = append(hits, r)
			}
		}
		if len(hits) != 1 {
			continue
		}
		hits[0].r.Req = orNode(hits[0].r.Req, textNode(types.TextContent, cleanNote(note)))
		addRaw(hits[0], note)
		a.dropNoteAll(note)
	}
}
