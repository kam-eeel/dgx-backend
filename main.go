package main

import (
	"context"
	"degree-explorer/api"
	"degree-explorer/jobs"
	"degree-explorer/store"
	"degree-explorer/utils"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/robfig/cron/v3"
)

// cron schedule that fires once, at scheduler start
type once struct{ done bool }

func (o *once) Next(t time.Time) time.Time {
	if o.done {
		return time.Time{}
	}
	o.done = true
	return t.Add(time.Second)
}

func run() error {
	cfg := utils.LoadConfig()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// initialize db connection
	db, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	// check for db snapshot (for GET /db) and create if not exists
	if _, err := os.Stat(cfg.SnapshotPath); os.IsNotExist(err) {
		if err := db.Snapshot(ctx, cfg.SnapshotPath); err != nil {
			return err
		}
	}

	// register db update job (checks for updates daily; runs only once for each new school year)
	loc, err := time.LoadLocation(cfg.TZ)
	if err != nil {
		return err
	}
	cronLogger := cron.VerbosePrintfLogger(log.New(os.Stdout, "cron: ", log.LstdFlags))
	c := cron.New(
		cron.WithLocation(loc),
		cron.WithChain(
			cron.Recover(cronLogger),
			cron.SkipIfStillRunning(cronLogger),
		),
	)
	update := jobs.UpdateDB(ctx, cfg, db, loc)
	if _, err := c.AddFunc(cfg.UpdateSchedule, update); err != nil {
		return err
	}

	// also run at startup, to recover a missed or failed update
	c.Schedule(&once{}, cron.FuncJob(update))

	c.Start()

	// start api server
	serveErr := make(chan error, 1)
	go func() { serveErr <- api.Serve(ctx, cfg, db) }()

	// block until server exit (signal-triggered or server failure)
	var runErr error
	select {
	case <-ctx.Done():
		log.Println("shutdown signal received; exiting...")
		// wait for server to finish in-flight requests
		if err := <-serveErr; err != nil {
			log.Printf("server shutdown: %v", err)
		}
	case err := <-serveErr:
		if err != nil {
			runErr = fmt.Errorf("server: %v", err)
		}
	}
	// cancel ctx for jobs even if exiting via server error
	stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	select {
	case <-c.Stop().Done(): // stop scheduling jobs; let running jobs finish
	case <-shutdownCtx.Done():
		log.Println("timed out waiting for cron jobs")
	}

	log.Println("shutdown complete")
	return runErr
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
