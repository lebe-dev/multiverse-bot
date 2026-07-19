# Version from VERSION file, build number from git short commit hash

app_version := `cat VERSION`
app_build := `git rev-parse --short HEAD 2>/dev/null || echo unknown`
image := "tinyops/multiverse-bot"

default:
    @just --list

bump-deps:
    go get -u ./...
    go mod tidy

########################################
# LINT & TEST
########################################

lint:
    golangci-lint run ./...

test name="./...":
    go test -v -race {{ name }}

build: lint test
    go build -ldflags="-X main.Version={{ app_version }} -X main.Build={{ app_build }}" -o bin/multiverse-bot ./cmd/bot

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
    docker build --progress=plain --platform=linux/amd64 \
        --build-arg VERSION={{ app_version }} \
        --build-arg BUILD={{ app_build }} \
        -t {{ image }}:{{ app_version }} -t {{ image }}:latest .

release: build-release-image
    docker push {{ image }}:{{ app_version }}
    docker push {{ image }}:latest

deploy:
    ssh kaiman 'cd /opt/multiverse-bot && docker compose pull && docker compose down && docker compose up -d'
