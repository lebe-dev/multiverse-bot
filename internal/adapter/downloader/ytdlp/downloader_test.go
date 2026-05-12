package ytdlp

import (
	"log/slog"
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
	cookieFn := func(_ string) string { return "/tmp/cookies.txt" }
	d := New("/usr/bin/yt-dlp", cookieFn, slog.Default())

	if d.execPath != "/usr/bin/yt-dlp" {
		t.Errorf("expected execPath /usr/bin/yt-dlp, got %s", d.execPath)
	}
	if got := d.cookiePath(""); got != "/tmp/cookies.txt" {
		t.Errorf("expected cookiePath /tmp/cookies.txt, got %s", got)
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
	if formatAudio == "" {
		t.Error("formatAudio must not be empty")
	}
}

func TestAudioFormat(t *testing.T) {
	// formatAudio must select audio-only streams (no video).
	for _, want := range []string{"bestaudio", "m4a"} {
		found := false
		for i := 0; i <= len(formatAudio)-len(want); i++ {
			if formatAudio[i:i+len(want)] == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("formatAudio = %q, want it to contain %q", formatAudio, want)
		}
	}
}

func TestQualityFormat(t *testing.T) {
	tests := []struct {
		quality  string
		contains string
	}{
		{"360p", "height<=360"},
		{"480p", "height<=480"},
		{"720p", "height<=720"},
		{"1080p", "height<=1080"},
		{"unknown", "height<=720"}, // defaults to 720p
		{"", "height<=720"},
	}
	for _, tt := range tests {
		t.Run(tt.quality, func(t *testing.T) {
			got := qualityFormat(tt.quality)
			if got == "" {
				t.Fatal("qualityFormat returned empty string")
			}
			found := false
			for i := 0; i <= len(got)-len(tt.contains); i++ {
				if got[i:i+len(tt.contains)] == tt.contains {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("qualityFormat(%q) = %q, want it to contain %q", tt.quality, got, tt.contains)
			}
		})
	}

	// Each quality should return a distinct format (except default → 720p).
	f360 := qualityFormat("360p")
	f720 := qualityFormat("720p")
	f1080 := qualityFormat("1080p")
	if f360 == f720 {
		t.Error("360p and 720p should produce different formats")
	}
	if f720 == f1080 {
		t.Error("720p and 1080p should produce different formats")
	}
}

func TestBuildSummary(t *testing.T) {
	intPtr := func(v int) *int { return &v }
	int64Ptr := func(v int64) *int64 { return &v }
	float64Ptr := func(v float64) *float64 { return &v }

	t.Run("basic formats", func(t *testing.T) {
		info := &ytdlpJSON{
			Title:    "Test Video",
			Duration: 120,
			Formats: []ytdlpFormat{
				{Height: intPtr(360), VCodec: "avc1", ACodec: "mp4a", FileSize: int64Ptr(5_000_000)},
				{Height: intPtr(720), VCodec: "avc1", ACodec: "mp4a", FileSize: int64Ptr(15_000_000)},
				{Height: intPtr(1080), VCodec: "avc1", ACodec: "mp4a", FileSize: int64Ptr(30_000_000)},
			},
		}
		s := buildSummary(info)

		if s.Title != "Test Video" {
			t.Errorf("Title = %q, want %q", s.Title, "Test Video")
		}
		if s.Duration != 120 {
			t.Errorf("Duration = %v, want 120", s.Duration)
		}
		if len(s.Entries) != 3 {
			t.Fatalf("len(Entries) = %d, want 3", len(s.Entries))
		}
		// Entries should be sorted by height.
		if s.Entries[0].Height != 360 || s.Entries[1].Height != 720 || s.Entries[2].Height != 1080 {
			t.Errorf("entries not sorted: %v", s.Entries)
		}
		if s.MinHeight != 360 {
			t.Errorf("MinHeight = %d, want 360", s.MinHeight)
		}
		if s.MaxHeight != 1080 {
			t.Errorf("MaxHeight = %d, want 1080", s.MaxHeight)
		}
		if s.P720Height != 720 {
			t.Errorf("P720Height = %d, want 720", s.P720Height)
		}
	})

	t.Run("video-only with audio track", func(t *testing.T) {
		info := &ytdlpJSON{
			Title:    "Split Tracks",
			Duration: 60,
			Formats: []ytdlpFormat{
				// Audio-only track
				{VCodec: "none", ACodec: "mp4a", FileSize: int64Ptr(1_000_000)},
				// Video-only track
				{Height: intPtr(720), VCodec: "avc1", ACodec: "none", FileSize: int64Ptr(10_000_000)},
			},
		}
		s := buildSummary(info)

		if len(s.Entries) != 1 {
			t.Fatalf("len(Entries) = %d, want 1", len(s.Entries))
		}
		// Should add audio size to video-only track
		if s.Entries[0].Size != 11_000_000 {
			t.Errorf("Entries[0].Size = %d, want 11000000 (10M video + 1M audio)", s.Entries[0].Size)
		}
	})

	t.Run("empty formats", func(t *testing.T) {
		info := &ytdlpJSON{Title: "Empty", Duration: 10}
		s := buildSummary(info)

		if len(s.Entries) != 0 {
			t.Errorf("len(Entries) = %d, want 0", len(s.Entries))
		}
		if s.MinHeight != 0 || s.MaxHeight != 0 {
			t.Errorf("expected zero heights for empty formats")
		}
	})

	t.Run("size from TBR estimation", func(t *testing.T) {
		info := &ytdlpJSON{
			Duration: 100,
			Formats: []ytdlpFormat{
				{Height: intPtr(480), VCodec: "avc1", ACodec: "mp4a", TBR: float64Ptr(800)},
			},
		}
		s := buildSummary(info)

		if len(s.Entries) != 1 {
			t.Fatalf("len(Entries) = %d, want 1", len(s.Entries))
		}
		// TBR 800 kbps * 100s = 800*1000/8*100 = 10_000_000 bytes
		expectedSize := int64(800 * 1000 / 8 * 100)
		if s.Entries[0].Size != expectedSize {
			t.Errorf("Size = %d, want %d (estimated from TBR)", s.Entries[0].Size, expectedSize)
		}
	})

	t.Run("nil height formats skipped", func(t *testing.T) {
		info := &ytdlpJSON{
			Duration: 10,
			Formats: []ytdlpFormat{
				{Height: nil, VCodec: "avc1", ACodec: "mp4a", FileSize: int64Ptr(1000)},
				{Height: intPtr(0), VCodec: "avc1", ACodec: "mp4a", FileSize: int64Ptr(2000)},
				{Height: intPtr(480), VCodec: "avc1", ACodec: "mp4a", FileSize: int64Ptr(5_000_000)},
			},
		}
		s := buildSummary(info)

		if len(s.Entries) != 1 {
			t.Fatalf("len(Entries) = %d, want 1", len(s.Entries))
		}
		if s.Entries[0].Height != 480 {
			t.Errorf("expected height 480, got %d", s.Entries[0].Height)
		}
	})

	t.Run("low resolution below 240 excluded from min", func(t *testing.T) {
		info := &ytdlpJSON{
			Duration: 10,
			Formats: []ytdlpFormat{
				{Height: intPtr(144), VCodec: "avc1", ACodec: "mp4a", FileSize: int64Ptr(500_000)},
				{Height: intPtr(360), VCodec: "avc1", ACodec: "mp4a", FileSize: int64Ptr(2_000_000)},
			},
		}
		s := buildSummary(info)

		if s.MinHeight != 360 {
			t.Errorf("MinHeight = %d, want 360 (144p should be excluded)", s.MinHeight)
		}
	})
}
