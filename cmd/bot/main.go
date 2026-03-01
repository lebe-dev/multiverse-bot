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
	ytdlpdl "gitlab.com/tiny-services/multiverse-bot/internal/adapter/downloader/ytdlp"
	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/telegram"
	"gitlab.com/tiny-services/multiverse-bot/internal/usecase"
)

const Version = "0.1.0"

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
	ytdlpDownloader := ytdlpdl.New()
	cobaltDownloader := cobalt.New(cfg.CobaltAPIURL)
	comp := composite.New(log, ytdlpDownloader, cobaltDownloader)

	svc := usecase.NewVideoService(det, comp, log, cfg.MaxFileSize)

	bot, err := telegram.New(cfg.TelegramToken, svc, log)
	if err != nil {
		log.Error("failed to create bot", "error", err)
		os.Exit(1)
	}

	bot.RegisterHandlers(cfg.AllowedUsers)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("bot started")
		bot.Start()
	}()

	<-ctx.Done()
	log.Info("shutting down")
	bot.Stop()
}
