package jobs

import (
	"context"
	"degree-explorer/scraper"
	"degree-explorer/store"
	"degree-explorer/utils"
	"log"
	"time"
)

// UpdateSessions refreshes which courses the timetable offers in its current
// sessions. On failure, the stored sessions are kept. The /db snapshot is
// rewritten only if any course's sessions changed.
func UpdateSessions(ctx context.Context, cfg *utils.Config, db *store.Store) func() {
	return func() {
		cfg.SetStatus("updating sessions")
		defer cfg.SetStatus("healthy")

		offerings, err := scraper.FetchOfferings(ctx)
		if err != nil {
			log.Printf("fetch offerings: %v; the next scheduled run retries", err)
			return
		}
		changed, err := db.ReplaceSessions(ctx, offerings, time.Now())
		if err != nil {
			log.Printf("db.ReplaceSessions: %v", err)
			return
		}
		if changed == 0 {
			return // keep the snapshot, so its Last-Modified stays put
		}
		log.Printf("sessions changed for %d courses", changed)

		if err := db.Snapshot(ctx, cfg.SnapshotPath); err != nil {
			log.Printf("db.Snapshot: %v", err)
		}
	}
}
