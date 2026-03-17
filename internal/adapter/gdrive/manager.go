package gdrive

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

const driveFileScope = "https://www.googleapis.com/auth/drive.file"

// Manager implements domain.DriveManager using Google OAuth2 Device Flow.
type Manager struct {
	cfg   *oauth2.Config
	store *TokenStore
	log   *slog.Logger
}

// NewManager creates a DriveManager backed by per-user OAuth2 Device Flow.
func NewManager(clientID, clientSecret string, store *TokenStore, log *slog.Logger) *Manager {
	endpoint := google.Endpoint
	endpoint.DeviceAuthURL = "https://oauth2.googleapis.com/device/code"

	return &Manager{
		cfg: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Scopes:       []string{driveFileScope},
			Endpoint:     endpoint,
		},
		store: store,
		log:   log,
	}
}

// StartAuth initiates the device auth flow. Returns info to display to the user
// and a poll function that blocks until the user completes authorization.
func (m *Manager) StartAuth(ctx context.Context) (domain.DeviceAuthInfo, func(ctx context.Context, userID int64) error, error) {
	resp, err := m.cfg.DeviceAuth(ctx)
	if err != nil {
		return domain.DeviceAuthInfo{}, nil, fmt.Errorf("device auth: %w", err)
	}

	info := domain.DeviceAuthInfo{
		UserCode:        resp.UserCode,
		VerificationURI: resp.VerificationURI,
		Expiry:          resp.Expiry,
	}

	pollFn := func(ctx context.Context, userID int64) error {
		token, err := m.cfg.DeviceAccessToken(ctx, resp)
		if err != nil {
			return err
		}
		m.store.Save(userID, token)
		return nil
	}

	return info, pollFn, nil
}

func (m *Manager) IsConnected(userID int64) bool {
	return m.store.Get(userID) != nil
}

func (m *Manager) Disconnect(userID int64) {
	m.store.Delete(userID)
}

// Upload downloads nothing — it uploads the already-downloaded file to the user's Drive.
func (m *Manager) Upload(ctx context.Context, userID int64, title, filePath string) (string, error) {
	token := m.store.Get(userID)
	if token == nil {
		return "", fmt.Errorf("not connected — use /auth")
	}

	src := &persistingTokenSource{
		userID: userID,
		src:    m.cfg.TokenSource(ctx, token),
		store:  m.store,
		last:   token,
	}
	svc, err := drive.NewService(ctx, option.WithTokenSource(src))
	if err != nil {
		return "", fmt.Errorf("creating drive service: %w", err)
	}

	return uploadUserFile(ctx, svc, title, filePath)
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
