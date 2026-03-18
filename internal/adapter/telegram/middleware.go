package telegram

import (
	"log/slog"
	"strings"

	tele "gopkg.in/telebot.v4"
)

func (b *Bot) adminTrackMiddleware() tele.MiddlewareFunc {
	return func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			if b.IsAdmin(c.Sender().Username) {
				b.adminChats.Track(c.Sender().Username, c.Chat().ID)
			}
			return next(c)
		}
	}
}

func requestLogMiddleware(log *slog.Logger) tele.MiddlewareFunc {
	return func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			sender := c.Sender()
			if sender == nil {
				return next(c)
			}

			user := sender.Username
			userID := sender.ID

			switch {
			case c.Callback() != nil:
				data := strings.TrimPrefix(c.Callback().Data, "\f")
				log.Info("callback",
					"user", user,
					"user_id", userID,
					"data", data,
				)
			case c.Message() != nil && c.Message().Document != nil:
				log.Info("document received",
					"user", user,
					"user_id", userID,
					"file", c.Message().Document.FileName,
					"size", c.Message().Document.FileSize,
				)
			case c.Message() != nil:
				text := c.Message().Text
				if strings.HasPrefix(text, "/") {
					parts := strings.Fields(text)
					log.Info("command",
						"user", user,
						"user_id", userID,
						"cmd", parts[0],
					)
				} else {
					log.Debug("text message",
						"user", user,
						"user_id", userID,
					)
				}
			}

			return next(c)
		}
	}
}

func whitelistMiddleware(allowed map[string]bool, log *slog.Logger) tele.MiddlewareFunc {
	return func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			username := strings.ToLower(c.Sender().Username)
			if !allowed[username] {
				log.Warn("unauthorized access attempt",
					"username", c.Sender().Username,
					"user_id", c.Sender().ID,
				)
				return nil
			}
			return next(c)
		}
	}
}
