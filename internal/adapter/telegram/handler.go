package telegram

import (
	"context"
	"errors"
	"fmt"
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
		return c.Send(
			"Welcome to Multiverse Bot\n\n" +
			"I can download videos from:\n" +
			"• YouTube\n" +
			"• Instagram\n" +
			"• X (Twitter)\n" +
			"• Threads\n\n" +
			"Just send me a link and I'll download the video.",
		)
	})

	b.bot.Handle("/config", b.handleConfigCommand)
	b.bot.Handle(tele.OnText, b.handleText)
}

func (b *Bot) handleText(c tele.Context) error {
	url := strings.TrimSpace(c.Text())
	if !isURL(url) {
		return c.Send("Please send a valid URL.")
	}

	// Send acknowledgment message
	statusMsg, err := c.Send("Processing your video...\nDetecting platform...")
	if err != nil {
		b.log.Error("failed to send status message", "error", err)
	}

	if err := c.Notify(tele.UploadingVideo); err != nil {
		b.log.Error("failed to send typing action", "error", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer cancel()

	// Create a progress callback to update the status message
	updateStatus := func(status string) {
		if statusMsg != nil {
			_, err := b.bot.Edit(statusMsg, "Processing your video...\n\n"+status)
			if err != nil {
				b.log.Debug("failed to update status", "error", err)
			}
		}
	}

	// Update status with platform information
	go func() {
		// Wait a moment for platform detection to complete, then update
		time.Sleep(500 * time.Millisecond)
		updateStatus("Downloading video...")
	}()

	video, cleanup, err := b.service.ProcessURL(ctx, url)
	if err != nil {
		// Try to delete the status message before sending error
		if statusMsg != nil {
			_ = b.bot.Delete(statusMsg)
		}
		return b.handleError(c, err)
	}
	defer cleanup()

	// Update status before sending video
	if statusMsg != nil {
		_ = b.bot.Delete(statusMsg)
	}

	if err := c.Notify(tele.UploadingDocument); err != nil {
		b.log.Error("failed to send upload action", "error", err)
	}

	return c.Send(&tele.Video{
		File: tele.FromDisk(video.FilePath),
	})
}

func (b *Bot) handleError(c tele.Context, err error) error {
	switch {
	case errors.Is(err, domain.ErrUnsupportedPlatform):
		return c.Send(
			"Unsupported platform\n\n" +
			"I support:\n" +
			"• YouTube\n" +
			"• Instagram\n" +
			"• X (Twitter)\n" +
			"• Threads\n\n" +
			"Please send a link from one of these platforms.",
		)
	case errors.Is(err, domain.ErrVideoTooLarge):
		return c.Send(
			"Video is too large (over 50MB)\n\n" +
			"Try:\n" +
			"• A shorter video\n" +
			"• Lower quality version\n" +
			"• A different source",
		)
	case errors.Is(err, domain.ErrDownloadFailed):
		b.log.Error("download failed", "error", err)
		return c.Send(
			"Download failed\n\n" +
			"The video couldn't be downloaded. This might happen if:\n" +
			"• The video is restricted or private\n" +
			"• The link is broken\n" +
			"• The platform blocked the request\n\n" +
			"Please try again with a different video.",
		)
	default:
		b.log.Error("unexpected error", "error", err)
		return c.Send(
			"Something went wrong\n\n" +
			"Please try again. If the problem persists, try with a different video.",
		)
	}
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func (b *Bot) handleConfigCommand(c tele.Context) error {
	username := c.Sender().Username
	if !b.IsAdmin(username) {
		return c.Send("❌ You don't have permission to use this command")
	}

	maxFileSizeMB := b.maxFileSize / (1024 * 1024)
	config := strings.Builder{}
	config.WriteString("⚙️ Bot Configuration\n\n")
	config.WriteString(fmt.Sprintf("Version: <code>%s</code>\n", b.version))
	config.WriteString(fmt.Sprintf("Max file size: <code>%d MB</code>\n", maxFileSizeMB))

	return c.Send(config.String(), &tele.SendOptions{ParseMode: tele.ModeHTML})
}
