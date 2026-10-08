@search @buyer
Feature: Search read-model deletes
  As a buyer
  I want a deleted listing to stay out of every search read path
  So that I never open a listing the seller removed

  Scenario: A stale listing update after delete does not bring it back
    Given a seller's published listing that appears in search next to a control listing
    When the seller deletes it through the gateway and a stale ListingChanged UPDATED published for it with an occurred_at before the delete is published to listing.events
    Then after the search indexer has consumed past that event, neither SearchListings for its title nor Suggest for its title prefix returns it

  Scenario: A redelivered create after delete does not bring it back
    Given a seller's published listing that appears in search next to a control listing
    And that listing is deleted through the gateway
    When the original ListingChanged CREATED record for it is read from listing.events and published again unchanged
    Then after the search indexer has consumed past the copy, SearchListings for its title does not return it

  Scenario: A newer status event does not revive a deleted listing
    Given a seller's published listing that appears in search next to a control listing
    And that listing is deleted through the gateway
    When a ListingStatusChanged with status PUBLISHED and an occurred_at after the delete is published to listing.events
    Then after the search indexer has consumed past that event, SearchListings for its title does not return it

  Scenario: A stock event for a deleted listing is ignored
    Given a seller's published listing that appears in search next to a control listing
    And that listing is deleted through the gateway
    When a valid ListingStockChanged with stock 5 and an occurred_at after the delete is published to listing.events
    Then after the search indexer has consumed past that event, an in-stock search for its title does not return it and the record is not on listing.events.dlq

  Scenario: A deleted listing leaves search results, totals and facets
    Given a seller with two published listings that a seller_id search counts as total 2 and a sellers facet of 2
    When the seller deletes one of them through the gateway
    Then within 30 seconds the same search returns total 1, only the remaining listing and a sellers facet of 1, and a saved search with that filter run by a buyer returns only the remaining listing

  Scenario: A deleted draft leaves the owner's draft view
    Given a seller's draft listing that appears in the owner's draft search next to a control draft
    When the seller deletes the draft through the gateway
    Then within 30 seconds that owner search no longer returns it, and a search with status deleted fails with invalid_argument

  # Needs the short-tombstone overlay platform-e2e/compose/search-tombstones.override.yaml
  # (TOMBSTONE_TTL=30s, TOMBSTONE_PURGE_INTERVAL=2s on team-search-indexer); the integrator runs
  # it in the serial lane.
  @destructive
  Scenario: An expired tombstone is purged
    Given the search indexer runs with a short tombstone TTL and purge interval
    And a seller with two published listings that a seller_id search counts as total 2 and a sellers facet of 2
    When the seller deletes one of them through the gateway
    Then the read-model holds a tombstone right after the delete is consumed, no document once the TTL plus one purge interval has passed, and the other listing still appears in search
