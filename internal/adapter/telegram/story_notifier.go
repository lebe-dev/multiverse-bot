package telegram

import (
	"context"
	"fmt"
	"log/slog"

	tele "gopkg.in/telebot.v4"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

type storyNotifier struct {
	bot *tele.Bot
	log *slog.Logger
}

// NewStoryNotifier returns a domain.StoryNotifier that sends story
// notifications as Telegram messages with a download button.
func (b *Bot) NewStoryNotifier(log *slog.Logger) domain.StoryNotifier {
	return &storyNotifier{bot: b.bot, log: log}
}

func (n *storyNotifier) NotifyNewStory(_ context.Context, userID int64, story domain.StoryItem) error {
	text := fmt.Sprintf("📷 Новая сторис @%s", story.Username)

	// igs:{username}:{storyID}
	btn := tele.InlineButton{Text: "📥 Скачать", Data: "igs:" + story.Username + ":" + story.StoryID}
	markup := &tele.ReplyMarkup{
		InlineKeyboard: [][]tele.InlineButton{{btn}},
	}

	recipient := tele.ChatID(userID)
	if _, err := n.bot.Send(recipient, text, markup); err != nil {
		n.log.Error("failed to send story notification",
			"user_id", userID,
			"story_id", story.StoryID,
			"username", story.Username,
			"error", err,
		)
		return err
	}
	return nil
}
