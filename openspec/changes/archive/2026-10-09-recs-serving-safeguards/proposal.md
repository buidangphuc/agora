## Why

AI-first change 7 of 8. The serving path in team-ai has the gaps the 2026-10-08 audit listed, plus the recommendation
items moved here from the retired `gateway-and-ai-hardening`.
- **Any cache or Qdrant error fails the request.** `Recommend` aborts with `UNAVAILABLE`, so a hiccup in Redis or Qdrant
  blanks every recommendation row on the storefront.
- **Cold start is random.** Users without recommendations get "popular" items from an arbitrary Qdrant scroll, not the
  popularity list the producer computes.
- **Serving can use the in-process memory backend anywhere.** In a deployed environment that serves a fake catalogue.
- **One entrypoint is missing Recommend.** `scripts/run_grpc.py` starts a gRPC server without the recommendation service.
- **The frontend cannot attribute recommendations precisely.** The response carries no placement and no server request
  id, so impressions and clicks cannot be joined to what the server served. The online evaluation in change 8 needs that
  join.
- **Online features are never used.** `featurestore-materialization` now writes features to Redis, but ranking still uses
  an `InMemoryFeatureStore`.

## What Changes

**platform-core (proto, additive).** `RecommendResponse` gains `placement_id = 3` (the placement the server served) and
`request_id = 4` (a server-minted id for this response). Vendored into team-ai, team-gateway and team-frontend.

**team-ai**
- `Recommend` never fails because of the cache or Qdrant. On an error it serves the serving generation's popular list,
  or an empty list when that is unavailable too. It returns `OK` with `model_version` `serving-fallback`.
- Cold start uses the serving generation's popular list (`recs:v1[:gen:<g>]:popular`) instead of a Qdrant scroll.
- `RECS_BACKEND=memory` is refused at boot outside dev, local and test.
- `scripts/run_grpc.py` registers the recommendation service, the same as the main entrypoint.
- Every response carries `placement_id` and a fresh `request_id`. One structured log line `recs.served` per response
  records request id, placement, model version, item ids and whether it was a fallback.
- Ranking reads item features from the feature store's online keys (`fs:item_popularity:current` and
  `fs:item_popularity:v<n>:<listing_id>`) when `RECS_FEATURESTORE_REDIS_URL` is set. It degrades to the current behaviour
  when keys are missing.
- The two-stage ranking boosts candidates by `ctr_7d` and `favorites_current`.

**team-frontend.** Recommendation rows use the response's `request_id` as the `impressionId`, and `placement_id` as the
`placementId`, on their impression and click beacons.

**platform-e2e.** Scenarios through the gateway and the storefront.

Repos touched: platform-core, team-ai, team-gateway (re-vendor only), team-frontend, platform-e2e. **Proto change:
additive.**

## Capabilities

### New Capabilities
- `recs-serving-safeguards`: how recommendation serving degrades, how it cold-starts, which backends it refuses, how a
  response identifies itself for attribution, and how it uses online features.

### Modified Capabilities
- None. The `recommendations` capability's existing requirements are unchanged; this adds the serving safeguards.

## Non-goals

- **Server-side eligibility filtering through team-search.** The storefront already hydrates every recommended id and
  drops missing or out-of-stock listings before rendering. A server-side filter would duplicate it at the cost of a
  cross-service call per request.
- **Online metrics and reports.** That is `recsys-online-evaluation` (change 8).
- **Two-tower or GBDT serving changes.**

## Impact

- **New team-ai setting:** `RECS_FEATURESTORE_REDIS_URL`, empty by default (feature store off).
- **Boot refusal:** `RECS_BACKEND=memory` is refused outside local.
- **Frontend beacons:** `impressionId` changes from a client-minted id to the server `request_id` for recommendation rows
  only.
