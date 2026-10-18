# [W1-T1] analytics-loop-event-storage

## Role
feature-run worker (change `openspec/changes/archive/2026-10-18-analytics-loop-event-storage`)

## Objective
Implement OpenSpec change `analytics-loop-event-storage` test-first per its tasks.md (scenario ids there); context in plans/q4-loop-instrumentation/00-spec.md and the change's design.md Clarifications.
- team-analytics stores EventType 15-21 under their lowercase names; e2e produces envelopes directly onto analytics.events and reads the warehouse.

## Write-set (EXCLUSIVE)
- team-analytics/internal/consumer/tracking.go
- team-analytics/internal/consumer/tracking_test.go
- team-analytics/README.md
- team-analytics/FEATURES.yaml
- platform-e2e/tests/e2e/features/tracking/loop_event_storage.feature
- platform-e2e/tests/e2e/step_definitions/test_loop_event_storage.py

## Read-only dependencies
- openspec/changes/archive/2026-10-18-analytics-loop-event-storage/** (frozen by LOCK.md digest)
- platform-e2e/tests/e2e/support/ga4_relay_support.py, tii_support.py (reuse, never edit)
- plans/q4-loop-instrumentation/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-analytics && make proto && go vet ./... && go test ./...); (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_loop_event_storage.py); $PY platform-e2e/scripts/spec_sync.py analytics-loop-event-storage --strict
