package telegram

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	tele "gopkg.in/telebot.v4"

	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/probe"
	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

const (
	downloadTimeout    = 10 * time.Minute
	saveTimeout        = 30 * time.Minute
	analyzeTimeout     = 45 * time.Second
	localBotAPIMaxSize = 2000 * 1024 * 1024 // 2 GB
	minDiskSpaceBytes  = 1024 * 1024 * 1024 // 1 GB
)

func (b *Bot) RegisterHandlers(allowedUsers []string) {
	if len(allowedUsers) > 0 {
		allowed := make(map[string]bool, len(allowedUsers))
		for _, u := range allowedUsers {
			allowed[strings.ToLower(u)] = true
		}
		b.bot.Use(whitelistMiddleware(allowed, b.log))
	}

	b.bot.Use(requestLogMiddleware(b.log))

	// Track admin chat IDs so we can send startup notifications.
	if len(b.adminIDs) > 0 && b.adminChats != nil {
		b.bot.Use(b.adminTrackMiddleware())
	}

	b.bot.Handle("/start", b.handleStartCommand)
	b.bot.Handle("/help", b.handleHelpCommand)
	b.bot.Handle("/settings", b.handleSettingsCommand)
	b.bot.Handle("/details", b.handleDetailsCommand)
	b.bot.Handle("/audio", b.handleAudioCommand)
	b.bot.Handle("/save", b.handleSaveCommand)
	b.bot.Handle("/drive", b.handleDriveCommand)
	b.bot.Handle("/admin", b.handleAdminCommand)

	b.bot.Handle("/add_youtube_cookies", b.handleAddCookies("youtube"))
	b.bot.Handle("/delete_youtube_cookies", b.handleDeleteCookies("youtube"))
	if b.instagramEnabled {
		b.bot.Handle("/add_instagram_cookies", b.handleAddCookies("instagram"))
		b.bot.Handle("/delete_instagram_cookies", b.handleDeleteCookies("instagram"))
	}

	// Legacy redirects (one release cycle).
	b.bot.Handle("/auth", func(c tele.Context) error { return c.Send("Команда перенесена → /drive") })
	b.bot.Handle("/disconnect", func(c tele.Context) error { return c.Send("Команда перенесена → /drive") })
	b.bot.Handle("/config", func(c tele.Context) error { return c.Send("Команда перенесена → /admin") })
	b.bot.Handle("/cookies", func(c tele.Context) error {
		return c.Send("Команда удалена. Используйте /add_youtube_cookies или /add_instagram_cookies")
	})
	b.bot.Handle("/add-youtube-cookies", func(c tele.Context) error {
		return c.Send("Команда переименована → /add_youtube_cookies")
	})
	b.bot.Handle("/add-instagram-cookies", func(c tele.Context) error {
		return c.Send("Команда переименована → /add_instagram_cookies")
	})

	// Register plugin commands.
	if b.plugins != nil {
		for _, m := range b.plugins.AllManifests() {
			for _, cmd := range m.Commands {
				pluginName := m.Name
				command := cmd.Command
				b.bot.Handle(command, func(c tele.Context) error {
					return b.handlePluginCommand(c, pluginName, command)
				})
			}
		}
	}

	b.bot.Handle(tele.OnDocument, b.handleDocument)
	b.bot.Handle(tele.OnText, b.handleText)
	b.bot.Handle(tele.OnCallback, b.handleCallback)
}

func (b *Bot) handleStartCommand(c tele.Context) error {
	return c.Send(b.buildHelpText(c))
}

func (b *Bot) handleHelpCommand(c tele.Context) error {
	return c.Send(b.buildHelpText(c))
}

func (b *Bot) buildHelpText(c tele.Context) string {
	platforms := "YouTube, X (Twitter), Threads"
	if b.instagramEnabled {
		platforms = "YouTube, Instagram, X (Twitter), Threads"
	}

	msg := "Multiverse Bot\n\n" +
		"Платформы: " + platforms + "\n\n" +
		"Отправьте ссылку — бот скачает и пришлёт видео.\n\n" +
		"Команды:\n" +
		"/settings — настройки (качество, подпись)\n" +
		"/watch_youtube <url> — подписаться на YouTube-канал\n"

	if b.instagramEnabled {
		msg += "/watch_instagram_stories <url> — подписаться на сторис\n" +
			"/watch_instagram_posts <url> — подписаться на посты\n"
	}

	msg += "/export — экспорт подписок и настроек\n" +
		"/import — импорт подписок и настроек\n" +
		"/details <url> — доступные форматы и размеры\n" +
		"/audio <url> — скачать только аудио (YouTube)\n" +
		"/save [url] — сохранить в Google Drive\n" +
		"/drive — управление Google Drive\n" +
		"/help — список команд\n"

	if b.plugins != nil {
		for _, m := range b.plugins.AllManifests() {
			for _, cmd := range m.Commands {
				msg += fmt.Sprintf("%s — %s\n", cmd.Command, cmd.Description)
			}
		}
	}

	if b.IsAdmin(c.Sender().Username) {
		msg += "\nАдмин:\n" +
			"/admin — панель администратора\n" +
			"/add_youtube_cookies — загрузить YouTube cookies\n" +
			"/delete_youtube_cookies — удалить YouTube cookies\n"
		if b.instagramEnabled {
			msg += "/add_instagram_cookies — загрузить Instagram cookies\n" +
				"/delete_instagram_cookies — удалить Instagram cookies\n"
		}
	}
	return msg
}

