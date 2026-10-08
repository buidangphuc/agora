## MODIFIED Requirements

### Requirement: A "Gợi ý cho bạn" recommendations row is shown to buyers

The system SHALL render recommendation rows populated from `team-ai` (`RecommendationService/Recommend`)
via the gateway using the caller's session: on the home page the **"Gợi ý cho bạn"** row (placement
`home.for_you`) and a trending row (placement `home.trending`), and on the product-detail page a similar-items
row (placement `pdp.similar`) seeded with the current listing id, each displaying up to ten product cards.
Each row SHALL request its own `placement_id`, and SHALL log, through the platform tracking SDK, one
impression per card shown and a click when a card is opened, each carrying the response's `placement_id`,
`request_id`, `model_version` and the card's `position` (its rank). The browser SHALL never call team-ai
directly.

#### Scenario: Logged-in buyer sees a recommendations row sourced from team-ai

- **WHEN** a logged-in buyer opens the page carrying the recommendations row
- **THEN** the "Gợi ý cho bạn" row is populated with product cards sourced from team-ai via the
  gateway (not a client-side mock or hardcoded list)

#### Scenario: Recommendations are unavailable without breaking the page

- **WHEN** the recommendation service returns `UNAVAILABLE` (e.g. `RECS_ENABLED=false`)
- **THEN** the page still renders and the "Gợi ý cho bạn" row is hidden or empty rather than
  erroring the whole page

#### Scenario: The home page shows a trending row

- **WHEN** a visitor opens the home page while the catalog holds eligible listings
- **THEN** a trending row is rendered from a `Recommend` call with placement `home.trending`, next to the
  "Gợi ý cho bạn" row

#### Scenario: Impressions and clicks carry the serving attribution

- **WHEN** a buyer sees the "Gợi ý cho bạn" row and opens its third card
- **THEN** the tracking events sent for that row carry `placement_id=home.for_you`, the `request_id` and
  `model_version` of the `Recommend` response that filled it, and `position=3` on the click

### Requirement: Recommend request carries caller identity, an optional seed, context, and a limit

`RecommendRequest` SHALL let a caller ask for recommendations as either an authenticated user
(`user_id`) or an anonymous visitor (`anonymous_id`), SHALL allow an optional `seed_listing_id` to
anchor item-to-item contexts, SHALL carry a `RecommendationContext` enum and an optional `placement_id`
(`<surface>.<slot>`) identifying where the recommendation is shown, and SHALL accept a `limit`
(0 = server default). When `placement_id` is empty the server SHALL derive it from `context`
(`HOMEPAGE` → `home.for_you`, `SIMILAR_ITEMS` → `pdp.similar`), so callers that predate the field keep
working; an unknown `placement_id` SHALL be rejected with `INVALID_ARGUMENT`.

#### Scenario: An anonymous PDP "similar items" request is expressible

- **WHEN** a caller builds a `RecommendRequest` with an empty `user_id`, an `anonymous_id`, a
  `seed_listing_id` set to the viewed listing, `context = RECOMMENDATION_CONTEXT_SIMILAR_ITEMS`, and
  `limit = 12`
- **THEN** the message is valid under the contract, so the serving layer has every field it needs to
  choose an item-item strategy for an anonymous visitor without a schema change

#### Scenario: A request without a placement id is served under the derived placement

- **WHEN** a caller sends `context = RECOMMENDATION_CONTEXT_HOMEPAGE` and no `placement_id`
- **THEN** the response carries `placement_id = home.for_you`

#### Scenario: An unknown placement is rejected

- **WHEN** a caller sends `placement_id = home.unknown`
- **THEN** the call fails with `INVALID_ARGUMENT` naming the placement

### Requirement: Recommend response returns ranked listing ids with scores and a model version

`RecommendResponse` SHALL return a `repeated RecommendedItem`, each carrying a `listing_id`, a
`score` (higher = more relevant), and a 1-based `rank`, ordered best-first, plus a `model_version`
identifying the model generation that produced the ranking, a `request_id` unique to this response, and
the `placement_id` it was served for. The proto fields are additive (`buf breaking` passes) and are added by the
tracking event platform change; this capability only fills them. The
response SHALL carry **listing ids, scores and this provenance only** (not hydrated listing cards and no
personal data), so the recommendation side never owns listing content (Rule 3).

#### Scenario: A ranked result set is expressible with provenance

