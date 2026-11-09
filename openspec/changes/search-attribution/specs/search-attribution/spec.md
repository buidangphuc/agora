## ADDED Requirements

### Requirement: Every search response carries a fresh request id

`team-search` SHALL set `request_id` on every successful `SearchListingsResponse` and `RunSavedSearchResponse` to a newly generated UUID v4 rendered as 32 lowercase hex characters, one per call, independent of the query, the caller and the page.

#### Scenario: A search returns a request id
- **WHEN** a client calls `SearchListings` with query "laptop"
- **THEN** the response `request_id` matches `^[0-9a-f]{32}$`

#### Scenario: Paging gets a new request id
- **WHEN** a client calls `SearchListings` twice with the same query, once for page 1 and once with the `next_cursor` of page 1
- **THEN** the two responses carry different `request_id` values

#### Scenario: Repeating one search gets a new request id
- **WHEN** a client calls `SearchListings` 1000 times with identical parameters
- **THEN** the responses carry 1000 distinct `request_id` values

#### Scenario: A saved search run returns a request id
- **WHEN** the owner of a saved search calls `RunSavedSearch`
- **THEN** the response `request_id` matches `^[0-9a-f]{32}$` and differs from the `request_id` of the previous run of the same saved search

#### Scenario: A failed search returns no request id
- **WHEN** the index is unavailable and `SearchListings` fails with an error status
- **THEN** the call returns the error status and no response body
- **VERIFIED BY**: team-search `internal/rpc` `TestSearchAttributionFailedSearchReturnsNoResponse`

#### Scenario: A saved search of another user is rejected
- **WHEN** a user calls `RunSavedSearch` for a saved search owned by another user
- **THEN** the call fails with `NOT_FOUND` and no `request_id` is returned

#### Scenario: An anonymous caller can still search
- **WHEN** a caller with no principal calls `SearchListings`
- **THEN** the response carries a valid `request_id` and `model_version`

### Requirement: model_version names the retrieval path that ran

`team-search` SHALL set `model_version` on `SearchListingsResponse` and `RunSavedSearchResponse` to `<mode>[+rerank]@<SEARCH_RANKER_VERSION>`, where `<mode>` is `lexical`, `semantic` or `hybrid` and names the path that produced the returned hits, `+rerank` is present only when `<mode>` is `hybrid` and the cross-encoder reordered the hits (a page served by lexical, semantic or a fallback never carries `+rerank`), and `SEARCH_RANKER_VERSION` is read from the environment with default `v1`. `engine.Execute` SHALL report this mode to the handler instead of only a fallback flag.

#### Scenario: A hybrid search reports hybrid
- **WHEN** a client searches with `search_mode` hybrid, a non-empty query, sort relevance and page 1, and both legs succeed
- **THEN** `model_version` equals `hybrid@v1`

#### Scenario: The reranker is reported
- **WHEN** `ENABLE_RERANKER` is true and the reranker reorders the fused hits of a hybrid search
- **THEN** `model_version` equals `hybrid+rerank@v1`

#### Scenario: A reranker failure is not reported as rerank
- **WHEN** the reranker call fails and the fused order is kept
- **THEN** `model_version` equals `hybrid@v1`

#### Scenario: A fallback page never carries rerank
- **WHEN** `ENABLE_RERANKER` is true and a hybrid search is served by the lexical leg because the semantic leg failed
- **THEN** `model_version` equals `lexical@v1` and does not contain `+rerank`

#### Scenario: A lexical request reports lexical
- **WHEN** a client searches with `search_mode` lexical
- **THEN** `model_version` equals `lexical@v1`

#### Scenario: Semantic fallback reports lexical
- **WHEN** the embedding call fails during a semantic search and `Execute` falls back to the lexical index
- **THEN** `model_version` equals `lexical@v1`, not `semantic@v1`

#### Scenario: Hybrid with a failed semantic leg reports lexical
- **WHEN** the semantic leg of a hybrid search fails and the lexical results are returned
- **THEN** `model_version` equals `lexical@v1`

#### Scenario: A key sort in hybrid mode reports lexical
- **WHEN** a client requests `search_mode` hybrid with `sort_by` price ascending
- **THEN** `model_version` equals `lexical@v1`, because the lexical leg served the page

#### Scenario: Deep paging beyond the fusion window reports lexical
- **WHEN** a client requests hybrid mode with an offset of at least `HYBRID_FUSION_WINDOW`
- **THEN** `model_version` equals `lexical@v1`

#### Scenario: An empty query reports lexical
- **WHEN** a client searches with an empty query and `search_mode` hybrid
- **THEN** `model_version` equals `lexical@v1`

#### Scenario: The unspecified mode resolves to the mode that ran
- **WHEN** a client searches with `search_mode` unspecified, a non-empty query, hybrid enabled and the embedding client configured
- **THEN** `model_version` equals `hybrid@v1`

