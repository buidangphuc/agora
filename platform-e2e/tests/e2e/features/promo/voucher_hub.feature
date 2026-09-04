@promo
Feature: Voucher hub and promotion discovery
  As a buyer, I can explore promotion coupons and claim vouchers.

  Scenario: Buyer explores voucher hub
    Given a buyer is logged in
    When the buyer navigates to the voucher hub
    Then the vouchers page is displayed
    And available promotional vouchers are rendered
