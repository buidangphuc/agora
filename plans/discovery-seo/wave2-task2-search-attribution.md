# [W2-T2] search-attribution

## Role
feature-run worker (change `openspec/changes/search-attribution`)

## Objective
Implement OpenSpec change `search-attribution` test-first per its tasks.md; context in plans/discovery-seo/00-spec.md and the change's design.md.
- team-search mints `request_id` (UUID v4 hex) and sets `model_version` from the effective retrieval path (`SEARCH_RANKER_VERSION`) on SearchListings and RunSavedSearch; team-ai search servicer fills the same fields; team-frontend passes `impressionId`/`modelVersion` to `ListingGrid` (`placementId="search_results"`); team-analytics serves `GetSearchPerformance` (admin, window 0→24, max 168).
- depends_on: discovery-seo-contract (merged)
- base: `feat/discovery-seo` after wave 1 merged
- hot files: `team-search/internal/rpc/search.go` (next: search-bestseller-rating, wave 3); `team-frontend/src/features/search/SearchBlocks.tsx` and `src/app/(shop)/search/page.tsx` (after frontend-seo, before frontend-discovery-sort-rating).

## Write-set (EXCLUSIVE)
- team-search/internal/config/**
- team-search/internal/retrieval/**
- team-search/internal/rpc/**
- team-search/internal/app/server.go
- team-search/internal/app/server_test.go
- team-search/.env.example
- team-search/README.md
- team-search/FEATURES.yaml
- team-ai/app/transport/grpc/servicers/search.py
- team-ai/app/core/config/**
- team-ai/tests/unit/transport/test_search_servicer.py
- team-ai/.env.example
- team-ai/README.md
- team-ai/FEATURES.yaml
- team-frontend/src/lib/gateway/search.ts
- team-frontend/src/lib/gateway/search.test.ts
- team-frontend/src/features/search/data.ts
- team-frontend/src/features/search/data.test.ts
- team-frontend/src/features/search/SearchBlocks.tsx
- team-frontend/src/features/search/SearchBlocks.attribution.test.tsx
- team-frontend/src/app/(shop)/search/page.tsx
- team-frontend/FEATURES.yaml
- team-analytics/internal/query/**
- team-analytics/internal/rpc/**
- team-analytics/README.md
- team-analytics/FEATURES.yaml
- platform-e2e/tests/e2e/features/search/search_attribution.feature
- platform-e2e/tests/e2e/features/analytics/search_performance.feature
- platform-e2e/tests/e2e/step_definitions/search_attribution_steps.py
- platform-e2e/tests/e2e/step_definitions/test_search_attribution.py
- platform-e2e/tests/e2e/step_definitions/test_search_performance.py
- platform-e2e/src/pages/search_page.py

## Verify
- `cd team-search && buf generate && go vet ./... && go test ./...`
- `cd team-analytics && buf generate --template buf.gen.local.yaml && go vet ./... && go test ./...`
- `cd team-ai && uv sync --dev --all-extras && uv run --all-extras python -m scripts.gen_proto && uv run --all-extras pytest -q`
- `cd team-frontend && npm run proto && npm run check`
- e2e: `cd platform-e2e && HEADLESS=true $PY -m pytest tests/e2e/step_definitions/test_search_attribution.py tests/e2e/step_definitions/test_search_performance.py -q` (PY=/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python)