- **WHEN** the serving layer fills a `RecommendResponse` with ordered `RecommendedItem`s and the
  `model_version` of the batch artifact it read
- **THEN** the consumer can render the ranking in order using each `listing_id`/`rank`, sort/threshold
  on `score`, and trace which offline model produced the result via `model_version`

#### Scenario: Every response is attributable

- **WHEN** the same buyer calls `Recommend` twice for placement `home.for_you`
- **THEN** both responses carry `placement_id = home.for_you` and the served `model_version`, and their
  `request_id`s are non-empty and different

### Requirement: Two-stage retrieval then ranking with business-rule filtering

The system SHALL produce recommendations for a placement by running that placement's configured strategy
chain in order until up to `RECS_CANDIDATE_TOP_K` (default 100) candidates are collected, then filtering —
de-duplicating, excluding the request's `seed_listing_id`, and keeping only candidates that the search
read-model reports as published and in stock (the eligibility check) — then ranking through the placement's
configured ranker, truncated to the requested limit or `RECS_RESULT_TOP_K` (default 10). The eligibility
check SHALL be applied to every strategy's candidates, SHALL preserve candidate order among kept items,
and SHALL be a single bounded call per request.

#### Scenario: Out-of-stock and seed items are filtered from candidates

- **WHEN** the recommend module retrieves candidates that include an out-of-stock item, a duplicate, and
  the request's own `seed_listing_id`
- **THEN** those items are removed and the response contains at most ten distinct in-stock products, none
  of them the seed listing, in the model's order

#### Scenario: Filtering refills from deeper candidates

- **WHEN** three of the first ten candidates are not eligible and the candidate list holds more than ten
  eligible items
- **THEN** the response still contains ten items, taken in order from the eligible candidates

### Requirement: Redis pre-computed cache fast path with Qdrant fallback

The system SHALL, for a logged-in `user_id` on placement `home.for_you`, first read the served generation's
pre-computed Top-N list for that user from Redis — the generation named by the producer's
`{RECS_CACHE_PREFIX}:v2:current` pointer (or by the challenger pointer for users in the test bucket) — and
serve it after filtering. On a cache miss, an expired key, a missing pointer, or a Redis error, the system
SHALL continue with the placement's next strategy and SHALL NOT fail the RPC on a cache error. Pointers
SHALL be read through an in-process cache of at most `RECS_MODEL_VERSION_CACHE_SECONDS`, so the hot path
makes at most one cache round trip.

#### Scenario: Cache hit serves without a Qdrant query

- **WHEN** `Recommend` is called for a user whose list is present in the current generation
- **THEN** the response is built from that list, no Qdrant ANN query is issued, and the response carries
  the current generation's model version

#### Scenario: Cache miss falls back to Qdrant retrieval

- **WHEN** `Recommend` is called with a seed for a user with no list in the current generation (or Redis
  is unreachable)
- **THEN** the module retrieves candidates from the current generation's item collection and still
  returns up to ten eligible items

### Requirement: Cold-start and anonymous requests still return a non-empty row

The system SHALL serve anonymous requests and cold users (no list in the served generation) from the
placement's chain: items similar to the visitor's recent items (`user.recent_items_24h` from the online
feature store), then items similar to or co-visited with the seed when present, then trending items
(ranked by `item.trending_1h`), then the generation's popular list, and, when none of these yields an
eligible item, a catalog floor of the newest published in-stock listings from the search read-model, so a
`Recommend` call returns a non-empty product list whenever the catalog holds an eligible listing. An empty
response SHALL be returned only when no eligible listing exists or eligibility cannot be checked.

#### Scenario: Anonymous request seeded from a listing returns similar items

- **WHEN** `Recommend` is called with an empty `user_id` and a `seed_listing_id`
- **THEN** the module returns items similar to the seed listing from the current generation, filtered to
  eligible items and excluding the seed

#### Scenario: No user, no seed falls back to popular items

- **WHEN** `Recommend` is called with no `user_id`, no `seed_listing_id`, and no online features for the
  visitor
- **THEN** the module returns trending or popular eligible items rather than an empty response

#### Scenario: A cold-start user gets items related to what they just viewed

- **WHEN** a logged-in user who is not part of the current generation viewed two listings in the last hour
  and calls `Recommend` for `home.for_you`
- **THEN** the response succeeds and contains listings similar to those two, ahead of the popular list

