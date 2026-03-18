package lovethreads

import (
	"log/slog"
	"testing"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

func TestDownloader_Supports(t *testing.T) {
	d := New(slog.Default())

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
