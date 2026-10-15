package reqs

import (
	"regexp"
	"slices"
	"strings"

	"degree-explorer/types"
)

// Audience exemptions: "Faculty of Arts and Science co-requisite and
// pre-requisite requirements do not apply to the students in the Faculty of
// Applied Science and Engineering who choose to take this course." The group
// G is exempt from the requirement R it names, so R becomes
// or(R, text{program: G}): a student who meets R is Met, anyone else gets
// Review, since the checker doesn't know a student's faculty. This is the
// shape an audience label gives with an empty branch ("<R> G: <nothing>").
// The sentence itself moves to notes.
//
// The subject says which fields the exemption covers: "pre-requisite"
// and/or "co-requisite", or neither ("These requirements ...") for the field
// it is in. Its owner ("Faculty of Arts and Science ...") is the faculty that
// sets the whole field, so R is the whole field, never a part of it.

var (
	exemptApplyRe = regexp.MustCompile(`(?i)\bdo(?:es)?\s+not\s+apply\s+to\s+`)
	// exemptSubjectRe is the last word of the subject: "... requirements",
	// "... prerequisites", "... co-requisite".
	exemptSubjectRe = regexp.MustCompile(`(?i)^(?:requirements?|(?:pre|co)-?requisites?)$`)
	exemptWordRe    = regexp.MustCompile(`^[A-Za-z&-]+$`) // no codes, no punctuation
	wordSpanRe      = regexp.MustCompile(`\S+`)
	exemptPreRe     = regexp.MustCompile(`(?i)\bpre(?:-|requisite)`)
	exemptCoRe      = regexp.MustCompile(`(?i)\bco(?:-|requisite)`)
	// exemptEndRe ends the clause: a sentence break, ";" or the end.
	exemptEndRe = regexp.MustCompile(`\.\s+[A-Z(]|\.\s*$|;`)
	// exemptTailRe is wording after the group that doesn't narrow it.
	exemptTailRe = regexp.MustCompile(`(?i)\s+who\s+(?:choose|chooses|wish|wishes|want|wants|elect|elects)\s+to\s+(?:take|enrol\s+in|enroll\s+in)\s+(?:this|the)\s+course$`)
	// exemptGroupRe: the group must name students, a faculty or FASE.
	exemptGroupRe = regexp.MustCompile(`\b(?:[Ss]tudents?|Faculty|FASE)\b`)

	audienceStopRe = regexp.MustCompile(`(?i)\b(?:for|the|students?|in|faculty|of|enrolled)\b`)
	// audienceAliases maps a group key to the group it abbreviates.
	audienceAliases = map[string]string{
		"fase":        "applied science and engineering",
		"engineering": "applied science and engineering",
	}
)

// exemptMaxWords bounds the subject, so a clause can't swallow a sentence
// that lost its full stop.
const exemptMaxWords = 10

// exemption is one "… requirements do not apply to G" clause.
type exemption struct {
	note     string // the clause as written, for notes
	group    string // G: "students in the Faculty of Applied Science and Engineering"
	pre, co  bool   // fields it names; neither means the field it is in
	from, to int    // the clause's span in the text it was found in
}

// inField reports whether the exemption certainly covers the field it was
// found in: it names no field, or both.
func (e exemption) inField() bool { return e.pre == e.co }

// findExemption returns the first exemption clause in s.
func findExemption(s string) (exemption, bool) {
	for off := 0; off < len(s); {
		loc := exemptApplyRe.FindStringIndex(s[off:])
		if loc == nil {
			break
		}
		at, groupStart := off+loc[0], off+loc[1]
		off = groupStart
		// The subject is the run of plain words before "do not apply",
		// back to punctuation, a course code or the start, and starts a
		// sentence (capitalized).
		spans := wordSpanRe.FindAllStringIndex(s[:at], -1)
		n := 0
		for n < len(spans) && n <= exemptMaxWords {
			sp := spans[len(spans)-1-n]
			if !exemptWordRe.MatchString(s[sp[0]:sp[1]]) {
				break
			}
			n++
		}
		if n == 0 || n > exemptMaxWords {
			continue
		}
		first, last := spans[len(spans)-n], spans[len(spans)-1]
		if !exemptSubjectRe.MatchString(s[last[0]:last[1]]) || !isUpperStart(s[first[0]:]) {
			continue
		}
		from, subject := first[0], s[first[0]:last[1]]
		end := len(s)
		if m := exemptEndRe.FindStringIndex(s[groupStart:]); m != nil {
			end = groupStart + m[0]
		}
		group := strings.TrimSpace(s[groupStart:end])
		group = exemptTailRe.ReplaceAllString(group, "")
		group = strings.TrimPrefix(group, "the ")
		if !exemptGroupRe.MatchString(group) || codeRe.MatchString(group) || strings.ContainsAny(group, ":()") {
			continue
		}
		return exemption{
			note:  cleanNote(s[from:end]),
			group: group,
			pre:   exemptPreRe.MatchString(subject),
			co:    exemptCoRe.MatchString(subject),
			from:  from,
			to:    end,
		}, true
	}
	return exemption{}, false
}

