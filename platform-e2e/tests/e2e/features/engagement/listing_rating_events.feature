@engagement
Feature: Listing rating changes are published as absolute values
  CreateReview and an effective HideReview write a ListingRatingChanged fact with the absolute
  average and count in the same transaction; it reaches engagement.events and equals
  GetListingRatingSummary. Rejected calls write nothing. Migration 0015 backfills reviewed
  listings. Everything runs through the gateway.
  Scenario names echo the spec scenario titles (engagement-listing-rating-events) so spec_sync
  can match them.

  Scenario: A new review publishes the new average and count
    Given a seller with a published listing "L1"
    And a buyer "b1" with a completed order of "L1"
    And a buyer "b2" with a completed order of "L1"
    And "b1" has reviewed "L1" with rating 5 from their completed order
    When "b2" reviews "L1" with rating 2 from their completed order
    Then a "ListingRatingChanged" envelope for "L1" with review count 2 and average 3.5 appears on engagement.events
    And GetListingRatingSummary of "L1" returns review count 2 and average 3.5
    And no "ListingRatingChanged" envelope for "L1" carries a principal

  Scenario: Hiding a review lowers the published values
    Given a seller with a published listing "L1"
    And a buyer "b1" with a completed order of "L1"
    And a buyer "b2" with a completed order of "L1"
    And "b1" has reviewed "L1" with rating 5 from their completed order
    And "b2" has reviewed "L1" with rating 1 from their completed order
    And an admin
    When the admin hides the review of "b2"
    Then the hide call succeeds
    And a "ListingRatingChanged" envelope for "L1" with review count 1 and average 5.0 appears on engagement.events after the "ReviewHidden" of the review of "b2"
    And GetListingRatingSummary of "L1" returns review count 1 and average 5.0

  Scenario: A rejected review publishes no rating event
    Given a seller with a published listing "L1"
    And a buyer "b1" with a completed order of "L1"
    And a buyer "b2"
    And "b1" has reviewed "L1" with rating 4 from their completed order
    And the rating event of "L1" with review count 1 and average 4.0 has reached engagement.events
    When "b2" reviews "L1" with rating 3 from the completed order of "b1"
    Then the review call fails with "failed_precondition"
    And GetListingRatingSummary of "L1" returns review count 1 and average 4.0
    And no further "ListingRatingChanged" envelope for "L1" appears on engagement.events within 10 seconds

  Scenario: An unauthorised hide publishes no rating event
    Given a seller with a published listing "L1"
    And a buyer "b1" with a completed order of "L1"
    And "b1" has reviewed "L1" with rating 5 from their completed order
    And the rating event of "L1" with review count 1 and average 5.0 has reached engagement.events
    When "b1" hides their own review
    Then the hide call is refused with HTTP 403 and "permission_denied"
    And GetListingRatingSummary of "L1" returns review count 1 and average 5.0
    And no further "ListingRatingChanged" envelope for "L1" appears on engagement.events within 10 seconds

  # @destructive: stops redpanda (shared by every service); restored in cleanup. Serial lane only.
  @destructive
  Scenario: A rating event written while Kafka is down is published later
    Given a seller with a published listing "L1"
    And a buyer "b1" with a completed order of "L1"
    When Redpanda is stopped, "b1" reviews "L1" with rating 4 from their completed order, and Redpanda is started again
    Then exactly one "ListingRatingChanged" envelope for "L1" with review count 1 and average 4.0 appears on engagement.events
    And the outbox row of that rating event of "L1" is published and was never parked

  Scenario: Backfill reaches the topic
    Given a seller with a published listing "L1"
    And listing "L1" has 2 verified review rows seeded directly in the database
    When migration 0015 is applied to the engagement database
    Then exactly one "ListingRatingChanged" envelope for "L1" with review count 2 and average 2.5 appears on engagement.events
    And GetListingRatingSummary of "L1" returns review count 2 and average 2.5
