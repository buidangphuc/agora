@order
Feature: An approved RMA return refunds the buyer exactly once
  team-order records a refunded return as one ReturnRefunded fact in the same transaction, and
  team-payment applies it once to the order's payment and the seller's ledger. A return can never
  take back more than the order and the payment still allow.
  (payment-refund-model / return-refund-settlement)
  Written from the spec ahead of the code (red-first). The scenarios that stop team-payment,
  replay Kafka records or dead-letter a fact are @destructive: serial lane only.

  Scenario: Refunding an approved return emits one ReturnRefunded with the stored amount
    Given a seller with a listing whose order pays 500000
    And "b1" has paid an order of the listing
    And "b1" requests the return "R1" of 200000
    When the seller approves the return "R1"
    And the seller moves the return "R1" to REFUNDED
    Then the return "R1" reads REFUNDED
    And order.events carries exactly 1 ReturnRefunded record for the return "R1", keyed by the order id, with refund_amount 200000

  Scenario: Refunding a pending return is refused and emits nothing
    Given a seller with a listing whose order pays 500000
    And "b1" has paid an order of the listing
    And "b1" requests the return "R1" of 200000
    When the seller moves the return "R1" to REFUNDED
    Then the call fails with "failed_precondition"
    And the return "R1" reads PENDING
    And order.events carries no ReturnRefunded record for the return "R1"

  Scenario: Concurrent refunds of one return emit one fact
    Given a seller with a listing whose order pays 500000
    And "b1" has paid an order of the listing
    And "b1" requests the return "R1" of 200000
    And the seller approves the return "R1"
    When the seller sends 8 concurrent UpdateReturnStatus calls moving the return "R1" to REFUNDED
    Then exactly 1 of the calls succeeds and the others fail with "failed_precondition"
    And order.events carries exactly 1 ReturnRefunded record for the return "R1"

  Scenario: Refunding a return on an order never paid online is refused
    Given a seller with a listing whose order pays 500000
    And "b1" has a cash-on-delivery order of the listing that the seller handed over without an online payment
    And "b1" requests the return "R1" of 200000
    And the seller approves the return "R1"
    When the seller moves the return "R1" to REFUNDED
    Then the call fails with "failed_precondition" and message "order was not paid online; cash-on-delivery refunds are handled outside the system"
    And the return "R1" reads APPROVED
    And order.events carries no ReturnRefunded record for the return "R1"
    And the settlement dead-letter topic has no record for the return "R1"

  Scenario: The buyer cannot refund their own return
    Given a seller with a listing whose order pays 500000
    And "b1" has paid an order of the listing
    And "b1" requests the return "R1" of 200000
    And the seller approves the return "R1"
    When "b1" moves the return "R1" to REFUNDED
    Then the call fails with "permission_denied"
    And the return "R1" reads APPROVED
    And order.events carries no ReturnRefunded record for the return "R1"

  Scenario: An RMA refund refunds the buyer and deducts the seller
    Given a seller with a listing whose order pays 500000
    And "b1" has paid an order of the listing and the seller is credited
    And "b1" requests the return "R1" of 200000
    When the seller approves the return "R1"
    And the seller moves the return "R1" to REFUNDED
    Then within the settle window the payment read by "b1" is PARTIALLY_REFUNDED with 200000 refunded
    And the payment lists 1 RETURN refund whose source id is the return "R1"
    And within the settle window the seller has exactly 1 REFUND_DEDUCTION of -200000 referencing the return "R1"

  # @destructive: stops team-payment. Serial lane only.
  @destructive
  Scenario: An RMA refund while team-payment is stopped is applied when it restarts
    Given a seller with a listing whose order pays 500000
    And "b1" has paid an order of the listing and the seller is credited
    And "b1" requests the return "R1" of 200000
    And the seller approves the return "R1"
    When team-payment is stopped
    And the seller moves the return "R1" to REFUNDED
    Then the return "R1" reads REFUNDED
    When team-payment is started again
    Then within the settle window the payment read by "b1" is PARTIALLY_REFUNDED with 200000 refunded
    And within the settle window the seller has exactly 1 REFUND_DEDUCTION of -200000 referencing the return "R1"

  # @destructive: replays a record on order.events. Serial lane only.
  @destructive
  Scenario: A redelivered ReturnRefunded does not refund again
    Given a seller with a listing whose order pays 500000
    And "b1" has paid an order of the listing and the seller is credited
    And "b1" requests the return "R1" of 200000
    And the seller approves the return "R1"
    And the seller moves the return "R1" to REFUNDED
    And within the settle window the payment read by "b1" is PARTIALLY_REFUNDED with 200000 refunded
    When the ReturnRefunded record of the return "R1" is produced to order.events again, byte for byte
    And a sentinel order of the seller is credited
    Then the payment read by "b1" lists 1 RETURN refund and has 200000 refunded
    And the seller has exactly 1 REFUND_DEDUCTION referencing the return "R1"

  Scenario: Two returns on one order refund cumulatively
    Given a seller with a listing whose order pays 500000
    And "b1" has paid an order of the listing and the seller is credited
    And "b1" requests the return "R1" of 200000
    And "b1" requests the return "R2" of 300000
    When the seller approves the return "R1"
    And the seller moves the return "R1" to REFUNDED
    Then within the settle window the payment read by "b1" is PARTIALLY_REFUNDED with 200000 refunded
    When the seller approves the return "R2"
    And the seller moves the return "R2" to REFUNDED
    Then within the settle window the payment read by "b1" is REFUNDED with 500000 refunded
    And the payment lists 2 RETURN refunds
    And within the settle window the seller has exactly 2 REFUND_DEDUCTION entries
    And the seller has a REFUND_DEDUCTION of -200000 referencing the return "R1"
    And the seller has a REFUND_DEDUCTION of -300000 referencing the return "R2"

  Scenario: A return refund and a cancel of the same paid order refund the payment once in total
    Given a seller with a listing whose order pays 500000
    And "b1" has paid an order of the listing and the seller is credited
    And "b1" requests the return "R1" of 200000
    And the seller approves the return "R1"
    When the seller moves the return "R1" to REFUNDED
    And "b1" cancels the order
    Then within the settle window the payment read by "b1" is REFUNDED with 500000 refunded
    And the payment lists 1 RETURN refund and 1 ORDER_CANCEL refund whose applied amounts sum to 500000
    And within the settle window the seller's REFUND_DEDUCTION entries sum to -500000

  Scenario: A return above the order's remaining returnable amount is refused
    Given a seller with a listing whose order pays 500000
    And "b1" has paid an order of the listing
    And "b1" requests the return "R1" of 300000
    When "b1" requests a return of 300000
    Then the call fails with "invalid_argument"
    And "b1" requests the return "R2" of 200000

  Scenario: A rejected return frees its amount for a new return
    Given a seller with a listing whose order pays 500000
    And "b1" has paid an order of the listing
    And "b1" requests the return "R1" of 500000
    And the seller rejects the return "R1"
    When "b1" requests the return "R2" of 500000
    Then the return "R2" reads PENDING

  Scenario: Concurrent return requests cannot exceed the order total
    Given a seller with a listing whose order pays 500000
    And "b1" has paid an order of the listing
    When "b1" sends 8 concurrent return requests of 100000
    Then exactly 5 of the calls succeed and the others fail with "invalid_argument"
    And ListOrderReturns lists 5 returns totalling 500000

  Scenario: An RMA refund larger than what remains refunds only the remainder
    Given a seller with a listing whose order pays 500000
    And "b1" has paid an order of the listing and the seller is credited
    And "b1" requests the return "R1" of 300000
    And the seller approves the return "R1"
    And the seller refunds 400000 of the payment directly
    When the seller moves the return "R1" to REFUNDED
    Then within the settle window the payment read by "b1" is REFUNDED with 500000 refunded
    And the RETURN refund of the return "R1" lists requested 300000 and applied 100000
    And within the settle window the seller's REFUND_DEDUCTION amounts are -400000 and -100000

  Scenario: An RMA refund after the payment was fully refunded records nothing to refund
    Given a seller with a listing whose order pays 500000
    And "b1" has paid an order of the listing and the seller is credited
    And the seller refunds 500000 of the payment directly
    And "b1" requests the return "R1" of 200000
    And the seller approves the return "R1"
    When the seller moves the return "R1" to REFUNDED
    Then the RETURN refund of the return "R1" lists requested 200000 and applied 0
    And the payment read by "b1" is REFUNDED with 500000 refunded
    And the seller has no REFUND_DEDUCTION referencing the return "R1"
    And the settlement dead-letter topic has no record for the return "R1"

  # @destructive: produces a record to order.events that lands on the DLQ. Serial lane only.
  @destructive
  Scenario: A ReturnRefunded for an order without a paid payment is dead-lettered
    Given a seller with a listing whose order pays 500000
    And "b1" has an unpaid order of the listing
    When a well-formed ReturnRefunded record for that unpaid order is produced to order.events
    And "b2" pays a new order of the listing
    Then the ReturnRefunded record appears on the settlement dead-letter topic
    And the seller has no REFUND_DEDUCTION
    And within the settle window the seller is credited exactly 1 time of 500000

  Scenario: The seller lists the returns of their order
    Given a seller with a listing whose order pays 500000
    And "b1" has paid an order of the listing
    And "b1" requests the return "R1" of 200000
    And "b1" requests the return "R2" of 100000
    When the seller lists the returns of the order through the gateway
    Then both returns are listed newest first, each with its reason, refund amount and status

  Scenario: Another buyer cannot list the returns of an order
    Given a seller with a listing whose order pays 500000
    And "b1" has paid an order of the listing
    And "b1" requests the return "R1" of 200000
    And a buyer "b2"
    When "b2" lists the returns of the order through the gateway
    Then the call fails with "permission_denied"
