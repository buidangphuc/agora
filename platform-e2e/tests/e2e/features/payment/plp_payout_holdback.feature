@fintech
Feature: Payouts draw only on proceeds outside the refund window
  withdrawable = max(0, balance - held); a refund consumes the held amount it was held for;
  refusals say why without revealing amounts; the window is configured and validated at startup.
  The short-window overlay platform-e2e/compose/payment-ledger.override.yaml (PAYOUT_HOLD_WINDOW=20s
  on team-payment) is what lets a scenario wait the window out; scenarios that do are @destructive.
  (port-payment-ledger-integrity / seller-payout-holdback)

  Scenario: Fresh sale proceeds cannot be paid out
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    When the seller requests a wallet payout of 100000
    Then the call fails with "failed_precondition"
    And the seller's ledger has no PAYOUT entry
    And both GetWalletBalance and GetSellerWallet return 500000

  # @destructive: waits the hold window out (needs payment-ledger.override.yaml). Serial lane only.
  @destructive
  Scenario: Proceeds can be paid out once the hold window passes
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" whose credit has aged past the hold window
    When the seller requests a wallet payout of 500000
    Then the payout is accepted with a PENDING PAYOUT entry of -500000
    And the seller's balance is 0

  # @destructive: waits the hold window out (needs payment-ledger.override.yaml). Serial lane only.
  @destructive
  Scenario: Only the proceeds inside the window are held
    Given a seller with listings "L3" whose order pays 300000 and "L5" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L3" whose credit has aged past the hold window
    And "b1" has paid an order of "L5" and the seller is credited
    When the seller requests a wallet payout of 300000
    Then the payout is accepted with a PENDING PAYOUT entry of -300000
    When the seller requests a wallet payout of 1
    Then the call fails with "failed_precondition" because the amount is held

  Scenario: A bank payout of held proceeds is refused and records nothing
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    When the seller requests a bank payout of 100000
    Then the call fails with "failed_precondition" because the amount is held
    And the seller's payout history is empty
    And the seller's ledger has no PAYOUT entry

  # @destructive: waits the hold window out (needs payment-ledger.override.yaml). Serial lane only.
  @destructive
  Scenario: Concurrent payouts cannot overdraw the withdrawable amount
    Given a seller with listings "L1" whose order pays 100000 and "L5" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L1" whose credit has aged past the hold window
    And "b1" has paid an order of "L5" and the seller is credited
    When the seller sends 8 concurrent wallet payouts of 40000
    Then exactly 2 of the calls succeed and the others fail with "failed_precondition"
    And the seller's balance is 520000

  # @destructive: waits the hold window out (needs payment-ledger.override.yaml). Serial lane only.
  @destructive
  Scenario: A refund of a held sale leaves older proceeds withdrawable
    Given a seller with listings "L3" whose order pays 300000 and "L5" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L3" whose credit has aged past the hold window
    And "b1" has paid an order of "L5" and the seller is credited
    When the seller refunds 500000 of the payment of the order of "L5"
    And the seller requests a wallet payout of 300000
    Then the payout is accepted with a PENDING PAYOUT entry of -300000

  # @destructive: waits the hold window out (needs payment-ledger.override.yaml). Serial lane only.
  @destructive
  Scenario: A refund of a sale past the window reduces what can be paid out
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" whose credit has aged past the hold window
    And the seller refunds 200000 of the payment
    When the seller requests a wallet payout of 300001
    Then the call fails with "failed_precondition" and the message "insufficient wallet balance"
    When the seller requests a wallet payout of 300000
    Then the payout is accepted with a PENDING PAYOUT entry of -300000

  # @destructive: waits the hold window out (needs payment-ledger.override.yaml). Serial lane only.
  # (payment-refund-model / seller-payout-holdback: a deduction belongs to the payment through its refund)
  @destructive
  Scenario: Two partial refunds of a held sale both reduce its held amount
    Given a seller with listings "L3" whose order pays 300000 and "L5" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L3" whose credit has aged past the hold window
    And "b1" has paid an order of "L5" and the seller is credited
    When the seller refunds 200000 of the payment of the order of "L5"
    And the seller refunds 100000 of the payment of the order of "L5"
    And the seller requests a wallet payout of 300000
    Then the payout is accepted with a PENDING PAYOUT entry of -300000
    When the seller requests a wallet payout of 1
    Then the call fails with "failed_precondition" because the amount is held

  Scenario: A payout above the balance is refused as insufficient
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    When the seller requests a wallet payout of 500001
    Then the call fails with "failed_precondition" and the message "insufficient wallet balance"

  Scenario: A held payout names when the amount is released
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    When the seller requests a wallet payout of 100000
    Then the held message names the instant the credit leaves the window and shows no amount

  Scenario: An invalid hold window refuses to start
    When the team-payment image is started with PAYOUT_HOLD_DAYS=-1, with PAYOUT_HOLD_DAYS=abc and with PAYOUT_HOLD_WINDOW=soon
    Then each container exits non-zero and its log names the offending key
