# [W5-T1] seller-getting-started

## Role
feature-run worker (change `openspec/changes/seller-getting-started`)

## Objective
Implement OpenSpec change `seller-getting-started` test-first per its tasks.md; context in plans/seller-centre/00-spec.md and the decisions in plans/seller-centre/LOCK.md.
- `GettingStartedCard` on `/seller`: four steps (shop name, banner/featured, first listing, first voucher or flash sale) as done / not done / chưa rõ with a 3 s per-source timeout; hidden when all done; seller id from `getPrincipal()` only.
- depends_on: seller-marketing-hub, seller-shop-customisation-ui
- base: `asdlc/seller-centre` after wave 4

## Write-set (EXCLUSIVE)
`?` stands for a `(`, `)`, `[` or `]` in a Next.js route segment (validate_plan.py cuts entries at `(`).
- team-frontend-seller/src/features/seller/**
- team-frontend-seller/src/app/seller/?dashboard?/**
- team-frontend-seller/FEATURES.yaml
- team-frontend-seller/README.md
- platform-e2e/tests/e2e/features/frontend/seller_getting_started.feature
- platform-e2e/tests/e2e/step_definitions/seller_getting_started_steps.py
- platform-e2e/tests/e2e/step_definitions/test_seller_getting_started.py
- platform-e2e/src/pages/seller_dashboard_page.py
- platform-e2e/src/pages/seller_getting_started_card.py (widened at build: P1, e2e page object of this change (SellerGettingStartedPage), named differently from the packet)
- platform-e2e/src/pages/__init__.py (widened at build: P1, e2e page-object registration of this change's SellerGettingStartedPage)

## Verify
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py vitest team-frontend-seller`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py tsc team-frontend-seller`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py biome team-frontend-seller`
- `npm --prefix team-frontend-seller run vendor:check`
- e2e: seller_getting_started feature against env seller-centre (team-frontend-seller, team-gateway, team-domain, team-promotion, team-identity)
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py spec-sync . seller-getting-started` and `openspec validate seller-getting-started --strict` and `scripts/asdlc/spec_lint.py seller-getting-started`
