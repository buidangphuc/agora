@buyer
Feature: Checkout wizard - shell, URL state, address, shipping, payment and summary
  OpenSpec change ui-phase-cart-checkout, checkout wizard. Seeded through the gateway; the
  order paths use COD or the mock payment, never real payment data.

  Scenario: Shell hides consumer chrome
    Given a d2 buyer has a cart with one item priced 300000 and stock 50
    When the buyer opens the checkout page
    Then the checkout shell shows the logo link, the stepper and the secure footer only

  Scenario: Browser back returns to the previous step
    Given a d2 buyer has a cart with one item priced 300000 and stock 50
    When the buyer opens the checkout page
    And the buyer goes to the "payment" checkout step
    And the buyer uses the browser back button
    Then the shipping step is shown with the selections preserved

  Scenario: Skipping ahead is redirected
    Given a d2 buyer has a cart with one item priced 300000 and stock 50
    When the buyer opens the checkout URL "/checkout?step=confirm" directly
    Then the browser is redirected to step "address"

  Scenario: Empty cart and signed-out user are redirected
    Given a d2 buyer has an empty cart
    When the buyer opens the checkout page directly
    Then the browser lands on "/cart"
    When the visitor signs out and opens the checkout page
    Then the browser lands on "/login"

  Scenario: No saved address blocks the step
    Given a d2 buyer with the address city "-" has a cart with one item priced 300000
    When the buyer opens the checkout page
    Then the address step shows the empty state and a disabled continue action

  Scenario: Adding an address from the modal
    Given a d2 buyer with the address city "-" has a cart with one item priced 300000
    When the buyer opens the checkout page
    And the buyer adds the address for "Pham Thi Moi" from the empty state
    Then a success toast shows and the new address is listed and selected

  Scenario: Free shipping above the threshold
    Given a d2 buyer has a cart with one item priced 500000 and stock 50
    When the buyer opens the checkout page
    And the buyer goes to the "shipping" checkout step
    Then the shipping step shows the Freeship tag and the summary shipping row is free

  Scenario: Payment grid wraps on mobile
    Given a d2 buyer has a cart with one item priced 300000 and stock 50
    When the buyer sets the viewport to 375 by 812
    And the buyer opens the checkout page
    And the buyer goes to the "payment" checkout step
    Then the payment cards are one per row, fit the viewport and are at least 44px tall

  Scenario: Total reflects discount and shipping
    Given a d2 buyer with the address city "Hà Nội" has a cart with one item priced 300000
    When the buyer opens the checkout page
    And the buyer goes to the "payment" checkout step
    And the buyer applies the code "SAVE10" on the payment step
    Then the summary shows subtotal 300000, discount 30000, shipping 20000 and total 290000

  Scenario: Mobile summary bar and drawer
    Given a d2 buyer has a cart with one item priced 300000 and stock 50
    When the buyer sets the viewport to 375 by 812
    And the buyer opens the checkout page
    Then a sticky bottom bar shows the total and the primary action
    When the buyer taps "Chi tiết" in the bottom bar
    Then a drawer shows the full breakdown without horizontal scroll

  Scenario: Desktop shows a sticky summary
    Given a d2 buyer has a cart with one item priced 300000 and stock 50
    When the buyer sets the viewport to 1280 by 720
    And the buyer opens the checkout page
    And the buyer scrolls the page down
    Then the order summary is sticky and still visible in the right column

  Scenario: Applying a voucher does not shift the layout
    Given a d2 buyer has a cart with one item priced 300000 and stock 50
    When the buyer opens the checkout page
    And the buyer goes to the "payment" checkout step
    And the buyer applies the code "SAVE10" on the payment step
    Then the continue action and the total row did not move

  Scenario: begin_checkout fires once per entry
    Given a d2 buyer has a cart with one item priced 300000 and stock 50
    When the buyer moves through the four checkout steps
    Then exactly one "begin_checkout" event was pushed with the cart items and value

  Scenario: apply_promotion still fires for valid and invalid codes
    Given a d2 buyer has a cart with one item priced 300000 and stock 50
    When the buyer opens the checkout page
    And the buyer goes to the "payment" checkout step
    And the buyer previews the codes "SAVE10" and then "BOGUS-NOPE-999" on the payment step
    Then two "apply_promotion" events were pushed with valid "true" and then "false"
