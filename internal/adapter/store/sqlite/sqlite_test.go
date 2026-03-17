package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"gitlab.com/tiny-services/multiverse-bot/internal/adapter/store/sqlite"
	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

func newTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	store, err := sqlite.New(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestAddSubscription(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	err := store.AddSubscription(ctx, 1, "UC123", "Test Channel")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Duplicate should fail
	err = store.AddSubscription(ctx, 1, "UC123", "Test Channel")
	if !errors.Is(err, domain.ErrAlreadySubscribed) {
		t.Errorf("expected ErrAlreadySubscribed, got %v", err)
	}
}

func TestRemoveSubscription(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	_ = store.AddSubscription(ctx, 1, "UC123", "Test Channel")

	err := store.RemoveSubscription(ctx, 1, "UC123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Remove non-existent should fail
	err = store.RemoveSubscription(ctx, 1, "UC123")
	if !errors.Is(err, domain.ErrNotSubscribed) {
		t.Errorf("expected ErrNotSubscribed, got %v", err)
	}
}

func TestRemoveSubscription_CleansUpSeenVideos(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	_ = store.AddSubscription(ctx, 1, "UC123", "Test Channel")
	_ = store.MarkVideoSeen(ctx, 1, "UC123", "vid1")

	seen, _ := store.HasSeenVideo(ctx, 1, "UC123", "vid1")
	if !seen {
		t.Fatal("expected video to be seen before unsubscribe")
	}

	_ = store.RemoveSubscription(ctx, 1, "UC123")

	seen, _ = store.HasSeenVideo(ctx, 1, "UC123", "vid1")
	if seen {
		t.Error("expected seen_videos to be cleaned up after last subscriber removed")
	}
}

func TestGetSubscriptions(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	_ = store.AddSubscription(ctx, 1, "UC123", "Channel A")
	_ = store.AddSubscription(ctx, 1, "UC456", "Channel B")

	subs, err := store.GetSubscriptions(ctx, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(subs) != 2 {
		t.Errorf("expected 2 subscriptions, got %d", len(subs))
	}
}

func TestCountSubscriptions(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	count, _ := store.CountSubscriptions(ctx, 1)
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}

	_ = store.AddSubscription(ctx, 1, "UC123", "Channel A")
	_ = store.AddSubscription(ctx, 1, "UC456", "Channel B")

	count, _ = store.CountSubscriptions(ctx, 1)
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}

func TestGetAllUniqueChannels(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	_ = store.AddSubscription(ctx, 1, "UC123", "Channel A")
	_ = store.AddSubscription(ctx, 2, "UC123", "Channel A") // same channel, different user
	_ = store.AddSubscription(ctx, 1, "UC456", "Channel B")

	channels, err := store.GetAllUniqueChannels(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(channels) != 2 {
		t.Errorf("expected 2 unique channels, got %d", len(channels))
	}
}

func TestGetSubscribers(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	_ = store.AddSubscription(ctx, 1, "UC123", "Channel A")
	_ = store.AddSubscription(ctx, 2, "UC123", "Channel A")
	_ = store.AddSubscription(ctx, 3, "UC456", "Channel B")

	subs, err := store.GetSubscribers(ctx, "UC123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(subs) != 2 {
		t.Errorf("expected 2 subscribers for UC123, got %d", len(subs))
	}
}

func TestSeenVideos(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	seen, err := store.HasSeenVideo(ctx, 1, "UC123", "vid1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seen {
		t.Error("expected video to be unseen initially")
	}

	if err := store.MarkVideoSeen(ctx, 1, "UC123", "vid1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	seen, err = store.HasSeenVideo(ctx, 1, "UC123", "vid1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !seen {
		t.Error("expected video to be seen after marking")
	}

	// Idempotent
	if err := store.MarkVideoSeen(ctx, 1, "UC123", "vid1"); err != nil {
		t.Errorf("second MarkVideoSeen should be idempotent: %v", err)
	}
}

func TestCleanupExpiredSeen(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	_ = store.MarkVideoSeen(ctx, 1, "UC123", "vid1")

	// Nothing expired yet
	deleted, err := store.CleanupExpiredSeen(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted, got %d", deleted)
	}
}
