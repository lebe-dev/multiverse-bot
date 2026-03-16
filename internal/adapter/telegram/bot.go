package telegram

import (
	"fmt"
	"log/slog"
	"time"

	tele "gopkg.in/telebot.v4"

	"gitlab.com/tiny-services/multiverse-bot/internal/usecase"
)

type Bot struct {
	bot         *tele.Bot
	service     *usecase.VideoService
	log         *slog.Logger
	adminIDs    map[string]struct{}
	version     string
	maxFileSize int64
	cookiesFile string
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
		bot:      b,
		service:  service,
		log:      log,
		adminIDs: make(map[string]struct{}),
	}, nil
}

func (b *Bot) SetConfig(version string, maxFileSize int64, cookiesFile string) {
	b.version = version
	b.maxFileSize = maxFileSize
	b.cookiesFile = cookiesFile
}

func (b *Bot) SetAdminUsers(admins []string) {
	for _, admin := range admins {
		b.adminIDs[admin] = struct{}{}
	}
}

func (b *Bot) NotifyAdminsStarted(version string) {
	if len(b.adminIDs) == 0 {
		return
	}

	msg := fmt.Sprintf("🚀 Запустилась версия %s", version)
	for admin := range b.adminIDs {
		recipient := &tele.Chat{Username: admin}
		if _, err := b.bot.Send(recipient, msg); err != nil {
			b.log.Error("failed to notify admin", "admin", admin, "error", err)
		}
	}
}

func (b *Bot) IsAdmin(username string) bool {
	_, exists := b.adminIDs[username]
	return exists
}

func (b *Bot) Start() {
	b.bot.Start()
}

func (b *Bot) Stop() {
	b.bot.Stop()
}
