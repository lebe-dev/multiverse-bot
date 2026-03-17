package usecase_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
	"gitlab.com/tiny-services/multiverse-bot/internal/usecase"
)

type mockDetector struct {
	result domain.Platform
}

func (m *mockDetector) Detect(string) domain.Platform {
	return m.result
}

type mockDownloader struct {
	video *domain.Video
	err   error
}

func (m *mockDownloader) Download(_ context.Context, _ string, _ domain.Platform) (*domain.Video, error) {
	return m.video, m.err
}

func newLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestProcessURL_UnknownPlatform(t *testing.T) {
	svc := usecase.NewVideoService(
		&mockDetector{result: domain.PlatformUnknown},
		&mockDownloader{},
		newLogger(),
		0,
	)

	_, _, err := svc.ProcessURL(context.Background(), "https://example.com/video")
	if !errors.Is(err, domain.ErrUnsupportedPlatform) {
		t.Errorf("expected ErrUnsupportedPlatform, got %v", err)
	}
}

func TestProcessURL_DownloadError(t *testing.T) {
	svc := usecase.NewVideoService(
		&mockDetector{result: domain.PlatformYouTube},
		&mockDownloader{err: domain.ErrDownloadFailed},
		newLogger(),
		0,
	)

	_, _, err := svc.ProcessURL(context.Background(), "https://youtube.com/watch?v=abc")
	if !errors.Is(err, domain.ErrDownloadFailed) {
		t.Errorf("expected ErrDownloadFailed, got %v", err)
	}
}

// Service no longer enforces file size — that's the handler's responsibility (it can offer Google Drive).
// This test verifies that large files are returned successfully so the handler can decide what to do.
func TestProcessURL_LargeVideoReturnedToHandler(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "test-*")
	tmpFile := filepath.Join(tmpDir, "video.mp4")
	_ = os.WriteFile(tmpFile, []byte("data"), 0644)

	svc := usecase.NewVideoService(
		&mockDetector{result: domain.PlatformYouTube},
		&mockDownloader{video: &domain.Video{
			FilePath: tmpFile,
			Size:     100 * 1024 * 1024, // 100MB
		}},
		newLogger(),
		50*1024*1024,
	)

	video, cleanup, err := svc.ProcessURL(context.Background(), "https://youtube.com/watch?v=abc")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defer cleanup()
	if video.Size != 100*1024*1024 {
		t.Errorf("expected size 100MB, got %d", video.Size)
	}
}

func TestProcessURL_Success(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "test-*")
	tmpFile := filepath.Join(tmpDir, "video.mp4")
	_ = os.WriteFile(tmpFile, []byte("data"), 0644)

	svc := usecase.NewVideoService(
		&mockDetector{result: domain.PlatformInstagram},
		&mockDownloader{video: &domain.Video{
			URL:      "https://instagram.com/reel/abc",
			FilePath: tmpFile,
			Size:     1024,
		}},
		newLogger(),
		0,
	)

	video, cleanup, err := svc.ProcessURL(context.Background(), "https://instagram.com/reel/abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()

	if video.Platform != domain.PlatformInstagram {
		t.Errorf("expected PlatformInstagram, got %v", video.Platform)
	}
	if video.Size != 1024 {
		t.Errorf("expected size 1024, got %d", video.Size)
	}
}
