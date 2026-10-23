# [W4-T6] engagement-go-sdk-v1

## Role
feature-run worker (change `openspec/changes/engagement-go-sdk-v1`)

## Objective
Implement OpenSpec change `engagement-go-sdk-v1` test-first per its tasks.md: migrate team-engagement to go-sdk v1.0.2, ADR-0016 layout (internal/config, internal/store, internal/engagement, internal/upstream, internal/rpc, internal/events, internal/app), shared make/lint, no dependency enable flags, no runtime fallback when an upstream is not wired, outbox and listing consumer on the SDK with keyed `key_fp` logging, migration 0014, `migrate up` subcommand, page sizes through `pagination.Clamp`; context design.md Clarifications. Compose and gitops edits belong to deploy-go-sdk-v1, not here.

## Write-set (EXCLUSIVE)
- team-engagement/**
- openspec/changes/engagement-go-sdk-v1/**
- team-engagement/go.mod (widened at build: task 2)
- team-engagement/go.sum (widened at build: task 2)

## Verify
- cd team-engagement && make proto && make check && go build ./... && GO_SDK_REQUIRE_DOCKER=1 go test -race ./... -count=1
- docker build -t team-engagement:sdk team-engagement
- openspec validate engagement-go-sdk-v1 --strict
- make -C platform-e2e features-check
