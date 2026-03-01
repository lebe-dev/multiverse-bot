package ytdlp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lrstanley/go-ytdlp"
	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

type Downloader struct {
	supported map[domain.Platform]bool
}

func New() *Downloader {
	return &Downloader{
		supported: map[domain.Platform]bool{
			domain.PlatformYouTube:   true,
			domain.PlatformInstagram: true,
			domain.PlatformTwitter:   true,
		},
	}
}

func (d *Downloader) Supports(p domain.Platform) bool {
	return d.supported[p]
}

func (d *Downloader) Download(ctx context.Context, url string) (*domain.Video, error) {
	tmpDir, err := os.MkdirTemp("", "multiverse-ytdlp-*")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}

	outputTemplate := filepath.Join(tmpDir, "%(id)s.%(ext)s")

	cmd := ytdlp.New().
		FormatSort("ext:mp4:m4a").
		Output(outputTemplate).
		NoPlaylist()

	_, err = cmd.Run(ctx, url)
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil || len(entries) == 0 {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("%w: no file downloaded", domain.ErrDownloadFailed)
	}

	filePath := filepath.Join(tmpDir, entries[0].Name())
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
