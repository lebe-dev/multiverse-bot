package plugin

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	tele "gopkg.in/telebot.v4"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

const (
	fileDownloadTimeout = 60 * time.Second
	maxPluginFileSize   = 50 * 1024 * 1024 // 50 MB (Telegram limit)
)

// Executor converts PluginActions into telebot calls.
type Executor struct {
	bot *tele.Bot
	log *slog.Logger
}

// NewExecutor creates a new plugin action executor.
func NewExecutor(bot *tele.Bot, log *slog.Logger) *Executor {
	return &Executor{bot: bot, log: log}
}

// Execute runs a list of plugin actions in order, returning on the first hard error.
func (e *Executor) Execute(c tele.Context, pluginName string, actions []domain.PluginAction) error {
	for _, action := range actions {
		if err := e.executeOne(c, pluginName, action); err != nil {
			return err
		}
	}
	return nil
}

func (e *Executor) executeOne(c tele.Context, pluginName string, a domain.PluginAction) error {
	switch a.Type {
	case "text":
		return e.sendText(c, pluginName, a)
	case "file":
		return e.sendFile(c, a)
	case "edit":
		return e.editMessage(c, a)
	case "delete":
		return e.deleteMessage(c, a)
	default:
		e.log.Warn("unknown plugin action type", "plugin", pluginName, "type", a.Type)
		return nil
	}
}

func (e *Executor) sendText(c tele.Context, pluginName string, a domain.PluginAction) error {
	opts := &tele.SendOptions{}

	switch a.ParseMode {
	case "HTML":
		opts.ParseMode = tele.ModeHTML
	case "Markdown":
		opts.ParseMode = tele.ModeMarkdown
	}

	if len(a.Buttons) > 0 {
		kb := &tele.ReplyMarkup{}
		var btns []tele.Btn
		for _, btn := range a.Buttons {
			if btn.URL != "" {
				if _, err := url.Parse(btn.URL); err != nil {
					e.log.Warn("plugin button URL invalid", "plugin", pluginName, "url", btn.URL)
					continue
				}
				btns = append(btns, kb.URL(btn.Text, btn.URL))
			} else if btn.Callback != "" {
				// Prefix callback data with p_<pluginName>| for routing.
				data := fmt.Sprintf("p_%s|%s", pluginName, btn.Callback)
				btns = append(btns, kb.Data(btn.Text, data))
			}
		}
		if len(btns) > 0 {
			kb.Inline(kb.Row(btns...))
			opts.ReplyMarkup = kb
		}
	}

	_, err := e.bot.Send(c.Recipient(), a.Text, opts)
	return err
}

func (e *Executor) sendFile(c tele.Context, a domain.PluginAction) error {
	if a.URL == "" {
		return nil
	}

	// Validate URL.
	u, err := url.Parse(a.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return c.Send("❌ Некорректный URL файла от плагина.")
	}

	_ = c.Notify(tele.UploadingDocument)

	client := &http.Client{Timeout: fileDownloadTimeout}
	resp, err := client.Get(a.URL)
	if err != nil {
		return c.Send("❌ Не удалось загрузить файл от плагина.")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return c.Send("❌ Не удалось загрузить файл от плагина.")
	}

	if resp.ContentLength > maxPluginFileSize {
		return c.Send("❌ Файл от плагина слишком большой.")
	}

	reader := io.LimitReader(resp.Body, maxPluginFileSize+1)

	filename := a.Filename
	if filename == "" {
		filename = "file"
	}

	mime := a.MimeType
	if mime == "" && resp.Header.Get("Content-Type") != "" {
		mime = resp.Header.Get("Content-Type")
	}

	switch {
	case strings.HasPrefix(mime, "video/"):
		_ = c.Notify(tele.UploadingVideo)
		video := &tele.Video{
			File:    tele.FromReader(reader),
			Caption: a.Caption,
		}
		video.FileName = filename
		_, err = e.bot.Send(c.Recipient(), video)
	case strings.HasPrefix(mime, "image/"):
		_ = c.Notify(tele.UploadingPhoto)
		photo := &tele.Photo{
			File:    tele.FromReader(reader),
			Caption: a.Caption,
		}
		_, err = e.bot.Send(c.Recipient(), photo)
	default:
		doc := &tele.Document{
			File:    tele.FromReader(reader),
			Caption: a.Caption,
		}
		doc.FileName = filename
		_, err = e.bot.Send(c.Recipient(), doc)
	}

	return err
}

func (e *Executor) editMessage(c tele.Context, a domain.PluginAction) error {
	if a.MessageID == 0 {
		return nil
	}
	msg := &tele.Message{
		ID:   a.MessageID,
		Chat: c.Chat(),
	}
	opts := &tele.SendOptions{}
	switch a.ParseMode {
	case "HTML":
		opts.ParseMode = tele.ModeHTML
	case "Markdown":
		opts.ParseMode = tele.ModeMarkdown
	}
	_, err := e.bot.Edit(msg, a.Text, opts)
	return err
}

func (e *Executor) deleteMessage(c tele.Context, a domain.PluginAction) error {
	if a.MessageID == 0 {
		return nil
	}
	msg := &tele.Message{
		ID:   a.MessageID,
		Chat: c.Chat(),
	}
	return e.bot.Delete(msg)
}
