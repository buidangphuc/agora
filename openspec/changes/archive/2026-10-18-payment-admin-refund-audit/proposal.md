## Why

`PaymentService.RefundPayment` (`team-payment/internal/handler/payment.go`) lets any principal holding the `admin` scope move money back to a buyer for any payment, skipping the seller check entirely, and leaves no record of who did it. The q3-audit-log initiative (locked 2026-10-15) requires every state change that succeeds only because of `admin` to write one team-audit event first and to fail closed when the audit write fails. This slice covers the `payment.admin_refund` row of the catalogue. Seller refunds are out of scope by the initiative's non-goals.

initiative: q3-audit-log

## What Changes

- **team-payment**
  - New `internal/upstream/audit.go`, mirroring `team-engagement/internal/upstream/audit.go`:
    - `AuditClient` with `NewAuditClient(addr, timeout)`; empty `UPSTREAM_AUDIT_ADDR` returns `(nil, nil)`.
    - Own interceptor that replaces any outgoing principal with service principal `service-team-payment`, type `service`, scope `audit.write`. The caller's principal is never forwarded. (The existing order client's principal, scope `order.read`, is untouched.)
    - One write method taking actor id, payment id, order id, amount; call bounded by the upstream timeout (2 s), no retry.
  - Config: `UPSTREAM_AUDIT_ADDR` (default empty) and the upstream call timeout in `internal/config/config.go`, documented in `.env.example` and `README.md`.
  - A small `AuditWriter` interface injected into `PaymentHandler` through a handler option (`WithAuditWriter`), so tests use a fake; `cmd/server/main.go` wires it only when the client is non-nil (a nil writer on the admin path means `Unavailable`).
  - `RefundPayment` admin path (caller holds the `admin` scope):
    - Order of checks is unchanged: principal, `payment_id`, `refund_id`, `amount`, transaction lookup (`NotFound`). All of these refuse before any audit write.
    - Then, before `svc.RefundPayment`, the handler writes `action=payment.admin_refund`, `target_type=payment`, `target_id=<payment id>`, `actor_id=<principal id>`, `metadata={order_id, amount}`. `order_id` is the transaction's order; `amount` is the request amount in minor units as a decimal string. `reason` is not copied into the event.
    - Audit error, timeout or nil writer returns `Unavailable` and the ledger, payment status and refund rows are unchanged.
    - The refund itself stays idempotent by `refund_id`: a retry after "audit ok, refund failed" applies the refund exactly once; the trail may then hold one extra event for the same `refund_id` (at-least-once, as in Q2), never a refund without an event.
  - Seller path (no `admin` scope) is unchanged: no audit call, no added latency, not affected by team-audit availability.
- **Compose:** `UPSTREAM_AUDIT_ADDR=team-audit-svc:50066` for `team-payment` is added by `q3-audit-log-contract` in the root `docker-compose.services.yaml`; this slice does not edit it.
- **Proto:** the `platform/audit/v1` copy under `team-payment/proto` and its generated Go are vendored/regenerated from the contract slice's output; this slice never hand-edits proto copies.
- **platform-e2e** (feature and steps owned by this slice; `support/audit_support.py` is owned by `audit-query-filters` and only imported):
  - `features/payment/admin_refund_audit.feature` plus step bindings.
- Not changed: seller refunds, the return/cancel refund consumers, `ProcessMockPayment`, ledger schema, team-order, team-audit, the proto, the gateway.

## NFR

- Added latency on the admin refund path is one `WriteAuditEvent` call bounded by the 2 s upstream timeout, never retried. Seller refunds add no call.
- p95 of an admin `RefundPayment` with team-audit up: audit call p95 < 100 ms in the handler test harness with a stub audit server (non-gating).
- Availability: team-audit down blocks admin refunds only (`Unavailable`, fail closed); seller refunds, payments and payouts are unaffected.
- Limits: unchanged (`refund_id` 1-64 chars, `amount` positive and capped by the refundable remainder).
- Integrity: a refund is never applied without a preceding audit event; idempotency and caps of the existing refund model (`payment.cumulative-refunds`) are preserved.

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| audit `actor_id` (admin user id) | personal (staff identifier) | accountability of a money-moving privileged action | legal obligation / legitimate interest (Decree 13/2023 processing records) | team-audit retention policy (immutable) |
| audit `target_id` (payment id), metadata `order_id` | business identifier | locate the refunded payment and order | same | same |
| audit metadata `amount` | financial, not personal | size of the refund | same | same |
| refund `reason` (existing free text) | existing, unchanged | refund record | contract | unchanged; NOT copied into the audit event |

No buyer name, address, or payment instrument data enters the event. Nothing new goes to analytics or logs.

## Threat model

- **Assets:** buyer and seller money (ledger, `payment_refunds`), the audit trail, the `audit.write` service credential.
- **Abuse cases:**
  - A compromised or rogue admin refunds without a trace: audit is written first and the refund is refused if it cannot be written.
  - Forcing team-audit down or leaving `UPSTREAM_AUDIT_ADDR` unset to refund silently: fail closed with `Unavailable`.
  - Replaying a refund request to double-refund after an audit-then-failure: `refund_id` idempotency returns the same refund; the cap by remainder still applies.
  - Forged audit entries: the caller's principal is never forwarded; `actor_id` comes from the verified Principal, never from the request, which has no actor field.
  - Non-admin non-seller refunding: still `PermissionDenied` before any audit write.
  - Audit-noise by a seller: seller refunds write no event, so sellers cannot flood the trail.
- **Authz rule:** unchanged. `RefundPayment` requires the order's seller (a USER principal) or a principal holding `admin`. Any caller holding `admin` is audited, including an admin who is also the order's seller (clarified rule, see design.md).
