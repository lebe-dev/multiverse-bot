# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
just run          # run the bot locally
just build        # build binary to bin/multiverse-bot
just test         # run all tests
just test ./internal/usecase/...  # run a focused test
just lint         # run golangci-lint
just docker-build # build Docker image
just docker-up    # start with docker compose
just docker-down  # stop docker compose
just docker-logs  # tail bot logs
```

## Configuration (environment variables)

| Variable | Required | Default | Description |
|---|---|---|---|
| `TELEGRAM_BOT_TOKEN` | yes | — | Bot token from BotFather |
| `ALLOWED_USERS` | no | (open) | Comma-separated Telegram usernames; empty = allow everyone |
| `ADMIN_USERS` | no | — | Comma-separated Telegram admin usernames |
| `COBALT_API_URL` | no | `https://api.cobalt.tools` | Cobalt API base URL |
| `YTDLP_PATH` | no | `yt-dlp` | Path to yt-dlp executable |
| `YTDLP_COOKIES_FILE` | no | `./cookies.txt` | Path to cookies file for yt-dlp |
| `LOG_LEVEL` | no | `info` | `debug`, `info`, `warn`, `error` |

Copy `.env-example` to `.env` to get started.

## Architecture

The project follows clean architecture with three layers:

```
internal/
  domain/        # Pure interfaces and types — no dependencies
  usecase/       # Business logic (VideoService) — depends only on domain
  adapter/       # Implementations — depend on domain interfaces
    config/      # Env-var config loader
    detector/    # URL → Platform detection (regex-based)
    downloader/
      ytdlp/     # yt-dlp backend (YouTube)
      cobalt/    # Cobalt API backend (Instagram, Twitter, Threads)
      composite/ # Fan-out: tries each backend in order until one succeeds
    telegram/    # telebot.v4 bot, handlers, middleware
cmd/bot/main.go  # Wires everything together
```

### Key design decisions

- **Domain interfaces** (`domain.Downloader`, `domain.PlatformDetector`) are defined in `internal/domain/` and implemented in adapters. Adapters never import each other.
- **Composite downloader** iterates registered backends in order (yt-dlp first, then Cobalt). To add a new platform, implement `domain.Downloader` and register it in `main.go`.
- **`VideoService.ProcessURL`** returns a cleanup `func()` that must be called after the video is sent — it deletes the temp dir.
- **File size limit** is hardcoded to 50 MB (Telegram bot API limit). `MAX_FILE_SIZE` env var field exists in the struct but is not yet exposed.
- **`ALLOWED_USERS`** middleware short-circuits at the Telegram handler layer; if the list is empty, all users are allowed.
