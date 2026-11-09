# [W4-T1] frontend-discovery-sort-rating

## Role
feature-run worker (change `openspec/changes/frontend-discovery-sort-rating`)

## Objective
Implement OpenSpec change `frontend-discovery-sort-rating` test-first per its tasks.md; context in plans/discovery-seo/00-spec.md and the change's design.md.
- `sort=best_selling` ("Bán chạy") mapped to `SORT_BY_BEST_SELLING`; `minRating` (3 or 4) mapped to `min_rating`; rating facet group in sidebar and drawer, chip, active-filter count; legacy `rating=` ignored; rewrite the rating-removed and "no Bán Chạy" e2e and FEATURES entries.
- depends_on: search-bestseller-rating (merged)
- base: `feat/discovery-seo` after wave 3 merged
- hot files: `SearchBlocks.tsx` and `src/app/(shop)/search/page.tsx` (rebase on frontend-seo and search-attribution).

## Write-set (EXCLUSIVE)
- team-frontend/src/features/search/**
- team-frontend/src/app/(shop)/search/page.tsx
- team-frontend/src/app/(shop)/search/page.test.tsx
- team-frontend/FEATURES.yaml
- platform-e2e/tests/e2e/features/frontend/search_rating_removed.feature
- platform-e2e/tests/e2e/features/frontend/search_facets.feature
- platform-e2e/tests/e2e/step_definitions/srm_frontend_steps.py
- platform-e2e/tests/e2e/step_definitions/test_srm_frontend.py
- platform-e2e/tests/e2e/step_definitions/discovery_steps.py
- platform-e2e/tests/e2e/step_definitions/search_sort_steps.py
- platform-e2e/tests/e2e/step_definitions/test_search_facets.py
- platform-e2e/src/pages/search_page.py

## Verify
- `cd team-frontend && npm run proto && npm run check && npm run build`
- e2e: `cd platform-e2e && HEADLESS=true $PY -m pytest tests/e2e/step_definitions/test_srm_frontend.py tests/e2e/step_definitions/test_search_facets.py -q` (PY=/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python)
