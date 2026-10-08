## Purpose

Defines how much of a seller's wallet may be withdrawn: recent sale proceeds stay in the platform until the refund window passes, so refunds remain covered.

## ADDED Requirements

### Requirement: Withdrawable amount excludes proceeds still inside the hold window

The seller's withdrawable amount SHALL be `max(0, balance - held)` where `balance` is the ledger sum and `held` is the sum, over `ORDER_SETTLEMENT` credits created within the last `PAYOUT_HOLD_DAYS` days, of the credit minus any `REFUND_DEDUCTION` with the same reference (never below 0). `RequestPayout` and `RequestWalletPayout` SHALL reject, writing nothing, any `amount` greater than the withdrawable amount, in the same per-seller locked transaction that appends the `PAYOUT` entry. The wallet balance reported by `GetSellerWallet` and `GetWalletBalance` SHALL remain the full ledger sum.

#### Scenario: Fresh proceeds cannot be withdrawn

- **WHEN** a seller's only entry is an `ORDER_SETTLEMENT` credit of 500000 created now and the hold is 7 days and the seller requests a payout of 100000
- **THEN** the request fails with `FAILED_PRECONDITION`, no `PAYOUT` entry exists, and `GetWalletBalance` still returns 500000

#### Scenario: Proceeds become withdrawable after the window

- **WHEN** the same credit is 8 days old with a 7-day hold and the seller requests 500000
- **THEN** the payout is accepted and the balance becomes 0

#### Scenario: Only the held part is withheld

- **WHEN** a seller has a 300000 credit from 10 days ago and a 500000 credit from now, with a 7-day hold, and requests 300000
- **THEN** the payout is accepted, and a further request of 1 fails as held

#### Scenario: A refund of a held sale does not also reduce free money

- **WHEN** a seller has a 300000 credit from 10 days ago and a 500000 credit from now, and the recent sale is refunded in full
- **THEN** the withdrawable amount is still 300000

#### Scenario: A refund of a sale past the window reduces what can be withdrawn

- **WHEN** a seller has a 500000 credit from 10 days ago and that sale is refunded for 200000
- **THEN** the withdrawable amount is 300000 and a payout of 300001 is rejected

#### Scenario: Concurrent payouts cannot overdraw the withdrawable amount

- **WHEN** many concurrent payouts of 40000 are requested against 100000 of withdrawable money and more held money
- **THEN** exactly two succeed and the ledger never withdraws more than 100000

#### Scenario: A hold of 0 days keeps the previous behaviour

- **WHEN** `PAYOUT_HOLD_DAYS=0` and a seller has a credit of 500000 created now and requests 500000
- **THEN** the payout is accepted

### Requirement: Payout rejection distinguishes a short balance from held funds

When `amount` exceeds the ledger balance the error SHALL be `FAILED_PRECONDITION` `insufficient wallet balance`. When `amount` is within the balance but exceeds the withdrawable amount the error SHALL be `FAILED_PRECONDITION` `amount is held until <YYYY-MM-DD> (refund window)`, where the date is when enough held credits expire to cover the amount, and the message SHALL NOT include any amount.

#### Scenario: Amount above the balance

- **WHEN** a seller with balance 100000 requests a payout of 200000
- **THEN** the error is `FAILED_PRECONDITION` with message `insufficient wallet balance`

#### Scenario: Amount within the balance but held

- **WHEN** a seller with balance 500000, all credited 2 days ago with a 7-day hold, requests 100000
- **THEN** the error is `FAILED_PRECONDITION` with a message starting `amount is held until ` and ending `(refund window)` whose date is 5 days from now

### Requirement: The hold window is configured and validated

`PAYOUT_HOLD_DAYS` SHALL default to 7, SHALL accept integers from 0 to 3650, and SHALL fail startup with a clear error otherwise. 0 SHALL disable the hold. The variable SHALL be documented in the README and `.env.example`.

#### Scenario: Default and bounds

- **WHEN** the configuration is loaded with `PAYOUT_HOLD_DAYS` unset, `-1`, `3651` or `abc`
- **THEN** it yields 7 for unset and a startup error for the other three
