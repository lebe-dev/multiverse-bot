package savevids

import (
	"testing"
)

func TestPickBestVideo(t *testing.T) {
	resources := []resource{
		{Type: "video", Quality: "360p", DownloadURL: "https://example.com/360.mp4", Size: 5_000_000},
		{Type: "video", Quality: "720p", DownloadURL: "https://example.com/720.mp4", Size: 20_000_000},
		{Type: "video", Quality: "1080p", DownloadURL: "https://example.com/1080.mp4", Size: 60_000_000},
		{Type: "audio", Quality: "128k", DownloadURL: "https://example.com/audio.m4a", Size: 3_000_000},
	}

	t.Run("picks best fitting within limit", func(t *testing.T) {
		url, err := pickBestVideo(resources, 50_000_000)
		if err != nil {
			t.Fatal(err)
		}
		if url != "https://example.com/720.mp4" {
			t.Errorf("expected 720p, got %s", url)
		}
	})

	t.Run("picks best when no limit", func(t *testing.T) {
		url, err := pickBestVideo(resources, 0)
		if err != nil {
			t.Fatal(err)
		}
		if url != "https://example.com/1080.mp4" {
			t.Errorf("expected 1080p, got %s", url)
		}
	})

	t.Run("returns smallest when all exceed limit", func(t *testing.T) {
		url, err := pickBestVideo(resources, 1_000_000)
		if err != nil {
			t.Fatal(err)
		}
		if url != "https://example.com/360.mp4" {
			t.Errorf("expected 360p, got %s", url)
		}
	})

	t.Run("ignores audio resources", func(t *testing.T) {
		audioOnly := []resource{
			{Type: "audio", Quality: "128k", DownloadURL: "https://example.com/audio.m4a", Size: 3_000_000},
		}
		_, err := pickBestVideo(audioOnly, 50_000_000)
		if err == nil {
			t.Error("expected error for audio-only resources")
		}
	})

	t.Run("error on empty resources", func(t *testing.T) {
		_, err := pickBestVideo(nil, 50_000_000)
		if err == nil {
			t.Error("expected error for empty resources")
		}
	})
}
