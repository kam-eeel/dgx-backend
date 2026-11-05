package scraper

import (
	"bytes"
	"context"
	"degree-explorer/types"
	"degree-explorer/utils"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"time"
)

const (
	ttbSessionURL = "https://api.easi.utoronto.ca/ttb/current-session"
	ttbCoursesURL = "https://api.easi.utoronto.ca/ttb/getCourses"
)

// sessionRe matches fall, winter, full-year and summer session codes: "20269",
// "20271", "20269-20271", "20275", "20275F", "20275F-20275S".
var sessionRe = regexp.MustCompile(`^(?:\d{4}[19]|\d{4}9-\d{4}1|\d{4}5[FS]?|\d{4}5F-\d{4}5S)$`)

var ttbClient = &http.Client{}

type ttbSession struct {
	Value string `json:"value"`
}

type ttbCourse struct {
	Code     string   `json:"code"`
	Section  string   `json:"sectionCode"`
	Sessions []string `json:"sessions"` // ex. ["20269", "20271"]
}

// FetchOfferings returns the timetable's sections ("F", "S", "Y") by term and
// course code for the current sessions.
func FetchOfferings(ctx context.Context) (map[string]types.Sessions, error) {
	sctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var sessions []ttbSession
	if err := fetchTTB(sctx, http.MethodGet, ttbSessionURL, nil, &sessions); err != nil {
		return nil, fmt.Errorf("current session: %w", err)
	}
	codes := currentSessions(sessions)
	if len(codes) == 0 {
		return nil, errors.New("current session: none listed")
	}

	body, err := json.Marshal(struct {
		Direction string   `json:"direction"`
		Divisions []string `json:"divisions"`
		Sessions  []string `json:"sessions"`
	}{"asc", []string{"ARTSC"}, codes})
	if err != nil {
		return nil, err
	}

	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var courses []ttbCourse
	if err := fetchTTB(cctx, http.MethodPost, ttbCoursesURL, body, &courses); err != nil {
		return nil, fmt.Errorf("courses: %w", err)
	}
	if len(courses) == 0 {
		return nil, fmt.Errorf("courses: none listed for %v", codes)
	}
	return offeringsByCode(courses), nil
}

// fetchTTB decodes a timetable API response's payload into out.
func fetchTTB(ctx context.Context, method, url string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := ttbClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &utils.HTTPError{StatusCode: resp.StatusCode}
	}

	rb, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var r struct {
		Payload json.RawMessage `json:"payload"`
		Status  []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"status"`
	}
	if err := json.Unmarshal(rb, &r); err != nil {
		return err
	}
	if len(r.Status) == 0 {
		return errors.New("response has no status")
	}
	if s := r.Status[0]; s.Code != 0 {
		return fmt.Errorf("api error %d: %s", s.Code, s.Message)
	}
	return json.Unmarshal(r.Payload, out)
}

func currentSessions(ss []ttbSession) []string {
	var out []string
	for _, s := range ss {
		if sessionRe.MatchString(s.Value) {
			out = append(out, s.Value)
		}
	}
	return out
}

// offeringsByCode groups section codes by course and term, sorted and
// deduplicated.
func offeringsByCode(cs []ttbCourse) map[string]types.Sessions {
	out := make(map[string]types.Sessions)
	for _, c := range cs {
		s := out[c.Code]
		if isSummer(c.Sessions) {
			s.Summer = append(s.Summer, c.Section)
		} else {
			s.FallWinter = append(s.FallWinter, c.Section)
		}
		out[c.Code] = s
	}
	for code, s := range out {
		out[code] = types.Sessions{FallWinter: sortedSet(s.FallWinter), Summer: sortedSet(s.Summer)}
	}
	return out
}

// isSummer reports whether a section's sessions are summer ones, whose fifth
// digit is 5: ["20275F"].
func isSummer(sessions []string) bool {
	return len(sessions) > 0 && len(sessions[0]) >= 5 && sessions[0][4] == '5'
}

func sortedSet(l types.StringList) types.StringList {
	slices.Sort(l)
	return slices.Compact(l)
}
