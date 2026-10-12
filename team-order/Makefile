# Local dev + merge-gate targets for team-order (Go CartService/OrderService).
#
# The shared targets (build, run, test, lint, check, check-env, docker, proto)
# come from make/go-service.mk, synced from platform-core (ADR-0016). `make check`
# is the LOCAL MERGE-GATE CONTRACT: it must pass before a PR merges.

SERVICE   := team-order
GRPC_PORT := 50055
BINARIES  := server

include make/go-service.mk
