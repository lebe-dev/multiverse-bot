package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/config"
	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/detector"
	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/downloader/cobalt"
	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/downloader/composite"
	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/downloader/lovethreads"
	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/downloader/savevids"
	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/downloader/threads"
	ytdlpdl "gitlab.com/tiny-services/multiverse-bot/internal/adapter/downloader/ytdlp"
	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/gdrive"
	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/telegram"
	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
	"gitlab.com/tiny-services/multiverse-bot/internal/usecase"
)

const Version = "0.5.0"

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))

	log.Info("starting multiverse-bot",
		"version", Version,
		"tg_limit_mb", cfg.TGLimit/(1024*1024),
	)

	// ── Startup cleanup ───────────────────────────────────────────────────────
	// Remove stray temp dirs left by crashed/killed previous runs.
	if stray, err := filepath.Glob("/tmp/multiverse-ytdlp-*"); err == nil {
		for _, dir := range stray {
			if rmErr := os.RemoveAll(dir); rmErr == nil {
				log.Info("cleaned stray temp dir", "path", dir)
			}
		}
	}

	// ── Downloaders ───────────────────────────────────────────────────────────
	det := detector.New()
	ytdlpDownloader := ytdlpdl.New(cfg.YtdlpPath, cfg.CookiesFile, 0)
	cobaltDownloader := cobalt.New(cfg.CobaltAPIURL)

	var threadsDownloader domain.Downloader
	switch cfg.ThreadsEngine {
	case "lovethreads":
		threadsDownloader = lovethreads.New()
		log.Info("threads engine: lovethreads")
	default:
		threadsDownloader = threads.New(cfg.BrowserUserAgent)
		log.Info("threads engine: default (direct)")
	}

	var youtubeDownloader domain.Downloader
	switch cfg.YouTubeEngine {
	case "savevids":
		youtubeDownloader = savevids.New(0)
		log.Info("youtube engine: savevids")
	default:
		log.Info("youtube engine: yt-dlp")
	}

	backends := []domain.Downloader{threadsDownloader}
	if youtubeDownloader != nil {
		backends = append(backends, youtubeDownloader)
	}
	backends = append(backends, ytdlpDownloader, cobaltDownloader)
	comp := composite.New(log, backends...)

	svc := usecase.NewVideoService(det, comp, log, 0)

	// ── Bot ───────────────────────────────────────────────────────────────────
	bot, err := telegram.New(cfg.TelegramToken, svc, log)
	if err != nil {
		log.Error("failed to create bot", "error", err)
		os.Exit(1)
	}

	bot.SetConfig(Version, cfg.TGLimit, cfg.CookiesFile)
	bot.SetYtdlp(ytdlpDownloader)
	bot.SetAdminUsers(cfg.AdminUsers)
	bot.SetSettings(telegram.NewSettingsStore("./user_settings.json"))

	if cfg.LocalBotAPIURL != "" {
		if err := bot.SetLocalBotAPI(cfg.LocalBotAPIURL); err != nil {
			log.Error("failed to configure local bot API", "error", err)
		} else {
			log.Info("local bot API enabled", "url", cfg.LocalBotAPIURL)
		}
	}

	if cfg.GDriveFolderID != "" {
		bot.SetGDrive(gdrive.New(cfg.GDriveKeyFile, cfg.GDriveFolderID))
		log.Info("google drive enabled", "folder", cfg.GDriveFolderID)
	}

	// ── Per-user OAuth2 for Google Drive (Device Flow — no HTTP server needed) ─
	if cfg.GoogleClientID != "" && cfg.GoogleClientSecret != "" {
		tokenStore := gdrive.NewTokenStore("./user_drive_tokens.json")
		oauthMgr := gdrive.NewOAuthManager(cfg.GoogleClientID, cfg.GoogleClientSecret, tokenStore)
		bot.SetOAuth(oauthMgr)
		log.Info("google drive OAuth enabled (device flow)")
	}

	bot.RegisterHandlers(cfg.AllowedUsers)

	// ── Run ───────────────────────────────────────────────────────────────────
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("bot started")
		bot.NotifyAdminsStarted(Version)
		bot.Start()
	}()

	<-ctx.Done()
	log.Info("shutting down")
	bot.Stop()
}
