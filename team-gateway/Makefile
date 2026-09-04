# Local dev + merge-gate targets for team-gateway (Connect edge).

ifneq (,$(wildcard ./.env))
include .env
export
endif

.PHONY: help proto run check check-env test

help: ## Show this help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

proto: ## Generate Go + Connect code from the vendored proto module (needs buf)
	buf generate

run: ## Run the gateway
	go run ./cmd/gateway

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
