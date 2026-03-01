package threads

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

// Downloader implements domain.Downloader for Threads posts.
// It extracts the video URL from Threads SSR data and downloads the file.
type Downloader struct {
	client *client
}

func New(userAgent string) *Downloader {
	return &Downloader{client: newClient(userAgent)}
}

func (d *Downloader) Supports(p domain.Platform) bool {
	return p == domain.PlatformThreads
}

func (d *Downloader) Download(ctx context.Context, url string) (*domain.Video, error) {
	videos, err := d.client.extract(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}
	if len(videos) == 0 {
		return nil, fmt.Errorf("%w: no video found", domain.ErrDownloadFailed)
	}

	tmpDir, err := os.MkdirTemp("", "multiverse-threads-*")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}

	filePath := filepath.Join(tmpDir, "video.mp4")
	f, err := os.Create(filePath)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}

	if err := d.client.downloadVideo(ctx, videos[0].URL, f); err != nil {
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
