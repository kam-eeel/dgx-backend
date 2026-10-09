package main

import (
	"context"
	"degree-explorer/scraper"
	"degree-explorer/store"
	"degree-explorer/utils"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	cfg := utils.LoadConfig()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// initialize db connection
	db, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// scraper.DownloadPages(cfg)
	scraper.ParsePages(ctx, cfg, db)
}