// ── Main video handler ────────────────────────────────────────────────────────

type downloadResult struct {
	filePath string
	fileSize int64
	title    string
	channel  string
	cleanup  func()
}

// downloadForURL tries quality downloader for YouTube, then composite, then plugins.
// Returns errPluginHandled when a plugin took over (caller should return nil).
func (b *Bot) downloadForURL(ctx context.Context, c tele.Context, url, quality string) (*downloadResult, error) {
	platform := b.service.DetectPlatform(url)
	b.log.Info("processing url",
		"user", c.Sender().Username,
		"user_id", c.Sender().ID,
		"url", url,
		"platform", platform.String(),
		"quality", quality,
	)

	// Try quality downloader for YouTube first.
	if platform == domain.PlatformYouTube && b.qualityDl != nil {
		ytVideo, err := b.qualityDl.DownloadQuality(ctx, url, quality)
		if err == nil {
			dir := filepath.Dir(ytVideo.FilePath)
			return &downloadResult{
				filePath: ytVideo.FilePath,
				fileSize: ytVideo.Size,
				title:    ytVideo.Title,
				channel:  ytVideo.Channel,
				cleanup:  func() { _ = os.RemoveAll(dir) },
			}, nil
		}
		b.log.Warn("quality download failed, falling back to composite", "error", err)
	}

	// Fallback: composite downloader.
	video, svcCleanup, svcErr := b.service.ProcessURL(ctx, url)
	if svcErr != nil {
		// If built-in fails with ErrUnsupportedPlatform, try plugins.
		if errors.Is(svcErr, domain.ErrUnsupportedPlatform) && b.plugins != nil {
			if pluginClient, pluginName, matchedPattern := b.plugins.FindByURL(url); pluginClient != nil {
				return nil, pluginHandledError{c: c, client: pluginClient, name: pluginName, url: url, pattern: matchedPattern}
			}
		}
		return nil, svcErr
	}

	return &downloadResult{
		filePath: video.FilePath,
		fileSize: video.Size,
		title:    video.Title,
		channel:  video.Channel,
		cleanup:  svcCleanup,
	}, nil
}

// pluginHandledError signals that a plugin matched the URL and should handle the response.
type pluginHandledError struct {
	c       tele.Context
	client  domain.PluginClient
	name    string
	url     string
	pattern string
}

func (e pluginHandledError) Error() string { return "plugin handled" }

func (b *Bot) handleText(c tele.Context) error {
	// Check for pending cookies text paste.
	if val, ok := b.pendingCookies.LoadAndDelete(c.Sender().ID); ok {
		return b.saveCookiesFromText(c, val.(string))
	}

	url := extractURL(c.Text())
	if url == "" {
		return c.Send("Пожалуйста, отправьте корректную ссылку.")
	}

	if free, err := freeDiskBytes("."); err == nil && free < minDiskSpaceBytes {
		b.log.Warn("low disk space", "free_mb", free/(1024*1024))
		return c.Send(fmt.Sprintf("⚠️ Мало места на диске (%d МБ). Попробуйте позже.", free/(1024*1024)))
	}

	st := b.userSettings(c.Sender().ID)
	quality := st.Quality

	// YouTube with quality downloader — separate path (preserves existing logic).
	platform := b.service.DetectPlatform(url)
	if platform == domain.PlatformYouTube && b.qualityDl != nil {
		startedAt := time.Now()
		statusMsg, _ := b.bot.Send(c.Recipient(), fmt.Sprintf("⏳ Скачиваю %s...", quality))

		dlCtx, dlCancel := context.WithTimeout(context.Background(), downloadTimeout)
		defer dlCancel()

		result, err := b.downloadForURL(dlCtx, c, url, quality)
		if err != nil {
			b.deleteMsg(statusMsg)
			var pe pluginHandledError
			if errors.As(err, &pe) {
				return b.handlePluginURL(pe.c, pe.client, pe.name, pe.url, pe.pattern)
			}
			return b.handleError(c, err)
		}

		sizeMB := result.fileSize / (1024 * 1024)
		downloadedAt := time.Now()
		b.log.Info("downloaded",
			"user", c.Sender().Username,
			"user_id", c.Sender().ID,
			"url", url,
			"size_mb", sizeMB,
			"quality", quality,
			"download_s", int(downloadedAt.Sub(startedAt).Seconds()),
		)

		b.deleteMsg(statusMsg)
		b.lastURL.Store(c.Sender().ID, url)

		caption := result.title
		if !st.Caption {
			caption = ""
		} else {
			if result.channel != "" {
				caption = "🎬 " + result.channel + "\n" + caption
			}
			if url != "" {
				caption += "\n\n" + url
			}
		}

		var sendErr error
		if result.fileSize <= b.tgLimit {
			defer result.cleanup()
			sendErr = b.sendVideo(b.bot, c, result.filePath, caption)
		} else if b.localBot != nil && result.fileSize < localBotAPIMaxSize {
			sendErr = b.sendVideo(b.localBot, c, result.filePath, caption)
			result.cleanup()
		} else {
			result.cleanup()
			b.log.Warn("video too large for telegram",
				"user", c.Sender().Username,
				"user_id", c.Sender().ID,
				"url", url,
				"size_mb", sizeMB,
			)
			return c.Send(fmt.Sprintf(
				"❌ Видео %d МБ — слишком большое.\n\nИспользуйте /save %s для загрузки в Google Drive.",
				sizeMB, url,
			))
		}

		if sendErr == nil {
			b.log.Info("sent",
				"user", c.Sender().Username,
				"user_id", c.Sender().ID,
				"url", url,
				"size_mb", sizeMB,
				"send_s", int(time.Since(downloadedAt).Seconds()),
				"total_s", int(time.Since(startedAt).Seconds()),
			)
		} else {
			b.log.Error("send failed",
				"user", c.Sender().Username,
				"user_id", c.Sender().ID,
				"url", url,
				"size_mb", sizeMB,
				"error", sendErr,
			)
		}
		return sendErr
	}

	// All other platforms — unified media path.
	return b.handleMediaURL(c, url, st)
}

