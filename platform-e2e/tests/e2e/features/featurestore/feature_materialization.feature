@analytics @batch
Feature: Features are materialised from the warehouse exports, point in time
  `python -m featurestore materialize` computes user_activity@v2 and item_popularity@v1 from the
  Parquet exports of team-analytics, as of AS_OF, into an offline snapshot with a manifest and into
  Redis (DB 2) under versioned keys. It runs here as the real job image on the stack network.
  (featurestore-materialization / feature-materialization)

  The scenarios share one batch of tracked activity (one visitor, one seller with a listing and one
  new buyer), created by the first scenario that runs, so the 300 s export cycle is waited out once.
  No @slow marker exists in pyproject.toml; the scenarios that wait for the export are unmarked.

  Scenario: A tracked view reaches the resolved export
    When a visitor posts a view of a unique listing and the next export cycle completes
    Then tracking_events_resolved.parquet on the analytics volume contains that listing with a user_key

  Scenario: A buyer's activity becomes features
    Given a seller with a published listing "L1"
    When a new buyer "b1" views "L1" three times and favourites it, the export cycle completes, and the materialisation job runs
    Then the online features of "b1" under user_activity@v2 have 3 views in the last 7 days and 1 current favourite
    And the item_popularity@v1 features of "L1" have at least 3 views and at least 1 current favourite

  Scenario: Events after AS_OF are not used
    Given a seller with a published listing "L1"
    And a new buyer "b1" has viewed "L1" three times and favourited it, and the export cycle has completed
    When the job runs with AS_OF set to a time just before "b1"'s first event
    Then "b1" has no user_activity@v2 row in the offline snapshot of that run

  Scenario: A run leaves a snapshot and a manifest
    Given a new buyer "b1" has viewed a listing and the export cycle has completed
    When the materialisation job runs twice with different AS_OF values
    Then two snapshots exist for each view, and each manifest names its as_of, a positive row count and a 64-hex definition hash

  Scenario: The online store says how fresh it is
    Given a new buyer "b1" has viewed a listing and the export cycle has completed
    When the materialisation job finishes
    Then fs:user_activity:current is "2" and fs:user_activity:meta carries the run's as_of and an input watermark no later than it

  Scenario: A tampered online value fails the parity check
    Given a new buyer "b1" has viewed a listing and the export cycle has completed
    When after a run, "b1"'s user_activity@v2 online value is overwritten with a different view count, and python -m featurestore parity runs
    Then the command exits non-zero and its output names "b1"
