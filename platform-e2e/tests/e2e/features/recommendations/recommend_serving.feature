@recommendations @ai
Feature: team-ai serves the Recommend RPC (serve-recommendations-teamai)
  team-ai implements platform.recommendation.v1.RecommendationService/Recommend
  as a thin gRPC transport over a recommend module. The module runs a two-stage
  Qdrant ANN retrieval plus business-rule ranking, with a Redis pre-computed
  fast path, all gated behind RECS_ENABLED. Anonymous and cold-start callers
  still get a non-empty row, and the serve path stays within its latency budget
  by falling back to popularity on retrieval timeout.

  # Exercises recommendation.v1.Recommend directly against team-ai's gRPC port.
  # A green run needs team-ai up with RECS_ENABLED=true and the training-job
  # Qdrant/Redis artifacts populated.

  @needsRecsEnabled
  Scenario: Recommend returns a Top-10 product list
    Given team-ai is running with RECS_ENABLED=true and training-job artifacts populated
    When a caller invokes Recommend with a user_id
    Then team-ai returns a RecommendResponse carrying at most ten recommended product cards
    And the products are produced by the recommend module, not an empty or mocked response

  @needsRecsDisabled
  Scenario: Recommend is unavailable when the flag is off
    Given team-ai is running with RECS_ENABLED=false
    When a caller invokes Recommend
    Then the call fails with gRPC status UNAVAILABLE and a message naming the flag
    And no Qdrant or Redis access is attempted

  @needsRecsEnabled
  Scenario: Out-of-stock and seed items are filtered from candidates
    Given the recommend module retrieves ANN candidates including an out-of-stock item, a duplicate, and the seed_listing_id
    When Recommend ranks and filters the candidates
    Then those items are removed from the result
    And the response contains at most ten distinct in-stock products, none of them the seed listing

  @needsRecsEnabled
  Scenario: Cache hit serves without a Qdrant query
    Given a user whose pre-computed Top-N list is present in Redis
    When Recommend is called for that user
    Then the response is built from the cached list
    And no Qdrant ANN query is issued

  @needsRecsEnabled
  Scenario: Cache miss falls back to Qdrant retrieval
    Given a user with no cached list or an unreachable Redis
    When Recommend is called for that user
    Then the module retrieves candidates from Qdrant
    And it still returns a Top-10 result

  @needsRecsEnabled
  Scenario: Anonymous request seeded from a listing returns similar items
    Given Recommend is called with an empty user_id and a seed_listing_id
    When the module retrieves candidates from Qdrant seeded by the listing
    Then it returns items similar to the seed listing
    And the items are filtered to in-stock and exclude the seed

  @needsRecsEnabled
  Scenario: No user, no seed falls back to popular items
    Given Recommend is called with no user_id and no seed_listing_id
    When the module has no anchor to retrieve from
    Then it returns a non-empty popularity-based Top-10 rather than an empty response

  @needsRecsEnabled
  Scenario: Retrieval timeout yields a fallback, not an error
    Given the Qdrant retrieval exceeds RECS_RETRIEVE_TIMEOUT_MS
    When Recommend hits the retrieval timeout
    Then Recommend returns the popularity fallback list
    And it does not fail or block past the latency budget
