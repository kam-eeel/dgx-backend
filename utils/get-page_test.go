package utils

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRetryAfter(t *testing.T) {
	tests := []struct {
		v        string
		min, max time.Duration
	}{
		{"120", 120 * time.Second, 120 * time.Second},
		{"", 0, 0},
		{"0", 0, 0},
		{"soon", 0, 0},
		{time.Now().Add(time.Minute).UTC().Format(http.TimeFormat), 55 * time.Second, time.Minute},
		{time.Now().Add(-time.Minute).UTC().Format(http.TimeFormat), 0, 0},
	}
	for _, tt := range tests {
		if got := retryAfter(tt.v); got < tt.min || got > tt.max {
			t.Errorf("retryAfter(%q) = %s, want %s to %s", tt.v, got, tt.min, tt.max)
		}
	}
}

func TestGetPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.Write([]byte("page"))
		case "/limited":
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusTooManyRequests)
		case "/huge":
			w.Write([]byte(strings.Repeat("x", maxPageSize+1)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	ctx := context.Background()

	if page, err := GetPage(ctx, srv.URL+"/ok"); err != nil || string(page) != "page" {
		t.Errorf("ok: got %q, %v", page, err)
	}
	var he *HTTPError
	if _, err := GetPage(ctx, srv.URL+"/limited"); !errors.As(err, &he) || he.StatusCode != 429 || he.RetryAfter != 30*time.Second {
		t.Errorf("limited: got %v", err)
	}
	if _, err := GetPage(ctx, srv.URL+"/missing"); !errors.As(err, &he) || he.StatusCode != 404 {
		t.Errorf("missing: got %v", err)
	}
	if _, err := GetPage(ctx, srv.URL+"/huge"); err == nil || !strings.Contains(err.Error(), "page over") {
		t.Errorf("huge: got %v", err)
	}
}
