package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	tele "gopkg.in/telebot.v4"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
	"gitlab.com/tiny-services/multiverse-bot/internal/usecase"
)

type telegramNotifier struct {
	bot *tele.Bot
	log *slog.Logger
}

// NewNotifier returns a domain.Notifier that sends Telegram messages.
// It has access to the internal *tele.Bot without exposing it outside the package.
func (b *Bot) NewNotifier(log *slog.Logger) domain.Notifier {
	return &telegramNotifier{bot: b.bot, log: log}
}

func (n *telegramNotifier) NotifyNewVideo(_ context.Context, userID int64, video domain.FeedVideo) error {
	text := fmt.Sprintf("🎬 Новое видео на %s\n\n%s\n\n🔗 %s",
		video.ChannelName,
		video.Title,
		video.URL,
	)

	btn := tele.InlineButton{Text: "📥 Скачать", Data: "dl:" + video.VideoID}
	markup := &tele.ReplyMarkup{
		InlineKeyboard: [][]tele.InlineButton{{btn}},
	}

	recipient := tele.ChatID(userID)
	if _, err := n.bot.Send(recipient, text, markup); err != nil {
		n.log.Error("failed to send new video notification",
			"user_id", userID,
			"video_id", video.VideoID,
			"error", err,
		)
		return err
	}
	return nil
}

// autoDownloadNotifier downloads the video and sends the file directly.
// Falls back to the regular notification with a button on download failure.
type autoDownloadNotifier struct {
	teleBot  *tele.Bot
	localBot *tele.Bot
	tgLimit  int64
	service  *usecase.VideoService
	fallback domain.Notifier
	log      *slog.Logger
}

// NewAutoDownloadNotifier returns a domain.Notifier that auto-downloads videos
// and sends them as files. On download failure it falls back to a regular
// notification with a "Download" button.
func (b *Bot) NewAutoDownloadNotifier(log *slog.Logger) domain.Notifier {
	return &autoDownloadNotifier{
		teleBot:  b.bot,
		localBot: b.localBot,
		tgLimit:  b.tgLimit,
		service:  b.service,
		fallback: b.NewNotifier(log),
		log:      log,
	}
}

const autoDownloadTimeout = 10 * time.Minute

func (n *autoDownloadNotifier) NotifyNewVideo(ctx context.Context, userID int64, video domain.FeedVideo) error {
	dlCtx, cancel := context.WithTimeout(ctx, autoDownloadTimeout)
	defer cancel()

	result, cleanup, err := n.service.ProcessURL(dlCtx, video.URL)
	if err != nil {
		n.log.Warn("auto-download failed, falling back to button notification",
			"user_id", userID,
			"video_id", video.VideoID,
			"error", err,
		)
		return n.fallback.NotifyNewVideo(ctx, userID, video)
	}
	defer cleanup()

	caption := fmt.Sprintf("🎬 %s\n%s", video.ChannelName, video.Title)
	recipient := tele.ChatID(userID)

	client := n.teleBot
	if result.Size > n.tgLimit && n.localBot != nil && result.Size < localBotAPIMaxSize {
		client = n.localBot
	}

	if _, err := client.Send(recipient, &tele.Video{
		File:    tele.FromDisk(result.FilePath),
		Caption: caption,
	}); err != nil {
		n.log.Error("failed to send auto-downloaded video",
			"user_id", userID,
			"video_id", video.VideoID,
			"error", err,
		)
		return err
	}
	return nil
}
