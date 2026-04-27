package usecase

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

// maxVideoAge bounds how old a video may be and still trigger a notification.
// Prevents re-flooding users with ancient clips after a long downtime or after
// CleanupExpiredSeen wipes the seen_videos history older than its TTL.
const maxVideoAge = 48 * time.Hour

type WatchService struct {
	store            domain.SubscriptionStore
	fetcher          domain.FeedFetcher
	resolver         domain.ChannelResolver
	notifier         domain.Notifier
	log              *slog.Logger
	pollInterval     time.Duration
	maxSubs          int
	maxChannelsTotal int
}

func NewWatchService(
	store domain.SubscriptionStore,
	fetcher domain.FeedFetcher,
	resolver domain.ChannelResolver,
	notifier domain.Notifier,
	log *slog.Logger,
	pollInterval time.Duration,
	maxSubs int,
	maxChannelsTotal int,
) *WatchService {
	return &WatchService{
		store:            store,
		fetcher:          fetcher,
		resolver:         resolver,
		notifier:         notifier,
		log:              log,
		pollInterval:     pollInterval,
		maxSubs:          maxSubs,
		maxChannelsTotal: maxChannelsTotal,
	}
}

func (s *WatchService) Subscribe(ctx context.Context, userID int64, channelInput string) error {
	channelID, channelName, err := s.resolver.Resolve(ctx, channelInput)
	if err != nil {
		return err
	}

	count, err := s.store.CountSubscriptions(ctx, userID)
	if err != nil {
		return err
	}
	if count >= s.maxSubs {
		return domain.ErrMaxSubscriptions
	}

	channels, err := s.store.GetAllUniqueChannels(ctx)
	if err != nil {
		return err
	}
	if !slices.Contains(channels, channelID) && len(channels) >= s.maxChannelsTotal {
		return domain.ErrMaxChannels
	}

	if err := s.store.AddSubscription(ctx, userID, channelID, channelName); err != nil {
		return err
	}

	// Seed current feed as seen to avoid sending old videos as notifications.
	videos, err := s.fetcher.FetchFeed(ctx, channelID)
	if err != nil {
		s.log.Warn("failed to seed feed on subscribe", "channel", channelID, "error", err)
		return nil
	}
	for _, v := range videos {
		if err := s.store.MarkVideoSeen(ctx, userID, channelID, v.VideoID); err != nil {
			s.log.Warn("failed to mark video as seen on subscribe", "video", v.VideoID, "error", err)
		}
	}
	return nil
}

func (s *WatchService) Unsubscribe(ctx context.Context, userID int64, channelID string) error {
	return s.store.RemoveSubscription(ctx, userID, channelID)
}

func (s *WatchService) ListSubscriptions(ctx context.Context, userID int64) ([]domain.Subscription, error) {
	return s.store.GetSubscriptions(ctx, userID)
}

// Poll runs one poll cycle: fetches feeds for all tracked channels and notifies
// subscribers about new videos. Channels are processed sequentially to avoid
// hammering YouTube. Errors per channel are logged but do not abort the cycle.
func (s *WatchService) Poll(ctx context.Context) {
	start := time.Now()
	s.log.Debug("starting poll cycle")

	channels, err := s.store.GetAllUniqueChannels(ctx)
	if err != nil {
		s.log.Error("failed to get channels for polling", "error", err)
		return
	}

	for _, channelID := range channels {
		if ctx.Err() != nil {
			return
		}
		s.pollChannel(ctx, channelID)
	}

	deleted, err := s.store.CleanupExpiredSeen(ctx)
	if err != nil {
		s.log.Warn("failed to cleanup expired seen_videos", "error", err)
	} else if deleted > 0 {
		s.log.Debug("cleaned up expired seen_videos", "count", deleted)
	}

	s.log.Debug("poll cycle done", "duration", time.Since(start), "channels", len(channels))
}

func (s *WatchService) pollChannel(ctx context.Context, channelID string) {
	fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	videos, err := s.fetcher.FetchFeed(fetchCtx, channelID)
	if err != nil {
		s.log.Error("failed to fetch feed", "channel", channelID, "error", err)
		return
	}

	subscribers, err := s.store.GetSubscribers(ctx, channelID)
	if err != nil {
		s.log.Error("failed to get subscribers", "channel", channelID, "error", err)
		return
	}

	for _, userID := range subscribers {
		for _, video := range videos {
			if video.Published.IsZero() || time.Since(video.Published) > maxVideoAge {
				s.log.Debug("skipping stale video", "video", video.VideoID, "published", video.Published)
				continue
			}

			seen, err := s.store.HasSeenVideo(ctx, userID, channelID, video.VideoID)
			if err != nil {
				s.log.Error("failed to check seen video", "video", video.VideoID, "error", err)
				continue
			}
			if seen {
				continue
			}

			// Notify first, then mark seen — prefer duplicate notifications over missed ones.
			if err := s.notifier.NotifyNewVideo(ctx, userID, video); err != nil {
				s.log.Error("failed to notify user", "user", userID, "video", video.VideoID, "error", err)
				continue
			}
			if err := s.store.MarkVideoSeen(ctx, userID, channelID, video.VideoID); err != nil {
				s.log.Error("failed to mark video seen", "video", video.VideoID, "error", err)
			}
		}
	}
}

// Run loops forever, calling Poll then sleeping for pollInterval. The next
// timer starts only after Poll completes. Returns ctx.Err() when cancelled.
func (s *WatchService) Run(ctx context.Context) error {
	for {
		s.Poll(ctx)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(s.pollInterval):
		}
	}
}
