package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	TelegramToken string
	LogLevel      slog.Level
	AllowedUsers  []string
	AdminUsers    []string
	CobaltAPIURL  string
	MaxFileSize   int64
}

func Load() (*Config, error) {
	_ = godotenv.Load(".env")

	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}

	cfg := &Config{
		TelegramToken: token,
		LogLevel:      parseLogLevel(os.Getenv("LOG_LEVEL")),
		AllowedUsers:  parseAllowedUsers(os.Getenv("ALLOWED_USERS")),
		AdminUsers:    parseAllowedUsers(os.Getenv("ADMIN_USERS")),
		CobaltAPIURL:  getEnvOrDefault("COBALT_API_URL", "https://api.cobalt.tools"),
		MaxFileSize:   50 * 1024 * 1024, // 50MB
	}

	return cfg, nil
}

func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func parseAllowedUsers(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	users := make([]string, 0, len(parts))
	for _, p := range parts {
		u := strings.TrimSpace(p)
		u = strings.TrimPrefix(u, "@")
		if u != "" {
			users = append(users, u)
		}
	}
	return users
}

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
