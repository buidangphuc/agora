# [W1-T2] gateway-loop-event-intake

## Role
feature-run worker (change `openspec/changes/archive/2026-10-18-gateway-loop-event-intake`)

## Objective
Implement OpenSpec change `gateway-loop-event-intake` test-first per its tasks.md (scenario ids there); context in plans/q4-loop-instrumentation/00-spec.md and the change's design.md Clarifications.
- Edge collector accepts the 7 new beacon types; ai_interaction properties exactly {surface, action} from the allowlists else rejected.

## Write-set (EXCLUSIVE)
- team-gateway/internal/edge/collector.go
- team-gateway/internal/edge/collector_test.go
- team-gateway/README.md
- team-gateway/FEATURES.yaml
- platform-e2e/tests/e2e/features/tracking/loop_event_intake.feature
- platform-e2e/tests/e2e/step_definitions/test_loop_event_intake.py

## Read-only dependencies
- openspec/changes/archive/2026-10-18-gateway-loop-event-intake/** (frozen by LOCK.md digest)
- platform-e2e/tests/e2e/step_definitions/tracking_steps.py, tii_validation_steps.py (import, never edit)
- plans/q4-loop-instrumentation/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-gateway && make proto && go vet ./... && go test ./...); (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_loop_event_intake.py); $PY platform-e2e/scripts/spec_sync.py gateway-loop-event-intake --strict
