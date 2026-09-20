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

#### Scenario: Placement without a GBDT model keeps retrieval ordering

- **WHEN** `service.recommend` is called for a placement whose `ranking.model` is not `gbdt`
- **THEN** the returned order is the retrieval-score order
- **AND** `explain["ranking_model"]` reports that model

### Requirement: Feature-store enrichment is observable per request

When the active placement declares `use_featurestore: true`, the service SHALL enrich candidates
with item features before ranking and SHALL report how many candidates were enriched.

#### Scenario: Enrichment count proves the feature store was read

- **WHEN** `service.recommend` runs for a placement with `use_featurestore: true` and the feature
  store holds features for at least one returned candidate
- **THEN** `explain["featurestore_hit_count"]` is greater than zero

### Requirement: Placements cannot declare unbound capabilities

The application SHALL reject, at startup, any placement declaring a ranking model or capability
that has no bound implementation. A declared capability that no code reads SHALL NOT be loadable.

#### Scenario: Unbound ranking model fails at startup

- **WHEN** the placement registry loads a placement whose `ranking.model` names a model with no
  registered implementation
- **THEN** application startup fails with an error naming the placement and the missing binding
- **AND** no request is served with the unbound configuration

### Requirement: Ranking failure degrades rather than errors

Ranker or feature-store failure SHALL NOT fail the request. The service SHALL fall back to
retrieval-score ordering and mark the response degraded.

#### Scenario: Ranker failure falls back and marks degraded

- **WHEN** the ranker raises during `service.recommend`
- **THEN** items are returned in retrieval-score order
- **AND** the response `status` is `"degraded"`
