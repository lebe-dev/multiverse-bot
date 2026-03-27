package usecase

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

// Export/import data types.

type ExportData struct {
	Version                     int                `json:"version"`
	ExportedAt                  time.Time          `json:"exported_at"`
	YoutubeSubscriptions        []YoutubeSubExport `json:"youtube_subscriptions"`
	InstagramStorySubscriptions []InstagramSubExport `json:"instagram_story_subscriptions"`
	InstagramPostSubscriptions  []InstagramSubExport `json:"instagram_post_subscriptions"`
	Settings                    *SettingsExport      `json:"settings,omitempty"`
}

type YoutubeSubExport struct {
	ChannelID   string `json:"channel_id"`
	ChannelName string `json:"channel_name"`
}

type InstagramSubExport struct {
	Username string `json:"username"`
}

type SettingsExport struct {
	Quality string `json:"quality"`
	Caption bool   `json:"caption"`
}

type ImportResult struct {
	YoutubeAdded   int
	YoutubeSkipped int
	YoutubeFailed  int

	StoriesAdded   int
	StoriesSkipped int
	StoriesFailed  int

	PostsAdded   int
	PostsSkipped int
	PostsFailed  int

	SettingsApplied bool
}

const currentExportVersion = 1

var ErrUnsupportedVersion = errors.New("unsupported export version")

// TransferService handles export and import of user subscriptions.
type TransferService struct {
	subStore   domain.SubscriptionStore
	storyStore domain.StorySubscriptionStore
	postStore  domain.PostSubscriptionStore
	fetcher    domain.FeedFetcher
	storyFetch domain.StoryFetcher
	postFetch  domain.PostFetcher
	log        *slog.Logger
}

func NewTransferService(
	subStore domain.SubscriptionStore,
	storyStore domain.StorySubscriptionStore,
	postStore domain.PostSubscriptionStore,
	fetcher domain.FeedFetcher,
	storyFetch domain.StoryFetcher,
	postFetch domain.PostFetcher,
	log *slog.Logger,
) *TransferService {
	return &TransferService{
		subStore:   subStore,
		storyStore: storyStore,
		postStore:  postStore,
		fetcher:    fetcher,
		storyFetch: storyFetch,
		postFetch:  postFetch,
		log:        log,
	}
}

// Export collects all user subscriptions and returns them as ExportData.
// Settings are passed in from the caller (adapter layer owns settings).
func (s *TransferService) Export(ctx context.Context, userID int64, settings *SettingsExport) (*ExportData, error) {
	subs, err := s.subStore.GetSubscriptions(ctx, userID)
	if err != nil {
		return nil, err
	}

	storySubs, err := s.storyStore.GetStorySubscriptions(ctx, userID)
	if err != nil {
		return nil, err
	}

	postSubs, err := s.postStore.GetPostSubscriptions(ctx, userID)
	if err != nil {
		return nil, err
	}

	data := &ExportData{
		Version:                     currentExportVersion,
		ExportedAt:                  time.Now().UTC(),
		YoutubeSubscriptions:        make([]YoutubeSubExport, 0, len(subs)),
		InstagramStorySubscriptions: make([]InstagramSubExport, 0, len(storySubs)),
		InstagramPostSubscriptions:  make([]InstagramSubExport, 0, len(postSubs)),
		Settings:                    settings,
	}

	for _, sub := range subs {
		data.YoutubeSubscriptions = append(data.YoutubeSubscriptions, YoutubeSubExport{
			ChannelID:   sub.ChannelID,
			ChannelName: sub.ChannelName,
		})
	}
	for _, sub := range storySubs {
		data.InstagramStorySubscriptions = append(data.InstagramStorySubscriptions, InstagramSubExport{
			Username: sub.Username,
		})
	}
	for _, sub := range postSubs {
		data.InstagramPostSubscriptions = append(data.InstagramPostSubscriptions, InstagramSubExport{
			Username: sub.Username,
		})
	}

	return data, nil
}

// Import restores subscriptions from ExportData and seeds feeds to avoid old notifications.
func (s *TransferService) Import(ctx context.Context, userID int64, data *ExportData) (*ImportResult, error) {
	if data.Version != currentExportVersion {
		return nil, ErrUnsupportedVersion
	}

	result := &ImportResult{}

	// YouTube subscriptions.
	for _, sub := range data.YoutubeSubscriptions {
		if err := s.subStore.AddSubscription(ctx, userID, sub.ChannelID, sub.ChannelName); err != nil {
			if errors.Is(err, domain.ErrAlreadySubscribed) {
				result.YoutubeSkipped++
				continue
			}
			s.log.Warn("failed to import youtube subscription", "channel", sub.ChannelID, "error", err)
			result.YoutubeFailed++
			continue
		}
		result.YoutubeAdded++
		s.seedYouTubeFeed(ctx, userID, sub.ChannelID)
	}

	// Instagram story subscriptions.
	for _, sub := range data.InstagramStorySubscriptions {
		if err := s.storyStore.AddStorySubscription(ctx, userID, sub.Username); err != nil {
			if errors.Is(err, domain.ErrAlreadySubscribedStory) {
				result.StoriesSkipped++
				continue
			}
			s.log.Warn("failed to import story subscription", "username", sub.Username, "error", err)
			result.StoriesFailed++
			continue
		}
		result.StoriesAdded++
		s.seedStories(ctx, userID, sub.Username)
	}

	// Instagram post subscriptions.
	for _, sub := range data.InstagramPostSubscriptions {
		if err := s.postStore.AddPostSubscription(ctx, userID, sub.Username); err != nil {
			if errors.Is(err, domain.ErrAlreadySubscribedPost) {
				result.PostsSkipped++
				continue
			}
			s.log.Warn("failed to import post subscription", "username", sub.Username, "error", err)
			result.PostsFailed++
			continue
		}
		result.PostsAdded++
		s.seedPosts(ctx, userID, sub.Username)
	}

	return result, nil
}

func (s *TransferService) seedYouTubeFeed(ctx context.Context, userID int64, channelID string) {
	videos, err := s.fetcher.FetchFeed(ctx, channelID)
	if err != nil {
		s.log.Warn("failed to seed feed on import", "channel", channelID, "error", err)
		return
	}
	for _, v := range videos {
		if err := s.subStore.MarkVideoSeen(ctx, userID, channelID, v.VideoID); err != nil {
			s.log.Warn("failed to mark video as seen on import", "video", v.VideoID, "error", err)
		}
	}
}

func (s *TransferService) seedStories(ctx context.Context, userID int64, username string) {
	stories, err := s.storyFetch.FetchStoryIDs(ctx, username)
	if err != nil {
		s.log.Warn("failed to seed stories on import", "username", username, "error", err)
		return
	}
	for _, st := range stories {
		if err := s.storyStore.MarkStorySeen(ctx, userID, username, st.StoryID); err != nil {
			s.log.Warn("failed to mark story as seen on import", "story", st.StoryID, "error", err)
		}
	}
}

func (s *TransferService) seedPosts(ctx context.Context, userID int64, username string) {
	posts, err := s.postFetch.FetchPostIDs(ctx, username)
	if err != nil {
		s.log.Warn("failed to seed posts on import", "username", username, "error", err)
		return
	}
	for _, p := range posts {
		if err := s.postStore.MarkPostSeen(ctx, userID, username, p.PostID); err != nil {
			s.log.Warn("failed to mark post as seen on import", "post", p.PostID, "error", err)
		}
	}
}
