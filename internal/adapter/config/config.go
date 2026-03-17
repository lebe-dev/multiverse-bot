package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const defaultBrowserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

type Config struct {
	TelegramToken        string
	LogLevel             slog.Level
	AllowedUsers         []string
	AdminUsers           []string
	CobaltAPIURL         string
	MaxFileSize          int64
	CookiesFile          string
	YtdlpPath            string
	BrowserUserAgent     string
	ThreadsEngine        string
	YouTubeEngine        string
	WatchPollInterval    time.Duration
	WatchMaxSubs         int
	WatchMaxChannelsTotal int
	SQLitePath           string
}

func Load() (*Config, error) {
	_ = godotenv.Load(".env")

	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}

	cfg := &Config{
		TelegramToken:         token,
		LogLevel:              parseLogLevel(os.Getenv("LOG_LEVEL")),
		AllowedUsers:          parseAllowedUsers(os.Getenv("ALLOWED_USERS")),
		AdminUsers:            parseAllowedUsers(os.Getenv("ADMIN_USERS")),
		CobaltAPIURL:          getEnvOrDefault("COBALT_API_URL", "https://api.cobalt.tools"),
		MaxFileSize:           50 * 1024 * 1024, // 50MB
		CookiesFile:           getEnvOrDefault("YTDLP_COOKIES_FILE", "./cookies.txt"),
		YtdlpPath:             getEnvOrDefault("YTDLP_PATH", "yt-dlp"),
		BrowserUserAgent:      getEnvOrDefault("BROWSER_USER_AGENT", defaultBrowserUserAgent),
		ThreadsEngine:         getEnvOrDefault("THREADS_ENGINE", "default"),
		YouTubeEngine:         getEnvOrDefault("YOUTUBE_ENGINE", "default"),
		WatchPollInterval:     parseDuration(os.Getenv("WATCH_POLL_INTERVAL"), "15m"),
		WatchMaxSubs:          parseInt(os.Getenv("WATCH_MAX_SUBSCRIPTIONS"), 20),
		WatchMaxChannelsTotal: parseInt(os.Getenv("WATCH_MAX_CHANNELS_TOTAL"), 100),
		SQLitePath:            getEnvOrDefault("SQLITE_PATH", "./data/bot.db"),
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

func parseDuration(s, defaultVal string) time.Duration {
	if s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			return d
		}
	}
	d, _ := time.ParseDuration(defaultVal)
	return d
}

func parseInt(s string, defaultVal int) int {
	if s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			return n
		}
	}
	return defaultVal
}