func (b *Bot) handleMediaURL(c tele.Context, url string, st UserSettings) error {
	startedAt := time.Now()
	statusMsg, _ := b.bot.Send(c.Recipient(), "⏳ Скачиваю медиа...")

	dlCtx, dlCancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer dlCancel()

	result, cleanup, err := b.service.ProcessMedia(dlCtx, url)
	if err != nil {
		b.deleteMsg(statusMsg)
		if errors.Is(err, domain.ErrUnsupportedPlatform) && b.plugins != nil {
			if pluginClient, pluginName, matchedPattern := b.plugins.FindByURL(url); pluginClient != nil {
				return b.handlePluginURL(c, pluginClient, pluginName, url, matchedPattern)
			}
		}
		return b.handleError(c, err)
	}
	defer cleanup()

	sizeMB := result.MaxItemSize() / (1024 * 1024)
	b.log.Info("downloaded",
		"user", c.Sender().Username,
		"user_id", c.Sender().ID,
		"url", url,
		"items", len(result.Items),
		"max_item_mb", sizeMB,
		"download_s", int(time.Since(startedAt).Seconds()),
	)

	b.deleteMsg(statusMsg)
	b.lastURL.Store(c.Sender().ID, url)

	caption := result.Title
	if !st.Caption {
		caption = ""
	} else if url != "" {
		caption += "\n\n" + url
	}

	maxItem := result.MaxItemSize()
	if maxItem > b.tgLimit && b.localBot == nil {
		b.log.Warn("media too large for telegram",
			"user", c.Sender().Username,
			"user_id", c.Sender().ID,
			"url", url,
			"size_mb", sizeMB,
		)
		return c.Send(fmt.Sprintf("❌ Файл %d МБ — слишком большой для Telegram.", sizeMB))
	}

	client := b.bot
	if maxItem > b.tgLimit && b.localBot != nil {
		client = b.localBot
	}

	sendErr := b.sendMedia(client, c, result, caption)
	if sendErr == nil {
		b.log.Info("sent",
			"user", c.Sender().Username,
			"user_id", c.Sender().ID,
			"url", url,
			"items", len(result.Items),
			"send_s", int(time.Since(startedAt).Seconds()),
		)
	} else {
		b.log.Error("send failed",
			"user", c.Sender().Username,
			"user_id", c.Sender().ID,
			"url", url,
			"error", sendErr,
		)
	}
	return sendErr
}

func (b *Bot) sendVideo(client *tele.Bot, c tele.Context, filePath, caption string) error {
	_ = c.Notify(tele.UploadingVideo)
	w, h, dur := probe.VideoMeta(context.Background(), filePath)
	video := &tele.Video{
		File:      tele.FromDisk(filePath),
		Width:     w,
		Height:    h,
		Duration:  dur,
		Streaming: true,
	}
	if caption != "" {
		video.Caption = caption
	}
	_, err := client.Send(c.Recipient(), video)
	return err
}

func (b *Bot) sendPhoto(client *tele.Bot, c tele.Context, filePath, caption string) error {
	_ = c.Notify(tele.UploadingPhoto)
	photo := &tele.Photo{File: tele.FromDisk(filePath)}
	if caption != "" {
		photo.Caption = caption
	}
	_, err := client.Send(c.Recipient(), photo)
	return err
}

func (b *Bot) sendMedia(client *tele.Bot, c tele.Context, result *domain.MediaResult, caption string) error {
	if len(result.Items) == 1 {
		item := result.Items[0]
		if item.Type == domain.MediaPhoto {
			return b.sendPhoto(client, c, item.FilePath, caption)
		}
		return b.sendVideo(client, c, item.FilePath, caption)
	}

	if result.HasVideo() {
		_ = c.Notify(tele.UploadingVideo)
	} else {
		_ = c.Notify(tele.UploadingPhoto)
	}

	var album tele.Album
	for i, item := range result.Items {
		if i >= 10 {
			break
		}
		switch item.Type {
		case domain.MediaPhoto:
			photo := &tele.Photo{File: tele.FromDisk(item.FilePath)}
			if i == 0 && caption != "" {
				photo.Caption = caption
			}
			album = append(album, photo)
		case domain.MediaVideo:
			w, h, _ := probe.VideoMeta(context.Background(), item.FilePath)
			video := &tele.Video{
				File:   tele.FromDisk(item.FilePath),
				Width:  w,
				Height: h,
			}
			if i == 0 && caption != "" {
				video.Caption = caption
			}
			album = append(album, video)
		}
	}

	_, err := client.SendAlbum(c.Recipient(), album)
	return err
}

// ── /settings command ─────────────────────────────────────────────────────────

