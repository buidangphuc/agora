# [W1-T1] domain-my-listings-filter

## Role
feature-run worker (change `openspec/changes/domain-my-listings-filter`)

## Objective
Implement OpenSpec change `domain-my-listings-filter` test-first per its tasks.md; context in plans/seller-centre/00-spec.md and the decisions in plans/seller-centre/LOCK.md.
- `ListMyListings` honours `statuses` (OR) and `query` (trimmed, ≤ 100 runes, case-insensitive substring, LIKE metacharacters escaped) with filtered `total`, cursor and clamp, in Postgres and `storetest`; migration `0017_listings_owner_status_idx` on `listings (seller_id, status, id)`; query text never logged.
- depends_on: seller-centre-contract
- base: `asdlc/seller-centre` after wave 0

## Write-set (EXCLUSIVE)
- team-domain/internal/store/**
- team-domain/internal/listing/listing.go
- team-domain/internal/listing/listing_test.go
- team-domain/internal/rpc/listing.go
- team-domain/internal/rpc/listing_test.go
- team-domain/internal/rpc/listing_filter_test.go
- team-domain/migrations/0017_listings_owner_status_idx.up.sql
- team-domain/migrations/0017_listings_owner_status_idx.down.sql
- team-domain/migrations/migrations_test.go
- team-domain/FEATURES.yaml
- team-domain/README.md
- platform-e2e/tests/e2e/features/domain/my_listings_filter.feature
- platform-e2e/tests/e2e/step_definitions/my_listings_filter_steps.py
- platform-e2e/tests/e2e/step_definitions/test_my_listings_filter.py
- platform-e2e/src/api/services/listing_service.py
- widened (P1):
  - team-domain/internal/app/migrate_test.go — pinned embedded-migration count (16 → 17) directly broken by migration 0017

## Verify
- `DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-domain --vet`
- e2e: my_listings_filter feature against env seller-centre (team-gateway, team-domain, team-identity)
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py spec-sync . domain-my-listings-filter` and `openspec validate domain-my-listings-filter --strict`
