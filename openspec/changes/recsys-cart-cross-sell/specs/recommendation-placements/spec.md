## ADDED Requirements

### Requirement: Cart cross-sell uses the cart lines as context

The `Recommend` RPC SHALL accept `context_listing_ids` (at most 50) and the `cart_cross_similarity` strategy SHALL
return listings similar to those context listings. Each listing's score SHALL be its maximum similarity across the
context lines and results SHALL be in descending score.

#### Scenario: Similar listings for cart lines

- **WHEN** `Recommend` is called with `placement_id=cart_cross_sell` and `context_listing_ids=[A,B]` where A and B have similar-item neighbours
- **THEN** the response contains at most 8 items in descending score, each item is a neighbour of A or B and its score is the maximum of its similarities to A and B
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recsys/cart_cross_sell.feature` `Similar listings for cart lines`

#### Scenario: Empty context ends at global popular

- **WHEN** `Recommend` is called with `placement_id=cart_cross_sell`, an empty `context_listing_ids` and no seed
- **THEN** the response is not an error and its items are the `global_popular` listings
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recsys/cart_cross_sell.feature` `Empty context ends at global popular`

#### Scenario: Context over the limit is rejected by team-ai

- **WHEN** a unit test calls the team-ai Recommend servicer with 51 `context_listing_ids`
- **THEN** the call aborts with `INVALID_ARGUMENT` and no candidate lookup runs
- **VERIFIED BY**: `team-ai/tests/unit/test_recommend_servicer.py` `context over the limit is rejected`

#### Scenario: Exactly 50 context ids are served

- **WHEN** `Recommend` is called through the gateway with exactly 50 `context_listing_ids`
- **THEN** the response status is OK with at most 8 items
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recsys/cart_cross_sell.feature` `Exactly 50 context ids are served`

#### Scenario: Unknown context listing is skipped

- **WHEN** `Recommend` is called with `context_listing_ids=[A,unknown]` and `unknown` has no vector
- **THEN** the response is OK and every item is a neighbour of A
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recsys/cart_cross_sell.feature` `Unknown context listing is skipped`

#### Scenario: Logs carry the context count and no ids

- **WHEN** a unit test serves a `cart_cross_sell` request with three context ids and captures the log output
- **THEN** the logs contain `context_count=3` and none of the three listing ids
- **VERIFIED BY**: `team-ai/tests/unit/test_recommend_servicer.py` `logs carry context_count and no ids`

### Requirement: The gateway caps and forwards cart context

The gateway `RecommendationForwarder.Recommend` SHALL forward `context_listing_ids` to team-ai unchanged and SHALL reject a request with more than 50 ids with `INVALID_ARGUMENT` without calling team-ai.

#### Scenario: Gateway rejects 51 ids

- **WHEN** a client calls `RecommendationService.Recommend` on the gateway with 51 `context_listing_ids`
- **THEN** the response is `INVALID_ARGUMENT` and team-ai receives no call
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recsys/cart_cross_sell.feature` `Gateway rejects 51 ids`

#### Scenario: Gateway forwards context ids

- **WHEN** a client calls the gateway with `placement_id=cart_cross_sell` and `context_listing_ids=[A,B]`
- **THEN** the items are neighbours of A or B and contain neither A nor B
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recsys/cart_cross_sell.feature` `Gateway forwards context ids`

### Requirement: Category-popular returns popular listings of the relevant categories

The `category_popular` strategy SHALL return popular listings from the category of the seed listing; only when there is no seed SHALL it use the categories of the context listings, and a context listing with no category SHALL contribute nothing. It SHALL return no candidates when the query has neither.

#### Scenario: Seed category

- **WHEN** `Recommend` reaches `category_popular` with a `seed_listing_id` in category C and earlier steps empty
- **THEN** every returned listing belongs to category C
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recsys/cart_cross_sell.feature` `Seed category`

#### Scenario: Seed category wins over context categories

- **WHEN** `Recommend` reaches `category_popular` with a seed in category C and context listings in category D
- **THEN** every returned listing belongs to C and none to D
- **VERIFIED BY**: `team-ai/tests/unit/test_category_popular.py` `seed category wins over context categories`

#### Scenario: Cart categories

- **WHEN** `Recommend` is called with `placement_id=cart_cross_sell`, no seed, context listings in categories C1 and C2, and `cart_cross_similarity` empty
- **THEN** every returned listing belongs to C1 or C2
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recsys/cart_cross_sell.feature` `Cart categories`

#### Scenario: Context listing without category contributes nothing

- **WHEN** `category_popular` runs with context listings A (category C1) and B (no category)
- **THEN** every returned listing belongs to C1
- **VERIFIED BY**: `team-ai/tests/unit/test_category_popular.py` `context listing without category contributes nothing`

#### Scenario: No seed and no context

- **WHEN** `Recommend` reaches `category_popular` with neither a seed nor context listings
- **THEN** the step yields no candidates and the ladder continues to `global_popular`
- **VERIFIED BY**: `team-ai/tests/unit/test_category_popular.py` `no seed and no context`

### Requirement: Placement filters change the output

