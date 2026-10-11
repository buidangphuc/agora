@recsys @recommendations @batch
Feature: ML recsys changes run end to end (add-recsys-nearline-signals, add-recsys-drift-monitoring, wire-two-tower-batch-pipeline)
  The real platform-recsys job image runs against the stack in an isolated namespace (a dedicated Redis DB and
  Qdrant collections, see mlr_job_flow). Nearline scenarios feed it through the gateway edge (POST /api/track ->
  analytics.events -> `python -m recsys.nearline`). Needs the rebuilt platform-recsys image.

  Scenario: User recent views update nearline signals
    Given a buyer is logged in
    When the buyer views two listings in one session and the nearline consumer drains the topic
    Then the nearline layer records the second listing then the first in the buyer's recent items list in Redis
    And the category affinities of both listings are incremented

  Scenario: Real-time item co-occurrence is tracked
    Given a buyer is logged in
    When the buyer views the same two listings in two sessions and the nearline consumer drains the topic
    Then the co-view count between the two listings is 2 in Redis

  Scenario: A generation switch leaves nearline keys alone
    Given nearline keys exist in Redis
    When the recsys job promotes three generations in a row
    Then the nearline keys are unchanged and keep their TTL

  Scenario: The first run has no drift baseline
    When the recsys job runs with an empty registry
    Then the run summary and the model's metadata report the drift status no_baseline
    And the model stores its distribution summary for the next run

  Scenario: A second run records its drift against the first
    When the recsys job runs twice on the same governed dataset
    Then the second run's drift names the first run as its baseline and reports a PSI and a level for each feature
    And none of the features is flagged

  Scenario: A drifted run is flagged and still decided by the gate
    When the recsys job runs a second time with DRIFT_ALERT_THRESHOLD set to 0
    Then every feature of the second run's drift is flagged and its metadata carries the same verdict
    And the second run is promoted

  Scenario: A drift report is written as Prometheus text
    When the recsys job runs twice with DRIFT_METRICS_PATH set
    Then the drift metrics file holds a recsys_feature_psi line for each feature and a recsys_model_drift_alert line

  Scenario: Pipeline run indexes two-tower vectors and reports the count
    When the recsys job runs with the two-tower stage enabled over the feature snapshots
    Then the summary reports two_tower_items greater than zero
    And the generation's two-tower collection holds that many points

  Scenario: Two-tower vectors carry the run generation
    When the recsys job runs three times with the two-tower stage enabled
    Then every point of each run's two-tower collection carries that run's model version
    And only the serving and previous generations' two-tower collections remain

  Scenario: Cold-start item receives a vector that ALS cannot produce
    When the recsys job runs with the two-tower stage enabled over the feature snapshots
    Then the item without interactions has a non-zero two-tower vector that differs from another item's
    And the item without interactions has no ALS vector

  Scenario: A run trains the towers on the dataset's pairs
    When the recsys job runs with the two-tower stage enabled over the feature snapshots
    Then the summary reports a first-epoch and a last-epoch loss
    And the model's metadata records the pairs, the epochs and the feature snapshots it trained on

  Scenario: Missing feature snapshots stop the run
    When the recsys job runs with the two-tower stage enabled and no feature snapshots
    Then the job exits 2 and its log names ITEM_FEATURES_DIR
    And no model is registered

  Scenario: A degenerate vector never reaches the vector store
    When the recsys job runs with untrained towers over a catalogue with an item without any features
    Then the summary counts one refused vector
    And the two-tower collection has no point for that item

  Scenario: Disabled stage leaves the ALS run unchanged
    When the recsys job runs with the two-tower stage disabled
    Then the summary has no two-tower entries
    And no two-tower collection is written
