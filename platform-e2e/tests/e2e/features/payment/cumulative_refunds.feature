@fintech
Feature: A payment is refunded in several parts, each identified by its refund id
  A payment can be refunded several times up to its amount; every refund carries a caller-chosen
  refund id that makes it idempotent; the payment reports what was refunded and by which refund;
  the order's seller can read the order's payment; payments refunded under the single-refund model
  keep their outcome after the migration.
  Refunds go through the gateway as the order's seller. The migration scenarios use a scratch
  database and a throwaway team-payment (serial lane, @destructive).
  (payment-refund-model / payment-cumulative-refunds)

  Scenario: Two partial refunds leave the payment partially refunded and then refunded
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    When the seller refunds 200000 of the payment with a fresh refund id
    Then the payment read by "b1" reads PARTIALLY_REFUNDED with a refunded amount of 200000
    When the seller refunds 300000 of the payment with a fresh refund id
    Then the payment read by "b1" reads REFUNDED with a refunded amount of 500000

  Scenario: A refund above the remainder is refused and writes nothing
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    And the seller refunds 200000 of the payment with a fresh refund id
    When the seller refunds 300001 of the payment with a fresh refund id
    Then the call fails with "failed_precondition" and the message "refund amount exceeds the refundable remainder"
    And the payment read by "b1" reads PARTIALLY_REFUNDED with a refunded amount of 200000 and 1 refund

  Scenario: Two partial refunds racing near the cap cannot over-refund
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    And the seller refunds 300000 of the payment with a fresh refund id
    When the seller sends 2 concurrent refunds of 150000 for the payment, each with its own refund id
    Then exactly 1 of the calls succeeds and the others fail with "failed_precondition"
    And the payment read by "b1" reads PARTIALLY_REFUNDED with a refunded amount of 450000
    And the seller's REFUND_DEDUCTION entries are -300000 and -150000
    And each REFUND_DEDUCTION entry references the refund it deducts

  Scenario: Concurrent refunds with their own ids refund at most the payment amount
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    When the seller sends 8 concurrent refunds of 125000 for the payment, each with its own refund id
    Then exactly 4 of the calls succeed and the others fail with "failed_precondition"
    And the payment read by "b1" reads REFUNDED with a refunded amount of 500000 and 4 refunds
    And the seller's REFUND_DEDUCTION entries are 4 of -125000, each referencing one of the payment's refunds

  Scenario: Retrying a refund with the same refund id refunds once
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    When the seller refunds 200000 of the payment with refund id "R"
    And the seller sends the same refund call again
    Then the call succeeds
    And the payment read by "b1" reads PARTIALLY_REFUNDED with a refunded amount of 200000 and 1 refund
    And within the settle window the seller has exactly 1 REFUND_DEDUCTION entry of -200000

  Scenario: Reusing a refund id for a different amount is refused
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    And the seller refunds 200000 of the payment with refund id "R"
    When the seller refunds 100000 of the payment with refund id "R"
    Then the call fails with "already_exists"
    And the payment read by "b1" reads PARTIALLY_REFUNDED with a refunded amount of 200000 and 1 refund

  Scenario: A refund without a refund id is refused
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    When the seller refunds 100000 of the payment without a refund id
    Then the call fails with "invalid_argument"
    And the payment read by "b1" reads PAID with a refunded amount of 0 and no refund

  Scenario: A partially refunded payment lists its refunds through the gateway
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    And the seller refunds 200000 of the payment with refund id "R1" and reason "damaged"
    And the seller refunds 100000 of the payment with refund id "R2"
    Then the payment read by "b1" reads PARTIALLY_REFUNDED with a refunded amount of 300000
    And the payment read by "b1" lists 2 refunds with source SELLER_OR_ADMIN, source ids "R1" and "R2" and requested and applied amounts of 200000 and 100000
    And the first refund of the payment read by "b1" has reason "damaged"

  Scenario: The seller's ledger entries name the refund each deduction belongs to
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    And the seller refunds 200000 of the payment with refund id "R1"
    And the seller refunds 100000 of the payment with refund id "R2"
    Then the seller's ledger shows the ORDER_SETTLEMENT referencing the payment id
    And the seller's REFUND_DEDUCTION entries are -200000 and -100000
    And each REFUND_DEDUCTION entry references the refund it deducts

  Scenario: The order's seller reads the payment of their order
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    And the seller refunds 200000 of the payment with refund id "R1"
    When the seller reads the payment of the order through the gateway
    Then the call returns the order's payment with 200000 refunded and 1 refund

  Scenario: Another seller cannot read the payment of an order
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    When another seller reads the payment of the order through the gateway
    Then the call fails with "permission_denied"

  # @destructive: scratch database + throwaway team-payment container; serial lane only.
  @destructive
  Scenario: A legacy partial refund becomes one legacy refund and stays closed
    Given a scratch payment database at the previous schema
    And it holds a payment of 500000 refunded under the old model: status REFUNDED, refunded amount 200000, a credit of +500000 and a REFUND_DEDUCTION of -200000 referencing the payment
    When the migration is applied with the team-payment migrate image
    Then the database holds one LEGACY refund of 200000 for the legacy payment
    And the legacy deduction references the legacy refund
    And the legacy payment is still REFUNDED with a refunded amount of 200000, and the seller's ledger sum is still 300000

  # @destructive: scratch database + throwaway team-payment container; serial lane only.
  @destructive
  Scenario: A legacy partially refunded payment refuses a further refund
    Given a scratch payment database at the previous schema
    And it holds a payment of 500000 refunded under the old model: status REFUNDED, refunded amount 200000, a credit of +500000 and a REFUND_DEDUCTION of -200000 referencing the payment
    And the migration is applied with the team-payment migrate image
    When the team-payment image runs against the scratch database and an admin refunds 100000 of the legacy payment with a fresh refund id
    Then the scratch call fails with "failed_precondition"
    And the database holds 1 refund of the legacy payment

  # @destructive: scratch database + throwaway team-payment container; serial lane only.
  @destructive
  Scenario: A payment refunded before refunded amounts were recorded is left as it was
    Given a scratch payment database at the previous schema
    And it holds a payment of 500000 refunded under the old model: status REFUNDED, refunded amount 200000, a credit of +500000 and a REFUND_DEDUCTION of -200000 referencing the payment
    And it also holds a REFUNDED payment with a refunded amount of 0 and no deduction
    When the migration is applied with the team-payment migrate image
    Then the zero-refunded payment has no refund, keeps status REFUNDED and a refunded amount of 0
    And no ledger row changed apart from the legacy deduction's reference

  # @destructive: scratch database + throwaway team-payment container; serial lane only.
  @destructive
  Scenario: Rolling back the migration restores the payment-id references
    Given a scratch payment database at the previous schema
    And it holds a payment of 500000 refunded under the old model: status REFUNDED, refunded amount 200000, a credit of +500000 and a REFUND_DEDUCTION of -200000 referencing the payment
    And the migration is applied with the team-payment migrate image
    When the down migration is applied
    Then the legacy deduction references the payment id again
    And the seller's ledger sum is still 300000
