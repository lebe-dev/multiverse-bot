package telegram

import (
	"encoding/json"
	"log/slog"
	"os"
	"sync"
)

// AdminChatStore persists admin username → chat ID mappings so the bot
// can send startup notifications (Telegram requires chat IDs, not usernames).
type AdminChatStore struct {
	mu   sync.RWMutex
	data map[string]int64 // username → chat ID
	file string
	log  *slog.Logger
}

func NewAdminChatStore(file string, log *slog.Logger) *AdminChatStore {
	s := &AdminChatStore{data: make(map[string]int64), file: file, log: log}
	s.load()
	return s
}

// Track saves the chat ID for a given admin username.
func (s *AdminChatStore) Track(username string, chatID int64) {
	s.mu.Lock()
	if s.data[username] == chatID {
		s.mu.Unlock()
		return
	}
	s.data[username] = chatID
	s.mu.Unlock()
	if err := s.save(); err != nil {
		s.log.Error("failed to persist admin chat IDs", "error", err)
	}
}

// ChatID returns the stored chat ID for a username, or 0 if unknown.
func (s *AdminChatStore) ChatID(username string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data[username]
}

func (s *AdminChatStore) load() {
	data, err := os.ReadFile(s.file)
	if err != nil {
		return
	}
	s.mu.Lock()
	_ = json.Unmarshal(data, &s.data)
	s.mu.Unlock()
}

func (s *AdminChatStore) save() error {
	s.mu.RLock()
	data, err := json.Marshal(s.data)
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(s.file, data, 0o600)
}
