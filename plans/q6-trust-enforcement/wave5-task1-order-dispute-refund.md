# [W5-T1] order-dispute-refund

## Role
feature-run worker (change `openspec/changes/order-dispute-refund`)

## Objective
Implement OpenSpec change `order-dispute-refund` test-first per its tasks.md (scenario ids there); context in plans/q6-trust-enforcement/00-spec.md and the change's design.md Clarifications.
- HELD until q10-order-lifecycle merges to main. team-order consumes engagement DisputeResolved: a BUYER_FAVOUR ruling re-approves the linked return (fits remainder) and drives the ReturnRefunded saga for online-paid orders, audit-first (return.dispute_refund), idempotent, DLQ for permanent errors.

**HELD:** build only after q10-order-lifecycle has merged to main.
- depends_on: engagement-dispute-escalation

## Write-set (EXCLUSIVE)
- team-order/proto/platform/engagement/v1/events.proto
- team-order/proto/platform/audit/v1/audit.proto
- team-order/generated/**
- team-order/internal/repository/return.go
- team-order/internal/repository/return_reapprove_test.go
- team-order/internal/config/config.go
- team-order/internal/config/config_test.go
- team-order/internal/consumer/dispute.go
- team-order/internal/consumer/dispute_test.go
- team-order/internal/consumer/**
- team-order/internal/handler/return_refund_test.go
- team-order/internal/bootstrap/kafka.go
- team-order/internal/bootstrap/kafka_test.go
- team-order/cmd/server/main.go
- team-order/README.md
- team-order/.env.example # widened at build: task 3
- team-order/FEATURES.yaml
- docker-compose.services.yaml
- platform-e2e/tests/e2e/features/order/dispute_refund.feature
- platform-e2e/tests/e2e/step_definitions/test_order_dispute_refund.py
- openspec/changes/order-dispute-refund/**

## Read-only dependencies
- openspec/changes/order-dispute-refund/** is frozen by the LOCK.md digest (only task ticks change)
- platform-e2e/tests/e2e/support/oic_order_support.py, prm_rma_support.py, audit_support.py, dde_support.py (reuse)
- q10 timer overlay
- plans/q6-trust-enforcement/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-order && make proto && go vet ./... && go test ./... -race); (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_order_dispute_refund.py -m 'not destructive') then the @destructive audit-down scenario alone; $PY platform-e2e/scripts/spec_sync.py order-dispute-refund --strict
