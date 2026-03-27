package instagram

import (
	"testing"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

func TestParsePlaylistJSON(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		want    int
		wantErr bool
	}{
		{
			name: "multiple entries",
			json: `{"entries": [
				{"id": "123", "url": "https://...", "timestamp": 1711234567},
				{"id": "456", "url": "https://...", "timestamp": 1711234890}
			]}`,
			want: 2,
		},
		{
			name: "empty entries",
			json: `{"entries": []}`,
			want: 0,
		},
		{
			name: "null entries",
			json: `{"entries": null}`,
			want: 0,
		},
		{
			name:    "invalid json",
			json:    `{invalid}`,
			wantErr: true,
		},
		{
			name: "entry without id is skipped",
			json: `{"entries": [{"id": "", "url": "https://..."}, {"id": "789", "url": "https://..."}]}`,
			want: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, err := parsePlaylistJSON([]byte(tt.json), "testuser")
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(items) != tt.want {
				t.Errorf("got %d items, want %d", len(items), tt.want)
			}
			for _, item := range items {
				if item.Username != "testuser" {
					t.Errorf("expected username 'testuser', got %q", item.Username)
				}
			}
		})
	}
}

func TestDetectMediaType(t *testing.T) {
	tests := []struct {
		path string
		want domain.MediaType
	}{
		{"/tmp/story.mp4", domain.MediaVideo},
		{"/tmp/story.mkv", domain.MediaVideo},
		{"/tmp/story.webm", domain.MediaVideo},
		{"/tmp/story.mov", domain.MediaVideo},
		{"/tmp/story.jpg", domain.MediaPhoto},
		{"/tmp/story.webp", domain.MediaPhoto},
		{"/tmp/story.png", domain.MediaPhoto},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := detectMediaType(tt.path)
			if got != tt.want {
				t.Errorf("detectMediaType(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
