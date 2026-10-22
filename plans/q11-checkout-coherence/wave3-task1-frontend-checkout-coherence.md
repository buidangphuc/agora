# [W3-T1] frontend-checkout-coherence

## Role
feature-run worker (change `openspec/changes/frontend-checkout-coherence`)

## Objective
Implement OpenSpec change `frontend-checkout-coherence` test-first per its tasks.md (scenario ids "Checkout coherence: ..."); context in plans/q11-checkout-coherence/00-spec.md and the change's design.md Clarifications.
- Checkout page shows server quote groups from `PreviewCheckout` only (frontend adds nothing up), per-shop and platform voucher slots, server delivery dates (PDP and groups), confirmed address sent with the order and its error mapping, place order goes to the first pay page, pay page creates a missing transaction and offers the next unpaid order.
- depends_on: order-multishop-checkout. Rebase, never hand-merge `(summary)/page.tsx`, `checkoutParams.ts`, `PlaceOrderForm.tsx`, `pay/[id]/page.tsx`.

## Write-set (EXCLUSIVE)
- team-frontend/src/generated/**
- team-frontend/src/lib/gateway/orders.ts
- team-frontend/src/lib/gateway/orders.test.ts
- team-frontend/src/features/order/deliveryDate.ts
- team-frontend/src/features/order/deliveryDate.test.ts
- team-frontend/src/features/listing/ShippingInfo.tsx
- team-frontend/src/features/listing/ShippingInfo.test.tsx
- team-frontend/src/features/listing/fabricatedText.ts
- team-frontend/src/features/order/checkoutParams.ts
- team-frontend/src/features/order/checkoutParams.test.ts
- team-frontend/src/app/(checkout)/checkout/(summary)/page.tsx
- team-frontend/src/app/(checkout)/checkout/(summary)/page.test.tsx
- team-frontend/src/features/order/OrderSummary.tsx
- team-frontend/src/features/order/OrderSummary.test.tsx
- team-frontend/src/features/order/CheckoutGroups.tsx
- team-frontend/src/features/order/CheckoutGroups.test.tsx
- team-frontend/src/features/order/CheckoutSteps.tsx
- team-frontend/src/features/order/CheckoutSteps.test.tsx
- team-frontend/src/features/voucher/VoucherModal.tsx
- team-frontend/src/features/voucher/VoucherModal.test.tsx
- team-frontend/src/features/order/actions.ts
- team-frontend/src/features/order/actions.test.ts
- team-frontend/src/features/order/PlaceOrderForm.tsx
- team-frontend/src/features/order/PlaceOrderForm.test.tsx
- team-frontend/src/features/order/shippingCopy.ts
- team-frontend/src/app/(checkout)/checkout/pay/[id]/page.tsx
- team-frontend/src/app/(checkout)/checkout/pay/[id]/page.test.tsx
- team-frontend/src/app/(checkout)/checkout/pay/[id]/data.ts
- team-frontend/src/features/payment/PaymentSimulator.tsx
- team-frontend/src/features/payment/PaymentSimulator.test.tsx
- team-frontend/FEATURES.yaml
- platform-e2e/tests/e2e/features/frontend/checkout_coherence_ui.feature
- platform-e2e/tests/e2e/step_definitions/checkout_coherence_ui_steps.py
- platform-e2e/tests/e2e/step_definitions/test_checkout_coherence_ui.py
- platform-e2e/tests/e2e/support/checkout_coherence_ui_support.py
- openspec/changes/frontend-checkout-coherence/**
- team-frontend/src/features/order/CheckoutStepper.tsx # widened at build: task 1.8
- team-frontend/src/features/order/orderFixtures.ts # widened at build: task 1.8

## Read-only dependencies
- openspec/changes/frontend-checkout-coherence/** is frozen by the LOCK.md digest (only task ticks change)
- team-frontend/proto/** (copied by q11-checkout-contract; never hand-edited), team-frontend/src/app/(checkout)/checkout/pay/[id]/layout.tsx (guest guard, covered by the pay page test)
- platform-e2e two-seller checkout steps, `d2 buyer has a cart` steps, support/voucher_seed.py (ensure_known_voucher for SAVE10), voucher/address helpers from waves 1-2
- plans/q11-checkout-coherence/00-spec.md, slices.yaml, LOCK.md

## Verify
cmp platform-core/packages/proto/platform/order/v1/order.proto team-frontend/proto/platform/order/v1/order.proto; (cd team-frontend && PATH=$PWD/node_modules/.bin:$PATH npx buf generate && npm test && npm run check); bash $BUILD/e2e.sh $WT tests/e2e/step_definitions/test_checkout_coherence_ui.py; $PY platform-e2e/scripts/spec_sync.py frontend-checkout-coherence --strict

Vars: PY=/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python; BUILD=/Users/phuc.buidang/Library/Caches/ai-first-runs/c4q11-build; WT=<slice worktree>. Run every test/gate via scripts/asdlc/gate_digest.py (gate-digest skill).