func (b *Bot) handleSettingsCommand(c tele.Context) error {
	st := b.userSettings(c.Sender().ID)
	return c.Send(settingsText(st), &tele.SendOptions{
		ParseMode:   tele.ModeHTML,
		ReplyMarkup: settingsKeyboard(st),
	})
}

func settingsText(st UserSettings) string {
	captionStatus := "Вкл"
	if !st.Caption {
		captionStatus = "Выкл"
	}
	return fmt.Sprintf(
		"⚙️ <b>Настройки</b>\n\n"+
			"🎬 <b>Качество:</b> <code>%s</code>\n"+
			"📝 <b>Подпись к видео:</b> <code>%s</code>",
		st.Quality, captionStatus,
	)
}

func settingsKeyboard(st UserSettings) *tele.ReplyMarkup {
	kb := &tele.ReplyMarkup{}

	qualityBtns := make([]tele.Btn, len(ValidQualities))
	for i, q := range ValidQualities {
		label := q
		if q == st.Quality {
			label = q + " ✓"
		}
		qualityBtns[i] = kb.Data(label, "set_quality", q)
	}

	captionOn := kb.Data("Вкл", "set_caption", "on")
	captionOff := kb.Data("Выкл", "set_caption", "off")
	if st.Caption {
		captionOn = kb.Data("Вкл ✓", "set_caption", "on")
	} else {
		captionOff = kb.Data("Выкл ✓", "set_caption", "off")
	}

	kb.Inline(
		kb.Row(qualityBtns...),
		kb.Row(captionOn, captionOff),
	)
	return kb
}

// ── Unified callback handler (settings + watch) ──────────────────────────────

func (b *Bot) handleCallback(c tele.Context) error {
	// telebot.v4 prepends \f to all kb.Data() callback values.
	data := strings.TrimPrefix(c.Data(), "\f")

	// Drive / admin callbacks use exact match.
	switch {
	case data == "drive_connect":
		return b.callbackDriveConnect(c)
	case data == "drive_disconnect":
		return b.callbackDriveDisconnect(c)
	case data == "admin_refresh":
		return b.callbackAdminRefresh(c)
	// Watch callbacks (from watch_handler.go) use raw prefixes.
	case strings.HasPrefix(data, "dl:"):
		return b.handleDownloadCallback(c, strings.TrimPrefix(data, "dl:"))
	case strings.HasPrefix(data, "watch_rm:"):
		return b.handleUnsubscribeCallback(c, strings.TrimPrefix(data, "watch_rm:"))
	case strings.HasPrefix(data, "story_rm:"):
		return b.handleStoryUnsubscribeCallback(c, strings.TrimPrefix(data, "story_rm:"))
	case strings.HasPrefix(data, "post_rm:"):
		return b.handlePostUnsubscribeCallback(c, strings.TrimPrefix(data, "post_rm:"))
	case strings.HasPrefix(data, "igdl:"):
		return b.handleInstagramPostDownloadCallback(c, strings.TrimPrefix(data, "igdl:"))
	case strings.HasPrefix(data, "igs:"):
		parts := strings.SplitN(strings.TrimPrefix(data, "igs:"), ":", 2)
		if len(parts) == 2 {
			return b.handleStoryDownloadCallback(c, parts[0], parts[1])
		}
		return c.Respond()
	}

	// Plugin callbacks use "p_<name>|<callback_id>" format.
	if strings.HasPrefix(data, "p_") && b.plugins != nil {
		rest := strings.TrimPrefix(data, "p_")
		if idx := strings.Index(rest, "|"); idx > 0 {
			pluginName := rest[:idx]
			callbackID := rest[idx+1:]
			return b.handlePluginCallback(c, pluginName, callbackID)
		}
	}

	// Settings callbacks use telebot.v4 "\f{unique}|{payload}" format.
	data = strings.TrimPrefix(data, "\f")
	parts := strings.SplitN(data, "|", 2)
	if len(parts) == 2 {
		action, key := parts[0], parts[1]
		switch action {
		case "set_quality":
			return b.callbackSetQuality(c, key)
		case "set_caption":
			return b.callbackSetCaption(c, key)
		}
	}

	return c.Respond()
}

func (b *Bot) callbackSetQuality(c tele.Context, quality string) error {
	for _, q := range ValidQualities {
		if q == quality {
			if b.settings != nil {
				b.settings.SetQuality(c.Sender().ID, quality)
			}
			b.log.Info("setting changed",
				"user", c.Sender().Username,
				"user_id", c.Sender().ID,
				"key", "quality",
				"value", quality,
			)
			st := b.userSettings(c.Sender().ID)
			_ = c.Respond(&tele.CallbackResponse{Text: quality + " ✓"})
			return c.Edit(settingsText(st), &tele.SendOptions{
				ParseMode:   tele.ModeHTML,
				ReplyMarkup: settingsKeyboard(st),
			})
		}
	}
	return c.Respond()
}

func (b *Bot) callbackSetCaption(c tele.Context, value string) error {
	enabled := value == "on"
	if b.settings != nil {
		b.settings.SetCaption(c.Sender().ID, enabled)
	}
	b.log.Info("setting changed",
		"user", c.Sender().Username,
		"user_id", c.Sender().ID,
		"key", "caption",
		"value", enabled,
	)
	st := b.userSettings(c.Sender().ID)
	label := "Подпись включена ✓"
	if !enabled {
		label = "Подпись отключена"
	}
	_ = c.Respond(&tele.CallbackResponse{Text: label})
	return c.Edit(settingsText(st), &tele.SendOptions{
		ParseMode:   tele.ModeHTML,
		ReplyMarkup: settingsKeyboard(st),
	})
}