#### Scenario: With no model at all the catalog floor is served

- **WHEN** no generation has ever been published, no online features exist, and the catalog holds
  published in-stock listings
- **THEN** `Recommend` succeeds with the newest eligible listings and the configured fallback model
  version

### Requirement: Recommend stays within the latency budget

The system SHALL answer `Recommend` within a serve-path budget of under 15ms (p99, excluding network to the
caller) for candidate retrieval and ranking, plus at most one eligibility call bounded by
`RECS_ELIGIBILITY_TIMEOUT_MS` (default 50ms), at most one online-feature call bounded by
`RECS_FEATURES_TIMEOUT_MS` (default 30ms), and at most one catalog-floor call, making at most one cache
round trip on the hot path. It SHALL cap the Qdrant path with `RECS_RETRIEVE_TIMEOUT_MS` and continue with
the next strategy on timeout, and SHALL cap the eligibility call with its timeout, returning an empty result
(never an unchecked one) when it expires.

#### Scenario: Retrieval timeout yields a fallback, not an error

- **WHEN** the Qdrant retrieval exceeds `RECS_RETRIEVE_TIMEOUT_MS`
- **THEN** `Recommend` returns the eligible part of the next strategy's list instead of failing or
  blocking past the budget

#### Scenario: Eligibility timeout yields an empty result, not an error

- **WHEN** the eligibility call exceeds `RECS_ELIGIBILITY_TIMEOUT_MS` or the search read-model is
  unreachable
- **THEN** `Recommend` succeeds with no items within the budget, and the outcome is counted as an
  eligibility error

### Requirement: An offline batch job trains ALS from the behavioral warehouse

The platform SHALL provide a `platform-recsys` PySpark batch job that reads only the dataset
build of its declared feature view `als_interactions@v1` that it requested from the feature store for this run (see
`recsys-feature-consumption`), uses `(user, item, user_item.implicit_score_decayed)` rows as
implicit-feedback triples, and fits a Spark MLlib ALS model with `implicitPrefs=true`. The job SHALL never
read raw behavioural events or a service database (Rule 3), SHALL write only its own artifact stores, and
SHALL run as a scheduled offline batch — not on the request path.

#### Scenario: The job trains a model from sample warehouse data

- **WHEN** the job runs against a dataset build of `als_interactions@v1` containing weighted interactions
  for several users and listings
- **THEN** it produces an ALS model with item factors and user factors, one weighted interaction per
  (user, item) row

### Requirement: The job publishes item and user vectors to Qdrant

The job SHALL load the ALS **item factors** and **user factors** into Qdrant as a collection pair owned by
one generation (`{collection}__{model_version}`), each point payload carrying the source id and the
`model_version`, so the online serving layer can nearest-neighbor query the generation it serves.

#### Scenario: A training run populates the Qdrant collections

- **WHEN** the job finishes training and loads its outputs
- **THEN** the item-vector and user-vector collections of that generation exist and hold one point per
  trained listing / user, each stamped with the run's `model_version`

### Requirement: The job writes a precomputed recommendation cache to Redis

The job SHALL write a precomputed top-N recommendation cache into Redis under generation-scoped keys:
per-user ranked recommendations (logged-in users only), per-listing similar items and the popular list of
that generation, plus generation metadata, and SHALL then flip the `current` (or `challenger`) pointer to
the generation as the last write (see `recsys-model-lifecycle`). Generation keys SHALL carry a TTL longer
than twice the batch cadence. The cache SHALL hold listing ids and scores only (Rule 3).

#### Scenario: A training run populates the Redis cache

- **WHEN** the job finishes loading its outputs and the gate passed
- **THEN** Redis holds, for the new generation, a per-user recommendation entry, a per-listing
  similar-items entry and a popular list (ranked listing ids + scores), and the `current` pointer names
  that generation

### Requirement: The job runs locally without a Spark cluster

The job SHALL run in Spark local mode (no cluster) reading a feature-view dataset build from the local
object storage and writing to the local Qdrant and Redis of the compose stack, so the full train → publish
loop is runnable on a developer laptop and in CI on a small dataset.

#### Scenario: A developer runs the job end-to-end locally

- **WHEN** a developer runs the job in local mode against a small `als_interactions@v1` dataset build with
  local Qdrant and Redis running
- **THEN** the job completes without a Spark cluster and leaves a published generation in Qdrant and
  Redis

