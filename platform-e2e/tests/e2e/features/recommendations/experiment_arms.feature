@recommendations @ai @destructive
Feature: Recommendation responses name their experiment arm and serving path (recs-experiment-arms)
  The home feed evaluates the Flipt variant flag recs-home-feed-ranker with a salted hash of the caller's
  id, and every response's model_version reads <generation>/<tier>/<ranker>/<arm>.

  # Serial lane: the scenarios toggle the shared Flipt flag (restored disabled in teardown) and one
  # of them stops and restarts the flipt container.

  Scenario: A visitor always lands in the same arm
    Given the experiment flag is enabled with a 50/50 split
    When one anonymous visitor requests homepage recommendations 5 times through the gateway
    Then all 5 responses carry the same arm segment, control or similarity of recs-home-feed-ranker

  Scenario: Both variants receive traffic
    Given the experiment flag is enabled with a 50/50 split
    When 40 distinct anonymous visitors request homepage recommendations
    Then the responses contain both arm "recs-home-feed-ranker:control" and arm "recs-home-feed-ranker:similarity"

  Scenario: A disabled flag means no arm
    Given a buyer is logged in
    And the experiment flag exists and is disabled
    When a buyer requests homepage recommendations through the gateway
    Then the arm segment of model_version is "none"

  Scenario: The similarity arm changes the ranker segment
    Given the experiment flag is enabled with a 50/50 split
    When visitors are requested until one is assigned the similarity variant
    Then the ranker segment of that response is "similarity"
    And the tier segment of that response is one the default ladder produces

  Scenario: Flipt unreachable serves the default ranker as unassigned
    Given a buyer is logged in
    And the experiment flag is enabled with a 50/50 split
    When Flipt is stopped and a buyer requests homepage recommendations through the gateway
    Then the call succeeds, the ranker segment is fixed_weights or gbdt, and the arm segment is "recs-home-feed-ranker:unassigned"

  Scenario: Seeding creates a disabled flag and is idempotent
    Given the experiment flag does not exist
    When the seed script runs twice
    Then the flag exists with enabled false and the variants control and similarity
    And both runs exited 0 and the second changed nothing

  Scenario: A buyer with precomputed recommendations reports the user tier
    Given a buyer is logged in
    When a buyer with a precomputed list requests homepage recommendations through the gateway
    Then model_version matches the serving generation, user_precomputed, a ranker and an arm

  Scenario: A similar-items request reports the seed tier and similarity ranker
    Given a buyer is logged in
    And a seed listing and five neighbours, all published with stock 5 in search, each of a different seller and each with Qdrant in_stock true
    When Recommend for similar_items is called through the gateway with the seed listing
    Then model_version matches the serving generation, seed_vector_similarity, similarity and none

  Scenario: The served log line carries tier, ranker and arm
    Given a buyer is logged in
    When a buyer requests homepage recommendations twice through the gateway
    Then each recs.served log line carries the tier, ranker and arm of that response's model_version and no raw user id
