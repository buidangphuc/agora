@fintech
Feature: A paid order credits its seller exactly once, from the order's own fact
  The seller is credited by team-payment's consumer of team-order's OrderPaidEvent on order.events
  (never by the payment RPC), once per payment, only for an order that became Paid, and the ledger
  store keeps its rows unique and sign-consistent.
  (port-payment-ledger-integrity / seller-settlement-credit)

  Scenario: Paying an order credits its seller with the paid amount
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    When "b1" pays a new order of "L"
    Then within the settle window the seller has exactly 1 ORDER_SETTLEMENT entry of 500000 with status COMPLETED
    And that entry references the payment transaction of the order
    And the seller's balance grew by 500000

  Scenario: Paying an already paid order again does not credit twice
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    When "b1" calls the mock payment again for the same order
    Then the call reports the order already paid
    When a sentinel order of the seller is credited
    Then the seller has exactly 1 ORDER_SETTLEMENT entry of 500000

  Scenario: Concurrent settlements of one order credit the seller once
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has a Pending order of "L" with an open payment
    When "b1" sends 8 concurrent successful mock payments for the order
    Then the order read by "b1" is Paid
    When a sentinel order of the seller is credited
    Then the seller has exactly 1 ORDER_SETTLEMENT entry of 500000

  Scenario: A late payment for a cancelled order does not credit the seller
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has a Pending order of "L" with an open payment
    When "b1" cancels the order
    And the payment of "b1" succeeds
    Then the order read by "b1" is Cancelled
    And the payment read by "b1" is PAID
    When a sentinel order of the seller is credited
    Then the seller has exactly 0 ORDER_SETTLEMENT entries of 500000
    And the seller's balance equals the amounts of the credited orders

  # @destructive: produces a record onto the shared order.events topic (replay); serial lane only.
  @destructive
  Scenario: Replaying a paid-order event does not credit again
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    When the OrderPaidEvent record of the order is produced to order.events again, byte for byte
    And a sentinel order of the seller is credited
    Then the seller has exactly 1 ORDER_SETTLEMENT entry of 500000
    And the seller's balance equals the amounts of the credited orders

  # @destructive: stops team-payment, moves its consumer group back with rpk and starts it again; serial lane only.
  @destructive
  Scenario: Re-consuming order events after a restart does not credit again
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid 2 orders of "L" and the seller is credited for each
    When team-payment is stopped
    And the consumer group is moved back to before those orders' events
    And team-payment is started again
    Then once the group has caught up the seller's ledger entries and balance are unchanged
    When "b1" pays a new order of "L" and the seller is credited
    Then the seller has exactly 3 ORDER_SETTLEMENT entries of 500000

  # @destructive: produces a malformed record onto the shared order.events topic; serial lane only.
  @destructive
  Scenario: A malformed order event is dead-lettered and later credits still apply
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    When a malformed record is produced to order.events
    And "b1" pays a new order of "L"
    Then the malformed record appears on the settlement dead-letter topic
    And within the settle window the seller has exactly 1 ORDER_SETTLEMENT entry of 500000

  Scenario: A shipped-order event does not touch the ledger
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    When the seller ships the order
    And the OrderShipped event of the order is on order.events
    And a sentinel order of the seller is credited
    Then the seller's ledger holds only 2 ORDER_SETTLEMENT entries

  Scenario: The ledger store refuses a second settlement credit for one payment
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    When a second ORDER_SETTLEMENT row with the reference of the credited payment is inserted directly into team-payment's database
    Then the insert fails with a unique violation
    And the seller's balance through the gateway is unchanged

  Scenario: The ledger store refuses a settlement credit that is not positive
    Given a seller with a listing "L" whose order pays 500000
    When an ORDER_SETTLEMENT row of -1 with a fresh reference is inserted directly into team-payment's database
    Then the insert fails with a check violation

  Scenario: The ledger store refuses a refund deduction without a reference
    Given a seller with a listing "L" whose order pays 500000
    When a REFUND_DEDUCTION row of -1 without a reference is inserted directly into team-payment's database
    Then the insert fails with a check violation
