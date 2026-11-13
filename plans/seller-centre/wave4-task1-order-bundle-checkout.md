# [W4-T1] order-bundle-checkout

## Role
feature-run worker (change `openspec/changes/order-bundle-checkout`)

## Objective
Implement OpenSpec change `order-bundle-checkout` test-first per its tasks.md; context in plans/seller-centre/00-spec.md and the decisions in plans/seller-centre/LOCK.md.
- `CartService.AddBundleToCart` (GetBundle + concurrent GetListing checks, pro-rata allocation with remainder on the last line, atomic write); bundle tag dropped on line edit/remove with merge; bundle step in `precheckPrices` for PreviewCheckout and CreateOrder; migration `*_bundle_lines`; gateway `edge/cart.go` forwarder.
- depends_on: seller-centre-contract, domain-bundle-integrity
- base: `asdlc/seller-centre` after wave 3

## Write-set (EXCLUSIVE)
- team-order/internal/**
- team-order/migrations/*_bundle_lines.up.sql
- team-order/migrations/*_bundle_lines.down.sql
- team-order/migrations/migration_test.go
- team-order/FEATURES.yaml
- team-order/README.md
- team-gateway/internal/edge/cart.go
- team-gateway/internal/edge/cart_bundle_test.go
- team-gateway/README.md
- platform-e2e/tests/e2e/features/order/bundle_checkout.feature
- platform-e2e/tests/e2e/step_definitions/bundle_checkout_steps.py
- platform-e2e/tests/e2e/step_definitions/test_bundle_checkout.py
- platform-e2e/src/api/services/cart_service.py
- platform-e2e/src/api/services/order_service.py
- platform-e2e/src/api/services/listing_service.py
- widened (P1):
  - platform-e2e/tests/e2e/step_definitions/order_bundle_checkout_steps.py — e2e step file for bundle_checkout.feature (named after the slice instead of bundle_checkout_steps.py)
  - platform-e2e/tests/e2e/step_definitions/test_order_bundle_checkout.py — e2e pytest-bdd scenario binding for bundle_checkout.feature (named after the slice instead of test_bundle_checkout.py)

## Verify
- `DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-order --vet`
- `DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-gateway --vet` (stubs: `buf generate --template buf.gen.local.yaml` first)
- e2e: bundle_checkout feature against env seller-centre (team-gateway, team-order, team-domain, team-payment, team-promotion, team-identity)
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py spec-sync . order-bundle-checkout` and `openspec validate order-bundle-checkout --strict`
