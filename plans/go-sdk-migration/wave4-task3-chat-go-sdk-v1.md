# [W4-T3] chat-go-sdk-v1

## Role
feature-run worker (change `openspec/changes/chat-go-sdk-v1`)

## Objective
Implement OpenSpec change `chat-go-sdk-v1` test-first per its tasks.md: migrate team-chat to go-sdk v1.0.2, ADR-0016 layout (internal/config, internal/chat, internal/store, internal/rpc, internal/app), shared make/lint, no dependency enable flags, no in-memory runtime fallback, `migrate up` subcommand; context design.md Clarifications. Compose and gitops edits belong to deploy-go-sdk-v1, not here.

## Write-set (EXCLUSIVE)
- team-chat/**
- openspec/changes/chat-go-sdk-v1/**

## Verify
- cd team-chat && make proto && make check && go build ./... && GO_SDK_REQUIRE_DOCKER=1 go test -race ./... -count=1
- docker build -t team-chat:sdk team-chat
- openspec validate chat-go-sdk-v1 --strict
- make -C platform-e2e features-check
