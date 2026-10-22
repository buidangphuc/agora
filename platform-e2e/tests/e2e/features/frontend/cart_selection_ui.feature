@buyer @cart_selection
Feature: Cart selection in the buyer UI
  OpenSpec change frontend-cart-selection (initiative q11-checkout-coherence). The buyer ticks
  cart lines or whole shops and checks out only those; "Mua ngay" checks out only the line it
  added; another buyer's line id is never accepted. Shops, listings, buyers and carts are seeded
  through the gateway; the placed orders and the remaining cart are read back from the gateway.

  Scenario: Cart selection: checking out some lines leaves the others in the cart
    Given a cs buyer has 3 cart lines, two from shop A and one from shop B
    When the cs buyer opens the cart
    And the cs buyer ticks only the line "A1"
    Then the cs buy action links to the checkout of the lines "A1"
    When the cs buyer follows the buy action and places the order
    Then the cs buyer has exactly 1 order and it holds only the line "A1"
    And the cs cart afterwards lists the lines "A2, B1" only

  Scenario: Cart selection: a shop's select-all checks out that shop only
    Given a cs buyer has 3 cart lines, two from shop A and one from shop B
    When the cs buyer opens the cart
    And the cs buyer unticks the select-all of shop B
    Then the cs buy action links to the checkout of the lines "A1, A2"
    When the cs buyer follows the buy action and places the order
    Then every cs order of the buyer belongs to shop A and holds only the lines "A1, A2"
    And the cs cart afterwards lists the lines "B1" only

  Scenario: Cart selection: another buyer's line id places nothing
    Given a cs buyer has a cart line and another cs buyer has a cart line
    When the cs buyer opens the checkout of their own line
    Then the cs browser stays on the checkout of that line and its back link keeps the selection
    When the cs buyer opens the checkout of the other buyer's line
    Then the cs browser is redirected to the cart
    And the cs buyer has no order and the other buyer's cart is unchanged

  Scenario: Cart selection: an anonymous visitor is sent to login
    Given a cs buyer has 2 cart lines from two shops
    When the cs buyer opens the checkout of the line "A1"
    Then the cs checkout lists the line "A1" and not the line "B1"
    And the cs back link to the cart keeps the line "A1"
    When the cs visitor is anonymous and opens the checkout of the line "A1"
    Then the cs browser is sent to login with the return path "/checkout?items=" and the line "A1"

  Scenario: Cart selection: the cart first response still holds the line rows
    Given a cs buyer has 3 cart lines, two from shop A and one from shop B
    When the cs buyer requests the cart page
    Then the cs first HTML response holds the 3 line rows and a checkbox per line and per shop
    And the cs cart page source has no "use client" directive

  Scenario: Cart selection: Mua ngay checks out only that line end to end
    Given a cs buyer has 2 cart lines from two shops
    And a cs listing of a third shop that is not in the cart
    When the cs buyer opens that listing and presses Mua ngay
    Then the cs browser opens the checkout of the new line only
    When the cs buyer places the order from the confirm step
    Then the cs buyer has exactly 1 order and it holds only the new line
    And the cs cart afterwards lists the lines "A1, B1" only

  Scenario: Cart selection: Mua ngay on an existing line checks out the merged quantity
    Given a cs buyer has 2 cart lines from two shops
    And the cs cart already holds the third shop's listing with quantity 2
    When the cs buyer opens that listing and presses Mua ngay
    Then the cs browser opens the checkout of the merged line only
    And the cs checkout lists the merged line and not the line "A1"
    When the cs buyer places the order from the confirm step
    Then the cs buyer has exactly 1 order and it holds the merged line with quantity 3
    And the cs cart afterwards lists the lines "A1, B1" only
