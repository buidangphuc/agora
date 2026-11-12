# [W1-T2] order-cod-sales-count

## Role
feature-run worker (change `openspec/changes/order-cod-sales-count`)

## Objective
Implement OpenSpec change `order-cod-sales-count` test-first per its tasks.md; context in plans/discovery-followups/00-spec.md.
- Count COD orders into `sold_count` on SHIPPED→COMPLETED inside `UpdateOrderStatusFrom`; publish `ListingSalesChanged`; migration `0024_listing_sales_cod` backfills COMPLETED COD orders idempotently.
- depends_on: none (runs in wave 1 with the others)
- base: `asdlc/discovery-followups` after wave 0

## Write-set (EXCLUSIVE)
- team-order/internal/store/**
- team-order/internal/rpc/*_test.go
- team-order/internal/order/*_test.go
- team-order/migrations/0024_listing_sales_cod.up.sql
- team-order/migrations/0024_listing_sales_cod.down.sql
- team-order/migrations/*_test.go
- team-order/README.md
- team-order/FEATURES.yaml
- platform-e2e/tests/e2e/features/order/cod_sales_count.feature
- platform-e2e/tests/e2e/step_definitions/cod_sales_count_steps.py
- platform-e2e/tests/e2e/step_definitions/test_cod_sales_count.py

## Verify
- `cd team-order && DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 go test ./...`
- e2e: cod_sales_count feature against env discovery-followups (team-order, team-search, team-domain)
