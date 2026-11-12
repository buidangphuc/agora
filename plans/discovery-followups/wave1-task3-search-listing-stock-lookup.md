# [W1-T3] search-listing-stock-lookup

## Role
feature-run worker (change `openspec/changes/search-listing-stock-lookup`)

## Objective
Implement OpenSpec change `search-listing-stock-lookup` test-first per its tasks.md; context in plans/discovery-followups/00-spec.md.
- `SearchService.GetListingStock` in team-search: 1..10 ids, service principal + `search:read`, one batch index read with 200 ms deadline → UNAVAILABLE, entries in request order, absent stock for unknown/tombstoned/unprojected, ids never logged. No gateway route.
- depends_on: discovery-followups-contract (merged)
- base: `asdlc/discovery-followups` after wave 0

## Write-set (EXCLUSIVE)
- team-search/internal/rpc/**
- team-search/internal/index/**
- team-search/cmd/**
- team-search/README.md
- team-search/FEATURES.yaml
- platform-e2e/tests/e2e/features/search/listing_stock_lookup.feature
- platform-e2e/tests/e2e/step_definitions/listing_stock_lookup_steps.py
- platform-e2e/tests/e2e/step_definitions/test_listing_stock_lookup.py
- platform-e2e/tests/e2e/flows/listing_stock_support.py

## Verify
- `cd team-search && DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 go test ./...`
- e2e: listing_stock_lookup feature against env discovery-followups (team-search + OpenSearch overlay)
