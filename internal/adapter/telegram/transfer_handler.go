package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	tele "gopkg.in/telebot.v4"

	"gitlab.com/tiny-services/multiverse-bot/internal/usecase"
)

const (
	maxImportSize   = 1 << 20 // 1 MB
	exportTimeout   = 30 * time.Second
	importTimeout   = 5 * time.Minute
)

func (b *Bot) RegisterTransferHandlers(transferSvc *usecase.TransferService) {
	b.transferSvc = transferSvc
	b.bot.Handle("/export", b.handleExport)
	b.bot.Handle("/import", b.handleImportCommand)
}

func (b *Bot) handleExport(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), exportTimeout)
	defer cancel()

	userID := c.Sender().ID
	st := b.userSettings(userID)
	settings := &usecase.SettingsExport{
		Quality: st.Quality,
		Caption: st.Caption,
	}

	data, err := b.transferSvc.Export(ctx, userID, settings)
	if err != nil {
		b.log.Error("export failed", "user_id", userID, "error", err)
		return c.Send("Не удалось выполнить экспорт.")
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		b.log.Error("export marshal failed", "error", err)
		return c.Send("Не удалось сформировать файл экспорта.")
	}

	filename := fmt.Sprintf("multiverse-export-%s.json", time.Now().Format("2006-01-02"))
	doc := &tele.Document{
		File: tele.FromReader(bytes.NewReader(jsonData)),
	}
	doc.FileName = filename
	doc.Caption = "Для импорта на другом сервере: /import"

	_, err = b.bot.Send(c.Recipient(), doc)
	return err
}

func (b *Bot) handleImportCommand(c tele.Context) error {
	b.pendingImport.Store(c.Sender().ID, struct{}{})
	return c.Send("📥 Отправьте файл экспорта (JSON), полученный через /export.")
}

func (b *Bot) handleImportFile(c tele.Context, doc *tele.Document) error {
	if doc.FileSize > maxImportSize {
		return c.Send("Файл слишком большой (макс. 1 МБ).")
	}

	reader, err := b.bot.File(&doc.File)
	if err != nil {
		return c.Send("Не удалось получить файл из Telegram.")
	}
	defer func() { _ = reader.Close() }()

	data, err := io.ReadAll(io.LimitReader(reader, maxImportSize))
	if err != nil {
		return c.Send("Не удалось прочитать файл.")
	}

	var exportData usecase.ExportData
	if err := json.Unmarshal(data, &exportData); err != nil {
		return c.Send("Файл не является валидным JSON.")
	}

	statusMsg, _ := b.bot.Send(c.Recipient(), "⏳ Импортирую...")

	ctx, cancel := context.WithTimeout(context.Background(), importTimeout)
	defer cancel()

	userID := c.Sender().ID

	// Apply settings before subscriptions.
	settingsApplied := false
	if exportData.Settings != nil && b.settings != nil {
		b.settings.SetQuality(userID, exportData.Settings.Quality)
		b.settings.SetCaption(userID, exportData.Settings.Caption)
		settingsApplied = true
	}

	result, err := b.transferSvc.Import(ctx, userID, &exportData)
	b.deleteMsg(statusMsg)

	if err != nil {
		if err == usecase.ErrUnsupportedVersion {
			return c.Send(fmt.Sprintf("Неподдерживаемая версия файла экспорта: %d.", exportData.Version))
		}
		b.log.Error("import failed", "user_id", userID, "error", err)
		return c.Send("Не удалось выполнить импорт.")
	}

	if result != nil {
		result.SettingsApplied = settingsApplied
	}

	return c.Send(formatImportResult(result))
}

func formatImportResult(r *usecase.ImportResult) string {
	if r == nil {
		return "Импорт завершён."
	}

	var sb strings.Builder
	sb.WriteString("✅ Импорт завершён:\n")

	hasYT := r.YoutubeAdded > 0 || r.YoutubeSkipped > 0 || r.YoutubeFailed > 0
	hasStories := r.StoriesAdded > 0 || r.StoriesSkipped > 0 || r.StoriesFailed > 0
	hasPosts := r.PostsAdded > 0 || r.PostsSkipped > 0 || r.PostsFailed > 0

	if hasYT {
		fmt.Fprintf(&sb, "\nYouTube: %d добавлено", r.YoutubeAdded)
		if r.YoutubeSkipped > 0 {
			fmt.Fprintf(&sb, ", %d пропущено", r.YoutubeSkipped)
		}
		if r.YoutubeFailed > 0 {
			fmt.Fprintf(&sb, ", ⚠️ %d не удалось", r.YoutubeFailed)
		}
	}

	if hasStories {
		fmt.Fprintf(&sb, "\nInstagram сторис: %d добавлено", r.StoriesAdded)
		if r.StoriesSkipped > 0 {
			fmt.Fprintf(&sb, ", %d пропущено", r.StoriesSkipped)
		}
		if r.StoriesFailed > 0 {
			fmt.Fprintf(&sb, ", ⚠️ %d не удалось", r.StoriesFailed)
		}
	}

	if hasPosts {
		fmt.Fprintf(&sb, "\nInstagram посты: %d добавлено", r.PostsAdded)
		if r.PostsSkipped > 0 {
			fmt.Fprintf(&sb, ", %d пропущено", r.PostsSkipped)
		}
		if r.PostsFailed > 0 {
			fmt.Fprintf(&sb, ", ⚠️ %d не удалось", r.PostsFailed)
		}
	}

	if r.SettingsApplied {
		sb.WriteString("\nНастройки: применены")
	}

	if !hasYT && !hasStories && !hasPosts && !r.SettingsApplied {
		sb.WriteString("\nФайл не содержал данных для импорта.")
	}

	return sb.String()
}
