# seller-fee-charge Specification

## Purpose
Defines how team-payment debits a seller fee once per reference from the seller's withdrawable balance, rejects invalid requests before any write, and accepts fee charges only from a service principal.

## Requirements

### Requirement: A seller fee is debited once per reference from the withdrawable balance

`PaymentService.ChargeSellerFee` SHALL append exactly one `SUBSCRIPTION_FEE` ledger entry with status `COMPLETED` and
amount `-amount` for `seller_id`, referencing `reference_id`, and return its `entry_id` with `created=true`. A second
call with the same `reference_id`, sequential or concurrent, SHALL book nothing and return the original `entry_id` with
`created=false`. The new entry SHALL appear in the seller's `ListLedgerEntries` as type `SUBSCRIPTION_FEE` with a
negative amount and SHALL lower `GetWalletBalance` by `amount`.

#### Scenario: A fee is debited from the wallet

- **WHEN** a service principal calls `ChargeSellerFee` for a seller with withdrawable balance 500000, kind
  `SUBSCRIPTION`, amount 199000 and a new `reference_id`
- **THEN** the response has `created=true`, the seller's `ListLedgerEntries` shows one new `SUBSCRIPTION_FEE` entry of
  -199000 with status `COMPLETED`, and `GetWalletBalance` is 301000
- **VERIFIED BY**: `team-payment/internal/rpc/seller_fee_test.go` `TestChargeSellerFee` subtest "A fee is debited from the wallet"

#### Scenario: Replaying a reference books nothing

- **WHEN** the same call is repeated with the same `reference_id`
- **THEN** the response has `created=false` with the first call's `entry_id`, and the seller still has exactly one
  `SUBSCRIPTION_FEE` entry for it and the same balance
- **VERIFIED BY**: `team-payment/internal/rpc/seller_fee_test.go` `TestChargeSellerFee` subtest "Replaying a reference books nothing"

#### Scenario: Concurrent charges of one reference book once

- **WHEN** eight concurrent `ChargeSellerFee` calls with one `reference_id` arrive for a seller who could pay it
- **THEN** exactly one response has `created=true`, the other seven have `created=false`, and the seller has exactly
  one `SUBSCRIPTION_FEE` entry of -amount
- **VERIFIED BY**: `team-payment/internal/rpc/seller_fee_test.go` `TestChargeSellerFee` subtest "Concurrent charges of one reference book once"

#### Scenario: A replay still succeeds after the balance dropped

- **WHEN** the fee was booked, a payout then took the remaining withdrawable balance to 0, and the same call is repeated
- **THEN** the response has `created=false` with the original `entry_id` and no error
- **VERIFIED BY**: `team-payment/internal/rpc/seller_fee_test.go` `TestChargeSellerFee` subtest "A replay still succeeds after the balance dropped"

#### Scenario: Reusing a reference with a different amount is rejected

- **WHEN** a call reuses a booked `reference_id` with a different `amount`
- **THEN** the call fails with `ALREADY_EXISTS` and the ledger and balance are unchanged
- **VERIFIED BY**: `team-payment/internal/rpc/seller_fee_test.go` `TestChargeSellerFee` subtest "Reusing a reference with a different amount is rejected"

### Requirement: A fee draws only on the withdrawable balance

`ChargeSellerFee` SHALL book a fee only when `amount` is within the seller's withdrawable balance (balance minus the
proceeds still inside the payout hold window), checked under the per-seller ledger lock together with the insert.
Otherwise it SHALL return `FAILED_PRECONDITION` and write nothing. A fee SHALL NOT make the balance negative, also under
concurrent fee charges and payouts for the same seller.

#### Scenario: A fee above the balance is refused

- **WHEN** a service principal charges 199000 to a seller whose wallet balance is 100000
- **THEN** the call fails with `FAILED_PRECONDITION`, no `SUBSCRIPTION_FEE` entry exists for the reference and the
  balance is still 100000
