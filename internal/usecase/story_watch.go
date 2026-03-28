package usecase

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"time"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

type StoryWatchService struct {
	store         domain.StorySubscriptionStore
	fetcher       domain.StoryFetcher
	resolver      domain.StoryResolver
	notifier      domain.StoryNotifier
	enricher      domain.StoryMetadataEnricher
	log           *slog.Logger
	pollInterval  time.Duration
	maxSubs       int
	maxUsersTotal int
}

// SetMetadataEnricher sets an optional enricher that detects reshared stories.
func (s *StoryWatchService) SetMetadataEnricher(e domain.StoryMetadataEnricher) {
	s.enricher = e
}

func NewStoryWatchService(
	store domain.StorySubscriptionStore,
	fetcher domain.StoryFetcher,
	resolver domain.StoryResolver,
	notifier domain.StoryNotifier,
	log *slog.Logger,
	pollInterval time.Duration,
	maxSubs int,
	maxUsersTotal int,
) *StoryWatchService {
	return &StoryWatchService{
		store:         store,
		fetcher:       fetcher,
		resolver:      resolver,
		notifier:      notifier,
		log:           log,
		pollInterval:  pollInterval,
		maxSubs:       maxSubs,
		maxUsersTotal: maxUsersTotal,
	}
}

func (s *StoryWatchService) Subscribe(ctx context.Context, userID int64, input string) error {
	s.log.Debug("resolving instagram username", "user_id", userID, "input", input)

	username, err := s.resolver.Resolve(ctx, input)
	if err != nil {
		s.log.Warn("instagram resolve failed", "user_id", userID, "input", input, "error", err)
		return err
	}
	s.log.Debug("instagram username resolved", "user_id", userID, "input", input, "username", username)

	count, err := s.store.CountStorySubscriptions(ctx, userID)
	if err != nil {
		return err
	}
	if count >= s.maxSubs {
		return domain.ErrMaxSubscriptions
	}

	usernames, err := s.store.GetAllUniqueStoryUsernames(ctx)
	if err != nil {
		return err
	}
	if !slices.Contains(usernames, username) && len(usernames) >= s.maxUsersTotal {
		return domain.ErrMaxChannels
	}

	if err := s.store.AddStorySubscription(ctx, userID, username); err != nil {
		return err
	}
	s.log.Info("story subscription added", "user_id", userID, "username", username, "existing_subs", count)

	// Seed current stories as seen to avoid sending existing stories.
	stories, err := s.fetcher.FetchStoryIDs(ctx, username)
	if err != nil {
		s.log.Warn("failed to seed stories on subscribe", "username", username, "error", err)
		return nil
	}
	for _, st := range stories {
		if err := s.store.MarkStorySeen(ctx, userID, username, st.StoryID); err != nil {
			s.log.Warn("failed to mark story as seen on subscribe", "story", st.StoryID, "error", err)
		}
	}
	s.log.Debug("seeded existing stories as seen", "username", username, "count", len(stories))
	return nil
}

func (s *StoryWatchService) Unsubscribe(ctx context.Context, userID int64, username string) error {
	if err := s.store.RemoveStorySubscription(ctx, userID, username); err != nil {
		return err
	}
	s.log.Info("story subscription removed", "user_id", userID, "username", username)
	return nil
}

func (s *StoryWatchService) ListSubscriptions(ctx context.Context, userID int64) ([]domain.StorySubscription, error) {
	return s.store.GetStorySubscriptions(ctx, userID)
}

func (s *StoryWatchService) Poll(ctx context.Context) {
	start := time.Now()
	s.log.Debug("starting story poll cycle")

	usernames, err := s.store.GetAllUniqueStoryUsernames(ctx)
	if err != nil {
		s.log.Error("failed to get story usernames for polling", "error", err)
		return
	}

	for _, username := range usernames {
		if ctx.Err() != nil {
			return
		}
		s.pollUsername(ctx, username)
	}

	deleted, err := s.store.CleanupExpiredSeenStories(ctx)
	if err != nil {
		s.log.Warn("failed to cleanup expired seen_stories", "error", err)
	} else if deleted > 0 {
		s.log.Debug("cleaned up expired seen_stories", "count", deleted)
	}

	s.log.Debug("story poll cycle done", "duration", time.Since(start), "usernames", len(usernames))
}

func (s *StoryWatchService) pollUsername(ctx context.Context, username string) {
	fetchCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	s.log.Debug("fetching stories", "username", username)

	stories, err := s.fetcher.FetchStoryIDs(fetchCtx, username)
	if err != nil {
		s.log.Error("failed to fetch stories", "username", username, "error", err)
		return
	}

	s.log.Debug("stories fetched", "username", username, "count", len(stories))

	subscribers, err := s.store.GetStorySubscribers(ctx, username)
	if err != nil {
		s.log.Error("failed to get story subscribers", "username", username, "error", err)
		return
	}
	s.log.Debug("story subscribers", "username", username, "count", len(subscribers))

	for _, story := range stories {
		if ctx.Err() != nil {
			return
		}

		// Find subscribers who haven't seen this story.
		var unseenUsers []int64
		for _, userID := range subscribers {
			seen, err := s.store.HasSeenStory(ctx, userID, username, story.StoryID)
			if err != nil {
				s.log.Error("failed to check seen story", "story", story.StoryID, "error", err)
				continue
			}
			if !seen {
				unseenUsers = append(unseenUsers, userID)
			}
		}

		if len(unseenUsers) == 0 {
			s.log.Debug("story already seen by all subscribers", "username", username, "story_id", story.StoryID, "subscribers", len(subscribers))
			continue
		}

		s.log.Info("new story detected", "username", username, "story_id", story.StoryID, "unseen_users", len(unseenUsers))

		// Download once for all subscribers.
		dlCtx, dlCancel := context.WithTimeout(ctx, 2*time.Minute)
		media, err := s.fetcher.DownloadStory(dlCtx, username, story.StoryID)
		dlCancel()
		if err != nil {
			s.log.Error("failed to download story", "username", username, "story", story.StoryID, "error", err)
			continue
		}
		s.log.Debug("story downloaded", "username", username, "story_id", story.StoryID, "type", media.Type, "size", media.Size)

		if s.enricher != nil {
			enrichCtx, enrichCancel := context.WithTimeout(ctx, 15*time.Second)
			reshare, err := s.enricher.EnrichStoryMetadata(enrichCtx, username, story.StoryID)
			enrichCancel()
			if err != nil {
				s.log.Warn("failed to enrich story metadata", "username", username, "story_id", story.StoryID, "error", err)
			} else if reshare != nil {
				media.Reshare = reshare
				s.log.Info("story is a reshare", "username", username, "story_id", story.StoryID, "original_author", reshare.Username)
			}
		}

		// Notify all unseen users, then mark as seen.
		for _, userID := range unseenUsers {
			if err := s.notifier.NotifyNewStory(ctx, userID, *media); err != nil {
				s.log.Error("failed to notify user about story", "user_id", userID, "story_id", story.StoryID, "error", err)
				continue
			}
			s.log.Debug("story notification sent", "user_id", userID, "username", username, "story_id", story.StoryID)
			if err := s.store.MarkStorySeen(ctx, userID, username, story.StoryID); err != nil {
				s.log.Error("failed to mark story seen", "story", story.StoryID, "error", err)
			}
		}

		// Cleanup downloaded file.
		if media.FilePath != "" {
			_ = os.RemoveAll(filepath.Dir(media.FilePath))
		}
	}
}

func (s *StoryWatchService) Run(ctx context.Context) error {
	for {
		s.Poll(ctx)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(s.pollInterval):
		}
	}
}

