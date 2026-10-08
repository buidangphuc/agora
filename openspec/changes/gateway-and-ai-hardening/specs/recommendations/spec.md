## MODIFIED Requirements

### Requirement: Two-stage retrieval then ranking with business-rule filtering

The system SHALL produce recommendations in two stages: candidate retrieval of up to
`RECS_CANDIDATE_TOP_K` (default 100) items via Qdrant ANN over the collection populated by the
training job (seed addressed by the producer's point id and results identified by the payload
`listing_id`, see `recsys-serving-contract`), then ranking and business-rule filtering —
dropping candidates explicitly marked unavailable (`in_stock=false`) when the data carries that
flag, de-duplicating, and excluding the request's `seed_listing_id` — truncated to
`RECS_RESULT_TOP_K` (default 10). Authoritative stock filtering is the hydrating caller's job.

#### Scenario: Out-of-stock and seed items are filtered from candidates

- **WHEN** the recommend module retrieves ANN candidates that include an item marked
  `in_stock=false`, a duplicate, and the request's own `seed_listing_id`
- **THEN** those items are removed and the response contains at most ten distinct
  products, none of them the seed listing and none marked unavailable

### Requirement: Redis pre-computed cache fast path with Qdrant fallback

The system SHALL, for a logged-in `user_id`, first read the training job's pre-computed Top-N
list from Redis (`{RECS_CACHE_PREFIX}:{schema_ver}:user:{user_id}`) and serve it after applying
only the freshness filters (dedupe, seed exclusion, and unavailability when the entry carries
`in_stock=false`). On a cache miss, expired key, or Redis error, the
system SHALL fall back to the live two-stage Qdrant path and SHALL NOT fail the RPC on a cache
error.

#### Scenario: Cache hit serves without a Qdrant query

- **WHEN** `Recommend` is called for a user whose pre-computed list is present in Redis
- **THEN** the response is built from the cached list and no Qdrant ANN query is issued

#### Scenario: Cache miss falls back to Qdrant retrieval

- **WHEN** `Recommend` is called for a user with no cached list (or Redis is unreachable)
- **THEN** the module retrieves candidates from Qdrant and still returns a Top-10 result

### Requirement: Cold-start and anonymous requests still return a non-empty row

The system SHALL serve anonymous requests (empty `user_id`) and cold users (no cached list) via
Qdrant ANN seeded from `seed_listing_id` when present, otherwise via the training job's popularity
list in Redis (`{RECS_CACHE_PREFIX}:{schema_ver}:popular`), so a `Recommend` call returns a
non-empty product list whenever the producer's popularity list is loaded. When that list is absent
the popularity stage SHALL return an empty list rather than arbitrary catalog items.

#### Scenario: Anonymous request seeded from a listing returns similar items

- **WHEN** `Recommend` is called with an empty `user_id` and a `seed_listing_id`
- **THEN** the module returns items similar to the seed listing from Qdrant, excluding the seed
  and any item marked unavailable

#### Scenario: No user, no seed falls back to popular items

- **WHEN** `Recommend` is called with no `user_id` and no `seed_listing_id` and the popularity list is loaded
- **THEN** the module returns a non-empty popularity-based Top-10 rather than an empty response
