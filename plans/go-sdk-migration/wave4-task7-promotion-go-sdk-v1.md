# [W4-T7] promotion-go-sdk-v1

## Role
feature-run worker (change `openspec/changes/promotion-go-sdk-v1`)

## Objective
Implement OpenSpec change `promotion-go-sdk-v1` test-first per its tasks.md: migrate team-promotion to go-sdk v1.0.2, ADR-0016 layout (internal/config, internal/store, internal/events, internal/upstream, internal/listing, internal/voucher, internal/flashsale, internal/subscription, internal/sponsored, internal/rpc, internal/app, internal/docs), shared make/lint, no dependency enable flags, no runtime fallback when an upstream is not wired, outbox relay on the SDK, migration 0005_outbox, `migrate up` subcommand, FEATURES.yaml entries of task 2.1; context design.md Clarifications. Compose and gitops edits belong to deploy-go-sdk-v1, not here.

## Write-set (EXCLUSIVE)
- team-promotion/**
- openspec/changes/promotion-go-sdk-v1/**
- team-promotion/internal/listing/owner.go (widened at build: task 1.6)
- platform-e2e/unit/test_promotion_features_manifest.py (widened at build: task 2.1)

## Verify
- cd team-promotion && make proto && make check && go build ./... && GO_SDK_REQUIRE_DOCKER=1 go test -race ./... -count=1
- docker build --build-arg BIN=server --build-arg GRPC_PORT=50061 -t team-promotion:go-sdk-check team-promotion
- openspec validate promotion-go-sdk-v1 --strict
- make -C platform-e2e features-check
