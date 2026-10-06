@promo
Feature: Voucher hub and promotion discovery
  As a buyer, I can explore the promotion coupons that are available.

  Scenario: Buyer explores voucher hub
    Given a buyer is logged in
    And a voucher has been seeded via the gateway
    When the buyer navigates to the voucher hub
    Then the vouchers page is displayed
    And available promotional vouchers are rendered
