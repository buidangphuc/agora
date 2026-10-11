## MODIFIED Requirements

### Requirement: Payouts draw only on proceeds outside the hold window

The withdrawable amount SHALL be `max(0, balance − held)`, where:

- `balance` is the ledger sum.
- `held` is a sum over the seller's `COMPLETED` `ORDER_SETTLEMENT` credits created within the hold window. Each credit
  contributes the credit plus the `REFUND_DEDUCTION` entries of every refund of the credited payment, never below 0.
  A deduction belongs to the payment through its refund, because a deduction references the refund, not the payment.

`RequestWalletPayout` and `RequestPayout` SHALL refuse any amount above the withdrawable amount and write nothing (no
ledger entry, no payout request). The decision SHALL be made in the same per-seller serialised step that writes the
payout debit. `GetWalletBalance` and `GetSellerWallet` SHALL keep returning the full ledger sum, held proceeds
included.

#### Scenario: Fresh sale proceeds cannot be paid out

- **WHEN** a seller's only entry is a 500000 credit written moments ago and the seller requests a wallet payout of
  100000
- **THEN** the call fails with `FAILED_PRECONDITION` and the ledger has no `PAYOUT` entry
- **AND** both `GetWalletBalance` and `GetSellerWallet` still return 500000

#### Scenario: Proceeds can be paid out once the hold window passes

- **WHEN** the same seller waits until the credit is older than the hold window and requests a wallet payout of
  500000
- **THEN** the payout is accepted with a `PENDING` `PAYOUT` entry of -500000
- **AND** the balance becomes 0

#### Scenario: Only the proceeds inside the window are held

- **WHEN** a seller has a 300000 credit older than the hold window and a 500000 credit written moments ago, and
  requests a wallet payout of 300000 and then one of 1
- **THEN** the first payout is accepted
- **AND** the second fails with `FAILED_PRECONDITION` as held

#### Scenario: A bank payout of held proceeds is refused and records nothing

- **WHEN** a seller whose only proceeds are inside the hold window requests a bank payout through `RequestPayout`
- **THEN** the call fails with `FAILED_PRECONDITION` as held
- **AND** `ListPayoutHistory` has no new payout and the ledger has no `PAYOUT` entry

#### Scenario: Concurrent payouts cannot overdraw the withdrawable amount

- **WHEN** a seller has 100000 withdrawable and a further 500000 held, and sends eight concurrent wallet payouts of
  40000
- **THEN** exactly two succeed and the others fail with `FAILED_PRECONDITION`
- **AND** the balance is 520000

#### Scenario: Two partial refunds of a held sale both reduce its held amount

- **WHEN** a seller has a 300000 credit older than the hold window and a 500000 credit written moments ago, the recent
  sale is refunded 200000 and then 100000 with two refund ids, and the seller requests a wallet payout of 300000 and
  then one of 1
- **THEN** the first payout is accepted
- **AND** the second fails with `FAILED_PRECONDITION` as held, because 200000 of the recent sale is still held
