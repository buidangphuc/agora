@buyer @destructive
Feature: Cart failure path with a real service outage (ui-phase-cart-checkout)
  The scenario genuinely stops team-order (it backs the cart read) and restores it in teardown.
  Destructive: serial lane only.

  Scenario: Gateway failure on the cart
    Given a d2 buyer has a cart with one item priced 300000 and stock 50
    When team-order is stopped to force a real failure
    And the d2 buyer opens the cart
    Then the segment error alert "Không thể tải giỏ hàng" offers a retry button
    When team-order is running again and answers through the gateway
    And the buyer presses the retry button of the error
    Then the cart is rendered again with its item
