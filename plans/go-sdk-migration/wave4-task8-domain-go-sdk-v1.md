# [W4-T8] domain-go-sdk-v1

## Role
feature-run worker (change `openspec/changes/domain-go-sdk-v1`)

## Objective
Implement OpenSpec change `domain-go-sdk-v1` test-first per its tasks.md: migrate team-domain to go-sdk v1.0.2, ADR-0016 layout (internal/app, internal/config, internal/rpc, internal/listing, internal/store, internal/events, internal/objects, internal/screen, internal/audit), shared make/lint, no dependency enable flags, no runtime fallback when an upstream is not wired, outbox relay on the SDK, migrations 0013-0016, `migrate up` subcommand, FEATURES.yaml entries of task 2.1; context design.md Clarifications. Compose and gitops edits belong to deploy-go-sdk-v1, not here.

## Write-set (EXCLUSIVE)
- team-domain/**
- openspec/changes/domain-go-sdk-v1/**
- team-domain/internal/events/publisher.go (widened at build: task 1.6)
- team-domain/internal/events/publisher_test.go (widened at build: task 1.6)

## Verify
- cd team-domain && make proto && make check && go build ./... && GO_SDK_REQUIRE_DOCKER=1 go test -race ./... -count=1
- docker build --build-arg BIN=server --build-arg GRPC_PORT=50051 -t team-domain:go-sdk-check team-domain
- openspec validate domain-go-sdk-v1 --strict
- make -C platform-e2e features-check
