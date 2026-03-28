package usecase

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"slices"
	"time"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

// defaultPollJitter is the default maximum random delay between processing
// Instagram usernames during a poll cycle (range: jitter/3 .. jitter).
const defaultPollJitter = 180 * time.Second

type StoryWatchService struct {
	store         domain.StorySubscriptionStore
	fetcher       domain.StoryFetcher
	resolver      domain.StoryResolver
	notifier      domain.StoryNotifier
	enricher      domain.StoryMetadataEnricher
	log           *slog.Logger
	pollInterval  time.Duration
	pollJitter    time.Duration // max inter-username delay; 0 disables
	maxSubs       int
	maxUsersTotal int
}

// SetMetadataEnricher sets an optional enricher that detects reshared stories.
func (s *StoryWatchService) SetMetadataEnricher(e domain.StoryMetadataEnricher) {
	s.enricher = e
}

// SetPollJitter overrides the maximum inter-username delay (default 180s).
// Set to 0 to disable delays (useful in tests).
func (s *StoryWatchService) SetPollJitter(d time.Duration) { s.pollJitter = d }

// Fetcher returns the story fetcher for on-demand downloads (e.g. callback handlers).
func (s *StoryWatchService) Fetcher() domain.StoryFetcher { return s.fetcher }

// Enricher returns the optional metadata enricher (may be nil).
func (s *StoryWatchService) Enricher() domain.StoryMetadataEnricher { return s.enricher }

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
		pollJitter:    defaultPollJitter,
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

	for i, username := range usernames {
		if ctx.Err() != nil {
			return
		}
		if i > 0 && s.pollJitter > 0 {
			minDelay := s.pollJitter / 3
			delay := minDelay + time.Duration(rand.Int64N(int64(s.pollJitter-minDelay)))
			s.log.Debug("sleeping between story usernames", "delay", delay)
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
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

		for _, userID := range subscribers {
			seen, err := s.store.HasSeenStory(ctx, userID, username, story.StoryID)
			if err != nil {
				s.log.Error("failed to check seen story", "story", story.StoryID, "error", err)
				continue
			}
			if seen {
				continue
			}

			// Notify first, then mark seen — prefer duplicate notifications over missed ones.
			if err := s.notifier.NotifyNewStory(ctx, userID, story); err != nil {
				s.log.Error("failed to notify user about story", "user_id", userID, "story_id", story.StoryID, "error", err)
				continue
			}
			s.log.Debug("story notification sent", "user_id", userID, "username", username, "story_id", story.StoryID)
			if err := s.store.MarkStorySeen(ctx, userID, username, story.StoryID); err != nil {
				s.log.Error("failed to mark story seen", "story", story.StoryID, "error", err)
			}
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

