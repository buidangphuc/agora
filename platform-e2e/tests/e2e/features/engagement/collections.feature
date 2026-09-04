@engagement @buyer
Feature: Wishlist collections
  As a buyer, I can group saved products into named collections so I can
  organise my wishlist and find things later.

  Scenario: Buyer creates a collection, adds a listing, views it, then removes it
    Given a buyer with a seeded listing to collect
    When the buyer creates a collection named "Đồ công nghệ"
    Then the collection appears in the buyer's collections
    When the buyer adds the listing to the collection "Đồ công nghệ"
    Then the collection shows the listing among its items
    When the buyer removes the listing from the collection "Đồ công nghệ"
    Then the collection has no items
