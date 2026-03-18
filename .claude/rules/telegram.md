---
paths:
  - "internal/adapter/telegram/**/*.go"
---

# Telegram handler rules

- Bot uses `gopkg.in/telebot.v4` (`tele` alias)
- UI text is in Russian — keep user-facing messages in Russian
- Admin-only commands must check `b.IsAdmin(c.Sender().Username)` first
- Always clean up status messages with `b.deleteMsg(statusMsg)` after operation completes
- Use `context.WithTimeout` for all download/upload operations
- Callback data format: settings use `\f{action}|{payload}`, plugins use `p_{name}|{id}`, watch uses `dl:` / `watch_rm:` prefixes
- HTML parse mode: escape user content with `escapeHTML()`
