@buyer
Feature: Checkout - placing an order is guarded against double submit
  OpenSpec change ui-phase-cart-checkout. A slow server is simulated in the browser by delaying
  the Next server-action POST; the order itself is created by the real stack (COD).

  Scenario: Button is disabled while pending
    Given a d2 buyer has a cart with one item priced 300000 and stock 50
    When the buyer opens the checkout page
    And the buyer goes to the "confirm" checkout step
    And the buyer places the order while the server is slow
    Then the place order button is disabled, busy and keeps its width, and back and stepper navigation are inert

  Scenario: Enter key cannot bypass the guard
    Given a d2 buyer has a cart with one item priced 300000 and stock 50
    When the buyer opens the checkout page
    And the buyer goes to the "confirm" checkout step
    And the buyer places the order while the server is slow
    And the buyer presses Enter in the confirm form while the order is pending
    Then only one order action was sent and exactly one order exists

  Scenario: Button stays locked after success
    Given a d2 buyer has a cart with one item priced 300000 and stock 50
    When the buyer opens the checkout page
    And the buyer goes to the "confirm" checkout step
    And the buyer places the order while the next page is slow
    Then once the order is created the button stays disabled and no second order action can be sent

  Scenario: Button is re-enabled after failure
    Given a d2 buyer has a cart with one item priced 300000 and stock 5
    And another d2 buyer buys all the remaining stock of the listing
    When the buyer opens the checkout page
    And the buyer goes to the "confirm" checkout step
    And the buyer places the order in the browser
    Then the place order button is enabled again with an error alert and an error toast

  Scenario: purchase fires once
    Given a d2 buyer has a cart with one item priced 300000 and stock 50
    When the buyer opens the checkout page
    And the buyer goes to the "confirm" checkout step
    And the buyer places the order with a double click
    Then exactly one purchase event carries the id of the created order

  @destructive
  Scenario: Kill-switch shows an info Result
    Given a d2 buyer has a cart with one item priced 300000 and stock 50
    When the "checkout-enabled" flag is turned OFF for the d2 scenario
    And the buyer opens the checkout page directly
    Then the checkout page shows the unavailable Result with a back to cart button and no wizard

  @destructive
  Scenario: Checkout kill-switch disables the CTA
    Given a d2 buyer has a cart with one item priced 300000 and stock 50
    When the "checkout-enabled" flag is turned OFF for the d2 scenario
    And the d2 buyer opens the cart
    Then the buy button is disabled with aria-disabled and an alert explains checkout is unavailable
