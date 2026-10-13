# Local dev + merge-gate targets for team-payment (Go PaymentService).
#
# The shared targets (build, run, test, lint, check, check-env, docker, proto) come from
# make/go-service.mk, synced from platform-core (ADR-0016). `make check` is the LOCAL
# MERGE-GATE CONTRACT: it must pass before a PR merges.

SERVICE   := team-payment
GRPC_PORT := 50056
BINARIES  := server

# Load .env if present (for `make run`).
ifneq (,$(wildcard ./.env))
include .env
export
endif

include make/go-service.mk
