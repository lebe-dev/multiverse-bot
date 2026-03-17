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

// RegisterWatchHandlers registers /watch command handler.
// Callback handling for watch buttons (dl:, watch_rm:) is in the unified
// handleCallback in handler.go.
func (b *Bot) RegisterWatchHandlers(watchSvc *usecase.WatchService) {
	b.watchSvc = watchSvc
	b.bot.Handle("/watch", b.handleWatch)
}

func (b *Bot) handleWatch(c tele.Context) error {
	payload := strings.TrimSpace(c.Message().Payload)
	if payload == "" {
		return b.handleWatchList(c)
	}
	return b.handleWatchSubscribe(c, payload)
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
		return c.Send("У вас нет активных подписок.\n\nОтправьте `/watch <url>` для подписки на YouTube-канал.")
	}

	var rows [][]tele.InlineButton
	for _, sub := range subs {
		rows = append(rows, []tele.InlineButton{
			{Text: "❌ " + sub.ChannelName, Data: "watch_rm:" + sub.ChannelID},
		})
	}

	return c.Send(
		fmt.Sprintf("📺 Ваши подписки (%d):\n\nНажмите на канал для отписки.\n\nДобавить: `/watch <url>`", len(subs)),
		&tele.ReplyMarkup{InlineKeyboard: rows},
	)
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
		return c.Send(watchSubscribeError(err))
	}
	return c.Send("✅ Подписка оформлена! Вы будете получать уведомления о новых видео.")
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

		if _, sendErr := b.bot.Send(c.Sender(), &tele.Video{File: tele.FromDisk(video.FilePath)}); sendErr != nil {
			b.log.Error("failed to send video from callback", "error", sendErr)
		}
	}()

	return nil
}

func (b *Bot) handleUnsubscribeCallback(c tele.Context, channelID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), watchTimeout)
	defer cancel()

	if err := b.watchSvc.Unsubscribe(ctx, c.Sender().ID, channelID); err != nil {
		_ = c.Respond(&tele.CallbackResponse{Text: "Ошибка при отписке"})
		return nil
	}
	_ = c.Respond(&tele.CallbackResponse{Text: "Отписка выполнена ✅"})

	subs, err := b.watchSvc.ListSubscriptions(ctx, c.Sender().ID)
	if err != nil || len(subs) == 0 {
		_, editErr := b.bot.Edit(c.Message(),
			"У вас нет активных подписок.\n\nОтправьте `/watch <url>` для подписки на YouTube-канал.",
			&tele.ReplyMarkup{},
		)
		return editErr
	}

	var rows [][]tele.InlineButton
	for _, sub := range subs {
		rows = append(rows, []tele.InlineButton{
			{Text: "❌ " + sub.ChannelName, Data: "watch_rm:" + sub.ChannelID},
		})
	}
	_, editErr := b.bot.Edit(c.Message(),
		fmt.Sprintf("📺 Ваши подписки (%d):\n\nНажмите на канал для отписки.\n\nДобавить: `/watch <url>`", len(subs)),
		&tele.ReplyMarkup{InlineKeyboard: rows},
	)
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
