package savevids

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

// Downloader implements domain.Downloader for YouTube videos
// using the vidssave.com API.
type Downloader struct {
	client  *client
	maxSize int64
}

func New(maxSize int64) *Downloader {
	return &Downloader{
		client:  newClient(),
		maxSize: maxSize,
	}
}

func (d *Downloader) Supports(p domain.Platform) bool {
	return p == domain.PlatformYouTube
}

func (d *Downloader) Download(ctx context.Context, url string) (*domain.Video, error) {
	downloadURL, err := d.client.fetchVideoURL(ctx, url, d.maxSize)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}

	tmpDir, err := os.MkdirTemp("", "multiverse-savevids-*")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}

	filePath := filepath.Join(tmpDir, "video.mp4")
	f, err := os.Create(filePath)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}

	if err := d.client.downloadFile(ctx, downloadURL, f); err != nil {
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
