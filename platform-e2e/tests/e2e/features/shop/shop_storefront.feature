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

  # ui-phase-product-detail: the shared header card, ?sort= price order and follow.
  @needsSeller
  Scenario: The storefront shows the shared header card, a URL price sort and the follow toggle
    Given a buyer is logged in
    And the seller sets the shop display name to "Tiem Hoa Mai"
    And the seller has published listings priced 900000 and 300000
    When the buyer opens the seeded seller's shop page
    Then the shared shop header card shows "Tiem Hoa Mai"
    When the buyer sorts the shop by "Giá: Thấp đến Cao"
    Then the URL contains sort=price_asc and the cheaper listing comes first
    And reloading the shop keeps the ascending price order
    And the buyer can toggle following the shop
