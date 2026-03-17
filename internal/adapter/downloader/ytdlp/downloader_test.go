package ytdlp

import (
	"testing"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "threads.com to threads.net",
			input:    "https://www.threads.com/@user/post/abc123",
			expected: "https://www.threads.net/@user/post/abc123",
		},
		{
			name:     "already threads.net",
			input:    "https://www.threads.net/@user/post/abc123",
			expected: "https://www.threads.net/@user/post/abc123",
		},
		{
			name:     "youtube unchanged",
			input:    "https://www.youtube.com/watch?v=abc",
			expected: "https://www.youtube.com/watch?v=abc",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeURL(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeURL(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNew(t *testing.T) {
	d := New("/usr/bin/yt-dlp", "/tmp/cookies.txt")

	if d.execPath != "/usr/bin/yt-dlp" {
		t.Errorf("expected execPath /usr/bin/yt-dlp, got %s", d.execPath)
	}
	if d.cookiesFile != "/tmp/cookies.txt" {
		t.Errorf("expected cookiesFile /tmp/cookies.txt, got %s", d.cookiesFile)
	}
	if !d.Supports(1) { // PlatformYouTube
		t.Error("expected YouTube to be supported")
	}
}

func TestFormats(t *testing.T) {
	if format720 == "" {
		t.Error("format720 must not be empty")
	}
	if formatBest == "" {
		t.Error("formatBest must not be empty")
	}
}
