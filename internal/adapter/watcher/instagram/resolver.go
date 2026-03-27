package instagram

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

var (
	profileURLRe = regexp.MustCompile(`(?i)instagram\.com/([a-zA-Z0-9_.]+)/?$`)
	usernameRe   = regexp.MustCompile(`^@?([a-zA-Z0-9_.]{1,30})$`)
)

var reservedPaths = map[string]struct{}{
	"p": {}, "reel": {}, "reels": {}, "tv": {}, "stories": {},
	"explore": {}, "accounts": {}, "direct": {}, "about": {},
}

type Resolver struct {
	ytdlpPath  string
	cookiePath func() string
}

func NewResolver(ytdlpPath string, cookiePath func() string) *Resolver {
	return &Resolver{ytdlpPath: ytdlpPath, cookiePath: cookiePath}
}

func (r *Resolver) Resolve(ctx context.Context, input string) (string, error) {
	input = strings.TrimSpace(input)

	username := extractUsername(input)
	if username == "" {
		return "", fmt.Errorf("%w: could not extract username from %q", domain.ErrUsernameNotFound, input)
	}

	if _, reserved := reservedPaths[strings.ToLower(username)]; reserved {
		return "", fmt.Errorf("%w: %q is a reserved path", domain.ErrUsernameNotFound, username)
	}

	if err := r.validate(ctx, username); err != nil {
		return "", err
	}

	return username, nil
}

func extractUsername(input string) string {
	if m := profileURLRe.FindStringSubmatch(input); len(m) > 1 {
		return m[1]
	}
	if m := usernameRe.FindStringSubmatch(input); len(m) > 1 {
		return m[1]
	}
	return ""
}

func (r *Resolver) validate(ctx context.Context, username string) error {
	args := []string{"--flat-playlist", "-J", "--no-warnings"}
	if cp := r.cookiePath(); cp != "" {
		args = append(args, "--cookies", cp)
	}
	args = append(args, "https://www.instagram.com/stories/"+username+"/")

	if err := exec.CommandContext(ctx, r.ytdlpPath, args...).Run(); err != nil {
		return fmt.Errorf("%w: yt-dlp validation failed for @%s", domain.ErrUsernameNotFound, username)
	}
	return nil
}
