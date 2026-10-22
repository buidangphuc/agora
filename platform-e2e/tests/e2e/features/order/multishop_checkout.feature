@order
Feature: Multishop checkout quotes and places every shop group
  PreviewCheckout quotes one group per shop, shop and platform vouchers discount the right
  groups, and CreateOrder places each order at the quoted figures.
  (change order-multishop-checkout, initiative q11-checkout-coherence)

  Scenario: Multishop checkout: Two shops are quoted per group, not as one total
    Given a shop "A" selling a listing priced 300000
    And a shop "B" selling a listing priced 300000
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    When "b1" previews the checkout
    Then the preview holds 2 groups each with items subtotal 300000, shipping fee 35000 and no free shipping
    And the preview total amount is 670000

  Scenario: Multishop checkout: Preview totals equal the placed orders
    Given a shop "A" selling a listing priced 300000
    And a shop "B" selling a listing priced 300000
    And a shop voucher "VA" of shop "A" worth 10 percent off
    And a platform voucher "PV" worth 60000 off with min spend 0
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    And "b1" applies shop voucher "VA" to shop "A"
    And "b1" applies platform voucher "PV"
    When "b1" previews the checkout
    And "b1" places the order with the same selection
    Then the preview total amount equals the sum of the 2 placed orders' total amount
    And the preview total amount is below 670000

  Scenario: Multishop checkout: Preview ignores another buyer's cart
    Given a shop "A" selling a listing priced 300000
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    And a buyer "b2" in "Đà Nẵng" with 1 of each shop's listing in the cart
    When "b1" previews with the cart line ids of "b1"
    And "b2" previews with the cart line ids of "b1"
    Then the preview of "b1" succeeds
    And the preview of "b2" is refused with "failed_precondition" and message "cart is empty"
    And the cart of "b1" still holds 1 line

  Scenario: Multishop checkout: Preview is reachable through the gateway for a signed-in buyer
    Given a shop "A" selling a listing priced 300000
    And a shop "B" selling a listing priced 300000
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    When "b1" previews the checkout
    And an anonymous visitor previews the checkout
    Then the preview holds 2 groups each with items subtotal 300000, shipping fee 35000 and no free shipping
    And the anonymous preview is rejected as "unauthenticated"

  Scenario: Multishop checkout: Each shop voucher discounts its own group
    Given a shop "A" selling a listing priced 300000
    And a shop "B" selling a listing priced 300000
    And a shop voucher "VA" of shop "A" worth 10 percent off
    And a shop voucher "VB" of shop "B" worth 50000 off
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    And "b1" applies shop voucher "VA" to shop "A"
    And "b1" applies shop voucher "VB" to shop "B"
    When "b1" previews the checkout
    And "b1" places the order with the same selection
    Then the group of shop "A" has shop discount 30000 and the group of shop "B" has 50000
    And the placed order of shop "A" stores voucher "VA" with discount 30000
    And the placed order of shop "B" stores voucher "VB" with discount 50000

  Scenario: Multishop checkout: A shop voucher on another shop's group is refused
    Given a shop "A" selling a listing priced 300000
    And a shop "B" selling a listing priced 300000
    And a shop voucher "VA" of shop "A" worth 10 percent off
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    And "b1" applies shop voucher "VA" to shop "B"
    When "b1" previews the checkout
    Then the preview reports voucher "VA" valid=false with a reason
    And no group of the preview carries a shop discount
    When "b1" clears its voucher request
    And "b1" applies shop voucher "VA" to shop "A"
    And "b1" previews the checkout
    Then the preview reports voucher "VA" valid=true
    And the group of shop "A" has shop discount 30000 and the group of shop "B" has 0

  Scenario: Multishop checkout: Platform voucher is split and stored per order
    Given a shop "A" selling a listing priced 300000
    And a shop "B" selling a listing priced 300000
    And a platform voucher "PV" worth 60000 off with min spend 0
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    And "b1" applies platform voucher "PV"
    When "b1" places the order with the same selection
    Then each of the 2 placed orders has platform discount 30000, voucher "PV", total amount 305000 and one shared non-empty checkout id

  Scenario: Multishop checkout: Platform voucher below min spend is refused
    Given a shop "A" selling a listing priced 300000
    And a shop "B" selling a listing priced 300000
    And a platform voucher "PHIGH" worth 60000 off with min spend 700000
    And a platform voucher "PLOW" worth 60000 off with min spend 500000
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    And "b1" applies platform voucher "PHIGH"
    When "b1" previews the checkout
    Then the preview reports voucher "PHIGH" valid=false with a reason
    And no group of the preview carries a platform discount
    When "b1" clears its voucher request
    And "b1" applies platform voucher "PLOW"
    And "b1" previews the checkout
    Then the preview reports voucher "PLOW" valid=true
    And every group of the preview carries a platform discount of 30000

  Scenario: Multishop checkout: Orders of one checkout share a checkout_id
    Given a shop "A" selling a listing priced 300000
    And a shop "B" selling a listing priced 300000
    And a shop "C" selling a listing priced 300000
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    When "b1" places the order with the same selection
    Then the 3 placed orders carry one shared non-empty checkout id
    And GetOrder returns that checkout id for each placed order

  Scenario: Multishop checkout: Platform voucher is counted once for two paid orders
    Given a shop "A" selling a listing priced 300000
    And a shop "B" selling a listing priced 300000
    And a platform voucher "PV" worth 60000 off with min spend 0
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    And "b1" applies platform voucher "PV"
    When "b1" places the order with the same selection
    And "b1" pays the first placed order
    Then the used count of voucher "PV" becomes 1
    When "b1" pays the second placed order
    Then both placed orders are paid
    And the used count of voucher "PV" stays 1

  Scenario: Multishop checkout: Cancelling one order keeps the platform voucher and sibling prices
    Given a shop "A" selling a listing priced 300000
    And a shop "B" selling a listing priced 300000
    And a platform voucher "PV" worth 60000 off with min spend 0
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    And "b1" applies platform voucher "PV"
    When "b1" places the order with the same selection
    And "b1" cancels the first placed order
    Then the first placed order is cancelled
    And the platform voucher hold of the checkout is still held
    And the second placed order keeps total amount 305000 and platform discount 30000
