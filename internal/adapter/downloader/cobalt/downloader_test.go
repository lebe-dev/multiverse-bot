package cobalt

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newTestServer serves the given API response and a fixed file body for any
// other path (used as the tunnel/stream target).
func newTestServer(t *testing.T, apiResp map[string]any, fileBody []byte) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			// Rewrite relative media URLs to point back at this server.
			resp := make(map[string]any, len(apiResp))
			for k, v := range apiResp {
				if s, ok := v.(string); ok && k == "url" {
					v = srv.URL + s
				}
				resp[k] = v
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		_, _ = w.Write(fileBody)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDownloadMediaTunnelVideo(t *testing.T) {
	content := []byte("fake video bytes")
	srv := newTestServer(t, map[string]any{
		"status":   "tunnel",
		"url":      "/tunnel?id=abc",
		"filename": "instagram_DbKTeD9PKTz.mp4",
	}, content)

	d := New(srv.URL, testLogger())
	result, err := d.DownloadMedia(context.Background(), "https://www.instagram.com/reel/DbKTeD9PKTz/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = os.RemoveAll(filepath.Dir(result.Items[0].FilePath)) }()

	if len(result.Items) != 1 {
		t.Fatalf("got %d items, want 1", len(result.Items))
	}
	item := result.Items[0]
	if item.Type != domain.MediaVideo {
		t.Errorf("type = %v, want MediaVideo", item.Type)
	}
	if filepath.Ext(item.FilePath) != ".mp4" {
		t.Errorf("file extension = %q, want .mp4", filepath.Ext(item.FilePath))
	}
	if item.Size != int64(len(content)) {
		t.Errorf("size = %d, want %d", item.Size, len(content))
	}
	data, err := os.ReadFile(item.FilePath)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("content = %q, want %q", data, content)
	}
}

func TestDownloadMediaTunnelPhoto(t *testing.T) {
	srv := newTestServer(t, map[string]any{
		"status":   "tunnel",
		"url":      "/tunnel?id=abc",
		"filename": "instagram_DbKTeD9PKTz.jpg",
	}, []byte("fake jpeg bytes"))

	d := New(srv.URL, testLogger())
	result, err := d.DownloadMedia(context.Background(), "https://www.instagram.com/p/DbKTeD9PKTz/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = os.RemoveAll(filepath.Dir(result.Items[0].FilePath)) }()

	item := result.Items[0]
	if item.Type != domain.MediaPhoto {
		t.Errorf("type = %v, want MediaPhoto", item.Type)
	}
	if filepath.Ext(item.FilePath) != ".jpg" {
		t.Errorf("file extension = %q, want .jpg", filepath.Ext(item.FilePath))
	}
}

func TestDownloadMediaStreamStillWorks(t *testing.T) {
	srv := newTestServer(t, map[string]any{
		"status": "stream",
		"url":    "/stream?id=abc",
	}, []byte("fake video bytes"))

	d := New(srv.URL, testLogger())
	result, err := d.DownloadMedia(context.Background(), "https://www.instagram.com/reel/x/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = os.RemoveAll(filepath.Dir(result.Items[0].FilePath)) }()

	if result.Items[0].Type != domain.MediaVideo {
		t.Errorf("type = %v, want MediaVideo", result.Items[0].Type)
	}
}

func TestDownloadMediaLocalProcessing(t *testing.T) {
	srv := newTestServer(t, map[string]any{
		"status": "local-processing",
		"type":   "remux",
		"tunnel": []any{"/tunnel?id=1", "/tunnel?id=2"},
	}, []byte("fake video bytes"))

	d := New(srv.URL, testLogger())
	_, err := d.DownloadMedia(context.Background(), "https://www.instagram.com/reel/x/")
	if !errors.Is(err, domain.ErrDownloadFailed) {
		t.Fatalf("error = %v, want ErrDownloadFailed", err)
	}
}

func TestDownloadTunnel(t *testing.T) {
	content := []byte("fake video bytes")
	srv := newTestServer(t, map[string]any{
		"status":   "tunnel",
		"url":      "/tunnel?id=abc",
		"filename": "instagram_x.mp4",
	}, content)

	d := New(srv.URL, testLogger())
	video, err := d.Download(context.Background(), "https://www.instagram.com/reel/x/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = os.RemoveAll(filepath.Dir(video.FilePath)) }()

	if video.Size != int64(len(content)) {
		t.Errorf("size = %d, want %d", video.Size, len(content))
	}
}

func TestDownloadMediaAPIError(t *testing.T) {
	srv := newTestServer(t, map[string]any{
		"status": "error",
		"error":  map[string]any{"code": "error.api.link.invalid"},
	}, nil)

	d := New(srv.URL, testLogger())
	_, err := d.DownloadMedia(context.Background(), "https://www.instagram.com/reel/x/")
	if !errors.Is(err, domain.ErrDownloadFailed) {
		t.Fatalf("error = %v, want ErrDownloadFailed", err)
	}
}

func TestMediaKindFromFilename(t *testing.T) {
	tests := []struct {
		filename string
		wantType domain.MediaType
		wantExt  string
	}{
		{filename: "instagram_x.mp4", wantType: domain.MediaVideo, wantExt: ".mp4"},
		{filename: "instagram_x.webm", wantType: domain.MediaVideo, wantExt: ".webm"},
		{filename: "instagram_x.jpg", wantType: domain.MediaPhoto, wantExt: ".jpg"},
		{filename: "instagram_x.JPEG", wantType: domain.MediaPhoto, wantExt: ".jpeg"},
		{filename: "instagram_x.webp", wantType: domain.MediaPhoto, wantExt: ".webp"},
		{filename: "", wantType: domain.MediaVideo, wantExt: ".mp4"},
		{filename: "noext", wantType: domain.MediaVideo, wantExt: ".mp4"},
	}

	for _, tt := range tests {
		gotType, gotExt := mediaKindFromFilename(tt.filename)
		if gotType != tt.wantType || gotExt != tt.wantExt {
			t.Errorf("mediaKindFromFilename(%q) = (%v, %q), want (%v, %q)",
				tt.filename, gotType, gotExt, tt.wantType, tt.wantExt)
		}
	}
}
