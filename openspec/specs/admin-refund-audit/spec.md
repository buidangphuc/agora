# admin-refund-audit Specification

## Purpose
Admin refunds in team-payment are written to the audit log before they are applied, fail closed when audit is unavailable, and never double-refund on retry.

## Requirements

### Requirement: Admin refunds are audited before they are applied

When `PaymentService.RefundPayment` is called by a principal holding the `admin` scope, the system SHALL write one team-audit event with `action=payment.admin_refund`, `target_type=payment`, `target_id` equal to the payment id, `actor_id` equal to the caller's id and `metadata` holding `order_id` and `amount`, and only then apply the refund. Calls that are refused before the audit write SHALL write no event.

#### Scenario: Admin refund writes one audit event and refunds
- **WHEN** an admin with user id "adm-1" refunds 200000 of a PAID payment "P-1" of order "O-1" with a fresh refund id
- **THEN** the call returns success, the payment reads PARTIALLY_REFUNDED with refunded amount 200000, and exactly one audit event exists with action `payment.admin_refund`, target_type `payment`, target_id "P-1", actor_id "adm-1" and metadata `order_id`="O-1", `amount`="200000"
- **VERIFIED BY**: team-payment `internal/handler` `TestAdminRefundWritesAuditEvent`

#### Scenario: Audit is written before the refund
- **WHEN** an admin refund runs
- **THEN** the recorded call order is the audit write first and the refund second
- **VERIFIED BY**: team-payment `internal/handler` `TestAdminRefundAuditBeforeRefund`

#### Scenario: Admin refund is audited end to end
- **WHEN** an admin refunds 200000 of a PAID payment through the gateway
- **THEN** the call succeeds and QueryAuditLog by target_type `payment` and the payment id returns exactly one event with action `payment.admin_refund`, the admin's user id as actor and metadata `amount`="200000"
- **VERIFIED BY**: platform-e2e `features/payment/admin_refund_audit.feature` `Admin refund is audited`

#### Scenario: Admin who is the order's seller is audited
- **WHEN** a user holding the `admin` scope who is also the seller of the order refunds 200000 of a PAID payment
- **THEN** the call returns success and exactly one audit event exists with action `payment.admin_refund` and that user's id as actor
- **VERIFIED BY**: team-payment `internal/handler` `TestAdminWhoIsSellerRefundIsAudited`

#### Scenario: Payment audit: Audit is called as the service principal
- **WHEN** the audit client sends WriteAuditEvent
- **THEN** the outgoing principal metadata is id `service-team-payment`, type `service`, scopes `audit.write`, and the caller's own principal metadata is absent
- **VERIFIED BY**: team-payment `internal/upstream` `TestAuditClientUsesServicePrincipal`

#### Scenario: Reason is not copied into the audit event
- **WHEN** an admin refunds with reason "buyer complaint 123"
- **THEN** the audit event metadata keys are exactly `order_id` and `amount`
- **VERIFIED BY**: team-payment `internal/handler` `TestAdminRefundAuditMetadataKeys`

#### Scenario: Refusal before the audit write writes no event
- **WHEN** an admin calls RefundPayment with an empty payment id, an invalid refund id, a non-positive amount, or an unknown payment id
- **THEN** the call returns `InvalidArgument` (first three) or `NotFound` (last) and no audit event is written
- **VERIFIED BY**: team-payment `internal/handler` `TestAdminRefundRefusedBeforeAuditWritesNothing`

#### Scenario: Unauthenticated caller writes no event
- **WHEN** a call to RefundPayment carries no principal
- **THEN** the call returns `Unauthenticated` and no audit event is written
- **VERIFIED BY**: team-payment `internal/handler` `TestRefundPaymentAnonymousNoAudit`

### Requirement: Admin refunds fail closed when audit is unavailable

If the audit event cannot be written (team-audit returns an error, times out, or `UPSTREAM_AUDIT_ADDR` is unset), an admin `RefundPayment` SHALL return `Unavailable` and the payment status, refund rows and ledger SHALL be unchanged.

#### Scenario: Audit error leaves the ledger unchanged
- **WHEN** team-audit returns an error during an admin refund of a PAID payment
- **THEN** the call returns `Unavailable`, the payment still reads PAID with refunded amount 0 and 0 refunds, and the seller's REFUND_DEDUCTION entries are 0
- **VERIFIED BY**: team-payment `internal/handler` `TestAdminRefundAuditErrorFailsClosed`

