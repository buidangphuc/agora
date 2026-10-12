# Shared targets for Go services (ADR-0016). Synced into a service by
# platform-core/tools/sync-go-sdk.sh; do not edit the copy.
#
# The including Makefile sets:
#   SERVICE    image and binary name, e.g. team-sharing   (required)
#   GRPC_PORT  gRPC port, e.g. 50065                      (required)
#   BINARIES   binaries under cmd/, first one is run      (default: server)
#
#   SERVICE := team-sharing
#   GRPC_PORT := 50065
#   include make/go-service.mk

BINARIES ?= server
BIN      := $(firstword $(BINARIES))

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
	  | awk 'BEGIN{FS=":.*?## "}{printf "  %-12s %s\n", $$1, $$2}'

.PHONY: proto
proto: ## Generate gRPC stubs with buf
	buf generate

.PHONY: build
build: ## Build all packages
	go build ./...

.PHONY: run
run: ## Run the first binary in BINARIES
	go run ./cmd/$(BIN)

.PHONY: test
test: ## Run tests with the race detector (DOCKER_HOST from the docker context if unset)
	@h="$${DOCKER_HOST:-$$(docker context inspect -f '{{.Endpoints.docker.Host}}' 2>/dev/null)}"; \
	if [ -n "$$h" ]; then export DOCKER_HOST="$$h"; fi; \
	go test -race ./...

.PHONY: lint
lint: ## Run golangci-lint with .golangci.yml
	go tool golangci-lint run ./...

.PHONY: check-env
check-env: ## Check .env.example matches the config struct
	go test ./internal/config -run TestEnvExampleInSync

.PHONY: check
check: ## vet + lint + test + check-env (Docker tests must run, not skip)
	go vet ./...
	$(MAKE) lint
	GO_SDK_REQUIRE_DOCKER=1 $(MAKE) test
	$(MAKE) check-env

.PHONY: migrate
migrate: ## Apply migrations (the binary must implement `migrate up`)
	go run ./cmd/$(BIN) migrate up

.PHONY: docker
docker: ## Build the service image
	docker build -t $(SERVICE):dev --build-arg BIN=$(BIN) .
