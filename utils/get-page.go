package utils

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

var client = &http.Client{Timeout: 30 * time.Second}

func GetPage(url string) ([]byte, error) {
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

	return rb, nil
}
