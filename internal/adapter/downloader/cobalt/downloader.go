package cobalt

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/lostdusty/gobalt"
	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

type Downloader struct {
	supported map[domain.Platform]bool
}

func New(apiURL string) *Downloader {
	if apiURL != "" {
		gobalt.CobaltApi = apiURL
	}
	return &Downloader{
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

func (d *Downloader) Download(ctx context.Context, url string) (*domain.Video, error) {
	settings := gobalt.CreateDefaultSettings()
	settings.Url = url

	result, err := gobalt.Run(settings)
	if err != nil {
		return nil, fmt.Errorf("%w: cobalt: %v", domain.ErrDownloadFailed, err)
	}

	tmpDir, err := os.MkdirTemp("", "multiverse-cobalt-*")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}

	filePath := filepath.Join(tmpDir, "video.mp4")
	if err := downloadFile(ctx, result.URL, filePath); err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}

	info, err := os.Stat(filePath)
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}

	return &domain.Video{
		URL:      url,
		FilePath: filePath,
		Size:     info.Size(),
	}, nil
}

func downloadFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}
