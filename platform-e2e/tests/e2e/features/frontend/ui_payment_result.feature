@buyer
Feature: Payment outcome is a Result page
  OpenSpec change ui-phase-cart-checkout, /checkout/pay/[id]. Mock payment only: no real
  payment data is entered.

  Scenario: Successful payment shows the success Result
    Given a d2 buyer has an order awaiting mock payment
    When the buyer opens the d2 payment page for the order
    And the buyer presses "Thanh toán thành công" while the payment server is slow
    Then both simulator buttons are disabled while pending
    And a success result links to the order list and the order status becomes PAID

  Scenario: Failed payment offers recovery
    Given a d2 buyer has an order awaiting mock payment
    When the buyer opens the d2 payment page for the order
    And the buyer presses "Thanh toán thất bại" while the payment server is slow
    Then an error result offers retry and change of payment method and an error toast is displayed

  Scenario: Saga compensation is explained
    Given a d2 buyer has an order awaiting mock payment
    When the payment step of the order is forced to fail
    And the buyer opens the d2 payment page for the order
    Then the page explains that stock was released and the order was cancelled

  Scenario: Unknown order id
    Given a d2 buyer has an empty cart
    When the buyer opens the payment page of the unknown order id
    Then a 404 result links to the order list
