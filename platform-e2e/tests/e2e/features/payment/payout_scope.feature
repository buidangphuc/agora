@fintech
Feature: Payouts require the seller scope
  RequestPayout and RequestWalletPayout need listing.write in addition to acting on one's own
  wallet. A caller without it gets permission_denied and no payout is recorded.
  (port-edge-authz-residuals / payment-access-control)

  Scenario: A buyer cannot request a payout
    Given a buyer "b1"
    When "b1" requests a wallet payout of 100000 for their own id
    Then the call fails with "permission_denied"
    And the wallet ledger of "b1" has no payout entry

  # @destructive: waits the hold window out (needs payment-ledger.override.yaml, PAYOUT_HOLD_WINDOW=20s)
  # so the seller has an available balance. Serial lane only; stops nothing.
  @destructive
  Scenario: A seller can still request a payout
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" whose credit has aged past the hold window
    When the seller requests a wallet payout of 100000
    Then the payout is accepted with a PENDING PAYOUT entry of -100000
    And a payout entry of 100000 appears in the seller's wallet ledger
