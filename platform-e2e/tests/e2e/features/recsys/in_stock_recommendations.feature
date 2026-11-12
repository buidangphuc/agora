@recsys @recommendations @destructive
Feature: In-stock placements read live search stock (recs-in-stock-filter)
  Placements with `in_stock_only` (home_feed, similar_items, cart_cross_sell) keep a candidate only when its live
  search stock, read through team-search GetListingStock, is greater than 0. Every listing of the scenarios is a
  real published listing created through the gateway, so its stock reaches search through listing.events; the
  serving item collection and popular list are seeded with the listing ids and `in_stock=true`, and everything
  else is hidden and restored in teardown. A lookup that cannot be answered empties the placement (fail closed).

  Scenario: Sold-out listing is dropped from an in_stock_only placement
    Given a buyer is logged in
    And a seed listing and five neighbours, all published with stock 5 in search, each of a different seller and each with Qdrant in_stock true
    And the seller sells out one neighbour through the gateway and search shows its stock as 0
    When Recommend for similar_items is called through the gateway with the seed listing
    Then the sold-out listing is absent from the response
    And every returned listing has search stock greater than 0 and the four in-stock neighbours are returned

  Scenario: Stock sold out after indexing is dropped without a batch run
    Given a buyer is logged in
    And the global popular list holds four published listings of four sellers with stock 5 in search
    And Recommend for home_feed returns all four listings
    When the seller sells out one of them through the gateway and search shows its stock as 0
    And Recommend for home_feed is called again through the gateway
    Then the second response no longer contains that listing, still contains the other three and carries the same model version

  @destructive
  Scenario: Search unavailable empties the placement
    Given a buyer is logged in
    And a cart listing A and five neighbours, all published with stock 5 in search
    And Recommend for cart_cross_sell with the context listing A returns neighbours
    When team-search is stopped
    And Recommend for cart_cross_sell is called through the gateway with the context listing A
    Then the response is OK with zero items
    When team-search is started again
    Then a later Recommend for cart_cross_sell with the context listing A returns neighbours again

  @destructive
  Scenario: Lookup without a valid principal fails closed
    Given a buyer is logged in
    And a cart listing A and five neighbours, all published with stock 5 in search
    And Recommend for cart_cross_sell with the context listing A returns neighbours
    And a team-ai whose search principal lacks scope search:read serves a private gateway
    When Recommend for cart_cross_sell is called through the private gateway with the context listing A
    Then the response is OK with zero items
    And the stack's own gateway still returns neighbours for the same request
