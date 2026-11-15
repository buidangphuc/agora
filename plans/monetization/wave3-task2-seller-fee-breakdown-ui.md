# [W3-T2] seller-fee-breakdown-ui

## Role
feature-run worker (change `openspec/changes/seller-fee-breakdown-ui`)

## Objective
Implement OpenSpec change `seller-fee-breakdown-ui` test-first per its tasks.md; context in plans/monetization/00-spec.md (decisions in LOCK.md, defaults in brackets apply).
- team-frontend-seller `getOrderSettlement` (2 s timeout, NOT_FOUND = not settled), `OrderSettlementCard` on `/seller/orders/[id]`, Vietnamese ledger labels on `/seller/wallet` with raw-string fallback.
- depends_on: payment-settlement-commission
- base: `asdlc/monetization` after wave 2
- hot file: team-frontend-seller FEATURES.yaml shared with seller-ads-ui (wave 4) and seller-plan-ui (wave 5); lib/gateway/payment.ts only this slice

## Write-set (EXCLUSIVE)
- team-frontend-seller/src/lib/gateway/payment.ts
- team-frontend-seller/src/lib/gateway/payment.test.ts
- team-frontend-seller/src/features/seller/OrderSettlementCard.tsx
- team-frontend-seller/src/features/seller/OrderSettlementCard.test.tsx
- team-frontend-seller/src/features/seller/ledgerLabels.ts
- team-frontend-seller/src/features/seller/ledgerLabels.test.ts
- team-frontend-seller/src/app/seller/orders/[id]/**
- team-frontend-seller/src/app/seller/wallet/**
- team-frontend-seller/README.md
- team-frontend-seller/FEATURES.yaml
- platform-e2e/tests/e2e/features/frontend/seller_fee_breakdown_ui.feature
- platform-e2e/tests/e2e/step_definitions/seller_fee_breakdown_ui_steps.py
- platform-e2e/tests/e2e/support/seller_fee_breakdown_ui_support.py
- platform-e2e/tests/e2e/step_definitions/test_seller_fee_breakdown_ui.py

### widened (P1)
- openspec/changes/seller-fee-breakdown-ui/specs/ui-seller/spec.md (this change's OpenSpec folder: VERIFIED BY citations matched to the real test names; no requirement or scenario wording change)

## Verify
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py vitest team-frontend-seller`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py tsc team-frontend-seller`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py biome team-frontend-seller`
- `cd team-frontend-seller && npm run check`
- e2e: frontend/seller_fee_breakdown_ui.feature against env monetization (team-frontend-seller, team-gateway, team-payment, team-order)
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py spec-sync . seller-fee-breakdown-ui`
