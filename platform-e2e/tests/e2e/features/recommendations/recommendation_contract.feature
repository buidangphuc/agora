@recommendations @contract
Feature: Recommendation serving contract in platform-core (add-recommendation-contract)
  platform-core/packages/proto defines platform.recommendation.v1.RecommendationService
  with a single Recommend(RecommendRequest) returns (RecommendResponse) RPC, added
  additively so no existing contract is renumbered or broken. RecommendRequest carries
  caller identity, an optional seed listing, a RecommendationContext, and a limit;
  RecommendResponse returns ranked listing ids with scores and a model_version.

  # Contract-level checks: buf lint/breaking over packages/proto and message shape
  # assertions on the generated recommendation types. No live stack required.

  @proto
  Scenario: The recommendation proto lints and stays non-breaking
    Given platform/recommendation/v1/recommendation.proto is added to packages/proto
    When buf lint and buf breaking run over packages/proto
    Then lint passes under STANDARD rules with enum values prefixed RECOMMENDATION_CONTEXT_
    And the breaking check passes because only a new package is added with no existing message, field number, or RPC touched

  @proto
  Scenario: An anonymous PDP "similar items" request is expressible
    Given a caller builds a RecommendRequest with an empty user_id and an anonymous_id
    And a seed_listing_id set to the viewed listing, context RECOMMENDATION_CONTEXT_SIMILAR_ITEMS, and limit 12
    When the message is validated against the contract
    Then the message is valid
    And the serving layer has every field it needs to choose an item-item strategy for an anonymous visitor without a schema change

  @proto
  Scenario: A ranked result set is expressible with provenance
    Given the serving layer fills a RecommendResponse with ordered RecommendedItems
    And the model_version of the batch artifact it read
    When a consumer reads the response
    Then it can render the ranking in order using each listing_id and rank
    And it can sort or threshold on score and trace the producing model via model_version