// ── /details and /save commands ───────────────────────────────────────────────

func (b *Bot) handleDetailsCommand(c tele.Context) error {
	if b.qualityDl == nil {
		return c.Send("⚙️ Анализ форматов недоступен.")
	}

	url := extractURL(strings.Join(c.Args(), " "))
	if url == "" {
		return c.Send("Использование: /details <url>")
	}

	statusMsg, _ := b.bot.Send(c.Recipient(), "📋 Получаю список форматов...")

	ctx, cancel := context.WithTimeout(context.Background(), analyzeTimeout)
	defer cancel()

	summary, err := b.qualityDl.AnalyzeFormats(ctx, url)
	b.deleteMsg(statusMsg)
	if err != nil {
		b.log.Error("analyze failed", "url", url, "error", err)
		return c.Send(fmt.Sprintf("❌ Не удалось получить форматы: <code>%v</code>", err),
			&tele.SendOptions{ParseMode: tele.ModeHTML})
	}

	return c.Send(formatDetailsMsg(summary), &tele.SendOptions{ParseMode: tele.ModeHTML})
}

func (b *Bot) handleSaveCommand(c tele.Context) error {
	if b.drive == nil {
		return c.Send("⚙️ Google Drive не настроен.")
	}
	if b.qualityDl == nil {
		return c.Send("⚙️ Загрузчик недоступен.")
	}

	userID := c.Sender().ID
	if !b.drive.IsConnected(userID) {
		kb := &tele.ReplyMarkup{}
		kb.Inline(kb.Row(kb.Data("🔗 Подключить Google Drive", "drive_connect")))
		return c.Send("⚙️ Google Drive не подключён.", kb)
	}

	url := extractURL(strings.Join(c.Args(), " "))
	if url == "" {
		if val, ok := b.lastURL.Load(userID); ok {
			url = val.(string)
		}
	}
	if url == "" {
		return c.Send("Использование: /save <url>\n\nИли отправьте ссылку, затем /save")
	}

	if free, err := freeDiskBytes("."); err == nil && free < minDiskSpaceBytes {
		b.log.Warn("low disk space before /save", "free_mb", free/(1024*1024))
		return c.Send(fmt.Sprintf("⚠️ Мало места на диске (%d МБ). Попробуйте позже.", free/(1024*1024)))
	}

	statusMsg, _ := b.bot.Send(c.Recipient(), "⏳ Скачиваю оригинальное качество...")

	ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
	defer cancel()

	best, err := b.qualityDl.DownloadBest(ctx, url)
	if err != nil {
		b.deleteMsg(statusMsg)
		b.log.Error("best download failed", "url", url, "error", err)
		return b.handleError(c, err)
	}
	defer func() { _ = os.RemoveAll(filepath.Dir(best.FilePath)) }()

	bestMB := best.Size / (1024 * 1024)
	b.editMsg(statusMsg, fmt.Sprintf("☁️ Загружаю %d МБ в Google Drive...", bestMB))

	link, err := b.drive.Upload(ctx, userID, best.Title, best.FilePath)
	b.deleteMsg(statusMsg)
	if err != nil {
		b.log.Error("gdrive upload failed", "error", err)
		return c.Send(fmt.Sprintf("❌ Ошибка загрузки: <code>%v</code>", err),
			&tele.SendOptions{ParseMode: tele.ModeHTML})
	}

	b.log.Info("saved to drive",
		"user", c.Sender().Username,
		"user_id", c.Sender().ID,
		"url", url,
		"size_mb", bestMB,
	)
	return c.Send(fmt.Sprintf("✅ Сохранено в Google Drive\n\nРазмер: %d МБ\n\n%s", bestMB, link))
}

// ── /audio command ────────────────────────────────────────────────────────────

func (b *Bot) handleAudioCommand(c tele.Context) error {
	if b.qualityDl == nil {
		return c.Send("⚙️ Загрузчик недоступен.")
	}

	userID := c.Sender().ID
	url := extractURL(strings.Join(c.Args(), " "))
	if url == "" {
		if val, ok := b.lastURL.Load(userID); ok {
			url = val.(string)
		}
	}
	if url == "" {
		return c.Send("Использование: /audio <url>\n\nИли отправьте ссылку, затем /audio")
	}

	if free, err := freeDiskBytes("."); err == nil && free < minDiskSpaceBytes {
		b.log.Warn("low disk space before /audio", "free_mb", free/(1024*1024))
		return c.Send(fmt.Sprintf("⚠️ Мало места на диске (%d МБ). Попробуйте позже.", free/(1024*1024)))
	}

	statusMsg, _ := b.bot.Send(c.Recipient(), "⏳ Скачиваю аудио...")

	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer cancel()

	video, err := b.qualityDl.DownloadAudio(ctx, url)
	if err != nil {
		b.deleteMsg(statusMsg)
		b.log.Error("audio download failed", "url", url, "error", err)
		return b.handleError(c, err)
	}
	defer func() { _ = os.RemoveAll(filepath.Dir(video.FilePath)) }()

	sizeMB := video.Size / (1024 * 1024)
	b.log.Info("audio downloaded",
		"user", c.Sender().Username,
		"user_id", userID,
		"url", url,
		"size_mb", sizeMB,
	)

	b.deleteMsg(statusMsg)

	audio := &tele.Audio{
		File:      tele.FromDisk(video.FilePath),
		Title:     video.Title,
		Performer: video.Channel,
	}

	client := b.bot
	if video.Size > b.tgLimit && b.localBot != nil {
		client = b.localBot
	} else if video.Size > b.tgLimit {
		return c.Send(fmt.Sprintf("❌ Файл %d МБ — слишком большой для Telegram.", sizeMB))
	}

	_, sendErr := client.Send(c.Recipient(), audio)
	if sendErr != nil {
		b.log.Error("audio send failed",
			"user", c.Sender().Username,
			"user_id", userID,
			"url", url,
			"error", sendErr,
		)
	}
	return sendErr
}

