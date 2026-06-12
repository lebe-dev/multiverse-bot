package observability

import (
	"context"
	"log/slog"

	"github.com/getsentry/sentry-go"
)

// errorAttrKey is the conventional slog attribute key the codebase uses to
// attach the underlying error (e.g. log.Error("...", "error", err)).
const errorAttrKey = "error"

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

	for _, a := range h.attrs {
		if err, ok := asError(a); ok {
			capturedErr = err
			continue
		}
		logCtx[a.Key] = a.Value.Any()
	}
	r.Attrs(func(a slog.Attr) bool {
		if err, ok := asError(a); ok {
			capturedErr = err
			return true
		}
		logCtx[a.Key] = a.Value.Any()
		return true
	})

	logCtx["message"] = r.Message

	hub.WithScope(func(scope *sentry.Scope) {
		scope.SetLevel(sentry.LevelError)
		scope.SetContext("log", logCtx)

		if capturedErr != nil {
			hub.CaptureException(capturedErr)
			return
		}
		hub.CaptureMessage(r.Message)
	})
}

func asError(a slog.Attr) (error, bool) {
	if a.Key != errorAttrKey {
		return nil, false
	}
	err, ok := a.Value.Any().(error)
	return err, ok
}
