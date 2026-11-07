# [W3-T2] search-bestseller-rating

## Role
feature-run worker (change `openspec/changes/search-bestseller-rating`)

## Objective
Implement OpenSpec change `search-bestseller-rating` test-first per its tasks.md (BR-1..BR-20); context in plans/discovery-seo/00-spec.md and the change's design.md.
- team-search consumes `ListingSalesChanged` (order.events) and `ListingRatingChanged` (engagement.events) into `sold_count`/`rating_avg`/`review_count` with a `changed_at` guard and pending docs (`pending_at`, purged by the sweep); `SORT_BY_BEST_SELLING` on the lexical leg; `min_rating` 0..5 filter for all modes and saved search; ratings facet floors 4 and 3; hit fields mapped.
- depends_on: order-listing-sales, engagement-listing-rating-events (merged); rebase on search-attribution for `internal/rpc/search.go`
- base: `feat/discovery-seo` after wave 2 merged

## Write-set (EXCLUSIVE)
- team-search/internal/index/**
- team-search/internal/events/**
- team-search/internal/rpc/**
- team-search/internal/tombstone/**
- team-search/internal/config/**
- team-search/internal/app/**
- team-search/internal/retrieval/**
- team-search/cmd/**
- team-search/.env.example
- team-search/README.md
- team-search/FEATURES.yaml
- platform-e2e/tests/e2e/features/search/search_query_correctness.feature
- platform-e2e/tests/e2e/features/search/search_best_selling_rating.feature
- platform-e2e/tests/e2e/features/search/search_read_model_deletes.feature
- platform-e2e/tests/e2e/step_definitions/srm_query_steps.py
- platform-e2e/tests/e2e/step_definitions/srm_deletes_steps.py
- platform-e2e/tests/e2e/step_definitions/search_best_selling_rating_steps.py
- platform-e2e/tests/e2e/step_definitions/test_search_best_selling_rating.py

## Verify
- `cd team-search && buf generate && go vet ./... && go test ./...`
- e2e: `cd platform-e2e && HEADLESS=true $PY -m pytest tests/e2e/step_definitions/test_srm_query.py tests/e2e/step_definitions/test_search_best_selling_rating.py tests/e2e/step_definitions/test_srm_deletes.py -q` (PY=/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python)
