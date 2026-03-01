package detector_test

import (
	"testing"

	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/detector"
	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

func TestRegexDetector_Detect(t *testing.T) {
	d := detector.New()

	tests := []struct {
		name     string
		url      string
		expected domain.Platform
	}{
		{"youtube long", "https://www.youtube.com/watch?v=dQw4w9WgXcQ", domain.PlatformYouTube},
		{"youtube short", "https://youtu.be/dQw4w9WgXcQ", domain.PlatformYouTube},
		{"instagram reel", "https://www.instagram.com/reel/ABC123/", domain.PlatformInstagram},
		{"instagram post", "https://www.instagram.com/p/ABC123/", domain.PlatformInstagram},
		{"instagram reels", "https://www.instagram.com/reels/ABC123/", domain.PlatformInstagram},
		{"twitter", "https://twitter.com/user/status/123456", domain.PlatformTwitter},
		{"x.com", "https://x.com/user/status/123456", domain.PlatformTwitter},
		{"threads.net", "https://www.threads.net/@user/post/ABC123", domain.PlatformThreads},
		{"threads.com", "https://www.threads.com/@vuthiduyen48/post/DVTksS6iWNk", domain.PlatformThreads},
		{"unknown", "https://example.com/video", domain.PlatformUnknown},
		{"empty", "", domain.PlatformUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := d.Detect(tt.url)
			if got != tt.expected {
				t.Errorf("Detect(%q) = %v, want %v", tt.url, got, tt.expected)
			}
		})
	}
}
