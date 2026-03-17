package telegram

import (
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	tele "gopkg.in/telebot.v4"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
	"gitlab.com/tiny-services/multiverse-bot/internal/usecase"
)

type Bot struct {
	bot      *tele.Bot
	localBot *tele.Bot // non-nil when LOCAL_BOT_API_URL is configured
	service  *usecase.VideoService
	watchSvc *usecase.WatchService
	log      *slog.Logger
	adminIDs map[string]struct{}

	qualityDl domain.QualityDownloader // for quality selection and format analysis
	drive     domain.DriveManager      // per-user Google Drive upload

	version     string
	tgLimit     int64
	cookiesFile string

	settings *SettingsStore
	lastURL  sync.Map // map[int64]string — last URL per user
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

func (b *Bot) SetLocalBotAPI(url string) error {
	lb, err := tele.NewBot(tele.Settings{
		Token:  b.bot.Token,
		URL:    url,
		Client: &http.Client{Timeout: 45 * time.Minute},
	})
	if err != nil {
		return fmt.Errorf("creating local bot API client: %w", err)
	}
	b.localBot = lb
	return nil
}

func (b *Bot) SetConfig(version string, tgLimit int64, cookiesFile string) {
	b.version = version
	b.tgLimit = tgLimit
	b.cookiesFile = cookiesFile
}

func (b *Bot) SetQualityDownloader(d domain.QualityDownloader) {
	b.qualityDl = d
}

func (b *Bot) SetDrive(d domain.DriveManager) {
	b.drive = d
}

func (b *Bot) SetSettings(s *SettingsStore) {
	b.settings = s
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

func (b *Bot) Start() { b.bot.Start() }
func (b *Bot) Stop()  { b.bot.Stop() }
