@order
Feature: A shipped order completes by buyer confirmation or by itself, and returns follow a window
  The buyer confirms receipt of a shipped order (ConfirmReceipt); an unconfirmed shipped order
  completes by itself after the auto-complete window unless a return is still open. Completion
  opens a return window and emits one OrderCompleted. A return is accepted only on a shipped
  order or inside the window, emits one ReturnRequested, and a return the seller never answers is
  approved by the system. Payment (or COD placement) stamps the seller ship-by deadline.
  (order-receipt-completion, initiative q10-order-lifecycle; e2e overlay: auto-complete 12s,
  return window 20s, return auto-approve 30s, ship-by 60s, sweep every 2s)

  Scenario: Order receipt: Buyer confirms a shipped order
    Given a seller "sa" with a listing "L" priced 600000
    And a buyer "b1"
    And "b1" has a shipped order for "L"
    When "b1" confirms receipt of the order
    Then the call succeeds
    And the order reads Completed with completed_at set, no auto_complete_at and the configured return window

  Scenario: Order receipt: Another user cannot confirm receipt
    Given a seller "sa" with a listing "L" priced 600000
    And a buyer "b1"
    And a buyer "b2"
    And "b1" has a shipped order for "L"
    When "sa" confirms receipt of the order
    Then the call fails with "permission_denied"
    When "b2" confirms receipt of the order
    Then the call fails with "permission_denied"
    And the order still reads Shipped with no completed_at
    When "b1" confirms receipt of the order
    Then the call succeeds

  Scenario: Order receipt: Confirming an order that is not shipped is refused
    Given a seller "sa" with a listing "L" priced 600000
    And a buyer "b1"
    And "b1" has a paid online order for "L"
    When "b1" confirms receipt of the order
    Then the call fails with "failed_precondition"
    And the order still reads Paid with no completed_at
    And "b1" has a second shipped order for "L"
    When "b1" confirms receipt of the control order
    Then the call succeeds
    And the control order is recorded
    When "b1" confirms receipt of the control order
    Then the call fails with "failed_precondition"
    And the control order is unchanged

  Scenario: Order receipt: A buyer cannot complete through UpdateOrderStatus
    Given a seller "sa" with a listing "L" priced 600000
    And a buyer "b1"
    And "b1" has a shipped order for "L"
    When "b1" calls UpdateOrderStatus to Completed on the order
    Then the call fails with "permission_denied"
    And the order still reads Shipped with no completed_at
    When "sa" calls UpdateOrderStatus to Completed on the order
    Then the call succeeds
    And the order reads Completed with completed_at set and no auto_complete_at

  Scenario: Order receipt: Shipping stamps the auto-complete deadline
    Given a seller "sa" with a listing "L" priced 600000
    And a buyer "b1"
    And "b1" has a shipped order for "L"
    Then the order reads Shipped with shipped_at set and auto_complete_at one auto-complete window later

  Scenario: Order receipt: Unconfirmed shipped order completes after the window
    Given a seller "sa" with a listing "L" priced 600000
    And a buyer "b1"
    And "b1" has a shipped order for "L"
    When the auto-complete window passes without a confirm
    Then the order reads Completed with completed_at set and no auto_complete_at
    And exactly one OrderCompleted is published for the order with completed_by SYSTEM

  Scenario: Order receipt: An open return blocks auto-complete
    Given a seller "sa" with a listing "L" priced 600000
    And a buyer "b1"
    And "b1" has a shipped order for "L"
    And "b1" has a return of 100000 on the order
    Then the order stays Shipped past its auto-complete deadline
    When "sa" rejects the return
    Then the order read by "b1" is Completed

  Scenario: Order receipt: Buyer confirm emits OrderCompleted with completed_by BUYER
    Given a seller "sa" with a listing "L" priced 600000
    And a buyer "b1"
    And "b1" has a shipped order paid online for "L"
    When "b1" confirms receipt of the order
    Then the call succeeds
    And exactly one OrderCompleted is published for the order with completed_by BUYER
    And that OrderCompleted carries the order total, completed_at, return_window_ends_at and paid_online true

  Scenario: Order receipt: Seller completion still works and emits OrderCompleted
    Given a seller "sa" with a listing "L" priced 600000
    And a buyer "b1"
    And "b1" has a shipped order for "L"
    When "sa" calls UpdateOrderStatus to Completed on the order
    Then the call succeeds
    And the order read by "b1" is Completed
    And exactly one OrderCompleted is published for the order with completed_by SELLER

  Scenario: Order receipt: A refused completion emits nothing
    Given a seller "sa" with a listing "L" priced 600000
    And a buyer "b1"
    And "b1" has a shipped order paid online for "L"
    And "b1" has a return of 100000 on the order
    And "b1" has a second shipped order for "L"
    And "b1" has a third paid online order for "L"
    When "sa" confirms receipt of the order
    Then the call fails with "permission_denied"
    When "b1" confirms receipt of the paid order
    Then the call fails with "failed_precondition"
    When "sa" calls UpdateOrderStatus to Completed on the paid order
    Then the call fails with "failed_precondition"
    When "b1" confirms receipt of the control order
    Then the call succeeds
    And exactly one OrderCompleted is published for the control order with completed_by BUYER
    And no OrderCompleted is published for the order
    And no OrderCompleted is published for the paid order

  Scenario: Order receipt: Concurrent confirms emit one OrderCompleted
    Given a seller "sa" with a listing "L" priced 600000
    And a buyer "b1"
    And "b1" has a shipped order for "L"
    When "b1" sends 8 concurrent confirms of the order
    Then exactly one confirm succeeds and the other 7 fail with "failed_precondition"
    And exactly one OrderCompleted is published for the order with completed_by BUYER

  Scenario: Order receipt: Return on a shipped order is accepted and announced
    Given a seller "sa" with a listing "L" priced 600000
    And a buyer "b1"
    And "b1" has a shipped order for "L"
    When "b1" requests a return of 200000 on the order
    Then the call succeeds
    And the return reads PENDING for "b1"
    And exactly one ReturnRequested is published for the order with the buyer, the seller and refund_amount 200000

  Scenario: Order receipt: Return inside the window after completion is accepted
    Given a seller "sa" with a listing "L" priced 600000
    And a buyer "b1"
    And "b1" has a shipped order for "L"
    When "b1" confirms receipt of the order
    Then the call succeeds
    When "b1" requests a return of 100000 on the order
    Then the call succeeds
    And the return reads PENDING for "b1"

  Scenario: Order receipt: Return after the window closes is refused
    Given a seller "sa" with a listing "L" priced 600000
    And a buyer "b1"
    And "b1" has a shipped order for "L"
    And "b1" has a second shipped order for "L"
    When "b1" requests a return of 100000 on the control order
    Then the call succeeds
    And exactly one ReturnRequested is published for the control order with the buyer, the seller and refund_amount 100000
    When "b1" confirms receipt of the order
    Then the call succeeds
    When the return window of the order has passed
    And "b1" requests a return of 100000 on the order
    Then the call is refused with "failed_precondition" and the message "return window closed"
    And the order has no returns
    And no ReturnRequested is published for the order

  Scenario: Order receipt: Return on a paid unshipped order is refused
    Given a seller "sa" with a listing "L" priced 600000
    And a buyer "b1"
    And "b1" has a paid online order for "L"
    When "b1" requests a return of 100000 on the order
    Then the call fails with "failed_precondition"
    And the order has no returns
    When "sa" ships the order
    And "b1" requests a return of 100000 on the order
    Then the call succeeds

  Scenario: Order receipt: Unanswered return is approved after the window
    Given a paid online order of 500000 shipped by its seller
    And "b1" has a return of 100000 on the order
    When the system approves the return after the auto-approve window
    Then no ReturnRefunded is published for the return
    When "seller" refunds the return
    Then the call succeeds
    And exactly one ReturnRefunded is published for the return

  Scenario: Order receipt: Rejected return is never auto-approved
    Given a seller "sa" with a listing "L" priced 600000
    And a buyer "b1"
    And "b1" has a shipped order for "L"
    And "b1" has a return of 100000 on the order
    And "sa" rejects the return
    And "b1" has a second shipped order for "L"
    And "b1" has a return of 100000 on the control order
    When the control return is approved by the system
    Then the return reads REJECTED for "b1"

  Scenario: Order receipt: Auto-approved return is refunded through the existing path
    Given a paid online order of 500000 shipped by its seller
    And "b1" has a return of 100000 on the order
    When the system approves the return after the auto-approve window
    And "seller" refunds the return
    Then the call succeeds
    And the return reads REFUNDED for "b1"
    And exactly one ReturnRefunded is published for the return

  Scenario: Order receipt: A caller cannot request the system approve
    Given a seller "sa" with a listing "L" priced 600000
    And a buyer "b1"
    And "b1" has a shipped order for "L"
    And "b1" has a return of 100000 on the order
    When "b1" approves the return
    Then the call fails with "permission_denied"
    And the return reads PENDING for "b1"
    When the system approves the return after the auto-approve window
    Then the return reads APPROVED for "b1"

  Scenario: Order receipt: A rejected return on a shipped order can escalate
    Given a seller "sa" with a listing "L" priced 600000
    And a buyer "b1"
    And "b1" has a shipped order for "L"
    And "b1" has a return of 100000 on the order
    When "sa" rejects the return
    Then the return reads REJECTED for "b1"
    And the return reads can_escalate true for "b1"

  Scenario: Order receipt: A rejected return cannot escalate after the window closes
    Given a seller "sa" with a listing "L" priced 600000
    And a buyer "b1"
    And "b1" has a shipped order for "L"
    And "b1" has a return of 100000 on the order
    And "sa" rejects the return
    When "b1" confirms receipt of the order
    Then the call succeeds
    And the return reads can_escalate true for "b1"
    When the return window of the order has passed
    Then the return reads REJECTED for "b1"
    And the return reads can_escalate false for "b1"

  Scenario: Order receipt: Payment stamps ship_by and the paid event carries it
    Given a seller "sa" with a listing "L" priced 600000
    And a buyer "b1"
    And "b1" has a paid online order for "L"
    Then the order reads ship_by at paid_at plus the ship-by window
    And the OrderPaidEvent of the order carries the same ship_by and the seller id

  Scenario: Order receipt: A cash-on-delivery order gets ship_by at placement
    Given a seller "sa" with a listing "L" priced 600000
    And a buyer "b1"
    When "b1" places a COD order for 1 of "L"
    Then the order reads ship_by at its creation plus the ship-by window
    And the OrderPlaced of the order carries the same ship_by

  Scenario: Order receipt: An unpaid online order has no ship_by
    Given a seller "sa" with a listing "L" priced 600000
    And a buyer "b1"
    When "b1" places a MOCK_MOMO order for 1 of "L"
    Then the order reads no ship_by and its OrderPlaced carries none
    When "b1" pays the order
    Then the order reads ship_by at paid_at plus the ship-by window

  Scenario: Order receipt: A completed order cannot be reopened or cancelled
    Given a seller "sa" with a listing "L" priced 600000
    And a buyer "b1"
    And "b1" has a shipped order for "L"
    When "b1" confirms receipt of the order
    Then the call succeeds
    And the order is recorded
    When "b1" cancels the order
    Then the call fails with "failed_precondition"
    And the order is unchanged
