package parser

import (
	"bytes"
	"context"
	"degree-explorer/types"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"regexp"
	"time"
)

const (
	SESSION_ENDPOINT string = "https://api.easi.utoronto.ca/ttb/current-session"
	COURSE_ENDPOINT  string = "https://api.easi.utoronto.ca/ttb/getCourses"
)

func getCurrentSessions(ctx context.Context) []string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, SESSION_ENDPOINT, nil)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	rb, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	var sessionResp struct {
		Payload []struct {
			Label string `json:"label"`
			Value string `json:"value"`
		} `json:"payload"`
		Status []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"status"`
	}

	if err := json.Unmarshal(rb, &sessionResp); err != nil {
		log.Fatal(err)
	}
	if sessionResp.Status[0].Code != 0 {
		log.Fatalf("api error %d: %s", sessionResp.Status[0].Code, sessionResp.Status[0].Message)
	}

	out := []string{}

	for _, s := range sessionResp.Payload {
		if m, _ := regexp.MatchString(`\d{5}`, s.Value); m {
			out = append(out, s.Value)
		} else if m, _ := regexp.MatchString(`\d{5}-\d{5}`, s.Value); m {
			out = append(out, s.Value)
		}
	}

	return out
}

func getCurrentOfferings(ctx context.Context) map[string][]string {
	payload, err := json.Marshal(struct {
		Direction string   `json:"direction"`
		Divisions []string `json:"divisions"`
		Sessions  []string `json:"sessions"`
	}{
		Direction: "asc",
		Divisions: []string{"ARTSC"},
		Sessions:  getCurrentSessions(ctx),
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, COURSE_ENDPOINT, bytes.NewReader(payload))
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	rb, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	var ttbResp struct {
		Payload []struct {
			Code    string `json:"code"`
			Section string `json:"sectionCode"`
		} `json:"payload"`
		Status []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"status"`
	}
	if err := json.Unmarshal(rb, &ttbResp); err != nil {
		log.Fatal(err)
	}
	if ttbResp.Status[0].Code != 0 {
		log.Fatalf("api error %d: %s", ttbResp.Status[0].Code, ttbResp.Status[0].Message)
	}

	out := make(map[string][]string)

	for _, c := range ttbResp.Payload {
		if _, ok := out[c.Code]; !ok {
			out[c.Code] = []string{}
		}
		out[c.Code] = append(out[c.Code], c.Section)
	}

	return out
}

// LinkSessions adds to courses which semester in the current academic year
// a given course is offered. If a course's Sessions is empty, that course
// is not offered in the current academic year.
func LinkSessions(ctx context.Context, courseList []*types.Course) {
	offerings := getCurrentOfferings(ctx)
	for _, c := range courseList {
		if v, ok := offerings[c.Code]; ok {
			sl := types.StringList(v)
			c.Sessions = &sl
		}
	}
}
