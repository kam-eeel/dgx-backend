package scraper

import (
	"degree-explorer/types"
	"reflect"
	"testing"
)

func TestCurrentSessions(t *testing.T) {
	in := []ttbSession{
		{"Fall-Winter"}, {"20269"}, {"20271"}, {"20269-20271"},
		{"20275"}, {"20275F"}, {"20275S"}, {"20275F-20275S"},
		{"Summer"}, {"20273"}, {"20275Y"}, {"x20269"},
	}
	got := currentSessions(in)
	want := []string{"20269", "20271", "20269-20271", "20275", "20275F", "20275S", "20275F-20275S"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("currentSessions = %v; want %v", got, want)
	}
}

func TestOfferingsByCode(t *testing.T) {
	in := []ttbCourse{
		{"CSC108H1", "S", []string{"20271"}},
		{"CSC108H1", "F", []string{"20269"}},
		{"CSC108H1", "S", []string{"20271"}},
		{"CSC108H1", "F", []string{"20275F"}},
		{"ABP100Y1", "Y", []string{"20269", "20271"}},
		{"MAT137Y1", "Y", []string{"20275"}},
	}
	got := offeringsByCode(in)
	want := map[string]types.Sessions{
		"CSC108H1": {FallWinter: types.StringList{"F", "S"}, Summer: types.StringList{"F"}},
		"ABP100Y1": {FallWinter: types.StringList{"Y"}},
		"MAT137Y1": {Summer: types.StringList{"Y"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("offeringsByCode = %v; want %v", got, want)
	}
}
