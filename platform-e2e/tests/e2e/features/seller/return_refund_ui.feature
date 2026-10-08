@seller
Feature: The seller handles returns and sees the refund the payment applied
  The seller approves, rejects and refunds an order's returns from the order page and sees what the
  payment actually applied; the buyer follows the return without a refund control.
  (payment-refund-model / seller-return-refund-ui)
  Written from the spec ahead of the code (red-first). The scenario that stops team-payment is
  tagged destructive (serial lane only).

  Scenario: The seller approves a return from the order page
    Given a seller with a listing whose order pays 500000
    And "b1" has paid an order of the listing and the seller is credited
    And "b1" requests the return "R1" of 200000
    When the seller opens the returns tab of the order
    And the seller clicks "Duyệt" on the return "R1"
    Then the return "R1" reads "Đã duyệt" and offers the actions "Hoàn tiền, Từ chối"
    When the seller reloads the page
    Then the return "R1" reads "Đã duyệt" and offers the actions "Hoàn tiền, Từ chối"

  Scenario: The seller refunds an approved return and it reads refunded
    Given a seller with a listing whose order pays 500000
    And "b1" has paid an order of the listing and the seller is credited
    And "b1" requests the return "R1" of 200000
    And the seller approves the return "R1"
    When the seller opens the returns tab of the order
    And the seller clicks "Hoàn tiền" on the return "R1" and confirms
    Then the return "R1" reads "Đã hoàn tiền" and offers the actions ""
    And within the settle window the payment read by "b1" is PARTIALLY_REFUNDED with 200000 refunded

  Scenario: The seller rejects a pending return
    Given a seller with a listing whose order pays 500000
    And "b1" has paid an order of the listing and the seller is credited
    And "b1" requests the return "R1" of 200000
    When the seller opens the returns tab of the order
    And the seller clicks "Từ chối" on the return "R1"
    Then the return "R1" reads "Đã từ chối" and offers the actions ""
    And the payment read by "b1" is PAID with 0 refunded

  Scenario: A COD return shows that its refund is handled outside the system
    Given a seller with a listing whose order pays 500000
    And "b1" has a cash-on-delivery order of the listing that the seller handed over without an online payment
    And "b1" requests the return "R1" of 200000
    And the seller approves the return "R1"
    When the seller opens the returns tab of the order
    Then the return "R1" reads "Đã duyệt" and offers the actions "Từ chối"
    And the return "R1" shows the message "Đơn thanh toán khi nhận hàng (COD): việc hoàn tiền được xử lý ngoài hệ thống."

  Scenario: Refunding a return that is no longer approved shows the error
    Given a seller with a listing whose order pays 500000
    And "b1" has paid an order of the listing and the seller is credited
    And "b1" requests the return "R1" of 200000
    And the seller approves the return "R1"
    And the seller opens the returns tab of the order
    And the return "R1" reads "Đã duyệt" and offers the actions "Hoàn tiền, Từ chối"
    When the seller rejects the return "R1"
    And the seller clicks "Hoàn tiền" on the return "R1" and confirms
    Then the page shows an error
    When the seller reloads the page
    Then the return "R1" reads "Đã từ chối" and offers the actions ""
    And the payment read by "b1" is PAID with 0 refunded

  # @destructive: stops team-payment. Serial lane only.
  @destructive
  Scenario: A refunded return shows processing until the payment applies it
    Given a seller with a listing whose order pays 500000
    And "b1" has paid an order of the listing and the seller is credited
    And "b1" requests the return "R1" of 200000
    And the seller approves the return "R1"
    And the seller opens the returns tab of the order
    When team-payment is stopped
    And the seller clicks "Hoàn tiền" on the return "R1" and confirms
    Then the return "R1" reads "Đã hoàn tiền" and offers the actions ""
    And the return "R1" shows the refund state "Đang xử lý hoàn tiền"
    And the payment summary says the payment could not be loaded
    When team-payment is started again
    And the page is reloaded until the return "R1" shows the refund state "Đã hoàn 200.000₫"
    Then the payment summary reads "200.000₫" refunded of "500.000₫"

  Scenario: A return refunded only in part says how much was refunded
    Given a seller with a listing whose order pays 500000
    And "b1" has paid an order of the listing and the seller is credited
    And "b1" requests the return "R1" of 300000
    And the seller approves the return "R1"
    And the seller refunds 400000 of the payment directly
    And the seller opens the returns tab of the order
    When the seller clicks "Hoàn tiền" on the return "R1" and confirms
    And the page is reloaded until the return "R1" shows the refund state "Chỉ hoàn được 100.000₫"
    Then the payment summary reads "500.000₫" refunded of "500.000₫"

  Scenario: The buyer sees the return status without a refund button
    Given a seller with a listing whose order pays 500000
    And "b1" has paid an order of the listing and the seller is credited
    And "b1" requests the return "R1" of 200000
    And the seller approves the return "R1"
    And the seller moves the return "R1" to REFUNDED
    When "b1" opens the returns tab of the order
    And the page is reloaded
    Then the return "R1" reads "Đã hoàn tiền" on the buyer's page
    And the buyer's page has no "Hoàn tiền" button
