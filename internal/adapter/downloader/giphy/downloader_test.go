package giphy

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

func TestExtractID(t *testing.T) {
	const id = "GK7dGndK4kyOozR9Xk"
	tests := []struct {
		url  string
		want string
	}{
		{"https://giphy.com/gifs/RespectiveCollective-dance-ai-kitty-GK7dGndK4kyOozR9Xk", id},
		{"https://giphy.com/gifs/GK7dGndK4kyOozR9Xk", id},
		{"https://www.giphy.com/gifs/cat-GK7dGndK4kyOozR9Xk/fullscreen", id},
		{"https://giphy.com/stickers/cat-GK7dGndK4kyOozR9Xk?utm_source=x", id},
		{"https://giphy.com/embed/GK7dGndK4kyOozR9Xk", id},
		{"https://media.giphy.com/media/GK7dGndK4kyOozR9Xk/giphy.gif", id},
		{"https://media3.giphy.com/media/v1.Y2lkPTc5MGI3NjEx/GK7dGndK4kyOozR9Xk/giphy.webp", id},
		{"https://i.giphy.com/GK7dGndK4kyOozR9Xk.webp", id},
		{"https://i.giphy.com/media/GK7dGndK4kyOozR9Xk/giphy.mp4", id},
		{"https://giphy.com/", ""},
		{"https://giphy.com/explore/cats", ""},
		{"https://example.com/gifs/cat-GK7dGndK4kyOozR9Xk", ""},
		{"https://notgiphy.com/gifs/cat-GK7dGndK4kyOozR9Xk", ""},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			u, err := url.Parse(tt.url)
			if err != nil {
				t.Fatal(err)
			}
			if got := ExtractID(u); got != tt.want {
				t.Errorf("ExtractID(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func newTestDownloader(srv *httptest.Server) *Downloader {
	d := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	d.mediaBaseURL = srv.URL + "/media"
	return d
}

func TestDownloadMedia_PrefersMP4(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/media/GK7dGndK4kyOozR9Xk/giphy.mp4" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("mp4-data"))
	}))
	defer srv.Close()

	res, err := newTestDownloader(srv).DownloadMedia(context.Background(),
		"https://giphy.com/gifs/RespectiveCollective-dance-ai-kitty-GK7dGndK4kyOozR9Xk")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(filepath.Dir(res.Items[0].FilePath)) }()

	if len(res.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(res.Items))
	}
	item := res.Items[0]
	if item.Type != domain.MediaAnimation {
		t.Errorf("type = %v, want MediaAnimation", item.Type)
	}
	if filepath.Ext(item.FilePath) != ".mp4" {
		t.Errorf("file = %q, want .mp4", item.FilePath)
	}
	if item.Size != int64(len("mp4-data")) {
		t.Errorf("size = %d", item.Size)
	}
}

func TestDownloadMedia_FallsBackToGIF(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/media/abc123/giphy.gif" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("GIF89a"))
	}))
	defer srv.Close()

	res, err := newTestDownloader(srv).DownloadMedia(context.Background(), "https://giphy.com/stickers/x-abc123")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(filepath.Dir(res.Items[0].FilePath)) }()

	if filepath.Ext(res.Items[0].FilePath) != ".gif" {
		t.Errorf("file = %q, want .gif", res.Items[0].FilePath)
	}
}

func TestDownloadMedia_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	_, err := newTestDownloader(srv).DownloadMedia(context.Background(), "https://giphy.com/gifs/abc123")
	if !errors.Is(err, domain.ErrDownloadFailed) {
		t.Fatalf("err = %v, want ErrDownloadFailed", err)
	}
}

func TestDownloadMedia_InvalidURL(t *testing.T) {
	d := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err := d.DownloadMedia(context.Background(), "https://giphy.com/")
	if !errors.Is(err, domain.ErrDownloadFailed) || !errors.Is(err, ErrInvalidURL) {
		t.Fatalf("err = %v, want ErrDownloadFailed wrapping ErrInvalidURL", err)
	}
}
