# [W0-T1] q10-order-lifecycle-contract

## Role
feature-run worker (change `openspec/changes/archive/2026-10-18-q10-order-lifecycle-contract`, owner platform-core)

## Objective
Implement OpenSpec change `q10-order-lifecycle-contract` test-first per its tasks.md (scenario ids there); context in plans/q10-order-lifecycle/00-spec.md and the change's design.md Clarifications.
- Additive order.proto/payment.proto contract: ConfirmReceipt RPC, CancelReason/CompletedBy enums, Order fields 18-24, OrderReturn.can_escalate, OrderCancelled.reason_code, OrderPaidEvent seller_id/ship_by, OrderPlaced/OrderPaymentReminder/OrderCompleted/ReturnRequested, PAYMENT_STATUS_CANCELLED; byte-identical copies into twenty consumer paths.

## Write-set (EXCLUSIVE)
- platform-core/packages/proto/platform/order/v1/order.proto
- platform-core/packages/proto/platform/payment/v1/payment.proto
- platform-core/packages/proto/platform/order/v1/lifecycle_compat_test.go
- team-frontend/proto/platform/order/v1/order.proto
- team-order/proto/platform/order/v1/order.proto
- team-engagement/proto/platform/order/v1/order.proto
- team-gateway/proto/platform/order/v1/order.proto
- team-domain/proto/platform/order/v1/order.proto
- team-notification/proto/platform/order/v1/order.proto
- team-chat/proto/platform/order/v1/order.proto
- team-payment/proto/platform/order/v1/order.proto
- team-search/proto/platform/order/v1/order.proto
- team-analytics/proto/platform/order/v1/order.proto
- team-chat/proto/platform/payment/v1/payment.proto
- team-gateway/proto/platform/payment/v1/payment.proto
- team-frontend/proto/platform/payment/v1/payment.proto
- team-payment/proto/platform/payment/v1/payment.proto
- team-search/proto/platform/payment/v1/payment.proto
- team-analytics/proto/platform/payment/v1/payment.proto
- team-notification/proto/platform/payment/v1/payment.proto
- team-order/proto/platform/payment/v1/payment.proto
- team-domain/proto/platform/payment/v1/payment.proto
- team-engagement/proto/platform/payment/v1/payment.proto
- platform-core/FEATURES.yaml
- openspec/changes/archive/2026-10-18-q10-order-lifecycle-contract/**
- platform-core/packages/proto/gen_lifecycle_contract_test.sh (widened at build: task 2.2)
- platform-core/packages/proto/consumer_copies_test.sh (widened at build: task 2.3)

## Read-only dependencies
- openspec/changes/archive/2026-10-18-q10-order-lifecycle-contract/** is frozen by its LOCK.md digest except tasks.md ticks
- platform-core/packages/proto/platform/analytics/v1/loopevents_compat_test.go (pattern to mirror)
- plans/q10-order-lifecycle/00-spec.md, slices.yaml, LOCK.md

## Verify
make -C platform-core lint-proto breaking; (cd platform-core/packages/proto && buf lint && buf build && go test ./platform/order/v1/... ./platform/payment/v1/...); for r in team-frontend team-order team-engagement team-gateway team-domain team-notification team-chat team-payment team-search team-analytics; do cmp platform-core/packages/proto/platform/order/v1/order.proto $r/proto/platform/order/v1/order.proto || exit 1; done; for r in team-chat team-gateway team-frontend team-payment team-search team-analytics team-notification team-order team-domain team-engagement; do cmp platform-core/packages/proto/platform/payment/v1/payment.proto $r/proto/platform/payment/v1/payment.proto || exit 1; done; $PY platform-e2e/scripts/spec_sync.py q10-order-lifecycle-contract --strict
# E2E: none (pure contract; FEATURES.yaml entries not-testable)
