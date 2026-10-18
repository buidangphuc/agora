@engagement
Feature: Hiding a review is published as a ReviewHidden fact
  An effective admin hide enqueues one ReviewHidden fact in the same transaction; it reaches
  engagement.events and the analytics warehouse without moderator data. A repeated or refused
  hide publishes nothing. Everything runs through the gateway.
  Scenario names echo the spec scenario titles (engagement-review-hidden-event) so spec_sync
  can match them.

  Scenario: Review hidden event: admin hide appears on engagement.events
    Given a seller with a published listing "L1"
    And a buyer "b1" with a completed order of "L1"
    And "b1" has reviewed "L1" with rating 2 from their completed order
    And an admin
    When the admin hides the review of "b1" with the reason "spam link"
    Then the hide call succeeds
    And exactly one "ReviewHidden" envelope for the review of "b1" and "L1" appears on engagement.events
    And that "ReviewHidden" envelope does not contain "spam link"

  Scenario: Review hidden event: repeated hide publishes nothing new
    Given a seller with a published listing "L1"
    And a buyer "b1" with a completed order of "L1"
    And "b1" has reviewed "L1" with rating 2 from their completed order
    And an admin
    And the admin hides the review of "b1" with the reason "spam link"
    When the admin hides the review of "b1" with the reason "spam link"
    Then the hide call succeeds
    And exactly one "ReviewHidden" envelope for the review of "b1" and "L1" appears on engagement.events

  Scenario: Review hidden event: a buyer cannot hide a review
    Given a seller with a published listing "L1"
    And a buyer "b1" with a completed order of "L1"
    And "b1" has reviewed "L1" with rating 5 from their completed order
    And the "ReviewCreated" envelope of the review of "b1" has reached engagement.events
    When "b1" hides their own review with the reason "changed my mind"
    Then the hide call is refused with HTTP 403 and "permission_denied"
    And no "ReviewHidden" envelope for the review of "b1" appears on engagement.events within 10 seconds

  Scenario: Review hidden event: analytics records review_hidden
    Given a seller with a published listing "L1"
    And a buyer "b1" with a completed order of "L1"
    And "b1" has reviewed "L1" with rating 2 from their completed order
    And an admin
    When the admin hides the review of "b1" with the reason "spam link"
    Then the hide call succeeds
    And within 30 seconds the warehouse holds one review_hidden record for the review of "b1" and "L1"