#### Scenario: The ranker version comes from the environment
- **WHEN** team-search starts with `SEARCH_RANKER_VERSION=v2` and a client runs a lexical search
- **THEN** `model_version` equals `lexical@v2`
- **VERIFIED BY**: team-search `internal/config` `TestSearchRankerVersionComesFromEnvironment`

#### Scenario: An unset ranker version defaults to v1
- **WHEN** team-search starts without `SEARCH_RANKER_VERSION`
- **THEN** `model_version` ends with `@v1`

#### Scenario: A saved search run reports lexical
- **WHEN** the owner calls `RunSavedSearch`
- **THEN** `model_version` equals `lexical@v1`, because the run uses the lexical index search

#### Scenario: The engine is not configured
- **WHEN** the handler has no retrieval engine and falls back to the plain index search
- **THEN** `model_version` equals `lexical@v1`
- **VERIFIED BY**: team-search `internal/rpc` `TestSearchAttributionModelVersionPerPath`

### Requirement: team-ai search fills the same attribution fields

The `SearchListings` servicer of `team-ai` SHALL set `request_id` to a fresh UUID v4 as 32 lowercase hex characters per call and `model_version` to `semantic@<SEARCH_RANKER_VERSION>`, reading the same env var with default `v1`.

#### Scenario: team-ai returns both fields
- **WHEN** a client with scope `search:read` calls team-ai `SearchListings` with a query that has matches
- **THEN** the response `request_id` matches `^[0-9a-f]{32}$` and `model_version` equals `semantic@v1`
- **VERIFIED BY**: team-ai `tests/unit/transport/test_search_servicer.py` `test_sa_team_ai_search_sets_request_id_as_uuid4_hex`

#### Scenario: Two team-ai calls differ
- **WHEN** a client calls team-ai `SearchListings` twice with the same query
- **THEN** the two `request_id` values differ
- **VERIFIED BY**: team-ai `tests/unit/transport/test_search_servicer.py` `test_sa_two_team_ai_calls_differ`

#### Scenario: team-ai search unavailable
- **WHEN** the retrieval backend is not configured
- **THEN** the call fails with `UNAVAILABLE` and the response carries no `request_id`
- **VERIFIED BY**: team-ai `tests/unit/transport/test_search_servicer.py` `test_sa_store_failure_returns_no_attribution_fields`

#### Scenario: team-ai search without scope is denied
- **WHEN** a caller without scope `search:read` calls team-ai `SearchListings`
- **THEN** the call fails with `PERMISSION_DENIED`
- **VERIFIED BY**: team-ai `tests/unit/transport/test_search_servicer.py` `test_sa_denied_scope_returns_no_attribution_fields`

### Requirement: Search page beacons carry the response identity

The `/search` page SHALL pass `SearchListingsResponse.request_id` of the response that produced the visible page as `impressionId` and `SearchListingsResponse.model_version` as `modelVersion` to `ListingGrid`, so the impression and click beacons of every result card carry `placement_id=search_results`, `impression_id` and `model_version`.

#### Scenario: Impression and click carry the identity
- **WHEN** a buyer opens `/search?q=laptop` and clicks the first result
- **THEN** the impression beacons and the click beacon each carry `impression_id` equal to the page's response `request_id` and `model_version` equal to the response `model_version`

#### Scenario: Re-sorting gets a new impression id
- **WHEN** a buyer opens `/search?q=laptop` and then picks a different sort
- **THEN** the second page's beacons carry an `impression_id` different from the first page's

#### Scenario: Paging gets a new impression id
- **WHEN** a buyer moves from page 1 to page 2 of `/search?q=laptop`
- **THEN** the page 2 beacons carry an `impression_id` different from page 1 and equal to the `request_id` of the response that returned page 2

#### Scenario: A fallback is labelled with the mode that ran
- **WHEN** the semantic leg is unavailable and the lexical results are shown
- **THEN** the beacons carry a `model_version` that starts with `lexical@`

#### Scenario: A failed search sends no beacons
- **WHEN** the search call fails and the page shows the error state
- **THEN** no impression beacon is sent
- **VERIFIED BY**: team-frontend `src/features/search/SearchBlocks.attribution.test.tsx` `[Search page beacons carry the response identity] a failed search sends no beacons`

#### Scenario: An empty result sends no impression
- **WHEN** a buyer searches for a term with no matches
- **THEN** the page renders the empty state and sends no impression beacon

#### Scenario: A gateway response without an id still renders
- **WHEN** the response carries an empty `request_id`
- **THEN** the page renders the results and sends beacons without `impression_id`
- **VERIFIED BY**: team-frontend `src/features/search/SearchBlocks.attribution.test.tsx` `[A gateway response without an id still renders] a response with no attribution fields renders the listings and beacons without impressionId`

### Requirement: Analytics reports search performance per model version

