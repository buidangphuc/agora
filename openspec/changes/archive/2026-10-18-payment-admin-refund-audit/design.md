# Design

## Which calls are "admin path"

The current handler checks `slices.Contains(principal.GetScopes(), "admin")` and, when true, skips the order-seller lookup. The rule is uniform: every call from a principal holding `admin` is audited, including an admin who is also the order's seller, and a caller without `admin` is never audited. No seller lookup is added to the admin path.

## Order of operations

1. Existing validation and `FindTransaction` (refuse with `InvalidArgument` / `NotFound`; no audit).
2. Non-admin: existing seller check; no audit.
3. Admin: `AuditWriter.WriteAdminRefund`; error or nil writer gives `Unavailable`.
4. `svc.RefundPayment` (idempotent by `refund_id`, capped by the remainder).

## Failure after the audit write

- Refund fails with `Internal`, then retried with the same `refund_id`: the refund applies once. The trail holds two events (at-least-once), consistent with the initiative's no-outbox, no-idempotency-key non-goals.
- Refund refused after the audit write with `FailedPrecondition` (exceeds remainder, invalid state) or `AlreadyExists` (refund id conflict): an event exists for an attempt that changed nothing. Accepted; the audit records the privileged attempt, and pre-checking would duplicate the ledger transaction's own checks outside its lock.
- An admin replay of an already applied `refund_id` writes another event and returns the original refund.

## Clarifications
### Round 1 — 2026-10-15
- `amount` is `RefundPaymentRequest.amount` (int64 minor units) rendered with base-10, e.g. `"200000"`.
- Q1 (permission): uniform rule confirmed; an admin who is the order's seller is audited, no seller lookup.
- Audit events for admin refund attempts refused after the audit write (`FailedPrecondition`, `AlreadyExists`) and for replays are accepted (the locked spec allows post-audit failure rows).
- Gateway check: `PaymentForwarder.RefundPayment` uses `callWrite` (single attempt, no retry); `toConnectErr` maps Unavailable to `unavailable` and PermissionDenied to `permission_denied`, the codes the e2e scenarios assert.

### Round 2 — 2026-10-15

- Q1 [naming] scenario "Audit is called as the service principal" duplicates a title owned by engagement-review-integrity; rename to "Payment audit: Audit is called as the service principal"? → **approved** (human, converge)
