@engagement
Feature: Reviews are tied to a completed order and moderated by admins
  A review needs the caller's COMPLETED order, one review per order and listing, only verified
  non-hidden reviews feed the star ratings, and an admin can hide a review with an audited
  reason. Everything runs through the gateway; legacy rows are seeded by direct SQL.
  Scenario names echo the spec scenario titles (engagement-review-integrity, review-integrity
  and review-moderation capabilities) so spec_sync can match them.

  Scenario: Review from a completed order, end to end
    Given a seller with a published listing "L1"
    And a buyer "b1" with a completed order of "L1"
    When "b1" reviews "L1" with rating 5 from their completed order
    Then the listing "L1" lists 1 review that is a verified purchase
    And the summary of "L1" shows an average of 5.0 over 1 review

  Scenario: Second review for the same order is refused, end to end
    Given a seller with a published listing "L1"
    And a buyer "b1" with a completed order of "L1"
    And "b1" has reviewed "L1" with rating 5 from their completed order
    When "b1" reviews "L1" with rating 4 from their completed order
    Then the review call fails with "already_exists"
    And the listing "L1" lists 1 review that is a verified purchase
    And the summary of "L1" shows an average of 5.0 over 1 review

  Scenario: Review without an order is refused, end to end
    Given a seller with a published listing "L1"
    And a buyer "b1"
    When "b1" reviews "L1" with rating 5 and an empty order id
    Then the review call fails with "invalid_argument"
    And the listing "L1" lists 0 reviews

  Scenario: Review of an order that is not completed is refused, end to end
    Given a seller with a published listing "L1"
    And a buyer "b1" with a pending order of "L1"
    When "b1" reviews "L1" with rating 5 from their pending order
    Then the review call fails with "failed_precondition"
    And the listing "L1" lists 0 reviews

  Scenario: Another buyer's completed order is refused as ineligible, end to end
    Given a seller with a published listing "L1"
    And a buyer "b1" with a completed order of "L1"
    And a buyer "b2"
    When "b2" reviews "L1" with rating 5 from the completed order of "b1"
    Then the review call fails with "failed_precondition"
    And the listing "L1" lists 0 reviews

  Scenario: Summary reflects verified reviews, end to end
    Given a seller with a published listing "L1"
    And a buyer "b1" with a completed order of "L1"
    And a buyer "b2" with a completed order of "L1"
    When "b1" reviews "L1" with rating 5 from their completed order
    And "b2" reviews "L1" with rating 3 from their completed order
    And a legacy unverified review of 1 star exists for "L1"
    Then the summary of "L1" shows an average of 4.0 over 2 reviews

  Scenario: Legacy unverified review is listed but excluded from stars, end to end
    Given a seller with a published listing "L1"
    And a buyer "b1" with a completed order of "L1"
    And "b1" has reviewed "L1" with rating 5 from their completed order
    And a legacy unverified review of 1 star exists for "L1"
    Then the listing "L1" lists 2 reviews of which 1 is not a verified purchase
    And the summary of "L1" shows an average of 5.0 over 1 review

  Scenario: Hide a review end to end
    Given a seller with a published listing "L1"
    And a buyer "b1" with a completed order of "L1"
    And a buyer "b2" with a completed order of "L1"
    And a buyer "b3" with a completed order of "L1"
    And "b1" has reviewed "L1" with rating 5 from their completed order
    And "b2" has reviewed "L1" with rating 3 from their completed order
    And "b3" has reviewed "L1" with rating 1 from their completed order
    And an admin
    When the admin hides the review of "b3" with the reason "spam link"
    Then the hide call succeeds
    And the listing "L1" lists 2 reviews that are all verified purchases
    And the summary of "L1" shows an average of 4.0 over 2 reviews
    And the audit log holds 1 "review.hide" event for the review of "b3" by the admin with the reason "spam link"

  Scenario: Re-hide writes no second audit event, end to end
    Given a seller with a published listing "L1"
    And a buyer "b1" with a completed order of "L1"
    And "b1" has reviewed "L1" with rating 2 from their completed order
    And an admin
    And the admin hides the review of "b1" with the reason "spam link"
    When the admin hides the review of "b1" with the reason "spam link"
    Then the hide call succeeds
    And the audit log holds 1 "review.hide" event for the review of "b1" by the admin with the reason "spam link"

  Scenario: Buyer cannot hide a review, end to end
    Given a seller with a published listing "L1"
    And a buyer "b1" with a completed order of "L1"
    And "b1" has reviewed "L1" with rating 5 from their completed order
    When "b1" hides their own review with the reason "changed my mind"
    Then the hide call fails with "permission_denied"
    And the listing "L1" lists 1 review that is a verified purchase

  Scenario: Anonymous caller cannot hide a review, end to end
    Given a seller with a published listing "L1"
    And a buyer "b1" with a completed order of "L1"
    And "b1" has reviewed "L1" with rating 5 from their completed order
    When an anonymous client hides the review of "b1" with the reason "changed my mind"
    Then the hide call fails with "unauthenticated"
    And the listing "L1" lists 1 review that is a verified purchase
