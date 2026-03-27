package instagram

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

type Fetcher struct {
	ytdlpPath  string
	cookiePath func() string
	log        *slog.Logger
}

func NewFetcher(ytdlpPath string, cookiePath func() string, log *slog.Logger) *Fetcher {
	return &Fetcher{ytdlpPath: ytdlpPath, cookiePath: cookiePath, log: log}
}

type playlistJSON struct {
	Entries []entryJSON `json:"entries"`
}

type entryJSON struct {
	ID        string  `json:"id"`
	URL       string  `json:"url"`
	Timestamp float64 `json:"timestamp"`
}

func (f *Fetcher) FetchStoryIDs(ctx context.Context, username string) ([]domain.StoryItem, error) {
	args := []string{"--flat-playlist", "-J", "--no-warnings"}
	if cp := f.cookiePath(); cp != "" {
		args = append(args, "--cookies", cp)
	}
	args = append(args, "https://www.instagram.com/stories/"+username+"/")

	out, err := exec.CommandContext(ctx, f.ytdlpPath, args...).Output()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp fetch stories for @%s: %w", username, err)
	}

	return parsePlaylistJSON(out, username)
}

func parsePlaylistJSON(data []byte, username string) ([]domain.StoryItem, error) {
	var pl playlistJSON
	if err := json.Unmarshal(data, &pl); err != nil {
		return nil, fmt.Errorf("parsing yt-dlp JSON: %w", err)
	}

	items := make([]domain.StoryItem, 0, len(pl.Entries))
	for _, e := range pl.Entries {
		if e.ID == "" {
			continue
		}
		var ts time.Time
		if e.Timestamp > 0 {
			ts = time.Unix(int64(e.Timestamp), 0)
		}
		items = append(items, domain.StoryItem{
			StoryID:   e.ID,
			Username:  username,
			Timestamp: ts,
		})
	}
	return items, nil
}

func (f *Fetcher) DownloadStory(ctx context.Context, username string, storyID string) (*domain.StoryMedia, error) {
	tmpDir, err := os.MkdirTemp("", "multiverse-igstory-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}

	outTmpl := filepath.Join(tmpDir, "%(id)s.%(ext)s")
	storyURL := fmt.Sprintf("https://www.instagram.com/stories/%s/%s/", username, storyID)

	args := []string{
		"--no-playlist",
		"--no-warnings",
		"-o", outTmpl,
	}
	if cp := f.cookiePath(); cp != "" {
		args = append(args, "--cookies", cp)
	}
	args = append(args, "--js-runtimes", "node")
	args = append(args, storyURL)

	if out, err := exec.CommandContext(ctx, f.ytdlpPath, args...).CombinedOutput(); err != nil {
		_ = os.RemoveAll(tmpDir)
		f.log.Debug("yt-dlp download story failed", "story", storyID, "output", string(out))
		return nil, fmt.Errorf("yt-dlp download story %s: %w", storyID, err)
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil || len(entries) == 0 {
		_ = os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("no files downloaded for story %s", storyID)
	}

	filePath := filepath.Join(tmpDir, entries[0].Name())
	info, err := os.Stat(filePath)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("stat downloaded file: %w", err)
	}

	mediaType := detectMediaType(filePath)

	return &domain.StoryMedia{
		StoryID:  storyID,
		Username: username,
		FilePath: filePath,
		Type:     mediaType,
		Size:     info.Size(),
	}, nil
}

func detectMediaType(filePath string) domain.MediaType {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".mp4", ".mkv", ".webm", ".mov":
		return domain.MediaVideo
	default:
		return domain.MediaPhoto
	}
}
