@recsys @recommendations @destructive
Feature: Candidate payloads carry seller, category and visibility (recsys-candidate-attributes)
  The real recsys job publishes a generation whose item points and cache entries carry `visible`, and
  the cart cross-sell placement served through the gateway returns only candidates with `visible` true.
  The gateway-entry assertions need recsys-cart-cross-sell (placement `cart_cross_sell`,
  `context_listing_ids`).

  Scenario: Contract is visible end to end
    Given a buyer is logged in
    When the recsys job publishes a live generation whose attribute snapshot marks one similar listing published and another banned
    And Recommend for cart_cross_sell is called through the gateway with the cart context listing
    Then the response contains the published listing
    And the response does not contain the banned listing

  Scenario: Payload without visible is not eligible
    Given a buyer is logged in
    When the recsys job publishes a live generation whose attribute snapshot marks one similar listing published and leaves another with no status
    And Recommend for cart_cross_sell is called through the gateway with the cart context listing
    Then the response contains the published listing
    And the response does not contain the listing without visible

  Scenario: A refused run keeps serving the previous generation
    Given a buyer is logged in
    When the recsys job publishes a live generation whose attribute snapshot marks the similar listings published
    And a training run starts with no attribute snapshot
    And Recommend for cart_cross_sell is called through the gateway with the cart context listing
    Then the run exits 2 and trains nothing
    And the first generation is still the serving generation
    And the response contains the published listing
