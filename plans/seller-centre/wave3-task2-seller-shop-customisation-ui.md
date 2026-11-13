# [W3-T2] seller-shop-customisation-ui

## Role
feature-run worker (change `openspec/changes/seller-shop-customisation-ui`)

## Objective
Implement OpenSpec change `seller-shop-customisation-ui` test-first per its tasks.md; context in plans/seller-centre/00-spec.md and the decisions in plans/seller-centre/LOCK.md.
- `/seller/shop` edits banner (upload via `getUploadUrlAction`), tagline, theme and up to 6 ordered featured listings; `upsertStorefrontAction` reads then sends the full storefront, seller id from the session; picker reuses `ListingPicker`/`searchMyListingsAction` from seller-server-paging-ui.
- depends_on: domain-storefront-customisation, domain-my-listings-filter
- base: `asdlc/seller-centre` after wave 2

## Write-set (EXCLUSIVE)
- team-frontend-seller/src/features/seller/**
- team-frontend-seller/src/lib/gateway/listings.ts
- team-frontend-seller/src/lib/gateway/listings.test.ts
- team-frontend-seller/src/lib/gateway/shops.ts
- team-frontend-seller/src/lib/gateway/shops.test.ts
- team-frontend-seller/src/app/seller/shop/**
- team-frontend-seller/FEATURES.yaml
- team-frontend-seller/README.md
- platform-e2e/tests/e2e/features/frontend/seller_shop_customisation_ui.feature
- platform-e2e/tests/e2e/step_definitions/seller_shop_customisation_ui_steps.py
- platform-e2e/tests/e2e/step_definitions/test_seller_shop_customisation_ui.py
- platform-e2e/src/pages/seller_shop_page.py

## Verify
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py vitest team-frontend-seller`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py tsc team-frontend-seller`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py biome team-frontend-seller`
- `npm --prefix team-frontend-seller run vendor:check`
- e2e: seller_shop_customisation_ui feature against env seller-centre (team-frontend-seller, team-frontend, team-gateway, team-domain, team-identity, minio)
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py spec-sync . seller-shop-customisation-ui` and `openspec validate seller-shop-customisation-ui --strict`
