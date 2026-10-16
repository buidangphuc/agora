@order
Feature: An unpaid online-pay order is reminded, then expires and releases its stock
  team-order stamps a payment deadline on every online-pay order, reminds the buyer once
  before it, and cancels the order with reason PAYMENT_EXPIRED once it passes, releasing
  stock through the cancel path. COD orders have no deadline. A buyer's cancel stores the
  buyer's reason and cannot carry a system reason. (order-unpaid-expiry, e2e overlay:
  deadline 45s, reminder 30s before, sweep every 2s)

  Scenario: Order expiry: Reminder is emitted two hours before the deadline
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    When "b1" places a MOCK_MOMO order for 1 of "L"
    Then the order has a payment deadline
    And exactly one payment reminder is published for the order before its deadline
    And the order carries the same payment deadline in the reminder

  Scenario: Order expiry: Order unpaid past its deadline is cancelled and its stock released
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    When "b1" places a MOCK_MOMO order for 3 of "L"
    Then the stock of "L" is 7
    And the order is still Pending
    When the payment deadline passes without payment
    Then the order is Cancelled with cancel reason "CANCEL_REASON_PAYMENT_EXPIRED"
    And the stock of "L" is 10

  Scenario: Order expiry: Expiry emits OrderCancelled with the expiry reason
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    When "b1" places a MOCK_MOMO order for 1 of "L"
    And the payment deadline passes without payment
    Then exactly one OrderCancelled is published for the order with previous status Pending and reason "CANCEL_REASON_PAYMENT_EXPIRED"

  Scenario: Order expiry: A COD order is never expired
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And a buyer "b2"
    When "b1" places a COD order for 1 of "L"
    And "b2" places a control order paid by MOCK_MOMO for 1 of "L"
    And the control order expires
    Then the control order is Cancelled with reason "CANCEL_REASON_PAYMENT_EXPIRED"
    And the order is still Pending
    And no OrderCancelled is published for the order
    And the order has no payment deadline

  Scenario: Order expiry: Buyer cancel stores the chosen reason
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    When "b1" places a COD order for 1 of "L"
    And "b1" cancels the order with reason "CANCEL_REASON_CHANGED_MIND"
    Then the call succeeds
    And the order is Cancelled with cancel reason "CANCEL_REASON_CHANGED_MIND"
    And exactly one OrderCancelled is published for the order with previous status Pending and reason "CANCEL_REASON_CHANGED_MIND"

  Scenario: Order expiry: Buyer cannot send a system reason code
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And a buyer "b2"
    When "b1" places a COD order for 1 of "L"
    And "b2" places a control order paid by MOCK_MOMO for 1 of "L"
    And "b1" cancels the order with reason "CANCEL_REASON_PAYMENT_EXPIRED"
    Then the call fails with "permission_denied"
    When "b1" cancels the order with reason "CANCEL_REASON_ADMIN_FORCE_FAIL"
    Then the call fails with "permission_denied"
    And the order is still Pending
    When the control order expires
    Then the control order is Cancelled with reason "CANCEL_REASON_PAYMENT_EXPIRED"
