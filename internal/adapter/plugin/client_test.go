package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

func TestHTTPClient_Health(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	c := newHTTPClient("test", srv.URL, 5*time.Second)
	if err := c.Health(context.Background()); err != nil {
		t.Fatalf("health check failed: %v", err)
	}
}

func TestHTTPClient_HealthUnhealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := newHTTPClient("test", srv.URL, 5*time.Second)
	if err := c.Health(context.Background()); err == nil {
		t.Fatal("expected error for unhealthy plugin")
	}
}

func TestHTTPClient_Manifest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(manifestResponse{
			Name:        "tiktok",
			Description: "Download TikTok videos",
			Commands: []manifestCommand{
				{Command: "/tiktok", Description: "Download a TikTok video"},
			},
			URLPatterns: []string{`(?i)tiktok\.com`},
		})
	}))
	defer srv.Close()

	c := newHTTPClient("tiktok", srv.URL, 5*time.Second)
	m, err := c.Manifest(context.Background())
	if err != nil {
		t.Fatalf("manifest failed: %v", err)
	}

	if m.Name != "tiktok" {
		t.Errorf("expected name 'tiktok', got %q", m.Name)
	}
	if len(m.Commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(m.Commands))
	}
	if m.Commands[0].Command != "/tiktok" {
		t.Errorf("expected command '/tiktok', got %q", m.Commands[0].Command)
	}
}

func TestHTTPClient_Execute(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/execute" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		_ = json.NewEncoder(w).Encode(executeResponse{
			Actions: []actionJSON{
				{Type: "text", Text: "Hello from plugin"},
			},
		})
	}))
	defer srv.Close()

	c := newHTTPClient("tiktok", srv.URL, 5*time.Second)
	resp, err := c.Execute(context.Background(), &domain.PluginRequest{
		Trigger: domain.PluginTrigger{
			Type:    "command",
			Command: "/tiktok",
			RawText: "/tiktok https://example.com",
		},
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if len(resp.Actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(resp.Actions))
	}
	if resp.Actions[0].Text != "Hello from plugin" {
		t.Errorf("unexpected text: %q", resp.Actions[0].Text)
	}
}

func TestHTTPClient_ExecuteError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Video not found"})
	}))
	defer srv.Close()

	c := newHTTPClient("tiktok", srv.URL, 5*time.Second)
	resp, err := c.Execute(context.Background(), &domain.PluginRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Error != "Video not found" {
		t.Errorf("expected error 'Video not found', got %q", resp.Error)
	}
}

func TestHTTPClient_Callback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/callback" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(executeResponse{
			Toast: "Loading...",
			Actions: []actionJSON{
				{Type: "text", Text: "More options"},
			},
		})
	}))
	defer srv.Close()

	c := newHTTPClient("tiktok", srv.URL, 5*time.Second)
	resp, err := c.Callback(context.Background(), &domain.PluginCallbackRequest{
		CallbackID: "more",
	})
	if err != nil {
		t.Fatalf("callback failed: %v", err)
	}

	if resp.Toast != "Loading..." {
		t.Errorf("expected toast 'Loading...', got %q", resp.Toast)
	}
	if len(resp.Actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(resp.Actions))
	}
}