- **VERIFIED BY**: `team-payment/internal/rpc/seller_fee_test.go` `TestChargeSellerFee` subtest "A fee above the balance is refused"

#### Scenario: A fee against funds still on hold is refused

- **WHEN** a service principal charges 199000 to a seller whose balance is 300000 of which 250000 is a sale settled
  inside the hold window (withdrawable 50000)
- **THEN** the call fails with `FAILED_PRECONDITION`, nothing is booked, and the balance is still 300000
- **VERIFIED BY**: `team-payment/internal/rpc/seller_fee_test.go` `TestChargeSellerFeeWithdrawable` subtest "A fee against funds still on hold is refused"

#### Scenario: A fee equal to the withdrawable balance is accepted

- **WHEN** a service principal charges 50000 to a seller whose withdrawable balance is exactly 50000
- **THEN** the response has `created=true` and the seller's balance falls by 50000 and never below the held amount
- **VERIFIED BY**: `team-payment/internal/rpc/seller_fee_test.go` `TestChargeSellerFeeWithdrawable` subtest "A fee equal to the withdrawable balance is accepted"

#### Scenario: Concurrent fees and payouts cannot overdraw

- **WHEN** four `ChargeSellerFee` calls of 100000 with distinct references and four payouts of 100000 run concurrently
  for a seller whose withdrawable balance is 300000
- **THEN** at most three of the eight succeed, the others fail with `FAILED_PRECONDITION`, and the ledger sum equals
  the balance and is not below 0
- **VERIFIED BY**: `team-payment/internal/rpc/seller_fee_test.go` `TestChargeSellerFeeConcurrentWithPayout`

### Requirement: Invalid fee requests are rejected before any write

`ChargeSellerFee` SHALL return `INVALID_ARGUMENT` and write nothing when `seller_id` is empty, `amount` is not greater
than 0, `reference_id` is empty or longer than 64 characters, or `kind` is `SELLER_FEE_KIND_UNSPECIFIED` or
`SELLER_FEE_KIND_AD_SPEND`. No `AD_SPEND` ledger entry SHALL be written in v1.

#### Scenario: A zero or negative amount is rejected

- **WHEN** a service principal calls `ChargeSellerFee` with amount 0, then with amount -100
- **THEN** both calls fail with `INVALID_ARGUMENT` and the seller has no new ledger entry
- **VERIFIED BY**: `team-payment/internal/rpc/seller_fee_test.go` `TestChargeSellerFeeValidation` cases "A zero amount is rejected", "A negative amount is rejected"

#### Scenario: The reserved ad-spend kind is rejected

- **WHEN** a service principal calls `ChargeSellerFee` with kind `SELLER_FEE_KIND_AD_SPEND`
- **THEN** the call fails with `INVALID_ARGUMENT` and no ledger entry of any type is written
- **VERIFIED BY**: `team-payment/internal/rpc/seller_fee_test.go` `TestChargeSellerFeeValidation` case "The reserved ad-spend kind is rejected"

#### Scenario: A missing reference or seller is rejected

- **WHEN** a service principal calls `ChargeSellerFee` with an empty `reference_id`, then with an empty `seller_id`
- **THEN** both calls fail with `INVALID_ARGUMENT` and nothing is booked
- **VERIFIED BY**: `team-payment/internal/rpc/seller_fee_test.go` `TestChargeSellerFeeValidation` cases "A missing reference is rejected", "A missing seller is rejected"

#### Scenario: A reference of 65 characters is rejected and 64 is accepted

- **WHEN** a service principal calls `ChargeSellerFee` with a 65-character `reference_id`, then with a 64-character one
- **THEN** the first fails with `INVALID_ARGUMENT` and the second returns `created=true`
- **VERIFIED BY**: `team-payment/internal/rpc/seller_fee_test.go` `TestChargeSellerFeeValidation` case "A reference of 65 characters is rejected" and subtest "A reference of 64 characters is accepted"

