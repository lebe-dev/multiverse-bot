package telegram

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tele "gopkg.in/telebot.v4"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
	"gitlab.com/tiny-services/multiverse-bot/internal/usecase"
)

// RegisterStoryWatchHandlers registers /watch_instagram_stories command handler.
func (b *Bot) RegisterStoryWatchHandlers(storyWatchSvc *usecase.StoryWatchService) {
	b.storyWatchSvc = storyWatchSvc
	b.bot.Handle("/watch_instagram_stories", b.handleStoryWatch)
	b.bot.Handle("/watch-instagram-stories", func(c tele.Context) error {
		return c.Send("Команда переименована → /watch_instagram_stories")
	})
}

func (b *Bot) handleStoryWatch(c tele.Context) error {
	payload := strings.TrimSpace(c.Message().Payload)
	if payload == "" {
		return b.handleStoryWatchList(c)
	}
	return b.handleStorySubscribe(c, payload)
}

func (b *Bot) handleStoryWatchList(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), watchTimeout)
	defer cancel()

	subs, err := b.storyWatchSvc.ListSubscriptions(ctx, c.Sender().ID)
	if err != nil {
		b.log.Error("failed to list story subscriptions", "error", err)
		return c.Send("Не удалось получить список подписок.")
	}

	if len(subs) == 0 {
		return c.Send("У вас нет подписок на сторис.\n\nОтправьте /watch_instagram_stories <url> для подписки на сторис Instagram-аккаунта.")
	}

	text, kb := storyWatchListMessage(subs)
	return c.Send(text, kb)
}

func (b *Bot) handleStorySubscribe(c tele.Context, input string) error {
	statusMsg, _ := b.bot.Send(c.Recipient(), "🔍 Проверяю аккаунт...")

	ctx, cancel := context.WithTimeout(context.Background(), watchTimeout)
	defer cancel()

	err := b.storyWatchSvc.Subscribe(ctx, c.Sender().ID, input)

	if statusMsg != nil {
		_ = b.bot.Delete(statusMsg)
	}

	if err != nil {
		b.log.Warn("story subscribe failed",
			"user", c.Sender().Username,
			"user_id", c.Sender().ID,
			"input", input,
			"error", err,
		)
		return c.Send(storySubscribeError(err))
	}
	b.log.Info("story subscribed",
		"user", c.Sender().Username,
		"user_id", c.Sender().ID,
		"input", input,
	)
	return c.Send("✅ Подписка на сторис оформлена! Вы будете получать новые сторис этого аккаунта.\n\n⚠️ Сторис, которые уже были опубликованы на момент подписки, не отправляются — только те, что появятся после.")
}

func storyWatchListMessage(subs []domain.StorySubscription) (string, *tele.ReplyMarkup) {
	var rows [][]tele.InlineButton
	for _, sub := range subs {
		rows = append(rows, []tele.InlineButton{
			{Text: "❌ @" + sub.Username, Data: "story_rm:" + sub.Username},
		})
	}
	text := fmt.Sprintf("📷 Ваши подписки на сторис (%d):\n\nНажмите для отписки.\n\nДобавить: `/watch\\-instagram\\-stories <url>`", len(subs))
	return text, &tele.ReplyMarkup{InlineKeyboard: rows}
}

func storySubscribeError(err error) string {
	switch {
	case errors.Is(err, domain.ErrAlreadySubscribedStory):
		return "Вы уже подписаны на сторис этого аккаунта."
	case errors.Is(err, domain.ErrMaxSubscriptions):
		return "Достигнут лимит подписок. Отпишитесь от какого-нибудь аккаунта, чтобы добавить новый."
	case errors.Is(err, domain.ErrMaxChannels):
		return "Достигнут глобальный лимит отслеживаемых аккаунтов. Попробуйте позже."
	case errors.Is(err, domain.ErrUsernameNotFound):
		return "Instagram-аккаунт не найден или нет доступа к его сторис.\n\nУбедитесь, что аккаунт существует и Instagram cookies загружены (`/add\\-instagram\\-cookies`)."
	default:
		return fmt.Sprintf("Не удалось подписаться: %v", err)
	}
}

