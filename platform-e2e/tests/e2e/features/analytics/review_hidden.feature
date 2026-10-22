@analytics
Feature: The warehouse records a review_hidden fact beside the review_created fact
  team-analytics maps a ReviewHidden envelope on engagement.events to a review_hidden row in
  engagement_facts and keeps the review_created row, which now stores its review_id.
  (analytics-review-hidden)

  Scenario: Analytics review hidden: a produced ReviewHidden reaches the warehouse
    Given a ReviewCreated envelope for a new review of a new listing is produced to engagement.events
    And a ReviewHidden envelope for the same review is produced to engagement.events
    Then within 30 seconds engagement_facts holds one review_created row and one review_hidden row for that review
    And the review_hidden row has that listing id and a NULL rating
    And the review_created row keeps its rating and review id
    And the dead-letter topic holds no record for either event id
