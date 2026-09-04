# Local dev + merge-gate targets for this service.
#
# `make check` is the LOCAL MERGE-GATE CONTRACT: it must pass before a PR merges.
# Keep CI running the exact same target so local == CI.

# Load .env if present (for `make run`, `make migrate`).
ifneq (,$(wildcard ./.env))
include .env
export
endif

GRPC_PORT ?= 50051
DATABASE_URL ?= postgresql://listing_svc:listing_pass@localhost:5433/listing_db

.PHONY: proto run check test migrate

## proto: generate Go code from the vendored proto module into ./generated.
## Requires the proto submodule (see proto-vendor/README.md) and `buf`.
proto:
	buf generate

## run: start the gRPC server locally.
run:
	go run ./cmd/server

## check: the merge gate — formatting, vet, lint (if available), and tests.
check:
	@echo ">> gofmt (report unformatted files)"
	@unformatted=$$(gofmt -l .); \
	  if [ -n "$$unformatted" ]; then \
	    echo "gofmt needs to run on:"; echo "$$unformatted"; exit 1; \
	  fi
	@echo ">> go vet"
	go vet ./...
	@echo ">> golangci-lint (skipped if not installed)"
	@if command -v golangci-lint >/dev/null 2>&1; then \
	    golangci-lint run ./...; \
	  else \
	    echo "golangci-lint not found; skipping"; \
	  fi
	@echo ">> go test"
	go test ./...

## test: run the unit tests only.
test:
	go test ./...

## migrate: apply DB migrations with golang-migrate (optional; uncomment to use).
## Install: https://github.com/golang-migrate/migrate
## migrate -path ./migrations -database "$(DATABASE_URL)" up
migrate:
	@echo "migrate is optional — install golang-migrate and uncomment the recipe."
	@echo "e.g.: migrate -path ./migrations -database \"$(DATABASE_URL)\" up"
