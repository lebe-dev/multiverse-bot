package probe

import (
	"context"
	"encoding/json"
	"os/exec"
)

type streamInfo struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type probeResult struct {
	Streams []streamInfo `json:"streams"`
}

// VideoDimensions returns the width and height of a video file using ffprobe.
// Returns (0, 0) if dimensions cannot be determined.
func VideoDimensions(ctx context.Context, path string) (width, height int) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height",
		"-of", "json",
		path,
	)

	out, err := cmd.Output()
	if err != nil {
		return 0, 0
	}

	var result probeResult
	if err := json.Unmarshal(out, &result); err != nil {
		return 0, 0
	}

	if len(result.Streams) == 0 {
		return 0, 0
	}

	return result.Streams[0].Width, result.Streams[0].Height
}
