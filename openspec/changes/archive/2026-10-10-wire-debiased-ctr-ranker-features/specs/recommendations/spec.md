## ADDED Requirements

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
