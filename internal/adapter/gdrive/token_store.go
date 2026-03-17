package gdrive

import (
	"encoding/json"
	"os"
	"sync"

	"golang.org/x/oauth2"
)

// TokenStore persists per-user OAuth2 tokens to a JSON file.
type TokenStore struct {
	mu   sync.RWMutex
	data map[int64]*oauth2.Token
	file string
}

func NewTokenStore(file string) *TokenStore {
	s := &TokenStore{data: make(map[int64]*oauth2.Token), file: file}
	s.load()
	return s
}

func (s *TokenStore) Get(userID int64) *oauth2.Token {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data[userID]
}

func (s *TokenStore) Save(userID int64, token *oauth2.Token) {
	s.mu.Lock()
	s.data[userID] = token
	s.mu.Unlock()
	_ = s.persist()
}

func (s *TokenStore) Delete(userID int64) {
	s.mu.Lock()
	delete(s.data, userID)
	s.mu.Unlock()
	_ = s.persist()
}

func (s *TokenStore) load() {
	data, err := os.ReadFile(s.file)
	if err != nil {
		return
	}
	s.mu.Lock()
	_ = json.Unmarshal(data, &s.data)
	s.mu.Unlock()
}

func (s *TokenStore) persist() error {
	s.mu.RLock()
	data, err := json.MarshalIndent(s.data, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(s.file, data, 0o600)
}
