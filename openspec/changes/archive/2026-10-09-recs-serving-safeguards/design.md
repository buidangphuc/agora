## Context

See proposal.md for the motivation. Current code as of 2026-10-09, after `recsys-generation-publish`:

- **`team-ai/app/transport/grpc/servicers/recommend.py`**
  - Binds the caller.
  - Maps the context to a placement.
  - Calls `RecommendationService.recommend`.
  - Aborts with `UNAVAILABLE` on `ServiceUnavailableError`.
- **`app/modules/business/recommend/`**
  - `service.py` is the two-stage pipeline: candidate retrieval from the cache or the backend, then ranking with a
    `FeatureStorePort`. The factory wires `InMemoryFeatureStore`.
  - `cache.py` is the pointer-aware `PrecomputedCache`: user and popular lists, plus `model_version`.
  - `backends.py` has `MemoryBackend` and `QdrantBackend`. `QdrantBackend.popular` scrolls the collection.
- **`scripts/run_grpc.py`** calls `serve(settings, rag_provider, chat_streamer)` without the recommendation provider that
  the main bootstrap passes.
- **Frontend.** `src/lib/gateway/recommendations.ts` calls `recommend` and hydrates the results. Rows pass
  `impressionId`/`placementId` to `ListingCard`, which emits the beacons.

## Goals / Non-Goals

**Goals:**
- Serving never errors on a dependency failure.
- Cold start is deterministic.
- Every response is attributable.
- Online features are used.

**Non-Goals:** server-side eligibility, online metrics, and changes to two-tower or GBDT serving.

## Decisions

### D1. Fallback in the service, not the servicer
- `RecommendationService.recommend` wraps both stages.
- On any cache or backend exception it calls `_fallback(limit)`, which returns:
  - the cache's popular list if it can be read;
  - otherwise the backend's `popular`;
  - otherwise `[]`.
- It returns `model_version="serving-fallback"` and `fallback=True`.
- The servicer keeps `UNAVAILABLE` only for `RECS_ENABLED=false`.
- The unrecoverable "no data at all" case is an empty list, which the storefront already hides.

### D2. Cold start from the producer's popular list
- `QdrantBackend.popular` is replaced by `PrecomputedCache.get_popular_candidates()`. That reads the serving
  generation's popular list, or the unscoped one when there is no pointer.
- The backend scroll is removed. `MemoryBackend.popular` is unchanged, for local use.

### D3. Proto and attribution
- `RecommendResponse.placement_id` (3) and `request_id` (4).
- `request_id` is `uuid4().hex`, minted in the servicer per response.
- `placement_id` is the placement the service actually used. It falls back to `""` when it is unknown.
- `recs.served` is one loguru line with structured extras: `request_id`, `placement_id`, `model_version`,
  `listing_ids` (a list), `fallback` and `principal_type`.
- The gateway only re-vendors. Connect passes the new fields through.
- Frontend:
  - `getRecommendations` returns `{items, requestId, placementId, modelVersion}`.
  - The row components pass `impressionId=requestId` and `placementId`.
  - Other callers of `ListingCard` are unchanged.

### D4. Online features
- `RedisFeatureStore(FeatureStorePort)` reads `GET fs:item_popularity:current` (memoised 5 s), then
  `MGET fs:item_popularity:v<n>:<id>…`.
- It decodes the flat JSON. Missing keys give `{}`.
- Any Redis error returns `{}` for the whole batch and is logged once per minute.
- Ranking sorts by the model score, then `ctr_7d` descending, then `favorites_current` descending.
  - Features only break ties and add a bounded boost: `score += 0.05 * ctr_7d`, capped. A single model score therefore
    stays dominant.
  - A fake catalogue with equal scores makes the tie-break observable in tests and e2e.
- The factory wires `RedisFeatureStore` when `RECS_FEATURESTORE_REDIS_URL` is set, else `InMemoryFeatureStore`.
- Local compose sets it to `redis://redis:6379/2`, which is the feature store's DB.

### D5. Boot refusal and entrypoint
- `validate_runtime_safety` refuses `RECS_ENABLED and RECS_BACKEND=="memory" and not ENVIRONMENT.is_local`.
- `scripts/run_grpc.py` builds the recommendation provider through the same factory call as `app/bootstrap`. That code
  is extracted into one helper used by both entrypoints.

## Risks / Trade-offs

- **[A permanent fallback hides a broken pipeline]** → Mitigation: every fallback is logged (`fallback=true`), and change 8
  reports fallback rate per placement.
- **[Feature boost changes ranking quality]** → Mitigation: the boost is bounded and acts mostly as a tie-break. The
  promotion gate still evaluates models offline.

## Migration Plan

- Deploy in this order: proto and vendoring, then team-ai, then team-frontend.
- An old frontend ignores the new fields.
- Rollback is a revert of the images.
