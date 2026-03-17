package telegram

import (
	"context"
	"fmt"
	"log/slog"

	tele "gopkg.in/telebot.v4"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
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
