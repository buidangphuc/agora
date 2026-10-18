# [W3-T1] frontend-advertised-price

## Role
feature-run worker (change `openspec/changes/archive/2026-10-18-frontend-advertised-price`)

## Objective
Implement OpenSpec change `frontend-advertised-price` test-first per its tasks.md; context in plans/q5-advertised-price/00-spec.md and design.md Clarifications.
- PDP shipping row from CalculateShippingFee (fabricated SPX/insurance text removed), checkout shipping from the server, price-changed UX.

## Write-set (EXCLUSIVE)
- team-frontend/src/lib/gateway/orders.ts
- team-frontend/src/lib/gateway/orders.test.ts
- team-frontend/src/lib/action-result.ts
- team-frontend/src/features/order/**
- team-frontend/src/features/listing/ShippingInfo.tsx
- team-frontend/src/features/listing/ShippingInfo.test.tsx
- team-frontend/src/features/listing/fabricatedText.ts
- team-frontend/src/app/(shop)/listing/[id]/page.tsx
- team-frontend/src/app/(shop)/listing/[id]/page.test.tsx
- team-frontend/src/app/(checkout)/checkout/**
- team-frontend/FEATURES.yaml
- team-gateway/internal/edge/order.go
- team-gateway/internal/edge/order_test.go
- platform-e2e/tests/e2e/support/advertised_price_ui_support.py
- platform-e2e/tests/e2e/features/frontend/advertised_price_ui.feature
- platform-e2e/tests/e2e/step_definitions/advertised_price_ui_steps.py
- platform-e2e/tests/e2e/step_definitions/test_advertised_price_ui.py

## Read-only dependencies
- openspec/changes/archive/2026-10-18-frontend-advertised-price/** (frozen by LOCK.md digest)
- platform-e2e/tests/e2e/support/flash_support.py, flash_hold_support.py (reuse)
- plans/q5-advertised-price/00-spec.md, LOCK.md

## Verify
(cd team-frontend && npx vitest run && npx tsc --noEmit && npx biome check src); (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_advertised_price_ui.py); $PY platform-e2e/scripts/spec_sync.py frontend-advertised-price --strict
