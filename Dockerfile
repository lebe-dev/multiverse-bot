# ── Stage 1: build Go binary ────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder

ARG TARGETARCH

RUN apk add --no-cache git upx

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN GOOS=linux GOARCH=${TARGETARCH} CGO_ENABLED=0 go build -ldflags="-s -w" -o /bot ./cmd/bot \
    && upx --best --lzma /bot

# ── Stage 2: fetch yt-dlp static binary ─────────────────────────────────────
FROM alpine:3.23.3 AS ytdlp

ARG TARGETARCH

RUN apk add --no-cache wget \
    && case "${TARGETARCH}" in \
         amd64) BIN="yt-dlp_musllinux"          ;; \
         arm64) BIN="yt-dlp_musllinux_aarch64"  ;; \
         *)     echo "Unsupported arch: ${TARGETARCH}" && exit 1 ;; \
       esac \
    && wget -qO /usr/local/bin/yt-dlp \
         "https://github.com/yt-dlp/yt-dlp/releases/latest/download/${BIN}" \
    && chmod +x /usr/local/bin/yt-dlp

# ── Stage 3: minimal runtime ─────────────────────────────────────────────────
FROM alpine:3.23.3

RUN apk add --no-cache ca-certificates ffmpeg nodejs tzdata

ENV TZ=Europe/Moscow

RUN addgroup -g 10001 -S bot \
    && adduser  -u 10001 -S -G bot -H -s /sbin/nologin bot \
    && mkdir -p /data && chown bot:bot /data

COPY --from=builder          /bot                  /bot
COPY --from=ytdlp  /usr/local/bin/yt-dlp  /usr/local/bin/yt-dlp

USER bot

ENTRYPOINT ["/bot"]
