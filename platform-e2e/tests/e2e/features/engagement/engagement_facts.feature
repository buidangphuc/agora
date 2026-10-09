@engagement
Feature: Engagement state changes are published as facts
  Each favourite, follow and review state change is written to the outbox in the same transaction
  and published to engagement.events inside the standard EventEnvelope. A call that changes no
  state publishes nothing. (engagement-fact-events / engagement-facts)

  Scenario: Favouriting a listing publishes a fact
    Given a seller with 1 published listing "L1"
    And a buyer "b1"
    When "b1" adds "L1" to their favourites through the gateway
    Then one "FavoriteAdded" envelope for "b1" and "L1" appears on engagement.events

  Scenario: Removing a favourite publishes a removal
    Given a seller with 1 published listing "L1"
    And a buyer "b1"
    And "b1" adds "L1" to their favourites through the gateway
    When "b1" removes "L1" from their favourites through the gateway
    Then a "FavoriteRemoved" envelope for "b1" and "L1" follows the "FavoriteAdded" on engagement.events

  Scenario: A repeated favourite publishes nothing new
    Given a seller with 1 published listing "L1"
    And a buyer "b1"
    When "b1" adds "L1" to their favourites through the gateway
    And "b1" adds "L1" to their favourites through the gateway
    Then exactly one "FavoriteAdded" envelope for "b1" and "L1" appears on engagement.events

  Scenario: A review publishes its rating without its text
    Given a seller with 1 published listing "L1"
    And a buyer "b1" with a delivered order of "L1"
    When "b1" reviews "L1" with rating 4 and the text "rất tốt"
    Then a "ReviewCreated" envelope with rating 4, "L1" and its seller appears on engagement.events
    And the "ReviewCreated" payload does not contain "rất tốt"

  # @destructive: stops redpanda (shared by every service); restored in cleanup. Serial lane only.
  @destructive
  Scenario: A fact written while Kafka is down is published later
    Given a seller with 1 published listing "L1"
    And a buyer "b1"
    When Redpanda is stopped, "b1" follows the seller, and Redpanda is started again
    Then one "SellerFollowed" envelope for "b1" and the seller appears on engagement.events
