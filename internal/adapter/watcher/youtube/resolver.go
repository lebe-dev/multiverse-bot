package youtube

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

var (
	ucIDRe       = regexp.MustCompile(`^UC[a-zA-Z0-9_-]{22}$`)
	channelURLRe = regexp.MustCompile(`youtube\.com/channel/(UC[a-zA-Z0-9_-]{22})`)
	handleURLRe  = regexp.MustCompile(`youtube\.com/@([a-zA-Z0-9_.\-]+)`)
	customURLRe  = regexp.MustCompile(`youtube\.com/c/([a-zA-Z0-9_.\-]+)`)
)

type Resolver struct {
	ytdlpPath   string
	cookiesFile string
	fetcher     *FeedFetcher
}

func NewResolver(ytdlpPath, cookiesFile string) *Resolver {
	return &Resolver{
		ytdlpPath:   ytdlpPath,
		cookiesFile: cookiesFile,
		fetcher:     NewFeedFetcher(),
	}
}

func (r *Resolver) Resolve(ctx context.Context, input string) (string, string, error) {
	input = strings.TrimSpace(input)

	// Direct UC channel ID — validate via RSS feed (no yt-dlp needed).
	if ucIDRe.MatchString(input) {
		return r.validateAndName(ctx, input)
	}

	// Channel URL with UC ID embedded — extract and validate.
	if m := channelURLRe.FindStringSubmatch(input); len(m) > 1 {
		return r.validateAndName(ctx, m[1])
	}

	// Handle (@name), handle URL, custom URL — resolve via yt-dlp.
	var ytdlpURL string
	if m := handleURLRe.FindStringSubmatch(input); len(m) > 1 {
		ytdlpURL = "https://www.youtube.com/@" + m[1]
	} else if h, ok := strings.CutPrefix(input, "@"); ok {
		ytdlpURL = "https://www.youtube.com/@" + h
	} else if m := customURLRe.FindStringSubmatch(input); len(m) > 1 {
		ytdlpURL = "https://www.youtube.com/c/" + m[1]
	}

	if ytdlpURL != "" {
		return r.resolveViaYtdlp(ctx, ytdlpURL)
	}

	return "", "", domain.ErrChannelNotFound
}

func (r *Resolver) resolveViaYtdlp(ctx context.Context, url string) (string, string, error) {
	args := []string{"--print", "channel_id", "--print", "channel", "--playlist-items", "0", "--no-warnings"}
	if _, err := os.Stat(r.cookiesFile); err == nil {
		args = append(args, "--cookies", r.cookiesFile)
	}
	args = append(args, url)

	out, err := exec.CommandContext(ctx, r.ytdlpPath, args...).Output()
	if err != nil {
		return "", "", fmt.Errorf("%w: yt-dlp resolve failed", domain.ErrChannelNotFound)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 || !strings.HasPrefix(lines[0], "UC") {
		return "", "", fmt.Errorf("%w: channel ID not found", domain.ErrChannelNotFound)
	}
	return lines[0], lines[1], nil
}

func (r *Resolver) validateAndName(ctx context.Context, channelID string) (string, string, error) {
	videos, err := r.fetcher.FetchFeed(ctx, channelID)
	if err != nil {
		return "", "", fmt.Errorf("%w: %v", domain.ErrChannelNotFound, err)
	}

	name := channelID
	if len(videos) > 0 && videos[0].ChannelName != "" {
		name = videos[0].ChannelName
	}
	return channelID, name, nil
}
