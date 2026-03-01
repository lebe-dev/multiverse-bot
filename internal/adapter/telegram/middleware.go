package telegram

import (
	"log/slog"
	"strings"

	tele "gopkg.in/telebot.v4"
)

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
