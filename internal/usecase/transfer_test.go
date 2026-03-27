package usecase_test

import (
	"context"
	"testing"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
	"gitlab.com/tiny-services/multiverse-bot/internal/usecase"
)

func newTransferSvc(
	store *mockStore,
	storyStore *mockStoryStore,
	postStore *mockPostStore,
	fetcher *mockFetcher,
	storyFetcher *mockStoryFetcher,
	postFetcher *mockPostFetcher,
) *usecase.TransferService {
	return usecase.NewTransferService(store, storyStore, postStore, fetcher, storyFetcher, postFetcher, newLogger())
}

func TestExport_WithData(t *testing.T) {
	store := newMockStore()
	storyStore := newMockStoryStore()
	postStore := newMockPostStore()

	ctx := context.Background()
	_ = store.AddSubscription(ctx, 1, "UC123", "Channel A")
	_ = store.AddSubscription(ctx, 1, "UC456", "Channel B")
	_ = storyStore.AddStorySubscription(ctx, 1, "natgeo")
	_ = postStore.AddPostSubscription(ctx, 1, "nasa")

	settings := &usecase.SettingsExport{Quality: "1080p", Caption: true}
	svc := newTransferSvc(store, storyStore, postStore, &mockFetcher{}, &mockStoryFetcher{}, &mockPostFetcher{})

	data, err := svc.Export(ctx, 1, settings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if data.Version != 1 {
		t.Errorf("version = %d, want 1", data.Version)
	}
	if len(data.YoutubeSubscriptions) != 2 {
		t.Errorf("youtube subs = %d, want 2", len(data.YoutubeSubscriptions))
	}
	if data.YoutubeSubscriptions[0].ChannelID != "UC123" {
		t.Errorf("youtube sub[0] channel = %q, want UC123", data.YoutubeSubscriptions[0].ChannelID)
	}
	if len(data.InstagramStorySubscriptions) != 1 {
		t.Errorf("story subs = %d, want 1", len(data.InstagramStorySubscriptions))
	}
	if data.InstagramStorySubscriptions[0].Username != "natgeo" {
		t.Errorf("story sub[0] username = %q, want natgeo", data.InstagramStorySubscriptions[0].Username)
	}
	if len(data.InstagramPostSubscriptions) != 1 {
		t.Errorf("post subs = %d, want 1", len(data.InstagramPostSubscriptions))
	}
	if data.Settings == nil || data.Settings.Quality != "1080p" {
		t.Errorf("settings quality = %v, want 1080p", data.Settings)
	}
}

func TestExport_Empty(t *testing.T) {
	svc := newTransferSvc(newMockStore(), newMockStoryStore(), newMockPostStore(), &mockFetcher{}, &mockStoryFetcher{}, &mockPostFetcher{})

	data, err := svc.Export(context.Background(), 999, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(data.YoutubeSubscriptions) != 0 {
		t.Errorf("youtube subs = %d, want 0", len(data.YoutubeSubscriptions))
	}
	if len(data.InstagramStorySubscriptions) != 0 {
		t.Errorf("story subs = %d, want 0", len(data.InstagramStorySubscriptions))
	}
	if len(data.InstagramPostSubscriptions) != 0 {
		t.Errorf("post subs = %d, want 0", len(data.InstagramPostSubscriptions))
	}
	if data.Settings != nil {
		t.Errorf("settings = %v, want nil", data.Settings)
	}
}

func TestImport_Success(t *testing.T) {
	store := newMockStore()
	storyStore := newMockStoryStore()
	postStore := newMockPostStore()
	fetcher := &mockFetcher{videos: []domain.FeedVideo{{VideoID: "v1", ChannelID: "UC123"}}}
	storyFetcher := &mockStoryFetcher{stories: []domain.StoryItem{{StoryID: "s1", Username: "natgeo"}}}
	postFetcher := &mockPostFetcher{posts: []domain.PostItem{{PostID: "p1", Username: "nasa"}}}

	svc := newTransferSvc(store, storyStore, postStore, fetcher, storyFetcher, postFetcher)

	data := &usecase.ExportData{
		Version:                     1,
		YoutubeSubscriptions:        []usecase.YoutubeSubExport{{ChannelID: "UC123", ChannelName: "Test"}},
		InstagramStorySubscriptions: []usecase.InstagramSubExport{{Username: "natgeo"}},
		InstagramPostSubscriptions:  []usecase.InstagramSubExport{{Username: "nasa"}},
	}

	result, err := svc.Import(context.Background(), 1, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.YoutubeAdded != 1 {
		t.Errorf("youtube added = %d, want 1", result.YoutubeAdded)
	}
	if result.StoriesAdded != 1 {
		t.Errorf("stories added = %d, want 1", result.StoriesAdded)
	}
	if result.PostsAdded != 1 {
		t.Errorf("posts added = %d, want 1", result.PostsAdded)
	}

	// Verify feed was seeded.
	seen, _ := store.HasSeenVideo(context.Background(), 1, "UC123", "v1")
	if !seen {
		t.Error("video v1 should be marked as seen after import")
	}

	storySeen, _ := storyStore.HasSeenStory(context.Background(), 1, "natgeo", "s1")
	if !storySeen {
		t.Error("story s1 should be marked as seen after import")
	}

	postSeen, _ := postStore.HasSeenPost(context.Background(), 1, "nasa", "p1")
	if !postSeen {
		t.Error("post p1 should be marked as seen after import")
	}
}

func TestImport_DuplicatesSkipped(t *testing.T) {
	store := newMockStore()
	storyStore := newMockStoryStore()
	postStore := newMockPostStore()

	ctx := context.Background()
	_ = store.AddSubscription(ctx, 1, "UC123", "Test")
	_ = storyStore.AddStorySubscription(ctx, 1, "natgeo")
	_ = postStore.AddPostSubscription(ctx, 1, "nasa")

	svc := newTransferSvc(store, storyStore, postStore, &mockFetcher{}, &mockStoryFetcher{}, &mockPostFetcher{})

	data := &usecase.ExportData{
		Version:                     1,
		YoutubeSubscriptions:        []usecase.YoutubeSubExport{{ChannelID: "UC123", ChannelName: "Test"}},
		InstagramStorySubscriptions: []usecase.InstagramSubExport{{Username: "natgeo"}},
		InstagramPostSubscriptions:  []usecase.InstagramSubExport{{Username: "nasa"}},
	}

	result, err := svc.Import(ctx, 1, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.YoutubeSkipped != 1 {
		t.Errorf("youtube skipped = %d, want 1", result.YoutubeSkipped)
	}
	if result.StoriesSkipped != 1 {
		t.Errorf("stories skipped = %d, want 1", result.StoriesSkipped)
	}
	if result.PostsSkipped != 1 {
		t.Errorf("posts skipped = %d, want 1", result.PostsSkipped)
	}
	if result.YoutubeAdded != 0 || result.StoriesAdded != 0 || result.PostsAdded != 0 {
		t.Error("no subscriptions should have been added")
	}
}

func TestImport_InvalidVersion(t *testing.T) {
	svc := newTransferSvc(newMockStore(), newMockStoryStore(), newMockPostStore(), &mockFetcher{}, &mockStoryFetcher{}, &mockPostFetcher{})

	data := &usecase.ExportData{Version: 99}
	_, err := svc.Import(context.Background(), 1, data)
	if err != usecase.ErrUnsupportedVersion {
		t.Errorf("err = %v, want ErrUnsupportedVersion", err)
	}
}

func TestImport_MixedResults(t *testing.T) {
	store := newMockStore()
	storyStore := newMockStoryStore()
	postStore := newMockPostStore()

	ctx := context.Background()
	// Pre-subscribe to one channel so it gets skipped.
	_ = store.AddSubscription(ctx, 1, "UC123", "Old")

	fetcher := &mockFetcher{videos: []domain.FeedVideo{{VideoID: "v1", ChannelID: "UC456"}}}
	svc := newTransferSvc(store, storyStore, postStore, fetcher, &mockStoryFetcher{}, &mockPostFetcher{})

	data := &usecase.ExportData{
		Version: 1,
		YoutubeSubscriptions: []usecase.YoutubeSubExport{
			{ChannelID: "UC123", ChannelName: "Old"},
			{ChannelID: "UC456", ChannelName: "New"},
		},
	}

	result, err := svc.Import(ctx, 1, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.YoutubeAdded != 1 {
		t.Errorf("youtube added = %d, want 1", result.YoutubeAdded)
	}
	if result.YoutubeSkipped != 1 {
		t.Errorf("youtube skipped = %d, want 1", result.YoutubeSkipped)
	}
}
