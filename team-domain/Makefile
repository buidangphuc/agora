# Local dev + merge-gate targets for team-domain (Go ListingService).
#
# `make check` is the LOCAL MERGE-GATE CONTRACT: it must pass before a PR merges.
# Keep CI running the exact same target so local == CI.

# Load .env if present (for `make run`, `make migrate`).
ifneq (,$(wildcard ./.env))
include .env
export
endif

DATABASE_URL ?= postgresql://listing_svc:listing_pass@localhost:5433/listing_db

.PHONY: help proto run check check-env test migrate dev stop

help: ## Show this help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-14s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

proto: ## Generate Go code from the vendored proto module into ./generated (needs buf)
	buf generate

run: ## Start the gRPC server locally
	go run ./cmd/server

check: check-env ## Merge gate: env-drift, gofmt, vet, lint (if present), tests
	@echo ">> gofmt (report unformatted files)"
	@unformatted=$$(gofmt -l .); \
	  if [ -n "$$unformatted" ]; then \
	    echo "gofmt needs to run on:"; echo "$$unformatted"; exit 1; \
	  fi
	@echo ">> go vet"
	go vet ./...
	@echo ">> golangci-lint (skipped if not installed)"
	@if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run ./...; else echo "golangci-lint not found; skipping"; fi
	@echo ">> go test"
	go test ./...

check-env: ## Verify .env.example matches the fields declared in internal/config
	go test ./internal/config/ -run TestEnvExampleInSync -count=1

test: ## Run the unit + in-process gRPC tests
	go test ./...

dev: ## Start the local Postgres dependency (docker compose)
	docker compose -f docker-compose.local.yaml up -d postgres-listing

stop: ## Stop the local Postgres dependency
	docker compose -f docker-compose.local.yaml down

migrate: ## Apply DB migrations with golang-migrate (via docker; needs `make dev` up)
	## macOS/Windows: host.docker.internal reaches the host's mapped 5433.
	docker run --rm -v $(PWD)/migrations:/migrations migrate/migrate \
	  -path=/migrations \
	  -database "$(subst localhost,host.docker.internal,$(DATABASE_URL))" up
