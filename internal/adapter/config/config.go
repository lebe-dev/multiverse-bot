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

func parseBool(s string) bool {
	b, _ := strconv.ParseBool(s)
	return b
}

const defaultBrowserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

type Config struct {
	TelegramToken    string
	LogLevel         slog.Level
	AllowedUsers     []string
	AdminUsers       []string
	CobaltAPIURL     string
	TGLimit        int64  // max file size for standard Telegram API (bytes)
	LocalBotAPIURL string // local Telegram Bot API server URL for large files
	YtdlpPath      string
	BrowserUserAgent string
	ThreadsEngine    string
	YouTubeEngine    string

	// Per-user OAuth2 for Google Drive via Device Flow.
	GoogleClientID     string
	GoogleClientSecret string

	// Storage paths
	SettingsFile    string // per-user quality/caption prefs
	DriveTokensFile string // per-user Google Drive OAuth tokens
	AdminChatsFile  string // admin username → chat ID mapping

	// Plugins
	PluginsConfig string // path to plugins.yml

	// Debug mode — verbose error details sent to admin chats.
	Debug bool

	// YouTube watcher
	WatchPollInterval     time.Duration
	WatchMaxSubs          int
	WatchMaxChannelsTotal int
	SQLitePath            string

	// Auto-download: send video file immediately instead of notification with button.
	WatchAutoDownload bool

	// Instagram features toggle (default: false — all Instagram functionality disabled).
	InstagramFeaturesEnabled bool

	// Instagram story watcher
	WatchInstagramPollInterval time.Duration

	// Instagram post watcher
	WatchInstagramPostsPollInterval time.Duration
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
		LocalBotAPIURL: os.Getenv("LOCAL_BOT_API_URL"),
		YtdlpPath:      getEnvOrDefault("YTDLP_PATH", "yt-dlp"),
		BrowserUserAgent: getEnvOrDefault("BROWSER_USER_AGENT", defaultBrowserUserAgent),
		ThreadsEngine:    getEnvOrDefault("THREADS_ENGINE", "default"),
		YouTubeEngine:    getEnvOrDefault("YOUTUBE_ENGINE", "default"),

		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		SettingsFile:       getEnvOrDefault("SETTINGS_FILE", "./user_settings.json"),
		DriveTokensFile:    getEnvOrDefault("DRIVE_TOKENS_FILE", "./user_drive_tokens.json"),
		AdminChatsFile:     getEnvOrDefault("ADMIN_CHATS_FILE", "./data/admin_chats.json"),

		PluginsConfig: os.Getenv("PLUGINS_CONFIG"),

		Debug: parseBool(os.Getenv("DEBUG")),

		WatchAutoDownload: parseBool(os.Getenv("WATCH_AUTO_DOWNLOAD")),

		InstagramFeaturesEnabled: parseBool(os.Getenv("INSTAGRAM_FEATURES_ENABLED")),

		WatchInstagramPollInterval: parseDuration(os.Getenv("WATCH_INSTAGRAM_POLL_INTERVAL"), "24h"),

		WatchInstagramPostsPollInterval: parseDuration(os.Getenv("WATCH_INSTAGRAM_POSTS_POLL_INTERVAL"), "24h"),

		WatchPollInterval:     parseDuration(os.Getenv("WATCH_POLL_INTERVAL"), "15m"),
		WatchMaxSubs:          parseInt(os.Getenv("WATCH_MAX_SUBSCRIPTIONS"), 20),
		WatchMaxChannelsTotal: parseInt(os.Getenv("WATCH_MAX_CHANNELS_TOTAL"), 100),
		SQLitePath:            getEnvOrDefault("SQLITE_PATH", "./data/bot.db"),
	}

	return cfg, nil
}

// parseMegabytes reads an env var as a megabyte integer and returns bytes.
// Case-insensitive "MB" suffix is optional.
func parseMegabytes(key string, defaultMB int64) int64 {
	v := strings.ToUpper(strings.TrimSpace(os.Getenv(key)))
	v = strings.TrimSuffix(v, "MB")
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
