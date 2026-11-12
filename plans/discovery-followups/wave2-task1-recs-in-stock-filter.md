# [W2-T1] recs-in-stock-filter

## Role
feature-run worker (change `openspec/changes/recs-in-stock-filter`)

## Objective
Implement OpenSpec change `recs-in-stock-filter` test-first per its tasks.md; context in plans/discovery-followups/00-spec.md (Clarified).
- team-ai `in_stock_only` uses live `SearchService.GetListingStock`: top limit×2 (cap 20, ≤2 calls) with refill, stock > 0, missing entry drops only that candidate, call error/timeout/malformed empties the placement; `Candidate.in_stock: bool | None`; service-principal client modelled on team-search's ClassifyTags caller.
- depends_on: search-listing-stock-lookup (merged)
- base: `asdlc/discovery-followups` after wave 1
- hot file: platform-e2e/tests/e2e/flows/item_attributes_support.py (shared with analytics-listing-deleted-status, wave 1)

## Write-set (EXCLUSIVE)
- team-ai/app/modules/business/recommendation/**
- team-ai/app/config*
- team-ai/app/core/**
- team-ai/app/bootstrap/addons/recommendation.py
- team-ai/tests/unit/**
- team-ai/tests/fixtures/**
- team-ai/.env.example
- team-ai/README.md
- team-ai/FEATURES.yaml
- team-ai/ops/**
- docker-compose.yaml
- docker-compose.services.yaml
- platform-e2e/tests/e2e/features/recsys/in_stock_recommendations.feature
- platform-e2e/tests/e2e/features/recsys/cart_cross_sell.feature
- platform-e2e/tests/e2e/features/recommendations/cart_cross_sell.feature
- platform-e2e/tests/e2e/step_definitions/in_stock_recommendations_steps.py
- platform-e2e/tests/e2e/step_definitions/test_in_stock_recommendations.py
- platform-e2e/tests/e2e/step_definitions/cart_cross_sell_steps.py
- platform-e2e/tests/e2e/step_definitions/test_cart_cross_sell.py
- platform-e2e/tests/e2e/flows/item_attributes_support.py
- platform-e2e/tests/e2e/flows/recsys_*.py

## Verify
- `cd team-ai && make lint test` (unit + eval)
- e2e: in_stock_recommendations + cart_cross_sell features against env discovery-followups (team-search, team-ai)
