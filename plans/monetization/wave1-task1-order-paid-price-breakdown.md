# [W1-T1] order-paid-price-breakdown

## Role
feature-run worker (change `openspec/changes/archive/2026-11-17-order-paid-price-breakdown`)

## Objective
Implement OpenSpec change `order-paid-price-breakdown` test-first per its tasks.md; context in plans/monetization/00-spec.md (decisions in LOCK.md, defaults in brackets apply).
- team-order fills `OrderPaidEvent` fields 9-12 in `BuildOrderPaidEnvelope` with a coherence guard and `order_paid_breakdown_skipped_total`; breakdown committed in the PAID outbox row; legacy pending rows untouched.
- depends_on: monetization-contract
- base: `asdlc/monetization` after wave 0
- hot file: none (team-order only in this wave)

## Write-set (EXCLUSIVE)
- team-order/internal/events/**
- team-order/internal/store/**
- team-order/internal/order/**
- team-order/go.mod
- team-order/go.sum
- team-order/README.md
- team-order/FEATURES.yaml
- platform-e2e/tests/e2e/features/order/order_events_published.feature
- platform-e2e/tests/e2e/step_definitions/order_events_published_steps.py
- platform-e2e/tests/e2e/support/order_events_published_support.py
- platform-e2e/tests/e2e/step_definitions/test_order_events_published.py

### widened (P1)
- platform-e2e/tests/e2e/flows/order_events_flow.py (e2e support flow: decode OrderPaidEvent breakdown fields 9-12)
- platform-e2e/tests/e2e/step_definitions/order_paid_breakdown_steps.py (e2e step file for the breakdown scenarios)
- platform-e2e/tests/e2e/step_definitions/test_order_events.py (e2e step module import of the breakdown steps)

## Verify
- `DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-order` (equivalent: `cd team-order && DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 go test ./...`)
- `cd team-order && make check`
- e2e: order/order_events_published.feature against env monetization (team-order, team-payment, team-promotion)
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py spec-sync . order-paid-price-breakdown`
