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

	ytdlpdl "gitlab.com/tiny-services/multiverse-bot/internal/adapter/downloader/ytdlp"
	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/gdrive"
	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

const (
	downloadTimeout    = 30 * time.Minute
	uploadTimeout      = 30 * time.Minute
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

	b.bot.Handle("/start", func(c tele.Context) error {
		msg := "Multiverse Bot\n\n" +
			"Платформы: YouTube, Instagram, X (Twitter), Threads\n\n" +
			"Команды:\n" +
			"/settings — настройки (качество, подпись)\n" +
			"/details <url> — доступные форматы и размеры\n" +
			"/save [url] — сохранить в Google Drive\n" +
			"/auth — подключить Google Drive\n" +
			"/disconnect — отключить Google Drive\n"
		if b.IsAdmin(c.Sender().Username) {
			msg += "\nАдмин:\n" +
				"/config — статус бота и место на диске\n" +
				"/cookies — статус cookies.txt (отправь файл для обновления)\n" +
				"/gdrive_test — тест загрузки в Google Drive"
		}
		return c.Send(msg)
	})

	b.bot.Handle("/settings", b.handleSettingsCommand)
	b.bot.Handle("/config", b.handleConfigCommand)
	b.bot.Handle("/cookies", b.handleCookiesStatus)
	b.bot.Handle("/gdrive_test", b.handleGDriveTest)
	b.bot.Handle("/details", b.handleDetailsCommand)
	b.bot.Handle("/save", b.handleSaveCommand)
	b.bot.Handle("/auth", b.handleAuthCommand)
	b.bot.Handle("/disconnect", b.handleDisconnectCommand)
	b.bot.Handle(tele.OnDocument, b.handleDocument)
	b.bot.Handle(tele.OnText, b.handleText)
	b.bot.Handle(tele.OnCallback, b.handleCallback)
}

// ── Main video handler ────────────────────────────────────────────────────────

func (b *Bot) handleText(c tele.Context) error {
	url := extractURL(c.Text())
	if url == "" {
		return c.Send("Пожалуйста, отправьте корректную ссылку.")
	}

	if free, err := freeDiskBytes("."); err == nil && free < minDiskSpaceBytes {
		b.log.Warn("low disk space", "free_mb", free/(1024*1024))
		return c.Send(fmt.Sprintf("⚠️ Мало места на диске (%d МБ). Попробуйте позже.", free/(1024*1024)))
	}

	st := defaultSettings()
	if b.settings != nil {
		st = b.settings.Get(c.Sender().ID)
	}
	quality := st.Quality

	b.log.Debug("incoming message", "url", url, "sender", c.Sender().Username, "quality", quality)

	startedAt := time.Now()
	statusMsg, _ := b.bot.Send(c.Recipient(), fmt.Sprintf("⏳ Скачиваю %s...", quality))

	dlCtx, dlCancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer dlCancel()

	var (
		filePath string
		sizeMB   int64
		title    string
		cleanup  func()
	)

	ytVideo, err := b.ytdlp.DownloadQuality(dlCtx, url, quality)
	if err != nil {
		b.log.Warn("ytdlp failed, trying composite", "error", err)
		video, svcCleanup, svcErr := b.service.ProcessURL(dlCtx, url)
		if svcErr != nil {
			b.deleteMsg(statusMsg)
			return b.handleError(c, svcErr)
		}
		filePath = video.FilePath
		sizeMB = video.Size / (1024 * 1024)
		title = video.Title
		cleanup = svcCleanup
	} else {
		filePath = ytVideo.FilePath
		sizeMB = ytVideo.Size / (1024 * 1024)
		title = ytVideo.Title
		dir := filepath.Dir(filePath)
		cleanup = func() { _ = os.RemoveAll(dir) }
	}

	downloadedAt := time.Now()
	dlDur := downloadedAt.Sub(startedAt)
	b.log.Info("downloaded",
		"size_mb", sizeMB,
		"quality", quality,
		"download_s", int(dlDur.Seconds()),
		"speed_mbs", fmt.Sprintf("%.1f", float64(sizeMB)/dlDur.Seconds()),
		"url", url,
	)

	b.deleteMsg(statusMsg)

	// Remember last URL for this user (/save without args)
	b.lastURL.Store(c.Sender().ID, url)

	// Apply caption setting
	caption := title
	if !st.Caption {
		caption = ""
	}

	fileSize := sizeMB * 1024 * 1024
	var sendErr error

	if fileSize <= b.tgLimit {
		defer cleanup()
		sendErr = b.sendVideo(b.bot, c, filePath, caption)
	} else if b.localBot != nil && fileSize < localBotAPIMaxSize {
		sendErr = b.sendVideo(b.localBot, c, filePath, caption)
		cleanup()
	} else {
		cleanup()
		return c.Send(fmt.Sprintf(
			"❌ Видео %d МБ — слишком большое.\n\nИспользуйте /save %s для загрузки в Google Drive.",
			sizeMB, url,
		))
	}

	if sendErr == nil {
		b.log.Info("sent",
			"size_mb", sizeMB,
			"send_s", int(time.Since(downloadedAt).Seconds()),
			"total_s", int(time.Since(startedAt).Seconds()),
		)
	}
	return sendErr
}

