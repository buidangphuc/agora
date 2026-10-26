# [W4-T4] notification-go-sdk-v1

## Role
feature-run worker (change `openspec/changes/notification-go-sdk-v1`)

## Objective
Implement OpenSpec change `notification-go-sdk-v1` test-first per its tasks.md: migrate team-notification to go-sdk v1.0.2, ADR-0016 layout (internal/config, internal/inbox, internal/alerts, internal/prefs, internal/store, internal/rpc, internal/names, internal/events, internal/app), shared make/lint, no dependency enable flags, no in-memory runtime fallback, `migrate up` subcommand; context design.md Clarifications. Compose and gitops edits belong to deploy-go-sdk-v1, not here.

## Write-set (EXCLUSIVE)
- team-notification/**
- openspec/changes/notification-go-sdk-v1/**

## Verify
- cd team-notification && make proto && make check && go build ./... && GO_SDK_REQUIRE_DOCKER=1 go test -race ./... -count=1
- docker build -t team-notification:sdk team-notification
- openspec validate notification-go-sdk-v1 --strict
- make -C platform-e2e features-check
