package courses

import (
	"regexp"
	"slices"
	"strings"

	"degree-explorer/parser/grammar"
	"degree-explorer/types"
)

var (
	// notConcurrentRe matches a timing restriction: "May not be taken concurrently
	// with FSL312H1".
	notConcurrentRe = regexp.MustCompile(`(?i)\b(?:(?:may|must|should)\s+not|cannot|can\s+not)\s+be\s+taken\s+concurrently\s+with\s+(.+?)\.?$`)
	codeListRe      = regexp.MustCompile(`^` + grammar.CodePat + `(?:\s*(?:,|/|(?i:and|or))\s*` + grammar.CodePat + `)*$`)
)

func RepairRequisite(q *types.Requisite, known map[string]bool) *types.Requisite {
	if q == nil {
		return nil
	}
	fixed, ok := grammar.RepairCatalogCodes(q.Raw, known)
	if !ok {
		return q
	}
	r := grammar.ParseRequisite(fixed)
	r.Raw = q.Raw
	return r
}

func RepairExclusions(e *types.Exclusions, known map[string]bool) *types.Exclusions {
	if e == nil {
		return nil
	}
	fixed, ok := grammar.RepairCatalogCodes(e.Raw, known)
	if !ok {
		return e
	}
	r := ParseExclusions(fixed)
	r.Raw = e.Raw
	return r
}

// timingItems reads a "may not be taken concurrently with A, B" sentence as
// concurrent exclusion items, one per course. It returns nil for anything
// else, including a list that isn't only course codes.
func timingItems(sentence string) []types.ExclusionItem {
	m := notConcurrentRe.FindStringSubmatch(sentence)
	if m == nil || !codeListRe.MatchString(strings.TrimSpace(m[1])) {
		return nil
	}
	var items []types.ExclusionItem
	for _, c := range grammar.CodeRe.FindAllString(m[1], -1) {
		items = append(items, types.ExclusionItem{Courses: []string{c}, Concurrent: true})
	}
	return items
}

// ApplyTiming adds the timing restrictions found in the requisites' notes to
// the exclusions, creating them if the field was empty.
func ApplyTiming(e *types.Exclusions, qs ...*types.Requisite) *types.Exclusions {
	for _, q := range qs {
		if q == nil {
			continue
		}
		for _, note := range q.Notes {
			items := timingItems(note)
			if items == nil {
				continue
			}
			if e == nil {
				e = &types.Exclusions{Items: []types.ExclusionItem{}}
			}
			for _, it := range items {
				if !slices.ContainsFunc(e.Items, func(x types.ExclusionItem) bool { return itemKey(x) == itemKey(it) }) {
					e.Items = append(e.Items, it)
				}
			}
			if !slices.Contains(e.Notes, note) {
				e.Notes = append(e.Notes, note)
			}
		}
	}
	return e
}
