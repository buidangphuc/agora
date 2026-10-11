@buyer
Feature: The storefront sends one idempotency key per checkout attempt
  The checkout page generates a fresh Idempotency-Key per attempt, reuses it for a
  resubmission of the same attempt and starts a new one after a completed checkout. Observed
  through the UI and the buyer's orders and the listing's stock at the gateway.

  Scenario: A replayed checkout submission creates no second order
    Given a logged-in buyer with a saved address and one unit of a listing with stock 10 in the cart
    When the buyer opens the checkout page
    And the buyer goes to the "confirm" checkout step
    And the buyer places the order on the checkout page recording the checkout submission
    And the buyer's cart holds the listing again
    And the recorded checkout submission is sent a second time
    Then the buyer has exactly one new order and the listing's stock is reduced once

  Scenario: A new checkout after a completed one creates a new order
    Given a logged-in buyer with a saved address and one unit of a listing with stock 10 in the cart
    When the buyer opens the checkout page
    And the buyer goes to the "confirm" checkout step
    And the buyer places the order on the checkout page recording the checkout submission
    And the buyer adds the listing to the cart again and places a second order from the checkout page
    Then the buyer has two distinct new orders
