package domain

import "context"

// CookieStore persists platform cookies (Netscape format) in the database.
type CookieStore interface {
	SaveCookies(ctx context.Context, platform string, data []byte) error
	GetCookies(ctx context.Context, platform string) ([]byte, error)
	DeleteCookies(ctx context.Context, platform string) error
	ListCookiePlatforms(ctx context.Context) ([]string, error)
}
