@buyer
Feature: Cart and checkout UI
  As a buyer
  I want a cart grouped by shop and a guided, double-submit-safe checkout
  So that I can review and pay for my order with confidence

  # Extends buyer/cart_management.feature (quantity, clear), buyer/purchase.feature
  # (reach checkout), promo/vouchers.feature (voucher redemption), payment/mock_pay.feature
  # and order/saga_compensation.feature; none of their scenarios is repeated here.

  @needsSeller
  Scenario: Cart groups items by shop with the real shop name
    Given the seller sets the shop display name to "Shop Alpha"
    And a second seller sets the shop display name to "Shop Beta"
    And a buyer has one listing from each seller in the cart
    When the buyer opens the cart with the seeded session
    Then the cart shows two shop groups headed "Shop Alpha" and "Shop Beta"

  @needsListing
  Scenario: Clearing the cart offers a way back to shopping
    Given a buyer is logged in
    And a listing has been seeded via the API
    When the buyer opens the seeded listing
    And the buyer adds the product to the cart
    And the buyer opens the cart
    And the buyer clears all items from the cart
    Then the cart displays the empty state
    And the empty cart offers a link to continue shopping

  Scenario: Step state lives in the URL
    Given a promotion buyer has a qualifying cart and a saved address
    When the buyer opens the checkout page
    Then the checkout page shows the stepper without the global search
    When the buyer goes to the "shipping" checkout step
    Then the checkout URL shows step "shipping" and the selected address
    When the buyer reloads the checkout page
    Then the "shipping" step is the current step

  Scenario: Changing the address updates the URL
    Given a promotion buyer has a qualifying cart and a saved address
    And the buyer has a second saved address for "Tran Thi B"
    When the buyer opens the checkout page
    And the buyer changes the delivery address to "Tran Thi B"
    Then the address card shows "Tran Thi B" and the URL carries that address

  Scenario: Payment method is one-of-many and keyboard operable
    Given a promotion buyer has a qualifying cart and a saved address
    When the buyer opens the checkout page
    And the buyer goes to the "payment" checkout step
    And the buyer presses the Down arrow on the selected payment method
    Then exactly one payment method is selected and the URL pay parameter follows it

  Scenario: Rapid double click creates one order
    Given a promotion buyer has a qualifying cart and a saved address
    When the buyer opens the checkout page
    And the buyer goes to the "confirm" checkout step
    And the buyer double-clicks the place order button
    Then exactly one order exists for the buyer

  Scenario: Out-of-stock failure shows a recoverable error
    Given a promotion buyer has a qualifying cart and a saved address
    And another buyer exhausts the listing stock
    When the buyer opens the checkout page
    And the buyer goes to the "confirm" checkout step
    And the buyer places the order in the browser
    Then an error alert with retry and back-to-cart actions is shown
    And the place order button is enabled again and did not move
    And the cart still holds the same item

  @needsListing @needsOrder
  Scenario: Successful mock payment shows a success result
    Given a buyer has an order awaiting mock payment
    When the buyer opens the payment page for the order
    And the buyer presses "Thanh toán thành công" on the payment page
    Then a success result links to the order list and to continue shopping

  @needsListing @needsOrder
  Scenario: Failed mock payment shows an error result with recovery
    Given a buyer has an order awaiting mock payment
    When the buyer opens the payment page for the order
    And the buyer presses "Thanh toán thất bại" on the payment page
    Then an error result offers retry and change of payment method

  Scenario: Mobile cart fits the viewport
    Given a promotion buyer has a qualifying cart and a saved address
    When the buyer opens the cart at 375px width
    Then the cart has no horizontal overflow and the quantity control and buy button are visible

  Scenario: The purchase path still emits begin_checkout, apply_promotion and purchase
    Given a promotion buyer has a qualifying cart and a saved address
    When the buyer applies the voucher code "SAVE10" at checkout
    Then the voucher discount is shown and the order total is reduced
    And placing the order creates the order
    And the page analytics dataLayer holds exactly 1 "begin_checkout" event
    And the page analytics dataLayer holds at least 1 "apply_promotion" event
    And the page analytics dataLayer holds exactly 1 "purchase" event
