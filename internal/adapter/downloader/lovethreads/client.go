// Package lovethreads downloads videos from Threads posts via the lovethreads.net proxy service.
//
// Instead of scraping threads.net directly, this backend delegates extraction
// to lovethreads.net which returns HTML with direct media links.
package lovethreads

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const apiURL = "https://lovethreads.net/api/ajaxSearch"

// apiResponse represents the JSON response from lovethreads.net.
type apiResponse struct {
	Status string `json:"status"`
	Data   string `json:"data"`
}

type client struct {
	httpClient *http.Client
}

func newClient() *client {
	return &client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// extractVideoURLs sends the Threads post URL to lovethreads.net and parses
// the response HTML to extract direct video download links.
func (c *client) extractVideoURLs(ctx context.Context, postURL string) ([]string, error) {
	body := fmt.Sprintf("q=%s&t=media&lang=en", postURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("Origin", "https://lovethreads.net")
	req.Header.Set("Referer", "https://lovethreads.net/en")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to lovethreads.net: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lovethreads.net returned status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var apiResp apiResponse
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return nil, fmt.Errorf("parse response JSON: %w", err)
	}

	if apiResp.Status != "ok" {
		return nil, fmt.Errorf("lovethreads.net status: %s", apiResp.Status)
	}

	return parseVideoURLs(apiResp.Data), nil
}

// downloadVideo streams the video content from the given URL to w.
func (c *client) downloadVideo(ctx context.Context, videoURL string, w io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, videoURL, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("video download failed with status %d", resp.StatusCode)
	}

	_, err = io.Copy(w, resp.Body)
	return err
}

// Two patterns to handle both attribute orderings in <a> tags.
var (
	videoLinkTitleFirst = regexp.MustCompile(`<a[^>]*title="Download Video"[^>]*href="([^"]+)"`)
	videoLinkHrefFirst  = regexp.MustCompile(`<a[^>]*href="([^"]+)"[^>]*title="Download Video"`)
)

// parseVideoURLs extracts video download URLs from the lovethreads.net HTML response.
func parseVideoURLs(html string) []string {
	seen := make(map[string]struct{})
	var urls []string

	for _, re := range []*regexp.Regexp{videoLinkTitleFirst, videoLinkHrefFirst} {
		for _, m := range re.FindAllStringSubmatch(html, -1) {
			u := strings.ReplaceAll(m[1], "&amp;", "&")
			if _, ok := seen[u]; ok {
				continue
			}
			seen[u] = struct{}{}
			urls = append(urls, u)
		}
	}

	return urls
}
