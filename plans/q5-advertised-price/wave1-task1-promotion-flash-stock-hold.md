# [W1-T1] promotion-flash-stock-hold

## Role
feature-run worker (change `openspec/changes/promotion-flash-stock-hold`)

## Objective
Implement OpenSpec change `promotion-flash-stock-hold` test-first per its tasks.md; context in plans/q5-advertised-price/00-spec.md and design.md Clarifications.
- team-promotion holds flash stock: reserve/commit/release service-only RPCs, 15-minute expiry, release returns committed units, stock meter counts holds, 30-day purge of released/expired holds.

## Write-set (EXCLUSIVE)
- team-promotion/migrations/0004_flash_sale_holds.up.sql
- team-promotion/migrations/0004_flash_sale_holds.down.sql
- team-promotion/generated/**
- team-promotion/internal/repository/**
- team-promotion/internal/service/flashsale.go
- team-promotion/internal/service/flashsale_test.go
- team-promotion/internal/service/flashsale_hold_test.go
- team-promotion/internal/handler/flashsale.go
- team-promotion/internal/handler/*_test.go
- team-promotion/internal/bootstrap/resources.go
- team-promotion/cmd/server/main.go
- team-promotion/README.md
- team-promotion/FEATURES.yaml
- platform-e2e/tests/e2e/support/flash_hold_support.py
- platform-e2e/tests/e2e/features/promo/flash_sale_stock_hold.feature
- platform-e2e/tests/e2e/step_definitions/flash_sale_stock_hold_steps.py
- platform-e2e/tests/e2e/step_definitions/test_flash_sale_stock_hold.py

## Read-only dependencies
- openspec/changes/promotion-flash-stock-hold/** (frozen by LOCK.md digest)
- team-promotion voucher reserve/commit/release code and tests (pattern)
- plans/q5-advertised-price/00-spec.md, LOCK.md

## Verify
make -C team-promotion check; (cd team-promotion && go test ./... -race); (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_flash_sale_stock_hold.py); $PY platform-e2e/scripts/spec_sync.py promotion-flash-stock-hold --strict
