# recommendations Specification

## Purpose

Recommendations specification covering serving, offline evaluation, and model promotion.

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

### Requirement: The training run SHALL evaluate the generation it produced

Every pipeline run SHALL score its training recipe against a temporal holdout drawn from the
same interaction window, using the existing `ModelEvaluator`. It SHALL record the resulting
metrics against that run's `model_version`, stamped with the evaluation protocol. The holdout
SHALL NOT leak into the scored model:
- for each user with at least two distinct listings, the target is the most recently
  discovered listing;
- the evaluation model is trained only on that user's events before the discovery;
- the scored ranking excludes the user's training items.

The published model MAY be trained on every event. A run that cannot produce metrics SHALL NOT
be treated as a promotable candidate. The promotion gate SHALL NOT compare metrics produced
under different evaluation protocols.

#### Scenario: A run produces ranking metrics for the generation it trained

- **WHEN** the pipeline completes ALS training over a warehouse with enough interactions to form
  a holdout
- **THEN** the run reports `ndcg@10` and `coverage@10` for that generation
- **AND** the reported metrics are attributed to the run's own `model_version`, not to a
  previous run's

#### Scenario: A run with no usable holdout is not a candidate

- **WHEN** the interaction window yields no test events after the temporal split
- **THEN** the run records that no evaluation was possible
- **AND** no candidate is registered, so the promotion gate is not consulted

#### Scenario: The evaluation model never trains on its targets

- **WHEN** a run evaluates its training recipe
- **THEN** no held-out (user, listing) pair, and none of that user's later events, is in the
  evaluation model's training data
- **AND** the metrics carry the evaluation protocol identifier

#### Scenario: Metrics from another evaluation protocol are not compared

- **WHEN** the incumbent champion's metrics were produced under a different evaluation protocol
- **THEN** the gate does not compare the two values, and the candidate becomes the champion with
  a reason that names both protocols

### Requirement: A candidate SHALL reach serving only after passing the promotion gate

The pipeline SHALL register the evaluated run as a `ModelMetadata` candidate and SHALL call the
promotion gate before any serving artifact is published. Vectors and cache entries for a rejected
candidate SHALL NOT replace the generation currently being served, and the previous generation
SHALL remain readable.

#### Scenario: A regressing candidate does not become champion

- **WHEN** a run evaluates below the incumbent champion's `ndcg@10` by more than the configured
  tolerance
- **THEN** the candidate is recorded with status `rejected`
- **AND** `recs:model:champion` still names the previous version

#### Scenario: A rejected candidate leaves the previous generation serving

- **WHEN** a candidate is rejected by the gate
- **THEN** the Qdrant points and Redis keys of the previous generation are unchanged
- **AND** `recs:v1:model_version` still names the previous generation
- **AND** no stale-generation prune has been performed

#### Scenario: The first run bootstraps the registry

- **WHEN** a run completes and no champion is registered yet
- **THEN** that run is promoted and becomes the champion
- **AND** its artifacts are published to Qdrant and Redis

### Requirement: The promotion decision SHALL be observable from the run itself

The pipeline summary SHALL report the metrics that were computed, the comparison that was made,
and the resulting decision, so that a scheduled run can be audited from its own output without
inspecting the registry.

#### Scenario: The run summary states the decision and its reason

- **WHEN** a pipeline run finishes, whether the candidate was promoted or rejected
- **THEN** the summary names the candidate `model_version`, the metric compared, the incumbent
  value, the candidate value, and the decision
- **AND** a rejected run exits without signalling failure, because rejection is a normal outcome

### Requirement: Offline evaluation owns its train/test split

The offline evaluation entrypoint SHALL derive the train and holdout sets from raw interactions
using a time-based split, rather than accepting a caller-prepared ground truth as its only input.
The report SHALL record the cutoff timestamp, the train and test event counts, and the split
strategy used.

#### Scenario: Evaluation report exposes the split it performed

