---
paths:
  - "internal/domain/**/*.go"
---

# Domain layer rules

- Domain package contains only interfaces, types, and sentinel errors — zero external dependencies
- Never import adapter or usecase packages from domain
- Platform enum uses `iota`; always add new values before the closing parenthesis
- Interfaces must stay minimal: one method per concern where possible (e.g. `Downloader.Download`, `Downloader.Supports`)
