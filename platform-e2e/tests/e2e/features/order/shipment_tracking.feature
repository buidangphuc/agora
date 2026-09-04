@buyer @needsBuyer @needsListing @needsOrder
Feature: SPX shipment tracking
  As a buyer, my order generates a trackable SPX delivery timeline.

  Scenario: Order shipment generates trackable timeline
    Given a buyer has a pending order
    When the order is shipped
    Then the order detail shows the SPX tracking timeline
