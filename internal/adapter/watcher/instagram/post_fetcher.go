package instagram

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"time"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

// PostFetcher lists recent posts from an Instagram profile via yt-dlp.
type PostFetcher struct {
	ytdlpPath  string
	cookiePath func() string
	log        *slog.Logger
	limit      int
}

func NewPostFetcher(ytdlpPath string, cookiePath func() string, log *slog.Logger) *PostFetcher {
	return &PostFetcher{ytdlpPath: ytdlpPath, cookiePath: cookiePath, log: log, limit: 20}
}

func (f *PostFetcher) FetchPostIDs(ctx context.Context, username string) ([]domain.PostItem, error) {
	args := []string{
		"--flat-playlist", "-J", "--no-warnings",
		"--playlist-end", fmt.Sprintf("%d", f.limit),
	}
	hasCookies := false
	if cp := f.cookiePath(); cp != "" {
		args = append(args, "--cookies", cp)
		hasCookies = true
	}
	url := "https://www.instagram.com/" + username + "/"
	args = append(args, url)

	f.log.Debug("yt-dlp fetch post IDs", "username", username, "has_cookies", hasCookies)

	out, err := exec.CommandContext(ctx, f.ytdlpPath, args...).Output()
	if err != nil {
		f.log.Debug("yt-dlp fetch post IDs failed", "username", username, "error", err)
		return nil, fmt.Errorf("yt-dlp fetch posts for @%s: %w", username, err)
	}

	items, err := parsePostPlaylistJSON(out, username)
	if err != nil {
		return nil, err
	}
	f.log.Debug("yt-dlp fetch post IDs done", "username", username, "count", len(items))
	return items, nil
}

type postEntryJSON struct {
	ID        string  `json:"id"`
	URL       string  `json:"url"`
	Title     string  `json:"title"`
	Timestamp float64 `json:"timestamp"`
}

type postPlaylistJSON struct {
	Entries []postEntryJSON `json:"entries"`
}

func parsePostPlaylistJSON(data []byte, username string) ([]domain.PostItem, error) {
	var pl postPlaylistJSON
	if err := json.Unmarshal(data, &pl); err != nil {
		return nil, fmt.Errorf("parsing yt-dlp JSON: %w", err)
	}

	items := make([]domain.PostItem, 0, len(pl.Entries))
	for _, e := range pl.Entries {
		if e.ID == "" {
			continue
		}
		var ts time.Time
		if e.Timestamp > 0 {
			ts = time.Unix(int64(e.Timestamp), 0)
		}
		postURL := e.URL
		if postURL == "" {
			postURL = "https://www.instagram.com/p/" + e.ID + "/"
		}
		items = append(items, domain.PostItem{
			PostID:    e.ID,
			Username:  username,
			Title:     e.Title,
			URL:       postURL,
			Timestamp: ts,
		})
	}
	return items, nil
}
