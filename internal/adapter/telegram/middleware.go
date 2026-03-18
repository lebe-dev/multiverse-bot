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