`team-analytics` SHALL provide a `SearchPerformance` repository query, mirroring `RecommendationPerformance`, that for placement `search_results` returns per `model_version` the distinct impression ids, item impressions, clicks, add-to-carts and purchases, using the same join on `impression_id` and `listing_id`, the same one-impression-per-click credit rule and the same attribution window. The admin-only RPC `GetSearchPerformance` SHALL return `window_hours` (0 in the request resolves to 24, maximum 168), `attribution_window_hours` equal to the value `GetRecommendationPerformance` reports, and one `RecommendationPerformanceRow` per `model_version` with `placement_id` equal to `search_results`, `model_version`, `impressions`, `item_impressions`, `clicks`, `add_to_carts`, `purchases`, `ctr` (clicks divided by item impressions) and `conversion_rate` (purchases divided by clicks whose attribution window has closed, as in RecommendationPerformance).

#### Scenario: CTR per model version
- **GIVEN** 10 item impressions and 2 clicks with `impression_id` A, placement `search_results`, `model_version` `hybrid@v1`, and 10 item impressions and 1 click with `impression_id` B, `model_version` `lexical@v1`
- **WHEN** an admin calls the search performance RPC over a window that contains them
- **THEN** the response has a row for `hybrid@v1` with `ctr` 0.2 and a row for `lexical@v1` with `ctr` 0.1

#### Scenario: Only search results are counted
- **GIVEN** impressions under placement `similar_items` with a `model_version` of `als-v3`
- **WHEN** an admin calls the search performance RPC
- **THEN** no row for `als-v3` appears

#### Scenario: A click on a listing the search did not show is not credited
- **WHEN** a click carries an `impression_id` of a search impression but a `listing_id` that the impression did not show
- **THEN** the click adds to no row

#### Scenario: A click before its impression is not credited
- **WHEN** a click event has `occurred_at` earlier than the matching impression
- **THEN** the click adds to no row
- **VERIFIED BY**: team-analytics `internal/query` `TestSearchPerformance_ClickBeforeImpressionIsNotCredited`

#### Scenario: A reused impression id under another model is separate
- **WHEN** the same `impression_id` appears with two different `model_version` values
- **THEN** the response has two rows, one per model version

#### Scenario: Purchases come from server order facts
- **WHEN** a purchase beacon claims a search click but no PAID order line of that user and listing exists in the attribution window
- **THEN** the row's `purchases` stays 0

#### Scenario: Row fields and attribution window
- **GIVEN** 10 item impressions and 4 clicks of `hybrid@v1` whose attribution window has closed, and 2 PAID order lines of those clickers on the clicked listings
- **WHEN** an admin calls `GetSearchPerformance`
- **THEN** the `hybrid@v1` row has `placement_id` `search_results`, `clicks` 4, `purchases` 2, `ctr` 0.4, `conversion_rate` 0.5, and the response `attribution_window_hours` equals the value `GetRecommendationPerformance` returns
- **VERIFIED BY**: team-analytics `internal/query` `TestSearchPerformance_RowFieldsAndAttributionWindow`

#### Scenario: A zero window resolves to 24 hours
- **WHEN** an admin calls `GetSearchPerformance` with `window_hours` 0
- **THEN** the response `window_hours` equals 24

#### Scenario: An admin through the gateway receives search_results rows
- **WHEN** an admin calls `GetSearchPerformance` through the gateway after search impressions exist
- **THEN** every returned row has `placement_id` equal to `search_results`
- **VERIFIED BY**: team-analytics `internal/rpc` `TestGetSearchPerformanceRowsArePlacementSearchResults`; e2e `search/search_attribution.feature`

#### Scenario: window_hours 169 is rejected by team-analytics
- **WHEN** an admin calls `GetSearchPerformance` with `window_hours` 169
- **THEN** team-analytics fails the call with `INVALID_ARGUMENT`
- **VERIFIED BY**: team-analytics `internal/rpc` `TestGetSearchPerformanceRejectsWindowOver168`; e2e `search/search_attribution.feature`

#### Scenario: An empty window returns no rows
- **WHEN** an admin calls the RPC over a window with no search impressions
- **THEN** the response has zero rows and no error
- **VERIFIED BY**: team-analytics `internal/query` `TestSearchPerformance_EmptyWindowReturnsNoRows`

#### Scenario: window_hours out of range is rejected
- **WHEN** an admin calls the RPC with `window_hours` greater than 168
- **THEN** the call fails with `INVALID_ARGUMENT`

#### Scenario: The window boundary is inclusive
- **WHEN** an admin calls the RPC with `window_hours` 168 and an impression occurred exactly 168 hours earlier
- **THEN** that impression counts
- **VERIFIED BY**: team-analytics `internal/query` `TestSearchPerformance_WindowBoundaryIsInclusive`

#### Scenario: A non-admin is denied
- **WHEN** a seller or buyer principal calls the search performance RPC
- **THEN** the call fails with `PERMISSION_DENIED`

#### Scenario: An unauthenticated caller is denied
- **WHEN** a caller with no principal calls the search performance RPC
- **THEN** the call fails with `UNAUTHENTICATED`