### Requirement: Only a service principal may charge a seller fee

`ChargeSellerFee` SHALL be callable only by a principal of type `service`. A user principal (seller, buyer or admin) and
an anonymous caller SHALL be refused, and `team-gateway` SHALL NOT route the RPC.

#### Scenario: A seller cannot charge a fee

- **WHEN** a seller user principal calls `ChargeSellerFee` for their own `seller_id`
- **THEN** the call fails with `PERMISSION_DENIED` and nothing is booked
- **VERIFIED BY**: `team-payment/internal/rpc/seller_fee_test.go` `TestChargeSellerFeeAuthz` case "A seller cannot charge a fee"

#### Scenario: An admin cannot charge a fee

- **WHEN** an admin user principal calls `ChargeSellerFee` for another seller
- **THEN** the call fails with `PERMISSION_DENIED` and nothing is booked
- **VERIFIED BY**: `team-payment/internal/rpc/seller_fee_test.go` `TestChargeSellerFeeAuthz` case "An admin cannot charge a fee"

#### Scenario: An unauthenticated call is rejected

- **WHEN** a call arrives without a principal
- **THEN** the call fails with `UNAUTHENTICATED` and nothing is booked
- **VERIFIED BY**: `team-payment/internal/rpc/seller_fee_test.go` `TestChargeSellerFeeAuthz` case "An unauthenticated call is rejected"

#### Scenario: The gateway does not expose the fee RPC

- **WHEN** a client calls the public gateway path for `ChargeSellerFee` with a valid seller token
- **THEN** the gateway answers 404 and the seller's ledger is unchanged
- **VERIFIED BY**: platform-e2e `payment/seller_fee_charge.feature::The gateway does not expose the fee RPC`

### Requirement: The ledger store keeps fee rows unique and sign-consistent

The `wallet_ledger` constraints SHALL allow a `SUBSCRIPTION_FEE` row only with `amount < 0` and a non-null
`reference_id`, SHALL keep `(type, reference_id)` unique for it, and SHALL keep accepting every ledger type and sign the
earlier migrations accepted. The reserved `AD_SPEND` type SHALL NOT be accepted by the constraint in v1.

#### Scenario: The store refuses a positive fee row

- **WHEN** a `SUBSCRIPTION_FEE` row with amount +1000 is inserted directly
- **THEN** the database rejects it with a check violation and no row exists
- **VERIFIED BY**: `team-payment/internal/store/ledger_fee_integration_test.go` `TestSubscriptionFeeConstraints` subtest "refuses a positive fee row"

#### Scenario: The store refuses a fee row without a reference

- **WHEN** a `SUBSCRIPTION_FEE` row with amount -1000 and a null `reference_id` is inserted directly
- **THEN** the database rejects it with a check violation and no row exists
- **VERIFIED BY**: `team-payment/internal/store/ledger_fee_integration_test.go` `TestSubscriptionFeeConstraints` subtest "refuses a fee row without a reference"

#### Scenario: The store refuses a second fee row for one reference

- **WHEN** two `SUBSCRIPTION_FEE` rows with the same `reference_id` are inserted
- **THEN** the second insert is rejected by the unique index and the seller still has one row
- **VERIFIED BY**: `team-payment/internal/store/ledger_fee_integration_test.go` `TestSubscriptionFeeConstraints` subtest "refuses a second fee row for one reference"

#### Scenario: Existing ledger types still migrate cleanly

- **WHEN** the migration is applied to a database holding ORDER_SETTLEMENT, REFUND_DEDUCTION and PAYOUT rows
- **THEN** it succeeds without rewriting those rows and the ledger sum is unchanged
- **VERIFIED BY**: `team-payment/internal/store/ledger_fee_integration_test.go` `TestSubscriptionFeeMigration_ExistingTypesStillMigrate`