func (b *Bot) handleStoryDownloadCallback(c tele.Context, username, storyID string) error {
	b.log.Info("instagram story download requested",
		"user", c.Sender().Username,
		"user_id", c.Sender().ID,
		"username", username,
		"story_id", storyID,
	)
	_ = c.Respond(&tele.CallbackResponse{Text: "⏳ Загружаю сторис..."})

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
		defer cancel()

		fetcher := b.storyWatchSvc.Fetcher()
		media, err := fetcher.DownloadStory(ctx, username, storyID)
		if err != nil {
			if _, sendErr := b.bot.Send(c.Sender(), "Не удалось скачать сторис: "+friendlyDownloadError(err)); sendErr != nil {
				b.log.Error("failed to send story download error", "error", sendErr)
			}
			return
		}
		defer func() {
			if media.FilePath != "" {
				_ = os.RemoveAll(filepath.Dir(media.FilePath))
			}
		}()

		// Enrich with reshare metadata if available.
		if enricher := b.storyWatchSvc.Enricher(); enricher != nil {
			enrichCtx, enrichCancel := context.WithTimeout(ctx, 15*time.Second)
			reshare, err := enricher.EnrichStoryMetadata(enrichCtx, username, storyID)
			enrichCancel()
			if err != nil {
				b.log.Warn("failed to enrich story metadata", "username", username, "story_id", storyID, "error", err)
			} else if reshare != nil {
				media.Reshare = reshare
			}
		}

		caption := fmt.Sprintf("📷 Story @%s", username)
		if media.Reshare != nil {
			link := "https://instagram.com/" + media.Reshare.Username
			if media.Reshare.StoryID != "" {
				link = fmt.Sprintf("https://instagram.com/stories/%s/%s/", media.Reshare.Username, media.Reshare.StoryID)
			}
			caption += fmt.Sprintf("\n\n↩️ @%s\n%s", media.Reshare.Username, link)
		}

		client := b.bot
		if media.Size > b.tgLimit && b.localBot != nil {
			client = b.localBot
		}

		var sendErr error
		if media.Type == domain.MediaVideo {
			sendErr = b.sendVideo(client, c, media.FilePath, caption)
		} else {
			sendErr = b.sendPhoto(client, c, media.FilePath, caption)
		}
		if sendErr != nil {
			b.log.Error("failed to send story from callback", "error", sendErr)
		}
	}()

	return nil
}

func (b *Bot) handleStoryUnsubscribeCallback(c tele.Context, username string) error {
	ctx, cancel := context.WithTimeout(context.Background(), watchTimeout)
	defer cancel()

	if err := b.storyWatchSvc.Unsubscribe(ctx, c.Sender().ID, username); err != nil {
		b.log.Warn("story unsubscribe failed",
			"user", c.Sender().Username,
			"user_id", c.Sender().ID,
			"username", username,
			"error", err,
		)
		_ = c.Respond(&tele.CallbackResponse{Text: "Ошибка при отписке"})
		return nil
	}
	b.log.Info("story unsubscribed",
		"user", c.Sender().Username,
		"user_id", c.Sender().ID,
		"username", username,
	)
	_ = c.Respond(&tele.CallbackResponse{Text: "Отписка выполнена ✅"})

	subs, err := b.storyWatchSvc.ListSubscriptions(ctx, c.Sender().ID)
	if err != nil || len(subs) == 0 {
		_, editErr := b.bot.Edit(c.Message(),
			"У вас нет подписок на сторис.\n\nОтправьте `/watch\\-instagram\\-stories <url>` для подписки на сторис Instagram-аккаунта.",
			&tele.ReplyMarkup{},
		)
		return editErr
	}

	text, kb := storyWatchListMessage(subs)
	_, editErr := b.bot.Edit(c.Message(), text, kb)
	return editErr
}
