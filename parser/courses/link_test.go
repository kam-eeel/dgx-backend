package courses

import (
	"testing"
)

func TestTimingExclusions(t *testing.T) {
	e := ParseExclusions("FSL321Y1. May not be taken concurrently with FSL312H1, FSL313H1.")
	assertJSON(t, e.Items, `[{"courses":["FSL321Y1"]},{"courses":["FSL312H1"],"concurrent":true},{"courses":["FSL313H1"],"concurrent":true}]`)
	if e.Notes != nil {
		t.Errorf("notes = %q, want none", e.Notes)
	}
	if timingItems("May not be taken concurrently with another research project") != nil {
		t.Error("a list that isn't course codes became items")
	}
}
