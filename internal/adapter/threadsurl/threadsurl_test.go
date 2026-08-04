package threadsurl

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestCanonical(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr error
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
			name:  "non-xmt query params stripped",
			input: "https://www.threads.com/@user/post/ABC123?igsh=foo",
			want:  "https://www.threads.com/@user/post/ABC123",
		},
		{
			name:  "xmt query param preserved",
			input: "https://www.threads.com/@user/post/ABC123?xmt=AQF0x9&igsh=foo",
			want:  "https://www.threads.com/@user/post/ABC123?xmt=AQF0x9",
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
			name:  "root URL with injected_media_ids",
			input: "https://www.threads.com/?xmt=AQG0BLQ&injected_media_ids=%5B%223954841119435301839%22%5D",
			want:  "https://www.threads.com/t/DbiawtjDKPP?xmt=AQG0BLQ",
		},
		{
			name:  "root URL with injected_media_ids and no xmt",
			input: "https://www.threads.com/?injected_media_ids=%5B%223954929350411598284%22%5D",
			want:  "https://www.threads.com/t/Dbiu0pDDLnM",
		},
		{
			name:  "post URL with injected_media_ids still uses the path",
			input: "https://www.threads.com/@user/post/ABC123?injected_media_ids=%5B%223954841119435301839%22%5D",
			want:  "https://www.threads.com/@user/post/ABC123",
		},
		{
			name:    "share URL needs redirect",
			input:   "https://www.threads.com/share/_1-9exY8p/",
			wantErr: ErrNeedsRedirect,
		},
		{
			name:    "root URL without media ids",
			input:   "https://www.threads.com/",
			wantErr: ErrUnsupportedURL,
		},
		{
			name:    "not a threads URL",
			input:   "https://instagram.com/p/ABC123",
			wantErr: ErrUnsupportedURL,
		},
		{
			name:    "bad path format",
			input:   "https://threads.com/something/else",
			wantErr: ErrUnsupportedURL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Canonical(tt.input)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error = %v, want %v (got %q)", err, tt.wantErr, got)
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

func TestShortcodeFromMediaID(t *testing.T) {
	tests := []struct {
		id   string
		want string
		ok   bool
	}{
		{id: "3954841119435301839", want: "DbiawtjDKPP", ok: true},
		{id: "3954929350411598284", want: "Dbiu0pDDLnM", ok: true},
		{id: "1", want: "B", ok: true},
		{id: "0", ok: false},
		{id: "", ok: false},
		{id: "not-a-number", ok: false},
		{id: "-5", ok: false},
	}

	for _, tt := range tests {
		got, ok := shortcodeFromMediaID(tt.id)
		if ok != tt.ok {
			t.Errorf("shortcodeFromMediaID(%q) ok = %v, want %v", tt.id, ok, tt.ok)
			continue
		}
		if ok && got != tt.want {
			t.Errorf("shortcodeFromMediaID(%q) = %q, want %q", tt.id, got, tt.want)
		}
	}
}

func TestMediaIDFromQuery(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "json array", raw: `["3954841119435301839"]`, want: "3954841119435301839"},
		{name: "first of many", raw: `["111","222"]`, want: "111"},
		{name: "bare id", raw: "3954841119435301839", want: "3954841119435301839"},
		{name: "empty array", raw: "[]", want: ""},
		{name: "garbage", raw: "abc", want: ""},
	}

	for _, tt := range tests {
		if got := mediaIDFromParam(tt.raw); got != tt.want {
			t.Errorf("%s: mediaIDFromParam(%q) = %q, want %q", tt.name, tt.raw, got, tt.want)
		}
	}
}

func TestResolve(t *testing.T) {
	const shareURL = "https://www.threads.com/share/_1-9exY8p/"

	tests := []struct {
		name     string
		finalURL string
		want     string
		wantErr  bool
	}{
		{
			name:     "redirect to post URL",
			finalURL: "https://www.threads.com/@user/post/ABC123?xmt=tok&slof=1",
			want:     "https://www.threads.com/@user/post/ABC123?xmt=tok",
		},
		{
			name:     "redirect to root with injected_media_ids",
			finalURL: "https://www.threads.com/?injected_media_ids=%5B%223954841119435301839%22%5D",
			want:     "https://www.threads.com/t/DbiawtjDKPP",
		},
		{
			name:     "redirect leads nowhere useful",
			finalURL: "https://www.threads.com/",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requested string
			doer := doerFunc(func(r *http.Request) (*http.Response, error) {
				requested = r.URL.String()
				return redirectedResponse(t, r, tt.finalURL), nil
			})

			got, err := Resolve(context.Background(), doer, "", shareURL)
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
			if requested != shareURL {
				t.Errorf("requested %q, want %q", requested, shareURL)
			}
		})
	}
}

func TestResolveSkipsNetworkForCanonicalURL(t *testing.T) {
	doer := doerFunc(func(*http.Request) (*http.Response, error) {
		t.Error("Resolve should not perform a request for an already canonical URL")
		return nil, errors.New("unexpected request")
	})

	got, err := Resolve(context.Background(), doer, "", "https://www.threads.com/@user/post/ABC123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "https://www.threads.com/@user/post/ABC123"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveRejectsNonThreadsURL(t *testing.T) {
	doer := doerFunc(func(*http.Request) (*http.Response, error) {
		t.Error("Resolve should not perform a request for a non-Threads URL")
		return nil, errors.New("unexpected request")
	})

	if _, err := Resolve(context.Background(), doer, "", "https://instagram.com/p/ABC123"); !errors.Is(err, ErrUnsupportedURL) {
		t.Fatalf("error = %v, want %v", err, ErrUnsupportedURL)
	}
}

// redirectedResponse mimics what net/http returns after following redirects:
// resp.Request.URL points at the final location.
func redirectedResponse(t *testing.T, req *http.Request, finalURL string) *http.Response {
	t.Helper()
	final, err := url.Parse(finalURL)
	if err != nil {
		t.Fatalf("bad final URL %q: %v", finalURL, err)
	}
	finalReq := req.Clone(req.Context())
	finalReq.URL = final
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("")),
		Request:    finalReq,
	}
}

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }
