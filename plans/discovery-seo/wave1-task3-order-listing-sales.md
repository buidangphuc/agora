# [W1-T3] order-listing-sales

## Role
feature-run worker (change `openspec/changes/order-listing-sales`)

## Objective
Implement OpenSpec change `order-listing-sales` test-first per its tasks.md; context in plans/discovery-seo/00-spec.md and the change's design.md.
- `listing_sales` + `listing_sales_orders` ledger updated inside the PAID, cancel-from-PAID and ReturnRefunded transactions; `ListingSalesChanged` (absolute sold count, key `listing_id`, deterministic event id) on the outbox; migration 0023 backfills and emits one event per listing; no client can set the count.
- depends_on: discovery-seo-contract (merged)
- base: `feat/discovery-seo` after wave 0 merged

## Write-set (EXCLUSIVE)
- team-order/internal/store/**
- team-order/internal/events/**
- team-order/internal/rpc/**
- team-order/internal/order/**
- team-order/migrations/0023_listing_sales.up.sql
- team-order/migrations/0023_listing_sales.down.sql
- team-order/migrations/embed.go
- team-order/migrations/embed_test.go
- team-order/migrations/migration_test.go
- team-order/README.md
- team-order/FEATURES.yaml
- platform-e2e/tests/e2e/features/order/listing_sales.feature
- platform-e2e/tests/e2e/step_definitions/order_listing_sales_steps.py
- platform-e2e/tests/e2e/step_definitions/test_order_listing_sales.py

## Verify
- `cd team-order && buf generate --template buf.gen.local.yaml && go vet ./... && go test ./...`
- e2e: `cd platform-e2e && HEADLESS=true $PY -m pytest tests/e2e/step_definitions/test_order_listing_sales.py -q` (PY=/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python)
