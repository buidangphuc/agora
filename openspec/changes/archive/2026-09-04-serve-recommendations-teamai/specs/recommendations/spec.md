## ADDED Requirements

### Requirement: team-ai serves the Recommend RPC over gRPC

The system SHALL implement `platform.recommendation.v1.RecommendationService/Recommend` in
`team-ai` as a new recommend module exposed on team-ai's gRPC listen port, returning up to ten
recommended products for the request's `user_id` / `seed_listing_id`. The servicer SHALL be a
thin transport (parse, scope-check, map) over the module, holding no retrieval or ranking logic
itself.

#### Scenario: Recommend returns a Top-10 product list

- **WHEN** a caller invokes `RecommendationService/Recommend` with a `user_id` while
  `RECS_ENABLED=true`
- **THEN** team-ai returns a `RecommendResponse` carrying at most ten recommended product cards
  produced by the recommend module (not an empty or mocked response)

### Requirement: Recommendations are gated behind RECS_ENABLED

The system SHALL provide the recommend module to the servicer only when `RECS_ENABLED=true`.
When the flag is `false`, `Recommend` SHALL abort with gRPC status `UNAVAILABLE` and a message
naming the flag, mirroring how SearchService behaves under `RAG_ENABLED=false`.

#### Scenario: Recommend is unavailable when the flag is off

- **WHEN** a caller invokes `Recommend` while `RECS_ENABLED=false`
- **THEN** the call fails with `UNAVAILABLE` and a message indicating recommendations are not
  enabled, and no Qdrant or Redis access is attempted

### Requirement: Two-stage retrieval then ranking with business-rule filtering

The system SHALL produce recommendations in two stages: candidate retrieval of up to
`RECS_CANDIDATE_TOP_K` (default 100) items via Qdrant ANN over the collection populated by the
training job, then ranking and business-rule filtering — dropping out-of-stock items,
de-duplicating, and excluding the request's `seed_listing_id` — truncated to
`RECS_RESULT_TOP_K` (default 10).

#### Scenario: Out-of-stock and seed items are filtered from candidates

- **WHEN** the recommend module retrieves ANN candidates that include an out-of-stock item, a
  duplicate, and the request's own `seed_listing_id`
- **THEN** those items are removed and the response contains at most ten distinct in-stock
  products, none of them the seed listing

### Requirement: Redis pre-computed cache fast path with Qdrant fallback

The system SHALL, for a logged-in `user_id`, first read the training job's pre-computed Top-N
list from Redis (`{RECS_CACHE_PREFIX}:{schema_ver}:user:{user_id}`) and serve it after applying
only the freshness filters (in-stock, dedupe). On a cache miss, expired key, or Redis error, the
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
Qdrant ANN seeded from `seed_listing_id` when present, otherwise via a popularity fallback, so a
`Recommend` call returns a non-empty product list whenever the catalog is non-empty.

#### Scenario: Anonymous request seeded from a listing returns similar items

- **WHEN** `Recommend` is called with an empty `user_id` and a `seed_listing_id`
- **THEN** the module returns items similar to the seed listing from Qdrant, filtered to in-stock
  and excluding the seed

#### Scenario: No user, no seed falls back to popular items

- **WHEN** `Recommend` is called with no `user_id` and no `seed_listing_id`
- **THEN** the module returns a non-empty popularity-based Top-10 rather than an empty response

### Requirement: Recommend stays within the latency budget

The system SHALL answer `Recommend` within a serve-path budget of under 15ms (p99, excluding
network to the caller), making at most one datastore round trip on the hot path — all
business-rule fields read from the Qdrant payload or the Redis value — and SHALL cap the Qdrant
path with `RECS_RETRIEVE_TIMEOUT_MS`, returning the popularity fallback on timeout rather than
exceeding budget.

#### Scenario: Retrieval timeout yields a fallback, not an error

- **WHEN** the Qdrant retrieval exceeds `RECS_RETRIEVE_TIMEOUT_MS`
- **THEN** `Recommend` returns the popularity fallback list instead of failing or blocking past
  the budget
