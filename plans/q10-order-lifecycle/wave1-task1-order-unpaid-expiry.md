# [W1-T1] order-unpaid-expiry

## Role
feature-run worker (change `openspec/changes/order-unpaid-expiry`, owner team-order)

## Objective
Implement OpenSpec change `order-unpaid-expiry` test-first per its tasks.md (scenario ids there); context in plans/q10-order-lifecycle/00-spec.md and the change's design.md Clarifications.
- team-order payment deadline (24h) snapshot, OrderPlaced and OrderPaymentReminder outbox events, lifecycle sweep (reminder + expiry passes), Pending->Cancelled system transition, CancelReason on cancel path and OrderCancelled; e2e timer overlay defined once.

## Write-set (EXCLUSIVE)
- team-order/proto/platform/order/v1/order.proto
- team-order/proto/platform/payment/v1/payment.proto
- team-order/generated/**
- team-order/migrations/0008_order_payment_deadline.up.sql
- team-order/migrations/0008_order_payment_deadline.down.sql
- team-order/internal/config/config.go
- team-order/internal/config/config_test.go
- team-order/internal/repository/order.go
- team-order/internal/repository/placer.go
- team-order/internal/repository/*_test.go
- team-order/internal/events/publisher.go
- team-order/internal/events/*_test.go
- team-order/internal/service/order.go
- team-order/internal/service/transitions.go
- team-order/internal/service/lifecycle.go
- team-order/internal/service/*_test.go
- team-order/internal/handler/order.go
- team-order/internal/handler/*_test.go
- team-order/internal/consumer/payment_test.go
- team-order/internal/consumer/payment_expiry_test.go
- team-order/cmd/server/main.go
- team-order/README.md
- team-order/FEATURES.yaml
- platform-e2e/compose/order-inventory.override.yaml
- platform-e2e/tests/e2e/support/**
- platform-e2e/tests/e2e/features/order/unpaid_expiry.feature
- platform-e2e/tests/e2e/step_definitions/test_order_unpaid_expiry.py
- openspec/changes/order-unpaid-expiry/**
- team-order/.env.example (# widened at build: task 2)
- plans/q10-order-lifecycle/wave1-task1-order-unpaid-expiry.md (# widened at build: task 2; packet widening record)
- team-order/internal/consumer/payment.go (# widened at build: task 4)
- team-order/cmd/server/lifecycle_sweeper_test.go (# widened at build: task 8)

## Read-only dependencies
- openspec/changes/order-unpaid-expiry/** is frozen by its LOCK.md digest except tasks.md ticks
- team-order runReservationSweeper, ReservationTTL accessor, cancel/ForceFailSaga paths (pattern)
- platform-e2e/tests/e2e/support/oic_order_support.py and order_events_published.feature helpers (reuse)
- plans/q10-order-lifecycle/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-order && test -z "$(gofmt -l . | grep -v generated)" && go vet ./... && go test ./... -race); for f in order/v1/order.proto payment/v1/payment.proto; do cmp platform-core/packages/proto/platform/$f team-order/proto/platform/$f || exit 1; done; (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_order_unpaid_expiry.py); $PY platform-e2e/scripts/spec_sync.py order-unpaid-expiry --strict
