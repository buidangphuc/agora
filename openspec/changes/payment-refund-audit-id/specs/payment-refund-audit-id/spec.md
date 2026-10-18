## ADDED Requirements

### Requirement: Admin refund audit event carries the refund id

When an admin calls `PaymentService.RefundPayment`, the `payment.admin_refund` audit event SHALL include metadata `refund_id` equal to the request's `refund_id`, in addition to `order_id` and `amount`. The event's action, `target_type=payment`, `target_id` (payment id) and `actor_id` SHALL be unchanged, and the refund `reason` SHALL NOT be copied. The audit write SHALL still happen before the refund and SHALL still fail closed.

#### Scenario: Refund audit id: Admin refund event carries refund_id
- **WHEN** admin "adm-1" refunds 200000 on payment "p-1" (order "o-1") with refund_id "rf-1"
- **THEN** exactly one WriteAuditEvent call is made with action `payment.admin_refund`, target_type `payment`, target_id "p-1", actor_id "adm-1" and metadata `refund_id`="rf-1", `order_id`="o-1", `amount`="200000"
- **VERIFIED BY**: team-payment `internal/handler` `TestAdminRefundAuditCarriesRefundID`

#### Scenario: Refund audit id: Audit client puts refund_id in metadata
- **WHEN** `AuditClient.WriteAdminRefund` is called with refund id "rf-1"
- **THEN** the outgoing request metadata has exactly the keys `refund_id`, `order_id`, `amount` and the principal is still the service principal with scope `audit.write`
- **VERIFIED BY**: team-payment `internal/upstream` `TestAuditClientRefundIDDistinctAndVerbatim`

#### Scenario: Refund audit id: Retry with the same refund_id is distinguishable
- **WHEN** an admin refund with refund_id "rf-1" fails after the audit write and the admin retries with the same refund_id "rf-1"
- **THEN** the money moves once, and both audit events carry `refund_id`="rf-1" while an admin refund with refund_id "rf-2" on the same payment produces an event with `refund_id`="rf-2"
- **VERIFIED BY**: team-payment `internal/handler` `TestAdminRefundRetryKeepsRefundID`

#### Scenario: Refund audit id: Audit is still written before the refund
- **WHEN** an admin refunds with refund_id "rf-1"
- **THEN** the recorded call order is the audit write, then the ledger refund
- **VERIFIED BY**: team-payment `internal/handler` `TestAdminRefundAuditBeforeRefund`

#### Scenario: Refund audit id: Invalid refund_id writes no event
- **WHEN** an admin calls RefundPayment with refund_id "" or one of 65 characters or containing a space
- **THEN** the call returns InvalidArgument and zero audit events are written
- **VERIFIED BY**: team-payment `internal/handler` `TestAdminRefundInvalidRefundIDWritesNoEvent`

#### Scenario: Refund audit id: Boundary 64-character refund_id is recorded intact
- **WHEN** an admin refunds with a 64-character valid refund_id
- **THEN** the audit metadata `refund_id` equals that value unchanged (no truncation)
- **VERIFIED BY**: team-payment `internal/handler` `TestAdminRefundAuditRefundIDMaxLength`

#### Scenario: Refund audit id: Audit failure still refuses the refund
- **WHEN** the audit write fails or times out
- **THEN** the call returns Unavailable and the ledger is unchanged
- **VERIFIED BY**: team-payment `internal/handler` `TestAdminRefundAuditErrorLeavesLedgerUnchanged`

#### Scenario: Refund audit id: Seller refund writes no audit event
- **WHEN** the order's seller (non-admin) refunds with refund_id "rf-1"
- **THEN** the refund succeeds and zero audit events are written; a non-seller non-admin gets PermissionDenied with zero events
- **VERIFIED BY**: team-payment `internal/handler` `TestSellerRefundWritesNoAuditEvent`

#### Scenario: Refund audit id: Admin refund audit row exposes refund_id end to end
- **WHEN** an admin refunds 200000 of a PAID payment through the gateway with refund_id "rf-e2e-1" and then queries the audit log by target_type `payment` and the payment id (admin QueryAuditLog)
- **THEN** exactly one event with action `payment.admin_refund` is returned whose metadata `refund_id`="rf-e2e-1" and `amount`="200000"

#### Scenario: Refund audit id: Replayed refund_id is traceable end to end
- **WHEN** an admin refunds with refund_id "rf-e2e-2", repeats the same call, then refunds again with refund_id "rf-e2e-3", and queries the audit log for that payment
- **THEN** every returned `payment.admin_refund` event has a `refund_id` in {"rf-e2e-2","rf-e2e-3"}, the payment's refunded total counts "rf-e2e-2" once, and no event lacks `refund_id`

#### Scenario: Refund audit id: Non-admin cannot read the refund audit row end to end
- **WHEN** a buyer calls QueryAuditLog for the payment's target
- **THEN** the call is denied (PermissionDenied) and no metadata is returned
