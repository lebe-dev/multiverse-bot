package cobalt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/dlutil"
	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

type Downloader struct {
	apiURL     string
	httpClient *http.Client
	supported  map[domain.Platform]bool
	log        *slog.Logger
}

func New(apiURL string, log *slog.Logger) *Downloader {
	if apiURL == "" {
		apiURL = "https://api.cobalt.tools"
	}
	return &Downloader{
		apiURL:     apiURL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		log:        log,
		supported: map[domain.Platform]bool{
			domain.PlatformInstagram: true,
			domain.PlatformTwitter:   true,
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
	Status string       `json:"status"`
	URL    string       `json:"url"`
	Picker []pickerItem `json:"picker"`
	Error  *struct {
		Code string `json:"code"`
	} `json:"error"`
}

type pickerItem struct {
	Type  string `json:"type"` // "photo" or "video"
	URL   string `json:"url"`
	Thumb string `json:"thumb"`
}

const maxPickerItems = 10 // Telegram album limit

func (d *Downloader) callAPI(ctx context.Context, url string) (*cobaltResponse, error) {
	d.log.Debug("calling cobalt API", "url", url, "api", d.apiURL)
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

	d.log.Debug("cobalt API response", "status", cr.Status)

	if cr.Status == "error" {
		code := "unknown"
		if cr.Error != nil {
			code = cr.Error.Code
		}
		return nil, fmt.Errorf("%w: cobalt: %s", domain.ErrDownloadFailed, code)
	}

	return &cr, nil
}

func (d *Downloader) Download(ctx context.Context, url string) (*domain.Video, error) {
	cr, err := d.callAPI(ctx, url)
	if err != nil {
		return nil, err
	}

	if cr.URL == "" {
		return nil, fmt.Errorf("%w: cobalt returned no URL (status: %s)", domain.ErrDownloadFailed, cr.Status)
	}

	d.log.Debug("downloading cobalt file")
	return dlutil.SaveToTemp("multiverse-cobalt-*", url, func(f *os.File) error {
		return downloadToFile(ctx, d.httpClient, cr.URL, f)
	})
}

func (d *Downloader) DownloadMedia(ctx context.Context, url string) (*domain.MediaResult, error) {
	cr, err := d.callAPI(ctx, url)
	if err != nil {
		return nil, err
	}

	switch cr.Status {
	case "picker":
		return d.downloadPicker(ctx, url, cr.Picker)
	case "stream", "redirect":
		if cr.URL == "" {
			return nil, fmt.Errorf("%w: cobalt returned no URL (status: %s)", domain.ErrDownloadFailed, cr.Status)
		}
		video, err := dlutil.SaveToTemp("multiverse-cobalt-*", url, func(f *os.File) error {
			return downloadToFile(ctx, d.httpClient, cr.URL, f)
		})
		if err != nil {
			return nil, err
		}
		return &domain.MediaResult{
			Items: []domain.MediaItem{{
				Type:     domain.MediaVideo,
				FilePath: video.FilePath,
				Size:     video.Size,
				URL:      cr.URL,
			}},
			Title: video.Title,
			URL:   url,
		}, nil
	default:
		return nil, fmt.Errorf("%w: cobalt: unexpected status %s", domain.ErrDownloadFailed, cr.Status)
	}
}

func (d *Downloader) downloadPicker(ctx context.Context, sourceURL string, items []pickerItem) (*domain.MediaResult, error) {
	tmpDir, err := os.MkdirTemp("", "multiverse-cobalt-picker-*")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}

	if len(items) > maxPickerItems {
		items = items[:maxPickerItems]
	}

	type downloadedItem struct {
		index     int
		mediaItem domain.MediaItem
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(4)

	var (
		mu         sync.Mutex
		downloaded []downloadedItem
	)

	for i, item := range items {
		i, item := i, item
		g.Go(func() error {
			if gctx.Err() != nil {
				return nil
			}

			ext := ".mp4"
			mediaType := domain.MediaVideo
			if item.Type == "photo" {
				ext = ".jpg"
				mediaType = domain.MediaPhoto
			}

			filePath := filepath.Join(tmpDir, fmt.Sprintf("item_%d%s", i, ext))
			if err := downloadToPath(gctx, d.httpClient, item.URL, filePath); err != nil {
				d.log.Warn("picker item download failed", "index", i, "error", err)
				return nil
			}

			info, err := os.Stat(filePath)
			if err != nil {
				d.log.Warn("picker item stat failed", "index", i, "error", err)
				return nil
			}

			mu.Lock()
			downloaded = append(downloaded, downloadedItem{
				index: i,
				mediaItem: domain.MediaItem{
					Type:     mediaType,
					FilePath: filePath,
					Size:     info.Size(),
					URL:      item.URL,
				},
			})
			mu.Unlock()
			return nil
		})
	}

	_ = g.Wait()

	if len(downloaded) == 0 {
		_ = os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("%w: all picker items failed", domain.ErrDownloadFailed)
	}

	sort.Slice(downloaded, func(i, j int) bool {
		return downloaded[i].index < downloaded[j].index
	})

	mediaItems := make([]domain.MediaItem, 0, len(downloaded))
	for _, d := range downloaded {
		mediaItems = append(mediaItems, d.mediaItem)
	}

	return &domain.MediaResult{
		Items: mediaItems,
		URL:   sourceURL,
	}, nil
}

func downloadToPath(ctx context.Context, client *http.Client, url, filePath string) error {
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

	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	_, err = io.Copy(f, resp.Body)
	return err
}

func downloadToFile(ctx context.Context, client *http.Client, url string, f *os.File) error {
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

	_, err = io.Copy(f, resp.Body)
	return err
}
