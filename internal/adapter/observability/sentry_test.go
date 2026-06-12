package observability

import "testing"

func TestInitSentry_EmptyDSNDisabled(t *testing.T) {
	flush, err := InitSentry(SentryOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if flush == nil {
		t.Fatal("expected non-nil no-op flush")
	}
	flush() // must not panic
}

func TestInitSentry_InvalidDSN(t *testing.T) {
	flush, err := InitSentry(SentryOptions{DSN: "not-a-valid-dsn"})
	if err == nil {
		t.Fatal("expected error for invalid DSN")
	}
	if flush == nil {
		t.Fatal("expected non-nil flush even on error")
	}
	flush()
}
