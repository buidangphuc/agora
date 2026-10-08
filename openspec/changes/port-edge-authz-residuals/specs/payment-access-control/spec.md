## ADDED Requirements

### Requirement: Payouts require the seller scope

`RequestPayout` and `RequestWalletPayout` SHALL require the caller to hold `listing.write`, in addition to acting on
their own wallet. A caller without it SHALL get `PERMISSION_DENIED` and no payout SHALL be recorded.

#### Scenario: A buyer cannot request a payout

- **WHEN** a logged-in buyer, who holds no `listing.write`, calls `RequestWalletPayout` for their own id through the
  gateway
- **THEN** the call fails with `permission_denied` and the buyer's wallet ledger has no `PAYOUT` entry

#### Scenario: A seller can still request a payout

- **WHEN** a seller with an available balance calls `RequestWalletPayout` for part of it through the gateway
- **THEN** a `PAYOUT` entry for that amount appears in the seller's wallet ledger