## ADDED Requirements

### Requirement: Opted-out visitors are served non-personalized recommendations

When the consent state forwarded by the gateway for a request is `denied`, team-ai SHALL serve the request as
an anonymous caller without an anonymous id: it SHALL NOT use the caller's personal list, SHALL NOT request
any `user` feature for the caller, and SHALL serve the current generation (no challenger arm), while seed-based,
trending, popular and floor strategies still run so the row is not empty. A request without a forwarded consent
state SHALL be treated as granted. Caller binding is unchanged.

#### Scenario: An opted-out buyer gets no personal list

- **WHEN** a logged-in buyer who has a personal list in the current generation sends `Recommend` for
  `home.for_you` with the forwarded consent state `denied`
- **THEN** the response is non-empty, contains no item taken from the buyer's personal list, carries the
  current generation's `model_version`, and no feature read for the buyer's `user` entity is made


### Requirement: Placements are configured with per-placement strategies

team-ai SHALL serve exactly the placements declared in its placement configuration — `home.for_you`,
`pdp.similar` and `home.trending` — each with an ordered strategy chain and a ranker. Adding, removing or
reordering a strategy for a placement SHALL be a configuration change only. An invalid configuration
(unknown strategy, unknown ranker, unpinned feature version) SHALL fail at startup, never at request time.
The only ranker in this change SHALL be the identity ranker, and a ranker SHALL receive the candidates and
access to online features so a learned ranker can be added by configuration.

#### Scenario: Each placement serves its own strategy

- **WHEN** a promoted generation exists and a buyer calls `Recommend` for `home.for_you`, then for
  `pdp.similar` with a seed, then for `home.trending`
- **THEN** the three responses are non-empty, carry their own `placement_id`, and the PDP response
  contains listings similar to the seed

#### Scenario: A bad placement configuration stops startup

- **WHEN** team-ai starts with a placement that names an unknown strategy
- **THEN** startup fails with an error naming the placement and the strategy

### Requirement: Serving never errors because the model or features are missing

The system SHALL NOT fail `Recommend` because a model, an online feature or a store is absent: a missing
item collection, a missing pointer, a missing popular list, an unavailable or slow feature service, or a
Redis or Qdrant outage SHALL each make that strategy yield nothing and the chain continue. The item
collection's presence and contract SHALL be re-checked periodically (`RECS_COLLECTION_RECHECK_SECONDS`),
so a collection created after boot is used without a restart. Only a present collection whose dimension
or distance contradicts the configuration SHALL make `Recommend` answer `UNAVAILABLE`, and that state SHALL
also be re-checked so a corrected producer recovers without a restart.

#### Scenario: Boot before the first training run

- **WHEN** team-ai starts with `RECS_BACKEND=qdrant` before any training run has created the collection
- **THEN** `Recommend` succeeds (trending, popular or catalog floor), and after the first promoted run a
  seeded request returns similar items without restarting team-ai

#### Scenario: Qdrant outage degrades to the next strategy

- **WHEN** Qdrant is unreachable and a seeded request arrives
- **THEN** `Recommend` succeeds with eligible items from a later strategy and the outcome is counted as a
  fallback

#### Scenario: Feature service outage degrades to the next strategy

- **WHEN** the feature service is unreachable and a cold user calls `Recommend` for `home.for_you`
- **THEN** `Recommend` succeeds with eligible popular or floor items and the outcome is counted as a
  `feature_failure`

### Requirement: Recommendations never include unpublished, deleted or unavailable listings

The system SHALL return only listings that the search read-model reports as published with stock greater
than zero at request time. A draft, rejected, deleted or sold-out listing SHALL never appear in a
`Recommend` response, for any caller and any placement, including when it is part of a precomputed list,
a similar-items result, a feature-derived list or the popular list. When eligibility cannot be determined,
the system SHALL return no items rather than unchecked items.

#### Scenario: A deleted listing in a precomputed list is not served

- **WHEN** a listing in a user's current-generation list is deleted by its seller after the run
- **THEN** that user's next `Recommend` response does not contain it

#### Scenario: A listing moved back to draft is not served

- **WHEN** a listing in the popular list is unpublished (status no longer published) after the run
- **THEN** no `Recommend` response contains it, including for its owner

#### Scenario: A sold-out listing is not served

