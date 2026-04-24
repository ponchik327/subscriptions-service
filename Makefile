BINARY      := bin/app
MAIN        := ./cmd/app
SWAG_OUT    := docs
GOPATH_BIN  := $(shell go env GOPATH)/bin

# Load local overrides (.env.local) if present — takes priority over defaults below.
-include .env.local

# Local-dev defaults (used by run / migrate-* when no .env.local is present).
POSTGRES_DSN             ?= postgres://app:app@localhost:5432/subscriptions?sslmode=disable
POSTGRES_MIGRATIONS_PATH ?= file://migrations

.PHONY: build run dev lint swag mocks \
        test test-integration test-e2e test-all \
        migrate-up migrate-down \
        infra-up infra-down \
        docker-up docker-down \
        help dev-setup fmt tidy vuln

# ── Build ─────────────────────────────────────────────────────────────────────

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BINARY) $(MAIN)

dev: infra-up run ## Start DB (if not running) and run the app locally

run: build
	POSTGRES_DSN="$(POSTGRES_DSN)" POSTGRES_MIGRATIONS_PATH="$(POSTGRES_MIGRATIONS_PATH)" ./$(BINARY)

lint:
	golangci-lint run ./...

# ── Swagger ───────────────────────────────────────────────────────────────────

swag:
	$(GOPATH_BIN)/swag init -g $(MAIN)/main.go -o $(SWAG_OUT)

# ── Mocks ─────────────────────────────────────────────────────────────────────

mocks:
	mockery

# ── Tests ─────────────────────────────────────────────────────────────────────

test:
	go test ./internal/domain/... ./internal/handler/... ./internal/service/...

test-integration:
	go test -tags=integration -count=1 -timeout=120s ./internal/repository/...

test-e2e:
	go test -tags=e2e -count=1 -timeout=180s ./test/e2e/...

test-all: test test-integration test-e2e

# ── Migrations (local dev) ────────────────────────────────────────────────────

migrate-up:
	migrate -path migrations -database "$(POSTGRES_DSN)" up

migrate-down:
	migrate -path migrations -database "$(POSTGRES_DSN)" down

# ── Infrastructure (local dev — DB only, no app) ──────────────────────────────

infra-up:
	docker compose up postgres -d --wait

infra-down:
	docker compose down

# ── Docker (full stack) ───────────────────────────────────────────────────────

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down

# ── Help ──────────────────────────────────────────────────────────────────────

help: ## Show available make targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
	  awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ── Dev setup ─────────────────────────────────────────────────────────────────

dev-setup: ## Install dev tools (golangci-lint, gofumpt, goimports, govulncheck, lefthook) and activate git hooks
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	go install mvdan.cc/gofumpt@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install github.com/evilmartians/lefthook@latest
	lefthook install

# ── Format ────────────────────────────────────────────────────────────────────

fmt: ## Format code with gofumpt and goimports
	gofumpt -l -w .
	goimports -local github.com/ponchik327/subscriptions-service -w .

# ── Module ────────────────────────────────────────────────────────────────────

tidy: ## Tidy go modules
	go mod tidy

# ── Security ──────────────────────────────────────────────────────────────────

vuln: ## Run govulncheck
	govulncheck ./...
