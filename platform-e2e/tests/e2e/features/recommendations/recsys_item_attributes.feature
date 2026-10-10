@recsys @recommendations @batch
Feature: The two-tower stage learns from item attributes (featurestore-item-attributes)
  The real platform-recsys job image runs against the stack in an isolated namespace (a dedicated Redis DB and
  Qdrant collections, see fir_job_flow) over feature snapshot fixtures that include item_attributes@v1 and
  user_preferences@v1. Needs the rebuilt platform-recsys image.

  Scenario: Cold-start items with different categories get different vectors
    When the recsys job runs with the two-tower stage enabled over attribute snapshots that hold two listings with no engagement and no ALS factor
    Then both cold listings have a non-zero two-tower vector and the vectors differ

  Scenario: The model records the attribute snapshots it trained on
    When the recsys job runs with the two-tower stage enabled over attribute and preference snapshots
    Then the model's metadata records item_attributes@v1 and user_preferences@v1 with their file and SHA-256 and the category vocabulary size

  Scenario: Required attribute snapshots missing stop the run
    When the recsys job runs with the two-tower stage enabled, TWO_TOWER_REQUIRE_ATTRIBUTES true and no attribute snapshots
    Then the job exits 2, its log names ITEM_ATTRIBUTES_DIR, and no model is registered

  Scenario: Without attribute snapshots the stage runs as before
    When the recsys job runs with the two-tower stage enabled over popularity and activity snapshots only
    Then the job exits 0 and the model's metadata records no attribute snapshot
