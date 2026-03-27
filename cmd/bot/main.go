package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sync/errgroup"

	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/config"
	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/cookies"
	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/detector"
	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/downloader/cobalt"
	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/downloader/composite"
	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/downloader/lovethreads"
	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/downloader/savevids"
	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/downloader/threads"
	ytdlpdl "gitlab.com/tiny-services/multiverse-bot/internal/adapter/downloader/ytdlp"
	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/gdrive"
	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/plugin"
	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/store/sqlite"
	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/telegram"
	instagramwatcher "gitlab.com/tiny-services/multiverse-bot/internal/adapter/watcher/instagram"
	youtubewatcher "gitlab.com/tiny-services/multiverse-bot/internal/adapter/watcher/youtube"
	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
	"gitlab.com/tiny-services/multiverse-bot/internal/usecase"
)

const Version = "0.10.0"

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
		"debug", cfg.Debug,
	)

	// ── Startup cleanup ───────────────────────────────────────────────────────
	if stray, err := filepath.Glob("/tmp/multiverse-ytdlp-*"); err == nil {
		for _, dir := range stray {
			if rmErr := os.RemoveAll(dir); rmErr == nil {
				log.Info("cleaned stray temp dir", "path", dir)
			}
		}
	}

	// ── Database ──────────────────────────────────────────────────────────────
	store, err := sqlite.New(cfg.SQLitePath)
	if err != nil {
		log.Error("failed to open sqlite", "error", err)
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()

	// ── Cookie Manager ────────────────────────────────────────────────────────
	cookieMgr := cookies.NewManager(store)
	if err := cookieMgr.Materialize(context.Background()); err != nil {
		log.Error("failed to materialize cookies", "error", err)
		os.Exit(1)
	}
	defer cookieMgr.Cleanup()

	igCookiePath := func() string { return cookieMgr.CookieFilePath("instagram") }
	ytCookiePath := func() string { return cookieMgr.CookieFilePath("youtube") }
	ytdlpCookiePath := func(url string) string {
		if strings.Contains(url, "instagram.com") {
			return cookieMgr.CookieFilePath("instagram")
		}
		return cookieMgr.CookieFilePath("youtube")
	}

	// ── Downloaders ───────────────────────────────────────────────────────────
	det := detector.New()
	ytdlpDownloader := ytdlpdl.New(cfg.YtdlpPath, ytdlpCookiePath, log)
	cobaltDownloader := cobalt.New(cfg.CobaltAPIURL, log)

	var threadsDownloader domain.Downloader
	switch cfg.ThreadsEngine {
	case "lovethreads":
		threadsDownloader = lovethreads.New(log)
		log.Info("threads engine: lovethreads")
	default:
		threadsDownloader = threads.New(cfg.BrowserUserAgent, log)
		log.Info("threads engine: default (direct)")
	}

	var youtubeDownloader domain.Downloader
	switch cfg.YouTubeEngine {
	case "savevids":
		youtubeDownloader = savevids.New(cfg.TGLimit, log)
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

	svc := usecase.NewVideoService(det, comp, log, cfg.TGLimit)

	// ── Bot ───────────────────────────────────────────────────────────────────
	bot, err := telegram.New(cfg.TelegramToken, svc, log)
	if err != nil {
		log.Error("failed to create bot", "error", err)
		os.Exit(1)
	}

	// ── Bot configuration ─────────────────────────────────────────────────────
	// Must happen before creating notifiers — they capture tgLimit and localBot.
	bot.SetConfig(Version, cfg.TGLimit, cookieMgr, cfg.Debug)
	bot.SetQualityDownloader(ytdlpDownloader)
	bot.SetAdminUsers(cfg.AdminUsers)
	bot.SetAdminChatStore(telegram.NewAdminChatStore(cfg.AdminChatsFile, log))
	bot.SetSettings(telegram.NewSettingsStore(cfg.SettingsFile, log))

	if cfg.LocalBotAPIURL != "" {
		if err := bot.SetLocalBotAPI(cfg.LocalBotAPIURL); err != nil {
			log.Error("failed to configure local bot API", "error", err)
		} else {
			log.Info("local bot API enabled", "url", cfg.LocalBotAPIURL)
		}
	}

	// ── Per-user OAuth2 for Google Drive (Device Flow) ────────────────────────
	if cfg.GoogleClientID != "" && cfg.GoogleClientSecret != "" {
		tokenStore := gdrive.NewTokenStore(cfg.DriveTokensFile, log)
		driveMgr := gdrive.NewManager(cfg.GoogleClientID, cfg.GoogleClientSecret, tokenStore, log)
		bot.SetDrive(driveMgr)
		log.Info("google drive OAuth enabled (device flow)")
	}

	// ── Plugins ──────────────────────────────────────────────────────────────
	if cfg.PluginsConfig != "" {
		registry, err := plugin.LoadRegistry(cfg.PluginsConfig, log)
		if err != nil {
			log.Warn("failed to load plugins", "error", err)
		} else {
			bot.SetPlugins(registry)
			log.Info("plugins loaded", "count", len(registry.AllManifests()))
		}
	}

	// ── Watcher ───────────────────────────────────────────────────────────────
	feedFetcher := youtubewatcher.NewFeedFetcher()
	channelResolver := youtubewatcher.NewResolver(cfg.YtdlpPath, ytCookiePath)

	var notifier domain.Notifier
	if cfg.WatchAutoDownload {
		notifier = bot.NewAutoDownloadNotifier(log)
		log.Info("watch auto-download enabled")
	} else {
		notifier = bot.NewNotifier(log)
	}

	watchSvc := usecase.NewWatchService(
		store, feedFetcher, channelResolver, notifier, log,
		cfg.WatchPollInterval, cfg.WatchMaxSubs, cfg.WatchMaxChannelsTotal,
	)

	// ── Instagram Story Watcher ─────────────────────────────────────────────
	igFetcher := instagramwatcher.NewFetcher(cfg.YtdlpPath, igCookiePath, log)
	igResolver := instagramwatcher.NewResolver(cfg.YtdlpPath, igCookiePath, log)
	storyNotifier := bot.NewStoryNotifier(log)

	storyWatchSvc := usecase.NewStoryWatchService(
		store, igFetcher, igResolver, storyNotifier, log,
		cfg.WatchInstagramPollInterval, cfg.WatchMaxSubs, cfg.WatchMaxChannelsTotal,
	)

	// ── Instagram Post Watcher ──────────────────────────────────────────
	igPostFetcher := instagramwatcher.NewPostFetcher(cfg.YtdlpPath, igCookiePath, log)
	postNotifier := bot.NewPostNotifier(log)

	postWatchSvc := usecase.NewPostWatchService(
		store, igPostFetcher, igResolver, postNotifier, log,
		cfg.WatchInstagramPostsPollInterval, cfg.WatchMaxSubs, cfg.WatchMaxChannelsTotal,
	)

	bot.RegisterHandlers(cfg.AllowedUsers)
	bot.RegisterWatchHandlers(watchSvc)
	bot.RegisterStoryWatchHandlers(storyWatchSvc)
	bot.RegisterPostWatchHandlers(postWatchSvc)

	// ── Run ───────────────────────────────────────────────────────────────────
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error { return watchSvc.Run(gCtx) })
	g.Go(func() error { return storyWatchSvc.Run(gCtx) })
	g.Go(func() error { return postWatchSvc.Run(gCtx) })

	go func() {
		log.Info("bot started")
		bot.NotifyAdminsStarted(Version)
		bot.Start()
	}()

	<-ctx.Done()
	log.Info("shutting down")
	bot.Stop()

	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("watcher stopped with error", "error", err)
	}
}
