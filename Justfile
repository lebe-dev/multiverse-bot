default:
    @just --list

run:
    go run ./cmd/bot

build:
    go build -o bin/multiverse-bot ./cmd/bot

test name="./...":
    go test -v -race {{ name }}

lint:
    golangci-lint run ./...

docker-build:
    docker compose build

docker-up:
    docker compose up -d

docker-down:
    docker compose down

docker-logs:
    docker compose logs -f bot
