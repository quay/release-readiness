.DEFAULT_GOAL := help

DOCKER ?= docker
VERSION ?= v2.2
IMAGE ?= release-readiness:latest
ENV_FILE ?= .env
# URL prefix baked into the web bundle; must match the server -base-path.
VITE_BASE ?= /
REVISION := $(shell git rev-parse HEAD)

export RR_ENV_FILE := $(abspath $(ENV_FILE))
COMPOSE = $(DOCKER) compose --env-file "$(ENV_FILE)"

.PHONY: help up down logs ps deps build-web test test-go test-web test-e2e lint build push

help:
	@printf '%s\n' \
	  'make up          Build and start the local app with Docker Compose' \
	  'make down        Stop the local app; preserve the database volume' \
	  'make logs        Follow local app logs' \
	  'make ps          Show local app status' \
	  'make deps        Install frontend dependencies and Playwright Chromium' \
	  'make test        Run Go, web, and browser tests' \
	  'make lint        Run Go and web linters' \
	  'make build       Build the Docker image' \
	  'make push        Push an already-built image; does not build or deploy' \
	  '' \
	  'Overrides: ENV_FILE=deploy/.env VERSION=v2.2 IMAGE=quay.io/your-org/release-readiness:latest'

up:
	$(COMPOSE) up -d --build

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f

ps:
	$(COMPOSE) ps

deps:
	npm --prefix web ci
	cd web && npx playwright install chromium

build-web:
	npm --prefix web run build

test: test-go test-web test-e2e

test-go: build-web
	go test ./...
	go vet ./...

test-web:
	npm --prefix web test
	npm --prefix web run lint

test-e2e:
	npm --prefix web run typecheck:e2e
	npm --prefix web run test:e2e

lint:
	golangci-lint run
	npm --prefix web run lint

build:
	$(DOCKER) build -f deploy/Containerfile --build-arg APP_VERSION="$(VERSION)" \
	  --build-arg VITE_BASE="$(VITE_BASE)" \
	  --label org.opencontainers.image.revision="$(REVISION)" -t "$(IMAGE)" .

push:
	$(DOCKER) push "$(IMAGE)"
