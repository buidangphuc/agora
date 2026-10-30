# [W4-T10] gateway-go-sdk-v1

## Role
feature-run worker (change `openspec/changes/gateway-go-sdk-v1`)

## Objective
Implement OpenSpec change `gateway-go-sdk-v1` test-first per its tasks.md: migrate team-gateway to go-sdk v1.0.2 (shared make/lint, `config.Load`, `jwtx` types in token, producer on `kafkax`, revocation consumer with a caught-up latch, `auth`/`reqctx`/`idempotency` metadata in forward.go, read retry on `resilience.Do` with no breaker, bounded track publish, `app.Run` on `lifecycle` with gRPC health over Connect), no dependency enable flags, no runtime in-memory fallback, FEATURES.yaml entries and the e2e edits of tasks 2.1-2.2; context design.md. Compose and gitops edits belong to deploy-go-sdk-v1, not here.

## Write-set (EXCLUSIVE)
- team-gateway/**
- openspec/changes/gateway-go-sdk-v1/**
- platform-e2e/tests/e2e/features/ops/boot_guard_gateway.feature
- platform-e2e/tests/e2e/step_definitions/pear_edge_boot_steps.py
- platform-e2e/tests/e2e/support/pear_edge_support.py
- platform-e2e/tests/e2e/step_definitions/pear_edge_stream_steps.py
- platform-e2e/tests/e2e/features/security/edge_ai_and_reflection.feature

## Verify
- cd team-gateway && make proto && make check && go build ./... && GO_SDK_REQUIRE_DOCKER=1 go test -race ./... -count=1
- docker build -t team-gateway:go-sdk-check team-gateway
- openspec validate gateway-go-sdk-v1 --strict
- make -C platform-e2e features-check
