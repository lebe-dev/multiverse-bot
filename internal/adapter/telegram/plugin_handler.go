package telegram

import (
	"context"
	"strings"
	"time"

	tele "gopkg.in/telebot.v4"

	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/plugin"
	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

const pluginExecuteTimeout = 60 * time.Second

// handlePluginCommand handles a slash command routed to a plugin.
func (b *Bot) handlePluginCommand(c tele.Context, pluginName, command string) error {
	client, _ := b.plugins.FindByCommand(command)
	if client == nil {
		return c.Send("Плагин недоступен.")
	}

	args := strings.TrimSpace(c.Message().Payload)

	req := &domain.PluginRequest{
		Trigger: domain.PluginTrigger{
			Type:    "command",
			Command: command,
			Args:    args,
			RawText: c.Text(),
		},
		User: domain.PluginUser{
			ID:       c.Sender().ID,
			Username: c.Sender().Username,
		},
		MessageID: c.Message().ID,
	}

	return b.executePlugin(c, client, pluginName, req)
}

// handlePluginURL handles a URL matched by a plugin pattern.
func (b *Bot) handlePluginURL(c tele.Context, client domain.PluginClient, pluginName, url, matchedPattern string) error {
	req := &domain.PluginRequest{
		Trigger: domain.PluginTrigger{
			Type:           "url",
			URL:            url,
			MatchedPattern: matchedPattern,
			RawText:        c.Text(),
		},
		User: domain.PluginUser{
			ID:       c.Sender().ID,
			Username: c.Sender().Username,
		},
		MessageID: c.Message().ID,
	}

	return b.executePlugin(c, client, pluginName, req)
}

// executePlugin calls a plugin's /execute endpoint and processes the response actions.
func (b *Bot) executePlugin(c tele.Context, client domain.PluginClient, pluginName string, req *domain.PluginRequest) error {
	ctx, cancel := context.WithTimeout(context.Background(), pluginExecuteTimeout)
	defer cancel()

	b.log.Info("plugin executing",
		"user", c.Sender().Username,
		"user_id", c.Sender().ID,
		"plugin", pluginName,
	)

	resp, err := client.Execute(ctx, req)
	if err != nil {
		b.log.Error("plugin execute failed", "plugin", pluginName, "user", c.Sender().Username, "error", err)
		return c.Send("⚠️ Ошибка плагина. Попробуйте позже.")
	}

	if resp.Error != "" {
		return c.Send(resp.Error)
	}

	executor := plugin.NewExecutor(b.bot, b.log)
	return executor.Execute(c, pluginName, resp.Actions)
}

// handlePluginCallback handles a callback routed to a plugin (data starts with "p_<name>|").
func (b *Bot) handlePluginCallback(c tele.Context, pluginName, callbackID string) error {
	client := b.plugins.FindByName(pluginName)
	if client == nil {
		_ = c.Respond(&tele.CallbackResponse{Text: "Плагин недоступен"})
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), pluginExecuteTimeout)
	defer cancel()

	resp, err := client.Callback(ctx, &domain.PluginCallbackRequest{
		CallbackID: callbackID,
		User: domain.PluginUser{
			ID:       c.Sender().ID,
			Username: c.Sender().Username,
		},
		MessageID: c.Message().ID,
	})

	if err != nil {
		b.log.Error("plugin callback failed", "plugin", pluginName, "error", err)
		_ = c.Respond(&tele.CallbackResponse{Text: "Ошибка плагина"})
		return nil
	}

	if resp.Toast != "" {
		_ = c.Respond(&tele.CallbackResponse{Text: resp.Toast})
	} else {
		_ = c.Respond()
	}

	if resp.Error != "" {
		return c.Send(resp.Error)
	}

	if len(resp.Actions) > 0 {
		executor := plugin.NewExecutor(b.bot, b.log)
		return executor.Execute(c, pluginName, resp.Actions)
	}

	return nil
}