// ── /drive command (replaces /auth + /disconnect) ─────────────────────────────

func driveConnectKeyboard() *tele.ReplyMarkup {
	kb := &tele.ReplyMarkup{}
	kb.Inline(kb.Row(kb.Data("🔗 Подключить Google Drive", "drive_connect")))
	return kb
}

func driveDisconnectKeyboard() *tele.ReplyMarkup {
	kb := &tele.ReplyMarkup{}
	kb.Inline(kb.Row(kb.Data("🔌 Отключить", "drive_disconnect")))
	return kb
}

func (b *Bot) handleDriveCommand(c tele.Context) error {
	if b.drive == nil {
		return c.Send("⚙️ Google Drive не настроен.")
	}
	if b.drive.IsConnected(c.Sender().ID) {
		return c.Send("✅ Google Drive подключён.", driveDisconnectKeyboard())
	}
	return c.Send("Google Drive не подключён.", driveConnectKeyboard())
}

func (b *Bot) callbackDriveConnect(c tele.Context) error {
	if b.drive == nil {
		return c.Respond(&tele.CallbackResponse{Text: "OAuth не настроен"})
	}

	_ = c.Respond()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	info, pollFn, err := b.drive.StartAuth(ctx)
	if err != nil {
		b.log.Error("device auth start failed", "error", err)
		return c.Edit("❌ Не удалось запустить авторизацию. Попробуйте позже.", driveConnectKeyboard())
	}

	authText := fmt.Sprintf(
		"🔐 <b>Подключение Google Drive</b>\n\n"+
			"<b>1.</b> Откройте: <code>%s</code>\n"+
			"<b>2.</b> Введите код: <code>%s</code>\n\n"+
			"Жду подтверждения... (код действует %d мин)\n\n"+
			"⚠️ Бот получит доступ <b>только к файлам, которые сам загрузит</b> — "+
			"ваши существующие файлы недоступны.",
		info.VerificationURI,
		info.UserCode,
		int(time.Until(info.Expiry).Minutes()),
	)
	if err := c.Edit(authText, &tele.SendOptions{ParseMode: tele.ModeHTML}); err != nil {
		_ = c.Send(authText, &tele.SendOptions{ParseMode: tele.ModeHTML})
	}

	userID := c.Sender().ID
	msg := c.Message()

	go func() {
		pollCtx, pollCancel := context.WithDeadline(context.Background(), info.Expiry)
		defer pollCancel()

		if err := pollFn(pollCtx, userID); err != nil {
			b.log.Warn("drive auth failed", "user_id", userID, "error", err)
			kb := &tele.ReplyMarkup{}
			kb.Inline(kb.Row(kb.Data("🔄 Попробовать снова", "drive_connect")))
			_, _ = b.bot.Edit(msg, "⏱ Время вышло или доступ отклонён.", kb)
			return
		}
		b.log.Info("drive connected", "user_id", userID)
		_, _ = b.bot.Edit(msg, "✅ Google Drive подключён!\n\nТеперь /save будет сохранять видео в ваш личный Drive.", driveDisconnectKeyboard())
	}()

	return nil
}

func (b *Bot) callbackDriveDisconnect(c tele.Context) error {
	if b.drive == nil {
		return c.Respond(&tele.CallbackResponse{Text: "OAuth не настроен"})
	}
	b.drive.Disconnect(c.Sender().ID)
	b.log.Info("drive disconnected", "user", c.Sender().Username, "user_id", c.Sender().ID)
	_ = c.Respond(&tele.CallbackResponse{Text: "Google Drive отключён"})
	return c.Edit("Google Drive не подключён.", driveConnectKeyboard())
}

// ── Format messages ───────────────────────────────────────────────────────────

func formatDetailsMsg(s *domain.FormatSummary) string {
	var sb strings.Builder
	sb.WriteString("📋 <b>Доступные форматы</b>\n")
	if s.Title != "" {
		fmt.Fprintf(&sb, "<i>%s</i>\n", escapeHTML(s.Title))
	}
	fmt.Fprintf(&sb, "⏱ %s\n\n", formatDuration(s.Duration))

	if len(s.Entries) == 0 {
		sb.WriteString("Форматы не найдены.")
		return sb.String()
	}

	sb.WriteString("<code>Разрешение    Размер\n")
	sb.WriteString("─────────────────────\n")
	for _, e := range s.Entries {
		sizeStr := "?"
		if e.Size > 0 {
			sizeStr = fmt.Sprintf("~%.0f МБ", math.Ceil(float64(e.Size)/(1024*1024)))
		}
		fmt.Fprintf(&sb, "  %5dp      %s\n", e.Height, sizeStr)
	}
	sb.WriteString("</code>")
	return sb.String()
}