func (b *Bot) sendVideo(client *tele.Bot, c tele.Context, filePath, caption string) error {
	_ = c.Notify(tele.UploadingVideo)
	video := &tele.Video{File: tele.FromDisk(filePath)}
	if caption != "" {
		video.Caption = caption
	}
	_, err := client.Send(c.Recipient(), video)
	return err
}

// ── /settings command ─────────────────────────────────────────────────────────

func (b *Bot) handleSettingsCommand(c tele.Context) error {
	st := defaultSettings()
	if b.settings != nil {
		st = b.settings.Get(c.Sender().ID)
	}
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

	// Quality row
	qualityBtns := make([]tele.Btn, len(ValidQualities))
	for i, q := range ValidQualities {
		label := q
		if q == st.Quality {
			label = q + " ✓"
		}
		qualityBtns[i] = kb.Data(label, "set_quality", q)
	}

	// Caption row
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

// ── Callback handler ──────────────────────────────────────────────────────────

func (b *Bot) handleCallback(c tele.Context) error {
	// telebot.v4 formats button data as "\f{unique}|{payload}"
	data := strings.TrimPrefix(c.Data(), "\f")
	parts := strings.SplitN(data, "|", 2)
	if len(parts) != 2 {
		return c.Respond()
	}
	action, key := parts[0], parts[1]

	switch action {
	case "set_quality":
		return b.callbackSetQuality(c, key)
	case "set_caption":
		return b.callbackSetCaption(c, key)
	}
	return c.Respond()
}

func (b *Bot) callbackSetQuality(c tele.Context, quality string) error {
	for _, q := range ValidQualities {
		if q == quality {
			if b.settings != nil {
				b.settings.SetQuality(c.Sender().ID, quality)
			}
			st := defaultSettings()
			if b.settings != nil {
				st = b.settings.Get(c.Sender().ID)
			}
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
	st := defaultSettings()
	if b.settings != nil {
		st = b.settings.Get(c.Sender().ID)
	}
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
	url := extractURL(strings.Join(c.Args(), " "))
	if url == "" {
		return c.Send("Использование: /details <url>")
	}

	statusMsg, _ := b.bot.Send(c.Recipient(), "📋 Получаю список форматов...")

	ctx, cancel := context.WithTimeout(context.Background(), analyzeTimeout)
	defer cancel()

	summary, err := b.ytdlp.AnalyzeFormats(ctx, url)
	b.deleteMsg(statusMsg)
	if err != nil {
		b.log.Error("analyze failed", "url", url, "error", err)
		return c.Send(fmt.Sprintf("❌ Не удалось получить форматы: <code>%v</code>", err),
			&tele.SendOptions{ParseMode: tele.ModeHTML})
	}

	return c.Send(formatDetailsMsg(summary), &tele.SendOptions{ParseMode: tele.ModeHTML})
}

func (b *Bot) handleSaveCommand(c tele.Context) error {
	userID := c.Sender().ID
	hasOAuth := b.oauth != nil && b.oauth.IsConnected(userID)
	hasServiceAccount := b.gdrive != nil

	if !hasOAuth && !hasServiceAccount {
		if b.oauth != nil {
			return c.Send("⚙️ Google Drive не подключён.\n\nИспользуйте /auth чтобы подключить свой Google Drive.")
		}
		return c.Send("⚙️ Google Drive не настроен.")
	}

	// Use provided URL or fall back to last sent URL for this user
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

	ctx, cancel := context.WithTimeout(context.Background(), uploadTimeout)
	defer cancel()

	best, err := b.ytdlp.DownloadBest(ctx, url)
	if err != nil {
		b.deleteMsg(statusMsg)
		b.log.Error("best download failed", "url", url, "error", err)
		return b.handleError(c, err)
	}
	defer func() { _ = os.RemoveAll(filepath.Dir(best.FilePath)) }()

	bestMB := best.Size / (1024 * 1024)
	b.editMsg(statusMsg, fmt.Sprintf("☁️ Загружаю %d МБ в Google Drive...", bestMB))

	var link string
	if hasOAuth {
		// Per-user OAuth: file goes to their own Google Drive
		svc, svcErr := b.oauth.DriveService(ctx, userID)
		if svcErr != nil {
			b.deleteMsg(statusMsg)
			return c.Send("⚠️ Сессия Google Drive устарела. Используйте /auth для повторной авторизации.")
		}
		link, err = gdrive.UploadUserFile(ctx, svc, best.Title, best.FilePath)
	} else {
		// Fallback: shared service account folder
		link, err = b.gdrive.Upload(ctx, best.FilePath)
	}

	b.deleteMsg(statusMsg)
	if err != nil {
		b.log.Error("gdrive upload failed", "error", err)
		return c.Send(fmt.Sprintf("❌ Ошибка загрузки: <code>%v</code>", err),
			&tele.SendOptions{ParseMode: tele.ModeHTML})
	}

	return c.Send(fmt.Sprintf("✅ Сохранено в Google Drive\n\nРазмер: %d МБ\n\n%s", bestMB, link))
}

// ── /auth and /disconnect commands ────────────────────────────────────────────

func (b *Bot) handleAuthCommand(c tele.Context) error {
	if b.oauth == nil {
		return c.Send("⚙️ OAuth не настроен. Обратитесь к администратору.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := b.oauth.StartDeviceAuth(ctx)
	if err != nil {
		b.log.Error("device auth start failed", "error", err)
		return c.Send("❌ Не удалось запустить авторизацию. Попробуйте позже.")
	}

	_ = c.Send(fmt.Sprintf(
		"🔐 <b>Подключение Google Drive</b>\n\n"+
			"<b>1.</b> Откройте: <code>%s</code>\n"+
			"<b>2.</b> Введите код: <code>%s</code>\n\n"+
			"Жду подтверждения... (код действует %d мин)\n\n"+
			"⚠️ Бот получит доступ <b>только к файлам, которые сам загрузит</b> — "+
			"ваши существующие файлы недоступны.",
		resp.VerificationURI,
		resp.UserCode,
		int(time.Until(resp.Expiry).Minutes()),
	), &tele.SendOptions{ParseMode: tele.ModeHTML})

	userID := c.Sender().ID
	recipient := c.Recipient()

	go func() {
		pollCtx, pollCancel := context.WithDeadline(context.Background(), resp.Expiry)
		defer pollCancel()

		if err := b.oauth.PollDeviceAuth(pollCtx, userID, resp); err != nil {
			_, _ = b.bot.Send(recipient, "⏱ Время вышло или доступ отклонён. Используйте /auth снова.")
			return
		}
		_, _ = b.bot.Send(recipient, "✅ Google Drive подключён!\n\nТеперь /save будет сохранять видео в ваш личный Drive.")
	}()

	return nil
}

func (b *Bot) handleDisconnectCommand(c tele.Context) error {
	if b.oauth == nil {
		return c.Send("⚙️ OAuth не настроен.")
	}
	if !b.oauth.IsConnected(c.Sender().ID) {
		return c.Send("Google Drive не подключён.")
	}
	b.oauth.Disconnect(c.Sender().ID)
	return c.Send("✅ Google Drive отключён. Токен удалён.")
}

// ── Format messages ───────────────────────────────────────────────────────────

func formatDetailsMsg(s *ytdlpdl.FormatSummary) string {
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
	isAdmin := b.IsAdmin(c.Sender().Username)
	switch {
	case errors.Is(err, domain.ErrUnsupportedPlatform):
		return c.Send("Платформа не поддерживается. Поддерживаются: YouTube, Instagram, X (Twitter), Threads.")
	case errors.Is(err, domain.ErrVideoTooLarge):
		return c.Send("Видео слишком большое.")
	case errors.Is(err, domain.ErrDownloadFailed):
		b.log.Error("download failed", "error", err)
		msg := "Ошибка загрузки. Видео может быть закрыто, приватным или ссылка повреждена."
		if isAdmin {
			msg += fmt.Sprintf("\n\n<code>%v</code>", err)
		}
		return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeHTML})
	default:
		b.log.Error("unexpected error", "error", err)
		msg := "Что-то пошло не так. Попробуйте ещё раз."
		if isAdmin {
			msg += fmt.Sprintf("\n\n<code>%v</code>", err)
		}
		return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeHTML})
	}
}

// ── Admin commands ────────────────────────────────────────────────────────────

func (b *Bot) handleGDriveTest(c tele.Context) error {
	if !b.IsAdmin(c.Sender().Username) {
		return c.Send("❌ Нет доступа.")
	}
	if b.gdrive == nil {
		return c.Send("Google Drive не настроен.")
	}
	_ = c.Send("Загружаю тестовый файл...")
	tmpFile, err := os.CreateTemp("", "gdrive-test-*.txt")
	if err != nil {
		return c.Send("Не удалось создать тестовый файл: " + err.Error())
	}
	defer os.Remove(tmpFile.Name())
	_, _ = tmpFile.WriteString("Google Drive test upload from multiverse-bot ✅")
	_ = tmpFile.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	link, err := b.gdrive.Upload(ctx, tmpFile.Name())
	if err != nil {
		b.log.Error("gdrive test failed", "error", err)
		return c.Send(fmt.Sprintf("❌ Ошибка:\n<code>%v</code>", err),
			&tele.SendOptions{ParseMode: tele.ModeHTML})
	}
	return c.Send("✅ Google Drive работает!\n\n" + link)
}

func (b *Bot) handleConfigCommand(c tele.Context) error {
	if !b.IsAdmin(c.Sender().Username) {
		return c.Send("❌ Нет доступа.")
	}
	var sb strings.Builder
	sb.WriteString("⚙️ <b>Конфигурация бота</b>\n\n")
	fmt.Fprintf(&sb, "Версия: <code>%s</code>\n", b.version)
	fmt.Fprintf(&sb, "Лимит TG: <code>%d МБ</code>\n", b.tgLimit/(1024*1024))
	if b.localBot != nil {
		sb.WriteString("Local Bot API: <code>включён ✅</code>\n")
	} else {
		sb.WriteString("Local Bot API: <code>отключён</code>\n")
	}
	if b.gdrive != nil {
		sb.WriteString("Google Drive: <code>включён ✅</code>\n")
	} else {
		sb.WriteString("Google Drive: <code>отключён</code>\n")
	}
	if free, err := freeDiskBytes("."); err == nil {
		fmt.Fprintf(&sb, "Диск свободно: <code>%d МБ</code>\n", free/(1024*1024))
	}
	return c.Send(sb.String(), &tele.SendOptions{ParseMode: tele.ModeHTML})
}

func (b *Bot) handleCookiesStatus(c tele.Context) error {
	if !b.IsAdmin(c.Sender().Username) {
		return c.Send("❌ Нет доступа.")
	}
	info, err := os.Stat(b.cookiesFile)
	if err == nil {
		return c.Send(fmt.Sprintf(
			"Cookies: ✅\nПуть: <code>%s</code>\nРазмер: %d байт\n\nОтправьте новый cookies.txt для обновления.",
			b.cookiesFile, info.Size(),
		), &tele.SendOptions{ParseMode: tele.ModeHTML})
	}
	return c.Send("Cookies: ❌\n\nОтправьте <code>cookies.txt</code> (Netscape формат) для авторизации YouTube.",
		&tele.SendOptions{ParseMode: tele.ModeHTML})
}

const maxCookiesFileSize = 1 * 1024 * 1024

func (b *Bot) handleDocument(c tele.Context) error {
	doc := c.Message().Document
	if doc == nil {
		return nil
	}
	if !b.IsAdmin(c.Sender().Username) {
		if doc.FileName == "cookies.txt" {
			return c.Send("❌ Нет доступа.")
		}
		return nil
	}
	if doc.FileName != "cookies.txt" {
		return c.Send("Принимаются файлы: <code>cookies.txt</code>.", &tele.SendOptions{ParseMode: tele.ModeHTML})
	}
	return b.saveBotFile(c, doc, b.cookiesFile, maxCookiesFileSize, "✅ Cookies сохранены.")
}

func (b *Bot) saveBotFile(c tele.Context, doc *tele.Document, destPath string, maxSize int64, successMsg string) error {
	if doc.FileSize > maxSize {
		return c.Send(fmt.Sprintf("Файл слишком большой (макс. %d МБ).", maxSize/(1024*1024)))
	}
	reader, err := b.bot.File(&doc.File)
	if err != nil {
		return c.Send("Не удалось получить файл из Telegram.")
	}
	defer func() { _ = reader.Close() }()
	f, err := os.Create(destPath)
	if err != nil {
		return c.Send("Не удалось сохранить файл.")
	}
	defer func() { _ = f.Close() }()
	if _, err := io.Copy(f, io.LimitReader(reader, maxSize)); err != nil {
		return c.Send("Не удалось записать файл.")
	}
	b.log.Info("file updated", "path", destPath)
	return c.Send(successMsg)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

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
