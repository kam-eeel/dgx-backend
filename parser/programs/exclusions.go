package programs

import (
	"regexp"
	"strings"

	"degree-explorer/parser/grammar"
	"degree-explorer/types"
)

// Sentences naming programs a student must not be in ("cannot be combined
// with ..."), and the program phrases they list.

var (
	// KindRe matches "This is a limited enrolment Certificate."
	KindRe = regexp.MustCompile(`(?i)\b(?:this\s+is\s+an?|is\s+an?)\s+(open|limited)\s+(?:enrol(?:l)?ment\s+)?(?:program|certificate)\b(.*)$`)

	// Programs that can't be combined with this one, and the phrases naming them.
	NotOpenRe      = regexp.MustCompile(`(?i)\bnot\s+open\s+to\s+students\s+enrolled\s+in\s+(?:a\s+|an\s+)?(.+)$`)
	CombinedWithRe = regexp.MustCompile(`(?i)\bcannot\s+be\s+combined\s+with(?:\s+the)?\s*:?\s*(.*)$`)
	CombineRe      = regexp.MustCompile(`(?i)\bcannot\s+combine\s+(?:a|an|the)\s+.+?\s+with\s+(.+?)(?:\s+for\s+their\s+degree)?$`)
	SimultaneousRe = regexp.MustCompile(`(?i)\bcannot\s+be\s+simultaneously\s+enrolled\s+in\s+(?:or\s+complete\s+)?(.+)$`)
	// enrolProgTypesParenRe matches "another Sociology program (Specialist or
	// Minor)", read as "Sociology Specialist or Minor".
	enrolProgTypesParenRe = regexp.MustCompile(`(?i)^(?:another|any\s+other)\s+(.+?)\s+program\s+\(((?:Specialist|Major|Minor)(?:\s+or\s+(?:Specialist|Major|Minor))*)\)$`)
	enrolDeptParenRe      = regexp.MustCompile(`(?i)^(?:either\s+)?departmental\s+(Major|Specialist|Minor)\s+programs?\s+\((.+)\)$`)
	enrolProgSplitRe      = regexp.MustCompile(`(?i)\s*,?\s+or\s+the\s+|\s*,\s+the\s+|\s+or\s+(?:the\s+)?(?:Environmental|Biomedical)\b`)
	enrolProgArticleRe    = regexp.MustCompile(`(?i)^(?:the|a|an|either|any)\s+`)
	enrolTypeOnlyRe       = regexp.MustCompile(`^(?i:specialist|major|minor)$`)
	enrolAltSepRe         = regexp.MustCompile(`\s*,\s*(?:or\s+)?|\s+or\s+`)
	// exclusionIncludingRe drops what "including" lists. enrolSubjectOnlyRe
	// matches a subject or department, not named programs: "any Minor, Major, or
	// Specialist offered by the Department of French".
	exclusionIncludingRe = regexp.MustCompile(`(?i),\s+including\s+[^;]*`)
	exclusionPartsRe     = regexp.MustCompile(`\s*;\s*(?:nor\s+)?|\s+nor\s+`)
	enrolSubjectOnlyRe   = regexp.MustCompile(`(?i)^other\b|\boffered\s+by\b|\bdepartment\b`)
)

// exclusionNodes reads a sentence naming programs the student must not be
// enrolled in: "cannot be combined with ...". ok reports whether s is one;
// lead, that it only introduces a list ("cannot be combined with the:").
func ExclusionNodes(s string) (nodes []*types.ReqNode, ok, lead bool) {
	for _, re := range []*regexp.Regexp{NotOpenRe, CombinedWithRe, CombineRe, SimultaneousRe} {
		m := re.FindStringSubmatch(s)
		if m == nil {
			continue
		}
		phrase := strings.TrimSpace(strings.TrimRight(m[1], ".:"))
		if phrase == "" {
			return nil, true, true
		}
		return ExclusionPhraseNodes(phrase), true, false
	}
	return nil, false, false
}

// ExclusionPhraseNodes returns the programs named in phrase, one node per
// explicit alternative. A phrase about a subject or department stays
// membership text.
func ExclusionPhraseNodes(phrase string) []*types.ReqNode {
	var out []*types.ReqNode
	phrase = enrolProgArticleRe.ReplaceAllString(phrase, "")
	// each ";" or "nor" part on its own; what "including" lists is already in
	// the part before it
	phrase = exclusionIncludingRe.ReplaceAllString(phrase, "")
	if parts := exclusionPartsRe.Split(phrase, -1); len(parts) > 1 {
		for _, part := range parts {
			out = append(out, ExclusionPhraseNodes(part)...)
		}
		return out
	}
	if m := enrolDeptParenRe.FindStringSubmatch(phrase); m != nil {
		for _, name := range enrolAltSepRe.Split(m[2], -1) {
			out = append(out, &types.ReqNode{Type: types.NodeProgram, Text: name + " " + m[1]})
		}
		return out
	}
	for _, piece := range enrolProgSplitRe.Split(phrase, -1) {
		piece = enrolProgArticleRe.ReplaceAllString(strings.TrimSpace(piece), "")
		if m := enrolProgTypesParenRe.FindStringSubmatch(piece); m != nil {
			out = append(out, &types.ReqNode{Type: types.NodeProgram, Text: m[1] + " " + m[2]})
			continue
		}
		_, ok := grammar.ParseProgramRef(piece)
		if enrolSubjectOnlyRe.MatchString(piece) || !ok && !strings.Contains(piece, "(") {
			// a subject or a department, not a list the program grammar can read: Link
			// resolves it by subject area, or leaves it as text
			out = append(out, &types.ReqNode{Type: types.NodeProgram, Text: piece})
			continue
		}
		out = append(out, ProgramPhraseNode(piece))
	}
	return out
}

// ProgramPhraseNode turns a phrase naming programs into program nodes: codes
// go into Programs, names are left for Link. A list mixing subjects with bare
// types splits into one node per subject, OR'd.
func ProgramPhraseNode(phrase string) *types.ReqNode {
	phrase = strings.TrimSpace(strings.TrimRight(phrase, ".,"))
	phrase = grammar.ProgSuffixRe.ReplaceAllString(phrase, "")
	if codes := grammar.ProgCodeRe.FindAllString(phrase, -1); codes != nil {
		return &types.ReqNode{Type: types.NodeProgram, Programs: grammar.Dedupe(codes), Text: phrase}
	}
	if _, ok := grammar.ParseProgramRef(phrase); ok || strings.Contains(phrase, "(") {
		// a bracket qualifies a title; Link resolves it by title
		return &types.ReqNode{Type: types.NodeProgram, Text: phrase}
	}
	var groups []string
	for _, piece := range enrolAltSepRe.Split(phrase, -1) {
		if enrolTypeOnlyRe.MatchString(piece) && len(groups) > 0 {
			groups[len(groups)-1] += " or " + piece
		} else {
			groups = append(groups, piece)
		}
	}
	var nodes []*types.ReqNode
	for _, g := range groups {
		nodes = append(nodes, &types.ReqNode{Type: types.NodeProgram, Text: g})
	}
	if len(nodes) == 1 {
		return nodes[0]
	}
	return &types.ReqNode{Type: types.NodeOr, Items: nodes}
}
