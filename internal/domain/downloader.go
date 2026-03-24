package domain

import "context"

type Downloader interface {
	Download(ctx context.Context, url string) (*Video, error)
	DownloadMedia(ctx context.Context, url string) (*MediaResult, error)
	Supports(platform Platform) bool
}
