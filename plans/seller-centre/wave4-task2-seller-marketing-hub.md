# [W4-T2] seller-marketing-hub

## Role
feature-run worker (change `openspec/changes/seller-marketing-hub`)

## Objective
Implement OpenSpec change `seller-marketing-hub` test-first per its tasks.md; context in plans/seller-centre/00-spec.md and the decisions in plans/seller-centre/LOCK.md.
- `/seller/marketing` hub (voucher and flash-sale tabs, URL state, per-tab error) over the seller promotion RPCs; create/deactivate vouchers, create/cancel flash sales with `ListingPicker`; "Marketing" in `SELLER_NAV`.
- team-frontend: remove `VoucherManager`, `createVoucherAction` and the `canManage` branch from `/vouchers`.
- depends_on: promotion-seller-self-service, domain-my-listings-filter
- base: `asdlc/seller-centre` after wave 3

## Write-set (EXCLUSIVE)
`?` stands for a `(`, `)`, `[` or `]` in a Next.js route segment (validate_plan.py cuts entries at `(`).
- team-frontend-seller/src/lib/gateway/promotion.ts
- team-frontend-seller/src/lib/gateway/promotion.test.ts
- team-frontend-seller/src/features/seller/**
- team-frontend-seller/src/app/seller/marketing/**
- team-frontend-seller/FEATURES.yaml
- team-frontend-seller/README.md
- team-frontend/src/features/voucher/**
- team-frontend/src/app/?shop?/vouchers/**
- team-frontend/FEATURES.yaml
- platform-e2e/tests/e2e/features/seller/seller_marketing_hub.feature
- platform-e2e/tests/e2e/step_definitions/seller_marketing_hub_steps.py
- platform-e2e/tests/e2e/step_definitions/test_seller_marketing_hub.py
- platform-e2e/src/pages/seller_marketing_page.py
- platform-e2e/src/pages/vouchers_page.py
- platform-e2e/src/constants/routes.py
- platform-e2e/src/constants/pages.py
- widened (P1):
  - openspec/changes/seller-marketing-hub/specs/seller-marketing-hub/spec.md — own OpenSpec folder: VERIFIED BY citation lines only, no requirement or scenario wording change
  - platform-e2e/src/pages/__init__.py — e2e page-object registration of this change's SellerMarketingPage
  - platform-e2e/tests/e2e/support/seller_marketing_hub_support.py — e2e support helpers of this change's seller_marketing_hub feature

## Verify
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py vitest team-frontend-seller`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py tsc team-frontend-seller`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py biome team-frontend-seller`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py vitest team-frontend`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py tsc team-frontend`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py biome team-frontend`
- e2e: seller_marketing_hub feature against env seller-centre (team-frontend-seller, team-frontend, team-gateway, team-promotion, team-domain, team-identity)
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py spec-sync . seller-marketing-hub` and `openspec validate seller-marketing-hub --strict`
