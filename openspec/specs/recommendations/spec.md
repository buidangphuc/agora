# recommendations Specification

## Purpose
TBD - created by archiving change surface-recommendations. Update Purpose after archive.

## Requirements

### Requirement: Gateway forwards Recommend to team-ai without business logic

The system SHALL expose `platform.recommendation.v1.RecommendationService/Recommend` at the
gateway as a read-only forwarder to team-ai's gRPC service, verifying auth once and forwarding
`x-principal-{id,type,scopes}` downstream, holding no retrieval, ranking, or filtering logic
itself (architecture Rules 1–2). The frontend SHALL reach recommendations only through this
gateway path, never by calling team-ai directly.

#### Scenario: Recommend routes through the gateway to team-ai

- **WHEN** the frontend calls `Recommend` through the gateway with the caller's session
- **THEN** the gateway forwards the request to team-ai over gRPC with the forwarded
  `x-principal-*` metadata and returns team-ai's product list unchanged (no gateway-side business
  logic)

### Requirement: A "Gợi ý cho bạn" recommendations row is shown to buyers

The system SHALL render a **"Gợi ý cho bạn"** recommendations row, on the home page and/or the
product-detail page, populated from `team-ai` (`RecommendationService/Recommend`) via the gateway
using the caller's session, displaying up to ten product cards. On the product-detail page the
row SHALL be seeded with the current listing id; the browser SHALL never call team-ai directly.

#### Scenario: Logged-in buyer sees a recommendations row sourced from team-ai

- **WHEN** a logged-in buyer opens the page carrying the recommendations row
- **THEN** the "Gợi ý cho bạn" row is populated with product cards sourced from team-ai via the
  gateway (not a client-side mock or hardcoded list)

#### Scenario: Recommendations are unavailable without breaking the page

- **WHEN** the recommendation service returns `UNAVAILABLE` (e.g. `RECS_ENABLED=false`)
- **THEN** the page still renders and the "Gợi ý cho bạn" row is hidden or empty rather than
  erroring the whole page

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

### Requirement: A recommendation serving contract exists in platform-core

The platform SHALL define a `platform.recommendation.v1.RecommendationService` gRPC service in
`platform-core/packages/proto` with a single `Recommend(RecommendRequest) returns (RecommendResponse)`
RPC, additively (a new package/file) so no existing contract is renumbered, removed, or otherwise
broken. The contract SHALL follow the platform's proto conventions (buf v2, STANDARD lint with the
enum-value-prefix rule, reuse of `platform.common.v1`) and SHALL be the single source of truth for
the online recommendation RPC (Rule 4).

#### Scenario: The recommendation proto lints and stays non-breaking

- **WHEN** `buf lint` and `buf breaking` run over `packages/proto` after adding
  `platform/recommendation/v1/recommendation.proto`
- **THEN** lint passes (STANDARD rules, enum values prefixed `RECOMMENDATION_CONTEXT_*`) and the
  breaking check passes because the change only adds a new package and touches no existing message,
  field number, or RPC

### Requirement: Recommend request carries caller identity, an optional seed, context, and a limit

`RecommendRequest` SHALL let a caller ask for recommendations as either an authenticated user
(`user_id`) or an anonymous visitor (`anonymous_id`), SHALL allow an optional `seed_listing_id` to
anchor item-to-item contexts (PDP "similar items", cart), SHALL carry a `RecommendationContext`
enum identifying where the recommendation is shown, and SHALL accept a `limit` (0 = server default).

#### Scenario: An anonymous PDP "similar items" request is expressible

- **WHEN** a caller builds a `RecommendRequest` with an empty `user_id`, an `anonymous_id`, a
  `seed_listing_id` set to the viewed listing, `context = RECOMMENDATION_CONTEXT_SIMILAR_ITEMS`, and
  `limit = 12`
- **THEN** the message is valid under the contract, so the serving layer has every field it needs to
  choose an item-item strategy for an anonymous visitor without a schema change

