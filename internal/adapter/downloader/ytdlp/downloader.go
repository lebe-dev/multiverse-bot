package ytdlp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lrstanley/go-ytdlp"
	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/probe"
	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

// format720 downloads the best quality that fits within 720p.
const format720 = "bestvideo[height<=720][ext=mp4]+bestaudio[ext=m4a]/best[height<=720][ext=mp4]/best[height<=720]"

// formatBest downloads the highest available quality (for Google Drive archive).
const formatBest = "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best"

type Downloader struct {
	execPath   string
	cookiePath func(url string) string
	supported  map[domain.Platform]bool
	log        *slog.Logger
}

func New(execPath string, cookiePath func(url string) string, log *slog.Logger) *Downloader {
	return &Downloader{
		execPath:   execPath,
		cookiePath: cookiePath,
		log:        log,
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

// DownloadMedia is not implemented for yt-dlp; composite downloader will fall back to Download.
func (d *Downloader) DownloadMedia(_ context.Context, _ string) (*domain.MediaResult, error) {
	return nil, domain.ErrNotImplemented
}

// Download downloads at 720p — the standard quality for Telegram delivery.
func (d *Downloader) Download(ctx context.Context, url string) (*domain.Video, error) {
	return d.download(ctx, normalizeURL(url), format720)
}

// DownloadBest downloads at the highest available quality — used for Google Drive.
func (d *Downloader) DownloadBest(ctx context.Context, url string) (*domain.Video, error) {
	return d.download(ctx, normalizeURL(url), formatBest)
}

// DownloadQuality downloads at the given quality ("360p", "480p", "720p", "1080p").
// Falls back to 720p for unknown values.
func (d *Downloader) DownloadQuality(ctx context.Context, url, quality string) (*domain.Video, error) {
	return d.download(ctx, normalizeURL(url), qualityFormat(quality))
}

// qualityFormat returns the yt-dlp format selector for the given quality level.
func qualityFormat(quality string) string {
	switch quality {
	case "360p":
		return "bestvideo[height<=360][ext=mp4]+bestaudio[ext=m4a]/best[height<=360][ext=mp4]/best[height<=360]"
	case "480p":
		return "bestvideo[height<=480][ext=mp4]+bestaudio[ext=m4a]/best[height<=480][ext=mp4]/best[height<=480]"
	case "1080p":
		return "bestvideo[height<=1080][ext=mp4]+bestaudio[ext=m4a]/best[height<=1080][ext=mp4]/best[height<=1080]"
	default: // "720p"
		return format720
	}
}

// AnalyzeFormats runs yt-dlp --dump-json and returns a compact format summary
// without downloading the video.
func (d *Downloader) AnalyzeFormats(ctx context.Context, url string) (*domain.FormatSummary, error) {
	url = normalizeURL(url)

	args := []string{"--dump-json", "--no-playlist", "--no-warnings", "--js-runtimes", "node"}
	if cp := d.cookiePath(url); cp != "" {
		args = append(args, "--cookies", cp)
	}
	args = append(args, url)

	out, err := exec.CommandContext(ctx, d.execPath, args...).Output()
	if err != nil {
		return nil, fmt.Errorf("%w: yt-dlp dump-json: %v", domain.ErrDownloadFailed, err)
	}

	var info ytdlpJSON
	if err := json.Unmarshal(out, &info); err != nil {
		return nil, fmt.Errorf("%w: parse yt-dlp json: %v", domain.ErrDownloadFailed, err)
	}

	return buildSummary(&info), nil
}

// ── Internal helpers ──────────────────────────────────────────────────────────

func (d *Downloader) download(ctx context.Context, url, format string) (*domain.Video, error) {
	tmpDir, err := os.MkdirTemp("", "multiverse-ytdlp-*")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}

	outputTemplate := filepath.Join(tmpDir, "%(id)s.%(ext)s")

	d.log.Debug("running yt-dlp", "url", url, "format", format)

	cmd := ytdlp.New().
		SetExecutable(d.execPath).
		Format(format).
		Output(outputTemplate).
		NoPlaylist().
		JsRuntimes("node").
		Print("after_move:%(title)s")

	if cp := d.cookiePath(url); cp != "" {
		cmd = cmd.Cookies(cp)
	}

	result, err := cmd.Run(ctx, url)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil || len(entries) == 0 {
		_ = os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("%w: no file downloaded", domain.ErrDownloadFailed)
	}

	filePath := filepath.Join(tmpDir, entries[0].Name())
	fi, err := os.Stat(filePath)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}

	title := strings.TrimSpace(result.Stdout)
	size := probe.ApplyFaststart(ctx, filePath)
	if size == 0 {
		size = fi.Size()
	}
	d.log.Debug("yt-dlp finished", "title", title, "size_bytes", size)
	return &domain.Video{
		URL:      url,
		FilePath: filePath,
		Size:     size,
		Title:    title,
	}, nil
}

