# [W2-T1] order-advertised-price

## Role
feature-run worker (change `openspec/changes/order-advertised-price`)

## Objective
Implement OpenSpec change `order-advertised-price` test-first per its tasks.md; context in plans/q5-advertised-price/00-spec.md and design.md Clarifications.
- team-order is the price authority: flash price at add-to-cart, checkout re-check (FAILED_PRECONDITION "price changed" + re-price), flash holds in the saga (Phase A reserve, Phase B commit before the domain commit, release on compensation/cancel).

## Write-set (EXCLUSIVE)
- team-order/proto/platform/promotion/v1/promotion.proto
- team-order/generated/**
- team-order/internal/upstream/promotion.go
- team-order/internal/upstream/promotion_test.go
- team-order/internal/service/**
- team-order/internal/repository/cart.go
- team-order/internal/repository/*_test.go
- team-order/internal/handler/order.go
- team-order/internal/handler/*_test.go
- team-order/internal/consumer/payment_flash_test.go
- team-order/cmd/server/main.go
- team-order/README.md
- team-order/FEATURES.yaml
- platform-e2e/tests/e2e/support/flash_support.py
- platform-e2e/tests/e2e/features/order/advertised_price.feature
- platform-e2e/tests/e2e/step_definitions/test_order_advertised_price.py
- platform-e2e/tests/e2e/step_definitions/order_advertised_price_steps.py

## Read-only dependencies
- openspec/changes/order-advertised-price/** (frozen by LOCK.md digest)
- platform-e2e/tests/e2e/support/flash_hold_support.py (from promotion-flash-stock-hold; reuse)
- plans/q5-advertised-price/00-spec.md, LOCK.md

## Verify
(cd team-order && go vet ./... && go test ./... -race); (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_order_advertised_price.py); $PY platform-e2e/scripts/spec_sync.py order-advertised-price --strict
