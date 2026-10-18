@order
Feature: A cancelled unpaid order's payment is voided and cannot be paid
  When team-order cancels an unpaid order (the payment deadline sweep or the buyer), team-payment
  voids the order's payment transaction to PAYMENT_STATUS_CANCELLED and refuses any later pay
  attempt. (payment-unpaid-cancel, q10-order-lifecycle; e2e overlay deadline 45s, sweep every 2s)

  Scenario: Expired order's payment is cancelled and cannot be paid
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And a buyer "b2"
    When "b1" places a MOCK_MOMO order for 1 of "L" and opens its payment
    Then the payment of the order reads PAYMENT_STATUS_PENDING
    When "b2" places a control MOCK_MOMO order for 1 of "L", opens its payment and pays it
    Then the control order is Paid
    When the payment deadline passes without payment
    Then within 30 s the payment of the order reads PAYMENT_STATUS_CANCELLED
    When "b1" pays the order's transaction successfully
    Then the call fails with "failed_precondition"
    And the order stays Cancelled

  Scenario: Buyer-cancelled pending order voids its failed payment
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And a buyer "b2"
    When "b1" places a MOCK_MOMO order for 1 of "L" and opens its payment
    And "b1" pays the order's transaction with a simulated failure
    Then the call succeeds
    And the payment of the order reads PAYMENT_STATUS_FAILED
    When "b2" places a control MOCK_MOMO order for 1 of "L", opens its payment and pays it
    Then the control order is Paid
    When "b1" cancels the unpaid order
    Then within 30 s the payment of the order reads PAYMENT_STATUS_CANCELLED
    When "b1" pays the order's transaction successfully
    Then the call fails with "failed_precondition"
    And the order stays Cancelled

  Scenario: Another buyer cannot pay a cancelled order's transaction
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And a buyer "b2"
    When "b1" places a MOCK_MOMO order for 1 of "L" and opens its payment
    And the payment deadline passes without payment
    Then within 30 s the payment of the order reads PAYMENT_STATUS_CANCELLED
    When "b1" pays the order's transaction successfully
    Then the call fails with "failed_precondition"
    When "b2" pays the order's transaction successfully
    Then the call fails with "permission_denied"
    And the payment of the order reads PAYMENT_STATUS_CANCELLED
