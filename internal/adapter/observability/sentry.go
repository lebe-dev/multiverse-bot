// Package observability wires the bot's error reporting to Sentry.
//
// Scope is intentionally narrow: only error events with stack traces are sent.
// Performance tracing, profiling, and other advanced Sentry features stay off.
package observability

import (
	"time"

	"github.com/getsentry/sentry-go"
)

// flushTimeout bounds how long Flush waits for buffered events on shutdown.
const flushTimeout = 2 * time.Second

// SentryOptions configures the Sentry client. Tracing is never enabled.
type SentryOptions struct {
	DSN         string
	Environment string
	Release     string
}

// InitSentry initialises the global Sentry hub for error reporting only.
//
// It returns a flush function that should be deferred in main to drain
// buffered events before exit. When DSN is empty Sentry stays disabled and the
// returned flush is a no-op.
func InitSentry(opts SentryOptions) (flush func(), err error) {
	if opts.DSN == "" {
		return func() {}, nil
	}

	if err := sentry.Init(sentry.ClientOptions{
		Dsn:           opts.DSN,
		Environment:   opts.Environment,
		Release:       opts.Release,
		EnableTracing: false,
	}); err != nil {
		return func() {}, err
	}

	return func() { sentry.Flush(flushTimeout) }, nil
}

// RecoverAndReport captures a panic to Sentry and re-panics so the process
// crashes exactly as it would without Sentry. Use as `defer RecoverAndReport()`
// at the top of goroutines.
func RecoverAndReport() {
	r := recover()
	if r == nil {
		return
	}
	sentry.CurrentHub().Recover(r)
	sentry.Flush(flushTimeout)
	panic(r)
}
