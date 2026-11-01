# Versions are pinned by the tool directives in go.mod, so they're covered by
# go.sum and bumpable by Dependabot rather than re-resolved at build time.
GOOSE ?= go tool goose
SQLC  ?= go tool sqlc

-include .env
export

# The daemon's SQLite file. Defaults to a local file; override in .env.
OPENSHIN_DB_PATH ?= ./openshin.db

.PHONY: migrate-create migrate-up migrate-down migrate-status sqlc sqlc-check build test vet run

# make migrate-create name=add_conversation
migrate-create:
	$(GOOSE) -dir ./migrations create $(name) sql

migrate-up:
	$(GOOSE) -dir ./migrations sqlite "$(OPENSHIN_DB_PATH)" up

migrate-down:
	$(GOOSE) -dir ./migrations sqlite "$(OPENSHIN_DB_PATH)" down

migrate-status:
	$(GOOSE) -dir ./migrations sqlite "$(OPENSHIN_DB_PATH)" status

sqlc:
	$(SQLC) generate

# CI gate: fails if the committed generated code is stale relative to the SQL.
sqlc-check:
	$(SQLC) diff

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

run:
	go run ./cmd/openshin
