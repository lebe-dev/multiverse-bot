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

// --- mocks ---

type mockStore struct {
	subs   map[string][]int64  // channelID → userIDs
	seen   map[string]struct{} // "userID:channelID:videoID" → exists
	subMap map[int64][]domain.Subscription
}

func newMockStore() *mockStore {
	return &mockStore{
		subs:   make(map[string][]int64),
		seen:   make(map[string]struct{}),
		subMap: make(map[int64][]domain.Subscription),
	}
}

func (m *mockStore) AddSubscription(_ context.Context, userID int64, channelID, channelName string) error {
	for _, sub := range m.subMap[userID] {
		if sub.ChannelID == channelID {
			return domain.ErrAlreadySubscribed
		}
	}
	m.subMap[userID] = append(m.subMap[userID], domain.Subscription{
		UserID: userID, ChannelID: channelID, ChannelName: channelName,
	})
	m.subs[channelID] = append(m.subs[channelID], userID)
	return nil
}

func (m *mockStore) RemoveSubscription(_ context.Context, userID int64, channelID string) error {
	subs := m.subMap[userID]
	for i, sub := range subs {
		if sub.ChannelID == channelID {
			m.subMap[userID] = append(subs[:i], subs[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotSubscribed
}

func (m *mockStore) GetSubscriptions(_ context.Context, userID int64) ([]domain.Subscription, error) {
	return m.subMap[userID], nil
}

func (m *mockStore) CountSubscriptions(_ context.Context, userID int64) (int, error) {
	return len(m.subMap[userID]), nil
}

func (m *mockStore) GetAllUniqueChannels(_ context.Context) ([]string, error) {
	seen := make(map[string]struct{})
	var result []string
	for _, subs := range m.subMap {
		for _, sub := range subs {
			if _, ok := seen[sub.ChannelID]; !ok {
				seen[sub.ChannelID] = struct{}{}
				result = append(result, sub.ChannelID)
			}
		}
	}
	return result, nil
}

func (m *mockStore) GetSubscribers(_ context.Context, channelID string) ([]int64, error) {
	return m.subs[channelID], nil
}

func (m *mockStore) HasSeenVideo(_ context.Context, userID int64, channelID, videoID string) (bool, error) {
	key := seenKey(userID, channelID, videoID)
	_, ok := m.seen[key]
	return ok, nil
}

func (m *mockStore) MarkVideoSeen(_ context.Context, userID int64, channelID, videoID string) error {
	m.seen[seenKey(userID, channelID, videoID)] = struct{}{}
	return nil
}

func (m *mockStore) CleanupExpiredSeen(_ context.Context) (int64, error) {
	return 0, nil
}

func seenKey(userID int64, channelID, videoID string) string {
	return fmt.Sprintf("%d:%s:%s", userID, channelID, videoID)
}

type mockResolver struct {
	channelID   string
	channelName string
	err         error
}

func (m *mockResolver) Resolve(_ context.Context, _ string) (string, string, error) {
	return m.channelID, m.channelName, m.err
}

type mockFetcher struct {
	videos []domain.FeedVideo
	err    error
}

func (m *mockFetcher) FetchFeed(_ context.Context, _ string) ([]domain.FeedVideo, error) {
	return m.videos, m.err
}

type mockNotifier struct {
	notified []domain.FeedVideo
	err      error
}

func (m *mockNotifier) NotifyNewVideo(_ context.Context, _ int64, video domain.FeedVideo) error {
	m.notified = append(m.notified, video)
	return m.err
}

func newWatchSvc(store *mockStore, fetcher domain.FeedFetcher, resolver *mockResolver, notifier *mockNotifier) *usecase.WatchService {
	return usecase.NewWatchService(store, fetcher, resolver, notifier, newLogger(), time.Minute, 20, 100)
}

// --- tests ---

func TestSubscribe_Success(t *testing.T) {
	store := newMockStore()
	fetcher := &mockFetcher{videos: []domain.FeedVideo{{VideoID: "v1", ChannelID: "UC123"}}}
	resolver := &mockResolver{channelID: "UC123", channelName: "Test Channel"}
	notifier := &mockNotifier{}

	svc := newWatchSvc(store, fetcher, resolver, notifier)

	if err := svc.Subscribe(context.Background(), 1, "UC123"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	subs, _ := store.GetSubscriptions(context.Background(), 1)
	if len(subs) != 1 {
		t.Errorf("expected 1 subscription, got %d", len(subs))
	}

	// Feed seeded: video should be marked seen
	seen, _ := store.HasSeenVideo(context.Background(), 1, "UC123", "v1")
	if !seen {
		t.Error("expected feed to be seeded after subscribe")
	}
}

func TestSubscribe_Duplicate(t *testing.T) {
	store := newMockStore()
	resolver := &mockResolver{channelID: "UC123", channelName: "Test Channel"}
	fetcher := &mockFetcher{}
	svc := newWatchSvc(store, fetcher, resolver, &mockNotifier{})

	_ = svc.Subscribe(context.Background(), 1, "UC123")
	err := svc.Subscribe(context.Background(), 1, "UC123")
	if !errors.Is(err, domain.ErrAlreadySubscribed) {
		t.Errorf("expected ErrAlreadySubscribed, got %v", err)
	}
}

func TestSubscribe_LimitExceeded(t *testing.T) {
	store := newMockStore()
	svc := usecase.NewWatchService(store, &mockFetcher{}, &mockResolver{channelID: "UC1", channelName: "C"}, &mockNotifier{}, newLogger(), time.Minute, 2, 100)

	// Use different channels
	for i, id := range []string{"UCaaaaaaaaaaaaaaaaaaaaa1", "UCaaaaaaaaaaaaaaaaaaaaa2"} {
		resolver := &mockResolver{channelID: id, channelName: "C"}
		svc2 := usecase.NewWatchService(store, &mockFetcher{}, resolver, &mockNotifier{}, newLogger(), time.Minute, 2, 100)
		_ = svc2.Subscribe(context.Background(), int64(100+i), id) // different users to avoid affecting user 1
		_ = store.AddSubscription(context.Background(), 1, id, "C")
	}
	_ = svc // suppress unused warning

	// Now user 1 has 2 subscriptions (at limit)
	resolver := &mockResolver{channelID: "UCaaaaaaaaaaaaaaaaaaaaa3", channelName: "C"}
	svc3 := usecase.NewWatchService(store, &mockFetcher{}, resolver, &mockNotifier{}, newLogger(), time.Minute, 2, 100)

	err := svc3.Subscribe(context.Background(), 1, "UCaaaaaaaaaaaaaaaaaaaaa3")
	if !errors.Is(err, domain.ErrMaxSubscriptions) {
		t.Errorf("expected ErrMaxSubscriptions, got %v", err)
	}
}

func TestSubscribe_ChannelNotFound(t *testing.T) {
	store := newMockStore()
	resolver := &mockResolver{err: domain.ErrChannelNotFound}
	svc := newWatchSvc(store, &mockFetcher{}, resolver, &mockNotifier{})

	err := svc.Subscribe(context.Background(), 1, "badchannel")
	if !errors.Is(err, domain.ErrChannelNotFound) {
		t.Errorf("expected ErrChannelNotFound, got %v", err)
	}
}

func TestUnsubscribe_Success(t *testing.T) {
	store := newMockStore()
	resolver := &mockResolver{channelID: "UC123", channelName: "Test"}
	svc := newWatchSvc(store, &mockFetcher{}, resolver, &mockNotifier{})

	_ = svc.Subscribe(context.Background(), 1, "UC123")
	if err := svc.Unsubscribe(context.Background(), 1, "UC123"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnsubscribe_NotSubscribed(t *testing.T) {
	store := newMockStore()
	svc := newWatchSvc(store, &mockFetcher{}, &mockResolver{}, &mockNotifier{})

	err := svc.Unsubscribe(context.Background(), 1, "UC123")
	if !errors.Is(err, domain.ErrNotSubscribed) {
		t.Errorf("expected ErrNotSubscribed, got %v", err)
	}
}

func TestPoll_NewVideo_Notified(t *testing.T) {
	store := newMockStore()
	_ = store.AddSubscription(context.Background(), 1, "UC123", "Test")

	video := domain.FeedVideo{VideoID: "newvid", ChannelID: "UC123", Title: "New Video", URL: "https://youtube.com/watch?v=newvid", Published: time.Now()}
	fetcher := &mockFetcher{videos: []domain.FeedVideo{video}}
	notifier := &mockNotifier{}

	svc := newWatchSvc(store, fetcher, &mockResolver{}, notifier)
	svc.Poll(context.Background())

	if len(notifier.notified) != 1 {
		t.Errorf("expected 1 notification, got %d", len(notifier.notified))
	}
	if notifier.notified[0].VideoID != "newvid" {
		t.Errorf("expected video 'newvid', got %q", notifier.notified[0].VideoID)
	}

	// Video should now be marked seen
	seen, _ := store.HasSeenVideo(context.Background(), 1, "UC123", "newvid")
	if !seen {
		t.Error("expected video marked seen after notification")
	}
}

func TestPoll_AlreadySeen_NotNotified(t *testing.T) {
	store := newMockStore()
	_ = store.AddSubscription(context.Background(), 1, "UC123", "Test")
	_ = store.MarkVideoSeen(context.Background(), 1, "UC123", "oldvid")

	video := domain.FeedVideo{VideoID: "oldvid", ChannelID: "UC123"}
	fetcher := &mockFetcher{videos: []domain.FeedVideo{video}}
	notifier := &mockNotifier{}

	svc := newWatchSvc(store, fetcher, &mockResolver{}, notifier)
	svc.Poll(context.Background())

	if len(notifier.notified) != 0 {
		t.Errorf("expected 0 notifications, got %d", len(notifier.notified))
	}
}

func TestPoll_FetchError_DoesNotAbortCycle(t *testing.T) {
	store := newMockStore()
	_ = store.AddSubscription(context.Background(), 1, "UC123", "Test")
	_ = store.AddSubscription(context.Background(), 2, "UC456", "Other")

	callCount := 0
	fetcher := &mockFetcherFn{fn: func(channelID string) ([]domain.FeedVideo, error) {
		callCount++
		if channelID == "UC123" {
			return nil, errors.New("network error")
		}
		return []domain.FeedVideo{{VideoID: "v1", ChannelID: "UC456", Published: time.Now()}}, nil
	}}
	notifier := &mockNotifier{}

	svc := newWatchSvc(store, fetcher, &mockResolver{}, notifier)
	svc.Poll(context.Background())

	if callCount < 2 {
		t.Errorf("expected both channels to be polled, got %d calls", callCount)
	}
	if len(notifier.notified) != 1 {
		t.Errorf("expected 1 notification for UC456, got %d", len(notifier.notified))
	}
}

func TestPoll_StaleVideo_NotNotified(t *testing.T) {
	store := newMockStore()
	_ = store.AddSubscription(context.Background(), 1, "UC123", "Test")

	// Video older than maxVideoAge (48h) — must not be sent, even though
	// it is unseen. Guards against CleanupExpiredSeen wiping history and
	// causing old clips to be re-announced after downtime.
	old := domain.FeedVideo{VideoID: "oldvid", ChannelID: "UC123", Published: time.Now().Add(-72 * time.Hour)}
	fresh := domain.FeedVideo{VideoID: "freshvid", ChannelID: "UC123", Published: time.Now().Add(-1 * time.Hour)}
	fetcher := &mockFetcher{videos: []domain.FeedVideo{old, fresh}}
	notifier := &mockNotifier{}

	svc := newWatchSvc(store, fetcher, &mockResolver{}, notifier)
	svc.Poll(context.Background())

	if len(notifier.notified) != 1 {
		t.Fatalf("expected exactly 1 notification (fresh video only), got %d", len(notifier.notified))
	}
	if notifier.notified[0].VideoID != "freshvid" {
		t.Errorf("expected freshvid, got %q", notifier.notified[0].VideoID)
	}
}

func TestPoll_MissingPublished_NotNotified(t *testing.T) {
	store := newMockStore()
	_ = store.AddSubscription(context.Background(), 1, "UC123", "Test")

	// Zero Published — parse failure or malformed feed. Prefer silence over flooding.
	video := domain.FeedVideo{VideoID: "unknown", ChannelID: "UC123"}
	fetcher := &mockFetcher{videos: []domain.FeedVideo{video}}
	notifier := &mockNotifier{}

	svc := newWatchSvc(store, fetcher, &mockResolver{}, notifier)
	svc.Poll(context.Background())

	if len(notifier.notified) != 0 {
		t.Errorf("expected 0 notifications for video with zero Published, got %d", len(notifier.notified))
	}
}

type mockFetcherFn struct {
	fn func(channelID string) ([]domain.FeedVideo, error)
}

func (m *mockFetcherFn) FetchFeed(_ context.Context, channelID string) ([]domain.FeedVideo, error) {
	return m.fn(channelID)
}
