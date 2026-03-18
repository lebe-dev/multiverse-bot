package dlutil

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

func TestSaveToTemp_Success(t *testing.T) {
	content := []byte("fake video data")
	video, err := SaveToTemp("test-*", "https://example.com/video", func(f *os.File) error {
		_, err := f.Write(content)
		return err
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = os.RemoveAll(video.FilePath[:len(video.FilePath)-len("/video.mp4")]) }()

	if video.URL != "https://example.com/video" {
		t.Errorf("URL = %q, want %q", video.URL, "https://example.com/video")
	}
	if video.Size != int64(len(content)) {
		t.Errorf("Size = %d, want %d", video.Size, len(content))
	}
	if _, err := os.Stat(video.FilePath); err != nil {
		t.Errorf("file does not exist: %v", err)
	}
}

func TestSaveToTemp_WriteFnError(t *testing.T) {
	writeErr := fmt.Errorf("write failed")
	video, err := SaveToTemp("test-*", "https://example.com/video", func(f *os.File) error {
		return writeErr
	})
	if video != nil {
		t.Fatal("expected nil video on error")
	}
	if !errors.Is(err, domain.ErrDownloadFailed) {
		t.Errorf("expected ErrDownloadFailed, got %v", err)
	}
}

func TestSaveToTemp_CleansUpOnError(t *testing.T) {
	// Capture the temp dir path by writing a marker first.
	var tmpDir string
	_, _ = SaveToTemp("test-cleanup-*", "https://example.com", func(f *os.File) error {
		tmpDir = f.Name()[:len(f.Name())-len("/video.mp4")]
		return fmt.Errorf("intentional error")
	})

	if tmpDir == "" {
		t.Fatal("tmpDir was not captured")
	}
	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Errorf("temp dir should have been cleaned up, but still exists")
		_ = os.RemoveAll(tmpDir)
	}
}
