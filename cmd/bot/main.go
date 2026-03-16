package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/config"
	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/detector"
	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/downloader/cobalt"
	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/downloader/composite"
	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/downloader/lovethreads"
	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/downloader/savevids"
	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/downloader/threads"
	ytdlpdl "gitlab.com/tiny-services/multiverse-bot/internal/adapter/downloader/ytdlp"
	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/telegram"
	"gitlab.com/tiny-services/multiverse-bot/internal/usecase"
)

const Version = "0.4.0"

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))

	log.Info("starting multiverse-bot", "version", Version)

	det := detector.New()
	ytdlpDownloader := ytdlpdl.New(cfg.YtdlpPath, cfg.CookiesFile, cfg.MaxFileSize)
	cobaltDownloader := cobalt.New(cfg.CobaltAPIURL)

	var threadsDownloader domain.Downloader
	switch cfg.ThreadsEngine {
	case "lovethreads":
		threadsDownloader = lovethreads.New()
		log.Info("using lovethreads.net engine for Threads")
	default:
		threadsDownloader = threads.New(cfg.BrowserUserAgent)
		log.Info("using default (direct) engine for Threads")
	}

	var youtubeDownloader domain.Downloader
	switch cfg.YouTubeEngine {
	case "savevids":
		youtubeDownloader = savevids.New(cfg.MaxFileSize)
		log.Info("using savevids engine for YouTube")
	default:
		log.Info("using default (yt-dlp) engine for YouTube")
	}

	backends := []domain.Downloader{threadsDownloader}
	if youtubeDownloader != nil {
		backends = append(backends, youtubeDownloader)
	}
	backends = append(backends, ytdlpDownloader, cobaltDownloader)

	comp := composite.New(log, backends...)

	svc := usecase.NewVideoService(det, comp, log, cfg.MaxFileSize)

	bot, err := telegram.New(cfg.TelegramToken, svc, log)
	if err != nil {
		log.Error("failed to create bot", "error", err)
		os.Exit(1)
	}

	bot.SetAdminUsers(cfg.AdminUsers)
	bot.SetConfig(Version, cfg.MaxFileSize, cfg.CookiesFile)
	bot.RegisterHandlers(cfg.AllowedUsers)

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
