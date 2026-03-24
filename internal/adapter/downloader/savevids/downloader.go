package savevids

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/dlutil"
	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

// Downloader implements domain.Downloader for YouTube videos
// using the vidssave.com API.
type Downloader struct {
	client  *client
	maxSize int64
	log     *slog.Logger
}

func New(maxSize int64, log *slog.Logger) *Downloader {
	return &Downloader{
		client:  newClient(),
		maxSize: maxSize,
		log:     log,
	}
}

func (d *Downloader) Supports(p domain.Platform) bool {
	return p == domain.PlatformYouTube
}

// DownloadMedia is not implemented for savevids; composite downloader will fall back to Download.
func (d *Downloader) DownloadMedia(_ context.Context, _ string) (*domain.MediaResult, error) {
	return nil, domain.ErrNotImplemented
}

func (d *Downloader) Download(ctx context.Context, url string) (*domain.Video, error) {
	d.log.Debug("fetching savevids URL", "url", url)
	downloadURL, err := d.client.fetchVideoURL(ctx, url, d.maxSize)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}
	d.log.Debug("savevids got download URL")

	return dlutil.SaveToTemp("multiverse-savevids-*", url, func(f *os.File) error {
		return d.client.downloadFile(ctx, downloadURL, f)
	})
}
