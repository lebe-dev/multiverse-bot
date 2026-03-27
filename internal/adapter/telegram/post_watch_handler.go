package telegram

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tele "gopkg.in/telebot.v4"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
	"gitlab.com/tiny-services/multiverse-bot/internal/usecase"
)

// RegisterPostWatchHandlers registers /watch_instagram_posts command handler.
func (b *Bot) RegisterPostWatchHandlers(postWatchSvc *usecase.PostWatchService) {
	b.postWatchSvc = postWatchSvc
	b.bot.Handle("/watch_instagram_posts", b.handlePostWatch)
}

func (b *Bot) handlePostWatch(c tele.Context) error {
	payload := strings.TrimSpace(c.Message().Payload)
	if payload == "" {
		return b.handlePostWatchList(c)
	}
	return b.handlePostSubscribe(c, payload)
}

func (b *Bot) handlePostWatchList(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), watchTimeout)
	defer cancel()

	subs, err := b.postWatchSvc.ListSubscriptions(ctx, c.Sender().ID)
	if err != nil {
		b.log.Error("failed to list post subscriptions", "error", err)
		return c.Send("Не удалось получить список подписок.")
	}

	if len(subs) == 0 {
		return c.Send("У вас нет подписок на посты.\n\nОтправьте /watch_instagram_posts <url> для подписки на посты Instagram-аккаунта.")
	}

	text, kb := postWatchListMessage(subs)
	return c.Send(text, kb)
}

func (b *Bot) handlePostSubscribe(c tele.Context, input string) error {
	statusMsg, _ := b.bot.Send(c.Recipient(), "🔍 Проверяю аккаунт...")

	ctx, cancel := context.WithTimeout(context.Background(), watchTimeout)
	defer cancel()

	err := b.postWatchSvc.Subscribe(ctx, c.Sender().ID, input)

	if statusMsg != nil {
		_ = b.bot.Delete(statusMsg)
	}

	if err != nil {
		b.log.Warn("post subscribe failed",
			"user", c.Sender().Username,
			"user_id", c.Sender().ID,
			"input", input,
			"error", err,
		)
		return c.Send(postSubscribeError(err))
	}
	b.log.Info("post subscribed",
		"user", c.Sender().Username,
		"user_id", c.Sender().ID,
		"input", input,
	)
	return c.Send("✅ Подписка на посты оформлена! Вы будете получать уведомления о новых постах.\n\n⚠️ Посты, которые уже были опубликованы на момент подписки, не отправляются — только те, что появятся после.")
}

func postWatchListMessage(subs []domain.PostSubscription) (string, *tele.ReplyMarkup) {
	var rows [][]tele.InlineButton
	for _, sub := range subs {
		rows = append(rows, []tele.InlineButton{
			{Text: "❌ @" + sub.Username, Data: "post_rm:" + sub.Username},
		})
	}
	text := fmt.Sprintf("📸 Ваши подписки на посты (%d):\n\nНажмите для отписки.\n\nДобавить: `/watch\\-instagram\\-posts <url>`", len(subs))
	return text, &tele.ReplyMarkup{InlineKeyboard: rows}
}

func postSubscribeError(err error) string {
	switch {
	case errors.Is(err, domain.ErrAlreadySubscribedPost):
		return "Вы уже подписаны на посты этого аккаунта."
	case errors.Is(err, domain.ErrMaxSubscriptions):
		return "Достигнут лимит подписок. Отпишитесь от какого-нибудь аккаунта, чтобы добавить новый."
	case errors.Is(err, domain.ErrMaxChannels):
		return "Достигнут глобальный лимит отслеживаемых аккаунтов. Попробуйте позже."
	case errors.Is(err, domain.ErrUsernameNotFound):
		return "Instagram-аккаунт не найден.\n\nУбедитесь, что аккаунт существует и Instagram cookies загружены (`/add\\-instagram\\-cookies`)."
	default:
		return fmt.Sprintf("Не удалось подписаться: %v", err)
	}
}

func (b *Bot) handlePostUnsubscribeCallback(c tele.Context, username string) error {
	ctx, cancel := context.WithTimeout(context.Background(), watchTimeout)
	defer cancel()

	if err := b.postWatchSvc.Unsubscribe(ctx, c.Sender().ID, username); err != nil {
		b.log.Warn("post unsubscribe failed",
			"user", c.Sender().Username,
			"user_id", c.Sender().ID,
			"username", username,
			"error", err,
		)
		_ = c.Respond(&tele.CallbackResponse{Text: "Ошибка при отписке"})
		return nil
	}
	b.log.Info("post unsubscribed",
		"user", c.Sender().Username,
		"user_id", c.Sender().ID,
		"username", username,
	)
	_ = c.Respond(&tele.CallbackResponse{Text: "Отписка выполнена ✅"})

	subs, err := b.postWatchSvc.ListSubscriptions(ctx, c.Sender().ID)
	if err != nil || len(subs) == 0 {
		_, editErr := b.bot.Edit(c.Message(),
			"У вас нет подписок на посты.\n\nОтправьте `/watch\\-instagram\\-posts <url>` для подписки на посты Instagram-аккаунта.",
			&tele.ReplyMarkup{},
		)
		return editErr
	}

	text, kb := postWatchListMessage(subs)
	_, editErr := b.bot.Edit(c.Message(), text, kb)
	return editErr
}

func (b *Bot) handleInstagramPostDownloadCallback(c tele.Context, postID string) error {
	b.log.Info("instagram post download requested",
		"user", c.Sender().Username,
		"user_id", c.Sender().ID,
		"post_id", postID,
	)
	_ = c.Respond(&tele.CallbackResponse{Text: "⏳ Загружаю пост..."})

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
		defer cancel()

		url := "https://www.instagram.com/p/" + postID + "/"
		video, cleanup, err := b.service.ProcessURL(ctx, url)
		if err != nil {
			if _, sendErr := b.bot.Send(c.Sender(), "Не удалось скачать пост: "+friendlyDownloadError(err)); sendErr != nil {
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
			b.log.Error("failed to send video from ig post callback", "error", sendErr)
		}
	}()

	return nil
}
