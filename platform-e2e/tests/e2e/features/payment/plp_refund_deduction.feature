@fintech
Feature: A refund comes out of the credited seller's wallet exactly once
  A refund deducts the refunded amount only for money the seller was credited, in either arrival
  order, is never blocked by balance or hold, and a cancel of a paid order refunds the buyer and
  deducts the seller automatically.
  (port-payment-ledger-integrity / seller-refund-deduction)

  Scenario: Refunding a credited payment deducts the refunded amount from the seller
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    When the seller refunds 200000 of the payment
    Then the payment read by "b1" is REFUNDED
    And within the settle window the seller has exactly 1 REFUND_DEDUCTION entry of -200000
    And the seller's balance is 300000

  Scenario: A second refund of the same payment is refused and deducts nothing more
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    And the seller refunds 200000 of the payment
    When the seller refunds 200000 of the payment
    Then the call fails with "failed_precondition"
    And the seller has exactly 1 REFUND_DEDUCTION entry of -200000
    And the seller's balance is 300000

  Scenario: Concurrent refunds of one payment deduct once
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    When the seller sends 8 concurrent refunds of 100000 for the payment
    Then exactly 1 of the calls succeeds and the others fail with "failed_precondition"
    And the seller has exactly 1 REFUND_DEDUCTION entry of -100000

  Scenario: Refunding a payment whose order was cancelled deducts nothing
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has a Pending order of "L" with an open payment
    When "b1" cancels the order
    And the payment of "b1" succeeds
    And the seller refunds 500000 of the payment
    Then the call succeeds
    And the payment read by "b1" is REFUNDED
    When a sentinel order of the seller is credited
    Then the seller's ledger has no entry of 500000 or -500000
    And the seller's balance equals the amounts of the credited orders

  Scenario: A refund issued right after payment ends with one credit and one deduction
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    When "b1" pays a new order of "L"
    And the seller refunds 500000 of the payment
    Then within the settle window the seller has exactly 1 ORDER_SETTLEMENT entry of 500000
    And within the settle window the seller has exactly 1 REFUND_DEDUCTION entry of -500000
    And the seller's balance is 0

  # @destructive: waits out the payout hold window, so it needs the short-window overlay
  # platform-e2e/compose/payment-ledger.override.yaml (PAYOUT_HOLD_WINDOW=20s on team-payment). Serial lane only.
  @destructive
  Scenario: A refund after the proceeds were paid out takes the balance negative
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" whose credit has aged past the hold window
    And the seller requests a wallet payout of 500000
    When the seller refunds 200000 of the payment
    Then the call succeeds
    And the seller has exactly 1 REFUND_DEDUCTION entry of -200000
    And the seller's balance is -200000

  Scenario: Cancelling a credited paid order refunds the buyer and deducts the seller
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    When "b1" cancels the order
    Then within the settle window the payment read by "b1" is REFUNDED
    And within the settle window the seller has exactly 1 REFUND_DEDUCTION entry of -500000
    And the seller's balance is 0

  Scenario: Cancelling a paid order before its credit is observed ends with one credit and one deduction
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    When "b1" pays a new order of "L"
    And "b1" cancels the order as soon as it reads Paid
    Then within the settle window the payment read by "b1" is REFUNDED
    And within the settle window the seller has exactly 1 ORDER_SETTLEMENT entry of 500000
    And within the settle window the seller has exactly 1 REFUND_DEDUCTION entry of -500000

  # @destructive: stops team-payment (docker stop) while the cancel is recorded; serial lane only.
  @destructive
  Scenario: A cancel while team-payment is stopped is refunded when it restarts
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    And team-payment is stopped
    When "b1" cancels the order
    And team-payment is started again
    Then within the settle window the payment read by "b1" is REFUNDED
    And within the settle window the seller has exactly 1 REFUND_DEDUCTION entry of -500000

  # @destructive: produces a record onto the shared order.events topic (replay); serial lane only.
  @destructive
  Scenario: A redelivered cancellation does not refund or deduct again
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    And "b1" cancels the order
    And within the settle window the seller has exactly 1 REFUND_DEDUCTION entry of -500000
    When the OrderCancelled record of the order is produced to order.events again, byte for byte
    And a sentinel order of the seller is credited
    Then the seller has exactly 1 REFUND_DEDUCTION entry of -500000

  Scenario: Cancelling an order its seller already refunded deducts nothing more
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    And the seller refunds 200000 of the payment
    When "b1" cancels the order
    Then the call succeeds
    And the payment read by "b1" is REFUNDED
    When a sentinel order of the seller is credited
    Then the seller has exactly 1 REFUND_DEDUCTION entry of -200000
    And the seller's ledger has no entry of -500000
