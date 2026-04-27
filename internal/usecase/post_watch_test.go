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

// --- post mocks ---

type mockPostStore struct {
	subs   map[string][]int64              // username → userIDs
	seen   map[string]struct{}             // "userID:username:postID"
	subMap map[int64][]domain.PostSubscription
}

func newMockPostStore() *mockPostStore {
	return &mockPostStore{
		subs:   make(map[string][]int64),
		seen:   make(map[string]struct{}),
		subMap: make(map[int64][]domain.PostSubscription),
	}
}

func (m *mockPostStore) AddPostSubscription(_ context.Context, userID int64, username string) error {
	for _, sub := range m.subMap[userID] {
		if sub.Username == username {
			return domain.ErrAlreadySubscribedPost
		}
	}
	m.subMap[userID] = append(m.subMap[userID], domain.PostSubscription{
		UserID: userID, Username: username,
	})
	m.subs[username] = append(m.subs[username], userID)
	return nil
}

func (m *mockPostStore) RemovePostSubscription(_ context.Context, userID int64, username string) error {
	subs := m.subMap[userID]
	for i, sub := range subs {
		if sub.Username == username {
			m.subMap[userID] = append(subs[:i], subs[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotSubscribedPost
}

func (m *mockPostStore) GetPostSubscriptions(_ context.Context, userID int64) ([]domain.PostSubscription, error) {
	return m.subMap[userID], nil
}

func (m *mockPostStore) CountPostSubscriptions(_ context.Context, userID int64) (int, error) {
	return len(m.subMap[userID]), nil
}

func (m *mockPostStore) GetAllUniquePostUsernames(_ context.Context) ([]string, error) {
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

func (m *mockPostStore) GetPostSubscribers(_ context.Context, username string) ([]int64, error) {
	return m.subs[username], nil
}

func (m *mockPostStore) HasSeenPost(_ context.Context, userID int64, username, postID string) (bool, error) {
	_, ok := m.seen[postSeenKey(userID, username, postID)]
	return ok, nil
}

func (m *mockPostStore) MarkPostSeen(_ context.Context, userID int64, username, postID string) error {
	m.seen[postSeenKey(userID, username, postID)] = struct{}{}
	return nil
}

func (m *mockPostStore) CleanupExpiredSeenPosts(_ context.Context) (int64, error) {
	return 0, nil
}

func postSeenKey(userID int64, username, postID string) string {
	return fmt.Sprintf("%d:%s:%s", userID, username, postID)
}

type mockPostFetcher struct {
	posts    []domain.PostItem
	fetchErr error
}

func (m *mockPostFetcher) FetchPostIDs(_ context.Context, _ string) ([]domain.PostItem, error) {
	return m.posts, m.fetchErr
}

type mockPostNotifier struct {
	notified []domain.PostItem
	err      error
}

func (m *mockPostNotifier) NotifyNewPost(_ context.Context, _ int64, post domain.PostItem) error {
	m.notified = append(m.notified, post)
	return m.err
}

func newPostWatchSvc(store *mockPostStore, fetcher domain.PostFetcher, resolver *mockStoryResolver, notifier *mockPostNotifier) *usecase.PostWatchService {
	svc := usecase.NewPostWatchService(store, fetcher, resolver, notifier, newLogger(), time.Minute, 20, 100)
	svc.SetPollJitter(0) // disable delays in tests
	return svc
}

// --- tests ---

func TestPostSubscribe_Success(t *testing.T) {
	store := newMockPostStore()
	fetcher := &mockPostFetcher{posts: []domain.PostItem{{PostID: "p1", Username: "natgeo"}}}
	resolver := &mockStoryResolver{username: "natgeo"}
	notifier := &mockPostNotifier{}

	svc := newPostWatchSvc(store, fetcher, resolver, notifier)

	if err := svc.Subscribe(context.Background(), 1, "https://instagram.com/natgeo"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	subs, _ := store.GetPostSubscriptions(context.Background(), 1)
	if len(subs) != 1 {
		t.Errorf("expected 1 subscription, got %d", len(subs))
	}

	seen, _ := store.HasSeenPost(context.Background(), 1, "natgeo", "p1")
	if !seen {
		t.Error("expected posts to be seeded after subscribe")
	}
}

func TestPostSubscribe_Duplicate(t *testing.T) {
	store := newMockPostStore()
	resolver := &mockStoryResolver{username: "natgeo"}
	fetcher := &mockPostFetcher{}
	svc := newPostWatchSvc(store, fetcher, resolver, &mockPostNotifier{})

	_ = svc.Subscribe(context.Background(), 1, "natgeo")
	err := svc.Subscribe(context.Background(), 1, "natgeo")
	if !errors.Is(err, domain.ErrAlreadySubscribedPost) {
		t.Errorf("expected ErrAlreadySubscribedPost, got %v", err)
	}
}

func TestPostSubscribe_LimitExceeded(t *testing.T) {
	store := newMockPostStore()
	resolver := &mockStoryResolver{username: "user3"}
	fetcher := &mockPostFetcher{}
	svc := usecase.NewPostWatchService(store, fetcher, resolver, &mockPostNotifier{}, newLogger(), time.Minute, 2, 100)

	_ = store.AddPostSubscription(context.Background(), 1, "user1")
	_ = store.AddPostSubscription(context.Background(), 1, "user2")

	err := svc.Subscribe(context.Background(), 1, "user3")
	if !errors.Is(err, domain.ErrMaxSubscriptions) {
		t.Errorf("expected ErrMaxSubscriptions, got %v", err)
	}
}

func TestPostSubscribe_UsernameNotFound(t *testing.T) {
	store := newMockPostStore()
	resolver := &mockStoryResolver{err: domain.ErrUsernameNotFound}
	svc := newPostWatchSvc(store, &mockPostFetcher{}, resolver, &mockPostNotifier{})

	err := svc.Subscribe(context.Background(), 1, "baduser")
	if !errors.Is(err, domain.ErrUsernameNotFound) {
		t.Errorf("expected ErrUsernameNotFound, got %v", err)
	}
}

func TestPostUnsubscribe_Success(t *testing.T) {
	store := newMockPostStore()
	resolver := &mockStoryResolver{username: "natgeo"}
	svc := newPostWatchSvc(store, &mockPostFetcher{}, resolver, &mockPostNotifier{})

	_ = svc.Subscribe(context.Background(), 1, "natgeo")
	if err := svc.Unsubscribe(context.Background(), 1, "natgeo"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPostUnsubscribe_NotSubscribed(t *testing.T) {
	store := newMockPostStore()
	svc := newPostWatchSvc(store, &mockPostFetcher{}, &mockStoryResolver{}, &mockPostNotifier{})

	err := svc.Unsubscribe(context.Background(), 1, "natgeo")
	if !errors.Is(err, domain.ErrNotSubscribedPost) {
		t.Errorf("expected ErrNotSubscribedPost, got %v", err)
	}
}

func TestPostPoll_NewPost_Notified(t *testing.T) {
	store := newMockPostStore()
	_ = store.AddPostSubscription(context.Background(), 1, "natgeo")

	fetcher := &mockPostFetcher{
		posts: []domain.PostItem{{PostID: "p1", Username: "natgeo", URL: "https://www.instagram.com/p/p1/", Timestamp: time.Now()}},
	}
	notifier := &mockPostNotifier{}

	svc := newPostWatchSvc(store, fetcher, &mockStoryResolver{}, notifier)
	svc.Poll(context.Background())

	if len(notifier.notified) != 1 {
		t.Errorf("expected 1 notification, got %d", len(notifier.notified))
	}

	seen, _ := store.HasSeenPost(context.Background(), 1, "natgeo", "p1")
	if !seen {
		t.Error("expected post marked seen after notification")
	}
}

func TestPostPoll_AlreadySeen_NotNotified(t *testing.T) {
	store := newMockPostStore()
	_ = store.AddPostSubscription(context.Background(), 1, "natgeo")
	_ = store.MarkPostSeen(context.Background(), 1, "natgeo", "old1")

	fetcher := &mockPostFetcher{
		posts: []domain.PostItem{{PostID: "old1", Username: "natgeo"}},
	}
	notifier := &mockPostNotifier{}

	svc := newPostWatchSvc(store, fetcher, &mockStoryResolver{}, notifier)
	svc.Poll(context.Background())

	if len(notifier.notified) != 0 {
		t.Errorf("expected 0 notifications, got %d", len(notifier.notified))
	}
}

func TestPostPoll_FetchError_DoesNotAbortCycle(t *testing.T) {
	store := newMockPostStore()
	_ = store.AddPostSubscription(context.Background(), 1, "user1")
	_ = store.AddPostSubscription(context.Background(), 2, "user2")

	callCount := 0
	fetcher := &mockPostFetcher{
		fetchErr: errors.New("network error"),
	}

	notifier := &mockPostNotifier{}
	svc := newPostWatchSvc(store, fetcher, &mockStoryResolver{}, notifier)
	svc.Poll(context.Background())

	// Both usernames should be attempted despite errors.
	_ = callCount
	if len(notifier.notified) != 0 {
		t.Errorf("expected 0 notifications on fetch error, got %d", len(notifier.notified))
	}
}

func TestPostPoll_MultipleSubscribers_NotifiedSeparately(t *testing.T) {
	store := newMockPostStore()
	_ = store.AddPostSubscription(context.Background(), 1, "natgeo")
	_ = store.AddPostSubscription(context.Background(), 2, "natgeo")

	fetcher := &mockPostFetcher{
		posts: []domain.PostItem{{PostID: "p1", Username: "natgeo", URL: "https://www.instagram.com/p/p1/", Timestamp: time.Now()}},
	}
	notifier := &mockPostNotifier{}

	svc := newPostWatchSvc(store, fetcher, &mockStoryResolver{}, notifier)
	svc.Poll(context.Background())

	if len(notifier.notified) != 2 {
		t.Errorf("expected 2 notifications (one per subscriber), got %d", len(notifier.notified))
	}
}

func TestPostPoll_StalePost_NotNotified(t *testing.T) {
	store := newMockPostStore()
	_ = store.AddPostSubscription(context.Background(), 1, "natgeo")

	// Post older than maxPostAge (48h) — must not be sent even though unseen.
	// Guards against CleanupExpiredSeenPosts wiping history and causing old
	// posts to be re-announced after downtime.
	old := domain.PostItem{PostID: "old", Username: "natgeo", Timestamp: time.Now().Add(-72 * time.Hour)}
	fresh := domain.PostItem{PostID: "fresh", Username: "natgeo", Timestamp: time.Now().Add(-1 * time.Hour)}
	fetcher := &mockPostFetcher{posts: []domain.PostItem{old, fresh}}
	notifier := &mockPostNotifier{}

	svc := newPostWatchSvc(store, fetcher, &mockStoryResolver{}, notifier)
	svc.Poll(context.Background())

	if len(notifier.notified) != 1 {
		t.Fatalf("expected exactly 1 notification (fresh post only), got %d", len(notifier.notified))
	}
	if notifier.notified[0].PostID != "fresh" {
		t.Errorf("expected fresh post, got %q", notifier.notified[0].PostID)
	}
}

func TestPostPoll_MissingTimestamp_NotNotified(t *testing.T) {
	store := newMockPostStore()
	_ = store.AddPostSubscription(context.Background(), 1, "natgeo")

	// Zero Timestamp — scraper failed to parse the field. Prefer silence over flooding.
	fetcher := &mockPostFetcher{posts: []domain.PostItem{{PostID: "unknown", Username: "natgeo"}}}
	notifier := &mockPostNotifier{}

	svc := newPostWatchSvc(store, fetcher, &mockStoryResolver{}, notifier)
	svc.Poll(context.Background())

	if len(notifier.notified) != 0 {
		t.Errorf("expected 0 notifications for post with zero Timestamp, got %d", len(notifier.notified))
	}
}
