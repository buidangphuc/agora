# [W2-T1] frontend-review-integrity

## Role
feature-run worker (change `openspec/changes/frontend-review-integrity`)

## Objective
Implement OpenSpec change `frontend-review-integrity` test-first per its tasks.md (scenario ids there); context in plans/q2-review-integrity/00-spec.md and the change's design.md Clarifications. Imports (never edits) platform-e2e/tests/e2e/support/review_order_support.py.

## Write-set (EXCLUSIVE)
- team-frontend/src/lib/gateway/reviews.ts
- team-frontend/src/lib/gateway/reviews.test.ts
- team-frontend/src/features/review/**
- team-frontend/src/features/order/OrderActions.tsx
- team-frontend/src/features/order/OrderActions.test.tsx
- team-frontend/src/app/(shop)/listing/[id]/page.tsx
- team-frontend/src/app/(shop)/listing/[id]/page.test.tsx
- team-frontend/FEATURES.yaml
- platform-e2e/tests/e2e/features/frontend/product_detail_ui.feature
- platform-e2e/tests/e2e/features/frontend/review_integrity_ui.feature
- platform-e2e/tests/e2e/step_definitions/d1_pdp_steps.py
- platform-e2e/tests/e2e/step_definitions/uif_pdp_steps.py
- platform-e2e/tests/e2e/step_definitions/review_integrity_steps.py
- platform-e2e/tests/e2e/step_definitions/test_review_integrity_ui.py

## Read-only dependencies
- openspec/changes/frontend-review-integrity/** (frozen by LOCK.md digest)
- platform-e2e/tests/e2e/support/review_order_support.py, platform-e2e/tests/e2e/step_definitions/rvp_steps.py (engagement-review-integrity output)
- team-frontend/proto/** (contract output)

## Verify
(cd team-frontend && npx vitest run && npx tsc --noEmit && npm run lint); (cd platform-e2e && HEADLESS=true .venv/bin/python -m pytest -q tests/e2e/features/frontend/review_integrity_ui.feature tests/e2e/features/frontend/product_detail_ui.feature tests/e2e/features/frontend/rvp_reviews_pagination_ui.feature tests/e2e/features/frontend/uif_product_detail.feature); platform-e2e/.venv/bin/python platform-e2e/scripts/spec_sync.py frontend-review-integrity --strict
