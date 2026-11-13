@analytics @integration
Feature: A UserErased event erases the ML and serving stores and the local stack wires every consumer
  platform-recsys (`python -m recsys.erasure`), platform-featurestore (`materialize`) and team-ai (an
  erasure addon) act on `identity.events/UserErased` with their own consumer groups and DLQs
  (identity.events.recsys.dlq, identity.events.team-ai.dlq). Serving keys live in the stack Redis
  (AUE_REDIS_DB, default 0) and Qdrant (collections user_als_vectors__*); the featurestore job is the
  real image. Groups default to platform-recsys.erasure and team-ai.user-erasure. Users of the Redis,
  Qdrant and team-ai scenarios are synthetic ids, because no gateway RPC creates those keys.
  (analytics-user-erasure / ml-stores-erasure)

  Scenario: AUE-19 Recommendation and nearline keys are removed
    Given a recsys user "U" with recommendation keys under two generations, a legacy key, two nearline keys and a Qdrant vector
    And a recsys user "V" with the same keys
    When a UserErased event for the synthetic user "U" is produced
    Then within 60 seconds zero Redis keys and zero Qdrant points contain the id of "U"
    And the keys and the Qdrant point of "V" are unchanged

  Scenario: AUE-21 Redelivery and missing keys are no-ops
    Given a recsys user "U" with recommendation keys under two generations, a legacy key, two nearline keys and a Qdrant vector
    And a recsys user "V" with the same keys
    When a UserErased event for the synthetic user "U" is produced
    And the same UserErased event for the synthetic user "U" is produced again
    And a UserErased event for the synthetic user "W" with no keys is produced
    Then the recsys erasure consumer group has no lag
    And zero Redis keys and zero Qdrant points contain the id of "U"
    And the keys and the Qdrant point of "V" are unchanged
    And the recsys DLQ holds no record for those events

  Scenario: AUE-22 Poison record goes to the DLQ
    Given a recsys user "V" with the same keys
    When a record that is not an EventEnvelope and a UserErased with an empty user_id are produced
    Then both records are on identity.events.recsys.dlq
    And the recsys erasure consumer group has no lag
    And the keys and the Qdrant point of "V" are unchanged

  Scenario: AUE-24 A user erased upstream disappears from the online store
    Given a seller "S" with listings "L1", "L2" and "L3"
    And a buyer "U"
    And "U" has 5 tracking views of "L1" in the warehouse
    And the Parquet export files contain the id of "U"
    And the featurestore online store holds user_activity and user_preferences keys of "U"
    When a UserErased event for "U" is produced
    And after one export cycle the Parquet files hold no id of "U"
    And the materialize job runs
    Then both featurestore keys of "U" are gone and no featurestore key has an erased entity
    And the parity check passes

  Scenario: AUE-28 Chat sessions, quota and rate-limit keys are removed
    Given a team-ai user "U" with three chat sessions, a quota counter and a rate-limit counter
    And a team-ai user "V" with the same keys
    When a UserErased event for the synthetic user "U" is produced
    Then within 60 seconds none of the team-ai keys of "U" exists
    And the team-ai keys of "V" are present

  Scenario: AUE-29 A prefix-sharing id is not matched
    Given a team-ai user "abc" with a chat session
    And a team-ai user "abcd" with a chat session
    When a UserErased event for the synthetic user "abc" is produced
    Then within 60 seconds none of the team-ai keys of "abc" exists
    And the team-ai keys of "abcd" are present

  Scenario: AUE-31 Poison record goes to the DLQ
    Given a team-ai user "V" with the same keys
    When a record that is not an EventEnvelope and a UserErased with an empty user_id are produced
    Then both records are on identity.events.team-ai.dlq
    And the team-ai erasure consumer group has no lag
    And the team-ai keys of "V" are present

  Scenario: AUE-33 End to end across the stores
    Given a seller "S" with listings "L1", "L2" and "L3"
    And a buyer "U"
    And "U" has 5 tracking views of "L1" in the warehouse
    And "U" has a paid order of "L1", "L2" and "L3" in the warehouse
    And the Parquet export files contain the id of "U"
    And the seeded user "U" also has recommendation keys, a Qdrant vector, featurestore keys and a chat session
    When a UserErased event for the seeded user "U" is produced
    And after one export cycle the Parquet files hold no id of "U"
    And the materialize job runs
    Then the warehouse, Parquet files, Redis and Qdrant contain zero occurrences of the id of "U"
    And the three erasure DLQ topics hold zero records for that event

  Scenario: AUE-34 A missing DLQ topic stops the consumer
    Then the three erasure DLQ topics exist on the broker
    And the analytics, recsys and team-ai erasure consumer groups exist
