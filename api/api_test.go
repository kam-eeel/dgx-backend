package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"degree-explorer/store"
)

func TestLimitKey(t *testing.T) {
	tests := []struct{ ip, want string }{
		{"203.0.113.7", "203.0.113.7"},
		{"::ffff:203.0.113.7", "::ffff:203.0.113.7"},
		{"2001:db8:1:2:aaaa::1", "2001:db8:1:2::/64"},
		{"2001:db8:1:2:bbbb::9", "2001:db8:1:2::/64"},
		{"not an ip", "not an ip"},
	}
	for _, tt := range tests {
		if got := limitKey(tt.ip); got != tt.want {
			t.Errorf("limitKey(%q) = %q, want %q", tt.ip, got, tt.want)
		}
	}
}

func TestWriteLookupError(t *testing.T) {
	tests := []struct {
		err      error
		code     int
		wantBody string
	}{
		{fmt.Errorf("get course X: %w", store.ErrNotFound), http.StatusNotFound, `"get course X: not found"`},
		{errors.New("get course X: database is locked"), http.StatusInternalServerError, `"internal server error"`},
	}
	for _, tt := range tests {
		w := httptest.NewRecorder()
		writeLookupError(w, tt.err)
		if w.Code != tt.code || !strings.Contains(w.Body.String(), tt.wantBody) {
			t.Errorf("%v: got %d %s", tt.err, w.Code, w.Body)
		}
	}
}

func TestWithHeaders(t *testing.T) {
	h := withHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			w.Header().Set("Cache-Control", cacheData)
		}
	}))
	for path, want := range map[string]string{"/ok": cacheData, "/other": "no-store"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if got := w.Header().Get("Cache-Control"); got != want {
			t.Errorf("%s: Cache-Control = %q, want %q", path, got, want)
		}
		if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("%s: X-Content-Type-Options = %q", path, got)
		}
	}
}
