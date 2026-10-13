# Local dev + merge-gate targets for team-domain (Go ListingService).
#
# The shared targets (build, run, test, lint, check, check-env, docker, proto)
# come from make/go-service.mk, synced from platform-core (ADR-0016). `make check`
# is the LOCAL MERGE-GATE CONTRACT: it must pass before a PR merges.

SERVICE   := team-domain
GRPC_PORT := 50051
BINARIES  := server

# Load .env if present (for `make run`, `make migrate`).
ifneq (,$(wildcard ./.env))
include .env
export
endif

DATABASE_URL ?= postgresql://listing_svc:listing_pass@localhost:5433/listing_db

include make/go-service.mk

.PHONY: dev stop migrate

dev: ## Start the local Postgres dependency (docker compose)
	docker compose -f docker-compose.local.yaml up -d postgres-listing

stop: ## Stop the local Postgres dependency
	docker compose -f docker-compose.local.yaml down

# The service does not migrate at boot: the compose job team-domain-migrate does
# it in the stack, and this target does it for local dev. It replaces the generic
# `migrate` target of go-service.mk.
migrate: ## Apply DB migrations with golang-migrate (via docker; needs `make dev` up)
	## macOS/Windows: host.docker.internal reaches the host's mapped 5433.
	docker run --rm -v $(PWD)/migrations:/migrations migrate/migrate \
	  -path=/migrations \
	  -database "$(subst localhost,host.docker.internal,$(DATABASE_URL))" up
