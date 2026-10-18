package api

import (
	"context"
	"degree-explorer/store"
	"degree-explorer/utils"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"time"
)

type Status struct {
	Status string    `json:"status"`
	Time   time.Time `json:"time"`
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	w.WriteHeader(code)
	if err := enc.Encode(v); err != nil {
		log.Printf("encode error: %v", err)
	}
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func clientIP(r *http.Request) string {
	if ip := net.ParseIP(r.Header.Get("CF-Connecting-IP")); ip != nil {
		return ip.String()
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// GET /health -> JSON
func healthHandler(cfg *utils.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, Status{Status: cfg.Status(), Time: time.Now().UTC()})
	}
}

// GET /course/{id} -> JSON
func courseHandler(db *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		searchCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		c_id := r.PathValue("id")
		course, err := db.GetCourse(searchCtx, c_id)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, course)
	}
}

// GET /program/{id} -> JSON
func programHandler(db *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		searchCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		p_id := r.PathValue("id")
		course, err := db.GetProgram(searchCtx, p_id)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, course)
	}
}

// GET /db -> file download
func dbHandler(snapshotPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="dgx.db"`)
		http.ServeFile(w, r, snapshotPath)
	}
}

// blocks until ctx is cancelled, then graceful shutdown - or server fails.
func Serve(ctx context.Context, cfg *utils.Config, db *store.Store) error {
	addr := cfg.Host + ":" + cfg.Port

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", healthHandler(cfg))
	mux.HandleFunc("GET /course/{id}", courseHandler(db))
	mux.HandleFunc("GET /program/{id}", programHandler(db))
	mux.HandleFunc("GET /db", dbHandler(cfg.SnapshotPath))

	// initialize rate limiter at 5 req/s per IP, bursts up to 5
	limiter := newRateLimiter(ctx, 5, 5)

	srv := &http.Server{
		Addr:              addr,
		Handler:           logRequests(limiter.middleware(mux)),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// buffered so goroutine can always exit, even if no reads
	errCh := make(chan error, 1)
	go func() {
		log.Printf("listening on %s", addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		// failed to start or crashed
		return err
	case <-ctx.Done():
	}

	// stop accepting new connections; give in-progress requests time to finish
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
