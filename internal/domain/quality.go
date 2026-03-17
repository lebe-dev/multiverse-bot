package domain

import "context"

// FormatEntry is a single available resolution with its estimated size.
type FormatEntry struct {
	Height int
	Size   int64 // bytes, 0 if unknown
}

// FormatSummary holds key size information extracted from format metadata.
type FormatSummary struct {
	Title    string
	Duration float64 // seconds

	MinHeight int
	MinSize   int64

	P720Height int
	P720Size   int64

	MaxHeight int
	MaxSize   int64

	// Entries lists all available video resolutions sorted by height ascending.
	Entries []FormatEntry
}

// QualityDownloader can download at a specific quality or analyze available formats.
type QualityDownloader interface {
	DownloadQuality(ctx context.Context, url, quality string) (*Video, error)
	DownloadBest(ctx context.Context, url string) (*Video, error)
	AnalyzeFormats(ctx context.Context, url string) (*FormatSummary, error)
}
