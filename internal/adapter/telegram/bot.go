package telegram

import (
	"fmt"
	"log/slog"
	"time"

	tele "gopkg.in/telebot.v4"

	"gitlab.com/tiny-services/multiverse-bot/internal/usecase"
)

type Bot struct {
	bot     *tele.Bot
	service *usecase.VideoService
	log     *slog.Logger
}

func New(token string, service *usecase.VideoService, log *slog.Logger) (*Bot, error) {
	b, err := tele.NewBot(tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		return nil, fmt.Errorf("creating bot: %w", err)
	}

	return &Bot{
		bot:     b,
		service: service,
		log:     log,
	}, nil
}

func (b *Bot) Start() {
	b.bot.Start()
}

func (b *Bot) Stop() {
	b.bot.Stop()
}
