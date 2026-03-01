package telegram

import (
	"context"
	"errors"
	"strings"
	"time"

	tele "gopkg.in/telebot.v4"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

const downloadTimeout = 5 * time.Minute

func (b *Bot) RegisterHandlers(allowedUsers []string) {
	if len(allowedUsers) > 0 {
		allowed := make(map[string]bool, len(allowedUsers))
		for _, u := range allowedUsers {
			allowed[strings.ToLower(u)] = true
		}
		b.bot.Use(whitelistMiddleware(allowed, b.log))
	}

	b.bot.Handle("/start", func(c tele.Context) error {
		return c.Send("Send me a video link from YouTube, Instagram, X/Twitter, or Threads.")
	})

	b.bot.Handle(tele.OnText, b.handleText)
}

func (b *Bot) handleText(c tele.Context) error {
	url := strings.TrimSpace(c.Text())
	if !isURL(url) {
		return c.Send("Please send a valid URL.")
	}

	if err := c.Notify(tele.UploadingVideo); err != nil {
		b.log.Error("failed to send typing action", "error", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer cancel()

	video, cleanup, err := b.service.ProcessURL(ctx, url)
	if err != nil {
		return b.handleError(c, err)
	}
	defer cleanup()

	return c.Send(&tele.Video{
		File: tele.FromDisk(video.FilePath),
	})
}

func (b *Bot) handleError(c tele.Context, err error) error {
	switch {
	case errors.Is(err, domain.ErrUnsupportedPlatform):
		return c.Send("Unsupported platform. I support YouTube, Instagram, X/Twitter, and Threads.")
	case errors.Is(err, domain.ErrVideoTooLarge):
		return c.Send("Video is too large (over 50MB). Try a shorter video or lower quality.")
	case errors.Is(err, domain.ErrDownloadFailed):
		b.log.Error("download failed", "error", err)
		return c.Send("Failed to download the video. Please try again later.")
	default:
		b.log.Error("unexpected error", "error", err)
		return c.Send("Something went wrong. Please try again.")
	}
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}
