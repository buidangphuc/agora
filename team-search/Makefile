# Local dev + merge-gate targets for team-search (Go SearchService + indexer).

ifneq (,$(wildcard ./.env))
include .env
export
endif

.PHONY: help proto server indexer check check-env test

help: ## Show this help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

proto: ## Generate Go code from the vendored proto module (needs buf)
	buf generate

server: ## Run the gRPC search query API
	go run ./cmd/server

indexer: ## Run the Kafka indexer (needs KAFKA_ENABLED=true)
	go run ./cmd/indexer

check: check-env ## Merge gate: env-drift, gofmt, vet, tests
	@echo ">> gofmt (report unformatted files)"
	@unformatted=$$(gofmt -l .); \
	  if [ -n "$$unformatted" ]; then echo "gofmt needs to run on:"; echo "$$unformatted"; exit 1; fi
	@echo ">> go vet"
	go vet ./...
	@echo ">> go test"
	go test ./...

check-env: ## Verify .env.example matches internal/config fields
	go test ./internal/config/ -run TestEnvExampleInSync -count=1

test: ## Run all tests
	go test ./...
