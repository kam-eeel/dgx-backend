package scraper

import (
	"bytes"
	"context"
	"degree-explorer/utils"
	"errors"
	"fmt"
	"log"
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

func savePages(cfg *utils.Config, ch <-chan *PageData, wg *sync.WaitGroup) {
	defer wg.Done()
	for page := range ch {
		fp := filepath.Join(cfg.DataDir, string(page.pType), fmt.Sprintf("page-%d.html", page.n))
		if err := os.WriteFile(fp, page.data, 0644); err != nil {
			log.Printf("failed to write file %s: %v", fp, err)
		}
	}
}

func dispatchScraper(ctx context.Context, searchURL string, pt PageType, ch chan<- *PageData, wg *sync.WaitGroup) {
	defer wg.Done()
	i, retries, skipped := 0, 0, 0
	for ctx.Err() == nil {
		pURL := searchURL + strconv.Itoa(i)
		page, err := pageHasEntries(ctx, pURL)
		if ctx.Err() != nil {
			break
		} else if errors.Is(err, ErrNoEntries) {
			break
		} else if err != nil {
			retries++
			log.Printf("error getting page %s (attempt %d): %v", pURL, retries, err)
			if retries >= 3 {
				log.Printf("skipping page %s after %d attempts", pURL, retries)
				retries = 0
				skipped++
				if skipped >= 3 {
					log.Printf("giving up on %s after %d consecutive skipped pages", searchURL, skipped)
					break
				}
				i++
			}
			sleep(ctx, 5*time.Second)
			continue
		}
		retries, skipped = 0, 0
		ch <- &PageData{n: i, pType: pt, data: page}
		sleep(ctx, time.Second)
		i++
	}
}

func DownloadPages(ctx context.Context, cfg *utils.Config) {
	ch := make(chan *PageData)

	var wg sync.WaitGroup
	wg.Add(2)

	var pageWG sync.WaitGroup
	pageWG.Add(2)

	go dispatchScraper(ctx, CoursesURL, COURSES, ch, &pageWG)
	go dispatchScraper(ctx, ProgramsURL, PROGRAMS, ch, &pageWG)

	go func() {
		defer wg.Done()
		pageWG.Wait()
		close(ch)
	}()

	go savePages(cfg, ch, &wg)

	wg.Wait()
}
