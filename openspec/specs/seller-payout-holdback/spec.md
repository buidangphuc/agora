# seller-payout-holdback Specification

## Purpose
Defines how much of a seller's wallet may be withdrawn: sale proceeds stay in the platform until the refund window
has passed, so a refund of a fresh sale is always covered.

## Requirements

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

### Requirement: A refund consumes the held amount it was held for

A refund of a sale still inside the hold window SHALL reduce that sale's held amount rather than the seller's
withdrawable money; a refund of a sale past the window SHALL reduce the withdrawable amount.

#### Scenario: A refund of a held sale leaves older proceeds withdrawable

- **WHEN** a seller has a 300000 credit older than the hold window and a 500000 credit written moments ago, and the
  recent sale is refunded in full
- **THEN** a wallet payout of 300000 is accepted

#### Scenario: A refund of a sale past the window reduces what can be paid out

- **WHEN** a seller's only credit is a 500000 sale older than the hold window and 200000 of it is refunded
- **THEN** a wallet payout of 300001 fails with `FAILED_PRECONDITION` `insufficient wallet balance` and a payout of
  300000 is accepted

### Requirement: Payout refusals say why without revealing amounts

An amount above the ledger balance SHALL fail with `FAILED_PRECONDITION` and message `insufficient wallet balance`.
An amount within the balance but above the withdrawable amount SHALL fail with `FAILED_PRECONDITION` and message
`amount is held until <instant> (refund window)`, where `<instant>` is an RFC3339 UTC timestamp at which enough held
credits (oldest first) leave the window to cover the amount. Neither message SHALL contain an amount.

#### Scenario: A payout above the balance is refused as insufficient

- **WHEN** a seller with a balance of 500000 requests a wallet payout of 500001
- **THEN** the call fails with `FAILED_PRECONDITION` and message `insufficient wallet balance`

#### Scenario: A held payout names when the amount is released

- **WHEN** a seller whose 500000 credit was written at time T, inside a hold window W, requests a wallet payout of
  100000
- **THEN** the message starts with `amount is held until `, ends with ` (refund window)`, carries an RFC3339 timestamp
  within a few seconds of T + W, and contains neither 100000 nor 500000

### Requirement: The hold window is configured and validated at startup

`team-payment` SHALL read the hold window from `PAYOUT_HOLD_DAYS` (integer days, default 7, valid 0–3650, 0 disables
the hold) unless `PAYOUT_HOLD_WINDOW` (a Go duration, valid 0 to 3650 days) is set, which then takes precedence. Any
invalid value SHALL make `team-payment` refuse to start with an error naming the key. Both keys SHALL be documented in
the README and `.env.example`.

#### Scenario: An invalid hold window refuses to start

- **WHEN** the `team-payment` image is started with `PAYOUT_HOLD_DAYS=-1`, with `PAYOUT_HOLD_DAYS=abc`, or with
  `PAYOUT_HOLD_WINDOW=soon`
- **THEN** each container exits non-zero and its log names the offending key
