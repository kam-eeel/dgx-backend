package jobs

import (
	"context"
	"degree-explorer/scraper"
	"degree-explorer/store"
	"degree-explorer/utils"
	"fmt"
	"log"
	"os"
	"time"
)

// academic calendar updates in May, so use June as "start-of-year"
const yearStart = time.June

func schoolYear(t time.Time) string {
	y := t.Year()
	if t.Month() < yearStart {
		y--
	}
	return fmt.Sprintf("%d-%d", y, y+1)
}

func UpdateDB(ctx context.Context, cfg *utils.Config, db *store.Store, loc *time.Location) func() {
	return func() {
		year := schoolYear(time.Now().In(loc))
		latest, err := db.SchoolYear(ctx)
		if err != nil {
			log.Printf("db.SchoolYear: %v", err)
			return
		}
		if latest == year {
			return
		}
		log.Printf("updating db for %s", year)

		err = scrape(ctx, cfg, db)
		if rmErr := os.RemoveAll(cfg.DataDir); rmErr != nil {
			log.Printf("clean up %s: %v", cfg.DataDir, rmErr)
		}
		if err != nil {
			log.Printf("update %s: %v", year, err)
			return
		}

		if err := db.SetVersion(ctx, year, time.Now()); err != nil {
			log.Printf("db.SetVersion: %v", err)
			return
		}
		if err := db.Snapshot(ctx, cfg.SnapshotPath); err != nil {
			log.Printf("db.Snapshot: %v", err)
		}
	}
}

// scrape downloads, parses and stores the calendar pages.
func scrape(ctx context.Context, cfg *utils.Config, db *store.Store) error {
	// remove any stale pages from interrupted runs
	if err := os.RemoveAll(cfg.DataDir); err != nil {
		return err
	}
	for _, dir := range []string{cfg.CoursesDir, cfg.ProgramsDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	scraper.DownloadPages(ctx, cfg)
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("cancelled during download: %w", err)
	}
	return scraper.ParsePages(ctx, cfg, db)
}
