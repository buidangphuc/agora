## Why

Search results cannot be attributed. `SearchListingsResponse` carries no `request_id` or `model_version`, the `/search` page renders `ListingGrid placementId="search_results"` with no `impressionId` or `modelVersion`, and `engine.Execute` returns a fallback flag that `team-search/internal/rpc/search.go` discards (`res, _, err = h.engine.Execute(...)`). Impression and click beacons from search therefore cannot be joined to the query that produced them, and ranking versions (lexical, hybrid, rerank) cannot be compared, as recommendations already can through `RecommendationPerformance`. Backlog Q26, evidence `ai-data.md` #9.

initiative: discovery-seo

## What Changes

- team-search: `SearchListings` and `RunSavedSearch` mint a `request_id` (UUID v4, 32 lowercase hex characters) per call and return `model_version` = `<mode>[+rerank]@<SEARCH_RANKER_VERSION>`. `<mode>` is the retrieval path that produced the hits (`lexical`, `semantic` or `hybrid`). `+rerank` appears only when the cross-encoder reordered the page. `SEARCH_RANKER_VERSION` is a new env var, default `v1`.
- team-search retrieval: `engine.Execute` reports the mode that actually ran, not only a fallback bool, so a semantic or hybrid request that fell back, or was served by the lexical leg (key sort, deep paging, empty query), is labelled `lexical`. `RunSavedSearch` always runs lexical and reports `lexical@<version>`.
- team-ai: the `SearchListings` servicer fills `request_id` and `model_version` (`semantic@<SEARCH_RANKER_VERSION>`, same env var and format).
- team-frontend: `searchListings` in `src/lib/gateway/search.ts` returns `requestId` and `modelVersion` of the response that produced the page; `SearchBlocks.tsx` passes them to `ListingGrid` as `impressionId` and `modelVersion`, so `TrackImpression` and `TrackLink` send them as `RecommendationsRow.tsx` does.
- team-analytics: new `SearchPerformance` repository query mirroring `RecommendationPerformance` for placement `search_results`, exposed as the admin-only `GetSearchPerformance` RPC.

## Capabilities

### New Capabilities
- `search-attribution`: request identity and ranker version on every search response, carried by beacons, aggregated per model version.

### Modified Capabilities
none

## Impact

- `team-search/internal/rpc/search.go`, `team-search/internal/rpc/saved_search.go`, `team-search/internal/retrieval/engine.go`, `team-search/internal/config/config.go`, `team-search/FEATURES.yaml`.
- `team-ai/app/transport/grpc/servicers/search.py` and its settings.
- `team-frontend/src/lib/gateway/search.ts`, `src/features/search/data.ts`, `src/features/search/SearchBlocks.tsx`, `team-frontend/FEATURES.yaml`.
- `team-analytics/internal/query/repository.go`, `internal/query/duckdb.go`, `internal/rpc/handler.go`.
- Hot files shared with `search-bestseller-rating` (`rpc/search.go`, `index/opensearch.go`, `SearchBlocks.tsx`): this slice is serialised with it by initiative-build and must rebase on its handler edits.
- Depends on `discovery-seo-contract` for `request_id` and `model_version` on both search responses, for the admin-only `GetSearchPerformance` RPC (request `window_hours`, 0 means 24, maximum 168; response `window_hours`, `attribution_window_hours`, `repeated RecommendationPerformanceRow rows`), and for regenerated stubs in team-search, team-ai, team-frontend and team-analytics.

## Non-goals

Learning-to-rank and `rank_training` rows for search; a dashboard UI for the new query; changing the `TrackingEvent` schema (fields 11 and 12 already exist); persisting `request_id` server-side.

## Non-functional requirements

- `request_id` is unique per call: 1000 consecutive calls return 1000 distinct ids (gating, scenario "Repeating one search gets a new request id").
- Measured and non-gating: minting the id and building the version string adds at most 1 ms to `SearchListings` p95 on the local stack; `GetSearchPerformance` over a 168 h window answers in under 2 s on the local warehouse. Both are recorded in the build report, no scenario gates on them.
- Search availability is unchanged: no new network dependency on the request path.

## Data classification

| field | personal data | purpose | consent basis | retention |
|---|---|---|---|---|
| `request_id` | no: random id of one query, not derived from user, session or query text | join a click to the impression it came from | not personal; not applicable | same as the `TrackingEvent` row that carries it |
| `model_version` | no: ranker label, for example `hybrid+rerank@v1` | compare ranking versions | not personal; not applicable | same as the `TrackingEvent` row |
| `SearchPerformance` rows | no: aggregates per `model_version`, no user key in the output | ranking evaluation | not personal; not applicable | not stored; computed on request |

The beacons that carry these fields already carry the user or anonymous id under the existing consent rules; this slice adds no new identifier of a person and no query text to the beacon.

## Threat model

- Assets: ranking-experiment integrity (CTR per version), the admin-only analytics RPC.
- Abuse cases:
  - A client forges `impressionId` or `modelVersion` in a beacon to skew CTR. Mitigation: the join already requires a matching impression event with the same `impression_id` and `listing_id` that precedes the click; a forged id with no impression is not counted. Residual risk of forged impressions is accepted and equals recommendations today.
  - A guessable `request_id` lets one visitor poison another's attribution. Mitigation: 122 random bits from `crypto/rand`; not sequential.
  - A non-admin reads ranking performance. Mitigation: `GetSearchPerformance` RPC requires the `admin` scope, as `GetRecommendationPerformance` does.
  - The id leaks the query. Mitigation: it is random and carries no query text.
- Authz rule: `SearchListings` keeps `search:read` with anonymous allowed; `RunSavedSearch` keeps its owner check; `GetSearchPerformance` is admin only.
