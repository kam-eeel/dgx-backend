package jobs

import (
	"context"
	"degree-explorer/scraper"
	"degree-explorer/store"
	"degree-explorer/utils"
	"os"
)

func UpdateDB(ctx context.Context, cfg *utils.Config, db *store.Store) error {
	// make directories if not exists
	os.MkdirAll(cfg.CoursesDir, 0755)
	os.MkdirAll(cfg.ProgramsDir, 0755)

	// defer clean up data directory
	defer os.RemoveAll(cfg.DataDir)

	scraper.DownloadPages(cfg)
	if err := scraper.ParsePages(ctx, cfg, db); err != nil {
		return err
	}

	return nil
}
