# Capability: RecSys Drift Monitoring

## ADDED Requirements

### Requirement: Population Stability Index Calculation
The system MUST calculate PSI for numerical and categorical feature distributions between a baseline dataset and a target dataset.

#### Scenario: Identical distributions have zero drift
- **GIVEN** a baseline feature sample and an identical target feature sample
- **WHEN** PSI is computed
- **THEN** the PSI score is approximately 0.0 and drift status is NO_DRIFT.
- **VERIFIED BY**: platform-recsys/tests/test_drift.py › test_identical_distributions_have_zero_drift. Not verifiable end to end: PSI is arithmetic over two in-memory samples; no deployed surface takes a baseline and a target sample.

#### Scenario: Shifted distribution triggers significant drift alert
- **GIVEN** a baseline distribution centered at one range and a target distribution shifted significantly
- **WHEN** PSI is computed
- **THEN** the PSI score exceeds 0.25 and drift status is SIGNIFICANT_DRIFT.
- **VERIFIED BY**: platform-recsys/tests/test_drift.py › test_shifted_distribution_triggers_significant_drift. Not verifiable end to end: PSI is arithmetic over two in-memory samples; no deployed surface takes a baseline and a target sample.

### Requirement: Multi-Feature Drift Report
The system MUST generate a comprehensive drift assessment across multiple features, reporting individual PSI metrics and an overall model drift flag.

#### Scenario: Multi-feature dataset drift evaluation
- **GIVEN** a dictionary of baseline and target feature columns
- **WHEN** drift evaluation is run
- **THEN** a structured report containing per-feature PSI, drift levels, and whether any feature exceeds the alert threshold is produced.
- **VERIFIED BY**: platform-recsys/tests/test_drift.py › test_drift_detector_multi_feature_and_prometheus. Not verifiable end to end: the report is a library return value; its deployed form is the per-run record covered by the scenarios below.

### Requirement: Each run is compared with the generation it replaces
Every recsys run SHALL summarise its training distribution (the dataset's pair weights, items per user, users per item)
and its predictions (each user's best recommendation score) and store that summary on its model metadata. It SHALL
compute the PSI of those features against the summary stored by the current champion, which is the generation the run
would replace, and record the verdict on the run's model metadata (`parameters.drift`, with the baseline version, the
alert threshold, per-feature PSI and level, and an overall flag; the largest PSI as the metric `drift_psi_max`) and in the
run summary. A feature is flagged when its PSI reaches `DRIFT_ALERT_THRESHOLD` (0.25). The verdict SHALL NOT reject,
delay or otherwise change the promotion decision. A run with no champion, or whose champion has no stored summary, SHALL
record the status `no_baseline`. The gate-independent comparison SHALL NOT use the ALS factors themselves, whose values are
only defined up to a rotation.

#### Scenario: The first run has no drift baseline

- **WHEN** the recsys job runs with an empty registry
- **THEN** the run summary and the model's metadata report the drift status `no_baseline`, and the model stores its
  distribution summary for the next run

#### Scenario: A second run records its drift against the first

- **WHEN** the recsys job runs twice on the same governed dataset
- **THEN** the second run's drift names the first run as its baseline, reports a PSI and a level for each of
  `weight`, `user_items`, `item_users` and `top_score`, and flags none of them

#### Scenario: A drifted run is flagged and still decided by the gate

- **WHEN** the recsys job runs a second time with `DRIFT_ALERT_THRESHOLD` set to 0
- **THEN** the second run's drift is flagged for every feature, its metadata carries the same verdict, and its promotion
  decision is the one the gate gives (the run is promoted)

### Requirement: A run's drift can be exposed as Prometheus metrics

When `DRIFT_METRICS_PATH` is set, the run SHALL write its drift report there in Prometheus text format, replacing the
file atomically, with `recsys_feature_psi{feature,level}` per feature and `recsys_model_drift_alert` (1 when any feature
is flagged). Without a baseline there is no report and nothing is written.

#### Scenario: A drift report is written as Prometheus text

- **WHEN** the recsys job runs twice with `DRIFT_METRICS_PATH` set
- **THEN** the file holds a `recsys_feature_psi` line for each feature and a `recsys_model_drift_alert` line
