@buyer @checkout_coherence
Feature: Checkout coherence in the buyer UI
  OpenSpec change frontend-checkout-coherence (initiative q11-checkout-coherence). Checkout shows
  one group per shop with the server's figures, vouchers are entered per scope, the delivery date
  comes from the server, an order is placed only at the address the buyer saw, and every
  online-pay order of a checkout can be paid. Shops, vouchers, addresses and carts are seeded
  through the gateway; the expected figures are read from the real PreviewCheckout.

  Scenario: Checkout coherence: the shown totals equal the placed orders
    Given a cc buyer in "Đà Nẵng" has one item of each price 300000 from two shops
    When the cc buyer opens checkout at the "confirm" step
    Then the cc summary total shown is the server preview total
    And the cc confirm page shows the per-group shipping fee 35000 twice
    When the cc buyer places the order
    Then the cc buyer has 2 orders whose total amounts sum to the summary total shown before placing

  Scenario: Checkout coherence: another shop's voucher is refused for a group
    Given a cc buyer in "Đà Nẵng" has one item of each price 300000 from two shops
    And shop A has a cc shop voucher and shop B has a cc shop voucher
    When the cc buyer opens the "payment" step with shop B's own code on group B
    Then the cc summary shows a voucher discount and a total below the undiscounted total
    When the cc buyer opens the "payment" step with shop A's code on group B
    Then the cc page shows the server's refusal reason for group B's voucher
    And the cc summary shows no voucher discount and the undiscounted total

  Scenario: Checkout coherence: two shop vouchers and a platform voucher place discounted orders
    Given a cc buyer in "Đà Nẵng" has one item of each price 300000 from two shops
    And shop A has a cc shop voucher and shop B has a cc shop voucher
    And a cc platform voucher exists
    When the cc buyer opens the "payment" step with both shop vouchers and the platform voucher
    And the cc buyer goes on to the "confirm" step
    Then the cc summary discount is the server preview discount
    When the cc buyer places the order
    Then each placed cc order discount equals its group's shop discount plus platform share
    And the cc placed orders discounts sum to the summary discount shown

  Scenario: Checkout coherence: the delivery date matches the mock rule end to end
    Given a cc buyer in "Hà Nội" has one item of each price 300000 from one shop
    When the cc buyer opens the product page of the item
    Then the cc page shows the delivery date of today plus 2 days
    When the cc buyer opens checkout at the "confirm" step
    Then the cc page shows the delivery date of today plus 2 days
    Given a cc buyer in "Cần Thơ" has one item of each price 300000 from one shop
    When the cc buyer opens the product page of the item
    Then the cc page shows the delivery date of today plus 4 days
    When the cc buyer opens checkout at the "confirm" step
    Then the cc page shows the delivery date of today plus 4 days
    When the cc visitor is anonymous and opens the product page of the item
    Then the cc page shows no delivery date

  Scenario: Checkout coherence: an edit after confirm shows the new address
    Given a cc buyer in "Đà Nẵng" has one item of each price 300000 from one shop
    When the cc buyer opens checkout at the "confirm" step
    Then the cc confirm page shows the street "29 Lieu Giai"
    When the cc buyer's address street is edited to "77 Edited Street" through the address API
    And the cc buyer presses the place order button
    Then the cc buyer has no order
    And the cc page shows the street "77 Edited Street" and a message that the address changed
    When the cc buyer presses the place order button
    Then the cc buyer has 1 order placed at the street "77 Edited Street"

  Scenario: Checkout coherence: both orders of a two-shop checkout are payable
    Given a cc buyer in "Đà Nẵng" has one item of each price 300000 from two shops
    When the cc buyer places the order paying with MoMo
    Then the cc browser lands on the pay page of one of the two orders
    When the cc buyer pays successfully on the pay page
    Then the cc success state links to "Thanh toán đơn tiếp theo"
    When the cc buyer follows "Thanh toán đơn tiếp theo"
    Then the cc browser lands on the pay page of the other order
    When the cc buyer pays successfully on the pay page
    Then both cc orders reach status PAID with exactly one payment transaction each
    Given a cc buyer in "Đà Nẵng" has one item of each price 300000 from two shops
    When the cc buyer places the order paying cash on delivery
    Then the cc buyer has 2 orders and none has a payment transaction

  Scenario: Checkout coherence: pay now works for an order without a transaction
    Given a cc buyer in "Hà Nội" has an online order with no payment transaction
    When the cc buyer opens the order list
    And the cc buyer presses "Thanh toán ngay" on the order
    Then the cc pay page answers 200 with the order amount and the payment simulator
