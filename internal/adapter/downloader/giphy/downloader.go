// Package giphy downloads GIFs from giphy.com.
//
// Every GIPHY asset is addressed by an opaque id that appears in all of its
// public URLs (page, embed, media CDN). The downloader extracts that id and
// fetches the pre-rendered MP4 rendition from the media CDN, falling back to
// the original GIF when no MP4 exists. Both are sent to Telegram as an
// animation.
package giphy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

const (
	defaultMediaBaseURL = "https://media.giphy.com/media"
	userAgent           = "Mozilla/5.0 (compatible; multiverse-bot)"
	// maxSize bounds a single rendition; GIPHY caps uploads well below this.
	maxSize = 200 * 1024 * 1024
)

// ErrInvalidURL is returned when no GIPHY id can be extracted from a URL.
var ErrInvalidURL = errors.New("giphy: cannot extract gif id from url")

// idRe matches a GIPHY id: alphanumeric, no dashes (dashes separate the slug).
var idRe = regexp.MustCompile(`^[A-Za-z0-9]+$`)

type rendition struct {
	file string // file name on the media CDN
	ext  string
}

// renditions are tried in order: MP4 is an order of magnitude smaller than GIF.
var renditions = []rendition{
	{"giphy.mp4", ".mp4"},
	{"giphy.gif", ".gif"},
}

// Downloader implements domain.Downloader for giphy.com.
type Downloader struct {
	httpClient   *http.Client
	mediaBaseURL string
	log          *slog.Logger
}

func New(log *slog.Logger) *Downloader {
	return &Downloader{
		httpClient:   &http.Client{Timeout: 2 * time.Minute},
		mediaBaseURL: defaultMediaBaseURL,
		log:          log,
	}
}

func (d *Downloader) Supports(p domain.Platform) bool {
	return p == domain.PlatformGiphy
}

func (d *Downloader) Download(ctx context.Context, rawURL string) (*domain.Video, error) {
	res, err := d.DownloadMedia(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	return res.AsVideo(), nil
}

func (d *Downloader) DownloadMedia(ctx context.Context, rawURL string) (*domain.MediaResult, error) {
	id, err := d.resolveID(ctx, rawURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrDownloadFailed, err)
	}

	tmpDir, err := os.MkdirTemp("", "multiverse-giphy-*")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}

	var lastErr error
	for _, r := range renditions {
		mediaURL := fmt.Sprintf("%s/%s/%s", d.mediaBaseURL, id, r.file)
		filePath := filepath.Join(tmpDir, "animation"+r.ext)
		d.log.Debug("downloading giphy rendition", "id", id, "url", mediaURL)

		size, err := d.fetch(ctx, mediaURL, filePath)
		if err != nil {
			lastErr = err
			continue
		}
		return &domain.MediaResult{
			Items: []domain.MediaItem{{
				Type:     domain.MediaAnimation,
				FilePath: filePath,
				Size:     size,
				URL:      mediaURL,
			}},
			Platform: domain.PlatformGiphy,
			URL:      rawURL,
		}, nil
	}

	_ = os.RemoveAll(tmpDir)
	return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, lastErr)
}

// resolveID extracts the GIPHY id, following gph.is short-link redirects.
func (d *Downloader) resolveID(ctx context.Context, rawURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", err
	}
	if strings.EqualFold(u.Hostname(), "gph.is") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("User-Agent", userAgent)
		resp, err := d.httpClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("resolve short link: %w", err)
		}
		_ = resp.Body.Close()
		u = resp.Request.URL
	}
	id := ExtractID(u)
	if id == "" {
		return "", ErrInvalidURL
	}
	return id, nil
}

// ExtractID returns the GIPHY id from a parsed URL, or "" if there is none.
//
// Supported forms:
//
//	giphy.com/gifs/<slug>-<id>, giphy.com/gifs/<id>
//	giphy.com/stickers/<slug>-<id>
//	giphy.com/embed/<id>
//	media*.giphy.com/media/[v1.<token>/]<id>/<file>
//	i.giphy.com/<id>.<ext>, i.giphy.com/media/<id>/<file>
func ExtractID(u *url.URL) string {
	host := strings.ToLower(u.Hostname())
	if host != "giphy.com" && !strings.HasSuffix(host, ".giphy.com") {
		return ""
	}
	parts := strings.FieldsFunc(u.Path, func(r rune) bool { return r == '/' })
	if len(parts) == 0 {
		return ""
	}

	var candidate string
	switch {
	case parts[0] == "media":
		// /media/<id>/<file> or /media/v1.<token>/<id>/<file>
		rest := parts[1:]
		if len(rest) > 0 && strings.HasPrefix(rest[0], "v1.") {
			rest = rest[1:]
		}
		if len(rest) > 0 {
			candidate = rest[0]
		}
	case host == "i.giphy.com" && len(parts) == 1:
		candidate = strings.TrimSuffix(parts[0], filepath.Ext(parts[0]))
	case (parts[0] == "gifs" || parts[0] == "stickers" || parts[0] == "embed") && len(parts) >= 2:
		slug := parts[1]
		candidate = slug[strings.LastIndex(slug, "-")+1:]
	}

	if !idRe.MatchString(candidate) {
		return ""
	}
	return candidate
}

func (d *Downloader) fetch(ctx context.Context, mediaURL, filePath string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("GET %s: status %d", mediaURL, resp.StatusCode)
	}

	f, err := os.Create(filePath)
	if err != nil {
		return 0, err
	}
	n, copyErr := io.Copy(f, io.LimitReader(resp.Body, maxSize+1))
	closeErr := f.Close()
	if copyErr == nil && n > maxSize {
		copyErr = fmt.Errorf("%s exceeds %d bytes", mediaURL, maxSize)
	}
	if err := errors.Join(copyErr, closeErr); err != nil {
		_ = os.Remove(filePath)
		return 0, err
	}
	if n == 0 {
		_ = os.Remove(filePath)
		return 0, fmt.Errorf("GET %s: empty body", mediaURL)
	}
	return n, nil
}
