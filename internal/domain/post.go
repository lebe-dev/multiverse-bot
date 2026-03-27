package domain

import (
	"context"
	"time"
)

// PostSubscription represents a user's subscription to an Instagram account's posts.
type PostSubscription struct {
	ID        int64
	UserID    int64
	Username  string
	CreatedAt time.Time
}

// PostItem represents a single post found during a fetch (metadata only).
type PostItem struct {
	PostID    string
	Username  string
	Title     string
	URL       string
	Timestamp time.Time
}

// PostSubscriptionStore persists Instagram post subscriptions and seen state.
type PostSubscriptionStore interface {
	AddPostSubscription(ctx context.Context, userID int64, username string) error
	RemovePostSubscription(ctx context.Context, userID int64, username string) error
	GetPostSubscriptions(ctx context.Context, userID int64) ([]PostSubscription, error)
	CountPostSubscriptions(ctx context.Context, userID int64) (int, error)
	GetAllUniquePostUsernames(ctx context.Context) ([]string, error)
	GetPostSubscribers(ctx context.Context, username string) ([]int64, error)
	HasSeenPost(ctx context.Context, userID int64, username, postID string) (bool, error)
	MarkPostSeen(ctx context.Context, userID int64, username, postID string) error
	CleanupExpiredSeenPosts(ctx context.Context) (int64, error)
}

// PostFetcher lists recent post IDs for an Instagram profile.
type PostFetcher interface {
	FetchPostIDs(ctx context.Context, username string) ([]PostItem, error)
}

// PostNotifier sends a notification about a new Instagram post to a Telegram user.
type PostNotifier interface {
	NotifyNewPost(ctx context.Context, userID int64, post PostItem) error
}
