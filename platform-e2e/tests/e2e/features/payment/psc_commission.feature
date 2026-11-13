@fintech
Feature: Settlement books a commission against the seller and refunds return it
  team-payment books one COMMISSION entry per settled order (5% of items subtotal minus shop
  voucher, rounded half up, shipping and platform voucher outside the base), credits the platform
  voucher to the seller on top of the paid amount, returns the commission in proportion to each
  refund, holds a sale net of its commission, and reports gross, commission and net through
  GetOrderSettlement. The stack quotes a 35000 shipping fee, so credits are compared with the
  order's own total; commissions and reversals are the spec's figures.
  (payment-settlement-commission)

  Scenario: A paid order books 5% on subtotal minus shop voucher
    Given a shop "A" selling a listing priced 300000
    And the seller of shop "A" is the settling seller
    And a shop voucher "VA" of shop "A" worth 20000 off
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    And "b1" applies shop voucher "VA" to shop "A"
    When "b1" places the order with the same selection
    And "b1" pays the first placed order online and the seller is credited
    Then the seller's ORDER_SETTLEMENT entry is the order's total amount plus its platform discount
    And exactly 1 COMMISSION entry of -14000 references the credited payment
    And the seller's balance is the order's credit minus 14000

  Scenario: The seller reads the breakdown of a settled order
    Given a shop "A" selling a listing priced 300000
    And the seller of shop "A" is the settling seller
    And a shop voucher "VA" of shop "A" worth 20000 off
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    And "b1" applies shop voucher "VA" to shop "A"
    And "b1" places the order with the same selection
    And "b1" pays the first placed order online and the seller is credited
    When the seller calls GetOrderSettlement for the order
    Then the call succeeds
    And the settlement reports commission 14000, commission reversed 0, refund deducted 0 and rate 500 bps
    And the settlement credited amount is the order's total amount plus its platform discount
    And the settlement net equals the seller's balance growth

  Scenario: A platform voucher is credited to the seller on top of the paid amount
    Given a shop "A" selling a listing priced 300000
    And the seller of shop "A" is the settling seller
    And a platform voucher "PV" worth 10000 off with min spend 0
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    And "b1" applies platform voucher "PV"
    When "b1" places the order with the same selection
    And "b1" pays the first placed order online and the seller is credited
    Then the seller's ORDER_SETTLEMENT entry is 10000 above the order's total amount

  Scenario: The platform voucher and shipping are outside the base
    Given a shop "A" selling a listing priced 300000
    And the seller of shop "A" is the settling seller
    And a platform voucher "PV" worth 10000 off with min spend 0
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    And "b1" applies platform voucher "PV"
    When "b1" places the order with the same selection
    And "b1" pays the first placed order online and the seller is credited
    Then exactly 1 COMMISSION entry of -15000 references the credited payment
    And the seller's ORDER_SETTLEMENT entry is 10000 above the order's total amount

  Scenario: A 40% partial refund returns 40% of the commission
    Given a shop "A" selling a listing priced 300000
    And the seller of shop "A" is the settling seller
    And a shop voucher "VA" of shop "A" worth 20000 off
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    And "b1" applies shop voucher "VA" to shop "A"
    And "b1" places the order with the same selection
    And "b1" pays the first placed order online and the seller is credited
    When the seller refunds 126000 of the payment with a fresh refund id
    Then within the settle window the seller has exactly 1 REFUND_DEDUCTION entry of -126000
    And within the settle window the seller has exactly 1 COMMISSION_REVERSAL entry of 5600
    When the seller calls GetOrderSettlement for the order
    Then the settlement reports commission 14000, commission reversed 5600, refund deducted 126000 and rate 500 bps
    And the settlement net equals the seller's balance growth

  Scenario: A full refund returns exactly the commission
    Given a shop "A" selling a listing priced 300000
    And the seller of shop "A" is the settling seller
    And a shop voucher "VA" of shop "A" worth 20000 off
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    And "b1" applies shop voucher "VA" to shop "A"
    And "b1" places the order with the same selection
    And "b1" pays the first placed order online and the seller is credited
    And the seller refunds 126000 of the payment with a fresh refund id
    When the seller refunds 189000 of the payment with a fresh refund id
    Then within the settle window the seller has exactly 1 COMMISSION_REVERSAL entry of 8400
    And the seller's COMMISSION_REVERSAL entries are 5600 and 8400
    And the seller's balance is 0

  Scenario: A tiny refund whose share rounds to zero books no reversal
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    And within the settle window the seller has exactly 1 COMMISSION entry of -25000
    When the seller refunds 1 of the payment with a fresh refund id
    Then within the settle window the seller has exactly 1 REFUND_DEDUCTION entry of -1
    And the seller's ledger has no COMMISSION_REVERSAL entry

  Scenario: Cancelling a paid order returns the whole commission
    Given a shop "A" selling a listing priced 300000
    And the seller of shop "A" is the settling seller
    And a shop voucher "VA" of shop "A" worth 20000 off
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    And "b1" applies shop voucher "VA" to shop "A"
    And "b1" places the order with the same selection
    And "b1" pays the first placed order online and the seller is credited
    When "b1" cancels the order
    Then within the settle window the seller has exactly 1 COMMISSION_REVERSAL entry of 14000
    When the seller calls GetOrderSettlement for the order
    Then the settlement net is 0

  # @destructive: produces a record onto the shared order.events topic (replay); serial lane only.
  @destructive
  Scenario: A redelivered cancellation books no second reversal
    Given a shop "A" selling a listing priced 300000
    And the seller of shop "A" is the settling seller
    And a shop voucher "VA" of shop "A" worth 20000 off
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    And "b1" applies shop voucher "VA" to shop "A"
    And "b1" places the order with the same selection
    And "b1" pays the first placed order online and the seller is credited
    And "b1" cancels the order
    And within the settle window the seller has exactly 1 COMMISSION_REVERSAL entry of 14000
    When the OrderCancelled record of the order is produced to order.events again, byte for byte
    And a sentinel order of the seller is credited
    Then the seller has exactly 1 COMMISSION_REVERSAL entry of 14000
    And the seller has exactly 1 COMMISSION entry of -14000

  # @destructive: produces a record onto the shared order.events topic (replay); serial lane only.
  @destructive
  Scenario: A redelivered paid event books no second commission
    Given a shop "A" selling a listing priced 300000
    And the seller of shop "A" is the settling seller
    And a shop voucher "VA" of shop "A" worth 20000 off
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    And "b1" applies shop voucher "VA" to shop "A"
    And "b1" places the order with the same selection
    And "b1" pays the first placed order online and the seller is credited
    When the OrderPaidEvent record of the order is produced to order.events again, byte for byte
    And a sentinel order of the seller is credited
    Then the seller has exactly 1 COMMISSION entry of -14000
    And the seller's balance equals the credits minus the commissions of the order and the sentinel

  Scenario: Another seller gets NOT_FOUND
    Given a shop "A" selling a listing priced 300000
    And the seller of shop "A" is the settling seller
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    And "b1" places the order with the same selection
    And "b1" pays the first placed order online and the seller is credited
    And the seller calls GetOrderSettlement for the order
    And the call succeeds
    When another seller calls GetOrderSettlement for the order
    Then the call fails with "not_found"

  Scenario: A buyer gets NOT_FOUND
    Given a shop "A" selling a listing priced 300000
    And the seller of shop "A" is the settling seller
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    And "b1" places the order with the same selection
    And "b1" pays the first placed order online and the seller is credited
    And the seller calls GetOrderSettlement for the order
    And the call succeeds
    When "b1" calls GetOrderSettlement for the order
    Then the call fails with "not_found"

  Scenario: An admin can read any settlement
    Given a shop "A" selling a listing priced 300000
    And the seller of shop "A" is the settling seller
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    And "b1" places the order with the same selection
    And "b1" pays the first placed order online and the seller is credited
    And the seller calls GetOrderSettlement for the order
    When an admin calls GetOrderSettlement for the order
    Then the call succeeds
    And both settlements are equal

  Scenario: An unauthenticated call is refused
    Given a shop "A" selling a listing priced 300000
    And the seller of shop "A" is the settling seller
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    And "b1" places the order with the same selection
    And "b1" pays the first placed order online and the seller is credited
    And the seller calls GetOrderSettlement for the order
    And the call succeeds
    When an anonymous caller calls GetOrderSettlement for the order
    Then the call fails with "unauthenticated"

  Scenario: A refund by a seller who is not the order's seller is denied and reverses nothing
    Given a shop "A" selling a listing priced 300000
    And the seller of shop "A" is the settling seller
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    And "b1" places the order with the same selection
    And "b1" pays the first placed order online and the seller is credited
    And within the settle window the seller has exactly 1 COMMISSION entry of -15000
    When another seller refunds 1000 of the payment
    Then the call fails with "permission_denied"
    And the seller's ledger has no REFUND_DEDUCTION or COMMISSION_REVERSAL entry

  Scenario: An unsettled order is not found
    Given a shop "A" selling a listing priced 300000
    And the seller of shop "A" is the settling seller
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    When "b1" places an order without paying it
    And the seller calls GetOrderSettlement for the order
    Then the call fails with "not_found"
    When "b1" cancels the first placed order before paying
    And the seller calls GetOrderSettlement for the order
    Then the call fails with "not_found"

  Scenario: A missing order id is rejected
    Given a seller with a listing "L" whose order pays 500000
    When the seller calls GetOrderSettlement with an empty order id
    Then the call fails with "invalid_argument"

  Scenario: Reading the settlement writes nothing
    Given a shop "A" selling a listing priced 300000
    And the seller of shop "A" is the settling seller
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    And "b1" places the order with the same selection
    And "b1" pays the first placed order online and the seller is credited
    When the seller calls GetOrderSettlement for the order 10 times
    Then the ledger entries and the balance are unchanged by the reads

  Scenario: A client cannot set or forge a commission
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    And within the settle window the seller has exactly 1 COMMISSION entry of -25000
    When the seller calls ChargeSellerFee through the gateway with a fresh reference
    Then the gateway answers HTTP 404 to the fee call
    And the seller has exactly 1 COMMISSION entry of -25000

  # @destructive: waits the hold window out (needs payment-ledger.override.yaml). Serial lane only.
  @destructive
  Scenario: A held sale is held net of its commission
    Given a seller with listings "Old" whose order pays 300000 and "New" whose order pays 310000
    And a buyer "b1"
    And "b1" has paid an order of "Old" whose credit has aged past the hold window
    And "b1" has paid an order of "New" and the seller is credited
    When the seller requests a wallet payout of 286750
    Then the payout is accepted for 286750
    When the seller requests a wallet payout of 1
    Then the call fails with "failed_precondition" because the amount is held

  Scenario: The store refuses a second commission for one payment
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    And within the settle window the seller has exactly 1 COMMISSION entry of -25000
    When a second COMMISSION row with the reference of the credited payment is inserted directly into team-payment's database
    Then the insert fails with a unique violation
    And the seller's balance through the gateway is unchanged

  Scenario: The store refuses a commission that is not negative
    Given a seller with a listing "L" whose order pays 500000
    And a COMMISSION row of -1 with a fresh reference is accepted directly by team-payment's database
    When a COMMISSION row of 1 with a fresh reference is inserted directly into team-payment's database
    Then the insert fails with a check violation

  Scenario: The store refuses a reversal that is not positive
    Given a seller with a listing "L" whose order pays 500000
    And a COMMISSION_REVERSAL row of 1 with a fresh reference is accepted directly by team-payment's database
    When a COMMISSION_REVERSAL row of -1 with a fresh reference is inserted directly into team-payment's database
    Then the insert fails with a check violation

  Scenario: The store refuses a commission without a reference
    Given a seller with a listing "L" whose order pays 500000
    And a COMMISSION row of -1 with a fresh reference is accepted directly by team-payment's database
    When a COMMISSION row of -1 without a reference is inserted directly into team-payment's database
    Then the insert fails with a check violation
