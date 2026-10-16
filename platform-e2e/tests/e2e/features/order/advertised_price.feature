@order @promo
Feature: A buyer is charged the price advertised to them
  team-order prices a cart line at the active flash price, refuses a checkout whose price changed
  (failed_precondition "price changed") after re-pricing the cart, holds and commits flash stock
  while the order is placed, and returns the units when the order is cancelled. Everything goes
  through the gateway; campaigns are seeded as the owning seller and a sale ends by moving its
  ends_at into the past, so no scenario flips shared feature flags.
  (openspec change order-advertised-price, initiative q5-advertised-price; scenario names echo the
  spec titles, the unit-level scenarios are not-testable entries in team-order/FEATURES.yaml)

  Scenario: Advertised price: Flash price is visible end to end after add-to-cart
    Given a seller's listing priced 500000 with stock 20
    And an active flash campaign on the listing with sale price 350000 and stock cap 10
    And a buyer with a saved address
    When the buyer adds 2 of the listing to the cart
    Then the buyer's cart line has unit price 350000 and the cart subtotal is 700000

  Scenario: Advertised price: Ended sale is refused end to end and the retry is charged the regular price
    Given a seller's listing priced 500000 with stock 20
    And an active flash campaign on the listing with sale price 350000 and stock cap 10
    And a buyer with a saved address
    And the buyer has 1 of the listing in the cart
    When the flash campaign ends
    And the buyer places the order
    Then the checkout is refused with failed_precondition "price changed"
    And the buyer's cart line has unit price 500000
    And the buyer has no orders
    When the buyer places the order
    Then the buyer's order item has unit price 500000

  Scenario: Advertised price: Concurrent buyers cannot exceed the campaign cap end to end
    Given a seller's listing priced 500000 with stock 20
    And an active flash campaign on the listing with sale price 350000 and stock cap 4
    And a buyer "b1" with a saved address and 3 of the listing in the cart
    And a buyer "b2" with a saved address and 3 of the listing in the cart
    When "b1" and "b2" place their orders at the same time
    Then exactly one order is created and its item has unit price 350000
    And the other buyer's checkout is refused with failed_precondition "price changed"

  Scenario: Advertised price: Placing a flash-priced order grows stock_sold end to end
    Given a seller's listing priced 500000 with stock 20
    And an active flash campaign on the listing with sale price 350000 and stock cap 10
    And a buyer with a saved address
    And the buyer has 2 of the listing in the cart
    When the buyer places the order
    Then the buyer's order item has unit price 350000
    And the campaign's stock_sold is 2 higher without a payment

  Scenario: Advertised price: Cancelled placed order returns flash units end to end
    Given a seller's listing priced 500000 with stock 20
    And an active flash campaign on the listing with sale price 350000 and stock cap 2
    And a buyer "b1" with a saved address and 2 of the listing in the cart
    And a buyer "b2" with a saved address and 2 of the listing in the cart
    When "b1" places the order
    Then the campaign's stock_sold is 2 higher
    When "b1" cancels the order
    Then the campaign's stock_sold decreases by 2 and the flash stock remaining is 2
    When "b2" places the order
    Then the order of "b2" has item unit price 350000

  Scenario: Advertised price: A buyer token cannot reserve flash stock end to end
    Given a seller's listing priced 500000 with stock 20
    And an active flash campaign on the listing with sale price 350000 and stock cap 5
    And a buyer with a saved address
    When the buyer calls ReserveFlashSaleStock directly through the gateway
    Then the call is refused with permission_denied, not_found or unimplemented
    And the flash stock remaining is 5
