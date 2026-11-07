@recsys @recommendations @destructive
Feature: Cart cross-sell recommendations (recsys-cart-cross-sell)
  The cart_cross_sell placement uses the cart lines as context (`context_listing_ids`, at most 50, capped by
  the gateway), falls back to the popular listings of the relevant category and then to global popular, and
  applies the placement filters. Every candidate of the scenarios is seeded with `visible=true`, directly in
  the serving item collection and popular list; everything else is hidden and restored in teardown.

  Scenario: Similar listings for cart lines
    Given a buyer is logged in
    And a cross-sell catalogue where cart listings A and B each have ten similar listings from ten different sellers
    When Recommend for cart_cross_sell is called through the gateway with the context listings A and B
    Then the response holds at most 8 items in descending score
    And every item is a neighbour of the context listings
    And every item score is its maximum similarity to the context listings

  Scenario: Empty context ends at global popular
    Given a buyer is logged in
    And the global popular list holds one hidden listing with the top score and three visible listings of three sellers
    When Recommend for cart_cross_sell is called through the gateway with no context listings and no seed
    Then every item is a visible global popular listing and the hidden one is absent

  Scenario: Exactly 50 context ids are served
    Given a buyer is logged in
    And a cross-sell catalogue where cart listings A and B each have ten similar listings from ten different sellers
    When Recommend for cart_cross_sell is called through the gateway with the context listing A and 49 listings unknown to the index
    Then the response holds between 1 and 8 items
    And every item is a neighbour of the context listings

  Scenario: Unknown context listing is skipped
    Given a buyer is logged in
    And a cross-sell catalogue where cart listings A and B each have ten similar listings from ten different sellers
    When Recommend for cart_cross_sell is called through the gateway with the context listing A and one listing unknown to the index
    Then every item is a neighbour of the context listings

  Scenario: Gateway rejects 51 ids
    Given a buyer is logged in
    When Recommend for cart_cross_sell is called through the gateway with 51 context listings
    Then the gateway answers INVALID_ARGUMENT
    And team-ai served no Recommend call for it

  Scenario: Gateway forwards context ids
    Given a buyer is logged in
    And a cross-sell catalogue where cart listings A and B each have ten similar listings from ten different sellers
    When Recommend for cart_cross_sell is called through the gateway with the context listings A and B
    Then every item is a neighbour of the context listings
    And the response contains none of the context listings

  Scenario: Seed category
    Given a buyer is logged in
    And a cross-sell catalogue where cart listings A and B each have ten similar listings from ten different sellers
    When Recommend for cart_cross_sell is called through the gateway with the seed listing A and no context listings
    Then every item is a global popular listing of category C

  Scenario: Cart categories
    Given a buyer is logged in
    And a cross-sell catalogue where cart listings A and B each have ten similar listings from ten different sellers
    And every listing similar to A or B is hidden
    When Recommend for cart_cross_sell is called through the gateway with the context listings A and B whose similar listings are all hidden
    Then every item is a global popular listing of category C or D

  Scenario: Cart items and out-of-stock excluded
    Given a buyer is logged in
    And a cross-sell catalogue where cart listings A and B each have ten similar listings from ten different sellers
    And the most similar listing of A is a cart line and the second most similar is out of stock
    When Recommend for cart_cross_sell is called through the gateway with the context listings A and its most similar listing
    Then every item is a neighbour of the context listings
    And the response contains none of the context listings
    And the out-of-stock listing is not in the response

  Scenario: One listing per seller
    Given a buyer is logged in
    And a cross-sell catalogue where cart listings A and B each have ten similar listings from ten different sellers
    And the three most similar listings of A belong to one seller
    When Recommend for cart_cross_sell is called through the gateway with the context listing A
    Then exactly one of those three listings is in the response and it is the most similar one

  Scenario: Seed excluded when enabled
    Given a buyer is logged in
    And a cross-sell catalogue where cart listings A and B each have ten similar listings from ten different sellers
    When Recommend for similar_items is called through the gateway with the seed listing A
    Then the seed listing is not in the response

  Scenario: Existing placements still resolve
    Given a buyer is logged in
    And a cross-sell catalogue where cart listings A and B each have ten similar listings from ten different sellers
    When Recommend is called through the gateway for home_feed, similar_items and an undefined placement
    Then each of the three placements answers OK with items and names its own placement
