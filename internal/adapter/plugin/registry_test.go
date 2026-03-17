package plugin

import (
	"context"
	"regexp"
	"testing"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

// mockPluginClient implements domain.PluginClient for testing.
type mockPluginClient struct{}

func (m *mockPluginClient) Health(context.Context) error                                       { return nil }
func (m *mockPluginClient) Manifest(context.Context) (*domain.PluginManifest, error)           { return nil, nil }
func (m *mockPluginClient) Execute(context.Context, *domain.PluginRequest) (*domain.PluginResponse, error) {
	return &domain.PluginResponse{}, nil
}
func (m *mockPluginClient) Callback(context.Context, *domain.PluginCallbackRequest) (*domain.PluginResponse, error) {
	return &domain.PluginResponse{}, nil
}

func newTestRegistry() *registry {
	r := &registry{
		commandMap: make(map[string]int),
		nameMap:    make(map[string]int),
	}

	r.plugins = append(r.plugins, pluginEntry{
		client: &mockPluginClient{},
		manifest: domain.PluginManifest{
			Name:        "tiktok",
			Description: "Download TikTok videos",
			Commands: []domain.PluginCommand{
				{Command: "/tiktok", Description: "Download a TikTok video"},
			},
			URLPatterns: []string{`(?i)tiktok\.com`},
		},
		patterns: []*regexp.Regexp{regexp.MustCompile(`(?i)tiktok\.com`)},
	})

	r.nameMap["tiktok"] = 0
	r.commandMap["/tiktok"] = 0

	return r
}

func TestRegistryFindByCommand(t *testing.T) {
	r := newTestRegistry()

	client, name := r.FindByCommand("/tiktok")
	if client == nil {
		t.Fatal("expected client for /tiktok")
	}
	if name != "tiktok" {
		t.Errorf("expected name 'tiktok', got %q", name)
	}

	client, _ = r.FindByCommand("/unknown")
	if client != nil {
		t.Fatal("expected nil for unknown command")
	}
}

func TestRegistryFindByURL(t *testing.T) {
	r := newTestRegistry()

	client, name, pattern := r.FindByURL("https://www.tiktok.com/@user/video/123")
	if client == nil {
		t.Fatal("expected client for tiktok URL")
	}
	if name != "tiktok" {
		t.Errorf("expected name 'tiktok', got %q", name)
	}
	if pattern != `(?i)tiktok\.com` {
		t.Errorf("unexpected pattern: %q", pattern)
	}

	client, _, _ = r.FindByURL("https://youtube.com/watch?v=123")
	if client != nil {
		t.Fatal("expected nil for non-matching URL")
	}
}

func TestRegistryFindByName(t *testing.T) {
	r := newTestRegistry()

	client := r.FindByName("tiktok")
	if client == nil {
		t.Fatal("expected client for name 'tiktok'")
	}

	client = r.FindByName("unknown")
	if client != nil {
		t.Fatal("expected nil for unknown name")
	}
}

func TestRegistryAllManifests(t *testing.T) {
	r := newTestRegistry()

	manifests := r.AllManifests()
	if len(manifests) != 1 {
		t.Fatalf("expected 1 manifest, got %d", len(manifests))
	}
	if manifests[0].Name != "tiktok" {
		t.Errorf("expected 'tiktok', got %q", manifests[0].Name)
	}
}
