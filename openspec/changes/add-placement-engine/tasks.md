# Tasks

## 1. Code — team-ai
- [x] Update `app/modules/business/recommend/schemas.py` with `placement_id`, `category_id`, `cart_listing_ids`, `fallback_tier`.
- [x] Update `app/modules/business/recommend/service.py` to implement placement-aware routing and fallback ladder.
- [x] Write unit tests in `tests/unit/modules/recommend/test_placement_engine.py`.

## 2. Verification
- [x] Validate OpenSpec change (`openspec validate add-placement-engine --strict`).
- [x] Run `pytest -v tests/unit/modules/recommend/test_placement_engine.py` in `team-ai/`.

## Evidence (2026-10-10)

- Code and unit tests: each repo's `make check` / test suite was green at merge (see the commit bodies).
- e2e after rebuilding team-ai, team-search (server and indexer), gateway, frontend and the recsys image, with
  platform-recsys-nearline and the modelserve overlay (fake TEI + router) running:
  - ML scenarios: 23/23, twice;
  - modelserve, hybrid and taxonomy: 27/27, three times;
  - placement and serve-trained scenarios: green three times.
- Scenarios that cannot be produced end to end carry a VERIFIED BY line in the spec and a not-testable FEATURES
  entry.
- spec_sync --strict reports e2e-ready.
