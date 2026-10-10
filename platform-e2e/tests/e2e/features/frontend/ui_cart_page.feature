@buyer
Feature: Cart page - shop groups, mutations and the voucher modal
  OpenSpec change ui-phase-cart-checkout, cart page. Preconditions are seeded through the
  gateway by the principal the browser is logged in as; no real payment data is involved.

  Scenario: Items from two shops render as two groups
    Given a d2 buyer has a cart with one item from each of the shops "Shop Alpha, Shop Beta"
    When the d2 buyer opens the cart
    Then the cart shows one group per shop, each with only its own item, and the subtotal is their sum

  Scenario: Shop header shows the real shop name
    Given a d2 buyer has a cart with one item from each of the shops "Cửa hàng Hoa Mai"
    When the d2 buyer opens the cart
    Then the group header shows "Cửa hàng Hoa Mai" linking to the shop page and not "Shop #"

  Scenario: Empty shop name falls back to the short id
    Given a d2 buyer has a cart with one item from each of the shops "-"
    When the d2 buyer opens the cart
    Then the group header shows "Shop #" followed by the first 6 characters of the seller id

  Scenario: The cart shows no invented shop data
    Given a d2 buyer has a cart with one item from each of the shops "Shop Alpha, -"
    When the d2 buyer opens the cart
    Then every shop header holds only the shop name

  Scenario: Cart page is not a client component
    Given a d2 buyer has a cart with one item from each of the shops "Shop Alpha"
    When the d2 buyer opens the cart
    Then the cart page source has no "use client" directive
    And the cart HTML response already contains the item rows

  Scenario: Increasing quantity shows pending and then the new total
    Given a d2 buyer has a cart with one item priced 300000 and stock 50
    When the d2 buyer opens the cart
    And the d2 buyer increases the quantity watching for pending state
    Then the quantity, line total and subtotal show the server values for quantity 2

  Scenario: A failed quantity update is reported and not applied
    Given a d2 buyer has a cart with one item priced 300000 and stock 50
    When the d2 buyer opens the cart
    And the cart is emptied from another session
    And the d2 buyer increases the quantity of the stale row
    Then an error toast gives the reason and the quantity is back to its server value

  Scenario: Clearing the cart shows the empty state
    Given a d2 buyer has a cart with one item priced 300000 and stock 50
    When the d2 buyer opens the cart
    And the d2 buyer clears the cart watching for pending state
    Then an info toast is shown and the cart shows the empty state with a continue shopping link

  Scenario: Quantity cannot go below the minimum
    Given a d2 buyer has a cart with one item priced 300000 and stock 50
    When the d2 buyer opens the cart
    Then the decrease control is disabled

  Scenario: Valid code applies a discount
    Given a d2 buyer has a cart with one item priced 300000 and stock 50
    When the d2 buyer opens the cart
    And the d2 buyer opens the voucher modal
    And the d2 buyer applies the code "SAVE10" in the voucher modal
    Then the modal closes, a success toast shows and the discount row and total reflect the server amount

  Scenario: Invalid code is rejected with a reason
    Given a d2 buyer has a cart with one item priced 300000 and stock 50
    When the d2 buyer opens the cart
    And the d2 buyer opens the voucher modal
    And the d2 buyer applies the code "BOGUS-NOPE-999" in the voucher modal
    Then the form item shows the server reason, an error toast shows, no discount applies and the modal stays open

  Scenario: Voucher modal traps focus and closes with Escape
    Given a d2 buyer has a cart with one item priced 300000 and stock 50
    When the d2 buyer opens the cart
    And the d2 buyer opens the voucher modal
    And the d2 buyer presses Tab 10 times in the voucher modal
    Then focus is still inside the voucher modal
    When the d2 buyer presses Escape
    Then the voucher modal is closed and focus is back on the voucher selector

  Scenario: No available vouchers
    Given a d2 buyer has a cart with one item priced 300000 and stock 50
    When the d2 buyer opens the cart
    And the d2 buyer opens the voucher modal
    Then the voucher modal shows the empty state and keeps the manual code input usable
