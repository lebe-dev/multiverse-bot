package dlutil

import (
	"fmt"
	"os"
	"path/filepath"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

// SaveToTemp creates a temp dir, writes content via writeFn, returns *domain.Video.
// On error, cleans up the temp dir automatically.
func SaveToTemp(prefix, sourceURL string, writeFn func(f *os.File) error) (*domain.Video, error) {
	tmpDir, err := os.MkdirTemp("", prefix)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}

	filePath := filepath.Join(tmpDir, "video.mp4")
	f, err := os.Create(filePath)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}

	if err := writeFn(f); err != nil {
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
		URL:      sourceURL,
		FilePath: filePath,
		Size:     info.Size(),
	}, nil
}
