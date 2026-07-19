package observability

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
)

// mockTransport captures events instead of sending them over the network.
type mockTransport struct {
	mu     sync.Mutex
	events []*sentry.Event
}

func (m *mockTransport) Configure(sentry.ClientOptions)    {}
func (m *mockTransport) Flush(time.Duration) bool          { return true }
func (m *mockTransport) FlushWithContext(context.Context) bool { return true }
func (m *mockTransport) Close()                            {}

func (m *mockTransport) SendEvent(e *sentry.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, e)
}

func (m *mockTransport) captured() []*sentry.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.events
}

// newTestLogger builds a logger whose error records are forwarded to a Sentry
// hub backed by a mock transport, and returns both for assertions.
func newTestLogger(t *testing.T) (*slog.Logger, context.Context, *mockTransport) {
	t.Helper()

	transport := &mockTransport{}
	client, err := sentry.NewClient(sentry.ClientOptions{
		Dsn:       "https://test@example.com/1",
		Transport: transport,
	})
	if err != nil {
		t.Fatalf("new sentry client: %v", err)
	}

	hub := sentry.NewHub(client, sentry.NewScope())
	ctx := sentry.SetHubOnContext(context.Background(), hub)

	base := slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(NewSlogHandler(base)), ctx, transport
}

func TestSlogHandler_CaptureException(t *testing.T) {
	log, ctx, transport := newTestLogger(t)

	sentinel := errors.New("boom")
	log.ErrorContext(ctx, "download failed",
		"error", sentinel,
		"url", "https://x",
		"platform", "youtube",
		"user", "alice",
		"user_id", int64(42),
	)

	events := transport.captured()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	e := events[0]
	if e.Level != sentry.LevelError {
		t.Errorf("expected error level, got %v", e.Level)
	}
	if len(e.Exception) == 0 || e.Exception[len(e.Exception)-1].Value != "boom" {
		t.Errorf("expected exception with value 'boom', got %+v", e.Exception)
	}
	if logCtx, ok := e.Contexts["log"]; !ok || logCtx["url"] != "https://x" {
		t.Errorf("expected log context with url attr, got %+v", e.Contexts["log"])
	}
	if e.Transaction != "download failed" {
		t.Errorf("expected transaction set to message, got %q", e.Transaction)
	}
	if e.Tags["platform"] != "youtube" {
		t.Errorf("expected platform tag, got %+v", e.Tags)
	}
	if _, isTag := e.Tags["url"]; isTag {
		t.Errorf("url must stay out of tags (high cardinality), got %+v", e.Tags)
	}
	if e.User.Username != "alice" || e.User.ID != "42" {
		t.Errorf("expected user populated, got %+v", e.User)
	}
	wantFP := []string{"download failed", "youtube"}
	if !reflect.DeepEqual(e.Fingerprint, wantFP) {
		t.Errorf("expected fingerprint %v, got %v", wantFP, e.Fingerprint)
	}
}

func TestSlogHandler_FingerprintSeparatesCategories(t *testing.T) {
	log, ctx, transport := newTestLogger(t)

	// Same stack trace (both go through this handler), different log messages and
	// platforms: each must get its own fingerprint instead of collapsing into one
	// "Unknown error" issue.
	log.ErrorContext(ctx, "download failed", "error", errors.New("boom"), "platform", "youtube")
	log.ErrorContext(ctx, "send failed", "error", errors.New("boom"), "platform", "instagram")

	events := transport.captured()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	fp0, fp1 := events[0].Fingerprint, events[1].Fingerprint
	if reflect.DeepEqual(fp0, fp1) {
		t.Fatalf("expected distinct fingerprints, both were %v", fp0)
	}
	if want := []string{"download failed", "youtube"}; !reflect.DeepEqual(fp0, want) {
		t.Errorf("event 0: expected fingerprint %v, got %v", want, fp0)
	}
	if want := []string{"send failed", "instagram"}; !reflect.DeepEqual(fp1, want) {
		t.Errorf("event 1: expected fingerprint %v, got %v", want, fp1)
	}
}

func TestSlogHandler_CaptureMessageWithoutError(t *testing.T) {
	log, ctx, transport := newTestLogger(t)

	log.ErrorContext(ctx, "something odd happened", "count", 3)

	events := transport.captured()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Message != "something odd happened" {
		t.Errorf("expected message event, got %q", events[0].Message)
	}
}

func TestSlogHandler_IgnoresBelowError(t *testing.T) {
	log, ctx, transport := newTestLogger(t)

	log.InfoContext(ctx, "all good")
	log.WarnContext(ctx, "be careful")

	if n := len(transport.captured()); n != 0 {
		t.Fatalf("expected no events for info/warn, got %d", n)
	}
}

func TestSlogHandler_WithAttrs(t *testing.T) {
	log, ctx, transport := newTestLogger(t)

	log.With("component", "watcher").ErrorContext(ctx, "tick failed", "error", errors.New("nope"))

	events := transport.captured()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if logCtx := events[0].Contexts["log"]; logCtx["component"] != "watcher" {
		t.Errorf("expected component attr from With(), got %+v", logCtx)
	}
}
