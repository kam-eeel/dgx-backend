package parser

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"degree-explorer/types"
)

// assertJSON compares got (marshalled) with want structurally, ignoring key order.
func assertJSON(t *testing.T, got any, want string) {
	t.Helper()
	gb, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var g, w any
	if err := json.Unmarshal(gb, &g); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("bad want JSON %s: %v", want, err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Errorf("\n got: %s\nwant: %s", gb, want)
	}
}

func jC(code string) string { return fmt.Sprintf(`{"type":"course","code":%q}`, code) }

func jCG(code string, grade int) string {
	return fmt.Sprintf(`{"type":"course","code":%q,"min_grade":%d}`, code, grade)
}

func jAnd(items ...string) string { return `{"type":"and","items":[` + strings.Join(items, ",") + `]}` }

func jOr(items ...string) string { return `{"type":"or","items":[` + strings.Join(items, ",") + `]}` }

// jT builds a text node; kind "" means "not classified yet".
func jT(kind, text string) string {
	if kind == "" {
		return fmt.Sprintf(`{"type":"text","text":%q}`, text)
	}
	return fmt.Sprintf(`{"type":"text","kind":%q,"text":%q}`, kind, text)
}

// jP builds a program node: unresolved without codes, resolved with them.
func jP(text string, codes ...string) string {
	if len(codes) == 0 {
		return fmt.Sprintf(`{"type":"program","text":%q}`, text)
	}
	b, _ := json.Marshal(codes)
	return fmt.Sprintf(`{"type":"program","programs":%s,"text":%q}`, b, text)
}

// complCompact writes c as lines: "N kind pP req" per row (pP for a parent,
// "scope:S" for a scope) with its audience and notes after it, then the named
// lists, program notes and exclusions. It leaves out labels, sections and raws.
func complCompact(c *types.Completion) []string {
	var out []string
	for _, r := range c.Requirements {
		j, _ := json.Marshal(r.Req)
		line := fmt.Sprintf("%d %s", r.Number, r.Kind)
		if r.Parent != 0 {
			line += fmt.Sprintf(" p%d", r.Parent)
		}
		if r.Scope != "" {
			line += " scope:" + string(r.Scope)
		}
		out = append(out, line+" "+string(j))
		if r.Audience != nil {
			a, _ := json.Marshal(r.Audience)
			out = append(out, "  audience: "+string(a))
		}
		for _, n := range r.Notes {
			out = append(out, "  note: "+n)
		}
	}
	for _, l := range c.Lists {
		j, _ := json.Marshal(l.Items)
		name := l.Name
		if l.Title != "" {
			name += " (" + l.Title + ")"
		}
		out = append(out, "list: "+name+": "+string(j))
	}
	for _, n := range c.Notes {
		out = append(out, "note: "+n)
	}
	for _, e := range c.Exclusions {
		j, _ := json.Marshal(e)
		out = append(out, "exclusion: "+string(j))
	}
	return out
}
