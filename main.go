package main

import (
	"context"
	"degree-explorer/store"
	"log"
)

const (
	GetNewCoursePages bool = false
)

func main() {
	cfg := LoadConfig()

	db, err := store.Open(context.Background(), cfg.DBPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if GetNewCoursePages {
		GetCoursePages(cfg)
	}

	ParseCoursePages(cfg)
}
