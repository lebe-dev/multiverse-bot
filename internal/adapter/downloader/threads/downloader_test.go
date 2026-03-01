package threads

import (
	"testing"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

func TestDownloader_Supports(t *testing.T) {
	d := New("test-agent")

	if !d.Supports(domain.PlatformThreads) {
		t.Error("should support PlatformThreads")
	}
	if d.Supports(domain.PlatformYouTube) {
		t.Error("should not support PlatformYouTube")
	}
	if d.Supports(domain.PlatformInstagram) {
		t.Error("should not support PlatformInstagram")
	}
	if d.Supports(domain.PlatformTwitter) {
		t.Error("should not support PlatformTwitter")
	}
}
