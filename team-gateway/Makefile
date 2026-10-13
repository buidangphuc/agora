# Local dev + merge-gate targets for team-gateway (Connect edge).
#
# The shared targets (build, run, test, lint, check, check-env, docker, proto)
# come from make/go-service.mk, synced from platform-core (ADR-0016). `make check`
# is the LOCAL MERGE-GATE CONTRACT: it must pass before a PR merges.
#
# The gateway serves Connect over HTTP (HTTP_PORT, default 8080), not gRPC, so
# GRPC_PORT is not set; the binary is cmd/gateway.

SERVICE  := team-gateway
BINARIES := gateway

# Load .env if present (for `make run`).
ifneq (,$(wildcard ./.env))
include .env
export
endif

include make/go-service.mk

.PHONY: proto-local

# `make proto` (go-service.mk) runs `buf generate` with buf.gen.yaml, which uses
# the remote buf.build plugins (messages, gRPC stubs, Connect handlers). When
# buf.build rate-limits, use the locally installed plugins of buf.gen.local.yaml.
proto-local: ## Generate Go + gRPC + Connect code with local protoc plugins (offline)
	buf generate --template buf.gen.local.yaml
