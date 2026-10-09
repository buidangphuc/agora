@buyer
Feature: Cart and checkout - empty state, layout shift, lazy images and token lint
  OpenSpec change ui-phase-cart-checkout. The skeleton is observed by delaying the cart's
  RSC request on a client-side navigation; the layout shift comes from the browser's
  layout-shift entries.

  Scenario: Empty cart
    Given a d2 buyer has an empty cart
    When the d2 buyer opens the cart
    Then the empty cart state links to continue shopping and no summary or buy button is rendered

  Scenario: Loading skeleton matches the page footprint
    Given a d2 buyer has a cart with one item from each of the shops "Shop Alpha"
    When the d2 buyer opens the cart from the home page while the cart data is slow
    Then a skeleton with a shop card, three item rows and the summary is shown before the cart
    And the layout shift score of the cart is 0

  Scenario: Below-the-fold thumbnails are lazy
    Given a d2 buyer has a cart with one item from each of 3 shops
    When the d2 buyer opens the cart
    Then thumbnails in the second and third shop groups are lazy and the first group's are eager

  Scenario: Token lint is clean
    Then the token lint reports no violation under "src/features/cart,src/features/order,src/features/payment,src/app/(shop)/cart,src/app/(checkout)"
