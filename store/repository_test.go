package store

import (
	"context"
	"degree-explorer/types"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestReplaceSessions(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var courses []*types.Course
	for _, code := range []string{"CSC108H1", "ABP100Y1", "MAT137Y1"} {
		courses = append(courses, &types.Course{Code: code, Designator: code[:3], Number: code[3:6], Level: 100, Weight: 0.5, Campus: "1"})
	}
	if err := db.CreateCourses(ctx, courses); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	if err := db.SetVersion(ctx, "2026-2027", start); err != nil {
		t.Fatal(err)
	}

	csc := types.Sessions{FallWinter: types.StringList{"F", "S"}}
	abp := types.Sessions{FallWinter: types.StringList{"Y"}}
	replace := func(offerings map[string]types.Sessions, now time.Time, want int64) {
		t.Helper()
		got, err := db.ReplaceSessions(ctx, offerings, now)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("ReplaceSessions changed %d; want %d", got, want)
		}
	}
	check := func(code string, want *types.Sessions) {
		t.Helper()
		c, err := db.GetCourse(ctx, code)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(c.Sessions, want) {
			t.Errorf("%s sessions = %+v; want %+v", code, c.Sessions, want)
		}
	}
	lastUpdated := func() string {
		t.Helper()
		var s string
		if err := db.db.QueryRowContext(ctx, `SELECT last_updated FROM version`).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}

	// Codes not in courses are ignored.
	first := start.Add(24 * time.Hour)
	replace(map[string]types.Sessions{"CSC108H1": csc, "ABP100Y1": abp, "ZZZ999H1": abp}, first, 2)
	check("CSC108H1", &csc)
	check("ABP100Y1", &abp)
	check("MAT137Y1", nil)
	if got := lastUpdated(); got != first.Format(time.RFC3339) {
		t.Errorf("last_updated = %s; want %s", got, first.Format(time.RFC3339))
	}

	// The same data writes nothing and leaves last_updated alone.
	replace(map[string]types.Sessions{"CSC108H1": csc, "ABP100Y1": abp}, first.Add(24*time.Hour), 0)
	if got := lastUpdated(); got != first.Format(time.RFC3339) {
		t.Errorf("unchanged run set last_updated = %s", got)
	}

	// A course dropped from offerings is cleared; one changed is rewritten.
	summer := types.Sessions{Summer: types.StringList{"Y"}}
	replace(map[string]types.Sessions{"CSC108H1": summer}, first.Add(48*time.Hour), 2)
	check("CSC108H1", &summer)
	check("ABP100Y1", nil)
}
