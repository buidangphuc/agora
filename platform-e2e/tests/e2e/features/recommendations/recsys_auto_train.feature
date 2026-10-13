@recsys @recommendations @destructive
Feature: The nightly recsys job is gated by the MLflow model registry (recsys-auto-train)
  The recsys job registers every candidate as a version of recsys-als in MLflow, lets the shared
  gate decide against the seeded champion, and publishes a new generation only when the candidate
  is promotable. It fails closed when the registry is down, refuses a stale dataset, and never puts
  user keys into the registry.

  # Every scenario rewrites the shared serving pointer, the Qdrant aliases and the MLflow registry,
  # so the whole feature is @destructive (serial lane). Determinism (D13): each scenario starts with
  # a reset (delete the recsys-* registered models, forget every generation), then seeds explicitly:
  # a prior generation g1 published by the job, registry champions through the library's seed. Pass
  # and fail are driven by a weak or a strong seeded champion. The scenarios assert "the newest
  # version of recsys-als"; no generation to version mapping is assumed. A module teardown republishes
  # a real generation, so other recommendation scenarios do not read fixture listings.

  Scenario: Gate pass serves the new generation
    Given a buyer is logged in
    And the recsys serving state and the recsys registry are reset
    And a prior generation g1 is published and serving, then the registry is cleared again
    And a weak champion is seeded for "recsys-als"
    When the recsys job runs on a new dataset
    Then the job exits with code 0
    And the newest version of "recsys-als" is tagged "gate.outcome" "promoted"
    And the alias "champion" of "recsys-als" names its newest version
    And the serving pointer names a generation other than g1
    And the previous pointer names g1
    And a buyer's Recommend answers from the new generation

  Scenario: Gate fail keeps the previous generation
    Given a buyer is logged in
    And the recsys serving state and the recsys registry are reset
    And a prior generation g1 is published and serving, then the registry is cleared again
    And a strong champion is seeded for "recsys-als"
    When the recsys job runs on a new dataset
    Then the job exits with code 0
    And the newest version of "recsys-als" is tagged "gate.outcome" "rejected"
    And the "gate.reason" of the newest version of "recsys-als" names "ndcg@10"
    And the alias "champion" of "recsys-als" still names the seeded version
    And the serving and previous pointers are unchanged
    And no new generation keys or collections exist
    And a buyer's Recommend answers are identical to before the run

  Scenario: Registry down fails closed
    Given the recsys serving state and the recsys registry are reset
    And a prior generation g1 is published and serving, then the registry is cleared again
    And the recsys registry is down
    When the recsys job runs
    Then the job exits with code 1 within 20 seconds
    And the job did not start Spark
    And the serving and previous pointers are unchanged
    And no new generation keys or collections exist

  Scenario: Stale dataset is refused
    Given the recsys serving state and the recsys registry are reset
    And a prior generation g1 is published and serving, then the registry is cleared again
    And a weak champion is seeded for "recsys-als"
    And the newest dataset snapshot is 26 hours old
    When the recsys job runs with MAX_DATASET_AGE_MINUTES set to 1500
    Then the job exits with code 2
    And no recsys model has a new version in MLflow
    And the serving and previous pointers are unchanged
    And no new generation keys or collections exist

  Scenario: Registry holds no user data
    Given the recsys serving state and the recsys registry are reset
    And a weak champion is seeded for "recsys-als"
    And a dataset of fifty users keyed u-0001 to u-0050
    When the recsys job runs
    Then the job exits with code 0
    And the newest version of "recsys-als" is tagged "gate.outcome" "promoted"
    And a scan of every run's params, metrics, tags, run name and artifacts finds no fixture user key
    And no artifact is named like user factors

  # Support scenario (negative control of the scan used above): it needs no recsys code, so it is
  # expected to pass before the change is implemented. It proves the scan CAN fail.
  Scenario: PII scan negative control
    Given a scratch MLflow run with "u-0001" logged as a param and as artifact text
    When the PII scan runs on that run for the fixture user keys
    Then the scan reports "u-0001" in a param and in an artifact

  Scenario: Nightly chain registers all three models
    Given the recsys serving state and the recsys registry are reset
    And weak champions are seeded for recsys-als, recsys-gbdt and recsys-two-tower
    When the featurestore dataset job and then the recsys compose job run
    Then the chain's jobs both exit 0
    And each of recsys-als, recsys-gbdt and recsys-two-tower has a newest version tagged promoted
