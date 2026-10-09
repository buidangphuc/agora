@recsys @recommendations
Feature: Recommendations are routed by placement (add-placement-engine)
  team-ai routes a Recommend request to the placement its context names (HOMEPAGE is home_feed, SIMILAR_ITEMS
  is similar_items), walks that placement's candidate ladder, and reports the placement in the response. The
  gateway response carries `placementId`; the ladder tier is internal to team-ai and not on the wire.

  # The home_feed scenario needs a buyer the training job knows, so it publishes a generation from a fixture
  # that includes the buyer (as recsys-generation-publish does) and is @destructive (serial lane); its binder
  # republishes a real generation afterwards.

  @needsBuyer
  Scenario: Similar items placement retrieves item similarities
    Given a buyer is logged in
    And a listing present in the serving generation's trained collection
    When a client queries similar_items with that listing as seed_listing_id
    Then the answer is stamped with the similar_items placement and holds the vector neighbours of the seed
    And a seed the trained collection does not know is answered with the global popular items

  @destructive @needsBuyer
  Scenario: Home feed surfaces personalized recommendations with popularity fallback
    Given a buyer is logged in
    And a model has been promoted on the good dataset
    When a user the model trained on queries the home_feed placement
    And a user the model never saw queries the home_feed placement
    Then the trained user's answer is stamped home_feed and drawn from that user's personalized list
    And the unknown user's answer is stamped home_feed and drawn from the global popular items
