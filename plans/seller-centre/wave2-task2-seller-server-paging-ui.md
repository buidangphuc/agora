# [W2-T2] seller-server-paging-ui

## Role
feature-run worker (change `openspec/changes/seller-server-paging-ui`)

## Objective
Implement OpenSpec change `seller-server-paging-ui` test-first per its tasks.md; context in plans/seller-centre/00-spec.md and the decisions in plans/seller-centre/LOCK.md.
- Seller Centre `/seller/orders` and `/seller` use server paging/filter/search (`listSellerOrdersPage`, `listMyListings` with `statuses`/`query`), cursor-mode `SellerPagination`, `status_counts` badges/KPIs, per-block degradation; shared `ListingPicker` + `searchMyListingsAction` wired into ads and bundles (later slices reuse them).
- depends_on: order-seller-list-paging, domain-my-listings-filter
- base: `asdlc/seller-centre` after wave 1

## Write-set (EXCLUSIVE)
`?` stands for a `(`, `)`, `[` or `]` in a Next.js route segment (validate_plan.py cuts entries at `(`).
- team-frontend-seller/src/lib/gateway/orders.ts
- team-frontend-seller/src/lib/gateway/orders.test.ts
- team-frontend-seller/src/lib/gateway/listings.ts
- team-frontend-seller/src/lib/gateway/listings.test.ts
- team-frontend-seller/src/features/seller/**
- team-frontend-seller/src/features/order/**
- team-frontend-seller/src/app/seller/?dashboard?/**
- team-frontend-seller/src/app/seller/orders/**
- team-frontend-seller/src/app/seller/ads/**
- team-frontend-seller/src/app/seller/bundles/**
- team-frontend-seller/FEATURES.yaml
- team-frontend-seller/README.md
- platform-e2e/tests/e2e/features/seller/seller_server_paging.feature
- platform-e2e/tests/e2e/step_definitions/seller_server_paging_steps.py
- platform-e2e/tests/e2e/step_definitions/test_seller_server_paging.py
- platform-e2e/src/pages/seller_orders_page.py
- platform-e2e/src/pages/seller_listings_page.py
- platform-e2e/src/pages/seller_dashboard_page.py
- platform-e2e/src/pages/components/listing_picker.py

## Verify
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py vitest team-frontend-seller`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py tsc team-frontend-seller`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py biome team-frontend-seller`
- e2e: seller_server_paging feature against env seller-centre (team-frontend-seller, team-gateway, team-order, team-domain, team-payment, team-identity)
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py spec-sync . seller-server-paging-ui` and `openspec validate seller-server-paging-ui --strict`
