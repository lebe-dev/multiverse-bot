---
paths:
  - "internal/usecase/**/*.go"
---

# Use-case layer rules

- Depends only on `internal/domain` — never import adapters
- `VideoService.ProcessURL` returns a cleanup `func()` — callers must defer it
- Size enforcement is the handler's responsibility, not the service's
- Use `log/slog` for structured logging (not `log` or `fmt.Println`)
