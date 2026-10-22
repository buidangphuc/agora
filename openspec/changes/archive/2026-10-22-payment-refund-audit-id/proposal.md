# payment-refund-audit-id

initiative: privacy-audit-hardening

## Why
`PaymentService.RefundPayment` validates `refund_id` (idempotency key, 1-64 chars of `[A-Za-z0-9._:-]`) but the `payment.admin_refund` audit event drops it: metadata holds only `order_id` and `amount`. An auditor cannot tell two partial refunds of the same amount on one payment apart, nor link an audit row to the ledger refund entry, nor see that a retry (audit written, refund failed, admin retried) is the same refund and not a second one.

## What changes
- `AuditWriter.WriteAdminRefund` (team-payment `internal/handler/payment.go`) gains a `refundID` argument; the handler passes `req.GetRefundId()`.
- `AuditClient.WriteAdminRefund` (`internal/upstream/audit.go`) adds metadata `refund_id` next to `order_id` and `amount`. Action, target (`payment` / payment id), actor and the service-principal call are unchanged.
- Audit still precedes the money movement and still fails closed; a retry with the same `refund_id` writes another `payment.admin_refund` row carrying the same `refund_id` (the ledger applies the refund once); a different `refund_id` yields a different value.
- No proto, migration, gateway or compose change. `AuditService.WriteAuditEvent` is used unchanged (metadata is a free-form map).

## NFR
- No added RPCs or latency: same single audit call, bounded by the existing audit timeout. Metadata grows by at most 64 bytes.
- Availability unchanged: audit down still returns UNAVAILABLE and moves no money.
- Limits: `refund_id` is already validated (<= 64 chars, restricted charset) before the audit write, so only validated values reach metadata.

## Data classification
| field | class | purpose | consent / basis | retention |
|---|---|---|---|---|
| `refund_id` (audit metadata) | pseudonymous client-generated idempotency key, no user data by contract | audit traceability of admin refunds | legitimate interest, security/financial audit (same basis as the existing `payment.admin_refund` row; DPO to confirm, not decided here) | same as the audit row (retention is a DPO item per initiative non-goals) |
Existing fields (`order_id`, `amount`, actor id) are unchanged. The refund `reason` stays out of the event.

## Threat model
- Assets: integrity and completeness of the admin refund audit trail.
- Abuse cases: an admin reuses amounts to obscure which refund was which; a malicious admin supplies a free-text `refund_id` to inject content into audit (mitigated by existing `ValidRefundID`, which runs before the audit write); a retry is mistaken for a double refund (or a double refund hidden as a retry).
- Authz: only principals with scope `admin` reach the audit write; seller refunds write no audit row (unchanged); the audit call uses the team-payment service principal with `audit.write`.
