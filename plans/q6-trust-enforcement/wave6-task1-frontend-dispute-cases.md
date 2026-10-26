# [W6-T1] frontend-dispute-cases

## Role
feature-run worker (change `openspec/changes/archive/2026-10-26-frontend-dispute-cases`)

## Objective
Implement OpenSpec change `frontend-dispute-cases` test-first per its tasks.md (scenario ids there); context in plans/q6-trust-enforcement/00-spec.md and the change's design.md Clarifications.
- HELD until q10-order-lifecycle merges to main. Buyer escalates a return to a dispute from the order page and sees the order's disputes; admin disputes tab in /admin/cases (investigate, rule buyer/seller favour with confirmation, reject).

**HELD:** build only after q10-order-lifecycle has merged to main.
- depends_on: frontend-listing-moderation, order-dispute-refund

## Write-set (EXCLUSIVE)
- team-frontend/src/generated/platform/engagement/**
- team-frontend/src/generated/platform/order/**
- team-frontend/src/lib/gateway/orders.ts
- team-frontend/src/lib/gateway/orders.test.ts
- team-frontend/src/lib/gateway/disputes.ts
- team-frontend/src/lib/gateway/disputes.test.ts
- team-frontend/src/features/dispute/**
- team-frontend/src/features/order/ReturnRequestSection.tsx
- team-frontend/src/features/order/ReturnRequestSection.test.tsx
- team-frontend/src/features/order/OrderDetailView.tsx
- team-frontend/src/features/order/OrderDetailView.test.tsx
- team-frontend/src/app/(shop)/account/orders/[id]/page.tsx
- team-frontend/src/app/(shop)/account/orders/[id]/page.test.tsx
- team-frontend/src/features/admin/DisputeCasesPanel.tsx
- team-frontend/src/features/admin/DisputeCasesPanel.test.tsx
- team-frontend/src/app/(shop)/admin/cases/page.tsx
- team-frontend/src/app/(shop)/admin/cases/page.test.tsx
- team-frontend/FEATURES.yaml
- platform-e2e/tests/e2e/features/frontend/dispute_cases_ui.feature
- platform-e2e/tests/e2e/step_definitions/dispute_cases_steps.py
- platform-e2e/tests/e2e/step_definitions/test_dispute_cases_ui.py
- platform-e2e/tests/e2e/support/dispute_cases_ui_support.py
- openspec/changes/archive/2026-10-26-frontend-dispute-cases/**
- team-frontend/src/features/order/OrderActions.test.tsx (# widened at build: task 1.7)
- team-frontend/src/features/order/OrderDetailTabs.test.tsx (# widened at build: task 1.7)
- team-frontend/src/features/order/returns.actions.test.ts (# widened at build: task 1.7)
- team-frontend/src/features/seller/SellerReturns.test.tsx (# widened at build: task 1.7)
- team-frontend/src/features/seller/returnActions.test.ts (# widened at build: task 1.7)
- plans/q6-trust-enforcement/wave6-task1-frontend-dispute-cases.md (# widened at build: task 1.7; packet widening record)

## Read-only dependencies
- openspec/changes/archive/2026-10-26-frontend-dispute-cases/** is frozen by the LOCK.md digest (only task ticks change)
- team-frontend/proto/** (never edit)
- platform-e2e/tests/e2e/support/roe_support.py, oic_order_support.py, audit_support.py, q10 return-window helpers (never edit)
- plans/q6-trust-enforcement/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-frontend && npm run proto && npm test && npm run check); (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_dispute_cases_ui.py); $PY platform-e2e/scripts/spec_sync.py frontend-dispute-cases --strict
