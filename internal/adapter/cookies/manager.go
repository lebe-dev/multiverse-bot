package cookies

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

// Manager stores cookies in the database and materializes them to temp files
// so that yt-dlp (which requires --cookies <path>) can use them.
type Manager struct {
	store domain.CookieStore
	mu    sync.RWMutex
	paths map[string]string // platform → temp file path
	dir   string            // temp directory for materialized files
}

func NewManager(store domain.CookieStore) *Manager {
	return &Manager{
		store: store,
		paths: make(map[string]string),
	}
}

// Materialize loads all cookies from the DB and writes them to temp files.
// Call once at startup.
func (m *Manager) Materialize(ctx context.Context) error {
	platforms, err := m.store.ListCookiePlatforms(ctx)
	if err != nil {
		return fmt.Errorf("listing cookie platforms: %w", err)
	}

	for _, p := range platforms {
		data, err := m.store.GetCookies(ctx, p)
		if err != nil {
			return fmt.Errorf("loading %s cookies: %w", p, err)
		}
		if len(data) == 0 {
			continue
		}
		if err := m.writeFile(p, data); err != nil {
			return fmt.Errorf("materializing %s cookies: %w", p, err)
		}
	}
	return nil
}

// SaveCookies persists cookies to DB and materializes a temp file.
func (m *Manager) SaveCookies(ctx context.Context, platform string, data []byte) error {
	if err := m.store.SaveCookies(ctx, platform, data); err != nil {
		return err
	}
	return m.writeFile(platform, data)
}

// DeleteCookies removes cookies from DB and cleans up the temp file.
func (m *Manager) DeleteCookies(ctx context.Context, platform string) error {
	if err := m.store.DeleteCookies(ctx, platform); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if p, ok := m.paths[platform]; ok {
		_ = os.Remove(p)
		delete(m.paths, platform)
	}
	return nil
}

// CookieFilePath returns the path to the materialized cookie file, or "" if none.
func (m *Manager) CookieFilePath(platform string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.paths[platform]
}

// HasCookies reports whether cookies exist for the given platform.
func (m *Manager) HasCookies(platform string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.paths[platform] != ""
}

// Cleanup removes all temp files.
func (m *Manager) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.dir != "" {
		_ = os.RemoveAll(m.dir)
	}
	m.paths = make(map[string]string)
	m.dir = ""
}

func (m *Manager) writeFile(platform string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.dir == "" {
		d, err := os.MkdirTemp("", "multiverse-cookies-*")
		if err != nil {
			return fmt.Errorf("creating temp dir: %w", err)
		}
		m.dir = d
	}

	path := filepath.Join(m.dir, platform+"-cookies.txt")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing cookie file: %w", err)
	}
	m.paths[platform] = path
	return nil
}