func formatDuration(secs float64) string {
	total := int(secs)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// ── Disk space ────────────────────────────────────────────────────────────────

func freeDiskBytes(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}

// ── Error handling ────────────────────────────────────────────────────────────

func (b *Bot) handleError(c tele.Context, err error) error {
	showDebug := b.debug && b.IsAdmin(c.Sender().Username)
	user := c.Sender().Username
	userID := c.Sender().ID
	switch {
	case errors.Is(err, domain.ErrUnsupportedPlatform):
		b.log.Warn("unsupported platform", "user", user, "user_id", userID)
		platforms := "YouTube, X (Twitter), Threads"
		if b.instagramEnabled {
			platforms = "YouTube, Instagram, X (Twitter), Threads"
		}
		return c.Send(fmt.Sprintf("Платформа не поддерживается. Поддерживаются: %s.", platforms))
	case errors.Is(err, domain.ErrVideoTooLarge):
		b.log.Warn("video too large", "user", user, "user_id", userID)
		return c.Send("Видео слишком большое.")
	case errors.Is(err, domain.ErrDownloadFailed):
		b.log.Error("download failed", "user", user, "user_id", userID, "error", err)
		msg := "Ошибка загрузки. Видео может быть закрыто, приватным или ссылка повреждена."
		if showDebug {
			msg += fmt.Sprintf("\n\n<code>%v</code>", escapeHTML(err.Error()))
		}
		return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeHTML})
	default:
		b.log.Error("unexpected error", "user", user, "user_id", userID, "error", err)
		msg := "Что-то пошло не так. Попробуйте ещё раз."
		if showDebug {
			msg += fmt.Sprintf("\n\n<code>%v</code>", escapeHTML(err.Error()))
		}
		return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeHTML})
	}
}

// ── /admin command (replaces /config + /cookies) ──────────────────────────────

func (b *Bot) adminPanelMsg() string {
	var sb strings.Builder
	sb.WriteString("⚙️ <b>Панель администратора</b>\n\n")
	fmt.Fprintf(&sb, "Версия: <code>%s</code>\n", b.version)
	if b.debug {
		sb.WriteString("Debug: <code>включён ✅</code>\n")
	}
	fmt.Fprintf(&sb, "Лимит TG: <code>%d МБ</code>\n", b.tgLimit/(1024*1024))
	if b.localBot != nil {
		sb.WriteString("Local Bot API: <code>включён ✅</code>\n")
	} else {
		sb.WriteString("Local Bot API: <code>отключён</code>\n")
	}
	if b.drive != nil {
		sb.WriteString("Google Drive OAuth: <code>включён ✅</code>\n")
	} else {
		sb.WriteString("Google Drive OAuth: <code>отключён</code>\n")
	}
	if free, err := freeDiskBytes("."); err == nil {
		fmt.Fprintf(&sb, "Диск свободно: <code>%d МБ</code>\n", free/(1024*1024))
	}

	// Cookies status.
	sb.WriteString("\n")
	cookiePlatforms := []struct{ name, key string }{{"YouTube", "youtube"}}
	if b.instagramEnabled {
		cookiePlatforms = append(cookiePlatforms, struct{ name, key string }{"Instagram", "instagram"})
	}
	for _, p := range cookiePlatforms {
		if b.cookies != nil && b.cookies.HasCookies(p.key) {
			fmt.Fprintf(&sb, "%s Cookies: ✅\n", p.name)
		} else {
			fmt.Fprintf(&sb, "%s Cookies: ❌\n", p.name)
		}
	}
	sb.WriteString("\n/add_youtube_cookies — загрузить YouTube cookies")
	if b.instagramEnabled {
		sb.WriteString("\n/add_instagram_cookies — загрузить Instagram cookies")
	}
	sb.WriteString("\n/delete_youtube_cookies — удалить YouTube cookies")
	sb.WriteString("\n/delete_instagram_cookies — удалить Instagram cookies")
	return sb.String()
}

func adminRefreshKeyboard() *tele.ReplyMarkup {
	kb := &tele.ReplyMarkup{}
	kb.Inline(kb.Row(kb.Data("🔄 Обновить", "admin_refresh")))
	return kb
}

func (b *Bot) handleAdminCommand(c tele.Context) error {
	if !b.IsAdmin(c.Sender().Username) {
		return c.Send("❌ Нет доступа.")
	}
	return c.Send(b.adminPanelMsg(), &tele.SendOptions{
		ParseMode:   tele.ModeHTML,
		ReplyMarkup: adminRefreshKeyboard(),
	})
}

func (b *Bot) callbackAdminRefresh(c tele.Context) error {
	if !b.IsAdmin(c.Sender().Username) {
		return c.Respond(&tele.CallbackResponse{Text: "Нет доступа"})
	}
	_ = c.Respond(&tele.CallbackResponse{Text: "Обновлено"})
	return c.Edit(b.adminPanelMsg(), &tele.SendOptions{
		ParseMode:   tele.ModeHTML,
		ReplyMarkup: adminRefreshKeyboard(),
	})
}

const maxCookiesSize = 1 * 1024 * 1024

