@analytics
Feature: The warehouse stores engagement facts and their current state
  team-analytics consumes engagement.events into engagement_facts (one row per event_id) and
  derives favorites_current and follows_current. Undecodable records go to the DLQ.
  (engagement-fact-events / engagement-facts)

  Scenario: A favourite that was removed is not current
    Given a seller with 2 published listings
    And a buyer "b1"
    When "b1" adds "L1" and "L2" to their favourites and then removes "L1"
    Then engagement_facts holds three rows for "b1" and favorites_current lists only "L2" for "b1"

  Scenario: A follow appears in the current follows
    Given a seller with 1 published listing "L1"
    And a buyer "b1"
    When "b1" follows the seller
    Then follows_current lists "b1" and the seller

  Scenario: A malformed engagement record is dead-lettered
    Given a seller with 1 published listing "L1"
    And a buyer "b1"
    When a malformed record is produced to engagement.events
    And "b1" adds "L1" to their favourites through the gateway
    Then the malformed record appears on engagement.events.analytics.dlq
    And the favourite of "b1" and "L1" reaches engagement_facts
