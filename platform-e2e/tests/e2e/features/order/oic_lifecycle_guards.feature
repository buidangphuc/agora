@order
Feature: Order status changes follow one transition table and are compare-and-set
  Which status changes are legal and who may make them, how team-order enforces them atomically
  under concurrency, what a cancel does to stock and vouchers, and how late payments are handled.
  (port-order-inventory-correctness / order-lifecycle-guards)

  Scenario: A completed order cannot be reopened
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Completed order for 2 of "L"
    When "sa" calls UpdateOrderStatus to Pending on the order
    Then the call fails
    And the order read by "b1" remains Completed

  Scenario: A seller cannot mark an order paid
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Pending order for 2 of "L"
    When "sa" calls UpdateOrderStatus to Paid on the order
    Then the call fails with "permission_denied"
    And the order read by "b1" remains Pending

  Scenario: Skipping from paid straight to completed is refused
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Paid order for 2 of "L"
    When "sa" calls UpdateOrderStatus to Completed on the order
    Then the call fails with "failed_precondition"
    And the order read by "b1" remains Paid

  Scenario: The seller ships a paid order
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Paid order for 2 of "L"
    When "sa" calls UpdateOrderStatus to Shipped on the order
    Then the call succeeds
    And the order read by "b1" is Shipped

  Scenario: A stranger cannot change an order's status
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And a buyer "stranger"
    And "b1" has a Pending order for 2 of "L"
    When "stranger" calls UpdateOrderStatus to Shipped on the order
    Then the call fails with "permission_denied"
    And the order read by "b1" remains Pending

  Scenario: Concurrent cancels restore stock once
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Pending order for 2 of "L"
    When "b1" sends two simultaneous cancels of the order
    Then exactly one call succeeds and the other fails with failed_precondition
    And the stock of "L" is back to 10

  Scenario: A cancel racing a shipment applies exactly one of them
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Paid order for 2 of "L"
    When "sa" ships the order while "b1" cancels it at the same time
    Then exactly one call succeeds and the order is Shipped with stock 8 or Cancelled with stock 10

  Scenario: Cancelling a paid order restores its stock once
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Paid order for 2 of "L"
    When "b1" cancels the order
    Then the call succeeds
    And the order read by "b1" is Cancelled
    And the stock of "L" is back to 10

  Scenario: Cancelling a shipped order is refused and keeps its stock
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Shipped order for 2 of "L"
    When "b1" cancels the order
    Then the call fails with "failed_precondition"
    And the order read by "b1" remains Shipped
    And the stock of "L" is unchanged at 8

  # @destructive: stops team-domain (docker stop) and relies on the short-TTL overlay
  # platform-e2e/compose/order-inventory.override.yaml (RESERVATION_TTL=20s, RESERVATION_SWEEP_INTERVAL=2s)
  # so the sweep returns the parked release within TTL + 2 sweep intervals. Serial lane only.
  @destructive
  Scenario: A cancel whose stock release fails is retried until the stock returns
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Pending order for 2 of "L"
    And team-domain is stopped
    When "b1" cancels the order
    And team-domain is started again
    Then the call succeeds
    And the order read by "b1" is Cancelled
    And the stock of "L" is back to 10 within the reservation TTL and two sweep intervals

  Scenario: A late payment after cancel is ignored
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Pending order for 2 of "L"
    When "b1" starts an online payment for the order
    And "b1" cancels the order
    And the payment of "b1" succeeds
    Then the order read by "b1" stays Cancelled after the payment settles
    And the stock of "L" is back to 10

  Scenario: Payment racing cancel ends cancelled with stock restored once
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Pending order for 2 of "L"
    When "b1" starts an online payment for the order
    And the payment of "b1" succeeds while "b1" cancels the order
    Then the order read by "b1" ends Cancelled
    And the stock of "L" is back to 10

  Scenario: A late payment of a cancelled voucher order leaves the voucher unused
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And a platform voucher with a quota of 1
    And "b1" has a Pending order for 2 of "L" placed with the voucher
    When "b1" starts an online payment for the order
    And "b1" cancels the order
    And the payment of "b1" succeeds
    Then the order read by "b1" stays Cancelled after the payment settles
    And another buyer can still check out with that voucher

  Scenario: Shipping a cancelled order is refused
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Pending order for 2 of "L"
    When "b1" cancels the order
    And "sa" calls CreateShipment for the order
    Then the call fails with "failed_precondition"
    And no shipment exists for the order
    And the order read by "b1" remains Cancelled
