package cobalt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

type Downloader struct {
	apiURL     string
	httpClient *http.Client
	supported  map[domain.Platform]bool
}

func New(apiURL string) *Downloader {
	if apiURL == "" {
		apiURL = "https://api.cobalt.tools"
	}
	return &Downloader{
		apiURL:     apiURL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		supported: map[domain.Platform]bool{
			domain.PlatformYouTube:   true,
			domain.PlatformInstagram: true,
			domain.PlatformTwitter:   true,
			domain.PlatformThreads:   true,
		},
	}
}

func (d *Downloader) Supports(p domain.Platform) bool {
	return d.supported[p]
}

type cobaltRequest struct {
	URL string `json:"url"`
}

type cobaltResponse struct {
	Status string `json:"status"`
	URL    string `json:"url"`
	Error  *struct {
		Code string `json:"code"`
	} `json:"error"`
}

func (d *Downloader) Download(ctx context.Context, url string) (*domain.Video, error) {
	body, err := json.Marshal(cobaltRequest{URL: url})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	var cr cobaltResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return nil, fmt.Errorf("%w: invalid response: %v", domain.ErrDownloadFailed, err)
	}

	if cr.Status == "error" {
		code := "unknown"
		if cr.Error != nil {
			code = cr.Error.Code
		}
		return nil, fmt.Errorf("%w: cobalt: %s", domain.ErrDownloadFailed, code)
	}

	if cr.URL == "" {
		return nil, fmt.Errorf("%w: cobalt returned no URL (status: %s)", domain.ErrDownloadFailed, cr.Status)
	}

	tmpDir, err := os.MkdirTemp("", "multiverse-cobalt-*")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}

	filePath := filepath.Join(tmpDir, "video.mp4")
	if err := downloadFile(ctx, d.httpClient, cr.URL, filePath); err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}

	info, err := os.Stat(filePath)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}

	return &domain.Video{
		URL:      url,
		FilePath: filePath,
		Size:     info.Size(),
	}, nil
}

func downloadFile(ctx context.Context, client *http.Client, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	_, err = io.Copy(f, resp.Body)
	return err
}
