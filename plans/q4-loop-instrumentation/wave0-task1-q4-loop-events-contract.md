# [W0-T1] q4-loop-events-contract

## Role
feature-run worker (change `openspec/changes/q4-loop-events-contract`)

## Objective
Implement OpenSpec change `q4-loop-events-contract` test-first per its tasks.md (scenario ids there); context in plans/q4-loop-instrumentation/00-spec.md and the change's design.md Clarifications.
- Additive EventType 15-21 in analytics.proto with property-key comments; vendor byte-identical into team-gateway, team-analytics, team-frontend.

## Write-set (EXCLUSIVE)
- platform-core/packages/proto/platform/analytics/v1/analytics.proto
- platform-core/packages/proto/platform/analytics/v1/loopevents_compat_test.go
- team-gateway/proto/platform/analytics/v1/analytics.proto
- team-analytics/proto/platform/analytics/v1/analytics.proto
- team-frontend/proto/platform/analytics/v1/analytics.proto
- platform-core/FEATURES.yaml

## Read-only dependencies
- openspec/changes/q4-loop-events-contract/** (frozen by LOCK.md digest)
- platform-core/packages/proto/platform/analytics/v1/consent_compat_test.go (pattern to mirror)
- plans/q4-loop-instrumentation/00-spec.md, slices.yaml, LOCK.md

## Verify
make -C platform-core lint-proto breaking; (cd platform-core/packages/proto && buf lint && buf build && go test ./platform/analytics/v1/...); for r in team-gateway team-analytics team-frontend; do cmp platform-core/packages/proto/platform/analytics/v1/analytics.proto $r/proto/platform/analytics/v1/analytics.proto || exit 1; (cd $r/proto && buf lint); done; $PY platform-e2e/scripts/spec_sync.py q4-loop-events-contract --strict
