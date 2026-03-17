package youtube

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

var (
	ucIDRe       = regexp.MustCompile(`^UC[a-zA-Z0-9_-]{22}$`)
	channelURLRe = regexp.MustCompile(`youtube\.com/channel/(UC[a-zA-Z0-9_-]{22})`)
	handleURLRe  = regexp.MustCompile(`youtube\.com/@([a-zA-Z0-9_.\-]+)`)
	customURLRe  = regexp.MustCompile(`youtube\.com/c/([a-zA-Z0-9_.\-]+)`)
	channelIDRe  = regexp.MustCompile(`"channelId":"(UC[a-zA-Z0-9_-]{22})"`)
)

type Resolver struct {
	client    *http.Client
	userAgent string
	fetcher   *FeedFetcher
}

func NewResolver(userAgent string) *Resolver {
	return &Resolver{
		client:    &http.Client{Timeout: 10 * time.Second},
		userAgent: userAgent,
		fetcher:   NewFeedFetcher(),
	}
}

func (r *Resolver) Resolve(ctx context.Context, input string) (string, string, error) {
	input = strings.TrimSpace(input)

	if ucIDRe.MatchString(input) {
		return r.validateAndName(ctx, input)
	}

	if m := channelURLRe.FindStringSubmatch(input); len(m) > 1 {
		return r.validateAndName(ctx, m[1])
	}

	var handle string
	if m := handleURLRe.FindStringSubmatch(input); len(m) > 1 {
		handle = m[1]
	} else if h, ok := strings.CutPrefix(input, "@"); ok {
		handle = h
	}

	if handle != "" {
		return r.resolveHandle(ctx, handle)
	}

	if m := customURLRe.FindStringSubmatch(input); len(m) > 1 {
		return r.resolveHandle(ctx, m[1])
	}

	return "", "", domain.ErrChannelNotFound
}

func (r *Resolver) resolveHandle(ctx context.Context, handle string) (string, string, error) {
	pageURL := "https://www.youtube.com/@" + handle
	channelID, err := r.extractChannelIDFromPage(ctx, pageURL)
	if err != nil {
		return "", "", fmt.Errorf("%w: @%s not resolved (tip: use UC... channel ID directly as a reliable fallback)", domain.ErrChannelNotFound, handle)
	}
	return r.validateAndName(ctx, channelID)
}

func (r *Resolver) extractChannelIDFromPage(ctx context.Context, pageURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", r.userAgent)

	resp, err := r.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	m := channelIDRe.FindSubmatch(body)
	if len(m) < 2 {
		return "", fmt.Errorf("channel ID not found in page")
	}
	return string(m[1]), nil
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
