// Package savevids downloads YouTube videos via the vidssave.com API.
package savevids

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const apiURL = "https://api.vidssave.com/api/contentsite_api/media/parse"

type resource struct {
	Type        string  `json:"type"`
	Format      string  `json:"format"`
	Quality     string  `json:"quality"`
	DownloadURL string  `json:"download_url"`
	Size        float64 `json:"size"` // bytes
}

type mediaData struct {
	Title     string     `json:"title"`
	Thumbnail string     `json:"thumbnail"`
	Duration  string     `json:"duration"`
	Resources []resource `json:"resources"`
}

type apiResponse struct {
	Status int       `json:"status"`
	Data   mediaData `json:"data"`
}

type client struct {
	httpClient *http.Client
}

func newClient() *client {
	return &client{
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// fetchVideoURL calls the vidssave API and returns the best video download URL
// that fits within maxSize. If maxSize <= 0, the best quality is returned.
func (c *client) fetchVideoURL(ctx context.Context, videoURL string, maxSize int64) (string, error) {
	form := url.Values{
		"auth":   {"20250901majwlqo"},
		"domain": {"api-ak.vidssave.com"},
		"origin": {"cache"},
		"link":   {videoURL},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Referer", "https://vidssave.com/")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request to vidssave: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("vidssave returned status %d", resp.StatusCode)
	}

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	if apiResp.Status != 1 {
		return "", fmt.Errorf("vidssave status: %d", apiResp.Status)
	}

	return pickBestVideo(apiResp.Data.Resources, maxSize)
}

// downloadFile streams the video from downloadURL into w.
func (c *client) downloadFile(ctx context.Context, downloadURL string, w io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	_, err = io.Copy(w, resp.Body)
	return err
}

// pickBestVideo selects the largest video resource that fits within maxSize.
// Resources are sorted by size descending; the first one <= maxSize is returned.
func pickBestVideo(resources []resource, maxSize int64) (string, error) {
	var videos []resource
	for _, r := range resources {
		if r.Type == "video" && r.DownloadURL != "" {
			videos = append(videos, r)
		}
	}

	if len(videos) == 0 {
		return "", fmt.Errorf("no video resources found")
	}

	sort.Slice(videos, func(i, j int) bool {
		return videos[i].Size > videos[j].Size
	})

	if maxSize <= 0 {
		return videos[0].DownloadURL, nil
	}

	for _, v := range videos {
		if int64(v.Size) <= maxSize {
			return v.DownloadURL, nil
		}
	}

	// All videos exceed maxSize — return the smallest one and let
	// the upper layer (usecase) handle the size check.
	return videos[len(videos)-1].DownloadURL, nil
}
