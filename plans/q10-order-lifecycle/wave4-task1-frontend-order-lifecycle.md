# [W4-T1] frontend-order-lifecycle

## Role
feature-run worker (change `openspec/changes/frontend-order-lifecycle`, owner team-frontend)

## Objective
Implement OpenSpec change `frontend-order-lifecycle` test-first per its tasks.md (scenario ids there); context in plans/q10-order-lifecycle/00-spec.md and the change's design.md Clarifications.
- Buyer/seller UI for the lifecycle: payment deadline + retry, pay page states (expired, COD, paid, cancelled; create transaction when none), cancel reason codes, ConfirmReceipt button, return window, seller 'Cần gửi' KPI and ship-by. No proto edit (copies refreshed only).

## Write-set (EXCLUSIVE)
- team-frontend/proto/platform/order/v1/order.proto
- team-frontend/proto/platform/payment/v1/payment.proto
- team-frontend/src/generated/**
- team-frontend/src/lib/gateway/orders.ts
- team-frontend/src/lib/gateway/orders.test.ts
- team-frontend/src/lib/gateway/payment.ts
- team-frontend/src/lib/gateway/payment.test.ts
- team-frontend/src/features/order/lifecycle.ts
- team-frontend/src/features/order/lifecycle.test.ts
- team-frontend/src/features/order/cancelReasons.ts
- team-frontend/src/features/order/OrderDetailView.tsx
- team-frontend/src/features/order/OrderDetailView.test.tsx
- team-frontend/src/features/order/OrderList.tsx
- team-frontend/src/features/order/OrderList.test.tsx
- team-frontend/src/features/order/OrderActions.tsx
- team-frontend/src/features/order/OrderActions.test.tsx
- team-frontend/src/features/order/actions.ts
- team-frontend/src/features/order/actions.test.ts
- team-frontend/src/features/order/returns.actions.test.ts
- team-frontend/src/features/order/ConfirmReceiptButton.tsx
- team-frontend/src/features/order/ConfirmReceiptButton.test.tsx
- team-frontend/src/app/(checkout)/checkout/pay/[id]/page.tsx
- team-frontend/src/app/(checkout)/checkout/pay/[id]/page.test.tsx
- team-frontend/src/features/payment/PaymentSimulator.tsx
- team-frontend/src/features/payment/PaymentSimulator.test.tsx
- team-frontend/src/app/(shop)/seller/orders/[id]/page.test.tsx
- team-frontend/src/features/seller/kpi.ts
- team-frontend/src/features/seller/kpi.test.ts
- team-frontend/src/features/seller/KpiRow.tsx
- team-frontend/src/app/(shop)/seller/(dashboard)/page.tsx
- team-frontend/src/app/(shop)/seller/(dashboard)/page.test.tsx
- team-frontend/src/features/seller/SellerOrdersTable.tsx
- team-frontend/src/features/seller/OrderDetailView.tsx
- team-frontend/src/features/seller/shipBy.tsx
- team-frontend/src/features/seller/shipBy.test.tsx
- team-frontend/src/features/seller/status.tsx
- team-frontend/src/app/(shop)/seller/layout.test.tsx
- team-frontend/FEATURES.yaml
- platform-e2e/tests/e2e/features/frontend/order_lifecycle_ui.feature
- platform-e2e/tests/e2e/step_definitions/order_lifecycle_ui_steps.py
- platform-e2e/tests/e2e/step_definitions/test_order_lifecycle_ui.py
- platform-e2e/tests/e2e/support/order_lifecycle_ui_support.py
- platform-e2e/compose/order-inventory.override.yaml
- platform-e2e/tests/e2e/step_definitions/d2_seller_steps.py
- platform-e2e/tests/e2e/step_definitions/uif_seller_steps.py
- openspec/changes/frontend-order-lifecycle/**
- team-frontend/src/features/order/orderFixtures.ts # widened at build: task 1.10

## Read-only dependencies
- openspec/changes/frontend-order-lifecycle/** is frozen by its LOCK.md digest except tasks.md ticks
- team-gateway/internal/edge (ConfirmReceipt forwarder owned by order-receipt-completion)
- platform-e2e/tests/e2e/support/** order helpers from backend slices (reuse, do not copy)
- plans/q10-order-lifecycle/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-frontend && npx biome check . && npx tsc --noEmit && npm test); for f in order/v1/order.proto payment/v1/payment.proto; do cmp platform-core/packages/proto/platform/$f team-frontend/proto/platform/$f || exit 1; done; ! grep -rn "Thanh toán ngay\|hoàn tác: tồn kho" team-frontend/src; (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_order_lifecycle_ui.py tests/e2e/step_definitions/test_d2_seller_workplace.py tests/e2e/step_definitions/test_uif_seller.py); $PY platform-e2e/scripts/spec_sync.py frontend-order-lifecycle --strict
