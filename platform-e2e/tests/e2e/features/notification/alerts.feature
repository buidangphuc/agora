@buyer
Feature: Price-drop and back-in-stock alert subscriptions
  As a buyer, I can subscribe to price-drop and back-in-stock alerts on a
  listing and manage those subscriptions from the notifications center.

  Scenario: Buyer subscribes to a price-drop alert and then cancels it
    Given a buyer viewing a seeded listing they want alerts for
    When the buyer enables the "price_drop" alert on the listing
    Then the "price_drop" alert toggle is active
    And the notifications center lists a "price_drop" alert subscription
    When the buyer cancels the "price_drop" alert subscription
    Then the notifications center lists no "price_drop" alert subscription for that listing

  Scenario: Buyer subscribes to a back-in-stock alert
    Given a buyer viewing a seeded listing they want alerts for
    When the buyer enables the "back_in_stock" alert on the listing
    Then the "back_in_stock" alert toggle is active
    And the notifications center lists a "back_in_stock" alert subscription