#### Scenario: Audit timeout is bounded and refuses the refund
- **WHEN** team-audit does not answer within the upstream timeout of 2 s during an admin refund
- **THEN** the call returns `Unavailable` within 3 s and no refund is applied
- **VERIFIED BY**: team-payment `internal/upstream` `TestAuditClientTimeoutBounded`; team-payment `internal/handler` `TestAdminRefundAuditTimeoutFailsClosed`

#### Scenario: Payment audit: no audit upstream configured
- **WHEN** an admin refunds while `UPSTREAM_AUDIT_ADDR` is unset
- **THEN** the call returns `Unavailable` and no refund is applied
- **VERIFIED BY**: team-payment `internal/upstream` `TestNewAuditClientEmptyAddrIsNil`; team-payment `internal/handler` `TestAdminRefundNilAuditWriterUnavailable`

#### Scenario: Audit down refuses the admin refund end to end
- **WHEN** team-audit is stopped and an admin refunds 200000 of a PAID payment through the gateway (the gateway forwards writes once, no retry)
- **THEN** the call fails with connect code `unavailable`, and after team-audit is started again the payment read by the buyer reads PAID with refunded amount 0 and `assert_no_event` for the payment passes
- **VERIFIED BY**: platform-e2e `features/payment/admin_refund_audit.feature` `Admin refund is refused while audit is down`

### Requirement: A retried admin refund never double-refunds

The refund SHALL remain idempotent by `refund_id` and capped by the refundable remainder on the admin path, including after a failure that follows a successful audit write.

#### Scenario: Retry after audit success and refund failure refunds once
- **WHEN** an admin refund of 200000 with refund id "R-1" gets a successful audit write and then a failing ledger update (`Internal`), and the same request is retried with the ledger healthy
- **THEN** the first call returns `Internal`, the retry returns success, the payment reads refunded amount 200000 with 1 refund, the seller's REFUND_DEDUCTION entries are exactly -200000, and the audit trail holds 2 `payment.admin_refund` events for the payment
- **VERIFIED BY**: team-payment `internal/handler` `TestAdminRefundRetryAfterFailureRefundsOnce`

#### Scenario: Replay of an applied refund id does not move money again
- **WHEN** an admin sends the same refund id and amount twice after the first succeeded
- **THEN** both calls return success with the same refund, the payment reads refunded amount 200000 with 1 refund, and the seller's REFUND_DEDUCTION entries are exactly -200000
- **VERIFIED BY**: team-payment `internal/handler` `TestAdminRefundReplayIsIdempotent`

#### Scenario: Refund above the remainder is refused on the admin path
- **WHEN** an admin refunds 300001 of a payment with 300000 refundable remaining
- **THEN** the call returns `FailedPrecondition` and the payment's refunded amount is unchanged
- **VERIFIED BY**: team-payment `internal/handler` `TestAdminRefundExceedsRemainderRefused`

### Requirement: Seller refunds are not audited and not affected by audit availability

A refund by the order's seller (no `admin` scope) SHALL make no audit call and SHALL NOT depend on team-audit. A caller who is neither the seller nor an admin SHALL be denied with `PermissionDenied` and no event.

#### Scenario: Seller refund writes no audit event
- **WHEN** the order's seller refunds 200000 of a PAID payment
- **THEN** the call returns success, the payment reads PARTIALLY_REFUNDED, and zero audit events exist for the payment
- **VERIFIED BY**: team-payment `internal/handler` `TestSellerRefundWritesNoAudit`; platform-e2e `features/payment/admin_refund_audit.feature` `Seller refund is not audited`

#### Scenario: Seller refund succeeds while audit is down
- **WHEN** team-audit errors and the order's seller refunds 200000 of a PAID payment
- **THEN** the call returns success and the payment reads PARTIALLY_REFUNDED with refunded amount 200000
- **VERIFIED BY**: team-payment `internal/handler` `TestSellerRefundIgnoresAuditOutage`

#### Scenario: Non-seller non-admin is denied without an event
- **WHEN** a USER principal that is not the order's seller and has no admin scope calls RefundPayment
- **THEN** the call returns `PermissionDenied`, no refund is applied and no audit event is written
- **VERIFIED BY**: team-payment `internal/handler` `TestRefundPaymentNonSellerDeniedNoAudit`

#### Scenario: Non-admin buyer cannot trigger an admin audit
- **WHEN** the buyer of the payment calls RefundPayment
- **THEN** the call fails with connect code `permission_denied`, the payment is unchanged and `assert_no_event` for the payment passes
- **VERIFIED BY**: platform-e2e `features/payment/admin_refund_audit.feature` `Buyer cannot refund and leaves no audit event`
