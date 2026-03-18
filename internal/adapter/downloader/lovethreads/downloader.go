package lovethreads

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

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

	tmpDir, err := os.MkdirTemp("", "multiverse-lovethreads-*")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}

	filePath := filepath.Join(tmpDir, "video.mp4")
	f, err := os.Create(filePath)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}

	d.log.Debug("downloading lovethreads video")
	if err := d.client.downloadVideo(ctx, urls[0], f); err != nil {
		_ = f.Close()
		_ = os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}
	_ = f.Close()

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
