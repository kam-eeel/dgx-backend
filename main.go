package main

import (
	"context"
	"degree-explorer/courses"
	"degree-explorer/store"
	"degree-explorer/utils"
	"log"
	"os"
	"os/signal"
	"syscall"
)

const (
	GetNewCoursePages bool = true
	ParseCoursePages  bool = true
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

	if GetNewCoursePages {
		courses.GetCoursePages(cfg)
	}

	if ParseCoursePages {
		courses.ParseCoursePages(cfg, ctx, db)
	}
}
