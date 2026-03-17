package gdrive

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// DriveFileScope grants access only to files the app itself created.
// Users' existing files are completely invisible to the bot.
const DriveFileScope = "https://www.googleapis.com/auth/drive.file"

// OAuthManager handles per-user Google OAuth2 via Device Authorization Grant.
// No HTTP server, no redirect URL, no HTTPS required.
type OAuthManager struct {
	cfg   *oauth2.Config
	store *TokenStore
}

func NewOAuthManager(clientID, clientSecret string, store *TokenStore) *OAuthManager {
	// google.Endpoint doesn't set DeviceAuthURL — set it explicitly.
	endpoint := google.Endpoint
	endpoint.DeviceAuthURL = "https://oauth2.googleapis.com/device/code"

	return &OAuthManager{
		cfg: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Scopes:       []string{DriveFileScope},
			Endpoint:     endpoint,
		},
		store: store,
	}
}

// StartDeviceAuth requests a device code from Google.
// Returns the response containing UserCode and VerificationURI to show the user.
func (m *OAuthManager) StartDeviceAuth(ctx context.Context) (*oauth2.DeviceAuthResponse, error) {
	resp, err := m.cfg.DeviceAuth(ctx)
	if err != nil {
		return nil, fmt.Errorf("device auth: %w", err)
	}
	return resp, nil
}

// PollDeviceAuth blocks until the user approves in the browser, the code
// expires, or ctx is cancelled. Saves the token on success.
// Designed to be called in a goroutine.
func (m *OAuthManager) PollDeviceAuth(ctx context.Context, userID int64, resp *oauth2.DeviceAuthResponse) error {
	token, err := m.cfg.DeviceAccessToken(ctx, resp)
	if err != nil {
		return err
	}
	m.store.Save(userID, token)
	return nil
}

// IsConnected returns true if the user has a stored OAuth token.
func (m *OAuthManager) IsConnected(userID int64) bool {
	return m.store.Get(userID) != nil
}

// DriveService creates a Drive API client using the user's stored token.
// Automatically refreshes and persists the refreshed token.
func (m *OAuthManager) DriveService(ctx context.Context, userID int64) (*drive.Service, error) {
	token := m.store.Get(userID)
	if token == nil {
		return nil, fmt.Errorf("not connected — use /auth")
	}
	src := &persistingTokenSource{
		userID: userID,
		src:    m.cfg.TokenSource(ctx, token),
		store:  m.store,
		last:   token,
	}
	svc, err := drive.NewService(ctx, option.WithTokenSource(src))
	if err != nil {
		return nil, fmt.Errorf("creating drive service: %w", err)
	}
	return svc, nil
}

// Disconnect removes the user's stored token.
func (m *OAuthManager) Disconnect(userID int64) {
	m.store.Delete(userID)
}

// persistingTokenSource wraps oauth2.TokenSource and saves refreshed tokens to disk.
type persistingTokenSource struct {
	userID int64
	src    oauth2.TokenSource
	store  *TokenStore
	mu     sync.Mutex
	last   *oauth2.Token
}

func (s *persistingTokenSource) Token() (*oauth2.Token, error) {
	t, err := s.src.Token()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.last == nil || t.AccessToken != s.last.AccessToken {
		s.store.Save(s.userID, t)
		s.last = t
	}
	s.mu.Unlock()
	return t, nil
}
