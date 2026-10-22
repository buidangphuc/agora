# [W0-T1] q2-review-integrity-contract

## Role
feature-run worker (change `openspec/changes/archive/2026-10-18-q2-review-integrity-contract`)

## Objective
Implement OpenSpec change `q2-review-integrity-contract` test-first per its tasks.md (scenario ids there); context in plans/q2-review-integrity/00-spec.md.
- Also vendors the updated engagement.proto into its consumers: team-engagement, team-gateway, team-frontend (`proto/` copies; consumers regenerate their own gitignored code).

## Write-set (EXCLUSIVE)
- platform-core/packages/proto/platform/engagement/v1/engagement.proto
- platform-core/packages/proto/platform/engagement/v1/hidereview_roundtrip_test.go
- platform-core/packages/proto/go.mod
- platform-core/packages/proto/go.sum
- platform-core/FEATURES.yaml
- team-engagement/proto/platform/engagement/v1/engagement.proto
- team-gateway/proto/platform/engagement/v1/engagement.proto
- team-frontend/proto/platform/engagement/v1/engagement.proto

## Read-only dependencies
- openspec/changes/archive/2026-10-18-q2-review-integrity-contract/** (frozen by LOCK.md digest)
- plans/q2-review-integrity/00-spec.md, plans/q2-review-integrity/slices.yaml, plans/q2-review-integrity/LOCK.md

## Verify
(cd platform-core/packages/proto && buf lint && buf breaking --against '.git#branch=main,subdir=packages/proto' && buf generate && go test ./...); for r in team-engagement team-gateway team-frontend; do cmp platform-core/packages/proto/platform/engagement/v1/engagement.proto $r/proto/platform/engagement/v1/engagement.proto; done; platform-e2e/.venv/bin/python platform-e2e/scripts/spec_sync.py q2-review-integrity-contract --strict
