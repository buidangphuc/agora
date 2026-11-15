@seller @fintech
Feature: The seller order page shows the settlement breakdown and the wallet labels commission rows
  OpenSpec change seller-fee-breakdown-ui. Orders, online payments, vouchers and refunds go
  through the real gateway flows; the browser is signed in as the seller. The stack quotes a
  35000 shipping fee, so credits and nets are compared with the order's own total; the
  commission figures are the spec's (5% of items subtotal minus shop voucher).

  Scenario: Breakdown of a settled order
    Given a shop "A" selling a listing priced 300000
    And the seller of shop "A" is the settling seller
    And a shop voucher "VA" of shop "A" worth 20000 off
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    And "b1" applies shop voucher "VA" to shop "A"
    And "b1" places the order with the same selection
    And "b1" pays the first placed order online and the seller is credited
    When the seller opens the order page in the seller app
    Then the settlement card lists the credit, commission 14000 labelled "5%", returned 0, deducted 0 and the net

  Scenario: Breakdown after a partial refund
    Given a shop "A" selling a listing priced 300000
    And the seller of shop "A" is the settling seller
    And a shop voucher "VA" of shop "A" worth 20000 off
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    And "b1" applies shop voucher "VA" to shop "A"
    And "b1" places the order with the same selection
    And "b1" pays the first placed order online and the seller is credited
    And the seller refunds 126000 of the payment with a fresh refund id
    And within the settle window the seller has exactly 1 COMMISSION_REVERSAL entry of 5600
    When the seller opens the order page in the seller app
    Then the settlement card lists the credit, commission 14000 labelled "5%", returned 5600, deducted 126000 and the net

  Scenario: Unsettled order shows the not-settled state
    Given a shop "A" selling a listing priced 300000
    And the seller of shop "A" is the settling seller
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    And "b1" places an order without paying it
    When the seller opens the order page in the seller app
    Then the settlement card says "Đơn chưa được đối soát vào ví" and shows no amount
    And the order page still renders its other content

  Scenario: Another seller's order is not found and nothing is fetched
    Given a shop "A" selling a listing priced 300000
    And the seller of shop "A" is the settling seller
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    And "b1" places the order with the same selection
    And "b1" pays the first placed order online and the seller is credited
    When another seller opens the order page in the seller app
    Then the not-found page is shown and no settlement card or amount appears

  Scenario: Guest cannot see the breakdown
    Given a shop "A" selling a listing priced 300000
    And the seller of shop "A" is the settling seller
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    And "b1" places the order with the same selection
    And "b1" pays the first placed order online and the seller is credited
    When a visitor without a session opens the order page in the seller app
    Then the visitor is redirected to login and the page holds no settlement card or amount

  Scenario: Commission rows are labelled
    Given a shop "A" selling a listing priced 300000
    And the seller of shop "A" is the settling seller
    And a shop voucher "VA" of shop "A" worth 20000 off
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    And "b1" applies shop voucher "VA" to shop "A"
    And "b1" places the order with the same selection
    And "b1" pays the first placed order online and the seller is credited
    When the seller opens the wallet in the seller app
    Then the ledger shows a "Thanh toán đơn hàng" row and a "Phí hoa hồng" row of "-₫14.000" and no cell reads "COMMISSION"
