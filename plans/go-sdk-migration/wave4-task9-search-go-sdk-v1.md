# [W4-T9] search-go-sdk-v1

## Role
feature-run worker (change `openspec/changes/search-go-sdk-v1`)

## Objective
Implement OpenSpec change `search-go-sdk-v1` test-first per its tasks.md: migrate team-search (server and indexer binaries) to go-sdk v1.0.2, ADR-0016 layout (internal/app, internal/config, internal/rpc, internal/store, internal/events, internal/index, internal/taxonomy, internal/tombstone), shared make/lint, no dependency enable flags, no runtime in-memory fallback, classifier on `upstream.NewClient`, indexer on `consume`, `migrate up` subcommand, p95 baseline of task 1.0.1, FEATURES.yaml entries and boot-guard e2e edits of tasks 2.1-2.3; context design.md Clarifications. Compose and gitops edits belong to deploy-go-sdk-v1, not here.

## Write-set (EXCLUSIVE)
- team-search/**
- openspec/changes/search-go-sdk-v1/**
- platform-e2e/tests/e2e/features/ops/boot_guard_search.feature
- platform-e2e/tests/e2e/step_definitions/visibility_boot_guard_steps.py

## Verify
- cd team-search && make proto && make check && go build ./... && GO_SDK_REQUIRE_DOCKER=1 go test -race ./... -count=1
- docker build --build-arg BIN=server -t team-search:go-sdk-check team-search
- openspec validate search-go-sdk-v1 --strict
- make -C platform-e2e features-check