// normalizeURL rewrites threads.com → threads.net (yt-dlp extractor requirement).
func normalizeURL(url string) string {
	return strings.ReplaceAll(url, "threads.com/", "threads.net/")
}

// ── JSON structs for --dump-json output ──────────────────────────────────────

type ytdlpJSON struct {
	Title    string        `json:"title"`
	Duration float64       `json:"duration"`
	TBR      float64       `json:"tbr"`
	Formats  []ytdlpFormat `json:"formats"`
}

type ytdlpFormat struct {
	Height         *int     `json:"height"`
	VCodec         string   `json:"vcodec"`
	ACodec         string   `json:"acodec"`
	FileSize       *int64   `json:"filesize"`
	FileSizeApprox *int64   `json:"filesize_approx"`
	TBR            *float64 `json:"tbr"`
}

func (f *ytdlpFormat) formatSize(duration float64) int64 {
	if f.FileSize != nil && *f.FileSize > 0 {
		return *f.FileSize
	}
	if f.FileSizeApprox != nil && *f.FileSizeApprox > 0 {
		return *f.FileSizeApprox
	}
	if f.TBR != nil && *f.TBR > 0 && duration > 0 {
		return int64(*f.TBR * 1000 / 8 * duration)
	}
	return 0
}

func buildSummary(info *ytdlpJSON) *domain.FormatSummary {
	s := &domain.FormatSummary{
		Title:    info.Title,
		Duration: info.Duration,
	}

	var audioSize int64
	for _, f := range info.Formats {
		if strings.EqualFold(f.VCodec, "none") {
			sz := f.formatSize(info.Duration)
			if sz > audioSize {
				audioSize = sz
			}
		}
	}

	heightMap := make(map[int]int64)

	for _, f := range info.Formats {
		if f.Height == nil || *f.Height == 0 {
			continue
		}
		if strings.EqualFold(f.VCodec, "none") {
			continue
		}

		h := *f.Height
		sz := f.formatSize(info.Duration)

		if strings.EqualFold(f.ACodec, "none") && audioSize > 0 {
			sz += audioSize
		}

		if sz > heightMap[h] {
			heightMap[h] = sz
		}

		if h >= 240 && (s.MinHeight == 0 || h < s.MinHeight || (h == s.MinHeight && sz < s.MinSize)) {
			s.MinHeight = h
			s.MinSize = sz
		}
		if h <= 720 && (s.P720Height == 0 || h > s.P720Height || (h == s.P720Height && sz > s.P720Size)) {
			s.P720Height = h
			s.P720Size = sz
		}
		if s.MaxHeight == 0 || h > s.MaxHeight || (h == s.MaxHeight && sz > s.MaxSize) {
			s.MaxHeight = h
			s.MaxSize = sz
		}
	}

	heights := make([]int, 0, len(heightMap))
	for h := range heightMap {
		heights = append(heights, h)
	}
	sort.Ints(heights)
	s.Entries = make([]domain.FormatEntry, len(heights))
	for i, h := range heights {
		s.Entries[i] = domain.FormatEntry{Height: h, Size: heightMap[h]}
	}

	return s
}
