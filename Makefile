# Plumber — Makefile. See CLAUDE.md §7 for the contract these targets satisfy.
# CI blocks (not suggests) on: test -race, lint, gofmt, arch-test, diff-coverage.

GO         ?= go
BINARY     := plumber
PKG        := ./...
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS    := -ldflags "-X main.version=$(VERSION)"
COVERPROF  := coverage.out

.DEFAULT_GOAL := help

## help: list targets
.PHONY: help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | awk -F': ' '{printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

## build: compile the plumber binary into ./bin
.PHONY: build
build:
	$(GO) build $(LDFLAGS) -o bin/$(BINARY) ./cmd/plumber

## run: build and run (pass ARGS="target list")
.PHONY: run
run:
	$(GO) run $(LDFLAGS) ./cmd/plumber $(ARGS)

## test: unit tests, race detector on — the default gate (CLAUDE §4/§6)
.PHONY: test
test:
	$(GO) test -race $(PKG)

## cover: unit tests with a coverage profile (floor is on NEW/changed code)
.PHONY: cover
cover:
	$(GO) test -race -covermode=atomic -coverprofile=$(COVERPROF) $(PKG)
	$(GO) tool cover -func=$(COVERPROF) | tail -1

## int: integration tests (real Postgres via testcontainers-go); needs Docker
.PHONY: int
int:
	$(GO) test -race -tags=integration $(PKG)

## arch: belt-and-braces import-boundary test (domain/verdict stay pure)
# -count=1 is mandatory: the test reads the import graph via `go list` (external
# state the build cache can't see), so a cached pass could hide a real violation.
.PHONY: arch
arch:
	$(GO) test -count=1 -run TestArch -tags=archtest ./internal/arch/

## lint: golangci-lint (depguard enforces the dependency rule — see .golangci.yml)
.PHONY: lint
lint:
	golangci-lint run

## fmt: gofmt + goimports are not opinions (CLAUDE §4)
.PHONY: fmt
fmt:
	gofmt -w -s .
	@command -v goimports >/dev/null 2>&1 && goimports -w -local plumber . || true

## fmt-check: fail if anything is unformatted (CI gate)
.PHONY: fmt-check
fmt-check:
	@test -z "$$(gofmt -l -s .)" || { echo "unformatted files:"; gofmt -l -s .; exit 1; }

## tidy: go mod tidy
.PHONY: tidy
tidy:
	$(GO) mod tidy

## migrate: apply numbered SQL migrations (needs PLUMBER_DATABASE_URL)
.PHONY: migrate
migrate:
	$(GO) run $(LDFLAGS) ./cmd/plumber migrate

## dev: one-step dev env — compose Postgres up, migrate, build (PLAN §SD)
# Prereq failures are one-line and actionable, not compose stack traces.
# Idempotent: re-running on an up env is a fast no-op (compose --wait +
# migration runner both converge).
.PHONY: dev
dev:
	@command -v $(GO) >/dev/null 2>&1 || { echo "make dev: go not found — install Go 1.22+ (https://go.dev/dl)"; exit 1; }
	@command -v docker >/dev/null 2>&1 || { echo "make dev: docker not found — install Docker Desktop"; exit 1; }
	@docker info >/dev/null 2>&1 || { echo "make dev: Docker daemon not running — start Docker"; exit 1; }
	@test -f .env || { cp .env.example .env && echo "make dev: created .env from .env.example"; }
	@docker compose up -d --wait || { echo "make dev: postgres failed to come up — try 'docker compose logs postgres'"; exit 1; }
	@set -a; . ./.env; set +a; $(GO) run $(LDFLAGS) ./cmd/plumber migrate
	@$(MAKE) --no-print-directory build
	@echo ""
	@echo "dev env ready. Next:"
	@echo "  set -a; . ./.env; set +a                                # export the dev DSN"
	@echo "  bin/plumber target add sluice --url http://localhost:8080 --allowlisted"
	@echo "  bin/plumber target list"

## dev-down: stop the dev Postgres, keep its data volume
.PHONY: dev-down
dev-down:
	docker compose down

## dev-nuke: stop the dev Postgres AND drop its data volume (destructive)
.PHONY: dev-nuke
dev-nuke:
	docker compose down -v

## tools: install dev tooling not bundled with the Go toolchain
.PHONY: tools
tools:
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	$(GO) install golang.org/x/tools/cmd/goimports@latest

## ci: everything CI blocks on
.PHONY: ci
ci: fmt-check lint arch test

## clean: remove build/coverage artifacts
.PHONY: clean
clean:
	rm -rf bin $(COVERPROF)
