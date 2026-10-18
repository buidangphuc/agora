# kyc-audit Specification

## Purpose
KYC review decisions in team-verification are audited before they are applied and fail closed when audit is unavailable.

## Requirements

### Requirement: KYC reviews are audited before they are applied

`VerificationService.ReviewKyc` SHALL, after the authentication check, the `admin` scope check, loading the submission, the self-review check, decision validation and the PENDING check, write exactly one team-audit event and only then apply the review. The step order is: authenticate, `admin` scope, empty-id check, load submission, self-review check, decision validation, PENDING check, audit write, apply. The event SHALL have `action=kyc.review`, `target_type=kyc_submission`, `target_id` equal to the submission id, `actor_id` equal to the admin's user id from the verified Principal (never from the request), and `metadata` keys `decision` (normalized to `approve` or `reject`) and `user_id` (the submission's owner). No other metadata key SHALL be written. A call refused before the audit write SHALL write no event.

#### Scenario: Admin approval writes one audit event
- **WHEN** admin "adm-1" approves PENDING submission "kyc_1" owned by "u-1"
- **THEN** ReviewKyc returns VERIFIED and exactly one WriteAuditEvent call is made with actor_id "adm-1", action `kyc.review`, target_type `kyc_submission`, target_id "kyc_1" and metadata `decision`="approve", `user_id`="u-1"
- **VERIFIED BY**: team-verification `internal/handler` `TestReviewKycWritesAuditEvent`

#### Scenario: Decision alias is normalized in the event
- **WHEN** an admin reviews a submission with decision " Rejected " (alias, padded, mixed case)
- **THEN** the submission becomes REJECTED and the audit metadata `decision` is exactly "reject"
- **VERIFIED BY**: team-verification `internal/handler` `TestReviewKycAuditDecisionNormalized`

#### Scenario: Audit is written before the review is applied
- **WHEN** ReviewKyc runs for an admin
- **THEN** the recorded call order is audit write, then repository status update
- **VERIFIED BY**: team-verification `internal/handler` `TestReviewKycAuditBeforeUpdate`

#### Scenario: Actor comes from the principal, not the request
- **WHEN** admin "adm-1" calls ReviewKyc for a submission
- **THEN** the audit event actor_id is "adm-1" and the audit metadata holds no `doc_type` or `doc_ref`
- **VERIFIED BY**: team-verification `internal/handler` `TestReviewKycAuditActorAndNoDocumentData`

#### Scenario: Non-admin caller is denied without an audit write
- **WHEN** a caller without the `admin` scope (403 at the edge, PermissionDenied at the service) calls ReviewKyc
- **THEN** the call is refused, zero audit calls are made and the submission is unchanged
- **VERIFIED BY**: team-verification `internal/handler` `TestReviewKycNonAdminNoAudit`

#### Scenario: Audit call uses the service principal
- **WHEN** the audit client sends WriteAuditEvent
- **THEN** the outgoing metadata principal is id `service-team-verification`, type `service`, scopes `audit.write`, and the admin's own principal metadata is absent
- **VERIFIED BY**: team-verification `internal/upstream` `TestAuditClientUsesServicePrincipal`

#### Scenario: Audited approve is visible end to end
- **WHEN** an admin approves a seller's pending KYC through the gateway
- **THEN** the gateway returns 200, the seller's status is verified, and QueryAuditLog as admin for target_type `kyc_submission` and the submission id returns exactly 1 event with action `kyc.review`, the admin's user id as actor, and metadata `decision`="approve" and `user_id`=the seller's id
- **VERIFIED BY**: platform-e2e `features/services/verification_kyc_audit.feature` "An audited KYC approval leaves one audit event"

#### Scenario: Audited reject is visible end to end
- **WHEN** an admin rejects a seller's pending KYC through the gateway
- **THEN** the gateway returns 200, the seller's status is rejected (not verified), and QueryAuditLog for that submission returns exactly 1 `kyc.review` event with metadata `decision`="reject"
- **VERIFIED BY**: platform-e2e `features/services/verification_kyc_audit.feature` "An audited KYC rejection leaves one audit event"

