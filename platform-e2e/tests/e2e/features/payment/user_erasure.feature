@fintech
Feature: team-payment masks a user's payout bank details on UserErased
  A UserErased event on identity.events replaces account_number and account_name on the seller's
  payout requests and keeps ids, bank code, amounts, statuses and the ledger. Sellers need an
  aged credit to request a payout, so these scenarios use the short-window overlay
  platform-e2e/compose/payment-ledger.override.yaml (PAYOUT_HOLD_WINDOW=20s).
  (change commerce-user-erasure)

  @destructive
  Scenario: PER-1 Bank details are masked and money records are kept
    Given a seller "S" with a payout history of 2 pending payouts
    When a UserErased event for seller "S" is consumed
    Then the payout requests of "S" read account number and account name "Đã xóa" with ids, bank code, amount, status and ledger balance unchanged

  @destructive
  Scenario: PER-2 Another seller's payouts are untouched
    Given a seller "S" with a payout history of 2 pending payouts
    And a seller "T" with a payout history of 2 pending payouts
    When a UserErased event for seller "S" is consumed
    Then the payout requests of "S" read account number and account name "Đã xóa" with ids, bank code, amount, status and ledger balance unchanged
    And the payout requests of "T" still show their original bank details

  @destructive
  Scenario: PER-3 A redelivered event changes nothing
    Given a seller "S" with a payout history of 2 pending payouts
    When a UserErased event for seller "S" is consumed
    Then the payout requests of "S" read account number and account name "Đã xóa" with ids, bank code, amount, status and ledger balance unchanged
    When the same UserErased event for "S" and then one with a new event id are consumed
    Then the payment erasure consumer completes both without error and "S" is unchanged

  @destructive
  Scenario: PER-8 Payout history shows the mask
    Given a seller "S" with a payout history of 2 pending payouts
    When a UserErased event for seller "S" is consumed
    Then an admin lists the payout history of "S" and each entry shows account number and account name "Đã xóa" with its original amount and status
