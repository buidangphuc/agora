@ai @auth
Feature: AI tools and recommendations are authorized per caller
  As the platform
  I want AI seller tools to need seller rights and recommendations to be bound to the caller
  So that a buyer cannot use seller tools and nobody reads another user's personalised feed

  Scenario: A buyer cannot use the listing generator
    Given a logged-in buyer
    When the buyer calls MagicListing through the gateway
    Then the gateway answers HTTP 403

  Scenario: A seller cannot run the copilot as another seller
    Given a logged-in seller
    And another seller exists
    When the seller calls ChatCopilot through the gateway with the other seller's seller_id
    Then the gateway answers HTTP 403
    And the copilot returns no quick replies

  @recommendations
  Scenario: A buyer cannot fetch another user's recommendations
    Given two buyers with distinct precomputed recommendation lists
    When buyer A calls Recommend through the gateway naming buyer B's user_id
    Then the recommendations returned are buyer A's own list
    And none of buyer B's recommended listings are returned
    And buyer B calling Recommend as themselves gets buyer B's list
