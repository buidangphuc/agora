@seller
Feature: A seller's wallet is reachable only by its owner (and read-only by an admin)
  Every wallet RPC (GetSellerWallet, RequestPayout, ListPayoutHistory,
  GetWalletBalance, ListLedgerEntries, RequestWalletPayout) is enforced in
  team-payment from the gateway-forwarded principal, never from the request
  seller_id: the owner may read and pay out, an admin may read but never pay
  out, any other caller is rejected.

  Scenario: A seller reads their own wallet
    Given a signed-in seller
    When the seller requests their wallet
    Then the wallet call returns 200

  Scenario: A buyer cannot request a payout from a seller's wallet
    Given a signed-in seller
    And a signed-in buyer acting against that seller
    When the buyer requests a payout from the seller's wallet to their own bank account
    Then the wallet call returns 403
    And the seller's payout history has no payout

  Scenario: A user cannot read another seller's ledger
    Given a signed-in seller
    And a signed-in buyer acting against that seller
    When the buyer requests the seller's ledger
    Then the wallet call returns 403

  Scenario: An admin reads a seller's wallet but cannot pay it out
    Given a signed-in seller
    And an admin is logged in
    When the admin requests the seller's wallet
    Then the wallet call returns 200
    When the admin requests a payout from the seller's wallet
    Then the wallet call returns 403
