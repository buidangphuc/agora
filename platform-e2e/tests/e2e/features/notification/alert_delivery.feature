@buyer
Feature: Alert notification delivery
  As a subscribed buyer, when the seller lowers the price or restocks the item,
  I receive the corresponding notification in my notifications center.

  # Drives the real async flow: team-notification diffs each ListingChanged
  # snapshot against the last-seen price/stock, so events must arrive in order.

  Scenario: Price-drop notification after the seller lowers the price
    Given a buyer subscribed to a "price_drop" alert on a seeded listing
    When the seller lowers the listing price
    Then a "price_drop" notification appears in the notifications center

  Scenario: Back-in-stock notification after the seller restocks
    Given a buyer subscribed to a "back_in_stock" alert on a seeded listing
    When the seller restocks the out-of-stock listing
    Then a "back_in_stock" notification appears in the notifications center

  Scenario: Two buyers do not see each other's notifications
    Given two buyers where only the first is subscribed to a "price_drop" alert on a listing
    When the seller lowers the listing price
    Then only the first buyer receives a "price_drop" notification for that listing
