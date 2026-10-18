# [W2-T1] frontend-loop-events

## Role
feature-run worker (change `openspec/changes/archive/2026-10-18-frontend-loop-events`)

## Objective
Implement OpenSpec change `frontend-loop-events` test-first per its tasks.md (scenario ids there); context in plans/q4-loop-instrumentation/00-spec.md and the change's design.md Clarifications.
- Shop UI fires sign_up/login (one-shot agora_auth_evt cookie), favorite (GA4 add_to_wishlist), share, follow_seller, referral_redeem, daily_check_in, alert_subscribe through trackEcommerce after server success.

## Write-set (EXCLUSIVE)
- team-frontend/src/lib/analytics/schema.ts
- team-frontend/src/lib/analytics/map.ts
- team-frontend/src/lib/analytics/map.test.ts
- team-frontend/src/lib/analytics/dataLayer.ts
- team-frontend/src/lib/analytics/dataLayer.test.ts
- team-frontend/src/lib/analytics/dataLayer.privacy.test.ts
- team-frontend/src/features/auth/actions.ts
- team-frontend/src/features/auth/actions.test.ts
- team-frontend/src/features/auth/AuthEventBeacon.tsx
- team-frontend/src/features/auth/AuthEventBeacon.test.tsx
- team-frontend/src/app/(shop)/layout.tsx
- team-frontend/src/features/engagement/FavoriteButton.tsx
- team-frontend/src/features/engagement/FavoriteButton.test.tsx
- team-frontend/src/features/listing/ShareButton.tsx
- team-frontend/src/features/listing/ShareButton.test.tsx
- team-frontend/src/features/engagement/FollowSellerButton.tsx
- team-frontend/src/features/engagement/FollowSellerButton.test.tsx
- team-frontend/src/features/account/referral/RedeemReferralForm.tsx
- team-frontend/src/features/account/referral/RedeemReferralForm.test.tsx
- team-frontend/src/features/engagement/LoyaltyWidget.tsx
- team-frontend/src/features/engagement/LoyaltyWidget.test.tsx
- team-frontend/src/components/alerts/AlertToggle.tsx
- team-frontend/src/components/alerts/AlertToggle.test.tsx
- team-frontend/FEATURES.yaml
- platform-e2e/tests/e2e/support/loop_events_support.py
- platform-e2e/tests/e2e/features/tracking/loop_events.feature
- platform-e2e/tests/e2e/step_definitions/loop_events_steps.py
- platform-e2e/tests/e2e/step_definitions/test_loop_events.py

## Read-only dependencies
- openspec/changes/archive/2026-10-18-frontend-loop-events/** (frozen by LOCK.md digest)
- team-frontend/src/lib/analytics/dispatcher.ts (unchanged)
- plans/q4-loop-instrumentation/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-frontend && npx vitest run && npx tsc --noEmit && npx prettier --check src); (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_loop_events.py); $PY platform-e2e/scripts/spec_sync.py frontend-loop-events --strict
