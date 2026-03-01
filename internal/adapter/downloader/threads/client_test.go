package threads

import (
	"testing"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "threads.net with user and post",
			input: "https://www.threads.net/@user/post/ABC123",
			want:  "https://www.threads.com/@user/post/ABC123",
		},
		{
			name:  "threads.com with user and post",
			input: "https://threads.com/@user/post/ABC123",
			want:  "https://www.threads.com/@user/post/ABC123",
		},
		{
			name:  "trailing slash stripped",
			input: "https://www.threads.com/@user/post/ABC123/",
			want:  "https://www.threads.com/@user/post/ABC123",
		},
		{
			name:  "query params stripped",
			input: "https://www.threads.com/@user/post/ABC123?igsh=foo",
			want:  "https://www.threads.com/@user/post/ABC123",
		},
		{
			name:  "short URL /t/CODE",
			input: "https://www.threads.com/t/ABC123",
			want:  "https://www.threads.com/t/ABC123",
		},
		{
			name:  "short URL threads.net",
			input: "https://threads.net/t/ABC123",
			want:  "https://www.threads.com/t/ABC123",
		},
		{
			name:  "without scheme",
			input: "threads.com/@user/post/ABC123",
			want:  "https://www.threads.com/@user/post/ABC123",
		},
		{
			name:    "not a threads URL",
			input:   "https://instagram.com/p/ABC123",
			wantErr: true,
		},
		{
			name:    "bad path format",
			input:   "https://threads.com/something/else",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseSSRData(t *testing.T) {
	tests := []struct {
		name      string
		html      string
		wantCount int
		wantURL   string
		wantErr   bool
	}{
		{
			name: "single video post",
			html: `<html>some stuff BarcelonaPostPage more stuff "edges":[{"node":{"thread_items":[{"post":{"code":"ABC","media_type":2,"video_versions":[{"type":101,"url":"https://cdn.example.com/video_hd.mp4"},{"type":0,"url":"https://cdn.example.com/video_sd.mp4"}],"caption":{"text":"hello"},"user":{"username":"testuser"}}}]}}] more</html>`,
			wantCount: 1,
			wantURL:   "https://cdn.example.com/video_hd.mp4",
		},
		{
			name: "carousel with videos",
			html: `<html>BarcelonaPostPage "edges":[{"node":{"thread_items":[{"post":{"code":"XYZ","media_type":8,"carousel_media":[{"media_type":2,"video_versions":[{"type":0,"url":"https://cdn.example.com/v1.mp4"}]},{"media_type":1},{"media_type":2,"video_versions":[{"type":0,"url":"https://cdn.example.com/v2.mp4"}]}],"caption":{"text":"carousel"},"user":{"username":"bob"}}}]}}]</html>`,
			wantCount: 2,
			wantURL:   "https://cdn.example.com/v1.mp4",
		},
		{
			name:    "no marker",
			html:    `<html>no data here</html>`,
			wantErr: true,
		},
		{
			name:    "photo post",
			html:    `<html>BarcelonaPostPage "edges":[{"node":{"thread_items":[{"post":{"code":"IMG","media_type":1,"caption":{"text":"photo"},"user":{"username":"alice"}}}]}}]</html>`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			videos, err := parseSSRData(tt.html)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(videos) != tt.wantCount {
				t.Fatalf("got %d videos, want %d", len(videos), tt.wantCount)
			}
			if videos[0].URL != tt.wantURL {
				t.Errorf("got URL %q, want %q", videos[0].URL, tt.wantURL)
			}
		})
	}
}

func TestParseEmbedPage(t *testing.T) {
	tests := []struct {
		name    string
		html    string
		wantURL string
		wantErr bool
	}{
		{
			name:    "valid source tag",
			html:    `<video><source src="https://cdn.example.com/video.mp4?a=1&amp;b=2" type="video/mp4"></video>`,
			wantURL: "https://cdn.example.com/video.mp4?a=1&b=2",
		},
		{
			name:    "no source tag",
			html:    `<video></video>`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			videos, err := parseEmbedPage(tt.html)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(videos) == 0 {
				t.Fatal("no videos returned")
			}
			if videos[0].URL != tt.wantURL {
				t.Errorf("got URL %q, want %q", videos[0].URL, tt.wantURL)
			}
		})
	}
}

func TestFindMatchingBracket(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		start   int
		want    int
		wantErr bool
	}{
		{name: "simple array", input: `[1,2,3]`, start: 0, want: 6},
		{name: "nested", input: `[1,[2,3],4]`, start: 0, want: 10},
		{name: "with strings", input: `["a]b","c"]`, start: 0, want: 10},
		{name: "escaped quotes", input: `["a\"b"]`, start: 0, want: 7},
		{name: "object", input: `{"k":"v"}`, start: 0, want: 8},
		{name: "unmatched", input: `[1,2`, start: 0, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := findMatchingBracket(tt.input, tt.start)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestBestVideoURL(t *testing.T) {
	tests := []struct {
		name     string
		versions []videoVersion
		want     string
	}{
		{
			name:     "empty",
			versions: nil,
			want:     "",
		},
		{
			name:     "prefers type 101",
			versions: []videoVersion{{Type: 0, URL: "sd.mp4"}, {Type: 101, URL: "hd.mp4"}},
			want:     "hd.mp4",
		},
		{
			name:     "fallback to first",
			versions: []videoVersion{{Type: 0, URL: "sd.mp4"}, {Type: 1, URL: "med.mp4"}},
			want:     "sd.mp4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bestVideoURL(tt.versions)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