- **WHEN** the evaluation entrypoint runs over a raw interaction fixture
- **THEN** the report contains `cutoff_timestamp`, `train_events`, `test_events` and
  `split_strategy`
- **AND** the metrics are computed against the holdout produced by that split
- **VERIFIED BY**: platform-recsys/tests/test_evals_temporal_cli.py › test_evaluator_owns_temporal_split_and_reports_metadata. Not verifiable end to end: offline evaluation logic over a fixture with no edge-visible effect; the pipeline's use of it is covered by the archived wire-pipeline-eval-registry e2e.

#### Scenario: Split admits no temporal leakage

- **WHEN** the temporal split is applied to an interaction fixture
- **THEN** every training event occurs at or before the cutoff
- **AND** every holdout event occurs after the cutoff
- **VERIFIED BY**: platform-recsys/tests/test_evals_temporal_cli.py › test_temporal_split_zero_leakage. Not verifiable end to end: offline evaluation logic over a fixture with no edge-visible effect; the pipeline's use of it is covered by the archived wire-pipeline-eval-registry e2e.

#### Scenario: Post-cutoff-only signal is not learnable from the training half

- **WHEN** a fixture places a user's entire affinity for one item after the cutoff, and a model is
  trained on the training half only
- **THEN** that item's recall on the holdout is near zero
- **VERIFIED BY**: platform-recsys/tests/test_evals_temporal_cli.py › test_post_cutoff_only_signal_yields_zero_recall_on_training_model. Not verifiable end to end: offline evaluation logic over a fixture with no edge-visible effect; the pipeline's use of it is covered by the archived wire-pipeline-eval-registry e2e.

### Requirement: Externally supplied ground truth is labelled

When the caller supplies a pre-split ground truth, the report SHALL label the strategy as
external so reports produced under different splits are not silently compared.

#### Scenario: External split is marked in the report

- **WHEN** evaluation runs against caller-supplied ground-truth sets
- **THEN** `split_strategy` in the report is `"external"`
- **VERIFIED BY**: platform-recsys/tests/test_evals_temporal_cli.py › test_external_ground_truth_is_marked_external. Not verifiable end to end: offline evaluation logic over a fixture with no edge-visible effect; the pipeline's use of it is covered by the archived wire-pipeline-eval-registry e2e.

### Requirement: Nearline real-time session signals

The system SHALL maintain real-time user session signals and recent interactions in Redis updated within seconds of tracking event receipt.

#### Scenario: User recent views update nearline signals

- **WHEN** a user views item `item-A` and then `item-B`
- **THEN** the nearline signal layer records `[item-B, item-A]` in the user's recent items list in Redis and increments the respective category affinities

#### Scenario: Real-time item co-occurrence is tracked

- **WHEN** multiple users view `item-A` and `item-B` within the same session
- **THEN** the co-view count between `item-A` and `item-B` is incremented in Redis

### Requirement: Serving ranks with the nearline signals

The recommendation serving path SHALL read the position-debiased CTR from the nearline Redis keys (`recs:nearline:ctr:<listing_id>`,
see design.md) for the candidates of a GBDT-ranked request, and SHALL rank with it. When the nearline store is unreachable, slow, or has
no usable data for a candidate, serving SHALL continue on the prior CTR without failing or degrading the request.

#### Scenario: Serving ranks with the nearline CTR written by the consumer

- **WHEN** two home-feed candidates have equal model scores and only the second has a nearline CTR in Redis
- **THEN** a `Recommend` call through the gateway ranks the second above the first
- **AND** with no nearline row for either, the order is the candidates' own order

#### Scenario: Nearline outage leaves the request served