func isUpperStart(w string) bool { return w != "" && w[0] >= 'A' && w[0] <= 'Z' }

// extractExemptions removes exemption clauses from s, so they can't be read
// as requirements, and returns them. A clause glued to a code with its "."
// missing ("… MIE358H1 Faculty of Arts and Science … do not apply to …")
// is cut there.
func extractExemptions(s string) (string, []exemption) {
	var out []exemption
	for {
		e, ok := findExemption(s)
		if !ok {
			return s, out
		}
		out = append(out, e)
		before := strings.TrimRight(s[:e.from], " ,;.")
		after := strings.TrimLeft(s[e.to:], " ;.")
		switch {
		case before == "":
			s = after
		case after == "":
			s = before
		default:
			s = before + ". " + after
		}
	}
}

// exempt returns n with group g as an alternative, unless n already gives
// g a branch of its own (an audience label for the same group): that
// branch, not the exemption, says what g needs.
func exempt(n *types.ReqNode, g string) *types.ReqNode {
	if n == nil || hasAudience(n, audienceKey(g)) {
		return n
	}
	alt := &types.ReqNode{Type: types.NodeText, Kind: types.TextMembership, Text: g}
	if n.Type == types.NodeOr {
		n.Items = append(n.Items, alt)
		return n
	}
	return &types.ReqNode{Type: types.NodeOr, Items: []*types.ReqNode{n, alt}}
}

// hasAudience reports whether one of n's alternatives is for the group
// with the given key: the group itself, or a branch that starts with it.
func hasAudience(n *types.ReqNode, key string) bool {
	alts := []*types.ReqNode{n}
	if n.Type == types.NodeOr {
		alts = n.Items
	}
	return slices.ContainsFunc(alts, func(a *types.ReqNode) bool {
		if a.Type == types.NodeAnd && len(a.Items) > 0 {
			a = a.Items[0]
		}
		return a.Type == types.NodeText && a.Kind == types.TextMembership && audienceKey(a.Text) == key
	})
}

// audienceKey reduces an audience to its distinguishing words, so "For
// students in the Faculty of Applied Science and Engineering", "Engineering
// students" and "FASE students" compare equal.
func audienceKey(s string) string {
	k := strings.Join(strings.Fields(strings.ToLower(audienceStopRe.ReplaceAllString(s, " "))), " ")
	if a, ok := audienceAliases[k]; ok {
		return a
	}
	return k
}

// ApplyExemptions applies exemptions that one requisite field states about
// the other: an exemption in the corequisites from "Faculty of Arts and
// Science pre-requisite requirements" makes the prerequisites
// or(R, text{program: G}) and copies the sentence to their notes. Exemptions
// already applied by ParseRequisite are left alone. Either argument may be nil.
func ApplyExemptions(prereq, coreq *types.Requisite) {
	for _, src := range []*types.Requisite{prereq, coreq} {
		if src == nil {
			continue
		}
		for _, note := range slices.Clone(src.Notes) {
			e, ok := findExemption(note)
			if !ok {
				continue
			}
			for _, dst := range []*types.Requisite{prereq, coreq} {
				names := dst == prereq && e.pre || dst == coreq && e.co
				if dst == nil || dst.Req == nil || !(names || dst == src && e.inField()) {
					continue
				}
				dst.Req = exempt(dst.Req, e.group)
				if !slices.Contains(dst.Notes, e.note) {
					dst.Notes = append(dst.Notes, e.note)
				}
			}
		}
	}
}