### Requirement: KYC review fails closed when audit is unavailable

If the audit write returns an error, times out (default 2 s, `UPSTREAM_CALL_TIMEOUT_SECONDS`), or no audit client is configured (`UPSTREAM_AUDIT_ADDR` unset), `ReviewKyc` SHALL return `Unavailable` and SHALL NOT change the submission (status stays PENDING, `reviewed_at` unset). The audit call SHALL be a single attempt with no retry. Calls that never reach the audit write are not affected by team-audit availability.

#### Scenario: Audit error leaves the submission pending
- **WHEN** team-audit returns an error during an admin's ReviewKyc
- **THEN** the call returns `Unavailable` and GetVerificationStatus for the owner is still PENDING with no badge
- **VERIFIED BY**: team-verification `internal/handler` `TestReviewKycAuditErrorUnavailableNoChange`

#### Scenario: Audit client call is bounded by the timeout
- **WHEN** team-audit does not answer and the client timeout is 100 ms
- **THEN** WriteKycReviewEvent returns an error within 1 s
- **VERIFIED BY**: team-verification `internal/upstream` `TestAuditClientTimeoutBounded`

#### Scenario: Audit timeout error is Unavailable
- **WHEN** the audit writer returns a deadline-exceeded error during an admin's ReviewKyc
- **THEN** the call returns `Unavailable` and the submission is still PENDING
- **VERIFIED BY**: team-verification `internal/handler` `TestReviewKycAuditTimeoutUnavailableNoChange`

#### Scenario: KYC audit: no audit upstream configured
- **WHEN** ReviewKyc runs with a nil AuditWriter (`UPSTREAM_AUDIT_ADDR` unset)
- **THEN** the call returns `Unavailable` and the submission stays PENDING
- **VERIFIED BY**: team-verification `internal/handler` `TestReviewKycNilAuditUnavailable`

#### Scenario: Empty audit address yields no client
- **WHEN** NewAuditClient is called with an empty address
- **THEN** it returns a nil client and a nil error
- **VERIFIED BY**: team-verification `internal/upstream` `TestNewAuditClientEmptyAddrNil`

#### Scenario: Non-positive call timeout is rejected
- **WHEN** config loads with `UPSTREAM_CALL_TIMEOUT_SECONDS` of 0 and then of -1
- **THEN** both are rejected with a validation error
- **VERIFIED BY**: team-verification `internal/config` `TestCallTimeoutMustBePositive`

#### Scenario: Audit down blocks approval end to end
- **WHEN** team-audit is stopped and an admin approves a seller's pending KYC through the gateway (tagged @destructive)
- **THEN** the gateway returns HTTP 503 with Connect code `unavailable` and the seller's status is still not verified; after team-audit is started again the same approval returns 200 and exactly 1 `kyc.review` event exists for the submission
- **VERIFIED BY**: platform-e2e `features/services/verification_kyc_audit.feature` "KYC approval is refused while the audit service is down"

### Requirement: Refused reviews write no audit event

ReviewKyc SHALL write no audit event when the call is refused before the audit step: unauthenticated or anonymous caller (Unauthenticated), caller without the `admin` scope (PermissionDenied), unknown or empty submission id (NotFound / InvalidArgument), reviewer is the submission's owner (PermissionDenied), unrecognised decision (InvalidArgument), or a submission that is no longer PENDING (FailedPrecondition). Authorization SHALL be checked before the submission is loaded.

#### Scenario: Non-admin is denied and nothing is written
- **WHEN** a buyer without the `admin` scope calls ReviewKyc on a pending submission
- **THEN** the call returns `PermissionDenied`, zero audit calls are made and the submission stays PENDING
- **VERIFIED BY**: team-verification `internal/handler` `TestReviewKycNonAdminNoAudit`

