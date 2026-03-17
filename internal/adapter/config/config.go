package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

const defaultBrowserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

type Config struct {
	TelegramToken    string
	LogLevel         slog.Level
	AllowedUsers     []string
	AdminUsers       []string
	CobaltAPIURL     string
	TGLimit          int64  // max file size for standard Telegram API (bytes)
	LocalBotAPIURL   string // local Telegram Bot API server URL for large files
	CookiesFile      string
	YtdlpPath        string
	BrowserUserAgent string
	ThreadsEngine    string
	YouTubeEngine    string
	GDriveKeyFile    string
	GDriveFolderID   string

	// Per-user OAuth2 for Google Drive via Device Flow (drive.file scope).
	// Requires a "Desktop app" OAuth2 client in Google Cloud Console — no redirect URI needed.
	GoogleClientID     string
	GoogleClientSecret string
}

func Load() (*Config, error) {
	_ = godotenv.Load(".env")

	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}

	cfg := &Config{
		TelegramToken:    token,
		LogLevel:         parseLogLevel(os.Getenv("LOG_LEVEL")),
		AllowedUsers:     parseAllowedUsers(os.Getenv("ALLOWED_USERS")),
		AdminUsers:       parseAllowedUsers(os.Getenv("ADMIN_USERS")),
		CobaltAPIURL:     getEnvOrDefault("COBALT_API_URL", "https://api.cobalt.tools"),
		TGLimit:          parseMegabytes("TG_LIMIT", 50),
		LocalBotAPIURL:   os.Getenv("LOCAL_BOT_API_URL"),
		CookiesFile:      getEnvOrDefault("YTDLP_COOKIES_FILE", "./cookies.txt"),
		YtdlpPath:        getEnvOrDefault("YTDLP_PATH", "yt-dlp"),
		BrowserUserAgent: getEnvOrDefault("BROWSER_USER_AGENT", defaultBrowserUserAgent),
		ThreadsEngine:    getEnvOrDefault("THREADS_ENGINE", "default"),
		YouTubeEngine:    getEnvOrDefault("YOUTUBE_ENGINE", "default"),
		GDriveKeyFile:    getEnvOrDefault("GDRIVE_KEY_FILE", "./gdrive-key.json"),
		GDriveFolderID:   os.Getenv("GDRIVE_FOLDER_ID"),

		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
	}

	return cfg, nil
}

// parseMegabytes reads an env var as a megabyte integer and returns bytes.
func parseMegabytes(key string, defaultMB int64) int64 {
	v := os.Getenv(key)
	// strip optional "MB" suffix
	v = strings.TrimSuffix(strings.TrimSpace(v), "MB")
	v = strings.TrimSuffix(v, "mb")
	if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil && n > 0 {
		return n * 1024 * 1024
	}
	return defaultMB * 1024 * 1024
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
