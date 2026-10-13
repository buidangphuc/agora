# Local dev + merge-gate targets for team-identity (Go AuthService).
#
# The shared targets (build, run, test, lint, check, check-env, docker, proto)
# come from make/go-service.mk, synced from platform-core (ADR-0016). `make check`
# is the LOCAL MERGE-GATE CONTRACT: it must pass before a PR merges.

SERVICE   := team-identity
GRPC_PORT := 50053
BINARIES  := server

# Load .env if present (for `make run`, `make migrate`).
ifneq (,$(wildcard ./.env))
include .env
export
endif

DATABASE_URL ?= postgresql://identity_svc:identity_pass@localhost:5435/identity_db

include make/go-service.mk

.PHONY: migrate

# The service does not migrate at boot: the compose job team-identity-migrate
# does it in the stack, and this target does it for local dev. It replaces the
# generic `migrate` target of go-service.mk.
migrate: ## Apply DB migrations with golang-migrate (via docker)
	docker run --rm -v $(PWD)/migrations:/migrations migrate/migrate \
	  -path=/migrations -database "$(subst localhost,host.docker.internal,$(DATABASE_URL))" up
