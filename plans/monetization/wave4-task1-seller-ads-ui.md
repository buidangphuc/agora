# [W4-T1] seller-ads-ui

## Role
feature-run worker (change `openspec/changes/seller-ads-ui`)

## Objective
Implement OpenSpec change `seller-ads-ui` test-first per its tasks.md; context in plans/monetization/00-spec.md (decisions in LOCK.md, defaults in brackets apply).
- team-frontend-seller `/seller/ads` lists campaigns from `ListMyAdCampaigns` (page 20, spend and counters, remaining = max(0, budget - spent)), pause/resume actions, bid >= 500 and budget >= bid validation; drops the client-only "Chiến dịch vừa tạo" list.
- depends_on: promotion-ads-campaign-mgmt
- base: `asdlc/monetization` after wave 3
- hot file: team-frontend-seller src/lib/gateway/promotion.ts, features/seller/actions.ts and FEATURES.yaml shared with seller-plan-ui (wave 5)

## Write-set (EXCLUSIVE)
- team-frontend-seller/src/lib/gateway/promotion.ts
- team-frontend-seller/src/lib/gateway/promotion.test.ts
- team-frontend-seller/src/features/seller/actions.ts
- team-frontend-seller/src/features/seller/actions.test.ts
- team-frontend-seller/src/features/seller/AdCampaignTable.tsx
- team-frontend-seller/src/features/seller/AdCampaignTable.test.tsx
- team-frontend-seller/src/features/seller/AdCampaignForm.tsx
- team-frontend-seller/src/features/seller/SellerForms.test.tsx
- team-frontend-seller/src/features/listing/validation.ts
- team-frontend-seller/src/app/seller/ads/**
- team-frontend-seller/README.md
- team-frontend-seller/FEATURES.yaml
- platform-e2e/tests/e2e/features/frontend/seller_ads_ui.feature
- platform-e2e/tests/e2e/step_definitions/seller_ads_ui_steps.py
- platform-e2e/tests/e2e/support/seller_ads_ui_support.py
- platform-e2e/tests/e2e/step_definitions/test_seller_ads_ui.py

## Verify
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py vitest team-frontend-seller`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py tsc team-frontend-seller`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py biome team-frontend-seller`
- `cd team-frontend-seller && npm run check`
- e2e: frontend/seller_ads_ui.feature against env monetization (team-frontend-seller, team-frontend, team-gateway, team-promotion, team-domain)
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py spec-sync . seller-ads-ui`
