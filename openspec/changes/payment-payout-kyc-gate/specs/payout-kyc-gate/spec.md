## ADDED Requirements

### Requirement: Payouts require a VERIFIED seller

`RequestPayout` and `RequestWalletPayout` SHALL read the caller's KYC status from `VerificationService.GetVerificationStatus` and SHALL proceed only when the status is `VERIFIED`; for any other status they SHALL fail `FAILED_PRECONDITION` with message "seller verification required" before any ledger or payout row is written.

#### Scenario: Verified seller requests a bank payout
- **WHEN** a seller whose status is `VERIFIED` and whose available balance covers the amount calls `RequestPayout`
- **THEN** the call succeeds and `ListPayoutHistory` returns the new payout with the requested amount
- **VERIFIED BY**: platform-e2e/tests/e2e/features/payment/payout_kyc_gate.feature :: Verified seller requests a bank payout

#### Scenario: Verified seller requests a wallet payout
- **WHEN** a seller whose status is `VERIFIED` and whose available balance covers the amount calls `RequestWalletPayout`
- **THEN** the call succeeds and `ListLedgerEntries` shows one pending payout entry of minus the requested amount
- **VERIFIED BY**: platform-e2e/tests/e2e/features/payment/payout_kyc_gate.feature :: Verified seller requests a wallet payout

#### Scenario: Seller who never submitted is refused
- **WHEN** a seller with status `NOT_SUBMITTED` and a positive balance calls `RequestPayout`
- **THEN** the call fails `FAILED_PRECONDITION` with message "seller verification required", `ListPayoutHistory` returns 0 payouts and the wallet balance is unchanged
- **VERIFIED BY**: platform-e2e/tests/e2e/features/payment/payout_kyc_gate.feature :: Seller who never submitted is refused

#### Scenario: Seller with a pending submission is refused
- **WHEN** a seller whose latest submission is `PENDING` calls `RequestWalletPayout`
- **THEN** the call fails `FAILED_PRECONDITION` and `ListLedgerEntries` contains 0 new entries
- **VERIFIED BY**: platform-e2e/tests/e2e/features/payment/payout_kyc_gate.feature :: Seller with a pending submission is refused

#### Scenario: Seller with a rejected submission is refused
- **WHEN** a seller whose latest submission is `REJECTED` calls `RequestPayout`
- **THEN** the call fails `FAILED_PRECONDITION` and the wallet balance is unchanged
- **VERIFIED BY**: platform-e2e/tests/e2e/features/payment/payout_kyc_gate.feature :: Seller with a rejected submission is refused

#### Scenario: Seller who loses verification is refused on the next call
- **WHEN** a seller who completed one payout has a newer submission that is not `VERIFIED` and calls `RequestPayout` again
- **THEN** the second call fails `FAILED_PRECONDITION` and `ListPayoutHistory` still returns exactly 1 payout
- **VERIFIED BY**: platform-e2e/tests/e2e/features/payment/payout_kyc_gate.feature :: Seller who loses verification is refused on the next call

### Requirement: The gate fails closed when verification cannot be read

team-payment SHALL call team-verification with service principal `service-team-payment` and scope `verification.read`, one attempt and a 2 s deadline, and SHALL fail `UNAVAILABLE` when the status cannot be read, writing no ledger or payout row.

#### Scenario: Verification service unreachable
- **WHEN** team-verification is stopped and a `VERIFIED` seller calls `RequestPayout`
- **THEN** the call fails `UNAVAILABLE` within 3 seconds and `ListPayoutHistory` returns 0 new payouts
- **VERIFIED BY**: platform-e2e/tests/e2e/features/payment/payout_kyc_gate.feature :: Verification service unreachable

#### Scenario: Verification call exceeds the deadline
- **WHEN** team-verification answers `GetVerificationStatus` after 5 seconds
- **THEN** the payout call fails `UNAVAILABLE` after at most 2.5 seconds and the call is attempted exactly once
- **VERIFIED BY**: team-payment/internal/upstream/verification_test.go :: TestPayoutGateSlowServerUnavailableOneAttempt

