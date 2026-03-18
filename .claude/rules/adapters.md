---
paths:
  - "internal/adapter/**/*.go"
---

# Adapter layer rules

- Adapters implement domain interfaces — import only `internal/domain`, never other adapters
- Each downloader adapter lives in its own sub-package under `adapter/downloader/`
- New downloaders must implement `domain.Downloader` and be registered in `cmd/bot/main.go`
- Composite downloader tries backends in registration order; first success wins
- Use `context.Context` for cancellation and timeouts in all external calls
- HTTP clients should set reasonable timeouts and User-Agent headers
