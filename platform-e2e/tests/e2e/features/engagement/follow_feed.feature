@engagement @buyer
Feature: Follow feed from seller listings
  As a buyer who follows a seller, I see that seller's newly published listings
  in my follow feed, newest first, without refreshing anything by hand.

  # The listing reaches the feed asynchronously: team-domain emits
  # ListingChanged on listing.events and team-engagement consumes it into the
  # follow-feed source, so the feed assertions poll with a timeout.

  Scenario: A listing published by a followed seller appears in the buyer's feed
    Given a buyer who follows a seller
    When the seller publishes a listing
    Then the listing appears in the buyer's follow feed

  Scenario: The follow feed lists the newest listing first
    Given a buyer who follows a seller
    When the seller publishes a listing
    And the seller publishes a second listing
    Then the buyer's follow feed lists the second listing before the first

  Scenario: A deleted listing leaves the buyer's feed
    Given a buyer who follows a seller
    When the seller publishes a listing
    And the listing appears in the buyer's follow feed
    And the seller deletes the listing
    Then the listing leaves the buyer's follow feed

  Scenario: A seller the buyer does not follow never shows up in the feed
    Given a buyer who follows a seller
    When another seller publishes a listing
    And the seller publishes a listing
    And the listing appears in the buyer's follow feed
    Then the other seller's listing is not in the buyer's follow feed
