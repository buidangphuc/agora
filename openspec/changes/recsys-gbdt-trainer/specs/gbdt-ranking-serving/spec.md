## ADDED Requirements

### Requirement: Serving ranks with the trained GBDT model of its generation when it is usable

For a `gbdt` placement with online features, team-ai SHALL read `recs:v1:gen:<generation>:ranker` for the request's pinned
serving generation and, when the artifact has `format` `agora-gbdt/1` and a `features` list equal to `RANKING_FEATURES`, rank
candidates by its score (higher first, retrieval similarity as tie-break), with each feature read from the `item_popularity`
and `item_attributes` online rows (0.0 when missing, null or not finite) and `item_popularity.ctr_7d` replaced by the nearline
debiased CTR when usable. The parsed model SHALL be cached per generation and reloaded when the serving generation changes.
When the key is absent, unparseable, of another format, or its feature list differs, or the feature store gave no rows, team-ai
SHALL rank with the built-in fixed weights, never fail or degrade the request, and report `ranker_source` (`trained` or
`fixed`), `ranker_fallback` (the reason) and `ranker_fallbacks` (artifacts rejected so far) in `explain`.

#### Scenario: A published ranker artifact changes the home feed order

- **WHEN** the serving generation carries a ranker artifact that scores the second of two home-feed candidates higher, and the candidates' online features exist
- **THEN** a Recommend call through the gateway ranks the second above the first, and without the artifact the same candidates were ranked in their own order

#### Scenario: A ranker with another feature list is ignored

- **WHEN** the artifact's `features` differ from `RANKING_FEATURES`
- **THEN** the fixed weights rank the request, `explain.ranker_source` is `fixed` with `ranker_fallback` `feature_mismatch`, and the request is served

#### Scenario: A generation switch reloads the model

- **WHEN** the serving pointer moves to a generation whose ranker artifact differs or is absent
- **THEN** the next request scores with that generation's artifact, or with the fixed weights when it has none
