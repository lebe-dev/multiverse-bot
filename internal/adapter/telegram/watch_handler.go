package telegram

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tele "gopkg.in/telebot.v4"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
	"gitlab.com/tiny-services/multiverse-bot/internal/usecase"
)

const watchTimeout = 30 * time.Second

// RegisterWatchHandlers registers /watch_youtube command handler.
// Callback handling for watch buttons (dl:, watch_rm:) is in the unified
// handleCallback in handler.go.
func (b *Bot) RegisterWatchHandlers(watchSvc *usecase.WatchService) {
	b.watchSvc = watchSvc
	b.bot.Handle("/watch_youtube", b.handleWatch)
	b.bot.Handle("/watch", b.handleWatchAll)
	b.bot.Handle("/watch-youtube", func(c tele.Context) error {
		return c.Send("Команда переименована → /watch_youtube")
	})
}

func (b *Bot) handleWatch(c tele.Context) error {
	payload := strings.TrimSpace(c.Message().Payload)
	if payload == "" {
		return b.handleWatchList(c)
	}
	return b.handleWatchSubscribe(c, payload)
}

func (b *Bot) handleWatchAll(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), watchTimeout)
	defer cancel()

	ytSubs, err := b.watchSvc.ListSubscriptions(ctx, c.Sender().ID)
	if err != nil {
		b.log.Error("failed to list youtube subscriptions", "error", err)
		return c.Send("Не удалось получить список подписок.")
	}

	var storySubs []domain.StorySubscription
	if b.storyWatchSvc != nil {
		storySubs, err = b.storyWatchSvc.ListSubscriptions(ctx, c.Sender().ID)
		if err != nil {
			b.log.Error("failed to list story subscriptions", "error", err)
			return c.Send("Не удалось получить список подписок.")
		}
	}

	var postSubs []domain.PostSubscription
	if b.postWatchSvc != nil {
		postSubs, err = b.postWatchSvc.ListSubscriptions(ctx, c.Sender().ID)
		if err != nil {
			b.log.Error("failed to list post subscriptions", "error", err)
			return c.Send("Не удалось получить список подписок.")
		}
	}

	if len(ytSubs) == 0 && len(storySubs) == 0 && len(postSubs) == 0 {
		return c.Send("У вас нет активных подписок.\n\nДобавить:\n• /watch_youtube <url>\n• /watch_instagram_stories <url>\n• /watch_instagram_posts <url>")
	}

	text, kb := allWatchListMessage(ytSubs, storySubs, postSubs)
	return c.Send(text, kb)
}

func allWatchListMessage(ytSubs []domain.Subscription, storySubs []domain.StorySubscription, postSubs []domain.PostSubscription) (string, *tele.ReplyMarkup) {
	total := len(ytSubs) + len(storySubs) + len(postSubs)
	var rows [][]tele.InlineButton
	for _, sub := range ytSubs {
		rows = append(rows, []tele.InlineButton{
			{Text: "❌ " + sub.ChannelName + " (YouTube)", Data: "watch_rm:" + sub.ChannelID},
		})
	}
	for _, sub := range storySubs {
		rows = append(rows, []tele.InlineButton{
			{Text: "❌ @" + sub.Username + " (IG сторис)", Data: "story_rm:" + sub.Username},
		})
	}
	for _, sub := range postSubs {
		rows = append(rows, []tele.InlineButton{
			{Text: "❌ @" + sub.Username + " (IG посты)", Data: "post_rm:" + sub.Username},
		})
	}
	text := fmt.Sprintf("📋 Подписки (%d):\n\nНажмите для отписки.", total)
	return text, &tele.ReplyMarkup{InlineKeyboard: rows}
}

func (b *Bot) handleWatchList(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), watchTimeout)
	defer cancel()

	subs, err := b.watchSvc.ListSubscriptions(ctx, c.Sender().ID)
	if err != nil {
		b.log.Error("failed to list subscriptions", "error", err)
		return c.Send("Не удалось получить список подписок.")
	}

	if len(subs) == 0 {
		return c.Send("У вас нет активных подписок.\n\nОтправьте /watch_youtube <url> для подписки на YouTube-канал.")
	}

	text, kb := watchListMessage(subs)
	return c.Send(text, kb)
}

func (b *Bot) handleWatchSubscribe(c tele.Context, channelInput string) error {
	statusMsg, _ := b.bot.Send(c.Recipient(), "🔍 Ищу канал...")

	ctx, cancel := context.WithTimeout(context.Background(), watchTimeout)
	defer cancel()

	err := b.watchSvc.Subscribe(ctx, c.Sender().ID, channelInput)

	if statusMsg != nil {
		_ = b.bot.Delete(statusMsg)
	}

	if err != nil {
		b.log.Warn("subscribe failed",
			"user", c.Sender().Username,
			"user_id", c.Sender().ID,
			"channel", channelInput,
			"error", err,
		)
		return c.Send(watchSubscribeError(err))
	}
	b.log.Info("subscribed",
		"user", c.Sender().Username,
		"user_id", c.Sender().ID,
		"channel", channelInput,
	)
	return c.Send("✅ Подписка оформлена! Вы будете получать уведомления о новых видео.")
}

