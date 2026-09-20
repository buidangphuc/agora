# Tasks

## 1. Code — team-ai
- [x] Update `app/modules/business/recommend/schemas.py` with `placement_id`, `category_id`, `cart_listing_ids`, `fallback_tier`.
- [x] Update `app/modules/business/recommend/service.py` to implement placement-aware routing and fallback ladder.
- [x] Write unit tests in `tests/unit/modules/recommend/test_placement_engine.py`.

## 2. Verification
- [x] Validate OpenSpec change (`openspec validate add-placement-engine --strict`).
- [x] Run `pytest -v tests/unit/modules/recommend/test_placement_engine.py` in `team-ai/`.
