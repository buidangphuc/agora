## Why

Following **ADR-0012**, `team-ai`'s recommendation service needs to support distinct placements (`home_feed`, `similar_items`, `cart_cross_sell`) with placement-specific retrieval strategies, a 4-tier fallback ladder, cold-start handling, and attribution metadata.

## What Changes

- **team-ai** (`app/modules/business/recommend/`):
  - Update `RecommendQuery` to accept `placement_id`, `category_id`, and `cart_listing_ids`.
  - Update `RecommendResult` to include `placement_id` and `fallback_tier`.
  - Update `RecommendationService` to implement placement routing and the 4-tier relaxation ladder.
  - Unit tests in `team-ai/tests/unit/modules/recommend/test_placement_engine.py`.

## Non-goals

- No change to gRPC contract signature — proto fields map cleanly into `RecommendQuery`.
