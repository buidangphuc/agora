@recsys @recommendations
Feature: The local stack trains on its own events and serves the result (serve-trained-recs-locally)
  team-analytics exports its tracking events to Parquet, the recsys job trains on them and fills Qdrant and
  Redis, and team-ai serves the generation by listing id. The evaluation of a training run never scores a
  model on the pairs it trained on, and never compares numbers from different evaluation protocols.

  # The export scenarios run the REAL team-analytics image as a throwaway container on the stack network, with
  # its own warehouse directory and its own Kafka consumer groups (it never takes events from the stack's
  # consumer), so they do not wait for the stack's 300 s interval or touch its volume. The evaluation scenarios
  # run the real platform-recsys image in a per-worker namespace (recsys_job_flow).

  Scenario: The export replaces the file atomically
    Given a team-analytics exporting every second whose previous export file exists
    When further exports replace the file while a reader keeps opening it
    Then the file was replaced by a new one at least twice
    And the reader never saw an incomplete Parquet file

  Scenario: Export disabled by default
    Given team-analytics started with PARQUET_EXPORT_INTERVAL_SECONDS set to 0 and another with it unset
    When both have run for several seconds
    Then neither wrote an export file
    And both are still running with a warehouse

  @needsBuyer
  Scenario: Similar items come back as listing ids
    Given a buyer is logged in
    And a listing present in the serving generation's trained collection
    When similar items are requested for that listing through the gateway
    Then the answer is the nearest neighbours of that listing's uuid5 point
    And every returned id is a listing id from the payload, never a Qdrant point id

  @needsBuyer
  Scenario: Home page shows trained recommendations
    Given a buyer is logged in
    And the training job has published a generation from the stack's tracking events
    When I navigate to the "home" page
    Then the "Gợi ý cho bạn" row shows cards for listings of that trained generation

  Scenario: The evaluation model never trains on its targets
    Given every user's most recently discovered listing is one nobody else interacted with
    When the pipeline evaluates and trains over those interactions
    Then the evaluation was trained on every pair except the held-out ones
    And the run's metrics carry the evaluation protocol identifier
    And the held-out listings score zero for the evaluation model while the published model learned them

  Scenario: Metrics from another evaluation protocol are not compared
    Given an incumbent champion whose metrics carry no evaluation protocol and a score the candidate cannot match
    When the pipeline completes a run under the current protocol
    Then the candidate becomes the champion
    And the promotion reason names both protocols
