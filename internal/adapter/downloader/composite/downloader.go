package composite

import (
	"context"
	"fmt"
	"log/slog"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

type Downloader struct {
	backends []domain.Downloader
	log      *slog.Logger
}

func New(log *slog.Logger, backends ...domain.Downloader) *Downloader {
	return &Downloader{
		backends: backends,
		log:      log,
	}
}

func (d *Downloader) Download(ctx context.Context, url string, platform domain.Platform) (*domain.Video, error) {
	var lastErr error
	for _, b := range d.backends {
		if !b.Supports(platform) {
			continue
		}
		video, err := b.Download(ctx, url)
		if err == nil {
			return video, nil
		}
		d.log.Warn("downloader failed, trying next",
			"error", err,
			"backend", fmt.Sprintf("%T", b),
		)
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, domain.ErrUnsupportedPlatform
}
