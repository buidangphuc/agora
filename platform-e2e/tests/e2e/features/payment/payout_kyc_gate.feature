@fintech
Feature: Payouts require a VERIFIED seller
  RequestPayout and RequestWalletPayout read the caller's KYC status from team-verification and
  proceed only for VERIFIED; any other status fails FAILED_PRECONDITION "seller verification
  required" before any ledger or payout row is written, and an unreadable status fails closed
  with UNAVAILABLE. Authorization and validation errors keep their codes and run before the gate.
  Aged-credit scenarios need platform-e2e/compose/payment-ledger.override.yaml (PAYOUT_HOLD_WINDOW=20s);
  scenarios that stop team-verification are @destructive (serial lane).
  (payment-payout-kyc-gate)

  # @destructive: waits the hold window out (needs payment-ledger.override.yaml). Serial lane only.
  @destructive
  Scenario: Verified seller requests a bank payout
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" whose credit has aged past the hold window
    And the seller is verified through the KYC RPCs
    When the seller requests a bank payout of 100000
    Then the call succeeds
    And the seller's payout history lists a payout of 100000

  # @destructive: waits the hold window out (needs payment-ledger.override.yaml). Serial lane only.
  @destructive
  Scenario: Verified seller requests a wallet payout
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" whose credit has aged past the hold window
    And the seller is verified through the KYC RPCs
    When the seller requests a wallet payout of 100000
    Then the payout is accepted with a PENDING PAYOUT entry of -100000
    And the seller's ledger has exactly 1 PAYOUT entry

  Scenario: Seller who never submitted is refused
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    And the seller has never submitted verification
    When the seller requests a bank payout of 100000
    Then the call fails with "failed_precondition" and the message "seller verification required"
    And the seller's payout history is empty
    And the seller's balance is 500000

  Scenario: Seller with a pending submission is refused
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    And the seller has a PENDING verification submission
    When the seller requests a wallet payout of 100000
    Then the call fails with "failed_precondition" and the message "seller verification required"
    And the seller's ledger has no PAYOUT entry

  Scenario: Seller with a rejected submission is refused
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    And the seller has a REJECTED verification submission
    When the seller requests a bank payout of 100000
    Then the call fails with "failed_precondition" and the message "seller verification required"
    And the seller's balance is 500000

  # @destructive: waits the hold window out (needs payment-ledger.override.yaml). Serial lane only.
  @destructive
  Scenario: Seller who loses verification is refused on the next call
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" whose credit has aged past the hold window
    And the seller is verified through the KYC RPCs
    When the seller requests a bank payout of 100000
    Then the call succeeds
    When the seller submits a newer verification that stays PENDING
    And the seller requests a bank payout of 100000
    Then the call fails with "failed_precondition" and the message "seller verification required"
    And the seller's payout history lists exactly 1 payout

  @destructive
  Scenario: Verification service unreachable
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    And the seller is verified through the KYC RPCs
    And team-verification is stopped
    When the seller requests a bank payout of 100000 and the call is timed
    Then the call fails with "unavailable" within 3 seconds
    And the seller's payout history is empty

  # @destructive: waits the hold window out (needs payment-ledger.override.yaml). Serial lane only.
  @destructive
  Scenario: Gate recovers after the outage
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" whose credit has aged past the hold window
    And the seller is verified through the KYC RPCs
    And team-verification is stopped
    When the seller requests a bank payout of 100000
    Then the call fails with "unavailable"
    When team-verification is started again
    And the seller requests a bank payout of 100000
    Then the call succeeds
    And the seller's payout history lists exactly 1 payout

  Scenario: Invalid amount is rejected before the gate
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    And the seller has never submitted verification
    When the seller requests a bank payout of 0
    Then the call fails with "invalid_argument"

  Scenario: Another seller's wallet is denied before the gate
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And the seller has never submitted verification
    When "b1" requests a bank payout from the seller's wallet
    Then the call fails with "permission_denied"
    And the seller's payout history is empty

  Scenario: Missing listing write scope is denied before the gate
    Given a buyer "b1"
    When "b1" requests a wallet payout of 100000 for their own id
    Then the call fails with "permission_denied"

  Scenario: Status comes from the principal not the request
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    And the seller is verified through the KYC RPCs
    And a second seller "other" who has never submitted verification
    When "other" requests a bank payout naming the verified seller's id
    Then the call fails with "permission_denied"
    And the seller's payout history is empty
    And "other" has no payout in the payout history

  Scenario: Boot fails without the verification address
    When team-payment is started with UPSTREAM_VERIFICATION_ADDR unset and then blank
    Then each container exits non-zero and its log names "UPSTREAM_VERIFICATION_ADDR"

  Scenario: Seeded demo seller can pay out
    Given the marketplace seed has run
    When the seeded shop seller requests a wallet payout of 1
    Then the seeded seller's verification status is VERIFIED
    And the call is not refused for seller verification

  Scenario: Freshly registered seller is blocked at rollout
    Given a seller with a listing "L" whose order pays 500000
    And the seller has never submitted verification
    When the seller requests a wallet payout of 100000
    Then the call fails with "failed_precondition"
    And the seller's verification status is NOT_SUBMITTED
