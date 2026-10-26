# [W4-T1] verification-go-sdk-v1

## Role
feature-run worker (change `openspec/changes/verification-go-sdk-v1`)

## Objective
Implement OpenSpec change `verification-go-sdk-v1` test-first per its tasks.md: migrate team-verification to go-sdk v1.0.2, ADR-0016 layout, shared make/lint, no dependency enable flags, no in-memory runtime fallback, `migrate up` subcommand, audit through auditx; context design.md Clarifications. Compose and gitops edits belong to deploy-go-sdk-v1, not here.

## Write-set (EXCLUSIVE)
- team-verification/**
- openspec/changes/verification-go-sdk-v1/**

## Verify
- cd team-verification && make check && go build ./... && GO_SDK_REQUIRE_DOCKER=1 go test -race ./... -count=1
- docker build -t team-verification:sdk team-verification
- openspec validate verification-go-sdk-v1 --strict
- make -C platform-e2e features-check
