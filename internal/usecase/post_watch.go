package usecase

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

type PostWatchService struct {
	store         domain.PostSubscriptionStore
	fetcher       domain.PostFetcher
	resolver      domain.StoryResolver // reuses Instagram username resolver
	notifier      domain.PostNotifier
	log           *slog.Logger
	pollInterval  time.Duration
	maxSubs       int
	maxUsersTotal int
}

func NewPostWatchService(
	store domain.PostSubscriptionStore,
	fetcher domain.PostFetcher,
	resolver domain.StoryResolver,
	notifier domain.PostNotifier,
	log *slog.Logger,
	pollInterval time.Duration,
	maxSubs int,
	maxUsersTotal int,
) *PostWatchService {
	return &PostWatchService{
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

func (s *PostWatchService) Subscribe(ctx context.Context, userID int64, input string) error {
	s.log.Debug("resolving instagram username for posts", "user_id", userID, "input", input)

	username, err := s.resolver.Resolve(ctx, input)
	if err != nil {
		s.log.Warn("instagram resolve failed", "user_id", userID, "input", input, "error", err)
		return err
	}
	s.log.Debug("instagram username resolved for posts", "user_id", userID, "input", input, "username", username)

	count, err := s.store.CountPostSubscriptions(ctx, userID)
	if err != nil {
		return err
	}
	if count >= s.maxSubs {
		return domain.ErrMaxSubscriptions
	}

	usernames, err := s.store.GetAllUniquePostUsernames(ctx)
	if err != nil {
		return err
	}
	if !slices.Contains(usernames, username) && len(usernames) >= s.maxUsersTotal {
		return domain.ErrMaxChannels
	}

	if err := s.store.AddPostSubscription(ctx, userID, username); err != nil {
		return err
	}
	s.log.Info("post subscription added", "user_id", userID, "username", username, "existing_subs", count)

	// Seed current posts as seen to avoid sending existing posts.
	posts, err := s.fetcher.FetchPostIDs(ctx, username)
	if err != nil {
		s.log.Warn("failed to seed posts on subscribe", "username", username, "error", err)
		return nil
	}
	for _, p := range posts {
		if err := s.store.MarkPostSeen(ctx, userID, username, p.PostID); err != nil {
			s.log.Warn("failed to mark post as seen on subscribe", "post", p.PostID, "error", err)
		}
	}
	s.log.Debug("seeded existing posts as seen", "username", username, "count", len(posts))
	return nil
}

func (s *PostWatchService) Unsubscribe(ctx context.Context, userID int64, username string) error {
	if err := s.store.RemovePostSubscription(ctx, userID, username); err != nil {
		return err
	}
	s.log.Info("post subscription removed", "user_id", userID, "username", username)
	return nil
}

func (s *PostWatchService) ListSubscriptions(ctx context.Context, userID int64) ([]domain.PostSubscription, error) {
	return s.store.GetPostSubscriptions(ctx, userID)
}

func (s *PostWatchService) Poll(ctx context.Context) {
	start := time.Now()
	s.log.Debug("starting post poll cycle")

	usernames, err := s.store.GetAllUniquePostUsernames(ctx)
	if err != nil {
		s.log.Error("failed to get post usernames for polling", "error", err)
		return
	}

	for _, username := range usernames {
		if ctx.Err() != nil {
			return
		}
		s.pollUsername(ctx, username)
	}

	deleted, err := s.store.CleanupExpiredSeenPosts(ctx)
	if err != nil {
		s.log.Warn("failed to cleanup expired seen_posts", "error", err)
	} else if deleted > 0 {
		s.log.Debug("cleaned up expired seen_posts", "count", deleted)
	}

	s.log.Debug("post poll cycle done", "duration", time.Since(start), "usernames", len(usernames))
}

func (s *PostWatchService) pollUsername(ctx context.Context, username string) {
	fetchCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	s.log.Debug("fetching posts", "username", username)

	posts, err := s.fetcher.FetchPostIDs(fetchCtx, username)
	if err != nil {
		s.log.Error("failed to fetch posts", "username", username, "error", err)
		return
	}

	s.log.Debug("posts fetched", "username", username, "count", len(posts))

	subscribers, err := s.store.GetPostSubscribers(ctx, username)
	if err != nil {
		s.log.Error("failed to get post subscribers", "username", username, "error", err)
		return
	}
	s.log.Debug("post subscribers", "username", username, "count", len(subscribers))

	for _, post := range posts {
		if ctx.Err() != nil {
			return
		}

		for _, userID := range subscribers {
			seen, err := s.store.HasSeenPost(ctx, userID, username, post.PostID)
			if err != nil {
				s.log.Error("failed to check seen post", "post", post.PostID, "error", err)
				continue
			}
			if seen {
				continue
			}

			// Notify first, then mark seen — prefer duplicate notifications over missed ones.
			if err := s.notifier.NotifyNewPost(ctx, userID, post); err != nil {
				s.log.Error("failed to notify user about post", "user_id", userID, "post_id", post.PostID, "error", err)
				continue
			}
			s.log.Debug("post notification sent", "user_id", userID, "username", username, "post_id", post.PostID)
			if err := s.store.MarkPostSeen(ctx, userID, username, post.PostID); err != nil {
				s.log.Error("failed to mark post seen", "post", post.PostID, "error", err)
			}
		}
	}
}

func (s *PostWatchService) Run(ctx context.Context) error {
	for {
		s.Poll(ctx)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(s.pollInterval):
		}
	}
}
