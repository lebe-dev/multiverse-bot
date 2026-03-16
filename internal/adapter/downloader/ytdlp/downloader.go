package ytdlp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lrstanley/go-ytdlp"
	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

// qualityFormats lists format selectors from best to worst quality.
// The downloader tries each in order until the file fits within maxSize.
var qualityFormats = []string{
	"bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best",
	"bestvideo[height<=720][ext=mp4]+bestaudio[ext=m4a]/best[height<=720][ext=mp4]/best[height<=720]",
	"bestvideo[height<=480][ext=mp4]+bestaudio[ext=m4a]/best[height<=480][ext=mp4]/best[height<=480]",
	"bestvideo[height<=360][ext=mp4]+bestaudio[ext=m4a]/best[height<=360][ext=mp4]/best[height<=360]",
	"worst[ext=mp4]/worst",
}

type Downloader struct {
	execPath    string
	cookiesFile string
	maxSize     int64
	supported   map[domain.Platform]bool
}

func New(execPath, cookiesFile string, maxSize int64) *Downloader {
	return &Downloader{
		execPath:    execPath,
		cookiesFile: cookiesFile,
		maxSize:     maxSize,
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

// normalizeURL rewrites known domain aliases to the canonical form expected by yt-dlp.
// yt-dlp's Threads extractor only matches threads.net, not threads.com.
func normalizeURL(url string) string {
	return strings.ReplaceAll(url, "threads.com/", "threads.net/")
}

func (d *Downloader) Download(ctx context.Context, url string) (*domain.Video, error) {
	url = normalizeURL(url)

	for _, format := range qualityFormats {
		video, err := d.tryDownload(ctx, url, format)
		if err != nil {
			return nil, err
		}
		if d.maxSize <= 0 || video.Size <= d.maxSize {
			return video, nil
		}
		// File too large — clean up and try lower quality
		_ = os.RemoveAll(filepath.Dir(video.FilePath))
	}

	return nil, fmt.Errorf("%w: video is too large even at lowest quality", domain.ErrDownloadFailed)
}

func (d *Downloader) tryDownload(ctx context.Context, url, format string) (*domain.Video, error) {
	tmpDir, err := os.MkdirTemp("", "multiverse-ytdlp-*")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}

	outputTemplate := filepath.Join(tmpDir, "%(id)s.%(ext)s")

	cmd := ytdlp.New().
		SetExecutable(d.execPath).
		Format(format).
		Output(outputTemplate).
		NoPlaylist().
		JsRuntimes("node")

	if _, err := os.Stat(d.cookiesFile); err == nil {
		cmd = cmd.Cookies(d.cookiesFile)
	}

	if _, err = cmd.Run(ctx, url); err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil || len(entries) == 0 {
		_ = os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("%w: no file downloaded", domain.ErrDownloadFailed)
	}

	filePath := filepath.Join(tmpDir, entries[0].Name())
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
