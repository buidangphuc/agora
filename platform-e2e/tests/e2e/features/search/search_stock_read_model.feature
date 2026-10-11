@search @buyer
Feature: Search read-model stock
  As a buyer
  I want search to show each listing's current stock and let me restrict a search to listings in stock
  So that I do not chase a sold-out listing

  Scenario: A checkout lowers the stock shown in search
    Given a published listing with stock 10 that appears in search
    When a buyer checks out quantity 2 of it through the gateway
    Then within 30 seconds search returns that listing with stock 8

  Scenario: Cancelling the order restores the stock shown in search
    Given a published listing with stock 10 whose search stock is 8 after a real checkout of quantity 2
    When the buyer cancels the order through the gateway
    Then within 30 seconds search returns that listing with stock 10

  Scenario: A saved search run shows the current stock
    Given a buyer has saved a search whose query matches a listing with stock 10
    When another buyer checks out quantity 3 of that listing
    Then within 30 seconds RunSavedSearch returns that listing with stock 7

  Scenario: A stale stock event does not roll stock back
    Given a published listing with stock 10 whose search stock is 8 after a real checkout of quantity 2
    When a ListingStockChanged with stock 10 and an occurred_at earlier than that checkout event is published to listing.events
    Then after the search indexer has consumed past that event, search still returns that listing with stock 8

  Scenario: A stock event for an unknown listing creates nothing
    When a valid ListingStockChanged with stock 5 for a listing id that was never created is published to listing.events
    Then after the search indexer has consumed past that event, the read-model holds no document for that id and no search returns it

  Scenario: A malformed stock event is parked, not applied
    Given a published listing whose search stock is 10
    When a ListingStockChanged for that listing with stock -1 is published to listing.events
    Then within 60 seconds that record appears on listing.events.dlq and search still returns the listing with stock 10

  Scenario: A late listing update does not overwrite newer stock
    Given a published listing with stock 10 whose search stock is 8 after a real checkout of quantity 2
    When a ListingChanged UPDATED with a new title, stock 10 and an occurred_at between its creation and that checkout is published to listing.events
    Then within 30 seconds a search for the new title returns that listing with stock 8

  Scenario: A seller's stock edit is reflected in search
    Given a published listing with stock 10 whose search stock is 8 after a real checkout of quantity 2
    When the seller updates the listing's stock to 50 through the gateway
    Then within 30 seconds search returns that listing with stock 50

  Scenario: An in-stock search hides a sold-out listing
    Given a published listing with stock 1 that appears in search
    When a buyer checks out quantity 1 of it through the gateway
    Then within 30 seconds an in-stock search for that query no longer returns or counts it while the same search without the filter returns it with stock 0

  Scenario: The in-stock filter applies in semantic and hybrid search
    Given a sold-out listing and an in-stock listing that both match a query
    When a buyer runs that query with the in-stock filter in semantic mode, then in hybrid mode
    Then both responses return the in-stock listing and neither returns the sold-out one

  Scenario: A saved in-stock search excludes sold-out listings
    Given a sold-out listing and an in-stock listing that both match a query
    And a buyer has saved that query with filters_json in_stock true
    When the buyer runs the saved search through the gateway
    Then the response returns the in-stock listing with its stock and not the sold-out one

  Scenario: An invalid in_stock value is rejected
    When a buyer calls SearchListings with in_stock maybe, then SaveSearch with filters_json in_stock yes
    Then both calls fail with invalid_argument and no saved search is created
