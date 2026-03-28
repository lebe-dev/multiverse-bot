package probe

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ApplyFaststart rewrites an MP4 file in-place with -movflags +faststart,
// moving the moov atom to the beginning so Telegram can stream it without
// downloading the full file first.
//
// Only processes .mp4 files; other formats are left untouched.
// On any error (ffmpeg missing, not an MP4, etc.) the original file is kept
// and the original size is returned — faststart is best-effort.
func ApplyFaststart(ctx context.Context, filePath string) int64 {
	if !strings.EqualFold(filepath.Ext(filePath), ".mp4") {
		if fi, err := os.Stat(filePath); err == nil {
			return fi.Size()
		}
		return 0
	}

	tmpPath := filePath + ".faststart.tmp"

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", filePath,
		"-c", "copy",
		"-movflags", "+faststart",
		"-y",
		tmpPath,
	)

	if err := cmd.Run(); err != nil {
		_ = os.Remove(tmpPath)
		if fi, err := os.Stat(filePath); err == nil {
			return fi.Size()
		}
		return 0
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		if fi, err := os.Stat(filePath); err == nil {
			return fi.Size()
		}
		return 0
	}

	fi, err := os.Stat(filePath)
	if err != nil {
		return 0
	}
	return fi.Size()
}
