package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
	"gitlab.com/tiny-services/multiverse-bot/internal/usecase"
)

// --- story mocks ---

type mockStoryStore struct {
	subs   map[string][]int64              // username → userIDs
	seen   map[string]struct{}             // "userID:username:storyID"
	subMap map[int64][]domain.StorySubscription
}

func newMockStoryStore() *mockStoryStore {
	return &mockStoryStore{
		subs:   make(map[string][]int64),
		seen:   make(map[string]struct{}),
		subMap: make(map[int64][]domain.StorySubscription),
	}
}

func (m *mockStoryStore) AddStorySubscription(_ context.Context, userID int64, username string) error {
	for _, sub := range m.subMap[userID] {
		if sub.Username == username {
			return domain.ErrAlreadySubscribedStory
		}
	}
	m.subMap[userID] = append(m.subMap[userID], domain.StorySubscription{
		UserID: userID, Username: username,
	})
	m.subs[username] = append(m.subs[username], userID)
	return nil
}

func (m *mockStoryStore) RemoveStorySubscription(_ context.Context, userID int64, username string) error {
	subs := m.subMap[userID]
	for i, sub := range subs {
		if sub.Username == username {
			m.subMap[userID] = append(subs[:i], subs[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotSubscribedStory
}

func (m *mockStoryStore) GetStorySubscriptions(_ context.Context, userID int64) ([]domain.StorySubscription, error) {
	return m.subMap[userID], nil
}

func (m *mockStoryStore) CountStorySubscriptions(_ context.Context, userID int64) (int, error) {
	return len(m.subMap[userID]), nil
}

func (m *mockStoryStore) GetAllUniqueStoryUsernames(_ context.Context) ([]string, error) {
	seen := make(map[string]struct{})
	var result []string
	for _, subs := range m.subMap {
		for _, sub := range subs {
			if _, ok := seen[sub.Username]; !ok {
				seen[sub.Username] = struct{}{}
				result = append(result, sub.Username)
			}
		}
	}
	return result, nil
}

func (m *mockStoryStore) GetStorySubscribers(_ context.Context, username string) ([]int64, error) {
	return m.subs[username], nil
}

func (m *mockStoryStore) HasSeenStory(_ context.Context, userID int64, username, storyID string) (bool, error) {
	_, ok := m.seen[storySeenKey(userID, username, storyID)]
	return ok, nil
}

func (m *mockStoryStore) MarkStorySeen(_ context.Context, userID int64, username, storyID string) error {
	m.seen[storySeenKey(userID, username, storyID)] = struct{}{}
	return nil
}

func (m *mockStoryStore) CleanupExpiredSeenStories(_ context.Context) (int64, error) {
	return 0, nil
}

func storySeenKey(userID int64, username, storyID string) string {
	return fmt.Sprintf("%d:%s:%s", userID, username, storyID)
}

type mockStoryResolver struct {
	username string
	err      error
}

func (m *mockStoryResolver) Resolve(_ context.Context, _ string) (string, error) {
	return m.username, m.err
}

type mockStoryFetcher struct {
	stories       []domain.StoryItem
	fetchErr      error
	downloaded    []string // storyIDs that were downloaded
	downloadMedia *domain.StoryMedia
	downloadErr   error
}

func (m *mockStoryFetcher) FetchStoryIDs(_ context.Context, _ string) ([]domain.StoryItem, error) {
	return m.stories, m.fetchErr
}

func (m *mockStoryFetcher) DownloadStory(_ context.Context, _ string, storyID string) (*domain.StoryMedia, error) {
	m.downloaded = append(m.downloaded, storyID)
	if m.downloadErr != nil {
		return nil, m.downloadErr
	}
	media := *m.downloadMedia
	media.StoryID = storyID
	return &media, nil
}

type mockStoryNotifier struct {
	notified []domain.StoryItem
	err      error
}

func (m *mockStoryNotifier) NotifyNewStory(_ context.Context, _ int64, story domain.StoryItem) error {
	m.notified = append(m.notified, story)
	return m.err
}

func newStoryWatchSvc(store *mockStoryStore, fetcher domain.StoryFetcher, resolver *mockStoryResolver, notifier *mockStoryNotifier) *usecase.StoryWatchService {
	svc := usecase.NewStoryWatchService(store, fetcher, resolver, notifier, newLogger(), time.Minute, 20, 100)
	svc.SetPollJitter(0) // disable delays in tests
	return svc
}

// --- tests ---

func TestStorySubscribe_Success(t *testing.T) {
	store := newMockStoryStore()
	fetcher := &mockStoryFetcher{stories: []domain.StoryItem{{StoryID: "s1", Username: "natgeo"}}}
	resolver := &mockStoryResolver{username: "natgeo"}
	notifier := &mockStoryNotifier{}

	svc := newStoryWatchSvc(store, fetcher, resolver, notifier)

	if err := svc.Subscribe(context.Background(), 1, "https://instagram.com/natgeo"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	subs, _ := store.GetStorySubscriptions(context.Background(), 1)
	if len(subs) != 1 {
		t.Errorf("expected 1 subscription, got %d", len(subs))
	}

	// Feed seeded: story should be marked seen
	seen, _ := store.HasSeenStory(context.Background(), 1, "natgeo", "s1")
	if !seen {
		t.Error("expected stories to be seeded after subscribe")
	}
}

func TestStorySubscribe_Duplicate(t *testing.T) {
	store := newMockStoryStore()
	resolver := &mockStoryResolver{username: "natgeo"}
	fetcher := &mockStoryFetcher{}
	svc := newStoryWatchSvc(store, fetcher, resolver, &mockStoryNotifier{})

	_ = svc.Subscribe(context.Background(), 1, "natgeo")
	err := svc.Subscribe(context.Background(), 1, "natgeo")
	if !errors.Is(err, domain.ErrAlreadySubscribedStory) {
		t.Errorf("expected ErrAlreadySubscribedStory, got %v", err)
	}
}

func TestStorySubscribe_LimitExceeded(t *testing.T) {
	store := newMockStoryStore()
	resolver := &mockStoryResolver{username: "user3"}
	fetcher := &mockStoryFetcher{}
	svc := usecase.NewStoryWatchService(store, fetcher, resolver, &mockStoryNotifier{}, newLogger(), time.Minute, 2, 100)

	_ = store.AddStorySubscription(context.Background(), 1, "user1")
	_ = store.AddStorySubscription(context.Background(), 1, "user2")

	err := svc.Subscribe(context.Background(), 1, "user3")
	if !errors.Is(err, domain.ErrMaxSubscriptions) {
		t.Errorf("expected ErrMaxSubscriptions, got %v", err)
	}
}

func TestStorySubscribe_UsernameNotFound(t *testing.T) {
	store := newMockStoryStore()
	resolver := &mockStoryResolver{err: domain.ErrUsernameNotFound}
	svc := newStoryWatchSvc(store, &mockStoryFetcher{}, resolver, &mockStoryNotifier{})

	err := svc.Subscribe(context.Background(), 1, "baduser")
	if !errors.Is(err, domain.ErrUsernameNotFound) {
		t.Errorf("expected ErrUsernameNotFound, got %v", err)
	}
}

func TestStoryUnsubscribe_Success(t *testing.T) {
	store := newMockStoryStore()
	resolver := &mockStoryResolver{username: "natgeo"}
	svc := newStoryWatchSvc(store, &mockStoryFetcher{}, resolver, &mockStoryNotifier{})

	_ = svc.Subscribe(context.Background(), 1, "natgeo")
	if err := svc.Unsubscribe(context.Background(), 1, "natgeo"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStoryUnsubscribe_NotSubscribed(t *testing.T) {
	store := newMockStoryStore()
	svc := newStoryWatchSvc(store, &mockStoryFetcher{}, &mockStoryResolver{}, &mockStoryNotifier{})

	err := svc.Unsubscribe(context.Background(), 1, "natgeo")
	if !errors.Is(err, domain.ErrNotSubscribedStory) {
		t.Errorf("expected ErrNotSubscribedStory, got %v", err)
	}
}

func TestStoryPoll_NewStory_Notified(t *testing.T) {
	store := newMockStoryStore()
	_ = store.AddStorySubscription(context.Background(), 1, "natgeo")

	fetcher := &mockStoryFetcher{
		stories: []domain.StoryItem{{StoryID: "new1", Username: "natgeo"}},
	}
	notifier := &mockStoryNotifier{}

	svc := newStoryWatchSvc(store, fetcher, &mockStoryResolver{}, notifier)
	svc.Poll(context.Background())

	if len(notifier.notified) != 1 {
		t.Errorf("expected 1 notification, got %d", len(notifier.notified))
	}
	if notifier.notified[0].StoryID != "new1" {
		t.Errorf("expected story ID new1, got %s", notifier.notified[0].StoryID)
	}
	// No downloads during polling — download happens on-demand via callback.
	if len(fetcher.downloaded) != 0 {
		t.Errorf("expected 0 downloads during poll, got %d", len(fetcher.downloaded))
	}

	seen, _ := store.HasSeenStory(context.Background(), 1, "natgeo", "new1")
	if !seen {
		t.Error("expected story marked seen after notification")
	}
}

func TestStoryPoll_AlreadySeen_NotNotified(t *testing.T) {
	store := newMockStoryStore()
	_ = store.AddStorySubscription(context.Background(), 1, "natgeo")
	_ = store.MarkStorySeen(context.Background(), 1, "natgeo", "old1")

	fetcher := &mockStoryFetcher{
		stories: []domain.StoryItem{{StoryID: "old1", Username: "natgeo"}},
	}
	notifier := &mockStoryNotifier{}

	svc := newStoryWatchSvc(store, fetcher, &mockStoryResolver{}, notifier)
	svc.Poll(context.Background())

	if len(notifier.notified) != 0 {
		t.Errorf("expected 0 notifications, got %d", len(notifier.notified))
	}
}

func TestStoryPoll_MultipleSubscribers_NotifiedSeparately(t *testing.T) {
	store := newMockStoryStore()
	_ = store.AddStorySubscription(context.Background(), 1, "natgeo")
	_ = store.AddStorySubscription(context.Background(), 2, "natgeo")

	fetcher := &mockStoryFetcher{
		stories: []domain.StoryItem{{StoryID: "s1", Username: "natgeo"}},
	}
	notifier := &mockStoryNotifier{}

	svc := newStoryWatchSvc(store, fetcher, &mockStoryResolver{}, notifier)
	svc.Poll(context.Background())

	if len(notifier.notified) != 2 {
		t.Errorf("expected 2 notifications (one per subscriber), got %d", len(notifier.notified))
	}
	// No downloads during polling.
	if len(fetcher.downloaded) != 0 {
		t.Errorf("expected 0 downloads during poll, got %d", len(fetcher.downloaded))
	}
}