func watchListMessage(subs []domain.Subscription) (string, *tele.ReplyMarkup) {
	var rows [][]tele.InlineButton
	for _, sub := range subs {
		rows = append(rows, []tele.InlineButton{
			{Text: "❌ " + sub.ChannelName, Data: "watch_rm:" + sub.ChannelID},
		})
	}
	text := fmt.Sprintf("📺 Подписки на YouTube (%d):\n\nНажмите на канал для отписки.\n\nДобавить: `/watch\\-youtube <url>`", len(subs))
	return text, &tele.ReplyMarkup{InlineKeyboard: rows}
}

func watchSubscribeError(err error) string {
	switch {
	case errors.Is(err, domain.ErrAlreadySubscribed):
		return "Вы уже подписаны на этот канал."
	case errors.Is(err, domain.ErrMaxSubscriptions):
		return "Достигнут лимит подписок. Отпишитесь от какого-нибудь канала, чтобы добавить новый."
	case errors.Is(err, domain.ErrMaxChannels):
		return "Достигнут глобальный лимит отслеживаемых каналов. Попробуйте позже."
	case errors.Is(err, domain.ErrChannelNotFound):
		return fmt.Sprintf("Канал не найден.\n\n%v\n\nПопробуйте использовать ID канала (UC...) напрямую.", err)
	default:
		return fmt.Sprintf("Не удалось подписаться: %v", err)
	}
}

func (b *Bot) handleDownloadCallback(c tele.Context, videoID string) error {
	b.log.Info("watch download requested",
		"user", c.Sender().Username,
		"user_id", c.Sender().ID,
		"video", videoID,
	)
	_ = c.Respond(&tele.CallbackResponse{Text: "⏳ Загружаю видео..."})

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
		defer cancel()

		url := "https://www.youtube.com/watch?v=" + videoID
		video, cleanup, err := b.service.ProcessURL(ctx, url)
		if err != nil {
			if _, sendErr := b.bot.Send(c.Sender(), "Не удалось скачать видео: "+friendlyDownloadError(err)); sendErr != nil {
				b.log.Error("failed to send download error", "error", sendErr)
			}
			return
		}
		defer cleanup()

		st := b.userSettings(c.Sender().ID)
		caption := video.Title
		if !st.Caption {
			caption = ""
		}

		client := b.bot
		if video.Size > b.tgLimit && b.localBot != nil && video.Size < localBotAPIMaxSize {
			client = b.localBot
		}

		if sendErr := b.sendVideo(client, c, video.FilePath, caption); sendErr != nil {
			b.log.Error("failed to send video from callback", "error", sendErr)
		}
	}()

	return nil
}

func (b *Bot) handleUnsubscribeCallback(c tele.Context, channelID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), watchTimeout)
	defer cancel()

	if err := b.watchSvc.Unsubscribe(ctx, c.Sender().ID, channelID); err != nil {
		b.log.Warn("unsubscribe failed",
			"user", c.Sender().Username,
			"user_id", c.Sender().ID,
			"channel", channelID,
			"error", err,
		)
		_ = c.Respond(&tele.CallbackResponse{Text: "Ошибка при отписке"})
		return nil
	}
	b.log.Info("unsubscribed",
		"user", c.Sender().Username,
		"user_id", c.Sender().ID,
		"channel", channelID,
	)
	_ = c.Respond(&tele.CallbackResponse{Text: "Отписка выполнена ✅"})

	subs, err := b.watchSvc.ListSubscriptions(ctx, c.Sender().ID)
	if err != nil || len(subs) == 0 {
		_, editErr := b.bot.Edit(c.Message(),
			"У вас нет активных подписок.\n\nОтправьте `/watch\\-youtube <url>` для подписки на YouTube-канал.",
			&tele.ReplyMarkup{},
		)
		return editErr
	}

	text, kb := watchListMessage(subs)
	_, editErr := b.bot.Edit(c.Message(), text, kb)
	return editErr
}

func friendlyDownloadError(err error) string {
	switch {
	case errors.Is(err, domain.ErrVideoTooLarge):
		return "видео слишком большое (>50MB)"
	case errors.Is(err, domain.ErrUnsupportedPlatform):
		return "неподдерживаемая платформа"
	default:
		return "попробуйте позже"
	}
}
