# [W0-T1] q1-session-refresh-contract

## Role
feature-run worker (change `openspec/changes/q1-session-refresh-contract`)

## Objective
Implement OpenSpec change `q1-session-refresh-contract` test-first per its tasks.md (scenario ids there); context in plans/q1-session-continuity/00-spec.md (incl. Amendments) and the change's design.md Clarifications.
- Also vendors the updated identity.proto into its AuthService consumers: team-identity, team-gateway, team-frontend (`proto/` copies; consumers regenerate their own gitignored code).

## Write-set (EXCLUSIVE)
- platform-core/packages/proto/platform/identity/v1/identity.proto
- platform-core/packages/proto/platform/identity/v1/authresult_roundtrip_test.go
- platform-core/packages/proto/go.mod
- platform-core/packages/proto/go.sum
- platform-core/FEATURES.yaml
- team-identity/proto/platform/identity/v1/identity.proto
- team-gateway/proto/platform/identity/v1/identity.proto
- team-frontend/proto/platform/identity/v1/identity.proto

## Read-only dependencies
- openspec/changes/q1-session-refresh-contract/** (frozen by LOCK.md digest)
- plans/q1-session-continuity/00-spec.md, plans/q1-session-continuity/slices.yaml, plans/q1-session-continuity/LOCK.md (reviewer notes)

## Verify
(cd platform-core/packages/proto && buf lint && buf breaking --against '.git#branch=main,subdir=packages/proto' && buf generate && go test ./...); platform-e2e/.venv/bin/python platform-e2e/scripts/spec_sync.py q1-session-refresh-contract --strict
