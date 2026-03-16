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
	d := New("/usr/bin/yt-dlp", "/tmp/cookies.txt", 50*1024*1024)

	if d.execPath != "/usr/bin/yt-dlp" {
		t.Errorf("expected execPath /usr/bin/yt-dlp, got %s", d.execPath)
	}
	if d.cookiesFile != "/tmp/cookies.txt" {
		t.Errorf("expected cookiesFile /tmp/cookies.txt, got %s", d.cookiesFile)
	}
	if d.maxSize != 50*1024*1024 {
		t.Errorf("expected maxSize 50MB, got %d", d.maxSize)
	}
	if !d.Supports(1) { // PlatformYouTube
		t.Error("expected YouTube to be supported")
	}
}

func TestQualityFormats(t *testing.T) {
	if len(qualityFormats) == 0 {
		t.Fatal("qualityFormats should not be empty")
	}
	// First should be best quality, last should be worst
	if qualityFormats[0] != "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best" {
		t.Errorf("first format should be best quality, got %s", qualityFormats[0])
	}
	if qualityFormats[len(qualityFormats)-1] != "worst[ext=mp4]/worst" {
		t.Errorf("last format should be worst quality, got %s", qualityFormats[len(qualityFormats)-1])
	}
}
