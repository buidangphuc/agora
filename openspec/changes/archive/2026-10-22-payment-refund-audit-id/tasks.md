No proto, migration, gateway or compose change. Existing e2e support (`platform-e2e/tests/e2e/support/audit_support.py`, from `audit-query-filters`) queries events by `target_type` + `target_id` as admin. Existing feature `payment/admin_refund_audit.feature` already covers the event without `refund_id`; the e2e track extends it.

## Code track (team-payment)

- [x] 1. Audit client. Write-set: `team-payment/internal/upstream/audit.go`, `internal/upstream/audit_test.go`. `WriteAdminRefund(ctx, actorID, paymentID, orderID, refundID string, amount int64)`; metadata gains `refund_id`. Scenarios: Refund audit id: Audit client puts refund_id in metadata.
- [x] 2. Handler. Write-set: `team-payment/internal/handler/payment.go` (`AuditWriter` interface and the call at the admin branch pass `req.GetRefundId()`), `internal/handler/*_test.go` (update fakes; add tests). Scenarios: Refund audit id: Admin refund event carries refund_id, Refund audit id: Retry with the same refund_id is distinguishable, Refund audit id: Audit is still written before the refund, Refund audit id: Invalid refund_id writes no event, Refund audit id: Boundary 64-character refund_id is recorded intact, Refund audit id: Audit failure still refuses the refund, Refund audit id: Seller refund writes no audit event.

## E2E track (platform-e2e)

- [x] 3. Extend `platform-e2e/tests/e2e/features/payment/admin_refund_audit.feature` and its bindings: send a known refund_id, read the audit row via the admin QueryAuditLog helper and assert `refund_id`; replay and second-refund scenario; buyer denied on QueryAuditLog. Scenarios: Refund audit id: Admin refund audit row exposes refund_id end to end, Refund audit id: Replayed refund_id is traceable end to end, Refund audit id: Non-admin cannot read the refund audit row end to end.
- [x] 4. `team-payment/FEATURES.yaml` entries are written by the e2e-author, not by this slice. Scenarios: all end-to-end scenarios above.
