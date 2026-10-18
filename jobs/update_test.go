package jobs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClearDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "courses"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "courses", "page-0.html"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := clearDir(dir); err != nil {
		t.Fatalf("clearDir: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Errorf("after clearDir: %d entries, err %v; want an empty dir", len(entries), err)
	}
	if err := clearDir(filepath.Join(dir, "missing")); err != nil {
		t.Errorf("clearDir on a missing dir: %v", err)
	}
}
