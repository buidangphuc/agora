## Purpose

Defines how the recommendation system consumes behavioural data only through the feature store owned by
`team-analytics`: the trainer declares a feature view and reads that view's point-in-time dataset builds,
and serving reads pinned online features through the feature read contract — never raw events, never
another service's database.

## ADDED Requirements

### Requirement: The trainer declares its input as a feature view

`platform-recsys` SHALL declare its training input in the consumer-owned view file
`platform-recsys/feature_views/als_interactions@v1.yaml`, naming the entity (`user_item`), each feature with
a pinned version (`user_item.implicit_score_decayed@v1`), the point-in-time snapshots it needs and the
history window. Changing the training input SHALL be an edit of the view (or a new view version), not of
feature logic inside `platform-recsys`.

#### Scenario: The view file pins its features

- **WHEN** the trainer's configuration is checked in CI
- **THEN** the view file exists, every feature it lists carries an explicit `@v<N>` version, and the
  trainer's input configuration names that view and no other data source

### Requirement: The trainer triggers and consumes only complete, fresh, matching dataset builds

Before every training run the trainer SHALL request a build of its view from the feature store
(`BuildDataset`, as its service principal holding the service-only scope `features.dataset`) and wait for that
build (`GetDatasetBuild`) up to a configured timeout, and SHALL train only on that build, never on an older one.
It SHALL refuse to train — recording the reason in the run report and leaving the served generation unchanged —
when the build fails or does not finish in time, when the manifest's schema version is unknown, when the
build's feature list or versions differ from the view file, when the build's watermark is older than the
configured maximum age at completion, or when its row count is below the configured minimum. The rows read
SHALL equal the manifest's row count.

#### Scenario: Each run trains on the build it requested

- **WHEN** a training run starts and an older complete build of the view also exists
- **THEN** the trainer requests a new build, waits until it is done, trains on it, and the report names the
  requested build id

#### Scenario: A failed or slow build refuses the run

- **WHEN** the requested build ends in a failed state, or is not done within the configured timeout
- **THEN** the run is recorded as refused with reason `dataset build failed` or `dataset build timeout`, and
  `Recommend` keeps returning the previous model version

#### Scenario: A stale build is refused

- **WHEN** the requested build's watermark is older than the configured maximum dataset age because
  ingestion has stalled
- **THEN** the run is recorded as refused with reason `dataset stale` and `Recommend` keeps returning the
  previous model version

#### Scenario: A build with other feature versions is refused

- **WHEN** the requested build lists `user_item.implicit_score_decayed@v2` while the view pins `@v1`
- **THEN** the run is recorded as refused with reason `feature version mismatch`

#### Scenario: An unknown dataset schema is refused

- **WHEN** the requested build's manifest carries a schema version the trainer does not know
- **THEN** the run is recorded as refused with reason `unknown schema version` and nothing is published

### Requirement: The trainer never reads raw events

No code path, configuration option or deployed setting of the training job SHALL read raw behavioural
events (`tracking_events`, `analytics.events` or any warehouse table) or any service database. A test
input driver reading a local directory in the dataset layout MAY exist only for test and local
environments. CI SHALL fail when the trainer's code or configuration references a raw event source.

#### Scenario: A raw-event reader fails CI

- **WHEN** a change adds a reference to `tracking_events` anywhere under the trainer's source or
  configuration
- **THEN** the trainer's contract test fails naming the file

#### Scenario: The test input driver is refused in a deployed environment

- **WHEN** the trainer starts with the test input driver and `ENV=staging`
- **THEN** it exits with an error naming the input driver before reading any data

### Requirement: Serving reads pinned online features through the feature read contract

team-ai SHALL read online features only through `FeatureService.GetOnlineFeatures` as a service principal
holding the `features.read` scope, requesting only the feature versions pinned in its placement
configuration, with at most one call per request bounded by `RECS_FEATURES_TIMEOUT_MS`. A missing feature,
an error or a timeout SHALL make the strategies that need it yield nothing, SHALL be counted as a
`feature_failure` distinct from a visitor without history, and SHALL never fail the request.

#### Scenario: Recent views personalize a cold visitor

- **WHEN** an anonymous visitor views two listings and the online store holds them in
  `user.recent_items_24h` for that visitor
- **THEN** the visitor's next `home.for_you` response starts with listings similar to those two

#### Scenario: A slow feature service does not slow the row past its budget

- **WHEN** `GetOnlineFeatures` takes longer than `RECS_FEATURES_TIMEOUT_MS`
- **THEN** `Recommend` answers within its budget from the remaining strategies and the `feature_failure`
  counter increases
