# [W2-T1] payment-unpaid-cancel

## Role
feature-run worker (change `openspec/changes/payment-unpaid-cancel`, owner team-payment)

## Objective
Implement OpenSpec change `payment-unpaid-cancel` test-first per its tasks.md (scenario ids there); context in plans/q10-order-lifecycle/00-spec.md and the change's design.md Clarifications.
- team-payment voids PENDING/FAILED transactions of a cancelled unpaid order (PAYMENT_STATUS_CANCELLED), refuses ProcessMockPayment on CANCELLED, re-reads order status before settle (M1, fail closed), refunds a late settle once (key cancel:<order_id>).

## Write-set (EXCLUSIVE)
- team-payment/proto/**
- team-payment/generated/**
- team-payment/internal/repository/payment.go
- team-payment/internal/repository/payment_test.go
- team-payment/internal/repository/unpaid_cancel_test.go (# widened at build: task 1)
- team-payment/internal/service/payment.go
- team-payment/internal/service/settlement.go
- team-payment/internal/service/*_test.go
- team-payment/internal/consumer/settlement.go
- team-payment/internal/consumer/unpaid_cancel_test.go
- team-payment/internal/consumer/*_test.go
- team-payment/internal/handler/payment.go
- team-payment/internal/handler/*_test.go
- team-payment/README.md
- team-payment/FEATURES.yaml
- platform-e2e/tests/e2e/features/payment/unpaid_cancel.feature
- platform-e2e/tests/e2e/step_definitions/test_unpaid_cancel.py
- openspec/changes/payment-unpaid-cancel/**

## Read-only dependencies
- openspec/changes/payment-unpaid-cancel/** is frozen by its LOCK.md digest except tasks.md ticks
- platform-e2e/compose/order-inventory.override.yaml (timers owned by order-unpaid-expiry)
- platform-e2e/tests/e2e/features/payment/mock_pay.feature and features/order steps (reuse)
- platform-e2e/tests/e2e/support/** (reuse, do not edit)
- plans/q10-order-lifecycle/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-payment && test -z "$(gofmt -l . | grep -v generated)" && go vet ./... && go test ./... -race); for f in order/v1/order.proto payment/v1/payment.proto; do cmp platform-core/packages/proto/platform/$f team-payment/proto/platform/$f || exit 1; done; (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_unpaid_cancel.py); $PY platform-e2e/scripts/spec_sync.py payment-unpaid-cancel --strict
