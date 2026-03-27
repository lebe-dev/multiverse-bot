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
	log           *slog.Logger
	pollInterval  time.Duration
	maxSubs       int
	maxUsersTotal int
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
	username, err := s.resolver.Resolve(ctx, input)
	if err != nil {
		return err
	}

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
	return nil
}

func (s *StoryWatchService) Unsubscribe(ctx context.Context, userID int64, username string) error {
	return s.store.RemoveStorySubscription(ctx, userID, username)
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

	stories, err := s.fetcher.FetchStoryIDs(fetchCtx, username)
	if err != nil {
		s.log.Error("failed to fetch stories", "username", username, "error", err)
		return
	}

	subscribers, err := s.store.GetStorySubscribers(ctx, username)
	if err != nil {
		s.log.Error("failed to get story subscribers", "username", username, "error", err)
		return
	}

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
			continue
		}

		// Download once for all subscribers.
		dlCtx, dlCancel := context.WithTimeout(ctx, 2*time.Minute)
		media, err := s.fetcher.DownloadStory(dlCtx, username, story.StoryID)
		dlCancel()
		if err != nil {
			s.log.Error("failed to download story", "username", username, "story", story.StoryID, "error", err)
			continue
		}

		// Notify all unseen users, then mark as seen.
		for _, userID := range unseenUsers {
			if err := s.notifier.NotifyNewStory(ctx, userID, *media); err != nil {
				s.log.Error("failed to notify user about story", "user", userID, "story", story.StoryID, "error", err)
				continue
			}
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

