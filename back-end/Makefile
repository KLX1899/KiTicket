SHELL := /bin/sh

GOCACHE ?= $(CURDIR)/.cache/go-build
GOMODCACHE ?= $(CURDIR)/.cache/go-mod
GO := GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) GOWORK=off go
COMPOSE := docker compose --env-file .env

.PHONY: bootstrap up down migrate seed format lint unit integration test race coverage mutation load-test generate clean build package

bootstrap:
	@test -f .env || cp .env.example .env
	$(COMPOSE) up --build -d

up: bootstrap

down:
	$(COMPOSE) down

migrate:
	$(COMPOSE) run --rm migrate -dir /migrations

seed:
	$(COMPOSE) run --rm migrate -dir /migrations -seed /migrations/002_seed.sql

format:
	@test -z "$$(gofmt -l cmd internal)" || (gofmt -d cmd internal; exit 1)

lint:
	$(GO) vet ./...
	golangci-lint run ./...

unit:
	$(GO) test -short ./...

integration:
	@set -a; . ./.env; set +a; \
	TEST_POSTGRES_URL=postgres://$${POSTGRES_USER}:$${POSTGRES_PASSWORD}@127.0.0.1:$${POSTGRES_HOST_PORT}/$${POSTGRES_DB}?sslmode=disable \
	TEST_REDIS_URL=redis://:$${REDIS_PASSWORD}@127.0.0.1:$${REDIS_HOST_PORT}/0 \
	$(GO) test -count=1 ./...

test: unit

race:
	$(GO) test -race ./internal/reservation/... ./internal/checkout/...

coverage:
	$(GO) test -coverprofile=coverage.out ./internal/reservation/... ./internal/checkout/...
	$(GO) tool cover -func=coverage.out

mutation:
	go run github.com/avito-tech/go-mutesting/cmd/go-mutesting@latest ./internal/reservation/... ./internal/checkout/...

load-test:
	k6 run tests/load/reservation-contention.js

generate:
	docker run --rm -v "$(CURDIR)/docs/diagrams:/data" plantuml/plantuml:1.2025.4 -tsvg /data/*.puml

clean:
	rm -rf .cache bin dist coverage.out coverage.html

build:
	$(GO) build ./cmd/...

package:
	mkdir -p dist
	git archive --format=zip --output=dist/kiticket.zip HEAD
