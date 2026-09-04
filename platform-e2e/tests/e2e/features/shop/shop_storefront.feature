@shop
Feature: Public Shop Storefront
  As a buyer
  I want to view a seller's dedicated shop storefront and products
  So that I can follow the store and browse all their merchandise

  @needsListing
  Scenario: Buyer explores public seller shop storefront
    Given a buyer is logged in
    When the buyer opens a seller shop page
    Then the shop profile header displays the store rating and product catalog
    And the buyer can toggle following the shop
