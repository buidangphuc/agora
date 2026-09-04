@promo
Feature: Voucher redemption and flash-sale storefront
  As a buyer, I can redeem a voucher at checkout and see live flash-sale pricing,
  so that promotions from team-promotion are honoured end to end through the UI.

  @buyer
  Scenario: Buyer redeems a valid voucher at checkout
    Given a promotion buyer has a qualifying cart and a saved address
    When the buyer applies the voucher code "SAVE10" at checkout
    Then the voucher discount is shown and the order total is reduced
    And placing the order creates the order

  @buyer
  Scenario: Invalid voucher code is rejected at checkout
    Given a promotion buyer has a qualifying cart and a saved address
    When the buyer applies the voucher code "BOGUS-NOPE-999" at checkout
    Then a voucher error reason is shown and no discount is applied

  @buyer
  Scenario: Flash-sale listing shows sale price and live remaining-stock meter
    Given a listing with an active flash-sale campaign
    When a visitor views that listing
    Then the flash-sale meter shows the sale price and remaining stock

  Scenario: Visitor browses available vouchers
    Given the "vouchers" page is open
    Then the vouchers hub lists available vouchers
