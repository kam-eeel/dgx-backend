package main

import (
	"context"
	"degree-explorer/api"
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

	// start api server
	serveErr := make(chan error, 1)
	go func() { serveErr <- api.Serve(ctx, cfg, db) }()

	// block until server exit (signal-triggered or app shutdown)
	if err := <-serveErr; err != nil {
		log.Fatalf("server: %v", err)
	}
	log.Println("shutdown complete")

	// scraper.DownloadPages(cfg)
	// if err := scraper.ParsePages(ctx, cfg, db); err != nil {
	// 	log.Fatal(err)
	// }
}
