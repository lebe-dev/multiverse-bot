package config

import (
	"log/slog"
	"os"
	"testing"
)

func TestLoad_MissingToken(t *testing.T) {
	_ = os.Unsetenv("TELEGRAM_BOT_TOKEN")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error when TELEGRAM_BOT_TOKEN is missing")
	}
}

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	t.Setenv("COBALT_API_URL", "")
	t.Setenv("YTDLP_PATH", "")
	t.Setenv("YTDLP_COOKIES_FILE", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("ALLOWED_USERS", "")
	t.Setenv("ADMIN_USERS", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.CobaltAPIURL != "https://api.cobalt.tools" {
		t.Errorf("expected default CobaltAPIURL, got %s", cfg.CobaltAPIURL)
	}
	if cfg.YtdlpPath != "yt-dlp" {
		t.Errorf("expected default YtdlpPath 'yt-dlp', got %s", cfg.YtdlpPath)
	}
	if cfg.CookiesFile != "./cookies.txt" {
		t.Errorf("expected default CookiesFile './cookies.txt', got %s", cfg.CookiesFile)
	}
	if cfg.TGLimit != 50*1024*1024 {
		t.Errorf("expected TGLimit 50MB, got %d", cfg.TGLimit)
	}
	if cfg.ThreadsEngine != "default" {
		t.Errorf("expected default ThreadsEngine 'default', got %s", cfg.ThreadsEngine)
	}
}

func TestLoad_ThreadsEngine(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	t.Setenv("THREADS_ENGINE", "lovethreads")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ThreadsEngine != "lovethreads" {
		t.Errorf("expected ThreadsEngine 'lovethreads', got %s", cfg.ThreadsEngine)
	}
}

func TestLoad_CustomYtdlpPath(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	t.Setenv("YTDLP_PATH", "/custom/path/yt-dlp")
	t.Setenv("YTDLP_COOKIES_FILE", "/custom/cookies.txt")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.YtdlpPath != "/custom/path/yt-dlp" {
		t.Errorf("expected custom YtdlpPath, got %s", cfg.YtdlpPath)
	}
	if cfg.CookiesFile != "/custom/cookies.txt" {
		t.Errorf("expected custom CookiesFile, got %s", cfg.CookiesFile)
	}
}

func TestParseMegabytes(t *testing.T) {
	tests := []struct {
		name       string
		envVal     string
		defaultMB  int64
		wantBytes  int64
	}{
		{"empty uses default", "", 50, 50 * 1024 * 1024},
		{"plain number", "100", 50, 100 * 1024 * 1024},
		{"with MB suffix", "100MB", 50, 100 * 1024 * 1024},
		{"with mb suffix lowercase", "100mb", 50, 100 * 1024 * 1024},
		{"with spaces", " 100 MB ", 50, 100 * 1024 * 1024},
		{"invalid uses default", "abc", 50, 50 * 1024 * 1024},
		{"zero uses default", "0", 50, 50 * 1024 * 1024},
		{"negative uses default", "-10", 50, 50 * 1024 * 1024},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_PARSE_MB_" + tt.name
			t.Setenv(key, tt.envVal)
			got := parseMegabytes(key, tt.defaultMB)
			if got != tt.wantBytes {
				t.Errorf("parseMegabytes(%q, %d) = %d, want %d", tt.envVal, tt.defaultMB, got, tt.wantBytes)
			}
		})
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"", slog.LevelInfo},
		{"unknown", slog.LevelInfo},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseLogLevel(tt.input)
			if got != tt.want {
				t.Errorf("parseLogLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		defaultVal string
		wantStr    string
	}{
		{"valid duration", "30s", "15m", "30s"},
		{"valid minutes", "5m", "15m", "5m0s"},
		{"empty uses default", "", "15m", "15m0s"},
		{"invalid uses default", "notaduration", "10s", "10s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDuration(tt.input, tt.defaultVal)
			if got.String() != tt.wantStr {
				t.Errorf("parseDuration(%q, %q) = %v, want %v", tt.input, tt.defaultVal, got, tt.wantStr)
			}
		})
	}
}

func TestParseInt(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		defaultVal int
		want       int
	}{
		{"valid", "42", 10, 42},
		{"empty uses default", "", 10, 10},
		{"invalid uses default", "abc", 10, 10},
		{"zero", "0", 10, 0},
		{"negative", "-5", 10, -5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseInt(tt.input, tt.defaultVal)
			if got != tt.want {
				t.Errorf("parseInt(%q, %d) = %d, want %d", tt.input, tt.defaultVal, got, tt.want)
			}
		})
	}
}

func TestParseAllowedUsers(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"empty", "", 0},
		{"single", "user1", 1},
		{"multiple", "user1,user2,user3", 3},
		{"with @", "@user1,@user2", 2},
		{"with spaces", " user1 , user2 ", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAllowedUsers(tt.input)
			if len(result) != tt.expected {
				t.Errorf("parseAllowedUsers(%q) returned %d users, want %d", tt.input, len(result), tt.expected)
			}
		})
	}
}
