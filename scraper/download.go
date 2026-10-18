package scraper

import (
	"bytes"
	"context"
	"degree-explorer/utils"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type PageType string

const (
	PROGRAMS PageType = "programs"
	COURSES  PageType = "courses"
)

type PageData struct {
	n     int
	pType PageType
	data  []byte
}

const (
	CoursesURL  = "https://artsci.calendar.utoronto.ca/search-courses?page="
	ProgramsURL = "https://artsci.calendar.utoronto.ca/search-programs?page="
)

var ErrNoEntries = errors.New("no entries found on the page")

var (
	pageDelay        = time.Second
	rateLimitWait    = 30 * time.Second
	maxRateLimitWait = 10 * time.Minute
)

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func pageHasEntries(ctx context.Context, url string) ([]byte, error) {
	page, err := utils.GetPage(ctx, url)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(page))
	if err != nil {
		return nil, err
	}

	entries := doc.Find(".views-row")
	if entries.Length() == 0 {
		return nil, ErrNoEntries
	}

	return page, nil
}

// rateLimit reports whether err is a rate-limit response (429, or 503 with
// Retry-After) and the wait the server asked for (0 if none).
func rateLimit(err error) (time.Duration, bool) {
	var he *utils.HTTPError
	if !errors.As(err, &he) {
		return 0, false
	}
	switch {
	case he.StatusCode == http.StatusTooManyRequests:
		return he.RetryAfter, true
	case he.StatusCode == http.StatusServiceUnavailable && he.RetryAfter > 0:
		return he.RetryAfter, true
	}
	return 0, false
}

// dispatchScraper sends search pages from page 0 until one has no entries.
// A rate-limited page is retried after a wait; any other error ends the run.
func dispatchScraper(ctx context.Context, searchURL string, pt PageType, ch chan<- *PageData) error {
	backoff := rateLimitWait
	for i := 0; ; {
		pURL := searchURL + strconv.Itoa(i)
		page, err := pageHasEntries(ctx, pURL)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, ErrNoEntries) {
			return nil
		}
		if wait, ok := rateLimit(err); ok {
			if wait == 0 {
				wait, backoff = backoff, min(2*backoff, maxRateLimitWait)
			}
			wait = min(wait, time.Hour)
			log.Printf("rate limited on %s; retrying in %s", pURL, wait)
			sleep(ctx, wait)
			continue
		}
		if err != nil {
			return fmt.Errorf("get %s: %w", pURL, err)
		}
		backoff = rateLimitWait
		select {
		case ch <- &PageData{n: i, pType: pt, data: page}:
		case <-ctx.Done():
			return ctx.Err()
		}
		sleep(ctx, pageDelay)
		i++
	}
}

func DownloadPages(ctx context.Context, cfg *utils.Config) error {
	return downloadPages(ctx, cfg, map[PageType]string{COURSES: CoursesURL, PROGRAMS: ProgramsURL})
}

func downloadPages(ctx context.Context, cfg *utils.Config, sources map[PageType]string) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	ch := make(chan *PageData)
	var scrapers sync.WaitGroup
	for pt, url := range sources {
		scrapers.Go(func() {
			if err := dispatchScraper(ctx, url, pt, ch); err != nil {
				cancel(err)
			}
		})
	}
	go func() {
		scrapers.Wait()
		close(ch)
	}()

	for page := range ch {
		fp := filepath.Join(cfg.TempDir, string(page.pType), fmt.Sprintf("page-%d.html", page.n))
		if err := os.WriteFile(fp, page.data, 0644); err != nil {
			cancel(fmt.Errorf("write %s: %w", fp, err))
		}
	}
	return context.Cause(ctx)
}
