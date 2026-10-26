# [W0-T1] q11-checkout-contract

## Role
feature-run worker (change `openspec/changes/archive/2026-10-26-q11-checkout-contract`)

## Objective
Implement OpenSpec change `q11-checkout-contract` test-first per its tasks.md (scenario ids there); context in plans/q11-checkout-coherence/00-spec.md and the change's design.md Clarifications.
- Additive contract: `OrderService.PreviewCheckout`, five quote messages, local `CheckoutVoucherScope` enum, `CreateOrderRequest` 5-7, `CalculateShippingFeeResponse` 4-5 (ETA), `Order` 25-27, `ValidateAndReserveResponse` scope/seller_id; vendor `order.proto` into ten trees and `promotion.proto` into five, byte-identical.
- Re-verify field numbers against the base (q10-order-lifecycle is merged to main) and record any shift in design.md.

## Write-set (EXCLUSIVE)
- platform-core/packages/proto/platform/order/v1/order.proto
- platform-core/packages/proto/platform/promotion/v1/promotion.proto
- platform-core/packages/proto/platform/order/v1/checkout_compat_test.go
- team-analytics/proto/platform/order/v1/order.proto
- team-chat/proto/platform/order/v1/order.proto
- team-domain/proto/platform/order/v1/order.proto
- team-engagement/proto/platform/order/v1/order.proto
- team-frontend/proto/platform/order/v1/order.proto
- team-gateway/proto/platform/order/v1/order.proto
- team-notification/proto/platform/order/v1/order.proto
- team-order/proto/platform/order/v1/order.proto
- team-payment/proto/platform/order/v1/order.proto
- team-search/proto/platform/order/v1/order.proto
- team-analytics/proto/platform/promotion/v1/promotion.proto
- team-frontend/proto/platform/promotion/v1/promotion.proto
- team-gateway/proto/platform/promotion/v1/promotion.proto
- team-order/proto/platform/promotion/v1/promotion.proto
- team-promotion/proto/platform/promotion/v1/promotion.proto
- platform-core/FEATURES.yaml
- openspec/changes/archive/2026-10-26-q11-checkout-contract/**
- platform-core/packages/proto/checkout_consumer_copies_test.sh (# widened at build: task 2-3)

## Read-only dependencies
- openspec/changes/archive/2026-10-26-q11-checkout-contract/** is frozen by the LOCK.md digest (only task ticks and the 1.1 field-number note change)
- platform-core/packages/proto/platform/promotion/v1/flashstock_compat_test.go (pattern to mirror)
- plans/q11-checkout-coherence/00-spec.md, slices.yaml, LOCK.md

## Verify
make -C platform-core lint-proto breaking; (cd platform-core/packages/proto && buf lint && buf build && go test ./platform/order/v1/... ./platform/promotion/v1/...); for r in team-analytics team-chat team-domain team-engagement team-frontend team-gateway team-notification team-order team-payment team-search; do cmp platform-core/packages/proto/platform/order/v1/order.proto $r/proto/platform/order/v1/order.proto || exit 1; done; for r in team-analytics team-frontend team-gateway team-order team-promotion; do cmp platform-core/packages/proto/platform/promotion/v1/promotion.proto $r/proto/platform/promotion/v1/promotion.proto || exit 1; done; E2E: none (pure contract; platform-core/FEATURES.yaml entries not-testable); $PY platform-e2e/scripts/spec_sync.py q11-checkout-contract --strict

Vars: PY=/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python; BUILD=/Users/phuc.buidang/Library/Caches/ai-first-runs/c4q11-build; WT=<slice worktree>. Run every test/gate via scripts/asdlc/gate_digest.py (gate-digest skill).
