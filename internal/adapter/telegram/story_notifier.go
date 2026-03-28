package telegram

import (
	"context"
	"fmt"
	"log/slog"

	tele "gopkg.in/telebot.v4"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

type storyNotifier struct {
	bot      *tele.Bot
	localBot *tele.Bot
	tgLimit  int64
	log      *slog.Logger
}

// NewStoryNotifier returns a domain.StoryNotifier that sends story media via Telegram.
func (b *Bot) NewStoryNotifier(log *slog.Logger) domain.StoryNotifier {
	return &storyNotifier{
		bot:      b.bot,
		localBot: b.localBot,
		tgLimit:  b.tgLimit,
		log:      log,
	}
}

func (n *storyNotifier) NotifyNewStory(_ context.Context, userID int64, story domain.StoryMedia) error {
	caption := fmt.Sprintf("📷 Story @%s", story.Username)
	if story.Reshare != nil {
		link := "https://instagram.com/" + story.Reshare.Username
		if story.Reshare.StoryID != "" {
			link = fmt.Sprintf("https://instagram.com/stories/%s/%s/", story.Reshare.Username, story.Reshare.StoryID)
		}
		caption += fmt.Sprintf("\n\n↩️ @%s\n%s", story.Reshare.Username, link)
	}
	recipient := tele.ChatID(userID)

	client := n.bot
	if story.Size > n.tgLimit && n.localBot != nil {
		client = n.localBot
	}

	var err error
	if story.Type == domain.MediaVideo {
		_, err = client.Send(recipient, &tele.Video{
			File:    tele.FromDisk(story.FilePath),
			Caption: caption,
		})
	} else {
		_, err = client.Send(recipient, &tele.Photo{
			File:    tele.FromDisk(story.FilePath),
			Caption: caption,
		})
	}

	if err != nil {
		n.log.Error("failed to send story notification",
			"user_id", userID,
			"story_id", story.StoryID,
			"username", story.Username,
			"error", err,
		)
	}
	return err
}
