package domain

import "context"

type Downloader interface {
	Download(ctx context.Context, url string) (*Video, error)
	Supports(platform Platform) bool
}
