package observability

import (
	"context"
	"log/slog"
	"sort"

	"github.com/getsentry/sentry-go"
)

// Conventional slog attribute keys the codebase uses. The handler gives some of
// them special meaning when building a Sentry event so issues are searchable and
// readable instead of showing up as a context-less "Unknown error".
const (
	// errorAttrKey carries the underlying error (e.g. log.Error("...", "error", err)).
	errorAttrKey = "error"
	// userAttrKey / userIDAttrKey populate Sentry's User panel.
	userAttrKey   = "user"
	userIDAttrKey = "user_id"
)

// tagAttrKeys lists low-cardinality attributes that are promoted to Sentry tags
// (shown in the issue header and usable for search/filtering) in addition to
// living in the "log" context. URLs are intentionally left out — they are high
// cardinality and stay in the context only.
var tagAttrKeys = map[string]bool{
	"platform": true,
	"plugin":   true,
	"engine":   true,
}

// SlogHandler wraps another slog.Handler and forwards records at error level
// (or above) to Sentry as exception events. All records are still passed to the
// wrapped handler unchanged, so normal logging is unaffected.
type SlogHandler struct {
	next  slog.Handler
	attrs []slog.Attr
}

// NewSlogHandler wraps next so that error-level logs become Sentry events.
func NewSlogHandler(next slog.Handler) *SlogHandler {
	return &SlogHandler{next: next}
}

func (h *SlogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *SlogHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level >= slog.LevelError {
		h.capture(ctx, r)
	}
	return h.next.Handle(ctx, r)
}

func (h *SlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	merged = append(merged, h.attrs...)
	merged = append(merged, attrs...)
	return &SlogHandler{next: h.next.WithAttrs(attrs), attrs: merged}
}

func (h *SlogHandler) WithGroup(name string) slog.Handler {
	return &SlogHandler{next: h.next.WithGroup(name), attrs: h.attrs}
}

// capture turns an error-level log record into a Sentry event. When an "error"
// attribute carrying a Go error is present it is reported via CaptureException
// (preserving the stack trace); otherwise the message is sent as-is.
func (h *SlogHandler) capture(ctx context.Context, r slog.Record) {
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentry.CurrentHub()
	}

	var capturedErr error
	logCtx := sentry.Context{}
	tags := map[string]string{}
	var user sentry.User

	collect := func(a slog.Attr) {
		switch a.Key {
		case errorAttrKey:
			if err, ok := a.Value.Any().(error); ok {
				capturedErr = err
				return
			}
		case userAttrKey:
			user.Username = a.Value.String()
		case userIDAttrKey:
			user.ID = a.Value.String()
		}
		if tagAttrKeys[a.Key] {
			tags[a.Key] = a.Value.String()
		}
		logCtx[a.Key] = a.Value.Any()
	}

	for _, a := range h.attrs {
		collect(a)
	}
	r.Attrs(func(a slog.Attr) bool {
		collect(a)
		return true
	})

	logCtx["message"] = r.Message

	hub.WithScope(func(scope *sentry.Scope) {
		scope.SetLevel(sentry.LevelError)
		scope.SetContext("log", logCtx)
		// Group by the log message plus low-cardinality tags instead of by stack
		// trace. CaptureException records the stack at the point of capture, which
		// for every slog-driven event is this same handler — so the SDK's default
		// grouping collapses unrelated errors ("download failed", "send failed", …)
		// into a single "Unknown error" issue. An explicit fingerprint keeps each
		// failure category as its own searchable issue.
		scope.SetFingerprint(fingerprint(r.Message, tags))
		// The log message becomes the transaction so the event has a non-nil
		// culprit and a readable title (e.g. "download failed") instead of the
		// SDK's default "Unknown error". v0.46 has no scope.SetTransaction, so we
		// set the field on the outgoing event directly.
		scope.AddEventProcessor(func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			event.Transaction = r.Message
			return event
		})
		if len(tags) > 0 {
			scope.SetTags(tags)
		}
		if user.ID != "" || user.Username != "" {
			scope.SetUser(user)
		}

		if capturedErr != nil {
			hub.CaptureException(capturedErr)
			return
		}
		hub.CaptureMessage(r.Message)
	})
}

// fingerprint builds a stable Sentry grouping key from the log message and the
// promoted (low-cardinality) tags. Tags are appended in a deterministic order so
// the same message+context always lands in the same issue.
func fingerprint(message string, tags map[string]string) []string {
	fp := []string{message}
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fp = append(fp, tags[k])
	}
	return fp
}
