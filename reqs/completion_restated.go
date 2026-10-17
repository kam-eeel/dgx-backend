package reqs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"degree-explorer/types"
)

// More binding notes (Task 10, the binding notes; completion_binding.go
// has the rest): the ones that restate what the rows encode or are advice
// in one wording are advisory (complRestatedRes, read by classifySentence
// before every other rule), and the passes bindingNotes calls from here
// encode the last binding ones.

// complRestatedRes are sentences that restate what the rows already say, or
// are advice, though a binding word or a code is in them. Unlike
// complAsideRes, "must" doesn't veto them: each is one wording.
var complRestatedRes = []*regexp.Regexp{
	// A course's overlap with others, which their exclusions encode:
	// "MGT100H1, MGT201H1, MGT230H1, MGT250H1 have considerable overlap with
	// RSM100H1, RSM219H1, RSM230H1 and RSM250H1 taught in Rotman Commerce".
	regexp.MustCompile(`^` + codePat + `(?:,\s*` + codePat + `)*\s+have\s+considerable\s+overlap\s+with\b`),
	// The claims rule (a course counts toward one allocation), restated:
	// "The course taken in lieu of STA130H1 cannot be one that is already
	// being used to satisfy Requirement 9 ...", "Please note that the 300 or
	// 400-level STA course used to replace STA130H1 cannot be a course that
	// is already being used to meet a program completion requirement",
	// "Students admitted to the program with ECO101H1/ ECO105Y1/ IRW230H1:
	// The course will be applied to Requirement #3 (Higher Years)" (only that
	// row's pool has them). "(if not taken in list 1)" is complNotBindingRes'.
	regexp.MustCompile(`(?i)\b(?:taken\s+in\s+lieu\s+of|used\s+to\s+replace)\s+` + codePat + `\s+cannot\s+be\s+(?:one|a\s+course)\s+that\s+is\s+already\s+being\s+used\s+to\s+(?:satisfy|meet)\b|^students\s+admitted\s+to\s+the\s+program\s+with\s+[^:]+:\s+the\s+course\s+will\s+be\s+applied\s+to\s+requirement\b`),
	// A pointer to the note an item's alternative comes from: "If you are
	// unable to complete STA130H1 in first year, see Note A below for
	// accepted substitutions for this requirement".
	regexp.MustCompile(`(?i)^if\s+you\s+are\s+unable\s+to\s+complete\s+` + codePat + `\b[^.]*,\s+see\s+note\s+[A-Z]\s+below\s+for\b`),
	// What a list's courses need, which the list leaves out: "PHY392H1 and
	// PHY492H1 include relevant environmental content, but prerequisites for
	// these courses cannot be used towards Group A requirements" (their
	// prerequisites aren't on Group A); a course's own prerequisites: "(note
	// that both require BIO130H1 and BIO230H1/ BIO255H1)", "CSC236H1 may be
	// taken without CSC165H1 for students who completed CSC111H1". (Timing,
	// "should be taken together with ...", is complAsideMustRe's.)
	regexp.MustCompile(`(?i)\bprerequisites\s+for\s+these\s+courses\s+cannot\s+be\s+used\s+towards\b|^note\s+that\s+(?:both|these(?:\s+courses)?|this\s+course|they|it)\s+requires?\s|^` + codePat + `\s+may\s+be\s+taken\s+without\s+` + codePat + `\s+for\s+students\s+who\b`),
	// Amounts the rows encode: "(if taking AST425Y1, the extra 0.5 credit
	// can go to point 3 below)" (half of AST425Y1 in each), "Or any of the following: ANA300Y1/ ..." (courses among the item's
	// "any courses", remarkItem's note), "Sub-total = 8.5 credits" (a running
	// sum of the items above), "In addition to the above core courses, you
	// need to take 2.5 credits of electives" (the item's lead restated),
	// "plus a disciplinary focus requiring 2.0-3.5 credits" (the header's
	// part the focus row and its program say).
	regexp.MustCompile(`(?i)^if\s+taking\s+` + codePat + `,\s+the\s+extra\s+\d+(?:\.\d+)?\s+credit\s+can\s+go\s+to\b|^or\s+any\s+of\s+the\s+following:\s*` + codePat + `(?:\s*[/,]\s*` + codePat + `)*$|^sub-?total\s*=\s*\d+(?:\.\d+)?(?:\s+or\s+\d+(?:\.\d+)?)?\s+credits?(?:\s*\([^)]*\))?$|^in\s+addition\s+to\s+the\s+above\s+core\s+courses,\s+you\s+need\s+to\s+take\s+\d+(?:\.\d+)?\s+credits?\s+of\s+electives$|^plus\s+a\s+disciplinary\s+focus\s+requiring\s+\d+(?:\.\d+)?\s*-\s*\d+(?:\.\d+)?\s+credits$`),
	// A focus's own requirements, which focusChoices reads beside the
	// focus's program node (focusPart): "Health Studies (ASFOC2270A): (2.0
	// credits) HMB342H1, ...". The lines a focus heading carries are left:
	// "Pharmacology and Biomedical Toxicology (ASFOC2270Q): (3.0 credits)
	// Immunology (3.0 Credits) - ..." (the uncoded focus's membership text
	// holds it), "... (3.0 credits) Students in the Focus in Immunology can
	// request ..." (advice).
	regexp.MustCompile(`(?i)^[^:()]{1,80}\((?-i:[A-Z]{2}FOC\d{4}[A-Z])\):\s*\(\d+(?:\.\d+)?(?:\s+or\s+\d+(?:\.\d+)?)?\s+credits\)`),
	// A designator definition, which aliasDesignators applies where a row
	// uses the designator: "ANT courses are those offered with the following
	// prefixes: ANT, ARH, INS, JAL, JAA, and JGA".
	complAliasPrefixesRe,
	// Enrolment in courses, not what counts: "Students in the Actuarial
	// Science Specialist Program who have successfully completed ACT348H1 and
	// ACT349H1 may request to enroll in the following RSM courses ..."
	// ("Enrolment in either PCL472Y1 or JPM400Y1 is limited ..." is
	// complNotBindingRes').
	regexp.MustCompile(`(?i)\bmay\s+request\s+to\s+enrol+\s+in\s+the\s+following\b`),
}

