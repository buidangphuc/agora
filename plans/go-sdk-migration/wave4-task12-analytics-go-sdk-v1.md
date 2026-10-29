# [W4-T12] analytics-go-sdk-v1

## Role
feature-run worker (change `openspec/changes/analytics-go-sdk-v1`)

## Objective
Implement OpenSpec change `analytics-go-sdk-v1` test-first per its tasks.md sections 1 and 2: migrate team-analytics (consumer, ga4import and ga4-relay binaries) to go-sdk v1.0.2, ADR-0016 layout, shared make/lint, fetch-bounded batches with at-least-once commits and dead-lettering after 5 attempts, readiness on the warehouse only, gRPC health on the relay, `MOCK_DATA=ga4export` running the real importer, no dependency enable flags, no runtime in-memory fallback, FEATURES.yaml entries of task 2.1; context design.md Clarifications. Section 3 (compose, gitops and the platform-e2e edits of 3.9 and 3.11) belongs to deploy-go-sdk-v1, not here.

## Write-set (EXCLUSIVE)
- team-analytics/**
- openspec/changes/analytics-go-sdk-v1/**
- openspec/specs/**/spec.md

## Verify
- cd team-analytics && make proto && make check && go build ./... && GO_SDK_REQUIRE_DOCKER=1 go test -race ./... -count=1
- docker build -t team-analytics:go-sdk-check team-analytics
- openspec validate analytics-go-sdk-v1 --strict
- make -C platform-e2e features-check
