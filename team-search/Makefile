# Local dev + merge-gate targets for team-search (Go SearchService + indexer).
#
# The shared targets (build, run, test, lint, check, check-env, docker, proto)
# come from make/go-service.mk, synced from platform-core (ADR-0016). `make check`
# is the LOCAL MERGE-GATE CONTRACT: it must pass before a PR merges.

SERVICE   := team-search
GRPC_PORT := 50052
BINARIES  := server indexer

# Load .env if present (for `make run`, `make indexer`).
ifneq (,$(wildcard ./.env))
include .env
export
endif

include make/go-service.mk

.PHONY: server indexer

server: ## Run the gRPC search query API
	go run ./cmd/server

indexer: ## Run the Kafka indexer
	go run ./cmd/indexer
