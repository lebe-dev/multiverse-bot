package usecase

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

const defaultMaxFileSize = 50 * 1024 * 1024 // 50MB

type VideoDownloader interface {
	Download(ctx context.Context, url string, platform domain.Platform) (*domain.Video, error)
}

type VideoService struct {
	detector   domain.PlatformDetector
	downloader VideoDownloader
	log        *slog.Logger
	maxSize    int64
}

func NewVideoService(
	detector domain.PlatformDetector,
	downloader VideoDownloader,
	log *slog.Logger,
	maxSize int64,
) *VideoService {
	if maxSize <= 0 {
		maxSize = defaultMaxFileSize
	}
	return &VideoService{
		detector:   detector,
		downloader: downloader,
		log:        log,
		maxSize:    maxSize,
	}
}

// DetectPlatform exposes platform detection for callers that need to choose
// a download strategy before calling ProcessURL (e.g. quality selection).
func (s *VideoService) DetectPlatform(url string) domain.Platform {
	return s.detector.Detect(url)
}

// MaxFileSize returns the configured file-size limit (bytes).
// The service no longer enforces it — the handler decides (Telegram, local API, Drive).
func (s *VideoService) MaxFileSize() int64 {
	return s.maxSize
}

// ProcessURL detects the platform, downloads the video, and returns it.
// Size enforcement is the caller's responsibility (handler can offer Drive upload).
func (s *VideoService) ProcessURL(ctx context.Context, url string) (*domain.Video, func(), error) {
	platform := s.detector.Detect(url)
	s.log.Debug("platform detected", "url", url, "platform", platform.String())
	if platform == domain.PlatformUnknown {
		return nil, nil, domain.ErrUnsupportedPlatform
	}

	s.log.Info("downloading video", "url", url, "platform", platform.String())

	video, err := s.downloader.Download(ctx, url, platform)
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() {
		dir := filepath.Dir(video.FilePath)
		_ = os.RemoveAll(dir)
	}

	video.Platform = platform
	return video, cleanup, nil
}
