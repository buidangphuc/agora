## MODIFIED Requirements

### Requirement: Placement filters change the output

`ranking/filter.py` SHALL apply each `filters` key a placement enables and no other: `in_stock_only`, `deduplicate_sellers`, `exclude_seed`, `exclude_cart_items`. `in_stock_only` SHALL keep a candidate only when its live search stock (the projected base stock, no variant rollup) is greater than 0. team-ai SHALL apply the other filters and ranking first, then read the stock of only the top ranked candidates through `SearchService.GetListingStock`: the first `limit` x 2 candidates, capped at 20 distinct ids, in at most 10 ids per call (so at most 2 calls). A candidate whose entry has no `stock`, or has no entry, SHALL be dropped alone. If fewer than `limit` candidates remain, team-ai SHALL refill by looking up the next ranked candidates (up to 10 ids per call) until `limit` is reached, the candidates are exhausted or the `RECS_RETRIEVE_TIMEOUT_MS` budget is spent; a spent budget with an incomplete result SHALL return the in-stock candidates found so far. If any lookup call fails, times out or returns a malformed response (an entry count or order that does not match the request), team-ai SHALL drop every candidate of that placement, return an empty item list for it and SHALL NOT fail the RPC. The outgoing call SHALL carry team-ai's service principal with scope `search:read`. The `in_stock` value carried by the Qdrant payload or the Redis entry SHALL NOT be used for an `in_stock_only` placement, and a candidate whose `in_stock` is unknown SHALL never pass `in_stock_only`. A placement without `in_stock_only` SHALL make no stock lookup. `cart_cross_sell` SHALL enable `in_stock_only`, `deduplicate_sellers` and `exclude_cart_items`.

#### Scenario: Cart items and out-of-stock excluded

- **WHEN** `Recommend` is called on `cart_cross_sell` with context listings A,B and one candidate listing has search stock 0
- **THEN** neither A, B nor the zero-stock listing appears in the response
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recommendations/cart_cross_sell.feature` `Cart items and out-of-stock excluded`

#### Scenario: One listing per seller

- **WHEN** `Recommend` is called on `cart_cross_sell` and three in-stock candidates share seller S
- **THEN** exactly one of them, the highest scored, appears in the response
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recommendations/cart_cross_sell.feature` `One listing per seller`

#### Scenario: Toggling a filter off changes the output

- **WHEN** a unit test filters the same candidates with `deduplicate_sellers` off
- **THEN** all three listings of seller S are eligible
- **VERIFIED BY**: `team-ai/tests/unit/test_filter.py` `deduplicate_sellers off keeps all sellers listings`

#### Scenario: Sold-out listing is dropped from an in_stock_only placement

- **WHEN** `Recommend` is called on `similar_items` and one of the retrieved candidates has search stock 0 while its Qdrant payload says `in_stock=true`
- **THEN** that listing is absent from the response and every returned listing has search stock greater than 0
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recsys/in_stock_recommendations.feature` `Sold-out listing is dropped from an in_stock_only placement`

#### Scenario: Stock sold out after indexing is dropped without a batch run

- **WHEN** a listing that `Recommend` returned on `home_feed` has its search stock set to 0 and `Recommend` is called again with no training run in between
- **THEN** the second response no longer contains that listing
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recsys/in_stock_recommendations.feature` `Stock sold out after indexing is dropped without a batch run`

#### Scenario: Absent stock or missing entry drops only that candidate

- **WHEN** a unit test runs an `in_stock_only` placement and the lookup returns an entry with no `stock` for candidate X, stock 3 for candidate Y, and stock 5 for candidate W, and no entry for candidate Z (entries otherwise matching the request)
- **THEN** Y and W are returned, X and Z are not, and the result is not empty
- **VERIFIED BY**: `team-ai/tests/unit/test_in_stock_lookup.py` `absent stock drops only that candidate`

#### Scenario: Stock of exactly 1 is in stock and 0 is not

- **WHEN** a unit test runs an `in_stock_only` placement with lookup results stock 1 for candidate A and stock 0 for candidate B
- **THEN** A is returned and B is not
- **VERIFIED BY**: `team-ai/tests/unit/test_in_stock_lookup.py` `stock boundary one in zero out`

#### Scenario: Search unavailable empties the placement

