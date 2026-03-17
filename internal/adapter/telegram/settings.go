package telegram

import (
	"encoding/json"
	"os"
	"sync"
)

// ValidQualities is the ordered list of supported quality levels.
var ValidQualities = []string{"360p", "480p", "720p", "1080p"}

const defaultQuality = "720p"

// UserSettings holds all per-user preferences.
type UserSettings struct {
	Quality string `json:"quality"` // "360p" / "480p" / "720p" / "1080p"
	Caption bool   `json:"caption"` // include video title as caption
}

func defaultSettings() UserSettings {
	return UserSettings{Quality: defaultQuality, Caption: true}
}

// SettingsStore persists per-user settings to a JSON file.
type SettingsStore struct {
	mu   sync.RWMutex
	data map[int64]UserSettings
	file string
}

func NewSettingsStore(file string) *SettingsStore {
	s := &SettingsStore{data: make(map[int64]UserSettings), file: file}
	s.load()
	return s
}

func (s *SettingsStore) Get(userID int64) UserSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.data[userID]; ok {
		return v
	}
	return defaultSettings()
}

func (s *SettingsStore) SetQuality(userID int64, quality string) {
	s.mu.Lock()
	st := s.getOrDefault(userID)
	st.Quality = quality
	s.data[userID] = st
	s.mu.Unlock()
	_ = s.save()
}

func (s *SettingsStore) SetCaption(userID int64, enabled bool) {
	s.mu.Lock()
	st := s.getOrDefault(userID)
	st.Caption = enabled
	s.data[userID] = st
	s.mu.Unlock()
	_ = s.save()
}

func (s *SettingsStore) getOrDefault(userID int64) UserSettings {
	if v, ok := s.data[userID]; ok {
		return v
	}
	return defaultSettings()
}

func (s *SettingsStore) load() {
	data, err := os.ReadFile(s.file)
	if err != nil {
		return
	}
	s.mu.Lock()
	_ = json.Unmarshal(data, &s.data)
	s.mu.Unlock()
}

func (s *SettingsStore) save() error {
	s.mu.RLock()
	data, err := json.Marshal(s.data)
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(s.file, data, 0o600)
}
