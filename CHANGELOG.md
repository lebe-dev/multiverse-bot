# Changelog

All notable changes to this project will be documented in this file.

## [0.9.0] - 2026-03-24

### Added
- Download images from post URLs — the bot now sends photo albums when a post contains multiple images (using Cobalt picker API, up to 10 items per album)
- Command `watch`: support direct channel links with `@channel-name` format, not just channel IDs

### Fixed
- Fixed YouTube channel ID resolution for channel monitoring

### Chores
- Updated ignore files
- Removed binary from repository
- Reorganized project for documentation

---

## [0.8.0] - 2026-03-18

### Added
- Debug mode for admins (`DEBUG=true`): verbose error details shown in admin chats
- Bot now remembers the admin chat ID across restarts — improved admin notifications

### Improved
- Improved logging throughout the codebase
- Better organization of bot commands
- Refactoring and code cleanup

---

## [0.7.1] - 2026-03-17

### Fixed
- Pass video dimensions to Telegram to preserve correct aspect ratio on playback

---

## [0.7.0] - 2026-03-17

### Added
- Plugin system — extend the bot with external HTTP services via `plugins.yml`. Plugins can register new commands and URL handlers. Built-in commands always take priority.

---

## [0.6.0] - 2026-03-17

### Added
- Per-user settings storage
- Google Drive integration via OAuth Device Flow (per-user auth)
- Video quality selection
- Caption support when sending videos
- Configurable storage file paths via env vars

### Changed
- Removed service account support; Google Drive now uses only per-user OAuth

---

## [0.5.0] - 2026-03-17

### Added
- YouTube channel monitoring (`watch` command) — the bot polls for new videos on a channel and notifies the chat
- `YOUTUBE_ENGINE=savevids` — alternative YouTube download backend via vidssave.com API

---

## [0.4.0] - 2026-03-16

### Added
- `lovethreads` engine for Threads downloads — alternative backend via lovethreads.net proxy (`THREADS_ENGINE=lovethreads`)

[0.9.0]: https://github.com/lebe-dev/multiverse-bot/compare/0.8.0...0.9.0
[0.8.0]: https://github.com/lebe-dev/multiverse-bot/compare/0.7.1...0.8.0
[0.7.1]: https://github.com/lebe-dev/multiverse-bot/compare/0.7.0...0.7.1
[0.7.0]: https://github.com/lebe-dev/multiverse-bot/compare/0.6.0...0.7.0
[0.6.0]: https://github.com/lebe-dev/multiverse-bot/compare/0.5.0...0.6.0
[0.5.0]: https://github.com/lebe-dev/multiverse-bot/compare/0.4.0...0.5.0
[0.4.0]: https://github.com/lebe-dev/multiverse-bot/releases/tag/0.4.0