- **WHEN** `Recommend` is called on `cart_cross_sell` while team-search is stopped and Qdrant and Redis hold candidates
- **THEN** the response is OK with zero items, and a later call after team-search is restarted returns items again
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recsys/in_stock_recommendations.feature` `Search unavailable empties the placement`

#### Scenario: Slow search fails closed within the budget

- **WHEN** a unit test runs an `in_stock_only` placement with a lookup that exceeds `RECS_RETRIEVE_TIMEOUT_MS`
- **THEN** the call returns zero items, no candidate is returned, the RPC does not abort, and the call finishes within `RECS_RETRIEVE_TIMEOUT_MS` + 50 ms
- **VERIFIED BY**: `team-ai/tests/unit/test_in_stock_lookup.py` `timeout fails closed within budget`

#### Scenario: One failed call drops the whole placement

- **WHEN** a unit test uses a limit of 10 (20 ids, two lookup calls of 10) and the second call raises an error although the first reported stock greater than 0
- **THEN** the result has zero items
- **VERIFIED BY**: `team-ai/tests/unit/test_in_stock_lookup.py` `one failed call drops all candidates`

#### Scenario: Malformed response empties the placement

- **WHEN** a unit test makes the lookup return 9 entries for 10 requested ids, or 10 entries in a different order
- **THEN** the result has zero items and the RPC does not abort
- **VERIFIED BY**: `team-ai/tests/unit/test_in_stock_lookup.py` `malformed response empties the placement`

#### Scenario: Only the top ranked candidates are looked up

- **WHEN** a unit test serves `limit` 8 with 100 eligible candidates, all in stock
- **THEN** the lookup receives exactly the 16 top ranked ids in at most 2 calls of at most 10 ids, and the response is the first 8 of them in rank order
- **VERIFIED BY**: `team-ai/tests/unit/test_in_stock_lookup.py` `looks up only limit times two top ranked ids`

#### Scenario: Lookup is capped at 20 ids

- **WHEN** a unit test serves `limit` 15 with 100 eligible candidates, all in stock
- **THEN** the first lookup phase requests exactly 20 distinct ids in 2 calls and the response has 15 items
- **VERIFIED BY**: `team-ai/tests/unit/test_in_stock_lookup.py` `lookup capped at twenty ids`

#### Scenario: Too few in stock triggers a refill

- **WHEN** a unit test serves `limit` 8 where 12 of the first 16 ranked candidates have stock 0 and later ranked candidates are in stock
- **THEN** team-ai looks up the next ranked candidates and the response has 8 in-stock items in rank order
- **VERIFIED BY**: `team-ai/tests/unit/test_in_stock_lookup.py` `refill from next ranked candidates`

#### Scenario: Refill stops when candidates are exhausted

- **WHEN** a unit test serves `limit` 8 with only 5 eligible candidates in stock
- **THEN** the response has 5 items and no further lookup call is made
- **VERIFIED BY**: `team-ai/tests/unit/test_in_stock_lookup.py` `refill stops when candidates are exhausted`

#### Scenario: Refill stops when the budget is spent

- **WHEN** a unit test lets the first phase return 3 in-stock candidates for `limit` 8 and the refill call exceeds the remaining `RECS_RETRIEVE_TIMEOUT_MS` budget
- **THEN** the response is OK, holds no more candidates than were verified in stock, and the call finishes within `RECS_RETRIEVE_TIMEOUT_MS` + 50 ms
- **VERIFIED BY**: `team-ai/tests/unit/test_in_stock_lookup.py` `refill bounded by budget`

#### Scenario: Stock is looked up once per id per request

- **WHEN** a unit test serves a request whose ladder steps yield overlapping ids and the popular floor repeats them
- **THEN** each distinct listing id is sent to the lookup at most once in the request
- **VERIFIED BY**: `team-ai/tests/unit/test_in_stock_lookup.py` `lookup memoised across ladder steps`

#### Scenario: Unknown payload stock does not leak through

- **WHEN** a unit test serves an `in_stock_only` placement whose candidates come from a Qdrant payload and a Redis entry that carry no `in_stock` value, with the lookup reporting stock 0 for them
- **THEN** none of them is returned, and with the lookup unavailable none is returned either
- **VERIFIED BY**: `team-ai/tests/unit/test_in_stock_lookup.py` `default in_stock does not leak`

#### Scenario: Placement without in_stock_only makes no search call

- **WHEN** a unit test serves a placement whose `filters.in_stock_only` is false with a lookup that records its calls
- **THEN** the lookup records zero calls and the candidates are ranked as before
- **VERIFIED BY**: `team-ai/tests/unit/test_in_stock_lookup.py` `no lookup without in_stock_only`

#### Scenario: Popular fallback is also stock-checked

- **WHEN** the ranking step raises and `Recommend` takes the popular fallback on an `in_stock_only` placement, with one popular listing at search stock 0
- **THEN** the fallback list excludes that listing, and with the lookup failing the list is empty
- **VERIFIED BY**: `team-ai/tests/unit/test_in_stock_lookup.py` `popular fallback is stock checked`

#### Scenario: The lookup carries the service principal

- **WHEN** a unit test captures the metadata of the outgoing `GetListingStock` call
- **THEN** it carries team-ai's service principal credential with scope `search:read`
- **VERIFIED BY**: `team-ai/tests/unit/test_in_stock_lookup.py` `lookup carries service principal with search read`

#### Scenario: Lookup without a valid principal fails closed

- **WHEN** team-search rejects the call with `UNAUTHENTICATED` or `PERMISSION_DENIED` (team-ai principal lacks `search:read`)
- **THEN** `Recommend` on an `in_stock_only` placement returns OK with zero items
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recsys/in_stock_recommendations.feature` `Lookup without a valid principal fails closed`

#### Scenario: Logs carry no listing ids

- **WHEN** a unit test serves an `in_stock_only` request whose lookup fails and captures the log output
- **THEN** the logs contain the outcome and a candidate count and none of the listing ids
- **VERIFIED BY**: `team-ai/tests/unit/test_in_stock_lookup.py` `logs carry outcome and no ids`

#### Scenario: Seed excluded when enabled

- **WHEN** `Recommend` is called on `similar_items` with `exclude_seed: true` and the seed is its own nearest neighbour
- **THEN** the seed is absent from the response
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recommendations/cart_cross_sell.feature` `Seed excluded when enabled`

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
