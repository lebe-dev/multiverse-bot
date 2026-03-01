# Version from source code

app_version := `grep 'const Version = ' cmd/bot/main.go | cut -d'"' -f2`
image := "tinyops/multiverse-bot"

default:
    @just --list

########################################
# LINT & TEST
########################################

lint:
    golangci-lint run ./...

test name="./...":
    go test -v -race {{ name }}

build: lint test
    go build -ldflags="-X main.Version={{ app_version }}" -o bin/multiverse-bot ./cmd/bot

########################################
# DEV
########################################

run:
    go run ./cmd/bot

start-dev-image:
    docker compose -f docker-compose.yml up -d --build --force-recreate

stop-dev-image:
    docker compose down

########################################
# RELEASE
########################################

build-release-image: lint test
    docker build --progress=plain -t {{ image }}:{{ app_version }} .

release: build-release-image
    docker push {{ image }}:{{ app_version }}

deploy:
    ssh kaiman 'cd /opt/multiverse-bot && docker compose pull && docker compose down && docker compose up -d'
