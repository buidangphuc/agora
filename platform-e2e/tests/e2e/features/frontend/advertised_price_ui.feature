@buyer @promo
Feature: The PDP and checkout show the server's shipping and price figures
  The PDP "Vận chuyển" row and the checkout summary show figures from the real
  CalculateShippingFee, a price change at checkout is explained and recoverable, and the
  fabricated shipping and insurance texts are gone. Everything is seeded through the gateway;
  a sale ends by moving its ends_at into the past (never the shared Flipt kill switch).
  (openspec change frontend-advertised-price, initiative q5-advertised-price; scenario names echo
  the spec titles, the unit-level scenarios are not-testable entries in team-frontend/FEATURES.yaml)

  Scenario: Advertised price: PDP never exposes another buyer's city
    Given an advertised-price listing priced 400000
    And an advertised-price buyer "hn" with the default address city "Hà Nội"
    And an advertised-price buyer "dn" with the default address city "Đà Nẵng"
    When "hn" opens the advertised-price listing page
    Then the shipping row states the threshold 500.000, the city "Hà Nội" and the server fee for it, and not the city "Đà Nẵng"
    When "dn" opens the advertised-price listing page
    Then the shipping row states the threshold 500.000, the city "Đà Nẵng" and the server fee for it, and not the city "Hà Nội"
    When an anonymous visitor opens the advertised-price listing page
    Then the shipping row states the threshold 500.000 and the range 20.000 to 35.000 and names no city
    And none of the removed shipping and insurance texts is on the page

  Scenario: Advertised price: displayed shipping figures match the server rule
    Given an advertised-price listing priced 400000
    And an advertised-price buyer "hn" with the default address city "Hà Nội"
    When the real shipping fee is asked for subtotal 500000 in "Hà Nội"
    Then the server answers free shipping with a fee of 0
    When the real shipping fee is asked for subtotal 499999 in "Hà Nội"
    Then the server answers a fee of 20000 that is not free
    When the real shipping fee is asked for subtotal 499999 in "Cần Thơ"
    Then the server answers a fee of 35000 that is not free
    When the real shipping fee is asked for subtotal 0 in "Hà Nội"
    Then the server answers a fee of 20000 that is not free
    When an anonymous visitor opens the advertised-price listing page
    Then the shipping row states the threshold 500.000 and the range 20.000 to 35.000 and names no city

  Scenario: Advertised price: shown checkout total equals the placed order total
    Given an advertised-price buyer in "Hà Nội" has a cart with one flash-priced item regular 400000 and sale 350000
    When the buyer opens the checkout page
    And the buyer goes to the "confirm" checkout step
    Then the summary shows subtotal 350000, shipping 20000 and total 370000
    When the buyer places the order in the browser
    Then the buyer ends up with exactly one order whose total_amount is 370000

  Scenario: Advertised price: the buyer sees the new total and places again
    Given an advertised-price buyer in "Hà Nội" has a cart with one flash-priced item regular 400000 and sale 350000
    When the buyer opens the checkout page
    And the buyer goes to the "confirm" checkout step
    And the flash campaign of the cart item is switched off
    And the buyer places the order in the browser
    Then the buyer has no orders
    And the checkout alert shows the price-changed message
    And the summary total is the subtotal 400000 plus the server shipping fee
    When the buyer places the order in the browser
    Then the buyer ends up with exactly one order whose total_amount is the shown total

  Scenario: Advertised price: a sale that started after add is also explained
    Given an advertised-price buyer in "Hà Nội" has a cart with one regular-priced item priced 400000
    When the buyer opens the checkout page
    And the buyer goes to the "confirm" checkout step
    And a flash campaign with sale price 350000 starts on the cart item
    And the buyer places the order in the browser
    Then the buyer has no orders
    And the checkout alert shows the price-changed message
    And the summary total is the subtotal 350000 plus the server shipping fee

  Scenario: Advertised price: an anonymous visitor is redirected from checkout
    Given an advertised-price buyer in "Hà Nội" has a cart with one regular-priced item priced 400000
    When the browser is signed out and requests the checkout page
    Then the response redirects to the login page
    And the checkout page shows no cart, shipping fee or price-changed message
