# [W2-T1] order-multishop-checkout

## Role
feature-run worker (change `openspec/changes/order-multishop-checkout`)

## Objective
Implement OpenSpec change `order-multishop-checkout` test-first per its tasks.md (scenario ids there); context in plans/q11-checkout-coherence/00-spec.md and the change's design.md Clarifications.
- `PreviewCheckout` (reserves nothing), per-shop quote groups, shop + platform voucher routing (platform prorated by items_subtotal, remainder to largest group, ties lowest seller id), `checkout_id` and platform fields on orders, platform commit on first paid order, release only when every order of the checkout is cancelled before payment (also on q10's system cancel via `CancelOrderWithReason`), gateway forwarder.
- depends_on: promotion-voucher-scope, order-address-eta (both merged into asdlc/q11-checkout-build first). q10-order-lifecycle is on main; rebase, never hand-merge order.go.

## Write-set (EXCLUSIVE)
- team-order/proto/platform/order/v1/order.proto
- team-order/proto/platform/promotion/v1/promotion.proto
- team-order/generated/**
- team-gateway/proto/platform/order/v1/order.proto
- team-gateway/generated/**
- team-order/migrations/**
- team-order/internal/repository/order.go
- team-order/internal/repository/*_test.go
- team-order/internal/service/checkoutquote.go
- team-order/internal/service/redemption.go
- team-order/internal/service/order.go
- team-order/internal/service/saga.go
- team-order/internal/service/*_test.go
- team-order/internal/handler/order.go
- team-order/internal/handler/*_test.go
- team-order/internal/consumer/payment.go
- team-order/internal/consumer/*_test.go
- team-gateway/internal/edge/order.go
- team-gateway/internal/edge/order_test.go
- platform-core/docs/ROADMAP.md
- team-order/README.md
- team-order/FEATURES.yaml
- team-gateway/FEATURES.yaml
- platform-e2e/tests/e2e/support/**
- platform-e2e/tests/e2e/features/order/multishop_checkout.feature
- platform-e2e/tests/e2e/step_definitions/test_order_multishop_checkout.py
- openspec/changes/order-multishop-checkout/**

## Read-only dependencies
- openspec/changes/order-multishop-checkout/** is frozen by the LOCK.md digest (only task ticks change)
- team-order/internal/service/lifecycle.go (q10 expiry sweeper; it cancels through CancelOrderWithReason in order.go)
- platform-e2e two-seller checkout steps (oic-checkout-a-two-seller-checkout-places-one-order-per-seller), support/voucher_seed.py (ensure_known_voucher for SAVE10), voucher_scope_support.py and address_support.py from wave 1
- plans/q11-checkout-coherence/00-spec.md, slices.yaml, LOCK.md

## Verify
for r in team-order team-gateway; do cmp platform-core/packages/proto/platform/order/v1/order.proto $r/proto/platform/order/v1/order.proto || exit 1; done; cmp platform-core/packages/proto/platform/promotion/v1/promotion.proto team-order/proto/platform/promotion/v1/promotion.proto; bash $BUILD/gen.sh $WT team-order team-gateway; (cd team-order && gofmt -l . && go vet ./... && go test ./... -race) with TEST_DATABASE_URL on a throwaway postgres; make -C team-gateway check; bash $BUILD/e2e.sh $WT tests/e2e/step_definitions/test_order_multishop_checkout.py; $PY platform-e2e/scripts/spec_sync.py order-multishop-checkout --strict

Vars: PY=/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python; BUILD=/Users/phuc.buidang/Library/Caches/ai-first-runs/c4q11-build; WT=<slice worktree>. Run every test/gate via scripts/asdlc/gate_digest.py (gate-digest skill).
