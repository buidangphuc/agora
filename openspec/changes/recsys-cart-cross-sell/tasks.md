## 1. Code track: team-ai

- [x] 1.1 Unit tests first, verify red: `tests/unit/test_filter.py`, `test_placements.py`, `test_category_popular.py`, `test_recommend_servicer.py`, `test_candidate_attributes.py`
- [x] 1.2 Add `context_listing_ids` to `RecommendationQuery`, map it in the Recommend servicer, abort over 50 with INVALID_ARGUMENT, log `context_count` only (Context over the limit is rejected by team-ai, Logs carry the context count and no ids)
- [x] 1.3 Extend `Candidate` with `seller_id`, `category_id`, `visible`; parse them in `adapters/qdrant.py` `_candidate` and in `adapters/redis_cache.py` entry parsing; missing `visible` means not visible (Adapters read the attribute keys, Missing visible fails closed). Depends on slice recsys-candidate-attributes
- [x] 1.4 `ranking/filter.py`: candidates without `seller_id` are never deduplicated (Missing seller id is not deduplicated)
- [x] 1.5 `cart_cross_similarity` over `source.similar` per context id, score = max across lines (Similar listings for cart lines, Empty context ends at global popular, Unknown context listing is skipped, Exactly 50 context ids are served)
- [x] 1.6 Declare `Filters` on `Placement`; `ranking/filter.py` applies `in_stock_only`, `deduplicate_sellers`, `exclude_seed`, `exclude_cart_items`, and drops non-visible candidates; set `deduplicate_sellers: true` on `cart_cross_sell` in `placements.yaml` (Cart items and out-of-stock excluded, One listing per seller, Toggling a filter off changes the output, Seed excluded when enabled, Seed eligible when exclude_seed is off, Unknown filter key fails at load, Hidden and unpublished listings never appear)
- [x] 1.7 `category_popular`: seed category first, else context categories, skipping listings without category (Seed category, Seed category wins over context categories, Cart categories, Context listing without category contributes nothing, No seed and no context). Read `seller_id`, `category_id`, `visible` from the candidate payload (needs recsys-candidate-attributes)
- [x] 1.8 Forbid unknown keys on `Placement`, `LadderStep`, `Filters`; delete `name`, `enabled`, `timeout_ms`, `allow_cache`, `tier` (placement and ladder-step level) from `placements.yaml`; update the `placements.py` docstring (No ignored keys remain, A removed key is rejected, A ladder step with tier is rejected, Existing placements still resolve)

## 2. Code track: team-gateway

- [x] 2.1 Go test first, then `RecommendationForwarder.Recommend` rejects over 50 ids with INVALID_ARGUMENT and forwards the field (Gateway rejects 51 ids, Gateway forwards context ids)

## 3. Code track: team-frontend

- [x] 3.1 `crossSell.test.tsx` first, verify red, then a cross-sell row on `(shop)/cart/page.tsx` reusing the `RecommendationsRow` beacon wiring; send the last 50 lines in `GetCart` order; failures and empty results hide the row (Row shows for a cart with items, Row is hidden for an empty cart, Cart renders when recommendations fail, Row is hidden for an empty result, At most 50 lines are sent, Beacons carry the recommendation identity)

## 4. E2E track

- [x] 4.0 Seed `in_stock=false` for the out-of-stock candidate and `visible=true` on every e2e candidate directly in the Qdrant/Redis fixture; no producer dependency (C14; real stock source is C3-U1)
- [x] 4.1 `platform-e2e/tests/e2e/features/recsys/cart_cross_sell.feature`: gateway-entry scenarios (Similar listings for cart lines, Empty context ends at global popular, Exactly 50 context ids are served, Unknown context listing is skipped, Gateway rejects 51 ids, Gateway forwards context ids, Seed category, Cart categories, Cart items and out-of-stock excluded, One listing per seller, Seed excluded when enabled, Existing placements still resolve); team-ai over-limit is covered by unit test 1.1 (team-ai gRPC is `team-ai-svc:50060`, `docker-compose.services.yaml:691`)
- [x] 4.2 `platform-e2e/tests/e2e/features/frontend/cart_cross_sell_ui.feature` (Row shows for a cart with items, Row is hidden for an empty cart, Beacons carry the recommendation identity); FEATURES.yaml entries via the e2e-author
- [ ] 4.3 Run on the local stack with only gateway, team-ai, frontend and their infra; measure `cart_cross_sell` p95 at 50 ids, non-gating
- [x] 4.4 `openspec validate recsys-cart-cross-sell --strict`, per-repo checks, pre-commit-ci
