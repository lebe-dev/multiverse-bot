package config

import (
	"os"
	"testing"
)

func TestLoad_MissingToken(t *testing.T) {
	os.Unsetenv("TELEGRAM_BOT_TOKEN")
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
	if cfg.MaxFileSize != 50*1024*1024 {
		t.Errorf("expected MaxFileSize 50MB, got %d", cfg.MaxFileSize)
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
