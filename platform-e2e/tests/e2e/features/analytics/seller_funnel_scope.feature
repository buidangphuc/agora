@analytics @seller
Feature: Seller funnel tracking counts are scoped to the seller's own listings
  Tracking events carry only a listing id. team-analytics learns which seller owns
  each listing from listing.events and attributes tracking events through that
  mapping, so one seller's funnel never includes events on another seller's
  listings.

  Scenario: Events on seller A's listing do not appear in seller B's funnel
    Given two sellers exist
    And seller A has published a listing
    When 3 view beacons are collected for seller A's listing
    Then seller A's funnel reports 3 views
    And seller B's funnel reports no views