`ranking/filter.py` SHALL apply each `filters` key a placement enables and no other: `in_stock_only`, `deduplicate_sellers`, `exclude_seed`, `exclude_cart_items`. `in_stock_only` filters on the candidate `in_stock` value, which defaults to true until a producer exists (C14). `cart_cross_sell` SHALL enable `in_stock_only`, `deduplicate_sellers` and `exclude_cart_items`.

#### Scenario: Cart items and out-of-stock excluded

- **WHEN** `Recommend` is called on `cart_cross_sell` with context listings A,B and one candidate is seeded with `in_stock=false` directly in the test fixture (no producer involved)
- **THEN** neither A, B nor the out-of-stock listing appears in the response
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recsys/cart_cross_sell.feature` `Cart items and out-of-stock excluded`

#### Scenario: One listing per seller

- **WHEN** `Recommend` is called on `cart_cross_sell` and three candidates share seller S
- **THEN** exactly one of them, the highest scored, appears in the response
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recsys/cart_cross_sell.feature` `One listing per seller`

#### Scenario: Toggling a filter off changes the output

- **WHEN** a unit test filters the same candidates with `deduplicate_sellers` off
- **THEN** all three listings of seller S are eligible
- **VERIFIED BY**: `team-ai/tests/unit/test_filter.py` `deduplicate_sellers off keeps all sellers listings`

#### Scenario: Seed excluded when enabled

- **WHEN** `Recommend` is called on `similar_items` with `exclude_seed: true` and the seed is its own nearest neighbour
- **THEN** the seed is absent from the response
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recsys/cart_cross_sell.feature` `Seed excluded when enabled`

#### Scenario: Seed eligible when exclude_seed is off

- **WHEN** a unit test filters candidates containing the seed with `exclude_seed` off
- **THEN** the seed is among the eligible candidates
- **VERIFIED BY**: `team-ai/tests/unit/test_filter.py` `seed eligible when exclude_seed is off`

#### Scenario: Unknown filter key fails at load

- **WHEN** `placements.yaml` contains `filters: {bogus: true}`
- **THEN** loading fails with a validation error naming the key
- **VERIFIED BY**: `team-ai/tests/unit/test_placements.py` `unknown filter key fails at load`

#### Scenario: Hidden and unpublished listings never appear

- **WHEN** a unit test runs any placement over candidates including a listing whose `visible` payload flag is false
- **THEN** that listing is absent from the response
- **VERIFIED BY**: `team-ai/tests/unit/test_filter.py` `hidden listings never appear`

#### Scenario: Missing visible fails closed

- **WHEN** a unit test runs any placement over a candidate whose payload has no `visible` key
- **THEN** that candidate is absent from the response
- **VERIFIED BY**: `team-ai/tests/unit/test_filter.py` `missing visible fails closed`

#### Scenario: Missing seller id is not deduplicated

- **WHEN** a unit test filters with `deduplicate_sellers` on over two candidates that have no `seller_id` and one with `seller_id` S
- **THEN** all three are eligible
- **VERIFIED BY**: `team-ai/tests/unit/test_filter.py` `missing seller id is not deduplicated`

#### Scenario: Adapters read the attribute keys

- **WHEN** a unit test parses a Qdrant hit and a Redis cache entry that carry `seller_id`, `category_id` and `visible`
- **THEN** each yields a `Candidate` with those three values, and an entry without them yields empty `seller_id`, empty `category_id` and `visible` false
- **VERIFIED BY**: `team-ai/tests/unit/test_candidate_attributes.py` `adapters read the attribute keys`

### Requirement: Placement config carries only keys the loader uses

`placements.yaml` SHALL NOT contain `name`, `enabled`, `timeout_ms`, `allow_cache` or `tier` (at placement level or inside any `candidate_ladder` step) and the file SHALL NOT declare `home.trending`, which does not exist. `Placement`, `LadderStep` and `Filters` SHALL forbid unknown keys, and the `placements.py` docstring SHALL list no key the loader implements as ignored.

#### Scenario: No ignored keys remain

- **WHEN** a unit test scans `placements.yaml` for the removed keys and loads it
- **THEN** the file loads, contains none of the removed keys at any level, and declares no `home.trending` placement
- **VERIFIED BY**: `team-ai/tests/unit/test_placements.py` `no ignored keys remain`

#### Scenario: A removed key is rejected

- **WHEN** a placement declares `timeout_ms: 35`
- **THEN** loading fails with a validation error naming `timeout_ms`
- **VERIFIED BY**: `team-ai/tests/unit/test_placements.py` `removed key is rejected`

#### Scenario: A ladder step with tier is rejected

- **WHEN** a placement's `candidate_ladder` step declares `tier: "tier1"`
- **THEN** loading fails with a validation error naming `tier`
- **VERIFIED BY**: `team-ai/tests/unit/test_placements.py` `ladder step with tier is rejected`

#### Scenario: Existing placements still resolve

- **WHEN** `Recommend` is called with `home_feed`, `similar_items` and an undefined placement id
- **THEN** the first two use their own ladders and the third falls back to `home_feed` with status OK
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recsys/cart_cross_sell.feature` `Existing placements still resolve`
