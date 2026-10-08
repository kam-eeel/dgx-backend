package main

import (
	"degree-explorer/scraper"
	"degree-explorer/utils"
)

const (
	GetNewCoursePages bool = false
	ParseCoursePages  bool = true
)

func main() {
	cfg := utils.LoadConfig()

	// ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	// defer stop()

	// // initialize db connection
	// db, err := store.Open(ctx, cfg.DBPath)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// defer db.Close()

	// if GetNewCoursePages {
	// 	courses.GetCoursePages(cfg)
	// }

	// if ParseCoursePages {
	// 	courses.ParseCoursePages(cfg, ctx, db)
	// }
	scraper.DownloadPages(cfg)
}
