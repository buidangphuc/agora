# [W3-T1] notification-order-work

## Role
feature-run worker (change `openspec/changes/notification-order-work`, owner team-notification)

## Objective
Implement OpenSpec change `notification-order-work` test-first per its tasks.md (scenario ids there); context in plans/q10-order-lifecycle/00-spec.md and the change's design.md Clarifications.
- team-notification order-work notifications (OrderPlaced COD, OrderPaidEvent, OrderPaymentReminder, expiry OrderCancelled, OrderCompleted, ReturnRequested, ReturnRefunded) and durable low-stock state with LOW_STOCK_THRESHOLD (default 5).

## Write-set (EXCLUSIVE)
- team-notification/proto/platform/order/v1/order.proto
- team-notification/proto/platform/payment/v1/payment.proto
- team-notification/generated/**
- team-notification/internal/consumer/user_events.go
- team-notification/internal/consumer/listing.go
- team-notification/internal/consumer/order_work_test.go
- team-notification/internal/consumer/low_stock_test.go
- team-notification/internal/consumer/*_test.go
- team-notification/migrations/005_low_stock_state.up.sql
- team-notification/migrations/005_low_stock_state.down.sql
- team-notification/internal/repository/consumer_state.go
- team-notification/internal/repository/*_test.go
- team-notification/internal/config/config.go
- team-notification/internal/config/config_test.go
- team-notification/cmd/server/main.go
- team-notification/.env.example # widened at build: task 4
- team-notification/README.md
- team-notification/FEATURES.yaml
- platform-e2e/tests/e2e/support/**
- platform-e2e/tests/e2e/features/notification/order_work.feature
- platform-e2e/tests/e2e/step_definitions/test_notification_order_work.py
- openspec/changes/notification-order-work/**

## Read-only dependencies
- openspec/changes/notification-order-work/** is frozen by its LOCK.md digest except tasks.md ticks
- team-notification/internal/consumer/user_events_test.go fakes, durable_state_pg_test.go (pattern)
- order helpers from order-unpaid-expiry / order-receipt-completion in support/ (extend, reuse before adding)
- plans/q10-order-lifecycle/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-notification && test -z "$(gofmt -l . | grep -v generated)" && go vet ./... && go test ./... -race); for f in order/v1/order.proto payment/v1/payment.proto; do cmp platform-core/packages/proto/platform/$f team-notification/proto/platform/$f || exit 1; done; (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_notification_order_work.py); $PY platform-e2e/scripts/spec_sync.py notification-order-work --strict
