package telegram

import (
	"testing"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

func TestExtractURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain URL", "https://youtube.com/watch?v=abc", "https://youtube.com/watch?v=abc"},
		{"URL with text before", "check this https://example.com/video", "https://example.com/video"},
		{"URL with text after", "https://example.com/video is cool", "https://example.com/video"},
		{"http URL", "http://example.com/path", "http://example.com/path"},
		{"no URL", "just some text", ""},
		{"empty", "", ""},
		{"whitespace only", "   ", ""},
		{"URL with leading whitespace", "  https://example.com  ", "https://example.com"},
		{"multiple URLs picks first", "https://first.com https://second.com", "https://first.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractURL(tt.input)
			if got != tt.want {
				t.Errorf("extractURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name string
		secs float64
		want string
	}{
		{"zero", 0, "0:00"},
		{"seconds only", 45, "0:45"},
		{"one minute", 60, "1:00"},
		{"minutes and seconds", 125, "2:05"},
		{"one hour", 3600, "1:00:00"},
		{"hours minutes seconds", 3661, "1:01:01"},
		{"fractional rounds down", 65.9, "1:05"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.secs)
			if got != tt.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.secs, got, tt.want)
			}
		})
	}
}

func TestEscapeHTML(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no special chars", "hello world", "hello world"},
		{"ampersand", "a&b", "a&amp;b"},
		{"less than", "a<b", "a&lt;b"},
		{"greater than", "a>b", "a&gt;b"},
		{"all special", "<script>alert('xss')&</script>", "&lt;script&gt;alert('xss')&amp;&lt;/script&gt;"},
		{"empty", "", ""},
		{"double ampersand", "&&", "&amp;&amp;"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeHTML(tt.input)
			if got != tt.want {
				t.Errorf("escapeHTML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatDetailsMsg(t *testing.T) {
	t.Run("with entries", func(t *testing.T) {
		s := &domain.FormatSummary{
			Title:    "Test Video",
			Duration: 125,
			Entries: []domain.FormatEntry{
				{Height: 360, Size: 10 * 1024 * 1024},
				{Height: 720, Size: 25 * 1024 * 1024},
			},
		}
		got := formatDetailsMsg(s)
		if got == "" {
			t.Fatal("expected non-empty result")
		}
		// Check key elements are present
		for _, want := range []string{"Test Video", "2:05", "360p", "720p", "10 МБ", "25 МБ"} {
			if !contains(got, want) {
				t.Errorf("formatDetailsMsg missing %q in:\n%s", want, got)
			}
		}
	})

	t.Run("empty entries", func(t *testing.T) {
		s := &domain.FormatSummary{
			Title:    "No Formats",
			Duration: 60,
		}
		got := formatDetailsMsg(s)
		if !contains(got, "Форматы не найдены") {
			t.Errorf("expected 'Форматы не найдены' in:\n%s", got)
		}
	})

	t.Run("unknown size shows question mark", func(t *testing.T) {
		s := &domain.FormatSummary{
			Duration: 10,
			Entries:  []domain.FormatEntry{{Height: 480, Size: 0}},
		}
		got := formatDetailsMsg(s)
		if !contains(got, "?") {
			t.Errorf("expected '?' for unknown size in:\n%s", got)
		}
	})

	t.Run("html escapes title", func(t *testing.T) {
		s := &domain.FormatSummary{
			Title:    "<script>xss</script>",
			Duration: 10,
			Entries:  []domain.FormatEntry{{Height: 720, Size: 1024 * 1024}},
		}
		got := formatDetailsMsg(s)
		if contains(got, "<script>") {
			t.Errorf("title should be HTML-escaped in:\n%s", got)
		}
		if !contains(got, "&lt;script&gt;") {
			t.Errorf("expected escaped title in:\n%s", got)
		}
	})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