// restatedOmission drops the note of a sentence the total's omission
// encodes (omitTotal, whose raw holds it): "There is no need to replace the
// missing half-credit for program completion; however, please base your
// course choice on what you are ready to take, not on "saving" a
// half-credit" keeps only its advice, the part after the ";".
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

// complCognateMayCountRe: "Up to 2.0 credits in identified or preapproved
// cognate courses may count (i.e. courses offered by other departments
// containing a relevant Germanic component ...); please inquire at the
// Department and get approved in advance". The group: the share.
var complCognateMayCountRe = regexp.MustCompile(`(?i)^up\s+to\s+(\d+(?:\.\d+)?)\s+credits?\s+in\s+identified\s+or\s+pre-?approved\s+cognate\s+courses\s+may\s+count\b.*\bapprov`)

// cognateRouteTotal reads a complCognateMayCountRe note beside a total of
// any credits whose allocations all draw on one designator's courses (their
// routes aside): as
// cognateTotal reads "of which 3.0 credits must be GER courses", the
// total's pool is the designator's courses, or any courses with the
// approval (design 3.3), or(credits{N, D}, and(credits{N}, text{permission})),
// and the rest beyond the share is a constraint under it, credits{N less the
// share, D}. The note joins the total's raw.
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

// complEnrolledExceptRe: a cap's note that lifts it for the students of
// another program: "Only 0.5 credit of APM306Y1/ MAT329Y1 may count toward
// the Minor program except for student enrolled in the Education and
// Society Minor may apply 1.0 credit of MAT329Y1 toward the Minor program".
// Groups: the credits, the course.
var complEnrolledExceptRe = regexp.MustCompile(`(?i)^except\s+for\s+students?\s+enrolled\s+in\s+the\s+.+?\s+may\s+apply\s+(\d+(?:\.\d+)?)\s+credits?\s+of\s+(` + codePat + `)\s+toward\s+the\s+minor\s+program$`)

// enrolledException reads a complEnrolledExceptRe note on a limit under a
// total its items make, that caps the course below the credits it allows:
// the students of
// that program may count the course's credits the cap trims, which no node
// can say of a program the parser can't name (a row's audience is a
// profile). The total gets the lenient reading beside it,
// or(credits{N}, and(credits{N less what the cap trims}, course,
// text{other: the note})): Review, never NotMet, for a student short only by
// that course's trimmed credits. The note joins the limit's raw.
func (a *compAssembler) enrolledException() {
	t := a.total
	if t == nil || t.r.Req == nil || t.r.Req.Type != types.NodeCredits || t.r.Req.Amount == 0 {
		return
	}
	// Only where the items make the total: a shortfall keeps its marker
	// (markIncomplete reads a total of credits only).
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

// complRepeatTopicRe: "DHU339H1 can be taken twice if the topic is
// different". The group: the course.
var complRepeatTopicRe = regexp.MustCompile(`(?i)^(` + codePat + `)\s+can\s+be\s+taken\s+twice\s+if\s+the\s+topic\s+is\s+different$`)

// repeatTopic reads a complRepeatTopicRe note (the program's or a row's): a
// special-topics course that
// counts twice with a different topic, which the transcript's codes can't
// tell (left to the app, future-todo). It is the text alternative, of kind
// content, of the one allocation whose pool holds the course (its courses
// or its lists'): Review rather than a rule hidden in a note.
func (a *compAssembler) repeatTopic(lists []*types.CourseList) {
	byName := map[string]*types.CourseList{}
	for _, l := range lists {
		byName[l.Name] = l
	}
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
