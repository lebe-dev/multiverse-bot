package plugin

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfig_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugins.yml")
	content := `plugins:
  - name: tiktok
    url: http://plugin-tiktok:8080
    enabled: true
    timeout: 45s
  - name: summarize
    url: http://plugin-summarize:8080
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	configs, err := loadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(configs) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(configs))
	}

	if configs[0].Name != "tiktok" {
		t.Errorf("expected name 'tiktok', got %q", configs[0].Name)
	}
	if configs[0].Timeout != 45*time.Second {
		t.Errorf("expected timeout 45s, got %v", configs[0].Timeout)
	}
	if configs[1].Timeout != 30*time.Second {
		t.Errorf("expected default timeout 30s, got %v", configs[1].Timeout)
	}
}

func TestLoadConfig_DisabledPlugin(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugins.yml")
	enabled := false
	_ = enabled
	content := `plugins:
  - name: tiktok
    url: http://plugin-tiktok:8080
    enabled: false
  - name: summarize
    url: http://plugin-summarize:8080
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	configs, err := loadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(configs) != 1 {
		t.Fatalf("expected 1 enabled plugin, got %d", len(configs))
	}
	if configs[0].Name != "summarize" {
		t.Errorf("expected 'summarize', got %q", configs[0].Name)
	}
}

func TestLoadConfig_InvalidName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugins.yml")
	content := `plugins:
  - name: "INVALID NAME"
    url: http://localhost:8080
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := loadConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid name")
	}
}

func TestLoadConfig_DuplicateName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugins.yml")
	content := `plugins:
  - name: tiktok
    url: http://a:8080
  - name: tiktok
    url: http://b:8080
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := loadConfig(path)
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
}

func TestLoadConfig_MissingURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugins.yml")
	content := `plugins:
  - name: tiktok
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := loadConfig(path)
	if err == nil {
		t.Fatal("expected error for missing URL")
	}
}
