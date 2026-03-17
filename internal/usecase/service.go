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

// ProcessURL detects the platform, downloads the video, and checks its size.
// Returns the video and a cleanup function that must be called after use.
func (s *VideoService) ProcessURL(ctx context.Context, url string) (*domain.Video, func(), error) {
	platform := s.detector.Detect(url)
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
