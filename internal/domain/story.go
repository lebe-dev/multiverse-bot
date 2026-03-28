package domain

import (
	"context"
	"time"
)

// StorySubscription represents a user's subscription to an Instagram account's stories.
type StorySubscription struct {
	ID        int64
	UserID    int64
	Username  string
	CreatedAt time.Time
}

// StoryItem represents a single story item found during a fetch (metadata only).
type StoryItem struct {
	StoryID   string
	Username  string
	Timestamp time.Time
}

// StoryReshare holds information about the original story when a story is a reshare/quote.
type StoryReshare struct {
	Username string // original story author's Instagram username
	StoryID  string // original story ID (may be empty if unavailable)
}

// StoryMedia represents a downloaded story media file ready for sending.
type StoryMedia struct {
	StoryID  string
	Username string
	FilePath string
	Type     MediaType
	Size     int64
	Reshare  *StoryReshare // nil if not a reshare
}

// StorySubscriptionStore persists Instagram story subscriptions and seen state.
type StorySubscriptionStore interface {
	AddStorySubscription(ctx context.Context, userID int64, username string) error
	RemoveStorySubscription(ctx context.Context, userID int64, username string) error
	GetStorySubscriptions(ctx context.Context, userID int64) ([]StorySubscription, error)
	CountStorySubscriptions(ctx context.Context, userID int64) (int, error)
	GetAllUniqueStoryUsernames(ctx context.Context) ([]string, error)
	GetStorySubscribers(ctx context.Context, username string) ([]int64, error)
	HasSeenStory(ctx context.Context, userID int64, username, storyID string) (bool, error)
	MarkStorySeen(ctx context.Context, userID int64, username, storyID string) error
	CleanupExpiredSeenStories(ctx context.Context) (int64, error)
}

// StoryFetcher lists story IDs and downloads individual stories via yt-dlp.
type StoryFetcher interface {
	FetchStoryIDs(ctx context.Context, username string) ([]StoryItem, error)
	DownloadStory(ctx context.Context, username string, storyID string) (*StoryMedia, error)
}

// StoryResolver validates and normalizes an Instagram profile URL or username.
type StoryResolver interface {
	Resolve(ctx context.Context, input string) (username string, err error)
}

// StoryMetadataEnricher enriches a story with additional metadata (e.g. reshare info).
// Implementations must tolerate failures — the caller delivers the story without
// enrichment when this returns an error.
type StoryMetadataEnricher interface {
	EnrichStoryMetadata(ctx context.Context, username string, storyID string) (*StoryReshare, error)
}

// StoryNotifier sends a downloaded story to a Telegram user.
type StoryNotifier interface {
	NotifyNewStory(ctx context.Context, userID int64, story StoryMedia) error
}
