## 1. Code track

### team-search
- [ ] 1.1 Add `SEARCH_RANKER_VERSION` (default `v1`) to `team-search/internal/config/config.go` and wire it to the handler. Covers: ranker version scenarios under "model_version names the retrieval path that ran".
- [ ] 1.2 Change `engine.Execute` to return the effective mode and a `reranked` flag (replace the discarded fallback bool): lexical for fallback, key sort, deep paging, empty query and failed semantic leg; `reranked` only when the reranker reordered. Covers: all "model_version names the retrieval path that ran" scenarios.
- [ ] 1.3 In `rpc/search.go`, mint `request_id` (UUID v4, 32 lowercase hex, `crypto/rand`) and set `model_version` from 1.2; `lexical` when the engine is nil. Rebase on `search-bestseller-rating` edits to the same file. Covers: "Every search response carries a fresh request id", "The engine is not configured".
- [ ] 1.4 In `rpc/saved_search.go`, set `request_id` and `lexical@<version>` on `RunSavedSearchResponse`. Covers: saved search scenarios.
- [ ] 1.5 Unit tests for engine mode reporting and handler fields (rpc_spec_test style), including 1000-call uniqueness and anonymous caller. Covers: all team-search scenarios.

### team-ai
- [ ] 1.6 In `servicers/search.py`, set `request_id` (uuid4 hex) and `model_version` `semantic@<SEARCH_RANKER_VERSION>` (default `v1`); add the setting. Tests for fields, uniqueness, UNAVAILABLE and denied scope. Covers: "team-ai search fills the same attribution fields".

### team-frontend
- [ ] 1.7 Return `requestId` and `modelVersion` of the final response from `searchListings` (`src/lib/gateway/search.ts`) and keep them through `loadSearch` (`features/search/data.ts`). Covers: "Search page beacons carry the response identity".
- [ ] 1.8 In `SearchBlocks.tsx` pass `impressionId` and `modelVersion` to `ListingGrid` with `placementId="search_results"`; omit `impressionId` when empty. Vitest like `SearchBlocks.listId.test.tsx`; the scenario "A gateway response without an id still renders" is verified by Vitest only (no e2e). Covers: all beacon scenarios.

### team-analytics
- [ ] 1.9 Add `SearchPerformance` to `PerformanceRepository` and `DuckDBRepository`, sharing the SQL of `RecommendationPerformance` with placement fixed to `search_results`. Covers: all analytics scenarios except authz.
- [ ] 1.10 Implement `GetSearchPerformance` (contract from discovery-seo-contract; rows reuse `RecommendationPerformanceRow`) in `team-analytics/internal/rpc/handler.go` and `team-analytics/internal/rpc` tests: admin-only, window rules of discovery-seo-contract (0 means 24, max 168); do not copy the 168 default that GetRecommendationPerformance resolves today (team-analytics/internal/rpc/handler.go ~L436-447), `ctr`, `conversion_rate`, `attribution_window_hours`. Covers: row fields, window and authz scenarios, "CTR per model version", gateway `search_results` rows, `window_hours` 169 rejection (tests `TestGetSearchPerformanceRowsArePlacementSearchResults`, `TestGetSearchPerformanceRejectsWindowOver168`).
- [ ] 1.11 Repository tests with DuckDB fixtures for each analytics scenario. Covers: all analytics scenarios.

## 2. E2E track

- [ ] 2.0 Record in the build report the measured p95 cost of id minting and `GetSearchPerformance` 168 h latency (non-gating). Covers: NFR.

- [ ] 2.1 Feature `platform-e2e/tests/e2e/features/search/search_attribution.feature`: search through the gateway, assert `request_id` shape, new id on page 2, `model_version` per mode (lexical, key sort in hybrid), saved search run. Covers: request id and model_version requirements.
- [ ] 2.2 UI scenario: open `/search?q=...`, capture the tracking beacons, click a result, assert `impression_id`, `model_version` and `placement_id` on impression and click; re-sort changes the id. The empty `request_id` scenario is not covered here (Vitest only, task 1.8). Covers: "Search page beacons carry the response identity".
- [ ] 2.3 Analytics scenario: drive impressions and clicks, call the performance RPC as admin, assert CTR per `model_version`, and a seller gets `PERMISSION_DENIED`. Covers: "Analytics reports search performance per model version".
- [ ] 2.4 Add FEATURES entries (e2e-author owns `team-search/FEATURES.yaml`, `team-frontend/FEATURES.yaml`, `team-analytics/FEATURES.yaml`) citing these scenarios. Covers: all.
