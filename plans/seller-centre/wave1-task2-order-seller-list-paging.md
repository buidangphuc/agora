# [W1-T2] order-seller-list-paging

## Role
feature-run worker (change `openspec/changes/order-seller-list-paging`)

## Objective
Implement OpenSpec change `order-seller-list-paging` test-first per its tasks.md; context in plans/seller-centre/00-spec.md and the decisions in plans/seller-centre/LOCK.md.
- `ListSellerOrders` keyset paging (`page`, default 20, max 100), `statuses`, `query` (title substring / order-id prefix), filtered total and query-scoped `status_counts`; legacy unpaged call unchanged; migration `0025_orders_seller_status_created_idx`; query text never logged.
- depends_on: seller-centre-contract
- base: `asdlc/seller-centre` after wave 0

## Write-set (EXCLUSIVE)
- team-order/internal/store/**
- team-order/internal/order/**
- team-order/internal/rpc/**
- team-order/migrations/0026_orders_seller_status_created_idx.up.sql
- team-order/migrations/0026_orders_seller_status_created_idx.down.sql
- team-order/migrations/migration_test.go
- team-order/FEATURES.yaml
- team-order/README.md
- platform-e2e/tests/e2e/features/order/seller_list_paging.feature
- platform-e2e/tests/e2e/step_definitions/seller_list_paging_steps.py
- platform-e2e/tests/e2e/step_definitions/test_seller_list_paging.py
- platform-e2e/src/api/services/order_service.py
- widened (P1):
  - openspec/changes/order-seller-list-paging/proposal.md — own OpenSpec folder; migration renumbered 0025→0026 (user decision 2026-11-15)
  - openspec/changes/order-seller-list-paging/specs/order-seller-list-paging/spec.md — own OpenSpec folder; VERIFIED BY citation + 0025→0026 renumber (user decision 2026-11-15)
  - openspec/changes/order-seller-list-paging/tasks.md — own OpenSpec folder; migration renumbered 0025→0026
  - platform-e2e/tests/e2e/step_definitions/order_seller_list_paging_steps.py — e2e step file of this change, named differently from the packet
  - platform-e2e/tests/e2e/step_definitions/test_order_seller_list_paging.py — e2e step file of this change, named differently from the packet

## Verify
- `DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-order --vet`
- e2e: seller_list_paging feature against env seller-centre (team-gateway, team-order, team-domain, team-payment, team-identity)
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py spec-sync . order-seller-list-paging` and `openspec validate order-seller-list-paging --strict`
