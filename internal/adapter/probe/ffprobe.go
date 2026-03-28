package probe

import (
	"context"
	"encoding/json"
	"os/exec"
	"strconv"
)

type streamInfo struct {
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Duration string `json:"duration"` // seconds as string, e.g. "123.456"
}

type formatInfo struct {
	Duration string `json:"duration"` // container-level duration
}

type probeResult struct {
	Streams []streamInfo `json:"streams"`
	Format  formatInfo   `json:"format"`
}

// VideoMeta returns width, height, and duration (seconds) of a video file.
// Returns (0, 0, 0) for any field that cannot be determined.
func VideoMeta(ctx context.Context, path string) (width, height, duration int) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,duration:format=duration",
		"-of", "json",
		path,
	)

	out, err := cmd.Output()
	if err != nil {
		return 0, 0, 0
	}

	var result probeResult
	if err := json.Unmarshal(out, &result); err != nil {
		return 0, 0, 0
	}

	if len(result.Streams) > 0 {
		width = result.Streams[0].Width
		height = result.Streams[0].Height
		if d, err := strconv.ParseFloat(result.Streams[0].Duration, 64); err == nil {
			duration = int(d)
		}
	}

	// Fall back to container-level duration if stream duration is missing.
	if duration == 0 {
		if d, err := strconv.ParseFloat(result.Format.Duration, 64); err == nil {
			duration = int(d)
		}
	}

	return width, height, duration
}

// VideoDimensions returns the width and height of a video file using ffprobe.
// Returns (0, 0) if dimensions cannot be determined.
//
// Deprecated: use VideoMeta which also returns duration.
func VideoDimensions(ctx context.Context, path string) (width, height int) {
	w, h, _ := VideoMeta(ctx, path)
	return w, h
}
