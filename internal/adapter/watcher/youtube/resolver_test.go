package youtube

import (
	"regexp"
	"testing"
)

func TestUCIDRegex(t *testing.T) {
	valid := []string{
		"UCxxxxxxxxxxxxxxxxxxxxxx",
		"UC1234567890abcdefghijkl",
		"UC-_abcdefghijklmnopqrst",
	}
	for _, id := range valid {
		if !ucIDRe.MatchString(id) {
			t.Errorf("expected %q to match UC ID regex", id)
		}
	}

	invalid := []string{
		"UC123",       // too short
		"AB123456789", // wrong prefix
		"",
		"https://youtube.com/channel/UCxxxxxxxxxxxxxxxxxxxxxx",
	}
	for _, id := range invalid {
		if ucIDRe.MatchString(id) {
			t.Errorf("expected %q to NOT match UC ID regex", id)
		}
	}
}

func TestChannelURLRegex(t *testing.T) {
	tests := []struct {
		input   string
		wantID  string
		wantHit bool
	}{
		{"https://www.youtube.com/channel/UCxxxxxxxxxxxxxxxxxxxxxx", "UCxxxxxxxxxxxxxxxxxxxxxx", true},
		{"youtube.com/channel/UCxxxxxxxxxxxxxxxxxxxxxx/videos", "UCxxxxxxxxxxxxxxxxxxxxxx", true},
		{"https://youtube.com/watch?v=abc", "", false},
	}
	for _, tt := range tests {
		m := channelURLRe.FindStringSubmatch(tt.input)
		if tt.wantHit {
			if len(m) < 2 || m[1] != tt.wantID {
				t.Errorf("channelURLRe(%q): got %v, want ID %q", tt.input, m, tt.wantID)
			}
		} else {
			if len(m) > 0 {
				t.Errorf("channelURLRe(%q): expected no match, got %v", tt.input, m)
			}
		}
	}
}

func TestHandleURLRegex(t *testing.T) {
	re := regexp.MustCompile(`youtube\.com/@([a-zA-Z0-9_.\-]+)`)

	tests := []struct {
		input      string
		wantHandle string
		wantHit    bool
	}{
		{"https://www.youtube.com/@MrBeast", "MrBeast", true},
		{"youtube.com/@some.channel", "some.channel", true},
		{"https://youtube.com/channel/UCxxx", "", false},
	}
	for _, tt := range tests {
		m := re.FindStringSubmatch(tt.input)
		if tt.wantHit {
			if len(m) < 2 || m[1] != tt.wantHandle {
				t.Errorf("handleURLRe(%q): got %v, want handle %q", tt.input, m, tt.wantHandle)
			}
		} else {
			if len(m) > 0 {
				t.Errorf("handleURLRe(%q): expected no match, got %v", tt.input, m)
			}
		}
	}
}
