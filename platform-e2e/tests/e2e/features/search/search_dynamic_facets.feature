@search @buyer
Feature: Search dynamic facets from classified tags
  As a buyer
  I want to filter by technical specs and variant options
  So that I only see listings with a variant I could actually buy

  Scenario: Listing Events Index SPU Tags
    Given a seller publishes a listing titled with Bluetooth 5.3, ANC and 65W GaN fast charging
    When the listing event has been indexed
    Then SearchListings with the filter tag.connectivity bluetooth-5-3 returns the listing, and the same search with tag.connectivity wifi-6 does not

  Scenario: SKU Filters Match One Variant, Not Across Variants
    Given a published listing with in-stock variants Titan Tự Nhiên 256GB and Xanh Navy 512GB, and a second one with Xanh Navy 256GB and Titan Tự Nhiên 512GB
    When a buyer searches with sku.color xanh-navy and sku.capacity 512gb
    Then only the first listing is returned

  Scenario: Sold-Out Variants Do Not Satisfy SKU Filters
    Given a published listing whose Xanh Navy 512GB variant has stock 0, and another with that variant in stock
    When a buyer searches with sku.color xanh-navy and sku.capacity 512gb
    Then only the listing whose matching variant is in stock is returned

  Scenario: Malformed Facet Filters Are Rejected
    When a buyer calls SearchListings with the filter sku.Colour Name x, then with tag.color den:other
    Then the call fails with invalid_argument and no search is run

  Scenario: Search Response Carries Dynamic Facets
    Given published listings with classified SPU tags and variants
    When a buyer searches for their keyword through the gateway
    Then facets.tags lists a connectivity group with the bluetooth-5-3 bucket and facets.skus lists color and capacity groups, each bucket counting listings
