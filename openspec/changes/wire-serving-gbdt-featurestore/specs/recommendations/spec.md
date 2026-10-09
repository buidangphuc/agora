## ADDED Requirements

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