### Requirement: Recommend response returns ranked listing ids with scores and a model version

`RecommendResponse` SHALL return a `repeated RecommendedItem`, each carrying a `listing_id`, a
`score` (higher = more relevant), and a 1-based `rank`, ordered best-first, plus a `model_version`
identifying the offline artifact that produced the ranking. The response SHALL carry **listing ids
and scores only** (not hydrated listing cards), so the recommendation side never owns listing
content and the serving layer hydrates cards from the owning services (Rule 3).

#### Scenario: A ranked result set is expressible with provenance

- **WHEN** the serving layer fills a `RecommendResponse` with ordered `RecommendedItem`s and the
  `model_version` of the batch artifact it read
- **THEN** the consumer can render the ranking in order using each `listing_id`/`rank`, sort/threshold
  on `score`, and trace which offline model produced the result via `model_version`

### Requirement: An offline batch job trains ALS from the behavioral warehouse

The platform SHALL provide a `platform-recsys` PySpark batch job that reads the behavioral
warehouse (`tracking_events` — DuckDB/Parquet local, BigQuery prod), maps rows over a rolling
interaction window to implicit-feedback `(user, item, weight)` triples (user = `principal_id` when
present else `anonymous_id`; item = `listing_id`; weight derived from `event_type`), and fits a
Spark MLlib ALS model with `implicitPrefs=true`. The job SHALL read only the warehouse and write
only its own artifact stores (Rule 3), and SHALL run as a scheduled offline batch — not on the
request path.

#### Scenario: The job trains a model from sample warehouse data

- **WHEN** the job runs against a sample `tracking_events` dataset containing view/click/add-to-cart
  events for several users and listings
- **THEN** it produces an ALS model with item factors and user factors, having skipped rows with an
  empty `listing_id` and collapsed events to weighted per-(user,item) interactions

### Requirement: The job publishes item and user vectors to Qdrant

The job SHALL load the ALS **item factors** and **user factors** into Qdrant (`:6333`, the instance
`team-ai` already targets) as two collections — item vectors (for item-item similarity) and user
vectors (for personalized "for-you") — each point payload carrying the source id and the
`model_version` of the run, so the online serving layer can nearest-neighbor query them.

#### Scenario: A training run populates the Qdrant collections

- **WHEN** the job finishes training on the sample dataset and loads its outputs
- **THEN** the item-vector and user-vector Qdrant collections exist and are populated with one point
  per trained listing / user, each stamped with the run's `model_version`
  <!-- backend integration assertion: job runs on sample warehouse data → Qdrant collection populated -->

### Requirement: The job writes a precomputed recommendation cache to Redis

The job SHALL write a precomputed top-N recommendation cache into Redis (`:6379`): per-user ranked
recommendations, per-listing similar items, and the current `model_version`, with a TTL longer than
the batch cadence so a missed run degrades gracefully rather than emptying the cache. The cache
SHALL hold listing ids and scores only (no hydrated listing content, Rule 3).

#### Scenario: A training run populates the Redis cache

- **WHEN** the job finishes loading its outputs
- **THEN** Redis holds a per-user recommendation entry and a per-listing similar-items entry (ranked
  listing ids + scores) plus a model-version key identifying the generation just written

### Requirement: The job runs locally without a Spark cluster

The job SHALL run in Spark local mode (no cluster) reading the DuckDB-exported Parquet warehouse and
writing to local Qdrant/Redis from `platform-core`'s compose, so the full train→load loop is
runnable on a developer laptop and in CI on a small sample dataset.

#### Scenario: A developer runs the job end-to-end locally

- **WHEN** a developer runs the job in local mode against the sample Parquet warehouse with local
  Qdrant (`:6333`) and Redis (`:6379`) running
- **THEN** the job completes without a Spark cluster and leaves the Qdrant collections and Redis
  cache populated for that sample