- **WHEN** a listing in a user's list is drained to stock 0
- **THEN** that user's next `Recommend` response does not contain it, and it reappears after a restock
  makes it in stock again

### Requirement: Recommendations are bound to the caller

The system SHALL serve a `user` principal only its own recommendations, ignoring any `user_id` or
`anonymous_id` in the request; an anonymous principal SHALL never be served a user's list; only `admin` or
service principals MAY request on behalf of a user. The response SHALL carry listing ids, scores, ranks
and provenance only, never another user's identifier or any personal data.

#### Scenario: A buyer cannot read another buyer's list

- **WHEN** buyer A calls `Recommend` with `user_id` set to buyer B's id
- **THEN** the response is A's own recommendations (or A's cold-start result), never B's list

#### Scenario: An anonymous caller cannot claim a user

- **WHEN** an anonymous caller sends a `user_id`
- **THEN** the `user_id` is ignored and the response is the anonymous result

### Requirement: Logged-in users receive personalized lists from the published model

When a promoted generation exists, a logged-in user who is part of it SHALL receive that generation's
personal list (after filtering) on `home.for_you`, so two users whose behaviour differs receive different
recommendations, and each list SHALL favour listings related to that user's own behaviour.

#### Scenario: Two buyers with different behaviour get different lists

- **WHEN** buyer A browses and adds to cart only listings of category X and buyer B only listings of
  category Y, among enough other generated traffic of the same two tastes, and the pipeline runs
- **THEN** A's and B's `Recommend` responses carry the same new model version, their item sets differ,
  and A's list contains more category-X listings than category-Y listings while B's contains the reverse

### Requirement: Requests are assigned to a model arm by user hash

When a challenger generation is set with a share greater than zero, team-ai SHALL serve the challenger
generation to callers whose bucket (a hash of the challenger version and the caller's user key, modulo
100) is below the share, and the current generation to everyone else; the assignment SHALL be stable for a
caller during an experiment, and the response's `model_version` SHALL name the generation actually served.
A caller with neither a user id nor an anonymous id SHALL be served the current generation. With no
challenger or share 0, every caller SHALL be served the current generation.

#### Scenario: A user stays in the same arm

- **WHEN** a challenger is set with share 50 and the same buyer calls `Recommend` five times
- **THEN** all five responses carry the same `model_version`

#### Scenario: Both arms are served

- **WHEN** a challenger is set with share 50 and twenty different buyers call `Recommend`
- **THEN** some responses carry the challenger's `model_version` and others the current one's

#### Scenario: Stopping the experiment returns everyone to control

- **WHEN** the operator stops the challenger
- **THEN** within the version-cache interval every `Recommend` response carries the current generation's
  `model_version`

### Requirement: Serving exposes model and outcome metrics with staleness alerts

team-ai SHALL expose Prometheus metrics for: the served model versions and arms (as an info metric), the
current generation's publish time, the last run's status and finish time, `Recommend` outcomes by placement,
serving source (`cache`, `ann`, `recent`, `covisit`, `trending`, `popular`, `floor`, `empty`) and arm, cache
hits and misses, eligibility drops and errors, feature failures (`feature_failure`) and opted-out requests. The platform SHALL ship alert rules that
fire when the model age exceeds the configured staleness bound, when the last run is not successful for
longer than one cadence, when the fallback share exceeds a threshold, and when eligibility errors persist.

#### Scenario: Metrics follow a promotion

- **WHEN** a run promotes a new generation and a few `Recommend` calls are made
- **THEN** team-ai's metrics endpoint reports the new model version, a model age smaller than one cadence,
  and source counters that increased for the stages that served

#### Scenario: A stale model raises an alert

- **WHEN** no generation has been promoted for longer than the staleness bound
- **THEN** the model-staleness alert is in firing state in Prometheus

### Requirement: The memory backend is never used in a deployed environment

team-ai SHALL refuse to start with `RECS_ENABLED=true` and `RECS_BACKEND=memory` when its `ENVIRONMENT`
is not a local one, and every deployed environment SHALL set `RECS_BACKEND=qdrant` explicitly, so deployed
environments never serve fixture data.

#### Scenario: Non-local environment rejects the memory backend

- **WHEN** team-ai starts with `ENVIRONMENT=staging`, `RECS_ENABLED=true` and `RECS_BACKEND=memory`
- **THEN** startup fails with an error naming `RECS_BACKEND`
