# [W4-T13] order-go-sdk-v1

## Role
feature-run worker (change `openspec/changes/order-go-sdk-v1`)

## Objective
Implement OpenSpec change `order-go-sdk-v1` test-first per its tasks.md sections 1 and 2: migrate team-order (server and resync-commits binaries) to go-sdk v1.0.2, ADR-0016 layout (internal/app, internal/config, internal/rpc, internal/order, internal/store, internal/events, internal/upstream), shared make/lint, outbox upgrade migrations 0011-0022 with `migrate up`, consumers on `consume`, audit gate never nil, required upstream addresses, no dependency enable flags, no runtime in-memory fallback, FEATURES.yaml edits of task 2.1; context design.md Clarifications. Section 3 (compose, gitops and platform-e2e edits) belongs to deploy-go-sdk-v1, not here.

## Write-set (EXCLUSIVE)
- team-order/**
- openspec/changes/order-go-sdk-v1/**
- team-order/internal/order/order.go (widened at build: task 1.8)
- team-order/internal/order/idempotency.go (widened at build: task 1.8)
- team-order/internal/order/saga.go (widened at build: task 1.8)
- team-order/internal/order/flashprice.go (widened at build: task 1.8)
- team-order/internal/order/lifecycle.go (widened at build: task 1.8)
- team-order/internal/rpc/order.go (widened at build: task 1.8)

## Verify
- cd team-order && make proto && make check && go build ./... && GO_SDK_REQUIRE_DOCKER=1 go test -race ./... -count=1
- docker build -t team-order:go-sdk-check team-order
- openspec validate order-go-sdk-v1 --strict
- make -C platform-e2e features-check
