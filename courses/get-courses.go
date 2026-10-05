package courses

import (
	"bytes"
	"degree-explorer/utils"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type PageData struct {
	n      int
	campus string
	data   []byte
}

var ErrNoCourses = errors.New("no course entries found on the page")

var campusUrls = map[string]string{
	"utsg": "https://artsci.calendar.utoronto.ca/search-courses?page=",
	"utm":  "https://utm.calendar.utoronto.ca/course-search?page=",
	"utsc": "https://utsc.calendar.utoronto.ca/search-courses?page=",
}

var getFailures atomic.Int64

func checkPageHasCourses(url string) ([]byte, error) {
	page, err := utils.GetPage(url)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(page))
	if err != nil {
		return nil, err
	}

	entries := doc.Find(".views-row")
	if entries.Length() == 0 {
		return nil, ErrNoCourses
	}

	return page, nil
}

func savePages(cfg *utils.Config, ch <-chan *PageData, wg *sync.WaitGroup) {
	defer wg.Done()
	for page := range ch {
		filePath := filepath.Join(cfg.CoursesDir, page.campus, fmt.Sprintf("page-%d.html", page.n))
		log.Printf("\t\tsaving to %s...", filePath)
		if err := os.WriteFile(filePath, page.data, 0644); err != nil {
			log.Printf("\t\tfailed to write file %s: %v", filePath, err)
			getFailures.Add(1)
			continue
		}
		log.Printf("\t\tdone!")
	}
}

func GetCoursePages(cfg *utils.Config) {
	ch := make(chan *PageData)
	var wg sync.WaitGroup
	wg.Add(2)

	var pageWG sync.WaitGroup

	for _, campus := range cfg.Campuses {
		path := filepath.Join(cfg.CoursesDir, campus)
		if err := os.MkdirAll(path, 0755); err != nil {
			log.Fatalf("failed to create directory %s: %v", path, err)
		}
	}

	for _, campus := range cfg.Campuses {
		pageWG.Go(func() {
			i := 0
			retries := 0
			skipped := 0
			for {
				pageUrl := campusUrls[campus] + strconv.Itoa(i)
				page, err := checkPageHasCourses(pageUrl)
				if errors.Is(err, ErrNoCourses) {
					log.Printf("finished collecting all pages for %s", campus)
					break
				} else if err != nil {
					retries++
					log.Printf("error processing pages for %s at page %d (attempt %d): %v", campus, i, retries, err)
					if retries >= 3 {
						log.Printf("skipping %s page %d after %d attempts", campus, i, retries)
						retries = 0
						skipped++
						getFailures.Add(1)
						if skipped >= 3 {
							log.Printf("giving up on %s after %d consecutive skipped pages", campus, skipped)
							getFailures.Add(1)
							break
						}
						i++
					}
					time.Sleep(5 * time.Second)
					continue
				}
				retries = 0
				skipped = 0
				ch <- &PageData{n: i, campus: campus, data: page}
				time.Sleep(1500 * time.Millisecond)
				i++
			}
		})
	}

	go func() {
		defer wg.Done()
		pageWG.Wait()
		close(ch)
	}()

	go savePages(cfg, ch, &wg)

	wg.Wait()

	if n := getFailures.Load(); n > 0 {
		log.Printf("finished collecting course pages for all campuses with %d failures", n)
	} else {
		log.Printf("all course pages for all campuses collected successfully!")
	}
}
