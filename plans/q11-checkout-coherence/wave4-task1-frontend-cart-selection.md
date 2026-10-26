# [W4-T1] frontend-cart-selection

## Role
feature-run worker (change `openspec/changes/archive/2026-10-26-frontend-cart-selection`)

## Objective
Implement OpenSpec change `frontend-cart-selection` test-first per its tasks.md (scenario ids "Cart selection: ..."); context in plans/q11-checkout-coherence/00-spec.md and the change's design.md Clarifications.
- Cart line and shop checkboxes (all ticked by default) carried as `?items=` into checkout, preview and CreateOrder; "Mua ngay" checks out only the new line; analytics never carry line ids.
- depends_on: frontend-checkout-coherence (rebase on it; never hand-merge `(summary)/page.tsx`, `checkoutParams.ts`, `PlaceOrderForm.tsx`). `team-frontend/proto` is not edited here.

## Write-set (EXCLUSIVE)
- team-frontend/src/features/cart/selection.ts
- team-frontend/src/features/cart/selection.test.ts
- team-frontend/src/features/cart/CartSelection.tsx
- team-frontend/src/features/cart/CartSelection.test.tsx
- team-frontend/src/features/cart/CartGroups.tsx
- team-frontend/src/features/cart/CartGroups.test.tsx
- team-frontend/src/app/(shop)/cart/page.tsx
- team-frontend/src/app/(shop)/cart/page.test.tsx
- team-frontend/src/app/(checkout)/checkout/(summary)/page.tsx
- team-frontend/src/app/(checkout)/checkout/(summary)/page.test.tsx
- team-frontend/src/features/order/PlaceOrderForm.test.tsx
- team-frontend/src/features/order/checkoutParams.test.ts
- team-frontend/src/features/cart/actions.ts
- team-frontend/src/features/cart/actions.test.ts
- team-frontend/src/features/cart/PurchaseContext.tsx
- team-frontend/src/features/cart/PurchasePanel.test.tsx
- team-frontend/src/lib/analytics/destinations/gtm.ts
- team-frontend/src/lib/analytics/destinations/gtm.test.ts
- team-frontend/FEATURES.yaml
- platform-e2e/tests/e2e/features/frontend/cart_selection_ui.feature
- platform-e2e/tests/e2e/step_definitions/cart_selection_ui_steps.py
- platform-e2e/tests/e2e/step_definitions/test_cart_selection_ui.py
- platform-e2e/tests/e2e/support/cart_selection_ui_support.py
- openspec/changes/archive/2026-10-26-frontend-cart-selection/**
- team-frontend/src/features/order/OrderSummary.tsx (widened at build: task 1.2)
- team-frontend/src/features/order/OrderSummary.test.tsx (widened at build: task 1.2)

## Read-only dependencies
- openspec/changes/archive/2026-10-26-frontend-cart-selection/** is frozen by the LOCK.md digest (only task ticks change)
- team-frontend/src/features/order/checkoutParams.ts (`items` from frontend-checkout-coherence), team-frontend/src/features/cart/BuyBar.tsx, PurchasePanel.tsx
- platform-e2e `d2 buyer has a cart` steps, ui_cart_page.feature (regression "Cart page is not a client component" must stay green), cart_checkout_steps.py
- plans/q11-checkout-coherence/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-frontend && PATH=$PWD/node_modules/.bin:$PATH npx buf generate && npm test && npm run check); bash $BUILD/e2e.sh $WT tests/e2e/step_definitions/test_cart_selection_ui.py tests/e2e/step_definitions/test_d2_cart_page.py; $PY platform-e2e/scripts/spec_sync.py frontend-cart-selection --strict

Vars: PY=/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python; BUILD=/Users/phuc.buidang/Library/Caches/ai-first-runs/c4q11-build; WT=<slice worktree>. Run every test/gate via scripts/asdlc/gate_digest.py (gate-digest skill).