func cookieInstructions(platform string) string {
	label := "YouTube"
	site := "youtube.com"
	if platform == "instagram" {
		label = "Instagram"
		site = "instagram.com"
	}
	return fmt.Sprintf(
		"<b>Загрузка %s cookies</b>\n\n"+
			"Cookies нужны в формате Netscape (cookies.txt).\n\n"+
			"<b>Как получить:</b>\n"+
			"1. Установите расширение <b>Get cookies.txt LOCALLY</b> "+
			"(Chrome/Firefox)\n"+
			"2. Откройте %s и войдите в аккаунт\n"+
			"3. Нажмите иконку расширения → Export\n"+
			"4. Отправьте полученный файл сюда\n\n"+
			"Также можно вставить содержимое как текстовое сообщение.",
		label, site,
	)
}

func (b *Bot) handleAddCookies(platform string) tele.HandlerFunc {
	return func(c tele.Context) error {
		if !b.IsAdmin(c.Sender().Username) {
			return c.Send("❌ Нет доступа.")
		}
		b.pendingCookies.Store(c.Sender().ID, platform)
		return c.Send(cookieInstructions(platform), &tele.SendOptions{ParseMode: tele.ModeHTML})
	}
}

func (b *Bot) handleDeleteCookies(platform string) tele.HandlerFunc {
	return func(c tele.Context) error {
		if !b.IsAdmin(c.Sender().Username) {
			return c.Send("❌ Нет доступа.")
		}
		label := "YouTube"
		if platform == "instagram" {
			label = "Instagram"
		}
		if b.cookies == nil || !b.cookies.HasCookies(platform) {
			return c.Send(fmt.Sprintf("%s cookies не установлены.", label))
		}
		if err := b.cookies.DeleteCookies(context.Background(), platform); err != nil {
			b.log.Error("failed to delete cookies", "platform", platform, "error", err)
			return c.Send("Не удалось удалить cookies.")
		}
		return c.Send(fmt.Sprintf("✅ %s cookies удалены.", label))
	}
}

func (b *Bot) handleDocument(c tele.Context) error {
	doc := c.Message().Document
	if doc == nil {
		return nil
	}

	// Import (any user).
	if _, ok := b.pendingImport.LoadAndDelete(c.Sender().ID); ok {
		return b.handleImportFile(c, doc)
	}

	// Cookies (admin only).
	if !b.IsAdmin(c.Sender().Username) {
		return nil
	}

	val, ok := b.pendingCookies.LoadAndDelete(c.Sender().ID)
	if !ok {
		return c.Send("Используйте /add\\_youtube\\_cookies или /add\\_instagram\\_cookies для загрузки cookies.")
	}

	platform := val.(string)
	return b.saveCookiesFromFile(c, doc, platform)
}

func (b *Bot) saveCookiesFromFile(c tele.Context, doc *tele.Document, platform string) error {
	if doc.FileSize > maxCookiesSize {
		return c.Send("Файл слишком большой (макс. 1 МБ).")
	}
	reader, err := b.bot.File(&doc.File)
	if err != nil {
		return c.Send("Не удалось получить файл из Telegram.")
	}
	defer func() { _ = reader.Close() }()

	data, err := io.ReadAll(io.LimitReader(reader, maxCookiesSize))
	if err != nil {
		return c.Send("Не удалось прочитать файл.")
	}

	return b.saveCookies(c, platform, data)
}

func (b *Bot) saveCookiesFromText(c tele.Context, platform string) error {
	data := []byte(strings.TrimSpace(c.Text()))
	if len(data) > maxCookiesSize {
		return c.Send("Текст слишком большой (макс. 1 МБ).")
	}
	return b.saveCookies(c, platform, data)
}

func (b *Bot) saveCookies(c tele.Context, platform string, data []byte) error {
	if !looksLikeNetscapeCookies(data) {
		return c.Send("⚠️ Содержимое не похоже на Netscape cookies формат.\n" +
			"Ожидается файл с заголовком <code># Netscape HTTP Cookie File</code> " +
			"или строки, разделённые табуляцией.", &tele.SendOptions{ParseMode: tele.ModeHTML})
	}

	if b.cookies == nil {
		return c.Send("Хранилище cookies не настроено.")
	}

	if err := b.cookies.SaveCookies(context.Background(), platform, data); err != nil {
		b.log.Error("failed to save cookies", "platform", platform, "error", err)
		return c.Send("Не удалось сохранить cookies.")
	}

	label := "YouTube"
	if platform == "instagram" {
		label = "Instagram"
	}
	b.log.Info("cookies updated", "platform", platform, "size", len(data))
	return c.Send(fmt.Sprintf("✅ %s cookies сохранены (%d байт).", label, len(data)))
}

func looksLikeNetscapeCookies(data []byte) bool {
	text := string(data)
	if strings.Contains(text, "# Netscape HTTP Cookie File") {
		return true
	}
	for _, line := range strings.SplitN(text, "\n", 10) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if len(strings.Split(line, "\t")) >= 7 {
			return true
		}
	}
	return false
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (b *Bot) userSettings(userID int64) UserSettings {
	if b.settings != nil {
		return b.settings.Get(userID)
	}
	return defaultSettings()
}

var urlRe = regexp.MustCompile(`https?://\S+`)

func extractURL(text string) string {
	text = strings.TrimSpace(text)
	if m := urlRe.FindString(text); m != "" {
		return m
	}
	return ""
}

func (b *Bot) deleteMsg(msg *tele.Message) {
	if msg != nil {
		_ = b.bot.Delete(msg)
	}
}

func (b *Bot) editMsg(msg *tele.Message, text string) {
	if msg != nil {
		_, _ = b.bot.Edit(msg, text)
	}
}
