@buyer
Feature: Alert notification delivery
  As a subscribed buyer, when the seller lowers the price or restocks the item,
  I receive the corresponding notification in my notifications center.

  # NOTE: these two scenarios are marked xfail in the binding module. They drive
  # the real async flow, but it is currently broken in the backend: team-domain
  # emits platform.listing.v1.ListingChanged on listing.events, while
  # team-notification's consumer only reacts to ListingPricingChanged /
  # ListingStockChanged (which nothing produces), so no notification is created.
  # They self-heal (xpass) once the producer/consumer contract is reconciled.

  Scenario: Price-drop notification after the seller lowers the price
    Given a buyer subscribed to a "price_drop" alert on a seeded listing
    When the seller lowers the listing price
    Then a "price_drop" notification appears in the notifications center

  Scenario: Back-in-stock notification after the seller restocks
    Given a buyer subscribed to a "back_in_stock" alert on a seeded listing
    When the seller restocks the out-of-stock listing
    Then a "back_in_stock" notification appears in the notifications center
