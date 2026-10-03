package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
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

const dataDir = "data/"

var ErrNoCourses = errors.New("no course entries found on the page")

var campuses = []string{"utsg", "utm", "utsc"}

var campusUrls = map[string]string{
	"utsg": "https://artsci.calendar.utoronto.ca/search-courses?page=",
	"utm":  "https://utm.calendar.utoronto.ca/course-search?page=",
	"utsc": "https://utsc.calendar.utoronto.ca/search-courses?page=",
}

var client = &http.Client{Timeout: 30 * time.Second}

var failures atomic.Int64

func getPage(url string) ([]byte, error) {
	log.Printf("\tgetting page %s...", url)

	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected http status %d for %s", resp.StatusCode, url)
	}

	rb, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(rb))
	if err != nil {
		return nil, err
	}

	entries := doc.Find(".views-row")
	if entries.Length() == 0 {
		return nil, ErrNoCourses
	}

	return rb, nil
}

func GetPages(ch chan<- *PageData, wg *sync.WaitGroup) {
	defer wg.Done()
	defer close(ch)
	for _, campus := range campuses {
		log.Printf("beginning processing pages for %s...", campus)
		i := 0
		retries := 0
		skipped := 0
		for {
			pageUrl := campusUrls[campus] + strconv.Itoa(i)
			page, err := getPage(pageUrl)
			if errors.Is(err, ErrNoCourses) {
				log.Printf("finished processing all pages for %s", campus)
				break
			} else if err != nil {
				retries++
				log.Printf("error processing pages for %s at page %d (attempt %d): %v", campus, i, retries, err)
				if retries >= 3 {
					log.Printf("skipping %s page %d after %d attempts", campus, i, retries)
					retries = 0
					skipped++
					failures.Add(1)
					if skipped >= 3 {
						log.Printf("giving up on %s after %d consecutive skipped pages", campus, skipped)
						failures.Add(1)
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
	}
}

func SavePages(ch <-chan *PageData, wg *sync.WaitGroup) {
	defer wg.Done()
	for page := range ch {
		filePath := filepath.Join(dataDir, page.campus, fmt.Sprintf("page-%d.html", page.n))
		log.Printf("\t\tsaving to %s...", filePath)
		if err := os.WriteFile(filePath, page.data, 0644); err != nil {
			log.Printf("\t\tfailed to write file %s: %v", filePath, err)
			failures.Add(1)
			continue
		}
		log.Printf("\t\tdone!")
	}
}

func main() {
	ch := make(chan *PageData)
	var wg sync.WaitGroup
	wg.Add(2)

	for _, campus := range campuses {
		path := filepath.Join(dataDir, campus)
		if err := os.MkdirAll(path, 0755); err != nil {
			log.Fatalf("failed to create directory %s: %v", path, err)
		}
	}

	go GetPages(ch, &wg)
	go SavePages(ch, &wg)

	wg.Wait()

	if n := failures.Load(); n > 0 {
		log.Printf("finished with %d failures; see log above", n)
	} else {
		log.Printf("all pages for all campuses processed successfully!")
	}
}
