package scraper

import (
	"context"
	"degree-explorer/utils"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	entriesPage = `<html><body><div class="views-row">entry</div></body></html>`
	emptyPage   = `<html><body><p>No results</p></body></html>`
)

// fakeCalendar serves pages 0..last with entries and an empty page after.
// fail, if set, answers page requests before the normal page; it returns false
// to fall through.
func fakeCalendar(t *testing.T, last int, fail func(w http.ResponseWriter, page string) bool) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		if fail != nil && fail(w, page) {
			return
		}
		n := 0
		for _, c := range page {
			n = n*10 + int(c-'0')
		}
		if n <= last {
			w.Write([]byte(entriesPage))
			return
		}
		w.Write([]byte(emptyPage))
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/search?page="
}

func tempConfig(t *testing.T) *utils.Config {
	t.Helper()
	dir := t.TempDir()
	for _, pt := range []PageType{COURSES, PROGRAMS} {
		if err := os.MkdirAll(filepath.Join(dir, string(pt)), 0755); err != nil {
			t.Fatal(err)
		}
	}
	return &utils.Config{TempDir: dir}
}

func fastWaits(t *testing.T) {
	t.Helper()
	pd, rw, mw := pageDelay, rateLimitWait, maxRateLimitWait
	pageDelay, rateLimitWait, maxRateLimitWait = 0, 10*time.Millisecond, 20*time.Millisecond
	t.Cleanup(func() { pageDelay, rateLimitWait, maxRateLimitWait = pd, rw, mw })
}

func savedPages(t *testing.T, cfg *utils.Config, pt PageType) int {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(cfg.TempDir, string(pt), "*.html"))
	if err != nil {
		t.Fatal(err)
	}
	return len(files)
}

func TestDownloadPagesUntilEmptyPage(t *testing.T) {
	fastWaits(t)
	cfg := tempConfig(t)
	err := downloadPages(context.Background(), cfg, map[PageType]string{
		COURSES:  fakeCalendar(t, 2, nil),
		PROGRAMS: fakeCalendar(t, 0, nil),
	})
	if err != nil {
		t.Fatalf("downloadPages: %v", err)
	}
	if c, p := savedPages(t, cfg, COURSES), savedPages(t, cfg, PROGRAMS); c != 3 || p != 1 {
		t.Errorf("saved %d course and %d program pages, want 3 and 1", c, p)
	}
}

func TestDownloadPagesWaitsWhenRateLimited(t *testing.T) {
	tests := []struct {
		name   string
		status int
		header string
	}{
		{"429 without Retry-After", http.StatusTooManyRequests, ""},
		{"429 with Retry-After", http.StatusTooManyRequests, "0"},
		{"503 with Retry-After", http.StatusServiceUnavailable, "1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fastWaits(t)
			var limited atomic.Int32
			url := fakeCalendar(t, 2, func(w http.ResponseWriter, page string) bool {
				if page == "1" && limited.Add(1) <= 2 {
					if tt.header != "" {
						w.Header().Set("Retry-After", tt.header)
					}
					w.WriteHeader(tt.status)
					return true
				}
				return false
			})
			cfg := tempConfig(t)
			if err := downloadPages(context.Background(), cfg, map[PageType]string{COURSES: url}); err != nil {
				t.Fatalf("downloadPages: %v", err)
			}
			if n := savedPages(t, cfg, COURSES); n != 3 {
				t.Errorf("saved %d pages, want 3", n)
			}
		})
	}
}

func TestDownloadPagesStopsOnError(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   string
	}{
		{"server error", http.StatusInternalServerError, "http status 500"},
		{"blocked", http.StatusForbidden, "http status 403"},
		{"503 without Retry-After", http.StatusServiceUnavailable, "http status 503"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fastWaits(t)
			courses := fakeCalendar(t, 5, func(w http.ResponseWriter, page string) bool {
				if page == "2" {
					w.WriteHeader(tt.status)
					return true
				}
				return false
			})
			// the other scraper would run long; the error must cancel it
			programs := fakeCalendar(t, 1<<30, nil)
			err := downloadPages(context.Background(), tempConfig(t), map[PageType]string{COURSES: courses, PROGRAMS: programs})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("downloadPages error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestDownloadPagesStopsOnWriteError(t *testing.T) {
	fastWaits(t)
	cfg := &utils.Config{TempDir: filepath.Join(t.TempDir(), "missing")}
	err := downloadPages(context.Background(), cfg, map[PageType]string{COURSES: fakeCalendar(t, 2, nil)})
	if err == nil || !strings.Contains(err.Error(), "write ") {
		t.Fatalf("downloadPages error = %v, want a write error", err)
	}
}

func TestDownloadPagesCancelled(t *testing.T) {
	fastWaits(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := downloadPages(ctx, tempConfig(t), map[PageType]string{COURSES: fakeCalendar(t, 2, nil)})
	if err != context.Canceled {
		t.Fatalf("downloadPages error = %v, want context.Canceled", err)
	}
}
