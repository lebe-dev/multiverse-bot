package telegram

import (
	"context"
	"fmt"
	"log/slog"

	tele "gopkg.in/telebot.v4"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

type postNotifier struct {
	bot *tele.Bot
	log *slog.Logger
}

// NewPostNotifier returns a domain.PostNotifier that sends Instagram post
// notifications as Telegram messages with a download button.
func (b *Bot) NewPostNotifier(log *slog.Logger) domain.PostNotifier {
	return &postNotifier{bot: b.bot, log: log}
}

func (n *postNotifier) NotifyNewPost(_ context.Context, userID int64, post domain.PostItem) error {
	caption := truncateCaption(post.Title, 200)

	var text string
	if caption != "" {
		text = fmt.Sprintf("📸 Новый пост @%s\n\n%s\n\n🔗 %s", post.Username, caption, post.URL)
	} else {
		text = fmt.Sprintf("📸 Новый пост @%s\n\n🔗 %s", post.Username, post.URL)
	}

	btn := tele.InlineButton{Text: "📥 Скачать", Data: "igdl:" + post.PostID}
	markup := &tele.ReplyMarkup{
		InlineKeyboard: [][]tele.InlineButton{{btn}},
	}

	recipient := tele.ChatID(userID)
	if _, err := n.bot.Send(recipient, text, markup); err != nil {
		n.log.Error("failed to send new post notification",
			"user_id", userID,
			"post_id", post.PostID,
			"error", err,
		)
		return err
	}
	return nil
}

func truncateCaption(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "…"
}
