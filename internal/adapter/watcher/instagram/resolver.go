package instagram

import (
	"context"
	"fmt"
	"log/slog"
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
	log *slog.Logger
}

func NewResolver(_ string, _ func() string, log *slog.Logger) *Resolver {
	return &Resolver{log: log}
}

func (r *Resolver) Resolve(_ context.Context, input string) (string, error) {
	input = strings.TrimSpace(input)
	if i := strings.IndexByte(input, '?'); i != -1 {
		input = input[:i]
	}

	username := extractUsername(input)
	if username == "" {
		return "", fmt.Errorf("%w: could not extract username from %q", domain.ErrUsernameNotFound, input)
	}

	if _, reserved := reservedPaths[strings.ToLower(username)]; reserved {
		return "", fmt.Errorf("%w: %q is a reserved path", domain.ErrUsernameNotFound, username)
	}

	r.log.Debug("instagram username extracted", "input", input, "username", username)
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
