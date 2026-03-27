package instagram

import (
	"testing"
	"time"
)

func TestParsePostPlaylistJSON(t *testing.T) {
	tests := []struct {
		name      string
		data      string
		username  string
		wantCount int
		wantErr   bool
	}{
		{
			name: "multiple entries",
			data: `{
				"entries": [
					{"id": "abc123", "url": "https://www.instagram.com/p/abc123/", "title": "Post caption", "timestamp": 1700000000},
					{"id": "def456", "url": "https://www.instagram.com/p/def456/", "title": "", "timestamp": 1700100000}
				]
			}`,
			username:  "testuser",
			wantCount: 2,
		},
		{
			name:      "empty entries",
			data:      `{"entries": []}`,
			username:  "testuser",
			wantCount: 0,
		},
		{
			name: "entries without ID are skipped",
			data: `{
				"entries": [
					{"id": "", "url": "", "title": ""},
					{"id": "abc123", "url": "https://www.instagram.com/p/abc123/", "title": "ok", "timestamp": 1700000000}
				]
			}`,
			username:  "testuser",
			wantCount: 1,
		},
		{
			name:    "invalid JSON",
			data:    `not json`,
			wantErr: true,
		},
		{
			name:      "null entries",
			data:      `{"entries": null}`,
			username:  "testuser",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, err := parsePostPlaylistJSON([]byte(tt.data), tt.username)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parsePostPlaylistJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if len(items) != tt.wantCount {
				t.Fatalf("got %d items, want %d", len(items), tt.wantCount)
			}
		})
	}
}

func TestParsePostPlaylistJSON_Fields(t *testing.T) {
	data := `{
		"entries": [
			{"id": "abc123", "url": "https://www.instagram.com/p/abc123/", "title": "Hello world", "timestamp": 1700000000}
		]
	}`

	items, err := parsePostPlaylistJSON([]byte(data), "testuser")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatal("expected 1 item")
	}

	item := items[0]
	if item.PostID != "abc123" {
		t.Errorf("PostID = %q, want %q", item.PostID, "abc123")
	}
	if item.Username != "testuser" {
		t.Errorf("Username = %q, want %q", item.Username, "testuser")
	}
	if item.Title != "Hello world" {
		t.Errorf("Title = %q, want %q", item.Title, "Hello world")
	}
	if item.URL != "https://www.instagram.com/p/abc123/" {
		t.Errorf("URL = %q, want %q", item.URL, "https://www.instagram.com/p/abc123/")
	}
	if item.Timestamp != time.Unix(1700000000, 0) {
		t.Errorf("Timestamp = %v, want %v", item.Timestamp, time.Unix(1700000000, 0))
	}
}

func TestParsePostPlaylistJSON_FallbackURL(t *testing.T) {
	data := `{
		"entries": [
			{"id": "abc123", "url": "", "title": "", "timestamp": 0}
		]
	}`

	items, err := parsePostPlaylistJSON([]byte(data), "testuser")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatal("expected 1 item")
	}
	if items[0].URL != "https://www.instagram.com/p/abc123/" {
		t.Errorf("URL = %q, want fallback URL", items[0].URL)
	}
}
