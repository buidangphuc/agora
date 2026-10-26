# [W4-T2] audit-go-sdk-v1

## Role
feature-run worker (change `openspec/changes/audit-go-sdk-v1`)

## Objective
Implement OpenSpec change `audit-go-sdk-v1` test-first per its tasks.md: migrate team-audit to go-sdk v1.0.2, ADR-0016 layout (internal/audit, internal/store, internal/rpc, internal/app), shared make/lint, no dependency enable flags, no in-memory runtime fallback, `migrate up` subcommand; context design.md Clarifications. Compose and gitops edits belong to deploy-go-sdk-v1, not here.

## Write-set (EXCLUSIVE)
- team-audit/**
- openspec/changes/audit-go-sdk-v1/**

## Verify
- cd team-audit && make proto && make check && go build ./... && GO_SDK_REQUIRE_DOCKER=1 go test -race ./... -count=1
- docker build -t team-audit:sdk team-audit
- openspec validate audit-go-sdk-v1 --strict
- make -C platform-e2e features-check
