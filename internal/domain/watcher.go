package domain

import "context"

type SubscriptionStore interface {
	AddSubscription(ctx context.Context, userID int64, channelID, channelName string) error
	RemoveSubscription(ctx context.Context, userID int64, channelID string) error
	GetSubscriptions(ctx context.Context, userID int64) ([]Subscription, error)
	CountSubscriptions(ctx context.Context, userID int64) (int, error)
	GetAllUniqueChannels(ctx context.Context) ([]string, error)
	GetSubscribers(ctx context.Context, channelID string) ([]int64, error)
	HasSeenVideo(ctx context.Context, userID int64, channelID, videoID string) (bool, error)
	MarkVideoSeen(ctx context.Context, userID int64, channelID, videoID string) error
	CleanupExpiredSeen(ctx context.Context) (int64, error)
}

type FeedFetcher interface {
	FetchFeed(ctx context.Context, channelID string) ([]FeedVideo, error)
}

type ChannelResolver interface {
	Resolve(ctx context.Context, input string) (channelID, channelName string, err error)
}

type Notifier interface {
	NotifyNewVideo(ctx context.Context, userID int64, video FeedVideo) error
}
