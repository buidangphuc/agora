# Local dev + merge-gate targets for team-analytics (Kafka -> warehouse worker).
#
# The shared targets (build, run, test, lint, check, check-env, docker, proto)
# come from make/go-service.mk, synced from platform-core (ADR-0016). `make check`
# is the LOCAL MERGE-GATE CONTRACT: it must pass before a PR merges.

SERVICE   := team-analytics
GRPC_PORT := 50059
BINARIES  := consumer ga4-relay ga4import

# Load .env if present (for `make run`, `make ga4-relay`).
ifneq (,$(wildcard ./.env))
include .env
export
endif

include make/go-service.mk

.PHONY: tidy consumer ga4-relay

tidy: ## Resolve deps + go.sum
	go mod tidy

consumer: ## Run the Kafka analytics consumer
	go run ./cmd/consumer

ga4-relay: ## Run the GA4 purchase relay (disabled unless GA4_API_SECRET + GA4_MEASUREMENT_ID are set)
	go run ./cmd/ga4-relay
