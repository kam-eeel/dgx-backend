package courses

import (
	"context"
	"degree-explorer/store"
	"degree-explorer/types"
	"degree-explorer/utils"
	"log"
	"os"
	"path"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/PuerkitoBio/goquery"
)

var parseFailures atomic.Int64

type pageResult struct {
	pagePath      string
	courseEntries *[]*types.Course
}

func getEntryFromPage(s *goquery.Selection) *types.Course {
	c := &types.Course{}

	header := strings.TrimSpace(s.Find(".js-views-accordion-group-header").Text())

	c.Code = header[:8]
	c.Designator = header[:3]
	c.Number = c.Code[3:6]

	lvl := string(c.Code[3])
	switch lvl {
	case "1", "A":
		c.Level = 100
	case "2", "B":
		c.Level = 200
	case "3", "C":
		c.Level = 300
	case "4", "D":
		c.Level = 400
	}

	if string(c.Code[6]) == "Y" {
		c.Weight = 1.0
	} else {
		c.Weight = 0.5
	}

	switch string(c.Code[7]) {
	case "0":
		c.Campus = string(types.ONLINE)
	case "1":
		c.Campus = string(types.UTSG)
	case "3":
		c.Campus = string(types.UTSC)
	case "5":
		c.Campus = string(types.UTM)
	}

	c.Title = strings.SplitN(header, " ", 3)[2]

	if t := strings.TrimSpace(s.Find(".views-field-field-prerequisite").Find(".field-content").Text()); t != "" {
		c.Prerequisites = &t
	}
	if t := strings.TrimSpace(s.Find(".views-field-field-corequisite").Find(".field-content").Text()); t != "" {
		c.Corequisites = &t
	}
	if t := strings.TrimSpace(s.Find(".views-field-field-exclusion").Find(".field-content").Text()); t != "" {
		c.Exclusions = &t
	}
	if t := strings.TrimSpace(s.Find(".views-field-field-breadth-requirements").Find(".field-content").Text()); t != "" {
		c.BreadthReq = &t
	}
	if t := strings.TrimSpace(s.Find(".views-field-field-distribution-requirements").Find(".field-content").Text()); t != "" {
		c.DistributionReq = &t
	}

	return c
}

func processPage(pagePath string, pageResults chan<- *pageResult, wg *sync.WaitGroup) {
	defer wg.Done()
	var cl []*types.Course

	page, err := os.Open(pagePath)
	if err != nil {
		log.Printf("failed to read file %s: %v", pagePath, err)
		parseFailures.Add(1)
		return
	}
	defer page.Close()

	doc, err := goquery.NewDocumentFromReader(page)
	if err != nil {
		log.Printf("failed to initialize goquery document for page %s: %v", pagePath, err)
		parseFailures.Add(1)
		return
	}

	entries := doc.Find(".views-row")
	if entries.Length() == 0 {
		log.Printf("%s has no courses - skipping", pagePath)
		parseFailures.Add(1)
		return
	}

	entries.Each(func(_ int, s *goquery.Selection) {
		if strings.TrimSpace(s.Find(".js-views-accordion-group-header").Text()) != "" {
			new := getEntryFromPage(s)
			cl = append(cl, new)
		}
	})

	pageResults <- &pageResult{
		pagePath:      pagePath,
		courseEntries: &cl,
	}
}

func saveCourses(ctx context.Context, db *store.Store, pageResults <-chan *pageResult, wg *sync.WaitGroup) {
	defer wg.Done()
	for pageResult := range pageResults {
		if err := db.CreateCourses(ctx, *pageResult.courseEntries); err != nil {
			log.Printf("failed to send %s to db: %v", pageResult.pagePath, err)
			parseFailures.Add(1)
		}
	}
}

func ParseCoursePages(cfg *utils.Config, ctx context.Context, db *store.Store) types.ParseResult {
	pageResults := make(chan *pageResult)

	var wg sync.WaitGroup
	wg.Add(2)

	var pageWG sync.WaitGroup

	for _, campus := range cfg.Campuses {
		pages, err := os.ReadDir(path.Join(cfg.DataDir, campus))
		if err != nil {
			log.Fatal(err)
		}
		for _, page := range pages {
			pagePath := path.Join(cfg.DataDir, campus, page.Name())
			pageWG.Add(1)
			go processPage(pagePath, pageResults, &pageWG)
		}
	}

	go func() {
		defer wg.Done()
		pageWG.Wait()
		close(pageResults)
	}()

	go saveCourses(ctx, db, pageResults, &wg)

	wg.Wait()

	if n := parseFailures.Load(); n == 0 {
		log.Println("all course pages parsed and sent to db successfully!")
		return types.ParseSuccessful
	} else {
		log.Printf("all course pages parsed and sent to db with %d failures\n", n)
		return types.ParseFailure
	}

}
