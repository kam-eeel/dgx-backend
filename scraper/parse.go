package scraper

import (
	"context"
	"degree-explorer/store"
	"degree-explorer/types"
	"degree-explorer/utils"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
)

var programCodeRe = regexp.MustCompile(`^AS[A-Z]{3}\d{4}[A-Z]?$`)

func loadDoc(fp string) (*goquery.Document, error) {
	f, err := os.Open(fp)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	doc, err := goquery.NewDocumentFromReader(f)
	if err != nil {
		return nil, err
	}

	return doc, nil
}

func parseCourses(doc *goquery.Document) []*types.Course {
	courses := []*types.Course{}
	doc.Find(".views-row").Each(func(_ int, s *goquery.Selection) {
		header := strings.Join(strings.Fields(s.Find(".js-views-accordion-group-header").Text()), " ")
		if header == "" {
			return
		}
		c := &types.Course{}

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

		c.Description = strings.TrimSpace(s.Find(".views-field-body").Find(".field-content").Text())

		if t := strings.TrimSpace(s.Find(".views-field-field-prerequisite").Find(".field-content").Text()); t != "" {
			c.Prerequisites = &t
		}
		if t := strings.TrimSpace(s.Find(".views-field-field-corequisite").Find(".field-content").Text()); t != "" {
			c.Corequisites = &t
		}
		if t := strings.TrimSpace(s.Find(".views-field-field-exclusion").Find(".field-content").Text()); t != "" {
			c.Exclusions = &t
		}

		t := strings.TrimSpace(s.Find(".views-field-field-breadth-requirements").Find(".field-content").Text())
		if t != "" {
			for _, br := range types.UTSGBreadthReqs {
				if strings.Contains(t, br) {
					c.BreadthReq = append(c.BreadthReq, br)
				}
			}
		}

		courses = append(courses, c)
	})

	return courses
}

func parsePrograms(doc *goquery.Document) []*types.Program {
	programs := []*types.Program{}
	doc.Find(".views-row").Each(func(_ int, s *goquery.Selection) {
		header := strings.Join(strings.Fields(s.Find(".js-views-accordion-group-header").Text()), " ")
		if header == "" {
			return
		}
		p := &types.Program{}

		i := strings.LastIndex(header, " - ")
		if i < 0 || !programCodeRe.MatchString(header[i+3:]) {
			log.Printf("skipping program with unexpected header: %q", header)
			return
		}

		p.Code = header[i+3:]
		p.Number = p.Code[5:]
		p.Title = header[:i]

		switch p.Code[2:5] {
		case "SPE":
			p.Type = "Specialist"
		case "MAJ":
			p.Type = "Major"
		case "MIN":
			p.Type = "Minor"
		case "FOC":
			p.Type = "Focus"
		case "CER":
			p.Type = "Certificate"
		}

		if t := strings.TrimSpace(s.Find(".views-field-field-enrolment-requirements").Find(".field-content").Text()); t != "" {
			p.EnrolmentReqs = &t
		}
		if t := strings.TrimSpace(s.Find(".views-field-field-completion-requirements").Find(".field-content").Text()); t != "" {
			p.CompletionReqs = &t
		}

		s.Find(".views-field-field-section-link").Find("a").Each(func(_ int, ss *goquery.Selection) {
			p.ProgramArea = append(p.ProgramArea, ss.Text())
		})

		programs = append(programs, p)
	})

	return programs
}

func parseDir[T any](dir string, parse func(*goquery.Document) []*T) []*T {
	files, _ := filepath.Glob(filepath.Join(dir, "*.html"))
	results := make([][]*T, len(files))

	var wg sync.WaitGroup
	for i, fp := range files {
		wg.Go(func() {
			doc, err := loadDoc(fp)
			if err != nil {
				log.Printf("load %s: %v", fp, err)
				return
			}
			results[i] = parse(doc)
		})
	}

	wg.Wait()
	return slices.Concat(results...)
}

func ParsePages(ctx context.Context, cfg *utils.Config, db *store.Store) error {
	courses := parseDir(cfg.CoursesDir, parseCourses)
	if err := db.CreateCourses(ctx, courses); err != nil {
		return err
	}

	programs := parseDir(cfg.ProgramsDir, parsePrograms)
	if err := db.CreatePrograms(ctx, programs); err != nil {
		return err
	}

	return nil
}
