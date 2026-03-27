package instagram

import "testing"

func TestExtractUsername(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://www.instagram.com/natgeo/", "natgeo"},
		{"https://instagram.com/natgeo", "natgeo"},
		{"http://instagram.com/nasa/", "nasa"},
		{"instagram.com/user.name", "user.name"},
		{"instagram.com/user_123", "user_123"},
		{"@natgeo", "natgeo"},
		{"natgeo", "natgeo"},
		// Should NOT match post/reel URLs
		{"https://instagram.com/p/ABC123", ""},
		{"https://instagram.com/reel/ABC123", ""},
		{"https://instagram.com/reels/ABC123", ""},
		// Empty / invalid
		{"", ""},
		{"https://example.com/foo", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := extractUsername(tt.input)
			if got != tt.want {
				t.Errorf("extractUsername(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
