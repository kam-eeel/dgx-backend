package courses

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
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

func jsonString(s string) string { b, _ := json.Marshal(s); return string(b) }

func jC(code string) string { return fmt.Sprintf(`{"type":"course","code":%q}`, code) }

func jCG(code string, grade int) string {
	return fmt.Sprintf(`{"type":"course","code":%q,"min_grade":%d}`, code, grade)
}

func jAnd(items ...string) string { return `{"type":"and","items":[` + strings.Join(items, ",") + `]}` }

func jOr(items ...string) string { return `{"type":"or","items":[` + strings.Join(items, ",") + `]}` }

func jChoose(count int, items ...string) string {
	return fmt.Sprintf(`{"type":"choose","count":%d,"items":[%s]}`, count, strings.Join(items, ","))
}

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
