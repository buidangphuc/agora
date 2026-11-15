# [W6-T1] seller-bulk-ship

## Role
feature-run worker (change `openspec/changes/seller-bulk-ship`)

## Objective
Implement OpenSpec change `seller-bulk-ship` test-first per its tasks.md; context in plans/seller-centre/00-spec.md and the decisions in plans/seller-centre/LOCK.md.
- Select shippable rows on `/seller/orders` (≤ 50, current page), `bulkShipOrdersAction` (4 in flight, 15 s budget, per-row results), failed rows stay selected; `/seller/orders/print` prints one slip per order (no-store, noindex).
- depends_on: seller-server-paging-ui
- base: `asdlc/seller-centre` after wave 5

## Write-set (EXCLUSIVE)
- team-frontend-seller/src/features/seller/**
- team-frontend-seller/src/features/order/**
- team-frontend-seller/src/lib/gateway/orders.ts
- team-frontend-seller/src/lib/gateway/orders.test.ts
- team-frontend-seller/src/app/seller/orders/**
- team-frontend-seller/FEATURES.yaml
- team-frontend-seller/README.md
- platform-e2e/tests/e2e/features/frontend/seller_bulk_ship.feature
- platform-e2e/tests/e2e/step_definitions/seller_bulk_ship_steps.py
- platform-e2e/tests/e2e/step_definitions/test_seller_bulk_ship.py
- platform-e2e/src/pages/seller_orders_page.py
- platform-e2e/src/pages/seller_print_slips_page.py
- platform-e2e/src/constants/routes.py
- platform-e2e/src/pages/seller_bulk_ship_page.py (widened at build: P1, e2e page object of this change (bulk-ship bar/modal), named differently from the packet)

## Verify
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py vitest team-frontend-seller`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py tsc team-frontend-seller`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py biome team-frontend-seller`
- `npm --prefix team-frontend-seller run vendor:check`
- e2e: seller_bulk_ship feature against env seller-centre (team-frontend-seller, team-gateway, team-order, team-domain, team-payment, team-identity)
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py spec-sync . seller-bulk-ship` and `openspec validate seller-bulk-ship --strict` and `scripts/asdlc/spec_lint.py seller-bulk-ship`
