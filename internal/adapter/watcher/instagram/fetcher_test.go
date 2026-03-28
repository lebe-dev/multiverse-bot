package instagram

import (
	"os"
	"path/filepath"
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

func TestFindFileByID(t *testing.T) {
	tmpDir := t.TempDir()

	// Create files simulating yt-dlp output for multiple stories.
	files := []string{"AAA.mp4", "BBB.mp4", "CCC.mp4"}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		storyID  string
		wantFile string
	}{
		{"AAA", "AAA.mp4"},
		{"BBB", "BBB.mp4"},
		{"CCC", "CCC.mp4"},
		{"MISSING", "AAA.mp4"}, // fallback to first entry
	}
	for _, tt := range tests {
		t.Run(tt.storyID, func(t *testing.T) {
			got := findFileByID(entries, tmpDir, tt.storyID)
			if filepath.Base(got) != tt.wantFile {
				t.Errorf("findFileByID(%q) = %q, want %q", tt.storyID, filepath.Base(got), tt.wantFile)
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