- **WHEN** the nearline Redis cannot be read, or has only rows below the impression floor
- **THEN** the request is answered with the prior ordering and a non-degraded status
- **VERIFIED BY**: team-ai/tests/unit/modules/recommend/test_factory_serving_wiring.py › test_nearline_outage_does_not_fail_or_degrade_the_request, test_nearline_below_the_impression_floor_is_not_used; team-ai/tests/unit/modules/recommend/test_redis_nearline_store.py. Not verifiable end to end: it needs the stack's Redis (also the serving cache) to fail and `status` is not on the gateway wire.

### Requirement: Nearline signals are consumed from analytics.events

A process (`python -m recsys.nearline`) SHALL consume the Kafka topic `analytics.events` as its own consumer group and
apply each tracking event to the Redis keys of the nearline layer. Views, clicks and add-to-carts feed the actor's
recents, category affinity and the session's co-views; impressions and clicks feed the position-debiased click-through
rate. The actor SHALL be the warehouse `user_key` (the principal's id for a signed-in user, else `anon:<anonymous_id>`).
It SHALL commit an offset only after the events it covers were written to Redis, so a Redis failure replays them. An
event delivered twice within 15 minutes, or older than the 24 hour window, SHALL change nothing. A message that is not
a tracking event or cannot be decoded SHALL be skipped and counted, not stop the consumer.

#### Scenario: Redelivered and stale events change nothing

- **WHEN** the same event id is delivered twice, and an event older than the window is delivered
- **THEN** the recents, category affinities, co-view counts and click-through counters are as after one delivery of the
  first and none of the second
- **VERIFIED BY**: platform-recsys/tests/test_nearline.py › test_a_redelivered_event_is_applied_once and test_events_older_than_the_window_are_ignored. Not verifiable end to end: the gateway stamps every event's id and time, so no public-edge call can redeliver an envelope or produce an old one.

#### Scenario: An undecodable message is skipped

- **WHEN** a message on `analytics.events` is not a protobuf message, and a valid tracking event follows it
- **THEN** the consumer counts the first as undecodable and applies the second
- **VERIFIED BY**: platform-recsys/tests/test_nearline_consumer.py › test_other_envelopes_and_garbage_are_skipped_not_fatal. Not verifiable end to end: the gateway only produces well-formed envelopes, and e2e has no way to write raw bytes to the topic through the public edge.

### Requirement: Nearline keys live outside the generation namespace

Nearline keys (`recs:nearline:*`) SHALL NOT be scoped to a model generation and SHALL NOT be written, moved or deleted
by publishing a generation, by retention or by rollback; they expire by their own TTL (24 hours). Their layout is the
contract team-ai reads (see `design.md`).

#### Scenario: A generation switch leaves nearline keys alone

- **WHEN** nearline keys exist and the recsys job promotes three generations in a row
- **THEN** the nearline keys are unchanged and keep their TTL

### Requirement: Ranker features use position-debiased CTR

Candidate feature extraction SHALL source `historical_ctr` from the nearline position-debiased
CTR when a value is available for that item, so that ranking is not driven by raw click-through
rates inflated by favourable display positions.

#### Scenario: Equal raw CTR, worse positions, higher debiased CTR

- **WHEN** two candidates have accumulated identical raw click-through rates, but one item's
  impressions occurred at consistently worse positions
- **THEN** the item shown at worse positions receives the higher `historical_ctr` feature value
- **VERIFIED BY**: team-ai/tests/unit/modules/recommend/test_debiased_ctr_ranking.py › test_equal_raw_ctr_worse_position_yields_higher_debiased_ctr_and_gbdt_score, test_extract_features_records_source_and_keeps_the_prior_value_on_fallback. Not verifiable end to end: the feature vector and `explain` are not on the gateway wire (the ordering effect is the e2e scenario `Debiased value changes the ranking score`). The accumulation of positions happens in the platform-recsys consumer, outside team-ai.

#### Scenario: Debiased value changes the ranking score

- **WHEN** the candidates above are scored by the GBDT ranker
- **THEN** the item with the higher debiased CTR receives the higher ranking score

### Requirement: CTR source is recorded

The feature vector SHALL record which source produced `historical_ctr`, so that training-time and
serving-time feature provenance can be compared.

#### Scenario: Nearline data present

- **WHEN** the nearline store holds a debiased CTR for the candidate
- **THEN** the feature vector reports `ctr_source` as `"nearline"`
- **VERIFIED BY**: team-ai/tests/unit/modules/recommend/test_debiased_ctr_ranking.py › test_extract_features_records_source_and_keeps_the_prior_value_on_fallback, test_ranked_items_carry_the_ctr_source_of_their_feature_vector; team-ai/tests/unit/modules/recommend/test_factory_serving_wiring.py › test_explain_reports_the_ctr_source_of_every_returned_item. Not verifiable end to end: the feature vector and `explain` are not on the gateway wire (the ordering effect is the e2e scenario `Debiased value changes the ranking score`).

#### Scenario: Nearline data absent

- **WHEN** the nearline store holds no usable data for the candidate
- **THEN** `historical_ctr` retains its prior value
- **AND** the feature vector reports `ctr_source` as `"fallback"`
- **VERIFIED BY**: team-ai/tests/unit/modules/recommend/test_debiased_ctr_ranking.py › test_extract_features_records_source_and_keeps_the_prior_value_on_fallback; team-ai/tests/unit/modules/recommend/test_factory_serving_wiring.py › test_without_nearline_every_item_reports_fallback. Not verifiable end to end: the feature vector and `explain` are not on the gateway wire (the ordering effect is the e2e scenario `Debiased value changes the ranking score`).

### Requirement: Serving path supplies the nearline source

The recommendation serving path SHALL provide the nearline signal source during candidate
enrichment, so that debiased CTR reaches the ranker at request time and not only in offline
training.

#### Scenario: Serving request enriches from nearline

- **WHEN** a recommendation request runs for a placement whose ranking model is `gbdt`
- **THEN** candidate features were built with the nearline source
- **AND** the response `explain` payload reports the nearline enrichment (`nearline_enabled`, `nearline_hit_count`, and `ctr_sources`, the count of returned items per CTR source)
- **VERIFIED BY**: team-ai/tests/unit/modules/recommend/test_factory_serving_wiring.py › test_factory_built_service_consults_the_nearline_store, test_explain_reports_the_ctr_source_of_every_returned_item (service built by `build_recommendation_service`). Not verifiable end to end: the feature vector and `explain` are not on the gateway wire (the ordering effect is the e2e scenario `Debiased value changes the ranking score`).

### Requirement: Declared ranking model governs the serving path

The recommendation service SHALL rank candidates with the model named by the active placement's
`ranking.model`. When the value is `gbdt`, ordering SHALL be produced by the GBDT ranker rather
than by retrieval-score sorting, and the response `explain` payload SHALL report the model that
actually ran.

#### Scenario: GBDT placement produces ranker ordering, not cosine ordering

- **WHEN** `service.recommend` is called for a placement whose `ranking.model` is `gbdt`, against
  a candidate fixture where the GBDT weights rank two candidates opposite to their cosine order
- **THEN** the returned item order matches the GBDT ordering and differs from the cosine ordering
- **AND** `explain["ranking_model"]` equals `"gbdt"`
- **VERIFIED BY**: team-ai/tests/unit/modules/recommend/test_placement_engine.py › test_home_feed_personalized_gbdt_and_featurestore_hit_count; team-ai/tests/unit/modules/recommend/test_factory_serving_wiring.py › test_factory_built_service_reads_the_feature_store_and_ranks_with_gbdt. Not verifiable end to end: the response `explain` payload and `status` are not on the gateway wire (`RecommendResponse` carries items, model_version, placement_id, request_id).

#### Scenario: Placement without a GBDT model keeps retrieval ordering

- **WHEN** `service.recommend` is called for a placement whose `ranking.model` is not `gbdt`
- **THEN** the returned order is the retrieval-score order
- **AND** `explain["ranking_model"]` reports that model
- **VERIFIED BY**: team-ai/tests/unit/modules/recommend/test_placement_engine.py › test_similar_items_placement_keeps_cosine_order; team-ai/tests/unit/modules/recommend/test_factory_serving_wiring.py › test_non_gbdt_placement_keeps_retrieval_order_through_the_factory. Not verifiable end to end: the response `explain` payload and `status` are not on the gateway wire (`RecommendResponse` carries items, model_version, placement_id, request_id).

### Requirement: Feature-store enrichment is observable per request

When the active placement declares `use_featurestore: true`, the service SHALL enrich candidates
with item features before ranking and SHALL report how many candidates were enriched.

#### Scenario: Enrichment count proves the feature store was read

- **WHEN** `service.recommend` runs for a placement with `use_featurestore: true` and the feature
  store holds features for at least one returned candidate
- **THEN** `explain["featurestore_hit_count"]` is greater than zero
- **VERIFIED BY**: team-ai/tests/unit/modules/recommend/test_factory_serving_wiring.py › test_factory_built_service_reads_the_feature_store_and_ranks_with_gbdt (RedisFeatureStore built from RECS_FEATURESTORE_REDIS_URL). Not verifiable end to end: the response `explain` payload and `status` are not on the gateway wire (`RecommendResponse` carries items, model_version, placement_id, request_id). The feature store's effect on order is covered end to end by `recommendations/serving_safeguards.feature` (Online features break a tie).

### Requirement: Placements cannot declare unbound capabilities

The application SHALL reject, at startup, any placement declaring a ranking model or capability
that has no bound implementation. A declared capability that no code reads SHALL NOT be loadable.

#### Scenario: Unbound ranking model fails at startup

- **WHEN** the placement registry loads a placement whose `ranking.model` names a model with no
  registered implementation
- **THEN** application startup fails with an error naming the placement and the missing binding
- **AND** no request is served with the unbound configuration
- **VERIFIED BY**: team-ai/tests/unit/modules/recommend/test_placement_engine.py › test_startup_validation_rejects_unbound_ranking_model. Not verifiable end to end: it needs a placement file with an unbound model and a service restart, and a refused start serves nothing to probe.

### Requirement: Ranking failure degrades rather than errors

Ranker or feature-store failure SHALL NOT fail the request. The service SHALL fall back to
retrieval-score ordering and mark the response degraded.

#### Scenario: Ranker failure falls back and marks degraded

- **WHEN** the ranker raises during `service.recommend`
- **THEN** items are returned in retrieval-score order
- **AND** the response `status` is `"degraded"`
- **VERIFIED BY**: team-ai/tests/unit/modules/recommend/test_placement_engine.py › test_ranker_failure_degrades_to_cosine_order. Not verifiable end to end: it needs the ranker to raise inside the running service, and `status` is not on the gateway wire.

### Requirement: The factory binds what the placements declare

`build_recommendation_service` SHALL construct the service with the feature store implied by `RECS_FEATURESTORE_REDIS_URL`
and the ranker that `ranking.model` selects, so a declared `use_featurestore` or `gbdt` reaches behaviour in the built service
and not only in a service assembled by hand.

#### Scenario: A factory-built service honours use_featurestore and gbdt

- **WHEN** a service is built with `build_recommendation_service` from settings that set `RECS_FEATURESTORE_REDIS_URL`, and a `home_feed` request runs against features that invert the cosine order
- **THEN** `explain["featurestore_hit_count"]` is greater than zero and the order differs from the cosine order
- **AND** with the setting empty, `featurestore_hit_count` is `0` and the order is the cosine order
- **VERIFIED BY**: team-ai/tests/unit/modules/recommend/test_factory_serving_wiring.py › test_factory_built_service_reads_the_feature_store_and_ranks_with_gbdt, test_without_a_feature_store_url_the_same_request_keeps_cosine_order. Not verifiable end to end: the response `explain` payload and `status` are not on the gateway wire (`RecommendResponse` carries items, model_version, placement_id, request_id).

### Requirement: Serving reads the feature store's registry features

The ranker SHALL read item features by the names the feature registry declares for `item_popularity`, from the version named by
`fs:item_popularity:current`. A declared feature that is missing or not a finite number SHALL take its documented default, be
counted in `explain["feature_defaults"]`, and SHALL NOT fail the request.

#### Scenario: Registry feature names drive the ranking and defaults are counted

- **WHEN** a request ranks candidates whose online rows carry the registry names, and another whose row carries only names the registry does not declare
- **THEN** the first rows' `ctr_7d` and engagement counts reach the ranker's inputs, the other row ranks as an empty row, and `explain["feature_defaults"]` counts its missing registry features
- **AND** the request is not degraded
- **VERIFIED BY**: team-ai/tests/unit/modules/recommend/test_item_feature_contract.py › test_registry_names_drive_the_ranker_vector, test_service_counts_defaulted_features_and_does_not_crash, test_item_feature_list_matches_the_registry_view. Not verifiable end to end: `explain` is not on the gateway wire (the feature store's effect on order is covered by `recommendations/serving_safeguards.feature`).

### Requirement: Batch pipeline produces two-tower vectors

The offline recommendation pipeline SHALL, when the two-tower stage is enabled, train the
two-tower model and load its item vectors into a dedicated vector-store collection named for the run's generation
(`<QDRANT_TWO_TOWER_COLLECTION>__<model_version>`) as part of the same run that produces ALS factors. The collection SHALL
be written before the serving pointer moves, so a generation is complete or not visible. The run summary SHALL report the
number of items indexed.

The stage SHALL read only governed inputs: the catalogue and item features from the latest `item_popularity@v1`
featurestore snapshot, user features from the latest `user_activity@v2` snapshot, and the training pairs from the run's own
`als_interactions` dataset. With no snapshot the run SHALL exit 2 before Spark starts and register nothing. Item category and
price are not in those views yet; the towers read them as 0 and nothing substitutes a constant.

The towers SHALL be trained (in-batch softmax over the dataset's pairs) and the run summary SHALL report the first and last
epoch loss. A vector that is all zeros or not finite SHALL NOT be written to the vector store; it SHALL be counted in the
summary, and when no usable vector remains the stage SHALL fail, reject the candidate and publish nothing.

#### Scenario: Pipeline run indexes two-tower vectors and reports the count

- **WHEN** `pipeline.run()` executes with the two-tower stage enabled over a sample catalog
- **THEN** the returned summary contains `two_tower_items` greater than zero
- **AND** the vector-store loader received that many points in the two-tower collection

#### Scenario: Two-tower vectors carry the run generation

- **WHEN** the two-tower stage loads vectors during a run
- **THEN** every loaded point carries the run's `model_version`
- **AND** points from earlier generations are pruned

#### Scenario: Cold-start item receives a vector that ALS cannot produce

- **WHEN** the catalog contains an item with no recorded interactions
- **THEN** that item has a two-tower vector after the run
- **AND** that item has no ALS factor
- **AND** that vector is non-zero and differs from the vector of an item with different recorded features

#### Scenario: A run trains the towers on the dataset's pairs

- **WHEN** the two-tower stage runs over a dataset with interactions
- **THEN** the summary reports a first-epoch and a last-epoch loss, and the model's metadata records the number of pairs,
  the epochs and the feature snapshots (view, version, file, SHA-256) it trained on

#### Scenario: Missing feature snapshots stop the run

- **WHEN** the two-tower stage is enabled and no `item_popularity` snapshot exists
- **THEN** the job exits 2, its log names `ITEM_FEATURES_DIR`, and no model is registered

#### Scenario: A degenerate vector never reaches the vector store

- **WHEN** the catalog contains an item whose features are all zero and the towers are untrained
- **THEN** the summary counts one refused vector and the two-tower collection has no point for that item

### Requirement: Two-tower stage is additive to the ALS baseline

Enabling the two-tower stage SHALL NOT change ALS training, its output contract, or its
collection. With the stage disabled the pipeline SHALL behave exactly as before.

#### Scenario: Disabled stage leaves the ALS run unchanged

- **WHEN** `pipeline.run()` executes with the two-tower stage disabled
- **THEN** the summary matches the ALS-only summary produced before this change
- **AND** no two-tower collection is written

### Requirement: The towers project users and items into one space

Merged from the retired add-two-tower-retrieval. The user tower and the item tower SHALL project user features
(category preferences, activity, lifetime purchases) and item features (category, price, click-through rate, popularity)
into one D-dimensional space. Their vectors SHALL be normalised for cosine similarity.

#### Scenario: User tower and item tower embedding generation

- **GIVEN** user features and item features
- **WHEN** the user tower and the item tower compute embeddings
- **THEN** both vectors have dimension D and unit norm
- **VERIFIED BY**: platform-recsys/tests/test_two_tower.py › test_towers_projection_and_normalization. Not verifiable end to end: the projection is an in-process function; no deployed surface takes a feature dict and returns an embedding.

### Requirement: Two-tower candidates are retrieved by similarity

Merged from the retired add-two-tower-retrieval. Top-K retrieval for a user vector SHALL return item ids ranked by
similarity against the item index.

#### Scenario: Top-K candidate generation for user

- **GIVEN** an item catalog indexed into candidate vectors
- **WHEN** top-K retrieval is requested for a user vector
- **THEN** the top-K item ids are returned ranked by similarity score
- **VERIFIED BY**: platform-recsys/tests/test_two_tower.py › test_top_k_is_ranked_by_similarity_and_bounded. Not verifiable end to end: `TwoTowerModel.retrieve` is in-process and this change has no serving surface (serving-side blending is the placement engine's, a non-goal).

### Requirement: GBDT candidate re-ranking stage

The system SHALL provide a GBDT re-ranking model in `platform-recsys/recsys/ranker/` that scores candidate items using multi-signal feature vectors (similarity score, popularity weight, category affinity match, price affinity) and achieves higher NDCG@10 than raw cosine sorting.

#### Scenario: GBDT ranker outperforms raw cosine baseline on offline eval

- **WHEN** candidates are ranked by the GBDT model versus raw similarity cosine score on the evaluation holdout dataset
- **THEN** the GBDT ranker achieves a higher `ndcg@10` than the baseline cosine sorting

- **VERIFIED BY**: platform-recsys/tests/test_ranker.py › test_gbdt_ranker_outperforms_raw_cosine_ndcg. Not verifiable end to end: the NDCG comparison is offline arithmetic over a labelled holdout; no deployed surface takes a holdout and returns both rankings, and the serving path (team-ai) exposes only the re-ranked list, never the raw-cosine baseline.

### Requirement: Placement-specific recommendation execution

The recommendation service in `team-ai` SHALL route requests by `placement_id` (`home_feed`, `similar_items`, `cart_cross_sell`) and execute the 4-tier fallback ladder when candidates are sparse or cold-start conditions occur.

#### Scenario: Home feed surfaces personalized recommendations with popularity fallback

- **WHEN** a user queries the `home_feed` placement
- **THEN** the service attempts personalized retrieval (Tier 1) and falls back to global popular items (Tier 4) if personalized candidates are unavailable, stamping `placement_id: "home_feed"` and `fallback_tier` in the result

#### Scenario: Similar items placement retrieves item similarities

- **WHEN** a client queries `similar_items` with `seed_listing_id`
- **THEN** the service retrieves similar items via vector similarity and falls back to category/global popular items if sparse

### Requirement: Tracking events are exported for offline training

team-analytics SHALL keep its DuckDB warehouse on persistent storage and, when
`PARQUET_EXPORT_PATH` and a positive `PARQUET_EXPORT_INTERVAL_SECONDS` are set, SHALL
periodically export the `tracking_events` table to that path as Parquet. It SHALL
replace the file atomically, so a reader never sees a partial export. An export
failure SHALL NOT stop event consumption.

#### Scenario: The export replaces the file atomically

- **WHEN** an export runs while a previous export file exists
- **THEN** the new data is written to a temporary file and renamed over the old one

#### Scenario: Export disabled by default

- **WHEN** `PARQUET_EXPORT_INTERVAL_SECONDS` is 0 or unset
- **THEN** no export runs

### Requirement: team-ai reads the trained item vectors by listing id

team-ai's Qdrant backend SHALL look up a seed listing by the producer's point id
(uuid5 of the listing id in the shared namespace), and SHALL return the payload
`listing_id` of each hit as the candidate id, never the Qdrant point id.

#### Scenario: Similar items come back as listing ids

- **WHEN** similar items are requested for a listing present in the trained collection
- **THEN** the query uses that listing's uuid5 point id, and every returned candidate id
  is a listing id from the payload

### Requirement: The local stack serves trained recommendations

The local compose stack SHALL provide a runnable training job that reads the exported
tracking events and fills Qdrant and Redis, and team-ai SHALL serve recommendations
from them (`RECS_ENABLED=true`, Qdrant backend).

#### Scenario: Home page shows trained recommendations

- **WHEN** the training job has run against the stack's tracking events and a buyer opens
  the home page
- **THEN** the "Gợi ý cho bạn" row shows product cards for real listings, sourced from
  team-ai

### Requirement: The two-tower stage reads item attributes and user preferences

When the two-tower stage is enabled it SHALL also read the latest `item_attributes@v1` snapshot
(`ITEM_ATTRIBUTES_DIR`, or `ITEM_ATTRIBUTES_PATH`) and the latest `user_preferences@v1` snapshot (`USER_PREFERENCES_DIR`,
or `USER_PREFERENCES_PATH`), and feed the towers each item's category and price and each user's preferred categories.
A listing present only in the attribute snapshot SHALL be in the catalogue with no engagement. The category vocabulary
SHALL be built from the snapshot's categories (most frequent first, at most `TWO_TOWER_MAX_CATEGORIES`). The model's
metadata SHALL record both snapshots (view, version, file, SHA-256) and the vocabulary size. With
`TWO_TOWER_REQUIRE_ATTRIBUTES` true and no item attribute snapshot the run SHALL exit 2 naming `ITEM_ATTRIBUTES_DIR`
before Spark starts and register nothing; without it a missing snapshot leaves category, price and preferences at 0 and
the metadata records no attribute snapshot.

#### Scenario: Cold-start items with different categories get different vectors

- **WHEN** the job runs with the two-tower stage enabled over attribute snapshots that hold two listings with no
  engagement and no ALS factor, with different categories and the same price
- **THEN** both have a non-zero two-tower vector and the vectors differ

#### Scenario: The model records the attribute snapshots it trained on

- **WHEN** the job runs with the two-tower stage enabled over attribute and preference snapshots
- **THEN** the model's metadata records item_attributes@v1 and user_preferences@v1 with their file and SHA-256 and the category vocabulary size

#### Scenario: Required attribute snapshots missing stop the run

- **WHEN** the job runs with the two-tower stage enabled, TWO_TOWER_REQUIRE_ATTRIBUTES true and no attribute snapshots
- **THEN** it exits 2, its log names ITEM_ATTRIBUTES_DIR, and no model is registered

#### Scenario: Without attribute snapshots the stage runs as before

- **WHEN** the job runs with the two-tower stage enabled over popularity and activity snapshots only
- **THEN** it exits 0 and the model's metadata records no attribute snapshot
