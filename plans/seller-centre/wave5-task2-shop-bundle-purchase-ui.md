# [W5-T2] shop-bundle-purchase-ui

## Role
feature-run worker (change `openspec/changes/shop-bundle-purchase-ui`)

## Objective
Implement OpenSpec change `shop-bundle-purchase-ui` test-first per its tasks.md; context in plans/seller-centre/00-spec.md and the decisions in plans/seller-centre/LOCK.md.
- "Thêm combo vào giỏ" on `/shop/[id]` (`BundleCard`, `AddBundleButton`, `addBundleToCartAction`, availability helper); combo grouping and "Giảm giá combo" on `/cart` and the `/checkout` summary; bundle-drop notice. Featured/theme code from shop-page-featured-theme stays untouched.
- depends_on: order-bundle-checkout, shop-page-featured-theme
- base: `asdlc/seller-centre` after wave 4

## Write-set (EXCLUSIVE)
`?` stands for a `(`, `)`, `[` or `]` in a Next.js route segment (validate_plan.py cuts entries at `(`).
- team-frontend/src/lib/gateway/cart.ts
- team-frontend/src/lib/gateway/cart.test.ts
- team-frontend/src/features/cart/**
- team-frontend/src/features/shop/**
- team-frontend/src/app/?shop?/shop/?id?/**
- team-frontend/src/app/?shop?/cart/**
- team-frontend/src/app/?checkout?/checkout/**
- team-frontend/package-lock.json
- team-frontend/vendor/**
- team-frontend/FEATURES.yaml
- team-frontend/README.md
- platform-e2e/tests/e2e/features/frontend/shop_bundle_purchase_ui.feature
- platform-e2e/tests/e2e/step_definitions/shop_bundle_purchase_ui_steps.py
- platform-e2e/tests/e2e/step_definitions/test_shop_bundle_purchase_ui.py
- platform-e2e/src/pages/shop_profile_page.py
- platform-e2e/src/pages/cart_page.py
- platform-e2e/src/pages/checkout_page.py
- platform-e2e/src/api/services/listing_service.py

## Verify
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py vitest team-frontend`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py tsc team-frontend`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py biome team-frontend`
- `npm --prefix team-frontend run check`
- e2e: shop_bundle_purchase_ui feature against env seller-centre (team-frontend, team-gateway, team-order, team-domain, team-payment, team-promotion, team-identity)
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py spec-sync . shop-bundle-purchase-ui` and `openspec validate shop-bundle-purchase-ui --strict`