#### Scenario: Verification rejects the service principal
- **WHEN** team-verification answers `PermissionDenied` to the status call
- **THEN** the payout call fails `UNAVAILABLE` with message "verification unavailable" and not `FAILED_PRECONDITION`
- **VERIFIED BY**: team-payment/internal/upstream/verification_test.go :: TestPayoutGatePermissionDeniedIsUnavailable

#### Scenario: Gate recovers after the outage
- **WHEN** team-verification is started again and the `VERIFIED` seller repeats the payout
- **THEN** the call succeeds with no restart of team-payment
- **VERIFIED BY**: platform-e2e/tests/e2e/features/payment/payout_kyc_gate.feature :: Gate recovers after the outage

### Requirement: Existing payout authorization stays in front of the gate

The gate SHALL run only after the principal, `listing.write` scope, ownership and input checks pass, so authorization and validation errors keep their current codes and never reach team-verification.

#### Scenario: Invalid amount is rejected before the gate
- **WHEN** a seller calls `RequestPayout` with `amount` 0
- **THEN** the call fails `INVALID_ARGUMENT` and team-verification receives 0 status calls
- **VERIFIED BY**: platform-e2e/tests/e2e/features/payment/payout_kyc_gate.feature :: Invalid amount is rejected before the gate

#### Scenario: Another seller's wallet is denied before the gate
- **WHEN** a buyer calls `RequestPayout` with another seller's `seller_id`
- **THEN** the call fails `PERMISSION_DENIED` and team-verification receives 0 status calls
- **VERIFIED BY**: platform-e2e/tests/e2e/features/payment/payout_kyc_gate.feature :: Another seller's wallet is denied before the gate

#### Scenario: Missing listing write scope is denied before the gate
- **WHEN** a user token without `listing.write` calls `RequestWalletPayout`
- **THEN** the call fails `PERMISSION_DENIED` and team-verification receives 0 status calls
- **VERIFIED BY**: platform-e2e/tests/e2e/features/payment/payout_kyc_gate.feature :: Missing listing write scope is denied before the gate

#### Scenario: Status comes from the principal not the request
- **WHEN** an unverified seller calls `RequestPayout` with `seller_id` set to a verified seller's id
- **THEN** the call fails `PERMISSION_DENIED` and no payout exists for either seller
- **VERIFIED BY**: platform-e2e/tests/e2e/features/payment/payout_kyc_gate.feature :: Status comes from the principal not the request

### Requirement: Verification address is required configuration

team-payment SHALL require `UPSTREAM_VERIFICATION_ADDR` in every ENV and SHALL refuse to boot without it.

#### Scenario: Boot fails without the verification address
- **WHEN** team-payment starts with `UPSTREAM_VERIFICATION_ADDR` unset or blank
- **THEN** config loading fails with an error naming `UPSTREAM_VERIFICATION_ADDR` and the process exits non-zero
- **VERIFIED BY**: platform-e2e/tests/e2e/features/payment/payout_kyc_gate.feature :: Boot fails without the verification address

### Requirement: Demo and e2e sellers are verified through real RPCs

The seed and the e2e payout flows SHALL make a seller `VERIFIED` only through `CreateKycUploadUrl`, a presigned PUT, `SubmitKyc` and an admin `ReviewKyc` approve, with no fake service, no bypass flag and no grace period.

#### Scenario: Seeded demo seller can pay out
- **WHEN** the marketplace seed has run and a seeded shop seller with a settled balance calls `RequestPayout`
- **THEN** the call succeeds and `GetVerificationStatus` for that seller returns `VERIFIED`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/payment/payout_kyc_gate.feature :: Seeded demo seller can pay out

#### Scenario: Freshly registered seller is blocked at rollout
- **WHEN** a seller registered after the seed and without a submission calls `RequestWalletPayout`
- **THEN** the call fails `FAILED_PRECONDITION` and `GetVerificationStatus` returns `NOT_SUBMITTED`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/payment/payout_kyc_gate.feature :: Freshly registered seller is blocked at rollout
