# [W5-T1] seller-plan-ui

## Role
feature-run worker (change `openspec/changes/archive/2026-11-17-seller-plan-ui`)

## Objective
Implement OpenSpec change `seller-plan-ui` test-first per its tasks.md; context in plans/monetization/00-spec.md (decisions in LOCK.md, defaults in brackets apply).
- team-frontend-seller `/seller/plans`: prices per term, period end and renewal state, upgrade/downgrade dialogs, `CancelSubscriptionButton`, FAILED_PRECONDITION shown as insufficient balance; `ListingForm` shows the upgrade link on `PLAN_LISTING_LIMIT`.
- Button wording "Nâng cấp" / "Hạ gói"; update `ui_seller_orders_money.feature` accordingly.
- depends_on: promotion-plan-billing, listing-plan-cap, seller-ads-ui (hot file, serialised)
- base: `asdlc/monetization` after wave 4
- hot file: team-frontend-seller promotion.ts, actions.ts, FEATURES.yaml (after seller-ads-ui)

## Write-set (EXCLUSIVE)
- team-frontend-seller/src/lib/gateway/promotion.ts
- team-frontend-seller/src/lib/gateway/promotion.test.ts
- team-frontend-seller/src/features/seller/actions.ts
- team-frontend-seller/src/features/seller/actions.test.ts
- team-frontend-seller/src/features/seller/SubscribeButton.tsx
- team-frontend-seller/src/features/seller/SubscribeButton.test.tsx
- team-frontend-seller/src/features/seller/CancelSubscriptionButton.tsx
- team-frontend-seller/src/features/seller/CancelSubscriptionButton.test.tsx
- team-frontend-seller/src/features/listing/ListingForm.tsx
- team-frontend-seller/src/features/listing/ListingForm.test.tsx
- team-frontend-seller/src/features/listing/actions.ts
- team-frontend-seller/src/features/listing/actions.test.ts
- team-frontend-seller/src/app/seller/plans/**
- team-frontend-seller/README.md
- team-frontend-seller/FEATURES.yaml
- platform-e2e/tests/e2e/features/seller/seller_plans.feature
- platform-e2e/tests/e2e/features/seller/create-listing.feature
- platform-e2e/tests/e2e/features/frontend/ui_seller_orders_money.feature
- platform-e2e/tests/e2e/step_definitions/seller_plans_steps.py
- platform-e2e/tests/e2e/support/seller_plans_support.py
- platform-e2e/tests/e2e/step_definitions/test_seller_plans.py
- platform-e2e/tests/e2e/step_definitions/test_seller.py
- platform-e2e/tests/e2e/step_definitions/d2_seller_steps.py
- widened (P1):
  - platform-e2e/tests/e2e/step_definitions/seller_plan_ui_steps.py — e2e step file of this change (named differently from the packet's seller_plans_steps.py)
  - team-frontend-seller/src/app/seller/accounts.test.tsx — existing test directly broken: plans page now calls getMySubscription and labels the upgrade button "Nâng cấp"

## Verify
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py vitest team-frontend-seller`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py tsc team-frontend-seller`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py biome team-frontend-seller`
- `cd team-frontend-seller && npm run check`
- e2e: seller/seller_plans.feature, seller/create-listing.feature, frontend/ui_seller_orders_money.feature against env monetization (team-frontend-seller, team-gateway, team-promotion, team-payment, team-domain)
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py spec-sync . seller-plan-ui`
