package lovethreads

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/dlutil"
	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

// Downloader implements domain.Downloader for Threads posts
// using the lovethreads.net proxy service.
type Downloader struct {
	client *client
	log    *slog.Logger
}

func New(log *slog.Logger) *Downloader {
	return &Downloader{client: newClient(), log: log}
}

func (d *Downloader) Supports(p domain.Platform) bool {
	return p == domain.PlatformThreads
}

// DownloadMedia is not implemented for lovethreads; composite downloader will fall back to Download.
func (d *Downloader) DownloadMedia(_ context.Context, _ string) (*domain.MediaResult, error) {
	return nil, domain.ErrNotImplemented
}

func (d *Downloader) Download(ctx context.Context, url string) (*domain.Video, error) {
	d.log.Debug("extracting lovethreads video", "url", url)
	urls, err := d.client.extractVideoURLs(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("%w: no video found", domain.ErrDownloadFailed)
	}
	d.log.Debug("lovethreads extracted video URLs", "count", len(urls))

	d.log.Debug("downloading lovethreads video")
	return dlutil.SaveToTemp("multiverse-lovethreads-*", url, func(f *os.File) error {
		return d.client.downloadVideo(ctx, urls[0], f)
	})
}