#### Scenario: Anonymous and identity-less callers are refused
- **WHEN** an anonymous principal, and separately a principal with no identity, call ReviewKyc
- **THEN** both return `Unauthenticated` and zero audit calls are made
- **VERIFIED BY**: team-verification `internal/handler` `TestReviewKycAnonymousNoAudit`

#### Scenario: Admin cannot review their own submission
- **WHEN** an admin reviews a submission they submitted
- **THEN** the call returns `PermissionDenied` and zero audit calls are made
- **VERIFIED BY**: team-verification `internal/handler` `TestReviewKycSelfReviewNoAudit`

#### Scenario: Unknown submission
- **WHEN** an admin reviews submission id "kyc_missing"
- **THEN** the call returns `NotFound` and zero audit calls are made
- **VERIFIED BY**: team-verification `internal/handler` `TestReviewKycNotFoundNoAudit`

#### Scenario: Empty submission id is rejected
- **WHEN** an admin calls ReviewKyc with an empty submission id
- **THEN** the call returns `InvalidArgument` and zero audit calls are made
- **VERIFIED BY**: team-verification `internal/handler` `TestReviewKycEmptyIDNoAudit`

#### Scenario: Invalid decision is rejected before audit
- **WHEN** an admin calls ReviewKyc with decision "maybe" and then with decision ""
- **THEN** both return `InvalidArgument`, zero audit calls are made and the submission stays PENDING
- **VERIFIED BY**: team-verification `internal/handler` `TestReviewKycInvalidDecisionNoAudit`

#### Scenario: Already reviewed submission is refused before audit
- **WHEN** an admin reviews a submission that is already VERIFIED
- **THEN** the call returns `FailedPrecondition`, zero audit calls are made and the status is unchanged
- **VERIFIED BY**: team-verification `internal/handler` `TestReviewKycAlreadyReviewedNoAudit`

#### Scenario: Non-admin approval writes no event end to end
- **WHEN** a buyer approves their own pending KYC through the gateway
- **THEN** the gateway returns 403 and QueryAuditLog as admin for that submission id returns 0 events
- **VERIFIED BY**: platform-e2e `features/services/verification_kyc_audit.feature` "A refused KYC review writes no audit event"

### Requirement: Failure after audit is retryable

If the audit write succeeds and the repository update then fails, ReviewKyc SHALL return `Internal`; a retry with a healthy repository SHALL succeed. The trail MAY hold two `kyc.review` events for the submission (at-least-once; no idempotency key).

#### Scenario: Update failure after audit then retry
- **WHEN** the audit write succeeds, the repository update fails, and the admin retries with the repository healthy
- **THEN** the first call returns `Internal`, the retry returns VERIFIED, and the fake audit recorded 2 `kyc.review` events for the submission
- **VERIFIED BY**: team-verification `internal/handler` `TestReviewKycUpdateFailureInternalThenRetry`

### Requirement: Other KYC RPCs and configuration are unaffected

SubmitKyc and GetVerificationStatus SHALL make no team-audit call and SHALL behave identically with team-audit down or unconfigured. `UPSTREAM_AUDIT_ADDR` and `UPSTREAM_CALL_TIMEOUT_SECONDS` SHALL be read by `internal/config` and documented in `.env.example` and the README, and `cmd/server/main.go` SHALL wire the audit writer only when the client is non-nil.

#### Scenario: Submit and status work with audit down
- **WHEN** team-audit is unavailable (nil writer or failing fake) and a buyer calls SubmitKyc then GetVerificationStatus
- **THEN** both succeed, status is PENDING, and zero audit calls are made
- **VERIFIED BY**: team-verification `internal/handler` `TestSubmitAndStatusIndependentOfAudit`

#### Scenario: Config defaults and overrides
- **WHEN** config loads with no environment, then with `UPSTREAM_AUDIT_ADDR=team-audit-svc:50066` and `UPSTREAM_CALL_TIMEOUT_SECONDS=1.5`
- **THEN** the first has an empty audit address and a 2 s timeout, the second has that address and a 1.5 s timeout
- **VERIFIED BY**: team-verification `internal/config` `TestAuditConfigDefaultsAndOverrides`
