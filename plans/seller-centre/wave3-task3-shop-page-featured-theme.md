# [W3-T3] shop-page-featured-theme

## Role
feature-run worker (change `openspec/changes/archive/2026-11-17-shop-page-featured-theme`)

## Objective
Implement OpenSpec change `shop-page-featured-theme` test-first per its tasks.md; context in plans/seller-centre/00-spec.md and the decisions in plans/seller-centre/LOCK.md.
- Buyer `/shop/[id]` shows a featured block (stored order, ≤ 6, unpublished/foreign/missing skipped) and themes the hero via a closed theme table (unknown → default, AA contrast); page renders without a storefront; bundles block untouched.
- depends_on: domain-storefront-customisation
- base: `asdlc/seller-centre` after wave 2

## Write-set (EXCLUSIVE)
`?` stands for a `(`, `)`, `[` or `]` in a Next.js route segment (validate_plan.py cuts entries at `(`).
- team-frontend/src/features/shop/**
- team-frontend/src/app/?shop?/shop/?id?/**
- team-frontend/src/lib/gateway/shops.ts
- team-frontend/src/lib/gateway/shops.test.ts
- team-frontend/FEATURES.yaml
- team-frontend/README.md
- platform-e2e/tests/e2e/features/frontend/shop_storefront_ui.feature
- platform-e2e/tests/e2e/step_definitions/shop_storefront_ui_steps.py
- platform-e2e/tests/e2e/step_definitions/test_shop_storefront_ui.py
- platform-e2e/src/pages/shop_profile_page.py

## Verify
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py vitest team-frontend`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py tsc team-frontend`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py biome team-frontend`
- `npm --prefix team-frontend run check`
- e2e: shop_storefront_ui feature against env seller-centre (team-frontend, team-gateway, team-domain, team-search, team-identity)
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py spec-sync . shop-page-featured-theme` and `openspec validate shop-page-featured-theme --strict`
